package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 4 (15 everyday commands)
// File: toolpack4.go
// ============================================================================
//   .zip <country> <code>   -> postal/zip code -> city + state
//   .holiday <cc> [year]    -> public holidays of a country
//   .university <name>      -> university search
//   .iss                    -> live ISS location
//   .earthquake             -> recent significant earthquakes
//   .gender <name>          -> predicted gender of a name
//   .age <name>             -> predicted age of a name
//   .nationality <name>     -> predicted nationality of a name
//   .iban <iban>            -> IBAN validation + bank info
//   .recipe <dish>          -> food recipe
//   .cocktail <drink>       -> cocktail recipe
//   .dog                    -> random dog photo
//   .cat                    -> random cat photo
//   .advice                 -> random advice
//   .trivia                 -> random quiz question
// ============================================================================

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .ZIP ────────────────────────────────────────────────────────────────────

func zipGuide(prefix string) string {
	return "*🔰 ZIP / POSTAL CODE 🔰*\n\n" +
		"*GET CITY + STATE FROM ANY ZIP / POSTAL CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ZIP <COUNTRY> <CODE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ZIP US 90210 ❯*"
}

func handleZip(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, zipGuide(prefix))
			return
		}
		country := strings.ToLower(strings.TrimSpace(args[0]))
		code := strings.TrimSpace(strings.Join(args[1:], " "))
		waitID := s.ReplyWithID(info, "*LOOKING UP ZIP CODE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.zippopotam.us/" + url.PathEscape(country) + "/" + url.PathEscape(code)
		var res struct {
			Country  string `json:"country"`
			PostCode string `json:"post code"`
			Places   []struct {
				PlaceName string `json:"place name"`
				State     string `json:"state"`
				Lat       string `json:"latitude"`
				Lon       string `json:"longitude"`
			} `json:"places"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Places) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 ZIP CODE NOT FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 ZIP / POSTAL CODE 🔰*\n\n")
		b.WriteString("*📮 CODE ❯ " + res.PostCode + "*\n")
		b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(res.Country) + "*\n\n")
		for i, p := range res.Places {
			if i >= 5 {
				break
			}
			b.WriteString("*📍 " + strings.ToUpper(p.PlaceName) + "*\n")
			b.WriteString("• STATE: " + p.State + "\n")
			b.WriteString("• LAT/LON: " + p.Lat + ", " + p.Lon + "\n\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .HOLIDAY ────────────────────────────────────────────────────────────────

func holidayGuide(prefix string) string {
	return "*🔰 PUBLIC HOLIDAYS 🔰*\n\n" +
		"*GET PUBLIC HOLIDAYS OF ANY COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HOLIDAY <COUNTRY> [YEAR] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "HOLIDAY US 2025 ❯*"
}

func handleHoliday(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, holidayGuide(prefix))
			return
		}
		cc := strings.ToUpper(strings.TrimSpace(args[0]))
		year := time.Now().Year()
		if len(args) >= 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil && n > 1900 && n < 2200 {
				year = n
			}
		}
		waitID := s.ReplyWithID(info, "*FETCHING HOLIDAYS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://date.nager.at/api/v3/PublicHolidays/" + strconv.Itoa(year) + "/" + url.PathEscape(cc)
		var res []struct {
			Date      string `json:"date"`
			LocalName string `json:"localName"`
			Name      string `json:"name"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO HOLIDAYS FOUND (CHECK COUNTRY CODE)*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 PUBLIC HOLIDAYS 🔰*\n\n")
		b.WriteString("*🌍 COUNTRY ❯ " + cc + "*\n")
		b.WriteString("*📅 YEAR ❯ " + strconv.Itoa(year) + "*\n\n")
		for i, h := range res {
			if i >= 20 {
				b.WriteString("• ...and " + strconv.Itoa(len(res)-20) + " more\n")
				break
			}
			name := h.LocalName
			if name == "" {
				name = h.Name
			}
			b.WriteString("• " + h.Date + " — " + name + "\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .UNIVERSITY ─────────────────────────────────────────────────────────────

func universityGuide(prefix string) string {
	return "*🔰 UNIVERSITY SEARCH 🔰*\n\n" +
		"*SEARCH UNIVERSITIES AROUND THE WORLD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UNIVERSITY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "UNIVERSITY HARVARD ❯*"
}

func handleUniversity(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, universityGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING UNIVERSITIES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "http://universities.hipolabs.com/search?name=" + url.QueryEscape(q)
		var res []struct {
			Name    string   `json:"name"`
			Country string   `json:"country"`
			Webs    []string `json:"web_pages"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO UNIVERSITY FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 UNIVERSITY SEARCH 🔰*\n\n")
		b.WriteString("*🔎 QUERY ❯ " + strings.ToUpper(q) + "*\n\n")
		for i, r := range res {
			if i >= 8 {
				break
			}
			b.WriteString("*🎓 " + r.Name + "*\n")
			b.WriteString("• COUNTRY: " + r.Country + "\n")
			if len(r.Webs) > 0 {
				b.WriteString("• WEB: " + r.Webs[0] + "\n")
			}
			b.WriteString("\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .ISS ────────────────────────────────────────────────────────────────────

func issGuide(prefix string) string {
	return "*🔰 ISS TRACKER 🔰*\n\n" +
		"*GET THE LIVE LOCATION OF THE INTERNATIONAL SPACE STATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ISS ❯*"
}

func handleISS(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*LOCATING ISS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			ISS struct {
				Lat string `json:"latitude"`
				Lon string `json:"longitude"`
			} `json:"iss_position"`
			Timestamp int64 `json:"timestamp"`
		}
		if err := funGetJSON(ctx, "http://api.open-notify.org/iss-now.json", &res); err != nil || res.ISS.Lat == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 ISS DATA UNAVAILABLE, TRY AGAIN*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 ISS TRACKER 🔰*\n\n")
		b.WriteString("*🛰️ INTERNATIONAL SPACE STATION*\n\n")
		b.WriteString("*📍 LATITUDE ❯ " + res.ISS.Lat + "*\n")
		b.WriteString("*📍 LONGITUDE ❯ " + res.ISS.Lon + "*\n")
		b.WriteString("*🕐 UPDATED ❯ " + time.Unix(res.Timestamp, 0).UTC().Format("15:04:05 UTC") + "*\n\n")
		b.WriteString("*🗺️ MAP ❯ https://www.google.com/maps?q=" + res.ISS.Lat + "," + res.ISS.Lon + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .EARTHQUAKE ─────────────────────────────────────────────────────────────

func earthquakeGuide(prefix string) string {
	return "*🔰 EARTHQUAKE FEED 🔰*\n\n" +
		"*GET RECENT SIGNIFICANT EARTHQUAKES (PAST WEEK)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "EARTHQUAKE ❯*"
}

func handleEarthquake(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING EARTHQUAKE DATA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Features []struct {
				Properties struct {
					Mag   float64 `json:"mag"`
					Place string  `json:"place"`
					Time  int64   `json:"time"`
				} `json:"properties"`
			} `json:"features"`
		}
		u := "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/significant_week.geojson"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Features) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO SIGNIFICANT EARTHQUAKES THIS WEEK*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 EARTHQUAKE FEED 🔰*\n\n")
		b.WriteString("*🌍 SIGNIFICANT QUAKES (PAST WEEK)*\n\n")
		for i, f := range res.Features {
			if i >= 8 {
				break
			}
			b.WriteString("*💥 MAG " + strconv.FormatFloat(f.Properties.Mag, 'f', 1, 64) + "*\n")
			b.WriteString("• " + f.Properties.Place + "\n")
			b.WriteString("• " + time.Unix(f.Properties.Time/1000, 0).UTC().Format("02 Jan 15:04 UTC") + "\n\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .GENDER ─────────────────────────────────────────────────────────────────

func genderGuide(prefix string) string {
	return "*🔰 NAME GENDER 🔰*\n\n" +
		"*PREDICT THE GENDER OF ANY NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GENDER <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "GENDER ALI ❯*"
}

func handleGender(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, genderGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*ANALYZING NAME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Name        string  `json:"name"`
			Gender      string  `json:"gender"`
			Probability float64 `json:"probability"`
			Count       int     `json:"count"`
		}
		u := "https://api.genderize.io?name=" + url.QueryEscape(name)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Gender == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT PREDICT GENDER*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NAME GENDER 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*⚧ GENDER ❯ " + strings.ToUpper(res.Gender) + "*\n")
		b.WriteString("*📊 CONFIDENCE ❯ " + strconv.Itoa(int(res.Probability*100)) + "%*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .AGE ────────────────────────────────────────────────────────────────────

func ageGuide(prefix string) string {
	return "*🔰 NAME AGE 🔰*\n\n" +
		"*PREDICT THE AGE OF ANY NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AGE <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "AGE ALI ❯*"
}

func handleAge(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, ageGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*ANALYZING NAME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Name  string `json:"name"`
			Age   int    `json:"age"`
			Count int    `json:"count"`
		}
		u := "https://api.agify.io?name=" + url.QueryEscape(name)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Age == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT PREDICT AGE*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NAME AGE 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*🎂 PREDICTED AGE ❯ " + strconv.Itoa(res.Age) + " YEARS*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .NATIONALITY ────────────────────────────────────────────────────────────

func nationalityGuide(prefix string) string {
	return "*🔰 NAME NATIONALITY 🔰*\n\n" +
		"*PREDICT THE NATIONALITY OF ANY NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NATIONALITY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NATIONALITY ALI ❯*"
}

func handleNationality(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, nationalityGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*ANALYZING NAME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Name    string `json:"name"`
			Country []struct {
				ID          string  `json:"country_id"`
				Probability float64 `json:"probability"`
			} `json:"country"`
		}
		u := "https://api.nationalize.io?name=" + url.QueryEscape(name)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Country) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT PREDICT NATIONALITY*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NAME NATIONALITY 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n\n")
		for i, c := range res.Country {
			if i >= 5 {
				break
			}
			b.WriteString("• " + c.ID + " — " + strconv.Itoa(int(c.Probability*100)) + "%\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .IBAN ───────────────────────────────────────────────────────────────────

func ibanGuide(prefix string) string {
	return "*🔰 IBAN VALIDATOR 🔰*\n\n" +
		"*VALIDATE ANY IBAN + GET BANK INFO*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "IBAN <IBAN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "IBAN DE89370400440532013000 ❯*"
}

func handleIBAN(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		iban := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(strings.Join(args, " ")), " ", ""))
		if iban == "" {
			s.Reply(info, ibanGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*VALIDATING IBAN....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Valid    bool     `json:"valid"`
			Messages []string `json:"messages"`
			IBAN     string   `json:"iban"`
			BankData struct {
				Name string `json:"name"`
				BIC  string `json:"bic"`
				City string `json:"city"`
			} `json:"bankData"`
		}
		u := "https://openiban.com/validate/" + url.PathEscape(iban) + "?getBIC=true"
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 IBAN CHECK FAILED, TRY AGAIN*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 IBAN VALIDATOR 🔰*\n\n")
		b.WriteString("*🏦 IBAN ❯ " + res.IBAN + "*\n")
		if res.Valid {
			b.WriteString("*✅ STATUS ❯ VALID*\n")
		} else {
			b.WriteString("*❌ STATUS ❯ INVALID*\n")
		}
		if res.BankData.Name != "" {
			b.WriteString("*🏛️ BANK ❯ " + res.BankData.Name + "*\n")
		}
		if res.BankData.BIC != "" {
			b.WriteString("*🔑 BIC ❯ " + res.BankData.BIC + "*\n")
		}
		if res.BankData.City != "" {
			b.WriteString("*📍 CITY ❯ " + res.BankData.City + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .RECIPE ─────────────────────────────────────────────────────────────────

func recipeGuide(prefix string) string {
	return "*🔰 FOOD RECIPE 🔰*\n\n" +
		"*GET A RECIPE FOR ANY DISH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RECIPE <DISH> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RECIPE CHICKEN ❯*"
}

func handleRecipe(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, recipeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING RECIPE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Meals []struct {
				Name         string `json:"strMeal"`
				Category     string `json:"strCategory"`
				Area         string `json:"strArea"`
				Instructions string `json:"strInstructions"`
				Thumb        string `json:"strMealThumb"`
			} `json:"meals"`
		}
		u := "https://www.themealdb.com/api/json/v1/1/search.php?s=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Meals) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO RECIPE FOUND*")
			}
			return
		}
		m := res.Meals[0]
		ins := strings.ReplaceAll(m.Instructions, "\r\n", "\n")
		ins = strings.TrimSpace(ins)
		if len(ins) > 900 {
			ins = ins[:900] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 FOOD RECIPE 🔰*\n\n")
		b.WriteString("*🍽️ DISH ❯ " + strings.ToUpper(m.Name) + "*\n")
		if m.Category != "" {
			b.WriteString("*📂 CATEGORY ❯ " + m.Category + "*\n")
		}
		if m.Area != "" {
			b.WriteString("*🌍 CUISINE ❯ " + m.Area + "*\n")
		}
		b.WriteString("\n*📖 INSTRUCTIONS:*\n" + ins)
		caption := strings.TrimSpace(b.String())
		if m.Thumb != "" {
			if img, err := funGetBytes(ctx, m.Thumb); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .COCKTAIL ───────────────────────────────────────────────────────────────

func cocktailGuide(prefix string) string {
	return "*🔰 COCKTAIL RECIPE 🔰*\n\n" +
		"*GET A RECIPE FOR ANY COCKTAIL / DRINK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COCKTAIL <DRINK> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COCKTAIL MARGARITA ❯*"
}

func handleCocktail(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, cocktailGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING COCKTAIL....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Drinks []struct {
				Name         string `json:"strDrink"`
				Category     string `json:"strCategory"`
				Alcoholic    string `json:"strAlcoholic"`
				Glass        string `json:"strGlass"`
				Instructions string `json:"strInstructions"`
				Thumb        string `json:"strDrinkThumb"`
			} `json:"drinks"`
		}
		u := "https://www.thecocktaildb.com/api/json/v1/1/search.php?s=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Drinks) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO COCKTAIL FOUND*")
			}
			return
		}
		d := res.Drinks[0]
		ins := strings.ReplaceAll(d.Instructions, "\r\n", "\n")
		ins = strings.TrimSpace(ins)
		if len(ins) > 800 {
			ins = ins[:800] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 COCKTAIL RECIPE 🔰*\n\n")
		b.WriteString("*🍸 DRINK ❯ " + strings.ToUpper(d.Name) + "*\n")
		if d.Category != "" {
			b.WriteString("*📂 CATEGORY ❯ " + d.Category + "*\n")
		}
		if d.Alcoholic != "" {
			b.WriteString("*🍷 TYPE ❯ " + d.Alcoholic + "*\n")
		}
		if d.Glass != "" {
			b.WriteString("*🥃 GLASS ❯ " + d.Glass + "*\n")
		}
		b.WriteString("\n*📖 INSTRUCTIONS:*\n" + ins)
		caption := strings.TrimSpace(b.String())
		if d.Thumb != "" {
			if img, err := funGetBytes(ctx, d.Thumb); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .HIJRI ───────────────────────────────────────────────────────────────────

func hijriGuide(prefix string) string {
	return "*🔰 HIJRI DATE 🔰*\n\n" +
		"*GET TODAY'S HIJRI (ISLAMIC) DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HIJRI ❯*"
}

func handleHijri(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING HIJRI DATE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Code int `json:"code"`
			Data struct {
				Gregorian struct {
					Date string `json:"date"`
				} `json:"gregorian"`
				Hijri struct {
					Date    string `json:"date"`
					Day     string `json:"day"`
					Weekday struct {
						En string `json:"en"`
					} `json:"weekday"`
					Month struct {
						En string `json:"en"`
					} `json:"month"`
					Year string `json:"year"`
				} `json:"hijri"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://api.aladhan.com/v1/gToH", &res); err != nil || res.Code != 200 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH HIJRI DATE*")
			}
			return
		}
		h := res.Data.Hijri
		var b strings.Builder
		b.WriteString("*🔰 HIJRI DATE 🔰*\n\n")
		b.WriteString("*🌙 " + h.Day + " " + h.Month.En + " " + h.Year + " AH*\n")
		b.WriteString("*📅 " + h.Weekday.En + "*\n")
		b.WriteString("*🗓️ GREGORIAN ❯ " + res.Data.Gregorian.Date + "*")
		s.Reply(info, b.String())
	})
}

// ── .QIBLA ───────────────────────────────────────────────────────────────────

func qiblaGuide(prefix string) string {
	return "*🔰 QIBLA DIRECTION 🔰*\n\n" +
		"*GET THE QIBLA DIRECTION FOR ANY LOCATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "QIBLA <LAT> <LNG> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "QIBLA 24.86 67.01 ❯*"
}

func handleQibla(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, qiblaGuide(prefix))
			return
		}
		lat := strings.TrimSpace(args[0])
		lng := strings.TrimSpace(args[1])
		waitID := s.ReplyWithID(info, "*CALCULATING QIBLA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Code int `json:"code"`
			Data struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Direction float64 `json:"direction"`
			} `json:"data"`
		}
		u := "https://api.aladhan.com/v1/qibla/" + url.PathEscape(lat) + "/" + url.PathEscape(lng)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT CALCULATE QIBLA*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 QIBLA DIRECTION 🔰*\n\n")
		b.WriteString("*📍 COORDS ❯ " + lat + ", " + lng + "*\n")
		b.WriteString("*🕋 DIRECTION ❯ " + strconv.FormatFloat(res.Data.Direction, 'f', 2, 64) + "° FROM NORTH*")
		s.Reply(info, b.String())
	})
}

// ── .ADVICE ─────────────────────────────────────────────────────────────────

func adviceGuide(prefix string) string {
	return "*🔰 RANDOM ADVICE 🔰*\n\n" +
		"*GET A RANDOM PIECE OF ADVICE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ADVICE ❯*"
}

func handleAdvice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING ADVICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Slip struct {
				Advice string `json:"advice"`
			} `json:"slip"`
		}
		if err := funGetJSON(ctx, "https://api.adviceslip.com/advice", &res); err != nil || res.Slip.Advice == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH ADVICE*")
			}
			return
		}
		s.Reply(info, "*🔰 RANDOM ADVICE 🔰*\n\n*💡 "+res.Slip.Advice+"*")
	})
}

// ── .TRIVIA ─────────────────────────────────────────────────────────────────

func triviaGuide(prefix string) string {
	return "*🔰 TRIVIA QUIZ 🔰*\n\n" +
		"*GET A RANDOM QUIZ QUESTION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TRIVIA ❯*"
}

func handleTrivia(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QUESTION....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Results []struct {
				Category         string   `json:"category"`
				Difficulty       string   `json:"difficulty"`
				Question         string   `json:"question"`
				CorrectAnswer    string   `json:"correct_answer"`
				IncorrectAnswers []string `json:"incorrect_answers"`
			} `json:"results"`
		}
		if err := funGetJSON(ctx, "https://opentdb.com/api.php?amount=1", &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COULD NOT FETCH QUESTION*")
			}
			return
		}
		r := res.Results[0]
		var b strings.Builder
		b.WriteString("*🔰 TRIVIA QUIZ 🔰*\n\n")
		b.WriteString("*📂 " + html.UnescapeString(r.Category) + " • " + strings.ToUpper(r.Difficulty) + "*\n\n")
		b.WriteString("*❓ " + html.UnescapeString(r.Question) + "*\n\n")
		b.WriteString("*OPTIONS:*\n")
		opts := append([]string{r.CorrectAnswer}, r.IncorrectAnswers...)
		for i, o := range opts {
			b.WriteString(strconv.Itoa(i+1) + ". " + html.UnescapeString(o) + "\n")
		}
		b.WriteString("\n*✅ ANSWER ❯ " + html.UnescapeString(r.CorrectAnswer) + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

func init() {
	Register(Command{Name: "zip", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET CITY + STATE FROM ANY ZIP / POSTAL CODE. USE IT AS .ZIP <COUNTRY> <CODE>.", Run: handleZip})
	Register(Command{Name: "holiday", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET PUBLIC HOLIDAYS OF ANY COUNTRY. USE IT AS .HOLIDAY <COUNTRY> [YEAR].", Run: handleHoliday})
	Register(Command{Name: "university", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SEARCH UNIVERSITIES. USE IT AS .UNIVERSITY <NAME>.", Run: handleUniversity})
	Register(Command{Name: "iss", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE LOCATION OF THE ISS. USE IT AS .ISS.", Run: handleISS})
	Register(Command{Name: "earthquake", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET RECENT SIGNIFICANT EARTHQUAKES. USE IT AS .EARTHQUAKE.", Run: handleEarthquake})
	Register(Command{Name: "gender", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE GENDER OF A NAME. USE IT AS .GENDER <NAME>.", Run: handleGender})
	Register(Command{Name: "age", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE AGE OF A NAME. USE IT AS .AGE <NAME>.", Run: handleAge})
	Register(Command{Name: "nationality", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE NATIONALITY OF A NAME. USE IT AS .NATIONALITY <NAME>.", Run: handleNationality})
	Register(Command{Name: "iban", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO VALIDATE AN IBAN + GET BANK INFO. USE IT AS .IBAN <IBAN>.", Run: handleIBAN})
	Register(Command{Name: "recipe", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RECIPE FOR ANY DISH. USE IT AS .RECIPE <DISH>.", Run: handleRecipe})
	Register(Command{Name: "cocktail", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A COCKTAIL RECIPE. USE IT AS .COCKTAIL <DRINK>.", Run: handleCocktail})
	Register(Command{Name: "hijri", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TODAY'S HIJRI (ISLAMIC) DATE. USE IT AS .HIJRI.", Run: handleHijri})
	Register(Command{Name: "qibla", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE QIBLA DIRECTION FOR ANY LOCATION. USE IT AS .QIBLA <LAT> <LNG>.", Run: handleQibla})
	Register(Command{Name: "advice", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM PIECE OF ADVICE. USE IT AS .ADVICE.", Run: handleAdvice})
	Register(Command{Name: "trivia", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM QUIZ QUESTION. USE IT AS .TRIVIA.", Run: handleTrivia})

	// hidden aliases
	Register(Command{Name: "postal", Category: "TOOLS", Desc: "Short alias of .zip", Hidden: true, Run: handleZip})
	Register(Command{Name: "holidays", Category: "TOOLS", Desc: "Short alias of .holiday", Hidden: true, Run: handleHoliday})
	Register(Command{Name: "uni", Category: "TOOLS", Desc: "Short alias of .university", Hidden: true, Run: handleUniversity})
	Register(Command{Name: "quiz", Category: "TOOLS", Desc: "Short alias of .trivia", Hidden: true, Run: handleTrivia})
}

// keep fmt imported even if a future edit drops its only use
var _ = fmt.Sprintf
