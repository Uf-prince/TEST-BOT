package goldcmds

// ============================================================================
// GOLD-MD — .tg Pick → TYPE Menu (V1)
// File: tgmenu.go
// ============================================================================
// After a .tg search pick (number reply → channel), the bot scans the
// channel's /s/ preview page and shows ONLY the types it can actually
// deliver:
//
//   TYPE ❮ 1 ❯ TO GET TEXT ONLY   — newest post with text
//   TYPE ❮ 2 ❯ TO GET PHOTO       — newest photo post (background-image url)
//   TYPE ❮ 3 ❯ TO GET VIDEO       — newest video post (telesco.pe CDN)
//   TYPE ❮ 4 ❯ TO GET AUDIO       — newest video → ffmpeg MP3 extract
//
// Unavailable types simply don't get a line in the menu. The user replies
// with the number; TGTypeTryHandle (hooked in handler.go BEFORE the search
// pick session) consumes it and delivers.
//
// AUDIO note: the t.me /s/ preview does not expose audio-document URLs,
// so audio is delivered as MP3 extracted from the newest video post —
// that's why TYPE 4 needs BOTH a video post AND ffmpeg on the host.
// ============================================================================

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── channel inventory (one /s/ page scan) ───────────────────────────────────

// tgChannelInventory — what the channel's newest posts can deliver.
type tgChannelInventory struct {
	channel    string   // @username
	name       string   // display name
	textMedia  *tgMedia // newest post that carries text
	photoMedia *tgMedia // newest photo post
	videoMedia *tgMedia // newest video post
}

// empty reports whether nothing at all is deliverable.
func (inv *tgChannelInventory) empty() bool {
	return inv == nil ||
		(inv.textMedia == nil && inv.photoMedia == nil && inv.videoMedia == nil)
}

// audioAvailable — MP3-extract route needs a video post + ffmpeg.
func (inv *tgChannelInventory) audioAvailable() bool {
	return inv != nil && inv.videoMedia != nil && isFfmpegAvailable()
}

// displayName — channel display name or @username fallback.
func (inv *tgChannelInventory) displayName() string {
	if inv == nil {
		return "Telegram Channel"
	}
	if inv.name != "" {
		return inv.name
	}
	if inv.channel != "" {
		return "@" + inv.channel
	}
	return "Telegram Channel"
}

// tgChannelScan — one /s/ page fetch → newest text/photo/video posts.
// Accepts t.me/<chan>, t.me/s/<chan>, t.me/<chan>/<id>, telegram.me/dog.
func tgChannelScan(ctx context.Context, chanURL string) (*tgChannelInventory, error) {
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
	inv := &tgChannelInventory{channel: channel}
	// newest first (page renders posts oldest→newest)
	for i := len(blocks) - 1; i >= 0; i-- {
		media := tgParseBlock(blocks[i])
		if media == nil {
			continue
		}
		if media.channel == "" {
			media.channel = channel
		}
		if media.name != "" && inv.name == "" {
			inv.name = media.name
		}
		if inv.videoMedia == nil && media.kind == "video" && media.url != "" {
			inv.videoMedia = media
		}
		if inv.photoMedia == nil && media.kind == "photo" && media.url != "" {
			inv.photoMedia = media
		}
		if inv.textMedia == nil && strings.TrimSpace(media.text) != "" {
			inv.textMedia = media
		}
		if inv.videoMedia != nil && inv.photoMedia != nil && inv.textMedia != nil {
			break // all three found
		}
	}
	return inv, nil
}

// ── menu card ───────────────────────────────────────────────────────────────

// tgTypeMenuCard — ONLY the available types get a line (fixed numbering).
func tgTypeMenuCard(inv *tgChannelInventory, pickedTitle string) string {
	var b strings.Builder
	b.WriteString("🔵 *GOLD-MD TELEGRAM MEDIA* 🔵\n\n")
	if pickedTitle != "" {
		b.WriteString("*RESULT :* " + pickedTitle + "\n")
	}
	b.WriteString("*CHANNEL :* " + inv.displayName() + "\n\n")
	b.WriteString("*WHAT DO YOU WANT FROM THIS CHANNEL?* 🤔\n\n")
	if inv.textMedia != nil {
		b.WriteString("*TYPE ❮ 1 ❯ TO GET TEXT ONLY*\n")
	}
	if inv.photoMedia != nil {
		b.WriteString("*TYPE ❮ 2 ❯ TO GET PHOTO*\n")
	}
	if inv.videoMedia != nil {
		b.WriteString("*TYPE ❮ 3 ❯ TO GET VIDEO*\n")
	}
	if inv.audioAvailable() {
		b.WriteString("*TYPE ❮ 4 ❯ TO GET AUDIO*\n")
	}
	b.WriteString("\n*REPLY WITH THE NUMBER OF WHAT YOU WANT*\n\n*SESSION STAYS OPEN 30s — YOU CAN ASK AGAIN & AGAIN*")
	return b.String()
}

// tgTypeNotAvailCard — user picked a type this channel doesn't have.
func tgTypeNotAvailCard(kind string) string {
	return "❌ *TELEGRAM MEDIA ERROR*\n" + kind + " IS NOT AVAILABLE ON THIS CHANNEL\n" +
		"CHOOSE ANOTHER TYPE FROM THE MENU 🤷"
}

// ── choice session (menu → number reply window) ────────────────────────────

type tgTypeSession struct {
	inv    *tgChannelInventory
	title  string
	expiry time.Time
}

var (
	tgTypeMu   sync.Mutex
	tgTypeSess = map[string]*tgTypeSession{}
	tgTypeTTL  = 30 * time.Second // owner rule: har pick ke baad 30s fresh window
)

func setTGTypeChoice(jid string, inv *tgChannelInventory, title string) {
	tgTypeMu.Lock()
	defer tgTypeMu.Unlock()
	tgTypeSess[jid] = &tgTypeSession{inv: inv, title: title, expiry: time.Now().Add(tgTypeTTL)}
}

func getTGTypeChoice(jid string) *tgTypeSession {
	tgTypeMu.Lock()
	defer tgTypeMu.Unlock()
	sess, ok := tgTypeSess[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.expiry) {
		delete(tgTypeSess, jid)
		return nil
	}
	return sess
}

func clearTGTypeChoice(jid string) {
	tgTypeMu.Lock()
	defer tgTypeMu.Unlock()
	delete(tgTypeSess, jid)
}

// rearmTGTypeChoice — delivery ke baad 30s fresh window (owner rule).
// Zero-cost: sirf mutex + timestamp, koi goroutine/spawn nahi.
func rearmTGTypeChoice(jid string) {
	tgTypeMu.Lock()
	defer tgTypeMu.Unlock()
	if sess, ok := tgTypeSess[jid]; ok {
		sess.expiry = time.Now().Add(tgTypeTTL)
	}
}

// ── choice handler (call from handler.go BEFORE SearchTryHandle) ────────────

// TGTypeTryHandle — consumes a bare 1-4 reply while a TYPE menu window is
// live. Returns true when the message was consumed.
func TGTypeTryHandle(s SessionBridge, info types.MessageInfo, body, prefix string) bool {
	sess := getTGTypeChoice(info.Sender.String())
	if sess == nil {
		return false
	}
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, prefix) {
		trimmed = strings.TrimSpace(trimmed[len(prefix):])
	}
	var choice int
	if _, err := fmt.Sscanf(trimmed, "%d", &choice); err != nil || choice < 1 || choice > 4 {
		return false // not a TYPE pick — message flows on
	}
	if sess.inv == nil || sess.inv.empty() {
		clearTGTypeChoice(info.Sender.String())
		return false
	}
	// 30s persistent window (owner rule): session band NAHI hota — har pick
	// ke baad 30s fresh re-arm, taki user 1 → 2 → 3 → 4 sab kuch ek hi menu
	// se mangwa sake. 30s tak koi number na aaye to window khud mar jata hai.
	rearmTGTypeChoice(info.Sender.String())
	switch choice {
	case 1:
		if sess.inv.textMedia == nil || strings.TrimSpace(sess.inv.textMedia.text) == "" {
			s.Reply(info, tgTypeNotAvailCard("TEXT"))
			return true
		}
		tgDeliverText(s, info, sess.inv)
	case 2:
		if sess.inv.photoMedia == nil {
			s.Reply(info, tgTypeNotAvailCard("PHOTO"))
			return true
		}
		RunWithTimeout(s, info, func(ctx context.Context) {
			waitID := s.ReplyWithID(info, "⏳ *FETCHING TELEGRAM PHOTO...*")
			tgSendMedia(ctx, s, info, waitID, sess.inv.photoMedia, sess.title)
		})
	case 3:
		if sess.inv.videoMedia == nil {
			s.Reply(info, tgTypeNotAvailCard("VIDEO"))
			return true
		}
		RunWithTimeout(s, info, func(ctx context.Context) {
			waitID := s.ReplyWithID(info, "⏳ *FETCHING TELEGRAM VIDEO...*")
			tgSendMedia(ctx, s, info, waitID, sess.inv.videoMedia, sess.title)
		})
	case 4:
		if !sess.inv.audioAvailable() {
			s.Reply(info, tgTypeNotAvailCard("AUDIO"))
			return true
		}
		RunWithTimeout(s, info, func(ctx context.Context) {
			tgDeliverAudio(ctx, s, info, sess.inv, sess.title)
		})
	}
	return true
}

// ── delivery routes ─────────────────────────────────────────────────────────

// tgDeliverText — newest text post as a text message (instant, no download).
func tgDeliverText(s SessionBridge, info types.MessageInfo, inv *tgChannelInventory) {
	text := strings.TrimSpace(inv.textMedia.text)
	if text == "" {
		s.Reply(info, tgTypeNotAvailCard("TEXT"))
		return
	}
	if len(text) > 3500 {
		text = text[:3497] + "..."
	}
	card := "🔵 *TELEGRAM POST TEXT* 🔵\n\n" +
		text + "\n\n" +
		"🔵 *CHANNEL :* " + inv.displayName() + "\n"
	if inv.textMedia.views != "" {
		card += "🔵 *VIEWS :* " + inv.textMedia.views + "\n"
	}
	card += "\n*TELEGRAM TEXT DOWNLOAD*"
	s.Reply(info, card)
}

// tgDeliverAudio — newest video post → download → ffmpeg MP3 → SendAudioFile.
func tgDeliverAudio(ctx context.Context, s SessionBridge, info types.MessageInfo, inv *tgChannelInventory, fallbackTitle string) {
	media := inv.videoMedia
	waitID := s.ReplyWithID(info, "⬇️ *DOWNLOADING AUDIO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, media.url, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TELEGRAM DOWNLOAD ERROR*\nAUDIO COULD NOT BE DOWNLOADED 🤧")
		return
	}
	defer removeTempFile(path)

	s.EditMessage(info, waitID, "🎵 *EXTRACTING MP3....*")
	mp3Path, err := ffmpegToMP3(ctx, path)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TELEGRAM AUDIO ERROR*\nMP3 CONVERSION FAILED 🤧")
		return
	}
	defer removeTempFile(mp3Path)

	seconds := probeAudioDuration(mp3Path)

	title := strings.TrimSpace(media.text)
	if title == "" {
		title = fallbackTitle
	}
	if title == "" {
		title = "Telegram Audio"
	}
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	creator := media.name
	if creator == "" {
		creator = "@" + media.channel
	}
	caption := "🔵 *TELEGRAM AUDIO NAME* 🔵\n" +
		"*" + title + "*\n\n" +
		"🔵 *CREATOR :* " + creator + "\n"
	if media.views != "" {
		caption += "🔵 *VIEWS :* " + media.views + "\n"
	}
	caption += "\n*TELEGRAM AUDIO DOWNLOAD*"

	if err := s.SendAudioFile(info, mp3Path, caption, seconds); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TELEGRAM AUDIO ERROR*\nAUDIO COULD NOT BE SENT 🤧")
		return
	}
	s.DeleteMessage(info, waitID)
}
