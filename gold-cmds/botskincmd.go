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
	Register(Command{
		Name:      "botstyle",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE WHOLE BOT TEXT LOOK - FONTS, MARKS AND EMOJIS. PICK ONE OF 50 STYLES.",
		OwnerOnly: true,
		Hidden:    true,
		Run:       handleBotStyle,
	})
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
	// ONE command = whole-bot look: the text skin AND the menu chrome both
	// follow .botstyle, so a single SET restyles everything (per-menu
	// .<x>style can still override its own chrome afterwards).
	s.SetBotSkinSetting(fmt.Sprint(n))
	s.SetBotMenuStyleSetting(fmt.Sprint(n))
	st := MenuStyleAt(n)
	s.Reply(info, fmt.Sprintf("*%s BOT STYLE SET %d %s*\n\n*POORE BOT KA TEXT STYLE CHANGE HO GYA*\n\n%s",
		st.Sym, n, st.Sym, botStylePreview(st, prefix))+menuStyleInfoLine(prefix))
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
