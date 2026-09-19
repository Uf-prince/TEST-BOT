package goldcmds

// ============================================================================
// GOLD-MD — 1000 NAME LOGO DESIGNS (.logo1 ... .logo1000)
// File: logo1000.go
// ============================================================================
// OWNER ORDER (exact spec):
//   .logo                → sirf LIST message (CREATE TEXT TO IMAGE ( NAME LOGO )
//                          + {prefix}LOGO1 ❮ YOUR NAME ❯ ... 1000 tak)
//   .logoN <name>        → us design ka dhamakedar name-logo image
//
// HAR DESIGN ALAG: 100 hand-crafted base concepts × 10 text treatments ×
// 10 lighting moods = 1000 unique dhamakedar prompts. Koi do designs same
// nahi — base+text pair har N ke liye unique hai.
//
// API: Agnes image generation (agnes-image-2.1-flash, 2K, 1:1) — same as
// .imagine3. Keys: 15-key pool (OWNER ORDER):
//   5 keys from .aivideo   (hardcodedAIVideoKeys   first 5)
//   5 keys from .aivideo2  (hardcodedAIVideo2Keys  first 5)
//   5 keys from .imagine3  (hardcodedImagine3Keys  first 5)
// SAME indexing rotation + rest system as imagine3: sticky sequential fill,
// 15-req/70s window per key, exponential backoff rest (60s→max 5min),
// busy-lock per key. Env override: LOGO_API_KEY_1 ... LOGO_API_KEY_15.
//
// VISIBILITY (owner order): .logo VISIBLE (menu, display name .LOGO).
// logo1..logo1000 register MAIN-package me as hiddenCommands — .menu me
// nahi dikhte. Is file me SIRF .logo visible command hai; .logo likhne par
// fancy boxed menu banta hai (ShowLogoMenu → manager.go CmdLogoMenu).
//
// Waiting message ALWAYS delete hota hai (success / error / timeout —
// har path pe DeleteMessage).
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
)

// ── API config (same endpoint/model as imagine3) ──────────────────────────
const (
	logoGenURL       = "https://apihub.agnes-ai.com/v1/images/generations"
	logoModel        = "agnes-image-2.1-flash"
	logoSize         = "2K"
	logoRatio        = "1:1"
	logoDefaultRest  = 60 * 1000
	logoMaxRest      = 5 * 60 * 1000
	logoMaxReqPerKey = 15
	logoReqWindowMs  = 70 * 1000
	logoBusyPollMs   = 4000
	logoTimeout      = 120 * time.Second
	logoMaxNameLen   = 40
	LogoCount        = 1000
)

var logoRetryableStatuses = map[int]bool{
	408: true, 420: true, 429: true, 500: true, 502: true, 503: true, 504: true, 520: true, 522: true, 524: true,
}

// ── 15-key pool (5×aivideo + 5×aivideo2 + 5×imagine3 — owner order) ───────
func logoKeyList() []string {
	keys := make([]string, 0, 15)
	keys = append(keys, hardcodedAIVideoKeys[:5]...)  // .aivideo first 5
	keys = append(keys, hardcodedAIVideo2Keys[:5]...) // .aivideo2 first 5
	keys = append(keys, hardcodedImagine3Keys[:5]...) // .imagine3 first 5
	return keys
}

type logoKeyEntry struct {
	index        int
	key          string
	restUntil    int64 // unix nano; 0 = free
	busy         bool
	failCount    int
	requestCount int
	windowStart  int64
	mu           sync.Mutex
}

var (
	logoKeyPool   []*logoKeyEntry
	logoActiveIdx = 0
	logoPoolMu    sync.Mutex
	logoInitOnce  sync.Once
)

func initLogoPool() {
	logoInitOnce.Do(func() {
		hard := logoKeyList()
		for i := 0; i < 15; i++ {
			key := os.Getenv(fmt.Sprintf("LOGO_API_KEY_%d", i+1))
			if key == "" && i < len(hard) {
				key = hard[i]
			}
			logoKeyPool = append(logoKeyPool, &logoKeyEntry{index: i + 1, key: key})
		}
	})
}

func (e *logoKeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *logoKeyEntry) msLeft() int64 {
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
// as "full" (resting) if it hit the 15-request cap within the window.
func (e *logoKeyEntry) noteCapIfNeeded() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixNano()
	if e.windowStart != 0 && (now-e.windowStart)/int64(time.Millisecond) >= logoReqWindowMs {
		e.requestCount = 0
		e.windowStart = 0
	}
	if e.windowStart != 0 && e.requestCount >= logoMaxReqPerKey {
		capUntil := e.windowStart + int64(logoReqWindowMs)*int64(time.Millisecond)
		if e.restUntil == 0 || capUntil > e.restUntil {
			e.restUntil = capUntil
		}
	}
}

func (e *logoKeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := 0
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	} else {
		e.failCount++
		backoff := int64(logoDefaultRest)
		for i := 1; i < e.failCount; i++ {
			backoff *= 2
		}
		if backoff > logoMaxRest {
			backoff = logoMaxRest
		}
		ms = int(backoff)
	}
	e.failCount++
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

// logoGetNextAvailableKey — sticky sequential fill (same as imagine3).
func logoGetNextAvailableKey() *logoKeyEntry {
	logoPoolMu.Lock()
	defer logoPoolMu.Unlock()
	scan := logoActiveIdx
	for tries := 0; tries < len(logoKeyPool); tries++ {
		idx := scan % len(logoKeyPool)
		entry := logoKeyPool[idx]
		entry.noteCapIfNeeded()

		entry.mu.Lock()
		if entry.key == "" {
			entry.mu.Unlock()
			scan++
			logoActiveIdx = scan
			continue
		}
		if entry.restUntil != 0 && entry.restUntil > time.Now().UnixNano() {
			entry.mu.Unlock()
			scan++
			logoActiveIdx = scan
			continue
		}
		if entry.busy {
			entry.mu.Unlock()
			scan++
			continue
		}
		logoActiveIdx = idx
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

// logoGetPoolStatus returns combined wait info when no key is free.
func logoGetPoolStatus() (hasBusy bool, totalMs, soonestMs int64) {
	for _, e := range logoKeyPool {
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
func (e *logoKeyEntry) generateWithKey(prompt string) ([]byte, int, error) {
	body := map[string]any{
		"model":      logoModel,
		"prompt":     prompt,
		"size":       logoSize,
		"ratio":      logoRatio,
		"n":          1,
		"extra_body": map[string]string{"response_format": "url"},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", logoGenURL, bytes.NewReader(bodyBytes))
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

	if logoRetryableStatuses[resp.StatusCode] {
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

// LogoGenerateImage — main entry: 15-key rotation + busy lock + rest.
// Returns (imageBytes, waitMs, totalWaitMs, busyOnly, error).
func LogoGenerateImage(prompt string) ([]byte, int64, int64, bool, error) {
	initLogoPool()

	configured := 0
	for _, e := range logoKeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("no LOGO_API_KEY configured")
	}

	for attempt := 0; attempt < len(logoKeyPool); attempt++ {
		entry := logoGetNextAvailableKey()
		if entry == nil {
			break
		}
		imgData, retryAfter, err := entry.generateWithKey(prompt)
		entry.mu.Lock()
		entry.busy = false
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
		return nil, 0, 0, false, fmt.Errorf("logo generate fail (key #%d): %v", entry.index, err)
	}

	hasBusy, totalMs, soonestMs := logoGetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, logoBusyPollMs, logoBusyPollMs, true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}
