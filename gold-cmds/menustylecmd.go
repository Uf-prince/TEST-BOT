package goldcmds

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// GOLD-MD — MENU STYLE COMMANDS
// File: menustylecmd.go
// ----------------------------------------------------------------------------
// One hidden per-menu command (.<x>style) plus the bot-wide .botmenustyle.
// Both share the same setter flow: guide / SET <n> / RESET.
// ============================================================================

func init() {
	// One hidden style command per menu (.menustyle / .logostyle / .aimenustyle
	// ...). Registered hidden so nothing new appears in the menus.
	for _, mc := range menuStyleCommands() {
		mc := mc
		Register(Command{
			Name:      menuStyleCommandName(mc),
			Category:  "OWNER & SYSTEM",
			Desc:      "THIS COMMAND IS USED TO CHANGE ONE MENU'S LOOK. PICK ONE OF 50 FANCY STYLES.",
			OwnerOnly: true,
			Hidden:    true,
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				handleMenuStyle(s, info, args, prefix, mc.Key, mc.MenuCmd)
			},
		})
	}

	// Bot-wide style: apply the same style to every menu in one shot.
	Register(Command{
		Name:      "botmenustyle",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE LOOK OF EVERY MENU AT ONCE. PICK ONE OF 50 FANCY STYLES.",
		OwnerOnly: true,
		Hidden:    true,
		Run:       handleBotMenuStyle,
	})
}

// handleMenuStyle sets ONE menu's style. Stored as "menustyle:<key>" so it can
// be cleared without touching the bot-wide style.
func handleMenuStyle(s SessionBridge, info types.MessageInfo, args []string, prefix, key, menuCmd string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	label := menuMediaLabel(key)
	cmdName := ""
	for _, mc := range menuStyleCommands() {
		if mc.Key == key {
			cmdName = menuStyleCommandName(mc)
			break
		}
	}
	if cmdName == "" {
		return
	}
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.EqualFold(argRaw, "reset") {
		s.SetMenuStyleSetting(key, "")
		s.Reply(info, fmt.Sprintf("*🔰 %s STYLE RESET 🔰*\n\n*THIS MENU IS BACK TO THE BOT-WIDE STYLE*", label)+menuStyleInfoLine(prefix))
		return
	}
	if argRaw == "" {
		cur := s.GetMenuStyleSetting(key, "")
		guide := menuStyleGuide(prefix, label, cmdName)
		guide += "\n\n*CURRENT:❯ ❮ " + currentStyleLabel(s, cur) + " ❱*"
		s.Reply(info, guide)
		return
	}
	n, ok := ParseMenuStyleArg(argRaw)
	if !ok {
		s.Reply(info, "*🔰 STYLE NUMBER GALAT 🔰*\n\n*1 SE "+fmt.Sprint(MenuStyleCount)+" KE DARMIYAN KOI NUMBER DO*\n*JAISE:* *❰ "+prefix+cmdName+" SET 7 ❱*"+menuStyleInfoLine(prefix))
		return
	}
	s.SetMenuStyleSetting(key, fmt.Sprint(n))
	s.Reply(info, fmt.Sprintf("*🔰 %s STYLE SET %d 🔰*\n\n%s", label, n,
		MenuStylePreview(n, label, prefix))+menuStyleInfoLine(prefix))
}

// handleBotMenuStyle sets the bot-wide style applied to every menu.
func handleBotMenuStyle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.EqualFold(argRaw, "reset") {
		s.SetBotMenuStyleSetting("")
		s.Reply(info, "*🔰 BOT MENU STYLE RESET 🔰*\n\n*EVERY MENU IS BACK TO THE CLASSIC STYLE*"+menuStyleInfoLine(prefix))
		return
	}
	if argRaw == "" {
		guide := menuStyleGuide(prefix, "BOT MENU", "botmenustyle")
		guide += "\n\n*CURRENT:❯ ❮ " + currentStyleLabel(s, s.GetBotMenuStyleSetting("")) + " ❱*"
		s.Reply(info, guide)
		return
	}
	n, ok := ParseMenuStyleArg(argRaw)
	if !ok {
		s.Reply(info, "*🔰 STYLE NUMBER GALAT 🔰*\n\n*1 SE "+fmt.Sprint(MenuStyleCount)+" KE DARMIYAN KOI NUMBER DO*\n*JAISE:* *❰ "+prefix+"BOTMENUSTYLE SET 7 ❱*"+menuStyleInfoLine(prefix))
		return
	}
	s.SetBotMenuStyleSetting(fmt.Sprint(n))
	s.Reply(info, fmt.Sprintf("*🔰 BOT MENU STYLE SET %d 🔰*\n\n*SAARE MENUS KA LOOK CHANGE HO GYA*\n\n%s", n,
		MenuStylePreview(n, "ALL MENUS", prefix))+menuStyleInfoLine(prefix))
}

// currentStyleLabel resolves the effective style for a stored value, for the
// guide's CURRENT line. An empty value means "inherit/default".
func currentStyleLabel(s SessionBridge, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "DEFAULT (1)"
	}
	n, ok := ParseMenuStyleArg(raw)
	if !ok {
		return "DEFAULT (1)"
	}
	return fmt.Sprintf("%d — %s", n, MenuStyleName(n))
}
