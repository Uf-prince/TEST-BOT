package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestMenuCategorySlugsUniqueAndSafe verifies every category slug is unique
// and does not collide with a real registered command name.
func TestMenuCategorySlugsUniqueAndSafe(t *testing.T) {
	seen := map[string]string{}
	for cat, slug := range menuCategorySlugs {
		if slug == "" {
			t.Fatalf("category %q has empty slug", cat)
		}
		if prev, ok := seen[slug]; ok {
			t.Fatalf("slug %q used by both %q and %q", slug, prev, cat)
		}
		seen[slug] = cat
	}
	// "owner" and "system" are real commands -> must NOT be used as slugs.
	if _, bad := seen["owner"]; bad {
		t.Fatalf("slug 'owner' collides with real command")
	}
	if _, bad := seen["system"]; bad {
		t.Fatalf("slug 'system' collides with real command")
	}
}

// TestMenuCategoryFromCommand checks slug + normalized-name resolution.
func TestMenuCategoryFromCommand(t *testing.T) {
	cases := map[string]string{
		"core":              "OWNER & SYSTEM",
		"group":             "GROUP MANAGEMENT",
		"groupmanagement":   "GROUP MANAGEMENT",
		"protection":        "ANTI & PROTECTION",
		"antiprotection":    "ANTI & PROTECTION",
		"downloader":        "DOWNLOADER",
		"ai":                "AI & MEDIA",
		"aimedia":           "AI & MEDIA",
		"presence":          "PRESENCE & STATUS",
		"presencestatus":    "PRESENCE & STATUS",
		"converter":         "CONVERTER",
		"tools":             "TOOLS",
		"other":             "OTHER",
	}
	for in, want := range cases {
		got, ok := menuCategoryFromCommand(in)
		if !ok || got != want {
			t.Fatalf("menuCategoryFromCommand(%q) = (%q,%v), want (%q,true)", in, got, ok, want)
		}
	}
	// REGRESSION: handler.go passes the FULL category name (e.g. "GROUP
	// MANAGEMENT") to CmdMenu, which re-resolves it. Must still resolve.
	for _, full := range []string{"GROUP MANAGEMENT", "ANTI & PROTECTION", "OWNER & SYSTEM", "AI & MEDIA", "PRESENCE & STATUS", "TOOLS", "DOWNLOADER", "CONVERTER", "OTHER"} {
		got, ok := menuCategoryFromCommand(full)
		if !ok || got != full {
			t.Fatalf("re-resolve menuCategoryFromCommand(%q) = (%q,%v), want (%q,true)", full, got, ok, full)
		}
	}
	// Non-category inputs must NOT resolve.
	for _, in := range []string{"ping", "menu", "owner", "system", "xyz", ""} {
		if got, ok := menuCategoryFromCommand(in); ok {
			t.Fatalf("menuCategoryFromCommand(%q) unexpectedly = %q", in, got)
		}
	}
}

// TestBuildCategoryMenuCategoryListMode verifies .menu (no arg) shows ONLY
// category names (slugs), not individual commands.
func TestBuildCategoryMenuCategoryListMode(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "")
	for _, slug := range []string{"CORE", "GROUP", "PROTECTION", "DOWNLOADER", "AI", "PRESENCE", "CONVERTER", "TOOLS", "OTHER"} {
		if !strings.Contains(out, "."+slug) {
			t.Fatalf("category-list menu missing slug .%s\n%s", slug, out)
		}
	}
	// A known command name should NOT appear as a command line in list mode.
	if strings.Contains(out, ".ping") {
		t.Fatalf("category-list menu should not list individual commands (.ping found)")
	}
}

// TestBuildCategoryMenuSingleCategory verifies .group shows ONLY that
// category's commands, in the same banner format.
func TestBuildCategoryMenuSingleCategory(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "GROUP MANAGEMENT")
	if !strings.Contains(out, "GROUP MANAGEMENT") {
		t.Fatalf("single-category menu missing its banner\n%s", out)
	}
	// Should contain at least one group command.
	hasGroupCmd := false
	for _, c := range goldcmds.Commands() {
		if c.Category == "GROUP MANAGEMENT" && !c.Hidden {
			if strings.Contains(out, "."+c.Name) {
				hasGroupCmd = true
				break
			}
		}
	}
	if !hasGroupCmd {
		t.Fatalf("single-category menu has no group commands\n%s", out)
	}
	// Should NOT contain a command from a different category (e.g. a TOOLS cmd).
	for _, c := range goldcmds.Commands() {
		if c.Category == "TOOLS" && !c.Hidden {
			if strings.Contains(out, "."+c.Name+"*") {
				t.Fatalf("single-category menu leaked a TOOLS command .%s", c.Name)
			}
		}
	}
}
