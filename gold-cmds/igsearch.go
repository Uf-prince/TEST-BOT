package goldcmds

// ============================================================================
// GOLD-MD — Instagram Search Engine (V2 — hashtag reels + Bing profiles)
// File: igsearch.go
// ============================================================================
// ENGINE LAYERED (FB V9 pattern):
//   1) IG ka apna hashtag search (best): instagram.com/explore/tags/<slug>/
//      public hai (jina ke through) aur TOP REELS ke direct permalinks +
//      captions + like counts deta hai. Pick pe permalink seedha cobalt ko
//      milta hai -> instant video download.
//   2) Bing HTML via jina (u=a1 base64 decode): site:instagram.com profiles
//      + reels (IG-only postfilter).
//   3) Bing RSS site:instagram.com (backup profiles).
// Merge, dedupe, cap igMaxResults=15.
// ============================================================================

import (
	"context"
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
)

const igMaxResults = 15

// igSlugFromQuery — "funny cats videos" -> "funnycatsvideos".
func igSlugFromQuery(query string) string {
	s := strings.ToLower(strings.TrimSpace(query))
	s = strings.Join(strings.Fields(s), "")
	// strip common filler words
	for _, w := range []string{"videos", "video", "reels", "reel", "posts", "account", "accounts", "page", "official"} {
		if s != w && strings.HasSuffix(s, w) && len(s) > len(w) {
			s = strings.TrimSuffix(s, w)
			break
		}
	}
	if s == "" {
		s = strings.ToLower(strings.Join(strings.Fields(query), ""))
	}
	return s
}

// igHashtagSearch — instagram.com/explore/tags/<slug>/ via jina.
// Pattern (jina markdown):
//   [![Image N: CAPTION](scontent...jpg) LIKES CAPTION](https://www.instagram.com/reel/SHORTCODE/)
var igTagEntryRe = regexp.MustCompile(
	`\[!\[Image \d+:[^\]]*\]\((https://scontent[^)]+)\)\s*([\d.,]+[KM]?)\s+([^\]]*)\]\((https://www\.instagram\.com/(?:reel|p)/([A-Za-z0-9_-]+)/)`)

var igTagPermaRe = regexp.MustCompile(`https://www\.instagram\.com/(?:reel|p)/([A-Za-z0-9_-]{5,15})/`)

func igHashtagSearch(ctx context.Context, query string) ([]searchResult, error) {
	slug := igSlugFromQuery(query)
	if slug == "" {
		return nil, nil
	}
	md, err := jinaFetch(ctx, "https://www.instagram.com/explore/tags/"+url.PathEscape(slug)+"/")
	if err != nil {
		return nil, err
	}
	var out []searchResult
	seen := map[string]bool{}
	// full entries (thumbnail + likes + caption)
	for _, m := range igTagEntryRe.FindAllStringSubmatch(md, -1) {
		likes, caption, link, sc := m[2], strings.TrimSpace(m[3]), m[4], m[5]
		if sc == "" || seen[sc] {
			continue
		}
		seen[sc] = true
		title := igCleanCaption(caption)
		if title == "" {
			title = "Instagram Reel " + sc
		}
		out = append(out, searchResult{
			Title: title,
			Stats: likes + " likes",
			Link:  link,
		})
		if len(out) >= igMaxResults {
			return out, nil
		}
	}
	// bare permalinks (page par grid entries jo caption block me nahi aayi)
	if len(out) < 6 {
		for _, m := range igTagPermaRe.FindAllStringSubmatch(md, -1) {
			sc, link := m[1], "https://www.instagram.com/reel/"+m[1]+"/"
			if sc == "" || seen[sc] {
				continue
			}
			seen[sc] = true
			out = append(out, searchResult{Title: "Instagram Reel " + sc, Link: link})
			if len(out) >= igMaxResults {
				return out, nil
			}
		}
	}
	return out, nil
}

// igCleanCaption — caption se hashtags hatao, pehli line/title banao.
func igCleanCaption(c string) string {
	c = strings.TrimSpace(c)
	if i := strings.Index(c, "\n"); i > 0 {
		c = c[:i]
	}
	// drop hashtags
	fields := strings.Fields(c)
	var keep []string
	for _, f := range fields {
		if strings.HasPrefix(f, "#") {
			continue
		}
		keep = append(keep, f)
	}
	c = strings.Join(keep, " ")
	if len(c) > 90 {
		c = c[:87] + "..."
	}
	return c
}

// igBingDecodeRe — Bing ck/a redirect links in jina markdown.
var igBingLinkRe = regexp.MustCompile(`\[([^\]]*)\]\((https://www\.bing\.com/ck/a\?![^)]+)\)`)

// igBingHTMLSearch — Bing HTML results via jina; decode u=a1 base64 and keep
// instagram.com links only (Bing sometimes ignores site: — postfilter fixes).
func igBingHTMLSearch(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://www.bing.com/search?q=" + url.QueryEscape(query+" site:instagram.com") + "&count=25"
	md, err := jinaFetch(ctx, u)
	if err != nil {
		return nil, err
	}
	if len(md) < 3000 {
		return nil, nil // empty/challenge page
	}
	var out []searchResult
	for _, m := range igBingLinkRe.FindAllStringSubmatch(md, -1) {
		title, bingLink := strings.TrimSpace(m[1]), m[2]
		real := igDecodeBingLink(bingLink)
		if real == "" || !strings.Contains(real, "instagram.com") {
			continue
		}
		res, ok := igClassifyLink(title, real)
		if !ok {
			continue
		}
		out = append(out, res)
		if len(out) >= igMaxResults {
			break
		}
	}
	return out, nil
}

// igDecodeBingLink — bing.com/ck/a?!...&u=a1<base64url> -> real URL.
func igDecodeBingLink(bingLink string) string {
	q, err := url.ParseQuery(urlFromBing(bingLink))
	if err != nil {
		return ""
	}
	u := q.Get("u")
	if !strings.HasPrefix(u, "a1") {
		return ""
	}
	raw := strings.TrimPrefix(u, "a1")
	if pad := len(raw) % 4; pad != 0 {
		raw += strings.Repeat("=", 4-pad)
	}
	dec, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}
	return string(dec)
}

// urlFromBing extracts the query string from a full bing ck/a URL.
func urlFromBing(bingLink string) string {
	if i := strings.Index(bingLink, "?"); i >= 0 {
		return bingLink[i+1:]
	}
	return ""
}

// igClassifyLink — instagram URL ko normalize karta hai:
//   - reel/p/tv permalink  -> as-is (video pick, direct cobalt)
//   - profile              -> clean https://www.instagram.com/<user>/
//   - explore/accounts/... -> skip
func igClassifyLink(title, link string) (searchResult, bool) {
	// NOTE: shortcode CASE-SENSITIVE hai — original link parse karo,
	// lowercase sirf comparison ke liye.
	orig := strings.TrimSpace(link)
	orig = strings.TrimPrefix(strings.TrimPrefix(orig, "https://"), "http://")
	orig = strings.TrimPrefix(orig, "www.")
	orig = strings.TrimSuffix(orig, "/")
	parts := strings.SplitN(orig, "/", 3)
	host := strings.ToLower(parts[0])
	if host != "instagram.com" && host != "instagr.am" {
		return searchResult{}, false
	}
	first := ""
	if len(parts) > 1 {
		first = strings.SplitN(parts[1], "?", 2)[0]
	}
	if first == "" {
		return searchResult{}, false
	}
	switch strings.ToLower(first) {
	case "p", "reel", "reels", "tv":
		// shortcode is the next path segment (drop query)
		sc := ""
		if len(parts) > 2 {
			sc = strings.SplitN(parts[2], "?", 2)[0]
		}
		if sc == "" {
			return searchResult{}, false
		}
		return searchResult{Title: title, Link: "https://www.instagram.com/reel/" + sc + "/"}, true
	case "explore", "stories", "accounts", "directory", "about", "legal", "developer":
		return searchResult{}, false
	}
	// profile
	handle := "@" + first
	return searchResult{Title: title, Handle: handle, Link: "https://www.instagram.com/" + first + "/"}, true
}

// igBingRSSSearch — Bing RSS backup (profiles).
func igBingRSSSearch(ctx context.Context, query string) ([]searchResult, error) {
	res, err := bingRSS(ctx, query+" site:instagram.com")
	if err != nil {
		return nil, err
	}
	var out []searchResult
	for _, r := range res {
		if !strings.Contains(r.Link, "instagram.com") {
			continue
		}
		c, ok := igClassifyLink(r.Title, r.Link)
		if !ok {
			continue
		}
		out = append(out, c)
		if len(out) >= igMaxResults {
			break
		}
	}
	return out, nil
}

// igEngineSearch — merged multi-source IG search (FB V9 pattern).
//   1) hashtag reels (direct video permalinks)
//   2) bing HTML decoded profiles+reels
//   3) bing RSS profiles
func igEngineSearch(ctx context.Context, query string) ([]searchResult, error) {
	var merged []searchResult
	seen := map[string]bool{}
	add := func(rs []searchResult) {
		for _, r := range rs {
			if r.Link == "" || seen[r.Link] {
				continue
			}
			seen[r.Link] = true
			merged = append(merged, r)
			if len(merged) >= igMaxResults {
				return
			}
		}
	}

	// 1) hashtag reels (video-first, FB watch search jaisa)
	tags, errT := igHashtagSearch(ctx, query)
	if errT == nil {
		add(tags)
	}
	// 2) bing HTML decode (profiles + reels)
	if len(merged) < igMaxResults {
		bh, errB := igBingHTMLSearch(ctx, query)
		if errB == nil {
			add(bh)
		}
	}
	// 3) bing RSS backup
	if len(merged) < igMaxResults {
		br, errR := igBingRSSSearch(ctx, query)
		if errR == nil {
			add(br)
		}
	}
	return merged, nil
}
