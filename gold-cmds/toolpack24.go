package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 24 (10 new text analysis & manipulation commands)
// File: toolpack24.go
// ============================================================================
//   .readingtime <text>    -> estimated reading time
//   .speakingtime <text>   -> estimated speaking time
//   .isogram <word>        -> check if a word is an isogram
//   .pangram <text>        -> check if text is a pangram
//   .palindromecheck <text>-> check if text is a palindrome
//   .anagramcheck a | b    -> check if two words are anagrams
//   .wordshuffle <text>    -> shuffle the words in a sentence
//   .textrepeat <n> <text> -> repeat text n times
//   .textpad <n> <text>    -> pad text to a fixed width
//   .textcenter <n> <text> -> center text within a width
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow/types"
)

// ── .READINGTIME ─────────────────────────────────────────────────────────────

func readingtimeGuide(prefix string) string {
	return "*🔰 READING TIME 🔰*\n\n" +
		"*ESTIMATE HOW LONG TEXT TAKES TO READ*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "READINGTIME <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "READINGTIME your long text here ❯*"
}

func handleReadingtime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, readingtimeGuide(prefix))
			return
		}
		words := len(strings.Fields(text))
		// average reading speed ~200 wpm
		secs := float64(words) / 200.0 * 60.0
		var out strings.Builder
		out.WriteString("*🔰 READING TIME 🔰*\n\n")
		out.WriteString("*📝 WORDS ❯ " + strconv.Itoa(words) + "*\n")
		out.WriteString("*🔤 CHARACTERS ❯ " + strconv.Itoa(len([]rune(text))) + "*\n\n")
		out.WriteString("*⏱️ READING TIME ❯ " + humanDuration(secs) + "*\n")
		out.WriteString("*📖 AT 200 WORDS / MINUTE*")
		s.Reply(info, out.String())
	})
}

func humanDuration(secs float64) string {
	if secs < 1 {
		return "LESS THAN 1 SECOND"
	}
	if secs < 60 {
		return strconv.Itoa(int(secs+0.5)) + " SECONDS"
	}
	mins := int(secs / 60)
	rem := int(secs) % 60
	if rem == 0 {
		return strconv.Itoa(mins) + " MINUTE(S)"
	}
	return strconv.Itoa(mins) + " MIN " + strconv.Itoa(rem) + " SEC"
}

// ── .SPEAKINGTIME ────────────────────────────────────────────────────────────

func speakingtimeGuide(prefix string) string {
	return "*🔰 SPEAKING TIME 🔰*\n\n" +
		"*ESTIMATE HOW LONG TEXT TAKES TO SPEAK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SPEAKINGTIME <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SPEAKINGTIME your speech text ❯*"
}

func handleSpeakingtime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, speakingtimeGuide(prefix))
			return
		}
		words := len(strings.Fields(text))
		// average speaking speed ~130 wpm
		secs := float64(words) / 130.0 * 60.0
		var out strings.Builder
		out.WriteString("*🔰 SPEAKING TIME 🔰*\n\n")
		out.WriteString("*📝 WORDS ❯ " + strconv.Itoa(words) + "*\n\n")
		out.WriteString("*🎤 SPEAKING TIME ❯ " + humanDuration(secs) + "*\n")
		out.WriteString("*🗣️ AT 130 WORDS / MINUTE*")
		s.Reply(info, out.String())
	})
}

// ── .ISOGRAM ─────────────────────────────────────────────────────────────────

func isogramGuide(prefix string) string {
	return "*🔰 ISOGRAM CHECK 🔰*\n\n" +
		"*CHECK IF A WORD HAS NO REPEATED LETTERS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ISOGRAM <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ISOGRAM DERMATOGLYPHICS ❯*"
}

func handleIsogram(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.Join(args, " ")
		if strings.TrimSpace(word) == "" {
			s.Reply(info, isogramGuide(prefix))
			return
		}
		seen := map[rune]int{}
		for _, r := range strings.ToLower(word) {
			if r >= 'a' && r <= 'z' {
				seen[r]++
			}
		}
		repeats := []string{}
		for r, c := range seen {
			if c > 1 {
				repeats = append(repeats, string(r)+" x"+strconv.Itoa(c))
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 ISOGRAM CHECK 🔰*\n\n")
		out.WriteString("*📝 WORD ❯ " + strings.ToUpper(word) + "*\n\n")
		if len(repeats) == 0 {
			out.WriteString("*✅ RESULT ❯ YES, IT IS AN ISOGRAM*")
		} else {
			out.WriteString("*❌ RESULT ❯ NO, IT HAS REPEATED LETTERS*\n")
			out.WriteString("*🔁 REPEATS ❯ " + strings.Join(repeats, ", ") + "*")
		}
		s.Reply(info, out.String())
	})
}

// ── .PANGRAM ─────────────────────────────────────────────────────────────────

func pangramGuide(prefix string) string {
	return "*🔰 PANGRAM CHECK 🔰*\n\n" +
		"*CHECK IF TEXT USES EVERY LETTER A-Z*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PANGRAM <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PANGRAM the quick brown fox jumps over the lazy dog ❯*"
}

func handlePangram(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, pangramGuide(prefix))
			return
		}
		seen := map[rune]bool{}
		for _, r := range strings.ToLower(text) {
			if r >= 'a' && r <= 'z' {
				seen[r] = true
			}
		}
		missing := []string{}
		for r := 'a'; r <= 'z'; r++ {
			if !seen[r] {
				missing = append(missing, strings.ToUpper(string(r)))
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 PANGRAM CHECK 🔰*\n\n")
		out.WriteString("*🔤 LETTERS USED ❯ " + strconv.Itoa(len(seen)) + " / 26*\n\n")
		if len(missing) == 0 {
			out.WriteString("*✅ RESULT ❯ YES, IT IS A PANGRAM*")
		} else {
			out.WriteString("*❌ RESULT ❯ NO, NOT A PANGRAM*\n")
			out.WriteString("*🔍 MISSING ❯ " + strings.Join(missing, " ") + "*")
		}
		s.Reply(info, out.String())
	})
}

// ── .PALINDROMECHECK ─────────────────────────────────────────────────────────

func palindromecheckGuide(prefix string) string {
	return "*🔰 PALINDROME CHECK 🔰*\n\n" +
		"*CHECK IF TEXT READS THE SAME BACKWARDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PALINDROMECHECK <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PALINDROMECHECK racecar ❯*"
}

func handlePalindromecheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, palindromecheckGuide(prefix))
			return
		}
		var clean []rune
		for _, r := range strings.ToLower(text) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				clean = append(clean, r)
			}
		}
		isPal := true
		for i, j := 0, len(clean)-1; i < j; i, j = i+1, j-1 {
			if clean[i] != clean[j] {
				isPal = false
				break
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 PALINDROME CHECK 🔰*\n\n")
		out.WriteString("*📝 TEXT ❯ " + strings.ToUpper(text) + "*\n\n")
		if isPal {
			out.WriteString("*✅ RESULT ❯ YES, IT IS A PALINDROME*")
		} else {
			out.WriteString("*❌ RESULT ❯ NO, NOT A PALINDROME*")
		}
		s.Reply(info, out.String())
	})
}

// ── .ANAGRAMCHECK ────────────────────────────────────────────────────────────

func anagramcheckGuide(prefix string) string {
	return "*🔰 ANAGRAM CHECK 🔰*\n\n" +
		"*CHECK IF TWO WORDS ARE ANAGRAMS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ANAGRAMCHECK WORD1 | WORD2 ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ANAGRAMCHECK listen | silent ❯*"
}

func handleAnagramcheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, anagramcheckGuide(prefix))
			return
		}
		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 {
			s.Reply(info, "*🔰 ANAGRAM CHECK 🔰*\n\n*❌ SEPARATE THE TWO WORDS WITH A PIPE |*")
			return
		}
		a := normalizeLetters(parts[0])
		b := normalizeLetters(parts[1])
		if a == "" || b == "" {
			s.Reply(info, "*🔰 ANAGRAM CHECK 🔰*\n\n*❌ BOTH WORDS MUST CONTAIN LETTERS*")
			return
		}
		isAna := a == b
		var out strings.Builder
		out.WriteString("*🔰 ANAGRAM CHECK 🔰*\n\n")
		out.WriteString("*1️⃣ " + strings.ToUpper(strings.TrimSpace(parts[0])) + "*\n")
		out.WriteString("*2️⃣ " + strings.ToUpper(strings.TrimSpace(parts[1])) + "*\n\n")
		if isAna {
			out.WriteString("*✅ RESULT ❯ YES, THEY ARE ANAGRAMS*")
		} else {
			out.WriteString("*❌ RESULT ❯ NO, NOT ANAGRAMS*")
		}
		s.Reply(info, out.String())
	})
}

func normalizeLetters(s string) string {
	var b []rune
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b = append(b, r)
		}
	}
	// simple sort
	for i := 0; i < len(b); i++ {
		for j := i + 1; j < len(b); j++ {
			if b[j] < b[i] {
				b[i], b[j] = b[j], b[i]
			}
		}
	}
	return string(b)
}

// ── .WORDSHUFFLE ─────────────────────────────────────────────────────────────

func wordshuffleGuide(prefix string) string {
	return "*🔰 WORD SHUFFLE 🔰*\n\n" +
		"*RANDOMLY SHUFFLE THE WORDS IN A SENTENCE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORDSHUFFLE <SENTENCE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WORDSHUFFLE hello world how are you ❯*"
}

func handleWordshuffle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, wordshuffleGuide(prefix))
			return
		}
		words := strings.Fields(text)
		for i := len(words) - 1; i > 0; i-- {
			j := randIntn(i + 1)
			words[i], words[j] = words[j], words[i]
		}
		var out strings.Builder
		out.WriteString("*🔰 WORD SHUFFLE 🔰*\n\n")
		out.WriteString("*📝 ORIGINAL ❯ " + text + "*\n\n")
		out.WriteString("*🔀 SHUFFLED ❯ " + strings.Join(words, " ") + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTREPEAT ──────────────────────────────────────────────────────────────

func textrepeatGuide(prefix string) string {
	return "*🔰 TEXT REPEAT 🔰*\n\n" +
		"*REPEAT TEXT A NUMBER OF TIMES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTREPEAT <COUNT> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTREPEAT 3 hi ❯*"
}

func handleTextrepeat(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textrepeatGuide(prefix))
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || n < 1 || n > 50 {
			s.Reply(info, "*🔰 TEXT REPEAT 🔰*\n\n*❌ COUNT MUST BE BETWEEN 1 AND 50*")
			return
		}
		text := strings.Join(args[1:], " ")
		repeated := strings.Repeat(text+" ", n)
		var out strings.Builder
		out.WriteString("*🔰 TEXT REPEAT 🔰*\n\n")
		out.WriteString("*🔢 COUNT ❯ " + strconv.Itoa(n) + "*\n\n")
		out.WriteString("*📝 RESULT ❯ " + strings.TrimSpace(repeated) + "*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTPAD ─────────────────────────────────────────────────────────────────

func textpadGuide(prefix string) string {
	return "*🔰 TEXT PAD 🔰*\n\n" +
		"*PAD TEXT TO A FIXED WIDTH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTPAD <WIDTH> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTPAD 20 hi ❯*"
}

func handleTextpad(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textpadGuide(prefix))
			return
		}
		w, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || w < 1 || w > 200 {
			s.Reply(info, "*🔰 TEXT PAD 🔰*\n\n*❌ WIDTH MUST BE BETWEEN 1 AND 200*")
			return
		}
		text := strings.Join(args[1:], " ")
		padded := text
		if len([]rune(text)) < w {
			padded = text + strings.Repeat(" ", w-len([]rune(text)))
		}
		var out strings.Builder
		out.WriteString("*🔰 TEXT PAD 🔰*\n\n")
		out.WriteString("*📏 WIDTH ❯ " + strconv.Itoa(w) + "*\n\n")
		out.WriteString("*`" + padded + "`*")
		s.Reply(info, out.String())
	})
}

// ── .TEXTCENTER ──────────────────────────────────────────────────────────────

func textcenterGuide(prefix string) string {
	return "*🔰 TEXT CENTER 🔰*\n\n" +
		"*CENTER TEXT WITHIN A FIXED WIDTH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTCENTER <WIDTH> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TEXTCENTER 20 hi ❯*"
}

func handleTextcenter(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, textcenterGuide(prefix))
			return
		}
		w, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || w < 1 || w > 200 {
			s.Reply(info, "*🔰 TEXT CENTER 🔰*\n\n*❌ WIDTH MUST BE BETWEEN 1 AND 200*")
			return
		}
		text := strings.Join(args[1:], " ")
		runes := []rune(text)
		if len(runes) >= w {
			var out strings.Builder
			out.WriteString("*🔰 TEXT CENTER 🔰*\n\n")
			out.WriteString("*`" + text + "`*")
			s.Reply(info, out.String())
			return
		}
		total := w - len(runes)
		left := total / 2
		right := total - left
		centered := strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
		var out strings.Builder
		out.WriteString("*🔰 TEXT CENTER 🔰*\n\n")
		out.WriteString("*📏 WIDTH ❯ " + strconv.Itoa(w) + "*\n\n")
		out.WriteString("*`" + centered + "`*")
		s.Reply(info, out.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "readingtime", Category: "TOOLS", Desc: "Estimate how long text takes to read", Run: handleReadingtime})
	Register(Command{Name: "speakingtime", Category: "TOOLS", Desc: "Estimate how long text takes to speak", Run: handleSpeakingtime})
	Register(Command{Name: "isogram", Category: "TOOLS", Desc: "Check if a word has no repeated letters", Run: handleIsogram})
	Register(Command{Name: "pangram", Category: "TOOLS", Desc: "Check if text uses every letter A-Z", Run: handlePangram})
	Register(Command{Name: "palindromecheck", Category: "TOOLS", Desc: "Check if text is a palindrome", Run: handlePalindromecheck})
	Register(Command{Name: "anagramcheck", Category: "TOOLS", Desc: "Check if two words are anagrams", Run: handleAnagramcheck})
	Register(Command{Name: "wordshuffle", Category: "TOOLS", Desc: "Shuffle the words in a sentence", Run: handleWordshuffle})
	Register(Command{Name: "textrepeat", Category: "TOOLS", Desc: "Repeat text a number of times", Run: handleTextrepeat})
	Register(Command{Name: "textpad", Category: "TOOLS", Desc: "Pad text to a fixed width", Run: handleTextpad})
	Register(Command{Name: "textcenter", Category: "TOOLS", Desc: "Center text within a fixed width", Run: handleTextcenter})

	Register(Command{Name: "readtime", Category: "TOOLS", Desc: "Short alias of .readingtime", Hidden: true, Run: handleReadingtime})
	Register(Command{Name: "speaktime", Category: "TOOLS", Desc: "Short alias of .speakingtime", Hidden: true, Run: handleSpeakingtime})
	Register(Command{Name: "isogramcheck", Category: "TOOLS", Desc: "Short alias of .isogram", Hidden: true, Run: handleIsogram})
	Register(Command{Name: "pangramcheck", Category: "TOOLS", Desc: "Short alias of .pangram", Hidden: true, Run: handlePangram})
	Register(Command{Name: "palcheck", Category: "TOOLS", Desc: "Short alias of .palindromecheck", Hidden: true, Run: handlePalindromecheck})
	Register(Command{Name: "anacheck", Category: "TOOLS", Desc: "Short alias of .anagramcheck", Hidden: true, Run: handleAnagramcheck})
	Register(Command{Name: "shufflewords", Category: "TOOLS", Desc: "Short alias of .wordshuffle", Hidden: true, Run: handleWordshuffle})
	Register(Command{Name: "repeattext", Category: "TOOLS", Desc: "Short alias of .textrepeat", Hidden: true, Run: handleTextrepeat})
	Register(Command{Name: "padtext", Category: "TOOLS", Desc: "Short alias of .textpad", Hidden: true, Run: handleTextpad})
	Register(Command{Name: "centertext", Category: "TOOLS", Desc: "Short alias of .textcenter", Hidden: true, Run: handleTextcenter})
}
