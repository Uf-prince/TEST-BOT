package main

import "testing"

// TestReplaceCommandTokens checks the pure substitution helper that localizes
// command tokens in an already-translated reply: it swaps a delimited
// "<prefix><command>" token for its localized form, fills numeric-family gaps
// from the base word, and leaves look-alikes alone.
func TestReplaceCommandTokens(t *testing.T) {
	byCanon := map[string]string{
		"font":    "فونٹ",
		"font1":   "فونٹ 1",
		"logo":    "لوگو",
		"core":    "بنیادی",
		"group":   "گروپ",
		"equalizer": "برابر کرنے والا",
		"menu":    "مینو",
	}
	cases := []struct{ in, want string }{
		// exact localized form
		{"*| 🔰 | .FONT1 ❮ X ❯*", "*| 🔰 | .فونٹ1 ❮ X ❯*"},
		// numeric-family gap filled from the single-token base ("logo" -> لوگو)
		{"*| 🔰 | .LOGO5 ❮ X ❯*", "*| 🔰 | .لوگو5 ❮ X ❯*"},
		// category slug
		{"*| 🔰 | .CORE*", "*| 🔰 | .بنیادی*"},
		{"*| 🔰 | .GROUP*", "*| 🔰 | .گروپ*"},
		// multi-word base is used for the numeric family (eq -> equalizer)
		{"*| 🔰 | .EQ3 ❮ X ❯*", "*| 🔰 | .برابر کرنے والا3 ❮ X ❯*"},
		// delimited: a preceding word byte must block the match
		{"photo.font1 stays", "photo.font1 stays"},
		{".FONT1X stays", ".FONT1X stays"},
		// a sentence-ending period is not a token
		{"DONE. OK", "DONE. OK"},
	}
	for _, c := range cases {
		got := replaceCommandTokens(c.in, ".", byCanon)
		if got != c.want {
			t.Errorf("replaceCommandTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCollapseTrailingNumberSpace checks the "فونٹ 1" -> "فونٹ1" collapse.
func TestCollapseTrailingNumberSpace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"فونٹ 1", "فونٹ1"},
		{"لوگو 1000", "لوگو1000"},
		{"برابر کرنے والا", "برابر کرنے والا"},
		{"مینو", "مینو"},
		{"a 12b", "a 12b"},
	}
	for _, c := range cases {
		if got := collapseTrailingNumberSpace(c.in); got != c.want {
			t.Errorf("collapseTrailingNumberSpace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSplitTrailingDigits checks the base/digits split.
func TestSplitTrailingDigits(t *testing.T) {
	cases := []struct {
		in, base, digits string
		has              bool
	}{
		{"font1000", "font", "1000", true},
		{"eq1", "eq", "1", true},
		{"font", "font", "", false},
		{"1000", "1000", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		b, d, h := splitTrailingDigits(c.in)
		if b != c.base || d != c.digits || h != c.has {
			t.Errorf("splitTrailingDigits(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, b, d, h, c.base, c.digits, c.has)
		}
	}
}
