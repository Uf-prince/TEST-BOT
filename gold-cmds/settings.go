package goldcmds

// ═══════════════════════════════════════════════════════════════════════════════
//  🔰 SETTINGS PANEL — ported from UMAR-MD pair.js (.settings / .setting)
//
//  Original Node.js: number-based settings panel jo har bot setting ko number
//  ke saath list karta hai — owner sirf number bhej kar setting on/off ya
//  change karta hai. "ask" wali entries pehle POORA explanation message bhejti
//  hain (kya bhejna hai, kaise bhejna hai, kya hoga), phir value validate kar
//  ke seedha command mein rewrite kar deti hain — is tarah har setting apna
//  asli command-handler hi chalata hai, koi duplicate logic nahi.
//
//  Go port (whatsmeow): same 0% farak — panel text, prompts, validators,
//  session TTL (2 min), claim-map (duplicate message guard) sab identical.
//  Session key = bot|chat (owner-only panel, LID/phone mismatch safe).
//  Emoji 👑 → 🔰 (GOLD-MD branding), Umar* → Gold* function names.
// ═══════════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// GOLD_SETTINGS_TTL_MS = 2 minutes (same as UMAR_SETTINGS_TTL_MS).
const GOLD_SETTINGS_TTL_MS = 2 * 60 * 1000 // 2 minutes

// settingsEntry mirrors one UMAR_SETTINGS_ENTRIES row.
// s = section header | t = menu par dikhne wala text | c = asal command
// ask = user se text maangna hai (us text ke saath command chalega)
type settingsEntry struct {
	s       string      // section header (👑 SECURITY SETTINGS 👑 style)
	t       string      // menu text
	c       string      // real command to run
	ask     string      // prompt text (needs a value)
	ex      string      // example block for the prompt
	why     string      // "what will happen" line
	choices [][2]string // fixed modes (name + description)
	num     bool        // expects a WhatsApp number
	url     bool        // expects a direct image URL
	wrap    bool        // wrap in {…} (welcome/goodbye)
	limit   int         // warn-limit validator (1..limit)
	time    bool        // "H M AM/PM" time validator
}

// goldSettingsList mirrors UMAR_SETTINGS_ENTRIES exactly (same order/text),
// with these verified-command adjustments for GOLD-MD:
//   - antispam → not implemented in GOLD-MD (kept off-panel, same as pair.js
//     bot me jab tak antispam ported ho) — REMOVED to avoid dead numbers.
//   - server → system (GOLD-MD ka naam)
//   - amute/aunmute, aimode → not in GOLD-MD registry (removed).
var goldSettingsList = []settingsEntry{
	{s: "SECURITY SETTINGS"},
	{t: "ANTICALL ON", c: "anticall on"},
	{t: "ANTICALL OFF", c: "anticall off"},
	{t: "ANTICALL REJECT MSG CHANGE", c: "anticall msg", ask: "SEND THE NEW ANTICALL REJECT MESSAGE",
		ex: "SEND THE COMPLETE TEXT IN ONE MESSAGE.\n*EXAMPLE ❮ PLEASE DO NOT CALL ME, SEND A MESSAGE 🔰 ❯*",
		why: "WHEN ANTICALL REJECTS A CALL, THIS MESSAGE WILL BE SENT TO THAT CALLER."},
	{t: "ANTICALL PREMIUM ADD", c: "anticallprem add", num: true, ask: "SEND THE NUMBER OR MENTION TO ADD IN ANTICALL PREMIUM",
		why: "CALLS FROM THAT NUMBER WILL NEVER BE REJECTED."},
	{t: "ANTICALL PREMIUM DEL", c: "anticallprem del", num: true, ask: "SEND THE NUMBER OR MENTION TO REMOVE FROM ANTICALL PREMIUM",
		why: "THAT NUMBER WILL BE REMOVED FROM THE PREMIUM LIST AND ITS CALLS WILL BE REJECTED AGAIN."},
	{t: "ANTICALL PREMIUM LIST", c: "anticallprem list"},
	{t: "ANTIDELETE MODE CHANGE", c: "antidelete", ask: "SEND THE ANTIDELETE MODE NAME",
		choices: [][2]string{{"on", "WORKS IN EVERY CHAT (INBOX + GROUPS)"}, {"inbox", "ONLY PRIVATE CHATS"}, {"groups", "ONLY GROUPS"}, {"off", "COMPLETELY DISABLE"}},
		why: "DELETED MESSAGES WILL BE RESTORED ACCORDING TO THE SELECTED SCOPE."},
	{t: "ANTIDELETE STATUS SHOW", c: "antidelete"},
	{t: "ANTIEDIT MODE CHANGE", c: "antiedit", ask: "SEND THE ANTIEDIT MODE NAME",
		choices: [][2]string{{"on", "WORKS IN EVERY CHAT (INBOX + GROUPS)"}, {"inbox", "ONLY PRIVATE CHATS"}, {"groups", "ONLY GROUPS"}, {"off", "COMPLETELY DISABLE"}},
		why: "THE ORIGINAL TEXT OF EDITED MESSAGES WILL BE SHOWN ACCORDING TO THE SELECTED SCOPE."},
	{t: "ANTIEDIT STATUS SHOW", c: "antiedit"},
	{t: "ANTIBAD ON", c: "antibad on"},
	{t: "ANTIBAD OFF", c: "antibad off"},
	{t: "ANTIBAD PREMIUM ADD", c: "antibadprem add", num: true, ask: "SEND THE NUMBER OR MENTION TO ADD IN ANTIBAD PREMIUM",
		why: "ANTIBAD WILL NOT TAKE ACTION AGAINST THAT USER."},
	{t: "ANTIBAD PREMIUM DEL", c: "antibadprem del", num: true, ask: "SEND THE NUMBER OR MENTION TO REMOVE FROM ANTIBAD PREMIUM",
		why: "ANTIBAD WILL APPLY TO THAT USER AGAIN."},
	{t: "ANTIBAD PREMIUM LIST", c: "antibadprem list"},
	{t: "BOT BLOCK A USER", c: "botblock", num: true, ask: "SEND THE NUMBER OR MENTION YOU WANT TO BLOCK FROM BOT",
		why: "THAT USER WILL NOT BE ABLE TO USE ANY BOT COMMAND."},
	{t: "BOT UNBLOCK A USER", c: "botunblock", num: true, ask: "SEND THE NUMBER OR MENTION YOU WANT TO UNBLOCK",
		why: "THAT USER WILL BE ABLE TO USE BOT COMMANDS AGAIN."},
	{t: "BOT BLOCK LIST", c: "botblocklist"},
	{t: "STOP A COMMAND", c: "cmdstop", ask: "SEND THE COMMAND NAME YOU WANT TO STOP",
		ex: "SEND ONLY THE COMMAND NAME WITHOUT A PREFIX.\n*EXAMPLE ❮ VIDEO ❯*",
		why: "THAT COMMAND WILL BE DISABLED FOR ALL USERS."},
	{t: "START A COMMAND", c: "cmdstart", ask: "SEND THE COMMAND NAME YOU WANT TO START AGAIN",
		ex: "SEND ONLY THE COMMAND NAME WITHOUT A PREFIX.\n*EXAMPLE ❮ VIDEO ❯*",
		why: "THAT COMMAND WILL WORK FOR EVERYONE AGAIN."},
	{t: "STOPPED COMMANDS LIST", c: "cmdstoplist"},

	{s: "STATUS SETTINGS"},
	{t: "ALWAYS ONLINE ON", c: "alwaysonline on"},
	{t: "ALWAYS ONLINE OFF", c: "alwaysonline off"},
	{t: "AUTO TYPING ON", c: "autotyping on"},
	{t: "AUTO TYPING OFF", c: "autotyping off"},
	{t: "AUTO RECORDING ON", c: "autorecording on"},
	{t: "AUTO RECORDING OFF", c: "autorecording off"},
	{t: "AUTO READ MODE CHANGE", c: "autoread", ask: "SEND THE AUTO READ MODE NAME",
		choices: [][2]string{{"all", "MESSAGES IN BOTH INBOX AND GROUPS WILL BE READ"}, {"inbox", "PRIVATE CHATS ONLY"}, {"groups", "GROUPS ONLY"}, {"off", "DISABLE AUTO READ"}},
		why: "THIS COMMAND REQUIRES A MODE NAME; SIMPLY SENDING ❮ ON ❯ DOES NOT WORK."},
	{t: "AUTO READ STATUS SHOW", c: "autoread"},
	{t: "STATUS SEEN ON", c: "statusseen on"},
	{t: "STATUS SEEN OFF", c: "statusseen off"},
	{t: "STATUS REACT ON", c: "statusreact on"},
	{t: "STATUS REACT OFF", c: "statusreact off"},
	{t: "STATUS REACT EMOJI SET", c: "statusreact emoji", ask: "SEND YOUR EMOJIS IN ONE MESSAGE",
		ex: "SEPARATE EMOJIS WITH COMMAS.\n*EXAMPLE ❮ 🔰,🔰,🔰 ❯*",
		why: "THE BOT WILL USE THESE EMOJIS TO REACT TO PEOPLE’S STATUSES."},
	{t: "STATUS REACT RESET", c: "statusreact reset"},
	{t: "STATUS REPLY ON", c: "statusreply on"},
	{t: "STATUS REPLY OFF", c: "statusreply off"},
	{t: "STATUS REPLY MSG SET", c: "statusreply message", ask: "SEND YOUR NEW STATUS REPLY MESSAGE",
		ex: "SEND THE COMPLETE TEXT IN ONE MESSAGE.\n*EXAMPLE ❮ NICE STATUS 🔰 ❯*",
		why: "THE BOT WILL REPLY TO EVERY STATUS WITH THIS TEXT."},
	{t: "STATUS REPLY RESET", c: "statusreply reset"},
	{t: "VOICE LIST", c: "voicelist"},
	{t: "DELETE A VOICE", c: "delvoice", ask: "SEND THE SAVED VOICE NAME YOU WANT TO DELETE",
		ex: "SEND ONLY THE NAME USED WHEN THE VOICE WAS SAVED.\n*EXAMPLE ❮ HELLO ❯*",
		why: "THE SAVED VOICE CLIP WITH THAT NAME WILL BE DELETED."},

	{s: "GROUP SETTINGS"},
	{t: "ANTILINK ON", c: "antilink on"},
	{t: "ANTILINK OFF", c: "antilink off"},
	{t: "ANTILINK ACTION CHANGE", c: "antilink action", ask: "SEND THE ANTILINK ACTION NAME",
		choices: [][2]string{{"delete", "ONLY THE MESSAGE CONTAINING THE LINK WILL BE DELETED"}, {"kick", "THE MEMBER WHO SENT THE LINK WILL BE REMOVED FROM THE GROUP"}, {"warn", "A WARNING WILL BE GIVEN FIRST; ACTION WILL FOLLOW WHEN THE LIMIT IS REACHED"}},
		why: "THE BOT WILL TAKE THIS ACTION WHEN A LINK IS SENT IN THE GROUP."},
	{t: "ANTILINK WARN LIMIT SET", c: "antilink action warn", ask: "SEND THE WARN LIMIT NUMBER",
		ex: "SEND A NUMBER BETWEEN 1 AND 50 ONLY.\n*EXAMPLE ❮ 5 ❯*",
		why: "ACTION WILL BE TAKEN AGAINST THE MEMBER AFTER THIS MANY WARNINGS.", limit: 50},
	{t: "ANTILINK WARN RESET", c: "antilink action warn reset"},
	{t: "ANTILINK ALLOW A DOMAIN", c: "antilink allow", ask: "SEND THE DOMAIN YOU WANT TO ALLOW",
		ex: "SEND ONLY THE DOMAIN; DO NOT INCLUDE https://.\n*EXAMPLE ❮ youtube.com ❯*",
		why: "ANTILINK WILL NOT TAKE ACTION ON LINKS FROM THIS DOMAIN."},
	{t: "ANTILINK REMOVE A DOMAIN", c: "antilink delete", ask: "SEND THE DOMAIN YOU WANT TO REMOVE FROM THE ALLOWED LIST",
		ex: "EXAMPLE ❮ youtube.com ❯*",
		why: "ANTILINK WILL TAKE ACTION ON LINKS FROM THIS DOMAIN AGAIN."},
	{t: "ANTILINK ALLOWED LIST", c: "antilink allowedlist"},
	{t: "ANTILINK FULL RESET", c: "antilink reset"},
	{t: "ANTILINK PREMIUM ADD", c: "antilinkprem add", num: true, ask: "SEND THE NUMBER OR MENTION TO ADD IN ANTILINK PREMIUM",
		why: "ANTILINK WILL NOT TAKE ACTION WHEN THAT USER SENDS A LINK."},
	{t: "ANTILINK PREMIUM DEL", c: "antilinkprem del", num: true, ask: "SEND THE NUMBER OR MENTION TO REMOVE FROM ANTILINK PREMIUM",
		why: "ANTILINK WILL APPLY TO THAT USER AGAIN."},
	{t: "ANTILINK PREMIUM LIST", c: "antilinkprem list"},
	{t: "ANTIBOT ON", c: "antibot on"},
	{t: "ANTIBOT OFF", c: "antibot off"},
	{t: "ANTIBOT ACTION CHANGE", c: "antibot action", ask: "SEND THE ANTIBOT ACTION NAME",
		choices: [][2]string{{"delete", "THE OTHER BOT’S MESSAGE WILL BE DELETED"}, {"kick", "THE OTHER BOT WILL BE REMOVED FROM THE GROUP"}, {"warn", "A WARNING WILL BE GIVEN FIRST; ACTION WILL FOLLOW WHEN THE LIMIT IS REACHED"}},
		why: "THE BOT WILL TAKE THIS ACTION WHEN ANOTHER BOT SENDS A GROUP MESSAGE."},
	{t: "ANTIBOT WARN LIMIT SET", c: "antibot action warn", ask: "SEND THE WARN LIMIT NUMBER",
		ex: "SEND A NUMBER BETWEEN 1 AND 50 ONLY.\n*EXAMPLE ❮ 5 ❯*",
		why: "ACTION WILL BE TAKEN AFTER THIS MANY WARNINGS.", limit: 50},
	{t: "ANTIBOT WARN RESET", c: "antibot action warn reset"},
	{t: "ANTIBOT FULL RESET", c: "antibot reset"},
	{t: "ANTIBOT PREMIUM ADD", c: "antibotprem add", num: true, ask: "SEND THE NUMBER OR MENTION TO ADD IN ANTIBOT PREMIUM",
		why: "ANTIBOT WILL NOT TAKE ACTION AGAINST THAT USER."},
	{t: "ANTIBOT PREMIUM DEL", c: "antibotprem del", num: true, ask: "SEND THE NUMBER OR MENTION TO REMOVE FROM ANTIBOT PREMIUM",
		why: "ANTIBOT WILL APPLY TO THAT USER AGAIN."},
	{t: "ANTIBOT PREMIUM LIST", c: "antibotprem list"},

	{t: "AUTOREPLY ON", c: "autoreply on"},
	{t: "AUTOREPLY GROUPS", c: "autoreply groups"},
	{t: "AUTOREPLY INBOX", c: "autoreply inbox"},
	{t: "AUTOREPLY OFF", c: "autoreply off"},
	{t: "AUTOREPLY PREM ADD", c: "autoreplyprem add", num: true, ask: "SEND THE NUMBER OR MENTION — AUTOREPLY WILL STOP REPLYING TO THIS USER",
		why: "AUTOREPLY WILL NEVER REPLY TO THIS USER."},
	{t: "AUTOREPLY PREM DEL", c: "autoreplyprem del", num: true, ask: "SEND THE NUMBER OR MENTION — AUTOREPLY WILL START REPLYING TO THIS USER AGAIN",
		why: "AUTOREPLY WILL REPLY TO THIS USER AGAIN."},
	{t: "AUTOREPLY PREM LIST", c: "autoreplyprem list"},

	{t: "WELCOME ON", c: "welcome on"},
	{t: "WELCOME OFF", c: "welcome off"},
	{t: "WELCOME MSG CHANGE", c: "welcome msg", wrap: true, ask: "SEND YOUR NEW WELCOME MESSAGE",
		ex: "SEND ONLY YOUR TEXT; BRACKETS WILL BE ADDED AUTOMATICALLY.\n*EXAMPLE ❮ HEY @user, WELCOME TO @gname 🔰 ❯*",
		why: "❮ @user ❯ WILL MENTION THE NEW MEMBER AND ❮ @gname ❯ WILL SHOW THE GROUP NAME."},
	{t: "WELCOME RESET", c: "welcome reset"},
	{t: "GOODBYE ON", c: "goodbye on"},
	{t: "GOODBYE OFF", c: "goodbye off"},
	{t: "GOODBYE MSG CHANGE", c: "goodbye msg", wrap: true, ask: "SEND YOUR NEW GOODBYE MESSAGE",
		ex: "SEND ONLY YOUR TEXT; BRACKETS WILL BE ADDED AUTOMATICALLY.\n*EXAMPLE ❮ GOODBYE @user FROM @gname 🔰 ❯*",
		why: "❮ @user ❯ WILL MENTION THE LEAVING MEMBER AND ❮ @gname ❯ WILL SHOW THE GROUP NAME."},
	{t: "GOODBYE RESET", c: "goodbye reset"},
	{t: "LOCK GROUP (ADMINS ONLY)", c: "gcbotoff"},
	{t: "UNLOCK GROUP (ALL MEMBERS)", c: "gcboton"},
	{t: "BAN A MEMBER IN THIS GROUP", c: "usergcban", num: true, ask: "SEND THE NUMBER OR MENTION YOU WANT TO BAN IN THIS GROUP",
		why: "THAT MEMBER WILL NOT BE ABLE TO SEND MESSAGES IN THIS GROUP."},
	{t: "UNBAN A MEMBER IN THIS GROUP", c: "usergcunban", num: true, ask: "SEND THE NUMBER OR MENTION YOU WANT TO UNBAN IN THIS GROUP",
		why: "THAT MEMBER WILL BE ABLE TO SEND MESSAGES IN THIS GROUP AGAIN."},
	{t: "GROUP BANNED MEMBERS LIST", c: "usergcban list"},

	{s: "BOT SETTINGS"},
	{t: "BOT MODE CHANGE", c: "mode", ask: "SEND THE MODE NAME YOU WANT",
		choices: [][2]string{{"public", "ANYONE CAN USE THE BOT COMMANDS"}, {"private", "ONLY THE OWNER CAN USE THE BOT"}, {"groups", "THE BOT WILL WORK ONLY IN GROUPS, NOT IN PRIVATE CHATS"}, {"inbox", "THE BOT WILL WORK ONLY IN PRIVATE CHATS, NOT IN GROUPS"}},
		why: "THE BOT’S ENTIRE OPERATING SCOPE WILL FOLLOW THIS MODE."},
	{t: "CURRENT MODE SHOW", c: "mode"},
	{t: "CHANGE PREFIX", c: "prefix", ask: "SEND YOUR NEW PREFIX SYMBOL",
		ex: "SEND ONLY ONE SYMBOL.\n*EXAMPLE ❮ ! ❯*",
		why: "ALL COMMANDS WILL USE THE NEW SYMBOL AFTER THIS CHANGE."},
	{t: "AUTO REACT ON", c: "autoreact on"},
	{t: "AUTO REACT OFF", c: "autoreact off"},
	{t: "AUTO REACT EMOJI SET", c: "autoreact emoji", ask: "SEND YOUR EMOJIS IN ONE MESSAGE",
		ex: "MAXIMUM 20 EMOJIS, SEPARATED BY COMMAS.\n*EXAMPLE ❮ 🔰,🔰,🔰 ❯*",
		why: "THE BOT WILL USE THESE EMOJIS TO REACT TO INCOMING MESSAGES."},
	{t: "AUTO REACT RESET", c: "autoreact reset"},
	{t: "OWNER REACT ON", c: "ownerreact on"},
	{t: "OWNER REACT OFF", c: "ownerreact off"},
	{t: "OWNER REACT EMOJI SET", c: "ownerreact emoji", ask: "SEND YOUR EMOJIS IN ONE MESSAGE",
		ex: "MAXIMUM 20 EMOJIS, SEPARATED BY COMMAS.\n*EXAMPLE ❮ 🔰,🔰,😎 ❯*",
		why: "THE BOT WILL USE THESE EMOJIS ONLY ON THE OWNER’S MESSAGES."},
	{t: "OWNER REACT RESET", c: "ownerreact reset"},
	{t: "CHANGE BOT PIC (MENU + ALIVE)", c: "botpic", url: true, ask: "SEND THE NEW BOT PIC LINK",
		ex: "SEND A DIRECT IMAGE LINK ENDING IN .jpg, .jpeg, .png, .gif, OR .webp.\n*EXAMPLE ❮ https://example.com/photo.jpg ❯*",
		why: "YEH IMAGE MENU AUR ALIVE DONO MEIN DIKHEGI."},
	{t: "BOT PIC RESET", c: "botpic reset"},
	{t: "CHANGE ALIVE MSG", c: "alivemsg", ask: "SEND YOUR NEW ALIVE MESSAGE",
		ex: "SEND THE COMPLETE TEXT IN ONE MESSAGE.\n*EXAMPLE ❮ BOT IS ONLINE AND WORKING 🔰 ❯*",
		why: "THIS TEXT WILL APPEAR WITH THE ALIVE COMMAND."},
	{t: "ALIVE MSG RESET", c: "alivemsg reset"},
	{t: "CHANGE BOT NAME", c: "botname", ask: "SEND YOUR NEW BOT NAME",
		ex: "SEND ONLY THE NAME.\n*EXAMPLE ❮ UMAR MD ❯*",
		why: "THIS BOT NAME WILL APPEAR EVERYWHERE."},
	{t: "BOT NAME RESET", c: "botname reset"},
	{t: "CHANGE OWNER NAME", c: "ownername", ask: "SEND YOUR NEW OWNER NAME",
		ex: "SEND ONLY THE NAME, NOT A NUMBER.\n*EXAMPLE ❮ UMAR KING ❯*",
		why: "THIS OWNER NAME WILL APPEAR EVERYWHERE."},
	{t: "OWNER NAME RESET", c: "ownername reset"},
	{t: "CHANGE OWNER NUMBER", c: "ownernumber", num: true, ask: "SEND YOUR NEW OWNER WHATSAPP NUMBER",
		why: "THE BOT WILL RECOGNIZE THIS NUMBER AS ITS OWNER."},
	{t: "OWNER NUMBER RESET", c: "ownernumber reset"},
	{t: "SERVER INFO", c: "system"},
}

// settingsSession mirrors the Node.js session object (pending + expiry).
type settingsSession struct {
	pending  *settingsEntry
	expires  time.Time
}

// ─────────────────────────────────────────────────────────────────────────────
//  Session store — mirrors _UmarSettingsSessions + _UmarClaimSettingsMessage.
//  A single global mutex keeps both maps consistent (Go port detail; the
//  behaviour is 0% farak with the Node.js single-threaded equivalent).
// ─────────────────────────────────────────────────────────────────────────────
var (
	settingsMu       sync.Mutex
	settingsSessions = map[string]*settingsSession{}
	settingsHandled  = map[string]time.Time{}
)

// settingsKey mirrors _UmarSettingsKey: bot + chat only (owner-only panel;
// LID/phone mismatch safe — same fix as pair.js).
func settingsKey(botJID, chatJID string) string {
	return digitsOnly(botJID) + "|" + strings.ToLower(chatJID)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// claimSettingsMessage mirrors _UmarClaimSettingsMessage — one WhatsApp
// message ID can never produce two panel replies (whatsmeow reconnect dupes).
func claimSettingsMessage(sessionKey, messageID string) bool {
	if messageID == "" {
		return true
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	now := time.Now()
	for k, seenAt := range settingsHandled {
		if now.Sub(seenAt) > time.Duration(GOLD_SETTINGS_TTL_MS)*time.Millisecond {
			delete(settingsHandled, k)
		}
	}
	key := sessionKey + "|" + messageID
	if _, dup := settingsHandled[key]; dup {
		return false
	}
	settingsHandled[key] = now
	return true
}

// getSettingsSession mirrors _UmarGetSettingsSession (auto-expire).
func getSettingsSession(key string) *settingsSession {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s, ok := settingsSessions[key]
	if !ok {
		return nil
	}
	if time.Now().After(s.expires) {
		delete(settingsSessions, key)
		return nil
	}
	return s
}

// touchSettingsSession mirrors _UmarTouchSettingsSession (extend TTL).
func touchSettingsSession(key string, pending *settingsEntry) *settingsSession {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s := settingsSessions[key]
	if s == nil {
		s = &settingsSession{}
	}
	s.pending = pending
	s.expires = time.Now().Add(time.Duration(GOLD_SETTINGS_TTL_MS) * time.Millisecond)
	settingsSessions[key] = s
	return s
}

// deleteSettingsSession closes the panel for this chat.
func deleteSettingsSession(key string) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	delete(settingsSessions, key)
}

// hadSettingsSession reports whether the chat had ANY session (even expired)
// and whether it was waiting for a value — used for the expired notice.
func hadSettingsSession(key string) (had, pending bool) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s, ok := settingsSessions[key]
	if !ok {
		return false, false
	}
	return true, s.pending != nil
}

// ─────────────────────────────────────────────────────────────────────────────
//  goldSettingsMap — number → entry (UMAR_SETTINGS_MAP). The menu number is
//  always generated from this list, so numbering and action can never
//  mismatch.
// ─────────────────────────────────────────────────────────────────────────────
var goldSettingsMap = func() map[string]*settingsEntry {
	m := map[string]*settingsEntry{}
	n := 0
	for i := range goldSettingsList {
		e := &goldSettingsList[i]
		if e.s != "" {
			continue
		}
		n++
		m[strconv.Itoa(n)] = e
	}
	return m
}()

// settingsAliases mirrors the Node.js _stAliases list.
var settingsAliases = map[string]bool{
	"settings": true, "setting": true, "settng": true,
	"stng": true, "stting": true, "seting": true, "panel": true,
}

// settingsRetryTail mirrors _UmarSettingsRetryTail.
func settingsRetryTail() string {
	return "\n\n*SEND THE CORRECT VALUE AGAIN IN YOUR NEXT MESSAGE.*"
}

// buildSettingsPages mirrors _UmarBuildSettingsPages — SAB KUCH EK HI
// MESSAGE MEIN. Emoji 👑 → 🔰 (GOLD-MD branding).
func buildSettingsPages(prefix string) []string {
	n := 0
	var body strings.Builder
	for _, e := range goldSettingsList {
		if e.s != "" {
			body.WriteString("\n*🔰 " + e.s + " 🔰*\n\n")
			continue
		}
		n++
		hand := ""
		if e.ask != "" {
			hand = " 🔰"
		}
		body.WriteString(fmt.Sprintf("*❮ %d ❯ %s*%s\n", n, e.t, hand))
	}
	head := `*🔰 BOT ALL SETTINGS 🔰*

*SEE VIDEO HOW TO CHANGE SETTINGS*
https://youtu.be/HQfZ8AF6Teg?is=RMQ7pkSVkfOUYR8O

*JUST TYPE ANY NUMBER EG 1 , 2 , 3 , 4 WHICH SETTING DO YOU WANT TO CHANGE WHEN YOU TYPE ANY NUMBER BOT WILL CHANGE SETTINGS*

*🔰  WARNING WARNING  WARNING 🔰*
*MENTION THIS MESSAGE AND THEN AFTER TYPE NUMBER OK 🔰 MENTION THIS MESSAGE FIRST THEN REPLY NUMBER*
*🔰  WARNING WARNING WARNING 🔰*

*🔰 MEANS THE SETTING NEEDS A VALUE AND WILL NOT CHANGE IMMEDIATELY.*
*THE BOT WILL FIRST EXPLAIN WHAT TO SEND, HOW TO SEND IT, AND WHAT WILL HAPPEN.*
*THEN SEND THE VALUE, NUMBER, OR TEXT MESSAGES TO CHANGE THE SETTING.*

*OPTIONS WITHOUT 🔰 ARE ON/OFF OR LIST ACTIONS AND RUN AS SOON AS YOU SEND THEIR NUMBER.*`
	return []string{head + strings.TrimRight(body.String(), " \n")}
}

// buildSettingsAskPrompt mirrors _UmarBuildSettingsAskPrompt — the full
// explanation message for every ask-type setting. Emoji 👑 → 🔰.
func buildSettingsAskPrompt(e *settingsEntry, prefix string) string {
	title := e.t
	if title == "" {
		title = "SETTING"
	}
	command := e.c
	ask := e.ask
	if ask == "" {
		ask = "SEND THE REQUIRED VALUE"
	}
	why := e.why
	if why == "" {
		why = "YOUR VALUE WILL BE SAVED FOR ❮ " + title + " ❯."
	}

	example := e.ex
	if example == "" {
		example = "SEND THE VALUE AS A NORMAL TEXT MESSAGE."
	}
	modesBlock := ""

	if len(e.choices) > 0 {
		var list strings.Builder
		for _, ch := range e.choices {
			list.WriteString(fmt.Sprintf("*❮ %s ❯ ➜ %s*\n", strings.ToUpper(ch[0]), ch[1]))
		}
		modesBlock = "\n*AVAILABLE MODES:*\n" + list.String()
		example = fmt.Sprintf("SEND ONLY THE MODE NAME, NOT THE COMMAND.\n*EXAMPLE ❮ %s ❯*\n*THIS WILL CREATE THE FINAL COMMAND ❮ %s%s %s ❯*",
			strings.ToUpper(e.choices[0][0]), prefix, command, e.choices[0][0])
	} else if e.num {
		example = "SEND THE COMPLETE NUMBER WITH COUNTRY CODE, WITHOUT 🔰 OR A LEADING 0.\n*EXAMPLE ❮ 923001234567 ❯*\n*IN A GROUP, YOU CAN ALSO MENTION THE USER ❮ @USER ❯*"
	}

	// Example block: har non-empty line ko *…* me wrap karo (same as Node).
	var exLines []string
	for _, l := range strings.Split(example, "\n") {
		t := strings.TrimSpace(l)
		if t == "" {
			exLines = append(exLines, l)
			continue
		}
		starred := strings.HasPrefix(t, "*") && strings.HasSuffix(t, "*") && len(t) >= 2
		if starred {
			exLines = append(exLines, t)
		} else {
			exLines = append(exLines, "*"+t+"*")
		}
	}

	return fmt.Sprintf(`*🔰 %s 🔰*

%s
*WHAT TO SEND:*
*%s*

*HOW TO SEND IT:*
%s

*WHAT WILL HAPPEN:*
*%s*

*YOU DON'T HAVE TO DO ANYTHING JUST SEND YOUR SIMPLE VALUE BELOW AS SOON AS YOU SEND THE VALUES, THE BOT SETTINGS WILL CHANGE IMMEDIATELY*
`, title, modesBlock, ask, strings.Join(exLines, "\n"), why)
}

// isNumberAskSetting mirrors _UmarIsNumberAskSetting.
func isNumberAskSetting(e *settingsEntry) bool {
	if e.num {
		return true
	}
	c := strings.ToLower(e.c)
	for _, p := range []string{"botblock", "botunblock", "ownernumber", "usergcban", "usergcunban"} {
		if c == p || c == p+" list" {
			return true
		}
	}
	return strings.HasSuffix(c, "prem add") || strings.HasSuffix(c, "prem del")
}

// numCheck mirrors the Node number validator (section 6). Emoji 👑 → 🔰.
func numCheck(raw string) (ok bool, value, msg string) {
	if strings.Contains(raw, "@") {
		return true, raw, ""
	}
	digits := sanitizeDigits(raw)
	onlyDigits := isAllDigits(digits)
	startsZero := strings.HasPrefix(digits, "0")
	goodLen := len(digits) >= 11 && len(digits) <= 15
	if onlyDigits && !startsZero && goodLen {
		return true, digits, ""
	}
	reason := "*YOU DID NOT SEND A VALID WHATSAPP NUMBER 🔰*"
	if !onlyDigits {
		reason = "*YOU SENT SOME LETTERS OR SYMBOLS, ONLY DIGITS ARE ALLOWED 🔰*"
	} else if startsZero {
		reason = "*YOU STARTED THE NUMBER WITH 0 LIKE ❮ 03XXXXXXXXX ❯, THAT IS A LOCAL FORMAT 🔰*"
	} else if !goodLen {
		reason = "*YOUR NUMBER IS TOO SHORT OR TOO LONG, SEND THE COMPLETE NUMBER 🔰*"
	}
	return false, "", fmt.Sprintf(`*🔰 WRONG NUMBER 🔰*

%s

*TYPE NUMBER WITH YOUR COUNTRY CODE WITHOUT TYPING 🔰 AND TYPE THE FULL NUMBER SAME LIKE 923XXXXXXXXX*

*🔰 WRONG WAY*
*❮ 03001234567 ❯*
*❮ 3001234567 ❯*
*❮ +92 300 1234567 ❯*

*🔰 RIGHT WAY*
*❮ 923001234567 ❯*
*❮ 919876543210 ❯*
*❮ 8801712345678 ❯*

*RULE: COUNTRY CODE FIRST, NO + SIGN, NO ZERO, NO SPACE, NO DASH.*
*IN A GROUP YOU CAN ALSO JUST MENTION THE USER LIKE ❮ @USER ❯.*%s`, reason, settingsRetryTail())
}

func sanitizeDigits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch r {
		case ' ', '-', '(', ')', '+':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isAllDigits is defined in anti_common.go (same package) — reused.

// checkSettingsInput mirrors _UmarCheckSettingsNumberInput — the ONE
// validator for modes, warn limits, image URLs, times, wrapped messages and
// numbers. Ghalat input par session zinda rehta hai aur owner ko poora
// explain message jata hai. Emoji 👑 → 🔰.
func checkSettingsInput(e *settingsEntry, raw string) (ok bool, value, msg string) {
	raw = strings.TrimSpace(raw)
	if e == nil {
		return true, raw, ""
	}

	// ── 1) MODE / ACTION WALE COMMANDS ──
	if len(e.choices) > 0 {
		want := strings.ToLower(raw)
		want = strings.TrimLeft(want, ".!/#")
		want = strings.TrimSpace(want)
		base := strings.ToLower(e.c)
		clean := strings.TrimSpace(strings.TrimPrefix(want, base+" "))
		for _, ch := range e.choices {
			if strings.ToLower(ch[0]) == clean {
				return true, ch[0], ""
			}
		}
		var list strings.Builder
		for _, ch := range e.choices {
			list.WriteString(fmt.Sprintf("*❮ %s ❯ ➜ %s*\n", strings.ToUpper(ch[0]), ch[1]))
		}
		return false, "", fmt.Sprintf(`*🔰 WRONG MODE NAME 🔰*

*THE VALUE YOU SENT IS NOT A VALID MODE FOR THIS SETTING 🔰*

*SEND ONLY ONE OF THESE MODE NAMES:*
%s
*🔰 WRONG WAY*
*❮ %s ❯*

*🔰 RIGHT WAY*
*❮ %s ❯*%s`, list.String(), rawOr(raw, "YES"), strings.ToUpper(e.choices[0][0]), settingsRetryTail())
	}

	// ── 2) WARN LIMIT WALE COMMANDS ──
	if e.limit > 0 {
		n, _ := strconv.Atoi(digitsOnly(raw))
		if isAllDigits(raw) && n >= 1 && n <= e.limit {
			return true, strconv.Itoa(n), ""
		}
		return false, "", fmt.Sprintf(`*🔰 WRONG WARN LIMIT 🔰*

*SEND A NUMBER BETWEEN 1 AND %d ONLY 🔰*

*🔰 WRONG WAY*
*❮ %s ❯*

*🔰 RIGHT WAY*
*❮ 5 ❯*%s`, e.limit, rawOr(raw, "FIVE"), settingsRetryTail())
	}

	// ── 3) IMAGE URL WALE COMMANDS ──
	if e.url {
		if imgURLRe.MatchString(raw) {
			return true, raw, ""
		}
		shown := rawOr(raw, "PHOTO")
		if len(shown) > 60 {
			shown = shown[:60]
		}
		return false, "", fmt.Sprintf(`*🔰 WRONG IMAGE LINK 🔰*

*THE BOT NEEDS A DIRECT IMAGE LINK ENDING IN .jpg, .jpeg, .png, .gif, OR .webp 🔰*
*AN ATTACHED PHOTO WILL NOT WORK; SEND THE LINK ITSELF.*

*🔰 WRONG WAY*
*❮ %s ❯*

*🔰 RIGHT WAY*
*❮ https://example.com/photo.jpg ❯*%s`, shown, settingsRetryTail())
	}

	// ── 4) AUTO MUTE / AUTO UNMUTE TIME ──
	if e.time {
		m := timeRe.FindStringSubmatch(raw)
		if m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			if h >= 1 && h <= 12 && mi >= 0 && mi <= 59 {
				return true, fmt.Sprintf("%d %02d %s", h, mi, strings.ToUpper(m[3])), ""
			}
		}
		return false, "", fmt.Sprintf(`*🔰 WRONG TIME FORMAT 🔰*

*SEND TIME AS ❮ HOUR MINUTE AM/PM ❯ 🔰*

*🔰 WRONG WAY*
*❮ %s ❯*

*🔰 RIGHT WAY*
*❮ 7 30 PM ❯*
*❮ 8 00 AM ❯*%s`, rawOr(raw, "19:30"), settingsRetryTail())
	}

	// ── 5) WELCOME / GOODBYE JAISE BRACE WALE MESSAGES ──
	if e.wrap {
		if raw == "" {
			return false, "", "*🔰 EMPTY MESSAGE 🔰*\n\n*YOU DID NOT SEND ANY TEXT 🔰*" + settingsRetryTail()
		}
		inner := strings.TrimSpace(strings.TrimRight(strings.TrimLeft(raw, "{"), "}"))
		return true, "{" + inner + "}", ""
	}

	// ── 6) NUMBER WALE COMMANDS ──
	if !isNumberAskSetting(e) {
		if raw == "" {
			return false, "", "*🔰 EMPTY VALUE 🔰*\n\n*YOU DID NOT SEND A VALUE 🔰*" + settingsRetryTail()
		}
		return true, raw, ""
	}
	return numCheck(raw)
}

func rawOr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

var (
	imgURLRe = regexp.MustCompile(`(?i)^https?://\S+\.(jpg|jpeg|png|gif|webp)(\?\S*)?$`)
	timeRe   = regexp.MustCompile(`(?i)^([0-9]{1,2})[\s:]+([0-9]{1,2})\s*(AM|PM)$`)
)

// commandOnlyRe mirrors _reCmdOnlySp — bare command word (optional prefix).
var commandOnlyRe = regexp.MustCompile(`(?i)^[.!#/]?\s*([a-z0-9_]+)\s*$`)

// isBareNumber reports whether the body is a 1-3 digit selection number.
func isBareNumber(body string) bool {
	b := strings.TrimSpace(body)
	if b == "" {
		return false
	}
	for _, r := range b {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(b) >= 1 && len(b) <= 3
}

// ─────────────────────────────────────────────────────────────────────────────
//  SettingsTryHandle — the panel hook. Runs on EVERY incoming message,
//  BEFORE command dispatch (same position as the Node.js block in pair.js).
//
//  Returns:
//    handled=true, rewrite="" → message fully consumed (reply sent); return.
//    handled=true, rewrite!="" → the owner's number/value was converted into
//        the real prefixed command (e.g. ".anticall on"); caller must
//        re-dispatch that NEW text through the normal command pipeline.
//    handled=false → panel didn't consume the message; continue as normal.
// ─────────────────────────────────────────────────────────────────────────────
func SettingsTryHandle(s SessionBridge, info types.MessageInfo, body string, prefix string) (handled bool, rewrite string) {
	// NOTE: NO IsFromMe guard here (FIX for owner-number-paired bots).
	// Bot owner ke hi WhatsApp number par pair hai — owner phone se kuch
	// bhi bhejta hai to whatsmeow usko IsFromMe=true deta hai, aur
	// IsOwner() bhi fromMe=owner mana jata hai (commands_loader.go).
	// Original pair.js me bhi UmarIsOwner = fromMe === true hai (10769).
	// whatsmeow me bot ke APNE sends events nahi bante (Baileys me bante
	// the, isliye wahan BOT_TOKEN self-echo guard tha) — yahan zaroorat nahi.

	// Bare command (with or without prefix) for alias matching.
	cmd := ""
	if m := commandOnlyRe.FindStringSubmatch(strings.TrimSpace(body)); m != nil {
		cmd = strings.ToLower(m[1])
	}

	// ── .settings — panel kholo ──
	if settingsAliases[cmd] {
		if !s.IsOwner(info) {
			s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
			return true, ""
		}
		if !claimSettingsMessage(settingsKey(s.GetJID(), info.Chat.String()), info.ID) {
			return true, ""
		}
		pages := buildSettingsPages(prefix)
		for _, pg := range pages {
			s.Reply(info, pg)
		}
		touchSettingsSession(settingsKey(s.GetJID(), info.Chat.String()), nil)
		return true, ""
	}

	key := settingsKey(s.GetJID(), info.Chat.String())

	// ── panel khula hua hai? number / maanga hua text handle karo ──
	// FIX (same as Node): capture the RAW (maybe-expired) session BEFORE
	// auto-delete so a pending ask that expired still shows the expired
	// notice on TEXT replies too, not only digits.
	hadSess, wasPending := hadSettingsSession(key)
	sess := getSettingsSession(key)
	if sess == nil && hadSess && strings.TrimSpace(body) != "" && s.IsOwner(info) &&
		(wasPending || isBareNumber(body)) {
		s.Reply(info, "*🔰 SETTINGS COMMAND STOPPED 🔰*\n\n*TYPE ❮ "+prefix+"SETTINGS ❯ AND SELECT THE OPTION AGAIN.*")
		return true, ""
	}

	if sess != nil && strings.TrimSpace(body) != "" && s.IsOwner(info) {
		// panel band karna
		b := strings.TrimSpace(strings.ToLower(body))
		if b == "0" || b == "exit" || b == "close" || b == "cancel" || b == "band" || b == "khatam" {
			deleteSettingsSession(key)
			s.Reply(info, "*🔰 SETTINGS PANEL CLOSED 🔰*\n\n*OPEN IT AGAIN WHENEVER YOU WANT 🔰*")
			return true, ""
		}

		rewrite := ""

		// ── SESSION HAS 2 SEPARATE STAGES ──
		// Stage 1: OWNER SELECTS THE NUMBER — decides WHICH setting to
		//   change; opens Stage 2 only if a value is needed.
		// Stage 2 (pending): OWNER APNI VALUE LIKHTA HAI — validated, then
		//   applied immediately, no extra confirm step.
		if sess.pending != nil {
			// bot ne text maanga tha
			// In no-prefix mode an empty prefix means ANY text is the value
			// (same fix as Node: _pfxSrc !== '' check).
			if prefix != "" && strings.HasPrefix(strings.TrimSpace(body), prefix) {
				// user ne koi doosra command likh diya — text maangna chhodo
				touchSettingsSession(key, nil)
			} else {
				ent := sess.pending
				ok, value, failMsg := checkSettingsInput(ent, body)
				if !ok {
					touchSettingsSession(key, ent)
					s.Reply(info, failMsg)
					touchSettingsSession(key, ent)
					return true, ""
				}
				// Value sahi hai — number select karke owner ne pehle hi
				// bata diya tha ke change karna hai, ab seedha apply karo.
				touchSettingsSession(key, nil)
				v := value
				if v == "" {
					v = body
				}
				rewrite = strings.TrimSpace(ent.c + " " + v)
			}
		} else if isBareNumber(body) {
			if !claimSettingsMessage(key, info.ID) {
				return true, ""
			}
			ent := goldSettingsMap[strings.TrimSpace(body)]
			if ent == nil {
				s.Reply(info, "*🔰 WRONG NUMBER 🔰*\n\n*THIS NUMBER IS NOT IN THE SETTINGS LIST 🔰*\n\n*TYPE ❮ "+prefix+"SETTINGS ❯ TO SEE THE LIST AGAIN*")
				return true, ""
			}
			if ent.ask != "" {
				touchSettingsSession(key, ent)
				s.Reply(info, buildSettingsAskPrompt(ent, prefix))
				// Fresh two-minute window starts AFTER the question was sent
				// (delivery can itself take time under load) — same as Node.
				touchSettingsSession(key, ent)
				return true, ""
			}
			rewrite = ent.c
		}

		if rewrite != "" {
			// asli command banao (is bot ka apna prefix laga kar)
			newText := prefix + rewrite
			// FIX (same as Node): successful selection/value → session turant
			// band (delete). Owner agli setting ke liye naya ❮ .settings ❯
			// bhej ke fresh session khol sakta hai.
			deleteSettingsSession(key)
			return true, newText
		}
	}
	return false, ""
}

func init() {
	// Register the visible command so it shows in .menu (TOOLS category) and
	// can be invoked directly as .settings (aliases handled by the pre-hook).
	Register(Command{
		Name:      "settings",
		Category:  "TOOLS",
		Desc:      "Opens a number-based settings panel that lists every bot setting (anticall, antilink, welcome, mode, prefix etc.) with numbers — owner just sends the number to turn that setting on/off or change it. No arguments, e.g. \"settings\". Panel stays open for 2 minutes.",
		OwnerOnly: true,
		Run:       handleSettingsCmd,
	})
}

// handleSettingsCmd — direct .settings invocation (no args needed).
func handleSettingsCmd(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if len(args) > 0 {
		// .settings <number> → same as selecting the number in the panel.
		a := strings.TrimSpace(args[0])
		if isBareNumber(a) {
			if handled, _ := SettingsTryHandle(s, info, a, prefix); handled {
				return
			}
		}
	}
	pages := buildSettingsPages(prefix)
	for _, pg := range pages {
		s.Reply(info, pg)
	}
	touchSettingsSession(settingsKey(s.GetJID(), info.Chat.String()), nil)
}
