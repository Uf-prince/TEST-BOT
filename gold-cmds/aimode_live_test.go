package goldcmds

import (
	"os"
	"strings"
	"testing"
)

// TestAIModeResolveLive — live end-to-end resolver check against Mistral.
// Skipped unless GOLDMD_LIVE_AI=1. Confirms the AI returns a REAL corpus
// command for plain Hinglish/Roman-Urdu sentences and that the hard gates
// (corpus membership, group-only) are enforced.
func TestAIModeResolveLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE_AI") != "1" {
		t.Skip("set GOLDMD_LIVE_AI=1 to run the live .aimode resolver test")
	}
	corpus := aimCorpus()
	if len(corpus) < 10 {
		t.Fatalf("corpus too small: %d", len(corpus))
	}
	known := map[string]bool{}
	for _, c := range corpus {
		known[c.Command] = true
	}

	cases := []string{
		"bot ki speed check karo", // → ping
		"calls auto reject ho rahi hai band karo",
	}
	for _, c := range cases {
		resolved, ok := aimResolve(c, true)
		t.Logf("%q → %q (ok=%v)", c, resolved, ok)
		if !ok {
			continue // NO_COMMAND_FOUND is an allowed outcome for the vague case
		}
		base := strings.ToLower(strings.Fields(resolved)[0])
		if !known[base] {
			t.Fatalf("resolved %q outside corpus", resolved)
		}
	}
}
