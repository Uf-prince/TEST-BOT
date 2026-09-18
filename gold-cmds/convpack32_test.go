package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack32GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"bytestokb":   makeConvGuide("BYTES TO KB", "bytestokb", "B", "KB")("."),
		"kbtomb":      makeConvGuide("KB TO MB", "kbtomb", "KB", "MB")("."),
		"mbtogb":      makeConvGuide("MB TO GB", "mbtogb", "MB", "GB")("."),
		"gbtotb":      makeConvGuide("GB TO TB", "gbtotb", "GB", "TB")("."),
		"tbtopb":      makeConvGuide("TB TO PB", "tbtopb", "TB", "PB")("."),
		"gbtomb":      makeConvGuide("GB TO MB", "gbtomb", "GB", "MB")("."),
		"mbtokb":      makeConvGuide("MB TO KB", "mbtokb", "MB", "KB")("."),
		"kbtobytes":   makeConvGuide("KB TO BYTES", "kbtobytes", "KB", "B")("."),
		"bitstobytes": makeConvGuide("BITS TO BYTES", "bitstobytes", "BIT", "B")("."),
		"bytestobits": makeConvGuide("BYTES TO BITS", "bytestobits", "B", "BIT")("."),
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

func TestConvpack32Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"bytestokb", makeConvHandler("BYTES TO KB", "bytestokb", "B", "KB", func(v float64) float64 { return v / 1024 }), []string{"2048"}},
		{"kbtomb", makeConvHandler("KB TO MB", "kbtomb", "KB", "MB", func(v float64) float64 { return v / 1024 }), []string{"2048"}},
		{"mbtogb", makeConvHandler("MB TO GB", "mbtogb", "MB", "GB", func(v float64) float64 { return v / 1024 }), []string{"2048"}},
		{"gbtotb", makeConvHandler("GB TO TB", "gbtotb", "GB", "TB", func(v float64) float64 { return v / 1024 }), []string{"2048"}},
		{"tbtopb", makeConvHandler("TB TO PB", "tbtopb", "TB", "PB", func(v float64) float64 { return v / 1024 }), []string{"2048"}},
		{"gbtomb", makeConvHandler("GB TO MB", "gbtomb", "GB", "MB", func(v float64) float64 { return v * 1024 }), []string{"2"}},
		{"mbtokb", makeConvHandler("MB TO KB", "mbtokb", "MB", "KB", func(v float64) float64 { return v * 1024 }), []string{"2"}},
		{"kbtobytes", makeConvHandler("KB TO BYTES", "kbtobytes", "KB", "B", func(v float64) float64 { return v * 1024 }), []string{"2"}},
		{"bitstobytes", makeConvHandler("BITS TO BYTES", "bitstobytes", "BIT", "B", func(v float64) float64 { return v / 8 }), []string{"64"}},
		{"bytestobits", makeConvHandler("BYTES TO BITS", "bytestobits", "B", "BIT", func(v float64) float64 { return v * 8 }), []string{"8"}},
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
