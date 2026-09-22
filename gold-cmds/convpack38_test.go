package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestConvpack38GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"joulestocalories":     makeConvGuide("JOULES TO CALORIES", "joulestocalories", "J", "CAL")("."),
		"caloriestojoules":     makeConvGuide("CALORIES TO JOULES", "caloriestojoules", "CAL", "J")("."),
		"watttokilowatt":       makeConvGuide("WATTS TO KILOWATTS", "watttokilowatt", "W", "KW")("."),
		"kilowatttowatt":       makeConvGuide("KILOWATTS TO WATTS", "kilowatttowatt", "KW", "W")("."),
		"wattttohorsepower":    makeConvGuide("WATTS TO HORSEPOWER", "wattttohorsepower", "W", "HP")("."),
		"horsepowertowatt":     makeConvGuide("HORSEPOWER TO WATTS", "horsepowertowatt", "HP", "W")("."),
		"kilowatthourtojoules": makeConvGuide("KWH TO JOULES", "kilowatthourtojoules", "KWH", "J")("."),
		"jouletokilojoule":     makeConvGuide("JOULES TO KILOJOULES", "jouletokilojoule", "J", "KJ")("."),
		"kilojouletocalorie":   makeConvGuide("KJ TO KCAL", "kilojouletocalorie", "KJ", "KCAL")("."),
		"kilocaloriestojoules": makeConvGuide("KCAL TO JOULES", "kilocaloriestojoules", "KCAL", "J")("."),
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

func TestConvpack38Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"joulestocalories", makeConvHandler("JOULES TO CALORIES", "joulestocalories", "J", "CAL", func(v float64) float64 { return v * 0.239006 }), []string{"100"}},
		{"caloriestojoules", makeConvHandler("CALORIES TO JOULES", "caloriestojoules", "CAL", "J", func(v float64) float64 { return v * 4.184 }), []string{"100"}},
		{"watttokilowatt", makeConvHandler("WATTS TO KILOWATTS", "watttokilowatt", "W", "KW", func(v float64) float64 { return v / 1000 }), []string{"5000"}},
		{"kilowatttowatt", makeConvHandler("KILOWATTS TO WATTS", "kilowatttowatt", "KW", "W", func(v float64) float64 { return v * 1000 }), []string{"5"}},
		{"wattttohorsepower", makeConvHandler("WATTS TO HORSEPOWER", "wattttohorsepower", "W", "HP", func(v float64) float64 { return v * 0.00134102 }), []string{"1000"}},
		{"horsepowertowatt", makeConvHandler("HORSEPOWER TO WATTS", "horsepowertowatt", "HP", "W", func(v float64) float64 { return v * 745.699872 }), []string{"1"}},
		{"kilowatthourtojoules", makeConvHandler("KWH TO JOULES", "kilowatthourtojoules", "KWH", "J", func(v float64) float64 { return v * 3.6e6 }), []string{"1"}},
		{"jouletokilojoule", makeConvHandler("JOULES TO KILOJOULES", "jouletokilojoule", "J", "KJ", func(v float64) float64 { return v / 1000 }), []string{"5000"}},
		{"kilojouletocalorie", makeConvHandler("KJ TO KCAL", "kilojouletocalorie", "KJ", "KCAL", func(v float64) float64 { return v * 0.239006 }), []string{"100"}},
		{"kilocaloriestojoules", makeConvHandler("KCAL TO JOULES", "kilocaloriestojoules", "KCAL", "J", func(v float64) float64 { return v * 4184 }), []string{"1"}},
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
