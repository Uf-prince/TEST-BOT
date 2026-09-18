package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 26 (10 new random generator commands)
// File: toolpack26.go
// ============================================================================
//   .randomadvice          -> random life advice (API)
//   .randomcolor           -> random colour with hex code
//   .randomemoji           -> random emoji
//   .randomfact            -> random fun fact (API)
//   .randomhex             -> random hex colour code
//   .randomletter          -> random letter A-Z
//   .randomname            -> random person name
//   .randompin             -> random 4-6 digit PIN
//   .randomquote           -> random inspirational quote (API)
//   .randomtip             -> random useful tip
//
// Matches the GOLD-MD design language exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .RANDOMADVICE ────────────────────────────────────────────────────────────

func randomadviceGuide(prefix string) string {
	return "*🔰 RANDOM ADVICE 🔰*\n\n" +
		"*GET A RANDOM PIECE OF LIFE ADVICE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMADVICE ❯*"
}

func handleRandomadvice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var resp struct {
			Slip struct {
				Advice string `json:"advice"`
			} `json:"slip"`
		}
		if err := funGetJSON(ctx, "https://api.adviceslip.com/advice", &resp); err != nil || strings.TrimSpace(resp.Slip.Advice) == "" {
			funFail(s, info, "ADVICE")
			return
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM ADVICE 🔰*\n\n")
		out.WriteString("*💡 " + strings.ToUpper(resp.Slip.Advice) + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMCOLOR ─────────────────────────────────────────────────────────────

func randomcolorGuide(prefix string) string {
	return "*🔰 RANDOM COLOR 🔰*\n\n" +
		"*GET A RANDOM COLOUR WITH ITS HEX CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMCOLOR ❯*"
}

func handleRandomcolor(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		hex := fmt.Sprintf("#%02X%02X%02X", randIntn(256), randIntn(256), randIntn(256))
		var out strings.Builder
		out.WriteString("*🔰 RANDOM COLOR 🔰*\n\n")
		out.WriteString("*🎨 HEX ❯ " + hex + "*\n")
		out.WriteString("*🔴 RED ❯ " + strconv.Itoa(hexVal(hex, 1)) + "*\n")
		out.WriteString("*🟢 GREEN ❯ " + strconv.Itoa(hexVal(hex, 3)) + "*\n")
		out.WriteString("*🔵 BLUE ❯ " + strconv.Itoa(hexVal(hex, 5)) + "*")
		s.Reply(info, out.String())
	})
}

func hexVal(hex string, start int) int {
	v, _ := strconv.ParseInt(hex[start:start+2], 16, 32)
	return int(v)
}

// ── .RANDOMEMOJI ─────────────────────────────────────────────────────────────

func randomemojiGuide(prefix string) string {
	return "*🔰 RANDOM EMOJI 🔰*\n\n" +
		"*GET A RANDOM EMOJI*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMEMOJI ❯*"
}

func handleRandomemoji(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		emojis := []string{"😀", "😂", "🥰", "😎", "🤩", "🥳", "😴", "🤔", "😇", "🤗", "🐶", "🐱", "🦁", "🐼", "🦊", "🐸", "🍕", "🍔", "🍟", "🍩", "⚽", "🏀", "🎮", "🎸", "🚀", "🌈", "⭐", "🔥", "💎", "🎉", "🍀", "🌻", "🦋", "🐬", "🎯", "🏆", "💡", "🎁", "🧸", "🍉"}
		e := emojis[randIntn(len(emojis))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM EMOJI 🔰*\n\n")
		out.WriteString("*🎲 YOUR EMOJI ❯ " + e + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMFACT ──────────────────────────────────────────────────────────────

func randomfactGuide(prefix string) string {
	return "*🔰 RANDOM FACT 🔰*\n\n" +
		"*GET A RANDOM FUN FACT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMFACT ❯*"
}

func handleRandomfact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var resp struct {
			Text string `json:"text"`
		}
		if err := funGetJSON(ctx, "https://uselessfacts.jsph.pl/api/v2/facts/random", &resp); err != nil || strings.TrimSpace(resp.Text) == "" {
			funFail(s, info, "FACT")
			return
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM FACT 🔰*\n\n")
		out.WriteString("*🧠 " + strings.ToUpper(resp.Text) + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMHEX ───────────────────────────────────────────────────────────────

func randomhexGuide(prefix string) string {
	return "*🔰 RANDOM HEX 🔰*\n\n" +
		"*GET A RANDOM HEX COLOUR CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMHEX ❯*"
}

func handleRandomhex(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		hex := fmt.Sprintf("#%02X%02X%02X", randIntn(256), randIntn(256), randIntn(256))
		var out strings.Builder
		out.WriteString("*🔰 RANDOM HEX 🔰*\n\n")
		out.WriteString("*🎨 HEX CODE ❯ " + hex + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMLETTER ────────────────────────────────────────────────────────────

func randomletterGuide(prefix string) string {
	return "*🔰 RANDOM LETTER 🔰*\n\n" +
		"*GET A RANDOM LETTER FROM A TO Z*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMLETTER ❯*"
}

func handleRandomletter(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		letter := string(rune('A' + randIntn(26)))
		var out strings.Builder
		out.WriteString("*🔰 RANDOM LETTER 🔰*\n\n")
		out.WriteString("*🔤 LETTER ❯ " + letter + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMNAME ──────────────────────────────────────────────────────────────

func randomnameGuide(prefix string) string {
	return "*🔰 RANDOM NAME 🔰*\n\n" +
		"*GENERATE A RANDOM PERSON NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMNAME ❯*"
}

func handleRandomname(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		first := []string{"Ali", "Sara", "John", "Ayesha", "David", "Fatima", "Omar", "Emma", "Bilal", "Zara", "Liam", "Hina", "Noah", "Maya", "Hassan", "Aisha", "Adam", "Noor", "Ryan", "Sana"}
		last := []string{"Khan", "Ahmed", "Smith", "Malik", "Johnson", "Raza", "Brown", "Sheikh", "Wilson", "Iqbal", "Davis", "Butt", "Taylor", "Chaudhry", "Clark", "Qureshi", "Lewis", "Baig", "Walker", "Mirza"}
		name := first[randIntn(len(first))] + " " + last[randIntn(len(last))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM NAME 🔰*\n\n")
		out.WriteString("*👤 NAME ❯ " + name + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMPIN ───────────────────────────────────────────────────────────────

func randompinGuide(prefix string) string {
	return "*🔰 RANDOM PIN 🔰*\n\n" +
		"*GENERATE A RANDOM NUMERIC PIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMPIN [LENGTH] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMPIN 6 ❯*"
}

func handleRandompin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		length := 4
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n >= 3 && n <= 12 {
				length = n
			}
		}
		var b strings.Builder
		for i := 0; i < length; i++ {
			b.WriteByte(byte('0' + randIntn(10)))
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM PIN 🔰*\n\n")
		out.WriteString("*🔢 LENGTH ❯ " + strconv.Itoa(length) + "*\n\n")
		out.WriteString("*`" + b.String() + "`*\n\n")
		out.WriteString("*⚠️ NEVER SHARE THIS WITH ANYONE*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMQUOTE ─────────────────────────────────────────────────────────────

func randomquoteGuide(prefix string) string {
	return "*🔰 RANDOM QUOTE 🔰*\n\n" +
		"*GET A RANDOM INSPIRATIONAL QUOTE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMQUOTE ❯*"
}

func handleRandomquote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var resp []struct {
			Q string `json:"q"`
			A string `json:"a"`
		}
		if err := funGetJSON(ctx, "https://zenquotes.io/api/random", &resp); err != nil || len(resp) == 0 || strings.TrimSpace(resp[0].Q) == "" {
			funFail(s, info, "QUOTE")
			return
		}
		var out strings.Builder
		out.WriteString("*🔰 RANDOM QUOTE 🔰*\n\n")
		out.WriteString("*💬 " + strings.ToUpper(resp[0].Q) + "*\n\n")
		out.WriteString("*✍️ " + strings.ToUpper(resp[0].A) + "*")
		s.Reply(info, out.String())
	})
}

// ── .RANDOMTIP ───────────────────────────────────────────────────────────────

func randomtipGuide(prefix string) string {
	return "*🔰 RANDOM TIP 🔰*\n\n" +
		"*GET A RANDOM USEFUL TIP*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMTIP ❯*"
}

func handleRandomtip(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		tips := []string{
			"Drink a glass of water right after waking up to kickstart your metabolism.",
			"Use the 2-minute rule: if a task takes under 2 minutes, do it now.",
			"Put your phone away 30 minutes before bed for better sleep.",
			"Write down 3 things you are grateful for every night.",
			"Take the stairs whenever you can, small habits add up.",
			"Keep a reusable water bottle with you to stay hydrated.",
			"Review your spending once a week to stay on budget.",
			"Learn one new word every day to grow your vocabulary.",
			"Stretch for 5 minutes every hour if you sit for long.",
			"Back up your important files to the cloud regularly.",
			"Read for 20 minutes a day instead of scrolling social media.",
			"Plan your next day the night before to start focused.",
			"Keep a small emergency fund of at least 3 months of expenses.",
			"Turn off notifications for apps you do not need instantly.",
			"Walk for 10 minutes after meals to help digestion.",
		}
		tip := tips[randIntn(len(tips))]
		var out strings.Builder
		out.WriteString("*🔰 RANDOM TIP 🔰*\n\n")
		out.WriteString("*💡 " + strings.ToUpper(tip) + "*")
		s.Reply(info, out.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "randomadvice", Category: "TOOLS", Desc: "Get a random piece of life advice", Run: handleRandomadvice})
	Register(Command{Name: "randomcolor", Category: "TOOLS", Desc: "Get a random colour with its hex code", Run: handleRandomcolor})
	Register(Command{Name: "randomemoji", Category: "TOOLS", Desc: "Get a random emoji", Run: handleRandomemoji})
	Register(Command{Name: "randomfact", Category: "TOOLS", Desc: "Get a random fun fact", Run: handleRandomfact})
	Register(Command{Name: "randomhex", Category: "TOOLS", Desc: "Get a random hex colour code", Run: handleRandomhex})
	Register(Command{Name: "randomletter", Category: "TOOLS", Desc: "Get a random letter from A to Z", Run: handleRandomletter})
	Register(Command{Name: "randomname", Category: "TOOLS", Desc: "Generate a random person name", Run: handleRandomname})
	Register(Command{Name: "randompin", Category: "TOOLS", Desc: "Generate a random numeric PIN", Run: handleRandompin})
	Register(Command{Name: "randomquote", Category: "TOOLS", Desc: "Get a random inspirational quote", Run: handleRandomquote})
	Register(Command{Name: "randomtip", Category: "TOOLS", Desc: "Get a random useful tip", Run: handleRandomtip})

	Register(Command{Name: "advicetip", Category: "TOOLS", Desc: "Short alias of .randomadvice", Hidden: true, Run: handleRandomadvice})
	Register(Command{Name: "randcolor", Category: "TOOLS", Desc: "Short alias of .randomcolor", Hidden: true, Run: handleRandomcolor})
	Register(Command{Name: "randemoji", Category: "TOOLS", Desc: "Short alias of .randomemoji", Hidden: true, Run: handleRandomemoji})
	Register(Command{Name: "randfact", Category: "TOOLS", Desc: "Short alias of .randomfact", Hidden: true, Run: handleRandomfact})
	Register(Command{Name: "randhex", Category: "TOOLS", Desc: "Short alias of .randomhex", Hidden: true, Run: handleRandomhex})
	Register(Command{Name: "randletter", Category: "TOOLS", Desc: "Short alias of .randomletter", Hidden: true, Run: handleRandomletter})
	Register(Command{Name: "randname", Category: "TOOLS", Desc: "Short alias of .randomname", Hidden: true, Run: handleRandomname})
	Register(Command{Name: "randpin", Category: "TOOLS", Desc: "Short alias of .randompin", Hidden: true, Run: handleRandompin})
	Register(Command{Name: "randquote", Category: "TOOLS", Desc: "Short alias of .randomquote", Hidden: true, Run: handleRandomquote})
	Register(Command{Name: "randtip", Category: "TOOLS", Desc: "Short alias of .randomtip", Hidden: true, Run: handleRandomtip})
}
