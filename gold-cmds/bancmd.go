package goldcmds

// ============================================================================
// GOLD-MD — .cmdstop / .cmdstart / .cmdstoplist commands (bancmd system)
//
// Ported from Node.js pair.js (BannedCommand model L6977-7030, enforcement
// L11205-11240, handlers L16736-16850) — SAME WORK (0% farak), SAME TEXT
// (0% farak). Crown emoji in Node.js is replaced with 🔰 in GOLD-MD.
//
// COMMANDS (all owner-only):
//   .cmdstop / .bancmd        → stop commands (comma/space separated names)
//   .cmdstart / .unbancmd     → start (unban) stopped commands
//   .cmdstoplist / .bancmdlist / .bannedcmds / .bannedcommands → list
//
// ENFORCEMENT (handler.go hook, before command dispatch): if a command is
// banned and the sender is not the owner, the command is blocked with the
// exact Node.js text and processing stops. Owner bypasses — can always use
// any command AND always use cmdstop/cmdstart/cmdstoplist themselves.
//
// ALIASES: Node.js UmarResolvePluginAliases expands a name to its sibling
// aliases (same function). The Go equivalent is the command registry: names
// registered with the SAME Run func pointer are siblings. Banning one bans
// them all (e.g. banning "video" also bans its aliases).
//
// STORAGE (Redis, settings:<botJID> hash — same pattern as autoblock):
//   field "bancmd" = comma-joined banned command names
// ============================================================================

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ----------------------------------------------------------------------------
// banned-commands cache (1-minute TTL, write-through invalidate)
// ----------------------------------------------------------------------------

const bcField = "bancmd"

var (
	bcMu   sync.Mutex
	bcSet  = map[string]map[string]bool{}
	bcList = map[string][]string{}
	bcAt   = map[string]time.Time{}
)

// bcLoad reads the banned set for this bot from Redis (cached).
func bcLoad(s SessionBridge) map[string]bool {
	botJID := s.GetJID()
	bcMu.Lock()
	if set, ok := bcSet[botJID]; ok && time.Since(bcAt[botJID]) < time.Minute {
		bcMu.Unlock()
		return set
	}
	bcMu.Unlock()
	raw := s.GetBannedCommands("")
	var list []string
	set := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			if !set[p] {
				set[p] = true
				list = append(list, p)
			}
		}
	}
	bcMu.Lock()
	bcSet[botJID] = set
	bcList[botJID] = list
	bcAt[botJID] = time.Now()
	bcMu.Unlock()
	return set
}

func bcInvalidate(botJID string) {
	bcMu.Lock()
	delete(bcSet, botJID)
	delete(bcList, botJID)
	delete(bcAt, botJID)
	bcMu.Unlock()
}

// bcRawList returns the ordered banned list (for .cmdstoplist).
func bcRawList(s SessionBridge) []string {
	botJID := s.GetJID()
	bcMu.Lock()
	list, ok := bcList[botJID]
	fresh := ok && time.Since(bcAt[botJID]) < time.Minute
	bcMu.Unlock()
	if !fresh {
		bcLoad(s)
		bcMu.Lock()
		list = bcList[botJID]
		bcMu.Unlock()
	}
	return list
}

// goldResolveCommandAliases mirrors Node.js UmarResolvePluginAliases: a
// command name expands to itself + every registry name that shares the SAME
// Run function (siblings). Names not in the registry resolve to just
// themselves (still stoppable — same as unknown plugin names in Node.js).
func goldResolveCommandAliases(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	target, ok := goldRunFuncOf(name)
	if !ok {
		return []string{name}
	}
	var out []string
	seen := map[string]bool{}
	targetPtr := goldRunFuncPtr(target)
	for _, c := range Commands() {
		fn, ok := goldRunFuncOf(c.Name)
		if ok && goldRunFuncPtr(fn) == targetPtr {
			lower := strings.ToLower(c.Name)
			if !seen[lower] {
				seen[lower] = true
				out = append(out, lower)
			}
		}
	}
	if !seen[name] {
		out = append(out, name)
	}
	return out
}

// goldRunFuncOf returns the Run function registered for a command name.
// Multiple registry entries share the same Run pointer for aliases.
func goldRunFuncOf(name string) (cmdRunFn, bool) {
	for _, c := range Commands() {
		if strings.EqualFold(c.Name, name) {
			return c.Run, true
		}
	}
	return nil, false
}

// goldRunFuncPtr returns a comparable identity for a Run function. Go does
// not allow direct func==func comparison, so we use the code-pointer from
// reflect.ValueOf(...).Pointer(): funcs created from the same function
// literal share the same code pointer.
func goldRunFuncPtr(fn cmdRunFn) uintptr {
	if fn == nil {
		return 0
	}
	return reflect.ValueOf(fn).Pointer()
}

// cmdRunFn is the Run signature used for pointer comparison.
type cmdRunFn = func(s SessionBridge, info types.MessageInfo, args []string, prefix string)

// ----------------------------------------------------------------------------
// enforcement hook (called from handler.go before command dispatch)
// ----------------------------------------------------------------------------

// BancmdCheckBlocked mirrors the Node.js CMDSTOP check: returns true if the
// command is banned. Caller must ensure the sender is NOT the owner and the
// message is not from the bot itself.
func BancmdCheckBlocked(s SessionBridge, command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return false
	}
	set := bcLoad(s)
	if set[command] {
		return true
	}
	// Fallback: maybe the typed name is an alias whose sibling is banned
	// (same as Node.js sibling-alias check at enforcement time).
	for _, alias := range goldResolveCommandAliases(command) {
		if set[alias] {
			return true
		}
	}
	return false
}

// BancmdStopText is the exact Node.js enforcement reply text (0% farak).
func BancmdStopText() string {
	return "*THIS COMMAND HAS BEEN STOPPED BY ME* 😒\n\n" +
		"*ONLY I CAN USE THIS COMMAND*\n" +
		"*IT'S DISABLED FOR THE PUBLIC*"
}

// ----------------------------------------------------------------------------
// .cmdstop / .cmdstart / .cmdstoplist handlers — texts 0% farak (🔰 → 🔰)
// ----------------------------------------------------------------------------

// handleCmdStopList replies with the stopped-commands list.
func handleCmdStopList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "❌ *CMDSTOPLIST ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	all := bcRawList(s)
	if len(all) == 0 {
		s.Reply(info, "*NO COMMAND IS STOPPED*")
		return
	}
	var b strings.Builder
	for i, c := range all {
		b.WriteString(fmt.Sprintf("*%d.* .%s\n", i+1, c))
	}
	s.Reply(info, "*🔰 STOPPED COMMANDS LIST 🔰*\n\n"+
		strings.TrimRight(b.String(), "\n")+
		fmt.Sprintf("\n\n*TOTAL: %d COMMAND(S) STOPPED*", len(all)))
}

// handleCmdStop stops (bans) commands — comma or space separated names.
func handleCmdStop(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "❌ *CMDSTOP ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	// parse names: comma or space separated, lowercase, deduped
	names := bcParseNames(strings.Join(args, " "))
	if len(names) == 0 {
		s.Reply(info, fmt.Sprintf("*🔰 COMMAND STOP INFO 🔰*\n\n"+
			"*IF YOU WANT TO STOP A COMMAND, WRITE IT LIKE THIS:*\n\n"+
			"*TYPE ❮ %sCMDSTOP ❮COMMAND NAME❯ ❯*\n"+
			"*EXAMPLE LIKE THIS:*\n"+
			"*TYPE ❮ %sCMDSTOP VIDEO ❯*\n"+
			"*TYPE ❮ %sCMDSTOP FB ❯*\n"+
			"*TYPE ❮ %sCMDSTOP ANTILINK ❯*\n\n"+
			"*WRITE ANY COMMAND YOU WANT TO STOP LIKE THAT.*\n"+
			"*OR IF YOU WANT TO STOP MANY COMMANDS AT ONCE, WRITE LIKE THIS:*\n\n"+
			"*TYPE ❮ %sCMDSTOP CMD1,CMD2,CMD3 ❯*\n"+
			"*EXAMPLE LIKE THIS:*\n"+
			"*TYPE ❮ %sCMDSTOP VIDEO,INSTA,FB ❯*\n"+
			"*TYPE ❮ %sCMDSTOP ANTILINK,USERGCBAN,...... ❯*\n\n"+
			"*JUST DONOT GIVE SPACE AFTER THE COMMAND NAME. USE A COMMA , AND WRITE THE NEXT COMMAND NAME. THAT COMMAND WILL BE STOPPED IN THE BOT.*\n\n"+
			"*WHEN THE COMMAND IS STOPPED, THE BOT OWNER MEANING YOU CAN STILL USE THAT COMMAND YOURSELF.*\n\n"+
			"*FOR EVERYONE ELSE, NO ONE CAN USE THAT COMMAND IN YOUR INBOX OR GROUP UNTIL YOU START THE COMMAND AGAIN.*",
			prefix, prefix, prefix, prefix, prefix, prefix, prefix))
		return
	}
	// expand every name to its registry aliases (same Run func) — banning
	// one sibling bans them all (same as Node.js plugin-alias expansion)
	// preserve deterministic order: iterate original names, add their aliases
	var finalNames []string
	finalSeen := map[string]bool{}
	addFinal := func(n string) {
		if n != "" && !finalSeen[n] {
			finalSeen[n] = true
			finalNames = append(finalNames, n)
		}
	}
	for _, n := range names {
		for _, a := range goldResolveCommandAliases(n) {
			addFinal(a)
		}
	}
	// stop each name (already-stopped ones go to the "already" list)
	set := bcLoad(s)
	var already, newly []string
	for _, n := range finalNames {
		if set[n] {
			already = append(already, n)
		} else {
			s.BanCommand(n)
			newly = append(newly, n)
		}
	}
	bcInvalidate(s.GetJID())
	var text strings.Builder
	if len(newly) > 0 {
		text.WriteString(fmt.Sprintf("*✅ COMMAND STOPPED SUCCESS*\n*%s*\n"+
			"*THIS COMMAND IS STOPPED BY ME IN MY BOT. ONLY I CAN USE THIS COMMAND. YOU CANNOT USE THIS COMMAND UNTIL I START IT AGAIN IN MY BOT 😒*\n"+
			"*THESE ALL COMMANDS ARE STOPPED 😎*\n\n",
			strings.Join(prefixDots(newly), ", ")))
	}
	if len(already) > 0 {
		text.WriteString(fmt.Sprintf("*⚠️ ALREADY STOPPED*\n*%s*",
			strings.Join(prefixDots(already), ", ")))
	}
	s.Reply(info, strings.TrimSpace(text.String()))
}

// handleCmdStart starts (unbans) stopped commands.
func handleCmdStart(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "❌ *CMDSTART ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	names := bcParseNames(strings.Join(args, " "))
	if len(names) == 0 {
		s.Reply(info, fmt.Sprintf("*🔰 COMMAND START INFO 🔰*\n\n"+
			"*TO START A STOPPED COMMAND WRITE LIKE THIS:*\n\n"+
			"*TYPE ❮ %sCMDSTART ❮COMMAND NAME❯ ❯*\n"+
			"*EXAMPLE:*\n"+
			"*TYPE ❮ %sCMDSTART VIDEO ❯*", prefix, prefix))
		return
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		for _, a := range goldResolveCommandAliases(n) {
			nameSet[a] = true
		}
	}
	var finalNames []string
	finalSeen := map[string]bool{}
	for _, n := range names {
		for _, a := range goldResolveCommandAliases(n) {
			if a != "" && !finalSeen[a] {
				finalSeen[a] = true
				finalNames = append(finalNames, a)
			}
		}
	}
	set := bcLoad(s)
	var notBanned, unbanned []string
	for _, n := range finalNames {
		if !set[n] {
			notBanned = append(notBanned, n)
		} else {
			s.UnbanCommand(n)
			unbanned = append(unbanned, n)
		}
	}
	bcInvalidate(s.GetJID())
	var text strings.Builder
	if len(unbanned) > 0 {
		text.WriteString(fmt.Sprintf("*✅ COMMAND STARTED SUCCESS*\n*%s*\n"+
			"*THIS COMMAND IS NOW STARTED. NOW EVERYONE CAN USE THIS COMMAND 😇*\n\n",
			strings.Join(prefixDots(unbanned), ", ")))
	}
	if len(notBanned) > 0 {
		text.WriteString(fmt.Sprintf("*⚠️ ALREADY NOT STOPPED*\n*%s*",
			strings.Join(prefixDots(notBanned), ", ")))
	}
	s.Reply(info, strings.TrimSpace(text.String()))
}

// bcParseNames splits raw args into clean lowercase names (comma/space).
func bcParseNames(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		n := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(f, ".")))
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// prefixDots renders names as ".name" for the result texts.
func prefixDots(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "."+n)
	}
	return out
}

func init() {
	Register(Command{
		Name:      "cmdstop",
		Category:  "OWNER & SYSTEM",
		Desc:      "Stop commands from public use (owner only)",
		OwnerOnly: true,
		Run:       handleCmdStop,
	})
	Register(Command{Name: "bancmd", OwnerOnly: true, Hidden: true, Run: handleCmdStop})
	Register(Command{
		Name:      "cmdstart",
		Category:  "OWNER & SYSTEM",
		Desc:      "Start stopped commands again (owner only)",
		OwnerOnly: true,
		Run:       handleCmdStart,
	})
	Register(Command{Name: "unbancmd", OwnerOnly: true, Hidden: true, Run: handleCmdStart})
	Register(Command{
		Name:      "cmdstoplist",
		Category:  "OWNER & SYSTEM",
		Desc:      "List stopped commands (owner only)",
		OwnerOnly: true,
		Run:       handleCmdStopList,
	})
	Register(Command{Name: "bancmdlist", OwnerOnly: true, Hidden: true, Run: handleCmdStopList})
	Register(Command{Name: "bannedcmds", OwnerOnly: true, Hidden: true, Run: handleCmdStopList})
	Register(Command{Name: "bannedcommands", OwnerOnly: true, Hidden: true, Run: handleCmdStopList})
}
