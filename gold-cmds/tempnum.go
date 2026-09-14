package goldcmds

// ============================================================================
// GOLD-MD — .tempnumber command (free temporary numbers, AnonymSMS)
// File: tempnum.go
// ----------------------------------------------------------------------------
// Free unlimited temporary phone numbers with live SMS inboxes — no API
// key, no registration, no payment.  Data comes live from AnonymSMS
// (anonymsms.com), a public temp-number site:
//
//   /temporary-phone-number/            → list of all active numbers
//   /number/<digits>/                   → that number's SMS inbox
//   /wp-json/phone-numbers/v1/random    → jump to a random number
//     (needs a Referer header from any /number/ page, else 403)
//
// COMMANDS:
//   .tempnumber                → fresh temp number (menu entry — the only
//                                one visible; the others are guided here)
//   .checknumber <number>      → read that number's inbox (hidden alias)
//   .delnumber <number>        → drop the saved number, get a NEW one
//                                (hidden alias) — "delete + new generate"
//   .newnumber                 → random fresh number (hidden alias)
//
// AnonymSMS has no official REST API, so this scrapes the same HTML a
// browser renders.  A 5-minute cache keeps it light on the site.
// ============================================================================

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── AnonymSMS endpoints ───────────────────────────────────────────────

const (
	tnBaseURL     = "https://anonymsms.com"
	tnListPath    = "/temporary-phone-number/"
	tnNumPath     = "/number/%s/"
	tnRandomAPI   = "/wp-json/phone-numbers/v1/random?current=%s"
	tnUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	tnCacheTTL    = 5 * time.Minute
	tnHTTPTimeout = 20 * time.Second
	tnMaxMessages = 10 // max SMS shown per inbox
	tnMaxNumbers  = 40 // max numbers listed in one reply
)

// ── in-process cache (5-minute TTL) ──────────────────────────────────

var (
	tnMu    sync.Mutex
	tnCache []string
	tnAt    time.Time
)

// ── HTTP fetch helpers ────────────────────────────────────────────────

func tnFetch(path string, referer string) (string, error) {
	client := &http.Client{Timeout: tnHTTPTimeout}
	req, err := http.NewRequest("GET", tnBaseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", tnUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", tnBaseURL+referer)
	}
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

// ── parsing ───────────────────────────────────────────────────────────

var (
	tnNumberHrefRe = regexp.MustCompile(`(?:https://anonymsms\.com)?/number/([0-9]+)/`)
	tnTitleNumRe   = regexp.MustCompile(`<title>\s*\+?([0-9]{7,15})`)
	tnH1NumRe      = regexp.MustCompile(`(?s)<h1[^>]*>\s*\+?([0-9]{7,15})`)
	tnRowRe        = regexp.MustCompile(`(?s)<tr[^>]*data-message[^>]*>(.*?)</tr>`)
	tnCellRe       = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	tnTagRe        = regexp.MustCompile(`<[^>]+>`)
	tnOTPRe        = regexp.MustCompile(`\b(\d{4,8})\b`)
	tnCountryRe    = regexp.MustCompile(`(?s)<h1[^>]*>[^<]*</h1>\s*<[^>]*>\s*([A-Za-z][A-Za-z ()&.'-]{2,30})`)
)

// tnNumbersCached returns the live number list (cached 5 minutes).
func tnNumbersCached(force bool) ([]string, error) {
	tnMu.Lock()
	defer tnMu.Unlock()
	if !force && len(tnCache) > 0 && time.Since(tnAt) < tnCacheTTL {
		return tnCache, nil
	}
	html, err := tnFetch(tnListPath, "")
	if err != nil {
		return nil, err
	}
	list := tnParseNumbers(html)
	if len(list) == 0 {
		return nil, fmt.Errorf("no numbers found")
	}
	tnCache = list
	tnAt = time.Now()
	return list, nil
}

// tnParseNumbers extracts unique numbers from any AnonymSMS page.
func tnParseNumbers(html string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range tnNumberHrefRe.FindAllStringSubmatch(html, -1) {
		num := m[1]
		if len(num) >= 7 && !seen[num] {
			seen[num] = true
			out = append(out, num)
		}
	}
	return out
}

// tnPageNumber returns the number a /number/ page is about (title/H1).
func tnPageNumber(html string) string {
	if m := tnTitleNumRe.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	if m := tnH1NumRe.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	return ""
}

// tnInboxSMS is one SMS row in a number's inbox.
type tnInboxSMS struct {
	From, Text, Date, OTP string
}

// tnParseInbox extracts SMS rows from a /number/ page.
func tnParseInbox(html string) []tnInboxSMS {
	var out []tnInboxSMS
	for _, row := range tnRowRe.FindAllStringSubmatch(html, -1) {
		var cells []string
		for _, cell := range tnCellRe.FindAllStringSubmatch(row[1], -1) {
			c := tnTagRe.ReplaceAllString(cell[1], " ")
			c = strings.Join(strings.Fields(c), " ")
			c = strings.TrimSpace(c)
			cells = append(cells, c)
		}
		// rows with data-message have 3 cells: From / Text / Date
		if len(cells) >= 3 && cells[0] != "" && cells[1] != "" {
			otp := ""
			if m := tnOTPRe.FindStringSubmatch(cells[1]); m != nil {
				otp = m[1]
			}
			out = append(out, tnInboxSMS{From: cells[0], Text: cells[1], Date: cells[2], OTP: otp})
		}
	}
	return out
}

// tnGetInbox fetches and parses the inbox of a number.
func tnGetInbox(num string) ([]tnInboxSMS, string, error) {
	html, err := tnFetch(fmt.Sprintf(tnNumPath, num), "")
	if err != nil {
		return nil, "", err
	}
	return tnParseInbox(html), tnPageNumber(html), nil
}

// tnGetRandom uses the site's own random endpoint (Referer required) to
// land on a fresh random number page.
func tnGetRandom() (string, string, error) {
	html, err := tnFetch(fmt.Sprintf(tnRandomAPI, "0"), "/temporary-phone-number/")
	if err != nil {
		return "", "", err
	}
	num := tnPageNumber(html)
	if num == "" {
		return "", "", fmt.Errorf("random page unreadable")
	}
	return num, html, nil
}

// ── reply builders ────────────────────────────────────────────────────

// tnCard renders a temp-number reply with country + how to use it.
func tnCard(num, html string, prefix string) string {
	country := ""
	if m := tnCountryRe.FindStringSubmatch(html); m != nil {
		country = strings.TrimSpace(m[1])
	}
	var b strings.Builder
	b.WriteString("*🔰 FREE TEMP NUMBER 🔰*\n\n")
	b.WriteString("*NUMBER :❱ +" + num + "*\n")
	if country != "" {
		b.WriteString("*COUNTRY :❱ " + strings.ToUpper(country) + "*\n")
	}
	b.WriteString("\n*🔰 READ THE SMS :❱*\n*" + prefix + "checknumber +" + num + "*\n\n")
	b.WriteString("*🔰 WANT A DIFFERENT ONE :❱*\n*" + prefix + "newnumber*\n\n")
	b.WriteString("*🔰 MORE :❱*\n*" + prefix + "delnumber ❮ NUMBER ❯ — DROP A NUMBER AND GET A NEW ONE*\n*" + prefix + "tempnumber — FRESH TEMP NUMBER*")
	return b.String()
}

// tnInboxCard renders the latest SMS / OTP codes of a number.
func tnInboxCard(num string, smsList []tnInboxSMS, prefix string) string {
	var b strings.Builder
	b.WriteString("*🔰 INBOX :❱ +" + num + "*\n\n")
	n := smsList
	if len(n) > tnMaxMessages {
		n = n[:tnMaxMessages]
	}
	for _, sms := range n {
		line := "*❯ FROM :❱ " + sms.From + "*\n*" + sms.Text + "*"
		if sms.OTP != "" {
			line += "\n*🔰 CODE :❱ " + sms.OTP + "*"
		}
		b.WriteString(line + "\n\n")
	}
	if len(smsList) == 0 {
		b.WriteString("*NO SMS ON THIS NUMBER YET — TRY AGAIN AFTER A FEW MINUTES*\n\n")
	}
	b.WriteString("*🔰 REFRESH :❱ " + prefix + "checknumber +" + num + "*")
	return b.String()
}

// tnGuide — full 🔰 styled guide (pure English, menu entry reply).
func tnGuide(prefix string) string {
	return "*🔰 TEMP NUMBER GUIDE 🔰*\n\n" +
		"*🔰 GET A FRESH TEMP NUMBER :❱*\n*" + prefix + "tempnumber*\n*ONE TAP — A NEW FREE TEMP NUMBER EVERY TIME, NO API KEY, NO LOGIN, NO PAYMENT*\n\n" +
		"*🔰 READ THE SMS OF A NUMBER :❱*\n*" + prefix + "checknumber ❮ NUMBER ❯*\n*EXAMPLE :❱ " + prefix + "checknumber +447884641162*\n*SHOWS THE LATEST SMS AND OTP CODES OF THAT NUMBER*\n\n" +
		"*🔰 DROP A NUMBER AND GET A NEW ONE :❱*\n*" + prefix + "delnumber ❮ NUMBER ❯*\n*THE NUMBER IS DROPPED AND A FRESH ONE IS GENERATED IN ITS PLACE*\n\n" +
		"*🔰 RANDOM NEW NUMBER :❱*\n*" + prefix + "newnumber*\n*JUMPS TO A RANDOM FRESH NUMBER INSTANTLY*\n\n" +
		"*🔰 NOTE :❱*\n*NUMBERS ARE PUBLIC — EVERYONE CAN READ THE SMS, SO NEVER USE THEM FOR PRIVATE ACCOUNTS*\n\n" +
		"*🔰 FULL COMMAND LIST :❱*\n*" + prefix + "tempnumber — FRESH TEMP NUMBER*\n*" + prefix + "checknumber — READ THE INBOX*\n*" + prefix + "delnumber — DROP AND REGENERATE*\n*" + prefix + "newnumber — RANDOM NEW NUMBER*"
}

// ── handlers ──────────────────────────────────────────────────────────

// handleTempNumber — fresh temp number (menu entry).
func handleTempNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	num, html, err := tnGetRandom()
	if err != nil {
		s.Reply(info, "*🔰 TEMP NUMBER SERVICE DOWN :❱*\n\n*SITE NOT RESPONDING — TRY AGAIN IN A FEW MINUTES*")
		return
	}
	s.Reply(info, tnCard(num, html, prefix))
}

// handleCheckNumber — read a number's inbox.
func handleCheckNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	num := tnDigits(args)
	if num == "" {
		s.Reply(info, tnGuide(prefix))
		return
	}
	smsList, _, err := tnGetInbox(num)
	if err != nil {
		s.Reply(info, "*🔰 NUMBER NOT FOUND :❱*\n\n*CHECK THE NUMBER AND TRY AGAIN — GET A FRESH ONE WITH "+prefix+"tempnumber*")
		return
	}
	s.Reply(info, tnInboxCard(num, smsList, prefix))
}

// handleDelNumber — drop the number, generate a fresh one.
func handleDelNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// old number (if any) is simply dropped — public site numbers are
	// server-managed, "delete" means forget it and pull a new one.
	num, html, err := tnGetRandom()
	if err != nil {
		s.Reply(info, "*🔰 TEMP NUMBER SERVICE DOWN :❱*\n\n*SITE NOT RESPONDING — TRY AGAIN IN A FEW MINUTES*")
		return
	}
	var dropped string
	if len(args) > 0 {
		dropped = tnDigits(args)
	}
	var b strings.Builder
	b.WriteString("*🔰 OLD NUMBER DROPPED :❱ " + prefix + "tempnumber*\n")
	if dropped != "" {
		b.WriteString("*DROPPED :❱ +" + dropped + "*\n")
	}
	b.WriteString("\n")
	b.WriteString(tnCard(num, html, prefix))
	s.Reply(info, b.String())
}

// tnDigits extracts only the digits from a string.
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

// handleNewNumber — random fresh number (hidden alias).
func handleNewNumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	num, html, err := tnGetRandom()
	if err != nil {
		s.Reply(info, "*🔰 TEMP NUMBER SERVICE DOWN :❱*\n\n*SITE NOT RESPONDING — TRY AGAIN IN A FEW MINUTES*")
		return
	}
	s.Reply(info, tnCard(num, html, prefix))
}

func init() {
	Register(Command{Name: "tempnumber", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CREATE A TEMPORARY PHONE NUMBER.", Run: handleTempNumber})
	Register(Command{Name: "checknumber", Desc: "THIS COMMAND IS USED TO CHECK IF YOUR TEMPORARY NUMBER IS STILL ACTIVE.", Category: "TOOLS", Hidden: true, Run: handleCheckNumber})
	Register(Command{Name: "delnumber", Desc: "THIS COMMAND IS USED TO DELETE YOUR TEMPORARY PHONE NUMBER.", Category: "TOOLS", Hidden: true, Run: handleDelNumber})
	Register(Command{Name: "newnumber", Desc: "THIS COMMAND IS USED TO CREATE A NEW TEMPORARY PHONE NUMBER.", Category: "TOOLS", Hidden: true, Run: handleNewNumber})
}
