package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack40GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"hzrtokhz":     makeConvGuide("HZ TO KHZ", "hzrtokhz", "HZ", "KHZ")("."),
		"khztomhz":     makeConvGuide("KHZ TO MHZ", "khztomhz", "KHZ", "MHZ")("."),
		"mhztoghz":     makeConvGuide("MHZ TO GHZ", "mhztoghz", "MHZ", "GHZ")("."),
		"mpgtokmpl":    makeConvGuide("MPG TO KM/L", "mpgtokmpl", "MPG", "KM/L")("."),
		"kmpltompg":    makeConvGuide("KM/L TO MPG", "kmpltompg", "KM/L", "MPG")("."),
		"lp100kmtompg": makeConvGuide("L/100KM TO MPG", "lp100kmtompg", "L/100KM", "MPG")("."),
		"nmtolf":       makeConvGuide("NM TO FT-LB", "nmtolf", "NM", "FT-LB")("."),
		"lftonm":       makeConvGuide("FT-LB TO NM", "lftonm", "FT-LB", "NM")("."),
		"rpmtorad":     makeConvGuide("RPM TO RAD/S", "rpmtorad", "RPM", "RAD/S")("."),
		"radtorpm":     makeConvGuide("RAD/S TO RPM", "radtorpm", "RAD/S", "RPM")("."),
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

func TestConvpack40Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"hzrtokhz", makeConvHandler("HZ TO KHZ", "hzrtokhz", "HZ", "KHZ", func(v float64) float64 { return v / 1000 }), []string{"5000"}},
		{"khztomhz", makeConvHandler("KHZ TO MHZ", "khztomhz", "KHZ", "MHZ", func(v float64) float64 { return v / 1000 }), []string{"2500"}},
		{"mhztoghz", makeConvHandler("MHZ TO GHZ", "mhztoghz", "MHZ", "GHZ", func(v float64) float64 { return v / 1000 }), []string{"2400"}},
		{"mpgtokmpl", makeConvHandler("MPG TO KM/L", "mpgtokmpl", "MPG", "KM/L", func(v float64) float64 { return v * 0.425144 }), []string{"30"}},
		{"kmpltompg", makeConvHandler("KM/L TO MPG", "kmpltompg", "KM/L", "MPG", func(v float64) float64 { return v * 2.35215 }), []string{"12"}},
		{"lp100kmtompg", makeConvHandler("L/100KM TO MPG", "lp100kmtompg", "L/100KM", "MPG", func(v float64) float64 { return 235.215 / v }), []string{"8"}},
		{"nmtolf", makeConvHandler("NM TO FT-LB", "nmtolf", "NM", "FT-LB", func(v float64) float64 { return v * 0.737562 }), []string{"100"}},
		{"lftonm", makeConvHandler("FT-LB TO NM", "lftonm", "FT-LB", "NM", func(v float64) float64 { return v * 1.355818 }), []string{"100"}},
		{"rpmtorad", makeConvHandler("RPM TO RAD/S", "rpmtorad", "RPM", "RAD/S", func(v float64) float64 { return v * 0.10472 }), []string{"3000"}},
		{"radtorpm", makeConvHandler("RAD/S TO RPM", "radtorpm", "RAD/S", "RPM", func(v float64) float64 { return v * 9.549297 }), []string{"100"}},
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
