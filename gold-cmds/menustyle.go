package goldcmds

import (
	"fmt"
	"strings"
)

// ============================================================================
// GOLD-MD — MENU STYLE ENGINE
// File: menustyle.go
// ----------------------------------------------------------------------------
// Every menu (main + each category) can be re-skinned with one of 50 styles.
// A style swaps the fancy borders, the per-row decorations and the decorative
// font used for titles/labels. Command names stay plain ASCII so they remain
// copyable — only the framing changes.
//
// Style 1 is the classic look the menu already had; 2..50 are generated design
// combos (10 border sets x 5 row sets x a rotating decorative font).
//
// Commands (all hidden, owner-only):
//   .<menu>style              → guide + list of all 50 styles + a live preview
//   .<menu>style SET <1..50>  → apply to THAT menu only
//   .<menu>style RESET        → back to the bot-wide style (then the default)
//   .botmenustyle SET <n>     → apply to EVERY menu at once
//   .botmenustyle RESET       → clear the bot-wide style
// ============================================================================

// MenuStyleCount is how many designs the owner can pick from.
const MenuStyleCount = 50

// MenuStyleOff sentinel is unused today but reserved: a per-menu style override
// is cleared with RESET; there is no "silence the style" concept.
const MenuStyleOff = ""

// MenuStyle is one fully-resolved design.
type MenuStyle struct {
	N    int
	Name string
	Sym  string // the style's signature symbol (🔰 in style 1)

	HeaderTop  string // full line, no wrapping
	HeaderBot  string
	HeaderRowL string // decoration before a header line ("│🔰")

	ListTop    string // inner border, wrapped in *…* when rendered
	ListBottom string
	ListRowL   string // decoration before a list row ("| 🔰 | ")
	ListRowR   string

	ImpTop    string
	ImpBottom string
	ImpRowL   string
	ImpRowR   string

	BoldHeader bool // wrap header lines in *…* too
	font       int  // decorative-font index
}

// Styled renders a title/label in the style's decorative font.
func (st MenuStyle) Styled(s string) string { return applyMenuFont(st.font, s) }

// ResolveMenuStyle picks the effective style for one menu, reading the store
// through the bridge: per-menu override ("menustyle:<key>") wins, then the
// bot-wide style ("botmenustyle"), then the built-in classic style. Used by
// gold-cmds' own renderers (game headers) that do not run inside src.
func ResolveMenuStyle(s SessionBridge, key string) MenuStyle {
	if s == nil {
		return MenuStyleAt(1)
	}
	// A partially-implemented bridge (tests, early boot) must fall back to the
	// classic style instead of returning a zero (borderless) style.
	inner := func() (st MenuStyle) {
		st = MenuStyleAt(1)
		defer func() { _ = recover() }()
		if raw := strings.TrimSpace(s.GetMenuStyleSetting(key, "")); raw != "" {
			if n, ok := ParseMenuStyleArg(raw); ok {
				return MenuStyleAt(n)
			}
		}
		if raw := strings.TrimSpace(s.GetBotMenuStyleSetting("")); raw != "" {
			if n, ok := ParseMenuStyleArg(raw); ok {
				return MenuStyleAt(n)
			}
		}
		return st
	}
	return inner()
}

// BorderTop/Bottom return the list border wrapped for WhatsApp bold.
func (st MenuStyle) BorderTop() string    { return "*" + st.ListTop + "*" }
func (st MenuStyle) BorderBottom() string { return "*" + st.ListBottom + "*" }

// ImpBorderTop/Bottom return the IMPORTANT CMNDS border, bold-wrapped.
func (st MenuStyle) ImpBorderTop() string    { return "*" + st.ImpTop + "*" }
func (st MenuStyle) ImpBorderBottom() string { return "*" + st.ImpBottom + "*" }

// ImpTitle renders the IMPORTANT CMNDS block heading in the style's symbol and
// decorative font, e.g. "*🔰 IMPORTANT CMNDS 🔰*" for the classic style.
func (st MenuStyle) ImpTitle(title string) string {
	return "*" + st.Sym + " " + st.Styled(title) + " " + st.Sym + "*"
}

// RenderHeaderTop/Bot render the header frame with the (styled) title.
func (st MenuStyle) RenderHeaderTop(title string) string {
	line := fmt.Sprintf(st.HeaderTop, st.Styled(title))
	if st.BoldHeader {
		return "*" + line + "*"
	}
	return line
}

// RenderHeaderBot renders the header's closing frame. The bottom frame has no
// title slot, so it is emitted verbatim.
func (st MenuStyle) RenderHeaderBot() string {
	if st.BoldHeader {
		return "*" + st.HeaderBot + "*"
	}
	return st.HeaderBot
}

// ListRow renders one list row: "<deco> <prefix><name> <deco>", bold.
func (st MenuStyle) ListRow(prefix, name string) string {
	return "*" + st.ListRowL + st.SkinRow(prefix, name) + st.ListRowR + "*"
}

// SkinRow renders one menu row's prefix+name in the style's decorative font so
// a skinned bot looks consistent from frame to row. Style 1 is a no-op, so
// classic menus (and every existing test) keep their exact ASCII output.
func (st MenuStyle) SkinRow(prefix, name string) string {
	if st.font <= 0 || st.N <= 1 {
		return prefix + name
	}
	return prefix + applyMenuFont(st.font, name)
}

// ImpRow renders one IMPORTANT CMNDS row, bold.
func (st MenuStyle) ImpRow(prefix, name string) string {
	return "*" + st.ImpRowL + st.SkinRow(prefix, name) + st.ImpRowR + "*"
}

// ── style 1: the classic look ──────────────────────────────────────────────

var classicStyle = MenuStyle{
	N:    1,
	Name: "CLASSIC RING",
	Sym:  "🔰",

	HeaderTop:  "┏─━─━─🔰 %s 🔰─━─━─┓",
	HeaderBot:  "┗─━─━─━─━─━─━─━─━─┛",
	HeaderRowL: "│🔰",

	ListTop:    "╔════ ≪ • 🔰 • ≫ ════╗",
	ListBottom: "╚════ ≪ • 🔰 • ≫ ════╝",
	ListRowL:   "| 🔰 | ",
	ListRowR:   "",

	ImpTop:    "╔════ ≪ •❈• ≫ ════╗",
	ImpBottom: "╚════ ≪ •❈• ≫ ════╝",
	ImpRowL:   "|🔰| ",
	ImpRowR:   "",

	BoldHeader: false,
	font:       0,
}

// ── generated design pools ─────────────────────────────────────────────────

// 50 signature symbols, one per style.
var styleSymbols = []string{
	"🔰", "❈", "✦", "❃", "✿", "❖", "★", "◆", "♛", "⚜",
	"☬", "✵", "❂", "✹", "☯", "⚝", "✷", "❋", "✺", "❀",
	"◈", "▣", "⬢", "⧫", "⬟", "⭓", "⟁", "⌘", "⚹", "✧",
	"❉", "❊", "❇", "☸", "✤", "✜", "✥", "✢", "⊛", "◎",
	"◉", "▩", "▤", "▦", "▧", "▨", "▥", "⬣", "⬡", "⯃",
}

// 10 border templates. {S} = the style symbol, {T} = the styled title.
var styleBorders = []struct{ Top, Bot string }{
	{"╔════ ≪ • {S} • ≫ ════╗", "╚════ ≪ • {S} • ≫ ════╝"},
	{"✦━━━━ ❖ {S} ❖ ━━━━✦", "✦━━━━ ❖ {S} ❖ ━━━━✦"},
	{"╔═◤ {S} ◥════════╗", "╚═◣ {S} ◢════════╝"},
	{"✧･ﾟ: ✩ {S} ✩ :･ﾟ✧", "✧･ﾟ: ✩ {S} ✩ :･ﾟ✧"},
	{"╔══════【 {S} 】══════╗", "╚══════【 {S} 】══════╝"},
	{"▀▄▀▄▀▄ {S} ▄▀▄▀▄▀", "▀▄▀▄▀▄ {S} ▄▀▄▀▄▀"},
	{"╭━━━━❰ {S} ❱━━━━╮", "╰━━━━❰ {S} ❱━━━━╯"},
	{"⋆｡°✩ {S} ✩°｡⋆", "⋆｡°✩ {S} ✩°｡⋆"},
	{"╔╦═══ ≼ {S} ≽ ═══╦╗", "╚╩═══ ≼ {S} ≽ ═══╩╝"},
	{"❃━❃━❃❃ {S} ❃❃━❃━❃", "❃━❃━❃❃ {S} ❃❃━❃━❃"},
}

// 5 row-decoration templates. {S} = the style symbol.
var styleRows = []struct{ L, R string }{
	{"❃ ", " ❃"},
	{"✦ ▸ ", ""},
	{"【{S}】 ", ""},
	{"║ {S} ║ ", ""},
	{"⟦ {S} ⟧ ", ""},
}

// ── decorative fonts ───────────────────────────────────────────────────────

// applyMenuFont renders s in decorative font idx (0 = plain). Unknown runes and
// symbols pass through untouched, so digits/letters in command names are never
// lost. Only titles/labels go through this — command names stay ASCII.
func applyMenuFont(idx int, s string) string {
	if idx <= 0 {
		return strings.ToUpper(s)
	}
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		b.WriteRune(fontRune(idx, r))
	}
	return b.String()
}

func fontRune(idx int, r rune) rune {
	up := r >= 'A' && r <= 'Z'
	lo := r >= 'a' && r <= 'z'
	dg := r >= '0' && r <= '9'
	base := r
	if lo {
		base = r - 'a'
	} else if up {
		base = r - 'A'
	} else if dg {
		base = r - '0'
	}
	switch idx {
	case 1: // ｆｕｌｌｗｉｄｔｈ
		switch {
		case up:
			return 0xFF21 + base
		case lo:
			return 0xFF41 + base
		case dg:
			return 0xFF10 + base
		case r == ' ':
			return 0x3000
		}
	case 2: // 𝐌𝐀𝐓𝐇 𝐁𝐎𝐋𝐃
		switch {
		case up:
			return 0x1D400 + base
		case lo:
			return 0x1D41A + base
		case dg:
			return 0x1D7CE + base
		}
	case 3: // 𝑴𝒂𝒕𝒉 𝑩𝒐𝒍𝒅 𝑰𝒕𝒂𝒍𝒊𝒄
		switch {
		case up:
			return 0x1D468 + base
		case lo:
			return 0x1D482 + base
		case dg:
			return 0x1D7CE + base // bold-italic digits do not exist
		}
	case 4: // 𝘀𝗮𝗻𝘀-𝘀𝗲𝗿𝗶𝗳 𝗯𝗼𝗹𝗱
		switch {
		case up:
			return 0x1D5D4 + base
		case lo:
			return 0x1D5EE + base
		case dg:
			return 0x1D7EC + base
		}
	case 5: // 𝚖𝚘𝚗𝚘𝚜𝚙𝚊𝚌𝚎
		switch {
		case up:
			return 0x1D670 + base
		case lo:
			return 0x1D68A + base
		case dg:
			return 0x1D7F6 + base
		}
	case 6: // ⓒⓘⓡⓒⓛⓔⓓ
		switch {
		case up:
			return 0x24B6 + base
		case lo:
			return 0x24D0 + base
		case dg:
			if base == 0 {
				return 0x24EA
			}
			return 0x2460 + base - 1
		}
	case 7: // 🅢🅠🅤🅐🅡🅔🅓
		switch {
		case up:
			return 0x1F130 + base
		}
	case 8: // sᴍᴀʟʟ ᴄᴀᴘs
		if lo {
			if sm, ok := smallCapsLower[base]; ok {
				return sm
			}
		}
		if up || dg {
			return r
		}
	case 9: // 🇷🇪🇬🇮🇴🇳🇦🇱
		if up {
			return 0x1F1E6 + base
		}
	}
	return r
}

var smallCapsLower = map[rune]rune{
	'a': 0x1D00, 'b': 0x0299, 'c': 0x1D04, 'd': 0x1D05, 'e': 0x1D07,
	'f': 0xA730, 'g': 0x0262, 'h': 0x029C, 'i': 0x026A, 'j': 0x1D0A,
	'k': 0x1D0B, 'l': 0x029F, 'm': 0x1D0D, 'n': 0x0274, 'o': 0x1D0F,
	'p': 0x1D18, 'q': 0x01EB, 'r': 0x0280, 's': 0xA731, 't': 0x1D1B,
	'u': 0x1D1C, 'v': 0x1D20, 'w': 0x1D21, 'x': 0x02E3, 'y': 0x028F,
	'z': 0x1D22,
}

// styleNames are the pretty labels shown in the style guide.
var styleNames = []string{
	"CLASSIC RING", "ROYAL DIAMOND", "NEON BLAZE", "VELVET ROSES", "GOLD STAR",
	"OBSIDIAN GEM", "CROWN ROYALE", "MIDNIGHT ROSE", "QUEEN'S MARK", "IMPERIAL GOLD",
	"SHADOW SCROLL", "AURORA SPARK", "COSMIC EYE", "FIRE GEM", "ZEN BALANCE",
	"STAR CROSS", "PRISM LIGHT", "FROST BLOOM", "BLOSSOM GLOW", "SAKURA PETAL",
	"CRYSTAL GRID", "MOSAIC TILE", "HONEYCOMB", "THORN MARK", "DIAMOND SHIELD",
	"ROYAL TRIAD", "GLASS SHARD", "TERMINAL", "SPARK SEAL", "INK WHISPER",
	"SEED OF LIFE", "LOTUS RING", "FLORAL SEAL", "WHEEL OF LIFE", "CLOVER MARK",
	"ARROW FEATHER", "CROSS STAR", "TRIBE MARK", "BUBBLE WRAP", "TARGET EYE",
	"BULLSEYE", "WOVEN SILK", "CHECKER BOARD", "LATTICE", "BRICK WALL",
	"DOTTED VEIL", "WAVE CREST", "HEX SHIELD", "HEX CORE", "TRIBAL EDGE",
}

// MenuStyleAt resolves a stored style number (1..50; anything else → classic).
func MenuStyleAt(n int) MenuStyle {
	if n <= 1 || n > MenuStyleCount {
		return classicStyle
	}
	name := "STYLE " + fmt.Sprint(n)
	if n-1 < len(styleNames) {
		name = styleNames[n-1]
	}
	sym := "🔰"
	if n-1 < len(styleSymbols) {
		sym = styleSymbols[n-1]
	}
	border := styleBorders[(n-2)%len(styleBorders)]
	row := styleRows[((n-2)/len(styleBorders))%len(styleRows)]
	// Every styled style MUST change the font too (owner report: "sirf symbol
	// badla, font wahi hai"). The old (n-2)%10 gave font 0 to styles
	// 2, 12, 22, 32, 42 - which left the letters untouched. Rotate 1..9 instead.
	font := 1 + (n-2)%9

	fill := func(tpl, title string) string {
		out := strings.ReplaceAll(tpl, "{S}", sym)
		out = strings.ReplaceAll(out, "{T}", title)
		return out
	}
	st := MenuStyle{
		N:    n,
		Name: name,
		Sym:  sym,

		// The title sits inside the top frame, flanked by the style symbol.
		HeaderTop:  strings.ReplaceAll(border.Top, "{S}", sym+" %s "+sym),
		HeaderBot:  fill(border.Bot, ""),
		HeaderRowL: strings.TrimSpace(strings.ReplaceAll(row.L, "{S}", sym)),

		ListTop:    fill(border.Top, ""),
		ListBottom: fill(border.Bot, ""),
		ListRowL:   strings.ReplaceAll(row.L, "{S}", sym),
		ListRowR:   row.R,

		ImpTop:    fill(border.Top, ""),
		ImpBottom: fill(border.Bot, ""),
		ImpRowL:   strings.ReplaceAll(row.L, "{S}", sym),
		ImpRowR:   row.R,

		BoldHeader: true,
		font:       font,
	}
	return st
}

// MenuStyleName returns the display name for a style number.
func MenuStyleName(n int) string { return MenuStyleAt(n).Name }

// menuStyleCommandName maps a menu's voice command to its style sibling
// (.logovoice → .logostyle, .aimenuvoice → .aimenustyle).
func menuStyleCommandName(mc menuMediaCommand) string {
	return strings.TrimSuffix(mc.VoiceCmd, "voice") + "style"
}

// menuStyleCommands is the per-menu style command set, derived from the media
// table so every menu gets exactly one.
func menuStyleCommands() []menuMediaCommand { return menuVoiceCommands() }

// ParseMenuStyleArg accepts "1".."50" (also "SET 3") and returns the number.
func ParseMenuStyleArg(arg string) (int, bool) {
	f := strings.Fields(strings.ToLower(strings.TrimSpace(arg)))
	if len(f) == 0 {
		return 0, false
	}
	if f[0] == "set" {
		if len(f) < 2 {
			return 0, false
		}
		f = f[1:]
	}
	n := 0
	for _, r := range f[0] {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 || n > MenuStyleCount {
		return 0, false
	}
	return n, true
}

// menuStyleGuide renders the no-argument help card, ending with the shared
// TOMP3 info line when a prefix is supplied.
func menuStyleGuide(prefix, label, cmdName string) string {
	cmd := prefix + strings.ToUpper(cmdName)
	var b strings.Builder
	b.WriteString("*🔰 " + label + " STYLE GUIDE 🔰*\n\n")
	b.WriteString("*CHANGE THE WHOLE LOOK OF THIS MENU*\n")
	b.WriteString("*50 FANCY DESIGNS — BORDERS, SYMBOLS AND FONT*\n\n")
	b.WriteString("*❰ " + cmd + " ❱* → THIS GUIDE\n")
	b.WriteString("*❰ " + cmd + " SET <1-" + fmt.Sprint(MenuStyleCount) + "> ❱* → APPLY A STYLE\n")
	b.WriteString("*❰ " + cmd + " RESET ❱* → BACK TO BOT DEFAULT\n\n")
	b.WriteString("*🔰 ALL " + fmt.Sprint(MenuStyleCount) + " STYLES 🔰*\n")
	for n := 1; n <= MenuStyleCount; n++ {
		b.WriteString(fmt.Sprintf("*%d❯ %s*\n", n, MenuStyleName(n)))
	}
	b.WriteString("\n*BOT-WIDE:* *❰ " + prefix + "BOTMENUSTYLE SET <1-" + fmt.Sprint(MenuStyleCount) + "> ❱*")
	b.WriteString(menuStyleInfoLine(prefix))
	return b.String()
}

// menuStyleInfoLine is the shared footer for the style guides.
func menuStyleInfoLine(prefix string) string {
	return "\n\n*TYPE ❮ " + prefix + "TOMP3 ❯ FOR INFO*"
}

// MenuStylePreview renders a small sample menu in the given style, so the owner
// sees exactly what they are applying before/after saving.
func MenuStylePreview(styleN int, title, prefix string) string {
	st := MenuStyleAt(styleN)
	sample := []string{"CORE", "AI", "CONVERTER", "FONT"}
	var b strings.Builder
	b.WriteString(st.RenderHeaderTop(title) + "\n")
	b.WriteString("*" + st.HeaderRowL + " USER:❯ UMAR*\n")
	b.WriteString("*" + st.HeaderRowL + " STYLE:❯ ❮ " + fmt.Sprint(st.N) + " ❯*\n")
	b.WriteString(st.RenderHeaderBot() + "\n\n")
	b.WriteString(st.BorderTop() + "\n")
	for _, s := range sample {
		b.WriteString(st.ListRow(prefix, s) + "\n")
	}
	b.WriteString(st.BorderBottom())
	return b.String()
}
