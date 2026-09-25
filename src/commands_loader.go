package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	goldcmds "gold-md/gold-cmds"

	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type bridge struct {
	s *Session
	// cmdCtx is the running command's watchdog context (set by
	// RunWithTimeoutCmd / RunWithTimeoutDur). The guard compressor reads
	// it so ffmpeg is killed the instant the command timeout fires — the
	// compressor shares the SAME budget as the command (owner order).
	cmdCtx context.Context
}

func (b *bridge) GetClient() *whatsmeow.Client { return b.s.Client }
func (b *bridge) GetJID() string               { return b.s.JID }

// SetCmdContext stores the running command's watchdog context on the
// bridge (SessionBridge interface). The guard compressor uses it so its
// ffmpeg exec is cancelled together with the command timeout.
func (b *bridge) SetCmdContext(ctx context.Context) { b.cmdCtx = ctx }

// BeginGuard starts a per-session / per-user "latest wins" guard for the
// current command (SessionBridge interface). It is keyed by (session JID,
// user JID): a newer command from the SAME user on the SAME session cancels
// the previous one; other users / other sessions are never affected.
func (b *bridge) BeginGuard(userJID string) (context.Context, func()) {
	return BeginCmdGuard(b.s.JID, userJID)
}

// guardCtx returns the running command's watchdog context, or
// context.Background() when no command context is set (e.g. antidelete
// recovery outside a command).
func (b *bridge) guardCtx() context.Context {
	if b != nil && b.cmdCtx != nil {
		return b.cmdCtx
	}
	return context.Background()
}

// GetAllConnectedClients returns every currently-connected WhatsApp
// session known to the Manager. Each entry is a unique WhatsApp number
// that can independently react to a channel post (one account = +1 on
// the reaction counter). Used by .chreact to react from ALL sessions.
func (b *bridge) GetAllConnectedClients() []goldcmds.ConnectedClient {
	out := make([]goldcmds.ConnectedClient, 0)
	if b.s == nil || b.s.Manager == nil {
		return out
	}
	for _, sess := range b.s.Manager.List() {
		if sess == nil || sess.Client == nil {
			continue
		}
		if !sess.Client.IsConnected() {
			continue
		}
		out = append(out, goldcmds.ConnectedClient{
			JID:    sess.JID,
			Client: sess.Client,
		})
	}
	return out
}

func (b *bridge) Reply(info types.MessageInfo, text string) {
	b.s.Reply(info, text)
}

// ReplyWithID sends a message and returns its message ID (for later edit/delete).
func (b *bridge) ReplyWithID(info types.MessageInfo, text string) string {
	return b.s.SendTextWithID(info, text)
}

// ReplyWithMentions sends a text message that pings the supplied JIDs
// (ContextInfo.MentionedJID). Used by anti-* warn/kick notifications.
func (b *bridge) ReplyWithMentions(info types.MessageInfo, text string, mentioned []string) {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return
	}
	text = b.s.withFooter(text) // botname footer on every mention reply
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waProto.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	}
	_, _ = b.s.Client.SendMessage(context.Background(), info.Chat, msg)
}

// SendContact sends a WhatsApp Contact card (vCard) to the chat.
// displayName is the name shown on the contact card, number is the
// phone number (digits only, international format without +).
func (b *bridge) SendContact(info types.MessageInfo, displayName string, number string) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	vcard := "BEGIN:VCARD\n" +
		"VERSION:3.0\n" +
		"FN:" + displayName + "\n" +
		"TEL;type=CELL;type=VOICE;waid=" + number + ":" + number + "\n" +
		"END:VCARD"
	msg := &waProto.Message{
		ContactMessage: &waProto.ContactMessage{
			DisplayName: proto.String(displayName),
			Vcard:       proto.String(vcard),
			ContextInfo: &waProto.ContextInfo{},
		},
	}
	_, err := b.s.Client.SendMessage(context.Background(), info.Chat, msg)
	return err
}

// EditMessage edits an existing message by ID.
func (b *bridge) EditMessage(info types.MessageInfo, messageID string, newText string) bool {
	return b.s.EditMessage(info, messageID, newText)
}

// DeleteMessage deletes a message by ID.
func (b *bridge) DeleteMessage(info types.MessageInfo, messageID string) error {
	return b.s.DeleteMsg(info, messageID)
}

// SendVideo uploads and sends a video message.
func (b *bridge) SendVideo(info types.MessageInfo, data []byte, caption string, thumbnail []byte, seconds uint32, width uint32, height uint32) error {
	// GUARD (Render bandwidth shield): 50MB+ video → compressor room
	g := b.guardBytes(info, guardVideo, data, caption)
	if g.blocked() {
		return nil // guard ne chat me block message bhej diya
	}
	defer g.cleanupAll()
	if len(g.data) > 0 {
		data = g.data
		caption += g.note
		// bytes flow: compressed mp4 ka duration/w/h re-probe (temp file)
		secs, w, h := guardProbeBytesMeta(data)
		if secs > 0 {
			seconds, width, height = secs, w, h
		}
	}
	caption = b.s.withCaptionFooter(caption) // botname footer on every video
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}

	videoMsg := &waProto.VideoMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		Mimetype:      proto.String("video/mp4"),
		MediaKey:      resp.MediaKey,
		FileLength:    proto.Uint64(uint64(len(data))),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
		JPEGThumbnail: thumbnail,
		Seconds:       proto.Uint32(seconds),
		Height:        proto.Uint32(height),
		Width:         proto.Uint32(width),
		GifPlayback:   proto.Bool(false),
	}

	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		VideoMessage: videoMsg,
	})
	return err
}

// sendVideoFileCore uploads and sends a video file WITHOUT running the guard
// compressor. Callers must have already compressed the file (or decided not
// to). Kept private so the guard policy stays in one place.
func (b *bridge) sendVideoFileCore(info types.MessageInfo, path string, caption string, thumbnail []byte, seconds uint32, width uint32, height uint32) error {
	caption = b.s.withCaptionFooter(caption) // botname footer on every video
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	videoMsg := &waProto.VideoMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath), Caption: proto.String(caption),
		Mimetype: proto.String("video/mp4"), MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength),
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256, JPEGThumbnail: thumbnail,
		Seconds: proto.Uint32(seconds), Height: proto.Uint32(height), Width: proto.Uint32(width), GifPlayback: proto.Bool(false),
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{VideoMessage: videoMsg})
	return err
}

// SendVideoFile uploads and sends a video from a seekable file without loading it into RAM.
func (b *bridge) SendVideoFile(info types.MessageInfo, path string, caption string, thumbnail []byte, seconds uint32, width uint32, height uint32) error {
	// GUARD (Render bandwidth shield): 50MB+ video → compressor room
	g := b.guardPath(info, guardVideo, path, caption)
	if g.blocked() {
		return nil // guard ne chat me block message bhej diya
	}
	defer g.cleanupAll()
	if g.usePath {
		path = g.path
		caption += g.note
		secs, w, h := guardProbeMeta(path)
		if secs > 0 {
			seconds = secs
			width, height = w, h
		}
	}
	return b.sendVideoFileCore(info, path, caption, thumbnail, seconds, width, height)
}

// SendVideoThumbFirst is the OWNER-ORDERED send flow for .play/.play2/.video/
// .video2: the video is FULLY compressed FIRST (guard), and only once it is
// ready to send does the bot send the thumbnail + info caption, followed by
// the already-compressed video. This guarantees the thumbnail can never
// arrive before compression has finished. The video itself carries NO caption
// (the author/comments/views info lives on the thumbnail).
func (b *bridge) SendVideoThumbFirst(info types.MessageInfo, path string, caption string, thumbnail []byte) error {
	// STEP 1: GUARD compress FIRST — nothing is sent until this completes.
	g := b.guardPath(info, guardVideo, path, caption)
	if g.blocked() {
		return nil // guard ne chat me block message bhej diya
	}
	defer g.cleanupAll()
	if g.usePath {
		path = g.path
		caption += g.note
	}
	var seconds, width, height uint32
	if secs, w, h := guardProbeMeta(path); secs > 0 {
		seconds, width, height = secs, w, h
	}
	// STEP 2: send thumbnail + info caption FIRST (video is already compressed)
	if len(thumbnail) > 0 {
		_ = b.SendImage(info, thumbnail, caption)
	} else {
		b.Reply(info, caption)
	}
	// STEP 3: send the already-compressed video (no caption, no footer)
	return b.sendVideoFileCore(info, path, "", nil, seconds, width, height)
}

// sendAudioFileCore uploads and sends an audio file WITHOUT running the guard
// compressor. Callers must have already compressed the file (or decided not to).
func (b *bridge) sendAudioFileCore(info types.MessageInfo, path string, caption string, seconds uint32) error {
	// audio messages carry no visible caption on WhatsApp; footer is applied
	// only when a text caption is present
	caption = b.s.withCaptionFooter(caption)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{AudioMessage: &waProto.AudioMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath), Mimetype: proto.String("audio/mpeg"),
		MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength), FileSHA256: resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256, Seconds: proto.Uint32(seconds), PTT: proto.Bool(false),
	}})
	return err
}

// SendAudioFile uploads and sends audio from a seekable file without loading it into RAM.
func (b *bridge) SendAudioFile(info types.MessageInfo, path string, caption string, seconds uint32) error {
	// GUARD (Render bandwidth shield): 50MB+ audio → compressor room
	g := b.guardPath(info, guardAudio, path, caption)
	if g.blocked() {
		return nil
	}
	defer g.cleanupAll()
	if g.usePath {
		path = g.path
		// audio compressed → seconds re-probe
		if d := guardProbeDuration(path); d > 0 {
			seconds = uint32(d)
		}
	}
	return b.sendAudioFileCore(info, path, caption, seconds)
}

// SendAudioThumbFirst is the OWNER-ORDERED send flow for .play/.play2: the
// audio is FULLY compressed FIRST (guard), then the thumbnail + info caption
// is sent, and only then the already-compressed audio. Guarantees the
// thumbnail never arrives before compression finishes.
func (b *bridge) SendAudioThumbFirst(info types.MessageInfo, path string, caption string, thumbnail []byte) error {
	// STEP 1: GUARD compress FIRST — nothing is sent until this completes.
	g := b.guardPath(info, guardAudio, path, caption)
	if g.blocked() {
		return nil
	}
	defer g.cleanupAll()
	var seconds uint32
	if g.usePath {
		path = g.path
		caption += g.note
	}
	if d := guardProbeDuration(path); d > 0 {
		seconds = uint32(d)
	}
	// STEP 2: send thumbnail + info caption FIRST (audio is already compressed)
	if len(thumbnail) > 0 {
		_ = b.SendImage(info, thumbnail, caption)
	} else {
		b.Reply(info, caption)
	}
	// STEP 3: send the already-compressed audio (no caption, no footer)
	return b.sendAudioFileCore(info, path, "", seconds)
}

// SendVideoFileRaw is the .video3 raw variant of SendVideoFile: it runs the
// SAME guard compressor (bandwidth shield) but sends the video with NO
// caption, NO thumbnail and NO botname footer — just the bare video file.
func (b *bridge) SendVideoFileRaw(info types.MessageInfo, path string) error {
	// GUARD (Render bandwidth shield): same compressor policy as SendVideoFile.
	g := b.guardPath(info, guardVideo, path, "")
	if g.blocked() {
		return nil // guard ne chat me block message bhej diya
	}
	defer g.cleanupAll()
	var seconds, width, height uint32
	if g.usePath {
		path = g.path
		secs, w, h := guardProbeMeta(path)
		if secs > 0 {
			seconds, width, height = secs, w, h
		}
	} else {
		secs, w, h := guardProbeMeta(path)
		if secs > 0 {
			seconds, width, height = secs, w, h
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	// RAW: no caption, no thumbnail, no footer — sirf video.
	videoMsg := &waProto.VideoMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath),
		Mimetype: proto.String("video/mp4"), MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength),
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256,
		Seconds: proto.Uint32(seconds), Height: proto.Uint32(height), Width: proto.Uint32(width), GifPlayback: proto.Bool(false),
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{VideoMessage: videoMsg})
	return err
}

// SendAudioFileRaw is the .play3 raw variant of SendAudioFile: it runs the
// SAME guard compressor (bandwidth shield) but sends the audio with NO
// caption and NO botname footer — just the bare audio file.
func (b *bridge) SendAudioFileRaw(info types.MessageInfo, path string) error {
	// GUARD (Render bandwidth shield): same compressor policy as SendAudioFile.
	g := b.guardPath(info, guardAudio, path, "")
	if g.blocked() {
		return nil
	}
	defer g.cleanupAll()
	var seconds uint32
	if g.usePath {
		path = g.path
		if d := guardProbeDuration(path); d > 0 {
			seconds = uint32(d)
		}
	} else if d := guardProbeDuration(path); d > 0 {
		seconds = uint32(d)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	// RAW: no caption, no footer — sirf audio.
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{AudioMessage: &waProto.AudioMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath), Mimetype: proto.String("audio/mpeg"),
		MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength), FileSHA256: resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256, Seconds: proto.Uint32(seconds), PTT: proto.Bool(false),
	}})
	return err
}

// SendVideoFileRawWait is the .video3 OWNER-ORDERED raw variant: it runs the
// guard compressor FIRST (nothing is sent until compression is fully done),
// then invokes beforeSend (used to delete the waiting message), and only then
// sends the already-compressed RAW video (no caption / no thumbnail / no
// footer). This guarantees the waiting message stays visible until the video
// is fully downloaded AND compressed, and is removed only at send time.
func (b *bridge) SendVideoFileRawWait(info types.MessageInfo, path string, beforeSend func()) error {
	// GUARD (Render bandwidth shield): same compressor policy as SendVideoFile.
	g := b.guardPath(info, guardVideo, path, "")
	if g.blocked() {
		if beforeSend != nil {
			beforeSend()
		}
		return nil // guard ne chat me block message bhej diya
	}
	defer g.cleanupAll()
	var seconds, width, height uint32
	if g.usePath {
		path = g.path
	}
	if secs, w, h := guardProbeMeta(path); secs > 0 {
		seconds, width, height = secs, w, h
	}
	// Compression FULLY done -> now (and only now) remove the waiting message.
	if beforeSend != nil {
		beforeSend()
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	// RAW: no caption, no thumbnail, no footer - sirf video.
	videoMsg := &waProto.VideoMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath),
		Mimetype: proto.String("video/mp4"), MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength),
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256,
		Seconds: proto.Uint32(seconds), Height: proto.Uint32(height), Width: proto.Uint32(width), GifPlayback: proto.Bool(false),
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{VideoMessage: videoMsg})
	return err
}

// SendAudioFileRawWait is the .play3 OWNER-ORDERED raw variant: it runs the
// guard compressor FIRST (nothing is sent until compression is fully done),
// then invokes beforeSend (used to delete the waiting message), and only then
// sends the already-compressed RAW audio (no caption / no footer). This
// guarantees the waiting message stays visible until the audio is fully
// downloaded AND compressed, and is removed only at send time.
func (b *bridge) SendAudioFileRawWait(info types.MessageInfo, path string, beforeSend func()) error {
	// GUARD (Render bandwidth shield): same compressor policy as SendAudioFile.
	g := b.guardPath(info, guardAudio, path, "")
	if g.blocked() {
		if beforeSend != nil {
			beforeSend()
		}
		return nil
	}
	defer g.cleanupAll()
	var seconds uint32
	if g.usePath {
		path = g.path
	}
	if d := guardProbeDuration(path); d > 0 {
		seconds = uint32(d)
	}
	// Compression FULLY done -> now (and only now) remove the waiting message.
	if beforeSend != nil {
		beforeSend()
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	// RAW: no caption, no footer - sirf audio.
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{AudioMessage: &waProto.AudioMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath), Mimetype: proto.String("audio/mpeg"),
		MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength), FileSHA256: resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256, Seconds: proto.Uint32(seconds), PTT: proto.Bool(false),
	}})
	return err
}

// SendAudio uploads and sends an audio message.
func (b *bridge) SendAudio(info types.MessageInfo, data []byte, caption string, seconds uint32) error {
	// GUARD (Render bandwidth shield): 50MB+ audio → compressor room
	g := b.guardBytes(info, guardAudio, data, caption)
	if g.blocked() {
		return nil
	}
	defer g.cleanupAll()
	if len(g.data) > 0 {
		data = g.data
		if d := guardProbeBytes(data); d > 0 {
			seconds = uint32(d)
		}
	}
	caption = b.s.withCaptionFooter(caption)
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}

	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String("audio/mpeg"),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			Seconds:       proto.Uint32(seconds),
			PTT:           proto.Bool(false),
		},
	})
	return err
}

// SendImage uploads and sends an image message (used for thumbnails).
func (b *bridge) SendImage(info types.MessageInfo, data []byte, caption string) error {
	// GUARD (Render bandwidth shield): 50MB+ image → compressor room
	g := b.guardBytes(info, guardImage, data, caption)
	if g.blocked() {
		return nil
	}
	defer g.cleanupAll()
	if len(g.data) > 0 {
		data = g.data
		caption += g.note
	}
	caption = b.s.withCaptionFooter(caption) // botname footer on every image
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return err
	}

	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Caption:       proto.String(caption),
			Mimetype:      proto.String("image/jpeg"),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
		},
	})
	return err
}

// SendGif sends an MP4 as a WhatsApp looping GIF (VideoMessage with
// GifPlayback=true). The caller (breaction/greaction) converts the source
// .gif to mp4 first; here we just upload + send it with the gif flag so
// WhatsApp loops it like a GIF. No caption footer is added (reaction cards
// carry their own caption).
func (b *bridge) SendGif(info types.MessageInfo, data []byte, caption string, seconds uint32, width uint32, height uint32) error {
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	videoMsg := &waProto.VideoMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		Caption:       proto.String(caption),
		Mimetype:      proto.String("video/mp4"),
		MediaKey:      resp.MediaKey,
		FileLength:    proto.Uint64(uint64(len(data))),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
		Seconds:       proto.Uint32(seconds),
		Height:        proto.Uint32(height),
		Width:         proto.Uint32(width),
		GifPlayback:   proto.Bool(true),
	}
	if caption != "" {
		videoMsg.Caption = proto.String(caption)
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		VideoMessage: videoMsg,
	})
	return err
}

// DownloadImage downloads the image attached to the incoming message
// identified by info (direct image, view-once, or quoted image). Returns the
// raw bytes and true if an image was present and downloaded; nil,false otherwise.
func (b *bridge) DownloadImage(info types.MessageInfo) ([]byte, bool) {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return nil, false
	}
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return nil, false
	}
	img := extractImageMessage(msg)
	if img == nil {
		return nil, false
	}
	data, err := b.s.Client.Download(context.Background(), img)
	if err != nil {
		ErrLog("[%s] aivideo DownloadImage failed: %v", b.s.JID, err)
		return nil, false
	}
	return data, true
}

// SetVideoSession stores search results for later number selection.
func (b *bridge) SetAudioSession(jid string, results []goldcmds.VideoResult) {
	goldcmds.ClearYTSList(jid) // a new play search replaces any .yts pick
	var internalResults []VideoResult
	for _, r := range results {
		internalResults = append(internalResults, VideoResult{Title: r.Title, URL: r.URL, Thumbnail: r.Thumbnail, Duration: r.Duration})
	}
	setAudioSession(jid, internalResults)
}

// ClearSearchSession drops a pending search-list pick window (a play/video
// search just replaced the pick context).
func (b *bridge) ClearSearchSession(jid string) {
	goldcmds.ClearSearchSession(jid)
}

func (b *bridge) SetVideoSession(jid string, results []goldcmds.VideoResult) {
	goldcmds.ClearSearchSession(jid) // a new video search replaces any search pick
	goldcmds.ClearYTSList(jid)       // ...and any .yts pick
	var internalResults []VideoResult
	for _, r := range results {
		internalResults = append(internalResults, VideoResult{
			Title:     r.Title,
			URL:       r.URL,
			Thumbnail: r.Thumbnail,
			Duration:  r.Duration,
		})
	}
	setVideoSession(jid, internalResults)
}

// SetVideoSession2 stores a .video2 (turbo engine) search session so number
// picks route back through the video2 parallel engine, with HD when set.
func (b *bridge) SetAudioSession2(jid string, results []goldcmds.VideoResult, play2 bool) {
	goldcmds.ClearSearchSession(jid) // a new play search replaces any search pick
	goldcmds.ClearYTSList(jid)       // ...and any .yts pick
	internal := make([]VideoResult, 0, len(results))
	for _, r := range results {
		internal = append(internal, VideoResult{URL: r.URL, Thumbnail: r.Thumbnail, Title: r.Title, Duration: r.Duration})
	}
	setAudioSession2(jid, internal, play2)
}

func (b *bridge) SetVideoSession2(jid string, results []goldcmds.VideoResult, hd bool) {
	goldcmds.ClearYTSList(jid) // a new video search replaces any .yts pick
	var internalResults []VideoResult
	for _, r := range results {
		internalResults = append(internalResults, VideoResult{
			Title:     r.Title,
			URL:       r.URL,
			Thumbnail: r.Thumbnail,
			Duration:  r.Duration,
		})
	}
	setVideoSession2(jid, internalResults, hd)
}

// SetVideoSession3 stores a .video3 (raw video) search session so number
// picks route back through the video3 engine (raw video, no thumbnail/caption).
func (b *bridge) SetVideoSession3(jid string, results []goldcmds.VideoResult, hd bool) {
	goldcmds.ClearYTSList(jid) // a new video search replaces any .yts pick
	var internalResults []VideoResult
	for _, r := range results {
		internalResults = append(internalResults, VideoResult{
			Title:     r.Title,
			URL:       r.URL,
			Thumbnail: r.Thumbnail,
			Duration:  r.Duration,
		})
	}
	setVideoSession3(jid, internalResults, hd)
}

// SetAudioSession3 stores a .play3 (raw audio) search session so number picks
// route back through the play3 engine (raw audio, no thumbnail/caption).
func (b *bridge) SetAudioSession3(jid string, results []goldcmds.VideoResult) {
	goldcmds.ClearSearchSession(jid) // a new play search replaces any search pick
	goldcmds.ClearYTSList(jid)       // ...and any .yts pick
	internal := make([]VideoResult, 0, len(results))
	for _, r := range results {
		internal = append(internal, VideoResult{URL: r.URL, Thumbnail: r.Thumbnail, Title: r.Title, Duration: r.Duration})
	}
	setAudioSession3(jid, internal)
}

// extractMediaMessage returns the first downloadable whatsmeow
// DownloadableMessage found inside a message proto, handling direct media,
// view-once wrappers (V1/V2/V2-extension) and quoted messages. Returns the
// downloadable message, its mimetype (best-effort) and true; nil,"",false
// when no media is present.
func extractMediaMessage(msg *waProto.Message) (whatsmeow.DownloadableMessage, string, bool) {
	if msg == nil {
		return nil, "", false
	}
	if msg.ImageMessage != nil {
		return msg.ImageMessage, mt(msg.ImageMessage.Mimetype, "image/jpeg"), true
	}
	if msg.VideoMessage != nil {
		return msg.VideoMessage, mt(msg.VideoMessage.Mimetype, "video/mp4"), true
	}
	if msg.PtvMessage != nil {
		// Circle / video-note: PtvMessage IS a VideoMessage.
		return msg.PtvMessage, mt(msg.PtvMessage.Mimetype, "video/mp4"), true
	}
	if msg.AudioMessage != nil {
		return msg.AudioMessage, mt(msg.AudioMessage.Mimetype, "audio/mpeg"), true
	}
	if msg.StickerMessage != nil {
		return msg.StickerMessage, mt(msg.StickerMessage.Mimetype, "image/webp"), true
	}
	if msg.DocumentMessage != nil {
		return msg.DocumentMessage, mt(msg.DocumentMessage.Mimetype, "application/octet-stream"), true
	}
	for _, wrapper := range []*waProto.FutureProofMessage{msg.ViewOnceMessage, msg.ViewOnceMessageV2} {
		if wrapper != nil && wrapper.Message != nil {
			if d, m, ok := extractMediaMessage(wrapper.Message); ok {
				return d, m, ok
			}
		}
	}
	if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		if d, m, ok := extractMediaMessage(msg.ViewOnceMessageV2Extension.Message); ok {
			return d, m, ok
		}
	}
	if ci := extractContextInfoFromMsg(msg); ci != nil && ci.QuotedMessage != nil {
		if d, m, ok := extractMediaMessage(ci.QuotedMessage); ok {
			return d, m, ok
		}
	}
	return nil, "", false
}

// mt returns *s when non-empty, otherwise fallback.
func mt(s *string, fallback string) string {
	if s != nil && *s != "" {
		return *s
	}
	return fallback
}

// DownloadQuotedMedia downloads any media attached to the incoming message
// identified by info (direct, view-once, or quoted).
func (b *bridge) DownloadQuotedMedia(info types.MessageInfo) ([]byte, string, bool) {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return nil, "", false
	}
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return nil, "", false
	}
	dl, mime, ok := extractMediaMessage(msg)
	if !ok || dl == nil {
		return nil, "", false
	}
	data, err := b.s.Client.Download(context.Background(), dl)
	if err != nil {
		ErrLog("[%s] DownloadQuotedMedia failed: %v", b.s.JID, err)
		return nil, "", false
	}
	return data, mime, true
}

// isSudoOwnerJID checks whether a JID string ("923xxx@s.whatsapp.net" or
// "923xxx:5@s.whatsapp.net") belongs to a sudo owner or the permanent
// paired owner, by looking up the Redis "sudowners" field.
func (b *bridge) isSudoOwnerJID(jidStr string) bool {
	if jidStr == "" || b.s.Manager.Redis == nil {
		return false
	}
	num := botOwnNumber(jidStr)
	if num == "" {
		return false
	}
	return b.IsSudoOwner(num)
}

// IsOwner reports whether the sender of info is a configured owner or the bot.
func (b *bridge) IsOwner(info types.MessageInfo) bool {
	sender := info.Sender.String()
	if b.s.Manager.cfg.IsOwner(sender) || sender == b.s.JID {
		return true
	}
	// Redis sudo owners (added via .ownernumber add / .sudo add)
	if b.isSudoOwnerJID(sender) {
		return true
	}
	// Also check SenderAlt (LID <-> phone JID mapping) for owner matching
	if info.SenderAlt.Server != "" {
		altSender := info.SenderAlt.String()
		if b.s.Manager.cfg.IsOwner(altSender) || altSender == b.s.JID {
			return true
		}
		if b.isSudoOwnerJID(altSender) {
			return true
		}
	}
	// IsFromMe means the message was sent from the bot owner's own device
	if info.IsFromMe {
		return true
	}
	return false
}

// SendDocument uploads and sends a generic file as a WhatsApp document.
func (b *bridge) SendDocument(info types.MessageInfo, data []byte, fileName string, mimeType string, caption string) error {
	// GUARD (Render bandwidth shield): 50MB+ file → block (zip/apk/pdf re-encode impossible)
	if g := b.guardBytes(info, guardDocument, data, caption); g.blocked() {
		return nil
	}
	caption = b.s.withCaptionFooter(caption) // botname footer on every document
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return err
	}
	docMsg := &waProto.DocumentMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		Mimetype:      proto.String(mimeType),
		Title:         proto.String(fileName),
		FileName:      proto.String(fileName),
		Caption:       proto.String(caption),
		MediaKey:      resp.MediaKey,
		FileLength:    proto.Uint64(uint64(len(data))),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{DocumentMessage: docMsg})
	return err
}

// SendDocumentFile uploads and sends a generic file as a WhatsApp document
// from a path on disk (streamed, never fully in RAM).
func (b *bridge) SendDocumentFile(info types.MessageInfo, path string, fileName string, mimeType string, caption string) error {
	// GUARD (Render bandwidth shield): 50MB+ file → block (re-encode impossible)
	if g := b.guardPath(info, guardDocument, path, caption); g.blocked() {
		return nil
	}
	caption = b.s.withCaptionFooter(caption) // botname footer on every document
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := b.s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaDocument)
	if err != nil {
		return err
	}
	docMsg := &waProto.DocumentMessage{
		URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath), Mimetype: proto.String(mimeType),
		Title: proto.String(fileName), FileName: proto.String(fileName), Caption: proto.String(caption),
		MediaKey: resp.MediaKey, FileLength: proto.Uint64(resp.FileLength),
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256,
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{DocumentMessage: docMsg})
	return err
}

// SendSticker uploads and sends a .webp sticker from in-memory bytes.
func (b *bridge) SendSticker(info types.MessageInfo, data []byte) error {
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	stickerMsg := &waProto.StickerMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		Mimetype:      proto.String("image/webp"),
		MediaKey:      resp.MediaKey,
		FileLength:    proto.Uint64(uint64(len(data))),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{StickerMessage: stickerMsg})
	return err
}

// GetQuotedMessageID extracts the quoted (replied-to) message ID and its
// sender JID from the cached incoming message proto's ContextInfo. Used by
// the edit command to know which message to edit. Returns "","",false when
// the message is not a reply.
func (b *bridge) GetQuotedMessageID(info types.MessageInfo) (string, string, bool) {
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return "", "", false
	}
	if ci := extractContextInfoFromMsg(msg); ci != nil && ci.StanzaID != nil && *ci.StanzaID != "" {
		sender := ""
		if ci.Participant != nil {
			sender = *ci.Participant
		}
		return *ci.StanzaID, sender, true
	}
	return "", "", false
}

// RevokeQuotedMessage revokes (deletes for everyone) a message by its ID and
// sender. Used by the .del command to delete the quoted/replied-to message.
func (b *bridge) RevokeQuotedMessage(chat types.JID, senderJID string, messageID string) error {
	return b.s.RevokeAnyMessage(chat, senderJID, messageID)
}

// GetPresenceSetting reads a per-user presence setting from Redis (Upstash).
// Uses the settings:<botJID> hash with the given field.
func (b *bridge) GetPresenceSetting(field, def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, field, def)
}

// SetPresenceSetting writes a per-user presence setting to Redis (Upstash).
func (b *bridge) SetPresenceSetting(field, val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, field, val)
}

// SendPresenceUpdate sets the bot's global online/offline presence.
// state = "available" (always online) or "unavailable" (offline/last seen).
func (b *bridge) SendPresenceUpdate(state string) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	var p types.Presence
	switch state {
	case "available":
		p = types.PresenceAvailable
	case "unavailable":
		p = types.PresenceUnavailable
	default:
		return fmt.Errorf("unknown presence state: %s", state)
	}
	return b.s.Client.SendPresence(context.Background(), p)
}

// SendChatPresenceUpdate sends a typing/recording/paused chat presence to a
// specific chat. state = "composing" or "paused". media = "audio" (recording)
// or "" (typing text).
func (b *bridge) SendChatPresenceUpdate(chat types.JID, state string, media string) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	var cp types.ChatPresence
	switch state {
	case "composing":
		cp = types.ChatPresenceComposing
	case "paused":
		cp = types.ChatPresencePaused
	default:
		return fmt.Errorf("unknown chat presence state: %s", state)
	}
	var cpm types.ChatPresenceMedia
	switch media {
	case "audio":
		cpm = types.ChatPresenceMediaAudio
	default:
		cpm = types.ChatPresenceMediaText
	}
	return b.s.Client.SendChatPresence(context.Background(), chat, cp, cpm)
}

// GetStatusSetting reads a per-user status setting from Redis (Upstash).
// Uses the settings:<botJID> hash with the given field.
func (b *bridge) GetStatusSetting(field, def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, field, def)
}

// SetStatusSetting writes a per-user status setting to Redis (Upstash).
func (b *bridge) SetStatusSetting(field, val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, field, val)
}

// DelStatusSetting removes a status setting from Redis (redis-safe
// HDEL — used by cmdownerpublic reset to fully clear the cmdaccess
// owner-only list without leaving an empty-string field).
func (b *bridge) DelStatusSetting(field string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.DelSetting(b.s.JID, field)
}

// GetModeSetting reads the bot work-mode from Redis settings:<botJID>.
func (b *bridge) GetModeSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "mode", def)
}

// SetModeSetting writes the bot work-mode to Redis.
func (b *bridge) SetModeSetting(mode string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "mode", mode)
}

// GetBotPicSetting reads the custom bot menu/alive image URL from Redis.
func (b *bridge) GetBotPicSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "botpic", def)
}

// SetBotPicSetting writes the custom bot menu/alive image URL to Redis.
func (b *bridge) SetBotPicSetting(url string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "botpic", url)
}

// GetBotVideoSetting reads the custom bot menu/alive VIDEO URL from Redis.
func (b *bridge) GetBotVideoSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "botvideo", def)
}

// SetBotVideoSetting writes the custom bot menu/alive VIDEO URL to Redis.
func (b *bridge) SetBotVideoSetting(url string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "botvideo", url)
}

// GetBotVoiceSetting reads the custom bot menu/alive VOICE (audio) URL from
// Redis (field "botvoice").
func (b *bridge) GetBotVoiceSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "botvoice", def)
}

// SetBotVoiceSetting writes the custom bot menu/alive VOICE URL to Redis. An
// empty value clears it (back to the built-in default);
// goldcmds.MenuMediaVoiceOff stores the "off" sentinel and silences it.
func (b *bridge) SetBotVoiceSetting(url string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "botvoice", url)
}

// VoiceURLPlayable reports whether a voice URL actually serves audio rather
// than an HTML share/error page, which is what makes WhatsApp play it.
func (b *bridge) VoiceURLPlayable(url string) bool {
	return goldcmds.BotVoiceURLIsPlayable(url)
}

// GetMenuMediaSetting reads the per-menu custom media URL (Redis field
// "menumedia:<key>"). key is a menu slug such as "menu", "logo", "ai",
// "converter" or "alive". Empty when nothing is set for that menu.
func (b *bridge) GetMenuMediaSetting(key, def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "menumedia:"+strings.ToLower(strings.TrimSpace(key)), def)
}

// SetMenuMediaSetting writes the per-menu custom media URL. An empty url
// clears the override for that menu.
func (b *bridge) SetMenuMediaSetting(key, url string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	field := "menumedia:" + strings.ToLower(strings.TrimSpace(key))
	if strings.TrimSpace(url) == "" {
		b.s.Manager.Redis.DelSetting(b.s.JID, field)
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, field, url)
}

// ── OWNER NAME / OWNER NUMBER / BOT NAME / ALIVE MSG bridge impls ──

// GetOwnerNameSetting reads the owner display NAME from Redis settings:<botJID>.
func (b *bridge) GetOwnerNameSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "ownername", def)
}

// SetOwnerNameSetting writes the owner display NAME to Redis.
func (b *bridge) SetOwnerNameSetting(name string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "ownername", name)
}

// GetOwnerNumberSetting reads the owner WhatsApp NUMBER (digits) from Redis.
func (b *bridge) GetOwnerNumberSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "ownernumber", def)
}

// SetOwnerNumberSetting writes the owner WhatsApp NUMBER to Redis.
func (b *bridge) SetOwnerNumberSetting(number string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "ownernumber", number)
}

// GetSudoOwners returns the list of SUDO (additional) owner numbers from
// Redis field "sudowners" (comma-separated digits).  Returns empty slice
// if not set or Redis unavailable.
func (b *bridge) GetSudoOwners() []string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return nil
	}
	raw := b.s.Manager.Redis.GetSetting(b.s.JID, "sudowners", "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SetSudoOwners writes the full list of sudo owner numbers to Redis
// (field "sudowners", comma-separated).
func (b *bridge) SetSudoOwners(numbers []string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "sudowners", strings.Join(numbers, ","))
}

// IsSudoOwner reports whether the given number (digits only) is either the
// permanent paired owner number or in the sudo owners list.
func (b *bridge) IsSudoOwner(number string) bool {
	if number == "" {
		return false
	}
	// permanent owner = the paired number itself
	if number == b.GetPermanentOwnerNumber() {
		return true
	}
	for _, n := range b.GetSudoOwners() {
		if n == number {
			return true
		}
	}
	return false
}

// GetPermanentOwnerNumber returns the bot's own paired number (digits only),
// which is the permanent owner that can never be deleted.
func (b *bridge) GetPermanentOwnerNumber() string {
	return botOwnNumber(b.s.JID)
}

// GetBotNameSetting reads the bot display NAME from Redis.
func (b *bridge) GetBotNameSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "botname", def)
}

// SetBotNameSetting writes the bot display NAME to Redis.
func (b *bridge) SetBotNameSetting(name string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "botname", name)
}

// DelBotNameSetting removes the custom bot name from Redis so the bot
// falls back to the branded default footer (used by .botname reset).
func (b *bridge) DelBotNameSetting() {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.DelSetting(b.s.JID, "botname")
}

// GetAliveMsgSetting reads the custom .alive message text from Redis.
func (b *bridge) GetAliveMsgSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "alivemsg", def)
}

// SetAliveMsgSetting writes the custom .alive message text to Redis.
func (b *bridge) SetAliveMsgSetting(msg string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	if msg == "" {
		b.s.Manager.Redis.DelSetting(b.s.JID, "alivemsg")
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "alivemsg", msg)
}

// ── ANTIDELETE / ANTIEDIT bridge impls ──
// Stored as two fields in the settings:<botJID> hash:
//   antidelete_enabled = "true"/"false"
//   antidelete_scope   = "all"/"inbox"/"groups"
// (antiedit uses the antiedit_ prefix.)

// GetAntiDeleteSetting reads the antidelete config from Redis.
func (b *bridge) GetAntiDeleteSetting() goldcmds.AntiDeleteConfig {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return goldcmds.AntiDeleteConfig{Enabled: false, Scope: "all"}
	}
	en := b.s.Manager.Redis.GetSetting(b.s.JID, "antidelete_enabled", "false")
	scope := b.s.Manager.Redis.GetSetting(b.s.JID, "antidelete_scope", "all")
	return goldcmds.AntiDeleteConfig{Enabled: en == "true" || en == "1", Scope: scope}
}

// SetAntiDeleteSetting writes the antidelete config to Redis.
func (b *bridge) SetAntiDeleteSetting(enabled bool, scope string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	v := "false"
	if enabled {
		v = "true"
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "antidelete_enabled", v)
	b.s.Manager.Redis.SetSetting(b.s.JID, "antidelete_scope", scope)
}

// GetAntiDeleteMode reads the antidelete delivery mode from Redis.
// Returns "here" (default) or "inbox".
func (b *bridge) GetAntiDeleteMode() string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return "here"
	}
	m := b.s.Manager.Redis.GetSetting(b.s.JID, "antidelete_mode", "here")
	if m != "here" && m != "inbox" {
		m = "here"
	}
	return m
}

// SetAntiDeleteMode writes the antidelete delivery mode to Redis.
// mode must be "here" or "inbox"; anything else is normalized to "here".
func (b *bridge) SetAntiDeleteMode(mode string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	if mode != "inbox" {
		mode = "here"
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "antidelete_mode", mode)
}

// GetAntiEditSetting reads the antiedit config from Redis.
func (b *bridge) GetAntiEditSetting() goldcmds.AntiDeleteConfig {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		// goldcmds.JSONDebug("ANTIEDIT_REDIS", map[string]any{
		// "botJID": b.s.JID, "stage": "get_redis_nil", "enabled": false, "scope": "all",
		// })
		return goldcmds.AntiDeleteConfig{Enabled: false, Scope: "all"}
	}
	en := b.s.Manager.Redis.GetSetting(b.s.JID, "antiedit_enabled", "false")
	scope := b.s.Manager.Redis.GetSetting(b.s.JID, "antiedit_scope", "all")
	cfg := goldcmds.AntiDeleteConfig{Enabled: en == "true" || en == "1", Scope: scope}
	// goldcmds.JSONDebug("ANTIEDIT_REDIS", map[string]any{
	// "botJID":        b.s.JID,
	// "stage":         "get_ok",
	// "enabled":       cfg.Enabled,
	// "scope":         cfg.Scope,
	// "rawEnabled":    en,
	// "rawScope":      scope,
	// })
	return cfg
}

// SetAntiEditSetting writes the antiedit config to Redis.
func (b *bridge) SetAntiEditSetting(enabled bool, scope string) {
	// goldcmds.JSONDebug("ANTIEDIT_REDIS", map[string]any{
	// "botJID":  b.s.JID,
	// "stage":   "set_request",
	// "enabled": enabled,
	// "scope":   scope,
	// })
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		// goldcmds.JSONDebug("ANTIEDIT_REDIS", map[string]any{
		// "botJID": b.s.JID, "stage": "set_redis_nil", "enabled": enabled, "scope": scope,
		// })
		return
	}
	v := "false"
	if enabled {
		v = "true"
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "antiedit_enabled", v)
	b.s.Manager.Redis.SetSetting(b.s.JID, "antiedit_scope", scope)
	// goldcmds.JSONDebug("ANTIEDIT_REDIS", map[string]any{
	// "botJID":         b.s.JID,
	// "stage":          "set_ok",
	// "enabled":        enabled,
	// "scope":          scope,
	// "wroteEnabled":   v,
	// "wroteScope":     scope,
	// })
}

// GetAntiEditMode reads the antiedit delivery mode from Redis.
// Returns "here" (default) or "inbox".
func (b *bridge) GetAntiEditMode() string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return "here"
	}
	m := b.s.Manager.Redis.GetSetting(b.s.JID, "antiedit_mode", "here")
	if m != "here" && m != "inbox" {
		m = "here"
	}
	return m
}

// SetAntiEditMode writes the antiedit delivery mode to Redis.
// mode must be "here" or "inbox"; anything else is normalized to "here".
func (b *bridge) SetAntiEditMode(mode string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	if mode != "inbox" {
		mode = "here"
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "antiedit_mode", mode)
}

// GetAntiCallSetting reads whether anticall is ON for this bot from Redis.
// Default: false (same as Node.js ANTI_CALL: false).
func (b *bridge) GetAntiCallSetting() bool {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return false
	}
	en := b.s.Manager.Redis.GetSetting(b.s.JID, "anticall_enabled", "false")
	return en == "true" || en == "1"
}

// SetAntiCallSetting writes the anticall on/off flag to Redis.
func (b *bridge) SetAntiCallSetting(enabled bool) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	v := "false"
	if enabled {
		v = "true"
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "anticall_enabled", v)
}

// GetAntiCallMessage reads the custom reject-call message from Redis.
// Returns def if not set. def should be ANTICALL_DEFAULT_MSG.
func (b *bridge) GetAntiCallMessage(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "anticall_msg", def)
}

// SetAntiCallMessage writes the custom reject-call message to Redis.
// Empty string = reset to default (Redis-safe delete via DelSetting).
func (b *bridge) SetAntiCallMessage(msg string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	if msg == "" {
		b.s.Manager.Redis.DelSetting(b.s.JID, "anticall_msg")
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "anticall_msg", msg)
}

// ShowLogoMenu renders the .logo command's fancy boxed menu (same format as
// the other category menus) instead of plain text. Delegates to the main
// package's CmdLogoMenu (manager.go).
func (b *bridge) ShowLogoMenu(info types.MessageInfo, args []string, prefix string) {
	b.s.CmdLogoMenu(info, args, prefix)
}

// ShowFontMenu renders the .font command's fancy boxed menu (FONT1..FONT1000)
// in the same format as the other category menus. Delegates to the main
// package's CmdFontMenu (manager.go).
func (b *bridge) ShowFontMenu(info types.MessageInfo, args []string, prefix string) {
	b.s.CmdFontMenu(info, args, prefix)
}

// ShowEqualizerMenu renders the .equalizer command's fancy boxed menu
// (EQ1..EQ1000) in the same format as the other category menus. Delegates to
// the main package's CmdEqualizerMenu (manager.go).
func (b *bridge) ShowEqualizerMenu(info types.MessageInfo, args []string, prefix string) {
	b.s.CmdEqualizerMenu(info, args, prefix)
}

// ShowGameMenu renders the .game command's fancy boxed menu (GAME1..GAME1000)
// in the same format as the other category menus. Delegates to the main
// package's CmdGameMenu (manager.go).
func (b *bridge) ShowGameMenu(info types.MessageInfo, args []string, prefix string) {
	b.s.CmdGameMenu(info, args, prefix)
}

// GetPrefix reads the bot's command prefix from Redis (key prefix:<botJID>).
func (b *bridge) GetPrefix(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetPrefix(b.s.JID, def)
}

// SetPrefix writes the bot's command prefix to Redis. Empty string = no prefix.
func (b *bridge) SetPrefix(prefix string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetPrefix(b.s.JID, prefix)
}

// NotifyPrefixChanged re-sends the connected/startup card (fresh prefix ke
// sath) right after .prefix changes it. Reply pehle jaati hai (confirm card),
// phir 1s baad connected card — owner ko naya prefix turant dikhta hai.
// Prefix Redis cache SetPrefix me hi update ho chuka hota hai, isliye card me
// hamesha FRESH prefix aata hai (purana cached kabhi nahi).
func (b *bridge) NotifyPrefixChanged() {
	if b.s == nil || b.s.Client == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		time.Sleep(1 * time.Second)
		b.s.sendStartupNotification()
	}()
}

// MarkStatusRead marks a status (story) message as seen/read.
// chat = status@broadcast JID, sender = the status owner.
func (b *bridge) MarkStatusRead(chat, sender types.JID, msgID string) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	return b.s.Client.MarkRead(context.Background(), []types.MessageID{msgID}, time.Now(), chat, sender)
}

// SendStatusReaction sends an emoji reaction to a status message.
//
// CRITICAL: status reactions MUST be sent to types.StatusBroadcastJID
// ("status@broadcast"), NOT to the status owner's PN JID. When SendMessage
// sees to.Server == BroadcastServer it uses the broadcast/group encryption
// path (getBroadcastListParticipants + sendGroup). Sending to a user JID
// goes through sendDM (LID fetch) which WhatsApp rejects with error 479.
//
// Pattern follows ThruqeLabs/whatsrook cmd/events.go (working status reaction):
// build ReactionMessage manually with Key.RemoteJID="status@broadcast",
// Key.Participant=statusOwner (resolved PN), then SendMessage to broadcast.
func (b *bridge) SendStatusReaction(statusOwner types.JID, msgID string, emoji string) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	bcast := types.StatusBroadcastJID
	ownerStr := statusOwner.ToNonAD().String()

	// Build the reaction manually (whatsrook pattern) so we fully control the
	// MessageKey: RemoteJID = status@broadcast, Participant = status owner.
	// Sent to status@broadcast so whatsmeow uses the broadcast encryption path.
	msg := &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key: &waProto.MessageKey{
				RemoteJID:   proto.String(bcast.String()),
				FromMe:      proto.Bool(false),
				ID:          proto.String(msgID),
				Participant: proto.String(ownerStr),
			},
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
	_, err := b.s.Client.SendMessage(context.Background(), bcast, msg)
	if err != nil {
		// ErrLog("[STATUS] reaction failed owner=%s err=%v", ownerStr, err) // debug off
		return err
	}
	// InfoLog("[STATUS] reaction sent owner=%s emoji=%s", ownerStr, emoji) // debug off
	return nil
}

// SendStatusReply sends a private text reply to the person who posted a
// status, quoting the status message — exactly like the WhatsApp app's
// "reply to status" (a private DM to the status owner, NOT a status comment
// and NOT a new status post).
//
// IMPORTANT: this is sent as a normal DM to statusOwner (the resolved PN JID),
// NOT to status@broadcast. Sending a text ExtendedTextMessage to
// status@broadcast would publish it as the bot's OWN status, which is wrong.
// Only reactions go to status@broadcast (broadcast encryption path); a text
// reply is a private message to the owner with ContextInfo quoting the status.
//
// statusOwner = the person who posted the status (resolved PN JID),
// msgID = the status message ID (used to quote), text = the reply text.
func (b *bridge) SendStatusReply(statusOwner types.JID, msgID string, text string, quotedMsg *waProto.Message, guardOn bool) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	ownerStr := statusOwner.ToNonAD().String()

	// ── GUARD ───────────────────────────────────────────────────────────────────
	// guardOn is synced with statusreply on/off (handler passes replyOn).
	//
	// The REAL unwwrapped status proto (ImageMessage / VideoMessage with the
	// JPEGThumbnail) is ALWAYS placed in ContextInfo.QuotedMessage so the
	// receiver sees the status thumbnail quote preview ("quoted mention").
	//
	// What the guard actually does (Baileys-compatible, the FIX for double bubble):
	//   guardOn=true  -> real unwrapped proto in QuotedMessage (thumbnail intact)
	//                    AND set ContextInfo.RemoteJID = "status@broadcast".
	//                    This is exactly what Baileys does in
	//                    generateWAMessageFromContent: when the reply target (DM
	//                    to status owner) differs from the quoted message origin
	//                    (status@broadcast), it sets contextInfo.remoteJid to the
	//                    quoted origin. RemoteJID="status@broadcast" tells the
	//                    WhatsApp receiver client that the quoted message is a
	//                    STATUS, so it renders a SINGLE status-reply bubble with
	//                    the thumbnail quote preview instead of a plain media
	//                    fallback bubble + a quoted text bubble (the double).
	//                    quotePreview (thumbnail-safe field stripping) did NOT fix
	//                    the double; RemoteJID is the actual fix.
	//   guardOn=false -> send quotedMsg as-is WITHOUT RemoteJID (raw proto,
	//                    original behavior, may double on receiver — debug mode).
	var quoteProto *waProto.Message
	//var guardApplied bool
	quoteProto = quotedMsg // real unwrapped proto (thumbnail intact) in both paths
	//guardApplied = quoteProto != nil
	//guardApplied = quoteProto != nil
	ctx := &waProto.ContextInfo{
		StanzaID:    proto.String(msgID),
		Participant: proto.String(ownerStr),
	}
	if quoteProto != nil {
		ctx.QuotedMessage = quoteProto
	}
	// GUARD FIX: set RemoteJID so WhatsApp knows the quoted message is a STATUS
	// (origin status@broadcast), not a regular DM media. This prevents the
	// receiver from rendering a separate plain media fallback bubble (the double).
	// Baileys sets this when reply target != quoted origin. Only when guard on.
	if guardOn {
		ctx.RemoteJID = proto.String(types.StatusBroadcastJID.String())
	}
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ctx,
		},
	}

	// // ── JSON DEBUG (guard) ──────────────────────────────────────────────────
	//	JSONDebug("STATUS_REPLY", map[string]any{
	//		"owner":         ownerStr,
	//		"statusMsgID":   msgID,
	//		"replyText":     text,
	//		"guardOn":       guardOn,
	//		"guardApplied":  guardApplied,
	//		"origQuoteType": messageTypeLabel(quotedMsg),
	//		"quoteType":     messageTypeLabel(quoteProto),
	//		"hasQuotedMsg":  quoteProto != nil,
	//		"stanzaID":      msgID,
	//		"participant":   ownerStr,
	//		"remoteJID":     func() string { if ctx.RemoteJID != nil { return *ctx.RemoteJID }; return "" }(),
	//	})

	_, err := b.s.Client.SendMessage(context.Background(), statusOwner, msg)
	if err != nil {
		//		JSONDebug("STATUS_REPLY_ERR", map[string]any{
		//			"owner": ownerStr,
		//			"error": err.Error(),
		//			"guard": guardOn,
		//		})
		// ErrLog("[STATUS] reply failed owner=%s err=%v", ownerStr, err)
		return err
	}
	//	JSONDebug("STATUS_REPLY_OK", map[string]any{
	//		"owner":     ownerStr,
	//		"sentMsgID": resp.ID,
	//		"guard":     guardOn,
	//	})
	// InfoLog("[STATUS] reply sent owner=%s", ownerStr)
	return nil
}

// =============================================================================
// GCSTATUS BRIDGE METHODS
// Post the bot's own status (story) to status@broadcast and then mention that
// status to every group the bot is in -- replicating the manual WhatsApp flow
// of posting a status, tapping @ and selecting groups.
//
// Reference: gifted-baileys gcstatus.js sendStatusToGroups + handleGroupStory.
//   Step 1 -> SendMessage to status@broadcast with additionalNodes meta>
//             mentioned_users>to jid per group.
//   Step 2 -> for each group, send a GroupStatusMentionMessage whose inner
//             protocolMessage carries {key: statusKey, type: 25
//             (STATUS_MENTION_MESSAGE)} and additionalNodes
//             meta{is_group_status_mention:"true"}.
// =============================================================================

// PostStatusToBroadcast posts msg as the bot's own WhatsApp status (story).
// mentionedGroupJIDs are embedded as meta>mentioned_users nodes so WhatsApp
// links the status to those groups (the @-mention flow). Returns status msg ID.
func (b *bridge) PostStatusToBroadcast(msg *waProto.Message, mentionedGroupJIDs []types.JID) (string, error) {
	if b.s.Client == nil {
		return "", fmt.Errorf("client not ready")
	}
	if msg == nil {
		return "", fmt.Errorf("status message proto is nil")
	}

	// Build additionalNodes: meta > mentioned_users > [to jid] for each group.
	// This mirrors gifted-baileys additionalNodes in sendStatusToGroups step 1.
	var toNodes []waBinary.Node
	for _, gid := range mentionedGroupJIDs {
		toNodes = append(toNodes, waBinary.Node{
			Tag:   "to",
			Attrs: waBinary.Attrs{"jid": gid.String()},
		})
	}
	var extraNodes []waBinary.Node
	if len(toNodes) > 0 {
		extraNodes = []waBinary.Node{{
			Tag:   "meta",
			Attrs: waBinary.Attrs{},
			Content: []waBinary.Node{{
				Tag:     "mentioned_users",
				Attrs:   waBinary.Attrs{},
				Content: toNodes,
			}},
		}}
	}

	extra := whatsmeow.SendRequestExtra{}
	if len(extraNodes) > 0 {
		extra.AdditionalNodes = &extraNodes
	}

	resp, err := b.s.Client.SendMessage(context.Background(), types.StatusBroadcastJID, msg, extra)
	if err != nil {
		//		JSONDebug("GCSTATUS_POST_ERR", map[string]any{
		//			"error":          err.Error(),
		//			"mentionedCount": len(mentionedGroupJIDs),
		//		})
		return "", err
	}
	//	JSONDebug("GCSTATUS_POST_OK", map[string]any{
	//		"statusMsgID":    resp.ID,
	//		"timestamp":      resp.Timestamp.Unix(),
	//		"mentionedCount": len(mentionedGroupJIDs),
	//		"target":         types.StatusBroadcastJID.String(),
	//	})
	return resp.ID, nil
}

// MentionStatusToGroup sends a GroupStatusMentionMessage to a single group,
// referencing the just-posted status by statusMsgID. Produces the in-group
// "X mentioned your group in their status" notification with status preview.
func (b *bridge) MentionStatusToGroup(groupJID types.JID, statusMsgID string) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	if statusMsgID == "" {
		return fmt.Errorf("status message ID is empty")
	}

	// GroupStatusMentionMessage (field 92) is a FutureProofMessage wrapping an
	// inner Message. The inner Message is a ProtocolMessage with:
	//   Key  -> waCommon.MessageKey{RemoteJID:"status@broadcast", FromMe:true,
	//           ID: statusMsgID, Participant: bot JID}
	//   Type -> ProtocolMessage_STATUS_MENTION_MESSAGE (25)
	// additionalNodes: meta{is_group_status_mention:"true"} (groups).
	botJID := b.s.JID
	mentionMsg := &waProto.Message{
		GroupStatusMentionMessage: &waProto.FutureProofMessage{
			Message: &waProto.Message{
				ProtocolMessage: &waProto.ProtocolMessage{
					Key: &waCommon.MessageKey{
						RemoteJID:   proto.String(types.StatusBroadcastJID.String()),
						FromMe:      proto.Bool(true),
						ID:          proto.String(statusMsgID),
						Participant: proto.String(botJID),
					},
					Type: waE2E.ProtocolMessage_STATUS_MENTION_MESSAGE.Enum(),
				},
			},
		},
		MessageContextInfo: &waProto.MessageContextInfo{
			MessageSecret: random.Bytes(32),
		},
	}

	extraNodes := []waBinary.Node{{
		Tag:   "meta",
		Attrs: waBinary.Attrs{"is_group_status_mention": "true"},
	}}
	extra := whatsmeow.SendRequestExtra{AdditionalNodes: &extraNodes}

	_, err := b.s.Client.SendMessage(context.Background(), groupJID, mentionMsg, extra)
	if err != nil {
		//		JSONDebug("GCSTATUS_MENTION_ERR", map[string]any{
		//			"group":       groupJID.String(),
		//			"statusMsgID": statusMsgID,
		//			"error":       err.Error(),
		//		})
		return err
	}
	//	JSONDebug("GCSTATUS_MENTION_OK", map[string]any{
	//		"group":       groupJID.String(),
	//		"statusMsgID": statusMsgID,
	//		"sentMsgID":   resp.ID,
	//	})
	return nil
}

// SendGroupStory sends the actual status media/content directly to a group,
// wrapped inside a GroupStatusMessageV2 (proto field 103 -> FutureProofMessage).
// This is what creates the GREEN RING group story on the group profile picture
// -- the group status story that members see when they tap the group DP.
// Sent directly to the group JID (NOT status@broadcast).
// mediaMsg = the actual image/video/audio/text proto (same as what was posted
// to status@broadcast).
//
// Structure (mirrors gifted-baileys sendGroupStatus / handleGroupStory):
//
//	Message{
//	  GroupStatusMessageV2: FutureProofMessage{       // field 103
//	    Message: &Message{                             // inner = actual media
//	      ImageMessage: ...  (or VideoMessage/AudioMessage/ExtendedTextMessage)
//	    },
//	  },
//	  MessageContextInfo: &MessageContextInfo{
//	    MessageSecret: random 32 bytes,
//	  },
//	}
//
// Sent directly to groupJID with no special additional nodes.
func (b *bridge) SendGroupStory(groupJID types.JID, mediaMsg *waProto.Message) error {
	if b.s.Client == nil {
		return fmt.Errorf("client not ready")
	}
	if mediaMsg == nil {
		return fmt.Errorf("group story media message is nil")
	}

	// Wrap the actual media content inside GroupStatusMessageV2 (field 103).
	// GroupStatusMessageV2 is a *FutureProofMessage whose .Message field holds
	// the real content (image/video/audio/text). This is exactly what
	// gifted-baileys does in sendGroupStatus / handleGroupStory.
	storyMsg := &waProto.Message{
		GroupStatusMessageV2: &waProto.FutureProofMessage{
			Message: mediaMsg,
		},
		MessageContextInfo: &waProto.MessageContextInfo{
			MessageSecret: random.Bytes(32),
		},
	}

	//	JSONDebug("GCSTATUS_STORY_SEND", map[string]any{
	//		"group":   groupJID.String(),
	//		"wrapper": "GroupStatusMessageV2",
	//		"field":   103,
	//	})

	_, err := b.s.Client.SendMessage(context.Background(), groupJID, storyMsg)
	if err != nil {
		//		JSONDebug("GCSTATUS_STORY_ERR", map[string]any{
		//			"group": groupJID.String(),
		//			"error": err.Error(),
		//		})
		return err
	}
	//	JSONDebug("GCSTATUS_STORY_OK", map[string]any{
	//		"group":     groupJID.String(),
	//		"sentMsgID": resp.ID,
	//	})
	return nil
}

// GetJoinedGroupsList returns the JIDs of all groups the bot is a member of.
func (b *bridge) GetJoinedGroupsList() ([]types.JID, error) {
	if b.s.Client == nil {
		return nil, fmt.Errorf("client not ready")
	}
	groups, err := b.s.Client.GetJoinedGroups(context.Background())
	if err != nil {
		//		JSONDebug("GCSTATUS_GROUPS_ERR", map[string]any{"error": err.Error()})
		return nil, err
	}
	var jids []types.JID
	for _, g := range groups {
		jids = append(jids, g.JID)
	}
	//	JSONDebug("GCSTATUS_GROUPS_OK", map[string]any{"count": len(jids)})
	return jids, nil
}

// messageTypeLabel returns a short string identifying the top-level message
// type of a proto (e.g. "imageMessage", "viewOnceMessageV2", "conversation",
// "ephemeralMessage"). Used for JSON debug so we can see exactly what wrapper
// / content type arrived without dumping the whole proto.
func messageTypeLabel(m *waProto.Message) string {
	if m == nil {
		return "nil"
	}
	switch {
	case m.EphemeralMessage != nil:
		return "ephemeralMessage"
	case m.ViewOnceMessageV2 != nil:
		return "viewOnceMessageV2"
	case m.ViewOnceMessage != nil:
		return "viewOnceMessage"
	case m.DocumentWithCaptionMessage != nil:
		return "documentWithCaptionMessage"
	case m.EditedMessage != nil:
		return "editedMessage"
	case m.GroupMentionedMessage != nil:
		return "groupMentionedMessage"
	case m.ViewOnceMessageV2Extension != nil:
		return "viewOnceMessageV2Extension"
	case m.ImageMessage != nil:
		return "imageMessage"
	case m.VideoMessage != nil:
		return "videoMessage"
	case m.PtvMessage != nil:
		return "ptvMessage"
	case m.AudioMessage != nil:
		return "audioMessage"
	case m.StickerMessage != nil:
		return "stickerMessage"
	case m.DocumentMessage != nil:
		return "documentMessage"
	case m.Conversation != nil:
		return "conversation"
	case m.ExtendedTextMessage != nil:
		return "extendedTextMessage"
	case m.ContactMessage != nil:
		return "contactMessage"
	case m.LocationMessage != nil:
		return "locationMessage"
	case m.LiveLocationMessage != nil:
		return "liveLocationMessage"
	default:
		return "unknown"
	}
}

// quotePreview builds a thumbnail-safe COPY of a status message proto for
// use as ContextInfo.QuotedMessage. It keeps ONLY the fields needed for
// WhatsApp to render the status thumbnail quote preview (the "quoted
// mention" the user wants), and DROPS every download / crypto / path field
// that makes WhatsApp render a SECOND plain media fallback bubble (the
// "double message" the user reported).
//
// Why: whatsmeow puts the FULL media proto (with encrypted MediaKey, URL,
// DirectPath, FileEncSHA256) into QuotedMessage. The official WhatsApp
// receiver then renders it as two bubbles: a plain media message + the
// quoted text reply. Baileys/JS strips these heavy fields internally before
// building the quote; whatsmeow does not, so we replicate that here.
//
// Fields kept per media type:
//
//	ImageMessage  -> Mimetype, Caption, Height, Width, JPEGThumbnail, FileLength
//	VideoMessage  -> Mimetype, Caption, Height, Width, JPEGThumbnail, FileLength,
//	                 Seconds, GifPlayback
//	PtvMessage    -> (same as VideoMessage, via the inner VideoMessage)
//	AudioMessage  -> Mimetype, Seconds (no thumbnail, but single bubble)
//	DocumentMessage -> Title, FileName, Mimetype, JPEGThumbnail, Caption,
//	                   FileLength, PageCount (single bubble)
//	StickerMessage  -> Mimetype, Height, Width, FileLength (single bubble)
//
// Conversation / ExtendedTextMessage / others -> returned as-is (already light).
func quotePreview(msg *waProto.Message) *waProto.Message {
	if msg == nil {
		return nil
	}
	// Image status -> thumbnail-safe copy
	if img := msg.ImageMessage; img != nil {
		safe := &waProto.ImageMessage{}
		if img.Mimetype != nil {
			safe.Mimetype = proto.String(*img.Mimetype)
		}
		if img.Caption != nil {
			safe.Caption = proto.String(*img.Caption)
		}
		if img.Height != nil {
			safe.Height = proto.Uint32(*img.Height)
		}
		if img.Width != nil {
			safe.Width = proto.Uint32(*img.Width)
		}
		if len(img.JPEGThumbnail) > 0 {
			safe.JPEGThumbnail = img.JPEGThumbnail
		}
		if img.FileLength != nil {
			safe.FileLength = proto.Uint64(*img.FileLength)
		}
		return &waProto.Message{ImageMessage: safe}
	}
	// Video status -> thumbnail-safe copy
	if vid := msg.VideoMessage; vid != nil {
		safe := &waProto.VideoMessage{}
		if vid.Mimetype != nil {
			safe.Mimetype = proto.String(*vid.Mimetype)
		}
		if vid.Caption != nil {
			safe.Caption = proto.String(*vid.Caption)
		}
		if vid.Height != nil {
			safe.Height = proto.Uint32(*vid.Height)
		}
		if vid.Width != nil {
			safe.Width = proto.Uint32(*vid.Width)
		}
		if len(vid.JPEGThumbnail) > 0 {
			safe.JPEGThumbnail = vid.JPEGThumbnail
		}
		if vid.FileLength != nil {
			safe.FileLength = proto.Uint64(*vid.FileLength)
		}
		if vid.Seconds != nil {
			safe.Seconds = proto.Uint32(*vid.Seconds)
		}
		if vid.GifPlayback != nil {
			safe.GifPlayback = proto.Bool(*vid.GifPlayback)
		}
		return &waProto.Message{VideoMessage: safe}
	}
	// PTV (push-to-video) status -> msg.PtvMessage is already a *VideoMessage;
	// build a thumbnail-safe copy from it.
	if ptv := msg.PtvMessage; ptv != nil {
		safe := &waProto.VideoMessage{}
		if ptv.Mimetype != nil {
			safe.Mimetype = proto.String(*ptv.Mimetype)
		}
		if ptv.Caption != nil {
			safe.Caption = proto.String(*ptv.Caption)
		}
		if ptv.Height != nil {
			safe.Height = proto.Uint32(*ptv.Height)
		}
		if ptv.Width != nil {
			safe.Width = proto.Uint32(*ptv.Width)
		}
		if len(ptv.JPEGThumbnail) > 0 {
			safe.JPEGThumbnail = ptv.JPEGThumbnail
		}
		if ptv.FileLength != nil {
			safe.FileLength = proto.Uint64(*ptv.FileLength)
		}
		if ptv.Seconds != nil {
			safe.Seconds = proto.Uint32(*ptv.Seconds)
		}
		if ptv.GifPlayback != nil {
			safe.GifPlayback = proto.Bool(*ptv.GifPlayback)
		}
		return &waProto.Message{VideoMessage: safe}
	}
	// Audio status -> keep mimetype + seconds (no thumbnail, but single bubble)
	if aud := msg.AudioMessage; aud != nil {
		safe := &waProto.AudioMessage{}
		if aud.Mimetype != nil {
			safe.Mimetype = proto.String(*aud.Mimetype)
		}
		if aud.Seconds != nil {
			safe.Seconds = proto.Uint32(*aud.Seconds)
		}
		return &waProto.Message{AudioMessage: safe}
	}
	// Document status -> keep title/filename/mimetype/thumbnail/caption
	if doc := msg.DocumentMessage; doc != nil {
		safe := &waProto.DocumentMessage{}
		if doc.Title != nil {
			safe.Title = proto.String(*doc.Title)
		}
		if doc.FileName != nil {
			safe.FileName = proto.String(*doc.FileName)
		}
		if doc.Mimetype != nil {
			safe.Mimetype = proto.String(*doc.Mimetype)
		}
		if len(doc.JPEGThumbnail) > 0 {
			safe.JPEGThumbnail = doc.JPEGThumbnail
		}
		if doc.Caption != nil {
			safe.Caption = proto.String(*doc.Caption)
		}
		if doc.FileLength != nil {
			safe.FileLength = proto.Uint64(*doc.FileLength)
		}
		if doc.PageCount != nil {
			safe.PageCount = proto.Uint32(*doc.PageCount)
		}
		return &waProto.Message{DocumentMessage: safe}
	}
	// Sticker status -> keep mimetype/dimensions/length/pngThumbnail
	if stk := msg.StickerMessage; stk != nil {
		safe := &waProto.StickerMessage{}
		if stk.Mimetype != nil {
			safe.Mimetype = proto.String(*stk.Mimetype)
		}
		if stk.Height != nil {
			safe.Height = proto.Uint32(*stk.Height)
		}
		if stk.Width != nil {
			safe.Width = proto.Uint32(*stk.Width)
		}
		if stk.FileLength != nil {
			safe.FileLength = proto.Uint64(*stk.FileLength)
		}
		if len(stk.PngThumbnail) > 0 {
			safe.PngThumbnail = stk.PngThumbnail
		}
		return &waProto.Message{StickerMessage: safe}
	}
	// Text / link / contact / location / conversation -> already light, return as-is
	return msg
}

// GetMentionedJIDs returns the @-mentioned JIDs parsed from the incoming
// message identified by info. It reads ContextInfo.MentionedJID from the
// cached incoming message proto. Returns the list and true if any mentions
// were present; nil,false otherwise.
func (b *bridge) GetMentionedJIDs(info types.MessageInfo) ([]string, bool) {
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return nil, false
	}
	var mentioned []string
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
		mentioned = msg.ExtendedTextMessage.ContextInfo.MentionedJID
	}
	if len(mentioned) == 0 {
		return nil, false
	}
	return mentioned, true
}

// ── ANTI (antilink / antibot / antibad) per-group Redis config ──────────────

// GetGroupSetting reads a per-GROUP setting from Redis (Upstash). Uses the
// settings:<groupJID> hash with the given field. Returns def if not set.
// redisNilOnce ensures the "Redis not configured" warning prints only once
// per process lifetime, so the log isn't spammed on every setting access.
var redisNilOnce sync.Once

// warnRedisNil prints a one-time warning when Redis is not available.
// This is the root-cause guard: if Upstash Redis is nil (because
// GOLDMD_DISABLE_UPSTASH=1 is set or UPSTASH_REDIS_REST_URL/TOKEN are
// missing), settings will NOT persist — toggles appear to succeed but
// always read back the default. The warning makes this immediately visible.
func warnRedisNil() {
	redisNilOnce.Do(func() {
		ErrLog("Upstash Redis is NOT connected - settings (antilink, statusseen, " +
			"typing, mode, etc.) will NOT persist! Remove GOLDMD_DISABLE_UPSTASH=1 or " +
			"set UPSTASH_REDIS_REST_URL + UPSTASH_REDIS_REST_TOKEN in your environment.")
	})
}

func (b *bridge) GetGroupSetting(groupJID, field, def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(groupJID, field, def)
}

// SetGroupSetting writes a per-GROUP setting to Redis (Upstash).
func (b *bridge) SetGroupSetting(groupJID, field, val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(groupJID, field, val)
}

// ── ANTIGCCALL JSON debug (file: nexstore/gccall_debug.jsonl) ──────────

// groupSetKey builds the Redis key for a per-group SET, namespaced under the
// bot JID so different bots on the same Upstash don't collide.
func (b *bridge) groupSetKey(groupJID, setName string) string {
	return "goldmd:" + b.s.JID + ":groupset:" + groupJID + ":" + setName
}

// GroupSetAdd adds a member to a per-group Redis SET.
func (b *bridge) GroupSetAdd(groupJID, setName, member string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	return b.s.Manager.Redis.setAdd(b.groupSetKey(groupJID, setName), member)
}

// GroupSetRem removes a member from a per-group Redis SET.
func (b *bridge) GroupSetRem(groupJID, setName, member string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	return b.s.Manager.Redis.setRem(b.groupSetKey(groupJID, setName), member)
}

// GroupSetMembers returns all members of a per-group Redis SET.
func (b *bridge) GroupSetMembers(groupJID, setName string) []string {
	if b.s.Manager.Redis == nil {
		return nil
	}
	return b.s.Manager.Redis.setMembers(b.groupSetKey(groupJID, setName))
}

// GroupSetClear removes an entire per-group Redis SET (used on reset).
func (b *bridge) GroupSetClear(groupJID, setName string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	return b.s.Manager.Redis.setDel(b.groupSetKey(groupJID, setName))
}

// ── GROUP ADMIN / MODERATION actions ────────────────────────────────────────

// KickGroupMember removes the given user JIDs from the group. Mirrors the
// Baileys groupParticipantsUpdate(jid, [jid], 'remove') call.
func (b *bridge) KickGroupMember(groupJID types.JID, targets []types.JID) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	if len(targets) == 0 {
		return nil
	}
	_, err := b.s.Client.UpdateGroupParticipants(context.Background(), groupJID, targets, whatsmeow.ParticipantChangeRemove)
	return err
}

// DeleteAnyMessage revokes (deletes-for-everyone) a message in a chat by its
// message ID and the original sender JID. Mirrors Baileys sendMessage delete.
func (b *bridge) DeleteAnyMessage(chat types.JID, senderJID string, messageID string) error {
	return b.s.RevokeAnyMessage(chat, senderJID, messageID)
}

// IsGroupAdmin reports whether the given user JID is an admin/super-admin of
// the group identified by groupJID.
func (b *bridge) IsGroupAdmin(groupJID, userJID types.JID) bool {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return false
	}
	gi, err := b.s.Client.GetGroupInfo(context.Background(), groupJID)
	if err != nil {
		return false
	}
	for _, p := range gi.Participants {
		if p.JID == userJID && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

// ResolveToPN converts a LID (@lid) JID to its real phone-number (@s.whatsapp.net)
// JID using the whatsmeow LID store. If the JID is already a PN, or the mapping
// is unknown, the original JID is returned unchanged. This is the MANDATORY
// LID->PN conversion applied by every group command.
func (b *bridge) ResolveToPN(jid types.JID) types.JID {
	if b.s == nil || b.s.Client == nil || b.s.Client.Store == nil {
		return jid
	}
	if jid.Server == types.DefaultUserServer {
		return jid
	}
	if jid.Server != types.HiddenUserServer {
		return jid
	}
	if b.s.Client.Store.LIDs == nil {
		return jid
	}
	if pn, err := b.s.Client.Store.LIDs.GetPNForLID(context.Background(), jid); err == nil && !pn.IsEmpty() {
		return pn
	}
	return jid
}

// ── MESSAGE inspection (for anti-* detection) ───────────────────────────────

// GetMessageText returns the full text body of the incoming message identified
// by info, including image/video captions. "" if no text.
func (b *bridge) GetMessageText(info types.MessageInfo) string {
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return ""
	}
	return fullMessageText(msg)
}

// GetRawMessage returns the raw waProto.Message for the incoming message
// identified by info (from the in-memory message cache). Returns nil if the
// message is not cached. Used by antistatus enforcement to detect group
// status mention messages (GroupStatusMentionMessage field).
func (b *bridge) GetRawMessage(info types.MessageInfo) *waProto.Message {
	return b.s.getCachedMessage(info.ID)
}

// GetQuotedMessageText returns the text of the quoted/replied-to message.
// Walks into ContextInfo.QuotedMessage (which is a *waProto.Message) and
// reuses fullMessageText so conversation / extendedText / captions all
// work. Returns "" if not a reply or no text.
func (b *bridge) GetQuotedMessageText(info types.MessageInfo) string {
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return ""
	}
	ci := extractContextInfoFromMsg(msg)
	if ci == nil || ci.QuotedMessage == nil {
		return ""
	}
	return fullMessageText(ci.QuotedMessage)
}

// fullMessageText extracts text from conversation, extendedText, and
// image/video captions — mirroring the Node.js detection body extraction.
func fullMessageText(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil && *msg.Conversation != "" {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil && *msg.ExtendedTextMessage.Text != "" {
		return *msg.ExtendedTextMessage.Text
	}
	if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil && *msg.ImageMessage.Caption != "" {
		return *msg.ImageMessage.Caption
	}
	if msg.VideoMessage != nil && msg.VideoMessage.Caption != nil && *msg.VideoMessage.Caption != "" {
		return *msg.VideoMessage.Caption
	}
	// OWNER ORDER (AI media reading): a document (e.g. a PDF) may carry the
	// command as its caption — ".gpt summarise this". Read it too so the
	// command dispatches and the AI handler can OCR the document.
	if msg.DocumentMessage != nil && msg.DocumentMessage.Caption != nil && *msg.DocumentMessage.Caption != "" {
		return *msg.DocumentMessage.Caption
	}
	return ""
}

// isBotForwardStrong mirrors UmarIsBotForwardStrong: forwardingScore >= 2 OR
// forwardedNewsletterMessageInfo present OR remoteJid ends with @newsletter.
func isBotForwardStrong(ci *waProto.ContextInfo) bool {
	if ci == nil {
		return false
	}
	if ci.ForwardingScore != nil && *ci.ForwardingScore >= 2 {
		return true
	}
	if ci.ForwardedNewsletterMessageInfo != nil {
		return true
	}
	rj := ""
	if ci.RemoteJID != nil {
		rj = *ci.RemoteJID
	}
	if strings.HasSuffix(rj, "@newsletter") {
		return true
	}
	return false
}

// isBotForwardWeak mirrors UmarIsBotForwardWeak: isForwarded && score == 1.
func isBotForwardWeak(ci *waProto.ContextInfo) bool {
	if ci == nil {
		return false
	}
	if ci.IsForwarded != nil && *ci.IsForwarded {
		score := uint32(0)
		if ci.ForwardingScore != nil {
			score = *ci.ForwardingScore
		}
		return score == 1
	}
	return false
}

// extractContextInfoFromMsg pulls the ContextInfo from any message type that
// carries one (extendedText, image, video, audio, document, sticker).
func extractContextInfoFromMsg(msg *waProto.Message) *waProto.ContextInfo {
	if msg == nil {
		return nil
	}
	switch {
	case msg.ExtendedTextMessage != nil:
		return msg.ExtendedTextMessage.ContextInfo
	case msg.ImageMessage != nil:
		return msg.ImageMessage.ContextInfo
	case msg.VideoMessage != nil:
		return msg.VideoMessage.ContextInfo
	case msg.PtvMessage != nil:
		return msg.PtvMessage.ContextInfo
	case msg.AudioMessage != nil:
		return msg.AudioMessage.ContextInfo
	case msg.DocumentMessage != nil:
		return msg.DocumentMessage.ContextInfo
	case msg.StickerMessage != nil:
		return msg.StickerMessage.ContextInfo
	}
	for _, inner := range []*waProto.Message{
		msg.ViewOnceMessage.GetMessage(),
		msg.ViewOnceMessageV2.GetMessage(),
		msg.ViewOnceMessageV2Extension.GetMessage(),
	} {
		if inner != nil {
			if ci := extractContextInfoFromMsg(inner); ci != nil {
				return ci
			}
		}
	}
	return nil
}

// IsForwardedBot reports whether the incoming message looks like a forwarded
// bot/spam message. Mirrors UmarIsBotForward (strong OR weak+stanzaId).
func (b *bridge) IsForwardedBot(info types.MessageInfo) bool {
	msg := b.s.getCachedMessage(info.ID)
	if msg == nil {
		return false
	}
	ci := extractContextInfoFromMsg(msg)
	if isBotForwardStrong(ci) {
		return true
	}
	if isBotForwardWeak(ci) {
		// weak needs a stanzaId (quoted message) to count
		if ci.StanzaID != nil && *ci.StanzaID != "" {
			return true
		}
	}
	return false
}

// ── CUSTOM VOICE (addvoice) ─────────────────────────────────────────────────
//
// Voices are stored on the local filesystem under <DataDir>/voices/<botJID>/
// with the name as the filename (lowercased, sanitised). A small index of
// voice names is kept in a Redis SET (goldmd:<botJID>:voices) so voicelist
// works even if the filesystem is scanned lazily.

// voicesDir returns the directory where this bot's custom voices are stored.
func (b *bridge) voicesDir() string {
	return filepath.Join(b.s.Manager.cfg.DataDir, "voices", b.s.JID)
}

// voicePath returns the on-disk path for a named voice.
func (b *bridge) voicePath(name string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, strings.ToLower(strings.TrimSpace(name)))
	return filepath.Join(b.voicesDir(), safe+".bin")
}

// SaveCustomVoice saves an audio clip under the given name. The mime is
// recorded in a sidecar file (<name>.mime) so the trigger can replay it with
// the correct mimetype.
func (b *bridge) SaveCustomVoice(name string, data []byte, mime string) bool {
	return b.saveVoiceLocal(name, data, mime, true)
}

// saveVoiceLocal writes the voice payload + .mime sidecar and maintains the
// Redis name index. When backup is true the bytes are also mirrored to the
// durable store (async) so a disk wipe can restore them later.
func (b *bridge) saveVoiceLocal(name string, data []byte, mime string, backup bool) bool {
	if name == "" || len(data) == 0 {
		return false
	}
	dir := b.voicesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		ErrLog("[%s] saveVoiceLocal mkdir: %v", b.s.JID, err)
		return false
	}
	if err := os.WriteFile(b.voicePath(name), data, 0o644); err != nil {
		ErrLog("[%s] saveVoiceLocal write: %v", b.s.JID, err)
		return false
	}
	_ = os.WriteFile(b.voicePath(name)+".mime", []byte(mime), 0o644)
	// Track name in Redis set
	if b.s.Manager.Redis != nil {
		_ = b.s.Manager.Redis.setAdd("goldmd:"+b.s.JID+":voices", strings.ToLower(strings.TrimSpace(name)))
	}
	if backup {
		go assetStorjPut(assetStorjNS("voice"), assetStorjID(b.s.JID, sanitiseAssetName(name)), data, mime, "")
	}
	return true
}

// GetCustomVoice loads the audio bytes + mime for a saved voice name.
func (b *bridge) GetCustomVoice(name string) ([]byte, string, bool) {
	p := b.voicePath(name)
	data, err := os.ReadFile(p)
	if err != nil || len(data) == 0 {
		// Local file gone (RAM-disk wipe / fresh host): rehydrate from store.
		if b.voiceStorjRestoreInto(name) {
			data, err = os.ReadFile(p)
		}
		if err != nil || len(data) == 0 {
			return nil, "", false
		}
	}
	mime := "audio/mp4"
	if m, err := os.ReadFile(p + ".mime"); err == nil && len(m) > 0 {
		mime = string(m)
	}
	return data, mime, true
}

// DeleteCustomVoice removes a saved voice by name.
func (b *bridge) DeleteCustomVoice(name string) bool {
	p := b.voicePath(name)
	if _, err := os.Stat(p); err != nil {
		return false
	}
	_ = os.Remove(p)
	_ = os.Remove(p + ".mime")
	if b.s.Manager.Redis != nil {
		_ = b.s.Manager.Redis.setRem("goldmd:"+b.s.JID+":voices", strings.ToLower(strings.TrimSpace(name)))
	}
	go assetStorjDelete(assetStorjNS("voice"), assetStorjID(b.s.JID, sanitiseAssetName(name)))
	return true
}

// ListCustomVoices returns the names of all saved voices (from Redis set,
// falling back to scanning the directory).
func (b *bridge) ListCustomVoices() []string {
	var names []string
	if b.s.Manager.Redis != nil {
		names = b.s.Manager.Redis.setMembers("goldmd:" + b.s.JID + ":voices")
	}
	if len(names) == 0 {
		// fallback: scan directory
		dir := b.voicesDir()
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				n := e.Name()
				if strings.HasSuffix(n, ".mime") {
					continue
				}
				if strings.HasSuffix(n, ".bin") {
					n = strings.TrimSuffix(n, ".bin")
				}
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		// Fresh disk + empty index: rebuild the name list from the store.
		names = assetStorjListNames(assetStorjNS("voice"), b.s.JID)
	}
	sort.Strings(names)
	return names
}

// SendVoiceMessage sends an audio clip (raw bytes) as a non-PTT audio message
// to the chat identified by info. Mirrors Baileys { audio, mimetype, ptt:false }.
func (b *bridge) SendVoiceMessage(info types.MessageInfo, data []byte, mime string) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	if mime == "" {
		mime = "audio/mp4"
	}
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Mimetype:      proto.String(mime),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			PTT:           proto.Bool(false),
		},
	})
	return err
}

// ── BOT-WIDE BANNED USERS (botblock / botunblock / banlist) ──────────────
// Stored in Redis SET "banned:set" (JID membership, already used by
// upstash.go IsBanned) + a per-user metadata JSON key for number/reason/
// bannedBy/bannedAt. The metadata key is "goldmd:<botJID>:bannedmeta:<userJID>".

func (b *bridge) bannedMetaKey(userJID string) string {
	return "goldmd:" + b.s.JID + ":bannedmeta:" + userJID
}

func (b *bridge) BotBanAdd(jid, number, reason string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	b.s.Manager.Redis.BanUser(jid)
	// Store metadata as JSON
	meta := fmt.Sprintf(`{"number":"%s","reason":"%s","bannedby":"owner","bannedat":"%s"}`,
		number, reason, time.Now().UTC().Format("2006-01-02"))
	_ = b.s.Manager.Redis.setString(b.bannedMetaKey(jid), meta)
	return nil
}

func (b *bridge) BotBanRemove(jid string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	b.s.Manager.Redis.UnbanUser(jid)
	// Delete metadata key (redis-safe: DEL is idempotent)
	_, _ = b.s.Manager.Redis.cmd("DEL", b.bannedMetaKey(jid))
	return nil
}

func (b *bridge) BotBanIsBanned(jid string) bool {
	if b.s.Manager.Redis == nil {
		return false
	}
	return b.s.Manager.Redis.IsBanned(jid)
}

func (b *bridge) BotBanList() []goldcmds.BannedUserInfo {
	if b.s.Manager.Redis == nil {
		return nil
	}
	jids := b.s.Manager.Redis.BannedList()
	out := make([]goldcmds.BannedUserInfo, 0, len(jids))
	for _, jid := range jids {
		info := goldcmds.BannedUserInfo{JID: jid}
		// Extract number from JID (strip @s.whatsapp.net)
		info.Number = strings.ReplaceAll(strings.ReplaceAll(jid, "@s.whatsapp.net", ""), "@lid", "")
		// Try to load metadata
		raw := b.s.Manager.Redis.safeString(b.bannedMetaKey(jid), "")
		if raw != "" {
			var meta struct {
				Number   string `json:"number"`
				Reason   string `json:"reason"`
				BannedBy string `json:"bannedby"`
				BannedAt string `json:"bannedat"`
			}
			if json.Unmarshal([]byte(raw), &meta) == nil {
				if meta.Number != "" {
					info.Number = meta.Number
				}
				info.Reason = meta.Reason
				info.BannedBy = meta.BannedBy
				info.BannedAt = meta.BannedAt
			}
		}
		out = append(out, info)
	}
	return out
}

// ── PREMIUM USERS (antilinkprem / antibotprem / antibadprem) ─────────────
// Stored in Redis SET "premium:set" (already has IsPremium in upstash.go).
// Metadata (number) stored in "goldmd:<botJID>:premiummeta:<userJID>".

// BotBanMemberTails returns the "number tail" (last 10 digits) of every
// JID in the bot-wide banned list. Used by the fast number-tolerant ban
// check (botBanSenderCheck) so it can match a user blocked with a LOCAL
// number (e.g. 0327...) or INTERNATIONAL number (9232...) against any
// sender JID format (LID or phone).
//
// The underlying member list is CACHED in-memory (BannedMembersCached),
// so this is 0ms after the first call and never blocks the bot.
func (b *bridge) BotBanMemberTails() []string {
	if b.s.Manager.Redis == nil {
		return nil
	}
	// Fully cached (Upstash CachedBannedTails): one network call per member
	// on FIRST use, 0ms afterwards, instantly invalidated on ban/unban.
	// Previously this loop hit Upstash (~290ms per banned member) on EVERY
	// non-owner command dispatch.
	return b.s.Manager.Redis.CachedBannedTails(b.s.JID)
}

func (b *bridge) premiumMetaKey(userJID string) string {
	return "goldmd:" + b.s.JID + ":premiummeta:" + userJID
}

func (b *bridge) PremiumAdd(jid, number string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	_ = b.s.Manager.Redis.setAdd("premium:set", jid)
	_ = b.s.Manager.Redis.setString(b.premiumMetaKey(jid), number)
	return nil
}

func (b *bridge) PremiumRemove(jid string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	_ = b.s.Manager.Redis.setRem("premium:set", jid)
	_, _ = b.s.Manager.Redis.cmd("DEL", b.premiumMetaKey(jid))
	return nil
}

func (b *bridge) PremiumList() []goldcmds.PremiumUserInfo {
	if b.s.Manager.Redis == nil {
		return nil
	}
	jids := b.s.Manager.Redis.setMembers("premium:set")
	out := make([]goldcmds.PremiumUserInfo, 0, len(jids))
	for _, jid := range jids {
		info := goldcmds.PremiumUserInfo{JID: jid}
		info.Number = strings.ReplaceAll(strings.ReplaceAll(jid, "@s.whatsapp.net", ""), "@lid", "")
		raw := b.s.Manager.Redis.safeString(b.premiumMetaKey(jid), "")
		if raw != "" {
			info.Number = raw
		}
		out = append(out, info)
	}
	return out
}

func (b *bridge) PremiumIsMember(jid string) bool {
	if b.s.Manager.Redis == nil {
		return false
	}
	return b.s.Manager.Redis.IsPremium(jid)
}

// PremiumMemberTails returns the "number tail" (last 10 digits) of every
// JID in the premium list. Used by the fast number-tolerant bypass check
// (premiumSenderBypass) so it can match a user added with a LOCAL number
// (e.g. 0327...) against an INTERNATIONAL sender JID (9232...).
//
// The underlying member list is CACHED in-memory (PremiumMembersCached),
// so this is 0ms after the first call and never blocks the bot.
func (b *bridge) PremiumMemberTails() []string {
	if b.s.Manager.Redis == nil {
		return nil
	}
	jids := b.s.Manager.Redis.PremiumMembersCached()
	out := make([]string, 0, len(jids))
	for _, jid := range jids {
		num := jid
		if i := strings.IndexByte(jid, '@'); i >= 0 {
			num = jid[:i]
		}
		// prefer stored metadata number (may be local format)
		if raw := b.s.Manager.Redis.safeString(b.premiumMetaKey(jid), ""); raw != "" {
			num = raw
		}
		// last 10 digits
		var digits []byte
		for i := 0; i < len(num); i++ {
			c := num[i]
			if c >= '0' && c <= '9' {
				digits = append(digits, c)
			}
		}
		if len(digits) == 0 {
			continue
		}
		if len(digits) > 10 {
			digits = digits[len(digits)-10:]
		}
		out = append(out, string(digits))
	}
	return out
}

// ── PER-GROUP BANNED USERS (bangcuser / unbangcuser) ─────────────────────
// Stored in per-group Redis SET "bangcuser" (via GroupSetAdd/Rem/Members).
// BannedBy stored in "goldmd:<botJID>:groupset:<groupJID>:bangcusermeta:<userJID>".

func (b *bridge) groupBanUserMetaKey(groupJID, userJID string) string {
	return b.groupSetKey(groupJID, "bangcusermeta:"+userJID)
}

func (b *bridge) GroupBanUserAdd(groupJID, userJID, bannedBy string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	_ = b.s.Manager.Redis.setAdd(b.groupSetKey(groupJID, "bangcuser"), userJID)
	_ = b.s.Manager.Redis.setString(b.groupBanUserMetaKey(groupJID, userJID), bannedBy)
	return nil
}

func (b *bridge) GroupBanUserRemove(groupJID, userJID string) error {
	if b.s.Manager.Redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	_ = b.s.Manager.Redis.setRem(b.groupSetKey(groupJID, "bangcuser"), userJID)
	_, _ = b.s.Manager.Redis.cmd("DEL", b.groupBanUserMetaKey(groupJID, userJID))
	return nil
}

func (b *bridge) GroupBanUserIsBanned(groupJID, userJID string) bool {
	if b.s.Manager.Redis == nil {
		return false
	}
	members := b.s.Manager.Redis.setMembers(b.groupSetKey(groupJID, "bangcuser"))
	for _, m := range members {
		if m == userJID {
			return true
		}
	}
	return false
}

func (b *bridge) GroupBanUserList(groupJID string) []goldcmds.GroupBannedUserInfo {
	if b.s.Manager.Redis == nil {
		return nil
	}
	members := b.s.Manager.Redis.setMembers(b.groupSetKey(groupJID, "bangcuser"))
	out := make([]goldcmds.GroupBannedUserInfo, 0, len(members))
	for _, userJID := range members {
		info := goldcmds.GroupBannedUserInfo{UserJID: userJID}
		info.BannedBy = b.s.Manager.Redis.safeString(b.groupBanUserMetaKey(groupJID, userJID), "admin")
		out = append(out, info)
	}
	return out
}

// ── AUTO READ (autoread) ─────────────────────────────────────────────────
// Bot-wide setting stored in settings:<botJID> hash, field "autoread".
// Values: "off" / "inbox" / "groups" / "all".

func (b *bridge) GetAutoReadSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoread", def)
}

func (b *bridge) SetAutoReadSetting(mode string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoread", mode)
}

// ── AUTOBLOCK (country-code auto-block) ── per-bot Redis config ──

// GetAutoBlockSetting reads the autoblock on/off flag from Redis.
func (b *bridge) GetAutoBlockSetting(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoblock", def)
}

// SetAutoBlockSetting writes the autoblock on/off flag to Redis.
func (b *bridge) SetAutoBlockSetting(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoblock", val)
}

// GetAutoBlockCodes reads the blocked country codes list from Redis.
func (b *bridge) GetAutoBlockCodes(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoblock:codes", def)
}

// SetAutoBlockCodes writes the blocked country codes list to Redis.
func (b *bridge) SetAutoBlockCodes(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoblock:codes", val)
}

// GetAutoBlockContactSave reads the contact-save exemption mode ("on"/"off").
func (b *bridge) GetAutoBlockContactSave(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoblock:contactsave", def)
}

// SetAutoBlockContactSave writes the contact-save exemption mode ("on"/"off").
func (b *bridge) SetAutoBlockContactSave(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoblock:contactsave", val)
}

// ── AUTOREPLY (GOLD-MD AI auto-reply) ──

// GetAutoReplyMode reads the autoreply mode ("off"/"groups"/"inbox"/"on").
func (b *bridge) GetAutoReplyMode(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoreply", def)
}

// SetAutoReplyMode writes the autoreply mode to Redis.
func (b *bridge) SetAutoReplyMode(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoreply", val)
}

// GetAutoReplyDelay reads the human-typing delay flag ("true"/"false").
func (b *bridge) GetAutoReplyDelay(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoreply:delay", def)
}

// SetAutoReplyDelay writes the human-typing delay flag to Redis.
func (b *bridge) SetAutoReplyDelay(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoreply:delay", val)
}

// GetAutoReplyExcluded reads the comma-joined excluded-user JID list.
func (b *bridge) GetAutoReplyExcluded(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "autoreply:excluded", def)
}

// SetAutoReplyExcluded writes the comma-joined excluded-user JID list.
func (b *bridge) SetAutoReplyExcluded(val string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "autoreply:excluded", val)
}

// ── BANCMD (stopped commands list) ──

// GetBannedCommands reads the comma-joined stopped command names.
func (b *bridge) GetBannedCommands(def string) string {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return def
	}
	return b.s.Manager.Redis.GetSetting(b.s.JID, "bancmd", def)
}

// bcJoinList builds a clean comma-joined list from a raw string.
func bcJoinList(raw string) string {
	seen := map[string]bool{}
	var parts []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, ".")
		if p == "" {
			continue
		}
		if !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ",")
}

// BanCommand appends a name to the stopped-commands list (if absent).
func (b *bridge) BanCommand(name string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, ".")
	if name == "" {
		return
	}
	cur := bcJoinList(b.s.Manager.Redis.GetSetting(b.s.JID, "bancmd", ""))
	for _, p := range strings.Split(cur, ",") {
		if p == name {
			return
		}
	}
	if cur == "" {
		cur = name
	} else {
		cur = cur + "," + name
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "bancmd", cur)
}

// UnbanCommand removes a name from the stopped-commands list (if present).
func (b *bridge) UnbanCommand(name string) {
	if b.s.Manager.Redis == nil {
		warnRedisNil()
		return
	}
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, ".")
	if name == "" {
		return
	}
	cur := bcJoinList(b.s.Manager.Redis.GetSetting(b.s.JID, "bancmd", ""))
	var keep []string
	for _, p := range strings.Split(cur, ",") {
		if p != name {
			keep = append(keep, p)
		}
	}
	b.s.Manager.Redis.SetSetting(b.s.JID, "bancmd", strings.Join(keep, ","))
}

// ── QUOTED SEND (autoreply) ──

// SendQuotedTextWithID sends text quoting the given message, WITHOUT the
// bot footer. Returns the sent message ID ("" on failure).
func (b *bridge) SendQuotedTextWithID(info types.MessageInfo, quoted *waProto.Message, text string) string {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return ""
	}
	// keep a local copy so the caller's message object is never mutated
	quotedCopy := proto.Clone(quoted).(*waProto.Message)
	ctx := &waProto.ContextInfo{
		StanzaID:      proto.String(info.ID),
		Participant:   proto.String(info.Sender.String()),
		QuotedMessage: quotedCopy,
	}
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ctx,
		},
	}
	resp, err := b.s.Client.SendMessage(context.Background(), info.Chat, msg)
	if err != nil {
		return ""
	}
	return resp.ID
}

func init() {
	// Build the owner-only command set for handler.go dispatch.
	for name := range goldcmds.OwnerOnlySet() {
		ownerOnlyCommands[name] = true
	}
	for _, command := range goldcmds.Commands() {
		cmd := command
		RegisterCommand(cmd.Name, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			b := &bridge{s: s}
			// BUSY TRACKING: har gold-cmds command (video/play/tiktok/yts
			// picks - sab download pipelines) in-flight mark hoti hai;
			// watchdog + memoryWatchdog is dauran restart / fast-reconnect
			// nahi bhejenge. Panic-safe: defer endCmdBusy.
			beginCmdBusy()
			func() {
				defer endCmdBusy()
				cmd.Run(b, info, args, prefix)
			}()
		})
	}
}

// ───────────────────────────────────────────────────────────────────────────
//   GOLD-MD — Bot MEMORY bridge methods (used by .automsg)
//
//   These expose the bot's secure MEMORY to the gold-cmds plugin package.
//   Internally they delegate to the main-package global store that the bot
//   ALSO uses for antidelete / antiedit message recovery — so the .automsg
//   schedule lives in exactly the same secure place, fetched the same way.
//   (The internal backend name is deliberately NOT surfaced to the user; the
//   command replies only ever say "bot memory".)
// ───────────────────────────────────────────────────────────────────────────

func (b *bridge) MemoryReady() bool {
	return storj.Ready()
}

func (b *bridge) MemorySave(id string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.PutJSON(ctx, id, data)
}

func (b *bridge) MemoryLoad(id string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.GetJSON(ctx, id)
}

func (b *bridge) MemoryDelete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.DeleteJSON(ctx, id)
}

func (b *bridge) MemoryList() ([]goldcmds.AutomsgListItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	entries, err := storj.ListJSON(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]goldcmds.AutomsgListItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, goldcmds.AutomsgListItem{ID: e.ID, Data: e.Data})
	}
	return out, nil
}

// ── Namespaced bot MEMORY bridge methods (used by .dissmisstime / .admintime) ──
// Same secure store as the automsg methods above, but under a caller-chosen
// namespace so timed-admin timers live in their OWN space and never show up in
// .automsg list.

func (b *bridge) MemorySaveNS(ns, id string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.PutJSONNS(ctx, ns, id, data)
}

func (b *bridge) MemoryLoadNS(ns, id string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.GetJSONNS(ctx, ns, id)
}

func (b *bridge) MemoryDeleteNS(ns, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return storj.DeleteJSONNS(ctx, ns, id)
}

func (b *bridge) MemoryListNS(ns string) ([]goldcmds.AutomsgListItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	entries, err := storj.ListJSONNS(ctx, ns)
	if err != nil {
		return nil, err
	}
	out := make([]goldcmds.AutomsgListItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, goldcmds.AutomsgListItem{ID: e.ID, Data: e.Data})
	}
	return out, nil
}

// ── AUTOREACT / OWNERREACT ──────────────────────────────────────────────────

// SendReaction sends an emoji reaction to a specific message in a chat.
// Uses whatsmeow's BuildReaction which creates a proper ReactionMessage
// with the correct MessageKey (RemoteJID, FromMe, ID, Participant).
// chat = the chat where the target message lives,
// sender = the original sender of the target message,
// msgID = the target message ID,
// emoji = the reaction emoji to send.
func (b *bridge) SendReaction(chat, sender types.JID, msgID, emoji string) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not ready")
	}
	// BuildReaction produces a *waProto.Message with a ReactionMessage whose
	// Key points at the target message. This is the whatsmeow equivalent of
	// Baileys' sendMessage(..., { react: { text, key } }).
	reactMsg := b.s.Client.BuildReaction(chat, sender, msgID, emoji)
	_, err := b.s.Client.SendMessage(context.Background(), chat, reactMsg)
	if err != nil {
		return err
	}
	return nil
}

// ── WELCOME / GOODBYE ───────────────────────────────────────────────────────

// SendImageWithMentions sends an image (raw bytes) with a caption that
// @-mentions the given JIDs. Used by the welcome/goodbye engine to send the
// group DP image + caption + @mention when a member joins/leaves.
// chat = the group JID to send to,
// data = raw image bytes (JPEG),
// caption = the caption text (already has @user/@gname replaced),
// mentioned = list of full JIDs to @-mention.
func (b *bridge) SendImageWithMentions(chat types.JID, data []byte, caption string, mentioned []string) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not ready")
	}
	if len(data) == 0 {
		return fmt.Errorf("no image data")
	}
	caption = b.s.withCaptionFooter(caption) // botname footer on every image
	resp, err := b.s.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	msg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			Caption:       proto.String(caption),
			Mimetype:      proto.String("image/jpeg"),
			MediaKey:      resp.MediaKey,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			ContextInfo: &waProto.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	}
	_, err = b.s.Client.SendMessage(context.Background(), chat, msg)
	return err
}

// GetGroupName returns the subject (name) of the group identified by groupJID.
// Used by the welcome/goodbye engine to resolve @gname. Falls back to
// "this group" (same as Node.js fallback) if the group info cannot be fetched.
func (b *bridge) GetGroupName(groupJID types.JID) string {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return "this group"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := b.s.Client.GetGroupInfo(ctx, groupJID)
	if err != nil || info == nil || info.Name == "" {
		return "this group"
	}
	return info.Name
}

// GetGroupProfilePicture downloads the group's profile picture (DP) as raw
// JPEG bytes. Returns nil if the group has no picture or the download fails.
// Used by the welcome/goodbye engine to send the group DP with the caption.
func (b *bridge) GetGroupProfilePicture(groupJID types.JID) []byte {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pic, err := b.s.Client.GetProfilePictureInfo(ctx, groupJID, &whatsmeow.GetProfilePictureParams{})
	if err != nil || pic == nil || pic.URL == "" {
		return nil
	}
	// ProfilePictureInfo.URL is a plain HTTPS URL — download it with a
	// simple HTTP GET (per whatsmeow docs: "can be downloaded with a
	// simple HTTP request").
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pic.URL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// SendTextWithMentions sends a plain text message (no image) that @-mentions
// the given JIDs. Used by the welcome/goodbye engine as a fallback when the
// group has no DP image.
func (b *bridge) SendTextWithMentions(chat types.JID, text string, mentioned []string) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not ready")
	}
	text = b.s.withFooter(text) // botname footer on every text
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waProto.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	}
	_, err := b.s.Client.SendMessage(context.Background(), chat, msg)
	return err
}
