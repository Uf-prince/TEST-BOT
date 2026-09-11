package goldcmds

// ============================================================================
// GOLD-MD — PAIR Command  (.pair + aliases)  —  SMART MULTI-SERVER EDITION
// File: paircmd.go
// ----------------------------------------------------------------------------
// User .pair likhta hai → bot:
//   1. INSTANT waiting msg ("searching free server...")
//   2. Background me servers.json ke sab servers /health se PARALLEL check
//      (max 2.5s — bot ki message speed pe 0% asar, sab async goroutine me)
//   3. Jo server ONLINE + jagah bachi (sessions < maxPerServer) — sabse kam
//      load wala — usse PAIR CODE le leta hai (pre-warm: user ko turant milta hai)
//   4. Waiting msg DELETE + sirf bare pair code ka msg bhejta hai
//   5. Pair code wala msg EK bar EDIT hota hai (same bare code — WhatsApp
//      native edit, tempmail wala BuildEdit pattern, random delay)
//   6. Uske bad PAIR CODE FULL GUIDE msg (ye kabhi edit/delete NAHI hota)
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// gmPairWaitingText — waiting message (delete hota hai code milne pe).
const gmPairWaitingText = "⌛ *GOLD-MD PAIRING...*\n\n_Searching free server for you, please wait..._ 🔄"

// gmPairGuideText — PAIR CODE FULL GUIDE (ye msg edit/delete NAHI hota —
// pair code wale msg ke thik bad alag msg me aata hai).
const gmPairGuideText = "*🔰 PAIR CODE FULL GUIDE 🔰*\n\n" +
	"1❯ COPY THE *CODE IMPORTANT ⚠️*\n" +
	"2 ❯ CLICK ON *WHATSAPP 3 DOTS*\n" +
	"3 ❯ CLICK ON *LINKED DEVICE*\n" +
	"4 ❯ CLICK ON *LINK WITH PAIR CODE*\n" +
	"5 ❯ PASTE *THE CODE*\n" +
	"6 ❯ IMPORTANT WHEN *LOGGING...... DON'T CLOSE WHATSAPP IMPORTANT ⚠️*\n" +
	"*7 ❯ WHEN LOGGING COMPLETE SIMPLY USE YOUR FREE BOT ✅*"

// per-user .pair cooldown (10 min) — .pair .pair .pair spam se servers pe
// load nahi parta, sirf 1 check per user per 10 min.
var (
	gmPairCooldownMu sync.Mutex
	gmPairCooldown   = map[string]time.Time{}
)

// gmPairPhoneFromArgs — args se digits-only phone nikalta hai
// (.pair 923xxxx → "923xxxx"). Koi number nahi diya → "".
func gmPairPhoneFromArgs(args []string) string {
	for _, a := range args {
		if a == "" {
			continue
		}
		if isAllDigits(a) {
			return a
		}
	}
	return ""
}

// (isAllDigits already package-wide: anti_common.go)

// handlePair — .pair entry point. Instant waiting msg + background heavy work.
func handlePair(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Waiting message INSTANT — bot ki speed pe 0% farak (heavy kaam sab neeche
	// background goroutine me hota hai).
	waitID := s.ReplyWithID(info, gmPairWaitingText)
	if waitID == "" {
		return // reply fail → kuch mat karo
	}
	go gmPairAsync(s, info, waitID, args)
}

// gmPairAsync — background: cooldown → phone → server check → pre-warm code
// → waiting delete → bare code send → ek bar edit → guide msg.
func gmPairAsync(s SessionBridge, info types.MessageInfo, waitID string, args []string) {
	// per-user 10-min cooldown.
	senderKey := info.Sender.ToNonAD().String()
	gmPairCooldownMu.Lock()
	if t, ok := gmPairCooldown[senderKey]; ok && time.Since(t) < 10*time.Minute {
		gmPairCooldownMu.Unlock()
		gmPairEditDelete(s, info, waitID, "⚠️ *You already used .pair — wait 10 minutes between requests.*")
		return
	}
	gmPairCooldown[senderKey] = time.Now()
	if len(gmPairCooldown) > 200 { // map infinite na bade — cleanup
		for k, t := range gmPairCooldown {
			if time.Since(t) > 10*time.Minute {
				delete(gmPairCooldown, k)
			}
		}
	}
	gmPairCooldownMu.Unlock()

	// Phone: user ne diya (.pair 923xxx) to wahi, warna sender ka apna number.
	phone := gmPairPhoneFromArgs(args)
	if phone == "" {
		senderJID := info.Sender.String()
		phone = strings.SplitN(senderJID, "@", 2)[0]
	}

	// STEP 1: servers.json load (5-min memory cache — 0ms re-read nahi).
	cfg, ok := gmLoadServersConfig()
	if !ok {
		gmPairEditDelete(s, info, waitID, "❌ *servers.json not found / broken — owner ko bolo.*")
		return
	}

	// STEP 2: free server select (online + jagah bachi, sabse kam load first).
	srv, err := gmPickFreeServer(cfg)
	if err != nil {
		gmPairEditDelete(s, info, waitID, "🔴 *ALL SERVERS FULL / OFFLINE.*\n\n_Try again after some time._")
		return
	}

	// STEP 3: pre-warm — selected server se PEHLE hi pair code le lo
	// (user ko bar-bar pair nahi karna parta — code ready milta hai).
	code, perr := gmFetchPairCode(srv.URL, phone)
	if perr != nil {
		gmPairEditDelete(s, info, waitID, "⚠️ *Pair code lene me problem aayi. Thodi der bad .pair try karo.*")
		return
	}

	// STEP 4: waiting msg DELETE + sirf bare pair code ka msg SEND
	// (user foran copy kar ke paste kar de).
	_ = s.DeleteMessage(info, waitID)

	codeMsgID := s.ReplyWithID(info, code)
	if codeMsgID == "" {
		return // send fail → guide bekaar
	}

	// STEP 5: pair code wala msg EK bar EDIT (same bare code) — tempmail ka
	// native BuildEdit pattern: random delay, best-effort, non-blocking.
	go func() {
		time.Sleep(gmRandomEditDelay())
		s.EditMessage(info, codeMsgID, code)
	}()

	// STEP 6: guide msg — EDIT/DELETE NAHI hota, pair code ke thik bad.
	time.Sleep(600 * time.Millisecond)
	s.ReplyWithID(info, gmPairGuideText)
}

// gmPairEditDelete — waiting msg delete karke error msg bhejna.
func gmPairEditDelete(s SessionBridge, info types.MessageInfo, waitID string, errText string) {
	_ = s.DeleteMessage(info, waitID)
	s.ReplyWithID(info, errText)
}

// ─────────────────────────────────────────────────────────────────────────────
// servers.json reader + health check + pick-free-server + pair code fetch
// (gold-cmds package ka APNA local zero-dep reader — panel.go wala sirf
//  panel HTTP ke liye hai. 5-min cache → har .pair pe 0ms file read.)
// ─────────────────────────────────────────────────────────────────────────────

type gmServerEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type gmServersConfig struct {
	MaxPerServer int             `json:"maxPerServer"`
	Servers      []gmServerEntry `json:"servers"`
}

var (
	gmSrvCfgMu    sync.Mutex
	gmSrvCfgCache *gmServersConfig
	gmSrvCfgAt    time.Time
)

// gmLoadServersConfig — servers.json load with 5-minute memory cache.
func gmLoadServersConfig() (*gmServersConfig, bool) {
	gmSrvCfgMu.Lock()
	defer gmSrvCfgMu.Unlock()
	if gmSrvCfgCache != nil && time.Since(gmSrvCfgAt) < 5*time.Minute {
		return gmSrvCfgCache, true
	}
	data, err := os.ReadFile("servers.json")
	if err != nil {
		return nil, false
	}
	var cfg gmServersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	if cfg.MaxPerServer <= 0 {
		cfg.MaxPerServer = 5
	}
	gmSrvCfgCache = &cfg
	gmSrvCfgAt = time.Now()
	return &cfg, true
}

// gmPickFreeServer — sab online servers /health PARALLEL check (2.5s timeout
// per server) → jo online + jagah bachi (sessions < maxPerServer) unme se
// SABSE KAM SESSIONS wala return (load balance; tie → config order SERVER 1).
func gmPickFreeServer(cfg *gmServersConfig) (gmServerEntry, error) {
	type result struct {
		idx      int
		sessions int
	}
	results := make([]result, 0, len(cfg.Servers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 2500 * time.Millisecond}

	for i, srv := range cfg.Servers {
		if strings.TrimSpace(srv.URL) == "" {
			continue // blank URL = server abhi set nahi hua (REPLACE-ME skip bhi)
		}
		if strings.Contains(srv.URL, "REPLACE-ME") {
			continue
		}
		wg.Add(1)
		go func(idx int, s gmServerEntry) {
			defer wg.Done()
			healthURL := strings.TrimRight(s.URL, "/") + "/health"
			resp, err := client.Get(healthURL)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			var hr struct {
				Sessions int `json:"sessions"`
			}
			if jerr := json.NewDecoder(resp.Body).Decode(&hr); jerr != nil {
				return
			}
			if hr.Sessions < cfg.MaxPerServer {
				mu.Lock()
				results = append(results, result{idx: idx, sessions: hr.Sessions})
				mu.Unlock()
			}
		}(i, srv)
	}
	wg.Wait()

	if len(results) == 0 {
		return gmServerEntry{}, fmt.Errorf("no free server")
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].sessions != results[b].sessions {
			return results[a].sessions < results[b].sessions
		}
		return results[a].idx < results[b].idx
	})
	return cfg.Servers[results[0].idx], nil
}

// gmFetchPairCode — selected server ke /pair API se pair code le aata hai
// (pre-warm: user ki request ane se PEHLE code ready rehta hai).
func gmFetchPairCode(serverURL, phone string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	payload, _ := json.Marshal(map[string]string{"phone": phone})
	url := strings.TrimRight(serverURL, "/") + "/pair"
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var pr struct {
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if jerr := json.NewDecoder(resp.Body).Decode(&pr); jerr != nil {
		return "", jerr
	}
	if pr.Code == "" {
		return "", fmt.Errorf("empty code (status %s)", pr.Status)
	}
	return pr.Code, nil
}

func init() {
	Register(Command{
		Name:     "pair",
		Category: "OWNER & SYSTEM",
		Desc:     "Get your pair code from the nearest free server",
		Run:      handlePair,
	})
	// hidden aliases — same smart flow
	Register(Command{Name: "get", Hidden: true, Run: handlePair})
	Register(Command{Name: "bot", Hidden: true, Run: handlePair})
	Register(Command{Name: "botlink", Hidden: true, Run: handlePair})
	Register(Command{Name: "linkbot", Hidden: true, Run: handlePair})
	Register(Command{Name: "repo", Hidden: true, Run: handlePair})
	Register(Command{Name: "script", Hidden: true, Run: handlePair})
}
