package goldcmds

import (
	"strings"
	"testing"
)

func TestToolpackGuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"prayer":     prayerGuide("."),
		"quran":      quranGuide("."),
		"dictionary": dictionaryGuide("."),
		"currency":   currencyGuide("."),
		"timezone":   timezoneGuide("."),
		"news":       newsGuide("."),
		"lyrics":     lyricsGuide("."),
		"github":     githubGuide("."),
		"anime":      animeGuide("."),
		"pokemon":    pokemonGuide("."),
	}
	for name, g := range guides {
		if strings.Contains(g, `\n`) {
			t.Fatalf("%s guide contains literal backslash-n", name)
		}
		if !strings.Contains(g, "\n") {
			t.Fatalf("%s guide has no real newline", name)
		}
		if !strings.Contains(g, "🔰") {
			t.Fatalf("%s guide missing 🔰", name)
		}
	}
	// quran guide must list all 114 surahs
	if !strings.Contains(quranGuide("."), "114. An-Naas") {
		t.Fatalf("quran guide missing surah 114")
	}
	if !strings.Contains(quranGuide("."), "1. Al-Faatiha") {
		t.Fatalf("quran guide missing surah 1")
	}
}

func TestQuranLookup(t *testing.T) {
	cases := map[string]int{
		"yaseen":    36,
		"Yaseen":    36,
		"yasin":     36,
		"36":        36,
		"al-ikhlas": 112,
		"ikhlas":    112,
		"alfaatiha": 1,
		"kahf":      18,
	}
	for in, want := range cases {
		s, ok := quranLookupSurah(in)
		if !ok || s.Number != want {
			t.Fatalf("lookup %q = %v ok=%v, want %d", in, s, ok, want)
		}
	}
}
