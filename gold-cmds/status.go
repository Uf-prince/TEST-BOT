package goldcmds

// ============================================================================
// GOLD-MD — Status (story) commands: .statusseen, .statusreact, .statusreply
//
// Ported from UMAR-MD (Node.js pair.js) — same text style, same behaviour:
//   .statusseen on/off        → bot auto-sees (marks read) everyone's status
//   .statusreact on/off        → bot auto-reacts on statuses with emojis
//   .statusreact emoji <list>  → set custom emojis (comma/space separated)
//   .statusreact reset         → reset emojis to default
//   .statusreply on/off        → bot auto-replies on statuses with a message
//   .statusreply message <txt> → set custom reply message
//   .statusreply reset         → reset message to default
//
// Per-user config stored in Redis (Upstash) via settings:<botJID> hash:
//   field "statusseen"          = "true"/"false"
//   field "statusreact"         = "true"/"false"
//   field "statusreactemojis"   = comma-separated emoji list
//   field "statusreply"         = "true"/"false"
//   field "statusreplymessage"  = custom reply text
//
// Conflict rule (same as Node.js): statusreact ON and statusreply ON both
// require statusseen to be ON first. If you try .statusreact on or
// .statusreply on while statusseen is OFF → error message tells you to
// first turn statusseen on.
//
// The actual auto-seen/react/reply on every incoming status message
// (status@broadcast) is applied in handler.go (applyAutoStatus), exactly
// like pair.js lines 15866-15955.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"math/rand"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── defaults (same as Node.js pair.js) ──────────────────────────────────────

// DEFAULT_STATUS_REACT_EMOJIS — same as pair.js line 4593:
// ['😊','❤️','🥰','💮','☺️','🤗','🥰']
var defaultStatusReactEmojis = []string{"🔰", "🔰", "🔰", "🔰", "🔰", "🔰", "🔰"}

// DEFAULT_STATUS_REPLY_MESSAGE — same as pair.js line 5863:
// 'Thanks for your status! 💚'
const defaultStatusReplyMessage = "Thanks for your status! 🔰"

// ── helpers ─────────────────────────────────────────────────────────────────

// statusIsOn reads a status boolean setting from Redis.
func statusIsOn(s SessionBridge, field string) bool {
	v := s.GetStatusSetting(field, "false")
	return v == "true" || v == "1" || v == "on"
}

// statusSetOn writes a status boolean setting to Redis.
func statusSetOn(s SessionBridge, field string, on bool) {
	if on {
		s.SetStatusSetting(field, "true")
	} else {
		s.SetStatusSetting(field, "false")
	}
}

// getReactEmojis returns the custom emoji list if set, otherwise the default.
func getReactEmojis(s SessionBridge) []string {
	v := s.GetStatusSetting("statusreactemojis", "")
	if v == "" {
		return defaultStatusReactEmojis
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
		return defaultStatusReactEmojis
	}
	return out
}

// getReplyMessage returns the custom reply message if set, otherwise default.
func getReplyMessage(s SessionBridge) string {
	v := s.GetStatusSetting("statusreplymessage", "")
	if v == "" {
		return defaultStatusReplyMessage
	}
	return v
}

// ── .statusseen ─────────────────────────────────────────────────────────────
// Aliases (Node.js): statusseen, seenstatus, autostatusseen, autoseenstatus,
// autoviewstatus, autostatusview, viewstatus, statusview

func handleStatusSeen(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleStatusSeenAsync(s, info, args, prefix)
}

func handleStatusSeenAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		current := statusIsOn(s, "statusseen")
		status := "OFF"
		if current {
			status = "ON"
		}
		s.Reply(info, "*🔰 AUTO STATUS SEEN INFO 🔰*\n\n*TYPE ❰ "+prefix+"STATUSSEEN ON ❱*\n*WHEN ANYONE POST THEIR STATUS BOT WILL BE SEEN AUTOMATICALLY*\n\n*TYPE ❰ "+prefix+"STATUSSEEN OFF ❱*\n*TO STOP AUTO SEEN STATUS*\n\n*CURRENT STATUS :❱ "+status+"*")
		return
	}
	arg := strings.ToLower(strings.TrimSpace(args[0]))
	if arg == "on" {
		statusSetOn(s, "statusseen", true)
		s.Reply(info, "*🔰 AUTO STATUS SEEN ACTIVATED 🔰*\n\n*BOT NUMBER WILL NOWS SEEN AUTO EVERYONE STATUSES 🔰*\n")
		return
	}
	if arg == "off" {
		statusSetOn(s, "statusseen", false)
		s.Reply(info, "*🔰 AUTO STATUS SEEN DE-ACTIVATED 🔰*\n\n*BOT WILL NOT AUTO STATUSES ANYMORE 🔰*\n")
		return
	}
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ STATUSSEEN ❱ FOR HELP*")
}

// ── .statusreact ────────────────────────────────────────────────────────────
// Aliases (Node.js): statusreact, reactstatus, sr

func handleStatusReact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleStatusReactAsync(s, info, args, prefix)
}

func handleStatusReactAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// Current actual emoji list (custom or default)
	emojis := getReactEmojis(s)
	isEnabled := statusIsOn(s, "statusreact")

	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		status := "OFF"
		if isEnabled {
			status = "ON"
		}
		s.Reply(info, "*🔰 STATUS REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREACT ON ❱* \n*WHEN SOMEONE UPLOAD THEIR STATUS BOT WILL BE SEEN & REACT ON THEIR STATUSES*\n\n*TYPE ❰ "+prefix+"STATUSREACT OFF ❱* \n*TO STOP AUTO STATUS REACTS*\n\n*TYPE ❰ "+prefix+"STATUSREACT EMOJI 🔰,🔰,🔰 ❱*\n*SET YOUR OWN EMOJIES*\n\n*TYPE ❰ "+prefix+"STATUSREACT RESET ❱*\n*DEFOULT EMOJIES BACK*\n\n*CURRENT STATUS :❱ "+status+"*\n\n*CURRUNT EMOJIS*\n*"+strings.Join(emojis, ", ")+"*\n\n*TOTAL EMOJIES :❱ ❰ "+itoa(len(emojis))+" ❱*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		// CONFLICT CHECK: statusseen must be ON first (same as Node.js)
		if !statusIsOn(s, "statusseen") {
			s.Reply(info, "*🔰 AUTO STATUS SEEN IS OFF 🔰*\n\n*TYPE FIRST: STATUSSEEN ON*\n*THEN TYPE: STATUSREACT ON*")
			return
		}
		statusSetOn(s, "statusreact", true)
		s.Reply(info, "*🔰 STATUS REACT ACTIVATED 🔰*\n\n*BOT WILL NOW AUTO REACT ON STATUSES*\n*EMOJIS :❱ "+strings.Join(emojis, ", ")+"*")
		return
	}

	if subCmd == "off" {
		statusSetOn(s, "statusreact", false)
		s.Reply(info, "*🔰 STATUS REACT DE-ACTIVATED 🔰*\n\n*BOT WILL NOT REACT ON STATUSES ANYMORE*")
		return
	}

	// statusreact emoji <list> — set custom emojis
	if subCmd == "emoji" {
		rawAfterEmoji := ""
		if len(args) > 1 {
			rawAfterEmoji = strings.Join(args[1:], " ")
		}
		rawAfterEmoji = strings.TrimSpace(rawAfterEmoji)
		if rawAfterEmoji == "" {
			s.Reply(info, "*🔰 STATUS REACT EMOJI 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREACT EMOJI 🔰, 🔰, 🔰 ❱*\n\n*SET AS MANY EMOJIS AS YOU WANT, IT'S YOUR CHOICE BUT YOU MUST PUT A COMMA ❰ , ❱ AFTER EVERY EMOJI OTHERWISE YOUR NEW EMOJIS WON'T BE SET 🔰*")
			return
		}
		// Split by comma or whitespace, remove empty
		var newEmojis []string
		for _, e := range strings.FieldsFunc(rawAfterEmoji, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n'
		}) {
			e = strings.TrimSpace(e)
			if e != "" {
				newEmojis = append(newEmojis, e)
			}
		}
		if len(newEmojis) == 0 {
			s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ STATUSREACT ❱ FOR HELP*")
			return
		}
		s.SetStatusSetting("statusreactemojis", strings.Join(newEmojis, ","))
		s.Reply(info, "*🔰 STATUS REACT EMOJIS UPDATED 🔰*\n\n*NEW EMOJIES*\n "+strings.Join(newEmojis, ", ")+"\n \n*TOTAL EMOJIES :❱ ❰ "+itoa(len(newEmojis))+" ❱*")
		return
	}

	// statusreact reset — back to default emojis
	if subCmd == "reset" {
		s.SetStatusSetting("statusreactemojis", strings.Join(defaultStatusReactEmojis, ","))
		s.Reply(info, "*🔰 STATUS REACT EMOJIS RESET TO DEFAULT 🔰*\n\n*CURRUNT EMOJIS*\n "+strings.Join(defaultStatusReactEmojis, ", ")+"\n \n*TOTAL EMOJIES :❱ ❰ "+itoa(len(defaultStatusReactEmojis))+" ❱*")
		return
	}

	// Unknown subcommand → help menu (same as Node.js)
	s.Reply(info, "*🔰 STATUS REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREACT ON ❱*               ❰ Activate ❱\n*TYPE ❰ "+prefix+"STATUSREACT OFF ❱*              ❰ Stop ❱\n*TYPE ❰ "+prefix+"STATUSREACT EMOJI 🔰,🔰,🔰 ❱*    ❰ Set custom emojis ❱\n*TYPE ❰ "+prefix+"STATUSREACT RESET ❱*            ❰ Default emojis pe wapis ❱")
}

// ── .statusreply ────────────────────────────────────────────────────────────
// Aliases (Node.js): statusreply, replystatus, autoreplystatus, autostatusreply

func handleStatusReply(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleStatusReplyAsync(s, info, args, prefix)
}

func handleStatusReplyAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	// Current actual reply message (custom or default)
	currentMessage := getReplyMessage(s)
	isEnabled := statusIsOn(s, "statusreply")

	// No args → show info (same text as Node.js)
	if len(args) == 0 {
		status := "🔰 OFF"
		if isEnabled {
			status = "🔰 ON"
		}
		s.Reply(info, "*🔰 STATUS REPLY INFO 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREPLY ON ❱*\n*WHEN SOMEONE UPLOAD THEIR STATUS BOT WILL BE REPLY THEIR STATUS AUTOMATICALLY*\n\n*TYPE ❰ "+prefix+"STATUSREPLY OFF ❱*\n*TO STOP AUTO REPLYING STATUS*\n\n*TYPE ❰ "+prefix+"STATUSREPLY MESSAGE <text> ❱*      ❰ SET CUSTOM MSG ❱\n*TYPE ❰ "+prefix+"STATUSREPLY RESET ❱*                ❰ DEFAULT MSG PE WAPIS ❱\n\n*CURRENT STATUS :❱ "+status+"*\n*MESSAGE :❱ "+currentMessage+"*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		// CONFLICT CHECK: statusseen must be ON first (same as Node.js)
		if !statusIsOn(s, "statusseen") {
			s.Reply(info, "*🔰 AUTO STATUS SEEN IS OFF 🔰*\n\n*TYPE FIRST: STATUSSEEN ON*\n*THEN TYPE: STATUSREPLY ON*")
			return
		}
		statusSetOn(s, "statusreply", true)
		s.Reply(info, "*🔰 STATUS REPLY ACTIVATED*\n\n*BOT WILL NOW REPLY ON STATUSES*\n*MESSAGE :❱ "+currentMessage+"*")
		return
	}

	if subCmd == "off" {
		statusSetOn(s, "statusreply", false)
		s.Reply(info, "*🔰 STATUS REPLY DE-ACTIVATED*\n\n*BOT WILL NOT REPLY ON STATUSES ANYMORE*")
		return
	}

	// statusreply message <text> (also msg/set) — set custom message
	if subCmd == "message" || subCmd == "msg" || subCmd == "set" {
		newMessage := ""
		if len(args) > 1 {
			newMessage = strings.Join(args[1:], " ")
		}
		newMessage = strings.TrimSpace(newMessage)
		if newMessage == "" {
			s.Reply(info, "*🔰 STATUS REPLY MESSAGE 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREPLY MESSAGE Mashallah bohat acha status hai! ❱*\n\n*Jo bhi text doge wahi bot status pe UmarReply karega aur DM bhi karega.*")
			return
		}
		s.SetStatusSetting("statusreplymessage", newMessage)
		s.Reply(info, "*🔰 STATUS REPLY MESSAGE UPDATED*\n\n*NEW MESSAGE :❱ "+newMessage+"*")
		return
	}

	// statusreply reset — back to default message
	if subCmd == "reset" {
		s.SetStatusSetting("statusreplymessage", defaultStatusReplyMessage)
		s.Reply(info, "*🔰 STATUS REPLY MESSAGE RESET TO DEFAULT*\n\n*MESSAGE :❱ "+defaultStatusReplyMessage+"*")
		return
	}

	// Unknown subcommand → help menu (same as Node.js)
	s.Reply(info, "*🔰 STATUS REPLY INFO 🔰*\n\n*TYPE ❰ "+prefix+"STATUSREPLY ON ❱*                  ❰ Activate ❱\n*TYPE ❰ "+prefix+"STATUSREPLY OFF ❱*                 ❰ Stop ❱\n*TYPE ❰ "+prefix+"STATUSREPLY MESSAGE <text> ❱*      ❰ Set custom message ❱\n*TYPE ❰ "+prefix+"STATUSREPLY RESET ❱*               ❰ Default message pe wapis ❱")
}

// ── small helpers ───────────────────────────────────────────────────────────

// itoa converts int to string without importing strconv (keeps imports minimal).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// randEmoji picks a random emoji from the list (used by handler.go auto-react).
// Exported so handler.go can call it via the goldcmds package.
func randEmoji(emojis []string) string {
	if len(emojis) == 0 {
		return "🔰"
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return emojis[r.Intn(len(emojis))]
}

// ── exported helpers for handler.go auto-status logic ───────────────────────

// StatusSeenIsOn returns true if auto-status-seen is enabled for this bot.
func StatusSeenIsOn(s SessionBridge) bool {
	return statusIsOn(s, "statusseen")
}

// StatusReactIsOn returns true if auto-status-react is enabled.
func StatusReactIsOn(s SessionBridge) bool {
	return statusIsOn(s, "statusreact")
}

// StatusReplyIsOn returns true if auto-status-reply is enabled.
func StatusReplyIsOn(s SessionBridge) bool {
	return statusIsOn(s, "statusreply")
}

// StatusReactEmojis returns the current emoji list for auto-react.
func StatusReactEmojis(s SessionBridge) []string {
	return getReactEmojis(s)
}

// StatusReplyText returns the current reply message for auto-reply.
func StatusReplyText(s SessionBridge) string {
	return getReplyMessage(s)
}

// PickRandomEmoji picks a random emoji from the list (exported wrapper).
func PickRandomEmoji(emojis []string) string {
	return randEmoji(emojis)
}

// ── registration ────────────────────────────────────────────────────────────

func init() {
	// ── primary commands ──
	Register(Command{Name: "statusseen", Category: "PRESENCE & STATUS", Desc: "Auto-view everyone's status", OwnerOnly: true, Run: handleStatusSeen})
	Register(Command{Name: "statusreact", Category: "PRESENCE & STATUS", Desc: "Auto-react on statuses with emojis", OwnerOnly: true, Run: handleStatusReact})
	Register(Command{Name: "statusreply", Category: "PRESENCE & STATUS", Desc: "Auto-reply to statuses", OwnerOnly: true, Run: handleStatusReply})

	// ── statusseen aliases (Node.js) ──
	Register(Command{Name: "seenstatus", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "autostatusseen", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "autoseenstatus", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "autoviewstatus", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "autostatusview", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "viewstatus", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})
	Register(Command{Name: "statusview", OwnerOnly: true, Hidden: true, Run: handleStatusSeen})

	// ── statusreact aliases (Node.js) ──
	Register(Command{Name: "reactstatus", OwnerOnly: true, Hidden: true, Run: handleStatusReact})
	Register(Command{Name: "sr", OwnerOnly: true, Hidden: true, Run: handleStatusReact})

	// ── statusreply aliases (Node.js) ──
	Register(Command{Name: "replystatus", OwnerOnly: true, Hidden: true, Run: handleStatusReply})
	Register(Command{Name: "autoreplystatus", OwnerOnly: true, Hidden: true, Run: handleStatusReply})
	Register(Command{Name: "autostatusreply", OwnerOnly: true, Hidden: true, Run: handleStatusReply})
}
