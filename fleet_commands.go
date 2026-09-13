package main

// ═══════════════════════════════════════════════════════════════════════════
//   GOLD-MD — SERVER COMMANDS (developer-only)
//
//   OWNER REQUEST:
//   ".render5gb ka naam host5gb rakho, menu me show karo, sirf 2 numbers
//    use kar sake (923158930864, 923276650623), inke elava koi b likhe to
//    *THIS IS DEVELOPER COMMAND* bol do"
//
//   Commands:
//     .host5gb  → har RUNNING server ka 5GB / used / remaining bandwidth
//                 (REAL egress — /proc/net/dev, Storj-persisted) — MENU ME
//                 VISIBLE, developer-only.
//     .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//     .sessions → hidden servers menu (menu me KABHI nahi dikhta) —
//                 developer-only, same guard.
//
//   GUARD DESIGN (strict): sender ka BARE NUMBER check hota hai (User field
//   — device suffix / LID alt dono cover). IsFromMe bypass NAHI hai — har
//   paired bot account ka apna message bhi strictly check hota hai, warna
//   koi b user apne paired session se .host5gb chala leta.
//
//   In commands ko ownerOnlyCommands me NAHI rakha gaya (dispatcher ka
//   silent-return guard hata diya) taaki non-developer ko SILENT nahi,
//   balki "*THIS IS DEVELOPER COMMAND*" reply mile — owner ka exact order.
// ═══════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// developerNumbers: GOLD-MD developer numbers (BARE number form — JID ka
// User part, device suffix nahi). Sirf ye 2 numbers .host5gb / server-menu
// family use kar sakte hain. Inke elava KOI BHI (owner/sudo/paired user)
// → "*THIS IS DEVELOPER COMMAND*".
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
// wale secret commands (owner order: kisi ko pata na chle). .host5gb ab
// MENU ME VISIBLE hai — isliye yahan NAHI hai. Sirf server-menu family
// hidden rehti hai.
var hiddenCommands = map[string]bool{
	"server":     true,
	"servers":    true,
	"svr":        true,
	"svrinfo":    true,
	"serverinfo": true,
	"session":    true,
	"sessions":   true,
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

	// .server + aliases — hidden servers menu (developer-only).
	for _, name := range []string{"server", "servers", "svr", "svrinfo", "serverinfo", "session", "sessions"} {
		nm := name
		RegisterCommand(nm, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			if !isDeveloperCommand(info) {
				s.Reply(info, devCommandReply)
				return
			}
			s.CmdServerMenu(info, args, prefix)
		})
	}
}

// ── .host5gb — REAL bandwidth report (all running servers) ──
func (s *Session) CmdHost5GB(info types.MessageInfo, args []string, prefix string) {
	s.Reply(info, fleetRender5GBText())
}

// ── .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//    .sessions — hidden servers menu (user ka exact format, real checks) ──
func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {
	var b strings.Builder

	// servers.json ke HAR server ka real /health block (exact user format).
	b.WriteString(fleetServersInfoText())

	// ── compact fleet summary ──
	total, running := 0, 0
	for _, srv := range fleetScanAll() {
		if srv.Online {
			running++
			total += srv.Sessions
		}
	}
	b.WriteString(fmt.Sprintf("*🔰 SERVERS ONLINE :❯ %d*\n", running))
	b.WriteString(fmt.Sprintf("*🔰 TOTAL LIVE PAIRINGS :❯ %d*\n\n", total))

	// ── local sessions detail (purane .sessions ka data — dev ke liye) ──
	sessions := s.Manager.List()
	b.WriteString(fmt.Sprintf("*🔰 THIS SERVER (%s) :❯ ❮ %d/%d ❯*\n", fleetSelfID, s.Manager.Count(), maxPairedSessions()))
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

	s.Reply(info, b.String())
}
