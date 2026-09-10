package goldcmds

// ============================================================================
// GOLD-MD — .autoread command (aliases: .readmsg / .autoreadmsg)
//
// Ported from UMAR-MD (Node.js pair.js, lines 12942-13075) — SAME TEXT
// (0% farak), SAME WORK (0% farak):
//   .autoread          → show info + current status
//   .autoread on       → if off, show mode selection (inbox/groups/all)
//   .autoread inbox    → auto-read only private/inbox messages
//   .autoread groups   → auto-read only group messages
//   .autoread all      → auto-read all messages (inbox + groups)
//   .autoread off      → stop auto-reading
//
// Owner-only. Bot-wide setting stored in Redis settings:<botJID> hash,
// field "autoread". Values: "off" / "inbox" / "groups" / "all".
//
// The actual message-read enforcement is in handler.go (applyAutoRead),
// which calls MarkRead based on the mode.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handleAutoRead(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutoReadAsync(s, info, args, prefix)
}

func handleAutoReadAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	currentMode := s.GetAutoReadSetting("off")

	// No args → show info
	if len(args) == 0 {
		modeDisplay := "OFF 🔰"
		switch currentMode {
		case "inbox":
			modeDisplay = "INBOX 🔰"
		case "groups":
			modeDisplay = "GROUPS 🔰"
		case "all":
			modeDisplay = "ALL 🔰"
		}
		s.Reply(info, "*🔰 AUTO READ INFO 🔰*\n\n*TYPE ❬ "+prefix+"AUTOREAD ON ❭*\n*TYPE FIRST THEN BOT WILL TELL YOU MODES*\n*AUTOREAD IS CURRENTLY OFF - TURN ON FIRST*\n\n*TYPE ❬ "+prefix+"AUTOREAD INBOX ❭*\n*BOT WILL AUTO READ ONLY PRIVATE/INBOX MESSAGES*\n\n*TYPE ❬ "+prefix+"AUTOREAD GROUPS ❭*\n*BOT WILL AUTO READ ONLY GROUP MESSAGES*\n\n*TYPE ❬ "+prefix+"AUTOREAD ALL ❭*\n*BOT WILL AUTO READ ALL MESSAGES (INBOX + GROUPS)*\n\n*TYPE ❬ "+prefix+"AUTOREAD OFF ❭*\n*STOP AUTO READING ALL MESSAGES*\n\n*CURRENT STATUS ❯ "+modeDisplay+"*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	// .autoread on — sirf batao modes, actually on nahi karo
	if subCmd == "on" {
		if currentMode != "off" {
			s.Reply(info, "*🔰 AUTO READ ALREADY ACTIVE 🔰*\n\n*CURRENT MODE ❯ "+strings.ToUpper(currentMode)+"*\n\n*TO CHANGE MODE TYPE:*\n*TYPE ❬ "+prefix+"AUTOREAD INBOX ❭*\n*TYPE ❬ "+prefix+"AUTOREAD GROUPS ❭*\n*TYPE ❬ "+prefix+"AUTOREAD ALL ❭*\n\n*TYPE ❬ "+prefix+"AUTOREAD OFF ❭*\n*TO TURN OFF*")
			return
		}
		s.Reply(info, "*🔰 AUTO READ - SELECT MODE 🔰*\n\n*AUTOREAD IS OFF — FIRST SELECT A MODE:*\n\n*TYPE ❬ "+prefix+"AUTOREAD INBOX ❭*\n*BOT WILL AUTO READ ONLY PRIVATE CHATS*\n\n*TYPE ❬ "+prefix+"AUTOREAD GROUPS ❭*\n*BOT WILL AUTO READ ONLY GROUPS*\n\n*TYPE ❬ "+prefix+"AUTOREAD ALL ❭*\n*BOT WILL AUTO READ EVERYWHERE*")
		return
	}

	// .autoread inbox
	if subCmd == "inbox" {
		s.SetAutoReadSetting("inbox")
		s.Reply(info, "*🔰 AUTO READ INBOX ACTIVATED 🔰*\n\n*BOT WILL NOW AUTO READ ALL PRIVATE/INBOX MESSAGES*\n*GROUPS MESSAGES WILL NOT BE READ AUTOMATICALLY*")
		return
	}

	// .autoread groups
	if subCmd == "groups" {
		s.SetAutoReadSetting("groups")
		s.Reply(info, "*🔰 AUTO READ GROUPS ACTIVATED 🔰*\n\n*BOT WILL NOW AUTO READ ALL GROUP MESSAGES*\n*INBOX MESSAGES WILL NOT BE READ AUTOMATICALLY*")
		return
	}

	// .autoread all
	if subCmd == "all" {
		s.SetAutoReadSetting("all")
		s.Reply(info, "*🔰 AUTO READ ALL ACTIVATED 🔰*\n\n*BOT WILL NOW AUTO READ ALL MESSAGES*\n*(INBOX + GROUPS BOTH)*")
		return
	}

	// .autoread off
	if subCmd == "off" {
		s.SetAutoReadSetting("off")
		s.Reply(info, "*🔰 AUTO READ DE-ACTIVATED 🔰*\n\n*BOT WILL NO LONGER AUTO READ MESSAGES*")
		return
	}

	// Unknown
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❬ AUTOREAD ❭ FOR HELP*")
}

// ── exported helper for handler.go ─────────────────────────────────────────

// AutoReadMode returns the current autoread mode: "off"/"inbox"/"groups"/"all".
// Used by handler.go applyAutoRead to decide whether to mark a message read.
func AutoReadMode(s SessionBridge) string {
	return s.GetAutoReadSetting("off")
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "autoread", Category: "OWNER & SYSTEM", Desc: "Auto-read messages (on/inbox/groups/all/off)", OwnerOnly: true, Run: handleAutoRead})
	Register(Command{Name: "readmsg", OwnerOnly: true, Hidden: true, Run: handleAutoRead})
	Register(Command{Name: "autoreadmsg", OwnerOnly: true, Hidden: true, Run: handleAutoRead})
}
