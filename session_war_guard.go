package main

import (
	"strconv"
	"time"
)

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
		if m.Redis != nil {
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
