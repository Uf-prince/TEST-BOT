package main

// ============================================================================
// GOLD-MD — Telegram pairing bridge (Option 2: deep-link + gist HTML bridge)
//
// FLOW:
//   1. HTML page (public/static, e.g. GitHub Pages) pe user apna WhatsApp
//      number deta hai.
//   2. HTML t.me deep-link kholta hai (t.me/<bot>?start=<phone>) — user Telegram
//      chat mein /start <phone> send karta hai.
//   3. YE FILE (Go bot ke andar goroutine) getUpdates long-polling se message
//      uthati hai. Owner-only check. Phir mgr.PairWithCode(phone) chalati hai.
//   4. Pairing code Telegram chat mein reply hota hai AUR ek private GitHub
//      gist (paircode.json) mein likha jata hai:
//      { "<phone>": {"code": "ABCD-EFGH", "ts": 1697...} }
//   5. HTML page har 3s gist raw URL (usercontent with cache-bust) fetch
//      karta hai — jab apna phone milta hai to CODE page pe display hota hai.
//
// KYA KYA NEEDED:
//   • Telegram bot token (hardcoded TG_TOKEN — @BotFather se)
//   • Owner Telegram user ID (hardcoded TG_OWNER_ID — sirf isi ka /start chalega)
//   • GitHub token (hardcoded GH_TOKEN — gist update ke liye)
//   • Gist ID (hardcoded GIST_ID — HTML isi ka raw URL padhta hai)
//
// SECURITY: repo private hai; token owner-only lock ke saath safe — koi bhi
// doosra user /start kare to bot ignore karta hai.
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── hardcoded credentials (repo private — same policy as Storj shards) ──
const (
	tgToken   = "8909299641:AAGcVq7PavhpYNBsdV5eohvNdvTP7j_J3mM" // @gold_md_1_bot
	tgOwnerID = int64(8397200545)                                 // sirf ye ID pair kar sakti hai
	ghToken   = "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"        // gist update (paircode.json)
	gistID    = "0d24ea56e9559d94e7318c0c578befdf"                // HTML raw URL isi gist ka
)

const tgAPI = "https://api.telegram.org/bot" + tgToken

// telegramPairing holds the last pairing code sent to the gist (so the HTML
// bridge can read it even if Telegram chat is closed). Mirrored to gist.
var telegramPairing struct {
	mu     sync.Mutex
	phones map[string]pairEntry // phone -> latest code
}
type pairEntry struct {
	Code string `json:"code"`
	TS   int64  `json:"ts"`
	Msg  string `json:"msg,omitempty"`
}

// StartTelegramPairBridge launches the getUpdates long-poll goroutine.
// Called from main() after the manager is ready. Fails soft: agar Telegram
// unreachable ho to bot normal chalta rahega.
func StartTelegramPairBridge(mgr *Manager) {
	go telegramPairLoop(mgr)
	InfoLog("Telegram pair bridge → @gold_md_1_bot (owner-only, gist bridge ON)")
}

func telegramPairLoop(mgr *Manager) {
	offset := 0
	client := &http.Client{Timeout: 70 * time.Second}
	// backoff on errors
	backoff := time.Duration(3 * time.Second)

	for {
		url := fmt.Sprintf("%s/getUpdates?timeout=60&offset=%d&allowed_updates=[\"message\"]", tgAPI, offset)
		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 3 * time.Second

		var upd struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
				Message  *struct {
					MessageID int    `json:"message_id"`
					From      *struct {
						ID int64  `json:"id"`
					} `json:"from"`
					Text string `json:"text"`
				} `json:"message"`
			} `json:"result"`
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err := json.Unmarshal(b, &upd); err != nil || !upd.OK {
			time.Sleep(3 * time.Second)
			continue
		}

		for _, u := range upd.Result {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.From == nil {
				continue
			}
			// owner-only lock
			if u.Message.From.ID != tgOwnerID {
				continue
			}
			go handleTelegramPairCommand(mgr, u.Message.Text)
		}
	}
}

// handleTelegramPairCommand parses "/start <phone>" or "/pair <phone>".
func handleTelegramPairCommand(mgr *Manager, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// strip bot-mention suffix (/start@botname)
	if i := strings.IndexByte(text, '@'); i > 0 {
		text = text[:i]
	}
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	if cmd != "/start" && cmd != "/pair" {
		return
	}
	if len(fields) < 2 {
		tgSendMessage(tgOwnerID, "🔰 GIVE NUMBER LIKE: /pair 923012345678 (country code ke saath)")
		return
	}
	phone := digitsOnly(fields[1])
	if len(phone) < 10 {
		tgSendMessage(tgOwnerID, "❌ INVALID NUMBER — country code ke saath dein: /pair 923012345678")
		return
	}

	// limits — same policy as panel: max 3 paired sessions
	if mgr.AlreadyConnected(phone) {
		tgSendMessage(tgOwnerID, "✅ "+phone+" ALREADY CONNECTED — code ki zaroorat nahi.")
		return
	}
	if mgr.Count() >= 3 {
		tgSendMessage(tgOwnerID, "⚠️ MAX 3 PAIRING REACHED — pehle kisi session ko delete karo.")
		return
	}

	tgSendMessage(tgOwnerID, "⏳ GENERATING CODE FOR "+phone+" ...")
	code, err := mgr.PairWithCode(phone)
	if err != nil {
		tgSendMessage(tgOwnerID, "❌ PAIRING ERROR: "+err.Error())
		return
	}

	msg := "🔰 *GOLD-MD PAIRING CODE* 🔰\n\n*NUMBER:* " + phone + "\n*CODE:* `" + code + "`\n\n" +
		"WhatsApp → Linked Devices → Link with phone number instead → code daalo\n\n" +
		"_(HTML page pe bhi aa gaya hai)_"
	tgSendMessage(tgOwnerID, msg)

	// gist update — HTML bridge ke liye
	telegramPairing.mu.Lock()
	if telegramPairing.phones == nil {
		telegramPairing.phones = map[string]pairEntry{}
	}
	telegramPairing.phones[phone] = pairEntry{Code: code, TS: time.Now().Unix()}
	telegramPairing.mu.Unlock()
	gistWritePairCodes()
}

// gistWritePairCodes pushes the current pairing map to the private gist.
func gistWritePairCodes() {
	telegramPairing.mu.Lock()
	data, _ := json.MarshalIndent(telegramPairing.phones, "", "  ")
	telegramPairing.mu.Unlock()

	body := map[string]any{
		"description": "GOLD-MD pair codes (HTML bridge)",
		"files": map[string]any{
			"paircode.json": map[string]any{
				"content": string(data),
			},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", "https://api.github.com/gists/"+gistID, bytes.NewReader(raw))
	req.Header.Set("Authorization", "token "+ghToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ErrLog("gist write failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		ErrLog("gist write HTTP %d: %s", resp.StatusCode, string(b))
	}
}

// tgSendMessage sends a plain text message (Markdown enabled).
func tgSendMessage(chatID int64, text string) {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
		"parse_mode": "Markdown",
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", tgAPI+"/sendMessage", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
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

// TGPairBridgeInfo is used by /health to show bridge status (nice-to-have).
func TGPairBridgeInfo() string {
	telegramPairing.mu.Lock()
	n := len(telegramPairing.phones)
	telegramPairing.mu.Unlock()
	return "tg_bridge=on pairs=" + strconv.Itoa(n)
}
