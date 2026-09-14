package goldcmds

// ============================================================================
// GOLD-MD — AI Text-to-Image Command (Agnes AI)
// File: imagine3.go
// ============================================================================
// COMMAND: .imagine3 <prompt>
//   e.g.   .imagine3 a cyberpunk city street at night, neon lights, rain
//
// Powered by Agnes AI (agnes-image-2.1-flash) — pure text-to-image.
// Uses 10 API keys with SEQUENTIAL FILL system (not round-robin):
//   - Only 1 key is "active" at a time. All requests go to it until its
//     1-minute window hits 15 requests. Then it's marked "full" and the
//     next free key becomes active instantly.
//   - Busy lock: if active key is concurrently busy, try next key for that
//     scan only (active pointer doesn't move permanently).
//   - Rate-limit rest: if Agnes returns 429/transient error, key goes into
//     exponential backoff rest (60s -> 120s -> 240s ... max 5min).
//
// API keys from env: IMAGINE3_API_KEY_1 ... IMAGINE3_API_KEY_10
//   Falls back to hardcoded keys if env not set.
//
//
// Simple static "CREATING IMAGE........" message (no progress bar edits,
// to keep the server fast). Message deleted when image is ready.
//
// Debug logging controlled by GOLDMD_DEBUG env flag (default off).
// ============================================================================

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	imagine3GenURL       = "https://apihub.agnes-ai.com/v1/images/generations"
	imagine3Model        = "agnes-image-2.1-flash"
	imagine3Size         = "2K"
	imagine3Ratio        = "1:1"
	imagine3DefaultRest  = 60 * 1000
	imagine3MaxRest      = 5 * 60 * 1000
	imagine3MaxReqPerKey = 15
	imagine3ReqWindowMs  = 70 * 1000
	imagine3BusyPollMs   = 4000
)

var hardcodedImagine3Keys = []string{
	"sk-wVDBthraYMa0jz3cMhBTF4BFVdSmEPngShWSPmyBLTTSKTKo",
	"sk-bt0qf3bmyzZXJ0iAHSZSkMYPXtuFusfIR4ydAMrlyqRETp0G",
	"sk-mI5uXfd87p90T8c2dct8j39OxXjqAY1AiHM4xZDTfoYjOGfP",
	"sk-sRd1vEHGhvWw3pCwXaGJU82TfGHvHxvU4OpGeWAMZ27N8ZSS",
	"sk-YaPRl4VI0Qp3o77Pcy9kBdLvDhBzmc9vqLK37DroG9BLvmEO",
	"sk-MJLBPbyylTfk04MhN4QjAUvs8FmYsvXbEIbBNC7fe4q41dVE",
	"sk-EJYhaIwmYwL0dWqz3uevAwfTUksyMWRYZuwbL9moy6yYBadQ",
	"sk-VITDWowsXnv65ut0ApPSFA0TPAaE3poMA7ClPpSQYJk79352",
	"sk-O2OyLozI3SnqlyVgldDGrFIbLlojlmQlbAVAtpGn6JytMBP8",
	"sk-hdYzAbCaW2IBt10dj41ni19JpBr7STUCfV3oS2WZoQ9E6n3A",
}

var imagine3Seed = strings.TrimSpace(os.Getenv("IMAGINE3_SEED"))

var imagine3RetryableStatuses = map[int]bool{
	408: true, 420: true, 429: true, 500: true, 502: true, 503: true, 504: true, 520: true, 522: true, 524: true,
}

type imagine3KeyEntry struct {
	index        int
	key          string
	restUntil    int64 // unix nano; 0 = free
	busy         bool
	failCount    int
	requestCount int
	windowStart  int64 // unix nano; 0 = window not started
	mu           sync.Mutex
}

var (
	imagine3KeyPool   []*imagine3KeyEntry
	imagine3ActiveIdx = 0
	imagine3PoolMu    sync.Mutex
	imagine3InitOnce  sync.Once
)

func initImagine3Pool() {
	imagine3InitOnce.Do(func() {
		for i := 0; i < 10; i++ {
			key := os.Getenv(fmt.Sprintf("IMAGINE3_API_KEY_%d", i+1))
			if key == "" && i < len(hardcodedImagine3Keys) {
				key = hardcodedImagine3Keys[i]
			}
			imagine3KeyPool = append(imagine3KeyPool, &imagine3KeyEntry{
				index: i + 1,
				key:   key,
			})
		}
	})
}

func (e *imagine3KeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *imagine3KeyEntry) msLeft() int64 {
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

// noteCapIfNeeded resets the request window if expired, and marks the key
// as "full" (resting) if it hit the 15-request cap within the current window.
func (e *imagine3KeyEntry) noteCapIfNeeded() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixNano()
	if e.windowStart != 0 && (now-e.windowStart)/int64(time.Millisecond) >= imagine3ReqWindowMs {
		e.requestCount = 0
		e.windowStart = 0
	}
	if e.windowStart != 0 && e.requestCount >= imagine3MaxReqPerKey {
		capUntil := e.windowStart + int64(imagine3ReqWindowMs)*int64(time.Millisecond)
		if e.restUntil == 0 || capUntil > e.restUntil {
			e.restUntil = capUntil
		}
	}
}

func (e *imagine3KeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := 0
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	} else {
		e.failCount++
		// exponential backoff: 60s * 2^(failCount-1), capped at 5min
		backoff := int64(imagine3DefaultRest)
		for i := 1; i < e.failCount; i++ {
			backoff *= 2
		}
		if backoff > imagine3MaxRest {
			backoff = imagine3MaxRest
		}
		ms = int(backoff)
	}
	e.failCount++
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

// getNextAvailableKey finds the next free key (sticky sequential fill).
func imagine3GetNextAvailableKey() *imagine3KeyEntry {
	imagine3PoolMu.Lock()
	defer imagine3PoolMu.Unlock()
	scan := imagine3ActiveIdx
	for tries := 0; tries < len(imagine3KeyPool); tries++ {
		idx := scan % len(imagine3KeyPool)
		entry := imagine3KeyPool[idx]
		entry.noteCapIfNeeded()

		entry.mu.Lock()
		if entry.key == "" {
			entry.mu.Unlock()
			scan++
			imagine3ActiveIdx = scan
			continue
		}
		if entry.restUntil != 0 && entry.restUntil > time.Now().UnixNano() {
			entry.mu.Unlock()
			scan++
			imagine3ActiveIdx = scan
			continue
		}
		if entry.busy {
			entry.mu.Unlock()
			scan++
			continue
		}
		// Available — lock it
		imagine3ActiveIdx = idx
		entry.busy = true
		if entry.windowStart == 0 {
			entry.windowStart = time.Now().UnixNano()
		}
		entry.requestCount++
		entry.mu.Unlock()
		return entry
	}
	return nil
}

// getPoolStatus returns combined wait info when no key is free.
func imagine3GetPoolStatus() (hasBusy bool, totalMs, soonestMs int64) {
	for _, e := range imagine3KeyPool {
		if e.key == "" {
			continue
		}
		e.noteCapIfNeeded()
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

// generateWithKey calls Agnes image generation API with a specific key.
func (e *imagine3KeyEntry) generateWithKey(prompt string) ([]byte, int, error) {
	body := map[string]any{
		"model":      imagine3Model,
		"prompt":     prompt,
		"size":       imagine3Size,
		"ratio":      imagine3Ratio,
		"n":          1,
		"extra_body": map[string]string{"response_format": "url"},
	}
	if imagine3Seed != "" {
		var seedNum int
		if n, _ := fmt.Sscanf(imagine3Seed, "%d", &seedNum); n == 1 {
			body["seed"] = seedNum
		}
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", imagine3GenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // no time limit
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if imagine3RetryableStatuses[resp.StatusCode] {
		retryAfter := 0
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			fmt.Sscanf(ra, "%d", &retryAfter)
		}
		return nil, retryAfter, fmt.Errorf("retryable_status_%d", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("agnes status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		URL    string `json:"url"`
		B64    string `json:"b64_json"`
		Images []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"images"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, 0, fmt.Errorf("parse response: %v body=%s", err, string(respBody))
	}

	resultURL := ""
	resultB64 := ""
	if len(result.Data) > 0 {
		resultURL = result.Data[0].URL
		resultB64 = result.Data[0].B64JSON
	}
	if resultURL == "" {
		resultURL = result.URL
	}
	if resultB64 == "" {
		resultB64 = result.B64
	}
	if resultURL == "" && len(result.Images) > 0 {
		resultURL = result.Images[0].URL
		resultB64 = result.Images[0].B64JSON
	}

	if resultURL != "" {
		fileResp, err := http.Get(resultURL)
		if err != nil {
			return nil, 0, fmt.Errorf("download image: %v", err)
		}
		defer fileResp.Body.Close()
		imgData, err := io.ReadAll(fileResp.Body)
		if err != nil {
			return nil, 0, fmt.Errorf("read image: %v", err)
		}
		if len(imgData) == 0 {
			return nil, 0, fmt.Errorf("downloaded empty image")
		}
		return imgData, 0, nil
	}
	if resultB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(resultB64)
		if err != nil {
			return nil, 0, fmt.Errorf("decode b64: %v", err)
		}
		if len(decoded) == 0 {
			return nil, 0, fmt.Errorf("decoded empty b64 image")
		}
		return decoded, 0, nil
	}
	return nil, 0, fmt.Errorf("no url/b64 in response: %s", string(respBody))
}

// imagine3GenerateImage is the main entry: 10-key rotation + busy lock + rest.
// Returns (imageBytes, waitMs, totalWaitMs, busyOnly, error).
func imagine3GenerateImage(prompt string) ([]byte, int64, int64, bool, error) {
	initImagine3Pool()

	configured := 0
	for _, e := range imagine3KeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("no IMAGINE3_API_KEY configured")
	}

	for attempt := 0; attempt < len(imagine3KeyPool); attempt++ {
		entry := imagine3GetNextAvailableKey()
		if entry == nil {
			break
		}
		imgData, retryAfter, err := entry.generateWithKey(prompt)
		entry.mu.Lock()
		entry.busy = false // release lock
		entry.mu.Unlock()

		if err == nil {
			entry.mu.Lock()
			entry.failCount = 0
			entry.mu.Unlock()
			return imgData, 0, 0, false, nil
		}
		errStr := err.Error()
		if retryAfter > 0 || strings.Contains(errStr, "retryable_status_") {
			entry.markResting(retryAfter)
			continue
		}
		return nil, 0, 0, false, fmt.Errorf("image generate fail (key #%d): %v", entry.index, err)
	}

	// No free key
	hasBusy, totalMs, soonestMs := imagine3GetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, imagine3BusyPollMs, imagine3BusyPollMs, true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}

const imagine3ModeNotice = "*IMAGINE3 MODE: SEND IMAGES IN FULL HD QUALITY*"

func handleImagine3(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleImagine3Async(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func handleImagine3Async(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	prompt := strings.TrimSpace(strings.Join(args, " "))

	if prompt == "" {
		s.Reply(info, "*🔰 IMAGINE3 AI COMMAND INFO 🔰*\n\n*🔰 HOW TO USE - FULL STEPS 🔰*\n\n*STEP 1: WRITE .IMAGINE3*\n*STEP 2: AFTER IT WRITE YOUR PROMPT (WHAT IMAGE YOU WANT)*\n*STEP 3: SEND THE COMMAND AND WAIT*\n\n*EXAMPLE LIKE THIS*\n*IMAGINE3 A CAT WAS FLYING IN SKY*\n*IMAGINE3 A LION SITTING IN JUNGLE*\n*IMAGINE3 FUTURISTIC CITY AT NIGHT NEON LIGHTS*\n*IMAGINE3 ❮ IMAGE DESCRIPTION ❯*\n*TYPE YOUR IMAGE DESCRIPTION AND AI WILL CREATE IT*\n\n*NOTE: SIRF TEXT PROMPT SE IMAGE BANTI HAI, KOI PHOTO BHEJNE/REPLY KI ZAROORAT NAHI*\n\n*🔰 IMPORTANT RULES 🔰*\n*1. WRITE YOUR PROMPT CLEARLY AFTER .IMAGINE3*\n*2. YOU CAN WRITE COMMAND IN ENGLISH OR URDU*\n*3. USE ONLY 1 COMMAND AT A TIME*\n*4. IMAGE GENERATION HAS NO FIXED TIME LIMIT, PLEASE WAIT PATIENTLY*")
		return
	}

	// Simple static message — no progress bar edits (keeps server fast)
	waitMsgID := s.ReplyWithID(info, "*CREATING IMAGE........*")

	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
		imgData, waitMs, totalWaitMs, busyOnly, err := imagine3GenerateImage(prompt)
		if err != nil {
			s.DeleteMessage(info, waitMsgID)
			s.Reply(info, "🔰 *IMAGINE3 COMMAND ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*\n"+imagine3ModeNotice)
			return
		}
		if imgData != nil {
			// Done — delete wait message and send the result
			s.DeleteMessage(info, waitMsgID)
			caption := "*IMAGINE3 AI CREATED IMAGE*\n*YOUR PROMPT TEXT IS* 🔰\n\n" + strings.ToUpper(prompt)
			if sendErr := s.SendImage(info, imgData, caption); sendErr != nil {
				s.Reply(info, "🔰 *IMAGINE3 COMMAND ERROR* 🔰\nFailed to send image: "+sendErr.Error())
			}
			return
		}

		// All keys busy/resting — edit wait message with wait info, sleep, retry
		var waitText string
		if busyOnly {
			waitText = "*PLEASE WAIT IMAGINE3 AI SERVER ARE BUSY NOW, RETRYING SHORTLY...*\n" + imagine3ModeNotice
		} else {
			waitText = fmt.Sprintf("*PLEASE WAIT IMAGINE3 AI SERVER ARE BUSY NOW PLEASE WAIT %s TO CREATE NEW IMAGES*\n%s", formatWaitMs(totalWaitMs), imagine3ModeNotice)
		}
		s.EditMessage(info, waitMsgID, waitText)

		waitDuration := time.Duration(waitMs+500) * time.Millisecond
		if waitDuration < 1*time.Second {
			waitDuration = 1 * time.Second
		}
		time.Sleep(waitDuration)

		// Switch back to simple creating message before next attempt
		s.EditMessage(info, waitMsgID, "*CREATING IMAGE........*")
	}
	s.DeleteMessage(info, waitMsgID)
	s.Reply(info, "*TRY AGAIN LATER*")
}

func init() {
	Register(Command{Name: "imagine3", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO MAKE A THIRD STYLE AI IMAGE FROM YOUR TEXT. IT GIVES ONE MORE DIFFERENT STYLE PICTURE.", Run: handleImagine3})
}
