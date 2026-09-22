package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack31GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"kmphtomph":       makeConvGuide("KM/H TO MPH", "kmphtomph", "KM/H", "MPH")("."),
		"mphtokmph":       makeConvGuide("MPH TO KM/H", "mphtokmph", "MPH", "KM/H")("."),
		"knotstokmph":     makeConvGuide("KNOTS TO KM/H", "knotstokmph", "KN", "KM/H")("."),
		"msectokmh":       makeConvGuide("M/S TO KM/H", "msectokmh", "M/S", "KM/H")("."),
		"sqfttosqm":       makeConvGuide("SQ FT TO SQ M", "sqfttosqm", "SQFT", "SQM")("."),
		"sqmtosqft":       makeConvGuide("SQ M TO SQ FT", "sqmtosqft", "SQM", "SQFT")("."),
		"acrestohectares": makeConvGuide("ACRES TO HECTARES", "acrestohectares", "AC", "HA")("."),
		"hectarestoacres": makeConvGuide("HECTARES TO ACRES", "hectarestoacres", "HA", "AC")("."),
		"sqkmtosqmi":      makeConvGuide("SQ KM TO SQ MI", "sqkmtosqmi", "SQKM", "SQMI")("."),
		"sqmitosqkm":      makeConvGuide("SQ MI TO SQ KM", "sqmitosqkm", "SQMI", "SQKM")("."),
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

func TestConvpack31Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"kmphtomph", makeConvHandler("KM/H TO MPH", "kmphtomph", "KM/H", "MPH", func(v float64) float64 { return v * 0.621371 }), []string{"100"}},
		{"mphtokmph", makeConvHandler("MPH TO KM/H", "mphtokmph", "MPH", "KM/H", func(v float64) float64 { return v * 1.609344 }), []string{"100"}},
		{"knotstokmph", makeConvHandler("KNOTS TO KM/H", "knotstokmph", "KN", "KM/H", func(v float64) float64 { return v * 1.852 }), []string{"10"}},
		{"msectokmh", makeConvHandler("M/S TO KM/H", "msectokmh", "M/S", "KM/H", func(v float64) float64 { return v * 3.6 }), []string{"10"}},
		{"sqfttosqm", makeConvHandler("SQ FT TO SQ M", "sqfttosqm", "SQFT", "SQM", func(v float64) float64 { return v * 0.092903 }), []string{"100"}},
		{"sqmtosqft", makeConvHandler("SQ M TO SQ FT", "sqmtosqft", "SQM", "SQFT", func(v float64) float64 { return v * 10.763910 }), []string{"100"}},
		{"acrestohectares", makeConvHandler("ACRES TO HECTARES", "acrestohectares", "AC", "HA", func(v float64) float64 { return v * 0.404686 }), []string{"10"}},
		{"hectarestoacres", makeConvHandler("HECTARES TO ACRES", "hectarestoacres", "HA", "AC", func(v float64) float64 { return v * 2.471054 }), []string{"10"}},
		{"sqkmtosqmi", makeConvHandler("SQ KM TO SQ MI", "sqkmtosqmi", "SQKM", "SQMI", func(v float64) float64 { return v * 0.386102 }), []string{"10"}},
		{"sqmitosqkm", makeConvHandler("SQ MI TO SQ KM", "sqmitosqkm", "SQMI", "SQKM", func(v float64) float64 { return v * 2.589988 }), []string{"10"}},
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
