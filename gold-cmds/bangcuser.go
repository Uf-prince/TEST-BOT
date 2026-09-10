package goldcmds

// ============================================================================
// GOLD-MD — .bangcuser / .unbangcuser commands
//
// Ported from UMAR-MD (Node.js pair.js, lines 16388-16600) — SAME TEXT
// (0% farak), SAME WORK (0% farak):
//   .bangcuser @mention    → ban user in group (bot commands blocked for them)
//   .bangcuser 923xxx      → ban by number
//   .bangcuser (reply)     → ban by quoted message
//   .bangcuser list        → show banned users in this group
//   .unbangcuser @mention  → unban user
//   .unbangcuser 923xxx    → unban by number
//   .unbangcuser (reply)   → unban by quoted message
//
// Aliases: .usergcban / .bcuser / .gcban → bangcuser
//          .usergcunban / .unbcuser / .ungcban → unbangcuser
//
// Admin/owner-only, group-only. Per-group per-user ban stored in Redis:
//   SET "goldmd:<botJID>:groupset:<groupJID>:bangcuser" (user JID membership)
//   + bannedBy at "goldmd:<botJID>:groupset:<groupJID>:bangcusermeta:<userJID>"
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// bangcuserAdminAllowed checks whether the caller is owner OR group admin.
// Replies with rejection and returns false if neither.
func bangcuserAdminAllowed(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	if info.IsGroup {
		if s.IsGroupAdmin(info.Chat, info.Sender) {
			return true
		}
		if info.SenderAlt.Server != "" {
			if s.IsGroupAdmin(info.Chat, info.SenderAlt) {
				return true
			}
		}
	}
	s.Reply(info, "*THIS COMMAND IS ONLY FOR GROUP ADMINS / OWNER 😎*")
	return false
}

// bangcuserResolveTarget resolves the target user JID + number from:
// 1. @-mention  2. quoted-message participant  3. number arg.
func bangcuserResolveTarget(s SessionBridge, info types.MessageInfo, args []string) (jid, number string) {
	// 1. Mention check
	if mentions, ok := s.GetMentionedJIDs(info); ok && len(mentions) > 0 {
		jid = mentions[0]
		number = stripNonDigits(stripJIDSuffix(jid))
		return
	}
	// 2. Quoted message participant
	if _, quotedSender, ok := s.GetQuotedMessageID(info); ok && quotedSender != "" {
		jid = quotedSender
		number = stripNonDigits(stripJIDSuffix(jid))
		return
	}
	// 3. Number arg
	if len(args) > 0 && args[0] != "" {
		numArg := stripNonDigits(args[0])
		if len(numArg) >= 7 {
			jid = numArg + "@s.whatsapp.net"
			number = numArg
			return
		}
	}
	return "", ""
}

// ── bangcuser (ban user in group) ─────────────────────────────────────────

func handleBangcuser(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBangcuserAsync(s, info, args, prefix)
}

func handleBangcuserAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !bangcuserAdminAllowed(s, info) {
		return
	}

	if !info.IsGroup {
		s.Reply(info, "*USE THIS COMMAND IN GROUPS ONLY*")
		return
	}

	groupJID := info.Chat.String()

	// ── LIST ──
	if len(args) > 0 && strings.ToLower(strings.TrimSpace(args[0])) == "list" {
		bannedList := s.GroupBanUserList(groupJID)
		if len(bannedList) == 0 {
			s.Reply(info, "*🔰 USERGCBAN LIST*\n\n*NO BANNED USERS IN THIS GROUP 🔰*")
			return
		}
		var sb strings.Builder
		for i, b := range bannedList {
			userNum := stripNonDigits(stripJIDSuffix(b.UserJID))
			bannedBy := b.BannedBy
			if bannedBy == "" {
				bannedBy = "admin"
			}
			bannedByNum := stripNonDigits(stripJIDSuffix(bannedBy))
			sb.WriteString("*" + strconv.Itoa(i+1) + ". @" + userNum + "* (Banned By: " + bannedByNum + ")\n")
		}
		// Build mention list
		mentioned := make([]string, 0, len(bannedList))
		for _, b := range bannedList {
			mentioned = append(mentioned, b.UserJID)
		}
		s.ReplyWithMentions(info, "*🔰 GROUP BANNED USERS LIST 🔰*\n\n"+sb.String()+"\n*TOTAL ❯ "+strconv.Itoa(len(bannedList))+"*", mentioned)
		return
	}

	// ── GET TARGET USER ──
	targetJID, targetNumber := bangcuserResolveTarget(s, info, args)

	if targetJID == "" {
		s.Reply(info, "*🔰 USERGCBAN INFO 🔰*\n\n*3 METHODS TO BAN A USER*\n\n*METHOD 1:*\n*MENTION USER — WRITE ❬ USERGCBAN @MENTION ❭*\n\n*METHOD 2:*\n*REPLY TO USER — WRITE ❬ USERGCBAN ❭*\n\n*METHOD 3:*\n*TYPE NUMBER — ❬ USERGCBAN 923XXXXX ❭*")
		return
	}

	// ── CAN'T BAN SELF OR BOT ──
	senderNum := stripNonDigits(stripJIDSuffix(info.Sender.String()))
	targetNum := targetNumber
	if info.SenderAlt.Server != "" {
		altNum := stripNonDigits(stripJIDSuffix(info.SenderAlt.String()))
		if targetNum == altNum {
			s.Reply(info, "*🔰 YOU CANNOT BAN YOURSELF 🔰*")
			return
		}
	}
	if targetNum == senderNum {
		s.Reply(info, "*🔰 YOU CANNOT BAN YOURSELF 🔰*")
		return
	}

	// Check if target is the bot owner
	if isTargetOwner(s, info, targetJID, targetNum) {
		s.Reply(info, "*🔰 I AM THE OWNER — YOU CANNOT BAN ME 😎*")
		return
	}

	// ── CHECK IF ALREADY BANNED ──
	alreadyBanned := s.GroupBanUserIsBanned(groupJID, targetJID)
	if alreadyBanned {
		s.ReplyWithMentions(info, "*🔰 USER ALREADY BANNED*\n\n*@"+targetNum+"*\n*IS ALREADY BANNED IN THIS GROUP 🔰*", []string{targetJID})
		return
	}

	// ── BAN THE USER ──
	bannedBy := info.Sender.String()
	_ = s.GroupBanUserAdd(groupJID, targetJID, bannedBy)
	s.ReplyWithMentions(info, "*🔰 USER BANNED SUCCESSFULLY 🔰*\n\n*@"+targetNum+"*\n*HAS BEEN BANNED FROM THIS GROUP 🔰*\n\n*ALL MESSAGES WILL BE AUTO DELETED 🔰*\n*CONTACT ADMINS FOR REASON 🔰*", []string{targetJID})
}

// ── unbangcuser (unban user in group) ─────────────────────────────────────

func handleUnbangcuser(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleUnbangcuserAsync(s, info, args, prefix)
}

func handleUnbangcuserAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !bangcuserAdminAllowed(s, info) {
		return
	}

	if !info.IsGroup {
		s.Reply(info, "*USE THIS COMMAND IN GROUPS ONLY*")
		return
	}

	groupJID := info.Chat.String()

	// ── GET TARGET USER ──
	targetJID, targetNumber := bangcuserResolveTarget(s, info, args)

	if targetJID == "" {
		s.Reply(info, "*🔰 USERGCUNBAN INFO 🔰*\n\n*3 METHODS TO UNBAN A USER*\n\n*METHOD 1:*\n*MENTION USER — WRITE ❬ USERGCUNBAN @MENTION ❭*\n\n*METHOD 2:*\n*REPLY TO USER — WRITE ❬ USERGCUNBAN ❭*\n\n*METHOD 3:*\n*TYPE NUMBER — ❬ USERGCUNBAN 923XXXXX ❭*")
		return
	}

	// ── CHECK IF NOT BANNED ──
	isBanned := s.GroupBanUserIsBanned(groupJID, targetJID)
	if !isBanned {
		s.ReplyWithMentions(info, "*🔰 USER NOT BANNED*\n\n*@"+targetNumber+"*\n*IS NOT BANNED IN THIS GROUP 🔰*", []string{targetJID})
		return
	}

	// ── UNBAN THE USER ──
	_ = s.GroupBanUserRemove(groupJID, targetJID)
	s.ReplyWithMentions(info, "*🔰 USER UNBANNED SUCCESSFULLY 🔰*\n\n*@"+targetNumber+"*\n*HAS BEEN UNBANNED FROM THIS GROUP 🔰*\n\n*NOW CAN SEND MESSAGES FREELY 🔰*", []string{targetJID})
}

// ── exported helper for handler.go ─────────────────────────────────────────

// GroupBanUserIsBannedExported checks whether a user is banned in a group.
// Used by handler.go enforcement to block commands from group-banned users.
func GroupBanUserIsBannedExported(s SessionBridge, groupJID, userJID string) bool {
	return s.GroupBanUserIsBanned(groupJID, userJID)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "bangcuser", Category: "GROUP MANAGEMENT", Desc: "Ban a user from bot commands in this group", OwnerOnly: false, Run: handleBangcuser})
	Register(Command{Name: "usergcban", OwnerOnly: false, Hidden: true, Run: handleBangcuser})
	Register(Command{Name: "bcuser", OwnerOnly: false, Hidden: true, Run: handleBangcuser})
	// Note: "gcban" is already registered as an alias for bangc (lock group).
	// The Node.js also lists "gcban" for bangcuser, but since bangc (group lock)
	// takes priority in the Node.js code (it appears first in the if-chain),
	// we keep gcban → bangc and do NOT re-register it here to avoid conflict.

	Register(Command{Name: "unbangcuser", Category: "GROUP MANAGEMENT", Desc: "Unban a user from bot commands in this group", OwnerOnly: false, Run: handleUnbangcuser})
	Register(Command{Name: "usergcunban", OwnerOnly: false, Hidden: true, Run: handleUnbangcuser})
	Register(Command{Name: "unbcuser", OwnerOnly: false, Hidden: true, Run: handleUnbangcuser})
	Register(Command{Name: "ungcban", OwnerOnly: false, Hidden: true, Run: handleUnbangcuser})
}
