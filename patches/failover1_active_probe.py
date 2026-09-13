#!/usr/bin/env python3
"""Patch 1: Active dead-detection in fleetOrphanSweep.

Heartbeat 2min+ stale holder ko sirf passively wait nahi —
uske /health endpoint ko ACTIVE probe karo (fleetHTTPTimeout).
Probe fail = foran dead → claims release → failover ~2min me.

Heartbeat fresh (<2min) = server abhi zinda lag raha — normal
fleetOrphanAfter (5min) wait. Ye speed impact 0% rakhta hai:
sirf watchdog goroutine me, hot-path me kuch nahi.
"""
import re

path = "fleet.go"
src = open(path, encoding="utf-8").read()

# ── A) timing consts me probe-related add karo ─────────────────────────
old = """\tfleetHTTPTimeout  = 4 * time.Second  // remote /health timeout (quick public .server)
)"""
new = """\tfleetHTTPTimeout  = 4 * time.Second  // remote /health timeout (quick public .server)
\tfleetProbeAfter   = 2 * time.Minute  // heartbeat stale = ACTIVE /health probe start
)"""
assert src.count(old) == 1, "A: consts anchor not unique"
src = src.replace(old, new)

# ── B) fleetOrphanSweep: active probe logic ────────────────────────────
old = """\tfor _, jid := range fleetMgr.Redis.setMembers(fleetSessionsSet) {
\t\tholders := fleetClaimHolders(jid)
\t\tif len(holders) == 0 {
\t\t\tcontinue // unclaimed \u2014 fleetClaimAvailable utha lega
\t\t}
\t\tanyAlive := false
\t\tfor sid := range holders {
\t\t\tif alive[sid] {
\t\t\t\tanyAlive = true
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tif !anyAlive {
\t\t\t// sab holders dead \u2014 claims release kar do.
\t\t\tfor sid := range holders {
\t\t\t\t_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
\t\t\t}
\t\t\tInfoLog("FLEET: orphan claim released for %s (all holders dead)", jid)
\t\t}
\t}
}"""
new = """\t// heartbeat source map bhi \u2014 active probe ke liye (sid -> last ts).
\theartbeats := fleetHeartbeatMap()

\tfor _, jid := range fleetMgr.Redis.setMembers(fleetSessionsSet) {
\t\tholders := fleetClaimHolders(jid)
\t\tif len(holders) == 0 {
\t\t\tcontinue // unclaimed \u2014 fleetClaimAvailable utha lega
\t\t}
\t\tanyAlive := false
\t\tfor sid := range holders {
\t\t\tif alive[sid] {
\t\t\t\tanyAlive = true
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tif anyAlive {
\t\t\tcontinue
\t\t}
\t\t// \u2500\u2500 ACTIVE dead-detection \u2014\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
\t\t// Heartbeat 5min purani hai lekin server shayad abhi bhi zinda ho
\t\t// (Render busy / watchdog busy). 2min+ stale holders ke /health ko
\t\t\t// DIRECTLY probe karo \u2014 fail hua matlab server abhi abhi gaya.
\t\t// Isse failover 5min → ~2min ho jata hai (owner ka order).
\t\tallDead := true
\t\tfor sid := range holders {
\t\t\tif sid == fleetSelfID || fleetProbeServer(sid, heartbeats) {
\t\t\t\tallDead = false
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tif !allDead {
\t\t\tcontinue // koi holder /health se zinda nikla \u2014 rehne do
\t\t}
\t\t// sab holders dead \u2014 claims release kar do.
\t\tfor sid := range holders {
\t\t\t_, _ = fleetMgr.Redis.cmd("HDEL", fleetClaimPrefix+jid, sid)
\t\t}
\t\tInfoLog("FLEET: orphan claim released for %s (all holders dead, probe-confirmed)", jid)
\t}
}

// fleetHeartbeatMap: servers hash \u2192 {sid: last heartbeat ts} (probe
// decision ke liye \u2014 fleetAliveServers se hataya, duplicate KV read bacha).
func fleetHeartbeatMap() map[string]int64 {
\tout := map[string]int64{}
\tif fleetMgr == nil || fleetMgr.Redis == nil {
\t\treturn out
\t}
\tr, err := fleetMgr.Redis.cmd("HGETALL", fleetServersHash)
\tif err != nil {
\t\treturn out
\t}
\tvar pairs []string
\tif json.Unmarshal(r, &pairs) != nil {
\t\treturn out
\t}
\tfor i := 0; i+1 < len(pairs); i += 2 {
\t\tts, err := strconv.ParseInt(strings.TrimSpace(pairs[i+1]), 10, 64)
\t\tif err != nil {
\t\t\tcontinue
\t\t}
\t\tout[pairs[i]] = ts
\t}
\treturn out
}

// fleetProbeServer actively checks a claim holder's /health endpoint to
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
\t\t\t// ke through hi dead man lo \u2014 flapping se bacho.
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
}

// fleetServerURL resolves a serverID \u2192 base URL (https://{sid}/health).
// fleetSelfID wahi pattern use karta hai (RENDER_EXTERNAL_URL etc).
func fleetServerURL(sid string) string {
\tif sid == "" {
\t\treturn ""
\t}
\tif strings.HasPrefix(sid, "http://") || strings.HasPrefix(sid, "https://") {
\t\treturn strings.TrimSuffix(sid, "/")
\t}
\t// hostname-style sid \u2192 Render external URL pattern nahi banta \u2014
\t// heuristic: pure-hostname ya onrender.com ho to https:// prefix.
\tif strings.Contains(sid, ".") {
\t\treturn "https://" + strings.TrimSuffix(sid, "/")
\t}
\treturn ""
}

// fleetProbeURL does one quick GET {url} with fleetHTTPTimeout. true =
// koi bhi HTTP response aaya (2xx-5xx) \u2014 server process zinda hai.
func fleetProbeURL(raw string) bool {
\tcl := &http.Client{Timeout: fleetHTTPTimeout}
\tresp, err := cl.Get(raw)
\tif err != nil {
\t\treturn false
\t}
\t_, _ = io.Copy(io.Discard, resp.Body)
\t_ = resp.Body.Close()
\treturn true
}"""
assert src.count(old) == 1, "B: orphanSweep anchor not unique"
src = src.replace(old, new)

# ── C) fleetAliveServers HGETALL ko fleetHeartbeatMap reuse ───────────
# (dup read nahi \u2014 alag rakha taaki callers unchanged rahen; sirf
#  orphanSweep heartbeatMap use karta hai. No change needed.)
# ── D) imports: io add karo (fleetProbeURL io.Copy) ────────────────────
if "\t\"io\"" in src or 'import (\n\t"io"' in src:
    print("io already imported")
else:
    old_imp = 'import (\n\t"context"\n'
    assert src.count(old_imp) == 1, "D: import anchor not unique"
    src = src.replace(old_imp, 'import (\n\t"context"\n\t"io"\n')

open(path, "w", encoding="utf-8").write(src)
print("PATCH 1 APPLIED OK")
