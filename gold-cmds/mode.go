package goldcmds

// ============================================================================
// GOLD-MD — MODE Command  (public / private / groups / inbox)
// File: mode.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.mode).  Text style SAME TO SAME, only the
// 👑 emoji is replaced with 🔰 everywhere.  Work is identical: owner-only
// command that shows/changes the bot's work-mode, persisted in Redis
// settings:<botJID> field "mode".
//
//   .mode            → show current mode + guide
//   .mode public     → bot works everywhere (group + inbox)
//   .mode private    → bot works only for owner
//   .mode groups     → bot works only in groups
//   .mode inbox      → bot works only in private chats
//
// Mode enforcement happens in the dispatcher (handler.go) which reads the
// mode setting before running non-owner commands.
// ============================================================================

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handleMode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	// no arg → show current mode + guide
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		currentMode := s.GetModeSetting("public")
		s.Reply(info, fmt.Sprintf(
			"*🔰 BOT MODE INFO 🔰*\n\n"+
				"*TYPE ❰ %sMODE PUBLIC ❱*\n"+
				"*BOTS WORKS EVERWHERE*\n\n"+
				"*TYPE ❰ %sMODE PRIVATE ❱*\n"+
				"*BOT WORKS ONLY FOR OWNER*\n\n"+
				"*TYPE ❰ %sMODE GROUPS ❱*\n"+
				"*BOT WORKS ONLY IN GROUPS*\n\n"+
				"*TYPE ❰ %sMODE INBOX ❱*\n"+
				"*BOT WORKS ONLY IN PRIVATE CHATS*\n\n"+
				"*CURRENT MODE :❱ ❰ %s ❱*\n",
			prefix, prefix, prefix, prefix, strings.ToUpper(currentMode)))
		return
	}

	newMode := strings.ToLower(strings.TrimSpace(args[0]))
	validModes := map[string]bool{
		"public": true, "private": true, "groups": true, "inbox": true,
	}
	if !validModes[newMode] {
		s.Reply(info, fmt.Sprintf(
			"*🔰 INVALID MODE 🔰*\n\n"+
				"*AVAILABLE MODES:*\n"+
				"*TYPE ❰ %sMODE PUBLIC ❱*\n"+
				"*TYPE ❰ %sMODE PRIVATE ❱*\n"+
				"*TYPE ❰ %sMODE GROUPS ❱*\n"+
				"*TYPE ❰ %sMODE INBOX ❱*\n",
			prefix, prefix, prefix, prefix))
		return
	}

	s.SetModeSetting(newMode)

	var modeDesc string
	switch newMode {
	case "public":
		modeDesc = "🔰 BOT WORKS EVERYWHERE (GROUP + IB)"
	case "private":
		modeDesc = "🔰 BOT WORKS ONLY FOR OWNER"
	case "groups":
		modeDesc = "🔰 BOT WORKS ONLY IN GROUPS"
	case "inbox":
		modeDesc = "🔰 BOT WORKS ONLY IN PRIVATE CHATS"
	}

	s.Reply(info, fmt.Sprintf(
		"*🔰 MODE CHANGED 🔰*\n\n"+
			"*NEW MODE :❱ %s*\n\n"+
			"%s",
		strings.ToUpper(newMode), modeDesc))
}

func init() {
	Register(Command{
		Name:      "mode",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE BOT WORK MODE. IT CAN SET PUBLIC, PRIVATE, GROUPS OR INBOX MODE.",
		OwnerOnly: true,
		Run:       handleMode,
	})
	// hidden aliases (same as Node bot)
	Register(Command{Name: "workmode", OwnerOnly: true, Hidden: true, Run: handleMode})
	Register(Command{Name: "botmode", OwnerOnly: true, Hidden: true, Run: handleMode})
}
