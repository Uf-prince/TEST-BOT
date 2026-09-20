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
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
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
	fleetSessionsSet    = "goldmd:fleet:sessions" // set: all known JIDs
	fleetClaimPrefix    = "goldmd:fleet:claim:"   // hash: jid -> {serverID: ts}
	fleetServersHash    = "goldmd:fleet:servers"  // hash: serverID -> heartbeat ts
	fleetEgressPrefix   = "goldmd:fleet:egress:"  // str: "total|lastTx|ts|month"
	fleetBlobPrefix     = "goldmd:fleet:sess:"    // str: base64 per-JID sqlite
	fleetMetaPrefix     = "goldmd:fleet:meta:"    // str: JSON {owner,saved}
	fleetFailMarkPrefix = "goldmd:fleet:fail:"    // str: dead sid (failover marker)
)

// ── timing / limits ──
const (
	fleetTickInterval = 30 * time.Second // watchdog tick
	fleetClaimEvery   = 2                // claim check har 2nd tick (60s)
	fleetOrphanAfter  = 5 * time.Minute  // heartbeat itna purani = dead
	fleetStaleServer  = 24 * time.Hour   // heartbeat itni purani = purge
	fleetRaceWait     = 3 * time.Second  // claim race re-verify window
	fleetFailCooldown = 10 * time.Minute // failed restore retry cooldown
	fleetHTTPTimeout  = 4 * time.Second  // remote /health timeout (quick public .server)
	fleetProbeAfter   = 2 * time.Minute  // heartbeat stale = ACTIVE /health probe start
	fleetClaimFreshTrust = 10 * time.Minute // fresh claim = trust, probe nahi (race window)
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
// fleetBind: fleetMgr/fleetDBPath set karo BINA watchdog start kiye.
// main.go isko AutoLoad se PEHLE call karta hai taake AutoLoad ka
// zombie-return guard (fleetHeldByLiveServer) kaam kar sake. Watchdog
// fleetInit me baad me start hota hai — AutoLoad ke concurrent-start
// race se door.
func fleetBind(m *Manager, dbPath string) {
	if m == nil {
		return
	}
	fleetMgr = m
	if dbPath != "" {
		fleetDBPath = dbPath
	}
}

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
	go fleetHealGZ1()
	InfoLog("FLEET: session-distribution watchdog started (sid=%s, max=%d/server, tick=%s)",
		fleetSelfID, maxPairedSessions(), fleetTickInterval)
}

// ── EMERGENCY GZ1 BOOT HEAL (mixed-fleet recovery) ─────────────────────
//
// Broken build (c704231) ne shared KV values "GZ1:"+gzip me likh di thin.
// NAYA binary unhe padh leta hai (kvGunzipMaybe) — isliye naye binary pe
// sessions foran online aa sakte hain. Lekin PURANE binaries (jo abhi bhi
// 200-server fleet me chalein) plain base64 expect karte hain aur GZ1 pe
// base64 decode fail kar dete hain -> session restore fail -> OFFLINE.
//
// Heal un stored values ko PLAIN me rewrite karta hai taake poori fleet
// (purane binaries included) wapas padh sake. Bandwidth-safe design:
//
//	fleetGZ1HealFlag = "goldmd:gz1:heal:v1"
//	  - apna sessiondb key: HAR server boot pe heal (1 GET — sasta,
//	    broken build usi sid pe chala tha to yehi key GZ1 hui hogi)
//	  - global sweep (saare fleet blob keys): flag-guarded — sirf pehla
//	    naya binary jo boot hota hai sweep karta hai, flag SET karta hai,
//	    baaki servers flag dekh ke SKIP kar dete hain (0 bandwidth).
//	    Sweep idempotent hai: plain keys pe 1 GET + prefix-check + skip.
//	    Flag-set fail ho jaye to agli boot pe phir chalega (race-free —
//	    sab same plain value likhte hain).
//
// Boot pe go-routine me chalta hai — restore/watchdog pehle chalte hain
// (naya binary GZ1 khud padh leta hai, sweep sirf purane binaries ke
// liye plain format wapas laata hai, koi urgency nahi).
const fleetGZ1HealFlag = "goldmd:gz1:heal:v1"

func fleetHealGZ1() {
	defer func() {
		if r := recover(); r != nil {
			WarnLog("GZ1-HEAL: boot heal panic recovered: %v", r)
		}
	}()
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	// (a) apna sessiondb key — har server, har boot (1 GET, negligible).
	if m.Redis.HealGZ1Key(m.Redis.sessionDBKey()) {
		InfoLog("GZ1-HEAL: apna sessiondb blob plain me rewrite ho gaya")
	}
	// (b) global fleet-blob sweep — flag-guarded, fleet-wide EK baar.
	if v, ok := m.Redis.getStringKV(fleetGZ1HealFlag); ok && v != "" {
		return // kisi aur server ne sweep kar liya — skip (0 bandwidth)
	}
	keys := make([]string, 0, 256)
	for _, jid := range m.Redis.setMembers(fleetSessionsSet) {
		if jid == "" {
			continue
		}
		keys = append(keys, fleetBlobPrefix+jid)
	}
	if len(keys) == 0 {
		_ = m.Redis.setString(fleetGZ1HealFlag, fleetSelfID)
		return
	}
	n := m.Redis.HealGZ1Keys(keys)
	// flag SET — agli boots skip kar dein (sweep one-time hai).
	if err := m.Redis.setString(fleetGZ1HealFlag, fleetSelfID); err != nil {
		WarnLog("GZ1-HEAL: flag set nahi hua — agli boot pe sweep phir chalega (idempotent): %v", err)
	}
	InfoLog("GZ1-HEAL: fleet sweep done — %d/%d blob keys plain me rewrite hue", n, len(keys))
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

		JSONDebug("FLEET_WATCHDOG_TICK", map[string]any{
			"self": fleetSelfID, "tick": tick, "slots_used": fleetMgr.SlotsUsed(),
		})
		fleetHeartbeat()
		fleetPushEgress()
		fleetWarnPreCrash()

		if tick%2 == 0 {
			// orphan sweep + claim — 60s cadence (light on Storj reads).
			// alive-set ab per-holder probe se aata hai (fleetHolderAlive).
			fleetOrphanSweep()
			if !fleetMgr.IsShuttingDown() {
				fleetClaimAvailable()
			}
		}
		tick++

		time.Sleep(fleetTickInterval)
	}
}

// fleetHeartbeat marks this server alive in the global servers hash.
//
// BANDWIDTH JUGAD (60s cadence): watchdog 30s tick pehle HAR tick HSET
// mar raha tha (2880 PUT/day). Liveness window 2-min hai (fleetProbeAfter)
// aur stale-window me ACTIVE /health probe fallback bhi zinda hai — is
// liye 60s pe ek hi write bilkul safe hai: freshness margin 2x, aur agar
// tick miss bhi ho jaye to probe holder ko alive confirm karta hai.
// Failover timing pe ZERO asar (orphan sweep 60s pe hi chalta tha/rahega).
var (
	fleetHeartbeatMu   sync.Mutex
	fleetLastHeartbeat time.Time
)

func fleetHeartbeat() {
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return
	}
	fleetHeartbeatMu.Lock()
	if time.Since(fleetLastHeartbeat) < 55*time.Second {
		fleetHeartbeatMu.Unlock()
		return // abhi hi likha tha — skip (ek minute me ek hi HSET)
	}
	fleetLastHeartbeat = time.Now()
	fleetHeartbeatMu.Unlock()
	// NAYA format: "<ts>|<sessions>|<max>|<url>" — Session Resurrector isse
	// pata lagata hai kaunsa server FREE hai (sessions < max) bina kisi HTTP
	// probe ke, AUR (owner fix B) 4th segment = is server ka public URL —
	// hostname-style sid (svr11221) ka URL ab fleet me discoverable hai.
	// Purana "<ts>" / "<ts>|<sessions>|<max>" format bhi parse hota rehta hai.
	val := strconv.FormatInt(time.Now().Unix(), 10) + "|" +
		strconv.Itoa(fleetMgr.SlotsUsed()) + "|" +
		strconv.Itoa(maxPairedSessions()) + "|" +
		fleetSelfURL()
	_, _ = fleetMgr.Redis.cmd("HSET", fleetServersHash, fleetSelfID, val)
	JSONDebug("FLEET_HEARTBEAT", map[string]any{
		"self": fleetSelfID, "value": val, "slots_used": fleetMgr.SlotsUsed(), "slots_max": maxPairedSessions(),
	})
}

// fleetSelfURL: is server ka PUBLIC URL (fleet discoverability — owner fix
// B, 2026-09-16). GOLDMD_PUBLIC_URL (launch_fresh.sh me tunnel URL set hota
// hai) → RENDER_EXTERNAL_URL → "". Heartbeat ke 4th segment me jata hai;
// fleetServerURL isko hostname-style sid ke liye fallback me use karta hai.
func fleetSelfURL() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_PUBLIC_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return ""
}

// fleetURLFromHeartbeat: ek sid ka public URL heartbeat hash se (4th segment).
// fleetServerURL ka fallback — hostname-style sid (dot nahi) pe pehle ""
//// tha, ab heartbeat URL milta hai (cmd cache 3min TTL — sasta).
func fleetURLFromHeartbeat(sid string) string {
	if sid == "" || fleetMgr == nil || fleetMgr.Redis == nil {
		return ""
	}
	raw, err := fleetMgr.Redis.cmd("HGET", fleetServersHash, sid)
	if err != nil || len(raw) == 0 {
		return ""
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	parts := strings.Split(v, "|")
	if len(parts) < 4 {
		return ""
	}
	u := strings.TrimSpace(parts[3])
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return strings.TrimSuffix(u, "/")
	}
	return ""
}

// fleetHeartbeatURLs: servers hash → {sid: url} (ek hi HGETALL, 4th segment).
// guardRemoteServers isse sandbox/tunnel servers ko probe list me include
// karta hai (pehle sirf servers.json tha — sandbox invisible tha).
func fleetHeartbeatURLs() map[string]string {
	out := map[string]string{}
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return out
	}
	r, err := fleetMgr.Redis.cmd("HGETALL", fleetServersHash)
	if err != nil {
		return out
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return out
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		parts := strings.Split(strings.TrimSpace(pairs[i+1]), "|")
		if len(parts) < 4 {
			continue
		}
		u := strings.TrimSpace(parts[3])
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			out[pairs[i]] = strings.TrimSuffix(u, "/")
		}
	}
	return out
}

// fleetOrphanSweep releases claims held by dead servers so their sessions
// can be picked up by live servers (Render spin-down / crash scenario).
//
// LIVENESS (fleetHolderAlive — unified):
//
//	heartbeat fresh (<2min)  → ALIVE (koi probe nahi — tick chal raha hai)
//	heartbeat 2min+ stale    → ACTIVE /health probe (4s timeout)
//	                           probe OK → ALIVE (busy server, rehne do)
//	                           probe FAIL → DEAD → claim release
//	heartbeat hi nahi        → DEAD (purani entry)
//
// Isse failover 5min wait → ~2min ho jata hai (owner ka order) — bina
// kisi alive server ke claim ko galat release kiye.
func fleetOrphanSweep() {
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return
	}
	// heartbeat source map — per-holder liveness decision (sid -> last ts).
	heartbeats := fleetHeartbeatMap()

	for _, jid := range fleetMgr.Redis.setMembers(fleetSessionsSet) {
		holders := fleetClaimHolders(jid)
		if len(holders) == 0 {
			continue // unclaimed — fleetClaimAvailable utha lega
		}
		allDead := true
		for sid := range holders {
			if fleetHolderAlive(sid, heartbeats) {
				allDead = false
				break
			}
		}
		if !allDead {
			continue // koi holder zinda hai — session usi ke paas
		}
		// sab holders dead — claims release kar do + FAILOVER marker
		// save karo (takeover hone pe owner ko notify karne ke liye).
		for sid := range holders {
			_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
			_ = fleetMgr.Redis.setString(fleetFailMarkPrefix+jid, sid)
		}
		InfoLog("FLEET: orphan claim released for %s (all holders dead, probe-confirmed)", jid)
	}
}

// fleetHeartbeatMap: servers hash → {sid: last heartbeat ts} (probe
// decision ke liye — fleetAliveServers se hataya, duplicate KV read bacha).
func fleetHeartbeatMap() map[string]int64 {
	out := map[string]int64{}
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return out
	}
	r, err := fleetMgr.Redis.cmd("HGETALL", fleetServersHash)
	if err != nil {
		return out
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return out
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		// Backward-compatible: "<ts>|<sessions>|<max>" (naya) ya "<ts>"
		// (purana) — sirf pehla segment hi ts hai.
		v := strings.TrimSpace(pairs[i+1])
		if i := strings.IndexByte(v, '|'); i >= 0 {
			v = v[:i]
		}
		ts, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		// 24h+ purani heartbeat — server hash se field purge (hygiene,
		// purane fleetAliveServers ka side-effect yahan shift hua).
		if time.Now().Unix()-ts > int64(fleetStaleServer/time.Second) {
			_, _ = fleetMgr.Redis.cmd("HDEL", fleetServersHash, pairs[i])
			continue
		}
		out[pairs[i]] = ts
	}
	return out
}

// fleetHolderAlive: ek claim holder REAL me zinda hai ya nahi — unified
// decision (orphanSweep + claimAvailable + zombie-guard sab yahi use
// karte hain). Heartbeat fresh = zinda. Stale = ACTIVE /health probe.
// Sirf watchdog/background paths se call hota hai — hot-path cost 0.
func fleetHolderAlive(sid string, heartbeats map[string]int64) bool {
	if sid == "" {
		return false
	}
	// hum khud — obviously zinda (yeh code chal raha hai).
	if sid == fleetSelfID {
		return true
	}
	ts, ok := heartbeats[sid]
	if !ok || ts <= 0 {
		// heartbeat hi nahi — purani claim entry → dead.
		return false
	}
	age := time.Now().Unix() - ts
	if age < int64(fleetProbeAfter/time.Second) {
		// heartbeat fresh (<2min) — watchdog tick de raha hai → zinda.
		return true
	}
	// heartbeat 2min+ stale — ab ACTIVE /health probe (1 attempt, 4s).
	url := fleetServerURL(sid)
	if url == "" {
		return false // URL resolve nahi hua → dead man lo
	}
	if fleetProbeURL(url + "/health") {
		return true // busy tha lekin process zinda hai
	}
	InfoLog("FLEET: active probe FAILED for %s (hb %ds stale) — treating dead", sid, age)
	return false
}

// fleetHeldByLiveServer: kya ye JID kisi AUR live server ke fresh claim
// me hai? (ZOMBIE-RETURN GUARD: dead server wapas aaya to AutoLoad isko
// dobara start na kare — dusre server pe already failover ho chuka hai.
// Double-connect war = WhatsApp stream-replace = logout. Ye guard war
// rokta hai.) AutoLoad ke batch goroutines se call hota hai.
func fleetHeldByLiveServer(jid string) bool {
	m := fleetMgr
	if m == nil || m.Redis == nil || !storjReadyFlag() {
		return false // fleet inactive — sab local load karo
	}
	holders := fleetClaimHolders(jid)
	if len(holders) == 0 {
		return false
	}
	hb := fleetHeartbeatMap()
	for sid, ts := range holders {
		if sid == fleetSelfID {
			continue
		}
		if !fleetHolderAlive(sid, hb) {
			continue // dead holder — claim stale, ignore (orphan sweep)
		}
		// LIVE holder mila. Magar ye ZOMBIE-CLAIM ho sakta hai: server
		// process zinda hai magar ye session uspe chal hi nahi raha
		// (claim connect ke waqt EK baar likha jata hai, phir kabhi
		// refresh nahi hota — session chala gaya / restore aadha raha
		// / disk wipe ho gaya). Asli check: holder ke /sessions me ye
		// JID mojood hai ya nahi (online YA offline — offline = device
		// uske paas hai, backoff/reconnect-loop me hai = sahi holder).
		if fleetHolderRunsSession(sid, ts, jid) {
			return true // waqai is live server ke paas ye session hai
		}
	}
	return false
}

// fleetHolderRunsSession: live claim-holder ke paas ye JID ka session
// WAQAI hai? LIVE server + missing session = ZOMBIE CLAIM (owner ke
// 2347067958986 wala case: gold-md-botx zinda tha, session wahan tha hi
// nahi — takeover hamesha ke liye block tha).
//
// RULES (bandwidth-safe, war-safe):
//   claim FRESH (<10min) → holder ne abhi connect/boot kiya hai — race
//     window me hai, probe ki zaroorat nahi — RESPECT (steal karne pe
//     double-connect war = stream-replace = logout).
//   claim STALE (10min+) → holder ke /sessions me JID dhoondo:
//     mojood (online ya offline) → holder device rakh raha hai → RESPECT.
//     NAHI mila → ZOMBIE → takeover allowed (ye function false deta hai).
func fleetHolderRunsSession(sid string, claimTS int64, jid string) bool {
	if sid == "" {
		return false
	}
	// hum khud: device disk pe hai ya nahi — direct check.
	if sid == fleetSelfID {
		return fleetDeviceExists(jid)
	}
	// fresh claim → trust (boot/restore race window). Isi window me
	// probing karna race me hi ghusna hai — bilkul nahi.
	if claimTS > 0 && time.Now().Unix()-claimTS < int64(fleetClaimFreshTrust/time.Second) {
		return true
	}
	url := fleetServerURL(sid)
	if url == "" {
		// URL hi resolve nahi hua — full server-ID/URL nahi pata.
		// Err on the SAFE side: agar claim fresh nahi hai aur probe
		// possible hi nahi hai to zombie confirm nahi kar sakte.
		return claimTS > 0
	}
	// stale claim → holder ke /sessions me JID mojood hai ya nahi.
	// Present (online/offline) = device holder ke paas = respect.
	// Absent = zombie claim → takeover allowed.
	return guardRemoteSessionPresent(url, jid)
}

// fleetServerURL resolves a serverID → base URL (https://{sid}/health).
// fleetSelfID wahi pattern use karta hai (RENDER_EXTERNAL_URL etc).
func fleetServerURL(sid string) string {
	if sid == "" {
		return ""
	}
	if strings.HasPrefix(sid, "http://") || strings.HasPrefix(sid, "https://") {
		return strings.TrimSuffix(sid, "/")
	}
	// hostname-style sid → Render external URL pattern nahi banta —
	// heuristic: pure-hostname ya onrender.com ho to https:// prefix.
	if strings.Contains(sid, ".") {
		return "https://" + strings.TrimSuffix(sid, "/")
	}
	// OWNER FIX B (2026-09-16): hostname-style sid (svr11221 jaisa) pehle
	// "" return karta tha → holder-alive probe dead → claims orphan-sweep
	// → double-connect war. Ab heartbeat me published URL se resolve karo.
	return fleetURLFromHeartbeat(sid)
}

// fleetHTTPClient: SHARED keep-alive client (bandwidth fix). Pehle har
// fleetProbeURL / fleetRemoteHealth call naya http.Client banata tha ->
// har probe pe naya TCP+TLS handshake. Ab connections reuse hote hain.
var fleetHTTPClient = &http.Client{
	Timeout: fleetHTTPTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	},
}

// fleetProbeURL does one quick GET {url} with fleetHTTPTimeout. true =
// /health ne 2xx/3xx diya — server process waqai zinda hai. 4xx/5xx
// (Render error page, tunnel 502 Bad Gateway, 503 wake-fail) = ORIGIN
// DOWN = dead — pehle ye bhi "alive" gin jata tha is liye crashed server
// ka failover kabhi trigger nahi hota tha. Network error/DNS fail = dead.
// ── SHARED PROBE CACHE (BANDWIDTH JUGAD, 0% behavior change) ──
// fleetRemoteHealth + fleetProbeURL dono ek hi URL ko baar-baar probe
// karte the (fleetScanAll, fleetHolderAlive, guards). Ab ek shared 60s
// TTL cache: ek window me ek hi HTTP hit per URL. Data bilkul wahi
// (real /health), bas <=60s purana — liveness decisions (jo 2min
// staleness already tolerate karte hain) pe 0% asar.
type fleetProbeCacheEntry struct {
	ok bool
	ts time.Time
}

var (
	fleetProbeCacheMu sync.Mutex
	fleetProbeCache   = map[string]fleetProbeCacheEntry{}
)

const fleetProbeCacheTTL = 60 * time.Second

func fleetProbeCached(url string) (bool, bool) {
	fleetProbeCacheMu.Lock()
	defer fleetProbeCacheMu.Unlock()
	if e, hit := fleetProbeCache[url]; hit && time.Since(e.ts) < fleetProbeCacheTTL {
		return e.ok, true
	}
	return false, false
}

func fleetProbeStore(url string, ok bool) {
	fleetProbeCacheMu.Lock()
	fleetProbeCache[url] = fleetProbeCacheEntry{ok: ok, ts: time.Now()}
	fleetProbeCacheMu.Unlock()
}

func fleetProbeURL(raw string) bool {
	if ok, hit := fleetProbeCached(raw); hit {
		return ok
	}
	resp, err := fleetHTTPClient.Get(raw)
	if err != nil {
		fleetProbeStore(raw, false)
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	fleetProbeStore(raw, ok)
	return ok
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

// fleetActive: fleet watchdog/claims live hain? (fleetMgr set + Redis hai).
// StartSession cap sirf tab enforce hota hai jab fleet live hai — warna
// single-server (fleet off) mode me normal behaviour. fleet.go ke baaki
// readers ki tarah direct read (fleetMgr fleetBind pe set hota hai, boot
// race me koi torn read nahi — worst case false = cap off = safe side).
func fleetActive() bool {
	return fleetMgr != nil && fleetMgr.Redis != nil
}

func fleetClaimAvailable() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	max := maxPairedSessions()
	if m.SlotsUsed() >= max {
		return // server full (live + in-flight) — agla server uthayega
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
		if m.IsShuttingDown() || m.SlotsUsed() >= max {
			return
		}
		if m.AlreadyConnected(fleetUserPart(jid)) {
			continue
		}
		fleetFailedMu.Lock()
		_, recentlyFailed := fleetFailedAt[jid]
		fleetFailedMu.Unlock()
		if recentlyFailed {
			// WHATSAPP TRUTH (owner order): connect attempt fail hua aur
			// abhi cooldown chal raha hai. Agar StartSession ne "whatsapp
			// logged out" error diya tha to blob pehle hi purge ho chuka
			// (JID set se nikal chuka) — skip hota rahega. Sirf transient
			// network fail wale retry karenge (cooldown khatam hone pe).
			continue
		}

		holders := fleetClaimHolders(jid)
		if len(holders) > 0 {
			// koi live holder hai? (probe-based — 2min+ stale wale ko
			// /health se confirm karo; dead holder ke stale ts se race
			// tie-break me hum hi jeetenge, HDEL ka intezar nahi)
			hb := fleetHeartbeatMap()
			taken := false
			for sid, ts := range holders {
				if sid == fleetSelfID {
					// SELF-CLAIM HEAL: claim humare naam hai magar local
					// device nahi (ephemeral redeploy / disk wipe + per-sid
					// blob missing) → stale self-claim. Ise taken NAHI
					// mano — neeche se GLOBAL fleet blob se restore +
					// connect ho jayega. (Device local hai to AutoLoad/boot
					// sambhal lega — tab taken hi hai.)
					if fleetDeviceExists(jid) {
						taken = true
						break
					}
					continue
				}
				if fleetHolderAlive(sid, hb) {
					// LIVE holder — magar ZOMBIE-CLAIM check bhi (owner fix
					// 2347067958986 wala case): server zinda magar ye session
					// uspe chal nahi raha → purana claim → takeover ALLOWED.
					// Fresh-claim trust window (fleetClaimFreshTrust) race-safe.
					if fleetHolderRunsSession(sid, ts, jid) {
						taken = true
						break
					}
					// zombie claim — ONLINE-ELSEWHERE guard neeche dobara
					// verify karega, phir takeover chalega.
				}
			}
			if taken {
				continue
			}
		}

		// ONLINE-ELSEWHERE GUARD (owner order): claim free lag raha hai
		// magar session kisi AUR server pe already ONLINE hai (boot-restore
		// / manual pair hua tha, claim missing) → koi live holder nahi to
		// bhi isko claim/reconnect MAT karo — jo server chala raha hai wahi
		// chalane do. Sirf OFFLINE (magar logged-in) session hi claim hoga.
		if fleetSessionOnlineElsewhere(jid) {
			InfoLog("FLEET: skip %s — session already ONLINE on another server (no claim, no reconnect)", jid)
			continue
		}

		InfoLog("FLEET: claiming unowned session %s (slots %d/%d)", jid, m.SlotsUsed(), max)
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

	// FULL-TO-FULL JSON DEBUG (owner order): claim flow ka poora state.
	JSONDebug("FLEET_CLAIM_START", map[string]any{
		"jid": jid, "self": fleetSelfID, "slots": m.SlotsUsed(), "max": maxPairedSessions(),
		"deviceLocal": fleetDeviceExists(jid),
	})

	// 0. FAILOVER detection — orphanSweep ne dead-holder claims release
	// kiye the (isliye holders ab empty lagte hain) + failover MARKER
	// save kiya tha (goldmd:fleet:fail:<jid> = dead sid). marker se dead
	// server uthalo — restore success pe owner ko notify karenge.
	var takeoverFrom string
	if mk, ok := m.Redis.getStringKV(fleetFailMarkPrefix + jid); ok && mk != "" && mk != fleetSelfID {
		takeoverFrom = mk
	}

	// 1. claim stamp daalo.
	_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID, now)
	JSONDebug("FLEET_CLAIM_STAMP", map[string]any{"jid": jid, "self": fleetSelfID, "ts": now, "takeoverFrom": takeoverFrom})

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
	JSONDebug("FLEET_CLAIM_TIEBREAK", map[string]any{"jid": jid, "self": fleetSelfID, "myTS": myTS, "holders": holders, "won": won})
	if !won {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		return // doosre server ne jeet liya — hum chup
	}

	// 3.5 ONLINE-ELSEWHERE GUARD (owner order): race jeetne ke baad bhi ek
	// baar remote /sessions se confirm karo — agar session kisi AUR server
	// pe already ONLINE nikla to claim release kar ke chup-chaap nikal
	// jao (koi StartSession/reconnect NAHI). Jo server chala raha hai wahi
	// chalane do — double-connect war WhatsApp logout karva sakta hai.
	if fleetSessionOnlineElsewhere(jid) {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		JSONDebug("FLEET_CLAIM_ONLINE_ELSEWHERE", map[string]any{"jid": jid, "self": fleetSelfID, "action": "release-claim-no-reconnect"})
		InfoLog("FLEET: session %s already ONLINE on another server — claim released, no reconnect", jid)
		return
	}

	// 4. local device hai to seedha connect, warna Storj blob se restore.
	if !fleetDeviceExists(jid) {
		if err := fleetRestoreBlob(jid); err != nil {
			_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
			fleetMarkFailed(jid)
			// TRANSIENT vs DEAD: Storj glitch me restore ek baar fail ho
			// sakta hai — cooldown (10 min) baad ek aur try hota hai.
			// Baar-baar fail (3+ attempts) = blob hi corrupt/mara hua hai
			// — WhatsApp truth: is session ko koi server revive nahi kar
			// sakta. Silent purge (config safe) — fleet set se bhi nikal
			// gaya, watchdog kabhi dobara claim nahi karega.
			fleetFailedMu.Lock()
			attempts := fleetRestoreAttempts[jid] + 1
			fleetRestoreAttempts[jid] = attempts
			fleetFailedMu.Unlock()
			if attempts >= 3 {
				WarnLog("FLEET: blob restore failed %dx for %s — dead blob, silent purge (config safe): %v", attempts, jid, err)
				fleetPurgeLoggedOutSession(jid)
			} else {
				WarnLog("FLEET: blob restore failed for %s (attempt %d/3): %v", jid, attempts, err)
			}
			return
		}
		// restore success → attempt counter reset (session wapas zinda hai)
		fleetFailedMu.Lock()
		delete(fleetRestoreAttempts, jid)
		fleetFailedMu.Unlock()
	}

	// 5. connect (registration + AutoLoad-style flow StartSession me hai).
	// WHATSAPP-TRUTH VERIFY WINDOW StartSession ke andar hai — ye call
	// tabhi nil return karta hai jab WhatsApp ne LOGIN confirm kiya ho.
	if err := m.StartSession(jid); err != nil {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		fleetMarkFailed(jid)
		// WHATSAPP TRUTH (owner order): agar WhatsApp ne khud LOGOUT bola
		// (verify-window ne purge kar diya — blob/set/meta sab delete)
		// to ye dead session hai — bas silently nikal jao, dobara claim
		// nahi hoga (JID fleet set me nahi bacha). Sirf TRANSIENT fail
		// (network/full/tombstone timeout) wale retry karenge.
		if strings.Contains(err.Error(), "whatsapp logged out") {
			WarnLog("FLEET: WhatsApp ne %s ko logout kar diya — session data purge ho chuka (config safe), dobara claim nahi hoga", jid)
			return
		}
		WarnLog("FLEET: connect failed for %s: %v", jid, err)
		return
	}
	JSONDebug("FLEET_CLAIM_CONNECTED", map[string]any{"jid": jid, "self": fleetSelfID, "takeoverFrom": takeoverFrom})
	OkLog("FLEET: session %s restored from Storj and connected (server %s)", jid, fleetSelfID)

	// FAILOVER: ye session kisi aur (dead) server ka tha — owner ko batado
	// ki bot dusre server pe wapas online ho gaya hai. Marker ab delete —
	// ek hi baar notify (duplicate messages nahi).
	if takeoverFrom != "" {
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		go fleetNotifyFailover(jid, takeoverFrom)
		// DEAD server ka per-session Storj data delete kar do — naya/
		// replacement server us URL pe banne par purana session wapas
		// restore kar ke double-connect war shuru na kar de.
		go fleetPurgeDeadServerKV(takeoverFrom)
	}
}

// fleetNotifyFailover: failover-complete hone par session ke owner ko
// message bhejta hai. Owner Redis setting "owner" se milta hai (pairing
// time pe set hota hai). Message session ke khud ke client se jata hai —
// iska matlab bot ka number khud apne owner ko bata raha hai: "main ab
// dusre server pe online hoon, reconnect ho gaya".
func fleetNotifyFailover(jid string, deadServer string) {
	defer func() { _ = recover() }()
	JSONDebug("FAILOVER_NOTIFY_ENTER", map[string]any{"jid": jid, "deadServer": deadServer, "self": fleetSelfID})
	m := fleetMgr
	if m == nil {
		JSONDebug("FAILOVER_NOTIFY_SKIP", map[string]any{"jid": jid, "reason": "no manager"})
		return
	}
	// client settle hone do (connect ke turant baad message queues full
	// ho sakte hain) — 10s baad bhejo.
	time.Sleep(10 * time.Second)

	// session ki owner JID nikaalo (Redis setting).
	owner := fleetOwnerFor(jid)
	if owner == "" {
		InfoLog("FLEET: failover notify skip %s — owner unknown", jid)
		return
	}
	ownerJID, err := types.ParseJID(normalizeJID(owner))
	if err != nil || ownerJID.IsEmpty() {
		InfoLog("FLEET: failover notify skip %s — owner JID parse failed", jid)
		return
	}

	// jis session ke liye notify karna hai wo ab is server pe live hai —
	// uska client use karo (bot apne owner ko khud batayega).
	// NOTE: m.Get() nahi hai — m.List() se user-part match.
	var sess *Session
	for _, s := range m.List() {
		if fleetUserPart(s.JID) == fleetUserPart(jid) {
			sess = s
			break
		}
	}
	if sess == nil || sess.Client == nil || !sess.Client.IsConnected() {
		InfoLog("FLEET: failover notify skip %s — session not live here", jid)
		return
	}
	// ── OWNER ORDER (naya format): server NUMBERS ke saath — jis server
	//    pe user ne pair kiya tha (ab band) aur jis pe ab online hai.
	//    fleetServerNumberForSID: sid/URL -> servers.json number ("3").
	srvNum := fleetServerNumberForSID(fleetSelfID)
	deadNum := fleetServerNumberForSID(deadServer)
	text := "*\U0001F530 GOLD-MD SERVER ERROR \U0001F530*\n\n" +
		"*YOU PAIRED YOUR BOT ON THIS SERVER \u276e " + deadNum + " \u276f BUT THIS SERVER HAS BEEN SHUT DOWN*\n\n" +
		"*NOW YOUR BOT IS RUNNING ON THIS SERVER \u276e " + srvNum + " \u276f*\n\n" +
		"*DON'T WORRY ABOUT THIS ISSUE. YOUR BOT IS ONLINE AGAIN AND WORKING FINE. NO NEED TO PAIR AGAIN \u2705*"

	_, err = sess.Client.SendMessage(context.Background(), ownerJID, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: &text,
		},
	})
	if err != nil {
		WarnLog("FLEET: failover notify send failed for %s: %v", jid, err)
		JSONDebugErr("FAILOVER_NOTIFY_SEND_FAIL", err, map[string]any{"jid": jid, "owner": ownerJID.String()})
	} else {
		OkLog("FLEET: failover notification sent to owner of %s (from server %s → %s)", jid, deadNum, srvNum)
		JSONDebug("FAILOVER_NOTIFY_SENT", map[string]any{"jid": jid, "owner": ownerJID.String(), "deadNum": deadNum, "srvNum": srvNum, "self": fleetSelfID})
	}
}

// fleetPurgeDeadServerKV: failover complete hone ke baad DEAD server ke
// per-session KV footprints delete kar deta hai — zombie-return ya
// replacement server (same URL / naya server) boot hone pe purana session
// wapas restore hone aur double-connect war (WhatsApp stream-replace
// logout) se bachne ke liye. Ye delete hota hai:
//   - goldmd:sessiondb:<dead>:blob  (us server ka whole-DB backup)
//   - goldmd:sessiondb:<dead>:jids  (us server ka JID registry)
//   - goldmd:fleet:servers hash se uska heartbeat entry
//
// GLOBAL fleet blob (goldmd:fleet:sess:<jid>) KABHI delete nahi hota —
// wahi future failovers ka single source of truth hai (jis server ne
// takeover kiya wo use refresh karta rehta hai).
func fleetPurgeDeadServerKV(deadSid string) {
	if deadSid == "" {
		return
	}
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	_, _ = m.Redis.cmd("DEL", sessionDBKeyConst+deadSid+sessionDBKeySuffix)
	_, _ = m.Redis.cmd("DEL", sessionJidsKeyConst+deadSid+sessionJidsKeySuffix)
	_, _ = m.Redis.cmd("HDEL", fleetServersHash, deadSid)
	InfoLog("FLEET: purged dead server %q session data (sessiondb blob+jids+heartbeat)", deadSid)
}

func fleetMarkFailed(jid string) {
	fleetFailedMu.Lock()
	fleetFailedAt[jid] = time.Now().Unix()
	fleetFailedMu.Unlock()
}

// fleetRestoreAttempts: per-JID blob-restore fail counter (3+ fail = dead
// blob → silent purge). fleetFailedMu guard karta hai.
var fleetRestoreAttempts = map[string]int{}

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
//
// BANDWIDTH FIX (owner order — Render 5GB): whatsmeow_message_secrets
// blob se NIKALA GAYA. Ek purane session ka ye table 212,983 rows tak
// badh chuka tha → per-JID blob 44.8MB → 10-min refresh 6GB+/day egress.
// Connection ke liye ye table zaroori NAHI (sirf purane msgs re-decrypt
// ke liye). Naye messages bilkul theek decrypt hote hain. event_buffer /
// retry_buffer (transient retry queues) bhi hata diye — whatsmeow inhe
// khud rebuild kar leta hai.
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
	{"whatsmeow_privacy_tokens", "our_jid"},
	{"whatsmeow_nct_salt", "our_jid"},
	{"whatsmeow_lid_map", "jid"},
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

// fleetRestoreBlobFromStorj: StartSession/AutoLoad ka public wrapper —
// device local disk pe nahi mila to Storj fleet blob se restore karke
// true return. Restore fail / blob missing → false (caller purana path).
func fleetRestoreBlobFromStorj(jid string) bool {
	return fleetRestoreBlob(jid) == nil
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

// BANDWIDTH JUGAD (fleet blob hash-skip): per-JID last-uploaded blob
// sha256. Unchanged blob → blob PUT + meta PUT + SADD teeno skip —
// idle connected sessions ka 10-min refresh ab zero egress.
var (
	fleetBlobHashMu sync.Mutex
	fleetBlobHashes = map[string]string{} // jid → hex sha256 of tmp db bytes
)

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
		// BANDWIDTH JUGAD: unchanged blob → teeno Storj writes skip
		// (blob + meta + SADD). PUT body gzip transparent hota hai
		// (kvSet GZ1:) — changed blob ~8-15x chhota jata hai.
		sum := sha256.Sum256(data)
		hexSum := hex.EncodeToString(sum[:])
		fleetBlobHashMu.Lock()
		if fleetBlobHashes[jid] == hexSum {
			fleetBlobHashMu.Unlock()
			return // idle session — kuch bhi upload nahi
		}
		fleetBlobHashMu.Unlock()
		enc := base64.StdEncoding.EncodeToString(data)
		if err := m.Redis.setString(fleetBlobPrefix+jid, enc); err != nil {
			WarnLog("FLEET: blob upload failed for %s: %v", jid, err)
			return
		}
		fleetBlobHashMu.Lock()
		fleetBlobHashes[jid] = hexSum
		fleetBlobHashMu.Unlock()
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
			// DISK-ONLY (owner order): direct /code?phone= session ka blob
			// Storj pe KABHI push nahi hota — 10-min refresher bhi skip.
			if s.LocalOnly {
				continue
			}
			fleetSaveBlob(s.JID)
			// CLAIM RE-STAMP (owner fix — zombie-claim ka ROOT source):
			// claim connect pe EK baar likha jata tha, phir hamesha stale.
			// Har 10-min refresh pe claim fresh → doosre servers
			// fresh-trust window me asli owner ko bina probe respect
			// karte hain → /sessions probe traffic near-zero + stale
			// claims ka source hi khatam. (HSET chhota hai — bandwidth
			// cost negligible, blob upload to hash-skip ho hi jata hai.)
			_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+s.JID, fleetSelfID,
				strconv.FormatInt(time.Now().Unix(), 10))
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
// owner pairing panel se aata hai — Redis "owner" setting me save hota hai
// (failover notification isi JID pe jata hai; fleetOwnerFor padhta hai).
func fleetOnPairSuccess(jid string, owner string) {
	JSONDebug("FLEET_PAIR_SUCCESS", map[string]any{"jid": jid, "owner": owner, "self": fleetSelfID})
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
		// owner setting — sirf tab jab pehli baar set ho raha ho (kisi
		// ne .setowner style change kiya ho to usko overwrite mat karo).
		if owner != "" {
			if existing := m.Redis.GetSetting(jid, "owner", ""); existing == "" {
				m.Redis.SetSetting(jid, "owner", normalizeJID(owner))
			}
		}
	}()
}

// fleetOnConnected: session live hua → claim refresh (hum iske owner hain).
func fleetOnConnected(jid string) {
	JSONDebug("FLEET_ON_CONNECTED", map[string]any{"jid": jid, "self": fleetSelfID})
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		// BOOT-RESTORE PATH: AutoLoad ne (per-sid blob se) seedha connect
		// kar diya — fleet claim path se nahi aya. Stale dead-holder claim
		// pada ho to usko HDEL karo, apna fresh claim daalo, aur agar
		// failover marker pada hai to PURGE + NOTIFY bhi karo — warna
		// ye session bina claim ke chalta rehta hai aur takeover flow
		// (dead server ka data delete + owner ko msg) kabhi nahi chalta.
		holders := fleetClaimHolders(jid)
		live := false
		for sid, ts := range holders {
			if sid == fleetSelfID {
				continue
			}
			if fleetHolderAlive(sid, fleetHeartbeatMap()) && fleetHolderRunsSession(sid, ts, jid) {
				// live holder + session uske paas WAQAI hai (zombie claim
				// nahi) — usi ko chalane do, hum chup (war-safe).
				live = true
				break
			}
		}
		// ONLINE-ELSEWHERE GUARD (owner order): koi AUR live server is JID
		// ka claim rakh raha hai to uske claims/failover-marker ko HAATH
		// mat lagao — wo server hi session ka owner hai. Hume apna claim
		// bhi daalne ki zaroorat nahi (dono side se double-claim war se
		// bacha). Local session zinda hai to chalega; us server ke marne
		// pe orphan sweep + claim flow sab sambhal lega.
		if live {
			fleetSaveBlob(jid) // connected keys latest rakho
			return
		}
		if !live {
			if len(holders) > 0 {
				// stale dead-holder claims release karo (boot ke waqt
				// sweep abhi nahi chula hoga)
				for sid := range holders {
					_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
				}
			}
			_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID,
				strconv.FormatInt(time.Now().Unix(), 10))
			// FAILOVER COMPLETE: marker pada hai (is jid ka purana holder
			// dead tha) → purge + owner-notify ek hi baar.
			if mk, ok := m.Redis.getStringKV(fleetFailMarkPrefix + jid); ok && mk != "" {
				if mk == fleetSelfID {
					// ZOMBIE-RETURN HOME: marker humare hi purane crash ka hai
					// aur session wapas hum hi pe aa gaya — koi failover nahi
					// hua, marker silently hata do (notify/purge nahi).
					_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
				} else {
					_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
					go fleetNotifyFailover(jid, mk)
					go fleetPurgeDeadServerKV(mk)
				}
			}
		}
		fleetSaveBlob(jid) // connected keys latest rakho
	}()
}

// fleetOnCleanup: session locally hata gaya (logout) → blob + registry del.
// Ye session ab fleet me nahi aayega (WhatsApp ne khud logout kiya).
func fleetOnCleanup(jid string) {
	go fleetPurgeLoggedOutSession(jid)
}

// fleetPurgeLoggedOutSession (OWNER ORDER — WHATSAPP TRUTH):
// WhatsApp ne is JID ke liye LOGOUT bola hai (chahe session jis bhi URL /
// server se aaya ho). Silent purge:
//   - fleet blob (goldmd:fleet:sess:<jid>)     — DELETE (koi b server dobara
//     claim karke wapas na laaye)
//   - fleet meta + sessions-set + claim + fail-mark — DELETE
//   - own-server jids registry (goldmd:sessiondb:<sid>:jids) — REMOVE
//   - own-server blob — refresh (remaining devices) ya DELETE (empty)
//
// CONFIGURATION (settings:<jid> — owner, prefix, sudo, autoreact, welcome
// sab) KABHI delete NAHI hoti — re-pair karne par purani settings wapas
// mil jati hain. Ye SILENT hai: koi error loop nahi, koi retry nahi —
// WhatsApp ka faisla final hai.
func fleetPurgeLoggedOutSession(jid string) {
	defer func() { _ = recover() }()
	m := fleetMgr
	if m == nil || m.Redis == nil || jid == "" {
		return
	}
	// 1. fleet-level keys (GLOBAL — har server inhi se session uthata hai)
	_, _ = m.Redis.cmd("DEL", fleetClaimPrefix+jid)
	_ = m.Redis.setRem(fleetSessionsSet, jid)
	_ = m.Redis.setDel(fleetBlobPrefix + jid)
	_ = m.Redis.setDel(fleetMetaPrefix + jid)
	_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
	// failed-cooldown bhi hata do — JID set me hi nahi, cooldown bekaar
	fleetFailedMu.Lock()
	delete(fleetFailedAt, jid)
	fleetFailedMu.Unlock()

	// 2. own-server jids registry — AutoLoad isi se restore karta hai
	_ = m.Redis.RemoveJID(jid)

	// 3. own-server blob refresh: device-row SQLite se pehle hi delete ho
	// chuka hai (cleanupSession step 3). Blob me wo device nahi bacha —
	// lekin agar koi aur live device hai to blob REFRESH karo (delete
	// nahi), warna empty blob DELETE (stale restore se bachav).
	if m.container != nil {
		remaining, rerr := m.container.GetAllDevices(context.Background())
		if rerr == nil && len(remaining) > 0 {
			_ = m.Redis.SaveSessionDB(fleetDBPath)
		} else {
			_ = m.Redis.DelSessionDB()
		}
	}
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

	// throttle state — kitna/pichhla kab Storj pe persist hua
	fleetEgressPersisted      int64     // total jab last SET chala
	fleetEgressPersistedAt    time.Time // last SET ka time
	fleetEgressPersistedMonth string    // last SET ka month

	// pre-crash warning state — ek hi baar per month per process (render
	// bandwidth khatam hone se PEHLE owners ko batado).
	fleetWarnMu      sync.Mutex
	fleetWarnedMonth string // "2025-01" — jis month warning chala
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

	// BANDWIDTH JUGAD (egress-throttle): /proc delta hamesha memory me
	// jama hota hai (.host5gb exact hi rehta hai — wo memory padhta hai).
	// Storj pe sirf tab write jab (a) 256KB+ naya egress jama hua ho YA
	// (b) pichhle write se 5+ min ho gaye hon — 2880 tiny PUT/day ab
	// ~100-300 ho jayenge. Month rollover hamesha write (reset persist).
	fleetEgressMu.Lock()
	persist := (total-fleetEgressPersisted) >= 256*1024 ||
		now.Sub(fleetEgressPersistedAt) >= 5*time.Minute ||
		month != fleetEgressPersistedMonth
	if persist {
		fleetEgressPersisted = total
		fleetEgressPersistedAt = now
		fleetEgressPersistedMonth = month
	}
	fleetEgressMu.Unlock()
	if !persist {
		return
	}
	val := fmt.Sprintf("%d|%d|%d|%s", total, lastTx, now.Unix(), month)
	_, _ = m.Redis.cmd("SET", fleetEgressPrefix+fleetSelfID, val)
}

// fleetWarnPreCrash: RENDER PRE-CRASH WARNING (owner order) — jab is server
// ka egress ~90% of 5GB budget cross ho jaye (Render bandwidth khatam hone
// wala hai), is server ke LOCAL connected session owners ko PEHLE bata do:
//
//	*GOLD-MD SERVER ❮ N ❯ STOPPING*
//
//	*YOUR BOT IS MOVING TO ANOTHER SERVER NO NEED TO PAIR AGAIN YOUR BOT
//	COME BACK ONLINE IN 2/3 MINTS ONLY PLEASE WAIT.....*
//
// Ek hi baar per calendar month per process — spam nahi. Warning ke baad
// egress natural continue hota hai (sirf $100GB hard-stop tak alag).
const fleetWarnThreshold = 0.90 // 90% of budget

func fleetWarnPreCrash() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	used := fleetEgressUsedMB()
	budget := float64(fleetBudgetMB)
	if budget <= 0 || used < budget*fleetWarnThreshold {
		return
	}
	fleetWarnMu.Lock()
	month := time.Now().UTC().Format("2006-01")
	if fleetWarnedMonth == month {
		fleetWarnMu.Unlock()
		return // is month warning already chal chuka
	}
	fleetWarnedMonth = month // abhi mark karo — send fail ho to next boot
	fleetWarnMu.Unlock()

	srvNum := fleetServerNumberForSID(fleetSelfID)
	text := "*GOLD-MD SERVER \u276e " + srvNum + " \u276f STOPPING*\n\n" +
		"*YOUR BOT IS MOVING TO ANOTHER SERVER NO NEED TO PAIR AGAIN YOUR BOT COME BACK ONLINE IN 2/3 MINTS ONLY PLEASE WAIT.....*"

	sent := 0
	for _, sess := range m.List() {
		if sess.Client == nil || !sess.Client.IsConnected() {
			continue
		}
		owner := fleetOwnerFor(sess.JID)
		if owner == "" {
			continue
		}
		ownerJID, err := types.ParseJID(normalizeJID(owner))
		if err != nil || ownerJID.IsEmpty() {
			continue
		}
		if _, err := sess.Client.SendMessage(context.Background(), ownerJID, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: &text},
		}); err == nil {
			sent++
		}
	}
	InfoLog("FLEET: pre-crash warning sent to %d owners (egress %.0fMB / %dMB budget)", sent, used, fleetBudgetMB)
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
	Sess     []struct {
		JID    string `json:"jid"`
		Online bool   `json:"online"`
	} `json:"sess"`
}

// fleetRemoteHealth fetches another server's /health (6s timeout).
// fleetHealthCache: fleetRemoteHealth ka shared 60s TTL cache (BANDWIDTH
// JUGAD, 0% behavior change). fleetScanAll / .server / .render5gb reports
// ek hi URL ko baar-baar probe karte the — ab ek window me ek hi hit.
type fleetHealthCacheEntry struct {
	hf *fleetHealthFields
	ts time.Time
}

var (
	fleetHealthCacheMu sync.Mutex
	fleetHealthCache   = map[string]fleetHealthCacheEntry{}
)

const fleetHealthCacheTTL = 60 * time.Second

func fleetRemoteHealth(url string) (*fleetHealthFields, error) {
	fleetHealthCacheMu.Lock()
	if e, hit := fleetHealthCache[url]; hit && time.Since(e.ts) < fleetHealthCacheTTL {
		hf := e.hf
		fleetHealthCacheMu.Unlock()
		if hf == nil {
			return nil, fmt.Errorf("health cached error")
		}
		return hf, nil
	}
	fleetHealthCacheMu.Unlock()

	healthURL := strings.TrimRight(url, "/") + "/health"
	resp, err := fleetHTTPClient.Get(healthURL)
	if err != nil {
		fleetHealthCacheMu.Lock()
		fleetHealthCache[url] = fleetHealthCacheEntry{hf: nil, ts: time.Now()}
		fleetHealthCacheMu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fleetHealthCacheMu.Lock()
		fleetHealthCache[url] = fleetHealthCacheEntry{hf: nil, ts: time.Now()}
		fleetHealthCacheMu.Unlock()
		return nil, fmt.Errorf("health status %d", resp.StatusCode)
	}
	var hf fleetHealthFields
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&hf); err != nil {
		fleetHealthCacheMu.Lock()
		fleetHealthCache[url] = fleetHealthCacheEntry{hf: nil, ts: time.Now()}
		fleetHealthCacheMu.Unlock()
		return nil, err
	}
	fleetHealthCacheMu.Lock()
	fleetHealthCache[url] = fleetHealthCacheEntry{hf: &hf, ts: time.Now()}
	fleetHealthCacheMu.Unlock()
	return &hf, nil
}

// fleetServerInfo: one row of the .server / .render5gb report.
type fleetServerInfo struct {
	Name     string
	URL      string
	Online   bool
	Sessions int
	Max      int
	RE       string
	SID      string
	UsedMB   float64
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
				Name: s.Name,
				URL:  s.URL,
				Max:  cfg.MaxPerServer,
				RE:   "STOPPED", // offline = engine stopped
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
	// OWNER FIX A (2026-09-16): per-session online array — remote servers
	// ek hi /health request me liveness + session-state dono dekh lete
	// hain (purane readers unknown field ignore karte hain — safe).
	sessJSON := "[]"
	if m := fleetMgr; m != nil {
		type row struct {
			JID    string `json:"jid"`
			Online bool   `json:"online"`
		}
		rows := make([]row, 0, 8)
		for _, s := range m.List() {
			online := s.Paired && s.Client != nil && s.Client.IsConnected() &&
				s.Client.Store != nil && s.Client.Store.ID != nil
			rows = append(rows, row{JID: s.JID, Online: online})
		}
		if b, err := json.Marshal(rows); err == nil {
			sessJSON = string(b)
		}
	}
	fmt.Fprintf(w, `{"bot":"GOLD-MD","status":"online","sessions":%d,"max":%d,"re":"%s","sid":"%s","used_mb":%.1f,"sess":%s}`,
		sessions, maxPairedSessions(), fleetREStatus(), fleetSelfID, fleetEgressUsedMB(), sessJSON)
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
