package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack19GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"charfreq":    charfreqGuide("."),
		"uniquewords": uniquewordsGuide("."),
		"longestword": longestwordGuide("."),
		"textreverse": textreverseGuide("."),
		"textwrap":    textwrapGuide("."),
		"texttrim":    texttrimGuide("."),
		"textsplit":   textsplitGuide("."),
		"textreplace": textreplaceGuide("."),
		"textbanner":  textbannerGuide("."),
		"textflip":    textflipGuide("."),
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

func TestToolpack19Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"charfreq", handleCharfreq, []string{"hello", "world"}},
		{"uniquewords", handleUniquewords, []string{"the", "cat", "and", "the", "dog"}},
		{"longestword", handleLongestword, []string{"i", "love", "programming"}},
		{"textreverse", handleTextreverse, []string{"chars", "hello"}},
		{"textwrap", handleTextwrap, []string{"10", "hello", "world", "this", "is", "a", "test"}},
		{"texttrim", handleTexttrim, []string{"  hello   world  "}},
		{"textsplit", handleTextsplit, []string{",", "apple,banana,mango"}},
		{"textreplace", handleTextreplace, []string{"cat", "dog", "i", "love", "cat"}},
		{"textbanner", handleTextbanner, []string{"gold"}},
		{"textflip", handleTextflip, []string{"hello"}},
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
