#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Dead-Server Conflict Fix — PART 2
=================================
4. SELF-CLAIM HEAL (fleet.go, fleetClaimAvailable): hum khud ke claim me
   session hai magar local device nahi (ephemeral redeploy / disk wipe +
   per-sid blob missing) -> stale self-claim. Ab taken NAHI mana jayega —
   claim loop neeche se GLOBAL fleet blob (goldmd:fleet:sess:<jid>) se
   restore+connect kar dega. (Device hai to AutoLoad/boot sambhalega —
   tab taken hi rahega.)
5. Legacy svr1 helpers (upstash.go): RestoreLegacySessionDB +
   DeleteLegacySvr1Backup (one-time migration, main.go fix3 inka use
   karta hai).
6. legacySvr1BlobExists (main.go): boot pe check — purana shared svr1
   blob KV me hai ya nahi.
"""
import io, os, sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
fleet_go = os.path.join(ROOT, "fleet.go")
upstash_go = os.path.join(ROOT, "upstash.go")
main_go = os.path.join(ROOT, "main.go")

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

def patch(path, old, new, label):
    src = read(path)
    if new in src:
        print("SKIP [%s]: already applied" % label)
        return
    n = src.count(old)
    if n != 1:
        print("FAIL [%s]: anchor count=%d (expected 1)" % (label, n))
        sys.exit(1)
    write(path, src.replace(old, new, 1))
    print("OK   [%s]" % label)

# ─────────────────────────────────────────────────────────────────────
# FIX 4: self-claim heal — fleetClaimAvailable taken-loop.
# ─────────────────────────────────────────────────────────────────────
anchorA = '''\t\t\thb := fleetHeartbeatMap()
\t\t\ttaken := false
\t\t\tfor sid := range holders {
\t\t\t\tif fleetHolderAlive(sid, hb) {
\t\t\t\t\ttaken = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
\t\t\tif taken {
\t\t\t\tcontinue
\t\t\t}'''
newA = '''\t\t\thb := fleetHeartbeatMap()
\t\t\ttaken := false
\t\t\tfor sid := range holders {
\t\t\t\tif sid == fleetSelfID {
\t\t\t\t\t// SELF-CLAIM HEAL: claim humare naam hai magar local
\t\t\t\t\t// device Nahi (ephemeral redeploy / disk wipe + per-sid
\t\t\t\t\t// blob missing) \u2192 stale self-claim. Ise taken NAHI
\t\t\t\t\t// mano \u2014 neeche se GLOBAL fleet blob se restore +
\t\t\t\t\t// connect ho jayega. (Device local hai to AutoLoad/boot
\t\t\t\t\t// sambhal lega \u2014 tab taken hi hai.)
\t\t\t\t\tif fleetDeviceExists(jid) {
\t\t\t\t\t\ttaken = true
\t\t\t\t\t\tbreak
\t\t\t\t\t}
\t\t\t\t\tcontinue
\t\t\t\t}
\t\t\t\tif fleetHolderAlive(sid, hb) {
\t\t\t\t\ttaken = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
\t\t\tif taken {
\t\t\t\tcontinue
\t\t\t}'''
patch(fleet_go, anchorA, newA, "fix4 self-claim heal")

# ─────────────────────────────────────────────────────────────────────
# FIX 5: upstash.go — legacy svr1 helpers (RestoreSessionDB ke baad).
# ─────────────────────────────────────────────────────────────────────
anchorB = '''// RegisterJID remembers a paired JID so its pairing folder can be recreated'''
newB = '''// RestoreLegacySessionDB: purane (pre-fleet) shared "svr1" whole-DB backup
// ko disk pe wapas likhta hai \u2014 ONE-TIME migration (main.go boot flow).
// Uske baad DeleteLegacySvr1Backup use hata deta hai taake koi doosra
// upgraded server dobara restore na kare (shared-key conflict khatam).
func (u *Upstash) RestoreLegacySessionDB(path string) (bool, error) {
\tr, err := u.cmd("GET", sessionDBKeyConst+"svr1"+sessionDBKeySuffix)
\tif err != nil {
\t\treturn false, err
\t}
\tencoded := trimQuotes(string(r), "")
\tif encoded == "" {
\t\treturn false, nil
\t}
\tdata, err := base64.StdEncoding.DecodeString(encoded)
\tif err != nil {
\t\treturn false, err
\t}
\treturn true, os.WriteFile(path, data, 0o644)
}

// DeleteLegacySvr1Backup one-time migration ke baad purana shared svr1
// blob + uska JID registry delete kar deta hai \u2014 is se kabhi koi doosra
// server us shared key se restore karke conflict nahi kar sakta.
func (u *Upstash) DeleteLegacySvr1Backup() {
\t_, _ = u.cmd("DEL", sessionDBKeyConst+"svr1"+sessionDBKeySuffix)
\t_, _ = u.cmd("DEL", sessionJidsKeyConst+"svr1"+sessionJidsKeySuffix)
}

// RegisterJID remembers a paired JID so its pairing folder can be recreated'''
patch(upstash_go, anchorB, newB, "fix5 upstash legacy helpers")

# ─────────────────────────────────────────────────────────────────────
# FIX 6: main.go — legacySvr1BlobExists helper (boot me hasUsableWhatsAppDevice
# ke paas — wo function dhund ke uske pehle daalte hain).
# ─────────────────────────────────────────────────────────────────────
src = read(main_go)
if "func legacySvr1BlobExists" in src:
    print("SKIP [fix6 legacySvr1BlobExists]: already applied")
else:
    idx = src.find("func hasUsableWhatsAppDevice")
    if idx < 0:
        print("FAIL [fix6]: hasUsableWhatsAppDevice not found")
        sys.exit(1)
    helper = '''// legacySvr1BlobExists: purane (pre-fleet) version ka shared "svr1"
// session-DB blob KV me abhi bhi pada hai? (one-time migration check \u2014
// main boot flow isko dekh kar svr1 backup ko ek baar restore + delete
// karta hai taake do upgraded servers kabhi shared key se conflict na karein.)
func legacySvr1BlobExists(redis *Upstash) bool {
\tif redis == nil {
\t\treturn false
\t}
\tr, err := redis.cmd("GET", sessionDBKeyConst+"svr1"+sessionDBKeySuffix)
\tif err != nil {
\t\treturn false
\t}
\treturn trimQuotes(string(r), "") != ""
}

'''
    src = src[:idx] + helper + src[idx:]
    write(main_go, src)
    print("OK   [fix6 legacySvr1BlobExists helper]")

print("PART 2 APPLIED")
