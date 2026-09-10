package goldcmds

// ============================================================================
// GOLD-MD — AI Video Generator Command #2 (Agnes Video)
// File: aivideo2.go
// (mirror of aivideo.go with a separate 20-key pool, its own session map,
//  command names aivideo2 and label AIVIDEO2. NO progress bar.)
// ============================================================================
// COMMAND: .aivideo2 <prompt>
//   e.g.   .aivideo2 a cat eating fish
//          .aivideo2 I have 2 photos please make it video
//          .aivideo2 I have photos please make video   (asks for count)
//   Also:  .av2info   (shows full info)
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
	av2BaseURL          = "https://apihub.agnes-ai.com"
	av2CreateURL        = av2BaseURL + "/v1/videos"
	av2PollURL          = av2BaseURL + "/agnesapi"
	av2Model            = "agnes-video-v2.0"
	av2FrameRate        = 24
	av2MaxSeconds       = 16
	av2MinFrames        = 81
	av2MaxFrames        = 8*((av2MaxSeconds*av2FrameRate)-1)/8 + 1 // = 377 (~15.7s)
	av2DefaultRestMs    = 60 * 1000
	av2MaxRestMs        = 5 * 60 * 1000
	av2PollIntervalMs   = 5000
	av2BusyPollMs       = 4000
	av2InlineVideoMax   = 16 * 1024 * 1024 // 16 MB inline video threshold
	av2MinCombineImages = 1
	av2MaxCombineImages = 4
	av2NudgeAfterMs     = 25 * 1000
	av2SessionMaxMs     = 2 * 60 * 1000
	av2ImgbbTimeoutMs   = 30 * 1000
	av2PollMaxMs        = 2 * 60 * 1000 // hard ceiling: 2-minute timeout (user request)
	av2TimeoutMsg       = "YOUR PROMOT IS VERY BIG TRY TO GIVE THE SMALL PROMOT TO CREATE AI VIDEO SORRY 🔰"
	av2BodyLimit        = 64 * 1024 * 1024 // cap any single HTTP response read (64 MB)
)

var av2StyleSuffix = ", hyper stylish cinematic look, vibrant saturated colors, dramatic dynamic lighting, " +
	"punchy high-contrast color grade, smooth dynamic camera movement, energetic fast-paced motion, " +
	"trendy social-media reel aesthetic, ultra high quality, sharp detailed, 4k"

var av2NegativePrompt = "blurry, low quality, dull flat colors, static boring shot, washed out, overexposed, " +
	"distorted face, extra limbs, watermark, text artifacts, low resolution, choppy motion"

var av2RetryableStatuses = map[int]bool{
	408: true, 420: true, 429: true, 500: true, 502: true, 503: true, 504: true, 520: true, 522: true, 524: true,
}

var av2ImgbbKey = strings.TrimSpace(os.Getenv("IMGBB_API_KEY"))
var av2ImgbbOnce sync.Once

// av2Debug prints a JSON-tagged debug line for the aivideo2 flow when
// GOLDMD_DEBUG is set. Full visibility into dispatch, sessions, image
// download/upload, key rotation, generation, polling and sending.
var av2DebugEnabled = av2InitDebug()

func av2InitDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOLDMD_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func av2Debug(stage string, fields map[string]any) {
	// DISABLED — silent no-op per owner request (zero console output)
	_ = stage
	_ = fields
}

func av2JSONCompact(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("%v", m)
	}
	return string(b)
}

// av2TruncStr trims a string to maxLen and appends an ellipsis if truncated.
func av2TruncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

func av2EntryIndexOrNeg(e *av2KeyEntry) int {
	if e == nil {
		return -1
	}
	return e.index
}

func av2ErrToStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func av2ImgbbURL() string {
	av2ImgbbOnce.Do(func() {
		if av2ImgbbKey == "" {
			av2ImgbbKey = "d415dbed2b70b808654b120fb0ba1915"
		}
	})
	return "https://api.imgbb.com/1/upload?key=" + av2ImgbbKey
}

var (
	av2ImageIntentRegex = regexp.MustCompile(`\b(photo|photos|pic|pics|picture|pictures|image|images|tasveer|tasveerein)\b`)
	av2YesRegex         = regexp.MustCompile(`(?i)^(yes|y|ok|okay|haan|han|sure|proceed|go)$`)
	av2CancelRegex      = regexp.MustCompile(`(?i)^(cancel|stop|no|nahi|band)$`)
	av2DigitRegex       = regexp.MustCompile(`\b([1-4])\b`)
	av2SecondsRegex     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:seconds?|secs?|s)\b`)
)

var av2NumberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4,
	"ek": 1, "do": 2, "teen": 3, "char": 4, "chaar": 4,
}

// ----- 20-key pool -----

type av2KeyEntry struct {
	index     int
	key       string
	restUntil int64 // unix nano; 0 = free
	busy      bool
	failCount int
	mu        sync.Mutex
}

var (
	av2KeyPool   []*av2KeyEntry
	av2ActiveIdx = 0
	av2PoolMu    sync.Mutex
	av2InitOnce  sync.Once
	av2CmdLabel  = "AIVIDEO2" // overridden by aivideo2 via init
)

func av2InitPool(keys []string, envPrefix string) {
	av2InitOnce.Do(func() {
		for i := 0; i < len(keys); i++ {
			key := strings.TrimSpace(os.Getenv(fmt.Sprintf("%s_%d", envPrefix, i+1)))
			if key == "" {
				key = keys[i]
			}
			av2KeyPool = append(av2KeyPool, &av2KeyEntry{index: i + 1, key: key})
		}
	})
}

func (e *av2KeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *av2KeyEntry) msLeft() int64 {
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

func (e *av2KeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := 0
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	} else {
		e.failCount++
		backoff := int64(av2DefaultRestMs)
		for i := 1; i < e.failCount; i++ {
			backoff *= 2
		}
		if backoff > av2MaxRestMs {
			backoff = av2MaxRestMs
		}
		ms = int(backoff)
	}
	e.failCount++
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

func av2GetNextAvailableKey() *av2KeyEntry {
	av2PoolMu.Lock()
	defer av2PoolMu.Unlock()
	scan := av2ActiveIdx
	for tries := 0; tries < len(av2KeyPool); tries++ {
		idx := scan % len(av2KeyPool)
		entry := av2KeyPool[idx]
		entry.mu.Lock()
		if entry.key == "" {
			entry.mu.Unlock()
			scan++
			av2ActiveIdx = scan
			continue
		}
		if entry.restUntil != 0 && entry.restUntil > time.Now().UnixNano() {
			entry.mu.Unlock()
			scan++
			av2ActiveIdx = scan
			continue
		}
		if entry.busy {
			entry.mu.Unlock()
			scan++
			continue
		}
		av2ActiveIdx = idx
		entry.busy = true
		entry.mu.Unlock()
		return entry
	}
	return nil
}

func av2GetPoolStatus() (hasBusy bool, totalMs, soonestMs int64) {
	for _, e := range av2KeyPool {
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

func av2ResolveNumFrames(promptText string) int {
	m := av2SecondsRegex.FindStringSubmatch(promptText)
	if m != nil {
		seconds, _ := strconv.ParseFloat(m[1], 64)
		if seconds <= 0 {
			seconds = float64(av2MaxFrames) / float64(av2FrameRate)
		}
		if seconds > av2MaxSeconds {
			seconds = av2MaxSeconds
		}
		n := int(((seconds * float64(av2FrameRate)) - 1) / 8)
		frames := 8*n + 1
		if frames > av2MaxFrames {
			frames = av2MaxFrames
		}
		if frames < av2MinFrames {
			frames = av2MinFrames
		}
		return frames
	}
	// AUTO: random length within valid range, max 16s cap
	minN := (av2MinFrames - 1) / 8
	maxN := (av2MaxFrames - 1) / 8
	if maxN <= minN {
		return av2MinFrames
	}
	n := minN + rand.Intn(maxN-minN+1)
	return 8*n + 1
}

func av2BuildStylizedPrompt(basePrompt string) string {
	trimmed := strings.TrimSpace(basePrompt)
	if trimmed == "" {
		return trimmed
	}
	if len(trimmed) > 380 {
		return trimmed
	}
	return trimmed + av2StyleSuffix
}

// ----- ImgBB upload -----

func av2UploadImageToURL(data []byte) (string, error) {
	// av2Debug("AIVIDEO2_IMGBB_UPLOAD", map[string]any{"bytes": len(data)})
	url := av2ImgbbURL()
	for attempt := 1; attempt <= 3; attempt++ {
		bb := bytes.NewBuffer(data)
		req, err := http.NewRequest("POST", url, bb)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		client := &http.Client{Timeout: time.Duration(av2ImgbbTimeoutMs) * time.Millisecond}
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

func av2GenerateWithKey(entry *av2KeyEntry, prompt string, imageUrls []string) ([]byte, int, error) {
	// av2Debug("AIVIDEO2_GEN_START", map[string]any{"keyIndex": entry.index, "imageCount": len(imageUrls), "promptLen": len(prompt)})
	payload := map[string]any{
		"model":           av2Model,
		"prompt":          prompt,
		"num_frames":      av2ResolveNumFrames(prompt),
		"frame_rate":      av2FrameRate,
		"negative_prompt": av2NegativePrompt,
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

	req, err := http.NewRequest("POST", av2CreateURL, bytes.NewReader(bodyBytes))
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
	createBody, _ := io.ReadAll(io.LimitReader(createResp.Body, av2BodyLimit))
	createResp.Body.Close()

	if av2RetryableStatuses[createResp.StatusCode] {
		retryAfter := 0
		if ra := createResp.Header.Get("Retry-After"); ra != "" {
			fmt.Sscanf(ra, "%d", &retryAfter)
		}
		return nil, retryAfter, fmt.Errorf("retryable_status_%d", createResp.StatusCode)
	}
	if createResp.StatusCode != 200 {
		// av2Debug("AIVIDEO2_GEN_CREATE_RESP", map[string]any{"keyIndex": entry.index, "status": createResp.StatusCode, "bodyLen": len(createBody), "bodySnippet": av2TruncStr(string(createBody), 300)})
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
		// av2Debug("AIVIDEO2_GEN_VIDEO_ID", map[string]any{"keyIndex": entry.index, "videoID": videoID})
	}
	if videoID == "" {
		return nil, 0, fmt.Errorf("no video_id in create response: %s", string(createBody))
	}

	// Polling loop — BOUNDED by av2PollMaxMs so a stuck task can never leak a
	// goroutine / memory forever (which trips the 400 MB memory watchdog and
	// causes the "safe restart" the user reported).
	pollClient := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(time.Duration(av2PollMaxMs) * time.Millisecond)
	for {
		if time.Now().After(deadline) {
			return nil, 0, fmt.Errorf("video generation timed out after %d minutes", av2PollMaxMs/60000)
		}
		time.Sleep(time.Duration(av2PollIntervalMs) * time.Millisecond)
		pollReq, perr := http.NewRequest("GET", av2PollURL+"?video_id="+videoID, nil)
		if perr != nil {
			continue
		}
		pollReq.Header.Set("Authorization", "Bearer "+entry.key)
		pollResp, perr := pollClient.Do(pollReq)
		if perr != nil {
			continue // transient — keep polling
		}
		pollBody, _ := io.ReadAll(io.LimitReader(pollResp.Body, av2BodyLimit))
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
		// av2Debug("AIVIDEO2_POLL", map[string]any{"keyIndex": entry.index, "videoID": videoID, "bodySnippet": av2TruncStr(string(pollBody), 200)})

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
			// av2Debug("AIVIDEO2_POLL_DONE", map[string]any{"keyIndex": entry.index, "videoID": videoID, "videoURL": videoURL, "status": status})
			// Download the video straight to a temp file (streamed) so we do NOT
			// load the whole clip into RAM — the bot runs under a 400 MB container
			// cap and loading big videos in memory was tripping the watchdog.
			dlClient := &http.Client{Timeout: 180 * time.Second}
			dlResp, derr := dlClient.Get(videoURL)
			if derr != nil {
				return nil, 0, fmt.Errorf("download video: %v", derr)
			}
			tmpFile, ferr := os.CreateTemp("", "av2-*.mp4")
			if ferr != nil {
				dlResp.Body.Close()
				return nil, 0, fmt.Errorf("create temp: %v", ferr)
			}
			written, cerr := io.Copy(tmpFile, io.LimitReader(dlResp.Body, 200*1024*1024))
			// av2Debug("AIVIDEO2_DL_DONE", map[string]any{"keyIndex": entry.index, "bytes": written, "tmpPath": tmpFile.Name(), "writeErr": cerr != nil})
			dlResp.Body.Close()
			tmpFile.Close()
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
			// av2Debug("AIVIDEO2_POLL_FAILED", map[string]any{"keyIndex": entry.index, "videoID": videoID, "bodySnippet": av2TruncStr(string(pollBody), 300)})
			errMsg := "unknown"
			if s, ok := pr.Error.(string); ok && s != "" {
				errMsg = s
			}
			return nil, 0, fmt.Errorf("generation failed (%s)", errMsg)
		}
		// any other status -> keep polling
	}
}

// av2GenerateVideo rotates keys and returns either video bytes or wait info.
func av2GenerateVideo(prompt string, imageUrls []string) ([]byte, int64, int64, bool, error) {
	configured := 0
	for _, e := range av2KeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("API KEY NOT FOUND")
	}

	for attempt := 0; attempt < len(av2KeyPool); attempt++ {
		// av2Debug("AIVIDEO2_KEY_ROTATE", map[string]any{"attempt": attempt})
		entry := av2GetNextAvailableKey()
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
			videoData, retryAfter, err = av2GenerateWithKey(entry, prompt, imageUrls)
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

	hasBusy, totalMs, soonestMs := av2GetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, int64(av2BusyPollMs), int64(av2BusyPollMs), true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}

// ----- shared generation runner (NO progress bar — simple static message) -----

func av2RunGenerationFlow(s SessionBridge, info types.MessageInfo, prompt string, imageUrls []string) {
	// av2Debug("AIVIDEO2_FLOW_START", map[string]any{"sender": info.Sender.String(), "imageCount": len(imageUrls), "promptLen": len(prompt)})
	// Recovery guard: a panic anywhere in the generation flow must NOT kill the
	// whole bot process (which is what causes the "safe restart" the user sees).
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\nSomething went wrong while creating the video. Please try again.")
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

	stylizedPrompt := av2BuildStylizedPrompt(prompt)

	for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); {
		videoData, waitMs, totalWaitMs, busyOnly, err := av2GenerateVideo(stylizedPrompt, imageUrls)
		// av2Debug("AIVIDEO2_GEN_RESULT", map[string]any{"sender": info.Sender.String(), "hasVideo": videoData != nil, "busyOnly": busyOnly, "waitMs": waitMs, "err": av2ErrToStr(err)})
		if err != nil {
			s.DeleteMessage(info, waitMsgID)
			if strings.Contains(strings.ToLower(err.Error()), "timed out") {
				// av2Debug("AIVIDEO2_TIMEOUT", map[string]any{"sender": info.Sender.String(), "err": av2ErrToStr(err)})
				s.Reply(info, av2TimeoutMsg)
			} else {
				s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
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
				// av2Debug("AIVIDEO2_SEND_FILE", map[string]any{"sender": info.Sender.String(), "tmpPath": tmpPath, "captionLen": len(caption)})
				sendErr := s.SendVideoFile(info, tmpPath, caption, nil, 0, 0, 0)
				os.Remove(tmpPath)
				if sendErr != nil {
					s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\nFailed to upload the video. Please try again.")
				}
			} else {
				s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\nVideo file was lost. Please try again.")
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

// av2SaveTempVideo writes video bytes to a temp file and returns its path.
func av2SaveTempVideo(data []byte) string {
	f, err := os.CreateTemp("", "av2-*.mp4")
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

type av2Session struct {
	mode          string // "awaiting_count" or "collecting"
	expectedCount int
	imageUrls     []string
	promptText    string
	nudgeTimer    *time.Timer
	expireTimer   *time.Timer
}

var (
	av2Sessions = make(map[string]*av2Session)
	av2SessMu   sync.Mutex
)

// av2DeviceSuffixRe strips the ":<device>" suffix from a JID.
// e.g. 923158930864:60@s.whatsapp.net -> 923158930864@s.whatsapp.net
// Precompiled once (RE2-safe, no lookahead) instead of recompiling
// on every message (which previously panicked because Go RE2 does
// NOT support Perl lookahead `(?=)`).
var av2DeviceSuffixRe = regexp.MustCompile(`:\d+@`)

func av2StripDevice(jid string) string {
	return av2DeviceSuffixRe.ReplaceAllString(jid, "@")
}

func av2SessionKey(info types.MessageInfo) string {
	chat := info.Chat.String()
	sender := info.Sender.String()
	if sender == "" {
		sender = chat
	}
	// strip :device suffix from both
	chat = av2StripDevice(chat)
	sender = av2StripDevice(sender)
	return chat + "::" + sender
}

// HasPendingAIVideo2Session reports whether a sender has an active session.
// Called by the main package's message handler to decide whether to forward
// image messages to the aivideo flow.
func HasPendingAIVideo2Session(sender string) bool {
	av2SessMu.Lock()
	defer av2SessMu.Unlock()
	// sender may be a bare JID; check all session keys that end with this sender
	for k, sess := range av2Sessions {
		parts := strings.SplitN(k, "::", 2)
		if len(parts) == 2 && (parts[1] == sender || parts[0] == sender) {
			if sess != nil {
				return true
			}
		}
	}
	return false
}

func av2DestroySession(key string) {
	av2SessMu.Lock()
	sess, ok := av2Sessions[key]
	delete(av2Sessions, key)
	av2SessMu.Unlock()
	if ok && sess != nil {
		if sess.nudgeTimer != nil {
			sess.nudgeTimer.Stop()
		}
		if sess.expireTimer != nil {
			sess.expireTimer.Stop()
		}
	}
}

func av2StartCountAskSession(s SessionBridge, info types.MessageInfo, key, extraText string) {
	av2DestroySession(key)
	av2SessMu.Lock()
	sess := &av2Session{mode: "awaiting_count", promptText: extraText}
	av2Sessions[key] = sess
	av2SessMu.Unlock()
	sess.expireTimer = time.AfterFunc(time.Duration(av2SessionMaxMs)*time.Millisecond, func() {
		av2ExpireSession(s, info, key)
	})
	s.Reply(info, "*WELCOME! LET'S MAKE A VIDEO*\n\n*HOW MANY IMAGES DO YOU WANT TO USE?*\n\n*TYPE ❮ 2 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 2 ❯ IMAGES*\n\n*TYPE ❮ 3 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 3 ❯ IMAGES*\n\n*TYPE ❮ 4 ❯ AND SEND HERE AI MAKE VIDEO FROM ❮ 4 ❯ IMAGES*\n\n*JUST TYPE NUMBER AND SEND HERE*\n\n*MAX ALLOWED ❮ "+strconv.Itoa(av2MaxCombineImages)+" ❯ IMAGES*\n\n*OPTIONS:*\n*1 IMAGE = MAKE VIDEO FROM 1 PHOTO*\n*2 TO "+strconv.Itoa(av2MaxCombineImages)+" IMAGES = COMBINE MULTIPLE PHOTOS INTO 1 VIDEO*\n\n*HOW TO REPLY:*\n*JUST TYPE A NUMBER LIKE 2*\n*OR TYPE A WORD LIKE TWO*\n\n🔰 *YOU HAVE ONLY 2 MINUTES 🔰*\n*SEND YOUR REPLY FAST. IF 2 MINUTES PASS, THE SYSTEM WILL STOP.*\n*THEN YOU WILL HAVE TO TYPE*\n*"+av2CmdLabel+" ❮ IMG COMBINING PROMPT ❯*\n*AGAIN TO START THE PROCESS*")
}

func av2StartCollectingSession(s SessionBridge, info types.MessageInfo, key string, count int, extraText string) {
	// av2Debug("AIVIDEO2_SESSION_COLLECT", map[string]any{"sender": info.Sender.String(), "expectedCount": count})
	av2DestroySession(key)
	av2SessMu.Lock()
	sess := &av2Session{mode: "collecting", expectedCount: count, promptText: extraText}
	av2Sessions[key] = sess
	av2SessMu.Unlock()
	sess.expireTimer = time.AfterFunc(time.Duration(av2SessionMaxMs)*time.Millisecond, func() {
		av2ExpireSession(s, info, key)
	})
	plural := "IMAGES"
	if count == 1 {
		plural = "IMAGE"
	}
	s.Reply(info, "*OK! PLEASE SEND "+strconv.Itoa(count)+" "+plural+" NOW*\n*AI WILL MAKE A VIDEO FOR YOU AFTER RECEIVE THEM ALL THE IMAGES*\n\n*PENDING :❯ "+strconv.Itoa(count)+" "+plural+"*\n\n🔰 *YOU HAVE ONLY 2 MINUTES 🔰*\n*PLEASE SEND THE IMAGES WITHIN 2 MINUTES.*\n*IF YOU DON'T SEND IN TIME, THE SYSTEM WILL STOP.*\n*THEN YOU WILL TYPE*\n*"+av2CmdLabel+" ❮ IMG COMBINING PROMPT ❯*\n*TO START THE IMAGE TO VIDEO CREATION PROCESS AGAIN*")
}

func av2ExpireSession(s SessionBridge, info types.MessageInfo, key string) {
	// This runs in a time.AfterFunc goroutine (NOT the event handler), so it
	// needs its own recover guard. A panic here (e.g. stale client during the
	// s.Reply below) would otherwise crash the whole bot process.
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	_, exists := func() (bool, bool) {
		av2SessMu.Lock()
		defer av2SessMu.Unlock()
		_, ok := av2Sessions[key]
		return ok, ok
	}()
	if !exists {
		return
	}
	av2DestroySession(key)
	s.Reply(info, "*2 MINUTES ARE LEFT PLEASE TYPE*\n*"+av2CmdLabel+" ❮ IMAGE COMBINING PROMPT ❯*\n*TO CREATE NEW VIDEOS BY COMBINING THE IMAGES*")
}

func av2BuildDefaultPrompt(count int, extraText string) string {
	cleaned := av2ImageIntentRegex.ReplaceAllString(extraText, "")
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

func av2ParseImageCount(text string) int {
	clean := strings.ToLower(strings.TrimSpace(text))
	if m := av2DigitRegex.FindStringSubmatch(clean); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	for _, w := range strings.Fields(clean) {
		w = regexp.MustCompile(`[^a-z]`).ReplaceAllString(w, "")
		if n, ok := av2NumberWords[w]; ok {
			return n
		}
	}
	return 0
}

func av2HasImageIntent(text string) bool {
	return av2ImageIntentRegex.MatchString(text)
}

// av2HandleSessionImage collects an incoming image into the session.
func av2HandleSessionImage(s SessionBridge, info types.MessageInfo, key string) bool {
	av2SessMu.Lock()
	sess, ok := av2Sessions[key]
	av2SessMu.Unlock()
	if !ok || sess == nil || sess.mode != "collecting" {
		return false
	}

	imgData, found := s.DownloadImage(info)
	// av2Debug("AIVIDEO2_IMG_DOWNLOAD", map[string]any{"sender": info.Sender.String(), "found": found, "bytes": len(imgData)})
	if !found || len(imgData) == 0 {
		s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\nCould not download the image. Please send it again.")
		return true
	}

	url, err := av2UploadImageToURL(imgData)
	if err != nil {
		s.Reply(info, "🔰 *"+av2CmdLabel+" COMMAND ERROR* 🔰\nImage upload failed. Please try again.")
		return true
	}

	sess.imageUrls = append(sess.imageUrls, url)
	received := len(sess.imageUrls)
	remaining := sess.expectedCount - received

	if remaining <= 0 {
		prompt := av2BuildDefaultPrompt(received, sess.promptText)
		urls := make([]string, received)
		copy(urls, sess.imageUrls)
		av2DestroySession(key)
		s.Reply(info, "*GOT ALL "+strconv.Itoa(received)+" IMAGES NOW AI COMBINING ALL IMAGES AND CREATING VIDEO*\n*PLEASE WAIT.....*")
		go av2RunGenerationFlow(s, info, prompt, urls)
		return true
	}

	s.Reply(info, "*IMAGE "+strconv.Itoa(received)+" BY "+strconv.Itoa(sess.expectedCount)+" RECIEVED 🔰*\n*REMAINING ❮ "+strconv.Itoa(remaining)+" ❯ MORE*\n*SEND ALL THE IMAGES TO START THE COMBINING IMAGES AND START THE CREATING VIDEO PROCESS*\n\n*OR TYPE ❮ YES ❯ AND SEND HERE IF YOU WANT TO MAKE VIDEO WITH ❮ "+strconv.Itoa(received)+" ❯ IMAGES*")
	return true
}

// av2HandleSessionText handles text replies during a session (count / YES / cancel).
func av2HandleSessionText(s SessionBridge, info types.MessageInfo, key, text string) bool {
	av2SessMu.Lock()
	sess, ok := av2Sessions[key]
	av2SessMu.Unlock()
	if !ok || sess == nil {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	if av2CancelRegex.MatchString(trimmed) {
		av2DestroySession(key)
		s.Reply(info, "🔰 Ok, cancelled. Send *."+strings.ToLower(av2CmdLabel)+"* again whenever you want to make a video.")
		return true
	}

	if sess.mode == "awaiting_count" {
		count := av2ParseImageCount(trimmed)
		if count < av2MinCombineImages || count > av2MaxCombineImages {
			s.Reply(info, "Please reply with a valid number between "+strconv.Itoa(av2MinCombineImages)+" and "+strconv.Itoa(av2MaxCombineImages)+" (e.g. 2 or two).")
			return true
		}
		av2StartCollectingSession(s, info, key, count, sess.promptText)
		return true
	}

	if sess.mode == "collecting" {
		if av2YesRegex.MatchString(trimmed) {
			received := len(sess.imageUrls)
			if received == 0 {
				s.Reply(info, "*YOU HAVE NOT SENT ANY IMAGE HERE PLEASE SEND THE IMAGE FIRST*")
				return true
			}
			prompt := av2BuildDefaultPrompt(received, sess.promptText)
			urls := make([]string, received)
			copy(urls, sess.imageUrls)
			av2DestroySession(key)
			s.Reply(info, "*CREATING VIDEO ❮ "+strconv.Itoa(received)+" ❯ IMAGES YOU SENT HERE......*")
			go av2RunGenerationFlow(s, info, prompt, urls)
			return true
		}
		// collecting mode: ignore normal text (waiting for images)
		return false
	}
	return false
}

// DispatchAIVideo2Incoming is called by the main message handler for every
// incoming message when a pending aivideo session exists for the sender.
// It routes image messages and text replies to the session handlers.
// Returns true if the message was consumed.
func DispatchAIVideo2Incoming(s SessionBridge, info types.MessageInfo, text string, hasImg bool) bool {
	// av2Debug("AIVIDEO2_DISPATCH", map[string]any{"sender": info.Sender.String(), "hasImg": hasImg, "textLen": len(text)})
	key := av2SessionKey(info)
	av2SessMu.Lock()
	_, exists := av2Sessions[key]
	av2SessMu.Unlock()
	// av2Debug("AIVIDEO2_DISPATCH_SESS", map[string]any{"sessionExists": exists})
	if !exists {
		return false
	}
	if hasImg {
		return av2HandleSessionImage(s, info, key)
	}
	if text != "" {
		return av2HandleSessionText(s, info, key, text)
	}
	return false
}

// ----- command handlers -----

func handleAIVideo2(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleAIVideo2Async(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func handleAIVideo2Async(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// av2Debug("AIVIDEO2_CMD", map[string]any{"sender": info.Sender.String(), "argsLen": len(args)})
	prompt := strings.TrimSpace(strings.Join(args, " "))

	if prompt == "" {
		s.Reply(info, "🔰 *AIVIDEO2 AI COMMAND INFO* 🔰\n\n*🔰 HOW TO USE - FULL STEPS 🔰*\n\n*STEP 1: WRITE .AIVIDEO2*\n*STEP 2: AFTER IT WRITE YOUR PROMPT (WHAT VIDEO YOU WANT)*\n*STEP 3: SEND THE COMMAND AND WAIT*\n\n*EXAMPLE LIKE THIS*\n*AIVIDEO2 CAT WAS EATING FISH*\n*AIVIDEO2 AEROPLANE WAS FLYING SKY*\n*AIVIDEO2 A MAN WAS DRINKING*\n*AIVIDEO2 ❮ VIDEO NAME ❯*\n*TYPE YOUR VIDEO NAME AND AI WILL CREATE A VIDEO*\n\n*NOTE: FOR PURE TEXT PROMPT NO NEED TO SEND ANY PHOTO*\n*YOU HAVE 3/4 IMAGES AND YOU WANT TO CREATE THE VIDEO*\n*PHOTOS -> VIDEO (IMAGE-TO-VIDEO / COMBINE MULTIPLE PHOTOS)*\n*COMMAND: .AIVIDEO2 I HAVE 2/3 PHOTOS PLEASE MAKE IT VIDEO*\n*COMMAND (WITHOUT COUNT) :❯ .AIVIDEO2 I HAVE PHOTOS PLEASE MAKE VIDEO*\n\n*🔰 IMPORTANT RULES 🔰*\n*1. WRITE YOUR PROMPT CLEARLY AFTER .AIVIDEO2*\n*2. YOU CAN WRITE COMMAND IN ENGLISH OR URDU*\n*3. USE ONLY 1 COMMAND AT A TIME*\n*4. VIDEO GENERATION HAS NO FIXED TIME LIMIT, PLEASE WAIT PATIENTLY*")
		return
	}

	key := av2SessionKey(info)

	if av2HasImageIntent(prompt) {
		count := av2ParseImageCount(prompt)
		if count > 0 {
			av2StartCollectingSession(s, info, key, count, prompt)
		} else {
			av2StartCountAskSession(s, info, key, prompt)
		}
		return
	}

	// Plain text-to-video
	av2RunGenerationFlow(s, info, prompt, nil)
}

func init() {
	av2InitPool(hardcodedAIVideo2Keys, "AVIDEO2_API_KEY")
	Register(Command{Name: "aivideo2", Category: "AI & MEDIA", Desc: "Generate an AI video (pool #2)", Run: handleAIVideo2})
}

// hardcodedAIVideo2Keys are defined in aivideo2_keys.go.
