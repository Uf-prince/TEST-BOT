package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack27GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"textborder":      textborderGuide("."),
		"textbox":         textboxGuide("."),
		"textfind":        textfindGuide("."),
		"textinsert":      textinsertGuide("."),
		"textleet":        textleetGuide("."),
		"textmirror":      textmirrorGuide("."),
		"textsmallcaps":   textsmallcapsGuide("."),
		"textsubscript":   textsubscriptGuide("."),
		"textsuperscript": textsuperscriptGuide("."),
		"textupsidedown":  textupsidedownGuide("."),
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

func TestToolpack27Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"textborder", handleTextborder, []string{"hello"}},
		{"textbox", handleTextbox, []string{"hello"}},
		{"textfind", handleTextfind, []string{"cat", "|", "the", "cat", "sat", "on", "the", "mat"}},
		{"textinsert", handleTextinsert, []string{"3", "hello", "|", "XX"}},
		{"textleet", handleTextleet, []string{"hello"}},
		{"textmirror", handleTextmirror, []string{"hello"}},
		{"textsmallcaps", handleTextsmallcaps, []string{"hello", "world"}},
		{"textsubscript", handleTextsubscript, []string{"h2o"}},
		{"textsuperscript", handleTextsuperscript, []string{"x2"}},
		{"textupsidedown", handleTextupsidedown, []string{"hello"}},
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
