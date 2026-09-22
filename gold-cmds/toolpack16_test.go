package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack16GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack16GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"textstats":     textstatsGuide("."),
		"wordfreq":      wordfreqGuide("."),
		"leapyear":      leapyearGuide("."),
		"domaininfo":    domaininfoGuide("."),
		"phonevalidate": phonevalidateGuide("."),
		"colorinfo":     colorinfoGuide("."),
		"numberfact":    numberfactGuide("."),
		"wordmeaning":   wordmeaningGuide("."),
		"spellcheck":    spellcheckGuide("."),
		"mathcalc":      mathcalcGuide("."),
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

// TestToolpack16Live exercises each new command against its live API.
func TestToolpack16Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"textstats", handleTextstats, []string{"Hello", "world.", "This", "is", "a", "test!"}},
		{"wordfreq", handleWordfreq, []string{"the", "cat", "the", "dog", "the"}},
		{"leapyear", handleLeapyear, []string{"2024"}},
		{"domaininfo", handleDomaininfo, []string{"google.com"}},
		{"phonevalidate", handlePhonevalidate, []string{"+923158930864"}},
		{"colorinfo", handleColorinfo, []string{"FF0000"}},
		{"numberfact", handleNumberfact, []string{"42"}},
		{"wordmeaning", handleWordmeaning, []string{"hello"}},
		{"spellcheck", handleSpellcheck, []string{"recieve"}},
		{"mathcalc", handleMathcalc, []string{"2+2*3"}},
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
