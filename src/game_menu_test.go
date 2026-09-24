package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestGameMenuRenders: buildGameMenu must use the same fancy boxed format as
// .font / .equalizer and list .GAME1 .. .GAME1000 with design names.
func TestGameMenuRenders(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil)
	if !strings.Contains(m, "╔════ ≪ •❈• ≫ ════╗") {
		t.Errorf(".game menu me fancy box header nahi hai\n%s", m)
	}
	if !strings.Contains(m, "🔰 GAME 🔰") {
		t.Errorf(".game menu header title GAME missing\n%s", m)
	}
	if !strings.Contains(m, ".GAME1 ❮") {
		t.Errorf(".game menu me .GAME1 entry missing\n%s", m)
	}
	if !strings.Contains(m, ".GAME1000 ❮") {
		t.Errorf(".game menu me .GAME1000 entry missing\n%s", m)
	}
	if strings.Contains(m, "MENUS:") {
		t.Errorf(".game menu me MENUS line nahi honi chahiye\n%s", m)
	}
	if !strings.Contains(m, "❮ 1000 ❯") {
		t.Errorf(".game menu COMMANDS count 1000 nahi hai\n%s", m)
	}
	// The 1000 design labels must make the list actually useful.
	if strings.Count(m, "GAME") < 1000 {
		t.Errorf(".game menu me 1000 se kam GAME entries hain")
	}
}

// TestGameMenuNotPlainText: the .game reply must be the boxed menu, NOT a
// plain "GAME MENU" text blob (the earlier regression).
func TestGameMenuNotPlainText(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil)
	if strings.HasPrefix(strings.TrimSpace(m), "*GAME MENU*") {
		t.Error(".game abhi bhi plain text list de raha hai")
	}
	if !strings.Contains(m, "╔════") || !strings.Contains(m, "╚════") {
		t.Error(".game menu boxed format me nahi hai")
	}
}

// TestGameCommandsHidden: game1..game1000 must dispatch but stay hidden.
func TestGameCommandsHidden(t *testing.T) {
	for _, n := range []int{1, 2, 500, 999, 1000} {
		name := "game" + itoaT(n)
		if _, ok := Commands[name]; !ok {
			t.Errorf("%s Commands map me registered nahi hai (dispatch nahi hoga)", name)
		}
		if !hiddenCommands[name] {
			t.Errorf("%s hiddenCommands me nahi hai (menu me dikh jayega)", name)
		}
	}
}

// TestGameCategoryInMenu: plain .menu me .GAME line exactly ek baar.
func TestGameCategoryInMenu(t *testing.T) {
	m := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, "")
	if !strings.Contains(m, ".GAME") {
		t.Errorf("plain .menu me .GAME nazar nahi aata\n%s", m)
	}
	if got := strings.Count(m, ".GAME"); got != 1 {
		t.Errorf("plain .menu me .GAME %d bar hai, 1 hona chahiye\n%s", got, m)
	}
}

// TestGameCategoryShortcut: .game slug se GAME category menu khule.
func TestGameCategoryShortcut(t *testing.T) {
	cat, ok := menuCategoryFromCommand("game")
	if !ok {
		t.Fatal(".game category shortcut resolve nahi hua")
	}
	if cat != "GAME" {
		t.Errorf("category = %q, want GAME", cat)
	}
}

// TestGameBaseSlugDispatch: base slugs that do not collide with existing
// commands must be registered (hidden) and dispatch through GameRunN.
func TestGameBaseSlugDispatch(t *testing.T) {
	for _, slug := range []string{"rolldice", "slot", "poker", "roulette", "bingo"} {
		if _, ok := Commands[slug]; !ok {
			t.Errorf("base slug %s registered nahi hai", slug)
		}
		if !hiddenCommands[slug] {
			t.Errorf("base slug %s hidden nahi hai", slug)
		}
	}
	if goldcmds.GameCount != 1000 {
		t.Errorf("GameCount = %d, want 1000", goldcmds.GameCount)
	}
}

// TestGameMenuFitsWhatsApp: 1000 named entries must stay under the WhatsApp
// message cap (65536 bytes) or the menu would fail to send.
func TestGameMenuFitsWhatsApp(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil)
	if len(m) > 65536 {
		t.Errorf(".game menu %d bytes — WhatsApp cap 65536 se bada", len(m))
	}
}
