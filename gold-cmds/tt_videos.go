package goldcmds

// ============================================================================
// GOLD-MD — TikTok VIDEO search engine (Brave Search HTML scrape + oembed)
// File: tt_videos.go
// ============================================================================
// .tt <query> ka search asli VIDEOS return karta hai — 15 SHORTS
// (<= 60s) + 15 LONG (2 min+), total 30 links.
//
// OWNER DIRECTIVE (no API keys, render pe tikwm block ho jata hai):
//   "tiktok ki koi api key nahi dhundi direct html scrapee krwa
//    serch k lie b aur download k lie bhi"
//   → tikwm COMPLETELY REMOVED. Sirf HTML scraping:
//
// SEARCH ENGINE — Brave Search HTML (search.brave.com):
//   1. GET https://search.brave.com/search?q=site:tiktok.com <query>
//      (plain HTML, no key, no JS; page pe @user/video/ID links embed
//      hote hain result cards me)
//   2. Multiple query variations fire (\"<q>\", \"<q> videos\",
//      \"<q> viral\", \"<q> compilation\", ...) — har variation se
//      naye video links milte hain, duplicates skip.
//   3. Har unique video link ko TikTok oembed (https://www.tiktok.com/
//      oembed?url=...) se enrich karte hain — title + author.
//      oembed keyless public HTML endpoint hai (browser oembed standard).
//      Duration nahi milti — SHORTS/LONG split hat gaya (sab SHORTS list
//      me; owner chahe to baad me duration filter wapas la sakte hain).
//
// HAR SEARCH RESULT KA LINK https://www.tiktok.com/@user/video/ID hai —
// wahi canonical video page jo proven self-scrape engine (ttSelfFetch,
// tiktok.go) direct download karta hai. Download path me koi API nahi:
// video page HTML → __UNIVERSAL_DATA_FOR_REHYDRATION__ JSON → playAddr → MP4.
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ── Brave HTML search ───────────────────────────────────────────────

var (
	ttBraveVideoRe = regexp.MustCompile(
		`https://www\.tiktok\.com/(@[a-zA-Z0-9_.]+/video/[0-9]+)`)

	// ttBraveBadges — result cards in HTML (search.brave.com)
	ttBraveResultRe = regexp.MustCompile(`(?s)<div class="snippet[^"]*"[^>]*>.*?</div>`)
)

const (
	ttBraveUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	ttBraveHost  = "https://search.brave.com/search?q="
	ttMaxEnrich  = 30
	ttEnrichTTL  = 10 * time.Minute
)

// ttBraveSearch — one Brave HTML query, returns unique video paths.
func ttBraveSearch(ctx context.Context, query string) ([]string, error) {
	u := ttBraveHost + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ttBraveUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	res, err := searchHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("brave HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	matches := ttBraveVideoRe.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no tiktok video links in brave results")
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		path := m[1]
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out, nil
}

// ── TikTok oembed (title/author enrichment, keyless public) ────────

type ttOembedResp struct {
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	AuthorURL  string `json:"author_url"`
	Thumbnail  string `json:"thumbnail_url"`
}

// ttOembed — one video link ka title+author (nil on error).
func ttOembed(ctx context.Context, videoURL string) *ttOembedResp {
	u := "https://www.tiktok.com/oembed?url=" + url.QueryEscape(videoURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", ttMobileUA)
	res, err := searchHTTP.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil
	}
	var o ttOembedResp
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&o); err != nil {
		return nil
	}
	if o.AuthorURL != "" {
		o.AuthorName = o.AuthorName + " " + o.AuthorURL
	}
	return &o
}

// ── main entry: search + enrich ─────────────────────────────────────

// ttVideoSearch — REAL TikTok videos via Brave HTML + oembed enrichment.
// Returns a list capped at 30; no tikwm, no API keys.
func ttVideoSearch(ctx context.Context, query string) ([]searchResult, error) {
	// multiple query variations — har variation naye videos deti hai
	variations := []string{
		"site:tiktok.com " + query,
		query + " tiktok",
		"site:tiktok.com " + query + " videos",
		"site:tiktok.com " + query + " viral",
		"site:tiktok.com " + query + " compilation",
	}

	type found struct {
		path string
	}

	seen := map[string]bool{}
	var paths []string
	for _, v := range variations {
		if len(paths) >= ttMaxEnrich {
			break
		}
		foundPaths, err := ttBraveSearch(ctx, v)
		if err != nil {
			continue // ek variation fail ho to agli try
		}
		for _, p := range foundPaths {
			if seen[p] {
				continue
            }
			seen[p] = true
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no videos found")
	}
	if len(paths) > ttMaxEnrich {
		paths = paths[:ttMaxEnrich]
	}

	// oembed enrichment — parallel (10 workers)
	results := make([]searchResult, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			link := "https://www.tiktok.com/" + p
			r := searchResult{Link: link, Handle: p[:strings.Index(p, "/video")]}
			if o := ttOembed(ctx, link); o != nil {
				r.Title = o.Title
				if o.AuthorName != "" && !strings.HasPrefix(o.AuthorName, "@") {
					r.Handle = strings.TrimSpace(strings.Split(o.AuthorName, " ")[0])
					r.Handle = "@" + r.Handle
				}
			}
			if r.Title == "" {
				r.Title = "TikTok Video"
			}
			results[i] = r
		}(i, p)
	}
	wg.Wait()

	var out []searchResult
	for _, r := range results {
		if r.Link == "" {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
