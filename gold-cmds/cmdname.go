package goldcmds

// ============================================================================
// GOLD-MD — .cmdname (custom command names)
// ============================================================================
// Owner apne bot ke kisi bhi command ka naam apna naam rakh sakta hai:
//   .cmdname ping to umar      → .ping ka naam ab .umar (menu me b .umar)
//   .cmdname song to music     → rename example
//   .cmdname menu to list      → rename example
//   .cmdname alive to online   → rename example
//   .cmdname video to film     → rename example
//   .cmdname mine              → SIRF owner ke rename kiye commands chalen,
//                                bot apne original commands par chup rahe
//   .cmdname all               → owner ke names + bot ke original — sab chalen
//   .cmdname reset             → saare custom names delete, original only
//
// Storage (Redis settings:<botJID> hash — bancmd/cmdreact jaisa hi Redis-safe
// pattern, GetSetting/SetSetting full-value overwrite, koi orphan key nahi):
//   field "cmdnames"     = "old:new,old2:new2"  (comma pairs)
//   field "cmdnamemode"  = "all" (default) | "mine"
//
// Runtime hooks (handler.go, dispatch se pehle — prefix-fallback se bhi pehle):
//   1. CmdNameMineBlocked(typed) → mine mode me typed name custom nahi hai
//      to bilkul chup (silent return) — owner included, lifelines exempt
//   2. CmdNameResolve(typed)     → custom name (umar) → original (ping)
//   manager.go — CmdNameViewFor() menu me renamed commands apne NEW name se
//                dikhte hain; mine mode me sirf renamed + lifeline commands
//   main.go    — CmdNameAttachKnownCommands() full dispatchable name set
//                (gold-cmds registry + core commands like ping/menu)
//
// Lockout safety: cmdname khud rename NAHI ho sakta aur mine mode me bhi
// hamesha chalta rehta hai (menu bhi — jab tak usko rename nahi kiya).
// ============================================================================

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── storage fields (Redis settings:<botJID> hash) ──

const (
	cnNamesField = "cmdnames"    // "old:new,old2:new2"
	cnModeField  = "cmdnamemode" // "all" (default) | "mine"
)

// cnPair is one rename: original bot command → owner's custom name.
type cnPair struct{ Old, New string }

// cnState is the resolved rename state for one bot (cached).
type cnState struct {
	Pairs []cnPair
	ByOld map[string]string
	ByNew map[string]string
	Mode  string // "all" | "mine"
}

// ── cache (1-minute TTL, write-through invalidate — bancmd pattern) ──

var (
	cnMu    sync.Mutex
	cnCache = map[string]*cnState{}
	cnAt    = map[string]time.Time{}
)

// cnLoad reads the rename state for this bot from Redis (cached).
func cnLoad(s SessionBridge) *cnState {
	botJID := s.GetJID()
	cnMu.Lock()
	if st, ok := cnCache[botJID]; ok && time.Since(cnAt[botJID]) < time.Minute {
		cnMu.Unlock()
		return st
	}
	cnMu.Unlock()
	raw := s.GetStatusSetting(cnNamesField, "")
	mode := strings.ToLower(strings.TrimSpace(s.GetStatusSetting(cnModeField, "all")))
	if mode != "mine" {
		mode = "all"
	}
	st := &cnState{
		ByOld: map[string]string{},
		ByNew: map[string]string{},
		Mode:  mode,
	}
	for _, pair := range strings.Split(raw, ",") {
		halves := strings.SplitN(pair, ":", 2)
		if len(halves) != 2 {
			continue
		}
		o := cnClean(halves[0])
		n := cnClean(halves[1])
		if o == "" || n == "" || o == n {
			continue
		}
		if _, dup := st.ByOld[o]; dup {
			continue // first wins (order-stable)
		}
		if prev, dup := st.ByNew[n]; dup && prev != o {
			continue
		}
		st.ByOld[o] = n
		st.ByNew[n] = o
		st.Pairs = append(st.Pairs, cnPair{Old: o, New: n})
	}
	cnMu.Lock()
	cnCache[botJID] = st
	cnAt[botJID] = time.Now()
	cnMu.Unlock()
	return st
}

func cnInvalidate(botJID string) {
	cnMu.Lock()
	delete(cnCache, botJID)
	delete(cnAt, botJID)
	cnMu.Unlock()
}

// cnSave writes the full pair list back (full-value overwrite — Redis-safe,
// koi orphan/partial field nahi, dusre commands jaisa hi).
func cnSave(s SessionBridge, pairs []cnPair) {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, p.Old+":"+p.New)
	}
	s.SetStatusSetting(cnNamesField, strings.Join(parts, ","))
	cnInvalidate(s.GetJID())
}

// cnClean normalizes one command name: trim, strip a leading dot, lowercase.
func cnClean(n string) string {
	n = strings.TrimSpace(n)
	n = strings.TrimPrefix(n, ".")
	return strings.ToLower(n)
}

// ── known-commands hook (main package attaches its full dispatchable set) ──

var cmdNameKnownHook func() []string

// CmdNameAttachKnownCommands lets the main package supply every command name
// the dispatcher knows (gold-cmds registry + core commands like ping, menu,
// alive, uptime). Called from main.go after all init() registrations.
func CmdNameAttachKnownCommands(fn func() []string) { cmdNameKnownHook = fn }

func cmdNameKnownSet() map[string]bool {
	set := map[string]bool{}
	for _, c := range Commands() {
		set[strings.ToLower(c.Name)] = true
	}
	if cmdNameKnownHook != nil {
		for _, n := range cmdNameKnownHook() {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				set[n] = true
			}
		}
	}
	return set
}

// ── dispatch hooks (called from handler.go) ──

// CmdNameResolve maps a typed command name to the command that should run.
// If the typed name is an owner-set custom name (e.g. .umar for ping), the
// original bot command name is returned with ok=true — the handler dispatches
// the original handler. Otherwise the typed name is returned unchanged.
func CmdNameResolve(s SessionBridge, typed string) (string, bool) {
	typed = strings.ToLower(strings.TrimSpace(typed))
	if typed == "" {
		return typed, false
	}
	st := cnLoad(s)
	if orig, ok := st.ByNew[typed]; ok && orig != "" {
		return orig, true
	}
	return typed, false
}

// CmdNameMineBlocked reports whether the TYPED command name must be silently
// ignored: .cmdname mine → only the owner's renamed (custom) names work and
// the bot stays completely silent on its own original command names (renamed
// or not, for everyone including the owner). Lifelines that always stay
// alive so the owner can never get locked out: cmdname itself (unrenamable)
// and menu (as long as menu was not renamed — its custom name works then).
//
// CRITICAL: this must run on the TYPED name BEFORE CmdNameResolve — the
// custom name of a renamed command (e.g. .list for menu) resolves to the
// original and must NOT be blocked; only the original name (.menu) is silent.
func CmdNameMineBlocked(s SessionBridge, typed string) bool {
	typed = strings.ToLower(strings.TrimSpace(typed))
	if typed == "" {
		return false
	}
	st := cnLoad(s)
	if st.Mode != "mine" {
		return false
	}
	if typed == "cmdname" {
		return false
	}
	if _, custom := st.ByNew[typed]; custom {
		return false // owner's custom name — always works
	}
	if typed == "menu" {
		if _, renamed := st.ByOld[typed]; !renamed {
			return false // menu lifeline (not renamed)
		}
	}
	return true // bot's own original name — silent in mine mode
}

// CmdNameView carries the resolved rename state for menu rendering.
type CmdNameView struct {
	Renames map[string]string // original (lower) → custom name
	Mode    string            // "all" | "mine"
}

// CmdNameViewFor returns the menu view of the renames. ALWAYS returns a
// valid view (empty map + default mode when nothing is set) — an empty map
// lookup is a no-op, so the menu renders exactly as before for bots that
// never used .cmdname. Never returns nil (menu code dereferences directly).
func CmdNameViewFor(s SessionBridge) *CmdNameView {
	st := cnLoad(s)
	return &CmdNameView{Renames: st.ByOld, Mode: st.Mode}
}

// ── .cmdname command ──

// cnNameRe — a command name can only have letters, numbers and _ (max 20),
// same charset the dispatcher's cmdRegex accepts, warna type hi nahi ho sakta.
var cnNameRe = regexp.MustCompile(`^[a-z0-9_]{1,20}$`)

// cnReserved — names that can never be used as a custom command name
// (cmdname's own subcommands — warna mode switching toot jati).
var cnReserved = map[string]bool{
	"cmdname": true,
	"mine":    true,
	"all":     true,
	"reset":   true,
	"to":      true,
}

// cmdNameEnglishHelp — pure-English mode descriptions (har mode ki 4-5
// line) taake English expert user khud samajh jaye, ya user Meta AI ko
// bhej kar pooch le to Meta AI ko bhi samajhne me asani ho: kya hoga,
// kaise chalega.
func cmdNameEnglishHelp(prefix string) string {
	return "*🔰 ENGLISH DESCRIPTION 🔰*\n\n" +
		"*RENAME :❥ Example " + prefix + "cmdname ping to umar — the ping command is now renamed to umar. From now on typing " + prefix + "ping does NOT work anymore, you must type " + prefix + "umar instead and it answers exactly like ping did. The menu also shows the new name " + prefix + "umar instead of " + prefix + "ping. You can rename any command this way.*\n\n" +
		"*MINE MODE :❥ Example " + prefix + "cmdname mine — ONLY the commands you renamed yourself keep working, the bot goes completely silent on ALL of its own original commands. So if you renamed ping to umar, then " + prefix + "umar works but " + prefix + "ping gets no reply. Safety: " + prefix + "cmdname and " + prefix + "menu always stay alive so you can switch back anytime.*\n\n" +
		"*ALL MODE :❥ Example " + prefix + "cmdname all — your renamed names AND the bot's original commands both work together. " + prefix + "umar works AND " + prefix + "ping also works. The menu shows your custom names plus all original bot commands. This is the default mode.*\n\n" +
		"*RESET :❥ Example " + prefix + "cmdname reset — every custom name you created is deleted at once and the bot returns to its original command names only. " + prefix + "ping answers again, " + prefix + "umar stops working, the menu shows the original names again.*"
}

// cnArrowList renders the rename pairs as a numbered list.
func cnArrowList(pairs []cnPair, prefix string) string {
	var b strings.Builder
	for i, p := range pairs {
		b.WriteString(fmt.Sprintf("*%d. ❲ %s%s ❳  →  ❲ %s%s ❳*\n", i+1, prefix, p.Old, prefix, p.New))
	}
	return strings.TrimRight(b.String(), "\n")
}

func handleCmdName(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *CMDNAME ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	raw := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))

	// ── No args: info + examples + current status ──
	if raw == "" {
		st := cnLoad(s)
		mode := "ALL (MY NAMES + BOT'S OWN COMMANDS)"
		if st.Mode == "mine" {
			mode = "MINE (ONLY MY NAMES — BOT SILENT ON ITS OWN)"
		}
		var b strings.Builder
		b.WriteString("*🔰 CMDNAME INFO 🔰*\n\n")
		b.WriteString("*RENAME ANY BOT COMMAND TO YOUR OWN NAME. THE MENU ALSO SHOWS YOUR NEW NAME.*\n\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME PING TO UMAR ❳* — " + prefix + "ping becomes " + prefix + "umar\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME SONG TO MUSIC ❳* — " + prefix + "song becomes " + prefix + "music\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME MENU TO LIST ❳* — " + prefix + "menu becomes " + prefix + "list\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME ALIVE TO ONLINE ❳* — " + prefix + "alive becomes " + prefix + "online\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME VIDEO TO FILM ❳* — " + prefix + "video becomes " + prefix + "film\n\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME MINE ❳* — only my renamed commands work, bot stays silent on its own commands\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME ALL ❳* — my names + bot's own commands, everything works\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME RESET ❳* — delete all my names, bot back to its original commands\n\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDNAME LIST ❳* — show the list of my custom names\n\n")
		b.WriteString(cmdNameEnglishHelp(prefix) + "\n\n")
		b.WriteString("*CURRENT MODE :❥ " + mode + "*\n")
		b.WriteString("*MY CUSTOM NAMES :❥ ❰ " + itoa(len(st.Pairs)) + " ❱*\n")
		if len(st.Pairs) > 0 {
			b.WriteString(cnArrowList(st.Pairs, prefix))
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	// ── Mode subcommands ──
	switch raw {
	case "mine":
		st := cnLoad(s)
		s.SetStatusSetting(cnModeField, "mine")
		cnInvalidate(s.GetJID())
		if len(st.Pairs) == 0 {
			s.Reply(info, "*🔰 YOU HAVE NO CUSTOM NAMES YET*\n\n"+
				"*IN MINE MODE THE BOT WILL BE SILENT ON ALL ITS OWN COMMANDS*\n\n"+
				"*FIRST RENAME A COMMAND — TYPE ❲ "+prefix+"CMDNAME PING TO UMAR ❳*\n\n"+
				"*NOTE :❥ ❲ "+prefix+"CMDNAME ❳ AND ❲ "+prefix+"MENU ❳ ALWAYS STAY WORKING SO YOU CAN SWITCH BACK ANYTIME*")
			return
		}
		s.Reply(info, "*🔰 CMDNAME MODE : MINE 🔰*\n\n"+
			"*NOW ONLY YOUR RENAMED COMMANDS WORK*\n"+
			"*BOT STAYS SILENT ON ITS OWN ORIGINAL COMMANDS*\n\n"+
			"*YOUR COMMANDS :❥ ❰ "+itoa(len(st.Pairs))+" ❱*\n"+
			cnArrowList(st.Pairs, prefix)+"\n\n"+
			"*NOTE :❥ ❲ "+prefix+"CMDNAME ❳ AND ❲ "+prefix+"MENU ❳ ALWAYS STAY WORKING SO YOU CAN SWITCH BACK ANYTIME*\n\n"+
			"*TYPE ❲ "+prefix+"CMDNAME ALL ❳ TO ENABLE ALL COMMANDS AGAIN*")
		return
	case "all":
		st := cnLoad(s)
		s.SetStatusSetting(cnModeField, "all")
		cnInvalidate(s.GetJID())
		s.Reply(info, "*🔰 CMDNAME MODE : ALL 🔰*\n\n"+
			"*YOUR RENAMED NAMES + BOT'S OWN COMMANDS — EVERYTHING WORKS*\n\n"+
			"*YOUR COMMANDS :❥ ❰ "+itoa(len(st.Pairs))+" ❱*\n"+
			cnArrowList(st.Pairs, prefix)+"\n\n"+
			"*THE MENU NOW SHOWS YOUR NAMES + ALL BOT COMMANDS*")
		return
	case "reset":
		st := cnLoad(s)
		if len(st.Pairs) == 0 {
			s.Reply(info, "*NO CUSTOM NAMES TO DELETE*\n"+
				"*BOT IS ALREADY ON ITS ORIGINAL COMMAND NAMES*")
			return
		}
		deleted := cnArrowList(st.Pairs, prefix)
		total := len(st.Pairs)
		// Redis-safe full-value overwrite (empty = clean, koi orphan nahi)
		s.SetStatusSetting(cnNamesField, "")
		s.SetStatusSetting(cnModeField, "all")
		cnInvalidate(s.GetJID())
		s.Reply(info, "*🔰 ALL CUSTOM NAMES DELETED 🔰*\n\n"+
			"*BOT IS BACK TO ITS ORIGINAL COMMAND NAMES*\n\n"+
			"*TOTAL DELETED :❥ ❰ "+itoa(total)+" ❱*\n"+
			deleted)
		return
	case "list":
		st := cnLoad(s)
		var b strings.Builder
		b.WriteString("*🔰 CMDNAME LIST 🔰*\n\n")
		if len(st.Pairs) == 0 {
			b.WriteString("*NO CUSTOM NAMES*\n")
			b.WriteString("*BOT IS USING ITS ORIGINAL COMMAND NAMES*\n")
			b.WriteString("*RENAME WITH :❥ ❲ " + prefix + "CMDNAME PING TO UMAR ❳*")
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}
		b.WriteString("*THESE COMMANDS HAVE A CUSTOM NAME*\n\n")
		b.WriteString("*TOTAL :❥ ❰ " + itoa(len(st.Pairs)) + " ❱*\n")
		b.WriteString(cnArrowList(st.Pairs, prefix))
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	// ── Rename: <old> to <new> ──
	toks := strings.Fields(raw)
	if len(toks) != 3 || toks[1] != "to" {
		s.Reply(info, "*WRONG FORMAT*\n\n"+
			"*TYPE ❲ "+prefix+"CMDNAME PING TO UMAR ❳ TO RENAME A COMMAND*\n"+
			"*TYPE ❲ "+prefix+"CMDNAME ❳ FOR FULL HELP*")
		return
	}
	oldName := cnClean(toks[0])
	newName := cnClean(toks[2])

	if !cnNameRe.MatchString(oldName) || !cnNameRe.MatchString(newName) {
		s.Reply(info, "*🔰 INVALID COMMAND NAME*\n\n"+
			"*NAME CAN ONLY HAVE LETTERS, NUMBERS AND _ (MAX 20)*\n"+
			"*TYPE ❲ "+prefix+"CMDNAME PING TO UMAR ❳*")
		return
	}
	known := cmdNameKnownSet()
	if !known[oldName] {
		s.Reply(info, "*🔰 COMMAND NOT FOUND :❥ "+prefix+oldName+"*\n\n"+
			"*THAT COMMAND DOES NOT EXIST IN MY BOT*\n"+
			"*TYPE ❲ "+prefix+"CMDNAME ❳ FOR HELP*")
		return
	}
	if oldName == "cmdname" {
		s.Reply(info, "*🔰 YOU CANNOT RENAME THIS COMMAND*\n\n"+
			"*"+prefix+"CMDNAME ALWAYS STAYS WORKING SO YOU CAN NEVER GET LOCKED OUT*")
		return
	}
	if cnReserved[newName] {
		s.Reply(info, "*🔰 YOU CANNOT USE THIS NAME :❥ "+prefix+newName+"*\n\n"+
			"*IT IS RESERVED FOR THE CMDNAME COMMAND ITSELF*\n"+
			"*CHOOSE A DIFFERENT NAME*")
		return
	}
	if known[newName] {
		s.Reply(info, "*🔰 NAME ALREADY EXISTS :❥ "+prefix+newName+"*\n\n"+
			"*IT IS ALREADY ONE OF MY BOT COMMANDS*\n"+
			"*CHOOSE A DIFFERENT NAME*")
		return
	}
	st := cnLoad(s)
	if prev, used := st.ByNew[newName]; used && prev != oldName {
		s.Reply(info, "*🔰 NAME ALREADY USED :❥ "+prefix+newName+"*\n\n"+
			"*IT IS ALREADY SET FOR MY "+prefix+prev+" COMMAND*\n"+
			"*CHOOSE A DIFFERENT NAME*")
		return
	}
	if oldName == newName {
		if _, exists := st.ByOld[oldName]; exists {
			// same name = undo this one rename, restore the original
			var kept []cnPair
			for _, p := range st.Pairs {
				if p.Old != oldName {
					kept = append(kept, p)
				}
			}
			cnSave(s, kept)
			s.Reply(info, "*🔰 COMMAND NAME RESTORED 🔰*\n\n"+
				"*❲ "+prefix+oldName+" ❳ IS BACK TO ITS ORIGINAL NAME*\n"+
				"*NOW TYPE ❲ "+prefix+oldName+" ❳ TO USE THIS COMMAND*")
			return
		}
		s.Reply(info, "*🔰 OLD AND NEW NAME ARE SAME*\n"+
			"*TYPE ❲ "+prefix+"CMDNAME PING TO UMAR ❳*")
		return
	}

	// add or update the rename (full-value overwrite — Redis-safe)
	var pairs []cnPair
	replaced := false
	for _, p := range st.Pairs {
		if p.Old == oldName {
			pairs = append(pairs, cnPair{Old: oldName, New: newName})
			replaced = true
		} else {
			pairs = append(pairs, p)
		}
	}
	if !replaced {
		pairs = append(pairs, cnPair{Old: oldName, New: newName})
	}
	cnSave(s, pairs)
	extra := ""
	if replaced {
		extra = "\n\n*PREVIOUS NAME FOR " + prefix + oldName + " WAS REPLACED*"
	}
	s.Reply(info, "*🔰 COMMAND NAME CHANGED 🔰*\n\n"+
		"*❲ "+prefix+oldName+" ❳  →  ❲ "+prefix+newName+" ❳*"+extra+"\n\n"+
		"*NOW TYPE ❲ "+prefix+newName+" ❳ TO USE THIS COMMAND*\n"+
		"*THE MENU ALSO SHOWS THE NEW NAME ❲ "+prefix+newName+" ❳*")
}

// ── registration ──

func init() {
	Register(Command{
		Name:      "cmdname",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE NAME OF ANY BOT COMMAND. YOU CAN GIVE ANY COMMAND A NEW NAME.",
		OwnerOnly: true,
		Run:       handleCmdName,
	})
}
