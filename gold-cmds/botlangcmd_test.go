package goldcmds

import (
	"strings"
	"testing"
)

// .botlanguage must be registered (owner-only) with its hidden aliases.
func TestBotLanguageRegistered(t *testing.T) {
	found := map[string]bool{}
	for _, c := range Commands() {
		switch strings.ToLower(c.Name) {
		case "botlanguage":
			found["botlanguage"] = true
			if !c.OwnerOnly {
				t.Error("botlanguage must be OwnerOnly")
			}
		case "language", "botlang":
			found[strings.ToLower(c.Name)] = true
		}
	}
	for _, want := range []string{"botlanguage", "language", "botlang"} {
		if !found[want] {
			t.Errorf(".%s must be registered", want)
		}
	}
}

// The guide lists EVERY language in ENGLISH and shows the set/reset usage.
func TestBotLanguageGuideListsAllLanguages(t *testing.T) {
	g := botLanguageGuide(".", "")
	if !strings.Contains(g, ".BOTLANGUAGE SET <CODE>") {
		t.Errorf("guide missing SET usage\n%s", g)
	}
	if !strings.Contains(g, ".BOTLANGUAGE RESET") {
		t.Errorf("guide missing RESET usage\n%s", g)
	}
	if !strings.Contains(g, "CURRENT:❯ ❮ ENGLISH ❱") {
		t.Errorf("guide must show the current language in English\n%s", g)
	}
	for _, l := range trtLangs {
		if !strings.Contains(g, "*"+l.Code+" — "+l.Name+"*") {
			t.Errorf("guide missing language %s (%s)", l.Code, l.Name)
		}
	}
	// A broad spread of world languages must be present.
	for _, code := range []string{"ur", "hi", "ar", "zh-CN", "ru", "es", "fr", "sw", "ja", "pt"} {
		if _, ok := ResolveLanguage(code); !ok {
			t.Errorf("language %s must resolve", code)
		}
	}
}

// Resolution accepts a code OR the English name, case-insensitively.
func TestBotLanguageResolve(t *testing.T) {
	for _, tok := range []string{"ur", "UR", "urdu", "Urdu"} {
		code, ok := ResolveLanguage(tok)
		if !ok || code != "ur" {
			t.Errorf("ResolveLanguage(%q) = %q,%v want ur,true", tok, code, ok)
		}
	}
	if _, ok := ResolveLanguage("klingon"); ok {
		t.Error("unknown language must not resolve")
	}
	if LanguageName("ur") != "URDU" {
		t.Errorf("LanguageName(ur) = %q", LanguageName("ur"))
	}
}

// Command NAMES are never translated: the bot's command tokens stay ASCII
// English. The language catalog carries only language names, so no translated
// command name can ever be introduced.
func TestBotLanguageCommandsStayEnglish(t *testing.T) {
	for _, c := range Commands() {
		if strings.EqualFold(c.Name, "botlanguage") {
			continue
		}
		for _, r := range c.Name {
			if r > 127 {
				t.Errorf("command %q must be ASCII English", c.Name)
				break
			}
		}
	}
}
