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
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
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
}

// searchMaxResults caps every search list (same as ytsearch).
const searchMaxResults = 5

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
const searchBorder = "✧═══════════•❁❀❁•═══════════✧"

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
		if r.Snippet != "" {
			b.WriteString("*" + r.Snippet + "*\n")
		}
		b.WriteString("*LINK :❱ " + r.Link + "*\n")
		b.WriteString(searchBorder + "\n\n")
	}
	b.WriteString(footer)
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

func fbProfileSearch(ctx context.Context, query string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, "https://www.facebook.com/public/"+url.PathEscape(query))
	if err != nil {
		return nil, err
	}
	var out []searchResult
	seen := map[string]bool{}
	for _, m := range fbProfileRe.FindAllStringSubmatch(md, -1) {
		name, link := strings.TrimSpace(m[1]), m[2]
		base := strings.SplitN(link, "?", 2)[0]
		if name == "" || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, searchResult{Title: name, Link: base})
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

var (
	tgEntryRe = regexp.MustCompile(`\[([^\]]+)\]\((https://telegram-group\.com/[^)\s]+)\)`)
	tgLinkRe  = regexp.MustCompile(`https?://t\.me/(joinchat/[A-Za-z0-9_-]+|[A-Za-z0-9_]+)`)
)

func tgChannelSearch(ctx context.Context, query string) ([]searchResult, error) {
	md, err := jinaFetch(ctx, "https://www.telegram-group.com/search/"+url.PathEscape(query))
	if err != nil {
		return nil, err
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
		if strings.Contains(page, "wp-content") || strings.Contains(page, "/category/") ||
			strings.Contains(page, "/tag/") || strings.HasSuffix(page, "/search/") ||
			strings.HasSuffix(page, "telegram-group.com/") || strings.Contains(page, "/how-to-") {
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
		mu   sync.Mutex
		wg   sync.WaitGroup
		out  []searchResult
		sem  = make(chan struct{}, 3)
	)
	for _, e := range entries {
		wg.Add(1)
		go func(e tgEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			link := e.page
			if d, derr := jinaFetch(ctx, e.page); derr == nil {
				if m := tgLinkRe.FindString(d); m != "" {
					link = m
				}
			}
			mu.Lock()
			out = append(out, searchResult{Title: e.title, Link: link})
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return out, nil
}

// ── TWITTER / X ENGINE (Bing RSS site:x.com) ────────────────────────────

var twtHandleRe = regexp.MustCompile(`(?:twitter|x)\.com/([A-Za-z0-9_]+)(?:/[a-z]+)?/?$`)

func twtAccountSearch(ctx context.Context, query string) ([]searchResult, error) {
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
	apkRateRe  = regexp.MustCompile(`(N/A|[0-9.]+)\s*★\s*([0-9.]+\s*[KMG]?B)`)
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
		"*🔰 SEARCH TIKTOK USERS :❱*\n*" + prefix + "ttsearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "ttsearch carti*\n*SHOWS THE TOP TIKTOK USERS WITH NAME, FOLLOWERS AND LINK*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "tts ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "tiktok ❰ LINK ❯*"
}

func fbGuide(prefix string) string {
	return "*🔰 FACEBOOK SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH FACEBOOK PROFILES :❱*\n*" + prefix + "fbsearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "fbsearch mark zuckerberg*\n*SHOWS MATCHING FACEBOOK PROFILES WITH NAME AND LINK*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "fbs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "fb ❰ LINK ❯*"
}

func igGuide(prefix string) string {
	return "*🔰 INSTAGRAM SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH INSTAGRAM ACCOUNTS :❱*\n*" + prefix + "igsearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "igsearch ronaldo*\n*SHOWS MATCHING INSTAGRAM ACCOUNTS WITH NAME AND LINK*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "igs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "insta ❰ LINK ❯*"
}

func tgGuide(prefix string) string {
	return "*🔰 TELEGRAM SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH TELEGRAM CHANNELS :❱*\n*" + prefix + "tgsearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "tgsearch movies*\n*SHOWS CHANNELS AND GROUPS WITH THEIR JOIN LINKS*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "tgs ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD POSTS :❱*\n*" + prefix + "tg ❰ POST LINK ❯*"
}

func twtGuide(prefix string) string {
	return "*🔰 X / TWITTER SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH X ACCOUNTS :❱*\n*" + prefix + "twtsearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "twtsearch elon musk*\n*SHOWS MATCHING X / TWITTER ACCOUNTS WITH NAME AND LINK*\n\n" +
		"*🔰 HIDDEN ALIAS :❱*\n*" + prefix + "twts ❰ QUERY ❯*\n\n" +
		"*🔰 TO DOWNLOAD :❱*\n*" + prefix + "twitter ❰ LINK ❯*"
}

func apkGuide(prefix string) string {
	return "*🔰 APK SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH APK APPS :❱*\n*" + prefix + "apksearch ❰ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "apksearch whatsapp*\n*SHOWS APPS WITH NAME, PACKAGE, DOWNLOADS, RATING AND SIZE*\n\n" +
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
	RunWithTimeout(s, info, func(ctx context.Context) {
		results, err := ttUserSearch(ctx, query)
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

// handleIGSearch — Instagram account search (menu entry).
func handleIGSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, igGuide(prefix))
		return
	}
	RunWithTimeout(s, info, func(ctx context.Context) {
		results, err := igAccountSearch(ctx, query)
		if err != nil {
			s.Reply(info, searchFailed("INSTAGRAM"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickIG, query, results)
		s.Reply(info, searchCard("INSTAGRAM SEARCH", query, "ACCOUNT", "", results,
			searchPickFooter()))
	})
}

// handleTGSearch — Telegram channel search (menu entry).
func handleTGSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, tgGuide(prefix))
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
	RunWithTimeout(s, info, func(ctx context.Context) {
		results, err := twtAccountSearch(ctx, query)
		if err != nil {
			s.Reply(info, searchFailed("X / TWITTER"))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickTWT, query, results)
		s.Reply(info, searchCard("X / TWITTER SEARCH", query, "ACCOUNT", "", results,
			searchPickFooter()))
	})
}

// handleAPKSearch — APK app search (menu entry).
func handleAPKSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, apkGuide(prefix))
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
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), pickAPK, query, results)
		s.Reply(info, searchCard("APK SEARCH", query, "PACKAGE", "DETAILS", results,
			searchPickFooter()))
	})
}

func init() {
	Register(Command{Name: "ttsearch", Category: "SEARCH", Desc: "Search TikTok users (name, followers, videos, link)", Run: handleTTSearch})
	Register(Command{Name: "tts", Hidden: true, Run: handleTTSearch})

	Register(Command{Name: "fbsearch", Category: "SEARCH", Desc: "Search Facebook profiles (name, link)", Run: handleFBSearch})
	Register(Command{Name: "fbs", Hidden: true, Run: handleFBSearch})

	Register(Command{Name: "igsearch", Category: "SEARCH", Desc: "Search Instagram accounts (name, link)", Run: handleIGSearch})
	Register(Command{Name: "igs", Hidden: true, Run: handleIGSearch})

	Register(Command{Name: "tgsearch", Category: "SEARCH", Desc: "Search Telegram channels and groups (name, join link)", Run: handleTGSearch})
	Register(Command{Name: "tgs", Hidden: true, Run: handleTGSearch})

	Register(Command{Name: "twtsearch", Category: "SEARCH", Desc: "Search X / Twitter accounts (name, link)", Run: handleTWTSearch})
	Register(Command{Name: "twts", Hidden: true, Run: handleTWTSearch})

	Register(Command{Name: "apksearch", Category: "SEARCH", Desc: "Search APK apps (name, package, rating, size)", Run: handleAPKSearch})
	Register(Command{Name: "apks", Hidden: true, Run: handleAPKSearch})
}
