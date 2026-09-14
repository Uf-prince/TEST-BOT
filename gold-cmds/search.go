package goldcmds

// ============================================================================
// GOLD-MD — Search Commands (Social + APK)
// File: search.go
// ============================================================================
// Six search commands in the SEARCH category, each with a hidden short alias
// (same pattern as ytsearch / yts):
//
//   ttsearch / tts   — TikTok user search       (tikwm user search API)
//   fbsearch / fbs   — Facebook profile search  (facebook.com/public via jina)
//   igsearch / igs   — Instagram search         (Bing RSS site:instagram.com)
//   tgsearch / tgs   — Telegram channels        (telegram-group.com via jina)
//   twtsearch / twts — X / Twitter search       (Bing RSS site:x.com)
//   apksearch / apks — APK app search           (apkcombo.com via jina)
//
// Every list follows the ytsearch card style:
// 🔰 header, QUERY :❱, numbered results, LINK :❱, download footer.
// ============================================================================

import (
	"unicode/utf8"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// searchResult is one row of every search list.
type searchResult struct {
	Title   string
	Handle  string // @user, package name, @channel...
	Stats   string // followers / rating / size...
	Link    string
	Snippet string
	// DurationSec is filled by the TikTok video search (SHORTS <= 60s
	// vs LONG > 60s split ke liye); other engines leave it zero.
	DurationSec int64
}

// searchMaxResults caps every search list (same as ytsearch).
const searchMaxResults = 5

// fbMaxResults — FB video search ki cap (owner: 15 results chahiye).
const fbMaxResults = 15

// twtMaxResults — X/Twitter search ki cap (owner: 15 results chahiye).
const twtMaxResults = 15

// apkMaxResults — APK search ki cap (owner: 15 results chahiye).
const apkMaxResults = 15

var searchHTTP = &http.Client{Timeout: 40 * time.Second}

// ── shared fetch helpers ────────────────────────────────────────────────

// jinaFetch reads a page through the r.jina.ai reader proxy and returns the
// markdown text. Needed for sites that block plain HTTP clients
// (facebook.com/public, apkcombo.com, telegram-group.com).
// The proxy rate-limits rapid calls with HTTP 403/429, so transient failures
// are retried with backoff (up to 3 attempts).
func jinaFetch(ctx context.Context, target string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt*5) * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		md, err := jinaFetchOnce(ctx, target)
		if err == nil {
			return md, nil
		}
		lastErr = err
		if !searchRetryable(err) {
			return "", err
		}
	}
	return "", lastErr
}

// searchRetryable reports whether a fetch error is worth retrying
// (rate-limit responses and transient network faults).
func searchRetryable(err error) bool {
	s := err.Error()
	if strings.Contains(s, "HTTP 403") || strings.Contains(s, "HTTP 429") || strings.Contains(s, "HTTP 5") {
		return true
	}
	if strings.Contains(s, "Client.Timeout") || strings.Contains(s, "context deadline") ||
		strings.Contains(s, "reset by peer") || strings.Contains(s, "EOF") {
		return true
	}
	return false
}

func jinaFetchOnce(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/"+target, nil)
	if err != nil {
		return "", err
	}
	// NB: the reader proxy rejects browser-like UAs with a challenge page,
	// but serves plain clients — a curl UA keeps it working.
	req.Header.Set("User-Agent", "curl/8.5.0")
	req.Header.Set("Accept", "text/plain")
	res, err := searchHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 3<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// bingRSS — Bing search through the RSS output (direct result links, no
// redirect decoding needed).
var (
	bingItemRe  = regexp.MustCompile(`(?s)<item>(.*?)</item>`)
	bingTitleRe = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	bingLinkRe  = regexp.MustCompile(`(?s)<link>(.*?)</link>`)
)

func bingRSS(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://www.bing.com/search?q=" + url.QueryEscape(query) + "&format=rss&count=15"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	res, err := searchHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	for _, it := range bingItemRe.FindAllStringSubmatch(string(data), -1) {
		block := it[1]
		var t, l string
		if m := bingTitleRe.FindStringSubmatch(block); m != nil {
			t = html.UnescapeString(strings.TrimSpace(m[1]))
		}
		if m := bingLinkRe.FindStringSubmatch(block); m != nil {
			l = strings.TrimSpace(html.UnescapeString(m[1]))
		}
		if l != "" && !strings.Contains(l, "bing.com") {
			out = append(out, searchResult{Title: t, Link: l})
		}
	}
	return out, nil
}

// searchFmtCount humanizes big numbers (2452278 -> 2.5M).
func searchFmtCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + "B"
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
	case n >= 1_000:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 1, 64) + "K"
	}
	return strconv.FormatInt(n, 10)
}

// searchCard renders the results reply (ytsearch card style).
// searchBorder is the fancy result border (same as .video / .play lists).
const searchBorder = "🔰════════•❁❀❁•════════🔰"

// searchCardPlatform maps a card header to the short platform name used in
// the "TYPE ❰ N ❯ TO DOWNLOAD THIS FROM <PLATFORM>" line.
func searchCardPlatform(header string) string {
	h := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(header), "SEARCH"))
	switch h {
	case "X / TWITTER":
		return "X"
	case "APK":
		return "APKCOMBO"
	default:
		return h
	}
}

// searchCard renders the full list in the .video fancy-border style.
func searchCard(header, query, handleLabel, statsLabel string, results []searchResult, footer string) string {
	var b strings.Builder
	b.WriteString("*🔰 " + header + " 🔰*\n\n")
	b.WriteString("*QUERY :❱ " + strings.ToUpper(query) + "*\n\n")
	b.WriteString("*TOP " + strconv.Itoa(len(results)) + " RESULTS FOR YOUR SEARCH*\n\n")
	for i, r := range results {
		b.WriteString(searchCardEntry(i, r, header, handleLabel, statsLabel))
	}
	b.WriteString(footer)
	return b.String()
}

// searchCardEntry renders ONE numbered result block. Refactored out of
// searchCard so the TikTok SHORTS/LONG card (30 entries, two sections)
// reuses the exact same entry layout with continuous 1-30 numbering.
func searchCardEntry(i int, r searchResult, header, handleLabel, statsLabel string) string {
	var b strings.Builder
	title := r.Title
	if title == "" {
		title = "NOT FOUND"
	}
	b.WriteString("\n" + searchBorder + "\n")
	b.WriteString("*TYPE ❰ " + strconv.Itoa(i+1) + " ❱ TO DOWNLOAD THIS FROM " + searchCardPlatform(header) + "*\n")
	b.WriteString(strings.ToUpper(title) + "\n")
	if handleLabel != "" && r.Handle != "" {
		b.WriteString("*" + handleLabel + " :❱ " + r.Handle + "*\n")
	}
	if statsLabel != "" && r.Stats != "" {
		b.WriteString("*" + statsLabel + " :❱ " + r.Stats + "*\n")
	}
	// DURATION line (owner round 2+3) - video searches only
	if r.DurationSec > 0 {
		b.WriteString("*DURATION :❱ " + ttFmtDuration(r.DurationSec) + "*\n")
	}
	if r.Snippet != "" {
		b.WriteString("*" + r.Snippet + "*\n")
	}
	b.WriteString("*LINK :❱ " + r.Link + "*\n")
	b.WriteString(searchBorder + "\n\n")
	return b.String()
}

// ttVideoCard renders the TikTok video results card: up to 15 SHORTS
// (<= 60s) then 15 LONG (> 60s) videos in two labeled sections with
// continuous 1-30 numbering (owner: '15 shorts videos ka link aye 15
// long videos ka link aye ... list total 30 videos ki bane ge').
func ttVideoCard(query string, results []searchResult) string {
	var b strings.Builder
	b.WriteString("*🔰 TIKTOK VIDEO SEARCH 🔰*\n\n")
	b.WriteString("*QUERY :❱ " + strings.ToUpper(query) + "*\n\n")
	b.WriteString("*TOP " + strconv.Itoa(len(results)) + " VIDEO RESULTS FOR YOUR SEARCH*\n")
	shorts, longs := 0, 0
	for _, r := range results {
		if r.DurationSec > 60 {
			longs++
		} else {
			shorts++
		}
	}
	if shorts > 0 {
		b.WriteString("*🔰 SHORTS ( < 1 MIN ) :❱ " + strconv.Itoa(shorts) + " RESULTS*\n")
	}
	if longs > 0 {
		b.WriteString("*🔰 LONG VIDEOS ( 2 MIN + ) :❱ " + strconv.Itoa(longs) + " RESULTS*\n")
	}
	b.WriteString("\n")
	if shorts > 0 {
		b.WriteString("*🔰 SHORTS ( < 1 MIN ) 🔰*\n\n")
	}
	for i, r := range results {
		// section switch: SHORTS ke baad LONG section ka header
		if i > 0 && r.DurationSec > 60 && results[i-1].DurationSec <= 60 {
			b.WriteString("\n*🔰 LONG VIDEOS ( 2 MIN + ) 🔰*\n\n")
		}
		b.WriteString(searchCardEntry(i, r, "TIKTOK SEARCH", "USER", "STATS"))
	}
	b.WriteString("*TYPE NUMBER WHICH RESULT YOU WANT TO DOWNLOAD ❰ REPLY WITH ANY NUMBER 1 TO " + strconv.Itoa(len(results)) + " ❱*")
	return b.String()
}

// searchNoResults — standard empty-list reply.
func searchNoResults(query string) string {
	return "*🔰 NO RESULTS FOUND 🔰*\n\n*QUERY :❱ " + strings.ToUpper(query) + "*\n\n*TRY A DIFFERENT OR SHORTER QUERY*"
}

// searchFailed — standard engine-error reply.
func searchFailed(name string) string {
	return "*🔰 SEARCH FAILED :❱*\n\n*" + name + " NOT RESPONDING — TRY AGAIN IN A FEW MINUTES*"
}

// ── TIKTOK ENGINE (tikwm user search API) ──────────────────────────────

type ttSearchResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserList []struct {
			User struct {
				UniqueId  string `json:"uniqueId"`
				Nickname  string `json:"nickname"`
				Signature string `json:"signature"`
			} `json:"user"`
			Stats struct {
				FollowerCount int64 `json:"followerCount"`
				VideoCount    int64 `json:"videoCount"`
			} `json:"stats"`
		} `json:"user_list"`
	} `json:"data"`
}

func ttUserSearch(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://www.tikwm.com/api/user/search?keywords=" + url.QueryEscape(query) + "&count=10"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	res, err := searchHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var parsed ttSearchResp
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("%s", parsed.Msg)
	}
	var out []searchResult
	for _, ul := range parsed.Data.UserList {
		title := ul.User.Nickname
		if title == "" {
			title = ul.User.UniqueId
		}
		if ul.User.UniqueId == "" {
			continue
		}
		stats := searchFmtCount(ul.Stats.FollowerCount) + " FOLLOWERS ❰ " + searchFmtCount(ul.Stats.VideoCount) + " VIDEOS ❯"
		out = append(out, searchResult{
			Title:  title,
			Handle: "@" + ul.User.UniqueId,
			Stats:  stats,
			Link:   "https://www.tiktok.com/@" + ul.User.UniqueId,
		})
	}
	return out, nil
}

// ── FACEBOOK ENGINE (facebook.com/public via jina) ──────────────────────

var fbProfileRe = regexp.MustCompile(`\[([^\]]+)\]\((https://www\.facebook\.com/people/[^)\s]+)\s+"[^"]*"\)`)

// fbSeePhotosRe - "See Photos" link numeric profile id carry karta hai
// (?id=NNN&sk=photos) aur iska format title quotes ke bina hota hai.
var fbSeePhotosRe = regexp.MustCompile(`\[See Photos\]\((https://www\.facebook\.com/people/[^)\s]+)\)`)

// fbProfileSearch - FB public directory search with AUTO-SHORTEN retry.
// FB directory lambi queries (5+ words) pe khali page deti hai
// ("We couldn't find anything for aja ve mahiya song lyrics") jabke
// chhoti versions pe results milte hain. Is liye: full query first;
// 0 results par last word drop kar ke retry (max 3 attempts).
func fbProfileSearch(ctx context.Context, query string) ([]searchResult, error) {
	words := strings.Fields(strings.TrimSpace(query))
	if len(words) == 0 {
		return nil, fmt.Errorf("empty query")
	}
	attempts := 0
	var lastErr error
	for n := len(words); n >= 1 && attempts < 3; n-- {
		attempts++
		results, err := fbProfileSearchOnce(ctx, strings.Join(words[:n], " "))
		if err != nil {
			lastErr = err
			continue
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

// fbProfileSearchOnce - ek query ka FB public directory search.
func fbProfileSearchOnce(ctx context.Context, query string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, "https://www.facebook.com/public/"+url.PathEscape(query))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	// v5: search page pe har profile 2 links ke saath aata hai:
	//   (a) naam wala plain link  -> [New Videos](.../pfbidXXX/ "New Videos")
	//   (b) [See Photos](.../pfbidXXX/?id=NNN&sk=photos)   (title quotes NAHI)
	// Purana fbProfileRe sirf (a) match karta tha - (b) me title quotes
	// nahi hote - is liye numeric ID kabhi save nahi hoti thi aur resolver
	// ka profile.php timeline route (jahan reels milte hain) try hi nahi
	// hota tha -> hamesha "PRIVATE PROFILE" error.
	// FIX: pehle "See Photos" links se base->id map banao, phir naam wale
	// links se entries banao aur id maujood ho to link me ?id= attach karo.
	idByBase := map[string]string{}
	for _, m := range fbSeePhotosRe.FindAllStringSubmatch(md, -1) {
		link := m[1]
		base := strings.SplitN(link, "?", 2)[0]
		if id := fbNumericIDFromLink(link); id != "" {
			if _, ok := idByBase[base]; !ok {
				idByBase[base] = id
			}
		}
	}
	byBase := map[string]bool{}
	for _, m := range fbProfileRe.FindAllStringSubmatch(md, -1) {
		name, link := strings.TrimSpace(m[1]), m[2]
		if name == "" || name == "See Photos" {
			continue
		}
		base := strings.SplitN(link, "?", 2)[0]
		if byBase[base] {
			continue
		}
		byBase[base] = true
		final := base
		if id, ok := idByBase[base]; ok {
			final = base + "?id=" + id
		}
		out = append(out, searchResult{Title: name, Link: final})
	}
	return out, nil
}

// ── INSTAGRAM ENGINE (Bing RSS site:instagram.com) ──────────────────────

var igHandleRe = regexp.MustCompile(`instagram\.com/(?:p|reel|reels|explore|stories)/|instagram\.com/([A-Za-z0-9_.]+)/?`)

func igAccountSearch(ctx context.Context, query string) ([]searchResult, error) {
	res, err := bingRSS(ctx, query+" site:instagram.com")
	if err != nil {
		return nil, err
	}
	var out []searchResult
	for _, r := range res {
		if !strings.Contains(r.Link, "instagram.com") {
			continue
		}
		// profile links only (skip posts / explore pages)
		if strings.Contains(r.Link, "/p/") || strings.Contains(r.Link, "/reel") || strings.Contains(r.Link, "/explore") || strings.Contains(r.Link, "/stories/") {
			continue
		}
		handle := ""
		if m := igHandleRe.FindStringSubmatch(r.Link); m != nil {
			handle = "@" + m[1]
		}
		out = append(out, searchResult{Title: r.Title, Handle: handle, Link: r.Link})
	}
	if len(out) == 0 {
		// fallback: plain keyword search, keep only instagram links
		res2, err2 := bingRSS(ctx, query+" instagram")
		if err2 != nil {
			return out, nil
		}
		for _, r := range res2 {
			if strings.Contains(r.Link, "instagram.com") && !strings.Contains(r.Link, "/p/") && !strings.Contains(r.Link, "/reel") {
				handle := ""
				if m := igHandleRe.FindStringSubmatch(r.Link); m != nil {
					handle = "@" + m[1]
				}
				out = append(out, searchResult{Title: r.Title, Handle: handle, Link: r.Link})
			}
		}
	}
	return out, nil
}

// ── TELEGRAM ENGINE (telegram-group.com via jina) ───────────────────────

// tgEntryRe — REAL search results: telegram-group.com search pages list
// every entry as a "## [Title](detail-page-url)" heading. Nav menu links
// are plain [..](..) links — the heading anchor keeps them out.
// (Fix: pehle plain-link regex tha -> "No Results" page pe nav links hi
// results ban jate the -> downloader ko telegram-group.com link milta tha
// -> "not a channel link" download error.)
var (
	tgEntryRe = regexp.MustCompile(`(?m)^## \[([^\]]+)\]\((https://telegram-group\.com/[^)\s]+)\)`)
	tgLinkRe  = regexp.MustCompile(`https?://t\.me/(joinchat/[A-Za-z0-9_-]+|[A-Za-z0-9_]+)`)
)

func tgChannelSearch(ctx context.Context, query string) ([]searchResult, error) {
	// EN site route: results/labels English me hote hain + stable layout.
	// (Hebrew default site pe layout same hai, EN preferred.)
	md, err := jinaFetch(ctx, "https://telegram-group.com/en/search/"+url.PathEscape(query))
	if err != nil {
		return nil, err
	}
	if n := len(tgEntryRe.FindAllStringSubmatch(md, -1)); n == 0 {
	}
	// collect channel/group entries from the search page
	type tgEntry struct {
		title string
		page  string
	}
	var entries []tgEntry
	seenPage := map[string]bool{}
	for _, m := range tgEntryRe.FindAllStringSubmatch(md, -1) {
		title, page := strings.TrimSpace(m[1]), m[2]
		if title == "" || seenPage[page] {
			continue
		}
		// detail pages only: /<lang>/<category>/<slug>/ pattern (3+ segments).
		// Nav pages (publish, blog, telegram-apps, categories, search, policy,
		// language roots, "#") blacklist.
		if !tgIsDetailPage(page) {
			continue
		}
		seenPage[page] = true
		entries = append(entries, tgEntry{title: title, page: page})
	}
	if len(entries) > 8 {
		entries = entries[:8]
	}
	// fetch the detail pages in parallel to resolve the t.me links
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []searchResult
		sem = make(chan struct{}, 3)
	)
	for _, e := range entries {
		wg.Add(1)
		go func(e tgEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			link := ""
			if d, derr := jinaFetch(ctx, e.page); derr == nil {
				if m := tgLinkRe.FindString(d); m != "" {
					link = strings.Replace(m, "http://t.me/", "https://t.me/", 1)
				}
			} else {
			}
			mu.Lock()
			if link == "" {
				// t.me resolve nahi hua -> result SKIP (pehle telegram-group.com
				// link as-is jata tha -> download "not a channel link" error).
			} else {
				out = append(out, searchResult{Title: e.title, Link: link})
			}
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	if len(out) == 0 {
		// multi-word query pe EN search aksar "no results" deta hai
		// (e.g. "dua and azkar") -> first word pe ek retry
		words := strings.Fields(query)
		if len(words) > 1 {
			first := strings.ToLower(words[0])
			if first != "and" && first != "or" && first != "the" {
				return tgChannelSearch(ctx, first)
			}
		}
	}
	return out, nil
}

// ── TWITTER / X ENGINE (Bing RSS site:x.com) ────────────────────────────

var twtHandleRe = regexp.MustCompile(`(?:twitter|x)\.com/([A-Za-z0-9_]+)(?:/[a-z]+)?/?$`)

func twtAccountSearch(ctx context.Context, query string) ([]searchResult, error) {
	// ENGINE v3 (owner rule: 15 results): ek hi DDG query sirf 5-6 unique
	// twitter links deti thi (isliye list me sirf 2 dikhte the) — ab 6
	// query variants PARALLEL fetch + merge hote hain (ronaldo: 16+
	// unique), aur 15 se kam pade to Bing RSS se top-up.
	if out := twtDDGSearchMulti(ctx, query); len(out) > 0 {
		if len(out) < twtMaxResults {
			if extra, err := twtBingSearch(ctx, query); err == nil {
				seen := map[string]bool{}
				for _, r := range out {
					seen[strings.ToLower(r.Link)] = true
				}
				for _, r := range extra {
					lk := strings.ToLower(r.Link)
					if seen[lk] {
						continue
					}
					seen[lk] = true
					out = append(out, r)
					if len(out) >= twtMaxResults {
						break
					}
				}
			}
		}
		return out, nil
	}
	return twtBingSearch(ctx, query)
}

// twtDDGSearchMulti — 6 DDG query variants (jina reader proxy ke through)
// parallel fetch karke ek merged, case-insensitive deduped list banata
// hai. Ek DDG page ~6 twitter links deta hai; mukhtalif variants mukhtalif
// accounts surface karte hain (ronaldo: 16+ unique).
func twtDDGSearchMulti(ctx context.Context, query string) []searchResult {
	variants := []string{
		query + " twitter",
		query + " x.com",
		query + " x.com profile",
		query + " twitter account",
		query + " official twitter",
		"\"" + query + "\" x.com",
	}
	pages := make([][]searchResult, len(variants))
	var wg sync.WaitGroup
	for i, v := range variants {
		wg.Add(1)
		go func(i int, v string) {
			defer wg.Done()
			if r, err := twtDDGSearch(ctx, v); err == nil {
				pages[i] = r
			}
		}(i, v)
	}
	wg.Wait()
	var out []searchResult
	seen := map[string]bool{}
	for _, rs := range pages {
		for _, r := range rs {
			lk := strings.ToLower(r.Link)
			if seen[lk] {
				continue
			}
			seen[lk] = true
			out = append(out, r)
		}
	}
	return out
}

var (
	// DDG-jina markdown heading: "## [Title](https://duckduckgo.com/l/?uddg=<enc>&rut=...)"
	twDDGHeadRe = regexp.MustCompile(`(?m)^#{1,3}\s*\[([^\]]+)\]\((https://duckduckgo\.com/l/\?uddg=[^)\s]+)\)`)
	twDDGUddgRe = regexp.MustCompile(`uddg=([^&\s)]+)`)
	// profile sub-paths jo clean profile link me trim hone chahiye
	twPathTrimRe = regexp.MustCompile(`^(https?://(?:[a-z]+\.)?(?:twitter|x)\.com/[A-Za-z0-9_]{1,15})(?:/(?:with_replies|media|photo|video|search|likes|highlights|articles|followers|following))+/?$`)
	twHandlePathRe = regexp.MustCompile(`(?:twitter|x)\.com/([A-Za-z0-9_]{1,15})(?:/status(?:es)?/\d+)?/?$`)
)

// twHandleFromLink pulls @handle out of any x/twitter link (profile or
// status — status links pe twtHandleRe khali return karta tha).
func twHandleFromLink(link string) string {
	if m := twHandlePathRe.FindStringSubmatch(strings.TrimRight(strings.TrimSpace(link), "/")); m != nil {
		return "@" + m[1]
	}
	return ""
}

// twtDDGSearch searches DuckDuckGo (via the jina reader proxy) for X /
// Twitter accounts and tweets. Every result link is normalized:
//   - /with_replies /media ... suffixes trimmed -> clean profile link
//   - query/fragment stripped
//   - non-x/twitter links (facebook, wiki ...) skipped
func twtDDGSearch(ctx context.Context, query string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, "https://html.duckduckgo.com/html/?q="+url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	seen := map[string]bool{}
	for _, m := range twDDGHeadRe.FindAllStringSubmatch(md, -1) {
		title := strings.TrimSpace(m[1])
		link := ""
		if u := twDDGUddgRe.FindStringSubmatch(m[2]); u != nil {
			if dec, derr := url.QueryUnescape(u[1]); derr == nil {
				link = strings.TrimSpace(dec)
			}
		}
		if link == "" || (twExtractTweetID(link) == "" && twHandleFromLink(link) == "") {
			continue
		}
		if i := strings.IndexAny(link, "?#"); i >= 0 {
			link = link[:i]
		}
		if t := twPathTrimRe.FindStringSubmatch(link); t != nil {
			link = t[1]
		}
		if seen[link] {
			continue
		}
		seen[link] = true
		title = utf8Safe(title)
		out = append(out, searchResult{Title: title, Handle: twHandleFromLink(link), Link: link})
	}
	return out, nil
}

func twtBingSearch(ctx context.Context, query string) ([]searchResult, error) {
	res, err := bingRSS(ctx, query+" site:twitter.com OR site:x.com")
	if err != nil {
		return nil, err
	}
	var out []searchResult
	for _, r := range res {
		if !strings.Contains(r.Link, "x.com") && !strings.Contains(r.Link, "twitter.com") {
			continue
		}
		handle := ""
		if m := twtHandleRe.FindStringSubmatch(strings.TrimRight(r.Link, "/")); m != nil {
			handle = "@" + m[1]
		}
		out = append(out, searchResult{Title: r.Title, Handle: handle, Link: r.Link})
	}
	if len(out) == 0 {
		res2, err2 := bingRSS(ctx, query+" twitter")
		if err2 != nil {
			return out, nil
		}
		for _, r := range res2 {
			if strings.Contains(r.Link, "x.com") || strings.Contains(r.Link, "twitter.com") {
				handle := ""
				if m := twtHandleRe.FindStringSubmatch(strings.TrimRight(r.Link, "/")); m != nil {
					handle = "@" + m[1]
				}
				out = append(out, searchResult{Title: r.Title, Handle: handle, Link: r.Link})
			}
		}
	}
	return out, nil
}

// ── APK ENGINE (apkcombo.com search via jina) ───────────────────────────

var (
	apkEntryRe = regexp.MustCompile(`\[!\[Image[^]]*\]\([^)]+\)\s*([^\]]+)\]\((https://apkcombo\.com/[a-z0-9-]+/([a-z0-9._]+))/\s+"([^"]+) APK"\)`)
	apkRateRe  = regexp.MustCompile(`(N/A|[0-9.]+)\s*🔰\s*([0-9.]+\s*[KMG]?B)`)
	apkDlRe    = regexp.MustCompile(`([0-9][0-9.,]*\s*[BMK])\+`)
)

func apkAppSearch(ctx context.Context, query string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, "https://apkcombo.com/search/"+url.PathEscape(query))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	seenPkg := map[string]bool{}
	for _, m := range apkEntryRe.FindAllStringSubmatch(md, -1) {
		meta, appURL, pkg, title := m[1], m[2], m[3], m[4]
		if title == "" || pkg == "" || seenPkg[pkg] {
			continue
		}
		seenPkg[pkg] = true
		stats := ""
		if r := apkRateRe.FindStringSubmatch(meta); r != nil {
			stats = strings.TrimSpace(r[1]) + " ❰ " + strings.TrimSpace(r[2]) + " ❯"
		}
		if d := apkDlRe.FindStringSubmatch(meta); d != nil {
			dl := strings.TrimSpace(d[1]) + "+"
			if stats != "" {
				stats = dl + " DOWNLOADS ❰ " + stats
			} else {
				stats = dl + " DOWNLOADS"
			}
		}
		out = append(out, searchResult{Title: title, Handle: pkg, Stats: stats, Link: appURL})
	}
	return out, nil
}

// ── guides ──────────────────────────────────────────────────────────────

func ttGuide(prefix string) string {
	return "*🔰 TIKTOK SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH TIKTOK USERS :❱*\n*" + prefix + "tt ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "tt carti*\n*SHOWS THE TOP TIKTOK USERS WITH NAME, FOLLOWERS AND LINK*\n\n" +
		"*❁ DIRECT TIKTOK LINK :❱*\n*" + prefix + "tt ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "tt https://www.tiktok.com/@user/video/1234567890*\n*PASTE A TIKTOK LINK AND THE VIDEO DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "tts ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "tt ❰ LINK ❯*"
}

func fbGuide(prefix string) string {
	return "*🔰 FACEBOOK SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH FACEBOOK PROFILES :❱*\n*" + prefix + "fb ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "fb mark zuckerberg*\n*SHOWS MATCHING FACEBOOK PROFILES WITH NAME AND LINK*\n\n" +
		"*❁ DIRECT FACEBOOK LINK :❱*\n*" + prefix + "fb ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "fb https://www.facebook.com/watch?v=1234567890*\n*PASTE A FACEBOOK VIDEO / REEL LINK AND IT DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "fbs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "fb ❰ LINK ❯*"
}

func igGuide(prefix string) string {
	return "*🔰 INSTAGRAM SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH INSTAGRAM ACCOUNTS :❱*\n*" + prefix + "ig ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "ig ronaldo*\n*SHOWS MATCHING INSTAGRAM ACCOUNTS WITH NAME AND LINK*\n\n" +
		"*❁ DIRECT INSTAGRAM LINK :❱*\n*" + prefix + "ig ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "ig https://www.instagram.com/reel/Cxxxxxxxx/*\n*PASTE AN INSTAGRAM POST / REEL LINK AND IT DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "igs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "ig ❰ LINK ❯*"
}

func tgGuide(prefix string) string {
	return "*🔰 TELEGRAM SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH TELEGRAM CHANNELS :❱*\n*" + prefix + "tg ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "tg movies*\n*SHOWS CHANNELS AND GROUPS WITH THEIR JOIN LINKS*\n\n" +
		"*❁ DIRECT TELEGRAM LINK :❱*\n*" + prefix + "tg ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "tg https://t.me/channelname/123*\n*PASTE A T.ME POST LINK AND IT DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "tgs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD POSTS :❱*\n*" + prefix + "tg ❰ POST LINK ❯*"
}

func twtGuide(prefix string) string {
	return "*🔰 X / TWITTER SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH X ACCOUNTS :❱*\n*" + prefix + "twt ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "twt elon musk*\n*SHOWS MATCHING X / TWITTER ACCOUNTS WITH NAME AND LINK*\n\n" +
		"*❁ DIRECT X LINK :❱*\n*" + prefix + "twt ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "twt https://x.com/username/status/1234567890*\n*PASTE AN X / TWITTER LINK (TWEET OR PROFILE) AND IT DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "twts ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "twt ❰ LINK ❯*"
}

func apkGuide(prefix string) string {
	return "*🔰 APK SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH APK APPS :❱*\n*" + prefix + "apk ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "apk whatsapp*\n*SHOWS APPS WITH NAME, PACKAGE, DOWNLOADS, RATING AND SIZE*\n\n" +
		"*❁ DIRECT APK LINK :❱*\n*" + prefix + "apk ❰ LINK ❱*\n*EXAMPLE :❱ " + prefix + "apk https://apkcombo.com/whatsapp/com.whatsapp/*\n*PASTE AN APK LINK AND THE APK DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "apks ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "apk ❰ NUMBER OR NAME ❯*"
}

// ── handlers ────────────────────────────────────────────────────────────

// handleTTSearch — TikTok user search (menu entry).
func handleTTSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, ttGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickTT, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		// REAL VIDEO SEARCH (owner: "jab tak asal video ka link nai aye ga
		// to error hi bheje ga na bot") — feed/search/ asli videos lauta
		// hai (tiktok.com/@user/video/ID links), accounts nahi. Purane
		// user-search results profile links the jo pick pe fail hote the.
		waitID := s.ReplyWithID(info, "*SEARCHING TIKTOK VIDEOS....*")
		// video-first: 15 SHORTS + 15 LONG (commit 970d0e0); user-search
		// fallback only when the video engine is down/empty.
		results, err := ttVideoSearch(ctx, query)
		if err == nil {
			results = filterTTResults(results)
		}
		if err == nil && len(results) > 0 {
			s.DeleteMessage(info, waitID)
			setSearchSession(info.Sender.String(), pickTT, query, results)
			s.Reply(info, ttVideoCard(query, results))
			return
		}
		s.DeleteMessage(info, waitID)
		results, err = ttUserSearch(ctx, query)
		if err != nil {
			s.Reply(info, searchFailed("TIKTOK"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickTT, query, results)
		s.Reply(info, searchCard("TIKTOK SEARCH", query, "USER", "STATS", results,
			searchPickFooter()))
	})
}

// handleFBSearch — Facebook profile search (menu entry).
func handleFBSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, fbGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickFB, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*SEARCHING FACEBOOK....*")
		results, err := fbProfileSearch(ctx, query)
		s.DeleteMessage(info, waitID)
		if err != nil {
			s.Reply(info, searchFailed("FACEBOOK"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickFB, query, results)
		s.Reply(info, searchCard("FACEBOOK SEARCH", query, "", "", results,
			searchPickFooter()))
	})
}

// handleFBSearchV7 — FB video search (DDG-lite video links first, v7).
func handleFBSearchV7(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, fbGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickFB, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		// owner rule: query pe waiting msg — search hote hi auto-delete
		// (success card / no-results — sab paths pe).
		waitID := s.ReplyWithID(info, "*SEARCHING FACEBOOK....*")
		// v9: FB video search — watch search (primary) + DDG-lite (backup)
		// merge karke 15 tak results. Profile-directory sirf tab jab video
		// search khaali ho (private/personal profiles ke liye).
		var merged []searchResult
		seen := map[string]bool{}
		add := func(rs []searchResult) {
			for _, r := range rs {
				if seen[r.Link] {
					continue
				}
				seen[r.Link] = true
				merged = append(merged, r)
				if len(merged) >= fbMaxResults {
					return
				}
			}
		}

		// 1) FB ka apna watch search (best: public reels, titles, no limit)
		watch, err := fbWatchSearch(ctx, query)
		if err == nil {
			add(watch)
		}
		// 2) DDG-lite backup (jab watch khaali / kam results)
		if len(merged) < fbMaxResults {
			ddg, err2 := ddgFBVideoSearch(ctx, query)
			if err2 == nil {
				add(ddg)
			}
		}

		// 3) video list choti ho to profile-directory se bharo (15 tak)
		if len(merged) < fbMaxResults {
			prof, errP := fbProfileSearch(ctx, query)
			if errP == nil {
				add(prof)
			}
		}

		// owner rule: junk-title video entries ke naam reel pages se bharo
		fbEnrichVideoTitles(ctx, merged)

		s.DeleteMessage(info, waitID)
		if len(merged) > 0 {
			setSearchSession(info.Sender.String(), pickFB, query, merged)
			s.Reply(info, searchCard("FACEBOOK VIDEO SEARCH", query, "VIDEO", "", merged,
				searchPickFooterN(len(merged))))
			return
		}

		s.Reply(info, searchNoResults(query))
	})
}

// fbWatchSearch — FB ka apna watch search (jina ke through).
// facebook.com/watch/search/?q=QUERY public hai aur public reels/videos
// ke direct permalinks + titles deta hai (koi login nahi, no rate-limit).
func fbWatchSearch(ctx context.Context, query string) ([]searchResult, error) {
	// ENGINE v2 (owner rule: name + duration + valid links): m.facebook
	// watch search PRIMARY — www wala kuch queries pe login wall de deta
	// tha (funny/nasheed -> 0 results), m har tested query pe reels deta
	// hai. m kam de to www se merge. Purana www-only fetch REMOVE (naya
	// endpoint kaam kar gaya).
	q := url.PathEscape(query)
	out, mErr := fbWatchFetch(ctx, "https://m.facebook.com/watch/search/?q="+q)
	if mErr == nil && len(out) >= 4 {
		return out, nil
	}
	if extra, wErr := fbWatchFetch(ctx, "https://www.facebook.com/watch/search/?q="+q); wErr == nil {
		seen := map[string]bool{}
		for _, r := range out {
			seen[r.Link] = true
		}
		for _, r := range extra {
			if seen[r.Link] {
				continue
			}
			seen[r.Link] = true
			out = append(out, r)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if mErr != nil {
		return nil, mErr
	}
	return nil, fmt.Errorf("watch search empty")
}

// fbWatchFetch — ek watch-search page fetch + parse.
func fbWatchFetch(ctx context.Context, u string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, u)
	if err != nil {
		return nil, err
	}
	return fbWatchParse(md), nil
}

// fbWatchParse — watch-search markdown se name + duration + link entries.
func fbWatchParse(md string) []searchResult {
	// duration badges: [M:SS ![Image N](thumb)](reel-link)
	durs := map[string]int64{}
	for _, m := range fbWatchDurRe.FindAllStringSubmatch(md, -1) {
		link := fbNormLink(m[3])
		if _, ok := durs[link]; !ok {
			durs[link] = fbParseMMSS(m[1], m[2])
		}
	}
	// ## [Title](link) — asli video title (lambi captions bhi).
	titles := map[string]string{}
	for _, m := range fbWatchTitleRe.FindAllStringSubmatch(md, -1) {
		if t := strings.TrimSpace(m[1]); t != "" {
			titles[fbNormLink(m[2])] = t
		}
	}
	// entries: [text](reel-link) + badge-only links (unke nested image
	// brackets ki wajah se fbWatchRe me nahi aate — alag se add).
	type fbRawEnt struct {
		pos         int
		title, link string
	}
	var raws []fbRawEnt
	seen := map[string]bool{}
	for _, m := range fbWatchRe.FindAllStringSubmatchIndex(md, -1) {
		title, link := strings.TrimSpace(md[m[2]:m[3]]), fbNormLink(md[m[4]:m[5]])
		if seen[link] {
			continue
		}
		seen[link] = true
		raws = append(raws, fbRawEnt{m[0], title, link})
	}
	for _, m := range fbWatchDurRe.FindAllStringSubmatchIndex(md, -1) {
		link := fbNormLink(md[m[6]:m[7]])
		if seen[link] {
			continue
		}
		seen[link] = true
		raws = append(raws, fbRawEnt{m[0], "", link})
	}
	// page order me sort
	sort.Slice(raws, func(i, j int) bool { return raws[i].pos < raws[j].pos })
	// player duration (0:00 / M:SS) — jis entry ka badge nahi mila.
	for i := range raws {
		if durs[raws[i].link] > 0 {
			continue
		}
		end := len(md)
		if i+1 < len(raws) {
			end = raws[i+1].pos
		}
		for _, pl := range fbWatchPlayerRe.FindAllStringSubmatch(md[raws[i].pos:end], -1) {
			if pl[1] == "0" && pl[2] == "00" {
				if tot := fbParseMMSS(pl[3], pl[4]); tot > 0 {
					durs[raws[i].link] = tot
					break
				}
			}
		}
	}
	out := make([]searchResult, 0, len(raws))
	for _, r := range raws {
		title := r.title
		if t, ok := titles[r.link]; ok {
			title = t
		}
		if title == "" || fbDateLikeTitle(title) || fbRelAgeRe.MatchString(title) {
			title = "Facebook Video"
		}
		title = utf8Safe(title)
		out = append(out, searchResult{Title: title, Link: r.link, DurationSec: durs[r.link]})
	}
	return out
}

// fbReelIDRe — video permalink (reel / watch?v=) pe ID match.
var fbReelIDRe = regexp.MustCompile(`facebook\.com/(?:reel/|watch/\?v=)(\d{6,})`)

// fbEnrichVideoTitles — junk-title video entries (watch page date-only
// ya DDG fallback) ke liye reel page ka jina title fetch karke asli video
// naam nikaalta hai: "<stats> | <NAME> | <AUTHOR>". Parallel goroutines
// (har entry ~1s), sirf video permalinks pe, max fbMaxResults.
func fbEnrichVideoTitles(ctx context.Context, rs []searchResult) {
	type job struct {
		idx  int
		link string
	}
	var jobs []job
	for i, r := range rs {
		if r.Title != "" && r.Title != "Facebook" && r.Title != "Facebook Video" {
			continue // asli naam already hai
		}
		if !fbReelIDRe.MatchString(r.Link) {
			continue // profile links enrich nahi hote
		}
		jobs = append(jobs, job{i, r.Link})
		if len(jobs) >= fbMaxResults {
			break
		}
	}
	if len(jobs) == 0 {
		return
	}
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, j := range jobs {
		wg.Add(1)
		go func(idx int, link string) {
			defer wg.Done()
			md, err := jinaFetch(ctx, link)
			if err != nil {
				return
			}
			line := ""
			for _, l := range strings.Split(md, "\n") {
				if strings.HasPrefix(l, "Title: ") {
					line = strings.TrimSpace(strings.TrimPrefix(l, "Title: "))
					break
				}
			}
			if line == "" {
				return
			}
			// "<stats> | <NAME> | <AUTHOR>" — middle segment asli naam.
			parts := strings.Split(line, " | ")
			name := ""
			if len(parts) >= 3 {
				name = strings.TrimSpace(parts[1])
			} else if len(parts) == 2 {
				name = strings.TrimSpace(parts[1])
			}
			if name == "" {
				name = strings.TrimSpace(line)
			}
			if name == "" || len(name) < 3 {
				return
			}
			name = utf8Safe(name)
			mu.Lock()
			rs[idx].Title = name
			mu.Unlock()
		}(j.idx, j.link)
	}
	wg.Wait()
}

// fbNormLink — m.facebook link ko www form me (resolver/cobalt www
// permalinks se khelte hain).
func fbNormLink(l string) string {
	return strings.Replace(l, "https://m.facebook.com/", "https://www.facebook.com/", 1)
}

// fbParseMMSS — "M:SS" ko seconds me.
func fbParseMMSS(mm, ss string) int64 {
	m, _ := strconv.ParseInt(mm, 10, 64)
	s, _ := strconv.ParseInt(ss, 10, 64)
	return m*60 + s
}
// utf8Safe — 80 chars se lambe titles emoji ke beech se kaatne ke
// bajaye rune-aware truncate (broken UTF-8/WhatsApp render issue se bachata hai).
func utf8Safe(s string) string {
	if len(s) <= 80 {
		return s
	}
	t := s[:80]
	// 3 rune max — emoji boundary tak peeche jao
	for len(t) > 0 && !utf8.ValidString(t) {
		t = t[:len(t)-1]
	}
	if len(t) < 77 {
		t = s[:77]
		for len(t) > 0 && !utf8.ValidString(t) {
			t = t[:len(t)-1]
		}
	}
	return t + "..."
}

// fbDateLikeTitle — "February 19, 2024" / "August 20 at 4:11 AM" jaisi
// date titles ko pehchan kar generic title lagao.
func fbDateLikeTitle(t string) bool {
	months := []string{"January ", "February ", "March ", "April ", "May ", "June ",
		"July ", "August ", "September ", "October ", "November ", "December "}
	for _, mth := range months {
		if strings.HasPrefix(t, mth) {
			return true
		}
	}
	return false
}

// fbWatchRe — watch search page pe video titles reel/watch permalinks ke
// saath hote hain: ## [Title](reel-link) ya [date](reel-link).
var fbWatchRe = regexp.MustCompile(`\[([^\]]{2,200})\]\((https://(?:www|m)\.facebook\.com/(?:reel/[0-9]{6,}|watch/\?v=[0-9]{6,})/?)[^)]*\)`)

// fbWatchTitleRe — asli video title heading format me hota hai.
var fbWatchTitleRe = regexp.MustCompile(`## \[([^\]]+)\]\((https://(?:www|m)\.facebook\.com/(?:reel/[0-9]{6,}|watch/\?v=[0-9]{6,})/?)[^)]*\)`)

// fbWatchDurRe — duration badge: [M:SS ![Image N](thumb)](reel-link).
var fbWatchDurRe = regexp.MustCompile(`\[(\d{1,2}):(\d{2})\s+!\[Image[^\]]*\]\([^)]+\)\]\((https://(?:www|m)\.facebook\.com/(?:reel/[0-9]{6,}|watch/\?v=[0-9]{6,})/?)[^)]*\)`)

// fbWatchPlayerRe — video player bar "0:00 / M:SS" ("0:00 / 0:00"
// loading state skip hota hai — total 0 wale nahi lete).
var fbWatchPlayerRe = regexp.MustCompile(`(\d{1,2}):(\d{2})\s*/\s*(\d{1,2}):(\d{2})`)

// fbRelAgeRe — "2d" / "20h" jaise relative-age junk titles.
var fbRelAgeRe = regexp.MustCompile(`^[0-9]+[smhdwy]$`)

// ddgFBVideoSearch — DDG-lite se facebook.com ke direct video/reel links.
func ddgFBVideoSearch(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://lite.duckduckgo.com/lite/?q=" + url.QueryEscape(query+" site:facebook.com")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	res, err := searchHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	for _, m := range ddgLiteLinkRe.FindAllStringSubmatch(string(data), -1) {
		link, title := m[1], m[2]
		// uddg= param me actual facebook link URL-encoded hota hai
		if !strings.Contains(link, "uddg=") {
			continue
		}
		i := strings.Index(link, "uddg=")
		enc := link[i+5:]
		if j := strings.Index(enc, "&"); j >= 0 {
			enc = enc[:j]
		}
		dec, err := url.QueryUnescape(enc)
		if err != nil || dec == "" {
			continue
		}
		if !strings.Contains(dec, "facebook.com/") {
			continue
		}
		// mibextid jaise tracking params strip — clean permalink list me.
		if i := strings.IndexAny(dec, "?#"); i >= 0 {
			dec = dec[:i]
		}
		// sirf video/reel/watch permalinks (profiles/pages NAHI)
		if fbExtractPermalink(dec) == "" {
			continue
		}
		title = strings.TrimSpace(title)
		if title == "" {
			title = "Facebook Video"
		}
		// dedup
		dup := false
		for _, r := range out {
			if r.Link == dec {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, searchResult{Title: title, Link: dec})
		}
	}
	return out, nil
}

var ddgLiteLinkRe = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*class='result-link'[^>]*>(.*?)</a>`)

// handleIGSearch — Instagram account search (menu entry).
func handleIGSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, igGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickIG, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*SEARCHING INSTAGRAM....*")
		results, err := igEngineSearch(ctx, query)
		s.DeleteMessage(info, waitID)
		if err != nil {
			s.Reply(info, searchFailed("INSTAGRAM"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > igMaxResults {
			results = results[:igMaxResults]
		}
		setSearchSession(info.Sender.String(), pickIG, query, results)
		s.Reply(info, searchCard("INSTAGRAM SEARCH", query, "", "", results,
			searchPickFooterN(len(results))))
	})
}

// handleTGSearch — Telegram channel search (menu entry).
func handleTGSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, tgGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickTG, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*SEARCHING TELEGRAM....*")
		results, err := tgChannelSearch(ctx, query)
		s.DeleteMessage(info, waitID)
		if err != nil {
			s.Reply(info, searchFailed("TELEGRAM"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickTG, query, results)
		s.Reply(info, searchCard("TELEGRAM SEARCH", query, "", "", results,
			searchPickFooter()))
	})
}

// handleTWTSearch — X / Twitter account search (menu entry).
func handleTWTSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, twtGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickTWT, args, prefix) {
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		// owner rule: query pe waiting msg — search hote hi auto-delete
		// (success / error / no-results — sab paths pe).
		waitID := s.ReplyWithID(info, "*SEARCHING TWITTER....*")
		results, err := twtAccountSearch(ctx, query)
		s.DeleteMessage(info, waitID)
		if err != nil {
			s.Reply(info, searchFailed("X / TWITTER"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > twtMaxResults {
			results = results[:twtMaxResults]
		}
		setSearchSession(info.Sender.String(), pickTWT, query, results)
		s.Reply(info, searchCard("X / TWITTER SEARCH", query, "ACCOUNT", "", results,
			searchPickFooterN(len(results))))
	})
}

// handleAPKSearch — APK app search (menu entry).
func handleAPKSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, apkGuide(prefix))
		return
	}
	// direct link → instant download (no search list)
	if SearchDirectLink(s, info, pickAPK, args, prefix) {
		return
	}
	// bare package name → instant download
	if apkPkgRe.MatchString(strings.ToLower(query)) {
		searchPickAPK(s, info, searchResult{Title: query, Handle: strings.ToLower(query), Link: apkComboBase + "/app/" + strings.ToLower(query)})
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*SEARCHING APK....*")
		results, err := apkAppSearch(ctx, query)
		s.DeleteMessage(info, waitID)
		if err != nil {
			s.Reply(info, searchFailed("APK STORE"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > apkMaxResults {
			results = results[:apkMaxResults]
		}
		setSearchSession(info.Sender.String(), pickAPK, query, results)
		s.Reply(info, searchCard("APK SEARCH", query, "PACKAGE", "DETAILS", results,
			searchPickFooterN(len(results))))
	})
}

func init() {
	Register(Command{Name: "tt", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD TIKTOK VIDEOS WITHOUT WATERMARK. JUST SEND A TIKTOK LINK WITH THIS COMMAND.", Run: handleTTSearch})
	Register(Command{Name: "ttsearch", Hidden: true, Run: handleTTSearch})
	Register(Command{Name: "tts", Hidden: true, Run: handleTTSearch})
	Register(Command{Name: "tiktok", Hidden: true, Run: handleTTSearch})
	Register(Command{Name: "ttdl", Hidden: true, Run: handleTTSearch})
	Register(Command{Name: "ttvideo", Hidden: true, Run: handleTTSearch})
	Register(Command{Name: "tiktokvideo", Hidden: true, Run: handleTTSearch})

	Register(Command{Name: "fb", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD FACEBOOK VIDEOS. JUST SEND A FACEBOOK VIDEO LINK WITH THIS COMMAND.", Run: handleFBSearchV7})
	Register(Command{Name: "fbsearch", Hidden: true, Run: handleFBSearchV7})
	Register(Command{Name: "fbs", Hidden: true, Run: handleFBSearchV7})
	Register(Command{Name: "fbdl", Hidden: true, Run: handleFBSearchV7})
	Register(Command{Name: "facebook", Hidden: true, Run: handleFBSearchV7})
	Register(Command{Name: "reel", Hidden: true, Run: handleFBSearchV7})

	Register(Command{Name: "ig", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD INSTAGRAM VIDEOS AND PHOTOS. JUST SEND AN INSTAGRAM LINK WITH THIS COMMAND.", Run: handleIGSearch})
	Register(Command{Name: "igsearch", Hidden: true, Run: handleIGSearch})
	Register(Command{Name: "igs", Hidden: true, Run: handleIGSearch})
	Register(Command{Name: "instagram", Hidden: true, Run: handleIGSearch})
	Register(Command{Name: "insta", Hidden: true, Run: handleIGSearch})
	Register(Command{Name: "instavideo", Hidden: true, Run: handleIGSearch})

	Register(Command{Name: "tg", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO SEARCH AND DOWNLOAD TELEGRAM VIDEOS. SEND A TELEGRAM LINK OR SEARCH BY NAME.", Run: handleTGSearch})
	Register(Command{Name: "tgsearch", Hidden: true, Run: handleTGSearch})
	Register(Command{Name: "tgs", Hidden: true, Run: handleTGSearch})
	Register(Command{Name: "telegram", Hidden: true, Run: handleTGSearch})
	Register(Command{Name: "tgdl", Hidden: true, Run: handleTGSearch})
	Register(Command{Name: "tgvid", Hidden: true, Run: handleTGSearch})

	Register(Command{Name: "twt", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD TWITTER VIDEOS. JUST SEND A TWEET LINK WITH THIS COMMAND.", Run: handleTWTSearch})
	Register(Command{Name: "twtsearch", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "twts", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "twitter", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "tweet", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "twdl", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "xvideo", Hidden: true, Run: handleTWTSearch})
	Register(Command{Name: "x", Hidden: true, Run: handleTWTSearch})

	// ══════════════════════════════════════════════════════════════════
	// OWNER ORDER (bandwidth bachao): .apk family DISABLED — pura comment.
	// host change karte waqt neeche wali 7 lines uncomment karni hai.
	// (apkdl.go ka code preserved hai — kuch delete NAHI hua.)
	// ══════════════════════════════════════════════════════════════════
	// Register(Command{Name: "apk", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO SEARCH AND DOWNLOAD ANDROID APPS. TYPE AN APP NAME TO SEARCH AND GET ITS APK FILE.", Run: handleAPKSearch})
	// Register(Command{Name: "apksearch", Hidden: true, Run: handleAPKSearch})
	// Register(Command{Name: "apks", Hidden: true, Run: handleAPKSearch})
	// Register(Command{Name: "apkdl", Hidden: true, Run: handleAPKSearch})
	// Register(Command{Name: "app", Hidden: true, Run: handleAPKSearch})
	// Register(Command{Name: "apps", Hidden: true, Run: handleAPKSearch})
	// Register(Command{Name: "application", Hidden: true, Run: handleAPKSearch})
}

// FBProfileSearchLive - exported wrapper for the live sandbox test binary.
func FBProfileSearchLive(ctx context.Context, query string) ([]searchResult, error) {
	return fbProfileSearch(ctx, query)
}

// BingRSSLive - exported wrapper for the live sandbox test binary.
func BingRSSLive(ctx context.Context, query string) ([]searchResult, error) {
	return bingRSS(ctx, query)
}

// DDGFBLive - exported wrapper for the live sandbox test binary.
func DDGFBLive(ctx context.Context, query string) ([]searchResult, error) {
	return ddgFBVideoSearch(ctx, query)
}

// FBExtractPermalinkLive - exported wrapper for the live sandbox test binary.
func FBExtractPermalinkLive(link string) string {
	return fbExtractPermalink(link)
}

// FBWatchLive - exported wrapper for the live sandbox test binary.
func FBWatchLive(ctx context.Context, query string) ([]searchResult, error) {
	return fbWatchSearch(ctx, query)
}

// tgIsDetailPage — telegram-group.com URL ek real channel/group detail page
// hai ya nav/category/lang page. Detail pattern: /<lang>/<category>/<slug>/
// (kam se kam 3 path segments + trailing slash).
func tgIsDetailPage(page string) bool {
	u := strings.ToLower(strings.TrimSpace(page))
	if !strings.Contains(u, "telegram-group.com/") {
		return false
	}
	// strip scheme + host
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	u = strings.TrimPrefix(u, "telegram-group.com/")
	u = strings.TrimSuffix(u, "/")
	if u == "" || strings.Contains(u, "#") || strings.Contains(u, "?") {
		return false
	}
	// static/resource + known nav pages
	for _, bad := range []string{"wp-content", "wp-admin", "wp-login", "feed", "/category/", "/tag/", "how-to-", "policy", "publish", "blog", "telegram-apps", "search"} {
		if strings.Contains(u, bad) {
			return false
		}
	}
	// 2-letter lang segment (en/de/no/sv/nl/ja) optional; detail = 3+ segments
	segs := strings.Split(u, "/")
	if len(segs) < 3 {
		return false
	}
	// agar pehla segment 2-letter lang nahi hai to kam-se-kam 2 aur chahiye —
	// Hebrew default site category/slug bhi detail hi hai
	return true
}
