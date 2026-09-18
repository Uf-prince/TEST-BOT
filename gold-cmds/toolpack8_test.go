package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack8GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack8GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"dog":     dogGuide("."),
		"fox":     foxGuide("."),
		"duck":    duckGuide("."),
		"coffee":  coffeeGuide("."),
		"cat":     catGuide("."),
		"puppy":   puppyGuide("."),
		"bored":   boredGuide("."),
		"zen":     zenGuide("."),
		"rhyme":   rhymeGuide("."),
		"related": relatedGuide("."),
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

// TestToolpack8Live exercises each new command against its live API.
func TestToolpack8Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"dog", handleDog, nil},
		{"fox", handleFox, nil},
		{"duck", handleDuck, nil},
		{"coffee", handleCoffee, nil},
		{"cat", handleCat, nil},
		{"puppy", handlePuppy, nil},
		{"bored", handleBored, nil},
		{"zen", handleZen, nil},
		{"rhyme", handleRhyme, []string{"love"}},
		{"related", handleRelated, []string{"happy"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") {
			status = "FAIL"
		}
		t.Logf("%-10s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
