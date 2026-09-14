package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ══════════════════ (merged from storj.go) ══════════════════
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

// egUploadCapBytes — env-gated egress cap (bandwidth jugaad). Render free tier
// (5GB/month metered egress) pe har message proto upload karna bandwidth kha
// jata hai. GOLDMD_STORJ_MAX_UPLOAD_KB set karo to us size (KB) se bade proto
// upload hi nahi honge (skip ho jayenge). DEFAULT ON = 256KB (CID mode,
// owner request — Render bandwidth bachao) — text/chat protos
// (2-20KB) normally upload hote rahenge, media-heavy protos (MBs) skip.
// "51200" = purana behaviour (50MB, sab kuch upload).
//
// NOTE: ye sirf *message-archival* uploads (antidelete/view-once backup) ko
// gate karta hai — WhatsApp media send (Client.Upload) is se alag path hai.
var egUploadCapBytes int64 = func() int64 {
	v := strings.TrimSpace(os.Getenv("GOLDMD_STORJ_MAX_UPLOAD_KB"))
	if v == "" {
		// OWNER REQUEST (Render free-bandwidth plan): CID mode default ON.
		// Default cap 256KB - text/chat protos (2-20KB) normally upload
		// hote rahenge (antidelete/view-once recovery 100% kaam karta hai
		// - real media bytes proto me nahi hote, sirf URL+keys ~5-15KB),
		// media-heavy protos (MBs) skip - Storj egress ~80% kam.
		// Purana behaviour chahiye to GOLDMD_STORJ_MAX_UPLOAD_KB=51200 set karo.
		return 256 * 1024
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n <= 0 {
			return maxStorjBytes // "0" ya negative = purana behaviour
		}
		return n * 1024
	}
	return 512 * 1024
}()

// ttlDuration: objects older than this are deleted by the guard.
const ttlDuration = 48 * time.Hour

// guardInterval: how often the TTL guard wakes up to sweep.
const guardInterval = 24 * time.Hour

type storjShard struct {
	idx    int
	access string
	secret string
	bucket string
	client *minio.Client
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

// hardcodedStorjShards — embedded fallback credentials (mirror of .env) so
// antidelete/antiedite + automsg backups keep working on hosts where the
// .env file is skipped (Modal, ephemeral Docker hosts). Real env vars
// (STORJ_ACCESS_KEY_1..10 etc.) always win when present.
var hardcodedStorjShards = [10][3]string{
	{"jwfjua62w45i5ebrx3t5nex455ma", "j2d2e4gjkc43m5sosfe7bznevm25aq627hgljdfhjofr5ezrsxazk", "umar"},
	{"jwpiqh2d4ky56hrjrrhrpz7h47cq", "j2n53jczyngsdci5vpr47ni24mlkdhzzrh2deyq2h4qelsst62da6", "umar2"},
	{"jvkcczaogv7syhlhyss2oot3dliq", "j327kiloo7yhd4x6n6epcvfbzfjuekjrf6bc237cesrv3nohh5kkc", "umar3"},
	{"jwknkytkfzph7mtonxq6g6d4ze3q", "jz24qglsu5wl34kmbsgchpgt2fzyibbdwyyehvqbe6pxvv4pzkpb2", "umar4"},
	{"jwjsgr627fnccmxfc4gtllg5bb7q", "jzihzet2ecmyn3inuglzvxldx3d5i6jnsrs4ky35nsr5tenxro7hg", "umar5"},
	{"jxzvkrhsaebljlko6dv2lin7x4wa", "j3iyzcwnbirmrhup3hc6352gl5n56xfhkna2l4dn3ugm4i7a2gd6s", "umar6"},
	{"jw6pkivs3vp6rmdty2da36auzimq", "j33i2ybq7kd7ouw6w7ltfmew2dzeg2t3v4ryterop75kdeyjdvvvo", "umar7"},
	{"ju4a4oqbejb3w7ygbmikkr4vgsna", "jzo2xqmutggpkbwxpgw5fswerf35miykdavdcfimmghqdpkgyqpxe", "umar8"},
	{"juznozmcpfsbwoqboijqwpus3raa", "j236o3cx4eud3dxraq55lnsnod4aradltsekqsbwk2cv6iebbtwma", "umar9"},
	{"ju7o5eflwumsaxxhdgdy6y23nbsq", "j3oulw7wfaequvvm5ims7xzjdkr5kfqgwecogozv4v25r72ffysog", "umar10"},
}

// InitStorj loads up to 10 shard credential sets from the environment (with
// the hardcoded fallbacks above), creates the minio client for each, and
// ensures the bucket exists. If zero credential sets are configured it logs
// a clear warning and leaves the store not-ready (callers must check Ready()
// before using).
func InitStorj() error {
	storj.mu.Lock()
	defer storj.mu.Unlock()

	storj.shards = storj.shards[:0]
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		access := os.Getenv(fmt.Sprintf("STORJ_ACCESS_KEY_%d", i))
		secret := os.Getenv(fmt.Sprintf("STORJ_SECRET_KEY_%d", i))
		bucket := os.Getenv(fmt.Sprintf("STORJ_BUCKET_%d", i))
		// .env skipped on some hosts (Modal etc.) — use embedded fallbacks
		if access == "" || secret == "" || bucket == "" {
			access = hardcodedStorjShards[i-1][0]
			secret = hardcodedStorjShards[i-1][1]
			bucket = hardcodedStorjShards[i-1][2]
		}
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
	MsgID     string `json:"msg_id"`
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	PushName  string `json:"pushName,omitempty"`
	Timestamp int64  `json:"ts_ms"`
	MediaType string `json:"mediaType"`
	SizeBytes int    `json:"sizeBytes"`
	ShardIdx  int    `json:"shardIdx"`
	Bucket    string `json:"bucket"`
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
	// Bandwidth jugaad - egress cap (GOLDMD_STORJ_MAX_UPLOAD_KB). Bade proto
	// Storj pe upload nahi honge (metered egress bachao), chhote text protos
	// (2-20KB) normally chalte rahenge. skip=true antidelete recover is msg
	// pe kaam nahi karega - free-tier hosting pe acceptable tradeoff.
	if len(raw) > int(egUploadCapBytes) {
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
		ContentType:    "application/octet-stream",
		UserMetadata:   userMeta,
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

// ══════════════════ (merged from upstash.go) ══════════════════
// ============================================================================
// GOLD-MD — Storage layer (Storj-backed; Upstash Redis FULLY REMOVED)
//
// OWNER REQUEST: Redis (Upstash) ko poora hatao. Jo data pehle Redis me
// jata tha (prefix, sudo, banned, premium, settings, session DB blob,
// JID registry) wo SAB ab STORJ me store hota hai — bilkul waise hi
// jaise antidelete/antiedit apne messages Storj me safe karta hai
// (storj.go — 10 shards, minio S3 client, hardcoded fallback creds).
//
// API SURFACE UNCHANGED: struct ka naam (Upstash) aur saare method
// signatures EXACTLY wahi hain jo pehle the, taake manager.go / handler.go
// / commands_loader.go ke 100+ call sites ko touch na karna pade. Sirf
// internal implementation badla: cmd() ab Upstash REST API ki jagah
// Storj kv/ objects read/write karta hai.
//
// Storage layout (TTL-guard SAFE — guard sirf msgs/ prefix sweep karta hai):
//   kv/strings/<key>                 → string value   (GET/SET/DEL)
//   kv/sets/<key>/<member>           → one object per set member
//                                      (SADD/SREM/SMEMBERS/SISMEMBER)
//   kv/hashes/<hash>/<field>         → one object per hash field, body=value
//                                      (HSET/HGET/HDEL/HGETALL/HEXISTS)
//
// Session DB blob : kv/strings/goldmd:sessiondb:<serverID>:blob (base64 sqlite)
// Session JID set : kv/sets/goldmd:sessiondb:<serverID>:jids/<jid>
//
// kv/ data has NO TTL — permanent, aur 48h message TTL guard ka scope
// msgs/ prefix tak seemit hai, is liye ye data kabhi sweep nahi hota.
// ============================================================================

type Upstash struct {
	// Kept for API compatibility — no longer used (no REST endpoint).
	BaseURL string
	Token   string

	// BANDWIDTH JUGAD (hash-skip state): last successfully-uploaded
	// session-DB sha256 — unchanged DB → zero Storj upload.
	dbHashMu sync.Mutex
	dbHash   string

	// serverID namespaces the session-DB persistence keys so each
	// deployment restores only its own sessions (see resolveServerID).
	serverID string

	// in-memory cache (TTL below) — same as db.js.
	// The cache is the #1 speed optimisation: every GetSetting / GetPrefix
	// hits memory (0ms) instead of a synchronous S3 round-trip to Storj.
	mu    sync.Mutex
	cache map[string]cacheEntry

	// refreshCtx / refreshCancel control the background cache refresher
	// goroutine.
	refreshCtx    context.Context
	refreshCancel context.CancelFunc

	// ── Memory-guard safe re-fetch ──
	refetchMu      sync.Mutex
	refetching     bool
	pendingUpdates map[string]string // cacheKey → user's fresh value

	// warmedGroups tracks which group settings hashes have been warmed
	// (one HGETALL per group per process lifetime) — see WarmGroupSettings.
	warmedGroups map[string]bool

	// ── STORJ WRITE-SAFETY: background retry queue ──
	// Har failed Storj write (SET/DEL/SADD/SREM/HSET/HDEL) yahan queue hota
	// hai aur background ticker (30s) usay retry karta rehta hai jab tak
	// Storj me permanently save na ho jaye. Is se user ki settings kabhi
	// loss nahi hotin (Storj temporarily down hone par bhi). Owner order:
	// "background me storj me b safe hote rhe".
	retryMu  sync.Mutex
	retryOps []storjRetryOp
}

// storjRetryOp ek failed write operation jo Storj me baad me save hoga.
type storjRetryOp struct {
	args []string
	ts   time.Time
}

func (u *Upstash) sessionDBKey() string {
	return sessionDBKeyConst + u.serverID + sessionDBKeySuffix
}

func (u *Upstash) sessionJidsKey() string {
	return sessionJidsKeyConst + u.serverID + sessionJidsKeySuffix
}

type cacheEntry struct {
	value string
	ts    time.Time
}

const upstashCacheTTL = 3 * time.Minute

// upstashRefreshInterval is how often the background refresher wakes up.
const upstashRefreshInterval = 2 * time.Minute

func resolveServerID() string {
	// Per-server UNIQUE ID — fleetSelfID wali chain (GOLDMD_SERVER_ID ->
	// RENDER_EXTERNAL_URL -> RENDER_INSTANCE_ID -> hostname). Pehle ye
	// sirf GOLDMD_SERVER_ID -> "svr1" default tha, is liye bina env ke
	// SAB Render services EK HI "svr1" session-DB blob key share karti
	// thein — har reboot par kisi bhi doosre server ka pura DB restore
	// ho sakta tha (session conflict / double-connect logout). Ab har
	// deployment ka apna namespaced blob hota hai.
	if v := strings.TrimSpace(os.Getenv("GOLDMD_SERVER_ID")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); v != "" {
		return strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	}
	if v := strings.TrimSpace(os.Getenv("RENDER_INSTANCE_ID")); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "svr1"
}

const sessionDBKeyConst = "goldmd:sessiondb:"
const sessionJidsKeyConst = "goldmd:sessiondb:"
const sessionDBKeySuffix = ":blob"
const sessionJidsKeySuffix = ":jids"

// NewUpstash keeps the old signature (main.go compatibility) but now only
// needs Storj (global `storj` in storj.go) to be initialised. The url/token
// args are ignored — kept so existing call sites don't break.
func NewUpstash(url, token string) *Upstash {
	sid := resolveServerID()
	InfoLog("Storj-backed config storage ready (serverID=%q) — Redis fully removed", sid)
	ctx, cancel := context.WithCancel(context.Background())
	u := &Upstash{
		serverID:       sid,
		cache:          map[string]cacheEntry{},
		refreshCtx:     ctx,
		refreshCancel:  cancel,
		pendingUpdates: map[string]string{},
	}
	go u.startCacheRefresher()
	return u
}

// ─────────────────────────────────────────────────────────────────────────────
// Low-level command pipeline — Storj S3-backed implementation.
//
// Redis commands translated to Storj kv/ object operations. Return values
// are JSON-shaped EXACTLY like Upstash REST used to return them (quoted
// strings / JSON arrays / "1" / "0" / null) so every caller that does
// trimQuotes / json.Unmarshal keeps working unchanged.
// ─────────────────────────────────────────────────────────────────────────────

const (
	kvStringsPrefix = "kv/strings/"
	kvSetsPrefix    = "kv/sets/"
	kvHashesPrefix  = "kv/hashes/"
)

// kvEncode URL-encodes an unsafe key component (JIDs contain ':', '@', '#',
// spaces etc.) so it is always a single, safe S3 path component. '/' is
// escaped to %2F so a key can never fan out into nested prefixes.
func kvEncode(s string) string {
	return url.PathEscape(s)
}

func kvDecode(s string) string {
	out, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return out
}

// cmd is the public entry point (all internal + external callers use this).
// It dispatches via cmdCore and — CRITICAL for data safety — enqueues any
// FAILED write op (SET/DEL/SADD/SREM/HSET/HDEL) into the background retry
// queue so user settings are never lost when Storj hiccups (temporarily
// down, network blip). Reads are never queued (retrying a read is useless —
// the caller already got the def/fallback value and the cache holds it).
func (u *Upstash) cmd(args ...string) (json.RawMessage, error) {
	res, err := u.cmdCore(args...)
	if err != nil {
		if storjWriteRetryable(args) {
			u.queueWriteRetry(args)
		}
		return res, err
	}
	// WRITE-SUCCESS: is nayi (latest) write ne jo purane queued retries
	// supersede kar diye unko queue se hata do — warna 30s baad purana
	// failed op replay hokar user ki NAYI value overwrite kar deta
	// (RACE: "HSET mode off" fail→queued, phir "HSET mode on" success
	// → replay "off" = data loss).
	if storjWriteRetryable(args) {
		u.purgeSupersededRetry(args)
	}
	return res, nil
}

// cmdCore is the central dispatcher — mirrors the old Upstash REST pipeline.
// Signature kept identical (variadic string args) so direct call sites
// (commands_loader.go: cmd("DEL", key)) keep working.
func (u *Upstash) cmdCore(args ...string) (json.RawMessage, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("kv: empty command")
	}
	op := strings.ToUpper(args[0])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch op {
	case "PING":
		return u.kvPing(ctx)
	case "GET":
		if len(args) < 2 {
			return nil, fmt.Errorf("kv GET: missing key")
		}
		return u.kvGet(ctx, args[1])
	case "SET":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv SET: missing key/val")
		}
		return u.kvSet(ctx, args[1], args[2])
	case "DEL":
		if len(args) < 2 {
			return nil, fmt.Errorf("kv DEL: missing key")
		}
		res, err := u.kvDel(ctx, args[1])
		if err == nil {
			// CACHE-SYNC FIX: delete ka asar foran — empty sentinel. (Key SHAPE
			// ka pata hota hai to sirf wahi key ki cache hati hai — "settings:"
			// hash fields apne Set/Del handlers se alag handle hoti hain.)
			u.cacheDel("setmembers:" + args[1])
		}
		return res, err
	case "SADD":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv SADD: missing members")
		}
		res, err := u.kvSAdd(ctx, args[1], args[2:])
		if err == nil {
			// CACHE-SYNC FIX: nayi member list FORAN cache me (invalidation ki
			// jagah optimistic update) — next read 0ms me NAYI list degi, ~1s
			// S3 round-trip nahi. Storj me write already ho chuka hai (success
			// path), 2-min refresher baad me revalidate karega.
			u.setCacheApplyAdd(args[1], args[2:])
		}
		return res, err
	case "SREM":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv SREM: missing members")
		}
		res, err := u.kvSRem(ctx, args[1], args[2:])
		if err == nil {
			// CACHE-SYNC FIX: removed members FORAN cached list se hat jate hain
			// — next read 0ms me NAYI (chhoti) list degi. Storj updated hai,
			// refresher revalidate karega.
			u.setCacheApplyRem(args[1], args[2:])
		}
		return res, err
	case "SMEMBERS":
		if len(args) < 2 {
			return nil, fmt.Errorf("kv SMEMBERS: missing key")
		}
		return u.kvSMembers(ctx, args[1])
	case "SISMEMBER":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv SISMEMBER: missing member")
		}
		return u.kvSIsMember(ctx, args[1], args[2])
	case "HSET":
		if len(args) < 4 {
			return nil, fmt.Errorf("kv HSET: missing field/val")
		}
		return u.kvHSet(ctx, args[1], args[2], args[3])
	case "HGET":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv HGET: missing field")
		}
		return u.kvHGet(ctx, args[1], args[2])
	case "HDEL":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv HDEL: missing field")
		}
		return u.kvHDel(ctx, args[1], args[2])
	case "HGETALL":
		if len(args) < 2 {
			return nil, fmt.Errorf("kv HGETALL: missing key")
		}
		return u.kvHGetAll(ctx, args[1])
	case "HEXISTS":
		if len(args) < 3 {
			return nil, fmt.Errorf("kv HEXISTS: missing field")
		}
		return u.kvHExists(ctx, args[1], args[2])
	}
	return nil, fmt.Errorf("kv: unsupported command %q", op)
}

// ── shard helper (storj.go's global store + deterministic sharding) ──

func (u *Upstash) kvShard(key string) *storjShard {
	if !storj.Ready() {
		return nil
	}
	return storj.shardForID(key)
}

// kvRead fully reads an object; (nil,false,nil) when the key doesn't exist.
func (u *Upstash) kvRead(ctx context.Context, shard *storjShard, key string) ([]byte, bool, error) {
	obj, err := shard.client.GetObject(ctx, shard.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if minioIsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		if minioIsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func (u *Upstash) kvPing(ctx context.Context) (json.RawMessage, error) {
	if !storj.Ready() {
		return nil, fmt.Errorf("kv: storj not initialised")
	}
	shard := storj.shardForID("ping")
	if shard == nil {
		return nil, fmt.Errorf("kv: no shard")
	}
	if _, err := shard.client.BucketExists(ctx, shard.bucket); err != nil {
		return nil, err
	}
	return json.RawMessage(`"PONG"`), nil
}

// ── BANDWIDTH JUGAD (transparent gzip on kv/strings) ─────────────────────
// Render free tier me PutObject ka BODY hi metered egress hai (5GB/month).
// Session-DB blob + fleet blobs roz 144× base64-sqlite (MBs) jaate the.
// Ab badi string values Storj pe "GZ1:"+gzip body ke sath jaati hain —
// base64 text gzip me ~8-15x chhoti ho jati hai. Read side (kvGet) isko
// TRANSPARENT decompress karta hai, is liye value semantics 100% same
// hain aur mixed fleet (jo servers abhi naya binary nahi uthaye) bhi
// bilkul theek chalega — unko wahi value milegi jo pehle milti thi.
const kvGzPrefix = "GZ1:"
const kvGzMinBytes = 4096 // 4KB se chhoti values (settings etc.) plain hi

// kvGzipEnabled — EMERGENCY OFF (owner: "sare sessions offline").
// Mixed-fleet me purane binaries GZ1 payload NAHI padh sakte (plain base64
// expect karte hain — shared fleet blob keys pe failover toot gaya tha).
// Default OFF = wire format EXACTLY legacy. Enable sirf tab jab saare
// servers naya binary chala rahe hon: GOLDMD_KV_GZIP=1.
var kvGzipEnabled = strings.TrimSpace(os.Getenv("GOLDMD_KV_GZIP")) == "1"

// kvGzipMaybe compresses val when big enough & compression actually helps.
// Returns exact bytes to PUT ("GZ1:"+gzip) — ya original val as-is.
func kvGzipMaybe(val string) string {
	if !kvGzipEnabled {
		return val // EMERGENCY: default OFF — legacy plain wire format
	}
	if len(val) < kvGzMinBytes {
		return val
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(val)); err != nil {
		return val
	}
	if err := gz.Close(); err != nil {
		return val
	}
	comp := buf.Bytes()
	if len(comp)+len(kvGzPrefix) >= len(val) {
		return val // incompressible → plain bhejo (legacy body)
	}
	return kvGzPrefix + string(comp)
}

// kvGunzipMaybe — read-side mirror: "GZ1:"+gzip → original bytes.
// Koi bhi issue (unexpected prefix content / gunzip fail) → raw as-is
// (legacy values kabhi corrupt nahi hote — best-effort fallback).
func kvGunzipMaybe(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte(kvGzPrefix)) {
		return data // legacy plain value — untouched
	}
	zr, err := gzip.NewReader(bytes.NewReader(data[len(kvGzPrefix):]))
	if err != nil {
		return data
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return data
	}
	return out
}

func (u *Upstash) kvGet(ctx context.Context, key string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	data, found, err := u.kvRead(ctx, shard, kvStringsPrefix+kvEncode(key))
	if err != nil {
		return nil, err
	}
	if !found {
		return json.RawMessage(`null`), nil
	}
	// transparent read-side decompress (mixed-fleet safe)
	// + LAZY GZ1 HEAL: purane broken binary (c704231) ki GZ1 value
	// dikhi to decompress ke saath background me PLAIN rewrite bhi —
	// boot-sweep ke BAAD likhi gayi GZ1 values bhi aage-chale aati
	// hain (failover: purana binary bhi session padh le).
	if bytes.HasPrefix(data, []byte(kvGzPrefix)) {
		plain := kvGunzipMaybe(data)
		if len(plain) > 0 && !bytes.HasPrefix(plain, []byte(kvGzPrefix)) {
			go u.healGZ1Lazy(key, kvStringsPrefix+kvEncode(key), plain)
		}
		data = plain
	} else {
		data = kvGunzipMaybe(data) // legacy/plain — as-is
	}
	raw, err := json.Marshal(string(data))
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (u *Upstash) kvSet(ctx context.Context, key, val string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	// BANDWIDTH JUGAD: PUT body gzip jab help kare (badi values) —
	// metered egress 8-15x kam. Value semantics same (read-side decompress).
	body := kvGzipMaybe(val)
	_, err := shard.client.PutObject(ctx, shard.bucket, kvStringsPrefix+kvEncode(key),
		strings.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "text/plain"})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(`"OK"`), nil
}

func (u *Upstash) kvDel(ctx context.Context, key string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	// 1. plain string key
	_ = shard.client.RemoveObject(ctx, shard.bucket, kvStringsPrefix+kvEncode(key), minio.RemoveObjectOptions{})
	// 2. hash field objects (settings etc.)
	u.kvRemovePrefix(ctx, shard, kvHashesPrefix+kvEncode(key)+"/")
	// 3. set member objects
	u.kvRemovePrefix(ctx, shard, kvSetsPrefix+kvEncode(key)+"/")
	return json.RawMessage(`1`), nil
}

// kvRemovePrefix deletes every object under a Storj prefix (used by DEL on
// set/hash keys). Best-effort: errors are ignored so DEL never fails hard.
func (u *Upstash) kvRemovePrefix(ctx context.Context, shard *storjShard, prefix string) {
	objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for obj := range objCh {
		if obj.Err != nil {
			continue
		}
		_ = shard.client.RemoveObject(ctx, shard.bucket, obj.Key, minio.RemoveObjectOptions{})
	}
}

func (u *Upstash) kvSAdd(ctx context.Context, key string, members []string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	n := 0
	for _, m := range members {
		_, err := shard.client.PutObject(ctx, shard.bucket,
			kvSetsPrefix+kvEncode(key)+"/"+kvEncode(m),
			strings.NewReader("1"), 1, minio.PutObjectOptions{ContentType: "text/plain"})
		if err != nil {
			return nil, err
		}
		n++
	}
	return json.RawMessage(fmt.Sprintf(`%d`, n)), nil
}

func (u *Upstash) kvSRem(ctx context.Context, key string, members []string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	n := 0
	for _, m := range members {
		// S3 RemoveObject on a missing key is not an error — callers ignore
		// the count anyway.
		_ = shard.client.RemoveObject(ctx, shard.bucket,
			kvSetsPrefix+kvEncode(key)+"/"+kvEncode(m), minio.RemoveObjectOptions{})
		n++
	}
	return json.RawMessage(fmt.Sprintf(`%d`, n)), nil
}

func (u *Upstash) kvSMembers(ctx context.Context, key string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	prefix := kvSetsPrefix + kvEncode(key) + "/"
	members := []string{}
	objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for obj := range objCh {
		if obj.Err != nil {
			continue
		}
		m := kvDecode(strings.TrimPrefix(obj.Key, prefix))
		if m == "" {
			continue
		}
		members = append(members, m)
	}
	sort.Strings(members)
	raw, err := json.Marshal(members)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (u *Upstash) kvSIsMember(ctx context.Context, key, member string) (json.RawMessage, error) {
	shard := u.kvShard(key)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	_, err := shard.client.StatObject(ctx, shard.bucket,
		kvSetsPrefix+kvEncode(key)+"/"+kvEncode(member), minio.StatObjectOptions{})
	if err != nil {
		if minioIsNotFound(err) {
			return json.RawMessage(`0`), nil
		}
		return nil, err
	}
	return json.RawMessage(`1`), nil
}

func (u *Upstash) kvHSet(ctx context.Context, hash, field, val string) (json.RawMessage, error) {
	shard := u.kvShard(hash)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	_, err := shard.client.PutObject(ctx, shard.bucket,
		kvHashesPrefix+kvEncode(hash)+"/"+kvEncode(field),
		strings.NewReader(val), int64(len(val)), minio.PutObjectOptions{ContentType: "text/plain"})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(`1`), nil
}

func (u *Upstash) kvHGet(ctx context.Context, hash, field string) (json.RawMessage, error) {
	shard := u.kvShard(hash)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	data, found, err := u.kvRead(ctx, shard, kvHashesPrefix+kvEncode(hash)+"/"+kvEncode(field))
	if err != nil {
		return nil, err
	}
	if !found {
		return json.RawMessage(`null`), nil
	}
	raw, err := json.Marshal(string(data))
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (u *Upstash) kvHDel(ctx context.Context, hash, field string) (json.RawMessage, error) {
	shard := u.kvShard(hash)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	// Stat-first so we return an accurate 0/1 count like Redis HDEL.
	_, err := shard.client.StatObject(ctx, shard.bucket,
		kvHashesPrefix+kvEncode(hash)+"/"+kvEncode(field), minio.StatObjectOptions{})
	if err != nil && !minioIsNotFound(err) {
		return nil, err
	}
	existed := err == nil
	_ = shard.client.RemoveObject(ctx, shard.bucket,
		kvHashesPrefix+kvEncode(hash)+"/"+kvEncode(field), minio.RemoveObjectOptions{})
	if existed {
		return json.RawMessage(`1`), nil
	}
	return json.RawMessage(`0`), nil
}

func (u *Upstash) kvHGetAll(ctx context.Context, hash string) (json.RawMessage, error) {
	shard := u.kvShard(hash)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	prefix := kvHashesPrefix + kvEncode(hash) + "/"
	pairs := []string{}
	objCh := shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for obj := range objCh {
		if obj.Err != nil {
			continue
		}
		field := kvDecode(strings.TrimPrefix(obj.Key, prefix))
		if field == "" {
			continue
		}
		data, found, err := u.kvRead(ctx, shard, obj.Key)
		if err != nil || !found {
			continue
		}
		pairs = append(pairs, field, string(data))
	}
	raw, err := json.Marshal(pairs)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (u *Upstash) kvHExists(ctx context.Context, hash, field string) (json.RawMessage, error) {
	shard := u.kvShard(hash)
	if shard == nil {
		return nil, fmt.Errorf("kv: storj not ready")
	}
	_, err := shard.client.StatObject(ctx, shard.bucket,
		kvHashesPrefix+kvEncode(hash)+"/"+kvEncode(field), minio.StatObjectOptions{})
	if err != nil {
		if minioIsNotFound(err) {
			return json.RawMessage(`0`), nil
		}
		return nil, err
	}
	return json.RawMessage(`1`), nil
}

// minioIsNotFound reports whether an S3 error means "object doesn't exist".
func minioIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if resp := minio.ToErrorResponse(err); resp.Code == "NoSuchKey" || resp.Code == "NotFound" {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") ||
		strings.Contains(msg, "The specified key does not exist") ||
		strings.Contains(msg, "not found")
}

// ─────────────────────────────────────────────────────────────────────────────
// Safe wrappers (return fallback on error — like UmarSafe)
// ─────────────────────────────────────────────────────────────────────────────

// getStringKV fetches a plain string value with a FOUND flag (fleet engine
// ke liye — safeString me "key missing" vs "empty value" ka farak nahi
// hota, fleet ko dono me alag behaviour chahiye).
func (u *Upstash) getStringKV(key string) (string, bool) {
	r, err := u.cmd("GET", key)
	if err != nil {
		return "", false
	}
	s := trimQuotes(string(r), "")
	if s == "" || s == "null" {
		return "", false
	}
	return s, true
}

func (u *Upstash) safeString(key, fb string) string {
	r, err := u.cmd("GET", key)
	if err != nil {
		return fb
	}
	return trimQuotes(string(r), fb)
}

func (u *Upstash) setString(key, val string) error {
	_, err := u.cmd("SET", key, val)
	return err
}

func (u *Upstash) setMembers(key string) []string {
	// GROUP-SPEED FIX: set members are now cached in memory (same pattern as
	// GetSetting / PremiumMembersCached). SMEMBERS on Storj is an S3
	// ListObjects round-trip (~0.5-1s) — it was previously paid on EVERY
	// group message (bangcuser banned-user check runs synchronously per
	// message, and the anti-* detectors read their whitelists per message).
	// With the cache the check is 0ms. The cache is invalidated on every
	// write (SADD/SREM/DEL via cmd() below) and re-validated every 2 min by
	// the background refresher (refreshAllSettings) — exactly like the
	// settings hash cache. Behavior is identical: same members, same order
	// (sorted), same nil on error/empty.
	ck := "setmembers:" + key
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" { // sentinel: set exists but is empty
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	r, err := u.cmd("SMEMBERS", key)
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
	} else if b, err := json.Marshal(arr); err == nil {
		u.cacheSet(ck, string(b))
	}
	return arr
}

func (u *Upstash) setAdd(key string, members ...string) error {
	args := append([]string{"SADD", key}, members...)
	_, err := u.cmd(args...)
	if err == nil && key == "premium:set" {
		u.invalidatePremiumCache()
		for _, m := range members {
			u.cacheDel("premium:ismember:" + m)
		}
	}
	if err == nil && key == "banned:set" {
		u.invalidateBannedCache()
		for _, m := range members {
			u.cacheDel("banned:ismember:" + m)
		}
	}
	return err
}

func (u *Upstash) setRem(key string, members ...string) error {
	args := append([]string{"SREM", key}, members...)
	_, err := u.cmd(args...)
	if err == nil && key == "premium:set" {
		u.invalidatePremiumCache()
		for _, m := range members {
			u.cacheDel("premium:ismember:" + m)
		}
	}
	if err == nil && key == "banned:set" {
		u.invalidateBannedCache()
		for _, m := range members {
			u.cacheDel("banned:ismember:" + m)
		}
	}
	return err
}

// ─── optimistic SET-cache updates (CACHE-SYNC FIX) ─────────────────
// Ye helpers SADD/SREM ke SUCCESS ke foran baad chalte hain: cached member
// list me members add/remove karke NAYI list wapas cache me rakh dete hain.
// Is se (1) next read 0ms me nayi values deti hai (koi S3 round-trip nahi),
// aur (2) write ka asar instant dikhta hai — owner order: "user koi bhi
// setting change update kre to wo foran nay settings k sath Naya catch ban
// jana chahye". Agar key abhi cached nahi hai to kuch nahi karte (next read
// khud fresh S3 se padh legi aur cache karegi — pehli read ka normal path).

func (u *Upstash) setCacheApplyAdd(key string, members []string) {
	ck := "setmembers:" + key
	u.mu.Lock()
	e, ok := u.cache[ck]
	u.mu.Unlock()
	if !ok {
		return // not cached yet — first read will fetch+cache fresh
	}
	if e.value == "\x00" {
		// empty set → sirf naye members ki nayi list
		u.cacheSetList(ck, members)
		return
	}
	var arr []string
	if err := json.Unmarshal([]byte(e.value), &arr); err != nil {
		u.cacheDel(ck) // corrupt entry — next read re-fetches
		return
	}
	set := map[string]bool{}
	for _, m := range arr {
		set[m] = true
	}
	changed := false
	for _, m := range members {
		if !set[m] {
			set[m] = true
			changed = true
		}
	}
	if !changed {
		return
	}
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sort.Strings(out)
	u.cacheSetList(ck, out)
}

func (u *Upstash) setCacheApplyRem(key string, members []string) {
	ck := "setmembers:" + key
	u.mu.Lock()
	e, ok := u.cache[ck]
	u.mu.Unlock()
	if !ok {
		return // not cached yet — first read will fetch+cache fresh
	}
	if e.value == "\x00" {
		return // already empty
	}
	var arr []string
	if err := json.Unmarshal([]byte(e.value), &arr); err != nil {
		u.cacheDel(ck) // corrupt entry — next read re-fetches
		return
	}
	rem := map[string]bool{}
	for _, m := range members {
		rem[m] = true
	}
	out := make([]string, 0, len(arr))
	changed := false
	for _, m := range arr {
		if rem[m] {
			changed = true
			continue
		}
		out = append(out, m)
	}
	if !changed {
		return
	}
	u.cacheSetList(ck, out)
}

// cacheSetList JSON-encodes a member list into the cache (empty → \x00
// sentinel) so it stays consistent with setMembers' cache format.
func (u *Upstash) cacheSetList(ck string, arr []string) {
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
		return
	}
	if b, err := json.Marshal(arr); err == nil {
		u.cacheSet(ck, string(b))
	}
}

// ─── Storj write-safety retry queue (owner order) ──────────────────
// "background me storj me b safe hote rhe" — Storj temporarily down ho to
// bhi user ki settings loss na hon. Failed writes yahan queue hote hain,
// 30s background ticker retry karta rehta hai. Retry sirf WRITE ops pe
// (SET/DEL/SADD/SREM/HSET/HDEL) — read retry bekar hai (caller ko def/fallback
// already mil chuka hai, cache me value hai).

func storjWriteRetryable(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToUpper(args[0]) {
	case "SET", "DEL", "SADD", "SREM", "HSET", "HDEL":
		return true
	}
	return false
}

func (u *Upstash) queueWriteRetry(args []string) {
	cp := make([]string, len(args))
	copy(cp, args)
	u.retryMu.Lock()
	// dedup: bilkul same op already queued hai to skip (ops idempotent hain,
	// last-wins — pehli queued entry ke args same hain to dobara queue nahi)
	for i := range u.retryOps {
		op := u.retryOps[i]
		if len(op.args) == len(cp) {
			same := true
			for j := range cp {
				if op.args[j] != cp[j] {
					same = false
					break
				}
			}
			if same {
				u.retryMu.Unlock()
				return
			}
		}
	}
	u.retryOps = append(u.retryOps, storjRetryOp{args: cp, ts: time.Now()})
	n := len(u.retryOps)
	u.retryMu.Unlock()
	if n == 1 || n%20 == 0 {
		InfoLog("Storj write queued for retry (%d pending): op=%s", n, cp[0])
	}
}

// purgeSupersededRetry removes queued retry ops that a JUST-SUCCEEDED write
// (the newest user action) has made stale. Rules:
//   - SET/DEL success on key K: full-key write → saare purane queued ops
//     on K obsolete (key ka naya state already S3 me hai).
//   - HSET/HDEL success on (K, field): same-field queued HSET/HDEL stale
//     (nayi value jeet gayi), aur queued DEL on K bhi stale (purana
//     full-wipe replay naya field-write mita deta).
//   - SADD/SREM success on set K: exact-same queued op redundant, aur
//     queued DEL on K stale (purana clear replay naye members mita deta).
//
// flushWriteRetries me ye purge NAHI hota — wahan FIFO replay (oldest
// first) khud newest value pe converge karta hai, aur beech ka success
// newer queued value ko drop karne ka khatra hota hai.
func (u *Upstash) purgeSupersededRetry(successArgs []string) {
	if len(successArgs) < 2 {
		return
	}
	succOp := strings.ToUpper(successArgs[0])
	key := successArgs[1]
	field := ""
	if (succOp == "HSET" || succOp == "HDEL") && len(successArgs) >= 3 {
		field = successArgs[2]
	}
	u.retryMu.Lock()
	kept := u.retryOps[:0]
	dropped := 0
	for _, q := range u.retryOps {
		if retrySuperseded(q.args, succOp, key, field, successArgs) {
			dropped++
			continue
		}
		kept = append(kept, q)
	}
	u.retryOps = kept
	u.retryMu.Unlock()
	if dropped > 0 {
		InfoLog("Storj retry queue: %d stale op(s) dropped (superseded by newer successful write)", dropped)
	}
}

// retrySuperseded reports whether queued op q is made obsolete by the just
// succeeded write (succOp/key/field/succArgs). Only ops on the SAME key are
// ever superseded — dusri keys ka data untouched.
func retrySuperseded(qArgs []string, succOp, key, field string, succArgs []string) bool {
	if len(qArgs) < 2 {
		return false
	}
	if qArgs[1] != key {
		return false // different key — never superseded
	}
	qOp := strings.ToUpper(qArgs[0])
	switch succOp {
	case "SET", "DEL":
		// full-key write jeet gaya → us key ka purana sab kuch stale
		return true
	case "HSET", "HDEL":
		if field == "" {
			return false
		}
		if qOp == "HSET" || qOp == "HDEL" {
			// same field: nayi (latest) write jeeti — purani queued value stale
			return len(qArgs) >= 3 && qArgs[2] == field
		}
		if qOp == "DEL" {
			// purana full-hash DEL replay naya field-write mita deta — stale
			return true
		}
		return false
	case "SADD", "SREM":
		if qOp == "DEL" {
			// purana set-clear replay naye members mita deta — stale
			return true
		}
		// exactly wahi members wala queued op redundant hai (S3 me already)
		if qOp == succOp && len(qArgs) == len(succArgs) {
			same := true
			for i := range qArgs {
				if qArgs[i] != succArgs[i] {
					same = false
					break
				}
			}
			if same {
				return true
			}
		}
		return false
	}
	return false
}

// flushWriteRetries attempts all queued write ops via cmdCore (NOT cmd —
// warna fail hone pe dobara queue hota rahe). Success wale hata do, fail
// wale agli ticker tick pe dobara try honge. Ops idempotent hain
// (HSET/SADD/SREM/DEL/SET/HDEL — dobara chalana safe hai).
func (u *Upstash) flushWriteRetries() {
	u.retryMu.Lock()
	ops := u.retryOps
	u.retryOps = nil
	u.retryMu.Unlock()
	if len(ops) == 0 {
		return
	}
	var failed []storjRetryOp
	for _, op := range ops {
		if _, err := u.cmdCore(op.args...); err != nil {
			failed = append(failed, op)
		}
	}
	u.retryMu.Lock()
	u.retryOps = append(u.retryOps, failed...)
	left := len(u.retryOps)
	u.retryMu.Unlock()
	_ = left
	if len(failed) > 0 {
		ErrLog("Storj retry: %d/%d writes still pending (will retry in 30s)", len(failed), len(ops))
	} else {
		OkLog("Storj retry: %d queued write(s) saved successfully", len(ops))
	}
}

// ── cache helpers ────────────────────────────────────────────────────────────

func (u *Upstash) cacheGet(key string) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e, ok := u.cache[key]
	if !ok {
		return "", false
	}
	return e.value, true
}

func (u *Upstash) cacheGetFresh(key string) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e, ok := u.cache[key]
	if !ok {
		return "", false
	}
	if time.Since(e.ts) > upstashCacheTTL {
		return "", false
	}
	return e.value, true
}

func (u *Upstash) cacheSet(key, val string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cache[key] = cacheEntry{value: val, ts: time.Now()}
}

func (u *Upstash) setDel(key string) error {
	_, err := u.cmd("DEL", key)
	return err
}

func (u *Upstash) cacheDel(key string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.cache, key)
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API used by session/manager (mirrors db.js) — logic UNCHANGED
// ─────────────────────────────────────────────────────────────────────────────

func (u *Upstash) GetPrefix(jid, def string) string {
	ck := "prefix:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v
	}
	p := u.safeString("prefix:"+jid, def)
	u.cacheSet(ck, p)
	return p
}

func (u *Upstash) SetPrefix(jid, prefix string) {
	u.cacheSet("prefix:"+jid, prefix)
	_ = u.setString("prefix:"+jid, prefix)
	_ = u.setAdd("prefix:keys", jid)
}

func (u *Upstash) IsSudo(jid string) bool {
	r, err := u.cmd("SISMEMBER", "sudo:set", jid)
	if err != nil {
		return false
	}
	return trimQuotes(string(r), "0") == "1"
}

func (u *Upstash) AddSudo(jid string)    { _ = u.setAdd("sudo:set", jid) }
func (u *Upstash) RemoveSudo(jid string) { _ = u.setRem("sudo:set", jid) }
func (u *Upstash) SudoList() []string    { return u.setMembers("sudo:set") }

func (u *Upstash) IsBanned(jid string) bool {
	ck := "banned:ismember:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v == "1"
	}
	r, err := u.cmd("SISMEMBER", "banned:set", jid)
	if err != nil {
		return false
	}
	val := trimQuotes(string(r), "0")
	u.cacheSet(ck, val)
	return val == "1"
}

func (u *Upstash) BannedMembersCached() []string {
	ck := "banned:members"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	r, err := u.cmd("SMEMBERS", "banned:set")
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
	} else {
		if b, err := json.Marshal(arr); err == nil {
			u.cacheSet(ck, string(b))
		}
	}
	return arr
}

func (u *Upstash) CachedBannedTails(botJID string) []string {
	ck := "banned:tails"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var tails []string
		if err := json.Unmarshal([]byte(v), &tails); err == nil {
			return tails
		}
	}
	jids := u.BannedMembersCached()
	if len(jids) == 0 {
		u.cacheSet(ck, "\x00")
		return nil
	}
	tails := make([]string, 0, len(jids))
	for _, jid := range jids {
		num := jid
		if i := strings.IndexByte(jid, '@'); i >= 0 {
			num = jid[:i]
		}
		if raw := u.safeString("goldmd:"+botJID+":bannedmeta:"+jid, ""); raw != "" {
			if m := strings.Index(raw, `"number":`); m >= 0 {
				raw = raw[m+9:]
				if e := strings.Index(raw, `"`); e > 0 {
					raw = raw[:e]
				}
				if raw != "" {
					num = raw
				}
			}
		}
		var digits []byte
		for i := 0; i < len(num); i++ {
			if c := num[i]; c >= '0' && c <= '9' {
				digits = append(digits, c)
			}
		}
		if len(digits) == 0 {
			continue
		}
		if len(digits) > 10 {
			digits = digits[len(digits)-10:]
		}
		tails = append(tails, string(digits))
	}
	if b, err := json.Marshal(tails); err == nil {
		u.cacheSet(ck, string(b))
	}
	return tails
}

func (u *Upstash) invalidateBannedCache() {
	u.cacheDel("banned:members")
	u.cacheDel("banned:tails")
}

func (u *Upstash) BanUser(jid string)   { _ = u.setAdd("banned:set", jid) }
func (u *Upstash) UnbanUser(jid string) { _ = u.setRem("banned:set", jid) }
func (u *Upstash) BannedList() []string { return u.setMembers("banned:set") }

func (u *Upstash) IsPremium(jid string) bool {
	ck := "premium:ismember:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v == "1"
	}
	r, err := u.cmd("SISMEMBER", "premium:set", jid)
	if err != nil {
		return false
	}
	val := trimQuotes(string(r), "0")
	u.cacheSet(ck, val)
	return val == "1"
}

func (u *Upstash) PremiumMembersCached() []string {
	ck := "premium:members"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	r, err := u.cmd("SMEMBERS", "premium:set")
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
	} else {
		if b, err := json.Marshal(arr); err == nil {
			u.cacheSet(ck, string(b))
		}
	}
	return arr
}

func (u *Upstash) invalidatePremiumCache() {
	u.cacheDel("premium:members")
}

func (u *Upstash) GetSetting(jid, field, def string) string {
	ck := "settings:" + jid + ":" + field
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" { // sentinel for "field does not exist"
			return def
		}
		return v
	}
	// Single HGET: null (JSON) for a MISSING field, quoted "" for a field
	// that EXISTS with an empty-string value — same semantics as before.
	r, err := u.cmd("HGET", "settings:"+jid, field)
	if err != nil {
		return def
	}
	if strings.TrimSpace(string(r)) == "null" || len(r) == 0 {
		u.cacheSet(ck, "\x00")
		return def
	}
	val := trimQuotes(string(r), def)
	u.cacheSet(ck, val)
	return val
}

func (u *Upstash) SetSetting(jid, field, val string) {
	ck := "settings:" + jid + ":" + field
	u.cacheSet(ck, val)
	// If a background re-fetch (triggered by ClearCache) is in progress,
	// record this update so the re-fetch does NOT overwrite it. The user's
	// change always wins.
	u.refetchMu.Lock()
	if u.refetching {
		u.pendingUpdates[ck] = val
	}
	u.refetchMu.Unlock()
	_, _ = u.cmd("HSET", "settings:"+jid, field, val)
}

func (u *Upstash) DelSetting(jid, field string) {
	ck := "settings:" + jid + ":" + field
	// CACHE-SYNC FIX: pehle yahan cacheDel hota tha — is se agli read S3
	// round-trip karti thi (~1s). Ab field-delete ka asar FORAN cache me
	// \x00 sentinel (missing-field) ke saath likha jata hai — next read
	// def value 0ms me degi, Storj write background me safe ho jayega.
	u.cacheSet(ck, "\x00")
	// If a background re-fetch (triggered by ClearCache) is in progress,
	// record this update so the re-fetch does NOT overwrite it. The user's
	// change always wins.
	u.refetchMu.Lock()
	if u.refetching {
		u.pendingUpdates[ck] = "\x00"
	}
	u.refetchMu.Unlock()
	_, _ = u.cmd("HDEL", "settings:"+jid, field)
}

// ── background cache refresher ───────────────────────────────────────────────

func (u *Upstash) startCacheRefresher() {
	ticker := time.NewTicker(upstashRefreshInterval)
	defer ticker.Stop()
	// STORJ WRITE-SAFETY: 30s retry ticker — failed writes background me
	// Storj tak pahunchte rehte hain jab tak save na hon (owner order).
	retryTicker := time.NewTicker(30 * time.Second)
	defer retryTicker.Stop()

	for {
		select {
		case <-u.refreshCtx.Done():
			return
		case <-ticker.C:
			u.refreshAllSettings()
		case <-retryTicker.C:
			u.flushWriteRetries()
		}
	}
}

func (u *Upstash) refreshAllSettings() {
	u.refetchMu.Lock()
	busy := u.refetching
	u.refetchMu.Unlock()
	if busy {
		return
	}

	u.mu.Lock()
	var keys []string
	var setKeys []string
	for k := range u.cache {
		if strings.HasPrefix(k, "settings:") {
			keys = append(keys, k)
		} else if strings.HasPrefix(k, "setmembers:") { // GROUP-SPEED FIX
			setKeys = append(setKeys, k)
		}
	}
	u.mu.Unlock()

	if len(keys) == 0 && len(setKeys) == 0 {
		return
	}

	// GROUP-SPEED FIX: re-validate cached SET member lists (bangcuser bans,
	// anti-* whitelists, premium/banned/sudo lists) in the background every
	// 2 min so changes made from another server/deployment are picked up —
	// exactly like the settings hash cache below. One SMEMBERS per cached
	// SET key, off the reply path.
	for _, ck := range setKeys {
		key := strings.TrimPrefix(ck, "setmembers:")
		r, err := u.cmd("SMEMBERS", key)
		if err != nil {
			continue // best-effort: keep existing cached value
		}
		var arr []string
		if err := json.Unmarshal(r, &arr); err != nil {
			continue
		}
		if len(arr) == 0 {
			u.cacheSet(ck, "\x00")
		} else if b, err := json.Marshal(arr); err == nil {
			u.cacheSet(ck, string(b))
		}
	}
	if len(keys) == 0 {
		return
	}

	for _, ck := range keys {
		rest := strings.TrimPrefix(ck, "settings:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			continue
		}
		jid := rest[:idx]
		field := rest[idx+1:]

		r, err := u.cmd("HGET", "settings:"+jid, field)
		if err != nil {
			continue // best-effort: keep existing cached value
		}
		if strings.TrimSpace(string(r)) == "null" || len(r) == 0 {
			u.cacheSet(ck, "\x00")
			continue
		}
		val := trimQuotes(string(r), "")
		u.cacheSet(ck, val)
	}
}

func (u *Upstash) StopCacheRefresher() {
	if u.refreshCancel != nil {
		u.refreshCancel()
	}
}

func (u *Upstash) ClearCache() {
	u.mu.Lock()
	var keys []string
	for k := range u.cache {
		if strings.HasPrefix(k, "settings:") {
			keys = append(keys, k)
		}
	}
	n := len(u.cache)
	u.cache = map[string]cacheEntry{}
	u.warmedGroups = nil
	u.mu.Unlock()

	if n > 0 {
		InfoLog("Storj config cache cleared (%d entries, %d settings keys) — re-fetching in background", n, len(keys))
	}

	u.refetchMu.Lock()
	u.refetching = true
	u.pendingUpdates = map[string]string{}
	u.refetchMu.Unlock()

	go u.refetchAfterClear(keys)
}

func (u *Upstash) refetchAfterClear(keys []string) {
	for _, ck := range keys {
		rest := strings.TrimPrefix(ck, "settings:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			continue
		}
		jid := rest[:idx]
		field := rest[idx+1:]

		// HEXISTS-first: preserve empty-string values that resets store.
		he, err := u.cmd("HEXISTS", "settings:"+jid, field)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(he)) == "0" {
			u.cacheSet(ck, "\x00")
			continue
		}
		r, err := u.cmd("HGET", "settings:"+jid, field)
		if err != nil {
			continue
		}
		u.cacheSet(ck, trimQuotes(string(r), ""))
	}

	u.refetchMu.Lock()
	pending := u.pendingUpdates
	u.pendingUpdates = map[string]string{}
	u.refetching = false
	u.refetchMu.Unlock()

	for ck, val := range pending {
		u.cacheSet(ck, val)
	}

	if len(pending) > 0 {
		InfoLog("Re-fetch complete — %d user config updates preserved", len(pending))
	} else {
		InfoLog("Re-fetch complete — cache repopulated (%d keys)", len(keys))
	}
}

func (u *Upstash) CacheLen() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.cache)
}

func (u *Upstash) WarmCache() {
	banned := u.BannedList()
	u.mu.Lock()
	for _, b := range banned {
		u.cache["banned:"+b] = cacheEntry{value: "1", ts: time.Now()}
	}
	u.mu.Unlock()
	OkLog("Config cache warmed: %d banned entries", len(banned))
}

func (u *Upstash) WarmGroupSettings(groupJID string) {
	if u.warmedGroups == nil {
		u.mu.Lock()
		if u.warmedGroups == nil {
			u.warmedGroups = map[string]bool{}
		}
		u.mu.Unlock()
	}
	u.mu.Lock()
	_, seen := u.warmedGroups[groupJID]
	u.mu.Unlock()
	if seen {
		return
	}
	u.mu.Lock()
	u.warmedGroups[groupJID] = true
	u.mu.Unlock()
	go func() {
		defer func() { _ = recover() }()
		r, err := u.cmd("HGETALL", "settings:"+groupJID)
		if err != nil {
			return
		}
		var pairs []string
		if err := json.Unmarshal(r, &pairs); err != nil || len(pairs)%2 != 0 {
			return
		}
		for i := 0; i < len(pairs); i += 2 {
			u.cacheSet("settings:"+groupJID+":"+pairs[i], pairs[i+1])
		}
	}()
}

// WarmGroupBanList warms the per-group bangcuser banned-users SET into the
// in-memory set-members cache (GROUP-SPEED FIX). One background SMEMBERS per
// group per process lifetime — after this, the synchronous per-message
// banned check (handler.go) hits memory at 0ms instead of paying an S3
// ListObjects round-trip on every single group message.
func (u *Upstash) WarmGroupBanList(groupJID, botJID string) {
	if u.warmedGroups == nil {
		u.mu.Lock()
		if u.warmedGroups == nil {
			u.warmedGroups = map[string]bool{}
		}
		u.mu.Unlock()
	}
	warmKey := "bangcuser:" + botJID + ":" + groupJID
	u.mu.Lock()
	_, seen := u.warmedGroups[warmKey]
	u.mu.Unlock()
	if seen {
		return
	}
	u.mu.Lock()
	u.warmedGroups[warmKey] = true
	u.mu.Unlock()
	go func() {
		defer func() { _ = recover() }()
		_ = u.setMembers("goldmd:" + botJID + ":groupset:" + groupJID + ":bangcuser")
	}()
}

func (u *Upstash) PreloadSettings(jid, defPrefix string) {
	// 1. Bot settings hash → cache (single HGETALL round-trip).
	r, err := u.cmd("HGETALL", "settings:"+jid)
	if err == nil {
		var pairs []string
		if errJ := json.Unmarshal(r, &pairs); errJ == nil && len(pairs)%2 == 0 {
			count := 0
			for i := 0; i < len(pairs); i += 2 {
				field := pairs[i]
				val := pairs[i+1]
				u.cacheSet("settings:"+jid+":"+field, val)
				count++
			}
			if count > 0 {
				OkLog("Preloaded %d settings for %s", count, jid)
			}
		}
	}

	// 2. Hot-path fields: default-sentinel any NOT present in the hash.
	for _, f := range []string{"mode", "sudowners", "botname", "ownername", "ownernumber", "botpic"} {
		ck := "settings:" + jid + ":" + f
		if _, ok := u.cacheGet(ck); !ok {
			u.cacheSet(ck, "\x00")
		}
	}

	// 3. Prefix (own key, separate from the settings hash).
	ckp := "prefix:" + jid
	if _, ok := u.cacheGet(ckp); !ok {
		u.cacheSet(ckp, u.safeString("prefix:"+jid, defPrefix))
	}
}

// Ping tests connectivity (Storj bucket reachable).
func (u *Upstash) Ping() bool {
	_, err := u.cmd("PING")
	if err != nil {
		ErrLog("Storj storage ping failed: %v", err)
		return false
	}
	InfoLog("Storj storage health check passed")
	return true
}

// ── util ─────────────────────────────────────────────────────────────────────

// Values marshalled through JSON arrive quoted; trim them (same semantics
// the old Upstash REST layer had).
func trimQuotes(s, fb string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return fb
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ============================================================================
// ── SESSION PERSISTENCE (Storj-backed) ──────────────────────────────────────
//
// whatsmeow keeps every session's auth material (device identity, signal
// session keys, prekeys, etc.) inside ONE shared sqlite file ("goldmd.db"
// in cfg.DataDir). On ephemeral hosts that file disappears on every
// restart/redeploy, which is why sessions had to be re-paired.
//
// Fix: back the whole file up to Storj (base64 string) whenever a session
// connects/pairs, and restore it BEFORE the sqlite container is opened on
// the next boot. A JID set is kept so AutoLoad() can recreate the pairing
// marker folders it depends on.
// ============================================================================

// SaveSessionDB checkpoints SQLite before uploading the auth database.
//
// BANDWIDTH JUGAD (transparent, behaviour SAME):
//  1. HASH-SKIP — agar DB file last successful upload se UNCHANGED hai
//     (sha256 match) to upload hi skip — 0 bytes egress. Idle servers
//     (koi message/key-rotation nahi) ab kuch bhi nahi bhejenge. Sirf
//     successful upload ke baad hash yaad rakha jata hai — Storj fail
//     ho to next tick dobara try hoga (koi data-loss window nahi).
//  2. GZIP — base64 blob ab kvSet me transparent gzip hota hai (GZ1:),
//     ~8-15x chhota PUT body. Value/read path bilkul same.
func (u *Upstash) SaveSessionDB(path string) error {
	// DISK-ONLY GUARD (owner order): agar is server ke disk pe koi bhi
	// direct /code?phone= (local-only) session pada hai, to ye DB upload
	// KABHI nahi hoga — goldmd.db me us session ke creds hote hain, aur
	// owner ka order hai ke direct session Storj pe na jaye. Central gate
	// yahan hi hai taake main.go ticker / graceful shutdown / reconnect
	// path — koi bhi future caller leak na kar sake.
	if localOnlyUploadBlocked() {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := checkpointSQLite(path); err != nil {
		return fmt.Errorf("checkpoint session database: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// hash-skip: unchanged DB → zero upload (idle ke 144 uploads/day khatam)
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	u.dbHashMu.Lock()
	skip := u.dbHash == hexSum && hexSum != ""
	u.dbHashMu.Unlock()
	if skip {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	if err := u.setString(u.sessionDBKey(), encoded); err != nil {
		return fmt.Errorf("save session blob: %w", err)
	}
	u.dbHashMu.Lock()
	u.dbHash = hexSum
	u.dbHashMu.Unlock()
	return nil
}

func checkpointSQLite(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+absPath+"?mode=rwc&_busy_timeout=10000&_txlock=immediate")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	return err
}

// RestoreSessionDB writes the Storj-stored sqlite file back to disk.
// Returns (true, nil) if a backup was found and restored.
func (u *Upstash) RestoreSessionDB(path string) (bool, error) {
	r, err := u.cmd("GET", u.sessionDBKey())
	if err != nil {
		return false, err
	}
	encoded := trimQuotes(string(r), "")
	if encoded == "" {
		return false, nil // nothing backed up yet
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// healGZ1Lazy — lazy GZ1->PLAIN rewrite (kvGet se background me).
// Race-safe: rewrite se pehle re-read — object ab bhi GZ1 hai to hi
// overwrite (beech me kisi ne plain/naya likha to SKIP — newer data
// clobbering se bachav). Sirf default wire (gzip OFF) pe chalta hai.
func (u *Upstash) healGZ1Lazy(key, objKey string, plain []byte) {
	defer func() { _ = recover() }() // heal kabhi panic na kare
	if kvGzipEnabled {
		return // fleet GZ1 mode me hai — plain rewrite NAHI karo
	}
	shard := u.kvShard(key)
	if shard == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cur, found, err := u.kvRead(ctx, shard, objKey)
	if err != nil || !found {
		return
	}
	if !bytes.HasPrefix(cur, []byte(kvGzPrefix)) {
		return // already healed/updated — overwrite mat karo
	}
	if err := u.setString(key, string(plain)); err != nil {
		WarnLog("GZ1-HEAL: lazy rewrite failed for %s: %v", key, err)
		return
	}
	InfoLog("GZ1-HEAL: lazy heal %s -> plain (%d bytes) — purane binaries ab padh sakte hain",
		key, len(plain))
}

// ── EMERGENCY GZ1 HEAL (mixed-fleet recovery) ──────────────────────────────
// Broken build (c704231) ne badi string values "GZ1:"+gzip me likhi thin.
// Naya binary unhe padh leta hai, purane binaries NAHI (base64 decode fail
// -> failover/restore fail -> sessions offline gaye the). HealGZ1Key raw
// body padhta hai; agar GZ1 tha to ORIGINAL value decompress karke PLAIN
// rewrite karta hai taake poori fleet (purane binaries included) wapas
// padh sake. Boot pe one-time chalta hai (fleetInit se).
func (u *Upstash) HealGZ1Key(key string) bool {
	shard := u.kvShard(key)
	if shard == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, found, err := u.kvRead(ctx, shard, kvStringsPrefix+kvEncode(key))
	if err != nil || !found {
		return false
	}
	if !bytes.HasPrefix(raw, []byte(kvGzPrefix)) {
		return false // already plain — kuch nahi karna
	}
	plain := kvGunzipMaybe(raw)
	if len(plain) == 0 || bytes.HasPrefix(plain, []byte(kvGzPrefix)) {
		return false // corrupt/fallback — rewrite mat karo
	}
	// PLAIN rewrite (kvGzipEnabled false hai to kvSet body as-is jayegi)
	if err := u.setString(key, string(plain)); err != nil {
		WarnLog("GZ1-HEAL: rewrite failed for %s: %v", key, err)
		return false
	}
	InfoLog("GZ1-HEAL: rewrote %s in legacy plain format (%d -> %d bytes)",
		key, len(raw), len(plain))
	return true
}

// HealGZ1Keys multiple keys ka heal — kitne heal hue return.
func (u *Upstash) HealGZ1Keys(keys []string) int {
	n := 0
	for _, k := range keys {
		if u.HealGZ1Key(k) {
			n++
		}
	}
	return n
}

// RestoreLegacySessionDB: purane (pre-fleet) shared "svr1" whole-DB backup
// ko disk pe wapas likhta hai — ONE-TIME migration (main.go boot flow).
// Uske baad DeleteLegacySvr1Backup use hata deta hai taake koi doosra
// upgraded server dobara restore na kare (shared-key conflict khatam).
func (u *Upstash) RestoreLegacySessionDB(path string) (bool, error) {
	r, err := u.cmd("GET", sessionDBKeyConst+"svr1"+sessionDBKeySuffix)
	if err != nil {
		return false, err
	}
	encoded := trimQuotes(string(r), "")
	if encoded == "" {
		return false, nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o644)
}

// DeleteLegacySvr1Backup one-time migration ke baad purana shared svr1
// blob + uska JID registry delete kar deta hai — is se kabhi koi doosra
// server us shared key se restore karke conflict nahi kar sakta.
func (u *Upstash) DeleteLegacySvr1Backup() {
	_, _ = u.cmd("DEL", sessionDBKeyConst+"svr1"+sessionDBKeySuffix)
	_, _ = u.cmd("DEL", sessionJidsKeyConst+"svr1"+sessionJidsKeySuffix)
}

// RegisterJID remembers a paired JID so its pairing folder can be recreated
// after a disk wipe (AutoLoad scans folders under cfg.PairingDir).
func (u *Upstash) RegisterJID(jid string) error {
	return u.setAdd(u.sessionJidsKey(), jid)
}

// ServerID returns this deployment's unique namespace ID (per-URL pairing
// isolation ke liye guards use karte hain).
func (u *Upstash) ServerID() string { return u.serverID }

// ListJIDs returns every JID ever registered.
func (u *Upstash) ListJIDs() []string { return u.setMembers(u.sessionJidsKey()) }

// HasJID reports whether the registry has a session for the base JID.
func (u *Upstash) HasJID(jid string) bool {
	r, err := u.cmd("SISMEMBER", u.sessionJidsKey(), jid)
	return err == nil && trimQuotes(string(r), "0") == "1"
}

// RemoveJID drops a JID from the session registry.
func (u *Upstash) RemoveJID(jid string) error {
	return u.setRem(u.sessionJidsKey(), jid)
}

// DelSessionDB removes the session DB blob. Used by cleanupSession when the
// LAST paired device is removed so the next boot starts truly fresh.
func (u *Upstash) DelSessionDB() error {
	return u.setDel(u.sessionDBKey())
}

// ══════════════════ (merged from fleet_git.go) ══════════════════
// ═════════════════════════════════════════════════════════════════════════════
//   GOLD-MD — .svrchange GIT CLIENT (owner-only)
//
//   OWNER ORDER:
//   ".svrchange cmnd banao — GitHub + GitLab DONO repo me tokens se
//    servers.json dhunde, servers ke links change karke push kar de.
//    Render auto-deploy ON hai — push hote hi foran fresh changes
//    sab bots me chale jayenge (owner ko bar-bar git pe jana nahi prega)."
//
//   USAGE:
//     .svrchange 9 https://new-link.onrender.com
//     .svrchange 9/19/50 link1,link2,link3     ← multiple ek saath
//     (links comma YA space se separated — dono chalte hain)
//
//   FORMAT ERRORS (owner ka exact order):
//     ".svrchange 9 link1,link2"       → *FORMAT ERROR*
//       YOU HAVE SELECTED SERVER 9 ONLY AND GIVEN 2 LINKS
//     ".svrchange 9/60/100 link1,link2" → *FORMAT ERROR*
//       YOU HAVE SELECTED 3 SERVERS AND GIVEN 2 LINKS (1 missing)
//
//   TARGETS: GitHub Uf-prince/TEST-BOT (GitLab TEMP OFF — owner order:
//   .svrchange ab SIRF GitHub pe changes karta hai; GitLab untouched. (pehle dono me
//   servers.json root me, same structure).  Line-based URL replacement —
//   original file formatting EXACT preserve hota hai (clean git diff me
//   sirf changed URLs dikhte hain).  Push hote hi Render auto-deploy
//   foran trigger hota hai.
// ═════════════════════════════════════════════════════════════════════════════

// ── git targets + tokens (owner ke order pe hardcoded) ──
const (
	svrGitHubToken = "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" // fresh token (purana 401 tha)
	// ── GITLAB TEMP COMMENT (owner order: .svrchange ab SIRF GitHub pe changes kare;
	//    GitLab bilkul untouched — token/repo neeche commented hai) ──
	// svrGitLabToken = "glpat-L557rQxDcWXI0Yu-hUEQg2M6MQpvOjEKdTpuN2I0aQ8.01.170nzpruw"

	svrGitHubRepo = "Uf-prince/TEST-BOT" // api.github.com/repos/{repo}
	// svrGitLabRepo = "Uf-prince%2FGOLD-MD" // URL-encoded project path

	svrServersFile = "servers.json"
	svrBranch      = "main"
)

var svrHTTPClient = &http.Client{Timeout: 25 * time.Second}

// ── JSON line patterns (name/url entry match — compact + multi-line dono) ──
var (
	svrNameRe = regexp.MustCompile(`"name"\s*:\s*"([^"]*)"`)
	svrURLRe  = regexp.MustCompile(`"url"\s*:\s*"([^"]*)"`)
)

// svrServerNumber: "SERVER 9" → 9 (nahi mila → 0).
func svrServerNumber(name string) int {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) < 2 {
		return 0
	}
	num, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return 0
	}
	return num
}

// ═════════════════════════════════════════════════════════════════════════════
//   GITHUB CONTENTS API  (GET sha → PUT updated content)
// ═════════════════════════════════════════════════════════════════════════════

// svrGitHubGet: repo se servers.json raw content + blob sha (update ke liye).
func svrGitHubGet() (content string, sha string, err error) {
	url := "https://api.github.com/repos/" + svrGitHubRepo +
		"/contents/" + svrServersFile + "?ref=" + svrBranch
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+svrGitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}
	// GitHub base64 me har 60 chars pe newline — strip whitespace first.
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, payload.Content)
	raw, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", "", err
	}
	return string(raw), payload.SHA, nil
}

// svrGitHubPut: updated servers.json push (sha-based — no race/409).
func svrGitHubPut(content, sha, message string) (commit string, err error) {
	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"sha":     sha,
		"branch":  svrBranch,
	}
	body, _ := json.Marshal(payload)
	url := "https://api.github.com/repos/" + svrGitHubRepo + "/contents/" + svrServersFile
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "token "+svrGitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	_ = json.Unmarshal(respBody, &out)
	return out.Commit.SHA, nil
}

/* ── TEMP COMMENT (owner order: .svrchange ab SIRF GitHub pe changes kare — GitLab
   bilkul untouched rakhna hai. Wapas lana ho: is block ka start/end comment
   hatao + upar const block me svrGitLabToken/svrGitLabRepo uncomment karo
   + svrParseAndRun ka GitLab section uncomment karo) ──
// ═════════════════════════════════════════════════════════════════════════════
//   GITLAB REPOSITORY FILES API  (raw GET → PUT branch=main)
// ═════════════════════════════════════════════════════════════════════════════

// svrGitLabGet: project se servers.json raw content.
func svrGitLabGet() (string, error) {
	url := "https://gitlab.com/api/v4/projects/" + svrGitLabRepo +
		"/repository/files/" + svrServersFile + "/raw?ref=" + svrBranch
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", svrGitLabToken)
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(body), nil
}

// svrGitLabPut: updated servers.json push (files API PUT = update).
// NOTE: GitLab files API URL me ?ref=<branch> LAZMI chahiye (warna 400
// "ref is missing") — live E2E test me pakda gaya tha.
func svrGitLabPut(content, message string) (commit string, err error) {
	payload := map[string]string{
		"branch":         svrBranch,
		"content":        content,
		"commit_message": message,
	}
	body, _ := json.Marshal(payload)
	url := "https://gitlab.com/api/v4/projects/" + svrGitLabRepo +
		"/repository/files/" + svrServersFile + "?ref=" + svrBranch
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	req.Header.Set("PRIVATE-TOKEN", svrGitLabToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}
	var out struct {
		CommitID string `json:"commit_id"`
	}
	_ = json.Unmarshal(respBody, &out)
	return out.CommitID, nil
}

── /TEMP COMMENT (GitLab funcs) ── */
// ═════════════════════════════════════════════════════════════════════════════
//   SERVERS.JSON LINE-BASED UPDATE ENGINE
//   (original formatting EXACT preserve — sirf URL value splice hota hai,
//    is liye git diff me sirf changed lines dikhti hain)
// ═════════════════════════════════════════════════════════════════════════════

// svrApplyChanges: raw servers.json → URL updates apply → new raw.
// NOTE: input map MUTATE NAHI hota (dono repos ke liye same map reuse hota hai).
func svrApplyChanges(raw string, changes map[int]string) (newRaw string, updated []string, missing []int, err error) {
	pending := make(map[int]string, len(changes))
	for k, v := range changes {
		pending[k] = v
	}

	lines := strings.Split(raw, "\n")
	pendingName := 0 // jis server entry ke andar currently hain
	for i, line := range lines {
		if m := svrNameRe.FindStringSubmatch(line); m != nil {
			pendingName = svrServerNumber(m[1])
			// compact entry: name+url ek hi line pe ho sakte hain
			if m2 := svrURLRe.FindStringSubmatchIndex(line); m2 != nil {
				if newURL, ok := pending[pendingName]; ok {
					lines[i] = line[:m2[2]] + newURL + line[m2[3]:]
					updated = append(updated, fmt.Sprintf("SERVER %d → %s", pendingName, newURL))
					delete(pending, pendingName)
				}
				pendingName = 0
			}
			continue
		}
		if m := svrURLRe.FindStringSubmatchIndex(line); m != nil {
			if pendingName > 0 {
				if newURL, ok := pending[pendingName]; ok {
					lines[i] = line[:m[2]] + newURL + line[m[3]:]
					updated = append(updated, fmt.Sprintf("SERVER %d → %s", pendingName, newURL))
					delete(pending, pendingName)
				}
			}
			pendingName = 0
		}
	}

	if len(pending) > 0 {
		for num := range pending {
			missing = append(missing, num)
		}
		sort.Ints(missing)
	}
	return strings.Join(lines, "\n"), updated, missing, nil
}

// ═════════════════════════════════════════════════════════════════════════════
//   .svrchange — PARSE + VALIDATE + RUN (dono repos, aggregated reply)
// ═════════════════════════════════════════════════════════════════════════════

// svrParseAndRun: .svrchange ka pura flow.  fleet_commands.go se owner-guard
// ke BAAD call hota hai (non-owner ke liye command silently ignore hota hai).
func svrParseAndRun(args []string) string {
	const usage = "*FORMAT ERROR*\nUsage:\n.svrchange 9 https://new-link.onrender.com\n.svrchange 9/19/50 link1,link2,link3"

	if len(args) < 2 {
		return usage
	}
	selector := strings.TrimSpace(args[0])
	linksRaw := strings.TrimSpace(strings.Join(args[1:], " "))

	// ── parse selector: "9/19/50" → [9,19,50] ──
	var nums []int
	for _, part := range strings.Split(selector, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid server number: %q\n(sirf number — jaise 9 ya 9/19/50)", part)
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return usage
	}

	// ── parse links: comma YA space se separated (dono chalte hain) ──
	var links []string
	for _, tok := range strings.FieldsFunc(linksRaw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		links = append(links, tok)
	}

	// ── link validation: https:// prefix + safe chars + trailing slash strip ──
	for i, l := range links {
		if !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid link: %s\n(link http:// ya https:// se start hona chahiye)", l)
		}
		if strings.ContainsAny(l, `"\`) {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid characters in link: %s", l)
		}
		links[i] = strings.TrimRight(l, "/") // config convention: no trailing slash
	}

	// ── COUNT VALIDATION (owner ka exact error format) ──
	if len(nums) != len(links) {
		if len(nums) == 1 {
			return fmt.Sprintf("*FORMAT ERROR*\nYOU HAVE SELECTED SERVER %d ONLY AND GIVEN %d LINKS\n(1 server ke liye sirf 1 link do)", nums[0], len(links))
		}
		return fmt.Sprintf("*FORMAT ERROR*\nYOU HAVE SELECTED %d SERVERS AND GIVEN %d LINKS\n(%d link missing — sab servers ke liye links do)",
			len(nums), len(links), len(nums)-len(links))
	}
	if len(links) == 0 {
		return usage
	}

	// ── build change map (duplicate server numbers reject) ──
	changes := make(map[int]string, len(nums))
	for i, n := range nums {
		if _, dup := changes[n]; dup {
			return fmt.Sprintf("*FORMAT ERROR*\nSERVER %d do baar select kiya hai — ek baar hi select karo", n)
		}
		changes[n] = links[i]
	}

	// ── GITHUB push ──
	var b strings.Builder
	b.WriteString("*🔰 .svrchange — SERVER LINKS UPDATE 🔰*\n\n")
	ghOK := false // GitLab TEMP OFF (owner order) — sirf GitHub pe changes

	if raw, sha, err := svrGitHubGet(); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nGET error: %v\n\n", err))
	} else if newRaw, updated, missing, err := svrApplyChanges(raw, changes); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nParse error: %v\n\n", err))
	} else if len(missing) > 0 {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nSERVER %v servers.json me nahi mila (is repo me)\n\n", missing))
	} else if commit, err := svrGitHubPut(newRaw, sha, "svrchange: server link update (bot command)"); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nPUSH error: %v\n\n", err))
	} else {
		ghOK = true
		b.WriteString("*✅ GITHUB (TEST-BOT) — UPDATED*\n")
		if commit != "" {
			b.WriteString(fmt.Sprintf("Commit: %s\n", shortSHA(commit)))
		}
		for _, u := range updated {
			b.WriteString("• " + u + "\n")
		}
		b.WriteString("\n")
	}

	// ── GITLAB push — TEMP OFF (owner order: .svrchange SIRF GitHub pe changes kare.
	//    GitLab bilkul untouched. Wapas lana ho: ye section uncomment karo + upar
	//    svrGitLabToken/svrGitLabRepo consts + svrGitLabGet/svrGitLabPut funcs) ──
	/*
		// ── GITLAB push ──
		if raw, err := svrGitLabGet(); err != nil {
			b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nGET error: %v", err))
		} else if newRaw, updated, missing, err := svrApplyChanges(raw, changes); err != nil {
			b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nParse error: %v", err))
		} else if len(missing) > 0 {
			b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nSERVER %v servers.json me nahi mila (is repo me)", missing))
		} else if commit, err := svrGitLabPut(newRaw, "svrchange: server link update (bot command)"); err != nil {
			b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nPUSH error: %v", err))
		} else {
			glOK = true
			b.WriteString("*✅ GITLAB (GOLD-MD) — UPDATED*\n")
			if commit != "" {
				b.WriteString(fmt.Sprintf("Commit: %s\n", shortSHA(commit)))
			}
			for _, u := range updated {
				b.WriteString("• " + u + "\n")
			}
		}

	*/

	// ── summary ──
	if ghOK {
		b.WriteString("\n*⚡ RENDER AUTO-DEPLOY:* push hote hi foran trigger ho jayega — fresh changes sab bots me chale jayenge.\n")
	} else {
		b.WriteString("\n*❌ Koi repo update nahi hua — upar errors dekho.*\n")
	}
	return b.String()
}

// shortSHA: commit sha → first 7 chars (git style).
func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// truncateStr: error messages lambi na ho — cap lagata hai.
func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
