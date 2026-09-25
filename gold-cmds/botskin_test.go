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

// Every non-classic style MUST change the font, not just the symbol (owner
// report: "ek symbol badala hai bas font wahi same hai"). A style whose font is
// 0 would leave every letter untouched, which is exactly the bug.
func TestSkinEveryStyleChangesFont(t *testing.T) {
	colors := applyMenuFont(0, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for n := 2; n <= MenuStyleCount; n++ {
		st := MenuStyleAt(n)
		if st.font <= 0 {
			t.Fatalf("style %d (%s) has font 0 - letters would not change", n, st.Name)
		}
		// Some fonts (e.g. small-caps) only map one case, so check both cases
		// before declaring the font a no-op.
		upper := applyMenuFont(st.font, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		lower := applyMenuFont(st.font, "abcdefghijklmnopqrstuvwxyz")
		if upper == colors && lower == colors {
			t.Fatalf("style %d (%s) font renders like the plain font", n, st.Name)
		}
	}
}

// A skinned bot must not keep announcing the stock brand: the identity is
// rewritten to the style's own name so users cannot recognise it at a glance.
func TestSkinRewritesBrandIdentity(t *testing.T) {
	st := MenuStyleAt(7)
	out := st.SkinText("*GOLD-MD WHATSAPP BOT*")
	if strings.Contains(out, "GOLD") {
		t.Fatalf("stock brand survived the skin: %q", out)
	}
	if !strings.Contains(SkinNormalizeInput(out), "CROWN ROYALE") {
		t.Fatalf("style name missing from the rebranded line: %q", out)
	}
}

// The classic style must keep the stock brand untouched.
func TestSkinClassicKeepsBrand(t *testing.T) {
	in := "*GOLD-MD WHATSAPP BOT*"
	if got := MenuStyleAt(1).SkinText(in); got != in {
		t.Fatalf("classic skin touched the brand: %q", got)
	}
}

// The TEXT itself must change, not just the font: a skinned bot must not keep
// repeating the stock phrases a user could recognise.
func TestSkinRewritesWording(t *testing.T) {
	st := MenuStyleAt(7)
	out := st.SkinText("*GOLD-MD HAS BEEN STARTED*\n*CONNECTED SUCCESSFULLY*")
	norm := SkinNormalizeInput(out)
	if strings.Contains(norm, "HAS BEEN STARTED") {
		t.Fatalf("stock start phrase survived: %q", norm)
	}
	if strings.Contains(norm, "CONNECTED SUCCESSFULLY") {
		t.Fatalf("stock connected phrase survived: %q", norm)
	}
	if want := wordingPackFor(7).started; !strings.Contains(norm, want) {
		t.Fatalf("style 7 wording pack not applied: want %q in %q", want, norm)
	}
}

// Two different style groups must not speak the same stock phrases.
func TestSkinWordingVariesByStyle(t *testing.T) {
	a := SkinNormalizeInput(MenuStyleAt(2).SkinText("CONNECTED SUCCESSFULLY"))
	b := SkinNormalizeInput(MenuStyleAt(40).SkinText("CONNECTED SUCCESSFULLY"))
	if a == b {
		t.Fatalf("style groups share the same wording: %q", a)
	}
}

// Classic must keep the exact stock wording.
func TestSkinClassicKeepsWording(t *testing.T) {
	in := "*GOLD-MD HAS BEEN STARTED* CONNECTED SUCCESSFULLY"
	if got := MenuStyleAt(1).SkinText(in); got != in {
		t.Fatalf("classic skin changed wording: %q", got)
	}
}

// The footer must keep the user-owned name (.botname) but take the style's
// font/design - not the identity/wording rewrite that renames the bot.
func TestSkinFooterKeepsNameButTakesFont(t *testing.T) {
	st := MenuStyleAt(7)
	foot := "*GOLD-MD WHATSAPP BOT*\nTYPE *❮ .BOTNAME YOUR NAME ❯*"
	out := st.SkinFooter(foot)

	if out == foot {
		t.Fatalf("footer did not take the style font: %q", out)
	}
	if !strings.Contains(SkinNormalizeInput(out), "GOLD-MD WHATSAPP BOT") {
		t.Fatalf("footer name was rewritten (must stay user-owned): %q", out)
	}
	if strings.Contains(SkinNormalizeInput(out), "CROWN ROYALE") {
		t.Fatalf("footer must not be rebranded like the body: %q", out)
	}
}

// Classic style keeps the footer byte-identical.
func TestSkinFooterClassicNoOp(t *testing.T) {
	foot := "*GOLD-MD WHATSAPP BOT*"
	if got := MenuStyleAt(1).SkinFooter(foot); got != foot {
		t.Fatalf("classic footer changed: %q", got)
	}
}
