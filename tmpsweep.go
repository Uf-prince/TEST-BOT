package main

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

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
