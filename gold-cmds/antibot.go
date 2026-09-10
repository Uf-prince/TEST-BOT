package goldcmds

// ============================================================================
// GOLD-MD — .antibot command (aliases: .ab)
//
// Ported from UMAR-MD (Node.js pair.js, lines 14687-14972) — same text,
// same behaviour, adapted for whatsmeow:
//   .antibot                  → full command info
//   .antibot on / off         → enable / disable bot-forward detection in group
//   .antibot action           → action info (warn / delete / kick)
//   .antibot action warn      → set action=warn
//   .antibot action warn <n>  → set action=warn + max-warnings=n (1-50)
//   .antibot action warn reset → reset max-warnings to default
//   .antibot action delete    → set action=delete
//   .antibot action kick      → set action=kick
//   .antibot allow <jid>      → whitelist a bot JID (won't trigger)
//   .antibot delete <jid>     → remove a JID from whitelist
//   .antibot allowedlist      → show whitelisted bot JIDs
//   .antibot reset            → full reset (off + warn + default max + clear)
//
// Per-group config in Redis (settings:<groupJID>):
//   field "antibot"           = on/off
//   field "antibot:action"    = warn/delete/kick
//   field "antibot:maxwarn"   = 1-50
// Whitelist: per-group SET "antibot:allowed" (bot JIDs).
//
// Detection: a message is flagged as "external bot" if IsForwardedBot()
// returns true (forwardingScore>=2, or forwardedNewsletter, or
// isForwarded+score==1+stanzaId) — mirroring UmarIsBotForward. Commands
// (prefixed messages) are exempt. The actual detection on incoming group
// messages is in handler.go (applyAntiBot), which calls
// AntibotCheckAndEnforce.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const antibotFeature = "antibot"
const antibotUpper = "ANTIBOT"
const antibotAllowedSet = "antibot:allowed"

// ── exported helpers for handler.go ────────────────────────────────────

// AntibotIsOn reports whether antibot is enabled in the given group.
func AntibotIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antibotFeature)
}

// AntibotAllowedJIDs returns the whitelisted bot JIDs for a group.
func AntibotAllowedJIDs(s SessionBridge, groupJID string) []string {
	return s.GroupSetMembers(groupJID, antibotAllowedSet)
}

// AntibotCheckAndEnforce checks if an incoming message looks like a forwarded
// bot message and, if so, enforces the configured action. Commands (prefixed
// messages) are exempt. Returns true if an offence was detected, false
// otherwise. Mirrors the antibot detection handler in pair.js (lines 15679-15952).
func AntibotCheckAndEnforce(s SessionBridge, info types.MessageInfo, body, prefix string) bool {
	if !AntibotIsOn(s, info.Chat.String()) {
		return false
	}
	// Command exemption: if the message starts with the prefix, skip —
	// it's a bot command, not spam. (Mirrors abIsCommand in Node.js.)
	trimmed := strings.TrimSpace(body)
	if prefix != "" && strings.HasPrefix(trimmed, prefix) {
		return false
	}
	// Bot-forward detection (mirrors UmarIsBotForward).
	if !s.IsForwardedBot(info) {
		return false
	}
	// Whitelist check: sender JID matches allowed list (exact or suffix).
	senderJID := info.Sender.String()
	allowed := AntibotAllowedJIDs(s, info.Chat.String())
	for _, a := range allowed {
		if senderJID == a || strings.HasSuffix(senderJID, a) {
			return false
		}
	}
	// Offence detected → enforce
	senderNum := strings.SplitN(senderJID, "@", 2)[0]
	EnforceAntiAction(s, info, antibotFeature, antibotUpper, "BOT MESSAGES", "OTHER BOTS NOT ALLOWED", senderJID, senderNum)
	return true
}

// ── command handler ────────────────────────────────────────────────────

func handleAntibot(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAntibotAsync(s, info, args, prefix)
}

func handleAntibotAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroupOwner(s, info) {
		return
	}
	groupJID := info.Chat.String()

	// No args → full command info
	if len(args) == 0 {
		allowed := AntibotAllowedJIDs(s, groupJID)
		curAction := antiAction(s, groupJID, antibotFeature)
		maxW := antiMaxWarnings(s, groupJID, antibotFeature)
		enabled := antiEnabled(s, groupJID, antibotFeature)

		s.Reply(info, "*🔰 ANTIBOT COMMAND INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBOT ON ❱*\n*WHEN ANTIBOT IS ON THEN IF ANY BOT SENDS ANY MESSAGE IN GROUP THE BOT WILL DETECT AND ACTION WILL APPLY*\n\n*TYPE ❰ "+prefix+"ANTIBOT OFF ❱*\n*WHEN ANTIBOT IS OFF THEN ALL MEMBERS CAN FREELY SEND MESSAGES IN THIS GROUP*\n\n*🔰 ANTIBOT ACTIONS INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION ❱*\n*YOU WILL GET INFO OF ANTIBOT ACTIONS*\n\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION DELETE ❱*\n*WHEN ACTION IS DELETE THEN BOT MESSAGES WILL BE DETECTED AND AUTO DELETED*\n\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION KICK ❱*\n*WHEN ACTION IS KICK THEN AS SOON AS BOT MESSAGE IS DETECTED THE USER WILL BE REMOVED*\n\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION WARN ❱*\n*WHEN ACTION IS WARN THEN USER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE USER WILL BE AUTO REMOVED FROM GROUP*\n\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION WARN ❰40❱ ❱*\n*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n\n*TYPE ❰ "+prefix+"ANTIBOT ACTION RESET ❱*\n*ANTIBOT ACTIONS WILL BE RESET*\n\n\n\n\n*TYPE ❰ "+prefix+"ANTIBOT RESET ❱*\n*TO RESET FULL ANTIBOT COMMAND*\n\n\n*ANTIBOT NOW :❱ ❰ "+boolOnOff(enabled)+" ❱*\n*ACTION :❱ ❰ "+strings.ToUpper(curAction)+" ❱*\n*MAX WARNINGS :❱ ❰ "+strconv.Itoa(maxW)+" ❱*\n*ALLOWED BOT JIDS QUANTITY :❱ ❰ "+strconv.Itoa(len(allowed))+" ❱*")
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	// ON
	if sub == "on" {
		antiSetEnabled(s, groupJID, antibotFeature, true)
		curAction := antiAction(s, groupJID, antibotFeature)
		maxW := antiMaxWarnings(s, groupJID, antibotFeature)
		s.Reply(info, "*🔰 ANTIBOT ACTIVATED 🔰*\n\n*ACTION :❱ "+strings.ToUpper(curAction)+"*\n\n*🔰 ACTION INFO 🔰*\n"+antiActionInfo(curAction, maxW, "BOT MESSAGES"))
		return
	}

	// OFF
	if sub == "off" {
		antiSetEnabled(s, groupJID, antibotFeature, false)
		s.Reply(info, "*🔰 ANTIBOT DE-ACTIVATED 🔰*")
		return
	}

	// ACTION (shared tree)
	if sub == "action" {
		handleAntiAction(s, info, args[1:], prefix, antibotFeature, antibotUpper)
		return
	}

	// ALLOW <jid>
	if sub == "allow" {
		jidArg := strings.TrimSpace(strings.Join(args[1:], " "))
		if jidArg == "" {
			s.Reply(info, "*🔰 ANTIBOT ALLOW 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBOT ALLOW 1234567890@s.whatsapp.net ❱*")
			return
		}
		_ = s.GroupSetAdd(groupJID, antibotAllowedSet, jidArg)
		s.Reply(info, "*🔰 BOT JID ALLOWED :❱ "+jidArg+"*\n\n*Is bot JID pe ab antibot action nahi hoga.*")
		return
	}

	// DELETE <jid>
	if sub == "delete" {
		jidArg := strings.TrimSpace(strings.Join(args[1:], " "))
		if jidArg == "" {
			s.Reply(info, "*🔰 ANTIBOT DELETE 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBOT DELETE 1234567890@s.whatsapp.net ❱*")
			return
		}
		members := s.GroupSetMembers(groupJID, antibotAllowedSet)
		found := false
		for _, m := range members {
			if m == jidArg {
				found = true
				break
			}
		}
		if found {
			_ = s.GroupSetRem(groupJID, antibotAllowedSet, jidArg)
			s.Reply(info, "*🔰 BOT JID REMOVED FROM WHITELIST :❱ "+jidArg+"*")
		} else {
			s.Reply(info, "*🔰 BOT JID NOT FOUND IN WHITELIST :❱ "+jidArg+"*")
		}
		return
	}

	// ALLOWEDLIST
	if sub == "allowedlist" {
		allowed := AntibotAllowedJIDs(s, groupJID)
		if len(allowed) == 0 {
			s.Reply(info, "*🔰 ALLOWED LIST EMPTY*")
			return
		}
		var sb strings.Builder
		for i, j := range allowed {
			sb.WriteString(strconv.Itoa(i+1) + ". " + j + "\n")
		}
		s.Reply(info, "*🔰 ANTIBOT ALLOWED BOT JIDS 🔰*\n\n"+sb.String()+"\n*TOTAL :❱ "+strconv.Itoa(len(allowed))+"*")
		return
	}

	// RESET
	if sub == "reset" {
		antiResetSettings(s, groupJID, antibotFeature, antibotAllowedSet)
		s.Reply(info, "*🔰 ANTIBOT FULLY RESET*\n\n*STATUS :❱ OFF*\n*ACTION :❱ WARN*\n*MAX WARNINGS :❱ "+strconv.Itoa(defaultAntiMaxWarnings)+"*\n*ALLOWED LIST :❱ CLEARED*\n*WARNINGS :❱ CLEARED*")
		return
	}

	// Unknown
	s.Reply(info, "*TYPE ❰ ANTIBOT ❱ FOR HELP*")
}

// ── registration ───────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antibot", Category: "ANTI & PROTECTION", Desc: "Block bot-forwarded messages in group", OwnerOnly: true, Run: handleAntibot})
	Register(Command{Name: "ab", OwnerOnly: true, Hidden: true, Run: handleAntibot})
}
