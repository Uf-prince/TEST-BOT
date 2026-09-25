package goldcmds

// ============================================================================
// GOLD-MD — .botlanguage  (BOT OUTPUT LANGUAGE)
// File: botlangcmd.go
// ============================================================================
// COMMANDS:
//   .botlanguage                 -> guide listing EVERY supported language
//                                   (names in ENGLISH, as the owner ordered)
//   .botlanguage set <code|name> -> bot replies in that language from now on
//   .botlanguage reset           -> back to ENGLISH
//
// OWNER ORDER: the bot's OUTPUT (replies, menus, alerts) is rendered in the
// chosen language. COMMAND NAMES NEVER CHANGE — .ping stays .ping, .menu stays
// .menu — so every user can always type the same command; only the bot's
// answers come back in their language.
//
// The guide is intentionally ENGLISH (owner order: "waha sb kuch English me
// banana, default; baki users apne mrzi ki language set kr le ge"). Each user
// then sets the language they want.
// ============================================================================

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// BotLanguageName is the canonical code for "no translation".
const BotLanguageName = "en"

func init() {
	Register(Command{
		Name:      "botlanguage",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE LANGUAGE IN WHICH THE BOT REPLIES. COMMAND NAMES STAY ENGLISH — ONLY THE BOT'S ANSWERS CHANGE LANGUAGE.",
		OwnerOnly: true,
		Run:       handleBotLanguage,
	})
	Register(Command{Name: "language", Category: "OWNER & SYSTEM", Desc: "", OwnerOnly: true, Hidden: true, Run: handleBotLanguage})
	Register(Command{Name: "botlang", Category: "OWNER & SYSTEM", Desc: "", OwnerOnly: true, Hidden: true, Run: handleBotLanguage})
}

// LanguageCatalog is the exported language list (code + ENGLISH display name),
// shared with the main package's translation layer.
func LanguageCatalog() []trtLang { return trtLangs }

// ResolveLanguage maps a user token (code or English name) to a language code.
func ResolveLanguage(tok string) (string, bool) {
	return trtResolveLang(tok)
}

// LanguageName returns the ENGLISH display name for a code.
func LanguageName(code string) string { return trtLangName(code) }

// TranslateText translates text into the target language code using the same
// free Google endpoint the .trt command uses.
func TranslateText(ctx context.Context, text, target string) (string, error) {
	out, _, err := trtTranslate(ctx, text, target)
	return out, err
}

func handleBotLanguage(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.EqualFold(argRaw, "reset") || strings.EqualFold(argRaw, "off") || strings.EqualFold(argRaw, "english") {
		s.SetBotLanguageSetting("")
		s.Reply(info, "*🔰 BOT LANGUAGE RESET 🔰*\n\n*THE BOT REPLIES IN ENGLISH AGAIN*\n*COMMAND NAMES NEVER CHANGE*")
		return
	}

	f := strings.Fields(argRaw)
	if len(f) > 0 && strings.EqualFold(f[0], "set") {
		f = f[1:]
	}
	if len(f) == 0 {
		s.Reply(info, botLanguageGuide(prefix, s.GetBotLanguageSetting("")))
		return
	}

	code, ok := ResolveLanguage(strings.Join(f, " "))
	if !ok {
		s.Reply(info, "*🔰 LANGUAGE NOT FOUND 🔰*\n\n*USE ONE OF THESE CODES OR NAMES:*\n*❮ "+prefix+"BOTLANGUAGE ❱* SHOWS THE FULL LIST")
		return
	}
	s.SetBotLanguageSetting(code)
	s.Reply(info, fmt.Sprintf("*🔰 BOT LANGUAGE SET 🔰*\n\n*THE BOT NOW REPLIES IN ❮ %s ❯*\n*COMMAND NAMES STAY ENGLISH — .PING , .MENU WORK AS ALWAYS*", LanguageName(code)))
}

// botLanguageGuide lists every supported language with its English name. Kept
// in English on purpose (owner order): the default user is English, everyone
// else picks their own language from this list.
func botLanguageGuide(prefix, current string) string {
	cur := "ENGLISH"
	if strings.TrimSpace(current) != "" {
		cur = LanguageName(current)
	}
	cmd := strings.ToUpper(prefix + "BOTLANGUAGE")
	var b strings.Builder
	b.WriteString("*🔰 BOT LANGUAGE GUIDE 🔰*\n\n")
	b.WriteString("*SET THE LANGUAGE THE BOT REPLIES IN*\n")
	b.WriteString("*COMMAND NAMES NEVER CHANGE — .PING , .MENU STAY ENGLISH*\n\n")
	b.WriteString("*❮ " + cmd + " SET <CODE> ❱* → BOT REPLIES IN THAT LANGUAGE\n")
	b.WriteString("*❮ " + cmd + " RESET ❱* → BACK TO ENGLISH\n\n")
	b.WriteString("*CURRENT:❯ ❮ " + cur + " ❱*\n\n")
	b.WriteString("*🔰 SUPPORTED LANGUAGES (" + fmt.Sprint(len(trtLangs)) + ") 🔰*\n")
	for _, l := range trtLangs {
		b.WriteString("*" + l.Code + " — " + l.Name + "*\n")
	}
	b.WriteString("\n*MORE LANGUAGES CAN BE ADDED ON REQUEST*")
	return b.String()
}
