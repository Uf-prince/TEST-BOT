package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// ─────────────────────────────────────────────────────────────────────────────
// Storj (S3-compatible) message store.
//
// Design rules requested by the owner:
//   • 10 Storj credential/bucket sets (STORJ_ACCESS_KEY_1 .. _10, etc.).
//     Messages are sharded round-robin across the 10 buckets so load + quota
//     are spread. The shard index is derived from hash(msgID) % 10 so a given
//     message always lands in the same bucket (needed for GET/DELETE later).
//   • NOTHING is persisted to the server's local disk or kept in RAM as a
//     cache: the raw protobuf bytes of every incoming message are uploaded
//     straight to Storj, and recovered straight from Storj on delete/edit.
//     The only in-memory state is the minio client objects (no message bytes).
//   • Hard 50 MiB size cap: if a message proto is larger than 50 MiB it is
//     SKIPPED — not stored, and not eligible for antidelete recovery.
//   • Every message object carries metadata: chat, sender (PN), pushName,
//     timestamp (unix ms), mediaType, and sizeBytes — stored both as Storj
//     user-metadata and as a small JSON sidecar prefix in the object body so
//     the TTL guard can list + inspect without downloading the whole object.
//   • Full JSON debugging at every stage (STORJ_PUT / STORJ_GET / STORJ_DELETE
//     / STORJ_LIST / GUARD_TTL / STORJ_INIT) so the owner can verify behaviour
//     from the logs.
// ─────────────────────────────────────────────────────────────────────────────

// storjEndpoint is the Storj-hosted S3-compatible gateway (auto-routes to the
// nearest region). See https://docs.storj.io/dcs/api/s3/s3-compatible-gateway
const storjEndpoint = "gateway.storjshare.io:443"

// maxStorjBytes is the hard cap. 50 MiB == 50 * 1024 * 1024. Anything strictly
// larger is skipped (not stored, not recovered by antidelete).
const maxStorjBytes = 50 * 1024 * 1024

// ttlDuration: objects older than this are deleted by the guard.
const ttlDuration = 48 * time.Hour

// guardInterval: how often the TTL guard wakes up to sweep.
const guardInterval = 24 * time.Hour

type storjShard struct {
	idx     int
	access  string
	secret  string
	bucket  string
	client  *minio.Client
}

type StorjStore struct {
	mu     sync.Mutex
	shards []*storjShard
	rr     uint64 // round-robin counter (not used for placement; hash-based)
	ready  bool
}

var storj = &StorjStore{}

// storjDebug prints a STORJ_* JSON debug line, always-on (not gated by
// GOLDMD_DEBUG) so the owner can verify everything from the logs.
// ALL STORJ DEBUG CONSOLE LOGS — COMMENTED OUT (owner requested all debugs off).
// func storjDebug(stage string, fields map[string]any) {
// 	out := map[string]any{"stage": stage, "ts": time.Now().Format(time.RFC3339Nano)}
// 	for k, v := range fields {
// 		out[k] = v
// 	}
// 	raw, err := json.Marshal(out)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "%s [STORJ-JSON] marshal-err: %v stage=%s\n", time.Now().Format("15:04:05"), err, stage)
// 		return
// 	}
// 	fmt.Fprintf(os.Stderr, "%s [STORJ-JSON] %s\n", time.Now().Format("15:04:05"), string(raw))
// }

// InitStorj loads up to 10 shard credential sets from the environment, creates
// the minio client for each, and ensures the bucket exists. If zero credential
// sets are configured it logs a clear warning and leaves the store not-ready
// (callers must check Ready() before using).
func InitStorj() error {
	storj.mu.Lock()
	defer storj.mu.Unlock()

	storj.shards = storj.shards[:0]
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		access := os.Getenv(fmt.Sprintf("STORJ_ACCESS_KEY_%d", i))
		secret := os.Getenv(fmt.Sprintf("STORJ_SECRET_KEY_%d", i))
		bucket := os.Getenv(fmt.Sprintf("STORJ_BUCKET_%d", i))
		if access == "" || secret == "" || bucket == "" {
			continue
		}

		cli, err := minio.New(storjEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(access, secret, ""),
			Secure: true,
			Region: "us-east-1", // Storj gateway expects a region string; this is a placeholder.
		})
		if err != nil {
   // storjDebug("STORJ_INIT", map[string]any{"shard": i, "bucket": bucket, "error": err.Error(), "ok": false})
			return fmt.Errorf("storj shard %d: new client: %w", i, err)
		}

		// Ensure bucket exists (idempotent).
		exists, err := cli.BucketExists(ctx, bucket)
		if err != nil {
   // storjDebug("STORJ_INIT", map[string]any{"shard": i, "bucket": bucket, "error": err.Error(), "op": "BucketExists", "ok": false})
			return fmt.Errorf("storj shard %d: bucket exists check: %w", i, err)
		}
		if !exists {
			if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
    // storjDebug("STORJ_INIT", map[string]any{"shard": i, "bucket": bucket, "error": err.Error(), "op": "MakeBucket", "ok": false})
				return fmt.Errorf("storj shard %d: make bucket: %w", i, err)
			}
   // storjDebug("STORJ_INIT", map[string]any{"shard": i, "bucket": bucket, "ok": true, "op": "MakeBucket", "created": true})
		} else {
   // storjDebug("STORJ_INIT", map[string]any{"shard": i, "bucket": bucket, "ok": true, "op": "BucketExists", "created": false})
		}

		storj.shards = append(storj.shards, &storjShard{
			idx:    i,
			access: access,
			secret: secret,
			bucket: bucket,
			client: cli,
		})
	}

	if len(storj.shards) == 0 {
  // storjDebug("STORJ_INIT", map[string]any{"ok": false, "reason": "no STORJ_* env credentials found", "shardCount": 0})
		return errors.New("storj: no credential sets configured (STORJ_ACCESS_KEY_1..10 / STORJ_SECRET_KEY_1..10 / STORJ_BUCKET_1..10)")
	}
	storj.ready = true
 // storjDebug("STORJ_INIT", map[string]any{"ok": true, "shardCount": len(storj.shards), "buckets": storjBucketList(), "maxBytes": maxStorjBytes, "ttlHours": int(ttlDuration.Hours()), "guardIntervalHours": int(guardInterval.Hours())})
	return nil
}

func storjBucketList() []string {
	out := make([]string, 0, len(storj.shards))
	for _, s := range storj.shards {
		out = append(out, s.bucket)
	}
	return out
}

// Ready reports whether Storj is initialised.
func (ss *StorjStore) Ready() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.ready && len(ss.shards) > 0
}

// shardForID deterministically picks the shard for a given message ID using a
// stable hash so a later GET/DELETE for the same ID hits the same bucket.
func (ss *StorjStore) shardForID(id string) *storjShard {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if len(ss.shards) == 0 {
		return nil
	}
	var h uint32 = 2166136261 // FNV-1a 32-bit
	for i := 0; i < len(id); i++ {
		h ^= uint32(id[i])
		h *= 16777619
	}
	return ss.shards[int(h)%len(ss.shards)]
}

// objectKey builds the Storj object key for a message ID. The timestamp prefix
// lets the TTL guard list cheaply in chronological order.
func storjObjectKey(msgID string, ts time.Time) string {
	return fmt.Sprintf("msgs/%d/%s.pb", ts.UnixMilli(), msgID)
}

// storjMeta holds the per-message metadata stored as both Storj user-metadata
// and as a JSON prefix inside the object body (so the guard can inspect it
// without a full GET).
type storjMeta struct {
	MsgID      string `json:"msg_id"`
	Chat       string `json:"chat"`
	Sender     string `json:"sender"`
	PushName   string `json:"pushName,omitempty"`
	Timestamp  int64  `json:"ts_ms"`
	MediaType  string `json:"mediaType"`
	SizeBytes  int    `json:"sizeBytes"`
	ShardIdx   int    `json:"shardIdx"`
	Bucket     string `json:"bucket"`
}

// PutMessage serialises the message proto and uploads it to Storj. Returns the
// (bucket, key) handle. If the proto is bigger than maxStorjBytes it is skipped
// (not uploaded) and (skip=true) is returned.
func (ss *StorjStore) PutMessage(ctx context.Context, msgID, chat, sender, pushName string, ts time.Time, msg *waProto.Message, mediaType string) (bucket, key string, skip bool, err error) {
	if !ss.Ready() {
		return "", "", false, errors.New("storj not ready")
	}
	if msgID == "" || msg == nil {
		return "", "", false, errors.New("empty msgID or nil message")
	}

	raw, mErr := proto.Marshal(msg)
	if mErr != nil {
  // storjDebug("STORJ_PUT", map[string]any{"msgID": msgID, "chat": chat, "error": mErr.Error(), "ok": false, "stage_detail": "proto_marshal"})
		return "", "", false, fmt.Errorf("proto marshal: %w", mErr)
	}

	if len(raw) > maxStorjBytes {
  // storjDebug("STORJ_PUT", map[string]any{
			// "msgID": msgID, "chat": chat, "ok": false, "skip": true,
			// "reason": "size > 50MiB", "sizeBytes": len(raw), "maxBytes": maxStorjBytes,
		// })
		return "", "", true, nil
	}

	shard := ss.shardForID(msgID)
	if shard == nil {
		return "", "", false, errors.New("no shard available")
	}
	key = storjObjectKey(msgID, ts)

	meta := storjMeta{
		MsgID: msgID, Chat: chat, Sender: sender, PushName: pushName,
		Timestamp: ts.UnixMilli(), MediaType: mediaType, SizeBytes: len(raw),
		ShardIdx: shard.idx, Bucket: shard.bucket,
	}
	metaJSON, _ := json.Marshal(meta)

	// Object body = 8-byte big-endian meta-length + meta JSON + raw proto.
	// This lets us parse metadata on GET without relying solely on user-metadata.
	var body bytes.Buffer
	mlen := uint32(len(metaJSON))
	body.WriteByte(byte(mlen >> 24))
	body.WriteByte(byte(mlen >> 16))
	body.WriteByte(byte(mlen >> 8))
	body.WriteByte(byte(mlen))
	body.Write(metaJSON)
	body.Write(raw)

	userMeta := map[string]string{
		"msg-id":     msgID,
		"chat":       chat,
		"sender":     sender,
		"ts-ms":      fmt.Sprintf("%d", ts.UnixMilli()),
		"media-type": mediaType,
		"size-bytes": fmt.Sprintf("%d", len(raw)),
	}

	_, err = shard.client.PutObject(ctx, shard.bucket, key, &body, int64(body.Len()), minio.PutObjectOptions{
		ContentType:   "application/octet-stream",
		UserMetadata:  userMeta,
		SendContentMd5: false,
	})
	if err != nil {
  // storjDebug("STORJ_PUT", map[string]any{"msgID": msgID, "chat": chat, "shard": shard.idx, "bucket": shard.bucket, "key": key, "error": err.Error(), "ok": false, "sizeBytes": len(raw)})
		return shard.bucket, key, false, fmt.Errorf("put object: %w", err)
	}

	atomic.AddUint64(&ss.rr, 1)
 // storjDebug("STORJ_PUT", map[string]any{
		// "msgID": msgID, "chat": chat, "sender": sender, "mediaType": mediaType,
		// "shard": shard.idx, "bucket": shard.bucket, "key": key, "sizeBytes": len(raw),
		// "ok": true, "ts": ts.Format(time.RFC3339),
	// })
	return shard.bucket, key, false, nil
}

// parseMetaFromBody reads the 4-byte length prefix + JSON metadata from the
// object body (without consuming the proto part). Returns meta + offset where
// the raw proto begins.
func parseMetaFromBody(data []byte) (storjMeta, int, error) {
	if len(data) < 4 {
		return storjMeta{}, 0, errors.New("body too short for meta length")
	}
	mlen := int(uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]))
	if len(data) < 4+mlen {
		return storjMeta{}, 0, errors.New("body too short for meta json")
	}
	var m storjMeta
	if err := json.Unmarshal(data[4:4+mlen], &m); err != nil {
		return storjMeta{}, 0, fmt.Errorf("meta unmarshal: %w", err)
	}
	return m, 4 + mlen, nil
}

// GetMessage downloads + unmarshals a stored message proto by ID.
func (ss *StorjStore) GetMessage(ctx context.Context, msgID string) (*waProto.Message, storjMeta, error) {
	if !ss.Ready() {
		return nil, storjMeta{}, errors.New("storj not ready")
	}
	if msgID == "" {
		return nil, storjMeta{}, errors.New("empty msgID")
	}
	shard := ss.shardForID(msgID)
	if shard == nil {
		return nil, storjMeta{}, errors.New("no shard")
	}

	// We don't know the exact key (it has a timestamp prefix) so list the
	// shard's msgs/ prefix and match by msgID suffix. The key format is
	// msgs/<ts>/<msgID>.pb so we filter by suffix "/<msgID>.pb".
	suffix := "/" + msgID + ".pb"
	var foundKey string
	objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{
		Prefix:    "msgs/",
		Recursive: true,
	})
	for obj := range objCh {
		if obj.Err != nil {
   // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "error": obj.Err.Error(), "ok": false, "stage_detail": "list"})
			return nil, storjMeta{}, fmt.Errorf("list: %w", obj.Err)
		}
		if strings.HasSuffix(obj.Key, suffix) {
			foundKey = obj.Key
			break
		}
	}
	if foundKey == "" {
  // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "ok": false, "reason": "not_found"})
		return nil, storjMeta{}, errors.New("not found in storj")
	}

	obj, err := shard.client.GetObject(ctx, shard.bucket, foundKey, minio.GetObjectOptions{})
	if err != nil {
  // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "error": err.Error(), "ok": false})
		return nil, storjMeta{}, fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	var body bytes.Buffer
	if _, err := body.ReadFrom(obj); err != nil {
  // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "error": err.Error(), "ok": false, "stage_detail": "read"})
		return nil, storjMeta{}, fmt.Errorf("read body: %w", err)
	}

	meta, protoOff, perr := parseMetaFromBody(body.Bytes())
	if perr != nil {
  // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "error": perr.Error(), "ok": false, "stage_detail": "parse_meta"})
		return nil, storjMeta{}, perr
	}

	var msg waProto.Message
	if err := proto.Unmarshal(body.Bytes()[protoOff:], &msg); err != nil {
  // storjDebug("STORJ_GET", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "error": err.Error(), "ok": false, "stage_detail": "proto_unmarshal"})
		return nil, storjMeta{}, fmt.Errorf("proto unmarshal: %w", err)
	}

 // storjDebug("STORJ_GET", map[string]any{
		// "msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey,
		// "ok": true, "mediaType": meta.MediaType, "sizeBytes": meta.SizeBytes, "chat": meta.Chat, "sender": meta.Sender, "ts": time.UnixMilli(meta.Timestamp).Format(time.RFC3339),
	// })
	return &msg, meta, nil
}

// DeleteMessage removes a stored message by ID (used after antidelete recovery
// so the "guard" lifts and sleeps — the recovered message is purged from Storj).
func (ss *StorjStore) DeleteMessage(ctx context.Context, msgID, reason string) error {
	if !ss.Ready() {
		return errors.New("storj not ready")
	}
	if msgID == "" {
		return errors.New("empty msgID")
	}
	shard := ss.shardForID(msgID)
	if shard == nil {
		return errors.New("no shard")
	}
	suffix := "/" + msgID + ".pb"
	var foundKey string
	objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{Prefix: "msgs/", Recursive: true})
	for obj := range objCh {
		if obj.Err != nil {
   // storjDebug("STORJ_DELETE", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "error": obj.Err.Error(), "ok": false, "stage_detail": "list"})
			return obj.Err
		}
		if strings.HasSuffix(obj.Key, suffix) {
			foundKey = obj.Key
			break
		}
	}
	if foundKey == "" {
  // storjDebug("STORJ_DELETE", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "ok": false, "reason": "not_found", "detail": reason})
		return nil // already gone — treat as success
	}
	if err := shard.client.RemoveObject(ctx, shard.bucket, foundKey, minio.RemoveObjectOptions{}); err != nil {
  // storjDebug("STORJ_DELETE", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "error": err.Error(), "ok": false, "detail": reason})
		return err
	}
 // storjDebug("STORJ_DELETE", map[string]any{"msgID": msgID, "shard": shard.idx, "bucket": shard.bucket, "key": foundKey, "ok": true, "detail": reason})
	return nil
}

// StartTTLGuard launches a background goroutine that, every guardInterval,
// lists all objects across all shards and deletes any older than ttlDuration.
// This is the "guard" — it wakes every 24h, sweeps expired (48h+) messages.
func (ss *StorjStore) StartTTLGuard() {
	if !ss.Ready() {
  // storjDebug("GUARD_TTL", map[string]any{"ok": false, "reason": "storj not ready"})
		return
	}
	go func() {
		ticker := time.NewTicker(guardInterval)
		defer ticker.Stop()
		// Run once at startup too (in case the bot was down for a while).
		ss.runTTLSweep()
		for range ticker.C {
			ss.runTTLSweep()
		}
	}()
 // storjDebug("GUARD_TTL", map[string]any{"ok": true, "started": true, "intervalHours": int(guardInterval.Hours()), "ttlHours": int(ttlDuration.Hours())})
}

func (ss *StorjStore) runTTLSweep() {
	if !ss.Ready() {
		return
	}
	ctx := context.Background()
	cutoff := time.Now().Add(-ttlDuration).UnixMilli()
	totalChecked := 0
	totalDeleted := 0
	deletedIDs := []string{}

	ss.mu.Lock()
	shards := make([]*storjShard, len(ss.shards))
	copy(shards, ss.shards)
	ss.mu.Unlock()

	for _, shard := range shards {
		objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{Prefix: "msgs/", Recursive: true})
		for obj := range objCh {
			if obj.Err != nil {
    // storjDebug("GUARD_TTL", map[string]any{"shard": shard.idx, "bucket": shard.bucket, "error": obj.Err.Error(), "ok": false, "stage_detail": "list"})
				continue
			}
			totalChecked++
			// Parse timestamp from key: msgs/<ts>/<msgID>.pb
			parts := strings.SplitN(obj.Key, "/", 3)
			if len(parts) < 3 {
				continue
			}
			var tsMs int64
			fmt.Sscanf(parts[1], "%d", &tsMs)
			if tsMs == 0 {
				continue
			}
			if tsMs < cutoff {
				// Expired — delete.
				if err := shard.client.RemoveObject(ctx, shard.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
     // storjDebug("GUARD_TTL", map[string]any{"shard": shard.idx, "bucket": shard.bucket, "key": obj.Key, "error": err.Error(), "ok": false, "stage_detail": "delete"})
					continue
				}
				totalDeleted++
				// Extract msgID from key for debug.
				msgID := strings.TrimSuffix(parts[2], ".pb")
				deletedIDs = append(deletedIDs, msgID)
			}
		}
	}

 // storjDebug("GUARD_TTL", map[string]any{
		// "ok": true, "stage": "sweep_done", "cutoffMs": cutoff,
		// "checked": totalChecked, "deleted": totalDeleted,
		// "deletedIDs": deletedIDs,
	// })
}

// MetaOnly downloads just the metadata for a message (used by the guard for
// quick inspection without pulling the full proto). Currently the guard uses
// the key-embedded timestamp instead, so this is reserved for future use.
func (ss *StorjStore) MetaOnly(ctx context.Context, msgID string) (storjMeta, error) {
	_, meta, err := ss.GetMessage(ctx, msgID)
	return meta, err
}

// ───────────────────────────────────────────────────────────────────────────
//   GOLD-MD — Storj JSON key/value store (used by .automsg config persistence)
//
//   A small generic JSON put/get/delete layer that reuses the SAME Storj
//   shards (same account keys + buckets as antidelete) but lives under the
//   "automsg/" object prefix instead of "msgs/". This keeps the auto-message
//   config out of the protobuf message path while still using the owner's
//   configured Storj credentials.
//
//   Key naming:  automsg/<id>.json
//     id is caller-chosen (e.g. "<botJID>:<chat>"). The shard is picked with
//     the same FNV-1a hash as messages so a given id always maps to the same
//     bucket (needed for GET/DELETE).
// ───────────────────────────────────────────────────────────────────────────

// automsgObjectKey builds the Storj object key for an automsg config id.
func automsgObjectKey(id string) string {
	return "automsg/" + id + ".json"
}

// PutJSON stores an arbitrary JSON byte payload under the automsg/<id>.json
// key. Used by .automsg to persist the {duration, mode, msg, chat} config so
// a repeat schedule survives a server restart. Returns nil on success.
func (ss *StorjStore) PutJSON(ctx context.Context, id string, data []byte) error {
	if !ss.Ready() {
		return errors.New("storj not ready")
	}
	if id == "" {
		return errors.New("empty id")
	}
	shard := ss.shardForID(id)
	if shard == nil {
		return errors.New("no shard available")
	}
	key := automsgObjectKey(id)
	_, err := shard.client.PutObject(ctx, shard.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	return err
}

// GetJSON retrieves the JSON byte payload stored under automsg/<id>.json.
// Returns (data, true, nil) on success; (nil, false, nil) if not found;
// (nil, false, err) on a real error. The boolean lets callers distinguish
// "no config set" from "storage error".
func (ss *StorjStore) GetJSON(ctx context.Context, id string) ([]byte, bool, error) {
	if !ss.Ready() {
		return nil, false, errors.New("storj not ready")
	}
	if id == "" {
		return nil, false, errors.New("empty id")
	}
	shard := ss.shardForID(id)
	if shard == nil {
		return nil, false, errors.New("no shard")
	}
	key := automsgObjectKey(id)
	obj, err := shard.client.GetObject(ctx, shard.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" || strings.Contains(err.Error(), "NoSuchKey") ||
			strings.Contains(err.Error(), "not found") {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer obj.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(obj); err != nil {
		return nil, false, fmt.Errorf("read body: %w", err)
	}
	return body.Bytes(), true, nil
}

// DeleteJSON removes the automsg/<id>.json object. Idempotent — returns nil
// if the object does not exist. Used by .automsg once (after send) and by
// .automsg stop (cancel + safe delete config).
func (ss *StorjStore) DeleteJSON(ctx context.Context, id string) error {
	if !ss.Ready() {
		return errors.New("storj not ready")
	}
	if id == "" {
		return errors.New("empty id")
	}
	shard := ss.shardForID(id)
	if shard == nil {
		return errors.New("no shard")
	}
	key := automsgObjectKey(id)
	return shard.client.RemoveObject(ctx, shard.bucket, key, minio.RemoveObjectOptions{})
}

// ListJSON enumerates every automsg/<id>.json object across ALL shards and
// returns a slice of {ID, Data} pairs. Used by .automsg list / .automsg delete
// so the owner can see every saved schedule and delete one by number.
//
// It scans every shard (prefix "automsg/"), downloads each object's body, and
// strips the "automsg/" prefix + ".json" suffix to recover the raw id.
func (ss *StorjStore) ListJSON(ctx context.Context) ([]AutomsgEntry, error) {
	if !ss.Ready() {
		return nil, errors.New("storj not ready")
	}
	var out []AutomsgEntry
	for _, shard := range ss.shards {
		objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{
			Prefix:    "automsg/",
			Recursive: true,
		})
		for obj := range objCh {
			if obj.Err != nil {
				continue
			}
			// recover id from key: "automsg/<id>.json" → "<id>"
			id := strings.TrimPrefix(obj.Key, "automsg/")
			id = strings.TrimSuffix(id, ".json")
			if id == "" {
				continue
			}
			// download the body
			o, err := shard.client.GetObject(ctx, shard.bucket, obj.Key, minio.GetObjectOptions{})
			if err != nil {
				continue
			}
			var body bytes.Buffer
			_, rerr := body.ReadFrom(o)
			o.Close()
			if rerr != nil {
				continue
			}
			out = append(out, AutomsgEntry{ID: id, Data: body.Bytes()})
		}
	}
	return out, nil
}

// AutomsgEntry is one saved schedule returned by ListJSON.
type AutomsgEntry struct {
	ID   string
	Data []byte
}
