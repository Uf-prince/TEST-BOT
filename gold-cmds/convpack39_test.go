package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack39GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"bartopsi":        makeConvGuide("BAR TO PSI", "bartopsi", "BAR", "PSI")("."),
		"psitobar":        makeConvGuide("PSI TO BAR", "psitobar", "PSI", "BAR")("."),
		"pascaltobar":     makeConvGuide("PASCAL TO BAR", "pascaltobar", "PA", "BAR")("."),
		"bartopascal":     makeConvGuide("BAR TO PASCAL", "bartopascal", "BAR", "PA")("."),
		"atmtopsi":        makeConvGuide("ATM TO PSI", "atmtopsi", "ATM", "PSI")("."),
		"psitoatm":        makeConvGuide("PSI TO ATM", "psitoatm", "PSI", "ATM")("."),
		"newtontokgforce": makeConvGuide("NEWTON TO KGF", "newtontokgforce", "N", "KGF")("."),
		"kgforcetonewton": makeConvGuide("KGF TO NEWTON", "kgforcetonewton", "KGF", "N")("."),
		"mmhgtopascal":    makeConvGuide("MMHG TO PASCAL", "mmhgtopascal", "MMHG", "PA")("."),
		"pascaltommhg":    makeConvGuide("PASCAL TO MMHG", "pascaltommhg", "PA", "MMHG")("."),
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

func TestConvpack39Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"bartopsi", makeConvHandler("BAR TO PSI", "bartopsi", "BAR", "PSI", func(v float64) float64 { return v * 14.503774 }), []string{"1"}},
		{"psitobar", makeConvHandler("PSI TO BAR", "psitobar", "PSI", "BAR", func(v float64) float64 { return v * 0.068948 }), []string{"14.5"}},
		{"pascaltobar", makeConvHandler("PASCAL TO BAR", "pascaltobar", "PA", "BAR", func(v float64) float64 { return v / 100000 }), []string{"100000"}},
		{"bartopascal", makeConvHandler("BAR TO PASCAL", "bartopascal", "BAR", "PA", func(v float64) float64 { return v * 100000 }), []string{"1"}},
		{"atmtopsi", makeConvHandler("ATM TO PSI", "atmtopsi", "ATM", "PSI", func(v float64) float64 { return v * 14.695949 }), []string{"1"}},
		{"psitoatm", makeConvHandler("PSI TO ATM", "psitoatm", "PSI", "ATM", func(v float64) float64 { return v * 0.068046 }), []string{"14.7"}},
		{"newtontokgforce", makeConvHandler("NEWTON TO KGF", "newtontokgforce", "N", "KGF", func(v float64) float64 { return v * 0.101972 }), []string{"10"}},
		{"kgforcetonewton", makeConvHandler("KGF TO NEWTON", "kgforcetonewton", "KGF", "N", func(v float64) float64 { return v * 9.80665 }), []string{"1"}},
		{"mmhgtopascal", makeConvHandler("MMHG TO PASCAL", "mmhgtopascal", "MMHG", "PA", func(v float64) float64 { return v * 133.322387 }), []string{"760"}},
		{"pascaltommhg", makeConvHandler("PASCAL TO MMHG", "pascaltommhg", "PA", "MMHG", func(v float64) float64 { return v * 0.00750062 }), []string{"101325"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-20s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
