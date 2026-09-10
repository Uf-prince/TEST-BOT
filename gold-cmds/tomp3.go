package goldcmds

// ============================================================================
// GOLD-MD — Video → MP3 Command
// File: tomp3.go
// ============================================================================
// Extracts the audio track from a video (or converts any audio) to an MP3
// file. Converted from UMAR-MD tomp3.js (Node.js / Baileys — used ffmpeg).
//
// Command:
//   .tomp3  (reply to or attach a video / audio)  → sends an MP3 audio message
//   aliases: toaudio, mp3, audio, vtmp3
//
// Flow:
//   1. Download media (video or audio) attached to the current or replied-to
//      message via DownloadQuotedMedia.
//   2. Write bytes to a temp file.
//   3. ffmpeg: extract/encode audio to MP3 (file→file, no RAM buffering).
//   4. Probe duration with ffprobe and send via SendAudioFile.
//
// Memory safety: all processing is file-to-file on disk; nothing is held in
// RAM beyond small control buffers, well within the 400 MB cap.
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ffmpegToMP3 extracts/encodes audio from any media file to MP3.
func ffmpegToMP3(ctx context.Context, inputPath string) (string, error) {
	outputPath := inputPath + ".out.mp3"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath,
		"-vn", "-codec:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2",
		outputPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg mp3 error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

func handleToMP3(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog (0% speed impact — pure goroutine + select).
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleToMP3Async(ctx, s, info, args, prefix)
	})
}

func handleToMP3Async(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	data, mime, ok, tooBig := downloadMediaLimited(s, info)
	if tooBig {
		return // already replied: *❌ FILE TOO BIG — MAX 700MB*
	}
	if !ok || len(data) == 0 {
		s.Reply(info, "*FIRST MENTION THE VIDEO FIRST 🔰*\n*AFTER MENTION TYPE*\n\n*❰ TOMO3 ❱*\n*\n*TO CONVERT VIDEO TO MP3 AUDIO*")
		return
	}
	if !isFfmpegAvailable() {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER
			s.Reply(info, "🔰 *VIDEO TO AUDIO CONVERSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	waitID := s.ReplyWithID(info, "*PROCESSING...*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	ext := extForMime(mime)
	inPath, err := writeTempMedia(data, ext)
	if err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER
			s.Reply(info, "🔰 *VIDEO TO AUDIO CONVERSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	defer removeTempFile(inPath)

	outPath, err := ffmpegToMP3(ctx, inPath)
	if err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER
			s.Reply(info, "🔰 *VIDEO TO AUDIO CONVERSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	defer removeTempFile(outPath)

	seconds := probeAudioDuration(outPath)
	if err := s.SendAudioFile(info, outPath, "", seconds); err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER
			s.Reply(info, "🔰 *VIDEO TO AUDIO CONVERSION FAILED, PLEASE TRY AGAIN*")
		}
	}
}

func init() {
	Register(Command{Name: "tomp3", Category: "DOWNLOADER", Desc: "Convert a video/audio to MP3", Run: handleToMP3})

	// aliases (Hidden)
	Register(Command{Name: "toaudio", Hidden: true, Run: handleToMP3})
	Register(Command{Name: "mp3", Hidden: true, Run: handleToMP3})
	Register(Command{Name: "audio", Hidden: true, Run: handleToMP3})
	Register(Command{Name: "vtmp3", Hidden: true, Run: handleToMP3})
	Register(Command{Name: "v2mp3", Hidden: true, Run: handleToMP3})
}
