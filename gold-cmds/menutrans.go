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
	"strconv"
	"strings"
	"unicode"
)

// bracketTokenRe captures the content inside the decorative brackets every menu
// row uses, e.g. the ".BOTPIC" in "❰ .BOTPIC ❱".
var bracketTokenRe = regexp.MustCompile(`[❰❮«<]\s*([^❱❯»>]{1,80}?)\s*[❱❯»>]`)

// prefixTokenRe matches a bare prefix token (".botpic", "/menu", "!ping").
var prefixTokenRe = regexp.MustCompile(`(?i)[./!#]([a-z][a-z0-9_]{0,30})`)

// clMenuTokenHook lets the main package advertise typeable menu tokens that are
// not registered commands (category shortcuts like .core / .group). It is
// deliberately separate from cmdNameKnownHook: that hook feeds .cmdname /
// .cmdprefix validation, where a category slug must NOT become a renamable
// command. This one is used only for translation token protection.
var clMenuTokenHook func() []string

// CmdNameAttachMenuTokens registers the main package's menu-only token supplier.
func CmdNameAttachMenuTokens(fn func() []string) { clMenuTokenHook = fn }

// clKnownCommandNames returns every registered command name (visible and hidden
// alias) plus the core names the main package knows about.
func clKnownCommandNames() map[string]bool {
	known := map[string]bool{}
	for _, c := range Commands() {
		if n := strings.ToLower(strings.TrimSpace(c.Name)); n != "" {
			known[n] = true
		}
	}
	// logo1..logo1000 are registered by the main package, so they are absent from
	// the plugin registry above and from the hook in gold-cmds-only tests. They
	// are generated here from LogoCount so ".logo5" is protected everywhere.
	for n := 1; n <= LogoCount; n++ {
		known["logo"+strconv.Itoa(n)] = true
	}
	if cmdNameKnownHook != nil {
		for _, n := range cmdNameKnownHook() {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				known[n] = true
			}
		}
	}
	// Menu-advertised tokens that are NOT registered commands. The category
	// shortcuts (.CORE / .GROUP / .PROTECTION / .AI / .UTILITY / ...) are
	// dispatched by the main package's categoryMenuShortcut, so they appear in
	// neither Commands() nor the registered-name hook. Missing them meant the
	// menu printed ".کور" / ".گروپ" — a token no user can type. This is
	// language-independent: the token was translated in EVERY language, while
	// registered siblings (.EQUALIZER / .FONT / .GAME) stayed ASCII.
	if clMenuTokenHook != nil {
		for _, n := range clMenuTokenHook() {
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

// trtChunkBytes bounds one batched translation request. Google's free web
// endpoint rejects large GETs with HTTP 413, which used to revert a WHOLE menu to
// English (owner report: "kuch cmnds ke texts ki ho rhe, kch English reh jate").
// Lines are therefore sent in several requests, each under this budget.
const trtChunkBytes = 1200

// chunkLines groups lines so that no single batch exceeds trtChunkBytes.
func chunkLines(lines []string) [][]string {
	var chunks [][]string
	var cur []string
	size := 0
	for _, ln := range lines {
		n := len(ln) + 1
		if size+n > trtChunkBytes && len(cur) > 0 {
			chunks = append(chunks, cur)
			cur, size = nil, 0
		}
		cur = append(cur, ln)
		size += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// TranslatePreservingCommandTokens translates text into lang while leaving every
// line that carries a command token exactly as it was. A line that cannot be
// translated keeps its ORIGINAL text, so one bad line never blanks a reply.
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
			prefix[i] = head
			// A tail of pure decoration ("*", "❯") carries nothing to translate and
			// would only risk mangling or a line-count drift.
			if !trtHasLetters(tail) {
				continue
			}
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

	// Cache first (owner order: bar bar translate na ho). Only the lines that
	// miss are sent to the translator, so a warm cache costs zero HTTP calls and
	// a cold one costs one request per chunk.
	botJID := tcBotFromCtx(ctx)
	miss := make([]int, 0, len(batch))
	missText := make([]string, 0, len(batch))
	hit := make([]string, len(batch))
	if tcGet != nil && botJID != "" {
		for n, ln := range batch {
			if v, ok := tcGet(botJID, lang, ln); ok {
				hit[n] = v
				continue
			}
			miss = append(miss, n)
			missText = append(missText, ln)
		}
	} else {
		for n := range batch {
			miss = append(miss, n)
			missText = append(missText, batch[n])
		}
	}
	if len(miss) > 0 {
		// Two free-endpoint quirks are neutralised here, before the request:
		// ALL-CAPS labels are echoed back untranslated, and a bare "." prefix is
		// rewritten as the target's full stop. See trtsoft.go.
		prep := make([]string, len(missText))
		for n, src := range missText {
			prep[n] = trtProtectPrefixes(trtSoftCaps(src))
		}
		// Chunked so a big menu never trips Google's 413. A chunk that fails (or
		// comes back with a different line count) falls back to per-line requests,
		// and a single line that still fails keeps its ORIGINAL text.
		tr := clTranslator
		if tr == nil {
			tr = TranslateText
		}
		at := 0
		for _, chunk := range chunkLines(prep) {
			got, ok := trtTranslateChunk(ctx, tr, chunk, lang)
			if !ok {
				for _, src := range chunk {
					if v, err := tr(ctx, src, lang); err == nil && strings.TrimSpace(v) != "" {
						got = append(got, v)
					} else {
						got = append(got, src)
					}
				}
			}
			for n, v := range got {
				src := missText[at+n]
				if strings.TrimSpace(v) == "" {
					v = src
				} else {
					// Restore the real prefix, then put the caps house style back so
					// the translated label looks like the rest of the menu.
					v = trtRestorePrefixes(v)
					if trtAllCaps(src) {
						v = strings.ToUpper(v)
					}
					// Translators strip the leading space of a tail, which glued the
					// description to the token (".LOGO5❮ آپ کا نام ❯"). Put the
					// original spacing back so the row keeps its shape.
					if prefix[miss[at+n]] != "" {
						if lead := trtLeadingSpace(src); lead != "" && !strings.HasPrefix(v, lead) {
							v = lead + strings.TrimLeft(v, " \t")
						}
					}
				}
				hit[miss[at+n]] = v
				if tcPut != nil && botJID != "" && v != src {
					tcPut(botJID, lang, src, v)
				}
			}
			at += len(chunk)
		}
	}
	for n, i := range idx {
		// Digits are localised on the TRANSLATED part only. The verbatim token head
		// (prefix[i]) keeps ASCII digits, so ".logo1000" is still typed as-is
		// (owner order: "yeh 0123 numbers b usy zaban usy country k numbers me").
		lines[i] = prefix[i] + LocalizeDigits(lang, hit[n])
	}
	return strings.Join(lines, "\n"), nil
}

// trtTranslateChunk translates one batch and reports whether the result maps
// back onto the input line-for-line. A false return means the caller must fall
// back (per-line), never that the reply should stay English.
func trtTranslateChunk(ctx context.Context, tr func(context.Context, string, string) (string, error), chunk []string, lang string) ([]string, bool) {
	out, err := tr(ctx, strings.Join(chunk, "\n"), lang)
	if err != nil {
		return nil, false
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(chunk) {
		return nil, false
	}
	return got, true
}

// splitTokenLine splits a line at the end of its last command token, returning
// the verbatim head (through the closing bracket) and the translatable tail.
// ok is false when the line carries no command token.
//
// Both shapes a menu uses are covered: a BRACKETED token ("❰ .BOTPIC ❱ CHANGE
// BOT PIC") and a BARE token ("| 🔰 | .LOGO5 ❮ YOUR NAME ❯"). Missing the bare
// shape sent ".LOGO5" and ".BOTVIDEO" through the translator, which returned
// "لوگو۵" / ".بوٹویڈیو" — a token the user could never type.
func splitTokenLine(line string) (head, tail string, ok bool) {
	known := clKnownCommandNames()
	end := -1
	for _, m := range bracketTokenRe.FindAllStringSubmatchIndex(line, -1) {
		if tokenSegment(line[m[2]:m[3]], known) {
			end = m[1]
		}
	}
	for _, m := range prefixTokenRe.FindAllStringSubmatchIndex(line, -1) {
		// Same guard as LineHasCommandToken: ".jpg" in "photo.jpg" is not a token.
		if m[0] > 0 && isWordByte(line[m[0]-1]) {
			continue
		}
		if known[strings.ToLower(line[m[2]:m[3]])] && m[1] > end {
			end = m[1]
		}
	}
	if end < 0 {
		return "", "", false
	}
	return line[:end], line[end:], true
}

// trtHasLetters reports whether s carries anything a translator could translate.
// A tail of pure decoration ("*", "❯", "─━─") must not be sent: it would come
// back mangled and could drift the batch's line count.
func trtHasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// trtLeadingSpace returns the leading whitespace run of s. Translators trim it
// from each line, so it is restored when a split line is put back together.
func trtLeadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}
