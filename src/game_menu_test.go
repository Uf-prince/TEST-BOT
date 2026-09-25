package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestGameMenuRenders: buildGameMenu must use the same fancy boxed format as
// .font / .equalizer and list ALL 1000 games by their SHORT command name
// (converter-menu style: only the exact thing the user types) — no numeric
// game1..game1000 label and no long "CLASSIC ..." design name (owner order).
func TestGameMenuRenders(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, menuStyleFor(nil, ""))
	if !strings.Contains(m, "╔════ ≪ • 🔰 • ≫ ════╗") {
		t.Errorf(".game menu me fancy box header nahi hai\n%s", m)
	}
	if !strings.Contains(m, "🔰 GAME 🔰") {
		t.Errorf(".game menu header title GAME missing\n%s", m)
	}
	// Short command names, exactly as typed. flavor 0 = bare slug,
	// flavor k = slug + 2-digit edition.
	for _, slug := range []string{".ROLLDICE", ".SLOT", ".PICKNAME", ".ROLLDICE02", ".PICKNAME20"} {
		if !strings.Contains(m, slug) {
			t.Errorf(".game menu me short command name %s missing\n%s", slug, m)
		}
	}
	// The numeric label MUST be gone.
	if strings.Contains(m, "game1") || strings.Contains(m, "GAME1000") {
		t.Errorf(".game menu me game<number> label nahi hona chahiye (owner order)\n%s", m)
	}
	// The long design names MUST be gone.
	if strings.Contains(m, "CLASSIC") || strings.Contains(m, "ULTRA NAME PICKER") {
		t.Errorf(".game menu me lamba design name nahi hona chahiye (owner order)\n%s", m)
	}
	// Every row is a short command name — exactly one token after the prefix.
	for _, line := range strings.Split(m, "\n") {
		if !strings.HasPrefix(line, "*| 🔰 | .") || !strings.HasSuffix(line, "*") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(line, "*| 🔰 | ."), "*")
		if strings.ContainsAny(name, " |") {
			t.Errorf("game row ek hi command naam hona chahiye: %q", line)
		}
	}
	if strings.Contains(m, "MENUS:") {
		t.Errorf(".game menu me MENUS line nahi honi chahiye\n%s", m)
	}
	if !strings.Contains(m, "❮ 1000 ❯") {
		t.Errorf(".game menu COMMANDS count 1000 nahi hai\n%s", m)
	}
	// One header row + one row per game (all 1000).
	want := goldcmds.GameMenuCount() + 1
	if got := strings.Count(m, "*| 🔰 |"); got != want {
		t.Errorf(".game menu me %d rows, want %d (header + %d games)", got, want, want-1)
	}
}

// TestGameMenuNotPlainText: the .game reply must be the boxed menu, NOT a
// plain "GAME MENU" text blob (the earlier regression).
func TestGameMenuNotPlainText(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, menuStyleFor(nil, ""))
	if strings.HasPrefix(strings.TrimSpace(m), "*GAME MENU*") {
		t.Error(".game abhi bhi plain text list de raha hai")
	}
	if !strings.Contains(m, "╔════") || !strings.Contains(m, "╚════") {
		t.Error(".game menu boxed format me nahi hai")
	}
}

// TestGameShortSlugsAreUniqueAndDispatch: all 1000 short names must be unique,
// map to game 1..1000 in order, and every one must actually dispatch.
func TestGameShortSlugsAreUniqueAndDispatch(t *testing.T) {
	slugs := goldcmds.GameShortSlugs()
	if len(slugs) != 1000 || goldcmds.GameMenuCount() != 1000 {
		t.Fatalf("GameShortSlugs = %d, GameMenuCount = %d, want 1000", len(slugs), goldcmds.GameMenuCount())
	}
	seen := map[string]bool{}
	for i, slug := range slugs {
		if seen[slug] {
			t.Errorf("slug %q menu me do bar", slug)
		}
		seen[slug] = true
		if _, ok := Commands[slug]; !ok {
			t.Errorf("menu slug %q dispatch nahi hota (Commands map me nahi)", slug)
		}
		// The registered handle must run the SAME game index as the menu order.
		if got := goldcmds.GameShortSlug(i + 1); got != slug {
			t.Errorf("GameShortSlug(%d) = %q, want %q", i+1, got, slug)
		}
	}
	for _, n := range []int{1, 51, 500, 1000} {
		slug := goldcmds.GameShortSlug(n)
		if _, ok := Commands[slug]; !ok {
			t.Errorf("game %d ka slug %q dispatch nahi hota", n, slug)
		}
	}
}

// TestGameCommandsHidden: game1..game1000 must still dispatch but stay hidden
// (used by the classic .gameN plays) — they are simply no longer listed.
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
	m := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, "", menuStyleFor(nil, ""))
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

// TestGameMenuFitsWhatsApp: all 1000 named entries must stay under the
// WhatsApp message cap (65536 bytes) or the menu would fail to send.
func TestGameMenuFitsWhatsApp(t *testing.T) {
	m := buildGameMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, menuStyleFor(nil, ""))
	if len(m) > 65536 {
		t.Errorf(".game menu %d bytes — WhatsApp cap 65536 se bada", len(m))
	}
}
