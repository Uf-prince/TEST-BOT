package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// Every menu must have a VOICE command registered (hidden, owner-only), and
// the built-in default must be the GOLD-MD intro clip the owner supplied.
func TestMenuVoiceCommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, mc := range menuVoiceCommands() {
		c, ok := byName[mc.VoiceCmd]
		if !ok {
			t.Fatalf("voice command %q not registered", mc.VoiceCmd)
		}
		if !c.OwnerOnly {
			t.Errorf("%q must be owner-only", mc.VoiceCmd)
		}
		if !c.Hidden {
			t.Errorf("%q must be hidden", mc.VoiceCmd)
		}
	}
	// One voice command per menu (17 menus).
	if got := len(menuVoiceCommands()); got != len(menuMediaSpecs) {
		t.Errorf("voice command count = %d, want %d (one per menu)", got, len(menuMediaSpecs))
	}
	if DefaultMenuVoiceURL != "https://d.uguu.se/WHyKzyzH.mp3" {
		t.Errorf("default voice = %q", DefaultMenuVoiceURL)
	}
}

// The voice key must be distinct from pic and video so a menu can hold all
// three at once.
func TestMenuMediaSettingKeySeparatesVoice(t *testing.T) {
	if got := MenuMediaSettingKey("logo", "voice"); got != "logo:voice" {
		t.Errorf("voice key = %q, want logo:voice", got)
	}
	if got := MenuMediaSettingKey("LOGO", "VOICE"); got != "logo:voice" {
		t.Errorf("normalised voice key = %q, want logo:voice", got)
	}
}

// .logovoice <url> must store under "logo:voice" only, leaving the picture and
// video untouched, and the card must name the menu + its opening command.
func TestLogovoiceSetsPerMenuVoiceWithoutClobbering(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"https://x.test/a.jpg"}, ".")
	commandByName(t, "logovideo").Run(b, types.MessageInfo{}, []string{"https://x.test/v.mp4"}, ".")
	commandByName(t, "logovoice").Run(b, types.MessageInfo{}, []string{"https://x.test/v.mp3"}, ".")
	if b.media["logo"] != "https://x.test/a.jpg" {
		t.Errorf("pic clobbered: %q", b.media["logo"])
	}
	if b.media["logo:video"] != "https://x.test/v.mp4" {
		t.Errorf("video clobbered: %q", b.media["logo:video"])
	}
	if b.media["logo:voice"] != "https://x.test/v.mp3" {
		t.Errorf("voice = %q", b.media["logo:voice"])
	}
	got := b.last()
	if !strings.Contains(got, "LOGO MENU VOICE UPDATED") {
		t.Errorf("voice card missing menu label: %q", got)
	}
	if !strings.Contains(got, "❰ .logo ❱") {
		t.Errorf("voice card must point at the menu command: %q", got)
	}
}

// .aimenuvoice is the AI menu's voice sibling — the card must say .ai.
func TestAimenuvoiceUsesAiMenuCommand(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "aimenuvoice").Run(b, types.MessageInfo{}, []string{"https://x.test/a.mp3"}, ".")
	if b.media["ai:voice"] != "https://x.test/a.mp3" {
		t.Fatalf("ai voice = %q", b.media["ai:voice"])
	}
	if got := b.last(); !strings.Contains(got, "❰ .ai ❱") {
		t.Errorf("ai voice card must point at .ai: %q", got)
	}
}

// .logovoice reset must silence ONLY that menu, using the "off" sentinel (so
// the bot-wide voice does not re-appear for it).
func TestLogovoiceResetSilencesOnlyThatMenu(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logovoice").Run(b, types.MessageInfo{}, []string{"reset"}, ".")
	if b.media["logo:voice"] != MenuMediaVoiceOff {
		t.Fatalf("reset stored %q, want %q", b.media["logo:voice"], MenuMediaVoiceOff)
	}
	if !strings.Contains(b.last(), "LOGO MENU VOICE RESET") {
		t.Fatalf("reset card = %q", b.last())
	}
}

// A wrong-kind link must be rejected (a .jpg handed to .logovoice).
func TestLogovoiceRejectsImageLink(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logovoice").Run(b, types.MessageInfo{}, []string{"https://x.test/a.jpg"}, ".")
	if b.media["logo:voice"] != "" {
		t.Fatalf("image link accepted as voice: %q", b.media["logo:voice"])
	}
	if !strings.Contains(b.last(), "INVALID VOICE LINK") {
		t.Fatalf("reply = %q", b.last())
	}
}

// The .menuvoice guide lists every per-menu voice command (discovery).
func TestMenuVoiceFullListNamesEveryCommand(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "menuvoice").Run(b, types.MessageInfo{}, nil, ".")
	got := b.last()
	if !strings.Contains(got, "ALL MENU VOICE COMMANDS") {
		t.Fatalf("full list header missing: %q", got)
	}
	for _, mc := range menuVoiceCommands() {
		if !strings.Contains(got, "❰ ."+mc.VoiceCmd+" ❱") {
			t.Errorf("full list missing voice command %q", mc.VoiceCmd)
		}
	}
}

// .botvoice is the bot-wide sibling: default restores "" (→ built-in default),
// reset stores the "off" sentinel.
func TestBotvoiceCommandRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	c, ok := byName["botvoice"]
	if !ok {
		t.Fatal("botvoice not registered")
	}
	if !c.OwnerOnly || c.Hidden {
		t.Errorf("botvoice owner-only=%v hidden=%v; want true,false", c.OwnerOnly, c.Hidden)
	}
}

// The voice guide must carry its own command name (never a generic ".voice").
func TestMenuVoiceGuideShowsRealCommandName(t *testing.T) {
	for _, mc := range menuVoiceCommands() {
		b := newMMBridge()
		commandByName(t, mc.VoiceCmd).Run(b, types.MessageInfo{}, nil, ".")
		got := b.last()
		if !strings.Contains(got, "TYPE ❰ ."+mc.VoiceCmd+" ❱") {
			t.Errorf("%s guide missing its own name: %q", mc.VoiceCmd, got)
		}
		if !strings.Contains(got, "*❰ ."+mc.VoiceCmd+" <MP3-URL> ❱*") {
			t.Errorf("%s guide missing its URL form: %q", mc.VoiceCmd, got)
		}
		if !strings.Contains(got, "*❰ ."+mc.VoiceCmd+" RESET ❱*") {
			t.Errorf("%s guide missing its RESET form: %q", mc.VoiceCmd, got)
		}
		if !strings.Contains(got, "*TYPE ❮ .TOMP3 ❯ FOR INFO*") {
			t.Errorf("%s guide missing the TOMP3 info line: %q", mc.VoiceCmd, got)
		}
		if !strings.HasSuffix(strings.TrimSpace(got), "*TYPE ❮ .TOMP3 ❯ FOR INFO*") {
			t.Errorf("%s guide must END with the TOMP3 info line: %q", mc.VoiceCmd, got)
		}
	}
}

// The .botvoice guide must carry the TOMP3 info line at its end too.
func TestBotvoiceGuideHasTomp3InfoLine(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "botvoice").Run(b, types.MessageInfo{}, nil, ".")
	got := strings.TrimSpace(b.last())
	if !strings.HasSuffix(got, "*TYPE ❮ .TOMP3 ❯ FOR INFO*") {
		t.Errorf("botvoice guide must end with the TOMP3 info line: %q", got)
	}
}
