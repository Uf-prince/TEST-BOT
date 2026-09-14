package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// GOLD-MD — SESSION ONLINE-ELSEWHERE GUARD (owner order)
// File: session_online_guard.go
// ============================================================================
// OWNER ORDER: "jab guard Storj session check kare (kya ye session WhatsApp
// me logged in hai ya logged out) — agar session KISI AUR SERVER pe already
// ONLINE hai (koi aur server usko chala raha hai) to use IGNORE kar do,
// reconnect karne ki koi zaroorat nahi. Aur agar session OFFLINE hai magar
// logged in hai (Storj me session pada hai, WhatsApp ne logout nahi bola)
// to PEHLE jaisa hi reconnect wala kaam wapas chalne do."
//
// Matlab: LOCAL reconnect attempt sirf TAB start karo jab session kahin bhi
// online NAHI ho. Jo session already kisi aur server pe zinda hai usko
// haath mat lagao — double-connect war WhatsApp logout karva deta hai.
//
// CHECK ORDER (sasta pehle, mehanga baad me):
//   1. CLAIM CHECK (authoritative + free): goldmd:fleet:claim:<jid> me
//      koi AUR live server ka fresh claim hai? (fleetHeldByLiveServer —
//      heartbeat/probe-based). Zinda hai = wahi owner, hum ignore.
//   2. REMOTE ONLINE PROBE (fallback truth): claim nahi/mara hua, magar
//      server asal me us session ko chala raha hai (boot-restore ya manual
//      pair hua tha, claim ghayab). servers.json ki har server ke
//      /sessions ko probe karo — us JID ke liye online:true mila = wo
//      session WAQAI dusre server pe online hai = IGNORE.
//
// Dono checks sirf BACKGROUND guard paths (AutoLoad boot goroutines +
// fleet watchdog) se call hote hain — message/command hot path pe kabhi
// nahi. Probe wave ek baar me ~4-8s leta hai (parallel, 4s HTTP timeout
// per server) — background me bilkul theek.
//
// SILENT FILE: InfoLog/WarnLog/OkLog no-op hain (logger.go) — console pe
// kuch nahi chhapta, sirf debug hook ke liye rakhe gaye hain.
// ============================================================================

// guardProbeTimeout: ek remote /sessions request ka HTTP timeout (4s —
// fleetHTTPTimeout ke barabar; STOPPED Render server isse pehle hi
// DNS/connect fail pe mur jata hai).
const guardProbeTimeout = 4 * time.Second

// guardSessionsSeen maps a remote /sessions payload. (panel.go ka local
// /sessions iska local version hai — fields identical.)
type guardSessionsSeen struct {
	Sessions []struct {
		JID    string `json:"jid"`
		Owner  string `json:"owner"`
		Online bool   `json:"online"`
	} `json:"sessions"`
	Count int `json:"count"`
}

// fleetSessionOnlineElsewhere: kya ye JID kisi AUR server pe ONLINE hai?
//
// ORDER:
//  1. fleetHeldByLiveServer(jid) — claim-based (cheap, authoritative)
//  2. remote /sessions probe wave (expensive fallback — sirf tab jab
//     claim check keh de "free/unknown")
//
// Background paths (AutoLoad goroutine / fleet watchdog) se hi call karo.
func fleetSessionOnlineElsewhere(jid string) bool {
	// STEP 1 — claim check (free + authoritative): koi aur live server
	// is JID ka fresh claim rakhta hai → wo chala raha hai, hum ignore.
	if fleetHeldByLiveServer(jid) {
		return true
	}

	// STEP 2 — remote probe (fallback): claim ghayab/mara hua, magar
	// asal server us session ko chala raha hai. servers.json me se apne
	// aap ko (fleetSelfID) chhod kar har server ke /sessions dekho.
	user := fleetUserPart(jid)
	if user == "" {
		return false
	}
	target := user + "@s.whatsapp.net" // base JID form (panel /sessions isi me deta hai)

	servers := guardRemoteServers()
	if len(servers) == 0 {
		return false // fleet inactive / list empty — kuch nahi kar sakte
	}

	found := false
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, sid := range servers {
		if sid == fleetSelfID {
			continue
		}
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			defer func() { _ = recover() }() // probe kabhi panic na kare
			if guardRemoteSessionOnline(sid, target) {
				mu.Lock()
				found = true
				mu.Unlock()
			}
		}(sid)
	}
	wg.Wait()
	return found
}

// guardRemoteServers returns the remote server ID/URL list from
// servers.json (fleetScanAll isi list ko /health se probe karta hai).
// Fleet off / config missing → nil (guard ka probe step skip).
func guardRemoteServers() []string {
	if !fleetActive() {
		return nil
	}
	loadServersConfig()
	cfg := serversCfg
	out := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if s.URL == "" {
			continue
		}
		out = append(out, strings.TrimRight(s.URL, "/"))
	}
	return out
}

// guardRemoteSessionOnline probes one server's /sessions endpoint and
// reports whether the target JID is online there. Single attempt, 4s
// timeout — offline/STOPPED server false deta hai (network error/dead).
func guardRemoteSessionOnline(serverURL, jid string) bool {
	if serverURL == "" || jid == "" {
		return false
	}
	cl := &http.Client{Timeout: guardProbeTimeout}
	resp, err := cl.Get(serverURL + "/sessions")
	if err != nil {
		return false
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var seen guardSessionsSeen
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&seen); err != nil {
		return false
	}
	for _, s := range seen.Sessions {
		if !s.Online {
			continue
		}
		// exact base-JID match (panel local JID bhi base form me hai)
		if s.JID == jid {
			return true
		}
	}
	return false
}
