package goldcmds

// ============================================================================
// GOLD-MD — .cmdreact (command reaction control)
// File: cmdreact.go
// ============================================================================
// Bot jo 🔰 react karta hai har command par (handler.go reactCommand),
// usko control karo — UMAR style me:
//   .cmdreact                    → info + current status + emojis
//   .cmdreact on                 → reaction ON
//   .cmdreact off                → reaction band (koi react nahi)
//   .cmdreact set 👑              → ek emoji sab commands par
//   .cmdreact set 👑🔰😞😘        → multiple emojis random per command
//   .cmdreact ping 😘             → sirf .ping par 😘 (per-command emoji)
//   .cmdreact ping,menu,alive 🔰  → multiple commands per ek emoji
//   .cmdreact ping remove        → us command ka custom emoji hatao
//   .cmdreact reset              → saare custom emojis delete, default 🔰
//
// PER-COMMAND MODE: har command ka apna emoji —
//   .cmdreact ping 😘   → .ping par hamesha 😘
//   .cmdreact menu 👑    → .menu par hamesha 👑
// Per-command emoji general set se UPAR hota hai — command ka custom emoji
// set hai to wahi lagta hai, warna general emoji (single ya random).
// Parsing: [set] <cmnds> <emoji> — pehla token "set" ho to drop (bacche jo
// "cmdreact set ping 😘" likhte hain wo bhi chale). Commands comma/space
// separated. Last token = emoji.
//
// Info text me har mode ki pure-English description bhi hai (📘 ENGLISH
// DESCRIPTION 📘 section) — English expert user khud samajh jaye, ya user
// Meta AI ko bhej kar pooch le to asani ho.
//
// Storage: Redis settings:<botJID> hash (Redis-safe — GetSetting/SetSetting
// full-value overwrite, koi orphan key nahi):
//   field "cmdreact"       = "true"/"false" (ON/OFF)
//   field "cmdreactemojis" = general emoji list ("" = default 🔰)
//   field "cmdreactmap"    = "ping:😘,menu:👑" (per-command map)
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// defaultCmdReactEmoji — bot branding emoji (GOLD-MD me 🔰).
const defaultCmdReactEmoji = "🔰"

// cmdReactMapField — per-command emoji map storage field
// ("ping:😘,menu:👑" — colon pairs, comma entries).
const cmdReactMapField = "cmdreactmap"

// CmdReactIsOn reports whether the command reaction is enabled (default ON).
func CmdReactIsOn(s SessionBridge) bool {
	v := s.GetStatusSetting("cmdreact", "true")
	return v == "true" || v == "1" || v == "on"
}

// CmdReactEmojis returns the current general command-reaction emoji list
// (custom or the default 🔰).
func CmdReactEmojis(s SessionBridge) []string {
	v := s.GetStatusSetting("cmdreactemojis", "")
	if v == "" {
		return []string{defaultCmdReactEmoji}
	}
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{defaultCmdReactEmoji}
	}
	return out
}

// CmdReactEmojiMap returns the per-command emoji map (command → emoji).
// nil = koi per-command emoji set nahi.
func CmdReactEmojiMap(s SessionBridge) map[string]string {
	v := s.GetStatusSetting(cmdReactMapField, "")
	if v == "" {
		return nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(v, ",") {
		halves := strings.SplitN(pair, ":", 2)
		if len(halves) == 2 {
			cmd := strings.ToLower(strings.TrimSpace(halves[0]))
			emo := strings.TrimSpace(halves[1])
			if cmd != "" && emo != "" {
				out[cmd] = emo
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cmdReactSaveMap writes the full per-command map back (full-value
// overwrite — Redis-safe, koi orphan/partial field nahi).
func cmdReactSaveMap(s SessionBridge, m map[string]string) {
	if len(m) == 0 {
		s.SetStatusSetting(cmdReactMapField, "")
		return
	}
	var parts []string
	for c, e := range m {
		parts = append(parts, c+":"+e)
	}
	s.SetStatusSetting(cmdReactMapField, strings.Join(parts, ","))
}

// CmdReactEmoji returns the emoji to react with for the given command name,
// or "" when OFF. Priority: per-command emoji > general single > random
// from general list. The handler passes the RESOLVED command name (custom
// .cmdname renames bhi sahi emoji par react karenge).
func CmdReactEmoji(s SessionBridge, command string) string {
	if !CmdReactIsOn(s) {
		return ""
	}
	command = strings.ToLower(strings.TrimSpace(command))
	// 1) per-command custom emoji
	if m := CmdReactEmojiMap(s); m != nil {
		if e, ok := m[command]; ok && e != "" {
			return e
		}
	}
	// 2) general emojis: single → same, multiple → random
	emojis := CmdReactEmojis(s)
	if len(emojis) == 0 {
		return defaultCmdReactEmoji
	}
	if len(emojis) == 1 {
		return emojis[0]
	}
	return pickRandomEmoji(emojis)
}

// ── .cmdreact command ──

func handleCmdReact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleCmdReactAsync(s, info, args, prefix)
}

func handleCmdReactAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	isOn := CmdReactIsOn(s)
	emojis := CmdReactEmojis(s)
	perCmd := CmdReactEmojiMap(s)

	// ── No args: info ──
	if len(args) == 0 {
		status := "🔰 OFF"
		if isOn {
			status = "🔰 ON"
		}
		mode := "SINGLE (SAME EMOJI ON EVERY COMMAND)"
		if len(emojis) > 1 {
			mode = "RANDOM (HAR COMMAND PAR ALAG-ALAG)"
		}
		var b strings.Builder
		b.WriteString("*🔰 CMD REACT INFO 🔰*\n\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT ON ❳* — turn ON command reaction\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT OFF ❳* — turn OFF command reaction\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT SET 🔰 ❳* — change the emoji (single)\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT SET 🔰🔰🔰🔰 ❳* — multiple emojis (random on every command)\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT PING 🔰 ❳* — custom emoji for .ping only\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT PING,MENU,ALIVE 🔰 ❳* — one emoji for many commands\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT PING REMOVE ❳* — remove a command's custom emoji\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT RESET ❳* — delete all custom emojis, back to default " + defaultCmdReactEmoji + "\n\n")
		b.WriteString("*TYPE ❲ " + prefix + "CMDREACT LIST ❳* — show the per-command emoji list\n\n")
		b.WriteString(cmdReactEnglishHelp(prefix) + "\n\n")
		b.WriteString("*CURRENT STATUS :❥ " + status + "*\n")
		b.WriteString("*CURRENT EMOJIS :❥ " + strings.Join(emojis, " ") + "*\n")
		b.WriteString("*TOTAL EMOJIS :❥ ❲ " + itoa(len(emojis)) + " ❱*\n")
		b.WriteString("*MODE :❥ " + mode + "*\n")
		if len(perCmd) > 0 {
			b.WriteString("*PER-COMMAND EMOJIS :❥ ❲ " + itoa(len(perCmd)) + " ❱*\n")
			for i, c := range cmdReactSortedKeys(perCmd) {
				b.WriteString("*" + itoa(i+1) + ". ❲ " + prefix + c + " ❳ → " + perCmd[c] + "*\n")
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	// ── ON ──
	if sub == "on" {
		s.SetStatusSetting("cmdreact", "true")
		s.Reply(info, "*🔰 CMD REACT ACTIVATED 🔰*\n\n*BOT WILL NOW REACT ON EVERY COMMAND*\n*EMOJIS :❥ "+strings.Join(emojis, " ")+"*")
		return
	}

	// ── OFF ──
	if sub == "off" {
		s.SetStatusSetting("cmdreact", "false")
		s.Reply(info, "*🔰 CMD REACT DE-ACTIVATED 🔰*\n\n*BOT WILL NOT REACT ON COMMANDS ANYMORE*")
		return
	}

	// ── RESET ──
	// Saare custom emojis delete (general + per-command dono), wapas default
	// 🔰 par. ON/OFF status untouched. Redis-safe empty-value overwrite.
	if sub == "reset" {
		s.SetStatusSetting("cmdreactemojis", "")
		s.SetStatusSetting(cmdReactMapField, "")
		status := "🔰 OFF"
		if CmdReactIsOn(s) {
			status = "🔰 ON"
		}
		s.Reply(info, "*🔰 CMD REACT EMOJIS RESET 🔰*\n\n"+
			"*ALL CUSTOM EMOJIS DELETED*\n"+
			"*BACK TO DEFAULT EMOJI* "+defaultCmdReactEmoji+"\n\n"+
			"*CURRENT EMOJIS :❥ "+strings.Join(CmdReactEmojis(s), " ")+"*\n"+
			"*TOTAL EMOJIS :❥ ❲ 1 ❱*\n"+
			"*MODE :❥ SINGLE (SAME EMOJI ON EVERY COMMAND)*\n"+
			"*REACTION :❥ "+status+"*")
		return
	}

	// ── LIST ──
	// Per-command custom emoji list (numbered). Sirf guidance me zikr hai —
	// menu / command count me koi asar nahi (ye subcommand hai, alag command nahi).
	if sub == "list" {
		var b strings.Builder
		b.WriteString("*🔰 CMDREACT LIST 🔰*\n\n")
		if len(perCmd) == 0 {
			b.WriteString("*NO PER-COMMAND EMOJIS*\n")
			b.WriteString("*ALL COMMANDS USE THE GENERAL EMOJIS :❥ " + strings.Join(emojis, " ") + "*")
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}
		b.WriteString("*THESE COMMANDS HAVE THEIR OWN CUSTOM EMOJI*\n\n")
		b.WriteString("*TOTAL :❥ ❰ " + itoa(len(perCmd)) + " ❱*\n")
		for i, c := range cmdReactSortedKeys(perCmd) {
			b.WriteString("*" + itoa(i+1) + ". ❲ " + prefix + c + " ❳  →  ❲ " + perCmd[c] + " ❳*\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	// ── SET <emojis> (general list) ──
	if sub == "set" {
		raw := ""
		if len(args) > 1 {
			raw = strings.Join(args[1:], " ")
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			s.Reply(info,
				"*🔰 CMD REACT SET 🔰*\n\n"+
					"*TYPE ❲ "+prefix+"CMDREACT SET 🔰 ❳*\n*TO CHANGE THE EMOJI (SINGLE)*\n\n"+
					"*TYPE ❲ "+prefix+"CMDREACT SET 🔰🔰🔰🔰 ❳*\n*MULTIPLE EMOJIS — RANDOM EMOJI ON EVERY COMMAND*")
			return
		}
		newEmojis := cmdReactSplitEmojis(raw)
		if len(newEmojis) == 0 {
			s.Reply(info, "*WRONG COMMAND*\n*TYPE ❲ "+prefix+"CMDREACT ❳ FOR HELP*")
			return
		}
		// Max 20 emojis (autoreact jaisa hi limit)
		if len(newEmojis) > 20 {
			newEmojis = newEmojis[:20]
		}
		s.SetStatusSetting("cmdreactemojis", strings.Join(newEmojis, ","))

		mode := "SINGLE (SAME EMOJI ON EVERY COMMAND)"
		if len(newEmojis) > 1 {
			mode = "RANDOM (HAR COMMAND PAR ALAG-ALAG)"
		}
		s.Reply(info,
			"*🔰 CMD REACT EMOJIS UPDATED 🔰*\n\n"+
				"*NEW EMOJIS*\n "+strings.Join(newEmojis, " ")+" \n \n"+
				"*TOTAL EMOJIS :❥ ❲ "+itoa(len(newEmojis))+" ❱*\n"+
				"*MODE :❥ "+mode+"*")
		return
	}

	// ── REMOVE <cmnds> (per-command emoji hatao) ──
	if sub == "remove" {
		names := cmdReactSplitNames(strings.Join(args[1:], " "))
		if len(names) == 0 {
			s.Reply(info, "*WRONG COMMAND*\n\n"+
				"*TYPE ❲ "+prefix+"CMDREACT PING REMOVE ❳*\n*TO REMOVE A COMMAND'S CUSTOM EMOJI*")
			return
		}
		if perCmd == nil {
			perCmd = map[string]string{}
		}
		var removed, missing []string
		for _, n := range names {
			if _, ok := perCmd[n]; ok {
				delete(perCmd, n)
				removed = append(removed, n)
			} else {
				missing = append(missing, n)
			}
		}
		cmdReactSaveMap(s, perCmd)
		var b strings.Builder
		if len(removed) > 0 {
			b.WriteString("*🔰 PER-COMMAND EMOJI REMOVED 🔰*\n*" + strings.Join(cmdReactDots(removed), ", ") + "*\n\n")
		}
		if len(missing) > 0 {
			b.WriteString("*🔰 NO CUSTOM EMOJI SET FOR*\n*" + strings.Join(cmdReactDots(missing), ", ") + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}

	// ── PER-COMMAND: [set] <cmnds> <emoji> / <cmnds> remove ──
	rest := args
	if sub == "set" {
		rest = args[1:]
	}
	raw := strings.TrimSpace(strings.Join(rest, " "))
	if raw == "" {
		s.Reply(info, "*WRONG COMMAND*\n*TYPE ❲ "+prefix+"CMDREACT ❳ FOR HELP*")
		return
	}
	toks := strings.Fields(raw)
	// "<cmnds> remove" form
	if len(toks) >= 2 && strings.EqualFold(toks[len(toks)-1], "remove") {
		names := cmdReactSplitNames(strings.Join(toks[:len(toks)-1], " "))
		if len(names) == 0 {
			s.Reply(info, "*WRONG COMMAND*\n\n"+
				"*TYPE ❲ "+prefix+"CMDREACT PING REMOVE ❳*\n*TO REMOVE A COMMAND'S CUSTOM EMOJI*")
			return
		}
		if perCmd == nil {
			perCmd = map[string]string{}
		}
		var removed, missing []string
		for _, n := range names {
			if _, ok := perCmd[n]; ok {
				delete(perCmd, n)
				removed = append(removed, n)
			} else {
				missing = append(missing, n)
			}
		}
		cmdReactSaveMap(s, perCmd)
		var b strings.Builder
		if len(removed) > 0 {
			b.WriteString("*🔰 PER-COMMAND EMOJI REMOVED 🔰*\n*" + strings.Join(cmdReactDots(removed), ", ") + "*\n\n")
		}
		if len(missing) > 0 {
			b.WriteString("*🔰 NO CUSTOM EMOJI SET FOR*\n*" + strings.Join(cmdReactDots(missing), ", ") + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
		return
	}
	// "<cmnds> <emoji>" form — last token = emoji
	if len(toks) < 2 {
		s.Reply(info, "*WRONG FORMAT*\n\n"+
			"*TYPE ❲ "+prefix+"CMDREACT PING 🔰 ❳* — custom emoji for one command\n"+
			"*TYPE ❲ "+prefix+"CMDREACT PING,MENU,ALIVE 🔰 ❳* — one emoji for many commands\n"+
			"*TYPE ❲ "+prefix+"CMDREACT ❳ FOR FULL HELP*")
		return
	}
	emoji := toks[len(toks)-1]
	names := cmdReactSplitNames(strings.Join(toks[:len(toks)-1], " "))
	if len(names) == 0 || emoji == "" {
		s.Reply(info, "*WRONG FORMAT*\n\n"+
			"*TYPE ❲ "+prefix+"CMDREACT PING 🔰 ❳* — custom emoji for one command\n"+
			"*TYPE ❲ "+prefix+"CMDREACT PING,MENU,ALIVE 🔰 ❳* — one emoji for many commands\n"+
			"*TYPE ❲ "+prefix+"CMDREACT ❳ FOR FULL HELP*")
		return
	}
	if perCmd == nil {
		perCmd = map[string]string{}
	}
	for _, n := range names {
		perCmd[n] = emoji
	}
	cmdReactSaveMap(s, perCmd)
	s.Reply(info,
		"*🔰 PER-COMMAND EMOJI SET 🔰*\n\n"+
			"*COMMANDS*\n "+strings.Join(cmdReactDots(names), ", ")+" \n \n"+
			"*EMOJI* "+emoji+"\n\n"+
			"*NOW THESE COMMANDS ALWAYS GET THIS EMOJI*\n"+
			"*OTHER COMMANDS KEEP THE GENERAL EMOJIS :❥ "+strings.Join(emojis, " ")+"*")
}

// cmdReactSplitEmojis splits raw text into a clean emoji list.
func cmdReactSplitEmojis(raw string) []string {
	var out []string
	for _, e := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// cmdReactSplitNames splits raw text into clean lowercase command names
// (comma/space separated, leading dot stripped).
func cmdReactSplitNames(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		n := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(f, ".")))
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// cmdReactDots renders names as ".name".
func cmdReactDots(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "."+n)
	}
	return out
}

// cmdReactSortedKeys returns the map keys sorted (stable info list).
func cmdReactSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// cmdReactEnglishHelp — pure-English mode descriptions (har mode ki 4-5
// line) taake English expert user khud samajh jaye, ya user Meta AI ko
// bhej kar pooch le to Meta AI ko bhi samajhne me asani ho: kya hoga,
// kaise chalega.
func cmdReactEnglishHelp(prefix string) string {
	return "*🔰 ENGLISH DESCRIPTION 🔰*\n\n" +
		"*ON :❥ When reaction is ON, the bot reacts with an emoji on every command message. The reaction appears instantly on any command sent by any user, before the command reply comes. This is the default behaviour of the bot.*\n\n" +
		"*OFF :❥ When reaction is OFF, the bot stops reacting on commands completely. Commands still run normally and replies still come, but no emoji reaction appears on any command.*\n\n" +
		"*SET (SINGLE) :❥ Example " + prefix + "cmdreact set 🔰 — every command now gets this same emoji reaction every time. Whatever single emoji you set will appear on all commands.*\n\n" +
		"*SET (MULTIPLE) :❥ Example " + prefix + "cmdreact set 🔰🔰🔰🔰 — the bot picks a RANDOM emoji from your list on every command, so every command gets a different reaction each time.*\n\n" +
		"*PER-COMMAND :❥ Example " + prefix + "cmdreact ping 🔰 — ONLY the ping command always gets 🔰, all other commands keep the general emojis. Multiple commands at once: " + prefix + "cmdreact ping,menu,alive,uptime 🔰 — all of them get 🔰. Per-command emoji beats the general setting. Remove one with " + prefix + "cmdreact ping remove.*\n\n" +
		"*REMOVE :❥ Deletes a command's own custom emoji so it falls back to the general emojis again. Example " + prefix + "cmdreact ping remove.*\n\n" +
		"*RESET :❥ Deletes ALL custom emojis at once — the general list AND every per-command emoji. The bot goes back to its default emoji " + defaultCmdReactEmoji + ". Use this whenever you want the original reaction look back.*"
}

// ── registration ──

func init() {
	Register(Command{
		Name:      "cmdreact",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO SET THE REACTION THE BOT GIVES ON EVERY COMMAND. YOU CAN SET ONE EMOJI OR MANY.",
		OwnerOnly: true,
		Run:       handleCmdReact,
	})
}
