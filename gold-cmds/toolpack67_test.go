package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack67GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack67GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		// pack 6
		"bmi":         bmiGuide("."),
		"loan":        loanGuide("."),
		"tip":         tipGuide("."),
		"percentage":  percentageGuide("."),
		"unit":        unitGuide("."),
		"temperature": tempGuide("."),
		"zodiac":      zodiacGuide("."),
		"countdown":   countdownGuide("."),
		"daysbetween": daysBetweenGuide("."),
		"uuid":        uuidGuide("."),
		"password":    passwordGuide("."),
		"base64":      base64Guide("."),
		"morse":       morseGuide("."),
		"roman":       romanGuide("."),
		"calc":        calcGuide("."),
		// pack 7
		"binary":      binaryGuide("."),
		"hex":         hexGuide("."),
		"reverse":     reverseGuide("."),
		"wordcount":   wordCountGuide("."),
		"urlencode":   urlEncodeGuide("."),
		"dice":        diceGuide("."),
		"random":      randomGuide("."),
		"prime":       primeGuide("."),
		"fibonacci":   fibonacciGuide("."),
		"gcd":         gcdGuide("."),
		"cve":         cveGuide("."),
		"airquality":  airQualityGuide("."),
		"moon":        moonGuide("."),
		"countryinfo": countryInfoGuide("."),
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

// TestToolpack67Live exercises each new command (local + live API).
func TestToolpack67Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		// pack 6 (local)
		{"bmi", handleBMI, []string{"70", "175"}},
		{"loan", handleLoan, []string{"500000", "12", "60"}},
		{"tip", handleTip, []string{"1000", "10", "4"}},
		{"percentage", handlePercentage, []string{"25", "200"}},
		{"unit", handleUnit, []string{"10", "km", "mi"}},
		{"temperature", handleTemperature, []string{"100", "c", "f"}},
		{"zodiac", handleZodiac, []string{"15/08"}},
		{"countdown", handleCountdown, []string{"2027-01-01"}},
		{"daysbetween", handleDaysBetween, []string{"2025-01-01", "2025-12-31"}},
		{"uuid", handleUUID, []string{"3"}},
		{"password", handlePassword, []string{"16"}},
		{"base64", handleBase64, []string{"hello world"}},
		{"morse", handleMorse, []string{"sos"}},
		{"roman", handleRoman, []string{"2025"}},
		{"calc", handleCalc, []string{"(2+3)*4^2"}},
		// pack 7 (local)
		{"binary", handleBinary, []string{"hi"}},
		{"hex", handleHex, []string{"hi"}},
		{"reverse", handleReverse, []string{"hello"}},
		{"wordcount", handleWordCount, []string{"hello", "world"}},
		{"urlencode", handleURLEncode, []string{"hello world"}},
		{"dice", handleDice, []string{"6", "3"}},
		{"random", handleRandom, []string{"1", "100"}},
		{"prime", handlePrime, []string{"97"}},
		{"fibonacci", handleFibonacci, []string{"10"}},
		{"gcd", handleGCD, []string{"24", "36"}},
		// pack 7 (live API)
		{"cve", handleCVE, []string{"3"}},
		{"airquality", handleAirQuality, []string{"33.68", "73.04"}},
		{"moon", handleMoon, []string{"33.68", "73.04"}},
		{"countryinfo", handleCountryInfo, []string{"Pakistan"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FAILED") ||
			strings.Contains(r, "UNAVAILABLE") || strings.Contains(r, "COULD NOT") ||
			strings.Contains(r, "NO RESULTS") || strings.Contains(r, "INVALID") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
