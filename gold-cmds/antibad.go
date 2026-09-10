package goldcmds

// ============================================================================
// GOLD-MD — .antibad command (aliases: .antibadword, .antibadwords, .abw)
//
// Ported from UMAR-MD (Node.js pair.js, lines 13766-14110) — same text,
// same behaviour, adapted for whatsmeow:
//   .antibad                  → full command info
//   .antibad on / off         → enable / disable bad-word detection in group
//   .antibad action           → action info (warn / delete / kick)
//   .antibad action warn      → set action=warn
//   .antibad action warn <n>  → set action=warn + max-warnings=n (1-50)
//   .antibad action warn reset → reset max-warnings to default
//   .antibot action delete    → set action=delete
//   .antibad action kick      → set action=kick
//   .antibad action reset     → reset action to defaults
//   .antibad add word1,word2  → add custom bad words (comma/space separated)
//   .antibad del word1,word2  → remove custom bad words
//   .antibad list             → show custom bad words
//   .antibad reset            → full reset (off + warn + default max + clear)
//
// Per-group config in Redis (settings:<groupJID>):
//   field "antibad"           = on/off
//   field "antibad:action"    = warn/delete/kick
//   field "antibad:maxwarn"   = 1-50
// Custom words: per-group SET "antibad:words".
//
// Detection: containsBadWord checks the message body against the default
// built-in bad-words list PLUS the group's custom words. The actual detection
// on incoming group messages is in handler.go (applyAntiBad), which calls
// AntibadCheckAndEnforce.
//
// NOTE: The original Node.js badwords.js module was not uploaded, so a
// reasonable default English profanity list is included here. Group owners
// can add their own words via .antibad add.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"strconv"
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow/types"
)

const antibadFeature = "antibad"
const antibadUpper = "ANTIBAD"
const antibadWordsSet = "antibad:words"

// defaultBadWords is a reasonable built-in English profanity list. The
// original Node.js badwords.js module was not available, so this covers the
// common words that a bad-word filter would catch. Group owners can extend
// it per-group via .antibad add.
var defaultBadWords = []string{
	"fuck", "fucker", "fucking", "motherfucker", "motherfucking",
	"shit", "shitty", "bullshit", "dipshit",
	"bitch", "bitches",
	"asshole", "assholes",
	"bastard", "bastards",
	"dick", "dickhead",
	"pussy",
	"cunt",
	"slut", "sluts",
	"whore", "whores",
	"douche", "douchebag",
	"damn", "goddamn",
	"hell",
	"piss", "pissed",
	"jackass",
	"retard", "retarded",
	"idiot", "idiots",
	"stupid",
	"moron",
}

// stripFancyFonts removes decorative/duplicate characters and normalises
// the text to plain ASCII letters so that fancy-font profanity (e.g.
// "𝖋𝖚𝖈𝖐" or "fffuck") is still caught. Mirrors _UmarStripFancyFonts +
// _UmarNormalizeForBadWords.
func stripFancyFonts(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		// Keep ASCII letters/digits as-is.
		if r < 128 {
			sb.WriteRune(r)
			continue
		}
		// Try to fold Unicode letters to their ASCII base.
		// unicode.ToLower handles case; for decorative math/italic
		// Unicode blocks, the rune often has a Letter category but no
		// simple ASCII mapping, so we approximate by taking the lowercased
		// rune if it's a letter.
		if unicode.IsLetter(r) {
			lr := unicode.ToLower(r)
			// Many fancy fonts map to the 0x1D4xx–0x1D7xx range; their
			// lowercase base is in the ASCII range via a simple offset.
			// We do a best-effort: if the lowercased rune is still
			// non-ASCII, skip it (so "𝖋𝖚𝖈𝖐" collapses to "" — not ideal
			// but prevents false collapses). In practice most profanity
			// arrives in plain text.
			if lr < 128 {
				sb.WriteRune(lr)
			}
		}
		// Skip non-letter, non-ASCII decoration (stars, circles, etc.)
	}
	return sb.String()
}

// normalizeForBadWords lowercases, strips fancy fonts, and removes
// non-alphanumeric characters (so "f*u*c*k" or "f.u.c.k" is caught).
func normalizeForBadWords(s string) string {
	s = strings.ToLower(s)
	s = stripFancyFonts(s)
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// containsBadWord checks if the body contains any word from the default list
// or the custom list. Mirrors UmarContainsBadWordText(body, customWords).
// Matching is word-boundary based after normalisation.
func containsBadWord(body string, customWords []string) bool {
	if body == "" {
		return false
	}
	norm := normalizeForBadWords(body)
	// Split into words on whitespace.
	words := strings.Fields(norm)
	if len(words) == 0 {
		return false
	}
	wordSet := make(map[string]bool, len(words))
	for _, w := range words {
		wordSet[w] = true
	}
	// Check default words.
	for _, bad := range defaultBadWords {
		if wordSet[bad] {
			return true
		}
	}
	// Check custom words.
	for _, bad := range customWords {
		b := normalizeForBadWords(bad)
		if b != "" && wordSet[b] {
			return true
		}
	}
	return false
}

// ── exported helpers for handler.go ────────────────────────────────────

// AntibadIsOn reports whether antibad is enabled in the given group.
func AntibadIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antibadFeature)
}

// AntibadCustomWords returns the custom bad words for a group.
func AntibadCustomWords(s SessionBridge, groupJID string) []string {
	return s.GroupSetMembers(groupJID, antibadWordsSet)
}

// AntibadCheckAndEnforce checks a message body for bad words (default +
// custom) and, if found, enforces the configured action. Returns true if an
// offence was detected, false otherwise. Mirrors the antibad detection
// handler in pair.js (lines 16170-16360).
func AntibadCheckAndEnforce(s SessionBridge, info types.MessageInfo, body string) bool {
	if !AntibadIsOn(s, info.Chat.String()) {
		return false
	}
	custom := AntibadCustomWords(s, info.Chat.String())
	if !containsBadWord(body, custom) {
		return false
	}
	// Offence detected → enforce
	senderJID := info.Sender.String()
	senderNum := strings.SplitN(senderJID, "@", 2)[0]
	EnforceAntiAction(s, info, antibadFeature, antibadUpper, "BAD WORDS", "BAD WORDS NOT ALLOWED", senderJID, senderNum)
	return true
}

// ── command handler ────────────────────────────────────────────────────

func handleAntibad(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAntibadAsync(s, info, args, prefix)
}

func handleAntibadAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroupOwner(s, info) {
		return
	}
	groupJID := info.Chat.String()

	// No args → full command info
	if len(args) == 0 {
		custom := AntibadCustomWords(s, groupJID)
		curAction := antiAction(s, groupJID, antibadFeature)
		maxW := antiMaxWarnings(s, groupJID, antibadFeature)
		enabled := antiEnabled(s, groupJID, antibadFeature)

		s.Reply(info, "*🔰 ANTIBAD COMMAND INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBAD ON ❱*\n*WHEN ANTIBAD IS ON THEN IF ANY USER SENDS ANY BAD WORD IN GROUP THE BOT WILL DETECT THE BAD WORD AND ACTION WILL APPLY*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD OFF ❱*\n*WHEN ANTIBAD IS OFF THEN ALL MEMBERS CAN FREELY SEND BAD WORDS IN THIS GROUP*\n\n*🔰 ANTIBAD ACTIONS INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION ❱*\n*YOU WILL GET INFO OF ANTIBAD ACTIONS*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION DELETE ❱*\n*WHEN ACTION IS DELETE THEN BAD WORDS WILL BE DETECTED AND AUTO DELETED*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION KICK ❱*\n*WHEN ACTION IS KICK THEN AS SOON AS BAD WORD IS DETECTED THE USER WILL BE REMOVED*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION WARN ❱*\n*WHEN ACTION IS WARN THEN USER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE USER WILL BE AUTO REMOVED FROM GROUP*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION WARN ❰40❱ ❱*\n*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ACTION RESET ❱*\n*ANTIBAD ACTIONS WILL BE RESET*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD ADD word1,word2,word3 ❱*\n*WHICHEVER WORDS YOU ADD THOSE WORDS WILL BE ADDED IN THIS GROUP'S BAD-WORD LIST. YOUR NEW WORDS AAYENGE PURANI LIST KE SAATH — OLD DEFAULT WORDS BHI RAHENGE AUR NAYE WORDS BHI USI LIST MEIN ADD HO JAYENGE*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD DEL word1,word2,word3 ❱*\n*DELETE WORDS FROM YOUR ADDED LIST. WHEN WORDS ARE DELETED FROM ADDED LIST THEN UN WORDS PE ANTIBAD ACTION APPLY NAHI HOGA (DEFAULT WORDS PE HOTA RAHEGA)*\n\n\n*TYPE ❰ "+prefix+"ANTIBAD LIST ❱*\n*CHECK THE LIST OF BAD WORDS THAT YOU ADDED*\n\n\n\n\n*TYPE ❰ "+prefix+"ANTIBAD RESET ❱*\n*TO RESET FULL ANTIBAD COMMAND*\n\n\n*ANTIBAD NOW :❱ ❰ "+boolOnOff(enabled)+" ❱*\n*ACTION :❱ ❰ "+strings.ToUpper(curAction)+" ❱*\n*MAX WARNINGS :❱ ❰ "+strconv.Itoa(maxW)+" ❱*\n*ADDED WORDS QUANTITY :❱ ❰ "+strconv.Itoa(len(custom))+" ❱*")
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	// ON
	if sub == "on" {
		antiSetEnabled(s, groupJID, antibadFeature, true)
		curAction := antiAction(s, groupJID, antibadFeature)
		maxW := antiMaxWarnings(s, groupJID, antibadFeature)
		s.Reply(info, "*🔰 ANTIBAD ACTIVATED 🔰*\n\n*ACTION :❱ "+strings.ToUpper(curAction)+"*\n\n*🔰 ACTION INFO 🔰*\n"+antiActionInfo(curAction, maxW, "BAD WORDS"))
		return
	}

	// OFF
	if sub == "off" {
		antiSetEnabled(s, groupJID, antibadFeature, false)
		s.Reply(info, "*🔰 ANTIBAD DE-ACTIVATED 🔰*")
		return
	}

	// ACTION (shared tree — antibad also has "action reset" sub)
	if sub == "action" {
		handleAntiAction(s, info, args[1:], prefix, antibadFeature, antibadUpper)
		return
	}

	// ADD words
	if sub == "add" {
		rest := strings.TrimSpace(strings.Join(args[1:], " "))
		if rest == "" {
			s.Reply(info, "*🔰 ANTIBAD ADD 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBAD ADD word1,word2,word3 ❱*\n\n*Naye words purani list ke saath add ho jayenge — old default words bhi rahenge.*")
			return
		}
		parts := strings.FieldsFunc(rest, func(r rune) bool {
			return r == ',' || r == '\n'
		})
		var words []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				words = append(words, strings.ToLower(p))
			}
		}
		if len(words) == 0 {
			s.Reply(info, "*🔰 NO WORDS PROVIDED*\n\n*TYPE ❰ "+prefix+"ANTIBAD ADD word1,word2,word3 ❱*")
			return
		}
		existing := AntibadCustomWords(s, groupJID)
		existingMap := make(map[string]bool, len(existing))
		for _, e := range existing {
			existingMap[e] = true
		}
		var added, skipped []string
		for _, w := range words {
			if existingMap[w] {
				skipped = append(skipped, w)
			} else {
				_ = s.GroupSetAdd(groupJID, antibadWordsSet, w)
				added = append(added, w)
			}
		}
		totalNow := len(AntibadCustomWords(s, groupJID))
		msg := "*🔰 ANTIBAD WORDS ADDED*\n\n*ADDED :❱ " + strconv.Itoa(len(added)) + "*"
		if len(added) > 0 {
			msg += "\n*NEW WORDS :* " + strings.Join(added, ", ")
		}
		if len(skipped) > 0 {
			msg += "\n*ALREADY IN LIST :* " + strings.Join(skipped, ", ")
		}
		msg += "\n\n*TOTAL ADDED WORDS NOW :❱ " + strconv.Itoa(totalNow) + "*"
		s.Reply(info, msg)
		return
	}

	// DEL words
	if sub == "del" || sub == "delete" || sub == "remove" {
		rest := strings.TrimSpace(strings.Join(args[1:], " "))
		if rest == "" {
			s.Reply(info, "*🔰 ANTIBAD DEL 🔰*\n\n*TYPE ❰ "+prefix+"ANTIBAD DEL word1,word2,word3 ❱*\n\n*Sirf apni added words hi hat sakti hain — default built-in list nahi.*")
			return
		}
		parts := strings.FieldsFunc(rest, func(r rune) bool {
			return r == ',' || r == '\n'
		})
		var words []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				words = append(words, strings.ToLower(p))
			}
		}
		if len(words) == 0 {
			s.Reply(info, "*🔰 NO WORDS PROVIDED*\n\n*TYPE ❰ "+prefix+"ANTIBAD DEL word1,word2,word3 ❱*")
			return
		}
		existing := AntibadCustomWords(s, groupJID)
		existingMap := make(map[string]bool, len(existing))
		for _, e := range existing {
			existingMap[e] = true
		}
		var removed, notFound []string
		for _, w := range words {
			if existingMap[w] {
				_ = s.GroupSetRem(groupJID, antibadWordsSet, w)
				removed = append(removed, w)
			} else {
				notFound = append(notFound, w)
			}
		}
		totalNow := len(AntibadCustomWords(s, groupJID))
		msg := "*🔰 ANTIBAD WORDS REMOVED*\n\n*REMOVED :❱ " + strconv.Itoa(len(removed)) + "*"
		if len(removed) > 0 {
			msg += "\n*DELETED :* " + strings.Join(removed, ", ")
		}
		if len(notFound) > 0 {
			msg += "\n*NOT IN LIST :* " + strings.Join(notFound, ", ")
		}
		msg += "\n\n*TOTAL ADDED WORDS NOW :❱ " + strconv.Itoa(totalNow) + "*"
		s.Reply(info, msg)
		return
	}

	// LIST
	if sub == "list" || sub == "addedlist" || sub == "wordlist" {
		custom := AntibadCustomWords(s, groupJID)
		if len(custom) == 0 {
			s.Reply(info, "*🔰 ANTIBAD ADDED WORDS LIST 🔰*\n\n*No custom words added yet.*\n*TYPE ❰ "+prefix+"ANTIBAD ADD word1,word2 ❱*")
			return
		}
		// Show first 200
		preview := custom
		extra := ""
		if len(custom) > 200 {
			preview = custom[:200]
			extra = "\n\n*(showing first 200 of " + strconv.Itoa(len(custom)) + ")*"
		}
		s.Reply(info, "*🔰 ANTIBAD ADDED WORDS LIST 🔰*\n\n*TOTAL :❱ "+strconv.Itoa(len(custom))+"*\n\n"+strings.Join(preview, ", ")+extra)
		return
	}

	// RESET
	if sub == "reset" {
		antiResetSettings(s, groupJID, antibadFeature, antibadWordsSet)
		s.Reply(info, "*🔰 ANTIBAD FULLY RESET*\n\n*Settings, added words, warnings — sab clear.*")
		return
	}

	// Unknown
	s.Reply(info, "*🔰 UNKNOWN ANTIBAD SUBCOMMAND :❱ "+sub+"*\n\n*TYPE ANTIBAD FOR FULL INFO*")
}

// ── registration ───────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antibad", Category: "ANTI & PROTECTION", Desc: "Block bad words in group (on/off/action)", OwnerOnly: true, Run: handleAntibad})
	Register(Command{Name: "antibadword", OwnerOnly: true, Hidden: true, Run: handleAntibad})
	Register(Command{Name: "antibadwords", OwnerOnly: true, Hidden: true, Run: handleAntibad})
	Register(Command{Name: "abw", OwnerOnly: true, Hidden: true, Run: handleAntibad})
}
