package goldcmds

// ============================================================================
// GOLD-MD — YouTube Downloader Commands (Video + Audio)
// File: video-play.go
// ============================================================================
// Download pipeline (no third-party API keys):
//   1. DIRECT YouTube fetch (Innertube ANDROID_VR, ytdirect.go) — pre-signed
//      googlevideo URLs, usually under a second, no API/key needed.
//   2. loader.to fallback — handles music/age-gated videos the direct
//      client refuses (LOGIN_REQUIRED), polls progress until ready.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── Download API constants ──────────────────────────────────────────────────
const (
	// YouTube fallback API (loader.to) — used when the direct fetch is refused
	// (music/age-gated videos the Innertube client will not serve).
	// Flow: GET /ajax/download.php?format=360&url=<yt> -> progress_url -> poll
	// until download_url appears (usually 5-15s) -> direct MP4 (H.264 360p).
	ytLoaderAPI      = "https://loader.to/ajax/download.php"
	ytLoaderProgress = "https://lto2.affadaffa.com/api/progress"

	maxVideoMB      = 90
	maxVideoBytes   = maxVideoMB * 1024 * 1024
	videoMaxResults = 15

	// ── STORJ CONFIG ────────────────────────────────────────────────────────
	// Fill these values locally if Storj staging is enabled. Keep credentials
	// out of GitHub and never commit real access keys or secret keys.
	storjAccessKey = ""
	storjSecretKey = ""
	storjBucket    = ""
	storjEndpoint  = "https://gateway.storjshare.io"
)

// ── API response structs ────────────────────────────────────────────────────

// WSFastResponse is the shared download-result shape used by the direct
// YouTube fetch (ytdirect.go) and the loader.to fallback.
type WSFastResponse struct {
	Success  bool   `json:"success"`
	Creator  string `json:"creator"`
	Metadata struct {
		Title     string `json:"title"`
		Author    string `json:"author"`
		Views     int    `json:"views"`
		Likes     int    `json:"likes"`
		Comments  int    `json:"comments"`
		Duration  string `json:"duration"`
		Thumbnail string `json:"thumbnail"`
		URL       string `json:"url"`
	} `json:"metadata"`
	Result struct {
		VideoURL string `json:"video_url"`
		AudioURL string `json:"audio_url"`
	} `json:"result"`
}

// ============================================================================
// ▀▀   VIDEO COMMAND   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
// ============================================================================

// ── Command registration ────────────────────────────────────────────────────

func init() {
	// Main video command (visible in menu + count)
	Register(Command{Name: "video2", Category: "DOWNLOADER", Desc: "Download a YouTube video by name or URL", Run: handleVideo})
	// Aliases — fully functional but Hidden from menu + TOTAL COMMANDS count
	Register(Command{Name: "v", Hidden: true, Run: handleVideo2})
	Register(Command{Name: "ytvideo", Hidden: true, Run: handleVideo2})
	Register(Command{Name: "ytmp4", Hidden: true, Run: handleVideo2})
}

// handleVideo is the main entry point for the video command.
// Supports: direct YouTube URL, or search by name → number selection.
func handleVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutCmd(s, info, "VIDEO2", "VIDEO", func(ctx context.Context) {
		handleVideoAsync(ctx, s, info, args, prefix)
	})
}

func handleVideoAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	var pickedResult *VideoResult

	// Quick-pick: args may contain pre-parsed metadata from a previous session
	if len(args) >= 4 && (strings.Contains(args[0], "youtube.com/") || strings.Contains(args[0], "youtu.be/")) {
		pickedResult = &VideoResult{
			URL:       args[0],
			Thumbnail: args[1],
			Title:     args[2],
			Duration:  args[3],
		}
		downloadAndSend(ctx, s, info, args[0], pickedResult)
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO2 COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE*\n*TYPE SAME LIKE THAT*\n*%sVIDEO2 ❮ VIDEO NAME ❯*\n\n*EXAMPLE LIKE THIS*\n*%sVIDEO2 SHAPE OF YOU*\n\n*TYPE COMMAND + VIDEO NAME TO DOWNLOAD VIDEO FROM YOUTUBE*", prefix, prefix))
		return
	}

	// Direct YouTube URL → download immediately
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSend(ctx, s, info, input, nil)
		return
	}

	// Search by name
	searchProgress(ctx, s, info, input)
}

// searchProgress sends a single static "SEARCHING" message, searches YouTube,
// deletes the message, and sends the results list for number selection.
func searchProgress(ctx context.Context, s SessionBridge, info types.MessageInfo, query string) {
	// Single static message — NO progress edits, keeps server fast
	waitMsgID := s.ReplyWithID(info, "*SEARCHING ON YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	s.DeleteMessage(info, waitMsgID)

	if len(results) == 0 {
		videoCmdError(s, info)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*TOP %d RESULTS FOR YOUR SEARCH*\n *%s*\n\n", len(results), query))
	sb.WriteString("*FIRST CHECK THE WHOLE LIST AND TYPE ANY NUMBER 1, OR 6 OR 14 OR 2 OR OTHER ANY NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD FROM YOUTUBE*\n\n")

	for i, r := range results {
		durStr := r.Duration
		if durStr == "" {
			durStr = "NOT FOUND"
		}
		nameStr := r.Title
		if nameStr == "" {
			nameStr = "NULL"
		}
		linkStr := r.URL
		if linkStr == "" {
			linkStr = "NULL"
		}

		sb.WriteString(fmt.Sprintf("\n*✧═══════════•❁❀❁•═══════════✧*\n*TYPE ❰ %d ❱ TO DOWNLOAD THIS FROM YT*\n", i+1))
		sb.WriteString(fmt.Sprintf("%s\n", nameStr))
		sb.WriteString(fmt.Sprintf("%s\n", linkStr))
		sb.WriteString(fmt.Sprintf("*DURATION :❯ %s*\n*✧═══════════•❁❀❁•═══════════✧*\n\n", durStr))
		sb.WriteString("\n")
	}

	sb.WriteString("*TYPE NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO 15*")

	s.SetVideoSession(info.Sender.String(), results)
	s.Reply(info, sb.String())
}

// downloadAndSend is the complete video download pipeline:
// 1. Waiting message → 2. API call → 3. Metadata + thumbnail (held)
// 4. Video download + remux → 5. Delete waiting msg → 6. Send thumbnail+info
// 7. Send plain video
func downloadAndSend(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, pickedResult *VideoResult) {
	// Single static waiting message — NO progress edits, keeps server fast
	waitMsgID := s.ReplyWithID(info, "*DOWNLOADING VIDEOS FROM YOUTUBE.....*")

	clearWait := func() {
		if waitMsgID != "" {
			s.DeleteMessage(info, waitMsgID)
		}
	}

	// HTTP clients
	linkClient := &http.Client{Timeout: 30 * time.Second}
	dlClient := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			return nil
		},
	}

	// STEP 1: DIRECT YouTube fetch (Innertube ANDROID_VR, ytdirect.go) — fast path
	wsData, err := fetchYTDirect(ctx, ytDirectClient(), videoURL)
	if err != nil || wsData == nil || wsData.Result.VideoURL == "" {
		// Direct refused (music/age-gated) → loader.to fallback (360p MP4)
		wsData, err = ytLoaderToFallback(ctx, linkClient, videoURL, "360")
	}
	if err != nil || wsData == nil || wsData.Result.VideoURL == "" {
		clearWait()
		videoCmdError(s, info)
		return
	}

	// STEP 2: Prepare metadata (N/A if not available)
	title := "N/A"
	if pickedResult != nil && pickedResult.Title != "" {
		title = pickedResult.Title
	} else if wsData.Metadata.Title != "" {
		title = wsData.Metadata.Title
	}
	author := wsData.Metadata.Author
	if author == "" {
		author = "N/A"
	}
	duration := wsData.Metadata.Duration
	if duration == "" && pickedResult != nil {
		duration = pickedResult.Duration
	}
	if duration == "" {
		duration = "N/A"
	}
	views := formatMetadataNumber(wsData.Metadata.Views) // returns "N/A" if 0

	// Download thumbnail (prepare but DON'T send yet — hold until video download completes)
	thumbnailURL := wsData.Metadata.Thumbnail
	if pickedResult != nil && pickedResult.Thumbnail != "" {
		thumbnailURL = pickedResult.Thumbnail
	}
	thumbnail := downloadThumbnail(ctx, dlClient, thumbnailURL)

	// Prepare info caption (hold it — send after video download completes)
	infoCaption := fmt.Sprintf("*%s*\n\n🔰 *AUTHOR :❯ %s*\n🔰 *DURATION :❯ %s*\n🔰 *VIEWS :❯ %s*",
		strings.ToUpper(title), strings.ToUpper(author), strings.ToUpper(duration), strings.ToUpper(views))

	// STEP 3: Stream the H.264 video to a temporary file (bounded RAM).
	videoPath, dlErr := streamDownloadToFile(ctx, dlClient, wsData.Result.VideoURL, nil)
	if dlErr != nil {
		clearWait()
		videoCmdError(s, info)
		return
	}
	defer os.Remove(videoPath)

	// STEP 4: Fast remux — H.264 + AAC already WhatsApp-compatible, just +faststart.
	finalVideoPath, transErr := fastRemuxFile(ctx, videoPath)
	if transErr != nil {
		finalVideoPath = videoPath
	} else {
		defer os.Remove(finalVideoPath)
	}

	// STEP 5: Video download COMPLETE — delete waiting message
	clearWait()

	// STEP 6: NOW send thumbnail + info (after video download completes, before video send)
	if len(thumbnail) > 0 {
		if thumbErr := s.SendImage(info, thumbnail, infoCaption); thumbErr != nil {
		}
	} else {
		// No thumbnail available — send info as text message
		s.Reply(info, infoCaption)
	}

	// STEP 7: Send plain video (no caption, no footer)
	err = s.SendVideoFile(info, finalVideoPath, "", nil, 0, 0, 0)

	if err != nil {
		videoCmdError(s, info)
	} else {
	}
}

// ============================================================================
// ▀▀   AUDIO COMMAND   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
// ============================================================================

// ── Audio command registration ──────────────────────────────────────────────

func init() {
	// Main audio commands (visible in menu + count)
	Register(Command{Name: "play2", Category: "DOWNLOADER", Desc: "Play / download a song by name from YouTube", Run: handlePlay})
	Register(Command{Name: "song", Hidden: true, Run: handlePlay2}) // alias of play
	// Aliases — fully functional but Hidden from menu + TOTAL COMMANDS count
	Register(Command{Name: "ytaudio", Hidden: true, Run: handlePlay2})
	Register(Command{Name: "mp3", Hidden: true, Run: handlePlay2})
}

// handlePlay is the main entry point for the audio command.
// Supports: direct YouTube URL, or search by name → number selection.
func handlePlay(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutCmd(s, info, "PLAY2", "PLAY", func(ctx context.Context) {
		handlePlayAsync(ctx, s, info, args, prefix)
	})
}

func handlePlayAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	input := strings.TrimSpace(strings.Join(args, " "))
	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 PLAY2 COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD AUDIOS FROM YOUTUBE*\n*TYPE SAME LIKE THAT*\n*%sPLAY2 ❮ AUDIO NAME ❯*\n\n*EXAMPLE LIKE THIS*\n*PLAY2 SHAPE OF YOU*\n\n*TYPE COMMAND + AUDIO NAME TO DOWNLOAD AUDIO FROM YOUTUBE*", prefix))
		return
	}
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		sendAudio(ctx, s, info, input, nil)
		return
	}

	waitID := s.ReplyWithID(info, "*SEARCHING AUDIOS FROM YOUTUBE.....*")
	results := youtubeSearch(ctx, input)
	if waitID != "" {
		s.DeleteMessage(info, waitID)
	}
	if len(results) == 0 {
		playCmdError(s, info)
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*TOP %d RESULTS FOR YOUR SEARCH* \n *%s*\n\n", len(results), input))
	b.WriteString("*FIRST CHECK THE WHOLE LIST AND TYPE ANY NUMBER 1, OR 6 OR 14 OR 2 OR OTHER ANY NUMBER WHICH AUDIO DO YOU WANT TO DOWNLOAD FROM YOUTUBE*\n\n")
	for i, r := range results {
		durStr := r.Duration
		if durStr == "" {
			durStr = "NOT FOUND"
		}
		nameStr := r.Title
		if nameStr == "" {
			nameStr = "NULL"
		}
		linkStr := r.URL
		if linkStr == "" {
			linkStr = "NULL"
		}

		b.WriteString(fmt.Sprintf("\n*✧═══════════•❁❀❁•═══════════✧*\n*TYPE ❰ %d ❱ TO DOWNLOAD THIS FROM YT*\n", i+1))
		b.WriteString(fmt.Sprintf("%s\n", strings.ToUpper(nameStr)))
		b.WriteString(fmt.Sprintf("%s\n", linkStr))
		b.WriteString(fmt.Sprintf("*DURATION :❯ %s*\n*✧═══════════•❁❀❁•═══════════✧*\n\n", strings.ToUpper(durStr)))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("*TYPE NUMBER WHICH AUDIO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO 15*"))
	s.SetAudioSession(info.Sender.String(), results)
	s.Reply(info, b.String())
}

// sendAudio is the complete audio download pipeline:
// 1. Waiting message → 2. API call → 3. Metadata + thumbnail (held)
// 4. Audio download → 5. Delete waiting msg → 6. Send thumbnail+info
// 7. Send plain audio
func sendAudio(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, selected *VideoResult) {
	// sendAudio is already called from the asynchronous play handler.
	// Do not add a global semaphore here: other commands must remain usable.
	// Single static waiting message — NO progress edits, keeps server fast
	waitID := s.ReplyWithID(info, "*DOWNLOADING AUDIO FROM YOUTUBE.....*")

	clearWait := func() {
		if waitID != "" {
			s.DeleteMessage(info, waitID)
		}
	}

	client := &http.Client{Timeout: 120 * time.Second}
	// DIRECT YouTube fetch (Innertube ANDROID_VR, ytdirect.go) — fast path
	wsAudio, wsErr := fetchYTDirect(ctx, ytDirectClient(), videoURL)
	if wsErr != nil || wsAudio == nil || wsAudio.Result.AudioURL == "" {
		// Direct refused (music/age-gated) → loader.to fallback (mp3)
		wsAudio, wsErr = ytLoaderToFallback(ctx, client, videoURL, "mp3")
	}
	if wsErr != nil || wsAudio == nil || wsAudio.Result.AudioURL == "" {
		clearWait()
		playCmdError(s, info)
		return
	}
	data := *wsAudio

	// Prepare metadata (N/A if not available)
	title := "N/A"
	if selected != nil && selected.Title != "" {
		title = selected.Title
	} else if data.Metadata.Title != "" {
		title = data.Metadata.Title
	}
	author := data.Metadata.Author
	if author == "" {
		author = "N/A"
	}
	duration := data.Metadata.Duration
	if duration == "" && selected != nil {
		duration = selected.Duration
	}
	if duration == "" {
		duration = "N/A"
	}
	views := formatMetadataNumber(data.Metadata.Views) // returns "N/A" if 0

	// Download thumbnail (prepare but DON'T send yet — hold until audio download completes)
	thumbnail := downloadThumbnail(ctx, client, data.Metadata.Thumbnail)

	// Prepare info caption (hold it — send after audio download completes)
	infoCaption := fmt.Sprintf("*%s*\n\n🔰 *AUTHOR :❯ %s*\n🔰 *DURATION :❯ %s*\n🔰 *VIEWS :❯ %s*",
		strings.ToUpper(title), strings.ToUpper(author), strings.ToUpper(duration), strings.ToUpper(views))

	// STEP 1: Download audio (silent)
	rawAudioPath, err := streamDownloadToFile(ctx, client, data.Result.AudioURL, nil)
	if err != nil {
		clearWait()
		playCmdError(s, info)
		return
	}
	defer os.Remove(rawAudioPath)

	// STEP 1b: Convert to proper MP3 with ffmpeg (bounded RAM, file-to-file).
	audioPath, convErr := audioRemuxFile(ctx, rawAudioPath)
	if convErr != nil {
		audioPath = rawAudioPath // fallback: send raw audio if ffmpeg fails
	} else {
		defer os.Remove(audioPath)
	}

	// STEP 2: Audio download COMPLETE — delete waiting message
	clearWait()

	// STEP 3: NOW send thumbnail + info (after audio download completes, before audio send)
	if len(thumbnail) > 0 {
		if thumbErr := s.SendImage(info, thumbnail, infoCaption); thumbErr != nil {
		}
	} else {
		// No thumbnail available — send info as text message
		s.Reply(info, infoCaption)
	}

	// STEP 4: Send plain audio (no caption, no footer)
	if err := s.SendAudioFile(info, audioPath, "", 0); err != nil {
		playCmdError(s, info)
	}
}

// ============================================================================
// ▀▀   SHARED HELPER FUNCTIONS   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
// ============================================================================

// ytLoaderToFallback resolves a YouTube video via loader.to.
// Returns a WSFastResponse-shaped struct so existing code paths keep working.
// It polls the progress endpoint (1.5s interval, max ~45s) until the
// download_url is ready. format: "360" (video) or "mp3" (audio).
func ytLoaderToFallback(ctx context.Context, client *http.Client, videoURL, format string) (*WSFastResponse, error) {
	// kick off the conversion
	apiURL := fmt.Sprintf("%s?format=%s&url=%s", ytLoaderAPI, format, url.QueryEscape(videoURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var kick struct {
		Success     bool   `json:"success"`
		ID          string `json:"id"`
		ProgressURL string `json:"progress_url"`
		Title       string `json:"title"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&kick); err != nil || !kick.Success {
		return nil, fmt.Errorf("loader.to kick failed")
	}
	if kick.ProgressURL == "" {
		return nil, fmt.Errorf("loader.to no progress url")
	}
	// poll for the ready download url
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		preq, perr := http.NewRequestWithContext(ctx, http.MethodGet, kick.ProgressURL, nil)
		if perr != nil {
			return nil, perr
		}
		pres, err := client.Do(preq)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
			continue
		}
		var prog struct {
			Success     json.Number `json:"success"` // loader.to sends 0/1 numbers, NOT booleans
			Progress    int         `json:"progress"`
			DownloadURL string      `json:"download_url"`
		}
		derr := json.NewDecoder(io.LimitReader(pres.Body, 1<<20)).Decode(&prog)
		pres.Body.Close()
		if derr == nil && prog.DownloadURL != "" && (prog.Success.String() == "" || prog.Success.String() == "1" || prog.Success.String() == "true") {
			out := &WSFastResponse{Success: true}
			out.Metadata.Title = kick.Title
			out.Metadata.Thumbnail = ""
			if format == "mp3" {
				out.Result.AudioURL = prog.DownloadURL
			} else {
				out.Result.VideoURL = prog.DownloadURL
			}
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("loader.to timeout")
}

// streamDownloadToFile downloads media with a bounded 64 KiB buffer.
// The complete media is never accumulated in RAM.
func streamDownloadToFile(ctx context.Context, client *http.Client, dlURL string, progressFunc func(int)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "gold-md-download-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	removeOnError := func(e error) (string, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return "", e
	}

	contentLength, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	var downloaded int64
	buf := make([]byte, 64*1024)
	for {
		if cerr := ctx.Err(); cerr != nil {
			return removeOnError(cerr)
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := file.Write(buf[:n])
			if writeErr != nil || written != n {
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				return removeOnError(writeErr)
			}
			downloaded += int64(n)
			if progressFunc != nil && contentLength > 0 {
				progressFunc(int(downloaded * 100 / contentLength))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return removeOnError(readErr)
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// fastRemuxFile performs a file-to-file container remux without loading media into RAM.
func fastRemuxFile(ctx context.Context, inputPath string) (string, error) {
	outputPath := inputPath + ".out.mp4"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath,
		"-c:v", "copy", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// audioRemuxFile converts audio to MP3 using file-to-file FFmpeg processing.
func audioRemuxFile(ctx context.Context, inputPath string) (string, error) {
	outputPath := inputPath + ".out.mp3"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath,
		"-vn", "-codec:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2", outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg audio error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// formatMetadataNumber returns the number as a string, or "N/A" if 0 or negative
func formatMetadataNumber(n int) string {
	if n <= 0 {
		return "N/A"
	}
	return strconv.Itoa(n)
}

// downloadThumbnail fetches the thumbnail image bytes from a URL
func downloadThumbnail(ctx context.Context, client *http.Client, thumbnailURL string) []byte {
	if thumbnailURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbnailURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// ── YouTube search helpers ──────────────────────────────────────────────────

// youtubeSearch scrapes YouTube search results and returns up to videoMaxResults entries
func youtubeSearch(ctx context.Context, query string) []VideoResult {
	// Innertube search API (ytinfo.go). The HTML results page now serves a
	// CONSENT/CAPTCHA wall to datacenter IPs, so page scraping no longer works.
	yt, err := ytInnertubeSearch(query)
	if err != nil || len(yt) == 0 {
		return nil
	}
	results := make([]VideoResult, 0, len(yt))
	for _, v := range yt {
		results = append(results, VideoResult{
			Title:     v.Title,
			URL:       v.Link,
			Thumbnail: "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg",
			Duration:  v.Length,
		})
	}
	return results
}

// parseYouTubeResults extracts video results from YouTube HTML
func parseYouTubeResults(html string) []VideoResult {
	re := regexp.MustCompile(`ytInitialData\s*=\s*(\{)`)
	loc := re.FindStringIndex(html)
	if loc == nil {
		return nil
	}
	start := loc[1] - 1
	jsonStr := extractJSON(html, start)
	if jsonStr == "" {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}
	var results []VideoResult
	seen := make(map[string]bool)
	findVideoRenderers(raw, &results, seen)
	if len(results) > videoMaxResults {
		results = results[:videoMaxResults]
	}
	return results
}

// extractJSON extracts a balanced JSON object starting at the given position
func extractJSON(s string, start int) string {
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// findVideoRenderers recursively walks the JSON tree to find videoRenderer objects
func findVideoRenderers(obj interface{}, results *[]VideoResult, seen map[string]bool) {
	switch v := obj.(type) {
	case map[string]interface{}:
		if vr, ok := v["videoRenderer"].(map[string]interface{}); ok {
			if r := parseVideoRenderer(vr); r != nil && r.URL != "" {
				if !seen[r.URL] {
					seen[r.URL] = true
					*results = append(*results, *r)
				}
			}
		}
		for _, val := range v {
			findVideoRenderers(val, results, seen)
		}
	case []interface{}:
		for _, val := range v {
			findVideoRenderers(val, results, seen)
		}
	}
}

// parseVideoRenderer extracts a single VideoResult from a videoRenderer JSON node
func parseVideoRenderer(vr map[string]interface{}) *VideoResult {
	vid, _ := vr["videoId"].(string)
	if vid == "" {
		return nil
	}
	title := ""
	if titleObj, ok := vr["title"].(map[string]interface{}); ok {
		if runs, ok := titleObj["runs"].([]interface{}); ok {
			for _, r := range runs {
				if runMap, ok := r.(map[string]interface{}); ok {
					if t, ok := runMap["text"].(string); ok {
						title += t
					}
				}
			}
		}
		if title == "" {
			if t, ok := titleObj["simpleText"].(string); ok {
				title = t
			}
		}
	}
	duration := ""
	if lenObj, ok := vr["lengthText"].(map[string]interface{}); ok {
		if t, ok := lenObj["simpleText"].(string); ok {
			duration = t
		}
	}
	return &VideoResult{
		Title:     title,
		URL:       fmt.Sprintf("https://youtu.be/%s", vid),
		Thumbnail: fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", vid),
		Duration:  duration,
	}
}
