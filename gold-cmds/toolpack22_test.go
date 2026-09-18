package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack22GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"chinesezodiac": chinesezodiacGuide("."),
		"birthstone":    birthstoneGuide("."),
		"birthflower":   birthflowerGuide("."),
		"numerology":    numerologyGuide("."),
		"lifepath":      lifepathGuide("."),
		"horoscope":     horoscopeGuide("."),
		"tarot":         tarotGuide("."),
		"dream":         dreamGuide("."),
		"zodiacsign":    zodiacsignGuide("."),
		"compatibility": compatibilityGuide("."),
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
}

func TestToolpack22Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"chinesezodiac", handleChinesezodiac, []string{"1995"}},
		{"birthstone", handleBirthstone, []string{"6"}},
		{"birthflower", handleBirthflower, []string{"4"}},
		{"numerology", handleNumerology, []string{"john"}},
		{"lifepath", handleLifepath, []string{"1990-05-20"}},
		{"horoscope", handleHoroscope, []string{"leo"}},
		{"tarot", handleTarot, []string{}},
		{"dream", handleDream, []string{"water"}},
		{"zodiacsign", handleZodiacsign, []string{"1995-08-10"}},
		{"compatibility", handleCompatibility, []string{"leo", "aries"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
