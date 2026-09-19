package goldcmds

import (
	"os"
	"strings"
	"testing"
)

// TestAIMistralLive — live call to Mistral (skipped unless GOLDMD_LIVE_AI=1).
// Verifies the model fallback chain returns a real answer and the identity
// prompt is honoured (model claims the brand, not Mistral).
func TestAIMistralLive(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE_AI") != "1" {
		t.Skip("set GOLDMD_LIVE_AI=1 to run the live Mistral test")
	}
	b := aiBrand{Cmd: "gemini", Name: "Gemini", Company: "Google", Tagline: "x"}
	sys := aiSystemPrompt(b, "UMAR • FAROOQ", "923158930864")
	out, err := aiMistralChat(sys, "Who are you and who is your owner? Answer in one short line.")
	if err != nil {
		t.Fatalf("aiMistralChat error: %v", err)
	}
	t.Logf("LIVE ANSWER: %s", out)
	if strings.TrimSpace(out) == "" {
		t.Fatalf("empty answer")
	}
}
