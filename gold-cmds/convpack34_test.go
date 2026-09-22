package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack34GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"dectobin": makeConvGuide("DECIMAL TO BINARY", "dectobin", "DECIMAL", "BINARY")("."),
		"bintodec": makeConvGuide("BINARY TO DECIMAL", "bintodec", "BINARY", "DECIMAL")("."),
		"dectohex": makeConvGuide("DECIMAL TO HEX", "dectohex", "DECIMAL", "HEX")("."),
		"hextodec": makeConvGuide("HEX TO DECIMAL", "hextodec", "HEX", "DECIMAL")("."),
		"dectooct": makeConvGuide("DECIMAL TO OCTAL", "dectooct", "DECIMAL", "OCTAL")("."),
		"octtodec": makeConvGuide("OCTAL TO DECIMAL", "octtodec", "OCTAL", "DECIMAL")("."),
		"bintohex": makeConvGuide("BINARY TO HEX", "bintohex", "BINARY", "HEX")("."),
		"hextobin": makeConvGuide("HEX TO BINARY", "hextobin", "HEX", "BINARY")("."),
		"octtohex": makeConvGuide("OCTAL TO HEX", "octtohex", "OCTAL", "HEX")("."),
		"hextooct": makeConvGuide("HEX TO OCTAL", "hextooct", "HEX", "OCTAL")("."),
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

func TestConvpack34Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"dectobin", baseConvHandler("DECIMAL TO BINARY", "dectobin", 10, 2, "DECIMAL", "BINARY"), []string{"255"}},
		{"bintodec", baseConvHandler("BINARY TO DECIMAL", "bintodec", 2, 10, "BINARY", "DECIMAL"), []string{"11111111"}},
		{"dectohex", baseConvHandler("DECIMAL TO HEX", "dectohex", 10, 16, "DECIMAL", "HEX"), []string{"255"}},
		{"hextodec", baseConvHandler("HEX TO DECIMAL", "hextodec", 16, 10, "HEX", "DECIMAL"), []string{"FF"}},
		{"dectooct", baseConvHandler("DECIMAL TO OCTAL", "dectooct", 10, 8, "DECIMAL", "OCTAL"), []string{"255"}},
		{"octtodec", baseConvHandler("OCTAL TO DECIMAL", "octtodec", 8, 10, "OCTAL", "DECIMAL"), []string{"377"}},
		{"bintohex", baseConvHandler("BINARY TO HEX", "bintohex", 2, 16, "BINARY", "HEX"), []string{"11111111"}},
		{"hextobin", baseConvHandler("HEX TO BINARY", "hextobin", 16, 2, "HEX", "BINARY"), []string{"FF"}},
		{"octtohex", baseConvHandler("OCTAL TO HEX", "octtohex", 8, 16, "OCTAL", "HEX"), []string{"377"}},
		{"hextooct", baseConvHandler("HEX TO OCTAL", "hextooct", 16, 8, "HEX", "OCTAL"), []string{"FF"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-10s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
