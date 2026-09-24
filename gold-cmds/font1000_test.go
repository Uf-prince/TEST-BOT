package goldcmds

import (
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// Every alphabet must be exactly as long as fontSrc and map the same runes, so
// a lookup by index never goes out of range or lands on the wrong character.
func TestFontAlphabetsAlignWithSource(t *testing.T) {
	want := len([]rune(fontSrc))
	if want != 62 {
		t.Fatalf("fontSrc should be 62 runes, got %d", want)
	}
	for i, a := range fontAlphabets {
		if got := len([]rune(a)); got != want {
			t.Errorf("fontAlphabets[%d] has %d runes, want %d", i, got, want)
		}
	}
	if len(fontAlphabets) != 13 {
		t.Fatalf("want 13 alphabets, got %d", len(fontAlphabets))
	}
	if len(fontStyleNames) != len(fontAlphabets) {
		t.Errorf("fontStyleNames %d != fontAlphabets %d", len(fontStyleNames), len(fontAlphabets))
	}
	if len(fontDecorations) != 400 {
		t.Errorf("want 400 decorations, got %d", len(fontDecorations))
	}
	if len(fontDecorationNames) != len(fontDecorations) {
		t.Errorf("fontDecorationNames %d != fontDecorations %d", len(fontDecorationNames), len(fontDecorations))
	}
	if len(fontSpacings) != 10 {
		t.Errorf("want 10 spacings, got %d", len(fontSpacings))
	}
	if len(fontSpacingNames) != len(fontSpacings) {
		t.Errorf("fontSpacingNames %d != fontSpacings %d", len(fontSpacingNames), len(fontSpacings))
	}
}

// OWNER ORDER: font1..font1000 must all be different designs — no repeats.
func TestFont1000DesignsAreUnique(t *testing.T) {
	for _, name := range []string{"UMAR", "MUHAMMAD UMAR", "Ali123", "X", "Abdullah Khan"} {
		seen := make(map[string]int, FontCount)
		for n := 1; n <= FontCount; n++ {
			r := FontRender(n, name)
			if prev, dup := seen[r]; dup {
				t.Fatalf("name %q: font%d == font%d (%q)", name, n, prev, r)
			}
			seen[r] = n
		}
		if len(seen) != FontCount {
			t.Errorf("name %q: got %d unique designs, want %d", name, len(seen), FontCount)
		}
	}
}

// The name must actually be transformed — a design that returns the input
// unchanged (or drops characters) is broken.
func TestFontRenderTransformsName(t *testing.T) {
	for n := 1; n <= FontCount; n++ {
		out := FontRender(n, "Umar")
		if strings.Contains(out, "Umar") {
			t.Fatalf("font%d left the name untransformed: %q", n, out)
		}
		if strings.Contains(out, "U") && n > 13 {
			t.Errorf("font%d kept a plain latin letter: %q", n, out)
		}
	}
}

// Characters outside fontSrc must pass through so names with punctuation,
// spaces or emoji do not lose information.
func TestFontRenderKeepsUnknownRunes(t *testing.T) {
	got := FontRender(1, "Umar!")
	if !strings.Contains(got, "!") {
		t.Errorf("punctuation dropped: %q", got)
	}
	if strings.Contains(got, " ") {
		t.Errorf("unexpected space in tight single-word render: %q", got)
	}
	withSpace := FontRender(1, "Umar Khan")
	if !strings.Contains(withSpace, " ") {
		t.Errorf("space between words dropped: %q", withSpace)
	}
}

// Out-of-range numbers must clamp to design 1 rather than panic or return "".
func TestFontRenderClampsRange(t *testing.T) {
	for _, n := range []int{-5, 0, FontCount + 1, 99999} {
		if got := FontRender(n, "Umar"); got == "" {
			t.Errorf("FontRender(%d) returned empty", n)
		}
	}
}

func TestFontDesignNameNonEmpty(t *testing.T) {
	for _, n := range []int{1, 2, 13, 14, 500, 999, 1000} {
		if got := FontDesignName(n); got == "" {
			t.Errorf("FontDesignName(%d) empty", n)
		}
	}
}

// sendingBridge records the exact sequence of send/edit calls so we can assert
// the font handler sends once and then edits that same message once (the
// autoreply pattern the owner asked for).
type sendingBridge struct {
	SessionBridge
	order    []string
	sentID   string
	editID   string
	editText string
}

func (sb *sendingBridge) ReplyWithID(info types.MessageInfo, text string) string {
	sb.order = append(sb.order, "send")
	sb.sentID = "font-msg-1"
	return sb.sentID
}

func (sb *sendingBridge) EditMessage(info types.MessageInfo, messageID string, newText string) bool {
	sb.order = append(sb.order, "edit")
	sb.editID = messageID
	sb.editText = newText
	return true
}

func (sb *sendingBridge) Reply(info types.MessageInfo, text string) {
	sb.order = append(sb.order, "reply")
}

// The font reply must be sent once and then edited once, editing the very
// message that was just sent, with the same styled text.
func TestFontRunNSendsThenEditsSameMessage(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	FontRunN(sb, info, []string{"UMAR"}, ".", 14)

	want := "*" + FontRender(14, "UMAR") + "*"
	if len(sb.order) != 2 || sb.order[0] != "send" || sb.order[1] != "edit" {
		t.Fatalf("call order = %v, want [send edit]", sb.order)
	}
	if sb.editID != sb.sentID {
		t.Errorf("edit target = %q, want sent id %q", sb.editID, sb.sentID)
	}
	if sb.editText != want {
		t.Errorf("edit text = %q, want %q", sb.editText, want)
	}
}

// With no name the handler must guide the user, not send an empty styled reply.
func TestFontRunNNoNameGuides(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	FontRunN(sb, info, nil, ".", 7)

	if len(sb.order) != 1 || sb.order[0] != "reply" {
		t.Fatalf("call order = %v, want [reply]", sb.order)
	}
}

// .font must be a visible menu command in the FONT category, and the 1000
// .fontN commands must stay out of the menu.
func TestFontCommandRegisteredVisible(t *testing.T) {
	var found *Command
	for _, c := range Commands() {
		if c.Name == "font" {
			cc := c
			found = &cc
			break
		}
	}
	if found == nil {
		t.Fatal(".font command registry me nahi hai")
	}
	if found.Hidden {
		t.Error(".font hidden nahi hona chahiye — menu me dikhna hai")
	}
	if found.Category != "FONT" {
		t.Errorf("category = %q, want FONT", found.Category)
	}
	for n := 1; n <= 3; n++ {
		name := "font" + fmt.Sprintf("%d", n)
		for _, c := range Commands() {
			if c.Name == name {
				t.Errorf("%s registry me nahi hona chahiye (main package me hidden hai)", name)
			}
		}
	}
}
