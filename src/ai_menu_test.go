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

	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "", menuStyleFor(nil, ""))
	if !strings.Contains(out, ".AI") {
		t.Fatalf("category-list menu missing .AI entry\n%s", out)
	}
}

// TestAISingleCategoryMenu — .ai shows the AI category banner + commands,
// in the SAME fancy boxed format as every other category.
func TestAISingleCategoryMenu(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "AI", menuStyleFor(nil, ""))
	if !strings.Contains(out, "AI") {
		t.Fatalf("AI single-category menu missing banner\n%s", out)
	}
	if !strings.Contains(out, ".GPT") {
		t.Fatalf("AI menu missing .GPT (CAPS)\n%s", out)
	}
	// Must be the fancy boxed menu (same as other categories), not plain text.
	if !strings.Contains(out, "╔════ ≪ • 🔰 • ≫ ════╗") {
		t.Fatalf("AI menu missing fancy box header\n%s", out)
	}
	// OWNER ORDER: category menu header shows the CATEGORY name, not "MENU".
	if !strings.Contains(out, "🔰 AI 🔰") {
		t.Fatalf("AI menu header missing category title\n%s", out)
	}
	// MENUS count line must NOT appear in a category menu (only plain .menu).
	if strings.Contains(out, "MENUS:") {
		t.Fatalf("AI category menu must not show MENUS line\n%s", out)
	}
}

// TestAINotACommand — .ai must NOT be a registered command, otherwise the
// dispatcher would reply with plain text instead of the category menu.
func TestAINotACommand(t *testing.T) {
	if _, ok := Commands["ai"]; ok {
		t.Fatalf(".ai is registered as a command — it must be a category shortcut only")
	}
}
