package goldcmds

// ============================================================================
// GOLD-MD — .cmdowner / .cmdpublic / .cmdownerpublic command (cmdaccess system)
// File: cmdowner.go
// ----------------------------------------------------------------------------
// The owner can make any command owner-only or public — live, no restart.
//
//   .cmdchange                          → full guide (this is the menu entry)
//   .cmdowner ping                      → ping is now owner-only (silent for public)
//   .cmdowner ping,menu,alive           → multiple commands at once
//   .cmdpublic ping                     → ping is public again
//   .cmdownerpublic reset               → all commands back to normal
//
// HANDLER HOOK: handler.go runs CmdAccessCheck BEFORE dispatch —
//   - command is in the owner-only list + sender is not the owner → silent return
//   - .cmdowner / .cmdpublic / .cmdownerpublic are owner-only themselves
//
// ALIASES: registry names sharing the same Run func are siblings. Marking one
//   as owner-only covers all its aliases (alias-expanded on save).
//
// STORAGE (Redis, settings:<botJID> hash — Redis-safe):
//   field "cmdowner" = comma-joined owner-only command names ("" = none)
//   reset → DelStatusSetting (HDEL — full delete, no orphan fields)
//   Cache: 1-minute TTL in-process, write-through invalidate (bancmd pattern)
// ============================================================================

import (
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── storage field (Redis settings:<botJID> hash) ───────────────────────────

const cmdOwnerField = "cmdowner"

// ── in-process cache (1-minute TTL, write-through invalidate) ──────────────

var (
	caMu  sync.Mutex
	caSet = map[string]map[string]bool{}
	caAt  = map[string]time.Time{}
)

// caLoad reads the owner-only set for this bot from Redis (cached).
func caLoad(s SessionBridge) map[string]bool {
	botJID := s.GetJID()
	caMu.Lock()
	if set, ok := caSet[botJID]; ok && time.Since(caAt[botJID]) < time.Minute {
		caMu.Unlock()
		return set
	}
	caMu.Unlock()

	raw := s.GetStatusSetting(cmdOwnerField, "")
	set := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			set[p] = true
		}
	}

	caMu.Lock()
	caSet[botJID] = set
	caAt[botJID] = time.Now()
	caMu.Unlock()
	return set
}

func caInvalidate(botJID string) {
	caMu.Lock()
	delete(caSet, botJID)
	delete(caAt, botJID)
	caMu.Unlock()
}

// caRawList returns the ordered owner-only list (for the guide text).
func caRawList(s SessionBridge) []string {
	raw := s.GetStatusSetting(cmdOwnerField, "")
	var list []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			list = append(list, p)
		}
	}
	return list
}

// ── alias resolution ────────────────────────────────────────────────────────
// Registry names sharing the same Run func are siblings (bancmd pattern).
// .cmdowner ping marks ping AND all its aliases as owner-only.

func caResolveAliases(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	target, ok := goldRunFuncOf(name)
	if !ok {
		return []string{name} // unknown/core name — still markable
	}
	var out []string
	seen := map[string]bool{}
	targetPtr := goldRunFuncPtr(target)
	for _, c := range Commands() {
		if goldRunFuncPtr(c.Run) == targetPtr {
			if !seen[strings.ToLower(c.Name)] {
				seen[strings.ToLower(c.Name)] = true
				out = append(out, strings.ToLower(c.Name))
			}
		}
	}
	return out
}

// ── ENFORCEMENT (handler.go hook) ───────────────────────────────────────────

// CmdAccessIsOwnerOnly reports whether the command is currently in the
// owner-only list for this bot (aliases included, name already resolved).
func CmdAccessIsOwnerOnly(s SessionBridge, command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return false
	}
	return caLoad(s)[command]
}

// ── text helpers ────────────────────────────────────────────────────────────

func caDots(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "."+n)
	}
	return out
}

func caJoinComma(items []string) string { return strings.Join(items, ", ") }

func sortStringsCA(list []string) {
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
}

// cmdChangeGuide — full 🔰 styled guide (menu entry + .cmdchange reply).
func cmdChangeGuide(prefix string) string {
	return "*🔰 COMMAND ACCESS GUIDE 🔰*\n\n" +
		"*🔰 TO MAKE A COMMAND OWNER-ONLY :❱*\n" +
		"*CMDOWNER ❮ COMMAND NAME ❯*\n" +
		"*" + prefix + "cmdowner ping*\n" +
		"*" + prefix + "cmdowner ping,menu,alive*\n" +
		"*THE COMMAND WHOSE NAME YOU TYPE GETS ADDED TO THE OWNER COMMAND LIST — ONLY YOU CAN USE IT, THE BOT STAYS COMPLETELY SILENT FOR EVERYONE ELSE*\n\n" +
		"*🔰 TO MAKE A COMMAND PUBLIC :❱*\n" +
		"*CMDPUBLIC ❮ COMMAND NAME ❯*\n" +
		"*" + prefix + "cmdpublic ping*\n" +
		"*" + prefix + "cmdpublic ping,menu,alive*\n" +
		"*THE COMMAND YOU MAKE PUBLIC WORKS FOR EVERYONE AGAIN AND IS REMOVED FROM THE OWNER LIST*\n\n" +
		"*🔰 TO RESET EVERYTHING BACK TO NORMAL :❱*\n" +
		"*" + prefix + "cmdownerpublic reset*\n" +
		"*ALL COMMANDS GO BACK TO THEIR DEFAULT STATE — NO COMMAND REMAINS MARKED AS OWNER-ONLY OR PUBLIC, EVERYTHING WORKS EXACTLY LIKE BEFORE*\n\n" +
		"*🔰 TO SHOW THE OWNER-ONLY COMMAND LIST :❱*\n" +
		"*" + prefix + "cmdowner list*\n" +
		"*" + prefix + "cmdpublicowner list*\n" +
		"*SHOWS EVERY COMMAND THAT IS CURRENTLY MARKED OWNER-ONLY*\n\n" +
		"*🔰 NOTE :❱*\n" +
		"*ALIASES OF A COMMAND CHANGE TOGETHER WITH IT*\n" +
		"*IF A COMMAND IS IN THE OWNER LIST THE BOT ONLY REPLIES TO THE OWNER*\n" +
		"*FOR THE PUBLIC THE BOT DOES NOTHING (NO REPLY)*"
}

// ── main handler ────────────────────────────────────────────────────────────

func handleCmdOwner(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 🔰*")
		return
	}

	argRaw := strings.TrimSpace(strings.Join(args, " "))

	// no arg → full guide
	if argRaw == "" {
		s.Reply(info, cmdChangeGuide(prefix))
		return
	}

	// subcommand dispatch
	first := strings.ToLower(strings.Fields(argRaw)[0])
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(argRaw), first))

	switch {
	case first == "reset" && rest == "":
		// .cmdowner reset — also supported (same as cmdownerpublic reset)
		handleCmdAccessReset(s, info, prefix)
		return
	case first == "reset" && strings.EqualFold(rest, "all"):
		handleCmdAccessReset(s, info, prefix)
		return
	case first == "list" && rest == "":
		handleCmdAccessList(s, info, prefix)
		return
	}

	// default: .cmdowner <name>[,<name2>...] → add to owner-only list
	// NOTE: pass the FULL argRaw (not rest) — for ".cmdowner ping" the first
	// token IS the command name, so rest would be empty.
	handleCmdAccessAdd(s, info, prefix, argRaw, "owner")
}

func handleCmdPublic(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 🔰*")
		return
	}

	argRaw := strings.TrimSpace(strings.Join(args, " "))
	if argRaw == "" {
		s.Reply(info, "*🔰 USAGE :❱*\n\n*"+prefix+"cmdpublic ❮ COMMAND NAME ❯*\n\n"+
			"*"+prefix+"cmdpublic ping*\n"+
			"*"+prefix+"cmdpublic ping,menu,alive*\n\n"+
			"*TYPE THE NAME OF THE COMMAND YOU WANT TO MAKE PUBLIC — IT IS REMOVED FROM THE OWNER LIST AND STARTS WORKING FOR EVERYONE AGAIN*\n\n"+
			"*TO RESET EVERYTHING BACK TO NORMAL :❱ "+prefix+"cmdownerpublic reset*")
		return
	}

	handleCmdAccessAdd(s, info, prefix, argRaw, "public")
}

func handleCmdOwnerPublic(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 🔰*")
		return
	}

	argRaw := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if argRaw == "list" {
		handleCmdAccessList(s, info, prefix)
		return
	}
	if argRaw == "" || argRaw != "reset" {
		s.Reply(info, "*🔰 USAGE :❱*\n\n*"+prefix+"cmdownerpublic reset*\n"+
			"*"+prefix+"cmdownerpublic list*\n\n"+
			"*RESET : ALL COMMANDS GO BACK TO THEIR DEFAULT STATE — NO COMMAND REMAINS MARKED AS OWNER-ONLY OR PUBLIC, EVERYTHING WORKS EXACTLY LIKE BEFORE*\n"+
			"*LIST : SHOW THE CURRENT OWNER-ONLY COMMAND LIST*\n\n"+
			"*FULL GUIDE :❱ "+prefix+"cmdchange*")
		return
	}

	handleCmdAccessReset(s, info, prefix)
}

// ── core operations ─────────────────────────────────────────────────────────

// handleCmdAccessAdd — mode "owner" (add to owner-only list) or "public"
// (remove from list). Multiple names comma/space separated.
func handleCmdAccessAdd(s SessionBridge, info types.MessageInfo, prefix, namesRaw, mode string) {
	botJID := s.GetJID()

	// parse names (comma or space separated), normalize + alias-expand
	var names []string
	seen := map[string]bool{}
	for _, tok := range strings.FieldsFunc(namesRaw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		tok = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tok), ".")))
		if tok == "" {
			continue
		}
		for _, n := range caResolveAliases(tok) {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		s.Reply(info, "*🔰 NO COMMAND NAME FOUND :❱*\n\n*TYPE IT LIKE "+prefix+"cmdowner ping — MULTIPLE NAMES SEPARATED BY COMMAS ALSO WORK*")
		return
	}

	// protect system commands from being locked out
	locked := map[string]bool{
		"cmdowner":       true,
		"cmdpublic":      true,
		"cmdownerpublic": true,
		"cmdchange":      true,
		"cmdname":        true,
		"menu":           true,
	}
	for _, n := range names {
		if locked[n] {
			s.Reply(info, "*🔰 "+strings.ToUpper(n)+" CANNOT BE LOCKED :❱*\n\n*IT IS A SYSTEM COMMAND — LOCKED FOR YOUR OWN SAFETY SO YOU CAN NEVER LOCK YOURSELF OUT*")
			return
		}
	}

	// known-command check (warn on totally unknown, still allow — core cmds
	// + future names) — same approach as bancmd
	unknown := []string{}
	known := cmdAccessKnownSet()
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}

	if mode == "owner" {
		// add to owner-only list
		cur := caRawList(s)
		set := map[string]bool{}
		for _, c := range cur {
			set[c] = true
		}
		added := []string{}
		for _, n := range names {
			if !set[n] {
				set[n] = true
				added = append(added, n)
			}
		}
		var out []string
		for c := range set {
			out = append(out, c)
		}
		sortStringsCA(out)
		s.SetStatusSetting(cmdOwnerField, strings.Join(out, ","))

		caInvalidate(botJID)

		msg := "*🔰 OWNER-ONLY COMMANDS UPDATED 🔰*\n\n" +
			"*ADDED :❱ " + caJoinComma(caDots(added)) + "*\n\n" +
			"*NOW ONLY THE OWNER CAN USE THESE COMMANDS — THE BOT STAYS COMPLETELY SILENT FOR EVERYONE ELSE*\n"
		if len(cur) > 0 {
			msg += "\n*FULL OWNER-ONLY LIST :❱ " + caJoinComma(caDots(out)) + "*"
		}
		if len(unknown) > 0 {
			msg += "\n\n*🔰 THESE NAMES WERE NOT FOUND IN THE REGISTRY (ADDED ANYWAY) :❱ " + caJoinComma(unknown) + "*"
		}
		s.Reply(info, msg)
		return
	}

	// mode == "public" — remove from owner-only list
	cur := caRawList(s)
	var keep []string
	removedSet := map[string]bool{}
	for _, n := range names {
		removedSet[n] = true
	}
	for _, c := range cur {
		if !removedSet[c] {
			keep = append(keep, c)
		}
	}
	s.SetStatusSetting(cmdOwnerField, strings.Join(keep, ","))
	caInvalidate(botJID)

	removed := []string{}
	for _, n := range names {
		for _, c := range cur {
			if c == n {
				removed = append(removed, n)
				break
			}
		}
	}

	msg := "*🔰 PUBLIC COMMANDS UPDATED 🔰*\n\n"
	if len(removed) > 0 {
		msg += "*REMOVED FROM THE OWNER LIST :❱ " + caJoinComma(caDots(removed)) + "*\n\n" +
			"*THESE COMMANDS NOW WORK FOR EVERYONE AGAIN*\n"
	} else {
		msg += "*NONE OF THESE COMMANDS WERE IN THE OWNER LIST — THEY ARE ALL ALREADY PUBLIC*\n\n"
	}
	if len(keep) > 0 {
		msg += "\n*FULL OWNER-ONLY LIST :❱ " + caJoinComma(caDots(keep)) + "*"
	} else {
		msg += "\n*THE OWNER-ONLY LIST IS NOW EMPTY — ALL COMMANDS ARE PUBLIC*"
	}
	if len(unknown) > 0 {
		msg += "\n\n*🔰 THESE NAMES WERE NOT FOUND IN THE REGISTRY :❱ " + caJoinComma(unknown) + "*"
	}
	s.Reply(info, msg)
}

// handleCmdAccessReset — everything back to normal (Redis-safe delete).
func handleCmdAccessReset(s SessionBridge, info types.MessageInfo, prefix string) {
	botJID := s.GetJID()

	cur := caRawList(s)
	s.DelStatusSetting(cmdOwnerField) // HDEL — redis-safe, orphan-free
	caInvalidate(botJID)

	msg := "*🔰 COMMAND ACCESS HAS BEEN RESET 🔰*\n\n" +
		"*ALL COMMANDS ARE BACK TO THEIR DEFAULT STATE — EXACTLY LIKE BEFORE*\n"
	if len(cur) > 0 {
		msg += "\n*PREVIOUSLY OWNER-ONLY (NOW PUBLIC) :❱ " + caJoinComma(caDots(cur)) + "*"
	} else {
		msg += "\n*NO COMMAND WAS MARKED OWNER-ONLY — EVERYTHING WAS ALREADY NORMAL*"
	}
	msg += "\n\n*TO MAKE A COMMAND OWNER-ONLY :❱ " + prefix + "cmdowner ❮ NAME ❯*\n" +
		"*TO MAKE A COMMAND PUBLIC :❱ " + prefix + "cmdpublic ❮ NAME ❯*\n" +
		"*FULL GUIDE :❱ " + prefix + "cmdchange*"
	s.Reply(info, msg)
}

// handleCmdAccessList — current owner-only list (bonus, .cmdowner list).
func handleCmdAccessList(s SessionBridge, info types.MessageInfo, prefix string) {
	cur := caRawList(s)
	if len(cur) == 0 {
		s.Reply(info, "*🔰 THE OWNER-ONLY LIST IS EMPTY :❱*\n\n"+
			"*NO COMMAND IS MARKED OWNER-ONLY RIGHT NOW — ALL COMMANDS ARE PUBLIC*\n\n"+
			"*TO ADD ONE :❱ "+prefix+"cmdowner ❮ NAME ❯*")
		return
	}
	s.Reply(info, "*🔰 OWNER-ONLY COMMANDS :❱*\n\n"+
		"*"+caJoinComma(caDots(cur))+"*\n\n"+
		"*TO MAKE A COMMAND PUBLIC :❱ "+prefix+"cmdpublic ❮ NAME ❯*\n"+
		"*TO RESET EVERYTHING :❱ "+prefix+"cmdownerpublic reset*")
}

// ── known-command set (cmdname hook — registry + core names) ────────────────

func cmdAccessKnownSet() map[string]bool {
	out := map[string]bool{}
	for _, c := range Commands() {
		out[strings.ToLower(c.Name)] = true
	}
	if cmdNameKnownHook != nil {
		for _, n := range cmdNameKnownHook() {
			out[strings.ToLower(n)] = true
		}
	}
	return out
}

// ── registration ────────────────────────────────────────────────────────────

func init() {
	Register(Command{
		Name:      "cmdchange",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO SHOW THE GUIDE FOR CHANGING COMMAND NAMES. IT SHOWS HOW TO RENAME BOT COMMANDS.",
		OwnerOnly: true,
		Run:       handleCmdOwnerGuide,
	})
	Register(Command{Name: "cmdowner", Desc: "THIS COMMAND IS USED TO SET A COMMAND SO ONLY THE OWNER CAN USE IT.", Category: "OWNER & SYSTEM", OwnerOnly: true, Hidden: true, Run: handleCmdOwner})
	Register(Command{Name: "cmdpublic", Desc: "THIS COMMAND IS USED TO SET A COMMAND SO EVERYONE CAN USE IT.", Category: "OWNER & SYSTEM", OwnerOnly: true, Hidden: true, Run: handleCmdPublic})
	Register(Command{Name: "cmdownerpublic", Desc: "THIS COMMAND IS USED TO SET OWNER ONLY OR PUBLIC MODE FOR ALL COMMANDS AT ONCE.", Category: "OWNER & SYSTEM", OwnerOnly: true, Hidden: true, Run: handleCmdOwnerPublic})
	Register(Command{Name: "cmdpublicowner", Desc: "THIS COMMAND IS USED TO SET OWNER ONLY OR PUBLIC MODE FOR ALL COMMANDS AT ONCE.", Category: "OWNER & SYSTEM", OwnerOnly: true, Hidden: true, Run: handleCmdOwnerPublic})
}

// handleCmdOwnerGuide — .cmdchange → full guide (this is the menu entry too).
func handleCmdOwnerGuide(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 🔰*")
		return
	}
	s.Reply(info, cmdChangeGuide(prefix))
}
