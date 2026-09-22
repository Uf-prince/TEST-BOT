package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestLogo1000MenusAndRegistry — owner order invariants:
//  1. .menu me SIRF .logo dikhta hai (display name .LOGO), logoN kabhi nahi
//  2. Commands map me logo1..logo1000 sab registered hain (hidden)
//  3. gold-cmds registry me .logo visible hai
//  4. .logo likhne par fancy boxed menu banta hai (plain text nahi)
func TestLogo1000MenusAndRegistry(t *testing.T) {
	// 1) gold-cmds registry: .logo visible command with description
	var foundLogo *goldcmds.Command
	for i, c := range goldcmds.Commands() {
		if strings.EqualFold(c.Name, "logo") {
			foundLogo = &goldcmds.Commands()[i]
		}
	}
	if foundLogo == nil {
		t.Fatal("gold-cmds registry me .logo command nahi mila")
	}
	if foundLogo.Hidden {
		t.Error(".logo HIDDEN hai — visible hona chahiye")
	}
	if !strings.Contains(strings.ToUpper(foundLogo.Desc), "LOGO") {
		t.Error(".logo desc me LOGO mention nahi")
	}

	// 2) main Commands map me logo1..logo1000 sab hon
	missing := 0
	for n := 1; n <= goldcmds.LogoCount; n++ {
		name := fmt.Sprintf("logo%d", n)
		if _, ok := Commands[name]; !ok {
			missing++
			if missing <= 3 {
				t.Errorf("Commands map me %s missing", name)
			}
		}
		if !hiddenCommands[name] {
			t.Errorf("%s hiddenCommands me nahi hai", name)
		}
	}
	if missing > 0 {
		t.Fatalf("total missing logoN commands: %d", missing)
	}

	// 3) .menu caption build: logoN naam kabhi nahi, .LOGO naam hamesha
	menuView := &goldcmds.CmdNameView{Renames: map[string]string{}, Mode: ""}
	menu := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, menuView, "AI & MEDIA")
	if !strings.Contains(menu, ".LOGO") {
		t.Errorf(".menu me .LOGO entry nahi mili\n%s", menu)
	}
	for _, banned := range []string{".logo1 ", ".logo2 ", ".logo500 ", ".logo1000 ", "logo1\n", "logo2\n", "logo1000\n"} {
		if strings.Contains(menu, banned) {
			t.Errorf(".menu me %q dikh gaya — hidden hona chahiye", banned)
		}
	}

	// 3b) plain .menu (category list) must ALSO show .LOGO (owner order)
	plain := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, menuView, "")
	if !strings.Contains(plain, ".LOGO") {
		t.Errorf("plain .menu me .LOGO entry nahi mili\n%s", plain)
	}
	// 3c) COMMANDS total must include the 1000 hidden logo commands.
	idx := strings.Index(plain, "COMMANDS :❯ ❮ ")
	if idx < 0 {
		t.Fatalf("plain .menu me COMMANDS line nahi mili\n%s", plain)
	}
	rest := plain[idx+len("COMMANDS :❯ ❮ "):]
	end := strings.Index(rest, " ❯")
	if end < 0 {
		t.Fatalf("COMMANDS count parse fail\n%s", plain)
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if err != nil {
		t.Fatalf("COMMANDS count atoi fail: %v\n%s", err, plain)
	}
	if n < goldcmds.LogoCount {
		t.Errorf("COMMANDS total %d me 1000 logo commands count nahi hue (min %d chahiye)", n, goldcmds.LogoCount)
	}
	fmt.Printf("plain .menu COMMANDS total = %d (includes %d logo cmds) — OK\n", n, goldcmds.LogoCount)

	// 4) .logo menu: fancy boxed format (same as other category menus)
	logoMenu := buildLogoMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, menuView)
	if !strings.Contains(logoMenu, "╔════ ≪ •❈• ≫ ════╗") {
		t.Errorf(".logo menu missing fancy box header\n%s", logoMenu)
	}
	if !strings.Contains(logoMenu, "🔰 LOGO 🔰") {
		t.Errorf(".logo menu header title LOGO missing\n%s", logoMenu)
	}
	if !strings.Contains(logoMenu, ".LOGO1 ❮ YOUR NAME ❯") {
		t.Errorf(".logo menu missing .LOGO1 entry\n%s", logoMenu)
	}
	if !strings.Contains(logoMenu, ".LOGO1000 ❮ YOUR NAME ❯") {
		t.Errorf(".logo menu missing .LOGO1000 entry\n%s", logoMenu)
	}
	// MENUS line must NOT appear in the .logo menu (only plain .menu)
	if strings.Contains(logoMenu, "MENUS:") {
		t.Errorf(".logo menu me MENUS line nahi honi chahiye\n%s", logoMenu)
	}
	fmt.Printf(".logo boxed menu OK, 1000 logoN hidden — OK\n")
}

// TestLogo1000PromptUniqueness — har N ka design unique hona chahiye
// (100 base × 10 text × 10 light — koi duplicate prompt nahi).
func TestLogo1000PromptUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for n := 1; n <= goldcmds.LogoCount; n++ {
		p := goldcmds.LogoPromptForN(n, "UMAR")
		if seen[p] {
			t.Fatalf("duplicate prompt for n=%d", n)
		}
		seen[p] = true
	}
	// prompt me naam 2 bar aana chahiye (spelling emphasis)
	p1 := goldcmds.LogoPromptForN(1, "UMAR")
	if strings.Count(strings.ToUpper(p1), "UMAR") < 2 {
		t.Error("name spelling emphasis missing in prompt")
	}
	// design name bhi unique
	names := make(map[string]bool, 1000)
	for n := 1; n <= goldcmds.LogoCount; n++ {
		dn := goldcmds.LogoDesignName(n)
		if names[dn] {
			t.Fatalf("duplicate design name for n=%d: %s", n, dn)
		}
		names[dn] = true
	}
	fmt.Printf("1000 unique prompts + 1000 unique design names — OK\n")
}
