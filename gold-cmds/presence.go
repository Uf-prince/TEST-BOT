package goldcmds

// ============================================================================
// GOLD-MD — Presence commands: .alwaysonline, .typing, .recording
//
// Ported from UMAR-MD (Node.js pair.js) — same text style, same behaviour:
//   .alwaysonline on/off  → bot number shows always online / real last seen
//   .typing on/off        → bot shows auto typing when someone messages
//   .recording on/off     → bot shows auto recording when someone messages
//
// Per-user config stored in Redis (Upstash) via settings:<botJID> hash:
//   field "alwaysonline"   = "true"/"false"
//   field "autotyping"     = "true"/"false"
//   field "autorecording"  = "true"/"false"
//
// Conflict rule (same as Node.js): typing and recording cannot both be ON.
// If recording is ON and you try .typing on → error message tells you to
// first turn recording off. And vice versa.
//
// The actual presence updates (sendPresence / sendChatPresence) on every
// incoming message are applied in handler.go (auto-online/typing/recording
// handlers), exactly like pair.js lines 18660-18860.
// ============================================================================

import (
	"go.mau.fi/whatsmeow/types"
	"strings"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// isOn reads a presence setting from Redis and returns true/false.
func isOn(s SessionBridge, field string) bool {
	v := s.GetPresenceSetting(field, "false")
	return v == "true" || v == "1" || v == "on"
}

// setOn writes a presence setting to Redis as "true" or "false".
func setOn(s SessionBridge, field string, on bool) {
	if on {
		s.SetPresenceSetting(field, "true")
	} else {
		s.SetPresenceSetting(field, "false")
	}
}

// ownerGate checks if the sender is the owner; replies with the Node.js-style
// rejection message if not.
func ownerGate(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
	return false
}

// ── ALWAYS ONLINE ────────────────────────────────────────────────────────────
// Aliases (Node.js): online, alwaysonline, aon
// Go aliases (5+): always on, aonline, alwayson, onlineon, gconline, 24x7

func handleAlwaysOnline(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAlwaysOnlineAsync(s, info, args, prefix)
}

func handleAlwaysOnlineAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		current := isOn(s, "alwaysonline")
		status := "OFF"
		if current {
			status = "ON"
		}
		s.Reply(info, "*🔰 ALWAYS ONLINE INFO 🔰*\n\n*TYPE ❬ "+prefix+"ONLINE ON ❭*\n*BOT NUMBER WILL BE SHOW ALWAYS ONLINE*\n\n*TYPE ❬ "+prefix+"ONLINE OFF ❭*\n*BOT NUMBER WILL BE SHOW REAL LAST SEEN OR ONLINE*\n\n*CURRENT STATUS :❭ ❬ "+status+" ❭*")
		return
	}
	arg := strings.ToLower(strings.TrimSpace(args[0]))
	if arg == "on" {
		setOn(s, "alwaysonline", true)
		_ = s.SendPresenceUpdate("available")
		s.Reply(info, "*🔰 ALWAYS ONLINE ACTIVATED 🔰*\n\n*NOW BOT NUMBER SHOW ALWASY ONLINE*")
		return
	}
	if arg == "off" {
		setOn(s, "alwaysonline", false)
		_ = s.SendPresenceUpdate("unavailable")
		s.Reply(info, "*🔰 ALWAYS ONLINE DE-ACTIVATED 🔰*\n\n*NOW BOT NUMBER WILL BE SHOW REAL LAST SEEN OR ONLINE*")
		return
	}
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❬ ONLINE ❭ FOR HELP*")
}

// ── AUTO TYPING ──────────────────────────────────────────────────────────────
// Aliases (Node.js): autotyping, typing, at
// Go aliases (5+): autotype, typingon, showtyping, gctyping, auto_type, typemode

func handleAutoTyping(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutoTypingAsync(s, info, args, prefix)
}

func handleAutoTypingAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		current := isOn(s, "autotyping")
		status := "OFF"
		if current {
			status = "ON"
		}
		s.Reply(info, "*🔰 AUTO TYPING INFO 🔰*\n\n*TYPE ❬ "+prefix+"TYPING ON ❭*\n*TO ACTIVATE AUTOTYPING AND BOT WILL BE SHOW AUTO TYPING WHEN SOMEONE MSG YOU*\n\n*TYPE ❬ "+prefix+"TYPING OFF ❭*\n*TO STOP SHOWING AUTO TYPING*\n\n*CURRENT STATUS :❭ "+status+"*")
		return
	}
	arg := strings.ToLower(strings.TrimSpace(args[0]))
	if arg == "on" {
		// 🔥 CONFLICT CHECK: If AUTO_RECORDING is ON, tell user to turn it off first
		if isOn(s, "autorecording") {
			s.Reply(info, "*🔰 AUTO RECORDING IS ON 🔰*\n\n*FIRST TYPE*\n*TYPE ❬ "+prefix+"RECORDING OFF ❭*\n*AFTER ❬ RECORDING ❭ OFF*\n\n*TYPE AGAIN*\n*TYPE ❬ "+prefix+"RECORDING ON ❭*\n*TO ACTIVATE AUTO TYPING*")
			return
		}
		setOn(s, "autotyping", true)
		s.Reply(info, "*🔰 AUTO TYPING ACTIVATED 🔰*\n\n*NOW BOT SHOW AUTO TYPING WHEN SOMEONE MESSAGES 😎*")
		return
	}
	if arg == "off" {
		setOn(s, "autotyping", false)
		s.Reply(info, "*🔰 AUTO TYPING DE-ACTIVATED 🔰*\n\n*NOW BOT DOESN'T SHOW AUTO TYPING ANYMORE*")
		return
	}
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❬ TYPING ❭ FOR HELP*")
}

// ── AUTO RECORDING ───────────────────────────────────────────────────────────
// Aliases (Node.js): autorecording, recording, ar
// Go aliases (5+): autorecord, recordingon, showrecording, gcrecording, auto_record, recordmode

func handleAutoRecording(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutoRecordingAsync(s, info, args, prefix)
}

func handleAutoRecordingAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		current := isOn(s, "autorecording")
		status := "OFF"
		if current {
			status = "ON"
		}
		s.Reply(info, "*🔰 AUTO RECORDING INFO 🔰*\n\n*TYPE ❬ "+prefix+"RECORDING ON ❭*\n*WHEN SOMEONE MSG YOU AND YOUR NUMBER SHOW RECORDING....*\n\n*TYPE ❬ "+prefix+"RECORDING OFF ❭*\n*TO STOP SHOWING AUTO RECORDING...*\n\n*CURRENT STATUS :❭ "+status+"*")
		return
	}
	arg := strings.ToLower(strings.TrimSpace(args[0]))
	if arg == "on" {
		// 🔥 CONFLICT CHECK: If AUTO_TYPING is ON, tell user to turn it off first
		if isOn(s, "autotyping") {
			s.Reply(info, "*🔰 AUTO TYPING IS ON 🔰*\n\n*FIRST TYPE*\n*TYPE ❬ "+prefix+"TYPING OFF ❭*\n*AFTER ❬ TYPING ❭ OFF*\n\n*TYPE AGAIN*\n*TYPE ❬ "+prefix+"RECORDING ON ❭*\n*TO ACTIVATE AUTO RECORDING*")
			return
		}
		setOn(s, "autorecording", true)
		s.Reply(info, "*🔰 AUTO RECORDING ACTIVATED 🔰*\n\n*BOT WILL NOW SHOW AUTO RECORDING WHEN SOMEONE MESSAGES 😎*")
		return
	}
	if arg == "off" {
		setOn(s, "autorecording", false)
		s.Reply(info, "*🔰 AUTO RECORDING DE-ACTIVATED 🔰*\n\n*BOT WILL NOT SHOW AUTO RECORDING ANYMORE 😊*")
		return
	}
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❬ RECORDING ❭ FOR HELP*")
}

// ── registration ─────────────────────────────────────────────────────────────

func init() {
	// ── primary commands ──────────────────────────────────────────────────
	Register(Command{Name: "online", Category: "PRESENCE & STATUS", Desc: "Toggle always-online presence", OwnerOnly: true, Run: handleAlwaysOnline})
	Register(Command{Name: "typing", Category: "PRESENCE & STATUS", Desc: "Toggle auto typing presence", OwnerOnly: true, Run: handleAutoTyping})
	Register(Command{Name: "recording", Category: "PRESENCE & STATUS", Desc: "Toggle auto recording presence", OwnerOnly: true, Run: handleAutoRecording})

	// ── alwaysonline aliases (5+) ──────────────────────────────────────────
	Register(Command{Name: "alwaysonline", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})
	Register(Command{Name: "aon", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})
	Register(Command{Name: "alwayson", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})
	Register(Command{Name: "aonline", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})
	Register(Command{Name: "onlineon", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})
	Register(Command{Name: "24x7", OwnerOnly: true, Hidden: true, Run: handleAlwaysOnline})

	// ── typing aliases (5+) ────────────────────────────────────────────────
	Register(Command{Name: "autotyping", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "at", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "autotype", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "typingon", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "showtyping", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "gctyping", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})
	Register(Command{Name: "typemode", OwnerOnly: true, Hidden: true, Run: handleAutoTyping})

	// ── recording aliases (5+) ─────────────────────────────────────────────
	Register(Command{Name: "autorecording", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "ar", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "autorecord", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "recordingon", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "showrecording", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "gcrecording", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
	Register(Command{Name: "recordmode", OwnerOnly: true, Hidden: true, Run: handleAutoRecording})
}
