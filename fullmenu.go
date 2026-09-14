package main

// ============================================================================
// GOLD-MD — .fullmenu command (FULL command menu)
//
// OWNER ORDER: .menu sirf visible commands dikhata hai — .fullmenu SAB
// dikhata hai: har command, uski description, uske hidden aliases aur
// anti-family ke sub-commands. Format (owner spec):
//
//   *🔰 COMMAND ❮name❯*
//   *{DESCRIPTION}*
//   *🔰 ALIASES HIDDEN WORK 🔰*      ← sirf jab aliases hon
//   *{alias1 | alias2 | alias3}*      ← sirf jab aliases hon
//
// EXCLUDED (owner order): .svrchange + .host5gb — ye fullmenu me KABHI
// nahi dikhte.
//
// Aliases ka rule: gold-cmds registry me har alias ek alag Hidden
// registration hai jo SAME handler Run ko point karti hai. Isliye yahan
// registrations ko Run code-pointer se group kiya jata hai — ek group =
// ek command (primary) + uske aliases. Duplicate-name cases (mp3 / ban /
// ar / goodbye) me "last registration wins" (Commands map overwrite) ke
// hisaab se hi alias us group me jata hai jahan wo ACTUALLY dispatch hota
// hai.
// ============================================================================

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"go.mau.fi/whatsmeow/types"

	goldcmds "gold-md/gold-cmds"
)

// fmEntry: ek fullmenu block (command ya sub-command).
type fmEntry struct {
	Name    string
	Desc    string
	Aliases []string
	Cat     string
}

// fmCoreCommands: main-package core commands jo registry se nahi aate.
// (alive / ping / uptime / menu / fullmenu / server family)
// host5gb + svrchange JAAN-BOOJH ke missing — owner order.
var fmCoreCommands = []fmEntry{
	{Name: "alive", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO CHECK IF THE BOT IS ALIVE AND RUNNING. IT SHOWS A READY REPLY WITH UPTIME."},
	{Name: "ping", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO CHECK THE BOT SPEED. IT SHOWS THE RESPONSE TIME IN MILLISECONDS."},
	{Name: "uptime", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO SHOW HOW LONG THE BOT HAS BEEN RUNNING."},
	{Name: "menu", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO SHOW THE MAIN COMMAND MENU OF THE BOT.",
		Aliases: []string{"m"}},
	{Name: "fullmenu", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO SHOW THE FULL COMMAND MENU WITH ALL COMMANDS, DESCRIPTIONS AND HIDDEN ALIASES."},
	{Name: "server", Cat: "OWNER & SYSTEM",
		Desc: "THIS COMMAND IS USED TO SHOW ALL GOLD-MD SERVERS PAIRING STATUS. IT SHOWS ONLINE AND OFFLINE SERVERS.",
		Aliases: []string{"servers", "svr", "svrinfo", "serverinfo", "session", "sessions"}},
}

// fmSubcommands: anti-family + scheduler ke sub-commands (owner order:
// ".antilink menu me aa rha uske antilink action ki description fullmenu
// me aaye"). Parent command name → uske sub-commands.
var fmSubcommands = map[string][]fmEntry{
	"antilink": {
		{Name: "antilink on", Desc: "THIS SUB COMMAND IS USED TO TURN ON LINK DETECTION IN THE GROUP."},
		{Name: "antilink off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF LINK DETECTION IN THE GROUP."},
		{Name: "antilink action", Desc: "THIS SUB COMMAND IS USED TO SHOW THE CURRENT ANTILINK ACTION MODE."},
		{Name: "antilink action warn", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO WARN. MEMBERS WHO SEND LINKS GET A WARNING."},
		{Name: "antilink action warn <n>", Desc: "THIS SUB COMMAND IS USED TO SET THE MAX WARNING LIMIT FROM 1 TO 50 BEFORE THE ACTION."},
		{Name: "antilink action warn reset", Desc: "THIS SUB COMMAND IS USED TO RESET THE MAX WARNING LIMIT BACK TO DEFAULT."},
		{Name: "antilink action delete", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO DELETE. LINK MESSAGES GET DELETED."},
		{Name: "antilink action kick", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO KICK. MEMBERS WHO SEND LINKS GET REMOVED."},
		{Name: "antilink allow <domain>", Desc: "THIS SUB COMMAND IS USED TO ADD A WEBSITE TO THE ALLOWED LIST. ITS LINKS WILL NOT BE BLOCKED."},
		{Name: "antilink delete <domain>", Desc: "THIS SUB COMMAND IS USED TO REMOVE A WEBSITE FROM THE ALLOWED LIST."},
		{Name: "antilink allowedlist", Desc: "THIS SUB COMMAND IS USED TO SHOW ALL ALLOWED WEBSITES OF THE GROUP."},
		{Name: "antilink reset", Desc: "THIS SUB COMMAND IS USED TO FULL RESET ALL ANTILINK SETTINGS OF THE GROUP."},
	},
	"antibad": {
		{Name: "antibad on", Desc: "THIS SUB COMMAND IS USED TO TURN ON BAD WORD DETECTION IN THE GROUP."},
		{Name: "antibad off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF BAD WORD DETECTION IN THE GROUP."},
		{Name: "antibad action", Desc: "THIS SUB COMMAND IS USED TO SHOW THE CURRENT ANTIBAD ACTION MODE."},
		{Name: "antibad action warn", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO WARN. MEMBERS WHO SEND BAD WORDS GET A WARNING."},
		{Name: "antibad action warn <n>", Desc: "THIS SUB COMMAND IS USED TO SET THE MAX WARNING LIMIT FROM 1 TO 50 BEFORE THE ACTION."},
		{Name: "antibad action warn reset", Desc: "THIS SUB COMMAND IS USED TO RESET THE MAX WARNING LIMIT BACK TO DEFAULT."},
		{Name: "antibad action delete", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO DELETE. BAD WORD MESSAGES GET DELETED."},
		{Name: "antibad action kick", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO KICK. MEMBERS WHO SEND BAD WORDS GET REMOVED."},
		{Name: "antibad action reset", Desc: "THIS SUB COMMAND IS USED TO RESET THE ANTIBAD ACTION BACK TO DEFAULT."},
		{Name: "antibad add <words>", Desc: "THIS SUB COMMAND IS USED TO ADD CUSTOM BAD WORDS TO THE LIST."},
		{Name: "antibad del <words>", Desc: "THIS SUB COMMAND IS USED TO REMOVE CUSTOM BAD WORDS FROM THE LIST."},
		{Name: "antibad list", Desc: "THIS SUB COMMAND IS USED TO SHOW ALL CUSTOM BAD WORDS OF THE GROUP."},
		{Name: "antibad reset", Desc: "THIS SUB COMMAND IS USED TO FULL RESET ALL ANTIBAD SETTINGS OF THE GROUP."},
	},
	"antibot": {
		{Name: "antibot on", Desc: "THIS SUB COMMAND IS USED TO TURN ON BOT MESSAGE DETECTION IN THE GROUP."},
		{Name: "antibot off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF BOT MESSAGE DETECTION IN THE GROUP."},
		{Name: "antibot action", Desc: "THIS SUB COMMAND IS USED TO SHOW THE CURRENT ANTIBOT ACTION MODE."},
		{Name: "antibot action warn", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO WARN. MEMBERS WHO FORWARD BOT MESSAGES GET A WARNING."},
		{Name: "antibot action warn <n>", Desc: "THIS SUB COMMAND IS USED TO SET THE MAX WARNING LIMIT FROM 1 TO 50 BEFORE THE ACTION."},
		{Name: "antibot action warn reset", Desc: "THIS SUB COMMAND IS USED TO RESET THE MAX WARNING LIMIT BACK TO DEFAULT."},
		{Name: "antibot action delete", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO DELETE. BOT FORWARD MESSAGES GET DELETED."},
		{Name: "antibot action kick", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO KICK. MEMBERS WHO FORWARD BOT MESSAGES GET REMOVED."},
		{Name: "antibot allow <jid>", Desc: "THIS SUB COMMAND IS USED TO ADD A BOT TO THE ALLOWED LIST. ITS MESSAGES WILL NOT BE BLOCKED."},
		{Name: "antibot delete <jid>", Desc: "THIS SUB COMMAND IS USED TO REMOVE A BOT FROM THE ALLOWED LIST."},
		{Name: "antibot allowedlist", Desc: "THIS SUB COMMAND IS USED TO SHOW ALL ALLOWED BOTS OF THE GROUP."},
		{Name: "antibot reset", Desc: "THIS SUB COMMAND IS USED TO FULL RESET ALL ANTIBOT SETTINGS OF THE GROUP."},
	},
	"antistatus": {
		{Name: "antistatus on", Desc: "THIS SUB COMMAND IS USED TO TURN ON STATUS MENTION DETECTION."},
		{Name: "antistatus off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF STATUS MENTION DETECTION."},
		{Name: "antistatus action", Desc: "THIS SUB COMMAND IS USED TO SHOW THE CURRENT ANTISTATUS ACTION MODE."},
		{Name: "antistatus action warn", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO WARN. MEMBERS WHO MENTION THE BOT IN STATUS GET A WARNING."},
		{Name: "antistatus action warn <n>", Desc: "THIS SUB COMMAND IS USED TO SET THE MAX WARNING LIMIT FROM 1 TO 50 BEFORE THE ACTION."},
		{Name: "antistatus action warn reset", Desc: "THIS SUB COMMAND IS USED TO RESET THE MAX WARNING LIMIT BACK TO DEFAULT."},
		{Name: "antistatus action delete", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO DELETE. THE STATUS MENTION GETS DELETED."},
		{Name: "antistatus action kick", Desc: "THIS SUB COMMAND IS USED TO SET THE ACTION TO KICK. MEMBERS WHO MENTION THE BOT IN STATUS GET REMOVED."},
		{Name: "antistatus reset", Desc: "THIS SUB COMMAND IS USED TO FULL RESET ALL ANTISTATUS SETTINGS."},
	},
	"antidelete": {
		{Name: "antidelete on", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIDELETE FOR INBOX AND GROUPS BOTH."},
		{Name: "antidelete inbox", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIDELETE FOR PRIVATE CHATS ONLY."},
		{Name: "antidelete groups", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIDELETE FOR GROUPS ONLY."},
		{Name: "antidelete off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF ANTIDELETE."},
		{Name: "antidelete msg here", Desc: "THIS SUB COMMAND IS USED TO SEND DELETED MESSAGES BACK IN THE SAME CHAT."},
		{Name: "antidelete msg inbox", Desc: "THIS SUB COMMAND IS USED TO SEND DELETED MESSAGES TO THE BOT PRIVATE INBOX."},
	},
	"antiedit": {
		{Name: "antiedit on", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIEDIT FOR INBOX AND GROUPS BOTH."},
		{Name: "antiedit inbox", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIEDIT FOR PRIVATE CHATS ONLY."},
		{Name: "antiedit groups", Desc: "THIS SUB COMMAND IS USED TO TURN ON ANTIEDIT FOR GROUPS ONLY."},
		{Name: "antiedit off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF ANTIEDIT."},
		{Name: "antiedit msg here", Desc: "THIS SUB COMMAND IS USED TO SEND EDIT ALERTS IN THE SAME CHAT."},
		{Name: "antiedit msg inbox", Desc: "THIS SUB COMMAND IS USED TO SEND EDIT ALERTS TO THE BOT PRIVATE INBOX."},
	},
	"anticall": {
		{Name: "anticall on", Desc: "THIS SUB COMMAND IS USED TO TURN ON AUTO CALL REJECT."},
		{Name: "anticall off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF AUTO CALL REJECT."},
		{Name: "anticall msg", Desc: "THIS SUB COMMAND IS USED TO SHOW THE CURRENT CUSTOM REJECT CALL MESSAGE."},
		{Name: "anticall msg <text>", Desc: "THIS SUB COMMAND IS USED TO SET A CUSTOM MESSAGE FOR REJECTED CALLS."},
	},
	"amute": {
		{Name: "amute on", Desc: "THIS SUB COMMAND IS USED TO TURN ON THE AUTO MUTE SCHEDULER OF THE GROUP."},
		{Name: "amute off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF THE AUTO MUTE SCHEDULER OF THE GROUP."},
		{Name: "amute <time>", Desc: "THIS SUB COMMAND IS USED TO SET THE DAILY AUTO MUTE TIME. USE IT LIKE 7 30 PM."},
	},
	"aunmute": {
		{Name: "aunmute on", Desc: "THIS SUB COMMAND IS USED TO TURN ON THE AUTO UNMUTE SCHEDULER OF THE GROUP."},
		{Name: "aunmute off", Desc: "THIS SUB COMMAND IS USED TO TURN OFF THE AUTO UNMUTE SCHEDULER OF THE GROUP."},
		{Name: "aunmute <time>", Desc: "THIS SUB COMMAND IS USED TO SET THE DAILY AUTO UNMUTE TIME. USE IT LIKE 7 30 AM."},
	},
}

// buildFullMenuEntries gathers EVERY command (registry + core) into one flat
// entry list, grouped per handler. Returns entries in category order.
func buildFullMenuEntries() []fmEntry {
	// ── 1) Registry scan: group registrations by Run code-pointer ──
	type regInfo struct {
		ptr    uintptr
		name   string
		hidden bool
		desc   string
		cat    string
	}
	var regs []regInfo
	for _, c := range goldcmds.Commands() {
		regs = append(regs, regInfo{
			ptr:    reflect.ValueOf(c.Run).Pointer(),
			name:   strings.ToLower(c.Name),
			hidden: c.Hidden,
			desc:   c.Desc,
			cat:    c.Category,
		})
	}

	// Effective dispatch: Commands map me last registration jeet-ta hai —
	// wahi rule yahan (duplicate names: mp3 / ban / ar / goodbye).
	eff := make(map[string]uintptr, len(regs))
	for _, r := range regs {
		eff[r.name] = r.ptr
	}

	// Group by pointer, registry order preserve.
	type grpInfo struct {
		members []regInfo
	}
	byPtr := make(map[uintptr]*grpInfo)
	var ptrOrder []uintptr
	for _, r := range regs {
		g := byPtr[r.ptr]
		if g == nil {
			g = &grpInfo{}
			byPtr[r.ptr] = g
			ptrOrder = append(ptrOrder, r.ptr)
		}
		g.members = append(g.members, r)
	}

	var entries []fmEntry
	seen := make(map[string]bool)
	for _, ptr := range ptrOrder {
		g := byPtr[ptr]

		// Primary: pehla NON-hidden member (visible command ka naam hi
		// primary hai). Sab hidden hain to pehla member (fully-hidden
		// command — fullmenu me yeh bhi dikhta hai).
		primary := -1
		for i, m := range g.members {
			if !m.hidden {
				primary = i
				break
			}
		}
		if primary < 0 {
			primary = 0
		}
		p := g.members[primary]
		if seen[p.name] {
			continue // safety: same name dobara na aaye
		}
		seen[p.name] = true

		cat := p.cat
		if cat == "" {
			cat = "OTHER"
		}
		e := fmEntry{Name: p.name, Desc: p.desc, Cat: cat}

		// Multi-visible group (jaise logo1+logo2 ek hi closure share
		// karte hain): har visible apni alag entry hai — aliases sirf
		// tab banti hain jab group me sirf EK visible ho.
		visCount := 0
		for _, m := range g.members {
			if !m.hidden {
				visCount++
			}
		}
		if visCount <= 1 {
			for i, m := range g.members {
				if i == primary || m.name == p.name {
					continue
				}
				// Alias sirf tab jab wo naam ASLI me is handler par
				// dispatch hota ho (duplicate-name overwrite rule).
				if eff[m.name] == ptr {
					e.Aliases = append(e.Aliases, m.name)
				}
			}
		} else {
			// Har visible member apni entry (logo1 / logo2 case).
			for i, m := range g.members {
				if i == primary || m.hidden {
					continue
				}
				if seen[m.name] {
					continue
				}
				seen[m.name] = true
				mc := m.cat
				if mc == "" {
					mc = "OTHER"
				}
				entries = append(entries, fmEntry{Name: m.name, Desc: m.desc, Cat: mc})
			}
		}
		entries = append(entries, e)
	}

	// ── 2) Core (main-package) commands ──
	entries = append(entries, fmCoreCommands...)

	// ── 3) Category order (same as .menu) + alphabetical inside ──
	catRank := make(map[string]int)
	for i, c := range goldcmds.CategoryOrder {
		catRank[c] = i
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ri, okI := catRank[entries[i].Cat]
		rj, okJ := catRank[entries[j].Cat]
		if !okI {
			ri = 99
		}
		if !okJ {
			rj = 99
		}
		if ri != rj {
			return ri < rj
		}
		if entries[i].Cat != entries[j].Cat {
			return entries[i].Cat < entries[j].Cat
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// renderFullMenuBlock renders one entry in the owner-specified format:
//
//	*🔰 COMMAND ❮name❯*
//	*{DESCRIPTION}*
//	*🔰 ALIASES HIDDEN WORK 🔰*   ← only when aliases exist
//	*{alias1 | alias2}*            ← only when aliases exist
func renderFullMenuBlock(b *strings.Builder, e fmEntry) {
	fmt.Fprintf(b, "*🔰 COMMAND ❮%s❯*\n", e.Name)
	if e.Desc != "" {
		fmt.Fprintf(b, "*%s*\n", e.Desc)
	}
	if len(e.Aliases) > 0 {
		b.WriteString("*🔰 ALIASES HIDDEN WORK 🔰*\n")
		fmt.Fprintf(b, "*%s*\n", strings.Join(e.Aliases, " | "))
	}
	b.WriteString("\n")
}

// buildFullMenuText builds the whole .fullmenu message (list of message
// parts — agar text bohat lamba ho to multiple parts me split hota hai).
func buildFullMenuText(prefix, botName, botNum, ownerNum, uptimeHM string, sessCount int) []string {
	entries := buildFullMenuEntries()

	// Sub-command entries unke parent command ke FORAN baad lagte hain.
	type block struct {
		cat  string
		text string
	}
	var blocks []block
	total := 0
	for _, e := range entries {
		var sb strings.Builder
		renderFullMenuBlock(&sb, e)
		blocks = append(blocks, block{cat: e.Cat, text: sb.String()})
		total++
		if subs, ok := fmSubcommands[e.Name]; ok {
			for _, sc := range subs {
				var ss strings.Builder
				renderFullMenuBlock(&ss, sc)
				blocks = append(blocks, block{cat: e.Cat, text: ss.String()})
				total++
			}
		}
	}

	// ── Header (menu jesa fancy) ──
	var hdr strings.Builder
	hdr.WriteString("┏━┳━🔰 FULL MENU 🔰━┳━┓\n")
	fmt.Fprintf(&hdr, "*┃🔰 USER:❯ %s*\n", botNum)
	if ownerNum != "" {
		fmt.Fprintf(&hdr, "*┃🔰 OWNER:❯ %s*\n", ownerNum)
	}
	fmt.Fprintf(&hdr, "*┃🔰 TOTAL COMMANDS :❯ ❮ %d ❯*\n", total)
	fmt.Fprintf(&hdr, "*┃🔰 UPTIME :❯ %s*\n", uptimeHM)
	fmt.Fprintf(&hdr, "*┃🔰 PREFIX :❯ ❮ %s ❯*\n", prefix)
	if sessCount > 0 {
		fmt.Fprintf(&hdr, "*┃🔰 SESSIONS :❯ ❮ %d ❯*\n", sessCount)
	}
	hdr.WriteString("┗━┻━━━━━━━━━━━━┻━┛\n\n")
	hdr.WriteString(fmt.Sprintf("*%s — FULL COMMAND LIST WITH HIDDEN ALIASES*\n\n", strings.ToUpper(botName)))

	// ── Category banners ke saath blocks ──
	catRank := make(map[string]int)
	for i, c := range goldcmds.CategoryOrder {
		catRank[c] = i
	}
	// entries already sorted; walk blocks in order, banner jab category badle
	var body strings.Builder
	lastCat := ""
	for _, blk := range blocks {
		if blk.cat != lastCat {
			emoji := goldcmds.CategoryEmoji[blk.cat]
			if emoji == "" {
				emoji = "🔰"
			}
			fmt.Fprintf(&body, "*╭──〈 %s %s 〉──╮*\n\n", emoji, blk.cat)
			lastCat = blk.cat
		}
		body.WriteString(blk.text)
	}

	full := hdr.String() + body.String()

	// ── WhatsApp limit safety: 60k chars per message ──
	const maxLen = 60000
	if len(full) <= maxLen {
		return []string{full}
	}
	// Split at block boundaries.
	var parts []string
	var cur strings.Builder
	cur.WriteString(hdr.String())
	for _, blk := range blocks {
		if cur.Len()+len(blk.text) > maxLen-200 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
		cur.WriteString(blk.text)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// CmdFullMenu — .fullmenu handler. Puraa (hidden samet) command menu.
func (s *Session) CmdFullMenu(info types.MessageInfo, args []string, prefix string) {
	botName := "GOLD-MD WHATSAPP BOT"
	if s.Manager != nil && s.Manager.Redis != nil {
		if bn := s.Manager.Redis.GetSetting(s.JID, "botname", ""); bn != "" &&
			bn != goldcmds.DefaultBotNameMarker &&
			!strings.Contains(bn, "GOLD_MD_DEFAULT") {
			botName = bn
		}
	}
	// USER field = owner display name (.ownername, default UMAR) — .menu jesa.
	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}
	// OWNER field = owner number (.ownernumber + sudowners, fallback bot number).
	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}
	sessCount := 0
	if s.Manager != nil {
		sessCount = s.Manager.Count()
	}

	parts := buildFullMenuText(prefix, botName, menuUser, ownerNum, formatUptimeHM(uptime()), sessCount)
	for i, p := range parts {
		if i == len(parts)-1 {
			s.ReplyWithNewsletter(info, p)
		} else {
			s.Reply(info, p)
		}
	}
}
