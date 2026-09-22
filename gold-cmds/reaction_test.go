package goldcmds

import (
	"strings"
	"testing"
)

// TestReactionCatalog verifies the reaction catalog is non-empty and that
// every entry has at least one provider source.
func TestReactionCatalog(t *testing.T) {
	if len(reactionDefs) < 100 {
		t.Fatalf("expected >=100 reactions, got %d", len(reactionDefs))
	}
	for _, d := range reactionDefs {
		if d.Name == "" {
			t.Fatalf("reaction with empty name")
		}
		if d.Gifukai == "" && d.Otaku == "" && d.Purr == "" && d.Neko == "" {
			t.Fatalf("reaction %q has no provider source", d.Name)
		}
	}
}

// TestReactionCommandsRegistered verifies .b<name> and .g<name> are registered
// for every reaction, in the BREACTION / GREACTION categories.
func TestReactionCommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	bCount, gCount := 0, 0
	for _, d := range reactionDefs {
		bc, ok := byName["b"+d.Name]
		if !ok {
			t.Fatalf("missing command .b%s", d.Name)
		}
		if bc.Category != "BREACTION" {
			t.Fatalf(".b%s category = %q, want BREACTION", d.Name, bc.Category)
		}
		gc, ok := byName["g"+d.Name]
		if !ok {
			t.Fatalf("missing command .g%s", d.Name)
		}
		if gc.Category != "GREACTION" {
			t.Fatalf(".g%s category = %q, want GREACTION", d.Name, gc.Category)
		}
		bCount++
		gCount++
	}
	if bCount != len(reactionDefs) || gCount != len(reactionDefs) {
		t.Fatalf("counts b=%d g=%d want %d each", bCount, gCount, len(reactionDefs))
	}
}

// TestReactionFind checks name resolution (case-insensitive).
func TestReactionFind(t *testing.T) {
	for _, name := range []string{"happy", "sad", "angry", "smile", "cry"} {
		if _, ok := reactionFind(name); !ok {
			t.Fatalf("reactionFind(%q) not found", name)
		}
		if _, ok := reactionFind(strings.ToUpper(name)); !ok {
			t.Fatalf("reactionFind(%q) not found (uppercase)", name)
		}
	}
	if _, ok := reactionFind("definitely_not_a_reaction"); ok {
		t.Fatalf("reactionFind returned a bogus reaction")
	}
}
