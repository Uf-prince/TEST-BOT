package goldcmds

// ============================================================================
// GOLD-MD — Shared helpers for antilink / antibot / antibad commands.
//
// All three anti-commands share the exact same sub-command structure:
//   on / off / action (warn|delete|kick + warn limit + reset) / reset
// plus per-group on/off, action, max-warnings stored in Redis, and per-user
// warning counters. This file centralises that logic so each anti-*.go only
// needs its own feature-specific bits (link extraction, forward detection,
// bad-word matching) plus its command text.
//
// Per-GROUP config stored in Redis (Upstash) via settings:<groupJID> hash:
//   field "<feature>"          = "on"/"off"
//   field "<feature>:action"   = "warn"/"delete"/"kick"
//   field "<feature>:maxwarn"  = "1".."50"
//   field "warn:<feature>:<userJID>" = current warning count for that user
//   SET  "warnusers:<feature>"  = which users have warnings (for reset wipe)
//
// Whitelists (allowed domains / allowed bot JIDs) and custom bad-word lists
// are stored in per-group Redis SETs (see GroupSetAdd/Rem/Members in the
// bridge), so each feature manages its own SET name.
//
// Constants mirror Node.js pair.js:
//   DEFAULT_ANTILINK_ACTION='warn', DEFAULT_ANTILINK_MAX_WARNINGS=50,
//   ANTILINK_ABSOLUTE_MAX_WARNINGS=50  (same for antibot / antibad).
// ============================================================================

import (
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── constants (same as Node.js pair.js) ─────────────────────────────────

const defaultAntiMaxWarnings = 50
const absoluteAntiMaxWarnings = 50
const defaultAntiAction = "warn"

// ── premium bypass helper ───────────────────────────────────────────────────
//
// premiumSenderBypass returns true if the sender of `info` is in the shared
// premium/whitelist list ("premium:set") and therefore must be EXEMPT from
// anti-* enforcement (antilink / antibad / antibot / antistatus).
//
// It checks TWO ways so it works regardless of how the premium user was added:
//
//  1. EXACT JID match — PremiumIsMember(senderJID) and also the SenderAlt JID
//     (in groups whatsmeow may report the sender as a LID while the premium
//     list stores the phone JID, or vice-versa).
//
//  2. NUMBER match — the user may have been added with a LOCAL number format
//     (e.g. ".antilinkprem 03274765023" stores "03274765023@s.whatsapp.net")
//     while the actual WhatsApp sender JID uses the INTERNATIONAL format
//     ("923274765023@s.whatsapp.net"). To handle this we compare the LAST
//     10 digits of both numbers — "3274765023" matches in both cases.
//
// This mirrors the UMAR-MD Node.js behaviour where the premium check is
// number-tolerant.
func premiumSenderBypass(s SessionBridge, info types.MessageInfo) bool {
	senderJID := info.Sender.String()

	// 1. exact JID match (primary + alt)
	if s.PremiumIsMember(senderJID) {
		return true
	}
	if info.SenderAlt.Server != "" {
		if s.PremiumIsMember(info.SenderAlt.String()) {
			return true
		}
	}

	// 2. number-based match against the whole premium list.
	// Uses PremiumMemberTails() which is CACHED in-memory (0ms after first
	// call, never a network round-trip) so the bot speed is unaffected.
	senderNum := jidNumber(senderJID)
	senderTail := numberTail(senderNum)
	if senderTail == "" {
		return false
	}
	for _, tail := range s.PremiumMemberTails() {
		if tail == senderTail {
			return true
		}
	}
	return false
}

// jidNumber returns the user-identifier part of a JID (before '@').
func jidNumber(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

// numberTail keeps only digits and returns the last 10 (enough to match across
// local "0327..." vs international "9232..." formats). Returns "" if there are
// no digits.
func numberTail(num string) string {
	var digits []byte
	for i := 0; i < len(num); i++ {
		c := num[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	if len(digits) == 0 {
		return ""
	}
	if len(digits) > 10 {
		digits = digits[len(digits)-10:]
	}
	return string(digits)
}

// ── settings read/write (per-group Redis hash) ─────────────────────────

// antiEnabled reads the on/off field for a feature in a group.
func antiEnabled(s SessionBridge, groupJID, feature string) bool {
	v := s.GetGroupSetting(groupJID, feature, "off")
	return v == "on" || v == "true" || v == "1"
}

// antiSetEnabled writes the on/off field.
func antiSetEnabled(s SessionBridge, groupJID, feature string, on bool) {
	if on {
		s.SetGroupSetting(groupJID, feature, "on")
	} else {
		s.SetGroupSetting(groupJID, feature, "off")
	}
}

// antiAction reads the configured action (warn/delete/kick).
func antiAction(s SessionBridge, groupJID, feature string) string {
	v := strings.ToLower(s.GetGroupSetting(groupJID, feature+":action", defaultAntiAction))
	if v != "warn" && v != "delete" && v != "kick" {
		return defaultAntiAction
	}
	return v
}

// antiSetAction writes the action field.
func antiSetAction(s SessionBridge, groupJID, feature, action string) {
	s.SetGroupSetting(groupJID, feature+":action", strings.ToLower(action))
}

// antiMaxWarnings reads the max-warnings limit (clamped 1..absolute).
func antiMaxWarnings(s SessionBridge, groupJID, feature string) int {
	v := s.GetGroupSetting(groupJID, feature+":maxwarn", strconv.Itoa(defaultAntiMaxWarnings))
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultAntiMaxWarnings
	}
	if n > absoluteAntiMaxWarnings {
		return absoluteAntiMaxWarnings
	}
	return n
}

// antiSetMaxWarnings writes the max-warnings limit, clamped to 1..absolute.
// Returns the clamped value actually stored.
func antiSetMaxWarnings(s SessionBridge, groupJID, feature string, requested int) int {
	if requested < 1 {
		requested = 1
	}
	if requested > absoluteAntiMaxWarnings {
		requested = absoluteAntiMaxWarnings
	}
	s.SetGroupSetting(groupJID, feature+":maxwarn", strconv.Itoa(requested))
	return requested
}

// antiResetMaxWarnings resets the max-warnings to the default.
func antiResetMaxWarnings(s SessionBridge, groupJID, feature string) {
	s.SetGroupSetting(groupJID, feature+":maxwarn", strconv.Itoa(defaultAntiMaxWarnings))
}

// ── per-user warning counters ──────────────────────────────────────────
//
// Stored as a field "warn:<feature>:<userJID>" inside the settings:<groupJID>
// hash. A companion SET "warnusers:<feature>" tracks which users have a
// non-zero count so that reset can wipe them all without HGETALL.

// warnField builds the Redis hash field for a user's warning count.
func warnField(feature, userJID string) string {
	return "warn:" + feature + ":" + userJID
}

// warnUsersSet returns the SET name that tracks which users have warnings.
func warnUsersSet(feature string) string {
	return "warnusers:" + feature
}

// antiIncWarning increments and returns a user's warning count.
func antiIncWarning(s SessionBridge, groupJID, feature, userJID string) int {
	cur := s.GetGroupSetting(groupJID, warnField(feature, userJID), "0")
	n, _ := strconv.Atoi(cur)
	n++
	s.SetGroupSetting(groupJID, warnField(feature, userJID), strconv.Itoa(n))
	_ = s.GroupSetAdd(groupJID, warnUsersSet(feature), userJID)
	return n
}

// antiResetWarning zeroes a single user's warning count.
func antiResetWarning(s SessionBridge, groupJID, feature, userJID string) {
	s.SetGroupSetting(groupJID, warnField(feature, userJID), "0")
	_ = s.GroupSetRem(groupJID, warnUsersSet(feature), userJID)
}

// antiClearAllWarnings wipes every user's warning count for a feature in a
// group, using the warnusers SET to know who to wipe.
func antiClearAllWarnings(s SessionBridge, groupJID, feature string) {
	users := s.GroupSetMembers(groupJID, warnUsersSet(feature))
	for _, u := range users {
		s.SetGroupSetting(groupJID, warnField(feature, u), "0")
	}
	_ = s.GroupSetClear(groupJID, warnUsersSet(feature))
}

// ── full reset ─────────────────────────────────────────────────────────

// antiResetSettings does a full reset: turn off, set action to default,
// reset max-warnings to default, clear all per-user warnings, and clear the
// given whitelist SET (so callers pass their feature's SET name, or "" to
// skip). Mirrors UmarResetAntilinkSettings / UmarResetAntibotSettings /
// UmarResetAntibadSettings.
func antiResetSettings(s SessionBridge, groupJID, feature, whitelistSet string) {
	antiSetEnabled(s, groupJID, feature, false)
	antiSetAction(s, groupJID, feature, defaultAntiAction)
	antiResetMaxWarnings(s, groupJID, feature)
	antiClearAllWarnings(s, groupJID, feature)
	if whitelistSet != "" {
		_ = s.GroupSetClear(groupJID, whitelistSet)
	}
}

// ── owner + group gate ─────────────────────────────────────────────────

// requireGroupOwner checks that the message is from a group AND the sender is
// the owner. Replies with the Node.js-style rejection messages and returns
// false if either check fails. Uses the default "ANTILINK" feature name for
// the non-group message (antilink/antibot/antibad all share this).
func requireGroupOwner(s SessionBridge, info types.MessageInfo) bool {
	return requireGroupOwnerNamed(s, info, "ANTILINK")
}

// requireGroupOwnerNamed is the same as requireGroupOwner but lets the caller
// specify the feature name (upper-case) that appears in the non-group
// rejection message, e.g. "ANTISTATUS ONLY WORKS IN GROUPS".
func requireGroupOwnerNamed(s SessionBridge, info types.MessageInfo, featureUpper string) bool {
	if !info.IsGroup {
		s.Reply(info, "*"+featureUpper+" ONLY WORKS IN GROUPS*")
		return false
	}
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR OWNER 😎*")
		return false
	}
	return true
}

// ── action info line (for ON reply) ────────────────────────────────────

// antiActionInfo returns the "ACTION INFO" line shown when a feature is
// turned ON, matching the Node.js text for each action type.
func antiActionInfo(action string, maxW int, offenceLabel string) string {
	switch action {
	case "delete":
		return "*ACTION IS DELETE — NOW BOT WILL DELETE ALL " + offenceLabel + "*"
	case "kick":
		return "*ACTION IS KICK — NOW BOT WILL REMOVE USER AS SOON AS " + offenceLabel + " IS SENT*"
	default:
		return fmt.Sprintf("*ACTION IS WARN — USER WILL GET WARNINGS, AFTER %d WARNINGS USER WILL BE AUTO REMOVED*", maxW)
	}
}

// ── shared "action" sub-command tree ───────────────────────────────────
//
// handleAntiAction processes the ".<feature> action ..." sub-command that is
// identical across antilink/antibot/antibad:
//
//	action              → show action info
//	action delete       → set action=delete
//	action kick         → set action=kick
//	action warn         → set action=warn (keep current max)
//	action warn <n>     → set action=warn + set max-warnings=n (clamped)
//	action warn reset   → reset max-warnings to default + clear all warnings
//	action reset        → (antibad only) full action reset
//
// prefix = the bot's command prefix (for example text).
// feature = "antilink" / "antibot" / "antibad" (Redis field prefix).
// upperName = "ANTILINK" / "ANTIBOT" / "ANTIBAD" (for reply text).
// warnUsersClear is called on warn-reset to wipe per-user warning counts
// (passed as a closure so each feature can use its own warning store).
func handleAntiAction(s SessionBridge, info types.MessageInfo, args []string, prefix, feature, upperName string) {
	if len(args) == 0 {
		// ".<feature> action" with no action arg → show action info
		curAction := antiAction(s, info.Chat.String(), feature)
		maxW := antiMaxWarnings(s, info.Chat.String(), feature)
		s.Reply(info, "*🔰 "+upperName+" ACTIONS INFO 🔰*\n\n*TYPE ❰ "+prefix+upperName+" ACTION DELETE ❱*\n*WHEN ACTION IS DELETE THEN "+offenceLabelFor(feature)+" WILL BE DETECTED AND AUTO DELETED*\n\n\n*TYPE ❰ "+prefix+upperName+" ACTION KICK ❱*\n*WHEN ACTION IS KICK THEN AS SOON AS "+offenceArticle(feature)+" IS DETECTED THE USER WILL BE REMOVED*\n\n\n*TYPE ❰ "+prefix+upperName+" ACTION WARN ❱*\n*WHEN ACTION IS WARN THEN USER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE USER WILL BE AUTO REMOVED FROM GROUP*\n\n\n*TYPE ❰ "+prefix+upperName+" ACTION WARN ❰40❱ ❱*\n*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n*TYPE ❰ "+prefix+upperName+" ACTION WARN RESET ❱*\n*TO RESET WARNINGS*\n\n*CURRENT ACTION :❱ "+strings.ToUpper(curAction)+"*\n*MAX WARNINGS :❱ "+strconv.Itoa(maxW)+"*")
		return
	}

	actionArg := strings.ToLower(strings.TrimSpace(args[0]))
	groupJID := info.Chat.String()

	if actionArg == "delete" || actionArg == "kick" {
		antiSetAction(s, groupJID, feature, actionArg)
		s.Reply(info, "*✅ "+upperName+" ACTION SET TO :❱ "+strings.ToUpper(actionArg)+"*")
		return
	}

	if actionArg == "warn" {
		// Optional sub-arg: reset | <number>
		if len(args) >= 2 {
			warnSubArg := strings.ToLower(strings.TrimSpace(args[1]))
			if warnSubArg == "reset" {
				antiResetMaxWarnings(s, groupJID, feature)
				antiSetAction(s, groupJID, feature, "warn")
				antiClearAllWarnings(s, groupJID, feature)
				s.Reply(info, "*✅ "+upperName+" WARN LIMIT RESET TO DEFAULT ("+strconv.Itoa(defaultAntiMaxWarnings)+")*\n*ALL USERS' WARNINGS CLEARED*")
				return
			}
			if isAllDigits(warnSubArg) {
				requested, _ := strconv.Atoi(warnSubArg)
				clamped := antiSetMaxWarnings(s, groupJID, feature, requested)
				antiSetAction(s, groupJID, feature, "warn")
				note := ""
				if requested > absoluteAntiMaxWarnings {
					note = "\n*(Max limit " + strconv.Itoa(absoluteAntiMaxWarnings) + " hai, isi pe clamp kar diya gaya)*"
				}
				s.Reply(info, "*✅ "+upperName+" ACTION SET TO :❱ WARN*\n*MAX WARNINGS :❱ "+strconv.Itoa(clamped)+"*"+note)
				return
			}
		}
		// ".<feature> action warn" with no number → just set action
		antiSetAction(s, groupJID, feature, "warn")
		maxW := antiMaxWarnings(s, groupJID, feature)
		s.Reply(info, "*✅ "+upperName+" ACTION SET TO :❱ WARN*\n*MAX WARNINGS :❱ "+strconv.Itoa(maxW)+"*")
		return
	}

	// antibad has an extra "action reset" sub-command (full action reset)
	if actionArg == "reset" {
		antiSetAction(s, groupJID, feature, defaultAntiAction)
		antiResetMaxWarnings(s, groupJID, feature)
		antiClearAllWarnings(s, groupJID, feature)
		s.Reply(info, "*✅ "+upperName+" ACTION RESET TO DEFAULTS*")
		return
	}

	s.Reply(info, "*❌ INVALID ACTION*\n\n*USE :* WARN / DELETE / KICK")
}

// ── EnforceAntiAction — the detection enforcement (delete/kick/warn) ────
//
// Called by each feature's CheckAndEnforce when an offence is detected.
// Mirrors the Node.js action blocks (antilink/antibot/antibad handlers).
//
//	feature    = "antilink" / "antibot" / "antibad"
//	upperName  = "ANTILINK" / "ANTIBOT" / "ANTIBAD"
//	offenceLabel  = word used in the delete message, e.g. "LINKS" / "BOT
//	                MESSAGES" / "BAD WORDS"
//	kickedLabel   = word used in the kick message, e.g. "LINKS NOT ALLOWED"
//	                / "OTHER BOTS NOT ALLOWED" / "BAD WORDS NOT ALLOWED"
//	senderJID  = the offender's full JID (user@s.whatsapp.net)
//	senderNum  = the offender's phone number (without @server)
//
// It always tries to delete the offending message first, then applies the
// configured action (delete → just notify; kick → remove + notify; warn →
// increment count, kick if max reached, else warn message).
func EnforceAntiAction(s SessionBridge, info types.MessageInfo, feature, upperName, offenceLabel, kickedLabel, senderJID, senderNum string) {
	defer func() {
		if r := recover(); r != nil {
			// silent — like Node.js .catch(()=>{})
		}
	}()

	// ── PREMIUM BYPASS (mirrors UMAR-MD Node.js) ──
	// If the sender is in the premium/whitelist list (same shared "premium:set"
	// used by .antilinkprem / .antibadprem / .antibotprem / .antistatusprem),
	// skip ALL enforcement — no delete, no kick, no warn.
	// premiumSenderBypass checks exact JID + SenderAlt AND number-based match
	// (so a user added with a local number like 0327... still matches the
	// international sender JID 9232...).
	if premiumSenderBypass(s, info) {
		return
	}

	groupJID := info.Chat.String()
	action := antiAction(s, groupJID, feature)
	maxW := antiMaxWarnings(s, groupJID, feature)

	// Always try to delete the offending message first.
	_ = s.DeleteAnyMessage(info.Chat, senderJID, info.ID)

	switch action {
	case "delete":
		s.ReplyWithMentions(info, "*🗑️ "+offenceLabel+" DELETED — "+offenceLabel+" NOT ALLOWED IN THIS GROUP*", []string{senderJID})

	case "kick":
		if err := s.KickGroupMember(info.Chat, []types.JID{info.Sender}); err != nil {
			s.Reply(info, "*⚠️ COULD NOT REMOVE USER — BOT NEEDS ADMIN RIGHTS*")
			return
		}
		s.ReplyWithMentions(info, "*🚫 @"+senderNum+" REMOVED — "+kickedLabel+"*", []string{senderJID})

	default: // warn
		newCount := antiIncWarning(s, groupJID, feature, senderJID)
		if newCount >= maxW {
			// Max warnings reached → kick
			if err := s.KickGroupMember(info.Chat, []types.JID{info.Sender}); err != nil {
				s.ReplyWithMentions(info, "*⚠️ @"+senderNum+" REACHED MAX WARNINGS, BUT BOT NEEDS ADMIN RIGHTS TO REMOVE*", []string{senderJID})
				return
			}
			s.ReplyWithMentions(info, "*🚫 @"+senderNum+" REMOVED — MAX WARNINGS ("+strconv.Itoa(maxW)+") REACHED*", []string{senderJID})
			antiResetWarning(s, groupJID, feature, senderJID)
		} else {
			s.ReplyWithMentions(info, "*⚠️ @"+senderNum+" "+offenceLabel+" NOT ALLOWED!*\n*WARNING :❱ "+strconv.Itoa(newCount)+"/"+strconv.Itoa(maxW)+"*", []string{senderJID})
		}
	}
}

// ── small text helpers ─────────────────────────────────────────────────

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func boolOnOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// offenceLabelFor returns the label word for the ACTION INFO delete line.
func offenceLabelFor(feature string) string {
	switch feature {
	case "antilink":
		return "LINKS"
	case "antibot":
		return "BOT MESSAGES"
	case "antibad":
		return "BAD WORDS"
	}
	return "MESSAGES"
}

// offenceArticle returns "LINK" / "BOT MESSAGE" / "BAD WORD" for the kick line.
func offenceArticle(feature string) string {
	switch feature {
	case "antilink":
		return "LINK"
	case "antibot":
		return "BOT MESSAGE"
	case "antibad":
		return "BAD WORD"
	}
	return "MESSAGE"
}
