#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Dead-Server Session Conflict Fix (Task E)
==========================================
Problem (user-reported):
  Server crash -> session failover dusre server pe -> LEKIN dead server ka
  per-session Storj data (blob + jids registry) delete NAHI hota tha.
  Naya/replacement server (ya same-URL zombie return) boot hote hi wahi
  purana session restore kar leta -> double-connect war -> WhatsApp
  stream-replace -> logout/crash.

Fixes:
  1. fleetPurgeDeadServerKV (fleet.go): failover COMPLETE hone par dead
     server ka per-server KV data delete:
       - goldmd:sessiondb:<dead>:blob   (whole-DB backup)
       - goldmd:sessiondb:<dead>:jids   (JID registry)
       - goldmd:fleet:servers hash me se heartbeat entry (HDEL)
     GLOBAL fleet blob (goldmd:fleet:sess:<jid>) delete NAHI hota — wahi
     future failovers ka source of truth hai.

  2. resolveServerID (upstash.go): pehle sirf GOLDMD_SERVER_ID -> "svr1"
     default tha. SAB Render services (bina env ke) EK HI "svr1" blob key
     share karte the = har server dusre ka DB restore karta = conflict
     factory. Ab fleetSelfID wali unique chain: GOLDMD_SERVER_ID ->
     RENDER_EXTERNAL_URL -> RENDER_INSTANCE_ID -> hostname -> "svr1".
     (fleetSelfID bhi yahi chain use karta hai — dono ab consistent.)

  3. Legacy svr1 migration (main.go): upgrade pe purana shared "svr1" blob
     ek BAAR fallback-restore hota hai (sirf jab current-sid blob missing
     ho) aur restore ke baad DELETE ho jata hai — one-time migration,
     is se dobara conflict nahi hoga.
"""
import io, os, sys

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
assert os.path.isdir(os.path.join(ROOT, "fleet.go")) or os.path.isfile(os.path.join(ROOT, "fleet.go")) or True
# patches/ ke andar se ROOT = repo root
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
# FIX 1a: fleetPurgeDeadServerKV — fleet.go me fleetMarkFailed se pehle.
# ─────────────────────────────────────────────────────────────────────
anchor1 = """func fleetMarkFailed(jid string) {"""
newfn = """// fleetPurgeDeadServerKV: failover complete hone ke baad DEAD server ke
// per-session KV footprints delete kar deta hai \u2014 zombie-return ya
// replacement server (same URL / naya server) boot hone pe purana session
// wapas restore hone aur double-connect war (WhatsApp stream-replace
// logout) se bachne ke liye. Ye delete hota hai:
//   - goldmd:sessiondb:<dead>:blob  (us server ka whole-DB backup)
//   - goldmd:sessiondb:<dead>:jids  (us server ka JID registry)
//   - goldmd:fleet:servers hash se uska heartbeat entry
// GLOBAL fleet blob (goldmd:fleet:sess:<jid>) KABHI delete nahi hota \u2014
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

func fleetMarkFailed(jid string) {"""
patch(fleet_go, anchor1, newfn, "fix1a fleetPurgeDeadServerKV func")

# ─────────────────────────────────────────────────────────────────────
# FIX 1b: call-site — fleetRestoreAndConnect ke takeover block me.
# ─────────────────────────────────────────────────────────────────────
anchor2 = """	if takeoverFrom != "" {
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		go fleetNotifyFailover(jid, takeoverFrom)
	}"""
new2 = """	if takeoverFrom != "" {
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		go fleetNotifyFailover(jid, takeoverFrom)
		// DEAD server ka per-session Storj data delete kar do \u2014 naya/
		// replacement server us URL pe banne par purana session wapas
		// restore kar ke double-connect war shuru na kar de.
		go fleetPurgeDeadServerKV(takeoverFrom)
	}"""
patch(fleet_go, anchor2, new2, "fix1b purge call-site")

# ─────────────────────────────────────────────────────────────────────
# FIX 2: resolveServerID — unique chain (fleetSelfID jaisi).
# ─────────────────────────────────────────────────────────────────────
anchor3 = """func resolveServerID() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_SERVER_ID")); v != "" {
		return v
	}
	return "svr1"
}"""
new3 = """func resolveServerID() string {
	// Per-server UNIQUE ID \u2014 fleetSelfID wali chain (GOLDMD_SERVER_ID ->
	// RENDER_EXTERNAL_URL -> RENDER_INSTANCE_ID -> hostname). Pehle ye
	// sirf GOLDMD_SERVER_ID -> "svr1" default tha, is liye bina env ke
	// SAB Render services EK HI "svr1" session-DB blob key share karti
	// thein \u2014 har reboot par kisi bhi doosre server ka pura DB restore
	// ho sakta tha (session conflict / double-connect logout). Ab har
	// deployment ka apna namespaced blob hota hai.
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
	return "svr1"
}"""
patch(upstash_go, anchor3, new3, "fix2 resolveServerID unique chain")

# ─────────────────────────────────────────────────────────────────────
# FIX 3: main.go — legacy shared "svr1" blob one-time migration.
# Current-sid blob missing ho to svr1 fallback restore + DELETE (ek hi
# baar, taake doosre upgraded servers dobara restore na karein).
# ─────────────────────────────────────────────────────────────────────
anchor4 = """			restored, rerr := redis.RestoreSessionDB(tmpPath)
			if rerr != nil {
				ErrLog("Could not restore session DB from Storj: %v", rerr)
			} else if restored {"""
new4 = """\t\t\trestored, rerr := redis.RestoreSessionDB(tmpPath)
\t\t\tif (rerr != nil || !restored) && legacySvr1BlobExists(redis) {
\t\t\t\t// LEGACY MIGRATION: purane version me sab servers shared
\t\t\t\t// "svr1" blob key use karte the. Upgrade ke baad naya
\t\t\t\t// per-server sid apna blob nahi milega to svr1 wala EK
\t\t\t\t// BAAR restore karo aur phir DELETE kar do \u2014 one-time
\t\t\t\t// migration (is se kabhi dobara conflict nahi hoga).
\t\t\t\tInfoLog("No blob for this server \u2014 trying legacy svr1 backup (one-time migration)...")
\t\t\t\tif lr, lerr := redis.RestoreLegacySessionDB(tmpPath); lerr == nil && lr {
\t\t\t\t\trestored, rerr = true, nil
\t\t\t\t\tredis.DeleteLegacySvr1Backup()
\t\t\t\t\tOkLog("Legacy svr1 session DB migrated & removed (will save under this server's own ID from now on)")
\t\t\t\t}
\t\t\t}
\t\t\tif rerr != nil {
\t\t\t\tErrLog("Could not restore session DB from Storj: %v", rerr)
\t\t\t} else if restored {"""
patch(main_go, anchor4, new4, "fix3 main.go legacy svr1 migration hook")

print("ALL PATCHES APPLIED")
