package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	goldcmds "gold-md/gold-cmds"
)

// ============================================================================
// GOLD-MD — Session Manager (port of autoload.js + pair.js)
//
// One Manager runs many WhatsApp connectors. Each session is stored in
// the shared sqlstore container (file-based sqlite) keyed by JID, exactly
// mirroring the Node bot's nexstore/pairing/<number>@s.whatsapp.net dirs.
// ============================================================================

// ══════════════════════════════════════════════════════════════════════════
// 😡😡😡  WARNING — DO NOT CHANGE THIS VALUE. EVER.  😡😡😡
// ══════════════════════════════════════════════════════════════════════════
// MAX SESSIONS = 30. OWNER ORDER (2026).
//
// OWNER KA HUKM: "Isko max pairing 30 kr de fir restart kr auto connect hoga
// khud." Ye value HARDCODED hai — koi env var (GOLDMD_MAX_SESSIONS) ise
// override NAHI kar sakta. 30 pairings per server.
// ══════════════════════════════════════════════════════════════════════════
func maxPairedSessions() int {
	return 30 // ✅ OWNER ORDER (2026): max pairing 30 per server.
}

type Session struct {
	JID     string
	Owner   string // the number that owns/paired this bot session
	Client  *whatsmeow.Client
	Manager *Manager
	Started time.Time

	// LoginDeadSince: logout-linter (watchdog) ka grace-window timestamp.
	// Socket zinda + IsLoggedIn() false pe pehli baar set; 60s+ dead pe
	// cleanupSession. Login wapas aane pe reset. (515 relogin transient safe)
	LoginDeadSince time.Time

	// Bot-wide TEXT skin (.botstyle): the resolved style is cached per
	// session so every send path does not re-read Redis. skinLoaded is
	// cleared whenever .botstyle changes. Style 1 / empty == no skin.
	skinStyle  goldcmds.MenuStyle
	skinValue  string
	skinLoaded bool

	// Paired is true only after WhatsApp confirms the link
	// (*events.PairSuccess / Store.ID != nil). Until then the session is
	// "pending": a pairing code was generated but the user has NOT linked it
	// in WhatsApp yet. Pending sessions are NOT counted and NOT saved to Redis.
	Paired bool

	// LocalOnly = DISK-ONLY session (owner order): direct /code?phone= se
	// pair hua. Session creds SIRF disk pe (goldmd.db + pairing folder) —
	// Storj pe kabhi nahi jata (SaveSessionDB upload-guard isi se block
	// hota hai). Restart pe AutoLoad disk se uthata hai, fleet guards
	// bypass. Disk data KABHI delete nahi hota (khabardar rule).
	LocalOnly bool

	// msgCache holds recent incoming message protos keyed by message ID, so
	// command handlers (e.g. the aivideo multi-image flow) can download media
	// attached to a message after it has been routed. Entries expire after a
	// few minutes to bound memory.
	msgCache   map[string]*waProto.Message
	msgCacheMu sync.Mutex

	// statusSeenIDs dedups incoming status broadcasts: WhatsApp sometimes
	// delivers the same status message twice, which would cause double
	// auto-reply / double auto-react. Keyed by status message ID.
	statusSeenIDs   map[string]struct{}
	statusSeenIDsMu sync.Mutex

	// bangcuserCooldown rate-limits the "banned user" warning notice in
	// bangcuser enforcement (handler.go). Keyed by "groupJID:userJID".
	// Prevents flooding: only one notice per 30 seconds per (group, user).
	bangcuserCooldown   map[string]time.Time
	bangcuserCooldownMu sync.Mutex
}

// cacheMessage stores a raw incoming message proto keyed by its ID.
func (s *Session) cacheMessage(id string, msg *waProto.Message) {
	if id == "" || msg == nil {
		return
	}
	s.msgCacheMu.Lock()
	defer s.msgCacheMu.Unlock()
	if s.msgCache == nil {
		s.msgCache = make(map[string]*waProto.Message)
	}
	// OWNER REQUEST (RAM 30-40 MB): cache cap tightened 200 → 50 entries.
	if len(s.msgCache) > 50 {
		for k := range s.msgCache {
			delete(s.msgCache, k)
			break
		}
	}
	s.msgCache[id] = msg
}

// getCachedMessage returns a previously cached message proto by ID, or nil.
func (s *Session) getCachedMessage(id string) *waProto.Message {
	s.msgCacheMu.Lock()
	defer s.msgCacheMu.Unlock()
	return s.msgCache[id]
}

// extractImageMessage returns the *waProto.ImageMessage from a message proto,
// handling direct images, view-once wrappers, and quoted images. Returns nil
// when no image is present.
func extractImageMessage(msg *waProto.Message) *waProto.ImageMessage {
	if msg == nil {
		return nil
	}
	if msg.ImageMessage != nil {
		return msg.ImageMessage
	}
	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		if img := msg.ViewOnceMessage.Message.ImageMessage; img != nil {
			return img
		}
	}
	if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		if img := msg.ViewOnceMessageV2.Message.ImageMessage; img != nil {
			return img
		}
	}
	// Quoted image inside an extended text message
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
		q := msg.ExtendedTextMessage.ContextInfo.QuotedMessage
		if q != nil && q.ImageMessage != nil {
			return q.ImageMessage
		}
	}
	return nil
}

type Manager struct {
	cfg       *Config
	container *sqlstore.Container
	Redis     *Upstash

	mu       sync.Mutex
	sessions map[string]*Session // key = JID

	// slotMu guards slotRes — the CONNECT-LEVEL quota reservation. Sirf
	// in-flight StartSession/PairWithCode goroutines ko yahan mark karo
	// (Count() in-flight sessions ko nahi dekh sakta — connect hone tak
	// wo map me nahi aati, isliye BOOT CAP/FLEET CAP/panel quota race me
	// sab goroutines Count()=0 dekh kar pass ho jate the). Reservation
	// SUCCESS pe session ke jid-key se merge ho jati hai (same key — no
	// double count), FAIL pe releaseSlot se hat jati hai. Session ke
	// zinda rehne tak ye set usi jid ko rakhta hai — Count() usko
	// connected session se replace kar deta hai.
	slotMu  sync.Mutex
	slotRes map[string]time.Time // jid → reserve time

	shutdown bool
}

func NewManager(cfg *Config, container *sqlstore.Container) *Manager {
	return &Manager{
		cfg:       cfg,
		container: container,
		sessions:  map[string]*Session{},
		slotRes:   map[string]time.Time{},
	}
}

// ── lifecycle ─────────────────────────────────────────────────────────────

func (m *Manager) IsShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdown
}

// Count returns the number of sessions that WhatsApp has confirmed as
// actually linked (PairSuccess received / Store.ID set). Pending sessions
// (code generated but not yet linked in WhatsApp) are NOT counted, because
// WhatsApp is the source of truth, not Redis and not the pairing folder.
// SLOT RESERVATION — connect-level quota. Count() sirf connected sessions
// ginta hai, aur StartSession/PairWithCode session ko connect hone ke BAAD
// map me daalte hain — isliye in-flight connects (AutoLoad batch-5 mass-boot,
// concurrent panel pairings, fleet claim + panel race) Count() ke liye
// invisible the. Race window me 5 goroutines ek saath Count()=0 dekh kar
// pass ho jati thi → FULL 5/2 over-max (owner ne ye exactly dekha tha).
// Fix: pehle SLOT reserve karo, phir connect karo.
//
// Model: slotRes[jid] = reservation. Live session (jid map me, WhatsApp-truth
// connected) aur uska reservation SAME jid-key share karte hain → jid-level
// dedup → double-count kabhi nahi. Orphan reservation (crash/leak) 10 min
// me self-expire — quota permanently block nahi hota.

// slotsUsedLocked: live sessions + fresh in-flight reservations (quota view).
// Lock order hamesha slotMu → mu (caller dono hold kare).
func (m *Manager) slotsUsedLocked() int {
	live := map[string]bool{}
	for j, s := range m.sessions {
		if s != nil && s.Paired && s.Client != nil &&
			s.Client.IsConnected() &&
			s.Client.Store != nil && s.Client.Store.ID != nil {
			live[j] = true
		}
	}
	used := len(live)
	stale := 10 * time.Minute
	for r, ts := range m.slotRes {
		if live[r] {
			continue // reservation live session ke saath merged — overlap OK
		}
		if time.Since(ts) > stale {
			delete(m.slotRes, r) // self-heal: leaked reservation expire
			continue
		}
		used++ // fresh in-flight reservation — quota me gino
	}
	return used
}

// SlotsUsed: public quota view — live + in-flight dono (Count() sirf live).
// Panel pairing pre-check + fleet claim cycle + AutoLoad BOOT CAP yehi use
// karte hain taake race window me bhi quota sach bole.
func (m *Manager) SlotsUsed() int {
	if m == nil {
		return 0
	}
	m.slotMu.Lock()
	defer m.slotMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.slotsUsedLocked()
}

// reserveSlot: jid ke liye connect-slot book karo. Idempotent — jid already
// WhatsApp-truth live hai to true (fleet retry / re-pair / reconnect).
// Quota (live + in-flight) full hai to false.
func (m *Manager) reserveSlot(jid string) bool {
	if m == nil {
		return false
	}
	m.slotMu.Lock()
	defer m.slotMu.Unlock()
	if m.slotRes == nil {
		m.slotRes = map[string]time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// already live? — idempotent reserve (koi naya slot nahi chahiye)
	if s, ok := m.sessions[jid]; ok && s != nil && s.Paired && s.Client != nil &&
		s.Client.IsConnected() &&
		s.Client.Store != nil && s.Client.Store.ID != nil {
		return true
	}
	if m.slotsUsedLocked() >= maxPairedSessions() {
		return false // quota full — live + in-flight dono gine gaye
	}
	m.slotRes[jid] = time.Now()
	return true
}

// releaseSlot: StartSession/PairWithCode ke ERROR paths + cleanupSession pe
// slot free karo. Connect SUCCESS pe slot live session ke saath merge hota
// hai (same jid key) — release ki zaroorat nahi.
func (m *Manager) releaseSlot(jid string) {
	if m == nil {
		return
	}
	m.slotMu.Lock()
	defer m.slotMu.Unlock()
	delete(m.slotRes, jid)
}

func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sessions {
		// WHATSAPP IS TRUTH: sirf Paired flag (memory/Storj-restore) kaafi
		// nahi. Socket zinda + device linked tabhi gino — WhatsApp logout
		// kar de to Storj/flag jhuta bole to bhi count sahi rahega.
		if s != nil && s.Paired && s.Client != nil &&
			s.Client.IsConnected() &&
			s.Client.Store != nil && s.Client.Store.ID != nil {
			n++
		}
	}
	return n
}

// cleanupPending removes a session that never completed pairing (the user
// generated a code but never linked it in WhatsApp). It disconnects the
// client and clears any stale Redis entry so WhatsApp remains the truth.
// Safe to call multiple times.
func (m *Manager) cleanupPending(jid string) {
	m.mu.Lock()
	sess, ok := m.sessions[jid]
	m.mu.Unlock()
	if !ok || sess == nil {
		return
	}
	// If it actually got paired while we were waiting, leave it alone.
	if sess.Client != nil && sess.Client.Store != nil && sess.Client.Store.ID != nil {
		sess.Paired = true
		return
	}
	WarnLog("Pending pairing for %s never completed in WhatsApp — clearing stale session (WhatsApp is truth)", jid)
	// DISK-ONLY (owner order): pending (never-linked) pairing ka session
	// hi nahi bana — sirf expired pair-code marker folder tha. Marker +
	// folder hatana zaroori hai taake AutoLoad har restart pe is jid ka
	// kutta-pallva na karta rahe. Ye DISK SESSION ka deletion NAHI hai
	// (creds/device row kabhi bani hi nahi — WhatsApp ne link reject/expire
	// kiya). Linked session ka folder cleanupSession ke LocalOnly-guard se
	// SAFE rehta hai.
	if isLocalOnlyJID(localPairingDir(), jid) {
		_ = os.RemoveAll(localMarkerDirFor(jid))
	}
	m.cleanupSession(sess, "pairing code never linked in WhatsApp")
}

// AlreadyConnected reports whether a WhatsApp session for the given phone
// number already exists and is actively connected. The panel uses this to
// short-circuit duplicate pairing attempts for an already-live bot.
func (m *Manager) AlreadyConnected(phone string) bool {
	jid := normalizeJID(phone)
	if jid == "" {
		return false
	}
	m.mu.Lock()
	sess, ok := m.sessions[jid]
	m.mu.Unlock()
	if !ok || sess == nil || sess.Client == nil {
		return false
	}
	// TRUST WHATSAPP: IsConnected() only means the websocket is open, not
	// that the device is actually linked. Store.ID is set by whatsmeow ONLY
	// after a successful pairing/login. Require both so a pending (code
	// generated but never linked) session is NOT reported as connected.
	if !sess.Client.IsConnected() {
		return false
	}
	if sess.Client.Store == nil || sess.Client.Store.ID == nil {
		return false
	}
	return sess.Paired
}

func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// AutoLoad scans the pairing directory and reconnects every saved session
// in batches (mirrors autoload.js UmarProcessBatch).
func (m *Manager) AutoLoad() {
	// ensure pairing dir exists
	_ = os.MkdirAll(m.cfg.PairingDir, 0o755)

	entries, err := os.ReadDir(m.cfg.PairingDir)
	if err != nil {
		WarnLog("Pairing dir not readable: %v", err)
		return
	}

	var users []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), m.cfg.PairingSuffix) {
			users = append(users, e.Name())
		}
	}

	// LOCAL-ONLY store scan (owner order): direct /code?phone= sessions ALAG
	// folder (nexstore/local/pairing) me hain — reconnector inhe bhi uthaye.
	users = appendLocalOnlyJIDs(users)

	// Also pull JIDs from Redis registry in case the pairing folder was wiped
	// but Redis still remembers them (this is the Render-restart scenario).
	if m.Redis != nil {
		redisJids := m.Redis.ListJIDs()
		//		JSONDebug("AUTOLOAD_REDIS_JIDS", map[string]any{"jids": redisJids})
		for _, rj := range redisJids {
			already := false
			for _, u := range users {
				if u == rj {
					already = true
					break
				}
			}
			if !already {
				// recreate the pairing folder so the rest of AutoLoad logic is uniform
				_ = os.MkdirAll(filepath.Join(m.cfg.PairingDir, rj), 0o755)
				users = append(users, rj)
				//				JSONDebug("AUTOLOAD_REDIS_ADD", map[string]any{"jid": rj, "source": "redis"})
			}
		}
	}

	// Path B fix: GLOBAL fleet sessions set (goldmd:fleet:sessions) as a JID
	// source. Per-server JID registry (goldmd:sessiondb:<sid>:jids) aksar KHAALI
	// hoti hai (URL-isolated), lekin global fleet set me saare known JIDs hote
	// hain. Isse pairing folder wipe hone par bhi session local folder me aata
	// hai. (Connect-time guards ab bhi lagu — agar session kisi live server pe
	// ONLINE hai to StartSession usse rok deta hai, war nahi hoti.)
	if m.Redis != nil && fleetActive() {
		for _, fj := range m.Redis.setMembers(fleetSessionsSet) {
			if fj == "" {
				continue
			}
			already := false
			for _, u := range users {
				if u == fj {
					already = true
					break
				}
			}
			if !already {
				_ = os.MkdirAll(filepath.Join(m.cfg.PairingDir, fj), 0o755)
				users = append(users, fj)
				InfoLog("AUTOLOAD: fleet sessions set se JID mila: %s", fj)
			}
		}
	}

	// Fallback: if both pairing folders AND Redis JID registry are empty,
	// scan the restored SQLite DB directly for stored devices. This catches
	// the case where JID registry was cleared but the DB blob still holds
	// valid credentials (e.g. a botched cleanup / older code version).
	if len(users) == 0 && m.container != nil {
		//		JSONDebug("AUTOLOAD_DB_SCAN_START", map[string]any{"reason": "no users from folders/redis, scanning DB"})
		allDevs, derr := m.container.GetAllDevices(context.Background())
		if derr != nil {
			WarnLog("DB scan for devices failed: %v", derr)
		} else {
			//			JSONDebug("AUTOLOAD_DB_SCAN", map[string]any{"deviceCount": len(allDevs)})
			for _, d := range allDevs {
				if d.ID == nil || d.ID.User == "" {
					continue
				}
				// build the BASE jid (strip the :device suffix) so StartSession
				// and the pairing folder match the original pairing flow.
				baseJid := d.ID.User + "@" + d.ID.Server
				already := false
				for _, u := range users {
					if u == baseJid {
						already = true
						break
					}
				}
				if !already {
					_ = os.MkdirAll(filepath.Join(m.cfg.PairingDir, baseJid), 0o755)
					users = append(users, baseJid)
					//					JSONDebug("AUTOLOAD_DB_ADD", map[string]any{
					//						"baseJid":   baseJid,
					//						"storedJid": d.ID.String(),
					//						"source":    "db_scan",
					//					})
					// also register in Redis so future restarts find it via registry
					if m.Redis != nil {
						_ = m.Redis.RegisterJID(baseJid)
					}
				}
			}
		}
	}

	//	JSONDebug("AUTOLOAD_USERS", map[string]any{"count": len(users), "users": users})

	if len(users) == 0 {
		WarnLog("No paired users found in %s", m.cfg.PairingDir)
		InfoLog("Hint: POST a number to the panel (http://0.0.0.0:%d/pair) or run with GOLDMD_DEBUG=1", m.cfg.PanelPort)
		return
	}

	OkLog("Found %d paired users. Starting connections in batches of %d...", len(users), m.cfg.BatchSize)
	start := time.Now()
	successful := 0

	batchSize := m.cfg.BatchSize
	totalBatches := (len(users) + batchSize - 1) / batchSize

	for i := 0; i < len(users); i += batchSize {
		if m.IsShuttingDown() {
			WarnLog("Stopping batch processing due to shutdown")
			break
		}

		batchNum := i/batchSize + 1
		end := i + batchSize
		if end > len(users) {
			end = len(users)
		}
		batch := users[i:end]

		InfoLog("Processing batch %d/%d (%d users)", batchNum, totalBatches, len(batch))

		var wg sync.WaitGroup
		for idx, user := range batch {
			wg.Add(1)
			go func(u string, n int) {
				defer wg.Done()
				if m.IsShuttingDown() {
					return
				}
				InfoLog("Connecting %d/%d: %s", n, len(users), u)
				// BOOT CAP: is server ka quota (maxPerServer) already full hai
				// to over-max session connect MAT karo. Mass-boot race me
				// (Render auto-deploy pe saare servers ek saath uthte hain)
				// fleet claims abhi bane hi nahi hote the — pehle version
				// is race window me 3/2 jaisa over-max load kar deta tha.
				// Over-max jids ko chhod dena SAFE hai: fleet blob GLOBAL hai,
				// watchdog inhe baad me doosre server pe claim kar dega.
				if m.SlotsUsed() >= maxPairedSessions() {
					WarnLog("BOOT CAP: %s skipped — server full (%d/%d), fleet baad me sambhalega", u, m.SlotsUsed(), maxPairedSessions())
					return
				}
				// DISK-ONLY (owner order): local-only session KABHI fleet
				// guards se nahi ruka jaata — ye is server ki DISK property
				// hai. Stale claims (jo humne abhi clean ki) chahe duniya me
				// kahin bhi padi hon, AutoLoad disk se connect karega hi.
				// Fleet blob is JID ka Storj pe hai hi nahi → koi aur server
				// connect kar hi nahi sakta → war ka saval hi nahi.
				if !isLocalOnlyJID(m.cfg.PairingDir, u) {
					// ZOMBIE-RETURN GUARD: ye session kisi AUR live server ke fresh
					// claim me hai (failover ho chuka) to yahan start mat karo —
					// double-connect war WhatsApp logout karva deta hai. Fleet
					// watchdog us server ke marne pe ye session wapas le lega.
					if fleetHeldByLiveServer(u) {
						InfoLog("FLEET: skip %s — live claim on another server (failover target)", u)
						return
					}
					// ONLINE-ELSEWHERE GUARD (owner order): claim free ho tab bhi
					// CHECK karo ke session kisi AUR server pe already ONLINE to
					// nahi (remote /sessions probe). Online hai = koi aur server
					// isko chala raha hai = IGNORE, reconnect ki zaroorat nahi.
					// Sirf OFFLINE (magar logged-in) session hi reconnect hoga.
					if fleetSessionOnlineElsewhere(u) {
						InfoLog("FLEET: skip %s — session already ONLINE on another server (no reconnect needed)", u)
						return
					}
				}
				if err := m.StartSession(u); err != nil {
					ErrLog("Failed for %s: %v", u, err)
					JSONDebugErr("AUTOLOAD_CONNECT_FAIL", err, map[string]any{"jid": u})
					return
				}
				OkLog("Connected: %s", u)
				successful++
			}(user, i+idx+1)
		}
		wg.Wait()

		// delay between batches (like autoload.js UmarDelay(2000))
		if end < len(users) && !m.IsShuttingDown() {
			InfoLog("Waiting %d seconds before next batch...", m.cfg.BatchDelaySec)
			time.Sleep(time.Duration(m.cfg.BatchDelaySec) * time.Second)
		}
	}

	dur := time.Since(start).Seconds()
	failed := len(users) - successful
	OkLog("Auto-load completed in %.2fs | Success: %d | Failed: %d | Total: %d",
		dur, successful, failed, len(users))
}

// StartSession connects (or reconnects) a WhatsApp session for the given JID.
// The JID is also used as the pairing dir name so the Node bot's directory
// pattern is preserved.
//
// FIX: previously this called m.container.GetFirstDevice(), which always
// returns the FIRST device row in the sqlite store — not the device that
// actually belongs to this jid. On restart, every saved session ended up
// sharing the same device/credentials and fighting over the same WhatsApp
// connection, which WhatsApp kills with a stream-replace / logout, and the
// LoggedOut handler then deletes the session from the map instead of it
// ever reconnecting. Using GetDevice(ctx, jid) fixes this by loading each
// session's own stored device.
func (m *Manager) StartSession(jid string) error {
	if m.IsShuttingDown() {
		return fmt.Errorf("shutdown in progress")
	}
	// SLOT CAP (race-safe FLEET CAP): pehle slot RESERVE karo, connect baad
	// me. Purana Count()-based check in-flight connects ko nahi dekh sakta
	// tha — AutoLoad batch-5 / concurrent pairings race window me sab
	// Count()=0 dekh kar pass ho jate the (FULL 5/2 over-max). reserveSlot
	// live + in-flight dono ginta hai, atomic (slotMu). Failover path bhi
	// yahin se hi quota paata hai — fail ke pe HDEL claim + cooldown
	// (fleet.go) isliye failover kabhi hard-block nahi hota, sirf clean
	// retry hota hai. Fleet off (direct instance) me GOLDMD_MAX_SESSIONS
	// hi quota hai — AutoLoad BOOT CAP ke barabar.
	if !m.reserveSlot(jid) {
		return fmt.Errorf("server full (%d/%d) — fleet is active, try another server or wait for failover", m.SlotsUsed(), maxPairedSessions())
	}
	ok := false
	defer func() {
		if !ok {
			m.releaseSlot(jid) // connect fail/qr timeout/no-device — slot wapas
		}
	}()

	// local-only JID → marker ALAG folder me; fleet JID → global pairing dir.
	if isLocalOnlyJID(localPairingDir(), jid) {
		_ = os.MkdirAll(localMarkerDirFor(jid), 0o755)
	} else {
		// ensure pairing dir exists for this jid (marks it as paired)
		pairDir := filepath.Join(m.cfg.PairingDir, jid)
		_ = os.MkdirAll(pairDir, 0o755)
	}

	//	JSONDebug("RECONNECT_START", map[string]any{
	//		"jid":      jid,
	//		"pairDir":  pairDir,
	//		"redisOn":  m.Redis != nil,
	//		"redisHas": m.Redis != nil && m.Redis.HasJID(jid),
	//	})

	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("parse jid %s: %w", jid, err)
	}

	// fetch THIS jid's device from the shared container.
	// NOTE: whatsmeow stores devices with an AD-JID (user:device@server),
	// e.g. "923158930864:52@s.whatsapp.net". But our pairing folder / Redis
	// registry uses the BASE jid ("923158930864@s.whatsapp.net"). An exact
	// GetDevice(baseJID) returns nil because device 0 != stored device number.
	// Fix: try exact first, then fall back to GetAllDevices and pick the
	// device whose user matches (ignore the device suffix).
	devCon := m.deviceContainerFor(jid)
	if devCon == nil {
		devCon = m.container
	}
	dev, err := devCon.GetDevice(context.Background(), parsedJID)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	//devFoundBy := "exact"
	if dev == nil {
		// Exact match failed — scan all devices for a matching user.
		//		JSONDebug("RECONNECT_DEVICE_FALLBACK", map[string]any{
		//			"jid":    jid,
		//			"reason": "exact GetDevice nil, trying user match across all devices",
		//		})
		allDevs, aerr := devCon.GetAllDevices(context.Background())
		if aerr != nil {
			return fmt.Errorf("get all devices: %w", aerr)
		}
		baseUser := parsedJID.User
		//		JSONDebug("RECONNECT_ALL_DEVICES", map[string]any{
		//			"baseUser":  baseUser,
		//			"totalDevs": len(allDevs),
		//		})
		for _, d := range allDevs {
			if d.ID != nil && d.ID.User == baseUser && d.ID.Server == parsedJID.Server {
				// DISK-ONLY / multi-row safety (owner order): same user ke
				// multiple device rows ho sakti hain (purana logged-out
				// row + naya linked row — direct re-pair ke baad aisa hota
				// hai). HIGHEST device number = sabse naya link wahi uthao
				// — purani dead creds se connect karne pe bot restart ke
				// baad wapas online hi nahi hota.
				if dev == nil || d.ID.Device > dev.ID.Device {
					dev = d
				}
			}
		}
	}
	//devJidStr := ""
	if dev != nil && dev.ID != nil {
		//devJidStr = dev.ID.String()
	}
	//	JSONDebug("RECONNECT_DEVICE", map[string]any{
	//		"jid":      jid,
	//		"found":    dev != nil,
	//		"foundBy":  devFoundBy,
	//		"hasCreds": dev != nil && dev.ID != nil,
	//		"devJid":   devJidStr,
	//	})
	if dev == nil {
		// OWNER FIX ("old sessions load hi nahi hote"): device disk pe nahi
		// mila — LEKIN fleet blob Storj pe ho sakta hai (ye session kisi
		// aur server pe pair hua tha). Pehle WALI Storj se restore karo —
		// 45s verify-window ke andar hi WhatsApp truth bhi mil jayega.
		// BLOB MISSING = sach me purge/mara hua → neeche wala purana path.
		// (local-only direct pair ka blob hota hi nahi — restore no-op.)
		if !isLocalOnlyJID(m.cfg.PairingDir, jid) && fleetRestoreBlobFromStorj(jid) {
			// restore ho gaya — ab dobara device lookup
			parsed2, perr := types.ParseJID(jid)
			if perr == nil {
				if d2, derr := m.container.GetDevice(context.Background(), parsed2); derr == nil && d2 != nil {
					dev = d2
				} else {
					allDevs2, _ := m.container.GetAllDevices(context.Background())
					for _, d := range allDevs2 {
						if d.ID != nil && d.ID.User == parsed2.User && d.ID.Server == parsed2.Server {
							if dev == nil || d.ID.Device > dev.ID.Device {
								dev = d
							}
						}
					}
				}
			}
		}
	}
	if dev == nil {
		// WhatsApp has no saved device for this JID.
		// Check Redis: if Redis says this JID is registered, Redis is lying
		// (WhatsApp is the source of truth — no device = logged out or never paired).
		// Clear the stale Redis entry so AutoLoad doesn't keep trying.
		if m.Redis != nil && m.Redis.HasJID(jid) {
			WarnLog("WhatsApp has no device for %s but Redis says connected — WhatsApp is truth, clearing stale Redis entry", jid)
			// DISK-ONLY (owner order — khabardar): is JID ka pairing folder
			// KABHI delete nahi hota. Ye sirf non-local-only sessions ke
			// liye valid cleanup hai. Local-only JID ka folder + DB row
			// disk pe SAFE — reconnector AutoLoad me ise uthata rahega.
			if !isLocalOnlyJID(localPairingDir(), jid) {
				_ = m.Redis.RemoveJID(jid)
				// Also remove the pairing folder so AutoLoad skips it next time.
				// NOTE (owner rule): fleet blob (goldmd:fleet:sess:<jid>) SAFE
				// rehta hai — wahi GLOBAL backup hai jis se koi bhi server is
				// session ko restore kar sakta hai. Local device missing = DB
				// wipe/corruption, WhatsApp ka logout statement NAHI. Watchdog
				// blob se revive karega; agar blob sach me mara hua hai to 3x
				// restore-fail purge (fleet.go) khud sambhal lega.
				_ = os.RemoveAll(filepath.Join(m.cfg.PairingDir, jid))
			}
		}
		return fmt.Errorf("no saved device found for %s (was it ever paired?)", jid)
	}

	cliLogger := waLog.Noop
	cli := whatsmeow.NewClient(dev, cliLogger)
	cli.EnableAutoReconnect = true
	// 0 = pehla built-in auto-reconnect attempt INSTANT (delay =
	// AutoReconnectErrors * 2s). Pehle 10 tha -> 20s wait, har fail pe
	// aur badhta jata. Ab watchdog + fast autoreconnect dono saath hain.
	cli.AutoReconnectErrors = 0

	sess := &Session{
		JID:       jid,
		Owner:     strings.TrimSuffix(jid, m.cfg.PairingSuffix),
		Client:    cli,
		Manager:   m,
		Started:   time.Now(),
		LocalOnly: isLocalOnlyJID(m.cfg.PairingDir, jid),
	}

	// register our event router
	cli.AddEventHandler(sess.EventHandler)

	// connect (QR or existing creds)
	if cli.Store.ID == nil {
		// no saved creds → we need a pairing. For QR-based flow we print QR
		// to terminal AND expose it on the panel. For number-based flow the
		// panel calls PairWithCode() which sets the code first.
		if err := sess.connectWithQR(); err != nil {
			return fmt.Errorf("qr connect: %w", err)
		}
	} else {
		if err := cli.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	// This is a RECONNECT: the device already existed in the store, which
	// means WhatsApp previously confirmed the link (Store.ID != nil). Mark
	// the session Paired so Count() counts it.
	// This is a RECONNECT: the device already existed in the store, which
	// means WhatsApp previously confirmed the link. Mark Paired true only
	// if the device actually has credentials (Store.ID != nil). WhatsApp is truth.
	if sess.Client != nil && sess.Client.Store != nil && sess.Client.Store.ID != nil {
		sess.Paired = true
	}

	// ══ WHATSAPP-TRUTH VERIFY WINDOW (owner order) ════════════════════════
	// Session JID chahe jis bhi URL se aaya ho (boot-restore, failover,
	// claim, panel) — Connect() sirf websocket kholta hai; WhatsApp ka
	// login/logout faisla ASYNC event me 1-3s baad aata hai. Purana code
	// turant JID re-register + blob save kar deta tha, aur logout event
	// cleanup ke BAAD late save land ho jata tha → dead JID wapas Storj.
	// Ab pehle VERIFY karo (12s window, sync):
	//   • IsLoggedIn() true → WhatsApp ne login confirm kiya → reconnect
	//     complete, registration/save aage chalenge (kisi bhi server pe).
	//   • Store.Deleted / session map se hata → WhatsApp ne LOGOUT bola →
	//     silent purge (fleetPurgeLoggedOutSession — config SAFE) aur
	//     StartSession error ke saath nikal jao. Kabhi re-register NAHI.
	{
		verified, explicitLogout, whyNot := whatsappTruthVerified(sess, 12*time.Second)
		if explicitLogout {
			// WHATSAPP NE KHUD LOGOUT BOLA (401/403/410/device-removed):
			// silently ignore — session data har taraf se purge (fleet
			// blob, meta, set, claim, own jids, blob refresh/delete).
			// CONFIGURATION (settings:<jid>) SAFE — re-pair par wapas.
			OkLog("WHATSAPP TRUTH: %s — logout confirm, silent purge (config safe) [%s]", jid, whyNot)
			fleetPurgeLoggedOutSession(jid)
			return fmt.Errorf("whatsapp logged out %s — session data purged, configuration kept", jid)
		}
		// verified=true (login OK) ya transient (na login na logout — slow
		// network / 515 race): session REGISTER karo, aage ki normal flow
		// chale. Transient case ko runtime safety-nets sambhalte hain —
		// LoggedOut event → cleanupSession → silent purge, watchdog linter
		// (60s login-dead) → cleanupSession → purge. Is tarah client ka
		// background socket orphan nahi rehta (agar yahan error return
		// karte to session map me kabhi na aata lekin socket zinda reh
		// jata — commands isko kabhi nahi dhundhte).
		if verified {
			OkLog("WHATSAPP TRUTH: %s — WhatsApp login confirmed, reconnect complete", jid)
		} else {
			OkLog("WHATSAPP TRUTH: %s — inconclusive [%s] — registered, runtime linter sambhalega", jid, whyNot)
		}
	}

	// AUTOBLOCK CONTACT SYNC — force-fetch the WhatsApp server's contact
	// list (critical_unblock_low patch) right after every reconnect. The
	// normal bootstrap only resyncs if not synced (onlyResyncIfNotSynced=
	// true), so a stale DB would keep old data forever. This keeps the
	// saved-contact exemption (autoblock contact mode) fresh without the
	// owner having to run .autoblock sync manually. Fire-and-forget: it
	// runs in the background and can never block the bot startup.
	if sess.Client != nil && sess.Client.Store != nil && sess.Client.Store.ID != nil {
		go func(cli *whatsmeow.Client) {
			defer func() { _ = recover() }()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_ = cli.FetchAppState(ctx, appstate.WAPatchCriticalUnblockLow, true, false)
		}(sess.Client)
	}
	m.mu.Lock()
	m.sessions[jid] = sess
	m.mu.Unlock()

	// Register this JID in Redis so AutoLoad can find it after a restart.
	// This is a RECONNECT (device already existed in the store), which means
	// WhatsApp confirmed the session is still valid.
	// DISK-ONLY (owner order): local-only JID Storj registry/DB me NAHI —
	// marker file + goldmd.db hi iski identity hai.
	if m.Redis != nil && !sess.LocalOnly {
		if err := m.Redis.RegisterJID(jid); err != nil {
			// ErrLog("Failed to register JID %s in Redis: %v", jid, err)
		} else {
			//			JSONDebug("REDIS_REGISTER", map[string]any{"jid": jid})
		}
		// Also save the session DB right after a reconnect so any key changes
		// during the previous run are persisted to Redis immediately.
		if err := m.Redis.SaveSessionDB(filepath.Join(m.cfg.DataDir, "goldmd.db")); err != nil {
			// ErrLog("Failed to save session DB after reconnect: %v", err)
		} else {
			//			JSONDebug("REDIS_SAVE", map[string]any{"jid": jid, "stage": "after_reconnect"})
		}
	}

	//	JSONDebug("RECONNECT_OK", map[string]any{"jid": jid})
	ok = true // connect SUCCESS — reservation live session me merge (same jid key)
	return nil
}

// connectWithQR generates a QR, prints it to the terminal, and registers it
// on the QR panel so a phone can scan it. It blocks until paired or timeout.
func (s *Session) connectWithQR() error {
	cli := s.Client

	qrChan, err := cli.GetQRChannel(context.Background())
	if err != nil {
		// fallback: direct connect (some whatsmeow versions)
		return cli.Connect()
	}

	if err := cli.Connect(); err != nil {
		return err
	}

	// store the latest QR so the panel can serve it
	s.Manager.mu.Lock()
	if s.Manager.sessions == nil {
		s.Manager.sessions = map[string]*Session{}
	}
	s.Manager.mu.Unlock()

	timeout := time.NewTimer(120 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case evt, ok := <-qrChan:
			if !ok {
				return nil
			}
			if evt.Event == "code" {
				// DISABLED — no console output per owner request
				// fmt.Printf("\n%s 📱 Scan this QR for session %s%s\n", cCyan, s.JID, cReset)
				// qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// fmt.Printf("%s────────────────────────%s\n", cGray, cReset)
				_ = evt
			}
		case <-timeout.C:
			return fmt.Errorf("QR pairing timed out (120s)")
		case <-time.After(2 * time.Second):
			// check if connected
			if cli.IsConnected() && cli.Store.ID != nil {
				return nil
			}
		}
	}
}

// PairWithCode starts a session using a phone number + pairing code (no QR).
// The user enters the code in WhatsApp → Link a device.
//
// FIX: previously used m.container.GetFirstDevice(), which — once any
// session already existed — would hand a brand-new pairing the FIRST
// session's device/credentials instead of a clean one. Using NewDevice()
// guarantees every new pairing gets its own fresh device record.
func (m *Manager) PairWithCode(phone string) (string, error) {
	return m.pairWithCodeMode(phone, false)
}

// PairWithCodeDirect: direct /code?phone= endpoint ka DISK-ONLY mode (owner
// order). Session + creds sirf local disk pe — Storj pe KUCH nahi jata.
// Config (settings) wapas Storj se hi aati hai (owner clarification).
func (m *Manager) PairWithCodeDirect(phone string) (string, error) {
	return m.pairWithCodeMode(phone, true)
}

func (m *Manager) pairWithCodeMode(phone string, localOnly bool) (string, error) {
	if m.IsShuttingDown() {
		return "", fmt.Errorf("shutdown in progress")
	}

	jid := normalizeJID(phone)
	if jid == "" {
		return "", fmt.Errorf("invalid phone number")
	}

	// TRUST WHATSAPP OVER REDIS: if a previous pairing code was generated for
	// this number but the user never actually linked it in WhatsApp, that
	// stale session is still sitting in our map / Redis / pairing folder and
	// would inflate the count and block new pairings. Clear it now so we can
	// give a fresh code. WhatsApp is the truth: if Store.ID is nil the device
	// was never linked, so Redis's "connected" claim is a lie.
	m.mu.Lock()
	oldSess, hadOld := m.sessions[jid]
	m.mu.Unlock()
	if hadOld && oldSess != nil {
		linked := oldSess.Client != nil && oldSess.Client.Store != nil && oldSess.Client.Store.ID != nil
		if !linked {
			WarnLog("Stale unlinked session for %s found — WhatsApp says NOT linked. Clearing Redis + giving fresh code (WhatsApp is truth)", jid)
			m.cleanupSession(oldSess, "previous pairing code never linked in WhatsApp")
		}
	}

	//	JSONDebug("PAIR_START", map[string]any{
	//		"phone":      phone,
	//		"jid":        jid,
	//		"pairingDir": m.cfg.PairingDir,
	//		"redisOn":    m.Redis != nil,
	//	})

	// SLOT CAP (race-safe): concurrent /pair requests dono Count()=0 dekh
	// kar pass ho jate the. Ab pehle slot reserve — pending pairing bhi
	// quota occupy karega (120s watchdog cleanup pe release). Stale-cleanup
	// ke BAAD reserve karo (cleanup releaseSlot karta hai — warna apni hi
	// reservation ud jati).
	if !m.reserveSlot(jid) {
		return "", fmt.Errorf("server full (%d/%d) — try another server", m.SlotsUsed(), maxPairedSessions())
	}
	pairedOK := false
	defer func() {
		if !pairedOK {
			m.releaseSlot(jid)
		}
	}()

	if localOnly {
		// DIRECT /code?phone= (owner order): marker ALAG local folder me,
		// device row ALAG local.db me. Fleet /pair path yahan se untouched.
		writeLocalOnlyMarker(localPairingDir(), jid)
		localOnlyCleanFleetKeys(jid)
	} else {
		_ = os.MkdirAll(filepath.Join(m.cfg.PairingDir, jid), 0o755)
	}

	devCon := m.deviceContainerFor(jid)
	if devCon == nil {
		devCon = m.container
	}

	// brand-new pairing → brand-new device, never reuse another session's
	dev := devCon.NewDevice()

	cli := whatsmeow.NewClient(dev, waLog.Noop)
	cli.EnableAutoReconnect = true
	// 0 = pehla built-in auto-reconnect attempt INSTANT (delay =
	// AutoReconnectErrors * 2s). Pehle 10 tha -> 20s wait, har fail pe
	// aur badhta jata. Ab watchdog + fast autoreconnect dono saath hain.
	cli.AutoReconnectErrors = 0

	// IMPORTANT: Do NOT override SetOSInfo / PlatformType / fingerprint fields.
	// Issue #1191 (Jul 2026) proved that custom fingerprint overrides (Safari/macOS,
	// Android, etc) cause WhatsApp to reject pairing with 400 bad-request or
	// "Can't link new devices right now". whatsmeow defaults are the most stable.
	// Baileys (latest Jun 2026) also uses Chrome + macOS defaults without manual
	// fingerprint spoofing. PairClientChrome + "Chrome (Linux)" works (issue #1233).

	sess := &Session{
		JID:       jid,
		Owner:     phone,
		Client:    cli,
		Manager:   m,
		Started:   time.Now(),
		LocalOnly: localOnly,
	}

	cli.AddEventHandler(sess.EventHandler)

	// Whatsmeow requires the login websocket to be connected and fully
	// initialized before PairPhone can send the link-code IQ request.
	// Calling PairPhone before Connect causes: "websocket not connected".
	//
	// Correct order:
	//   1. GetQRChannel() — prepare the pairing channel
	//   2. Connect()      — open the websocket
	//   3. Wait for the first QR event — this is the readiness signal
	//      that the websocket handshake is complete
	//   4. PairPhone()    — now safe to send the link-code IQ request
	qrChan, err := cli.GetQRChannel(context.Background())
	if err != nil {
		return "", fmt.Errorf("prepare pairing channel: %w", err)
	}

	if err := cli.Connect(); err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}

	// Wait until WhatsApp emits the first QR event. We do NOT display this
	// QR (the panel uses phone-number pairing), but this event is the
	// readiness signal documented by whatsmeow before PairPhone is safe.
	select {
	case _, ok := <-qrChan:
		if !ok {
			return "", fmt.Errorf("pairing channel closed prematurely")
		}
	case <-time.After(15 * time.Second):
		return "", fmt.Errorf("readiness signal (QR event) timed out")
	}

	// Now the websocket is connected and ready — PairPhone is safe.
	pairingCode, err := cli.PairPhone(context.Background(), phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		//		JSONDebug("PAIR_ERR", map[string]any{"jid": jid, "error": err.Error()})
		return "", fmt.Errorf("pair phone: %w", err)
	}

	// Register the session as PENDING (Paired=false). It is in the map so the
	// EventHandler can find it when WhatsApp sends PairSuccess, but it is NOT
	// counted (Count only counts Paired=true) and NOT saved to Redis yet.
	// WhatsApp is the truth: only *events.PairSuccess confirms a real link.
	sess.Paired = false
	m.mu.Lock()
	m.sessions[jid] = sess
	m.mu.Unlock()

	// Watchdog: if the user never links the code in WhatsApp, clean up the
	// stale pending session after 120s so it doesn't linger in the map /
	// Redis / pairing folder and inflate the count or block future pairings.
	go func() {
		timer := time.NewTimer(120 * time.Second)
		defer timer.Stop()
		<-timer.C
		m.mu.Lock()
		cur, ok := m.sessions[jid]
		m.mu.Unlock()
		if !ok || cur != sess {
			return // replaced or already cleaned up
		}
		if cur.Paired {
			return // got linked, all good
		}
		linked := cur.Client != nil && cur.Client.Store != nil && cur.Client.Store.ID != nil
		if linked {
			cur.Paired = true
			return
		}
		WarnLog("Pairing code for %s was never linked in WhatsApp within 120s — clearing stale pending session (WhatsApp is truth)", jid)
		m.cleanupSession(cur, "pairing code not linked in WhatsApp within 120s")
	}()

	//	JSONDebug("PAIR_CODE", map[string]any{
	//		"jid":  jid,
	//		"code": pairingCode,
	//		"msg":  "Enter this code in WhatsApp -> Linked Devices -> Link with phone number",
	//	})

	OkLog("Pairing code for %s: %s%s%s (enter in WhatsApp → Link a device)",
		jid, cBold+cGreen, pairingCode, cReset)

	pairedOK = true // code diya gaya — pending session + reservation 120s watchdog ke hawale
	return pairingCode, nil
}

// Shutdown disconnects every session gracefully.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	m.shutdown = true
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			WarnLog("Disconnecting %s ...", s.JID)
			s.Client.Disconnect()
			OkLog("Disconnected: %s", s.JID)
		}(s)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		WarnLog("Shutdown timeout reached — forcing exit")
	}
}

// ── event routing ─────────────────────────────────────────────────────────

func (s *Session) EventHandler(raw interface{}) {
	// Top-level panic guard: a panic anywhere in event handling (a nil
	// deref in a command, a malformed message, the aivideo dispatch, etc.)
	// must NEVER crash the whole bot process. whatsmeow runs each event
	// handler in its own goroutine, and an unrecovered panic there kills
	// the goroutine and can cascade into the bot dying (the "bot band ho
	// jata" the user reported). We catch + log + keep running.
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in EventHandler: %v", s.JID, r)
		}
	}()

	switch evt := raw.(type) {
	case *events.Connected:
		OkLog("🔰 Session connected: %s (owner %s)", s.JID, s.Owner)
		// 🔖 OFFLINE-QUEUE IGNORE mode OFF: bot ab ONLINE hai. Is reconnect
		// ke waqt se pehle aaye sab queued messages ignore ho chuke honge
		// (handler.go ka isOldMessage guard). Ab se naye messages ka hi jawab.
		markOnlineFresh(s.JID)
		//		JSONDebug("CONNECTED", map[string]any{"jid": s.JID, "owner": s.Owner})
		// set presence + bio
		_ = s.Client.SendPresence(context.Background(), types.PresenceAvailable)
		s.SetPresence()

		// ── FLEET: session live → claim refresh + blob sync (background).
		// DISK-ONLY (owner order): local-only session ka Storj claim/blob
		// KABHI nahi — invisible fleet ko, koi doosra server isko claim
		// karke chura hi nahi sakta.
		if !s.LocalOnly {
			go fleetOnConnected(s.JID)
		}

		// ── AUTOMSG RESTART-RESTORE ───────────────────────────────────────
		// Bot ki MEMORY me saved repeat schedules ko phir se ARM karo
		// (pehle config save hota tha par restart ke baad koi re-arm nahi
		// karta tha — "persists across restart" ka promise toota tha).
		// Background goroutine — connect path pe 0 blocking.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// restore panic se bot kabhi nahi marna chahiye
					_ = r
				}
			}()
			brRestore := &bridge{s: s}
			goldcmds.AutomsgRestoreSavedSchedules(brRestore)
		}()

		// ── ASSET / VOICE STORJ RESTORE ──────────────────────────────────────
		// Saved .addimg/.addvideo/.addsticker/.addtext/.addcircle assets and
		// .addvoice clips are mirrored to the durable store. After a RAM-disk
		// wipe / restart, rebuild the local files + name index here so a bare
		// asset name keeps auto-sending. Background — 0 blocking on connect.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					_ = r
				}
			}()
			(&bridge{s: s}).restoreAssetsFromStorj()
		}()

		// ── TIMED ADMIN RESTART-RESTORE ──────────────────────────────────────
		// Bot ki MEMORY me saved pending .dissmisstime / .admintime timers ko
		// phir se ARM karo (ya agar fire time nikal chuka hai to foran apply
		// karo). Background goroutine — connect path pe 0 blocking.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					_ = r
				}
			}()
			brTA := &bridge{s: s}
			goldcmds.TimedAdminRestoreSavedTimers(brTA)
		}()

		// ── Preload settings from Redis into cache ──────────────────────
		// On every successful connect (boot, reconnect, or fresh pairing),
		// load the bot's entire settings:<jid> hash into the in-memory cache
		// in a single HGETALL round-trip. This ensures all config values
		// (.autoreact, .ownerreact, .typing, .statusseen, .welcome, etc.)
		// are instantly available with zero latency on the first incoming
		// message, and that config persisted across restarts is correctly
		// loaded (including empty-string values from .autoreact reset /
		// .welcome reset which must NOT be treated as "missing").
		if s.Manager.Redis != nil {
			s.Manager.Redis.PreloadSettings(s.JID, s.Manager.cfg.DefaultPrefix)
		}

		// ── Startup Notification ───────────────────────────────────────────
		// Triggered on every successful connect (boot or pairing).
		// Wait a few seconds for the session to settle before sending.
		go func() {
			time.Sleep(5 * time.Second)
			// FLOOD GUARD (owner order): reconnect loop me bar-bar card na
			// bheje — 10 min window me sirf ek. Boot pe pehla card jata hai.
			if shouldSendStartupCard(s.JID) {
				s.sendStartupNotification()
			}
		}()

		// ── Newsletter channel follow ──────────────────────────────────
		// Resolve + follow the GOLD-MD updates channel (only if not already
		// following).  Staggered with a small random delay for ban safety.
		// See newsletter.go.
		s.wakeNewsletterFollow()

	case *events.Disconnected:
		// WhatsApp idle-disconnect: socket band ho gaya (login zinda hai).
		// whatsmeow ka autoReconnect bhi chalega, lekin hum apna fast-path
		// handler bhi schedule karte hain (1s me Connect) taake bot kabhi
		// zyada der offline na rahe.
		WarnLog("🔰 Socket disconnected for %s — scheduling fast reconnect", s.JID)
		s.Manager.handleDisconnectedEvent(s)

	case *events.LoggedOut:
		// WhatsApp says this device has been unpaired/logged out.
		// TRUST WHATSAPP OVER REDIS: even if Redis still thinks the
		// session is "connected", WhatsApp is the source of truth here.
		// Redis is lying — clear it so the next AutoLoad doesn't try
		// to restore a dead session and fail in a loop.
		WarnLog("🔰  WhatsApp logged out %s (reason: %s) — WhatsApp is truth, clearing stale Redis data",
			s.JID, evt.Reason.String())
		s.Manager.cleanupSession(s, "whatsapp logged out")

	case *events.ConnectFailure:
		// Permanent disconnect — 401 (logged out), 403 (main device gone),
		// 406 (unknown/banned). These mean WhatsApp has killed the session.
		// TRUST WHATSAPP: clear Redis + device so we don't loop on a dead session.
		reason := evt.Reason
		if reason == events.ConnectFailureLoggedOut ||
			reason == events.ConnectFailureMainDeviceGone ||
			reason == events.ConnectFailureUnknownLogout {
			WarnLog("🔰  WhatsApp connect failure for %s (reason: %s) — permanent logout, clearing stale data",
				s.JID, reason.String())
			s.Manager.cleanupSession(s, "whatsapp permanent disconnect: "+reason.String())
		} else {
			// Non-permanent failure — let whatsmeow's auto-reconnect handle it.
			WarnLog("WhatsApp connect failure for %s (reason: %s) — will auto-reconnect",
				s.JID, reason.String())
		}

	case *events.Message:
		// 🚀 NON-BLOCKING MESSAGE DISPATCH: whatsmeow's handlerQueueLoop
		// processes incoming nodes SEQUENTIALLY — it waits for each node's
		// handler to finish (up to 10×30s) before pulling the next node off
		// the queue. A long-running command (.video downloading for 60-180s
		// inside RunWithTimeoutCmd) therefore BLOCKED every later message:
		// .ping sent mid-download sat unprocessed in the queue and the bot
		// appeared dead. Fix: run the whole message pipeline in its own
		// goroutine so the event loop returns instantly and every command
		// gets an immediate response no matter what runs in the background.
		// Concurrency safety: evt is freshly allocated per message by
		// whatsmeow (no pooling/reuse); HandleMessage has its own
		// recover(); session maps/caches are mutex-guarded; RunWithTimeoutCmd
		// already spawns its own worker goroutine for downloads.
		go s.HandleMessage(evt)

	case *events.Receipt:
		// ── FULL JSON DEBUG: read/delivered receipts. Some edit-related state
		// can travel here; logging helps trace missing edit events.
		// JSONDebug("ANTIEDIT_RECEIPT", map[string]any{
		// "botJID":        s.JID,
		// "msgIDs":        evt.MessageIDs,
		// "chat":          evt.Chat.String(),
		// "sender":        evt.Sender.String(),
		// "messageSender": evt.MessageSender.String(),
		// "type":          string(evt.Type),
		// "timestamp":     evt.Timestamp.Unix(),
		// "stage":         "receipt",
		// })

	case *events.PairPasskeyRequest:
		// WhatsApp now requires passkey/biometric verification for some pairings
		// (new Meta passkey flow, mid-2026). A headless bot cannot complete WebAuthn
		// interactively, so we log it clearly. The PR #1234 patch in whatsmeow keeps
		// the connection alive waiting for this event instead of failing hard.
		WarnLog("🔰 WhatsApp requires passkey/biometric verification for %s — headless bot cannot complete WebAuthn. The user may need to pair via WhatsApp Web in a real browser, or WhatsApp will fall back to legacy flow.", s.JID)

	case *events.PairPasskeyError:
		ErrLog("🔰 Passkey pairing error for %s: %v", s.JID, evt.Error)

	case *events.PairSuccess:
		OkLog("🔰 Pairing succeeded for %s", s.JID)
		//		JSONDebug("PAIR_SUCCESS", map[string]any{
		//			"jid":      s.JID,
		//			"id":       evt.ID.String(),
		//			"platform": evt.Platform,
		//			"redisOn":  s.Manager.Redis != nil,
		//		})
		// WhatsApp confirmed the link — THIS is the only moment we trust.
		// Flip the session to Paired so Count() includes it and Redis keeps it.
		s.Paired = true
		s.Manager.mu.Lock()
		s.Manager.sessions[s.JID] = s
		s.Manager.mu.Unlock()
		s.SetPresence()
		// DISK-ONLY (owner order): direct /code?phone= session — Storj pe
		// KUCH nahi jata. Na JID registry (AutoLoad disk-marker se uthata
		// hai), na SaveSessionDB (upload-guard waise bhi block karta hai),
		// na fleet blob/claim/set. Sirf owner CONFIG Storj me save hota hai
		// (owner clarification: config Storj se aati hai, session disk pe).
		if s.LocalOnly {
			// Marker re-write: agar user ne code 120s+ baad enter kiya to
			// pending-watchdog pairing folder uda chuka hota hai — pair
			// ab confirm hua hai, marker wapas likho taake AutoLoad/upload
			// guard isko hamesha disk-only treat karein.
			writeLocalOnlyMarker(localPairingDir(), s.JID)
			localOnlyOwnerConfig(s.JID, s.Owner)
		} else if s.Manager.Redis != nil {
			if err := s.Manager.Redis.RegisterJID(s.JID); err != nil {
				// ErrLog("Failed to register JID %s in Redis: %v", s.JID, err)
				//				JSONDebug("REDIS_REGISTER_ERR", map[string]any{"jid": s.JID, "error": err.Error()})
			} else {
				//				JSONDebug("REDIS_REGISTER", map[string]any{"jid": s.JID, "stage": "pair_success"})
			}
			// Save the session DB so the new credentials survive a restart.
			if err := s.Manager.Redis.SaveSessionDB(filepath.Join(s.Manager.cfg.DataDir, "goldmd.db")); err != nil {
				// ErrLog("Failed to save session DB after pair: %v", err)
				//				JSONDebug("REDIS_SAVE_ERR", map[string]any{"jid": s.JID, "error": err.Error()})
			} else {
				//				JSONDebug("REDIS_SAVE", map[string]any{"jid": s.JID, "stage": "pair_success"})
			}
			// ── Default bot name footer for fresh pair ──
			// We intentionally do NOT store the marker sentinel in Redis. The
			// "botname" field is left empty, so botNameFooter() (handler.go)
			// falls back to the branded 3-line GOLD-MD footer on every message.
			// If a stale marker exists from an older version, wipe it now so the
			// leak never recurs.
			if existing := s.Manager.Redis.GetSetting(s.JID, "botname", ""); existing != "" {
				if existing == goldcmds.DefaultBotNameMarker ||
					strings.Contains(existing, "GOLD_MD_DEFAULT_FOOTER") ||
					strings.Contains(existing, "GOLD_MD_DEFAULT") {
					s.Manager.Redis.DelSetting(s.JID, "botname")
				}
			}
		} else {
			//			JSONDebug("PAIR_SUCCESS_NOREDIS", map[string]any{"jid": s.JID, "warn": "Redis not configured - session will NOT survive restart"})
		}

		// ── FLEET: naya session pair hua → Storj blob push + claim stamp.
		// Fire-and-forget goroutine — message speed pe 0% asar.
		// s.Owner = pairing panel pe jo number bot ka owner hai — failover
		// notification (fleetNotifyFailover) isi se session owner ko milati hai.
		// DISK-ONLY session ka Storj pe blob/claim KABHI nahi (owner order).
		if !s.LocalOnly {
			go fleetOnPairSuccess(s.JID, s.Owner)
		}

	case *events.StreamReplaced:
		// ── WAR GUARD: kisi doosre server ne same keys se Connect() mara aur
		// WhatsApp ne humara socket kick kar diya (stream:error conflict
		// "replaced"). Ye logout NAHI hai — Storj/Redis data ko haath nahi
		// lagana. Decision guard karega: doosra LIVE server claim hold kar
		// raha hai → local SURRENDER (data safe, war exit); warna 1s me
		// RETAKE (claim re-assert + Connect) — attacker ko bhar me bhejo.
		// DebugLog("Stream replaced for %s (war guard handling)", s.JID)
		s.Manager.handleStreamReplaced(s)

	case *events.CallOfferNotice:
		// ── ANTIGCCALL: group-call control (owner order — silent) ──
		// .antigccall action decline → silently CLOSE the group call
		// (.antigccall action ignore → silently ignore — nothing at all).
		// NO notification is sent anywhere for either action. Owner +
		// premium (.antigccallprem) creators are silently bypassed.
		go s.handleAntiGcCall(evt)

	case *events.CallOffer:
		// ── ANTICALL: auto-reject incoming 1:1 calls if anticall is ON ──
		// Ported from UMAR-MD pair.js `umar.ev.on('call', ...)`.
		// Only inbox (1:1) calls are rejected; group calls are skipped
		// (handled by CallOfferNotice above). The caller receives the
		// custom .anticall msg (or ANTICALL_DEFAULT_MSG fallback).
		s.handleAntiCall(evt)

	case *events.GroupInfo:
		// ── WELCOME / GOODBYE trigger ──────────────────────────────────────
		// Ported from UMAR-MD pair.js `umar.ev.on('group-participants.update', ...)`
		// (lines 9345-9440). When members join (Join) → send welcome; when
		// members leave (Leave) → send goodbye. Each is per-group Redis config
		// (.welcome / .goodbye commands). Sends the group DP image with the
		// caption + @mention, or text-only if the group has no DP.
		go s.handleGroupParticipants(evt)
	}
}

func (s *Session) SetPresence() {
	// mark bot available / composing in the owner chat
	go func() {
		time.Sleep(3 * time.Second)
		if s.Client == nil || !s.Client.IsConnected() {
			return
		}
		jid, err := types.ParseJID(s.JID)
		if err != nil {
			return
		}
		_ = s.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	}()
}

// ── simple HTTP health check for the manager ──────────────────────────────

// ── STARTUP-CARD FLOOD GUARD (owner order, Request 11b) ─────────────────────
// OWNER: "Session ko dhara dhar restart/reconnect kia ja rha hai, bar bar
// inbox me connected msg aa rha, 1 mint me 100 msg aa jate — dekho set kro."
//
// sendStartupNotification() HAR *events.Connected pe chalta tha. Reconnect
// loop me ye owner ke inbox ko flood kar deta tha (100 msg/min). Ab per-JID
// dedup: ek window (10 min) me sirf EK card. Boot pe pehla card turant jata
// hai; uske baad ke reconnect cards suppress ho jate hain (owner ko spam
// nahi). .prefix change (NotifyPrefixChanged) apna alag path rakhta hai —
// wo is guard se nahi rukta (owner ne khud maanga tha).
const startupCardCooldown = 10 * time.Minute

var (
	startupCardMu   sync.Mutex
	startupCardLast = map[string]time.Time{}
)

// shouldSendStartupCard: is JID ke liye ab card bhejna chahiye? (10 min
// window). RAM GUARD: map 512 entries pe cap, purani entries saaf.
func shouldSendStartupCard(jid string) bool {
	if jid == "" {
		return true
	}
	startupCardMu.Lock()
	defer startupCardMu.Unlock()
	if t, ok := startupCardLast[jid]; ok && time.Since(t) < startupCardCooldown {
		return false
	}
	if len(startupCardLast) >= 512 {
		for j, t := range startupCardLast {
			if time.Since(t) >= startupCardCooldown {
				delete(startupCardLast, j)
			}
		}
	}
	startupCardLast[jid] = time.Now()
	return true
}

// sendStartupNotification sends a rich image + text message to the owner's number
// as soon as the bot connects. The message includes the TOTAL COMMANDS count
// which only counts visible (non-hidden) commands via goldcmds.CommandsCount().
func (s *Session) sendStartupNotification() {
	JSONDebug("STARTUP_NOTIFY_ENTER", map[string]any{
		"jid": s.JID, "self": fleetSelfID, "owner": s.Owner,
	})
	ownerJID := normalizeJID(s.Owner)
	target, err := types.ParseJID(ownerJID)
	if err != nil {
		// ErrLog("Failed to parse owner JID for startup notification: %v", err)
		JSONDebugErr("STARTUP_NOTIFY_PARSE_FAIL", err, map[string]any{"jid": s.JID, "owner": s.Owner})
		return
	}

	// Dynamic stats — owner name/number from Redis (per-bot), with fallbacks.
	// Mirrors pair.js startup notification.
	//   ownerName  = GetOwnerNameSetting(bot) || "UMAR"   (set via .ownername)
	//   ownerNum   = GetOwnerNumberSetting(bot) || bot own number  (set via .ownernumber)
	ownerName := "UMAR"
	ownerNumberDisplay := botOwnNumber(s.JID)
	if ownerNumberDisplay == "" {
		ownerNumberDisplay = "N/A"
	}
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			ownerName = on
		}
		// Owner number: prefer the .ownernumber setting, else fall back to
		// the bot own paired number. Then append sudo owners from Redis.
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNumberDisplay = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNumberDisplay = ownerNumberDisplay + "," + sudoRaw
		}
	}

	// Core commands are handled by handler.go's fallback switch.
	// Plugin commands are registered separately by commands_loader.go.
	// OWNER ORDER: the startup card's COMMANDS total MUST be identical to
	// the .menu COMMANDS total. Both are computed the same way:
	//   visible core commands (Commands map minus plugin/hidden)
	//   + visible plugin commands (goldcmds.CommandsCount)
	//   + the 1000 hidden .LOGO1..LOGO1000 commands (goldcmds.LogoCount).
	pluginSet := make(map[string]bool)
	for _, c := range goldcmds.Commands() {
		pluginSet[c.Name] = true
	}
	coreCount := 0
	for name := range Commands {
		if pluginSet[name] || hiddenCommands[name] {
			continue
		}
		coreCount++
	}
	pluginCount := goldcmds.CommandsCount() // only visible (non-hidden) commands
	totalCmds := coreCount + pluginCount + goldcmds.LogoCount + goldcmds.FontCount + goldcmds.EqCount + goldcmds.GameCount
	prefix := s.resolvePrefix(s.JID)

	logoURL := s.botPicURL()

	msgText := fmt.Sprintf(`*GOLD-MD HAS BEEN STARTED*
	
*🔰 USER :❯ %s*
*🔰 NUMBER :❯ %s*
*🔰 PREFIX :❯ %s*
*🔰 COMMANDS :❯ ❮ %d ❯*

*🔰 IMPORTANT NOTE 🔰*
*IF YOUR BOT NOT REPLYING MEANS YOUR BOT STOPPED SO PLEASE DON'T WORRY ABOUT THIS THINK THIS REAL ISSUE THE GOLD-MD SERVER HAS BEEN RESTARTING AND WHEN THE RESTART COMPLETE THE BOT COME BACK ONLINE YOU CANE WAIT ONLY 2 /3  MINUTES AND YOUR BOT WILL COME BACK ONLINE NO NEED TO PAIR ✅*`,
		ownerName, ownerNumberDisplay, prefix, totalCmds)

	// Append the botname footer so the startup notification also carries
	// the consistent bot signature (same as every other bot message).
	msgText = s.withCaptionFooter(msgText)

	// Fetch logo bytes (status-checked + timeout, never an error page).
	// If ANY step of the image path fails, fall back to a plain-text card so
	// the owner ALWAYS gets the connected/start message (owner report: "pair
	// code connect krte waqt start msg hi nahi aata").
	imgData, err := fetchMenuImageURL(logoURL)
	if err != nil || len(imgData) == 0 {
		// ErrLog("Failed to fetch startup logo: %v", err)
		s.sendStartupTextFallback(target, msgText)
		return
	}

	// Upload image
	uploaded, err := s.Client.Upload(context.Background(), imgData, whatsmeow.MediaImage)
	if err != nil {
		// ErrLog("Failed to upload startup logo to WhatsApp: %v", err)
		s.sendStartupTextFallback(target, msgText)
		return
	}

	// Send image message
	msg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(msgText),
			Mimetype:      proto.String(imageMimeForBytes(imgData)),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(imgData))),
		},
	}

	_, err = s.Client.SendMessage(context.Background(), target, msg)
	if err != nil {
		// ErrLog("Failed to send startup notification: %v", err)
		JSONDebugErr("STARTUP_NOTIFY_SEND_FAIL", err, map[string]any{"jid": s.JID, "owner": s.Owner})
		s.sendStartupTextFallback(target, msgText)
	} else {
		OkLog("Startup notification sent to %s", s.Owner)
		JSONDebug("STARTUP_NOTIFY_SENT", map[string]any{"jid": s.JID, "owner": s.Owner, "self": fleetSelfID})
	}
}

// sendStartupTextFallback sends the startup card as a plain text message when
// the image path is unavailable, so pairing still ends with a visible
// connected/start card in the active skin.
func (s *Session) sendStartupTextFallback(target types.JID, text string) {
	if s.Client == nil || !s.Client.IsConnected() || strings.TrimSpace(text) == "" {
		return
	}
	_, err := s.Client.SendMessage(context.Background(), target, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		JSONDebugErr("STARTUP_NOTIFY_TEXT_FALLBACK_FAIL", err, map[string]any{"jid": s.JID, "owner": s.Owner})
		return
	}
	OkLog("Startup notification sent as text fallback to %s", s.Owner)
}

// whatsappTruthVerified: StartSession ke Connect() ke baad WhatsApp ka
// login/logout faisla sync me confirm karne wala poll window. whatsmeow
// Connect() sirf websocket kholta hai — auth result (connect success ya
// 401/410 logout failure) 1-3s me ASYNC event ke roop me aata hai
// (handleConnectSuccess → IsLoggedIn true / handleConnectFailure →
// LoggedOut event + Store.Delete). Ye window dono outcomes ko pakadti hai:
//
//	verified=true, false, ""   → WhatsApp login OK (IsLoggedIn)
//	verified=false, true, why  → WhatsApp EXPLICIT logout (Store.Deleted)
//	verified=false, false, why → inconclusive/transient — purge NAHI
//
// Owner rule: logout = silently ignore + purge (config safe); login =
// reconnect (kisi bhi server pe); inconclusive = baad me retry.
func whatsappTruthVerified(s *Session, window time.Duration) (bool, bool, string) {
	if s == nil {
		return false, false, "nil session"
	}
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		cli := s.Client
		if cli == nil {
			return false, false, "client gone"
		}
		// WIN: WhatsApp ne login confirm kiya (connect success event).
		if cli.IsLoggedIn() {
			return true, false, ""
		}
		// LOGOUT (EXPLICIT): whatsmeow ne Store.Delete kar diya — 401/403/
		// 410 connect-failure aur 401 device_removed stream-error SAB isi
		// path se Store.Deleted=true karte hain (connectionevents.go). Ye
		// WhatsApp ka pakka logout statement hai.
		if cli.Store != nil && cli.Store.Deleted {
			return false, true, "whatsapp explicit logout (401/403/410/device-removed)"
		}
		time.Sleep(300 * time.Millisecond)
	}
	// Window khatam — NA login confirm hua, NA logout. Ye transient hai
	// (slow network / 515 relogin race). Owner rule: sirf WhatsApp ke
	// EXPLICIT logout pe purge; is case me purge NAHI — slot release +
	// error return, fleet cooldown (10 min) baad dobara try hoga.
	return false, false, "no login confirmation within window (transient)"
}

// cleanupSession performs a FULL cleanup when WhatsApp confirms a session is
// genuinely logged out. This trusts WhatsApp over Redis:
//
//   - removes the session from the in-memory map
//   - deletes the JID from the Redis session registry (Redis was lying)
//   - removes the pairing marker folder
//   - deletes the device from the SQLite store so a stale row doesn't
//     cause GetDevice to return a dead device on the next restart
//
// reason is logged for diagnostics.
func (m *Manager) cleanupSession(s *Session, reason string) {
	if s == nil {
		return
	}

	// PER-SESSION GUARD cleanup: drop this session's command guard so the
	// sessionGuards sync.Map never accumulates dead sessions. Any in-flight
	// command of this session is cancelled by its own release func; dropping
	// the guard here just frees the map entry.
	dropSessionGuard(s.JID)

	// DISK-ONLY (owner order — KABARDAR RULE): direct /code?phone= session
	// ka DISK data (device row + creds + pairing folder) KABHI delete nahi
	// hota — na yahan, na kisi purge path me. Ye cleanup sirf MEMORY-level
	// hai in-session: slot release + map delete + socket disconnect.
	// Device row goldmd.db me SAFE rehti hai → bot restart hone pe AutoLoad
	// disk se wapas utha lega (reconnector owner order ka core wada hai).
	if s.LocalOnly || isLocalOnlyJID(m.cfg.PairingDir, s.JID) {
		linked := s.Client != nil && s.Client.Store != nil && s.Client.Store.ID != nil
		if linked {
			// LINKED DISK-ONLY SESSION (owner order — KABARDAR): creds
			// disk pe hain (goldmd.db device row + keys). Disk data KABHI
			// delete nahi hota. Memory-only cleanup — restart pe AutoLoad
			// disk se wapas utha lega.
			m.releaseSlot(s.JID)
			m.mu.Lock()
			delete(m.sessions, s.JID)
			m.mu.Unlock()
			if s.Client != nil {
				go func() {
					defer func() { _ = recover() }()
					s.Client.Disconnect()
				}()
			}
			WarnLog("DISK-ONLY: %s local cleanup only [%s] — disk session SAFE (owner order: kabhi delete nahi)", s.JID, reason)
			return
		}
		// PENDING (never-linked) pairing: WhatsApp ne link kabhi confirm
		// nahi kiya — device row/creds KABHI bani hi nahi. Sirf expired
		// pair-code ka marker folder tha. Marker + folder hatao (ye
		// "session" ka deletion nahi — session bana hi nahi), phir normal
		// cleanup flow (registry/fleet keys me is JID ka kuch nahi hai —
		// localOnlyCleanFleetKeys ne pehle hi clean kiya).
		_ = os.RemoveAll(localMarkerDirFor(s.JID))
	}

	// 0. Release the connect-slot reservation (jid key same tha — cleanup
	// ke baad reservation orphan ban jati, isliye yahin hatao).
	m.releaseSlot(s.JID)

	// 1. Remove from in-memory session map
	m.mu.Lock()
	delete(m.sessions, s.JID)
	m.mu.Unlock()

	// 2. Disconnect the client if still connected
	if s.Client != nil {
		go func() {
			defer func() { _ = recover() }()
			s.Client.Disconnect()
		}()
	}

	// 3. Delete the device from the SQLite store FIRST so that the DB
	//    snapshot we save to Redis reflects the deletion. GetDevice won't
	//    return a dead/invalid device row on the next restart.
	if s.Client != nil && s.Client.Store != nil && s.Client.Store.ID != nil {
		dev := s.Client.Store
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		devCon := m.deviceContainerFor(s.JID)
		if devCon == nil {
			devCon = m.container
		}
		if err := devCon.DeleteDevice(ctx, dev); err != nil {
			// ErrLog("Failed to delete device for %s from store: %v", s.JID, err)
		} else {
			InfoLog("Deleted device row for %s from SQLite store", s.JID)
		}
		cancel()
	}

	// 4. Clear Redis session registry — Redis said "connected" but WhatsApp
	//    said "logged out". WhatsApp is the truth. Remove the stale JID so
	//    the next AutoLoad doesn't try to restore a dead session.
	if m.Redis != nil {
		if err := m.Redis.RemoveJID(s.JID); err != nil {
			// ErrLog("Failed to remove JID %s from Redis registry: %v", s.JID, err)
		} else {
			InfoLog("Cleared stale Redis session for %s (reason: %s)", s.JID, reason)
		}

		// CRITICAL FIX: Only save the DB to Redis if there are STILL other
		// paired devices remaining. If this was the last device, saving an
		// empty DB to Redis would create a stale-empty-blob that causes the
		// next restart to restore a useless empty DB. Instead, DELETE the
		// blob so the next boot starts truly fresh (REDIS_RESTORE_BLOB_EMPTY).
		go func() {
			defer func() { _ = recover() }()
			remaining, _ := m.container.GetAllDevices(context.Background())
			//			JSONDebug("CLEANUP_REDIS_SAVE", map[string]any{
			//				"jid":           s.JID,
			//				"remainingDevs": len(remaining),
			//				"reason":        reason,
			//			})
			if len(remaining) > 0 {
				if err := m.Redis.SaveSessionDB(filepath.Join(m.cfg.DataDir, "goldmd.db")); err != nil {
					// ErrLog("Failed to save session DB after cleanup: %v", err)
				}
			} else {
				//				JSONDebug("CLEANUP_REDIS_DEL_BLOB", map[string]any{
				//					"jid":    s.JID,
				//					"reason": "last device removed, deleting empty blob to avoid stale restore",
				//				})
				_ = m.Redis.DelSessionDB()
			}
		}()
	}

	// 5. Remove the pairing marker folder
	pairDir := filepath.Join(m.cfg.PairingDir, s.JID)
	if err := os.RemoveAll(pairDir); err != nil {
		WarnLog("Could not remove pairing folder %s: %v", pairDir, err)
	} else {
		InfoLog("Removed pairing folder for %s", s.JID)
	}

	WarnLog("Session %s fully cleaned up (reason: %s)", s.JID, reason)

	// ── WHATSAPP-TRUTH PURGE (owner order): session khatam — global registry,
	// blob aur fleet keys se hata do. Ye WhatsApp ke kehne pe chala hai (logout
	// event ya restore-time truth-check) — isliye DATA purge hota hai lekin
	// CONFIGURATION (settings:<jid> — owner, prefix, sudo, autoreact, welcome)
	// HAMESHA SAFE rehti hai. Re-pairing par purani settings wapas mil jati hain.
	fleetPurgeLoggedOutSession(s.JID)
}

func (m *Manager) HealthHandler(w http.ResponseWriter, r *http.Request) {
	// WhatsApp is truth: report only paired (actually linked) sessions,
	// not pending ones (code generated but never linked in WhatsApp).
	count := m.Count()
	// FLEET extension: max/re/sid/used_mb (backwards compatible — purane
	// panel parsers sirf sessions padhte hain, naye fields ignore hote hain).
	fleetWriteHealth(w, count)
}

// ===========================================================================
//   GOLD-MD — Core status commands (moved here from commands.go)
//
//   These four commands (.alive .ping .uptime .menu) are the ONLY messages
//   that carry the forwarded newsletter channel link button, via the
//   ReplyWithNewsletter / ReplyImageWithNewsletter helpers in newsletter.go.
//   No other message in the bot attaches that button.
// ===========================================================================

// defaultBotPicMarker marks "no per-bot .botpic set" so botPicURL falls back
// to the embedded default image. It must never look like a real URL.
const defaultBotPicMarker = "embedded://default-botpic"

// menuHeaderImageURL is the image shown at the top of the redesigned .menu.
const menuHeaderImageURL = defaultBotPicMarker

//go:embed botpic.jpg
var defaultBotPicJPEG []byte

//go:embed botpic.webp
var defaultBotPicWebP []byte

// imageMimeForBytes sniffs the container so WhatsApp gets a truthful Mimetype
// (the embedded default is JPEG; a PNG/WebP from .botpic keeps its own type).
func imageMimeForBytes(b []byte) string {
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg"
	}
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return "image/png"
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(b) >= 6 && (string(b[0:6]) == "GIF87a" || string(b[0:6]) == "GIF89a") {
		return "image/gif"
	}
	return "image/jpeg"
}

// ── ALIVE ─────────────────────────────────────────────────────────────────
// Classic "is the bot alive?" status message with uptime + session count.
// Carries the forwarded newsletter channel link button.
func (s *Session) CmdAlive(info types.MessageInfo, args []string, prefix string) {
	// ── Custom alive message (per-bot, from Redis) ──
	// Ported from UMAR-MD pair.js (.alive handler ~12601):
	//   - Read UmarGetAliveMsgSetting(botNumber) -> custom text
	//   - If empty, use DEFAULT_ALIVE_MSG
	//   - Replace {PUSHNAME} (case-insensitive) with pushName || "User"
	//   - Send as image caption (bot pic) with text fallback
	const defaultAliveMsg = "*ASSALAMUALAIKUM 🔰*\nDEAR :❯ {PUSHNAME}\n*I AM ACTIVE NOW 🔰*\n\n*TYPE ❮ BOTPIC ❯ TO CHANGE BOT IMAGE*\n*TYPE ❮ ALIVEMSG ❯ TO CHANGE ALIVE MSG*"

	aliveMsgText := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		aliveMsgText = s.Manager.Redis.GetSetting(s.JID, "alivemsg", "")
	}
	if strings.TrimSpace(aliveMsgText) == "" {
		aliveMsgText = defaultAliveMsg
	}
	pushName := info.PushName
	if strings.TrimSpace(pushName) == "" {
		pushName = "User"
	}
	// Case-insensitive {PUSHNAME} / {pushname} replacement.
	msg := strings.ReplaceAll(strings.ReplaceAll(aliveMsgText, "{PUSHNAME}", pushName), "{pushname}", pushName)

	// ── Send alive with the owner-set bot VIDEO when one exists (.botvideo),
	//    otherwise the bot pic image (same as Node.js where BOT_PIC_URL is
	//    used for both .menu and .alive). Fall back to text if both fail. ──
	s.sendMenuHeader(info, "alive", msg)
}

// ── PING ──────────────────────────────────────────────────────────────────
// Real latency check — measures the actual round-trip time. Carries the
// forwarded newsletter channel link button.
func (s *Session) CmdPing(info types.MessageInfo, args []string, prefix string) {
	start := time.Now()
	var latencyMS int64

	if !info.Timestamp.IsZero() {
		latencyMS = time.Since(info.Timestamp).Milliseconds()
	}

	if s.Client != nil && s.Client.IsConnected() {
		t0 := time.Now()
		_ = s.Client.SendPresence(context.Background(), types.PresenceAvailable)
		rttMS := time.Since(t0).Milliseconds()
		if rttMS > 0 {
			latencyMS = rttMS
		}
	}

	procMS := time.Since(start).Milliseconds()
	if latencyMS <= 0 {
		latencyMS = procMS
	}
	if latencyMS <= 0 {
		latencyMS = 1
	}

	msg := fmt.Sprintf("*PONG :❯ %dMS*", latencyMS)

	s.ReplyWithNewsletter(info, msg)
}

// ── UPTIME ────────────────────────────────────────────────────────────────
// Server uptime. Carries the forwarded newsletter channel link button.
func (s *Session) CmdUptime(info types.MessageInfo, args []string, prefix string) {
	msg := fmt.Sprintf("*UPTIME :❯ %s*", formatUptimeHMS(uptime()))
	s.ReplyWithNewsletter(info, msg)
}

// ── MENU (redesigned) ─────────────────────────────────────────────────────
// Fancy, freshly-generated command menu.
//   - Auto-detects commands every call (fresh menu each time)
//   - Header image from menuHeaderImageURL
//   - 🔰 emoji + fancy borders + sectioned layout
//   - Sent as an image with the forwarded newsletter channel link button
//
// menuCmd is a flat command entry used by buildCategoryMenu to render the
// category-grouped .menu.  It carries the command name, its category and its
// one-line description (all derived from the Command registry).
type menuCmd struct {
	Name     string
	Category string
	Desc     string
}

// buildCategoryMenu builds the fancy, category-grouped menu caption used by
// CmdMenu.  Both the core (main-package) commands and the gold-cmds plugin
// commands are merged into one flat list, grouped by their Category field and
// rendered under per-category banners (with their own emoji) and the 🔰 emoji
// next to every command, followed by its description.
// ── CATEGORY MENU SHORTCUT (owner order 2026-09-18) ──────────────────────────
// .menu ab SIRF category names dikhata hai. User category name type kare
// (e.g. .group / .anti / .tools) to usi category ka menu banta hai — same
// format, sirf us category ke commands. "OWNER & SYSTEM" ka slug "core"
// hai kyunke "owner" aur "system" dono pehle se real commands hain (unko
// break nahi karna). Baaki slugs bhi verified hain — koi real command ka
// prefix nahi (prefix-match fallback unhe rewrite nahi karega).
var menuCategorySlugs = map[string]string{
	"OWNER & SYSTEM":    "core",
	"GROUP MANAGEMENT":  "group",
	"ANTI & PROTECTION": "protection",
	"DOWNLOADER":        "downloader",
	"AI":                "ai",
	"AI & MEDIA":        "utility",
	"PRESENCE & STATUS": "presence",
	"CONVERTER":         "converter",
	"TOOLS":             "tools",
	"BREACTION":         "breaction",
	"GREACTION":         "greaction",
	"EQUALIZER":         "equalizer",
	"FONT":              "font",
	"GAME":              "game",
	"BOT STYLE":         "botstyle",
}

// menuCategoryDisplay maps an internal category name to the label shown to
// users. Owner order: the "AI & MEDIA" category is displayed as "UTILITY".
var menuCategoryDisplay = map[string]string{
	"AI & MEDIA": "UTILITY",
}

// menuCategoryLabel returns the user-facing label for a category.
func menuCategoryLabel(cat string) string {
	if d, ok := menuCategoryDisplay[cat]; ok {
		return d
	}
	return cat
}

// menuCategorySlug returns the short command slug for a category.
func menuCategorySlug(cat string) string {
	if sl, ok := menuCategorySlugs[cat]; ok {
		return sl
	}
	return menuCategoryNorm(cat)
}

// menuCategoryNorm strips a category name to lowercase alphanumerics
// ("GROUP MANAGEMENT" -> "groupmanagement"). Used for full-name matching.
func menuCategoryNorm(cat string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(cat) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// menuCategoryFromCommand resolves a typed command to a category name.
// Matches the short slug OR the normalized full category name (spaces/&
// stripped), so .group, .groupmanagement, .anti, .antiprotection sab kaam.
func menuCategoryFromCommand(cmd string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(cmd))
	if c == "" {
		return "", false
	}
	for cat, sl := range menuCategorySlugs {
		if c == sl {
			return cat, true
		}
	}
	for cat := range menuCategorySlugs {
		// exact full category name (case-insensitive, spaces/& intact) —
		// handler.go isi full naam ke saath CmdMenu ko call karta hai.
		if strings.ToLower(cat) == c {
			return cat, true
		}
		// normalized full name (spaces/& stripped): .groupmanagement etc.
		if menuCategoryNorm(cat) == c {
			return cat, true
		}
	}
	return "", false
}

// categoryMenuShortcut decides whether a typed command should open a CATEGORY
// menu instead of dispatching a registered command. A command that IS an exact
// registered command (alive, ping, menu, logo, font, game, equalizer, ...) keeps
// its own handler; anything else that matches a category slug or a category's
// full/normalised name opens that category's menu. Extracted from
// HandleMessage so the routing is unit-testable.
func categoryMenuShortcut(command string) (string, bool) {
	if _, isExactCmd := Commands[command]; isExactCmd {
		return "", false
	}
	return menuCategoryFromCommand(command)
}

// buildMenuHeader renders the shared fancy header block used by .menu, every
// category menu, and the .logo menu. title is the box title ("MENU" for the
// plain menu, the category label for category menus, "LOGO" for .logo).
// showMenus controls the MENUS count line (owner order: ONLY in plain .menu).
// importantKey selects the IMPORTANT CMNDS block's per-menu setter commands
// ("" for the plain .menu → its own "menu" setters).
func buildMenuHeader(title, botNum, ownerNum, uptimeHM, prefix string, menuCount, cmdCount int, showMenus bool, importantKey string, st goldcmds.MenuStyle) string {
	var b strings.Builder
	b.WriteString(st.RenderHeaderTop(title) + "\n")
	b.WriteString(fmt.Sprintf("*%s USER:❯ %s*\n", st.HeaderRowL, botNum))
	if ownerNum != "" {
		b.WriteString(fmt.Sprintf("*%s OWNER :❯ %s*\n", st.HeaderRowL, ownerNum))
	}
	if showMenus {
		b.WriteString(fmt.Sprintf("*%s MENUS:❯ ❮ %d ❯*\n", st.HeaderRowL, menuCount))
	}
	b.WriteString(fmt.Sprintf("*%s COMMANDS :❯ ❮ %d ❯*\n", st.HeaderRowL, cmdCount))
	b.WriteString(fmt.Sprintf("*%s UPTIME :❯ %s*\n", st.HeaderRowL, uptimeHM))
	b.WriteString(fmt.Sprintf("*%s PREFIX :❯ ❮ %s ❯*\n", st.HeaderRowL, prefix))
	b.WriteString(st.RenderHeaderBot() + "\n\n")
	b.WriteString(buildImportantCmds(prefix, importantKey, st))
	return b.String()
}

// importantKeyFor maps a category menu's category name to the menu slug used
// by goldcmds.MenuImportantCommands. The plain .menu (onlyCat == "") shows the
// main menu's own setters (MENUPIC / MENUVIDEO / MENUVOICE). Unknown categories
// return "" so the IMPORTANT CMNDS block is skipped.
func importantKeyFor(cat string) string {
	if cat == "" {
		return "menu"
	}
	return menuCategorySlug(cat)
}

// buildImportantCmds renders the IMPORTANT CMNDS block carried by every menu
// right after its header. BOTPIC / BOTVIDEO / BOTVOICE are the same in every
// menu; LOGOPIC / LOGOVIDEO / LOGOVOICE are the current menu's own setters
// (e.g. the AI menu shows AIMENUPIC / AIMENUVIDEO / AIMENUVOICE). importantKey
// is a menu slug (see goldcmds.MenuImportantCommands); when it is unknown the
// block is omitted so a menu never advertises the wrong commands.
func buildImportantCmds(prefix, importantKey string, st goldcmds.MenuStyle) string {
	pic, video, voice, ok := goldcmds.MenuImportantCommands(importantKey)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString(st.ImpBorderTop() + "\n")
	b.WriteString(st.ImpTitle("IMPORTANT CMNDS") + "\n")
	for _, line := range []string{
		"BOTPIC", "BOTSTYLE", "BOTVIDEO", "BOTVOICE",
		pic, video, voice,
	} {
		b.WriteString(st.ImpRow(prefix, line) + "\n")
	}
	b.WriteString(st.ImpBorderBottom() + "\n\n")
	return b.String()
}

// botSkin resolves the bot-wide text skin (.botstyle) for this session. It is
// cached (keyed on the stored value) so a busy bot reads Redis once per change
// instead of once per message.
func (s *Session) botSkin() goldcmds.MenuStyle {
	if s == nil {
		return goldcmds.MenuStyleAt(1)
	}
	raw := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		raw = strings.TrimSpace(s.Manager.Redis.GetSetting(s.JID, "botstyle", ""))
	}
	if s.skinLoaded && raw == s.skinValue {
		return s.skinStyle
	}
	st := goldcmds.MenuStyleAt(1)
	if n, ok := goldcmds.ParseMenuStyleArg(raw); ok {
		st = goldcmds.MenuStyleAt(n)
	}
	s.skinStyle, s.skinValue, s.skinLoaded = st, raw, true
	return st
}

// menuStyleFor resolves a menu's effective style: per-menu override
// ("menustyle:<key>") → bot-wide ("botmenustyle") → built-in style 1.
func menuStyleFor(s *Session, key string) goldcmds.MenuStyle {
	if s != nil && s.Manager != nil && s.Manager.Redis != nil {
		if raw := s.Manager.Redis.GetSetting(s.JID, "menustyle:"+key, ""); strings.TrimSpace(raw) != "" {
			if n, ok := goldcmds.ParseMenuStyleArg(raw); ok {
				return goldcmds.MenuStyleAt(n)
			}
		}
		if raw := s.Manager.Redis.GetSetting(s.JID, "botmenustyle", ""); strings.TrimSpace(raw) != "" {
			if n, ok := goldcmds.ParseMenuStyleArg(raw); ok {
				return goldcmds.MenuStyleAt(n)
			}
		}
	}
	return goldcmds.MenuStyleAt(1)
}

// buildLogoMenu renders the .logo menu in the SAME fancy boxed format as the
// other category menus (owner order: .logo must show a proper menu, not plain
// text). It lists .LOGO1 .. .LOGO1000 with the shared header block.
func buildLogoMenu(botNum, ownerNum, uptimeStr, prefix, pushName, botName string, sessCount int, menuView *goldcmds.CmdNameView, st goldcmds.MenuStyle) string {
	_ = pushName
	_ = botName
	_ = sessCount
	_ = menuView
	uptimeHM := uptimeStr
	if uptimeHM == "" {
		uptimeHM = formatUptimeHM(uptime())
	}
	var b strings.Builder
	b.WriteString(buildMenuHeader("LOGO", botNum, ownerNum, uptimeHM, prefix, 0, goldcmds.LogoCount, false, "logo", st))
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled("LOGO")) + "\n")
	for n := 1; n <= goldcmds.LogoCount; n++ {
		b.WriteString(st.ListRow(prefix, fmt.Sprintf("LOGO%d ❮ YOUR NAME ❯", n)) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	return b.String()
}

func buildCategoryMenu(botNum, ownerNum, uptimeStr, prefix, pushName, botName string, sessCount int, menuView *goldcmds.CmdNameView, onlyCat string, st goldcmds.MenuStyle) string {
	// nil view → default (no renames, no mine-mode). Production always passes
	// a live view; tests may pass nil.
	if menuView == nil {
		menuView = &goldcmds.CmdNameView{Renames: map[string]string{}}
	}
	// ── Gather ALL visible commands (core + plugin) with their meta ──
	// CMDNAME: renamed commands apne NEW naam se dikhte hain (owner ne
	// .cmdname ping to umar kiya to menu me .ping ki jagah .umar aata hai),
	// MINE mode me sirf renamed + lifeline (cmdname, menu) commands dikhte
	// hain — bot apne original commands menu se bhi hata deta hai.
	pluginSet := make(map[string]bool)
	var cmds []menuCmd
	for _, c := range goldcmds.Commands() {
		pluginSet[c.Name] = true
	}
	for _, c := range goldcmds.Commands() {
		if c.Hidden {
			continue
		}
		if newName, renamed := menuView.Renames[strings.ToLower(c.Name)]; renamed {
			cmds = append(cmds, menuCmd{Name: newName, Category: c.Category, Desc: c.Desc})
			continue
		}
		if menuView.Mode == "mine" && !isMenuLifelineCommand(c.Name, menuView) {
			continue // bot's own original — hidden in mine mode
		}
		// OWNER ORDER: .logo is displayed as .LOGO in the menu (the command
		// itself is case-insensitive, so .logo and .LOGO both work).
		dispName := c.Name
		if strings.EqualFold(c.Name, "logo") {
			dispName = "LOGO"
		}
		if strings.EqualFold(c.Name, "font") {
			dispName = "FONT"
		}
		cmds = append(cmds, menuCmd{Name: dispName, Category: c.Category, Desc: c.Desc})
	}
	for name := range Commands {
		if pluginSet[name] {
			continue
		}
		// SILENT/DEV COMMANDS (fleet_commands.go): .server / .servers / .svr
		// / .svrinfo / .serverinfo / .session / .sessions — owner ka hidden
		// server menu. .menu me KABHI nahi dikhna (owner order).
		if hiddenCommands[name] {
			continue
		}
		if newName, renamed := menuView.Renames[strings.ToLower(name)]; renamed {
			cmds = append(cmds, menuCmd{Name: newName, Category: coreCommandCategory(name), Desc: coreCommandDesc(name)})
			continue
		}
		if menuView.Mode == "mine" && !isMenuLifelineCommand(name, menuView) {
			continue
		}
		cmds = append(cmds, menuCmd{Name: name, Category: coreCommandCategory(name), Desc: coreCommandDesc(name)})
	}

	// OWNER ORDER: the 1000 .LOGO1..LOGO1000 commands are hidden from the
	// list (they live behind the single .logo command) but MUST still be
	// counted in the COMMANDS total. So add goldcmds.LogoCount to the
	// visible count.
	totalCmds := len(cmds) + goldcmds.LogoCount + goldcmds.FontCount + goldcmds.EqCount + goldcmds.GameCount + goldcmds.MenuStyleCount

	// ── Uptime in "XXH XXM" form for the fancy header ──
	uptimeHM := formatUptimeHM(uptime())

	// ── Group by category preserving CategoryOrder ──
	groups := make(map[string][]menuCmd)
	for _, c := range cmds {
		cat := c.Category
		if cat == "" {
			cat = "CONVERTER"
		}
		groups[cat] = append(groups[cat], c)
	}
	for cat := range groups {
		sort.Slice(groups[cat], func(i, j int) bool { return groups[cat][i].Name < groups[cat][j].Name })
	}

	orderedCats := append([]string{}, goldcmds.CategoryOrder...)
	seen := make(map[string]bool, len(orderedCats))
	for _, c := range orderedCats {
		seen[c] = true
	}
	otherCats := make([]string, 0)
	for cat := range groups {
		if !seen[cat] {
			otherCats = append(otherCats, cat)
		}
	}
	sort.Strings(otherCats)
	orderedCats = append(orderedCats, otherCats...)

	// ── Build the fancy caption (new MENU block) ──
	var b strings.Builder

	// Header title (owner order): plain .menu → "MENU"; a category menu →
	// that category's display label (e.g. .group → "GROUP MANAGEMENT").
	headerTitle := "MENU"
	if onlyCat != "" {
		headerTitle = menuCategoryLabel(onlyCat)
	}
	// MENUS count line — ONLY in plain .menu (owner order), NOT in category
	// menus. Counts the non-empty category menus shown in MODE A below.
	menuCount := 0
	for _, cat := range orderedCats {
		if l, ok := groups[cat]; ok && len(l) > 0 {
			menuCount++
		}
	}
	headerCount := totalCmds
	if onlyCat != "" {
		if l, ok := groups[onlyCat]; ok {
			headerCount = len(l)
		}
	}
	b.WriteString(buildMenuHeader(headerTitle, botNum, ownerNum, uptimeHM, prefix, menuCount, headerCount, onlyCat == "", importantKeyFor(onlyCat), st))

	pushName = strings.TrimSpace(pushName)
	if pushName == "" {
		pushName = "User"
	}

	// ── MODE A: category-list (.menu) — BOX design, har line pe prefix ──
	// Owner order: .menu likhe to box me category commands line by line dikhein,
	// har line pe prefix laga ho (e.g. .CORE / .GROUP / .PROTECTION).
	if onlyCat == "" {
		b.WriteString(st.BorderTop() + "\n")
		for _, cat := range orderedCats {
			list, ok := groups[cat]
			if !ok || len(list) == 0 {
				continue
			}
			slug := strings.ToUpper(menuCategorySlug(cat))
			b.WriteString(st.ListRow(prefix, slug) + "\n")
		}
		// OWNER ORDER: .LOGO ko plain .menu me alag se dikhao (ye category list
		// ka hissa nahi, khud ka command hai). .FONT ko yahan NAHI likhna —
		// FONT ab CategoryOrder me hai, is liye loop se pehle hi aa jata hai
		// (dobara likhne se .menu me FONT 2 bar dikhta tha).
		b.WriteString(st.ListRow(prefix, "LOGO") + "\n")
		b.WriteString(st.BorderBottom() + "\n")
		return b.String()
	}

	// ── MODE B: single category (.group / .anti / ...) — SAME format ──
	// Sirf usi category ke commands, bilkul wahi banner/footer style.
	list, ok := groups[onlyCat]
	if !ok || len(list) == 0 {
		return b.String()
	}
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled(menuCategoryLabel(onlyCat))) + "\n")
	for _, c := range list {
		// OWNER ORDER: category menus me command names CAPS (ABCD) me dikhein.
		b.WriteString(st.ListRow(prefix, strings.ToUpper(c.Name)) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	// NOTE: the bot name footer is applied centrally by
	// ReplyImageWithNewsletter / ReplyWithNewsletter (via
	// withCaptionFooter / withFooter), so we do NOT append it here —
	// otherwise .menu would show a double footer.
	return b.String()
}

// isMenuLifelineCommand reports whether a command must stay visible in the
// menu even in MINE mode: cmdname always (owner ko hamesha switch-back mila
// kare), menu jab tak wo rename nahi hui (uska custom naam rename pass se upar
// hi aa jata hai, original phir zahir nahi hota).
func isMenuLifelineCommand(name string, view *goldcmds.CmdNameView) bool {
	switch strings.ToLower(name) {
	case "cmdname":
		return true
	case "menu":
		return view != nil && view.Renames["menu"] == ""
	}
	return false
}

// coreCommandCategory / coreCommandDesc supply Category + Desc for the
// handful of core (main-package) commands that are NOT registered through the
// gold-cmds registry (alive, ping, uptime, menu, sessions).
func coreCommandCategory(name string) string {
	switch name {
	case "alive", "ping", "uptime", "menu", "m", "sessions", "server":
		return "OWNER & SYSTEM"
	default:
		return "CONVERTER"
	}
}

func coreCommandDesc(name string) string {
	switch name {
	case "alive":
		return "THIS COMMAND IS USED TO CHECK IF THE BOT IS ALIVE AND RUNNING. IT SHOWS A READY REPLY WITH UPTIME."
	case "ping":
		return "THIS COMMAND IS USED TO CHECK THE BOT SPEED. IT SHOWS THE RESPONSE TIME IN MILLISECONDS."
	case "uptime":
		return "THIS COMMAND IS USED TO SHOW HOW LONG THE BOT HAS BEEN RUNNING."
	case "menu":
		return "THIS COMMAND IS USED TO SHOW THE MAIN COMMAND MENU OF THE BOT."
	case "sessions":
		return "THIS COMMAND IS USED TO SHOW ALL GOLD-MD SERVERS PAIRING STATUS. IT SHOWS ONLINE AND OFFLINE SERVERS."
	case "server":
		return "THIS COMMAND IS USED TO SHOW ALL GOLD-MD SERVERS PAIRING STATUS. IT SHOWS ONLINE AND OFFLINE SERVERS."
	default:
		return ""
	}
}

// CmdLogoMenu renders the .logo menu in the SAME fancy boxed format as the
// other category menus (owner order). It reads the same per-bot settings as
// CmdMenu (owner name/number, prefix) and sends the boxed .LOGO1..1000 list.
func (s *Session) CmdLogoMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()

	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	if botName == "" ||
		botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}

	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}

	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}

	menuView := goldcmds.CmdNameViewFor(&bridge{s: s})
	caption := buildLogoMenu(menuUser, ownerNum, uptimeStr, prefix, info.PushName, botName, sessCount, menuView, menuStyleFor(s, "logo"))

	// Owner-set custom bot video (.botvideo) takes precedence; otherwise the
	// bot pic image (.botpic / default) is used, then a text-only menu.
	s.sendMenuHeader(info, "logo", caption)
}

// buildFontMenu renders the .font menu in the same fancy boxed format as the
// .logo menu and the other category menus. It lists .FONT1 .. .FONT1000.
func buildFontMenu(botNum, ownerNum, uptimeStr, prefix, pushName, botName string, sessCount int, menuView *goldcmds.CmdNameView, st goldcmds.MenuStyle) string {
	_ = pushName
	_ = botName
	_ = sessCount
	_ = menuView
	uptimeHM := uptimeStr
	if uptimeHM == "" {
		uptimeHM = formatUptimeHM(uptime())
	}
	var b strings.Builder
	b.WriteString(buildMenuHeader("FONT", botNum, ownerNum, uptimeHM, prefix, 0, goldcmds.FontCount, false, "font", st))
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled("FONT")) + "\n")
	for n := 1; n <= goldcmds.FontCount; n++ {
		b.WriteString(st.ListRow(prefix, fmt.Sprintf("FONT%d ❮ YOUR NAME ❯", n)) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	return b.String()
}

// buildGameMenu renders the .game menu in the same fancy boxed format as the
// .font / .equalizer menus. It lists .GAME1 .. .GAME1000.
func buildGameMenu(botNum, ownerNum, uptimeStr, prefix, pushName, botName string, sessCount int, menuView *goldcmds.CmdNameView, st goldcmds.MenuStyle) string {
	_ = pushName
	_ = botName
	_ = sessCount
	_ = menuView
	uptimeHM := uptimeStr
	if uptimeHM == "" {
		uptimeHM = formatUptimeHM(uptime())
	}
	var b strings.Builder
	b.WriteString(buildMenuHeader("GAME", botNum, ownerNum, uptimeHM, prefix, 0, goldcmds.GameMenuCount(), false, "game", st))
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled("GAME")) + "\n")
	for _, slug := range goldcmds.GameShortSlugs() {
		b.WriteString(st.ListRow(prefix, strings.ToUpper(slug)) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	return b.String()
}

func buildEqualizerMenu(botNum, ownerNum, uptimeStr, prefix string, sessCount int, st goldcmds.MenuStyle) string {
	_ = sessCount
	uptimeHM := uptimeStr
	if uptimeHM == "" {
		uptimeHM = formatUptimeHM(uptime())
	}
	var b strings.Builder
	b.WriteString(buildMenuHeader("EQUALIZER", botNum, ownerNum, uptimeHM, prefix, 0, goldcmds.EqCount, false, "equalizer", st))
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled("EQUALIZER")) + "\n")
	for n := 1; n <= goldcmds.EqCount; n++ {
		b.WriteString(st.ListRow(prefix, fmt.Sprintf("EQ%d ❮ %s ❯", n, goldcmds.EqDesignName(n))) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	return b.String()
}

// buildBotStyleMenu renders the .botstyle menu in the same fancy boxed format
// as the other category menus. It lists .BOTSTYLE1 .. .BOTSTYLE1000.
func buildBotStyleMenu(botNum, ownerNum, uptimeStr, prefix, pushName, botName string, sessCount int, menuView *goldcmds.CmdNameView, st goldcmds.MenuStyle) string {
	_ = pushName
	_ = botName
	_ = sessCount
	_ = menuView
	uptimeHM := uptimeStr
	if uptimeHM == "" {
		uptimeHM = formatUptimeHM(uptime())
	}
	var b strings.Builder
	b.WriteString(buildMenuHeader("BOT STYLE", botNum, ownerNum, uptimeHM, prefix, 0, goldcmds.MenuStyleCount, false, "botstyle", st))
	b.WriteString(st.BorderTop() + "\n")
	b.WriteString(st.ListRow("", st.Styled("BOT STYLE")) + "\n")
	for n := 1; n <= goldcmds.MenuStyleCount; n++ {
		b.WriteString(st.ListRow(prefix, fmt.Sprintf("BOTSTYLE%d", n)) + "\n")
	}
	b.WriteString(st.BorderBottom() + "\n\n")
	return b.String()
}

// CmdBotStyleMenu renders the .botstyle menu in the SAME fancy boxed format as
// the other category menus (owner order). Bare .botstyle opens it; each row
// .BOTSTYLE1..1000 applies that style to the whole bot.
func (s *Session) CmdBotStyleMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()
	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	if botName == "" || botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}
	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}
	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}
	menuView := goldcmds.CmdNameViewFor(&bridge{s: s})
	caption := buildBotStyleMenu(menuUser, ownerNum, uptimeStr, prefix, info.PushName, botName, sessCount, menuView, menuStyleFor(s, "botstyle"))
	s.sendMenuHeader(info, "botstyle", caption)
}

// CmdFontMenu renders the .font menu in the SAME fancy boxed format as the
// other category menus (owner order). It reads the same per-bot settings as
// CmdMenu and sends the boxed .FONT1..1000 list.
func (s *Session) CmdFontMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()

	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	if botName == "" || botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}

	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}

	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}

	menuView := goldcmds.CmdNameViewFor(&bridge{s: s})
	caption := buildFontMenu(menuUser, ownerNum, uptimeStr, prefix, info.PushName, botName, sessCount, menuView, menuStyleFor(s, "font"))

	s.sendMenuHeader(info, "font", caption)
}

// CmdGameMenu renders the .game menu in the SAME fancy boxed format as the
// other category menus (owner order). It reads the same per-bot settings as
// CmdMenu and sends the boxed .GAME1..1000 list.
func (s *Session) CmdGameMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()

	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	if botName == "" || botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}

	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}

	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}

	menuView := goldcmds.CmdNameViewFor(&bridge{s: s})
	caption := buildGameMenu(menuUser, ownerNum, uptimeStr, prefix, info.PushName, botName, sessCount, menuView, menuStyleFor(s, "game"))

	s.sendMenuHeader(info, "game", caption)
}

// CmdEqualizerMenu renders the .equalizer menu in the SAME fancy boxed format
// as the other category menus (owner order). It reads the same per-bot settings
// as CmdMenu and sends the boxed .EQ1..EQ1000 list.
func (s *Session) CmdEqualizerMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()

	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	if botName == "" || botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}

	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}

	ownerNum := botOwnNumber(s.JID)
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}

	caption := buildEqualizerMenu(menuUser, ownerNum, uptimeStr, prefix, sessCount, menuStyleFor(s, "equalizer"))

	s.sendMenuHeader(info, "equalizer", caption)
}

func (s *Session) CmdMenu(info types.MessageInfo, args []string, prefix string) {
	uptimeStr := formatUptime(uptime())
	sessCount := s.Manager.Count()

	// Custom bot name (per-bot, from Redis via .botname).
	botName := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		botName = s.Manager.Redis.GetSetting(s.JID, "botname", "")
	}
	// If the marker (or any mangled variant) is stored, show the branded
	// default label in the menu instead of the raw sentinel. The menu
	// builds its own caption, so we render the clean label here.
	if botName == "" ||
		botName == goldcmds.DefaultBotNameMarker ||
		strings.Contains(botName, "GOLD_MD_DEFAULT_FOOTER") ||
		strings.Contains(botName, "GOLD_MD_DEFAULT") {
		botName = "GOLD-MD WHATSAPP BOT"
	}

	// -- MENU "USER" field = owner display NAME (from .ownername, default "UMAR") --
	// This is the name the owner sets with  .ownername UMAR
	menuUser := "UMAR"
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownername", ""); on != "" {
			menuUser = on
		}
	}

	// -- MENU "OWNER" field = owner NUMBER (from .ownernumber, fallback = bot own number) --
	// Set with  .ownernumber 923001234567   +  sudo owners appended.
	ownerNum := botOwnNumber(s.JID) // fallback: bot own paired number
	if s.Manager != nil && s.Manager.Redis != nil {
		if on := s.Manager.Redis.GetSetting(s.JID, "ownernumber", ""); on != "" {
			ownerNum = on
		}
		if sudoRaw := s.Manager.Redis.GetSetting(s.JID, "sudowners", ""); sudoRaw != "" {
			ownerNum = ownerNum + "," + sudoRaw
		}
	}

	// ── CATEGORY SHORTCUT (owner order): .menu <category> → usi category
	// ka menu. (.group / .anti / .tools ... bhi yahan aate hain — handler.go
	// unhe CmdMenu pe route karta hai.)
	onlyCat := ""
	if len(args) > 0 {
		if cat, ok := menuCategoryFromCommand(args[0]); ok {
			onlyCat = cat
		}
	}

	// ── CMDNAME view: this bot's active command renames (nil → default menu)
	menuView := goldcmds.CmdNameViewFor(&bridge{s: s})
	// Per-menu media/style: the plain menu uses key "menu"; a category menu
	// uses its slug (e.g. "converter"), falling back to bot-wide settings.
	menuKey := "menu"
	if onlyCat != "" {
		menuKey = menuCategorySlug(onlyCat)
	}
	caption := buildCategoryMenu(menuUser, ownerNum, uptimeStr, prefix, info.PushName, botName, sessCount, menuView, onlyCat, menuStyleFor(s, menuKey))
	s.sendMenuHeader(info, menuKey, caption)
}

// menuVideoURL returns the header VIDEO for one menu (key: "menu", "logo",
// "alive", category slug, ...). Order: per-menu video override → bot-wide
// .botvideo → "" (caller falls back to the image path). Empty key means "no
// per-menu override", so callers without a specific menu keep the old
// behaviour.
//
// A menu that has its OWN picture set must NOT be overridden by the bot-wide
// video: otherwise setting .logopic appears to do nothing because every list
// still shows the global .botvideo. The per-menu picture therefore suppresses
// the bot-wide video (the owner asked for the list to carry his picture). An
// explicit per-menu VIDEO still wins over that picture.
func (s *Session) menuVideoURL(key string) string {
	if key == "" || s.Manager == nil || s.Manager.Redis == nil {
		return s.botVideoURL()
	}
	if v := s.Manager.Redis.GetSetting(s.JID, "menumedia:"+goldcmds.MenuMediaSettingKey(key, "video"), ""); v != "" {
		return v
	}
	if s.Manager.Redis.GetSetting(s.JID, "menumedia:"+goldcmds.MenuMediaSettingKey(key, "pic"), "") != "" {
		return ""
	}
	return s.botVideoURL()
}

// menuPicURL returns the header IMAGE for one menu. Order: per-menu override →
// bot-wide .botpic → default marker.
func (s *Session) menuPicURL(key string) string {
	if key != "" && s.Manager != nil && s.Manager.Redis != nil {
		if p := s.Manager.Redis.GetSetting(s.JID, "menumedia:"+goldcmds.MenuMediaSettingKey(key, "pic"), ""); p != "" {
			return p
		}
	}
	return s.botPicURL()
}

// sendMenuHeader delivers one menu's caption with the best available header:
// per-menu (then bot-wide) video first, otherwise per-menu (then bot-wide)
// picture, otherwise a text-only menu. Shared by every menu renderer so all
// menus behave identically. The menu's VOICE (per-menu → bot-wide → default)
// follows the header, and only when the owner has not silenced it.
func (s *Session) sendMenuHeader(info types.MessageInfo, key, caption string) {
	defer s.sendMenuVoice(info, key)
	if vid := s.menuVideoURL(key); vid != "" {
		if s.SendAliveVideoWithNewsletter(info, vid, caption) {
			return
		}
	}
	imgData, fetchErr := fetchMenuImageURL(s.menuPicURL(key))
	if fetchErr != nil || len(imgData) == 0 {
		s.ReplyWithNewsletter(info, caption)
		return
	}
	if ok := s.ReplyImageWithNewsletter(info, imgData, caption); !ok {
		s.ReplyWithNewsletter(info, caption)
	}
}

// fetchMenuImageURL downloads the image at the given URL and returns
// its bytes. Used by CmdMenu to fetch either the per-bot custom bot
// pic (set via .botpic) or the default menu header image.
func fetchMenuImageURL(imgURL string) ([]byte, error) {
	// A 404/HTML error page must never be uploaded as if it were a picture.
	imgURL = strings.TrimSpace(imgURL)
	if imgURL == "" || imgURL == defaultBotPicMarker {
		return defaultBotPicJPEG, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(imgURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image fetch %s: HTTP %d", imgURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return nil, fmt.Errorf("image fetch %s: not an image (%s)", imgURL, http.DetectContentType(data))
	}
	return data, nil
}

// botPicURL returns the per-bot custom menu/alive image URL when the
// owner has set one via the .botpic command (stored in Redis under the
// "botpic" field of the settings:<jid> hash).  If none is set it falls
// back to the default menuHeaderImageURL constant.
func (s *Session) botPicURL() string {
	if s.Manager != nil && s.Manager.Redis != nil {
		if pic := s.Manager.Redis.GetSetting(s.JID, "botpic", ""); pic != "" {
			return pic
		}
	}
	return menuHeaderImageURL
}

// botVideoURL returns the per-bot custom menu/alive VIDEO URL when the owner
// has set one via the .botvideo command (Redis field "botvideo"). An empty
// string means "no custom video" — the caller then uses the image path.
func (s *Session) botVideoURL() string {
	if s.Manager != nil && s.Manager.Redis != nil {
		if vid := s.Manager.Redis.GetSetting(s.JID, "botvideo", ""); vid != "" {
			return vid
		}
	}
	return ""
}

// botVoiceURL returns the bot-wide VOICE URL (Redis field "botvoice"). Three
// states, because a voice is always expected unless silenced:
//   - "off"    → "" (the owner silenced it with .botvoice reset)
//   - set URL  → that URL
//   - unset    → the built-in DefaultMenuVoiceURL, so every menu carries a
//     voice out of the box.
func (s *Session) botVoiceURL() string {
	if s.Manager != nil && s.Manager.Redis != nil {
		if v := s.Manager.Redis.GetSetting(s.JID, "botvoice", ""); v != "" {
			if v == goldcmds.MenuMediaVoiceOffValue() {
				return ""
			}
			return v
		}
	}
	return goldcmds.MenuMediaDefaultVoiceURL()
}

// menuVoiceURL resolves the voice for one menu. Order: per-menu override →
// bot-wide .botvoice → built-in default. A per-menu "off" silences JUST that
// menu without touching the bot-wide voice.
func (s *Session) menuVoiceURL(key string) string {
	if key != "" && s.Manager != nil && s.Manager.Redis != nil {
		if v := s.Manager.Redis.GetSetting(s.JID, "menumedia:"+goldcmds.MenuMediaSettingKey(key, "voice"), ""); v != "" {
			if v == goldcmds.MenuMediaVoiceOffValue() {
				return ""
			}
			return v
		}
	}
	return s.botVoiceURL()
}

// sendMenuVoice fetches and sends one menu's voice as a push-to-talk-style
// audio message carrying the forwarded channel button. It returns true when an
// audio message was actually sent. All failures are silent: the menu itself
// has already been delivered, so a missing voice must never surface an error.
func (s *Session) sendMenuVoice(info types.MessageInfo, key string) bool {
	url := s.menuVoiceURL(key)
	if strings.TrimSpace(url) == "" {
		return false
	}
	data, err := fetchMediaBytes(goldcmds.ResolveDirectMediaURL(url))
	if err != nil || len(data) == 0 {
		return false
	}
	return s.SendVoiceWithNewsletter(info, data)
}

// init registers the four core status commands into the Commands map so the
// HandleMessage dispatcher (handler.go) routes them here, and so they show up
// in the auto-generated menu.
func init() {
	RegisterCommand("alive", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdAlive(info, args, prefix)
	})
	RegisterCommand("ping", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdPing(info, args, prefix)
	})
	RegisterCommand("uptime", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdUptime(info, args, prefix)
	})
	RegisterCommand("menu", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdMenu(info, args, prefix)
	})
	// .m — menu ka HIDDEN alias (chalta hai par .menu me nahi dikhta —
	// fleet_commands.go hiddenCommands me "m" true hai). Owner order.
	RegisterCommand("m", func(s *Session, info types.MessageInfo, args []string, prefix string) {
		s.CmdMenu(info, args, prefix)
	})
	// setprefix has been REMOVED - prefix management is done via the .prefix
	// command (gold-cmds/prefix.go). "sessions" ki registration ab
	// fleet_commands.go me hai (public server-menu family) - yahan se
	// duplicate hata di gayi thi.
}

// ── WELCOME / GOODBYE ENGINE (group join/leave) ────────────────────────────
//
// Ported from UMAR-MD pair.js lines 9345-9440 (group-participants.update).
// When the *events.GroupInfo event fires:
//   - evt.Join  = members who joined/were added → send WELCOME for each
//   - evt.Leave = members who left/were removed  → send GOODBYE for each
//
// For each participant:
//  1. Read the per-group config (.welcome / .goodbye on/off + custom msg).
//  2. If enabled, build the message text: replace @user with @<number>,
//     @gname with the group subject.
//  3. Fetch the group DP. If present → send image + caption + @mention.
//     If absent → send text-only + @mention.
//
// Everything runs in its own goroutine (the case launches `go ...`) so it
// never blocks the event loop.
func (s *Session) handleGroupParticipants(evt *events.GroupInfo) {
	defer func() {
		if r := recover(); r != nil {
			_ = r // never crash the event goroutine
		}
	}()
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}
	if evt == nil {
		return
	}
	br := &bridge{s: s}
	groupJID := evt.JID

	// ── JOIN → welcome ──
	if len(evt.Join) > 0 {
		if goldcmds.WelcomeIsOn(br, groupJID) {
			template := goldcmds.WelcomeMessage(br, groupJID)
			gname := br.GetGroupName(groupJID)
			// Fetch the group DP once (shared across all joiners in this event).
			dp := br.GetGroupProfilePicture(groupJID)
			for _, userJID := range evt.Join {
				s.sendWelcomeGoodbye(br, groupJID, userJID, gname, template, dp)
			}
		}
	}

	// ── LEAVE → goodbye ──
	if len(evt.Leave) > 0 {
		if goldcmds.GoodbyeIsOn(br, groupJID) {
			template := goldcmds.GoodbyeMessage(br, groupJID)
			gname := br.GetGroupName(groupJID)
			dp := br.GetGroupProfilePicture(groupJID)
			for _, userJID := range evt.Leave {
				s.sendWelcomeGoodbye(br, groupJID, userJID, gname, template, dp)
			}
		}
	}
}

// sendWelcomeGoodbye sends a single welcome/goodbye message for one user.
// It replaces @user with @<number> and @gname with the group name, then sends
// the group DP image + caption + @mention (or text-only if dp is nil).
func (s *Session) sendWelcomeGoodbye(br *bridge, groupJID, userJID types.JID, gname, template string, dp []byte) {
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	// Resolve the user's JID to a non-AD PN JID for the mention.
	cleanJID := userJID.ToNonAD()
	numberOnly := cleanJID.User
	mentionTag := "@" + numberOnly

	// Replace placeholders (case-insensitive, same as Node.js .replace(/@user/gi,...))
	text := template
	text = replaceAllIgnoreCase(text, "@user", mentionTag)
	text = replaceAllIgnoreCase(text, "@gname", gname)

	mentioned := []string{cleanJID.String()}

	if len(dp) > 0 {
		// Send image + caption + @mention.
		if err := br.SendImageWithMentions(groupJID, dp, text, mentioned); err != nil {
			// Fall back to text-only if the image send fails.
			_ = br.SendTextWithMentions(groupJID, text, mentioned)
		}
	} else {
		// No group DP → text-only with @mention.
		_ = br.SendTextWithMentions(groupJID, text, mentioned)
	}
}

// replaceAllIgnoreCase replaces all occurrences of old (case-insensitive) in s
// with new. Used for @user / @gname placeholder replacement.
func replaceAllIgnoreCase(s, old, new string) string {
	if old == "" {
		return s
	}
	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)
	var b strings.Builder
	b.Grow(len(s) + len(new))
	i := 0
	for {
		idx := strings.Index(lower[i:], oldLower)
		if idx < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+idx])
		b.WriteString(new)
		i += idx + len(old)
	}
	return b.String()
}
