package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 5 (15 everyday commands)
// File: toolpack5.go
// ============================================================================
//   .myip                   -> your public IP + location
//   .pincode <code>         -> India PIN code -> post offices
//   .hackernews             -> top Hacker News stories
//   .synonym <word>         -> synonyms of a word
//   .mac <mac>              -> MAC address vendor lookup
//   .sunrise <lat> <lng>    -> sunrise / sunset times
//   .apod                   -> NASA Astronomy Picture of the Day
//   .starwars <name>        -> Star Wars character info
//   .rickandmorty <name>    -> Rick & Morty character info
//   .chucknorris [cat]      -> random Chuck Norris joke
//   .kanye                  -> random Kanye West quote
//   .randomword [n]         -> random English words
//   .catfact                -> random cat fact
//   .drug <name>            -> FDA drug label info
//   .stackoverflow <query>  -> search Stack Overflow
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .PINCODE ─────────────────────────────────────────────────────────────────

func pincodeGuide(prefix string) string {
	return "*\U0001f530 PINCODE LOOKUP \U0001f530*\n\n" +
		"*FIND POST OFFICES BY PIN / POSTAL CODE (ANY COUNTRY)*\n\n" +
		"*HOW TO USE:*\n" +
		"*\u276e " + prefix + "PINCODE <CODE> \u276f*  (DEFAULT: INDIA)\n" +
		"*\u276e " + prefix + "PINCODE <COUNTRY> <CODE> \u276f*\n" +
		"*EXAMPLE \u276e " + prefix + "PINCODE PK 44000 \u276f*"
}

func handlePincode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, pincodeGuide(prefix))
			return
		}
		country := "in"
		code := ""
		if len(args) >= 2 {
			country = strings.ToLower(strings.TrimSpace(args[0]))
			code = strings.TrimSpace(strings.Join(args[1:], " "))
		} else {
			code = strings.TrimSpace(args[0])
		}
		if code == "" {
			s.Reply(info, pincodeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP PINCODE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		// Primary: zippopotam.us (covers many countries worldwide).
		var z struct {
			Country  string `json:"country"`
			PostCode string `json:"post code"`
			Places   []struct {
				PlaceName string `json:"place name"`
				State     string `json:"state"`
				Lat       string `json:"latitude"`
				Lon       string `json:"longitude"`
			} `json:"places"`
		}
		zu := "https://api.zippopotam.us/" + url.PathEscape(country) + "/" + url.PathEscape(code)
		if err := funGetJSONRetry(ctx, zu, &z, 2); err == nil && len(z.Places) > 0 {
			var b strings.Builder
			b.WriteString("*\U0001f530 PINCODE LOOKUP \U0001f530*\n\n")
			b.WriteString("*\U0001f4ee CODE \u276f " + z.PostCode + "*\n")
			b.WriteString("*\U0001f30d COUNTRY \u276f " + strings.ToUpper(z.Country) + "*\n\n")
			for i, pl := range z.Places {
				if i >= 8 {
					break
				}
				b.WriteString("*\U0001f4cd " + strings.ToUpper(pl.PlaceName) + "*\n")
				if pl.State != "" {
					b.WriteString("\u2022 STATE: " + pl.State + "\n")
				}
				if pl.Lat != "" && pl.Lon != "" {
					b.WriteString("\u2022 LAT/LON: " + pl.Lat + ", " + pl.Lon + "\n")
				}
				b.WriteString("\n")
			}
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}

		// Fallback: India Post API (only for India).
		if country == "in" || country == "india" {
			var res []struct {
				Status     string `json:"Status"`
				PostOffice []struct {
					Name     string `json:"Name"`
					District string `json:"District"`
					State    string `json:"State"`
				} `json:"PostOffice"`
			}
			iu := "https://api.postalpincode.in/pincode/" + url.PathEscape(code)
			if err := funGetJSONRetry(ctx, iu, &res, 2); err == nil && len(res) > 0 && res[0].Status == "Success" && len(res[0].PostOffice) > 0 {
				var b strings.Builder
				b.WriteString("*\U0001f530 PINCODE LOOKUP \U0001f530*\n\n")
				b.WriteString("*\U0001f4ee PIN \u276f " + code + "*\n")
				b.WriteString("*\U0001f4cd " + res[0].PostOffice[0].District + ", " + res[0].PostOffice[0].State + "*\n\n")
				for i, po := range res[0].PostOffice {
					if i >= 8 {
						break
					}
					b.WriteString("\u2022 " + po.Name + "\n")
				}
				s.Reply(info, strings.TrimSpace(b.String()))
				return
			}
		}

		if !ctxTimedOut(ctx) {
			s.Reply(info, "*\U0001f530 PINCODE NOT FOUND*")
		}
	})
}

// ── .HACKERNEWS ──────────────────────────────────────────────────────────────

func hackernewsGuide(prefix string) string {
	return "*🔰 HACKER NEWS 🔰*\n\n" +
		"*TOP STORIES FROM HACKER NEWS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HACKERNEWS ❯*"
}

func handleHackerNews(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING HACKER NEWS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var ids []int
		if err := funGetJSON(ctx, "https://hacker-news.firebaseio.com/v0/topstories.json", &ids); err != nil || len(ids) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH NEWS*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 HACKER NEWS — TOP STORIES 🔰*\n\n")
		n := 0
		for _, id := range ids {
			if n >= 6 {
				break
			}
			var item struct {
				Title string `json:"title"`
				URL   string `json:"url"`
				Score int    `json:"score"`
			}
			iu := "https://hacker-news.firebaseio.com/v0/item/" + strconv.Itoa(id) + ".json"
			if err := funGetJSON(ctx, iu, &item); err != nil || item.Title == "" {
				continue
			}
			n++
			b.WriteString("*" + strconv.Itoa(n) + ". " + item.Title + "*\n")
			b.WriteString("• ⬆️ " + strconv.Itoa(item.Score) + " points\n")
			if item.URL != "" {
				b.WriteString("• 🔗 " + item.URL + "\n")
			}
			b.WriteString("\n")
		}
		if n == 0 {
			s.Reply(info, "*🔰 COULD NOT FETCH NEWS*")
			return
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .SYNONYM ─────────────────────────────────────────────────────────────────

func synonymGuide(prefix string) string {
	return "*🔰 SYNONYMS 🔰*\n\n" +
		"*FIND SYNONYMS OF ANY WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SYNONYM <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SYNONYM happy ❯*"
}

func handleSynonym(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, synonymGuide(prefix))
			return
		}
		word := strings.TrimSpace(args[0])
		waitID := s.ReplyWithID(info, "*FINDING SYNONYMS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res []struct {
			Word string `json:"word"`
		}
		u := "https://api.datamuse.com/words?rel_syn=" + url.QueryEscape(word) + "&max=12"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO SYNONYMS FOUND*")
			}
			return
		}
		var words []string
		for _, r := range res {
			words = append(words, r.Word)
		}
		s.Reply(info, "*🔰 SYNONYMS OF "+strings.ToUpper(word)+" 🔰*\n\n*📖 "+strings.Join(words, ", ")+"*")
	})
}

// ── .MAC ─────────────────────────────────────────────────────────────────────

func macGuide(prefix string) string {
	return "*🔰 MAC VENDOR 🔰*\n\n" +
		"*FIND THE VENDOR OF ANY MAC ADDRESS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MAC <MAC> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MAC 44:38:39:ff:ef:57 ❯*"
}

func handleMac(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, macGuide(prefix))
			return
		}
		mac := strings.TrimSpace(args[0])
		waitID := s.ReplyWithID(info, "*LOOKING UP MAC....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		data, err := funGetBytes(ctx, "https://api.macvendors.com/"+url.PathEscape(mac))
		if err != nil || len(data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 VENDOR NOT FOUND*")
			}
			return
		}
		vendor := strings.TrimSpace(string(data))
		s.Reply(info, "*🔰 MAC VENDOR 🔰*\n\n*🖥️ MAC ❯ "+mac+"*\n*🏢 VENDOR ❯ "+vendor+"*")
	})
}

// ── .SUNRISE ─────────────────────────────────────────────────────────────────

func sunriseGuide(prefix string) string {
	return "*🔰 SUNRISE / SUNSET 🔰*\n\n" +
		"*GET SUNRISE + SUNSET TIMES FOR ANY LOCATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SUNRISE <LAT> <LNG> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SUNRISE 24.86 67.01 ❯*"
}

func handleSunrise(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, sunriseGuide(prefix))
			return
		}
		lat := strings.TrimSpace(args[0])
		lng := strings.TrimSpace(args[1])
		waitID := s.ReplyWithID(info, "*CALCULATING SUN TIMES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Results struct {
				Sunrise   string `json:"sunrise"`
				Sunset    string `json:"sunset"`
				SolarNoon string `json:"solar_noon"`
				DayLength int    `json:"day_length"`
			} `json:"results"`
			Status string `json:"status"`
		}
		u := "https://api.sunrise-sunset.org/json?lat=" + url.QueryEscape(lat) + "&lng=" + url.QueryEscape(lng) + "&formatted=0"
		if err := funGetJSON(ctx, u, &res); err != nil || res.Status != "OK" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT CALCULATE*")
			}
			return
		}
		fmtTime := func(t string) string {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				return parsed.UTC().Format("15:04 UTC")
			}
			return t
		}
		var b strings.Builder
		b.WriteString("*🔰 SUNRISE / SUNSET 🔰*\n\n")
		b.WriteString("*📍 COORDS ❯ " + lat + ", " + lng + "*\n")
		b.WriteString("*🌅 SUNRISE ❯ " + fmtTime(res.Results.Sunrise) + "*\n")
		b.WriteString("*🌇 SUNSET ❯ " + fmtTime(res.Results.Sunset) + "*\n")
		b.WriteString("*☀️ NOON ❯ " + fmtTime(res.Results.SolarNoon) + "*\n")
		b.WriteString("*⏳ DAY LENGTH ❯ " + strconv.Itoa(res.Results.DayLength/3600) + "h " +
			strconv.Itoa((res.Results.DayLength%3600)/60) + "m*")
		s.Reply(info, b.String())
	})
}

// ── .APOD ────────────────────────────────────────────────────────────────────

func apodGuide(prefix string) string {
	return "*🔰 NASA APOD 🔰*\n\n" +
		"*NASA ASTRONOMY PICTURE OF THE DAY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "APOD ❯*"
}

func handleApod(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING NASA APOD....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Title       string `json:"title"`
			Date        string `json:"date"`
			Explanation string `json:"explanation"`
			URL         string `json:"url"`
			MediaType   string `json:"media_type"`
		}
		if err := funGetJSON(ctx, "https://api.nasa.gov/planetary/apod?api_key=DEMO_KEY", &res); err != nil || res.Title == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH APOD*")
			}
			return
		}
		expl := res.Explanation
		if len(expl) > 700 {
			expl = expl[:700] + "..."
		}
		caption := "*🔰 NASA APOD 🔰*\n\n*🌌 " + res.Title + "*\n*📅 " + res.Date + "*\n\n*" + expl + "*"
		if res.MediaType == "image" && res.URL != "" {
			if img, err := funGetBytes(ctx, res.URL); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption+"\n\n*🔗 "+res.URL+"*")
	})
}

// ── .STARWARS ────────────────────────────────────────────────────────────────

func starwarsGuide(prefix string) string {
	return "*🔰 STAR WARS 🔰*\n\n" +
		"*GET INFO ABOUT ANY STAR WARS CHARACTER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "STARWARS <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "STARWARS luke ❯*"
}

func handleStarwars(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, starwarsGuide(prefix))
			return
		}
		name := strings.Join(args, " ")
		waitID := s.ReplyWithID(info, "*SEARCHING STAR WARS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Message string `json:"message"`
			Result  []struct {
				Properties struct {
					Name      string `json:"name"`
					Gender    string `json:"gender"`
					Height    string `json:"height"`
					Mass      string `json:"mass"`
					HairColor string `json:"hair_color"`
					EyeColor  string `json:"eye_color"`
					BirthYear string `json:"birth_year"`
				} `json:"properties"`
			} `json:"result"`
		}
		u := "https://swapi.tech/api/people/?name=" + url.QueryEscape(name)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Result) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 CHARACTER NOT FOUND*")
			}
			return
		}
		p := res.Result[0].Properties
		var b strings.Builder
		b.WriteString("*🔰 STAR WARS 🔰*\n\n")
		b.WriteString("*⭐ " + strings.ToUpper(p.Name) + "*\n")
		b.WriteString("*👤 GENDER ❯ " + p.Gender + "*\n")
		b.WriteString("*📏 HEIGHT ❯ " + p.Height + " cm*\n")
		b.WriteString("*⚖️ MASS ❯ " + p.Mass + " kg*\n")
		b.WriteString("*💇 HAIR ❯ " + p.HairColor + "*\n")
		b.WriteString("*👁️ EYES ❯ " + p.EyeColor + "*\n")
		b.WriteString("*🎂 BORN ❯ " + p.BirthYear + "*")
		s.Reply(info, b.String())
	})
}

// ── .RICKANDMORTY ────────────────────────────────────────────────────────────

func rickmortyGuide(prefix string) string {
	return "*🔰 RICK & MORTY 🔰*\n\n" +
		"*GET INFO ABOUT ANY RICK & MORTY CHARACTER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RICKANDMORTY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RICKANDMORTY rick ❯*"
}

func handleRickMorty(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, rickmortyGuide(prefix))
			return
		}
		name := strings.Join(args, " ")
		waitID := s.ReplyWithID(info, "*SEARCHING CHARACTER....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Results []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Species string `json:"species"`
				Gender  string `json:"gender"`
				Origin  struct {
					Name string `json:"name"`
				} `json:"origin"`
				Location struct {
					Name string `json:"name"`
				} `json:"location"`
				Image string `json:"image"`
			} `json:"results"`
		}
		u := "https://rickandmortyapi.com/api/character/?name=" + url.QueryEscape(name)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 CHARACTER NOT FOUND*")
			}
			return
		}
		c := res.Results[0]
		caption := "*🔰 RICK & MORTY 🔰*\n\n*🧬 " + strings.ToUpper(c.Name) + "*\n" +
			"*❤️ STATUS ❯ " + c.Status + "*\n" +
			"*👽 SPECIES ❯ " + c.Species + "*\n" +
			"*👤 GENDER ❯ " + c.Gender + "*\n" +
			"*🏠 ORIGIN ❯ " + c.Origin.Name + "*\n" +
			"*📍 LOCATION ❯ " + c.Location.Name + "*"
		if c.Image != "" {
			if img, err := funGetBytes(ctx, c.Image); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .CHUCKNORRIS ─────────────────────────────────────────────────────────────

func chucknorrisGuide(prefix string) string {
	return "*🔰 CHUCK NORRIS 🔰*\n\n" +
		"*GET A RANDOM CHUCK NORRIS JOKE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CHUCKNORRIS [CATEGORY] ❯*\n" +
		"*CATEGORIES ❯ animal, career, dev, food, money, movie, music, science, sport*\n" +
		"*EXAMPLE ❮ " + prefix + "CHUCKNORRIS dev ❯*"
}

func handleChuckNorris(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		u := "https://api.chucknorris.io/jokes/random"
		if len(args) >= 1 {
			u += "?category=" + url.QueryEscape(strings.TrimSpace(args[0]))
		}
		waitID := s.ReplyWithID(info, "*FETCHING JOKE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Value string `json:"value"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Value == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH JOKE*")
			}
			return
		}
		s.Reply(info, "*🔰 CHUCK NORRIS 🔰*\n\n*💪 "+res.Value+"*")
	})
}

// ── .KANYE ───────────────────────────────────────────────────────────────────

func kanyeGuide(prefix string) string {
	return "*🔰 KANYE QUOTE 🔰*\n\n" +
		"*GET A RANDOM KANYE WEST QUOTE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "KANYE ❯*"
}

func handleKanye(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QUOTE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Quote string `json:"quote"`
		}
		if err := funGetJSON(ctx, "https://api.kanye.rest/", &res); err != nil || res.Quote == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH QUOTE*")
			}
			return
		}
		s.Reply(info, "*🔰 KANYE QUOTE 🔰*\n\n*❝ "+res.Quote+" ❞*\n\n*— KANYE WEST*")
	})
}

// ── .RANDOMWORD ──────────────────────────────────────────────────────────────

func randomwordGuide(prefix string) string {
	return "*🔰 RANDOM WORDS 🔰*\n\n" +
		"*GET RANDOM ENGLISH WORDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMWORD [COUNT] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMWORD 5 ❯*"
}

func handleRandomWord(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		n := 5
		if len(args) >= 1 {
			if v, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && v > 0 && v <= 20 {
				n = v
			}
		}
		waitID := s.ReplyWithID(info, "*GENERATING WORDS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var words []string
		u := "https://random-word-api.herokuapp.com/word?number=" + strconv.Itoa(n)
		if err := funGetJSON(ctx, u, &words); err != nil || len(words) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT GENERATE WORDS*")
			}
			return
		}
		s.Reply(info, "*🔰 RANDOM WORDS 🔰*\n\n*📝 "+strings.Join(words, ", ")+"*")
	})
}

// ── .CATFACT ─────────────────────────────────────────────────────────────────

func catfactGuide(prefix string) string {
	return "*🔰 CAT FACT 🔰*\n\n" +
		"*GET A RANDOM CAT FACT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CATFACT ❯*"
}

func handleCatFact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING CAT FACT....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Fact string `json:"fact"`
		}
		if err := funGetJSON(ctx, "https://catfact.ninja/fact", &res); err != nil || res.Fact == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH FACT*")
			}
			return
		}
		s.Reply(info, "*🔰 CAT FACT 🔰*\n\n*🐱 "+res.Fact+"*")
	})
}

// ── .DRUG ────────────────────────────────────────────────────────────────────

func drugGuide(prefix string) string {
	return "*🔰 DRUG INFO 🔰*\n\n" +
		"*GET FDA LABEL INFO FOR ANY DRUG*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DRUG <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DRUG ibuprofen ❯*"
}

func handleDrug(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, drugGuide(prefix))
			return
		}
		name := strings.Join(args, " ")
		waitID := s.ReplyWithID(info, "*SEARCHING DRUG INFO....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Results []struct {
				OpenFDA struct {
					BrandName    []string `json:"brand_name"`
					GenericName  []string `json:"generic_name"`
					Manufacturer []string `json:"manufacturer_name"`
				} `json:"openfda"`
				Purpose     []string `json:"purpose"`
				Indications []string `json:"indications_and_usage"`
			} `json:"results"`
		}
		u := "https://api.fda.gov/drug/label.json?search=openfda.generic_name:" +
			url.QueryEscape(name) + "&limit=1"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 DRUG NOT FOUND*")
			}
			return
		}
		r := res.Results[0]
		first := func(a []string) string {
			if len(a) > 0 {
				return a[0]
			}
			return "-"
		}
		trim := func(t string) string {
			t = strings.TrimSpace(t)
			if len(t) > 400 {
				t = t[:400] + "..."
			}
			return t
		}
		var b strings.Builder
		b.WriteString("*🔰 DRUG INFO 🔰*\n\n")
		b.WriteString("*💊 BRAND ❯ " + first(r.OpenFDA.BrandName) + "*\n")
		b.WriteString("*🧪 GENERIC ❯ " + first(r.OpenFDA.GenericName) + "*\n")
		if m := first(r.OpenFDA.Manufacturer); m != "-" {
			b.WriteString("*🏭 MAKER ❯ " + m + "*\n")
		}
		if p := first(r.Purpose); p != "-" {
			b.WriteString("\n*📋 PURPOSE ❯* " + trim(p) + "\n")
		}
		if ind := first(r.Indications); ind != "-" {
			b.WriteString("\n*🩺 USES ❯* " + trim(ind))
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .STACKOVERFLOW ───────────────────────────────────────────────────────────

func stackoverflowGuide(prefix string) string {
	return "*🔰 STACK OVERFLOW 🔰*\n\n" +
		"*SEARCH STACK OVERFLOW FOR ANSWERS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "STACKOVERFLOW <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "STACKOVERFLOW golang goroutine ❯*"
}

func handleStackOverflow(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, stackoverflowGuide(prefix))
			return
		}
		q := strings.Join(args, " ")
		waitID := s.ReplyWithID(info, "*SEARCHING STACK OVERFLOW....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Items []struct {
				Title       string `json:"title"`
				Link        string `json:"link"`
				Score       int    `json:"score"`
				IsAnswered  bool   `json:"is_answered"`
				AnswerCount int    `json:"answer_count"`
				ViewCount   int    `json:"view_count"`
			} `json:"items"`
		}
		u := "https://api.stackexchange.com/2.3/search/advanced?order=desc&sort=relevance&q=" +
			url.QueryEscape(q) + "&site=stackoverflow&pagesize=5&filter=default"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Items) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO RESULTS FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 STACK OVERFLOW 🔰*\n\n")
		for i, it := range res.Items {
			if i >= 5 {
				break
			}
			mark := "❌"
			if it.IsAnswered {
				mark = "✅"
			}
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + it.Title + "*\n")
			b.WriteString("• " + mark + " " + strconv.Itoa(it.AnswerCount) + " answers • ⬆️ " +
				strconv.Itoa(it.Score) + " • 👁️ " + strconv.Itoa(it.ViewCount) + "\n")
			b.WriteString("• 🔗 " + it.Link + "\n\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "pincode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND POST OFFICES BY INDIA PIN CODE. USE IT AS .PINCODE <CODE>.", Run: handlePincode})
	Register(Command{Name: "hackernews", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TOP STORIES FROM HACKER NEWS. USE IT AS .HACKERNEWS.", Run: handleHackerNews})
	Register(Command{Name: "synonym", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND SYNONYMS OF ANY WORD. USE IT AS .SYNONYM <WORD>.", Run: handleSynonym})
	Register(Command{Name: "mac", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE VENDOR OF ANY MAC ADDRESS. USE IT AS .MAC <MAC>.", Run: handleMac})
	Register(Command{Name: "sunrise", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET SUNRISE + SUNSET TIMES FOR ANY LOCATION. USE IT AS .SUNRISE <LAT> <LNG>.", Run: handleSunrise})
	Register(Command{Name: "apod", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET NASA ASTRONOMY PICTURE OF THE DAY. USE IT AS .APOD.", Run: handleApod})
	Register(Command{Name: "starwars", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO ABOUT ANY STAR WARS CHARACTER. USE IT AS .STARWARS <NAME>.", Run: handleStarwars})
	Register(Command{Name: "rickandmorty", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO ABOUT ANY RICK & MORTY CHARACTER. USE IT AS .RICKANDMORTY <NAME>.", Run: handleRickMorty})
	Register(Command{Name: "chucknorris", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM CHUCK NORRIS JOKE. USE IT AS .CHUCKNORRIS [CATEGORY].", Run: handleChuckNorris})
	Register(Command{Name: "kanye", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM KANYE WEST QUOTE. USE IT AS .KANYE.", Run: handleKanye})
	Register(Command{Name: "randomword", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET RANDOM ENGLISH WORDS. USE IT AS .RANDOMWORD [COUNT].", Run: handleRandomWord})
	Register(Command{Name: "catfact", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM CAT FACT. USE IT AS .CATFACT.", Run: handleCatFact})
	Register(Command{Name: "drug", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET FDA LABEL INFO FOR ANY DRUG. USE IT AS .DRUG <NAME>.", Run: handleDrug})
	Register(Command{Name: "stackoverflow", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SEARCH STACK OVERFLOW FOR ANSWERS. USE IT AS .STACKOVERFLOW <QUERY>.", Run: handleStackOverflow})

	// hidden short aliases
	Register(Command{Name: "hn", Category: "TOOLS", Desc: "Short alias of .hackernews", Hidden: true, Run: handleHackerNews})
	Register(Command{Name: "syn", Category: "TOOLS", Desc: "Short alias of .synonym", Hidden: true, Run: handleSynonym})
	Register(Command{Name: "sw", Category: "TOOLS", Desc: "Short alias of .starwars", Hidden: true, Run: handleStarwars})
	Register(Command{Name: "cn", Category: "TOOLS", Desc: "Short alias of .chucknorris", Hidden: true, Run: handleChuckNorris})
	Register(Command{Name: "so", Category: "TOOLS", Desc: "Short alias of .stackoverflow", Hidden: true, Run: handleStackOverflow})
	Register(Command{Name: "word", Category: "TOOLS", Desc: "Short alias of .randomword", Hidden: true, Run: handleRandomWord})
}

var _ = fmt.Sprintf
