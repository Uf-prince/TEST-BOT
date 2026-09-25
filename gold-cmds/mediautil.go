package goldcmds

// ============================================================================
// GOLD-MD — Shared media helpers for downloader commands
// File: mediautil.go
// ============================================================================
// Utility functions reused by fb.go, insta.go, tiktok.go, tomp3.go, sticker.go:
//   - removeTempFile        : best-effort temp file cleanup
//   - probeVideoMeta        : ffprobe-based duration/width/height (0 if absent)
//   - mediaHTTPClient       : a shared HTTP client with a generous timeout
//   - isFfmpegAvailable     : runtime check for ffmpeg/ffprobe binaries
// ============================================================================

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// removeTempFile deletes a temp file, ignoring errors (best-effort cleanup).
func removeTempFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// mediaHTTPClient is a shared HTTP client used by downloader commands.
func mediaHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute}
}

// isFfmpegAvailable reports whether the ffmpeg binary is callable.
// OWNER RULE (any-platform deploy): ffmpeg system pe nahi bhi ho to
// ensureFfmpeg() static build download karke PATH me daal deta hai —
// commands kabhi "ffmpeg not found" error na dein.
func isFfmpegAvailable() bool {
	if ensureFfmpeg() {
		return true
	}
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// isFfprobeAvailable reports whether the ffprobe binary is callable.
// (ensureFfmpeg() static build me ffprobe bhi included hai.)
func isFfprobeAvailable() bool {
	if ensureFfmpeg() {
		_, err := exec.LookPath("ffprobe")
		return err == nil
	}
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// probeVideoMeta runs ffprobe to extract the duration (seconds) and the
// width/height of a video file. Returns (0,0,0) when ffprobe is unavailable
// or the probe fails — the caller can still send the video with zeros
// (WhatsApp accepts it). Keeps memory bounded (no media loaded into RAM).
func probeVideoMeta(path string) (seconds uint32, width uint32, height uint32) {
	if !isFfprobeAvailable() {
		return 0, 0, 0
	}
	// Duration
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err == nil {
		if f, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); perr == nil && f > 0 {
			seconds = uint32(f)
		}
	}
	// Width x Height
	out, err = exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "x")
		if len(parts) == 2 {
			if w, werr := strconv.ParseUint(parts[0], 10, 32); werr == nil {
				width = uint32(w)
			}
			if h, herr := strconv.ParseUint(parts[1], 10, 32); herr == nil {
				height = uint32(h)
			}
		}
	}
	return seconds, width, height
}

// muxVideoAudio merges a DASH video file with an audio file into a single
// MP4 (stream copy — no re-encode, fast). Returns the new file path; the
// caller owns cleanup of BOTH source files and the merged output.
func muxVideoAudio(ctx context.Context, videoPath, audioPath string) (string, error) {
	if !isFfmpegAvailable() {
		return "", fmt.Errorf("ffmpeg not available")
	}
	out, err := os.CreateTemp("", "goldmux-*.mp4")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	out.Close()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", videoPath, "-i", audioPath,
		"-c", "copy", "-shortest",
		outPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		os.Remove(outPath)
		return "", err
	}
	st, serr := os.Stat(outPath)
	if serr != nil || st.Size() == 0 {
		os.Remove(outPath)
		return "", fmt.Errorf("empty mux output")
	}
	return outPath, nil
}

// probeAudioDuration returns the duration of an audio file in seconds via
// ffprobe, or 0 when unavailable.
func probeAudioDuration(path string) uint32 {
	if !isFfprobeAvailable() {
		return 0
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	if f, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); perr == nil && f > 0 {
		return uint32(f)
	}
	return 0
}

// isWhatsAppVideoReady — ffprobe se check karta hai ke video WhatsApp-playable
// hai: (1) h264 video codec (HEVC/mjpeg WhatsApp error deti hai), (2) moov atom
// front pe (faststart) — nahi to mobile stream fail hota hai, (3) duration > 0.
func isWhatsAppVideoReady(path string) bool {
	if !isFfprobeAvailable() {
		return true // ffprobe nahi hai to assume theek (purana behaviour)
	}
	// 1) codec check
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return false // corrupt file bhi yahan pakda jayega
	}
	codec := strings.TrimSpace(string(out))
	if codec != "h264" {
		return false // hevc / mjpeg / vp9 -> transcode chahiye
	}
	// 2) duration check (corrupt/truncated file)
	out2, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return false
	}
	d, perr := strconv.ParseFloat(strings.TrimSpace(string(out2)), 64)
	if perr != nil || d <= 0 {
		return false
	}
	return true
}

// whatsappifyVideo — video ko WhatsApp-ready banata hai:
//   - h264 + aac + faststart MP4
//   - already-ready file seedha wapas (no re-encode)
//   - non-h264 (HEVC/mjpeg) -> full transcode
//
// Path wapas deta hai (naya temp file jab transcode hua ho). Caller cleanup kare.
func whatsappifyVideo(ctx context.Context, path string) (string, error) {
	if isWhatsAppVideoReady(path) {
		return path, nil
	}
	if !isFfmpegAvailable() {
		return path, nil // ffmpeg nahi hai to as-is bhejo (best effort)
	}
	out := path + ".wa.mp4"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", path,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", out)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(out)
		return path, fmt.Errorf("ffmpeg transcode error: %v, stderr: %s", err, stderr.String())
	}
	if info, err := os.Stat(out); err != nil || info.Size() < 1024 {
		_ = os.Remove(out)
		return path, nil // empty output -> original bhejo
	}
	return out, nil
}
