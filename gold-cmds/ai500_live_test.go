package goldcmds

import (
	"os"
	"strings"
	"testing"
)

// TestAINewBrandLive — live call for a NEW batch-2 brand (replika). Skipped
// unless GOLDMD_LIVE_AI=1. Proves new commands reply exactly like old ones.
func TestAINewBrandLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE_AI") != "1" {
		t.Skip("set GOLDMD_LIVE_AI=1 to run")
	}
	var b aiBrand
	for _, x := range aiBrandsExtra {
		if x.Cmd == "replika" {
			b = x
		}
	}
	if b.Cmd == "" {
		t.Fatal("replika not found in aiBrandsExtra")
	}
	sys := aiSystemPrompt(b, "", "")
	out, err := aiMistralChat(sys, "Who are you? Answer in one short line.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	t.Logf("NEW BRAND LIVE ANSWER: %s", out)
	if strings.TrimSpace(out) == "" {
		t.Fatal("empty answer")
	}
	if strings.Contains(out, "UMAR") || strings.Contains(out, "FAROOQ") {
		t.Fatalf("owner name leaked: %s", out)
	}
}
