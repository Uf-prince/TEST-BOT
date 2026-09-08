package goldcmds

// ============================================================================
// GOLD-MD — OWNER NUMBER Command  (set main owner / add / del sudo / list)
// File: ownernumber.go
// ----------------------------------------------------------------------------
// Menu ka "OWNER" field isi se update hota hai.
//
//   .ownernumber                  → show all owners (main + sudo) + guide
//   .ownernumber 923xxx           → SET the main owner NUMBER (menu OWNER field)
//                                   (ye Redis field "ownernumber" set karta hai)
//   .ownernumber add 923xxx       → add one sudo owner
//   .ownernumber add 923xxx,...   → add multiple (comma-separated)
//   .ownernumber del 923xxx       → delete one sudo owner
//   .ownernumber del 923xxx,...   → delete multiple (comma-separated)
//   .ownernumber reset            → clear main owner number + all sudo owners
//
// Persisted in Redis:
//   field "ownernumber" → main owner number (digits only)  [menu OWNER field]
//   field "sudowners"   → comma-separated sudo owner numbers (digits only)
//
// .sudo is a HIDDEN alias (not shown in menu) — same handler.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handleOwnerNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	aiArgs := strings.TrimSpace(strings.Join(args, " "))
	permanentNum := s.GetPermanentOwnerNumber()
	sudoList := s.GetSudoOwners()
	mainOwner := s.GetOwnerNumberSetting(permanentNum)

	// no arg → show all owners + change guide
	if aiArgs == "" {
		var b strings.Builder
		b.WriteString("*🟰 OWNER NUMBER INFO 🟰*\n\n")
		b.WriteString("*MAIN OWNER (MENU OWNER FIELD)*\n")
		b.WriteString("*🟢 " + mainOwner + "*\n")
		b.WriteString("*CHANGE WITH: ❮ " + prefix + "OWNERNUMBER 923001234567 ❯*\n\n")

		if len(sudoList) > 0 {
			b.WriteString("*SUDO OWNERS LIST*\n")
			for _, n := range sudoList {
				b.WriteString("*🟢 " + n + "*\n")
			}
			b.WriteString("\n")
		} else {
			b.WriteString("*SUDO OWNERS :❯ NONE*\n\n")
		}

		b.WriteString("*TO ADD SUDO OWNER :❯*\n")
		b.WriteString("*TYPE ❮ " + prefix + "OWNERNUMBER ADD 923001234567 ❯*\n")
		b.WriteString("*MULTIPLE :❯ ❮ " + prefix + "OWNERNUMBER ADD 923xxx,923xxx,923xxx ❯*\n\n")
		b.WriteString("*TO DELETE SUDO OWNER :❯*\n")
		b.WriteString("*TYPE ❮ " + prefix + "OWNERNUMBER DEL 923001234567 ❯*\n")
		b.WriteString("*MULTIPLE :❯ ❮ " + prefix + "OWNERNUMBER DEL 923xxx,923xxx ❯*\n\n")
		b.WriteString("*TO RESET EVERYTHING :❯*\n")
		b.WriteString("*TYPE ❮ " + prefix + "OWNERNUMBER RESET ❯*\n\n")
		b.WriteString("*NUMBER FORMAT: Country Code + Number*\n")
		b.WriteString("*NO + OR SPACES*")
		s.Reply(info, b.String())
		return
	}

	// parse subcommand: add / del / reset — OR a plain number (set main owner)
	lower := strings.ToLower(aiArgs)

	if lower == "reset" {
		// clear main owner number + all sudo owners
		if d, ok := s.(interface{ DelOwnerNumberSetting() }); ok {
			_ = d
		}
		// delete the ownernumber field directly via Redis-safe delete
		s.SetOwnerNumberSetting("")
		// best-effort: if bridge exposes Del, use it; otherwise empty string is fine
		s.SetSudoOwners(nil)
		s.Reply(info, "*🟰 OWNER NUMBER RESET ✅*\n\n*MAIN OWNER RESET TO PAIRED NUMBER :❯ "+permanentNum+"*\n*ALL SUDO OWNERS CLEARED*")
		return
	}

	// split into subcommand + rest
	parts := strings.SplitN(aiArgs, " ", 2)
	sub := strings.ToLower(strings.TrimSpace(parts[0]))

	// ── PLAIN NUMBER → set the main owner number (menu OWNER field) ──
	// If first token is all digits (7-15), treat it as the main owner number.
	firstClean := cleanDigits(parts[0])
	if (sub != "add" && sub != "del") && len(firstClean) >= 7 && len(firstClean) <= 15 {
		// could be a single number with no spaces, OR "923xxx 923xxx" — join & clean
		rawAll := cleanDigits(strings.Join(parts, ""))
		if len(rawAll) >= 7 && len(rawAll) <= 15 {
			s.SetOwnerNumberSetting(rawAll)
			s.Reply(info, "*🟰 OWNER NUMBER SET ✅*\n\n*MENU OWNER FIELD UPDATED*\n*NEW OWNER NUMBER :❯ "+rawAll+"*\n\n*USE ❮ "+prefix+"MENU ❯ TO SEE IT*")
			return
		}
	}

	if len(parts) < 2 {
		s.Reply(info, "*❌ INVALID FORMAT*\n\n*SET MAIN OWNER:*\n*❮ "+prefix+"OWNERNUMBER 923001234567 ❯*\n\n*ADD SUDO:*\n*❮ "+prefix+"OWNERNUMBER ADD 923001234567 ❯*\n\n*DEL SUDO:*\n*❮ "+prefix+"OWNERNUMBER DEL 923001234567 ❯*")
		return
	}
	numbersRaw := strings.TrimSpace(parts[1])

	// parse comma-separated numbers, keep digits only, validate 7-15
	var nums []string
	for _, chunk := range strings.Split(numbersRaw, ",") {
		clean := cleanDigits(strings.TrimSpace(chunk))
		if len(clean) < 7 || len(clean) > 15 {
			s.Reply(info, "*❌ INVALID NUMBER*\n\n*Please enter a valid number*\n*EXAMPLE: 923001234567*")
			return
		}
		nums = append(nums, clean)
	}

	switch sub {
	case "add":
		// add numbers that aren't already in the list and aren't the permanent owner
		existing := make(map[string]bool)
		existing[permanentNum] = true
		for _, n := range sudoList {
			existing[n] = true
		}
		var added []string
		for _, n := range nums {
			if !existing[n] {
				sudoList = append(sudoList, n)
				existing[n] = true
				added = append(added, n)
			}
		}
		s.SetSudoOwners(sudoList)
		if len(added) > 0 {
			s.Reply(info, "*🟰 SUDO OWNER ADDED ✅*\n\n*NEW SUDO OWNERS :❯*\n*🟢 "+strings.Join(added, "\n🟢 ")+"*")
		} else {
			s.Reply(info, "*ℹ️ NUMBER(S) ALREADY IN OWNER LIST*")
		}

	case "del":
		// check if any number is the permanent (paired) owner → refuse
		for _, n := range nums {
			if n == permanentNum {
				s.Reply(info, "*THIS IS PERMANENT BOT OWNER YOU CAN'T DELETE THIS NUMBER FROM OWNER LIST*")
				return
			}
		}
		// remove from sudo list
		toRemove := make(map[string]bool)
		for _, n := range nums {
			toRemove[n] = true
		}
		var newList []string
		var removed []string
		for _, n := range sudoList {
			if toRemove[n] {
				removed = append(removed, n)
			} else {
				newList = append(newList, n)
			}
		}
		s.SetSudoOwners(newList)
		if len(removed) > 0 {
			s.Reply(info, "*🟰 SUDO OWNER DELETED ✅*\n\n*REMOVED :❯*\n*❌ "+strings.Join(removed, "\n❌ ")+"*")
		} else {
			s.Reply(info, "*ℹ️ NUMBER(S) NOT FOUND IN SUDO OWNER LIST*")
		}

	default:
		s.Reply(info, "*❌ INVALID SUBCOMMAND*\n\n*USE: a plain number OR add / del / reset*\n*EXAMPLE: ❮ "+prefix+"OWNERNUMBER 923001234567 ❯*\n*EXAMPLE: ❮ "+prefix+"OWNERNUMBER ADD 923001234567 ❯*")
	}
}

// cleanDigits extracts only digit characters from a string.
func cleanDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// botOwnNumber extracts the raw phone number (digits) from a JID like
// "923001234567@s.whatsapp.net" or "923001234567:5@s.whatsapp.net".
func botOwnNumber(jid string) string {
	// strip everything from the first '@' onward
	if i := strings.Index(jid, "@"); i >= 0 {
		jid = jid[:i]
	}
	// strip the :device suffix
	if i := strings.Index(jid, ":"); i >= 0 {
		jid = jid[:i]
	}
	// keep digits only
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, jid)
}

func init() {
	Register(Command{
		Name:      "ownernumber",
		Category:  "OWNER & SYSTEM",
		Desc:      "Set main owner number / add / del sudo owners (menu OWNER field)",
		OwnerOnly: true,
		Run:       handleOwnerNumber,
	})
	// hidden aliases (same as Node.js) — NOT shown in menu
	Register(Command{Name: "setownernumber", OwnerOnly: true, Hidden: true, Run: handleOwnerNumber})
	Register(Command{Name: "botnumber", OwnerOnly: true, Hidden: true, Run: handleOwnerNumber})
	Register(Command{Name: "sudo", OwnerOnly: true, Hidden: true, Run: handleOwnerNumber})
}
