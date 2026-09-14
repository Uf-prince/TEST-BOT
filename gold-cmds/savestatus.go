package goldcmds

// ============================================================================
// GOLD-MD — .save command (save ANYTHING to your own inbox)
// File: savestatus.go
// ============================================================================
// Ported from UMAR-MD plugins/savestatus.js — SAME WORK (0% farak):
//   .save (reply to a STATUS / broadcast / any message) → saves it to
//        your own inbox (the bot's saved-messages chat)
//   .save (send media/text/link directly with the caption) → saves
//        that message itself
//
// Works for: image / video / audio / voice note / gif / sticker /
// document / plain text / links. Download from the stanza (quoted
// first, else self), re-upload to the bot's own JID. Raw forward,
// no re-encoding — instant regardless of codec.
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const saveHelpText = "*❮ SAVE ❯ — SAVE ANYTHING TO YOUR INBOX*\n\n" +
	"*REPLY TO A STATUS / BROADCAST / ANY MESSAGE, THEN TYPE*\n" +
	"*SAVE\n\n" +
	"*TO SAVE MESSAG AND WILL BE SENTED TO YOUR (YOU) INBOX*" +
	"*WORKS FOR IMAGE / VIDEO / AUDIO / VOICE NOTE / GIF / STICKER / DOCUMENT / TEXT / LINKS EVERYTHING*\n"

// saveGetText extracts text/link content from a target proto message —
// plain text, extendedText (links), or a media caption.
func saveGetText(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	if m.Conversation != nil {
		return *m.Conversation
	}
	if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.Text != nil {
		return *m.ExtendedTextMessage.Text
	}
	if m.ImageMessage != nil && m.ImageMessage.Caption != nil {
		return *m.ImageMessage.Caption
	}
	if m.VideoMessage != nil && m.VideoMessage.Caption != nil {
		return *m.VideoMessage.Caption
	}
	if m.DocumentMessage != nil && m.DocumentMessage.Caption != nil {
		return *m.DocumentMessage.Caption
	}
	return ""
}

// saveTargetKind detects the media type of a proto message.
type saveMediaKind int

const (
	saveKindNone saveMediaKind = iota
	saveKindImage
	saveKindVideo
	saveKindAudio
	saveKindSticker
	saveKindDocument
)

func saveDetectKind(m *waProto.Message) saveMediaKind {
	if m == nil {
		return saveKindNone
	}
	if m.ImageMessage != nil {
		return saveKindImage
	}
	if m.VideoMessage != nil {
		return saveKindVideo
	}
	if m.AudioMessage != nil {
		return saveKindAudio
	}
	if m.StickerMessage != nil {
		return saveKindSticker
	}
	if m.DocumentMessage != nil {
		return saveKindDocument
	}
	return saveKindNone
}

// saveQuotedOf returns the quoted (replied-to) proto message of the
// command message, if any.
func saveQuotedOf(raw *waProto.Message) *waProto.Message {
	if raw == nil {
		return nil
	}
	if raw.ExtendedTextMessage != nil && raw.ExtendedTextMessage.ContextInfo != nil &&
		raw.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
		return raw.ExtendedTextMessage.ContextInfo.QuotedMessage
	}
	return nil
}

// saveDownloadTarget downloads the media of the target message (quoted
// first, else the command message itself) using the bridge's
// DownloadQuotedMedia (quoted) or a manual client download (self).
// Returns bytes, mimetype, kind, ok.
func saveDownloadTarget(s SessionBridge, info types.MessageInfo) ([]byte, string, saveMediaKind, bool) {
	// Quoted first — bridge helper handles view-once wrappers too.
	if data, mime, ok := s.DownloadQuotedMedia(info); ok && len(data) > 0 {
		raw := s.GetRawMessage(info)
		kind := saveKindNone
		if qm := saveQuotedOf(raw); qm != nil {
			kind = saveDetectKind(qm)
		}
		return data, mime, kind, true
	}

	// Self — the command message itself carries the media.
	raw := s.GetRawMessage(info)
	if raw == nil {
		return nil, "", saveKindNone, false
	}
	kind := saveDetectKind(raw)
	if kind == saveKindNone {
		return nil, "", saveKindNone, false
	}
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		return nil, "", saveKindNone, false
	}
	data, err := saveDownloadFromProto(cli, raw)
	if err != nil || len(data) == 0 {
		return nil, "", saveKindNone, false
	}
	mime := saveMimetypeOf(raw)
	return data, mime, kind, true
}

// saveDownloadFromProto performs a manual client download of the media
// attached directly to a proto message.
func saveDownloadFromProto(cli *whatsmeow.Client, m *waProto.Message) ([]byte, error) {
	switch {
	case m.ImageMessage != nil:
		return cli.Download(context.Background(), m.ImageMessage)
	case m.VideoMessage != nil:
		return cli.Download(context.Background(), m.VideoMessage)
	case m.AudioMessage != nil:
		return cli.Download(context.Background(), m.AudioMessage)
	case m.StickerMessage != nil:
		return cli.Download(context.Background(), m.StickerMessage)
	case m.DocumentMessage != nil:
		return cli.Download(context.Background(), m.DocumentMessage)
	}
	return nil, fmt.Errorf("no media")
}

// saveMimetypeOf returns the mimetype of the media in a proto message.
func saveMimetypeOf(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	if m.ImageMessage != nil && m.ImageMessage.Mimetype != nil {
		return *m.ImageMessage.Mimetype
	}
	if m.VideoMessage != nil && m.VideoMessage.Mimetype != nil {
		return *m.VideoMessage.Mimetype
	}
	if m.AudioMessage != nil && m.AudioMessage.Mimetype != nil {
		return *m.AudioMessage.Mimetype
	}
	if m.StickerMessage != nil && m.StickerMessage.Mimetype != nil {
		return *m.StickerMessage.Mimetype
	}
	if m.DocumentMessage != nil && m.DocumentMessage.Mimetype != nil {
		return *m.DocumentMessage.Mimetype
	}
	return ""
}

// saveSelfEditOnce sends a message, then edits it into itself once
// (Node.js UmarSelfEditOnce — protocolMessage type 14 edit relay).
// It re-declares the sent message's own content as the editedMessage,
// exactly like the Node.js version reuses sentMsg.message. Best-effort:
// edit errors are swallowed — the original send already succeeded.
func saveSelfEditOnce(cli *whatsmeow.Client, chat types.JID, resp whatsmeow.SendResponse, sent *waProto.Message) {
	if cli == nil || !cli.IsConnected() {
		return
	}
	if resp.ID == "" || sent == nil {
		return
	}
	// whatsmeow official BuildEdit = protocolMessage type 14 wrapper.
	edited := cli.BuildEdit(chat, resp.ID, sent)
	if edited == nil {
		return
	}
	// Best-effort — failure is ignored (original send still stands).
	_, _ = cli.SendMessage(context.Background(), chat, edited)
}

// handleSave is the .save command entry point.
func handleSave(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSaveAsync(s, info, args, prefix)
}

func handleSaveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ client not ready*")
		return
	}

	// Own inbox = the bot's own JID, device suffix stripped (the
	// Node.js jidNormalizedUser(devtrust.user.id)).
	ownJIDRaw := strings.SplitN(s.GetJID(), ":", 2)[0] + "@s.whatsapp.net"
	ownJID, err := types.ParseJID(ownJIDRaw)
	if err != nil {
		s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ invalid own JID*")
		return
	}

	// ── download target (quoted first, else self) ──
	data, mime, kind, ok := saveDownloadTarget(s, info)

	// Plain-text / link — no media: send text to own inbox.
	if !ok || kind == saveKindNone {
		raw := s.GetRawMessage(info)
		var text string
		if raw != nil {
			if qm := saveQuotedOf(raw); qm != nil {
				text = saveGetText(qm)
			}
			if text == "" {
				text = saveGetText(raw)
			}
		}
		if text == "" {
			s.Reply(info, saveHelpText)
			return
		}
		msg := &waProto.Message{
			Conversation: proto.String(text),
		}
		resp, err := cli.SendMessage(context.Background(), ownJID, msg)
		if err != nil {
			s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ "+err.Error()+"*")
			return
		}
		saveSelfEditOnce(cli, ownJID, resp, msg)
		s.Reply(info, "*🔰 SAVED TO YOUR INBOX*\n *CHEK YOUR (YOU) INBOX*")
		return
	}

	if len(data) == 0 {
		s.Reply(info, "*THIS HAS EXPIRED OR IS NO LONGER AVAILABLE*")
		return
	}

	// ── upload + send media to own inbox ──
	var uploadType whatsmeow.MediaType
	switch kind {
	case saveKindImage:
		uploadType = whatsmeow.MediaImage
	case saveKindVideo:
		uploadType = whatsmeow.MediaVideo
	case saveKindAudio:
		uploadType = whatsmeow.MediaAudio
	case saveKindSticker:
		// whatsmeow has no MediaSticker — stickers upload with the
		// image key type (same as the bridge's SendSticker).
		uploadType = whatsmeow.MediaImage
	case saveKindDocument:
		uploadType = whatsmeow.MediaDocument
	default:
		s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ unsupported media type*")
		return
	}

	resp, err := cli.Upload(context.Background(), data, uploadType)
	if err != nil {
		s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ "+err.Error()+"*")
		return
	}

	// Caption (raw caption of the media — a direct client send, so the
	// footer is NOT applied; raw-forward semantics like the Node.js
	// relayMessage path).
	raw := s.GetRawMessage(info)
	caption := saveCaptionOf(raw)

	var outMsg *waProto.Message
	switch kind {
	case saveKindImage:
		outMsg = &waProto.Message{ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(mime),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			Caption:       proto.String(caption),
		}}
	case saveKindVideo:
		seconds, w, h := saveVideoMeta(raw)
		outMsg = &waProto.Message{VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(mime),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			Caption:       proto.String(caption),
			Seconds:       proto.Uint32(seconds),
			Width:         proto.Uint32(w),
			Height:        proto.Uint32(h),
			GifPlayback:   proto.Bool(saveGifFlag(raw)),
		}}
	case saveKindAudio:
		// Voice notes carry the ptt flag — keep it faithful.
		ptt := saveAudioPtt(raw)
		mimetype := mime
		if mimetype == "" {
			mimetype = "audio/mp4"
		}
		outMsg = &waProto.Message{AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(mimetype),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			PTT:           proto.Bool(ptt),
		}}
	case saveKindSticker:
		outMsg = &waProto.Message{StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(mime),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
		}}
	case saveKindDocument:
		fileName := saveDocFileName(raw)
		if fileName == "" {
			fileName = "saved_file"
		}
		docMime := mime
		if docMime == "" {
			docMime = "application/octet-stream"
		}
		outMsg = &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(docMime),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileName:      proto.String(fileName),
			Caption:       proto.String(caption),
		}}
	}

	sendResp, err := cli.SendMessage(context.Background(), ownJID, outMsg)
	if err != nil {
		errMsg := err.Error()
		if saveIsExpiredErr(errMsg) {
			s.Reply(info, "*THIS HAS EXPIRED OR IS NO LONGER AVAILABLE*")
		} else {
			s.Reply(info, "*🔰 FAILED TO SAVE*\n\n*REASON:➭ "+errMsg+"*")
		}
		return
	}
	saveSelfEditOnce(cli, ownJID, sendResp, outMsg)

	s.Reply(info, "*🔰 SAVED TO YOUR INBOX*\n*CHECK YOUR (YOU) INBOX*")
}

// saveIsExpiredErr matches the Node.js expired-media error sniffing.
func saveIsExpiredErr(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "expired") ||
		strings.Contains(m, "gone") ||
		strings.Contains(m, "404") ||
		strings.Contains(m, "not found") ||
		strings.Contains(m, "media key") ||
		strings.Contains(m, "fetch failed") ||
		strings.Contains(m, "decrypt")
}

// saveCaptionOf pulls the caption of the target media (quoted first).
func saveCaptionOf(raw *waProto.Message) string {
	if raw != nil {
		if qm := saveQuotedOf(raw); qm != nil {
			if c := saveMediaCaption(qm); c != "" {
				return c
			}
		}
		if c := saveMediaCaption(raw); c != "" {
			return c
		}
	}
	return ""
}

func saveMediaCaption(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	if m.ImageMessage != nil && m.ImageMessage.Caption != nil {
		return *m.ImageMessage.Caption
	}
	if m.VideoMessage != nil && m.VideoMessage.Caption != nil {
		return *m.VideoMessage.Caption
	}
	if m.DocumentMessage != nil && m.DocumentMessage.Caption != nil {
		return *m.DocumentMessage.Caption
	}
	return ""
}

// saveVideoMeta returns (seconds, width, height) of the target video.
func saveVideoMeta(raw *waProto.Message) (uint32, uint32, uint32) {
	vm := saveFindVideoMessage(raw)
	if vm == nil {
		return 0, 0, 0
	}
	seconds := uint32(0)
	if vm.Seconds != nil {
		seconds = *vm.Seconds
	}
	w := uint32(0)
	if vm.Width != nil {
		w = *vm.Width
	}
	h := uint32(0)
	if vm.Height != nil {
		h = *vm.Height
	}
	return seconds, w, h
}

// saveGifFlag reports whether the target video is a gif.
func saveGifFlag(raw *waProto.Message) bool {
	vm := saveFindVideoMessage(raw)
	if vm == nil {
		return false
	}
	return vm.GifPlayback != nil && *vm.GifPlayback
}

// saveAudioPtt reports whether the target audio is a voice note (ptt).
func saveAudioPtt(raw *waProto.Message) bool {
	am := saveFindAudioMessage(raw)
	if am == nil {
		return false
	}
	return am.PTT != nil && *am.PTT
}

func saveFindVideoMessage(raw *waProto.Message) *waProto.VideoMessage {
	if raw == nil {
		return nil
	}
	if raw.VideoMessage != nil {
		return raw.VideoMessage
	}
	if qm := saveQuotedOf(raw); qm != nil && qm.VideoMessage != nil {
		return qm.VideoMessage
	}
	return nil
}

func saveFindAudioMessage(raw *waProto.Message) *waProto.AudioMessage {
	if raw == nil {
		return nil
	}
	if raw.AudioMessage != nil {
		return raw.AudioMessage
	}
	if qm := saveQuotedOf(raw); qm != nil && qm.AudioMessage != nil {
		return qm.AudioMessage
	}
	return nil
}

// saveDocFileName pulls the fileName of the target document (quoted first).
func saveDocFileName(raw *waProto.Message) string {
	dm := saveFindDocMessage(raw)
	if dm == nil || dm.FileName == nil {
		return ""
	}
	return *dm.FileName
}

func saveFindDocMessage(raw *waProto.Message) *waProto.DocumentMessage {
	if raw == nil {
		return nil
	}
	if raw.DocumentMessage != nil {
		return raw.DocumentMessage
	}
	if qm := saveQuotedOf(raw); qm != nil && qm.DocumentMessage != nil {
		return qm.DocumentMessage
	}
	return nil
}

// ── registration ────────────────────────────────────────────────────────────
func init() {
	Register(Command{
		Name:     "save",
		Category: "AI & MEDIA",
		Desc:     "THIS COMMAND IS USED TO DOWNLOAD AND SAVE ANY WHATSAPP STATUS MEDIA. REPLY TO A STATUS OR USE IT WITH A STATUS LINK.",
		Run:      handleSave,
	})
	// Hidden aliases wired to the same handler (Node.js: saved/send/give/safe
	// + .sv/.grab/.keep). NOTE: "s" is NOT registered here because it is
	// already the .sticker alias in sticker.go.
	Register(Command{Name: "sv", Hidden: true, Run: handleSave})
	Register(Command{Name: "grab", Hidden: true, Run: handleSave})
	Register(Command{Name: "keep", Hidden: true, Run: handleSave})
	Register(Command{Name: "saved", Hidden: true, Run: handleSave})
	Register(Command{Name: "send", Hidden: true, Run: handleSave})
	Register(Command{Name: "give", Hidden: true, Run: handleSave})
	Register(Command{Name: "safe", Hidden: true, Run: handleSave})
}
