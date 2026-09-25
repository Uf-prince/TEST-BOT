package goldcmds

import (
	"fmt"
	"strings"
)

// ============================================================================
// GOLD-MD — MENU STYLE COMMANDS
// File: menustylecmd.go
// ----------------------------------------------------------------------------
// OWNER ORDER: the per-menu style commands (.menustyle / .logostyle /
// .fontstyle / .aimenustyle / .botmenustyle ...) were REMOVED — the single
// .botstyle command (botskincmd.go) is the only style switch. It sets the
// text skin AND the menu chrome for the whole bot in one shot.
// ============================================================================

func init() {
	// OWNER ORDER: per-menu style commands (.menustyle / .logostyle /
	// .aimenustyle / .botmenustyle ...) are REMOVED. The single .botstyle
	// command is the only style switch — it already sets the text skin AND the
	// menu chrome across the whole bot. menuStyleFor() resolves every menu from
	// the botstyle setting, so nothing is left half-styled.
	_ = menuStyleCommands // keep the helper (tests + guide reference it)
}

// currentStyleLabel resolves the effective style for a stored value, for the
// guide's CURRENT line. An empty value means "inherit/default".
func currentStyleLabel(s SessionBridge, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "DEFAULT (1)"
	}
	n, ok := ParseMenuStyleArg(raw)
	if !ok {
		return "DEFAULT (1)"
	}
	return fmt.Sprintf("%d — %s", n, MenuStyleName(n))
}
