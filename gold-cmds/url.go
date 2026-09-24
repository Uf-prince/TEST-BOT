package goldcmds

// ============================================================================
// GOLD-MD — Universal File → Link Command  (.url)
// File: url.go
// ============================================================================
// COMMAND: .url   (reply to / send ANY media)
//   Uploads the replied/sent media to a free, unlimited-bandwidth host and
//   replies with the direct link. Supports EVERYTHING:
//     photo, video, audio, voice note, sticker, document (PDF, ZIP, APK, ...)
//   including view-once variants and quoted media.
//
// HOSTS (tried in order — first success wins):
//   1. catbox.moe   — permanent, unlimited (blocked on some datacenter IPs)
//   2. qu.ax        — free, unlimited, no expiry for normal files
//   3. uguu.se      — free, unlimited (3-hour expiry)
//   4. gofile.io    — free, unlimited, permanent download page
//
// Aliases (all Hidden — do NOT appear in the menu / TOTAL COMMANDS count):
//   tourl, imgtourl, img2url, tolink, geturl, converturl, getlink
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// handleURL is the entry point for the .url command and all its hidden aliases.
func handleURL(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleURLAsync(s, info, args, prefix)
}

func handleURLAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Grab ANY media via the bridge (image/video/audio/sticker/document,
	// including view-once & quoted). Falls back to the image-only path.
	data, mime, ok := s.DownloadQuotedMedia(info)
	if (!ok || len(data) == 0) && mime == "" {
		if img, iok := s.DownloadImage(info); iok && len(img) > 0 {
			data, mime, ok = img, "image/jpeg", true
		}
	}
	if !ok || len(data) == 0 {
		s.Reply(info, "*\U0001F530 CONVERT MEDIA TO LINK \U0001F530*\n\n*REPLY ANY MEDIA TO CONVERT LINK*\n\n*FIRST UPLOAD YOUR PHOTO/VIDEO/AUDIO/FILE ETC....*\n\n*MENTION IT FIRST \u26A0\uFE0F*\n*THEN TYPE SAME*\n*\u276E "+prefix+"URL \u276F*\n\n*TO CONVERT YOUR MEDIA TO URL*")
		return
	}

	waitID := s.ReplyWithID(info, "*GETTING URL PLEASE WAIT....*")

	ext := extForMimeURL(mime)
	fileName := "goldmd" + ext
	link, _, err := uploadAnyHost(data, fileName, mime)

	s.DeleteMessage(info, waitID)
	if err != nil || link == "" {
		msg := "\U0001F530 Upload failed"
		if err != nil {
			msg += ": " + err.Error()
		}
		s.Reply(info, msg)
		return
	}

	label := mediaLabelURL(mime)
	s.Reply(info, fmt.Sprintf(
		"*YOUR \u276E %s \u276F LINK IS HERE*\n\n%s",
		label, link))
}

// ---------------------------------------------------------------------------
// Host chain
// ---------------------------------------------------------------------------

// uploadAnyHost tries each free host in order and returns the first direct link.
func uploadAnyHost(data []byte, fileName, mime string) (link, host string, err error) {
	type attempt struct {
		name string
		fn   func([]byte, string, string) (string, error)
	}
	chain := []attempt{
		{"catbox.moe", uploadCatbox},
		{"qu.ax", uploadQuAx},
		{"uguu.se", uploadUguu},
		{"gofile.io", uploadGofile},
	}

	var lastErr error
	for _, a := range chain {
		l, e := a.fn(data, fileName, mime)
		if e == nil && l != "" {
			return l, a.name, nil
		}
		if e != nil {
			lastErr = e
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all hosts failed")
	}
	return "", "", lastErr
}

// uploadCatbox uploads to catbox.moe (permanent, unlimited). Returns the URL.
func uploadCatbox(data []byte, fileName, mime string) (string, error) {
	body, ctype, err := buildMultipart(map[string]string{"reqtype": "fileupload"}, "fileToUpload", fileName, data)
	if err != nil {
		return "", err
	}
	raw, err := postRaw("https://catbox.moe/user/api.php", ctype, body, 120*time.Second)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(string(raw))
	if strings.HasPrefix(out, "http") {
		return out, nil
	}
	return "", fmt.Errorf("catbox: %s", firstLine(out))
}

// uploadQuAx uploads to qu.ax (free, unlimited). Returns the direct URL.
func uploadQuAx(data []byte, fileName, mime string) (string, error) {
	body, ctype, err := buildMultipart(nil, "file", fileName, data)
	if err != nil {
		return "", err
	}
	raw, err := postRaw("https://qu.ax/upload.php", ctype, body, 180*time.Second)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Success bool `json:"success"`
		Files   []struct {
			URL string `json:"url"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("qu.ax: bad response")
	}
	if !parsed.Success || len(parsed.Files) == 0 || parsed.Files[0].URL == "" {
		return "", fmt.Errorf("qu.ax: upload rejected")
	}
	return parsed.Files[0].URL, nil
}

// uploadUguu uploads to uguu.se (free, unlimited, 3h expiry). Returns the URL.
func uploadUguu(data []byte, fileName, mime string) (string, error) {
	body, ctype, err := buildMultipart(nil, "files[]", fileName, data)
	if err != nil {
		return "", err
	}
	raw, err := postRaw("https://uguu.se/upload?output=text", ctype, body, 180*time.Second)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(string(raw))
	if strings.HasPrefix(out, "http") {
		return out, nil
	}
	return "", fmt.Errorf("uguu: %s", firstLine(out))
}

// uploadGofile uploads to gofile.io (free, unlimited, permanent page).
func uploadGofile(data []byte, fileName, mime string) (string, error) {
	body, ctype, err := buildMultipart(nil, "file", fileName, data)
	if err != nil {
		return "", err
	}
	raw, err := postRaw("https://upload.gofile.io/uploadfile", ctype, body, 180*time.Second)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			DownloadPage string `json:"downloadPage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("gofile: bad response")
	}
	if parsed.Data.DownloadPage == "" {
		return "", fmt.Errorf("gofile: upload rejected")
	}
	return parsed.Data.DownloadPage, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildMultipart builds a multipart/form-data body with optional extra fields
// and one file field. Returns the body bytes and the Content-Type header value.
func buildMultipart(fields map[string]string, fileField, fileName string, data []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	part, err := w.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// postRaw POSTs a pre-built body and returns the raw response bytes.
func postRaw(endpoint, contentType string, body []byte, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return raw, nil
}

// mediaLabelURL returns a human label for the media type the user sent:
// PHOTO / VIDEO / AUDIO / FILE (documents, stickers, everything else).
func mediaLabelURL(mime string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.Contains(m, "image"):
		return "PHOTO"
	case strings.Contains(m, "video"):
		return "VIDEO"
	case strings.Contains(m, "audio"):
		return "AUDIO"
	default:
		return "FILE"
	}
}

// extForMimeURL returns a sensible file extension for a broad range of
// mimetypes (images, video, audio, documents, archives, ...).
func extForMimeURL(mime string) string {
	m := strings.ToLower(mime)
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
	case strings.Contains(m, "3gp"):
		return ".3gp"
	case strings.Contains(m, "webm"):
		return ".webm"
	case strings.Contains(m, "quicktime"), strings.Contains(m, "mov"):
		return ".mov"
	case strings.Contains(m, "mpeg"), strings.Contains(m, "mp3"):
		return ".mp3"
	case strings.Contains(m, "ogg"), strings.Contains(m, "opus"):
		return ".ogg"
	case strings.Contains(m, "wav"):
		return ".wav"
	case strings.Contains(m, "aac"):
		return ".aac"
	case strings.Contains(m, "m4a"):
		return ".m4a"
	case strings.Contains(m, "pdf"):
		return ".pdf"
	case strings.Contains(m, "zip"):
		return ".zip"
	case strings.Contains(m, "rar"):
		return ".rar"
	case strings.Contains(m, "7z"):
		return ".7z"
	case strings.Contains(m, "word"), strings.Contains(m, "msword"):
		return ".doc"
	case strings.Contains(m, "excel"), strings.Contains(m, "spreadsheet"):
		return ".xlsx"
	case strings.Contains(m, "powerpoint"), strings.Contains(m, "presentation"):
		return ".pptx"
	case strings.Contains(m, "text/plain"):
		return ".txt"
	case strings.Contains(m, "apk"), strings.Contains(m, "android"):
		return ".apk"
	case strings.Contains(m, "video"):
		return ".mp4"
	case strings.Contains(m, "audio"):
		return ".mp3"
	case strings.Contains(m, "image"):
		return ".jpg"
	default:
		return ".bin"
	}
}

// directFileRe matches a URL that already points straight at a file.
var directFileRe = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|webp|mp4|3gp|webm|mov|mkv|mp3|ogg|opus|wav|m4a|aac|pdf|zip|rar|7z|apk|txt)(\?|$)`)

// ogMediaRe pulls the direct media URL out of a host's share page — qu.ax and
// gofile return a PAGE link (e.g. https://qu.ax/O7xfZ) whose og:video / og:image
// meta tag holds the real file URL. Without this the page HTML itself gets
// uploaded as if it were the video and WhatsApp refuses to play it.
var ogMediaRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:(?:video|image|audio)["'][^>]+content=["']([^"']+)["']`)

// resolveDirectMediaURL turns a host PAGE url into the direct file URL.
// Returns raw unchanged when it already looks like a file, or when the page
// cannot be parsed (best-effort — callers fall back to the original value).
func resolveDirectMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || directFileRe.MatchString(raw) {
		return raw
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return raw
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", originOfURL(raw)+"/")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return raw
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return raw
	}
	if m := ogMediaRe.FindSubmatch(body); len(m) == 2 {
		cand := strings.TrimSpace(string(m[1]))
		if strings.HasPrefix(cand, "//") {
			cand = "https:" + cand
		} else if strings.HasPrefix(cand, "/") {
			cand = originOfURL(raw) + cand
		}
		if strings.HasPrefix(cand, "http") {
			return cand
		}
	}
	return raw
}

// BrowserUA is the User-Agent required by qu.ax/catbox media endpoints.
const BrowserUA = browserUA

// MediaReferer returns the Referer a host requires on its file URL: qu.ax
// answers a 302 → HTML page when the Referer is missing (hotlink protection),
// which is exactly how a share page ends up being sent as a "video".
func MediaReferer(raw string) string {
	if o := originOfURL(raw); o != "" {
		return o + "/"
	}
	return ""
}

// ResolveDirectMediaURL is the exported wrapper of resolveDirectMediaURL for
// the session layer (src/handlers_extra.go), which must store/stream the real
// file URL and never a host share page.
func ResolveDirectMediaURL(raw string) string { return resolveDirectMediaURL(raw) }

// originOfURL returns scheme://host for an absolute URL ("" when unparsable).
func originOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// browserUA is sent on every media download: qu.ax/catbox refuse the default
// Go user agent and answer with a 302 HTML page instead of the file bytes.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// humanSizeURL formats a byte count as a human-readable size string.
func humanSizeURL(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := int64(n) / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// firstLine returns the first line of s (trimmed) for compact error messages.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func init() {
	// Main command — VISIBLE in the TOOLS category (counted in TOTAL COMMANDS).
	Register(Command{
		Name:     "url",
		Category: "TOOLS",
		Desc:     "THIS COMMAND IS USED TO CONVERT ANY FILE INTO A LINK. REPLY TO A PHOTO, VIDEO, AUDIO, STICKER OR DOCUMENT (PDF ETC.) AND THE BOT GIVES YOU A DIRECT LINK OF IT.",
		Run:      handleURL,
	})

	// Hidden aliases — work exactly the same but do NOT appear in the menu
	// or the TOTAL COMMANDS count.
	for _, alias := range []string{"tourl", "imgtourl", "img2url", "tolink", "geturl", "converturl", "getlink"} {
		Register(Command{Name: alias, Hidden: true, Run: handleURL})
	}
}
