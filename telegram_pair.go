package main

// ============================================================================
// GOLD-MD — Pairing bridge (ntfy STREAM relay + gist results, Telegram notify)
//
// FLOW (100% browser → page pe code, koi Telegram chat kholna NAHI):
//   1. HTML page (permanent static URL) pe user number deta hai.
//   2. HTML ntfy.sh topic pe POST karta hai: {"phone":"923..","ts":...}
//      (tokenless, CORS-open — koi token HTML me nahi hai).
//   3. YE FILE (Go bot) ntfy se EK STREAMING connection hold karta hai —
//      har request instant milti hai (koi polling rate-limit issue nahi,
//      ntfy.sh free = 60 req/hr hai, 3s polling usse tod deti thi).
//   4. Request mili → checks (max 3, already connected) → PairWithCode.
//   5. Result GIST (paircode.json) me likha jata hai:
//        {"923..":{"code":"ABCD-EFGH","ts":...}}   (ya {"error":"..."})
//   6. HTML paircode.json poll karke CODE PAGE PE dikha deta hai.
//   7. BONUS: owner ko Telegram chat me bhi code chala jata hai.
// ============================================================================

import (
	"bufio"
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
	tgToken   = "8909299641:AAGcVq7PavhpYNBsdV5eohvNdvTP7j_J3mM" // @gold_md_1_bot (optional notify)
	tgOwnerID = int64(8397200545)                                 // owner Telegram ID
	ghToken   = "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"        // gist read/write
	gistID    = "0d24ea56e9559d94e7318c0c578befdf"                // paircode gist
)

const gistRawBase = "https://gist.githubusercontent.com/Uf-prince/" + gistID + "/raw/"
const tgAPI = "https://api.telegram.org/bot" + tgToken

// pairState — in-memory mirror of the results gist.
var pairState struct {
	mu     sync.Mutex
	phones map[string]pairEntry
}
type pairEntry struct {
	Code  string `json:"code"`
	TS    int64  `json:"ts"`
	Error string `json:"error,omitempty"`
}

var lastReqTS int64 // dedupe: last seen request ts (ms)

// StartPairBridge launches the ntfy stream + Telegram long-poll (notify).
func StartPairBridge(mgr *Manager) {
	go ntfyStreamLoop(mgr)
	go telegramNotifyLoop(mgr)
	InfoLog("Pair bridge → ntfy STREAM in + gist results out; TG notify @gold_md_1_bot")
}

// ── ntfy STREAM (HTML → bot) — 1 persistent connection, rate-limit-proof ──
const ntfyTopic = "goldmd-pair-relay" // HTML POST karta hai, bot stream sunta hai

// ntfyStreamLoop holds ONE long-lived streaming connection to ntfy.
// ntfy.sh free tier = 60 requests/hour per IP. 3s polling (1200/hr) broke
// that quota and got the container IP rate-limited/banned — the exact bug
// that froze the pair page at "GENERATING...". A single held-open stream
// is 1 request; with since=<ts> catch-up on reconnect nothing is missed.
func ntfyStreamLoop(mgr *Manager) {
	// 12 min client timeout → connection recycles ~5 req/hr (well under 60).
	client := &http.Client{Timeout: 12 * time.Minute}
	since := time.Now().Add(-5 * time.Minute).Unix()
	backoff := 5 * time.Second

	for {
		url := fmt.Sprintf("https://ntfy.sh/%s/json?since=%d", ntfyTopic, since)
		resp, err := client.Get(url)
		if err != nil {
			WarnLog("ntfy stream connect fail: %v — retry in %s", err, backoff)
			time.Sleep(backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		if resp.StatusCode == 429 {
			// Rate-limited/banned (purane 3s-polling binary ki wajah se ho sakta
			// hai) — cool-down dedo, phir stream pakad lenge.
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			WarnLog("ntfy 429 (rate-limit/ban): %s — 60s cool-down", string(b))
			time.Sleep(60 * time.Second)
			continue
		}
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			WarnLog("ntfy stream HTTP %d: %s — retry", resp.StatusCode, string(b))
			time.Sleep(10 * time.Second)
			continue
		}
		backoff = 5 * time.Second
		InfoLog("ntfy stream connected (topic=%s since=%d)", ntfyTopic, since)

		// Read NDJSON events as they arrive (connection open rehti hai).
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var ev struct {
				Event string `json:"event"`
				Time  int64  `json:"time"`
				Msg   string `json:"message"`
			}
			if json.Unmarshal([]byte(line), &ev) != nil || ev.Event != "message" {
				continue
			}
			if ev.Time > since {
				since = ev.Time
			}
			var req struct {
				Phone string `json:"phone"`
				TS    int64  `json:"ts"`
			}
			if json.Unmarshal([]byte(ev.Msg), &req) != nil || req.Phone == "" || req.TS == 0 {
				continue
			}
			if req.TS <= lastReqTS { // duplicate — skip
				continue
			}
			lastReqTS = req.TS
			InfoLog("Pair bridge: naya pairing request → %s", digitsOnly(req.Phone))
			go handlePairRequest(mgr, digitsOnly(req.Phone))
		}
		resp.Body.Close()
		// Stream toot gaya (timeout/reconnect) — since= catch-up ke saath
		// turant dobara jod do. Miss kuch nahi hoga.
		time.Sleep(3 * time.Second)
	}
}

// handlePairRequest — checks + PairWithCode + result gist write (+ TG notify).
func handlePairRequest(mgr *Manager, phone string) {
	if len(phone) < 10 {
		writePairResult(phone, "", "INVALID NUMBER — country code ke saath dein")
		return
	}
	if mgr.AlreadyConnected(phone) {
		writePairResult(phone, "", "ALREADY CONNECTED — code ki zaroorat nahi")
		return
	}
	if mgr.Count() >= 3 {
		writePairResult(phone, "", "MAX 3 PAIRING REACHED — pehle delete karo")
		return
	}
	code, err := mgr.PairWithCode(phone)
	if err != nil {
		writePairResult(phone, "", "ERROR: "+err.Error())
		tgNotify("❌ PAIR "+phone+" ERROR: "+err.Error())
		return
	}
	writePairResult(phone, code, "")
	tgNotify("🔓 PAIR CODE " + phone + ": `" + code + "` (page pe bhi aa gaya)")
}

// writePairResult updates the in-memory map + pushes paircode.json to the gist.
func writePairResult(phone, code, errMsg string) {
	pairState.mu.Lock()
	if pairState.phones == nil {
		pairState.phones = map[string]pairEntry{}
	}
	pairState.phones[phone] = pairEntry{Code: code, TS: time.Now().Unix(), Error: errMsg}
	data, _ := json.MarshalIndent(pairState.phones, "", "  ")
	pairState.mu.Unlock()
	gistSetFile("paircode.json", string(data))
}

// gistSetFile PATCHes one file inside the gist.
func gistSetFile(name, content string) {
	body := map[string]any{
		"files": map[string]any{
			name: map[string]any{"content": content},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", "https://api.github.com/gists/"+gistID, bytes.NewReader(raw))
	req.Header.Set("Authorization", "token "+ghToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ErrLog("gist write %s failed: %v", name, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		ErrLog("gist write %s HTTP %d: %s", name, resp.StatusCode, string(b))
	}
}

// ── Telegram long-poll (owner ke liye optional /pair command + notify) ──
func telegramNotifyLoop(mgr *Manager) {
	offset := 0
	client := &http.Client{Timeout: 70 * time.Second}
	backoff := 3 * time.Second
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
					From *struct {
						ID int64 `json:"id"`
					} `json:"from"`
					Text string `json:"text"`
				} `json:"message"`
			} `json:"result"`
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if json.Unmarshal(b, &upd) != nil || !upd.OK {
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range upd.Result {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.From == nil || u.Message.From.ID != tgOwnerID {
				continue // owner-only
			}
			go tgHandleCommand(mgr, u.Message.Text)
		}
	}
}

func tgHandleCommand(mgr *Manager, text string) {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '@'); i > 0 {
		text = text[:i]
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])
	if cmd != "/start" && cmd != "/pair" {
		return
	}
	if len(fields) < 2 {
		tgNotify("🔓 /pair 923012345678 — country code ke saath")
		return
	}
	handlePairRequest(mgr, digitsOnly(fields[1]))
}

func tgNotify(text string) {
	payload := map[string]any{"chat_id": tgOwnerID, "text": text, "parse_mode": "Markdown"}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", tgAPI+"/sendMessage", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	if resp, err := client.Do(req); err == nil {
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

// PairBridgeInfo — /health ke liye.
func PairBridgeInfo() string {
	pairState.mu.Lock()
	n := len(pairState.phones)
	pairState.mu.Unlock()
	return "pair_bridge=on results=" + strconv.Itoa(n)
}
