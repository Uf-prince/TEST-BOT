// ============================================================================
//   GOLD-MD — COMPREHENSIVE SESSION WATCHDOG (owner order, Request 10)
//
//   OWNER ORDER (Roman Urdu, verbatim intent):
//     "Isy watchdog ko set kr de — lagatar nazar rakhe aur HAR WAQT har
//      session ko check karta rahe (live status status:true/false). Jese hi
//      kisi session ne status:false bola, pehle servers.json check kare —
//      koi server me jaga khali ho to us server ko bhej de ke ise FORAN
//      reconnect karo. Lekin foran reconnect bhejne se PEHLE session ko 3
//      baar check kare: WhatsApp ne bola logged-in / online:true to kuch na
//      kare; agar online:false dekhe to foran reconnect bheje. Iske aage 2
//      aur verify guards bhi laga de — wo bhi verify karein ke jo session
//      reconnect bhej raha hai wo WAQAI online:false hai. Verify karke false
//      dekha to agle ko bheje, wo bhi verify kare, phir dusre server pe
//      reconnect karwa de. Agar check karte waqt WhatsApp ne bola 'Logged
//      out' to us session ko Storj se bhi FORAN delete kar do aur uska
//      session folder bhi delete kar do — sirf session configurations nahi.
//      Har baar ye read SIRF local folder se kare. Ek guard bithao jo in
//      session folders pe nazar rakhe — jese hi server crash/restart hoga,
//      ye foran Storj se configurations aur sessions wapas disk me le aaye,
//      aur har cheez disk se read ho. Ye guard tab load kare (ek baar) jab
//      dekhe ke saare session configurations/sessions disk me available NA
//      hain — jab available hon to kuch na kare."
//
//   "Bot ki speed pe 0% farak, RAM/memory/disk pe 0% farak — apna kaam bhi
//    hota rahe, aur Render ka 5GB free bandwidth bhi bach jaye."
//
//   ── KAISE KAAM KARTA HAI (3 layers, sab background goroutines) ──────────
//
//   1) BOOT SESSION-FOLDER GUARD (StartSessionFolderGuard)
//      Boot pe EK BAAR check: disk pe sessions/configs available hain?
//        • HAAN  → kuch na karo (0 network, 0 bandwidth).
//        • NAHI  → Storj se session DB + pairing folders + configs wapas
//                  disk pe le aao (crash/restart recovery).
//      Phir ek light continuous guard (60s) jo SIRF local file-count dekhta
//      hai (os.ReadDir — 0 network) aur disk khali hone pe hi Storj se
//      reload karta hai. Ye wahi pattern hai jo dcGuardLoop use karta hai.
//
//   2) CONTINUOUS LIVE-STATUS WATCHDOG (StartSessionWatchdog)
//      Adaptive loop: sab healthy → 60s deep sleep; koi dead → 5s fast.
//      JID list SIRF DISK se (pairing folders + disk-cached registry) —
//      koi Storj read nahi. Har JID ka live status (true/false) check:
//        • LIVE HERE (socket + login) → healthy, kuch nahi.
//        • 3x STATUS CHECK (live-here + claim) → koi bhi "online" bole to
//          kuch nahi; teeno "offline" bolein to aage badho.
//        • 2 VERIFY GUARDS (claim re-check + remote /sessions probe wave)
//          → koi bhi "online" bole to kuch nahi (war-safe).
//        • LOGGED OUT (WhatsApp truth) → Storj keys + session folder purge
//          (configurations settings:<jid> HAMESHA SAFE).
//        • OFFLINE EVERYWHERE → servers.json/heartbeat se FREE server
//          chuno (zero probe-storm) aur /watchdogdispatch pe bhejo.
//
//   3) RECEIVING-SERVER VERIFY (/watchdogdispatch endpoint)
//      Target server khud verify karta hai (online-elsewhere + claim race +
//      WhatsApp truth 12s window) — phir reconnect. Status wapas bhejta
//      hai: connected / online_elsewhere / logged_out / full / busy / error.
//      "logged_out" aaya to watchdog Storj + folder purge kar deta hai.
//
//   BANDWIDTH (Render 5GB): JID list disk se, KV reads disk-cache se,
//   candidate selection heartbeat hash se (single cached read), remote
//   probe SIRF genuinely-offline JID ke liye (ek wave), adaptive sleep se
//   healthy state me 60s me sirf ek cheap pass. 0% hot-path impact.
// ============================================================================

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── timing / limits (session watchdog) ──────────────────────────────────────
const (
	// swHealthyInterval: sab sessions healthy → deep sleep 60s (0 bandwidth).
	swHealthyInterval = 60 * time.Second
	// swActiveInterval: koi session dead/offline → fast 5s cycle.
	swActiveInterval = 5 * time.Second
	// swStartWait: boot settle (AutoLoad + folder guard ko kaam karne do).
	swStartWait = 20 * time.Second
	// swStatusChecks: dispatch se PEHLE kitni baar status check (owner: 3).
	swStatusChecks = 3
	// swStatusGap: do status checks ke beech gap (transient blip absorb).
	swStatusGap = 2 * time.Second
	// swDispatchWait: /watchdogdispatch POST ka overall timeout.
	swDispatchWait = 75 * time.Second
	// swRetryCooldown: ek JID ke liye do dispatch attempts ke beech gap.
	swRetryCooldown = 5 * time.Minute
	// swFolderGuardTick: boot folder-guard ka continuous check interval.
	swFolderGuardTick = 60 * time.Second
	// swMaxDispatchTries: ek pass me max candidate servers.
	swMaxDispatchTries = 3
)

// swEnabled: master switch. OWNER ORDER — default ON (ye naya watchdog
// system owner ne explicitly maanga hai). GOLDMD_SESSION_WATCHDOG=off se
// band ho sakta hai.
func swEnabled() bool { return envBool("GOLDMD_SESSION_WATCHDOG", true) }

// swRetryState: per-JID last dispatch attempt (cooldown + bounded RAM).
var (
	swMu       sync.Mutex
	swLastTry  = map[string]time.Time{}
	swLastPass = time.Time{}
)

// swRetryAfter: kitni der baad wahi JID dobara try ho sakta hai.
// RAM GUARD: map kabhi unlimited na badhe — 512 entries cap, purani saaf.
func swRetryAfter(jid string) bool {
	swMu.Lock()
	defer swMu.Unlock()
	if t, ok := swLastTry[jid]; ok && time.Since(t) < swRetryCooldown {
		return false
	}
	if len(swLastTry) >= 512 {
		for j, t := range swLastTry {
			if time.Since(t) >= swRetryCooldown {
				delete(swLastTry, j)
			}
		}
	}
	swLastTry[jid] = time.Now()
	return true
}

// ════════════════════════════════════════════════════════════════════════════
//   LAYER 1 — BOOT SESSION-FOLDER GUARD
// ════════════════════════════════════════════════════════════════════════════

// StartSessionFolderGuard: boot pe ek baar + phir har 60s (light) check.
// Disk pe sessions/configs available hain to kuch nahi karta (0 bandwidth).
// Disk khali ho (crash/restart/wipe) to Storj se session DB + pairing
// folders + configs wapas disk pe le aata hai. Har cheez disk se read hoti
// hai — ye guard sirf disk-empty case me Storj touch karta hai.
func StartSessionFolderGuard(m *Manager) {
	if m == nil || m.Redis == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		// boot: ek baar (crash/restart recovery)
		swFolderGuardPass(m)
		// continuous: har 60s SIRF local check (0 network)
		t := time.NewTicker(swFolderGuardTick)
		defer t.Stop()
		for range t.C {
			if m.IsShuttingDown() {
				return
			}
			swFolderGuardPass(m)
		}
	}()
}

// swFolderGuardPass: disk pe sessions/configs available hain?
//
//	HAAN → kuch na karo (0 bandwidth).
//	NAHI → Storj se ek baar restore (session DB + pairing folders + configs).
func swFolderGuardPass(m *Manager) {
	if m == nil || m.Redis == nil {
		return
	}
	dbPath := filepath.Join(m.cfg.DataDir, "goldmd.db")
	diskHasDevice := hasUsableWhatsAppDevice(dbPath)
	diskFolders := len(resurrectorDiskJIDs(m.cfg.PairingDir))

	// AVAILABLE → kuch na karo (owner: "jab available ho to kuch na kre").
	if diskHasDevice || diskFolders > 0 {
		return
	}

	// DISK KHALI → Storj se restore (ek baar).
	InfoLog("SESSION-GUARD: disk khali (no device, no folders) — Storj se restore...")
	tmpPath := dbPath + ".guardrestore.tmp"
	_ = os.Remove(tmpPath)
	restored, srcSID, err := m.Redis.RestoreSessionDBAnyNamespace(tmpPath)
	if err != nil {
		WarnLog("SESSION-GUARD: restore error: %v", err)
		return
	}
	if restored {
		if rerr := os.Rename(tmpPath, dbPath); rerr != nil {
			WarnLog("SESSION-GUARD: activate failed: %v", rerr)
		} else {
			OkLog("SESSION-GUARD: session DB restored (namespace=%s)", srcSID)
		}
	} else {
		_ = os.Remove(tmpPath)
	}

	// pairing folders + configs recreate from JID registry (disk pe).
	jids := m.Redis.ListJIDs()
	for _, jid := range jids {
		_ = os.MkdirAll(filepath.Join(m.cfg.PairingDir, jid), 0o755)
	}
	if len(jids) > 0 {
		OkLog("SESSION-GUARD: %d pairing folder(s) recreated from Storj registry", len(jids))
	}
}

// ════════════════════════════════════════════════════════════════════════════
//   LAYER 2 — CONTINUOUS LIVE-STATUS WATCHDOG
// ════════════════════════════════════════════════════════════════════════════

// StartSessionWatchdog: main() se call — background loop, kabhi exit nahi.
// Adaptive: sab healthy → 60s; koi dead/offline → 5s fast cycle.
func StartSessionWatchdog(m *Manager) {
	if m == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				time.Sleep(swActiveInterval)
				go StartSessionWatchdog(m) // kabhi die nahi hoga
			}
		}()
		time.Sleep(swStartWait)
		for {
			if m.IsShuttingDown() {
				return
			}
			healthy := swWatchdogPass(m)
			interval := swHealthyInterval
			if !healthy {
				interval = swActiveInterval
			}
			time.Sleep(interval)
		}
	}()
}

// swWatchdogPass: ek pass — har disk JID ka live status check + offline
// wale ko revive/dispatch. Returns true jab SAARI sessions healthy hon.
func swWatchdogPass(m *Manager) bool {
	defer func() { _ = recover() }() // pass kabhi panic na kare

	// HEARTBEAT (bandwidth-safe): apni liveness + capacity + URL publish karo
	// (self-throttled 55s — max 1 Storj PUT/min). Isse doosre servers humein
	// discover kar sakte hain aur claim-based war guard (cheap) kaam karta hai.
	// fleetInit OFF hai is liye ye naya watchdog hi heartbeat ka zimmedar hai.
	fleetHeartbeat()

	jids := swDiskJIDs(m) // SIRF DISK (0 Storj read)
	swMu.Lock()
	swLastPass = time.Now()
	swMu.Unlock()
	if len(jids) == 0 {
		return true
	}

	allHealthy := true
	for _, jid := range jids {
		if m.IsShuttingDown() {
			return allHealthy
		}
		if !swCheckOne(m, jid) {
			allHealthy = false
		}
	}
	return allHealthy
}

// swDiskJIDs: JID list SIRF DISK se — pairing folders + disk-cached registry.
// Koi Storj read nahi (bandwidth-safe). Dedup.
func swDiskJIDs(m *Manager) []string {
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
	for _, jid := range resurrectorDiskJIDs(m.cfg.PairingDir) {
		add(jid)
	}
	if m.Redis != nil {
		for _, jid := range m.Redis.ListJIDs() { // disk-cache se
			add(jid)
		}
	}
	return out
}

// swCheckOne: ek JID ka poora check + action. Returns true = healthy.
func swCheckOne(m *Manager, jid string) bool {
	// 1) LIVE HERE (WhatsApp truth: socket + login) → healthy.
	if resurrectorLiveHere(m, jid) {
		flSetOnline(jid, true) // LOCAL: session live hai
		return true
	}

	// 2) PENDING QR / pairing in-flight → skip.
	if strings.HasPrefix(fleetUserPart(jid), "pending") || strings.HasPrefix(jid, "pending") {
		return true
	}

	// 3) Retry-throttle (5 min cooldown per JID).
	if !swRetryAfter(jid) {
		return true
	}

	// 4) LOCAL-ONLY (direct-pair disk property): blob Storj me hai hi nahi —
	//    dispatch impossible. Yahin revive karo (slot free ho to).
	if isLocalOnlyJID(m.cfg.PairingDir, jid) {
		swLocalRevive(m, jid)
		return true
	}

	// 5) 3x STATUS CHECK (owner: "session ko 3 bar check kare").
	//    Koi bhi check "online" bole → WhatsApp logged-in hai → kuch nahi.
	if swStatusCheck(m, jid) == "online" {
		return true
	}

	// 6) 2 VERIFY GUARDS (owner: "iske aage 2 aur verify guards").
	//    Confirm karo ke session WAQAI online:false hai (war-safe).
	if !swVerifyOffline(m, jid) {
		return true // koi aur server chala raha hai → kuch nahi
	}

	// 7) OFFLINE EVERYWHERE → fleet ko notify (owner: "jese hi offline ho
	//    fleet walo ko info kr de k ye session off hai, iska claim dusre
	//    server ko do"). Local online=false + apna claim release + fail
	//    marker → phir multi-hop verify chain dispatch (wo server EK BAAR
	//    claim karega).
	flNotifyOffline(jid)
	return swDispatchChain(m, jid)
}

// swStatusCheck: 3 baar status check (live-here + claim). Koi bhi "online"
// bole to "online" return. Teeno "offline" bolein to "offline".
// Cheap checks (claim disk-cached) — expensive probe verify guards me.
func swStatusCheck(m *Manager, jid string) string {
	for i := 0; i < swStatusChecks; i++ {
		if i > 0 {
			time.Sleep(swStatusGap)
		}
		if resurrectorLiveHere(m, jid) {
			return "online"
		}
		if fleetHeldByLiveServer(jid) {
			return "online"
		}
	}
	return "offline"
}

// swVerifyOffline: 5 verify guards — confirm session WAQAI online:false hai.
// OWNER ORDER (Request 11): "Ab 3 verifyer guards or lagao ... whatsapp se
// verify kare ... agar whatsapp ne bola logged out foran session delete
// setbora se b local folder se bhi ... agar logged in hai status false dekh
// rha agle k pass bhej de".
//
//	Guard 1: claim re-check (cheap, authoritative).
//	Guard 2: remote /sessions probe wave (expensive, ek baar).
//	Guard 3: WhatsApp-truth LOCAL probe — agar is server pe session object
//	         hai to WhatsApp se poochho: logged-in? (online → false) ya
//	         explicit logout? (purge + false). Ye "status false magar logged
//	         in" case ko pakadta hai.
//	Guard 4: doosri remote probe wave (fresh — transient blip absorb).
//	Guard 5: final claim re-check (race window band).
//
// Koi bhi "online" bole to false (dispatch nahi).
func swVerifyOffline(m *Manager, jid string) bool {
	// Guard 1 — claim re-check.
	if fleetHeldByLiveServer(jid) {
		return false
	}
	// Guard 2 — remote probe wave (claim + /sessions).
	if fleetSessionOnlineElsewhere(jid) {
		return false
	}
	// Guard 3 — WhatsApp-truth LOCAL probe (owner: "whatsapp se verify kare").
	//   logged-in  → online (dispatch nahi).
	//   logged-out → purge (Storj + folder) + dispatch nahi.
	if swLocalTruth(m, jid) {
		return false
	}
	// Guard 4 — doosri remote probe wave (fresh; transient blip absorb).
	time.Sleep(swStatusGap)
	if fleetSessionOnlineElsewhere(jid) {
		return false
	}
	// Guard 5 — final claim re-check (race window band).
	if fleetHeldByLiveServer(jid) {
		return false
	}
	return true
}

// swLocalTruth: is server pe session object hai to WhatsApp se poochho.
//
//	logged-in  → true  (online — dispatch nahi).
//	logged-out → true  (purge ho gaya — dispatch nahi).
//	object nahi / transient → false (aage dispatch chain verify karegi).
func swLocalTruth(m *Manager, jid string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	sess, ok := m.sessions[jid]
	m.mu.Unlock()
	if !ok || sess == nil || sess.Client == nil {
		return false
	}
	// WhatsApp truth (chhota window — hot-path pe asar nahi, background).
	verified, explicitLogout, _ := whatsappTruthVerified(sess, 3*time.Second)
	if verified {
		return true // WhatsApp ne login confirm kiya → online
	}
	if explicitLogout {
		swPurgeLoggedOut(m, jid) // owner: logged out → foran delete
		return true
	}
	return false // transient — chain aage verify karegi
}

// swLocalRevive: is server pe device hai + slot free → yahin revive.
// WhatsApp logout confirm hua to purge (configs SAFE).
func swLocalRevive(m *Manager, jid string) bool {
	if !fleetDeviceExists(jid) {
		return false
	}
	if m.SlotsUsed() >= maxPairedSessions() {
		return false
	}
	if err := m.StartSession(jid); err != nil {
		if strings.Contains(err.Error(), "whatsapp logged out") {
			swPurgeLoggedOut(m, jid)
		}
		return false
	}
	InfoLog("SESSION-WATCHDOG: %s local revive ho gaya", jid)
	return true
}

// swDispatch: servers.json/heartbeat se FREE server chuno + /watchdogdispatch
// pe bhejo (max 3 candidates). Receiving server khud verify karta hai.
func swDispatch(m *Manager, jid string) bool {
	if m == nil || m.Redis == nil {
		return false
	}
	// dispatch lock (race-safety: 200 servers ka thundering herd yahin rukta).
	if dispatchLockFresh(m, jid) {
		return true // koi aur is JID ko handle kar raha hai
	}
	cands := swCandidates(m) // servers.json + heartbeat — zero probe-storm
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
		res := swDispatchToServer(t.URL, jid, dispatchID)
		if res == nil {
			continue // offline/misroute — agla candidate
		}
		switch res.Status {
		case "connected":
			dispatchLockClear(m, jid)
			OkLog("SESSION-WATCHDOG: %s → %s reconnect ho gaya", jid, t.SID)
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

// swDispatchToServer: ek candidate pe POST /watchdogdispatch (blocking ≤75s).
func swDispatchToServer(targetURL, jid, dispatchID string) *fleetDispatchResponse {
	if targetURL == "" || jid == "" {
		return nil
	}
	client := &http.Client{Timeout: swDispatchWait}
	body, _ := json.Marshal(map[string]string{"jid": jid, "dispatch_id": dispatchID})
	url := strings.TrimRight(targetURL, "/") + "/watchdogdispatch"
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GOLDMD-WATCHDOG/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out fleetDispatchResponse
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}

// swPurgeLoggedOut: WhatsApp ne logout bola → Storj se session delete +
// session folder delete. CONFIGURATIONS (settings:<jid> — owner, prefix,
// sudo, autoreact, welcome) HAMESHA SAFE rehti hain (owner order).
func swPurgeLoggedOut(m *Manager, jid string) {
	if m == nil || jid == "" {
		return
	}
	// Storj: fleet keys + blob + registry (configs SAFE — fleetPurgeLoggedOutSession
	// settings:<jid> ko haath nahi lagata).
	fleetPurgeLoggedOutSession(jid)
	// Session folder delete (configs Storj me hain, folder me nahi).
	_ = os.RemoveAll(filepath.Join(m.cfg.PairingDir, jid))
	WarnLog("SESSION-WATCHDOG: %s WhatsApp logout — Storj + session folder purged (configs SAFE)", jid)
}

// ── CANDIDATE SELECTION (servers.json + heartbeat, zero probe-storm) ──

// swSelfURL: is server ka public URL (heartbeat 4th segment + self-skip).
func swSelfURL() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_PUBLIC_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return ""
}

// swCandidates: FREE server candidates — servers.json (owner: "pehle
// servers.json check kare") + heartbeat hash (live capacity). Khud ko
// chhod kar. Zero HTTP probe — sirf disk-cached KV reads.
//
//	rank 0: heartbeat-known FREE (sessions < max) — best
//	rank 1: unknown (endpoint khud check karega)
//	rank 2: heartbeat-known FULL — last resort
func swCandidates(m *Manager) []dispatchTarget {
	if m == nil || m.Redis == nil {
		return nil
	}
	self := swSelfURL()
	seen := map[string]bool{}
	var out []dispatchTarget
	add := func(sid, url string, sessions, max int, known bool) {
		url = strings.TrimRight(strings.TrimSpace(url), "/")
		if url == "" || seen[url] {
			return
		}
		if self != "" && url == self {
			return // khud ko dispatch nahi karte
		}
		seen[url] = true
		out = append(out, dispatchTarget{SID: sid, URL: url, Sessions: sessions, Max: max, KnownCapc: known})
	}
	// 1) servers.json (owner order: pehle servers.json check karo)
	loadServersConfig()
	for _, s := range serversCfg.Servers {
		add(s.URL, s.URL, -1, -1, false)
	}
	// 2) heartbeat hash — live capacity enrich (single cached HGETALL)
	hb := fleetHeartbeatMap()
	for sid, ts := range hb {
		if sid == "" || sid == fleetSelfID {
			continue
		}
		if time.Now().Unix()-ts > 180 {
			continue // 3 min freshness
		}
		url := dispatchServerURL(sid)
		if url == "" {
			continue
		}
		sessions, max := -1, -1
		known := false
		if v, ok := m.Redis.cmdReadHashField(fleetServersHash, sid); ok {
			parts := strings.Split(v, "|")
			if len(parts) >= 3 {
				if s, e1 := strconv.Atoi(parts[1]); e1 == nil {
					sessions = s
				}
				if mx, e2 := strconv.Atoi(parts[2]); e2 == nil {
					max = mx
				}
				known = sessions >= 0 && max > 0
			}
		}
		add(sid, url, sessions, max, known)
	}
	// sort: known-free pehle, phir unknown, phir known-full
	rank := func(t dispatchTarget) int {
		switch {
		case t.KnownCapc && t.Max > 0 && t.Sessions < t.Max:
			return 0
		case !t.KnownCapc:
			return 1
		default:
			return 2
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rank(out[j]) < rank(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
