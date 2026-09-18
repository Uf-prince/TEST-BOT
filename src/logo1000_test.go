package main

import (
	"fmt"
	"strings"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// TestLogo1000MenusAndRegistry — owner order invariants:
//  1. .menu aur .fullmenu me SIRF .logo dikhta hai (logoN kabhi nahi)
//  2. Commands map me logo1..logo1000 sab registered hain
//  3. gold-cmds registry me .logo visible hai + desc fullmenu me aata hai
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

	// 3) .menu caption build: logoN naam kabhi nahi, .logo naam hamesha
	// (nil view nahi — CmdNameViewFor ko live session chahiye; default view me
	// koi rename nahi hota aur Mode "" hota hai)
	menuView := &goldcmds.CmdNameView{Renames: map[string]string{}, Mode: ""}
	menu := buildCategoryMenu("UMAR", "92X", "0H 5M", ".", "USER", "GOLD-MD WHATSAPP BOT", 1, menuView, "AI & MEDIA")
	// .logo line prefix ke sath aani chahiye (menu me command names prefix ke sath likhe hain)
	if !strings.Contains(menu, ".logo") && !strings.Contains(menu, "LOGO") {
		t.Error(".menu me .logo entry nahi mili")
	}
	for _, banned := range []string{".logo1 ", ".logo2 ", ".logo500 ", ".logo1000 ", "logo1\n", "logo2\n", "logo1000\n"} {
		if strings.Contains(menu, banned) {
			t.Errorf(".menu me %q dikh gaya — hidden hona chahiye", banned)
		}
	}

	// 4) .fullmenu text: sirf .logo block, koi logoN block nahi
	parts := buildFullMenuText(".", "GOLD-MD WHATSAPP BOT", "UMAR", "92X", "0H 5M", 1)
	full := strings.Join(parts, "")
	if !strings.Contains(full, "COMMAND ❮logo❯") {
		t.Error(".fullmenu me ❮logo❯ entry nahi mili")
	}
	for _, banned := range []string{"❮logo1❯", "❮logo2❯", "❮logo500❯", "❮logo1000❯", "❮logo12❯", "❮logo123❯"} {
		if strings.Contains(full, banned) {
			t.Errorf(".fullmenu me %q dikh gaya — hidden hona chahiye", banned)
		}
	}
	// desc visible in fullmenu (owner: "descryption sirf .fullmenu me")
	if !strings.Contains(full, "1000 DHAMAKEDAR NAME LOGO DESIGNS") {
		t.Error(".fullmenu me .logo description nahi mili")
	}
	// fullmenu size safety: har part WhatsApp limit ke andar
	for i, p := range parts {
		if len(p) > 60000 {
			t.Errorf("fullmenu part %d too big: %d chars", i, len(p))
		}
	}
	fmt.Printf("FULLMENU PARTS: %d, .logo visible, 1000 logoN hidden — OK\n", len(parts))
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
