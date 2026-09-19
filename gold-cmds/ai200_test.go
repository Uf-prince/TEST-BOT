package goldcmds

import (
	"strings"
	"testing"
)

// TestAI200Count — exactly 200 AI brand commands + the .ai menu command.
func TestAI200Count(t *testing.T) {
	if len(aiBrands) != 200 {
		t.Fatalf("aiBrands = %d, want 200", len(aiBrands))
	}
	seen := map[string]bool{}
	for _, b := range aiBrands {
		if b.Cmd == "" || b.Name == "" || b.Company == "" {
			t.Fatalf("brand with empty field: %+v", b)
		}
		if seen[b.Cmd] {
			t.Fatalf("duplicate command name %q", b.Cmd)
		}
		seen[b.Cmd] = true
	}
}

// TestAICommandsRegistered — every brand is registered under category "AI",
// and the .ai menu command exists.
func TestAICommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	if _, ok := byName["ai"]; !ok {
		t.Fatalf(".ai command not registered")
	}
	if byName["ai"].Category != "AI" {
		t.Fatalf(".ai category = %q, want AI", byName["ai"].Category)
	}
	for _, b := range aiBrands {
		c, ok := byName[b.Cmd]
		if !ok {
			t.Fatalf("brand command %q not registered", b.Cmd)
		}
		if c.Category != "AI" {
			t.Fatalf("%q category = %q, want AI", b.Cmd, c.Category)
		}
		if c.Run == nil {
			t.Fatalf("%q has nil Run", b.Cmd)
		}
	}
}

// TestAIMenuText — the .ai menu lists all 200 names with the prefix.
func TestAIMenuText(t *testing.T) {
	out := aiMenuText(".")
	if !strings.Contains(out, "AI COMMANDS") {
		t.Fatalf("menu missing header\n%s", out)
	}
	if !strings.Contains(out, "TOTAL ❮ 200 ❯") {
		t.Fatalf("menu missing total 200\n%s", out)
	}
	for _, b := range aiBrands {
		if !strings.Contains(out, "."+b.Cmd) {
			t.Fatalf("menu missing command .%s", b.Cmd)
		}
	}
}

// TestAIGuidanceText — each command has its own guidance message.
func TestAIGuidanceText(t *testing.T) {
	b := aiBrand{Cmd: "gpt", Name: "ChatGPT", Company: "OpenAI", Tagline: "The world's most popular AI assistant"}
	out := aiGuidanceText(b, ".")
	for _, want := range []string{"ChatGPT", "OpenAI", ".gpt", "UMAR • FAROOQ"} {
		if !strings.Contains(out, want) {
			t.Fatalf("guidance missing %q\n%s", want, out)
		}
	}
}

// TestAISystemPromptIdentity — the prompt tells the model it IS the brand and
// its owner is UMAR • FAROOQ, and it must NOT claim to be Mistral.
func TestAISystemPromptIdentity(t *testing.T) {
	b := aiBrand{Cmd: "gemini", Name: "Gemini", Company: "Google", Tagline: "x"}
	p := aiSystemPrompt(b, "UMAR • FAROOQ", "923158930864")
	if !strings.Contains(p, "You are Gemini") {
		t.Fatalf("prompt missing identity\n%s", p)
	}
	if !strings.Contains(p, "You are NOT Mistral") {
		t.Fatalf("prompt missing anti-Mistral instruction\n%s", p)
	}
	if !strings.Contains(p, "UMAR • FAROOQ") {
		t.Fatalf("prompt missing owner\n%s", p)
	}
	if !strings.Contains(p, "923158930864") {
		t.Fatalf("prompt missing owner number\n%s", p)
	}
}
