package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 7 (15 everyday converters, generators & live data)
// File: toolpack7.go
// ============================================================================
//   .binary <text>            -> text to binary (use -d to decode)
//   .hex <text>               -> text to hex (use -d to decode)
//   .reverse <text>           -> reverse any text
//   .wordcount <text>         -> count words, chars, lines
//   .slug <text>              -> make a clean URL slug
//   .urlencode <text>         -> URL encode (use -d to decode)
//   .dice [sides] [count]     -> roll dice
//   .random <min> <max>       -> random number in range
//   .prime <number>           -> check if a number is prime
//   .fibonacci <n>            -> first n fibonacci numbers
//   .gcd <a> <b>              -> GCD and LCM of two numbers
//   .cve [count]              -> latest published CVEs
//   .airquality <lat> <lng>   -> live air quality index
//   .moon <lat> <lng>         -> moon phase + moonrise/moonset
//   .countryinfo <country>    -> capital + ISO codes of a country
// ============================================================================

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// ── .BINARY ──────────────────────────────────────────────────────────────────

func binaryGuide(prefix string) string {
	return "*🔰 BINARY CONVERTER 🔰*\n\n" +
		"*TEXT TO BINARY OR BINARY TO TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BINARY <TEXT> ❯*  (ENCODE)\n" +
		"*❮ " + prefix + "BINARY -D <BITS> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "BINARY HI ❯*"
}

func handleBinary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, binaryGuide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, binaryGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	if decode {
		var out strings.Builder
		for _, grp := range strings.Fields(text) {
			v, err := strconv.ParseInt(grp, 2, 32)
			if err != nil {
				s.Reply(info, "*🔰 INVALID BINARY*")
				return
			}
			out.WriteRune(rune(v))
		}
		s.Reply(info, "*🔰 BINARY DECODED 🔰*\n\n*"+out.String()+"*")
		return
	}
	var out strings.Builder
	for _, r := range text {
		out.WriteString(fmt.Sprintf("%08b ", r))
	}
	s.Reply(info, "*🔰 BINARY ENCODED 🔰*\n\n*"+strings.TrimSpace(out.String())+"*")
}

// ── .HEX ─────────────────────────────────────────────────────────────────────

func hexGuide(prefix string) string {
	return "*🔰 HEX CONVERTER 🔰*\n\n" +
		"*TEXT TO HEX OR HEX TO TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HEX <TEXT> ❯*  (ENCODE)\n" +
		"*❮ " + prefix + "HEX -D <HEX> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "HEX HI ❯*"
}

func handleHex(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, hexGuide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, hexGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	if decode {
		clean := strings.ReplaceAll(text, " ", "")
		if len(clean)%2 != 0 {
			s.Reply(info, "*🔰 INVALID HEX*")
			return
		}
		var out strings.Builder
		for i := 0; i < len(clean); i += 2 {
			v, err := strconv.ParseInt(clean[i:i+2], 16, 32)
			if err != nil {
				s.Reply(info, "*🔰 INVALID HEX*")
				return
			}
			out.WriteRune(rune(v))
		}
		s.Reply(info, "*🔰 HEX DECODED 🔰*\n\n*"+out.String()+"*")
		return
	}
	var out strings.Builder
	for _, r := range text {
		out.WriteString(fmt.Sprintf("%02x ", r))
	}
	s.Reply(info, "*🔰 HEX ENCODED 🔰*\n\n*"+strings.TrimSpace(out.String())+"*")
}

// ── .REVERSE ─────────────────────────────────────────────────────────────────

func reverseGuide(prefix string) string {
	return "*🔰 TEXT REVERSER 🔰*\n\n" +
		"*REVERSE ANY TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "REVERSE <TEXT> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "REVERSE HELLO ❯*"
}

func handleReverse(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, reverseGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	r := []rune(text)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	s.Reply(info, "*🔰 REVERSED 🔰*\n\n*"+string(r)+"*")
}

// ── .WORDCOUNT ───────────────────────────────────────────────────────────────

func wordCountGuide(prefix string) string {
	return "*🔰 WORD COUNTER 🔰*\n\n" +
		"*COUNT WORDS, CHARS AND LINES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORDCOUNT <TEXT> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "WORDCOUNT HELLO WORLD ❯*"
}

func handleWordCount(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, wordCountGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	words := len(strings.Fields(text))
	chars := len([]rune(text))
	charsNoSpace := len([]rune(strings.ReplaceAll(text, " ", "")))
	lines := len(strings.Split(text, "\n"))
	var b strings.Builder
	b.WriteString("*🔰 WORD COUNT 🔰*\n\n")
	b.WriteString("*📝 WORDS ❯ " + strconv.Itoa(words) + "*\n")
	b.WriteString("*🔤 CHARACTERS ❯ " + strconv.Itoa(chars) + "*\n")
	b.WriteString("*🔡 WITHOUT SPACES ❯ " + strconv.Itoa(charsNoSpace) + "*\n")
	b.WriteString("*📄 LINES ❯ " + strconv.Itoa(lines) + "*")
	s.Reply(info, b.String())
}

// ── .URLENCODE ───────────────────────────────────────────────────────────────

func urlEncodeGuide(prefix string) string {
	return "*🔰 URL ENCODER 🔰*\n\n" +
		"*ENCODE OR DECODE A URL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "URLENCODE <TEXT> ❯*  (ENCODE)\n" +
		"*❮ " + prefix + "URLENCODE -D <URL> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "URLENCODE hello world ❯*"
}

func handleURLEncode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, urlEncodeGuide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, urlEncodeGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	if decode {
		out, err := url.QueryUnescape(text)
		if err != nil {
			s.Reply(info, "*🔰 INVALID URL ENCODING*")
			return
		}
		s.Reply(info, "*🔰 URL DECODED 🔰*\n\n*"+out+"*")
		return
	}
	s.Reply(info, "*🔰 URL ENCODED 🔰*\n\n*"+url.QueryEscape(text)+"*")
}

// ── .DICE ────────────────────────────────────────────────────────────────────

func diceGuide(prefix string) string {
	return "*🔰 DICE ROLLER 🔰*\n\n" +
		"*ROLL ONE OR MORE DICE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DICE [SIDES] [COUNT] ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "DICE 6 3 ❯*"
}

func handleDice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	sides := 6
	count := 1
	if len(args) >= 1 {
		if v, err := strconv.Atoi(args[0]); err == nil && v >= 2 {
			sides = v
		}
	}
	if len(args) >= 2 {
		if v, err := strconv.Atoi(args[1]); err == nil && v >= 1 {
			count = v
		}
	}
	if count > 10 {
		count = 10
	}
	var b strings.Builder
	b.WriteString("*🔰 DICE ROLLED 🔰*\n\n")
	total := 0
	for i := 0; i < count; i++ {
		r := randInt(sides) + 1
		total += r
		b.WriteString("*🎲 " + strconv.Itoa(r) + "*\n")
	}
	if count > 1 {
		b.WriteString("\n*➕ TOTAL ❯ " + strconv.Itoa(total) + "*")
	}
	s.Reply(info, strings.TrimSpace(b.String()))
}

// ── .RANDOM ──────────────────────────────────────────────────────────────────

func randomGuide(prefix string) string {
	return "*🔰 RANDOM NUMBER 🔰*\n\n" +
		"*GET A RANDOM NUMBER IN A RANGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOM <MIN> <MAX> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "RANDOM 1 100 ❯*"
}

func handleRandom(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, randomGuide(prefix))
		return
	}
	lo, e1 := strconv.Atoi(args[0])
	hi, e2 := strconv.Atoi(args[1])
	if e1 != nil || e2 != nil {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+randomGuide(prefix))
		return
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	n := lo + randInt(hi-lo+1)
	s.Reply(info, "*🔰 RANDOM NUMBER 🔰*\n\n*🎯 "+strconv.Itoa(n)+"*")
}

// ── .PRIME ───────────────────────────────────────────────────────────────────

func primeGuide(prefix string) string {
	return "*🔰 PRIME CHECKER 🔰*\n\n" +
		"*CHECK IF A NUMBER IS PRIME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PRIME <NUMBER> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "PRIME 97 ❯*"
}

func handlePrime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, primeGuide(prefix))
		return
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 0 {
		s.Reply(info, "*🔰 INVALID NUMBER*\n\n"+primeGuide(prefix))
		return
	}
	if isPrime(n) {
		s.Reply(info, "*🔰 PRIME CHECK 🔰*\n\n*✅ "+strconv.Itoa(n)+" IS A PRIME NUMBER*")
	} else {
		s.Reply(info, "*🔰 PRIME CHECK 🔰*\n\n*❌ "+strconv.Itoa(n)+" IS NOT A PRIME NUMBER*")
	}
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	for i := 5; i*i <= n; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}

// ── .FIBONACCI ───────────────────────────────────────────────────────────────

func fibonacciGuide(prefix string) string {
	return "*🔰 FIBONACCI 🔰*\n\n" +
		"*FIRST N FIBONACCI NUMBERS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "FIBONACCI <N> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "FIBONACCI 10 ❯*"
}

func handleFibonacci(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, fibonacciGuide(prefix))
		return
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 {
		s.Reply(info, "*🔰 INVALID NUMBER*\n\n"+fibonacciGuide(prefix))
		return
	}
	if n > 50 {
		n = 50
	}
	seq := make([]string, 0, n)
	a, b := 0, 1
	for i := 0; i < n; i++ {
		seq = append(seq, strconv.Itoa(a))
		a, b = b, a+b
	}
	s.Reply(info, "*🔰 FIBONACCI ("+strconv.Itoa(n)+") 🔰*\n\n*"+strings.Join(seq, ", ")+"*")
}

// ── .GCD ─────────────────────────────────────────────────────────────────────

func gcdGuide(prefix string) string {
	return "*🔰 GCD / LCM 🔰*\n\n" +
		"*FIND GCD AND LCM OF TWO NUMBERS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GCD <A> <B> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "GCD 24 36 ❯*"
}

func handleGCD(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, gcdGuide(prefix))
		return
	}
	a, e1 := strconv.Atoi(args[0])
	b, e2 := strconv.Atoi(args[1])
	if e1 != nil || e2 != nil || a <= 0 || b <= 0 {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+gcdGuide(prefix))
		return
	}
	g := gcdInt(a, b)
	l := a / g * b
	var sb strings.Builder
	sb.WriteString("*🔰 GCD / LCM 🔰*\n\n")
	sb.WriteString("*🔢 NUMBERS ❯ " + strconv.Itoa(a) + ", " + strconv.Itoa(b) + "*\n")
	sb.WriteString("*➗ GCD ❯ " + strconv.Itoa(g) + "*\n")
	sb.WriteString("*✖️ LCM ❯ " + strconv.Itoa(l) + "*")
	s.Reply(info, sb.String())
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// ── .CVE ─────────────────────────────────────────────────────────────────────

func cveGuide(prefix string) string {
	return "*🔰 LATEST CVES 🔰*\n\n" +
		"*LATEST PUBLISHED SECURITY VULNERABILITIES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CVE [COUNT] ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "CVE 5 ❯*"
}

func handleCVE(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	n := 5
	if len(args) >= 1 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	if n > 10 {
		n = 10
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING LATEST CVES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		// NVD CVE API 2.0 — official, reliable, no key required.
		// Query the last 30 days of published CVEs, newest first.
		end := time.Now().UTC()
		start := end.AddDate(0, 0, -30)
		u := "https://services.nvd.nist.gov/rest/json/cves/2.0?pubStartDate=" +
			url.QueryEscape(start.Format("2006-01-02T15:04:05.000")) +
			"&pubEndDate=" + url.QueryEscape(end.Format("2006-01-02T15:04:05.000")) +
			"&resultsPerPage=" + strconv.Itoa(n)

		var res struct {
			Vulnerabilities []struct {
				CVE struct {
					ID           string `json:"id"`
					Published    string `json:"published"`
					Descriptions []struct {
						Lang  string `json:"lang"`
						Value string `json:"value"`
					} `json:"descriptions"`
				} `json:"cve"`
			} `json:"vulnerabilities"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Vulnerabilities) == 0 {
			// Fallback: CIRCL mirror (older but still live).
			var alt []struct {
				ID        string   `json:"id"`
				Published string   `json:"published"`
				Aliases   []string `json:"aliases"`
				Details   string   `json:"details"`
			}
			if err2 := funGetJSON(ctx, "https://cve.circl.lu/api/last", &alt); err2 != nil || len(alt) == 0 {
				if !ctxTimedOut(ctx) {
					s.Reply(info, "*🔰 COULD NOT FETCH CVES*")
				}
				return
			}
			var b strings.Builder
			b.WriteString("*🔰 LATEST CVES 🔰*\n\n")
			for i, c := range alt {
				if i >= n {
					break
				}
				id := c.ID
				for _, a := range c.Aliases {
					if strings.HasPrefix(a, "CVE-") {
						id = a
						break
					}
				}
				date := c.Published
				if len(date) >= 10 {
					date = date[:10]
				}
				desc := c.Details
				if len(desc) > 160 {
					desc = desc[:160] + "..."
				}
				b.WriteString("*" + strconv.Itoa(i+1) + ". " + id + "*\n")
				b.WriteString("• 📅 " + date + "\n")
				b.WriteString("• " + desc + "\n\n")
			}
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 LATEST CVES 🔰*\n\n")
		for i, v := range res.Vulnerabilities {
			if i >= n {
				break
			}
			c := v.CVE
			date := c.Published
			if len(date) >= 10 {
				date = date[:10]
			}
			desc := ""
			for _, d := range c.Descriptions {
				if d.Lang == "en" {
					desc = d.Value
					break
				}
			}
			if desc == "" && len(c.Descriptions) > 0 {
				desc = c.Descriptions[0].Value
			}
			if len(desc) > 160 {
				desc = desc[:160] + "..."
			}
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + c.ID + "*\n")
			b.WriteString("• 📅 " + date + "\n")
			b.WriteString("• " + desc + "\n\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .AIRQUALITY ──────────────────────────────────────────────────────────────

func airQualityGuide(prefix string) string {
	return "*🔰 AIR QUALITY 🔰*\n\n" +
		"*LIVE AIR QUALITY INDEX BY LOCATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AIRQUALITY <LAT> <LNG> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "AIRQUALITY 33.68 73.04 ❯*"
}

func handleAirQuality(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, airQualityGuide(prefix))
		return
	}
	lat, e1 := strconv.ParseFloat(args[0], 64)
	lng, e2 := strconv.ParseFloat(args[1], 64)
	if e1 != nil || e2 != nil {
		s.Reply(info, "*🔰 INVALID COORDINATES*\n\n"+airQualityGuide(prefix))
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*CHECKING AIR QUALITY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Current struct {
				PM10        float64 `json:"pm10"`
				PM25        float64 `json:"pm2_5"`
				EuropeanAQI int     `json:"european_aqi"`
				USAQI       int     `json:"us_aqi"`
			} `json:"current"`
		}
		u := "https://air-quality-api.open-meteo.com/v1/air-quality?latitude=" +
			strconv.FormatFloat(lat, 'f', 4, 64) + "&longitude=" + strconv.FormatFloat(lng, 'f', 4, 64) +
			"&current=pm10,pm2_5,european_aqi,us_aqi"
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH AIR QUALITY*")
			}
			return
		}
		level := aqiLevel(res.Current.EuropeanAQI)
		var b strings.Builder
		b.WriteString("*🔰 AIR QUALITY 🔰*\n\n")
		b.WriteString("*📍 LOCATION ❯ " + strconv.FormatFloat(lat, 'f', 2, 64) + ", " + strconv.FormatFloat(lng, 'f', 2, 64) + "*\n\n")
		b.WriteString("*🌫️ PM10 ❯ " + strconv.FormatFloat(res.Current.PM10, 'f', 1, 64) + " µg/m³*\n")
		b.WriteString("*🌫️ PM2.5 ❯ " + strconv.FormatFloat(res.Current.PM25, 'f', 1, 64) + " µg/m³*\n")
		b.WriteString("*🇪🇺 EUROPEAN AQI ❯ " + strconv.Itoa(res.Current.EuropeanAQI) + "*\n")
		b.WriteString("*🇺🇸 US AQI ❯ " + strconv.Itoa(res.Current.USAQI) + "*\n")
		b.WriteString("*🏷️ LEVEL ❯ " + level + "*")
		s.Reply(info, b.String())
	})
}

func aqiLevel(aqi int) string {
	switch {
	case aqi <= 20:
		return "GOOD ✅"
	case aqi <= 40:
		return "FAIR 🟢"
	case aqi <= 60:
		return "MODERATE 🟡"
	case aqi <= 80:
		return "POOR 🟠"
	case aqi <= 100:
		return "VERY POOR 🔴"
	default:
		return "EXTREMELY POOR ☠️"
	}
}

// ── .MOON ────────────────────────────────────────────────────────────────────

func moonGuide(prefix string) string {
	return "*🔰 MOON PHASE 🔰*\n\n" +
		"*MOON PHASE + MOONRISE / MOONSET*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MOON <LAT> <LNG> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "MOON 33.68 73.04 ❯*"
}

func handleMoon(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, moonGuide(prefix))
		return
	}
	lat, e1 := strconv.ParseFloat(args[0], 64)
	lng, e2 := strconv.ParseFloat(args[1], 64)
	if e1 != nil || e2 != nil {
		s.Reply(info, "*🔰 INVALID COORDINATES*\n\n"+moonGuide(prefix))
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*CHECKING MOON PHASE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Results struct {
				Date             string  `json:"date"`
				Moonrise         string  `json:"moonrise"`
				Moonset          string  `json:"moonset"`
				MoonPhase        string  `json:"moon_phase"`
				MoonIllumination float64 `json:"moon_illumination"`
			} `json:"results"`
		}
		u := "https://api.sunrisesunset.io/json?lat=" + strconv.FormatFloat(lat, 'f', 4, 64) +
			"&lng=" + strconv.FormatFloat(lng, 'f', 4, 64)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Results.MoonPhase == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH MOON DATA*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 MOON PHASE 🔰*\n\n")
		b.WriteString("*📅 DATE ❯ " + res.Results.Date + "*\n")
		b.WriteString("*🌙 PHASE ❯ " + res.Results.MoonPhase + "*\n")
		b.WriteString("*💡 ILLUMINATION ❯ " + strconv.FormatFloat(res.Results.MoonIllumination, 'f', 1, 64) + "%*\n")
		b.WriteString("*🌄 MOONRISE ❯ " + res.Results.Moonrise + "*\n")
		b.WriteString("*🌇 MOONSET ❯ " + res.Results.Moonset + "*")
		s.Reply(info, b.String())
	})
}

// ── .COUNTRYINFO ─────────────────────────────────────────────────────────────

func countryInfoGuide(prefix string) string {
	return "*🔰 COUNTRY INFO 🔰*\n\n" +
		"*CAPITAL + ISO CODES OF ANY COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTRYINFO <COUNTRY> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "COUNTRYINFO Pakistan ❯*"
}

func handleCountryInfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, countryInfoGuide(prefix))
		return
	}
	q := strings.ToLower(strings.Join(args, " "))
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*LOOKING UP COUNTRY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Error bool `json:"error"`
			Data  []struct {
				Name    string `json:"name"`
				Capital string `json:"capital"`
				ISO2    string `json:"iso2"`
				ISO3    string `json:"iso3"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://countriesnow.space/api/v0.1/countries/capital", &res); err != nil || len(res.Data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH COUNTRY DATA*")
			}
			return
		}
		var found *struct {
			Name    string `json:"name"`
			Capital string `json:"capital"`
			ISO2    string `json:"iso2"`
			ISO3    string `json:"iso3"`
		}
		for i := range res.Data {
			if strings.ToLower(res.Data[i].Name) == q {
				found = &res.Data[i]
				break
			}
		}
		if found == nil {
			for i := range res.Data {
				if strings.Contains(strings.ToLower(res.Data[i].Name), q) {
					found = &res.Data[i]
					break
				}
			}
		}
		if found == nil {
			s.Reply(info, "*🔰 COUNTRY NOT FOUND*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 COUNTRY INFO 🔰*\n\n")
		b.WriteString("*🌍 COUNTRY ❯ " + found.Name + "*\n")
		b.WriteString("*🏛️ CAPITAL ❯ " + found.Capital + "*\n")
		b.WriteString("*🔤 ISO2 ❯ " + found.ISO2 + "*\n")
		b.WriteString("*🔤 ISO3 ❯ " + found.ISO3 + "*")
		s.Reply(info, b.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "binary", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO BINARY. USE IT AS .BINARY <TEXT> OR .BINARY -D <BITS>.", Run: handleBinary})
	Register(Command{Name: "hex", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO HEX. USE IT AS .HEX <TEXT> OR .HEX -D <HEX>.", Run: handleHex})
	Register(Command{Name: "reverse", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO REVERSE ANY TEXT. USE IT AS .REVERSE <TEXT>.", Run: handleReverse})
	Register(Command{Name: "wordcount", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT WORDS AND CHARACTERS. USE IT AS .WORDCOUNT <TEXT>.", Run: handleWordCount})
	Register(Command{Name: "urlencode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ENCODE OR DECODE A URL. USE IT AS .URLENCODE <TEXT> OR .URLENCODE -D <URL>.", Run: handleURLEncode})
	Register(Command{Name: "dice", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ROLL DICE. USE IT AS .DICE [SIDES] [COUNT].", Run: handleDice})
	Register(Command{Name: "random", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM NUMBER IN A RANGE. USE IT AS .RANDOM <MIN> <MAX>.", Run: handleRandom})
	Register(Command{Name: "prime", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK IF A NUMBER IS PRIME. USE IT AS .PRIME <NUMBER>.", Run: handlePrime})
	Register(Command{Name: "fibonacci", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE FIRST N FIBONACCI NUMBERS. USE IT AS .FIBONACCI <N>.", Run: handleFibonacci})
	Register(Command{Name: "gcd", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND GCD AND LCM OF TWO NUMBERS. USE IT AS .GCD <A> <B>.", Run: handleGCD})
	Register(Command{Name: "cve", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LATEST SECURITY VULNERABILITIES. USE IT AS .CVE [COUNT].", Run: handleCVE})
	Register(Command{Name: "airquality", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET LIVE AIR QUALITY BY LOCATION. USE IT AS .AIRQUALITY <LAT> <LNG>.", Run: handleAirQuality})
	Register(Command{Name: "moon", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE MOON PHASE AND MOONRISE. USE IT AS .MOON <LAT> <LNG>.", Run: handleMoon})
	Register(Command{Name: "countryinfo", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET CAPITAL AND ISO CODES OF A COUNTRY. USE IT AS .COUNTRYINFO <COUNTRY>.", Run: handleCountryInfo})

	// hidden short aliases
	Register(Command{Name: "aqi", Category: "TOOLS", Desc: "Short alias of .airquality", Hidden: true, Run: handleAirQuality})
	Register(Command{Name: "moonphase", Category: "TOOLS", Desc: "Short alias of .moon", Hidden: true, Run: handleMoon})
	Register(Command{Name: "rev", Category: "TOOLS", Desc: "Short alias of .reverse", Hidden: true, Run: handleReverse})
}

var _ = fmt.Sprintf
