package goldcmds

// ============================================================================
// GOLD-MD — TRANSLATION PRE/POST PROCESSING
// File: trtsoft.go
// ============================================================================
// Two free-endpoint quirks made whole menu lines stay English (owner report:
// "kuch cmnds ke texts ki ho rhe, kch English reh jate"):
//
//  1. ALL-CAPS ECHO. The endpoint treats an all-caps token as a proper noun and
//     returns it untouched: "UPTIME" -> "UPTIME" while "Uptime" -> "اپ ٹائم".
//     The bot writes every label in caps (USER, OWNER, MENUS, COMMANDS, UPTIME,
//     PREFIX), so exactly the labels the owner complained about were echoed back.
//     Fix: soften caps runs to Title case for the request, then re-apply caps to
//     the translated line so the bot keeps its house style.
//
//  2. PREFIX MANGLING. A bare "." is rewritten as the target's full stop:
//     Bengali turned the prefix row into "❮। ❯", so the shown prefix was not the
//     one users must type. Fix: a bare prefix character is swapped for a
//     private-use sentinel that survives translation, then restored.
// ============================================================================

import (
	"strings"
	"unicode"
)

// trtPrefixSentinel maps a prefix character to the private-use rune that stands
// in for it while the line is being translated. These code points are unassigned
// in Unicode, so no translator produces them on its own.
var trtPrefixSentinel = map[rune]rune{
	'.': '\uE000',
	'/': '\uE001',
	'!': '\uE002',
	'#': '\uE003',
}

var trtSentinelPrefix = map[rune]rune{
	'\uE000': '.',
	'\uE001': '/',
	'\uE002': '!',
	'\uE003': '#',
}

// trtBoundaryRune reports whether r can sit next to a standalone prefix token:
// whitespace, a decorative bracket, or the menu row separators.
func trtBoundaryRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '❮', '❰', '«', '<', '❱', '❯', '»', '>', '│', '|', '*', '❨', '❩', '(', ')', ':':
		return true
	}
	return unicode.IsSpace(r)
}

// trtProtectPrefixes replaces every standalone prefix character (".", "/", "!",
// "#") with a sentinel so the translator cannot turn it into punctuation. A
// prefix is "standalone" when it is delimited by whitespace or decorative
// brackets, which is exactly how the menu shows it — a dot inside "photo.jpg" or
// "1.5" is left alone.
func trtProtectPrefixes(s string) string {
	if !strings.ContainsAny(s, "./!#") {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		sent, ok := trtPrefixSentinel[r]
		if ok {
			prevOK := i == 0 || trtBoundaryRune(rs[i-1])
			nextOK := i == len(rs)-1 || trtBoundaryRune(rs[i+1])
			if prevOK && nextOK {
				b.WriteRune(sent)
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trtRestorePrefixes puts the real prefix characters back after translation.
func trtRestorePrefixes(s string) string {
	if !strings.ContainsAny(s, "\uE000\uE001\uE002\uE003") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if orig, ok := trtSentinelPrefix[r]; ok {
			b.WriteRune(orig)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trtSoftCaps title-cases runs of two or more ASCII capitals ("UPTIME" ->
// "Uptime") so the translator recognises them as ordinary words instead of
// proper nouns. Mixed-case text is left untouched.
func trtSoftCaps(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); {
		if !isAsciiUpper(rs[i]) {
			b.WriteRune(rs[i])
			i++
			continue
		}
		j := i
		for j < len(rs) && isAsciiUpper(rs[j]) {
			j++
		}
		// Only a run of 2+ capitals is a house-style label; a lone capital (the
		// "I" in "I AM") reads as normal prose already.
		if j-i >= 2 {
			b.WriteRune(rs[i])
			for k := i + 1; k < j; k++ {
				b.WriteRune(unicode.ToLower(rs[k]))
			}
		} else {
			b.WriteRune(rs[i])
		}
		i = j
	}
	return b.String()
}

// trtAllCaps reports whether s is written in the bot's caps house style: it has
// at least one letter and no lowercase letters. The translated line is put back
// in caps when this is true, so a French or Spanish bot keeps the same look.
func trtAllCaps(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

func isAsciiUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
