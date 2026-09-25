package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestEqualizerMenuRenders: buildEqualizerMenu must use the same fancy boxed
// format as the .font / .logo menus and list .EQ1 .. .EQ1000.
func TestEqualizerMenuRenders(t *testing.T) {
	m := buildEqualizerMenu("UMAR", "92X", "0H 5M", ".", 1)
	if !strings.Contains(m, "╔════ ≪ • 🔰 • ≫ ════╗") {
		t.Errorf(".equalizer menu me fancy box header nahi hai\n%s", m)
	}
	if !strings.Contains(m, "🔰 EQUALIZER 🔰") {
		t.Errorf(".equalizer menu header title EQUALIZER missing\n%s", m)
	}
	if !strings.Contains(m, ".EQ1 ❮") {
		t.Errorf(".equalizer menu me .EQ1 entry missing\n%s", m)
	}
	if !strings.Contains(m, ".EQ1000 ❮") {
		t.Errorf(".equalizer menu me .EQ1000 entry missing\n%s", m)
	}
	if !strings.Contains(m, "❮ 1000 ❯") {
		t.Errorf(".equalizer menu COMMANDS count 1000 nahi hai\n%s", m)
	}
}

// TestEqCommandsHidden: eq1..eq1000 must be registered so they dispatch, but
// hidden from .menu (owner order: only .EQUALIZER + the 5 named aliases show).
func TestEqCommandsHidden(t *testing.T) {
	for _, n := range []int{1, 2, 500, 999, 1000} {
		name := "eq" + itoaT(n)
		if _, ok := Commands[name]; !ok {
			t.Errorf("%s Commands map me registered nahi hai (dispatch nahi hoga)", name)
		}
		if !hiddenCommands[name] {
			t.Errorf("%s hiddenCommands me nahi hai (menu me dikh jayega)", name)
		}
	}
	if len(hiddenCommands) < goldcmds.EqCount {
		t.Errorf("hiddenCommands me kaafi eq entries nahi: %d", len(hiddenCommands))
	}
}

// TestEqualizerCategoryShortcut: .equalizer slug se EQUALIZER category menu
// khulna chahiye.
func TestEqualizerCategoryShortcut(t *testing.T) {
	cat, ok := menuCategoryFromCommand("equalizer")
	if !ok {
		t.Fatal(".equalizer category shortcut resolve nahi hua")
	}
	if cat != "EQUALIZER" {
		t.Errorf(".equalizer -> %q, want EQUALIZER", cat)
	}
}
