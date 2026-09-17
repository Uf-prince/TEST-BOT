package main

import (
	"fmt"
	signalLogger "go.mau.fi/libsignal/logger"
	"go.mau.fi/whatsmeow/types"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ══════════════════ (merged from config.go) ══════════════════
// ============================================================================
//   GOLD-MD — configuration loaded from environment (NO .env FILE — EVER)
// ============================================================================

type Config struct {
	DataDir       string // where session DB + pairing dir live (default: nexstore)
	PairingDir    string // nexstore/pairing  (matches the Node bot)
	PairingSuffix string // @s.whatsapp.net   (matches the Node bot)
	DefaultPrefix string // . (matches the Node bot)
	OwnerNumbers  []string
	BatchSize     int // auto-load batch size (Node: 5)
	BatchDelaySec int // delay between batches (Node: 2s)
	PanelEnabled  bool
	PanelPort     int
	UpstashURL    string
	UpstashToken  string
	OwnerSet      map[string]bool
}

func LoadConfig() *Config {
	c := &Config{
		DataDir:       envOr("GOLDMD_DATA_DIR", "nexstore"),
		PairingSuffix: "@s.whatsapp.net",
		DefaultPrefix: envOr("GOLDMD_DEFAULT_PREFIX", "."),
		BatchSize:     envInt("GOLDMD_BATCH_SIZE", 5),
		BatchDelaySec: envInt("GOLDMD_BATCH_DELAY", 2),
		PanelEnabled:  envBool("GOLDMD_PANEL_ENABLED", true),
		PanelPort:     envInt("PORT", 11221), // hardcoded default — NO .env FILE EVER (platform env may override)
		// Upstash Redis REMOVED — storage is Storj-backed now (upstash.go).
		// Fields kept only for struct/compat; values unused.
		UpstashURL:   envOr("UPSTASH_REDIS_REST_URL", ""),
		UpstashToken: envOr("UPSTASH_REDIS_REST_TOKEN", ""),
		OwnerSet:     map[string]bool{},
	}
	c.PairingDir = c.DataDir + "/pairing"

	// owners — comma separated JIDs or raw numbers
	// Owner sirf GOLDMD_OWNER_NUMBERS env se — koi hardcoded default NAHI. (.env FILE HARGIZ NAHI)
	// FIX (trace se root cause): purana default "923158930864" ek unknown
	// number tha (original repo author ka) — unknown users ka LID SenderAlt
	// isi se match ho ke owner ban rahe the (.vv/.sudo pass). Ab owner =
	// paired phone (original system) + env-configured owners + sudo list.
	raw := envOr("GOLDMD_OWNER_NUMBERS", "")

	for _, o := range splitCSV(raw) {
		jid := normalizeJID(o)
		if jid != "" {
			c.OwnerSet[jid] = true
			c.OwnerNumbers = append(c.OwnerNumbers, jid)
		}
	}
	return c
}

// ── helpers ──────────────────────────────────────────────────────────────

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' || r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// normalizeJID turns "923078071982" or "923078071982:1@s.whatsapp.net"
// into "923078071982@s.whatsapp.net".
func normalizeJID(s string) string {
	if s == "" {
		return ""
	}
	if i := indexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	if i := indexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return ""
	}
	return s + "@s.whatsapp.net"
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func (c *Config) IsOwner(jid string) bool {
	return c.OwnerSet[jid]
}

func (c *Config) String() string {
	return fmt.Sprintf("data=%s pairing=%s prefix=%q batch=%d delay=%d panel=%v/%d owners=%d redis=%v",
		c.DataDir, c.PairingDir, c.DefaultPrefix, c.BatchSize, c.BatchDelaySec,
		c.PanelEnabled, c.PanelPort, len(c.OwnerNumbers), true)
}


// ══════════════════ (merged from logger.go) ══════════════════
const (
	cReset   = "\x1b[0m"
	cRed     = "\x1b[31m"
	cGreen   = "\x1b[32m"
	cCyan    = "\x1b[36m"
	cGray    = "\x1b[90m"
	cBold    = "\x1b[1m"
	cYellow  = "\x1b[33m"
	cMagenta = "\x1b[35m"
)

func ts() string { return time.Now().Format("15:04:05") }

// debugEnabled is read once at startup. When GOLDMD_DEBUG=1 (or "true"/"yes")
// the bot prints verbose JSON-tagged logs so the full pairing / reconnect /
// Redis-save / Redis-restore flow is visible in the console. Once everything
// is confirmed working, simply leave GOLDMD_DEBUG unset and the bot goes quiet
// again (same behavior as the original code).
//
// NOTE: ALL console/debug logging has been DISABLED per owner request.
// Every log function below is now a no-op (silent). To re-enable, restore
// the original fmt.Fprintf bodies. (GOLDMD_DEBUG env var no longer has any
// effect because all functions return immediately.)
var debugEnabled = initDebug()

func initDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOLDMD_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ============================================================================
// ALL LOG FUNCTIONS ARE NO-OPS (silent) — owner requested zero console output.
// To re-enable any of these, uncomment the fmt.Fprintf lines inside each body.
// ============================================================================

func errLine(tag, msg string, args ...any) {
	// DISABLED — no console output
	_ = tag
	_ = msg
	_ = args
}

func infoLine(tag, color, msg string, args ...any) {
	// DISABLED — no console output
	_ = tag
	_ = color
	_ = msg
	_ = args
}

// When GOLDMD_DEBUG is set, these print. Otherwise they are silent (original behavior).
// NOW: always silent (no-op) per owner request.
func InfoLog(msg string, args ...any)  { _ = msg; _ = args }
func WarnLog(msg string, args ...any)  { _ = msg; _ = args }
func OkLog(msg string, args ...any)    { _ = msg; _ = args }
func DebugLog(msg string, args ...any) { _ = msg; _ = args }

func ErrLog(msg string, args ...any) { _ = msg; _ = args }

func FatalLog(msg string, args ...any) {
	// DISABLED — no console output, but still exit on fatal
	_ = msg
	_ = args
	os.Exit(1)
}

// antiDebugAlwaysOn forces the antidelete/antiedit JSON debugging in the MAIN
// package to ALWAYS print (regardless of GOLDMD_DEBUG).
// NOW: disabled — all JSONDebug calls are silent no-ops.
const antiDebugAlwaysOn = true

// isAntiStage reports whether a JSONDebug stage belongs to the
// antidelete/antiedit/antistatus subsystem (so it should always be printed).
// NOW: always returns false so JSONDebug never prints.
func isAntiStage(stage string) bool {
	_ = stage
	return false
}

// JSONDebug prints a structured JSON log line tagged with a stage label.
// NOW: silent no-op per owner request (zero console output).
func JSONDebug(stage string, fields map[string]any) {
	_ = stage
	_ = fields
}

// JSONDebugErr is a convenience wrapper that prints an error-tagged JSON line.
// NOW: silent no-op per owner request.
func JSONDebugErr(stage string, err error, extra map[string]any) {
	_ = stage
	_ = err
	_ = extra
}

func jsonCompact(m map[string]any) string {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for k, v := range m {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteByte('"')
		b.WriteString(k)
		b.WriteString("\": ")
		b.WriteString(jsonVal(v))
	}
	b.WriteByte('}')
	return b.String()
}

func jsonVal(v any) string {
	switch x := v.(type) {
	case string:
		return "\"" + x + "\""
	case fmt.Stringer:
		return "\"" + x.String() + "\""
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ══════════════════ (merged from busy.go) ══════════════════
// ============================================================================
// GOLD-MD — In-flight command (busy) tracking
// File: busy.go
// ============================================================================
// The watchdog (reconnect_watchdog.go) and the memory watchdog (main.go
// memoryWatchdog) both used to fire "bot is stuck" reactions while a heavy
// DOWNLOAD command was still running:
//   - cgroup memory.current (which includes file page-cache) spikes while a
//     big media file is being written to /tmp -> memoryWatchdog Tier 2
//     self-restarted the whole process mid-download.
//   - a transient socket blip during heavy traffic -> Disconnected-event
//     fast-reconnect raced the in-flight download.
//
// Every command dispatch in handler.go now wraps its run with
// beginCmdBusy()/defer-style endCmdBusy(). While ANY command (a download
// pipeline runs up to 3 minutes inside RunWithTimeout) is in flight,
// cmdBusyActive() == true and both watchdogs HOLD OFF:
//   - no self-restart (the download completes first),
//   - no fast-path reconnect (whatsmeow's own EnableAutoReconnect is still
//     there as the safety net for a real disconnect).
// ============================================================================

// cmdBusyCount is the number of currently-executing command dispatches.
var cmdBusyCount int32

// beginCmdBusy marks a command dispatch as in-flight.
func beginCmdBusy() { atomic.AddInt32(&cmdBusyCount, 1) }

// endCmdBusy clears one in-flight command dispatch.
func endCmdBusy() {
	// floor at zero — a bug must never make this go negative and stick busy.
	for {
		v := atomic.LoadInt32(&cmdBusyCount)
		if v <= 0 || atomic.CompareAndSwapInt32(&cmdBusyCount, v, v-1) {
			return
		}
	}
}

// cmdBusyActive reports whether at least one command (possibly a long
// download) is currently executing.
func cmdBusyActive() bool { return atomic.LoadInt32(&cmdBusyCount) > 0 }

// ══════════════════ (merged from memlow.go) ══════════════════
// ============================================================================
// GOLD-MD — memlow.go  (OWNER REQUEST: "RAM sirf 30-40 MB, baki sab disk pe")
// Ultra-aggressive memory optimization:
//   1. GOMEMLIMIT   → hard soft-limit: GC runs the moment heap crosses cap
//   2. GOGC=10      → GC fires ~10x more often (tiny live-set = tiny RAM)
//   3. Periodic GC  → every 60s a background GC sweep keeps RSS pinned low
//   4. Disk cache   → message/media cache → disk files (nexstore/cache)
//                      instead of RAM maps
// ============================================================================

// ramTargetMB is the soft RAM target (default 40 MB, env-overridable).
var ramTargetMB = envInt("GOLDMD_RAM_TARGET_MB", 300)

// memlowInit MUST be called first thing in main() — BEFORE LoadConfig.
// It pins the Go runtime into ultra-low-RAM mode:
//   - GOMEMLIMIT = ramTarget (soft limit; GC runs aggressively near it)
//   - GOGC = 10  → GC fires when heap grows just 10% above live-set
//   - periodic 60s forced GC keeps RSS from drifting
func memlowInit() {
	// 1) GOMEMLIMIT — soft ceiling. GC harder as heap approaches target.
	//    (Runtime env GOMEMLIMIT still overrides via os.Setenv → debug.SetMemoryLimit)
	limitBytes := int64(ramTargetMB) * 1024 * 1024
	debug.SetMemoryLimit(limitBytes)

	// 2) GOGC — GC every 10% growth (default 100 = 2x live-set → lazy).
	debug.SetGCPercent(35)

	// 3) Background sweeper: force a GC every 60s (idle RAM trimming).
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			runtime.GC()
			debug.FreeOSMemory()
		}
	}()
}

// diskCacheDir returns the on-disk cache location (created lazily).
// Owner rule: media/message caches → disk, RAM sirf sessions ke liye.
func diskCacheDir() string {
	dir := envOr("GOLDMD_CACHE_DIR", "nexstore/cache")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// memlowStatus reports current runtime memory stats (used by panel/status cmds).
func memlowStatus() map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return map[string]any{
		"target_mb":   ramTargetMB,
		"heap_alloc":  ms.HeapAlloc / (1024 * 1024),
		"heap_sys":    ms.HeapSys / (1024 * 1024),
		"rss_mb":      selfRSSBytes() / (1024 * 1024),
		"num_gc":      ms.NumGC,
		"gogc":        "10",
		"memlimit_mb": ramTargetMB,
	}
}

// ══════════════════ (merged from tmpsweep.go) ══════════════════
// ============================================================================
// GOLD-MD — tmpsweep.go  (OWNER REQUEST: "downloaded files bhejne ke baad
// delete karwa do, disk free rahe")
//
// Safety net — downloader commands ke defer cleanup ke UPAR ek guaranteed
// sweeper:
//   1. Startup sweep   → bot start hone pe /tmp ki saari gold-md temp
//                        files delete (crash/kill se leaked files bhi clean)
//   2. Periodic sweep  → har 10 minute pe /tmp ki PURANI (>10 min) temp
//                        files delete. Commands max 3 min timeout pe hain,
//                        isliye in-flight download kabhi delete nahi hoga.
//
// Sirf hamari files ko touch karta hai (safe patterns):
//   gold-md-download-*  → streamDownloadToFile outputs
//   gold-md-media-*     → sticker/media temp files
//   goldcmp-*           → compress command temp files
//
// ffmpeg static build (nexstore/ffmpeg ya /tmp/gold-ffmpeg) HATH NAHI
// lagata — wo 10 min se zyada purani rehti hai aur zaroori hai.
// ============================================================================

// tmpSweepPatterns — sirf GOLD-MD ki apni temp files.
var tmpSweepPatterns = []string{
	"gold-md-download-",
	"gold-md-media-",
	"goldcmp-",
}

// tmpSweepOnce runs at startup: delete ALL gold-md temp files.
// (Startup pe koi command in-flight nahi hota, sab safe hai — crash se
// leaked 36MB videos bhi yahi clean honge.)
func tmpSweepOnce() {
	n := tmpSweepDir(0) // age 0 = sab delete
	_ = n               // silent bot policy — zero console/file output
}

// tmpSweepLoop runs forever: every 10 minutes delete temp files older
// than 10 minutes. (Commands 3 min hard timeout pe abort hote hain, to
// 10 min purani file guaranteed orphan/leaked hai.)
func tmpSweepLoop() {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			tmpSweepDir(10 * time.Minute)
		}
	}()
}

// tmpSweepDir removes gold-md temp files older than maxAge from the
// system temp dir. Returns count of removed files. Errors ignored
// (best-effort, koi file busy ho to chhod do, agli sweep me aa jayegi).
func tmpSweepDir(maxAge time.Duration) int {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		matched := false
		for _, p := range tmpSweepPatterns {
			if strings.HasPrefix(name, p) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if maxAge > 0 && info.ModTime().After(cutoff) {
			continue // nahi purani — in-flight ho sakti hai, chhod do
		}
		if err := os.Remove(full); err == nil {
			removed++
		}
	}
	return removed
}

// tmpSweepInit — main() se call hota hai (memlowInit ke baad).
func tmpSweepInit() {
	tmpSweepOnce()
	tmpSweepLoop()
}

// ══════════════════ (merged from silent_signal_logger.go) ══════════════════
// ============================================================================
// GOLD-MD — Silent Signal Library Logger (permanent)
// File: silent_signal_logger.go
// ============================================================================
// libsignal (whatsmeow ki crypto layer) ka apna built-in logger hai jo
// logger.Setup() na hone ki soorat me DEFAULT logger use karta hai — wo
// [ERROR]/[WARNING] lines (SessionCipher.go:319 "Unable to verify
// ciphertext mac" waghera) directly stdout pe fmt.Println kar deta hai,
// hamare silent logger system ko COMPLETELY bypass kar ke.
//
// Ye file ek no-op Loggable implementation install karti hai — ab libsignal
// ke saare internal logs (Debug/Info/Warning/Error) silently drop honge.
// Zero console output, zero file writes. Vendor code untouched.
//
// (Note: SessionCipher MAC mismatch errors normal hote hain — duplicate /
// out-of-order WhatsApp messages se aate hain, connection pe recover ho
// jate hain. Owner: zero logs policy.)
// ============================================================================

// silentSignalLogger libsignal ke logger.Logger interface (Loggable) ka
// no-op implementation hai — har level silently drop.
type silentSignalLogger struct{}

func (silentSignalLogger) Debug(caller, message string)                       {}
func (silentSignalLogger) Info(caller, message string)                        {}
func (silentSignalLogger) Warning(caller, message string)                     {}
func (silentSignalLogger) Warningf(caller string, format string, args ...any) {}
func (silentSignalLogger) Error(caller, message string)                       {}
func (silentSignalLogger) Configure(settings string)                          {}

// logger-package init: silent logger ko jaldi se install karo taake koi
// bhi libsignal call (boot me hi) na print ho sake.
func init() {
	var impl signalLogger.Loggable = silentSignalLogger{}
	signalLogger.Setup(&impl)
}

// ══════════════════ (merged from commands.go) ══════════════════
// ===========================================================================
//   GOLD-MD — Owner helper commands:  sessions
//
//   The core status commands (alive, ping, menu, uptime) now live in
//   manager.go (the "main file") and use the forwarded newsletter channel
//   link button via ReplyWithNewsletter.
// ===========================================================================

// ── SESSIONS (owner) ──────────────────────────────────────────────────────
func (s *Session) CmdSessions(info types.MessageInfo, args []string, prefix string) {
	sessions := s.Manager.List()
	if len(sessions) == 0 {
		s.Reply(info, "🔰 No active sessions.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔰 *Active Sessions (%d):*\n\n", len(sessions)))
	for i, sess := range sessions {
		status := "🔴 disconnected"
		if sess.Client != nil && sess.Client.IsConnected() {
			status = "🟢 online"
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s | %s\n", i+1,
			sess.JID, status, formatUptime(time.Since(sess.Started))))
	}
	s.Reply(info, sb.String())
}
