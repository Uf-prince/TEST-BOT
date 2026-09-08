package goldcmds

// ============================================================================
// GOLD-MD — OWNER NAME Command  (set / reset / show)
// File: ownername.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.ownername / .setownername / .botowner).
// Text style SAME TO SAME 0% farak — only the 👑 crown emoji is replaced with
// 🔰 everywhere. Work is identical to the Node bot:
//
//   .ownername              → show current owner name + change guide
//   .ownername reset        → reset owner name to default "UMAR"
//   .ownername <your name>  → set a new owner name
//
// Persisted in Redis settings:<botJID> field "ownername". Owner-only.
//
// Aliases (same as Node.js): ownername, setownername, botowner
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// defaultOwnerName is the reset default for the owner name.
// Shown in .menu USER field and startup notification when not set yet.
const defaultOwnerName = "UMAR"

func handleOwnerName(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	aiArgs := strings.TrimSpace(strings.Join(args, " "))

	// current owner name (for the info display)
	currentOwnerName := s.GetOwnerNameSetting(defaultOwnerName)

	// no arg → show current owner name + change guide
	if aiArgs == "" {
		s.Reply(info, "*🔰 OWNER NAME INFO 🔰*\n\n*CURRENT OWNER NAME IS*\n*"+currentOwnerName+"*\n\n*TO CHANGE OWNER NAME :❯*\n*TYPE ❮ "+prefix+"OWNERNAME ❮ YOUR NAME ❯ ❯*\n\n*EXAMPLE LIKE THIS*\n*❮ "+prefix+"OWNERNAME UMAR BUG ❯*\n*❮ "+prefix+"OWNERNAME UMAR ❯* \n*❮ "+prefix+"OWNERNAME KING ❯*\n*TYPE YOUR NAME*\n\n*TO RESET TO DEFAULT :❯*\n*TYPE ❮ "+prefix+"OWNERNAME RESET ❯*")
		return
	}

	// reset → default "UMAR"
	if strings.ToLower(aiArgs) == "reset" {
		s.SetOwnerNameSetting(defaultOwnerName)
		s.Reply(info, "*🔰 OWNER NAME RESET ✅*\n\n*OWNER NAME :❯ UMAR*")
		return
	}

	// set new owner name
	s.SetOwnerNameSetting(aiArgs)
	s.Reply(info, "*🔰 OWNER NAME CHANGED ✅*\n\n*NEW OWNER NAME IS*\n "+aiArgs+"*")
}

func init() {
	Register(Command{
		Name:      "ownername",
		Category:  "OWNER & SYSTEM",
		Desc:      "Show/change the owner display name (reset = UMAR)",
		OwnerOnly: true,
		Run:       handleOwnerName,
	})
	// hidden aliases (same as Node.js)
	Register(Command{Name: "setownername", OwnerOnly: true, Hidden: true, Run: handleOwnerName})
	Register(Command{Name: "botowner", OwnerOnly: true, Hidden: true, Run: handleOwnerName})
}
