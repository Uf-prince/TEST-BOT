package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	goldcmds "gold-md/gold-cmds"
)

// TestLiveBespokeMenus builds each bespoke menu (.logo/.font/.game/.equalizer/
// .botstyle) and runs it through the REAL translation pipeline (ur/hi) to see
// exactly which strings stay English. GOLDMD_LIVE=1 to run.
func TestLiveBespokeMenus(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") == "" {
		t.Skip("set GOLDMD_LIVE=1 to run")
	}
	st := menuStyleFor(nil, "")
	view := &goldcmds.CmdNameView{Renames: map[string]string{}}
	menus := map[string]string{
		"logo":      buildLogoMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, view, st),
		"font":      buildFontMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, view, st),
		"game":      buildGameMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, view, st),
		"equalizer": buildEqualizerMenu("92300", "92301", "1H 2M", ".", 0, st),
		"botstyle":  buildBotStyleMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, view, st),
	}
	for _, lang := range []string{"ur", "hi"} {
		for _, key := range []string{"logo", "font", "game", "equalizer", "botstyle"} {
			menu := menus[key]
			ctx, cancel := context.WithTimeout(goldcmds.TrtWithPrefix(context.Background(), "."), 30*time.Second)
			out, err := goldcmds.TranslatePreservingCommandTokens(ctx, menu, lang)
			cancel()
			if err != nil {
				t.Fatalf("%s/%s: %v", lang, key, err)
			}
			out = strings.ReplaceAll(out, "."+key, "LOCALIZED_"+key)
			fmt.Printf("\n===== %s / %s =====\n%s\n", lang, key, firstLines(out, 12))
		}
	}
}

func firstLines(s string, n int) string {
	out := ""
	count := 0
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out += s[start:i+1]
			start = i + 1
			count++
			if count >= n {
				break
			}
		}
	}
	if start < len(s) {
		out += s[start:]
	}
	return out
}
