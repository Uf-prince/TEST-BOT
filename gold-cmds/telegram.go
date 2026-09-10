package goldcmds

// ============================================================================
// GOLD-MD — Telegram Media Downloader (V2)
// File: telegram.go
// ============================================================================
// HANDLER: handleTG — .tg / .tgsearch direct-link router
//   Downloads a video / photo from a PUBLIC Telegram post and sends it.
//   Channel link diya ho (bina post id) to latest media post bhejta hai.
//
// METHOD (V2 — embed page locked by Telegram):
//   Telegram ne ?embed=1&mode=tme pages lock kar diye — ab wahan sirf
//   "Please open Telegram to view this post" dikhta hai, <video> tag nahi.
//   Naya FREE route: t.me/s/<channel> public preview page. Wahan:
//     • video : <video src="https://cdnN.telesco.pe/file/<id>.mp4?token=...">
//     • photo : tgme_widget_message_photo_wrap style background-image url
//   `?before=<postID+1>` pagination param se KOI BHI post target hota hai
//   (window usi post pe khatam hota hai). Post exist nahi karta to window
//   us se pehle wale existing post pe khatam hoti hai — easily detectable.
//   Bot token / API ID / HASH kuch nahi chahiye — 100% free.
//
// FALLBACK: agar /s/ route fail ho jaye (post deleted / channel private)
//   to purana embed-page scraper ek baar try hota hai (future-proof).
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const tgEmbedUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

var (
	// /s/ preview page — per-message-block media patterns
	tgPreviewVideoRe = regexp.MustCompile(`<video[^>]*src="([^"]+)"`)
	tgPreviewPhotoRe = regexp.MustCompile(`tgme_widget_message_photo_wrap[^>]*background-image:url\('([^']+)'\)`)
	tgPostIDsRe      = regexp.MustCompile(`data-post="[^"]+/(\d+)"`)
	tgDataPostRe     = regexp.MustCompile(`data-post="([^"]+)"`)
	// per-block meta
	tgChanRe   = regexp.MustCompile(`(?s)tgme_widget_message_owner_name" href="https://t\.me/([^/"]+)"[^>]*>(.*?)</a>`)
	tgChanName = regexp.MustCompile(`<span[^>]*>([^<]+)</span>`)
	tgTextRe   = regexp.MustCompile(`(?s)class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>`)
	tgViewsRe  = regexp.MustCompile(`tgme_widget_message_views">([^<]*)<`)
	// legacy embed-page scrapers (fallback route)
	tgVideoRe   = regexp.MustCompile(`<video[^>]+src="([^"]+)"`)
	tgPhotoRe   = regexp.MustCompile(`<a class="tgme_widget_message_photo_wrap"[^>]*href='([^']+)'`)
	tgPhotoBgRe = regexp.MustCompile(`background-image:url\('([^']+)'\)`)
	// link parsers
	tgPostLinkRe   = regexp.MustCompile(`^https?://t\.me/(?:s/)?([A-Za-z0-9_]+)/(\d+)$`)
	tgChannelRe    = regexp.MustCompile(`^https?://t\.me/(?:s/)?([A-Za-z0-9_]+)(?:/\d+)?$`)
	tgChannelInStr = regexp.MustCompile(`t\.me/(?:s/)?([A-Za-z0-9_]+)(?:/\d+)?`)
)

// tgMedia — ek post se nikaala hua media.
type tgMedia struct {
	kind    string // "video" | "photo" | "" (text-only)
	url     string
	channel string // @username
	name    string // channel display name
	views   string
	text    string
}

// handleTG — t.me link (via .tg / .tgsearch) ya channel link.
func handleTG(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleTGAsync(ctx, s, info, args, prefix)
	})
}

func handleTGAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	tgURL := strings.TrimSpace(strings.Join(args, " "))
	if tgURL == "" {
		s.Reply(info, tgHelpText)
		return
	}
	// normalize: telegram.me/telegram.dog -> t.me, strip query
	tgURL = strings.ReplaceAll(tgURL, "telegram.me", "t.me")
	tgURL = strings.ReplaceAll(tgURL, "telegram.dog", "t.me")
	if i := strings.Index(tgURL, "?"); i >= 0 {
		tgURL = tgURL[:i]
	}
	tgURL = strings.TrimRight(tgURL, "/")
	if !strings.Contains(tgURL, "t.me/") {
		s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nPlease provide a valid Telegram link (t.me).")
		return
	}
	if !strings.HasPrefix(tgURL, "http") {
		tgURL = "https://" + tgURL
	}

	waitID := s.ReplyWithID(info, "🔰 *Fetching Telegram media...*")

	// post link (t.me/chan/123) → usi post ka media
	// channel link (t.me/chan) → channel ka latest media post
	var media *tgMedia
	var err error
	if tgPostLinkRe.MatchString(tgURL) {
		media, err = tgFetchPost(ctx, tgURL)
	} else {
		media, err = tgLatestMedia(ctx, tgURL)
	}
	if err != nil || media == nil || media.url == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nMedia not found — is the post public? 🔰")
		return
	}

	if !tgSendMedia(ctx, s, info, waitID, media, "") {
		return
	}
}

// tgSendMedia — media download (whatsappify guard ke sath) + caption + send.
// handleTGAsync aur searchPickTG dono yahi use karte hain.
// waitID edit/delete karta hai; fail hone pe error card bhejta hai.
// Returns false jab kuch bhi send nahi hua.
func tgSendMedia(ctx context.Context, s SessionBridge, info types.MessageInfo, waitID string, media *tgMedia, fallbackTitle string) bool {
	if media == nil || media.url == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nMEDIA NOT AVAILABLE\nTRY ANOTHER POST 🔰")
		return false
	}

	s.EditMessage(info, waitID, "🔰 *DOWNLOADING "+strings.ToUpper(media.kind)+"....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, media.url, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nPLEASE TRY AGAIN 🔰")
		return false
	}
	// WhatsApp-compat guard: HEVC/mjpeg ko h264+faststart me convert
	if media.kind == "video" {
		waPath, werr := whatsappifyVideo(ctx, path)
		if werr == nil && waPath != path {
			removeTempFile(path)
			path = waPath
		}
	}
	defer removeTempFile(path)

	// caption build
	title := media.text
	if title == "" {
		title = fallbackTitle
	}
	if title == "" {
		title = "Telegram Post"
	}
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	creator := media.name
	if creator == "" {
		creator = "@" + media.channel
	}
	caption := "🔰 *TELEGRAM VIDEO NAME* 🔰\n" +
		"*" + title + "*\n\n" +
		"*🔰 CREATOR :* " + creator + "\n"
	if media.views != "" {
		caption += "🔰 *VIEWS :* " + media.views + "\n"
	}
	caption += "\n*TELEGRAM VIDEO DOWNLOAD*"

	if media.kind == "photo" {
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nPhoto could not be read.")
			return false
		}
		if err := s.SendImage(info, data, caption); err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nPhoto could not be sent.")
			return false
		}
		s.DeleteMessage(info, waitID)
		return true
	}

	secs, w, h := probeVideoMeta(path)
	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *TELEGRAM DOWNLOAD ERROR*\nVideo could not be sent.")
		return false
	}
	s.DeleteMessage(info, waitID)
	return true
}

// ── V2: /s/ preview page engine ─────────────────────────────────────────────

// tgFetchPost — post link → media. Primary: /s/ preview route. Fallback:
// legacy embed-page scraper (agar Telegram embed wapas khol de).
func tgFetchPost(ctx context.Context, tgURL string) (*tgMedia, error) {
	if m, err := tgFetchPostViaPreview(ctx, tgURL); err == nil && m != nil {
		return m, nil
	}
	return tgFetchPostViaEmbed(ctx, tgURL)
}

// tgFetchPostViaPreview — t.me/s/<chan>?before=<pid+1> se target post ka
// media. Window normally usi post pe khatam hota hai (single fetch); agar
// post deleted ho to max id se detect ho jata hai. Safety ke liye 3 hops.
func tgFetchPostViaPreview(ctx context.Context, tgURL string) (*tgMedia, error) {
	m := tgPostLinkRe.FindStringSubmatch(tgURL)
	if m == nil {
		return nil, fmt.Errorf("not a post link: %s", tgURL)
	}
	channel, pid := m[1], 0
	if v, err := strconv.Atoi(m[2]); err == nil {
		pid = v
	} else {
		return nil, fmt.Errorf("bad post id: %s", m[2])
	}

	before := pid + 1
	for hop := 0; hop < 3; hop++ {
		html, err := tgGetPage(ctx, fmt.Sprintf("https://t.me/s/%s?before=%d", channel, before))
		if err != nil {
			return nil, err
		}
		if media := tgExtractBlock(html, channel, pid); media != nil {
			return media, nil
		}
		// post is window me nahi — kyun, wo batao
		ids := tgPageIDs(html)
		if len(ids) == 0 {
			return nil, fmt.Errorf("no posts — channel missing or private")
		}
		min, max := ids[0], ids[len(ids)-1]
		if pid > max {
			return nil, fmt.Errorf("post %d does not exist (deleted/invalid)", pid)
		}
		if pid < min {
			before = min // aur purane posts ki taraf hop
			continue
		}
		return nil, fmt.Errorf("post %d not found in window (%d..%d) — deleted", pid, min, max)
	}
	return nil, fmt.Errorf("post %d not found", pid)
}

// tgLatestMedia — channel link → newest post jo video/photo rakhta ho.
// searchPickTG + direct channel-link (.tg t.me/chan) ke liye.
func tgLatestMedia(ctx context.Context, chanURL string) (*tgMedia, error) {
	chanURL = strings.ReplaceAll(chanURL, "telegram.me", "t.me")
	chanURL = strings.ReplaceAll(chanURL, "telegram.dog", "t.me")
	if i := strings.Index(chanURL, "?"); i >= 0 {
		chanURL = chanURL[:i]
	}
	chanURL = strings.TrimRight(chanURL, "/")
	m := tgChannelInStr.FindStringSubmatch(chanURL)
	if m == nil {
		return nil, fmt.Errorf("not a channel link: %s", chanURL)
	}
	channel := m[1]
	html, err := tgGetPage(ctx, "https://t.me/s/"+channel)
	if err != nil {
		return nil, err
	}
	blocks := tgSplitBlocks(html)
	// newest first scan (page me posts oldest→newest order me hote hain)
	for i := len(blocks) - 1; i >= 0; i-- {
		media := tgParseBlock(blocks[i])
		if media != nil && media.url != "" {
			if media.channel == "" {
				media.channel = channel
			}
			return media, nil
		}
	}
	return nil, fmt.Errorf("no media post on channel %s", channel)
}

// ── parsing helpers ─────────────────────────────────────────────────────────

// tgSplitBlocks — /s/ page ko message blocks me split karta hai.
func tgSplitBlocks(html string) []string {
	return strings.Split(html, `<div class="tgme_widget_message_wrap`)
}

// tgPageIDs — page par diye gaye post ids (sorted ascending).
func tgPageIDs(html string) []int {
	var ids []int
	for _, g := range tgPostIDsRe.FindAllStringSubmatch(html, -1) {
		if v, err := strconv.Atoi(g[1]); err == nil {
			ids = append(ids, v)
		}
	}
	// ascending sort (page oldest→newest hota hai, phir bhi safe)
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	return ids
}

// tgExtractBlock — html se data-post="chan/pid" wala block dhoondh kar
// media + meta nikaalta hai. Text-only post pe kind=="" hota hai.
func tgExtractBlock(html, channel string, pid int) *tgMedia {
	target := channel + "/" + strconv.Itoa(pid)
	for _, b := range tgSplitBlocks(html) {
		dm := tgDataPostRe.FindStringSubmatch(b)
		if dm == nil || dm[1] != target {
			continue
		}
		media := tgParseBlock(b)
		if media != nil {
			if media.channel == "" {
				media.channel = channel
			}
			return media
		}
	}
	return nil
}

// tgParseBlock — ek message block se media + meta (kind=="" → text-only).
func tgParseBlock(b string) *tgMedia {
	dm := tgDataPostRe.FindStringSubmatch(b)
	if dm == nil {
		return nil
	}
	out := &tgMedia{}
	if vm := tgPreviewVideoRe.FindStringSubmatch(b); vm != nil {
		out.kind = "video"
		out.url = vm[1]
	} else if pm := tgPreviewPhotoRe.FindStringSubmatch(b); pm != nil {
		out.kind = "photo"
		out.url = pm[1]
	}
	if vw := tgViewsRe.FindStringSubmatch(b); vw != nil {
		out.views = strings.TrimSpace(vw[1])
	}
	if tx := tgTextRe.FindStringSubmatch(b); tx != nil {
		out.text = tgStripTags(tx[1])
	}
	if cm := tgChanRe.FindStringSubmatch(b); cm != nil {
		out.channel = cm[1]
		if nm := tgChanName.FindStringSubmatch(cm[2]); nm != nil {
			out.name = strings.TrimSpace(nm[1])
		}
	}
	return out
}

// tgGetPage — UA ke sath GET, 4MB tak body.
func tgGetPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
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
		return "", fmt.Errorf("page status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ── FALLBACK: legacy embed-page scraper ────────────────────────────────────

// tgFetchPostViaEmbed — purana route (embed page ab locked hai, lekin
// future me Telegram wapas khol de to kaam karega).
func tgFetchPostViaEmbed(ctx context.Context, tgURL string) (*tgMedia, error) {
	m := tgPostLinkRe.FindStringSubmatch(tgURL)
	if m == nil {
		return nil, fmt.Errorf("not a post link: %s", tgURL)
	}
	page, err := tgGetPage(ctx, "https://t.me/"+m[1]+"/"+m[2]+"?embed=1&mode=tme")
	if err != nil {
		return nil, err
	}
	out := &tgMedia{channel: m[1]}
	if vm := tgVideoRe.FindStringSubmatch(page); vm != nil {
		out.kind = "video"
		out.url = vm[1]
	} else if pm := tgPhotoRe.FindStringSubmatch(page); pm != nil {
		out.kind = "photo"
		out.url = pm[1]
	} else if pbm := tgPhotoBgRe.FindStringSubmatch(page); pbm != nil {
		out.kind = "photo"
		out.url = pbm[1]
	}
	if t := tgViewsRe.FindStringSubmatch(page); t != nil {
		out.views = strings.TrimSpace(t[1])
	}
	if t := tgTextRe.FindStringSubmatch(page); t != nil {
		out.text = tgStripTags(t[1])
	}
	if cm := tgChanRe.FindStringSubmatch(page); cm != nil {
		out.channel = cm[1]
		if nm := tgChanName.FindStringSubmatch(cm[2]); nm != nil {
			out.name = strings.TrimSpace(nm[1])
		}
	}
	return out, nil
}

// tgStripTags — HTML tags hatao + entities decode karo (basic).
func tgStripTags(s string) string {
	s = regexp.MustCompile(`<br/?>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'", "&nbsp;", " ")
	return strings.TrimSpace(r.Replace(s))
}

const tgHelpText = "\U0001f530 *TELEGRAM VIDEO DOWNLOAD COMMAND* \U0001f530\n" +
	"*DO YOU WANT TO DOWNLOAD A TELEGRAM VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE TELEGRAM VIDEO LINK \U0001f530*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*.TG ❰TELEGRAM LINK❱*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR TELEGRAM VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"
