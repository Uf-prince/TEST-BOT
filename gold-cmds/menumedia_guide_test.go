package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// The no-arg guide must show the EXACT command the owner typed, never a
// generic ".pic" / ".video". Regression test for the hardcoded-suffix bug.
func TestMenuMediaGuideShowsRealCommandName(t *testing.T) {
	cases := []struct {
		cmd   string
		wrong string
	}{
		{"logopic", ".pic"},
		{"logovideo", ".video"},
		{"converterpic", ".pic"},
		{"convertervideo", ".video"},
		{"aimenupic", ".pic"},
		{"corevideo", ".video"},
		{"menupic", ".pic"},
		{"menuvideo", ".video"},
		{"groupmenupic", ".pic"},
		{"toolsvideo", ".video"},
	}
	for _, c := range cases {
		b := newMMBridge()
		commandByName(t, c.cmd).Run(b, types.MessageInfo{}, nil, ".")
		got := b.last()
		// The guide must carry its own name in the reply-link form.
		if !strings.Contains(got, "TYPE ❰ ."+c.cmd+" ❱") {
			t.Errorf("%s guide missing its own name: %q", c.cmd, got)
		}
		if !strings.Contains(got, "*❰ ."+c.cmd+" <URL> ❱*") {
			t.Errorf("%s guide missing its own URL form: %q", c.cmd, got)
		}
		if !strings.Contains(got, "*❰ ."+c.cmd+" RESET ❱*") {
			t.Errorf("%s guide missing its own RESET form: %q", c.cmd, got)
		}
		if strings.Contains(got, "TYPE ❰ "+c.wrong+" ") || strings.Contains(got, "*❰ "+c.wrong+" ") {
			t.Errorf("%s guide leaked the generic %q command: %q", c.cmd, c.wrong, got)
		}
	}
}

// The success card must name the menu the owner actually changed AND point at
// the command that OPENS that menu (.logo / .game / .ai), never the media
// command (.logopic only sets the pic) and never a generic ALIVE/MENU hint.
func TestMenuMediaConfirmationIsPerMenu(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"https://x/p.jpg"}, ".")
	got := b.last()
	if !strings.Contains(got, "LOGO MENU PIC UPDATED") {
		t.Errorf("confirmation missing menu label: %q", got)
	}
	if !strings.Contains(got, "❰ .logo ❱") {
		t.Errorf("confirmation must point at the menu command: %q", got)
	}
	if strings.Contains(got, "❰ .logopic ❱") {
		t.Errorf("confirmation must not point at the media command: %q", got)
	}
	if strings.Contains(got, "ALIVE") || strings.Contains(got, "❰ .menu ❱") {
		t.Errorf("confirmation leaked the generic ALIVE/MENU hint: %q", got)
	}

	// Category menu: the hint is the category command (.game), not .gamevideo.
	b = newMMBridge()
	commandByName(t, "gamevideo").Run(b, types.MessageInfo{}, []string{"https://x/g.mp4"}, ".")
	got = b.last()
	if !strings.Contains(got, "GAME MENU VIDEO UPDATED") {
		t.Errorf("video confirmation missing menu label: %q", got)
	}
	if !strings.Contains(got, "❰ .game ❱") {
		t.Errorf("video confirmation must point at the menu command: %q", got)
	}
	if strings.Contains(got, "❰ .gamevideo ❱") {
		t.Errorf("video confirmation must not point at the media command: %q", got)
	}

	// The AI menu is opened by `.ai` even though `.aimenupic` set the picture.
	b = newMMBridge()
	commandByName(t, "aimenupic").Run(b, types.MessageInfo{}, []string{"https://x/a.jpg"}, ".")
	if got = b.last(); !strings.Contains(got, "❰ .ai ❱") {
		t.Errorf("ai confirmation must point at .ai: %q", got)
	}

	// Resets are per-menu too.
	b = newMMBridge()
	commandByName(t, "converterpic").Run(b, types.MessageInfo{}, []string{"reset"}, ".")
	if got = b.last(); !strings.Contains(got, "CONVERTER MENU PIC RESET") {
		t.Errorf("reset card missing menu label: %q", got)
	}

	// The hint is a complete bold token: exactly one pair of surrounding stars.
	b = newMMBridge()
	commandByName(t, "fontpic").Run(b, types.MessageInfo{}, []string{"https://x/f.jpg"}, ".")
	hint := ""
	for _, line := range strings.Split(b.last(), "\n") {
		if strings.Contains(line, "FOR TEST TYPE") {
			hint = line
		}
	}
	if hint != "*FOR TEST TYPE ❰ .font ❱*" {
		t.Errorf("hint not fully bold/malformed: %q", hint)
	}
}

// The list card shown by .menupic / .menuvideo must name each command too.
func TestMenuMediaFullListNamesEveryCommand(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "menupic").Run(b, types.MessageInfo{}, nil, ".")
	got := b.last()
	if !strings.Contains(got, "ALL MENU MEDIA COMMANDS") {
		t.Fatalf("full list header missing: %q", got)
	}
	for _, mc := range menuMediaCommands {
		if !strings.Contains(got, "*❰ ."+mc.Name+" ❱") {
			t.Errorf("full list missing command %q", mc.Name)
		}
	}
}
