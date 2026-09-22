#!/usr/bin/env python3
"""Patch 2 (FIXED): Failover takeover detection + owner notification.

fleetRestoreAndConnect me: agar session ka claim PEHLE se kisi
AUR server ke paas tha (jo ab dead hai) = FAILOVER takeover.
Restore+connect success ke baad us session ke client se owner
JID pe notification bhejta hai.

NOTE: m.Get() nahi hai — m.List() se user-part match. waProto
import = "go.mau.fi/whatsmeow/binary/proto" (named import).
"""
path = "fleet.go"
src = open(path, encoding="utf-8").read()

# ── A) fleetRestoreAndConnect: takeover detect ───────────────────────
old = """\t// 1. claim stamp daalo.
\t_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID, now)
"""
new = """\t// 0. FAILOVER detection \u2014 pehle se koi holder tha? (dead hai isliye
\t// claim kar rahe hain). takeover = true to restore ke baad owner ko
\t// notify karenge ki session dusre server pe shift ho gaya.
\tprev := fleetClaimHolders(jid)
\tvar takeoverFrom string
\tfor sid := range prev {
\t\tif sid != fleetSelfID {
\t\t\ttakeoverFrom = sid
\t\t\tbreak
\t\t}
\t}

\t// 1. claim stamp daalo.
\t_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID, now)
"""
assert src.count(old) == 1, "A: restore anchor not unique"
src = src.replace(old, new)

# ── B) connect success ke baad notification ────────────────────────────
old = """\tOkLog("FLEET: session %s restored from Storj and connected (server %s)", jid, fleetSelfID)
}"""
new = """\tOkLog("FLEET: session %s restored from Storj and connected (server %s)", jid, fleetSelfID)

\t// FAILOVER: ye session kisi aur (dead) server ka tha \u2014 owner ko batado
\t// ki bot dusre server pe wapas online ho gaya hai.
\tif takeoverFrom != "" {
\t\tgo fleetNotifyFailover(jid, takeoverFrom)
\t}
}

// fleetNotifyFailover: failover-complete hone par session ke owner ko
// message bhejta hai. Owner Redis setting "owner" se milta hai (pairing
// time pe set hota hai). Message session ke khud ke client se jata hai \u2014
// iska matlab bot ka number khud apne owner ko bata raha hai: "main ab
// dusre server pe online hoon, reconnect ho gaya".
func fleetNotifyFailover(jid string, deadServer string) {
\tdefer func() { _ = recover() }()
\tm := fleetMgr
\tif m == nil {
\t\treturn
\t}
\t// client settle hone do (connect ke turant baad message queues full
\t// ho sakte hain) \u2014 10s baad bhejo.
\ttime.Sleep(10 * time.Second)

\t// session ki owner JID nikaalo (Redis setting).
\towner := fleetOwnerFor(jid)
\tif owner == "" {
\t\tInfoLog("FLEET: failover notify skip %s \u2014 owner unknown", jid)
\t\treturn
\t}
\townerJID, err := types.ParseJID(normalizeJID(owner))
\tif err != nil || ownerJID.IsEmpty() {
\t\tInfoLog("FLEET: failover notify skip %s \u2014 owner JID parse failed", jid)
\t\treturn
\t}

\t// jis session ke liye notify karna hai wo ab is server pe live hai \u2014
\t// uska client use karo (bot apne owner ko khud batayega).
\t// NOTE: m.Get() nahi hai — m.List() se user-part match.
\tvar sess *Session
\tfor _, s := range m.List() {
\t\tif fleetUserPart(s.JID) == fleetUserPart(jid) {
\t\t\tsess = s
\t\t\tbreak
\t\t}
\t}
\tif sess == nil || sess.Client == nil || !sess.Client.IsConnected() {
\t\tInfoLog("FLEET: failover notify skip %s \u2014 session not live here", jid)
\t\treturn
\t}
\tsrv := fleetSelfID
\tif strings.Contains(srv, ".onrender.com") {
\t\tif i := strings.Index(srv, ".onrender.com"); i > 0 {
\t\t\tsrv = srv[:i]
\t\t}
\t}
\tdead := deadServer
\tif strings.Contains(dead, ".onrender.com") {
\t\tif i := strings.Index(dead, ".onrender.com"); i > 0 {
\t\t\tdead = dead[:i]
\t\t}
\t}
\ttext := "*\u2500\u2500 SESSION FAILOVER \u2500\u2500*\n\n" +
\t\t"\u26a1 Purana server band ho gaya tha (" + dead + ").\n" +
\t\t"\u2705 Bot ab dusre server pe *online* hai \u2014 session *reconnect ho gaya*\n" +
\t\t"\u2139\ufe0f New Server: " + srv + "\n\n" +
\t\t"\u2705 Apne pas rakh liya \u2014 ab jaise pehle hi use karo, koi farak nahi."

\t_, err = sess.Client.SendMessage(context.Background(), ownerJID, &waProto.Message{
\t\tExtendedTextMessage: &waProto.ExtendedTextMessage{
\t\t\tText: &text,
\t\t},
\t})
\tif err != nil {
\t\tWarnLog("FLEET: failover notify send failed for %s: %v", jid, err)
\t} else {
\t\tOkLog("FLEET: failover notification sent to owner of %s (from %s \u2192 %s)", jid, dead, srv)
\t}
}"""
assert src.count(old) == 1, "B: success anchor not unique"
src = src.replace(old, new)
print("A+B applied")

# ── C) imports: waProto + types add ────────────────────────────────────
old_imp = '\t"strings"\n'
assert src.count(old_imp) == 1, "C: strings anchor not unique"
new_imp = '\t"strings"\n\twaProto "go.mau.fi/whatsmeow/binary/proto"\n\t"go.mau.fi/whatsmeow/types"\n'
src = src.replace(old_imp, new_imp, 1)

open(path, "w", encoding="utf-8").write(src)
print("PATCH 2 APPLIED OK")
