package goldcmds

// ============================================================================
// GOLD-MD — TikTok VIDEO search engine (feed/search, multi-page)
// File: tt_videos.go
// ============================================================================
// .tt <query> ka search asli VIDEOS return karta hai — 15 SHORTS
// (<= 60s) + 15 LONG (2 min+), total 30 links (owner: "15 shorts videos
// ka link aye 15 long videos ka link aye ... list total 30 videos ki
// bane ge").
//
// Owner feedback (round 2): "~1 min ki videos LONG me nahi chahiye — wo
// to shorts list me already aa rahi thin. LONG me 4/5 min aur 10 min ki
// videos hon, aur har entry pe alag DURATION line ho take user ko pata
// chale video kitni lambi hai."
//
// Rules:
//   SHORTS: <= 60s   — search relevance order me pehli 15
//   LONG:   >= 120s  — 5 pages ke tamam candidates DURATION DESC sort
//                      hote hain: sab se lambi (10 min) video list ke
//                      TOP pe, phir 5 min, 4 min ...
//   61-119s videos: na SHORTS me (header < 1 min galat ho jata) na LONG
//                      me (owner: wo shorts jaisi hi hain) — drop.
//
// Source: tikwm /api/feed/search/ (TRAILING SLASH zaroori — bina slash
// ke Cloudflare challenge). 30 videos/page, cursor+hasMore pagination,
// 5 pages tak scan (150 videos); duplicates skip; pages ke darmiyan
// 1.2s gap (tikwm free 1 req/s).
//
// Har result ka link https://www.tiktok.com/@user/video/ID hai — wahi
// canonical video page jo proven self-scrape engine (ttSelfFetch)
// direct download karta hai; tikwm /api/?url= fallback bhi chalta hai.
// DurationSec field card ki DURATION line + SHORTS/LONG split ke liye
// fill hota hai.
//
// Engine ladder:
//   1. tikwm feed/search/  (primary — real videos with ids + stats)
//   2. tikwm user/search   (fallback — feed/search down ho to wahi
//      purane account results; pick wahi profile flow use karta hai)
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ttVideoSearch finds REAL TikTok videos for a query (not accounts) and
// returns a combined list: SHORTS (<= 60s) in relevance order first,
// then LONG (>= 120s) sorted longest-first, capped at 15 + 15 = 30.
func ttVideoSearch(ctx context.Context, query string) ([]searchResult, error) {
	const (
		wantShorts = 15
		wantLongs  = 15
		maxPages   = 5   // 30/page -> up to 150 videos scanned
		longMin    = 120 // LONG = 2 min+ (owner: 4/5/10 min songs, NOT ~1 min)
	)
	var (
		shorts    []searchResult
		longCands []searchResult // uncapped — sort ke BAAD top 15
		seen      = map[string]bool{}
		cursor    int64
	)
	for page := 0; page < maxPages; page++ {
		u := "https://www.tikwm.com/api/feed/search/?keywords=" + url.QueryEscape(query) + "&count=30"
		if page > 0 {
			u += fmt.Sprintf("&cursor=%d", cursor)
		}
		pageResults, next, more, err := ttVideoSearchPage(ctx, u)
		if err != nil {
			// page 1 failing = engine down (caller falls back to the old
			// user-search); a later page failing just stops pagination —
			// jo mil gaya wahi card me jata hai.
			if page == 0 {
				return nil, err
			}
			break
		}
		for _, r := range pageResults {
			if seen[r.Link] {
				continue
			}
			seen[r.Link] = true
			switch {
			case r.DurationSec >= longMin:
				longCands = append(longCands, r)
			case r.DurationSec > 60:
				// 61-119s: shorts jaisi hi (owner feedback) — dono lists
				// me nahi jati.
				continue
			default:
				if len(shorts) < wantShorts {
					shorts = append(shorts, r)
				}
			}
		}
		if len(shorts) >= wantShorts && len(longCands) >= wantLongs {
			break
		}
		if !more || next == 0 {
			break
		}
		cursor = next
		// tikwm free tier: 1 req/s — pages ke darmiyan chhota gap
		select {
		case <-ctx.Done():
		case <-time.After(1200 * time.Millisecond):
		}
	}
	// LONG list: sab se lambi video TOP pe (owner: "10 mint ki videos ki
	// b list me") — duration DESC sort, phir top 15.
	sort.Slice(longCands, func(i, j int) bool {
		return longCands[i].DurationSec > longCands[j].DurationSec
	})
	if len(longCands) > wantLongs {
		longCands = longCands[:wantLongs]
	}
	return append(shorts, longCands...), nil
}

// filterTTResults drops junk entries before the card: tikwm kabhi
// kabhi 23min+ compilation/live-replay videos lauta deta hai jo
// TikTok k limit se bhar hain (10 min max) aur card me jagah
// kha jati hain. DurationSec 0 (unknown) skip nahi karte — stats
// hi miss ho jate, video valid hoti hai.
func filterTTResults(results []searchResult) []searchResult {
	out := results[:0]
	for _, r := range results {
		if r.DurationSec > 1400 { // > 23min = junk
			continue
		}
		out = append(out, r)
	}
	return out
}

// ttVideoSearchPage fetches one feed/search/ page and returns normalized
// results plus the next cursor and hasMore flag for pagination.
func ttVideoSearchPage(ctx context.Context, u string) (results []searchResult, cursor int64, hasMore bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.tikwm.com/")
	resp, err := searchHTTP.Do(req)
	if err != nil {
		return nil, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, 0, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed ttVideoSearchResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return nil, 0, false, err
	}
	if parsed.Code != 0 {
		return nil, 0, false, fmt.Errorf("%s", parsed.Msg)
	}
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
		// duration ab card ki alag DURATION line me hai (owner round 2)
		stats := searchFmtCount(v.PlayCount) + " PLAYS ❰ " +
			searchFmtCount(v.DiggCount) + " LIKES ❱"
		results = append(results, searchResult{
			Title:       title,
			Handle:      handle,
			Stats:       stats,
			Link:        "https://www.tiktok.com/@" + v.Author.UniqueId + "/video/" + v.VideoID,
			DurationSec: v.Duration,
		})
	}
	return results, parsed.Data.Cursor, parsed.Data.HasMore, nil
}

// ttFmtDuration renders seconds as "45s" / "4m" / "3m45s" (card ki
// DURATION line is ko ToUpper kar ke "4M05S" render karti hai).
func ttFmtDuration(sec int64) string {
	if sec <= 0 {
		return ""
	}
	if sec < 60 {
		return strconv.FormatInt(sec, 10) + "s"
	}
	m, s := sec/60, sec%60
	if s == 0 {
		return strconv.FormatInt(m, 10) + "m"
	}
	return fmt.Sprintf("%dm%02ds", m, s)
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
		Cursor  int64 `json:"cursor"`
		HasMore bool  `json:"hasMore"`
	} `json:"data"`
}
