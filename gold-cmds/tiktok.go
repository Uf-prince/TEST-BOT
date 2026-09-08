package goldcmds

// ============================================================================
// GOLD-MD — TikTok Downloader
// File: tiktok.go
// ============================================================================
// HANDLER: handleTikTok — used by .ttsearch direct-link router
//   Downloads a TikTok video (HD preferred) and sends it.
//
// API: tikwm  (GET https://www.tikwm.com/api/?url=<URL>)
//   Response JSON: { code: 0, msg: "success", data: { id, title, cover,
//     duration, play, wmplay, size, wm_size, music, play_count, digg_count,
//     comment_count, share_count, collect_count, author: { nickname,
//     unique_id } } }
//
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const tikwmAPI = "https://www.tikwm.com/api/"

const tiktokHelpText = "*\U0001f3c5 TIKTOK COMMAND INFO \U0001f3c5*\n" +
	"*COPY THE TIKTOK VIDEO LINK*\n" +
	"*PASTE TIKTOK VIDEO LINK LIKE THIS \U0001f60a*\n\n" +
	"*.TTSEARCH \u2770TIKTOK LINK\u2771*\n" +
	"*EXAMPLE.....*\n" +
	"*.TTSEARCH https://vm.tiktok.com/xxxxx*\n\n" +
	"*YOUR TIKTOK VIDEO WILL BE SENT HERE \U0001f917*"

// tikwmResponse models the tikwm API response.
type tikwmResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Cover        string `json:"cover"`
		OriginCover  string `json:"origin_cover"`
		Duration     int    `json:"duration"`
		Play         string `json:"play"`
		HDPlay       string `json:"hdplay"`
		WMPlay       string `json:"wmplay"`
		Size         int64  `json:"size"`
		WMSize       int64  `json:"wm_size"`
		Music        string `json:"music"`
		PlayCount    int64  `json:"play_count"`
		DiggCount    int64  `json:"digg_count"`
		CommentCount int64  `json:"comment_count"`
		ShareCount   int64  `json:"share_count"`
		CollectCount int64  `json:"collect_count"`
		Author       struct {
			UniqueID string `json:"unique_id"`
			Nickname string `json:"nickname"`
		} `json:"author"`
	} `json:"data"`
}

func handleTikTok(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleTikTokAsync(ctx, s, info, args, prefix)
	})
}

func handleTikTokAsync(	ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	ttURL := strings.TrimSpace(strings.Join(args, " "))
	if ttURL == "" {
		s.Reply(info, tiktokHelpText)
		return
	}
	if !strings.Contains(ttURL, "tiktok.com") {
		s.Reply(info, "❌ *TIKTOK DOWNLOAD ERROR*\nPlease provide a valid TikTok link.")
		return
	}

	waitID := s.ReplyWithID(info, "⏳ *Fetching TikTok video...*")

	resp, err := tikwmFetch(ctx, ttURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ VIDEO NOT FOUND. PLEASE CHECK THE LINK AND TRY AGAIN 🤗")
		return
	}

	// Preference order: hdplay -> play -> wmplay
	videoURL := firstNonEmpty(resp.Data.HDPlay, resp.Data.Play, resp.Data.WMPlay)
	if videoURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ VIDEO URL NOT FOUND. PLEASE TRY AGAIN 😓")
		return
	}

	s.EditMessage(info, waitID, "⬇️ *Downloading video...*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, videoURL, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TIKTOK DOWNLOAD ERROR*\nPlease try again.")
		return
	}
	defer removeTempFile(path)

	secs, w, h := probeVideoMeta(path)

	title := resp.Data.Title
	if title == "" {
		title = "TikTok Video"
	}
	creator := resp.Data.Author.Nickname
	if creator == "" {
		creator = resp.Data.Author.UniqueID
	}
	if creator == "" {
		creator = "User"
	}
	caption := "*🏅 TIKTOK VIDEO NAME 🏅*\n" +
		"*" + title + "*\n\n" +
		"*🏅 CREATOR :* " + creator + "\n" +
		fmt.Sprintf("*🏅 TIME :* %ds\n", resp.Data.Duration) +
		fmt.Sprintf("*🏅 LIKES :* %d\n", resp.Data.DiggCount) +
		fmt.Sprintf("*🏅 COMMENTS :* %d\n", resp.Data.CommentCount) +
		fmt.Sprintf("*🏅 VIEWS :* %d\n\n", resp.Data.PlayCount) +
		"*TIKTOK VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TIKTOK DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// tikwmFetch calls the tikwm API and returns the parsed response.
func tikwmFetch(ctx context.Context, ttURL string) (*tikwmResponse, error) {
	fullURL := fmt.Sprintf("%s?url=%s", tikwmAPI, url.QueryEscape(ttURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	var parsed tikwmResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %v", err)
	}
	if parsed.Code != 0 {
		msg := parsed.Msg
		if msg == "" {
			msg = "API returned error"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return &parsed, nil
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
