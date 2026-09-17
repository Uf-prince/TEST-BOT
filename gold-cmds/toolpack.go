package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK (10 high-value everyday commands)
// File: toolpack.go
// ============================================================================
// A bundle of 10 genuinely useful, always-working TOOLS commands. Every one
// uses a FREE public API (no key) and matches the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS) so it blends into the bot seamlessly.
//
//   .prayer <city>          -> daily prayer (namaz) times + hijri date
//   .quran                  -> random Quran ayah (Arabic + English)
//   .dictionary <word>      -> English word meaning + example
//   .currency <amt> <from> <to> -> live currency conversion
//   .timezone <city/zone>   -> current date & time in any timezone
//   .news [category]        -> top news headlines
//   .lyrics <artist> - <song> -> song lyrics
//   .github <username>      -> GitHub user profile
//   .anime <title>          -> anime info (rating, episodes, synopsis)
//   .pokemon <name>         -> Pokemon stats & info
//
// All commands run inside RunWithTimeout (which carries the per-session /
// per-user latest-wins guard), so a newer request instantly cancels an older
// one — same behaviour as every other GOLD-MD command.
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// .PRAYER — DAILY PRAYER (NAMAZ) TIMES
// ============================================================================

func prayerGuide(prefix string) string {
	return "*🔰 PRAYER TIMES 🔰*\n\n" +
		"*GET TODAY'S NAMAZ TIMES OF ANY CITY*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "PRAYER <CITY> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "PRAYER LAHORE ❯*"
}

func handlePrayer(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		city := strings.TrimSpace(strings.Join(args, " "))
		if city == "" {
			s.Reply(info, prayerGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PRAYER TIMES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.aladhan.com/v1/timingsByCity?city=" + url.QueryEscape(city) + "&country=&method=2"
		var res struct {
			Code int `json:"code"`
			Data struct {
				Timings struct {
					Fajr    string `json:"Fajr"`
					Sunrise string `json:"Sunrise"`
					Dhuhr   string `json:"Dhuhr"`
					Asr     string `json:"Asr"`
					Maghrib string `json:"Maghrib"`
					Isha    string `json:"Isha"`
				} `json:"timings"`
				Date struct {
					Readable string `json:"readable"`
					Hijri    struct {
						Date string `json:"date"`
					} `json:"hijri"`
				} `json:"date"`
				Meta struct {
					Timezone string `json:"timezone"`
				} `json:"meta"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 CITY NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		t := res.Data.Timings
		var b strings.Builder
		b.WriteString("*🔰 PRAYER TIMES 🔰*\n\n")
		b.WriteString("*📍 CITY ❯ " + strings.ToUpper(city) + "*\n")
		b.WriteString("*📅 DATE ❯ " + res.Data.Date.Readable + "*\n")
		b.WriteString("*🌙 HIJRI ❯ " + res.Data.Date.Hijri.Date + "*\n")
		b.WriteString("*🕐 ZONE ❯ " + res.Data.Meta.Timezone + "*\n\n")
		b.WriteString("*🕌 FAJR ❯ " + t.Fajr + "*\n")
		b.WriteString("*🌅 SUNRISE ❯ " + t.Sunrise + "*\n")
		b.WriteString("*☀️ DHUHR ❯ " + t.Dhuhr + "*\n")
		b.WriteString("*🌤️ ASR ❯ " + t.Asr + "*\n")
		b.WriteString("*🌇 MAGHRIB ❯ " + t.Maghrib + "*\n")
		b.WriteString("*🌃 ISHA ❯ " + t.Isha + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .QURAN — RANDOM QURAN AYAH
// ============================================================================

func handleQuran(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QURAN AYAH....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.alquran.cloud/v1/ayah/random/editions/quran-uthmani,en.asad"
		var res struct {
			Code int `json:"code"`
			Data []struct {
				Text   string `json:"text"`
				Number int    `json:"numberInSurah"`
				Surah  struct {
					Number int    `json:"number"`
					Name   string `json:"name"`
					EnName string `json:"englishName"`
				} `json:"surah"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 || len(res.Data) < 2 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "QURAN AYAH")
			}
			return
		}
		ar := res.Data[0]
		en := res.Data[1]
		var b strings.Builder
		b.WriteString("*🔰 HOLY QURAN 🔰*\n\n")
		b.WriteString("*📖 SURAH ❯ " + strings.ToUpper(ar.Surah.EnName) + " (" + ar.Surah.Name + ")*\n")
		b.WriteString("*🔢 AYAH ❯ " + strconv.Itoa(ar.Number) + "*\n\n")
		b.WriteString(ar.Text + "\n\n")
		b.WriteString("*" + en.Text + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .DICTIONARY — ENGLISH WORD MEANING
// ============================================================================

func dictionaryGuide(prefix string) string {
	return "*🔰 DICTIONARY 🔰*\n\n" +
		"*GET THE MEANING OF ANY ENGLISH WORD*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "DICTIONARY <WORD> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "DICTIONARY HELLO ❯*"
}

func handleDictionary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, dictionaryGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING DICTIONARY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(word)
		var res []struct {
			Word     string `json:"word"`
			Phonetic string `json:"phonetic"`
			Meanings []struct {
				PartOfSpeech string `json:"partOfSpeech"`
				Definitions  []struct {
					Definition string `json:"definition"`
					Example    string `json:"example"`
				} `json:"definitions"`
			} `json:"meanings"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 WORD NOT FOUND, PLEASE CHECK THE SPELLING*")
			}
			return
		}
		e := res[0]
		var b strings.Builder
		b.WriteString("*🔰 DICTIONARY 🔰*\n\n")
		b.WriteString("*📖 WORD ❯ " + strings.ToUpper(e.Word) + "*\n")
		if e.Phonetic != "" {
			b.WriteString("*🔊 " + e.Phonetic + "*\n")
		}
		count := 0
		for _, m := range e.Meanings {
			if len(m.Definitions) == 0 {
				continue
			}
			b.WriteString("\n*▪️ " + strings.ToUpper(m.PartOfSpeech) + "*\n")
			b.WriteString("*" + m.Definitions[0].Definition + "*\n")
			if m.Definitions[0].Example != "" {
				b.WriteString("_EXAMPLE: " + m.Definitions[0].Example + "_\n")
			}
			count++
			if count >= 3 {
				break
			}
		}
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .CURRENCY — LIVE CURRENCY CONVERSION
// ============================================================================

func currencyGuide(prefix string) string {
	return "*🔰 CURRENCY CONVERTER 🔰*\n\n" +
		"*CONVERT ANY CURRENCY TO ANY CURRENCY*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "CURRENCY <AMOUNT> <FROM> <TO> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "CURRENCY 100 USD PKR ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "CURRENCY 5000 PKR USD ❯*"
}

func handleCurrency(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, currencyGuide(prefix))
			return
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, currencyGuide(prefix))
			return
		}
		from := strings.ToUpper(strings.TrimSpace(args[1]))
		to := strings.ToUpper(strings.TrimSpace(args[2]))
		waitID := s.ReplyWithID(info, "*CONVERTING CURRENCY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://open.er-api.com/v6/latest/" + url.PathEscape(from)
		var res struct {
			Result   string             `json:"result"`
			BaseCode string             `json:"base_code"`
			Rates    map[string]float64 `json:"rates"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Result != "success" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID CURRENCY CODE, PLEASE CHECK*")
			}
			return
		}
		rate, ok := res.Rates[to]
		if !ok {
			s.Reply(info, "*🔰 TARGET CURRENCY NOT FOUND*")
			return
		}
		converted := amount * rate
		var b strings.Builder
		b.WriteString("*🔰 CURRENCY CONVERTER 🔰*\n\n")
		b.WriteString("*💵 FROM ❯ " + fmt.Sprintf("%.2f", amount) + " " + from + "*\n")
		b.WriteString("*💱 RATE ❯ 1 " + from + " = " + fmt.Sprintf("%.4f", rate) + " " + to + "*\n")
		b.WriteString("*✅ RESULT ❯ " + fmt.Sprintf("%.2f", converted) + " " + to + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .TIMEZONE — CURRENT TIME IN ANY TIMEZONE
// ============================================================================

func timezoneGuide(prefix string) string {
	return "*🔰 WORLD CLOCK 🔰*\n\n" +
		"*GET THE CURRENT TIME IN ANY TIMEZONE*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "TIMEZONE <ZONE> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "TIMEZONE ASIA/KARACHI ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "TIMEZONE AMERICA/NEW_YORK ❯*"
}

func handleTimezone(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		zone := strings.TrimSpace(strings.Join(args, " "))
		if zone == "" {
			s.Reply(info, timezoneGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CHECKING TIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://timeapi.io/api/Time/current/zone?timeZone=" + url.QueryEscape(zone)
		var res struct {
			Year      int    `json:"year"`
			Month     int    `json:"month"`
			Day       int    `json:"day"`
			Hour      int    `json:"hour"`
			Minute    int    `json:"minute"`
			Seconds   int    `json:"seconds"`
			DayOfWeek string `json:"dayOfWeek"`
			TimeZone  string `json:"timeZone"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.TimeZone == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID TIMEZONE, PLEASE CHECK*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 WORLD CLOCK 🔰*\n\n")
		b.WriteString("*🌍 ZONE ❯ " + strings.ToUpper(res.TimeZone) + "*\n")
		b.WriteString("*📅 DATE ❯ " + fmt.Sprintf("%02d-%02d-%d", res.Day, res.Month, res.Year) + "*\n")
		b.WriteString("*📆 DAY ❯ " + strings.ToUpper(res.DayOfWeek) + "*\n")
		b.WriteString("*🕐 TIME ❯ " + fmt.Sprintf("%02d:%02d:%02d", res.Hour, res.Minute, res.Seconds) + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .NEWS — TOP NEWS HEADLINES
// ============================================================================

func newsGuide(prefix string) string {
	return "*🔰 NEWS HEADLINES 🔰*\n\n" +
		"*GET THE LATEST TOP NEWS HEADLINES*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "NEWS ❯*  (general news)\\n" +
		"*❮ " + prefix + "NEWS <CATEGORY> ❯*\\n" +
		"*CATEGORIES ❯ BUSINESS, TECHNOLOGY, SPORTS, SCIENCE, HEALTH, ENTERTAINMENT*\\n" +
		"*EXAMPLE ❮ " + prefix + "NEWS SPORTS ❯*"
}

func handleNews(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		cat := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		if cat == "" {
			cat = "general"
		}
		valid := map[string]bool{
			"general": true, "business": true, "technology": true, "sports": true,
			"science": true, "health": true, "entertainment": true,
		}
		if !valid[cat] {
			s.Reply(info, newsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING NEWS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://saurav.tech/NewsAPI/top-headlines/category/" + cat + "/in.json"
		var res struct {
			Status   string `json:"status"`
			Articles []struct {
				Title  string `json:"title"`
				Source struct {
					Name string `json:"name"`
				} `json:"source"`
				URL string `json:"url"`
			} `json:"articles"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Status != "ok" || len(res.Articles) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "NEWS")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NEWS HEADLINES 🔰*\n\n")
		b.WriteString("*📰 CATEGORY ❯ " + strings.ToUpper(cat) + "*\n\n")
		n := 0
		for _, a := range res.Articles {
			if strings.TrimSpace(a.Title) == "" || a.Title == "[Removed]" {
				continue
			}
			n++
			b.WriteString("*" + strconv.Itoa(n) + ". " + a.Title + "*\n")
			if a.Source.Name != "" {
				b.WriteString("_— " + a.Source.Name + "_\n")
			}
			if a.URL != "" {
				b.WriteString(a.URL + "\n")
			}
			b.WriteString("\n")
			if n >= 8 {
				break
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ============================================================================
// .LYRICS — SONG LYRICS
// ============================================================================

func lyricsGuide(prefix string) string {
	return "*🔰 SONG LYRICS 🔰*\n\n" +
		"*GET THE FULL LYRICS OF ANY SONG*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "LYRICS <ARTIST> - <SONG> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "LYRICS COLDPLAY - YELLOW ❯*"
}

func handleLyrics(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(strings.Join(args, " "))
		if raw == "" {
			s.Reply(info, lyricsGuide(prefix))
			return
		}
		parts := strings.SplitN(raw, "-", 2)
		if len(parts) < 2 {
			s.Reply(info, lyricsGuide(prefix))
			return
		}
		artist := strings.TrimSpace(parts[0])
		song := strings.TrimSpace(parts[1])
		if artist == "" || song == "" {
			s.Reply(info, lyricsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING LYRICS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.lyrics.ovh/v1/" + url.PathEscape(artist) + "/" + url.PathEscape(song)
		var res struct {
			Lyrics string `json:"lyrics"`
			Error  string `json:"error"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || strings.TrimSpace(res.Lyrics) == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 LYRICS NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		lyr := strings.TrimSpace(res.Lyrics)
		if len(lyr) > 3500 {
			lyr = lyr[:3500] + "\n\n...(TRUNCATED)"
		}
		var b strings.Builder
		b.WriteString("*🔰 SONG LYRICS 🔰*\n\n")
		b.WriteString("*🎵 " + strings.ToUpper(song) + "*\n")
		b.WriteString("*🎤 " + strings.ToUpper(artist) + "*\n\n")
		b.WriteString(lyr)
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .GITHUB — GITHUB USER PROFILE
// ============================================================================

func githubGuide(prefix string) string {
	return "*🔰 GITHUB LOOKUP 🔰*\n\n" +
		"*GET THE PROFILE OF ANY GITHUB USER*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "GITHUB <USERNAME> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "GITHUB TORVALDS ❯*"
}

func handleGithub(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		user := strings.TrimSpace(strings.Join(args, " "))
		if user == "" {
			s.Reply(info, githubGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING GITHUB PROFILE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.github.com/users/" + url.PathEscape(user)
		var res struct {
			Login       string `json:"login"`
			Name        string `json:"name"`
			Bio         string `json:"bio"`
			Company     string `json:"company"`
			Location    string `json:"location"`
			Blog        string `json:"blog"`
			PublicRepos int    `json:"public_repos"`
			Followers   int    `json:"followers"`
			Following   int    `json:"following"`
			HTMLURL     string `json:"html_url"`
			AvatarURL   string `json:"avatar_url"`
			Message     string `json:"message"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Login == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 GITHUB USER NOT FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 GITHUB PROFILE 🔰*\n\n")
		b.WriteString("*👤 USER ❯ " + res.Login + "*\n")
		if res.Name != "" {
			b.WriteString("*📛 NAME ❯ " + res.Name + "*\n")
		}
		if res.Bio != "" {
			b.WriteString("*📝 BIO ❯ " + res.Bio + "*\n")
		}
		if res.Company != "" {
			b.WriteString("*🏢 COMPANY ❯ " + res.Company + "*\n")
		}
		if res.Location != "" {
			b.WriteString("*📍 LOCATION ❯ " + res.Location + "*\n")
		}
		b.WriteString("*📦 REPOS ❯ " + strconv.Itoa(res.PublicRepos) + "*\n")
		b.WriteString("*👥 FOLLOWERS ❯ " + strconv.Itoa(res.Followers) + "*\n")
		b.WriteString("*➡️ FOLLOWING ❯ " + strconv.Itoa(res.Following) + "*\n")
		b.WriteString("*🔗 " + res.HTMLURL + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .ANIME — ANIME INFO
// ============================================================================

func animeGuide(prefix string) string {
	return "*🔰 ANIME INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY ANIME*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "ANIME <TITLE> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "ANIME NARUTO ❯*"
}

func handleAnime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, animeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING ANIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.jikan.moe/v4/anime?q=" + url.QueryEscape(q) + "&limit=1"
		var res struct {
			Data []struct {
				Title    string  `json:"title"`
				Synopsis string  `json:"synopsis"`
				Episodes int     `json:"episodes"`
				Score    float64 `json:"score"`
				Status   string  `json:"status"`
				Type     string  `json:"type"`
				URL      string  `json:"url"`
				Aired    struct {
					String string `json:"string"`
				} `json:"aired"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 ANIME NOT FOUND, PLEASE CHECK THE TITLE*")
			}
			return
		}
		a := res.Data[0]
		syn := strings.TrimSpace(a.Synopsis)
		if len(syn) > 900 {
			syn = syn[:900] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 ANIME INFO 🔰*\n\n")
		b.WriteString("*🎬 TITLE ❯ " + strings.ToUpper(a.Title) + "*\n")
		if a.Type != "" {
			b.WriteString("*📺 TYPE ❯ " + a.Type + "*\n")
		}
		if a.Episodes > 0 {
			b.WriteString("*🔢 EPISODES ❯ " + strconv.Itoa(a.Episodes) + "*\n")
		}
		if a.Score > 0 {
			b.WriteString("*⭐ SCORE ❯ " + fmt.Sprintf("%.2f", a.Score) + "/10*\n")
		}
		if a.Status != "" {
			b.WriteString("*📡 STATUS ❯ " + strings.ToUpper(a.Status) + "*\n")
		}
		if a.Aired.String != "" {
			b.WriteString("*📅 AIRED ❯ " + a.Aired.String + "*\n")
		}
		if syn != "" {
			b.WriteString("\n*📖 SYNOPSIS:*\n" + syn + "\n")
		}
		if a.URL != "" {
			b.WriteString("\n*🔗 " + a.URL + "*")
		}
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .POKEMON — POKEMON INFO
// ============================================================================

func pokemonGuide(prefix string) string {
	return "*🔰 POKEDEX 🔰*\n\n" +
		"*GET DETAILS OF ANY POKEMON*\\n\\n" +
		"*HOW TO USE:*\\n" +
		"*❮ " + prefix + "POKEMON <NAME> ❯*\\n" +
		"*EXAMPLE ❮ " + prefix + "POKEMON PIKACHU ❯*"
}

func handlePokemon(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		if name == "" {
			s.Reply(info, pokemonGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING POKEDEX....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://pokeapi.co/api/v2/pokemon/" + url.PathEscape(name)
		var res struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Height  int    `json:"height"`
			Weight  int    `json:"weight"`
			Types   []struct {
				Type struct {
					Name string `json:"name"`
				} `json:"type"`
			} `json:"types"`
			Abilities []struct {
				Ability struct {
					Name string `json:"name"`
				} `json:"ability"`
			} `json:"abilities"`
			Stats []struct {
				BaseStat int `json:"base_stat"`
				Stat     struct {
					Name string `json:"name"`
				} `json:"stat"`
			} `json:"stats"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 POKEMON NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 POKEDEX 🔰*\n\n")
		b.WriteString("*🎮 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*🔢 ID ❯ #" + strconv.Itoa(res.ID) + "*\n")
		b.WriteString("*📏 HEIGHT ❯ " + fmt.Sprintf("%.1f", float64(res.Height)/10) + " M*\n")
		b.WriteString("*⚖️ WEIGHT ❯ " + fmt.Sprintf("%.1f", float64(res.Weight)/10) + " KG*\n")
		if len(res.Types) > 0 {
			ts := make([]string, 0, len(res.Types))
			for _, t := range res.Types {
				ts = append(ts, strings.ToUpper(t.Type.Name))
			}
			b.WriteString("*🧬 TYPE ❯ " + strings.Join(ts, ", ") + "*\n")
		}
		if len(res.Abilities) > 0 {
			as := make([]string, 0, len(res.Abilities))
			for _, a := range res.Abilities {
				as = append(as, strings.ToUpper(a.Ability.Name))
			}
			b.WriteString("*✨ ABILITIES ❯ " + strings.Join(as, ", ") + "*\n")
		}
		if len(res.Stats) > 0 {
			b.WriteString("\n*📊 BASE STATS:*\n")
			for _, st := range res.Stats {
				b.WriteString("*▪️ " + strings.ToUpper(st.Stat.Name) + " ❯ " + strconv.Itoa(st.BaseStat) + "*\n")
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ============================================================================
// REGISTRATION
// ============================================================================

func init() {
	Register(Command{Name: "prayer", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TODAY'S PRAYER (NAMAZ) TIMES OF ANY CITY. USE IT AS .PRAYER <CITY>.", Run: handlePrayer})
	Register(Command{Name: "quran", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM QURAN AYAH WITH ARABIC AND ENGLISH TRANSLATION. JUST TYPE .QURAN.", Run: handleQuran})
	Register(Command{Name: "dictionary", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE MEANING OF ANY ENGLISH WORD. USE IT AS .DICTIONARY <WORD>.", Run: handleDictionary})
	Register(Command{Name: "currency", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT ANY CURRENCY TO ANY CURRENCY. USE IT AS .CURRENCY <AMOUNT> <FROM> <TO>.", Run: handleCurrency})
	Register(Command{Name: "timezone", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE CURRENT TIME IN ANY TIMEZONE. USE IT AS .TIMEZONE <ZONE>.", Run: handleTimezone})
	Register(Command{Name: "news", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LATEST TOP NEWS HEADLINES. USE IT AS .NEWS <CATEGORY>.", Run: handleNews})
	Register(Command{Name: "lyrics", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE FULL LYRICS OF ANY SONG. USE IT AS .LYRICS <ARTIST> - <SONG>.", Run: handleLyrics})
	Register(Command{Name: "github", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE PROFILE OF ANY GITHUB USER. USE IT AS .GITHUB <USERNAME>.", Run: handleGithub})
	Register(Command{Name: "anime", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DETAILS OF ANY ANIME. USE IT AS .ANIME <TITLE>.", Run: handleAnime})
	Register(Command{Name: "pokemon", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DETAILS OF ANY POKEMON. USE IT AS .POKEMON <NAME>.", Run: handlePokemon})

	// ── short aliases (hidden — menu me sirf long form dikhta hai, same work) ──
	Register(Command{Name: "namaz", Category: "TOOLS", Desc: "Short alias of .prayer", Hidden: true, Run: handlePrayer})
	Register(Command{Name: "salah", Category: "TOOLS", Desc: "Short alias of .prayer", Hidden: true, Run: handlePrayer})
	Register(Command{Name: "ayah", Category: "TOOLS", Desc: "Short alias of .quran", Hidden: true, Run: handleQuran})
	Register(Command{Name: "dict", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "meaning", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "define", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "rate", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "exchange", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "forex", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "worldtime", Category: "TOOLS", Desc: "Short alias of .timezone", Hidden: true, Run: handleTimezone})
	Register(Command{Name: "headlines", Category: "TOOLS", Desc: "Short alias of .news", Hidden: true, Run: handleNews})
	Register(Command{Name: "lyric", Category: "TOOLS", Desc: "Short alias of .lyrics", Hidden: true, Run: handleLyrics})
	Register(Command{Name: "ghuser", Category: "TOOLS", Desc: "Short alias of .github", Hidden: true, Run: handleGithub})
	Register(Command{Name: "gituser", Category: "TOOLS", Desc: "Short alias of .github", Hidden: true, Run: handleGithub})
	Register(Command{Name: "animeinfo", Category: "TOOLS", Desc: "Short alias of .anime", Hidden: true, Run: handleAnime})
	Register(Command{Name: "pokedex", Category: "TOOLS", Desc: "Short alias of .pokemon", Hidden: true, Run: handlePokemon})
	Register(Command{Name: "poke", Category: "TOOLS", Desc: "Short alias of .pokemon", Hidden: true, Run: handlePokemon})
}
