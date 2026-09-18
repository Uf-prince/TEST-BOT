package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 17 (10 new everyday commands)
// File: toolpack17.go
// ============================================================================
//   .emailcheck <email>  -> validate an email and check its domain MX
//   .dnscheck <domain>   -> DNS records (A, MX, TXT, NS) of a domain
//   .httpheaders <url>   -> HTTP response headers of a website
//   .uuidgen             -> generate a random UUID v4
//   .passwordgen [len]   -> generate a strong random password
//   .base64tool <enc|dec> <text> -> base64 encode or decode
//   .hashgen <algo> <text> -> md5 / sha1 / sha256 hash of text
//   .randomnumber <min> <max> -> random number in a range
//   .coinflip            -> flip a coin (heads or tails)
//   .diceroll [sides]    -> roll a dice
//
// All use FREE public APIs (no key) or run locally and match the GOLD-MD
// design language exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .EMAILCHECK ──────────────────────────────────────────────────────────────

func emailcheckGuide(prefix string) string {
	return "*🔰 EMAIL CHECKER 🔰*\n\n" +
		"*VALIDATE AN EMAIL AND CHECK IF ITS DOMAIN CAN RECEIVE MAIL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "EMAILCHECK <EMAIL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "EMAILCHECK TEST@GMAIL.COM ❯*"
}

func handleEmailcheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		email := strings.TrimSpace(args[0])
		if email == "" {
			s.Reply(info, emailcheckGuide(prefix))
			return
		}
		if !emailRe.MatchString(email) {
			s.Reply(info, "*🔰 INVALID EMAIL FORMAT*")
			return
		}
		parts := strings.SplitN(email, "@", 2)
		domain := parts[1]
		waitID := s.ReplyWithID(info, "*CHECKING EMAIL....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Status int `json:"Status"`
			Answer []struct {
				Data string `json:"data"`
			} `json:"Answer"`
		}
		_ = funGetJSON(ctx, "https://dns.google/resolve?name="+url.QueryEscape(domain)+"&type=MX", &res)
		mxOK := len(res.Answer) > 0
		status := "❌ DOMAIN CANNOT RECEIVE MAIL"
		if mxOK {
			status = "✅ DOMAIN CAN RECEIVE MAIL"
		}
		var b strings.Builder
		b.WriteString("*🔰 EMAIL CHECKER 🔰*\n\n")
		b.WriteString("*📧 EMAIL ❯ " + email + "*\n")
		b.WriteString("*✅ FORMAT ❯ VALID*\n")
		b.WriteString("*🌐 DOMAIN ❯ " + strings.ToUpper(domain) + "*\n")
		b.WriteString("*📬 MX RECORDS ❯ " + strconv.Itoa(len(res.Answer)) + "*\n")
		b.WriteString("*" + status + "*")
		s.Reply(info, b.String())
	})
}

// ── .DNSCHECK ────────────────────────────────────────────────────────────────

func dnscheckGuide(prefix string) string {
	return "*🔰 DNS LOOKUP 🔰*\n\n" +
		"*GET DNS RECORDS (A, MX, TXT, NS) OF ANY DOMAIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DNSCHECK <DOMAIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DNSCHECK GOOGLE.COM ❯*"
}

func handleDnscheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		domain := strings.TrimSpace(args[0])
		if domain == "" {
			s.Reply(info, dnscheckGuide(prefix))
			return
		}
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimSuffix(domain, "/")
		waitID := s.ReplyWithID(info, "*LOOKING UP DNS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		type dnsResp struct {
			Status int `json:"Status"`
			Answer []struct {
				Name string `json:"name"`
				Type int    `json:"type"`
				Data string `json:"data"`
			} `json:"Answer"`
		}
		typesMap := map[int]string{1: "A", 2: "NS", 5: "CNAME", 15: "MX", 16: "TXT", 28: "AAAA"}
		var b strings.Builder
		b.WriteString("*🔰 DNS LOOKUP 🔰*\n\n")
		b.WriteString("*🌐 DOMAIN ❯ " + strings.ToUpper(domain) + "*\n\n")
		found := false
		for _, t := range []int{1, 28, 15, 2, 16} {
			var r dnsResp
			if err := funGetJSON(ctx, "https://dns.google/resolve?name="+url.QueryEscape(domain)+"&type="+strconv.Itoa(t), &r); err != nil {
				continue
			}
			if len(r.Answer) == 0 {
				continue
			}
			found = true
			label := typesMap[t]
			limit := len(r.Answer)
			if limit > 4 {
				limit = 4
			}
			for i := 0; i < limit; i++ {
				d := r.Answer[i].Data
				if len(d) > 80 {
					d = d[:80] + "..."
				}
				b.WriteString("*📌 " + label + " ❯ " + d + "*\n")
			}
		}
		if !found {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "DNS LOOKUP")
			}
			return
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .HTTPHEADERS ─────────────────────────────────────────────────────────────

func httpheadersGuide(prefix string) string {
	return "*🔰 HTTP HEADERS 🔰*\n\n" +
		"*GET THE HTTP RESPONSE HEADERS OF ANY WEBSITE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HTTPHEADERS <URL> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "HTTPHEADERS HTTPS://GITHUB.COM ❯*"
}

func handleHttpheaders(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(args[0])
		if raw == "" {
			s.Reply(info, httpheadersGuide(prefix))
			return
		}
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "https://" + raw
		}
		waitID := s.ReplyWithID(info, "*FETCHING HEADERS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "HTTP HEADERS")
			}
			return
		}
		req.Header.Set("User-Agent", funUA)
		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "HTTP HEADERS")
			}
			return
		}
		defer resp.Body.Close()
		var b strings.Builder
		b.WriteString("*🔰 HTTP HEADERS 🔰*\n\n")
		b.WriteString("*🌐 URL ❯ " + raw + "*\n")
		b.WriteString("*📊 STATUS ❯ " + strconv.Itoa(resp.StatusCode) + " " + strings.ToUpper(resp.Status) + "*\n\n")
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := strings.Join(resp.Header[k], ", ")
			if len(v) > 100 {
				v = v[:100] + "..."
			}
			b.WriteString("*📌 " + strings.ToUpper(k) + " ❯ " + v + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

// ── .UUIDGEN ─────────────────────────────────────────────────────────────────

func uuidgenGuide(prefix string) string {
	return "*🔰 UUID GENERATOR 🔰*\n\n" +
		"*GENERATE A RANDOM UUID V4*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "UUIDGEN ❯*"
}

func handleUuidgen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*GENERATING UUID....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []string
		if err := funGetJSON(ctx, "https://www.uuidtools.com/api/generate/v4/count/1", &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "UUID GENERATOR")
			}
			return
		}
		s.Reply(info, "*🔰 UUID GENERATOR 🔰*\n\n*🆔 UUID ❯ "+res[0]+"*")
	})
}

// ── .PASSWORDGEN ─────────────────────────────────────────────────────────────

func passwordgenGuide(prefix string) string {
	return "*🔰 PASSWORD GENERATOR 🔰*\n\n" +
		"*GENERATE A STRONG RANDOM PASSWORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PASSWORDGEN ❯*  (16 chars)\n" +
		"*❮ " + prefix + "PASSWORDGEN <LENGTH> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PASSWORDGEN 20 ❯*"
}

func handlePasswordgen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		length := 16
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n >= 6 && n <= 64 {
				length = n
			}
		}
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
		var sb strings.Builder
		for i := 0; i < length; i++ {
			idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				funFail(s, info, "PASSWORD GENERATOR")
				return
			}
			sb.WriteByte(charset[idx.Int64()])
		}
		s.Reply(info, "*🔰 PASSWORD GENERATOR 🔰*\n\n*🔑 PASSWORD ❯ "+sb.String()+"*\n*📏 LENGTH ❯ "+strconv.Itoa(length)+"*")
	})
}

// ── .BASE64TOOL ──────────────────────────────────────────────────────────────

func base64toolGuide(prefix string) string {
	return "*🔰 BASE64 TOOL 🔰*\n\n" +
		"*ENCODE OR DECODE BASE64 TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BASE64TOOL ENC <TEXT> ❯*\n" +
		"*❮ " + prefix + "BASE64TOOL DEC <BASE64> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BASE64TOOL ENC HELLO ❯*"
}

func handleBase64tool(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, base64toolGuide(prefix))
			return
		}
		mode := strings.ToLower(strings.TrimSpace(args[0]))
		text := strings.Join(args[1:], " ")
		switch mode {
		case "enc", "encode":
			out := base64.StdEncoding.EncodeToString([]byte(text))
			s.Reply(info, "*🔰 BASE64 TOOL 🔰*\n\n*📝 INPUT ❯ "+text+"*\n*🔒 ENCODED ❯ "+out+"*")
		case "dec", "decode":
			data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
			if err != nil {
				s.Reply(info, "*🔰 INVALID BASE64 STRING*")
				return
			}
			s.Reply(info, "*🔰 BASE64 TOOL 🔰*\n\n*🔒 INPUT ❯ "+text+"*\n*📝 DECODED ❯ "+string(data)+"*")
		default:
			s.Reply(info, base64toolGuide(prefix))
		}
	})
}

// ── .HASHGEN ─────────────────────────────────────────────────────────────────

func hashgenGuide(prefix string) string {
	return "*🔰 HASH GENERATOR 🔰*\n\n" +
		"*GENERATE MD5, SHA1 OR SHA256 HASH OF ANY TEXT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HASHGEN <MD5|SHA1|SHA256> <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "HASHGEN SHA256 HELLO ❯*"
}

func handleHashgen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, hashgenGuide(prefix))
			return
		}
		algo := strings.ToLower(strings.TrimSpace(args[0]))
		text := strings.Join(args[1:], " ")
		var hash string
		switch algo {
		case "md5":
			h := md5.Sum([]byte(text))
			hash = hex.EncodeToString(h[:])
		case "sha1":
			h := sha1.Sum([]byte(text))
			hash = hex.EncodeToString(h[:])
		case "sha256":
			h := sha256.Sum256([]byte(text))
			hash = hex.EncodeToString(h[:])
		default:
			s.Reply(info, hashgenGuide(prefix))
			return
		}
		s.Reply(info, "*🔰 HASH GENERATOR 🔰*\n\n*📝 TEXT ❯ "+text+"*\n*🔐 ALGO ❯ "+strings.ToUpper(algo)+"*\n*#️⃣ HASH ❯ "+hash+"*")
	})
}

// ── .RANDOMNUMBER ────────────────────────────────────────────────────────────

func randomnumberGuide(prefix string) string {
	return "*🔰 RANDOM NUMBER 🔰*\n\n" +
		"*GENERATE A RANDOM NUMBER BETWEEN TWO VALUES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "RANDOMNUMBER <MIN> <MAX> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "RANDOMNUMBER 1 100 ❯*"
}

func handleRandomnumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, randomnumberGuide(prefix))
			return
		}
		min, err1 := strconv.Atoi(strings.TrimSpace(args[0]))
		max, err2 := strconv.Atoi(strings.TrimSpace(args[1]))
		if err1 != nil || err2 != nil || min >= max {
			s.Reply(info, "*🔰 INVALID RANGE, ENSURE MIN < MAX*")
			return
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
		if err != nil {
			funFail(s, info, "RANDOM NUMBER")
			return
		}
		result := int(n.Int64()) + min
		s.Reply(info, "*🔰 RANDOM NUMBER 🔰*\n\n*📊 RANGE ❯ "+strconv.Itoa(min)+" - "+strconv.Itoa(max)+"*\n*🎲 RESULT ❯ "+strconv.Itoa(result)+"*")
	})
}

// ── .COINFLIP ────────────────────────────────────────────────────────────────

func coinflipGuide(prefix string) string {
	return "*🔰 COIN FLIP 🔰*\n\n" +
		"*FLIP A COIN AND GET HEADS OR TAILS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COINFLIP ❯*"
}

func handleCoinflip(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		n, err := rand.Int(rand.Reader, big.NewInt(2))
		if err != nil {
			funFail(s, info, "COIN FLIP")
			return
		}
		result := "🪙 HEADS"
		if n.Int64() == 1 {
			result = "🪙 TAILS"
		}
		s.Reply(info, "*🔰 COIN FLIP 🔰*\n\n*"+result+"*")
	})
}

// ── .DICEROLL ────────────────────────────────────────────────────────────────

func dicerollGuide(prefix string) string {
	return "*🔰 DICE ROLL 🔰*\n\n" +
		"*ROLL A DICE (DEFAULT 6 SIDES)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DICEROLL ❯*\n" +
		"*❮ " + prefix + "DICEROLL <SIDES> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DICEROLL 20 ❯*"
}

func handleDiceroll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		sides := 6
		if len(args) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && n >= 2 && n <= 1000 {
				sides = n
			}
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(sides)))
		if err != nil {
			funFail(s, info, "DICE ROLL")
			return
		}
		result := int(n.Int64()) + 1
		s.Reply(info, "*🔰 DICE ROLL 🔰*\n\n*🎲 SIDES ❯ "+strconv.Itoa(sides)+"*\n*🎯 RESULT ❯ "+strconv.Itoa(result)+"*")
	})
}

func init() {
	Register(Command{Name: "emailcheck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO VALIDATE AN EMAIL AND CHECK ITS DOMAIN MX. USE IT AS .EMAILCHECK <EMAIL>.", Run: handleEmailcheck})
	Register(Command{Name: "dnscheck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DNS RECORDS OF ANY DOMAIN. USE IT AS .DNSCHECK <DOMAIN>.", Run: handleDnscheck})
	Register(Command{Name: "httpheaders", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET HTTP RESPONSE HEADERS OF A WEBSITE. USE IT AS .HTTPHEADERS <URL>.", Run: handleHttpheaders})
	Register(Command{Name: "uuidgen", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A RANDOM UUID V4. USE IT AS .UUIDGEN.", Run: handleUuidgen})
	Register(Command{Name: "passwordgen", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A STRONG RANDOM PASSWORD. USE IT AS .PASSWORDGEN <LENGTH>.", Run: handlePasswordgen})
	Register(Command{Name: "base64tool", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ENCODE OR DECODE BASE64 TEXT. USE IT AS .BASE64TOOL <ENC|DEC> <TEXT>.", Run: handleBase64tool})
	Register(Command{Name: "hashgen", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE MD5, SHA1 OR SHA256 HASH OF TEXT. USE IT AS .HASHGEN <ALGO> <TEXT>.", Run: handleHashgen})
	Register(Command{Name: "randomnumber", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GENERATE A RANDOM NUMBER IN A RANGE. USE IT AS .RANDOMNUMBER <MIN> <MAX>.", Run: handleRandomnumber})
	Register(Command{Name: "coinflip", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FLIP A COIN AND GET HEADS OR TAILS. USE IT AS .COINFLIP.", Run: handleCoinflip})
	Register(Command{Name: "diceroll", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ROLL A DICE. USE IT AS .DICEROLL <SIDES>.", Run: handleDiceroll})

	// hidden aliases
	Register(Command{Name: "mailcheck2", Category: "TOOLS", Desc: "Short alias of .emailcheck", Hidden: true, Run: handleEmailcheck})
	Register(Command{Name: "dnsrecords", Category: "TOOLS", Desc: "Short alias of .dnscheck", Hidden: true, Run: handleDnscheck})
	Register(Command{Name: "headers", Category: "TOOLS", Desc: "Short alias of .httpheaders", Hidden: true, Run: handleHttpheaders})
	Register(Command{Name: "newuuid", Category: "TOOLS", Desc: "Short alias of .uuidgen", Hidden: true, Run: handleUuidgen})
	Register(Command{Name: "passgen", Category: "TOOLS", Desc: "Short alias of .passwordgen", Hidden: true, Run: handlePasswordgen})
	Register(Command{Name: "b64tool", Category: "TOOLS", Desc: "Short alias of .base64tool", Hidden: true, Run: handleBase64tool})
	Register(Command{Name: "hash", Category: "TOOLS", Desc: "Short alias of .hashgen", Hidden: true, Run: handleHashgen})
	Register(Command{Name: "randnum", Category: "TOOLS", Desc: "Short alias of .randomnumber", Hidden: true, Run: handleRandomnumber})
	Register(Command{Name: "flip", Category: "TOOLS", Desc: "Short alias of .coinflip", Hidden: true, Run: handleCoinflip})
	Register(Command{Name: "roll", Category: "TOOLS", Desc: "Short alias of .diceroll", Hidden: true, Run: handleDiceroll})
}
