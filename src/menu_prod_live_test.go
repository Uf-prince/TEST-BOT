package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	goldcmds "gold-md/gold-cmds"
)

// TestProdMenuFlow reproduces the live bot's exact flow for the bespoke menus
// using the REAL reply cache for the live bot JID + language. This shows what
// the bot actually sends. GOLDMD_LIVE=1 to run.
func TestProdMenuFlow(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") == "" {
		t.Skip("set GOLDMD_LIVE=1 to run")
	}
	rcInit()
	botJID := os.Getenv("GOLDMD_BOT_JID")
	if botJID == "" {
		botJID = "923158930864@s.whatsapp.net"
	}
	lang := os.Getenv("GOLDMD_LANG")
	if lang == "" {
		lang = "ur"
	}
	st := menuStyleFor(nil, "")
	view := &goldcmds.CmdNameView{Renames: map[string]string{}}
	menus := map[string]string{
		"font":      buildFontMenu("000000000000", "000000000000", "1H 2M", ".", "OWNER", "GOLD-MD", 1, view, st),
		"game":      buildGameMenu("000000000000", "000000000000", "1H 2M", ".", "OWNER", "GOLD-MD", 1, view, st),
		"equalizer": buildEqualizerMenu("000000000000", "000000000000", "1H 2M", ".", 1, st),
		"logo":      buildLogoMenu("000000000000", "000000000000", "1H 2M", ".", "OWNER", "GOLD-MD", 1, view, st),
		"botstyle":  buildBotStyleMenu("000000000000", "000000000000", "1H 2M", ".", "OWNER", "GOLD-MD", 1, view, st),
	}
	for _, key := range []string{"font", "game", "equalizer", "logo", "botstyle"} {
		ctx, cancel := context.WithTimeout(goldcmds.TrtCacheWithBot(context.Background(), botJID), 30*time.Second)
		out, err := goldcmds.TranslatePreservingCommandTokens(ctx, menus[key], lang)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		fmt.Printf("\n===== %s (%s, cache=%s) =====\n%s\n", key, lang, botJID, firstLines(out, 16))
	}
}
