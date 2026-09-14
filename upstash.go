package main

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

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
)

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
