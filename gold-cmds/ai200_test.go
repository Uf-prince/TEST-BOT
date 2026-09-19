package goldcmds

import (
	"strings"
	"testing"
)

// TestAI200Count — exactly 500 AI brand commands + the .ai menu command.
func TestAI200Count(t *testing.T) {
	if len(aiBrands) != 500 {
		t.Fatalf("aiBrands = %d, want 500", len(aiBrands))
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

// TestAICommandsRegistered — every brand is registered under category "AI".
// NOTE: .ai must NOT be registered as a command — otherwise the dispatcher
// would match it as a command (plain text) instead of the category shortcut
// (fancy boxed menu). See src/ai_menu_test.go for the menu behaviour.
func TestAICommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	if _, ok := byName["ai"]; ok {
		t.Fatalf(".ai must NOT be registered as a command (it is a category shortcut)")
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

// TestAIGuidanceText — each command has its own guidance message.
// Owner order: personal name must NOT appear anywhere.
func TestAIGuidanceText(t *testing.T) {
	b := aiBrand{Cmd: "gpt", Name: "ChatGPT", Company: "OpenAI", Tagline: "The world's most popular AI assistant"}
	out := aiGuidanceText(b, ".")
	for _, want := range []string{"ChatGPT", "OpenAI", ".gpt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("guidance missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "UMAR") || strings.Contains(out, "FAROOQ") {
		t.Fatalf("guidance must NOT contain owner name\n%s", out)
	}
}

// TestAISystemPromptIdentity — the prompt tells the model it IS the brand and
// it must NOT claim to be Mistral. Owner order: no personal name at all.
func TestAISystemPromptIdentity(t *testing.T) {
	b := aiBrand{Cmd: "gemini", Name: "Gemini", Company: "Google", Tagline: "x"}
	p := aiSystemPrompt(b, "UMAR • FAROOQ", "923158930864")
	if !strings.Contains(p, "You are Gemini") {
		t.Fatalf("prompt missing identity\n%s", p)
	}
	if !strings.Contains(p, "You are NOT Mistral") {
		t.Fatalf("prompt missing anti-Mistral instruction\n%s", p)
	}
	if strings.Contains(p, "UMAR") || strings.Contains(p, "FAROOQ") {
		t.Fatalf("prompt must NOT contain owner name\n%s", p)
	}
	if strings.Contains(p, "923158930864") {
		t.Fatalf("prompt must NOT contain owner number\n%s", p)
	}
}

// TestAIWhatsAppFormat — Markdown emphasis must be converted to WhatsApp-native
// formatting so the ** stars hide (owner report + screenshot).
func TestAIWhatsAppFormat(t *testing.T) {
	cases := map[string]string{
		"I am **ChatGPT**, a model from OpenAI.": "I am *ChatGPT*, a model from OpenAI.",
		"**Bold** and *italic*.":                 "*Bold* and _italic_.",
		"~~strike~~":                             "~strike~",
		"`code`":                                 "```code```",
		"# Heading":                              "*Heading*",
		"[Google](https://google.com)":           "Google (https://google.com)",
		"plain text":                             "plain text",
	}
	for in, want := range cases {
		got := aiWhatsAppFormat(in)
		if got != want {
			t.Fatalf("aiWhatsAppFormat(%q) = %q, want %q", in, got, want)
		}
	}
	// No double-star must survive.
	if strings.Contains(aiWhatsAppFormat("**x**"), "**") {
		t.Fatal("double-star survived conversion")
	}
}
