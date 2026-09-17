package goldcmds

// ============================================================================
// GOLD-MD — FUN PACK (12 small working commands)
// File: funpack.go
// ============================================================================
// A bundle of lightweight, always-working utility / fun commands. Every one
// uses a free public API (no key) and matches the GOLD-MD design language
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
//
//   .qr <text>        -> QR code image
//   .weather <city>   -> live weather
//   .wiki <query>     -> Wikipedia summary
//   .joke             -> random joke
//   .fact             -> random useless fact
//   .quote            -> random quote
//   .shorten <url>    -> short URL (tinyurl)  [aliases: tiny, shorturl, urltiny, smalllink, smallurl, shortlink, tinyurl]
//   .crypto <coin>    -> live crypto price (USD + PKR)
//   .ip <ip|empty>    -> IP geolocation lookup
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// funUA is the browser-like User-Agent used for all fun-pack requests.
const funUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// funGetBytes performs a GET and returns the raw body (capped at 10MB).
func funGetBytes(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", funUA)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	return data, nil
}

// funGetBytesNoGzip is like funGetBytes but explicitly disables gzip. Some
// APIs (e.g. Jikan / MyAnimeList) return HTTP 504 when the client advertises
// Accept-Encoding: gzip, which Go's http.Client does automatically. Sending
// "Accept-Encoding: identity" avoids that and makes those APIs reliable.
func funGetBytesNoGzip(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", funUA)
	// DisableCompression stops Go from adding "Accept-Encoding: gzip"; we also
	// never set the header ourselves, so NO Accept-Encoding is sent at all.
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DisableCompression: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	return data, nil
}

// funGetJSONNoGzip performs a no-gzip GET and unmarshals the JSON body into v.
func funGetJSONNoGzip(ctx context.Context, rawURL string, v any) error {
	data, err := funGetBytesNoGzip(ctx, rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// funGetJSON performs a GET and unmarshals the JSON body into v.
func funGetJSON(ctx context.Context, rawURL string, v any) error {
	data, err := funGetBytes(ctx, rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// funPostJSON performs a POST with a JSON body and unmarshals the JSON
// response into v. Used for GraphQL APIs such as AniList.
func funPostJSON(ctx context.Context, rawURL string, body []byte, v any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", rawURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", funUA)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty response")
	}
	return json.Unmarshal(data, v)
}

// funFail sends the standard failure line.
func funFail(s SessionBridge, info types.MessageInfo, what string) {
	s.Reply(info, "*🔰 "+what+" FAILED, PLEASE TRY AGAIN*")
}

// ============================================================================
// .QR — QR CODE
// ============================================================================

func qrGuide(prefix string) string {
	return "*🔰 QR CODE MAKER 🔰*\n\n" +
		"*TURN ANY TEXT OR LINK INTO A QR CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "QR <TEXT OR LINK> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "QR HTTPS://GITHUB.COM ❯*"
}

func handleQR(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.TrimSpace(strings.Join(args, " "))
		if text == "" {
			s.Reply(info, qrGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*MAKING QR CODE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.qrserver.com/v1/create-qr-code/?size=512x512&margin=10&data=" + url.QueryEscape(text)
		img, err := funGetBytes(ctx, u)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "QR CODE")
			}
			return
		}
		if err := s.SendImage(info, img, "*🔰 QR CODE 🔰*\n\n*"+text+"*"); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "QR CODE")
			}
		}
	})
}

// ============================================================================
// .WEATHER — LIVE WEATHER
// ============================================================================

func weatherGuide(prefix string) string {
	return "*🔰 WEATHER 🔰*\n\n" +
		"*GET LIVE WEATHER OF ANY CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WEATHER <CITY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WEATHER LAHORE ❯*"
}

func handleWeather(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		city := strings.TrimSpace(strings.Join(args, " "))
		if city == "" {
			s.Reply(info, weatherGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING WEATHER....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://wttr.in/" + url.PathEscape(city) + "?format=j1"
		var res struct {
			Current []struct {
				TempC      string `json:"temp_C"`
				TempF      string `json:"temp_F"`
				FeelsC     string `json:"FeelsLikeC"`
				FeelsF     string `json:"FeelsLikeF"`
				Desc       []struct {
					Value string `json:"value"`
				} `json:"weatherDesc"`
				WindKmph   string `json:"windspeedKmph"`
				WindDir    string `json:"winddir16Point"`
				Humidity   string `json:"humidity"`
				Visibility string `json:"visibility"`
				UVIndex    string `json:"uvIndex"`
				ObsTime    string `json:"observation_time"`
			} `json:"current_condition"`
			Nearest []struct {
				AreaName []struct {
					Value string `json:"value"`
				} `json:"areaName"`
				Region []struct {
					Value string `json:"value"`
				} `json:"region"`
				Country []struct {
					Value string `json:"value"`
				} `json:"country"`
			} `json:"nearest_area"`
			Weather []struct {
				Astronomy []struct {
					Sunrise string `json:"sunrise"`
					Sunset  string `json:"sunset"`
				} `json:"astronomy"`
			} `json:"weather"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Current) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "WEATHER")
			}
			return
		}
		c := res.Current[0]
		desc := ""
		if len(c.Desc) > 0 {
			desc = c.Desc[0].Value
		}
		area, region, country := "", "", ""
		if len(res.Nearest) > 0 {
			n := res.Nearest[0]
			if len(n.AreaName) > 0 {
				area = n.AreaName[0].Value
			}
			if len(n.Region) > 0 {
				region = n.Region[0].Value
			}
			if len(n.Country) > 0 {
				country = n.Country[0].Value
			}
		}
		place := strings.Trim(strings.Join([]string{area, region, country}, ", "), ", ")
		if place == "" {
			place = city
		}
		sunrise, sunset := "", ""
		if len(res.Weather) > 0 && len(res.Weather[0].Astronomy) > 0 {
			sunrise = res.Weather[0].Astronomy[0].Sunrise
			sunset = res.Weather[0].Astronomy[0].Sunset
		}
		comfort := "🥶 COLD"
		if t, err := strconv.Atoi(c.TempC); err == nil {
			switch {
			case t >= 40:
				comfort = "🔥 EXTREME HEAT"
			case t >= 35:
				comfort = "🔥 VERY HOT"
			case t >= 28:
				comfort = "☀️ HOT"
			case t >= 20:
				comfort = "😊 PLEASANT"
			case t >= 10:
				comfort = "🧥 COOL"
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 WEATHER 🔰*\n\n")
		b.WriteString("*📍 CITY ❯ " + strings.ToUpper(place) + "*\n")
		b.WriteString("*🕒 TIME ❯ " + c.ObsTime + "*\n")
		b.WriteString("*🌡️ TEMP ❯ " + c.TempC + "°C (" + c.TempF + "°F)*\n")
		b.WriteString("*🤗 FEELS LIKE ❯ " + c.FeelsC + "°C (" + c.FeelsF + "°F)*\n")
		b.WriteString("*☁️ CONDITION ❯ " + strings.ToUpper(desc) + "*\n")
		b.WriteString("*💨 WIND ❯ " + c.WindKmph + " KM/H " + c.WindDir + "*\n")
		b.WriteString("*💧 HUMIDITY ❯ " + c.Humidity + "%*\n")
		b.WriteString("*🔆 UV INDEX ❯ " + c.UVIndex + "*\n")
		b.WriteString("*👁️ VISIBILITY ❯ " + c.Visibility + " KM*\n")
		b.WriteString("*🌅 SUNRISE ❯ " + sunrise + "*\n")
		b.WriteString("*🌇 SUNSET ❯ " + sunset + "*\n")
		b.WriteString("*🎯 FEEL ❯ " + comfort + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .WIKI — WIKIPEDIA SUMMARY
// ============================================================================

func wikiGuide(prefix string) string {
	return "*🔰 WIKIPEDIA 🔰*\n\n" +
		"*GET A QUICK SUMMARY OF ANY TOPIC*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WIKI <TOPIC> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WIKI WHATSAPP ❯*"
}

func handleWiki(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, wikiGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING WIKIPEDIA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(strings.ReplaceAll(q, " ", "_"))
		var res struct {
			Title       string `json:"title"`
			Extract     string `json:"extract"`
			Description string `json:"description"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || strings.TrimSpace(res.Extract) == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO WIKIPEDIA RESULT FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 WIKIPEDIA 🔰*\n\n")
		b.WriteString("*" + strings.ToUpper(res.Title) + "*\n")
		if res.Description != "" {
			b.WriteString("_" + res.Description + "_\n")
		}
		b.WriteString("\n" + res.Extract)
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .JOKE — RANDOM JOKE
// ============================================================================

func handleJoke(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var res struct {
			Setup      string `json:"setup"`
			Punchline  string `json:"punchline"`
			Joke       string `json:"joke"`
			Type       string `json:"type"`
		}
		if err := funGetJSON(ctx, "https://official-joke-api.appspot.com/random_joke", &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "JOKE")
			}
			return
		}
		if res.Setup != "" {
			s.Reply(info, "*🔰 RANDOM JOKE 🔰*\n\n*"+res.Setup+"*\n\n*"+res.Punchline+"*")
			return
		}
		s.Reply(info, "*🔰 RANDOM JOKE 🔰*\n\n*"+res.Joke+"*")
	})
}

// ============================================================================
// .FACT — RANDOM FACT
// ============================================================================

func handleFact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var res struct {
			Text string `json:"text"`
		}
		if err := funGetJSON(ctx, "https://uselessfacts.jsph.pl/api/v2/facts/random?language=en", &res); err != nil || res.Text == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "FACT")
			}
			return
		}
		s.Reply(info, "*🔰 RANDOM FACT 🔰*\n\n*"+res.Text+"*")
	})
}

// ============================================================================
// .QUOTE — RANDOM QUOTE
// ============================================================================

func handleQuote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var res []struct {
			Q string `json:"q"`
			A string `json:"a"`
		}
		if err := funGetJSON(ctx, "https://zenquotes.io/api/random", &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "QUOTE")
			}
			return
		}
		s.Reply(info, "*🔰 RANDOM QUOTE 🔰*\n\n*"+res[0].Q+"*\n\n*— "+res[0].A+"*")
	})
}

// ============================================================================
// .DADJOKE — RANDOM DAD JOKE

// ============================================================================
// .CAT — RANDOM CAT PHOTO

// ============================================================================
// .DOG — RANDOM DOG PHOTO

// ============================================================================
// .SHORTEN — URL SHORTENER
// ============================================================================

func shortenGuide(prefix string) string {
	return "*🔰 URL SHORTENER 🔰*\n\n" +
		"*MAKE ANY LONG LINK SHORT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TINY <LINK> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TINY HTTPS://FACEBOOK.COM/sjbdnnd2eekekdi ❯*"
}

func handleShorten(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		link := strings.TrimSpace(strings.Join(args, " "))
		if link == "" {
			s.Reply(info, shortenGuide(prefix))
			return
		}
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			link = "https://" + link
		}
		waitID := s.ReplyWithID(info, "*SHORTENING LINK....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		data, err := funGetBytes(ctx, "https://tinyurl.com/api-create.php?url="+url.QueryEscape(link))
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SHORTEN")
			}
			return
		}
		short := strings.TrimSpace(string(data))
		if short == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SHORTEN")
			}
			return
		}
		s.Reply(info, "*🔰 URL SHORTENER 🔰*\n\n*ORIGINAL:*\n"+link+"\n\n*SHORT:*\n"+short)
	})
}

// ============================================================================
// .CRYPTO — LIVE CRYPTO PRICE
// ============================================================================

// cryptoAliases maps common names to CoinGecko ids.
var cryptoAliases = map[string]string{
	"btc": "bitcoin", "bitcoin": "bitcoin",
	"eth": "ethereum", "ethereum": "ethereum",
	"bnb": "binancecoin", "binance": "binancecoin",
	"sol": "solana", "solana": "solana",
	"xrp": "ripple", "ripple": "ripple",
	"ada": "cardano", "cardano": "cardano",
	"doge": "dogecoin", "dogecoin": "dogecoin",
	"matic": "matic-network", "polygon": "matic-network",
	"dot": "polkadot", "polkadot": "polkadot",
	"ltc": "litecoin", "litecoin": "litecoin",
	"trx": "tron", "tron": "tron",
	"shib": "shiba-inu", "shiba": "shiba-inu",
}

func cryptoGuide(prefix string) string {
	return "*🔰 CRYPTO PRICE 🔰*\n\n" +
		"*GET LIVE PRICE OF ANY COIN (USD + PKR)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CRYPTO <COIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CRYPTO BTC ❯*\n\n" +
		"*SUPPORTED:* BTC, ETH, BNB, SOL, XRP, ADA, DOGE, MATIC, DOT, LTC, TRX, SHIB"
}

func handleCrypto(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		tok := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		if tok == "" {
			s.Reply(info, cryptoGuide(prefix))
			return
		}
		id, ok := cryptoAliases[tok]
		if !ok {
			id = tok // allow raw coingecko id
		}
		waitID := s.ReplyWithID(info, "*FETCHING PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.coingecko.com/api/v3/simple/price?ids=" + url.QueryEscape(id) + "&vs_currencies=usd,pkr"
		var res map[string]map[string]float64
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "CRYPTO PRICE")
			}
			return
		}
		prices, ok := res[id]
		if !ok || len(prices) == 0 {
			s.Reply(info, "*🔰 COIN NOT FOUND, PLEASE CHECK THE NAME*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 CRYPTO PRICE 🔰*\n\n")
		b.WriteString("*" + strings.ToUpper(id) + "*\n")
		if v, ok := prices["usd"]; ok {
			b.WriteString("*USD ❯ $" + fmt.Sprintf("%.4f", v) + "*\n")
		}
		if v, ok := prices["pkr"]; ok {
			b.WriteString("*PKR ❯ ₨ " + fmt.Sprintf("%.2f", v) + "*")
		}
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .IP — IP GEOLOCATION
// ============================================================================

func ipGuide(prefix string) string {
	return "*🔰 IP LOOKUP 🔰*\n\n" +
		"*GET DETAILS OF ANY IP ADDRESS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "IP <IP ADDRESS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "IP 8.8.8.8 ❯*\n\n" +
		"*LEAVE EMPTY TO LOOK UP YOUR OWN IP*"
}

func handleIP(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		ip := strings.TrimSpace(strings.Join(args, " "))
		if ip == "" {
			s.Reply(info, ipGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP IP....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var res struct {
			Status      string `json:"status"`
			Query       string `json:"query"`
			Country     string `json:"country"`
			RegionName  string `json:"regionName"`
			City        string `json:"city"`
			ISP         string `json:"isp"`
			Org         string `json:"org"`
			Timezone    string `json:"timezone"`
		}
		if err := funGetJSON(ctx, "http://ip-api.com/json/"+url.PathEscape(ip), &res); err != nil || res.Status != "success" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID IP OR LOOKUP FAILED*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 IP LOOKUP 🔰*\n\n")
		b.WriteString("*IP ❯ " + res.Query + "*\n")
		b.WriteString("*COUNTRY ❯ " + res.Country + "*\n")
		b.WriteString("*REGION ❯ " + res.RegionName + "*\n")
		b.WriteString("*CITY ❯ " + res.City + "*\n")
		b.WriteString("*ISP ❯ " + res.ISP + "*\n")
		b.WriteString("*ORG ❯ " + res.Org + "*\n")
		b.WriteString("*TIMEZONE ❯ " + res.Timezone + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// REGISTRATION
// ============================================================================

func init() {
	Register(Command{Name: "qr", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO MAKE A QR CODE FROM ANY TEXT OR LINK. USE IT AS .QR <TEXT OR LINK>.", Run: handleQR})
	Register(Command{Name: "weather", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE WEATHER OF ANY CITY. USE IT AS .WEATHER <CITY>.", Run: handleWeather})
	Register(Command{Name: "wiki", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A QUICK WIKIPEDIA SUMMARY OF ANY TOPIC. USE IT AS .WIKI <TOPIC>.", Run: handleWiki})
	Register(Command{Name: "joke", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM JOKE. JUST TYPE .JOKE.", Run: handleJoke})
	Register(Command{Name: "fact", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM INTERESTING FACT. JUST TYPE .FACT.", Run: handleFact})
	Register(Command{Name: "quote", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM MOTIVATIONAL QUOTE. JUST TYPE .QUOTE.", Run: handleQuote})
	Register(Command{Name: "shorten", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHORTEN ANY LONG LINK. USE IT AS .SHORTEN <LINK>.", Run: handleShorten})
	Register(Command{Name: "crypto", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE PRICE OF ANY CRYPTO COIN IN USD AND PKR. USE IT AS .CRYPTO <COIN>.", Run: handleCrypto})
	Register(Command{Name: "ip", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO LOOK UP THE DETAILS OF ANY IP ADDRESS. USE IT AS .IP <IP ADDRESS>.", Run: handleIP})

	// aliases (Hidden)
	Register(Command{Name: "qrcode", Hidden: true, Run: handleQR})
	Register(Command{Name: "wthr", Hidden: true, Run: handleWeather})
	Register(Command{Name: "wikipedia", Hidden: true, Run: handleWiki})
	Register(Command{Name: "tiny", Hidden: true, Run: handleShorten})
	Register(Command{Name: "shorturl", Hidden: true, Run: handleShorten})
	Register(Command{Name: "urltiny", Hidden: true, Run: handleShorten})
	Register(Command{Name: "smalllink", Hidden: true, Run: handleShorten})
	Register(Command{Name: "smallurl", Hidden: true, Run: handleShorten})
	Register(Command{Name: "shortlink", Hidden: true, Run: handleShorten})
	Register(Command{Name: "tinyurl", Hidden: true, Run: handleShorten})
	Register(Command{Name: "ipinfo", Hidden: true, Run: handleIP})
	Register(Command{Name: "coin", Hidden: true, Run: handleCrypto})
}
