package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack24GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"readingtime":     readingtimeGuide("."),
		"speakingtime":    speakingtimeGuide("."),
		"isogram":         isogramGuide("."),
		"pangram":         pangramGuide("."),
		"palindromecheck": palindromecheckGuide("."),
		"anagramcheck":    anagramcheckGuide("."),
		"wordshuffle":     wordshuffleGuide("."),
		"textrepeat":      textrepeatGuide("."),
		"textpad":         textpadGuide("."),
		"textcenter":      textcenterGuide("."),
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

func TestToolpack24Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"readingtime", handleReadingtime, []string{"this", "is", "a", "test", "sentence"}},
		{"speakingtime", handleSpeakingtime, []string{"this", "is", "a", "test", "sentence"}},
		{"isogram", handleIsogram, []string{"dermatoglyphics"}},
		{"pangram", handlePangram, []string{"the", "quick", "brown", "fox", "jumps", "over", "the", "lazy", "dog"}},
		{"palindromecheck", handlePalindromecheck, []string{"racecar"}},
		{"anagramcheck", handleAnagramcheck, []string{"listen", "|", "silent"}},
		{"wordshuffle", handleWordshuffle, []string{"hello", "world", "how", "are", "you"}},
		{"textrepeat", handleTextrepeat, []string{"3", "hi"}},
		{"textpad", handleTextpad, []string{"20", "hi"}},
		{"textcenter", handleTextcenter, []string{"20", "hi"}},
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
		time.Sleep(100 * time.Millisecond)
	}
}
