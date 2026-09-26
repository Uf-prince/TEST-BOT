package goldcmds

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The bot OUTPUT language (.botlanguage) is the feature these languages are for:
// a user sets their language and every reply comes back in it. These tests pin
// the catalog the command exposes and the Pakistani coverage the owner asked for.

// The catalog must expose the full Google language set, not the old 136.
func TestBotLanguageCatalogIsFull(t *testing.T) {
	cat := LanguageCatalog()
	if len(cat) < 240 {
		t.Fatalf("LanguageCatalog() has %d languages, want 240+", len(cat))
	}
	seen := map[string]bool{}
	for _, l := range cat {
		if l.Code == "" || l.Name == "" {
			t.Fatalf("catalog entry with empty code/name: %+v", l)
		}
		if seen[l.Code] {
			t.Errorf("duplicate language code %q", l.Code)
		}
		seen[l.Code] = true
	}
}

// Every language a Pakistani user is likely to name must resolve to a code the
// reply translator actually accepts.
func TestBotLanguagePakistaniCoverage(t *testing.T) {
	want := map[string]string{
		"urdu":               "ur",
		"muhajir":            "ur",
		"punjabi (pakistan)": "pa-Arab",
		"saraiki":            "pa-Arab",
		"hindko":             "pa-Arab",
		"pothwari":           "pa-Arab",
		"lahore":             "pa-Arab",
		"multan":             "pa-Arab",
		"rawalpindi":         "pa-Arab",
		"sindhi":             "sd",
		"pashto":             "ps",
		"balochi":            "bal",
		"kashmiri":           "ur",
		"shahmukhi":          "pa-Arab",
		"punjabi (india)":    "pa",
	}
	for tok, code := range want {
		got, ok := ResolveLanguage(tok)
		if !ok || got != code {
			t.Errorf("ResolveLanguage(%q) = %q,%v want %s,true", tok, got, ok, code)
		}
	}
}

// The guide the command sends must list every language in the catalog.
func TestBotLanguageGuideListsWholeCatalog(t *testing.T) {
	g := botLanguageGuide(".", newBLBridge())
	for _, l := range LanguageCatalog() {
		if !strings.Contains(g, l.Code) {
			t.Fatalf("guide missing language code %q", l.Code)
		}
	}
}

// Live end-to-end: setting the bot language to a Pakistani code must produce a
// reply in that language through the same path replies use. Opt-in because the
// normal suite must not depend on the network.
func TestBotLanguageReplyLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE_TEST") != "1" {
		t.Skip("set GOLDMD_LIVE_TEST=1 to run the live reply check")
	}
	cases := []struct{ tok, code, label string }{
		{"saraiki", "pa-Arab", "Saraiki -> Shahmukhi Punjabi"},
		{"lahore", "pa-Arab", "Lahore -> Shahmukhi Punjabi"},
		{"urdu", "ur", "Urdu"},
		{"sindhi", "sd", "Sindhi"},
		{"pashto", "ps", "Pashto"},
		{"yue", "yue", "Cantonese"},
		{"cebuano", "ceb", "Cebuano"},
	}
	for _, c := range cases {
		code, ok := ResolveLanguage(c.tok)
		if !ok || code != c.code {
			t.Errorf("ResolveLanguage(%q) = %q,%v want %s", c.tok, code, ok, c.code)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		out, err := TranslateText(ctx, "The bot is online and working", code)
		cancel()
		if err != nil || strings.TrimSpace(out) == "" {
			t.Errorf("%s: reply translate failed: %v", c.label, err)
			continue
		}
		t.Logf("%-34s -> %s", c.label, out)
	}
}
