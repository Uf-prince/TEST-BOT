package goldcmds

// ============================================================================
// GOLD-MD — ToPDF Command
// File: topdf.go
// ============================================================================
// Converts WhatsApp media (image / video / document) into a PDF file.
//
// Command:  .topdf   (alias: pdf)
//
// Flow:
//   1. Download media attached to current or replied-to message.
//   2. Show a GOLD-MD style waiting message (bold caps English, 🔰 emoji).
//   3. Convert to PDF based on media type:
//        - Image (jpg/png/webp/gif) → Pillow → single-page PDF
//        - Video (mp4/webm/gif)     → ffmpeg extract key frames → Pillow PDF
//        - Document (docx/xlsx/pptx/txt/html/odt) → libreoffice headless → PDF
//        - Already PDF             → just re-send as-is
//        - Audio                   → not convertible → error message
//   4. On success: send the PDF document + DELETE the waiting message.
//      On error:   send error message + DELETE the waiting message.
//
// Conversion uses external helpers: python3+Pillow (images/frames),
// libreoffice (office docs), ffmpeg (video frames). Temp files cleaned
// with defer.
// ============================================================================

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const topdfHelpText = "*╭─🔰「 TOPDF 」──⊷*\n" +
	"*┃* 🔰 CONVERT MEDIA TO PDF\n" +
	"*┃*\n" +
	"*┃* 🔰 REPLY TO OR ATTACH:\n" +
	"*┃*   • IMAGE → SINGLE PDF\n" +
	"*┃*   • VIDEO → FRAMES PDF\n" +
	"*┃*   • DOCUMENT → PDF\n" +
	"*╰───────────────⊷*\n\n" +
	"*USAGE:* _REPLY TO MEDIA + .TOPDF_"

// ──────────────────────────────────────────────────────────────────────────
//  .topdf  — main handler
// ──────────────────────────────────────────────────────────────────────────

func handleToPDF(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleToPDFAsync(s, info, args, prefix)
}

func handleToPDFAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// 1 ── Download the media (image/video/audio/document/sticker/quoted)
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, topdfHelpText)
		return
	}

	// 2 ── Show waiting message (GOLD-MD bold caps English style + 🔰)
	waitID := s.ReplyWithID(info, "*╭─🔰「 TOPDF 」──⊷*\n*┃* 🔰 CONVERTING TO PDF...\n*┃* 🔰 PLEASE WAIT...\n*╰───────────────⊷*")

	// 3 ── Convert to PDF
	pdfPath, pdfName, err := convertToPDF(data, mime)
	if err != nil {
		_ = s.DeleteMessage(info, waitID)
		s.Reply(info, "*╭─🔰「 TOPDF 」──⊷*\n*┃* 🔰 ❌ TOPDF FAILED\n*┃* 🔰 "+strings.ToUpper(err.Error())+"\n*╰───────────────⊷*")
		return
	}
	defer removeTempFile(pdfPath)

	// 4 ── Read the PDF and send it
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		_ = s.DeleteMessage(info, waitID)
		s.Reply(info, "*╭─🔰「 TOPDF 」──⊷*\n*┃* 🔰 ❌ FAILED TO READ PDF OUTPUT\n*╰───────────────⊷*")
		return
	}

	if err := s.SendDocument(info, pdfBytes, pdfName, "application/pdf",
		"*╭─🔰「 TOPDF 」──⊷*\n*┃* 🔰 ✅ TOPDF CONVERTED SUCCESSFULLY\n*┃* 🔰 FILE: "+strings.ToUpper(pdfName)+"\n*╰───────────────⊷*"); err != nil {
		_ = s.DeleteMessage(info, waitID)
		s.Reply(info, "*╭─🔰「 TOPDF 」──⊷*\n*┃* 🔰 ❌ FAILED TO SEND PDF\n*╰───────────────⊷*")
		return
	}

	// 5 ── Delete the waiting message on success
	_ = s.DeleteMessage(info, waitID)
}

// ──────────────────────────────────────────────────────────────────────────
//  convertToPDF — dispatches by mimetype, returns pdf path + filename
// ──────────────────────────────────────────────────────────────────────────

func convertToPDF(data []byte, mime string) (string, string, error) {
	m := strings.ToLower(mime)

	// Already a PDF — just write and return
	if strings.Contains(m, "pdf") {
		return writeTempPDF(data)
	}

	// Image → Pillow PDF
	if isImageMime(m) {
		return imageToPDF(data, m)
	}

	// Sticker (webp) → treat as image
	if strings.Contains(m, "webp") || strings.Contains(m, "sticker") {
		return imageToPDF(data, m)
	}

	// Video → ffmpeg frames → Pillow PDF
	if isVideoMime(m) {
		return videoToPDF(data, m)
	}

	// Audio → can't convert to PDF meaningfully
	if strings.Contains(m, "audio") {
		return "", "", fmt.Errorf("audio cannot be converted to PDF")
	}

	// Document (office/text/html) → libreoffice
	return docToPDF(data, m)
}

// ──────────────────────────────────────────────────────────────────────────
//  Helpers — type detection
// ──────────────────────────────────────────────────────────────────────────

func isImageMime(m string) bool {
	return strings.Contains(m, "image") || strings.Contains(m, "jpeg") ||
		strings.Contains(m, "jpg") || strings.Contains(m, "png") || strings.Contains(m, "gif")
}

func isDocMime(m string) bool {
	return strings.Contains(m, "document") || strings.Contains(m, "officedocument") ||
		strings.Contains(m, "msword") || strings.Contains(m, "text") ||
		strings.Contains(m, "html") || strings.Contains(m, "opendocument") ||
		strings.Contains(m, "rtf") || strings.Contains(m, "csv") ||
		strings.Contains(m, "json") || strings.Contains(m, "xml") ||
		strings.Contains(m, "octet-stream")
}

func extForMimePDF(m string) string {
	switch {
	case strings.Contains(m, "jpeg"), strings.Contains(m, "jpg"):
		return ".jpg"
	case strings.Contains(m, "png"):
		return ".png"
	case strings.Contains(m, "webp"):
		return ".webp"
	case strings.Contains(m, "gif"):
		return ".gif"
	case strings.Contains(m, "mp4"):
		return ".mp4"
	case strings.Contains(m, "webm"):
		return ".webm"
	case strings.Contains(m, "pdf"):
		return ".pdf"
	case strings.Contains(m, "officedocument.wordprocessing"), strings.Contains(m, "msword"):
		return ".docx"
	case strings.Contains(m, "officedocument.spreadsheet"):
		return ".xlsx"
	case strings.Contains(m, "officedocument.presentation"):
		return ".pptx"
	case strings.Contains(m, "opendocument.text"):
		return ".odt"
	case strings.Contains(m, "html"):
		return ".html"
	case strings.Contains(m, "text/plain"):
		return ".txt"
	case strings.Contains(m, "rtf"):
		return ".rtf"
	default:
		return ".bin"
	}
}

// ──────────────────────────────────────────────────────────────────────────
//  Python script for image → PDF (passed via -c, args via sys.argv)
// ──────────────────────────────────────────────────────────────────────────

const imageToPDFScript = `import sys
from PIL import Image
img = Image.open(sys.argv[1])
if img.mode in ("RGBA", "P", "LA"):
    bg = Image.new("RGB", img.size, (255, 255, 255))
    if img.mode == "P":
        img = img.convert("RGBA")
    bg.paste(img, mask=img.split()[-1] if "A" in img.mode else None)
    img = bg
elif img.mode != "RGB":
    img = img.convert("RGB")
img.save(sys.argv[2], "PDF", resolution=150.0)
`

// framesToPDFScript takes frame paths in sys.argv[1..n-1], output in sys.argv[n]
const framesToPDFScript = `import sys
from PIL import Image
out = sys.argv[-1]
paths = sys.argv[1:-1]
imgs = []
for p in paths:
    try:
        img = Image.open(p)
        if img.mode in ("RGBA","P","LA"):
            bg = Image.new("RGB", img.size, (255,255,255))
            if img.mode == "P": img = img.convert("RGBA")
            bg.paste(img, mask=img.split()[-1] if "A" in img.mode else None)
            img = bg
        elif img.mode != "RGB":
            img = img.convert("RGB")
        imgs.append(img)
    except Exception:
        continue
if not imgs:
    sys.exit(1)
imgs[0].save(out, "PDF", save_all=True, append_images=imgs[1:], resolution=150.0)
`

// ──────────────────────────────────────────────────────────────────────────
//  Image → PDF  (via Python + Pillow)
// ──────────────────────────────────────────────────────────────────────────

func imageToPDF(data []byte, mime string) (string, string, error) {
	ext := extForMimePDF(mime)
	inPath, err := writeTempMedia(data, ext)
	if err != nil {
		return "", "", fmt.Errorf("write image: %v", err)
	}
	defer removeTempFile(inPath)

	outPath := inPath + ".topdf.pdf"
	cmd := exec.Command("python3", "-c", imageToPDFScript, inPath, outPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		removeTempFile(outPath)
		return "", "", fmt.Errorf("image to pdf: %v | %s", err, stderr.String())
	}
	return outPath, "image.pdf", nil
}

// ──────────────────────────────────────────────────────────────────────────
//  Video → PDF  (ffmpeg extract frames → Pillow multi-page PDF)
// ──────────────────────────────────────────────────────────────────────────

func videoToPDF(data []byte, mime string) (string, string, error) {
	ext := extForMimePDF(mime)
	inPath, err := writeTempMedia(data, ext)
	if err != nil {
		return "", "", fmt.Errorf("write video: %v", err)
	}
	defer removeTempFile(inPath)

	// Create a temp dir for frames
	framesDir, err := os.MkdirTemp("", "topdf-frames-*")
	if err != nil {
		return "", "", fmt.Errorf("mkdir frames: %v", err)
	}
	defer os.RemoveAll(framesDir)

	// Extract up to 10 key frames at ~1fps (capped to avoid huge PDFs)
	framePattern := filepath.Join(framesDir, "frame_%03d.png")
	ffcmd := exec.Command("ffmpeg", "-y", "-i", inPath,
		"-vf", "fps=1,scale=1280:-1",
		"-frames:v", "10",
		"-q:v", "3",
		framePattern)
	var fstderr strings.Builder
	ffcmd.Stderr = &fstderr
	if err := ffcmd.Run(); err != nil {
		return "", "", fmt.Errorf("ffmpeg extract: %v | %s", err, fstderr.String())
	}

	// Collect frame files
	frames, err := filepath.Glob(filepath.Join(framesDir, "frame_*.png"))
	if err != nil || len(frames) == 0 {
		return "", "", fmt.Errorf("no frames extracted from video")
	}

	// Build multi-page PDF with Pillow
	outPath := inPath + ".topdf.pdf"
	args := []string{"-c", framesToPDFScript}
	args = append(args, frames...)
	args = append(args, outPath)

	cmd := exec.Command("python3", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		removeTempFile(outPath)
		return "", "", fmt.Errorf("frames to pdf: %v | %s", err, stderr.String())
	}
	return outPath, "video.pdf", nil
}

// ──────────────────────────────────────────────────────────────────────────
//  Document → PDF  (libreoffice headless)
// ──────────────────────────────────────────────────────────────────────────

func docToPDF(data []byte, mime string) (string, string, error) {
	ext := extForMimePDF(mime)
	inPath, err := writeTempMedia(data, ext)
	if err != nil {
		return "", "", fmt.Errorf("write doc: %v", err)
	}
	defer removeTempFile(inPath)

	// libreoffice headless converts to PDF in the same dir
	outDir := filepath.Dir(inPath)
	loCmd := exec.Command("libreoffice", "--headless", "--convert-to", "pdf",
		"--outdir", outDir, inPath)
	var stderr strings.Builder
	loCmd.Stderr = &stderr
	if err := loCmd.Run(); err != nil {
		return "", "", fmt.Errorf("libreoffice convert: %v | %s", err, stderr.String())
	}

	// libreoffice produces <basename>.pdf (original ext replaced by .pdf)
	base := strings.TrimSuffix(filepath.Base(inPath), filepath.Ext(inPath))
	pdfPath := filepath.Join(outDir, base+".pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		return "", "", fmt.Errorf("libreoffice produced no pdf output")
	}
	return pdfPath, base + ".pdf", nil
}

// ──────────────────────────────────────────────────────────────────────────
//  Already-PDF helper
// ──────────────────────────────────────────────────────────────────────────

func writeTempPDF(data []byte) (string, string, error) {
	f, err := os.CreateTemp("", "topdf-*.pdf")
	if err != nil {
		return "", "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", "", err
	}
	_ = f.Close()
	return f.Name(), "document.pdf", nil
}

// ──────────────────────────────────────────────────────────────────────────
//  Registration
// ──────────────────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "topdf", Category: "AI & MEDIA", Desc: "Convert media (image/video/document) to PDF", Run: handleToPDF})
	Register(Command{Name: "pdf", Hidden: true, Run: handleToPDF})
	Register(Command{Name: "topdf2", Hidden: true, Run: handleToPDF})
}
