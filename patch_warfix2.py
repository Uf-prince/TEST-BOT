#!/usr/bin/env python3
# WAR-LOOP FIX part 2: apply fresh-claim trust in fleetClaimAvailable +
# fleetOnConnected (same ordering bug as fleetHeldByLiveServer).
import io, sys

FLEET = "src/fleet.go"

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

src = read(FLEET)

# ── fleetClaimAvailable: respect fresh claims from other servers ──
old1 = '''\t\t\thb := fleetHeartbeatMap()
\t\t\ttaken := false
\t\t\tfor sid, ts := range holders {
\t\t\t\tif sid == fleetSelfID {'''
new1 = '''\t\t\thb := fleetHeartbeatMap()
\t\t\ttaken := false
\t\t\tfor sid, ts := range holders {
\t\t\t\tif sid == fleetSelfID {'''
# (no change to that line; we insert fresh-claim check after the self block)

old2 = '''\t\t\t\tif fleetHolderAlive(sid, hb) {
\t\t\t\t\t// LIVE holder \u2014 magar ZOMBIE-CLAIM check bhi (owner fix
\t\t\t\t\t// 2347067958986 wala case): server zinda magar ye session
\t\t\t\t\t// uspe chal nahi raha \u2192 purana claim \u2192 takeover ALLOWED.
\t\t\t\t\t// Fresh-claim trust window (fleetClaimFreshTrust) race-safe.
\t\t\t\t\tif fleetHolderRunsSession(sid, ts, jid) {
\t\t\t\t\t\ttaken = true
\t\t\t\t\t\tbreak
\t\t\t\t\t}
\t\t\t\t\t// zombie claim \u2014 ONLINE-ELSEWHERE guard neeche dobara
\t\t\t\t\t// verify karega, phir takeover chalega.
\t\t\t\t}'''
new2 = '''\t\t\t\t// FRESH-CLAIM TRUST (WAR-LOOP FIX): claim <10min = holder ne
\t\t\t\t// abhi claim kiya \u2014 heartbeat hash me na hone par bhi RESPECT
\t\t\t\t// (purane binaries heartbeat nahi likhte). Warna har 60s pass
\t\t\t\t// pe hum claim chheente hain \u2192 infinite war.
\t\t\t\tif fleetClaimFresh(ts) {
\t\t\t\t\ttaken = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tif fleetHolderAlive(sid, hb) {
\t\t\t\t\t// LIVE holder \u2014 magar ZOMBIE-CLAIM check bhi (owner fix
\t\t\t\t\t// 2347067958986 wala case): server zinda magar ye session
\t\t\t\t\t// uspe chal nahi raha \u2192 purana claim \u2192 takeover ALLOWED.
\t\t\t\t\t// Fresh-claim trust window (fleetClaimFreshTrust) race-safe.
\t\t\t\t\tif fleetHolderRunsSession(sid, ts, jid) {
\t\t\t\t\t\ttaken = true
\t\t\t\t\t\tbreak
\t\t\t\t\t}
\t\t\t\t\t// zombie claim \u2014 ONLINE-ELSEWHERE guard neeche dobara
\t\t\t\t\t// verify karega, phir takeover chalega.
\t\t\t\t}'''

if old2 not in src:
    print("ERR: fleetClaimAvailable block not found")
    sys.exit(1)
src = src.replace(old2, new2, 1)

# ── fleetOnConnected: respect fresh claims from other servers ──
old3 = '''\t\tholders := fleetClaimHolders(jid)
\t\tlive := false
\t\tfor sid, ts := range holders {
\t\t\tif sid == fleetSelfID {
\t\t\t\tcontinue
\t\t\t}
\t\t\tif fleetHolderAlive(sid, fleetHeartbeatMap()) && fleetHolderRunsSession(sid, ts, jid) {
\t\t\t\t// live holder + session uske paas WAQAI hai (zombie claim
\t\t\t\t// nahi) \u2014 usi ko chalane do, hum chup (war-safe).
\t\t\t\tlive = true
\t\t\t\tbreak
\t\t\t}
\t\t}'''
new3 = '''\t\tholders := fleetClaimHolders(jid)
\t\tlive := false
\t\tfor sid, ts := range holders {
\t\t\tif sid == fleetSelfID {
\t\t\t\tcontinue
\t\t\t}
\t\t\t// FRESH-CLAIM TRUST (WAR-LOOP FIX): doosre server ki fresh claim
\t\t\t// ko respect karo \u2014 heartbeat me na hone par bhi (purane binaries).
\t\t\tif fleetClaimFresh(ts) {
\t\t\t\tlive = true
\t\t\t\tbreak
\t\t\t}
\t\t\tif fleetHolderAlive(sid, fleetHeartbeatMap()) && fleetHolderRunsSession(sid, ts, jid) {
\t\t\t\t// live holder + session uske paas WAQAI hai (zombie claim
\t\t\t\t// nahi) \u2014 usi ko chalane do, hum chup (war-safe).
\t\t\t\tlive = true
\t\t\t\tbreak
\t\t\t}
\t\t}'''

if old3 not in src:
    print("ERR: fleetOnConnected block not found")
    sys.exit(1)
src = src.replace(old3, new3, 1)

write(FLEET, src)
print("OK: fleet.go patched (fleetClaimAvailable + fleetOnConnected)")
