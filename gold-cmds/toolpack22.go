package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 22 (10 new astrology & personality commands)
// File: toolpack22.go
// ============================================================================
//   .chinesezodiac <year>      -> Chinese zodiac animal
//   .birthstone <month>        -> birthstone for a month
//   .birthflower <month>       -> birth flower for a month
//   .numerology <name>         -> numerology number of a name
//   .lifepath <YYYY-MM-DD>     -> life path number
//   .horoscope <sign>          -> daily horoscope (API)
//   .tarot                     -> draw a random tarot card
//   .dream <keyword>           -> dream interpretation
//   .zodiacsign <YYYY-MM-DD>   -> western zodiac sign
//   .compatibility <s1> <s2>   -> zodiac compatibility score
//
// All run locally except .horoscope (uses a free API) and match the GOLD-MD
// design language exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .CHINESEZODIAC ──────────────────────────────────────────────────────────

func chinesezodiacGuide(prefix string) string {
	return "*🔰 CHINESE ZODIAC 🔰*\n\n" +
		"*FIND THE CHINESE ZODIAC ANIMAL FOR ANY YEAR*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CHINESEZODIAC <YEAR> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CHINESEZODIAC 1995 ❯*"
}

var chineseAnimals = []string{"RAT", "OX", "TIGER", "RABBIT", "DRAGON", "SNAKE", "HORSE", "GOAT", "MONKEY", "ROOSTER", "DOG", "PIG"}
var chineseEmoji = []string{"🐀", "🐂", "🐅", "🐇", "🐉", "🐍", "🐎", "🐐", "🐒", "🐓", "🐕", "🐖"}

func handleChinesezodiac(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, chinesezodiacGuide(prefix))
			return
		}
		year, err := strconv.Atoi(args[0])
		if err != nil || year < 1900 || year > 2100 {
			s.Reply(info, "*🔰 INVALID YEAR, USE "+prefix+"CHINESEZODIAC <YEAR>*")
			return
		}
		idx := ((year-4)%12 + 12) % 12
		var b strings.Builder
		b.WriteString("*🔰 CHINESE ZODIAC 🔰*\n\n")
		b.WriteString("*📅 YEAR ❯ " + strconv.Itoa(year) + "*\n\n")
		b.WriteString("*" + chineseEmoji[idx] + " ANIMAL ❯ " + chineseAnimals[idx] + "*\n")
		b.WriteString("*🔢 POSITION ❯ " + strconv.Itoa(idx+1) + " OF 12*")
		s.Reply(info, b.String())
	})
}

// ── .BIRTHSTONE ─────────────────────────────────────────────────────────────

func birthstoneGuide(prefix string) string {
	return "*🔰 BIRTHSTONE 🔰*\n\n" +
		"*FIND THE BIRTHSTONE FOR ANY MONTH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BIRTHSTONE <MONTH 1-12> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BIRTHSTONE 6 ❯*"
}

var birthstones = []string{"GARNET", "AMETHYST", "AQUAMARINE", "DIAMOND", "EMERALD", "PEARL", "RUBY", "PERIDOT", "SAPPHIRE", "OPAL", "TOPAZ", "TURQUOISE"}
var birthstoneEmoji = []string{"🔴", "🟣", "🔵", "💎", "🟢", "⚪", "🔴", "🟢", "🔵", "🌈", "🟡", "🔵"}

func handleBirthstone(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, birthstoneGuide(prefix))
			return
		}
		m, err := strconv.Atoi(args[0])
		if err != nil || m < 1 || m > 12 {
			s.Reply(info, "*🔰 INVALID MONTH, USE "+prefix+"BIRTHSTONE <1-12>*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 BIRTHSTONE 🔰*\n\n")
		b.WriteString("*📅 MONTH ❯ " + time.Month(m).String() + "*\n\n")
		b.WriteString("*" + birthstoneEmoji[m-1] + " STONE ❯ " + birthstones[m-1] + "*")
		s.Reply(info, b.String())
	})
}

// ── .BIRTHFLOWER ────────────────────────────────────────────────────────────

func birthflowerGuide(prefix string) string {
	return "*🔰 BIRTH FLOWER 🔰*\n\n" +
		"*FIND THE BIRTH FLOWER FOR ANY MONTH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BIRTHFLOWER <MONTH 1-12> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BIRTHFLOWER 4 ❯*"
}

var birthflowers = []string{"CARNATION", "VIOLET", "DAFFODIL", "DAISY", "LILY OF THE VALLEY", "ROSE", "LARKSPUR", "GLADIOLUS", "ASTER", "MARIGOLD", "CHRYSANTHEMUM", "NARCISSUS"}

func handleBirthflower(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, birthflowerGuide(prefix))
			return
		}
		m, err := strconv.Atoi(args[0])
		if err != nil || m < 1 || m > 12 {
			s.Reply(info, "*🔰 INVALID MONTH, USE "+prefix+"BIRTHFLOWER <1-12>*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 BIRTH FLOWER 🔰*\n\n")
		b.WriteString("*📅 MONTH ❯ " + time.Month(m).String() + "*\n\n")
		b.WriteString("*🌸 FLOWER ❯ " + birthflowers[m-1] + "*")
		s.Reply(info, b.String())
	})
}

// ── .NUMEROLOGY ─────────────────────────────────────────────────────────────

func numerologyGuide(prefix string) string {
	return "*🔰 NUMEROLOGY 🔰*\n\n" +
		"*FIND THE NUMEROLOGY NUMBER OF YOUR NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NUMEROLOGY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NUMEROLOGY JOHN ❯*"
}

func reduceNum(n int) int {
	for n > 9 && n != 11 && n != 22 && n != 33 {
		sum := 0
		for n > 0 {
			sum += n % 10
			n /= 10
		}
		n = sum
	}
	return n
}

func handleNumerology(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.Join(args, " ")
		if strings.TrimSpace(name) == "" {
			s.Reply(info, numerologyGuide(prefix))
			return
		}
		total := 0
		for _, r := range strings.ToUpper(name) {
			if r >= 'A' && r <= 'Z' {
				total += int(r-'A') + 1
			}
		}
		num := reduceNum(total)
		var b strings.Builder
		b.WriteString("*🔰 NUMEROLOGY 🔰*\n\n")
		b.WriteString("*📝 NAME ❯ " + strings.ToUpper(name) + "*\n")
		b.WriteString("*➕ LETTER SUM ❯ " + strconv.Itoa(total) + "*\n\n")
		b.WriteString("*🔢 NUMBER ❯ " + strconv.Itoa(num) + "*")
		s.Reply(info, b.String())
	})
}

// ── .LIFEPATH ───────────────────────────────────────────────────────────────

func lifepathGuide(prefix string) string {
	return "*🔰 LIFE PATH NUMBER 🔰*\n\n" +
		"*FIND YOUR LIFE PATH NUMBER FROM YOUR BIRTHDATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LIFEPATH <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LIFEPATH 1990-05-20 ❯*"
}

func handleLifepath(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, lifepathGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		digits := d.Format("20060102")
		total := 0
		for _, c := range digits {
			total += int(c - '0')
		}
		num := reduceNum(total)
		var b strings.Builder
		b.WriteString("*🔰 LIFE PATH NUMBER 🔰*\n\n")
		b.WriteString("*📅 BIRTHDATE ❯ " + d.Format("02 Jan 2006") + "*\n")
		b.WriteString("*➕ DIGIT SUM ❯ " + strconv.Itoa(total) + "*\n\n")
		b.WriteString("*🔢 LIFE PATH ❯ " + strconv.Itoa(num) + "*")
		s.Reply(info, b.String())
	})
}

// ── .HOROSCOPE ──────────────────────────────────────────────────────────────

func horoscopeGuide(prefix string) string {
	return "*🔰 DAILY HOROSCOPE 🔰*\n\n" +
		"*GET TODAY'S HOROSCOPE FOR YOUR ZODIAC SIGN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HOROSCOPE <SIGN> ❯*\n" +
		"*SIGNS ❯ ARIES TAURUS GEMINI CANCER LEO VIRGO LIBRA SCORPIO SAGITTARIUS CAPRICORN AQUARIUS PISCES*\n" +
		"*EXAMPLE ❮ " + prefix + "HOROSCOPE LEO ❯*"
}

func handleHoroscope(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, horoscopeGuide(prefix))
			return
		}
		sign := strings.ToLower(strings.TrimSpace(args[0]))
		valid := map[string]bool{"aries": true, "taurus": true, "gemini": true, "cancer": true, "leo": true, "virgo": true, "libra": true, "scorpio": true, "sagittarius": true, "capricorn": true, "aquarius": true, "pisces": true}
		if !valid[sign] {
			s.Reply(info, "*🔰 INVALID SIGN, USE "+prefix+"HOROSCOPE <SIGN>*")
			return
		}
		var resp struct {
			Data struct {
				Date        string `json:"date"`
				Horoscope   string `json:"horoscope"`
				Zodiac      string `json:"zodiac"`
				Description string `json:"description"`
			} `json:"data"`
		}
		url := "https://horoscope-app-api.vercel.app/api/v1/get-horoscope/daily?sign=" + sign + "&day=TODAY"
		if err := funGetJSON(ctx, url, &resp); err != nil || resp.Data.Horoscope == "" {
			funFail(s, info, "HOROSCOPE")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DAILY HOROSCOPE 🔰*\n\n")
		b.WriteString("*♈ SIGN ❯ " + strings.ToUpper(sign) + "*\n")
		b.WriteString("*📅 DATE ❯ " + resp.Data.Date + "*\n\n")
		b.WriteString("*🔮 " + resp.Data.Horoscope + "*")
		s.Reply(info, b.String())
	})
}

// ── .TAROT ──────────────────────────────────────────────────────────────────

func tarotGuide(prefix string) string {
	return "*🔰 TAROT CARD 🔰*\n\n" +
		"*DRAW A RANDOM TAROT CARD FOR GUIDANCE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TAROT ❯*"
}

var tarotCards = []string{
	"The Fool", "The Magician", "The High Priestess", "The Empress", "The Emperor",
	"The Hierophant", "The Lovers", "The Chariot", "Strength", "The Hermit",
	"Wheel of Fortune", "Justice", "The Hanged Man", "Death", "Temperance",
	"The Devil", "The Tower", "The Star", "The Moon", "The Sun", "Judgement", "The World",
}

func handleTarot(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tarotCards))))
		if err != nil {
			funFail(s, info, "TAROT")
			return
		}
		card := tarotCards[n.Int64()]
		var b strings.Builder
		b.WriteString("*🔰 TAROT CARD 🔰*\n\n")
		b.WriteString("*🃏 CARD ❯ " + strings.ToUpper(card) + "*\n")
		b.WriteString("*🔢 NUMBER ❯ " + strconv.FormatInt(n.Int64()+1, 10) + " OF 22*")
		s.Reply(info, b.String())
	})
}

// ── .DREAM ──────────────────────────────────────────────────────────────────

func dreamGuide(prefix string) string {
	return "*🔰 DREAM MEANING 🔰*\n\n" +
		"*GET A SIMPLE INTERPRETATION OF A DREAM SYMBOL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DREAM <KEYWORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DREAM WATER ❯*"
}

var dreamMeanings = map[string]string{
	"water":   "Emotions and feelings. Clear water means peace, muddy water means confusion.",
	"fire":    "Passion, anger or transformation. It signals strong energy in your life.",
	"flying":  "Freedom and ambition. You want to rise above your limits.",
	"falling": "Insecurity or loss of control. You may fear failing at something.",
	"snake":   "Change, healing or hidden fear. Shedding old skin means growth.",
	"death":   "Not literal death, but the end of a phase and a new beginning.",
	"baby":    "New beginnings, innocence or a new project you are nurturing.",
	"money":   "Self worth and security. Finding money means new opportunity.",
	"teeth":   "Anxiety about appearance or communication. Often linked to stress.",
	"chase":   "Avoidance. You are running from a problem or responsibility.",
	"exam":    "Fear of judgement or being tested in real life.",
	"wedding": "Commitment and union. A new bond or promise is forming.",
	"rain":    "Release and cleansing. Sadness that will soon pass.",
	"cat":     "Independence and intuition. Trust your instincts.",
	"dog":     "Loyalty and friendship. A trusted companion supports you.",
	"house":   "Yourself and your mind. Different rooms show different emotions.",
	"car":     "Your direction in life. Who drives shows who is in control.",
	"road":    "Your life journey. A fork means an important decision.",
	"mirror":  "Self reflection. You are examining your own identity.",
	"door":    "Opportunity. An open door means a chance, a closed one a block.",
}

func handleDream(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		key := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		if key == "" {
			s.Reply(info, dreamGuide(prefix))
			return
		}
		meaning, ok := dreamMeanings[key]
		if !ok {
			var b strings.Builder
			b.WriteString("*🔰 DREAM MEANING 🔰*\n\n")
			b.WriteString("*🔍 SYMBOL ❯ " + strings.ToUpper(key) + "*\n\n")
			b.WriteString("*❓ NO EXACT MATCH. TRY WORDS LIKE WATER, FIRE, FLYING, FALLING, SNAKE, DEATH, BABY, MONEY, TEETH, CHASE, EXAM, WEDDING, RAIN, CAT, DOG, HOUSE, CAR, ROAD, MIRROR, DOOR*")
			s.Reply(info, b.String())
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DREAM MEANING 🔰*\n\n")
		b.WriteString("*🔍 SYMBOL ❯ " + strings.ToUpper(key) + "*\n\n")
		b.WriteString("*💭 " + meaning + "*")
		s.Reply(info, b.String())
	})
}

// ── .ZODIACSIGN ─────────────────────────────────────────────────────────────

func zodiacsignGuide(prefix string) string {
	return "*🔰 ZODIAC SIGN 🔰*\n\n" +
		"*FIND YOUR WESTERN ZODIAC SIGN FROM YOUR BIRTHDATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ZODIACSIGN <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ZODIACSIGN 1995-08-10 ❯*"
}

func zodiacFromDate(m time.Month, d int) (string, string) {
	switch {
	case (m == time.March && d >= 21) || (m == time.April && d <= 19):
		return "ARIES", "♈"
	case (m == time.April && d >= 20) || (m == time.May && d <= 20):
		return "TAURUS", "♉"
	case (m == time.May && d >= 21) || (m == time.June && d <= 20):
		return "GEMINI", "♊"
	case (m == time.June && d >= 21) || (m == time.July && d <= 22):
		return "CANCER", "♋"
	case (m == time.July && d >= 23) || (m == time.August && d <= 22):
		return "LEO", "♌"
	case (m == time.August && d >= 23) || (m == time.September && d <= 22):
		return "VIRGO", "♍"
	case (m == time.September && d >= 23) || (m == time.October && d <= 22):
		return "LIBRA", "♎"
	case (m == time.October && d >= 23) || (m == time.November && d <= 21):
		return "SCORPIO", "♏"
	case (m == time.November && d >= 22) || (m == time.December && d <= 21):
		return "SAGITTARIUS", "♐"
	case (m == time.December && d >= 22) || (m == time.January && d <= 19):
		return "CAPRICORN", "♑"
	case (m == time.January && d >= 20) || (m == time.February && d <= 18):
		return "AQUARIUS", "♒"
	default:
		return "PISCES", "♓"
	}
}

func handleZodiacsign(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, zodiacsignGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		sign, emoji := zodiacFromDate(d.Month(), d.Day())
		var b strings.Builder
		b.WriteString("*🔰 ZODIAC SIGN 🔰*\n\n")
		b.WriteString("*📅 BIRTHDATE ❯ " + d.Format("02 Jan 2006") + "*\n\n")
		b.WriteString("*" + emoji + " SIGN ❯ " + sign + "*")
		s.Reply(info, b.String())
	})
}

// ── .COMPATIBILITY ──────────────────────────────────────────────────────────

func compatibilityGuide(prefix string) string {
	return "*🔰 ZODIAC COMPATIBILITY 🔰*\n\n" +
		"*CHECK THE COMPATIBILITY BETWEEN TWO ZODIAC SIGNS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COMPATIBILITY <SIGN1> <SIGN2> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COMPATIBILITY LEO ARIES ❯*"
}

var zodiacOrder = []string{"ARIES", "TAURUS", "GEMINI", "CANCER", "LEO", "VIRGO", "LIBRA", "SCORPIO", "SAGITTARIUS", "CAPRICORN", "AQUARIUS", "PISCES"}

func handleCompatibility(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, compatibilityGuide(prefix))
			return
		}
		a := strings.ToUpper(strings.TrimSpace(args[0]))
		c := strings.ToUpper(strings.TrimSpace(args[1]))
		ia, ic := -1, -1
		for i, z := range zodiacOrder {
			if z == a {
				ia = i
			}
			if z == c {
				ic = i
			}
		}
		if ia < 0 || ic < 0 {
			s.Reply(info, "*🔰 INVALID SIGN, USE "+prefix+"COMPATIBILITY <SIGN1> <SIGN2>*")
			return
		}
		diff := ia - ic
		if diff < 0 {
			diff = -diff
		}
		if diff > 6 {
			diff = 12 - diff
		}
		score := 100 - diff*10
		if score < 30 {
			score = 30
		}
		verdict := "CHALLENGING"
		switch {
		case score >= 90:
			verdict = "SOULMATES"
		case score >= 70:
			verdict = "GREAT MATCH"
		case score >= 50:
			verdict = "GOOD MATCH"
		}
		var b strings.Builder
		b.WriteString("*🔰 ZODIAC COMPATIBILITY 🔰*\n\n")
		b.WriteString("*" + a + " ❤️ " + c + "*\n\n")
		b.WriteString("*📊 SCORE ❯ " + strconv.Itoa(score) + " %*\n")
		b.WriteString("*💫 VERDICT ❯ " + verdict + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "chinesezodiac", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE CHINESE ZODIAC ANIMAL FOR A YEAR. USE IT AS .CHINESEZODIAC <YEAR>.", Run: handleChinesezodiac})
	Register(Command{Name: "birthstone", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE BIRTHSTONE FOR A MONTH. USE IT AS .BIRTHSTONE <MONTH 1-12>.", Run: handleBirthstone})
	Register(Command{Name: "birthflower", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE BIRTH FLOWER FOR A MONTH. USE IT AS .BIRTHFLOWER <MONTH 1-12>.", Run: handleBirthflower})
	Register(Command{Name: "numerology", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE NUMEROLOGY NUMBER OF A NAME. USE IT AS .NUMEROLOGY <NAME>.", Run: handleNumerology})
	Register(Command{Name: "lifepath", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND YOUR LIFE PATH NUMBER. USE IT AS .LIFEPATH <YYYY-MM-DD>.", Run: handleLifepath})
	Register(Command{Name: "horoscope", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TODAY'S HOROSCOPE FOR A ZODIAC SIGN. USE IT AS .HOROSCOPE <SIGN>.", Run: handleHoroscope})
	Register(Command{Name: "tarot", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO DRAW A RANDOM TAROT CARD. USE IT AS .TAROT.", Run: handleTarot})
	Register(Command{Name: "dream", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A SIMPLE INTERPRETATION OF A DREAM SYMBOL. USE IT AS .DREAM <KEYWORD>.", Run: handleDream})
	Register(Command{Name: "zodiacsign", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND YOUR WESTERN ZODIAC SIGN. USE IT AS .ZODIACSIGN <YYYY-MM-DD>.", Run: handleZodiacsign})
	Register(Command{Name: "compatibility", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK ZODIAC COMPATIBILITY BETWEEN TWO SIGNS. USE IT AS .COMPATIBILITY <SIGN1> <SIGN2>.", Run: handleCompatibility})

	// hidden aliases
	Register(Command{Name: "zodiacanimal", Category: "TOOLS", Desc: "Short alias of .chinesezodiac", Hidden: true, Run: handleChinesezodiac})
	Register(Command{Name: "mystonestone", Category: "TOOLS", Desc: "Short alias of .birthstone", Hidden: true, Run: handleBirthstone})
	Register(Command{Name: "myflower", Category: "TOOLS", Desc: "Short alias of .birthflower", Hidden: true, Run: handleBirthflower})
	Register(Command{Name: "numerologynum", Category: "TOOLS", Desc: "Short alias of .numerology", Hidden: true, Run: handleNumerology})
	Register(Command{Name: "lifepathnum", Category: "TOOLS", Desc: "Short alias of .lifepath", Hidden: true, Run: handleLifepath})
	Register(Command{Name: "dailyhoroscope", Category: "TOOLS", Desc: "Short alias of .horoscope", Hidden: true, Run: handleHoroscope})
	Register(Command{Name: "tarotcard", Category: "TOOLS", Desc: "Short alias of .tarot", Hidden: true, Run: handleTarot})
	Register(Command{Name: "dreammeaning", Category: "TOOLS", Desc: "Short alias of .dream", Hidden: true, Run: handleDream})
	Register(Command{Name: "mysign", Category: "TOOLS", Desc: "Short alias of .zodiacsign", Hidden: true, Run: handleZodiacsign})
	Register(Command{Name: "zodiacmatch", Category: "TOOLS", Desc: "Short alias of .compatibility", Hidden: true, Run: handleCompatibility})
}
