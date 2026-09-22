#!/usr/bin/env python3
# WAR-LOOP ROOT-CAUSE FIX patcher.
# Fix: fresh-claim trust must be checked BEFORE the heartbeat-alive gate,
# otherwise attackers invisible to the heartbeat hash always trigger RETAKE.
import io, sys

FLEET = "src/fleet.go"

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

src = read(FLEET)

# ── 1. Insert fleetClaimFresh() helper + fix fleetHeldByLiveServer ──
old_block = '''\thb := fleetHeartbeatMap()
\tfor sid, ts := range holders {
\t\tif sid == fleetSelfID {
\t\t\tcontinue
\t\t}
\t\tif !fleetHolderAlive(sid, hb) {
\t\t\tcontinue // dead holder \u2014 claim stale, ignore (orphan sweep)
\t\t}'''

new_block = '''\thb := fleetHeartbeatMap()
\tfor sid, ts := range holders {
\t\tif sid == fleetSelfID {
\t\t\tcontinue
\t\t}
\t\t// FRESH-CLAIM TRUST (WAR-LOOP ROOT-CAUSE FIX 2026-09-21): claim
\t\t// <10min purani = holder ne abhi claim kiya (connect/boot race
\t\t// window). Heartbeat hash me na hona DEAD nahi \u2014 purane binaries
\t\t// heartbeat nahi likhte (evidence: FLEET_WAR_EVIDENCE.txt \u2014 Render
\t\t// attackers heartbeat me KABHI nahi aate). Pehle ye check
\t\t// fleetHolderAlive() ke BAAD tha \u2192 alive=false \u2192 continue \u2192
\t\t// fresh-trust KABHI reach nahi hota \u2192 fleetHeldByLiveServer()
\t\t// hamesha false \u2192 WAR GUARD hamesha RETAKE \u2192 infinite war.
\t\t// Ab fresh claim ko alive-check se PEHLE respect karo.
\t\tif fleetClaimFresh(ts) {
\t\t\treturn true
\t\t}
\t\tif !fleetHolderAlive(sid, hb) {
\t\t\tcontinue // dead holder \u2014 claim stale, ignore (orphan sweep)
\t\t}'''

if old_block not in src:
    print("ERR: fleetHeldByLiveServer block not found")
    sys.exit(1)
src = src.replace(old_block, new_block, 1)

# ── 2. Add fleetClaimFresh helper right before fleetHeldByLiveServer ──
anchor = "// fleetHeldByLiveServer: kya ye JID kisi AUR live server ke fresh claim"
helper = '''// fleetClaimFresh: claim timestamp itni fresh hai ki bina probe trust karein?
// (fleetClaimFreshTrust window \u2014 boot/restore/connect race window.)
//
// \u26a0\ufe0f WAR-LOOP ROOT-CAUSE FIX (2026-09-21): ye check PEHLE sirf
// fleetHolderRunsSession ke ANDAR tha, jo fleetHolderAlive() ke BAAD chalta
// tha. Render attackers heartbeat hash me KABHI nahi aate \u2192
// fleetHolderAlive() false \u2192 loop `continue` \u2192 fresh-claim trust KABHI
// reach nahi hota \u2192 fleetHeldByLiveServer() hamesha false \u2192 WAR GUARD
// hamesha RETAKE \u2192 infinite war. Ab fresh-claim trust alive-check se
// PEHLE lagta hai (heartbeat ki parwah nahi).
func fleetClaimFresh(ts int64) bool {
\treturn ts > 0 && time.Now().Unix()-ts < int64(fleetClaimFreshTrust/time.Second)
}

'''
if anchor not in src:
    print("ERR: anchor for helper not found")
    sys.exit(1)
src = src.replace(anchor, helper + anchor, 1)

write(FLEET, src)
print("OK: fleet.go patched (fleetClaimFresh + fleetHeldByLiveServer)")
