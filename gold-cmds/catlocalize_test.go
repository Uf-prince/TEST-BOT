package goldcmds

import "testing"

// TestCanonicalNamesIncludeMenuTokens proves the .menu category shortcuts are
// part of the localized command-name set, so each category name gets translated
// and its localized form resolves back to the category (owner report: the .menu
// category list stayed English in every language).
func TestCanonicalNamesIncludeMenuTokens(t *testing.T) {
	old := clMenuTokenHook
	clMenuTokenHook = func() []string { return []string{"core", "group", "protection", "downloader"} }
	defer func() { clMenuTokenHook = old }()

	set := map[string]bool{}
	for _, n := range clCanonicalNames() {
		set[n] = true
	}
	for _, want := range []string{"core", "group", "protection", "downloader"} {
		if !set[want] {
			t.Errorf("clCanonicalNames() missing menu token %q", want)
		}
	}
}

// TestClVersionBumpForcesRebuild proves a stored map written by an older build
// (no version, or a different one) is recognised as stale so it is rebuilt once
// to pick up the category names.
func TestClVersionBumpForcesRebuild(t *testing.T) {
	// Legacy map (no version prefix): Ver is empty, so it must NOT match.
	legacy := clParse("ur:مینو=menu")
	if legacy.Lang != "ur" {
		t.Fatalf("legacy Lang = %q, want ur", legacy.Lang)
	}
	if legacy.Ver == clVersion {
		t.Fatalf("legacy map must not carry the current version")
	}
	// Fresh map: version matches.
	fresh := clParse(clEncode("ur", [][2]string{{"بنیادی", "core"}}))
	if fresh.Ver != clVersion || fresh.Lang != "ur" {
		t.Fatalf("fresh map Ver=%q Lang=%q, want %q/ur", fresh.Ver, fresh.Lang, clVersion)
	}
	if fresh.ByLoc["بنیادی"] != "core" {
		t.Fatalf("fresh map lost the category row: %v", fresh.ByLoc)
	}
}
