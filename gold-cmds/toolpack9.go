package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 9 (10 new everyday commands)
// File: toolpack9.go
// ============================================================================
//   .launch       -> next upcoming space rocket launch
//   .forecast     -> live weather forecast for a city
//   .art <query>  -> artwork from the Art Institute of Chicago
//   .met <query>  -> artwork from the Metropolitan Museum of Art
//   .bible <ref>  -> Bible verse (e.g. John 3:16)
//   .spaceflight  -> latest space news article
//   .blockchain   -> live Bitcoin blockchain stats
//   .wazirx <sym> -> live crypto price from WazirX (e.g. btcusdt)
//   .daylight     -> sunrise / sunset / day length for a city
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

// ── .LAUNCH ─────────────────────────────────────────────────────────────────

func launchGuide(prefix string) string {
	return "*🔰 SPACE LAUNCH 🔰*\n\n" +
		"*GET THE NEXT UPCOMING ROCKET LAUNCH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LAUNCH ❯*"
}

func handleLaunch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING LAUNCH....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Results []struct {
				Name   string `json:"name"`
				Net    string `json:"net"`
				Status struct {
					Abbrev string `json:"abbrev"`
				} `json:"status"`
				LaunchServiceProvider struct {
					Name string `json:"name"`
				} `json:"launch_service_provider"`
				Rocket struct {
					Configuration struct {
						FullName string `json:"full_name"`
					} `json:"configuration"`
				} `json:"rocket"`
				Pad struct {
					Name     string `json:"name"`
					Location struct {
						Name string `json:"name"`
					} `json:"location"`
				} `json:"pad"`
			} `json:"results"`
		}
		if err := funGetJSON(ctx, "https://lldev.thespacedevs.com/2.2.0/launch/upcoming/?limit=1", &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "LAUNCH")
			}
			return
		}
		r := res.Results[0]
		var b strings.Builder
		b.WriteString("*🔰 NEXT SPACE LAUNCH 🔰*\n\n")
		b.WriteString("*🚀 MISSION ❯ " + strings.ToUpper(r.Name) + "*\n")
		if r.Rocket.Configuration.FullName != "" {
			b.WriteString("*🛠️ ROCKET ❯ " + strings.ToUpper(r.Rocket.Configuration.FullName) + "*\n")
		}
		if r.LaunchServiceProvider.Name != "" {
			b.WriteString("*🏢 PROVIDER ❯ " + strings.ToUpper(r.LaunchServiceProvider.Name) + "*\n")
		}
		if r.Pad.Location.Name != "" {
			b.WriteString("*📍 LOCATION ❯ " + strings.ToUpper(r.Pad.Location.Name) + "*\n")
		}
		b.WriteString("*⏰ TIME ❯ " + r.Net + "*\n")
		b.WriteString("*📌 STATUS ❯ " + strings.ToUpper(r.Status.Abbrev) + "*")
		s.Reply(info, b.String())
	})
}

// ── .FORECAST ───────────────────────────────────────────────────────────────

func forecastGuide(prefix string) string {
	return "*🔰 WEATHER FORECAST 🔰*\n\n" +
		"*GET THE LIVE WEATHER FORECAST FOR A CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "FORECAST <CITY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "FORECAST LONDON ❯*"
}

func handleForecast(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		city := strings.TrimSpace(strings.Join(args, " "))
		if city == "" {
			s.Reply(info, forecastGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING FORECAST....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			CurrentCondition []struct {
				TempC       string `json:"temp_C"`
				FeelsLikeC  string `json:"FeelsLikeC"`
				Humidity    string `json:"humidity"`
				WindSpeed   string `json:"windspeedKmph"`
				WindDir     string `json:"winddir16Point"`
				WeatherDesc []struct {
					Value string `json:"value"`
				} `json:"weatherDesc"`
			} `json:"current_condition"`
			Weather []struct {
				Date      string `json:"date"`
				MaxtempC  string `json:"maxtempC"`
				MintempC  string `json:"mintempC"`
				Astronomy []struct {
					Sunrise string `json:"sunrise"`
					Sunset  string `json:"sunset"`
				} `json:"astronomy"`
			} `json:"weather"`
			NearestArea []struct {
				AreaName []struct {
					Value string `json:"value"`
				} `json:"areaName"`
				Country []struct {
					Value string `json:"value"`
				} `json:"country"`
			} `json:"nearest_area"`
		}
		u := "https://wttr.in/" + url.QueryEscape(city) + "?format=j1"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.CurrentCondition) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "FORECAST")
			}
			return
		}
		cc := res.CurrentCondition[0]
		desc := ""
		if len(cc.WeatherDesc) > 0 {
			desc = strings.TrimSpace(cc.WeatherDesc[0].Value)
		}
		place := strings.ToUpper(city)
		if len(res.NearestArea) > 0 && len(res.NearestArea[0].AreaName) > 0 {
			place = strings.ToUpper(res.NearestArea[0].AreaName[0].Value)
			if len(res.NearestArea[0].Country) > 0 {
				place += ", " + strings.ToUpper(res.NearestArea[0].Country[0].Value)
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 WEATHER FORECAST 🔰*\n\n")
		b.WriteString("*📍 " + place + "*\n\n")
		b.WriteString("*🌡️ TEMP ❯ " + cc.TempC + "°C*\n")
		b.WriteString("*🤔 FEELS LIKE ❯ " + cc.FeelsLikeC + "°C*\n")
		if desc != "" {
			b.WriteString("*☁️ CONDITION ❯ " + strings.ToUpper(desc) + "*\n")
		}
		b.WriteString("*💧 HUMIDITY ❯ " + cc.Humidity + "%*\n")
		b.WriteString("*🌬️ WIND ❯ " + cc.WindSpeed + " KM/H " + strings.ToUpper(cc.WindDir) + "*")
		if len(res.Weather) > 0 {
			w := res.Weather[0]
			b.WriteString("\n\n*📅 TODAY ❯ " + w.Date + "*\n")
			b.WriteString("*🔺 MAX ❯ " + w.MaxtempC + "°C*\n")
			b.WriteString("*🔻 MIN ❯ " + w.MintempC + "°C*")
			if len(w.Astronomy) > 0 {
				b.WriteString("\n*🌅 SUNRISE ❯ " + w.Astronomy[0].Sunrise + "*\n")
				b.WriteString("*🌇 SUNSET ❯ " + w.Astronomy[0].Sunset + "*")
			}
		}
		s.Reply(info, b.String())
	})
}

// ── .ART ────────────────────────────────────────────────────────────────────

func artGuide(prefix string) string {
	return "*🔰 ART SEARCH 🔰*\n\n" +
		"*FIND ARTWORK FROM THE ART INSTITUTE OF CHICAGO*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ART <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ART MONET ❯*"
}

func handleArt(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, artGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING ART....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data []struct {
				ID            int    `json:"id"`
				Title         string `json:"title"`
				ArtistDisplay string `json:"artist_display"`
				DateDisplay   string `json:"date_display"`
				ImageID       string `json:"image_id"`
			} `json:"data"`
			Config struct {
				IIIFURL string `json:"iiif_url"`
			} `json:"config"`
		}
		u := "https://api.artic.edu/api/v1/artworks/search?limit=1&fields=id,title,artist_display,date_display,image_id&q=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO ARTWORK FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		a := res.Data[0]
		var b strings.Builder
		b.WriteString("*🔰 ART FOUND 🔰*\n\n")
		b.WriteString("*🖼️ TITLE ❯ " + strings.ToUpper(a.Title) + "*\n")
		if a.ArtistDisplay != "" {
			ad := strings.ReplaceAll(a.ArtistDisplay, "\n", " ")
			b.WriteString("*🎨 ARTIST ❯ " + strings.ToUpper(ad) + "*\n")
		}
		if a.DateDisplay != "" {
			b.WriteString("*📅 DATE ❯ " + a.DateDisplay + "*")
		}
		if a.ImageID != "" && res.Config.IIIFURL != "" {
			imgURL := res.Config.IIIFURL + "/" + a.ImageID + "/full/843,/0/default.jpg"
			if data, err := funGetBytes(ctx, imgURL); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .MET ────────────────────────────────────────────────────────────────────

func metGuide(prefix string) string {
	return "*🔰 MET MUSEUM 🔰*\n\n" +
		"*FIND ARTWORK FROM THE METROPOLITAN MUSEUM OF ART*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MET <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MET SUNFLOWERS ❯*"
}

func handleMet(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, metGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING MET....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var search struct {
			ObjectIDs []int `json:"objectIDs"`
		}
		u := "https://collectionapi.metmuseum.org/public/collection/v1/search?q=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &search); err != nil || len(search.ObjectIDs) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO ARTWORK FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		var obj struct {
			Title             string `json:"title"`
			ArtistDisplayName string `json:"artistDisplayName"`
			ObjectDate        string `json:"objectDate"`
			Medium            string `json:"medium"`
			PrimaryImage      string `json:"primaryImage"`
		}
		u2 := "https://collectionapi.metmuseum.org/public/collection/v1/objects/" + strconv.Itoa(search.ObjectIDs[0])
		if err := funGetJSON(ctx, u2, &obj); err != nil || obj.Title == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "MET")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 MET MUSEUM 🔰*\n\n")
		b.WriteString("*🖼️ TITLE ❯ " + strings.ToUpper(obj.Title) + "*\n")
		if obj.ArtistDisplayName != "" {
			b.WriteString("*🎨 ARTIST ❯ " + strings.ToUpper(obj.ArtistDisplayName) + "*\n")
		}
		if obj.ObjectDate != "" {
			b.WriteString("*📅 DATE ❯ " + obj.ObjectDate + "*\n")
		}
		if obj.Medium != "" {
			b.WriteString("*🖌️ MEDIUM ❯ " + strings.ToUpper(obj.Medium) + "*")
		}
		if obj.PrimaryImage != "" {
			if data, err := funGetBytes(ctx, obj.PrimaryImage); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .BIBLE ──────────────────────────────────────────────────────────────────

func bibleGuide(prefix string) string {
	return "*🔰 BIBLE VERSE 🔰*\n\n" +
		"*GET ANY BIBLE VERSE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BIBLE <REFERENCE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BIBLE JOHN 3:16 ❯*"
}

func handleBible(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		ref := strings.TrimSpace(strings.Join(args, " "))
		if ref == "" {
			s.Reply(info, bibleGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING VERSE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Reference string `json:"reference"`
			Text      string `json:"text"`
		}
		u := "https://bible-api.com/" + url.QueryEscape(ref)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Text == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 VERSE NOT FOUND, PLEASE CHECK THE REFERENCE*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 BIBLE VERSE 🔰*\n\n")
		b.WriteString("*📖 " + strings.ToUpper(res.Reference) + "*\n\n")
		b.WriteString("*\"" + strings.TrimSpace(res.Text) + "\"*")
		s.Reply(info, b.String())
	})
}

// ── .SPACEFLIGHT ────────────────────────────────────────────────────────────

func spaceflightGuide(prefix string) string {
	return "*🔰 SPACE NEWS 🔰*\n\n" +
		"*GET THE LATEST SPACE NEWS ARTICLE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SPACEFLIGHT ❯*"
}

func handleSpaceflight(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING SPACE NEWS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				ImageURL    string `json:"image_url"`
				NewsSite    string `json:"news_site"`
				PublishedAt string `json:"published_at"`
				Summary     string `json:"summary"`
			} `json:"results"`
		}
		if err := funGetJSON(ctx, "https://api.spaceflightnewsapi.net/v4/articles/?limit=1", &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SPACEFLIGHT")
			}
			return
		}
		a := res.Results[0]
		sum := a.Summary
		if len(sum) > 500 {
			sum = sum[:500] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 SPACE NEWS 🔰*\n\n")
		b.WriteString("*📰 " + strings.ToUpper(a.Title) + "*\n")
		b.WriteString("*🏢 SOURCE ❯ " + strings.ToUpper(a.NewsSite) + "*\n")
		b.WriteString("*📅 " + a.PublishedAt + "*\n\n")
		b.WriteString("*" + sum + "*\n\n")
		b.WriteString("*🔗 " + a.URL + "*")
		if a.ImageURL != "" {
			if data, err := funGetBytes(ctx, a.ImageURL); err == nil {
				_ = s.SendImage(info, data, b.String())
				return
			}
		}
		s.Reply(info, b.String())
	})
}

// ── .BLOCKCHAIN ─────────────────────────────────────────────────────────────

func blockchainGuide(prefix string) string {
	return "*🔰 BITCOIN STATS 🔰*\n\n" +
		"*GET LIVE BITCOIN BLOCKCHAIN STATISTICS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BLOCKCHAIN ❯*"
}

func handleBlockchain(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING BLOCKCHAIN STATS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			MarketPriceUSD float64 `json:"market_price_usd"`
			HashRate       float64 `json:"hash_rate"`
			TotalBC        float64 `json:"totalbc"`
			NBlocksTotal   int     `json:"n_blocks_total"`
			NTx            int     `json:"n_tx"`
			TotalFeesBTC   float64 `json:"total_fees_btc"`
		}
		if err := funGetJSON(ctx, "https://api.blockchain.info/stats", &res); err != nil || res.MarketPriceUSD == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "BLOCKCHAIN")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 BITCOIN STATS 🔰*\n\n")
		b.WriteString("*💰 PRICE ❯ $" + fmt.Sprintf("%.2f", res.MarketPriceUSD) + "*\n")
		b.WriteString("*⛏️ HASH RATE ❯ " + fmt.Sprintf("%.0f GH/S", res.HashRate/1e9) + "*\n")
		b.WriteString("*🪙 TOTAL MINED ❯ " + fmt.Sprintf("%.0f BTC", res.TotalBC/1e8) + "*\n")
		b.WriteString("*🧱 BLOCK HEIGHT ❯ " + strconv.Itoa(res.NBlocksTotal) + "*\n")
		b.WriteString("*📊 TOTAL TXS ❯ " + strconv.Itoa(res.NTx) + "*")
		s.Reply(info, b.String())
	})
}

// ── .WAZIRX ─────────────────────────────────────────────────────────────────

func wazirxGuide(prefix string) string {
	return "*🔰 WAZIRX PRICE 🔰*\n\n" +
		"*GET LIVE CRYPTO PRICE FROM WAZIRX*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WAZIRX <SYMBOL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WAZIRX BTCUSDT ❯*"
}

func handleWazirx(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		sym := strings.ToUpper(strings.TrimSpace(strings.Join(args, "")))
		if sym == "" {
			s.Reply(info, wazirxGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Symbol             string `json:"symbol"`
			LastPrice          string `json:"lastPrice"`
			OpenPrice          string `json:"openPrice"`
			HighPrice          string `json:"highPrice"`
			LowPrice           string `json:"lowPrice"`
			Volume             string `json:"volume"`
			PriceChangePercent string `json:"priceChangePercent"`
		}
		u := "https://api.wazirx.com/sapi/v1/ticker/24hr?symbol=" + url.QueryEscape(strings.ToLower(sym))
		if err := funGetJSON(ctx, u, &res); err != nil || res.LastPrice == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 SYMBOL NOT FOUND, PLEASE CHECK THE SYMBOL*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 WAZIRX PRICE 🔰*\n\n")
		b.WriteString("*💱 SYMBOL ❯ " + strings.ToUpper(res.Symbol) + "*\n")
		b.WriteString("*💰 LAST PRICE ❯ " + res.LastPrice + "*\n")
		b.WriteString("*📈 HIGH ❯ " + res.HighPrice + "*\n")
		b.WriteString("*📉 LOW ❯ " + res.LowPrice + "*\n")
		b.WriteString("*📊 CHANGE ❯ " + res.PriceChangePercent + "%*")
		s.Reply(info, b.String())
	})
}

// ── .DAYLIGHT ───────────────────────────────────────────────────────────────

func daylightGuide(prefix string) string {
	return "*🔰 DAYLIGHT INFO 🔰*\n\n" +
		"*GET SUNRISE, SUNSET AND DAY LENGTH FOR A CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DAYLIGHT <LAT> <LON> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DAYLIGHT 31.5 74.3 ❯*"
}

func handleDaylight(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, daylightGuide(prefix))
			return
		}
		lat, err1 := strconv.ParseFloat(args[0], 64)
		lon, err2 := strconv.ParseFloat(args[1], 64)
		if err1 != nil || err2 != nil {
			s.Reply(info, daylightGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CALCULATING DAYLIGHT....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Results struct {
				Sunrise   string `json:"sunrise"`
				Sunset    string `json:"sunset"`
				SolarNoon string `json:"solar_noon"`
				DayLength string `json:"day_length"`
			} `json:"results"`
			Status string `json:"status"`
		}
		u := "https://api.sunrise-sunset.org/json?lat=" + url.QueryEscape(args[0]) + "&lng=" + url.QueryEscape(args[1])
		if err := funGetJSON(ctx, u, &res); err != nil || res.Status != "OK" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DAYLIGHT")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DAYLIGHT INFO 🔰*\n\n")
		b.WriteString("*📍 LAT ❯ " + fmt.Sprintf("%.4f", lat) + "*\n")
		b.WriteString("*📍 LON ❯ " + fmt.Sprintf("%.4f", lon) + "*\n")
		b.WriteString("*🌅 SUNRISE ❯ " + res.Results.Sunrise + " UTC*\n")
		b.WriteString("*🌇 SUNSET ❯ " + res.Results.Sunset + " UTC*\n")
		b.WriteString("*☀️ SOLAR NOON ❯ " + res.Results.SolarNoon + " UTC*\n")
		b.WriteString("*⏳ DAY LENGTH ❯ " + res.Results.DayLength + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "launch", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE NEXT UPCOMING SPACE ROCKET LAUNCH. USE IT AS .LAUNCH.", Run: handleLaunch})
	Register(Command{Name: "forecast", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE WEATHER FORECAST FOR A CITY. USE IT AS .FORECAST <CITY>.", Run: handleForecast})
	Register(Command{Name: "art", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND ARTWORK FROM THE ART INSTITUTE OF CHICAGO. USE IT AS .ART <QUERY>.", Run: handleArt})
	Register(Command{Name: "met", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND ARTWORK FROM THE METROPOLITAN MUSEUM OF ART. USE IT AS .MET <QUERY>.", Run: handleMet})
	Register(Command{Name: "bible", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET ANY BIBLE VERSE. USE IT AS .BIBLE <REFERENCE>.", Run: handleBible})
	Register(Command{Name: "spaceflight", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LATEST SPACE NEWS ARTICLE. USE IT AS .SPACEFLIGHT.", Run: handleSpaceflight})
	Register(Command{Name: "blockchain", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET LIVE BITCOIN BLOCKCHAIN STATISTICS. USE IT AS .BLOCKCHAIN.", Run: handleBlockchain})
	Register(Command{Name: "wazirx", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET LIVE CRYPTO PRICE FROM WAZIRX. USE IT AS .WAZIRX <SYMBOL>.", Run: handleWazirx})
	Register(Command{Name: "daylight", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET SUNRISE, SUNSET AND DAY LENGTH FOR A LOCATION. USE IT AS .DAYLIGHT <LAT> <LON>.", Run: handleDaylight})

	// hidden aliases
	Register(Command{Name: "rocketlaunch", Category: "TOOLS", Desc: "Short alias of .launch", Hidden: true, Run: handleLaunch})
	Register(Command{Name: "weathernow", Category: "TOOLS", Desc: "Short alias of .forecast", Hidden: true, Run: handleForecast})
	Register(Command{Name: "artwork", Category: "TOOLS", Desc: "Short alias of .art", Hidden: true, Run: handleArt})
	Register(Command{Name: "metmuseum", Category: "TOOLS", Desc: "Short alias of .met", Hidden: true, Run: handleMet})
	Register(Command{Name: "verse", Category: "TOOLS", Desc: "Short alias of .bible", Hidden: true, Run: handleBible})
	Register(Command{Name: "btcstats", Category: "TOOLS", Desc: "Short alias of .blockchain", Hidden: true, Run: handleBlockchain})
	Register(Command{Name: "suninfo", Category: "TOOLS", Desc: "Short alias of .daylight", Hidden: true, Run: handleDaylight})
}
