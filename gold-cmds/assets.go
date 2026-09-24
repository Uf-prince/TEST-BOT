package goldcmds

// ============================================================================
// GOLD-MD — .addimg / .addvideo / .addsticker / .addtext / .addcircle
//
// Same model as .addvoice: reply to a media (or send text) and the asset is
// saved under a NAME for this bot. Later, whenever anyone writes that name,
// the asset is auto-sent. Each family also gets .del<kind> and .<kind>list.
//
//   .addimg <name>      reply to a PHOTO          (aliases: addimage, saveimg)
//   .addvideo <name>    reply to a VIDEO          (aliases: addvid, savevideo)
//   .addsticker <name>  reply to a STICKER        (aliases: addstkr, savesticker)
//   .addtext <name>     reply to any TEXT         (aliases: addtxt, savetext)
//   .addcircle <name>   reply to a VIDEO — saved  (aliases: addptv, savecircle)
//                       AND sent as a sampled circle video so the owner can
//                       verify it immediately.
//
// Owner-only. Stored under <DataDir>/assets/<botJID>/<kind>/ with a Redis name
// index. The trigger (auto-send on name mention) lives in src/handler.go.
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// assetSpec describes one family of .add* commands.
type assetSpec struct {
	kind     string // storage kind (img/video/sticker/text/circle)
	verb     string // command name without the prefix, e.g. "addimg"
	label    string // display label used in replies, e.g. "IMAGE"
	expect   string // what the user must quote, e.g. "A PHOTO"
	mimeHint string // mime used when the host message has none
}

var (
	assetImg     = assetSpec{"img", "addimg", "IMAGE", "A PHOTO", "image/jpeg"}
	assetVideo   = assetSpec{"video", "addvideo", "VIDEO", "A VIDEO", "video/mp4"}
	assetSticker = assetSpec{"sticker", "addsticker", "STICKER", "A STICKER", "image/webp"}
	assetText    = assetSpec{"text", "addtext", "TEXT", "A TEXT MESSAGE", "text/plain"}
	assetCircle  = assetSpec{"circle", "addcircle", "CIRCLE VIDEO", "A VIDEO", "video/mp4"}
)

// assetOwnerGate — same rule as .addvoice (owner only, DM or group).
func assetOwnerGate(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
	return false
}

// assetNameArg joins the args into a single asset name ("my car" stays one
// name). Returns "" when nothing was supplied.
func assetNameArg(args []string) string {
	return strings.TrimSpace(strings.Join(args, " "))
}

// assetDisplayName is the uppercased name shown in replies.
func assetDisplayName(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

// prepareAssetData validates and normalises a downloaded media before it is
// stored, so a saved asset always plays back correctly later:
//   - video/circle: transcoded to WhatsApp-ready h264+aac+faststart mp4
//   - sticker:      any image/video is converted to a webp sticker
//   - img:          a sticker reply is converted back to a still image
//
// Returns ("",false) when the media is the wrong family for the command.
func prepareAssetData(kind string, data []byte, mime, fallbackMime string) ([]byte, string, bool) {
	if len(data) == 0 {
		return nil, "", false
	}
	if mime == "" {
		mime = fallbackMime
	}
	lower := strings.ToLower(mime)

	switch kind {
	case "text":
		return data, "text/plain", true
	case "sticker":
		if strings.Contains(lower, "webp") {
			return data, "image/webp", true
		}
		if !strings.Contains(lower, "image") && !strings.Contains(lower, "video") {
			return nil, "", false
		}
		if out, ok := convertToStickerBytes(data, mime); ok {
			return out, "image/webp", true
		}
		return data, mime, strings.Contains(lower, "image")
	case "img":
		if strings.Contains(lower, "webp") {
			if out, ok := convertStickerToImageBytes(data); ok {
				return out, "image/jpeg", true
			}
		}
		if !strings.Contains(lower, "image") {
			return nil, "", false
		}
		return data, mime, true
	case "video", "circle":
		if !strings.Contains(lower, "video") {
			return nil, "", false
		}
		// Best effort: keep the original bytes when ffmpeg is missing so the
		// save still succeeds.
		if norm, ok := normaliseVideoBytes(data); ok {
			data = norm
		}
		return data, "video/mp4", true
	}
	return data, mime, true
}

// normaliseVideoBytes transcodes raw video bytes to a WhatsApp-ready mp4.
// Returns (nil,false) when ffmpeg is unavailable or the transcode fails.
func normaliseVideoBytes(data []byte) ([]byte, bool) {
	if !isFfmpegAvailable() {
		return nil, false
	}
	p, err := writeTempMedia(data, ".mp4")
	if err != nil {
		return nil, false
	}
	defer os.Remove(p)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := whatsappifyVideo(ctx, p)
	if err != nil || out == "" {
		return nil, false
	}
	if out != p {
		defer os.Remove(out)
	}
	b, rerr := os.ReadFile(out)
	if rerr != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// convertToStickerBytes turns image/video bytes into a webp sticker.
func convertToStickerBytes(data []byte, mime string) ([]byte, bool) {
	if !isFfmpegAvailable() {
		return nil, false
	}
	ext := extForMime(mime)
	p, err := writeTempMedia(data, ext)
	if err != nil {
		return nil, false
	}
	defer os.Remove(p)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var out string
	if isVideoMime(mime) {
		out, err = ffmpegVideoToSticker(ctx, p)
	} else {
		out, err = ffmpegImageToSticker(ctx, p, ext)
	}
	if err != nil || out == "" {
		return nil, false
	}
	defer os.Remove(out)
	b, rerr := os.ReadFile(out)
	if rerr != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// convertStickerToImageBytes turns a webp sticker into a still JPEG.
func convertStickerToImageBytes(data []byte) ([]byte, bool) {
	if !isFfmpegAvailable() {
		return nil, false
	}
	p, err := writeTempMedia(data, ".webp")
	if err != nil {
		return nil, false
	}
	defer os.Remove(p)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := ffmpegStickerToImage(ctx, p)
	if err != nil || out == "" {
		return nil, false
	}
	defer os.Remove(out)
	b, rerr := os.ReadFile(out)
	if rerr != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// addAsset is the shared implementation for the media-backed families.
func addAsset(s SessionBridge, info types.MessageInfo, args []string, prefix string, spec assetSpec) {
	if !assetOwnerGate(s, info) {
		return
	}

	name := assetNameArg(args)
	if name == "" {
		ex := examplePrefix(prefix)
		s.Reply(info, fmt.Sprintf(
			"*🔰 %s INFO 🔰*\n\n*QUOTE %s AND WRITE:*\n*TYPE ❰ %s%s <NAME> ❱*\n\n*EXAMPLE:*\n*TYPE ❰ %s%s MYNAME ❱*\n\n*AFTER SAVING, WHENEVER ANYONE WRITES THAT NAME THE %s WILL BE SENT AUTOMATICALLY.*",
			strings.ToUpper(spec.verb), spec.expect, ex, strings.ToUpper(spec.verb), ex, strings.ToUpper(spec.verb), spec.label))
		return
	}

	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, fmt.Sprintf("*🔰 QUOTE %s FIRST THEN WRITE .%s <NAME>*", spec.expect, strings.ToUpper(spec.verb)))
		return
	}
	prepared, outMime, ok := prepareAssetData(spec.kind, data, mime, spec.mimeHint)
	if !ok || len(prepared) == 0 {
		s.Reply(info, fmt.Sprintf("*🔰 QUOTE %s FIRST THEN WRITE .%s <NAME>*", spec.expect, strings.ToUpper(spec.verb)))
		return
	}
	data, mime = prepared, outMime

	if !s.SaveCustomAsset(spec.kind, name, data, mime) {
		s.Reply(info, "*🔰 FAILED TO SAVE — TRY AGAIN*")
		return
	}

	note := ""
	if spec.kind == "circle" {
		// The owner asked for an immediate sample so they can verify the circle
		// actually renders before relying on it later.
		note = "\n\n*SENDING SAMPLE CIRCLE...*"
	}
	s.Reply(info, fmt.Sprintf(
		"*🔰 %s SAVED SUCCESSFULLY*\n\n*NAME :❰ %s ❱*\n\n*NOW WHENEVER ANYONE WRITES* *%s* *THIS %s WILL BE SENT AUTOMATICALLY 🔰*%s",
		strings.ToUpper(spec.verb), assetDisplayName(name), assetDisplayName(name), spec.label, note))
}

// addText saves a quoted text message under a name.
func addText(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !assetOwnerGate(s, info) {
		return
	}
	name := assetNameArg(args)
	if name == "" {
		ex := examplePrefix(prefix)
		s.Reply(info, fmt.Sprintf(
			"*🔰 ADDTEXT INFO 🔰*\n\n*QUOTE A TEXT MESSAGE AND WRITE:*\n*TYPE ❰ %sADDTEXT <NAME> ❱*\n\n*EXAMPLE:*\n*TYPE ❰ %sADDTEXT HELLO ❱*\n\n*AFTER SAVING, WHENEVER ANYONE WRITES THAT NAME THE TEXT WILL BE SENT AUTOMATICALLY.*",
			ex, ex))
		return
	}

	body := s.GetQuotedMessageText(info)
	if strings.TrimSpace(body) == "" {
		s.Reply(info, "*🔰 QUOTE A TEXT MESSAGE FIRST THEN WRITE .ADDTEXT <NAME>*")
		return
	}

	if !s.SaveCustomAsset("text", name, []byte(body), "text/plain") {
		s.Reply(info, "*🔰 FAILED TO SAVE — TRY AGAIN*")
		return
	}
	s.Reply(info, fmt.Sprintf(
		"*🔰 ADDTEXT SAVED SUCCESSFULLY*\n\n*NAME :❰ %s ❱*\n\n*NOW WHENEVER ANYONE WRITES* *%s* *THIS TEXT WILL BE SENT AUTOMATICALLY 🔰*",
		assetDisplayName(name), assetDisplayName(name)))
}

// delAsset removes a saved asset by name. Accepts the name directly or, when
// omitted, the list NUMBER shown by .<kind>list.
func delAsset(s SessionBridge, info types.MessageInfo, args []string, prefix string, spec assetSpec) {
	if !assetOwnerGate(s, info) {
		return
	}
	name := assetNameArg(args)
	if name == "" {
		s.Reply(info, fmt.Sprintf("*WRITE THE %s NAME TO DELETE\nEXAMPLE: .DEL%s MYNAME*", spec.label, strings.ToUpper(spec.verb[3:])))
		return
	}
	// number shortcut: .delimg 2
	if n, err := strconv.Atoi(name); err == nil && n >= 1 {
		names := s.ListCustomAssets(spec.kind)
		if n <= len(names) {
			name = names[n-1]
		}
	}

	if s.DeleteCustomAsset(spec.kind, name) {
		s.Reply(info, fmt.Sprintf("*🔰 %s DELETED*\n\n*NAME :❰ %s ❱*", spec.label, assetDisplayName(name)))
	} else {
		s.Reply(info, fmt.Sprintf("*🔰 %s \"%s\" NOT FOUND*", spec.label, assetDisplayName(name)))
	}
}

// listAssets shows every saved asset of a kind.
func listAssets(s SessionBridge, info types.MessageInfo, prefix string, spec assetSpec) {
	names := s.ListCustomAssets(spec.kind)
	if len(names) == 0 {
		s.Reply(info, fmt.Sprintf("*NO %sS SAVED YET*\n*Use .%s <NAME> to save one*", spec.label, strings.ToUpper(spec.verb)))
		return
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*🔰 SAVED %sS 🔰*\n\n", spec.label))
	for i, n := range names {
		sb.WriteString(fmt.Sprintf("*%d. %s*\n", i+1, assetDisplayName(n)))
	}
	sb.WriteString(fmt.Sprintf("\n*Total: %d %s(s)*", len(names), strings.ToLower(spec.label)))
	s.Reply(info, sb.String())
}

// ---------------------------------------------------------------------------
// AssetTriggerMatch — same shape as VoiceTriggerMatch: a bare word (no prefix,
// <=50 chars, single token) can name a saved asset. The caller
// (src/handler.go) resolves the name against each asset kind.
// ---------------------------------------------------------------------------

// AssetTriggerOrder is the kind lookup order for auto-send. Media kinds come
// before text so a name shared by, say, a photo and a text sends the photo.
// Exported so src.applyAssetTrigger and the tests share one source of truth.
var AssetTriggerOrder = []string{"img", "video", "sticker", "circle", "text"}

func AssetTriggerMatch(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || len(trimmed) > 50 {
		return "", false
	}
	if strings.ContainsAny(trimmed, " \t\n") {
		return "", false
	}
	if trimmed[0] == '.' || trimmed[0] == '!' || trimmed[0] == '/' || trimmed[0] == '#' {
		return "", false
	}
	return strings.ToLower(trimmed), true
}

// ---------------------------------------------------------------------------
// handlers
// ---------------------------------------------------------------------------

func handleAddImg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go addAsset(s, info, args, prefix, assetImg)
}
func handleAddVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go addAsset(s, info, args, prefix, assetVideo)
}
func handleAddSticker(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go addAsset(s, info, args, prefix, assetSticker)
}
func handleAddCircle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAddCircleAsync(s, info, args, prefix)
}
func handleAddText(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go addText(s, info, args, prefix)
}

func handleDelImg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go delAsset(s, info, args, prefix, assetImg)
}
func handleDelVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go delAsset(s, info, args, prefix, assetVideo)
}
func handleDelSticker(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go delAsset(s, info, args, prefix, assetSticker)
}
func handleDelText(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go delAsset(s, info, args, prefix, assetText)
}
func handleDelCircle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go delAsset(s, info, args, prefix, assetCircle)
}

func handleListImg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go listAssets(s, info, prefix, assetImg)
}
func handleListVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go listAssets(s, info, prefix, assetVideo)
}
func handleListSticker(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go listAssets(s, info, prefix, assetSticker)
}
func handleListText(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go listAssets(s, info, prefix, assetText)
}
func handleListCircle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go listAssets(s, info, prefix, assetCircle)
}

func init() {
	Register(Command{Name: "addimg", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SAVE A PHOTO UNDER A NAME. REPLY TO A PHOTO AND USE THIS COMMAND. LATER WRITE THAT NAME TO SEND THE PHOTO.", OwnerOnly: true, Run: handleAddImg})
	Register(Command{Name: "addvideo", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SAVE A VIDEO UNDER A NAME. REPLY TO A VIDEO AND USE THIS COMMAND. LATER WRITE THAT NAME TO SEND THE VIDEO.", OwnerOnly: true, Run: handleAddVideo})
	Register(Command{Name: "addsticker", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SAVE A STICKER UNDER A NAME. REPLY TO A STICKER AND USE THIS COMMAND. LATER WRITE THAT NAME TO SEND THE STICKER.", OwnerOnly: true, Run: handleAddSticker})
	Register(Command{Name: "addtext", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SAVE A TEXT MESSAGE UNDER A NAME. REPLY TO ANY TEXT AND USE THIS COMMAND. LATER WRITE THAT NAME TO SEND THE TEXT.", OwnerOnly: true, Run: handleAddText})
	Register(Command{Name: "addcircle", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SAVE A VIDEO AS A WHATSAPP CIRCLE VIDEO. REPLY TO A VIDEO AND USE THIS COMMAND. LATER WRITE THAT NAME TO SEND THE CIRCLE.", OwnerOnly: true, Run: handleAddCircle})

	Register(Command{Name: "delimg", Category: "AI & MEDIA", Desc: "THIS COMMAND DELETES A SAVED PHOTO BY NAME OR BY ITS NUMBER IN THE IMAGE LIST.", OwnerOnly: true, Run: handleDelImg})
	Register(Command{Name: "delvideo", Category: "AI & MEDIA", Desc: "THIS COMMAND DELETES A SAVED VIDEO BY NAME OR BY ITS NUMBER IN THE VIDEO LIST.", OwnerOnly: true, Run: handleDelVideo})
	Register(Command{Name: "delsticker", Category: "AI & MEDIA", Desc: "THIS COMMAND DELETES A SAVED STICKER BY NAME OR BY ITS NUMBER IN THE STICKER LIST.", OwnerOnly: true, Run: handleDelSticker})
	Register(Command{Name: "deltext", Category: "AI & MEDIA", Desc: "THIS COMMAND DELETES A SAVED TEXT BY NAME OR BY ITS NUMBER IN THE TEXT LIST.", OwnerOnly: true, Run: handleDelText})
	Register(Command{Name: "delcircle", Category: "AI & MEDIA", Desc: "THIS COMMAND DELETES A SAVED CIRCLE VIDEO BY NAME OR BY ITS NUMBER IN THE CIRCLE LIST.", OwnerOnly: true, Run: handleDelCircle})

	Register(Command{Name: "imglist", Category: "AI & MEDIA", Desc: "THIS COMMAND SHOWS ALL SAVED PHOTOS OF THE BOT WITH THEIR NUMBERS.", OwnerOnly: true, Run: handleListImg})
	Register(Command{Name: "videolist", Category: "AI & MEDIA", Desc: "THIS COMMAND SHOWS ALL SAVED VIDEOS OF THE BOT WITH THEIR NUMBERS.", OwnerOnly: true, Run: handleListVideo})
	Register(Command{Name: "stickerlist", Category: "AI & MEDIA", Desc: "THIS COMMAND SHOWS ALL SAVED STICKERS OF THE BOT WITH THEIR NUMBERS.", OwnerOnly: true, Run: handleListSticker})
	Register(Command{Name: "textlist", Category: "AI & MEDIA", Desc: "THIS COMMAND SHOWS ALL SAVED TEXTS OF THE BOT WITH THEIR NUMBERS.", OwnerOnly: true, Run: handleListText})
	Register(Command{Name: "circlelist", Category: "AI & MEDIA", Desc: "THIS COMMAND SHOWS ALL SAVED CIRCLE VIDEOS OF THE BOT WITH THEIR NUMBERS.", OwnerOnly: true, Run: handleListCircle})

	// hidden aliases — same work, never shown in the menu
	for _, a := range []string{"addimage", "saveimg", "saveimage"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleAddImg})
	}
	for _, a := range []string{"addvid", "savevideo", "savevid"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleAddVideo})
	}
	for _, a := range []string{"addstkr", "savesticker", "savestkr"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleAddSticker})
	}
	for _, a := range []string{"addtxt", "savetext", "savetxt"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleAddText})
	}
	for _, a := range []string{"addptv", "savecircle", "addvideonote"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleAddCircle})
	}
	for _, a := range []string{"deleteimg", "removeimg", "delimage"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleDelImg})
	}
	for _, a := range []string{"deletevideo", "removevideo", "delvid"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleDelVideo})
	}
	for _, a := range []string{"deletesticker", "removesticker", "delstkr"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleDelSticker})
	}
	for _, a := range []string{"deletetext", "removetext", "deltxt"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleDelText})
	}
	for _, a := range []string{"deletecircle", "removecircle", "delptv"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleDelCircle})
	}
	for _, a := range []string{"imgs", "images", "imagelist", "listimg"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleListImg})
	}
	for _, a := range []string{"videos", "vidlist", "listvideo"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleListVideo})
	}
	for _, a := range []string{"stickers", "stkrlist", "liststicker"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleListSticker})
	}
	for _, a := range []string{"texts", "txtlist", "listtext"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleListText})
	}
	for _, a := range []string{"circles", "ptvlist", "listcircle"} {
		Register(Command{Name: a, OwnerOnly: true, Hidden: true, Run: handleListCircle})
	}
}
