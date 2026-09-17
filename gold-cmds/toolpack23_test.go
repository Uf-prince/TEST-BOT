package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestNewToolpackGuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestNewToolpackGuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"country": countryGuide("."),
		"stock":   stockGuide("."),
		"tvshow":  tvshowGuide("."),
		"book":    bookGuide("."),
		"whois":   whoisGuide("."),
		"dns":     dnsGuide("."),
		"color":   colorGuide("."),
		"npm":     npmGuide("."),
		"pypi":    pypiGuide("."),
		"urban":   urbanGuide("."),
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

// TestNewToolpackLive exercises each new command against its live API.
func TestNewToolpackLive(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"country", handleCountry, []string{"pakistan"}},
		{"stock", handleStock, []string{"AAPL"}},
		{"tvshow", handleTVShow, []string{"breaking", "bad"}},
		{"book", handleBook, []string{"harry", "potter"}},
		{"whois", handleWhois, []string{"google.com"}},
		{"dns", handleDNS, []string{"google.com"}},
		{"color", handleColor, []string{"ff5733"}},
		{"npm", handleNPM, []string{"express"}},
		{"pypi", handlePyPI, []string{"requests"}},
		{"urban", handleUrban, []string{"lit"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FAILED") || strings.Contains(r, "UNAVAILABLE") {
			status = "FAIL"
		}
		t.Logf("%-10s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
