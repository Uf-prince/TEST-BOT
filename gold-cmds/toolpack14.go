package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 14 (10 new everyday commands)
// File: toolpack14.go
// ============================================================================
//   .qrgen <text>        -> generate a QR code image
//   .barcode <text>      -> generate a barcode image
//   .urlshort <url>      -> shorten a long URL
//   .goldprice           -> live gold price per ounce
//   .silverprice         -> live silver price per ounce
//   .currencyconvert <amt> <from> <to> -> live currency conversion
//   .randomuser          -> generate a random user profile
//   .synwords <word>     -> find synonyms
//   .antonym <word>      -> find antonyms
//   .wordassoc <word>    -> find related words
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .QRGEN ───────────────────────────────────────────────────────────────────

func qrgenGuide(prefix string) string {
	return "*🔰 QR CODE GENERATOR 🔰*\n\n" +
		"*TURN ANY TEXT OR LINK INTO A QR CODE IMAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "QRGEN <TEXT OR LINK> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "QRGEN HTTPS://GITHUB.COM ❯*"
}

func handleQrgen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.TrimSpace(strings.Join(args, " "))
		if text == "" {
			s.Reply(info, qrgenGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*GENERATING QR CODE....*")
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

// ── .BARCODE ─────────────────────────────────────────────────────────────────

func barcodeGuide(prefix string) string {
	return "*🔰 BARCODE GENERATOR 🔰*\n\n" +
		"*TURN ANY NUMBER OR TEXT INTO A BARCODE IMAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BARCODE <TEXT OR NUMBER> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BARCODE 123456789 ❯*"
}

func handleBarcode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		text := strings.TrimSpace(strings.Join(args, " "))
		if text == "" {
			s.Reply(info, barcodeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*GENERATING BARCODE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		u := "https://barcode.tec-it.com/barcode.ashx?data=" + url.QueryEscape(text) + "&code=Code128&translate-esc=on"
		img, err := funGetBytes(ctx, u)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "BARCODE")
			}
			return
		}
		if err := s.SendImage(info, img, "*🔰 BARCODE 🔰*\n\n*"+text+"*"); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "BARCODE")
			}
		}
	})
}

// ── .URLSHORT ────────────────────────────────────────────────────────────────

func urlshortGuide(prefix string) string {
	return "*🔰 URL SHORTENER 🔰*\n\n" +
		"*SHORTEN ANY LONG LINK INTO A SHORT ONE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "URLSHORT <LONG URL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "URLSHORT HTTPS://EXAMPLE.COM/VERY/LONG/PATH ❯*"
}

func handleUrlshort(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(args[0])
		if raw == "" {
			s.Reply(info, urlshortGuide(prefix))
			return
		}
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "https://" + raw
		}
		waitID := s.ReplyWithID(info, "*SHORTENING LINK....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		body, err := funGetBytes(ctx, "https://is.gd/create.php?format=simple&url="+url.QueryEscape(raw))
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "URL SHORTENER")
			}
			return
		}
		short := strings.TrimSpace(string(body))
		if !strings.HasPrefix(short, "http") {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "URL SHORTENER")
			}
			return
		}
		s.Reply(info, "*🔰 URL SHORTENER 🔰*\n\n*🔗 ORIGINAL ❯ "+raw+"*\n*✂️ SHORT ❯ "+short+"*")
	})
}

// ── .GOLDPRICE ───────────────────────────────────────────────────────────────

func goldpriceGuide(prefix string) string {
	return "*🔰 GOLD PRICE 🔰*\n\n" +
		"*GET THE LIVE GOLD PRICE PER OUNCE IN USD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GOLDPRICE ❯*"
}

func handleGoldprice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING GOLD PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Name      string  `json:"name"`
			Price     float64 `json:"price"`
			Symbol    string  `json:"symbol"`
			UpdatedAt string  `json:"updatedAtReadable"`
		}
		if err := funGetJSON(ctx, "https://api.gold-api.com/price/XAU", &res); err != nil || res.Price == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "GOLD PRICE")
			}
			return
		}
		s.Reply(info, "*🔰 GOLD PRICE 🔰*\n\n*🥇 METAL ❯ "+strings.ToUpper(res.Name)+"*\n*💰 PRICE ❯ $"+strconv.FormatFloat(res.Price, 'f', 2, 64)+" / OUNCE*\n*🕒 UPDATED ❯ "+strings.ToUpper(res.UpdatedAt)+"*")
	})
}

// ── .SILVERPRICE ─────────────────────────────────────────────────────────────

func silverpriceGuide(prefix string) string {
	return "*🔰 SILVER PRICE 🔰*\n\n" +
		"*GET THE LIVE SILVER PRICE PER OUNCE IN USD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SILVERPRICE ❯*"
}

func handleSilverprice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING SILVER PRICE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Name      string  `json:"name"`
			Price     float64 `json:"price"`
			Symbol    string  `json:"symbol"`
			UpdatedAt string  `json:"updatedAtReadable"`
		}
		if err := funGetJSON(ctx, "https://api.gold-api.com/price/XAG", &res); err != nil || res.Price == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SILVER PRICE")
			}
			return
		}
		s.Reply(info, "*🔰 SILVER PRICE 🔰*\n\n*🥈 METAL ❯ "+strings.ToUpper(res.Name)+"*\n*💰 PRICE ❯ $"+strconv.FormatFloat(res.Price, 'f', 2, 64)+" / OUNCE*\n*🕒 UPDATED ❯ "+strings.ToUpper(res.UpdatedAt)+"*")
	})
}

// ── .CURRENCYCONVERT ─────────────────────────────────────────────────────────

func currencyconvertGuide(prefix string) string {
	return "*🔰 CURRENCY CONVERTER 🔰*\n\n" +
		"*CONVERT MONEY BETWEEN ANY TWO CURRENCIES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CURRENCYCONVERT <AMOUNT> <FROM> <TO> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CURRENCYCONVERT 100 USD PKR ❯*"
}

func handleCurrencyconvert(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, currencyconvertGuide(prefix))
			return
		}
		amt, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID AMOUNT, PLEASE GIVE A NUMBER*")
			return
		}
		from := strings.ToUpper(strings.TrimSpace(args[1]))
		to := strings.ToUpper(strings.TrimSpace(args[2]))
		waitID := s.ReplyWithID(info, "*CONVERTING CURRENCY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Result string             `json:"result"`
			Rates  map[string]float64 `json:"rates"`
		}
		if err := funGetJSON(ctx, "https://open.er-api.com/v6/latest/"+url.QueryEscape(from), &res); err != nil || res.Result != "success" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "CURRENCY CONVERTER")
			}
			return
		}
		rate, ok := res.Rates[to]
		if !ok {
			s.Reply(info, "*🔰 UNKNOWN CURRENCY CODE ❯ "+to+"*")
			return
		}
		conv := amt * rate
		s.Reply(info, "*🔰 CURRENCY CONVERTER 🔰*\n\n*💵 "+strconv.FormatFloat(amt, 'f', 2, 64)+" "+from+"*\n*➡️ "+strconv.FormatFloat(conv, 'f', 2, 64)+" "+to+"*\n*📊 RATE ❯ 1 "+from+" = "+strconv.FormatFloat(rate, 'f', 4, 64)+" "+to+"*")
	})
}

// ── .RANDOMUSER ──────────────────────────────────────────────────────────────

func randomuserGuide(prefix string) string {
	return "*🔰 RANDOM USER 🔰*\n\n" +
		"*GENERATE A RANDOM FAKE USER PROFILE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMUSER ❯*"
}

func handleRandomuser(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*GENERATING USER....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Results []struct {
				Gender string `json:"gender"`
				Name   struct {
					Title string `json:"title"`
					First string `json:"first"`
					Last  string `json:"last"`
				} `json:"name"`
				Email    string `json:"email"`
				Phone    string `json:"phone"`
				Location struct {
					City    string `json:"city"`
					State   string `json:"state"`
					Country string `json:"country"`
				} `json:"location"`
				Nat string `json:"nat"`
			} `json:"results"`
		}
		if err := funGetJSON(ctx, "https://randomuser.me/api/", &res); err != nil || len(res.Results) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "RANDOM USER")
			}
			return
		}
		u := res.Results[0]
		full := strings.TrimSpace(u.Name.Title + " " + u.Name.First + " " + u.Name.Last)
		s.Reply(info, "*🔰 RANDOM USER 🔰*\n\n*👤 NAME ❯ "+strings.ToUpper(full)+"*\n*⚧ GENDER ❯ "+strings.ToUpper(u.Gender)+"*\n*📧 EMAIL ❯ "+u.Email+"*\n*📞 PHONE ❯ "+u.Phone+"*\n*🏙️ CITY ❯ "+strings.ToUpper(u.Location.City)+"*\n*🌍 COUNTRY ❯ "+strings.ToUpper(u.Location.Country)+" ("+u.Nat+")*")
	})
}

// ── .SYNWORDS ────────────────────────────────────────────────────────────────

func synwordsGuide(prefix string) string {
	return "*🔰 SYNONYMS 🔰*\n\n" +
		"*FIND WORDS WITH THE SAME MEANING*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SYNWORDS <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SYNWORDS HAPPY ❯*"
}

func handleSynwords(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, synwordsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING SYNONYMS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word string `json:"word"`
		}
		if err := funGetJSON(ctx, "https://api.datamuse.com/words?max=15&rel_syn="+url.QueryEscape(word), &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SYNONYMS")
			}
			return
		}
		var words []string
		for _, w := range res {
			words = append(words, strings.ToUpper(w.Word))
		}
		s.Reply(info, "*🔰 SYNONYMS 🔰*\n\n*📝 WORD ❯ "+strings.ToUpper(word)+"*\n\n*"+strings.Join(words, ", ")+"*")
	})
}

// ── .ANTONYM ─────────────────────────────────────────────────────────────────

func antonymGuide(prefix string) string {
	return "*🔰 ANTONYMS 🔰*\n\n" +
		"*FIND WORDS WITH THE OPPOSITE MEANING*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ANTONYM <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ANTONYM HAPPY ❯*"
}

func handleAntonym(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, antonymGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING ANTONYMS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word string `json:"word"`
		}
		if err := funGetJSON(ctx, "https://api.datamuse.com/words?max=15&rel_ant="+url.QueryEscape(word), &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "ANTONYMS")
			}
			return
		}
		var words []string
		for _, w := range res {
			words = append(words, strings.ToUpper(w.Word))
		}
		s.Reply(info, "*🔰 ANTONYMS 🔰*\n\n*📝 WORD ❯ "+strings.ToUpper(word)+"*\n\n*"+strings.Join(words, ", ")+"*")
	})
}

// ── .WORDASSOC ───────────────────────────────────────────────────────────────

func wordassocGuide(prefix string) string {
	return "*🔰 RELATED WORDS 🔰*\n\n" +
		"*FIND WORDS RELATED TO ANY WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORDASSOC <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WORDASSOC OCEAN ❯*"
}

func handleWordassoc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, wordassocGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING RELATED WORDS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Word string `json:"word"`
		}
		if err := funGetJSON(ctx, "https://api.datamuse.com/words?max=15&ml="+url.QueryEscape(word), &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "RELATED WORDS")
			}
			return
		}
		var words []string
		for _, w := range res {
			words = append(words, strings.ToUpper(w.Word))
		}
		s.Reply(info, "*🔰 RELATED WORDS 🔰*\n\n*📝 WORD ❯ "+strings.ToUpper(word)+"*\n\n*"+strings.Join(words, ", ")+"*")
	})
}

func init() {
	Register(Command{Name: "qrgen", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A QR CODE IMAGE FROM ANY TEXT OR LINK. USE IT AS .QRGEN <TEXT>.", Run: handleQrgen})
	Register(Command{Name: "barcode", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A BARCODE IMAGE FROM ANY TEXT OR NUMBER. USE IT AS .BARCODE <TEXT>.", Run: handleBarcode})
	Register(Command{Name: "urlshort", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHORTEN ANY LONG URL INTO A SHORT LINK. USE IT AS .URLSHORT <URL>.", Run: handleUrlshort})
	Register(Command{Name: "goldprice", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE GOLD PRICE PER OUNCE IN USD. USE IT AS .GOLDPRICE.", Run: handleGoldprice})
	Register(Command{Name: "silverprice", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LIVE SILVER PRICE PER OUNCE IN USD. USE IT AS .SILVERPRICE.", Run: handleSilverprice})
	Register(Command{Name: "currencyconvert", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT MONEY BETWEEN ANY TWO CURRENCIES. USE IT AS .CURRENCYCONVERT <AMOUNT> <FROM> <TO>.", Run: handleCurrencyconvert})
	Register(Command{Name: "randomuser", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A RANDOM FAKE USER PROFILE. USE IT AS .RANDOMUSER.", Run: handleRandomuser})
	Register(Command{Name: "synwords", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND SYNONYMS OF ANY WORD. USE IT AS .SYNWORDS <WORD>.", Run: handleSynwords})
	Register(Command{Name: "antonym", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND ANTONYMS OF ANY WORD. USE IT AS .ANTONYM <WORD>.", Run: handleAntonym})
	Register(Command{Name: "wordassoc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND WORDS RELATED TO ANY WORD. USE IT AS .WORDASSOC <WORD>.", Run: handleWordassoc})

	// hidden aliases
	Register(Command{Name: "makeqr", Category: "TOOLS", Desc: "Short alias of .qrgen", Hidden: true, Run: handleQrgen})
	Register(Command{Name: "makebarcode", Category: "TOOLS", Desc: "Short alias of .barcode", Hidden: true, Run: handleBarcode})
	Register(Command{Name: "shorten", Category: "TOOLS", Desc: "Short alias of .urlshort", Hidden: true, Run: handleUrlshort})
	Register(Command{Name: "goldrate", Category: "TOOLS", Desc: "Short alias of .goldprice", Hidden: true, Run: handleGoldprice})
	Register(Command{Name: "silverrate", Category: "TOOLS", Desc: "Short alias of .silverprice", Hidden: true, Run: handleSilverprice})
	Register(Command{Name: "currency", Category: "TOOLS", Desc: "Short alias of .currencyconvert", Hidden: true, Run: handleCurrencyconvert})
	Register(Command{Name: "fakeuser", Category: "TOOLS", Desc: "Short alias of .randomuser", Hidden: true, Run: handleRandomuser})
	Register(Command{Name: "synonym", Category: "TOOLS", Desc: "Short alias of .synwords", Hidden: true, Run: handleSynwords})
	Register(Command{Name: "opposite", Category: "TOOLS", Desc: "Short alias of .antonym", Hidden: true, Run: handleAntonym})
	Register(Command{Name: "relatedwords", Category: "TOOLS", Desc: "Short alias of .wordassoc", Hidden: true, Run: handleWordassoc})
}
