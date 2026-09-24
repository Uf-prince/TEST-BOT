package goldcmds

// ============================================================================
// GOLD-MD — BOTVIDEO Command  (change menu + alive video)
// File: botvideo.go
// ----------------------------------------------------------------------------
// Sibling of .botpic: the owner sets a VIDEO that .menu + .alive and the
// startup card use instead of the image. Uploads go through the SAME host
// chain as .url (catbox → qu.ax → uguu → gofile, first success wins) so a
// .botvideo clip is stored exactly where .url stores videos. The resulting
// URL is kept in Redis settings:<botJID> field "botvideo".
//
//   .botvideo                → guide
//   .botvideo reset          → remove the custom video (image path again)
//   .botvideo <video-url>    → set directly from a link
//   (reply to / send a video + .botvideo)
//                            → download, upload to ImageKit, save the URL
//
// Owner-only command. Hidden aliases work silently (never in the menu).
// ============================================================================

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// videoLinkRe matches a bare http(s) media URL ending in a video extension.
var videoLinkRe = regexp.MustCompile(`^(https?://\S+\.(mp4|3gp|webm|mov|mkv|avi))$`)

// botVideoGuide is the no-argument help card.
func botVideoGuide(prefix string) string {
	return "*🔰 BOT VIDEO CHANGE GUIDE 🔰*\n\n" +
		"*DO YOU WANT TO CHANGE YOUR BOT MENU + ALIVE VIDEO*\n\n" +
		"*1❯ SIMPLY SEND YOUR VIDEO HERE*\n" +
		"*2❯ MENTION THE VIDEO IMPORTANT 🔰*\n" +
		"*3❯ AFTER MENTION THE VIDEO TYPE ❰ " + prefix + "BOTVIDEO ❱*\n" +
		"*TO CHANGE THE BOT MENU + ALIVE VIDEO*\n\n" +
		"*TO SET A VIDEO FROM A LINK TYPE*\n" +
		"*❰ " + prefix + "BOTVIDEO <VIDEO-URL> ❱*\n\n" +
		"*TO REMOVE THE CUSTOM VIDEO TYPE*\n" +
		"*❰ " + prefix + "BOTVIDEO RESET ❱*"
}

// botVideoProbe reads duration + dimensions from a local media file using the
// same static ffprobe the compress/guard code uses (nexstore/ffmpeg, resolved
// through PATH by the boot self-install). Returns 0s when probing fails — a
// missing probe is not fatal, but WhatsApp plays a video far more reliably
// when Seconds/Width/Height are present.
func botVideoProbe(path string) (secs, w, h uint32) {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "x")
		if len(parts) == 2 {
			ww, _ := strconv.ParseUint(parts[0], 10, 32)
			hh, _ := strconv.ParseUint(parts[1], 10, 32)
			w, h = uint32(ww), uint32(hh)
		}
	}
	dout, derr := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "csv=p=0", path).Output()
	if derr == nil {
		if f, perr := strconv.ParseFloat(strings.TrimSpace(string(dout)), 64); perr == nil && f > 0 {
			secs = uint32(f)
		}
	}
	return secs, w, h
}

// botVideoIsValidMedia reports whether data looks like real playable media
// rather than an HTML error/share page. This is what stops "this video is not
// available" — a host page or a 404 body must never reach WhatsApp as a video.
func botVideoIsValidMedia(data []byte, mime string) bool {
	if len(data) < 1024 {
		return false
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	trimmed := strings.TrimSpace(string(head))
	if strings.HasPrefix(trimmed, "<") || strings.Contains(strings.ToLower(trimmed), "<!doctype html") || strings.Contains(strings.ToLower(trimmed), "<html") {
		return false
	}
	ct := strings.ToLower(http.DetectContentType(data))
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "html") {
		return false
	}
	_ = mime
	return true
}

// botVideoURLIsPlayable does a ranged GET (with the browser UA + Referer the
// host expects) and confirms the server answers with real media bytes rather
// than an HTML share/error page. This is the check that guarantees a stored
// .botvideo URL will actually play inside WhatsApp.
func botVideoURLIsPlayable(raw string) bool {
	raw = resolveDirectMediaURL(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", browserUA)
	if ref := MediaReferer(raw); ref != "" {
		req.Header.Set("Referer", ref)
	}
	req.Header.Set("Range", "bytes=0-4095")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	buf := make([]byte, 2048)
	n, _ := io.ReadFull(resp.Body, buf)
	if n <= 0 {
		return false
	}
	return botVideoIsValidMedia(buf[:n], "video/mp4")
}

// IsPlayableMedia is the exported wrapper of botVideoIsValidMedia for the
// session layer, so a HTML/404 body can never be uploaded as a bot video.
func IsPlayableMedia(data []byte, mime string) bool { return botVideoIsValidMedia(data, mime) }

// BotVideoThumbnail extracts a single JPEG frame (max 320px wide) from a video
// file with ffmpeg. WhatsApp shows this frame (progressive JPEG) while the clip
// is still downloading and refuses to render a video without a thumbnail on
// some clients, so the menu/alive video must always carry one.
func BotVideoThumbnail(path string) []byte {
	if path == "" {
		return nil
	}
	out := path + ".thumb.jpg"
	defer os.Remove(out)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", path,
		"-frames:v", "1", "-vf", "scale=320:-2", "-q:v", "5", out)
	if err := cmd.Run(); err != nil {
		return nil
	}
	b, err := os.ReadFile(out)
	if err != nil || len(b) == 0 {
		return nil
	}
	return b
}

// botVideoExtForMime returns the file extension for a video mimetype.
func botVideoExtForMime(mime string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.Contains(m, "3gp"):
		return ".3gp"
	case strings.Contains(m, "webm"):
		return ".webm"
	case strings.Contains(m, "quicktime"), strings.Contains(m, "mov"):
		return ".mov"
	case strings.Contains(m, "x-matroska"), strings.Contains(m, "mkv"):
		return ".mkv"
	default:
		return ".mp4"
	}
}

func handleBotVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	argRaw := ""
	if len(args) > 0 {
		argRaw = strings.TrimSpace(strings.Join(args, " "))
	}

	// ── RESET ──
	if strings.ToLower(argRaw) == "reset" {
		s.SetBotVideoSetting("")
		s.Reply(info, "*🔰 BOT VIDEO RESET 🔰*\n\n*CUSTOM VIDEO REMOVED*\n*MENU AND ALIVE ARE BACK TO THE BOT PIC*")
		return
	}

	// ── URL se set ──
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := videoLinkRe.FindString(firstTok); m != "" {
			s.SetBotVideoSetting(m)
			s.Reply(info, fmt.Sprintf(
				"*🔰 BOT VIDEO UPDATED 🔰*\n\n"+
					"*MENU + ALIVE DONO KA VIDEO CHANGE HO GYA*\n\n"+
					"*NEW VIDEO:*\n%s", m))
			return
		}
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, "*🔰 INVALID VIDEO LINK 🔰*\n\n*LINK .mp4 / .3gp / .webm / .mov / .mkv / .avi pe khatam honi chahiye*")
			return
		}
	}

	// ── VIDEO DETECTION (direct or quoted) ──
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 || !strings.Contains(strings.ToLower(mime), "video") {
		s.Reply(info, botVideoGuide(prefix))
		return
	}
	// A real video must actually BE a video: reject HTML/host pages and tiny
	// bodies so a broken file can never be set (and later fail to play).
	if !botVideoIsValidMedia(data, mime) {
		s.Reply(info, "*🔰 INVALID VIDEO 🔰*\n\n*YE FILE ASAL VIDEO NAHI HAI (BROKEN YA HTML PAGE)*\n*DOBARA PROPER VIDEO BHEJ KAR ❰ "+prefix+"BOTVIDEO ❱ TRY KARO*")
		return
	}

	// ── PROCESSING animation ──
	waitID := s.ReplyWithID(info, "*🔰 BOT VIDEO UPLOAD HO RAHI HAI...*\n*PROCESSING: 00%*")

	stop := make(chan struct{})
	go func() {
		percent := 0
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if percent >= 90 {
					continue
				}
				percent += 7
				if percent > 90 {
					percent = 90
				}
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING BOT VIDEO*\n*PROCESSING: %02d%%*", percent))
			}
		}
	}()

	// ── UPLOAD through the .url host chain (catbox → qu.ax → uguu → gofile,
	//    first success wins) so a .botvideo clip is stored exactly where .url
	//    stores videos. ──
	fileName := "goldmd-botvideo" + botVideoExtForMime(mime)
	uploadURL, _, err := uploadAnyHost(data, fileName, mime)
	close(stop)
	s.DeleteMessage(info, waitID)

	if err != nil || uploadURL == "" {
		s.Reply(info, fmt.Sprintf("🔰 *Upload fail:* %v", err))
		return
	}

	// Hosts answer with a share PAGE (qu.ax/O7xfZ) whose og:video tag holds the
	// file URL. Storing the page link is what made WhatsApp say "this video is
	// not available" — resolve to the file, then prove it is real playable mp4.
	if direct := resolveDirectMediaURL(uploadURL); direct != "" {
		uploadURL = direct
	}
	if !botVideoURLIsPlayable(uploadURL) {
		s.Reply(info, "*🔰 VIDEO SETUP FAIL 🔰*\n\n*UPLOADED FILE PLAYABLE VIDEO NAHI NIKLI*\n*DOBARA PROPER MP4 BHEJ KAR ❰ "+prefix+"BOTVIDEO ❱ TRY KARO*")
		return
	}

	s.SetBotVideoSetting(uploadURL)

	s.Reply(info, "*🔰 BOT VIDEO UPDATED 🔰*\n\n"+
		"*NEW VIDEO FOR MENU + ALIVE HAS BEEN APPLIED*\n\n"+
		"*FOR TEST TYPE ❰ ALIVE ❱ OR ❰ MENU ❱*")
}

func init() {
	Register(Command{
		Name:      "botvideo",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE BOT MENU AND ALIVE VIDEO. REPLY TO A VIDEO AND USE THIS COMMAND.",
		OwnerOnly: true,
		Run:       handleBotVideo,
	})
	// hidden aliases (same work, never in the menu)
	for _, alias := range []string{"botvid", "botclip", "botmp4", "videobot", "vidbot"} {
		Register(Command{Name: alias, OwnerOnly: true, Hidden: true, Run: handleBotVideo})
	}
}
