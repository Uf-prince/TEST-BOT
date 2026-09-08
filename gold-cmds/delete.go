package goldcmds

// ============================================================================
// GOLD-MD — Delete Message (DEL) Command
// File: delete.go
// ============================================================================
// COMMAND: .del   /  .dl   /  .delete    (owner-only, reply to a message)
//   Deletes the replied-to message.
//
// BEHAVIOR:
//   INBOX (DM):
//     - Reply to YOUR OWN (bot/owner) message  -> DELETE FOR EVERYONE
//     - Reply to THE OTHER PERSON'S message    -> DELETE FOR ME ONLY
//       (WhatsApp does not allow revoking someone else's DM message)
//
//   GROUP:
//     - Reply to YOUR OWN (bot/owner) message  -> DELETE FOR EVERYONE
//     - Reply to SOMEONE ELSE'S message        -> ALWAYS tries DELETE FOR
//       EVERYONE first (no admin check — we let WhatsApp decide).
//       If the revoke is silently rejected by WhatsApp (e.g. bot is not
//       group admin), the error is reported.
//
//   IN EVERY CASE: after handling the target message, the ".del" command
//   message itself is also deleted (it's the bot's own message, so it's
//   always removed for everyone).
//
//   MEDIA: works on any content type (text/image/video/pdf/document/
//   sticker/audio/viewOnce...) because delete targets the message KEY,
//   not the content type.
//
// ACCESS: OWNER-ONLY. Only the number the bot is logged in on can run
// this command. Anyone else gets a polite refusal.
//
// Source: UMAR-MD delete.js  (Node.js / Baileys)
// Converted to Go / whatsmeow for GOLD-MD.
// ============================================================================
// LID vs PN FIX:
//   Checks both the bot's classic JID and LID when deciding if a quoted
//   message belongs to the bot — fixes silent fromMe=false mismatches.
// ============================================================================

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const delNotOwnerText = "*THIS COMMAND IS ONLY FOR ME 😎*"

const delNoQuoteText = "*🔰 DELETE MESSAGE INFO* 🔰\n" +
	"*MENTION TO THE MESSAGE IMPORTANT ⚠️ WHICH MESSAGE DO YOU WANT TO DELETE*\n\n" +
	"*AND TYPE ❮ DL ❯ TO DELETE MSG*"

// isGroupChat reports whether the chat JID is a group.
func isGroupChat(chat types.JID) bool {
	return chat.Server == "g.us"
}

// normalizeJidStr strips ":device" suffix so JID string comparisons don't
// silently fail (e.g. "923158930864:61@s.whatsapp.net" -> "923158930864@s.whatsapp.net").
func normalizeJidStr(jid string) string {
	if jid == "" {
		return jid
	}
	atIdx := strings.Index(jid, "@")
	var user, server string
	if atIdx >= 0 {
		user = jid[:atIdx]
		server = jid[atIdx+1:]
	} else {
		user = jid
		server = "s.whatsapp.net"
	}
	if colonIdx := strings.Index(user, ":"); colonIdx >= 0 {
		user = user[:colonIdx]
	}
	return user + "@" + server
}

// isBotOwnJid checks a JID string against the bot's own JID (both classic
// and LID forms). Used to determine if a quoted message was sent by the bot.
func isBotOwnJid(botJID string, jid string) bool {
	if jid == "" || botJID == "" {
		return false
	}
	target := strings.ToLower(normalizeJidStr(jid))
	candidate := strings.ToLower(normalizeJidStr(botJID))
	return target == candidate
}

func handleDel(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDelAsync(s, info, args, prefix)
}

func handleDelAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {

	if !s.IsOwner(info) {
		s.Reply(info, delNotOwnerText)
		return
	}

	// Check if there is a quoted message at all.
	hasQuoted := false
	var quotedID, quotedSender string
	if q, ok := s.(interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}); ok {
		if qid, qsender, ok2 := q.GetQuotedMessageID(info); ok2 && qid != "" {
			hasQuoted = true
			quotedID = qid
			quotedSender = qsender
		}
	}

	if !hasQuoted {
		s.Reply(info, delNoQuoteText)
		return
	}

	botJID := s.GetJID()
	group := isGroupChat(info.Chat)

	// Determine if the quoted message was sent by the bot (fromMe).
	// Check the quoted sender against the bot's JID (both normalized forms).
	reallyFromMe := isBotOwnJid(botJID, quotedSender)
	// Also consider IsFromMe from the message info as a fallback.
	if !reallyFromMe && info.IsFromMe {
		// In a DM, if the message is from me, the quoted message could be
		// from the other person or from me. We rely on quotedSender.
		// But if quotedSender is empty, assume fromMe.
		if quotedSender == "" {
			reallyFromMe = true
		}
	}

	// ── DECIDE MODE FOR THE TARGET (QUOTED) MESSAGE ──
	// DM + other person's message  -> 'me' (WhatsApp platform rule)
	// Everything else              -> 'everyone'
	var mode string
	if !reallyFromMe && !group {
		mode = "me"
	} else {
		mode = "everyone"
	}

	// ── 1) APPLY DELETE TO THE TARGET (QUOTED) MESSAGE ──
	if mode == "everyone" {
		// Build the revoke: for own messages, sender is empty (bot).
		// For others' messages in a group, pass the quoted sender.
		var revokeSender string
		if !reallyFromMe && group {
			revokeSender = quotedSender
		}
		err := s.RevokeQuotedMessage(info.Chat, revokeSender, quotedID)
		if err != nil {
			// If revoke failed in a group, report the error.
			s.Reply(info, fmt.Sprintf("🔰 *DEL ERROR* 🔰\n*%s*", strings.ToUpper(err.Error())))
			return
		}
	} else {
		// mode === 'me' (DM, other person's message)
		// whatsmeow does not have a direct "delete for me" API like Baileys
		// chatModify. The best we can do is attempt a revoke (which WhatsApp
		// will reject for others' DM messages) and report the outcome.
		err := s.RevokeQuotedMessage(info.Chat, quotedSender, quotedID)
		if err != nil {
			s.Reply(info, fmt.Sprintf("🔰 *DEL ERROR* 🔰\n*%s*", strings.ToUpper(err.Error())))
			return
		}
	}

	// ── 2) DELETE THE ".DEL" COMMAND MESSAGE ITSELF (ALWAYS FOR EVERYONE) ──
	if err := s.DeleteMessage(info, info.ID); err != nil {
		// Target was already handled — don't error out over this.
	}

}

func init() {
	Register(Command{Name: "del", Category: "OWNER & SYSTEM", Desc: "Delete the replied-to message", OwnerOnly: true, Run: handleDel})
	Register(Command{Name: "dl", OwnerOnly: true, Hidden: true, Run: handleDel})
	Register(Command{Name: "delete", OwnerOnly: true, Hidden: true, Run: handleDel})
}
