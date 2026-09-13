package main

// ═══════════════════════════════════════════════════════════════════════════
//   GOLD-MD — FLEET ENGINE (Render 5GB survival system)
//
//   OWNER REQUEST: Render free tier pe bandwidth (5GB/month) sabse badi
//   expense hai. Ye file poora session-distribution + bandwidth-survival
//   engine hai — ZERO speed impact (sab kuch background goroutines me):
//
//   1. SESSION DISTRIBUTION (global watchdog):
//      - Har paired session ka per-JID sqlite blob Storj pe store hota hai
//        (goldmd:fleet:sess:<jid>) — koi bhi server utha ke connect kar sakta hai.
//      - goldmd:fleet:sessions = global JID registry (set).
//      - goldmd:fleet:claim:<jid> = hash {serverID: ts} — kaun sa server
//        is session ko chala raha hai.
//      - Watchdog har 30s: (a) heartbeat daalta hai (goldmd:fleet:servers),
//        (b) real egress (/proc/net/dev tx delta) Storj pe persist karta hai,
//        (c) orphan claims sweep karta hai (dead server ki claim hatao),
//        (d) agar is server pe jagah hai (Count() < max 2) aur koi session
//        bina live claim ke pada hai → claim karke Storj se restore + connect.
//      - Naya server online aaya → uska watchdog khud sessions claim kar
//        leta hai (cascade: server1 full → server2 → server3 ...).
//
//   2. BANDWIDTH REPORT (.host5gb):
//      - /proc/net/dev tx_bytes delta se REAL egress measure hota hai,
//        har 30s Storj pe "total|lastTx|ts|month" persist hota hai —
//        restart/deploy ke baad bhi count bacha rehta hai.
//      - Month badalne pe total reset (Render monthly cycle).
//
//   3. HEALTH EXTENSION (/health):
//      - {"sessions":N,"max":2,"re":"ACTIVE|STOPPED","sid":"svrX","used_mb":N}
//      - re = memoryWatchdog zinda hai ya nahi (fleetTouchMemWatch se).
//
//   RACE SAFETY: Storj S3 me atomic compare-and-swap nahi hai. Claim flow:
//   HSET(claim) → 3s ruk → re-verify HGETALL → agar do server ne race me
//   claim kiya to (a) newer ts jeetta hai, (b) equal ts pe lexicographically
//   chhota serverID jeetta hai (deterministic tie-break — dono server same
//   decision lete hain, split-brain nahi).
//
//   SPEED GUARANTEE: fleet ka koi bhi function message-processing path ko
//   nahi chhoota. PairSuccess/Connected hooks fire-and-forget goroutines
//   hain. Watchdog pure background me chalta hai. Bot speed pe 0% asar.
// ═══════════════════════════════════════════════════════════════════════════

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

)

// ── fleet KV keys (GLOBAL — koi server-namespace nahi, sab servers share) ──
const (
	fleetSessionsSet  = "goldmd:fleet:sessions"   // set: all known JIDs
	fleetClaimPrefix  = "goldmd:fleet:claim:"     // hash: jid -> {serverID: ts}
	fleetServersHash  = "goldmd:fleet:servers"    // hash: serverID -> heartbeat ts
	fleetEgressPrefix = "goldmd:fleet:egress:"    // str: "total|lastTx|ts|month"
	fleetBlobPrefix   = "goldmd:fleet:sess:"      // str: base64 per-JID sqlite
	fleetMetaPrefix   = "goldmd:fleet:meta:"      // str: JSON {owner,saved}
)

// ── timing / limits ──
const (
	fleetTickInterval = 30 * time.Second // watchdog tick
	fleetClaimEvery   = 2                // claim check har 2nd tick (60s)
	fleetOrphanAfter  = 5 * time.Minute  // heartbeat itna purani = dead
	fleetStaleServer  = 24 * time.Hour   // heartbeat itni purani = purge
	fleetRaceWait     = 3 * time.Second  // claim race re-verify window
	fleetFailCooldown = 10 * time.Minute // failed restore retry cooldown
	fleetHTTPTimeout  = 6 * time.Second  // remote /health timeout
)

// fleetBudgetMB: Render free monthly egress budget (5GB). .render5gb iske
// against remaining dikhaata hai. GOLDMD_FLEET_BANDWIDTH_MB se override.
var fleetBudgetMB = func() int {
	if v := envInt("GOLDMD_FLEET_BANDWIDTH_MB", 0); v > 0 {
		return v
	}
	return 5120
}()

// fleetSelfID: is deployment ka unique ID. GOLDMD_SERVER_ID (Render env me
// set karo) → RENDER_EXTERNAL_URL → RENDER_INSTANCE_ID → hostname → "standalone".
var fleetSelfID = func() string {
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
	return "standalone"
}()

// fleet state
var (
	fleetMgr     *Manager
	fleetDBPath  string
	fleetRunning atomic.Bool
	fleetLastMem atomic.Int64 // unix nano of last memoryWatchdog touch
	// fleetFailedAt: local cooldown map — jinka restore/connect fail hua,
	// unko 10min tak dobara try nahi karte (flap prevention).
	fleetFailedMu sync.Mutex
	fleetFailedAt = make(map[string]int64)
)

// fleetInit bootstraps the fleet engine. Background goroutine launch hoti
// hai — caller (main.go) kabhi block nahi hota. Agar Storj ready nahi hai
// to fleet silently skip (single-server mode, sab kuch pehle jaisa).
func fleetInit(m *Manager, dbPath string) {
	fleetMgr = m
	fleetDBPath = dbPath
	if m == nil || m.Redis == nil || !storjReadyFlag() {
		InfoLog("FLEET: Storj/KV not ready — session-distribution watchdog disabled (standalone mode)")
		return
	}
	if !fleetRunning.CompareAndSwap(false, true) {
		return
	}
	go fleetWatchdog()
	InfoLog("FLEET: session-distribution watchdog started (sid=%s, max=%d/server, tick=%s)",
		fleetSelfID, maxPairedSessions(), fleetTickInterval)
}

// storjReadyFlag reports whether the global Storj store is initialised.
func storjReadyFlag() bool { return storj.Ready() }

// ═══════════════════════════════════════════════════════════════════════
//   WATCHDOG LOOP
// ═══════════════════════════════════════════════════════════════════════

func fleetWatchdog() {
	defer func() {
		if r := recover(); r != nil {
			// watchdog kabhi die nahi hoga — panic ho to 30s baad phir
			_ = r
			time.Sleep(fleetTickInterval)
			go fleetWatchdog()
		}
	}()

	// pehla tick foran — boot ke turant baad heartbeat + orphan sweep.
	tick := 0
	for {
		if fleetMgr == nil || fleetMgr.IsShuttingDown() {
			return
		}

		fleetHeartbeat()
		fleetPushEgress()

		if tick%2 == 0 {
			// orphan sweep + claim — 60s cadence (light on Storj reads).
			alive := fleetAliveServers()
			fleetOrphanSweep(alive)
			if !fleetMgr.IsShuttingDown() {
				fleetClaimAvailable()
			}
		}
		tick++

		time.Sleep(fleetTickInterval)
	}
}

// fleetHeartbeat marks this server alive in the global servers hash.
func fleetHeartbeat() {
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return
	}
	_, _ = fleetMgr.Redis.cmd("HSET", fleetServersHash, fleetSelfID,
		strconv.FormatInt(time.Now().Unix(), 10))
}

// fleetAliveServers returns the set of serverIDs whose heartbeat is fresh
// (newer than fleetOrphanAfter). Khud ko hamesha include karta hai.
func fleetAliveServers() map[string]bool {
	alive := map[string]bool{fleetSelfID: true}
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return alive
	}
	r, err := fleetMgr.Redis.cmd("HGETALL", fleetServersHash)
	if err != nil {
		return alive
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return alive
	}
	now := time.Now().Unix()
	for i := 0; i+1 < len(pairs); i += 2 {
		sid, tsStr := pairs[i], pairs[i+1]
		ts, err := strconv.ParseInt(strings.TrimSpace(tsStr), 10, 64)
		if err != nil {
			continue
		}
		if now-ts < int64(fleetOrphanAfter/time.Second) {
			alive[sid] = true
		} else if now-ts > int64(fleetStaleServer/time.Second) {
			// 24h+ purani heartbeat — field purge (best effort).
			_, _ = fleetMgr.Redis.cmd("HDEL", fleetServersHash, sid)
		}
	}
	return alive
}

// fleetOrphanSweep releases claims held by dead servers so their sessions
// can be picked up by live servers (Render spin-down / crash scenario).
func fleetOrphanSweep(alive map[string]bool) {
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return
	}
	for _, jid := range fleetMgr.Redis.setMembers(fleetSessionsSet) {
		holders := fleetClaimHolders(jid)
		if len(holders) == 0 {
			continue // unclaimed — fleetClaimAvailable utha lega
		}
		anyAlive := false
		for sid := range holders {
			if alive[sid] {
				anyAlive = true
				break
			}
		}
		if !anyAlive {
			// sab holders dead — claims release kar do.
			for sid := range holders {
				_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
			}
			InfoLog("FLEET: orphan claim released for %s (all holders dead)", jid)
		}
	}
}

// fleetClaimHolders parses the claim hash for a JID → {serverID: ts}.
func fleetClaimHolders(jid string) map[string]int64 {
	out := map[string]int64{}
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return out
	}
	r, err := fleetMgr.Redis.cmd("HGETALL", fleetClaimPrefix+jid)
	if err != nil {
		return out
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return out
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		ts, err := strconv.ParseInt(strings.TrimSpace(pairs[i+1]), 10, 64)
		if err != nil {
			ts = 0
		}
		out[pairs[i]] = ts
	}
	return out
}

// fleetClaimAvailable: agar is server pe pairing ki jagah hai (Count() <
// max 2) aur koi global session bina LIVE claim ke pada hai → restore +
// connect. Ek hi claim per tick — cascade naturally failta hai jab tak
// sab servers full na ho jayein.
func fleetClaimAvailable() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	max := maxPairedSessions()
	if m.Count() >= max {
		return // server full — agla server uthayega
	}
	now := time.Now().Unix()
	fleetFailedMu.Lock()
	for jid, ts := range fleetFailedAt {
		if now-ts > int64(fleetFailCooldown/time.Second) {
			delete(fleetFailedAt, jid)
		}
	}
	fleetFailedMu.Unlock()

	for _, jid := range m.Redis.setMembers(fleetSessionsSet) {
		if m.IsShuttingDown() || m.Count() >= max {
			return
		}
		if m.AlreadyConnected(fleetUserPart(jid)) {
			continue
		}
		fleetFailedMu.Lock()
		_, recentlyFailed := fleetFailedAt[jid]
		fleetFailedMu.Unlock()
		if recentlyFailed {
			continue
		}

		holders := fleetClaimHolders(jid)
		if len(holders) > 0 {
			// koi live holder hai? (dead holders ko orphan sweep hata dega)
			alive := fleetAliveServers()
			taken := false
			for sid := range holders {
				if alive[sid] {
					taken = true
					break
				}
			}
			if taken {
				continue
			}
		}

		InfoLog("FLEET: claiming unowned session %s (paired %d/%d)", jid, m.Count(), max)
		fleetRestoreAndConnect(jid)
		// claim attempts ke beech thoda gap — Storj read storm nahi.
		time.Sleep(5 * time.Second)
	}
}

// fleetRestoreAndConnect executes the race-safe claim + restore + connect
// flow for one JID. Blocking hai — sirf watchdog goroutine se call hota hai.
func fleetRestoreAndConnect(jid string) {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)

	// 1. claim stamp daalo.
	_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID, now)

	// 2. race window — doosre servers ko bhi ye milega hoga.
	time.Sleep(fleetRaceWait)

	// 3. re-verify + deterministic tie-break.
	holders := fleetClaimHolders(jid)
	myTS := holders[fleetSelfID]
	won := true
	for sid, ts := range holders {
		if sid == fleetSelfID {
			continue
		}
		if ts > myTS || (ts == myTS && sid < fleetSelfID) {
			won = false
			break
		}
	}
	if !won {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		return // doosre server ne jeet liya — hum chup
	}

	// 4. local device hai to seedha connect, warna Storj blob se restore.
	if !fleetDeviceExists(jid) {
		if err := fleetRestoreBlob(jid); err != nil {
			WarnLog("FLEET: blob restore failed for %s: %v", jid, err)
			_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
			fleetMarkFailed(jid)
			return
		}
	}

	// 5. connect (registration + AutoLoad-style flow StartSession me hai).
	if err := m.StartSession(jid); err != nil {
		WarnLog("FLEET: connect failed for %s: %v", jid, err)
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		fleetMarkFailed(jid)
		return
	}
	OkLog("FLEET: session %s restored from Storj and connected (server %s)", jid, fleetSelfID)
}

func fleetMarkFailed(jid string) {
	fleetFailedMu.Lock()
	fleetFailedAt[jid] = time.Now().Unix()
	fleetFailedMu.Unlock()
}

// fleetUserPart extracts the bare phone from a JID ("9231...@s.whatsapp.net"
// ya "9231...:12@s.whatsapp.net" → "9231...").
func fleetUserPart(jid string) string {
	if i := strings.Index(jid, ":"); i > 0 {
		jid = jid[:i]
	}
	if i := strings.Index(jid, "@"); i > 0 {
		jid = jid[:i]
	}
	return jid
}

// ═══════════════════════════════════════════════════════════════════════
//   SQLITE: per-JID blob extract / merge
// ═══════════════════════════════════════════════════════════════════════

// fleetJIDTables: whatsmeow ke wo tables jinme session-scoped rows hain.
// Pattern: "9231...:%" matches "9231...:5@s.whatsapp.net" (ad-JID form)
// aur "9231...:@..." (device 0) — phone numbers me %/_ nahi hote, safe.
var fleetJIDTables = []struct{ table, col string }{
	{"whatsmeow_device", "jid"},
	{"whatsmeow_pre_keys", "jid"},
	{"whatsmeow_identity_keys", "our_jid"},
	{"whatsmeow_sessions", "our_jid"},
	{"whatsmeow_sender_keys", "our_jid"},
	{"whatsmeow_app_state_sync_keys", "jid"},
	{"whatsmeow_app_state_version", "jid"},
	{"whatsmeow_app_state_mutation_macs", "jid"},
	{"whatsmeow_contacts", "our_jid"},
	{"whatsmeow_chat_settings", "our_jid"},
	{"whatsmeow_message_secrets", "our_jid"},
	{"whatsmeow_privacy_tokens", "our_jid"},
	{"whatsmeow_nct_salt", "our_jid"},
	{"whatsmeow_event_buffer", "our_jid"},
	{"whatsmeow_retry_buffer", "our_jid"},
}

// fleetDeviceExists checks the local sqlite store for this JID's device
// (read-only connection — container ko disturb nahi karta).
func fleetDeviceExists(jid string) bool {
	if fleetDBPath == "" {
		return false
	}
	user := fleetUserPart(jid)
	if user == "" {
		return false
	}
	db, err := sql.Open("sqlite", "file:"+fleetDBPath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM whatsmeow_device WHERE jid LIKE ?", user+":%").Scan(&n)
	return err == nil && n > 0
}

// fleetRestoreBlob downloads the per-JID sqlite blob from Storj and merges
// its rows into the local goldmd.db (INSERT OR IGNORE — local rows safe).
func fleetRestoreBlob(jid string) error {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return fmt.Errorf("fleet not initialised")
	}
	b64, ok := m.Redis.getStringKV(fleetBlobPrefix + jid)
	if !ok || b64 == "" {
		return fmt.Errorf("no session blob in Storj")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("blob base64: %w", err)
	}
	tmp := filepath.Join(os.TempDir(), "goldmd-fleet-"+fleetUserPart(jid)+".db")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("tmp write: %w", err)
	}
	defer os.Remove(tmp)
	if err := fleetMergeSessionDB(fleetDBPath, tmp); err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	return nil
}

// fleetMergeSessionDB merges src (per-JID transport DB) rows into dst
// (local goldmd.db). ATTACH + INSERT OR IGNORE — existing rows untouched.
func fleetMergeSessionDB(dst, src string) error {
	db, err := sql.Open("sqlite", "file:"+dst+"?mode=rwc&_busy_timeout=10000")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "ATTACH DATABASE "+sqlQuoteIdent(src)+" AS srcdb"); err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	defer db.Exec("DETACH DATABASE srcdb")

	// device table pehle (FK order), baaki best-effort.
	for _, t := range fleetJIDTables {
		stmt := fmt.Sprintf("INSERT OR IGNORE INTO %s SELECT * FROM srcdb.%s", t.table, t.table)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			// non-fatal: ek table skip ho jaye to bhi device+prekeys se
			// connect ho sakta hai.
			_ = err
		}
	}
	return nil
}

// fleetExtractJIDDB builds a fresh per-JID transport DB (dst) containing
// ONLY the given JID's rows from the source DB (src = goldmd.db).
func fleetExtractJIDDB(jid, src, dst string) error {
	user := fleetUserPart(jid)
	if user == "" {
		return fmt.Errorf("bad jid %q", jid)
	}
	_ = os.Remove(dst)
	db, err := sql.Open("sqlite", "file:"+dst+"?mode=rwc&_busy_timeout=10000")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "ATTACH DATABASE "+sqlQuoteIdent(src)+" AS srcdb"); err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	defer db.Exec("DETACH DATABASE srcdb")

	pattern := user + ":%"
	for _, t := range fleetJIDTables {
		stmt := fmt.Sprintf(
			"CREATE TABLE %s AS SELECT * FROM srcdb.%s WHERE %s LIKE ?",
			t.table, t.table, t.col)
		if _, err := db.ExecContext(ctx, stmt, pattern); err != nil {
			_ = err // best-effort: jo table na ho wo skip
		}
	}
	return nil
}

// sqlQuoteIdent wraps a filesystem path in single quotes for sqlite DSN/ATTACH.
func sqlQuoteIdent(p string) string {
	return "'" + strings.ReplaceAll(p, "'", "''") + "'"
}

// ═══════════════════════════════════════════════════════════════════════
//   BLOB SAVE (pair/connect hooks) + lifecycle
// ═══════════════════════════════════════════════════════════════════════

// fleetSaveBlob extracts this JID's rows from the shared goldmd.db into a
// fresh per-JID sqlite, base64-encodes it, and pushes it to Storj. Registry
// set + owner meta bhi update. Fire-and-forget goroutine me chalao.
func fleetSaveBlob(jid string) {
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil || fleetDBPath == "" || !storjReadyFlag() {
			return
		}
		if err := checkpointSQLite(fleetDBPath); err != nil {
			_ = err // WAL merge best-effort — blob phir bhi ban jayega
		}
		tmp := filepath.Join(os.TempDir(), "goldmd-fleet-x-"+fleetUserPart(jid)+".db")
		defer os.Remove(tmp)
		if err := fleetExtractJIDDB(jid, fleetDBPath, tmp); err != nil {
			WarnLog("FLEET: extract failed for %s: %v", jid, err)
			return
		}
		data, err := os.ReadFile(tmp)
		if err != nil {
			return
		}
		enc := base64.StdEncoding.EncodeToString(data)
		if err := m.Redis.setString(fleetBlobPrefix+jid, enc); err != nil {
			WarnLog("FLEET: blob upload failed for %s: %v", jid, err)
			return
		}
		meta, _ := json.Marshal(map[string]any{
			"owner": fleetOwnerFor(jid),
			"saved": time.Now().Unix(),
			"by":    fleetSelfID,
		})
		_ = m.Redis.setString(fleetMetaPrefix+jid, string(meta))
		_ = m.Redis.setAdd(fleetSessionsSet, jid)
	}()
}

// fleetRefreshBlobs re-pushes blobs for every CURRENTLY CONNECTED session
// (periodic safety net — key rotation / prekey updates pakadne ke liye).
// 10-min backup ticker se call hota hai, background me.
func fleetRefreshBlobs() {
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		for _, s := range m.List() {
			if s == nil || !s.Paired {
				continue
			}
			fleetSaveBlob(s.JID)
			time.Sleep(2 * time.Second) // Storj pe load spread
		}
	}()
}

func fleetOwnerFor(jid string) string {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return ""
	}
	return m.Redis.GetSetting(jid, "owner", "")
}

// ── lifecycle hooks (EventHandler se fire — sab async) ──

// fleetOnPairSuccess: naya session pair hua → blob push + claim + registry.
func fleetOnPairSuccess(jid string) {
	fleetSaveBlob(jid)
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID,
			strconv.FormatInt(time.Now().Unix(), 10))
		_ = m.Redis.setAdd(fleetSessionsSet, jid)
	}()
}

// fleetOnConnected: session live hua → claim refresh (hum iske owner hain).
func fleetOnConnected(jid string) {
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		holders := fleetClaimHolders(jid)
		if len(holders) == 0 {
			_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID,
				strconv.FormatInt(time.Now().Unix(), 10))
		}
		fleetSaveBlob(jid) // connected keys latest rakho
	}()
}

// fleetOnCleanup: session locally hata gaya (logout) → blob + registry del.
// Ye session ab fleet me nahi aayega (WhatsApp ne khud logout kiya).
func fleetOnCleanup(jid string) {
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		_, _ = m.Redis.cmd("DEL", fleetClaimPrefix+jid)
		_ = m.Redis.setRem(fleetSessionsSet, jid)
		_ = m.Redis.setDel(fleetBlobPrefix + jid)
		_ = m.Redis.setDel(fleetMetaPrefix + jid)
	}()
}

// ═══════════════════════════════════════════════════════════════════════
//   REAL EGRESS TRACKING (/proc/net/dev) — .render5gb data
// ═══════════════════════════════════════════════════════════════════════

// netTxBytes sums tx_bytes across all non-loopback interfaces.
func netTxBytes() int64 {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(data), "\n")[2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(parts[1]))
		if len(fields) < 9 {
			continue // rx(8) ... tx(9th index = bytes sent)
		}
		if v, err := strconv.ParseInt(fields[8], 10, 64); err == nil {
			total += v
		}
	}
	return total
}

var (
	fleetEgressMu     sync.Mutex
	fleetEgressTotal  int64  // bytes this month
	fleetEgressLastTx int64  // last /proc snapshot
	fleetEgressMonth  string // "2025-01"
)

// fleetPushEgress snapshots /proc/net/dev, adds the delta to the running
// total (counter reset / restart gap handled) and persists it to Storj.
func fleetPushEgress() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	now := time.Now().UTC()
	month := now.Format("2006-01")
	cur := netTxBytes()

	fleetEgressMu.Lock()
	if fleetEgressMonth == "" {
		fleetEgressMonth = month
	}
	if month != fleetEgressMonth {
		// naya calendar month — Render ka monthly cycle reset.
		fleetEgressTotal = 0
		fleetEgressMonth = month
		fleetEgressLastTx = cur
	}
	delta := cur - fleetEgressLastTx
	if delta > 0 {
		fleetEgressTotal += delta
	}
	fleetEgressLastTx = cur
	total, lastTx := fleetEgressTotal, fleetEgressLastTx
	fleetEgressMu.Unlock()

	val := fmt.Sprintf("%d|%d|%d|%s", total, lastTx, now.Unix(), month)
	_, _ = m.Redis.cmd("SET", fleetEgressPrefix+fleetSelfID, val)
}

// fleetLoadEgress restores the persisted egress counters at boot (survives
// restarts). Boot-gap > 2 ticks → lastTx reset (downtime me egress zero).
func fleetLoadEgress() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	val, ok := m.Redis.getStringKV(fleetEgressPrefix + fleetSelfID)
	if !ok || val == "" {
		fleetEgressMu.Lock()
		fleetEgressTotal = 0
		fleetEgressLastTx = netTxBytes()
		fleetEgressMonth = time.Now().UTC().Format("2006-01")
		fleetEgressMu.Unlock()
		return
	}
	parts := strings.Split(val, "|")
	if len(parts) != 4 {
		return
	}
	total, _ := strconv.ParseInt(parts[0], 10, 64)
	lastTx, _ := strconv.ParseInt(parts[1], 10, 64)
	ts, _ := strconv.ParseInt(parts[2], 10, 64)
	month := parts[3]

	fleetEgressMu.Lock()
	defer fleetEgressMu.Unlock()
	nowMonth := time.Now().UTC().Format("2006-01")
	cur := netTxBytes()
	if month != nowMonth {
		// month badal gaya — cycle reset.
		fleetEgressTotal = 0
		fleetEgressLastTx = cur
		fleetEgressMonth = nowMonth
		return
	}
	fleetEgressMonth = month
	gap := time.Now().Unix() - ts
	if gap > int64(fleetTickInterval/time.Second)*3 {
		// lambe downtime ke baad tx counter bhi reset ho chuka hoga.
		fleetEgressLastTx = cur
	} else if cur >= lastTx {
		fleetEgressTotal = total + (cur - lastTx)
		fleetEgressLastTx = cur
	} else {
		fleetEgressTotal = total
		fleetEgressLastTx = cur
	}
}

// fleetEgressUsedMB returns this server's REAL used egress in MB.
func fleetEgressUsedMB() float64 {
	fleetEgressMu.Lock()
	defer fleetEgressMu.Unlock()
	return float64(fleetEgressTotal) / (1024 * 1024)
}

// ═══════════════════════════════════════════════════════════════════════
//   RE STATUS (memory watchdog liveness) + HEALTH EXTENSION
// ═══════════════════════════════════════════════════════════════════════

// fleetTouchMemWatch: memoryWatchdog loop har iteration me ye call karta
// hai — is se /health ka "re" field batata hai ki RAM watchdog zinda hai.
func fleetTouchMemWatch() { fleetLastMem.Store(time.Now().UnixNano()) }

// fleetREStatus: ACTIVE = memoryWatchdog (400MB clean / 450MB hard-restart
// engine) har 2min se fresh touch de raha hai. STOPPED = mara hua.
func fleetREStatus() string {
	last := fleetLastMem.Load()
	if last == 0 {
		return "STOPPED"
	}
	if time.Since(time.Unix(0, last)) > 120*time.Second {
		return "STOPPED"
	}
	return "ACTIVE"
}

// fleetHealthFields is the extended /health payload (backwards compatible —
// purane parsers sirf "sessions" padhte hain, extra fields ignore hote hain).
type fleetHealthFields struct {
	Bot      string  `json:"bot"`
	Status   string  `json:"status"`
	Sessions int     `json:"sessions"`
	Max      int     `json:"max"`
	RE       string  `json:"re"`
	SID      string  `json:"sid"`
	UsedMB   float64 `json:"used_mb"`
}

// fleetRemoteHealth fetches another server's /health (6s timeout).
func fleetRemoteHealth(url string) (*fleetHealthFields, error) {
	client := &http.Client{Timeout: fleetHTTPTimeout}
	healthURL := strings.TrimRight(url, "/") + "/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health status %d", resp.StatusCode)
	}
	var hf fleetHealthFields
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&hf); err != nil {
		return nil, err
	}
	return &hf, nil
}

// fleetServerInfo: one row of the .server / .render5gb report.
type fleetServerInfo struct {
	Name    string
	URL     string
	Online  bool
	Sessions int
	Max     int
	RE      string
	SID     string
	UsedMB  float64
}

// fleetScanAll checks every server in servers.json in PARALLEL (real /health
// hits — koi fake data nahi). servers.json me jo hai wahi dikhta hai.
func fleetScanAll() []fleetServerInfo {
	loadServersConfig()
	cfg := serversCfg
	out := make([]fleetServerInfo, len(cfg.Servers))
	var wg sync.WaitGroup
	for i, srv := range cfg.Servers {
		wg.Add(1)
		go func(idx int, s serverEntry) {
			defer wg.Done()
			info := fleetServerInfo{
				Name:    s.Name,
				URL:     s.URL,
				Max:     cfg.MaxPerServer,
				RE:      "STOPPED", // offline = engine stopped
			}
			if hf, err := fleetRemoteHealth(s.URL); err == nil {
				info.Online = true
				info.Sessions = hf.Sessions
				if hf.Max > 0 {
					info.Max = hf.Max
				}
				info.RE = hf.RE
				info.SID = hf.SID
				info.UsedMB = hf.UsedMB
			}
			out[idx] = info
		}(i, srv)
	}
	wg.Wait()
	return out
}

// ═══════════════════════════════════════════════════════════════════════
//   HEALTH HANDLER (manager.go HealthHandler isko use karta hai)
// ═══════════════════════════════════════════════════════════════════════

// fleetWriteHealth emits the extended health JSON. Old clients (panel) jo
// sirf {"bot","status","sessions"} padhte the, wo ab bhi kaam karte hain.
func fleetWriteHealth(w io.Writer, sessions int) {
	fmt.Fprintf(w, `{"bot":"GOLD-MD","status":"online","sessions":%d,"max":%d,"re":"%s","sid":"%s","used_mb":%.1f}`,
		sessions, maxPairedSessions(), fleetREStatus(), fleetSelfID, fleetEgressUsedMB())
}

// ═══════════════════════════════════════════════════════════════════════
//   RENDER REPORT TEXT (silent commands isko use karte hain)
// ═══════════════════════════════════════════════════════════════════════

// fleetRender5GBText builds the .host5gb report: har RUNNING server ka
// 5GB / used / remaining (real /proc egress se, Storj-persisted).
func fleetRender5GBText() string {
	var b strings.Builder
	b.WriteString("*🔰 GOLD-MD HOST 5GB BANDWIDTH 🔰*\n\n")
	running := 0
	for _, s := range fleetScanAll() {
		if !s.Online {
			continue
		}
		running++
		remaining := float64(fleetBudgetMB) - s.UsedMB
		remStr := fmt.Sprintf("%.0fMB", remaining)
		if remaining >= 1024 {
			remStr = fmt.Sprintf("%.2fGB", remaining/1024)
		}
		pct := 0.0
		if fleetBudgetMB > 0 {
			pct = (s.UsedMB / float64(fleetBudgetMB)) * 100
		}
		b.WriteString(fmt.Sprintf("*🔰 %s*\n", s.Name))
		b.WriteString(fmt.Sprintf("*🔰 BANDWIDTH :❯ ❮ 5GB / %.0fMB used / %s remaining ❯*\n", s.UsedMB, remStr))
		b.WriteString(fmt.Sprintf("*🔰 USAGE :❯ %.1f%%*\n\n", pct))
	}
	if running == 0 {
		b.WriteString("❯ _Koi server ONLINE nahi mila — servers on karo._\n")
	}
	b.WriteString("_Real egress (/proc/net/dev), Storj-persisted, restart-safe._")
	return b.String()
}

// fleetServersInfoText builds the .server menu — user ka exact format,
// REAL checks (live /health + real pairing count + RE status):
//
//	*🔰 GOLD-MD SERVERS INFO 🔰*
//
//	*🔰 SERVER 1 INFORMATION 🔰*
//	*🔰 STATUS :❯ ACTIVE*
//	*🔰 MAX PAIRING :❯ 2*
//	*🔰 PAIRED :❯ ❮ 1/2 ❯*
//	*🔰 RE :❯ ACTIVE*
func fleetServersInfoText() string {
	var b strings.Builder
	b.WriteString("*🔰 GOLD-MD SERVERS INFO 🔰*\n\n")
	for _, s := range fleetScanAll() {
		status := "STOPPED"
		if s.Online {
			status = "ACTIVE"
		}
		paired := "0/0"
		if s.Max > 0 {
			if s.Sessions >= s.Max {
				paired = fmt.Sprintf("%d/%d FULL", s.Sessions, s.Max)
			} else {
				paired = fmt.Sprintf("%d/%d", s.Sessions, s.Max)
			}
		}
		b.WriteString(fmt.Sprintf("*🔰 %s INFORMATION 🔰*\n", s.Name))
		b.WriteString(fmt.Sprintf("*🔰 STATUS :❯ %s*\n", status))
		b.WriteString(fmt.Sprintf("*🔰 MAX PAIRING :❯ %d*\n", s.Max))
		b.WriteString(fmt.Sprintf("*🔰 PAIRED :❯ ❮ %s ❯*\n", paired))
		b.WriteString(fmt.Sprintf("*🔰 RE :❯ %s*\n\n", s.RE))
	}
	return b.String()
}

// fleetSessionsCountText: .sessions alias ka short fleet view (owner only).
func fleetSessionsCountText() string {
	m := fleetMgr
	var b strings.Builder
	b.WriteString("*🔰 GOLD-MD FLEET SESSIONS 🔰*\n\n")
	total := 0
	for _, s := range fleetScanAll() {
		if !s.Online {
			continue
		}
		total += s.Sessions
		b.WriteString(fmt.Sprintf("*🔰 %s :❯ ❮ %d/%d ❯*\n", s.Name, s.Sessions, s.Max))
	}
	if m != nil {
		b.WriteString(fmt.Sprintf("\n*🔰 THIS SERVER (%s) :❯ ❮ %d/%d ❯*\n", fleetSelfID, m.Count(), maxPairedSessions()))
	}
	b.WriteString(fmt.Sprintf("\n*🔰 TOTAL LIVE SESSIONS :❯ %d*\n", total))
	return b.String()
}
