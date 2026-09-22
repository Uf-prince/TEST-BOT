package goldcmds

// ============================================================================
// GOLD-MD — ANTICALL Command  (on / off / status / msg)
// File: anticall.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.anticall / .acall / .antical / .rejectcall).
// Text style SAME TO SAME 0% farak. Work is identical to the Node bot:
//
//   .anticall          → show current status + command guide
//   .anticall on       → enable anticall (bot auto-rejects incoming calls)
//   .anticall off      → disable anticall
//   .anticall msg      → show current custom reject-call message + guide
//   .anticall msg <text> → set custom reject-call message
//
// Persisted in Redis settings:<botJID>:
//   anticall_enabled = "true"/"false"
//   anticall_msg     = custom message text (deleted if empty/reset)
//
// The actual call-rejection (rejecting the call + sending the custom msg
// to the caller) is wired in the main package's EventHandler (manager.go)
// — it listens for *events.CallOffer, checks the anticall setting, rejects
// the call via Client.RejectCall, and sends the custom/default message.
//
// Aliases (same as Node.js): anticall, acall, antical, rejectcall
// Crown emoji 👑 replaced with 🔰 as per GOLD-MD convention.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ANTICALL_DEFAULT_MSG is the default message sent to a caller when their
// call is auto-rejected and no custom message is set. Same as Node.js
// ANTICALL_DEFAULT_MSG = '*PLEASE WAIT....!!!*'.
const ANTICALL_DEFAULT_MSG = "*PLEASE WAIT....!!!*"

func handleAntiCall(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only — same as Node.js: *THIS COMMAND IS ONLY FOR ME 😎*
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	arg := ""
	if len(args) > 0 {
		arg = strings.ToLower(strings.TrimSpace(args[0]))
	}

	// ── No args → status dikhao (same as Node.js) ──
	if arg == "" {
		currentStatus := s.GetAntiCallSetting()
		statusStr := "OFF"
		if currentStatus {
			statusStr = "ON"
		}
		s.Reply(info, "*🔰 ANTI CALL FUNCTION INFO 🔰*\n\n*TYPE ❮ "+prefix+"ANTICALL ON ❯*\n\n*IF YOU TURN THIS ON THEN WHOEVER CALLS YOU BOT WILL REJECT CALLS AUTOMATICALLY*\n\n*TYPE ❮ "+prefix+"ANTICALL OFF ❯*\n\n*THIS WILL TURN OFF THE ANTICALL SYSTEM BOT WILL NEVER REJECT CALLS AUTOMATICALLY*\n\n\n*NOW ANTICALL IS ❮ "+statusStr+" ❯*\n\n*TYPE ❮ "+prefix+"ANTICALLPREM ❯ FOR INFO*\n*TYPE ❮ "+prefix+"ANTICALLPREM ADD ❯ FOR INFO*\n*TYPE ❮ "+prefix+"ANTICALLPREM DEL ❯ FOR INFO*\n*TYPE ❮ "+prefix+"ANTICALLPREM LIST ❯ FOR INFO*\n*TYPE ❮ "+prefix+"ANTICALL MSG ❯ FOR INFO*")
		return
	}

	// ── ON ──
	if arg == "on" {
		s.SetAntiCallSetting(true)
		s.Reply(info, "*🔰 ANTICALL ACTIVATED 🔰*\n\n*NOW IF ANYONE CALLS BOT  WILL REJECT CALLS AUTOMATICALLY*")
		return
	}

	// ── OFF ──
	if arg == "off" {
		s.SetAntiCallSetting(false)
		s.Reply(info, "*ANTICALL DE-ACTIVATED*\n*NOW IF ANYONE CALLS BOT WILL NEVER REJECT AUTOMATICALLY*")
		return
	}

	// ── MSG subcommand ── .anticall msg <new msg> — custom handler msg
	// (same as Node.js: acArgs[0] is "msg", rest is the new message text)
	if arg == "msg" {
		// Extract the raw message text after "msg" — we need the original
		// case/spacing, not the lowercased args. Reconstruct from args[1:].
		// But args are already split + trimmed. For the msg subcommand we
		// need to get the original text. The handler passes args as the
		// split args, so we join args[1:] back.
		// However, the Node.js code uses acCmdMatch[2] (raw rest text) to
		// preserve original spacing. In our Go command framework, args is
		// already split. We join them back with spaces.
		newMsg := ""
		if len(args) > 1 {
			newMsg = strings.TrimSpace(strings.Join(args[1:], " "))
		}

		if newMsg == "" {
			// Show current msg + guide
			currentMsg := s.GetAntiCallMessage(ANTICALL_DEFAULT_MSG)
			s.Reply(info, "*🔰 ANTICALL MESSAGE CHANGE GUIDE 🔰*\n\n*WHEN THE CALLS REJECTED AUTOMATICALLY THE BOT SEND THIS MSG IS*\n\n"+currentMsg+"\n\n*DO YOU WANT TO CHANGE THIS MSG ?*\n*TYPE ❮ "+prefix+"ANTICALL MSG ❮ NEW MSG ❯ ❯*\n*EXAMPLE LIKE THIS*\n*❮ "+prefix+"ANTICALL MSG NO CALLS PLZ ❯*\n*❮ "+prefix+"ANTICALL MSG DON'T CALL ME ❯*\n*❮ "+prefix+"ANTICALL MSG I AM BUSSY WAIT ❯*\n*❮ "+prefix+"ANTICALL MSG CALLS NOT ALLOWED ❯*\n*❮ "+prefix+"ANTICALL MSG ❮ UR OWN MSG ❯ ❯*\n\n*WHEN YOU SET YOUR OWN MSG AND WHEN THE BOT WILL REJECT THE CALLS BOT WILL SEND THIS  YOUR CUSTUM MSG*")
			return
		}

		s.SetAntiCallMessage(newMsg)
		s.Reply(info, "*🔰 ANTICALL MESSAGE CHANGED 🔰*\n\n*YOUR NEW MESSAGE IS*\n"+newMsg)
		return
	}

	// ── Invalid arg ──
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❮ ANTICALL ❯ FOR HELP*")
}

func init() {
	Register(Command{
		Name:      "anticall",
		Category:  "ANTI & PROTECTION",
		Desc:      "THIS COMMAND IS USED TO AUTO REJECT INCOMING CALLS ON THE BOT. IT CAN ALSO SEND A CUSTOM MESSAGE TO THE CALLER.",
		OwnerOnly: true,
		Run:       handleAntiCall,
	})
	// hidden aliases (same as Node.js: acall, antical, rejectcall)
	Register(Command{Name: "acall", OwnerOnly: true, Hidden: true, Run: handleAntiCall})
	Register(Command{Name: "antical", OwnerOnly: true, Hidden: true, Run: handleAntiCall})
	Register(Command{Name: "rejectcall", OwnerOnly: true, Hidden: true, Run: handleAntiCall})
}
