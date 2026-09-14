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
//     .host5gb  → 5GB bandwidth report — OWNER-ONLY command,
//                 bilkul baaki owner commands jaisa (sirf owner / sudo
//                 owner / bot ka apna account). Non-owner: silent ignore
//                 (menu me visible).
//
//   PUBLIC vs PRIVATE split:
//     • Pairing/status data (servers.json ke 200 servers ka /health) →
//       PUBLIC — koi b dekh sakta hai.
//     • LOCAL SESSIONS detail (asli WhatsApp JIDs / phone numbers) →
//       server report me KABHI nahi — private numbers leak nahi hote.
//
//   OFFLINE FORMAT (owner ka exact order): online servers ke full INFO
//   blocks ke baad \n\n\n (3 enter) lagta hai, phir har offline server sirf
//   EK choti line me:  *SERVER ❮ N ❯ OFFLINE*
// ═════════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// isFleetOwnerCommand: .host5gb / .svrchange ka owner-check — bilkul
// handler.go ke owner-check ke SAARE layers (owner order: "dusre cmnds
// jese"):
//  1. cfg.IsOwner(sender)  — GOLDMD_OWNER_NUMBERS + paired owner set
//  2. sender == bot JID    — bot ka apna account (exact JID)
//  3. bare-number match    — bot ka apna number (device-suffix strip,
//     multi-device safe)
//  4. Redis sudo owners    — .ownernumber add / .sudo add wale numbers
//  5. SenderAlt layers     — LID chat me alt form = phone JID (sab
//     upar wale checks alt form pe bhi)
//  6. IsFromMe             — bot owner ke apne device se bheja message
//
// Nil-safe: Manager/cfg/Redis nil ho (tests / edge cases) to sirf layers
// 2/3/5/6 chalete hain — panic kabhi nahi.
func isFleetOwnerCommand(s *Session, info types.MessageInfo) bool {
	sender := ""
	if !info.Sender.IsEmpty() {
		sender = info.Sender.String()
	}

	// 6) bot owner ke apne device se (self-chat / own device)
	if info.IsFromMe {
		return true
	}

	if s == nil {
		return false
	}

	// 2) sender bot ka hi JID hai (exact form)
	if sender != "" && sender == s.JID {
		return true
	}

	// 3) sender bot ka hi number hai (device-suffix tolerant —
	//    bot ke dusre device se bheja ho to bhi owner)
	botNum := botOwnNumber(s.JID)
	if botNum != "" && sender != "" && botOwnNumber(sender) == botNum {
		return true
	}

	// 5a) LID chat: SenderAlt bot ka hi phone JID hai (alt form bina
	//     :device suffix ke, s.JID me suffix ho sakta hai)
	if info.SenderAlt.Server != "" {
		alt := info.SenderAlt.String()
		if alt == s.JID || (botNum != "" && botOwnNumber(alt) == botNum) {
			return true
		}
	}

	// Manager ke bina layers 1/4 nahi chale sakte
	if s.Manager == nil || s.Manager.cfg == nil {
		return false
	}

	// 1) owner set (GOLDMD_OWNER_NUMBERS env + paired owner normalization)
	if sender != "" && s.Manager.cfg.IsOwner(sender) {
		return true
	}

	// 4) Redis sudo owners (.ownernumber add / .sudo add)
	if sender != "" && s.Manager.Redis != nil {
		num := botOwnNumber(sender)
		rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
		if num != "" && rawSudo != "" {
			for _, n := range strings.Split(rawSudo, ",") {
				if strings.TrimSpace(n) == num {
					return true
				}
			}
		}
	}

	// 5b) SenderAlt (LID <-> phone JID mapping) — owner-set + sudo lookup
	if info.SenderAlt.Server != "" {
		alt := info.SenderAlt.String()
		if s.Manager.cfg.IsOwner(alt) {
			return true
		}
		if s.Manager.Redis != nil {
			altNum := botOwnNumber(alt)
			rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
			if altNum != "" && rawSudo != "" {
				for _, n := range strings.Split(rawSudo, ",") {
					if strings.TrimSpace(n) == altNum {
						return true
					}
				}
			}
		}
	}
	return false
}

// hiddenCommands: Commands-map me registered par .menu me KABHI nahi dikhne
// wale secret commands. .host5gb MENU ME VISIBLE hai — owner-only
// command hai. Server-menu family ab PUBLIC hai (koi bhi chala sakta hai) par .menu
// me ab bhi nahi dikhti (secret rahegi, sirf wahi jaanne wale use karenge).
var hiddenCommands = map[string]bool{
	"servers":    true,
	"svr":        true,
	"svrinfo":    true,
	"serverinfo": true,
	"session":    true,
	"sessions":   true,
	"svrchange":  true, // git-token command — hidden (owner-only)
}

func init() {
	// OWNER-ONLY: .host5gb / .svrchange handler.go ke ownerOnlyCommands
	// set me bhi — non-owner ke liye silently ignored, bilkul baaki
	// owner-only commands jaisa (in-guard owner-check ke saath double lock).
	ownerOnlyCommands["host5gb"] = true
	ownerOnlyCommands["svrchange"] = true

	// .host5gb — bandwidth report (MENU VISIBLE, owner-only).
	RegisterCommand("host5gb", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		if !isFleetOwnerCommand(s, info) {
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

	// .svrchange — git-token servers.json update (owner-only, hidden).
	// GitHub + GitLab dono repos me server links badal ke push — Render
	// auto-deploy foran trigger hota hai (owner ko git pe jana nahi prega).
	RegisterCommand("svrchange", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		if !isFleetOwnerCommand(s, info) {
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

	// OWNER ORDER: LOCAL SESSIONS / THIS SERVER block REMOVED — server
	// report me koi bhi private JID / local session detail NAHI dikhega
	// (sirf fleet-wide pairing counts).

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
