package main

// ============================================================================
// GOLD-MD — WhatsApp Circle (video note / "PTV") sender + generic named asset
// store (used by .addimg/.addvideo/.addsticker/.addtext/.addcircle).
//
// Circle video: WhatsApp expects a Message.PtvMessage (field 66) holding a
// VideoMessage. The clip must be SQUARE (the caller square-crops it with
// ffmpeg before uploading) — otherwise WhatsApp rejects/letterboxes it.
//
// Named assets: same on-disk model as .addvoice — one folder per bot under
// <DataDir>/assets/<jid>/<kind>/, a sidecar file for the mimetype (and the
// original text for .addtext), plus a Redis SET index for fast listing.
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	goldcmds "gold-md/gold-cmds"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// assetKinds are the folders .add* commands write into.
var assetKinds = map[string]string{
	"img":     "img",
	"video":   "video",
	"sticker": "sticker",
	"text":    "text",
	"circle":  "circle",
}

func (b *bridge) assetsDir(kind string) string {
	sub := assetKinds[kind]
	if sub == "" {
		sub = kind
	}
	return filepath.Join(b.s.Manager.cfg.DataDir, "assets", b.s.JID, sub)
}

// sanitiseAssetName maps a user-supplied asset name to the on-disk / object
// key form (lowercase, [a-z0-9_-] only) so a name can never escape the folder
// or collide with the Storj namespace separator.
func sanitiseAssetName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, strings.ToLower(strings.TrimSpace(name)))
}

// ---------------------------------------------------------------------------
// AUTO-SEND EDIT STYLE (.addtext)
// ---------------------------------------------------------------------------

// The saved .addtext body is delivered the same way the autoreply pipeline
// delivers its replies (see arRunReplyJob): send the text, then after a short
// pause EDIT that very message to the same text. The edit is what puts the
// "Edited" mark on the message, which is exactly the look the owner wants.
const assetEditDelay = 1000 * time.Millisecond

// assetTextSender is the slice of the bridge the edited text delivery needs.
// Narrow interface so the send-then-edit order can be tested with a recorder.
type assetTextSender interface {
	ReplyWithID(info types.MessageInfo, text string) string
	EditMessage(info types.MessageInfo, messageID string, newText string) bool
}

// sendAssetTextEdited sends the saved text and then edits that same message to
// the same text, exactly like arRunReplyJob does for autoreplies. Returns the
// message ID (empty when the send failed, in which case no edit is attempted).
func sendAssetTextEdited(s assetTextSender, info types.MessageInfo, text string, delay time.Duration) string {
	id := s.ReplyWithID(info, text)
	if id == "" {
		return ""
	}
	time.Sleep(delay)
	s.EditMessage(info, id, text)
	return id
}

// assetPath returns the on-disk path for a named asset.
func (b *bridge) assetPath(kind, name string) string {
	safe := sanitiseAssetName(name)
	return filepath.Join(b.assetsDir(kind), safe+".bin")
}

// SaveCustomAsset stores bytes for a named asset of the given kind. mime is
// recorded in a sidecar (<name>.mime); text assets keep their payload as-is.
func (b *bridge) SaveCustomAsset(kind, name string, data []byte, mime string) bool {
	return b.writeAssetLocal(kind, name, data, mime, "", true)
}

// writeAssetLocal writes the disk payload + sidecars and maintains the Redis
// name index. When backup is true the bytes are also mirrored to the durable
// store (async) so a disk wipe can restore them later.
func (b *bridge) writeAssetLocal(kind, name string, data []byte, mime, meta string, backup bool) bool {
	if name == "" || len(data) == 0 {
		return false
	}
	dir := b.assetsDir(kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		ErrLog("[%s] writeAssetLocal mkdir: %v", b.s.JID, err)
		return false
	}
	p := b.assetPath(kind, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		ErrLog("[%s] writeAssetLocal write: %v", b.s.JID, err)
		return false
	}
	_ = os.WriteFile(p+".mime", []byte(mime), 0o644)
	if meta != "" {
		_ = os.WriteFile(p+".meta", []byte(meta), 0o644)
	}
	if b.s.Manager.Redis != nil {
		_ = b.s.Manager.Redis.setAdd("goldmd:"+b.s.JID+":"+kind, strings.ToLower(strings.TrimSpace(name)))
	}
	if backup {
		ns := assetStorjNS(kind)
		id := assetStorjID(b.s.JID, sanitiseAssetName(name))
		go assetStorjPut(ns, id, data, mime, meta)
	}
	return true
}

// SaveCustomAssetMeta is SaveCustomAsset plus a metadata sidecar
// (<name>.meta) used by .addcircle to remember seconds,width,height so the
// asset can be replayed as a real WhatsApp circle later.
func (b *bridge) SaveCustomAssetMeta(kind, name string, data []byte, mime, meta string) bool {
	return b.writeAssetLocal(kind, name, data, mime, meta, true)
}

// GetCustomAssetMeta loads bytes + mime + meta for a named asset. meta is ""
// when no sidecar exists.
func (b *bridge) GetCustomAssetMeta(kind, name string) ([]byte, string, string, bool) {
	data, mime, ok := b.GetCustomAsset(kind, name)
	if !ok {
		return nil, "", "", false
	}
	meta := ""
	if m, err := os.ReadFile(b.assetPath(kind, name) + ".meta"); err == nil {
		meta = string(m)
	}
	return data, mime, meta, true
}

// GetCustomAsset loads bytes + mime for a named asset. Returns nil,"",false
// when the asset does not exist.
func (b *bridge) GetCustomAsset(kind, name string) ([]byte, string, bool) {
	p := b.assetPath(kind, name)
	data, err := os.ReadFile(p)
	if err != nil || len(data) == 0 {
		// Local file gone (RAM-disk wipe / fresh host): rehydrate from the
		// durable store, which also rewrites the sidecars + index.
		if b.assetStorjRestoreInto(kind, name) {
			data, err = os.ReadFile(p)
		}
		if err != nil || len(data) == 0 {
			return nil, "", false
		}
	}
	mime := "application/octet-stream"
	if m, err := os.ReadFile(p + ".mime"); err == nil && len(m) > 0 {
		mime = string(m)
	}
	return data, mime, true
}

// DeleteCustomAsset removes a named asset of the given kind.
func (b *bridge) DeleteCustomAsset(kind, name string) bool {
	p := b.assetPath(kind, name)
	if _, err := os.Stat(p); err != nil {
		return false
	}
	_ = os.Remove(p)
	_ = os.Remove(p + ".mime")
	_ = os.Remove(p + ".meta")
	if b.s.Manager.Redis != nil {
		_ = b.s.Manager.Redis.setRem("goldmd:"+b.s.JID+":"+kind, strings.ToLower(strings.TrimSpace(name)))
	}
	go assetStorjDelete(assetStorjNS(kind), assetStorjID(b.s.JID, sanitiseAssetName(name)))
	return true
}

// ListCustomAssets returns the sorted names of all assets of a kind. Falls
// back to scanning the folder when Redis has no index (fresh disk restore).
func (b *bridge) ListCustomAssets(kind string) []string {
	var names []string
	if b.s.Manager.Redis != nil {
		names = b.s.Manager.Redis.setMembers("goldmd:" + b.s.JID + ":" + kind)
	}
	if len(names) == 0 {
		entries, err := os.ReadDir(b.assetsDir(kind))
		if err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
					continue
				}
				names = append(names, strings.TrimSuffix(e.Name(), ".bin"))
			}
		}
	}
	if len(names) == 0 {
		// Fresh disk + empty index: rebuild the name list from the durable
		// store (no bodies downloaded here — GetCustomAsset handles that).
		names = assetStorjListNames(assetStorjNS(kind), b.s.JID)
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------------------
// CIRCLE VIDEO (WhatsApp video note / "PTV")
// ---------------------------------------------------------------------------

// SendCircleVideo uploads a SQUARE mp4 and sends it as a WhatsApp circle video
// (Message.PtvMessage). The caller must have square-cropped the clip already —
// WhatsApp renders the video note as a circle and expects a 1:1 frame.
//
// Side effect: turns the bot's presence to "recording" for the duration so the
// circle feels like a live video note (best effort, ignored on failure).
func (b *bridge) SendCircleVideo(info types.MessageInfo, data []byte, seconds uint32, width uint32, height uint32, thumbnail []byte) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
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
		Seconds:       proto.Uint32(seconds),
		Width:         proto.Uint32(width),
		Height:        proto.Uint32(height),
		JPEGThumbnail: thumbnail,
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		PtvMessage: videoMsg,
	})
	return err
}

// SendCircleVideoFile is the streaming variant of SendCircleVideo — the file
// is uploaded directly (no full read into RAM), same wire format.
func (b *bridge) SendCircleVideoFile(info types.MessageInfo, path string, seconds uint32, width uint32, height uint32, thumbnail []byte) error {
	if b.s.Client == nil || !b.s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
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
	videoMsg := &waProto.VideoMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		Mimetype:      proto.String("video/mp4"),
		MediaKey:      resp.MediaKey,
		FileLength:    proto.Uint64(resp.FileLength),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
		Seconds:       proto.Uint32(seconds),
		Width:         proto.Uint32(width),
		Height:        proto.Uint32(height),
		JPEGThumbnail: thumbnail,
	}
	_, err = b.s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		PtvMessage: videoMsg,
	})
	return err
}

// ---------------------------------------------------------------------------
// ASSET AUTO-SEND TRIGGER
// ---------------------------------------------------------------------------

// applyAssetTrigger sends a saved asset when the incoming plain-text body is a
// bare asset name (single word, no command prefix). Mirrors applyVoiceTrigger
// and silently no-ops when nothing matches. Media kinds are checked in
// assetTriggerOrder; the first hit wins.
func (s *Session) applyAssetTrigger(info types.MessageInfo, body string) {
	defer func() {
		if r := recover(); r != nil {
			// best-effort: never break message handling
		}
	}()
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}
	name, ok := goldcmds.AssetTriggerMatch(body)
	if !ok {
		return
	}
	br := &bridge{s: s}
	// One name can hold several kinds at once (a photo AND a video AND a
	// sticker AND a text AND a circle AND a voice). Send EVERY kind the owner
	// saved under this name, each as its own message, so nothing shadows
	// anything else and deleting one kind never lets another take its place.
	kinds := goldcmds.AssetKindsToSend(name, func(kind string) bool {
		data, _, _, found := br.GetCustomAssetMeta(kind, name)
		return found && len(data) > 0
	})
	for _, kind := range kinds {
		data, mime, meta, found := br.GetCustomAssetMeta(kind, name)
		if !found || len(data) == 0 {
			continue
		}
		switch kind {
		case "img":
			_ = br.SendImage(info, data, "")
		case "video":
			_ = br.SendVideo(info, data, "", nil, 0, 0, 0)
		case "sticker":
			_ = br.SendSticker(info, data)
		case "circle":
			f, err := os.CreateTemp("", "goldmd-circle-*"+extFromAssetMime(mime))
			if err != nil {
				continue
			}
			path := f.Name()
			if _, werr := f.Write(data); werr != nil {
				f.Close()
				os.Remove(path)
				continue
			}
			f.Close()
			secs, w, h := parseAssetMeta(meta)
			_ = br.SendCircleVideoFile(info, path, secs, w, h, nil)
			os.Remove(path)
		case "text":
			// Mirror the autoreply pipeline: send the saved text, then after a
			// short pause EDIT that same message to the same text (the edit is
			// what marks the message as "Edited", same as arRunReplyJob does).
			sendAssetTextEdited(br, info, string(data), assetEditDelay)
		}
		// Keep the send order deterministic: WhatsApp can reorder messages that
		// are uploaded in parallel, and the owner wants img→video→…→text.
		time.Sleep(400 * time.Millisecond)
	}
}

// extFromAssetMime maps the common asset mimetypes to a file extension.
func extFromAssetMime(mime string) string {
	switch {
	case strings.Contains(mime, "webm"):
		return ".webm"
	case strings.Contains(mime, "3gpp"):
		return ".3gp"
	case strings.Contains(mime, "quicktime"):
		return ".mov"
	default:
		return ".mp4"
	}
}

// parseAssetMeta decodes the "seconds,width,height" circle meta sidecar.
// Missing/garbled values fall back to zeros (WhatsApp still plays the clip).
func parseAssetMeta(meta string) (uint32, uint32, uint32) {
	parts := strings.Split(strings.TrimSpace(meta), ",")
	if len(parts) != 3 {
		return 0, 0, 0
	}
	var vals [3]uint32
	for i, p := range parts {
		if n, err := strconv.ParseUint(strings.TrimSpace(p), 10, 32); err == nil {
			vals[i] = uint32(n)
		}
	}
	return vals[0], vals[1], vals[2]
}
