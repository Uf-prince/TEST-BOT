package main

// ============================================================================
// GOLD-MD — GUARD COMPRESSOR (Render bandwidth shield)
// ============================================================================
// OWNER ORDER (Render free 5GB/month bandwidth bachao):
//   Media downloader 50MB ki 52 files bhej deta tha → Render ki free
//   bandwidth khatam → Render bot band kar deta tha. Ab GUARD har
//   media send pe lagta hai:
//
//     1. Size <= LIMIT (default 50MB) → seedha bhejo (zero cost, no msg).
//     2. Size > LIMIT → GUARD rok leta hai, chat me message:
//        "bhai itna size hum nahi bhej sakte — compressor room me
//         bheji ja rahi hai, 5-10MB karke wapas bhejenge"
//     3. COMPRESSOR ROOM: ffmpeg fast re-encode —
//          video : smart-bitrate + resolution ladder (720→480→360→240)
//          audio : 96→64→48→32 kbps mp3
//          image : scale 1920px + JPEG quality ladder
//        Target: ~5-10 MB (GOLDMD_GUARD_TARGET_MB, default 8).
//     4. Compressed media wapas WhatsApp pe — caption me size note:
//        "GUARD: 52.4 MB → 7.9 MB (-85%)"
//     5. Compress na ho paye (zip/apk/pdf jaise documents re-encode
//        nahi hote) → block + clear message (media nahi jati).
//
// ENV KNOBS (sab optional, default ON):
//   GOLDMD_GUARD_DISABLED=1    → guard OFF (emergency / host change)
//   GOLDMD_GUARD_LIMIT_MB=<n>  → reject limit (default 50)
//   GOLDMD_GUARD_TARGET_MB=<n> → compress target (default 8)
//
// FAST hona zaroori hai (owner order): libx264 "veryfast" preset +
// bitrate-precision targeting (2-pass NAHI — single pass smart bitrate),
// isliye 50MB video ~30-60s me compress ho jati hai, quality 480p SD.
// ============================================================================

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── guard kinds ────────────────────────────────────────────────────────────

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

// ── env knobs ──────────────────────────────────────────────────────────────

func guardEnabled() bool { return os.Getenv("GOLDMD_GUARD_DISABLED") != "1" }

func guardLimitBytes() int64 {
	mb := int64(50)
	if v := os.Getenv("GOLDMD_GUARD_LIMIT_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
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

// ── result ─────────────────────────────────────────────────────────────────

// guardResult is what the guard hands back to the send helper.
//   ok=false      → BLOCKED (guard already replied in chat — media NOT sent)
//   usePath=true  → caller must send g.path instead of original path
//   len(data)>0   → caller must send g.data instead of original bytes
//   note          → caption suffix (guard footer) to append when changed
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

// pass-through result (small media — no guard action).
func guardPass() guardResult { return guardResult{ok: true} }

// ── public entry points (bridge flows: info available) ────────────────────

// guardBytesBytes checks + compresses in-memory media bytes.
// When blocked it replies the guard message into info's chat.
func (b *bridge) guardBytes(info MsgInfoT, kind guardKind, data []byte, caption string) guardResult {
	if !guardEnabled() || int64(len(data)) <= guardLimitBytes() {
		return guardPass()
	}
	orig := int64(len(data))
	b.guardNotifyStart(info, kind, orig)

	// bytes → temp file → compress → read back
	src, err := writeGuardTemp(data, kind)
	if err != nil {
		b.guardNotifyFail(info, kind, orig, "temp file nahi bana")
		return guardResult{ok: false}
	}
	out, _, note, ok := guardCompressFile(kind, src, guardTargetBytes(), guardLimitBytes())
	_ = os.Remove(src)
	if !ok {
		b.guardNotifyFail(info, kind, orig, "compress nahi ho payi")
		return guardResult{ok: false}
	}
	comp, err := os.ReadFile(out)
	_ = os.Remove(out)
	if err != nil || len(comp) == 0 {
		b.guardNotifyFail(info, kind, orig, "compressed file read fail")
		return guardResult{ok: false}
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
	if !guardEnabled() {
		return guardPass()
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() <= guardLimitBytes() {
		return guardPass()
	}
	orig := st.Size()
	b.guardNotifyStart(info, kind, orig)

	out, outSize, note, ok := guardCompressFile(kind, path, guardTargetBytes(), guardLimitBytes())
	if !ok {
		b.guardNotifyFail(info, kind, orig, "compress nahi ho payi")
		return guardResult{ok: false}
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

// ── guard messages (WhatsApp style, owner ke style me) ─────────────────────

func guardStartText(kind guardKind, orig int64) string {
	return "*🛡️ GOLD GUARD — BANDWIDTH SHIELD*\n\n" +
		"📦 *" + kind.label() + " SIZE:* " + guardFmtMB(orig) + "\n" +
		"🚫 *LIMIT:* " + guardFmtMB(guardLimitBytes()) + "\n\n" +
		"❌ _Bhai itna size hum nahi bhej sakte!_\n" +
		"_(server ki free bandwidth khatam ho jayegi)_\n\n" +
		"🏭 Media *COMPRESSOR ROOM* me bheji gayi hai...\n" +
		"⚙️ Fast compress chal raha hai (target ~" + guardFmtMB(guardTargetBytes()) + ")\n" +
		"⏳ Ruko, compressed version hi bhejunga..."
}

func guardDoneText(orig, comp int64) string {
	saved := 100 - (comp * 100 / orig)
	return "*🛡️ GUARD COMPRESS DONE ✅*\n" +
		"📉 " + guardFmtMB(orig) + " → " + guardFmtMB(comp) + " (-" + strconv.FormatInt(saved, 10) + "%)\n" +
		"💾 Server bandwidth bach gayi!"
}

func guardFailText(kind guardKind, orig int64, why string) string {
	return "*🛡️ GOLD GUARD — MEDIA BLOCKED 🚫*\n\n" +
		"📦 *" + kind.label() + " SIZE:* " + guardFmtMB(orig) + "\n" +
		"🚫 *LIMIT:* " + guardFmtMB(guardLimitBytes()) + "\n\n" +
		"❌ _Bhai itna size hum nahi bhej sakte!_\n" +
		"⚠️ Compressor bhi ise chhota nahi kar paya (" + why + ")\n\n" +
		"_Ye " + kind.label() + " server se send NAHI hogi — bandwidth bachani hai._\n" +
		"_Chhoti file bhejo ya compress karke bhejo 🙏_"
}

// notify helpers route text into the right chat without media recursion.
func (b *bridge) guardNotifyStart(info MsgInfoT, kind guardKind, orig int64) {
	if b == nil || b.s == nil {
		return
	}
	if mi, ok := info.(InfoT); ok {
		b.s.Reply(mi, guardStartText(kind, orig))
	}
	ErrLog("[GUARD] %s %s blocked-send, compressor room me — %s", b.s.JID, kind.label(), guardFmtMB(orig))
}

func (b *bridge) guardNotifyDone(info MsgInfoT, orig, comp int64) {
	if b == nil || b.s == nil {
		return
	}
	if mi, ok := info.(InfoT); ok {
		b.s.Reply(mi, guardDoneText(orig, comp))
	}
}

func (b *bridge) guardNotifyFail(info MsgInfoT, kind guardKind, orig int64, why string) {
	if b == nil || b.s == nil {
		return
	}
	if mi, ok := info.(InfoT); ok {
		b.s.Reply(mi, guardFailText(kind, orig, why))
	}
	ErrLog("[GUARD] %s %s %s compress FAIL (%s) — blocked", b.s.JID, kind.label(), guardFmtMB(orig), why)
}

// InfoT mirrors types.MessageInfo (guarded import lives in guard_impl.go).
type InfoT = types.MessageInfo

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
