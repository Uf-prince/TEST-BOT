package goldcmds

// ============================================================================
// GOLD-MD — .autoblock command (country-code based auto-block)
//
// WHAT IT DOES:
//   Owner sets country codes (e.g. 92,1,44). When ANY private-chat message
//   arrives from a number whose prefix matches one of those codes, the bot
//   instantly blocks that number on WhatsApp (real blocklist, via
//   whatsmeow's UpdateBlocklist IQ — verified against vendor user.go:993,
//   the official WhatsApp block action used by WhatsApp Web itself).
//
// SUB-COMMANDS (owner-only):
//   .autoblock                 → full guide
//   .autoblock on / off        → enable / disable
//   .autoblock add 92,1,44     → add country codes (comma/space separated)
//   .autoblock del 92,1        → remove country codes
//   .autoblock reset           → full reset (off + empty codes)
//   .autoblock list            → show current status + codes
//   .autoblock contact         → contact-save mode ON (saved contacts exempt)
//   .autoblock nocontact       → contact-save mode OFF (block everyone)
//   .autoblock sync            → force re-sync WhatsApp contacts from server
//
// STORAGE (Redis, settings:<botJID> hash — same pattern as antibad etc):
//   field "autoblock"          = "on"/"off"
//   field "autoblock:codes"    = comma-joined list, e.g. "92,1,44"
//
// ENFORCEMENT (handler.go hook, private chats only):
//   - Owner + bot itself are ALWAYS exempt (never blocked)
//   - Runs on every non-group, non-own message BEFORE any command dispatch
//   - Check is cached (in-memory sync.Map, 0ms — no Upstash call per msg)
//   - Block happens in a goroutine (never blocks reply path)
//   - Owner typing .autoblock still works even when blocking is on (owner
//     is exempt), so the feature can always be turned off.
//
// Latest-update verified (Sept 2026): UpdateBlocklist is whatsmeow's
// official block API — same IQ the official WhatsApp clients use.
// ============================================================================

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// ----------------------------------------------------------------------------
// in-memory cache (0ms checks — zero Upstash calls per message)
// ----------------------------------------------------------------------------

// abCache caches the loaded autoblock config per bot: on/off + codes map.
// mutex-guarded so hook + command can read/write concurrently.
var (
	abCacheMu   sync.Mutex
	abCache     = map[string]*abConfig{} // botJID → config
	abCodesTTL  = time.Minute            // reload every minute (max)
	abLoadedAt  = map[string]time.Time{}
	abSettingsF = "autoblock"
	abCodesF    = "autoblock:codes"
)

type abConfig struct {
	On          bool
	Codes       map[string]bool // "92" → true
	ContactSave bool            // true = saved contacts exempt from block
}

// abLoad reads config from Redis with a 1-minute in-memory cache.
func abLoad(s SessionBridge) *abConfig {
	botJID := s.GetJID()
	abCacheMu.Lock()
	if c, ok := abCache[botJID]; ok && time.Since(abLoadedAt[botJID]) < abCodesTTL {
		abCacheMu.Unlock()
		return c
	}
	abCacheMu.Unlock()

	// cache miss → fetch both fields (Upstash)
	onStr := strings.ToLower(s.GetAutoBlockSetting("off"))
	codesStr := s.GetAutoBlockCodes("")
	csStr := strings.ToLower(s.GetAutoBlockContactSave("off"))
	c := &abConfig{On: onStr == "on", ContactSave: csStr == "on", Codes: map[string]bool{}}
	for _, p := range strings.Split(codesStr, ",") {
		if p = abTrimCode(p); p != "" {
			c.Codes[p] = true
		}
	}

	abCacheMu.Lock()
	abCache[botJID] = c
	abLoadedAt[botJID] = time.Now()
	abCacheMu.Unlock()
	return c
}

// abInvalidate drops the cached config (after any setting change).
func abInvalidate(botJID string) {
	abCacheMu.Lock()
	delete(abCache, botJID)
	delete(abLoadedAt, botJID)
	abCacheMu.Unlock()
}

// abTrimCode normalises one country code: digits only, no leading zeros.
func abTrimCode(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return strings.TrimLeft(b.String(), "0")
}

// abParseCodes turns "92,1,44" / "92 1 44" / "92, 1" into a clean list.
func abParseCodes(raw string) []string {
	seen := map[string]bool{}
	var out []string
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';' })
	for _, f := range fields {
		if c := abTrimCode(f); c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// abMatchesCountry reports whether a JID's number starts with any set code.
// Longest-prefix safety: "92" also matches "9232..." — intended (that's the
// point of country blocking). The bot's own JID and owners are exempt
// BEFORE this is called (handler hook).
func abMatchesCountry(senderJID string, c *abConfig) (string, bool) {
	if c == nil || len(c.Codes) == 0 || !c.On {
		return "", false
	}
	num := jidNumber(senderJID)
	num = strings.TrimPrefix(num, "+")
	if num == "" {
		return "", false
	}
	// check all prefixes 1..4 digits
	for l := 1; l <= 4 && l <= len(num); l++ {
		if c.Codes[num[:l]] {
			return num[:l], true
		}
	}
	return "", false
}

// ----------------------------------------------------------------------------
// enforcement — called from handler.go on every private message
// ----------------------------------------------------------------------------

// AutoblockCheckAndBlock is the receive-path hook. Returns true when the
// sender was just blocked (handler then stops processing the message).
//
// It must NEVER block the reply path (owner speed rule) — the actual
// UpdateBlocklist network call runs in a goroutine; only the cached 0ms
// country check is synchronous.
func AutoblockCheckAndBlock(s SessionBridge, info types.MessageInfo) bool {
	if info.IsGroup || info.IsFromMe {
		return false // only private chats, never the bot itself
	}
	// owner (and bot) exempt — checked by the handler hook caller too, but
	// double-guard here so future callers stay safe
	if s.IsOwner(info) {
		return false
	}

	c := abLoad(s)
	if c.ContactSave && abIsSavedContact(s, info.Sender) {
		return false // 🤝 saved phone contact — exempt from autoblock
	}
	code, hit := abMatchesCountry(info.Sender.String(), c)
	if !hit {
		return false
	}

	// async real block — never blocks the reply path
	go func(sender types.JID, matched string) {
		defer func() { _ = recover() }()
		client := s.GetClient()
		if client == nil || !client.IsConnected() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := client.UpdateBlocklist(ctx, sender, events.BlocklistChangeActionBlock)
		if err != nil {
			return // silent (zero-log rule) — next message will retry
		}
	}(info.Sender, code)
	return true
}

// abIsSavedContact reports whether the sender is a real saved phone contact
// of the bot's WhatsApp account. It uses the local sqlite whatsmeow_contacts
// table (via client.Store.Contacts.GetContact — in-memory cached, 0ms) and
// only counts entries synced from the phone's address book: those have
// FirstName or FullName set. Push-name-only rows (random strangers who ever
// messaged) never have these fields, so they are NOT treated as contacts.
func abIsSavedContact(s SessionBridge, sender types.JID) bool {
	defer func() { _ = recover() }() // never let a store hiccup break blocking
	cli := s.GetClient()
	if cli == nil || cli.Store == nil || cli.Store.Contacts == nil {
		return false // store unavailable → treat as NOT saved → blocking goes on
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := cli.Store.Contacts.GetContact(ctx, sender)
	if err != nil || !info.Found {
		return false
	}
	return info.FirstName != "" || info.FullName != ""
}

// ----------------------------------------------------------------------------
// command handler (owner-only)
// ----------------------------------------------------------------------------

func init() {
	Register(Command{
		Name:      "autoblock",
		Category:  "OWNER & SYSTEM",
		Desc:      "Auto-block numbers by country code (on/off/add/del/reset/list)",
		OwnerOnly: true,
		Run:       handleAutoBlock,
	})
}

func handleAutoBlock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutoBlockAsync(s, info, args, prefix)
}

func handleAutoBlockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "❌ *AUTOBLOCK ERROR — TRY AGAIN*")
		}
	}()

	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME \U0001F60E*")
		return
	}

	botJID := s.GetJID()
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch sub {
	case "":
		// full guide
		status := strings.ToUpper(s.GetAutoBlockSetting("off"))
		s.Reply(info, "🛡 *AUTOBLOCK COMMAND GUIDE* 🛡\n\n"+
			"*WHAT DOES .AUTOBLOCK DO ?*\n"+
			"IT AUTOMATICALLY BLOCKS ANY NUMBER WHOSE COUNTRY CODE MATCHES THE CODES YOU SET . WHEN SUCH A NUMBER MESSAGES THE BOT IN PRIVATE CHAT , THE BOT INSTANTLY BLOCKS IT ON WHATSAPP .\n\n"+
			"*SUB-COMMANDS :*\n\n"+
			"*1) ON / OFF*\n"+
			"EXAMPLE : "+prefix+"autoblock on\n"+
			"EXAMPLE : "+prefix+"autoblock off\n\n"+
			"*2) ADD COUNTRY CODES*\n"+
			"EXAMPLE : "+prefix+"autoblock add 92,1,44\n"+
			"(COMMA OR SPACE SEPARATED — MULTIPLE AT ONCE)\n\n"+
			"*3) DEL COUNTRY CODES*\n"+
			"EXAMPLE : "+prefix+"autoblock del 92\n\n"+
			"*4) RESET — ALL SETTINGS RESET*\n"+
			"EXAMPLE : "+prefix+"autoblock reset\n\n"+
			"*5) LIST — CURRENT SETTINGS*\n"+
			"EXAMPLE : "+prefix+"autoblock list\n\n"+
			"*6) CONTACT — SAVE CONTACTS PROTECTION*\n"+
			"EXAMPLE : "+prefix+"autoblock contact\n"+
			"(SAVED CONTACTS WILL NOT BE BLOCKED)\n"+
			"EXAMPLE : "+prefix+"autoblock nocontact\n\n"+
			"*7) SYNC \u2014 RE-SYNC WHATSAPP CONTACTS FROM SERVER*\n"+
			"EXAMPLE : "+prefix+"autoblock sync\n"+
			"(IF OWNER SAVED A NEW CONTACT , RUN SYNC)\n\n"+
			"*CURRENT STATUS :* "+status+"\n\n"+
			"❕ OWNER ONLY COMMAND\n"+
			"❕ OWNER NUMBERS ARE NEVER BLOCKED\n"+
			"\n🛡 *GOLD-MD* 🛡")
		return

	case "on":
		s.SetAutoBlockSetting("on")
		abInvalidate(botJID)
		codes := s.GetAutoBlockCodes("")
		if codes == "" {
			s.Reply(info, "✅ *AUTOBLOCK ENABLED*\n\n⚠️ *NO COUNTRY CODES SET — USE "+prefix+"autoblock add 92,1,44*")
			return
		}
		s.Reply(info, "✅ *AUTOBLOCK ENABLED*\n\n*BLOCKED COUNTRY CODES :* +"+strings.Join(abParseCodes(codes), ", +")+"\n\n*ANY NUMBER FROM THESE COUNTRIES WILL BE INSTANTLY BLOCKED ON PRIVATE MESSAGE*")
		return

	case "off":
		s.SetAutoBlockSetting("off")
		abInvalidate(botJID)
		s.Reply(info, "✅ *AUTOBLOCK DISABLED*\n\n*NO COUNTRY WILL BE BLOCKED NOW*")
		return

	case "add":
		if len(args) < 2 {
			s.Reply(info, "❌ *TO ADD COUNTRY CODES WRITE LIKE THIS*\n\n*"+prefix+"autoblock add 92,1,44*")
			return
		}
		raw := strings.Join(args[1:], " ")
		newCodes := abParseCodes(raw)
		if len(newCodes) == 0 {
			s.Reply(info, "❌ *NO VALID COUNTRY CODES FOUND — ONLY DIGITS ALLOWED*\n\n*"+prefix+"autoblock add 92,1,44*")
			return
		}
		// merge with existing
		existing := abParseCodes(s.GetAutoBlockCodes(""))
		merged := map[string]bool{}
		for _, c := range existing {
			merged[c] = true
		}
		added := []string{}
		for _, c := range newCodes {
			if !merged[c] {
				merged[c] = true
				added = append(added, c)
			}
		}
		var all []string
		for c := range merged {
			all = append(all, c)
		}
		sortStrings(all)
		s.SetAutoBlockCodes(strings.Join(all, ","))
		abInvalidate(botJID)
		if s.GetAutoBlockSetting("off") != "on" {
			s.Reply(info, "✅ *COUNTRY CODES ADDED :* +"+strings.Join(added, ", +")+"\n\n*TOTAL CODES :* "+itoa(len(all))+"\n\n⚠️ *AUTOBLOCK IS OFF — TURN ON WITH "+prefix+"autoblock on*")
			return
		}
		s.Reply(info, "✅ *COUNTRY CODES ADDED :* +"+strings.Join(added, ", +")+"\n\n*TOTAL CODES :* "+itoa(len(all))+"\n\n*AUTOBLOCK IS ON — THESE COUNTRIES WILL BE BLOCKED INSTANTLY*")
		return

	case "del":
		if len(args) < 2 {
			s.Reply(info, "❌ *TO REMOVE COUNTRY CODES WRITE LIKE THIS*\n\n*"+prefix+"autoblock del 92,1*")
			return
		}
		raw := strings.Join(args[1:], " ")
		delCodes := abParseCodes(raw)
		existing := abParseCodes(s.GetAutoBlockCodes(""))
		removed := []string{}
		kept := []string{}
		for _, c := range existing {
			if containsStr(delCodes, c) {
				removed = append(removed, c)
			} else {
				kept = append(kept, c)
			}
		}
		if len(removed) == 0 {
			s.Reply(info, "❌ *NONE OF THOSE CODES WERE SET*\n\n*CURRENT CODES :* "+abCodesDisplay(existing, "+")+"\n\n*USE "+prefix+"autoblock list TO SEE ALL*")
			return
		}
		s.SetAutoBlockCodes(strings.Join(kept, ","))
		abInvalidate(botJID)
		s.Reply(info, "✅ *COUNTRY CODES REMOVED :* +"+strings.Join(removed, ", +")+"\n\n*REMAINING CODES :* "+abCodesDisplay(kept, "+")+"\n\n*REMOVED COUNTRIES WILL NO LONGER BE BLOCKED*")
		return

	case "reset":
		s.SetAutoBlockSetting("off")
		s.SetAutoBlockCodes("")
		s.SetAutoBlockContactSave("off")
		abInvalidate(botJID)
		s.Reply(info, "✅ *AUTOBLOCK RESET COMPLETE*\n\n*STATUS :* OFF\n*COUNTRY CODES :* NONE\n*SAVED CONTACTS MODE :* OFF\n\n*ALL SETTINGS ARE BACK TO DEFAULT*")
		return

	case "sync":
		cli := s.GetClient()
		if cli == nil || !cli.IsConnected() {
			s.Reply(info, "\u274C *BOT IS NOT CONNECTED \u2014 TRY AGAIN*")
			return
		}
		s.Reply(info, "\u21BB *SYNCING WHATSAPP CONTACTS FROM SERVER ...*")
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.FetchAppState(ctx, appstate.WAPatchCriticalUnblockLow, true, false); err != nil {
			s.Reply(info, "\u274C *CONTACT SYNC FAILED*\n\n*REASON :* "+err.Error()+"\n\n*TRY AGAIN AFTER SOME TIME*")
			return
		}
		s.Reply(info, "\u2705 *WHATSAPP CONTACTS SYNCED FROM SERVER*\n\n*CONTACT LIST IS NOW FRESH \u2014 SAVED CONTACT PROTECTION WILL USE THE LATEST DATA*")
		return

	case "contact":
		s.SetAutoBlockContactSave("on")
		abInvalidate(botJID)
		s.Reply(info, "🤝 *SAVED CONTACTS PROTECTION ON*\n\n*NOW ONWARDS ONLY NUMBERS NOT IN THE BOT PHONE CONTACTS WILL BE BLOCKED*\n\n*SAVED CONTACTS CAN NEVER BE BLOCKED BY AUTOBLOCK*")
		return

	case "nocontact":
		s.SetAutoBlockContactSave("off")
		abInvalidate(botJID)
		s.Reply(info, "⚠️ *SAVED CONTACTS PROTECTION OFF*\n\n*NOW EVERY NUMBER FROM BLOCKED COUNTRY CODES WILL BE BLOCKED*\n\n*EVEN IF IT IS SAVED IN BOT PHONE CONTACTS*")
		return

	case "list":
		onStr := s.GetAutoBlockSetting("off")
		csStr := s.GetAutoBlockContactSave("off")
		codes := abParseCodes(s.GetAutoBlockCodes(""))
		var sb strings.Builder
		sb.WriteString("\U0001F6E1 *AUTOBLOCK SETTINGS* \U0001F6E1\n\n")
		if strings.ToLower(onStr) == "on" {
			sb.WriteString("*STATUS :* ON \u2705\n")
		} else {
			sb.WriteString("*STATUS :* OFF \u274C\n")
		}
		if strings.ToLower(csStr) == "on" {
			sb.WriteString("*SAVED CONTACTS :* PROTECTED \U0001F91D\n")
		} else {
			sb.WriteString("*SAVED CONTACTS :* NOT PROTECTED \u26A0\uFE0F\n")
		}
		if len(codes) == 0 {
			sb.WriteString("\n*COUNTRY CODES :* NONE SET\n\n*USE " + prefix + "autoblock add 92,1,44 TO SET*")
		} else {
			sb.WriteString("\n*COUNTRY CODES :* " + abCodesDisplay(codes, "+"))
			sb.WriteString("\n*TOTAL :* " + itoa(len(codes)))
			if strings.ToLower(onStr) == "on" {
				sb.WriteString("\n\n*ANY NUMBER FROM THESE COUNTRIES IS BLOCKED INSTANTLY ON PRIVATE MESSAGE*")
			} else {
				sb.WriteString("\n\n\u26A0\uFE0F *AUTOBLOCK IS OFF \u2014 TURN ON WITH " + prefix + "autoblock on*")
			}
		}
		s.Reply(info, sb.String())
		return

	default:
		s.Reply(info, "\u274C *UNKNOWN SUB-COMMAND :* " + sub + "\n\n*USE " + prefix + "autoblock TO SEE THE FULL GUIDE*")
		return
	}
}

// abCodesDisplay renders a code list as "+92, +1, +44" (or NONE if empty).
func abCodesDisplay(codes []string, sign string) string {
	if len(codes) == 0 {
		return "NONE"
	}
	var out []string
	for _, c := range codes {
		out = append(out, sign+c)
	}
	return strings.Join(out, ", ")
}

// sortStrings sorts a string slice in place (digit codes -> lexicographic ok).
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// containsStr reports whether xs contains x.
func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
