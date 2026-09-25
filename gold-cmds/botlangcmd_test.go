package goldcmds

import (
	"context"
	"strings"
	"testing"
	"time"
)

// blBridge is a fake SessionBridge for the .botlanguage tests: it stores the
// per-bot settings map and the bot JID.
type blBridge struct {
	SessionBridge
	jid  string
	vals map[string]string
}

func newBLBridge() *blBridge {
	return &blBridge{jid: "923000000000@s.whatsapp.net", vals: map[string]string{}}
}

func (f *blBridge) GetJID() string { return f.jid }
func (f *blBridge) GetStatusSetting(field, def string) string {
	if v, ok := f.vals[field]; ok && v != "" {
		return v
	}
	return def
}
func (f *blBridge) SetStatusSetting(field, val string) { f.vals[field] = val }
func (f *blBridge) GetBotLanguageSetting(def string) string {
	if v, ok := f.vals["botlanguage"]; ok && v != "" {
		return v
	}
	return def
}
func (f *blBridge) SetBotLanguageSetting(val string) { f.vals["botlanguage"] = val }

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

// The guide lists EVERY language in ENGLISH and shows set/reset/commands usage.
func TestBotLanguageGuideListsAllLanguages(t *testing.T) {
	g := botLanguageGuide(".", newBLBridge())
	if !strings.Contains(g, ".BOTLANGUAGE SET <CODE>") {
		t.Errorf("guide missing SET usage\n%s", g)
	}
	if !strings.Contains(g, ".BOTLANGUAGE RESET") {
		t.Errorf("guide missing RESET usage\n%s", g)
	}
	if !strings.Contains(g, ".BOTLANGUAGE COMMANDS") {
		t.Errorf("guide missing COMMANDS usage\n%s", g)
	}
	if !strings.Contains(g, "CURRENT:❯ ❮ ENGLISH ❱") {
		t.Errorf("guide must show the current language in English\n%s", g)
	}
	for _, l := range trtLangs {
		if !strings.Contains(g, "*"+l.Code+" — "+l.Name+"*") {
			t.Errorf("guide missing language %s (%s)", l.Code, l.Name)
		}
	}
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

// A COUNTRY / CITY / DIALECT name must resolve to the nearest language Google
// actually supports (owner order: har sheher / har gaon ki zuban).
func TestBotLanguageResolvesRegionsAndDialects(t *testing.T) {
	cases := map[string]string{
		"saraiki":            "pa",
		"hindko":             "pa",
		"pothwari":           "pa",
		"lahore":             "pa",
		"karachi":            "ur",
		"islamabad":          "ur",
		"kashmiri":           "ur",
		"peshawar":           "ps",
		"quetta":             "bal",
		"mumbai":             "mr",
		"chennai":            "ta",
		"kolkata":            "bn",
		"nairobi":            "sw",
		"lagos":              "yo",
		"cairo":              "ar",
		"istanbul":           "tr",
		"paris":              "fr",
		"tokyo":              "ja",
		"nepal":              "ne",
		"dhaka":              "bn",
		"punjabi (pakistan)": "pa",
	}
	for tok, want := range cases {
		code, ok := ResolveLanguage(tok)
		if !ok || code != want {
			t.Errorf("ResolveLanguage(%q) = %q,%v want %s,true", tok, code, ok, want)
		}
	}
}

// Every region alias must point at a code that exists in the language catalog,
// otherwise the bot would accept a region and then fail to translate.
func TestBotLanguageRegionAliasesPointAtRealCodes(t *testing.T) {
	known := map[string]bool{}
	for _, l := range trtLangs {
		known[l.Code] = true
	}
	for name, code := range trtRegionAliases {
		if !known[code] {
			t.Errorf("region alias %q -> %q is not in trtLangs", name, code)
		}
	}
}

// Command NAMES are never translated. Localized names are pure aliases kept in
// Redis, never registry entries — so the registry stays ASCII English only.
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

// A localized typed name resolves back to its English command; English names
// pass through untouched; unknown names never match.
func TestCmdLocalizeResolve(t *testing.T) {
	st := clParse("ur:مینو=menu|پنگ=ping|زندہ=alive")
	if got, ok := clResolveLocal(st, "مینو"); !ok || got != "menu" {
		t.Errorf("مینو -> %q,%v want menu,true", got, ok)
	}
	if got, ok := clResolveLocal(st, "پنگ"); !ok || got != "ping" {
		t.Errorf("پنگ -> %q,%v want ping,true", got, ok)
	}
	if got, ok := clResolveLocal(st, "menu"); ok || got != "menu" {
		t.Errorf("english menu must pass through unchanged, got %q,%v", got, ok)
	}
	if _, ok := clResolveLocal(st, "kuchbhi"); ok {
		t.Error("unknown localized token must not resolve")
	}
}

// The stored field round-trips through parse/encode.
func TestCmdLocalizeStoreRoundTrip(t *testing.T) {
	raw := clEncode("hi", [][2]string{{"मेनू", "menu"}, {"पिंग", "ping"}})
	st := clParse(raw)
	if st.Lang != "hi" {
		t.Errorf("lang = %q want hi", st.Lang)
	}
	if len(st.ByLoc) != 2 || st.ByLoc["मेनू"] != "menu" || st.ByLoc["पिंग"] != "ping" {
		t.Errorf("parsed map wrong: %#v", st.ByLoc)
	}
}

// The background builder stores one localized name per canonical command for the
// bot's language, chunking the translation (the endpoint rejects very large
// payloads) and dropping unusable output (unchanged / multi-word).
func TestCmdLocalizeBuildStoresNames(t *testing.T) {
	b := newBLBridge()
	canon := clCanonicalNames()
	if len(canon) < 3 {
		t.Fatalf("expected several canonical command names, got %d", len(canon))
	}
	// Fake translator: prefix every line so nothing is "unchanged", except the
	// first line of each chunk, which is returned verbatim to exercise the drop
	// rule. Also assert the builder keeps chunks within the endpoint limit.
	const limit = 150
	CmdLocalizeAttachTranslator(func(ctx context.Context, text, target string) (string, error) {
		lines := strings.Split(text, "\n")
		if len(lines) > limit {
			return "", context.DeadlineExceeded // simulates the HTTP 400
		}
		for i := range lines {
			if i > 0 {
				lines[i] = "xx" + lines[i]
			}
		}
		return strings.Join(lines, "\n"), nil
	})
	defer CmdLocalizeAttachTranslator(nil)

	clBuildAsync(b, b.jid, "ur")
	for i := 0; i < 400 && b.GetStatusSetting(clMapField, "") == ""; i++ {
		time.Sleep(5 * time.Millisecond)
	}

	st := clParse(b.GetStatusSetting(clMapField, ""))
	if st.Lang != "ur" {
		t.Fatalf("stored lang = %q want ur", st.Lang)
	}
	// One name is dropped per chunk (the unchanged first line).
	chunks := (len(canon) + limit - 1) / limit
	if len(st.ByLoc) != len(canon)-chunks {
		t.Errorf("stored %d names, want %d (%d chunks x 1 dropped)", len(st.ByLoc), len(canon)-chunks, chunks)
	}
	if got := st.ByLoc["xx"+canon[1]]; got != canon[1] {
		t.Errorf("xx%s -> %q want %s", canon[1], got, canon[1])
	}
	// canon[0] came back unchanged from the translator, so it must be dropped.
	if _, present := st.ByLoc["xx"+canon[0]]; present {
		t.Errorf("unchanged translation for %q must be dropped", canon[0])
	}
}

// LocalizedCommandList returns (english, localized) pairs for the menu/guide.
func TestCmdLocalizeListAndLookup(t *testing.T) {
	b := newBLBridge()
	b.SetStatusSetting(clMapField, clEncode("ur", [][2]string{{"مینو", "menu"}, {"پنگ", "ping"}}))
	clMu.Lock()
	delete(clCache, b.jid)
	clMu.Unlock()

	pairs := LocalizedCommandList(b)
	if len(pairs) != 2 {
		t.Fatalf("pairs = %d want 2", len(pairs))
	}
	if pairs[0][0] != "menu" || pairs[1][0] != "ping" {
		t.Errorf("pairs not sorted by english name: %#v", pairs)
	}
	if got := CmdLocalizeLookup(b, "menu"); got != "مینو" {
		t.Errorf("lookup menu = %q want مینو", got)
	}
	if got := CmdLocalizeLookup(b, "nomatch"); got != "" {
		t.Errorf("lookup nomatch = %q want empty", got)
	}
}
