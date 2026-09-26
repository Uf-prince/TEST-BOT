package goldcmds

import "testing"

// TestLocalizeDigitsPerScript: har language ke apne numerals aayen.
func TestLocalizeDigitsPerScript(t *testing.T) {
	cases := []struct{ lang, in, want string }{
		{"ur", "UPTIME 12H 30M", "UPTIME ۱۲H ۳۰M"},
		{"fa", "123", "۱۲۳"},
		{"ps", "09", "۰۹"},
		{"ar", "0123456789", "٠١٢٣٤٥٦٧٨٩"},
		{"hi", "1250", "१२५०"},
		{"bn", "15", "১৫"},
		{"pa", "7", "੭"},
		{"gu", "8", "૮"},
		{"ta", "9", "௯"},
		{"te", "3", "౩"},
		{"kn", "4", "೪"},
		{"ml", "5", "൫"},
		{"si", "6", "෬"},
		{"th", "0", "๐"},
		{"lo", "2", "໒"},
		{"my", "1", "၁"},
		{"km", "1", "១"},
		{"bo", "1", "༡"},
	}
	for _, c := range cases {
		if got := LocalizeDigits(c.lang, c.in); got != c.want {
			t.Errorf("LocalizeDigits(%q, %q) = %q, want %q", c.lang, c.in, got, c.want)
		}
	}
}

// TestLocalizeDigitsLeavesLatinScriptsAlone: jin languages ke apne numerals nahi
// (ya jo ASCII hi use karti hain) unke digits wahi rahen.
func TestLocalizeDigitsLeavesLatinScriptsAlone(t *testing.T) {
	for _, lang := range []string{"en", "fr", "de", "es", "tr", "id", "sw", "tl", "pt", "it", "vi", ""} {
		in := "MENU 1250 09"
		if got := LocalizeDigits(lang, in); got != in {
			t.Errorf("LocalizeDigits(%q) ne badal diya: %q", lang, got)
		}
		if HasLocalDigits(lang) {
			t.Errorf("HasLocalDigits(%q) true hona nahi chahiye", lang)
		}
	}
}

// TestLocalizeDigitsKeepsCommandTokenDigits: ".logo1" / "/menu2" ke digits ASCII
// hi rahen, warna menu jo dikhata hai wo user type nahi kar payega.
func TestLocalizeDigitsKeepsCommandTokenDigits(t *testing.T) {
	cases := []struct{ lang, in, want string }{
		{"ur", "*|🔰| .LOGO1 ❮ YOUR NAME ❯*", "*|🔰| .LOGO1 ❮ YOUR NAME ❯*"},
		{"ur", "*|🔰| .logo1000 ❮ YOUR NAME ❯*", "*|🔰| .logo1000 ❮ YOUR NAME ❯*"},
		{"ur", "/menu2 SHOW", "/menu2 SHOW"},
		// counts/uptime ARE localised, and both can appear on the same line
		{"ur", "COMMANDS :❯ ❮ 1250 ❯", "COMMANDS :❯ ❮ ۱۲۵۰ ❯"},
		{"ur", ".logo1 of 1250", ".logo1 of ۱۲۵۰"},
		{"ar", "UPTIME 02H 05M", "UPTIME ٠٢H ٠٥M"},
	}
	for _, c := range cases {
		if got := LocalizeDigits(c.lang, c.in); got != c.want {
			t.Errorf("LocalizeDigits(%q, %q) = %q, want %q", c.lang, c.in, got, c.want)
		}
	}
}

// TestLocalizeDigitsIsComplete: har mapped language ke liye das alag numerals.
func TestLocalizeDigitsIsComplete(t *testing.T) {
	for lang := range trtDigitBase {
		set, ok := trtDigitSet(lang)
		if !ok {
			t.Fatalf("%s mapped hai magar set nahi mila", lang)
		}
		rs := []rune(set)
		if len(rs) != 10 {
			t.Fatalf("%s: 10 numerals chahiye, mile %d", lang, len(rs))
		}
		seen := map[rune]bool{}
		for _, r := range rs {
			if seen[r] {
				t.Fatalf("%s: duplicate numeral %q", lang, r)
			}
			seen[r] = true
		}
		// The localisation must round-trip every ASCII digit distinctly.
		got := LocalizeDigits(lang, "0123456789")
		if len([]rune(got)) != 10 || got == "0123456789" {
			t.Fatalf("%s: digits localise nahi hue: %q", lang, got)
		}
	}
}
