package goldcmds

// ============================================================================
// GOLD-MD — PREFIX Command  (show info + change prefix)
// File: prefix.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.prefix).  Text style SAME TO
// SAME, only the 👑 emoji is replaced with 🔰 everywhere.  Work is identical
// to the Node bot:
//
//   .prefix            → show current prefix + full change guide
//   .prefix null       → remove prefix (commands work without prefix)
//   .prefix <symbol>   → set new prefix (any keyboard symbol or emoji, max 5)
//
// Persisted via the SessionBridge prefix mechanism (handled by the main
// package's resolvePrefix / SetPrefix — same Redis key the prefix command
// command uses).  Here we call a bridge helper that mirrors the core
// command behaviour.  Owner-only.
// ============================================================================

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handlePrefix(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	argRaw := ""
	if len(args) > 0 {
		argRaw = strings.TrimSpace(strings.Join(args, " "))
	}

	// no arg → current prefix info + guide
	if argRaw == "" {
		s.Reply(info, fmt.Sprintf(
			"*🔰 PREFIX INFO 🔰*\n\n"+
				"*CURRENT PREFIX :❱ %s*\n\n"+
				"*TYPE LIKE THIS TO CHANGE :*\n"+
				"*PREFIX WORKS FOR ALL SYMBOLS OF KEYBOARD*\n"+
				"*PREFIX .*\n"+
				"*PREFIX +*\n"+
				"*PREFIX ?*\n"+
				"*PREFIX !*\n"+
				"*ALL KEYBOARD SYMBOLS ARE WORK FOR PREFIX*\n\n"+
				"*ALL EMOJIES CAN ACCESS PREFIX*\n\n"+
				"*PREFIX 🔰*\n"+
				"*PREFIX 🔰*\n"+
				"*PREFIX 🔰*\n"+
				"*SET ANY EMOJIE FOR PREFIX*\n"+
				"*PREFIX NULL*\n"+
				"*WITHOUT PREFIX COMMANDS WORK*\n"+
				"*TYPE COMMMANDS WITHOUT PREFIX SAME LIKE THIS*\n"+
				"*MENU*\n"+
				"*PING*\n"+
				"*VIDEO*\n\n"+
				"*ALL COMMANDS WORK WITHOUT PREFIX*",
			prefix))
		return
	}

	// null / none → prefix-less mode
	lower := strings.ToLower(argRaw)
	if lower == "null" || lower == "none" {
		s.SetPrefix("")
		s.Reply(info, "*🔰 PREFIX REMOVED 🔰*\n\n*AB SAARI COMMANDS BINA PREFIX KE CHALEIN GI (EXAMPLE :❱ menu)*")
		return
	}

	// new prefix (first token, max 5 chars)
	newPfx := strings.Fields(argRaw)[0]
	if len(newPfx) > 5 {
		s.Reply(info, "*🔰 PREFIX BOHOT LAMBA HAI 🔰*\n\n*MAX 5 CHARACTERS TAK RAKHO*")
		return
	}

	s.SetPrefix(newPfx)
	s.Reply(info, fmt.Sprintf("*🔰 PREFIX CHANGED 🔰*\n\n*NEW PREFIX :❱ %s*", newPfx))
}

func init() {
	Register(Command{
		Name:      "prefix",
		Category:  "OWNER & SYSTEM",
		Desc:      "Show prefix info / change the command prefix (owner)",
		OwnerOnly: true,
		Run:       handlePrefix,
	})
	// .setprefix has been REMOVED from the bot — .prefix (above) is the
	// single prefix management command (UMAR-MD pair.js style).
}
