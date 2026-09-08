package goldcmds

// ============================================================================
// GOLD-MD — Temp mail system (Guerrilla Mail JSON API)
// File: tempmail.go
// ============================================================================
// Ported from UMAR-MD plugins/tempmail.js — SAME WORK (0% farak):
//   .tempmail — get a throwaway inbox (one per sender, stored locally)
//   .checkmail — see what's arrived (first 5 mails, full body visible)
//   .newmail — throw the old address away, get a fresh one
//   .delmail — delete the address for good
//
// Guerrilla Mail JSON API — no API key, no account creation. Every call
// needs f=<function>, ip, agent, and (after the first call) the PHPSESSID
// cookie so the server knows which session/mailbox we're talking about.
//
// Send-then-edit-strip trick (UmarSendThenStripFooter): the address is
// sent like a normal reply (footer included), then shortly after, edited
// down to just the bare address via WhatsApp's native edit
// (protocolMessage type 14 / BuildEdit) — footer gone. The edit delay is
// randomized each time (not a fixed pattern).
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const (
	gmBase    = "http://api.guerrillamail.com/ajax.php"
	gmIP      = "127.0.0.1"
	gmAgent   = "Mozilla/5.0 (TempMailBot)"
	gmTimeout = 15 * time.Second
	gmRetries = 3
	gmDBDir   = "nexstore"
	gmDBFile  = "tempmail.json"
)

// gmSession is one temp-mail session per WhatsApp sender.
type gmSession struct {
	Address   string `json:"address"`
	SID       string `json:"sid"`
	CreatedAt int64  `json:"createdAt"`
}

var (
	gmMu   sync.Mutex
	gmHTTP = &http.Client{Timeout: gmTimeout}
)

// ── local JSON storage (one temp mail session per sender) ───────────────────

func gmDBPath() string {
	// Prefer the bot's runtime data dir; fall back to ./nexstore next to
	// the binary (the same layout the Node bot used with data/).
	return filepath.Join(gmDBDir, gmDBFile)
}

func gmLoadDB() map[string]gmSession {
	gmMu.Lock()
	defer gmMu.Unlock()
	out := map[string]gmSession{}
	data, err := os.ReadFile(gmDBPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func gmSaveDB(db map[string]gmSession) {
	gmMu.Lock()
	defer gmMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(gmDBPath()), 0o755)
	out, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(gmDBPath(), out, 0o644)
}

// ── Guerrilla Mail API helpers ──────────────────────────────────────────────

// gmFetch calls the Guerrilla Mail AJAX API. Returns the parsed JSON and
// the (possibly rotated) PHPSESSID from the response cookies.
func gmFetch(fn string, params map[string]string, sid string) (map[string]any, string, error) {
	vals := url.Values{}
	vals.Set("f", fn)
	vals.Set("ip", gmIP)
	vals.Set("agent", gmAgent)
	for k, v := range params {
		vals.Set(k, v)
	}
	reqURL := gmBase + "?" + vals.Encode()

	var lastErr error
	for attempt := 1; attempt <= gmRetries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, reqURL, nil)
		if err != nil {
			return nil, sid, err
		}
		if sid != "" {
			req.Header.Set("Cookie", "PHPSESSID="+sid)
		}
		res, err := gmHTTP.Do(req)
		if err != nil {
			lastErr = err
			if attempt < gmRetries {
				time.Sleep(time.Duration(800*attempt) * time.Millisecond)
			}
			continue
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", res.StatusCode)
			if attempt < gmRetries {
				time.Sleep(time.Duration(800*attempt) * time.Millisecond)
			}
			continue
		}

		// Pull the (possibly new/rotated) session id out of the cookies.
		newSid := sid
		for _, c := range res.Header.Values("Set-Cookie") {
			if m := strings.SplitN(c, "PHPSESSID=", 2); len(m) == 2 {
				v := strings.SplitN(m[1], ";", 2)[0]
				if v != "" {
					newSid = v
				}
			}
		}

		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, newSid, err
		}
		return data, newSid, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Guerrilla Mail request failed.")
	}
	return nil, sid, lastErr
}

// gmCreateTempAccount starts a brand-new session + mailbox.
func gmCreateTempAccount() (gmSession, error) {
	data, sid, err := gmFetch("get_email_address", map[string]string{"lang": "en"}, "")
	if err != nil {
		return gmSession{}, err
	}
	addr, _ := data["email_addr"].(string)
	if addr == "" {
		return gmSession{}, fmt.Errorf("No Guerrilla Mail address available right now.")
	}
	return gmSession{
		Address:   addr,
		SID:       sid,
		CreatedAt: time.Now().UnixMilli(),
	}, nil
}

// gmMail is one inbox entry from check_email.
type gmMail struct {
	ID      string
	From    string
	Subject string
	Excerpt string
}

func gmFetchInbox(sid string) ([]gmMail, error) {
	data, _, err := gmFetch("check_email", map[string]string{"seq": "0"}, sid)
	if err != nil {
		return nil, err
	}
	list, _ := data["list"].([]any)
	out := make([]gmMail, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["mail_id"].(string)
		from, _ := m["mail_from"].(string)
		subject, _ := m["mail_subject"].(string)
		excerpt, _ := m["mail_excerpt"].(string)
		out = append(out, gmMail{
			ID:      id,
			From:    from,
			Subject: subject,
			Excerpt: excerpt,
		})
	}
	return out, nil
}

// gmFetchMessageBody returns the full body of one mail (fetch_email).
func gmFetchMessageBody(sid, emailID string) (string, error) {
	data, _, err := gmFetch("fetch_email", map[string]string{"email_id": emailID}, sid)
	if err != nil {
		return "", err
	}
	body, _ := data["mail_body"].(string)
	return body, nil
}

// gmForgetAddress asks the server to forget the mailbox (best-effort).
func gmForgetAddress(sid, address string) {
	_, _, _ = gmFetch("forget_me", map[string]string{"email_addr": address}, sid)
}

// ── send, then native-edit down to just the address ─────────────────────────

// gmEditDelaysMs is the randomized edit-delay pool (Node: 1500,2300,600,
// 100,1400 ms — not a fixed pattern).
var gmEditDelaysMs = []int{1500, 2300, 600, 100, 1400}

func gmRandomEditDelay() time.Duration {
	return time.Duration(gmEditDelaysMs[rand.Intn(len(gmEditDelaysMs))]) * time.Millisecond
}

// gmSendThenStripFooter sends the address like a normal reply (footer
// included), then shortly after edits that same message down to just the
// bare address via WhatsApp's native edit (BuildEdit = protocolMessage
// type 14) — footer gone, exactly like the Node.js version.
func gmSendThenStripFooter(s SessionBridge, info types.MessageInfo, address string) {
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		return
	}

	// Send like a normal reply — footer included, so it looks the same
	// as every other message going out (Node: the footer is injected by
	// the sendMessage patch). ReplyWithID returns the message ID needed
	// for the later edit.
	msgID := s.ReplyWithID(info, address)
	if msgID == "" {
		return
	}

	// Random delay, then one edit that rewrites that same message down
	// to just the bare address — footer gone (Node: raw relayMessage
	// with protocolMessage type 14, which bypasses the footer wrapper).
	// Non-blocking like the Node setTimeout — the caller's info reply is
	// NOT delayed by the edit.
	go func() {
		time.Sleep(gmRandomEditDelay())
		edited := cli.BuildEdit(info.Chat, types.MessageID(msgID), &waProto.Message{
			Conversation: proto.String(address),
		})
		if edited == nil {
			return
		}
		// Best-effort — failure ignored (the original send already stands).
		_, _ = cli.SendMessage(context.Background(), info.Chat, edited)
	}()
}

// ── sender identity ─────────────────────────────────────────────────────────

// gmSenderKey returns the per-sender storage key (Node: m.sender ||
// participant || remoteJid).
func gmSenderKey(info types.MessageInfo) string {
	if !info.Sender.IsEmpty() {
		return info.Sender.ToNonAD().String()
	}
	if !info.SenderAlt.IsEmpty() {
		return info.SenderAlt.ToNonAD().String()
	}
	return info.Chat.ToNonAD().String()
}

// ── command handlers ────────────────────────────────────────────────────────

func handleTempMail(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTempMailAsync(s, info, args, prefix)
}

func handleTempMailAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	sender := gmSenderKey(info)
	db := gmLoadDB()

	account, isNew := db[sender]
	if !isNew {
		var err error
		account, err = gmCreateTempAccount()
		if err != nil {
			s.Reply(info, "❌ Couldn't create a temp mail: "+err.Error())
			return
		}
		db[sender] = account
		gmSaveDB(db)
	}

	// 1) address sent normally, then auto-edited down to just the
	// address (footer stripped).
	gmSendThenStripFooter(s, info, account.Address)

	if isNew {
		s.Reply(info, "📧 *YOUR TEMP MAIL IS READY*\n`"+account.Address+"`\n\n"+
			"*What this is:* a throwaway inbox. Any email sent to this address can be read here in this chat — you don't need Gmail, Outlook, etc.\n\n"+
			"*What it's good for:* signing up on random sites, getting one-time OTP/verification codes, avoiding spam on your real email.\n\n"+
			"*What it's NOT for:* banking, social media recovery, anything important or long-term — this address can disappear (see below), so don't rely on it for accounts you actually care about.\n\n"+
			"*Commands:*\n"+
			"➤ *checkmail* — see what's arrived in the inbox\n"+
			"➤ *newmail* — throw this one away, get a fresh address\n"+
			"➤ *delmail* — delete this address for good\n\n"+
			"⚠️ _If nobody checks mail on this address for 60 minutes straight, it expires automatically and any mail in it is lost. Just running *checkmail* keeps it alive._")
	} else {
		s.Reply(info, "📧 *THIS IS ALREADY YOUR ACTIVE TEMP MAIL*\n`"+account.Address+"`\n\n"+
			"You already had one, so it wasn't recreated. Type *newmail* if you want to throw this away and get a different address instead.")
	}
}

func handleNewMail(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleNewMailAsync(s, info, args, prefix)
}

func handleNewMailAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	sender := gmSenderKey(info)
	db := gmLoadDB()

	old, hadOld := db[sender]
	if hadOld {
		gmForgetAddress(old.SID, old.Address)
	}

	account, err := gmCreateTempAccount()
	if err != nil {
		s.Reply(info, "❌ Couldn't create a new temp mail: "+err.Error())
		return
	}
	db[sender] = account
	gmSaveDB(db)

	// 1) address sent normally, then auto-edited down to just the
	// address (footer stripped).
	gmSendThenStripFooter(s, info, account.Address)

	replacedLine := "This is your first temp mail — nothing was replaced."
	if hadOld {
		replacedLine = "Your old address is now dead — any mail sent to it is gone and can't be recovered."
	}
	s.Reply(info, "♻️ *NEW TEMP MAIL GENERATED*\n`"+account.Address+"`\n\n"+
		replacedLine+"\n\n"+
		"Use *checkmail* to see what arrives here, or *newmail* again anytime for another fresh one.")
}

func handleCheckMail(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleCheckMailAsync(s, info, args, prefix)
}

func handleCheckMailAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	sender := gmSenderKey(info)
	db := gmLoadDB()
	account, ok := db[sender]
	if !ok {
		s.Reply(info, "⚠️ *NO TEMP MAIL YET*\n\n"+
			"You haven't generated an address for yourself. Type *tempmail* first to get one, then come back and run *checkmail* to see what's arrived.")
		return
	}

	messages, err := gmFetchInbox(account.SID)
	if err != nil {
		s.Reply(info, "❌ Couldn't check inbox: "+err.Error())
		return
	}

	if len(messages) == 0 {
		s.Reply(info, "📭 *No mail yet* for:\n`"+account.Address+"`\n\n"+
			"Nothing's arrived at this address so far. This is normal right after signing up somewhere — it can take a minute or two for the email to come through. Go sign up/verify with this address if you haven't yet, then run *checkmail* again in a bit.")
		return
	}

	// header message first
	s.Reply(info, "📬 *INBOX* ("+itoa(len(messages))+" total) — `"+account.Address+"`")

	toShow := messages
	if len(toShow) > 5 {
		toShow = toShow[:5]
	}
	for i, msg := range toShow {
		fromAddr := msg.From
		if fromAddr == "" {
			fromAddr = "Unknown sender"
		}
		subject := msg.Subject
		if subject == "" {
			subject = "(no subject)"
		}
		preview := msg.Excerpt
		// fetch full body for every message so OTPs/links are visible
		if body, ferr := gmFetchMessageBody(account.SID, msg.ID); ferr == nil && body != "" {
			preview = body
		}
		preview = gmStripHTML(preview)
		preview = gmCollapseSpaces(preview)
		if len(preview) > 500 {
			preview = preview[:500]
		}

		// each mail sent as its own message, so it's easy to
		// scroll/copy individually
		s.Reply(info, "*Mail "+itoa(i+1)+"/"+itoa(len(toShow))+"*\n"+
			"From: "+fromAddr+"\n"+
			"Subject: "+subject+"\n\n"+
			orString(preview, "(no preview available)"))
	}

	if len(messages) > len(toShow) {
		s.Reply(info, "…and "+itoa(len(messages)-len(toShow))+" more. Type *checkmail* again later to see them as older ones clear.")
	}
}

func handleDelMail(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDelMailAsync(s, info, args, prefix)
}

func handleDelMailAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	sender := gmSenderKey(info)
	db := gmLoadDB()
	account, ok := db[sender]
	if !ok {
		s.Reply(info, "⚠️ *NOTHING TO DELETE*\n\n"+
			"You don't currently have a temp mail address. Type *tempmail* to create one.")
		return
	}

	gmForgetAddress(account.SID, account.Address)
	delete(db, sender)
	gmSaveDB(db)

	s.Reply(info, "🗑️ *DELETED*\n`"+account.Address+"`\n\n"+
		"This address is gone for good — any mail still sitting in it is lost, and it can't be reused. Type *tempmail* whenever you want a new one.")
}

// ── small helpers ───────────────────────────────────────────────────────────

// gmStripHTML removes HTML tags (Node: replace(/<[^>]+>/g, ' ')).
func gmStripHTML(s string) string {
	var b strings.Builder
	depth := false
	for _, r := range s {
		if r == '<' {
			depth = true
			b.WriteRune(' ')
			continue
		}
		if r == '>' {
			depth = false
			continue
		}
		if !depth {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// gmCollapseSpaces collapses runs of whitespace (Node: replace(/\s+/g,' ')).
func gmCollapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// orString mirrors the Node.js `preview || '(no preview available)'`.
func orString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// ── registration ────────────────────────────────────────────────────────────
func init() {
	Register(Command{
		Name:     "tempmail",
		Category: "AI & MEDIA",
		Desc:     "Get a throwaway temp-mail inbox (Guerrilla Mail). One address per user, kept alive by checkmail. Aliases: .checkmail .newmail .delmail",
		Hidden:   true, // works silently — menu me sirf short alias .tmail dikhta hai
		Run:      handleTempMail,
	})
	Register(Command{
		Name:     "checkmail",
		Category: "AI & MEDIA",
		Desc:     "Check your temp-mail inbox — first 5 mails with full body (OTPs/links visible).",
		Hidden:   true, // works silently — menu me sirf short alias .cmail dikhta hai
		Run:      handleCheckMail,
	})
	Register(Command{
		Name:     "delmail",
		Category: "AI & MEDIA",
		Desc:     "Delete your temp-mail address for good — all mail in it is lost.",
		Hidden:   true, // works silently — menu me sirf short alias .dmail dikhta hai
		Run:      handleDelMail,
	})
	Register(Command{
		Name:     "newmail",
		Category: "AI & MEDIA",
		Desc:     "Throw the current temp mail away and generate a fresh address.",
		Hidden:   true, // works silently — menu me sirf short alias .nmail dikhta hai
		Run:      handleNewMail,
	})

	// ── short aliases (visible in menu, same work — 0% farak) ──
	Register(Command{
		Name:     "tmail",
		Category: "AI & MEDIA",
		Desc:     "Get a throwaway temp-mail inbox (Guerrilla Mail). Long form .tempmail also works",
		Run:      handleTempMail,
	})
	Register(Command{
		Name:     "cmail",
		Category: "AI & MEDIA",
		Desc:     "Check your temp-mail inbox — first 5 mails with full body. Long form .checkmail also works",
		Run:      handleCheckMail,
	})
	Register(Command{
		Name:     "dmail",
		Category: "AI & MEDIA",
		Desc:     "Delete your temp-mail address for good. Long form .delmail also works",
		Run:      handleDelMail,
	})
	Register(Command{
		Name:     "nmail",
		Category: "AI & MEDIA",
		Desc:     "Generate a fresh temp-mail address. Long form .newmail also works",
		Run:      handleNewMail,
	})
}
