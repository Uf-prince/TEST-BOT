package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	goldcmds "gold-md/gold-cmds"
)

// TestReplaceCategoryToken checks the pure substitution helper: it swaps a
// delimited ".slug" token for the localized form and leaves look-alikes alone.
func TestReplaceCategoryToken(t *testing.T) {
	cases := []struct{ in, want string }{
		{"*| 🔰 | .CORE*", "*| 🔰 | .بنیادی*"},
		{"*| 🔰 | .GROUP*\n*| 🔰 | .PROTECTION*", "*| 🔰 | .گروپ*\n*| 🔰 | .تحفظ*"},
		{"photo.core stays", "photo.core stays"},
		{".COREX stays", ".COREX stays"},
		{"*.CORE*", "*.بنیادی*"},
	}
	for _, c := range cases {
		got := replaceCategoryToken(c.in, ".", "core", "بنیادی")
		if c.in == "*| 🔰 | .GROUP*\n*| 🔰 | .PROTECTION*" {
			got = replaceCategoryToken(got, ".", "group", "گروپ")
			got = replaceCategoryToken(got, ".", "protection", "تحفظ")
		}
		if got != c.want {
			t.Errorf("replaceCategoryToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLiveMenuTranslation builds the REAL .menu caption, runs it through the live
// translation pipeline, then applies the category substitution — the exact
// production flow — so we can see the fully localized menu.
func TestLiveMenuTranslation(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") == "" {
		t.Skip("set GOLDMD_LIVE=1 to run")
	}
	catLoc := map[string]string{
		"core": "بنیادی", "group": "گروپ", "protection": "تحفظ",
		"downloader": "ڈاؤن لوڈر", "ai": "اے آئی", "utility": "افادیت",
		"presence": "موجودگی", "converter": "کنورٹر", "tools": "اوزار",
		"breaction": "بی ری ایکشن", "greaction": "جی ری ایکشن",
		"equalizer": "ایکویلائزر", "font": "فونٹ", "game": "گیم",
		"botstyle": "بوٹ اسٹائل", "logo": "لوگو",
	}
	menu := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "", menuStyleFor(nil, ""))
	fmt.Println("===== ORIGINAL MENU =====")
	fmt.Println(menu)
	for _, lang := range []string{"ur", "hi"} {
		ctx, cancel := context.WithTimeout(goldcmds.TrtWithPrefix(context.Background(), "."), 25*time.Second)
		out, err := goldcmds.TranslatePreservingCommandTokens(ctx, menu, lang)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for _, slug := range []string{"core", "group", "protection", "downloader", "ai", "utility", "presence", "converter", "tools", "equalizer", "font", "game", "botstyle", "logo"} {
			out = replaceCategoryToken(out, ".", slug, catLoc[slug])
		}
		fmt.Printf("\n===== %s (after substitution) =====\n", lang)
		fmt.Println(out)
	}
}
