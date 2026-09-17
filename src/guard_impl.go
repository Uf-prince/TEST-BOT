package main

// ============================================================================
// GOLD-MD — GUARD COMPRESSOR ENGINE (fast ffmpeg re-encode)
// ============================================================================
// guardCompressFile: kind-specific fast compression ladder.
//   VIDEO : FIXED 360p + bitrate ladder (500→350→250→180→120k) ultrafast
//   AUDIO : 96→64→48→32 kbps mp3
//   IMAGE : JPEG quality 80→60→40 + scale 1920→1280→1024
//   DOC/OTHER : zip/apk/pdf re-encode nahi hote → FAIL (block path)
// Success jab tak size <= maxLimit (ye limit-chasing accept hai — video
// 60MB target 8 pe na pahunche to 48/32/16 tak neeche bitrate jaata).
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	goldcmds "gold-md/gold-cmds"
)

// guardCompressFile compresses the media at src for kind, chasing target
// bytes. Returns (outPath, outSize, captionNote, ok). ok=false → caller
// must block the send (compressor room se bahar nahi bani).
func guardCompressFile(ctx context.Context, kind guardKind, src string, target, maxLimit int64) (string, int64, string, bool) {
	// ffmpeg/ffprobe availability — same self-install as commands (root PATH me hai)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if !goldEnsureFfmpeg() {
			return "", 0, "", false
		}
	}
	switch kind {
	case guardVideo:
		return guardCompressVideo(ctx, src, target, maxLimit)
	case guardAudio:
		return guardCompressAudio(ctx, src, target, maxLimit)
	case guardImage, guardSticker:
		return guardCompressImage(ctx, src, target, maxLimit)
	default: // documents (zip/apk/pdf/etc.) — re-encode impossible
		return "", 0, "", false
	}
}

// goldEnsureFfmpeg mirrors gold-cmds' ensureFfmpeg without an import cycle
// (main pkg → gold-cmds is fine; this wrapper just calls the exported one).
func goldEnsureFfmpeg() bool { return goldcmds.EnsureFfmpegPublic() }

// ── duration probing (ffprobe) ──────────────────────────────────────────────

func guardProbeDuration(src string) float64 {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", src).Output()
	if err != nil {
		return 0
	}
	var f float64
	if _, perr := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &f); perr == nil {
		return f
	}
	return 0
}

// guardProbeHeight returns video height (0 = unknown).
func guardProbeHeight(src string) int {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "csv=p=0", src).Output()
	if err != nil {
		return 0
	}
	h, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return h
}

func guardTempOut(ext string) (string, error) {
	f, err := os.CreateTemp("", "goldguard-*"+ext)
	if err != nil {
		return "", err
	}
	p := f.Name()
	f.Close()
	return p, nil
}

// ── VIDEO (.compress engine pattern: ultrafast + fixed 360p) ───────────────
//
// Owner order: video FIXED 360p + .compress wala FAST engine
// (libx264 ultrafast, target-bitrate, maxrate/bufsize, faststart, threads 2).
// Bitrate ladder 500→350→250→180→120k chase karta hai jab tak target hit.
// Source 360p se chhota ho to upscale NAHI (same-res lower bitrate).

func guardCompressVideo(ctx context.Context, src string, target, maxLimit int64) (string, int64, string, bool) {
	dur := guardProbeDuration(src)
	if dur <= 0 {
		dur = 600 // unknown duration — assume 10 min worst case for bitrate math
	}
	srcH := guardProbeHeight(src)
	if srcH <= 0 {
		srcH = 1080
	}

	// fixed 360p (source chhota to same-res — never upscale)
	height := 360
	if srcH < height {
		height = srcH
	}

	// bitrate ladder — .compress 360p tier = 500k start, target chase se neeche
	type cand struct {
		out  string
		size int64
		br   int
	}
	var best cand
	for _, br := range []int{500, 350, 250, 180, 120, 96, 64} {
		out, err := guardTempOut(".mp4")
		if err != nil {
			break
		}
		// 1GB tak videos: timeout size ke hisaab se (max 10 min)
		tmo := 4 * time.Minute
		if srcSt, e := os.Stat(src); e == nil && srcSt.Size() > int64(200<<20) {
			tmo = 10 * time.Minute
		}
		cctx, cancel := context.WithTimeout(ctx, tmo)
		args := []string{"-y", "-i", src,
			"-c:v", "libx264",
			"-b:v", strconv.Itoa(br) + "k",
			"-maxrate", strconv.Itoa(br*3/2) + "k",
			"-bufsize", strconv.Itoa(br*2) + "k",
			"-preset", "ultrafast",
			"-movflags", "+faststart",
			"-threads", "2",
			"-vf", "scale=-2:" + strconv.Itoa(height),
			"-c:a", "aac", "-b:a", "128k",
			out,
		}
		cmd := exec.CommandContext(cctx, "ffmpeg", args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		err = cmd.Run()
		cancel()
		if err != nil {
			os.Remove(out)
			continue
		}
		st, serr := os.Stat(out)
		if serr != nil || st.Size() == 0 {
			os.Remove(out)
			continue
		}
		size := st.Size()
		if size <= target {
			if best.out != "" {
				os.Remove(best.out)
			}
			return out, size, guardNoteVideo(height, size), true
		}
		if size <= maxLimit {
			// under-limit lekin target se bada — chhota hai to best yaad rakho
			// (pehla under-limit candidate bhi store ho — best.out=="" case)
			if best.out == "" || size < best.size {
				if best.out != "" {
					os.Remove(best.out)
				}
				best = cand{out, size, br}
			} else {
				os.Remove(out)
			}
			continue
		}
		os.Remove(out)
	}
	if best.out != "" && best.size <= maxLimit {
		return best.out, best.size, guardNoteVideo(height, best.size), true
	}
	return "", 0, "", false
}

func guardNoteAudio(br int, size int64) string {
	return "" // SILENT: audio compress note hata diya
}

func guardNoteVideo(h int, size int64) string {
	return "" // SILENT: video compress note hata diya
}

func guardSizeNote(size int64) string {
	return "\n📦 *Compressed:* " + guardFmtMB(size)
}

// ── AUDIO ladder ────────────────────────────────────────────────────────────

func guardCompressAudio(ctx context.Context, src string, target, maxLimit int64) (string, int64, string, bool) {
	// Owner order: audio FIXED 128kbps mp3 (.compress HIGH preset wala encoder).
	// Agar 128k pe bhi target cross ho jaye (bahut lambi audio) to hi neeche
	// bitrate ladder (96→64→48→32) — warna 128k hi (fastest, best quality).
	type cand struct {
		out  string
		size int64
		br   int
	}
	var best cand
	for _, br := range []int{128, 96, 64, 48, 32} {
		out, err := guardTempOut(".mp3")
		if err != nil {
			break
		}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		cmd := exec.CommandContext(cctx, "ffmpeg", "-y", "-i", src,
			"-vn", "-c:a", "libmp3lame", "-b:a", strconv.Itoa(br)+"k",
			"-f", "mp3", out)
		cmd.Stdout = nil
		cmd.Stderr = nil
		err = cmd.Run()
		cancel()
		if err != nil {
			os.Remove(out)
			continue
		}
		st, serr := os.Stat(out)
		if serr != nil || st.Size() == 0 {
			os.Remove(out)
			continue
		}
		size := st.Size()
		if size <= target {
			if best.out != "" {
				os.Remove(best.out)
			}
			return out, size, guardNoteAudio(br, size), true
		}
		if size <= maxLimit {
			// under-limit lekin target se bada — chhota hai to best yaad rakho
			if best.out == "" || size < best.size {
				if best.out != "" {
					os.Remove(best.out)
				}
				best = cand{out, size, br}
			} else {
				os.Remove(out)
			}
			continue
		}
		os.Remove(out)
	}
	if best.out != "" && best.size <= maxLimit {
		return best.out, best.size, guardNoteAudio(best.br, best.size), true
	}
	return "", 0, "", false
}

// ── IMAGE ladder (JPEG quality + scale) ────────────────────────────────────

func guardCompressImage(ctx context.Context, src string, target, maxLimit int64) (string, int64, string, bool) {
	// force-tier: compressed >= original ho to original hi behtar (caller
	// guardPass karega). Isliye output size vs SOURCE size bhi compare hota hai.
	if srcSt, e := os.Stat(src); e == nil {
		srcSize := srcSt.Size()
		out, size, note, ok := guardCompressImageRun(ctx, src, target, maxLimit, srcSize)
		if !ok {
			return "", 0, "", false
		}
		if size >= srcSize { // smaller-of-two: bada hua to original behtar
			if out != "" {
				os.Remove(out)
			}
			return "", 0, "", false
		}
		return out, size, note, true
	}
	return guardCompressImageRun(ctx, src, target, maxLimit, 0)
}

func guardCompressImageRun(ctx context.Context, src string, target, maxLimit, srcSize int64) (string, int64, string, bool) {

	for _, q := range []int{80, 60, 40} {
		for _, w := range []int{1920, 1280, 1024} {
			out, err := guardTempOut(".jpg")
			if err != nil {
				return "", 0, "", false
			}
			cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			cmd := exec.CommandContext(cctx, "ffmpeg", "-y", "-i", src,
				"-vf", "scale="+strconv.Itoa(w)+":-2",
				"-q:v", strconv.Itoa(q),
				out)
			cmd.Stdout = nil
			cmd.Stderr = nil
			err = cmd.Run()
			cancel()
			if err != nil {
				os.Remove(out)
				continue
			}
			st, serr := os.Stat(out)
			if serr != nil || st.Size() == 0 {
				os.Remove(out)
				continue
			}
			if st.Size() <= maxLimit {
				return out, st.Size(), "" /* SILENT: image compress note hata diya */, true
			}
			os.Remove(out)
		}
	}
	return "", 0, "", false
}

// writeGuardTemp writes bytes to a temp file with the right extension.
func writeGuardTemp(data []byte, kind guardKind) (string, error) {
	ext := ".bin"
	switch kind {
	case guardVideo:
		ext = ".mp4"
	case guardAudio:
		ext = ".mp3"
	case guardImage, guardSticker:
		ext = ".jpg"
	case guardDocument:
		ext = ".bin"
	}
	f, err := os.CreateTemp("", "goldguard-src-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// guardHookBytes: small in-memory media → guard note (no compression below limit).
// Kept for API symmetry; SendImage bytes path uses it for footer notes.
func guardHookBytes(kind guardKind, data []byte) (note string, changed bool) {
	if !guardEnabled() || int64(len(data)) <= guardLimitBytes() {
		return "", false
	}
	return "", false
}
