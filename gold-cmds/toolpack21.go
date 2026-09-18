package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 21 (10 new encoding & cipher commands)
// File: toolpack21.go
// ============================================================================
//   .rot13 <text>          -> ROT13 cipher
//   .caesar <shift> <text> -> Caesar cipher
//   .atbash <text>         -> Atbash cipher
//   .vigenere <key> <text> -> Vigenere cipher
//   .leetspeak <text>      -> convert text to leetspeak
//   .morseencode <text>    -> text to Morse code
//   .morsedecode <morse>   -> Morse code to text
//   .binaryencode <text>   -> text to binary
//   .binarydecode <binary> -> binary to text
//   .hexencode <text>      -> text to hex
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .ROT13 ──────────────────────────────────────────────────────────────────

func rot13Guide(prefix string) string {
	return "*🔰 ROT13 CIPHER 🔰*\n\n" +
		"*ENCODE OR DECODE TEXT WITH THE ROT13 CIPHER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ROT13 <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ROT13 HELLO ❯*"
}

func rot13Rune(r rune) rune {
	switch {
	case r >= 'a' && r <= 'z':
		return 'a' + (r-'a'+13)%26
	case r >= 'A' && r <= 'Z':
		return 'A' + (r-'A'+13)%26
	}
	return r
}

func handleRot13(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, rot13Guide(prefix))
			return
		}
		out := strings.Map(rot13Rune, text)
		var b strings.Builder
		b.WriteString("*🔰 ROT13 CIPHER 🔰*\n\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*🔐 OUTPUT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .CAESAR ─────────────────────────────────────────────────────────────────

func caesarGuide(prefix string) string {
	return "*🔰 CAESAR CIPHER 🔰*\n\n" +
		"*SHIFT LETTERS BY N POSITIONS (CLASSIC CAESAR CIPHER)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CAESAR <SHIFT> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CAESAR 3 HELLO ❯*"
}

func handleCaesar(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, caesarGuide(prefix))
			return
		}
		shift, err := strconv.Atoi(args[0])
		if err != nil {
			s.Reply(info, "*🔰 INVALID SHIFT, USE "+prefix+"CAESAR <SHIFT> <TEXT>*")
			return
		}
		shift = ((shift % 26) + 26) % 26
		text := strings.Join(args[1:], " ")
		out := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return 'a' + (r-'a'+rune(shift))%26
			case r >= 'A' && r <= 'Z':
				return 'A' + (r-'A'+rune(shift))%26
			}
			return r
		}, text)
		var b strings.Builder
		b.WriteString("*🔰 CAESAR CIPHER 🔰*\n\n")
		b.WriteString("*🔢 SHIFT ❯ " + strconv.Itoa(shift) + "*\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*🔐 OUTPUT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .ATBASH ─────────────────────────────────────────────────────────────────

func atbashGuide(prefix string) string {
	return "*🔰 ATBASH CIPHER 🔰*\n\n" +
		"*REVERSE THE ALPHABET (A=Z, B=Y, C=X...)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ATBASH <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ATBASH HELLO ❯*"
}

func handleAtbash(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, atbashGuide(prefix))
			return
		}
		out := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return 'z' - (r - 'a')
			case r >= 'A' && r <= 'Z':
				return 'Z' - (r - 'A')
			}
			return r
		}, text)
		var b strings.Builder
		b.WriteString("*🔰 ATBASH CIPHER 🔰*\n\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*🔐 OUTPUT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .VIGENERE ───────────────────────────────────────────────────────────────

func vigenereGuide(prefix string) string {
	return "*🔰 VIGENERE CIPHER 🔰*\n\n" +
		"*ENCODE TEXT WITH A KEYWORD USING THE VIGENERE CIPHER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "VIGENERE <KEY> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "VIGENERE KEY HELLO ❯*"
}

func handleVigenere(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, vigenereGuide(prefix))
			return
		}
		key := strings.ToLower(args[0])
		text := strings.Join(args[1:], " ")
		var keyRunes []rune
		for _, r := range key {
			if r >= 'a' && r <= 'z' {
				keyRunes = append(keyRunes, r-'a')
			}
		}
		if len(keyRunes) == 0 {
			s.Reply(info, "*🔰 KEY MUST CONTAIN LETTERS*")
			return
		}
		ki := 0
		out := strings.Map(func(r rune) rune {
			var base rune
			switch {
			case r >= 'a' && r <= 'z':
				base = 'a'
			case r >= 'A' && r <= 'Z':
				base = 'A'
			default:
				return r
			}
			shift := keyRunes[ki%len(keyRunes)]
			ki++
			return base + (r-base+shift)%26
		}, text)
		var b strings.Builder
		b.WriteString("*🔰 VIGENERE CIPHER 🔰*\n\n")
		b.WriteString("*🔑 KEY ❯ " + strings.ToUpper(key) + "*\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*🔐 OUTPUT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .LEETSPEAK ──────────────────────────────────────────────────────────────

func leetspeakGuide(prefix string) string {
	return "*🔰 LEETSPEAK 🔰*\n\n" +
		"*CONVERT YOUR TEXT INTO HACKER LEETSPEAK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LEETSPEAK <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LEETSPEAK ELITE ❯*"
}

var leetMap = map[rune]string{
	'a': "4", 'b': "8", 'e': "3", 'g': "6", 'i': "1", 'l': "1", 'o': "0",
	's': "5", 't': "7", 'z': "2",
	'A': "4", 'B': "8", 'E': "3", 'G': "6", 'I': "1", 'L': "1", 'O': "0",
	'S': "5", 'T': "7", 'Z': "2",
}

func handleLeetspeak(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, leetspeakGuide(prefix))
			return
		}
		var b strings.Builder
		for _, r := range text {
			if v, ok := leetMap[r]; ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
		var out strings.Builder
		out.WriteString("*🔰 LEETSPEAK 🔰*\n\n")
		out.WriteString("*📝 INPUT ❯ " + text + "*\n")
		out.WriteString("*💻 OUTPUT ❯ " + b.String() + "*")
		s.Reply(info, out.String())
	})
}

// ── .MORSEENCODE ────────────────────────────────────────────────────────────

func morseencodeGuide(prefix string) string {
	return "*🔰 MORSE ENCODE 🔰*\n\n" +
		"*CONVERT TEXT INTO MORSE CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MORSEENCODE <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MORSEENCODE SOS ❯*"
}

var morseCodeMap = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.",
	'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..",
	'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.",
	'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-",
	'Y': "-.--", 'Z': "--..",
	'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
	'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
}

func handleMorseencode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.ToUpper(strings.Join(args, " "))
		if strings.TrimSpace(text) == "" {
			s.Reply(info, morseencodeGuide(prefix))
			return
		}
		var parts []string
		for _, r := range text {
			if r == ' ' {
				parts = append(parts, "/")
				continue
			}
			if code, ok := morseCodeMap[r]; ok {
				parts = append(parts, code)
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 MORSE ENCODE 🔰*\n\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*📡 MORSE ❯ " + strings.Join(parts, " ") + "*")
		s.Reply(info, b.String())
	})
}

// ── .MORSEDECODE ────────────────────────────────────────────────────────────

func morsedecodeGuide(prefix string) string {
	return "*🔰 MORSE DECODE 🔰*\n\n" +
		"*CONVERT MORSE CODE BACK INTO TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MORSEDECODE <MORSE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MORSEDECODE ... --- ... ❯*"
}

func handleMorsedecode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, morsedecodeGuide(prefix))
			return
		}
		rev := map[string]rune{}
		for k, v := range morseCodeMap {
			rev[v] = k
		}
		var out strings.Builder
		for _, token := range strings.Fields(raw) {
			if token == "/" {
				out.WriteRune(' ')
				continue
			}
			if ch, ok := rev[token]; ok {
				out.WriteRune(ch)
			} else {
				out.WriteRune('?')
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 MORSE DECODE 🔰*\n\n")
		b.WriteString("*📡 MORSE ❯ " + raw + "*\n")
		b.WriteString("*📝 TEXT ❯ " + out.String() + "*")
		s.Reply(info, b.String())
	})
}

// ── .BINARYENCODE ───────────────────────────────────────────────────────────

func binaryencodeGuide(prefix string) string {
	return "*🔰 BINARY ENCODE 🔰*\n\n" +
		"*CONVERT TEXT INTO BINARY (8-BIT)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BINARYENCODE <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BINARYENCODE HI ❯*"
}

func handleBinaryencode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, binaryencodeGuide(prefix))
			return
		}
		var parts []string
		for _, by := range []byte(text) {
			parts = append(parts, strconv.FormatInt(int64(by), 2))
		}
		var b strings.Builder
		b.WriteString("*🔰 BINARY ENCODE 🔰*\n\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*💾 BINARY ❯ " + strings.Join(parts, " ") + "*")
		s.Reply(info, b.String())
	})
}

// ── .BINARYDECODE ───────────────────────────────────────────────────────────

func binarydecodeGuide(prefix string) string {
	return "*🔰 BINARY DECODE 🔰*\n\n" +
		"*CONVERT BINARY BACK INTO TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BINARYDECODE <BINARY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BINARYDECODE 01001000 01001001 ❯*"
}

func handleBinarydecode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, binarydecodeGuide(prefix))
			return
		}
		var out strings.Builder
		for _, token := range strings.Fields(raw) {
			n, err := strconv.ParseInt(token, 2, 64)
			if err != nil {
				out.WriteRune('?')
				continue
			}
			out.WriteByte(byte(n))
		}
		var b strings.Builder
		b.WriteString("*🔰 BINARY DECODE 🔰*\n\n")
		b.WriteString("*💾 BINARY ❯ " + raw + "*\n")
		b.WriteString("*📝 TEXT ❯ " + out.String() + "*")
		s.Reply(info, b.String())
	})
}

// ── .HEXENCODE ──────────────────────────────────────────────────────────────

func hexencodeGuide(prefix string) string {
	return "*🔰 HEX ENCODE 🔰*\n\n" +
		"*CONVERT TEXT INTO HEXADECIMAL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HEXENCODE <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "HEXENCODE HI ❯*"
}

func handleHexencode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, hexencodeGuide(prefix))
			return
		}
		enc := hex.EncodeToString([]byte(text))
		var b strings.Builder
		b.WriteString("*🔰 HEX ENCODE 🔰*\n\n")
		b.WriteString("*📝 INPUT ❯ " + text + "*\n")
		b.WriteString("*🔢 HEX ❯ " + strings.ToUpper(enc) + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "rot13", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ENCODE OR DECODE TEXT WITH THE ROT13 CIPHER. USE IT AS .ROT13 <TEXT>.", Run: handleRot13})
	Register(Command{Name: "caesar", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHIFT LETTERS WITH THE CAESAR CIPHER. USE IT AS .CAESAR <SHIFT> <TEXT>.", Run: handleCaesar})
	Register(Command{Name: "atbash", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO REVERSE THE ALPHABET WITH THE ATBASH CIPHER. USE IT AS .ATBASH <TEXT>.", Run: handleAtbash})
	Register(Command{Name: "vigenere", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ENCODE TEXT WITH A KEYWORD USING THE VIGENERE CIPHER. USE IT AS .VIGENERE <KEY> <TEXT>.", Run: handleVigenere})
	Register(Command{Name: "leetspeak", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT INTO LEETSPEAK. USE IT AS .LEETSPEAK <TEXT>.", Run: handleLeetspeak})
	Register(Command{Name: "morseencode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT INTO MORSE CODE. USE IT AS .MORSEENCODE <TEXT>.", Run: handleMorseencode})
	Register(Command{Name: "morsedecode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT MORSE CODE BACK INTO TEXT. USE IT AS .MORSEDECODE <MORSE>.", Run: handleMorsedecode})
	Register(Command{Name: "binaryencode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT INTO BINARY. USE IT AS .BINARYENCODE <TEXT>.", Run: handleBinaryencode})
	Register(Command{Name: "binarydecode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT BINARY BACK INTO TEXT. USE IT AS .BINARYDECODE <BINARY>.", Run: handleBinarydecode})
	Register(Command{Name: "hexencode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT INTO HEXADECIMAL. USE IT AS .HEXENCODE <TEXT>.", Run: handleHexencode})

	// hidden aliases
	Register(Command{Name: "rot13cipher", Category: "TOOLS", Desc: "Short alias of .rot13", Hidden: true, Run: handleRot13})
	Register(Command{Name: "caesarcipher", Category: "TOOLS", Desc: "Short alias of .caesar", Hidden: true, Run: handleCaesar})
	Register(Command{Name: "atbashcipher", Category: "TOOLS", Desc: "Short alias of .atbash", Hidden: true, Run: handleAtbash})
	Register(Command{Name: "vigenercipher", Category: "TOOLS", Desc: "Short alias of .vigenere", Hidden: true, Run: handleVigenere})
	Register(Command{Name: "leet", Category: "TOOLS", Desc: "Short alias of .leetspeak", Hidden: true, Run: handleLeetspeak})
	Register(Command{Name: "tomorse", Category: "TOOLS", Desc: "Short alias of .morseencode", Hidden: true, Run: handleMorseencode})
	Register(Command{Name: "frommorse", Category: "TOOLS", Desc: "Short alias of .morsedecode", Hidden: true, Run: handleMorsedecode})
	Register(Command{Name: "tobinary", Category: "TOOLS", Desc: "Short alias of .binaryencode", Hidden: true, Run: handleBinaryencode})
	Register(Command{Name: "frombinary", Category: "TOOLS", Desc: "Short alias of .binarydecode", Hidden: true, Run: handleBinarydecode})
	Register(Command{Name: "tohex", Category: "TOOLS", Desc: "Short alias of .hexencode", Hidden: true, Run: handleHexencode})
}
