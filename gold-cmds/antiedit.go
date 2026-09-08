package goldcmds

// ============================================================================
// GOLD-MD — ANTIEDIT Command  (on / off / inbox / groups / msg here / msg inbox / status)
// File: antiedit.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.antiedit / .editmsg / .antieditmsg).
// Text style SAME TO SAME 0% farak.
//
// Two independent settings:
//
//   SCOPE  — WHICH edited messages to capture:
//     .antiedit on       → enable antiedit (scope = all: inbox + groups)
//     .antiedit inbox    → enable, scope = inbox only
//     .antiedit groups   → enable, scope = groups only
//     .antiedit off      → disable antiedit
//
//   MODE   — WHERE to send the edited-message alert (OLD + NEW text):
//     .antiedit msg here  → send the alert to the SAME chat/group
//     .antiedit msg inbox → send the alert to the bot's PRIVATE inbox (YOU)
//
//   .antiedit             → show current status + scope + mode + full command guide
//
// Persisted in Redis settings:<botJID>:
//   antiedit_enabled = "true"/"false"
//   antiedit_scope   = "all"/"inbox"/"groups"
//   antiedit_mode    = "here"/"inbox"
//
// The actual edit-event capture (showing OLD + NEW text with the
// "✏️ EDITED ... DETECTED ✏️" header) is wired in the main package's
// detectAndHandleAntiEdit (antidelete_handler.go) — it detects an edited
// message (evt.IsEdit), looks up the cached original message text, and sends
// the alert either to the same chat (mode=here) or to the bot's own DM
// (mode=inbox). Owner-only.
//
// Aliases (same as Node.js): antiedit, editmsg, antieditmsg
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// antieditGuide is the shared command guide block (used in status + on replies).
func antieditGuide(prefix string) string {
	return "*COMMANDS:*\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT ON ⦊* — Enable (all chats)\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT INBOX ⦊* — Inbox only\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT GROUPS ⦊* — Groups only\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT OFF ⦊* — Disable\n\n" +
		"*DELIVERY MODE (where edited msg alert is sent):*\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT MSG HERE ⦊* — Send to same chat/group\n" +
		"*TYPE ⦉ " + prefix + "ANTIEDIT MSG INBOX ⦊* — Send to bot's private inbox (YOU)"
}

func handleAntiEdit(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// panic guard
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()

	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	arg := ""
	if len(args) > 0 {
		arg = strings.ToLower(strings.TrimSpace(args[0]))
	}

	// ── DELIVERY MODE: ".antiedit msg here" / ".antiedit msg inbox" ──
	if arg == "msg" {
		sub := ""
		if len(args) > 1 {
			sub = strings.ToLower(strings.TrimSpace(args[1]))
		}
		if sub == "here" {
			s.SetAntiEditMode("here")
			s.Reply(info, "*✅ ANTIEDIT MODE SET*\n\n*MODE :› HERE*\n\n*✏️ EDITED MESSAGE ALERTS WILL BE SENT TO THE SAME CHAT/GROUP*\n\n"+antieditGuide(prefix))
			return
		}
		if sub == "inbox" {
			s.SetAntiEditMode("inbox")
			s.Reply(info, "*✅ ANTIEDIT MODE SET*\n\n*MODE :› INBOX*\n\n*✏️ EDITED MESSAGE ALERTS WILL BE SENT TO YOUR PRIVATE INBOX (YOU)*\n\n"+antieditGuide(prefix))
			return
		}
		// ".antiedit msg" with no/invalid sub → show current mode + guide
		curMode := strings.ToUpper(s.GetAntiEditMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*✏️ ANTIEDIT MODE*\n\n*MODE :› "+curMode+"*\n\n"+antieditGuide(prefix))
		return
	}

	if arg == "on" {
		s.SetAntiEditSetting(true, "all")
		curMode := strings.ToUpper(s.GetAntiEditMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*✅ ANTIEDIT ENABLED*\n\n*✏️ BOT WILL NOW CAPTURE EDITED MESSAGES*\n\n*SCOPE :› ALL (Inbox + Groups)*\n*MODE :› "+curMode+"*\n\n"+antieditGuide(prefix))
		return
	}

	if arg == "inbox" {
		s.SetAntiEditSetting(true, "inbox")
		curMode := strings.ToUpper(s.GetAntiEditMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*✅ ANTIEDIT ENABLED*\n\n*SCOPE :› INBOX ONLY*\n*MODE :› "+curMode+"*\n\n"+antieditGuide(prefix))
		return
	}

	if arg == "groups" {
		s.SetAntiEditSetting(true, "groups")
		curMode := strings.ToUpper(s.GetAntiEditMode())
		if curMode == "" {
			curMode = "HERE"
		}
		s.Reply(info, "*✅ ANTIEDIT ENABLED*\n\n*SCOPE :› GROUPS ONLY*\n*MODE :› "+curMode+"*\n\n"+antieditGuide(prefix))
		return
	}

	if arg == "off" {
		s.SetAntiEditSetting(false, "all")
		s.Reply(info, "*❌ ANTIEDIT DISABLED*")
		return
	}

	// status (no arg or unknown)
	st := s.GetAntiEditSetting()
	statusStr := "❌ OFF"
	if st.Enabled {
		statusStr = "✅ ON"
	}
	scopeStr := strings.ToUpper(st.Scope)
	if scopeStr == "" {
		scopeStr = "ALL"
	}
	modeStr := strings.ToUpper(s.GetAntiEditMode())
	if modeStr == "" {
		modeStr = "HERE"
	}
	s.Reply(info, "*✏️ ANTIEDIT STATUS*\n\n*STATUS :› "+statusStr+"*\n*SCOPE :› "+scopeStr+"*\n*MODE :› "+modeStr+"*\n\n"+antieditGuide(prefix))
}

func init() {
	Register(Command{
		Name:      "antiedit",
		Category:  "ANTI & PROTECTION",
		Desc:      "Show original text when a message is edited (on/off/inbox/groups; msg here/inbox)",
		OwnerOnly: true,
		Run:       handleAntiEdit,
	})
	// hidden aliases (same as Node.js)
	Register(Command{Name: "editmsg", OwnerOnly: true, Hidden: true, Run: handleAntiEdit})
	Register(Command{Name: "antieditmsg", OwnerOnly: true, Hidden: true, Run: handleAntiEdit})
}
