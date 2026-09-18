package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 13 (10 new everyday commands)
// File: toolpack13.go
// ============================================================================
//   .timestamp <ts>      -> unix timestamp <-> date converter
//   .agecalc <date>      -> calculate age from birthdate
//   .unitconvert <v> <from> <to> -> unit conversion
//   .caseconvert <mode> <text>   -> change text case
//   .charcount <text>    -> count characters / words / lines
//   .palindrome <text>   -> check if text is a palindrome
//   .anagram <w1> <w2>   -> check if two words are anagrams
//   .loremipsum [n]      -> generate lorem ipsum text
//   .urlparse <url>      -> break a URL into parts
//   .emailvalidate <mail>-> validate an email address
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .TIMESTAMP ───────────────────────────────────────────────────────────────

func timestampGuide(prefix string) string {
	return "*🔰 TIMESTAMP CONVERTER 🔰*\n\n" +
		"*CONVERT UNIX TIMESTAMP TO DATE OR SHOW CURRENT TIME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIMESTAMP ❯*  (current time)\n" +
		"*❮ " + prefix + "TIMESTAMP <UNIX> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIMESTAMP 1700000000 ❯*"
}

func handleTimestamp(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		now := time.Now()
		var b strings.Builder
		b.WriteString("*🔰 TIMESTAMP CONVERTER 🔰*\n\n")
		if len(args) == 0 {
			b.WriteString("*🕒 CURRENT UNIX ❯ " + strconv.FormatInt(now.Unix(), 10) + "*\n")
			b.WriteString("*📅 UTC ❯ " + now.UTC().Format("2006-01-02 15:04:05") + "*\n")
			b.WriteString("*🌍 LOCAL ❯ " + now.Format("2006-01-02 15:04:05") + "*")
			s.Reply(info, b.String())
			return
		}
		raw := strings.TrimSpace(args[0])
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID TIMESTAMP, PLEASE GIVE A NUMBER*")
			return
		}
		t := time.Unix(ts, 0)
		b.WriteString("*🔢 UNIX ❯ " + raw + "*\n")
		b.WriteString("*📅 UTC ❯ " + t.UTC().Format("2006-01-02 15:04:05") + "*\n")
		b.WriteString("*🌍 LOCAL ❯ " + t.Format("2006-01-02 15:04:05") + "*\n")
		b.WriteString("*📆 DAY ❯ " + t.Weekday().String() + "*")
		s.Reply(info, b.String())
	})
}

// ── .AGECALC ─────────────────────────────────────────────────────────────────

func agecalcGuide(prefix string) string {
	return "*🔰 AGE CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR EXACT AGE FROM YOUR BIRTHDATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AGECALC <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "AGECALC 2000-01-15 ❯*"
}

func handleAgecalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, agecalcGuide(prefix))
			return
		}
		raw := strings.TrimSpace(args[0])
		dob, err := time.Parse("2006-01-02", raw)
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		now := time.Now()
		if dob.After(now) {
			s.Reply(info, "*🔰 BIRTHDATE CANNOT BE IN THE FUTURE*")
			return
		}
		years := now.Year() - dob.Year()
		months := int(now.Month()) - int(dob.Month())
		days := now.Day() - dob.Day()
		if days < 0 {
			months--
			prev := time.Date(now.Year(), now.Month(), 0, 0, 0, 0, 0, time.UTC)
			days += prev.Day()
		}
		if months < 0 {
			years--
			months += 12
		}
		totalDays := int(now.Sub(dob).Hours() / 24)
		var b strings.Builder
		b.WriteString("*🔰 AGE CALCULATOR 🔰*\n\n")
		b.WriteString("*🎂 BIRTHDATE ❯ " + dob.Format("02 Jan 2006") + "*\n")
		b.WriteString("*🎈 AGE ❯ " + strconv.Itoa(years) + " YEARS, " + strconv.Itoa(months) + " MONTHS, " + strconv.Itoa(days) + " DAYS*\n")
		b.WriteString("*📆 TOTAL DAYS ❯ " + strconv.Itoa(totalDays) + "*\n")
		b.WriteString("*⏰ TOTAL HOURS ❯ " + strconv.Itoa(totalDays*24) + "*")
		s.Reply(info, b.String())
	})
}

// ── .UNITCONVERT ─────────────────────────────────────────────────────────────

func unitconvertGuide(prefix string) string {
	return "*🔰 UNIT CONVERTER 🔰*\n\n" +
		"*CONVERT BETWEEN UNITS (LENGTH, WEIGHT, TEMPERATURE)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UNITCONVERT <VALUE> <FROM> <TO> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "UNITCONVERT 10 KM MI ❯*\n" +
		"*UNITS ❯ KM MI M FT CM IN KG LB G OZ C F K*"
}

func handleUnitconvert(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, unitconvertGuide(prefix))
			return
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID VALUE, PLEASE GIVE A NUMBER*")
			return
		}
		from := strings.ToLower(strings.TrimSpace(args[1]))
		to := strings.ToLower(strings.TrimSpace(args[2]))
		res, ok := convertUnit(val, from, to)
		if !ok {
			s.Reply(info, "*🔰 UNSUPPORTED UNIT CONVERSION*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 UNIT CONVERTER 🔰*\n\n")
		b.WriteString("*📥 INPUT ❯ " + strconv.FormatFloat(val, 'f', -1, 64) + " " + strings.ToUpper(from) + "*\n")
		b.WriteString("*📤 RESULT ❯ " + strconv.FormatFloat(res, 'f', -1, 64) + " " + strings.ToUpper(to) + "*")
		s.Reply(info, b.String())
	})
}

// convertUnit converts val from one unit to another. Returns ok=false when the
// pair is unsupported.
func convertUnit(val float64, from, to string) (float64, bool) {
	if from == to {
		return val, true
	}
	// Temperature handled separately (non-linear).
	temp := map[string]bool{"c": true, "f": true, "k": true}
	if temp[from] || temp[to] {
		if !temp[from] || !temp[to] {
			return 0, false
		}
		// to Celsius first
		var c float64
		switch from {
		case "c":
			c = val
		case "f":
			c = (val - 32) * 5 / 9
		case "k":
			c = val - 273.15
		}
		switch to {
		case "c":
			return c, true
		case "f":
			return c*9/5 + 32, true
		case "k":
			return c + 273.15, true
		}
	}
	// Linear units → factor to base (meters for length, grams for weight).
	length := map[string]float64{"km": 1000, "m": 1, "cm": 0.01, "mm": 0.001, "mi": 1609.344, "ft": 0.3048, "in": 0.0254, "yd": 0.9144}
	weight := map[string]float64{"kg": 1000, "g": 1, "mg": 0.001, "lb": 453.59237, "oz": 28.349523125, "t": 1000000}
	if f, ok := length[from]; ok {
		if t, ok2 := length[to]; ok2 {
			return val * f / t, true
		}
		return 0, false
	}
	if f, ok := weight[from]; ok {
		if t, ok2 := weight[to]; ok2 {
			return val * f / t, true
		}
		return 0, false
	}
	return 0, false
}

// ── .CASECONVERT ─────────────────────────────────────────────────────────────

func caseconvertGuide(prefix string) string {
	return "*🔰 CASE CONVERTER 🔰*\n\n" +
		"*CHANGE THE CASE OF ANY TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CASECONVERT <MODE> <TEXT> ❯*\n" +
		"*MODES ❯ UPPER LOWER TITLE SENTENCE REVERSE*\n" +
		"*EXAMPLE ❮ " + prefix + "CASECONVERT UPPER hello world ❯*"
}

func handleCaseconvert(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, caseconvertGuide(prefix))
			return
		}
		mode := strings.ToLower(strings.TrimSpace(args[0]))
		text := strings.Join(args[1:], " ")
		var out string
		switch mode {
		case "upper":
			out = strings.ToUpper(text)
		case "lower":
			out = strings.ToLower(text)
		case "title":
			out = strings.Title(strings.ToLower(text))
		case "sentence":
			lower := strings.ToLower(text)
			if len(lower) > 0 {
				out = strings.ToUpper(lower[:1]) + lower[1:]
			}
		case "reverse":
			r := []rune(text)
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
			out = string(r)
		default:
			s.Reply(info, "*🔰 UNKNOWN MODE, USE UPPER / LOWER / TITLE / SENTENCE / REVERSE*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 CASE CONVERTER 🔰*\n\n")
		b.WriteString("*🔤 MODE ❯ " + strings.ToUpper(mode) + "*\n")
		b.WriteString("*📝 RESULT ❯ " + out + "*")
		s.Reply(info, b.String())
	})
}

// ── .CHARCOUNT ───────────────────────────────────────────────────────────────

func charcountGuide(prefix string) string {
	return "*🔰 CHARACTER COUNTER 🔰*\n\n" +
		"*COUNT CHARACTERS, WORDS AND LINES IN ANY TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CHARCOUNT <TEXT> ❯*"
}

func handleCharcount(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, charcountGuide(prefix))
			return
		}
		chars := len([]rune(text))
		charsNoSpace := len([]rune(strings.ReplaceAll(text, " ", "")))
		words := len(strings.Fields(text))
		lines := len(strings.Split(text, "\n"))
		var b strings.Builder
		b.WriteString("*🔰 CHARACTER COUNTER 🔰*\n\n")
		b.WriteString("*🔡 CHARACTERS ❯ " + strconv.Itoa(chars) + "*\n")
		b.WriteString("*🔠 WITHOUT SPACES ❯ " + strconv.Itoa(charsNoSpace) + "*\n")
		b.WriteString("*📝 WORDS ❯ " + strconv.Itoa(words) + "*\n")
		b.WriteString("*📄 LINES ❯ " + strconv.Itoa(lines) + "*")
		s.Reply(info, b.String())
	})
}

// ── .PALINDROME ──────────────────────────────────────────────────────────────

func palindromeGuide(prefix string) string {
	return "*🔰 PALINDROME CHECKER 🔰*\n\n" +
		"*CHECK IF A WORD OR SENTENCE READS THE SAME BACKWARDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PALINDROME <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PALINDROME MADAM ❯*"
}

func handlePalindrome(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, palindromeGuide(prefix))
			return
		}
		clean := strings.ToLower(text)
		clean = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(clean, "")
		r := []rune(clean)
		isPal := true
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			if r[i] != r[j] {
				isPal = false
				break
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 PALINDROME CHECKER 🔰*\n\n")
		b.WriteString("*📝 TEXT ❯ " + text + "*\n")
		if isPal {
			b.WriteString("*✅ YES, IT IS A PALINDROME*")
		} else {
			b.WriteString("*❌ NO, IT IS NOT A PALINDROME*")
		}
		s.Reply(info, b.String())
	})
}

// ── .ANAGRAM ─────────────────────────────────────────────────────────────────

func anagramGuide(prefix string) string {
	return "*🔰 ANAGRAM CHECKER 🔰*\n\n" +
		"*CHECK IF TWO WORDS ARE ANAGRAMS OF EACH OTHER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ANAGRAM <WORD1> <WORD2> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ANAGRAM LISTEN SILENT ❯*"
}

func handleAnagram(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, anagramGuide(prefix))
			return
		}
		w1 := strings.ToLower(strings.TrimSpace(args[0]))
		w2 := strings.ToLower(strings.TrimSpace(args[1]))
		isAna := sortString(w1) == sortString(w2)
		var b strings.Builder
		b.WriteString("*🔰 ANAGRAM CHECKER 🔰*\n\n")
		b.WriteString("*🔤 WORD 1 ❯ " + strings.ToUpper(w1) + "*\n")
		b.WriteString("*🔤 WORD 2 ❯ " + strings.ToUpper(w2) + "*\n")
		if isAna {
			b.WriteString("*✅ YES, THEY ARE ANAGRAMS*")
		} else {
			b.WriteString("*❌ NO, THEY ARE NOT ANAGRAMS*")
		}
		s.Reply(info, b.String())
	})
}

// sortString returns the characters of s sorted alphabetically.
func sortString(s string) string {
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		for j := i + 1; j < len(r); j++ {
			if r[j] < r[i] {
				r[i], r[j] = r[j], r[i]
			}
		}
	}
	return string(r)
}

// ── .LOREMIPSUM ──────────────────────────────────────────────────────────────

func loremipsumGuide(prefix string) string {
	return "*🔰 LOREM IPSUM 🔰*\n\n" +
		"*GENERATE PLACEHOLDER TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LOREMIPSUM ❯*  (1 paragraph)\n" +
		"*❮ " + prefix + "LOREMIPSUM <COUNT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LOREMIPSUM 3 ❯*"
}

var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "eu", "fugiat", "nulla", "pariatur", "excepteur",
	"sint", "occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui",
	"officia", "deserunt", "mollit", "anim", "id", "est", "laborum",
}

func handleLoremipsum(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		count := 1
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n > 0 {
				count = n
			}
		}
		if count > 10 {
			count = 10
		}
		var b strings.Builder
		b.WriteString("*🔰 LOREM IPSUM 🔰*\n\n")
		for p := 0; p < count; p++ {
			var words []string
			for i := 0; i < 40; i++ {
				words = append(words, loremWords[(p*40+i)%len(loremWords)])
			}
			sentence := strings.Join(words, " ")
			sentence = strings.ToUpper(sentence[:1]) + sentence[1:] + "."
			b.WriteString("*" + sentence + "*\n\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .URLPARSE ────────────────────────────────────────────────────────────────

func urlparseGuide(prefix string) string {
	return "*🔰 URL PARSER 🔰*\n\n" +
		"*BREAK ANY URL INTO ITS PARTS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "URLPARSE <URL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "URLPARSE HTTPS://GOOGLE.COM/SEARCH?Q=TEST ❯*"
}

func handleUrlparse(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(strings.Join(args, " "))
		if raw == "" {
			s.Reply(info, urlparseGuide(prefix))
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			s.Reply(info, "*🔰 INVALID URL*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 URL PARSER 🔰*\n\n")
		b.WriteString("*🔗 SCHEME ❯ " + strings.ToUpper(u.Scheme) + "*\n")
		b.WriteString("*🌐 HOST ❯ " + u.Host + "*\n")
		if u.Path != "" {
			b.WriteString("*📁 PATH ❯ " + u.Path + "*\n")
		}
		if u.RawQuery != "" {
			b.WriteString("*❓ QUERY ❯ " + u.RawQuery + "*\n")
		}
		if u.Fragment != "" {
			b.WriteString("*#️⃣ FRAGMENT ❯ " + u.Fragment + "*\n")
		}
		if u.Port() != "" {
			b.WriteString("*🔌 PORT ❯ " + u.Port() + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .EMAILVALIDATE ───────────────────────────────────────────────────────────

func emailvalidateGuide(prefix string) string {
	return "*🔰 EMAIL VALIDATOR 🔰*\n\n" +
		"*CHECK IF AN EMAIL ADDRESS IS VALID*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "EMAILVALIDATE <EMAIL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "EMAILVALIDATE TEST@GMAIL.COM ❯*"
}

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func handleEmailvalidate(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		email := strings.TrimSpace(strings.Join(args, " "))
		if email == "" {
			s.Reply(info, emailvalidateGuide(prefix))
			return
		}
		valid := emailRe.MatchString(email)
		var b strings.Builder
		b.WriteString("*🔰 EMAIL VALIDATOR 🔰*\n\n")
		b.WriteString("*📧 EMAIL ❯ " + email + "*\n")
		if valid {
			parts := strings.SplitN(email, "@", 2)
			b.WriteString("*✅ STATUS ❯ VALID* \n")
			b.WriteString("*👤 USER ❯ " + parts[0] + "*\n")
			b.WriteString("*🌐 DOMAIN ❯ " + parts[1] + "*")
		} else {
			b.WriteString("*❌ STATUS ❯ INVALID EMAIL FORMAT*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

func init() {
	Register(Command{Name: "timestamp", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT A UNIX TIMESTAMP TO DATE OR SHOW CURRENT TIME. USE IT AS .TIMESTAMP <UNIX>.", Run: handleTimestamp})
	Register(Command{Name: "agecalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR EXACT AGE FROM YOUR BIRTHDATE. USE IT AS .AGECALC <YYYY-MM-DD>.", Run: handleAgecalc})
	Register(Command{Name: "unitconvert", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT BETWEEN UNITS LIKE KM, MI, KG, LB, C, F. USE IT AS .UNITCONVERT <VALUE> <FROM> <TO>.", Run: handleUnitconvert})
	Register(Command{Name: "caseconvert", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHANGE THE CASE OF ANY TEXT. USE IT AS .CASECONVERT <MODE> <TEXT>.", Run: handleCaseconvert})
	Register(Command{Name: "charcount", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT CHARACTERS, WORDS AND LINES IN ANY TEXT. USE IT AS .CHARCOUNT <TEXT>.", Run: handleCharcount})
	Register(Command{Name: "palindrome", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK IF A WORD OR SENTENCE IS A PALINDROME. USE IT AS .PALINDROME <TEXT>.", Run: handlePalindrome})
	Register(Command{Name: "anagram", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK IF TWO WORDS ARE ANAGRAMS. USE IT AS .ANAGRAM <WORD1> <WORD2>.", Run: handleAnagram})
	Register(Command{Name: "loremipsum", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE PLACEHOLDER LOREM IPSUM TEXT. USE IT AS .LOREMIPSUM <COUNT>.", Run: handleLoremipsum})
	Register(Command{Name: "urlparse", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO BREAK ANY URL INTO ITS PARTS. USE IT AS .URLPARSE <URL>.", Run: handleUrlparse})
	Register(Command{Name: "emailvalidate", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK IF AN EMAIL ADDRESS IS VALID. USE IT AS .EMAILVALIDATE <EMAIL>.", Run: handleEmailvalidate})

	// hidden aliases
	Register(Command{Name: "unixtime", Category: "TOOLS", Desc: "Short alias of .timestamp", Hidden: true, Run: handleTimestamp})
	Register(Command{Name: "myage", Category: "TOOLS", Desc: "Short alias of .agecalc", Hidden: true, Run: handleAgecalc})
	Register(Command{Name: "convertunit", Category: "TOOLS", Desc: "Short alias of .unitconvert", Hidden: true, Run: handleUnitconvert})
	Register(Command{Name: "textcase", Category: "TOOLS", Desc: "Short alias of .caseconvert", Hidden: true, Run: handleCaseconvert})
	Register(Command{Name: "wordcount", Category: "TOOLS", Desc: "Short alias of .charcount", Hidden: true, Run: handleCharcount})
	Register(Command{Name: "ispalindrome", Category: "TOOLS", Desc: "Short alias of .palindrome", Hidden: true, Run: handlePalindrome})
	Register(Command{Name: "isanagram", Category: "TOOLS", Desc: "Short alias of .anagram", Hidden: true, Run: handleAnagram})
	Register(Command{Name: "lorem", Category: "TOOLS", Desc: "Short alias of .loremipsum", Hidden: true, Run: handleLoremipsum})
	Register(Command{Name: "urlinfo", Category: "TOOLS", Desc: "Short alias of .urlparse", Hidden: true, Run: handleUrlparse})
	Register(Command{Name: "checkemail", Category: "TOOLS", Desc: "Short alias of .emailvalidate", Hidden: true, Run: handleEmailvalidate})
}
