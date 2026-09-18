package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 34 (10 number base converters)
// File: convpack34.go
// ============================================================================
//   .dectobin  -> decimal to binary
//   .bintodec  -> binary to decimal
//   .dectohex  -> decimal to hexadecimal
//   .hextodec  -> hexadecimal to decimal
//   .dectooct  -> decimal to octal
//   .octtodec  -> octal to decimal
//   .bintohex  -> binary to hexadecimal
//   .hextobin  -> hexadecimal to binary
//   .octtohex  -> octal to hexadecimal
//   .hextooct  -> hexadecimal to octal
// ============================================================================

import (
	"context"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// baseConvHandler converts a value from one base to another.
func baseConvHandler(title, cmd string, fromBase, toBase int, fromName, toName string) func(SessionBridge, types.MessageInfo, []string, string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		RunWithTimeout(s, info, func(ctx context.Context) {
			if len(args) == 0 {
				s.Reply(info, convGuide(prefix, title, cmd, fromName, toName))
				return
			}
			raw := strings.TrimSpace(args[0])
			n, err := strconv.ParseInt(raw, fromBase, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID "+fromName+" NUMBER, PLEASE CHECK YOUR INPUT*")
				return
			}
			out := strconv.FormatInt(n, toBase)
			var b strings.Builder
			b.WriteString("*🔰 " + title + " 🔰*\n\n")
			b.WriteString("*📥 INPUT ❯ " + strings.ToUpper(raw) + " (" + fromName + ")*\n")
			b.WriteString("*📤 OUTPUT ❯ " + strings.ToUpper(out) + " (" + toName + ")*")
			s.Reply(info, b.String())
		})
	}
}

func init() {
	Register(Command{Name: "dectobin", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A DECIMAL NUMBER TO BINARY. USE IT AS .DECTOBIN <NUMBER>.", Run: baseConvHandler("DECIMAL TO BINARY", "dectobin", 10, 2, "DECIMAL", "BINARY")})
	Register(Command{Name: "bintodec", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A BINARY NUMBER TO DECIMAL. USE IT AS .BINTODEC <NUMBER>.", Run: baseConvHandler("BINARY TO DECIMAL", "bintodec", 2, 10, "BINARY", "DECIMAL")})
	Register(Command{Name: "dectohex", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A DECIMAL NUMBER TO HEXADECIMAL. USE IT AS .DECTOHEX <NUMBER>.", Run: baseConvHandler("DECIMAL TO HEX", "dectohex", 10, 16, "DECIMAL", "HEX")})
	Register(Command{Name: "hextodec", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A HEXADECIMAL NUMBER TO DECIMAL. USE IT AS .HEXTODEC <NUMBER>.", Run: baseConvHandler("HEX TO DECIMAL", "hextodec", 16, 10, "HEX", "DECIMAL")})
	Register(Command{Name: "dectooct", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A DECIMAL NUMBER TO OCTAL. USE IT AS .DECTOOCT <NUMBER>.", Run: baseConvHandler("DECIMAL TO OCTAL", "dectooct", 10, 8, "DECIMAL", "OCTAL")})
	Register(Command{Name: "octtodec", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT AN OCTAL NUMBER TO DECIMAL. USE IT AS .OCTTODEC <NUMBER>.", Run: baseConvHandler("OCTAL TO DECIMAL", "octtodec", 8, 10, "OCTAL", "DECIMAL")})
	Register(Command{Name: "bintohex", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A BINARY NUMBER TO HEXADECIMAL. USE IT AS .BINTOHEX <NUMBER>.", Run: baseConvHandler("BINARY TO HEX", "bintohex", 2, 16, "BINARY", "HEX")})
	Register(Command{Name: "hextobin", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A HEXADECIMAL NUMBER TO BINARY. USE IT AS .HEXTOBIN <NUMBER>.", Run: baseConvHandler("HEX TO BINARY", "hextobin", 16, 2, "HEX", "BINARY")})
	Register(Command{Name: "octtohex", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT AN OCTAL NUMBER TO HEXADECIMAL. USE IT AS .OCTTOHEX <NUMBER>.", Run: baseConvHandler("OCTAL TO HEX", "octtohex", 8, 16, "OCTAL", "HEX")})
	Register(Command{Name: "hextooct", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A HEXADECIMAL NUMBER TO OCTAL. USE IT AS .HEXTOOCT <NUMBER>.", Run: baseConvHandler("HEX TO OCTAL", "hextooct", 16, 8, "HEX", "OCTAL")})

	// hidden aliases
	Register(Command{Name: "dectobinary", Category: "CONVERTER", Desc: "Short alias of .dectobin", Hidden: true, Run: baseConvHandler("DECIMAL TO BINARY", "dectobin", 10, 2, "DECIMAL", "BINARY")})
	Register(Command{Name: "binarytodec", Category: "CONVERTER", Desc: "Short alias of .bintodec", Hidden: true, Run: baseConvHandler("BINARY TO DECIMAL", "bintodec", 2, 10, "BINARY", "DECIMAL")})
	Register(Command{Name: "dectohexadecimal", Category: "CONVERTER", Desc: "Short alias of .dectohex", Hidden: true, Run: baseConvHandler("DECIMAL TO HEX", "dectohex", 10, 16, "DECIMAL", "HEX")})
	Register(Command{Name: "hextodecimal", Category: "CONVERTER", Desc: "Short alias of .hextodec", Hidden: true, Run: baseConvHandler("HEX TO DECIMAL", "hextodec", 16, 10, "HEX", "DECIMAL")})
	Register(Command{Name: "dectooctal", Category: "CONVERTER", Desc: "Short alias of .dectooct", Hidden: true, Run: baseConvHandler("DECIMAL TO OCTAL", "dectooct", 10, 8, "DECIMAL", "OCTAL")})
	Register(Command{Name: "octaltodec", Category: "CONVERTER", Desc: "Short alias of .octtodec", Hidden: true, Run: baseConvHandler("OCTAL TO DECIMAL", "octtodec", 8, 10, "OCTAL", "DECIMAL")})
	Register(Command{Name: "binarytohex", Category: "CONVERTER", Desc: "Short alias of .bintohex", Hidden: true, Run: baseConvHandler("BINARY TO HEX", "bintohex", 2, 16, "BINARY", "HEX")})
	Register(Command{Name: "hextobinary", Category: "CONVERTER", Desc: "Short alias of .hextobin", Hidden: true, Run: baseConvHandler("HEX TO BINARY", "hextobin", 16, 2, "HEX", "BINARY")})
	Register(Command{Name: "octaltohex", Category: "CONVERTER", Desc: "Short alias of .octtohex", Hidden: true, Run: baseConvHandler("OCTAL TO HEX", "octtohex", 8, 16, "OCTAL", "HEX")})
	Register(Command{Name: "hextooctal", Category: "CONVERTER", Desc: "Short alias of .hextooct", Hidden: true, Run: baseConvHandler("HEX TO OCTAL", "hextooct", 16, 8, "HEX", "OCTAL")})
}
