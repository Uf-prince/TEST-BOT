// ============================================================================
//   GOLD-MD — MULTI-HOP VERIFY CHAIN (owner order, Request 11)
//
//   OWNER ORDER (Roman Urdu, verbatim intent):
//     "Ab 3 verifyer guards or lagao. Reconnect jaha kahi se kisi server ko
//      bheja ja raha ho — kisi session ko ek ke pass aaye wo TEST kare: kya
//      WAQAI session online:false hai? WhatsApp se verify kare. WAQAI online
//      false bola lekin logged-in hai to agle ke pass bheje. Agar WhatsApp ne
//      bola LOGGED OUT to foran session delete Storj se bhi, local folder se
//      bhi. Agar logged-in hai (status false dekh raha) to agle ke pass bhej
//      de — wo SAME verify kare, phir agle ke pass bheje — wo SAME verify
//      kare — phir ja kar kisi server pe session reconnect ho. Taake bar bar
//      connect/disconnect/restart na ho, WhatsApp ko spam na lage."
//
//   ── KAISE KAAM KARTA HAI ────────────────────────────────────────────────
//   Dispatch chain ab MULTI-HOP hai. Har hop pe server:
//     1. WhatsApp-truth verify (agar session object yahan hai):
//          • logged-in  → "online"      → chain STOP (kuch nahi).
//          • logged-out → "logged_out"  → Storj + folder PURGE, chain STOP.
//     2. online-elsewhere guard (claim + /sessions probe) → "online_elsewhere".
//     3. slot full → "full" (chain aage badhti hai).
//     4. warna → agle server ko FORWARD (hop+1) — SAME verify wahan.
//   Jab koi server LAST hop pe pahunche (ya koi free server mile) → wahan
//   session reconnect ho jata hai ("connected").
//
//   Isse: ek hi session ke liye bar-bar connect/disconnect/restart nahi hota
//   (har hop verify karta hai), aur WhatsApp ko spam nahi lagta.
//
//   BANDWIDTH (Render 5GB): hop cap (3) + per-hop 75s budget + dispatch lock
//   (thundering herd rok) + visited-list (loop rok). Sirf genuinely-offline
//   JID ke liye chalti hai — hot-path pe 0% asar.
// ============================================================================

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// vcMaxHops: chain ki max depth (owner: 3 verify guards → 3 hops).
	vcMaxHops = 3
	// vcHopWait: ek hop ka HTTP timeout.
	vcHopWait = 75 * time.Second
	// vcTruthWindow: WhatsApp-truth verify window (per hop).
	vcTruthWindow = 3 * time.Second
)

// vcHopRequest: ek hop ka request body.
type vcHopRequest struct {
	JID        string   `json:"jid"`
	DispatchID string   `json:"dispatch_id"`
	Hop        int      `json:"hop"`
	Origin     string   `json:"origin"`
	Visited    []string `json:"visited"` // loop-rok: jo servers dekh chuke
}

// vcHopResponse: ek hop ka jawab.
type vcHopResponse struct {
	Status      string `json:"status"` // connected / online_elsewhere / logged_out / full / busy / error
	JID         string `json:"jid"`
	Server      string `json:"server"`
	Hop         int    `json:"hop"`
	FinalServer string `json:"final_server,omitempty"`
}

// vcHopVerify: is server pe WhatsApp-truth verify (owner: "whatsapp se verify
// kare"). Returns:
//
//	"online"     → session logged-in hai (chain STOP).
//	"logged_out" → WhatsApp explicit logout (purge ho gaya, chain STOP).
//	"offline"    → session object nahi / transient (chain aage badhe).
func vcHopVerify(m *Manager, jid string) string {
	if m == nil {
		return "offline"
	}
	m.mu.Lock()
	sess, ok := m.sessions[jid]
	m.mu.Unlock()
	if !ok || sess == nil || sess.Client == nil {
		return "offline"
	}
	verified, explicitLogout, _ := whatsappTruthVerified(sess, vcTruthWindow)
	if verified {
		return "online"
	}
	if explicitLogout {
		swPurgeLoggedOut(m, jid) // owner: logged out → foran delete (Storj + folder)
		return "logged_out"
	}
	return "offline"
}

// vcNextHop: agla candidate server (self-skip + visited-skip — loop rok).
func vcNextHop(m *Manager, visited []string) *dispatchTarget {
	skip := map[string]bool{}
	for _, v := range visited {
		if v != "" {
			skip[v] = true
		}
	}
	if s := swSelfURL(); s != "" {
		skip[s] = true
	}
	cands := swCandidates(m)
	for i := range cands {
		if skip[cands[i].URL] {
			continue
		}
		return &cands[i]
	}
	return nil
}

// vcForward: agle server ko /verifyhop POST (hop+1).
func vcForward(targetURL, jid, dispatchID, origin string, hop int, visited []string) *vcHopResponse {
	if targetURL == "" || jid == "" {
		return nil
	}
	client := &http.Client{Timeout: vcHopWait}
	body, _ := json.Marshal(vcHopRequest{
		JID: jid, DispatchID: dispatchID, Hop: hop, Origin: origin, Visited: visited,
	})
	url := strings.TrimRight(targetURL, "/") + "/verifyhop"
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GOLDMD-VERIFYCHAIN/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out vcHopResponse
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}

// handleVerifyHop: /verifyhop endpoint — ek hop ka verify + forward/reconnect.
// Har server isi endpoint pe aata hai; SAME verify karta hai; phir agle ko
// forward karta hai ya (last hop pe) yahin reconnect kar deta hai.
func handleVerifyHop(m *Manager, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body vcHopRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(vcHopResponse{Status: "error"})
		return
	}
	jid := strings.TrimSpace(body.JID)
	if jid == "" || !strings.Contains(jid, "@") {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(vcHopResponse{Status: "error", JID: jid})
		return
	}
	hop := body.Hop
	if hop < 0 {
		hop = 0
	}
	origin := body.Origin
	if origin == "" {
		origin = fleetSelfID
	}
	write := func(st string) {
		_ = json.NewEncoder(w).Encode(vcHopResponse{Status: st, JID: jid, Server: fleetSelfID, Hop: hop})
	}
	writeFinal := func(st, finalServer string) {
		_ = json.NewEncoder(w).Encode(vcHopResponse{
			Status: st, JID: jid, Server: fleetSelfID, Hop: hop, FinalServer: finalServer,
		})
	}

	if m == nil || m.IsShuttingDown() {
		write("busy")
		return
	}

	// dispatch lock verify (race-safety: sirf wahi chain chale jo lock
	// likh ke aayi hai — 200 servers ka thundering herd yahin rukta hai).
	if m.Redis != nil && body.DispatchID != "" {
		if v, ok := m.Redis.getStringKV(fleetDispatchLockPrefix + jid); ok {
			if !strings.HasPrefix(v, body.DispatchID+"|") {
				write("busy") // doosra dispatcher jeet gaya
				return
			}
		}
	}

	// ── HOP VERIFY (owner: "ek ke pass aaye wo test kare") ──
	// 1. WhatsApp-truth (agar session object yahan hai).
	switch vcHopVerify(m, jid) {
	case "online":
		write("online_elsewhere") // logged-in → koi aur chala raha hai
		return
	case "logged_out":
		write("logged_out") // purge ho gaya (Storj + folder)
		return
	}

	// 2. online-elsewhere guard (claim + /sessions probe).
	if fleetSessionOnlineElsewhere(jid) {
		write("online_elsewhere")
		return
	}

	// 3. slot full → chain aage badhe (agle server pe jaga ho sakti hai).
	if m.SlotsUsed() >= maxPairedSessions() {
		write("full")
		return
	}

	// 4. already connected here?
	if m.AlreadyConnected(fleetUserPart(jid)) {
		write("connected")
		return
	}

	// 5. LAST HOP → yahin reconnect (owner: "phir ja kar kisi server pe
	//    session reconnect ho").
	if hop >= vcMaxHops {
		vcReconnectHere(m, jid, write)
		return
	}

	// 6. FORWARD to next server (SAME verify wahan).
	visited := append([]string{}, body.Visited...)
	if s := swSelfURL(); s != "" {
		visited = append(visited, s)
	}
	next := vcNextHop(m, visited)
	if next == nil {
		// Koi agla server nahi → yahin reconnect try karo.
		vcReconnectHere(m, jid, write)
		return
	}
	res := vcForward(next.URL, jid, body.DispatchID, origin, hop+1, visited)
	if res == nil {
		// Forward fail → yahin reconnect try karo.
		vcReconnectHere(m, jid, write)
		return
	}
	// Agle hop ka status wapas bhejo (chain propagate).
	final := res.FinalServer
	if final == "" {
		final = res.Server
	}
	writeFinal(res.Status, final)
}

// vcReconnectHere: is server pe session restore + connect (WhatsApp truth).
func vcReconnectHere(m *Manager, jid string, write func(string)) {
	// DEVICE: local nahi → Storj se blob restore (INSERT OR IGNORE — safe).
	if !fleetDeviceExists(jid) {
		if err := fleetRestoreBlob(jid); err != nil {
			fleetMarkFailed(jid)
			write("error")
			return
		}
	}
	if err := m.StartSession(jid); err != nil {
		fleetMarkFailed(jid)
		if strings.Contains(err.Error(), "whatsapp logged out") {
			swPurgeLoggedOut(m, jid)
			write("logged_out")
			return
		}
		if strings.Contains(err.Error(), "server full") {
			write("full")
			return
		}
		write("error")
		return
	}
	OkLog("VERIFY-CHAIN: %s reconnect ho gaya (server %s)", jid, fleetSelfID)
	write("connected")
}

// swDispatchChain: watchdog ka dispatch — multi-hop verify chain ka entry
// (hop=0). Pehle candidate pe /verifyhop POST; chain khud aage badhti hai.
// Ye swDispatch() ki jagah leta hai (owner order: multi-hop verify).
func swDispatchChain(m *Manager, jid string) bool {
	if m == nil || m.Redis == nil {
		return false
	}
	// dispatch lock (race-safety: 200 servers ka thundering herd yahin rukta).
	if dispatchLockFresh(m, jid) {
		return true // koi aur is JID ko handle kar raha hai
	}
	cands := swCandidates(m)
	if len(cands) == 0 {
		// Koi free server nahi — local revive try karo (slot free ho to).
		return swLocalRevive(m, jid)
	}
	tried := 0
	for _, t := range cands {
		if tried >= swMaxDispatchTries {
			break
		}
		if t.KnownCapc && t.Max > 0 && t.Sessions >= t.Max {
			continue // known-full — skip
		}
		tried++
		dispatchID := dispatchLockWrite(m, jid, t.SID)
		if dispatchID == "" {
			continue
		}
		res := vcForward(t.URL, jid, dispatchID, swSelfURL(), 0, []string{swSelfURL()})
		if res == nil {
			continue // offline/misroute — agla candidate
		}
		switch res.Status {
		case "connected":
			dispatchLockClear(m, jid)
			OkLog("SESSION-WATCHDOG: %s verify-chain se reconnect ho gaya (server %s)", jid, res.FinalServer)
			return true
		case "logged_out":
			dispatchLockClear(m, jid)
			swPurgeLoggedOut(m, jid)
			return true
		case "online_elsewhere":
			dispatchLockClear(m, jid)
			return true // koi aur chala raha hai — sab theek
		case "full":
			continue // agla candidate
		default:
			continue // busy / error — agla candidate
		}
	}
	return false
}
