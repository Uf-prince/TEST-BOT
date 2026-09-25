package goldcmds

import (
	"strings"
	"sync"
	"unicode"
)

// ============================================================================
// GOLD-MD - BOT-WIDE SKIN (.botstyle)
// ----------------------------------------------------------------------------
// Ek hi command se POORE bot ka text skin badalta hai:
//   * har letter/digit style ke decorative font me
//   * **bold** / __italic__ marks style ke symbol se
//   * accent emojis style ke symbol se
//
// Command TOKENS bhi fancy dikhte hain, lekin dispatch plain ASCII naam par
// bhi chalta hai (handler.go SkinNormalizeInput fallback) - token sirf
// cosmetic hai, is liye bot ko control karna kabhi mushkil nahi hota.
//
// Style 1 = classic = skin OFF (text jaisa hai waisa).
// ============================================================================

// skinSym is the style's signature symbol ("" when the style has none).
func (st MenuStyle) skinSym() string { return strings.TrimSpace(st.Sym) }

// SkinText renders a whole outgoing message in this style's skin. Pure: it
// never touches global state, so it is safe on every send path.
func (st MenuStyle) SkinText(s string) string {
	if s == "" || st.N <= 1 {
		return s
	}
	sym := st.skinSym()
	s = skinMarks(s, sym)
	if st.font > 0 {
		s = applyMenuFont(st.font, s)
	}
	s = skinAccents(s, sym)
	return s
}

// SkinKey renders one command token (prefix + name) in the style's font, so a
// menu can show the fancy spelling. The plain spelling keeps working because
// the handler normalises the typed token back to ASCII.
func (st MenuStyle) SkinKey(prefix, name string) string {
	if st.font <= 0 || st.N <= 1 {
		return prefix + name
	}
	return prefix + applyMenuFont(st.font, name)
}

// skinMarks converts WhatsApp bold/italic marks into the style's symbol pair,
// e.g. **ALIVE** -> *<sym> ALIVE <sym>*.
func skinMarks(s, sym string) string {
	if sym == "" {
		s = strings.ReplaceAll(s, "**", "*")
		s = strings.ReplaceAll(s, "__", "_")
		return s
	}
	s = skinReplacePaired(s, "**", "*"+sym+" ", " "+sym+"*")
	s = skinReplacePaired(s, "__", "_"+sym+" ", " "+sym+"_")
	return s
}

// skinReplacePaired replaces alternating occurrences of tok with open/close.
func skinReplacePaired(s, tok, open, close string) string {
	if !strings.Contains(s, tok) {
		return s
	}
	var b strings.Builder
	openNext := true
	for {
		i := strings.Index(s, tok)
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		if openNext {
			b.WriteString(open)
		} else {
			b.WriteString(close)
		}
		openNext = !openNext
		s = s[i+len(tok):]
	}
	return b.String()
}

// skinAccentEmojis are the decorative accents the bot sprinkles on headings /
// status lines. They are swapped for the style's symbol so the whole bot takes
// one consistent look. Written as escapes to keep this file ASCII-clean.
var skinAccentEmojis = []string{
	"\U0001F530",
	"\u2728",
	"\u2705",
	"\u274C",
	"\u26A0\uFE0F",
	"\U0001F4CC",
	"\u2B50",
	"\U0001F525",
	"\u2699\uFE0F",
	"\U0001F680",
}

func skinAccents(s, sym string) string {
	if sym == "" {
		return s
	}
	for _, e := range skinAccentEmojis {
		s = strings.ReplaceAll(s, e, sym)
	}
	return s
}

// ── reverse normalisation (typed fancy token -> ASCII) ─────────────────────

var (
	skinRevOnce sync.Once
	skinRevMap  map[rune]rune
)

func skinReverseTable() map[rune]rune {
	skinRevOnce.Do(func() {
		m := make(map[rune]rune, 1024)
		for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789" {
			for idx := 1; idx <= 9; idx++ {
				if d := fontRune(idx, r); d != r {
					m[d] = r
				}
			}
		}
		m['\u3000'] = ' '
		skinRevMap = m
	})
	return skinRevMap
}

// SkinNormalizeInput maps a fancy/decorated typed command token back to plain
// ASCII, so commands keep dispatching even when typed in the skinned font. It
// also strips the style decorations (*, _ and unknown symbol runes) that wrap
// a token. Only ever consulted as a fallback after the plain parse fails.
func SkinNormalizeInput(s string) string {
	m := skinReverseTable()
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '*' || r == '_' {
			continue
		}
		if a, ok := m[r]; ok {
			b.WriteRune(a)
			continue
		}
		if r < 128 {
			b.WriteRune(r)
			continue
		}
		// Unknown decorative rune: keep only real letters/digits, drop the
		// rest (a command token is ASCII alnum).
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SkinEnabled reports whether this style actually changes text.
func (st MenuStyle) SkinEnabled() bool { return st.N > 1 }
