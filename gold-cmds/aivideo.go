package goldcmds

// ============================================================================
// GOLD-MD — AI Video Generator Command (Agnes Video)
// File: aivideo.go
// ============================================================================
// COMMAND: .aivideo <prompt>
//   e.g.   .aivideo a cat eating fish
//          .aivideo I have 2 photos please make it video
//          .aivideo I have photos please make video   (asks for count)
//
// Powered by Agnes Video (agnes-video-v2.0). Three modes:
//   1) TEXT-TO-VIDEO   -> just a prompt
//   2) IMAGE-TO-VIDEO  -> 1 image (after collecting via session flow)
//   3) KEYFRAME VIDEO  -> 2-4 images combined (keyframe animation)
//
// 20-KEY ROTATION (sticky sequential-fill, Remini-style):
//   - One active key at a time. On 429/transient error the key goes into
//     exponential-backoff rest (60s -> 120s -> 240s ... max 5min) and the
//     next free key becomes active.
//   - On success the key is immediately free (video generation already
//     takes minutes, so no artificial rest needed).
//
// MULTI-IMAGE SESSION FLOW:
//   When the user says ".aivideo I have N photos please make it video" the
//   bot asks them to send the images. Each incoming image is uploaded to
//   ImgBB (Agnes needs a public URL) and collected. As soon as all expected
//   images arrive, video generation starts automatically. The user can also
//   type YES to proceed with fewer images, or a number/word for the count.
//   Sessions auto-expire after 2 minutes.
//
// NOTE (per user request): NO progress bar / "PROCESSING: XX%" edits. A
// single static "CREATING VIDEO......" message is sent, then deleted and
// replaced with the result when ready. This keeps the server fast.
//
// Debug logging controlled by GOLDMD_DEBUG env flag (default off).
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	avideoBaseURL          = "https://apihub.agnes-ai.com"
	avideoCreateURL        = avideoBaseURL + "/v1/videos"
	avideoPollURL          = avideoBaseURL + "/agnesapi"
	avideoModel            = "agnes-video-v2.0"
	avideoFrameRate        = 24
	avideoMaxSeconds       = 16
	avideoMinFrames        = 81
	avideoMaxFrames        = 8*((avideoMaxSeconds*avideoFrameRate)-1)/8 + 1 // = 377 (~15.7s)
	avideoDefaultRestMs    = 60 * 1000
	avideoMaxRestMs        = 5 * 60 * 1000
	avideoPollIntervalMs   = 5000
	avideoBusyPollMs       = 4000
	avideoInlineVideoMax   = 16 * 1024 * 1024 // 16 MB inline video threshold
	avideoMinCombineImages = 1
	avideoMaxCombineImages = 4
	avideoNudgeAfterMs     = 25 * 1000
	avideoSessionMaxMs     = 2 * 60 * 1000
	avideoImgbbTimeoutMs   = 30 * 1000
	avideoPollMaxMs        = 2 * 60 * 1000 // hard ceiling: 2-minute timeout (user request)
	avideoTimeoutMsg       = "YOUR PROMOT IS VERY BIG TRY TO GIVE THE SMALL PROMOT TO CREATE AI VIDEO SORRY 🔰"
	avideoBodyLimit        = 64 * 1024 * 1024 // cap any single HTTP response read (64 MB)
)

var avideoStyleSuffix = ", hyper stylish cinematic look, vibrant saturated colors, dramatic dynamic lighting, " +
	"punchy high-contrast color grade, smooth dynamic camera movement, energetic fast-paced motion, " +
	"trendy social-media reel aesthetic, ultra high quality, sharp detailed, 4k"

var avideoNegativePrompt = "blurry, low quality, dull flat colors, static boring shot, washed out, overexposed, " +
	"distorted face, extra limbs, watermark, text artifacts, low resolution, choppy motion"

var avideoRetryableStatuses = map[int]bool{
	408: true, 420: true, 429: true, 500: true, 502: true, 503: true, 504: true, 520: true, 522: true, 524: true,
}

var avideoImgbbKey = strings.TrimSpace(os.Getenv("IMGBB_API_KEY"))
var avideoImgbbOnce sync.Once

// avDebug prints a JSON-tagged debug line for the aivideo flow when
// GOLDMD_DEBUG is set. This gives full visibility into dispatch, sessions,
// image download/upload, key rotation, generation, polling and sending so
// crashes/errors can be diagnosed live.
var avideoDebugEnabled = avideoInitDebug()

func avideoInitDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOLDMD_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func avDebug(stage string, fields map[string]any) {
	// DISABLED — silent no-op per owner request (zero console output)
	_ = stage
	_ = fields
}

func avJSONCompact(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("%v", m)
	}
	return string(b)
}

func avideoImgbbURL() string {
	avideoImgbbOnce.Do(func() {
		if avideoImgbbKey == "" {
			avideoImgbbKey = "d415dbed2b70b808654b120fb0ba1915"
		}
	})
	return "https://api.imgbb.com/1/upload?key=" + avideoImgbbKey
}

var (
	avideoImageIntentRegex = regexp.MustCompile(`\b(photo|photos|pic|pics|picture|pictures|image|images|tasveer|tasveerein)\b`)
	avideoYesRegex         = regexp.MustCompile(`(?i)^(yes|y|ok|okay|haan|han|sure|proceed|go)$`)
	avideoCancelRegex      = regexp.MustCompile(`(?i)^(cancel|stop|no|nahi|band)$`)
	avideoDigitRegex       = regexp.MustCompile(`\b([1-4])\b`)
	avideoSecondsRegex     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:seconds?|secs?|s)\b`)
)

var avideoNumberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4,
	"ek": 1, "do": 2, "teen": 3, "char": 4, "chaar": 4,
}

// ----- 20-key pool -----

type avideoKeyEntry struct {
	index     int
	key       string
	restUntil int64 // unix nano; 0 = free
	busy      bool
	failCount int
	mu        sync.Mutex
}

var (
	avideoKeyPool   []*avideoKeyEntry
	avideoActiveIdx = 0
	avideoPoolMu    sync.Mutex
	avideoInitOnce  sync.Once
	avideoCmdLabel  = "AIVIDEO" // overridden by aivideo2 via init
)

func avideoInitPool(keys []string, envPrefix string) {
	avideoInitOnce.Do(func() {
		for i := 0; i < len(keys); i++ {
			key := strings.TrimSpace(os.Getenv(fmt.Sprintf("%s_%d", envPrefix, i+1)))
			if key == "" {
				key = keys[i]
			}
			avideoKeyPool = append(avideoKeyPool, &avideoKeyEntry{index: i + 1, key: key})
		}
	})
}

func (e *avideoKeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *avideoKeyEntry) msLeft() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.restUntil == 0 {
		return 0
	}
	left := (e.restUntil - time.Now().UnixNano()) / int64(time.Millisecond)
	if left < 0 {
		return 0
	}
	return left
}

func (e *avideoKeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := 0
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	} else {
		e.failCount++
		backoff := int64(avideoDefaultRestMs)
		for i := 1; i < e.failCount; i++ {
			backoff *= 2
		}
		if backoff > avideoMaxRestMs {
			backoff = avideoMaxRestMs
		}
		ms = int(backoff)
	}
	e.failCount++
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

func avideoGetNextAvailableKey() *avideoKeyEntry {
	avideoPoolMu.Lock()
	defer avideoPoolMu.Unlock()
	scan := avideoActiveIdx
	for tries := 0; tries < len(avideoKeyPool); tries++ {
		idx := scan % len(avideoKeyPool)
		entry := avideoKeyPool[idx]
		entry.mu.Lock()
		if entry.key == "" {
			entry.mu.Unlock()
			scan++
			avideoActiveIdx = scan
			continue
		}
		if entry.restUntil != 0 && entry.restUntil > time.Now().UnixNano() {
			entry.mu.Unlock()
			scan++
			avideoActiveIdx = scan
			continue
		}
		if entry.busy {
			entry.mu.Unlock()
			scan++
			continue
		}
		avideoActiveIdx = idx
		entry.busy = true
		entry.mu.Unlock()
		return entry
	}
	return nil
}

func avideoGetPoolStatus() (hasBusy bool, totalMs, soonestMs int64) {
	for _, e := range avideoKeyPool {
		if e.key == "" {
			continue
		}
		e.mu.Lock()
		if e.busy {
			e.mu.Unlock()
			hasBusy = true
			continue
		}
		e.mu.Unlock()
		left := e.msLeft()
		totalMs += left
		if soonestMs == 0 || left < soonestMs {
			soonestMs = left
		}
	}
	totalMs += 20 * 1000
	return
}

// ----- frame resolution + style -----

func avideoResolveNumFrames(promptText string) int {
	m := avideoSecondsRegex.FindStringSubmatch(promptText)
	if m != nil {
		seconds, _ := strconv.ParseFloat(m[1], 64)
		if seconds <= 0 {
			seconds = float64(avideoMaxFrames) / float64(avideoFrameRate)
		}
		if seconds > avideoMaxSeconds {
			seconds = avideoMaxSeconds
		}
		n := int(((seconds * float64(avideoFrameRate)) - 1) / 8)
		frames := 8*n + 1
		if frames > avideoMaxFrames {
			frames = avideoMaxFrames
		}
		if frames < avideoMinFrames {
			frames = avideoMinFrames
		}
		return frames
	}
	// AUTO: random length within valid range, max 16s cap
	minN := (avideoMinFrames - 1) / 8
	maxN := (avideoMaxFrames - 1) / 8
	if maxN <= minN {
		return avideoMinFrames
	}
	n := minN + rand.Intn(maxN-minN+1)
	return 8*n + 1
}

func avideoBuildStylizedPrompt(basePrompt string) string {
	trimmed := strings.TrimSpace(basePrompt)
	if trimmed == "" {
		return trimmed
	}
	if len(trimmed) > 380 {
		return trimmed
	}
	return trimmed + avideoStyleSuffix
}

// ----- ImgBB upload -----

func avideoUploadImageToURL(data []byte) (string, error) {
	// avDebug("AIVIDEO_IMGBB_UPLOAD", map[string]any{"bytes": len(data)})
	url := avideoImgbbURL()
	for attempt := 1; attempt <= 3; attempt++ {
		bb := bytes.NewBuffer(data)
		req, err := http.NewRequest("POST", url, bb)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		client := &http.Client{Timeout: time.Duration(avideoImgbbTimeoutMs) * time.Millisecond}
		resp, err := client.Do(req)
		if err != nil {
			if attempt < 3 {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return "", fmt.Errorf("imgbb request: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			if attempt < 3 {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return "", fmt.Errorf("imgbb status=%d body=%s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Data struct {
				DisplayURL string `json:"display_url"`
				URL        string `json:"url"`
			} `json:"data"`
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", fmt.Errorf("imgbb parse: %v", err)
		}
		if parsed.Data.DisplayURL != "" {
			return parsed.Data.DisplayURL, nil
		}
		if parsed.Data.URL != "" {
			return parsed.Data.URL, nil
		}
		return "", fmt.Errorf("imgbb no url in response: %s", string(body))
	}
	return "", fmt.Errorf("imgbb upload failed after retries")
}

// ----- video generation with a specific key -----

func avideoGenerateWithKey(entry *avideoKeyEntry, prompt string, imageUrls []string) ([]byte, int, error) {
	// avDebug("AIVIDEO_GEN_START", map[string]any{"keyIndex": entry.index, "imageCount": len(imageUrls), "promptLen": len(prompt)})
	payload := map[string]any{
		"model":           avideoModel,
		"prompt":          prompt,
		"num_frames":      avideoResolveNumFrames(prompt),
		"frame_rate":      avideoFrameRate,
		"negative_prompt": avideoNegativePrompt,
	}
	if len(imageUrls) == 1 {
		payload["image"] = imageUrls[0]
	} else if len(imageUrls) >= 2 {
		payload["extra_body"] = map[string]any{
			"image": imageUrls,
			"mode":  "keyframes",
		}
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", avideoCreateURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+entry.key)
	req.Header.Set("Content-Type", "application/json")

	createClient := &http.Client{Timeout: 120 * time.Second}
	createResp, err := createClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	createBody, _ := io.ReadAll(io.LimitReader(createResp.Body, avideoBodyLimit))
	createResp.Body.Close()

	if avideoRetryableStatuses[createResp.StatusCode] {
		retryAfter := 0
		if ra := createResp.Header.Get("Retry-After"); ra != "" {
			fmt.Sscanf(ra, "%d", &retryAfter)
		}
		return nil, retryAfter, fmt.Errorf("retryable_status_%d", createResp.StatusCode)
	}
	// avDebug("AIVIDEO_GEN_CREATE_RESP", map[string]any{"keyIndex": entry.index, "status": createResp.StatusCode, "bodyLen": len(createBody), "bodySnippet": avTruncStr(string(createBody), 300)})
	if createResp.StatusCode != 200 {
		// avDebug("AIVIDEO_GEN_CREATE_FAIL", map[string]any{"keyIndex": entry.index, "status": createResp.StatusCode, "body": avTruncStr(string(createBody), 500)})
		return nil, 0, fmt.Errorf("create status=%d body=%s", createResp.StatusCode, string(createBody))
	}

	var createResult struct {
		VideoID string `json:"video_id"`
		ID      string `json:"id"`
		TaskID  string `json:"task_id"`
		Data    struct {
			VideoID string `json:"video_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(createBody, &createResult)
	videoID := createResult.VideoID
	if videoID == "" {
		videoID = createResult.ID
	}
	if videoID == "" {
		videoID = createResult.TaskID
	}
	if videoID == "" {
		videoID = createResult.Data.VideoID
	}
	// avDebug("AIVIDEO_GEN_VIDEO_ID", map[string]any{"keyIndex": entry.index, "videoID": videoID})
	if videoID == "" {
		// avDebug("AIVIDEO_GEN_NO_VIDEO_ID", map[string]any{"keyIndex": entry.index, "body": avTruncStr(string(createBody), 500)})
		return nil, 0, fmt.Errorf("no video_id in create response: %s", string(createBody))
	}

	// Polling loop — BOUNDED by avideoPollMaxMs so a stuck task can never leak a
	// goroutine / memory forever (which trips the 400 MB memory watchdog and
	// causes the "safe restart" the user reported).
	pollClient := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(time.Duration(avideoPollMaxMs) * time.Millisecond)
	for {
		if time.Now().After(deadline) {
			return nil, 0, fmt.Errorf("video generation timed out after %d minutes", avideoPollMaxMs/60000)
		}
		time.Sleep(time.Duration(avideoPollIntervalMs) * time.Millisecond)
		pollReq, perr := http.NewRequest("GET", avideoPollURL+"?video_id="+videoID, nil)
		if perr != nil {
			continue
		}
		pollReq.Header.Set("Authorization", "Bearer "+entry.key)
		pollResp, perr := pollClient.Do(pollReq)
		if perr != nil {
			continue // transient — keep polling
		}
		pollBody, _ := io.ReadAll(io.LimitReader(pollResp.Body, avideoBodyLimit))
		pollResp.Body.Close()

		var pr struct {
			Status      string `json:"status"`
			State       string `json:"state"`
			Progress    string `json:"progress_state"`
			URL         string `json:"url"`
			VideoURL    string `json:"video_url"`
			DownloadURL string `json:"download_url"`
			Result      struct {
				URL string `json:"url"`
			} `json:"result"`
			Metadata struct {
				URL string `json:"url"`
			} `json:"metadata"`
			Data []struct {
				URL string `json:"url"`
			} `json:"data"`
			Error any `json:"error"`
		}
		_ = json.Unmarshal(pollBody, &pr)
		// avDebug("AIVIDEO_POLL", map[string]any{"keyIndex": entry.index, "videoID": videoID, "pollStatus": createResp.StatusCode, "bodySnippet": avTruncStr(string(pollBody), 200)})

		status := pr.Status
		if status == "" {
			status = pr.State
		}
		if status == "" {
			status = pr.Progress
		}
		if status == "" {
			status = "unknown"
		}

		videoURL := pr.Metadata.URL
		if videoURL == "" {
			videoURL = pr.URL
		}
		if videoURL == "" {
			videoURL = pr.VideoURL
		}
		if videoURL == "" {
			videoURL = pr.DownloadURL
		}
		if videoURL == "" {
			videoURL = pr.Result.URL
		}
		if videoURL == "" && len(pr.Data) > 0 {
			videoURL = pr.Data[0].URL
		}

		if status == "completed" && videoURL != "" {
			// avDebug("AIVIDEO_POLL_DONE", map[string]any{"keyIndex": entry.index, "videoID": videoID, "videoURL": videoURL, "status": status})
			// Download the video straight to a temp file (streamed) so we do NOT
			// load the whole clip into RAM — the bot runs under a 400 MB container
			// cap and loading big videos in memory was tripping the watchdog.
			dlClient := &http.Client{Timeout: 180 * time.Second}
			dlResp, derr := dlClient.Get(videoURL)
			if derr != nil {
				return nil, 0, fmt.Errorf("download video: %v", derr)
			}
			tmpFile, ferr := os.CreateTemp("", "avideo-*.mp4")
			if ferr != nil {
				dlResp.Body.Close()
				return nil, 0, fmt.Errorf("create temp: %v", ferr)
			}
			written, cerr := io.Copy(tmpFile, io.LimitReader(dlResp.Body, 200*1024*1024))
			dlResp.Body.Close()
			tmpFile.Close()
			// avDebug("AIVIDEO_DL_DONE", map[string]any{"keyIndex": entry.index, "bytes": written, "tmpPath": tmpFile.Name(), "writeErr": cerr != nil})
			if cerr != nil {
				os.Remove(tmpFile.Name())
				return nil, 0, fmt.Errorf("write video: %v", cerr)
			}
			if written == 0 {
				os.Remove(tmpFile.Name())
				return nil, 0, fmt.Errorf("downloaded empty video")
			}
			// Return the temp file PATH as bytes so the caller streams it.
			return []byte(tmpFile.Name()), 0, nil
		}
		if status == "failed" {
			errMsg := "unknown"
			if s, ok := pr.Error.(string); ok && s != "" {
				errMsg = s
			}
			// avDebug("AIVIDEO_POLL_FAILED", map[string]any{"keyIndex": entry.index, "videoID": videoID, "errMsg": errMsg, "bodySnippet": avTruncStr(string(pollBody), 300)})
			return nil, 0, fmt.Errorf("generation failed (%s)", errMsg)
		}
		// any other status -> keep polling
	}
}

// avideoGenerateVideo rotates keys and returns either video bytes or wait info.
func avideoGenerateVideo(prompt string, imageUrls []string) ([]byte, int64, int64, bool, error) {
	configured := 0
	for _, e := range avideoKeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("API KEY NOT FOUND")
	}

	for attempt := 0; attempt < len(avideoKeyPool); attempt++ {
		entry := avideoGetNextAvailableKey()
		// avDebug("AIVIDEO_KEY_ROTATE", map[string]any{"attempt": attempt, "keyIndex": entryIndexOrNeg(entry)})
		if entry == nil {
			break
		}
		var videoData []byte
		var retryAfter int
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("internal panic: %v", r)
				}
			}()
			videoData, retryAfter, err = avideoGenerateWithKey(entry, prompt, imageUrls)
		}()
		entry.mu.Lock()
		entry.busy = false
		entry.mu.Unlock()

		if err == nil {
			entry.mu.Lock()
			entry.failCount = 0
			entry.mu.Unlock()
			return videoData, 0, 0, false, nil
		}
		errStr := err.Error()
		if retryAfter > 0 || strings.Contains(errStr, "retryable_status_") {
			entry.markResting(retryAfter)
			continue
		}
		return nil, 0, 0, false, fmt.Errorf("video generate fail (key #%d): %v", entry.index, err)
	}

	hasBusy, totalMs, soonestMs := avideoGetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, int64(avideoBusyPollMs), int64(avideoBusyPollMs), true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}

// ----- shared generation runner (NO progress bar — simple static message) -----

func avideoRunGenerationFlow(s SessionBridge, info types.MessageInfo, prompt string, imageUrls []string) {
	// avDebug("AIVIDEO_FLOW_START", map[string]any{"sender": info.Sender.String(), "imageCount": len(imageUrls), "promptLen": len(prompt)})
	// Recovery guard: a panic anywhere in the generation flow must NOT kill the
	// whole bot process (which is what causes the "safe restart" the user sees).
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\nSomething went wrong while creating the video. Please try again.")
		}
	}()
	label := "AI CREATING VIDEO......"
	if len(imageUrls) >= 2 {
		label = "AI COMBINING YOUR IMAGES INTO VIDEO......"
	} else if len(imageUrls) == 1 {
		label = "AI ANIMATING YOUR IMAGE......"
	}

	// Simple static message — NO progress bar edits (keeps server fast)
	waitMsgID := s.ReplyWithID(info, "*"+label+"*")

	stylizedPrompt := avideoBuildStylizedPrompt(prompt)

	for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); {
		videoData, waitMs, totalWaitMs, busyOnly, err := avideoGenerateVideo(stylizedPrompt, imageUrls)
		// avDebug("AIVIDEO_GEN_RESULT", map[string]any{"sender": info.Sender.String(), "hasVideo": videoData != nil, "busyOnly": busyOnly, "waitMs": waitMs, "err": errToStr(err)})
		if err != nil {
			s.DeleteMessage(info, waitMsgID)
			if strings.Contains(strings.ToLower(err.Error()), "timed out") {
				// avDebug("AIVIDEO_TIMEOUT", map[string]any{"sender": info.Sender.String(), "err": errToStr(err)})
				s.Reply(info, avideoTimeoutMsg)
			} else {
				s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
			}
			return
		}
		if videoData != nil {
			// Done — delete the wait message and send the result.
			// videoData now carries a TEMP FILE PATH (streamed download) so we
			// always send via SendVideoFile to keep peak RAM low under the 400 MB cap.
			tmpPath := string(videoData)
			s.DeleteMessage(info, waitMsgID)
			modeTag := ""
			if len(imageUrls) >= 2 {
				modeTag = fmt.Sprintf("*MODE:* COMBINED %d IMAGES INTO 1 VIDEO\n", len(imageUrls))
			} else if len(imageUrls) == 1 {
				modeTag = "*MODE:* IMAGE TO VIDEO\n"
			}
			caption := "*AI CREATED VIDEO*\n" + modeTag + "*YOUR PROMPT TEXT IS* 🔰\n\n" + strings.ToUpper(prompt)
			if _, statErr := os.Stat(tmpPath); statErr == nil {
				// avDebug("AIVIDEO_SEND_FILE", map[string]any{"sender": info.Sender.String(), "tmpPath": tmpPath, "captionLen": len(caption)})
				sendErr := s.SendVideoFile(info, tmpPath, caption, nil, 0, 0, 0)
				os.Remove(tmpPath)
				if sendErr != nil {
					s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\nFailed to upload the video. Please try again.")
				}
			} else {
				s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\nVideo file was lost. Please try again.")
			}
			return
		}

		// All keys busy/resting — edit wait message, sleep, retry
		var waitText string
		if busyOnly {
			waitText = "*" + label + "*\n*AI SERVER BUSY - FINDING A FREE SLOT FOR YOU....*"
		} else {
			waitText = fmt.Sprintf("*PLEASE WAIT AI SERVER ARE BUSY AT THE MOMENT PLEASE WAIT %s TO CREATE NEW VIDEOS USING AI*", formatWaitMs(totalWaitMs))
		}
		s.EditMessage(info, waitMsgID, waitText)

		waitDuration := time.Duration(waitMs+500) * time.Millisecond
		if waitDuration < 1*time.Second {
			waitDuration = 1 * time.Second
		}
		time.Sleep(waitDuration)

		s.EditMessage(info, waitMsgID, "*"+label+"*")
	}
	s.DeleteMessage(info, waitMsgID)
	s.Reply(info, "*TRY AGAIN LATER*")
}

// avideoSaveTempVideo writes video bytes to a temp file and returns its path.
func avideoSaveTempVideo(data []byte) string {
	f, err := os.CreateTemp("", "avideo-*.mp4")
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

// ----- multi-image session registry -----

type avideoSession struct {
	mode          string // "awaiting_count" or "collecting"
	expectedCount int
	imageUrls     []string
	promptText    string
	nudgeTimer    *time.Timer
	expireTimer   *time.Timer
}

var (
	avideoSessions = make(map[string]*avideoSession)
	avideoSessMu   sync.Mutex
)

// avideoDeviceSuffixRe strips the ":<device>" suffix from a JID.
// e.g. 923158930864:60@s.whatsapp.net -> 923158930864@s.whatsapp.net
// Precompiled once (RE2-safe, no lookahead) instead of recompiling
// on every message (which previously panicked because Go RE2 does
// NOT support Perl lookahead `(?=)`).
var avideoDeviceSuffixRe = regexp.MustCompile(`:\d+@`)

func avideoStripDevice(jid string) string {
	return avideoDeviceSuffixRe.ReplaceAllString(jid, "@")
}

func avideoSessionKey(info types.MessageInfo) string {
	chat := info.Chat.String()
	sender := info.Sender.String()
	if sender == "" {
		sender = chat
	}
	// strip :device suffix from both
	chat = avideoStripDevice(chat)
	sender = avideoStripDevice(sender)
	return chat + "::" + sender
}

// HasPendingAIVideoSession reports whether a sender has an active session.
// Called by the main package's message handler to decide whether to forward
// image messages to the aivideo flow.
func HasPendingAIVideoSession(sender string) bool {
	avideoSessMu.Lock()
	defer avideoSessMu.Unlock()
	// sender may be a bare JID; check all session keys that end with this sender
	for k, sess := range avideoSessions {
		parts := strings.SplitN(k, "::", 2)
		if len(parts) == 2 && (parts[1] == sender || parts[0] == sender) {
			if sess != nil {
				return true
			}
		}
	}
	return false
}

func avideoDestroySession(key string) {
	avideoSessMu.Lock()
	sess, ok := avideoSessions[key]
	delete(avideoSessions, key)
	avideoSessMu.Unlock()
	if ok && sess != nil {
		if sess.nudgeTimer != nil {
			sess.nudgeTimer.Stop()
		}
		if sess.expireTimer != nil {
			sess.expireTimer.Stop()
		}
	}
}

func avideoStartCountAskSession(s SessionBridge, info types.MessageInfo, key, extraText string) {
	avideoDestroySession(key)
	avideoSessMu.Lock()
	sess := &avideoSession{mode: "awaiting_count", promptText: extraText}
	avideoSessions[key] = sess
	avideoSessMu.Unlock()
	sess.expireTimer = time.AfterFunc(time.Duration(avideoSessionMaxMs)*time.Millisecond, func() {
		avideoExpireSession(s, info, key)
	})
	s.Reply(info, "*WELCOME! LET'S MAKE A VIDEO*\n\n*HOW MANY IMAGES DO YOU WANT TO USE?*\n\n*TYPE ❮ 2 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 2 ❯ IMAGES*\n\n*TYPE ❮ 3 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 3 ❯ IMAGES*\n\n*TYPE ❮ 4 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 4 ❯ IMAGES*\n\n*JUST TYPE NUMBER AND SEND HERE*\n\n*MAX ALLOWED ❮ "+strconv.Itoa(avideoMaxCombineImages)+" ❯ IMAGES*\n\n*OPTIONS:*\n*1 IMAGE = MAKE VIDEO FROM 1 PHOTO*\n*2 TO "+strconv.Itoa(avideoMaxCombineImages)+" IMAGES = COMBINE MULTIPLE PHOTOS INTO 1 VIDEO*\n\n*HOW TO REPLY:*\n*JUST TYPE A NUMBER LIKE 2*\n*OR TYPE A WORD LIKE TWO*\n\n🔰 *YOU HAVE ONLY 2 MINUTES 🔰*\n*SEND YOUR REPLY FAST. IF 2 MINUTES PASS, THE SYSTEM WILL STOP.*\n*THEN YOU WILL HAVE TO TYPE*\n*"+avideoCmdLabel+" ❮ IMG COMBINING PROMPT ❯*\n*AGAIN TO START THE PROCESS*")
}

func avideoStartCollectingSession(s SessionBridge, info types.MessageInfo, key string, count int, extraText string) {
	// avDebug("AIVIDEO_SESSION_COLLECT", map[string]any{"sender": info.Sender.String(), "expectedCount": count})
	avideoDestroySession(key)
	avideoSessMu.Lock()
	sess := &avideoSession{mode: "collecting", expectedCount: count, promptText: extraText}
	avideoSessions[key] = sess
	avideoSessMu.Unlock()
	sess.expireTimer = time.AfterFunc(time.Duration(avideoSessionMaxMs)*time.Millisecond, func() {
		avideoExpireSession(s, info, key)
	})
	plural := "IMAGES"
	if count == 1 {
		plural = "IMAGE"
	}
	s.Reply(info, "*OK! PLEASE SEND "+strconv.Itoa(count)+" "+plural+" NOW*\n*AI WILL MAKE A VIDEO FOR YOU AFTER RECEIVE THEM ALL THE IMAGES*\n\n*PENDING :❯ "+strconv.Itoa(count)+" "+plural+"*\n\n🔰 *YOU HAVE ONLY 2 MINUTES 🔰*\n*PLEASE SEND THE IMAGES WITHIN 2 MINUTES.*\n*IF YOU DON'T SEND IN TIME, THE SYSTEM WILL STOP.*\n*THEN YOU WILL TYPE*\n*"+avideoCmdLabel+" ❮ IMG COMBINING PROMPT ❯*\n*TO START THE IMAGE TO VIDEO CREATION PROCESS AGAIN*")
}

func avideoExpireSession(s SessionBridge, info types.MessageInfo, key string) {
	// This runs in a time.AfterFunc goroutine (NOT the event handler), so it
	// needs its own recover guard. A panic here (e.g. stale client during the
	// s.Reply below) would otherwise crash the whole bot process.
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	_, exists := func() (bool, bool) {
		avideoSessMu.Lock()
		defer avideoSessMu.Unlock()
		_, ok := avideoSessions[key]
		return ok, ok
	}()
	if !exists {
		return
	}
	avideoDestroySession(key)
	s.Reply(info, "*2 MINUTES ARE LEFT PLEASE TYPE*\n*"+avideoCmdLabel+" ❮ IMAGE COMBINING PROMPT ❯*\n*TO CREATE NEW VIDEOS BY COMBINING THE IMAGES*")
}

func avideoBuildDefaultPrompt(count int, extraText string) string {
	cleaned := avideoImageIntentRegex.ReplaceAllString(extraText, "")
	cleaned = regexp.MustCompile(`(?i)\b(please|make|create|video|it|for|me|the|kar|karo|kro|bana|bnao|bnana|banani|hai)\b`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) >= 5 {
		return cleaned
	}
	if count >= 2 {
		return "Create a smooth cinematic transition between the provided images, natural motion, consistent subject and style, high quality realistic movement"
	}
	return "Animate this image with subtle natural motion and cinematic camera movement, keeping the subject and details consistent"
}

func avideoParseImageCount(text string) int {
	clean := strings.ToLower(strings.TrimSpace(text))
	if m := avideoDigitRegex.FindStringSubmatch(clean); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	for _, w := range strings.Fields(clean) {
		w = regexp.MustCompile(`[^a-z]`).ReplaceAllString(w, "")
		if n, ok := avideoNumberWords[w]; ok {
			return n
		}
	}
	return 0
}

func avideoHasImageIntent(text string) bool {
	return avideoImageIntentRegex.MatchString(text)
}

// avideoHandleSessionImage collects an incoming image into the session.
func avideoHandleSessionImage(s SessionBridge, info types.MessageInfo, key string) bool {
	avideoSessMu.Lock()
	sess, ok := avideoSessions[key]
	avideoSessMu.Unlock()
	if !ok || sess == nil || sess.mode != "collecting" {
		return false
	}

	imgData, found := s.DownloadImage(info)
	// avDebug("AIVIDEO_IMG_DOWNLOAD", map[string]any{"sender": info.Sender.String(), "found": found, "bytes": len(imgData)})
	if !found || len(imgData) == 0 {
		s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\nCould not download the image. Please send it again.")
		return true
	}

	url, err := avideoUploadImageToURL(imgData)
	if err != nil {
		s.Reply(info, "🔰 *"+avideoCmdLabel+" COMMAND ERROR* 🔰\nImage upload failed. Please try again.")
		return true
	}

	sess.imageUrls = append(sess.imageUrls, url)
	received := len(sess.imageUrls)
	remaining := sess.expectedCount - received

	if remaining <= 0 {
		prompt := avideoBuildDefaultPrompt(received, sess.promptText)
		urls := make([]string, received)
		copy(urls, sess.imageUrls)
		avideoDestroySession(key)
		s.Reply(info, "*GOT ALL "+strconv.Itoa(received)+" IMAGES NOW AI COMBINING ALL IMAGES AND CREATING VIDEO*\n*PLEASE WAIT.....*")
		go avideoRunGenerationFlow(s, info, prompt, urls)
		return true
	}

	s.Reply(info, "*IMAGE "+strconv.Itoa(received)+" BY "+strconv.Itoa(sess.expectedCount)+" RECIEVED 🔰*\n*REMAINING ❮ "+strconv.Itoa(remaining)+" ❯ MORE*\n*SEND ALL THE IMAGES TO START THE COMBINING IMAGES AND START THE CREATING VIDEO PROCESS*\n\n*OR TYPE ❮ YES ❯ AND SEND HERE IF YOU WANT TO MAKE VIDEO WITH ❮ "+strconv.Itoa(received)+" ❯ IMAGES*")
	return true
}

// avideoHandleSessionText handles text replies during a session (count / YES / cancel).
func avideoHandleSessionText(s SessionBridge, info types.MessageInfo, key, text string) bool {
	avideoSessMu.Lock()
	sess, ok := avideoSessions[key]
	avideoSessMu.Unlock()
	if !ok || sess == nil {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	if avideoCancelRegex.MatchString(trimmed) {
		avideoDestroySession(key)
		s.Reply(info, "🔰 Ok, cancelled. Send *."+strings.ToLower(avideoCmdLabel)+"* again whenever you want to make a video.")
		return true
	}

	if sess.mode == "awaiting_count" {
		count := avideoParseImageCount(trimmed)
		if count < avideoMinCombineImages || count > avideoMaxCombineImages {
			s.Reply(info, "Please reply with a valid number between "+strconv.Itoa(avideoMinCombineImages)+" and "+strconv.Itoa(avideoMaxCombineImages)+" (e.g. 2 or two).")
			return true
		}
		avideoStartCollectingSession(s, info, key, count, sess.promptText)
		return true
	}

	if sess.mode == "collecting" {
		if avideoYesRegex.MatchString(trimmed) {
			received := len(sess.imageUrls)
			if received == 0 {
				s.Reply(info, "*YOU HAVE NOT SENT ANY IMAGE HERE PLEASE SEND THE IMAGE FIRST*")
				return true
			}
			prompt := avideoBuildDefaultPrompt(received, sess.promptText)
			urls := make([]string, received)
			copy(urls, sess.imageUrls)
			avideoDestroySession(key)
			s.Reply(info, "*CREATING VIDEO ❮ "+strconv.Itoa(received)+" ❯ IMAGES YOU SENT HERE......*")
			go avideoRunGenerationFlow(s, info, prompt, urls)
			return true
		}
		// collecting mode: ignore normal text (waiting for images)
		return false
	}
	return false
}

// DispatchAIVideoIncoming is called by the main message handler for every
// incoming message when a pending aivideo session exists for the sender.
// It routes image messages and text replies to the session handlers.
// Returns true if the message was consumed.
func DispatchAIVideoIncoming(s SessionBridge, info types.MessageInfo, text string, hasImg bool) bool {
	key := avideoSessionKey(info)
	avideoSessMu.Lock()
	_, exists := avideoSessions[key]
	avideoSessMu.Unlock()
	// avDebug("AIVIDEO_DISPATCH", map[string]any{"sender": info.Sender.String(), "hasImg": hasImg, "textLen": len(text), "sessionExists": exists})
	if !exists {
		return false
	}
	if hasImg {
		return avideoHandleSessionImage(s, info, key)
	}
	if text != "" {
		return avideoHandleSessionText(s, info, key, text)
	}
	return false
}

// ----- command handlers -----

func handleAIVideo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleAIVideoAsync(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func handleAIVideoAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	// avDebug("AIVIDEO_CMD", map[string]any{"sender": info.Sender.String(), "prompt": prompt, "promptLen": len(prompt)})

	if prompt == "" {
		s.Reply(info, "🔰 *AVIDEO AI COMMAND INFO* 🔰\n\n*🔰 HOW TO USE - FULL STEPS 🔰*\n\n*STEP 1: WRITE .AVIDEO*\n*STEP 2: AFTER IT WRITE YOUR PROMPT (WHAT VIDEO YOU WANT)*\n*STEP 3: SEND THE COMMAND AND WAIT*\n\n*EXAMPLE LIKE THIS*\n*AVIDEO CAT WAS EATING FISH*\n*AVIDEO AEROPLANE WAS FLYING SKY*\n*AVIDEO A MAN WAS DRINKING*\n*AVIDEO ❮ VIDEO NAME ❯*\n*TYPE YOUR VIDEO NAME AND AI WILL CREATE A VIDEO*\n\n*NOTE: FOR PURE TEXT PROMPT NO NEED TO SEND ANY PHOTO*\n*YOU HAVE 3/4 IMAGES AND YOU WANT TO CREATE THE VIDEO*\n*PHOTOS -> VIDEO (IMAGE-TO-VIDEO / COMBINE MULTIPLE PHOTOS)*\n*COMMAND: .AVIDEO I HAVE 2/3 PHOTOS PLEASE MAKE IT VIDEO*\n*COMMAND (WITHOUT COUNT) :❯ .AVIDEO I HAVE PHOTOS PLEASE MAKE VIDEO*\n\n*🔰 IMPORTANT RULES 🔰*\n*1. WRITE YOUR PROMPT CLEARLY AFTER .AVIDEO*\n*2. YOU CAN WRITE COMMAND IN ENGLISH OR URDU*\n*3. USE ONLY 1 COMMAND AT A TIME*\n*4. VIDEO GENERATION HAS NO FIXED TIME LIMIT, PLEASE WAIT PATIENTLY*")
		return
	}

	key := avideoSessionKey(info)

	if avideoHasImageIntent(prompt) {
		count := avideoParseImageCount(prompt)
		if count > 0 {
			avideoStartCollectingSession(s, info, key, count, prompt)
		} else {
			avideoStartCountAskSession(s, info, key, prompt)
		}
		return
	}

	// Plain text-to-video
	avideoRunGenerationFlow(s, info, prompt, nil)
}

// avTruncStr trims a string to maxLen and appends an ellipsis if truncated.
func avTruncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

func entryIndexOrNeg(e *avideoKeyEntry) int {
	if e == nil {
		return -1
	}
	return e.index
}

func errToStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func init() {
	avideoInitPool(hardcodedAIVideoKeys, "AVIDEO_API_KEY")
	Register(Command{Name: "aivideo", Category: "AI & MEDIA", Desc: "Generate an AI video from a prompt", Run: handleAIVideo})
}

// hardcodedAIVideoKeys are defined in aivideo_keys.go (kept separate so the
// aivideo2 variant can supply its own key list without touching this file).
