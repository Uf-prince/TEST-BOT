package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 3 (5 more everyday commands)
// File: toolpack3.go
// ============================================================================
//   .dns <domain>     -> DNS lookup (A / MX / TXT / NS)
//   .color <hex>      -> colour info + swatch image
//   .npm <package>    -> npm package info
//   .pypi <package>   -> Python (PyPI) package info
//   .urban <word>     -> Urban Dictionary slang meaning
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .DNS ────────────────────────────────────────────────────────────────────

func dnsGuide(prefix string) string {
	return "*🔰 DNS LOOKUP 🔰*\n\n" +
		"*GET DNS RECORDS OF ANY DOMAIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DNS <DOMAIN> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DNS GOOGLE.COM ❯*"
}

func handleDNS(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		dom := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		dom = strings.TrimPrefix(dom, "http://")
		dom = strings.TrimPrefix(dom, "https://")
		if i := strings.IndexAny(dom, "/ "); i >= 0 {
			dom = dom[:i]
		}
		if dom == "" {
			s.Reply(info, dnsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP DNS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		type dnsResp struct {
			Status int `json:"Status"`
			Answer []struct {
				Name string `json:"name"`
				Type int    `json:"type"`
				TTL  int    `json:"TTL"`
				Data string `json:"data"`
			} `json:"Answer"`
		}
		typeNames := map[int]string{1: "A", 2: "NS", 5: "CNAME", 6: "SOA", 15: "MX", 16: "TXT", 28: "AAAA"}

		var b strings.Builder
		b.WriteString("*🔰 DNS LOOKUP 🔰*\n\n")
		b.WriteString("*🌐 DOMAIN ❯ " + strings.ToUpper(dom) + "*\n\n")

		got := false
		for _, t := range []int{1, 28, 15, 2, 16} {
			u := "https://dns.google/resolve?name=" + url.QueryEscape(dom) + "&type=" + strconv.Itoa(t)
			var r dnsResp
			if err := funGetJSON(ctx, u, &r); err != nil || len(r.Answer) == 0 {
				continue
			}
			got = true
			b.WriteString("*📌 " + typeNames[t] + " RECORDS:*\n")
			for _, a := range r.Answer {
				data := a.Data
				if len(data) > 120 {
					data = data[:120] + "..."
				}
				b.WriteString("• " + data + "\n")
			}
			b.WriteString("\n")
		}
		if !got {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO DNS RECORDS FOUND*")
			}
			return
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .COLOR ──────────────────────────────────────────────────────────────────

func colorGuide(prefix string) string {
	return "*🔰 COLOR INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY COLOR FROM ITS HEX CODE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COLOR <HEX> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COLOR FF5733 ❯*"
}

func handleColor(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		hex := strings.TrimSpace(strings.Join(args, " "))
		hex = strings.TrimPrefix(strings.ToLower(hex), "#")
		if hex == "" {
			s.Reply(info, colorGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING COLOR....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://www.thecolorapi.com/id?hex=" + url.QueryEscape(hex)
		var res struct {
			Hex struct {
				Value string `json:"value"`
			} `json:"hex"`
			RGB struct {
				Value string `json:"value"`
			} `json:"rgb"`
			HSL struct {
				Value string `json:"value"`
			} `json:"hsl"`
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Hex.Value == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID COLOR, PLEASE CHECK THE HEX CODE*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 COLOR INFO 🔰*\n\n")
		b.WriteString("*🎨 NAME ❯ " + strings.ToUpper(res.Name.Value) + "*\n")
		b.WriteString("*🔷 HEX ❯ " + res.Hex.Value + "*\n")
		b.WriteString("*🟥 RGB ❯ " + res.RGB.Value + "*\n")
		b.WriteString("*🌈 HSL ❯ " + res.HSL.Value + "*")
		caption := strings.TrimSpace(b.String())

		clean := strings.TrimPrefix(res.Hex.Value, "#")
		swatch := "https://singlecolorimage.com/get/" + clean + "/400x400"
		if img, err := funGetBytes(ctx, swatch); err == nil && len(img) > 0 {
			if s.SendImage(info, img, caption) == nil {
				return
			}
		}
		s.Reply(info, caption)
	})
}

// ── .NPM ────────────────────────────────────────────────────────────────────

func npmGuide(prefix string) string {
	return "*🔰 NPM PACKAGE 🔰*\n\n" +
		"*GET INFO OF ANY NPM (NODE.JS) PACKAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NPM <PACKAGE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NPM EXPRESS ❯*"
}

func handleNPM(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		pkg := strings.TrimSpace(strings.Join(args, " "))
		if pkg == "" {
			s.Reply(info, npmGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING NPM PACKAGE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://registry.npmjs.org/" + url.PathEscape(pkg) + "/latest"
		var res struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
			License     string `json:"license"`
			Homepage    string `json:"homepage"`
			Author      struct {
				Name string `json:"name"`
			} `json:"author"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NPM PACKAGE NOT FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NPM PACKAGE 🔰*\n\n")
		b.WriteString("*📦 NAME ❯ " + res.Name + "*\n")
		if res.Version != "" {
			b.WriteString("*🏷️ VERSION ❯ " + res.Version + "*\n")
		}
		if res.License != "" {
			b.WriteString("*📜 LICENSE ❯ " + res.License + "*\n")
		}
		if res.Author.Name != "" {
			b.WriteString("*✍️ AUTHOR ❯ " + res.Author.Name + "*\n")
		}
		if res.Description != "" {
			desc := res.Description
			if len(desc) > 400 {
				desc = desc[:400] + "..."
			}
			b.WriteString("\n*📖 DESCRIPTION:*\n" + desc + "\n")
		}
		b.WriteString("\n*🔗 https://www.npmjs.com/package/" + res.Name + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .PYPI ───────────────────────────────────────────────────────────────────

func pypiGuide(prefix string) string {
	return "*🔰 PYPI PACKAGE 🔰*\n\n" +
		"*GET INFO OF ANY PYTHON (PYPI) PACKAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PYPI <PACKAGE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PYPI REQUESTS ❯*"
}

func handlePyPI(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		pkg := strings.TrimSpace(strings.Join(args, " "))
		if pkg == "" {
			s.Reply(info, pypiGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PYPI PACKAGE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://pypi.org/pypi/" + url.PathEscape(pkg) + "/json"
		var res struct {
			Info struct {
				Name       string `json:"name"`
				Version    string `json:"version"`
				Summary    string `json:"summary"`
				Author     string `json:"author"`
				License    string `json:"license"`
				HomePage   string `json:"home_page"`
				ProjectURL string `json:"project_url"`
			} `json:"info"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Info.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 PYPI PACKAGE NOT FOUND*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 PYPI PACKAGE 🔰*\n\n")
		b.WriteString("*📦 NAME ❯ " + res.Info.Name + "*\n")
		if res.Info.Version != "" {
			b.WriteString("*🏷️ VERSION ❯ " + res.Info.Version + "*\n")
		}
		if res.Info.Author != "" {
			b.WriteString("*✍️ AUTHOR ❯ " + res.Info.Author + "*\n")
		}
		if res.Info.License != "" {
			lic := res.Info.License
			if len(lic) > 60 {
				lic = lic[:60] + "..."
			}
			b.WriteString("*📜 LICENSE ❯ " + lic + "*\n")
		}
		if res.Info.Summary != "" {
			sum := res.Info.Summary
			if len(sum) > 400 {
				sum = sum[:400] + "..."
			}
			b.WriteString("\n*📖 SUMMARY:*\n" + sum + "\n")
		}
		b.WriteString("\n*🔗 https://pypi.org/project/" + res.Info.Name + "*")
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .URBAN ──────────────────────────────────────────────────────────────────

func urbanGuide(prefix string) string {
	return "*🔰 URBAN DICTIONARY 🔰*\n\n" +
		"*GET THE SLANG MEANING OF ANY WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "URBAN <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "URBAN LIT ❯*"
}

func handleUrban(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, urbanGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING SLANG....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.urbandictionary.com/v0/define?term=" + url.QueryEscape(word)
		var res struct {
			List []struct {
				Word       string `json:"word"`
				Definition string `json:"definition"`
				Example    string `json:"example"`
			} `json:"list"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.List) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO SLANG MEANING FOUND*")
			}
			return
		}
		d := res.List[0]
		def := strings.ReplaceAll(d.Definition, "[", "")
		def = strings.ReplaceAll(def, "]", "")
		def = strings.TrimSpace(def)
		if len(def) > 900 {
			def = def[:900] + "..."
		}
		ex := strings.ReplaceAll(d.Example, "[", "")
		ex = strings.ReplaceAll(ex, "]", "")
		ex = strings.TrimSpace(ex)
		if len(ex) > 400 {
			ex = ex[:400] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 URBAN DICTIONARY 🔰*\n\n")
		b.WriteString("*📖 WORD ❯ " + strings.ToUpper(d.Word) + "*\n\n")
		b.WriteString("*MEANING:*\n" + def)
		if ex != "" {
			b.WriteString("\n\n*EXAMPLE:*\n" + ex)
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

func init() {
	Register(Command{Name: "dns", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DNS RECORDS OF ANY DOMAIN. USE IT AS .DNS <DOMAIN>.", Run: handleDNS})
	Register(Command{Name: "color", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY COLOR FROM ITS HEX CODE. USE IT AS .COLOR <HEX>.", Run: handleColor})
	Register(Command{Name: "npm", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY NPM PACKAGE. USE IT AS .NPM <PACKAGE>.", Run: handleNPM})
	Register(Command{Name: "pypi", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFO OF ANY PYPI PACKAGE. USE IT AS .PYPI <PACKAGE>.", Run: handlePyPI})
	Register(Command{Name: "urban", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE SLANG MEANING OF ANY WORD. USE IT AS .URBAN <WORD>.", Run: handleUrban})

	// hidden aliases
	Register(Command{Name: "slang", Category: "TOOLS", Desc: "Short alias of .urban", Hidden: true, Run: handleUrban})
	Register(Command{Name: "colour", Category: "TOOLS", Desc: "Short alias of .color", Hidden: true, Run: handleColor})
	Register(Command{Name: "nslookup", Category: "TOOLS", Desc: "Short alias of .dns", Hidden: true, Run: handleDNS})
}

// keep fmt imported even if a future edit drops its only use
var _ = fmt.Sprintf
