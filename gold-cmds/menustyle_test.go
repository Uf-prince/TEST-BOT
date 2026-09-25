package goldcmds

import (
	"strings"
	"testing"
)

// All 50 styles must resolve to a complete, renderable design.
func TestMenuStyleAllFiftyResolve(t *testing.T) {
	seen := map[string]bool{}
	for n := 1; n <= MenuStyleCount; n++ {
		st := MenuStyleAt(n)
		if st.N != n {
			t.Fatalf("style %d: N = %d", n, st.N)
		}
		if st.Name == "" || st.Sym == "" {
			t.Fatalf("style %d: empty name/sym", n)
		}
		for _, s := range []string{st.HeaderTop, st.HeaderBot, st.ListTop, st.ListBottom,
			st.ImpTop, st.ImpBottom, st.HeaderRowL, st.ListRowL} {
			if strings.TrimSpace(s) == "" {
				t.Fatalf("style %d (%s): empty frame field", n, st.Name)
			}
		}
		preview := MenuStylePreview(n, "AI MENU", ".")
		if preview == "" || !strings.Contains(preview, ".AI") {
			t.Fatalf("style %d: unusable preview\n%s", n, preview)
		}
		if strings.Contains(preview, "%!") {
			t.Fatalf("style %d: broken fmt verb in preview\n%s", n, preview)
		}
		// No unbalanced asterisks: each rendered line must have an even count,
		// otherwise WhatsApp bold spans leak between lines.
		for _, line := range strings.Split(preview, "\n") {
			if strings.Count(line, "*")%2 != 0 {
				t.Fatalf("style %d: unbalanced * in %q", n, line)
			}
		}
		if seen[st.Name] {
			t.Fatalf("style %d: duplicate name %q", n, st.Name)
		}
		seen[st.Name] = true
	}
	if len(seen) != MenuStyleCount {
		t.Fatalf("expected %d distinct style names, got %d", MenuStyleCount, len(seen))
	}
}

// MenuStyleAt clamps out-of-range input back to the classic design instead of
// panicking or returning a half-built style.
func TestMenuStyleOutOfRange(t *testing.T) {
	for _, n := range []int{-5, 0, 51, 999} {
		st := MenuStyleAt(n)
		if st.N != 1 || st.Name != classicStyle.Name {
			t.Fatalf("MenuStyleAt(%d) should clamp to classic, got %+v", n, st)
		}
	}
}

// ParseMenuStyleArg accepts the documented forms and rejects the rest.
func TestParseMenuStyleArg(t *testing.T) {
	ok := map[string]int{"1": 1, "50": 50, "SET 7": 7, "set 12": 12, " 3 ": 3}
	for in, want := range ok {
		got, good := ParseMenuStyleArg(in)
		if !good || got != want {
			t.Errorf("ParseMenuStyleArg(%q) = %d,%v want %d,true", in, got, good, want)
		}
	}
	for _, in := range []string{"", "0", "51", "abc", "set", "set 0", "set 51", "-1", "7x"} {
		if _, good := ParseMenuStyleArg(in); good {
			t.Errorf("ParseMenuStyleArg(%q) should fail", in)
		}
	}
}

// Every menu must get exactly one style command, named "<x>style".
func TestMenuStyleCommandsCoverAllMenus(t *testing.T) {
	cmds := menuStyleCommands()
	if len(cmds) != 17 {
		t.Fatalf("expected 17 per-menu style commands, got %d", len(cmds))
	}
	names := map[string]bool{}
	for _, mc := range cmds {
		name := menuStyleCommandName(mc)
		if !strings.HasSuffix(name, "style") {
			t.Errorf("style command %q must end in style", name)
		}
		if names[name] {
			t.Errorf("duplicate style command %q", name)
		}
		names[name] = true
	}
	for _, want := range []string{"menustyle", "logostyle", "fontstyle", "gamestyle", "equalizerstyle", "aimenustyle"} {
		if !names[want] {
			t.Errorf("missing style command .%s", want)
		}
	}
}

// A non-classic style must actually change the borders, the row decoration and
// the decorative font — not just the symbol.
func TestMenuStyleChangesLook(t *testing.T) {
	classic := MenuStyleAt(1)
	styled := MenuStyleAt(25)
	if styled.ListTop == classic.ListTop {
		t.Error("style 25 must use a different border than classic")
	}
	if styled.ListRowL == classic.ListRowL {
		t.Error("style 25 must use a different row decoration than classic")
	}
	if styled.Styled("AI") == classic.Styled("AI") {
		t.Error("style 25 must use a decorative font for titles")
	}
	// The decorative font must not touch ASCII command names.
	if got := styled.ListRow(".", "AIMENUPIC"); !strings.Contains(got, ".AIMENUPIC") {
		t.Errorf("command names must stay ASCII/copyable, got %q", got)
	}
}

// The style guide lists all 50 styles and ends with the shared TOMP3 info line.
func TestMenuStyleGuide(t *testing.T) {
	g := menuStyleGuide(".", "AI MENU", "aimenustyle")
	if !strings.Contains(g, ".AIMENUSTYLE SET <1-50>") {
		t.Errorf("guide missing SET usage\n%s", g)
	}
	if !strings.Contains(g, ".AIMENUSTYLE RESET") {
		t.Errorf("guide missing RESET usage\n%s", g)
	}
	if !strings.Contains(g, ".BOTMENUSTYLE SET <1-50>") {
		t.Errorf("guide missing bot-wide usage\n%s", g)
	}
	for n := 1; n <= MenuStyleCount; n++ {
		if !strings.Contains(g, MenuStyleName(n)) {
			t.Errorf("guide missing style %d name %q", n, MenuStyleName(n))
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(g), "*TYPE ❮ .TOMP3 ❯ FOR INFO*") {
		t.Errorf("guide must end with the TOMP3 info line\n%s", g)
	}
}
