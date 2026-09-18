package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 36 (10 number, roman & words converters)
// File: convpack36.go
// ============================================================================
//   .numtoroman   -> number to roman numerals
//   .romantonum   -> roman numerals to number
//   .numtowords   -> number to words
//   .wordstonum   -> words to number
//   .numtobinary  -> number to binary
//   .numtohex     -> number to hexadecimal
//   .numtooctal   -> number to octal
//   .numtobase36  -> number to base36
//   .base36tonum  -> base36 to number
//   .numtobase32  -> number to base32
// ============================================================================

import (
	"context"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

var convRomanVals = []struct {
	val int
	sym string
}{
	{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
	{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
	{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
}

func convIntToRoman(n int) string {
	if n <= 0 || n > 3999 {
		return ""
	}
	var b strings.Builder
	for _, rv := range convRomanVals {
		for n >= rv.val {
			b.WriteString(rv.sym)
			n -= rv.val
		}
	}
	return b.String()
}

func convRomanToInt(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	m := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	for i := 0; i < len(s); i++ {
		if _, ok := m[s[i]]; !ok {
			return -1
		}
		if i+1 < len(s) && m[s[i]] < m[s[i+1]] {
			total -= m[s[i]]
		} else {
			total += m[s[i]]
		}
	}
	return total
}

var convOnes = []string{"", "ONE", "TWO", "THREE", "FOUR", "FIVE", "SIX", "SEVEN", "EIGHT", "NINE", "TEN", "ELEVEN", "TWELVE", "THIRTEEN", "FOURTEEN", "FIFTEEN", "SIXTEEN", "SEVENTEEN", "EIGHTEEN", "NINETEEN"}
var convTens = []string{"", "", "TWENTY", "THIRTY", "FORTY", "FIFTY", "SIXTY", "SEVENTY", "EIGHTY", "NINETY"}

func convNumToWords(n int) string {
	if n == 0 {
		return "ZERO"
	}
	if n < 0 {
		return "MINUS " + convNumToWords(-n)
	}
	var parts []string
	if n >= 1000000 {
		parts = append(parts, convNumToWords(n/1000000)+" MILLION")
		n %= 1000000
	}
	if n >= 1000 {
		parts = append(parts, convNumToWords(n/1000)+" THOUSAND")
		n %= 1000
	}
	if n >= 100 {
		parts = append(parts, convOnes[n/100]+" HUNDRED")
		n %= 100
	}
	if n >= 20 {
		w := convTens[n/10]
		if n%10 != 0 {
			w += " " + convOnes[n%10]
		}
		parts = append(parts, w)
	} else if n > 0 {
		parts = append(parts, convOnes[n])
	}
	return strings.Join(parts, " ")
}

func numConvReply(s SessionBridge, info types.MessageInfo, title, inLabel, inVal, outLabel, outVal string) {
	var b strings.Builder
	b.WriteString("*🔰 " + title + " 🔰*\n\n")
	b.WriteString("*📥 " + inLabel + " ❯ " + inVal + "*\n")
	b.WriteString("*📤 " + outLabel + " ❯ " + outVal + "*")
	s.Reply(info, b.String())
}

func handleNumtoroman(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "NUMBER TO ROMAN", "numtoroman", "NUMBER", "ROMAN"))
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A WHOLE NUMBER*")
			return
		}
		r := convIntToRoman(n)
		if r == "" {
			s.Reply(info, "*🔰 ROMAN NUMERALS SUPPORT 1 TO 3999 ONLY*")
			return
		}
		numConvReply(s, info, "NUMBER TO ROMAN", "NUMBER", strconv.Itoa(n), "ROMAN", r)
	})
}

func handleRomantonum(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "ROMAN TO NUMBER", "romantonum", "ROMAN", "NUMBER"))
			return
		}
		raw := strings.TrimSpace(args[0])
		n := convRomanToInt(raw)
		if n < 0 {
			s.Reply(info, "*🔰 INVALID ROMAN NUMERAL, PLEASE CHECK YOUR INPUT*")
			return
		}
		numConvReply(s, info, "ROMAN TO NUMBER", "ROMAN", strings.ToUpper(raw), "NUMBER", strconv.Itoa(n))
	})
}

func handleNumtowords(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "NUMBER TO WORDS", "numtowords", "NUMBER", "WORDS"))
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A WHOLE NUMBER*")
			return
		}
		numConvReply(s, info, "NUMBER TO WORDS", "NUMBER", strconv.Itoa(n), "WORDS", convNumToWords(n))
	})
}

func handleWordstonum(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "WORDS TO NUMBER", "wordstonum", "WORDS", "NUMBER"))
			return
		}
		words := strings.Fields(strings.ToUpper(raw))
		ones := map[string]int{"ZERO": 0, "ONE": 1, "TWO": 2, "THREE": 3, "FOUR": 4, "FIVE": 5, "SIX": 6, "SEVEN": 7, "EIGHT": 8, "NINE": 9, "TEN": 10, "ELEVEN": 11, "TWELVE": 12, "THIRTEEN": 13, "FOURTEEN": 14, "FIFTEEN": 15, "SIXTEEN": 16, "SEVENTEEN": 17, "EIGHTEEN": 18, "NINETEEN": 19}
		tens := map[string]int{"TWENTY": 20, "THIRTY": 30, "FORTY": 40, "FIFTY": 50, "SIXTY": 60, "SEVENTY": 70, "EIGHTY": 80, "NINETY": 90}
		total, cur := 0, 0
		for _, w := range words {
			switch {
			case w == "AND":
				continue
			case w == "HUNDRED":
				if cur == 0 {
					cur = 1
				}
				cur *= 100
			case w == "THOUSAND":
				if cur == 0 {
					cur = 1
				}
				total += cur * 1000
				cur = 0
			case w == "MILLION":
				if cur == 0 {
					cur = 1
				}
				total += cur * 1000000
				cur = 0
			default:
				if v, ok := ones[w]; ok {
					cur += v
				} else if v, ok := tens[w]; ok {
					cur += v
				} else {
					s.Reply(info, "*🔰 UNKNOWN WORD ❯ "+w+"*")
					return
				}
			}
		}
		numConvReply(s, info, "WORDS TO NUMBER", "WORDS", strings.ToUpper(raw), "NUMBER", strconv.Itoa(total+cur))
	})
}

func numBaseHandler(title, cmd string, base int, baseName string) func(SessionBridge, types.MessageInfo, []string, string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		RunWithTimeout(s, info, func(ctx context.Context) {
			if len(args) == 0 {
				s.Reply(info, convGuide(prefix, title, cmd, "NUMBER", baseName))
				return
			}
			n, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A WHOLE NUMBER*")
				return
			}
			numConvReply(s, info, title, "NUMBER", strconv.FormatInt(n, 10), baseName, strings.ToUpper(strconv.FormatInt(n, base)))
		})
	}
}

func handleBase36tonum(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "BASE36 TO NUMBER", "base36tonum", "BASE36", "NUMBER"))
			return
		}
		raw := strings.TrimSpace(args[0])
		n, err := strconv.ParseInt(raw, 36, 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID BASE36, PLEASE CHECK YOUR INPUT*")
			return
		}
		numConvReply(s, info, "BASE36 TO NUMBER", "BASE36", strings.ToUpper(raw), "NUMBER", strconv.FormatInt(n, 10))
	})
}

func init() {
	Register(Command{Name: "numtoroman", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO ROMAN NUMERALS. USE IT AS .NUMTOROMAN <NUMBER>.", Run: handleNumtoroman})
	Register(Command{Name: "romantonum", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT ROMAN NUMERALS TO A NUMBER. USE IT AS .ROMANTONUM <ROMAN>.", Run: handleRomantonum})
	Register(Command{Name: "numtowords", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO WORDS. USE IT AS .NUMTOWORDS <NUMBER>.", Run: handleNumtowords})
	Register(Command{Name: "wordstonum", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT WORDS TO A NUMBER. USE IT AS .WORDSTONUM <WORDS>.", Run: handleWordstonum})
	Register(Command{Name: "numtobinary", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO BINARY. USE IT AS .NUMTOBINARY <NUMBER>.", Run: numBaseHandler("NUMBER TO BINARY", "numtobinary", 2, "BINARY")})
	Register(Command{Name: "numtohex", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO HEXADECIMAL. USE IT AS .NUMTOHEX <NUMBER>.", Run: numBaseHandler("NUMBER TO HEX", "numtohex", 16, "HEX")})
	Register(Command{Name: "numtooctal", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO OCTAL. USE IT AS .NUMTOOCTAL <NUMBER>.", Run: numBaseHandler("NUMBER TO OCTAL", "numtooctal", 8, "OCTAL")})
	Register(Command{Name: "numtobase36", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO BASE36. USE IT AS .NUMTOBASE36 <NUMBER>.", Run: numBaseHandler("NUMBER TO BASE36", "numtobase36", 36, "BASE36")})
	Register(Command{Name: "base36tonum", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BASE36 TO A NUMBER. USE IT AS .BASE36TONUM <BASE36>.", Run: handleBase36tonum})
	Register(Command{Name: "numtobase32", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A NUMBER TO BASE32. USE IT AS .NUMTOBASE32 <NUMBER>.", Run: numBaseHandler("NUMBER TO BASE32", "numtobase32", 32, "BASE32")})

	// hidden aliases
	Register(Command{Name: "toroman", Category: "CONVERTER", Desc: "Short alias of .numtoroman", Hidden: true, Run: handleNumtoroman})
	Register(Command{Name: "fromroman", Category: "CONVERTER", Desc: "Short alias of .romantonum", Hidden: true, Run: handleRomantonum})
	Register(Command{Name: "towords", Category: "CONVERTER", Desc: "Short alias of .numtowords", Hidden: true, Run: handleNumtowords})
	Register(Command{Name: "fromwords", Category: "CONVERTER", Desc: "Short alias of .wordstonum", Hidden: true, Run: handleWordstonum})
	Register(Command{Name: "tobin", Category: "CONVERTER", Desc: "Short alias of .numtobinary", Hidden: true, Run: numBaseHandler("NUMBER TO BINARY", "numtobinary", 2, "BINARY")})
	Register(Command{Name: "tohexnum", Category: "CONVERTER", Desc: "Short alias of .numtohex", Hidden: true, Run: numBaseHandler("NUMBER TO HEX", "numtohex", 16, "HEX")})
	Register(Command{Name: "tooct", Category: "CONVERTER", Desc: "Short alias of .numtooctal", Hidden: true, Run: numBaseHandler("NUMBER TO OCTAL", "numtooctal", 8, "OCTAL")})
	Register(Command{Name: "tobase36", Category: "CONVERTER", Desc: "Short alias of .numtobase36", Hidden: true, Run: numBaseHandler("NUMBER TO BASE36", "numtobase36", 36, "BASE36")})
	Register(Command{Name: "frombase36", Category: "CONVERTER", Desc: "Short alias of .base36tonum", Hidden: true, Run: handleBase36tonum})
	Register(Command{Name: "tobase32", Category: "CONVERTER", Desc: "Short alias of .numtobase32", Hidden: true, Run: numBaseHandler("NUMBER TO BASE32", "numtobase32", 32, "BASE32")})
}
