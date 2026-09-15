package goldcmds

// ============================================================================
// GOLD-MD — .antigccall command (aliases: .antigcall, .antgccall)
//
// Group-call control — same-to-same .antilink style. FINAL SETUP (owner order):
// sirf DO actions hain — WARN (default) + KICK. Controls GROUP CALLS started by
// group members (CallOfferNotice + CallOffer ring in manager.go):
//   .antigccall                   → full command info
//   .antigccall on / off          → enable / disable group-call control
//   .antigccall action            → action info (warn / kick)
//   .antigccall action kick       → caller DIRECT remove as soon as group call detected
//   .antigccall action warn       → caller ko warning, max warnings pe auto remove
//   .antigccall action warn <n>   → set action=warn + max-warnings=n (1-50)
//   .antigccall action warn reset → reset max-warnings + clear all warnings
//   .antigccall reset             → full reset (off + warn)
//
// Per-group config in Redis (settings:<groupJID>) — same pattern as antilink:
//   field "antigccall"         = on/off
//   field "antigccall:action"  = warn/kick
//
// ENFORCEMENT (owner order — direct action, caller-only):
//   - kick → caller (call banane wala) DIRECT group se remove — WhatsApp
//     removed participant ko ongoing group call se bhi force-kick karta hai,
//     creator nikal gaya to call sab ke liye khatam
//   - warn → caller ko warning message, max warnings pe caller remove
//   - Owner bypass: the owner's own group calls pass silently
//   - Premium bypass (.antigccallprem add): premium calls pass silently
//
// NOTE: 🔰 emoji style, ❰ ❱ markers — same as antilink.
// ============================================================================

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const antigccallFeature = "antigccall"
const antigccallUpper = "ANTIGCCALL"
const antigccallDefaultAction = "warn"

// ── exported helpers for the manager.go enforcement handler ──────────────────

// AntigccallIsOn reports whether antigccall is enabled in the given group.
func AntigccallIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antigccallFeature)
}

// AntigccallAction returns the configured action for the group:
// "warn" (default) / "kick".
func AntigccallAction(s SessionBridge, groupJID string) string {
	v := strings.ToLower(s.GetGroupSetting(groupJID, antigccallFeature+":action", antigccallDefaultAction))
	if v != "warn" && v != "kick" {
		return antigccallDefaultAction
	}
	return v
}

// AntigccallMaxWarnings returns the max-warnings limit for the warn action.
func AntigccallMaxWarnings(s SessionBridge, groupJID string) int {
	return antiMaxWarnings(s, groupJID, antigccallFeature)
}

// AntigccallIncWarn increments the caller's warning count and returns the new value.
func AntigccallIncWarn(s SessionBridge, groupJID, userJID string) int {
	return antiIncWarning(s, groupJID, antigccallFeature, userJID)
}

// AntigccallResetWarn clears the caller's warning count (after max-warnings kick).
func AntigccallResetWarn(s SessionBridge, groupJID, userJID string) {
	antiResetWarning(s, groupJID, antigccallFeature, userJID)
}

// ── info text builders ───────────────────────────────────────────────────────

// gccallFullInfo is the full .antigccall command info (antilink style).
func gccallFullInfo(prefix string, enabled bool, curAction string) string {
	return "*\U0001F530 ANTIGCCALL COMMAND INFO \U0001F530*\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ON ❱*\n" +
		"*WHEN ANTIGCCALL IS ON THEN IF ANY MEMBER STARTS A GROUP CALL IN THIS GROUP THE BOT WILL DETECT THE GROUP CALL AND ACTION WILL APPLY ON THE CALLER*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL OFF ❱*\n" +
		"*WHEN ANTIGCCALL IS OFF THEN ALL MEMBERS CAN FREELY START GROUP CALLS IN THIS GROUP*\n\n\n" +
		"*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION ❱*\n" +
		"*YOU WILL GET INFO OF ANTIGCCALL ACTIONS*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION KICK ❱*\n" +
		"*WHEN ACTION IS KICK THEN AS SOON AS GROUP CALL IS DETECTED THE CALLER WILL BE REMOVED AND CALL WILL BE CLOSED*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION WARN ❱*\n" +
		"*WHEN ACTION IS WARN THEN THE CALLER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE CALLER WILL BE AUTO REMOVED FROM GROUP*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION WARN ❰40❱ ❱*\n" +
		"*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL RESET ❱*\n" +
		"*TO RESET FULL ANTIGCCALL COMMAND*\n\n" +
		"*ANTIGCCALL NOW :❱ ❰ " + boolOnOff(enabled) + " ❱*\n" +
		"*ACTION :❱ ❰ " + strings.ToUpper(curAction) + " ❱*"
}

// gccallActionsInfo is the ".antigccall action" info (antilink actions style).
func gccallActionsInfo(prefix string, curAction string) string {
	return "*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION KICK ❱*\n" +
		"*WHEN ACTION IS KICK THEN AS SOON AS GROUP CALL IS DETECTED THE CALLER WILL BE REMOVED AND CALL WILL BE CLOSED*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION WARN ❱*\n" +
		"*WHEN ACTION IS WARN THEN THE CALLER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE CALLER WILL BE AUTO REMOVED FROM GROUP*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION WARN ❰40❱ ❱*\n" +
		"*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n\n" +
		"*TYPE ❰ " + prefix + "ANTIGCCALL ACTION WARN RESET ❱*\n" +
		"*TO RESET WARNINGS*\n\n" +
		"*CURRENT ACTION :❱ ❰ " + strings.ToUpper(curAction) + " ❱*"
}

// ── command handler ──────────────────────────────────────────────────────────

func handleAntiGcCall(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAntiGcCallAsync(s, info, args, prefix)
}

func handleAntiGcCallAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Group check only — NO admin check, NO member check. The command itself
	// is OwnerOnly, so the dispatcher already gates who can run it. Direct
	// action apply (owner order).
	if !info.IsGroup {
		s.Reply(info, "*"+antigccallUpper+" ONLY WORKS IN GROUPS*")
		return
	}
	groupJID := info.Chat.String()

	// No args → full command info (same style as .antilink)
	if len(args) == 0 {
		curAction := AntigccallAction(s, groupJID)
		enabled := AntigccallIsOn(s, groupJID)
		s.Reply(info, gccallFullInfo(prefix, enabled, curAction))
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	// ON
	if sub == "on" {
		antiSetEnabled(s, groupJID, antigccallFeature, true)
		curAction := AntigccallAction(s, groupJID)
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTIVATED \U0001F530*\n\n*ACTION :❱ ❰ "+strings.ToUpper(curAction)+" ❱*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo(curAction))
		return
	}

	// OFF
	if sub == "off" {
		antiSetEnabled(s, groupJID, antigccallFeature, false)
		s.Reply(info, "*\U0001F530 ANTIGCCALL DE-ACTIVATED \U0001F530*")
		return
	}

	// ACTION (warn / kick tree)
	if sub == "action" {
		handleAntiGcCallAction(s, info, args[1:], prefix)
		return
	}

	// RESET — full reset (off + action back to warn)
	if sub == "reset" {
		antiSetEnabled(s, groupJID, antigccallFeature, false)
		s.SetGroupSetting(groupJID, antigccallFeature+":action", antigccallDefaultAction)
		antiClearAllWarnings(s, groupJID, antigccallFeature)
		s.Reply(info, "*\U0001F530 ANTIGCCALL RESET DONE \U0001F530*\n\n*ANTIGCCALL NOW :❱ ❰ OFF ❱*\n*ACTION :❱ ❰ WARN ❱*")
		return
	}

	// Unknown subcommand → full command info again
	curAction := AntigccallAction(s, groupJID)
	enabled := AntigccallIsOn(s, groupJID)
	s.Reply(info, gccallFullInfo(prefix, enabled, curAction))
}

// handleAntiGcCallAction — the "action" sub-command tree (warn / kick only).
// NO admin check — direct set + apply (owner order).
func handleAntiGcCallAction(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupJID := info.Chat.String()

	// ".antigccall action" with no arg → action info
	if len(args) == 0 {
		curAction := AntigccallAction(s, groupJID)
		s.Reply(info, gccallActionsInfo(prefix, curAction))
		return
	}

	actionArg := strings.ToLower(strings.TrimSpace(args[0]))

	// KICK — caller direct remove on group call detection
	if actionArg == "kick" {
		antiSetAction(s, groupJID, antigccallFeature, "kick")
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :❱ ❰ KICK ❱*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("kick"))
		return
	}

	// WARN — warning to caller, max warnings pe auto remove (default action)
	if actionArg == "warn" {
		if len(args) >= 2 {
			warnSubArg := strings.ToLower(strings.TrimSpace(args[1]))
			if warnSubArg == "reset" {
				antiResetMaxWarnings(s, groupJID, antigccallFeature)
				antiSetAction(s, groupJID, antigccallFeature, "warn")
				antiClearAllWarnings(s, groupJID, antigccallFeature)
				s.Reply(info, "*\U0001F530 ANTIGCCALL WARN LIMIT RESET TO DEFAULT ("+strconv.Itoa(defaultAntiMaxWarnings)+")*\n*ALL USERS' WARNINGS CLEARED*")
				return
			}
			if isAllDigits(warnSubArg) {
				requested, _ := strconv.Atoi(warnSubArg)
				clamped := antiSetMaxWarnings(s, groupJID, antigccallFeature, requested)
				antiSetAction(s, groupJID, antigccallFeature, "warn")
				note := ""
				if requested > absoluteAntiMaxWarnings {
					note = "\n*(Max limit " + strconv.Itoa(absoluteAntiMaxWarnings) + " hai, isi pe clamp kar diya gaya)*"
				}
				s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :❱ ❰ WARN ❱*\n*MAX WARNINGS :❱ "+strconv.Itoa(clamped)+"*"+note)
				return
			}
		}
		antiSetAction(s, groupJID, antigccallFeature, "warn")
		maxW := antiMaxWarnings(s, groupJID, antigccallFeature)
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :❱ ❰ WARN ❱*\n*MAX WARNINGS :❱ "+strconv.Itoa(maxW)+"*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("warn"))
		return
	}

	// Unknown action arg → action info again
	curAction := AntigccallAction(s, groupJID)
	s.Reply(info, gccallActionsInfo(prefix, curAction))
}

// gccallActionInfo returns the ACTION INFO line shown when the feature is
// turned ON or the action is set (same pattern as antiActionInfo).
func gccallActionInfo(action string) string {
	switch action {
	case "kick":
		return "*ACTION IS KICK — NOW BOT WILL REMOVE THE CALLER AS SOON AS GROUP CALL IS DETECTED AND CALL WILL BE CLOSED*"
	case "warn":
		return "*ACTION IS WARN — NOW BOT WILL GIVE WARNINGS TO THE CALLER AS SOON AS WARNINGS ARE FINISHED THE CALLER WILL BE REMOVED FROM GROUP*"
	default:
		return "*ACTION IS WARN — NOW BOT WILL GIVE WARNINGS TO THE CALLER AS SOON AS WARNINGS ARE FINISHED THE CALLER WILL BE REMOVED FROM GROUP*"
	}
}

// ── registration ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antigccall", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO CONTROL GROUP CALLS IN THE GROUP. IT CAN WARN OR KICK THE MEMBER WHO STARTS THE GROUP CALL.", OwnerOnly: true, Run: handleAntiGcCall})
	// hidden aliases (user spellings: antigcall / antgccall)
	Register(Command{Name: "antigcall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
	Register(Command{Name: "antgccall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
}
