package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack15GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack15GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"prayertimes":     prayertimesGuide("."),
		"hijridate":       hijridateGuide("."),
		"qibladirection":  qibladirectionGuide("."),
		"asmaulhusna":     asmaulhusnaGuide("."),
		"worldclock":      worldclockGuide("."),
		"timezoneconvert": timezoneconvertGuide("."),
		"ipdetails":       ipdetailsGuide("."),
		"countryfacts":    countryfactsGuide("."),
		"countrycapital":  countrycapitalGuide("."),
		"sslcheck":        sslcheckGuide("."),
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

// TestToolpack15Live exercises each new command against its live API.
func TestToolpack15Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"prayertimes", handlePrayertimes, []string{"Karachi", "Pakistan"}},
		{"hijridate", handleHijridate, []string{"18-09-2026"}},
		{"qibladirection", handleQibladirection, []string{"24.86", "67.01"}},
		{"asmaulhusna", handleAsmaulhusna, []string{"1"}},
		{"worldclock", handleWorldclock, []string{"Asia/Karachi"}},
		{"timezoneconvert", handleTimezoneconvert, []string{"America/New_York", "Asia/Karachi", "2026-09-18", "12:00"}},
		{"ipdetails", handleIpdetails, []string{"8.8.8.8"}},
		{"countryfacts", handleCountryfacts, []string{"Pakistan"}},
		{"countrycapital", handleCountrycapital, []string{"Japan"}},
		{"sslcheck", handleSslcheck, []string{"github.com"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-16s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
