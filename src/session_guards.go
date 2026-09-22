package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	goldcmds "gold-md/gold-cmds"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ══════════════════ (merged from localonly_session.go) ══════════════════
// ============================================================================
// GOLD-MD — DISK-ONLY SESSIONS (owner order)
// File: localonly_session.go
// ============================================================================
// OWNER ORDER: "Direct?phone= ye endpoint isko set kr yeh session disk pe
// safe kre storj me na jaye iske sessions na configration sab disk pe hona
// chahye... khabardar hr chize delet ho jaye no issue disk se session nai
// jane chahye... jab cmnds naye banaye ge to bot restart kre ga to session
// disk se delete nai hone chahye"
//
// CLARIFICATION (owner): "Configrations storj se ane chahye sessions disk
// pe continu" — CONFIG (settings/prefix/owner name) Storj se hi aati rahegi,
// sirf SESSION (credentials/device) disk pe rahega.
//
// DIRECT /code?phone=<digits> se pair hone wala session = LOCAL-ONLY:
//   • Device row + creds  → sirf nexstore/goldmd.db (local disk)
//   • Pairing marker      → nexstore/pairing/<jid>/ (local disk) + "localonly"
//     marker file jo bina in-memory session ke bhi JID ko pehchane
//   • Storj pe KABHI nahi jata: na fleet blob (goldmd:fleet:sess:<jid>),
//     na claim (goldmd:fleet:claim:<jid>), na sessions set, na per-JID
//     registry (goldmd:sessiondb:<sid>:jids), na whole-DB backup
//     (goldmd:sessiondb:<sid>:blob) — central guard SaveSessionDB me hi
//     lagta hai taake KOI bhi future code-path leak na kar sake.
//   • Bot restart → AutoLoad disk se uthata hai (fleet guards BYPASS —
//     purani stale claims isko rok nahi sakti)
//   • cleanupSession / war-guard / koi bhi purge path disk data ko HAATH
//     nahi lagata (khabardar rule)
// ============================================================================

// localOnlyMarkerFile: pairing folder ke andar marker — iska hona = ye JID
// disk-only session hai. File system hi source of truth hai (restart ke
// baad bhi in-memory LocalOnly flag zero hota hai, marker rehta hai).
const localOnlyMarkerFile = "localonly"

// localOnlyMu: marker stat cache ki race safety.
var localOnlyMu sync.Mutex

// writeLocalOnlyMarker: direct pairing ke waqt pairing folder me marker
// file bana do. Marker = ye session Storj pe kabhi nahi jayega.
func writeLocalOnlyMarker(pairingDir, jid string) {
	if pairingDir == "" || jid == "" {
		return
	}
	dir := filepath.Join(pairingDir, jid)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, localOnlyMarkerFile), []byte("disk-only\n"), 0o644)
}

// isLocalOnlyJID: kya ye JID ka session disk-only hai? (marker file check)
// In-memory Session ho ya na ho — ye check hamesha kaam karta hai.
func isLocalOnlyJID(pairingDir, jid string) bool {
	if pairingDir == "" || jid == "" {
		return false
	}
	localOnlyMu.Lock()
	_, err := os.Stat(filepath.Join(pairingDir, jid, localOnlyMarkerFile))
	localOnlyMu.Unlock()
	return err == nil
}

// anyLocalOnlyOnDisk: kya ISS server ke pairing dir me koi bhi disk-only
// session pada hai? Whole-DB Storj upload (SaveSessionDB) isi se block
// hota hai — kyunki DB me local-only device rows bhi hoti hain, aur unke
// creds Storj pe leak hona owner order ka violation hai.
func anyLocalOnlyOnDisk(pairingDir string) bool {
	if pairingDir == "" {
		return false
	}
	entries, err := os.ReadDir(pairingDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(pairingDir, e.Name(), localOnlyMarkerFile)); err == nil {
			return true
		}
	}
	return false
}

// localOnlyUploadGuardFn: main.go boot pe register karta hai — SaveSessionDB
// (whole-DB Storj upload) se PEHLE puchta hai: koi local-only session disk
// pe hai? Ha → upload CANCEL (session creds Storj kabhi nahi jayenge).
var localOnlyUploadGuardFn func() bool

// RegisterLocalOnlyUploadGuard: boot-time registration (main.go).
func RegisterLocalOnlyUploadGuard(fn func() bool) {
	localOnlyMu.Lock()
	localOnlyUploadGuardFn = fn
	localOnlyMu.Unlock()
}

// localOnlyUploadBlocked: SaveSessionDB ke andar ka central gate.
func localOnlyUploadBlocked() bool {
	localOnlyMu.Lock()
	fn := localOnlyUploadGuardFn
	localOnlyMu.Unlock()
	return fn != nil && fn()
}

// localOnlyCleanFleetKeys: direct pairing ke WAQT purani fleet keys saaf
// karo (sirf ye ek jagah Storj DELETE hota hai — session ke liye, owner
// order: local-only session Storj se GAYAB rehna chahiye). Ye tab zaroori
// hai jab number pehle fleet session tha (jaise is JID ka blob kisi doosre
// server ne claim kiya hua tha) — purana blob/claim mila to doosre server
// usi se connect karke war shuru kar denge.
//
// FIRE-AND-FORGET: pairing speed pe 0% asar. Config (settings:<jid>)
// SAFE rehta hai — owner order ke mutabiq config Storj me hi rahega.
func localOnlyCleanFleetKeys(jid string) {
	if jid == "" {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		// fleet-level keys (blob / claim / set / meta / fail-mark)
		_, _ = m.Redis.cmd("DEL", fleetClaimPrefix+jid)
		_ = m.Redis.setRem(fleetSessionsSet, jid)
		_ = m.Redis.setDel(fleetBlobPrefix + jid)
		_ = m.Redis.setDel(fleetMetaPrefix + jid)
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		// own-server jids registry (AutoLoad restore list) — local-only
		// session disk se hi milta hai, registry ki zaroorat nahi.
		_ = m.Redis.RemoveJID(jid)
	}()
}

// localOnlyOwnerConfig: pair-success pe owner CONFIG Storj me save karo
// (owner clarification: "configrations storj se ane chahye" — settings
// Storj pe rehti hain, sirf SESSION disk pe). fleetOnPairSuccess ka
// owner-setting hissa hi — blob/claim/set us raaste me nahi lagenge.
func localOnlyOwnerConfig(jid, owner string) {
	if jid == "" || owner == "" {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		if existing := m.Redis.GetSetting(jid, "owner", ""); existing == "" {
			m.Redis.SetSetting(jid, "owner", normalizeJID(owner))
		}
	}()
}

// localOnlyDirEntries helper: nahi chahiye — AutoLoad seedha ReadDir karta
// hai; ye file sirf marker helpers + guard deti hai.

// ══════════════════ (merged from session_war_guard.go) ══════════════════
// ============================================================================
// GOLD-MD — WAR GUARD (victim-side defense — owner order)
// File: session_war_guard.go
// ============================================================================
// OWNER ORDER: "ek server pe bot online hai theek chal raha — kisi AUR
// server se us whatsapp session ko fazul reconnect aye (kyunki wo already
// chal raha hai) — wo bar bar reconnect bhejta rehta hai aur session crash
// kar deta hai. Aisi reconnect ko apne session me milne hi na do — use
// bhar me bhej do. 😂"
//
// ASLI WAR KA MECHANISM (root cause):
//   1. Dusra server same Storj blob (SAME KEYS) se Connect() karta hai.
//   2. WhatsApp humari websocket ko <stream:error conflict type=replaced>
//      se kick kar deta hai → humare side events.StreamReplaced aata hai.
//   3. Hara purana watchdog BINA kisi guard ke turant Connect() maar deta
//      tha → WhatsApp usko kick karta hai → wo wapas... INFINITE WAR.
//   4. Ladai ke dauran jis bhi side 401/device_removed lagta hai wahi
//      cleanupSession → Storj PURGE kar deta tha → SESSION DEAD. 💀
//
// DEFENSE (victim-side):
//   • Agar HUM rightful owner hain (fleet claim humare paas / koi aur live
//     holder nahi) → TURANT RETAKE: 1s me Connect() wapas + claim re-assert.
//     Attacker jo hai wo "bhar me" — jo jeeta wo session sambhalega.
//   • Agar koi AUR live server claim ka owner hai → SURRENDER: hum apna
//     local session hata dete hain (map + slot + socket), Storj/Redis ka
//     HAATH NAHI lagate (blob/claims/set safe — wo server owner hai) aur
//     war se nikal jate hain. WhatsApp ne logout NAHI bola to purge ka
//     koi saval hi nahi.
//
// GUARD PLACEMENT:
//   • events.StreamReplaced handler (manager.go) → retake ya surrender
//   • doReconnect (reconnect_watchdog.go) → Connect se PEHLE other-holder
//     check — agar aur koi zinda hai to hum Connect hi nahi karenge
//     (war me hissa hi nahi)
//   • handleDisconnectedEvent fast-path (reconnect_watchdog.go) → same check
//
// SILENT FILE: logs no-op hain (logger.go) — console pe kuch nahi.
// ============================================================================

// warRetakeDelay: StreamReplaced milne ke baad kitni der me RETAKE attempt
// (1s — jaldi, attacker ko settle hone ka mauka hi na do).
const warRetakeDelay = 1 * time.Second

// ── WAR GUARD CIRCUIT BREAKER (WAR-LOOP FIX 2026-09-21) ──
// Guarantee: war KABHI infinite nahi. Agar ek hi JID ke liye humne
// warRetakeMax baar retake kar liya (warRetakeWindow ke andar) to hum
// SURRENDER kar dete hain — chahe claim state kuch bhi ho. Ye un
// attackers ke against bhi kaam karta hai jo PURANE binary pe hain
// (fresh-claim fix unke paas nahi hai) aur claim grab karte rehte hain.
const (
	warRetakeMax    = 3                // itne retakes ke baad surrender
	warRetakeWindow = 5 * time.Minute  // is window me ginti
)

var (
	warRetakeMu    sync.Mutex
	warRetakeCount = map[string]int{}
	warRetakeFirst = map[string]time.Time{}
)

// warRetakeAllowed: circuit breaker — is JID ke liye retake allowed hai?
// false = limit cross → caller surrender kare (war khatam).
func warRetakeAllowed(jid string) bool {
	warRetakeMu.Lock()
	defer warRetakeMu.Unlock()
	first, ok := warRetakeFirst[jid]
	if !ok || time.Since(first) > warRetakeWindow {
		// naya window
		warRetakeFirst[jid] = time.Now()
		warRetakeCount[jid] = 0
		return true
	}
	return warRetakeCount[jid] < warRetakeMax
}

// warRetakeRecord: ek retake attempt record karo.
func warRetakeRecord(jid string) {
	warRetakeMu.Lock()
	warRetakeCount[jid]++
	warRetakeMu.Unlock()
}

// warRetakeReset: session stable ho gaya — counter saaf (agli war fresh ginti).
func warRetakeReset(jid string) {
	warRetakeMu.Lock()
	delete(warRetakeCount, jid)
	delete(warRetakeFirst, jid)
	warRetakeMu.Unlock()
}

// fleetOtherLiveHolder: koi AUR live server is JID ka claim holder hai?
// (fleetHeldByLiveServer hi hai — naam is file me padhne me aasan ho, is
// liye thin wrapper). false = hum rightful owner hain (ya fleet off).
func fleetOtherLiveHolder(jid string) bool {
	return fleetHeldByLiveServer(jid)
}

// warGuardActive: war-guard ka scope tabhi hai jab fleet (Storj Redis)
// active hai — warna single-server mode, koi "dusra server" hai hi nahi.
func warGuardActive() bool {
	return fleetActive()
}

// surrenderSession: WAR EXIT — hum ye session jeet nahi sakte (koi aur
// live server claim rakh raha hai). LOCAL cleanup ONLY:
//   - session map se hatao + slot release + socket disconnect
//   - Storj blob / fleet claims / sessions set / Redis registry SAFE
//     (owner server ki property — WhatsApp ne logout NAHI bola)
//   - dobara AutoLoad/fleet-claim isko tab claim karega jab wo server
//     mar jayega (orphan sweep + heartbeat liveness sab maujood hai)
//
// cleanupSession se FARK: cleanupSession = WhatsApp-truth logout purge
// (Storj/Redis sab delete). surrender = sirf local exit, data SAFE.
func (m *Manager) surrenderSession(s *Session, reason string) {
	if s == nil {
		return
	}
	// DISK-ONLY (owner order — khabardar rule): direct /code?phone= session
	// kabhi SURRENDER nahi hota. Ye session is server ki DISK property hai —
	// Storj pe iska blob hai hi nahi, to "dusra owner" ho hi nahi sakta. Sirf
	// socket ko zinda rakhne ka reconnect continue rahega; map/slot/disk sab
	// SAFE rehte hain.
	if s.LocalOnly || isLocalOnlyJID(m.cfg.PairingDir, s.JID) {
		WarnLog("WAR GUARD: %s DISK-ONLY session — surrender skip, disk data safe (owner order)", s.JID)
		return
	}
	// slot release (StartSession ki reservation bhi waapis)
	m.releaseSlot(s.JID)

	// session map se hatao
	m.mu.Lock()
	delete(m.sessions, s.JID)
	m.mu.Unlock()

	// socket band (background me — blocking na ho)
	if s.Client != nil {
		go func() {
			defer func() { _ = recover() }()
			// expectedDisconnect set karo taake Disconnect pe whatsmeow
			// "unexpected" na bole (hum jaan-boojh kar band kar rahe hain).
			s.Client.Disconnect()
		}()
	}
	// purge/re-register NAHI — war se exit, data SAFE.
	WarnLog("WAR GUARD: surrendered %s locally [%s] — remote owner zinda hai, Storj data safe", s.JID, reason)
}

// warRetake: HUM rightful owner hain aur koi attacker dusri jagah se same
// keys se connect ho gaya (StreamReplaced aaya). 1s me socket wapas le lo
// + apna claim fresh kar do. Attacker ka apna watchdog usko phir kick
// karega — humari claim-owner hone ki wajah se us side surrender-guard
// isko rok dega (naya guard us binary me bhi hai jab wo update hoga; purane
// binary wale attackers bhi phir khud hi war chhod denge jab unhe 401 mile).
func (m *Manager) warRetake(s *Session) {
	if s == nil || s.Client == nil {
		return
	}
	jid := s.JID
	// CIRCUIT BREAKER (WAR-LOOP FIX): agar humne is JID ko window me
	// warRetakeMax baar retake kar liya — ab SURRENDER (war khatam).
	// Ye purane-binary attackers ke against bhi war rokta hai.
	if !warRetakeAllowed(jid) {
		m.surrenderSession(s, "circuit breaker — retake limit cross, war khatam")
		return
	}
	warRetakeRecord(jid)
	go func() {
		defer func() { _ = recover() }()
		time.Sleep(warRetakeDelay)
		if m.IsShuttingDown() {
			return
		}
		// dobara sach check: ab bhi koi aur live holder nahi? (race me
		// takeover ho gaya ho to surrender hi sahi hai)
		if fleetOtherLiveHolder(jid) {
			m.surrenderSession(s, "retake ke waqt doosra live holder mila")
			return
		}
		// claim re-assert: humari ownership fresh ts ke sath
		// DISK-ONLY (owner order): local-only session Storj claim KABHI nahi
		// stamp karta — is JID ka Storj pe koi fingerprint nahi rehna chahiye.
		if m.Redis != nil && !isLocalOnlyJID(m.cfg.PairingDir, jid) {
			_, _ = m.Redis.cmd("HSET", fleetClaimPrefix+jid, fleetSelfID,
				strconv.FormatInt(time.Now().Unix(), 10))
		}
		// socket wapas — expectedDisconnect pattern me seedha Connect
		// (doReconnect ka cooldown slot isi goroutine me handle hoga).
		if !s.Client.IsConnected() {
			if err := s.Client.Connect(); err != nil {
				InfoLog("WAR GUARD: retake connect fail %s: %v (watchdog sambhalega)", jid, err)
			} else {
				OkLog("WAR GUARD: %s wapas le liya (attacker ko bhar me bheja)", jid)
			}
		}
	}()
}

// handleStreamReplaced: WAR-GUARD ka main entry (events.StreamReplaced pe
// manager.go ke event router se call hota hai).
//
//	ya to RETAKE (hum owner hain) ya SURRENDER (doosra live server owner).
func (m *Manager) handleStreamReplaced(s *Session) {
	if s == nil {
		return
	}
	if !warGuardActive() {
		return // fleet off — koi "dusra server" nahi, kuch mat karo
	}
	// DISK-ONLY (owner order): local-only session ke liye surrender ka
	// saval hi nahi — Storj claim exist nahi karta, "dusra owner" ho hi
	// nahi sakta. Attacker (koi bhi ho) ko retake se bhagao. Is JID ke
	// fleet keys localOnlyCleanFleetKeys ne pehle hi mita di hain.
	if s.LocalOnly || isLocalOnlyJID(m.cfg.PairingDir, s.JID) {
		m.warRetake(s)
		return
	}
	// koi AUR live server claim rakh raha hai? → wo asli owner hai (is
	// server ne boot-restore se galati se start kiya tha) → SURRENDER.
	if fleetOtherLiveHolder(s.JID) {
		m.surrenderSession(s, "stream replaced + doosra live server owner hai")
		return
	}
	// HUM rightful owner hain → attacker ko bhar me bhejo, wapas le lo.
	m.warRetake(s)
}

// warGuardShouldSkipConnect: doReconnect / handleDisconnectedEvent fast-path
// ke liye — agar koi AUR live server is JID ka owner hai to hum Connect()
// hi nahi karenge (uska watchdog-hi-war loop hum se shuru nahi hoga).
// true = SKIP reconnect (war-guard ne sambhal liya — surrender ho jata hai).
func (m *Manager) warGuardShouldSkipConnect(jid string) bool {
	if !warGuardActive() {
		return false
	}
	// DISK-ONLY (owner order): local-only session ka reconnect KABHI fleet
	// guard se nahi ruka — ye is server ki disk property hai. Agar koi
	// doosra server same WhatsApp account pe connect kare to StreamReplaced
	// aayega aur warRetake usko wapas le lega (bina claim stamp).
	if isLocalOnlyJID(m.cfg.PairingDir, jid) {
		return false
	}
	if !fleetOtherLiveHolder(jid) {
		return false // koi aur nahi — normal reconnect
	}
	// Doosra live server owner hai. Local session (agar map me hai)
	// surrender karo taake ye half-dead socket attack surface na bane.
	m.mu.Lock()
	sess := m.sessions[jid]
	m.mu.Unlock()
	if sess != nil {
		m.surrenderSession(sess, "watchdog reconnect-guard: doosra live owner hai")
	}
	return true
}

// ══════════════════ (merged from session_online_guard.go) ══════════════════
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
	// BOUNDED WAVE (512MB Render): 200 servers pe ek saath 200 goroutines
	// + 200 parked HTTP conns = RAM spike + scheduler churn. Ye sirf
	// background guard path hai (command speed pe 0% asar) is liye SEMA-8
	// chalao — max 8 concurrent probes, total latency ~2 min worst-case,
	// RAM spike ~1MB (8 stacks + 8 buffers). Overhead sab background me.
	// slot SPAWN se pehle le lo — kabhi bhi 32 se zyada goroutines hi
	// exist nahi karte (200 x 8KB stacks ki jagah sirf 32 x 8KB = 256KB).
	// EARLY-EXIT: pehle hi server pe mil gaya → baaki probes skip.
	// Worst case: 200 dead x 4s / 32 = ~25s — dispatch ke 75s budget me.
	sem := make(chan struct{}, 32) // 32 x ~8KB stack = ~256KB — 512MB pe nano
	for _, sid := range servers {
		if sid == fleetSelfID {
			continue
		}
		mu.Lock()
		already := found
		mu.Unlock()
		if already {
			break // mil gaya — aur probes ki zaroorat nahi
		}
		sem <- struct{}{} // slot le lo (block until free — fine, background)
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			defer func() { <-sem }()         // slot wapas karo
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
	// OWNER FIX B (2026-09-16): servers.json ke URLs + heartbeat me published
	// URLs — sandbox/tunnel (svr11221) servers.json me nahi tha is liye
	// online-elsewhere probe wave usko MISS karta tha → double-connect war.
	// Ab heartbeat URL wale servers bhi probe list me hain.
	seen := map[string]bool{}
	out := make([]string, 0, len(serversCfg.Servers)+4)
	add := func(u string) {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	loadServersConfig()
	for _, s := range serversCfg.Servers {
		add(s.URL)
	}
	for sid, u := range fleetHeartbeatURLs() {
		if sid == fleetSelfID {
			continue // apna URL nahi — hum khud probe nahi karte
		}
		add(u)
	}
	return out
}

// ── SHARED KEEP-ALIVE CLIENT + /sessions CACHE (bandwidth fix) ──
//
// MASLA: fleetSessionOnlineElsewhere() har unconnected JID ke liye SAARE
// servers.json (200) ko probe karta tha -> 14 JID x 200 = 2800 /sessions
// requests per pass. Har call naya http.Client banata tha (TLS handshake
// overhead) aur wahi server baar-baar fetch hota tha.
//
// HAL (0% behaviour change): ek shared keep-alive client + per-URL /sessions
// response cache (30s TTL). Ek hi pass me pehla JID saare servers fetch karta
// hai, baaki JIDs cache se padhte hain -> 2800 req -> ~200 req. DECISIONS
// bilkul same: wahi data, wahi safe-side policies (network fail pe online=false,
// present=true). Sirf network traffic kam.
var guardHTTPClient = &http.Client{
	Timeout: guardProbeTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	},
}

type guardSessCacheEntry struct {
	seen guardSessionsSeen
	ok   bool
	ts   time.Time
}

var (
	guardSessCacheMu sync.Mutex
	guardSessCache   = map[string]guardSessCacheEntry{}
)

// guardSessCacheTTL: ek pass ke andar saare JIDs same snapshot dekhein.
// 60s = watchdog pass duration se zyada, staleness window chhota.
const guardSessCacheTTL = 60 * time.Second

// guardFetchSessions: serverURL ka /sessions payload (cached 60s).
// ok=false = fetch fail (network / non-200 / decode) — caller apni
// safe-side policy lagata hai (online=false ya present=true).
func guardFetchSessions(serverURL string) (guardSessionsSeen, bool) {
	guardSessCacheMu.Lock()
	if e, hit := guardSessCache[serverURL]; hit && time.Since(e.ts) < guardSessCacheTTL {
		guardSessCacheMu.Unlock()
		return e.seen, e.ok
	}
	guardSessCacheMu.Unlock()

	var seen guardSessionsSeen
	ok := false
	resp, err := guardHTTPClient.Get(serverURL + "/sessions")
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&seen) == nil {
				ok = true
			}
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// SIRF SUCCESS cache karo (0% behaviour change): fetch fail hone pe har
	// caller apna fresh retry kare — bilkul original jaisa. Isse transient
	// network fail se cache poison nahi hota aur koi naya risk nahi banta.
	if ok {
		guardSessCacheMu.Lock()
		if len(guardSessCache) >= 512 {
			for k, e := range guardSessCache {
				if time.Since(e.ts) >= guardSessCacheTTL {
					delete(guardSessCache, k)
				}
			}
		}
		guardSessCache[serverURL] = guardSessCacheEntry{seen: seen, ok: ok, ts: time.Now()}
		guardSessCacheMu.Unlock()
	}
	return seen, ok
}

// guardRemoteSessionOnline probes one server's /sessions endpoint and
// reports whether the target JID is online there. Single attempt, 4s
// timeout — offline/STOPPED server false deta hai (network error/dead).
func guardRemoteSessionOnline(serverURL, jid string) bool {
	if serverURL == "" || jid == "" {
		return false
	}
	seen, ok := guardFetchSessions(serverURL)
	if !ok {
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

// guardRemoteSessionPresent: holder ke /sessions me ye JID mojood hai ya
// nahi — ONLINE ya OFFLINE dono gin lo (fleetHolderRunsSession isko use
// karta hai). OFFLINE-bhi-mojood = device us holder ke paas hai, bas
// reconnect-backoff me hai = sahi holder, respect. NAHI mila = holder
// ke paas session hai hi nahi = ZOMBIE claim (owner ke 2347067958986 wala
// case: server zinda tha, session wahan chal nahi raha tha).
//
// SAFE-SIDE RULE: network fail / 4xx-5xx / decode fail = zombie CONFIRM
// nahi hua = claim respect (true). Ye path sirf 10min+ STALE claim pe
// chalta hai — ghalat "present" hone se sirf takeover 10min der se hoga,
// ghalat "absent" hone se double-connect WAR (stream-replace = logout)
// ho sakta hai. Owner ka purana order: war kabhi nahi.
//
// Payload 1MB tak limit (panel list chhota hota hai — ek server max 2
// sessions + chhote pending entries, ~1-2KB hi hota hai).
func guardRemoteSessionPresent(serverURL, jid string) bool {
	if serverURL == "" || jid == "" {
		return false
	}
	seen, ok := guardFetchSessions(serverURL)
	if !ok {
		return true // network fail = confirm nahi = safe side (respect)
	}
	for _, s := range seen.Sessions {
		// ONLINE ya OFFLINE — dono me se koi bhi match kaafi hai
		// (device holder ke paas hai; offline sirf backoff hai).
		if s.JID == jid {
			return true
		}
	}
	// JID holder ke /sessions me NAHI mila — zombie claim confirm.
	return false
}

// ══════════════════ (merged from reconnect_watchdog.go) ══════════════════
// ============================================================================
// GOLD-MD — Always-On Reconnect Watchdog (ADAPTIVE / SLEEP-SMART)
// File: reconnect_watchdog.go
// ============================================================================
// WhatsApp ka naya update idle sessions ko silently disconnect kar deta hai
// (login rehta hai, socket band ho jata hai). whatsmeow ka built-in
// auto-reconnect 3 cases me fail hota hai:
//
//   1. AutoReconnectErrors=10 se pehla wait hi 20s hota hai, har fail pe
//      aur badhta jata hai (24s, 28s, ...) — bot der tak offline rehta hai.
//   2. Silent socket death: OnDisconnect event fire hi nahi hota to
//      auto-reconnect kabhi chalta hi nahi.
//   3. Server restart ke baad boot connect fail ho jaye to retry nahi.
//
// YE FILE SILENT HAI: koi bhi log call nahi (owner request — bot console
// pe kuch nahi chhaapta).
//
// ── ADAPTIVE DESIGN (owner request: "speed pe 0% farak, system ka kaam kam")
// Naukar (watchdog) ab tab smart SOTA HAI jab sab theek ho:
//
//   HEALTHY  → 60s deep sleep (1 halki jhaank/min, near-zero CPU)
//   MASLA    → 5s fast cycle  (fail/hang hone pe turant tez)
//   EVENT    → 1s (WhatsApp khud Disconnected event deta hai — sota hua
//              watchdog wakeup channel se FORAN uth jata hai, max 100ms)
//
// Healthy hone par system poora ka poora so jata hai. Silent socket death
// (rare case jab event fire hi nahi hota) ka max window 60s. Normal
// disconnect event-path 1s me pakar leta hai — wahi fast path unchanged.
// Command speed pe asar 0% (message path se ye loop kabhi guzarta hi nahi).
// ============================================================================

const (
	// ── ADAPTIVE intervals (owner request: healthy = so jao) ──
	// watchdogIdleInterval: sab sessions healthy → deep sleep 60s.
	watchdogIdleInterval = 60 * time.Second
	// watchdogActiveInterval: koi session dead/fail → fast 5s cycle.
	watchdogActiveInterval = 5 * time.Second
	// watchdogFirstRetry: disconnect event ke baad pehla attempt kitni
	// der baad (1s — foran, user ko farak na pade).
	watchdogFirstRetry = 1 * time.Second
	// reconnectCooldownWindow: same JID ke liye do attempts ke beech
	// minimum gap (watchdog + event handler race se bachne ke liye).
	reconnectCooldownWindow = 10 * time.Second

	// connectTimeoutSec: ek Connect() attempt ka max waqt. Isse zyada
	// hang ho jaye (network black-hole) to goroutine chhod dete hain —
	// agla watchdog pass fresh attempt karta hai. Ye pehle deadlock ka
	// fix hai: blocking Connect() hamesha ke liye stuck ho sakta tha.
	connectTimeoutSec = 10 * time.Second

	// selfRestartThreshold: itne CONSECUTIVE failed reconnect attempts
	// ke baad full process self-restart (partial reconnect loop torne
	// ke liye — community-verified: in-process retries se loop nahi
	// tootta, full restart turant todta hai).
	selfRestartThreshold = 3
)

// watchdogWakeup: event-handler isi channel pe non-blocking send karta hai
// jab WhatsApp se Disconnected event aaye — sota hua watchdog foran uth
// jata hai (max 100ms me next pass). Khali channel pe recv ka cost zero hai
// (epoll-level), isliye healthy state me ye bilkul muft hai.
var watchdogWakeup = make(chan struct{}, 1)

// reconnectCooldown tracks per-JID last reconnect attempt time so the
// watchdog loop and the Disconnected-event handler never retry the same
// session simultaneously. Guarded by its own mutex (NOT m.mu — that one
// must stay fast for command routing).
var (
	reconnectMu       sync.Mutex
	reconnectCooldown = map[string]time.Time{}
	// reconnectStorm: per-JID recent reconnect count (owner order, Request 11c).
	// "bar bar connect/disconnect/restart na ho, WhatsApp ko spam na lage."
	// Har acquire pe ++; 5 min tak koi reconnect na ho to decay (reset).
	// Storm badhne pe cooldown window exponentially badhta hai (10s → 20s →
	// 40s → ... cap 5 min) — tight loop tootta hai, WhatsApp spam rukta hai.
	reconnectStorm = map[string]int{}
)

// reconnectAttempts tracks consecutive failed reconnect attempts per JID.
var (
	reconnectAttemptsMu sync.Mutex
	reconnectAttempts   = map[string]int{}
)

// watchReconnects is the always-on watchdog loop. Started once from main()
// right after AutoLoad. It NEVER exits until shutdown.
//
// ADAPTIVE: har pass ke baad result dekh ke neend ka waqt tay hota hai —
// sab healthy → 60s; koi dead → 5s; event aaye → foran.
func (m *Manager) watchReconnects() {
	// Startup delay: AutoLoad ke turant baad kuch sessions abhi connect ho
	// rahi hoti hain — unhe settle hone do, false alarm na ho.
	time.Sleep(15 * time.Second)

	for {
		if m.IsShuttingDown() {
			return
		}
		allHealthy := m.watchdogPass()

		// ── ADAPTIVE SLEEP ──
		// Healthy: 60s deep sleep. Dead/fail: 5s fast cycle.
		// Disconnected event: wakeup channel foran tor deta hai neend ko.
		interval := watchdogIdleInterval
		if !allHealthy {
			interval = watchdogActiveInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			// normal expiry — next pass
		case <-watchdogWakeup:
			timer.Stop()
			// event mila — foran next pass (cleanup ke liye empty recv
			// ki zaroorat nahi, channel buffered hai)
		}
	}
}

// watchdogPass checks every paired session and revives dead ones.
// Returns true jab SAARI sessions healthy hon — caller (adaptive loop)
// isi se decide karta hai ke 60s so jaye ya 5s fast cycle chalaye.
func (m *Manager) watchdogPass() bool {
	// Snapshot the session list under the lock, work outside it so command
	// routing (which also takes m.mu) is never blocked.
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	allHealthy := true
	for _, s := range sessions {
		if !m.reviveIfDead(s) {
			allHealthy = false
		}
	}
	JSONDebug("WATCHDOG_PASS", map[string]any{"sessions": len(sessions), "allHealthy": allHealthy, "self": fleetSelfID})
	return allHealthy
}

// reviveIfDead checks one session; if disconnected (but paired/credentialed),
// it reconnects immediately. Fast path: IsConnected() is a cheap socket check.
// Returns true = session healthy (ya watchdog ke scope me hi nahi).
func (m *Manager) reviveIfDead(s *Session) bool {
	if s == nil || s.Client == nil {
		return true
	}
	if m.IsShuttingDown() {
		return true
	}
	// Sirf paired/credentialed sessions revive karo — pending QR pairing
	// sessions (Store.ID == nil) watchdog ke scope me nahi hain.
	if !s.Paired || s.Client.Store == nil || s.Client.Store.ID == nil {
		return true
	}
	if s.Client.IsConnected() {
		// LOGOUT-LINTER (WhatsApp-truth): socket zinda dikhta hai magar
		// login-token dead (WhatsApp ne logout kiya, event miss/out-of-band)
		// to ye session zinda NAHI hai. Storj/Paired-flag jhuta bole to bhi
		// isko gino mat. 515 stream-restart ke transient re-login window
		// (10-20s) me bhi IsLoggedIn() false reh sakta hai is liye 60s GRACE:
		// pehle fail pe sirf timestamp mark, 60s+ se bhi dead to cleanup.
		if !s.Client.IsLoggedIn() {
			if s.LoginDeadSince.IsZero() {
				s.LoginDeadSince = time.Now()
				WarnLog("⛑ WATCHDOG: %s socket alive but login dead — marking (60s grace, 515-relogin transient safe)", s.JID)
			} else if time.Since(s.LoginDeadSince) > 60*time.Second {
				WarnLog("⛑ WATCHDOG: %s login dead 60s+ — WhatsApp-side logout cleanup", s.JID)
				m.cleanupSession(s, "watchdog: socket alive but login dead 60s+")
				return true
			}
		} else {
			s.LoginDeadSince = time.Time{} // login wapas aaya — grace reset
		}
		// Healthy (ya grace window): reset attempts so a future blip again gets fast retries.
		m.resetAttempts(s.JID)
		// WAR GUARD circuit breaker reset — session stable hai, agli war
		// (agar aaye) fresh ginti se shuru ho.
		warRetakeReset(s.JID)
		return true
	}
	// Dead. Cooldown check: agar Disconnected-event wala goroutine isi
	// session ko abhi reconnect kar raha hai to race mat karo.
	JSONDebug("WATCHDOG_DEAD", map[string]any{"jid": s.JID, "self": fleetSelfID, "connected": s.Client.IsConnected()})
	if !m.tryAcquireReconnect(s.JID) {
		JSONDebug("WATCHDOG_COOLDOWN_SKIP", map[string]any{"jid": s.JID, "self": fleetSelfID})
		return false // koi aur is par kaam kar raha hai — loop jagta rahe
	}
	return m.doReconnect(s)
}

// doReconnect performs the actual Connect() for a session that the caller
// has already "won" via tryAcquireReconnect (cooldown slot owned).
// Returns true = session ab healthy (connected / no-retry-needed).
func (m *Manager) doReconnect(s *Session) bool {
	if s == nil || s.Client == nil {
		return true
	}
	// Re-check: kisi aur (whatsmeow autoReconnect) ne beech me connect kar
	// diya to Connect() call ki zaroorat hi nahi.
	if s.Client.IsConnected() {
		m.resetAttempts(s.JID)
		return true
	}
	// ── WAR GUARD (pre-Connect): koi DOOSRA live server is JID ka claim
	// hold kar raha hai (usi ne same keys se Connect() maara hoga — ye
	// disconnect usi ka asar hai). Ab yahan se Connect() karke war me
	// hissa lena fazul hai: local SURRENDER (slot release + session drop,
	// Storj/Redis bilkul safe) aur connect skip. Attacker ke marne ke
	// baad fleet scan/AutoLoad session wapas ghar le aayega.
	if m.warGuardShouldSkipConnect(s.JID) {
		m.resetAttempts(s.JID)
		return true
	}
	// Attempts counter maintain — consecutive failures ka hisaab.
	attempts := m.bumpAttempts(s.JID)
	JSONDebug("RECONNECT_ATTEMPT", map[string]any{"jid": s.JID, "self": fleetSelfID, "attempt": attempts})

	// DEADLOCK FIX: Connect() blocking hai — dial hang hone pe goroutine
	// hamesha stuck reh jati thi (socketLock hold karte hue), iske baad
	// har watchdog pass bhi usi lock pe atak jata tha. Ab 10s timeout
	// ke sath chalta hai; timeout pe goroutine leak acceptable hai kyunki
	// 3 consecutive timeouts/fails pe process self-restart sab clean kar
	// deta hai.
	hung := false
	done := make(chan error, 1)
	go func() {
		done <- s.Client.Connect()
	}()
	var err error
	select {
	case err = <-done:
		// normal path — fast fail ya success
	case <-time.After(connectTimeoutSec):
		hung = true
	}

	if !hung && err == nil {
		m.resetAttempts(s.JID)
		JSONDebug("RECONNECT_OK", map[string]any{"jid": s.JID, "self": fleetSelfID, "attempt": attempts})
		return true
	}
	if !hung && errors.Is(err, whatsmeow.ErrAlreadyConnected) {
		// Kisi aur goroutine ne already connect kar diya — success maano.
		m.resetAttempts(s.JID)
		return true
	}
	if !hung && isDeadSessionError(err) {
		// 401 / device removed — WhatsApp-side logout. Manager ka
		// LoggedOut / ConnectFailure event handler hi cleanup karta hai
		// (wo aur bhi sahi jagah hai). Yahan se bas attempts reset —
		// dead-session retries se restart-flood na ho.
		m.resetAttempts(s.JID)
		return true
	}
	// Fail (ya hang): attempts >= 3 -> full process self-restart.
	// Ye wahi community-verified fix hai: partial reconnect loop
	// (stale state re-seed) in-process retries se nahi tootta —
	// full process restart poori state clear kar ke todta hai.
	// Self-restart session-DB Redis me save karta hai aur fresh process
	// 2-3s me saari sessions restore kar leta hai.
	//
	// BUSY GUARD: koi command (download pipeline) chal rahi ho to restart
	// KABHI nahi — mid-download restart hi command ko maar deta tha.
	// Busy ke dauran attempts ginte raho; busy end hone ke baad watchdog
	// ke agle pass pe (dead session 5s cycle) restart fire hota hai.
	JSONDebug("RECONNECT_FAIL", map[string]any{"jid": s.JID, "self": fleetSelfID, "attempt": attempts, "hung": hung, "err": errStr(err), "busy": cmdBusyActive()})
	if attempts >= selfRestartThreshold && !cmdBusyActive() {
		m.requestSelfRestart()
	}
	// Baaki fail: koi log nahi — session abhi dead hai (return false,
	// taake adaptive loop 5s fast mode me recheck kare).
	return false
}

// handleDisconnectedEvent is called from the event router when WhatsApp
// emits *events.Disconnected. It schedules an immediate reconnect (1s)
// instead of waiting for the next watchdog pass — socket zinda rehta
// hai, user ko farak nahi padta.
func (m *Manager) handleDisconnectedEvent(s *Session) {
	if s == nil || s.Client == nil || m.IsShuttingDown() {
		return
	}
	// BUSY GUARD: jab koi download command (RunWithTimeout pipeline) chal
	// rahi ho to 1s fast-reconnect usi download goroutine ke sath race
	// karta tha. Busy hone par fast-path skip; whatsmeow ka apna
	// EnableAutoReconnect asli disconnect par phir bhi reconnect karega,
	// aur watchdog ka next pass (max 60s) socket ko revive karega.
	if cmdBusyActive() {
		return
	}
	// Pending pairing sessions skip karo.
	if !s.Paired || s.Client.Store == nil || s.Client.Store.ID == nil {
		return
	}
	// ── WAR GUARD: doosra live server owner nikla (StreamReplaced wala
	// attacker) → 1s fast-reconnect skip + local surrender — war ko
	// yahin khatam karo, Connect() race mat karo.
	if m.warGuardShouldSkipConnect(s.JID) {
		return
	}
	if !m.tryAcquireReconnect(s.JID) {
		return
	}
	JSONDebug("DISCONNECTED_EVENT", map[string]any{"jid": s.JID, "self": fleetSelfID, "action": "fast-reconnect-1s"})
	// Sota hua watchdog ko foran jaga do (non-blocking send — agar wo
	// pehle se jag raha hai to drop, koi race nahi).
	select {
	case watchdogWakeup <- struct{}{}:
	default:
	}
	go func() {
		defer func() { _ = recover() }()
		time.Sleep(watchdogFirstRetry)
		if m.IsShuttingDown() {
			return
		}
		// NOTE: cooldown slot humne event time pe hi acquire kar liya tha,
		// isliye yahan doReconnect direct call hota hai (reviveIfDead me
		// dobara tryAcquireReconnect fail ho jata — 10s window me khud se
		// hi block ho jata).
		m.doReconnect(s)
	}()
}

// tryAcquireReconnect returns true if this caller "owns" the reconnect for
// this JID right now (cooldown prevents concurrent retries).
func (m *Manager) tryAcquireReconnect(jid string) bool {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()
	last, ok := reconnectCooldown[jid]
	// STORM DECAY: 5 min tak koi reconnect nahi → counter reset (normal case
	// pe backoff base 10s hi rehta hai).
	if ok && time.Since(last) > 5*time.Minute {
		reconnectStorm[jid] = 0
	}
	// ADAPTIVE WINDOW (owner: "bar bar connect/disconnect na ho"): storm
	// count ke hisaab se window badhao — 10s, 20s, 40s, 80s, 160s, cap 5min.
	window := reconnectCooldownWindow
	if n := reconnectStorm[jid]; n > 1 {
		shift := n - 1
		if shift > 5 {
			shift = 5
		}
		window = reconnectCooldownWindow * time.Duration(1<<uint(shift))
		if window > 5*time.Minute {
			window = 5 * time.Minute
		}
	}
	if ok && time.Since(last) < window {
		return false
	}
	reconnectCooldown[jid] = time.Now()
	reconnectStorm[jid]++
	return true
}

// bumpAttempts increments and returns the consecutive failure count.
func (m *Manager) bumpAttempts(jid string) int {
	reconnectAttemptsMu.Lock()
	defer reconnectAttemptsMu.Unlock()
	reconnectAttempts[jid]++
	return reconnectAttempts[jid]
}

// resetAttempts clears the failure counter for a healthy session.
func (m *Manager) resetAttempts(jid string) {
	reconnectAttemptsMu.Lock()
	defer reconnectAttemptsMu.Unlock()
	delete(reconnectAttempts, jid)
}

// errStr safely stringifies an error for JSON debug (nil-safe).
func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// isDeadSessionError reports whether an error means the session is
// permanently dead (device deleted / logged out) — no point retrying.
func isDeadSessionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrDeviceDeleted) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "device removed") ||
		strings.Contains(msg, "logged out") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "406") ||
		strings.Contains(msg, "no session") ||
		strings.Contains(msg, "client is closed")
}

// ============================================================================
// SELF-RESTART (silent, cooldown-guarded)
// ============================================================================
// Jab 3 consecutive reconnect attempts fail/hang ho jayein (partial
// reconnect loop — in-process retry se wo nahi tootta, sirf full
// process restart todta hai), requestSelfRestart() graceful full
// restart trigger karta hai: session DB save -> clean shutdown ->
// fresh binary spawn (gracefulSelfRestart, main.go).
//
// Cooldown (10 min) restart-storm se bachata hai: fresh process agar
// phir bhi 3 fails pe pohnche to wo bhi restart karega, lekin 10 min
// me max ek hi baar — infinite restart loop kabhi nahi.
//
// YE FUNCTION SILENT HAI — koi log/console output nahi (owner rule).

const selfRestartCooldown = 10 * time.Minute

var (
	selfRestartMu        sync.Mutex
	selfRestartLast      time.Time
	selfRestartDBPath    string // main() set karta hai (goldmd.db ka full path)
	selfRestartTriggered bool
)

// requestSelfRestart: watchdog / event-handler kahin se bhi call kar
// sakte hain — call foran return hoti hai, actual restart background
// goroutine me hota hai. Do baar spawn hone se flag + cooldown dono
// bachaate hain; panic ho to bhi kuch nahi tootta (recover).
func (m *Manager) requestSelfRestart() {
	// BUSY GUARD (final gate): koi bhi command in-flight ho to full
	// process restart hamesha rokna hai (mid-download restart hi bot ko
	// "stuck" lagwa raha tha).
	if cmdBusyActive() {
		return
	}
	selfRestartMu.Lock()
	if selfRestartTriggered || time.Since(selfRestartLast) < selfRestartCooldown {
		selfRestartMu.Unlock()
		return
	}
	selfRestartTriggered = true
	selfRestartLast = time.Now()
	selfRestartMu.Unlock()

	go func() {
		defer func() { _ = recover() }() // fail-safe: restart kabhi panic na kare
		gracefulSelfRestart(m, m.Redis, selfRestartDBPath)
	}()
}

// ══════════════════ (merged from offline_queue_ignore.go) ══════════════════
// ============================================================================
// GOLD-MD — Offline-Queue Message Ignore (permanent, silent)
// File: offline_queue_ignore.go
// ============================================================================
// Owner: "jab bot band tha jo bhi messages aaye the, online aane pe unka
// jawab NAHI dena — sirf online hone ke baad bheje gaye commands ka jawab."
//
// WhatsApp offline messages QUEUE karta hai — jab socket reconnect hota hai
// to saare purane queued messages ek saath aate hain aur bot un sab ka jawab
// de deta tha (stale replies ek saath dikhte the). Ye module un sab ko ROK
// deta hai: handler.go ke start me guard isOldMessage() check karta hai.
//
// Logic (per-JID, multi-session safe):
//   - Har successful connect (events.Connected) pe markOnlineFresh(jid) us
//     JID ka onlineFreshAt set karta hai + 300ms grace-window (buffered
//     pushes ke liye — network WhatsApp koi purana msg connect ke turant
//     baad push kar deta hai).
//   - HandleMessage me: agar message ka timestamp us JID ke onlineFreshAt
//     se pehle ka hai, ya grace-window ke andar aaya — ye PURANA (queued)
//     message hai → poora ignore, na command chale na reply jaye.
//   - Boot pe zero-value onlineFreshAt: sab messages naye maane jayenge
//     (correct — server start ke baad aane wale messages fresh hi hain).
//   - Silent: koi logging nahi (owner: logs bilkul nahi chahiye).
// ============================================================================

// offlineGrace: reconnect ke turant baad aane wale buffered messages ko
// bhi purana maan ne ka window (network WhatsApp connect pe offline-time
// ke kuch messages push kar deta hai).
const offlineGrace = 300 * time.Millisecond

// offlineState per-JID online-tracking state.
type offlineState struct {
	onlineFreshAt time.Time // last time this session came ONLINE (events.Connected)
	ignoreUntil   time.Time // messages ts < iske tak abhi ke window me ignore
}

var (
	offlineMu       sync.Mutex
	offlineStateMap = map[string]offlineState{}
)

// markOnlineFresh events.Connected pe call hota hai (manager.go) — is JID
// ki session ab ONLINE hai: ab se aane wale messages naye hain, iske
// pehle wale sab queued-purane ignore honge.
func markOnlineFresh(jid string) {
	now := time.Now()
	offlineMu.Lock()
	offlineStateMap[jid] = offlineState{
		onlineFreshAt: now,
		ignoreUntil:   now.Add(offlineGrace),
	}
	offlineMu.Unlock()
}

// isOldMessage batata hai: ye message is JID ki session ke offline-period
// ka hai ya reconnect ke grace-window ke andar aaya (buffered) — matlab
// PURANA, ignore karna hai. Boot ke baad ya online hone ke baad ka naya
// message false lauta dega (uska jawab normal chalega).
func isOldMessage(jid string, ts time.Time) bool {
	offlineMu.Lock()
	st, ok := offlineStateMap[jid]
	offlineMu.Unlock()
	// JID ka record nahi = pehli baar ya boot — kuch mat karo, naya message.
	if !ok {
		return false
	}
	// Case 1: message online aane se PEHLE ka hai — 100% queued purana.
	if ts.Before(st.onlineFreshAt) {
		return true
	}
	// Case 2: grace-window chal raha hai aur message us window ke andar
	// ka hai (buffered push) — purana maano.
	if time.Now().Before(st.ignoreUntil) && ts.Before(st.ignoreUntil) {
		return true
	}
	return false
}

// ══════════════════ (merged from audio_sessions.go) ══════════════════
type AudioSession struct {
	Results []VideoResult
	Expiry  time.Time
	Play2   bool // true when the list came from .play2 (turbo audio engine)
	Play3   bool // true when the list came from .play3 (raw audio engine)
}

var (
	audioSessions = make(map[string]*AudioSession)
	audioMu       sync.Mutex
)

func setAudioSession(jid string, results []VideoResult) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioSessions[jid] = &AudioSession{Results: results, Expiry: time.Now().Add(2 * time.Minute)} // 2-minute guaranteed validity
}

func setAudioSession2(jid string, results []VideoResult, play2 bool) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioSessions[jid] = &AudioSession{Results: results, Expiry: time.Now().Add(2 * time.Minute), Play2: play2}
}

// setAudioSession3 stores a .play3 (raw audio) session so number picks route
// back through the play3 engine (raw audio, no thumbnail/caption).
func setAudioSession3(jid string, results []VideoResult) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioSessions[jid] = &AudioSession{Results: results, Expiry: time.Now().Add(2 * time.Minute), Play3: true}
}

func getAudioSession(jid string) *AudioSession {
	audioMu.Lock()
	defer audioMu.Unlock()
	s, ok := audioSessions[jid]
	if !ok {
		return nil
	}
	if time.Now().After(s.Expiry) {
		delete(audioSessions, jid)
		return nil
	}
	return s
}

func clearAudioSession(jid string) {
	audioMu.Lock()
	defer audioMu.Unlock()
	delete(audioSessions, jid)
}

func clearMediaSessions(jid string) {
	clearVideoSession(jid)
	clearAudioSession(jid)
}

// ══════════════════ (merged from video_sessions.go) ══════════════════
// VideoResult represents a single YouTube search result (internal type).
type VideoResult struct {
	Title     string
	URL       string
	Thumbnail string
	Duration  string
}

// VideoSession stores search results for a user pending number selection.
type VideoSession struct {
	Results []VideoResult
	Expiry  time.Time
	Video2  bool // true when the list came from .video2 (turbo engine)
	Video3  bool // true when the list came from .video3 (raw video engine)
	HD      bool // true when the user asked for HD quality
}

var (
	videoSessions = make(map[string]*VideoSession)
	sessionsMu    sync.Mutex
)

func setVideoSession(jid string, results []VideoResult) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	videoSessions[jid] = &VideoSession{
		Results: results,
		Expiry:  time.Now().Add(2 * time.Minute), // 2-minute guaranteed validity
	}
}

// setVideoSession2 stores a .video2 (turbo) session so number picks route
// back through the video2 engine, with HD mode when requested.
func setVideoSession2(jid string, results []VideoResult, hd bool) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	videoSessions[jid] = &VideoSession{
		Results: results,
		Expiry:  time.Now().Add(2 * time.Minute),
		Video2:  true,
		HD:      hd,
	}
}

// setVideoSession3 stores a .video3 (raw video) session so number picks route
// back through the video3 engine (raw video, no thumbnail/caption).
func setVideoSession3(jid string, results []VideoResult, hd bool) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	videoSessions[jid] = &VideoSession{
		Results: results,
		Expiry:  time.Now().Add(2 * time.Minute),
		Video3:  true,
		HD:      hd,
	}
}

func getVideoSession(jid string) *VideoSession {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	sess, ok := videoSessions[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		delete(videoSessions, jid)
		return nil
	}
	return sess
}

func clearVideoSession(jid string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	delete(videoSessions, jid)
}

// ══════════════════ (merged from logo1000_main.go) ══════════════════
// ============================================================================
// GOLD-MD — logo1..logo1000 hidden command registrations (main package)
// File: logo1000_main.go
// ============================================================================
// OWNER ORDER: logo1..logo1000 .menu me kabhi nahi dikhne chahiye. Isliye ye
// Commands map me direct register hote hain (gold-cmds registry me nahi) +
// hiddenCommands set me hain:
//   .menu → hiddenCommands filter (manager.go) unhe skip karta hai.
//   .logo → fancy boxed menu (ShowLogoMenu → manager.go CmdLogoMenu).
// Handler: goldcmds.LogoRunN(n) — 15-key Agnes pool + 1000 designs engine.
// ============================================================================

// registerLogo1000Commands registers logo1..logo1000 in the main-package
// Commands map (hidden from both menus per owner order).
func registerLogo1000Commands() {
	for n := 1; n <= goldcmds.LogoCount; n++ {
		n := n
		name := fmt.Sprintf("logo%d", n)
		RegisterCommand(name, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			beginCmdBusy()
			func() {
				defer endCmdBusy()
				goldcmds.LogoRunN(&bridge{s: s}, info, args, prefix, n)
			}()
		})
		hiddenCommands[name] = true
	}
}

func init() {
	registerLogo1000Commands()
}
