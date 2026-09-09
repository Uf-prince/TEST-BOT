package goldcmds

// ============================================================================
// GOLD-MD — TikTok VIDEO search engine (feed/search)
// File: tt_videos.go
// ============================================================================
// .tt <query> ka search ab USER accounts nahi, asli VIDEOS return karta hai.
//
// Pehle masla: ttUserSearch (tikwm /api/user/search) sirf accounts lauta
// raha tha — card pe "TO DOWNLOAD THIS FROM TIKTOK" likhta tha lekin pick
// pe profile scrape fail (datacenter IP block) hote the aur sirf error
// milta tha (owner: "jab tak asal video ka link nai aye ga to error hi
// bheje ga na bot").
//
// Fix: tikwm ka /api/feed/search/ endpoint (TRAILING SLASH zaroori hai —
// bina slash ke Cloudflare challenge lagta hai) VIDEOS lauta hai:
// video_id + title + author + play/hdplay/wmplay + stats. Har result ka
// link https://www.tiktok.com/@user/video/ID hai — wahi canonical video
// page jo proven self-scrape engine (ttSelfFetch) direct download karta
// hai. tikwm /api/?url= fallback bhi video pages pe chalta hai.
//
// Engine ladder:
//   1. tikwm feed/search/  (primary — real videos with ids + stats)
//   2. tikwm user/search   (fallback — agar feed/search down ho to wahi
//      purane account results, video-count ke sath; pick wahi profile
//      flow use karta hai)
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ttVideoSearch finds REAL TikTok videos for a query (not accounts).
func ttVideoSearch(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://www.tikwm.com/api/feed/search/?keywords=" + url.QueryEscape(query) + "&count=10"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.tikwm.com/")
	resp, err := searchHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed ttVideoSearchResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("%s", parsed.Msg)
	}
	var out []searchResult
	for _, v := range parsed.Data.Videos {
		if v.VideoID == "" {
			continue
		}
		title := strings.TrimSpace(v.Title)
		if title == "" {
			title = "TikTok Video"
		}
		handle := ""
		if v.Author.UniqueId != "" {
			handle = "@" + v.Author.UniqueId
		}
		stats := searchFmtCount(v.PlayCount) + " PLAYS ❰ " +
			searchFmtCount(v.DiggCount) + " LIKES ❱"
		out = append(out, searchResult{
			Title:  title,
			Handle: handle,
			Stats:  stats,
			Link:   "https://www.tiktok.com/@" + v.Author.UniqueId + "/video/" + v.VideoID,
		})
	}
	return out, nil
}

// ttVideoSearchResp is the tikwm feed/search/ response shape.
type ttVideoSearchResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Videos []struct {
			VideoID      string `json:"video_id"`
			Title        string `json:"title"`
			Play         string `json:"play"`
			PlayCount    int64  `json:"play_count"`
			DiggCount    int64  `json:"digg_count"`
			CommentCount int64  `json:"comment_count"`
			ShareCount   int64  `json:"share_count"`
			Duration     int64  `json:"duration"`
			Author       struct {
				UniqueId string `json:"unique_id"`
				Nickname string `json:"nickname"`
			} `json:"author"`
		} `json:"videos"`
	} `json:"data"`
}
