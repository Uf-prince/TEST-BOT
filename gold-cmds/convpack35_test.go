package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack35GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"texttobinary": convGuide(".", "TEXT TO BINARY", "texttobinary", "TEXT", "BINARY"),
		"binarytotext": convGuide(".", "BINARY TO TEXT", "binarytotext", "BINARY", "TEXT"),
		"texttohex":    convGuide(".", "TEXT TO HEX", "texttohex", "TEXT", "HEX"),
		"hextotext":    convGuide(".", "HEX TO TEXT", "hextotext", "HEX", "TEXT"),
		"texttooctal":  convGuide(".", "TEXT TO OCTAL", "texttooctal", "TEXT", "OCTAL"),
		"octaltotext":  convGuide(".", "OCTAL TO TEXT", "octaltotext", "OCTAL", "TEXT"),
		"texttobase64": convGuide(".", "TEXT TO BASE64", "texttobase64", "TEXT", "BASE64"),
		"base64totext": convGuide(".", "BASE64 TO TEXT", "base64totext", "BASE64", "TEXT"),
		"texttourl":    convGuide(".", "TEXT TO URL", "texttourl", "TEXT", "URL-ENCODED"),
		"urltotext":    convGuide(".", "URL TO TEXT", "urltotext", "URL-ENCODED", "TEXT"),
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

func TestConvpack35Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"texttobinary", handleTexttobinary, []string{"hi"}},
		{"binarytotext", handleBinarytotext, []string{"1101000", "1101001"}},
		{"texttohex", handleTexttohex, []string{"hi"}},
		{"hextotext", handleHextotext, []string{"68", "69"}},
		{"texttooctal", handleTexttooctal, []string{"hi"}},
		{"octaltotext", handleOctaltotext, []string{"150", "151"}},
		{"texttobase64", handleTexttobase64, []string{"hello"}},
		{"base64totext", handleBase64totext, []string{"aGVsbG8="}},
		{"texttourl", handleTexttourl, []string{"hello world"}},
		{"urltotext", handleUrltotext, []string{"hello%20world"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
