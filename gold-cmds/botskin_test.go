package goldcmds

import (
	"strings"
	"testing"
)

// Style 1 is the classic look: the skin must be a byte-for-byte no-op so
// enabling the feature can never change an unskinned bot's messages.
func TestSkinClassicIsNoOp(t *testing.T) {
	st := MenuStyleAt(1)
	in := "*🔰 ALIVE 🔰*\n*✅ CONNECTED*\n_ready_"
	if got := st.SkinText(in); got != in {
		t.Fatalf("classic skin changed text:\n in: %q\nout: %q", in, got)
	}
	if st.SkinEnabled() {
		t.Fatal("style 1 must not report skin enabled")
	}
}

// A skinned style must change font, wrap **bold** with the style symbol and
// swap the accent emojis for that same symbol.
func TestSkinRewritesFontMarksAndEmojis(t *testing.T) {
	st := MenuStyleAt(7)
	out := st.SkinText("*🔰 ALIVE 🔰*\n*✅ CONNECTED*")

	if out == "" || out == "*🔰 ALIVE 🔰*\n*✅ CONNECTED*" {
		t.Fatalf("style 7 did not change the text: %q", out)
	}
	if !strings.Contains(out, st.Sym) {
		t.Errorf("style symbol %q missing from skinned text: %q", st.Sym, out)
	}
	if strings.Contains(out, "🔰") {
		t.Errorf("old accent emoji survived the skin: %q", out)
	}
	if strings.Contains(out, "✅") {
		t.Errorf("old accent emoji survived the skin: %q", out)
	}
	// ALIVE in ASCII must not survive in the same spelling (font remapped it).
	if strings.Contains(out, "ALIVE") && st.font > 0 {
		same := applyMenuFont(st.font, "ALIVE") == "ALIVE"
		if !same {
			t.Errorf("letters were not remapped by the style font: %q", out)
		}
	}
}

// Marks must be balanced: every `**` becomes a matched symbol pair, never a
// leftover single `*` pair that WhatsApp would render as plain bold.
func TestSkinMarksAreBalanced(t *testing.T) {
	st := MenuStyleAt(3)
	out := st.SkinText("**ONE**\n**TWO**\n__THREE__")
	if strings.Count(out, st.Sym) < 5 {
		t.Errorf("not all marks got the style symbol: %q", out)
	}
}

// Fancy glyphs typed into a command token must normalise back to ASCII, so the
// bot stays controllable in any skin.
func TestSkinNormalizeInputRoundTrip(t *testing.T) {
	for _, name := range []string{"menu", "alive", "ping", "botstyle", "simdata", "logo1000"} {
		skinned := applyMenuFont(7, strings.ToUpper(name))
		got := SkinNormalizeInput("*" + skinned + "*")
		want := strings.ToUpper(name)
		if got != want {
			t.Errorf("normalise(%q) = %q, want %q", skinned, got, want)
		}
	}
}

// Plain ASCII input must survive normalisation unchanged (digits/letters kept).
func TestSkinNormalizeInputKeepsASCII(t *testing.T) {
	if got := SkinNormalizeInput("menu"); got != "menu" {
		t.Fatalf("got %q", got)
	}
	if got := SkinNormalizeInput("logo1000"); got != "logo1000" {
		t.Fatalf("got %q", got)
	}
}

// SkinKey renders the fancy token but must keep the prefix and stay non-empty.
func TestSkinKeyKeepsPrefix(t *testing.T) {
	st := MenuStyleAt(4)
	got := st.SkinKey(".", "menu")
	if !strings.HasPrefix(got, ".") {
		t.Fatalf("prefix lost: %q", got)
	}
	if got == ".menu" {
		t.Fatalf("style 4 should decorate the token: %q", got)
	}
	if norm := strings.ToUpper(SkinNormalizeInput(strings.TrimPrefix(got, "."))); norm != "MENU" {
		t.Fatalf("skinned key did not normalise back: %q -> %q", got, norm)
	}
}

// Every style 2..50 must produce a skin without panicking and without leaking
// the classic accent emojis.
func TestSkinAllStyles(t *testing.T) {
	for n := 2; n <= MenuStyleCount; n++ {
		st := MenuStyleAt(n)
		out := st.SkinText("*🔰 TEST 🔰*")
		if out == "" {
			t.Fatalf("style %d produced empty output", n)
		}
		if strings.Contains(out, "🔰") {
			t.Errorf("style %d leaked the classic accent: %q", n, out)
		}
	}
}
