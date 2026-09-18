package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 35 (10 text & encoding converters)
// File: convpack35.go
// ============================================================================
//   .texttobinary  -> text to binary
//   .binarytotext  -> binary to text
//   .texttohex     -> text to hexadecimal
//   .hextotext     -> hexadecimal to text
//   .texttooctal   -> text to octal
//   .octaltotext   -> octal to text
//   .texttobase64  -> text to base64
//   .base64totext  -> base64 to text
//   .texttourl     -> text to URL-encoded
//   .urltotext     -> URL-encoded to text
// ============================================================================

import (
	"context"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func textConvReply(s SessionBridge, info types.MessageInfo, title, inLabel, inVal, outLabel, outVal string) {
	var b strings.Builder
	b.WriteString("*🔰 " + title + " 🔰*\n\n")
	b.WriteString("*📥 " + inLabel + " ❯ " + inVal + "*\n")
	b.WriteString("*📤 " + outLabel + " ❯ " + outVal + "*")
	s.Reply(info, b.String())
}

func handleTexttobinary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		txt := strings.Join(args, " ")
		if txt == "" {
			s.Reply(info, convGuide(prefix, "TEXT TO BINARY", "texttobinary", "TEXT", "BINARY"))
			return
		}
		var parts []string
		for _, r := range txt {
			parts = append(parts, strconv.FormatInt(int64(r), 2))
		}
		textConvReply(s, info, "TEXT TO BINARY", "TEXT", txt, "BINARY", strings.Join(parts, " "))
	})
}

func handleBinarytotext(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "BINARY TO TEXT", "binarytotext", "BINARY", "TEXT"))
			return
		}
		fields := strings.Fields(raw)
		var b strings.Builder
		for _, f := range fields {
			n, err := strconv.ParseInt(f, 2, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID BINARY, USE SPACE-SEPARATED 0/1 GROUPS*")
				return
			}
			b.WriteRune(rune(n))
		}
		textConvReply(s, info, "BINARY TO TEXT", "BINARY", raw, "TEXT", b.String())
	})
}

func handleTexttohex(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		txt := strings.Join(args, " ")
		if txt == "" {
			s.Reply(info, convGuide(prefix, "TEXT TO HEX", "texttohex", "TEXT", "HEX"))
			return
		}
		var parts []string
		for _, r := range txt {
			parts = append(parts, strconv.FormatInt(int64(r), 16))
		}
		textConvReply(s, info, "TEXT TO HEX", "TEXT", txt, "HEX", strings.Join(parts, " "))
	})
}

func handleHextotext(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "HEX TO TEXT", "hextotext", "HEX", "TEXT"))
			return
		}
		fields := strings.Fields(raw)
		var b strings.Builder
		for _, f := range fields {
			n, err := strconv.ParseInt(f, 16, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID HEX, USE SPACE-SEPARATED HEX GROUPS*")
				return
			}
			b.WriteRune(rune(n))
		}
		textConvReply(s, info, "HEX TO TEXT", "HEX", raw, "TEXT", b.String())
	})
}

func handleTexttooctal(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		txt := strings.Join(args, " ")
		if txt == "" {
			s.Reply(info, convGuide(prefix, "TEXT TO OCTAL", "texttooctal", "TEXT", "OCTAL"))
			return
		}
		var parts []string
		for _, r := range txt {
			parts = append(parts, strconv.FormatInt(int64(r), 8))
		}
		textConvReply(s, info, "TEXT TO OCTAL", "TEXT", txt, "OCTAL", strings.Join(parts, " "))
	})
}

func handleOctaltotext(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "OCTAL TO TEXT", "octaltotext", "OCTAL", "TEXT"))
			return
		}
		fields := strings.Fields(raw)
		var b strings.Builder
		for _, f := range fields {
			n, err := strconv.ParseInt(f, 8, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID OCTAL, USE SPACE-SEPARATED OCTAL GROUPS*")
				return
			}
			b.WriteRune(rune(n))
		}
		textConvReply(s, info, "OCTAL TO TEXT", "OCTAL", raw, "TEXT", b.String())
	})
}

func handleTexttobase64(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		txt := strings.Join(args, " ")
		if txt == "" {
			s.Reply(info, convGuide(prefix, "TEXT TO BASE64", "texttobase64", "TEXT", "BASE64"))
			return
		}
		enc := base64.StdEncoding.EncodeToString([]byte(txt))
		textConvReply(s, info, "TEXT TO BASE64", "TEXT", txt, "BASE64", enc)
	})
}

func handleBase64totext(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "BASE64 TO TEXT", "base64totext", "BASE64", "TEXT"))
			return
		}
		dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
		if err != nil {
			s.Reply(info, "*🔰 INVALID BASE64, PLEASE CHECK YOUR INPUT*")
			return
		}
		textConvReply(s, info, "BASE64 TO TEXT", "BASE64", raw, "TEXT", string(dec))
	})
}

func handleTexttourl(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		txt := strings.Join(args, " ")
		if txt == "" {
			s.Reply(info, convGuide(prefix, "TEXT TO URL", "texttourl", "TEXT", "URL-ENCODED"))
			return
		}
		textConvReply(s, info, "TEXT TO URL", "TEXT", txt, "URL-ENCODED", url.QueryEscape(txt))
	})
}

func handleUrltotext(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.Join(args, " ")
		if strings.TrimSpace(raw) == "" {
			s.Reply(info, convGuide(prefix, "URL TO TEXT", "urltotext", "URL-ENCODED", "TEXT"))
			return
		}
		dec, err := url.QueryUnescape(raw)
		if err != nil {
			s.Reply(info, "*🔰 INVALID URL ENCODING, PLEASE CHECK YOUR INPUT*")
			return
		}
		textConvReply(s, info, "URL TO TEXT", "URL-ENCODED", raw, "TEXT", dec)
	})
}

func init() {
	Register(Command{Name: "texttobinary", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO BINARY. USE IT AS .TEXTTOBINARY <TEXT>.", Run: handleTexttobinary})
	Register(Command{Name: "binarytotext", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BINARY TO TEXT. USE IT AS .BINARYTOTEXT <BINARY>.", Run: handleBinarytotext})
	Register(Command{Name: "texttohex", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO HEXADECIMAL. USE IT AS .TEXTTOHEX <TEXT>.", Run: handleTexttohex})
	Register(Command{Name: "hextotext", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HEXADECIMAL TO TEXT. USE IT AS .HEXTOTEXT <HEX>.", Run: handleHextotext})
	Register(Command{Name: "texttooctal", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO OCTAL. USE IT AS .TEXTOOCTAL <TEXT>.", Run: handleTexttooctal})
	Register(Command{Name: "octaltotext", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT OCTAL TO TEXT. USE IT AS .OCTALTOTEXT <OCTAL>.", Run: handleOctaltotext})
	Register(Command{Name: "texttobase64", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO BASE64. USE IT AS .TEXTTOBASE64 <TEXT>.", Run: handleTexttobase64})
	Register(Command{Name: "base64totext", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BASE64 TO TEXT. USE IT AS .BASE64TOTEXT <BASE64>.", Run: handleBase64totext})
	Register(Command{Name: "texttourl", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEXT TO URL-ENCODED FORM. USE IT AS .TEXTTOURL <TEXT>.", Run: handleTexttourl})
	Register(Command{Name: "urltotext", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT URL-ENCODED TEXT BACK TO PLAIN TEXT. USE IT AS .URLTOTEXT <ENCODED>.", Run: handleUrltotext})

	// hidden aliases
	Register(Command{Name: "texttobin", Category: "CONVERTER", Desc: "Short alias of .texttobinary", Hidden: true, Run: handleTexttobinary})
	Register(Command{Name: "bintotext", Category: "CONVERTER", Desc: "Short alias of .binarytotext", Hidden: true, Run: handleBinarytotext})
	Register(Command{Name: "texttohexadecimal", Category: "CONVERTER", Desc: "Short alias of .texttohex", Hidden: true, Run: handleTexttohex})
	Register(Command{Name: "hextotext2", Category: "CONVERTER", Desc: "Short alias of .hextotext", Hidden: true, Run: handleHextotext})
	Register(Command{Name: "texttooct", Category: "CONVERTER", Desc: "Short alias of .texttooctal", Hidden: true, Run: handleTexttooctal})
	Register(Command{Name: "octtotext", Category: "CONVERTER", Desc: "Short alias of .octaltotext", Hidden: true, Run: handleOctaltotext})
	Register(Command{Name: "texttob64", Category: "CONVERTER", Desc: "Short alias of .texttobase64", Hidden: true, Run: handleTexttobase64})
	Register(Command{Name: "b64totext", Category: "CONVERTER", Desc: "Short alias of .base64totext", Hidden: true, Run: handleBase64totext})
	Register(Command{Name: "texttourlencode", Category: "CONVERTER", Desc: "Short alias of .texttourl", Hidden: true, Run: handleTexttourl})
	Register(Command{Name: "urldecodetotext", Category: "CONVERTER", Desc: "Short alias of .urltotext", Hidden: true, Run: handleUrltotext})
}
