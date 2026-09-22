package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 6 (15 everyday calculators & converters)
// File: toolpack6.go
// ============================================================================
//   .bmi <kg> <cm>            -> body mass index
//   .loan <amt> <rate> <mo>   -> monthly EMI + total interest
//   .tip <bill> <pct>         -> tip amount + grand total
//   .percentage <x> <y>       -> x% of y and x is what % of y
//   .unit <val> <from> <to>   -> length / weight unit converter
//   .temperature <v> <f> <t>  -> C / F / K temperature converter
//   .zodiac <DD/MM>           -> your zodiac sign
//   .countdown <YYYY-MM-DD>   -> days left until a date
//   .daysbetween <d1> <d2>    -> days between two dates
//   .uuid [count]             -> random UUID v4
//   .password [len]           -> strong random password
//   .base64 <text>            -> base64 encode (use -d to decode)
//   .morse <text>             -> text to morse (use -d to decode)
//   .roman <number>           -> number to roman (use -d to decode)
//   .calc <expression>        -> evaluate a math expression
// ============================================================================

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .BMI ─────────────────────────────────────────────────────────────────────

func bmiGuide(prefix string) string {
	return "*🔰 BMI CALCULATOR 🔰*\n\n" +
		"*FIND YOUR BODY MASS INDEX*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BMI <WEIGHT_KG> <HEIGHT_CM> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "BMI 70 175 ❯*"
}

func handleBMI(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, bmiGuide(prefix))
		return
	}
	w, err1 := strconv.ParseFloat(args[0], 64)
	h, err2 := strconv.ParseFloat(args[1], 64)
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+bmiGuide(prefix))
		return
	}
	hm := h / 100.0
	bmi := w / (hm * hm)
	cat := "NORMAL WEIGHT"
	switch {
	case bmi < 18.5:
		cat = "UNDERWEIGHT"
	case bmi < 25:
		cat = "NORMAL WEIGHT"
	case bmi < 30:
		cat = "OVERWEIGHT"
	default:
		cat = "OBESE"
	}
	var b strings.Builder
	b.WriteString("*🔰 BMI RESULT 🔰*\n\n")
	b.WriteString("*⚖️ WEIGHT ❯ " + strconv.FormatFloat(w, 'f', 1, 64) + " KG*\n")
	b.WriteString("*📏 HEIGHT ❯ " + strconv.FormatFloat(h, 'f', 1, 64) + " CM*\n")
	b.WriteString("*📊 BMI ❯ " + strconv.FormatFloat(bmi, 'f', 1, 64) + "*\n")
	b.WriteString("*🏷️ CATEGORY ❯ " + cat + "*")
	s.Reply(info, b.String())
}

// ── .LOAN ────────────────────────────────────────────────────────────────────

func loanGuide(prefix string) string {
	return "*🔰 LOAN / EMI CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR MONTHLY EMI*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LOAN <AMOUNT> <ANNUAL_RATE%> <MONTHS> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "LOAN 500000 12 60 ❯*"
}

func handleLoan(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 3 {
		s.Reply(info, loanGuide(prefix))
		return
	}
	p, e1 := strconv.ParseFloat(args[0], 64)
	r, e2 := strconv.ParseFloat(args[1], 64)
	n, e3 := strconv.ParseFloat(args[2], 64)
	if e1 != nil || e2 != nil || e3 != nil || p <= 0 || n <= 0 {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+loanGuide(prefix))
		return
	}
	mr := r / 12.0 / 100.0
	var emi float64
	if mr == 0 {
		emi = p / n
	} else {
		emi = p * mr * math.Pow(1+mr, n) / (math.Pow(1+mr, n) - 1)
	}
	total := emi * n
	interest := total - p
	var b strings.Builder
	b.WriteString("*🔰 LOAN / EMI RESULT 🔰*\n\n")
	b.WriteString("*💰 PRINCIPAL ❯ " + strconv.FormatFloat(p, 'f', 2, 64) + "*\n")
	b.WriteString("*📈 RATE ❯ " + strconv.FormatFloat(r, 'f', 2, 64) + "% / YEAR*\n")
	b.WriteString("*🗓️ TENURE ❯ " + strconv.FormatFloat(n, 'f', 0, 64) + " MONTHS*\n\n")
	b.WriteString("*💵 MONTHLY EMI ❯ " + strconv.FormatFloat(emi, 'f', 2, 64) + "*\n")
	b.WriteString("*🧾 TOTAL PAYABLE ❯ " + strconv.FormatFloat(total, 'f', 2, 64) + "*\n")
	b.WriteString("*🔥 TOTAL INTEREST ❯ " + strconv.FormatFloat(interest, 'f', 2, 64) + "*")
	s.Reply(info, b.String())
}

// ── .TIP ─────────────────────────────────────────────────────────────────────

func tipGuide(prefix string) string {
	return "*🔰 TIP CALCULATOR 🔰*\n\n" +
		"*SPLIT A BILL WITH TIP*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIP <BILL> <TIP%> [PEOPLE] ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "TIP 1000 10 4 ❯*"
}

func handleTip(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, tipGuide(prefix))
		return
	}
	bill, e1 := strconv.ParseFloat(args[0], 64)
	pct, e2 := strconv.ParseFloat(args[1], 64)
	if e1 != nil || e2 != nil || bill <= 0 {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+tipGuide(prefix))
		return
	}
	people := 1.0
	if len(args) >= 3 {
		if p, err := strconv.ParseFloat(args[2], 64); err == nil && p > 0 {
			people = p
		}
	}
	tip := bill * pct / 100.0
	total := bill + tip
	per := total / people
	var b strings.Builder
	b.WriteString("*🔰 TIP RESULT 🔰*\n\n")
	b.WriteString("*🧾 BILL ❯ " + strconv.FormatFloat(bill, 'f', 2, 64) + "*\n")
	b.WriteString("*💁 TIP (" + strconv.FormatFloat(pct, 'f', 1, 64) + "%) ❯ " + strconv.FormatFloat(tip, 'f', 2, 64) + "*\n")
	b.WriteString("*💰 TOTAL ❯ " + strconv.FormatFloat(total, 'f', 2, 64) + "*\n")
	if people > 1 {
		b.WriteString("*👥 PER PERSON (" + strconv.FormatFloat(people, 'f', 0, 64) + ") ❯ " + strconv.FormatFloat(per, 'f', 2, 64) + "*")
	}
	s.Reply(info, b.String())
}

// ── .PERCENTAGE ──────────────────────────────────────────────────────────────

func percentageGuide(prefix string) string {
	return "*🔰 PERCENTAGE CALCULATOR 🔰*\n\n" +
		"*QUICK PERCENTAGE MATH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PERCENTAGE <X> <Y> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "PERCENTAGE 25 200 ❯*"
}

func handlePercentage(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, percentageGuide(prefix))
		return
	}
	x, e1 := strconv.ParseFloat(args[0], 64)
	y, e2 := strconv.ParseFloat(args[1], 64)
	if e1 != nil || e2 != nil {
		s.Reply(info, "*🔰 INVALID NUMBERS*\n\n"+percentageGuide(prefix))
		return
	}
	of := x / 100.0 * y
	var pctOf float64
	if y != 0 {
		pctOf = x / y * 100.0
	}
	var b strings.Builder
	b.WriteString("*🔰 PERCENTAGE RESULT 🔰*\n\n")
	b.WriteString("*" + strconv.FormatFloat(x, 'f', 2, 64) + "% OF " + strconv.FormatFloat(y, 'f', 2, 64) + " ❯ " + strconv.FormatFloat(of, 'f', 2, 64) + "*\n")
	b.WriteString("*" + strconv.FormatFloat(x, 'f', 2, 64) + " IS " + strconv.FormatFloat(pctOf, 'f', 2, 64) + "% OF " + strconv.FormatFloat(y, 'f', 2, 64) + "*")
	s.Reply(info, b.String())
}

// ── .UNIT ────────────────────────────────────────────────────────────────────

var unitFactors = map[string]float64{
	// length (base: meter)
	"mm": 0.001, "cm": 0.01, "m": 1, "km": 1000,
	"in": 0.0254, "ft": 0.3048, "yd": 0.9144, "mi": 1609.344,
	// weight (base: gram)
	"mg": 0.001, "g": 1, "kg": 1000, "lb": 453.59237, "oz": 28.349523125,
}

func unitGuide(prefix string) string {
	return "*🔰 UNIT CONVERTER 🔰*\n\n" +
		"*CONVERT LENGTH / WEIGHT UNITS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UNIT <VALUE> <FROM> <TO> ❯*\n\n" +
		"*LENGTH ❯ MM CM M KM IN FT YD MI*\n" +
		"*WEIGHT ❯ MG G KG LB OZ*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "UNIT 10 KM MI ❯*"
}

func handleUnit(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 3 {
		s.Reply(info, unitGuide(prefix))
		return
	}
	v, err := strconv.ParseFloat(args[0], 64)
	from := strings.ToLower(args[1])
	to := strings.ToLower(args[2])
	f1, ok1 := unitFactors[from]
	f2, ok2 := unitFactors[to]
	if err != nil || !ok1 || !ok2 {
		s.Reply(info, "*🔰 UNKNOWN UNIT*\n\n"+unitGuide(prefix))
		return
	}
	res := v * f1 / f2
	var b strings.Builder
	b.WriteString("*🔰 UNIT CONVERTED 🔰*\n\n")
	b.WriteString("*" + strconv.FormatFloat(v, 'f', -1, 64) + " " + strings.ToUpper(from) + " ❯ " + strconv.FormatFloat(res, 'f', 4, 64) + " " + strings.ToUpper(to) + "*")
	s.Reply(info, b.String())
}

// ── .TEMPERATURE ─────────────────────────────────────────────────────────────

func tempGuide(prefix string) string {
	return "*🔰 TEMPERATURE CONVERTER 🔰*\n\n" +
		"*CONVERT C / F / K*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TEMPERATURE <VALUE> <FROM> <TO> ❯*\n\n" +
		"*UNITS ❯ C (CELSIUS) F (FAHRENHEIT) K (KELVIN)*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "TEMPERATURE 100 C F ❯*"
}

func handleTemperature(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 3 {
		s.Reply(info, tempGuide(prefix))
		return
	}
	v, err := strconv.ParseFloat(args[0], 64)
	from := strings.ToLower(args[1])
	to := strings.ToLower(args[2])
	if err != nil {
		s.Reply(info, "*🔰 INVALID NUMBER*\n\n"+tempGuide(prefix))
		return
	}
	// to celsius
	var c float64
	switch from {
	case "c":
		c = v
	case "f":
		c = (v - 32) * 5 / 9
	case "k":
		c = v - 273.15
	default:
		s.Reply(info, "*🔰 UNKNOWN UNIT*\n\n"+tempGuide(prefix))
		return
	}
	var res float64
	switch to {
	case "c":
		res = c
	case "f":
		res = c*9/5 + 32
	case "k":
		res = c + 273.15
	default:
		s.Reply(info, "*🔰 UNKNOWN UNIT*\n\n"+tempGuide(prefix))
		return
	}
	var b strings.Builder
	b.WriteString("*🔰 TEMPERATURE CONVERTED 🔰*\n\n")
	b.WriteString("*" + strconv.FormatFloat(v, 'f', 2, 64) + "°" + strings.ToUpper(from) + " ❯ " + strconv.FormatFloat(res, 'f', 2, 64) + "°" + strings.ToUpper(to) + "*")
	s.Reply(info, b.String())
}

// ── .ZODIAC ──────────────────────────────────────────────────────────────────

func zodiacGuide(prefix string) string {
	return "*🔰 ZODIAC FINDER 🔰*\n\n" +
		"*FIND YOUR ZODIAC SIGN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ZODIAC <DD/MM> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "ZODIAC 15/08 ❯*"
}

func handleZodiac(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, zodiacGuide(prefix))
		return
	}
	raw := strings.ReplaceAll(args[0], "-", "/")
	parts := strings.Split(raw, "/")
	if len(parts) < 2 {
		s.Reply(info, "*🔰 INVALID DATE*\n\n"+zodiacGuide(prefix))
		return
	}
	d, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || d < 1 || d > 31 || m < 1 || m > 12 {
		s.Reply(info, "*🔰 INVALID DATE*\n\n"+zodiacGuide(prefix))
		return
	}
	sign, emoji := zodiacSign(d, m)
	var b strings.Builder
	b.WriteString("*🔰 ZODIAC RESULT 🔰*\n\n")
	b.WriteString("*📅 DATE ❯ " + strconv.Itoa(d) + "/" + strconv.Itoa(m) + "*\n")
	b.WriteString("*" + emoji + " SIGN ❯ " + sign + "*")
	s.Reply(info, b.String())
}

func zodiacSign(d, m int) (string, string) {
	type z struct {
		name  string
		emoji string
		from  int // month*100+day
	}
	signs := []z{
		{"CAPRICORN", "♑", 1222}, {"AQUARIUS", "♒", 120}, {"PISCES", "♓", 219},
		{"ARIES", "♈", 321}, {"TAURUS", "♉", 420}, {"GEMINI", "♊", 521},
		{"CANCER", "♋", 621}, {"LEO", "♌", 723}, {"VIRGO", "♍", 823},
		{"LIBRA", "♎", 923}, {"SCORPIO", "♏", 1023}, {"SAGITTARIUS", "♐", 1122},
		{"CAPRICORN", "♑", 1222},
	}
	key := m*100 + d
	for i := 0; i < len(signs)-1; i++ {
		if key >= signs[i].from && key < signs[i+1].from {
			return signs[i].name, signs[i].emoji
		}
	}
	return "CAPRICORN", "♑"
}

// ── .COUNTDOWN ───────────────────────────────────────────────────────────────

func countdownGuide(prefix string) string {
	return "*🔰 COUNTDOWN 🔰*\n\n" +
		"*DAYS LEFT UNTIL A DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTDOWN <YYYY-MM-DD> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "COUNTDOWN 2026-01-01 ❯*"
}

func handleCountdown(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, countdownGuide(prefix))
		return
	}
	t, err := time.Parse("2006-01-02", args[0])
	if err != nil {
		s.Reply(info, "*🔰 INVALID DATE (USE YYYY-MM-DD)*\n\n"+countdownGuide(prefix))
		return
	}
	now := time.Now()
	days := int(t.Sub(now).Hours() / 24)
	var b strings.Builder
	b.WriteString("*🔰 COUNTDOWN 🔰*\n\n")
	b.WriteString("*📅 TARGET ❯ " + t.Format("02 Jan 2006") + "*\n")
	if days > 0 {
		b.WriteString("*⏳ " + strconv.Itoa(days) + " DAYS LEFT*")
	} else if days == 0 {
		b.WriteString("*🎉 IT IS TODAY!*")
	} else {
		b.WriteString("*✅ " + strconv.Itoa(-days) + " DAYS AGO*")
	}
	s.Reply(info, b.String())
}

// ── .DAYSBETWEEN ─────────────────────────────────────────────────────────────

func daysBetweenGuide(prefix string) string {
	return "*🔰 DAYS BETWEEN 🔰*\n\n" +
		"*DAYS BETWEEN TWO DATES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DAYSBETWEEN <YYYY-MM-DD> <YYYY-MM-DD> ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "DAYSBETWEEN 2025-01-01 2025-12-31 ❯*"
}

func handleDaysBetween(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 2 {
		s.Reply(info, daysBetweenGuide(prefix))
		return
	}
	t1, e1 := time.Parse("2006-01-02", args[0])
	t2, e2 := time.Parse("2006-01-02", args[1])
	if e1 != nil || e2 != nil {
		s.Reply(info, "*🔰 INVALID DATE (USE YYYY-MM-DD)*\n\n"+daysBetweenGuide(prefix))
		return
	}
	days := int(t2.Sub(t1).Hours() / 24)
	if days < 0 {
		days = -days
	}
	var b strings.Builder
	b.WriteString("*🔰 DAYS BETWEEN 🔰*\n\n")
	b.WriteString("*📅 FROM ❯ " + t1.Format("02 Jan 2006") + "*\n")
	b.WriteString("*📅 TO ❯ " + t2.Format("02 Jan 2006") + "*\n")
	b.WriteString("*🗓️ TOTAL ❯ " + strconv.Itoa(days) + " DAYS (" + strconv.Itoa(days/7) + " WEEKS)*")
	s.Reply(info, b.String())
}

// ── .UUID ────────────────────────────────────────────────────────────────────

func uuidGuide(prefix string) string {
	return "*🔰 UUID GENERATOR 🔰*\n\n" +
		"*GENERATE RANDOM UUID V4*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UUID [COUNT] ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "UUID 3 ❯*"
}

func handleUUID(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	n := 1
	if len(args) >= 1 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	if n > 10 {
		n = 10
	}
	var b strings.Builder
	b.WriteString("*🔰 UUID GENERATOR 🔰*\n\n")
	for i := 0; i < n; i++ {
		b.WriteString("*" + newUUID() + "*\n")
	}
	s.Reply(info, strings.TrimSpace(b.String()))
}

func newUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ── .PASSWORD ────────────────────────────────────────────────────────────────

func passwordGuide(prefix string) string {
	return "*🔰 PASSWORD GENERATOR 🔰*\n\n" +
		"*CREATE A STRONG RANDOM PASSWORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PASSWORD [LENGTH] ❯*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "PASSWORD 16 ❯*"
}

func handlePassword(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	n := 16
	if len(args) >= 1 {
		if v, err := strconv.Atoi(args[0]); err == nil && v >= 4 {
			n = v
		}
	}
	if n > 64 {
		n = 64
	}
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}
	s.Reply(info, "*🔰 PASSWORD GENERATOR 🔰*\n\n*🔑 "+b.String()+"*\n\n*⚠️ NEVER SHARE THIS WITH ANYONE*")
}

// ── .BASE64 ──────────────────────────────────────────────────────────────────

func base64Guide(prefix string) string {
	return "*🔰 BASE64 TOOL 🔰*\n\n" +
		"*ENCODE OR DECODE BASE64*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BASE64 <TEXT> ❯*  (ENCODE)\n" +
		"*❮ " + prefix + "BASE64 -D <CODE> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "BASE64 HELLO ❯*"
}

func handleBase64(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, base64Guide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, base64Guide(prefix))
		return
	}
	text := strings.Join(args, " ")
	if decode {
		out, err := base64Decode(text)
		if err != nil {
			s.Reply(info, "*🔰 INVALID BASE64*")
			return
		}
		s.Reply(info, "*🔰 BASE64 DECODED 🔰*\n\n*"+out+"*")
		return
	}
	s.Reply(info, "*🔰 BASE64 ENCODED 🔰*\n\n*"+base64Encode(text)+"*")
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func base64Decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ── .MORSE ───────────────────────────────────────────────────────────────────

var morseMap = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.",
	'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..",
	'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.",
	'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-",
	'Y': "-.--", 'Z': "--..", '0': "-----", '1': ".----", '2': "..---",
	'3': "...--", '4': "....-", '5': ".....", '6': "-....", '7': "--...",
	'8': "---..", '9': "----.",
}

func morseGuide(prefix string) string {
	return "*🔰 MORSE CODE 🔰*\n\n" +
		"*TEXT TO MORSE OR MORSE TO TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MORSE <TEXT> ❯*  (ENCODE)\n" +
		"*❮ " + prefix + "MORSE -D <CODE> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "MORSE SOS ❯*"
}

func handleMorse(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, morseGuide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, morseGuide(prefix))
		return
	}
	text := strings.Join(args, " ")
	if decode {
		rev := map[string]rune{}
		for k, v := range morseMap {
			rev[v] = k
		}
		var out strings.Builder
		for _, code := range strings.Fields(text) {
			if r, ok := rev[code]; ok {
				out.WriteRune(r)
			} else if code == "/" {
				out.WriteByte(' ')
			}
		}
		s.Reply(info, "*🔰 MORSE DECODED 🔰*\n\n*"+out.String()+"*")
		return
	}
	var out strings.Builder
	for _, r := range strings.ToUpper(text) {
		if r == ' ' {
			out.WriteString("/ ")
			continue
		}
		if m, ok := morseMap[r]; ok {
			out.WriteString(m + " ")
		}
	}
	s.Reply(info, "*🔰 MORSE ENCODED 🔰*\n\n*"+strings.TrimSpace(out.String())+"*")
}

// ── .ROMAN ───────────────────────────────────────────────────────────────────

func romanGuide(prefix string) string {
	return "*🔰 ROMAN NUMERALS 🔰*\n\n" +
		"*NUMBER TO ROMAN OR ROMAN TO NUMBER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ROMAN <NUMBER> ❯*  (1-3999)\n" +
		"*❮ " + prefix + "ROMAN -D <ROMAN> ❯*  (DECODE)\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "ROMAN 2025 ❯*"
}

func handleRoman(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, romanGuide(prefix))
		return
	}
	decode := false
	if args[0] == "-d" || args[0] == "-D" {
		decode = true
		args = args[1:]
	}
	if len(args) < 1 {
		s.Reply(info, romanGuide(prefix))
		return
	}
	if decode {
		n, ok := romanToInt(strings.ToUpper(args[0]))
		if !ok {
			s.Reply(info, "*🔰 INVALID ROMAN NUMERAL*")
			return
		}
		s.Reply(info, "*🔰 ROMAN DECODED 🔰*\n\n*"+strings.ToUpper(args[0])+" ❯ "+strconv.Itoa(n)+"*")
		return
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 || n > 3999 {
		s.Reply(info, "*🔰 ENTER A NUMBER BETWEEN 1 AND 3999*")
		return
	}
	s.Reply(info, "*🔰 ROMAN NUMERAL 🔰*\n\n*"+strconv.Itoa(n)+" ❯ "+intToRoman(n)+"*")
}

func intToRoman(n int) string {
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

func romanToInt(s string) (int, bool) {
	vals := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	for i := 0; i < len(s); i++ {
		v, ok := vals[s[i]]
		if !ok {
			return 0, false
		}
		if i+1 < len(s) && vals[s[i+1]] > v {
			total -= v
		} else {
			total += v
		}
	}
	return total, true
}

// ── .CALC ────────────────────────────────────────────────────────────────────

func calcGuide(prefix string) string {
	return "*🔰 CALCULATOR 🔰*\n\n" +
		"*EVALUATE A MATH EXPRESSION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CALC <EXPRESSION> ❯*\n\n" +
		"*SUPPORTS ❯ + - * / % ^ ( )*\n\n" +
		"*EXAMPLE:*\n" +
		"*❮ " + prefix + "CALC (2+3)*4^2 ❯*"
}

func handleCalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) < 1 {
		s.Reply(info, calcGuide(prefix))
		return
	}
	expr := strings.Join(args, "")
	val, err := evalExpr(expr)
	if err != nil {
		s.Reply(info, "*🔰 INVALID EXPRESSION*\n\n"+calcGuide(prefix))
		return
	}
	s.Reply(info, "*🔰 CALCULATOR 🔰*\n\n*"+expr+" = "+strconv.FormatFloat(val, 'f', -1, 64)+"*")
}

type exprParser struct {
	s   string
	pos int
}

func evalExpr(s string) (float64, error) {
	p := &exprParser{s: strings.ReplaceAll(s, " ", "")}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.s) {
		return 0, fmt.Errorf("unexpected char")
	}
	return v, nil
}

func (p *exprParser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *exprParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		c := p.peek()
		if c == '+' || c == '-' {
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			if c == '+' {
				left += right
			} else {
				left -= right
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		c := p.peek()
		if c == '*' || c == '/' || c == '%' {
			p.pos++
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			switch c {
			case '*':
				left *= right
			case '/':
				if right == 0 {
					return 0, fmt.Errorf("div by zero")
				}
				left /= right
			case '%':
				if right == 0 {
					return 0, fmt.Errorf("mod by zero")
				}
				left = math.Mod(left, right)
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parsePower() (float64, error) {
	base, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	if p.peek() == '^' {
		p.pos++
		exp, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return base, nil
}

func (p *exprParser) parseUnary() (float64, error) {
	if p.peek() == '-' {
		p.pos++
		v, err := p.parseUnary()
		return -v, err
	}
	if p.peek() == '+' {
		p.pos++
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (float64, error) {
	c := p.peek()
	if c == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.pos++
		return v, nil
	}
	start := p.pos
	for p.pos < len(p.s) && (p.s[p.pos] >= '0' && p.s[p.pos] <= '9' || p.s[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("expected number")
	}
	return strconv.ParseFloat(p.s[start:p.pos], 64)
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "bmi", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR BODY MASS INDEX. USE IT AS .BMI <KG> <CM>.", Run: handleBMI})
	Register(Command{Name: "loan", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE LOAN EMI AND INTEREST. USE IT AS .LOAN <AMOUNT> <RATE> <MONTHS>.", Run: handleLoan})
	Register(Command{Name: "tip", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE TIP AND SPLIT A BILL. USE IT AS .TIP <BILL> <PCT> [PEOPLE].", Run: handleTip})
	Register(Command{Name: "percentage", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO DO QUICK PERCENTAGE MATH. USE IT AS .PERCENTAGE <X> <Y>.", Run: handlePercentage})
	Register(Command{Name: "unit", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT LENGTH AND WEIGHT UNITS. USE IT AS .UNIT <VALUE> <FROM> <TO>.", Run: handleUnit})
	Register(Command{Name: "temperature", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT CELSIUS, FAHRENHEIT AND KELVIN. USE IT AS .TEMPERATURE <V> <FROM> <TO>.", Run: handleTemperature})
	Register(Command{Name: "zodiac", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND YOUR ZODIAC SIGN FROM YOUR BIRTH DATE. USE IT AS .ZODIAC <DD/MM>.", Run: handleZodiac})
	Register(Command{Name: "countdown", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT DAYS LEFT UNTIL A DATE. USE IT AS .COUNTDOWN <YYYY-MM-DD>.", Run: handleCountdown})
	Register(Command{Name: "daysbetween", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND DAYS BETWEEN TWO DATES. USE IT AS .DAYSBETWEEN <DATE1> <DATE2>.", Run: handleDaysBetween})
	Register(Command{Name: "uuid", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE RANDOM UUID V4. USE IT AS .UUID [COUNT].", Run: handleUUID})
	Register(Command{Name: "password", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A STRONG RANDOM PASSWORD. USE IT AS .PASSWORD [LENGTH].", Run: handlePassword})
	Register(Command{Name: "base64", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ENCODE OR DECODE BASE64. USE IT AS .BASE64 <TEXT> OR .BASE64 -D <CODE>.", Run: handleBase64})
	Register(Command{Name: "morse", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO MORSE CODE. USE IT AS .MORSE <TEXT> OR .MORSE -D <CODE>.", Run: handleMorse})
	Register(Command{Name: "roman", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT NUMBERS TO ROMAN NUMERALS. USE IT AS .ROMAN <NUMBER> OR .ROMAN -D <ROMAN>.", Run: handleRoman})
	Register(Command{Name: "calc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO EVALUATE A MATH EXPRESSION. USE IT AS .CALC <EXPRESSION>.", Run: handleCalc})

	// hidden short aliases
	Register(Command{Name: "emi", Category: "TOOLS", Desc: "Short alias of .loan", Hidden: true, Run: handleLoan})
	Register(Command{Name: "b64", Category: "TOOLS", Desc: "Short alias of .base64", Hidden: true, Run: handleBase64})
	Register(Command{Name: "calc2", Category: "TOOLS", Desc: "Short alias of .calc", Hidden: true, Run: handleCalc})
}

var _ = fmt.Sprintf
