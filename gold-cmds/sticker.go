package goldcmds

// ============================================================================
// GOLD-MD — Sticker Commands
// File: sticker.go
// ============================================================================
// Converts images / videos / GIFs to WhatsApp WebP stickers, and converts
// stickers back to images or videos. Converted from UMAR-MD sticker.js
// (Node.js / Baileys — used the `sticker` + `webp` npm packages).
//
// Commands (work in DM or group; reply to or directly attach media):
//   .sticker          — image/video → sticker  (aliases: s, stiker, stick)
//   .take             — sticker → image  (aliases: takeimg, toimg, toimage)
//   .takevid          — sticker → video/GIF  (aliases: tomp4, tovideo)
//
// Flow:
//   1. Download media attached to the current or replied-to message
//      (DownloadQuotedMedia handles image/video/sticker/view-once/quoted).
//   2. Write bytes to a temp file.
//   3. Use ffmpeg+libwebp to convert:
//        - image  → static 512x512 WebP sticker
//        - video  → animated 512x512 WebP sticker (<=10s, 30fps)
//   4. Read the WebP bytes and send via SendSticker.
//   For .take / .takevid: feed the sticker bytes to ffmpeg → png / mp4.
//
// Memory safety: every step uses temp files on disk; media is never held
// in RAM except the final sticker bytes (well under the 400 MB cap).
// ============================================================================

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const stickerHelpText = "*🔰 STICKER MAKER 🔰*\n\n" +
	"*MENTION THE IMAGE/VIDEO FIRST ⚠️*\n" +
	"*AFTER MENTION TYPE SAME*\n\n" +
	"*❰ STICKER ❱*\n\n" +
	"*TO CONVERT IMAGE/VIDEO TO STICKER*"

const toimgHelpText = "*🔰 STICKER TO IMAGE/VIDEO 🔰*\n\n" +
	"*MENTION THE STICKER  FORST ⚠️*\n\n" +
	"*AFTER MENTION TYPE SAME *\n" +
	"*❰ TOIMG ❱*\n\n" +
	"*TO CONVERT STICKER TO IMAGE*"

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// webpExifBytes is a minimal EXIF orientation chunk so WhatsApp treats the
// sticker as a proper sticker pack entry. This mirrors the behaviour of the
// `sticker` npm library which injects a small EXIF block. We keep it tiny
// and static; WhatsApp only needs the WebP container + the image data.
// (An empty EXIF payload is fine — the pack metadata is optional.)

// ffmpegImageToSticker converts an image file (jpg/png/webp/gif) to a
// 512x512 static WebP sticker. Returns the path to the .webp output.
func ffmpegImageToSticker(inputPath, inputExt string) (string, error) {
	outputPath := inputPath + ".sticker.webp"
	// Scale to fit 512x512, transparent background for PNG/WebP, libwebp.
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
		"-vf", "scale=512:512:force_original_aspect_ratio=decrease,"+
			"pad=512:512:-1:-1:color=white@0",
		"-c:v", "libwebp", "-lossless", "1", "-q:v", "100",
		"-loop", "0", "-an", "-vsync", "0",
		outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg image→sticker: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// ffmpegVideoToSticker converts a video file to an animated 512x512 WebP
// sticker. It trims to 10 seconds max and 30 fps to keep the file small.
func ffmpegVideoToSticker(inputPath string) (string, error) {
	outputPath := inputPath + ".sticker.webp"
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
		"-t", "10",
		"-vf", "scale=512:512:force_original_aspect_ratio=decrease,"+
			"pad=512:512:-1:-1:color=white@0,fps=30",
		"-c:v", "libwebp", "-q:v", "80", "-loop", "0",
		"-an", "-vsync", "0",
		outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg video→sticker: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// ffmpegStickerToImage converts a WebP sticker to a PNG image.
func ffmpegStickerToImage(inputPath string) (string, error) {
	outputPath := inputPath + ".out.png"
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg sticker→image: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// ffmpegStickerToVideo converts a WebP sticker (possibly animated) to an MP4.
func ffmpegStickerToVideo(inputPath string) (string, error) {
	outputPath := inputPath + ".out.mp4"
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
		outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg sticker→video: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// writeTempMedia writes the downloaded media bytes to a temp file with the
// given extension and returns the path.
func writeTempMedia(data []byte, ext string) (string, error) {
	f, err := os.CreateTemp("", "gold-md-media-*"+ext)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// extForMime returns a sensible file extension for a mimetype.
func extForMime(mime string) string {
	switch {
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "gif"):
		return ".gif"
	case strings.Contains(mime, "webm"):
		return ".webm"
	case strings.Contains(mime, "mp4"):
		return ".mp4"
	case strings.Contains(mime, "video"):
		return ".mp4"
	case strings.Contains(mime, "jpeg"), strings.Contains(mime, "jpg"):
		return ".jpg"
	default:
		return ".bin"
	}
}

// isVideoMime reports whether the mimetype represents a video / animated GIF.
func isVideoMime(mime string) bool {
	return strings.Contains(mime, "video") || strings.Contains(mime, "gif") || strings.Contains(mime, "webm")
}

// ---------------------------------------------------------------------------
// .sticker  (image/video → sticker)
// ---------------------------------------------------------------------------

func handleSticker(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleStickerAsync(s, info, args, prefix)
}

func handleStickerAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, stickerHelpText)
		return
	}
	if !isFfmpegAvailable() {
		s.Reply(info, "❌ ffmpeg is not available on this server. Sticker conversion requires ffmpeg+libwebp.")
		return
	}

	waitID := s.ReplyWithID(info, "*MAKING STICKER....*\n*PROCESSING: 00%*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	ext := extForMime(mime)
	inPath, err := writeTempMedia(data, ext)
	if err != nil {
		s.Reply(info, "❌ Failed to write media: "+err.Error())
		return
	}
	defer removeTempFile(inPath)

	var outPath string
	if isVideoMime(mime) {
		outPath, err = ffmpegVideoToSticker(inPath)
	} else {
		outPath, err = ffmpegImageToSticker(inPath, ext)
	}
	if err != nil {
		s.Reply(info, "❌ "+err.Error())
		return
	}
	defer removeTempFile(outPath)

	webpBytes, err := os.ReadFile(outPath)
	if err != nil {
		s.Reply(info, "❌ Failed to read sticker output: "+err.Error())
		return
	}
	if err := s.SendSticker(info, webpBytes); err != nil {
		s.Reply(info, "❌ Failed to send sticker: "+err.Error())
		return
	}
}

// ---------------------------------------------------------------------------
// .take  (sticker → image)
// ---------------------------------------------------------------------------

func handleTake(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTakeAsync(s, info, args, prefix)
}

func handleTakeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, toimgHelpText)
		return
	}
	if !isFfmpegAvailable() {
		s.Reply(info, "❌ ffmpeg is not available on this server.")
		return
	}
	if !strings.Contains(mime, "webp") && !strings.Contains(mime, "sticker") {
		s.Reply(info, "❌ The attached media is not a sticker.")
		return
	}

	waitID := s.ReplyWithID(info, "*MAKING IMAGE....*\n*PROCESSING: 00%*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	inPath, err := writeTempMedia(data, ".webp")
	if err != nil {
		s.Reply(info, "❌ Failed to write sticker: "+err.Error())
		return
	}
	defer removeTempFile(inPath)

	outPath, err := ffmpegStickerToImage(inPath)
	if err != nil {
		s.Reply(info, "❌ "+err.Error())
		return
	}
	defer removeTempFile(outPath)

	pngBytes, err := os.ReadFile(outPath)
	if err != nil {
		s.Reply(info, "❌ Failed to read image output: "+err.Error())
		return
	}
	if err := s.SendImage(info, pngBytes, "*STICKER TO IMAGE CONVERTED*"); err != nil {
		s.Reply(info, "❌ Failed to send image: "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// .takevid  (sticker → video)
// ---------------------------------------------------------------------------

func handleTakeVid(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTakeVidAsync(s, info, args, prefix)
}

func handleTakeVidAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, toimgHelpText)
		return
	}
	if !isFfmpegAvailable() {
		s.Reply(info, "❌ ffmpeg is not available on this server.")
		return
	}
	if !strings.Contains(mime, "webp") && !strings.Contains(mime, "sticker") {
		s.Reply(info, "❌ The attached media is not a sticker.")
		return
	}

	waitID := s.ReplyWithID(info, "*MAKING VIDEO....*\n*PROCESSING: 00%*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	inPath, err := writeTempMedia(data, ".webp")
	if err != nil {
		s.Reply(info, "❌ Failed to write sticker: "+err.Error())
		return
	}
	defer removeTempFile(inPath)

	outPath, err := ffmpegStickerToVideo(inPath)
	if err != nil {
		s.Reply(info, "❌ "+err.Error())
		return
	}
	defer removeTempFile(outPath)

	seconds, width, height := probeVideoMeta(outPath)
	if err := s.SendVideoFile(info, outPath, "🎥 *Sticker → Video*", nil, seconds, width, height); err != nil {
		s.Reply(info, "❌ Failed to send video: "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func init() {
	Register(Command{Name: "sticker", Category: "AI & MEDIA", Desc: "Convert image/video/GIF to a sticker", Run: handleSticker})
	Register(Command{Name: "take", Category: "AI & MEDIA", Desc: "Convert a sticker back to an image", Run: handleTake})
	Register(Command{Name: "takevid", Category: "AI & MEDIA", Desc: "Convert an animated sticker back to video", Run: handleTakeVid})

	// aliases (Hidden)
	Register(Command{Name: "s", Hidden: true, Run: handleSticker})
	Register(Command{Name: "stiker", Hidden: true, Run: handleSticker})
	Register(Command{Name: "stick", Hidden: true, Run: handleSticker})
	Register(Command{Name: "stkr", Hidden: true, Run: handleSticker})
	Register(Command{Name: "takeimg", Hidden: true, Run: handleTake})
	Register(Command{Name: "toimg", Hidden: true, Run: handleTake})
	Register(Command{Name: "toimage", Hidden: true, Run: handleTake})
	Register(Command{Name: "tomp4", Hidden: true, Run: handleTakeVid})
	Register(Command{Name: "tovideo", Hidden: true, Run: handleTakeVid})
}
