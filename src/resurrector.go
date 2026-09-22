// ============================================================================
//   GOLD-MD — SESSION RESURRECTOR (owner order)
//
//   "Watchdog ko ek aur kaam saunpna hai: jab Storj/disk me session folders
//   load hote hain to wo HAR WAQT disk wale sessions par nazar rakhe, Storj
//   walo par bhi. Jo bhi session load ho, FORAN check karo — kya ye session
//   waqai WhatsApp ke saath logged-in hai?
//     • LOGOUT mila  → IGNORE (silent purge — config safe)
//     • LOGIN mila, aur wo already kisi server pe ONLINE chal raha hai →
//       rehne do (koi kaam nahi)
//     • LOGIN mila magar OFFLINE hai (bot disconnected) → servers.json ke
//       saare servers check karo, jis server me ek pairing ki jagah bachi
//       ho us server pe bhej kar reconnect karwa do.
//   Bot ki speed par 0% farak, RAM/memory/disk par 0% farak — apna kaam
//   bhi hota rahe, sessions kabhi OFF na hon, lagatar chalte rahen."
//
//   KAISE KARTA HAI (3 layers, sab background goroutines me):
//
//   1) HAR-WAQT DISK+STORJ WATCH — har 90s me teeno sources merge hote hain:
//      pairing-disk folders + Storj fleet set + Redis JID registry. Har
//      JID ka status nikalta hai (live-here / online-elsewhere / offline).
//
//   2) WHATSAPP TRUTH — offline JID ke liye StartSession ka built-in 12s
//      verify window hi judge hai: logout → silent purge (ignore), login →
//      connect. Remote dispatch pe target server isi window se truth bhi
//      return karta hai (status=logged_out / connected).
//
//   3) SMART DISPATCH — server full ho ya device local na ho to heartbeat
//      hash (Storj — single read, zero probe-storm) se LIVE+FREE server
//      chunta hai aur POST /fleetdispatch se us server par bhej deta hai.
//      Race-safety: dispatch lock (Storj) + claim tie-break + double
//      online-elsewhere guard (local + target dono taraf).
//
//   SILENT FILE: InfoLog/WarnLog no-op hain — console clean.
// ============================================================================

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── timing / limits (resurrector) ──────────────────────────────────────────
const (
	resurrectTick      = 90 * time.Second // har 90s ek pass (light: 1 disk ReadDir + 1 Storj read)
	resurrectLockTTL   = 8 * time.Minute  // dispatch lock ki freshness window
	resurrectStartWait = 45 * time.Second // boot settle (AutoLoad + fleet watchdog kostart hone do)
	resurrectMaxTries  = 3                // ek JID ke liye ek pass me max 3 candidate servers
	resurrectHTTPWait  = 75 * time.Second // /fleetdispatch POST ka overall timeout
)

// dispatch lock key (Storj KV): value = "<dispatchID>|<unix ts>"
const fleetDispatchLockPrefix = "goldmd:fleet:dispatch:"

// resurrectState: per-JID last local-revive attempt (cooldown for local-only
// disk sessions — StartSession khud purge karta hai logout pe, isliye yahan
// sirf retry-throttle chahiye taake Storj/Disk par bar-bar load na ho).
var (
	resurrectMu      sync.Mutex
	resurrectLastTry = map[string]time.Time{}
)

// resurrectRetryAfter: kitni der baad wahi JID dobara try ho sakta hai.
func resurrectRetryAfter(jid string) bool {
	resurrectMu.Lock()
	defer resurrectMu.Unlock()
	if t, ok := resurrectLastTry[jid]; ok && time.Since(t) < 5*time.Minute {
		return false
	}
	// RAM GUARD (512MB Render): map kabhi unlimited na badhe — cap cross
	// hote hi 5min+ purani (ab irrelevant) entries saaf karo. Max ~512
	// entries x ~120B = ~60KB — guaranteed bounded memory.
	if len(resurrectLastTry) >= 512 {
		for j, t := range resurrectLastTry {
			if time.Since(t) >= 5*time.Minute {
				delete(resurrectLastTry, j)
			}
		}
	}
	resurrectLastTry[jid] = time.Now()
	return true
}

// StartSessionResurrector: main() se call — background loop, kabhi exit nahi
// hoti (panic-safe). Boot ke 45s baad + random 0-30s stagger (200 servers ek
// saath na uthein), phir har 90s ek pass.
func StartSessionResurrector(m *Manager) {
	if m == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				time.Sleep(resurrectTick)
				go StartSessionResurrector(m) // kabhi die nahi hoga
			}
		}()
		time.Sleep(resurrectStartWait)
		time.Sleep(time.Duration(rand.Intn(30)) * time.Second) // stagger
		ticker := time.NewTicker(resurrectTick)
		defer ticker.Stop()
		for range ticker.C {
			if m == nil || m.IsShuttingDown() {
				return
			}
			resurrectorPass(m)
		}
	}()
}

// ── 1) JID COLLECTION (disk + Storj fleet set + Redis registry) ────────────

// resurrectorDiskJIDs: pairing dir ke session folders (har-waqt disk watch).
func resurrectorDiskJIDs(pairingDir string) []string {
	entries, err := os.ReadDir(pairingDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.Contains(e.Name(), "@") {
			out = append(out, e.Name())
		}
	}
	return out
}

// resurrectorCollectJIDs: teeno sources merge (dedup).
func resurrectorCollectJIDs(m *Manager) []string {
	seen := map[string]bool{}
	var out []string
	add := func(jid string) {
		jid = strings.TrimSpace(jid)
		if jid == "" || seen[jid] {
			return
		}
		seen[jid] = true
		out = append(out, jid)
	}
	// disk folders
	for _, jid := range resurrectorDiskJIDs(m.cfg.PairingDir) {
		add(jid)
	}
	// Storj fleet set
	if m.Redis != nil {
		for _, jid := range m.Redis.setMembers(fleetSessionsSet) {
			add(jid)
		}
		// Redis JID registry (pairing-folder wipe + redis-restore case)
		for _, jid := range m.Redis.ListJIDs() {
			add(jid)
		}
	}
	return out
}

// ── 2) STATUS CHECKS ────────────────────────────────────────────────────────

// resurrectorLiveHere: session YAHAN live+logged-in hai? (WhatsApp truth —
// sirf socket nahi, login bhi).
func resurrectorLiveHere(m *Manager, jid string) bool {
	m.mu.Lock()
	sess, ok := m.sessions[jid]
	m.mu.Unlock()
	if !ok || sess == nil || sess.Client == nil {
		return false
	}
	if !sess.Client.IsConnected() {
		return false
	}
	if !sess.Paired || sess.Client.Store == nil || sess.Client.Store.ID == nil {
		return false
	}
	// login-dead grace window me watchdog linter sambhalega — yahan
	// IsLoggedIn hi truth hai (515 transient bhi handle karta hai watchdog).
	return sess.Client.IsLoggedIn()
}

// ── 3) SMART DISPATCH (server selection via heartbeat hash — zero probes) ──

// dispatchTarget: ek candidate server (heartbeat-hash se, HTTP probe NAHI).
type dispatchTarget struct {
	SID       string
	URL       string
	Sessions  int  // -1 = unknown (purana binary / parse fail)
	Max       int  // -1 = unknown
	KnownCapc bool // heartbeat value me capacity fields thin
}

// fleetServerURLBySID: servers.json me se SID ka URL nikaalo. SID match nahi
// hua (naya server jo list me nahi) → URL kaise mile? fleetSelfID rule:
// servers hash me SID = GOLDMD_SERVER_ID ya RENDER_EXTERNAL_URL. HGETALL ka
// key khud wahi hota hai jo fleetServerURL(sid) se resolve hota hai — isliye
// seedhe fleetServerURL use karo (wo SID ko URL me resolve karta hai).
//
// NOTE: fleetServerURL(sid) fleet.go me hai (sid → URL). Yahan wrapper.
func dispatchServerURL(sid string) string {
	return fleetServerURL(sid)
}

// dispatchCandidates: Storj heartbeat hash se LIVE servers, fresh heartbeat
// (< 3 min), khud ko chhod kar, capacity ke hisaab se SORT (kam load pehle,
// known-free pehle). Zero HTTP — single Storj read (HGETALL).
//
//	value format (naya binary): "<ts>|<sessions>|<max>"
//	value format (purana):      "<ts>"
func dispatchCandidates(m *Manager) []dispatchTarget {
	if m == nil || m.Redis == nil {
		return nil
	}
	hb := fleetHeartbeatMap() // {sid: ts}
	if len(hb) == 0 {
		return nil
	}
	now := time.Now().Unix()
	var out []dispatchTarget
	for sid, ts := range hb {
		if sid == "" || sid == fleetSelfID {
			continue
		}
		if now-ts > 180 { // 3 min freshness (ek heartbeat miss bhi maaf)
			continue
		}
		url := dispatchServerURL(sid)
		if url == "" {
			continue
		}
		out = append(out, dispatchTarget{SID: sid, URL: url, Sessions: -1, Max: -1})
	}
	// capacity enrich: servers hash ki poori value padho (ts|sessions|max)
	for i := range out {
		if v, ok := m.Redis.cmdReadHashField(fleetServersHash, out[i].SID); ok {
			parts := strings.Split(v, "|")
			if len(parts) >= 3 {
				if s, e1 := strconv.Atoi(parts[1]); e1 == nil {
					out[i].Sessions = s
				}
				if mx, e2 := strconv.Atoi(parts[2]); e2 == nil {
					out[i].Max = mx
				}
				out[i].KnownCapc = out[i].Sessions >= 0 && out[i].Max > 0
			}
		}
	}
	// sort: known-free (sessions < max) pehle, phir unknown, phir known-full
	// (last resort ke liye rakhe hain — endpoint wahan bhi full bola to skip)
	rank := func(t dispatchTarget) int {
		switch {
		case t.KnownCapc && t.Max > 0 && t.Sessions < t.Max:
			return 0 // FREE + known — best
		case !t.KnownCapc:
			return 1 // unknown — endpoint check karega
		default:
			return 2 // full (known) — sirf fallback
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rank(out[j]) < rank(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// dispatchLockFresh: Storj me lock pada hai aur fresh (< 8 min)? Fresh = koi
// aur server is JID ko abhi handle kar raha hai — skip.
func dispatchLockFresh(m *Manager, jid string) bool {
	if m == nil || m.Redis == nil {
		return false
	}
	v, ok := m.Redis.getStringKV(fleetDispatchLockPrefix + jid)
	if !ok || v == "" {
		return false
	}
	parts := strings.Split(v, "|")
	if len(parts) < 2 {
		return true // samajh nahi aaya — safe side: mana fresh
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return true
	}
	return time.Now().Unix()-ts < int64(resurrectLockTTL/time.Second)
}

// dispatchLockWrite: lock likho (dispatchID = unique). Return dispatchID.
func dispatchLockWrite(m *Manager, jid, targetSID string) string {
	if m == nil || m.Redis == nil {
		return ""
	}
	id := fmt.Sprintf("%s-%d", fleetSelfID, time.Now().UnixNano())
	val := id + "|" + strconv.FormatInt(time.Now().Unix(), 10)
	_, _ = m.Redis.cmd("SET", fleetDispatchLockPrefix+jid, val)
	return id
}

// dispatchLockClear: kaam khatam — lock hatao (jaldi retry possible).
func dispatchLockClear(m *Manager, jid string) {
	if m == nil || m.Redis == nil {
		return
	}
	_, _ = m.Redis.cmd("DEL", fleetDispatchLockPrefix+jid)
}

// fleetDispatchResponse: target server ka jawab.
type fleetDispatchResponse struct {
	Status string `json:"status"` // connected / online_elsewhere / logged_out / full / busy / error
	JID    string `json:"jid"`
	Server string `json:"server"`
}

// dispatchToServer: ek candidate pe POST /fleetdispatch (blocking, ≤75s).
func dispatchToServer(targetURL, jid, dispatchID string) *fleetDispatchResponse {
	client := &http.Client{Timeout: resurrectHTTPWait}
	body, _ := json.Marshal(map[string]string{"jid": jid, "dispatch_id": dispatchID})
	url := strings.TrimRight(targetURL, "/") + "/fleetdispatch"
	JSONDebug("DISPATCH_SEND", map[string]any{
		"jid": jid, "dispatch_id": dispatchID, "url": url,
		"from": fleetSelfID, "body": string(body),
	})
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		JSONDebugErr("DISPATCH_REQ_ERR", err, map[string]any{"jid": jid, "url": url})
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GOLDMD-RESURRECTOR/1.0")
	resp, err := client.Do(req)
	if err != nil {
		JSONDebugErr("DISPATCH_NET_FAIL", err, map[string]any{"jid": jid, "url": url})
		return nil // network fail — agla candidate
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out fleetDispatchResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		JSONDebugErr("DISPATCH_BAD_JSON", err, map[string]any{
			"jid": jid, "url": url, "http_status": resp.StatusCode, "raw": string(raw),
		})
		return nil
	}
	JSONDebug("DISPATCH_RESP", map[string]any{
		"jid": jid, "url": url, "http_status": resp.StatusCode,
		"status": out.Status, "server": out.Server, "raw": string(raw),
	})
	return &out
}

// resurrectorDispatchJID: free server dhundo + bhejo (max 3 candidates).
// Lokal fleet watchdog se race sirf claim-tie-break se bachta hai — dono
// taraf online-elsewhere + claim guards hain.
func resurrectorDispatchJID(m *Manager, jid string) string {
	if m == nil || m.Redis == nil {
		return ""
	}
	cands := dispatchCandidates(m)
	JSONDebug("RESURRECTOR_DISPATCH_START", map[string]any{
		"jid": jid, "candidates": len(cands), "self": fleetSelfID,
	})
	tried := 0
	for _, t := range cands {
		if tried >= resurrectMaxTries {
			break
		}
		if t.KnownCapc && t.Max > 0 && t.Sessions >= t.Max {
			JSONDebug("RESURRECTOR_SKIP_FULL", map[string]any{
				"jid": jid, "target": t.SID, "sessions": t.Sessions, "max": t.Max,
			})
			continue // known-full — skip (sirf unknown/free try karo)
		}
		tried++
		dispatchID := dispatchLockWrite(m, jid, t.SID)
		if dispatchID == "" {
			JSONDebug("RESURRECTOR_LOCK_FAIL", map[string]any{"jid": jid, "target": t.SID})
			continue
		}
		JSONDebug("RESURRECTOR_TRY", map[string]any{
			"jid": jid, "target": t.SID, "url": t.URL, "dispatch_id": dispatchID, "attempt": tried,
		})
		res := dispatchToServer(t.URL, jid, dispatchID)
		if res == nil {
			JSONDebug("RESURRECTOR_NO_RESP", map[string]any{"jid": jid, "target": t.SID})
			continue // offline/misroute — agla candidate (lock TTL bacha hai,
			// agli pass me fresh-check khud skip karega agar koi aur le gaya)
		}
		switch res.Status {
		case "connected":
			InfoLog("RESURRECTOR: %s → %s pe restore+connect ho gaya", jid, t.SID)
			JSONDebug("RESURRECTOR_RESULT", map[string]any{"jid": jid, "target": t.SID, "status": "connected"})
			dispatchLockClear(m, jid)
			return "connected"
		case "logged_out":
			WarnLog("RESURRECTOR: %s → WhatsApp logout confirm (target %s) — ignore", jid, t.SID)
			JSONDebug("RESURRECTOR_RESULT", map[string]any{"jid": jid, "target": t.SID, "status": "logged_out"})
			dispatchLockClear(m, jid)
			return "logged_out"
		case "online_elsewhere":
			// koi aur server chala raha hai — sab theek, kuch nahi karna
			JSONDebug("RESURRECTOR_RESULT", map[string]any{"jid": jid, "target": t.SID, "status": "online_elsewhere"})
			dispatchLockClear(m, jid)
			return "online_elsewhere"
		case "full":
			JSONDebug("RESURRECTOR_RESULT", map[string]any{"jid": jid, "target": t.SID, "status": "full"})
			continue // agla candidate
		default:
			JSONDebug("RESURRECTOR_RESULT", map[string]any{"jid": jid, "target": t.SID, "status": res.Status})
			continue // busy / error — agla candidate
		}
	}
	JSONDebug("RESURRECTOR_DISPATCH_END", map[string]any{"jid": jid, "result": "none", "tried": tried})
	return ""
}

// ── 4) THE PASS ─────────────────────────────────────────────────────────────

// resurrectorPass: har 90s — sab JIDs status-check + offline wale revive/
// dispatch. Background goroutine me — hot path (command routing) kabhi touch
// nahi hota (m.mu sirf nanoseconds snapshot ke liye).
func resurrectorPass(m *Manager) {
	defer func() { _ = recover() }() // pass kabhi panic na kare

	jids := resurrectorCollectJIDs(m)
	JSONDebug("RESURRECTOR_PASS_START", map[string]any{
		"self": fleetSelfID, "jids": len(jids), "slots_used": m.SlotsUsed(), "slots_max": maxPairedSessions(),
	})
	if len(jids) == 0 {
		return
	}

	for _, jid := range jids {
		if m.IsShuttingDown() {
			JSONDebug("RESURRECTOR_PASS_ABORT", map[string]any{"self": fleetSelfID, "reason": "shutting_down"})
			return
		}

		// 1) LIVE HERE (WhatsApp truth: socket + login dono) → all good.
		if resurrectorLiveHere(m, jid) {
			flSetOnline(jid, true) // LOCAL: session live hai
			continue
		}

		// 2) PENDING QR / pairing in-flight? (pending-* folders) → skip.
		if strings.HasPrefix(fleetUserPart(jid), "pending") || strings.HasPrefix(jid, "pending") {
			continue
		}

		// 3) RetRY-THROTTLE: yahi JID abhi (5 min me) try ho chuka hai.
		if !resurrectRetryAfter(jid) {
			continue
		}

		// 4) LOCAL-ONLY (direct-pair disk property): is JID ka blob Storj
		//    me hai hi nahi — doosre server pe dispatch IMPOSSIBLE. Yahin
		//    revive karo (slot kabhi free hoga to lag jayega; StartSession
		//    ka truth-window logout pe khud purge karega).
		if isLocalOnlyJID(m.cfg.PairingDir, jid) {
			if fleetDeviceExists(jid) && m.SlotsUsed() < maxPairedSessions() {
				if err := m.StartSession(jid); err != nil {
					if strings.Contains(err.Error(), "whatsapp logged out") {
						InfoLog("RESURRECTOR: %s local-only logout — ignore (config safe)", jid)
					}
					// warn nahi — cooldown khud sambhalega
				} else {
					InfoLog("RESURRECTOR: %s local-only revive ho gaya", jid)
				}
			}
			continue // local-only kabhi dispatch nahi hota
		}

		// 5) ONLINE ELSEWHERE? (claim-check + remote probe wave — background
		//    me ~4-8s) → jo server chala raha hai wahi chalane do. Koi kaam nahi.
		if fleetSessionOnlineElsewhere(jid) {
			continue
		}
		// 5b) OFFLINE EVERYWHERE → fleet ko notify (owner order): local
		//     online=false + apna claim release + fail marker → doosra
		//     server EK BAAR claim karega.
		flNotifyOffline(jid)

		// 6) OFFLINE EVERYWHERE + login-valid hona chahiye:
		//    a) Yahan device hai + slot free → LOCAL claim. Fleet watchdog
		//       ye 60s me khud karta hai — resurrector sirf turant karta hai
		//       (speed) — bina kisi extra Storj read ke (claim khud likhega).
		if fleetDeviceExists(jid) && m.SlotsUsed() < maxPairedSessions() {
			// fleet-failed cooldown respect karo (recent connect-fail)
			fleetFailedMu.Lock()
			_, recentlyFailed := fleetFailedAt[jid]
			fleetFailedMu.Unlock()
			if !recentlyFailed {
				if err := m.StartSession(jid); err == nil {
					InfoLog("RESURRECTOR: %s local revive ho gaya", jid)
					continue
				}
				// fail hua → neeche dispatch try karo (ya next pass)
			}
		}

		//    b) Server FULL / device yahan nahi → DISPATCH to free server
		//       (servers.json fleet — heartbeat hash se live+free, zero probes).
		if dispatchLockFresh(m, jid) {
			continue // koi aur is JID ko handle kar raha hai
		}
		resurrectorDispatchJID(m, jid)
	}
}

// ── 5) /fleetdispatch ENDPOINT (har server pe — target side) ───────────────
//
//	POST /fleetdispatch  {"jid": "923...@s.whatsapp.net", "dispatch_id": "..."}
//
//	→ {"status":"connected|online_elsewhere|logged_out|full|busy|error"}
//
// Ek hi baar race-safe claim flow (fleetRestoreAndConnect jaisa) + WhatsApp
// truth verify + owner failover-notify — status wapas resurrector ko.
func handleFleetDispatch(m *Manager, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		JID        string `json:"jid"`
		DispatchID string `json:"dispatch_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(fleetDispatchResponse{Status: "error", JID: body.JID})
		return
	}
	jid := strings.TrimSpace(body.JID)
	if jid == "" || !strings.Contains(jid, "@") {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(fleetDispatchResponse{Status: "error", JID: jid})
		return
	}

	JSONDebug("FLEETDISPATCH_RECV", map[string]any{
		"jid": jid, "dispatch_id": body.DispatchID, "self": fleetSelfID,
		"slots_used": func() int { if m != nil { return m.SlotsUsed() }; return -1 }(),
	})

	write := func(st string) {
		JSONDebug("FLEETDISPATCH_REPLY", map[string]any{
			"jid": jid, "dispatch_id": body.DispatchID, "self": fleetSelfID, "status": st,
		})
		_ = json.NewEncoder(w).Encode(fleetDispatchResponse{Status: st, JID: jid, Server: fleetSelfID})
	}

	if m == nil || m.IsShuttingDown() {
		write("busy")
		return
	}

	// dispatch lock verify (race-safety: sirf wahi request chale jo lock
	// likh ke aayi hai — 200 servers ka thundering herd yahin rukta hai).
	if m.Redis != nil && body.DispatchID != "" {
		if v, ok := m.Redis.getStringKV(fleetDispatchLockPrefix + jid); ok {
			if !strings.HasPrefix(v, body.DispatchID+"|") {
				write("busy") // doosra dispatcher jeet gaya
				return
			}
		}
	}

	// LOCAL-ONLY session kabhi remote dispatch se nahi aata — pair-marker
	// check (ye JID is server ki disk property nahi hai to marker nahi hoga,
	// par safety rakhi hai: marker mila to mana lo owner ne yahan direct
	// pair kiya tha — skip).
	if isLocalOnlyJID(m.cfg.PairingDir, jid) {
		write("busy")
		return
	}

	// full? (real-time — SlotsUsed live+in-flight dono ginta hai)
	if m.SlotsUsed() >= maxPairedSessions() {
		write("full")
		return
	}

	// already connected here?
	if m.AlreadyConnected(fleetUserPart(jid)) {
		write("connected")
		return
	}

	// ONLINE-ELSEWHERE guard (claim + probe wave): koi aur chala raha hai →
	// usi ko chalane do, hum chup.
	if fleetSessionOnlineElsewhere(jid) {
		write("online_elsewhere")
		return
	}

	// CLAIM RACE (fleetRestoreAndConnect flow): stamp → wait → tie-break.
	now := strconv.FormatInt(time.Now().Unix(), 10)
	_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID, now)
	time.Sleep(fleetRaceWait)
	holders := fleetClaimHolders(jid)
	myTS := holders[fleetSelfID]
	won := true
	for sid, ts := range holders {
		if sid == fleetSelfID {
			continue
		}
		if ts > myTS || (ts == myTS && sid < fleetSelfID) {
			won = false
			break
		}
	}
	if !won {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		write("busy")
		return
	}

	// Double-check online-elsewhere (race window me kisi aur ne uthaya?).
	if fleetSessionOnlineElsewhere(jid) {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		write("online_elsewhere")
		return
	}

	// DEVICE: local nahi → Storj se blob restore (INSERT OR IGNORE — safe).
	if !fleetDeviceExists(jid) {
		if err := fleetRestoreBlob(jid); err != nil {
			_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
			fleetMarkFailed(jid)
			fleetFailedMu.Lock()
			attempts := fleetRestoreAttempts[jid] + 1
			fleetRestoreAttempts[jid] = attempts
			fleetFailedMu.Unlock()
			if attempts >= 3 {
				WarnLog("FLEET-DISPATCH: blob restore failed %dx for %s — dead blob, silent purge (config safe): %v", attempts, jid, err)
				fleetPurgeLoggedOutSession(jid)
				write("logged_out") // blob dead = koi server isko revive nahi kar sakta
				return
			}
			write("error")
			return
		}
		fleetFailedMu.Lock()
		delete(fleetRestoreAttempts, jid)
		fleetFailedMu.Unlock()
	}

	// CONNECT + WHATSAPP TRUTH (12s verify window StartSession me hai):
	//   login confirm → connected | logout → silent purge + logged_out.
	if err := m.StartSession(jid); err != nil {
		_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
		fleetMarkFailed(jid)
		if strings.Contains(err.Error(), "whatsapp logged out") {
			WarnLog("FLEET-DISPATCH: WhatsApp ne %s ko logout kiya — purge (config safe)", jid)
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
	OkLog("FLEET-DISPATCH: %s restored + connected (server %s)", jid, fleetSelfID)

	// FAILOVER notify (agar marker pada tha — ek hi baar).
	if mk, ok := m.Redis.getStringKV(fleetFailMarkPrefix + jid); ok && mk != "" && mk != fleetSelfID {
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		go fleetNotifyFailover(jid, mk)
		go fleetPurgeDeadServerKV(mk)
	}
	write("connected")
}

// cmdReadHashField: servers hash me se ek field ki RAW value (ts|sessions|max).
// (Upstash cmd me HGET already hai — yahan sirf convenience wrapper.)
func (u *Upstash) cmdReadHashField(hash, field string) (string, bool) {
	if u == nil {
		return "", false
	}
	raw, err := u.cmd("HGET", hash, field)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}
