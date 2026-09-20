package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack9GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack9GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"launch":      launchGuide("."),
		"forecast":    forecastGuide("."),
		"art":         artGuide("."),
		"met":         metGuide("."),
		"bible":       bibleGuide("."),
		"spaceflight": spaceflightGuide("."),
		"blockchain":  blockchainGuide("."),
		"wazirx":      wazirxGuide("."),
		"daylight":    daylightGuide("."),
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

// TestToolpack9Live exercises each new command against its live API.
func TestToolpack9Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"launch", handleLaunch, nil},
		{"forecast", handleForecast, []string{"london"}},
		{"art", handleArt, []string{"monet"}},
		{"met", handleMet, []string{"sunflowers"}},
		{"bible", handleBible, []string{"john", "3:16"}},
		{"spaceflight", handleSpaceflight, nil},
		{"blockchain", handleBlockchain, nil},
		{"wazirx", handleWazirx, []string{"btcusdt"}},
		{"daylight", handleDaylight, []string{"31.5", "74.3"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") {
			status = "FAIL"
		}
		t.Logf("%-12s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
