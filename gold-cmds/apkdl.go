package goldcmds

// ============================================================================
// GOLD-MD — APK Downloader (Google Play apps)
// File: apkdl.go
// ============================================================================
// HANDLER: handleAPK — used by .apksearch direct-link router
//   Finds an app on APKCombo and sends the APK/XAPK file as a document.
//
// Source (free, permanent, no API key): apkcombo.com
//   Flow (all plain HTTP — no JavaScript needed):
//     1. If input looks like a package name (com.foo.bar):
//          GET https://apkcombo.com/app/<pkg>/download/apk
//          APKCombo 301-redirects to the correct slug automatically.
//        Otherwise:
//          GET https://apkcombo.com/search/<query> and take the first
//          /slug/pkg/ result link.
//     2. The download page contains one of two redirect links:
//          a) href="/r2?u=<urlencoded signed cloudflare-R2 url>"
//             (e.g. WhatsApp)  -> decode the "u" query param directly.
//          b) href="https://apkcombo.com/d?u=<base64 pureapk CDN url>"
//             (e.g. WiFi Analyzer) -> base64-decode the "u" param.
//     3. Stream-download the file (following redirects) and send it as a
//        WhatsApp document.
//
// ============================================================================

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const apkComboBase = "https://apkcombo.com"

const apkUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

const apkHelpText = "*\U0001f530 APK DOWNLOAD COMMAND \U0001f530*\n" +
	"*DO YOU WANT TO DOWNLOAD AN APP APK? \U0001f914*\n" +
	"*JUST WRITE THE APP NAME OR PACKAGE NAME \U0001f60a*\n\n" +
	"*.APK \u2770APP NAME \u276f*\n" +
	"*EXAMPLES.....*\n" +
	"*.APK whatsapp*\n" +
	"*.APK com.whatsapp*\n\n" +
	"*THE APK FILE WILL BE SENT HERE \U0001f917*"

// apkAppInfo holds everything needed to download one app file.
type apkAppInfo struct {
	Slug     string // apkcombo slug (whatsapp)
	Package  string // android package name (com.whatsapp)
	Title    string // display title (WhatsApp)
	Version  string // 2.26.34.81
	Size     string // "144 MB"
	FileURL  string // final direct file url
	FileName string // WhatsApp_2.26.34.81_apkcombo.com.xapk
	MimeType string // apk/xapk mimetype
}

var (
	// package name pattern: com.foo.bar (at least one dot, ascii lower)
	apkPkgRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	// /slug/pkg/ links in search results
	apkSearchRe = regexp.MustCompile(`href="/([a-z0-9-]+)/([a-z0-9._]+)/"`)
	// signed cloudflare-R2 redirect link (relative or absolute)
	apkR2Re = regexp.MustCompile(`href="((?:https?://apkcombo\.com)?/r2\?u=[^"]+)"`)
	// base64 pureapk CDN redirect link (relative or absolute)
	apkDRe = regexp.MustCompile(`href="((?:https?://apkcombo\.com)?/d\?u=[^"]+)"`)
	// version from /download/phone-<ver>-apk|xapk|apks link
	apkVerRe = regexp.MustCompile(`/download/[a-z]+-([0-9][0-9a-zA-Z.\-_]*)-(apk|xapk|apks)`)
	// size like "144 MB" or "89.5 MB"
	apkSizeRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?\s*[KMG]B)`)
	// app title from <h1>...</h1>
	apkTitleRe = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
)

func handleAPK(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleAPKAsync(ctx, s, info, args, prefix)
	})
}

func handleAPKAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, apkHelpText)
		return
	}

	waitID := s.ReplyWithID(info, "*DOWNLOADING APK....*")

	app, err := apkResolve(ctx, query)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *APK NOT FOUND*\n"+err.Error())
		return
	}

	s.EditMessage(info, waitID, fmt.Sprintf("*🔰 DOWNLOADING THIS APK 🔰%s %s (%s)...*", app.Title, app.Version, app.Size))

	client := &http.Client{Timeout: 10 * time.Minute}
	path, err := streamDownloadToFile(ctx, client, app.FileURL, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *APK DOWNLOAD ERROR*\nPlease try again.")
		return
	}
	defer removeTempFile(path)

	name := app.FileName
	if name == "" {
		name = app.Package + "_" + app.Version + ".apk"
	}
	caption := "*🔰 APK DOWNLOADED 🔰*\n" +
		"*" + app.Title + "*\n\n" +
		"*🔰 VERSION :* " + app.Version + "\n" +
		"*🔰 SIZE :* " + app.Size + "\n" +
		"*🔰 PACKAGE :* " + app.Package + "\n\n" +
		"*FOUND FROM GOOGLE PLAY*"

	if err := s.SendDocumentFile(info, path, name, app.MimeType, caption); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *APK SEND ERROR*\nFile could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// apkResolve turns a search query or package name into a downloadable app.
func apkResolve(ctx context.Context, query string) (*apkAppInfo, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}

	var slug, pkg string
	if apkPkgRe.MatchString(strings.ToLower(query)) {
		pkg = strings.ToLower(query)
		slug = "app" // placeholder — apkcombo 301s to the right slug
	} else {
		// search by app name and take the first /slug/pkg/ hit
		searchURL := apkComboBase + "/search/" + url.PathEscape(query)
		body, _, err := apkFetchPage(ctx, client, searchURL)
		if err != nil {
			return nil, fmt.Errorf("search failed: %v", err)
		}
		for _, m := range apkSearchRe.FindAllStringSubmatch(body, -1) {
			if len(m) >= 3 && !strings.Contains(m[1], "category") && !strings.Contains(m[1], "topic") && !strings.Contains(m[1], "search") {
				slug, pkg = m[1], m[2]
				break
			}
		}
		if pkg == "" {
			return nil, fmt.Errorf("no app matched %q", query)
		}
	}

	dlURL := fmt.Sprintf("%s/%s/%s/download/apk", apkComboBase, slug, pkg)
	body, finalURL, err := apkFetchPage(ctx, client, dlURL)
	if err != nil {
		return nil, fmt.Errorf("download page failed: %v", err)
	}

	app := &apkAppInfo{Slug: slug, Package: pkg}

	// title
	if m := apkTitleRe.FindStringSubmatch(body); len(m) > 1 {
		app.Title = apkStripTags(m[1])
	}
	if app.Title == "" {
		app.Title = pkg
	}

	// version + variant from the version links
	if m := apkVerRe.FindStringSubmatch(body); len(m) > 2 {
		app.Version = m[1]
	}
	if app.Version == "" {
		app.Version = "latest"
	}

	// size (first match on the page, appears near the main download button)
	if m := apkSizeRe.FindStringSubmatch(body); len(m) > 1 {
		app.Size = strings.TrimSpace(m[1])
	}
	if app.Size == "" {
		app.Size = "unknown"
	}

	// direct file url — try R2 (cloudflare, preferred) then D (pureapk CDN)
	if m := apkR2Re.FindStringSubmatch(body); len(m) > 1 {
		if fileURL, err := apkDecodeR2Link(m[1]); err == nil {
			app.FileURL = fileURL
		}
	}
	if app.FileURL == "" {
		if m := apkDRe.FindStringSubmatch(body); len(m) > 1 {
			if fileURL, err := apkDecodeDLink(m[1]); err == nil {
				app.FileURL = fileURL
			}
		}
	}
	if app.FileURL == "" {
		return nil, fmt.Errorf("download link not found for %s", pkg)
	}

	// filename + mimetype
	app.FileName = apkFilenameFromURL(app.FileURL, pkg, app.Version)
	app.MimeType = apkMimeForName(app.FileName)

	// if the final URL contains the real slug, remember it (cosmetic)
	if m := regexp.MustCompile(`/([a-z0-9-]+)/` + regexp.QuoteMeta(pkg) + `/download/`).FindStringSubmatch(finalURL); len(m) > 1 {
		app.Slug = m[1]
	}
	return app, nil
}

// apkDecodeR2Link decodes "/r2?u=<urlencoded signed url>" into the direct
// cloudflare-R2 file url.
func apkDecodeR2Link(link string) (string, error) {
	link = strings.ReplaceAll(link, "&amp;", "&")
	if !strings.HasPrefix(link, "http") {
		link = apkComboBase + link
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	fileURL := u.Query().Get("u")
	if fileURL == "" {
		return "", fmt.Errorf("empty r2 param")
	}
	return fileURL, nil
}

// apkDecodeDLink decodes "https://apkcombo.com/d?u=<base64 url>" into the
// direct pureapk CDN file url.
func apkDecodeDLink(link string) (string, error) {
	link = strings.ReplaceAll(link, "&amp;", "&")
	if !strings.HasPrefix(link, "http") {
		link = apkComboBase + link
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	b64 := u.Query().Get("u")
	if b64 == "" {
		return "", fmt.Errorf("empty d param")
	}
	// "+" inside a query string is parsed as space — restore it
	b64 = strings.ReplaceAll(b64, " ", "+")
	// ensure padding
	if m := len(b64) % 4; m != 0 {
		b64 += strings.Repeat("=", 4-m)
	}
	// try standard, then url-safe, then hybrid
	if dec, err := base64.StdEncoding.DecodeString(b64); err == nil {
		return string(dec), nil
	}
	if dec, err := base64.URLEncoding.DecodeString(b64); err == nil {
		return string(dec), nil
	}
	fixed := strings.NewReplacer("-", "+", "_", "/").Replace(b64)
	if dec, err := base64.StdEncoding.DecodeString(fixed); err == nil {
		return string(dec), nil
	}
	return "", fmt.Errorf("base64 decode failed")
}

// apkFetchPage GETs a page and returns (body, finalURL, error).
func apkFetchPage(ctx context.Context, client *http.Client, pageURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", apkUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	res, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	return string(data), res.Request.URL.String(), nil
}

// apkFilenameFromURL decodes the filename embedded in a signed/direct url
// (response-content-disposition), falling back to pkg_version.ext.
func apkFilenameFromURL(fileURL, pkg, version string) string {
	if u, err := url.Parse(fileURL); err == nil {
		if disp := u.Query().Get("response-content-disposition"); disp != "" {
			for _, part := range strings.Split(disp, ";") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "filename=") {
					name := strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
					name = strings.ReplaceAll(name, "%22", "")
					if name != "" {
						return name
					}
				}
			}
		}
		// pureapk style: /b/APK/<base64 pkg_ver_hash>
		if m := regexp.MustCompile(`/b/(?:X?APK|APKS)?/?([A-Za-z0-9+/=_-]+)$`).FindStringSubmatch(u.Path); len(m) > 1 {
			if dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(m[1], "-", "+") + "=="); err == nil {
				name := string(dec)
				if strings.Contains(name, "_") {
					return name + apkExtFromURL(fileURL)
				}
			}
		}
	}
	ext := apkExtFromURL(fileURL)
	return pkg + "_" + version + ext
}

// apkExtFromURL picks the archive extension from a url.
func apkExtFromURL(fileURL string) string {
	low := strings.ToLower(fileURL)
	switch {
	case strings.Contains(low, ".apks") || strings.Contains(low, "xapk-package"):
		return ".xapk"
	case strings.Contains(low, ".xapk"):
		return ".xapk"
	default:
		return ".apk"
	}
}

// apkMimeForName maps a file extension to its android package mimetype.
func apkMimeForName(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.HasSuffix(low, ".xapk"):
		return "application/xapk-package-archive"
	case strings.HasSuffix(low, ".apks"):
		return "application/apks"
	default:
		return "application/vnd.android.package-archive"
	}
}

// apkStripTags removes html tags and collapses whitespace.
func apkStripTags(s string) string {
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
