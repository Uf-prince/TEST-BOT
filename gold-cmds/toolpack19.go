package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 19 (10 new text commands)
// File: toolpack19.go
// ============================================================================
//   .charfreq <text>            -> character frequency table
//   .uniquewords <text>         -> unique words and count
//   .longestword <text>         -> longest word(s)
//   .textreverse <chars|words> <text> -> reverse text
//   .textwrap <n> <text>        -> wrap text at N characters
//   .texttrim <text>            -> trim and collapse spaces
//   .textsplit <delim> <text>   -> split text by delimiter
//   .textreplace <old> <new> <text> -> replace text
//   .textbanner <text>          -> big ASCII banner
//   .textflip <text>            -> upside-down text
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .CHARFREQ ───────────────────────────────────────────────────────────────

func charfreqGuide(prefix string) string {
	return "*🔰 CHARACTER FREQUENCY 🔰*\n\n" +
		"*COUNT HOW MANY TIMES EACH CHARACTER APPEARS IN TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CHARFREQ <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CHARFREQ HELLO WORLD ❯*"
}

func handleCharfreq(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, charfreqGuide(prefix))
			return
		}
		counts := map[rune]int{}
		for _, r := range strings.ToLower(text) {
			if r == ' ' {
				continue
			}
			counts[r]++
		}
		type kv struct {
			k rune
			v int
		}
		list := make([]kv, 0, len(counts))
		for k, v := range counts {
			list = append(list, kv{k, v})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].v != list[j].v {
				return list[i].v > list[j].v
			}
			return list[i].k < list[j].k
		})
		var b strings.Builder
		b.WriteString("*🔰 CHARACTER FREQUENCY 🔰*\n\n")
		b.WriteString("*📝 TEXT ❯ " + text + "*\n\n")
		limit := len(list)
		if limit > 20 {
			limit = 20
		}
		for i := 0; i < limit; i++ {
			b.WriteString("*" + string(list[i].k) + " ❯ " + strconv.Itoa(list[i].v) + " TIMES*\n")
		}
		b.WriteString("\n*🔢 UNIQUE CHARS ❯ " + strconv.Itoa(len(list)) + "*")
		s.Reply(info, b.String())
	})
}

// ── .UNIQUEWORDS ────────────────────────────────────────────────────────────

func uniquewordsGuide(prefix string) string {
	return "*🔰 UNIQUE WORDS 🔰*\n\n" +
		"*FIND ALL UNIQUE WORDS IN YOUR TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UNIQUEWORDS <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "UNIQUEWORDS THE CAT AND THE DOG ❯*"
}

func handleUniquewords(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, uniquewordsGuide(prefix))
			return
		}
		fields := strings.Fields(strings.ToLower(text))
		seen := map[string]bool{}
		var uniq []string
		for _, w := range fields {
			w = strings.Trim(w, ".,!?;:\"'()[]{}")
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			uniq = append(uniq, w)
		}
		var b strings.Builder
		b.WriteString("*🔰 UNIQUE WORDS 🔰*\n\n")
		b.WriteString("*📊 TOTAL WORDS ❯ " + strconv.Itoa(len(fields)) + "*\n")
		b.WriteString("*🔢 UNIQUE WORDS ❯ " + strconv.Itoa(len(uniq)) + "*\n\n")
		b.WriteString("*📝 LIST ❯ " + strings.Join(uniq, ", ") + "*")
		s.Reply(info, b.String())
	})
}

// ── .LONGESTWORD ────────────────────────────────────────────────────────────

func longestwordGuide(prefix string) string {
	return "*🔰 LONGEST WORD 🔰*\n\n" +
		"*FIND THE LONGEST WORD IN YOUR TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LONGESTWORD <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LONGESTWORD I LOVE PROGRAMMING ❯*"
}

func handleLongestword(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, longestwordGuide(prefix))
			return
		}
		fields := strings.Fields(text)
		maxLen := 0
		var longest []string
		for _, w := range fields {
			clean := strings.Trim(w, ".,!?;:\"'()[]{}")
			l := len([]rune(clean))
			if l > maxLen {
				maxLen = l
				longest = []string{clean}
			} else if l == maxLen && l > 0 {
				longest = append(longest, clean)
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 LONGEST WORD 🔰*\n\n")
		b.WriteString("*📏 LENGTH ❯ " + strconv.Itoa(maxLen) + " CHARS*\n")
		b.WriteString("*🏆 WORD(S) ❯ " + strings.Join(longest, ", ") + "*")
		s.Reply(info, b.String())
	})
}

// ── .TEXTREVERSE ────────────────────────────────────────────────────────────

func textreverseGuide(prefix string) string {
	return "*🔰 TEXT REVERSE 🔰*\n\n" +
		"*REVERSE TEXT BY CHARACTERS OR BY WORDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTREVERSE <CHARS|WORDS> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTREVERSE CHARS HELLO ❯*"
}

func handleTextreverse(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textreverseGuide(prefix))
			return
		}
		mode := strings.ToLower(args[0])
		text := strings.Join(args[1:], " ")
		var out string
		switch mode {
		case "chars", "char":
			r := []rune(text)
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
			out = string(r)
		case "words", "word":
			f := strings.Fields(text)
			for i, j := 0, len(f)-1; i < j; i, j = i+1, j-1 {
				f[i], f[j] = f[j], f[i]
			}
			out = strings.Join(f, " ")
		default:
			s.Reply(info, "*🔰 MODE MUST BE CHARS OR WORDS*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 TEXT REVERSE 🔰*\n\n")
		b.WriteString("*📝 ORIGINAL ❯ " + text + "*\n")
		b.WriteString("*🔄 REVERSED ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .TEXTWRAP ───────────────────────────────────────────────────────────────

func textwrapGuide(prefix string) string {
	return "*🔰 TEXT WRAP 🔰*\n\n" +
		"*WRAP LONG TEXT INTO LINES OF N CHARACTERS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTWRAP <N> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTWRAP 10 HELLO WORLD THIS IS A TEST ❯*"
}

func handleTextwrap(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textwrapGuide(prefix))
			return
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			s.Reply(info, "*🔰 INVALID WIDTH, USE "+prefix+"TEXTWRAP <N> <TEXT>*")
			return
		}
		text := strings.Join(args[1:], " ")
		var lines []string
		for _, word := range strings.Fields(text) {
			if len(lines) == 0 {
				lines = append(lines, word)
				continue
			}
			last := lines[len(lines)-1]
			if len([]rune(last))+1+len([]rune(word)) <= n {
				lines[len(lines)-1] = last + " " + word
			} else {
				lines = append(lines, word)
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 TEXT WRAP 🔰*\n\n")
		b.WriteString("*📏 WIDTH ❯ " + strconv.Itoa(n) + " CHARS*\n\n")
		for _, l := range lines {
			b.WriteString("*" + l + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .TEXTTRIM ───────────────────────────────────────────────────────────────

func texttrimGuide(prefix string) string {
	return "*🔰 TEXT TRIM 🔰*\n\n" +
		"*REMOVE EXTRA SPACES AND TRIM YOUR TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTTRIM <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTTRIM   HELLO    WORLD   ❯*"
}

func handleTexttrim(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, texttrimGuide(prefix))
			return
		}
		trimmed := strings.Join(strings.Fields(text), " ")
		var b strings.Builder
		b.WriteString("*🔰 TEXT TRIM 🔰*\n\n")
		b.WriteString("*📥 ORIGINAL LENGTH ❯ " + strconv.Itoa(len([]rune(text))) + "*\n")
		b.WriteString("*📤 TRIMMED LENGTH ❯ " + strconv.Itoa(len([]rune(trimmed))) + "*\n\n")
		b.WriteString("*✅ CLEAN TEXT ❯ " + trimmed + "*")
		s.Reply(info, b.String())
	})
}

// ── .TEXTSPLIT ──────────────────────────────────────────────────────────────

func textsplitGuide(prefix string) string {
	return "*🔰 TEXT SPLIT 🔰*\n\n" +
		"*SPLIT TEXT INTO PARTS USING A DELIMITER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTSPLIT <DELIMITER> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTSPLIT , APPLE,BANANA,MANGO ❯*"
}

func handleTextsplit(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textsplitGuide(prefix))
			return
		}
		delim := args[0]
		text := strings.Join(args[1:], " ")
		parts := strings.Split(text, delim)
		var b strings.Builder
		b.WriteString("*🔰 TEXT SPLIT 🔰*\n\n")
		b.WriteString("*✂️ DELIMITER ❯ " + delim + "*\n")
		b.WriteString("*🔢 PARTS ❯ " + strconv.Itoa(len(parts)) + "*\n\n")
		for i, p := range parts {
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + strings.TrimSpace(p) + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .TEXTREPLACE ────────────────────────────────────────────────────────────

func textreplaceGuide(prefix string) string {
	return "*🔰 TEXT REPLACE 🔰*\n\n" +
		"*REPLACE A WORD OR PHRASE IN YOUR TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTREPLACE <OLD> <NEW> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTREPLACE CAT DOG I LOVE CAT ❯*"
}

func handleTextreplace(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, textreplaceGuide(prefix))
			return
		}
		old := args[0]
		newW := args[1]
		text := strings.Join(args[2:], " ")
		count := strings.Count(text, old)
		out := strings.ReplaceAll(text, old, newW)
		var b strings.Builder
		b.WriteString("*🔰 TEXT REPLACE 🔰*\n\n")
		b.WriteString("*🔍 FIND ❯ " + old + "*\n")
		b.WriteString("*♻️ REPLACE ❯ " + newW + "*\n")
		b.WriteString("*🔢 REPLACED ❯ " + strconv.Itoa(count) + " TIMES*\n\n")
		b.WriteString("*✅ RESULT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .TEXTBANNER ─────────────────────────────────────────────────────────────

func textbannerGuide(prefix string) string {
	return "*🔰 TEXT BANNER 🔰*\n\n" +
		"*TURN YOUR TEXT INTO BIG ASCII ART*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTBANNER <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTBANNER GOLD ❯*"
}

var bannerFont = map[rune][]string{
	'A': {" ### ", "#   #", "#####", "#   #", "#   #"},
	'B': {"#### ", "#   #", "#### ", "#   #", "#### "},
	'C': {" ####", "#    ", "#    ", "#    ", " ####"},
	'D': {"#### ", "#   #", "#   #", "#   #", "#### "},
	'E': {"#####", "#    ", "#### ", "#    ", "#####"},
	'F': {"#####", "#    ", "#### ", "#    ", "#    "},
	'G': {" ####", "#    ", "#  ##", "#   #", " ####"},
	'H': {"#   #", "#   #", "#####", "#   #", "#   #"},
	'I': {"#####", "  #  ", "  #  ", "  #  ", "#####"},
	'J': {"#####", "   # ", "   # ", "#  # ", " ##  "},
	'K': {"#   #", "#  # ", "###  ", "#  # ", "#   #"},
	'L': {"#    ", "#    ", "#    ", "#    ", "#####"},
	'M': {"#   #", "## ##", "# # #", "#   #", "#   #"},
	'N': {"#   #", "##  #", "# # #", "#  ##", "#   #"},
	'O': {" ### ", "#   #", "#   #", "#   #", " ### "},
	'P': {"#### ", "#   #", "#### ", "#    ", "#    "},
	'Q': {" ### ", "#   #", "# # #", "#  # ", " ## #"},
	'R': {"#### ", "#   #", "#### ", "#  # ", "#   #"},
	'S': {" ####", "#    ", " ### ", "    #", "#### "},
	'T': {"#####", "  #  ", "  #  ", "  #  ", "  #  "},
	'U': {"#   #", "#   #", "#   #", "#   #", " ### "},
	'V': {"#   #", "#   #", "#   #", " # # ", "  #  "},
	'W': {"#   #", "#   #", "# # #", "## ##", "#   #"},
	'X': {"#   #", " # # ", "  #  ", " # # ", "#   #"},
	'Y': {"#   #", " # # ", "  #  ", "  #  ", "  #  "},
	'Z': {"#####", "   # ", "  #  ", " #   ", "#####"},
	'0': {" ### ", "#  ##", "# # #", "##  #", " ### "},
	'1': {"  #  ", " ##  ", "  #  ", "  #  ", "#####"},
	'2': {" ### ", "#   #", "  ## ", " #   ", "#####"},
	'3': {"#####", "   # ", "  ## ", "#   #", " ### "},
	'4': {"#   #", "#   #", "#####", "    #", "    #"},
	'5': {"#####", "#    ", "#### ", "    #", "#### "},
	'6': {" ### ", "#    ", "#### ", "#   #", " ### "},
	'7': {"#####", "   # ", "  #  ", " #   ", "#    "},
	'8': {" ### ", "#   #", " ### ", "#   #", " ### "},
	'9': {" ### ", "#   #", " ####", "    #", " ### "},
	' ': {"     ", "     ", "     ", "     ", "     "},
}

func handleTextbanner(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.ToUpper(strings.Join(args, " "))
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textbannerGuide(prefix))
			return
		}
		rows := make([]strings.Builder, 5)
		for _, r := range text {
			glyph, ok := bannerFont[r]
			if !ok {
				glyph = bannerFont[' ']
			}
			for i := 0; i < 5; i++ {
				rows[i].WriteString(glyph[i])
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 TEXT BANNER 🔰*\n\n")
		for i := 0; i < 5; i++ {
			b.WriteString("*" + rows[i].String() + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .TEXTFLIP ───────────────────────────────────────────────────────────────

func textflipGuide(prefix string) string {
	return "*🔰 TEXT FLIP 🔰*\n\n" +
		"*FLIP YOUR TEXT UPSIDE DOWN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTFLIP <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTFLIP HELLO ❯*"
}

var flipMap = map[rune]rune{
	'a': 'ɐ', 'b': 'q', 'c': 'ɔ', 'd': 'p', 'e': 'ǝ', 'f': 'ɟ', 'g': 'ƃ', 'h': 'ɥ',
	'i': 'ᴉ', 'j': 'ɾ', 'k': 'ʞ', 'l': 'l', 'm': 'ɯ', 'n': 'u', 'o': 'o', 'p': 'd',
	'q': 'b', 'r': 'ɹ', 's': 's', 't': 'ʇ', 'u': 'n', 'v': 'ʌ', 'w': 'ʍ', 'x': 'x',
	'y': 'ʎ', 'z': 'z',
	'A': '∀', 'B': '𐐒', 'C': 'Ɔ', 'D': '◖', 'E': 'Ǝ', 'F': 'Ⅎ', 'G': '⅁', 'H': 'H',
	'I': 'I', 'J': 'ſ', 'K': 'ʞ', 'L': '˥', 'M': 'W', 'N': 'N', 'O': 'O', 'P': 'Ԁ',
	'Q': 'Ό', 'R': 'ᴚ', 'S': 'S', 'T': '⊥', 'U': '∩', 'V': 'Λ', 'W': 'M', 'X': 'X',
	'Y': '⅄', 'Z': 'Z',
	'0': '0', '1': 'Ɩ', '2': 'ᄅ', '3': 'Ɛ', '4': 'ㄣ', '5': 'ϛ', '6': '9', '7': 'ㄥ',
	'8': '8', '9': '6',
	'.': '˙', ',': '\'', '?': '¿', '!': '¡', '\'': ',', '"': '„', '(': ')', ')': '(',
	'[': ']', ']': '[', '{': '}', '}': '{', '<': '>', '>': '<', '&': '⅋', '_': '‾',
}

func handleTextflip(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textflipGuide(prefix))
			return
		}
		r := []rune(text)
		out := make([]rune, 0, len(r))
		for i := len(r) - 1; i >= 0; i-- {
			if f, ok := flipMap[r[i]]; ok {
				out = append(out, f)
			} else {
				out = append(out, r[i])
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 TEXT FLIP 🔰*\n\n")
		b.WriteString("*📝 ORIGINAL ❯ " + text + "*\n")
		b.WriteString("*🙃 FLIPPED ❯ " + string(out) + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "charfreq", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT HOW MANY TIMES EACH CHARACTER APPEARS IN TEXT. USE IT AS .CHARFREQ <TEXT>.", Run: handleCharfreq})
	Register(Command{Name: "uniquewords", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND ALL UNIQUE WORDS IN YOUR TEXT. USE IT AS .UNIQUEWORDS <TEXT>.", Run: handleUniquewords})
	Register(Command{Name: "longestword", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE LONGEST WORD IN YOUR TEXT. USE IT AS .LONGESTWORD <TEXT>.", Run: handleLongestword})
	Register(Command{Name: "textreverse", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO REVERSE TEXT BY CHARACTERS OR WORDS. USE IT AS .TEXTREVERSE <CHARS|WORDS> <TEXT>.", Run: handleTextreverse})
	Register(Command{Name: "textwrap", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO WRAP LONG TEXT INTO LINES OF N CHARACTERS. USE IT AS .TEXTWRAP <N> <TEXT>.", Run: handleTextwrap})
	Register(Command{Name: "texttrim", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO REMOVE EXTRA SPACES AND TRIM TEXT. USE IT AS .TEXTTRIM <TEXT>.", Run: handleTexttrim})
	Register(Command{Name: "textsplit", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SPLIT TEXT INTO PARTS USING A DELIMITER. USE IT AS .TEXTSPLIT <DELIMITER> <TEXT>.", Run: handleTextsplit})
	Register(Command{Name: "textreplace", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO REPLACE A WORD OR PHRASE IN TEXT. USE IT AS .TEXTREPLACE <OLD> <NEW> <TEXT>.", Run: handleTextreplace})
	Register(Command{Name: "textbanner", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO TURN TEXT INTO BIG ASCII ART. USE IT AS .TEXTBANNER <TEXT>.", Run: handleTextbanner})
	Register(Command{Name: "textflip", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FLIP TEXT UPSIDE DOWN. USE IT AS .TEXTFLIP <TEXT>.", Run: handleTextflip})

	// hidden aliases
	Register(Command{Name: "charfrequency", Category: "TOOLS", Desc: "Short alias of .charfreq", Hidden: true, Run: handleCharfreq})
	Register(Command{Name: "uniqwords", Category: "TOOLS", Desc: "Short alias of .uniquewords", Hidden: true, Run: handleUniquewords})
	Register(Command{Name: "biggestword", Category: "TOOLS", Desc: "Short alias of .longestword", Hidden: true, Run: handleLongestword})
	Register(Command{Name: "revtext", Category: "TOOLS", Desc: "Short alias of .textreverse", Hidden: true, Run: handleTextreverse})
	Register(Command{Name: "wrap", Category: "TOOLS", Desc: "Short alias of .textwrap", Hidden: true, Run: handleTextwrap})
	Register(Command{Name: "trimtext", Category: "TOOLS", Desc: "Short alias of .texttrim", Hidden: true, Run: handleTexttrim})
	Register(Command{Name: "splittext", Category: "TOOLS", Desc: "Short alias of .textsplit", Hidden: true, Run: handleTextsplit})
	Register(Command{Name: "replacetext", Category: "TOOLS", Desc: "Short alias of .textreplace", Hidden: true, Run: handleTextreplace})
	Register(Command{Name: "banner", Category: "TOOLS", Desc: "Short alias of .textbanner", Hidden: true, Run: handleTextbanner})
	Register(Command{Name: "fliptext", Category: "TOOLS", Desc: "Short alias of .textflip", Hidden: true, Run: handleTextflip})
}
