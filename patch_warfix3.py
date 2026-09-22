#!/usr/bin/env python3
# WAR-LOOP FIX part 3: dedicated claim heartbeat.
# Problem: claim re-stamp cadence (10 min, in fleetRefreshBlobs) == fresh-trust
# window (10 min) -> at the boundary a claim looks stale -> another server
# steals it -> war. Fix: re-stamp claims every 3 min (well inside the window).
import io, sys

FLEET = "src/fleet.go"

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

src = read(FLEET)

# 1. Start the claim heartbeat in fleetInit (after fleetWatchdog).
old_init = '''\tgo fleetWatchdog()
\tgo fleetHealGZ1()'''
new_init = '''\tgo fleetWatchdog()
\tgo fleetHealGZ1()
\tgo fleetClaimHeartbeat()'''
if old_init not in src:
    print("ERR: fleetInit anchor not found")
    sys.exit(1)
src = src.replace(old_init, new_init, 1)

# 2. Add fleetClaimHeartbeat function right before fleetRefreshBlobs.
anchor = "// fleetRefreshBlobs re-pushes blobs for every CURRENTLY CONNECTED session"
func = '''// fleetClaimHeartbeat: connected sessions ke claims ko har 3 min me re-stamp
// karta hai (WAR-LOOP FIX). Pehle claim sirf connect pe + 10-min blob refresh
// pe likha jata tha \u2014 magar fresh-trust window bhi 10 min hai, is liye
// boundary pe claim STALE lagti thi aur doosra server chheenta tha \u2192 war.
// Ab 3-min cadence (window ka 30%) \u2014 claim kabhi stale nahi lagti, koi
// server chheenta nahi. HSET chhota hai (bandwidth negligible).
func fleetClaimHeartbeat() {
\tdefer func() { _ = recover() }()
\tif !fleetHandoffEnabled() {
\t\treturn
\t}
\tt := time.NewTicker(3 * time.Minute)
\tdefer t.Stop()
\tfor range t.C {
\t\tm := fleetMgr
\t\tif m == nil || m.Redis == nil || m.IsShuttingDown() {
\t\t\treturn
\t\t}
\t\tnow := strconv.FormatInt(time.Now().Unix(), 10)
\t\tfor _, s := range m.List() {
\t\t\tif s == nil || !s.Paired || s.LocalOnly {
\t\t\t\tcontinue
\t\t\t}
\t\t\tif isLocalOnlyJID(m.cfg.PairingDir, s.JID) {
\t\t\t\tcontinue
\t\t\t}
\t\t\t_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+s.JID, fleetSelfID, now)
\t\t}
\t}
}

'''
if anchor not in src:
    print("ERR: fleetRefreshBlobs anchor not found")
    sys.exit(1)
src = src.replace(anchor, func + anchor, 1)

write(FLEET, src)
print("OK: fleet.go patched (fleetClaimHeartbeat)")
