package goldcmds

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewLanguagesLive hits the real translation endpoints for a sample of the
// languages added on top of the original list. It is opt-in (set
// GOLDMD_LIVE_TEST=1) because the normal suite must not depend on the network.
func TestNewLanguagesLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE_TEST") != "1" {
		t.Skip("set GOLDMD_LIVE_TEST=1 to run the live translation check")
	}
	cases := map[string]string{
		"pa-Arab": "Shahmukhi Punjabi (Pakistan)",
		"yue":     "Cantonese",
		"war":     "Waray",
		"ceb":     "Cebuano",
		"ilo":     "Iloko",
		"ab":      "Abkhaz",
		"ace":     "Acehnese",
		"aa":      "Afar",
		"bem":     "Bemba",
		"scn":     "Sicilian",
	}
	for code, label := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		out, _, err := trtTranslate(ctx, "Good morning my friend", code)
		cancel()
		if err != nil {
			t.Errorf("%s (%s): translate error: %v", code, label, err)
			continue
		}
		if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "Good morning my friend" {
			t.Errorf("%s (%s): got passthrough/empty %q", code, label, out)
			continue
		}
		t.Logf("%-8s %-30s -> %s", code, label, out)
	}
}
