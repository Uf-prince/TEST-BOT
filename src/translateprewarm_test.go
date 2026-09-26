package main

import (
	"strings"
	"testing"
)

// TestCollectPrewarmTextsCoversEveryMenu: prewarm har menu render karta hai —
// .logo, .font, .game, .equalizer, .botstyle, main menu, aur har category.
func TestCollectPrewarmTextsCoversEveryMenu(t *testing.T) {
	s := &Session{JID: "923158930864@s.whatsapp.net"}
	texts := collectPrewarmTexts(s)
	if len(texts) < 6 {
		t.Fatalf("expected at least 6 menu texts, got %d", len(texts))
	}
	joined := strings.Join(texts, "\n")
	// Every render must carry the IMPORTANT CMNDS block; that is the marker that
	// we actually rendered a menu (and not an empty string).
	if n := strings.Count(joined, "IMPORTANT CMNDS"); n < len(texts) {
		t.Fatalf("har menu me IMPORTANT CMNDS hona chahiye: %d blocks for %d menus", n, len(texts))
	}
}

// TestPrewarmSkipsWhenNoLanguage: language empty ya English ho to kuch prewarm
// nahi hota (warna bekaar translate calls).
func TestPrewarmSkipsWhenNoLanguage(t *testing.T) {
	s := &Session{JID: "923158930864@s.whatsapp.net"}
	for _, lang := range []string{"", "en"} {
		prewarmAllTranslations(s, lang)
	}
	// No Manager → no cache writes; the call must simply return, not panic.
	if _, ok := rcGet(s.JID, "ur", "X"); ok {
		t.Fatalf("kuch cache ho gaya jo nahi hona chahiye tha")
	}
}
