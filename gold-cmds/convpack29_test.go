package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack29GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"kgtolbs":           makeConvGuide("KG TO LBS", "kgtolbs", "KG", "LB")("."),
		"lbstokg":           makeConvGuide("LBS TO KG", "lbstokg", "LB", "KG")("."),
		"gramstoounces":     makeConvGuide("GRAMS TO OUNCES", "gramstoounces", "G", "OZ")("."),
		"ouncestograms":     makeConvGuide("OUNCES TO GRAMS", "ouncestograms", "OZ", "G")("."),
		"tonstokg":          makeConvGuide("TONS TO KG", "tonstokg", "T", "KG")("."),
		"stonektopounds":    makeConvGuide("STONE TO POUNDS", "stonektopounds", "ST", "LB")("."),
		"milligramstograms": makeConvGuide("MG TO GRAMS", "milligramstograms", "MG", "G")("."),
		"caratstograms":     makeConvGuide("CARATS TO GRAMS", "caratstograms", "CT", "G")("."),
		"quintaltokg":       makeConvGuide("QUINTAL TO KG", "quintaltokg", "Q", "KG")("."),
		"metrictonstolbs":   makeConvGuide("TONS TO LBS", "metrictonstolbs", "T", "LB")("."),
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

func TestConvpack29Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"kgtolbs", makeConvHandler("KG TO LBS", "kgtolbs", "KG", "LB", func(v float64) float64 { return v * 2.204623 }), []string{"10"}},
		{"lbstokg", makeConvHandler("LBS TO KG", "lbstokg", "LB", "KG", func(v float64) float64 { return v * 0.453592 }), []string{"10"}},
		{"gramstoounces", makeConvHandler("GRAMS TO OUNCES", "gramstoounces", "G", "OZ", func(v float64) float64 { return v * 0.035274 }), []string{"10"}},
		{"ouncestograms", makeConvHandler("OUNCES TO GRAMS", "ouncestograms", "OZ", "G", func(v float64) float64 { return v * 28.349523 }), []string{"10"}},
		{"tonstokg", makeConvHandler("TONS TO KG", "tonstokg", "T", "KG", func(v float64) float64 { return v * 1000 }), []string{"10"}},
		{"stonektopounds", makeConvHandler("STONE TO POUNDS", "stonektopounds", "ST", "LB", func(v float64) float64 { return v * 14 }), []string{"10"}},
		{"milligramstograms", makeConvHandler("MG TO GRAMS", "milligramstograms", "MG", "G", func(v float64) float64 { return v / 1000 }), []string{"10"}},
		{"caratstograms", makeConvHandler("CARATS TO GRAMS", "caratstograms", "CT", "G", func(v float64) float64 { return v * 0.2 }), []string{"10"}},
		{"quintaltokg", makeConvHandler("QUINTAL TO KG", "quintaltokg", "Q", "KG", func(v float64) float64 { return v * 100 }), []string{"10"}},
		{"metrictonstolbs", makeConvHandler("TONS TO LBS", "metrictonstolbs", "T", "LB", func(v float64) float64 { return v * 2204.622622 }), []string{"10"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-18s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
