package goldcmds

// ============================================================================
// GOLD-MD — TikTok Downloader
// File: tiktok.go
// ============================================================================
// HANDLER: handleTikTok — used by .tt and the direct-link router
//   Downloads a TikTok video (HD preferred) and sends it.
//
// ENGINE — TikTok self-scrape (NO API, NO KEYS, free + permanent, works
// from datacenter IPs like modal.com because it hits tiktok.com directly):
//   1. Fresh cookie jar + iPhone mobile UA → GET the video page
//      (vm.tiktok.com / vt.tiktok.com short links auto-follow redirects).
//   2. Parse <script id="__UNIVERSAL_DATA_FOR_REHYDRATION__"> JSON:
//      __DEFAULT_SCOPE__ → webapp.reflow.video.detail (fallback
//      webapp.video-detail) → itemInfo.itemStruct → playAddr + meta.
//   3. Download playAddr with the SAME cookie jar + mobile UA +
//      Referer: https://www.tiktok.com/ → valid MP4.
//
// NO API KEYS, NO THIRD-PARTY API (owner directive: "tiktok ki koi api key
// nahi dhundi direct html scrapee krwa download k lie bhi"). tikwm engine
// REMOVED — sirf TikTok ka direct HTML scrape.
//
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	// ttMobileUA — TikTok serves the full rehydration JSON to mobile clients.
	ttMobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2_1 like Mac OS X) " +
		"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/E15AC Safari/604.1"

	ttReferer = "https://www.tiktok.com/"
	ttMaxPage = 8 << 20 // 8 MB cap for the video page HTML
)

var ttDataScriptRe = regexp.MustCompile(
	`(?s)<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">(.*?)</script>`)

const tiktokHelpText = "🔰 TIKTOK COMMAND INFO 🔰\n" +
	"*COPY THE TIKTOK VIDEO LINK*\n" +
	"*PASTE TIKTOK VIDEO LINK LIKE THIS 🔰*\n\n" +
	"*.TT ❰TIKTOK LINK❱*\n" +
	"*EXAMPLE.....*\n" +
	"*.TT https://vm.tiktok.com/xxxxx*\n\n" +
	"*YOUR TIKTOK VIDEO WILL BE SENT HERE 🔰*"

// ── self-scrape JSON models ────────────────────────────────────────────────

// ttSelfData models the rehydration script: outer wrapper __DEFAULT_SCOPE__,
// then the reflow key (mobile-UA page) with the desktop variant as fallback.
type ttSelfData struct {
	Scope struct {
		Reflow struct {
			StatusCode int `json:"statusCode"`
			ItemInfo struct {
				ItemStruct ttItem `json:"itemStruct"`
			} `json:"itemInfo"`
		} `json:"webapp.reflow.video.detail"`
		Detail struct {
			StatusCode int `json:"statusCode"`
			ItemInfo struct {
				ItemStruct ttItem `json:"itemStruct"`
			} `json:"itemInfo"`
		} `json:"webapp.video-detail"`
	} `json:"__DEFAULT_SCOPE__"`
}

// ttItem is one TikTok video item (itemStruct).
type ttItem struct {
	ID    string `json:"id"`
	Desc  string `json:"desc"`
	Video struct {
		Duration int    `json:"duration"`
		PlayAddr string `json:"playAddr"`
		Cover    string `json:"cover"`
	} `json:"video"`
	Stats struct {
		PlayCount    int64 `json:"playCount"`
		DiggCount    int64 `json:"diggCount"`
		CommentCount int64 `json:"commentCount"`
		ShareCount   int64 `json:"shareCount"`
	} `json:"stats"`
	Author struct {
		UniqueID string `json:"uniqueId"`
		Nickname string `json:"nickname"`
	} `json:"author"`
}

// ttResult is the unified download result. SrcClient keeps the cookie-jar
// client from the page fetch (playAddr download needs the same cookies).
type ttResult struct {
	ID           string
	Title        string
	Cover        string
	Duration     int
	Play         string // main MP4 URL
	HDPlay       string // HD URL if known
	WMPlay       string // watermarked URL if known
	PlayCount    int64
	DiggCount    int64
	CommentCount int64
	ShareCount   int64
	AuthorUnique string
	AuthorName   string
	SrcClient    *http.Client
}


func handleTikTok(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutDur(s, info, socialTimeout, downloaderTimeoutReplyText, func(ctx context.Context) {
		handleTikTokAsync(ctx, s, info, args, prefix)
	})
}

func handleTikTokAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	ttURL := strings.TrimSpace(strings.Join(args, " "))
	if ttURL == "" {
		s.Reply(info, tiktokHelpText)
		return
	}
	if !strings.Contains(ttURL, "tiktok.com") {
		s.Reply(info, "🔰 *TIKTOK DOWNLOAD ERROR*\nPlease provide a valid TikTok link.")
		return
	}

	waitID := s.ReplyWithID(info, "🔰 *Fetching TikTok video...*")

	// TikTok self-scrape (direct HTML, no API).
	res, err := ttSelfFetch(ctx, ttURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 VIDEO NOT FOUND. PLEASE CHECK THE LINK AND TRY AGAIN 🔰")
		return
	}

	// Preference order: hdplay -> play -> wmplay
	videoURL := firstNonEmpty(res.HDPlay, res.Play, res.WMPlay)
	if videoURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 VIDEO URL NOT FOUND. PLEASE TRY AGAIN 🔰")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := res.SrcClient
	if client == nil {
		client = mediaHTTPClient()
	}
	path, err := ttStreamDownload(ctx, client, videoURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TIKTOK DOWNLOAD ERROR*\nPlease try again.")
		return
	}
	defer removeTempFile(path)

	secs, w, h := probeVideoMeta(path)

	title := res.Title
	if strings.TrimSpace(title) == "" {
		title = "TikTok Video"
	}
	creator := res.AuthorName
	if creator == "" {
		creator = res.AuthorUnique
	}
	if creator == "" {
		creator = "User"
	}
	caption := "🔰 TIKTOK VIDEO NAME 🔰\n" +
		"*" + title + "*\n\n" +
		"🔰 *CREATOR :* " + creator + "\n" +
		fmt.Sprintf("🔰 *TIME :* %ds\n", res.Duration) +
		fmt.Sprintf("🔰 *LIKES :* %d\n", res.DiggCount) +
		fmt.Sprintf("🔰 *COMMENTS :* %d\n", res.CommentCount) +
		fmt.Sprintf("🔰 *VIEWS :* %d\n\n", res.PlayCount) +
		"*TIKTOK VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TIKTOK DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// ── ENGINE 1: TikTok self-scrape ──────────────────────────────────────────

// ttThrottleErr translates TikTok rehydration statusCode into a clear error.
// 10204 = web_visit_cnt_more_than_3: the video is globally throttled because
// it has been visited too many times (popular videos). Fresh videos always
// return statusCode 0 with full data — pick a different result instead.
func ttThrottleErr(reflow, detail int) error {
	sc := reflow
	if sc == 0 {
		sc = detail
	}
	if sc != 0 {
		return fmt.Errorf("TikTok is currently throttling this popular video (status %d). Fresh videos work fine — dusra number pick karo ya dobara .tt se search karo", sc)
	}
	return fmt.Errorf("no video item found in page data")
}

// ttSelfFetch fetches the TikTok video page with a fresh cookie jar and a
// mobile UA, extracts the rehydration JSON and maps the video item.
// vm/vt short links are resolved by the client's automatic redirect follow.
func ttSelfFetch(ctx context.Context, ttURL string) (*ttResult, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Jar:     jar,
	}

	// 1) fetch the page — the jar collects tt_chain_token / ttwid cookies.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ttURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ttMobileUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tiktok page returned status %d", resp.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, ttMaxPage))
	if err != nil {
		return nil, err
	}

	// 2) extract + parse the rehydration JSON.
	m := ttDataScriptRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("rehydration script not found in page")
	}
	var data ttSelfData
	if err := json.Unmarshal(m[1], &data); err != nil {
		return nil, fmt.Errorf("failed to parse rehydration JSON: %v", err)
	}

	// 3) pick the item — reflow key (mobile) first, desktop key second.
	var item ttItem
	switch {
	case data.Scope.Reflow.ItemInfo.ItemStruct.ID != "":
		item = data.Scope.Reflow.ItemInfo.ItemStruct
	case data.Scope.Detail.ItemInfo.ItemStruct.ID != "":
		item = data.Scope.Detail.ItemInfo.ItemStruct
	default:
		return nil, ttThrottleErr(data.Scope.Reflow.StatusCode, data.Scope.Detail.StatusCode)
	}
	if strings.TrimSpace(item.Video.PlayAddr) == "" {
		if e := ttThrottleErr(data.Scope.Reflow.StatusCode, data.Scope.Detail.StatusCode); e != nil {
			return nil, e
		}
		return nil, fmt.Errorf("playAddr empty in page data")
	}

	title := item.Desc
	if strings.TrimSpace(title) == "" {
		title = "TikTok Video"
	}
	return &ttResult{
		ID:           item.ID,
		Title:        title,
		Cover:        item.Video.Cover,
		Duration:     item.Video.Duration,
		Play:         item.Video.PlayAddr,
		PlayCount:    item.Stats.PlayCount,
		DiggCount:    item.Stats.DiggCount,
		CommentCount: item.Stats.CommentCount,
		ShareCount:   item.Stats.ShareCount,
		AuthorUnique: item.Author.UniqueID,
		AuthorName:   item.Author.Nickname,
		SrcClient:    client,
	}, nil
}

// ttStreamDownload downloads the MP4 with the mobile UA + TikTok Referer
// (required for TikTok CDN playAddr links).
func ttStreamDownload(ctx context.Context, client *http.Client, videoURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ttMobileUA)
	req.Header.Set("Referer", ttReferer)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "gold-md-tt-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err = io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// firstNonEmpty returns the first non-empty string from the given list.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
