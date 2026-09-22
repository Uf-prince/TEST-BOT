package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack14GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack14GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"qrgen":           qrgenGuide("."),
		"barcode":         barcodeGuide("."),
		"urlshort":        urlshortGuide("."),
		"goldprice":       goldpriceGuide("."),
		"silverprice":     silverpriceGuide("."),
		"currencyconvert": currencyconvertGuide("."),
		"randomuser":      randomuserGuide("."),
		"synwords":        synwordsGuide("."),
		"antonym":         antonymGuide("."),
		"wordassoc":       wordassocGuide("."),
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

// TestToolpack14Live exercises each new command against its live API.
func TestToolpack14Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"qrgen", handleQrgen, []string{"hello", "world"}},
		{"barcode", handleBarcode, []string{"123456789"}},
		{"urlshort", handleUrlshort, []string{"https://example.com/very/long/path"}},
		{"goldprice", handleGoldprice, nil},
		{"silverprice", handleSilverprice, nil},
		{"currencyconvert", handleCurrencyconvert, []string{"100", "USD", "PKR"}},
		{"randomuser", handleRandomuser, nil},
		{"synwords", handleSynwords, []string{"happy"}},
		{"antonym", handleAntonym, []string{"happy"}},
		{"wordassoc", handleWordassoc, []string{"ocean"}},
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
		time.Sleep(300 * time.Millisecond)
	}
}
