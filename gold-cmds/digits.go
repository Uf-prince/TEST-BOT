package goldcmds

// ============================================================================
// GOLD-MD — LOCAL DIGITS  (.botlanguage numbers)
// File: digits.go
// ============================================================================
// OWNER ORDER: "yeh 0123 numbers b usy zaban usy country k numbers me translate
// krwao". Google never touches digits, so a menu in Urdu showed "UPTIME :❯ 2H 5M"
// and "COMMANDS :❯ ❮ 1250 ❯" with ASCII numbers.
//
// Only the digits of languages whose script HAS its own numerals are mapped.
// Latin-script targets (English, French, Turkish, Indonesian, ...) use ASCII
// digits in their own country too, so they are left alone.
//
// Command tokens are never localised: TranslatePreservingCommandTokens keeps the
// bracketed token verbatim and only the translated part goes through
// LocalizeDigits, so ".logo1000" still types as ".logo1000".
// ============================================================================

import "strings"

// trtDigitBase maps a language code to the Unicode code point of its ZERO digit.
// The remaining digits follow consecutively, which is how every Unicode numeral
// block is laid out — so the ten runes are derived instead of hand-typed.
var trtDigitBase = map[string]rune{
	// Arabic-Indic (U+0660): ٠١٢٣٤٥٦٧٨٩
	"ar": 0x0660,

	// Extended Arabic-Indic / Perso-Arabic (U+06F0): ۰۱۲۳۴۵۶۷۸۹
	// Urdu, Persian, Pashto, Sindhi, Balochi, Sorani Kurdish, Uyghur.
	"ur": 0x06F0, "fa": 0x06F0, "ps": 0x06F0, "sd": 0x06F0,
	"bal": 0x06F0, "ckb": 0x06F0, "ug": 0x06F0,

	// Devanagari (U+0966): ०१२३४५६७८९
	"hi": 0x0966, "mr": 0x0966, "ne": 0x0966, "sa": 0x0966,
	"bho": 0x0966, "mai": 0x0966, "doi": 0x0966, "gom": 0x0966,
	"awa": 0x0966, "hne": 0x0966, "kok": 0x0966,

	// Bengali / Assamese (U+09E6): ০১২৩৪৫৬৭৮৯
	"bn": 0x09E6, "as": 0x09E6,

	// Gurmukhi (U+0A66): ੦੧੨੩੪੫੬੭੮੯
	"pa": 0x0A66,

	// Gujarati (U+0AE6): ૦૧૨૩૪૫૬૭૮૯
	"gu": 0x0AE6,

	// Odia (U+0B66): ୦୧୨୩୪୫୬୭୮୯
	"or": 0x0B66,

	// Tamil (U+0BE6): ௦௧௨௩௪௫௬௭௮௯
	"ta": 0x0BE6,

	// Telugu (U+0C66): ౦౧౨౩౪౫౬౭౮౯
	"te": 0x0C66,

	// Kannada (U+0CE6): ೦೧೨೩೪೫೬೭೮೯
	"kn": 0x0CE6,

	// Malayalam (U+0D66): ൦൧൨൩൪൫൬൭൮൯
	"ml": 0x0D66,

	// Sinhala (U+0DE6): ෦෧෨෩෪෫෬෭෮෯
	"si": 0x0DE6,

	// Thai (U+0E50): ๐๑๒๓๔๕๖๗๘๙
	"th": 0x0E50,

	// Lao (U+0ED0): ໐໑໒໓໔໕໖໗໘໙
	"lo": 0x0ED0,

	// Tibetan / Dzongkha (U+0F20): ༠༡༢༣༤༥༦༧༨༩
	"bo": 0x0F20, "dz": 0x0F20,

	// Myanmar / Burmese (U+1040): ၀၁၂၃၄၅၆၇၈၉
	"my": 0x1040,

	// Khmer (U+17E0): ០១២៣៤៥៦៧៨៩
	"km": 0x17E0,
}

// trtDigitSet returns the ten numerals for a language, derived from the block
// base. ok is false when the language uses ASCII digits (or is unknown).
func trtDigitSet(lang string) (string, bool) {
	base, ok := trtDigitBase[strings.ToLower(strings.TrimSpace(lang))]
	if !ok {
		return "", false
	}
	rs := make([]rune, 10)
	for i := range rs {
		rs[i] = base + rune(i)
	}
	return string(rs), true
}

// HasLocalDigits reports whether the language renders its own numerals.
func HasLocalDigits(lang string) bool {
	_, ok := trtDigitSet(lang)
	return ok
}

// LocalizeDigits rewrites ASCII 0-9 into the target language's own numerals.
// Text is returned unchanged for languages with ASCII digits or empty input.
//
// Digits that belong to a command token (".logo1", "/menu2", ".logo1000") are
// NEVER localised: the user must be able to type exactly what the menu showed.
// Counts and uptimes ("1250", "02H 05M") are not command tokens, so they do get
// the local numerals the owner asked for.
func LocalizeDigits(lang, text string) string {
	set, ok := trtDigitSet(lang)
	if !ok || text == "" {
		return text
	}
	rs := []rune(set)
	// Spans of command tokens, e.g. the ".logo1" in ".logo1 ❮ YOUR NAME ❯".
	skip := make([]bool, len(text))
	for _, m := range prefixTokenRe.FindAllStringIndex(text, -1) {
		if m[0] > 0 && isWordByte(text[m[0]-1]) {
			continue // part of a longer word ("photo.jpg", "1.5")
		}
		for i := m[0]; i < m[1]; i++ {
			skip[i] = true
		}
	}
	var b strings.Builder
	b.Grow(len(text) + 8)
	for i, r := range text {
		if r >= '0' && r <= '9' && !skip[i] {
			b.WriteRune(rs[r-'0'])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
