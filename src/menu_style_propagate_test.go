package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// A non-classic style must repaint EVERY part of a menu — header frame, list
// border, list rows and the IMPORTANT CMNDS block — not just the top header.
// This guards the reported bug where only the header changed.
func TestMenuStylePropagatesToWholeMenu(t *testing.T) {
	classic := goldcmds.MenuStyleAt(1)
	styled := goldcmds.MenuStyleAt(2)

	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "AI", styled)

	if !strings.Contains(out, styled.BorderTop()) {
		t.Errorf("styled list border missing from menu\n%s", out)
	}
	if strings.Contains(out, strings.Trim(classic.BorderTop(), "*")) {
		t.Errorf("classic list border leaked into a styled menu\n%s", out)
	}
	if !strings.Contains(out, styled.RenderHeaderTop("AI")) {
		t.Errorf("styled header frame missing\n%s", out)
	}
	if !strings.Contains(out, styled.ImpTitle("IMPORTANT CMNDS")) {
		t.Errorf("styled IMPORTANT CMNDS title missing\n%s", out)
	}
	// Rows are rendered in the style's decorative font (owner order: fancy
	// everywhere), but the token must still normalise back to a plain,
	// dispatcheable ASCII command.
	if norm := goldcmds.SkinNormalizeInput(out); !strings.Contains(norm, ".AIMENUPIC") {
		t.Errorf("command name no longer normalises back to ASCII\n%s", out)
	}
}

// The same style must also drive the per-menu renderers (.logo/.font/.game/.eq).
func TestMenuStylePropagatesToBespokeMenus(t *testing.T) {
	styled := goldcmds.MenuStyleAt(7)
	menus := map[string]string{
		"logo":      buildLogoMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, styled),
		"font":      buildFontMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, styled),
		"game":      buildGameMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, styled),
		"equalizer": buildEqualizerMenu("92300", "92301", "1H 2M", ".", 0, styled),
	}
	for key, out := range menus {
		if !strings.Contains(out, styled.BorderTop()) {
			t.Errorf("%s menu did not pick up the styled border", key)
		}
		if !strings.Contains(out, styled.ImpTitle("IMPORTANT CMNDS")) {
			t.Errorf("%s menu did not pick up the styled IMPORTANT CMNDS title", key)
		}
	}
}

// ResolveMenuStyle must fall back to the classic style for a nil bridge
// instead of rendering a borderless menu.
func TestResolveMenuStyleFallsBackToClassic(t *testing.T) {
	if got := goldcmds.ResolveMenuStyle(nil, "game"); got.N != 1 {
		t.Fatalf("nil bridge: want style 1, got %d", got.N)
	}
}
