package goldcmds

// ============================================================================
// GOLD-MD — BOT NAME Command  (set / reset / show)
// File: botname.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.botname / .setbotname / .botnametoggle).
// Text style SAME TO SAME 0% farak — only the 👑 crown emoji is replaced with
// 🔰 everywhere (including the default bot name). Work is identical to the
// Node bot:
//
//   .botname              → show current bot name + change guide
//   .botname reset        → reset bot name to default "🔰 UMAR-MD 🔰"
//   .botname <new name>   → set a new bot name
//
// Persisted in Redis settings:<botJID> field "botname". Owner-only.
//
// Aliases (same as Node.js): botname, setbotname, botnametoggle
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// defaultBotName is the branded GOLD-MD footer label. When the Redis
// "botname" field is empty, botNameFooter() (handler.go) shows the full
// 3-line branded footer with the live prefix. When the owner sets a
// custom name via .botname, that name replaces the footer entirely.
const defaultBotName = "*GOLD-MD WHASAPP BOT*"

func handleBotName(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	aiArgs := strings.TrimSpace(strings.Join(args, " "))

	// current bot name (for the info display)
	currentBotName := s.GetBotNameSetting("")
	if currentBotName == "" ||
		currentBotName == DefaultBotNameMarker ||
		strings.Contains(currentBotName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(currentBotName, "GOLD_MD_DEFAULT") {
		currentBotName = "*GOLD-MD WHATSAPP BOT* (default)"
	}

	// no arg → show current bot name + change guide
	if aiArgs == "" {
		s.Reply(info, "*🔰 BOT NAME INFO 🔰*\n\n*CURRENT BOT NAME IS*\n*"+currentBotName+"*\n\n*TO CHANGE BOT NAME :❯*\n*TYPE ❮ "+prefix+"BOTNAME ❮ NEW NAME ❯ ❯*\n\n*EXAMPLE LIKE THIS*\n*❮ "+prefix+"BOTNAME UMAR-MD ❯*\n*❮ "+prefix+"BOTNAME UMAR XMD ❯*\n*❮ "+prefix+"BOTNAME KING BOT ❯*\n\n*TO RESET TO DEFAULT :❯*\n*TYPE ❮ "+prefix+"BOTNAME RESET ❯*")
		return
	}

	// reset → restore the branded GOLD-MD default footer
	if strings.ToLower(aiArgs) == "reset" {
		s.SetBotNameSetting("")
		s.DelBotNameSetting()
		s.Reply(info, "*🔰 BOT NAME RESET 🔰*\n\n*BOT NAME RESET TO DEFAULT*\n*GOLD-MD WHASAPP BOT*")
		return
	}

	// set new bot name
	s.SetBotNameSetting(aiArgs)
	s.Reply(info, "*🔰 BOT NAME CHANGED 🔰*\n\n*NEW BOT NAME IS*\n*"+aiArgs+"*")
}

func init() {
	Register(Command{
		Name:      "botname",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE BOT NAME. USE IT WITH THE NEW NAME.",
		OwnerOnly: true,
		Run:       handleBotName,
	})
	// hidden aliases (same as Node.js)
	Register(Command{Name: "setbotname", OwnerOnly: true, Hidden: true, Run: handleBotName})
	Register(Command{Name: "botnametoggle", OwnerOnly: true, Hidden: true, Run: handleBotName})
}
