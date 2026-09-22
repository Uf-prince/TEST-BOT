package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack37GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"degtorad":      convGuide(".", "DEGREES TO RADIANS", "degtorad", "DEG", "RAD"),
		"radtodeg":      convGuide(".", "RADIANS TO DEGREES", "radtodeg", "RAD", "DEG"),
		"fractiontodec": convGuide(".", "FRACTION TO DECIMAL", "fractiontodec", "FRACTION", "DECIMAL"),
		"dectofraction": convGuide(".", "DECIMAL TO FRACTION", "dectofraction", "DECIMAL", "FRACTION"),
		"percenttodec":  convGuide(".", "PERCENT TO DECIMAL", "percenttodec", "PERCENT", "DECIMAL"),
		"dectopercent":  convGuide(".", "DECIMAL TO PERCENT", "dectopercent", "DECIMAL", "PERCENT"),
		"galtooz":       makeConvGuide("GALLONS TO FL OZ", "galtooz", "GAL", "FLOZ")("."),
		"oztogal":       makeConvGuide("FL OZ TO GALLONS", "oztogal", "FLOZ", "GAL")("."),
		"stonetokg":     makeConvGuide("STONE TO KG", "stonetokg", "ST", "KG")("."),
		"knotstomph":    makeConvGuide("KNOTS TO MPH", "knotstomph", "KN", "MPH")("."),
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

func TestConvpack37Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"degtorad", handleDegtorad, []string{"180"}},
		{"radtodeg", handleRadtodeg, []string{"3.14159"}},
		{"fractiontodec", handleFractiontodec, []string{"3/4"}},
		{"dectofraction", handleDectofraction, []string{"0.75"}},
		{"percenttodec", handlePercenttodec, []string{"25%"}},
		{"dectopercent", handleDectopercent, []string{"0.25"}},
		{"galtooz", makeConvHandler("GALLONS TO FL OZ", "galtooz", "GAL", "FLOZ", func(v float64) float64 { return v * 128 }), []string{"2"}},
		{"oztogal", makeConvHandler("FL OZ TO GALLONS", "oztogal", "FLOZ", "GAL", func(v float64) float64 { return v / 128 }), []string{"256"}},
		{"stonetokg", makeConvHandler("STONE TO KG", "stonetokg", "ST", "KG", func(v float64) float64 { return v * 6.350293 }), []string{"10"}},
		{"knotstomph", makeConvHandler("KNOTS TO MPH", "knotstomph", "KN", "MPH", func(v float64) float64 { return v * 1.150779 }), []string{"10"}},
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
