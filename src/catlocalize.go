package main

import (
	"strings"

	goldcmds "gold-md/gold-cmds"
)

// ============================================================================
// GOLD-MD — LOCALIZED .menu CATEGORY NAMES
// File: catlocalize.go
// ============================================================================
// OWNER ORDER: jab bot ki language set ho to .menu ki CATEGORY list bhi poori
// us language me ho (.CORE -> .بنیادی, .GROUP -> .گروپ, ...). Ye list translation
// se PEHLE English rehti hai (category tokens protected hote hain, taake chunk
// pure English rahe aur baaki prose theek translate ho), aur translation ke BAAD
// har token ko uske localized naam se badal diya jata hai.
//
// Localized naam khud ek WORKING alias hai: category slugs localized command-name
// set (CmdLocalizeResolve) ka hissa hain, is liye ".بنیادی" type karne par bhi
// CORE menu khulta hai. English token (.CORE) hamesha chalta rehta hai.
// ============================================================================

// localizeCategoryTokens swaps each English category shortcut in an already
// translated reply (.CORE -> .بنیادی, .GROUP -> .گروپ, ...) for its localized
// form. It is a no-op when no language is set or the localized names are not
// prepared yet (the English token is then shown, exactly as before).
func (s *Session) localizeCategoryTokens(text string) string {
	if s == nil || s.Manager == nil || s.Manager.Redis == nil || text == "" {
		return text
	}
	br := &bridge{s: s}
	prefix := s.resolvePrefix(s.JID)
	for _, slug := range menuCategorySlugs {
		loc := goldcmds.CmdLocalizeLookup(br, slug)
		if loc == "" || strings.EqualFold(loc, slug) {
			continue
		}
		text = replaceCategoryToken(text, prefix, slug, loc)
	}
	// .LOGO is shown in the same list and is a registered command.
	if loc := goldcmds.CmdLocalizeLookup(br, "logo"); loc != "" && !strings.EqualFold(loc, "logo") {
		text = replaceCategoryToken(text, prefix, "logo", loc)
	}
	return text
}

// replaceCategoryToken replaces every case-insensitive "<prefix><slug>" token in
// text with "<prefix><loc>". The token must be delimited — the byte before the
// prefix and the byte after the slug are not word bytes — so "photo.core" and
// ".corex" are left alone.
func replaceCategoryToken(text, prefix, slug, loc string) string {
	needle := prefix + slug
	if needle == "" || loc == "" {
		return text
	}
	lower := strings.ToLower(text)
	needleLower := strings.ToLower(needle)
	var b strings.Builder
	b.Grow(len(text) + 8)
	i := 0
	for i < len(text) {
		j := strings.Index(lower[i:], needleLower)
		if j < 0 {
			b.WriteString(text[i:])
			break
		}
		j += i
		end := j + len(needle)
		beforeOK := j == 0 || !catWordByte(text[j-1])
		afterOK := end >= len(text) || !catWordByte(text[end])
		if !beforeOK || !afterOK {
			b.WriteString(text[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(text[i:j])
		b.WriteString(prefix + loc)
		i = end
	}
	return b.String()
}

func catWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
