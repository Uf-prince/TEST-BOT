package goldcmds

// ============================================================================
// GOLD-MD — .RC COMMAND (react to a message with any emoji)
// Ported from Node.js react.js — same text, same behaviour.
//
// USAGE:
//   Reply to any message and type:
//     .rc🔰          (emoji directly after command, no space)
//     .rc 🔰         (space between command and emoji — both work)
//
// BEHAVIOR:
//   - Reply to any message + provide an emoji  -> REACT with that emoji
//   - No quoted message                        -> error
//   - No emoji provided                        -> error
//   - Works in DMs and Groups
//   - Works on any message type (text, image, video, sticker, etc.)
//
// ACCESS: OWNER-ONLY (checked inside the handler — OwnerOnly flag is silently
// ignored for non-owners by the framework, so we check s.IsOwner here).
//
// EMOJI EXTRACTION:
//   Node.js uses /\p{Emoji_Presentation}|\p{Emoji}\uFE0F/u — Go's regexp
//   lacks \p{Emoji_Presentation}, so we reuse the autoreact.go rune-range
//   emoji scanner (countEmojis) and take the FIRST emoji found.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// The framework's reactCommand already reacts 🔰 on EVERY command, so this
// command does NOT re-react on the user's command message (JS's 👑 react
// maps to the framework's 🔰).
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const (
	rcNotOwnerText = `*THIS COMMAND IS ONLY FOR ME 😎*`

	rcNoQuoteText = `*REACT ON ANY MESSAGE*` + "\n\n" +
		`*MENTION THE MESSAGE IMPORTANT 🔰*` + "\n" +
		`*AFTER MENTION THE MESSAGE TYPE*` + "\n\n" +
		`*RC 😎*` + "\n" +
		`*RC 🔰*` + "\n" +
		`*RC 🔰*` + "\n\n" +
		`*WHICH EMOJIE YOU SELECT THIS EMOJIE WILL REACT ON OTHER MESSAGE*`

	rcNoEmojiText = `*REACT ON ANY MESSAGE*` + "\n\n" +
		`*MENTION THE MESSAGE IMPORTANT 🔰*` + "\n" +
		`*AFTER MENTION THE MESSAGE TYPE*` + "\n\n" +
		`*RC 😎*` + "\n" +
		`*RC 🔰*` + "\n" +
		`*RC 🔰*` + "\n\n" +
		`*WHICH EMOJIE YOU SELECT THIS EMOJIE WILL REACT ON OTHER MESSAGE*`
)

// rcExtractEmoji returns the first emoji found in text (same as react.js's
// EMOJI_REGEX first-match), "" when none.
func rcExtractEmoji(text string) string {
	if text == "" {
		return ""
	}
	emojis := countEmojis(strings.TrimSpace(text))
	if len(emojis) == 0 {
		return ""
	}
	return emojis[0]
}

func handleRc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// ─── OWNER-ONLY (checked in-handler, same as react.js) ───
	if !s.IsOwner(info) {
		s.Reply(info, rcNotOwnerText)
		return
	}

	// ─── No quoted message -> error ───
	quotedID, quotedSender, hasQuote := s.GetQuotedMessageID(info)
	if !hasQuote || quotedID == "" {
		s.Reply(info, rcNoQuoteText)
		return
	}

	// ─── Emoji extraction (first emoji in the text after the command) ───
	emoji := rcExtractEmoji(strings.Join(args, " "))
	if emoji == "" {
		s.Reply(info, rcNoEmojiText)
		return
	}

	// ─── Build the key of the quoted message to react to ───
	// In groups the reaction key needs the quoted message's sender as
	// participant; in DMs the chat itself is the sender. GetQuotedMessageID
	// returns the participant when present (groups), otherwise "".
	reactSender := quotedSender
	if reactSender == "" {
		reactSender = info.Chat.String()
	}
	senderJID, err := types.ParseJID(reactSender)
	if err != nil {
		s.Reply(info, "🔰 *RC ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
		return
	}

	if err := s.SendReaction(info.Chat, senderJID, quotedID, emoji); err != nil {
		s.Reply(info, "🔰 *RC ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
		return
	}

	// ─── Delete the ".rc" command message itself (always revocable —
	// it's the bot's own message). Non-fatal, exactly like react.js. ───
	s.DeleteAnyMessage(info.Chat, info.Sender.String(), info.ID)
}

func init() {
	Register(Command{Name: "rc", Category: "PRESENCE & STATUS", Desc: "React to a replied message with any emoji", Run: handleRc})
	Register(Command{Name: "react", Category: "PRESENCE & STATUS", Hidden: true, Run: handleRc})
}
