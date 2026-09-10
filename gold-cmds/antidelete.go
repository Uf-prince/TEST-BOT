package goldcmds

// ============================================================================
// GOLD-MD — ANTIDELETE Command  (on / off / inbox / groups / msg here / msg inbox / status)
// File: antidelete.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.antidelete). Text style SAME TO SAME 0% farak.
//
// Two independent settings:
//
//   SCOPE  — WHICH deleted messages to capture:
//     .antidelete on       → enable antidelete (scope = all: inbox + groups)
//     .antidelete inbox    → enable, scope = inbox only
//     .antidelete groups   → enable, scope = groups only
//     .antidelete off      → disable antidelete
//
//   MODE   — WHERE to send the recovered deleted message:
//     .antidelete msg here  → send the deleted message back to the SAME chat/group
//     .antidelete msg inbox → send the deleted message to the bot's PRIVATE inbox (YOU)
//
//   .antidelete             → show current status + scope + mode + full command guide
//
// Persisted in Redis settings:<botJID>:
//   antidelete_enabled = "true"/"false"
//   antidelete_scope   = "all"/"inbox"/"groups"
//   antidelete_mode    = "here"/"inbox"
//
// The actual delete-event capture (resending the deleted message with the
// "🔒 DELETED ... DETECTED 🔒" header) is wired in the main package's
// detectAndHandleAntiDelete (antidelete_handler.go) — it detects a REVOKE
// protocolMessage, looks up the cached original message, and resends it
// either to the same chat (mode=here) or to the bot's own DM (mode=inbox).
// Owner-only.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// antideleteGuide is the shared command guide block (used in status + on replies).
func antideleteGuide(prefix string) string {
	return "*COMMANDS:*\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE ON ⦊* — Enable (all chats)\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE INBOX ⦊* — Inbox only\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE GROUPS ⦊* — Groups only\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE OFF ⦊* — Disable\n\n" +
		"*DELIVERY MODE (where deleted msg is sent):*\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE MSG HERE ⦊* — Send to same chat/group\n" +
		"*TYPE ⦉ " + prefix + "ANTIDELETE MSG INBOX ⦊* — Send to bot's private inbox (YOU)"
}

func handleAntiDelete(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	arg := ""
	if len(args) > 0 {
		arg = strings.ToLower(strings.TrimSpace(args[0]))
	}

	// ── DELIVERY MODE: ".antidelete msg here" / ".antidelete msg inbox" ──
	if arg == "msg" {
		sub := ""
		if len(args) > 1 {
			sub = strings.ToLower(strings.TrimSpace(args[1]))
		}
		if sub == "here" {
			s.SetAntiDeleteMode("here")
			s.Reply(info, "*🔰 ANTIDELETE MODE SET*\n\n*MODE :› HERE*\n\n*🔰 DELETED MESSAGES WILL BE SENT TO THE SAME CHAT/GROUP*\n\n"+antideleteGuide(prefix))
			return
		}
		if sub == "inbox" {
			s.SetAntiDeleteMode("inbox")
			s.Reply(info, "*🔰 ANTIDELETE MODE SET*\n\n*MODE :› INBOX*\n\n*🔰 DELETED MESSAGES WILL BE SENT TO YOUR PRIVATE INBOX (YOU)*\n\n"+antideleteGuide(prefix))
			return
		}
		// ".antidelete msg" with no/invalid sub → show current mode + guide
		curMode := strings.ToUpper(s.GetAntiDeleteMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*🔰 ANTIDELETE MODE*\n\n*MODE :› "+curMode+"*\n\n"+antideleteGuide(prefix))
		return
	}

	if arg == "on" {
		s.SetAntiDeleteSetting(true, "all")
		curMode := strings.ToUpper(s.GetAntiDeleteMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*🔰 ANTIDELETE ENABLED*\n\n*🔰 BOT WILL NOW CAPTURE DELETED MESSAGES*\n\n*SCOPE :› ALL (Inbox + Groups)*\n*MODE :› "+curMode+"*\n\n"+antideleteGuide(prefix))
		return
	}

	if arg == "inbox" {
		s.SetAntiDeleteSetting(true, "inbox")
		curMode := strings.ToUpper(s.GetAntiDeleteMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*🔰 ANTIDELETE ENABLED*\n\n*SCOPE :› INBOX ONLY*\n*MODE :› "+curMode+"*\n\n"+antideleteGuide(prefix))
		return
	}

	if arg == "groups" {
		s.SetAntiDeleteSetting(true, "groups")
		curMode := strings.ToUpper(s.GetAntiDeleteMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*🔰 ANTIDELETE ENABLED*\n\n*SCOPE :› GROUPS ONLY*\n*MODE :› "+curMode+"*\n\n"+antideleteGuide(prefix))
		return
	}

	if arg == "off" {
		s.SetAntiDeleteSetting(false, "all")
		s.Reply(info, "*🔰 ANTIDELETE DISABLED*")
		return
	}

	// status (no arg or unknown)
	st := s.GetAntiDeleteSetting()
	statusStr := "🔰 OFF"
	if st.Enabled {
		statusStr = "🔰 ON"
	}
	scopeStr := strings.ToUpper(st.Scope)
	if scopeStr == "" {
		scopeStr = "ALL"
	}
	modeStr := strings.ToUpper(s.GetAntiDeleteMode())
	if modeStr == "" {
		modeStr = "HERE"
	}
	s.Reply(info, "*🔰 ANTIDELETE STATUS*\n\n*STATUS :› "+statusStr+"*\n*SCOPE :› "+scopeStr+"*\n*MODE :› "+modeStr+"*\n\n"+antideleteGuide(prefix))
}

func init() {
	Register(Command{
		Name:      "antidelete",
		Category:  "ANTI & PROTECTION",
		Desc:      "Resend deleted messages (on/off/inbox/groups; msg here/inbox; no-arg shows status)",
		OwnerOnly: true,
		Run:       handleAntiDelete,
	})
}
