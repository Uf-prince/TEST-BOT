package goldcmds

// ============================================================================
// GOLD-MD — .JID COMMAND
// Ported from TEST-BOT Node.js jid.js — same text, same behaviour.
//
// MODES:
//   .jid                                    -> HELP MESSAGE
//   .jid 923xxxxxxxxx                       -> NUMBER's WhatsApp JID
//   .jid https://chat.whatsapp.com/CODE     -> GROUP invite link's JID
//   .jid https://whatsapp.com/channel/CODE  -> CHANNEL (newsletter) JID
//
// whatsmeow equivalents:
//   - client.IsOnWhatsApp(number)              -> [{ IsIn, JID }]
//   - client.GetGroupInfoFromLink(code)        -> GroupInfo { JID, ... }
//   - client.GetNewsletterInfoWithInvite(code) -> NewsletterMetadata { ID, ... }
//   (code = just the last part of the link, after the final "/")
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

var (
	jidGroupCodeRe   = regexp.MustCompile(`(?i)chat\.whatsapp\.com/([A-Za-z0-9]+)`)
	jidChannelCodeRe = regexp.MustCompile(`(?i)whatsapp\.com/channel/([A-Za-z0-9]+)`)
	jidWhatsAppLink  = regexp.MustCompile(`(?i)whatsapp\.com`)
	jidNonDigits     = regexp.MustCompile(`[^0-9]`)
	jidPlusSpaceDash = regexp.MustCompile(`[+\s-]`)
)

const jidHelpText = `*🔰 JID FINDER — HELP 🔰*` + "\n\n" +
	`*THIS COMMAND FINDS THE REAL WHATSAPP JID (ID) OF 3 THINGS:*` + "\n" +
	`*A NUMBER, A GROUP LINK, OR A CHANNEL LINK — JUST ATTACH IT TO THE COMMAND.*` + "\n\n" +

	`*🔰 NOTE: ONLY PUT A NUMBER OR LINK WITH THE COMMAND, NOTHING ELSE 🔰*` + "\n" +
	`*1. GET A NUMBER'S JID*` + "\n" +
	`*COMMAND: .JID 923001234567*` + "\n" +
	`*WORK: CHECKS THE NUMBER — IF IT'S ON WHATSAPP, RETURNS ITS REAL JID (@S.WHATSAPP.NET)*` + "\n" +
	`*EXAMPLE: .JID 923001234567*` + "\n\n" +

	`*🔰 NOTE: PASTE THE FULL GROUP INVITE LINK 🔰*` + "\n" +
	`*2. GET A GROUP'S JID*` + "\n" +
	`*COMMAND: .JID https://chat.whatsapp.com/CODE*` + "\n" +
	`*WORK: WITHOUT JOINING THE GROUP, PULLS THE GROUP'S REAL JID (@G.US) FROM THE LINK*` + "\n" +
	`*EXAMPLE: .JID https://chat.whatsapp.com/ABCDEFGH12*` + "\n\n" +

	`*🔰 NOTE: PASTE THE FULL CHANNEL LINK 🔰*` + "\n" +
	`*3. GET A CHANNEL (NEWSLETTER) JID*` + "\n" +
	`*COMMAND: .JID https://whatsapp.com/channel/CODE*` + "\n" +
	`*WORK: PULLS THE CHANNEL'S REAL JID (@NEWSLETTER) FROM THE LINK*` + "\n" +
	`*EXAMPLE: .JID https://whatsapp.com/channel/0029VaXXXXXX*` + "\n\n" +

	`*━━━━━━━━━━━━*` + "\n" +
	`*🔰 IMPORTANT RULES 🔰*` + "\n" +
	`*1. ALWAYS INCLUDE THE COUNTRY CODE WITH THE NUMBER (NO "+" SIGN)*` + "\n" +
	`*2. PASTE THE FULL LINK, OR JUST ITS CODE (THE LAST PART) — BOTH WORK*` + "\n" +
	`*3. ONLY ONE NUMBER OR ONE LINK CAN BE CHECKED AT A TIME*`

// extractGroupCode pulls the invite code out of a group link.
func jidExtractGroupCode(text string) string {
	m := jidGroupCodeRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// extractChannelCode pulls the invite key out of a channel link.
func jidExtractChannelCode(text string) string {
	m := jidChannelCodeRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// jidLooksLikeNumber: not a link + enough digits + nothing but digits/+-spaces.
func jidLooksLikeNumber(text string) bool {
	digits := jidNonDigits.ReplaceAllString(text, "")
	if jidWhatsAppLink.MatchString(text) {
		return false
	}
	return len(digits) >= 7 && len(digits) == len(jidPlusSpaceDash.ReplaceAllString(text, ""))
}

// ─── SEND "SEARCHING..." MESSAGE, WAIT 1s, THEN EDIT IT INTO THE FINAL JID ───
// (same edit technique as jid.js: protocolMessage type 14)
func jidSendWithEdit(s SessionBridge, info types.MessageInfo, jid string) {
	waitMsgID := s.ReplyWithID(info, "*🔰 SEARCHING JID... 🔰*")
	time.Sleep(1000 * time.Millisecond)
	s.EditMessage(info, waitMsgID, jid)
}

func handleJid(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, jidHelpText)
		return
	}

	client := s.GetClient()
	ctx := context.Background()

	// ─── 1) CHANNEL LINK -> NEWSLETTER JID ───
	if channelCode := jidExtractChannelCode(input); channelCode != "" {
		meta, err := client.GetNewsletterInfoWithInvite(ctx, channelCode)
		if err != nil {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if meta == nil || meta.ID.IsEmpty() {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*COULDN'T FETCH CHANNEL INFO, DOUBLE-CHECK THE LINK*")
			return
		}
		jidSendWithEdit(s, info, meta.ID.String())
		return
	}

	// ─── 2) GROUP LINK -> GROUP JID ───
	if groupCode := jidExtractGroupCode(input); groupCode != "" {
		meta, err := client.GetGroupInfoFromLink(ctx, groupCode)
		if err != nil {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if meta == nil || meta.JID.IsEmpty() {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*COULDN'T FETCH GROUP INFO, DOUBLE-CHECK THE LINK*")
			return
		}
		jidSendWithEdit(s, info, meta.JID.String())
		return
	}

	// ─── 3) PLAIN NUMBER -> USER JID ───
	if jidLooksLikeNumber(input) {
		digitsOnly := jidNonDigits.ReplaceAllString(input, "")
		results, err := client.IsOnWhatsApp(ctx, []string{digitsOnly})
		if err != nil {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if len(results) == 0 || !results[0].IsIn {
			s.Reply(info, "🔰 *JID ERROR* 🔰\n*THIS NUMBER IS NOT REGISTERED ON WHATSAPP*")
			return
		}
		jidSendWithEdit(s, info, results[0].JID.String())
		return
	}

	// ─── 4) NOTHING MATCHED ───
	s.Reply(info, "🔰 *JID ERROR* 🔰\n"+
		"*THIS IS NEITHER A VALID NUMBER, A GROUP LINK, NOR A CHANNEL LINK*\n\n"+
		jidHelpText)
}

func init() {
	Register(Command{Name: "jid", Category: "OWNER & SYSTEM", Desc: "Find the real JID of a number / group link / channel link", Run: handleJid})
}
