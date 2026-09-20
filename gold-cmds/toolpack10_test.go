package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack10GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack10GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"agify":        agifyGuide("."),
		"genderize":    genderizeGuide("."),
		"nationalize":  nationalizeGuide("."),
		"maclookup":    maclookupGuide("."),
		"zipcode":      zipcodeGuide("."),
		"avatar":       avatarGuide("."),
		"jokeapi":      jokeapiGuide("."),
		"coingecko":    coingeckoGuide("."),
		"exchangerate": exchangerateGuide("."),
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

// TestToolpack10Live exercises each new command against its live API.
func TestToolpack10Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"agify", handleAgify, []string{"michael"}},
		{"genderize", handleGenderize, []string{"alex"}},
		{"nationalize", handleNationalize, []string{"nathaniel"}},
		{"maclookup", handleMaclookup, []string{"44:38:39:ff:ef:57"}},
		{"zipcode", handleZipcode, []string{"us", "33162"}},
		{"avatar", handleAvatar, []string{"goldmd"}},
		{"jokeapi", handleJokeapi, nil},
		{"coingecko", handleCoingecko, []string{"bitcoin"}},
		{"exchangerate", handleExchangerate, []string{"usd", "pkr"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") {
			status = "FAIL"
		}
		t.Logf("%-12s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
