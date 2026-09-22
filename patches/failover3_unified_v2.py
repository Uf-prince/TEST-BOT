#!/usr/bin/env python3
"""Patch 3 (v2, fully index-based): Unified probe-based liveness + zombie-return guard.

A) watchdog: fleetOrphanSweep() no-arg
B) orphanSweep: unified fleetHolderAlive liveness
C) fleetProbeServer -> fleetHolderAlive + fleetHeldByLiveServer (zombie guard)
D) claimAvailable: probe-based holder check
E) fleetBind: AutoLoad se pehle fleetMgr set
F) manager.go AutoLoad: zombie-guard skip
G) main.go: fleetBind before AutoLoad
"""
path = "fleet.go"
src = open(path, encoding="utf-8").read()

def cut(start_marker, end_marker, label, include_end=True):
    global src
    s = src.find(start_marker)
    assert s > 0, label + ": start marker not found: " + start_marker[:40]
    e = src.find(end_marker, s)
    assert e > s, label + ": end marker not found after start"
    if include_end:
        e += len(end_marker)
    return s, e

# ── A) watchdog: orphanSweep call (alive param removed) ───────────────
s = src.find("if tick%2 == 0 {")
assert s > 0, "A: watchdog tick anchor not found"
e = src.find("tick++", s)
assert e > s, "A: tick++ not found"
e += len("tick++")
new = """\t\tif tick%2 == 0 {
\t\t\t// orphan sweep + claim \u2014 60s cadence (light on Storj reads).
\t\t\t// alive-set ab per-holder probe se aata hai (fleetHolderAlive).
\t\t\tfleetOrphanSweep()
\t\t\tif !fleetMgr.IsShuttingDown() {
\t\t\t\tfleetClaimAvailable()
\t\t\t}
\t\t}"""
src = src[:s] + new + src[e:]
print("A ok")

# ── B) orphanSweep: unified rewrite ────────────────────────────────────
s, e = cut("// fleetOrphanSweep releases claims held by dead servers",
           'probe-confirmed)", jid)\n\t}\n}', "B", True)
new = """// fleetOrphanSweep releases claims held by dead servers so their sessions
// can be picked up by live servers (Render spin-down / crash scenario).
//
// LIVENESS (fleetHolderAlive \u2014 unified):
//   heartbeat fresh (<2min)  \u2192 ALIVE (koi probe nahi \u2014 tick chal raha hai)
//   heartbeat 2min+ stale    \u2192 ACTIVE /health probe (4s timeout)
//                              probe OK \u2192 ALIVE (busy server, rehne do)
//                              probe FAIL \u2192 DEAD \u2192 claim release
//   heartbeat hi nahi        \u2192 DEAD (purani entry)
// Isse failover 5min wait \u2192 ~2min ho jata hai (owner ka order) \u2014 bina
// kisi alive server ke claim ko galat release kiye.
func fleetOrphanSweep() {
\tif fleetMgr == nil || fleetMgr.Redis == nil {
\t\treturn
\t}
\t// heartbeat source map \u2014 per-holder liveness decision (sid -> last ts).
\theartbeats := fleetHeartbeatMap()

\tfor _, jid := range fleetMgr.Redis.setMembers(fleetSessionsSet) {
\t\tholders := fleetClaimHolders(jid)
\t\tif len(holders) == 0 {
\t\t\tcontinue // unclaimed \u2014 fleetClaimAvailable utha lega
\t\t}
\t\tallDead := true
\t\tfor sid := range holders {
\t\t\tif fleetHolderAlive(sid, heartbeats) {
\t\t\t\tallDead = false
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tif !allDead {
\t\t\tcontinue // koi holder zinda hai \u2014 session usi ke paas
\t\t}
\t\t// sab holders dead \u2014 claims release kar do.
\t\tfor sid := range holders {
\t\t\t_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
\t\t}
\t\tInfoLog("FLEET: orphan claim released for %s (all holders dead, probe-confirmed)", jid)
\t}
}"""
src = src[:s] + new + src[e:]
print("B ok")

# ── C) fleetProbeServer -> fleetHolderAlive + fleetHeldByLiveServer ───
s, e = cut("// fleetProbeServer actively checks",
           "// fleetServerURL resolves a serverID", "C", False)
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
}

"""
src = src[:s] + new + src[e:]
print("C ok")

# ── D) claimAvailable: probe-based holder check ────────────────────────
s, e = cut('\t\tholders := fleetClaimHolders(jid)\n\t\tif len(holders) > 0 {',
           '\t\t\tif taken {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t}', "D", True)
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
src = src[:s] + new + src[e:]
print("D ok")

# ── E) fleetBind ────────────────────────────────────────────────────────
old_init = "func fleetInit(m *Manager, dbPath string) {"
assert src.count(old_init) == 1, "E: fleetInit anchor not unique"
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

func fleetInit(m *Manager, dbPath string) {"""
src = src.replace(old_init, new, 1)
print("E ok")

open(path, "w", encoding="utf-8").write(src)
print("PATCH 3 APPLIED OK (fleet.go)")

# ── F) manager.go: AutoLoad zombie-guard ───────────────────────────────
path2 = "manager.go"
src2 = open(path2, encoding="utf-8").read()

old_f = '\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)\n\t\t\tif err := m.StartSession(u); err != nil {'
assert src2.count(old_f) == 1, "F: AutoLoad anchor not unique"
new_f = """\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)
\t\t\t// ZOMBIE-RETURN GUARD: ye session kisi AUR live server ke fresh
\t\t\t// claim me hai (failover ho chuka) to yahan start mat karo \u2014
\t\t\t// double-connect war WhatsApp logout karva deta hai. Fleet
\t\t\t// watchdog us server ke marne pe ye session wapas le lega.
\t\t\tif fleetHeldByLiveServer(u) {
\t\t\t\tInfoLog("FLEET: skip %s \u2014 live claim on another server (failover target)", u)
\t\t\t\treturn
\t\t\t}
\t\t\tif err := m.StartSession(u); err != nil {"""
src2 = src2.replace(old_f, new_f, 1)
open(path2, "w", encoding="utf-8").write(src2)
print("PATCH 3 APPLIED OK (manager.go)")

# ── G) main.go: fleetBind AutoLoad se pehle ────────────────────────────
path3 = "main.go"
src3 = open(path3, encoding="utf-8").read()

old_g = '\t// \u2500\u2500 auto-load every saved session (batched, like autoload.js) \u2500\u2500\n\tmgr.AutoLoad()'
assert src3.count(old_g) == 1, "G: main.go AutoLoad anchor not unique"
new_g = """\t// \u2500\u2500 FLEET BIND (AutoLoad se pehle) \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
\t// fleetMgr set (watchdog NAHI \u2014 wo AutoLoad ke baad fleetInit me).
\t// AutoLoad ka zombie-return guard isi pe depend karta hai.
\tfleetBind(mgr, dbPath)

\t// \u2500\u2500 auto-load every saved session (batched, like autoload.js) \u2500\u2500
\tmgr.AutoLoad()"""
src3 = src3.replace(old_g, new_g, 1)
open(path3, "w", encoding="utf-8").write(src3)
print("PATCH 3 APPLIED OK (main.go)")
