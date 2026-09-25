package goldcmds

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// GOLD-MD - .botstyle (BOT-WIDE TEXT SKIN)
// ----------------------------------------------------------------------------
// Ek hi command se POORE bot ka text design badal jata hai: har text style ke
// decorative font me, **bold** / __italic__ marks style ke symbol se, aur
// accent emojis usi symbol se. Commands wahi rehte hain (aur dispatch bhi
// wahi rehta hai - fancy token handler normalise kar leta hai).
//
//   .botstyle            -> guide + current
//   .botstyle set 7      -> poora bot style 7 ka look
//   .botstyle reset      -> classic (skin off)
//
// Redis field: "botstyle" (style number). Menu chrome ke liye alag command
// .botmenustyle hai - ye usse independent hai.
// ============================================================================

func init() {
	// OWNER ORDER: .botstyle ek category menu hai (FONT/GAME/EQUALIZER ki
	// tarah). Bare .botstyle ek boxed list kholta hai - .BOTSTYLE1 ..
	// .BOTSTYLE50 - aur har number apne aap poore bot ka look set karta hai.
	Register(Command{
		Name:      "botstyle",
		Category:  "BOT STYLE",
		Desc:      "THIS COMMAND IS USED TO SHOW THE LIST OF 50 BOT TEXT STYLES. TYPE .BOTSTYLE1 TO .BOTSTYLE50 TO CHANGE THE WHOLE BOT INTO THAT STYLE.",
		OwnerOnly: true,
		Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
			if !s.IsOwner(info) {
				s.Reply(info, "*THIS COMMAND IS ONLY FOR ME \U0001F60E*")
				return
			}
			if strings.TrimSpace(strings.Join(args, " ")) == "" {
				s.ShowBotStyleMenu(info, args, prefix)
				return
			}
			handleBotStyle(s, info, args, prefix)
		},
	})
}

// BotStyleRunN is the entry point for the .botstyle1..botstyle50 commands.
// Each number applies its style to the WHOLE bot (text skin + menu chrome),
// exactly like ".botstyle set N" - the menu is just a friendlier front door.
func BotStyleRunN(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME \U0001F60E*")
		return
	}
	if n < 1 || n > MenuStyleCount {
		s.Reply(info, fmt.Sprintf("*STYLE NUMBER 1 SE %d TAK HI HAI*", MenuStyleCount))
		return
	}
	applyBotStyle(s, info, prefix, n)
}

func handleBotStyle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.EqualFold(argRaw, "reset") {
		s.SetBotSkinSetting("")
		s.SetBotMenuStyleSetting("")
		s.Reply(info, "*🔰 BOT STYLE RESET 🔰*\n\n*POORE BOT KA TEXT CLASSIC LOOK PE WAPAS*"+menuStyleInfoLine(prefix))
		return
	}
	if argRaw == "" {
		guide := botStyleGuide(prefix)
		guide += "\n\n*CURRENT:❯ ❮ " + currentStyleLabel(s, s.GetBotSkinSetting("")) + " ❱*"
		s.Reply(info, guide)
		return
	}
	n, ok := ParseMenuStyleArg(argRaw)
	if !ok {
		s.Reply(info, "*🔰 STYLE NUMBER GALAT 🔰*\n\n*1 SE "+fmt.Sprint(MenuStyleCount)+" KE DARMIYAN KOI NUMBER DO*\n*JAISE:* *❰ "+prefix+"BOTSTYLE SET 7 ❱*"+menuStyleInfoLine(prefix))
		return
	}
	applyBotStyle(s, info, prefix, n)
}

// applyBotStyle sets BOTH the text skin and the menu chrome to style n and
// confirms with a live preview. Shared by ".botstyle set N" and the
// .botstyleN menu commands so both behave identically.
func applyBotStyle(s SessionBridge, info types.MessageInfo, prefix string, n int) {
	s.SetBotSkinSetting(fmt.Sprint(n))
	s.SetBotMenuStyleSetting(fmt.Sprint(n))
	st := MenuStyleAt(n)
	s.Reply(info, fmt.Sprintf("*%s BOT STYLE SET %d %s*\n\n*BOT STYLE HAS BEEN CHANGED*\n\n*TYPE %sPING , %sMENU , %sALIVE TO TEST*\n\n*THE GOLD-MD NEW STYLE*",
		st.Sym, n, st.Sym, prefix, prefix, prefix))
}

// botStylePreview renders a small live sample of the skin so the owner sees the
// font / marks / emoji treatment before every reply changes.
func botStylePreview(st MenuStyle, prefix string) string {
	sample := "*🔰 GOLD-MD WHATSAPP BOT 🔰*\n*✅ CONNECTED SUCCESSFULLY*\n*❰ " + prefix + "MENU ❱*  _ALL COMMANDS_"
	return st.SkinText(sample)
}

// botStyleGuide lists every style number with a live, skinned name row.
func botStyleGuide(prefix string) string {
	var b strings.Builder
	b.WriteString("*❰ " + prefix + "BOTSTYLE SET <1-" + fmt.Sprint(MenuStyleCount) + "> ❱* → APPLY TO WHOLE BOT\n")
	b.WriteString("*❰ " + prefix + "BOTSTYLE RESET ❱* → BACK TO CLASSIC\n\n")
	b.WriteString("*🔰 ALL " + fmt.Sprint(MenuStyleCount) + " STYLES 🔰*\n")
	for n := 1; n <= MenuStyleCount; n++ {
		st := MenuStyleAt(n)
		b.WriteString(fmt.Sprintf("*%s %d • %s*\n", st.Sym, n, st.Styled(st.Name)))
	}
	b.WriteString("\n*NOTE: COMMANDS WAHI RAHENGE — SIRF DESIGN BADLEGA*")
	return b.String()
}
