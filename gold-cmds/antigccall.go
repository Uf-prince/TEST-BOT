package goldcmds

// ============================================================================
// GOLD-MD — .antigccall command (aliases: .antigcall, .antgccall)
//
// Group-call control — same-to-same .antilink style. Controls GROUP CALLS
// started by group members (events.CallOfferNotice in manager.go):
//   .antigccall                   → full command info
//   .antigccall on / off          → enable / disable group-call control
//   .antigccall action            → action info (decline / ignore / delete / kick)
//   .antigccall action decline    → SILENTLY close group calls (no notification)
//   .antigccall action ignore     → SILENTLY ignore (no action, no notification)
//   .antigccall action delete     → close group call + notice in group (antilink delete style)
//   .antigccall action kick       → close group call + remove caller (antilink kick style)
//   .antigccall debug             → last 15 JSON debug lines (how calls arrive + action)
//   .antigccall debug <n>         → last n lines (max 100)
//   .antigccall debug clear       → clear the debug file
//   .antigccall reset             → full reset (off + decline)
//
// Per-group config in Redis (settings:<groupJID>) — same pattern as antilink:
//   field "antigccall"         = on/off
//   field "antigccall:action"  = decline/ignore/delete/kick
//
// ENFORCEMENT (owner order — NO admin check, direct action):
//   - decline → RejectCall, group call closed SILENTLY, zero notification
//   - ignore  → completely silent pass — nothing at all happens
//   - delete  → RejectCall + group notice "*GROUP CALL CLOSED*" (antilink delete style)
//   - kick    → RejectCall + caller removed + notice (antilink kick style)
//   - Owner bypass: the owner's own group calls pass silently
//   - Premium bypass (.antigccallprem add): premium calls pass silently
//
// FULL JSON DEBUG — every group-call event is logged stage-by-stage to
// nexstore/gccall_debug.jsonl + bot.log (GCCALL_EVENT / GCCALL_CONFIG /
// GCCALL_BYPASS / GCCALL_DECLINE / GCCALL_KICK ...). View via .antigccall debug.
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
const antigccallDefaultAction = "decline"

// ── exported helpers for the manager.go enforcement handler ────────────────

// AntigccallIsOn reports whether antigccall is enabled in the given group.
func AntigccallIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antigccallFeature)
}

// AntigccallAction returns the configured action for the group:
// "decline" (default) / "ignore" / "delete" / "kick".
func AntigccallAction(s SessionBridge, groupJID string) string {
	v := strings.ToLower(s.GetGroupSetting(groupJID, antigccallFeature+":action", antigccallDefaultAction))
	if v != "decline" && v != "ignore" && v != "delete" && v != "kick" {
		return antigccallDefaultAction
	}
	return v
}

// ── info text builders ─────────────────────────────────────────────────────

// gccallFullInfo is the full .antigccall command info (antilink style).
func gccallFullInfo(prefix string, enabled bool, curAction string) string {
	return "*\U0001F530 ANTIGCCALL COMMAND INFO \U0001F530*\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ON \u2771*\n" +
		"*WHEN ANTIGCCALL IS ON THEN IF ANY MEMBER STARTS A GROUP CALL IN THIS GROUP THE BOT WILL DETECT THE GROUP CALL AND ACTION WILL APPLY*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL OFF \u2771*\n" +
		"*WHEN ANTIGCCALL IS OFF THEN ALL MEMBERS CAN FREELY START GROUP CALLS IN THIS GROUP*\n\n" +
		"*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION \u2771*\n" +
		"*YOU WILL GET INFO OF ANTIGCCALL ACTIONS*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION DECLINE \u2771*\n" +
		"*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION IGNORE \u2771*\n" +
		"*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION DELETE \u2771*\n" +
		"*WHEN ACTION IS DELETE THEN GROUP CALLS WILL BE DETECTED AND CLOSED AND A NOTICE WILL BE SENT IN THE GROUP*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION KICK \u2771*\n" +
		"*WHEN ACTION IS KICK THEN AS SOON AS GROUP CALL IS DETECTED THE CALLER WILL BE REMOVED AND CALL WILL BE CLOSED*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL RESET \u2771*\n" +
		"*TO RESET FULL ANTIGCCALL COMMAND*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL DEBUG \u2771*\n" +
		"*SHOWS LAST GROUP CALL EVENTS WITH FULL JSON DEBUG — HOW CALLS ARRIVE AND WHICH ACTION APPLIES*\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL DEBUG CLEAR \u2771*\n" +
		"*TO CLEAR THE DEBUG LOG*\n\n\n" +
		"*ANTIGCCALL NOW :\u2771 \u2770 " + boolOnOff(enabled) + " \u2771*\n" +
		"*ACTION :\u2771 \u2770 " + strings.ToUpper(curAction) + " \u2771*"
}

// gccallActionsInfo is the ".antigccall action" info (antilink actions style).
func gccallActionsInfo(prefix string, curAction string) string {
	return "*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION DECLINE \u2771*\n" +
		"*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION IGNORE \u2771*\n" +
		"*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION DELETE \u2771*\n" +
		"*WHEN ACTION IS DELETE THEN GROUP CALLS WILL BE DETECTED AND CLOSED AND A NOTICE WILL BE SENT IN THE GROUP*\n\n\n" +
		"*TYPE \u2770 " + prefix + "ANTIGCCALL ACTION KICK \u2771*\n" +
		"*WHEN ACTION IS KICK THEN AS SOON AS GROUP CALL IS DETECTED THE CALLER WILL BE REMOVED AND CALL WILL BE CLOSED*\n\n\n" +
		"*CURRENT ACTION :\u2771 \u2770 " + strings.ToUpper(curAction) + " \u2771*"
}

// ── command handler ────────────────────────────────────────────────────────

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
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTIVATED \U0001F530*\n\n*ACTION :\u2771 \u2770 "+strings.ToUpper(curAction)+" \u2771*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo(curAction))
		return
	}

	// OFF
	if sub == "off" {
		antiSetEnabled(s, groupJID, antigccallFeature, false)
		s.Reply(info, "*\U0001F530 ANTIGCCALL DE-ACTIVATED \U0001F530*")
		return
	}

	// ACTION (decline / ignore / delete / kick tree)
	if sub == "action" {
		handleAntiGcCallAction(s, info, args[1:], prefix)
		return
	}

	// DEBUG — last JSON debug lines (how calls arrive + which action applied)
	if sub == "debug" {
		handleAntiGcCallDebug(s, info, args[1:])
		return
	}

	// RESET — full reset (off + action back to decline)
	if sub == "reset" {
		antiSetEnabled(s, groupJID, antigccallFeature, false)
		s.SetGroupSetting(groupJID, antigccallFeature+":action", antigccallDefaultAction)
		s.Reply(info, "*\U0001F530 ANTIGCCALL RESET DONE \U0001F530*\n\n*ANTIGCCALL NOW :\u2771 \u2770 OFF \u2771*\n*ACTION :\u2771 \u2770 DECLINE \u2771*")
		return
	}

	// Unknown subcommand → full command info again
	curAction := AntigccallAction(s, groupJID)
	enabled := AntigccallIsOn(s, groupJID)
	s.Reply(info, gccallFullInfo(prefix, enabled, curAction))
}

// handleAntiGcCallAction — the "action" sub-command tree
// (decline / ignore / delete / kick). NO admin check — direct set + apply.
func handleAntiGcCallAction(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupJID := info.Chat.String()

	// ".antigccall action" with no arg → action info
	if len(args) == 0 {
		curAction := AntigccallAction(s, groupJID)
		s.Reply(info, gccallActionsInfo(prefix, curAction))
		return
	}

	actionArg := strings.ToLower(strings.TrimSpace(args[0]))

	// DECLINE — silently close group calls
	if actionArg == "decline" {
		antiSetAction(s, groupJID, antigccallFeature, "decline")
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :\u2771 \u2770 DECLINE \u2771*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("decline"))
		return
	}

	// IGNORE — silently ignore, no notification at all
	if actionArg == "ignore" {
		antiSetAction(s, groupJID, antigccallFeature, "ignore")
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :\u2771 \u2770 IGNORE \u2771*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("ignore"))
		return
	}

	// DELETE — close group call + notice in group (antilink delete style)
	if actionArg == "delete" {
		antiSetAction(s, groupJID, antigccallFeature, "delete")
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :\u2771 \u2770 DELETE \u2771*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("delete"))
		return
	}

	// KICK — close group call + remove caller (antilink kick style)
	if actionArg == "kick" {
		antiSetAction(s, groupJID, antigccallFeature, "kick")
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTION SET TO :\u2771 \u2770 KICK \u2771*\n\n*\U0001F530 ACTION INFO \U0001F530*\n"+gccallActionInfo("kick"))
		return
	}

	// Unknown action arg → action info again
	curAction := AntigccallAction(s, groupJID)
	s.Reply(info, gccallActionsInfo(prefix, curAction))
}

// handleAntiGcCallDebug — ".antigccall debug [n | clear]" — shows the last
// JSON debug lines from nexstore/gccall_debug.jsonl (written by the
// enforcement handler on every group-call event).
func handleAntiGcCallDebug(s SessionBridge, info types.MessageInfo, args []string) {
	if len(args) > 0 && strings.ToLower(strings.TrimSpace(args[0])) == "clear" {
		s.GCCallDebugClear()
		s.Reply(info, "*\U0001F530 ANTIGCCALL DEBUG LOG CLEARED \U0001F530*")
		return
	}

	n := 15
	if len(args) > 0 {
		if v, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && v > 0 && v <= 100 {
			n = v
		}
	}

	lines := s.GCCallDebugTail(n)
	if len(lines) == 0 {
		s.Reply(info, "*\U0001F530 ANTIGCCALL DEBUG EMPTY \U0001F530*\n\n*NO GROUP CALL EVENTS SEEN YET — WAIT FOR A GROUP CALL THEN CHECK AGAIN*")
		return
	}

	s.Reply(info, "*\U0001F530 ANTIGCCALL DEBUG \u2014 LAST "+strconv.Itoa(len(lines))+" EVENTS \U0001F530*\n\n```\n"+strings.Join(lines, "\n")+"\n```")
}

// gccallActionInfo returns the ACTION INFO line shown when the feature is
// turned ON or the action is set (same pattern as antiActionInfo).
func gccallActionInfo(action string) string {
	switch action {
	case "ignore":
		return "*ACTION IS IGNORE — NOW BOT WILL SILENTLY IGNORE ALL GROUP CALLS NO ACTION NO NOTIFICATION AT ALL*"
	case "delete":
		return "*ACTION IS DELETE — NOW BOT WILL CLOSE ALL GROUP CALLS AND SEND A NOTICE IN THE GROUP*"
	case "kick":
		return "*ACTION IS KICK — NOW BOT WILL REMOVE THE CALLER AS SOON AS GROUP CALL IS DETECTED AND CALL WILL BE CLOSED*"
	default:
		return "*ACTION IS DECLINE — NOW BOT WILL SILENTLY CLOSE ALL GROUP CALLS NO NOTIFICATION WILL BE SENT*"
	}
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antigccall", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO CONTROL GROUP CALLS IN THE GROUP. IT CAN DECLINE, IGNORE, DELETE OR KICK ON GROUP CALLS STARTED BY MEMBERS.", OwnerOnly: true, Run: handleAntiGcCall})
	// hidden aliases (user spellings: antigcall / antgccall)
	Register(Command{Name: "antigcall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
	Register(Command{Name: "antgccall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
}
