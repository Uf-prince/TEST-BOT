package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack36GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"numtoroman":  convGuide(".", "NUMBER TO ROMAN", "numtoroman", "NUMBER", "ROMAN"),
		"romantonum":  convGuide(".", "ROMAN TO NUMBER", "romantonum", "ROMAN", "NUMBER"),
		"numtowords":  convGuide(".", "NUMBER TO WORDS", "numtowords", "NUMBER", "WORDS"),
		"wordstonum":  convGuide(".", "WORDS TO NUMBER", "wordstonum", "WORDS", "NUMBER"),
		"numtobinary": convGuide(".", "NUMBER TO BINARY", "numtobinary", "NUMBER", "BINARY"),
		"numtohex":    convGuide(".", "NUMBER TO HEX", "numtohex", "NUMBER", "HEX"),
		"numtooctal":  convGuide(".", "NUMBER TO OCTAL", "numtooctal", "NUMBER", "OCTAL"),
		"numtobase36": convGuide(".", "NUMBER TO BASE36", "numtobase36", "NUMBER", "BASE36"),
		"base36tonum": convGuide(".", "BASE36 TO NUMBER", "base36tonum", "BASE36", "NUMBER"),
		"numtobase32": convGuide(".", "NUMBER TO BASE32", "numtobase32", "NUMBER", "BASE32"),
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

func TestConvpack36Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"numtoroman", handleNumtoroman, []string{"1987"}},
		{"romantonum", handleRomantonum, []string{"MCMLXXXVII"}},
		{"numtowords", handleNumtowords, []string{"1234"}},
		{"wordstonum", handleWordstonum, []string{"one", "thousand", "two", "hundred", "thirty", "four"}},
		{"numtobinary", numBaseHandler("NUMBER TO BINARY", "numtobinary", 2, "BINARY"), []string{"255"}},
		{"numtohex", numBaseHandler("NUMBER TO HEX", "numtohex", 16, "HEX"), []string{"255"}},
		{"numtooctal", numBaseHandler("NUMBER TO OCTAL", "numtooctal", 8, "OCTAL"), []string{"255"}},
		{"numtobase36", numBaseHandler("NUMBER TO BASE36", "numtobase36", 36, "BASE36"), []string{"123456"}},
		{"base36tonum", handleBase36tonum, []string{"2n9c"}},
		{"numtobase32", numBaseHandler("NUMBER TO BASE32", "numtobase32", 32, "BASE32"), []string{"123456"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-12s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
