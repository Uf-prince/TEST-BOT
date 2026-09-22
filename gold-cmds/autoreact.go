package goldcmds

// ============================================================================
// GOLD-MD — Auto-react commands: .autoreact and .ownerreact
//
// Ported from UMAR-MD (Node.js pair.js) — same text style, same behaviour:
//   .autoreact on/off        → bot auto-reacts on OTHER people's messages
//   .autoreact emoji <list>  → set custom emojis (comma/space separated, max 20)
//   .autoreact reset         → reset to default emojis + smart mode
//
//   .ownerreact on/off       → bot auto-reacts on OWNER/BOT's OWN messages
//   .ownerreact emoji <list> → set custom emojis
//   .ownerreact reset        → reset to default emojis + smart mode
//
// Per-bot config stored in Redis (Upstash) via settings:<botJID> hash:
//   field "autoreact"          = "true"/"false"
//   field "autoreactemojis"    = comma-separated emoji list ("" = default)
//   field "autoreactcustom"    = "true"/"false"  (custom mode vs smart mode)
//   field "ownerreact"         = "true"/"false"
//   field "ownerreactemojis"   = comma-separated emoji list
//   field "ownerreactcustom"   = "true"/"false"
//
// REACTION ENGINE (the actual react on every incoming message) lives in
// handler.go → ApplyAutoReact / ApplyOwnerReact, exactly like pair.js
// lines 10870-10980. This file only contains the CONFIG commands.
//
// SMART MODE (default / after reset):
//   - If the message has exactly 1 emoji → react with that same emoji
//   - If 0 emojis or 2+ emojis → react with fixed ❤️ (NOT random)
// CUSTOM MODE (after user sets their own emoji list):
//   - Pick a random emoji from the user's list on every message
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"math/rand"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── defaults (same as Node.js pair.js line 174) ────────────────────────────
//
// AUTO_REACT_EMOJIS: ['😊','❤️','🔥','💯','😎','👑','🤗','😘','💚','🫶','😄','🥰','🥳','🚩','💥','🎊']
// NOTE: the 👑 in the Node.js default list is kept as-is here because this is
// the BOT'S OWN reaction emoji pool (what the bot reacts WITH), not user-facing
// command text. The user asked to replace 👑 with 🔰 only in user-facing text.
var defaultAutoReactEmojis = []string{"😊", "❤️", "🔥", "💯", "😎", "👑", "🤗", "😘", "💚", "🫶", "😄", "🥰", "🥳", "🚩", "💥", "🎊"}

// ── emoji detection ────────────────────────────────────────────────────────
//
// Node.js uses /\p{Emoji_Presentation}|\p{Extended_Pictographic}/gu to find
// emojis in a message. Go's standard `regexp` package does NOT support Unicode
// property escapes (\p{Emoji_Presentation} etc.), so we detect emojis by
// scanning runes and checking whether each rune is an emoji-class code point.
// This matches the same set (Emoji_Presentation + Extended_Pictographic).
//
// We use the Unicode ranges that correspond to these properties. The list
// below covers the common emoji blocks used in WhatsApp messages.
var emojiRanges = []runeRange{
	{0x1F300, 0x1F5FF}, // Misc Symbols & Pictographs
	{0x1F600, 0x1F64F}, // Emoticons
	{0x1F680, 0x1F6FF}, // Transport & Map
	{0x1F700, 0x1F77F}, // Alchemical
	{0x1F780, 0x1F7FF}, // Geometric Shapes Ext
	{0x1F800, 0x1F8FF}, // Supplemental Arrows-C
	{0x1F900, 0x1F9FF}, // Supplemental Symbols & Pictographs
	{0x1FA00, 0x1FA6F}, // Chess Symbols
	{0x1FA70, 0x1FAFF}, // Symbols & Pictographs Ext-A
	{0x1FB00, 0x1FBFF}, // Symbols for Legacy Computing
	{0x2600, 0x26FF},   // Misc Symbols (☀️ ☔️ etc.)
	{0x2700, 0x27BF},   // Dingbats (✂️ ✈️ etc.)
	{0x2300, 0x23FF},   // Misc Technical (⌚ ⌛ etc.)
	{0x2B00, 0x2BFF},   // Misc Symbols & Arrows (⬛ ⬜ etc.)
	{0xFE0F, 0xFE0F},   // Variation Selector-16 (emoji presentation)
}

type runeRange struct{ lo, hi rune }

// isEmojiRune reports whether r is an emoji-class code point.
func isEmojiRune(r rune) bool {
	for _, rng := range emojiRanges {
		if r >= rng.lo && r <= rng.hi {
			return true
		}
	}
	return false
}

// countEmojis returns the list of emojis found in s (each emoji as a string).
// Skips the Variation Selector-16 (0xFE0F) as a standalone entry — it only
// modifies the preceding base character's presentation. This mirrors the
// Node.js regex match behaviour for "how many emojis are in this message".
func countEmojis(s string) []string {
	var out []string
	for _, r := range s {
		if r == 0xFE0F {
			// Variation selector — skip as a standalone; it's part of the
			// preceding emoji (e.g. ❤️ = U+2764 + U+FE0F).
			continue
		}
		if isEmojiRune(r) {
			out = append(out, string(r))
		}
	}
	return out
}

// ── helpers ────────────────────────────────────────────────────────────────

// reactIsOn reads an auto-react boolean setting from Redis (per-bot).
func reactIsOn(s SessionBridge, field string) bool {
	v := s.GetStatusSetting(field, "false")
	return v == "true" || v == "1" || v == "on"
}

// reactSetOn writes an auto-react boolean setting to Redis (per-bot).
func reactSetOn(s SessionBridge, field string, on bool) {
	if on {
		s.SetStatusSetting(field, "true")
	} else {
		s.SetStatusSetting(field, "false")
	}
}

// reactCustomMode returns true if the feature is in CUSTOM mode (user set
// their own emoji list). In custom mode every message gets a random emoji
// from the list. In SMART mode (default), 1-emoji messages get that same
// emoji and 0/2+ emoji messages get a fixed ❤️.
func reactCustomMode(s SessionBridge, field string) bool {
	v := s.GetStatusSetting(field, "false")
	return v == "true" || v == "1"
}

// reactEmojis returns the current emoji list (custom or default).
func reactEmojis(s SessionBridge, emojisField string) []string {
	v := s.GetStatusSetting(emojisField, "")
	if v == "" {
		return defaultAutoReactEmojis
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
		return defaultAutoReactEmojis
	}
	return out
}

// pickRandomEmoji picks a random emoji from the list.
func pickRandomEmoji(emojis []string) string {
	if len(emojis) == 0 {
		return "❤️"
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return emojis[r.Intn(len(emojis))]
}

// smartReactEmoji implements the SMART MODE logic:
//   - exactly 1 emoji in the message body → react with that emoji
//   - 0 emojis or 2+ emojis → react with fixed ❤️ (NOT random)
func smartReactEmoji(body string) string {
	found := countEmojis(body)
	if len(found) == 1 {
		return found[0]
	}
	return "❤️"
}

// ── .autoreact ─────────────────────────────────────────────────────────────
// Aliases (Node.js): autoreact, ar, reactauto

func handleAutoReact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutoReactAsync(s, info, args, prefix)
}

func handleAutoReactAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	isEnabled := reactIsOn(s, "autoreact")
	currentEmojis := reactEmojis(s, "autoreactemojis")

	// No args → show info (same text as Node.js pair.js lines 15394-15410)
	if len(args) == 0 {
		status := "❌ OFF"
		if isEnabled {
			status = "✅ ON"
		}
		s.Reply(info, "*🔰 AUTO REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"AUTOREACT ON ❱*\n*TO ACTIVATE AUTO REACT ON ALL MESSAGES*\n\n*TYPE ❰ "+prefix+"AUTOREACT OFF ❱*\n*TO STOP AUTO REACT*\n\n*TYPE ❰ "+prefix+"AUTOREACT EMOJI 😂,❤️,🔥,😎,🔰 ❱*\n*SET YOUR OWN EMOJIES (MAX 20)*\n\n*TYPE ❰ "+prefix+"AUTOREACT RESET ❱*\n*DEFAULT EMOJIES AND SETTINGS BACK*\n\n*CURRENT STATUS :❱ "+status+"*\n*CURRENT EMOJIS :❱ "+strings.Join(currentEmojis, ", ")+"*\n*TOTAL EMOJIS :❱ ❰ "+itoa(len(currentEmojis))+" ❱*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		reactSetOn(s, "autoreact", true)
		s.Reply(info, "*🔰 AUTO REACT ACTIVATED 🔰*\n\n*BOT WILL NOW AUTO REACT ON MESSAGES*\n*EMOJIS :❱ "+strings.Join(currentEmojis, ", ")+"*")
		return
	}

	if subCmd == "off" {
		reactSetOn(s, "autoreact", false)
		s.Reply(info, "*🔰 AUTO REACT DE-ACTIVATED 🔰*\n\n*BOT WILL NOT REACT ON MESSAGES ANYMORE*")
		return
	}

	// autoreact emoji <list> — set custom emojis (max 20), switch to CUSTOM mode
	if subCmd == "emoji" {
		rawAfterEmoji := ""
		if len(args) > 1 {
			rawAfterEmoji = strings.Join(args[1:], " ")
		}
		rawAfterEmoji = strings.TrimSpace(rawAfterEmoji)
		if rawAfterEmoji == "" {
			s.Reply(info, "*🔰 AUTO REACT EMOJI 🔰*\n\n*TYPE ❰ "+prefix+"AUTOREACT EMOJI 😂,❤️,🔥,😎,🔰 ❱*\n\n*SET AS MANY EMOJIS AS YOU WANT (MAX 20)*\n*PUT COMMA ❰ , ❱ AFTER EVERY EMOJI*\n*RANDOM EMOJI WILL BE SELECTED FROM THIS LIST*")
			return
		}
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
			s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ AUTOREACT ❱ FOR HELP*")
			return
		}
		// Max 20 emojis (same as Node.js .slice(0, 20))
		if len(newEmojis) > 20 {
			newEmojis = newEmojis[:20]
		}
		s.SetStatusSetting("autoreactemojis", strings.Join(newEmojis, ","))
		s.SetStatusSetting("autoreactcustom", "true") // switch to CUSTOM mode
		s.Reply(info, "*🔰 AUTO REACT EMOJIS UPDATED 🔰*\n\n*NEW EMOJIS*\n "+strings.Join(newEmojis, ", ")+" \n \n*TOTAL EMOJIS :❱ ❰ "+itoa(len(newEmojis))+" ❱*\n*MODE :❱ CUSTOM (SMART ❤️ DETECTION OFF)*\n*EVERY MESSAGE WILL GET RANDOM EMOJI FROM YOUR LIST*")
		return
	}

	// autoreact reset — back to default emojis + smart mode, status ON
	if subCmd == "reset" {
		s.SetStatusSetting("autoreactemojis", "")
		s.SetStatusSetting("autoreactcustom", "false") // back to SMART mode
		reactSetOn(s, "autoreact", true)               // Node.js reset → status ON
		s.Reply(info, "*🔰 AUTO REACT RESET TO DEFAULT 🔰*\n\n*STATUS :❱ ON*\n*MODE :❱ SMART (1 EMOJI = SAME, ELSE ❤️)*\n*EMOJIS :❱ "+strings.Join(defaultAutoReactEmojis, ", ")+"*\n \n*TOTAL EMOJIS :❱ ❰ "+itoa(len(defaultAutoReactEmojis))+" ❱*")
		return
	}

	// Unknown subcommand → help menu (same as Node.js pair.js lines 15486-15491)
	s.Reply(info, "*🔰 AUTO REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"AUTOREACT ON ❱*               ❰ Activate ❱\n*TYPE ❰ "+prefix+"AUTOREACT OFF ❱*              ❰ Stop ❱\n*TYPE ❰ "+prefix+"AUTOREACT EMOJI 😂,❤️,🔥 ❱*    ❰ Set custom emojis ❱\n*TYPE ❰ "+prefix+"AUTOREACT RESET ❱*            ❰ Default settings wapas ❱")
}

// ── .ownerreact ────────────────────────────────────────────────────────────
// Aliases (Node.js): ownerreact, or, reactowner

func handleOwnerReact(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleOwnerReactAsync(s, info, args, prefix)
}

func handleOwnerReactAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}
	isEnabled := reactIsOn(s, "ownerreact")
	currentEmojis := reactEmojis(s, "ownerreactemojis")

	// No args → show info (same text as Node.js pair.js lines 15554-15570)
	if len(args) == 0 {
		status := "❌ OFF"
		if isEnabled {
			status = "✅ ON"
		}
		s.Reply(info, "*🔰 OWNER REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"OWNERREACT ON ❱*\n*TO ACTIVATE REACT ON MY (OWNER/BOT) OWN MESSAGES*\n\n*TYPE ❰ "+prefix+"OWNERREACT OFF ❱*\n*TO STOP OWNER REACT*\n\n*TYPE ❰ "+prefix+"OWNERREACT EMOJI 😂,❤️,🔥,😎,🔰 ❱*\n*SET YOUR OWN EMOJIES (MAX 20)*\n\n*TYPE ❰ "+prefix+"OWNERREACT RESET ❱*\n*DEFAULT EMOJIES AND SETTINGS BACK*\n\n*CURRENT STATUS :❱ "+status+"*\n*CURRENT EMOJIS :❱ "+strings.Join(currentEmojis, ", ")+"*\n*TOTAL EMOJIS :❱ ❰ "+itoa(len(currentEmojis))+" ❱*")
		return
	}

	subCmd := strings.ToLower(strings.TrimSpace(args[0]))

	if subCmd == "on" {
		reactSetOn(s, "ownerreact", true)
		s.Reply(info, "*🔰 OWNER REACT ACTIVATED 🔰*\n\n*BOT WILL NOW REACT ON MY (OWNER/BOT) OWN MESSAGES*\n*EMOJIS :❱ "+strings.Join(currentEmojis, ", ")+"*")
		return
	}

	if subCmd == "off" {
		reactSetOn(s, "ownerreact", false)
		s.Reply(info, "*🔰 OWNER REACT DE-ACTIVATED 🔰*\n\n*BOT WILL NOT REACT ON MY OWN MESSAGES ANYMORE*")
		return
	}

	// ownerreact emoji <list> — set custom emojis (max 20), switch to CUSTOM mode
	if subCmd == "emoji" {
		rawAfterEmoji := ""
		if len(args) > 1 {
			rawAfterEmoji = strings.Join(args[1:], " ")
		}
		rawAfterEmoji = strings.TrimSpace(rawAfterEmoji)
		if rawAfterEmoji == "" {
			s.Reply(info, "*🔰 OWNER REACT EMOJI 🔰*\n\n*TYPE ❰ "+prefix+"OWNERREACT EMOJI 😂,❤️,🔥,😎,🔰 ❱*\n\n*SET AS MANY EMOJIS AS YOU WANT (MAX 20)*\n*PUT COMMA ❰ , ❱ AFTER EVERY EMOJI*\n*RANDOM EMOJI WILL BE SELECTED FROM THIS LIST*")
			return
		}
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
			s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ OWNERREACT ❱ FOR HELP*")
			return
		}
		if len(newEmojis) > 20 {
			newEmojis = newEmojis[:20]
		}
		s.SetStatusSetting("ownerreactemojis", strings.Join(newEmojis, ","))
		s.SetStatusSetting("ownerreactcustom", "true") // switch to CUSTOM mode
		s.Reply(info, "*🔰 OWNER REACT EMOJIS UPDATED 🔰*\n\n*NEW EMOJIS*\n "+strings.Join(newEmojis, ", ")+" \n \n*TOTAL EMOJIS :❱ ❰ "+itoa(len(newEmojis))+" ❱*\n*MODE :❱ CUSTOM (SMART ❤️ DETECTION OFF)*\n*EVERY MY MESSAGE WILL GET RANDOM EMOJI FROM YOUR LIST*")
		return
	}

	// ownerreact reset — back to default emojis + smart mode, status OFF
	if subCmd == "reset" {
		s.SetStatusSetting("ownerreactemojis", "")
		s.SetStatusSetting("ownerreactcustom", "false") // back to SMART mode
		reactSetOn(s, "ownerreact", false)              // Node.js reset → status OFF
		s.Reply(info, "*🔰 OWNER REACT RESET TO DEFAULT 🔰*\n\n*STATUS :❱ OFF*\n*MODE :❱ SMART (1 EMOJI = SAME, ELSE ❤️)*\n*EMOJIS :❱ "+strings.Join(defaultAutoReactEmojis, ", ")+"*\n \n*TOTAL EMOJIS :❱ ❰ "+itoa(len(defaultAutoReactEmojis))+" ❱*")
		return
	}

	// Unknown subcommand → help menu (same as Node.js pair.js lines 15646-15651)
	s.Reply(info, "*🔰 OWNER REACT INFO 🔰*\n\n*TYPE ❰ "+prefix+"OWNERREACT ON ❱*               ❰ Activate ❱\n*TYPE ❰ "+prefix+"OWNERREACT OFF ❱*              ❰ Stop ❱\n*TYPE ❰ "+prefix+"OWNERREACT EMOJI 😂,❤️,🔥 ❱*    ❰ Set custom emojis ❱\n*TYPE ❰ "+prefix+"OWNERREACT RESET ❱*            ❰ Default settings wapas ❱")
}

// ── exported helpers for handler.go reaction engine ────────────────────────

// AutoReactIsOn returns true if auto-react (on others' messages) is enabled.
func AutoReactIsOn(s SessionBridge) bool {
	return reactIsOn(s, "autoreact")
}

// OwnerReactIsOn returns true if owner-react (on bot's own messages) is enabled.
func OwnerReactIsOn(s SessionBridge) bool {
	return reactIsOn(s, "ownerreact")
}

// AutoReactCustomMode returns true if auto-react is in CUSTOM mode.
func AutoReactCustomMode(s SessionBridge) bool {
	return reactCustomMode(s, "autoreactcustom")
}

// OwnerReactCustomMode returns true if owner-react is in CUSTOM mode.
func OwnerReactCustomMode(s SessionBridge) bool {
	return reactCustomMode(s, "ownerreactcustom")
}

// AutoReactEmojis returns the current emoji list for auto-react.
func AutoReactEmojis(s SessionBridge) []string {
	return reactEmojis(s, "autoreactemojis")
}

// OwnerReactEmojis returns the current emoji list for owner-react.
func OwnerReactEmojis(s SessionBridge) []string {
	return reactEmojis(s, "ownerreactemojis")
}

// PickAutoReactEmoji decides which emoji to react with, given the mode.
// customMode=true → random from the emoji list.
// customMode=false (smart) → 1 emoji in body = that emoji, else ❤️.
func PickAutoReactEmoji(body string, customMode bool, emojis []string) string {
	if customMode {
		return pickRandomEmoji(emojis)
	}
	return smartReactEmoji(body)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "autoreact", Category: "PRESENCE & STATUS", Desc: "THIS COMMAND IS USED TO TURN ON AUTO REACT ON EVERY MESSAGE.", OwnerOnly: true, Run: handleAutoReact})
	Register(Command{Name: "ownerreact", Category: "PRESENCE & STATUS", Desc: "THIS COMMAND IS USED TO TURN ON AUTO REACT ONLY ON THE OWNER MESSAGES.", OwnerOnly: true, Run: handleOwnerReact})

	// Hidden aliases (same as Node.js)
	Register(Command{Name: "ar", OwnerOnly: true, Hidden: true, Run: handleAutoReact})
	Register(Command{Name: "reactauto", OwnerOnly: true, Hidden: true, Run: handleAutoReact})
	Register(Command{Name: "or", OwnerOnly: true, Hidden: true, Run: handleOwnerReact})
	Register(Command{Name: "reactowner", OwnerOnly: true, Hidden: true, Run: handleOwnerReact})
}
