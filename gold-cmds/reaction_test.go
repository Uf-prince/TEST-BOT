package goldcmds

import (
	"strings"
	"testing"
)

// TestReactionCatalog verifies the reaction catalog has exactly 500 unique
// English reaction names.
func TestReactionCatalog(t *testing.T) {
	if len(reactionNames) != 500 {
		t.Fatalf("expected 500 reactions, got %d", len(reactionNames))
	}
	seen := map[string]bool{}
	for _, n := range reactionNames {
		if n == "" {
			t.Fatalf("reaction with empty name")
		}
		if seen[n] {
			t.Fatalf("duplicate reaction name %q", n)
		}
		seen[n] = true
	}
}

// TestReactionCategoryCounts verifies each reaction category has exactly 500
// registered commands (owner requirement: 500 / 500).
func TestReactionCategoryCounts(t *testing.T) {
	bCount, gCount := 0, 0
	for _, c := range Commands() {
		switch c.Category {
		case "BREACTION":
			bCount++
		case "GREACTION":
			gCount++
		}
	}
	if bCount != 500 {
		t.Fatalf("BREACTION has %d commands, want 500", bCount)
	}
	if gCount != 500 {
		t.Fatalf("GREACTION has %d commands, want 500", gCount)
	}
}

// TestReactionCommandsRegistered verifies .b<name> and .g<name> are registered
// for every reaction, in the BREACTION / GREACTION categories.
func TestReactionCommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, n := range reactionNames {
		bc, ok := byName["b"+n]
		if !ok {
			t.Fatalf("missing command .b%s", n)
		}
		if bc.Category != "BREACTION" {
			t.Fatalf(".b%s category = %q, want BREACTION", n, bc.Category)
		}
		gc, ok := byName["g"+n]
		if !ok {
			t.Fatalf("missing command .g%s", n)
		}
		if gc.Category != "GREACTION" {
			t.Fatalf(".g%s category = %q, want GREACTION", n, gc.Category)
		}
	}
}

// TestReactionEmoji checks emoji resolution returns something for every name.
func TestReactionEmoji(t *testing.T) {
	for _, n := range reactionNames {
		if strings.TrimSpace(reactionEmoji(n)) == "" {
			t.Fatalf("reactionEmoji(%q) empty", n)
		}
	}
}
