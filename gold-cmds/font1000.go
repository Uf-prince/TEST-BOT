package goldcmds

// ============================================================================
// GOLD-MD — 1000 NAME FONT DESIGNS: render engine + .font list command
// File: font1000.go
// ============================================================================
// OWNER ORDER: .menu me ek naya category "FONT" ho (baaki menus ki tarah).
//   .font            → fancy boxed menu, FONT1 .. FONT1000 ki list
//   .fontN {NAME}    → us design me NAME ko fancy style me likh kar reply
// Font text-only hai (koi image generate nahi), is liye reply instantly aata hai.
// Tables font1000_keys.go me hain.
// ============================================================================

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const fontMaxNameLen = 200

// fontSrcRunes / fontAlphabetRunes are the rune views of fontSrc and
// fontAlphabets. Indexing the string form byte-wise would emit broken UTF-8
// (each fancy glyph is 3-4 bytes), so the lookup works in runes.
var (
	fontSrcRunes      = []rune(fontSrc)
	fontAlphabetRunes [][]rune
)

func init() {
	fontAlphabetRunes = make([][]rune, len(fontAlphabets))
	for i, a := range fontAlphabets {
		fontAlphabetRunes[i] = []rune(a)
	}
}

// FontRender maps every character of name through design N's alphabet, then
// wraps the result in that design's decoration pair and applies its spacing.
//
// Characters that are not in fontSrc (punctuation, emoji, non-Latin script)
// pass through unchanged so names like "Umar ❤️" stay readable.
func FontRender(n int, name string) string {
	if n < 1 || n > FontCount {
		n = 1
	}
	m := n - 1
	alpha := fontAlphabetRunes[m%len(fontAlphabetRunes)]
	m /= len(fontAlphabetRunes)
	dec := fontDecorations[m%len(fontDecorations)]
	m /= len(fontDecorations)
	sp := fontSpacings[m%len(fontSpacings)]

	var b strings.Builder
	b.Grow(len(name)*2 + 8)
	for _, r := range name {
		if r == ' ' {
			if sp != "" {
				b.WriteString(sp)
			} else {
				b.WriteRune(' ')
			}
			continue
		}
		if i := indexRune(fontSrcRunes, r); i >= 0 {
			b.WriteRune(alpha[i])
			continue
		}
		b.WriteRune(r)
	}
	return dec[0] + b.String() + dec[1]
}

// indexRune returns the position of r in rs, or -1. Linear scan is fine: the
// table is 62 entries and names are short.
func indexRune(rs []rune, r rune) int {
	for i, c := range rs {
		if c == r {
			return i
		}
	}
	return -1
}

// FontDesignName is the human-readable design label, e.g.
// "Bold + Sparkle Twinkle + Tight".
func FontDesignName(n int) string {
	if n < 1 || n > FontCount {
		n = 1
	}
	m := n - 1
	style := fontStyleNames[m%len(fontAlphabets)]
	m /= len(fontAlphabets)
	dec := fontDecorationNames[m%len(fontDecorations)]
	m /= len(fontDecorations)
	sp := fontSpacingNames[m%len(fontSpacings)]
	return fmt.Sprintf("%s + %s + %s", style, dec, sp)
}

// handleFontList implements bare .font — the boxed FONT1..FONT1000 menu.
func handleFontList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	s.ShowFontMenu(info, args, prefix)
}

// FontRunN is the entry point for the hidden .font1..font1000 commands.
// It is text-only, so unlike .logoN there is no generation timeout to manage.
func FontRunN(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	if n < 1 || n > FontCount {
		s.Reply(info, "*FONT NUMBER 1 SE 1000 TAK HI HAI*")
		return
	}
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		s.Reply(info, fmt.Sprintf(
			"*⬛ FONT %d — %s ⬛*\n\n"+
				"*IS DESIGN ME NAME FANCY BANANE KE LIYE:*\n"+
				"*❯ %sfont%d ❮ YOUR NAME ❯*\n\n"+
				"*EXAMPLE:*\n"+
				"*❯ %sfont%d UMAR*\n\n"+
				"*TYPE .FONT SE POORI LIST DEKHO — 1000 DHAMAKEDAR DESIGNS*",
			n, FontDesignName(n), prefix, n, prefix, n))
		return
	}
	if len([]rune(name)) > fontMaxNameLen {
		name = string([]rune(name)[:fontMaxNameLen])
	}
	styled := "*" + FontRender(n, name) + "*"

	// Same behaviour as the AI autoreply: send the text, then edit that same
	// message once after a short delay with the exact same text. WhatsApp shows
	// it as an edited message and the fancy glyphs re-render cleanly (some
	// clients cache the first paint of the astral-plane characters).
	sentID := s.ReplyWithID(info, styled)
	if sentID != "" {
		arSleep(arEditDelay)
		s.EditMessage(info, sentID, styled)
	}
}

func init() {
	// OWNER ORDER: menu me SIRF .font dikhta hai (display name .FONT).
	// font1..font1000 main-package Commands map me hidden hote hain
	// (font1000_main.go) — menu me kabhi nahi dikhte. .font likhne par
	// fancy boxed menu banta hai (ShowFontMenu → manager.go CmdFontMenu).
	Register(Command{
		Name:     "font",
		Category: "FONT",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF 1000 DHAMAKEDAR FANCY FONT DESIGNS. IT SHOWS .FONT1 TO .FONT1000 — TYPE ANY FONT NUMBER WITH YOUR NAME TO WRITE YOUR NAME IN THAT FANCY STYLE.",
		Run:      handleFontList,
	})
}
