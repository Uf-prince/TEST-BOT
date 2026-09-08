package main

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ===========================================================================
//   GOLD-MD — Owner helper commands:  sessions
//
//   The core status commands (alive, ping, menu, uptime) now live in
//   manager.go (the "main file") and use the forwarded newsletter channel
//   link button via ReplyWithNewsletter.
// ===========================================================================

// ── SESSIONS (owner) ──────────────────────────────────────────────────────
func (s *Session) CmdSessions(info types.MessageInfo, args []string, prefix string) {
	sessions := s.Manager.List()
	if len(sessions) == 0 {
		s.Reply(info, "📭 No active sessions.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📡 *Active Sessions (%d):*\n\n", len(sessions)))
	for i, sess := range sessions {
		status := "🔴 disconnected"
		if sess.Client != nil && sess.Client.IsConnected() {
			status = "🟢 online"
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s | %s\n", i+1,
			sess.JID, status, formatUptime(time.Since(sess.Started))))
	}
	s.Reply(info, sb.String())
}
