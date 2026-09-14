package goldcmds

// ============================================================================
// GOLD-MD — .antilink command (aliases: .al)
//
// Ported from UMAR-MD (Node.js pair.js, lines 14212-14495) — same text,
// same behaviour, adapted for whatsmeow:
//   .antilink                  → full command info
//   .antilink on / off         → enable / disable link detection in group
//   .antilink action           → action info (warn / delete / kick)
//   .antilink action warn      → set action=warn
//   .antilink action warn <n>  → set action=warn + max-warnings=n (1-50)
//   .antilink action warn reset → reset max-warnings to default
//   .antilink action delete    → set action=delete
//   .antilink action kick      → set action=kick
//   .antilink allow <domain>   → whitelist a domain (links from it won't trigger)
//   .antilink delete <domain>  → remove a domain from whitelist
//   .antilink allowedlist      → show whitelisted domains
//   .antilink reset            → full reset (off + warn + default max + clear)
//
// Per-group config in Redis (settings:<groupJID>):
//   field "antilink"           = on/off
//   field "antilink:action"    = warn/delete/kick
//   field "antilink:maxwarn"   = 1-50
// Whitelist: per-group SET "antilink:allowed" (domains).
//
// The actual link detection on incoming group messages is in handler.go
// (applyAntiLink), which calls AntilinkCheckAndEnforce.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"regexp"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const antilinkFeature = "antilink"
const antilinkUpper = "ANTILINK"
const antilinkAllowedSet = "antilink:allowed"

// linkRegex mirrors LINK_REGEX in pair.js line 7411:
//
//	/(https?:\/\/[^\s]+)|(\bwww\.[^\s]+)|(\b[a-zA-Z0-9-]+\.(com|net|org|io|co|me|link|xyz|info|live|tv|gg|app|to|ly)\b[^\s]*)/gi
var linkRegex = regexp.MustCompile(`(?i)(https?://[^\s]+)|(\bwww\.[^\s]+)|(\b[a-zA-Z0-9-]+\.(?:com|net|org|io|co|me|link|xyz|info|live|tv|gg|app|to|ly)\b[^\s]*)`)

// extractLinks finds all links in a text body (mirrors UmarExtractLinks).
func extractLinks(text string) []string {
	if text == "" {
		return nil
	}
	matches := linkRegex.FindAllString(text, -1)
	return matches
}

// getDomainFromLink extracts the bare domain from a link (mirrors
// UmarGetDomainFromLink): strip scheme, strip www., take up to first / or ?.
func getDomainFromLink(link string) string {
	d := strings.ToLower(strings.TrimSpace(link))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	if i := strings.IndexByte(d, '?'); i >= 0 {
		d = d[:i]
	}
	return d
}

// normalizeDomain mirrors _UmarNormalizeDomain: strip scheme/www, take up to
// first / (path removed). Used when storing/looking-up allowed domains.
func normalizeDomain(raw string) string {
	d := strings.ToLower(strings.TrimSpace(raw))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	return d
}

// domainAllowed checks if a link's domain is in the allowed list, matching
// either exactly or as a subdomain (linkDomain endsWith ".allowed").
// Mirrors the Node.js: linkDomain === allowed || linkDomain.endsWith('.'+allowed)
func domainAllowed(linkDomain string, allowedList []string) bool {
	for _, a := range allowedList {
		if linkDomain == a {
			return true
		}
		if strings.HasSuffix(linkDomain, "."+a) {
			return true
		}
	}
	return false
}

// ── exported helpers for handler.go ────────────────────────────────────

// AntilinkIsOn reports whether antilink is enabled in the given group.
func AntilinkIsOn(s SessionBridge, groupJID string) bool {
	return antiEnabled(s, groupJID, antilinkFeature)
}

// AntilinkAllowedDomains returns the whitelisted domains for a group.
func AntilinkAllowedDomains(s SessionBridge, groupJID string) []string {
	return s.GroupSetMembers(groupJID, antilinkAllowedSet)
}

// AntilinkCheckAndEnforce checks a message body for non-whitelisted links and,
// if found, enforces the configured action. Returns true if an offence was
// detected (and action taken), false otherwise. Mirrors the antilink detection
// handler in pair.js (lines 15953-16140).
func AntilinkCheckAndEnforce(s SessionBridge, info types.MessageInfo, body string) bool {
	if !AntilinkIsOn(s, info.Chat.String()) {
		return false
	}
	links := extractLinks(body)
	if len(links) == 0 {
		return false
	}
	allowed := AntilinkAllowedDomains(s, info.Chat.String())
	// If ALL links are allowed → no action
	allAllowed := true
	for _, link := range links {
		if !domainAllowed(getDomainFromLink(link), allowed) {
			allAllowed = false
			break
		}
	}
	if allAllowed {
		return false
	}
	// Offence detected → enforce
	senderJID := info.Sender.String()
	senderNum := strings.SplitN(senderJID, "@", 2)[0]
	EnforceAntiAction(s, info, antilinkFeature, antilinkUpper, "LINKS", "LINKS NOT ALLOWED", senderJID, senderNum)
	return true
}

// ── command handler ────────────────────────────────────────────────────

func handleAntilink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAntilinkAsync(s, info, args, prefix)
}

func handleAntilinkAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroupOwner(s, info) {
		return
	}
	groupJID := info.Chat.String()

	// No args → full command info
	if len(args) == 0 {
		allowed := AntilinkAllowedDomains(s, groupJID)
		curAction := antiAction(s, groupJID, antilinkFeature)
		maxW := antiMaxWarnings(s, groupJID, antilinkFeature)
		enabled := antiEnabled(s, groupJID, antilinkFeature)

		s.Reply(info, "*🔰 ANTILINK COMMAND INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTILINK ON ❱*\n*WHEN ANTILINK IS ON THEN IF ANY USER SENDS ANY LINK IN GROUP THE BOT WILL DETECT THE LINK AND ACTION WILL APPLY*\n\n\n*TYPE ❰ "+prefix+"ANTILINK OFF ❱*\n*WHEN ANTILINK IS OFF THEN ALL MEMBERS CAN FREELY SEND LINKS IN THIS GROUP*\n\n*🔰 ANTILINK ACTIONS INFO 🔰*\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION ❱*\n*YOU WILL GET INFO OF ANTILINK ACTIONS*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION DELETE ❱*\n*WHEN ACTION IS DELETE THEN LINKS WILL BE DETECTED AND AUTO DELETED*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION KICK ❱*\n*WHEN ACTION IS KICK THEN AS SOON AS LINK IS DETECTED THE USER WILL BE REMOVED*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION WARN ❱*\n*WHEN ACTION IS WARN THEN USER WILL GET WARNINGS AS SOON AS WARNINGS ARE FINISHED THE USER WILL BE AUTO REMOVED FROM GROUP*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION WARN ❰40❱ ❱*\n*SET YOUR WARNINGS AS MANY AS YOU WANT 5 10 15 25 AS YOU WISH MAX ❰ 50 ❱ ONLY*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ACTION RESET ❱*\n*ANTILINK ACTIONS WILL BE RESET*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ALLOW ❰youtube.com❱ ❱*\n*❰ "+prefix+"ANTILINK ALLOW ❰google.com❱ ❱*\n*❰ "+prefix+"ANTILINK ALLOW ❰whatsapp.com❱ ❱*\n*WHICHEVER LINKS YOU ALLOW THOSE LINKS WILL NOT HAVE ACTION APPLY BECAUSE THESE LINKS WILL BE SET IN ALLOWED LIST EXCEPT ALLOWED LINKS IF ANY OTHER LINK IS FOUND THEN ANTILINK ACTION WILL APPLY*\n\n\n*TYPE ❰ "+prefix+"ANTILINK DELETE ❰youtube.com❱ ❱*\n*❰ "+prefix+"ANTILINK DELETE ❰google.com❱ ❱*\n*❰ "+prefix+"ANTILINK DELETE ❰whatsapp.com❱ ❱*\n*DELETE ALLOWED LINKS FROM ALLOWED LIST WHEN LINKS ARE DELETED FROM ALLOWED LIST THEN ANTILINK ACTION WILL APPLY ON THESE LINKS*\n\n\n*TYPE ❰ "+prefix+"ANTILINK ALLOWEDLIST ❱*\n*CHECK THE LIST OF ALLOWED LINKS THAT YOU ALLOWED*\n\n\n\n\n*TYPE ❰ "+prefix+"ANTILINK RESET ❱*\n*TO RESET FULL ANTILINK COMMAND*\n\n\n*ANTILINK NOW :❱ ❰ "+boolOnOff(enabled)+" ❱*\n*ACTION :❱ ❰ "+strings.ToUpper(curAction)+" ❱*\n*MAX WARNINGS :❱ ❰ "+strconv.Itoa(maxW)+" ❱*\n*ALLOWED LINKS QUANTITY :❱ ❰ "+strconv.Itoa(len(allowed))+" ❱*")
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	// ON
	if sub == "on" {
		antiSetEnabled(s, groupJID, antilinkFeature, true)
		curAction := antiAction(s, groupJID, antilinkFeature)
		maxW := antiMaxWarnings(s, groupJID, antilinkFeature)
		s.Reply(info, "*🔰 ANTILINK ACTIVATED 🔰*\n\n*ACTION :❱ "+strings.ToUpper(curAction)+"*\n\n*🔰 ACTION INFO 🔰*\n"+antiActionInfo(curAction, maxW, "LINKS"))
		return
	}

	// OFF
	if sub == "off" {
		antiSetEnabled(s, groupJID, antilinkFeature, false)
		s.Reply(info, "*🔰 ANTILINK DE-ACTIVATED 🔰*")
		return
	}

	// ACTION (shared tree)
	if sub == "action" {
		handleAntiAction(s, info, args[1:], prefix, antilinkFeature, antilinkUpper)
		return
	}

	// ALLOW <domain>
	if sub == "allow" {
		domainArg := strings.TrimSpace(strings.Join(args[1:], " "))
		if domainArg == "" {
			s.Reply(info, "*🔰 ANTILINK ALLOW 🔰*\n\n*TYPE ❰ "+prefix+"ANTILINK ALLOW youtube.com ❱*")
			return
		}
		clean := normalizeDomain(domainArg)
		_ = s.GroupSetAdd(groupJID, antilinkAllowedSet, clean)
		s.Reply(info, "*🔰 LINKS ALLOWED :❱ "+clean+"*\n\n*LINKS LIKE THIS WILL NOT BE DELETED NOW*")
		return
	}

	// DELETE <domain>
	if sub == "delete" {
		domainArg := strings.TrimSpace(strings.Join(args[1:], " "))
		if domainArg == "" {
			s.Reply(info, "*🔰 ANTILINK DELETE 🔰*\n\n*TYPE ❰ "+prefix+"ANTILINK DELETE youtube.com ❱*")
			return
		}
		clean := normalizeDomain(domainArg)
		members := s.GroupSetMembers(groupJID, antilinkAllowedSet)
		found := false
		for _, m := range members {
			if m == clean {
				found = true
				break
			}
		}
		if found {
			_ = s.GroupSetRem(groupJID, antilinkAllowedSet, clean)
			s.Reply(info, "*🔰 LINK REMOVED FROM WHITELIST :❱ "+clean+"*")
		} else {
			s.Reply(info, "*🔰 LINK NOT FOUND IN WHITELIST :❱ "+clean+"*")
		}
		return
	}

	// ALLOWEDLIST
	if sub == "allowedlist" {
		allowed := AntilinkAllowedDomains(s, groupJID)
		if len(allowed) == 0 {
			s.Reply(info, "*🔰 ALLOWED LIST EMPTY*")
			return
		}
		var sb strings.Builder
		for i, d := range allowed {
			sb.WriteString(strconv.Itoa(i+1) + ". " + d + "\n")
		}
		s.Reply(info, "*🔰 ANTILINK ALLOWED LINKS 🔰*\n\n"+sb.String()+"\n*TOTAL :❱ "+strconv.Itoa(len(allowed))+"*")
		return
	}

	// RESET
	if sub == "reset" {
		antiResetSettings(s, groupJID, antilinkFeature, antilinkAllowedSet)
		s.Reply(info, "*🔰 ANTILINK FULLY RESET*\n\n*STATUS :❱ OFF*\n*ACTION :❱ WARN*\n*MAX WARNINGS :❱ "+strconv.Itoa(defaultAntiMaxWarnings)+"*\n*ALLOWED LIST :❱ CLEARED*\n*WARNINGS :❱ CLEARED*")
		return
	}

	// Unknown
	s.Reply(info, "*WRONG COMMAND*\n*TYPE ❰ ANTILINK ❱ FOR HELP*")
}

// ── registration ───────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "antilink", Category: "ANTI & PROTECTION", Desc: "THIS COMMAND IS USED TO STOP LINKS IN THE GROUP. IT CAN WARN, DELETE OR KICK MEMBERS WHO SEND LINKS.", OwnerOnly: true, Run: handleAntilink})
	Register(Command{Name: "al", OwnerOnly: true, Hidden: true, Run: handleAntilink})
}
