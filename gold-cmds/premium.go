package goldcmds

// ============================================================================
// GOLD-MD — .antilinkprem / .antibotprem / .antibadprem commands
//
// Ported from UMAR-MD (Node.js pair.js, lines 13490-13610 / 15260-15420 /
// 14095-14240) — SAME TEXT (0% farak), SAME WORK (0% farak):
//   .antilinkprem add @mention   → add user to premium whitelist
//   .antilinkprem add 923xxx      → add by number
//   .antilinkprem del @mention    → remove from whitelist
//   .antilinkprem list            → show all premium users
//
// All three commands share the SAME PremiumUser list (Redis SET "premium:set").
// The ONLY difference is the label word in the text:
//   antilinkprem → "ANTILINK" / "LINKS"
//   antibotprem  → "ANTIBOT"  / "MESSAGES"
//   antibadprem  → "ANTIBAD"  / "BAD WORDS"
//
// Caller must be owner OR group-admin. 👑 (crown) emoji in Node.js is replaced
// with 🔰 in GOLD-MD. Uses ❬ ❭ brackets and ❯ colons matching Node.js text.
//
// Redis: SET "premium:set" (JID membership) + per-user number at
//   "goldmd:<botJID>:premiummeta:<userJID>"
// ============================================================================

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── helper functions ──────────────────────────────────────────────────────

// stripNonDigits removes all non-digit characters from a string.
func stripNonDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripJIDSuffix removes the @s.whatsapp.net / @lid suffix from a JID,
// leaving just the phone number.
func stripJIDSuffix(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

// premCallerAllowed checks whether the caller is the bot owner OR a group
// admin. Mirrors the Node.js callerIsAllowed logic. Replies with the
// rejection message and returns false if neither.
func premCallerAllowed(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	if info.IsGroup {
		if s.IsGroupAdmin(info.Chat, info.Sender) {
			return true
		}
		// Also check SenderAlt (LID <-> phone JID mapping)
		if info.SenderAlt.Server != "" {
			if s.IsGroupAdmin(info.Chat, info.SenderAlt) {
				return true
			}
		}
	}
	s.Reply(info, "*THIS COMMAND IS ONLY FOR OWNER / GROUP ADMINS 😎*")
	return false
}

// resolvePremTarget resolves the target user JID + number from:
// 1. @-mention  2. quoted-message participant  3. number arg.
// Returns jid="" if none found. Mirrors the Node.js target resolution.
func resolvePremTarget(s SessionBridge, info types.MessageInfo, argsText string) (jid, number string) {
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
	// 3. Number arg (take last whitespace-split token, strip non-digits)
	if argsText != "" {
		tokens := strings.Fields(argsText)
		if len(tokens) > 0 {
			numArg := stripNonDigits(tokens[len(tokens)-1])
			if len(numArg) >= 7 {
				jid = numArg + "@s.whatsapp.net"
				number = numArg
				return
			}
		}
	}
	return "", ""
}

// ── shared premium handler ────────────────────────────────────────────────
//
// labelUpper  = "ANTILINK" / "ANTIBOT" / "ANTIBAD"  (the feature name in caps)
// cmdUpper    = "ANTILINKPREM" / "ANTIBOTPREM" / "ANTIBADPREM"  (command name)
// offenceWord = "LINKS" / "MESSAGES" / "BAD WORDS"  (what the user can now do)
// premAction  = "ANTILINK" / "ANTIBOT" / "ANTIBAD"  (action label in success msg)
// premSendWord = "SEND ALL LINKS" / "SEND ALL MESSAGES" / "SEND BAD WORDS"
//
// All text is SAME as Node.js pair.js with 👑 replaced by 🔰.

func handlePremium(s SessionBridge, info types.MessageInfo, args []string, prefix string, labelUpper, cmdUpper, offenceWord, premAction, premSendWord string) {
	go handlePremiumAsync(s, info, args, prefix, labelUpper, cmdUpper, offenceWord, premAction, premSendWord)
}

func handlePremiumAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, labelUpper, cmdUpper, offenceWord, premAction, premSendWord string) {
	if !premCallerAllowed(s, info) {
		return
	}

	argsText := ""
	if len(args) > 0 {
		argsText = strings.Join(args, " ")
	}

	targetJID, targetNumber := resolvePremTarget(s, info, argsText)

	subCmd := ""
	if len(args) > 0 {
		subCmd = strings.ToLower(strings.TrimSpace(args[0]))
	}

	// ── AUTO-ADD FALLBACK (mirrors UMAR-MD Node.js behavior) ──
	// If the caller writes just a number / @mention / quoted-message WITHOUT
	// the "add" keyword (e.g. ".antistatusprem 923xxx"), treat it as ADD.
	// Only do this when subCmd is NOT one of the known keywords (add/delete/del/
	// remove/list) AND a real target was resolved.
	isKnownKeyword := subCmd == "add" || subCmd == "delete" || subCmd == "del" ||
		subCmd == "remove" || subCmd == "list"
	if !isKnownKeyword && targetJID != "" && targetNumber != "" {
		subCmd = "add"
	}

	// ── ADD ──
	if subCmd == "add" {
		if targetJID == "" || targetNumber == "" {
			s.Reply(info, "*\U0001F514 "+labelUpper+" PREMIUM ADD COMMAND INFO \U0001F514*\n\n*3 METHODS TO ADD "+labelUpper+" PREMIUM USER*\n\n*METHOD 1*\n*MENTION ANY USER IN GROUP OR INBOX AND THEN WRITE ( "+prefix+cmdUpper+" ADD ) SO THAT WHATSAPP USER WILL BE ADDED TO "+labelUpper+" PREMIUM LIST THEN "+labelUpper+" ACTIONS WILL NOT APPLY ON THAT USER*\n\n*METHOD 2*\n*TYPE ( "+prefix+cmdUpper+" ADD 923XXXXXX )*\n*( FOR INBOX )*\n\n*METHOD 3*\n*TYPE ( "+prefix+cmdUpper+" ADD @MENTION )*\n*( FOR GROUP )*\n\n*WHEN USER IS ADDED TO "+labelUpper+" PREMIUM THEN NO "+labelUpper+" ACTION WILL APPLY ON THAT USER HE CAN "+premSendWord+" IN THIS GROUP*")
			return
		}
		_ = s.PremiumAdd(targetJID, targetNumber)
		s.Reply(info, "*\U0001F514 "+labelUpper+" PREMIUM USER ADDED \U0001F514*\n\n*NUMBER \u2764 ( "+targetNumber+" )*\n\n*NOW THIS USER IS IN "+labelUpper+" PREMIUM LIST NOW "+labelUpper+" ACTION WILL NOT APPLY ON THIS NUMBER NOW THIS USER CAN "+premSendWord+" IN THIS GROUP FREELY*")
		return
	}

	// ── REMOVE / DELETE / DEL ──
	if subCmd == "remove" || subCmd == "delete" || subCmd == "del" {
		if targetJID == "" || targetNumber == "" {
			s.Reply(info, "*\U0001F514 "+labelUpper+" PREMIUM DELETE COMMAND INFO \U0001F514*\n\n*3 METHODS TO REMOVE "+labelUpper+" PREMIUM USER*\n\n*METHOD 1*\n*MENTION ANY USER IN GROUP OR INBOX AND THEN WRITE ( "+prefix+cmdUpper+" DELETE ) SO THAT WHATSAPP USER WILL BE REMOVED FROM "+labelUpper+" PREMIUM LIST THEN "+labelUpper+" ACTIONS WILL APPLY ON THAT USER AGAIN*\n\n*METHOD 2*\n*TYPE ( "+prefix+cmdUpper+" DELETE 923XXXXXX )*\n*( FOR INBOX )*\n\n*METHOD 3*\n*TYPE ( "+prefix+cmdUpper+" DELETE @MENTION )*\n*( FOR GROUP )*\n\n*WHEN USER IS REMOVED FROM "+labelUpper+" PREMIUM THEN "+labelUpper+" ACTIONS WILL APPLY ON THAT USER AGAIN HE CANNOT "+premSendWord+" IN THIS GROUP*")
			return
		}
		_ = s.PremiumRemove(targetJID)
		s.Reply(info, "*\U0001F514 "+labelUpper+" PREMIUM USER DELETED \U0001F514*\n\n*NUMBER \u2764 ( "+targetNumber+" )*\n\n*NOW THIS USER IS REMOVED FROM "+labelUpper+" PREMIUM LIST NOW "+labelUpper+" ACTION WILL APPLY ON THIS USER AGAIN NOW THIS USER CANNOT "+premSendWord+" IN THIS GROUP*")
		return
	}

	// ── LIST ──
	if subCmd == "list" {
		allPrem := s.PremiumList()
		if len(allPrem) == 0 {
			s.Reply(info, "*\U0001F5C3\uFE0F NO "+labelUpper+" PREMIUM USERS YET*")
			return
		}
		var sb strings.Builder
		for i, u := range allPrem {
			sb.WriteString(strconv.Itoa(i+1) + ". " + u.Number + "\n")
		}
		s.Reply(info, "*\u2B50 "+labelUpper+" PREMIUM USERS LIST \u2B50*\n\n"+sb.String()+"\n*TOTAL \u2764 "+strconv.Itoa(len(allPrem))+"*")
		return
	}

	// ── NO / UNKNOWN SUBCOMMAND → full help ──
	s.Reply(info, "*\U0001F514 "+cmdUpper+" ADD COMMAND INFO \U0001F514*\n\n*3 METHODS TO ADD "+cmdUpper+" USER*\n\n*METHOD 1*\n*MENTION ANY USER IN GROUP OR INBOX AND THEN WRITE ( "+prefix+cmdUpper+" ADD ) SO THAT WHATSAPP USER WILL BE ADDED TO "+cmdUpper+" LIST THEN "+premAction+" ACTIONS WILL NOT APPLY ON THAT USER*\n\n*METHOD 2*\n*TYPE ( "+prefix+cmdUpper+" ADD 923XXXXXX )*\n*( FOR INBOX )*\n\n*METHOD 3*\n*TYPE ( "+prefix+cmdUpper+" ADD @MENTION )*\n*( FOR GROUP )*\n\n*WHEN USER IS ADDED TO "+cmdUpper+" THEN NO "+premAction+" ACTION WILL APPLY ON THAT USER HE CAN "+premSendWord+" IN THIS GROUP*\n\n*\U0001F514 "+cmdUpper+" DELETE COMMAND INFO \U0001F514*\n\n*3 METHODS TO REMOVE "+cmdUpper+" USER*\n\n*METHOD 1*\n*MENTION ANY USER IN GROUP OR INBOX AND THEN WRITE ( "+prefix+cmdUpper+" DELETE ) SO THAT WHATSAPP USER WILL BE REMOVED FROM "+cmdUpper+" LIST THEN "+premAction+" ACTIONS WILL APPLY ON THAT USER AGAIN*\n\n*METHOD 2*\n*TYPE ( "+prefix+cmdUpper+" DELETE 923XXXXXX )*\n*( FOR INBOX )*\n\n*METHOD 3*\n*TYPE ( "+prefix+cmdUpper+" DELETE @MENTION )*\n*( FOR GROUP )*\n\n*WHEN USER IS REMOVED FROM "+cmdUpper+" THEN "+premAction+" ACTIONS WILL APPLY ON THAT USER AGAIN HE CANNOT "+premSendWord+" IN THIS GROUP*\n\n*TO SHOW "+labelUpper+" PREMIUM USERS LIST*\n*TYPE ( "+prefix+cmdUpper+" LIST )*")
}

// ── specific handlers ─────────────────────────────────────────────────────

func handleAntilinkPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handlePremium(s, info, args, prefix, "ANTILINK", "ANTILINKPREMIUM", "LINKS", "ANTILINK", "SEND ALL LINKS")
}

func handleAntibotPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handlePremium(s, info, args, prefix, "ANTIBOT", "ANTIBOTPREMIUM", "MESSAGES", "ANTIBOT", "SEND ALL MESSAGES")
}

func handleAntibadPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handlePremium(s, info, args, prefix, "ANTIBAD", "ANTIBADPREMIUM", "BAD WORDS", "ANTIBAD", "SEND BAD WORDS")
}

func handleAntistatusPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handlePremium(s, info, args, prefix, "ANTISTATUS", "ANTISTATUSPREMIUM", "STATUS POSTS", "ANTISTATUS", "SEND STATUS POSTS")
}

func handleAntiCallPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handlePremium(s, info, args, prefix, "ANTICALL", "ANTICALLPREMIUM", "CALLS", "ANTICALL", "MAKE CALLS")
}

// ── exported helper for handler.go ─────────────────────────────────────────

// PremiumIsMemberExported checks whether a JID is in the premium whitelist.
// Used by anti-* enforcement in handler.go to skip actions for premium users.
func PremiumIsMemberExported(s SessionBridge, jid string) bool {
	return s.PremiumIsMember(jid)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	// antilinkprem (antilinkpremium / antilinkprem)
	Register(Command{Name: "antilinkprem", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO SET PREMIUM ANTILINK PROTECTION FOR THE GROUP.", OwnerOnly: false, Run: handleAntilinkPrem})
	Register(Command{Name: "antilinkpremium", OwnerOnly: false, Hidden: true, Run: handleAntilinkPrem})

	// antistatusprem (antistatusprem / antisprem / antistatuspremium)
	Register(Command{Name: "antistatusprem", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO SET PREMIUM ANTISTATUS PROTECTION FOR THE GROUP.", OwnerOnly: false, Run: handleAntistatusPrem})
	Register(Command{Name: "antisprem", OwnerOnly: false, Hidden: true, Run: handleAntistatusPrem})
	Register(Command{Name: "antistatuspremium", OwnerOnly: false, Hidden: true, Run: handleAntistatusPrem})

	// antibotprem (antibotpremium / antibotprem / abprem)
	Register(Command{Name: "antibotprem", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO SET PREMIUM ANTIBOT PROTECTION FOR THE GROUP.", OwnerOnly: false, Run: handleAntibotPrem})
	Register(Command{Name: "antibotpremium", OwnerOnly: false, Hidden: true, Run: handleAntibotPrem})
	Register(Command{Name: "abprem", OwnerOnly: false, Hidden: true, Run: handleAntibotPrem})

	// antibadprem (antibadpremium / antibadprem / abwprem)
	Register(Command{Name: "antibadprem", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO SET PREMIUM ANTIBAD PROTECTION FOR THE GROUP. IT WORKS WITH A PREMIUM WORD LIST.", OwnerOnly: false, Run: handleAntibadPrem})
	Register(Command{Name: "antibadpremium", OwnerOnly: false, Hidden: true, Run: handleAntibadPrem})
	Register(Command{Name: "abwprem", OwnerOnly: false, Hidden: true, Run: handleAntibadPrem})

	// anticallprem (anticallpremium / anticallprem / acallprem / anticalprem)
	Register(Command{Name: "anticallprem", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO SET PREMIUM AUTO CALL REJECT FOR THE BOT.", OwnerOnly: false, Run: handleAntiCallPrem})
	Register(Command{Name: "anticallpremium", OwnerOnly: false, Hidden: true, Run: handleAntiCallPrem})
	Register(Command{Name: "acallprem", OwnerOnly: false, Hidden: true, Run: handleAntiCallPrem})
	Register(Command{Name: "anticalprem", OwnerOnly: false, Hidden: true, Run: handleAntiCallPrem})
}
