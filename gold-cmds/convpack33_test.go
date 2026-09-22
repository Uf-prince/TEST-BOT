package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack33GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"secondstominutes": makeConvGuide("SECONDS TO MINUTES", "secondstominutes", "SEC", "MIN")("."),
		"minutestohours":   makeConvGuide("MINUTES TO HOURS", "minutestohours", "MIN", "HR")("."),
		"hourstodays":      makeConvGuide("HOURS TO DAYS", "hourstodays", "HR", "DAY")("."),
		"daystoweeks":      makeConvGuide("DAYS TO WEEKS", "daystoweeks", "DAY", "WK")("."),
		"weekstomonths":    makeConvGuide("WEEKS TO MONTHS", "weekstomonths", "WK", "MO")("."),
		"monthstoyears":    makeConvGuide("MONTHS TO YEARS", "monthstoyears", "MO", "YR")("."),
		"yearstodecades":   makeConvGuide("YEARS TO DECADES", "yearstodecades", "YR", "DEC")("."),
		"millistoseconds":  makeConvGuide("MILLISECONDS TO SECONDS", "millistoseconds", "MS", "SEC")("."),
		"secondstomillis":  makeConvGuide("SECONDS TO MILLISECONDS", "secondstomillis", "SEC", "MS")("."),
		"minutestoseconds": makeConvGuide("MINUTES TO SECONDS", "minutestoseconds", "MIN", "SEC")("."),
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

func TestConvpack33Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"secondstominutes", makeConvHandler("SECONDS TO MINUTES", "secondstominutes", "SEC", "MIN", func(v float64) float64 { return v / 60 }), []string{"3600"}},
		{"minutestohours", makeConvHandler("MINUTES TO HOURS", "minutestohours", "MIN", "HR", func(v float64) float64 { return v / 60 }), []string{"120"}},
		{"hourstodays", makeConvHandler("HOURS TO DAYS", "hourstodays", "HR", "DAY", func(v float64) float64 { return v / 24 }), []string{"48"}},
		{"daystoweeks", makeConvHandler("DAYS TO WEEKS", "daystoweeks", "DAY", "WK", func(v float64) float64 { return v / 7 }), []string{"14"}},
		{"weekstomonths", makeConvHandler("WEEKS TO MONTHS", "weekstomonths", "WK", "MO", func(v float64) float64 { return v / 4.345 }), []string{"52"}},
		{"monthstoyears", makeConvHandler("MONTHS TO YEARS", "monthstoyears", "MO", "YR", func(v float64) float64 { return v / 12 }), []string{"24"}},
		{"yearstodecades", makeConvHandler("YEARS TO DECADES", "yearstodecades", "YR", "DEC", func(v float64) float64 { return v / 10 }), []string{"50"}},
		{"millistoseconds", makeConvHandler("MILLISECONDS TO SECONDS", "millistoseconds", "MS", "SEC", func(v float64) float64 { return v / 1000 }), []string{"5000"}},
		{"secondstomillis", makeConvHandler("SECONDS TO MILLISECONDS", "secondstomillis", "SEC", "MS", func(v float64) float64 { return v * 1000 }), []string{"5"}},
		{"minutestoseconds", makeConvHandler("MINUTES TO SECONDS", "minutestoseconds", "MIN", "SEC", func(v float64) float64 { return v * 60 }), []string{"5"}},
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
