package goldcmds

// ============================================================================
// GOLD-MD — TOKEN-SAFE REPLY TRANSLATION
// File: menutrans.go
// ============================================================================
// BUG (fixed here): every reply — menus and .botlanguage lists included — was
// sent whole to Google Translate. That endpoint DESTROYS command tokens inside
// decorative brackets: "❰ .BOTPIC ❱ CHANGE BOT PIC" came back as just "❰".
// The menu therefore lost every command it advertised, so users could not type
// what they could not see.
//
// FIX: a line carrying a ".command" token is passed through VERBATIM, so the
// token the user types is always the token the menu showed. Lines without a
// token (headers, guidance, plain replies) are translated as before, in ONE
// batched request, so there is no extra HTTP cost.
// ============================================================================

import (
	"context"
	"regexp"
	"strings"
)

// bracketTokenRe captures the content inside the decorative brackets every menu
// row uses, e.g. the ".BOTPIC" in "❰ .BOTPIC ❱".
var bracketTokenRe = regexp.MustCompile(`[❰❮«<]\s*([^❱❯»>]{1,80}?)\s*[❱❯»>]`)

// prefixTokenRe matches a bare prefix token (".botpic", "/menu", "!ping").
var prefixTokenRe = regexp.MustCompile(`(?i)[./!#]([a-z][a-z0-9_]{0,30})`)

// clKnownCommandNames returns every registered command name (visible and hidden
// alias) plus the core names the main package knows about.
func clKnownCommandNames() map[string]bool {
	known := map[string]bool{}
	for _, c := range Commands() {
		if n := strings.ToLower(strings.TrimSpace(c.Name)); n != "" {
			known[n] = true
		}
	}
	if cmdNameKnownHook != nil {
		for _, n := range cmdNameKnownHook() {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				known[n] = true
			}
		}
	}
	return known
}

// LineHasCommandToken reports whether a line carries a typeable command token.
// Detection is deliberately conservative: a bracketed token always counts (that
// is how menus advertise commands), while a bare ".word" only counts when the
// word is a registered command. Without that check prose like "e.g." or
// "photo.jpg" would be mistaken for a command and left untranslated.
func LineHasCommandToken(line string) bool {
	known := clKnownCommandNames()
	for _, seg := range bracketTokenRe.FindAllStringSubmatch(line, -1) {
		if tokenSegment(seg[1], known) {
			return true
		}
	}
	for _, m := range prefixTokenRe.FindAllStringSubmatchIndex(line, -1) {
		// Reject when the prefix is preceded by a letter or digit, so ".jpg" in
		// "photo.jpg" never matches.
		if m[0] > 0 && isWordByte(line[m[0]-1]) {
			continue
		}
		if known[strings.ToLower(line[m[2]:m[3]])] {
			return true
		}
	}
	return false
}

// tokenSegment decides whether the text inside a bracket pair is a command token
// rather than ordinary prose or a URL. A single short word (BOTPIC) counts, as
// does anything containing a resolvable ".command"; "https://x/photo.jpg" and
// "BOT IS ONLINE 🔰" do not.
func tokenSegment(seg string, known map[string]bool) bool {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return false
	}
	if !strings.ContainsAny(seg, " \t") {
		letters, ok := true, false
		for _, r := range seg {
			if r == '_' || (r >= '0' && r <= '9') {
				continue
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				ok = true
				continue
			}
			if r == '.' || r == '/' || r == '!' || r == '#' {
				continue
			}
			letters = false
			break
		}
		if letters && ok {
			return true
		}
	}
	for _, m := range prefixTokenRe.FindAllStringSubmatchIndex(seg, -1) {
		if m[0] > 0 && isWordByte(seg[m[0]-1]) {
			continue
		}
		if known[strings.ToLower(seg[m[2]:m[3]])] {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TranslatePreservingCommandTokens translates text into lang while leaving every
// line that carries a command token exactly as it was. Returns the original text
// (and nil error) whenever the translation cannot be applied safely, so a reply
// is never mangled.
func TranslatePreservingCommandTokens(ctx context.Context, text, lang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	idx := make([]int, 0, len(lines))
	prefix := make([]string, len(lines))
	batch := make([]string, 0, len(lines))
	for i, ln := range lines {
		// A line that advertises a command is split: the bracketed token is kept
		// verbatim (that is the text the user must type) while the description after
		// it is still translated.
		if head, tail, ok := splitTokenLine(ln); ok {
			if strings.TrimSpace(tail) == "" {
				continue
			}
			prefix[i] = head
			idx = append(idx, i)
			batch = append(batch, tail)
			continue
		}
		// Blank lines are kept verbatim: sending them makes translators collapse
		// the run, which trips the line-count guard and reverts the whole reply to
		// English. Skipping them keeps the batch aligned with its output.
		if strings.TrimSpace(ln) == "" {
			continue
		}
		idx = append(idx, i)
		batch = append(batch, ln)
	}
	if len(batch) == 0 {
		return text, nil
	}
	tr := clTranslator
	if tr == nil {
		tr = TranslateText
	}
	out, err := tr(ctx, strings.Join(batch, "\n"), lang)
	if err != nil {
		return text, err
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(batch) {
		// Line-count drift means we cannot map translations back onto the original
		// lines; keeping the source text is safer than scrambling the menu.
		return text, nil
	}
	for n, i := range idx {
		lines[i] = prefix[i] + got[n]
	}
	return strings.Join(lines, "\n"), nil
}

// splitTokenLine splits a line at the end of its last command token, returning
// the verbatim head (through the closing bracket) and the translatable tail.
// ok is false when the line carries no command token.
func splitTokenLine(line string) (head, tail string, ok bool) {
	known := clKnownCommandNames()
	end := -1
	for _, m := range bracketTokenRe.FindAllStringSubmatchIndex(line, -1) {
		if tokenSegment(line[m[2]:m[3]], known) {
			end = m[1]
		}
	}
	if end < 0 {
		return "", "", false
	}
	return line[:end], line[end:], true
}
