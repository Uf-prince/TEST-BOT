package goldcmds

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// funPackNames are the 12 visible commands added by funpack.go.
var funPackNames = []string{
	"qr", "weather", "wiki", "joke", "fact", "quote",
	"shorten", "crypto", "ip",
}

// TestFunPackRegistered verifies all 12 commands are registered, visible,
// have a category, and use the GOLD-MD desc style.
func TestFunPackRegistered(t *testing.T) {
	all := Commands()
	byName := map[string]Command{}
	for _, c := range all {
		byName[c.Name] = c
	}
	for _, name := range funPackNames {
		c, ok := byName[name]
		if !ok {
			t.Errorf("command %q not registered", name)
			continue
		}
		if c.Hidden {
			t.Errorf("command %q should be visible", name)
		}
		if c.Category == "" {
			t.Errorf("command %q has empty category", name)
		}
		if !strings.HasPrefix(c.Desc, "THIS COMMAND IS USED TO") {
			t.Errorf("command %q desc not in GOLD-MD style: %q", name, c.Desc)
		}
		if c.Run == nil {
			t.Errorf("command %q has nil Run", name)
		}
	}
}

// TestFunPackAliasesHidden verifies the hidden aliases exist and are hidden.
func TestFunPackAliasesHidden(t *testing.T) {
	all := Commands()
	byName := map[string]Command{}
	for _, c := range all {
		byName[c.Name] = c
	}
	for _, name := range []string{"qrcode", "wthr", "wikipedia", "tiny", "shorturl", "urltiny", "smalllink", "smallurl", "shortlink", "tinyurl", "ipinfo", "coin"} {
		c, ok := byName[name]
		if !ok {
			t.Errorf("alias %q not registered", name)
			continue
		}
		if !c.Hidden {
			t.Errorf("alias %q should be hidden", name)
		}
	}
}

// TestFunPackLive hits the real endpoints (opt-in via GOLDMD_LIVE=1).
func TestFunPackLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") != "1" {
		t.Skip("set GOLDMD_LIVE=1 to run live endpoint tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	checks := []struct {
		name string
		url  string
	}{
		{"qr", "https://api.qrserver.com/v1/create-qr-code/?size=512x512&margin=10&data=GOLD-MD"},
		{"weather", "https://wttr.in/Karachi?format=j1"},
		{"wiki", "https://en.wikipedia.org/api/rest_v1/page/summary/Pakistan"},
		{"joke", "https://official-joke-api.appspot.com/random_joke"},
		{"fact", "https://uselessfacts.jsph.pl/api/v2/facts/random?language=en"},
		{"quote", "https://zenquotes.io/api/random"},
		{"shorten", "https://tinyurl.com/api-create.php?url=https://github.com/Uf-prince/TEST-BOT"},
		{"crypto", "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,pkr"},
		{"ip", "http://ip-api.com/json/8.8.8.8"},
	}
	for _, c := range checks {
		data, err := funGetBytes(ctx, c.url)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s: empty body", c.name)
		}
		t.Logf("%s: OK (%d bytes)", c.name, len(data))
	}
}
