package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"

	goldcmds "gold-md/gold-cmds"

	_ "github.com/mattn/go-sqlite3"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// ============================================================================
// GOLD-MD — Go (Golang) multi-session WhatsApp bot
// Commands   : alive , ping , menu (+ pair , restart-session , sessions)
// Architecture: one server , many WhatsApp connectors (whatsmeow)
// ============================================================================

const Banner = `
 ▄████  ▒█████   ██▓    ▓█████▄  ▄▄▄▄    ██▓ ▄▄▄       ███▄    █ 
 ██▒ ▀█▒▒██▒  ██▒▓██▒    ▒██▀ ██▌▓█████▄ ▓██▒▒████▄     ██ ▀█   █ 
▒██░▄▄▄░▒██░  ██▒▒██░    ░██   █▌▒██▒ ▄██▒██▒▒██  ▀█▄  ▓██  ▀█ ██▒
░▓█  ██▓▒██   ██░▒██░    ░▓█▄   ▌▒██░█▀  ░██░░██▄▄▄▄██ ▓██▒  ▐▌██▒
░▒▓███▀▒░ ████▓▒░░██████▒░▒████▓ ░▓█  ▀█▓░██░ ▓█   ▓██▒▒██░   ▓██░
 ░▒   ▒ ░ ▒░▒░▒░ ░ ▒░▓  ░ ▒▒▓  ▒ ░▒▓███▀▒░▓   ▒▒   ▓▒█░░ ▒░   ▒ ▒ 
  ░   ░   ░ ▒ ▒░ ░ ░ ▒  ░ ░ ▒  ▒ ▒░▒   ░  ▒ ░  ▒   ▒▒ ░░ ░░   ░ ▒░
░ ░   ░ ░ ░ ░ ▒    ░ ░    ░ ░  ░  ░    ░  ▒ ░  ░   ▒      ░   ░ ░ 
      ░     ░ ░      ░  ░   ░     ░       ░        ░  ░         ░ 
                           ░            ░                          
     GOLD-MD · Go Multi-Session WhatsApp Bot
`

func hasUsableWhatsAppDevice(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	// Open read-only so the probe cannot mutate or lock the auth database.
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return false
	}
	defer db.Close()
	var count int
	err = db.QueryRow(`SELECT COUNT(1) FROM whatsmeow_device WHERE jid IS NOT NULL AND registration_id > 0`).Scan(&count)
	return err == nil && count > 0
}

func main() {
	// load .env from the working directory (if present) BEFORE LoadConfig()
	// reads any environment variables — real env vars (e.g. Railway ones)
	// still win if both are set.
	loadDotEnv(".env")

	InfoLog("Starting GOLD-MD server...")
	cfg := LoadConfig()

//	JSONDebug("BOOT_CONFIG", map[string]any{
//		"dataDir":    cfg.DataDir,
//		"pairingDir": cfg.PairingDir,
//		"panelPort":  cfg.PanelPort,
//		"upstashURL": cfg.UpstashURL,
//		"upstashSet": cfg.UpstashURL != "" && cfg.UpstashToken != "",
//		"owners":     cfg.OwnerNumbers,
//		"debug":      debugEnabled,
//	})

	// ensure the data dir exists before we touch anything inside it
	_ = os.MkdirAll(cfg.DataDir, 0o755)
	dbPath := filepath.Join(cfg.DataDir, "goldmd.db")

	// ── STORJ: init the Storj S3-compatible store FIRST — it now backs BOTH
	// the antidelete/antiedit message store AND the config/session storage
	// layer (Redis/Upstash is fully removed). 10 shards + 48h TTL guard
	// (guard only sweeps msgs/, the kv/ config data is permanent).
	if err := InitStorj(); err != nil {
		FatalLog("Storj init failed: %v (config + session storage require Storj)", err)
	}
	OkLog("Storj store ready (%d shards)", len(storj.shards))
	storj.StartTTLGuard()

	// ── config + session persistence layer (Storj-backed) ──
	// Per-session prefix / sudo / settings AND the full WhatsApp auth store
	// now live in Storj (same shards/buckets as antidelete) and survive
	// across redeployments — even on ephemeral disks (Modal/Railway/Fly).
	var redis *Upstash
	if os.Getenv("GOLDMD_DISABLE_UPSTASH") != "1" {
		redis = NewUpstash(cfg.UpstashURL, cfg.UpstashToken)
		if !redis.Ping() {
			ErrLog("Storj storage health check failed; session persistence may be unavailable")
		}

		// Restore before opening sqlstore whenever the local DB is missing or
		// does not contain a usable WhatsApp device. A plain file-exists check
		// is not enough: an empty/stale sqlite file can survive a restart while
		// the real auth DB is safely stored in Storj.
		localOK := hasUsableWhatsAppDevice(dbPath)
		if !localOK {
			InfoLog("Local session DB has no usable WhatsApp device — checking Storj for a backup...")
			tmpPath := dbPath + ".restore.tmp"
			_ = os.Remove(tmpPath)
			restored, rerr := redis.RestoreSessionDB(tmpPath)
			if rerr != nil {
				ErrLog("Could not restore session DB from Storj: %v", rerr)
			} else if restored {
				if rerr = os.Rename(tmpPath, dbPath); rerr != nil {
					ErrLog("Could not activate restored session DB: %v", rerr)
				} else {
					InfoLog("Storj session DB restored successfully (local DB was missing/invalid)")
					// recreate the pairing marker folders AutoLoad() scans for
					jids := redis.ListJIDs()
					for _, jid := range jids {
						_ = os.MkdirAll(filepath.Join(cfg.PairingDir, jid), 0o755)
					}
					OkLog("Recreated %d pairing folder(s) from Storj JID registry", len(jids))
				}
			} else {
				ErrLog("Storj has no session DB backup; starting fresh")
			}
		} else {
			InfoLog("Local WhatsApp session DB is valid; keeping it and skipping Storj overwrite")
		}
	} else {
		WarnLog("GOLDMD_DISABLE_UPSTASH=1 — sessions will NOT survive a disk wipe/restart.")
	}

	// ── container holds every session's SQLite auth store ──
	// whatsmeow keeps each WhatsApp session inside its own sqlite file so
	// one process can safely run many connectors (multi-session).
	ctx := context.Background()
	container, err := sqlstore.New(
		ctx,
		"sqlite3",
		fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", dbPath),
		waLog.Noop,
	)
	if err != nil {
		FatalLog("Failed to open session container: %v", err)
	}

	// ── manager owns the live sessions and the autoload loop ──
	mgr := NewManager(cfg, container)
	if redis != nil {
		mgr.Redis = redis
	}

	// ── amute/aunmute scheduler: live session lookup bridge (Node ke
	// _UmarFindTrackerForBotNumber + setInterval(30s) equivalent) ──
	goldcmds.AmuteAttachSessionLookup(func(digits string) *whatsmeow.Client {
		dgOnly := func(s string) string {
			return strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, s)
		}
		for _, sess := range mgr.List() {
			if sess.Client == nil || !sess.Client.IsConnected() {
				continue
			}
			if dgOnly(sess.JID) == digits || dgOnly(sess.Owner) == digits {
				return sess.Client
			}
		}
		return nil
	})

	// ── cmdname: full dispatchable command-name set for rename validation ──
	// .cmdname ping to umar karne se pehle bot check karta hai ki "ping"
	// waqai ek command hai — gold-cmds registry + main-package core
	// commands (ping, menu, alive, uptime, sessions) dono se.
	goldcmds.CmdNameAttachKnownCommands(func() []string {
		names := make([]string, 0, len(Commands))
		for name := range Commands {
			names = append(names, name)
		}
		return names
	})

	// self-restart (reconnect_watchdog.go) ke liye DB path expose
	selfRestartDBPath = dbPath

	go memoryWatchdog(mgr, redis, dbPath)


	// ── HTTP control panel (pair new sessions / list sessions) ──
	if cfg.PanelEnabled {
		go StartPanel(mgr, cfg.PanelPort)
		InfoLog("Pairing panel → http://0.0.0.0:%d (POST /pair , GET /sessions)", cfg.PanelPort)
	}

	// ── auto-load every saved session (batched, like autoload.js) ──
	mgr.AutoLoad()

	// ── Always-on reconnect watchdog ────────────────────────────────────────────────
	// WhatsApp idle-disconnects sessions (login zinda, socket band). Ye
	// watchdog har 30s sab sessions check karta hai aur dead socket ko
	// foran Connect() se revive karta hai. Command speed pe 0% asar.
	go mgr.watchReconnects()

	// ── periodic session-DB backup so mid-session key updates aren't lost ──
	if redis != nil {
		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				if mgr.IsShuttingDown() {
					return
				}
				if err := redis.SaveSessionDB(dbPath); err != nil {
     // ErrLog("Periodic Upstash session backup failed: %v", err)
				}
			}
		}()
		InfoLog("Periodic Upstash session backup enabled (every 10 min).")
	}

	// ── graceful shutdown on SIGINT / SIGTERM ──
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	WarnLog("Received %s — shutting down all sessions gracefully...", sig)

	// final backup before exiting, so the last few minutes aren't lost
	if redis != nil {
		if err := redis.SaveSessionDB(dbPath); err != nil {
   // ErrLog("Final session DB backup failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mgr.Shutdown(ctx)

	InfoLog("GOLD-MD stopped. Bye!")
}

// ── memory watchdog thresholds (2-tier) ──
//
// Tier 1 (cleanupThreshold): when container memory crosses this, we clear
//   in-memory caches (Upstash settings cache, Go GC + FreeOSMemory).
//   This frees ~50-150 MB without any restart — the background cache
//   refresher re-populates the settings cache on its next tick.
//
// Tier 2 (restartThreshold): if memory STILL climbs past this after cleanup,
//   we do a graceful self-restart (SaveSessionDB → disconnect → fork+exec
//   a fresh copy of the binary → os.Exit). The fresh process starts at
//   ~50-80 MB and all WhatsApp sessions reconnect from Redis/Storj.
//
// Both thresholds use cgroup memory.current (container view, not host).
// On Render Free (512 MB) this gives: cleanup at 450 MB, restart at 500 MB,
// leaving ~12 MB headroom before the cgroup hard-kill.
const (
	cleanupThreshold  uint64 = 450 * 1024 * 1024 // 450 MB → cache cleanup
	restartThreshold  uint64 = 500 * 1024 * 1024 // 500 MB → self-restart
)

// memoryWatchdog monitors container RAM in a background goroutine and
// takes corrective action BEFORE the cgroup hard limit is hit.
//
// ADAPTIVE (owner request: "speed pe 0% farak, kaam kam"): RAM normal
// (< 400 MB) -> 30s deep sleep. Warning zone (400 MB+) -> 5s fast check
// taake 450/500 ke thresholds ko bilkul wakt pe pakre. Ye kabhi
// message-processing path ko nahi chhoota, isliye bot speed pe asar
// hamesha 0% rehta hai.
func memoryWatchdog(mgr *Manager, redis *Upstash, dbPath string) {
	cleanupDone := false // reset every cycle so cleanup can fire again later

	for {
		if mgr.IsShuttingDown() {
			return
		}

		used := goldcmds.CurrentContainerMemoryBytes()
		if used == 0 {
			// cgroup not exposed (local dev / non-Linux) — fall back to Go heap
			used = goHeapBytes()
		}

		// ── Tier 1: cache cleanup at 450 MB ──
		if used >= cleanupThreshold && !cleanupDone {
			WarnLog("Container memory at %.2f MB — clearing caches to free RAM", float64(used)/(1024*1024))
			if redis != nil {
				redis.ClearCache() // drop all settings/prefix cache entries
			}
			runtimeGC() // force Go GC + return memory to OS
			cleanupDone = true
			time.Sleep(5 * time.Second)
			continue
		}

		// ── Reset cleanup flag when memory drops back below 350 MB ──
		if used < 350*1024*1024 {
			cleanupDone = false
		}

		// ── Tier 2: self-restart at 500 MB ──
		if used >= restartThreshold {
			WarnLog("Container memory at %.2f MB — initiating self-restart", float64(used)/(1024*1024))
			gracefulSelfRestart(mgr, redis, dbPath)
			return // never reached (gracefulSelfRestart exits)
		}

		// ── ADAPTIVE SLEEP ──
		// Normal RAM -> 30s deep sleep (kaam kam). Warning zone (400 MB+)
		// -> 5s fast check (thresholds ko bilkul wakt pe pakarna hai).
		if used < 400*1024*1024 {
			time.Sleep(30 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
	}
}

// gracefulSelfRestart saves the session DB, disconnects all WhatsApp clients,
// then spawns a fresh copy of the current binary and exits. The fresh process
// inherits the same env (Storj creds, Upstash URL/token, PORT, etc.) and
// restores all sessions from Redis on startup — typically reconnecting within
// 2-3 seconds. This works on Render because the process never fully dies:
// exec replaces it in-place with the child.
func gracefulSelfRestart(mgr *Manager, redis *Upstash, dbPath string) {
	// 1. Save session DB to Redis so the fresh process can restore it.
	if redis != nil {
		if err := redis.SaveSessionDB(dbPath); err != nil {
			// ErrLog("Self-restart: session DB backup failed: %v", err)
		} else {
			InfoLog("Self-restart: session DB saved to Redis")
		}
	}

	// 2. Gracefully disconnect all WhatsApp clients (quick).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Shutdown(ctx)
	InfoLog("Self-restart: all sessions disconnected")

	// 3. Resolve our own executable path.
	exe, err := os.Executable()
	if err != nil {
		// ErrLog("Self-restart: cannot find executable: %v — falling back to os.Exit", err)
		os.Exit(0)
	}

	// 4. Spawn a fresh copy with the same args + env, inheriting stdout/stderr.
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	// Detach into a new session so it survives our exit.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		// ErrLog("Self-restart: failed to spawn fresh process: %v — falling back to os.Exit", err)
		os.Exit(0)
	}

	InfoLog("Self-restart: fresh process spawned (PID %d), exiting old process", cmd.Process.Pid)
	os.Exit(0)
}

// goHeapBytes returns the current Go heap allocation as a fallback when
// cgroup memory.current is not available (local dev / macOS).
func goHeapBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// runtimeGC forces a garbage collection and returns unused memory to the OS.
func runtimeGC() {
	runtime.GC()
	debug.FreeOSMemory()
}
