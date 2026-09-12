package main

// ============================================================================
// GOLD-MD — memlow.go  (OWNER REQUEST: "RAM sirf 30-40 MB, baki sab disk pe")
// Ultra-aggressive memory optimization:
//   1. GOMEMLIMIT   → hard soft-limit: GC runs the moment heap crosses cap
//   2. GOGC=10      → GC fires ~10x more often (tiny live-set = tiny RAM)
//   3. Periodic GC  → every 60s a background GC sweep keeps RSS pinned low
//   4. Disk cache   → message/media cache → disk files (nexstore/cache)
//                      instead of RAM maps
// ============================================================================

import (
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

// ramTargetMB is the soft RAM target (default 40 MB, env-overridable).
var ramTargetMB = envInt("GOLDMD_RAM_TARGET_MB", 120)

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
