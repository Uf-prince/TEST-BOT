package main

import (
	"strings"
	"testing"
)

// TestShortenAliasesInFullMenu verifies .shorten shows all hidden aliases.
func TestShortenAliasesInFullMenu(t *testing.T) {
	entries := buildFullMenuEntries()
	var found bool
	for _, e := range entries {
		if e.Name == "shorten" {
			found = true
			joined := strings.Join(e.Aliases, " | ")
			for _, want := range []string{"tiny", "shorturl", "urltiny", "smalllink", "smallurl", "shortlink", "tinyurl"} {
				if !strings.Contains(joined, want) {
					t.Errorf("shorten aliases missing %q (got: %s)", want, joined)
				}
			}
			t.Logf("shorten aliases: %s", joined)
		}
		if e.Name == "cat" || e.Name == "dog" {
			t.Errorf("removed command %q still in fullmenu", e.Name)
		}
	}
	if !found {
		t.Fatal("shorten not found in fullmenu entries")
	}
}
