package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 27 (10 new text styling & transformation commands)
// File: toolpack27.go
// ============================================================================
//   .textborder <text>     -> wrap text in a decorative border
//   .textbox <text>        -> put text inside a box
//   .textfind <needle> | <haystack> -> find a word in text
//   .textinsert <pos> <text> | <insert> -> insert text at a position
//   .textleet <text>       -> convert text to leetspeak
//   .textmirror <text>     -> mirror text
//   .textsmallcaps <text>  -> convert text to small caps
//   .textsubscript <text>  -> convert text to subscript
//   .textsuperscript <text>-> convert text to superscript
//   .textupsidedown <text> -> flip text upside down
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .TEXTBORDER ──────────────────────────────────────────────────────────────

func textborderGuide(prefix string) string {
	return "*🔰 TEXT BORDER 🔰*\n\n" +
		"*WRAP TEXT IN A DECORATIVE BORDER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTBORDER <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTBORDER hello ❯*"
}

func handleTextborder(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textborderGuide(prefix))
			return
		}
		line := strings.Repeat("═", len([]rune(text))+4)
		var out strings.Builder
		out.WriteString("*🔰 TEXT BORDER 🔰*\n\n")
		out.WriteString("*`╔" + line + "╗`*\n")
		out.WriteString("*`║  " + text + "  ║`*\n")
		out.WriteString("*`╚" + line + "╝`*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTBOX ─────────────────────────────────────────────────────────────────

func textboxGuide(prefix string) string {
	return "*🔰 TEXT BOX 🔰*\n\n" +
		"*PUT TEXT INSIDE A BOX*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTBOX <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTBOX hello ❯*"
}

func handleTextbox(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textboxGuide(prefix))
			return
		}
		line := strings.Repeat("─", len([]rune(text))+2)
		var out strings.Builder
		out.WriteString("*🔰 TEXT BOX 🔰*\n\n")
		out.WriteString("*`┌" + line + "┐`*\n")
		out.WriteString("*`│ " + text + " │`*\n")
		out.WriteString("*`└" + line + "┘`*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTFIND ────────────────────────────────────────────────────────────────

func textfindGuide(prefix string) string {
	return "*🔰 TEXT FIND 🔰*\n\n" +
		"*FIND A WORD INSIDE TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTFIND <WORD> | <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTFIND cat | the cat sat on the mat ❯*"
}

func handleTextfind(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, textfindGuide(prefix))
			return
		}
		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 {
			s.Reply(info, "*🔰 TEXT FIND 🔰*\n\n*❌ SEPARATE THE WORD AND TEXT WITH A PIPE |*")
			return
		}
		needle := strings.TrimSpace(parts[0])
		hay := parts[1]
		if needle == "" {
			s.Reply(info, "*🔰 TEXT FIND 🔰*\n\n*❌ PROVIDE A WORD TO FIND*")
			return
		}
		count := strings.Count(strings.ToLower(hay), strings.ToLower(needle))
		idx := strings.Index(strings.ToLower(hay), strings.ToLower(needle))
		var out strings.Builder
		out.WriteString("*🔰 TEXT FIND 🔰*\n\n")
		out.WriteString("*🔍 WORD ❯ " + strings.ToUpper(needle) + "*\n")
		out.WriteString("*📊 OCCURRENCES ❯ " + strconv.Itoa(count) + "*\n")
		if idx >= 0 {
			out.WriteString("*📍 FIRST AT ❯ POSITION " + strconv.Itoa(idx+1) + "*")
		} else {
			out.WriteString("*❌ NOT FOUND IN TEXT*")
		}
		s.Reply(info, out.String())
	})
}

// ── .TEXTINSERT ──────────────────────────────────────────────────────────────

func textinsertGuide(prefix string) string {
	return "*🔰 TEXT INSERT 🔰*\n\n" +
		"*INSERT TEXT AT A POSITION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTINSERT <POS> <TEXT> | <INSERT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTINSERT 3 hello | XX ❯*"
}

func handleTextinsert(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textinsertGuide(prefix))
			return
		}
		pos, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || pos < 0 {
			s.Reply(info, "*🔰 TEXT INSERT 🔰*\n\n*❌ PROVIDE A VALID POSITION*")
			return
		}
		raw := strings.Join(args[1:], " ")
		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 {
			s.Reply(info, "*🔰 TEXT INSERT 🔰*\n\n*❌ SEPARATE TEXT AND INSERT WITH A PIPE |*")
			return
		}
		text := strings.TrimSpace(parts[0])
		insert := strings.TrimSpace(parts[1])
		runes := []rune(text)
		if pos > len(runes) {
			pos = len(runes)
		}
		result := string(runes[:pos]) + insert + string(runes[pos:])
		var out strings.Builder
		out.WriteString("*🔰 TEXT INSERT 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n")
		out.WriteString("*➕ INSERTED ❯ " + insert + "*\n")
		out.WriteString("*📍 POSITION ❯ " + strconv.Itoa(pos) + "*\n\n")
		out.WriteString("*✅ RESULT ❯ " + result + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTLEET ────────────────────────────────────────────────────────────────

func textleetGuide(prefix string) string {
	return "*🔰 TEXT LEET 🔰*\n\n" +
		"*CONVERT TEXT TO LEETSPEAK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTLEET <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTLEET hello ❯*"
}

func handleTextleet(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textleetGuide(prefix))
			return
		}
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if v, ok := leetMap[r]; ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT LEET 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*💻 LEET ❯ " + b.String() + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTMIRROR ──────────────────────────────────────────────────────────────

func textmirrorGuide(prefix string) string {
	return "*🔰 TEXT MIRROR 🔰*\n\n" +
		"*MIRROR TEXT BACKWARDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTMIRROR <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTMIRROR hello ❯*"
}

func handleTextmirror(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textmirrorGuide(prefix))
			return
		}
		runes := []rune(text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT MIRROR 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🪞 MIRRORED ❯ " + string(runes) + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTSMALLCAPS ───────────────────────────────────────────────────────────

func textsmallcapsGuide(prefix string) string {
	return "*🔰 TEXT SMALL CAPS 🔰*\n\n" +
		"*CONVERT TEXT TO SMALL CAPITALS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTSMALLCAPS <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTSMALLCAPS hello world ❯*"
}

func handleTextsmallcaps(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textsmallcapsGuide(prefix))
			return
		}
		small := map[rune]rune{
			'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ꜰ', 'g': 'ɢ', 'h': 'ʜ', 'i': 'ɪ', 'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ', 'm': 'ᴍ',
			'n': 'ɴ', 'o': 'ᴏ', 'p': 'ᴘ', 'q': 'ǫ', 'r': 'ʀ', 's': 'ꜱ', 't': 'ᴛ', 'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x', 'y': 'ʏ', 'z': 'ᴢ',
		}
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if v, ok := small[r]; ok {
				b.WriteRune(v)
			} else {
				b.WriteRune(r)
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT SMALL CAPS 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🔤 SMALL CAPS ❯ " + b.String() + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTSUBSCRIPT ───────────────────────────────────────────────────────────

func textsubscriptGuide(prefix string) string {
	return "*🔰 TEXT SUBSCRIPT 🔰*\n\n" +
		"*CONVERT TEXT TO SUBSCRIPT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTSUBSCRIPT <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTSUBSCRIPT h2o ❯*"
}

func handleTextsubscript(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textsubscriptGuide(prefix))
			return
		}
		sub := map[rune]rune{
			'0': '₀', '1': '₁', '2': '₂', '3': '₃', '4': '₄', '5': '₅', '6': '₆', '7': '₇', '8': '₈', '9': '₉',
			'a': 'ₐ', 'e': 'ₑ', 'h': 'ₕ', 'i': 'ᵢ', 'j': 'ⱼ', 'k': 'ₖ', 'l': 'ₗ', 'm': 'ₘ', 'n': 'ₙ', 'o': 'ₒ', 'p': 'ₚ', 'r': 'ᵣ', 's': 'ₛ', 't': 'ₜ', 'u': 'ᵤ', 'v': 'ᵥ', 'x': 'ₓ',
		}
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if v, ok := sub[r]; ok {
				b.WriteRune(v)
			} else {
				b.WriteRune(r)
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT SUBSCRIPT 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🔽 SUBSCRIPT ❯ " + b.String() + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTSUPERSCRIPT ─────────────────────────────────────────────────────────

func textsuperscriptGuide(prefix string) string {
	return "*🔰 TEXT SUPERSCRIPT 🔰*\n\n" +
		"*CONVERT TEXT TO SUPERSCRIPT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTSUPERSCRIPT <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTSUPERSCRIPT x2 ❯*"
}

func handleTextsuperscript(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textsuperscriptGuide(prefix))
			return
		}
		sup := map[rune]rune{
			'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹',
			'a': 'ᵃ', 'b': 'ᵇ', 'c': 'ᶜ', 'd': 'ᵈ', 'e': 'ᵉ', 'f': 'ᶠ', 'g': 'ᵍ', 'h': 'ʰ', 'i': 'ⁱ', 'j': 'ʲ', 'k': 'ᵏ', 'l': 'ˡ', 'm': 'ᵐ', 'n': 'ⁿ', 'o': 'ᵒ', 'p': 'ᵖ', 'r': 'ʳ', 's': 'ˢ', 't': 'ᵗ', 'u': 'ᵘ', 'v': 'ᵛ', 'w': 'ʷ', 'x': 'ˣ', 'y': 'ʸ', 'z': 'ᶻ',
		}
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if v, ok := sup[r]; ok {
				b.WriteRune(v)
			} else {
				b.WriteRune(r)
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT SUPERSCRIPT 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🔼 SUPERSCRIPT ❯ " + b.String() + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTUPSIDEDOWN ──────────────────────────────────────────────────────────

func textupsidedownGuide(prefix string) string {
	return "*🔰 TEXT UPSIDE DOWN 🔰*\n\n" +
		"*FLIP TEXT UPSIDE DOWN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTUPSIDEDOWN <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTUPSIDEDOWN hello ❯*"
}

func handleTextupsidedown(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textupsidedownGuide(prefix))
			return
		}
		var b strings.Builder
		for _, r := range text {
			if v, ok := flipMap[r]; ok {
				b.WriteRune(v)
			} else {
				b.WriteRune(r)
			}
		}
		runes := []rune(b.String())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT UPSIDE DOWN 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🙃 FLIPPED ❯ " + string(runes) + "*")
		s.Reply(info, out.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "textborder", Category: "TOOLS", Desc: "Wrap text in a decorative border", Run: handleTextborder})
	Register(Command{Name: "textbox", Category: "TOOLS", Desc: "Put text inside a box", Run: handleTextbox})
	Register(Command{Name: "textfind", Category: "TOOLS", Desc: "Find a word inside text", Run: handleTextfind})
	Register(Command{Name: "textinsert", Category: "TOOLS", Desc: "Insert text at a position", Run: handleTextinsert})
	Register(Command{Name: "textleet", Category: "TOOLS", Desc: "Convert text to leetspeak", Run: handleTextleet})
	Register(Command{Name: "textmirror", Category: "TOOLS", Desc: "Mirror text backwards", Run: handleTextmirror})
	Register(Command{Name: "textsmallcaps", Category: "TOOLS", Desc: "Convert text to small capitals", Run: handleTextsmallcaps})
	Register(Command{Name: "textsubscript", Category: "TOOLS", Desc: "Convert text to subscript", Run: handleTextsubscript})
	Register(Command{Name: "textsuperscript", Category: "TOOLS", Desc: "Convert text to superscript", Run: handleTextsuperscript})
	Register(Command{Name: "textupsidedown", Category: "TOOLS", Desc: "Flip text upside down", Run: handleTextupsidedown})

	Register(Command{Name: "bordertext", Category: "TOOLS", Desc: "Short alias of .textborder", Hidden: true, Run: handleTextborder})
	Register(Command{Name: "boxtext", Category: "TOOLS", Desc: "Short alias of .textbox", Hidden: true, Run: handleTextbox})
	Register(Command{Name: "findtext", Category: "TOOLS", Desc: "Short alias of .textfind", Hidden: true, Run: handleTextfind})
	Register(Command{Name: "inserttext", Category: "TOOLS", Desc: "Short alias of .textinsert", Hidden: true, Run: handleTextinsert})
	Register(Command{Name: "leettext", Category: "TOOLS", Desc: "Short alias of .textleet", Hidden: true, Run: handleTextleet})
	Register(Command{Name: "mirrortext", Category: "TOOLS", Desc: "Short alias of .textmirror", Hidden: true, Run: handleTextmirror})
	Register(Command{Name: "smallcaps", Category: "TOOLS", Desc: "Short alias of .textsmallcaps", Hidden: true, Run: handleTextsmallcaps})
	Register(Command{Name: "subscript", Category: "TOOLS", Desc: "Short alias of .textsubscript", Hidden: true, Run: handleTextsubscript})
	Register(Command{Name: "superscript", Category: "TOOLS", Desc: "Short alias of .textsuperscript", Hidden: true, Run: handleTextsuperscript})
	Register(Command{Name: "upsidedown", Category: "TOOLS", Desc: "Short alias of .textupsidedown", Hidden: true, Run: handleTextupsidedown})
}
