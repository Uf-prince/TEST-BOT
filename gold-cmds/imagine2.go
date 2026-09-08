package goldcmds

// ============================================================================
// GOLD-MD — AI Text-to-Image Command (Mistral AI)
// File: imagine2.go
// ============================================================================
// COMMAND: .imagine2 <prompt>
//   e.g.   .imagine2 a cyberpunk city street at night, neon lights, rain
//
// Powered by Mistral AI Agents API with image_generation tool.
// Uses 3 API keys with round-robin rotation + rate-limit rest tracking.
//
// 3-KEY ROTATION SYSTEM:
//   3 Mistral API keys rotate (index-based round robin). If a key hits 429
//   rate-limit, it goes into "rest" mode with a timestamp. While resting,
//   it's skipped and the next available key is tried. If ALL keys are
//   resting, user sees a combined wait message.
//
// API keys from env: MISTRAL_API_KEY_1, MISTRAL_API_KEY_2, MISTRAL_API_KEY_3
//   Falls back to hardcoded keys if env not set.
//
// Simple static "CREATING IMAGE........" message (no progress bar edits,
// to keep the server fast). Message deleted when image is ready.
//
// Aliases: aiimage2, aiimg2, texttoimg2, aipic2, aiphoto2, bing2, txt2img2 (all Hidden)
//
// Debug logging controlled by GOLDMD_DEBUG env flag (default off).
// ============================================================================

import (
	"bytes"
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
	mistralBase          = "https://api.mistral.ai/v1"
	mistralAgentsURL     = mistralBase + "/agents"
	mistralConvURL       = mistralBase + "/conversations"
	mistralModel         = "mistral-medium-latest"
	mistralDefaultRestMs = 60 * 1000
)

var hardcodedMistralKeys = []string{
	"f2Vp0UxTJuLD2C2tTaWYNTnj3tSSNW49",
	"cWlOUuOBIqweIWNhRecyXMCQb3lx0xvL",
	"gmiwAxVbTLWAd7LkRLrxnwAugjD9hmPL",
}

type mistralKeyEntry struct {
	index     int
	key       string
	agentID   string
	restUntil int64 // unix nano; 0 = free
	mu        sync.Mutex
}

var (
	mistralKeyPool  []*mistralKeyEntry
	mistralRRMu     sync.Mutex
	mistralRRPtr    = 0
	mistralInitOnce sync.Once
)

func initMistralPool() {
	mistralInitOnce.Do(func() {
		for i := 0; i < 3; i++ {
			key := os.Getenv(fmt.Sprintf("MISTRAL_API_KEY_%d", i+1))
			if key == "" && i < len(hardcodedMistralKeys) {
				key = hardcodedMistralKeys[i]
			}
			mistralKeyPool = append(mistralKeyPool, &mistralKeyEntry{
				index: i + 1,
				key:   key,
			})
		}
	})
}

func (e *mistralKeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *mistralKeyEntry) msLeft() int64 {
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

func (e *mistralKeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := mistralDefaultRestMs
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	}
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

// resolveAgent creates a Mistral agent (cached per key) for image generation.
func (e *mistralKeyEntry) resolveAgent() (string, error) {
	e.mu.Lock()
	if e.agentID != "" {
		id := e.agentID
		e.mu.Unlock()
		return id, nil
	}
	e.mu.Unlock()

	body := map[string]any{
		"model":        mistralModel,
		"name":         "Image Generation Agent",
		"description":  "Agent used to generate images.",
		"instructions": "You must ALWAYS immediately call the image_generation tool to create an image for every user prompt, no matter how short, vague, or ambiguous it is. NEVER ask clarifying questions. NEVER reply with only text. If details like style, brand, or angle are not specified, pick sensible creative defaults yourself and generate the image right away. Every single response must include a generated image.",
		"tools":        []map[string]string{{"type": "image_generation"}},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", mistralAgentsURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("agent create status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.ID == "" {
		return "", fmt.Errorf("no agent id in response: %s", string(respBody))
	}

	e.mu.Lock()
	e.agentID = result.ID
	e.mu.Unlock()
	return result.ID, nil
}

// tryGenerateWithKey attempts to generate an image with a specific key.
// Returns (imageBytes, retryAfterSec, error). retryAfterSec > 0 means 429.
func (e *mistralKeyEntry) tryGenerate(prompt string) ([]byte, int, error) {
	agentID, err := e.resolveAgent()
	if err != nil {
		return nil, 0, err
	}

	body := map[string]any{
		"agent_id": agentID,
		"inputs":   prompt,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", mistralConvURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		retryAfter := 0
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			fmt.Sscanf(ra, "%d", &retryAfter)
		}
		return nil, retryAfter, fmt.Errorf("rate_limited_429")
	}
	if resp.StatusCode == 404 {
		e.mu.Lock()
		e.agentID = ""
		e.mu.Unlock()
		return nil, 0, fmt.Errorf("agent_404_invalid")
	}

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("conv status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var convResp struct {
		Outputs []struct {
			Content []struct {
				Type   string `json:"type"`
				FileID string `json:"file_id"`
				Text   string `json:"text"`
			} `json:"content"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(respBody, &convResp); err != nil {
		return nil, 0, fmt.Errorf("parse conv: %v body=%s", err, string(respBody))
	}

	var fileID string
	for _, out := range convResp.Outputs {
		for _, chunk := range out.Content {
			if chunk.Type == "tool_file" && chunk.FileID != "" {
				fileID = chunk.FileID
				break
			}
		}
		if fileID != "" {
			break
		}
	}

	if fileID == "" {
		return nil, 0, fmt.Errorf("no image file in response (agent may have replied with text only)")
	}

	// Download the file
	fileReq, _ := http.NewRequest("GET", mistralBase+"/files/"+fileID+"/content", nil)
	fileReq.Header.Set("Authorization", "Bearer "+e.key)
	fileClient := &http.Client{Timeout: 30 * time.Second}
	fileResp, err := fileClient.Do(fileReq)
	if err != nil {
		return nil, 0, fmt.Errorf("file download: %v", err)
	}
	defer fileResp.Body.Close()
	imgData, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("file read: %v", err)
	}
	if len(imgData) == 0 {
		return nil, 0, fmt.Errorf("downloaded empty image")
	}
	return imgData, 0, nil
}

// mistralGetNextAvailableKey returns the next non-resting key with a key configured.
func mistralGetNextAvailableKey() *mistralKeyEntry {
	mistralRRMu.Lock()
	defer mistralRRMu.Unlock()
	for tries := 0; tries < len(mistralKeyPool); tries++ {
		entry := mistralKeyPool[mistralRRPtr%len(mistralKeyPool)]
		mistralRRPtr++
		if entry.key == "" {
			continue
		}
		if !entry.isResting() {
			return entry
		}
	}
	return nil
}

// mistralGetRestSummary returns combined wait info when all keys are resting.
func mistralGetRestSummary() (totalMs, soonestMs int64) {
	for _, e := range mistralKeyPool {
		if e.key == "" {
			continue
		}
		left := e.msLeft()
		totalMs += left
		if soonestMs == 0 || left < soonestMs {
			soonestMs = left
		}
	}
	// Add 20s display buffer
	totalMs += 20 * 1000
	return
}

func formatWaitMs(ms int64) string {
	totalSec := (ms + 999) / 1000
	mm := totalSec / 60
	ss := totalSec % 60
	return fmt.Sprintf("❮ %02dm : %02ds ❯", mm, ss)
}

// mistralGenerateImage is the main entry: rotates keys, handles rest, returns image or wait info.
func mistralGenerateImage(prompt string) ([]byte, int64, int64, error) {
	initMistralPool()

	configured := 0
	for _, e := range mistralKeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, fmt.Errorf("no MISTRAL_API_KEY configured")
	}

	for attempt := 0; attempt < len(mistralKeyPool); attempt++ {
		entry := mistralGetNextAvailableKey()
		if entry == nil {
			break // all resting
		}
		imgData, retryAfter, err := entry.tryGenerate(prompt)
		if err == nil {
			return imgData, 0, 0, nil
		}
		if retryAfter > 0 || strings.Contains(err.Error(), "rate_limited_429") {
			entry.markResting(retryAfter)
			continue
		}
		if strings.Contains(err.Error(), "agent_404_invalid") {
			continue
		}
		return nil, 0, 0, fmt.Errorf("mistral key #%d: %v", entry.index, err)
	}

	// All keys resting
	totalMs, soonestMs := mistralGetRestSummary()
	return nil, soonestMs, totalMs, nil
}

func handleImagine2(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleImagine2Async(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func handleImagine2Async(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	prompt := strings.TrimSpace(strings.Join(args, " "))

	if prompt == "" {
		s.Reply(info, "👑 *TEXT TO IMAGE GUIDE* 👑\n\n*CREATE IMAGE USING TEXT*\n\n*TYPE LIKE THIS*\n*"+prefix+"IMAGINE2 ❮ TEXT PROMPT ❯*\n\n*EXAMPLE PROMPT TEXT LIKE.....*\n\n*IMAGINE2 A BEAUTIFUL SPORTS CAR ON THE MOUNTAIN ROAD*\n\n*IMAGINE2 A CAT AND THIS EATING THE FISH*\n\n*A BEAUTIFUL JUNGLE OF BEAUTIFUL TREES*\n\n*TYPE LIKE THIS AND AI WILL CREATE AN IMAGE FOR YOU* 👑")
		return
	}

	// Simple static message — no progress bar edits (keeps server fast)
	waitMsgID := s.ReplyWithID(info, "*CREATING IMAGE........*")

	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
		imgData, soonestMs, totalMs, err := mistralGenerateImage(prompt)
		if err != nil {
			s.DeleteMessage(info, waitMsgID)
			s.Reply(info, "👑 *IMAGINE2 COMMAND ERROR* 👑\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if imgData != nil {
			// Done — delete wait message and send the result
			s.DeleteMessage(info, waitMsgID)
			caption := "*AI CREATED IMAGE FOR THIS TEXT* 👇\n*" + strings.ToUpper(prompt) + "*"
			if sendErr := s.SendImage(info, imgData, caption); sendErr != nil {
				s.Reply(info, "👑 *IMAGINE2 COMMAND ERROR* 👑\nFailed to send image: "+sendErr.Error())
			}
			return
		}

		// All keys resting — edit the wait message with the wait time, sleep, then retry
		s.EditMessage(info, waitMsgID, fmt.Sprintf("*PLEASE WAIT IMAGE CREATING SERVER ARE BUSY NOW PLEASE WAIT %s TO CREATE NEW IMAGES*", formatWaitMs(totalMs)))

		waitDuration := time.Duration(soonestMs+500) * time.Millisecond
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
	Register(Command{Name: "imagine2", Category: "AI & MEDIA", Desc: "AI text-to-image (Mistral)", Run: handleImagine2})
	Register(Command{Name: "aiimage2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "aiimg2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "texttoimg2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "aipic2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "aiphoto2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "bing2", Hidden: true, Run: handleImagine2})
	Register(Command{Name: "txt2img2", Hidden: true, Run: handleImagine2})
}
