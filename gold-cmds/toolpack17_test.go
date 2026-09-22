package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack17GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack17GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"emailcheck":   emailcheckGuide("."),
		"dnscheck":     dnscheckGuide("."),
		"httpheaders":  httpheadersGuide("."),
		"uuidgen":      uuidgenGuide("."),
		"passwordgen":  passwordgenGuide("."),
		"base64tool":   base64toolGuide("."),
		"hashgen":      hashgenGuide("."),
		"randomnumber": randomnumberGuide("."),
		"coinflip":     coinflipGuide("."),
		"diceroll":     dicerollGuide("."),
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

// TestToolpack17Live exercises each new command against its live API.
func TestToolpack17Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"emailcheck", handleEmailcheck, []string{"test@gmail.com"}},
		{"dnscheck", handleDnscheck, []string{"google.com"}},
		{"httpheaders", handleHttpheaders, []string{"https://github.com"}},
		{"uuidgen", handleUuidgen, nil},
		{"passwordgen", handlePasswordgen, []string{"20"}},
		{"base64tool", handleBase64tool, []string{"enc", "hello"}},
		{"hashgen", handleHashgen, []string{"sha256", "hello"}},
		{"randomnumber", handleRandomnumber, []string{"1", "100"}},
		{"coinflip", handleCoinflip, nil},
		{"diceroll", handleDiceroll, []string{"20"}},
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
