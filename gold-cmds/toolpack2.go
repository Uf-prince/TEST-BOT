package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 2 (5 more everyday commands)
// File: toolpack2.go
// ============================================================================
//   .country <name>   -> country info (capital, currency, region, flag)
//   .stock <symbol>   -> live stock / share price
//   .tvshow <title>   -> TV show info (TVMaze)
//   .book <title>     -> book info (Open Library)
//   .whois <domain>   -> domain registration info (RDAP)
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .COUNTRY ────────────────────────────────────────────────────────────────

func countryGuide(prefix string) string {
	return "*🔰 COUNTRY INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTRY <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COUNTRY PAKISTAN ❯*"
}

func handleCountry(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, countryGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING COUNTRY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://countriesnow.space/api/v0.1/countries/info?returns=currency,flag,capital,region,iso2,iso3"
		var res struct {
			Data []struct {
				Name     string `json:"name"`
				Currency string `json:"currency"`
				Capital  string `json:"capital"`
				Region   string `json:"region"`
				ISO2     string `json:"iso2"`
				ISO3     string `json:"iso3"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COUNTRY")
			}
			return
		}
		needle := strings.ToLower(q)
		var found *struct {
			Name     string `json:"name"`
			Currency string `json:"currency"`
			Capital  string `json:"capital"`
			Region   string `json:"region"`
			ISO2     string `json:"iso2"`
			ISO3     string `json:"iso3"`
		}
		for i := range res.Data {
			if strings.ToLower(res.Data[i].Name) == needle {
				found = &res.Data[i]
				break
			}
		}
		if found == nil {
			for i := range res.Data {
				if strings.Contains(strings.ToLower(res.Data[i].Name), needle) {
					found = &res.Data[i]
					break
				}
			}
		}
		if found == nil {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 COUNTRY NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}

		var b strings.Builder
		b.WriteString("*🔰 COUNTRY INFO 🔰*\n\n")
		b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(found.Name) + "*\n")
		if found.Capital != "" {
			b.WriteString("*🏛️ CAPITAL ❯ " + strings.ToUpper(found.Capital) + "*\n")
		}
		if found.Region != "" {
			b.WriteString("*🗺️ REGION ❯ " + strings.ToUpper(found.Region) + "*\n")
		}
		if found.Currency != "" {
			b.WriteString("*💰 CURRENCY ❯ " + strings.ToUpper(found.Currency) + "*\n")
		}
		if found.ISO2 != "" {
			b.WriteString("*🔤 CODE ❯ " + strings.ToUpper(found.ISO2) + " / " + strings.ToUpper(found.ISO3) + "*")
		}
		caption := strings.TrimSpace(b.String())

		if found.ISO2 != "" {
			flagURL := "https://flagcdn.com/w640/" + strings.ToLower(found.ISO2) + ".png"
			if img, err := funGetBytes(ctx, flagURL); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .STOCK ──────────────────────────────────────────────────────────────────

func stockGuide(prefix string) string {
	return "*🔰 STOCK PRICE 🔰*\n\n" +
		"*GET THE LIVE PRICE OF ANY STOCK / SHARE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "STOCK <SYMBOL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "STOCK AAPL ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "STOCK TSLA ❯*"
}

func handleStock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		sym := strings.ToUpper(strings.TrimSpace(strings.Join(args, " ")))
		if sym == "" {
			s.Reply(info, stockGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING STOCK PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://query1.finance.yahoo.com/v8/finance/chart/" + url.PathEscape(sym) + "?interval=1d&range=1d"
		var res struct {
			Chart struct {
				Result []struct {
					Meta struct {
						Symbol             string  `json:"symbol"`
						ShortName          string  `json:"shortName"`
						Currency           string  `json:"currency"`
						RegularMarketPrice float64 `json:"regularMarketPrice"`
						ChartPreviousClose float64 `json:"chartPreviousClose"`
						ExchangeName       string  `json:"exchangeName"`
					} `json:"meta"`
				} `json:"result"`
			} `json:"chart"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Chart.Result) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 STOCK NOT FOUND, PLEASE CHECK THE SYMBOL*")
			}
			return
		}
		m := res.Chart.Result[0].Meta
		change := 0.0
		pct := 0.0
		if m.ChartPreviousClose > 0 {
			change = m.RegularMarketPrice - m.ChartPreviousClose
			pct = change / m.ChartPreviousClose * 100
		}
		arrow := "🔻"
		if change >= 0 {
			arrow = "🔺"
		}
		var b strings.Builder
		b.WriteString("*🔰 STOCK PRICE 🔰*\n\n")
		if m.ShortName != "" {
			b.WriteString("*🏢 " + strings.ToUpper(m.ShortName) + "*\n")
		}
		b.WriteString("*📈 SYMBOL ❯ " + m.Symbol + "*\n")
		b.WriteString("*💵 PRICE ❯ " + fmt.Sprintf("%.2f", m.RegularMarketPrice) + " " + m.Currency + "*\n")
		b.WriteString("*" + arrow + " CHANGE ❯ " + fmt.Sprintf("%+.2f", change) + " (" + fmt.Sprintf("%+.2f", pct) + "%)*")
		if m.ExchangeName != "" {
			b.WriteString("\n*🏛️ EXCHANGE ❯ " + m.ExchangeName + "*")
		}
		s.Reply(info, b.String())
	})
}

// ── .TVSHOW ─────────────────────────────────────────────────────────────────

func tvshowGuide(prefix string) string {
	return "*🔰 TV SHOW INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY TV SHOW / SERIES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TVSHOW <TITLE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TVSHOW BREAKING BAD ❯*"
}

func handleTVShow(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, tvshowGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING TV SHOW....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.tvmaze.com/singlesearch/shows?q=" + url.QueryEscape(q)
		var res struct {
			Name      string   `json:"name"`
			Type      string   `json:"type"`
			Language  string   `json:"language"`
			Genres    []string `json:"genres"`
			Status    string   `json:"status"`
			Premiered string   `json:"premiered"`
			Ended     string   `json:"ended"`
			Runtime   int      `json:"runtime"`
			Rating    struct {
				Average float64 `json:"average"`
			} `json:"rating"`
			Image struct {
				Medium string `json:"medium"`
			} `json:"image"`
			Summary string `json:"summary"`
			URL     string `json:"url"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 TV SHOW NOT FOUND, PLEASE CHECK THE TITLE*")
			}
			return
		}
		syn := stripHTML(res.Summary)
		if len(syn) > 700 {
			syn = syn[:700] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 TV SHOW INFO 🔰*\n\n")
		b.WriteString("*📺 TITLE ❯ " + strings.ToUpper(res.Name) + "*\n")
		if res.Type != "" {
			b.WriteString("*🎭 TYPE ❯ " + strings.ToUpper(res.Type) + "*\n")
		}
		if len(res.Genres) > 0 {
			b.WriteString("*🏷️ GENRES ❯ " + strings.ToUpper(strings.Join(res.Genres, ", ")) + "*\n")
		}
		if res.Status != "" {
			b.WriteString("*📡 STATUS ❯ " + strings.ToUpper(res.Status) + "*\n")
		}
		if res.Premiered != "" {
			b.WriteString("*📅 PREMIERED ❯ " + res.Premiered + "*\n")
		}
		if res.Runtime > 0 {
			b.WriteString("*⏱️ RUNTIME ❯ " + strconv.Itoa(res.Runtime) + " MIN*")
		}
		if res.Rating.Average > 0 {
			b.WriteString("\n*⭐ RATING ❯ " + fmt.Sprintf("%.1f", res.Rating.Average) + "/10*")
		}
		if syn != "" {
			b.WriteString("\n\n*📖 SUMMARY:*\n" + syn)
		}
		if res.URL != "" {
			b.WriteString("\n\n*🔗 " + res.URL + "*")
		}
		caption := strings.TrimSpace(b.String())

		if res.Image.Medium != "" {
			if img, err := funGetBytes(ctx, res.Image.Medium); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .BOOK ───────────────────────────────────────────────────────────────────

func bookGuide(prefix string) string {
	return "*🔰 BOOK INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY BOOK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BOOK <TITLE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BOOK HARRY POTTER ❯*"
}

func handleBook(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, bookGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING BOOK....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://openlibrary.org/search.json?limit=1&fields=title,author_name,first_publish_year,number_of_pages_median,key,cover_i&q=" + url.QueryEscape(q)
		var res struct {
			Docs []struct {
				Title       string   `json:"title"`
				AuthorName  []string `json:"author_name"`
				FirstYear   int      `json:"first_publish_year"`
				Pages       int      `json:"number_of_pages_median"`
				Key         string   `json:"key"`
				CoverID     int      `json:"cover_i"`
			} `json:"docs"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Docs) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 BOOK NOT FOUND, PLEASE CHECK THE TITLE*")
			}
			return
		}
		d := res.Docs[0]
		var b strings.Builder
		b.WriteString("*🔰 BOOK INFO 🔰*\n\n")
		b.WriteString("*📚 TITLE ❯ " + strings.ToUpper(d.Title) + "*\n")
		if len(d.AuthorName) > 0 {
			b.WriteString("*✍️ AUTHOR ❯ " + strings.ToUpper(strings.Join(d.AuthorName, ", ")) + "*\n")
		}
		if d.FirstYear > 0 {
			b.WriteString("*📅 FIRST PUBLISHED ❯ " + strconv.Itoa(d.FirstYear) + "*\n")
		}
		if d.Pages > 0 {
			b.WriteString("*📄 PAGES ❯ " + strconv.Itoa(d.Pages) + "*\n")
		}
		if d.Key != "" {
			b.WriteString("*🔗 https://openlibrary.org" + d.Key + "*")
		}
		caption := strings.TrimSpace(b.String())

		if d.CoverID > 0 {
			coverURL := "https://covers.openlibrary.org/b/id/" + strconv.Itoa(d.CoverID) + "-L.jpg"
			if img, err := funGetBytes(ctx, coverURL); err == nil && len(img) > 0 {
				if s.SendImage(info, img, caption) == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ── .WHOIS ──────────────────────────────────────────────────────────────────

func whoisGuide(prefix string) string {
	return "*🔰 DOMAIN WHOIS 🔰*\n\n" +
		"*GET REGISTRATION INFO OF ANY DOMAIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WHOIS <DOMAIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WHOIS GOOGLE.COM ❯*"
}

func handleWhois(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		dom := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		dom = strings.TrimPrefix(dom, "http://")
		dom = strings.TrimPrefix(dom, "https://")
		if i := strings.IndexAny(dom, "/ "); i >= 0 {
			dom = dom[:i]
		}
		if dom == "" {
			s.Reply(info, whoisGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING WHOIS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://rdap.org/domain/" + url.PathEscape(dom)
		var res struct {
			LDHName string `json:"ldhName"`
			Status  []string `json:"status"`
			Events  []struct {
				EventAction string `json:"eventAction"`
				EventDate   string `json:"eventDate"`
			} `json:"events"`
			Nameservers []struct {
				LDHName string `json:"ldhName"`
			} `json:"nameservers"`
			Entities []struct {
				Roles      []string `json:"roles"`
				VCardArray []any    `json:"vcardArray"`
			} `json:"entities"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.LDHName == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 DOMAIN NOT FOUND OR WHOIS UNAVAILABLE*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DOMAIN WHOIS 🔰*\n\n")
		b.WriteString("*🌐 DOMAIN ❯ " + strings.ToUpper(res.LDHName) + "*\n")
		for _, ev := range res.Events {
			switch ev.EventAction {
			case "registration":
				b.WriteString("*📅 REGISTERED ❯ " + ev.EventDate + "*\n")
			case "expiration":
				b.WriteString("*⌛ EXPIRES ❯ " + ev.EventDate + "*\n")
			case "last changed":
				b.WriteString("*🔄 UPDATED ❯ " + ev.EventDate + "*\n")
			}
		}
		if len(res.Status) > 0 {
			b.WriteString("*📋 STATUS ❯ " + strings.ToUpper(strings.Join(res.Status, ", ")) + "*\n")
		}
		if len(res.Nameservers) > 0 {
			ns := make([]string, 0, len(res.Nameservers))
			for _, n := range res.Nameservers {
				ns = append(ns, strings.ToUpper(n.LDHName))
			}
			b.WriteString("*🖥️ NAMESERVERS ❯ " + strings.Join(ns, ", ") + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

func init() {
	Register(Command{Name: "country", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY COUNTRY. USE IT AS .COUNTRY <NAME>.", Run: handleCountry})
	Register(Command{Name: "stock", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE PRICE OF ANY STOCK. USE IT AS .STOCK <SYMBOL>.", Run: handleStock})
	Register(Command{Name: "tvshow", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY TV SHOW. USE IT AS .TVSHOW <TITLE>.", Run: handleTVShow})
	Register(Command{Name: "book", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY BOOK. USE IT AS .BOOK <TITLE>.", Run: handleBook})
	Register(Command{Name: "whois", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET WHOIS INFO OF ANY DOMAIN. USE IT AS .WHOIS <DOMAIN>.", Run: handleWhois})

	// hidden aliases
	Register(Command{Name: "share", Category: "TOOLS", Desc: "Short alias of .stock", Hidden: true, Run: handleStock})
	Register(Command{Name: "series", Category: "TOOLS", Desc: "Short alias of .tvshow", Hidden: true, Run: handleTVShow})
	Register(Command{Name: "domain", Category: "TOOLS", Desc: "Short alias of .whois", Hidden: true, Run: handleWhois})
}
