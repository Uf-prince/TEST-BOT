#!/usr/bin/env python3
# BOOT CAP FIX — "FULL 3/2" over-max sessions bug:
#   Mass-boot (Render auto-deploy) pe AutoLoad ne fleet guard ke race window
#   me over-max sessions bhi connect kar li thi (3 jab max 2). Ab:
#   1) AutoLoad: m.Count() >= maxPerServer hone pe aage koi session start NAHI
#      (over-max pairing dirs/db rows local chhod do — fleet blob global SAFE)
#   2) StartSession: fleet active + Count() >= max -> deny (fleet claim cycle
#      m.Count() >= max pe pehle se rukta hai — ab StartSession bhi guard hai)
import sys

def die(m): print("PATCH-FAIL:", m); sys.exit(1)

# ---------- 1) manager.go: AutoLoad cap ----------
src = open("manager.go").read()
anchor1 = '''\t\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)
'''
new1 = '''\t\t\t\tInfoLog("Connecting %d/%d: %s", n, len(users), u)
\t\t\t\t// BOOT CAP: is server ka quota (maxPerServer) already full hai
\t\t\t\t// to over-max session connect MAT karo. Mass-boot race me
\t\t\t\t// (Render auto-deploy pe saare servers ek saath uthte hain)
\t\t\t\t// fleet claims abhi bane hi nahi hote the — pehle version
\t\t\t\t// is race window me 3/2 jaisa over-max load kar deta tha.
\t\t\t\t// Over-max jids ko chhod dena SAFE hai: fleet blob GLOBAL hai,
\t\t\t\t// watchdog inhe baad me doosre server pe claim kar dega.
\t\t\t\tif m.Count() >= maxPairedSessions() {
\t\t\t\t\tWarnLog("BOOT CAP: %s skipped \u2014 server full (%d/%d), fleet baad me sambhalega", u, m.Count(), maxPairedSessions())
\t\t\t\t\treturn
\t\t\t\t}
'''
if anchor1 in src:
    if new1 in src:
        print("manager.go AutoLoad cap: already patched, SKIP")
    elif src.count(anchor1) == 1:
        src = src.replace(anchor1, new1)
        print("manager.go AutoLoad cap: OK")
    else:
        die(f"AutoLoad anchor count {src.count(anchor1)} != 1")
else:
    die("AutoLoad anchor not found (InfoLog Connecting line)")

# ---------- 2) manager.go: StartSession fleet-active guard ----------
anchor2 = '''func (m *Manager) StartSession(jid string) error {
\tif m.IsShuttingDown() {
\t\treturn fmt.Errorf("shutdown in progress")
\t}
'''
new2 = '''func (m *Manager) StartSession(jid string) error {
\tif m.IsShuttingDown() {
\t\treturn fmt.Errorf("shutdown in progress")
\t}
\t// FLEET CAP: fleet active hai to quota (maxPerServer) enforce karo.
\t// Fleet claim cycle m.Count() >= max pe pehle se ruk jata hai \u2014 yahan
\t\tsirf direct-call paths (pairing panel ya AutoLoad boot) guard hai.
\t// Fleet FAILOVER path (fleetRestoreAndConnect) apna quota check khud
\t// karta hai (m.Count() >= max pe claim loop chhod deta hai), isliye
\t// failover-target sessions kabhi block nahi hongi.
\tif fleetActive() && m.Count() >= maxPairedSessions() && !m.AlreadyConnected(jid) {
\t\treturn fmt.Errorf("server full (%d/%d) \u2014 fleet is active, try another server or wait for failover", m.Count(), maxPairedSessions())
\t}
'''
if new2 in src:
    print("manager.go StartSession cap: already patched, SKIP")
elif src.count(anchor2) == 1:
    src = src.replace(anchor2, new2)
    print("manager.go StartSession cap: OK")
else:
    die(f"StartSession anchor count {src.count(anchor2)} != 1")

open("manager.go", "w").write(src)

# ---------- 3) fleet.go: fleetActive helper ----------
fsrc = open("fleet.go").read()
helper = '''
// fleetActive: fleet watchdog chal raha hai? (claims/failover enable).
// StartSession cap sirf tab enforce hota hai jab fleet live hai \u2014 warna
// single-server (fleet off) mode me normal behaviour.
func fleetActive() bool {
\tfleetMu.Lock()
\tdefer fleetMu.Unlock()
\treturn fleetMgr != nil && fleetMgr.Redis != nil
}

'''
if helper in fsrc:
    print("fleet.go fleetActive helper: already present, SKIP")
else:
    a = "func fleetClaimAvailable() {"
    if fsrc.count(a) != 1:
        die(f"fleet.go anchor count {fsrc.count(a)} != 1")
    fsrc = fsrc.replace(a, helper + a)
    print("fleet.go fleetActive helper: OK")

open("fleet.go", "w").write(fsrc)
print("ALL PATCHES OK")
