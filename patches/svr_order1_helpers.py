#!/usr/bin/env python3
# PATCH 1: .svr number-wise order + offline single lines (owner order)
# - fleetServerNumberForSID: sid -> servers.json number ("SERVER 3")
# - CmdServerMenu: iterate scan in index order (1,2,3...), inline offline
#   lines (N ke turant baad), summary me server count only.
import re

P = 'fleet_commands.go'

src = open(P).read()
orig = src

# ── A. helper fleetServerNumberForSID (fleet_commands.go me fleetServerNumber ke baad) ──
helper = r'''
// fleetServerNumberForSID: kisi bhi server sid/URL se servers.json ka
// server NUMBER nikaalta hai ("SERVER 3" -> "3"). Match chain:
//   1. GOLDMD_SERVER_ID / sid          (exact)
//   2. fleetServerURL(sid) == entry URL (http/https + trailing /)
//   3. sid base-hostname == entry URL host (onrender wale short names)
// Na mile to "0".
func fleetServerNumberForSID(sid string) string {
	loadServersConfig()
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return "0"
	}
	wantURL := fleetServerURL(sid)
	// base-hostname (dots + scheme strip) — "https://gold-x.onrender.com/" -> "gold-x.onrender.com"
	baseSid := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(sid, "https://"), "http://"), "/")
	for _, e := range serversCfg.Servers {
		if sid == e.URL {
			return fleetServerNumber(e.Name)
		}
		if wantURL != "" && wantURL == strings.TrimSuffix(e.URL, "/") {
			return fleetServerNumber(e.Name)
		}
		eBase := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(e.URL, "https://"), "http://"), "/")
		if baseSid != "" && baseSid == eBase {
			return fleetServerNumber(e.Name)
		}
	}
	return "0"
}
'''

anchor = '''func fleetServerNumber(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) >= 2 {
		return fields[len(fields)-1]
	}
	return name
}'''
assert src.count(anchor) == 1, 'anchor fleetServerNumber missing'
src = src.replace(anchor, anchor + helper)

# ── B. CmdServerMenu rebuild (number-wise order + inline offline lines) ──
start_marker = 'func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {'
end_marker = 'func fleetServerNumber(name string) string {'
si = src.index(start_marker)
ei = src.index(end_marker)
assert si < ei, 'CmdServerMenu position broken'

new_body = r'''func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {
	scan := fleetScanAll()
	var b strings.Builder

	b.WriteString("*🔰 GOLD-MD SERVERS INFO 🔰*\n\n")

	// ── OWNER ORDER: servers NUMBER-WISE (1, 2, 3...) — scan slices
	//    khud servers.json ke order me aate hain, isliye seedha iterate.
	//    ONLINE -> full INFO block. OFFLINE -> sirf EK choti line
	//    *SERVER ❮ N ❯ OFFLINE* (us number ke turant baad, number order
	//    me hi — Server 2 upar aur Server 1 niche wala mix NAHI hoga).
	online, offline, pairs := 0, 0, 0
	for _, srv := range scan {
		if srv.Online {
			online++
			pairs += srv.Sessions
			paired := "0/0"
			if srv.Max > 0 {
				if srv.Sessions >= srv.Max {
					paired = fmt.Sprintf("%d/%d FULL", srv.Sessions, srv.Max)
				} else {
					paired = fmt.Sprintf("%d/%d", srv.Sessions, srv.Max)
				}
			}
			b.WriteString(fmt.Sprintf("*🔰 %s INFORMATION 🔰*\n", srv.Name))
			b.WriteString("*🔰 STATUS :➯ ACTIVE*\n")
			b.WriteString(fmt.Sprintf("*🔰 MAX PAIRING :➯ %d*\n", srv.Max))
			b.WriteString(fmt.Sprintf("*🔰 PAIRED :➯ ❮ %s ❯*\n", paired))
			b.WriteString(fmt.Sprintf("*🔰 RE :➯ %s*\n\n", srv.RE))
		} else {
			offline++
			// OWNER ORDER: offline server sirf EK choti line — number order me.
			b.WriteString(fmt.Sprintf("*SERVER ❮ %s ❯ OFFLINE*\n\n", fleetServerNumber(srv.Name)))
		}
	}

	// ── compact fleet summary ──
	b.WriteString(fmt.Sprintf("*🔰 SERVERS ONLINE :➯ %d*\n", online))
	b.WriteString(fmt.Sprintf("*🔰 SERVERS OFFLINE :➯ %d*\n", offline))
	b.WriteString(fmt.Sprintf("*🔰 TOTAL LIVE PAIRINGS :➯ %d*\n", pairs))

	// ── LOCAL SESSIONS detail — SIRF DEVELOPERS (private JIDs public me
	//    leak nahi honge; pairing status sabko dikhta hai, numbers nahi) ──
	if isDeveloperCommand(info) {
		sessions := s.Manager.List()
		b.WriteString(fmt.Sprintf("\n*🔰 THIS SERVER (%s) :➯ ❮ %d/%d ❯*\n",
			fleetSelfID, s.Manager.Count(), maxPairedSessions()))
		if len(sessions) > 0 {
			b.WriteString("\n*🔰 LOCAL SESSIONS 🔰*\n\n")
			for i, sess := range sessions {
				status := "🔴 disconnected"
				if sess.Client != nil && sess.Client.IsConnected() {
					status = "🟢 online"
				}
				b.WriteString(fmt.Sprintf("%d. %s\n   %s | %s\n", i+1,
					sess.JID, status, formatUptime(time.Since(sess.Started))))
			}
		}
	}

	s.Reply(info, b.String())
}

'''
src = src[:si] + new_body + src[ei:]

open(P, 'w').write(src)
print('PATCH 1 OK: fleet_commands.go', len(orig), '->', len(src))
