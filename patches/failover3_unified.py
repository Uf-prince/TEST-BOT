#!/usr/bin/env python3
"""Patch 3: Unified probe-based liveness + zombie-return guard.

Problems fixed:
1. Patch 1 flaw: orphanSweep ka `anyAlive` pre-check (5min alive-map)
   2-5min stale holders ko probe hone se pehle hi skip kar deta tha.
   Ab fleetHolderAlive() single source: fresh(<2min)=alive, stale(>=2min)
   = ACTIVE /health probe, no-heartbeat=dead.
2. fleetClaimAvailable ka alive-check bhi probe-based (failover claim
   orphanSweep ka HDEL wait nahi karta — race-tiebreak khud handle).
3. ZOMBIE-RETURN GUARD: dead server wapas aaye to AutoLoad uski
   failover ho chuki sessions ko dobara start na kare (double-connect
   war = WhatsApp logout). AutoLoad ab check karta hai: koi AUR live
   server is JID ka fresh claim hold karta hai to skip.
4. fleetBind(): fleetMgr AutoLoad se PEHLE set ho sake (watchdog baad
   me, ordering race-free).
"""
path = "fleet.go"
src = open(path, encoding="utf-8").read()

# ── A) watchdog: orphanSweep call (alive param hata rahe hain) ─────────
old = """\t\tif tick%2 == 0 {
\t\t\t// orphan sweep + claim \u2014 60s cadence (light on Storj reads).
\t\t\talive := fleetAliveServers()
\t\t\tfleetOrphanSweep(alive)
\t\t\tif !fleetMgr.IsShuttingDown() {
\t\t\t\tfleetClaimAvailable()
\t\t\t}
\t\t}"""
new = """\t\tif tick%2 == 0 {
\t\t\t// orphan sweep + claim \u2014 60s cadence (light on Storj reads).
\t\t\t// NOTE: fleetAliveServers ka 24h-purge side-effect chahiye hi \u2014
\t\t\t// heartbeatMap me bhi hai. alive-set ab per-holder probe se aata hai.
\t\t\tfleetOrphanSweep()
\t\t\tif !fleetMgr.IsShuttingDown() {
\t\t\t\tfleetClaimAvailable()
\t\t\t}
\t\t}"""
assert src.count(old) == 1, "A: watchdog anchor not unique"
src = src.replace(old, new)

# ── B) orphanSweep: rewrite — index-based (tab typos immune) ────
start = src.find("// fleetOrphanSweep releases claims held by dead servers")
end_marker = 'probe-confirmed)", jid)\n\t}\n}'
end = src.find(end_marker)
assert start > 0 and end > start, "B: orphanSweep anchors not found"
end += len(end_marker)
new_b_src = """// fleetOrphanSweep releases claims held by dead servers so their sessions
// can be picked up by live servers (Render spin-down / crash scenario).
//
// LIVENESS (fleetHolderAlive — unified):
//   heartbeat fresh (<2min)  → ALIVE (koi probe nahi — tick chal raha hai)
//   heartbeat 2min+ stale    → ACTIVE /health probe (4s timeout)
//                              probe OK → ALIVE (busy server, rehne do)
//                              probe FAIL → DEAD → claim release
//   heartbeat hi nahi        → DEAD (purani entry)
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
		// sab holders dead — claims release kar do.
		for sid := range holders {
			_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
		}
		InfoLog("FLEET: orphan claim released for %s (all holders dead, probe-confirmed)", jid)
	}
}"""
src = src[:start] + new_b_src + src[end:]

# ── C) fleetProbeServer → fleetHolderAlive rewrite ─────────────────────
old = """// fleetProbeServer actively checks a claim holder's /health endpoint to
// confirm it is REALLY dead (heartbeat stale + probe fail = dead). Stale
// but fresh-ish (<2min) heartbeats ko probe nahi karte \u2014 shayad bas tick
// busy ho. Probe timeout fleetHTTPTimeout (4s) hai. Result cache nahi \u2014
// har sweep me fresh (sirf watchdog goroutine, no hot-path cost).
func fleetProbeServer(sid string, heartbeats map[string]int64) bool {
\tif sid == "" {
\t\treturn false
\t}
\t// khud ki heartbeat \u2014 hum zinda hain (fleetAliveServers ne include kiya).
\tif sid == fleetSelfID {
\t\treturn true
\t}
\tts, ok := heartbeats[sid]
\tif !ok || ts <= 0 {
\t\t// heartbeat hi nahi \u2014 purani claim entry. fleetOrphanAfter (5min)
\t\t// ke through hi dead man lo \u2014 flapping se bacho.
\t\treturn false
\t}
\tnow := time.Now().Unix()
\tage := now - ts
\tif age < int64(fleetProbeAfter/time.Second) {
\t\t// heartbeat fresh hai (<2min) \u2014 server tick de raha hai, zinda hai.
\t\treturn true
\t}
\t// heartbeat 2min+ stale \u2014 ab ACTIVE /health probe (1 attempt).
\turl := fleetServerURL(sid)
\tif url == "" {
\t\treturn false // URL resolve nahi hua \u2014 dead man lo
\t}
\tok2 := fleetProbeURL(url + "/health")
\tif !ok2 {
\t\tInfoLog("FLEET: active probe FAILED for %s (hb %ds stale) \u2014 treating dead", sid, age)
\t}
\treturn ok2
}"""
new = """// fleetHolderAlive: ek claim holder REAL me zinda hai ya nahi \u2014 unified
// decision (orphanSweep + claimAvailable + zombie-guard sab yahi use
// karte hain). Heartbeat fresh = zinda. Stale = ACTIVE /health probe.
// Sirf watchdog/background paths se call hota hai \u2014 hot-path cost 0.
func fleetHolderAlive(sid string, heartbeats map[string]int64) bool {
\tif sid == "" {
\t\treturn false
\t}
\t// hum khud \u2014 obviously zinda (yeh code chal raha hai).
\tif sid == fleetSelfID {
\t\treturn true
\t}
\tts, ok := heartbeats[sid]
\tif !ok || ts <= 0 {
\t\t// heartbeat hi nahi \u2014 purani claim entry \u2192 dead.
\t\treturn false
\t}
\tage := time.Now().Unix() - ts
\tif age < int64(fleetProbeAfter/time.Second) {
\t\t// heartbeat fresh (<2min) \u2014 watchdog tick de raha hai \u2192 zinda.
\t\treturn true
\t}
\t// heartbeat 2min+ stale \u2014 ab ACTIVE /health probe (1 attempt, 4s).
\turl := fleetServerURL(sid)
\tif url == "" {
\t\treturn false // URL resolve nahi hua \u2192 dead man lo
\t}
\tif fleetProbeURL(url + "/health") {
\t\treturn true // busy tha lekin process zinda hai
\t}
\tInfoLog("FLEET: active probe FAILED for %s (hb %ds stale) \u2014 treating dead", sid, age)
\treturn false
}

// fleetHeldByLiveServer: kya ye JID kisi AUR live server ke fresh claim
// me hai? (ZOMBIE-RETURN GUARD: dead server wapas aaya to AutoLoad isko
// dobara start na kare \u2014 dusre server pe already failover ho chuka hai.
// Double-connect war = WhatsApp stream-replace = logout. Ye guard war
// rokta hai.) AutoLoad ke batch goroutines se call hota hai.
func fleetHeldByLiveServer(jid string) bool {
\tm := fleetMgr
\tif m == nil || m.Redis == nil || !storjReadyFlag() {
\t\treturn false // fleet inactive \u2014 sab local load karo
\t}
\tholders := fleetClaimHolders(jid)
\tif len(holders) == 0 {
\t\treturn false
\t}
\thb := fleetHeartbeatMap()
\tfor sid := range holders {
\t\tif sid != fleetSelfID && fleetHolderAlive(sid, hb) {
\t\t\treturn true // koi aur live server isko chala raha hai
\t\t}
\t}
\treturn false
}"""
assert src.count(old) == 1, "C: probeServer anchor not unique"
src = src.replace(old, new)

# ── D) claimAvailable: probe-based holder check ────────────────────────
old = """\t\tholders := fleetClaimHolders(jid)
\t\tif len(holders) > 0 {
\t\t\t// koi live holder hai? (dead holders ko orphan sweep hata dega)
\t\t\talive := fleetAliveServers()
\t\t\ttaken := false
\t\t\tfor sid := range holders {
\t\t\t\tif alive[sid] {
\t\t\t\t\ttaken = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
\t\t\tif taken {
\t\t\t\tcontinue
\t\t\t}
\t\t}"""
new = """\t\tholders := fleetClaimHolders(jid)
\t\tif len(holders) > 0 {
\t\t\t// koi live holder hai? (probe-based \u2014 2min+ stale wale ko
\t\t\t// /health se confirm karo; dead holder ke stale ts se race
\t\t\t// tie-break me hum hi jeetenge, HDEL ka intezar nahi)
\t\t\thb := fleetHeartbeatMap()
\t\t\ttaken := false
\t\t\tfor sid := range holders {
\t\t\t\tif fleetHolderAlive(sid, hb) {
\t\t\t\t\ttaken = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
\t\t\tif taken {
\t\t\t\tcontinue
\t\t\t}
\t\t}"""
assert src.count(old) == 1, "D: claimAvailable anchor not unique"
src = src.replace(old, new)

# ── E) fleetBind: AutoLoad se pehle fleetMgr set ho sake ───────────────
old = """func fleetInit(m *Manager, dbPath string) {
\tfleetMgr = m
\tfleetDBPath = dbPath
\tif m == nil || m.Redis == nil || !storjReadyFlag() {"""
new = """// fleetBind: fleetMgr/fleetDBPath set karo BINA watchdog start kiye.
// main.go isko AutoLoad se PEHLE call karta hai taake AutoLoad ka
// zombie-return guard (fleetHeldByLiveServer) kaam kar sake. Watchdog
// fleetInit me baad me start hota hai \u2014 AutoLoad ke concurrent-start
// race se door.
func fleetBind(m *Manager, dbPath string) {
\tif m == nil {
\t\treturn
\t}
\tfleetMgr = m
\tfleetDBPath = dbPath
}

func fleetInit(m *Manager, dbPath string) {
\tfleetBind(m, dbPath)
\tif m == nil || m.Redis == nil || !storjReadyFlag() {"""
assert src.count(old) == 1, "E: fleetInit anchor not unique"
src = src.replace(old, new)

open(path, "w", encoding="utf-8").write(src)
print("PATCH 3 APPLIED OK (fleet.go)")

# ── F) manager.go: AutoLoad zombie-guard ───────────────────────────────
path2 = "manager.go"
src2 = open(path2, encoding="utf-8").read()

old = """\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)
\t\t\tif err := m.StartSession(u); err != nil {"""
new = """\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)
\t\t\t// ZOMBIE-RETURN GUARD: ye session kisi AUR live server ke fresh
\t\t\t// claim me hai (failover ho chuka) to yahan start mat karo \u2014
\t\t\t// double-connect war WhatsApp logout karva deta hai. Fleet
\t\t\t// watchdog us server ke marne pe ye session wapas le lega.
\t\t\tif fleetHeldByLiveServer(u) {
\t\t\t\tInfoLog("FLEET: skip %s \u2014 live claim on another server (failover target)", u)
\t\t\t\treturn
\t\t\t}
\t\t\tif err := m.StartSession(u); err != nil {"""
assert src2.count(old) == 1, "F: AutoLoad anchor not unique"
src2 = src2.replace(old, new)

open(path2, "w", encoding="utf-8").write(src2)
print("PATCH 3 APPLIED OK (manager.go)")

# ── G) main.go: fleetBind AutoLoad se pehle ────────────────────────────
path3 = "main.go"
src3 = open(path3, encoding="utf-8").read()

old = """\t// \u2500\u2500 auto-load every saved session (batched, like autoload.js) \u2500\u2500
\tmgr.AutoLoad()"""
new = """\t// \u2500\u2500 FLEET BIND (AutoLoad se pehle) \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
\t// fleetMgr set (watchdog NAHI \u2014 wo AutoLoad ke baad fleetInit me).
\t// AutoLoad ka zombie-return guard isi pe depend karta hai.
\tfleetBind(mgr, dbPath)

\t// \u2500\u2500 auto-load every saved session (batched, like autoload.js) \u2500\u2500
\tmgr.AutoLoad()"""
assert src3.count(old) == 1, "G: main.go AutoLoad anchor not unique"
src3 = src3.replace(old, new)

open(path3, "w", encoding="utf-8").write(src3)
print("PATCH 3 APPLIED OK (main.go)")
