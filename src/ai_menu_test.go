package main

import (
	"strings"
	"testing"
)

// TestAICategoryInMenu — .menu (category-list mode) must show the .AI entry,
// and .ai must resolve to the "AI" category.
func TestAICategoryInMenu(t *testing.T) {
	cat, ok := menuCategoryFromCommand("ai")
	if !ok || cat != "AI" {
		t.Fatalf("menuCategoryFromCommand(\"ai\") = (%q,%v), want (AI,true)", cat, ok)
	}
	// full name re-resolve (handler passes the full category name)
	if got, ok := menuCategoryFromCommand("AI"); !ok || got != "AI" {
		t.Fatalf("re-resolve menuCategoryFromCommand(\"AI\") = (%q,%v)", got, ok)
	}

	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "")
	if !strings.Contains(out, ".AI") {
		t.Fatalf("category-list menu missing .AI entry\n%s", out)
	}
}

// TestAISingleCategoryMenu — .ai shows the AI category banner + commands.
func TestAISingleCategoryMenu(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "AI")
	if !strings.Contains(out, "AI") {
		t.Fatalf("AI single-category menu missing banner\n%s", out)
	}
	if !strings.Contains(out, ".gpt") {
		t.Fatalf("AI menu missing .gpt\n%s", out)
	}
}
