package goldcmds

// ============================================================================
// GOLD-MD — .bangc / .unbangc commands
//
// Ported from UMAR-MD (Node.js pair.js, lines 16285-16370) — SAME TEXT
// (0% farak), SAME WORK (0% farak):
//   .bangc    → lock the entire group (no one can use bot commands)
//   .unbangc  → unlock the group (everyone can use bot commands again)
//
// Aliases: .gcbotoff / .bangroup / .groupban / .gcban → bangc
//          .gcboton → unbangc
//
// Owner-only, group-only. Per-group setting in Redis (settings:<groupJID>):
//   field "bangc" = "on" (locked) / "off" (open)
//
// The enforcement (blocking commands in a locked group) is in handler.go.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"go.mau.fi/whatsmeow/types"
)

const bangcFeature = "bangc"

// bangcIsOn reports whether the group is locked (bot commands disabled).
func bangcIsOn(s SessionBridge, groupJID string) bool {
	v := s.GetGroupSetting(groupJID, bangcFeature, "off")
	return v == "on" || v == "true" || v == "1"
}

// bangcSetOn locks the group (disables bot commands for everyone).
func bangcSetOn(s SessionBridge, groupJID string) {
	s.SetGroupSetting(groupJID, bangcFeature, "on")
}

// bangcSetOff unlocks the group (enables bot commands for everyone).
func bangcSetOff(s SessionBridge, groupJID string) {
	s.SetGroupSetting(groupJID, bangcFeature, "off")
}

// ── bangc (lock group) ────────────────────────────────────────────────────

func handleBangc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBangcAsync(s, info, args, prefix)
}

func handleBangcAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Owner-only check
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎**")
		return
	}

	// Group-only check
	if !info.IsGroup {
		s.Reply(info, "*USE THIS COMMAND IN GROUPS ONLY*")
		return
	}

	groupJID := info.Chat.String()

	// Check if already banned (locked)
	if bangcIsOn(s, groupJID) {
		s.Reply(info, "*THIS GROUP IS ALREADY TURNED OFF*")
		return
	}

	// Lock the group
	bangcSetOn(s, groupJID)
	s.Reply(info, "*🔰 GROUP TURNED OFF SUCCESS 🔰*\n\n*NOW IN THIS GROUP NO ONE INCLUDING ❬ ADMINS + MEMBERS ❭*\n*CAN USE MY BOT  COMMANDS 😎*")
}

// ── unbangc (unlock group) ────────────────────────────────────────────────

func handleUnbangc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleUnbangcAsync(s, info, args, prefix)
}

func handleUnbangcAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Owner-only check
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	// Group-only check
	if !info.IsGroup {
		s.Reply(info, "*USE THIS COMMAND IN GROUPS ONLY*")
		return
	}

	groupJID := info.Chat.String()

	// Check if already unbanned (unlocked)
	if !bangcIsOn(s, groupJID) {
		s.Reply(info, "*THIS GROUP IS ALREADY TURNED ON 🔰*")
		return
	}

	// Unlock the group
	bangcSetOff(s, groupJID)
	s.Reply(info, "*🔰 GROUP TURNED ON SUCCESS 🔰*\n\n*NOW IN THIS GROUP EVERYONE (ADMINS + MEMBERS)*\n*CAN USE MY BOT COMMANDS 🔰*")
}

// ── exported helper for handler.go ─────────────────────────────────────────

// BangcIsOn reports whether the group is locked (bot commands disabled).
// Used by handler.go enforcement to block all commands in a locked group.
func BangcIsOn(s SessionBridge, groupJID string) bool {
	return bangcIsOn(s, groupJID)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "bangc", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO TURN OFF THE BOT IN ALL GROUPS AT ONCE.", OwnerOnly: true, Run: handleBangc})
	Register(Command{Name: "gcbotoff", OwnerOnly: true, Hidden: true, Run: handleBangc})
	Register(Command{Name: "bangroup", OwnerOnly: true, Hidden: true, Run: handleBangc})
	Register(Command{Name: "groupban", OwnerOnly: true, Hidden: true, Run: handleBangc})
	Register(Command{Name: "gcban", OwnerOnly: true, Hidden: true, Run: handleBangc})

	Register(Command{Name: "unbangc", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO TURN ON THE BOT IN ALL GROUPS AGAIN.", OwnerOnly: true, Run: handleUnbangc})
	Register(Command{Name: "gcboton", OwnerOnly: true, Hidden: true, Run: handleUnbangc})
}
