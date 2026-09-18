package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack13GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack13GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"timestamp":     timestampGuide("."),
		"agecalc":       agecalcGuide("."),
		"unitconvert":   unitconvertGuide("."),
		"caseconvert":   caseconvertGuide("."),
		"charcount":     charcountGuide("."),
		"palindrome":    palindromeGuide("."),
		"anagram":       anagramGuide("."),
		"loremipsum":    loremipsumGuide("."),
		"urlparse":      urlparseGuide("."),
		"emailvalidate": emailvalidateGuide("."),
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

// TestToolpack13Live exercises each new command (all local, no API).
func TestToolpack13Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"timestamp", handleTimestamp, []string{"1700000000"}},
		{"agecalc", handleAgecalc, []string{"2000-01-15"}},
		{"unitconvert", handleUnitconvert, []string{"10", "km", "mi"}},
		{"caseconvert", handleCaseconvert, []string{"upper", "hello world"}},
		{"charcount", handleCharcount, []string{"hello", "world"}},
		{"palindrome", handlePalindrome, []string{"racecar"}},
		{"anagram", handleAnagram, []string{"listen", "silent"}},
		{"loremipsum", handleLoremipsum, []string{"2"}},
		{"urlparse", handleUrlparse, []string{"https://example.com/path?x=1"}},
		{"emailvalidate", handleEmailvalidate, []string{"test@example.com"}},
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
