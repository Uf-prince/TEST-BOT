package goldcmds

// ============================================================================
// GOLD-MD — .tempnumber command (free temporary numbers, receive-smss.live)
// File: tempnum.go
// ----------------------------------------------------------------------------
// Free unlimited temporary phone numbers with LIVE SMS inboxes — no API
// key, no registration, no payment.  Data comes live from receive-smss.live,
// a public temp-number site (plain HTML scraping, same as a browser):
//
//   /                       → all country links /sms/<slug>
//   /sms/<country>          → that country's free number list (fresh!)
//   /sms/<country>/<number> → that number's SMS inbox (LIVE messages)
//
// COMMANDS:
//   .tempnumber                → guide + supported country codes
//   .tempnumber +91            → India numbers list (TYPE ❮ X ❯ TO PICK)
//   .tempnumber 91             → same (bare code also accepted)
//   (then user types bare X)   → that number is picked & confirmed
//   .checknumber <number>      → LIVE inbox: FETCHING 15s auto-edit loop,
//                                then latest SMS + OTP codes
//
// receive-smss.live inboxes are genuinely LIVE (new SMS appear within
// seconds/minutes) — messages are embedded in the page HTML inside an
// astro-island props JSON blob, unmasked, no login needed.
// ============================================================================

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── receive-smss.live endpoints ─────────────────────────────────────────────

const (
	tnBaseURL     = "https://receive-smss.live"
	tnUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	tnCacheTTL    = 5 * time.Minute
	tnHTTPTimeout = 20 * time.Second
	tnMaxMessages = 10 // max SMS shown per inbox
	tnMaxNumbers  = 12 // max numbers in one country list
	tnLiveSeconds = 15 // .checknumber live-fetch window
	tnPickWindow  = 2 * time.Minute
)

// tnCountry — one supported country.
type tnCountry struct {
	Code string // dial code WITHOUT +, e.g. "91"
	Slug string // site slug, e.g. "in"
	Name string // display name, e.g. "INDIA"
}

// tnCountries — the site's supported free-number countries (27 live).
// Keep in sync with https://receive-smss.live/ (/sms/<slug> links).
var tnCountries = []tnCountry{
	{Code: "1", Slug: "us", Name: "UNITED STATES"},
	{Code: "1", Slug: "ca", Name: "CANADA"},
	{Code: "44", Slug: "uk", Name: "UNITED KINGDOM"},
	{Code: "91", Slug: "in", Name: "INDIA"},
	{Code: "49", Slug: "de", Name: "GERMANY"},
	{Code: "33", Slug: "fr", Name: "FRANCE"},
	{Code: "34", Slug: "es", Name: "SPAIN"},
	{Code: "39", Slug: "it", Name: "ITALY"},
	{Code: "31", Slug: "nl", Name: "NETHERLANDS"},
	{Code: "32", Slug: "be", Name: "BELGIUM"},
	{Code: "41", Slug: "ch", Name: "SWITZERLAND"},
	{Code: "43", Slug: "at", Name: "AUSTRIA"},
	{Code: "46", Slug: "se", Name: "SWEDEN"},
	{Code: "47", Slug: "no", Name: "NORWAY"},
	{Code: "45", Slug: "dk", Name: "DENMARK"},
	{Code: "358", Slug: "fi", Name: "FINLAND"},
	{Code: "48", Slug: "pl", Name: "POLAND"},
	{Code: "351", Slug: "pt", Name: "PORTUGAL"},
	{Code: "353", Slug: "ie", Name: "IRELAND"},
	{Code: "61", Slug: "au", Name: "AUSTRALIA"},
	{Code: "62", Slug: "id", Name: "INDONESIA"},
	{Code: "60", Slug: "my", Name: "MALAYSIA"},
	{Code: "63", Slug: "ph", Name: "PHILIPPINES"},
	{Code: "84", Slug: "vn", Name: "VIETNAM"},
	{Code: "90", Slug: "tr", Name: "TURKEY"},
	{Code: "7", Slug: "ru", Name: "RUSSIA"},
	{Code: "55", Slug: "br", Name: "BRAZIL"},
	{Code: "54", Slug: "ar", Name: "ARGENTINA"},
	{Code: "234", Slug: "ng", Name: "NIGERIA"},
	{Code: "966", Slug: "sa", Name: "SAUDI ARABIA"},
	{Code: "971", Slug: "ae", Name: "UAE"},
	{Code: "20", Slug: "eg", Name: "EGYPT"},
	{Code: "52", Slug: "mx", Name: "MEXICO"},
}

// ── HTTP fetch helper ───────────────────────────────────────────────────────

func tnFetch(path string) (string, error) {
	client := &http.Client{Timeout: tnHTTPTimeout}
	req, err := http.NewRequest("GET", tnBaseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", tnUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// cache-buster so every poll gets the FRESHEST page (no CDN cache)
	req.URL.RawQuery = strings.TrimSpace(req.URL.Query().Encode() + " _=" + strconv.FormatInt(time.Now().UnixNano(), 10))
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ── parsing ─────────────────────────────────────────────────────────────────

var (
	// country list page: /sms/<slug>/<digits>
	tnNumHrefRe = regexp.MustCompile(`href="/sms/([a-z]{2})/([0-9]{7,15})"`)
	// card detail: "+91 747 173 5893", "4382 messages", "6 minutes ago", "WhatsApp"
	tnCardNumRe   = regexp.MustCompile(`dir="ltr">\s*\+?\s*([0-9][0-9 ]{6,18})\s*<`)
	tnCardMsgRe   = regexp.MustCompile(`>([0-9][0-9,]*)\s*messages<`)
	tnCardFreshRe = regexp.MustCompile(`>([^<>]{0,30}?(?:second|minute|hour|day)s? ago)<`)
	tnCardAppRe   = regexp.MustCompile(`>(WhatsApp|Telegram|TikTok|Facebook|Instagram|Google|Amazon|Discord|Snapchat|PayPal|Uber|Netflix|Twitter|Imo|Binance|Apple|Microsoft|Outlook|LinkedIn|OTHER[^<>]*)<`)
	// inbox page: astro-island props JSON (HTML-escaped) —
	//   "sender":[0,"18575765739"],"content":[0,"..."],"receivedAt":[0,"..."]
	tnSenderRe = regexp.MustCompile(`&quot;sender&quot;:\[0,&quot;([^&]*?)&quot;\]`)
	tnTimeRe   = regexp.MustCompile(`&quot;receivedAt&quot;:\[0,&quot;([^&]*?)&quot;\]`)
	// content needs its own sub-block scan because it may contain &quot; …
	tnContentScanRe = regexp.MustCompile(`(?s)&quot;content&quot;:\[0,&quot;(.*?)&quot;\],&quot;receivedAt&quot;`)
	tnHtmlEscRe     = regexp.MustCompile(`&quot;|&amp;|&#39;|&lt;|&gt;|&nbsp;|\\n|\\u003c|\\u003e`)
	tnOTPRe         = regexp.MustCompile(`(?i)\b(?:code|otp|pin|token|password|passcode|verification|verif|confirm[a-z]*|security|login)\b[^\d]{0,40}?(\d{4,8}|\d{3}[- \.]\d{3})\b|\b(\d{4,8}|\d{3}[- \.]\d{3})\b[^\d]{0,40}?\b(?:is your|code|otp)\b`)
)

// tnUnescape turns HTML-escaped astro-props text into readable text.
func tnUnescape(s string) string {
	r := strings.NewReplacer(
		"&quot;", `"`,
		"&#39;", "'",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&nbsp;", " ",
		"\\n", "\n",
		"\\u003c", "<",
		"\\u003e", ">",
	)
	return r.Replace(s)
}

// tnStrip — remove any leftover HTML tags + collapse whitespace.
var tnTagRe = regexp.MustCompile(`<[^>]+>`)

func tnStrip(s string) string {
	s = tnTagRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// ── country list (cached 5 min) ─────────────────────────────────────────────

type tnNumEntry struct {
	Number   string // digits only, e.g. "917471735893"
	Display  string // pretty, e.g. "+91 747 173 5893"
	Messages string // e.g. "4382"
	Fresh    string // e.g. "6 minutes ago"
	App      string // e.g. "WhatsApp"
}

var (
	tnListMu    sync.Mutex
	tnListCache = map[string][]tnNumEntry{}
	tnListAt    = map[string]time.Time{}
)

func tnCountryNumbers(slug string) ([]tnNumEntry, error) {
	tnListMu.Lock()
	if c, ok := tnListCache[slug]; ok && time.Since(tnListAt[slug]) < tnCacheTTL && len(c) > 0 {
		tnListMu.Unlock()
		return c, nil
	}
	tnListMu.Unlock()

	html, err := tnFetch("/sms/" + slug)
	if err != nil {
		return nil, err
	}
	// split page into number cards
	cards := strings.Split(html, `<a href="/sms/`)
	var out []tnNumEntry
	seen := map[string]bool{}
	for i := 1; i < len(cards); i++ {
		blk := cards[i][:min(2000, len(cards[i]))]
		m := tnCardNumRe.FindStringSubmatch(blk)
		if m == nil {
			continue
		}
		num := strings.ReplaceAll(m[1], " ", "")
		if len(num) < 7 || seen[num] {
			continue
		}
		seen[num] = true
		e := tnNumEntry{Number: num, Display: "+" + m[1], Messages: "-", Fresh: "-", App: "SMS"}
		if mm := tnCardMsgRe.FindStringSubmatch(blk); mm != nil {
			e.Messages = mm[1]
		}
		if mf := tnCardFreshRe.FindStringSubmatch(blk); mf != nil {
			e.Fresh = mf[1]
		}
		if ma := tnCardAppRe.FindStringSubmatch(blk); ma != nil {
			e.App = strings.ToUpper(ma[1])
		}
		out = append(out, e)
		if len(out) >= 40 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no numbers found")
	}
	// cache
	tnListMu.Lock()
	tnListCache[slug] = out
	tnListAt[slug] = time.Now()
	tnListMu.Unlock()
	return out, nil
}

// ── inbox parsing (LIVE) ────────────────────────────────────────────────────

// tnInboxSMS is one SMS row in a number's inbox.
type tnInboxSMS struct {
	From, Text, Date, OTP string
}

// tnParseInbox extracts SMS rows from a number's inbox page (astro-island).
func tnParseInbox(html string) []tnInboxSMS {
	senders := tnSenderRe.FindAllStringSubmatch(html, -1)
	times := tnTimeRe.FindAllStringSubmatch(html, -1)
	scans := tnContentScanRe.FindAllStringSubmatch(html, -1)
	if len(senders) == 0 {
		return nil
	}
	n := len(senders)
	if len(times) < n {
		n = len(times)
	}
	if len(scans) < n {
		n = len(scans)
	}
	var out []tnInboxSMS
	for i := 0; i < n; i++ {
		text := tnUnescape(scans[i][1])
		sms := tnInboxSMS{
			From: tnUnescape(senders[i][1]),
			Text: text,
			Date: tnUnescape(times[i][1]),
		}
		if m := tnOTPRe.FindStringSubmatch(sms.Text); m != nil {
			if len(m) > 1 && m[1] != "" {
				sms.OTP = m[1]
			} else if len(m) > 2 && m[2] != "" {
				sms.OTP = m[2]
			}
		}
		out = append(out, sms)
	}
	return out
}

// tnGetInbox fetches (cache-busted) and parses the inbox of a number.
// country slug is derived from the number's dial prefix automatically.
func tnGetInbox(num string) ([]tnInboxSMS, error) {
	if slug := tnSlugForNumber(num); slug != "" {
		if html, err := tnFetch("/sms/" + slug + "/" + num); err == nil {
			if list := tnParseInbox(html); len(list) > 0 {
				return list, nil
			}
		}
	}
	// fallback: try every slug (shortest prefix first)
	for _, c := range tnCountries {
		if html, err := tnFetch("/sms/" + c.Slug + "/" + num); err == nil {
			if list := tnParseInbox(html); len(list) > 0 {
				return list, nil
			}
		}
	}
	return nil, fmt.Errorf("inbox not found")
}

// tnSlugForNumber maps a full number (e.g. 917471735893) to its country slug
// by longest-prefix match over tnCountries.
func tnSlugForNumber(num string) string {
	num = tnDigitsStr(num)
	if num == "" {
		return ""
	}
	best, bestLen := "", 0
	for _, c := range tnCountries {
		if strings.HasPrefix(num, c.Code) && len(c.Code) > bestLen {
			best, bestLen = c.Slug, len(c.Code)
		}
	}
	return best
}

func tnDigitsStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── pick session (bare-number reply, like .automsg delete) ─────────────────

type tnPickSess struct {
	Numbers []tnNumEntry
	Country tnCountry
	Expiry  time.Time
}

var (
	tnPickMu   sync.Mutex
	tnPickList = map[string]*tnPickSess{}
)

// TempnumTryPickReply — prefix-less hook: agar user ne abhi .tempnumber +91
// ka numbered list dekha hai aur uska next message sirf ek number hai
// (jaise "3"), to wahi number pick ho jata hai. handler.go isko apne
// prefix-check se PEHLE call karta hai. Baaki cases me false.
func TempnumTryPickReply(s SessionBridge, info types.MessageInfo, body string) bool {
	t := strings.TrimSpace(body)
	if t == "" || !tnIsJustNumber(t) {
		return false
	}
	key := s.GetJID() + "|" + info.Sender.String()
	tnPickMu.Lock()
	sess := tnPickList[key]
	tnPickMu.Unlock()
	if sess == nil || len(sess.Numbers) == 0 {
		return false
	}
	if time.Now().After(sess.Expiry) {
		tnPickMu.Lock()
		delete(tnPickList, key)
		tnPickMu.Unlock()
		return false
	}
	choice, err := strconv.Atoi(t)
	if err != nil || choice < 1 || choice > len(sess.Numbers) {
		return false
	}
	picked := sess.Numbers[choice-1]
	tnPickMu.Lock()
	delete(tnPickList, key)
	tnPickMu.Unlock()

	go tnConfirmPick(s, info, picked, sess.Country)
	return true
}

func tnIsJustNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// tnConfirmPick — user ne X pick kiya: confirm + live inbox preview.
func tnConfirmPick(s SessionBridge, info types.MessageInfo, e tnNumEntry, c tnCountry) {
	list, err := tnGetInbox(e.Number)
	if err != nil {
		list = nil // network hiccup pe bhi number pick confirm ho jata hai
	}
	var b strings.Builder
	b.WriteString("*\U0001f530 FREE TEMP NUMBER \U0001f530*\n\n")
	b.WriteString("*NUMBER :\u2771 " + e.Display + "*\n")
	b.WriteString("*COUNTRY :\u2771 " + c.Name + "*\n")
	b.WriteString("*STATUS :\u2771 ACTIVE \u2014 INBOX LIVE (RECEIVES SMS)*\n")
	if len(list) > 0 {
		b.WriteString("\n*\U0001f530 RECENT SMS (LAST 3) :\u2771*\n")
		n := list
		if len(n) > 3 {
			n = n[:3]
		}
		for _, sms := range n {
			b.WriteString("*\u276f FROM :\u2771 " + sms.From + "*\n*" + sms.Text + "*")
			if sms.OTP != "" {
				b.WriteString("\n*\U0001f530 LAST CODE :\u2771 " + sms.OTP + "*")
			}
			b.WriteString("\n\n")
		}
	} else {
		b.WriteString("\n*INBOX IS EMPTY RIGHT NOW \u2014 SMS WILL APPEAR WHEN SOMEONE SENDS ONE*\n\n")
	}
	b.WriteString("*\U0001f530 READ THE SMS LIVE :\u2771*\n*TYPE \u2770 " + "checknumber " + e.Display + " \u2771*\n\n")
	b.WriteString("*\U0001f530 WANT ANOTHER COUNTRY :\u2771*\n*TYPE \u2770 tempnumber +<CODE> \u2771 \u2014 EXAMPLE \u2770 tempnumber +91 \u2771*")
	s.Reply(info, b.String())
}

// ── reply builders (same 🔰 style as other GOLD-MD commands) ────────────────

const tnSep = "*\U0001f530\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2022\u2741\u2740\u2741\u2022\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\U0001f530*"

// tnGuide — .tempnumber (bare): guide + supported countries + codes.
func tnGuide(prefix string) string {
	var b strings.Builder
	b.WriteString("*\U0001f530 TEMP NUMBER \U0001f530*\n\n")
	b.WriteString("*FREE TEMPORARY PHONE NUMBERS WITH LIVE SMS INBOX \u2014 NO API KEY, NO LOGIN, NO PAYMENT*\n\n")
	b.WriteString(tnSep + "\n")
	b.WriteString("*HOW TO USE :*\n")
	b.WriteString("*STEP 1 :\u2771 TYPE \u2770 " + prefix + "tempnumber +<COUNTRY CODE> \u2771*\n")
	b.WriteString("*STEP 2 :\u2771 PICK ANY NUMBER FROM THE LIST \u2014 TYPE JUST ITS NUMBER LIKE \u2770 3 \u2771*\n")
	b.WriteString("*STEP 3 :\u2771 READ OTP CODES LIVE \u2014 TYPE \u2770 " + prefix + "checknumber <NUMBER> \u2771*\n\n")
	b.WriteString(tnSep + "\n")
	b.WriteString("*\U0001f530 SUPPORTED COUNTRIES AND CODES :*\n")
	// format codes nicely, several per line
	var lines []string
	var cur string
	for _, c := range tnCountries {
		item := c.Name + " +" + c.Code
		if cur == "" {
			cur = item
		} else if len(cur)+3+len(item) <= 44 {
			cur += " \u2022 " + item
		} else {
			lines = append(lines, cur)
			cur = item
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	for _, l := range lines {
		b.WriteString("*" + l + "*\n")
	}
	b.WriteString("\n" + tnSep + "\n")
	b.WriteString("*EXAMPLES :*\n")
	b.WriteString("*\u2770 " + prefix + "tempnumber +91 \u2771 \u2014 INDIA NUMBERS*\n")
	b.WriteString("*\u2770 " + prefix + "tempnumber +44 \u2771 \u2014 UK NUMBERS*\n")
	b.WriteString("*\u2770 " + prefix + "tempnumber +1 \u2771 \u2014 USA NUMBERS*\n\n")
	b.WriteString("*NOTE :\u2771 NUMBERS ARE PUBLIC \u2014 EVERYONE CAN READ THE SMS, NEVER USE THEM FOR PRIVATE ACCOUNTS*\n\n")
	b.WriteString("*MORE :*\n*" + prefix + "checknumber \u2014 READ ANY NUMBER'S INBOX LIVE*")
	return b.String()
}

// tnCountryListCard — numbered list for .tempnumber +<code>.
func tnCountryListCard(prefix string, c tnCountry, nums []tnNumEntry) string {
	var b strings.Builder
	n := nums
	if len(n) > tnMaxNumbers {
		n = n[:tnMaxNumbers]
	}
	b.WriteString(fmt.Sprintf("*\U0001f530 FREE %s TEMP NUMBERS \U0001f530*\n", c.Name))
	b.WriteString(fmt.Sprintf("*CODE :+%s \u2022 TOTAL :%d NUMBERS*\n\n", c.Code, len(nums)))
	for i, e := range n {
		b.WriteString(tnSep + "\n")
		b.WriteString(fmt.Sprintf("*TYPE \u2770 %d \u2771 TO PICK THIS NUMBER*\n", i+1))
		b.WriteString("*" + e.Display + "*\n")
		b.WriteString(fmt.Sprintf("*SMS :%s \u2022 LAST :%s \u2022 USED FOR :%s*\n", e.Messages, e.Fresh, e.App))
	}
	b.WriteString("\n" + tnSep + "\n")
	b.WriteString("*PICK ANY ONE \u2014 JUST TYPE ITS NUMBER LIKE \u2770 3 \u2771 AND SEND*\n")
	b.WriteString("*YOU HAVE 2 MINUTES*\n")
	return b.String()
}

// tnInboxCard renders the latest SMS / OTP codes of a number.
func tnInboxCard(num string, smsList []tnInboxSMS, live bool) string {
	var b strings.Builder
	if live {
		b.WriteString("*\U0001f530 LIVE INBOX :\u2771 +" + num + "*\n\n")
	} else {
		b.WriteString("*\U0001f530 INBOX :\u2771 +" + num + "*\n\n")
	}
	n := smsList
	if len(n) > tnMaxMessages {
		n = n[:tnMaxMessages]
	}
	for _, sms := range n {
		line := "*\u276f FROM :\u2771 " + sms.From + "*\n*" + sms.Text + "*"
		if sms.OTP != "" {
			line += "\n*\U0001f530 CODE :\u2771 " + sms.OTP + "*"
		}
		b.WriteString(line + "\n\n")
	}
	if len(smsList) == 0 {
		b.WriteString("*NO SMS ON THIS NUMBER YET \u2014 TRY AGAIN AFTER A FEW MINUTES*\n\n")
	}
	b.WriteString("*\U0001f530 REFRESH :\u2771 TYPE THE COMMAND AGAIN*")
	return b.String()
}

// ── handlers ─────────────────────────────────────────────────────────────────

// handleTempNumber — .tempnumber → guide; .tempnumber +91 → country list.
func handleTempNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	arg := ""
	if len(args) > 0 {
		arg = strings.TrimSpace(strings.Join(args, " "))
	}
	if arg == "" {
		s.Reply(info, tnGuide(prefix))
		return
	}
	code := tnDigitsStr(arg)
	if code == "" {
		s.Reply(info, tnGuide(prefix))
		return
	}
	// find country by dial code
	var found []tnCountry
	for _, c := range tnCountries {
		if c.Code == code {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		var b strings.Builder
		b.WriteString("*\U0001f530 COUNTRY NOT SUPPORTED :\u2771 +" + code + "*\n\n")
		b.WriteString("*THIS COUNTRY HAS NO FREE NUMBERS RIGHT NOW*\n\n")
		b.WriteString(tnSep + "\n")
		b.WriteString("*\U0001f530 SUPPORTED COUNTRY CODES :*\n")
		for _, c := range tnCountries {
			b.WriteString("*+" + c.Code + " \u2014 " + c.Name + "*\n")
		}
		b.WriteString("\n*TRY \u2770 " + prefix + "tempnumber +91 \u2771 OR \u2770 " + prefix + "tempnumber +44 \u2771*")
		s.Reply(info, b.String())
		return
	}
	// multiple slugs share a code (us/ca both +1) — list them all
	var cards []string
	var allNums []tnNumEntry
	var chosen tnCountry
	for _, c := range found {
		nums, err := tnCountryNumbers(c.Slug)
		if err != nil {
			continue
		}
		if chosen.Slug == "" {
			chosen = c
		}
		allNums = append(allNums, nums...)
	}
	if len(allNums) == 0 {
		s.Reply(info, "*\U0001f530 TEMP NUMBER SERVICE DOWN :\u2771*\n\n*SITE NOT RESPONDING \u2014 TRY AGAIN IN A FEW MINUTES*")
		return
	}
	// prefer numbers with freshest "X minutes ago"
	sort.SliceStable(allNums, func(i, j int) bool {
		return tnFreshRank(allNums[i].Fresh) < tnFreshRank(allNums[j].Fresh)
	})
	cards = append(cards, tnCountryListCard(prefix, chosen, allNums))
	s.Reply(info, strings.Join(cards, "\n"))
	// store pick session for bare-number reply
	tnPickMu.Lock()
	tnPickList[s.GetJID()+"|"+info.Sender.String()] = &tnPickSess{
		Numbers: allNums[:min(tnMaxNumbers, len(allNums))],
		Country: chosen,
		Expiry:  time.Now().Add(tnPickWindow),
	}
	tnPickMu.Unlock()
}

// tnFreshRank — lower = fresher (seconds < minutes < hours < days < old).
func tnFreshRank(f string) int {
	switch {
	case strings.Contains(f, "second"):
		return 0
	case strings.Contains(f, "minute"):
		return 1
	case strings.Contains(f, "hour"):
		return 2
	case strings.Contains(f, "day"):
		return 3
	default:
		return 9
	}
}

// handleCheckNumber — LIVE inbox with 15s FETCHING auto-edit loop.
func handleCheckNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	num := tnDigitsStr(strings.Join(args, " "))
	if num == "" {
		s.Reply(info, "*\U0001f530 CHECK NUMBER \U0001f530*\n\n*TYPE \u2770 "+prefix+"checknumber <FULL NUMBER> \u2771*\n*EXAMPLE :\u2771 \u2770 "+prefix+"checknumber +917471735893 \u2771*\n\n*SHOWS THE LATEST SMS AND OTP CODES OF THAT NUMBER \u2014 LIVE FOR 15 SECONDS*")
		return
	}
	go tnLiveInbox(s, info, num, prefix)
}

// tnLiveInbox — 15s live fetch loop: FETCHING... animation via message edits,
// new-SMS detection each poll; at the end, latest codes are delivered.
func tnLiveInbox(s SessionBridge, info types.MessageInfo, num string, prefix string) {
	waitID := s.ReplyWithID(info, "*\U0001f530 FETCHING....*\n\n*LIVE READING +"+num+" \u2014 15 SECONDS*")
	if waitID == "" {
		// edit not possible — single fetch fallback
		if list, err := tnGetInbox(num); err == nil {
			s.Reply(info, tnInboxCard(num, list, true))
		} else {
			s.Reply(info, "*\U0001f530 NUMBER NOT FOUND :\u2771*\n\n*CHECK THE NUMBER AND TRY AGAIN \u2014 GET A FRESH ONE WITH "+prefix+"tempnumber*")
		}
		return
	}
	deadline := time.Now().Add(tnLiveSeconds * time.Second)
	frames := []string{"FETCHING....", "FETCHING.", "FETCHING..", "FETCHING..."}
	var best []tnInboxSMS
	frame := 0
	poll := 0
	for time.Now().Before(deadline) {
		// animate edit
		if waitID != "" {
			s.EditMessage(info, waitID, "*\U0001f530 "+frames[frame%len(frames)]+"*\n\n*LIVE READING +"+num+" \u2014 "+fmt.Sprintf("%d", tnLiveSeconds-int(time.Since(deadline.Add(-tnLiveSeconds*time.Second)).Seconds()))+"S LEFT*")
			frame++
		}
		// live poll (cache-busted fetch inside tnGetInbox)
		if list, err := tnGetInbox(num); err == nil && len(list) > 0 {
			best = list
		}
		poll++
		time.Sleep(3 * time.Second)
	}
	// final card (edit the FETCHING message into the result)
	if len(best) > 0 {
		s.EditMessage(info, waitID, tnInboxCard(num, best, true))
	} else {
		s.EditMessage(info, waitID, "*\U0001f530 INBOX EMPTY :\u2771 +"+num+"*\n\n*NO SMS FOUND IN 15 SECONDS \u2014 TRY AGAIN AFTER SOME TIME OR PICK ANOTHER NUMBER WITH "+prefix+"tempnumber*")
	}
	_ = poll
}

// tnDigits extracts only the digits from args (legacy helper kept).
func tnDigits(args []string) string {
	var b strings.Builder
	for _, a := range args {
		for _, r := range a {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// handleNewNumber / handleDelNumber — hidden aliases now map to guide+list.
func handleNewNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handleTempNumber(s, info, args, prefix)
}
func handleDelNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	handleTempNumber(s, info, args, prefix)
}

func init() {
	Register(Command{Name: "tempnumber", Category: "TOOLS", Desc: "FREE TEMPORARY PHONE NUMBERS OF MANY COUNTRIES WITH LIVE SMS OTP READING. TYPE .tempnumber TO SEE ALL SUPPORTED COUNTRY CODES.", Run: handleTempNumber})
	Register(Command{Name: "checknumber", Desc: "READ ANY TEMP NUMBER'S INBOX LIVE FOR 15 SECONDS WITH LATEST OTP CODES.", Category: "TOOLS", Hidden: true, Run: handleCheckNumber})
	Register(Command{Name: "delnumber", Desc: "DROP THE CURRENT NUMBER AND GET A NEW ONE.", Category: "TOOLS", Hidden: true, Run: handleDelNumber})
	Register(Command{Name: "newnumber", Desc: "GET A NEW TEMP NUMBER LIST.", Category: "TOOLS", Hidden: true, Run: handleNewNumber})
}
