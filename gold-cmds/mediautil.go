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

// isFfmpegAvailable reports whether the ffmpeg binary is on PATH.
func isFfmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// isFfprobeAvailable reports whether the ffprobe binary is on PATH.
func isFfprobeAvailable() bool {
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
