package goldcmds

// ============================================================================
// GOLD-MD — .cmdprefix (per-command prefix control)
// File: cmdprefix.go
// ============================================================================
// Owner kisi bhi command ka prefix "stop" kar sakta hai — us command ko
// prefix ke SATH bhi aur prefix ke BAGHER bhi chalaya ja sakta hai:
//   .cmdprefix                       → info + guidance + current status
//   .cmdprefix stop ping             → .ping aur "ping" dono chalte hain
//   .cmdprefix stop ping,menu,alive  → ek sath kai commands (comma/space)
//   .cmdprefix start ping            → ping wapas sirf prefix ke sath
//   .cmdprefix start alive,menu,ping → ek sath kai commands
//   .cmdprefix reset                 → saare commands wapas prefix-only
//
// ORDER: action (stop/start) PEHLE, phir command name(s).
//
// Storage (Redis settings:<botJID> hash — cmdname/cmdreact jaisa hi Redis-safe
// pattern, GetSetting/SetSetting full-value overwrite, koi orphan key nahi):
//   field "cmdprefixstop" = "ping,menu,alive"  (comma-separated command names)
//
// Runtime hook (handler.go, prefix-check se PEHLE):
//   CmdPrefixRewrite(body, prefix) → agar body bina prefix ka hai aur uska
//   pehla word kisi "stopped" command ka naam hai, to body ko prefix+body me
//   rewrite kar deta hai — phir normal dispatch flow (owner-only / bancmd /
//   botblock / bangc / mode — sab checks) waise hi lagte hain. Isse koi
//   security bypass nahi hota, sirf prefix optional ho jata hai.
//
// Custom names (.cmdname) bhi kaam karte hain: agar owner ne ping ko umar
// rename kiya aur "ping" stop kiya, to "umar" bina prefix bhi chalega
// (typed word ko CmdNameResolve se original par map karke check hota hai).
// ============================================================================

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── storage field (Redis settings:<botJID> hash) ──

const cpStopField = "cmdprefixstop" // "ping,menu,alive"

// ── cache (1-minute TTL, write-through invalidate — cmdname pattern) ──

var (
	cpMu    sync.Mutex
	cpCache = map[string]map[string]bool{}
	cpAt    = map[string]time.Time{}
)

// cpLoad reads the stopped-prefix command set for this bot (cached).
func cpLoad(s SessionBridge) map[string]bool {
	botJID := s.GetJID()
	cpMu.Lock()
	if st, ok := cpCache[botJID]; ok && time.Since(cpAt[botJID]) < time.Minute {
		cpMu.Unlock()
		return st
	}
	cpMu.Unlock()
	raw := s.GetStatusSetting(cpStopField, "")
	st := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		n := cnClean(part)
		if n != "" {
			st[n] = true
		}
	}
	cpMu.Lock()
	cpCache[botJID] = st
	cpAt[botJID] = time.Now()
	cpMu.Unlock()
	return st
}

func cpInvalidate(botJID string) {
	cpMu.Lock()
	delete(cpCache, botJID)
	delete(cpAt, botJID)
	cpMu.Unlock()
}

// cpSave writes the full stopped set back (full-value overwrite — Redis-safe,
// koi orphan/partial field nahi, cmdname/cmdreact jaisa hi).
func cpSave(s SessionBridge, st map[string]bool) {
	names := make([]string, 0, len(st))
	for n := range st {
		names = append(names, n)
	}
	// stable order (alphabetical) so the stored value is deterministic
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	s.SetStatusSetting(cpStopField, strings.Join(names, ","))
	cpInvalidate(s.GetJID())
}

// cpWordRe — leading command token, same charset the dispatcher's cmdRegex
// accepts (`^([A-Za-z0-9_]+)(.*)$`).
var cpWordRe = regexp.MustCompile(`^([A-Za-z0-9_]+)`)

// CmdPrefixRewrite is the handler hook (called BEFORE the prefix check).
// If body has no prefix and its first word is a command whose prefix is
// stopped, it returns prefix+body with ok=true so the normal dispatch flow
// runs it. Otherwise ok=false and body is returned unchanged.
func CmdPrefixRewrite(s SessionBridge, body, prefix string) (string, bool) {
	if prefix == "" {
		return body, false // bot already in prefix-less mode — nothing to do
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || strings.HasPrefix(trimmed, prefix) {
		return body, false
	}
	m := cpWordRe.FindStringSubmatch(trimmed)
	if m == nil {
		return body, false
	}
	word := strings.ToLower(m[1])
	stopped := cpLoad(s)
	if len(stopped) == 0 {
		return body, false
	}
	if stopped[word] {
		return prefix + trimmed, true
	}
	// custom renamed name (.cmdname) → original command
	if orig, ok := CmdNameResolve(s, word); ok && stopped[orig] {
		return prefix + trimmed, true
	}
	return body, false
}

// cpSplitList splits a command list on commas and/or whitespace.
func cpSplitList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", " ")
	var out []string
	for _, f := range strings.Fields(raw) {
		n := cnClean(f)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// cpArrowList renders the stopped commands as a numbered list.
func cpArrowList(names []string, prefix string) string {
	var b strings.Builder
	for i, n := range names {
		b.WriteString(fmt.Sprintf("*%d. ❲ %s%s ❳  →  ❲ %s ❳*\\n", i+1, prefix, n, n))
	}
	return strings.TrimRight(b.String(), "\\n")
}

// cpEnglishHelp — pure-English mode descriptions (har mode ki 3-4 line) taake
// English expert user khud samajh jaye, ya user Meta AI ko bhej kar pooch le
// to Meta AI ko bhi samajhne me asani ho: kya hoga, kaise chalega.
func cpEnglishHelp(prefix string) string {
	return "*🔰 ENGLISH DESCRIPTION 🔰*\\n\\n" +
		"*STOP :❵ Example " + prefix + "cmdprefix stop ping — the ping command now works BOTH ways: with the prefix (" + prefix + "ping) and also WITHOUT the prefix (just type ping). You can stop the prefix for as many commands as you want at once by separating them with commas, like " + prefix + "cmdprefix stop ping,menu,alive.*\\n\\n" +
		"*START :❵ Example " + prefix + "cmdprefix start ping — the ping command goes back to prefix-only mode. From now on it ONLY works with the prefix (" + prefix + "ping) and typing just ping without the prefix does nothing. You can start several commands at once, like " + prefix + "cmdprefix start alive,menu,ping.*\\n\\n" +
		"*RESET :❵ Example " + prefix + "cmdprefix reset — every command you stopped is restored at once and the bot goes back to prefix-only for all commands.*"
}

func handleCmdPrefix(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *CMDPREFIX ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	raw := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))

	// ── No args: info + examples + current status ──
	if raw == "" {
		st := cpLoad(s)
		names := make([]string, 0, len(st))
		for n := range st {
			names = append(names, n)
		}
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				if names[j] < names[i] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 CMDPREFIX INFO 🔰*\\n\\n")
		b.WriteString("*MAKE ANY COMMAND WORK WITHOUT THE PREFIX TOO.*\\n\\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX STOP PING ❳* — " + prefix + "ping AND ping both work\\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX STOP PING,MENU,ALIVE ❳* — many commands at once\\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX START PING ❳* — " + prefix + "ping back to prefix-only\\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX START ALIVE,MENU,PING ❳* — many commands at once\\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX RESET ❳* — all commands back to prefix-only\\n\\n")
		b.WriteString(cpEnglishHelp(prefix) + "\\n\\n")
		b.WriteString("*CURRENT PREFIXLESS COMMANDS :❵ ❰ " + itoa(len(names)) + " ❱*\\n")
		if len(names) > 0 {
			b.WriteString(cpArrowList(names, prefix))
		} else {
			b.WriteString("*NONE — ALL COMMANDS NEED THE PREFIX*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	// ── reset ──
	if raw == "reset" {
		st := cpLoad(s)
		if len(st) == 0 {
			s.Reply(info, "*NO PREFIXLESS COMMANDS TO RESET*\\n"+
				"*ALL COMMANDS ALREADY NEED THE PREFIX*")
			return
		}
		total := len(st)
		s.SetStatusSetting(cpStopField, "")
		cpInvalidate(s.GetJID())
		s.Reply(info, "*🔰 ALL PREFIXLESS COMMANDS RESET 🔰*\\n\\n"+
			"*ALL COMMANDS ARE BACK TO PREFIX-ONLY*\\n\\n"+
			"*TOTAL RESET :❵ ❰ "+itoa(total)+" ❱*")
		return
	}

	// ── parse: <stop|start> <cmds>  (action FIRST) ──
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		s.Reply(info, "*WRONG FORMAT*\\n\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX STOP PING ❳ TO MAKE A COMMAND PREFIXLESS*\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX START PING ❳ TO MAKE IT PREFIX-ONLY AGAIN*\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX ❳ FOR FULL HELP*")
		return
	}
	action := ""
	var cmdPart string
	if fields[0] == "stop" || fields[0] == "start" {
		action = fields[0]
		cmdPart = strings.Join(fields[1:], " ")
	} else {
		s.Reply(info, "*WRONG FORMAT*\\n\\n"+
			"*FIRST WORD MUST BE ❲ STOP ❳ OR ❲ START ❳*\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX STOP PING ❳*\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX ❳ FOR FULL HELP*")
		return
	}

	names := cpSplitList(cmdPart)
	if len(names) == 0 {
		s.Reply(info, "*NO COMMAND NAME GIVEN*\\n\\n"+
			"*TYPE ❲ "+prefix+"CMDPREFIX STOP PING ❳*")
		return
	}

	known := cmdNameKnownSet()
	st := cpLoad(s)
	// copy so we don't mutate the cached map in place
	next := map[string]bool{}
	for k := range st {
		next[k] = true
	}

	var applied, unknown, already []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
			continue
		}
		if action == "stop" {
			if next[n] {
				already = append(already, n)
				continue
			}
			next[n] = true
			applied = append(applied, n)
		} else { // start
			if !next[n] {
				already = append(already, n)
				continue
			}
			delete(next, n)
			applied = append(applied, n)
		}
	}

	if len(applied) > 0 {
		cpSave(s, next)
	}

	var b strings.Builder
	if action == "stop" {
		b.WriteString("*🔰 CMDPREFIX STOPPED 🔰*\\n\\n")
		b.WriteString("*THESE COMMANDS NOW WORK WITH AND WITHOUT THE PREFIX*\\n\\n")
	} else {
		b.WriteString("*🔰 CMDPREFIX STARTED 🔰*\\n\\n")
		b.WriteString("*THESE COMMANDS NOW WORK ONLY WITH THE PREFIX*\\n\\n")
	}
	if len(applied) > 0 {
		b.WriteString("*UPDATED :❵ ❰ " + itoa(len(applied)) + " ❱*\\n")
		b.WriteString(cpArrowList(applied, prefix) + "\\n\\n")
	}
	if len(already) > 0 {
		if action == "stop" {
			b.WriteString("*ALREADY PREFIXLESS :❵ ❰ " + itoa(len(already)) + " ❱*\\n")
		} else {
			b.WriteString("*ALREADY PREFIX-ONLY :❵ ❰ " + itoa(len(already)) + " ❱*\\n")
		}
		b.WriteString(cpArrowList(already, prefix) + "\\n\\n")
	}
	if len(unknown) > 0 {
		b.WriteString("*COMMAND NOT FOUND :❵ ❰ " + itoa(len(unknown)) + " ❱*\\n")
		for _, n := range unknown {
			b.WriteString("*❲ " + prefix + n + " ❳ — NOT A BOT COMMAND*\\n")
		}
		b.WriteString("\\n")
	}
	if len(applied) == 0 && len(already) == 0 && len(unknown) > 0 {
		b.WriteString("*TYPE ❲ " + prefix + "CMDPREFIX ❳ FOR FULL HELP*")
	}
	s.Reply(info, strings.TrimSpace(b.String()))
}

// ── registration ──

func init() {
	Register(Command{
		Name:      "cmdprefix",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO MAKE ANY COMMAND WORK WITHOUT THE PREFIX TOO. STOP OR START THE PREFIX PER COMMAND.",
		OwnerOnly: true,
		Run:       handleCmdPrefix,
	})
}
