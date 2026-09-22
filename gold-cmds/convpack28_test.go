package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack28GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"kmtomiles":       makeConvGuide("KM TO MILES", "kmtomiles", "KM", "MI")("."),
		"milestokm":       makeConvGuide("MILES TO KM", "milestokm", "MI", "KM")("."),
		"mttofeet":        makeConvGuide("METRES TO FEET", "mttofeet", "M", "FT")("."),
		"feettometers":    makeConvGuide("FEET TO METRES", "feettometers", "FT", "M")("."),
		"cmtoinches":      makeConvGuide("CM TO INCHES", "cmtoinches", "CM", "IN")("."),
		"inchestocm":      makeConvGuide("INCHES TO CM", "inchestocm", "IN", "CM")("."),
		"mmtocm":          makeConvGuide("MM TO CM", "mmtocm", "MM", "CM")("."),
		"yardstometers":   makeConvGuide("YARDS TO METRES", "yardstometers", "YD", "M")("."),
		"nauticaltomiles": makeConvGuide("NAUTICAL TO MILES", "nauticaltomiles", "NM", "MI")("."),
		"lightyeartokm":   makeConvGuide("LIGHT YEAR TO KM", "lightyeartokm", "LY", "KM")("."),
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

func TestConvpack28Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"kmtomiles", makeConvHandler("KM TO MILES", "kmtomiles", "KM", "MI", func(v float64) float64 { return v * 0.621371 }), []string{"10"}},
		{"milestokm", makeConvHandler("MILES TO KM", "milestokm", "MI", "KM", func(v float64) float64 { return v * 1.609344 }), []string{"10"}},
		{"mttofeet", makeConvHandler("METRES TO FEET", "mttofeet", "M", "FT", func(v float64) float64 { return v * 3.28084 }), []string{"10"}},
		{"feettometers", makeConvHandler("FEET TO METRES", "feettometers", "FT", "M", func(v float64) float64 { return v * 0.3048 }), []string{"10"}},
		{"cmtoinches", makeConvHandler("CM TO INCHES", "cmtoinches", "CM", "IN", func(v float64) float64 { return v * 0.393701 }), []string{"10"}},
		{"inchestocm", makeConvHandler("INCHES TO CM", "inchestocm", "IN", "CM", func(v float64) float64 { return v * 2.54 }), []string{"10"}},
		{"mmtocm", makeConvHandler("MM TO CM", "mmtocm", "MM", "CM", func(v float64) float64 { return v / 10 }), []string{"10"}},
		{"yardstometers", makeConvHandler("YARDS TO METRES", "yardstometers", "YD", "M", func(v float64) float64 { return v * 0.9144 }), []string{"10"}},
		{"nauticaltomiles", makeConvHandler("NAUTICAL TO MILES", "nauticaltomiles", "NM", "MI", func(v float64) float64 { return v * 1.150779 }), []string{"10"}},
		{"lightyeartokm", makeConvHandler("LIGHT YEAR TO KM", "lightyeartokm", "LY", "KM", func(v float64) float64 { return v * 9.4607e12 }), []string{"1"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-16s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
