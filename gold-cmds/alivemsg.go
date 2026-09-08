package goldcmds

// ============================================================================
// GOLD-MD — ALIVE MSG Command  (set / reset / show)
// File: alivemsg.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.alivemsg / .setalivemsg / .alivemessage).
// Text style SAME TO SAME 0% farak — only the 👑 crown emoji is replaced with
// 🔰 everywhere. Work is identical to the Node bot:
//
//   .alivemsg                 → show the change guide (no current-msg display
//                               in Node, just the how-to guide)
//   .alivemsg reset           → clear custom alive msg → default alive msg
//   .alivemsg <your new msg>  → set a new alive message
//
// The alive message supports the {PUSHNAME} placeholder (replaced with the
// sender's WhatsApp display name when .alive is run).
//
// Persisted in Redis settings:<botJID> field "alivemsg". Owner-only.
//
// Aliases (same as Node.js): alivemsg, setalivemsg, alivemessage
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handleAliveMsg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	// join all args (alive message can be multi-word)
	aiArgs := strings.TrimSpace(strings.Join(args, " "))

	// no arg → show the change guide (same text as Node.js)
	if aiArgs == "" {
		s.Reply(info, "*IF YOU WANT TO CHANGE ALIVE MSG THEN WRITE LIKE THIS* \n\n*TYPE ❮ "+prefix+"ALIVEMSG (YOUR NEW MSG) ❯* \n\n*EXAMPLE LIKE THIS* \n\n*❮ "+prefix+"ALIVEMSG I AM ACTIVE ❯* \n\n*WHEN YOU WRITE LIKE THIS THEN YOUR ALIVE MSG WILL BE CHANGED* \n\n*IF YOU WANT TO ADD THE OTHER PERSON NAME IN ALIVE MSG THEN LIKE THIS* \n\n*❮ "+prefix+"ALIVEMSG HI {PUSHNAME} I AM ACTIVE NOW ❯* \n\n*THIS WAY THE NEXT PERSON NAME WILL ALSO COME IN ALIVE MSG*\n\n*TO BACK ALIVE MSG OLD TYPE*\n*TYPE ❮ "+prefix+"ALIVEMSG RESET ❯*")
		return
	}

	// reset → clear custom alive msg (redis-safe delete → default)
	if strings.ToLower(aiArgs) == "reset" {
		s.SetAliveMsgSetting("")
		s.Reply(info, "*🔰 ALIVE MSG RESET TO DEFAULT ✅*\n\n*WAIT 1 MINUTE FOR CHANGES IN BOT*\n*AFTER WAITING 1 MINUTE TYPE ❮ ALIVE ❯ TO CHECK NEW ALIVE MESSAGE*\n")
		return
	}

	// set new alive msg
	s.SetAliveMsgSetting(aiArgs)
	s.Reply(info, "*🔰 ALIVE MSG CHANGED ✅*\n\n*NEW ALIVE MSG :❯*\n"+aiArgs+"\n\n*WAIT 1 MINUTE FOR CHANGES IN BOT*\n*AFTER WAITING 1 MINUTE TYPE ❮ ALIVE ❯ TO CHECK NEW ALIVE MESSAGE*\n")
}

func init() {
	Register(Command{
		Name:      "alivemsg",
		Category:  "OWNER & SYSTEM",
		Desc:      "Sets/changes the text shown by the .alive command (reset = default)",
		OwnerOnly: true,
		Run:       handleAliveMsg,
	})
	// hidden aliases (same as Node.js)
	Register(Command{Name: "setalivemsg", OwnerOnly: true, Hidden: true, Run: handleAliveMsg})
	Register(Command{Name: "alivemessage", OwnerOnly: true, Hidden: true, Run: handleAliveMsg})
}
