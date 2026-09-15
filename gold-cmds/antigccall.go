package goldcmds

// ============================================================================
// GOLD-MD — .antigccall command (aliases: .antigcall, .antgccall)
//
// Group-call control — same-to-same .antilink style. Controls GROUP CALLS
// started by group members (events.CallOfferNotice in manager.go):
//   .antigccall                   → full command info
//   .antigccall on / off          → enable / disable group-call control
//   .antigccall action            → action info (decline / ignore)
//   .antigccall action decline    → SILENTLY close/decline group calls
//   .antigccall action ignore     → SILENTLY ignore, NO notification at all
//   .antigccall reset             → full reset (off + decline)
//
// Per-group config in Redis (settings:<groupJID>) — same pattern as antilink:
//   field "antigccall"         = on/off
//   field "antigccall:action"  = decline/ignore
//
// ENFORCEMENT (owner order — NO admin check, direct action):
//   - decline → whatsmeow Client.RejectCall(ctx, creator, callID) — the
//     group call is closed SILENTLY, no notification message is sent
//     anywhere (no group message, no inbox message, nothing).
//   - ignore  → completely silent pass — no reject, no notification.
//   - Owner bypass: the owner's own group calls are silently ignored
//     (no decline, no notification).
//   - Premium bypass (.antigccallprem add): premium members' group calls
//     are silently ignored too.
//
// NOTE: 🔰 emoji style, ❰ ❱ markers — same as antilink.
// ============================================================================

import (
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
// "decline" (default) or "ignore".
func AntigccallAction(s SessionBridge, groupJID string) string {
	v := strings.ToLower(s.GetGroupSetting(groupJID, antigccallFeature+":action", antigccallDefaultAction))
	if v != "decline" && v != "ignore" {
		return antigccallDefaultAction
	}
	return v
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

		s.Reply(info, "*\U0001F530 ANTIGCCALL COMMAND INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ON \u2771*\n*WHEN ANTIGCCALL IS ON THEN IF ANY MEMBER STARTS A GROUP CALL IN THIS GROUP THE BOT WILL DETECT THE GROUP CALL AND ACTION WILL APPLY SILENTLY*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL OFF \u2771*\n*WHEN ANTIGCCALL IS OFF THEN ALL MEMBERS CAN FREELY START GROUP CALLS IN THIS GROUP*\n\n*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION \u2771*\n*YOU WILL GET INFO OF ANTIGCCALL ACTIONS*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION DECLINE \u2771*\n*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION IGNORE \u2771*\n*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL RESET \u2771*\n*TO RESET FULL ANTIGCCALL COMMAND*\n\n\n*ANTIGCCALL NOW :\u2771 \u2770 "+boolOnOff(enabled)+" \u2771*\n*ACTION :\u2771 \u2770 "+strings.ToUpper(curAction)+" \u2771*")
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

	// ACTION (decline / ignore tree)
	if sub == "action" {
		handleAntiGcCallAction(s, info, args[1:], prefix)
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
	s.Reply(info, "*\U0001F530 ANTIGCCALL COMMAND INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ON \u2771*\n*WHEN ANTIGCCALL IS ON THEN IF ANY MEMBER STARTS A GROUP CALL IN THIS GROUP THE BOT WILL DETECT THE GROUP CALL AND ACTION WILL APPLY SILENTLY*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL OFF \u2771*\n*WHEN ANTIGCCALL IS OFF THEN ALL MEMBERS CAN FREELY START GROUP CALLS IN THIS GROUP*\n\n*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION \u2771*\n*YOU WILL GET INFO OF ANTIGCCALL ACTIONS*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION DECLINE \u2771*\n*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION IGNORE \u2771*\n*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL RESET \u2771*\n*TO RESET FULL ANTIGCCALL COMMAND*\n\n\n*ANTIGCCALL NOW :\u2771 \u2770 "+boolOnOff(enabled)+" \u2771*\n*ACTION :\u2771 \u2770 "+strings.ToUpper(curAction)+" \u2771*")
}

// handleAntiGcCallAction — the "action" sub-command tree (decline / ignore).
// NO admin check — direct set + apply (owner order).
func handleAntiGcCallAction(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupJID := info.Chat.String()

	// ".antigccall action" with no arg → action info
	if len(args) == 0 {
		curAction := AntigccallAction(s, groupJID)
		s.Reply(info, "*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION DECLINE \u2771*\n*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION IGNORE \u2771*\n*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n*CURRENT ACTION :\u2771 \u2770 "+strings.ToUpper(curAction)+" \u2771*")
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

	// Unknown action arg → action info again
	curAction := AntigccallAction(s, groupJID)
	s.Reply(info, "*\U0001F530 ANTIGCCALL ACTIONS INFO \U0001F530*\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION DECLINE \u2771*\n*WHEN ACTION IS DECLINE THEN GROUP CALLS WILL BE DETECTED AND SILENTLY CLOSED NO NOTIFICATION WILL BE SENT*\n\n\n*TYPE \u2770 "+prefix+"ANTIGCCALL ACTION IGNORE \u2771*\n*WHEN ACTION IS IGNORE THEN GROUP CALLS WILL BE SILENTLY IGNORED NO ACTION NO NOTIFICATION AT ALL*\n\n\n*CURRENT ACTION :\u2771 \u2770 "+strings.ToUpper(curAction)+" \u2771*")
}

// gccallActionInfo returns the ACTION INFO line shown when the feature is
// turned ON or the action is set (same pattern as antiActionInfo).
func gccallActionInfo(action string) string {
	if action == "ignore" {
		return "*ACTION IS IGNORE — NOW BOT WILL SILENTLY IGNORE ALL GROUP CALLS NO ACTION NO NOTIFICATION AT ALL*"
	}
	return "*ACTION IS DECLINE — NOW BOT WILL SILENTLY CLOSE ALL GROUP CALLS NO NOTIFICATION WILL BE SENT*"
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antigccall", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO CONTROL GROUP CALLS IN THE GROUP. IT CAN SILENTLY DECLINE OR SILENTLY IGNORE GROUP CALLS STARTED BY MEMBERS.", OwnerOnly: true, Run: handleAntiGcCall})
	// hidden aliases (user spellings: antigcall / antgccall)
	Register(Command{Name: "antigcall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
	Register(Command{Name: "antgccall", OwnerOnly: true, Hidden: true, Run: handleAntiGcCall})
}
