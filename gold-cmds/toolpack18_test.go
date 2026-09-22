package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack18GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack18GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"bmr":          bmrGuide("."),
		"caloriecalc":  caloriecalcGuide("."),
		"bodyfat":      bodyfatGuide("."),
		"idealweight":  idealweightGuide("."),
		"waterintake":  waterintakeGuide("."),
		"mortgage":     mortgageGuide("."),
		"loancalc":     loancalcGuide("."),
		"taxcalc":      taxcalcGuide("."),
		"tipcalc":      tipcalcGuide("."),
		"discountcalc": discountcalcGuide("."),
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

// TestToolpack18Live exercises each new command (all local, no API).
func TestToolpack18Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"bmr", handleBmr, []string{"male", "70", "175", "25"}},
		{"caloriecalc", handleCaloriecalc, []string{"male", "70", "175", "25", "3"}},
		{"bodyfat", handleBodyfat, []string{"male", "175", "38", "85"}},
		{"idealweight", handleIdealweight, []string{"male", "175"}},
		{"waterintake", handleWaterintake, []string{"70", "30"}},
		{"mortgage", handleMortgage, []string{"200000", "5.5", "30"}},
		{"loancalc", handleLoancalc, []string{"50000", "12", "24"}},
		{"taxcalc", handleTaxcalc, []string{"1000", "15"}},
		{"tipcalc", handleTipcalc, []string{"100", "15", "4"}},
		{"discountcalc", handleDiscountcalc, []string{"200", "25"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
