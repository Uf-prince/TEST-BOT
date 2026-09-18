package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack42GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"fahrenheittokelvin": makeConvGuide("FAHRENHEIT TO KELVIN", "fahrenheittokelvin", "F", "K")("."),
		"kelvintofahrenheit": makeConvGuide("KELVIN TO FAHRENHEIT", "kelvintofahrenheit", "K", "F")("."),
		"degreestoradians":   makeConvGuide("DEGREES TO RADIANS", "degreestoradians", "DEG", "RAD")("."),
		"radianstodegrees":   makeConvGuide("RADIANS TO DEGREES", "radianstodegrees", "RAD", "DEG")("."),
		"weekstodays":        makeConvGuide("WEEKS TO DAYS", "weekstodays", "WK", "D")("."),
		"monthstoweeks":      makeConvGuide("MONTHS TO WEEKS", "monthstoweeks", "MO", "WK")("."),
		"yearstomonths":      makeConvGuide("YEARS TO MONTHS", "yearstomonths", "YR", "MO")("."),
		"decadestoyears":     makeConvGuide("DECADES TO YEARS", "decadestoyears", "DEC", "YR")("."),
		"hourstominutes":     makeConvGuide("HOURS TO MINUTES", "hourstominutes", "HR", "MIN")("."),
		"daystohours":        makeConvGuide("DAYS TO HOURS", "daystohours", "D", "HR")("."),
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

func TestConvpack42Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"fahrenheittokelvin", makeConvHandler("FAHRENHEIT TO KELVIN", "fahrenheittokelvin", "F", "K", func(v float64) float64 { return (v-32)*5/9 + 273.15 }), []string{"98.6"}},
		{"kelvintofahrenheit", makeConvHandler("KELVIN TO FAHRENHEIT", "kelvintofahrenheit", "K", "F", func(v float64) float64 { return (v-273.15)*9/5 + 32 }), []string{"310"}},
		{"degreestoradians", makeConvHandler("DEGREES TO RADIANS", "degreestoradians", "DEG", "RAD", func(v float64) float64 { return v * 0.0174532925 }), []string{"180"}},
		{"radianstodegrees", makeConvHandler("RADIANS TO DEGREES", "radianstodegrees", "RAD", "DEG", func(v float64) float64 { return v * 57.2957795 }), []string{"3.14159"}},
		{"weekstodays", makeConvHandler("WEEKS TO DAYS", "weekstodays", "WK", "D", func(v float64) float64 { return v * 7 }), []string{"4"}},
		{"monthstoweeks", makeConvHandler("MONTHS TO WEEKS", "monthstoweeks", "MO", "WK", func(v float64) float64 { return v * 4.34524 }), []string{"6"}},
		{"yearstomonths", makeConvHandler("YEARS TO MONTHS", "yearstomonths", "YR", "MO", func(v float64) float64 { return v * 12 }), []string{"5"}},
		{"decadestoyears", makeConvHandler("DECADES TO YEARS", "decadestoyears", "DEC", "YR", func(v float64) float64 { return v * 10 }), []string{"3"}},
		{"hourstominutes", makeConvHandler("HOURS TO MINUTES", "hourstominutes", "HR", "MIN", func(v float64) float64 { return v * 60 }), []string{"2.5"}},
		{"daystohours", makeConvHandler("DAYS TO HOURS", "daystohours", "D", "HR", func(v float64) float64 { return v * 24 }), []string{"3"}},
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
