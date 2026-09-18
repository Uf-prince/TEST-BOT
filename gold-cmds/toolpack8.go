package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 8 (10 new everyday commands)
// File: toolpack8.go
// ============================================================================
//   .dog        -> random dog image
//   .fox        -> random fox image
//   .duck       -> random duck image
//   .coffee     -> random coffee image
//   .cat        -> random cat image
//   .puppy      -> random puppy image
//   .bored      -> random activity idea
//   .zen        -> random inspirational quote
//   .rhyme <w>  -> words that rhyme with <w>
//   .related <w>-> words related to <w>
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .DOG ────────────────────────────────────────────────────────────────────

func dogGuide(prefix string) string {
	return "*🔰 RANDOM DOG 🔰*\n\n" +
		"*GET A RANDOM DOG PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DOG ❯*"
}

func handleDog(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING DOG....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		}
		if err := funGetJSON(ctx, "https://dog.ceo/api/breeds/image/random", &res); err != nil || res.Message == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DOG")
			}
			return
		}
		data, err := funGetBytes(ctx, res.Message)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DOG")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM DOG 🔰*")
	})
}

// ── .FOX ────────────────────────────────────────────────────────────────────

func foxGuide(prefix string) string {
	return "*🔰 RANDOM FOX 🔰*\n\n" +
		"*GET A RANDOM FOX PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "FOX ❯*"
}

func handleFox(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING FOX....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Image string `json:"image"`
		}
		if err := funGetJSON(ctx, "https://randomfox.ca/floof/", &res); err != nil || res.Image == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "FOX")
			}
			return
		}
		data, err := funGetBytes(ctx, res.Image)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "FOX")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM FOX 🔰*")
	})
}

// ── .DUCK ───────────────────────────────────────────────────────────────────

func duckGuide(prefix string) string {
	return "*🔰 RANDOM DUCK 🔰*\n\n" +
		"*GET A RANDOM DUCK PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DUCK ❯*"
}

func handleDuck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING DUCK....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			URL string `json:"url"`
		}
		if err := funGetJSON(ctx, "https://random-d.uk/api/v2/random", &res); err != nil || res.URL == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DUCK")
			}
			return
		}
		data, err := funGetBytes(ctx, res.URL)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DUCK")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM DUCK 🔰*")
	})
}

// ── .COFFEE ─────────────────────────────────────────────────────────────────

func coffeeGuide(prefix string) string {
	return "*🔰 RANDOM COFFEE 🔰*\n\n" +
		"*GET A RANDOM COFFEE PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COFFEE ❯*"
}

func handleCoffee(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*BREWING COFFEE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			File string `json:"file"`
		}
		if err := funGetJSON(ctx, "https://coffee.alexflipnote.dev/random.json", &res); err != nil || res.File == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COFFEE")
			}
			return
		}
		data, err := funGetBytes(ctx, res.File)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COFFEE")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM COFFEE 🔰*")
	})
}

// ── .CAT ────────────────────────────────────────────────────────────────────

func catGuide(prefix string) string {
	return "*🔰 RANDOM CAT 🔰*\n\n" +
		"*GET A RANDOM CAT PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CAT ❯*"
}

func handleCat(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING CAT....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			URL string `json:"url"`
		}
		if err := funGetJSON(ctx, "https://api.thecatapi.com/v1/images/search", &res); err != nil || len(res) == 0 || res[0].URL == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "CAT")
			}
			return
		}
		data, err := funGetBytes(ctx, res[0].URL)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "CAT")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM CAT 🔰*")
	})
}

// ── .PUPPY ──────────────────────────────────────────────────────────────────

func puppyGuide(prefix string) string {
	return "*🔰 RANDOM PUPPY 🔰*\n\n" +
		"*GET A RANDOM PUPPY PICTURE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PUPPY ❯*"
}

func handlePuppy(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING PUPPY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			URL string `json:"url"`
		}
		if err := funGetJSON(ctx, "https://api.thedogapi.com/v1/images/search", &res); err != nil || len(res) == 0 || res[0].URL == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "PUPPY")
			}
			return
		}
		data, err := funGetBytes(ctx, res[0].URL)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "PUPPY")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 RANDOM PUPPY 🔰*")
	})
}

// ── .BORED ──────────────────────────────────────────────────────────────────

func boredGuide(prefix string) string {
	return "*🔰 BORED? 🔰*\n\n" +
		"*GET A RANDOM ACTIVITY IDEA*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BORED ❯*"
}

func handleBored(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*THINKING....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Activity     string  `json:"activity"`
			Type         string  `json:"type"`
			Participants int     `json:"participants"`
			Price        float64 `json:"price"`
		}
		if err := funGetJSON(ctx, "https://bored-api.appbrewery.com/random", &res); err != nil || res.Activity == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "BORED")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 BORED? TRY THIS 🔰*\n\n")
		b.WriteString("*🎯 ACTIVITY ❯ " + strings.ToUpper(res.Activity) + "*\n")
		if res.Type != "" {
			b.WriteString("*📂 TYPE ❯ " + strings.ToUpper(res.Type) + "*\n")
		}
		b.WriteString("*👥 PARTICIPANTS ❯ " + fmt.Sprintf("%d", res.Participants) + "*\n")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .ZEN ────────────────────────────────────────────────────────────────────

func zenGuide(prefix string) string {
	return "*🔰 ZEN QUOTE 🔰*\n\n" +
		"*GET A RANDOM INSPIRATIONAL QUOTE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ZEN ❯*"
}

func handleZen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QUOTE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Q string `json:"q"`
			A string `json:"a"`
		}
		if err := funGetJSON(ctx, "https://zenquotes.io/api/random", &res); err != nil || len(res) == 0 || res[0].Q == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "ZEN")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 ZEN QUOTE 🔰*\n\n")
		b.WriteString("*💬 \"" + res[0].Q + "\"*\n\n")
		b.WriteString("*✍️ — " + strings.ToUpper(res[0].A) + "*")
		s.Reply(info, b.String())
	})
}

// ── .RHYME ──────────────────────────────────────────────────────────────────

func rhymeGuide(prefix string) string {
	return "*🔰 RHYME FINDER 🔰*\n\n" +
		"*FIND WORDS THAT RHYME WITH ANY WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RHYME <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RHYME LOVE ❯*"
}

func handleRhyme(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, rhymeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING RHYMES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word string `json:"word"`
		}
		u := "https://api.datamuse.com/words?max=15&rel_rhy=" + url.QueryEscape(word)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO RHYMES FOUND, PLEASE TRY ANOTHER WORD*")
			}
			return
		}
		var words []string
		for _, r := range res {
			words = append(words, r.Word)
		}
		var b strings.Builder
		b.WriteString("*🔰 RHYMES FOR " + strings.ToUpper(word) + " 🔰*\n\n")
		b.WriteString("*📝 " + strings.Join(words, ", ") + "*")
		s.Reply(info, b.String())
	})
}

// ── .RELATED ────────────────────────────────────────────────────────────────

func relatedGuide(prefix string) string {
	return "*🔰 RELATED WORDS 🔰*\n\n" +
		"*FIND WORDS RELATED TO ANY WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RELATED <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RELATED HAPPY ❯*"
}

func handleRelated(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, relatedGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING RELATED WORDS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word string `json:"word"`
		}
		u := "https://api.datamuse.com/words?max=15&ml=" + url.QueryEscape(word)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO RELATED WORDS FOUND, PLEASE TRY ANOTHER WORD*")
			}
			return
		}
		var words []string
		for _, r := range res {
			words = append(words, r.Word)
		}
		var b strings.Builder
		b.WriteString("*🔰 WORDS RELATED TO " + strings.ToUpper(word) + " 🔰*\n\n")
		b.WriteString("*📝 " + strings.Join(words, ", ") + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "dog", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM DOG PICTURE. USE IT AS .DOG.", Run: handleDog})
	Register(Command{Name: "fox", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM FOX PICTURE. USE IT AS .FOX.", Run: handleFox})
	Register(Command{Name: "duck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM DUCK PICTURE. USE IT AS .DUCK.", Run: handleDuck})
	Register(Command{Name: "coffee", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM COFFEE PICTURE. USE IT AS .COFFEE.", Run: handleCoffee})
	Register(Command{Name: "cat", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM CAT PICTURE. USE IT AS .CAT.", Run: handleCat})
	Register(Command{Name: "puppy", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM PUPPY PICTURE. USE IT AS .PUPPY.", Run: handlePuppy})
	Register(Command{Name: "bored", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM ACTIVITY IDEA WHEN YOU ARE BORED. USE IT AS .BORED.", Run: handleBored})
	Register(Command{Name: "zen", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM INSPIRATIONAL QUOTE. USE IT AS .ZEN.", Run: handleZen})
	Register(Command{Name: "rhyme", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND WORDS THAT RHYME WITH ANY WORD. USE IT AS .RHYME <WORD>.", Run: handleRhyme})
	Register(Command{Name: "related", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND WORDS RELATED TO ANY WORD. USE IT AS .RELATED <WORD>.", Run: handleRelated})

	// hidden aliases
	Register(Command{Name: "dogpic", Category: "TOOLS", Desc: "Short alias of .dog", Hidden: true, Run: handleDog})
	Register(Command{Name: "foxpic", Category: "TOOLS", Desc: "Short alias of .fox", Hidden: true, Run: handleFox})
	Register(Command{Name: "duckpic", Category: "TOOLS", Desc: "Short alias of .duck", Hidden: true, Run: handleDuck})
	Register(Command{Name: "catpic", Category: "TOOLS", Desc: "Short alias of .cat", Hidden: true, Run: handleCat})
	Register(Command{Name: "doggo", Category: "TOOLS", Desc: "Short alias of .puppy", Hidden: true, Run: handlePuppy})
	Register(Command{Name: "rhymes", Category: "TOOLS", Desc: "Short alias of .rhyme", Hidden: true, Run: handleRhyme})
}
