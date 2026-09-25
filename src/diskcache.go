package main

// ═══════════════════════════════════════════════════════════════════════════
//   DISK-BACKED KV CACHE  (bandwidth bachao — owner order)
//
//   MASLA: Bot ka "Redis" asal me Storj/S3 hai. Har HGETALL / SMEMBERS =
//   poori bucket listing (ListObjects) = metered egress. Fleet watchdog har
//   60s me per-JID HGETALL maarta hai → 0 sessions pe bhi 5MB/5min kharch.
//
//   HAL: Har KV op ab DISK se serve hota hai. Storj sirf tab touch hota hai:
//     1. Boot pe disk khali ho  → ek baar bulk-load (GUARD)
//     2. Disk pe key na mile    → read-through miss (ek baar load)
//     3. Write ho               → write-through (disk + Storj ek baar)
//     4. Slow background refresh (cross-server freshness, default 5 min)
//
//   Is se Storj reads ~90% kam → Render 5GB bachta hai. Behaviour 100% same:
//   same values, same JSON shapes, same semantics — sirf source disk hai.
// ═══════════════════════════════════════════════════════════════════════════

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ── config (env-overridable) ────────────────────────────────────────────────
var diskCacheEnabled = envBool("GOLDMD_DISK_CACHE", true)
var diskCacheRoot = envOr("GOLDMD_DISK_CACHE_DIR", "nexstore/kvcache")
var diskCacheRefreshMin = envInt("GOLDMD_DISK_CACHE_REFRESH_MIN", 30)

// keyState — ek logical Redis key ka poora state (mini-Redis on disk).
//
//	Type "string" → Str
//	Type "set"    → Set (members)
//	Type "hash"   → Hash (field→value)
type keyState struct {
	// Key = the logical Redis key this file represents. Stored INSIDE the
	// file (the filename is only a sha256 hash, so the key is otherwise
	// unrecoverable). Used by dcPreloadRAM to rebuild the RAM cache from
	// disk without any network round-trip.
	Key  string            `json:"k,omitempty"`
	Type string            `json:"t"`
	Str  string            `json:"s,omitempty"`
	Set  []string          `json:"m,omitempty"`
	Hash map[string]string `json:"h,omitempty"`
	TS   int64             `json:"ts"`
}

var (
	dcReady bool
	dcDir   string
	dcMu    sync.Mutex // serialises file writes (atomic rename)
)

// diskCacheInit — boot pe ek baar. Dir banao, ready flag set karo.
func diskCacheInit() {
	if !diskCacheEnabled {
		InfoLog("DISK-CACHE: disabled (GOLDMD_DISK_CACHE=0)")
		return
	}
	dcDir = diskCacheRoot
	if err := os.MkdirAll(dcDir, 0o755); err != nil {
		ErrLog("DISK-CACHE: mkdir %s failed: %v — disabled", dcDir, err)
		diskCacheEnabled = false
		return
	}
	dcReady = true
	InfoLog("DISK-CACHE: ready (dir=%s, refresh=%dmin)", dcDir, diskCacheRefreshMin)
}

// dcPath — logical key → disk file (sha256 hex, collision-free, filename-safe).
func dcPath(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(dcDir, hex.EncodeToString(h[:])+".json")
}

// dcLoad — disk se key state padho. (nil,false) = miss.
func dcLoad(key string) (*keyState, bool) {
	if !dcReady {
		return nil, false
	}
	data, err := os.ReadFile(dcPath(key))
	if err != nil {
		return nil, false
	}
	var st keyState
	if json.Unmarshal(data, &st) != nil {
		return nil, false
	}
	return &st, true
}

// dcSave — atomic write (tmp + rename) so a crash never leaves a torn file.
func dcSave(key string, st *keyState) {
	if !dcReady {
		return
	}
	st.Key = key
	st.TS = time.Now().Unix()
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	dcMu.Lock()
	defer dcMu.Unlock()
	p := dcPath(key)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, p)
}

func dcDelete(key string) {
	if !dcReady {
		return
	}
	dcMu.Lock()
	defer dcMu.Unlock()
	_ = os.Remove(dcPath(key))
}

// dcIsReadOp — ops jo disk se serve ho sakte hain (0 Storj bandwidth).
func dcIsReadOp(op string) bool {
	switch op {
	case "GET", "SMEMBERS", "SISMEMBER", "HGETALL", "HGET", "HEXISTS":
		return true
	}
	return false
}

// dcIsWriteOp — ops jo disk state mutate karte hain.
func dcIsWriteOp(op string) bool {
	switch op {
	case "SET", "DEL", "SADD", "SREM", "HSET", "HDEL":
		return true
	}
	return false
}

// dcSkipKey — special keys jo disk-cache se bahar rakhe jate hain.
//   - sessiondb blob/jids — bade (MBs) + special persistence path (write-through
//     se disk bloat + restore semantics kharab ho sakte hain).
func dcSkipKey(args []string) bool {
	if len(args) < 2 {
		return false
	}
	k := args[1]
	return strings.Contains(k, "sessiondb")
}

// ── read path ───────────────────────────────────────────────────────────────
// dcRead — read op ko disk se serve karo. (res,true) = hit (0 bandwidth).
func dcRead(args []string) (json.RawMessage, bool) {
	if !dcReady || len(args) < 2 {
		return nil, false
	}
	op := strings.ToUpper(args[0])
	key := args[1]
	st, ok := dcLoad(key)
	if !ok {
		return nil, false
	}
	switch op {
	case "GET":
		if st.Type == "missing" {
			return json.RawMessage(`null`), true
		}
		if st.Type != "string" {
			return nil, false
		}
		b, _ := json.Marshal(st.Str)
		return b, true
	case "SMEMBERS":
		if st.Type != "set" {
			return nil, false
		}
		arr := append([]string(nil), st.Set...)
		sort.Strings(arr)
		b, _ := json.Marshal(arr)
		return b, true
	case "SISMEMBER":
		if st.Type != "set" || len(args) < 3 {
			return nil, false
		}
		for _, m := range st.Set {
			if m == args[2] {
				return json.RawMessage(`1`), true
			}
		}
		return json.RawMessage(`0`), true
	case "HGETALL":
		if st.Type != "hash" {
			return nil, false
		}
		fields := make([]string, 0, len(st.Hash))
		for k := range st.Hash {
			fields = append(fields, k)
		}
		sort.Strings(fields)
		pairs := make([]string, 0, len(fields)*2)
		for _, k := range fields {
			pairs = append(pairs, k, st.Hash[k])
		}
		b, _ := json.Marshal(pairs)
		return b, true
	case "HGET":
		if st.Type != "hash" || len(args) < 3 {
			return nil, false
		}
		v, ok := st.Hash[args[2]]
		if !ok {
			return json.RawMessage(`null`), true
		}
		b, _ := json.Marshal(v)
		return b, true
	case "HEXISTS":
		if st.Type != "hash" || len(args) < 3 {
			return nil, false
		}
		if _, ok := st.Hash[args[2]]; ok {
			return json.RawMessage(`1`), true
		}
		return json.RawMessage(`0`), true
	}
	return nil, false
}

// ── write path ──────────────────────────────────────────────────────────────
// dcApply — op ke result se disk state update karo (read populate + write mutate).
func dcApply(args []string, res json.RawMessage) {
	if !dcReady || len(args) < 2 {
		return
	}
	op := strings.ToUpper(args[0])
	key := args[1]
	switch op {
	case "GET":
		s := trimQuotes(string(res), "")
		if string(res) == "null" || s == "" {
			// MISSING sentinel — dobara Storj fetch na ho (bandwidth leak fix).
			dcSave(key, &keyState{Type: "missing"})
		} else {
			dcSave(key, &keyState{Type: "string", Str: s})
		}
	case "SET":
		if len(args) >= 3 {
			dcSave(key, &keyState{Type: "string", Str: args[2]})
		}
	case "DEL":
		dcDelete(key)
	case "SMEMBERS":
		var arr []string
		if json.Unmarshal(res, &arr) == nil {
			dcSave(key, &keyState{Type: "set", Set: arr})
		}
	case "SADD":
		st, _ := dcLoad(key)
		if st == nil {
			st = &keyState{Type: "set"}
		}
		st.Type = "set"
		st.Set = dcSetAdd(st.Set, args[2:])
		dcSave(key, st)
	case "SREM":
		st, _ := dcLoad(key)
		if st == nil {
			st = &keyState{Type: "set"}
		}
		st.Type = "set"
		st.Set = dcSetRem(st.Set, args[2:])
		dcSave(key, st)
	case "HGETALL":
		var pairs []string
		if json.Unmarshal(res, &pairs) == nil {
			h := map[string]string{}
			for i := 0; i+1 < len(pairs); i += 2 {
				h[pairs[i]] = pairs[i+1]
			}
			dcSave(key, &keyState{Type: "hash", Hash: h})
		}
	case "HGET":
		if len(args) < 3 {
			return
		}
		st, _ := dcLoad(key)
		if st == nil {
			st = &keyState{Type: "hash", Hash: map[string]string{}}
		}
		if st.Hash == nil {
			st.Hash = map[string]string{}
		}
		st.Type = "hash"
		s := trimQuotes(string(res), "")
		if string(res) == "null" || s == "" {
			delete(st.Hash, args[2])
		} else {
			st.Hash[args[2]] = s
		}
		dcSave(key, st)
	case "HSET":
		if len(args) < 4 {
			return
		}
		st, _ := dcLoad(key)
		if st == nil {
			st = &keyState{Type: "hash", Hash: map[string]string{}}
		}
		if st.Hash == nil {
			st.Hash = map[string]string{}
		}
		st.Type = "hash"
		st.Hash[args[2]] = args[3]
		dcSave(key, st)
	case "HDEL":
		if len(args) < 3 {
			return
		}
		st, _ := dcLoad(key)
		if st != nil && st.Hash != nil {
			delete(st.Hash, args[2])
			dcSave(key, st)
		}
	}
}

func dcSetAdd(cur []string, add []string) []string {
	seen := map[string]bool{}
	for _, m := range cur {
		seen[m] = true
	}
	for _, m := range add {
		if !seen[m] {
			cur = append(cur, m)
			seen[m] = true
		}
	}
	return cur
}

func dcSetRem(cur []string, rem []string) []string {
	drop := map[string]bool{}
	for _, m := range rem {
		drop[m] = true
	}
	out := cur[:0]
	for _, m := range cur {
		if !drop[m] {
			out = append(out, m)
		}
	}
	return out
}

// ── GUARD: disk khali ho to ek baar Storj se bulk-load ──────────────────────
// Marker file "_loaded" — agar mojood hai to bulk-load skip (dobara nahi).
func dcGuardLoad() {
	if !dcReady {
		return
	}
	marker := filepath.Join(dcDir, "_loaded")
	if _, err := os.Stat(marker); err == nil {
		InfoLog("DISK-CACHE: guard — disk already loaded, bulk-load skip")
		return
	}
	InfoLog("DISK-CACHE: guard — disk EMPTY, Storj se bulk-load (one time)...")
	n := dcBulkLoad()
	_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o644)
	InfoLog("DISK-CACHE: guard — bulk-load done (%d keys)", n)
}

// dcGuardLoop — CONTINUOUS GUARD (owner order: "guard hamesha disk check karta
// rahe, jese hi data khali ho foran Storj se load kar le"). Har 60s disk cache
// ka file-count dekhta hai. Agar cache khaali (ya near-empty) ho jaye — jaise
// disk wipe, manual delete, ya kisi wajah se files gayab — to foran ek baar
// Storj se bulk-load kar deta hai. Ye check SIRF local file-count padhta hai
// (os.ReadDir) — 0 network, 0 bandwidth. Bulk-load sirf tab jab waqai khali ho.
func dcGuardLoop() {
	if !dcReady {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			if !dcReady {
				return
			}
			// local file-count — 0 bandwidth.
			entries, err := os.ReadDir(dcDir)
			if err != nil {
				continue
			}
			live := 0
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					live++
				}
			}
			// Threshold: agar 5 se kam keys bachi hain to cache "khali" maano
			// (normal me 200+ keys hoti hain). Sirf tab Storj se reload.
			if live >= 5 {
				continue
			}
			InfoLog("DISK-CACHE: guard — disk khali (%d keys), Storj se reload...", live)
			n := dcBulkLoad()
			InfoLog("DISK-CACHE: guard — reload done (%d keys)", n)
		}
	}()
}

// dcBulkLoad — saare shards ke strings/sets/hashes ek baar padho aur disk pe
// likho. Sirf SMALL keys (<= 256KB) — bade session blobs skip (wo write-through
// se aate hain). Best-effort: koi error ho to read-through baad me sambhalega.
func dcBulkLoad() int {
	if !storj.Ready() {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	storj.mu.Lock()
	shards := append([]*storjShard(nil), storj.shards...)
	storj.mu.Unlock()

	count := 0
	for _, sh := range shards {
		count += dcBulkStrings(ctx, sh)
		count += dcBulkSets(ctx, sh)
		count += dcBulkHashes(ctx, sh)
	}
	return count
}

const dcBulkMaxBytes = 256 * 1024 // 256KB — bade blobs skip

func dcBulkStrings(ctx context.Context, sh *storjShard) int {
	n := 0
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(kvStringsPrefix))
	for obj := range objCh {
		if obj.Err != nil || obj.Size > dcBulkMaxBytes {
			continue
		}
		key := kvDecode(strings.TrimPrefix(obj.Key, kvStringsPrefix))
		if key == "" {
			continue
		}
		data, found, err := kvReadShard(ctx, sh, obj.Key)
		if err != nil || !found {
			continue
		}
		if strings.HasPrefix(string(data), kvGzPrefix) {
			data = kvGunzipMaybe(data)
		}
		dcSave(key, &keyState{Type: "string", Str: string(data)})
		n++
	}
	return n
}

func dcBulkSets(ctx context.Context, sh *storjShard) int {
	// group objects by set-key (first path component after prefix)
	groups := map[string][]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(kvSetsPrefix))
	for obj := range objCh {
		if obj.Err != nil {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, kvSetsPrefix)
		idx := strings.Index(rest, "/")
		if idx < 0 {
			continue
		}
		setKey := kvDecode(rest[:idx])
		member := kvDecode(rest[idx+1:])
		if setKey == "" || member == "" {
			continue
		}
		groups[setKey] = append(groups[setKey], member)
	}
	for k, members := range groups {
		sort.Strings(members)
		dcSave(k, &keyState{Type: "set", Set: members})
	}
	return len(groups)
}

func dcBulkHashes(ctx context.Context, sh *storjShard) int {
	groups := map[string]map[string]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(kvHashesPrefix))
	for obj := range objCh {
		if obj.Err != nil || obj.Size > dcBulkMaxBytes {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, kvHashesPrefix)
		idx := strings.Index(rest, "/")
		if idx < 0 {
			continue
		}
		hashKey := kvDecode(rest[:idx])
		field := kvDecode(rest[idx+1:])
		if hashKey == "" || field == "" {
			continue
		}
		data, found, err := kvReadShard(ctx, sh, obj.Key)
		if err != nil || !found {
			continue
		}
		if groups[hashKey] == nil {
			groups[hashKey] = map[string]string{}
		}
		groups[hashKey][field] = string(data)
	}
	for k, h := range groups {
		dcSave(k, &keyState{Type: "hash", Hash: h})
	}
	return len(groups)
}

// ── slow background refresh (cross-server freshness) ────────────────────────
// Har refreshMin me cached keys ko Storj se re-read karke disk update karta
// hai — taake doosre server ki changes bhi dikhein. Bandwidth bounded: ek
// full re-read per refreshMin (default 5 min) vs. pehle continuous per-60s.
func dcStartRefresher() {
	if !dcReady || diskCacheRefreshMin <= 0 {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(time.Duration(diskCacheRefreshMin) * time.Minute)
		defer t.Stop()
		for range t.C {
			dcRefreshPass()
		}
	}()
}

// dcRefreshPass — SIRF "settings:" hashes ko Storj se re-read karta hai
// (cross-server setting changes ke liye). Poora re-read NAHI — warna har
// refresh pe saara data dobara aata (bandwidth waste). Fleet keys alag
// 60s refresher sambhalta hai; baaki sab write-through hai.
//
// OWNER ORDER (2026-09-18): interval 5min → 30min. Wajah: ab SAB kuch disk
// pe hai + guard (dcGuardLoad) khud load kar leta hai + write-through same
// server pe turant disk update karta hai + read-miss guard missing key foran
// Storj se laata hai. Har JID ek waqt me sirf EK server pe chalti hai (fleet
// claim), is liye cross-server staleness ka window bahut chhota hai — 30min
// safety-refresh bilkul kaafi hai. Isse settings re-read ka Storj egress
// ~6x kam ho jata hai (behaviour me 0% farak).
func dcRefreshPass() {
	if !dcReady || !storj.Ready() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	storj.mu.Lock()
	shards := append([]*storjShard(nil), storj.shards...)
	storj.mu.Unlock()
	for _, sh := range shards {
		dcRefreshSettingsHashes(ctx, sh)
	}
}

// dcRefreshSettingsHashes — kv/hashes/settings:* ko re-read karke disk update.
func dcRefreshSettingsHashes(ctx context.Context, sh *storjShard) {
	prefix := kvHashesPrefix + kvEncode("settings:") // "kv/hashes/settings%3A"
	groups := map[string]map[string]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(prefix))
	for obj := range objCh {
		if obj.Err != nil || obj.Size > dcBulkMaxBytes {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, kvHashesPrefix)
		idx := strings.Index(rest, "/")
		if idx < 0 {
			continue
		}
		hashKey := kvDecode(rest[:idx])
		field := kvDecode(rest[idx+1:])
		if hashKey == "" || field == "" {
			continue
		}
		data, found, err := kvReadShard(ctx, sh, obj.Key)
		if err != nil || !found {
			continue
		}
		if groups[hashKey] == nil {
			groups[hashKey] = map[string]string{}
		}
		groups[hashKey][field] = string(data)
	}
	for k, h := range groups {
		dcSave(k, &keyState{Type: "hash", Hash: h})
	}
}

// dcStartFleetRefresher — fleet liveness keys (servers hash + sessions set)
// ko har 60s Storj se re-read karke disk update karta hai. Ye 2 chhoti keys
// hain (heartbeats + JID list) — bandwidth negligible, magar is se doosre
// servers ki heartbeats hamare disk view me FRESH rehti hain → fleetHolderAlive
// ko 2min+ stale nahi lagti → /health probe storm nahi uthta. (Probe storm
// se bachna = Render ingress bandwidth bachna.)
func dcStartFleetRefresher() {
	if !dcReady {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			dcRefreshFleetKeys()
		}
	}()
}

func dcRefreshFleetKeys() {
	if !dcReady || !storj.Ready() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// SAARE fleet hashes (servers heartbeat + per-JID claims) + sets (sessions)
	// ko refresh karo. Claims ZAROORI hain — warna doosre server ki nayi claim
	// hamare disk pe na dikhe → double-connect ka khatra. Ye sab chhoti keys
	// hain (heartbeats + 1-2 entry claims), bandwidth negligible.
	storj.mu.Lock()
	shards := append([]*storjShard(nil), storj.shards...)
	storj.mu.Unlock()
	for _, sh := range shards {
		dcRefreshAllHashesUnder(ctx, sh, kvHashesPrefix+kvEncode("goldmd:fleet:"))
		dcRefreshAllSetsUnder(ctx, sh, kvSetsPrefix+kvEncode("goldmd:fleet:"))
	}
}

// dcRefreshAllHashesUnder — prefix ke neeche saare hash keys ko Storj se
// padho aur disk pe likho (grouped by hash key).
func dcRefreshAllHashesUnder(ctx context.Context, sh *storjShard, prefix string) {
	groups := map[string]map[string]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(prefix))
	for obj := range objCh {
		if obj.Err != nil || obj.Size > dcBulkMaxBytes {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, kvHashesPrefix)
		idx := strings.Index(rest, "/")
		if idx < 0 {
			continue
		}
		hashKey := kvDecode(rest[:idx])
		field := kvDecode(rest[idx+1:])
		if hashKey == "" || field == "" {
			continue
		}
		data, found, err := kvReadShard(ctx, sh, obj.Key)
		if err != nil || !found {
			continue
		}
		if groups[hashKey] == nil {
			groups[hashKey] = map[string]string{}
		}
		groups[hashKey][field] = string(data)
	}
	for k, h := range groups {
		dcSave(k, &keyState{Type: "hash", Hash: h})
	}
}

// dcRefreshAllSetsUnder — prefix ke neeche saare set keys ko Storj se padho.
func dcRefreshAllSetsUnder(ctx context.Context, sh *storjShard, prefix string) {
	groups := map[string][]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(prefix))
	for obj := range objCh {
		if obj.Err != nil {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, kvSetsPrefix)
		idx := strings.Index(rest, "/")
		if idx < 0 {
			continue
		}
		setKey := kvDecode(rest[:idx])
		member := kvDecode(rest[idx+1:])
		if setKey == "" || member == "" {
			continue
		}
		groups[setKey] = append(groups[setKey], member)
	}
	for k, members := range groups {
		sort.Strings(members)
		dcSave(k, &keyState{Type: "set", Set: members})
	}
}

// dcBulkHashesPrefix — ek hi hash key ko Storj se padho aur disk pe likho.
func dcBulkHashesPrefix(ctx context.Context, sh *storjShard, prefix, hashKey string) {
	h := map[string]string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(prefix))
	for obj := range objCh {
		if obj.Err != nil || obj.Size > dcBulkMaxBytes {
			continue
		}
		field := kvDecode(strings.TrimPrefix(obj.Key, prefix))
		if field == "" {
			continue
		}
		data, found, err := kvReadShard(ctx, sh, obj.Key)
		if err != nil || !found {
			continue
		}
		h[field] = string(data)
	}
	if len(h) > 0 {
		dcSave(hashKey, &keyState{Type: "hash", Hash: h})
	}
}

// dcBulkSetPrefix — ek hi set key ko Storj se padho aur disk pe likho.
func dcBulkSetPrefix(ctx context.Context, sh *storjShard, prefix, setKey string) {
	members := []string{}
	objCh := sh.client.ListObjects(ctx, sh.bucket, minioListOpts(prefix))
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
	dcSave(setKey, &keyState{Type: "set", Set: members})
}
