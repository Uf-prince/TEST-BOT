package goldcmds

// ============================================================================
// GOLD-MD — PAIR Command  (.pair + aliases)  —  SMART MULTI-SERVER v2
// File: paircmd.go
// ----------------------------------------------------------------------------
// FLOW (owner fresh design — koi DELETE nahi, sirf EDIT):
//
//   .pair                (bina number) → GUIDANCE msg:
//       *DO YOU NEED THE PAIR CODE ?* ... {prefix}PAIR 923XXXXX ...
//       + isi waqt BACKGROUND PRE-WARM start (jugad): sender ka number
//         already pata hai → server select + pair code PEHLE se le liya
//         jata hai, 90s cache me. Jab user .pair 923xxx likhega to code
//         INSTANT milta hai (0 network wait).
//
//   .pair 923xxxxxxx     (number ke sath) → 2 alag msg INSTANT:
//       MSG1: *GETTING PAIR CODE*
//       MSG2: *PLEASE WAIT........*
//       Background: pre-warm cache check → hit? instant. Miss? jitne bhi
//       servers servers.json me hain sab /health check → sabse kam load
//       FREE server se pair code fetch.
//       Complete hone pe:
//       MSG1: edit → sirf BARE PAIR CODE (user foran copy-paste kare)
//       MSG2: edit → PAIR CODE FULL GUIDE (3 dots → linked device → ...)
//       Error pe:
//       MSG1: edit → ❌ error
//       MSG2: edit → retry line
//
// SERVERS DYNAMIC: background health poller har 60s servers.json ko RE-READ
// karta hai → jitne naye servers owner dalta jaye, .pair unko khud check
// karta rahega. Koi hard limit nahi (5, 10, 20 — sab chalega).
//
// SPEED: sab kuch async goroutine me — bot ke message reply pe 0% asar.
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

// ── message texts (owner exact) ─────────────────────────────────────────────

// gmPairGettingText — MSG1 (edit ho kar BARE PAIR CODE ban jata hai).
const gmPairGettingText = "*GETTING PAIR CODE*"

// gmPairWaitText — MSG2 (edit ho kar FULL GUIDE ban jata hai).
const gmPairWaitText = "*PLEASE WAIT........*"

// gmPairGuideText — MSG2 ka edit target (PAIR CODE FULL GUIDE).
const gmPairGuideText = "*🔰 PAIR CODE FULL GUIDE 🔰*\n\n" +
	"1❯ COPY THE *CODE IMPORTANT ⚠️*\n" +
	"2 ❯ CLICK ON *WHATSAPP 3 DOTS*\n" +
	"3 ❯ CLICK ON *LINKED DEVICE*\n" +
	"4 ❯ CLICK ON *LINK WITH PAIR CODE*\n" +
	"5 ❯ PASTE *THE CODE*\n" +
	"6 ❯ IMPORTANT WHEN *LOGGING...... DON'T CLOSE WHATSAPP IMPORTANT ⚠️*\n" +
	"*7 ❯ WHEN LOGGING COMPLETE SIMPLY USE YOUR FREE BOT ✅*"

// gmPairGuidanceText — .pair bina number pe ye guidance (prefix runtime lagta hai).
const gmPairGuidanceText = "*DO YOU NEED THE PAIR CODE ?*\n\n" +
	"*TYPE SAME LIKE THAT*\n" +
	"*%sPAIR 923XXXXX*\n\n" +
	"TYPE YOUR NUMBER WITH YOUR OWN COUNTRY CODE WITHOUT + SIGN WITHOUT 0 TYPE WITH COUNTRY CODE SAME TYPE FULL NUMBER 923XXXXXX"

// ── state ───────────────────────────────────────────────────────────────────

// per-user 90s cooldown (sirf LIVE fetch pe — cache hit pe nahi).
var (
	gmPairCooldownMu sync.Mutex
	gmPairCooldown   = map[string]time.Time{}
)

// pre-warm cache: phone → {code, at} — 90s TTL (WhatsApp pair code window).
// "request ane se pehle pair code ready" ka jugaad — guidance msg aane pe
// hi fetch shuru, user typing ke dauran code ready ho jata hai.
type gmPreWarmEntry struct {
	code string
	at   time.Time
}

var (
	gmPreWarmMu sync.Mutex
	gmPreWarm   = map[string]gmPreWarmEntry{}
)

// health cache: poller har 60s refresh — .pair server select 0ms.
type gmHealthEntry struct {
	name     string
	url      string
	online   bool
	sessions int
	at       time.Time
}

var (
	gmHealthMu    sync.Mutex
	gmHealthCache []gmHealthEntry
)

// gmPreWarmTTL / gmHealthTTL — cache windows.
const (
	gmPreWarmTTL      = 90 * time.Second
	gmHealthTTL       = 75 * time.Second
	gmPairCooldownTTL = 90 * time.Second
)

// ── entry point ─────────────────────────────────────────────────────────────

// handlePair — .pair router: bina number → guidance + pre-warm trigger;
// number ke sath → 2 waiting msgs + background work.
func handlePair(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	phone := gmPairPhoneFromArgs(args)

	// ── .pair (bina number) → GUIDANCE ──
	if phone == "" {
		s.Reply(info, fmt.Sprintf(gmPairGuidanceText, prefix))

		// JUGAD: pre-warm abhi start kar do — sender ka number JID me pata hai.
		// User guide padhne/typing me 10-30s leta hai — utne me code ready.
		if sender := gmSenderPhone(info); sender != "" {
			go gmPreWarmFetch(sender)
		}
		return
	}

	// ── .pair 923xxxx → 2 INSTANT waiting msgs, phir background ──
	msg1 := s.ReplyWithID(info, gmPairGettingText)
	msg2 := s.ReplyWithID(info, gmPairWaitText)
	if msg1 == "" && msg2 == "" {
		return
	}
	go gmPairAsync(s, info, msg1, msg2, phone)
}

// ── background worker ───────────────────────────────────────────────────────

// gmPairAsync — cache check → live fetch → MSG1/MSG2 edit (koi delete NAHI).
func gmPairAsync(s SessionBridge, info types.MessageInfo, msg1, msg2, phone string) {
	// 1) PRE-WARM CACHE HIT? → instant (user ne pehle .pair likha tha,
	//    code typing ke daaran ready ho chuka).
	if code, ok := gmPreWarmGet(phone); ok {
		gmPairFinish(s, info, msg1, msg2, code)
		return
	}

	// 2) LIVE FETCH — 90s per-user cooldown (network cost bachane ke liye).
	senderKey := info.Sender.ToNonAD().String()
	gmPairCooldownMu.Lock()
	if t, ok := gmPairCooldown[senderKey]; ok && time.Since(t) < gmPairCooldownTTL {
		gmPairCooldownMu.Unlock()
		gmPairEditBoth(s, info, msg1, msg2,
			"⚠️ *Wait 90 seconds — server pe request already chal rahi hai.*",
			"_Thodi der bad phir se .pair "+phone+" likho._")
		return
	}
	gmPairCooldown[senderKey] = time.Now()
	if len(gmPairCooldown) > 300 {
		for k, t := range gmPairCooldown {
			if time.Since(t) > gmPairCooldownTTL {
				delete(gmPairCooldown, k)
			}
		}
	}
	gmPairCooldownMu.Unlock()

	// 3) servers.json load → free server select → pair code fetch.
	cfg, ok := gmLoadServersConfig()
	if !ok {
		gmPairEditBoth(s, info, msg1, msg2,
			"❌ *servers.json missing / broken — owner ko bolo.*",
			"_File me servers ke links dale jayenge tab .pair chalega._")
		return
	}

	srv, err := gmPickFreeServer(cfg)
	if err != nil {
		gmPairEditBoth(s, info, msg1, msg2,
			"🔴 *ALL SERVERS FULL / OFFLINE.*",
			"_Thodi der bad try karo — owner naye servers add karta jata hai._")
		return
	}

	code, perr := gmFetchPairCode(srv.URL, phone)
	if perr != nil {
		gmPairEditBoth(s, info, msg1, msg2,
			"⚠️ *Pair code lene me problem aayi.*",
			"_Thodi der bad phir se .pair "+phone+" likho._")
		return
	}

	// 4) SUCCESS — dono msg EDIT (fresh ban jate hain).
	gmPairFinish(s, info, msg1, msg2, code)
}

// gmPairFinish — MSG1 → bare code, MSG2 → full guide. (edit only, no delete)
func gmPairFinish(s SessionBridge, info types.MessageInfo, msg1, msg2, code string) {
	s.EditMessage(info, msg1, code) // bare pair code — foran copy-paste
	time.Sleep(700 * time.Millisecond)
	s.EditMessage(info, msg2, gmPairGuideText)
}

// gmPairEditBoth — error case: dono msgs edit karke bata do.
func gmPairEditBoth(s SessionBridge, info types.MessageInfo, msg1, msg2, t1, t2 string) {
	if msg1 != "" {
		s.EditMessage(info, msg1, t1)
	}
	time.Sleep(500 * time.Millisecond)
	if msg2 != "" {
		s.EditMessage(info, msg2, t2)
	}
}

// ── pre-warm jugaad ─────────────────────────────────────────────────────────

// gmPreWarmFetch — guidance msg ke waqt call hota hai: server select + pair
// code fetch BACKGROUND me (user typing ke dauran). 90s cache me store.
func gmPreWarmFetch(phone string) {
	// pehle se fresh cache hai? skip (single-flight).
	if _, ok := gmPreWarmGet(phone); ok {
		return
	}
	// in-flight lock: dobara trigger na ho.
	gmPreWarmMu.Lock()
	if e, ok := gmPreWarm[phone]; ok && time.Since(e.at) < gmPreWarmTTL {
		gmPreWarmMu.Unlock()
		return
	}
	gmPreWarm[phone] = gmPreWarmEntry{code: "", at: time.Now()} // placeholder = in-flight
	gmPreWarmMu.Unlock()

	cfg, ok := gmLoadServersConfig()
	if !ok {
		return
	}
	srv, err := gmPickFreeServer(cfg)
	if err != nil {
		gmPreWarmMu.Lock()
		delete(gmPreWarm, phone) // fail → placeholder hatao, agli baar retry
		gmPreWarmMu.Unlock()
		return
	}
	code, err := gmFetchPairCode(srv.URL, phone)
	if err != nil {
		gmPreWarmMu.Lock()
		delete(gmPreWarm, phone)
		gmPreWarmMu.Unlock()
		return
	}
	gmPreWarmMu.Lock()
	gmPreWarm[phone] = gmPreWarmEntry{code: code, at: time.Now()}
	// map cleanup — 200+ entries pe purani hatao.
	if len(gmPreWarm) > 200 {
		for k, e := range gmPreWarm {
			if time.Since(e.at) > gmPreWarmTTL {
				delete(gmPreWarm, k)
			}
		}
	}
	gmPreWarmMu.Unlock()
}

// gmPreWarmGet — fresh (non-placeholder, non-expired) cached code?
func gmPreWarmGet(phone string) (string, bool) {
	gmPreWarmMu.Lock()
	defer gmPreWarmMu.Unlock()
	e, ok := gmPreWarm[phone]
	if !ok || e.code == "" || time.Since(e.at) > gmPreWarmTTL {
		return "", false
	}
	return e.code, true
}

// ── helpers ─────────────────────────────────────────────────────────────────

// gmPairPhoneFromArgs — args se digits-only phone (.pair 923xxxx → 923xxxx).
// (isAllDigits package-wide: anti_common.go)
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

// gmSenderPhone — info se sender ka number (digits, JID se).
func gmSenderPhone(info types.MessageInfo) string {
	jid := info.Sender.String()
	if jid == "" {
		return ""
	}
	return strings.SplitN(jid, "@", 2)[0]
}

// ── servers.json (dynamic — jitne servers dalo, sab check hote rahenge) ────

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

// gmLoadServersConfig — servers.json load, 60s cache (naye servers jaldi
// uthane ke liye 60s — owner link dale to 1 min me .pair use karne lage).
func gmLoadServersConfig() (*gmServersConfig, bool) {
	gmSrvCfgMu.Lock()
	defer gmSrvCfgMu.Unlock()
	if gmSrvCfgCache != nil && time.Since(gmSrvCfgAt) < 60*time.Second {
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

// gmServerUsable — blank / REPLACE-ME URL skip.
func gmServerUsable(s gmServerEntry) bool {
	u := strings.TrimSpace(s.URL)
	return u != "" && !strings.Contains(u, "REPLACE-ME")
}

// ── health poller (background — servers HAMESHA ready) ─────────────────────

// init — poller start: turant 1 cycle + phir har 60s. Sirf HTTP + file read,
// bridge ki zaroorat nahi → package init se safe.
func init() {
	go func() {
		defer func() { recover() }()
		for {
			gmRefreshHealth()
			time.Sleep(60 * time.Second)
		}
	}()
}

// gmRefreshHealth — sab usable servers /health PARALLEL check → cache.
func gmRefreshHealth() {
	defer func() { recover() }()
	cfg, ok := gmLoadServersConfig()
	if !ok {
		return
	}
	usable := make([]gmServerEntry, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if gmServerUsable(s) {
			usable = append(usable, s)
		}
	}
	if len(usable) == 0 {
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	entries := make([]gmHealthEntry, len(usable))
	var wg sync.WaitGroup
	for i, srv := range usable {
		wg.Add(1)
		go func(idx int, s gmServerEntry) {
			defer wg.Done()
			e := gmHealthEntry{name: s.Name, url: s.URL, at: time.Now()}
			resp, err := client.Get(strings.TrimRight(s.URL, "/") + "/health")
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var hr struct {
						Sessions int `json:"sessions"`
					}
					if json.NewDecoder(resp.Body).Decode(&hr) == nil {
						e.online = true
						e.sessions = hr.Sessions
					}
				}
			}
			entries[idx] = e
		}(i, srv)
	}
	wg.Wait()
	gmHealthMu.Lock()
	gmHealthCache = entries
	gmHealthMu.Unlock()
}

// gmPickFreeServer — cache fresh hai to 0ms select; warna live check.
// Sabse KAM sessions wala online server (load balance; tie → config order).
func gmPickFreeServer(cfg *gmServersConfig) (gmServerEntry, error) {
	gmHealthMu.Lock()
	cacheFresh := len(gmHealthCache) > 0 && time.Since(gmHealthCache[0].at) < gmHealthTTL
	entries := append([]gmHealthEntry(nil), gmHealthCache...)
	gmHealthMu.Unlock()

	if !cacheFresh {
		gmRefreshHealth()
		gmHealthMu.Lock()
		entries = append([]gmHealthEntry(nil), gmHealthCache...)
		gmHealthMu.Unlock()
	}

	type pick struct {
		srv      gmServerEntry
		sessions int
	}
	free := make([]pick, 0, len(entries))
	for _, e := range entries {
		if e.online && e.sessions < cfg.MaxPerServer {
			free = append(free, pick{srv: gmServerEntry{Name: e.name, URL: e.url}, sessions: e.sessions})
		}
	}
	if len(free) == 0 {
		return gmServerEntry{}, fmt.Errorf("no free server")
	}
	sort.Slice(free, func(a, b int) bool { return free[a].sessions < free[b].sessions })
	return free[0].srv, nil
}

// gmFetchPairCode — selected server ke /pair API se pair code.
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

// ── command registration ────────────────────────────────────────────────────

func init() {
	Register(Command{
		Name:     "pair",
		Category: "OWNER & SYSTEM",
		Desc:     "THIS COMMAND IS USED TO SHOW THE BOT PAIRING LINK AND DETAILS.",
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
