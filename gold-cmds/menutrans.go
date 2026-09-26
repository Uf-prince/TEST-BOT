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
// FIX: each command token is swapped for a private-use SENTINEL, the whole line
// is translated, then the token is put back where its sentinel landed. So the
// token the user types is always the token the menu showed, AND the text on BOTH
// sides of it is translated. (An earlier split "verbatim head + translated tail"
// left everything before a token in English — "EXAMPLE :❱ .tt carti" kept
// "EXAMPLE" — which is the "adhe English adhe Urdu" bug the owner reported for
// EVERY command.)
// ============================================================================

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// bracketTokenRe captures the content inside the decorative brackets every menu
// row uses, e.g. the ".BOTPIC" in "❰ .BOTPIC ❱".
var bracketTokenRe = regexp.MustCompile(`[❰❮«<]\s*([^❱❯»>]{1,80}?)\s*[❱❯»>]`)

// prefixTokenRe matches a bare prefix token (".botpic", "/menu", "!ping").
var prefixTokenRe = regexp.MustCompile(`(?i)[./!#]([a-z][a-z0-9_]{0,30})`)

// trtDefaultPrefix is the bot's default command prefix. It is used when no
// prefix is threaded through the context (unit tests, prewarm) so token
// detection matches the shipped "." prefix.
const trtDefaultPrefix = "."

// trtPrefixCtxKey carries the bot's ACTIVE command prefix through the
// translation call. In no-prefix mode (".prefix null") it is "" and a bare
// command name is a typeable token; otherwise a bare word in brackets is a
// placeholder ("QUERY" / "LINK") and must be translated.
type trtPrefixCtxKeyT struct{}

var trtPrefixCtxKey trtPrefixCtxKeyT

// TrtWithPrefix tags a context with the bot's active command prefix.
func TrtWithPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, trtPrefixCtxKey, prefix)
}

func trtPrefixFromCtx(ctx context.Context) string {
	if ctx != nil {
		if v, ok := ctx.Value(trtPrefixCtxKey).(string); ok {
			return v
		}
	}
	return trtDefaultPrefix
}

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
		if tokenSegment(seg[1], known, trtDefaultPrefix) {
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
// rather than ordinary prose or a URL. A token is either a resolvable ".command"
// or a BARE registered command name ("❰ BOTPIC ❱"); "https://x/photo.jpg" and
// "BOT IS ONLINE 🔰" are not.
//
// The bare-word branch used to accept ANY single word, so placeholders like
// "QUERY" / "LINK" / "YOUR NAME" inside the guide brackets were treated as
// commands and left in English forever (owner report: ".tt guide adha English").
// Requiring a REGISTERED name fixes every guide in every language.
func tokenSegment(seg string, known map[string]bool, prefix string) bool {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return false
	}
	// A resolvable ".command" anywhere inside the brackets.
	for _, m := range prefixTokenRe.FindAllStringSubmatchIndex(seg, -1) {
		if m[0] > 0 && isWordByte(seg[m[0]-1]) {
			continue
		}
		if known[strings.ToLower(seg[m[2]:m[3]])] {
			return true
		}
	}
	// No-prefix mode: a bare registered command name ("❰ BOTPIC ❱") is typeable.
	// With a real prefix (".") a bare word is a PLACEHOLDER ("❰ QUERY ❱",
	// "❰ LINK ❱") and MUST be translated — "link" is also a hidden command, so
	// matching bare words regardless of prefix left every guide half-English.
	if prefix == "" && !strings.ContainsAny(seg, " \t") && known[strings.ToLower(seg)] {
		return true
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
// command token byte-for-byte typeable. It REPLACES each token span with a
// private-use sentinel, translates the WHOLE line, then puts the tokens back —
// so the text BEFORE and AFTER a token is translated too.
//
// The previous design split each token line into a verbatim "head" (everything
// up to the token) and a translated "tail" (everything after). That left the
// text before a token in English forever: "EXAMPLE :❱ .tt carti" kept "EXAMPLE"
// because it sat in the head. Every guide (.tt / .fb / .ig / .tg / .twt / .apk)
// and the menu's "IMPORTANT CMNDS" block showed the same half-translated result
// (owner report: "adhe texts translate ho rhe adhe English reh jate").
//
// A line that cannot be translated keeps its ORIGINAL text, so one bad line
// never blanks a reply.
func TranslatePreservingCommandTokens(ctx context.Context, text, lang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	prefix := trtPrefixFromCtx(ctx)
	idx := make([]int, 0, len(lines))    // line indices that need translation
	prot := make([]string, len(lines))   // line with tokens swapped for sentinels
	toks := make([][]string, len(lines)) // the token spans, in order
	batch := make([]string, 0, len(lines))
	for i, ln := range lines {
		// Blank lines are kept verbatim: sending them makes translators collapse
		// the run, which trips the line-count guard and reverts the whole reply to
		// English. Skipping them keeps the batch aligned with its output.
		if strings.TrimSpace(ln) == "" {
			continue
		}
		p, tk := trtProtectTokenSpans(ln, prefix)
		// A line with NO letters carries nothing to translate — it is pure
		// box-drawing / symbols / digits (a menu border, a bare count row) or a
		// token-only row ("*| 🔰 | .CORE*"). Sending it anyway let Google rewrite
		// the decoration into garbage. Keep such lines verbatim.
		if !trtHasLetters(p) {
			continue
		}
		prot[i] = p
		toks[i] = tk
		idx = append(idx, i)
		batch = append(batch, p)
	}
	if len(batch) == 0 {
		return LocalizeDigits(lang, text), nil
	}

	// Cache first (owner order: bar bar translate na ho). The key is the ORIGINAL
	// line so the same header/description is translated only once per bot+lang.
	botJID := tcBotFromCtx(ctx)
	hit := make([]string, len(batch))
	miss := make([]int, 0, len(batch))
	if tcGet != nil && botJID != "" {
		for n, i := range idx {
			if v, ok := tcGet(botJID, lang, lines[i]); ok {
				hit[n] = v
				continue
			}
			miss = append(miss, n)
		}
	} else {
		for n := range batch {
			miss = append(miss, n)
		}
	}
	if len(miss) > 0 {
		// Neutralise the free-endpoint quirks before the request: ALL-CAPS labels
		// are echoed back untranslated, a bare "." prefix is rewritten as the
		// target's full stop, and house shorthand ("CMNDS", "02H 14M") is not a
		// dictionary word. See trtsoft.go.
		prep := make([]string, len(miss))
		for k, n := range miss {
			prep[k] = trtProtectPrefixes(trtSoftCaps(trtExpandHouseTokens(batch[n])))
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
				got = got[:0]
				for _, src := range chunk {
					if v, err := tr(ctx, src, lang); err == nil && strings.TrimSpace(v) != "" {
						got = append(got, v)
					} else {
						got = append(got, src)
					}
				}
			}
			for k, v := range got {
				n := miss[at+k]
				i := idx[n]
				orig := lines[i]
				v = trtFinishLine(v, orig, toks[i])
				hit[n] = v
				if tcPut != nil && botJID != "" && v != orig {
					tcPut(botJID, lang, orig, v)
				}
			}
			at += len(chunk)
		}
	}
	for n, i := range idx {
		lines[i] = hit[n]
	}
	// Localise EVERY number in the reply, not only the translated lines (owner
	// order: "hr number usy country language me"). LocalizeDigits protects
	// command-token digits, so ".logo1000" still shows ASCII and stays typeable.
	return LocalizeDigits(lang, strings.Join(lines, "\n")), nil
}

// trtFinishLine turns one raw translator response back into the final line: it
// restores the prefix sentinels and the command tokens, and re-applies the bot's
// caps house style. When the translator dropped a token sentinel (rare) it
// returns the ORIGINAL line, so a token is never lost.
func trtFinishLine(v, orig string, toks []string) string {
	if strings.TrimSpace(v) == "" {
		return orig
	}
	v = trtRestorePrefixes(v)
	restored, ok := trtRestoreTokenSpans(v, toks)
	if !ok {
		return orig
	}
	// Translators trim each line's leading space; put it back so a menu row keeps
	// its shape.
	if lead := trtLeadingSpace(orig); lead != "" {
		restored = lead + strings.TrimLeft(restored, " \t")
	}
	if trtAllCaps(orig) {
		restored = strings.ToUpper(restored)
	}
	return restored
}

// trtTokenSentinelBase is the first private-use rune used to stand in for a
// command token while its line is translated. One rune per token, so token #k
// becomes trtTokenSentinelBase+k. These code points are unassigned in Unicode,
// so no translator produces them on its own.
const trtTokenSentinelBase = '\uE200'

// trtTokenSpans returns the [start,end) byte spans of every command token in a
// line, sorted and non-overlapping. A bracketed token protects the WHOLE bracket
// pair (decoration + token); a bare token protects just the token.
func trtTokenSpans(line, prefix string) [][2]int {
	known := clKnownCommandNames()
	var spans [][2]int
	for _, m := range bracketTokenRe.FindAllStringSubmatchIndex(line, -1) {
		if tokenSegment(line[m[2]:m[3]], known, prefix) {
			spans = append(spans, [2]int{m[0], m[1]})
		}
	}
	for _, m := range prefixTokenRe.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > 0 && isWordByte(line[m[0]-1]) {
			continue
		}
		if !known[strings.ToLower(line[m[2]:m[3]])] {
			continue
		}
		if trtSpanCovered(spans, m[0], m[1]) {
			continue
		}
		spans = append(spans, [2]int{m[0], m[1]})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	return spans
}

// trtSpanCovered reports whether [a,b) is already inside one of spans.
func trtSpanCovered(spans [][2]int, a, b int) bool {
	for _, s := range spans {
		if a >= s[0] && b <= s[1] {
			return true
		}
	}
	return false
}

// trtProtectTokenSpans swaps every command token in a line for a sentinel rune
// and returns the rewritten line plus the tokens in order.
func trtProtectTokenSpans(line, prefix string) (string, []string) {
	spans := trtTokenSpans(line, prefix)
	if len(spans) == 0 {
		return line, nil
	}
	var b strings.Builder
	b.Grow(len(line))
	tokens := make([]string, 0, len(spans))
	last := 0
	for _, sp := range spans {
		b.WriteString(line[last:sp[0]])
		tokens = append(tokens, line[sp[0]:sp[1]])
		b.WriteRune(trtTokenSentinelBase + rune(len(tokens)-1))
		last = sp[1]
	}
	b.WriteString(line[last:])
	return b.String(), tokens
}

// trtRestoreTokenSpans puts the tokens back where their sentinels landed. ok is
// false when a sentinel is missing from the response (the translator dropped
// it), so the caller can keep the original line instead of losing a token.
func trtRestoreTokenSpans(s string, tokens []string) (string, bool) {
	if len(tokens) == 0 {
		return s, true
	}
	seen := make([]bool, len(tokens))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= trtTokenSentinelBase && int(r-trtTokenSentinelBase) < len(tokens) {
			k := int(r - trtTokenSentinelBase)
			b.WriteString(tokens[k])
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
		if tokenSegment(line[m[2]:m[3]], known, trtDefaultPrefix) {
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
