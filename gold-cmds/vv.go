package goldcmds

// ============================================================================
// GOLD-MD — View-Once Opener (VV) Command
// File: vv.go
// ============================================================================
// COMMAND: .vv    (owner-only, reply to a view-once image/video/audio message)
//   Downloads the view-once media and re-sends it as a normal, permanent
//   media message (image, video, or audio) so it can be saved/forwarded.
//
// Source: UMAR-MD vv.js  (Node.js / Baileys — downloadContentFromMessage)
// Converted to Go / whatsmeow for GOLD-MD.
//
// The bridge's DownloadQuotedMedia handles ViewOnceMessage /
// ViewOnceMessageV2 / ViewOnceMessageV2Extension wrappers (and quoted media),
// returning the raw bytes + mimetype. We then re-send based on mimetype.
//
// Aliases (Hidden): viewonce, vvopen, openvv, showvv, vvshow, privacyopen
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const vvHelpText = "*🔰 VIEWONCE COMMAND INFO 🔰*\n\n" +
	"*OPENS VIEWONCE MEDIA*\n" +
	"*SUPPORTED TYPES:*\n" +
	"• IMAGE 🔰\n" +
	"• VIDEO 🔰\n" +
	"• AUDIO 🔰\n" +
	"• VOICE NOTE 🔰\n\n" +
	"*REPLY TO ANY VIEWONCE MEDIA WITH THIS COMMAND*\n\n" +
	"*EXAMPLE:* .vv\n\n" +
	"*🔰 OWNER ONLY COMMAND 🔰*"

func handleVV(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleVVAsync(s, info, args, prefix)
}

func handleVVAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	// Check if there is a quoted message at all.
	hasQuoted := false
	if q, ok := s.(interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}); ok {
		if qid, _, ok2 := q.GetQuotedMessageID(info); ok2 && qid != "" {
			hasQuoted = true
		}
	}
	if !hasQuoted {
		s.Reply(info, vvHelpText)
		return
	}

	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, "🔰 *THIS IS NOT A VIEWONCE MEDIA*\n\n*Reply to a ViewOnce image/video/audio with .vv*")
		return
	}

	waitID := s.ReplyWithID(info, "🔰 *OPENING VIEWONCE MEDIA...*")

	low := strings.ToLower(mime)
	var sentErr error

	switch {
	case strings.HasPrefix(low, "image"):
		sentErr = s.SendImage(info, data, "*IMAGE OPENED*")
	case strings.HasPrefix(low, "video"):
		// We don't have width/height/seconds readily; send with zeros.
		sentErr = s.SendVideo(info, data, "*VIDEO OPENED*", nil, 0, 0, 0)
	case strings.HasPrefix(low, "audio"):
		sentErr = s.SendAudio(info, data, "", 0)
	case strings.HasPrefix(low, "image/webp"), strings.Contains(low, "webp"):
		sentErr = s.SendSticker(info, data)
	default:
		// Unknown type — send as a document so it is preserved.
		sentErr = s.SendDocument(info, data, "viewonce_media", mime, "*MEDIA OPENED*")
	}

	s.DeleteMessage(info, waitID)

	if sentErr != nil {
		s.Reply(info, "🔰 *FAILED TO OPEN VIEWONCE MEDIA*\n"+sentErr.Error())
		return
	}
}

func init() {
	Register(Command{Name: "vv", Category: "AI & MEDIA", Desc: "Open & re-send a view-once media", OwnerOnly: true, Run: handleVV})
	Register(Command{Name: "viewonce", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "vvopen", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "openvv", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "showvv", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "vvshow", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "privacyopen", OwnerOnly: true, Hidden: true, Run: handleVV})
}
