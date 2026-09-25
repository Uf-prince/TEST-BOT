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
