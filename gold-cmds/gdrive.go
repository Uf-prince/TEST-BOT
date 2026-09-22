package goldcmds

// ============================================================================
// GOLD-MD — Google Drive Downloader
// File: gdrive.go
// ============================================================================
// COMMANDS: .gdrive / .gdl / .drive   (Category: DOWNLOADER)
//
// Public Google Drive share-link (ya seedha FILE_ID) se file download karke
// seedha chat me document ke tor pe bhej deta hai. Bade files (jinpe Google
// "can't scan for viruses" warning deta hai) ke liye confirm-token khud
// handle karta hai.
//
// USAGE:
//     .gdrive https://drive.google.com/file/d/FILE_ID/view?usp=sharing
//     .gdrive FILE_ID
//
// OWNER ORDER (2026):
//   - Text same-to-same (Node source se), sirf 👑 -> 🔰.
//   - Live-editing progress function HATA diya — download ke doran ek
//     simple static message aata hai: *DOWNLOADING PLEASE WAIT....*
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const gdriveHelpText = "*\U0001f530 GOOGLE DRIVE DOWNLOADER \U0001f530*\n\n" +
	"*PASTE THE GOOGLE DRIVE FILE LINK*\n*OR PASTE THE GOOGLE DRIVE FILE_ID*\n" +
	"*GDRIVE \u276e PASTE LINK HERE\u276f*\n*GDRIVE \u276e FILE_ID \u276f*\n\n" +
	"*PASTE ANY LINK OR FIL_ID ANY ONE TO DOWNLOAD GOOGLE DRIVE FILES*"

// gdriveWaitText — static waiting message (live-edit progress hata diya).
const gdriveWaitText = "*DOWNLOADING PLEASE WAIT....*"

var gdriveFileIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/d/([a-zA-Z0-9_-]{15,})`),     // .../file/d/FILE_ID/view
	regexp.MustCompile(`[?&]id=([a-zA-Z0-9_-]{15,})`), // ...?id=FILE_ID
	regexp.MustCompile(`^([a-zA-Z0-9_-]{15,})$`),      // raw ID typed directly
}

var (
	gdriveConfirmRe = regexp.MustCompile(`confirm=([0-9A-Za-z_-]+)`)
	gdriveUUIDRe    = regexp.MustCompile(`name="uuid" value="([0-9A-Za-z_-]+)"`)
	gdriveDispRe    = regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";\n]+)"?`)
)

func init() {
	Register(Command{Name: "gdrive", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD GOOGLE DRIVE FILES. JUST SEND A GOOGLE DRIVE LINK OR FILE ID WITH THIS COMMAND.", Run: handleGdrive})
	Register(Command{Name: "gdl", Hidden: true, Run: handleGdrive})
	Register(Command{Name: "drive", Hidden: true, Run: handleGdrive})
}

// gdriveExtractFileID — har tarah ke Drive link/ID se seedha FILE_ID nikaalo.
func gdriveExtractFileID(input string) string {
	s := strings.TrimSpace(input)
	for _, re := range gdriveFileIDPatterns {
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func handleGdrive(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleGdriveAsync(ctx, s, info, args, prefix)
	})
}

func handleGdriveAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	input := strings.TrimSpace(strings.Join(args, " "))
	if input == "" {
		s.Reply(info, gdriveHelpText)
		return
	}

	fileID := gdriveExtractFileID(input)
	if fileID == "" {
		s.Reply(info, "\u274c *VALID GOOGLE DRIVE LINK YA FILE ID BHEJO*")
		return
	}

	waitID := s.ReplyWithID(info, gdriveWaitText)

	path, filename, err := gdriveDownloadToFile(ctx, fileID)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "*TRY AGAIN LATER*")
		return
	}
	defer removeTempFile(path)

	if err := s.SendDocumentFile(info, path, filename, "application/octet-stream", "*GDRIVE FILE DOWNLOADED*"); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "*TRY AGAIN LATER*")
		return
	}
	s.DeleteMessage(info, waitID)
}

// gdriveDownloadToFile — Google Drive se file download karo, confirm-token
// (badi file warning) khud handle karta hai. Temp file path + filename deta hai.
func gdriveDownloadToFile(ctx context.Context, fileID string) (string, string, error) {
	client := gdriveHTTPClient()
	baseURL := "https://drive.google.com/uc?export=download&id=" + fileID

	resp, err := gdriveGet(ctx, client, baseURL)
	if err != nil {
		return "", "", err
	}

	// Agar HTML confirm-page aaya (badi file warning) to confirm-token nikaal
	// ke dobara download karo.
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		html := string(body)

		confirm := ""
		if m := gdriveConfirmRe.FindStringSubmatch(html); len(m) > 1 {
			confirm = m[1]
		}
		uuid := ""
		if m := gdriveUUIDRe.FindStringSubmatch(html); len(m) > 1 {
			uuid = m[1]
		}
		if confirm == "" {
			confirm = "t" // fallback token jo aksar chal jata hai
		}

		confirmURL := "https://drive.usercontent.google.com/download?id=" + fileID +
			"&export=download&confirm=" + confirm
		if uuid != "" {
			confirmURL += "&uuid=" + uuid
		}

		resp, err = gdriveGet(ctx, client, confirmURL)
		if err != nil {
			return "", "", err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GDRIVE_HTTP_%d", resp.StatusCode)
	}

	path, size, err := dlStreamBodyToTempFile(ctx, resp.Body, "gold-md-gdrive-*")
	if err != nil {
		return "", "", err
	}

	// Abhi bhi HTML aaya matlab file private hai ya exist nahi karti.
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") && size < 20000 {
		removeTempFile(path)
		return "", "", fmt.Errorf("GDRIVE_NOT_PUBLIC_OR_INVALID")
	}
	if size < 1 {
		removeTempFile(path)
		return "", "", fmt.Errorf("GDRIVE_EMPTY_FILE")
	}

	filename := gdriveFilenameFromHeaders(resp.Header, "bilal-md-"+fileID)
	return path, filename, nil
}

// gdriveHTTPClient — cookie-jar wala client (confirm step ke cookies ke liye).
func gdriveHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 5 * time.Minute,
		Jar:     jar,
	}
}

// gdriveGet — ek GET request (redirects auto-follow, cookies auto-handle).
func gdriveGet(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return client.Do(req)
}

// gdriveFilenameFromHeaders — Content-Disposition header se real filename nikaalo.
func gdriveFilenameFromHeaders(h http.Header, fallback string) string {
	cd := h.Get("Content-Disposition")
	if cd != "" {
		if m := gdriveDispRe.FindStringSubmatch(cd); len(m) > 1 && m[1] != "" {
			if dec, err := url.QueryUnescape(m[1]); err == nil {
				return dec
			}
			return m[1]
		}
	}
	return fallback
}

// dlStreamBodyToTempFile — response body ko temp file me stream karo (RAM-safe).
// Shared helper: gdrive.go + mediafire.go dono use karte hain.
func dlStreamBodyToTempFile(ctx context.Context, body io.Reader, pattern string) (string, int64, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", 0, err
	}
	path := file.Name()

	var written int64
	buf := make([]byte, 64*1024)
	for {
		if cerr := ctx.Err(); cerr != nil {
			file.Close()
			removeTempFile(path)
			return "", 0, cerr
		}
		n, readErr := body.Read(buf)
		if n > 0 {
			w, writeErr := file.Write(buf[:n])
			if writeErr != nil || w != n {
				file.Close()
				removeTempFile(path)
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				return "", 0, writeErr
			}
			written += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			file.Close()
			removeTempFile(path)
			return "", 0, readErr
		}
	}
	if err := file.Close(); err != nil {
		removeTempFile(path)
		return "", 0, err
	}
	return path, written, nil
}
