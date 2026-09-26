package goldcmds

// ============================================================================
// GOLD-MD — URL PROTECTION (links must survive translation byte-for-byte)
// File: urlprotect.go
// ============================================================================
// BUG (owner report): "kisi b cmnd k sath link lagao to wo b language me
// translate ho jate hai error aa jate". When a user sends a command WITH a link
// (".fb <link>", ".tt <link>", ...) or the bot itself emits a link (from an API,
// a downloader, a wa.me / t.me invite), the whole reply — link included — was
// handed to Google Translate. Google rewrites the URL (drops "https://", swaps
// "." for the target full stop, localises digits, inserts spaces), so the link
// came back broken and the command errored.
//
// FIX: every URL is swapped for a private-use SENTINEL before the line is
// translated, then put back byte-for-byte afterwards — exactly the trick already
// used for command tokens. The URL is therefore never seen by the translator and
// is never touched by digit localisation (LocalizeDigits skips URL spans too).
//
// This covers BOTH directions the owner listed:
//   • a link the USER typed in a command, and
//   • a link the BOT sends on its own (API / downloader / invite).
// because every outgoing reply flows through TranslatePreservingCommandTokens.
// ============================================================================

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// trtURLRe matches a URL that must survive translation untouched. It covers the
// schemes and the bare hosts the bot actually emits: full http(s) links, "www."
// hosts, and the WhatsApp / Telegram short links (wa.me, t.me, chat.whatsapp.com,
// api.whatsapp.com). A bare "example.com" host is caught by trtBareHostRe below.
var trtURLRe = regexp.MustCompile(`(?i)(?:https?://|www\.|wa\.me/|t\.me/|chat\.whatsapp\.com/|api\.whatsapp\.com/)[^\s]+`)

// trtBareHostRe matches a scheme-less host with a common TLD ("example.com",
// "site.pk/download"). It requires at least one letter in the label so version
// strings like "1.5" are never mistaken for a link.
var trtBareHostRe = regexp.MustCompile(`(?i)\b[a-z][a-z0-9-]{0,62}\.(?:com|net|org|io|me|co|link|app|xyz|info|pk|in|us|uk|ru|de|fr|to|cc|tv|site|online|store|shop|top|club|live|dev|ai|gg|be|ly|gl|gd|ws|biz|pro|name|mobi|edu|gov)(?:/[^\s]*)?`)

// trtURLSentinelBase is the first private-use rune used to stand in for a URL
// while its line is translated. One rune per URL, so URL #k becomes
// trtURLSentinelBase+k. The range sits between the prefix sentinels (\uE000) and
// the command-token sentinels (\uE200), so the three never collide.
const trtURLSentinelBase = '\uE100'

// trtURLTailRune reports whether r is trailing punctuation that a sentence (or a
// decorative menu bracket) glues onto a URL and that must NOT be treated as part
// of it: "see https://x.com." or "❮ https://x.com ❯".
func trtURLTailRune(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', ')', ']', '}', '"', '\'', '>', '*',
		'\u276f', '\u276e', '\u00bb', '\u00ab', '\u2502', '|':
		return true
	}
	return false
}

// trtTrimURLTail drops trailing punctuation from a matched URL span.
func trtTrimURLTail(s string) string {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if trtURLTailRune(r) {
			s = s[:len(s)-size]
			continue
		}
		break
	}
	return s
}

// trtURLSpans returns the [start,end) byte spans of every URL in a line, sorted
// and non-overlapping. Scheme URLs win over bare-host matches, and a bare-host
// match already covered by a scheme URL is skipped.
func trtURLSpans(line string) [][2]int {
	var spans [][2]int
	for _, m := range trtURLRe.FindAllStringIndex(line, -1) {
		if t := trtTrimURLTail(line[m[0]:m[1]]); t != "" {
			spans = append(spans, [2]int{m[0], m[0] + len(t)})
		}
	}
	for _, m := range trtBareHostRe.FindAllStringIndex(line, -1) {
		// A bare host must not start right after a letter/digit (it would be the
		// tail of a longer token, e.g. the "x.com" in "mailx.com").
		if m[0] > 0 && isWordByte(line[m[0]-1]) {
			continue
		}
		t := trtTrimURLTail(line[m[0]:m[1]])
		if t == "" {
			continue
		}
		end := m[0] + len(t)
		// Trim BEFORE the coverage test: the raw match may carry a trailing dot
		// that pushes it past the scheme span it sits inside.
		if trtSpanCovered(spans, m[0], end) {
			continue
		}
		spans = append(spans, [2]int{m[0], end})
	}
	sortSpans(spans)
	return spans
}

// sortSpans sorts spans by start byte.
func sortSpans(spans [][2]int) {
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j-1][0] > spans[j][0]; j-- {
			spans[j-1], spans[j] = spans[j], spans[j-1]
		}
	}
}

// trtProtectURLs swaps every URL in a line for a sentinel rune and returns the
// rewritten line plus the URLs in order.
func trtProtectURLs(line string) (string, []string) {
	spans := trtURLSpans(line)
	if len(spans) == 0 {
		return line, nil
	}
	var b strings.Builder
	b.Grow(len(line))
	urls := make([]string, 0, len(spans))
	last := 0
	for _, sp := range spans {
		b.WriteString(line[last:sp[0]])
		urls = append(urls, line[sp[0]:sp[1]])
		b.WriteRune(trtURLSentinelBase + rune(len(urls)-1))
		last = sp[1]
	}
	b.WriteString(line[last:])
	return b.String(), urls
}

// trtRestoreURLs puts the URLs back where their sentinels landed. ok is false
// when a sentinel is missing from the response (the translator dropped it), so
// the caller can keep the original line instead of losing a link.
func trtRestoreURLs(s string, urls []string) (string, bool) {
	if len(urls) == 0 {
		return s, true
	}
	seen := make([]bool, len(urls))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= trtURLSentinelBase && int(r-trtURLSentinelBase) < len(urls) {
			k := int(r - trtURLSentinelBase)
			b.WriteString(urls[k])
			seen[k] = true
			continue
		}
		b.WriteRune(r)
	}
	for _, ok := range seen {
		if !ok {
			return s, false
		}
	}
	return b.String(), true
}
