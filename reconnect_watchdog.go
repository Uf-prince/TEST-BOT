package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
)

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
		return true
	}
	// Dead. Cooldown check: agar Disconnected-event wala goroutine isi
	// session ko abhi reconnect kar raha hai to race mat karo.
	if !m.tryAcquireReconnect(s.JID) {
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
	// Attempts counter maintain — consecutive failures ka hisaab.
	attempts := m.bumpAttempts(s.JID)

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
	if !m.tryAcquireReconnect(s.JID) {
		return
	}
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
	if ok && time.Since(last) < reconnectCooldownWindow {
		return false
	}
	reconnectCooldown[jid] = time.Now()
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
