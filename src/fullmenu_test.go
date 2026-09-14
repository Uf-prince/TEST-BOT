package main

import (
	"fmt"
	"testing"
)

// TestFullMenuFormat verifies the .fullmenu output: format per owner spec,
// exclusions, and WhatsApp-safe length.
func TestFullMenuFormat(t *testing.T) {
	parts := buildFullMenuText(".", "GOLD-MD WHATSAPP BOT", "UMAR", "92XXXXXXXXXX", "0H 5M", 1)
	if len(parts) == 0 {
		t.Fatal("no parts generated")
	}
	full := ""
	for _, p := range parts {
		full += p
	}
	fmt.Printf("PARTS: %d  TOTAL CHARS: %d\n", len(parts), len(full))

	// Exclusions — owner order
	for _, banned := range []string{"svrchange", "host5gb"} {
		if fmt.Sprintf("COMMAND \u276e%s\u276f", banned) == "" {
			continue
		}
		// search for the entry block header
		for _, p := range parts {
			if contains(p, fmt.Sprintf("\u276e%s\u276f", banned)) {
				t.Errorf("BANNED command %q found in fullmenu", banned)
			}
		}
	}

	// .m alias must be listed under menu
	foundMenuAlias := false
	for _, p := range parts {
		if contains(p, "COMMAND \u276emenu\u276f") && contains(p, "ALIASES HIDDEN WORK") && contains(p, "m |") {
			foundMenuAlias = true
		}
	}
	if !foundMenuAlias {
		t.Error("menu entry with .m alias not found")
	}

	// antilink subcommands present
	foundSub := false
	for _, p := range parts {
		if contains(p, "COMMAND \u276eantilink action\u276f") {
			foundSub = true
		}
	}
	if !foundSub {
		t.Error("antilink action subcommand missing")
	}

	// WhatsApp limit: each part must stay under 60k
	for i, p := range parts {
		if len(p) > 60000 {
			t.Errorf("part %d too long: %d chars", i, len(p))
		}
	}
	fmt.Printf("FIRST 1200 CHARS:\n%s\n", parts[0][:1200])
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
