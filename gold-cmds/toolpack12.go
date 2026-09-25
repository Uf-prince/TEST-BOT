package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 12 (10 new everyday commands)
// File: toolpack12.go
// ============================================================================
//   .chuck <query>       -> random Chuck Norris joke
//   .dadjoke             -> random dad joke
//   .disney <name>       -> Disney character information
//   .meme                -> random meme image
//   .animequote          -> random anime quote
//   .poetry <title>      -> poem by title
//   .mealdb              -> random meal recipe
//   .uselessfact         -> random useless fact
//   .ipgeo               -> geolocation of your IP
//   .seeip               -> your public IP address
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .CHUCK ───────────────────────────────────────────────────────────────────

func chuckGuide(prefix string) string {
	return "*🔰 CHUCK NORRIS 🔰*\n\n" +
		"*GET A RANDOM CHUCK NORRIS JOKE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CHUCK ❯*"
}

func handleChuck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING JOKE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Value string `json:"value"`
		}
		if err := funGetJSON(ctx, "https://api.chucknorris.io/jokes/random", &res); err != nil || res.Value == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "CHUCK")
			}
			return
		}
		s.Reply(info, "*🔰 CHUCK NORRIS 🔰*\n\n*"+res.Value+"*")
	})
}

// ── .DADJOKE ─────────────────────────────────────────────────────────────────

func dadjokeGuide(prefix string) string {
	return "🔰 DAD JOKE 🔰\n\n" +
		"*GET A RANDOM DAD JOKE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DADJOKE ❯*"
}

func handleDadjoke(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING JOKE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Joke struct {
				Opener    string `json:"Opener"`
				Punchline string `json:"Punchline"`
			} `json:"Joke"`
		}
		if err := funGetJSON(ctx, "https://dadjokes.online/api/random", &res); err != nil || res.Joke.Opener == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DADJOKE")
			}
			return
		}
		var b strings.Builder
		b.WriteString("🔰 DAD JOKE 🔰\n\n")
		b.WriteString(res.Joke.Opener + "\n\n")
		if res.Joke.Punchline != "" {
			b.WriteString("😂 " + res.Joke.Punchline)
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .DISNEY ──────────────────────────────────────────────────────────────────

func disneyGuide(prefix string) string {
	return "*🔰 DISNEY CHARACTER 🔰*\n\n" +
		"*GET INFORMATION ABOUT ANY DISNEY CHARACTER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DISNEY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DISNEY MICKEY MOUSE ❯*"
}

func handleDisney(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, disneyGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING CHARACTER....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data []struct {
				Name       string   `json:"name"`
				Films      []string `json:"films"`
				TVShows    []string `json:"tvShows"`
				VideoGames []string `json:"videoGames"`
				ImageURL   string   `json:"imageUrl"`
			} `json:"data"`
		}
		u := "https://api.disneyapi.dev/character?name=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO DISNEY CHARACTER FOUND, PLEASE TRY ANOTHER NAME*")
			}
			return
		}
		c := res.Data[0]
		var b strings.Builder
		b.WriteString("*🔰 DISNEY CHARACTER 🔰*\n\n")
		b.WriteString("*🎭 NAME ❯ " + strings.ToUpper(c.Name) + "*\n")
		if len(c.Films) > 0 {
			b.WriteString("*🎬 FILMS ❯ " + strings.ToUpper(strings.Join(c.Films, ", ")) + "*\n")
		}
		if len(c.TVShows) > 0 {
			b.WriteString("*📺 SHOWS ❯ " + strings.ToUpper(strings.Join(c.TVShows, ", ")) + "*\n")
		}
		if len(c.VideoGames) > 0 {
			b.WriteString("*🎮 GAMES ❯ " + strings.ToUpper(strings.Join(c.VideoGames, ", ")) + "*\n")
		}
		if c.ImageURL != "" {
			if data, err := funGetBytes(ctx, c.ImageURL); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .MEME ────────────────────────────────────────────────────────────────────

func memeGuide(prefix string) string {
	return "*🔰 RANDOM MEME 🔰*\n\n" +
		"*GET A RANDOM MEME IMAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MEME ❯*"
}

func handleMeme(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING MEME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			Subreddit string `json:"subreddit"`
			Author    string `json:"author"`
			Ups       int    `json:"ups"`
			NSFW      bool   `json:"nsfw"`
		}
		if err := funGetJSON(ctx, "https://meme-api.com/gimme", &res); err != nil || res.URL == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "MEME")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 RANDOM MEME 🔰*\n\n")
		if res.Title != "" {
			b.WriteString("*📝 " + res.Title + "*\n")
		}
		if res.Subreddit != "" {
			b.WriteString("*🌐 R/" + strings.ToUpper(res.Subreddit) + "*\n")
		}
		if res.Author != "" {
			b.WriteString("*👤 U/" + res.Author + "*\n")
		}
		b.WriteString("*⬆️ " + strconv.Itoa(res.Ups) + " UPVOTES*")
		data, err := funGetBytes(ctx, res.URL)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "MEME")
			}
			return
		}
		_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
	})
}

// ── .ANIMEQUOTE ──────────────────────────────────────────────────────────────

func animequoteGuide(prefix string) string {
	return "*🔰 ANIME QUOTE 🔰*\n\n" +
		"*GET A RANDOM ANIME QUOTE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ANIMEQUOTE ❯*"
}

func handleAnimequote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QUOTE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Status string `json:"status"`
			Data   struct {
				Content string `json:"content"`
				Anime   struct {
					Name string `json:"name"`
				} `json:"anime"`
				Character struct {
					Name string `json:"name"`
				} `json:"character"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://api.animechan.io/v1/quotes/random", &res); err != nil || res.Data.Content == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "ANIMEQUOTE")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 ANIME QUOTE 🔰*\n\n")
		b.WriteString("*\"" + res.Data.Content + "\"*\n\n")
		if res.Data.Character.Name != "" {
			b.WriteString("*👤 " + strings.ToUpper(res.Data.Character.Name) + "*\n")
		}
		if res.Data.Anime.Name != "" {
			b.WriteString("*🎌 " + strings.ToUpper(res.Data.Anime.Name) + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .POETRY ──────────────────────────────────────────────────────────────────

func poetryGuide(prefix string) string {
	return "*🔰 POETRY 🔰*\n\n" +
		"*GET ANY POEM BY ITS TITLE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "POETRY <TITLE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "POETRY THE RAVEN ❯*"
}

func handlePoetry(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, poetryGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING POEM....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Title  string   `json:"title"`
			Author string   `json:"author"`
			Lines  []string `json:"lines"`
		}
		u := "https://poetrydb.org/title/" + strings.ReplaceAll(url.QueryEscape(q), "+", "%20")
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO POEM FOUND, PLEASE TRY ANOTHER TITLE*")
			}
			return
		}
		p := res[0]
		var b strings.Builder
		b.WriteString("*🔰 POETRY 🔰*\n\n")
		b.WriteString("*📖 TITLE ❯ " + strings.ToUpper(p.Title) + "*\n")
		if p.Author != "" {
			b.WriteString("*✍️ AUTHOR ❯ " + strings.ToUpper(p.Author) + "*\n")
		}
		lines := p.Lines
		if len(lines) > 20 {
			lines = lines[:20]
		}
		b.WriteString("\n*" + strings.Join(lines, "\n") + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .MEALDB ──────────────────────────────────────────────────────────────────

func mealdbGuide(prefix string) string {
	return "*🔰 RANDOM MEAL 🔰*\n\n" +
		"*GET A RANDOM MEAL RECIPE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MEALDB ❯*"
}

func handleMealdb(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING RECIPE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Meals []map[string]any `json:"meals"`
		}
		if err := funGetJSON(ctx, "https://www.themealdb.com/api/json/v1/1/random.php", &res); err != nil || len(res.Meals) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "MEALDB")
			}
			return
		}
		m := res.Meals[0]
		str := func(k string) string {
			if v, ok := m[k].(string); ok {
				return strings.TrimSpace(v)
			}
			return ""
		}
		var b strings.Builder
		b.WriteString("*🔰 RANDOM MEAL 🔰*\n\n")
		b.WriteString("*🍽️ NAME ❯ " + strings.ToUpper(str("strMeal")) + "*\n")
		if c := str("strCategory"); c != "" {
			b.WriteString("*🏷️ CATEGORY ❯ " + strings.ToUpper(c) + "*\n")
		}
		if a := str("strArea"); a != "" {
			b.WriteString("*🌍 CUISINE ❯ " + strings.ToUpper(a) + "*\n")
		}
		var ings []string
		for i := 1; i <= 20; i++ {
			ing := str("strIngredient" + strconv.Itoa(i))
			mea := str("strMeasure" + strconv.Itoa(i))
			if ing == "" {
				continue
			}
			if mea != "" {
				ings = append(ings, strings.TrimSpace(mea+" "+ing))
			} else {
				ings = append(ings, ing)
			}
		}
		if len(ings) > 0 {
			b.WriteString("\n*🧂 INGREDIENTS:*\n*" + strings.Join(ings, "\n") + "*\n")
		}
		ins := str("strInstructions")
		if len(ins) > 600 {
			ins = ins[:600] + "..."
		}
		if ins != "" {
			b.WriteString("\n*📖 INSTRUCTIONS:*\n*" + ins + "*")
		}
		if img := str("strMealThumb"); img != "" {
			if data, err := funGetBytes(ctx, img); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .USELESSFACT ─────────────────────────────────────────────────────────────

func uselessfactGuide(prefix string) string {
	return "*🔰 USELESS FACT 🔰*\n\n" +
		"*GET A RANDOM USELESS FACT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "USELESSFACT ❯*"
}

func handleUselessfact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING FACT....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Text   string `json:"text"`
			Source string `json:"source"`
		}
		if err := funGetJSON(ctx, "https://uselessfacts.jsph.pl/api/v2/facts/random", &res); err != nil || res.Text == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "USELESSFACT")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 USELESS FACT 🔰*\n\n")
		b.WriteString("*💡 " + res.Text + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .IPGEO ───────────────────────────────────────────────────────────────────

func ipgeoGuide(prefix string) string {
	return "*🔰 IP GEOLOCATION 🔰*\n\n" +
		"*GET THE GEOLOCATION OF YOUR IP ADDRESS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "IPGEO ❯*"
}

func handleIpgeo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*LOCATING IP....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			IP       string  `json:"ip"`
			City     string  `json:"city"`
			Region   string  `json:"region"`
			Country  string  `json:"country"`
			Company  string  `json:"company"`
			ASN      string  `json:"asn"`
			Timezone string  `json:"timezone"`
			Lat      float64 `json:"lat"`
			Lon      float64 `json:"lon"`
		}
		if err := funGetJSON(ctx, "https://api.ipapi.is", &res); err != nil || res.IP == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "IPGEO")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 IP GEOLOCATION 🔰*\n\n")
		b.WriteString("*🌐 IP ❯ " + res.IP + "*\n")
		if res.City != "" {
			b.WriteString("*🏙️ CITY ❯ " + strings.ToUpper(res.City) + "*\n")
		}
		if res.Region != "" {
			b.WriteString("*📍 REGION ❯ " + strings.ToUpper(res.Region) + "*\n")
		}
		if res.Country != "" {
			b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(res.Country) + "*\n")
		}
		if res.Company != "" {
			b.WriteString("*🏢 ISP ❯ " + strings.ToUpper(res.Company) + "*\n")
		}
		if res.ASN != "" {
			b.WriteString("*🔢 ASN ❯ " + strings.ToUpper(res.ASN) + "*\n")
		}
		if res.Timezone != "" {
			b.WriteString("*🕒 TIMEZONE ❯ " + res.Timezone + "*\n")
		}
		b.WriteString("*📌 COORDS ❯ " + fmt.Sprintf("%.4f, %.4f", res.Lat, res.Lon) + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .SEEIP ───────────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "chuck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM CHUCK NORRIS JOKE. USE IT AS .CHUCK.", Run: handleChuck})
	Register(Command{Name: "dadjoke", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM DAD JOKE. USE IT AS .DADJOKE.", Run: handleDadjoke})
	Register(Command{Name: "disney", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFORMATION ABOUT ANY DISNEY CHARACTER. USE IT AS .DISNEY <NAME>.", Run: handleDisney})
	Register(Command{Name: "meme", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM MEME IMAGE. USE IT AS .MEME.", Run: handleMeme})
	Register(Command{Name: "animequote", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM ANIME QUOTE. USE IT AS .ANIMEQUOTE.", Run: handleAnimequote})
	Register(Command{Name: "poetry", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET ANY POEM BY ITS TITLE. USE IT AS .POETRY <TITLE>.", Run: handlePoetry})
	Register(Command{Name: "mealdb", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM MEAL RECIPE. USE IT AS .MEALDB.", Run: handleMealdb})
	Register(Command{Name: "uselessfact", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM USELESS FACT. USE IT AS .USELESSFACT.", Run: handleUselessfact})
	Register(Command{Name: "ipgeo", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE GEOLOCATION OF YOUR IP ADDRESS. USE IT AS .IPGEO.", Run: handleIpgeo})

	// hidden aliases
	Register(Command{Name: "cnjoke", Category: "TOOLS", Desc: "Short alias of .chuck", Hidden: true, Run: handleChuck})
	Register(Command{Name: "dadjokes", Category: "TOOLS", Desc: "Short alias of .dadjoke", Hidden: true, Run: handleDadjoke})
	Register(Command{Name: "disneychar", Category: "TOOLS", Desc: "Short alias of .disney", Hidden: true, Run: handleDisney})
	Register(Command{Name: "randommeme", Category: "TOOLS", Desc: "Short alias of .meme", Hidden: true, Run: handleMeme})
	Register(Command{Name: "aniquote", Category: "TOOLS", Desc: "Short alias of .animequote", Hidden: true, Run: handleAnimequote})
	Register(Command{Name: "poem", Category: "TOOLS", Desc: "Short alias of .poetry", Hidden: true, Run: handlePoetry})
	Register(Command{Name: "mealrecipe", Category: "TOOLS", Desc: "Short alias of .mealdb", Hidden: true, Run: handleMealdb})
	Register(Command{Name: "funfact", Category: "TOOLS", Desc: "Short alias of .uselessfact", Hidden: true, Run: handleUselessfact})
	Register(Command{Name: "myipgeo", Category: "TOOLS", Desc: "Short alias of .ipgeo", Hidden: true, Run: handleIpgeo})
}
