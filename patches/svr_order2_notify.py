#!/usr/bin/env python3
# PATCH 2 (v2): FAILOVER msg -> naya GOLD-MD SERVER ERROR format.
# NOTE: Go source me emoji LITERAL \u26a1 escapes hain — r-string se exact match.
P = 'fleet.go'
src = open(P).read()
orig = src

old = r'''	srv := fleetSelfID
	if strings.Contains(srv, ".onrender.com") {
		if i := strings.Index(srv, ".onrender.com"); i > 0 {
			srv = srv[:i]
		}
	}
	dead := deadServer
	if strings.Contains(dead, ".onrender.com") {
		if i := strings.Index(dead, ".onrender.com"); i > 0 {
			dead = dead[:i]
		}
	}
	text := "*── SESSION FAILOVER ──*\n\n" +
		"\u26a1 Purana server band ho gaya tha (" + dead + ").\n" +
		"\u2705 Bot ab dusre server pe *online* hai — session *reconnect ho gaya*\n" +
		"\u2139\ufe0f New Server: " + srv + "\n\n" +
		"\u2705 Apne pas rakh liya — ab jaise pehle hi use karo, koi farak nahi."
'''
assert src.count(old) == 1, 'old failover text block not found (count=%d)' % src.count(old)

new = r'''	// ── OWNER ORDER (naya format): server NUMBERS ke saath — jis server
	//    pe user ne pair kiya tha (ab band) aur jis pe ab online hai.
	//    fleetServerNumberForSID: sid/URL -> servers.json number ("3").
	srvNum := fleetServerNumberForSID(fleetSelfID)
	deadNum := fleetServerNumberForSID(deadServer)
	text := "*\U0001F530 GOLD-MD SERVER ERROR \U0001F530*\n\n" +
		"*YOU PAIRED YOUR BOT ON THIS SERVER \u276e " + deadNum + " \u276f BUT THIS SERVER HAS BEEN SHUT DOWN*\n\n" +
		"*NOW YOUR BOT IS RUNNING ON THIS SERVER \u276e " + srvNum + " \u276f*\n\n" +
		"*DON'T WORRY ABOUT THIS ISSUE. YOUR BOT IS ONLINE AGAIN AND WORKING FINE. NO NEED TO PAIR AGAIN \u2705*"
'''
src = src.replace(old, new)

open(P, 'w').write(src)
print('PATCH 2 OK: fleet.go', len(orig), '->', len(src))
