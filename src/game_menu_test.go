package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestGameCategoryInMenu: plain .menu me .GAME line exactly ek baar dikhni
// chahiye.
func TestGameCategoryInMenu(t *testing.T) {
	m := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, "")
	if !strings.Contains(m, ".GAME") {
		t.Errorf("plain .menu me .GAME nazar nahi aata\n%s", m)
	}
	if got := strings.Count(m, ".GAME"); got != 1 {
		t.Errorf("plain .menu me .GAME %d bar hai, 1 hona chahiye\n%s", got, m)
	}
}

// TestGameCategoryShortcut: .game slug se GAME category menu khulna chahiye.
func TestGameCategoryShortcut(t *testing.T) {
	cat, ok := menuCategoryFromCommand("game")
	if !ok {
		t.Fatal(".game category shortcut resolve nahi hua")
	}
	if cat != "GAME" {
		t.Errorf(".game -> %q, want GAME", cat)
	}
}

// TestGameCategoryMenuListsGames: GAME category menu me saare game commands
// dikhne chahiye.
func TestGameCategoryMenuListsGames(t *testing.T) {
	m := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, "GAME")
	for _, name := range []string{"ROLLDICE", "SLOT", "RPS", "SCRAMBLE", "BLACKJACK", "BALL"} {
		if !strings.Contains(m, name) {
			t.Errorf("GAME menu me %s missing\n%s", name, m)
		}
	}
	if !strings.Contains(m, "🎮") {
		t.Errorf("GAME menu me 🎮 emoji missing\n%s", m)
	}
	_ = goldcmds.CategoryOrder
}
