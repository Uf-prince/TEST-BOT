package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 16 (10 new everyday commands)
// File: toolpack16.go
// ============================================================================
//   .textstats <text>    -> detailed text statistics
//   .wordfreq <text>     -> word frequency counter
//   .leapyear <year>     -> check if a year is a leap year
//   .domaininfo <domain> -> domain registration info (RDAP)
//   .phonevalidate <num> -> validate a phone number format
//   .colorinfo <hex>     -> color name, RGB and HSL from a hex code
//   .numberfact <num>    -> interesting facts about a number
//   .wordmeaning <word>  -> dictionary meaning of a word
//   .spellcheck <word>   -> spelling suggestions for a word
//   .mathcalc <expr>     -> evaluate a math expression
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .TEXTSTATS ───────────────────────────────────────────────────────────────

func textstatsGuide(prefix string) string {
	return "*🔰 TEXT STATISTICS 🔰*\n\n" +
		"*GET DETAILED STATISTICS OF ANY TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEXTSTATS <TEXT> ❯*"
}

func handleTextstats(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			s.Reply(info, textstatsGuide(prefix))
			return
		}
		chars := len([]rune(text))
		noSpace := len([]rune(strings.ReplaceAll(text, " ", "")))
		words := len(strings.Fields(text))
		lines := len(strings.Split(text, "\n"))
		sentences := len(regexp.MustCompile(`[.!?]+`).FindAllString(text, -1))
		if sentences == 0 {
			sentences = 1
		}
		letters := 0
		digits := 0
		for _, r := range text {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				letters++
			} else if r >= '0' && r <= '9' {
				digits++
			}
		}
		avg := 0.0
		if words > 0 {
			avg = float64(chars) / float64(words)
		}
		var b strings.Builder
		b.WriteString("*🔰 TEXT STATISTICS 🔰*\n\n")
		b.WriteString("*🔤 CHARACTERS ❯ " + strconv.Itoa(chars) + "*\n")
		b.WriteString("*🚫 WITHOUT SPACES ❯ " + strconv.Itoa(noSpace) + "*\n")
		b.WriteString("*📝 WORDS ❯ " + strconv.Itoa(words) + "*\n")
		b.WriteString("*📄 LINES ❯ " + strconv.Itoa(lines) + "*\n")
		b.WriteString("*❓ SENTENCES ❯ " + strconv.Itoa(sentences) + "*\n")
		b.WriteString("*🔡 LETTERS ❯ " + strconv.Itoa(letters) + "*\n")
		b.WriteString("*🔢 DIGITS ❯ " + strconv.Itoa(digits) + "*\n")
		b.WriteString("*📊 AVG WORD LENGTH ❯ " + strconv.FormatFloat(avg, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

// ── .WORDFREQ ────────────────────────────────────────────────────────────────

func wordfreqGuide(prefix string) string {
	return "*🔰 WORD FREQUENCY 🔰*\n\n" +
		"*COUNT HOW MANY TIMES EACH WORD APPEARS IN TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORDFREQ <TEXT> ❯*"
}

func handleWordfreq(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.ToLower(strings.Join(args, " "))
		if strings.TrimSpace(text) == "" {
			s.Reply(info, wordfreqGuide(prefix))
			return
		}
		counts := map[string]int{}
		for _, w := range strings.Fields(text) {
			w = strings.Trim(w, ".,!?;:\"'()[]{}")
			if w != "" {
				counts[w]++
			}
		}
		type kv struct {
			k string
			v int
		}
		var list []kv
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
		b.WriteString("*🔰 WORD FREQUENCY 🔰*\n\n")
		b.WriteString("*📊 TOTAL UNIQUE ❯ " + strconv.Itoa(len(list)) + "*\n\n")
		limit := len(list)
		if limit > 15 {
			limit = 15
		}
		for i := 0; i < limit; i++ {
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + strings.ToUpper(list[i].k) + " ❯ " + strconv.Itoa(list[i].v) + "x*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .LEAPYEAR ────────────────────────────────────────────────────────────────

func leapyearGuide(prefix string) string {
	return "*🔰 LEAP YEAR CHECKER 🔰*\n\n" +
		"*CHECK IF A YEAR IS A LEAP YEAR*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LEAPYEAR <YEAR> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LEAPYEAR 2024 ❯*"
}

func handleLeapyear(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, leapyearGuide(prefix))
			return
		}
		y, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID YEAR, PLEASE GIVE A NUMBER*")
			return
		}
		leap := (y%4 == 0 && y%100 != 0) || y%400 == 0
		days := 365
		if leap {
			days = 366
		}
		status := "❌ NOT A LEAP YEAR"
		if leap {
			status = "✅ YES, IT IS A LEAP YEAR"
		}
		s.Reply(info, "*🔰 LEAP YEAR CHECKER 🔰*\n\n*📅 YEAR ❯ "+strconv.Itoa(y)+"*\n*"+status+"*\n*🗓️ DAYS ❯ "+strconv.Itoa(days)+"*")
	})
}

// ── .DOMAININFO ──────────────────────────────────────────────────────────────

func domaininfoGuide(prefix string) string {
	return "*🔰 DOMAIN INFO 🔰*\n\n" +
		"*GET REGISTRATION INFO OF ANY DOMAIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DOMAININFO <DOMAIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DOMAININFO GOOGLE.COM ❯*"
}

func handleDomaininfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		domain := strings.TrimSpace(args[0])
		if domain == "" {
			s.Reply(info, domaininfoGuide(prefix))
			return
		}
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimSuffix(domain, "/")
		waitID := s.ReplyWithID(info, "*LOOKING UP DOMAIN....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			LDHName string   `json:"ldhName"`
			Status  []string `json:"status"`
			Events  []struct {
				EventAction string `json:"eventAction"`
				EventDate   string `json:"eventDate"`
			} `json:"events"`
			Nameservers []struct {
				LDHName string `json:"ldhName"`
			} `json:"nameservers"`
		}
		if err := funGetJSON(ctx, "https://rdap.org/domain/"+url.QueryEscape(domain), &res); err != nil || res.LDHName == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DOMAIN INFO")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DOMAIN INFO 🔰*\n\n")
		b.WriteString("*🌐 DOMAIN ❯ " + strings.ToUpper(res.LDHName) + "*\n")
		for _, e := range res.Events {
			b.WriteString("*📅 " + strings.ToUpper(e.EventAction) + " ❯ " + strings.ToUpper(strings.Split(e.EventDate, "T")[0]) + "*\n")
		}
		if len(res.Nameservers) > 0 {
			var ns []string
			for _, n := range res.Nameservers {
				ns = append(ns, strings.ToUpper(n.LDHName))
			}
			b.WriteString("*🖥️ NAMESERVERS ❯ " + strings.Join(ns, ", ") + "*\n")
		}
		if len(res.Status) > 0 {
			b.WriteString("*📌 STATUS ❯ " + strings.ToUpper(strings.Join(res.Status, ", ")) + "*")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .PHONEVALIDATE ───────────────────────────────────────────────────────────

func phonevalidateGuide(prefix string) string {
	return "*🔰 PHONE VALIDATOR 🔰*\n\n" +
		"*VALIDATE A PHONE NUMBER AND DETECT ITS COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PHONEVALIDATE <NUMBER WITH COUNTRY CODE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PHONEVALIDATE +923158930864 ❯*"
}

func handlePhonevalidate(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(strings.Join(args, ""))
		if raw == "" {
			s.Reply(info, phonevalidateGuide(prefix))
			return
		}
		digits := regexp.MustCompile(`[^0-9]`).ReplaceAllString(raw, "")
		if len(digits) < 7 || len(digits) > 15 {
			s.Reply(info, "*🔰 INVALID PHONE NUMBER (MUST BE 7-15 DIGITS)*")
			return
		}
		country := "UNKNOWN"
		cc := ""
		codes := []struct {
			code, name string
		}{
			{"92", "PAKISTAN"}, {"91", "INDIA"}, {"1", "USA/CANADA"}, {"44", "UNITED KINGDOM"},
			{"971", "UAE"}, {"966", "SAUDI ARABIA"}, {"880", "BANGLADESH"}, {"62", "INDONESIA"},
			{"90", "TURKEY"}, {"20", "EGYPT"}, {"86", "CHINA"}, {"81", "JAPAN"},
			{"49", "GERMANY"}, {"33", "FRANCE"}, {"7", "RUSSIA"}, {"234", "NIGERIA"},
		}
		for _, c := range codes {
			if strings.HasPrefix(digits, c.code) {
				country = c.name
				cc = "+" + c.code
				break
			}
		}
		s.Reply(info, "*🔰 PHONE VALIDATOR 🔰*\n\n*📞 NUMBER ❯ "+raw+"*\n*✅ STATUS ❯ VALID FORMAT*\n*🌍 COUNTRY ❯ "+country+"*\n*🔢 COUNTRY CODE ❯ "+cc+"*\n*🔢 DIGITS ❯ "+strconv.Itoa(len(digits))+"*")
	})
}

// ── .COLORINFO ───────────────────────────────────────────────────────────────

func colorinfoGuide(prefix string) string {
	return "*🔰 COLOR INFO 🔰*\n\n" +
		"*GET THE NAME, RGB AND HSL OF ANY HEX COLOR*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COLORINFO <HEX> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COLORINFO FF0000 ❯*"
}

func handleColorinfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		hex := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
		if hex == "" {
			s.Reply(info, colorinfoGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*ANALYZING COLOR....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Hex struct {
				Value string `json:"value"`
			} `json:"hex"`
			RGB struct {
				Value string `json:"value"`
			} `json:"rgb"`
			HSL struct {
				Value string `json:"value"`
			} `json:"hsl"`
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
		}
		if err := funGetJSON(ctx, "https://www.thecolorapi.com/id?hex="+url.QueryEscape(hex), &res); err != nil || res.Hex.Value == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COLOR INFO")
			}
			return
		}
		s.Reply(info, "*🔰 COLOR INFO 🔰*\n\n*🎨 NAME ❯ "+strings.ToUpper(res.Name.Value)+"*\n*🔷 HEX ❯ "+strings.ToUpper(res.Hex.Value)+"*\n*🟥 RGB ❯ "+strings.ToUpper(res.RGB.Value)+"*\n*🌈 HSL ❯ "+strings.ToUpper(res.HSL.Value)+"*")
	})
}

// ── .NUMBERFACT ──────────────────────────────────────────────────────────────

func numberfactGuide(prefix string) string {
	return "*🔰 NUMBER FACTS 🔰*\n\n" +
		"*GET INTERESTING FACTS ABOUT ANY NUMBER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NUMBERFACT <NUMBER> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NUMBERFACT 42 ❯*"
}

func isPrimeNum(n int) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 {
		return false
	}
	for i := 3; i*i <= n; i += 2 {
		if n%i == 0 {
			return false
		}
	}
	return true
}

func handleNumberfact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, numberfactGuide(prefix))
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A WHOLE NUMBER*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NUMBER FACTS 🔰*\n\n")
		b.WriteString("*🔢 NUMBER ❯ " + strconv.Itoa(n) + "*\n")
		if n%2 == 0 {
			b.WriteString("*⚖️ PARITY ❯ EVEN*\n")
		} else {
			b.WriteString("*⚖️ PARITY ❯ ODD*\n")
		}
		if isPrimeNum(n) {
			b.WriteString("*🧮 PRIME ❯ YES*\n")
		} else {
			b.WriteString("*🧮 PRIME ❯ NO*\n")
		}
		if n >= 0 {
			root := int(math.Sqrt(float64(n)))
			if root*root == n {
				b.WriteString("*✅ PERFECT SQUARE ❯ YES (" + strconv.Itoa(root) + "²)*\n")
			} else {
				b.WriteString("*✅ PERFECT SQUARE ❯ NO*\n")
			}
		}
		b.WriteString("*✖️ SQUARE ❯ " + strconv.Itoa(n*n) + "*\n")
		b.WriteString("*➕ DIGIT SUM ❯ " + strconv.Itoa(digitSum(n)) + "*\n")
		b.WriteString("*🔁 REVERSED ❯ " + strconv.Itoa(reverseInt(n)) + "*")
		s.Reply(info, b.String())
	})
}

func digitSum(n int) int {
	if n < 0 {
		n = -n
	}
	sum := 0
	for n > 0 {
		sum += n % 10
		n /= 10
	}
	return sum
}

func reverseInt(n int) int {
	neg := n < 0
	if neg {
		n = -n
	}
	rev := 0
	for n > 0 {
		rev = rev*10 + n%10
		n /= 10
	}
	if neg {
		return -rev
	}
	return rev
}

// ── .WORDMEANING ─────────────────────────────────────────────────────────────

func wordmeaningGuide(prefix string) string {
	return "*🔰 WORD MEANING 🔰*\n\n" +
		"*GET THE DICTIONARY MEANING OF ANY ENGLISH WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORDMEANING <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WORDMEANING HELLO ❯*"
}

func handleWordmeaning(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(args[0])
		if word == "" {
			s.Reply(info, wordmeaningGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP WORD....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res map[string][]struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string `json:"definition"`
			} `json:"definitions"`
		}
		if err := funGetJSON(ctx, "https://en.wiktionary.org/api/rest_v1/page/definition/"+url.QueryEscape(word), &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "WORD MEANING")
			}
			return
		}
		entries, ok := res["en"]
		if !ok || len(entries) == 0 || len(entries[0].Definitions) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "WORD MEANING")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 WORD MEANING 🔰*\n\n")
		b.WriteString("*📖 WORD ❯ " + strings.ToUpper(word) + "*\n")
		b.WriteString("*🔤 TYPE ❯ " + strings.ToUpper(entries[0].PartOfSpeech) + "*\n\n")
		limit := len(entries[0].Definitions)
		if limit > 3 {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			def := stripHTMLTags(entries[0].Definitions[i].Definition)
			def = strings.TrimSpace(def)
			if len(def) > 300 {
				def = def[:300] + "..."
			}
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + def + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .SPELLCHECK ──────────────────────────────────────────────────────────────

func spellcheckGuide(prefix string) string {
	return "*🔰 SPELL CHECKER 🔰*\n\n" +
		"*GET SPELLING SUGGESTIONS FOR A MISSPELLED WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SPELLCHECK <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SPELLCHECK RECIEVE ❯*"
}

func handleSpellcheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(args[0])
		if word == "" {
			s.Reply(info, spellcheckGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CHECKING SPELLING....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word  string `json:"word"`
			Score int    `json:"score"`
		}
		if err := funGetJSON(ctx, "https://api.datamuse.com/sug?max=8&s="+url.QueryEscape(word), &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SPELL CHECKER")
			}
			return
		}
		var words []string
		for _, w := range res {
			words = append(words, strings.ToUpper(w.Word))
		}
		s.Reply(info, "*🔰 SPELL CHECKER 🔰*\n\n*📝 INPUT ❯ "+strings.ToUpper(word)+"*\n\n*💡 SUGGESTIONS:*\n*"+strings.Join(words, ", ")+"*")
	})
}

// ── .MATHCALC ────────────────────────────────────────────────────────────────

func mathcalcGuide(prefix string) string {
	return "*🔰 MATH CALCULATOR 🔰*\n\n" +
		"*EVALUATE ANY MATH EXPRESSION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MATHCALC <EXPRESSION> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MATHCALC 2+2*3 ❯*"
}

func handleMathcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		expr := strings.TrimSpace(strings.Join(args, " "))
		if expr == "" {
			s.Reply(info, mathcalcGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CALCULATING....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		body, err := funGetBytes(ctx, "http://api.mathjs.org/v4/?expr="+url.QueryEscape(expr))
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "MATH CALCULATOR")
			}
			return
		}
		result := strings.TrimSpace(string(body))
		if result == "" || strings.Contains(strings.ToLower(result), "error") {
			s.Reply(info, "*🔰 INVALID EXPRESSION, PLEASE CHECK YOUR MATH*")
			return
		}
		s.Reply(info, "*🔰 MATH CALCULATOR 🔰*\n\n*📝 EXPRESSION ❯ "+expr+"*\n*✅ RESULT ❯ "+result+"*")
	})
}

func init() {
	Register(Command{Name: "textstats", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DETAILED STATISTICS OF ANY TEXT. USE IT AS .TEXTSTATS <TEXT>.", Run: handleTextstats})
	Register(Command{Name: "wordfreq", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT HOW MANY TIMES EACH WORD APPEARS. USE IT AS .WORDFREQ <TEXT>.", Run: handleWordfreq})
	Register(Command{Name: "leapyear", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK IF A YEAR IS A LEAP YEAR. USE IT AS .LEAPYEAR <YEAR>.", Run: handleLeapyear})
	Register(Command{Name: "domaininfo", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET REGISTRATION INFO OF ANY DOMAIN. USE IT AS .DOMAININFO <DOMAIN>.", Run: handleDomaininfo})
	Register(Command{Name: "phonevalidate", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO VALIDATE A PHONE NUMBER AND DETECT ITS COUNTRY. USE IT AS .PHONEVALIDATE <NUMBER>.", Run: handlePhonevalidate})
	Register(Command{Name: "colorinfo", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE NAME, RGB AND HSL OF ANY HEX COLOR. USE IT AS .COLORINFO <HEX>.", Run: handleColorinfo})
	Register(Command{Name: "numberfact", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INTERESTING FACTS ABOUT ANY NUMBER. USE IT AS .NUMBERFACT <NUMBER>.", Run: handleNumberfact})
	Register(Command{Name: "wordmeaning", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE DICTIONARY MEANING OF ANY WORD. USE IT AS .WORDMEANING <WORD>.", Run: handleWordmeaning})
	Register(Command{Name: "spellcheck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET SPELLING SUGGESTIONS FOR A WORD. USE IT AS .SPELLCHECK <WORD>.", Run: handleSpellcheck})
	Register(Command{Name: "mathcalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO EVALUATE ANY MATH EXPRESSION. USE IT AS .MATHCALC <EXPRESSION>.", Run: handleMathcalc})

	// hidden aliases
	Register(Command{Name: "textinfo", Category: "TOOLS", Desc: "Short alias of .textstats", Hidden: true, Run: handleTextstats})
	Register(Command{Name: "wordcount2", Category: "TOOLS", Desc: "Short alias of .wordfreq", Hidden: true, Run: handleWordfreq})
	Register(Command{Name: "isleap", Category: "TOOLS", Desc: "Short alias of .leapyear", Hidden: true, Run: handleLeapyear})
	Register(Command{Name: "whoisdomain", Category: "TOOLS", Desc: "Short alias of .domaininfo", Hidden: true, Run: handleDomaininfo})
	Register(Command{Name: "checkphone", Category: "TOOLS", Desc: "Short alias of .phonevalidate", Hidden: true, Run: handlePhonevalidate})
	Register(Command{Name: "colorcode", Category: "TOOLS", Desc: "Short alias of .colorinfo", Hidden: true, Run: handleColorinfo})
	Register(Command{Name: "numfact", Category: "TOOLS", Desc: "Short alias of .numberfact", Hidden: true, Run: handleNumberfact})
	Register(Command{Name: "define", Category: "TOOLS", Desc: "Short alias of .wordmeaning", Hidden: true, Run: handleWordmeaning})
	Register(Command{Name: "spell", Category: "TOOLS", Desc: "Short alias of .spellcheck", Hidden: true, Run: handleSpellcheck})
	Register(Command{Name: "calculate", Category: "TOOLS", Desc: "Short alias of .mathcalc", Hidden: true, Run: handleMathcalc})
}
