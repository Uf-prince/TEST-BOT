package main

// ============================================================================
// GOLD-MD — GUARD COMPRESSOR ENGINE (fast ffmpeg re-encode)
// ============================================================================
// guardCompressFile: kind-specific fast compression ladder.
//   VIDEO : smart bitrate + res ladder (720→480→360→240) + veryfast preset
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
func guardCompressFile(kind guardKind, src string, target, maxLimit int64) (string, int64, string, bool) {
	// ffmpeg/ffprobe availability — same self-install as commands (root PATH me hai)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if !goldEnsureFfmpeg() {
			return "", 0, "", false
		}
	}
	switch kind {
	case guardVideo:
		return guardCompressVideo(src, target, maxLimit)
	case guardAudio:
		return guardCompressAudio(src, target, maxLimit)
	case guardImage, guardSticker:
		return guardCompressImage(src, target, maxLimit)
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

// ── VIDEO ladder ────────────────────────────────────────────────────────────
//
// Bitrate math: target-bytes chase. 3 accept levels:
//   A) size <= target   → PERFECT
//   B) size <= maxLimit → acceptable (limit ke andar, bandwidth safe)
//   C) size > maxLimit  → next tier (neeche resolution / bitrate)
// Ladder order: 720p → 480p → 360p → 240p (source height se clamp).
// Preset: veryfast (owner: FAST compressor). Audio 64k aac.

func guardCompressVideo(src string, target, maxLimit int64) (string, int64, string, bool) {
	dur := guardProbeDuration(src)
	if dur <= 0 {
		dur = 600 // unknown duration — assume 10 min worst case bitrate math
	}
	srcH := guardProbeHeight(src)
	if srcH <= 0 {
		srcH = 1080
	}

	type tier struct{ h, br int }
	ladder := []tier{{720, 900}, {480, 600}, {360, 420}, {240, 280}}
	// source se badi tier skip
	start := 0
	for i, t := range ladder {
		if t.h <= srcH {
			start = i
			break
		}
		if i == len(ladder)-1 {
			start = i
		}
	}
	ladder = ladder[start:]

	// best-candidate tracking: target-hit turant accept; warna sabse chhota
	// under-limit candidate yaad rakho (agla tier fail ho jaye to bhi safe).
	var bestOut string
	var bestSize int64
	var bestH int
	for _, t := range ladder {
		// audio 64k + video bitrate → target bytes chase karo
		brVideo := t.br
		// agar pehle estimate se hi target se 2x upar hai, bitrate ghatao
		est := int64(brVideo+64) * 1000 * int64(dur) / 8
		for est > target*2 && brVideo > 120 {
			brVideo = brVideo * 2 / 3
			est = int64(brVideo+64) * 1000 * int64(dur) / 8
		}
		out, err := guardTempOut(".mp4")
		if err != nil {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", src,
			"-vf", "scale=-2:"+strconv.Itoa(t.h),
			"-c:v", "libx264", "-preset", "veryfast", "-b:v", strconv.Itoa(brVideo)+"k",
			"-maxrate", strconv.Itoa(brVideo*2)+"k", "-bufsize", strconv.Itoa(brVideo*3)+"k",
			"-c:a", "aac", "-b:a", "64k",
			"-movflags", "+faststart",
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
		size := st.Size()
		if size <= target {
			// PERFECT — target hit
			if bestOut != "" {
				os.Remove(bestOut)
			}
			return out, size, guardNoteVideo(t.h, size), true
		}
		if size <= maxLimit {
			// under-limit candidate — chhota hai to best replace karo
			if bestOut != "" && size < bestSize {
				os.Remove(bestOut)
			}
			if bestOut == "" || size < bestSize {
				bestOut, bestSize, bestH = out, size, t.h
			} else {
				os.Remove(out)
			}
			continue // neeche tier se target chase ka ek aur try
		}
		// maxLimit cross — ye tier reject, agla tier
		os.Remove(out)
	}
	// target kisi tier se hit nahi hua — best under-limit fallback
	if bestOut != "" && bestSize <= maxLimit {
		return bestOut, bestSize, guardNoteVideo(bestH, bestSize), true
	}
	return "", 0, "", false
}

func guardNoteVideo(h int, size int64) string {
	return "\n\n🛡️ *GUARD:* 480p SD compress karke bheji gayi hai (bandwidth bach gayi)" + guardSizeNote(size)
}

func guardSizeNote(size int64) string {
	return "\n📦 *Compressed:* " + guardFmtMB(size)
}

// ── AUDIO ladder ────────────────────────────────────────────────────────────

func guardCompressAudio(src string, target, maxLimit int64) (string, int64, string, bool) {
	// best-tracking: target hit → turant accept; warna sabse chhota
	// under-limit bitrate yaad rakho (96→64→48→32 kbps ladder).
	var bestOut string
	var bestSize int64
	var bestBr int
	for _, br := range []int{128, 96, 64, 48, 32, 16} {
		out, err := guardTempOut(".mp3")
		if err != nil {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", src,
			"-c:a", "libmp3lame", "-b:a", strconv.Itoa(br)+"k", out)
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
			if bestOut != "" {
				os.Remove(bestOut)
			}
			return out, size, "\n\n🛡️ *GUARD:* audio compress hui (" + strconv.Itoa(br) + "kbps mp3)" + guardSizeNote(size), true
		}
		if size <= maxLimit {
			if bestOut != "" && size < bestSize {
				os.Remove(bestOut)
			}
			if bestOut == "" || size < bestSize {
				bestOut, bestSize, bestBr = out, size, br
			} else {
				os.Remove(out)
			}
			continue
		}
		os.Remove(out)
	}
	if bestOut != "" && bestSize <= maxLimit {
		return bestOut, bestSize, "\n\n🛡️ *GUARD:* audio compress hui (" + strconv.Itoa(bestBr) + "kbps mp3)" + guardSizeNote(bestSize), true
	}
	return "", 0, "", false
}

// ── IMAGE ladder (JPEG quality + scale) ────────────────────────────────────

func guardCompressImage(src string, target, maxLimit int64) (string, int64, string, bool) {
	for _, q := range []int{80, 60, 40} {
		for _, w := range []int{1920, 1280, 1024} {
			out, err := guardTempOut(".jpg")
			if err != nil {
				return "", 0, "", false
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", src,
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
				return out, st.Size(), "\n\n🛡️ *GUARD:* image compress hui" + guardSizeNote(st.Size()), true
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
