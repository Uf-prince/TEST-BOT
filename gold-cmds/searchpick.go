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
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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
	searchSessTTL  = 2 * time.Minute
)

// setSearchSession stores a fresh pick window.
func setSearchSession(jid string, kind searchPickKind, query string, results []searchResult) {
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
	default:
		// TT / FB / IG / TWT — profile downloaders are API-blocked, send
		// the result card with the direct link + downloader hint.
		searchPickLinkCard(s, info, sess.Kind, sess.Query, selected, prefix)
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
		return prefix + "ttsearch " + tiktokExampleLink
	case pickFB:
		return prefix + "fbsearch " + fbExampleLink
	case pickIG:
		return prefix + "igsearch " + igExampleLink
	case pickTG:
		return prefix + "tgsearch " + tgExampleLink
	case pickTWT:
		return prefix + "twtsearch " + twtExampleLink
	case pickAPK:
		return prefix + "apksearch " + apkExampleLink
	}
	return prefix + "search <query>"
}

// searchWrongLinkCard is the error reply when the pasted link does not belong
// to this search command's platform. Same style as the user's example:
//
//	.apksearch <facebook link>  →  "GIVE ME THE VALID APK LINK" + EXAMPLE
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

// searchPickLinkCard is the fallback pick card for TT / FB / IG / TWT:
// big link + how-to-download hint (profile downloads are API-blocked).
func searchPickLinkCard(s SessionBridge, info types.MessageInfo, kind searchPickKind, query string, selected searchResult, prefix string) {
	var header, dlCmd, cmdHint string
	switch kind {
	case pickTT:
		header, dlCmd, cmdHint = "TIKTOK USER", prefix+"ttsearch", "TIKTOK VIDEO LINK"
	case pickFB:
		header, dlCmd, cmdHint = "FACEBOOK PROFILE", prefix+"fbsearch", "FACEBOOK VIDEO / REEL LINK"
	case pickIG:
		header, dlCmd, cmdHint = "INSTAGRAM ACCOUNT", prefix+"igsearch", "INSTAGRAM POST / REEL LINK"
	default:
		header, dlCmd, cmdHint = "X / TWITTER ACCOUNT", prefix+"twtsearch", "X VIDEO LINK"
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
