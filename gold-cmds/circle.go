package goldcmds

// ============================================================================
// GOLD-MD — .circle  (video -> WhatsApp CIRCLE video / video note)
//
// WhatsApp renders a circle ("video note" / PTV) as a 1:1 rounded clip. The
// client sends it as Message.PtvMessage with a SQUARE mp4, so a normal 16:9
// video must first be centre-cropped to a square and re-encoded as
// h264+aac+faststart, or WhatsApp refuses to show the circle.
//
//   .circle            reply to a video       -> converted + sent as a circle
//   .circle <url>      direct video link      -> converted + sent as a circle
//
// Public command (any group member can make a circle), mirroring the other
// media converters (.sticker/.takevid). Heavy ffmpeg work runs in a goroutine
// and honours the per-user command watchdog context.
// ============================================================================

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// circleSide is the square edge (px) of the generated circle video. WhatsApp
// video notes are small; 480 keeps the file tiny without looking blurry.
const circleSide = 480

// circleMaxSeconds caps the clip at WhatsApp's 1-minute video-note limit.
const circleMaxSeconds = 60

// writeAssetTemp writes bytes to a temp file with the given extension and
// returns its path ("" on failure).
func writeAssetTemp(data []byte, ext string) string {
	f, err := os.CreateTemp("", "goldmd-asset-*"+ext)
	if err != nil {
		return ""
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return ""
	}
	f.Close()
	return f.Name()
}

// makeCircleMP4 converts any video at inPath into a SQUARE WhatsApp-ready mp4
// and returns the output path (caller removes it). It first normalises the
// source to h264 (whatsappifyVideo) and then centre-crops + scales to a square
// with faststart, so the moov atom is at the front for instant playback.
func makeCircleMP4(ctx context.Context, inPath string) (string, error) {
	if !isFfmpegAvailable() {
		return "", fmt.Errorf("ffmpeg not available")
	}
	src, err := whatsappifyVideo(ctx, inPath)
	if err != nil || src == "" {
		src = inPath
	}
	if src != inPath {
		defer os.Remove(src) // whatsappify produced a temp transcode
	}

	out, err := os.CreateTemp("", "goldcircle-*.mp4")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	out.Close()

	// crop=<min side>:<min side> centres the crop on the source frame, then
	// scale to a fixed square so WhatsApp always gets a true 1:1 clip.
	filter := fmt.Sprintf("crop='min(iw,ih)':'min(iw,ih)',scale=%d:%d", circleSide, circleSide)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", src,
		"-t", strconv.Itoa(circleMaxSeconds),
		"-vf", filter,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "26",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart",
		outPath)
	if err := cmd.Run(); err != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("circle convert failed: %w", err)
	}
	if st, serr := os.Stat(outPath); serr != nil || st.Size() == 0 {
		os.Remove(outPath)
		return "", fmt.Errorf("empty circle output")
	}
	return outPath, nil
}

// headerTransport stamps a browser User-Agent (and optional Referer) on every
// outgoing request — qu.ax/catbox refuse a bare Go agent.
type headerTransport struct {
	base    http.RoundTripper
	referer string
	ua      string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.ua != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", t.ua)
	}
	if t.referer != "" && req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", t.referer)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// circleProbe returns duration/width/height for a finished circle file.
func circleProbe(path string) (uint32, uint32, uint32) {
	if secs, w, h := probeVideoMeta(path); secs > 0 {
		return secs, w, h
	}
	return 0, circleSide, circleSide
}

// circleFromQuoted resolves the video the user replied to (or a URL argument),
// converts it to a circle, and returns the resulting file path.
func circleFromQuoted(s SessionBridge, info types.MessageInfo, urlArg string) (string, error) {
	var srcPath string

	if urlArg != "" {
		// Streamed straight to disk (never fully buffered in RAM) with the
		// browser UA + Referer the hosts expect.
		client := mediaHTTPClient()
		if ref := MediaReferer(urlArg); ref != "" {
			client = &http.Client{
				Timeout: 5 * time.Minute,
				Transport: &headerTransport{
					base:    http.DefaultTransport,
					referer: ref,
					ua:      BrowserUA,
				},
			}
		}
		p, err := streamDownloadToFile(context.Background(), client, urlArg, nil)
		if err != nil {
			return "", err
		}
		srcPath = p
	} else {
		data, mime, ok := s.DownloadQuotedMedia(info)
		if !ok || len(data) == 0 {
			return "", fmt.Errorf("no quoted media")
		}
		if mime != "" && !strings.Contains(strings.ToLower(mime), "video") {
			return "", fmt.Errorf("not a video")
		}
		srcPath = writeAssetTemp(data, ".mp4")
	}
	if srcPath == "" {
		return "", fmt.Errorf("temp write failed")
	}
	defer os.Remove(srcPath)

	ctx, release := s.BeginGuard(info.Sender.String())
	defer release()
	return makeCircleMP4(ctx, srcPath)
}

// circleTempTTL is how long a transient circle notice (progress, error,
// add-saved) stays before the bot deletes it, keeping the chat clean.
const circleTempTTL = 10 * time.Second

// circleHelpText is the .circle guidance block (shown when no video/URL is
// given, and as the error notice). Deliberately English.
func circleHelpText(prefix string) string {
	ex := examplePrefix(prefix)
	return "*🔰 CIRCLE COMMAND INFO 🔰*\n\n" +
		"*CONVERTS ANY VIDEO INTO A WHATSAPP CIRCLE VIDEO*\n\n" +
		"*OPTION 1:*\n" +
		"*REPLY TO A VIDEO AND TYPE:*\n" +
		"*❮ " + ex + "CIRCLE ❯*\n\n" +
		"*OPTION 2:*\n" +
		"*GIVE A DIRECT VIDEO LINK:*\n" +
		"*❮ " + ex + "CIRCLE <URL> ❯*\n\n" +
		"*THE BOT CROPS THE VIDEO TO A SQUARE AND SENDS IT AS A WHATSAPP CIRCLE.*"
}

// addCircleHelpText is the .addcircle guidance block. It shares the .add*
// layout (save / list / del) and appends the circle-specific note.
func addCircleHelpText(prefix string) string {
	ex := examplePrefix(prefix)
	return assetGroupGuidance(prefix, assetCircle,
		"*A SAMPLE CIRCLE IS SENT RIGHT AFTER SAVING SO YOU CAN VERIFY IT.*\n\n"+
			"*TYPE ❮ "+ex+"CIRCLE ❯ FOR INFO*")
}

// circleWaitText renders the live progress notice.
func circleWaitText(verb, percent string) string {
	return "*🔰 " + verb + "...*\n*PROCESSING: " + percent + "%*"
}

// replyTemporary sends a notice and auto-deletes it after ttl. Used for
// progress / error / save notices so they never clutter the chat.
func replyTemporary(s SessionBridge, info types.MessageInfo, text string, ttl time.Duration) {
	id := s.ReplyWithID(info, text)
	if id == "" {
		return
	}
	time.AfterFunc(ttl, func() { _ = s.DeleteMessage(info, id) })
}

// startCircleProgress sends the progress notice and returns a stop func that
// deletes it. The notice updates every 700ms up to 90%.
func startCircleProgress(s SessionBridge, info types.MessageInfo, verb string) func() {
	id := s.ReplyWithID(info, circleWaitText(verb, "00"))
	stop := make(chan struct{})
	go func() {
		percent := 0
		ticker := time.NewTicker(700 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if percent >= 90 {
					continue
				}
				percent += 9
				if percent > 90 {
					percent = 90
				}
				s.EditMessage(info, id, circleWaitText(verb, fmt.Sprintf("%02d", percent)))
			}
		}
	}()
	return func() {
		close(stop)
		_ = s.DeleteMessage(info, id)
	}
}

// circleURLArg returns the direct video URL given as an argument, or "".
func circleURLArg(args []string) string {
	u := strings.TrimSpace(strings.Join(args, " "))
	if u == "" || !strings.HasPrefix(strings.ToLower(u), "http") {
		return ""
	}
	return u
}

// handleCircleAsync is the .circle body: convert then send.
func handleCircleAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	urlArg := circleURLArg(args)
	// Nothing to convert: a bare .circle only ever shows the guidance, no
	// progress notice is created (so nothing flickers/deletes).
	if urlArg == "" && !VVHasQuotedMedia(s, info) {
		s.Reply(info, circleHelpText(prefix))
		return
	}

	stopProgress := startCircleProgress(s, info, "MAKING YOUR CIRCLE VIDEO")
	outPath, err := circleFromQuoted(s, info, urlArg)
	stopProgress()

	if err != nil || outPath == "" {
		replyTemporary(s, info, circleHelpText(prefix), circleTempTTL)
		return
	}
	defer os.Remove(outPath)

	seconds, w, h := circleProbe(outPath)
	thumb := BotVideoThumbnail(outPath)
	_ = s.SendCircleVideoFile(info, outPath, seconds, w, h, thumb)
}

// handleAddCircleAsync saves a video as a reusable circle asset and sends a
// sample so the owner can verify it immediately.
func handleAddCircleAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !assetOwnerGate(s, info) {
		return
	}
	sub, rest := addSubcommand(args)
	switch sub {
	case addSubList:
		listAssets(s, info, prefix, assetCircle)
		return
	case addSubDel:
		delAsset(s, info, rest, prefix, assetCircle)
		return
	}

	name := assetNameArg(rest)
	// No name: show the guidance directly (no progress notice to delete).
	if name == "" {
		s.Reply(info, addCircleHelpText(prefix))
		return
	}
	// Name given but no video attached: concise guidance, auto-deleted.
	if !VVHasQuotedMedia(s, info) {
		replyTemporary(s, info, addCircleHelpText(prefix), circleTempTTL)
		return
	}

	stopProgress := startCircleProgress(s, info, "SAVING YOUR CIRCLE VIDEO")
	outPath, err := circleFromQuoted(s, info, "")
	stopProgress()

	if err != nil || outPath == "" {
		replyTemporary(s, info, addCircleHelpText(prefix), circleTempTTL)
		return
	}
	defer os.Remove(outPath)

	data, rerr := os.ReadFile(outPath)
	if rerr != nil || len(data) == 0 {
		replyTemporary(s, info, "*🔰 FAILED TO SAVE THIS CIRCLE — TRY AGAIN*", circleTempTTL)
		return
	}
	seconds, w, h := circleProbe(outPath)
	meta := fmt.Sprintf("%d,%d,%d", seconds, w, h)
	if !s.SaveCustomAssetMeta("circle", name, data, "video/mp4", meta) {
		replyTemporary(s, info, "*🔰 FAILED TO SAVE THIS CIRCLE — TRY AGAIN*", circleTempTTL)
		return
	}

	replyTemporary(s, info, fmt.Sprintf(
		"*🔰 ADDCIRCLE SAVED*\n\n*NAME : ❮ %s ❱*\n\n*WRITE THAT NAME ANYTIME TO SEND THIS CIRCLE.*\n\n*SENDING SAMPLE CIRCLE...*",
		assetDisplayName(name)), circleTempTTL)

	thumb := BotVideoThumbnail(outPath)
	_ = s.SendCircleVideoFile(info, outPath, seconds, w, h, thumb)
}

func handleCircle(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleCircleAsync(s, info, args, prefix)
}

func init() {
	Register(Command{Name: "circle", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT ANY VIDEO INTO A WHATSAPP CIRCLE VIDEO. REPLY TO A VIDEO AND USE THIS COMMAND, OR GIVE A DIRECT VIDEO LINK.", Run: handleCircle})
	for _, a := range []string{"ptv", "videonote", "circlevideo", "mkcirlce", "makecircle"} {
		Register(Command{Name: a, Hidden: true, Run: handleCircle})
	}
}
