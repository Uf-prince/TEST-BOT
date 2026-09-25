package main

import (
	"strings"
	"testing"

	"gold-md/gold-cmds"
)

// Every menu must carry the IMPORTANT CMNDS block right after its header, with
// the three bot-wide setters first and then that menu's own three setters.
func TestImportantCmdsBlockOnEveryMenu(t *testing.T) {
	plain := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "", menuStyleFor(nil, ""))
	for _, want := range []string{
		goldcmds.ImportantBorderTop,
		"*🔰 IMPORTANT CMNDS 🔰*",
		"*|🔰| .BOTPIC*",
		"*|🔰| .BOTSTYLE*",
		"*|🔰| .BOTVIDEO*",
		"*|🔰| .BOTVOICE*",
		"*|🔰| .MENUPIC*",
		"*|🔰| .MENUVIDEO*",
		"*|🔰| .MENUVOICE*",
		goldcmds.ImportantBorderBottom,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("plain menu missing %q\n%s", want, plain)
		}
	}
	// The block must sit between the header and the category list.
	hdr := strings.Index(plain, "┗─━─━")
	imp := strings.Index(plain, "IMPORTANT CMNDS")
	list := strings.Index(plain, "*| 🔰 | .CORE*")
	if !(hdr < imp && imp < list) {
		t.Fatalf("IMPORTANT CMNDS must come after the header and before the list (hdr=%d imp=%d list=%d)", hdr, imp, list)
	}
}

// A category menu names ITS OWN setters, not another menu's.
func TestImportantCmdsPerMenuNamesOwnSetters(t *testing.T) {
	cases := map[string][3]string{
		"AI":               {"AIMENUPIC", "AIMENUVIDEO", "AIMENUVOICE"},
		"GROUP MANAGEMENT": {"GROUPMENUPIC", "GROUPMENUVIDEO", "GROUPMENUVOICE"},
		"CONVERTER":        {"CONVERTERPIC", "CONVERTERVIDEO", "CONVERTERVOICE"},
		"FONT":             {"FONTPIC", "FONTVIDEO", "FONTVOICE"},
		"GAME":             {"GAMEPIC", "GAMEVIDEO", "GAMEVOICE"},
		"EQUALIZER":        {"EQUALIZERPIC", "EQUALIZERVIDEO", "EQUALIZERVOICE"},
		"OWNER & SYSTEM":   {"COREPIC", "COREVIDEO", "COREVOICE"},
		"DOWNLOADER":       {"DOWNLOADERPIC", "DOWNLOADERVIDEO", "DOWNLOADERVOICE"},
		"UTILITY":          {"UTILITYPIC", "UTILITYVIDEO", "UTILITYVOICE"},
		"TOOLS":            {"TOOLSPIC", "TOOLSVIDEO", "TOOLSVOICE"},
		"PRESENCE":         {"PRESENCEPIC", "PRESENCEVIDEO", "PRESENCEVOICE"},
	}
	for cat, want := range cases {
		out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, cat, menuStyleFor(nil, ""))
		for _, w := range want {
			if !strings.Contains(out, "*|🔰| ."+w+"*") {
				t.Errorf("%s menu missing its setter .%s\n%s", cat, w, out)
			}
		}
	}
}

// .logo / .font / .game / .equalizer own menus must carry the block too.
func TestImportantCmdsOnBespokeMenus(t *testing.T) {
	menus := map[string]string{
		"logo":      buildLogoMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, menuStyleFor(nil, "")),
		"font":      buildFontMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, menuStyleFor(nil, "")),
		"game":      buildGameMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, menuStyleFor(nil, "")),
		"equalizer": buildEqualizerMenu("92300", "92301", "1H 2M", ".", 0, menuStyleFor(nil, "")),
	}
	for key, out := range menus {
		if !strings.Contains(out, "*🔰 IMPORTANT CMNDS 🔰*") {
			t.Errorf("%s menu missing IMPORTANT CMNDS\n%s", key, out)
		}
		pic, video, voice, ok := goldcmds.MenuImportantCommands(key)
		if !ok {
			t.Fatalf("MenuImportantCommands(%q) not ok", key)
		}
		for _, w := range []string{pic, video, voice} {
			if !strings.Contains(out, "*|🔰| ."+w+"*") {
				t.Errorf("%s menu missing setter .%s\n%s", key, w, out)
			}
		}
	}
}

// The block follows the owner's prefix instead of a hard-coded ".".
func TestImportantCmdsUsesPrefix(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", "!", "Tester", "GOLD-MD", 0, nil, "AI", menuStyleFor(nil, ""))
	if !strings.Contains(out, "*|🔰| !BOTPIC*") || !strings.Contains(out, "*|🔰| !AIMENUVOICE*") {
		t.Fatalf("IMPORTANT CMNDS must use the live prefix\n%s", out)
	}
}

// Menu commands must not double-prefix when the prefix is already ".".
func TestImportantCmdsSinglePrefix(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "AI", menuStyleFor(nil, ""))
	if strings.Contains(out, "*|🔰| ..BOTPIC*") {
		t.Fatalf("IMPORTANT CMNDS has a doubled prefix\n%s", out)
	}
}

// Every category's border must be the bold 🔰-embossed one (owner order),
// and must NOT be the old un-bolded ❈ border.
func TestMenuBordersAreBoldRings(t *testing.T) {
	out := buildCategoryMenu("92300", "92301", "1H 2M", ".", "Tester", "GOLD-MD", 0, nil, "AI", menuStyleFor(nil, ""))
	if !strings.Contains(out, goldcmds.MenuBorderTop) || !strings.Contains(out, goldcmds.MenuBorderBottom) {
		t.Fatalf("category menu must use the bold 🔰 borders\n%s", out)
	}
	// Only the LIST border must be the new one; the IMPORTANT CMNDS block
	// legitimately keeps the ❈ mark, so check the list section specifically.
	list := out[strings.Index(out, goldcmds.MenuBorderTop):]
	if strings.Contains(list, "╔════ ≪ •❈• ≫ ════╗") {
		t.Fatalf("category list still uses the old ❈ border\n%s", out)
	}
}

// MenuImportantCommands must map every media spec, so no menu silently loses
// its IMPORTANT CMNDS block.
func TestMenuImportantCommandsCoverAllMenus(t *testing.T) {
	for _, key := range []string{
		"menu", "alive", "logo", "font", "game", "equalizer", "ai", "utility",
		"converter", "tools", "downloader", "group", "protection", "presence",
		"core", "breaction", "greaction",
	} {
		pic, video, voice, ok := goldcmds.MenuImportantCommands(key)
		if !ok || pic == "" || video == "" || voice == "" {
			t.Errorf("MenuImportantCommands(%q) = %q,%q,%q ok=%v", key, pic, video, voice, ok)
		}
	}
	if _, _, _, ok := goldcmds.MenuImportantCommands("nope"); ok {
		t.Error("unknown menu key must not resolve")
	}
}
