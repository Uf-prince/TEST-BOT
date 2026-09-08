package goldcmds

// ============================================================================
// GOLD-MD — .gcstatus command
//
// User posts any media (photo/video/audio/music/file) or text, mentions it
// with .gcstatus. The bot:
//   1. Posts it as its OWN WhatsApp status (story) — SendMessage to
//      status@broadcast.
//   2. Then mentions/shares that status to ALL groups the bot is in —
//      GroupStatusMentionMessage per group (the "X mentioned your group in
//      their status" notification with status preview). Exactly like the
//      manual WhatsApp flow: post status → tap @ → select groups.
//
// Full JSON debugs via JSONDebug (prints when GOLDMD_DEBUG=1).
//
// Reference: gifted-baileys gcstatus.js sendStatusToGroups.
// ============================================================================

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(Command{
		Name:      "gcstatus",
		Category:  "PRESENCE & STATUS",
		Desc:      "Post status & mention to all groups (media/text + .gcstatus)",
		OwnerOnly: true,
		Run:       handleGCStatus,
	})
}

// handleGCStatus is the entry point. Runs the heavy work in a goroutine so
// the dispatch returns immediately (whatsmeow message-send lock + group
// fetches can take a few seconds).
func handleGCStatus(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGCStatusAsync(s, info, args, prefix)
}

func handleGCStatusAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !ownerGate(s, info) {
		return
	}

	//	JSONDebug("GCSTATUS_START", map[string]any{
	//		"bot":     s.GetJID(),
	//		"chat":    info.Chat.String(),
	//		"sender":  info.Sender.String(),
	//		"argsLen": len(args),
	//	})

	// ── Step 1: Detect + download media from the incoming message ──
	mediaData, mediaMime, hasMedia := s.DownloadQuotedMedia(info)
	//	JSONDebug("GCSTATUS_MEDIA_DETECT", map[string]any{
	//		"hasMedia": hasMedia,
	//		"mime":     mediaMime,
	//		"bytes":    len(mediaData),
	//	})

	// Caption = any text after the command (args), or the message caption.
	caption := strings.TrimSpace(strings.Join(args, " "))
	if caption == "" {
		caption = s.GetMessageText(info)
		caption = strings.TrimSpace(stripCommandFromText(caption, "gcstatus"))
	}

	// ── Guidance: if user typed ONLY .gcstatus with nothing else ──
	// (no media, no caption, no replied message) -> send styled help
	// message and return early. No status is posted.
	// Also strip any leftover prefix chars (: . ! # /) so ":gcstatus"
	// alone is detected as bare command.
	captionCleaned := strings.TrimLeft(caption, ":.!#/ \t\r\n")
	if !hasMedia && captionCleaned == "" {
		quotedText := s.GetQuotedMessageText(info)
		if strings.TrimSpace(quotedText) == "" {
			guidance := "*🔰 GCSTATUS COMMAND GUIDE 🔰*\n" +
				"\n" +
				"*WHAT DOES .GCSTATUS DO ?*\n" +
				"IT POSTS YOUR MEDIA OR TEXT AS THE BOTS OWN WHATSAPP STATUS (STORY) AND THEN MENTIONS / SHARES THAT STATUS TO EVERY SINGLE GROUP THE BOT IS IN . GROUP MEMBERS GET THE MENTIONED YOUR GROUP IN THEIR STATUS NOTIFICATION WITH A PREVIEW .\n" +
				"\n" +
				"*HOW TO USE IT — 3 WAYS :*\n" +
				"\n" +
				"*1) MEDIA STATUS*\n" +
				"SEND OR REPLY TO ANY PHOTO / VIDEO / AUDIO / MUSIC / FILE AND TYPE .gcstatus\n" +
				"EXAMPLE : SEND A PHOTO + CAPTION IT .gcstatus\n" +
				"\n" +
				"*2) TEXT STATUS*\n" +
				"TYPE ANY TEXT ALONGSIDE THE COMMAND\n" +
				"EXAMPLE : .gcstatus Hello this is my status\n" +
				"\n" +
				"*3) REPLY STATUS*\n" +
				"REPLY TO ANY MESSAGE (TEXT / MEDIA) AND TYPE .gcstatus\n" +
				"THE REPLIED MESSAGE CONTENT BECOMES THE STATUS\n" +
				"\n" +
				"*NOTE :*\n" +
				"🔹 OWNER ONLY COMMAND\n" +
				"🔹 STATUS IS EPHEMERAL (DISAPPEARS AFTER 24 HOURS)\n" +
				"🔹 ALL GROUPS ARE MENTIONED AUTOMATICALLY\n" +
				"\n" +
				"*🔰 GOLD-MD 🔰*"
			s.Reply(info, guidance)
			return
		}
	}

	// ── Step 2: Build the status message proto ──
	var statusMsg *waProto.Message
	var statusType string

	if hasMedia && len(mediaData) > 0 {
		statusMsg, statusType = buildStatusMediaProto(s, mediaData, mediaMime, caption)
	} else {
		// Text-only status. WhatsApp renders text statuses as a coloured
		// background card -- needs BackgroundArgb (ARGB uint32) + Font, not
		// just Text (a bare ExtendedTextMessage with only Text is treated
		// as a chat message and silently dropped from the story).
		text := caption
		// If no caption, fall back to the quoted/replied-to message text.
		// This handles: user replies to a text message + types .gcstatus.
		if text == "" {
			text = s.GetQuotedMessageText(info)
			//			JSONDebug("GCSTATUS_QUOTED_TEXT", map[string]any{"quotedText": text})
		}
		if text == "" {
			text = "📌 Status via GOLD-MD"
		}
		// Default background = WhatsApp green (#0E9C6B -> ARGB 0xFF0E9C6B)
		// and a standard system font.
		statusMsg = &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:           proto.String(text),
				BackgroundArgb: proto.Uint32(0xFF0E9C6B),
				Font:           waProto.ExtendedTextMessage_SYSTEM.Enum(),
			},
		}
		statusType = "text"
	}

	if statusMsg == nil {
		//		JSONDebug("GCSTATUS_BUILD_FAIL", map[string]any{"reason": "status proto nil"})
		s.Reply(info, "❌ *GCSTATUS: media build failed, could not create status.*")
		return
	}

	//	JSONDebug("GCSTATUS_BUILT", map[string]any{
	//		"type":     statusType,
	//		"caption":  caption,
	//		"hasMedia": hasMedia,
	//	})

	// ── Step 3: Get all groups first (to pass as mentionedGroupJIDs) ──
	groups, err := s.GetJoinedGroupsList()
	if err != nil {
		//		JSONDebug("GCSTATUS_GROUPS_FAIL", map[string]any{"error": err.Error()})
		s.Reply(info, "❌ *GCSTATUS: failed to fetch groups: "+err.Error()+"*")
		return
	}
	if len(groups) == 0 {
		//		JSONDebug("GCSTATUS_NO_GROUPS", map[string]any{})
		s.Reply(info, "⚠️ *GCSTATUS: bot is not in any group. Status posted but no groups to mention.*")
		// Still post the status
		statusID, perr := s.PostStatusToBroadcast(statusMsg, nil)
		if perr != nil {
			s.Reply(info, "❌ *GCSTATUS: status post failed: "+perr.Error()+"*")
			return
		}
		s.Reply(info, "🷲 *GCSTATUS status posted (no groups to mention).*\n*Status ID:* "+statusID)
		return
	}

	var groupJIDStrs []string
	for _, g := range groups {
		groupJIDStrs = append(groupJIDStrs, g.String())
	}
	//	JSONDebug("GCSTATUS_GROUPS_FOUND", map[string]any{
	//		"count":  len(groups),
	//		"groups": groupJIDStrs,
	//	})

	// ── Step 4: Post status to status@broadcast with mentioned groups ──
	statusID, err := s.PostStatusToBroadcast(statusMsg, groups)
	if err != nil {
		//		JSONDebug("GCSTATUS_POST_FAIL", map[string]any{"error": err.Error()})
		s.Reply(info, "❌ *GCSTATUS: status post failed: "+err.Error()+"*")
		return
	}
	//	JSONDebug("GCSTATUS_POSTED", map[string]any{
	//		"statusID":  statusID,
	//		"groupsLen": len(groups),
	//	})

	// ── Step 5: Mention status to each group ──
	s.Reply(info, "🷲 *GCSTATUS status posted!*\n*Mentioning to "+itoa(len(groups))+" groups...*\n*Status ID:* "+statusID)

	okCount := 0
	failCount := 0
	var failedGroups []string
	for _, gid := range groups {
		//		JSONDebug("GCSTATUS_MENTION_LOOP", map[string]any{
		//			"index":    i,
		//			"total":    len(groups),
		//			"group":    gid.String(),
		//			"statusID": statusID,
		//		})
		if err := s.MentionStatusToGroup(gid, statusID); err != nil {
			failCount++
			failedGroups = append(failedGroups, gid.String())
			//			JSONDebug("GCSTATUS_MENTION_FAIL", map[string]any{
			//				"group": gid.String(),
			//				"error": err.Error(),
			//			})
		} else {
			okCount++
		}
	}

	// -- Step 5b: Send GREEN RING group story to each group --
	// This wraps the actual status media inside GroupStatusMessageV2 (field
	// 103) and sends it directly to the group JID. This creates the green
	// ring group story on the group profile picture -- the group status
	// story that members see when they tap the group DP. This is in
	// ADDITION to the mention notification above (field 92).
	//	JSONDebug("GCSTATUS_STORY_LOOP_START", map[string]any{
	//		"total":      len(groups),
	//		"statusType": statusType,
	//	})
	storyOK := 0
	storyFail := 0
	var storyFailedGroups []string
	for _, gid := range groups {
		//		JSONDebug("GCSTATUS_STORY_LOOP", map[string]any{
		//			"index": i,
		//			"total": len(groups),
		//			"group": gid.String(),
		//		})
		if err := s.SendGroupStory(gid, statusMsg); err != nil {
			storyFail++
			storyFailedGroups = append(storyFailedGroups, gid.String())
			//			JSONDebug("GCSTATUS_STORY_FAIL", map[string]any{
			//				"group": gid.String(),
			//				"error": err.Error(),
			//			})
		} else {
			storyOK++
		}
	}

	//	JSONDebug("GCSTATUS_DONE", map[string]any{
	//		"statusID":         statusID,
	//		"total":            len(groups),
	//		"mentioned":        okCount,
	//		"failed":           failCount,
	//		"statusType":       statusType,
	//		"failedGroups":     failedGroups,
	//		"storyGreenRingOK": storyOK,
	//		"storyGreenRingFail": storyFail,
	//		"storyFailedGroups":  storyFailedGroups,
	//	})

	var sb strings.Builder
	sb.WriteString("✅ *GCSTATUS COMPLETE*\n")
	sb.WriteString("\n*Status Type:* " + statusType)
	sb.WriteString("\n*Status ID:* " + statusID)
	sb.WriteString("\n*Groups Total:* " + itoa(len(groups)))
	sb.WriteString("\n*Mentioned:* " + itoa(okCount) + " ✅")
	if failCount > 0 {
		sb.WriteString("\n*Failed:* " + itoa(failCount) + " ❌")
	}
	sb.WriteString("\n*Story Green Ring:* " + itoa(storyOK) + " ✅")
	if storyFail > 0 {
		sb.WriteString("\n*Story Failed:* " + itoa(storyFail) + " ❌")
	}
	s.Reply(info, sb.String())
}

// ---------------------------------------------------------------------------
// Media upload + proto builders
// ---------------------------------------------------------------------------

// buildStatusMediaProto uploads mediaData to WhatsApp servers and returns a
// status message proto suitable for sending to status@broadcast. Detects
// image/video/audio/document by mimetype and builds the right proto with
// caption.
func buildStatusMediaProto(s SessionBridge, data []byte, mime string, caption string) (*waProto.Message, string) {
	client := s.GetClient()
	if client == nil {
		//		JSONDebug("GCSTATUS_BUILD_NOCLIENT", map[string]any{})
		return nil, ""
	}

	mimeLower := strings.ToLower(mime)
	//	JSONDebug("GCSTATUS_UPLOAD_START", map[string]any{
	//		"mime":  mimeLower,
	//		"bytes": len(data),
	//	})

	ctx := context.Background()

	// ── Image (jpg/png/etc, NOT webp sticker) ──
	if strings.HasPrefix(mimeLower, "image/") && mimeLower != "image/webp" {
		return uploadAndBuildImage(ctx, client, data, mime, caption)
	}

	// ── Video (mp4, gif, etc.) ──
	if strings.HasPrefix(mimeLower, "video/") {
		return uploadAndBuildVideo(ctx, client, data, mime, caption)
	}

	// ── Audio (music/voice) ──
	if strings.HasPrefix(mimeLower, "audio/") {
		return uploadAndBuildAudio(ctx, client, data, mime, caption)
	}

	// ── Sticker (webp) → send as image status ──
	if mimeLower == "image/webp" {
		return uploadAndBuildImage(ctx, client, data, "image/png", caption)
	}

	// ── Document / file / fallback ──
	return uploadAndBuildDocument(ctx, client, data, mime, caption)
}

// uploadAndBuildImage uploads image bytes and builds an ImageMessage proto.
func uploadAndBuildImage(ctx context.Context, client *whatsmeow.Client, data []byte, mime string, caption string) (*waProto.Message, string) {
	resp, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		//		JSONDebug("GCSTATUS_UPLOAD_IMG_ERR", map[string]any{"error": err.Error(), "bytes": len(data)})
		return nil, ""
	}
	//	JSONDebug("GCSTATUS_UPLOAD_IMG_OK", map[string]any{
	//		"url":        resp.URL,
	//		"directPath": resp.DirectPath,
	//		"fileLength": resp.FileLength,
	//	})

	imgMsg := &waProto.ImageMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(resp.FileLength),
		Mimetype:      proto.String(mime),
	}
	if caption != "" {
		imgMsg.Caption = proto.String(caption)
	}
	return &waProto.Message{ImageMessage: imgMsg}, "image"
}

// uploadAndBuildVideo uploads video bytes and builds a VideoMessage proto.
func uploadAndBuildVideo(ctx context.Context, client *whatsmeow.Client, data []byte, mime string, caption string) (*waProto.Message, string) {
	resp, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		//		JSONDebug("GCSTATUS_UPLOAD_VID_ERR", map[string]any{"error": err.Error(), "bytes": len(data)})
		return nil, ""
	}
	//	JSONDebug("GCSTATUS_UPLOAD_VID_OK", map[string]any{
	//		"url":        resp.URL,
	//		"directPath": resp.DirectPath,
	//		"fileLength": resp.FileLength,
	//	})

	vidMsg := &waProto.VideoMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(resp.FileLength),
		Mimetype:      proto.String(mime),
	}
	if caption != "" {
		vidMsg.Caption = proto.String(caption)
	}
	return &waProto.Message{VideoMessage: vidMsg}, "video"
}

// uploadAndBuildAudio uploads audio bytes and builds an AudioMessage proto.
func uploadAndBuildAudio(ctx context.Context, client *whatsmeow.Client, data []byte, mime string, caption string) (*waProto.Message, string) {
	resp, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		//		JSONDebug("GCSTATUS_UPLOAD_AUD_ERR", map[string]any{"error": err.Error(), "bytes": len(data)})
		return nil, ""
	}
	//	JSONDebug("GCSTATUS_UPLOAD_AUD_OK", map[string]any{
	//		"url":        resp.URL,
	//		"directPath": resp.DirectPath,
	//		"fileLength": resp.FileLength,
	//	})

	audMsg := &waProto.AudioMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(resp.FileLength),
		Mimetype:      proto.String(mime),
		PTT:           proto.Bool(false),
	}
	// captions aren't a field on AudioMessage; log it for debug
	_ = caption
	return &waProto.Message{AudioMessage: audMsg}, "audio"
}

// uploadAndBuildDocument uploads document bytes and builds a DocumentMessage
// proto. Used for any file that isn't image/video/audio.
func uploadAndBuildDocument(ctx context.Context, client *whatsmeow.Client, data []byte, mime string, caption string) (*waProto.Message, string) {
	resp, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		//		JSONDebug("GCSTATUS_UPLOAD_DOC_ERR", map[string]any{"error": err.Error(), "bytes": len(data)})
		return nil, ""
	}
	//	JSONDebug("GCSTATUS_UPLOAD_DOC_OK", map[string]any{
	//		"url":        resp.URL,
	//		"directPath": resp.DirectPath,
	//		"fileLength": resp.FileLength,
	//	})

	title := caption
	if title == "" {
		title = "file"
	}
	docMsg := &waProto.DocumentMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(resp.FileLength),
		Mimetype:      proto.String(mime),
		Title:         proto.String(title),
		FileName:      proto.String(title),
	}
	if caption != "" {
		docMsg.Caption = proto.String(caption)
	}
	return &waProto.Message{DocumentMessage: docMsg}, "document"
}

// stripCommandFromText removes the command name (and prefix-adjacent) from
// the body text so the caption doesn't include ".gcstatus".
func stripCommandFromText(text, cmd string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, cmd)
	if idx < 0 {
		return text
	}
	// remove the command word + any leading prefix char right before it
	end := idx + len(cmd)
	// also trim a leading prefix char like '.' '!' '#' before the command
	start := idx
	if start > 0 {
		p := text[start-1]
		if p == '.' || p == '!' || p == '#' || p == '/' || p == ':' {
			start--
		}
	}
	out := text[:start] + text[end:]
	return strings.TrimSpace(out)
}
