package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack30GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"celsiustofahrenheit": makeConvGuide("CELSIUS TO FAHRENHEIT", "celsiustofahrenheit", "C", "F")("."),
		"fahrenheittocelsius": makeConvGuide("FAHRENHEIT TO CELSIUS", "fahrenheittocelsius", "F", "C")("."),
		"celsiustokelvin":     makeConvGuide("CELSIUS TO KELVIN", "celsiustokelvin", "C", "K")("."),
		"kelvintocelsius":     makeConvGuide("KELVIN TO CELSIUS", "kelvintocelsius", "K", "C")("."),
		"literstogallons":     makeConvGuide("LITRES TO GALLONS", "literstogallons", "L", "GAL")("."),
		"gallonstoliters":     makeConvGuide("GALLONS TO LITRES", "gallonstoliters", "GAL", "L")("."),
		"mltofloz":            makeConvGuide("ML TO FL OZ", "mltofloz", "ML", "FLOZ")("."),
		"cuptoml":             makeConvGuide("CUPS TO ML", "cuptoml", "CUP", "ML")("."),
		"pintstoliters":       makeConvGuide("PINTS TO LITRES", "pintstoliters", "PT", "L")("."),
		"quartstoliters":      makeConvGuide("QUARTS TO LITRES", "quartstoliters", "QT", "L")("."),
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

func TestConvpack30Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"celsiustofahrenheit", makeConvHandler("CELSIUS TO FAHRENHEIT", "celsiustofahrenheit", "C", "F", func(v float64) float64 { return v*9/5 + 32 }), []string{"100"}},
		{"fahrenheittocelsius", makeConvHandler("FAHRENHEIT TO CELSIUS", "fahrenheittocelsius", "F", "C", func(v float64) float64 { return (v - 32) * 5 / 9 }), []string{"212"}},
		{"celsiustokelvin", makeConvHandler("CELSIUS TO KELVIN", "celsiustokelvin", "C", "K", func(v float64) float64 { return v + 273.15 }), []string{"25"}},
		{"kelvintocelsius", makeConvHandler("KELVIN TO CELSIUS", "kelvintocelsius", "K", "C", func(v float64) float64 { return v - 273.15 }), []string{"300"}},
		{"literstogallons", makeConvHandler("LITRES TO GALLONS", "literstogallons", "L", "GAL", func(v float64) float64 { return v * 0.264172 }), []string{"10"}},
		{"gallonstoliters", makeConvHandler("GALLONS TO LITRES", "gallonstoliters", "GAL", "L", func(v float64) float64 { return v * 3.785412 }), []string{"10"}},
		{"mltofloz", makeConvHandler("ML TO FL OZ", "mltofloz", "ML", "FLOZ", func(v float64) float64 { return v * 0.033814 }), []string{"100"}},
		{"cuptoml", makeConvHandler("CUPS TO ML", "cuptoml", "CUP", "ML", func(v float64) float64 { return v * 236.588236 }), []string{"2"}},
		{"pintstoliters", makeConvHandler("PINTS TO LITRES", "pintstoliters", "PT", "L", func(v float64) float64 { return v * 0.473176 }), []string{"4"}},
		{"quartstoliters", makeConvHandler("QUARTS TO LITRES", "quartstoliters", "QT", "L", func(v float64) float64 { return v * 0.946353 }), []string{"4"}},
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
