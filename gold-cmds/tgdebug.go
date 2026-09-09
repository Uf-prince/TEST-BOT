package goldcmds

// ============================================================================
// GOLD-MD — TG / download engine JSON debug logger
// File: tgdebug.go
// ============================================================================
// Owner request: ".tg download engine per JSON debug lagao".
//
// Ye logger console ko SILENT rakhta hai (purani owner request ke mutabiq
// zero console output) aur structured JSON-lines logs is file me likhta hai:
//
//	logs/tg_debug.jsonl     (working dir ke relative)
//
// Har line ek JSON object hai:
//	{"ts":"...","stage":"download_ok","cmd":"tg",...}
//
// Stages jo instrument kiye gaye hain:
//	cmd_start          — .tg / search-pick entry (raw link, chat, sender)
//	fetch_ok           — media resolve hua (kind, cdn url, channel)
//	fetch_failed       — media resolve fail (poora error text)
//	send_media_start   — download engine start
//	dl_status          — CDN HTTP status + content-length
//	dl_done            — download bytes + duration
//	dl_error           — download fail (status/err)
//	whatsappify        — ffmpeg ready-check / transcode result
//	probe_meta         — ffprobe duration/w/h
//	send_video_ok      — WhatsApp upload+send success
//	send_video_failed  — WhatsApp upload/send fail (poora error)
//	page_fetch         — t.me/s/ page fetch status
//
// Env: GOLDMD_TG_DEBUG=0 disable karne ke liye (default ON).
// ============================================================================

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	tgDebugMu  sync.Mutex
	tgDebugOn  = tgDebugInit()
	tgLogFile  *os.File
	tgLogPath  string
	tgDisabled bool // true => open fail hua, aage try na karo
)

func tgDebugInit() bool {
	v := os.Getenv("GOLDMD_TG_DEBUG")
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return false
	}
	return true // default ON (owner ne debug maanga hai)
}

// tgDebugFilePath — logs/tg_debug.jsonl (working dir relative).
func tgDebugFilePath() string {
	return filepath.Join("logs", "tg_debug.jsonl")
}

// tgDebug — ek structured JSON line append karta hai log file me.
// Console par kuch nahi chhapta (owner ka zero-console-output rule).
// Fail hone par silently chup rehta hai (bot kabhi crash nahi hoga).
func tgDebug(stage string, fields map[string]any) {
	if !tgDebugOn || tgDisabled {
		return
	}
	tgDebugMu.Lock()
	defer tgDebugMu.Unlock()

	if tgLogFile == nil {
		_ = os.MkdirAll("logs", 0o755)
		f, err := os.OpenFile(tgDebugFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			tgDisabled = true // open fail — aage kabhi try nahi karenge
			return
		}
		tgLogFile = f
		tgLogPath = tgDebugFilePath()
	}

	line := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		line[k] = v
	}
	line["ts"] = time.Now().Format(time.RFC3339Nano)
	line["stage"] = stage
	line["src"] = "tg-engine"

	b, err := json.Marshal(line)
	if err != nil {
		// non-marshalable value — safe fallback: string key/values
		fb := map[string]any{"ts": time.Now().Format(time.RFC3339Nano), "stage": stage, "src": "tg-engine", "marshal_error": err.Error()}
		for k, v := range fields {
			fb[k] = toStringSafe(v)
		}
		if b, err = json.Marshal(fb); err != nil {
			return
		}
	}
	_, _ = tgLogFile.Write(append(b, '\n'))
}

// tgDebugErr — error-tagged JSON line. err nil ho to bhi fields log hote hain
// (err_text "" ke sath) — dono cases capture.
func tgDebugErr(stage string, err error, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	if err != nil {
		fields["err"] = err.Error()
	}
	tgDebug(stage, fields)
}

// toStringSafe — kisi bhi value ko JSON-safe string bana deta hai.
func toStringSafe(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "unmarshalable"
	}
	return string(b)
}

// tgDebugClose — process shutdown par file flush/close (best effort).
func tgDebugClose() {
	tgDebugMu.Lock()
	defer tgDebugMu.Unlock()
	if tgLogFile != nil {
		_ = tgLogFile.Close()
		tgLogFile = nil
	}
}
