package goldcmds

// ============================================================================
// GOLD-MD — YouTube rich metadata (no API key)
// File: ytmeta.go
// ============================================================================
// Two independent, key-less sources are used to fill the thumbnail caption
// (author / comments / views / duration) used by .play/.play2/.video/.video2:
//
//  1. yt2FetchDetails — Innertube /player with the ANDROID_TESTSUITE client.
//     This client returns videoDetails (title / author / lengthSeconds /
//     viewCount) EVEN WHEN playabilityStatus is UNPLAYABLE or LOGIN_REQUIRED
//     (music videos, age-gated videos). This is the RELIABLE source for
//     author + duration, which the streaming race cannot provide for those
//     videos (all streaming clients return LOGIN_REQUIRED).
//
//  2. yt2FetchMeta — Innertube /next (WEB client). Returns title / author /
//     views / comments. Used mainly for the COMMENT count (which the player
//     endpoint does not expose).
//
// Both are fetched in PARALLEL with the streaming race, so they add no
// latency to the download.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const ytNextURL = "https://www.youtube.com/youtubei/v1/next?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"

const ytNextUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// yt2TestsuiteUA is the User-Agent for the ANDROID_TESTSUITE metadata client.
const yt2TestsuiteUA = "com.google.android.youtube/1.9 (Linux; U; Android 11) gzip"

// yt2Details holds the videoDetails returned by the ANDROID_TESTSUITE player
// call. Unlike the streaming clients, this works for EVERY video (including
// music / age-gated where streaming is blocked).
type yt2Details struct {
	title    string
	author   string
	duration string // formatted, e.g. "3:33"
	views    int
}

// yt2FetchDetails queries the Innertube /player endpoint with the
// ANDROID_TESTSUITE client. It returns videoDetails (title / author /
// lengthSeconds / viewCount) even when playabilityStatus is UNPLAYABLE or
// LOGIN_REQUIRED. Returns nil on any failure.
func yt2FetchDetails(ctx context.Context, videoID string) *yt2Details {
	if videoID == "" {
		return nil
	}
	body := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":        "ANDROID_TESTSUITE",
				"clientVersion":     "1.9",
				"androidSdkVersion": 30,
				"osName":            "Android",
				"osVersion":         "11",
				"hl":                "en",
				"gl":                "US",
			},
		},
		"videoId":        videoID,
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ytDirectPlayerURL, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", yt2TestsuiteUA)
	res, err := yt2HTTP.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var pr ytDirectPlayerResp
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&pr); err != nil {
		return nil
	}
	// videoDetails is present even when the video is unplayable — that is the
	// whole point of this client. Bail only when it is genuinely empty.
	if pr.VideoDetails.Title == "" && pr.VideoDetails.Author == "" && pr.VideoDetails.LengthSec == "" {
		return nil
	}
	d := &yt2Details{
		title:  pr.VideoDetails.Title,
		author: pr.VideoDetails.Author,
	}
	if n, err := strconv.Atoi(pr.VideoDetails.LengthSec); err == nil && n > 0 {
		d.duration = ytDirectFmtDuration(n)
	}
	if n, err := strconv.Atoi(pr.VideoDetails.ViewCount); err == nil {
		d.views = n
	}
	return d
}

// yt2Meta holds the rich metadata scraped from the /next endpoint.
type yt2Meta struct {
	title    string
	author   string
	views    string // short form, e.g. "1.8B"
	comments string // e.g. "2.4M"
}

// yt2FetchMeta queries the Innertube /next endpoint for the given video id and
// returns the parsed metadata. Returns nil on any failure (callers fall back
// to the player-race data).
func yt2FetchMeta(ctx context.Context, videoID string) *yt2Meta {
	if videoID == "" {
		return nil
	}
	body := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20240101.00.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
		"videoId": videoID,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ytNextURL, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ytNextUserAgent)

	res, err := yt2HTTP.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var data map[string]any
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&data); err != nil {
		return nil
	}

	m := &yt2Meta{}
	if p := yt2FindRenderer(data, "videoPrimaryInfoRenderer"); p != nil {
		m.title = yt2DigStr(p, "title", "runs", 0, "text")
		m.views = yt2CleanViews(yt2DigStr(p, "viewCount", "videoViewCountRenderer", "shortViewCount", "simpleText"))
		if m.views == "" {
			m.views = yt2CleanViews(yt2DigStr(p, "viewCount", "videoViewCountRenderer", "viewCount", "simpleText"))
		}
	}
	if s := yt2FindRenderer(data, "videoSecondaryInfoRenderer"); s != nil {
		m.author = yt2DigStr(s, "owner", "videoOwnerRenderer", "title", "runs", 0, "text")
	}
	// Fallback: playerOverlayVideoDetailsRenderer.subtitle.runs[0].text is the
	// channel name (works even when videoSecondaryInfoRenderer is absent).
	if m.author == "" {
		if po := yt2FindRenderer(data, "playerOverlayVideoDetailsRenderer"); po != nil {
			m.author = yt2DigStr(po, "subtitle", "runs", 0, "text")
		}
	}
	m.comments = yt2FindComments(data)
	return m
}

// yt2CleanViews strips the trailing " views" / " watching" suffix and trims.
func yt2CleanViews(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " views")
	s = strings.TrimSuffix(s, " view")
	s = strings.TrimSuffix(s, " watching")
	return strings.TrimSpace(s)
}

// yt2FindRenderer recursively searches the decoded JSON tree for the first
// map stored under the given renderer key.
func yt2FindRenderer(node any, key string) map[string]any {
	switch v := node.(type) {
	case map[string]any:
		if r, ok := v[key]; ok {
			if rm, ok := r.(map[string]any); ok {
				return rm
			}
		}
		for _, cv := range v {
			if r := yt2FindRenderer(cv, key); r != nil {
				return r
			}
		}
	case []any:
		for _, cv := range v {
			if r := yt2FindRenderer(cv, key); r != nil {
				return r
			}
		}
	}
	return nil
}

// yt2FindComments finds the engagementPanelTitleHeaderRenderer whose title is
// "Comments" and returns its contextualInfo count (e.g. "2.4M").
func yt2FindComments(node any) string {
	switch v := node.(type) {
	case map[string]any:
		if r, ok := v["engagementPanelTitleHeaderRenderer"]; ok {
			if rm, ok := r.(map[string]any); ok {
				title := yt2DigStr(rm, "title", "runs", 0, "text")
				if strings.EqualFold(strings.TrimSpace(title), "Comments") {
					if c := yt2DigStr(rm, "contextualInfo", "runs", 0, "text"); c != "" {
						return c
					}
				}
			}
		}
		for _, cv := range v {
			if c := yt2FindComments(cv); c != "" {
				return c
			}
		}
	case []any:
		for _, cv := range v {
			if c := yt2FindComments(cv); c != "" {
				return c
			}
		}
	}
	return ""
}

// yt2DigStr walks a decoded JSON tree following the given path (string keys or
// int slice indices) and returns the terminal string value ("" if missing).
func yt2DigStr(node any, path ...any) string {
	cur := node
	for _, p := range path {
		switch key := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur, ok = m[key]
			if !ok {
				return ""
			}
		case int:
			arr, ok := cur.([]any)
			if !ok || key < 0 || key >= len(arr) {
				return ""
			}
			cur = arr[key]
		default:
			return ""
		}
	}
	s, _ := cur.(string)
	return s
}
