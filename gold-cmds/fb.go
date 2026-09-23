package goldcmds

// ============================================================================
// GOLD-MD — Facebook + Instagram Video Downloader (free, no-key scrapers)
// File: fb.go
// ============================================================================
// The old cobalt instance (cobalt-api-ufprince.onrender.com) and the ytdlp
// metadata service (ytdlp-ufprince.onrender.com) were both SUSPENDED by Render
// (HTTP 503 "This service has been suspended"). This file replaces them with
// self-contained scrapers that need NO API key and NO third-party service:
//
//   FACEBOOK  -> fetch https://www.facebook.com/video/embed?video_id=<ID>
//                and read the JSON-escaped hd_src / sd_src direct CDN .mp4 URLs.
//
//   INSTAGRAM -> fetch https://www.instagram.com/p/<shortcode>/ with a
//                Googlebot User-Agent (Instagram serves the full page to
//                crawlers) and read the video_versions[].url direct CDN .mp4
//                URL, plus og:title / og:description / og:image metadata.
//
// Both scrapers return the SAME fbCobaltResponse shape the rest of the
// downloader already understands (Status="redirect", URL=<direct mp4>), so
// every existing call site keeps working unchanged.
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const fbUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
const fbGooglebotUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

const fbHelpText = "*\U0001f3c5 FACEBOOK VIDEO DOWNLOAD COMMAND \U0001f3c5*\n" +
	"*DO YOU WANT TO DOWNLOAD A FACEBOOK VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE FACEBOOK VIDEO LINK \U0001f644*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*.FB \u2770FACEBOOK VIDEO LINK\u2771*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR FACEBOOK VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"

// fbCobaltResponse models the downloader response. Kept under the original
// name so every existing call site (searchpick.go, insta.go, tests) compiles
// unchanged. Status is always "redirect" for the scrapers below.
type fbCobaltResponse struct {
	Status   string `json:"status"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Error    string `json:"error"`
	Picker   []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"picker"`
}

// ---------------------------------------------------------------------------
// Regexes
// ---------------------------------------------------------------------------

// Facebook video-id patterns (checked in order).
var fbVideoIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[?&]v=(\d{6,})`),
	regexp.MustCompile(`/videos/(?:[^/]+/)?(\d{6,})`),
	regexp.MustCompile(`/reel/(\d{6,})`),
	regexp.MustCompile(`video_id=(\d{6,})`),
	regexp.MustCompile(`/watch/?\?v=(\d{6,})`),
	regexp.MustCompile(`/share/v/([A-Za-z0-9_-]{6,})`),
}

// Instagram shortcode patterns.
var igShortcodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`instagram\.com/(?:reel|reels|p|tv)/([A-Za-z0-9_-]{5,})`),
	regexp.MustCompile(`instagr\.am/(?:reel|reels|p|tv)/([A-Za-z0-9_-]{5,})`),
}

var (
	fbHDSrcRe   = regexp.MustCompile(`"hd_src":"(.*?)"`)
	fbSDSrcRe   = regexp.MustCompile(`"sd_src":"(.*?)"`)
	igVideoVer  = regexp.MustCompile(`"video_versions":\[(.*?)\]`)
	igURLRe     = regexp.MustCompile(`"url":"(https:.*?)"`)
	igOGTitleRe = regexp.MustCompile(`<meta property="og:title" content="(.*?)"`)
	igOGDescRe  = regexp.MustCompile(`<meta property="og:description" content="(.*?)"`)
	igOGImageRe = regexp.MustCompile(`<meta property="og:image" content="(.*?)"`)
)

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

func handleFB(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutDur(s, info, socialTimeout, downloaderTimeoutReplyText, func(ctx context.Context) {
		handleFBAsync(ctx, s, info, args, prefix)
	})
}

func handleFBAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	fbURL := strings.TrimSpace(strings.Join(args, " "))
	if fbURL == "" {
		s.Reply(info, fbHelpText)
		return
	}
	if !strings.Contains(fbURL, "facebook.com") && !strings.Contains(fbURL, "fb.watch") && !strings.Contains(fbURL, "fb.com") {
		s.Reply(info, "\U0001f530 *FACEBOOK DOWNLOAD ERROR*\nPlease provide a valid Facebook link.")
		return
	}

	waitID := s.ReplyWithID(info, "\U0001f530 *Fetching Facebook video...*")

	resp, err := fbCobaltFetch(ctx, fbURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 *FACEBOOK DOWNLOAD ERROR*\n"+err.Error())
		return
	}

	// Resolve the video URL from the scraper response.
	videoURL, quality := fbResolveVideoURL(resp)
	if videoURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 YOUR FACEBOOK VIDEO WAS NOT FOUND \U0001f530")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, videoURL, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 PLEASE TRY AGAIN \U0001f530")
		return
	}
	defer removeTempFile(path)

	title := "Facebook Video"
	if resp.Filename != "" {
		title = strings.TrimSuffix(resp.Filename, ".mp4")
	}
	secs, w, h := probeVideoMeta(path)
	if secs == 0 && w == 0 {
		quality = "HD"
	}
	caption := "*\U0001f530 FACEBOOK VIDEO NAME \U0001f530*\n" +
		"*" + title + "*\n\n" +
		"*\U0001f530 QUALITY :* " + quality + "\n\n" +
		"*FACEBOOK VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 *FACEBOOK DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// fbResolveVideoURL picks the best media URL from a scraper response.
func fbResolveVideoURL(resp *fbCobaltResponse) (videoURL string, quality string) {
	if resp == nil {
		return "", ""
	}
	switch resp.Status {
	case "redirect", "tunnel", "stream":
		if resp.URL != "" {
			return resp.URL, "HD"
		}
	case "picker":
		for _, item := range resp.Picker {
			if item.Type == "video" && item.URL != "" {
				return item.URL, "HD"
			}
		}
		if len(resp.Picker) > 0 && resp.Picker[0].URL != "" {
			return resp.Picker[0].URL, "SD"
		}
	}
	return "", ""
}

// ---------------------------------------------------------------------------
// Platform-aware dispatcher (keeps the old name/signature)
// ---------------------------------------------------------------------------

// fbCobaltFetch is the single entry point used by every downloader call site.
// It routes to the Facebook embed scraper or the Instagram page scraper based
// on the URL, and always returns a fbCobaltResponse with Status="redirect".
func fbCobaltFetch(ctx context.Context, rawURL string) (*fbCobaltResponse, error) {
	low := strings.ToLower(strings.TrimSpace(rawURL))
	if strings.Contains(low, "instagram.com") || strings.Contains(low, "instagr.am") {
		return fbScrapeIG(ctx, rawURL)
	}
	return fbScrapeFB(ctx, rawURL)
}

// ---------------------------------------------------------------------------
// Facebook scraper
// ---------------------------------------------------------------------------

// fbScrapeFB resolves a Facebook video URL to a direct CDN .mp4 URL by reading
// the public embed page (no key, no third-party service).
func fbScrapeFB(ctx context.Context, fbURL string) (*fbCobaltResponse, error) {
	// fb.watch / share short links must be resolved to a canonical URL first.
	if strings.Contains(strings.ToLower(fbURL), "fb.watch") || strings.Contains(strings.ToLower(fbURL), "/share/") {
		if final := fbResolveFinalURL(ctx, fbURL); final != "" {
			fbURL = final
		}
	}

	id := fbExtractVideoID(fbURL)
	if id == "" {
		return nil, fmt.Errorf("could not find a video id in this link")
	}

	embed := "https://www.facebook.com/video/embed?video_id=" + id
	html, err := fbHTTPGet(ctx, embed, fbUserAgent)
	if err != nil {
		return nil, fmt.Errorf("facebook fetch failed: %v", err)
	}

	hd := fbExtractSrc(html, fbHDSrcRe)
	sd := fbExtractSrc(html, fbSDSrcRe)
	best := hd
	if best == "" {
		best = sd
	}
	if best == "" {
		return nil, fmt.Errorf("no downloadable video found for this link")
	}

	return &fbCobaltResponse{
		Status:   "redirect",
		URL:      best,
		Filename: "facebook_" + id + ".mp4",
	}, nil
}

// fbExtractVideoID pulls the numeric (or share) video id out of a FB URL.
func fbExtractVideoID(u string) string {
	for _, re := range fbVideoIDPatterns {
		if m := re.FindStringSubmatch(u); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// fbResolveFinalURL follows redirects (fb.watch short links) and returns the
// final URL, or "" on failure.
func fbResolveFinalURL(ctx context.Context, u string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", fbUserAgent)
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.Request != nil && res.Request.URL != nil {
		return res.Request.URL.String()
	}
	return ""
}

// fbExtractSrc reads a JSON-escaped "hd_src"/"sd_src" value and unescapes it.
func fbExtractSrc(html string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return fbUnescapeURL(m[1])
}

// ---------------------------------------------------------------------------
// Instagram scraper
// ---------------------------------------------------------------------------

// fbScrapeIG resolves an Instagram reel/post URL to a direct CDN .mp4 URL by
// reading the public page with a Googlebot User-Agent.
func fbScrapeIG(ctx context.Context, igURL string) (*fbCobaltResponse, error) {
	sc := igExtractShortcode(igURL)
	if sc == "" {
		return nil, fmt.Errorf("could not find an Instagram shortcode in this link")
	}
	page := "https://www.instagram.com/p/" + sc + "/"
	html, err := fbHTTPGet(ctx, page, fbGooglebotUA)
	if err != nil {
		return nil, fmt.Errorf("instagram fetch failed: %v", err)
	}
	vurl := igExtractVideoVersion(html)
	if vurl == "" {
		return nil, fmt.Errorf("no downloadable video found for this link")
	}
	return &fbCobaltResponse{
		Status:   "redirect",
		URL:      vurl,
		Filename: "instagram_" + sc + ".mp4",
	}, nil
}

// igExtractShortcode pulls the shortcode out of an Instagram URL.
func igExtractShortcode(u string) string {
	for _, re := range igShortcodePatterns {
		if m := re.FindStringSubmatch(u); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// igExtractVideoVersion returns the best (first) direct .mp4 URL from the
// video_versions array on an Instagram page.
func igExtractVideoVersion(html string) string {
	m := igVideoVer.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	urls := igURLRe.FindAllStringSubmatch(m[1], -1)
	for _, u := range urls {
		if len(u) > 1 {
			clean := fbUnescapeURL(u[1])
			if strings.Contains(clean, ".mp4") {
				return clean
			}
		}
	}
	return ""
}

// igScrapeMeta extracts title / owner / thumbnail from an Instagram page.
// Returns (title, owner, thumbnail). Any field may be "".
func igScrapeMeta(ctx context.Context, igURL string) (title, owner, thumb string) {
	sc := igExtractShortcode(igURL)
	if sc == "" {
		return "", "", ""
	}
	page := "https://www.instagram.com/p/" + sc + "/"
	html, err := fbHTTPGet(ctx, page, fbGooglebotUA)
	if err != nil {
		return "", "", ""
	}
	if m := igOGTitleRe.FindStringSubmatch(html); len(m) > 1 {
		title = fbUnescapeURL(m[1])
		// og:title looks like: "Owner on Instagram: \"caption\""
		if i := strings.Index(title, " on Instagram"); i > 0 {
			owner = strings.TrimSpace(title[:i])
		}
		if i := strings.Index(title, "Instagram: "); i >= 0 {
			title = strings.Trim(strings.TrimSpace(title[i+len("Instagram: "):]), "\"")
		}
	}
	if owner == "" {
		if m := igOGDescRe.FindStringSubmatch(html); len(m) > 1 {
			desc := fbUnescapeURL(m[1])
			// og:description: "23 likes, 8 comments - zcairns on August 26, 2025: ..."
			if i := strings.Index(desc, " - "); i >= 0 {
				rest := desc[i+3:]
				if j := strings.Index(rest, " on "); j > 0 {
					owner = strings.TrimSpace(rest[:j])
				}
			}
		}
	}
	if m := igOGImageRe.FindStringSubmatch(html); len(m) > 1 {
		thumb = fbUnescapeURL(m[1])
	}
	return title, owner, thumb
}

// ---------------------------------------------------------------------------
// Shared HTTP + unescape helpers
// ---------------------------------------------------------------------------

// fbHTTPGet performs a GET with the given User-Agent and returns the body.
func fbHTTPGet(ctx context.Context, url, ua string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// fbUnescapeURL reverses the JSON/HTML escaping used inside Facebook and
// Instagram pages (\/ -> /, \u0026 -> &, &amp; -> &, etc.).
func fbUnescapeURL(s string) string {
	r := strings.NewReplacer(
		`\\u0025`, "%", `\u0025`, "%",
		`\\u0026`, "&", `\u0026`, "&",
		`\\u003D`, "=", `\u003D`, "=",
		`\\u003d`, "=", `\u003d`, "=",
		`\\/`, "/", `\/`, "/",
		`&amp;`, "&",
		`\\"`, `"`, `\"`, `"`,
	)
	s = r.Replace(s)
	// Cut at any leftover escape / tag boundary.
	for _, cut := range []string{`\u003C`, `\\u003C`, "<", `"`} {
		if p := strings.Index(s, cut); p > 0 {
			s = s[:p]
		}
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// Exported wrappers for the live sandbox test binary.
// ---------------------------------------------------------------------------

// FBCobaltFetchLive - exported wrapper for the live sandbox test binary.
func FBCobaltFetchLive(ctx context.Context, url string) (*fbCobaltResponse, error) {
	return fbCobaltFetch(ctx, url)
}

// FBResolveVideoURLLive - exported wrapper for the live sandbox test binary.
func FBResolveVideoURLLive(resp *fbCobaltResponse) (string, string) {
	return fbResolveVideoURL(resp)
}
