package goldcmds

// ============================================================================
// GOLD-MD — MediaFire Downloader
// File: mediafire.go
// ============================================================================
// COMMANDS: .mdf / .mf / .mediafire / .mfdl / .mdfdl / .mfdownlload
//           (Category: DOWNLOADER)
//
// Public MediaFire share-link se file download karke seedha chat me document
// ke tor pe bhej deta hai. Share-page se asli CDN direct-download link
// resolve karta hai.
//
// USAGE:
//     .mediafire https://www.mediafire.com/file/xxxxxxxxxx/filename/file
//     .mf https://www.mediafire.com/file/xxxxxxxxxx/filename/file
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
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const mfHelpText = "*\U0001f530 MEDIAFIRE DOWNLOADER \U0001f530*\n\n" +
	"**MEDIAFIRE \u276e PASTE LINK HERE \u276f*\n\n" +
	"*EXAMPLE*\n*MEDIAFIRE https://www.mediafire.com/file/xxxxx/filename/file*\n\nTO DOWNLOAD MEDIAFIRE FILES"

// mfWaitText — static waiting message (live-edit progress hata diya).
const mfWaitText = "*DOWNLOADING PLEASE WAIT....*"

var (
	mfSchemeRe = regexp.MustCompile(`(?i)^https?://`)
	mfHostRe   = regexp.MustCompile(`(?i)(^|\.)mediafire\.com$`)
	mfBtnRe1   = regexp.MustCompile(`(?i)id="downloadButton"[^>]*href="([^"]+)"`)
	mfBtnRe2   = regexp.MustCompile(`(?i)href="([^"]+)"[^>]*id="downloadButton"`)
	mfErrRe    = regexp.MustCompile(`(?i)error\.php|Invalid or Deleted File|has been removed|no longer available`)
	mfNameRe1  = regexp.MustCompile(`(?i)class="dl-btn-label"[^>]*title="([^"]+)"`)
	mfNameRe2  = regexp.MustCompile(`(?i)<div class="filename"[^>]*>([^<]+)</div>`)
	mfNameRe3  = regexp.MustCompile(`(?i)<title>([^<]+?)\s*-\s*MediaFire</title>`)
	mfDispRe   = regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";\n]+)"?`)
)

func init() {
	Register(Command{Name: "mdf", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD MEDIAFIRE FILES. JUST SEND A MEDIAFIRE LINK WITH THIS COMMAND.", Run: handleMediafire})
	Register(Command{Name: "mf", Hidden: true, Run: handleMediafire})
	Register(Command{Name: "mediafire", Hidden: true, Run: handleMediafire})
	Register(Command{Name: "mfdl", Hidden: true, Run: handleMediafire})
	Register(Command{Name: "mdfdl", Hidden: true, Run: handleMediafire})
	Register(Command{Name: "mfdownlload", Hidden: true, Run: handleMediafire})
}

// mfNormalizeURL — user ke input se valid mediafire.com/file/... share-URL banao.
func mfNormalizeURL(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if !mfSchemeRe.MatchString(s) {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	if !mfHostRe.MatchString(strings.ToLower(u.Hostname())) {
		return ""
	}
	return u.String()
}

func handleMediafire(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleMediafireAsync(ctx, s, info, args, prefix)
	})
}

func handleMediafireAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	rawInput := strings.TrimSpace(strings.Join(args, " "))
	if rawInput == "" {
		s.Reply(info, mfHelpText)
		return
	}

	shareURL := mfNormalizeURL(rawInput)
	if shareURL == "" {
		s.Reply(info, "*PASTE THE MEDIAFIRE LINK SAME* \n*\u276e MEDIAFIRE https://www.mediafire.com/file/xxxxx/filename/file \u276f*")
		return
	}

	waitID := s.ReplyWithID(info, mfWaitText)

	path, filename, err := mfDownloadToFile(ctx, shareURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		if err.Error() == "MF_FILE_NOT_FOUND" {
			s.Reply(info, "\u274c *FILE NOT FOUND / DELETED*")
			return
		}
		s.Reply(info, "\u274c *TRY AGAIN LATER*")
		return
	}
	defer removeTempFile(path)

	if err := s.SendDocumentFile(info, path, filename, "application/octet-stream", "*MEDIAFIRE FILE DOWNLOADED*"); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\u274c *TRY AGAIN LATER*")
		return
	}
	s.DeleteMessage(info, waitID)
}

// mfDownloadToFile — MediaFire se file download karo, direct-CDN link resolve
// karke. Temp file path + filename deta hai.
func mfDownloadToFile(ctx context.Context, shareURL string) (string, string, error) {
	client := mfHTTPClient()

	directURL, htmlName, err := mfResolveDirectLink(ctx, client, shareURL)
	if err != nil {
		return "", "", err
	}

	resp, err := mfGet(ctx, client, directURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("MF_FILE_HTTP_%d", resp.StatusCode)
	}

	path, size, err := dlStreamBodyToTempFile(ctx, resp.Body, "gold-md-mediafire-*")
	if err != nil {
		return "", "", err
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") && size < 20000 {
		removeTempFile(path)
		return "", "", fmt.Errorf("MF_NOT_A_FILE")
	}
	if size < 1 {
		removeTempFile(path)
		return "", "", fmt.Errorf("MF_EMPTY_FILE")
	}

	// fallback name: direct URL ka last path segment
	fallbackName := "bilal-md-mediafire"
	if u, err := url.Parse(directURL); err == nil {
		parts := strings.Split(u.Path, "/")
		if last := parts[len(parts)-1]; last != "" {
			fallbackName = last
		}
	}
	nameFallback := fallbackName
	if htmlName != "" {
		nameFallback = htmlName
	}
	filename := mfFilenameFromHeaders(resp.Header, nameFallback)
	return path, filename, nil
}

// mfResolveDirectLink — MediaFire share-page se asli CDN direct-download link
// nikaalo. downloadButton hi source-of-truth hai ki file valid hai ya nahi.
func mfResolveDirectLink(ctx context.Context, client *http.Client, shareURL string) (string, string, error) {
	resp, err := mfGet(ctx, client, shareURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("MF_HTTP_%d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	html := string(body)

	var direct string
	if m := mfBtnRe1.FindStringSubmatch(html); len(m) > 1 {
		direct = m[1]
	} else if m := mfBtnRe2.FindStringSubmatch(html); len(m) > 1 {
		direct = m[1]
	}

	if direct == "" {
		// Deleted / expired / not-found MediaFire page detect karo.
		if mfErrRe.MatchString(html) {
			return "", "", fmt.Errorf("MF_FILE_NOT_FOUND")
		}
		return "", "", fmt.Errorf("MF_DIRECT_LINK_NOT_FOUND")
	}

	directURL := strings.ReplaceAll(direct, "&amp;", "&")
	htmlName := mfFilenameFromHTML(html, "")
	return directURL, htmlName, nil
}

// mfFilenameFromHTML — share-page HTML se real filename nikaalo.
func mfFilenameFromHTML(html, fallback string) string {
	for _, re := range []*regexp.Regexp{mfNameRe1, mfNameRe2, mfNameRe3} {
		if m := re.FindStringSubmatch(html); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return fallback
}

// mfFilenameFromHeaders — Content-Disposition header se real filename nikaalo.
func mfFilenameFromHeaders(h http.Header, fallback string) string {
	cd := h.Get("Content-Disposition")
	if cd != "" {
		if m := mfDispRe.FindStringSubmatch(cd); len(m) > 1 && m[1] != "" {
			if dec, err := url.QueryUnescape(m[1]); err == nil {
				return dec
			}
			return m[1]
		}
	}
	return fallback
}

// mfHTTPClient — cookie-jar wala client.
func mfHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 5 * time.Minute,
		Jar:     jar,
	}
}

// mfGet — ek GET request (redirects auto-follow, cookies auto-handle).
// Accept-Encoding: identity — compressed (gzip/br) response se bachne ke liye,
// warna body.toString() garbage banata hai aur regex checks fail ho jaate hain.
func mfGet(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")
	return client.Do(req)
}
