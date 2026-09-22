package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 23 (10 new random & password commands)
// File: toolpack23.go
// ============================================================================
//   .passwordstrength <pass>  -> analyse password strength
//   .randompassword [len]     -> generate a strong random password
//   .randomuuid               -> generate a random UUID v4
//   .randint <min> <max>      -> random integer in range
//   .randomdice [sides] [n]   -> roll dice
//   .randomcard               -> draw a random playing card
//   .randomcoin               -> flip a coin
//   .randomwheel a, b, c      -> spin a wheel of options
//   .randomteam <n> a, b, c   -> split names into n teams
//   .randompick a, b, c       -> pick one random item
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// randIntn returns a cryptographically-random int in [0, n).
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// ── .PASSWORDSTRENGTH ────────────────────────────────────────────────────────

func passwordstrengthGuide(prefix string) string {
	return "*🔰 PASSWORD STRENGTH 🔰*\n\n" +
		"*CHECK HOW STRONG A PASSWORD IS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PASSWORDSTRENGTH <PASSWORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PASSWORDSTRENGTH Abc@12345 ❯*"
}

func handlePasswordstrength(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		pass := strings.Join(args, " ")
		if strings.TrimSpace(pass) == "" {
			s.Reply(info, passwordstrengthGuide(prefix))
			return
		}
		var lower, upper, digit, symbol bool
		for _, r := range pass {
			switch {
			case r >= 'a' && r <= 'z':
				lower = true
			case r >= 'A' && r <= 'Z':
				upper = true
			case r >= '0' && r <= '9':
				digit = true
			default:
				symbol = true
			}
		}
		score := 0
		score += len(pass) * 4
		if lower {
			score += 10
		}
		if upper {
			score += 10
		}
		if digit {
			score += 10
		}
		if symbol {
			score += 15
		}
		if len(pass) >= 12 {
			score += 10
		}
		if len(pass) >= 16 {
			score += 10
		}
		if score > 100 {
			score = 100
		}
		label := "VERY WEAK"
		emoji := "🔴"
		switch {
		case score >= 85:
			label, emoji = "VERY STRONG", "🟢"
		case score >= 65:
			label, emoji = "STRONG", "🟢"
		case score >= 45:
			label, emoji = "MEDIUM", "🟡"
		case score >= 25:
			label, emoji = "WEAK", "🟠"
		}
		var b strings.Builder
		b.WriteString("*🔰 PASSWORD STRENGTH 🔰*\n\n")
		b.WriteString("*🔑 LENGTH ❯ " + strconv.Itoa(len(pass)) + "*\n")
		b.WriteString("*🔡 LOWERCASE ❯ " + yesNo(lower) + "*\n")
		b.WriteString("*🔠 UPPERCASE ❯ " + yesNo(upper) + "*\n")
		b.WriteString("*🔢 DIGITS ❯ " + yesNo(digit) + "*\n")
		b.WriteString("*✳️ SYMBOLS ❯ " + yesNo(symbol) + "*\n\n")
		b.WriteString("*" + emoji + " SCORE ❯ " + strconv.Itoa(score) + " / 100*\n")
		b.WriteString("*📊 RATING ❯ " + label + "*")
		s.Reply(info, b.String())
	})
}

func yesNo(v bool) string {
	if v {
		return "YES"
	}
	return "NO"
}

// ── .RANDOMPASSWORD ──────────────────────────────────────────────────────────

func randompasswordGuide(prefix string) string {
	return "*🔰 RANDOM PASSWORD 🔰*\n\n" +
		"*GENERATE A STRONG RANDOM PASSWORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMPASSWORD [LENGTH] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMPASSWORD 16 ❯*"
}

func handleRandompassword(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		length := 16
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n >= 4 && n <= 128 {
				length = n
			}
		}
		const lower = "abcdefghijklmnopqrstuvwxyz"
		const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		const digits = "0123456789"
		const symbols = "!@#$%^&*()-_=+"
		all := lower + upper + digits + symbols
		var b strings.Builder
		for i := 0; i < length; i++ {
			b.WriteByte(all[randIntn(len(all))])
		}
		pass := b.String()
		var out strings.Builder
		out.WriteString("*🔰 RANDOM PASSWORD 🔰*\n\n")
		out.WriteString("*🔑 LENGTH ❯ " + strconv.Itoa(length) + "*\n\n")
		out.WriteString("*`" + pass + "`*\n\n")
		out.WriteString("*⚠️ NEVER SHARE THIS WITH ANYONE*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMUUID ──────────────────────────────────────────────────────────────

func randomuuidGuide(prefix string) string {
	return "*🔰 RANDOM UUID 🔰*\n\n" +
		"*GENERATE A RANDOM UUID V4*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMUUID ❯*"
}

func handleRandomuuid(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			funFail(s, info, "UUID GENERATION")
			return
		}
		buf[6] = (buf[6] & 0x0f) | 0x40
		buf[8] = (buf[8] & 0x3f) | 0x80
		uuid := fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
		var out strings.Builder
		out.WriteString("*🔰 RANDOM UUID 🔰*\n\n")
		out.WriteString("*🆔 UUID ❯*\n")
		out.WriteString("*`" + uuid + "`*")
		s.Reply(info, out.String())
	})
}

// ── .RANDINT ─────────────────────────────────────────────────────────────────

func randintGuide(prefix string) string {
	return "*🔰 RANDOM INTEGER 🔰*\n\n" +
		"*GET A RANDOM NUMBER BETWEEN MIN AND MAX*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDINT <MIN> <MAX> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDINT 1 100 ❯*"
}

func handleRandint(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, randintGuide(prefix))
			return
		}
		min, err1 := strconv.Atoi(strings.TrimSpace(args[0]))
		max, err2 := strconv.Atoi(strings.TrimSpace(args[1]))
		if err1 != nil || err2 != nil {
			s.Reply(info, "*🔰 RANDOM INTEGER 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		if min > max {
			min, max = max, min
		}
		val := min + randIntn(max-min+1)
		var out strings.Builder
		out.WriteString("*🔰 RANDOM INTEGER 🔰*\n\n")
		out.WriteString("*📉 MIN ❯ " + strconv.Itoa(min) + "*\n")
		out.WriteString("*📈 MAX ❯ " + strconv.Itoa(max) + "*\n\n")
		out.WriteString("*🎲 RESULT ❯ " + strconv.Itoa(val) + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMDICE ──────────────────────────────────────────────────────────────

func randomdiceGuide(prefix string) string {
	return "*🔰 RANDOM DICE 🔰*\n\n" +
		"*ROLL ONE OR MORE DICE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMDICE [SIDES] [COUNT] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMDICE 6 2 ❯*"
}

func handleRandomdice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		sides, count := 6, 1
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n >= 2 && n <= 1000 {
				sides = n
			}
		}
		if len(args) > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil && n >= 1 && n <= 20 {
				count = n
			}
		}
		rolls := make([]string, 0, count)
		total := 0
		for i := 0; i < count; i++ {
			v := 1 + randIntn(sides)
			total += v
			rolls = append(rolls, strconv.Itoa(v))
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM DICE 🔰*\n\n")
		out.WriteString("*🎲 SIDES ❯ " + strconv.Itoa(sides) + "*\n")
		out.WriteString("*🔢 COUNT ❯ " + strconv.Itoa(count) + "*\n\n")
		out.WriteString("*🎯 ROLLS ❯ " + strings.Join(rolls, " , ") + "*\n")
		out.WriteString("*➕ TOTAL ❯ " + strconv.Itoa(total) + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMCARD ──────────────────────────────────────────────────────────────

func randomcardGuide(prefix string) string {
	return "*🔰 RANDOM CARD 🔰*\n\n" +
		"*DRAW A RANDOM PLAYING CARD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMCARD ❯*"
}

func handleRandomcard(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
		suits := []string{"♠️ SPADES", "♥️ HEARTS", "♦️ DIAMONDS", "♣️ CLUBS"}
		rank := ranks[randIntn(len(ranks))]
		suit := suits[randIntn(len(suits))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM CARD 🔰*\n\n")
		out.WriteString("*🃏 CARD ❯ " + rank + " OF " + suit + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMCOIN ──────────────────────────────────────────────────────────────

func randomcoinGuide(prefix string) string {
	return "*🔰 RANDOM COIN 🔰*\n\n" +
		"*FLIP A COIN (HEADS OR TAILS)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMCOIN ❯*"
}

func handleRandomcoin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		result := "HEADS 🪙"
		if randIntn(2) == 1 {
			result = "TAILS 🪙"
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM COIN 🔰*\n\n")
		out.WriteString("*🎯 RESULT ❯ " + result + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMWHEEL ─────────────────────────────────────────────────────────────

func randomwheelGuide(prefix string) string {
	return "*🔰 RANDOM WHEEL 🔰*\n\n" +
		"*SPIN A WHEEL OF OPTIONS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMWHEEL OPTION1, OPTION2, OPTION3 ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMWHEEL PIZZA, BURGER, PASTA ❯*"
}

func handleRandomwheel(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, randomwheelGuide(prefix))
			return
		}
		parts := splitList(raw)
		if len(parts) < 2 {
			s.Reply(info, "*🔰 RANDOM WHEEL 🔰*\n\n*❌ PROVIDE AT LEAST 2 OPTIONS SEPARATED BY COMMAS*")
			return
		}
		pick := parts[randIntn(len(parts))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM WHEEL 🔰*\n\n")
		out.WriteString("*🎡 OPTIONS ❯ " + strconv.Itoa(len(parts)) + "*\n\n")
		out.WriteString("*🎯 WINNER ❯ " + strings.ToUpper(pick) + "*")
		s.Reply(info, out.String())
	})
}

func splitList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ── .RANDOMTEAM ──────────────────────────────────────────────────────────────

func randomteamGuide(prefix string) string {
	return "*🔰 RANDOM TEAM 🔰*\n\n" +
		"*SPLIT NAMES INTO RANDOM TEAMS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMTEAM <TEAMS> NAME1, NAME2, NAME3 ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMTEAM 2 ALI, BOB, SARA, JOHN ❯*"
}

func handleRandomteam(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, randomteamGuide(prefix))
			return
		}
		teams, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || teams < 2 || teams > 10 {
			s.Reply(info, "*🔰 RANDOM TEAM 🔰*\n\n*❌ TEAM COUNT MUST BE BETWEEN 2 AND 10*")
			return
		}
		names := splitList(strings.Join(args[1:], " "))
		if len(names) < teams {
			s.Reply(info, "*🔰 RANDOM TEAM 🔰*\n\n*❌ NEED AT LEAST AS MANY NAMES AS TEAMS*")
			return
		}
		// shuffle
		for i := len(names) - 1; i > 0; i-- {
			j := randIntn(i + 1)
			names[i], names[j] = names[j], names[i]
		}
		buckets := make([][]string, teams)
		for i, n := range names {
			buckets[i%teams] = append(buckets[i%teams], n)
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM TEAM 🔰*\n\n")
		out.WriteString("*👥 PLAYERS ❯ " + strconv.Itoa(len(names)) + "*\n")
		out.WriteString("*🏆 TEAMS ❯ " + strconv.Itoa(teams) + "*\n\n")
		for i, b := range buckets {
			out.WriteString("*🔹 TEAM " + strconv.Itoa(i+1) + " ❯ " + strings.Join(b, ", ") + "*\n")
		}
		s.Reply(info, strings.TrimRight(out.String(), "\n"))
	})
}

// ── .RANDOMPICK ──────────────────────────────────────────────────────────────

func randompickGuide(prefix string) string {
	return "*🔰 RANDOM PICK 🔰*\n\n" +
		"*PICK ONE RANDOM ITEM FROM A LIST*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMPICK ITEM1, ITEM2, ITEM3 ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMPICK RED, GREEN, BLUE ❯*"
}

func handleRandompick(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, randompickGuide(prefix))
			return
		}
		parts := splitList(raw)
		if len(parts) == 0 {
			s.Reply(info, randompickGuide(prefix))
			return
		}
		pick := parts[randIntn(len(parts))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM PICK 🔰*\n\n")
		out.WriteString("*📋 ITEMS ❯ " + strconv.Itoa(len(parts)) + "*\n\n")
		out.WriteString("*🎯 PICKED ❯ " + strings.ToUpper(pick) + "*")
		s.Reply(info, out.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "passwordstrength", Category: "TOOLS", Desc: "Check how strong a password is", Run: handlePasswordstrength})
	Register(Command{Name: "randompassword", Category: "TOOLS", Desc: "Generate a strong random password", Run: handleRandompassword})
	Register(Command{Name: "randomuuid", Category: "TOOLS", Desc: "Generate a random UUID v4", Run: handleRandomuuid})
	Register(Command{Name: "randint", Category: "TOOLS", Desc: "Random integer between min and max", Run: handleRandint})
	Register(Command{Name: "randomdice", Category: "TOOLS", Desc: "Roll one or more dice", Run: handleRandomdice})
	Register(Command{Name: "randomcard", Category: "TOOLS", Desc: "Draw a random playing card", Run: handleRandomcard})
	Register(Command{Name: "randomcoin", Category: "TOOLS", Desc: "Flip a coin (heads or tails)", Run: handleRandomcoin})
	Register(Command{Name: "randomwheel", Category: "TOOLS", Desc: "Spin a wheel of options", Run: handleRandomwheel})
	Register(Command{Name: "randomteam", Category: "TOOLS", Desc: "Split names into random teams", Run: handleRandomteam})
	Register(Command{Name: "randompick", Category: "TOOLS", Desc: "Pick one random item from a list", Run: handleRandompick})

	Register(Command{Name: "passstrength", Category: "TOOLS", Desc: "Short alias of .passwordstrength", Hidden: true, Run: handlePasswordstrength})
	Register(Command{Name: "randpass", Category: "TOOLS", Desc: "Short alias of .randompassword", Hidden: true, Run: handleRandompassword})
	Register(Command{Name: "uuidrand", Category: "TOOLS", Desc: "Short alias of .randomuuid", Hidden: true, Run: handleRandomuuid})
	Register(Command{Name: "randomint", Category: "TOOLS", Desc: "Short alias of .randint", Hidden: true, Run: handleRandint})
	Register(Command{Name: "dicerand", Category: "TOOLS", Desc: "Short alias of .randomdice", Hidden: true, Run: handleRandomdice})
	Register(Command{Name: "cardrand", Category: "TOOLS", Desc: "Short alias of .randomcard", Hidden: true, Run: handleRandomcard})
	Register(Command{Name: "coinrand", Category: "TOOLS", Desc: "Short alias of .randomcoin", Hidden: true, Run: handleRandomcoin})
	Register(Command{Name: "wheelspin", Category: "TOOLS", Desc: "Short alias of .randomwheel", Hidden: true, Run: handleRandomwheel})
	Register(Command{Name: "teamgen", Category: "TOOLS", Desc: "Short alias of .randomteam", Hidden: true, Run: handleRandomteam})
	Register(Command{Name: "pickrandom", Category: "TOOLS", Desc: "Short alias of .randompick", Hidden: true, Run: handleRandompick})
}
