package main

// ============================================================================
// GOLD-MD — GUARD COMPRESSOR (Render bandwidth shield)
// ============================================================================
// OWNER ORDER v2 (FORCE-COMPRESS): "chahye video audio image 5mb ki ho ya
// 500mb ki ya 1gb ki — jitni marzi mb ki file bot download kre compressor
// me bhejo compress krwa ker fir whatsapp me bhejo"
//
//   1. HAR media (video/audio/image) COMPRESSOR ROOM me jati hai:
//        - 5MB ho ya 20MB ya 500MB ya 1GB — sab compress hoke hi jati hai
//        - compressed version original se CHHOTA ho tabhi swap (warna
//          original hi jata hai — quality nuksan bekar me nahi)
//        - compress fail ho (<=limit files) → original passthrough
//   2. FLOOR (default 1MB) se chhoti media seedha pass — thumbnails,
//      stickers, chhoti profile pics pe ffmpeg spam nahi.
//   3. LIMIT (default 50MB) se badi media → GUARD rokta hai:
//      "bhai itna size hum nahi bhej sakte" → compressor room →
//      compress hua to bhejo, NAHI hua to BLOCK (bandwidth bachani hai).
//   4. Documents (zip/apk/pdf) re-encode nahi hote: <=limit pass,
//      >limit BLOCK.
//
// ENGINE (owner order): .compress wala FAST engine —
//   video : FIXED 360p + libx264 ultrafast + bitrate ladder 500→64k
//   audio : FIXED 128kbps mp3 (ladder 128→96→64→48→32)
//   image : JPEG quality 80→60→40 + scale 1920→1280→1024
//
// ENV KNOBS (sab optional):
//   GOLDMD_GUARD_DISABLED=1    → guard OFF (emergency / host change)
//   GOLDMD_GUARD_FORCE=0       → force mode OFF (sirf >limit compress hota)
//   GOLDMD_GUARD_LIMIT_MB=<n>  → reject limit (default 50)
//   GOLDMD_GUARD_FLOOR_MB=<n>  → is se chhoti media pass (default 1)
//   GOLDMD_GUARD_TARGET_MB=<n> → compress target (default 8)
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── guard kinds ─────────────────────────────────────────────────────────────

type guardKind int

const (
	guardVideo guardKind = iota
	guardAudio
	guardImage
	guardDocument
	guardSticker
)

func (k guardKind) label() string {
	switch k {
	case guardVideo:
		return "VIDEO"
	case guardAudio:
		return "AUDIO"
	case guardImage:
		return "IMAGE"
	case guardDocument:
		return "FILE"
	case guardSticker:
		return "STICKER"
	}
	return "MEDIA"
}

// ── env knobs ───────────────────────────────────────────────────────────────

func guardEnabled() bool { return os.Getenv("GOLDMD_GUARD_DISABLED") != "1" }

// guardForceMode: HAR media compressor room me (owner order v2). Default ON.
// GOLDMD_GUARD_FORCE=0 → purana limit-only mode.
func guardForceMode() bool {
	if v := os.Getenv("GOLDMD_GUARD_FORCE"); v != "" {
		return v != "0"
	}
	return true
}

func guardLimitBytes() int64 {
	mb := int64(150)
	if v := os.Getenv("GOLDMD_GUARD_LIMIT_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			mb = n
		}
	}
	return mb * 1024 * 1024
}

// guardFloorBytes: is se chhoti media compress NAHI hoti (thumbnails/stickers).
func guardFloorBytes() int64 {
	mb := int64(1)
	if v := os.Getenv("GOLDMD_GUARD_FLOOR_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			mb = n
		}
	}
	return mb * 1024 * 1024
}

func guardTargetBytes() int64 {
	mb := int64(8)
	if v := os.Getenv("GOLDMD_GUARD_TARGET_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			mb = n
		}
	}
	return mb * 1024 * 1024
}

func guardFmtMB(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

// ── result ──────────────────────────────────────────────────────────────────

// guardResult is what the guard hands back to the send helper.
//
//	ok=false      → BLOCKED (guard already replied in chat — media NOT sent)
//	usePath=true  → caller must send g.path instead of original path
//	len(data)>0   → caller must send g.data instead of original bytes
//	note          → caption suffix (guard footer) to append when changed
type guardResult struct {
	ok      bool
	usePath bool
	path    string
	data    []byte
	note    string
	origMB  string
	newMB   string
	cleanup []string
}

func (g *guardResult) blocked() bool { return g != nil && !g.ok }

func (g *guardResult) cleanupAll() {
	if g == nil {
		return
	}
	for _, p := range g.cleanup {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}

// pass-through result (no guard action — original media as-is).
func guardPass() guardResult { return guardResult{ok: true} }

// ── policy core (shared by bytes + path flows) ──────────────────────────────

// guardPolicy: 3-tier decision.
//
//	0 = pass       (guard OFF / below floor / force OFF & below limit)
//	1 = force      (<=limit: compress, fail/grew → original passthrough)
//	2 = overlimit  (>limit: compress, fail → BLOCK)
func guardPolicy(size int64) int {
	if !guardEnabled() || size <= guardFloorBytes() {
		return 0
	}
	if size <= guardLimitBytes() {
		if !guardForceMode() {
			return 0
		}
		return 1
	}
	return 2
}

// ── public entry points (bridge flows: info available) ──────────────────────

// guardBytes checks + compresses in-memory media bytes.
// When blocked it replies the guard message into info's chat.
func (b *bridge) guardBytes(info MsgInfoT, kind guardKind, data []byte, caption string) guardResult {
	switch guardPolicy(int64(len(data))) {
	case 0:
		return guardPass()
	case 2:
		return b.guardOverLimitBytes(info, kind, data)
	}
	// tier 1 — FORCE (owner order v2): chhoti/badi sab compress hoke jayegi
	orig := int64(len(data))
	if kind == guardDocument || kind == guardSticker {
		return guardPass() // docs re-encode nahi hote; <=limit → pass
	}
	src, err := writeGuardTemp(data, kind)
	if err != nil {
		return guardPass() // temp fail → original (block KAHI nahi)
	}
	out, size, note, ok := guardCompressFile(b.guardCtx(), kind, src, guardTargetBytes(), guardLimitBytes())
	_ = os.Remove(src)
	if !ok || size >= orig {
		if out != "" {
			_ = os.Remove(out)
		}
		return guardPass() // compress fail / aur bada → original hi jayegi
	}
	comp, err := os.ReadFile(out)
	_ = os.Remove(out)
	if err != nil || len(comp) == 0 {
		return guardPass()
	}
	b.guardNotifyDone(info, orig, size)
	return guardResult{
		ok:     true,
		data:   comp,
		note:   note,
		origMB: guardFmtMB(orig),
		newMB:  guardFmtMB(size),
	}
}

// guardOverLimitBytes: >limit → compressor room; fail → block + chat message.
func (b *bridge) guardOverLimitBytes(info MsgInfoT, kind guardKind, data []byte) guardResult {
	orig := int64(len(data))
	b.guardNotifyStart(info, kind, orig)

	// bytes → temp file → compress → read back
	src, err := writeGuardTemp(data, kind)
	if err != nil {
		b.guardNotifyFail(info, kind, orig, "temp file nahi bana")
		return guardPass() // SILENT: compress skip, original hi jayegi
	}
	out, _, note, ok := guardCompressFile(b.guardCtx(), kind, src, guardTargetBytes(), guardLimitBytes())
	_ = os.Remove(src)
	if !ok {
		b.guardNotifyFail(info, kind, orig, "compress nahi ho payi")
		return guardPass() // SILENT: compress fail -> original passthrough
	}
	comp, err := os.ReadFile(out)
	_ = os.Remove(out)
	if err != nil || len(comp) == 0 {
		b.guardNotifyFail(info, kind, orig, "compressed file read fail")
		return guardPass() // SILENT: read fail -> original passthrough
	}
	b.guardNotifyDone(info, orig, int64(len(comp)))
	return guardResult{
		ok:     true,
		data:   comp,
		note:   note,
		origMB: guardFmtMB(orig),
		newMB:  guardFmtMB(int64(len(comp))),
	}
}

// guardProbeMeta: ffprobe duration+size of compressed video (fallback 0s).
func guardProbeMeta(path string) (secs, w, h uint32) {
	if path == "" {
		return 0, 0, 0
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "x")
		if len(parts) == 2 {
			ww, _ := strconv.ParseUint(parts[0], 10, 32)
			hh, _ := strconv.ParseUint(parts[1], 10, 32)
			w, h = uint32(ww), uint32(hh)
		}
	}
	if d := guardProbeDuration(path); d > 0 {
		secs = uint32(d)
	}
	return secs, w, h
}

// guardProbeBytes: audio duration from in-memory bytes (temp file probe).
func guardProbeBytes(data []byte) float64 {
	f, err := os.CreateTemp("", "goldguard-probe-*")
	if err != nil {
		return 0
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		return 0
	}
	f.Close()
	return guardProbeDuration(f.Name())
}

// guardPath checks + compresses a media file on disk (streaming flows).
// When blocked it replies the guard message into info's chat.
func (b *bridge) guardPath(info MsgInfoT, kind guardKind, path string, caption string) guardResult {
	st, err := os.Stat(path)
	if err != nil {
		return guardPass()
	}
	switch guardPolicy(st.Size()) {
	case 0:
		return guardPass()
	case 2:
		return b.guardOverLimitPath(info, kind, path, st.Size())
	}
	// tier 1 — FORCE: sab media compressor room (owner order v2)
	if kind == guardDocument || kind == guardSticker {
		return guardPass()
	}
	orig := st.Size()
	out, outSize, note, ok := guardCompressFile(b.guardCtx(), kind, path, guardTargetBytes(), guardLimitBytes())
	if !ok || outSize >= orig {
		if out != "" {
			_ = os.Remove(out)
		}
		return guardPass() // fail/bada → original passthrough (block NAHI)
	}
	b.guardNotifyDone(info, orig, outSize)
	return guardResult{
		ok:      true,
		usePath: true,
		path:    out,
		note:    note,
		origMB:  guardFmtMB(orig),
		newMB:   guardFmtMB(outSize),
		cleanup: []string{out},
	}
}

// guardOverLimitPath: >limit → compressor room; fail → block + chat message.
func (b *bridge) guardOverLimitPath(info MsgInfoT, kind guardKind, path string, orig int64) guardResult {
	b.guardNotifyStart(info, kind, orig)

	out, outSize, note, ok := guardCompressFile(b.guardCtx(), kind, path, guardTargetBytes(), guardLimitBytes())
	if !ok {
		b.guardNotifyFail(info, kind, orig, "compress nahi ho payi")
		return guardPass() // SILENT: compress fail -> original passthrough
	}
	if outSize >= orig {
		// compressor hi bada bana raha — SILENT: original hi jayegi
		if out != "" {
			_ = os.Remove(out)
		}
		b.guardNotifyFail(info, kind, orig, "compressor ne chhota nahi banaya")
		return guardPass() // SILENT: no gain -> original passthrough
	}
	b.guardNotifyDone(info, orig, outSize)
	return guardResult{
		ok:      true,
		usePath: true,
		path:    out,
		note:    note,
		origMB:  guardFmtMB(orig),
		newMB:   guardFmtMB(outSize),
		cleanup: []string{out},
	}
}

// MsgInfoT keeps guard.go decoupled from concrete info type churn.
type MsgInfoT = interface{}

// ── guard messages (WhatsApp style, owner ke style me) ──────────────────────

func guardStartText(kind guardKind, orig int64) string {
	// SILENT MODE (owner order): koi "BANDWIDTH SHIELD" message NAHI.
	// Compress chupke se hota hai, user ko kuch nahi dikhta.
	_ = kind
	_ = orig
	return ""
}

func guardDoneText(orig, comp int64) string {
	// SILENT MODE (owner order): compress hone pe koi follow-up reply NAHI —
	// "GUARD COMPRESS DONE" message hata diya gaya hai. Sab silently hota hai.
	_ = orig
	_ = comp
	return ""
}

func guardFailText(kind guardKind, orig int64, why string) string {
	// SILENT MODE (owner order): koi "MEDIA BLOCKED" message NAHI.
	// Compress fail ho to chupke se ignore — user ko kuch nahi dikhta.
	_ = kind
	_ = orig
	_ = why
	return ""
}

// notify helpers route text into the right chat without media recursion.
func (b *bridge) guardNotifyStart(info MsgInfoT, kind guardKind, orig int64) {
	if b == nil || b.s == nil {
		return
	}
	if txt := guardStartText(kind, orig); txt != "" {
		if mi, ok := info.(InfoT); ok {
			b.s.Reply(mi, txt)
		}
	}
	InfoLog("[GUARD] %s %s over-limit, compressor room me — %s", b.s.JID, kind.label(), guardFmtMB(orig))
}

func (b *bridge) guardNotifyDone(info MsgInfoT, orig, comp int64) {
	// SILENT MODE: done pe koi reply nahi bhejta — sirf log (user ko pata nahi chalega).
	if b == nil || b.s == nil {
		return
	}
	if orig > 0 && comp > 0 {
		InfoLog("[GUARD] silent compress done %s -> %s", guardFmtMB(orig), guardFmtMB(comp))
	}
}

func (b *bridge) guardNotifyFail(info MsgInfoT, kind guardKind, orig int64, why string) {
	if b == nil || b.s == nil {
		return
	}
	if txt := guardFailText(kind, orig, why); txt != "" {
		if mi, ok := info.(InfoT); ok {
			b.s.Reply(mi, txt)
		}
	}
	InfoLog("[GUARD] %s %s %s compress FAIL (%s) — silently ignored", b.s.JID, kind.label(), guardFmtMB(orig), why)
}

// InfoT mirrors types.MessageInfo (guarded import lives in guard_impl.go).
type InfoT = types.MessageInfo

// ── ANTIDELETE MEDIA SHIELD (owner: jani, 2026-09-16) ───────────────────────
//
// OWNER ORDER: "antidelete k media ko b is shield me add krwao ... compress kr
// ke bheje ... take storage ka size bache render ka free 5gb bandwidth outbound
// b bache".
//
// Antidelete recovery flow:
//
//	WhatsApp (media) ──download──▶ bot ──compress──▶ WhatsApp (re-upload)
//	                                  ▲
//	                    Render ka metered OUTBOUND yahan kharch hota hai
//
// Storadera me sirf chhota proto (~5-15KB: URL + mediaKey + hashes) store hota
// hai — asli media bytes NAHI. Is liye bandwidth ka asli kharch recovery ke
// SEND (re-upload) pe hota hai. guardAntideleteBytes wahi media bytes ko
// compressor room se guzaar kar chhota karta hai, taake Render ka free 5GB
// outbound bache.
//
// guardAntideleteBytes compresses in-memory antidelete media bytes using the
// SAME fast engine as the download-command shield (guardCompressFile). Returns
// (compressedBytes, note). On any failure / no-gain it returns the ORIGINAL
// bytes unchanged (never blocks a recovery).
func guardAntideleteBytes(kind guardKind, data []byte) ([]byte, string) {
	if !guardEnabled() || len(data) == 0 {
		return data, ""
	}
	// docs/stickers re-encode nahi hote; floor se chhoti media skip.
	if kind == guardDocument || kind == guardSticker {
		return data, ""
	}
	if int64(len(data)) <= guardFloorBytes() {
		return data, ""
	}
	src, err := writeGuardTemp(data, kind)
	if err != nil {
		return data, ""
	}
	defer os.Remove(src)
	out, size, note, ok := guardCompressFile(context.Background(), kind, src, guardTargetBytes(), guardLimitBytes())
	if !ok || size <= 0 || size >= int64(len(data)) {
		if out != "" {
			os.Remove(out)
		}
		return data, "" // compress fail / bada bana → original hi bhejo
	}
	comp, rerr := os.ReadFile(out)
	os.Remove(out)
	if rerr != nil || len(comp) == 0 {
		return data, ""
	}
	InfoLog("[GUARD-AD] antidelete media compressed %s -> %s (outbound saved)",
		guardFmtMB(int64(len(data))), guardFmtMB(int64(len(comp))))
	return comp, note
}

// guardAntideleteKind maps an antidelete media-type string to a guardKind.
// Returns (kind, true) when the type is compressible media.
func guardAntideleteKind(mtype string) (guardKind, bool) {
	switch mtype {
	case "videoMessage":
		return guardVideo, true
	case "audioMessage":
		return guardAudio, true
	case "imageMessage":
		return guardImage, true
	}
	return guardVideo, false
}

// guardProbeBytesMeta: video duration+w+h from in-memory bytes (temp probe).
func guardProbeBytesMeta(data []byte) (secs, w, h uint32) {
	f, err := os.CreateTemp("", "goldguard-probe-*")
	if err != nil {
		return 0, 0, 0
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		return 0, 0, 0
	}
	f.Close()
	return guardProbeMeta(f.Name())
}
