package main

import (
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestFontMenuRenders: buildFontMenu must use the same fancy boxed format as
// the .logo menu and list .FONT1 .. .FONT1000.
func TestFontMenuRenders(t *testing.T) {
	m := buildFontMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil)
	if !strings.Contains(m, "╔════ ≪ •❈• ≫ ════╗") {
		t.Errorf(".font menu me fancy box header nahi hai\n%s", m)
	}
	if !strings.Contains(m, "🔰 FONT 🔰") {
		t.Errorf(".font menu header title FONT missing\n%s", m)
	}
	if !strings.Contains(m, ".FONT1 ❮ YOUR NAME ❯") {
		t.Errorf(".font menu me .FONT1 entry missing\n%s", m)
	}
	if !strings.Contains(m, ".FONT1000 ❮ YOUR NAME ❯") {
		t.Errorf(".font menu me .FONT1000 entry missing\n%s", m)
	}
	if strings.Contains(m, "MENUS:") {
		t.Errorf(".font menu me MENUS line nahi honi chahiye\n%s", m)
	}
	if !strings.Contains(m, "❮ 1000 ❯") {
		t.Errorf(".font menu COMMANDS count 1000 nahi hai\n%s", m)
	}
}

// TestFontCommandsHidden: font1..font1000 must be registered so they dispatch,
// but hidden from .menu.
func TestFontCommandsHidden(t *testing.T) {
	for _, n := range []int{1, 2, 500, 999, 1000} {
		name := "font" + itoaT(n)
		if _, ok := Commands[name]; !ok {
			t.Errorf("%s Commands map me registered nahi hai (dispatch nahi hoga)", name)
		}
		if !hiddenCommands[name] {
			t.Errorf("%s hiddenCommands me nahi hai (menu me dikh jayega)", name)
		}
	}
	if len(hiddenCommands) < 2*goldcmds.FontCount {
		t.Errorf("hiddenCommands me kaafi entries nahi: %d", len(hiddenCommands))
	}
}

func itoaT(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestFontCategoryInMenu: plain .menu me .FONT line dikhni chahiye.
func TestFontCategoryInMenu(t *testing.T) {
	m := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, nil, "")
	if !strings.Contains(m, ".FONT") {
		t.Errorf("plain .menu me .FONT nazar nahi aata\n%s", m)
	}
	if !strings.Contains(m, ".LOGO") {
		t.Errorf("plain .menu me .LOGO nazar nahi aata\n%s", m)
	}
	// OWNER ORDER: FONT exactly once — pehle ye category list se bhi aur
	// alag se bhi likha ja raha tha, is liye .menu me 2 bar dikhta tha.
	if got := strings.Count(m, ".FONT"); got != 1 {
		t.Errorf("plain .menu me .FONT %d bar hai, 1 hona chahiye\n%s", got, m)
	}
}

// TestFontCategoryShortcut: .font slug se FONT category menu khulna chahiye.
func TestFontCategoryShortcut(t *testing.T) {
	cat, ok := menuCategoryFromCommand("font")
	if !ok {
		t.Fatal(".font category shortcut resolve nahi hua")
	}
	if cat != "FONT" {
		t.Errorf("category = %q, want FONT", cat)
	}
}
