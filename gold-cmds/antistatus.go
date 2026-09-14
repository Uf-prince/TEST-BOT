package goldcmds

// ============================================================================
// GOLD-MD — .antistatus command (aliases: .as)
//
// Ported from UMAR-MD (Node.js pair.js, lines 14485-14668) — SAME TEXT
// (0% farak), SAME WORK (0% farak):
//   .antistatus                  → full command info
//   .antistatus on / off         → enable / disable status-mention detection
//   .antistatus action           → action info
//   .antistatus action delete    → set action=delete
//   .antistatus action kick      → set action=kick
//   .antistatus action warn      → set action=warn
//   .antistatus action warn <n>  → set action=warn + max-warnings=n (1-50)
//   .antistatus action warn reset → reset max-warnings to default
//   .antistatus reset            → full reset
//
// Group-only, owner-only. Per-group config in Redis (settings:<groupJID>):
//   field "antistatus"           = on/off
//   field "antistatus:action"    = warn/delete/kick
//   field "antistatus:maxwarn"   = 1-50
//   field "antistatus:warn:<userJID>" = per-user warning count
//
// Uses the shared anti_common.go helpers (antiEnabled, antiSetEnabled, etc.)
// with feature="antistatus". DEFAULT max-warnings = 50, ABSOLUTE max = 50.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"strconv"
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

const antistatusFeature = "antistatus"
const antistatusUpper = "ANTISTATUS"

// antistatusMaxWarnings reads the max-warnings limit (clamped 1..50).
func antistatusMaxWarnings(s SessionBridge, groupJID string) int {
	return antiMaxWarnings(s, groupJID, antistatusFeature)
}

// antistatusActionInfo returns the action info line shown when antistatus is
// turned ON, matching the Node.js text for each action type.
func antistatusActionInfo(action string, maxW int) string {
	switch action {
	case "delete":
		return "*ACTION IS DELETE — NOW BOT WILL DELETE GROUP STATUS POSTS*"
	case "kick":
		return "*ACTION IS KICK — NOW BOT WILL REMOVE USER AS SOON AS A GROUP STATUS POST IS DETECTED*"
	default:
		return "*ACTION IS WARN — USER WILL GET WARNINGS, AFTER " + strconv.Itoa(maxW) + " WARNINGS USER WILL BE AUTO REMOVED*"
	}
}

// ── command handler ───────────────────────────────────────────────────────

func handleAntistatus(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAntistatusAsync(s, info, args, prefix)
}

func handleAntistatusAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroupOwnerNamed(s, info, antistatusUpper) {
		return
	}
	groupJID := info.Chat.String()

	// Read current settings
	enabled := antiEnabled(s, groupJID, antistatusFeature)
	curAction := antiAction(s, groupJID, antistatusFeature)
	maxW := antistatusMaxWarnings(s, groupJID)

	// No args → full command info
	if len(args) == 0 {
		s.Reply(info, "*🔰 ANTISTATUS COMMAND INFO 🔰*\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ON ⦊*\n*WHEN ANTISTATUS IS ON THEN IF ANY MEMBER POSTS A GROUP STATUS (WHATSAPP STATUS SENT DIRECTLY INSIDE THIS GROUP) THE BOT WILL DETECT IT AND ACTION WILL APPLY*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS OFF ⦊*\n*WHEN ANTISTATUS IS OFF THEN NO ACTION WILL APPLY ON GROUP STATUS POSTS*\n\n*🔰 ANTISTATUS ACTIONS INFO 🔰*\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION ⦊*\n*YOU WILL GET INFO OF ANTISTATUS ACTIONS*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION DELETE ⦊*\n*WHEN ACTION IS DELETE THEN THE GROUP STATUS POST WILL BE AUTO DELETED*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION KICK ⦊*\n*WHEN ACTION IS KICK THEN AS SOON AS A GROUP STATUS POST IS DETECTED THE USER WILL BE REMOVED*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN ⦊*\n*WHEN ACTION IS WARN THEN USER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE USER WILL BE AUTO REMOVED FROM GROUP*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN ⦬24⦭ ⦊*\n*SET YOUR WARNINGS AS MANY AS YOU WANT 8 24 30 AS YOU WISH MAX ⦬ 50 ⦭ ONLY*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN RESET ⦊*\n*ANTISTATUS WARN LIMIT WILL RESET TO DEFAULT ("+strconv.Itoa(defaultAntiMaxWarnings)+")*\n\n\n*TYPE ⦉ "+prefix+"ANTISTATUS RESET ⦊*\n*TO RESET FULL ANTISTATUS COMMAND*\n\n\n*TYPE ⦉ "+prefix+"ANTISPREM ADD ⦊ FOR INFO*\n*TYPE ⦉ "+prefix+"ANTISPREM DELETE ⦊ FOR INFO*\n*TYPE ⦉ "+prefix+"ANTISPREM LIST ⦊ FOR INFO*\n\n\n*ANTISTATUS NOW ⦗ ⦉ "+boolOnOff(enabled)+" ⦊*\n*ACTION ⦗ ⦉ "+strings.ToUpper(curAction)+" ⦊*\n*MAX WARNINGS ⦗ ⦉ "+strconv.Itoa(maxW)+" ⦊*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	// ── ON / OFF ──
	if subCmd == "on" {
		antiSetEnabled(s, groupJID, antistatusFeature, true)
		s.Reply(info, "*🔰 ANTISTATUS ACTIVATED 🔰*\n\n*ACTION ⦗ "+strings.ToUpper(curAction)+"*\n\n*🔰 ACTION INFO 🔰*\n"+antistatusActionInfo(curAction, maxW))
		return
	}

	if subCmd == "off" {
		antiSetEnabled(s, groupJID, antistatusFeature, false)
		s.Reply(info, "*🔰 ANTISTATUS DE-ACTIVATED 🔰*")
		return
	}

	// ── ACTION ──
	if subCmd == "action" {
		handleAntistatusAction(s, info, args, prefix, groupJID, curAction, maxW)
		return
	}

	// ── RESET ──
	if subCmd == "reset" {
		antiResetSettings(s, groupJID, antistatusFeature, "")
		s.Reply(info, "*🔰 ANTISTATUS FULLY RESET*\n\n*STATUS ⦗ OFF*\n*ACTION ⦗ WARN*\n*MAX WARNINGS ⦗ "+strconv.Itoa(defaultAntiMaxWarnings)+"*\n*WARNINGS ⦗ CLEARED*")
		return
	}

	// Unknown
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ⦉ ANTISTATUS ⦊ FOR HELP*")
}

// handleAntistatusAction processes the ".antistatus action ..." sub-command.
func handleAntistatusAction(s SessionBridge, info types.MessageInfo, args []string, prefix, groupJID, curAction string, maxW int) {
	actionArg := ""
	if len(args) >= 2 {
		actionArg = strings.ToLower(strings.TrimSpace(args[1]))
	}

	if actionArg == "" {
		s.Reply(info, "*🔰 ANTISTATUS ACTION 🔰*\n\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN ⦊*\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION DELETE ⦊*\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION KICK ⦊*\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN <1-50> ⦊*\n*TYPE ⦉ "+prefix+"ANTISTATUS ACTION WARN RESET ⦊*\n\n*CURRENT ACTION ⦗ "+strings.ToUpper(curAction)+"*\n*MAX WARNINGS ⦗ "+strconv.Itoa(maxW)+"*")
		return
	}

	if actionArg == "delete" || actionArg == "kick" {
		antiSetAction(s, groupJID, antistatusFeature, actionArg)
		s.Reply(info, "*🔰 ANTISTATUS ACTION SET TO ⦗ "+strings.ToUpper(actionArg)+"*")
		return
	}

	if actionArg == "warn" {
		warnSubArg := ""
		if len(args) >= 3 {
			warnSubArg = strings.ToLower(strings.TrimSpace(args[2]))
		}

		// .antistatus action warn reset
		if warnSubArg == "reset" {
			antiResetMaxWarnings(s, groupJID, antistatusFeature)
			antiSetAction(s, groupJID, antistatusFeature, "warn")
			antiClearAllWarnings(s, groupJID, antistatusFeature)
			s.Reply(info, "*🔰 ANTISTATUS WARN LIMIT RESET TO DEFAULT ("+strconv.Itoa(defaultAntiMaxWarnings)+")*\n*ALL USERS' WARNINGS CLEARED*")
			return
		}

		// .antistatus action warn <number>
		if warnSubArg != "" && isAllDigits(warnSubArg) {
			requested, _ := strconv.Atoi(warnSubArg)
			clamped := antiSetMaxWarnings(s, groupJID, antistatusFeature, requested)
			antiSetAction(s, groupJID, antistatusFeature, "warn")
			note := ""
			if requested > absoluteAntiMaxWarnings {
				note = "\n*(Max limit " + strconv.Itoa(absoluteAntiMaxWarnings) + " hai, isi pe clamp kar diya gaya)*"
			}
			s.Reply(info, "*🔰 ANTISTATUS ACTION SET TO ⦗ WARN*\n*MAX WARNINGS ⦗ "+strconv.Itoa(clamped)+"*"+note)
			return
		}

		// .antistatus action warn (no number — just set action)
		antiSetAction(s, groupJID, antistatusFeature, "warn")
		s.Reply(info, "*🔰 ANTISTATUS ACTION SET TO ⦗ WARN*\n*MAX WARNINGS ⦗ "+strconv.Itoa(maxW)+"*")
		return
	}

	// Invalid action
	s.Reply(info, "*🔰 INVALID ACTION*\n\n*USE :* WARN / DELETE / KICK")
}

// ── exported helpers for handler.go ───────────────────────────────────────

// AntistatusIsOn reports whether antistatus is enabled in the given group.
func AntistatusIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antistatusFeature)
}

// AntistatusAction returns the configured action (warn/delete/kick).
func AntistatusAction(s SessionBridge, groupJID string) string {
	return antiAction(s, groupJID, antistatusFeature)
}

// AntistatusMaxWarnings returns the max-warnings limit.
func AntistatusMaxWarnings(s SessionBridge, groupJID string) int {
	return antistatusMaxWarnings(s, groupJID)
}

// AntistatusIncWarning increments and returns a user's warning count.
func AntistatusIncWarning(s SessionBridge, groupJID, userJID string) int {
	return antiIncWarning(s, groupJID, antistatusFeature, userJID)
}

// AntistatusClearAllWarnings wipes all per-user warning counts for antistatus.
func AntistatusClearAllWarnings(s SessionBridge, groupJID string) {
	antiClearAllWarnings(s, groupJID, antistatusFeature)
}

// ── enforcement (called from handler.go) ──────────────────────────────────
//
// AntistatusCheckAndEnforce mirrors the Node.js pair.js antistatus enforcement
// (lines 16083-16160). When a GROUP message is received:
//   1. If antistatus is enabled in the group AND the message is a group status
//      mention (GroupStatusMentionMessage / GroupStatusMessage /
//      GroupStatusMessageV2 in the proto — _UmarIsStatusMentionMsg),
//   2. Check premium bypass (premium users are exempt, same list as antilinkprem),
//   3. Apply the configured action:
//        delete → delete msg + "🗑️ GROUP STATUS POST DELETED — NOT ALLOWED IN THIS GROUP"
//        kick   → delete msg + remove user + "🚫 @num REMOVED — GROUP STATUS POST NOT ALLOWED"
//        warn   → delete msg + increment warning; if max reached → kick +
//                  "🚫 @num REMOVED — MAX WARNINGS (n) REACHED" (reset warning)
//                  else → "⚠️ @num GROUP STATUS POST NOT ALLOWED! WARNING ❯ n/max"
//
// The message is always deleted first (like Node.js), then the action applies.
// Silently no-ops when antistatus is off or the message is not a status mention.

// isStatusMentionMsg checks whether a raw waProto.Message is a group status
// mention message. Mirrors _UmarIsStatusMentionMsg (pair.js lines 7651-7668):
// checks GroupStatusMentionMessage (primary), GroupStatusMessage and
// GroupStatusMessageV2 (fallbacks).
func isStatusMentionMsg(msg *waProto.Message) bool {
	if msg == nil {
		return false
	}
	return msg.GroupStatusMentionMessage != nil ||
		msg.GroupStatusMessage != nil ||
		msg.GroupStatusMessageV2 != nil
}

// AntistatusCheckAndEnforce is the enforcement entry point called from
// handler.go for every group message. It is a no-op when antistatus is off
// or the message is not a group status mention.
func AntistatusCheckAndEnforce(s SessionBridge, info types.MessageInfo) {
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()

	// Only in groups. We do NOT check IsFromMe because whatsmeow sets
	// IsFromMe=TRUE for group status mention messages even when the sender
	// is a different person (not the bot). pair.js uses `!fromMe` but in
	// Baileys fromMe=FALSE for these; whatsmeow differs. So we enforce on
	// all group messages and act on info.Sender regardless.
	if !info.IsGroup {
		return
	}

	groupJID := info.Chat.String()

	// 1. antistatus must be enabled in this group
	if !antiEnabled(s, groupJID, antistatusFeature) {
		return
	}

	// 2. the message must be a group status mention
	msg := s.GetRawMessage(info)
	isMention := isStatusMentionMsg(msg)
	if !isMention {
		return
	}

	// 3. premium bypass — premium users are exempt (same list as antilinkprem /
	//    antistatusprem / antisprem — pair.js line 13501 shares the list).
	// Uses the shared premiumSenderBypass which is number-tolerant (handles
	// local 0327... vs international 9232... JID mismatch) AND cached (0ms,
	// bot speed unaffected).
	if premiumSenderBypass(s, info) {
		return
	}

	senderJID := info.Sender.String()
	senderNum := senderJID
	if i := strings.IndexByte(senderJID, '@'); i >= 0 {
		senderNum = senderJID[:i]
	}

	action := antiAction(s, groupJID, antistatusFeature)
	maxW := antistatusMaxWarnings(s, groupJID)

	// Always try to delete the offending message first (mirrors Node.js).
	_ = s.DeleteAnyMessage(info.Chat, senderJID, info.ID)

	switch action {
	case "delete":
		s.ReplyWithMentions(info, "*🔰 GROUP STATUS POST DELETED — NOT ALLOWED IN THIS GROUP*", []string{senderJID})

	case "kick":
		kickErr := s.KickGroupMember(info.Chat, []types.JID{info.Sender})
		if kickErr != nil {
			s.Reply(info, "*🔰 COULD NOT REMOVE USER — BOT NEEDS ADMIN RIGHTS*")
			return
		}
		s.ReplyWithMentions(info, "*🔰 @"+senderNum+" REMOVED — GROUP STATUS POST NOT ALLOWED*", []string{senderJID})

	default: // warn
		newCount := antiIncWarning(s, groupJID, antistatusFeature, senderJID)
		if newCount >= maxW {
			// Max warnings reached → kick
			kickErr := s.KickGroupMember(info.Chat, []types.JID{info.Sender})
			if kickErr != nil {
				s.ReplyWithMentions(info, "*🔰 @"+senderNum+" REACHED MAX WARNINGS, BUT BOT NEEDS ADMIN RIGHTS TO REMOVE*", []string{senderJID})
				return
			}
			s.ReplyWithMentions(info, "*🔰 @"+senderNum+" REMOVED — MAX WARNINGS ("+strconv.Itoa(maxW)+") REACHED*", []string{senderJID})
			antiResetWarning(s, groupJID, antistatusFeature, senderJID)
		} else {
			s.ReplyWithMentions(info, "*🔰 @"+senderNum+" GROUP STATUS POST NOT ALLOWED!*\n*WARNING ❯ "+strconv.Itoa(newCount)+"/"+strconv.Itoa(maxW)+"*", []string{senderJID})
		}
	}
}

// ── registration ──────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antistatus", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO STOP MEMBERS FROM MENTIONING THE BOT IN THEIR STATUS. IT CAN WARN, DELETE OR KICK THEM FROM THE GROUP.", OwnerOnly: true, Run: handleAntistatus})
	Register(Command{Name: "as", OwnerOnly: true, Hidden: true, Run: handleAntistatus})
}
