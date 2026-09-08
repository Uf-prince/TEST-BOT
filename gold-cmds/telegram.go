package goldcmds

// ============================================================================
// GOLD-MD — Telegram Media Downloader
// File: telegram.go
// ============================================================================
// HANDLER: handleTG — used by .tgsearch direct-link router
//   Downloads a video / photo from a PUBLIC Telegram post and sends it.
//
// METHOD (100% FREE — koi API key nahi):
//   t.me ka public embed page (https://t.me/<channel>/<id>?embed=1&mode=tme)
//   scrape karke direct CDN URL nikaalte hain:
//     • video : <video src="https://cdn1.telesco.pe/file/...mp4?token=...">
//     • photo : tgme_widget_message_photo_wrap href / background-image url
//   Embed page public posts ke liye hamesha open hota hai — bot token,
//   API ID/HASH kuch nahi chahiye.
//
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const tgEmbedUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

var (
	tgVideoRe   = regexp.MustCompile(`<video[^>]+src="([^"]+)"`)
	tgPhotoRe   = regexp.MustCompile(`<a class="tgme_widget_message_photo_wrap"[^>]*href='([^']+)'`)
	tgPhotoBgRe = regexp.MustCompile(`background-image:url\('([^']+)'\)`)
	tgTextRe    = regexp.MustCompile(`(?s)class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>`)
	tgViewsRe   = regexp.MustCompile(`tgme_widget_message_views">([^<]*)<`)
	tgChanRe    = regexp.MustCompile(`(?s)tgme_widget_message_owner_name" href="https://t\.me/([^/"]+)"[^>]*>(.*?)</a>`)
	tgChanName  = regexp.MustCompile(`<span[^>]*>([^<]+)</span>`)
)

// tgMedia — ek post se nikaala hua media.
type tgMedia struct {
	kind    string // "video" | "photo"
	url     string
	channel string // @username
	name    string // channel display name
	views   string
	text    string
}

// handleTG — t.me link (via .tgsearch)
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
	// normalize: telegram.me -> t.me, strip query/extra path
	tgURL = strings.Replace(tgURL, "telegram.me", "t.me", 1)
	if i := strings.Index(tgURL, "?"); i >= 0 {
		tgURL = tgURL[:i]
	}
	tgURL = strings.TrimRight(tgURL, "/")
	if !strings.Contains(tgURL, "t.me/") {
		s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nPlease provide a valid Telegram link (t.me).")
		return
	}

	waitID := s.ReplyWithID(info, "⏳ *Fetching Telegram media...*")

	media, err := tgFetchPost(ctx, tgURL)
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
		s.Reply(info, "❌ PLEASE TRY AGAIN 🤗")
		return
	}
	defer removeTempFile(path)

	// caption build
	title := media.text
	if title == "" {
		title = "Telegram Post"
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
		"*🔰 CREATOR :* " + creator + "\n"
	if media.views != "" {
		caption += "*🔰 VIEWS :* " + media.views + "\n"
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
}

// tgFetchPost — embed page fetch karke media + meta nikaalo.
func tgFetchPost(ctx context.Context, tgURL string) (*tgMedia, error) {
	embedURL := tgURL + "?embed=1&mode=tme"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", tgEmbedUA)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("post page status %d", resp.StatusCode)
	}
	html, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	page := string(html)

	m := &tgMedia{}

	// video first (preferred)
	if vm := tgVideoRe.FindStringSubmatch(page); vm != nil {
		m.kind = "video"
		m.url = vm[1]
	} else if pm := tgPhotoRe.FindStringSubmatch(page); pm != nil {
		m.kind = "photo"
		m.url = pm[1]
	} else if pbm := tgPhotoBgRe.FindStringSubmatch(page); pbm != nil {
		m.kind = "photo"
		m.url = pbm[1]
	}

	// meta
	if t := tgViewsRe.FindStringSubmatch(page); t != nil {
		m.views = strings.TrimSpace(t[1])
	}
	if t := tgTextRe.FindStringSubmatch(page); t != nil {
		m.text = tgStripTags(t[1])
	}
	if cm := tgChanRe.FindStringSubmatch(page); cm != nil {
		m.channel = cm[1]
		if nm := tgChanName.FindStringSubmatch(cm[2]); nm != nil {
			m.name = strings.TrimSpace(nm[1])
		}
	}
	return m, nil
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
	"*.TG \u2770TELEGRAM LINK\u2771*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR TELEGRAM VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"
