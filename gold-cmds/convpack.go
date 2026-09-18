package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK (shared helpers)
// File: convpack.go
// ============================================================================
// Shared helpers used by the CONVERTER category packs (convpack28..37).
// Every converter follows the GOLD-MD design language exactly:
// bold **, 🔰, ❮ ❯, ALL-CAPS.
// ============================================================================

import (
	"context"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// convGuide builds the standard "how to use" guide for a converter command.
func convGuide(prefix, title, cmd, fromUnit, toUnit string) string {
	return "*🔰 " + title + " 🔰*\n\n" +
		"*CONVERT " + fromUnit + " TO " + toUnit + "*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + strings.ToUpper(cmd) + " <VALUE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + strings.ToUpper(cmd) + " 10 ❯*"
}

// makeConvGuide returns a guide function bound to a converter.
func makeConvGuide(title, cmd, fromUnit, toUnit string) func(string) string {
	return func(prefix string) string {
		return convGuide(prefix, title, cmd, fromUnit, toUnit)
	}
}

// makeConvHandler returns a handler that converts a single numeric argument
// using fn and replies in the GOLD-MD design language.
func makeConvHandler(title, cmd, fromUnit, toUnit string, fn func(float64) float64) func(SessionBridge, types.MessageInfo, []string, string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		RunWithTimeout(s, info, func(ctx context.Context) {
			if len(args) == 0 {
				s.Reply(info, convGuide(prefix, title, cmd, fromUnit, toUnit))
				return
			}
			raw := strings.TrimSpace(args[0])
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
				return
			}
			out := fn(v)
			var b strings.Builder
			b.WriteString("*🔰 " + title + " 🔰*\n\n")
			b.WriteString("*📥 INPUT ❯ " + strconv.FormatFloat(v, 'f', -1, 64) + " " + fromUnit + "*\n")
			b.WriteString("*📤 OUTPUT ❯ " + strconv.FormatFloat(out, 'f', -1, 64) + " " + toUnit + "*")
			s.Reply(info, b.String())
		})
	}
}
