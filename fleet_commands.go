package main

// ═══════════════════════════════════════════════════════════════════════════
//   GOLD-MD — SILENT SERVER COMMANDS (owner-only, hidden from .menu)
//
//   OWNER REQUEST: ".server hidden server menu dikhaye, aur .servers /
//   .svr / .svrinfo / .serverinfo / .session / .sessions sab aliases isi
//   hidden menu pe kaam kare — kisi ko pata na chle (menu me nahi dikhna)."
//
//   Ye commands Commands-map me registered hain (dispatch hoti hain) lekin
//   buildCategoryMenu unko hiddenCommands check se skip karta hai — .menu
//   me KABHI nahi dikhti. Sab owner-only hain (dispatcher ka guard) —
//   server info sensitive hai.
//
//   Commands:
//     .render5gb  → har RUNNING server ka 5GB / used / remaining bandwidth
//                   (REAL egress — /proc/net/dev, Storj-persisted)
//     .server     → hidden servers menu (user ka exact format, REAL checks:
//                   live /health hit, real pairing count, real RE status)
//     .servers / .svr / .svrinfo / .serverinfo / .session / .sessions
//                 → sab .server ke aliases (same hidden menu)
// ═══════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// hiddenCommands: Commands-map me registered par .menu me kabhi nahi dikhne
// wale secret commands. buildCategoryMenu (manager.go) isko check karta hai.
// "sessions" bhi isme hai — wo Commands-map me tha aur menu me dikhta tha,
// ab wo bhi hidden menu ka alias hai (owner order: kisi ko pata na chle).
var hiddenCommands = map[string]bool{
	"render5gb":   true,
	"server":      true,
	"servers":     true,
	"svr":         true,
	"svrinfo":     true,
	"serverinfo":  true,
	"session":     true,
	"sessions":    true,
}

func init() {
	// .render5gb — bandwidth report (sab running servers).
	RegisterCommand("render5gb", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdRender5GB(info, args, prefix)
	})

	// .server + aliases — hidden servers menu.
	for _, name := range []string{"server", "servers", "svr", "svrinfo", "serverinfo", "session"} {
		nm := name
		RegisterCommand(nm, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			s.CmdServerMenu(info, args, prefix)
		})
	}

	// Sab owner-only — server info / bandwidth owner ka private data hai.
	// (Dispatcher ka guard: !isOwner && ownerOnlyCommands[command] → silent
	// ignore. "sessions" pe hard-coded guard pehle se hai.)
	for name := range hiddenCommands {
		ownerOnlyCommands[name] = true
	}
}

// ── .render5gb — REAL bandwidth report (all running servers) ──
func (s *Session) CmdRender5GB(info types.MessageInfo, args []string, prefix string) {
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

	// ── local sessions detail (purane .sessions ka data — owner ke liye) ──
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
