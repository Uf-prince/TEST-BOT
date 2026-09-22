package goldcmds

// ============================================================================
// GOLD-MD — YouTube rich metadata via Innertube /next (no API key)
// File: ytmeta.go
// ============================================================================
// The /next endpoint returns the full watch-page metadata in one call:
//   • title    → videoPrimaryInfoRenderer.title.runs[0].text
//   • views    → videoPrimaryInfoRenderer.viewCount.videoViewCountRenderer
//                .shortViewCount.simpleText  (e.g. "1.8B views")
//   • author   → videoSecondaryInfoRenderer.owner.videoOwnerRenderer
//                .title.runs[0].text
//   • comments → engagementPanelTitleHeaderRenderer (title "Comments")
//                .contextualInfo.runs[0].text  (e.g. "2.4M")
// This works even for music / age-gated videos where the player endpoint
// returns LOGIN_REQUIRED, so it is the reliable source for the thumbnail
// caption (author / comments / views) used by .play/.play2/.video/.video2.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const ytNextURL = "https://www.youtube.com/youtubei/v1/next?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"

const ytNextUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

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
