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

// handleCircleAsync is the .circle body: convert then send.
func handleCircleAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	urlArg := strings.TrimSpace(strings.Join(args, " "))
	if urlArg != "" && !strings.HasPrefix(strings.ToLower(urlArg), "http") {
		urlArg = ""
	}

	waitID := s.ReplyWithID(info, "*🔰 CIRCLE VIDEO BAN RAHI HAI...*\n*PROCESSING: 00%*")
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
				s.EditMessage(info, waitID, fmt.Sprintf("*CIRCLE VIDEO BAN RAHI HAI...*\n*PROCESSING: %02d%%*", percent))
			}
		}
	}()

	outPath, err := circleFromQuoted(s, info, urlArg)
	close(stop)
	s.DeleteMessage(info, waitID)

	if err != nil || outPath == "" {
		ex := examplePrefix(prefix)
		s.Reply(info, fmt.Sprintf(
			"*🔰 CIRCLE INFO 🔰*\n\n*QUOTE A VIDEO AND WRITE:*\n*TYPE ❰ %sCIRCLE ❱*\n\n*YA DIRECT VIDEO LINK DO:*\n*TYPE ❰ %sCIRCLE <URL> ❱*\n\n*BOT US VIDEO KO WHATSAPP CIRCLE SHAKAL ME BANA KAR BHEJ DE GA.*",
			ex, ex))
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
	name := assetNameArg(args)
	ex := examplePrefix(prefix)
	if name == "" {
		s.Reply(info, fmt.Sprintf(
			"*🔰 ADDCIRCLE INFO 🔰*\n\n*QUOTE A VIDEO AND WRITE:*\n*TYPE ❰ %sADDCIRCLE <NAME> ❱*\n\n*EXAMPLE:*\n*TYPE ❰ %sADDCIRCLE MYNAME ❱*\n\n*AFTER SAVING, WHENEVER ANYONE WRITES THAT NAME THE CIRCLE VIDEO WILL BE SENT AUTOMATICALLY.*\n\n*TYPE ❮ %sCIRCLE ❯ FOR INFO*",
			ex, ex, ex))
		return
	}

	waitID := s.ReplyWithID(info, "*🔰 CIRCLE VIDEO SAVE HO RAHI HAI...*\n*PROCESSING: 00%*")
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
				s.EditMessage(info, waitID, fmt.Sprintf("*CIRCLE VIDEO SAVE HO RAHI HAI...*\n*PROCESSING: %02d%%*", percent))
			}
		}
	}()

	outPath, err := circleFromQuoted(s, info, "")
	close(stop)
	s.DeleteMessage(info, waitID)

	if err != nil || outPath == "" {
		s.Reply(info, "*🔰 QUOTE A VIDEO FIRST THEN WRITE .ADDCIRCLE <NAME>*\n\n*YA SIRF VIDEO BHEJO AUR US PAR REPLY KARO.*")
		return
	}
	defer os.Remove(outPath)

	data, rerr := os.ReadFile(outPath)
	if rerr != nil || len(data) == 0 {
		s.Reply(info, "*🔰 FAILED TO SAVE — TRY AGAIN*")
		return
	}
	seconds, w, h := circleProbe(outPath)
	meta := fmt.Sprintf("%d,%d,%d", seconds, w, h)
	if !s.SaveCustomAssetMeta("circle", name, data, "video/mp4", meta) {
		s.Reply(info, "*🔰 FAILED TO SAVE — TRY AGAIN*")
		return
	}

	s.Reply(info, fmt.Sprintf(
		"*🔰 ADDCIRCLE SAVED SUCCESSFULLY*\n\n*NAME :❰ %s ❱*\n\n*NOW WHENEVER ANYONE WRITES* *%s* *THIS CIRCLE VIDEO WILL BE SENT AUTOMATICALLY 🔰*\n\n*SENDING SAMPLE CIRCLE...*\n\n*TYPE ❮ %sCIRCLE ❯ FOR INFO*",
		assetDisplayName(name), assetDisplayName(name), ex))

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
