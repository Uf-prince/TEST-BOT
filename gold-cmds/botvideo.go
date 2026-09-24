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
	"regexp"
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

	// ── VIDEO DETECTION + DOWNLOAD (direct or quoted) ──
	data, mime, ok := s.DownloadQuotedMedia(info)
	if (!ok || len(data) == 0) || !strings.Contains(strings.ToLower(mime), "video") {
		s.Reply(info, botVideoGuide(prefix))
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
