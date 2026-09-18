package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack25GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"compoundinterest": compoundinterestGuide("."),
		"inflation":        inflationGuide("."),
		"macrocalc":        macrocalcGuide("."),
		"ovulation":        ovulationGuide("."),
		"pacecalc":         pacecalcGuide("."),
		"pregnancy":        pregnancyGuide("."),
		"profitcalc":       profitcalcGuide("."),
		"salarycalc":       salarycalcGuide("."),
		"savingscalc":      savingscalcGuide("."),
		"sleepcalc":        sleepcalcGuide("."),
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

func TestToolpack25Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"compoundinterest", handleCompoundinterest, []string{"10000", "8", "5"}},
		{"inflation", handleInflation, []string{"100000", "6", "10"}},
		{"macrocalc", handleMacrocalc, []string{"2200", "muscle"}},
		{"ovulation", handleOvulation, []string{"2026-09-01", "28"}},
		{"pacecalc", handlePacecalc, []string{"5", "25"}},
		{"pregnancy", handlePregnancy, []string{"2026-03-01"}},
		{"profitcalc", handleProfitcalc, []string{"800", "1200"}},
		{"salarycalc", handleSalarycalc, []string{"50000"}},
		{"savingscalc", handleSavingscalc, []string{"1000000", "20000", "6"}},
		{"sleepcalc", handleSleepcalc, []string{"06:30"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-16s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
