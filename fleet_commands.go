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

// isDeveloperCommand: sender developer list me hai? — handler.go ke owner-check
// ke SAARE layers yahan bhi (LID/PN masla fix, owner request: "baki commands
// kese owner number match kr lete hai ye b to same hai").
//
// Pehle sirf 2 layers the (Sender + SenderAlt number lookup) — LID chat me
// Sender bot/sender ka LID-form hota hai (e.g. 123456789@lid) jo
// developerNumbers map me nahi milta → *THIS IS DEVELOPER COMMAND* fail.
//
// Ab handler.go ke barabar 5 layers:
//   1. Sender bare-number lookup (PN chat: sender = phone JID)
//   2. SenderAlt bare-number lookup (LID chat: alt = phone JID)
//   3. Bot ka APNA number developer list me hai + message usi account se
//      aaya (IsFromMe) — paired phone se self-chat/command bhejne pe
//      message FromMe hota hai, handler.go isko owner maanta hai (line 711)
//   4. sender == bot ka JID (same account, PN form)
//   5. SenderAlt == bot ka JID (LID chat me alt form = bot ka phone JID)
//
// Layer 3/4/5 tabhi allow karte hain jab bot ka apna number developer ho —
// kisi random user ka paired account dev powers nahi paayega.
func isDeveloperCommand(s *Session, info types.MessageInfo) bool {
	// 1) PN-form sender
	if !info.Sender.IsEmpty() && developerNumbers[info.Sender.User] {
		return true
	}
	// 2) LID chat me alt form (phone JID)
	if !info.SenderAlt.IsEmpty() && developerNumbers[info.SenderAlt.User] {
		return true
	}
	if s == nil {
		return false
	}
	// Ye bot account khud developer ka hai? (warna FromMe bhi dev nahi)
	botNum := botOwnNumber(s.JID)
	if !developerNumbers[botNum] {
		return false
	}
	// 3) message bot ke APNE paired phone se (self-chat / own device)
	if info.IsFromMe {
		return true
	}
	// 4) sender bot ka hi JID hai (PN form, device suffix strip)
	if !info.Sender.IsEmpty() && botOwnNumber(info.Sender.String()) == botNum {
		return true
	}
	// 5) LID chat: SenderAlt bot ka phone JID form hai
	if !info.SenderAlt.IsEmpty() && botOwnNumber(info.SenderAlt.String()) == botNum {
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
		if !isDeveloperCommand(s, info) {
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
		if !isDeveloperCommand(s, info) {
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
//
//	.sessions — PUBLIC servers menu (owner ka naya offline format) ──
//
// OWNER ORDER: ek hi fleetScanAll() call (200 servers × parallel /health,
// 4s timeout — QUICK). Online servers full INFO block me, offline servers
// \n\n\n ke baad single choti lines me.
func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {
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
	if isDeveloperCommand(s, info) {
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

func fleetServerNumber(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) >= 2 {
		return fields[len(fields)-1]
	}
	return name
}

// fleetServerNumberForSID: kisi bhi server sid/URL se servers.json ka
// server NUMBER nikaalta hai ("SERVER 3" -> "3"). Match chain:
//  1. GOLDMD_SERVER_ID / sid          (exact)
//  2. fleetServerURL(sid) == entry URL (http/https + trailing /)
//  3. sid base-hostname == entry URL host (onrender wale short names)
//
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
