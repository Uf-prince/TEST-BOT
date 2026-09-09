package goldcmds

// ============================================================================
// GOLD-MD — Search Pick System (number → instant download)
// File: searchpick.go
// ============================================================================
// Same UX as .play / .video: after a search card (ttsearch / fbsearch /
// igsearch / tgsearch / twtsearch / apksearch), the user types a bare number
// (e.g. "1", "2", "3") and the bot instantly acts on that result.
//
// Architecture (mirrors CompressTryHandle):
//
//   handler.go  ──1 line──>  SearchTryHandle(s, info, body, prefix)
//     ├─ no session / expired   → false (message flows on as a normal chat)
//     ├─ bare number ".1" / "1" → pick action + return true
//     └─ anything else          → false (session lives on, 2-min expiry)
//
// Pick actions per engine:
//
//   APK → apkcombo via jina (direct apkcombo.com hits get CF 403 from the
//         sandbox, the reader proxy bypasses it) → decode R2 signed URL →
//         stream download → send as document. Same route also un-breaks the
//         classic .apk command when its direct resolve fails.
//   TG  → t.me/s/<channel> preview page → latest post link → reuse the .tg
//         embed downloader (video/photo + caption).
//   TT / FB / IG / TWT → every public profile-downloader route is
//         API-blocked (tikwm user/posts 403, cobalt rejects profiles,
//         Instagram login wall, nitter dead), so the pick sends an info
//         card with the direct link + downloader hint.
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── session store ─────────────────────────────────────────────────────────

// searchPickKind marks which engine produced the results.
type searchPickKind string

const (
	pickTT  searchPickKind = "tt"
	pickFB  searchPickKind = "fb"
	pickIG  searchPickKind = "ig"
	pickTG  searchPickKind = "tg"
	pickTWT searchPickKind = "twt"
	pickAPK searchPickKind = "apk"
)

// searchSession is one pending search-list pick window for one sender.
type searchSession struct {
	Results []searchResult
	Kind    searchPickKind
	Query   string
	Expiry  time.Time
}

var (
	searchSessMu   sync.Mutex
	searchSessions = map[string]*searchSession{}
	searchSessTTL  = 3 * time.Minute // 30-result cards need extra reading time
)

// setSearchSession stores a fresh pick window.
func setSearchSession(jid string, kind searchPickKind, query string, results []searchResult) {
	// a new platform search replaces any pending .yts pick windows
	clearYTSList(jid)
	clearYTSChoice(jid)
	searchSessMu.Lock()
	defer searchSessMu.Unlock()
	searchSessions[jid] = &searchSession{
		Results: results,
		Kind:    kind,
		Query:   query,
		Expiry:  time.Now().Add(searchSessTTL),
	}
}

// getSearchSession returns the live session or nil (expired entries die).
func getSearchSession(jid string) *searchSession {
	searchSessMu.Lock()
	defer searchSessMu.Unlock()
	sess, ok := searchSessions[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		delete(searchSessions, jid)
		return nil
	}
	return sess
}

// ClearSearchSession kills a pending pick window for a JID (bridge-callable).
func ClearSearchSession(jid string) { clearSearchSession(jid) }

// clearSearchSession kills a pending pick window.
func clearSearchSession(jid string) {
	searchSessMu.Lock()
	defer searchSessMu.Unlock()
	delete(searchSessions, jid)
}

// searchPickFooter is the pick-style footer shown under every search card.
// searchPickFooter is the pick-style footer shown under every search card.
func searchPickFooter() string {
	return "*TYPE NUMBER WHICH RESULT YOU WANT TO OPEN OR DOWNLOAD — REPLY WITH ANY NUMBER 1 TO 5*"
}

// searchPickFooterN - footer with the actual list size (15 wali FB list
// ke liye "1 TO 15" sahi dikhega).
func searchPickFooterN(n int) string {
	return "*TYPE NUMBER WHICH RESULT YOU WANT TO OPEN OR DOWNLOAD — REPLY WITH ANY NUMBER 1 TO " + strconv.Itoa(n) + "*"
}

// SearchTryHandle — call from the main message handler BEFORE dispatch
// (same slot as CompressTryHandle). Returns true when the message was
// consumed by a pending search pick.
func SearchTryHandle(s SessionBridge, info types.MessageInfo, body, prefix string) bool {
	sess := getSearchSession(info.Sender.String())
	if sess == nil {
		return false
	}

	// accept both "1" and ".1"
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, prefix) {
		trimmed = strings.TrimSpace(trimmed[len(prefix):])
	}
	var choice int
	if _, err := fmt.Sscanf(trimmed, "%d", &choice); err != nil || choice < 1 || choice > len(sess.Results) {
		// not a pick: the session stays alive (2-min expiry is the only way
		// it dies) and the message is NOT swallowed — commands still run.
		return false
	}

	selected := sess.Results[choice-1]
	clearSearchSession(info.Sender.String())

	switch sess.Kind {
	case pickAPK:
		searchPickAPK(s, info, selected)
	case pickTG:
		searchPickTG(s, info, selected)
	case pickTWT:
		// tweet link → direct download (fxtwitter API works on tweet links)
		if !searchPickTWTDirect(s, info, selected) {
			searchPickLinkCard(s, info, sess.Kind, sess.Query, selected, prefix)
		}
	case pickIG:
		// v2: permalink → cobalt direct | profile → web_profile_info;
		// fail pe baqi results bhi try (FB v6/v7 pattern)
		if !searchPickIGDirect(s, info, selected, sess.Results) {
			// IG-STYLE SHORT ERROR (FB jaisa — link card NAHI)
			s.Reply(info, "❌ *INSTAGRAM DOWNLOAD ERROR*\nMEDIA NOT AVAILABLE\nTRY ANOTHER RESULT OR A DIFFERENT SEARCH \U0001f917")
		}
	case pickFB:
		// profile → multi-route /videos tab → latest video permalink → cobalt
		// (private profile pe baqi results bhi try hote hain)
		if !searchPickFBDirect(s, info, selected, sess.Results) {
			// FB-STYLE SHORT ERROR (TT jaisa — copy-link guidance card NAHI)
			s.Reply(info, "❌ *FACEBOOK DOWNLOAD ERROR*\nPRIVATE PROFILE VIDEO NOT AVAILABLE\nTRY ANOTHER RESULT OR A PAGE / PUBLIC PROFILE 🤗")
		}
	default:
		// TT — try a direct download of the picked profile’s latest
		// video; on failure send a short error card (no copy-link guidance).
		if !searchPickTTDirect(s, info, selected) {
			s.Reply(info, "❌ *TIKTOK DOWNLOAD ERROR*\nTRY AGAIN LATER 🤗")
		}
	}
	return true
}

// ── direct-link router ──────────────────────────────────────────────────────

// searchLinkRe matches any http(s) URL anywhere in the args (the smart
// link-detector for every search command).
var searchLinkRe = regexp.MustCompile(`https?://[^\s]+`)

// searchPlatformDomains lists every platform the search commands know.
// The first matching domain decides which platform a pasted link is for.
var searchPlatformDomains = []struct {
	kind   searchPickKind
	domain string
}{
	{pickTT, "tiktok.com"},
	{pickTT, "vm.tiktok.com"},
	{pickTT, "vt.tiktok.com"},
	{pickFB, "facebook.com"},
	{pickFB, "fb.watch"},
	{pickFB, "fb.com"},
	{pickIG, "instagram.com"},
	{pickIG, "instagr.am"},
	{pickIG, "ddinstagram.com"},
	{pickTG, "t.me/"},
	{pickTG, "telegram.me"},
	{pickTG, "telegram.dog"},
	{pickTWT, "twitter.com"},
	{pickTWT, "x.com"},
	{pickAPK, "apkcombo.com"},
	{pickAPK, "apkpure.com"},
	{pickAPK, "apk.support"},
	{pickAPK, "apkmirror.com"},
}

// searchPlatformName returns the human platform name for error cards.
func searchPlatformName(kind searchPickKind) string {
	switch kind {
	case pickTT:
		return "TIKTOK"
	case pickFB:
		return "FACEBOOK"
	case pickIG:
		return "INSTAGRAM"
	case pickTG:
		return "TELEGRAM"
	case pickTWT:
		return "X / TWITTER"
	case pickAPK:
		return "APK"
	}
	return "SEARCH"
}

// valid example links shown in the wrong-link error cards
const (
	tiktokExampleLink = "https://www.tiktok.com/@user/video/1234567890"
	fbExampleLink     = "https://www.facebook.com/watch?v=1234567890"
	igExampleLink     = "https://www.instagram.com/reel/Cxxxxxxxx/"
	tgExampleLink     = "https://t.me/channelname/123"
	twtExampleLink    = "https://x.com/username/status/1234567890"
	apkExampleLink    = "https://apkcombo.com/whatsapp/com.whatsapp/"
)

// searchPlatformExample returns a valid example command for error cards.
func searchPlatformExample(kind searchPickKind, prefix string) string {
	switch kind {
	case pickTT:
		return prefix + "tt " + tiktokExampleLink
	case pickFB:
		return prefix + "fb " + fbExampleLink
	case pickIG:
		return prefix + "ig " + igExampleLink
	case pickTG:
		return prefix + "tg " + tgExampleLink
	case pickTWT:
		return prefix + "twt " + twtExampleLink
	case pickAPK:
		return prefix + "apk " + apkExampleLink
	}
	return prefix + "search <query>"
}

// searchWrongLinkCard is the error reply when the pasted link does not belong
// to this search command's platform. Same style as the user's example:
//
//	.apk <facebook link>  →  "GIVE ME THE VALID APK LINK" + EXAMPLE
func searchWrongLinkCard(kind searchPickKind, pastedDomain string, prefix string) string {
	name := searchPlatformName(kind)
	return "❌ *" + name + " SEARCH ERROR* 🔰\n\n" +
		"*GIVE ME THE VALID " + name + " LINK* ❗\n\n" +
		"*THIS LINK IS FROM :❱ " + strings.ToUpper(pastedDomain) + "*\n" +
		"*IT IS NOT A " + name + " LINK* 🙅\n\n" +
		"*EXAMPLE SAME LIKE THAT :❱*\n" +
		"*" + searchPlatformExample(kind, prefix) + "*\n\n" +
		"*SEARCHED BY GOLD-MD* 🔰"
}

// searchLinkExtractQuery reports the query when the pasted link is a
// search-page URL that carries the query itself (apkcombo.com/search/<query>,
// facebook.com/public/<query>). The handler then runs a normal search with it.
func searchLinkExtractQuery(link string) string {
	low := strings.ToLower(link)
	if m := regexp.MustCompile(`apkcombo\.com/search/([^/?#\s]+)`).FindStringSubmatch(low); m != nil {
		q := m[1]
		if q != "" && q != "search" {
			q = strings.ReplaceAll(q, "%20", " ")
			q = strings.ReplaceAll(q, "+", " ")
			return strings.TrimSpace(q)
		}
	}
	if m := regexp.MustCompile(`facebook\.com/public/([^/?#\s]+)`).FindStringSubmatch(low); m != nil {
		q := m[1]
		q = strings.ReplaceAll(q, "%20", " ")
		q = strings.ReplaceAll(q, "+", " ")
		return strings.TrimSpace(q)
	}
	return ""

}

// searchPickAPKDirect routes a pasted APK-store link straight to the
// apkcombo downloader (app page link → instant APK download).
func searchPickAPKDirect(s SessionBridge, info types.MessageInfo, raw string) {
	raw = strings.TrimSpace(raw)
	link := strings.TrimRight(raw, "/")
	pkg := apkPkgFromLink(link)
	if pkg == "" {
		// apkpure / apkmirror / apk.support links → search by slug word
		low := strings.ToLower(link)
		low = strings.TrimPrefix(strings.TrimPrefix(low, "https://"), "http://")
		parts := strings.SplitN(low, "/", 3)
		if len(parts) >= 2 && parts[1] != "" {
			pkg = parts[1]
		}
	}
	searchPickAPK(s, info, searchResult{Title: raw, Handle: pkg, Link: link})
}

// searchLinkHost extracts the lowercase hostname of a pasted link
// (https://VM.TikTok.com/x/ → vm.tiktok.com) — precise domain matching.
func searchLinkHost(link string) string {
	u := strings.ToLower(strings.TrimSpace(link))
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "@"); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.Index(u, ":"); i >= 0 {
		u = u[:i]
	}
	return u
}

// searchLinkDomainMatch reports whether a host equals a known domain or is a
// subdomain of it (www. / m. / vm. ... all match).
func searchLinkDomainMatch(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// runSelfSearch runs the normal search flow for a self-searching link
// (apkcombo.com/search/<query> pasted into .apksearch): extract the query
// and show the regular search card, exactly like a typed query.
func runSelfSearch(s SessionBridge, info types.MessageInfo, kind searchPickKind, query, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		var (
			results                                 []searchResult
			err                                     error
			header, handleLabel, statsLabel, failed string
		)
		switch kind {
		case pickTT:
			header, handleLabel, statsLabel, failed = "TIKTOK SEARCH", "USER", "STATS", "TIKTOK"
			// video-first: 15 SHORTS + 15 LONG (owner rule), user-search fallback
			results, err = ttVideoSearch(ctx, query)
			if err == nil && len(results) > 0 {
				setSearchSession(info.Sender.String(), kind, query, results)
				s.Reply(info, ttVideoCard(query, results))
				return
			}
			results, err = ttUserSearch(ctx, query)
		case pickFB:
			header, handleLabel, statsLabel, failed = "FACEBOOK SEARCH", "", "", "FACEBOOK"
			results, err = fbProfileSearch(ctx, query)
		case pickIG:
			header, handleLabel, statsLabel, failed = "INSTAGRAM SEARCH", "ACCOUNT", "", "INSTAGRAM"
			results, err = igAccountSearch(ctx, query)
		case pickTG:
			header, handleLabel, statsLabel, failed = "TELEGRAM SEARCH", "", "", "TELEGRAM"
			results, err = tgChannelSearch(ctx, query)
		case pickTWT:
			header, handleLabel, statsLabel, failed = "X / TWITTER SEARCH", "ACCOUNT", "", "X / TWITTER"
			results, err = twtAccountSearch(ctx, query)
		case pickAPK:
			header, handleLabel, statsLabel, failed = "APK SEARCH", "PACKAGE", "DETAILS", "APK STORE"
			results, err = apkAppSearch(ctx, query)
		}
		if err != nil {
			s.Reply(info, searchFailed(failed))
			return
		}
		if len(results) == 0 {
			s.Reply(info, searchNoResults(query))
			return
		}
		if len(results) > searchMaxResults {
			results = results[:searchMaxResults]
		}
		setSearchSession(info.Sender.String(), kind, query, results)
		s.Reply(info, searchCard(header, query, handleLabel, statsLabel, results, searchPickFooter()))
	})
}

// SearchDirectLink — call at the top of every search handler: when the args
// contain ANY link, the bot instantly checks what platform that link is from:
//
//   - correct platform link → skip the search list, route straight to the
//     platform downloader (instant download, .video-style UX)
//   - wrong-platform link   → instant "GIVE ME THE VALID X LINK" error card
//     with a correct example (the user's requested behaviour)
//   - self-searching link (apkcombo.com/search/<q>) → extract the query and
//     run the normal search flow with it
//
// Returns true when the message was consumed.
func SearchDirectLink(s SessionBridge, info types.MessageInfo, kind searchPickKind, args []string, prefix string) bool {
	joined := strings.TrimSpace(strings.Join(args, " "))
	if joined == "" {
		return false
	}
	link := searchLinkRe.FindString(joined)
	if link == "" {
		return false
	}

	// identify the platform of the pasted link (precise host matching)
	host := searchLinkHost(link)
	var pastedKind searchPickKind
	for _, p := range searchPlatformDomains {
		if searchLinkDomainMatch(host, p.domain) {
			pastedKind = p.kind
			break
		}
	}

	route := func(run func()) bool {
		clearSearchSession(info.Sender.String())
		run()
		return true
	}

	// unknown domain (random website) → wrong-link error card
	if pastedKind == "" {
		s.Reply(info, searchWrongLinkCard(kind, host, prefix))
		return true
	}

	// self-searching link of THIS platform → extract the query, run the
	// normal search flow (search card + number-pick, like a typed query)
	if pastedKind == kind {
		if q := searchLinkExtractQuery(link); q != "" {
			return route(func() { runSelfSearch(s, info, kind, q, prefix) })
		}
	}

	switch kind {
	case pickTT:
		if pastedKind == pickTT {
			return route(func() { handleTikTok(s, info, args, prefix) })
		}
	case pickFB:
		if pastedKind == pickFB {
			return route(func() { handleFB(s, info, args, prefix) })
		}
	case pickIG:
		if pastedKind == pickIG {
			return route(func() { handleInsta(s, info, args, prefix) })
		}
	case pickTG:
		if pastedKind == pickTG {
			return route(func() { handleTG(s, info, args, prefix) })
		}
	case pickTWT:
		if pastedKind == pickTWT {
			return route(func() { handleTwitter(s, info, args, prefix) })
		}
	case pickAPK:
		if pastedKind == pickAPK {
			return route(func() { searchPickAPKDirect(s, info, strings.Join(args, " ")) })
		}
	}

	// pasted link belongs to another platform → error card
	s.Reply(info, searchWrongLinkCard(kind, host, prefix))
	return true
}

// ── pick actions ──────────────────────────────────────────────────────────

// searchPickAPK resolves the app through the jina reader proxy (direct
// apkcombo.com hits get CF 403 from this host) and sends the file.
func searchPickAPK(s SessionBridge, info types.MessageInfo, selected searchResult) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*DOWNLOADING APK....*")

		app, err := apkSearchResolve(ctx, selected)
		if err != nil || app == nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *APK NOT FOUND*\nPlease try again later. 🤔")
			return
		}

		s.EditMessage(info, waitID, fmt.Sprintf("🔰 *DOWNLOADING THIS APK 🔰*%s %s (%s)...", app.Title, app.Version, app.Size))

		client := &http.Client{Timeout: 10 * time.Minute}
		path, err := streamDownloadToFile(ctx, client, app.FileURL, nil)
		if err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *APK DOWNLOAD ERROR*\nPlease try again. 🤔")
			return
		}
		defer removeTempFile(path)

		name := app.FileName
		if name == "" {
			name = app.Package + "_" + app.Version + ".apk"
		}
		caption := "🔰 *APK DOWNLOADED 🔰*\n" +
			"*" + app.Title + "*\n\n" +
			"🔰 *VERSION :* " + app.Version + "\n" +
			"🔰 *SIZE :* " + app.Size + "\n" +
			"🔰 *PACKAGE :* " + app.Package + "\n\n" +
			"*FOUND FROM APK SEARCH*"

		if err := s.SendDocumentFile(info, path, name, app.MimeType, caption); err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *APK SEND ERROR*\nFile could not be sent.")
			return
		}
		s.DeleteMessage(info, waitID)
	})
}

// searchPickTG scrapes the latest post off the t.me/s/<channel> preview page
// and reuses the .tg embed downloader for that post.
func searchPickTG(s SessionBridge, info types.MessageInfo, selected searchResult) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*DOWNLOADING TELEGRAM MEDIA....*")

		postURL, err := tgLatestPost(ctx, selected.Link)
		if err != nil || postURL == "" {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nNo public post found for this channel. 🤔")
			return
		}

		media, err := tgFetchPost(ctx, postURL)
		if err != nil || media.url == "" {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nMedia not found — is the post public? 🤔")
			return
		}

		s.EditMessage(info, waitID, "⬇️ *Downloading "+media.kind+"...*")

		client := mediaHTTPClient()
		path, err := streamDownloadToFile(ctx, client, media.url, nil)
		if err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ PLEASE TRY AGAIN 🤔")
			return
		}
		defer removeTempFile(path)

		title := media.text
		if title == "" {
			title = selected.Title
		}
		if len(title) > 120 {
			title = title[:120] + "..."
		}
		creator := media.name
		if creator == "" {
			creator = "@" + media.channel
		}
		caption := "🔰 *TELEGRAM VIDEO NAME* 🔰\n" +
			"*" + title + "*\n\n" +
			"🔰 *CREATOR :* " + creator + "\n"
		if media.views != "" {
			caption += "🔰 *VIEWS :* " + media.views + "\n"
		}
		caption += "\n*TELEGRAM VIDEO DOWNLOAD*"

		if media.kind == "photo" {
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				s.DeleteMessage(info, waitID)
				s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nPhoto could not be read.")
				return
			}
			if err := s.SendImage(info, data, caption); err != nil {
				s.DeleteMessage(info, waitID)
				s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nPhoto could not be sent.")
				return
			}
			s.DeleteMessage(info, waitID)
			return
		}

		secs, w, h := probeVideoMeta(path)
		if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nVideo could not be sent.")
			return
		}
		s.DeleteMessage(info, waitID)
	})
}

// searchPickTTDirect tries to download the picked TikTok profile’s latest
// video. Returns false when nothing downloadable was found (caller then
// sends a short error card — NO link-copy guidance).
func searchPickTTDirect(s SessionBridge, info types.MessageInfo, selected searchResult) bool {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "⏳ *DOWNLOADING TIKTOK VIDEO....*")

		fail := func() {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "❌ *TIKTOK DOWNLOAD ERROR*\nTRY AGAIN LATER 🤗")
		}

		link := strings.TrimSpace(selected.Link)
		if !strings.Contains(link, "tiktok.com/@") {
			fail()
			return
		}

		// VIDEO LINK (new search): /video/ID link seedha proven engine pe
		// jata hai — profile scrape ki zaroorat hi nahi. PROFILE LINK
		// (fallback user-search ka result): profile se latest video nikalne
		// ki koshish, fail pe short error card.
		videoURL := link
		if !strings.Contains(link, "/video/") {
			var perr error
			videoURL, perr = ttProfileLatestVideo(ctx, link)
			if perr != nil || videoURL == "" {
				fail()
				return
			}
		}

		res, err := ttSelfFetch(ctx, videoURL)
		if err != nil {
			res, err = tikwmFetchResult(ctx, videoURL)
		}
		if err != nil {
			fail()
			return
		}

		videoDL := firstNonEmpty(res.HDPlay, res.Play, res.WMPlay)
		if videoDL == "" {
			fail()
			return
		}

		s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

		client := res.SrcClient
		if client == nil {
			client = mediaHTTPClient()
		}
		path, err := ttStreamDownload(ctx, client, videoDL)
		if err != nil {
			fail()
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
			creator = selected.Handle
		}
		caption := "🏆 TIKTOK VIDEO NAME 🏆\n" +
			"*" + title + "*\n\n" +
			"🏆 *CREATOR :* " + creator + "\n" +
			fmt.Sprintf("🏆 *TIME :* %ds\n", res.Duration) +
			fmt.Sprintf("🏆 *LIKES :* %d\n", res.DiggCount) +
			fmt.Sprintf("🏆 *COMMENTS :* %d\n", res.CommentCount) +
			fmt.Sprintf("🏆 *VIEWS :* %d\n\n", res.PlayCount) +
			"*TIKTOK VIDEO DOWNLOAD*"

		if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
			fail()
			return
		}
		s.DeleteMessage(info, waitID)
	})
	return true
}

// ttProfileLatestVideo scrapes a TikTok profile page (mobile UA + cookies)
// and returns the first /@user/video/<id> link found in the HTML.
func ttProfileLatestVideo(ctx context.Context, profileURL string) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ttMobileUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("profile page returned status %d", resp.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(`tiktok\.com/(@[A-Za-z0-9._]+)/video/([0-9]{15,})`).FindSubmatch(page)
	if m == nil {
		return "", fmt.Errorf("no video link found on profile page")
	}
	return "https://www.tiktok.com/" + string(m[1]) + "/video/" + string(m[2]), nil
}

// searchPickLinkCard is the fallback pick card for TT / FB / IG / TWT:
// big link + how-to-download hint (profile downloads are API-blocked).
func searchPickLinkCard(s SessionBridge, info types.MessageInfo, kind searchPickKind, query string, selected searchResult, prefix string) {
	var header, dlCmd, cmdHint string
	switch kind {
	case pickTT:
		header, dlCmd, cmdHint = "TIKTOK USER", prefix+"tt", "TIKTOK VIDEO LINK"
	case pickFB:
		header, dlCmd, cmdHint = "FACEBOOK PROFILE", prefix+"fb", "FACEBOOK VIDEO / REEL LINK"
	case pickIG:
		header, dlCmd, cmdHint = "INSTAGRAM ACCOUNT", prefix+"ig", "INSTAGRAM POST / REEL LINK"
	default:
		header, dlCmd, cmdHint = "X / TWITTER ACCOUNT", prefix+"twt", "X VIDEO LINK"
	}

	handle := selected.Handle
	if handle == "" {
		handle = selected.Link
	}
	stats := selected.Stats
	if stats != "" {
		stats = "❰ " + strings.Trim(stats, " ❰❯") + " ❯"
	} else {
		stats = "❰ " + query + " ❯"
	}

	card := "🔰 *PICKED FROM " + header + " SEARCH 🔰*\n\n" +
		"*" + selected.Title + "*\n" +
		"🔰 *HANDLE :❱ " + handle + "*\n" +
		"🔰 *STATS :❱ " + stats + "*\n\n" +
		"🔰 *OPEN :❱ " + selected.Link + "*\n\n" +
		"🔰 *TO DOWNLOAD A VIDEO :❱ COPY ANY " + cmdHint + "*\n" +
		"*THEN USE *" + dlCmd + " ❰ LINK ❯*\n\n" +
		"🔰 *SEARCHED BY GOLD-MD* 🔰"

	s.Reply(info, card)
}

// apkPkgFromLink pulls the package name out of an apkcombo.com URL:
// https://apkcombo.com/<slug>/<pkg>/  →  <pkg>
func apkPkgFromLink(link string) string {
	m := regexp.MustCompile(`apkcombo\.com/[a-z0-9-]+/([a-z0-9._]+)/?`).FindStringSubmatch(strings.ToLower(link))
	if m != nil {
		return m[1]
	}
	return ""
}

// ── APK resolver (jina route) ─────────────────────────────────────────────

var (
	// [![Image 1: icon](...)WhatsApp 2.26.34.81(263408100)XAPK 144 MB Android 6.0+](https://apkcombo.com/r2?u=...)
	apk2VerRe = regexp.MustCompile(`([0-9][0-9a-zA-Z.\-]*\([0-9]+\))[A-Z]+\s+([0-9]+(?:\.[0-9]+)?\s*[KMG]B)`)
	// Title: Download WhatsApp APK - Latest Version 2024 → app title
	apk2TitleRe = regexp.MustCompile(`(?m)^Title: Download (.+?) APK`)
	// (https://apkcombo.com/r2?u=<encoded>) — direct R2 redirect in markdown
	apk2R2Re = regexp.MustCompile(`\((https?://apkcombo\.com/r2\?u=[^)]+)\)`)
)

// apkSearchResolve turns an apksearch result (Link holds the correct
// apkcombo.com/<slug>/<pkg>/ page) into a downloadable app through the
// jina reader proxy. Returns an error when no R2 download link is present.
func apkSearchResolve(ctx context.Context, selected searchResult) (*apkAppInfo, error) {
	pkg := selected.Handle
	if pkg == "" {
		return nil, fmt.Errorf("no package in result")
	}
	pageURL := strings.TrimRight(selected.Link, "/")
	if !strings.Contains(pageURL, "apkcombo.com") {
		pageURL = apkComboBase + "/app/" + pkg
	}
	if !strings.HasSuffix(pageURL, "/download/apk") {
		if !strings.HasSuffix(pageURL, pkg) {
			pageURL += "/" + pkg
		}
		pageURL += "/download/apk"
	}

	md, err := jinaFetch(ctx, pageURL)
	if err != nil {
		return nil, err
	}

	app := &apkAppInfo{Package: pkg}
	// title: the "Title: Download <Name> APK" reader header line
	if m := apk2TitleRe.FindStringSubmatch(md); m != nil {
		app.Title = strings.TrimSpace(m[1])
	}
	if app.Title == "" {
		app.Title = pkg
	}
	// version + size: "2.26.34.81(263408100)XAPK 144 MB"
	if m := apk2VerRe.FindStringSubmatch(md); m != nil {
		app.Version = strings.TrimSpace(m[1])
		app.Size = strings.TrimSpace(m[2])
	}
	if app.Version == "" {
		app.Version = "latest"
	}
	if app.Size == "" {
		app.Size = "unknown"
	}
	// direct file url — the R2 redirect link in markdown parentheses
	if m := apk2R2Re.FindStringSubmatch(md); m != nil {
		if fu, derr := apkDecodeR2Link(m[1]); derr == nil {
			app.FileURL = fu
		}
	}
	if app.FileURL == "" {
		return nil, fmt.Errorf("no download link for %s", pkg)
	}
	app.FileName = apkFilenameFromURL(app.FileURL, pkg, app.Version)
	app.MimeType = apkMimeForName(app.FileName)
	return app, nil
}

// ── TG latest post scraper ────────────────────────────────────────────────

// tgPostRe matches post links on a channel preview page:
// href="https://t.me/<channel>/<id>" — the LAST match is the newest post.
var tgPostRe = regexp.MustCompile(`href="https?://t\.me/([A-Za-z0-9_]+)/([0-9]+)"`)

// tgLatestPost returns the URL of the newest post of a public channel.
// Accepts t.me/<channel>, t.me/s/<channel> and t.me/<channel>/<id> links.
func tgLatestPost(ctx context.Context, tgURL string) (string, error) {
	tgURL = strings.ReplaceAll(tgURL, "telegram.me", "t.me")
	if i := strings.Index(tgURL, "?"); i >= 0 {
		tgURL = tgURL[:i]
	}
	tgURL = strings.TrimRight(tgURL, "/")
	// normalize to t.me/<channel> (strip /s/ prefix and any post id)
	tgChan := ""
	if m := regexp.MustCompile(`t\.me/(?:s/)?([A-Za-z0-9_]+)(?:/\d+)?/?$`).FindStringSubmatch(tgURL); m != nil {
		tgChan = m[1]
	}
	if tgChan == "" {
		return "", fmt.Errorf("not a channel link: %s", tgURL)
	}
	preview := "https://t.me/s/" + tgChan

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, preview, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", tgEmbedUA)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("preview status %d", resp.StatusCode)
	}
	htmlBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	// last data-post / href pair is the newest post
	m := tgPostRe.FindAllStringSubmatch(string(htmlBody), -1)
	if len(m) == 0 {
		return "", fmt.Errorf("no posts found")
	}
	last := m[len(m)-1]
	return "https://t.me/" + last[1] + "/" + last[2], nil
}

// ── direct download pick actions ───────────────────────────────────────

// searchPickTWTDirect downloads the tweet behind an X/Twitter search result
// (search results are tweet links from Bing). Photo tweets send up to 4
// photos. Returns false when the download failed (caller falls back to the
// link card).
func searchPickTWTDirect(s SessionBridge, info types.MessageInfo, selected searchResult) bool {
	ok := false
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*DOWNLOADING X / TWITTER MEDIA....*")

		statusID := twExtractTweetID(selected.Link)
		if statusID == "" {
			s.DeleteMessage(info, waitID)
			return
		}
		tweet, err := twFetchTweet(ctx, statusID)
		if err != nil {
			s.DeleteMessage(info, waitID)
			return
		}

		client := mediaHTTPClient()
		caption := twBuildCaption(tweet)

		// Photo-only tweets → send up to 4 photos
		if len(tweet.Media.Videos) == 0 && len(tweet.Media.Photos) > 0 {
			sent := 0
			total := len(tweet.Media.Photos)
			if total > 4 {
				total = 4
			}
			for i := 0; i < total; i++ {
				data := instaFetchThumbnail(ctx, client, tweet.Media.Photos[i].URL)
				if len(data) == 0 {
					continue
				}
				cap := caption
				if len(tweet.Media.Photos) > 1 {
					cap = fmt.Sprintf("%s\n*(%d/%d)*", cap, i+1, total)
				}
				if s.SendImage(info, data, cap) == nil {
					sent++
				}
			}
			s.DeleteMessage(info, waitID)
			if sent > 0 {
				ok = true
			}
			return
		}

		if len(tweet.Media.Videos) == 0 || tweet.Media.Videos[0].URL == "" {
			s.DeleteMessage(info, waitID)
			return
		}
		vid := tweet.Media.Videos[0]

		s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

		path, err := streamDownloadToFile(ctx, client, vid.URL, nil)
		if err != nil {
			s.DeleteMessage(info, waitID)
			return
		}
		defer removeTempFile(path)

		var thumb []byte
		if vid.ThumbnailURL != "" {
			thumb = instaFetchThumbnail(ctx, client, vid.ThumbnailURL)
		}
		secs, w, h := probeVideoMeta(path)
		if w == 0 && h == 0 {
			w, h = uint32(vid.Width), uint32(vid.Height)
		}
		if err := s.SendVideoFile(info, path, caption, thumb, secs, w, h); err != nil {
			s.DeleteMessage(info, waitID)
			return
		}
		s.DeleteMessage(info, waitID)
		ok = true
	})
	return ok
}

// igProfileLatest is one media item from the Instagram web_profile_info API.
type igProfileLatest struct {
	Shortcode  string
	IsVideo    bool
	Caption    string
	LikeCount  int64
	DisplayURL string
	VideoURL   string
}

// igProfileMedia calls the Instagram web_profile_info endpoint for a profile
// link and returns the latest timeline media with direct CDN URLs.
func igProfileMedia(ctx context.Context, profileURL string) ([]igProfileLatest, bool) {
	// extract the username
	low := strings.ToLower(strings.TrimSpace(profileURL))
	low = strings.TrimPrefix(strings.TrimPrefix(low, "https://"), "http://")
	low = strings.TrimPrefix(low, "www.")
	parts := strings.SplitN(low, "/", 3)
	if len(parts) < 2 || parts[1] == "" {
		return nil, false
	}
	username := strings.SplitN(parts[1], "?", 2)[0]

	u := "https://www.instagram.com/api/v1/users/web_profile_info/?username=" + url.QueryEscape(username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("x-ig-app-id", "936619743392459")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 40 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, false
	}

	var parsed struct {
		Data struct {
			User struct {
				FullName  string `json:"full_name"`
				IsPrivate bool   `json:"is_private"`
				Timeline  struct {
					Edges []struct {
						Node struct {
							Shortcode  string `json:"shortcode"`
							IsVideo    bool   `json:"is_video"`
							DisplayURL string `json:"display_url"`
							VideoURL   string `json:"video_url"`
							LikeCount  struct {
								Count int64 `json:"count"`
							} `json:"edge_liked_by"`
							Caption struct {
								Edges []struct {
									Node struct {
										Text string `json:"text"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"edge_media_to_caption"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"edge_owner_to_timeline_media"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&parsed); err != nil {
		return nil, false
	}
	if parsed.Data.User.IsPrivate {
		return nil, false
	}
	var out []igProfileLatest
	for _, e := range parsed.Data.User.Timeline.Edges {
		n := e.Node
		item := igProfileLatest{
			Shortcode:  n.Shortcode,
			IsVideo:    n.IsVideo,
			Caption:    "",
			LikeCount:  n.LikeCount.Count,
			DisplayURL: n.DisplayURL,
			VideoURL:   n.VideoURL,
		}
		if len(n.Caption.Edges) > 0 {
			item.Caption = n.Caption.Edges[0].Node.Text
		}
		out = append(out, item)
		if len(out) >= 3 {
			break
		}
	}
	return out, len(out) > 0
}

// searchPickIGDirect downloads the latest reel/post from an Instagram
// profile search result (web_profile_info → direct CDN video/image URL).
// Returns false when it failed (caller falls back to the link card).
func searchPickIGDirect(s SessionBridge, info types.MessageInfo, selected searchResult, all []searchResult) bool {
	ok := false
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*DOWNLOADING INSTAGRAM MEDIA....*")

		// v2 (FB v6/v7 pattern): try-list — selected pehle, phir baqi results.
		// REEL/POST PERMALINK → seedha cobalt (instant download).
		// PROFILE → web_profile_info → latest reel direct CDN download.
		tryList := []searchResult{selected}
		for _, r := range all {
			if r.Link != selected.Link {
				tryList = append(tryList, r)
			}
		}

		client := mediaHTTPClient()

		for i, r := range tryList {
			link := strings.TrimSpace(r.Link)
			if link == "" {
				continue
			}

			// A) permalink (reel/p) → cobalt fast path
			if igIsPermalink(link) {
				s.EditMessage(info, waitID, "*FETCHING REEL....*")
				if igSendPermalink(ctx, s, info, waitID, client, link, r) {
					ok = true
					return
				}
				if i == 0 && len(tryList) > 1 {
					s.EditMessage(info, waitID, "*SELECTED NOT AVAILABLE — TRYING OTHER RESULTS....*")
				}
				continue
			}

			// B) profile → web_profile_info → latest media
			media, found := igProfileMedia(ctx, link)
			if !found {
				if i == 0 && len(tryList) > 1 {
					s.EditMessage(info, waitID, "*SELECTED NOT AVAILABLE — TRYING OTHER RESULTS....*")
				}
				continue
			}

			// Prefer the first VIDEO node; otherwise send the first image.
			var vid *igProfileLatest
			for j := range media {
				if media[j].IsVideo && media[j].VideoURL != "" {
					vid = &media[j]
					break
				}
			}
			if vid != nil {
				s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")
			path, err := streamDownloadToFile(ctx, client, vid.VideoURL, nil)
			if err != nil {
					if i == 0 && len(tryList) > 1 {
						s.EditMessage(info, waitID, "*SELECTED NOT AVAILABLE — TRYING OTHER RESULTS....*")
					}
					continue
				}
			// WhatsApp-compat: HEVC/mjpeg reels ko h264+faststart me convert
			waPath, werr := whatsappifyVideo(ctx, path)
			if werr == nil && waPath != path {
				defer removeTempFile(path)
				path = waPath
			} else {
				defer removeTempFile(path)
			}

			title := vid.Caption
			if title == "" {
					title = "Instagram " + vid.Shortcode
			}
			if len(title) > 120 {
				title = title[:117] + "..."
			}
			caption := "🏆 *INSTAGRAM VIDEO NAME 🏆*\n" +
				"*" + title + "*\n\n"
			if vid.LikeCount > 0 {
					caption += fmt.Sprintf("🏆 *LIKES :* %d\n", vid.LikeCount)
			}
			caption += "\n*INSTAGRAM VIDEO DOWNLOAD*"

			secs, w, h := probeVideoMeta(path)
			if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
					continue
			}
			s.DeleteMessage(info, waitID)
			ok = true
			return
		}

			// image post
			img := media[0]
			data := instaFetchThumbnail(ctx, client, img.DisplayURL)
			if len(data) == 0 {
				continue
			}
			title := img.Caption
			if title == "" {
				title = "Instagram " + img.Shortcode
			}
			if len(title) > 120 {
			title = title[:117] + "..."
			}
			caption := "🏆 *INSTAGRAM POST* 🏆\n*" + title + "*"
			if s.SendImage(info, data, caption) == nil {
				s.DeleteMessage(info, waitID)
				ok = true
				return
			}
		}
		s.DeleteMessage(info, waitID)
	})
	return ok
}

// igIsPermalink — instagram.com/(reel|p|tv)/SHORTCODE/ check.
func igIsPermalink(link string) bool {
	low := strings.ToLower(strings.TrimSpace(link))
	for _, seg := range []string{"instagram.com/reel/", "instagram.com/p/", "instagram.com/tv/"} {
		if strings.Contains(low, seg) {
			return true
		}
	}
	return false
}

// igSendPermalink — permalink → cobalt → video download + caption.
// Title r.Title (search se aaya) use karta hai, warna filename se.
func igSendPermalink(ctx context.Context, s SessionBridge, info types.MessageInfo, waitID string, client *http.Client, link string, r searchResult) bool {
	resp, err := fbCobaltFetch(ctx, link)
	if err != nil {
		return false
	}
	videoURL, quality := fbResolveVideoURL(resp)
	if videoURL == "" {
		return false
	}
	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")
	path, err := streamDownloadToFile(ctx, client, videoURL, nil)
	if err != nil {
		return false
	}
	// WhatsApp-compat: HEVC/mjpeg/thumbnail reels ko h264+faststart me convert
	waPath, werr := whatsappifyVideo(ctx, path)
	if werr == nil && waPath != path {
		defer removeTempFile(path)
		path = waPath
	} else {
		defer removeTempFile(path)
	}

	title := igCleanCaption(r.Title)
	if title == "" || strings.HasPrefix(title, "Instagram Reel") {
		title = instaTitleFromFilename(resp.Filename)
	}
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	caption := "🏆 *INSTAGRAM VIDEO NAME 🏆*\n" +
		"*" + title + "*\n\n"
	if r.Stats != "" {
		caption += "🏆 *LIKES :* " + igStatsLikes(r.Stats) + "\n"
	}
	caption += "🏆 *QUALITY :❱ " + quality + "*\n\n" +
		"*INSTAGRAM VIDEO DOWNLOAD*"

	secs, w, h := probeVideoMeta(path)
	if secs == 0 && w == 0 {
		quality = "HD"
	}
	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		return false
	}
	s.DeleteMessage(info, waitID)
	return true
}

// igStatsLikes — "28.8M likes" → "28.8M".
func igStatsLikes(stats string) string {
	if i := strings.Index(stats, " likes"); i > 0 {
		return stats[:i]
	}
	return stats
}


// ───────────────────────────────────────────────────────────────────────────
// FACEBOOK DIRECT PICK (profile → latest video → direct download)
// ──────────────────────────────────────────────────────────────────────────────────────

// fbVideoLinkRe matches a video permalink on a FB profile videos listing:
//
//	facebook.com/<user>/videos/<slug>/<id>
//	facebook.com/<id>/videos/<id>            (page posts)
var fbVideoLinkRe = regexp.MustCompile(
	`facebook\.com/([^\s)\"\']+)/videos/(?:([^\s)\"\']+)/)?([0-9]{6,})/?`)

// fbLatestVideoLink reads a FB profile's /videos tab through the jina reader
// proxy (direct hits are login-walled) and returns the permalink of the most
// recent video plus its pretty title (slug). Empty string when nothing was
// found.
func fbLatestVideoLink(ctx context.Context, profileURL string) string {
	low := strings.ToLower(strings.TrimSpace(profileURL))
	low = strings.TrimPrefix(strings.TrimPrefix(low, "https://"), "http://")
	low = strings.TrimPrefix(low, "www.")
	low = strings.TrimPrefix(low, "m.")
	low = strings.TrimSuffix(low, "/")
	// keep only the profile path (drop query strings / sub-paths)
	parts := strings.SplitN(low, "/", 3)
	profile := ""
	if len(parts) >= 2 && parts[1] != "" {
		profile = parts[1]
	}
	if profile == "" {
		return ""
	}
	listing := "https://www.facebook.com/" + profile + "/videos"

	md, err := jinaFetch(ctx, listing)
	if err != nil {
		return ""
	}
	m := fbVideoLinkRe.FindStringSubmatch(md)
	if m == nil {
		return ""
	}
	return "https://www.facebook.com/" + m[1] + "/videos/" + m[3]
}

// fbPrettyTitle turns a /videos/<slug>/<id> or filename into a readable title.
func fbPrettyTitle(slugOrFile, fallback string) string {
	t := slugOrFile
	t = strings.TrimSuffix(t, ".mp4")
	t = strings.ReplaceAll(t, "-", " ")
	t = strings.TrimSpace(t)
	if t == "" {
		return fallback
	}
	return t
}

// searchPickFBDirect downloads the latest video from a Facebook profile
// search result (jina /videos tab → latest permalink → cobalt → fbcdn MP4).
// Returns false when it failed (caller falls back to the link card).
func searchPickFBDirect(s SessionBridge, info types.MessageInfo, selected searchResult, all []searchResult) bool {
	ok := false
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*DOWNLOADING FACEBOOK VIDEO....*")

		// v6: selected profile ke saath shuru karo; wo private nikle
		// (ya uska koi video nahi) to baqi results try karo — pehla
		// milne wala public video download ho jata hai. Error card sirf
		// tab jab SAB private hon.
		tryList := []searchResult{selected}
		for _, r := range all {
			if r.Link != selected.Link {
				tryList = append(tryList, r)
			}
		}
		videoLink := ""
		for i, r := range tryList {
			// v7: DDG video results already direct permalinks (reel / videos /
			// watch) hote hain — unhe seedha cobalt ko do. Profile links ke
			// liye pura resolver (fbLatestVideoLinkFixed) chalega.
			link := fbExtractPermalink(r.Link)
			if link == "" {
				link = fbLatestVideoLinkFixed(ctx, r.Link)
			}
			if link != "" {
				videoLink = link
				_ = i
				break
			}
			// pehle fail hone par user ko batao ke ab baqi try ho rahe
			if i == 0 && len(tryList) > 1 {
				s.EditMessage(info, waitID, "*SELECTED NOT AVAILABLE — TRYING OTHER RESULTS....*")
			}
		}
		if videoLink == "" {
			s.DeleteMessage(info, waitID)
			return
		}

		resp, err := fbCobaltFetch(ctx, videoLink)
		if err != nil {
			s.DeleteMessage(info, waitID)
			return
		}
		videoURL, quality := fbResolveVideoURL(resp)
		if videoURL == "" {
			s.DeleteMessage(info, waitID)
			return
		}

		s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

		client := mediaHTTPClient()
		path, err := streamDownloadToFile(ctx, client, videoURL, nil)
		if err != nil {
			s.DeleteMessage(info, waitID)
			return
		}
		// WhatsApp-compat: HEVC/mjpeg ko h264+faststart me convert
		if waPath, werr := whatsappifyVideo(ctx, path); werr == nil && waPath != path {
			defer removeTempFile(path)
			path = waPath
		} else {
			defer removeTempFile(path)
		}

		title := fbPrettyTitle(resp.Filename, "Facebook Video")
		secs, w, h := probeVideoMeta(path)
		if secs == 0 && w == 0 {
			quality = "HD"
		}
		caption := "🏅 *FACEBOOK VIDEO NAME 🏅*\n" +
			"*" + title + "*\n\n" +
			"🏅 *QUALITY :❱ " + quality + "*\n\n" +
			"*FACEBOOK VIDEO DOWNLOAD*"

		if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
			s.DeleteMessage(info, waitID)
			return
		}
		s.DeleteMessage(info, waitID)
		ok = true
	})
	return ok
}
