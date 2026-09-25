package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 10 (10 new everyday commands)
// File: toolpack10.go
// ============================================================================
//   .agify <name>        -> predict the age of a name
//   .genderize <name>    -> predict the gender of a name
//   .nationalize <name>  -> predict the nationality of a name
//   .maclookup <mac>     -> find the vendor of a MAC address
//   .zipcode <zip>       -> location info for a postal code
//   .avatar <text>       -> generate a random avatar image
//   .jokeapi             -> get a random joke
//   .coingecko <id>      -> live crypto price from CoinGecko
//   .exchangerate <a> <b>-> live currency exchange rate
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

// ── .AGIFY ──────────────────────────────────────────────────────────────────

func agifyGuide(prefix string) string {
	return "*🔰 AGE PREDICTOR 🔰*\n\n" +
		"*PREDICT THE AGE OF A NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AGIFY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "AGIFY MICHAEL ❯*"
}

func handleAgify(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, agifyGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*PREDICTING AGE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Name  string `json:"name"`
			Age   int    `json:"age"`
			Count int    `json:"count"`
		}
		u := "https://api.agify.io/?name=" + url.QueryEscape(name)
		if err := funGetJSONRetry(ctx, u, &res, 3); err != nil || res.Age == 0 {
			// Fallback: route through the Jina reader proxy (different egress IP)
			// when agify rate-limits this host.
			if err2 := funGetJSONViaJina(ctx, u, &res); err2 != nil || res.Age == 0 {
				if !ctxTimedOut(ctx) {
					funFail(s, info, "AGIFY")
				}
				return
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 AGE PREDICTOR 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*🎂 PREDICTED AGE ❯ " + strconv.Itoa(res.Age) + " YEARS*\n")
		b.WriteString("*📊 SAMPLES ❯ " + strconv.Itoa(res.Count) + "*")
		s.Reply(info, b.String())
	})
}

// ── .GENDERIZE ──────────────────────────────────────────────────────────────

func genderizeGuide(prefix string) string {
	return "*🔰 GENDER PREDICTOR 🔰*\n\n" +
		"*PREDICT THE GENDER OF A NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GENDERIZE <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "GENDERIZE ALEX ❯*"
}

func handleGenderize(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, genderizeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*PREDICTING GENDER....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Name        string  `json:"name"`
			Gender      string  `json:"gender"`
			Probability float64 `json:"probability"`
			Count       int     `json:"count"`
		}
		u := "https://api.genderize.io/?name=" + url.QueryEscape(name)
		if err := funGetJSONRetry(ctx, u, &res, 3); err != nil || res.Gender == "" {
			// Fallback: genderapi.io (free tier) when genderize.io is
			// rate-limited or returns no prediction.
			var alt struct {
				Name        string `json:"name"`
				Gender      string `json:"gender"`
				Probability int    `json:"probability"`
				TotalNames  int    `json:"total_names"`
			}
			au := "https://api.genderapi.io/api/?name=" + url.QueryEscape(name)
			if err2 := funGetJSONRetry(ctx, au, &alt, 2); err2 == nil && alt.Gender != "" {
				var fb strings.Builder
				fb.WriteString("*\U0001f530 GENDER PREDICTOR \U0001f530*\n\n")
				fb.WriteString("*\U0001f464 NAME \u276f " + strings.ToUpper(alt.Name) + "*\n")
				fb.WriteString("*\u26a7 GENDER \u276f " + strings.ToUpper(alt.Gender) + "*\n")
				fb.WriteString("*\U0001f4ca PROBABILITY \u276f " + strconv.Itoa(alt.Probability) + "%*")
				s.Reply(info, fb.String())
				return
			}
			if !ctxTimedOut(ctx) {
				funFail(s, info, "GENDERIZE")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 GENDER PREDICTOR 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*⚧ GENDER ❯ " + strings.ToUpper(res.Gender) + "*\n")
		b.WriteString("*📊 PROBABILITY ❯ " + fmt.Sprintf("%.0f%%", res.Probability*100) + "*\n")
		b.WriteString("*🔢 SAMPLES ❯ " + strconv.Itoa(res.Count) + "*")
		s.Reply(info, b.String())
	})
}

// ── .NATIONALIZE ────────────────────────────────────────────────────────────

func nationalizeGuide(prefix string) string {
	return "*🔰 NATIONALITY PREDICTOR 🔰*\n\n" +
		"*PREDICT THE NATIONALITY OF A NAME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NATIONALIZE <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NATIONALIZE NATHANIEL ❯*"
}

func handleNationalize(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.TrimSpace(strings.Join(args, " "))
		if name == "" {
			s.Reply(info, nationalizeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*PREDICTING NATIONALITY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Name    string `json:"name"`
			Country []struct {
				CountryID   string  `json:"country_id"`
				Probability float64 `json:"probability"`
			} `json:"country"`
		}
		u := "https://api.nationalize.io/?name=" + url.QueryEscape(name)
		if err := funGetJSONRetry(ctx, u, &res, 3); err != nil || len(res.Country) == 0 {
			// Fallback: route through the Jina reader proxy (different egress IP)
			// when nationalize rate-limits this host.
			if err2 := funGetJSONViaJina(ctx, u, &res); err2 != nil || len(res.Country) == 0 {
				if !ctxTimedOut(ctx) {
					funFail(s, info, "NATIONALIZE")
				}
				return
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 NATIONALITY PREDICTOR 🔰*\n\n")
		b.WriteString("*👤 NAME ❯ " + strings.ToUpper(res.Name) + "*\n\n")
		limit := len(res.Country)
		if limit > 3 {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			c := res.Country[i]
			b.WriteString("*🌍 " + strings.ToUpper(c.CountryID) + " ❯ " + fmt.Sprintf("%.0f%%", c.Probability*100) + "*\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .MACLOOKUP ──────────────────────────────────────────────────────────────

func maclookupGuide(prefix string) string {
	return "*🔰 MAC VENDOR LOOKUP 🔰*\n\n" +
		"*FIND THE VENDOR OF A MAC ADDRESS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MACLOOKUP <MAC> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MACLOOKUP 44:38:39:FF:EF:57 ❯*"
}

func handleMaclookup(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		mac := strings.TrimSpace(strings.Join(args, ""))
		if mac == "" {
			s.Reply(info, maclookupGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP VENDOR....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		data, err := funGetBytes(ctx, "https://api.macvendors.com/"+url.QueryEscape(mac))
		if err != nil || len(data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 VENDOR NOT FOUND, PLEASE CHECK THE MAC ADDRESS*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 MAC VENDOR LOOKUP 🔰*\n\n")
		b.WriteString("*🔌 MAC ❯ " + strings.ToUpper(mac) + "*\n")
		b.WriteString("*🏢 VENDOR ❯ " + strings.ToUpper(strings.TrimSpace(string(data))) + "*")
		s.Reply(info, b.String())
	})
}

// ── .ZIPCODE ────────────────────────────────────────────────────────────────

func zipcodeGuide(prefix string) string {
	return "*🔰 ZIP CODE INFO 🔰*\n\n" +
		"*GET LOCATION INFO FOR A POSTAL CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ZIPCODE <COUNTRY> <ZIP> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ZIPCODE US 33162 ❯*"
}

func handleZipcode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, zipcodeGuide(prefix))
			return
		}
		country := strings.ToLower(strings.TrimSpace(args[0]))
		zip := strings.TrimSpace(args[1])
		waitID := s.ReplyWithID(info, "*FETCHING ZIP INFO....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Country  string `json:"country"`
			PostCode string `json:"post code"`
			Places   []struct {
				PlaceName string `json:"place name"`
				State     string `json:"state"`
				StateAbbr string `json:"state abbreviation"`
				Latitude  string `json:"latitude"`
				Longitude string `json:"longitude"`
			} `json:"places"`
		}
		u := "https://api.zippopotam.us/" + url.QueryEscape(country) + "/" + url.QueryEscape(zip)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Places) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 ZIP CODE NOT FOUND, PLEASE CHECK THE INPUT*")
			}
			return
		}
		p := res.Places[0]
		var b strings.Builder
		b.WriteString("*🔰 ZIP CODE INFO 🔰*\n\n")
		b.WriteString("*📮 ZIP ❯ " + res.PostCode + "*\n")
		b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(res.Country) + "*\n")
		b.WriteString("*🏙️ PLACE ❯ " + strings.ToUpper(p.PlaceName) + "*\n")
		if p.State != "" {
			b.WriteString("*🗺️ STATE ❯ " + strings.ToUpper(p.State) + "*\n")
		}
		b.WriteString("*📍 LAT ❯ " + p.Latitude + "*\n")
		b.WriteString("*📍 LON ❯ " + p.Longitude + "*")
		s.Reply(info, b.String())
	})
}

// ── .AVATAR ─────────────────────────────────────────────────────────────────

func avatarGuide(prefix string) string {
	return "*🔰 AVATAR GENERATOR 🔰*\n\n" +
		"*GENERATE A RANDOM AVATAR IMAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AVATAR <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "AVATAR GOLDMD ❯*"
}

func handleAvatar(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		seed := strings.TrimSpace(strings.Join(args, " "))
		if seed == "" {
			s.Reply(info, avatarGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*GENERATING AVATAR....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		u := "https://api.dicebear.com/7.x/bottts/png?seed=" + url.QueryEscape(seed) + "&size=512"
		data, err := funGetBytes(ctx, u)
		if err != nil || len(data) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "AVATAR")
			}
			return
		}
		_ = s.SendImage(info, data, "*🔰 AVATAR GENERATOR 🔰*\n\n*🎨 SEED ❯ "+strings.ToUpper(seed)+"*")
	})
}

// ── .JOKEAPI ────────────────────────────────────────────────────────────────

func jokeapiGuide(prefix string) string {
	return "*🔰 RANDOM JOKE 🔰*\n\n" +
		"*GET A RANDOM JOKE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "JOKEAPI ❯*"
}

func handleJokeapi(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING JOKE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Error    bool   `json:"error"`
			Category string `json:"category"`
			Type     string `json:"type"`
			Joke     string `json:"joke"`
			Setup    string `json:"setup"`
			Delivery string `json:"delivery"`
		}
		if err := funGetJSON(ctx, "https://v2.jokeapi.dev/joke/Any?type=single", &res); err != nil || res.Error || res.Joke == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "JOKEAPI")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 RANDOM JOKE 🔰*\n\n")
		b.WriteString("*🏷️ CATEGORY ❯ " + strings.ToUpper(res.Category) + "*\n\n")
		b.WriteString("*" + res.Joke + "*")
		s.Reply(info, b.String())
	})
}

// ── .COINGECKO ──────────────────────────────────────────────────────────────

func coingeckoGuide(prefix string) string {
	return "*🔰 CRYPTO PRICE 🔰*\n\n" +
		"*GET LIVE CRYPTO PRICE FROM COINGECKO*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COINGECKO <COIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COINGECKO BITCOIN ❯*"
}

func handleCoingecko(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		coin := strings.ToLower(strings.TrimSpace(strings.Join(args, "")))
		if coin == "" {
			s.Reply(info, coingeckoGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res map[string]struct {
			USD float64 `json:"usd"`
		}
		u := "https://api.coingecko.com/api/v3/simple/price?ids=" + url.QueryEscape(coin) + "&vs_currencies=usd"
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COINGECKO")
			}
			return
		}
		price, ok := res[coin]
		if !ok || price.USD == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COIN NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 CRYPTO PRICE 🔰*\n\n")
		b.WriteString("*🪙 COIN ❯ " + strings.ToUpper(coin) + "*\n")
		b.WriteString("*💵 PRICE ❯ $" + fmt.Sprintf("%.2f", price.USD) + "*")
		s.Reply(info, b.String())
	})
}

// ── .EXCHANGERATE ───────────────────────────────────────────────────────────

func exchangerateGuide(prefix string) string {
	return "*🔰 EXCHANGE RATE 🔰*\n\n" +
		"*GET THE LIVE CURRENCY EXCHANGE RATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "EXCHANGERATE <FROM> <TO> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "EXCHANGERATE USD PKR ❯*"
}

func handleExchangerate(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, exchangerateGuide(prefix))
			return
		}
		from := strings.ToUpper(strings.TrimSpace(args[0]))
		to := strings.ToUpper(strings.TrimSpace(args[1]))
		waitID := s.ReplyWithID(info, "*FETCHING RATE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Result string             `json:"result"`
			Rates  map[string]float64 `json:"rates"`
		}
		u := "https://open.er-api.com/v6/latest/" + url.QueryEscape(from)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Result != "success" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "EXCHANGERATE")
			}
			return
		}
		rate, ok := res.Rates[to]
		if !ok {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 CURRENCY NOT FOUND, PLEASE CHECK THE CODES*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 EXCHANGE RATE 🔰*\n\n")
		b.WriteString("*💱 FROM ❯ " + from + "*\n")
		b.WriteString("*💱 TO ❯ " + to + "*\n")
		b.WriteString("*📈 RATE ❯ 1 " + from + " = " + fmt.Sprintf("%.4f", rate) + " " + to + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "agify", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE AGE OF A NAME. USE IT AS .AGIFY <NAME>.", Run: handleAgify})
	Register(Command{Name: "genderize", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE GENDER OF A NAME. USE IT AS .GENDERIZE <NAME>.", Run: handleGenderize})
	Register(Command{Name: "nationalize", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PREDICT THE NATIONALITY OF A NAME. USE IT AS .NATIONALIZE <NAME>.", Run: handleNationalize})
	Register(Command{Name: "maclookup", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE VENDOR OF A MAC ADDRESS. USE IT AS .MACLOOKUP <MAC>.", Run: handleMaclookup})
	Register(Command{Name: "zipcode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET LOCATION INFO FOR A POSTAL CODE. USE IT AS .ZIPCODE <COUNTRY> <ZIP>.", Run: handleZipcode})
	Register(Command{Name: "avatar", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A RANDOM AVATAR IMAGE. USE IT AS .AVATAR <TEXT>.", Run: handleAvatar})
	Register(Command{Name: "jokeapi", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM JOKE. USE IT AS .JOKEAPI.", Run: handleJokeapi})
	Register(Command{Name: "coingecko", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET LIVE CRYPTO PRICE FROM COINGECKO. USE IT AS .COINGECKO <COIN>.", Run: handleCoingecko})
	Register(Command{Name: "exchangerate", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE CURRENCY EXCHANGE RATE. USE IT AS .EXCHANGERATE <FROM> <TO>.", Run: handleExchangerate})

	// hidden aliases
	Register(Command{Name: "agepredict", Category: "TOOLS", Desc: "Short alias of .agify", Hidden: true, Run: handleAgify})
	Register(Command{Name: "genderpredict", Category: "TOOLS", Desc: "Short alias of .genderize", Hidden: true, Run: handleGenderize})
	Register(Command{Name: "nationalitypredict", Category: "TOOLS", Desc: "Short alias of .nationalize", Hidden: true, Run: handleNationalize})
	Register(Command{Name: "macvendor", Category: "TOOLS", Desc: "Short alias of .maclookup", Hidden: true, Run: handleMaclookup})
	Register(Command{Name: "postalcode", Category: "TOOLS", Desc: "Short alias of .zipcode", Hidden: true, Run: handleZipcode})
	Register(Command{Name: "dicebear", Category: "TOOLS", Desc: "Short alias of .avatar", Hidden: true, Run: handleAvatar})
	Register(Command{Name: "coinprice", Category: "TOOLS", Desc: "Short alias of .coingecko", Hidden: true, Run: handleCoingecko})
	Register(Command{Name: "forexrate", Category: "TOOLS", Desc: "Short alias of .exchangerate", Hidden: true, Run: handleExchangerate})
}
