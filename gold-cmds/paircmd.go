package goldcmds

// ============================================================================
// GOLD-MD — PAIR Command  (.pair + aliases)
// File: paircmd.go
// ----------------------------------------------------------------------------
// Simple command that sends the bot-link message (with the sender mentioned).
//
//   .pair              → same message
//   .pair 923xxxxxx    → same message (number is ignored)
//   .pair923xxxxxx     → same message (no-space form, routed by handler.go
//                         prefix-match special case)
//   .get / .bot / .botlink / .linkbot → hidden aliases, same message
//
// Whatever the user types after .pair (or nothing at all) the bot replies
// with the same bot-link message and pings the sender.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// pairBotLinkURL is the public panel link where users pair their number.
const pairBotLinkURL = "https://whatsapp.com/channel/0029Vb956DuHFxP4EhLJA316"

// handlePair sends the bot-link message with the sender mentioned.
// It ignores args entirely — .pair, .pair 923xxx and .pair923xxx all reply
// the same message.
func handlePair(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	senderJID := info.Sender.String()
	senderNum := strings.SplitN(senderJID, "@", 2)[0]

	text := "*🔰 GOLD-MD WHATSAPP BOT 🔰*\n\n" +
		"*DO YOU NEED THE BOT ?*\n" +
		"*JOIN WHATSAPP CHANNEL TO GET THE FRESH BOT LINK*\n" +
		pairBotLinkURL + "\n\n" +
		"@" + senderNum

	s.ReplyWithMentions(info, text, []string{senderJID})
}

func init() {
	Register(Command{
		Name:     "pair",
		Category: "OWNER & SYSTEM",
		Desc:     "Get the bot link to pair your number",
		Run:      handlePair,
	})
	// hidden aliases — same message
	Register(Command{Name: "get", Hidden: true, Run: handlePair})
	Register(Command{Name: "bot", Hidden: true, Run: handlePair})
	Register(Command{Name: "botlink", Hidden: true, Run: handlePair})
	Register(Command{Name: "linkbot", Hidden: true, Run: handlePair})
	Register(Command{Name: "repo", Hidden: true, Run: handlePair})
	Register(Command{Name: "script", Hidden: true, Run: handlePair})
}
