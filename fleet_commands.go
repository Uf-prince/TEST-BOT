package main

// ═════════════════════════════════════════════════════════════════════════════
//   GOLD-MD — SERVER COMMANDS
//
//   OWNER ORDER (latest):
//   ".server command ko public bana do — koi bhi servers pair dekh ske
//    easily. Jo jo server OFFLINE ho, online servers ke bad \n\n\n mar ke
//    choti single line me line-by-line dikhta jaye:
//      *SERVER ❮ {server number} ❯ OFFLINE*"
//
//   Commands:
//     .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//     .sessions → PUBLIC servers menu (koi bhi chala sakta hai — pairing
//                 status sabko dikhta hai). .menu me ab bhi hidden.
//     .host5gb  → 5GB bandwidth report — DEVELOPER-ONLY (sirf
//                 923158930864 / 923276650623), inke elava koi b likhe to
//                 *THIS IS DEVELOPER COMMAND* (menu me visible).
//
//   PUBLIC vs PRIVATE split:
//     • Pairing/status data (servers.json ke 200 servers ka /health) →
//       PUBLIC — koi b dekh sakta hai.
//     • LOCAL SESSIONS detail (asli WhatsApp JIDs / phone numbers) → SIRF
//       developers — public users ko private numbers leak nahi hote.
//
//   OFFLINE FORMAT (owner ka exact order): online servers ke full INFO
//   blocks ke baad \n\n\n (3 enter) lagta hai, phir har offline server sirf
//   EK choti line me:  *SERVER ❮ N ❯ OFFLINE*
// ═════════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// developerNumbers: GOLD-MD developer numbers (BARE number form — JID ka
// User part, device suffix nahi). Sirf ye 2 numbers .host5gb aur private
// LOCAL SESSIONS detail dekh sakte hain.
var developerNumbers = map[string]bool{
	"923158930864": true,
	"923276650623": true,
}

// devCommandReply: non-developer ko milne wala fixed reply (owner order).
const devCommandReply = "*THIS IS DEVELOPER COMMAND*"

// isDeveloperCommand: sender developer list me hai? JID dono forms check
// hote hain (Sender = phone JID, SenderAlt = LID) — bare number compare.
func isDeveloperCommand(info types.MessageInfo) bool {
	if !info.Sender.IsEmpty() && developerNumbers[info.Sender.User] {
		return true
	}
	if !info.SenderAlt.IsEmpty() && developerNumbers[info.SenderAlt.User] {
		return true
	}
	return false
}

// hiddenCommands: Commands-map me registered par .menu me KABHI nahi dikhne
// wale secret commands. .host5gb MENU ME VISIBLE hai — isliye yahan NAHI
// hai. Server-menu family ab PUBLIC hai (koi bhi chala sakta hai) par .menu
// me ab bhi nahi dikhti (secret rahegi, sirf wahi jaanne wale use karenge).
var hiddenCommands = map[string]bool{
	"server":     true,
	"servers":    true,
	"svr":        true,
	"svrinfo":    true,
	"serverinfo": true,
	"session":    true,
	"sessions":   true,
	"svrchange":  true, // git-token command — hidden (dev-only)
}

func init() {
	// .host5gb — bandwidth report (MENU VISIBLE, developer-only).
	RegisterCommand("host5gb", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		if !isDeveloperCommand(info) {
			s.Reply(info, devCommandReply)
			return
		}
		s.CmdHost5GB(info, args, prefix)
	})

	// .server + aliases — PUBLIC servers menu (owner order: koi bhi
	// servers pair dekh ske). Koi guard nahi — sabke liye open.
	for _, name := range []string{"server", "servers", "svr", "svrinfo", "serverinfo", "session", "sessions"} {
		nm := name
		RegisterCommand(nm, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			s.CmdServerMenu(info, args, prefix)
		})
	}

	// .svrchange — git-token servers.json update (DEVELOPER-ONLY, hidden).
	// GitHub + GitLab dono repos me server links badal ke push — Render
	// auto-deploy foran trigger hota hai (owner ko git pe jana nahi prega).
	RegisterCommand("svrchange", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		if !isDeveloperCommand(info) {
			s.Reply(info, devCommandReply)
			return
		}
		s.Reply(info, svrParseAndRun(args))
	})
}

// ── .host5gb — REAL bandwidth report (all running servers) ──
func (s *Session) CmdHost5GB(info types.MessageInfo, args []string, prefix string) {
	s.Reply(info, fleetRender5GBText())
}

// ── .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//    .sessions — PUBLIC servers menu (owner ka naya offline format) ──
//
// OWNER ORDER: ek hi fleetScanAll() call (200 servers × parallel /health,
// 4s timeout — QUICK). Online servers full INFO block me, offline servers
// \n\n\n ke baad single choti lines me.
func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {
	scan := fleetScanAll()
	var b strings.Builder

	b.WriteString("*🔰 GOLD-MD SERVERS INFO 🔰*\n\n")

	// ── ONLINE servers: full INFO block (user ka approved format) ──
	online, offline, pairs := 0, 0, 0
	var offlineLines []string
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
			// OWNER ORDER: offline server sirf EK choti line —
			// *SERVER ❮ N ❯ OFFLINE* (server number ke saath).
			offlineLines = append(offlineLines,
				fmt.Sprintf("*SERVER ❮ %s ❯ OFFLINE*", fleetServerNumber(srv.Name)))
		}
	}

	// ── OFFLINE servers: online blocks ke baad \n\n\n (3 enter) lagakar
	//    line-by-line choti lines (owner ka exact order) ──
	// (online block ke \n\n ke saath milakar total 3 newline = owner order)
	if len(offlineLines) > 0 {
		b.WriteString("\n")
		for _, line := range offlineLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// ── compact fleet summary ──
	b.WriteString(fmt.Sprintf("\n*🔰 SERVERS ONLINE :➯ %d*\n", online))
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

// fleetServerNumber: servers.json ke name ("SERVER 14") se sirf number
// nikalta hai offline choti line ke liye — "SERVER 14" → "14".
func fleetServerNumber(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) >= 2 {
		return fields[len(fields)-1]
	}
	return name
}
