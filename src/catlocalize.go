package main

import (
	"strings"

	goldcmds "gold-md/gold-cmds"
)

// ============================================================================
// GOLD-MD — LOCALIZED MENU COMMAND NAMES
// File: catlocalize.go
// ============================================================================
// OWNER ORDER: jab bot ki language set ho to menu ke COMMAND NAMES bhi us
// language me dikhne chahiye — sirf header/description nahi. Jaise:
//   .FONT1   -> .فونٹ1
//   .LOGO1   -> .لوگو1
//   .ROLLDICE-> .رولڈائس
//   .CORE    -> .بنیادی   (category shortcut)
//   .GROUP   -> .گروپ
//
// YE KAISE HOTA HAI:
//   * Menu pehle English me banta hai (command tokens English rehte hain).
//   * Poora menu translate hota hai — command tokens PROTECTED hote hain, is
//     liye wo translation ke baad bhi English rehte hain (taake chunk saaf rahe
//     aur prose theek translate ho).
//   * Translation ke BAAD har command token ko uske LOCALIZED naam se badal
//     diya jata hai (yahi file).
//
// Localized naam khud ek WORKING alias hai: ye command-name localization set
// (CmdLocalizeResolve) ka hissa hain, is liye ".فونٹ1" type karne par bhi wahi
// command chalti hai. English token (.FONT1) hamesha chalta rehta hai.
// ============================================================================

// localizeCommandTokens swaps every English command token in an already
// translated reply (.FONT1 -> .فونٹ1, .CORE -> .بنیادی, .ROLLDICE -> .رولڈائس,
// ...) for its localized form. It is a no-op when no language is set or the
// localized names are not prepared yet (the English token is then shown,
// exactly as before).
func (s *Session) localizeCommandTokens(text string) string {
	if s == nil || s.Manager == nil || s.Manager.Redis == nil || text == "" {
		return text
	}
	br := &bridge{s: s}
	pairs := goldcmds.LocalizedCommandList(br)
	if len(pairs) == 0 {
		return text
	}
	byCanon := make(map[string]string, len(pairs))
	for _, p := range pairs {
		byCanon[p[0]] = p[1]
	}
	prefix := s.resolvePrefix(s.JID)
	return replaceCommandTokens(text, prefix, byCanon)
}

// replaceCommandTokens scans text for "<prefix><word>" tokens and replaces each
// one whose (lower-cased) word is a known canonical command with a localized
// form. A token must be delimited — the byte before the prefix and the byte
// after the word are not word bytes — so "photo.font1" and ".font1x" are left
// alone. The localized name is shown as-is; a trailing "word number" split
// (".فونٹ 1") is collapsed to ".فونٹ1" so the token stays a single typeable
// word.
func replaceCommandTokens(text, prefix string, byCanon map[string]string) string {
	if prefix == "" || len(byCanon) == 0 || text == "" {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + 16)
	i := 0
	for i < len(text) {
		j := strings.Index(text[i:], prefix)
		if j < 0 {
			b.WriteString(text[i:])
			break
		}
		j += i
		// The prefix must sit on a word boundary (so "photo.font1" is skipped).
		if j > 0 && catWordByte(text[j-1]) {
			b.WriteString(text[i : j+len(prefix)])
			i = j + len(prefix)
			continue
		}
		// Read the word that follows the prefix.
		k := j + len(prefix)
		start := k
		for k < len(text) && catWordByte(text[k]) {
			k++
		}
		if k == start {
			// No word after the prefix (e.g. a sentence-ending period).
			b.WriteString(text[i:k])
			i = k
			continue
		}
		word := text[start:k]
		loc, ok := lookupLocalized(byCanon, strings.ToLower(word))
		if !ok || loc == "" || strings.EqualFold(loc, word) {
			b.WriteString(text[i:k])
			i = k
			continue
		}
		b.WriteString(text[i:j])
		b.WriteString(prefix)
		b.WriteString(collapseTrailingNumberSpace(loc))
		i = k
	}
	return b.String()
}

// collapseTrailingNumberSpace turns "فونٹ 1" into "فونٹ1": it removes a single
// space that separates a trailing run of ASCII digits, so a localized command
// name stays a single typeable token. Anything else (e.g. a multi-word category
// name) is returned unchanged.
func collapseTrailingNumberSpace(s string) string {
	i := strings.LastIndexByte(s, ' ')
	if i < 0 {
		return s
	}
	tail := s[i+1:]
	if tail == "" {
		return s
	}
	for _, r := range tail {
		if r < '0' || r > '9' {
			return s
		}
	}
	return s[:i] + tail
}

func catWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// replaceCategoryToken is a thin single-token wrapper kept for the existing
// tests (and any caller that wants to swap exactly one slug). It delegates to
// the general replaceCommandTokens so behaviour stays identical.
func replaceCategoryToken(text, prefix, slug, loc string) string {
	if slug == "" || loc == "" {
		return text
	}
	return replaceCommandTokens(text, prefix, map[string]string{slug: loc})
}

// lookupLocalized resolves a (lower-cased) command token to its localized form.
// It first tries the exact name (font1 -> "فونٹ 1"). When that is missing — the
// translator skipped some rows of a 1000-strong family — it falls back to the
// BASE word: "logo5" -> localized("logo") + "5". The base form must be a single
// token (no spaces) so the menu still advertises a typeable command; a
// multi-word base (e.g. the Urdu for "equalizer") is skipped, leaving the
// English token in place rather than printing an untypeable one.
func lookupLocalized(byCanon map[string]string, word string) (string, bool) {
	if loc, ok := byCanon[word]; ok && loc != "" {
		return loc, true
	}
	base, digits, has := splitTrailingDigits(word)
	if !has {
		return "", false
	}
	if alias, ok := cmdBaseAlias[base]; ok {
		base = alias
	}
	bl, ok := byCanon[base]
	if !ok || bl == "" {
		return "", false
	}
	return collapseTrailingNumberSpace(bl) + digits, true
}

// cmdBaseAlias maps a short numeric-family base to the canonical command whose
// localized name should prefix it. The equalizer rows are .EQ1..EQ1000, but the
// translator has no "eq" entry — the registered command is "equalizer" — so
// "eq" is resolved through it (eq1 -> localized("equalizer") + "1").
var cmdBaseAlias = map[string]string{
	"eq": "equalizer",
}

// splitTrailingDigits splits "font1000" into ("font", "1000", true). A word with
// no trailing digits, or one that is ALL digits, returns has=false.
func splitTrailingDigits(word string) (string, string, bool) {
	i := len(word)
	for i > 0 && word[i-1] >= '0' && word[i-1] <= '9' {
		i--
	}
	if i == 0 || i == len(word) {
		return word, "", false
	}
	return word[:i], word[i:], true
}

// isSingleToken reports whether a localized name is a single typeable word (no
// spaces or tabs), so it can be used as a command token.
func isSingleToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t")
}
