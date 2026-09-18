package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack12GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack12GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"chuck":       chuckGuide("."),
		"dadjoke":     dadjokeGuide("."),
		"disney":      disneyGuide("."),
		"meme":        memeGuide("."),
		"animequote":  animequoteGuide("."),
		"poetry":      poetryGuide("."),
		"mealdb":      mealdbGuide("."),
		"uselessfact": uselessfactGuide("."),
		"ipgeo":       ipgeoGuide("."),
		"seeip":       seeipGuide("."),
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

// TestToolpack12Live exercises each new command against its live API.
func TestToolpack12Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"chuck", handleChuck, nil},
		{"dadjoke", handleDadjoke, nil},
		{"disney", handleDisney, []string{"mickey", "mouse"}},
		{"meme", handleMeme, nil},
		{"animequote", handleAnimequote, nil},
		{"poetry", handlePoetry, []string{"the", "raven"}},
		{"mealdb", handleMealdb, nil},
		{"uselessfact", handleUselessfact, nil},
		{"ipgeo", handleIpgeo, nil},
		{"seeip", handleSeeip, nil},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-12s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
