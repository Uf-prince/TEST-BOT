package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack21GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"rot13":        rot13Guide("."),
		"caesar":       caesarGuide("."),
		"atbash":       atbashGuide("."),
		"vigenere":     vigenereGuide("."),
		"leetspeak":    leetspeakGuide("."),
		"morseencode":  morseencodeGuide("."),
		"morsedecode":  morsedecodeGuide("."),
		"binaryencode": binaryencodeGuide("."),
		"binarydecode": binarydecodeGuide("."),
		"hexencode":    hexencodeGuide("."),
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

func TestToolpack21Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"rot13", handleRot13, []string{"hello"}},
		{"caesar", handleCaesar, []string{"3", "hello"}},
		{"atbash", handleAtbash, []string{"hello"}},
		{"vigenere", handleVigenere, []string{"key", "hello"}},
		{"leetspeak", handleLeetspeak, []string{"elite"}},
		{"morseencode", handleMorseencode, []string{"sos"}},
		{"morsedecode", handleMorsedecode, []string{"...", "---", "..."}},
		{"binaryencode", handleBinaryencode, []string{"hi"}},
		{"binarydecode", handleBinarydecode, []string{"01001000", "01001001"}},
		{"hexencode", handleHexencode, []string{"hi"}},
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
