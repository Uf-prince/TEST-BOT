package goldcmds

// ============================================================================
// GOLD-MD — Welcome & Goodbye commands: .welcome and .goodbye
//
// Ported from UMAR-MD (Node.js pair.js) — same text style, same behaviour:
//   .welcome on              → bot welcomes new joiners with DP + @mention
//   .welcome off             → stop welcoming
//   .welcome msg {text}      → set custom welcome text (@user, @gname)
//   .welcome reset           → fully reset (message + status deleted)
//
//   .goodbye on              → bot says goodbye to leavers with DP + @mention
//   .goodbye off             → stop goodbye
//   .goodbye msg {text}      → set custom goodbye text (@user, @gname)
//   .goodbye reset           → fully reset
//
// Per-GROUP config stored in Redis (Upstash) via settings:<groupJID> hash:
//   field "welcome"          = "true"/"false"
//   field "welcomemsg"       = custom message text ("" = default)
//   field "goodbye"          = "true"/"false"
//   field "goodbyemsg"       = custom message text ("" = default)
//
// The actual welcome/goodbye on group join/leave is applied in manager.go's
// EventHandler → *events.GroupInfo case (ApplyWelcome / ApplyGoodbye),
// exactly like pair.js lines 9345-9440 (group-participants.update).
//
// Owner-only AND group-only. The trigger sends the group DP image with the
// caption + @mention, or text-only if the group has no DP.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── defaults (same as Node.js pair.js lines 6125-6161) ─────────────────────

const defaultWelcomeMsg = `*MOST WELCOME TO OUR GROUP* 🔰  
DEAR @user

*TO OUR FAMILY* 🔰  
@gname

*WE ARE SO SO GLAD YOU'RE HERE* 🔰🔰  
*YOU ARE NOW A PART OF OUR FAMILY* 🔰  
*YOUR PRESENCE MAKES THIS GROUP EVEN MORE SPECIAL.*  
*WE ARE REALLY HAPPY FROM THE BOTTOM OF OUR HEART THAT YOU JOINED US* 🔰🔰

*TAKE CARE OF YOURSELF DEAR* 🔰🔰  
*STAY HAPPY AND STAY ACTIVE* 🔰🔰

*IF YOU HAVE ANY PROBLEM OR ANY QUESTION, FEEL FREE TO CONTACT ADMINS ANYTIME* 🔰🔰  
*WE ARE ALWAYS HERE TO HELP YOU* 🔰

*KINDLY READ OUR GROUP RULES CAREFULLY AND FOLLOW THEM* 🔰🔰  
*LET'S KEEP THIS GROUP POSITIVE, LOVING AND RESPECTFUL FOR EVERYONE* 🔰🔰`

const defaultGoodbyeMsg = `*GOOD BYE DEAR 🔰*
@user

*TO OUR GROUP 🔰*
@gname



*SAYING GOODBYE IS NEVER EASY, ESPECIALLY WHEN IT'S SOMEONE WHO MADE THIS GROUP FEEL LIKE HOME. 🔰🔰*

*THANK YOU* FOR ALL THE *LAUGHS* 🔰, *EVERYTHING YOU SAID MEANT A LOT TO US* 🔰, THE *SUPPORT* 🔰, AND ALL THE *LITTLE MOMENTS* THAT MADE THIS GROUP SPECIAL. YOU BROUGHT YOUR OWN *LIGHT* 🔰 HERE, AND WE'RE REALLY GOING TO FEEL THAT ABSENCE.  

YOU'LL BE *MISSED MORE THAN WORDS CAN SAY* 🔰. I TRULY HOPE LIFE TAKES YOU TO *AMAZING PLACES* 🔰 AND GIVES YOU EVERYTHING YOU'RE WISHING FOR. 🔰

THIS GROUP WILL *ALWAYS HAVE A SPACE FOR YOU* 🔰. OUR DOORS ARE OPEN ANYTIME YOU WANT TO COME BACK AND SAY *HI* 🔰

*GOODBYE FOR NOW, NOT FOREVER.*  
*TAKE CARE OF YOURSELF AND STAY HAPPY* 🔰🔰`

// ── helpers ────────────────────────────────────────────────────────────────

// welcomeIsOn reads a welcome/goodbye boolean setting from Redis (per-group).
func welcomeIsOn(s SessionBridge, groupJID types.JID, field string) bool {
	v := s.GetGroupSetting(groupJID.String(), field, "false")
	return v == "true" || v == "1" || v == "on"
}

// welcomeSetOn writes a welcome/goodbye boolean setting to Redis (per-group).
func welcomeSetOn(s SessionBridge, groupJID types.JID, field string, on bool) {
	if on {
		s.SetGroupSetting(groupJID.String(), field, "true")
	} else {
		s.SetGroupSetting(groupJID.String(), field, "false")
	}
}

// welcomeMessage returns the custom message if set, otherwise the default.
func welcomeMessage(s SessionBridge, groupJID types.JID, msgField, defMsg string) string {
	v := s.GetGroupSetting(groupJID.String(), msgField, "")
	if strings.TrimSpace(v) == "" {
		return defMsg
	}
	return v
}

// hasCustomMessage returns true if a custom message is set (not default).
func hasCustomMessage(s SessionBridge, groupJID types.JID, msgField string) bool {
	v := s.GetGroupSetting(groupJID.String(), msgField, "")
	return strings.TrimSpace(v) != ""
}

// stripBraces removes a surrounding {…} wrapper from a message argument, same
// as Node.js: raw.match(/^\{([\s\S]*)\}$/). Allows .welcome msg {text here}.
var braceRe = regexp.MustCompile(`^\{([\s\S]*)\}$`)

func stripBraces(raw string) string {
	m := braceRe.FindStringSubmatch(raw)
	if m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(raw)
}

// ── .welcome ───────────────────────────────────────────────────────────────
// Aliases (Node.js): welcome, wc, welcum, wlcm, welcm

func handleWelcome(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleWelcomeAsync(s, info, args, prefix)
}

func handleWelcomeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Group-only check (same text as Node.js)
	if !info.IsGroup {
		s.Reply(info, "*WELCOME ONLY WORKS IN GROUPS*")
		return
	}
	// Owner-only check (same text as Node.js — "ONLY FOR OWNER" not "ONLY FOR ME")
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR OWNER 😎*")
		return
	}

	groupJID := info.Chat
	enabled := welcomeIsOn(s, groupJID, "welcome")
	custom := hasCustomMessage(s, groupJID, "welcomemsg")

	// No args → show info (same text as Node.js pair.js lines 15005-15025)
	if len(args) == 0 {
		statusNow := "OFF"
		if enabled {
			statusNow = "ON"
		}
		customMsgLabel := "DEFAULT"
		if custom {
			customMsgLabel = "YES"
		}
		s.Reply(info, "*🔰 WELCOME COMMAND INFO 🔰*\n\n*TYPE ❰ "+prefix+"WELCOME ON ❱*\n*WHEN WELCOME IS ON THEN AS SOON AS ANY NEW USER JOINS THE GROUP THE BOT WILL SEND A WELCOME MESSAGE WITH THEIR WHATSAPP PROFILE PICTURE AND MENTION*\n\n\n*TYPE ❰ "+prefix+"WELCOME OFF ❱*\n*WHEN WELCOME IS OFF THEN NO WELCOME MESSAGE WILL BE SENT TO NEW MEMBERS IN THIS GROUP*\n\n\n*🔰 WELCOME MESSAGE INFO 🔰*\n\n*TYPE ❰ "+prefix+"WELCOME MSG {Hey @user welcome to @gname family 🔰} ❱*\n*SET YOUR OWN CUSTOM WELCOME TEXT*\n*USE @user FOR NEW MEMBER MENTION*\n*USE @gname FOR GROUP NAME*\n\n\n*TYPE ❰ "+prefix+"WELCOME MSG {Assalam o Alaikum @user join hone ka shukriya 🔰} ❱*\n*ANOTHER EXAMPLE — JAISI MARZI TEXT LIKHO CURLY BRACES KE ANDAR*\n\n\n*TYPE ❰ "+prefix+"WELCOME RESET ❱*\n*TO FULLY RESET WELCOME — MESSAGE AND STATUS BOTH GET DELETED FROM DATABASE, DEFAULT WELCOME TEXT DOBARA CHALU HO JATA HAI*\n\n\n*WELCOME NOW :❱ ❰ "+statusNow+" ❱*\n*CUSTOM MSG :❱ ❰ "+customMsgLabel+" ❱*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		welcomeSetOn(s, groupJID, "welcome", true)
		s.Reply(info, "*🔰 WELCOME ACTIVATED 🔰*\n\n*NOW BOT WILL WELCOME EVERY NEW MEMBER WITH THEIR PROFILE PICTURE AND MENTION 😎*")
		return
	}

	if subCmd == "off" {
		welcomeSetOn(s, groupJID, "welcome", false)
		s.Reply(info, "*🔰 WELCOME DE-ACTIVATED 🔰*\n\n*NOW BOT DOESN'T SEND WELCOME MESSAGES ANYMORE*")
		return
	}

	// welcome msg {text} — set custom message (msg/message/set/setmsg all work)
	if subCmd == "msg" || subCmd == "message" || subCmd == "set" || subCmd == "setmsg" {
		// Reconstruct the raw text after the sub-command word, then strip braces.
		// args[0] is the sub-command; args[1:] is everything after it.
		rawAfterSub := ""
		if len(args) > 1 {
			rawAfterSub = strings.Join(args[1:], " ")
		}
		rawAfterSub = strings.TrimSpace(rawAfterSub)
		raw := stripBraces(rawAfterSub)
		if raw == "" {
			s.Reply(info, "*🔰 WELCOME MSG 🔰*\n\n*TYPE ❰ "+prefix+"WELCOME MSG {Hey @user welcome to @gname 🔰} ❱*\n\n*USE @user FOR MEMBER MENTION*\n*USE @gname FOR GROUP NAME*")
			return
		}
		s.SetGroupSetting(groupJID.String(), "welcomemsg", raw)
		s.Reply(info, "*🔰 WELCOME MESSAGE SAVED*\n\n*PREVIEW :❱*\n"+raw)
		return
	}

	if subCmd == "reset" {
		// Fully reset: delete message + set status OFF (same as Node.js UmarResetWelcome)
		s.SetGroupSetting(groupJID.String(), "welcomemsg", "")
		welcomeSetOn(s, groupJID, "welcome", false)
		s.Reply(info, "*🔰 WELCOME FULLY RESET*\n\n*STATUS :❱ OFF*\n*MESSAGE :❱ DEFAULT*\n*DATABASE ENTRY :❱ SAFE DELETED*")
		return
	}

	// Unknown subcommand
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ WELCOME ❱ FOR HELP*")
}

// ── .goodbye ───────────────────────────────────────────────────────────────
// Aliases (Node.js): goodbye, gb, bye, gdby, goodby

func handleGoodbye(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGoodbyeAsync(s, info, args, prefix)
}

func handleGoodbyeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Group-only check (same text as Node.js)
	if !info.IsGroup {
		s.Reply(info, "*GOODBYE ONLY WORKS IN GROUPS*")
		return
	}
	// Owner-only check (same text as Node.js — "ONLY FOR OWNER" not "ONLY FOR ME")
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR OWNER 😎*")
		return
	}

	groupJID := info.Chat
	enabled := welcomeIsOn(s, groupJID, "goodbye")
	custom := hasCustomMessage(s, groupJID, "goodbyemsg")

	// No args → show info (same text as Node.js pair.js lines 15148-15168)
	if len(args) == 0 {
		statusNow := "OFF"
		if enabled {
			statusNow = "ON"
		}
		customMsgLabel := "DEFAULT"
		if custom {
			customMsgLabel = "YES"
		}
		s.Reply(info, "*🔰 GOODBYE COMMAND INFO 🔰*\n\n*TYPE ❰ "+prefix+"GOODBYE ON ❱*\n*WHEN GOODBYE IS ON THEN AS SOON AS ANY MEMBER LEAVES OR IS REMOVED FROM THE GROUP THE BOT WILL SEND A GOODBYE MESSAGE WITH THEIR WHATSAPP PROFILE PICTURE AND MENTION*\n\n\n*TYPE ❰ "+prefix+"GOODBYE OFF ❱*\n*WHEN GOODBYE IS OFF THEN NO GOODBYE MESSAGE WILL BE SENT WHEN MEMBERS LEAVE THIS GROUP*\n\n\n*🔰 GOODBYE MESSAGE INFO 🔰*\n\n*TYPE ❰ "+prefix+"GOODBYE MSG {Bye @user we will miss you from @gname 🔰} ❱*\n*SET YOUR OWN CUSTOM GOODBYE TEXT*\n*USE @user FOR LEAVING MEMBER MENTION*\n*USE @gname FOR GROUP NAME*\n\n\n*TYPE ❰ "+prefix+"GOODBYE MSG {Allah hafiz @user tumhari kami mehsoos hogi 🔰} ❱*\n*ANOTHER EXAMPLE — JAISI MARZI TEXT LIKHO CURLY BRACES KE ANDAR*\n\n\n*TYPE ❰ "+prefix+"GOODBYE RESET ❱*\n*TO FULLY RESET GOODBYE — MESSAGE AND STATUS BOTH GET DELETED FROM DATABASE, DEFAULT GOODBYE TEXT DOBARA CHALU HO JATA HAI*\n\n\n*GOODBYE NOW :❱ ❰ "+statusNow+" ❱*\n*CUSTOM MSG :❱ ❰ "+customMsgLabel+" ❱*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		welcomeSetOn(s, groupJID, "goodbye", true)
		s.Reply(info, "*🔰 GOODBYE ACTIVATED 🔰*\n\n*NOW BOT WILL SAY GOODBYE TO EVERY LEAVING MEMBER WITH THEIR PROFILE PICTURE AND MENTION 🔰*")
		return
	}

	if subCmd == "off" {
		welcomeSetOn(s, groupJID, "goodbye", false)
		s.Reply(info, "*🔰 GOODBYE DE-ACTIVATED 🔰*\n\n*NOW BOT DOESN'T SEND GOODBYE MESSAGES ANYMORE*")
		return
	}

	// goodbye msg {text} — set custom message
	if subCmd == "msg" || subCmd == "message" || subCmd == "set" || subCmd == "setmsg" {
		rawAfterSub := ""
		if len(args) > 1 {
			rawAfterSub = strings.Join(args[1:], " ")
		}
		rawAfterSub = strings.TrimSpace(rawAfterSub)
		raw := stripBraces(rawAfterSub)
		if raw == "" {
			s.Reply(info, "*🔰 GOODBYE MSG 🔰*\n\n*TYPE ❰ "+prefix+"GOODBYE MSG {Bye @user from @gname 🔰} ❱*\n\n*USE @user FOR MEMBER MENTION*\n*USE @gname FOR GROUP NAME*")
			return
		}
		s.SetGroupSetting(groupJID.String(), "goodbyemsg", raw)
		s.Reply(info, "*🔰 GOODBYE MESSAGE SAVED*\n\n*PREVIEW :❱*\n"+raw)
		return
	}

	if subCmd == "reset" {
		s.SetGroupSetting(groupJID.String(), "goodbyemsg", "")
		welcomeSetOn(s, groupJID, "goodbye", false)
		s.Reply(info, "*🔰 GOODBYE FULLY RESET*\n\n*STATUS :❱ OFF*\n*MESSAGE :❱ DEFAULT*\n*DATABASE ENTRY :❱ SAFE DELETED*")
		return
	}

	// Unknown subcommand
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ GOODBYE ❱ FOR HELP*")
}

// ── exported helpers for manager.go welcome/goodbye engine ──────────────────

// WelcomeIsOn returns true if welcome is enabled for the given group.
func WelcomeIsOn(s SessionBridge, groupJID types.JID) bool {
	return welcomeIsOn(s, groupJID, "welcome")
}

// GoodbyeIsOn returns true if goodbye is enabled for the given group.
func GoodbyeIsOn(s SessionBridge, groupJID types.JID) bool {
	return welcomeIsOn(s, groupJID, "goodbye")
}

// WelcomeMessage returns the welcome message (custom or default) for a group.
func WelcomeMessage(s SessionBridge, groupJID types.JID) string {
	return welcomeMessage(s, groupJID, "welcomemsg", defaultWelcomeMsg)
}

// GoodbyeMessage returns the goodbye message (custom or default) for a group.
func GoodbyeMessage(s SessionBridge, groupJID types.JID) string {
	return welcomeMessage(s, groupJID, "goodbyemsg", defaultGoodbyeMsg)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "welcome", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET AND CONTROL WELCOME MESSAGES FOR NEW GROUP MEMBERS.", OwnerOnly: true, Run: handleWelcome})
	Register(Command{Name: "goodbye", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET AND CONTROL GOODBYE MESSAGES FOR MEMBERS WHO LEAVE THE GROUP.", OwnerOnly: true, Run: handleGoodbye})

	// Hidden aliases (same as Node.js)
	Register(Command{Name: "wc", OwnerOnly: true, Hidden: true, Run: handleWelcome})
	Register(Command{Name: "welcum", OwnerOnly: true, Hidden: true, Run: handleWelcome})
	Register(Command{Name: "wlcm", OwnerOnly: true, Hidden: true, Run: handleWelcome})
	Register(Command{Name: "welcm", OwnerOnly: true, Hidden: true, Run: handleWelcome})
	Register(Command{Name: "gb", OwnerOnly: true, Hidden: true, Run: handleGoodbye})
	Register(Command{Name: "bye", OwnerOnly: true, Hidden: true, Run: handleGoodbye})
	Register(Command{Name: "gdby", OwnerOnly: true, Hidden: true, Run: handleGoodbye})
	Register(Command{Name: "goodby", OwnerOnly: true, Hidden: true, Run: handleGoodbye})
}
