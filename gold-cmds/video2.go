package goldcmds

// ============================================================================
// GOLD-MD — .video2 TURBO YouTube Downloader
// File: video2.go
// ============================================================================
// The fastest download path in the bot:
//   1. THREE Innertube clients fired in PARALLEL (ANDROID + ANDROID_VR +
//      VISIONOS). Whichever returns a usable stream FIRST wins the race
//      (~0.3s). All clients serve pre-signed googlevideo URLs (no signature
//      deciphering, no API key) — verified ~100 MB/s from this server.
//   2. Default: itag 18 (360p MP4 H.264+AAC combined) — single stream,
//      WhatsApp-ready, only a +faststart remux needed. Even a 100 MB file
//      lands in seconds.
//   3. HD mode ("hd" / "720" / "1080" arg): best adaptive video itag
//      (137=1080p / 136=720p / ...) + itag 140 audio downloaded in PARALLEL
//      and merged with ffmpeg stream-copy (no re-encode).
//   4. Last resort for blocked videos (music / age-gate): loader.to.
// Uses the shared helpers from video-play.go / ytdirect.go.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── Turbo race constants ────────────────────────────────────────────────────

const (
	yt2RaceTimeout = 8 * time.Second   // overall player race deadline
	yt2DlTimeout   = 150 * time.Second // media download deadline
	yt2MaxWASend   = 95 * 1024 * 1024  // WhatsApp-safe cap (fallback to 360p above)
)

// yt2HTTP is the shared HTTP client for the player race.
var yt2HTTP = &http.Client{Timeout: yt2RaceTimeout}

// yt2VideoItags lists adaptive VIDEO-only itags, best quality first.
var yt2VideoItags = []int{137, 136, 135, 134, 133} // 1080p → 240p (H.264)

// yt2AudioItags lists adaptive audio itags, best first.
var yt2AudioItags = []int{140, 139} // m4a AAC 128k → 48k

// yt2Client is one Innertube client configuration for the race.
type yt2Client struct {
	name    string
	client  map[string]any
	headers map[string]string
}

// yt2Clients returns the race participants. All three verified working from
// this server (pre-signed googlevideo URLs, ~100 MB/s downloads).
func yt2Clients() []yt2Client {
	return []yt2Client{
		{
			name: "ANDROID",
			client: map[string]any{
				"clientName": "ANDROID", "clientVersion": "21.26.364",
				"androidSdkVersion": 30, "osName": "Android", "osVersion": "11",
				"hl": "en", "gl": "US",
			},
			headers: map[string]string{
				"User-Agent": "com.google.android.youtube/21.26.364 (Linux; U; Android 11) gzip",
			},
		},
		{
			name: "ANDROID_VR",
			client: map[string]any{
				"clientName": "ANDROID_VR", "clientVersion": "1.61.48",
				"deviceMake": "Oculus", "deviceModel": "Quest 3",
				"androidSdkVersion": 32, "osName": "Android", "osVersion": "12",
				"hl": "en", "gl": "US",
			},
			headers: map[string]string{
				"User-Agent": "com.google.android.apps.youtube.vr.oculus/1.61.48 (Linux; U; Android 12; eureka-user Build/SQ3A.220605.009.A1) gzip",
			},
		},
		{
			name: "VISIONOS",
			client: map[string]any{
				"clientName": "VISIONOS", "clientVersion": "1.02",
				"deviceMake": "Apple", "deviceModel": "RealityDevice17,1",
				"osName": "visionOS", "osVersion": "26.5.23O471",
				"hl": "en", "gl": "US",
			},
			headers: map[string]string{
				"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15",
			},
		},
	}
}

// ── Command registration ────────────────────────────────────────────────────

func init() {
	// Main turbo command (visible in menu + count)
	Register(Command{Name: "video", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD YOUTUBE VIDEOS. TYPE A VIDEO NAME OR LINK AND THE BOT SENDS THE VIDEO FILE.", Run: handleVideo2})
	// Hidden aliases — fully functional but not in menu / TOTAL COMMANDS count
	Register(Command{Name: "v2", Hidden: true, Run: handleVideo})
	Register(Command{Name: "ytv2", Hidden: true, Run: handleVideo})
}

// handleVideo2 is the entry point for the .video2 turbo command.
// Supports: URL / name search / number pick from session / "hd" quality flag.
func handleVideo2(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeoutCmd(s, info, "VIDEO", "VIDEO2", func(ctx context.Context) {
		handleVideo2Async(ctx, s, info, args, prefix)
	})
}

func handleVideo2Async(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Quick-pick from a video2 search session:
	// args = [URL, thumbnail, title, duration, "HD"?]
	if len(args) >= 4 && (strings.Contains(args[0], "youtube.com/") || strings.Contains(args[0], "youtu.be/")) {
		hd := len(args) >= 5 && strings.EqualFold(strings.TrimSpace(args[4]), "HD")
		picked := &VideoResult{URL: args[0], Thumbnail: args[1], Title: args[2], Duration: args[3]}
		downloadAndSendVideo2(ctx, s, info, args[0], picked, hd)
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO TURBO COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE AT MAX SPEED* \n*TYPE SAME LIKE THAT* \n*%sVIDEO ❮ VIDEO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sVIDEO SHAPE OF YOU* \n\n*FOR HD QUALITY TYPE* \n*%sVIDEO HD SHAPE OF YOU* \n\n*TYPE COMMAND + VIDEO NAME TO DOWNLOAD VIDEO FROM YOUTUBE*", prefix, prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendVideo2(ctx, s, info, input, nil, false)
		return
	}

	// HD quality flag detection: "hd", "720", "1080" anywhere in the input
	hd := false
	words := strings.Fields(input)
	var nameParts []string
	for _, w := range words {
		lw := strings.ToLower(w)
		if lw == "hd" || lw == "720" || lw == "1080" || lw == "720p" || lw == "1080p" {
			hd = true
			continue
		}
		nameParts = append(nameParts, w)
	}
	query := strings.TrimSpace(strings.Join(nameParts, " "))

	if query == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO TURBO COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE AT MAX SPEED* \n*TYPE SAME LIKE THAT* \n*%sVIDEO ❮ VIDEO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sVIDEO HD SHAPE OF YOU*", prefix, prefix))
		return
	}

	// Search by name → number selection list
	searchProgressVideo2(ctx, s, info, query, hd)
}

// searchProgressVideo2 runs the search and shows the number-selection list.
// The session is tagged "video2"/"video2hd" so picks route back through the
// turbo engine (with HD when requested).
func searchProgressVideo2(ctx context.Context, s SessionBridge, info types.MessageInfo, query string, hd bool) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING ON YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	s.DeleteMessage(info, waitMsgID)

	if len(results) == 0 {
		video2CmdError(s, info)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*TOP %d RESULTS FOR YOUR SEARCH* \n *%s* \n\n", len(results), query))
	sb.WriteString("*FIRST CHECK THE WHOLE LIST AND TYPE ANY NUMBER 1, OR 6 OR 14 OR 2 OR OTHER ANY NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD FROM YOUTUBE* \n\n")
	if hd {
		sb.WriteString("*🔰 HD MODE ACTIVE — SELECTED VIDEO WILL DOWNLOAD IN HD* \n\n")
	}

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

		sb.WriteString(fmt.Sprintf("\n*🔰═══════•❖❁❖•═══════🔰* \n*TYPE ❰ %d ❱ TO DOWNLOAD THIS FROM YT* \n", i+1))
		sb.WriteString(fmt.Sprintf("%s \n", nameStr))
		sb.WriteString(fmt.Sprintf("%s \n", linkStr))
		sb.WriteString(fmt.Sprintf("*DURATION :❯ %s* \n*🔰═══════•❖❁❖•═══════🔰* \n\n", durStr))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*TYPE NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO %d*", len(results)))

	s.SetVideoSession2(info.Sender.String(), results, hd)
	s.Reply(info, sb.String())
}

// ── Turbo race engine ───────────────────────────────────────────────────────

// yt2Stream is a parsed playable result from one Innertube client.
type yt2Stream struct {
	client   string
	title    string
	author   string
	duration string
	views    int
	thumb    string
	itag18   string // 360p MP4 H.264+AAC combined (WhatsApp-ready)
	adaptive map[int]string
}

// yt2PlayerCall queries one Innertube client for the video. Returns nil on
// any failure or non-PLAYABLE status (e.g. LOGIN_REQUIRED blocks).
func yt2PlayerCall(ctx context.Context, c yt2Client, videoID string) *yt2Stream {
	body := map[string]any{
		"context":          map[string]any{"client": c.client},
		"videoId":          videoID,
		"contentCheckOk":   true,
		"racyCheckOk":      true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ytDirectPlayerURL, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	res, err := yt2HTTP.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()

	var pr ytDirectPlayerResp
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&pr); err != nil {
		return nil
	}
	if pr.PlayabilityStatus.Status != "OK" {
		return nil // blocked / login required / unavailable on this client
	}

	st := &yt2Stream{client: c.name, adaptive: map[int]string{}}
	for _, f := range pr.StreamingData.Formats {
		if f.Itag == ytDirectItagVideo && f.URL != "" {
			st.itag18 = f.URL
		}
	}
	for _, f := range pr.StreamingData.AdaptiveFormats {
		if f.URL != "" {
			st.adaptive[f.Itag] = f.URL
		}
	}
	if st.itag18 == "" && len(st.adaptive) == 0 {
		return nil
	}

	st.title = pr.VideoDetails.Title
	st.author = pr.VideoDetails.Author
	st.views, _ = strconv.Atoi(pr.VideoDetails.ViewCount)
	if n, err := strconv.Atoi(pr.VideoDetails.LengthSec); err == nil && n > 0 {
		st.duration = ytDirectFmtDuration(n)
	}
	if ts := pr.VideoDetails.Thumb.Thumbnails; len(ts) > 0 {
		st.thumb = ts[len(ts)-1].URL
	}
	return st
}

// yt2BestVideo returns the best adaptive video-only itag + URL.
func yt2BestVideo(st *yt2Stream) (int, string) {
	if st == nil {
		return 0, ""
	}
	for _, it := range yt2VideoItags {
		if u, ok := st.adaptive[it]; ok && u != "" {
			return it, u
		}
	}
	return 0, ""
}

// yt2BestAudio returns the best adaptive audio itag + URL.
func yt2BestAudio(st *yt2Stream) (int, string) {
	if st == nil {
		return 0, ""
	}
	for _, it := range yt2AudioItags {
		if u, ok := st.adaptive[it]; ok && u != "" {
			return it, u
		}
	}
	return 0, ""
}

// yt2RaceFetch fires all clients in parallel and returns the FIRST stream
// that satisfies the need (itag18 for standard, adaptive video+audio for
// HD). If nothing fully satisfies it before the deadline, the best partial
// result wins; nil only when every client failed.
func yt2RaceFetch(ctx context.Context, videoID string, wantHD bool) *yt2Stream {
	clients := yt2Clients()
	rctx, cancel := context.WithCancel(ctx)
	defer cancel() // kills losing goroutines once we have a winner

	ch := make(chan *yt2Stream, len(clients))
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c yt2Client) {
			defer wg.Done()
			if st := yt2PlayerCall(rctx, c, videoID); st != nil {
				ch <- st
			}
		}(c)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var bestAny *yt2Stream
	deadline := time.NewTimer(yt2RaceTimeout)
	defer deadline.Stop()

	for {
		select {
		case st, ok := <-ch:
			if !ok {
				return bestAny // all clients done, best partial wins
			}
			if bestAny == nil {
				bestAny = st
			}
			if wantHD {
				if vIt, _ := yt2BestVideo(st); vIt != 0 {
					if aIt, _ := yt2BestAudio(st); aIt != 0 {
						return st // immediate HD winner
					}
				}
			} else if st.itag18 != "" {
				return st // immediate 360p winner
			}
		case <-deadline.C:
			return bestAny
		case <-ctx.Done():
			return nil
		}
	}
}

// ── Download pipeline ───────────────────────────────────────────────────────

// yt2FetchSimple downloads a combined MP4 (itag18) and faststart-remuxes it.
// On remux failure the raw file is returned (still playable).
func yt2FetchSimple(ctx context.Context, client *http.Client, dlURL string) (string, error) {
	raw, err := streamDownloadToFile(ctx, client, dlURL, nil)
	if err != nil {
		return "", err
	}
	out, rErr := fastRemuxFile(ctx, raw)
	if rErr != nil {
		return raw, nil
	}
	os.Remove(raw)
	return out, nil
}

// yt2FetchMerged downloads video-only + audio streams IN PARALLEL and merges
// them with ffmpeg stream-copy (no re-encode → near-instant).
func yt2FetchMerged(ctx context.Context, client *http.Client, vURL, aURL string) (string, error) {
	var vPath, aPath string
	var vErr, aErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		vPath, vErr = streamDownloadToFile(ctx, client, vURL, nil)
	}()
	go func() {
		defer wg.Done()
		aPath, aErr = streamDownloadToFile(ctx, client, aURL, nil)
	}()
	wg.Wait()

	if vErr != nil || aErr != nil {
		if vPath != "" {
			os.Remove(vPath)
		}
		if aPath != "" {
			os.Remove(aPath)
		}
		return "", fmt.Errorf("parallel download failed")
	}
	defer os.Remove(vPath)
	defer os.Remove(aPath)

	return yt2MergeHD(ctx, vPath, aPath)
}

// yt2MergeHD merges H.264 video + AAC audio into a WhatsApp-ready MP4.
func yt2MergeHD(ctx context.Context, videoPath, audioPath string) (string, error) {
	outputPath := videoPath + ".hd.mp4"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", videoPath, "-i", audioPath,
		"-c:v", "copy", "-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg merge error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// downloadAndSendVideo2 is the turbo pipeline:
// waiting msg → parallel race → (HD merge | 360p stream) → remux →
// delete waiting → thumbnail+info → plain video.
func downloadAndSendVideo2(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult, hd bool) {
	waitMsgID := s.ReplyWithID(info, "*DOWNLOADING VIDEOS FROM YOUTUBE.....*")

	clearWait := func() {
		if waitMsgID != "" {
			s.DeleteMessage(info, waitMsgID)
		}
	}

	linkClient := &http.Client{Timeout: 20 * time.Second}
	dlClient := &http.Client{Timeout: yt2DlTimeout}

	// STEP 1: parallel Innertube race (turbo path)
	var st *yt2Stream
	videoID := ytDirectExtractID(videoURL)
	if videoID != "" {
		st = yt2RaceFetch(ctx, videoID, hd)
	}
	// STEP 1b: everything blocked → loader.to last resort (360p)
	if st == nil {
		ws, err := ytLoaderToFallback(ctx, linkClient, videoURL, "360")
		if err != nil || ws == nil || ws.Result.VideoURL == "" {
			clearWait()
			video2CmdError(s, info)
			return
		}
		st = &yt2Stream{title: ws.Metadata.Title, itag18: ws.Result.VideoURL}
	}

	// STEP 2: metadata (picked result wins, race data fills gaps)
	title := "N/A"
	if picked != nil && picked.Title != "" {
		title = picked.Title
	} else if st.title != "" {
		title = st.title
	}
	author := st.author
	if author == "" {
		author = "N/A"
	}
	duration := st.duration
	if duration == "" && picked != nil {
		duration = picked.Duration
	}
	if duration == "" {
		duration = "N/A"
	}
	views := formatMetadataNumber(st.views)

	thumbURL := st.thumb
	if picked != nil && picked.Thumbnail != "" {
		thumbURL = picked.Thumbnail
	}
	thumbnail := downloadThumbnail(ctx, dlClient, thumbURL)

	infoCaption := fmt.Sprintf("*%s* \n\n🔰 *AUTHOR :❯ %s* \n🔰 *DURATION :❯ %s* \n🔰 *VIEWS :❯ %s*",
		strings.ToUpper(title), strings.ToUpper(author), strings.ToUpper(duration), strings.ToUpper(views))

	// STEP 3: download — HD merge when possible, else single 360p stream
	_, vURL := yt2BestVideo(st)
	_, aURL := yt2BestAudio(st)
	useHD := hd && vURL != "" && aURL != ""

	var finalPath string
	var dlErr error
	if useHD {
		finalPath, dlErr = yt2FetchMerged(ctx, dlClient, vURL, aURL)
		// WhatsApp-safe size cap → fall back to 360p for huge HD files
		if dlErr == nil && st.itag18 != "" {
			if fi, err := os.Stat(finalPath); err == nil && fi.Size() > yt2MaxWASend {
				os.Remove(finalPath)
				finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
			}
		}
		// HD pipeline failed → try 360p before giving up
		if dlErr != nil && st.itag18 != "" {
			finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
		}
	} else if st.itag18 != "" {
		finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
	} else if vURL != "" && aURL != "" {
		// no combined format but adaptive available → merge best (rare)
		finalPath, dlErr = yt2FetchMerged(ctx, dlClient, vURL, aURL)
	} else {
		dlErr = fmt.Errorf("no downloadable stream")
	}

	if dlErr != nil || finalPath == "" {
		clearWait()
		video2CmdError(s, info)
		return
	}
	defer os.Remove(finalPath)

	// STEP 4: download complete — delete waiting message
	clearWait()

	// STEP 5: send thumbnail + info
	if len(thumbnail) > 0 {
		if thumbErr := s.SendImage(info, thumbnail, infoCaption); thumbErr != nil {
		}
	} else {
		s.Reply(info, infoCaption)
	}

	// STEP 6: send plain video (no caption, no footer)
	if err := s.SendVideoFile(info, finalPath, "", nil, 0, 0, 0); err != nil {
		video2CmdError(s, info)
	}
}
