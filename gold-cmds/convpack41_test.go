package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack41GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"gramstoml":        makeConvGuide("GRAMS TO ML", "gramstoml", "G", "ML")("."),
		"mltograms":        makeConvGuide("ML TO GRAMS", "mltograms", "ML", "G")("."),
		"oztoml":           makeConvGuide("FL OZ TO ML", "oztoml", "FL OZ", "ML")("."),
		"mltofluidoz":      makeConvGuide("ML TO FL OZ", "mltofluidoz", "ML", "FL OZ")("."),
		"tbspoml":          makeConvGuide("TBSP TO ML", "tbspoml", "TBSP", "ML")("."),
		"mltotbsp":         makeConvGuide("ML TO TBSP", "mltotbsp", "ML", "TBSP")("."),
		"tspoml":           makeConvGuide("TSP TO ML", "tspoml", "TSP", "ML")("."),
		"mltotsp":          makeConvGuide("ML TO TSP", "mltotsp", "ML", "TSP")("."),
		"celsiusrankine":   makeConvGuide("CELSIUS TO RANKINE", "celsiusrankine", "C", "R")("."),
		"rankinetocelsius": makeConvGuide("RANKINE TO CELSIUS", "rankinetocelsius", "R", "C")("."),
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

func TestConvpack41Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"gramstoml", makeConvHandler("GRAMS TO ML", "gramstoml", "G", "ML", func(v float64) float64 { return v }), []string{"250"}},
		{"mltograms", makeConvHandler("ML TO GRAMS", "mltograms", "ML", "G", func(v float64) float64 { return v }), []string{"250"}},
		{"oztoml", makeConvHandler("FL OZ TO ML", "oztoml", "FL OZ", "ML", func(v float64) float64 { return v * 29.57353 }), []string{"8"}},
		{"mltofluidoz", makeConvHandler("ML TO FL OZ", "mltofluidoz", "ML", "FL OZ", func(v float64) float64 { return v * 0.033814 }), []string{"250"}},
		{"tbspoml", makeConvHandler("TBSP TO ML", "tbspoml", "TBSP", "ML", func(v float64) float64 { return v * 14.78676 }), []string{"2"}},
		{"mltotbsp", makeConvHandler("ML TO TBSP", "mltotbsp", "ML", "TBSP", func(v float64) float64 { return v * 0.067628 }), []string{"30"}},
		{"tspoml", makeConvHandler("TSP TO ML", "tspoml", "TSP", "ML", func(v float64) float64 { return v * 4.928922 }), []string{"3"}},
		{"mltotsp", makeConvHandler("ML TO TSP", "mltotsp", "ML", "TSP", func(v float64) float64 { return v * 0.202884 }), []string{"15"}},
		{"celsiusrankine", makeConvHandler("CELSIUS TO RANKINE", "celsiusrankine", "C", "R", func(v float64) float64 { return (v + 273.15) * 1.8 }), []string{"25"}},
		{"rankinetocelsius", makeConvHandler("RANKINE TO CELSIUS", "rankinetocelsius", "R", "C", func(v float64) float64 { return v/1.8 - 273.15 }), []string{"536.67"}},
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
