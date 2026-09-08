package goldcmds

// ============================================================================
// GOLD-MD — AI Text-to-Image Command (Pollinations.ai)
// File: imagine.go
// ============================================================================
// COMMAND: .imagine <prompt>
//   e.g.   .imagine a cyberpunk city street at night, neon lights, rain
//
// Powered by Pollinations.ai — a simple HTTP GET returns the image directly.
// No API key required (free tier). Optional POLLINATIONS_API_KEY env for
// higher rate limits.
//
// FEATURES:
//   - Model fallback: flux -> turbo -> gptimage (if one model errors, try next)
//   - Flags: --turbo (fastest), --gpt (best text understanding), --hd (1024px),
//     --random (skip fixed seed, different image each time)
//   - Fixed seed by default (SHA-256 hash of prompt) — same prompt = same image
//   - Simple static "CREATING IMAGE........" message (no progress bar edits,
//     to keep the server fast). Message deleted when image is ready.
//
// Aliases: aiimage, aiimg, texttoimg, aipic, aiphoto, bing, txt2img (all Hidden)
//
// Debug logging controlled by GOLDMD_DEBUG env flag (default off).
// ============================================================================

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	pollinationsBase = "https://image.pollinations.ai/prompt"
	pollinationsUA   = "GOLD-MD/1.0"
)

var (
	pollinationsKey   = strings.TrimSpace(os.Getenv("POLLINATIONS_API_KEY"))
	imagineRandomFlag = regexp.MustCompile(`\s+--random\s*$`)
	imagineHdFlag     = regexp.MustCompile(`\s+--hd\s*$`)
	imagineTurboFlag  = regexp.MustCompile(`\s+--turbo\s*$`)
	imagineGptFlag    = regexp.MustCompile(`\s+--gpt\s*$`)
)

// modelFallbackOrder returns the list of models to try in order.
func imagineModelFallback(primary string) []string {
	switch primary {
	case "turbo":
		return []string{"turbo", "flux", "gptimage"}
	case "gpt":
		return []string{"gptimage", "flux", "turbo"}
	default:
		return []string{"flux", "turbo", "gptimage"}
	}
}

// seedFromPrompt generates a deterministic 31-bit seed from the prompt text.
func seedFromPrompt(text string) int64 {
	h := sha256.Sum256([]byte(text))
	v := binary.BigEndian.Uint32(h[:4])
	return int64(v & 0x7fffffff)
}

// generatePollinationsImage calls the Pollinations API and returns image bytes.
func generatePollinationsImage(prompt, model string, width, height int, seed int64, useSeed bool) ([]byte, error) {
	u := fmt.Sprintf("%s/%s", pollinationsBase, url.PathEscape(prompt))
	params := url.Values{}
	params.Set("width", fmt.Sprintf("%d", width))
	params.Set("height", fmt.Sprintf("%d", height))
	params.Set("model", model)
	params.Set("nologo", "true")
	if useSeed {
		params.Set("seed", fmt.Sprintf("%d", seed))
	}
	fullURL := u + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", pollinationsUA)
	if pollinationsKey != "" {
		req.Header.Set("Authorization", "Bearer "+pollinationsKey)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FAULTED:%s:err=%v", model, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("FAULTED:%s:read_err=%v", model, err)
	}

	if resp.StatusCode != 200 || len(data) < 500 {
		return nil, fmt.Errorf("FAULTED:%s:status=%d:len=%d", model, resp.StatusCode, len(data))
	}
	return data, nil
}

func handleImagine(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleImagineAsync(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func handleImagineAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	prompt := strings.TrimSpace(strings.Join(args, " "))

	if prompt == "" {
		s.Reply(info, "👑 *TEXT TO IMAGE GUIDE* 👑\n*CREATE IMAGE USING TEXT*\n\n*TYPE LIKE THIS*\n*"+prefix+"IMAGINE ❮ TEXT PROMPT ❯*\n\n*EXAMPLE PROMPT TEXT LIKE.....*\n\n*IMAGINE A BEAUTIFUL SPORTS CAR ON THE MOUNTAIN ROAD*\n\n*IMAGINE A CAT AND THIS EATING THE FISH*\n\n*A BEAUTIFUL JUNGLE OF BEAUTIFUL TREES*\n\n*TYPE LIKE THIS AND AI WILL CREATE AN IMAGE FOR YOU 😊*")
		return
	}

	// Parse flags
	wantRandom := imagineRandomFlag.MatchString(prompt)
	if wantRandom {
		prompt = strings.TrimSpace(imagineRandomFlag.ReplaceAllString(prompt, ""))
	}
	wantHd := imagineHdFlag.MatchString(prompt)
	if wantHd {
		prompt = strings.TrimSpace(imagineHdFlag.ReplaceAllString(prompt, ""))
	}
	wantTurbo := imagineTurboFlag.MatchString(prompt)
	if wantTurbo {
		prompt = strings.TrimSpace(imagineTurboFlag.ReplaceAllString(prompt, ""))
	}
	wantGpt := imagineGptFlag.MatchString(prompt)
	if wantGpt {
		prompt = strings.TrimSpace(imagineGptFlag.ReplaceAllString(prompt, ""))
	}

	primaryModel := "flux"
	if wantGpt {
		primaryModel = "gpt"
	} else if wantTurbo {
		primaryModel = "turbo"
	}
	modelsToTry := imagineModelFallback(primaryModel)

	size := 768
	if wantHd {
		size = 1024
	}

	useSeed := !wantRandom
	var seed int64
	if useSeed {
		seed = seedFromPrompt(prompt)
	}

	// Simple static message — no progress bar edits (keeps server fast)
	waitMsgID := s.ReplyWithID(info, "*CREATING IMAGE........*")

	// Try each model in fallback order
	var imageBytes []byte
	var lastErr error
	for _, modelName := range modelsToTry {
		imageBytes, lastErr = generatePollinationsImage(prompt, modelName, size, size, seed, useSeed)
		if lastErr == nil {
			break
		}
		if debugEnabled {
			//			JSONDebug("IMAGINE_MODEL_FALLBACK", map[string]any{
			//				"model": modelName, "error": lastErr.Error(),
			//			})
		}
	}

	if imageBytes == nil {
		errText := "Try Again Later."
		if lastErr != nil {
			errText = lastErr.Error()
		}
		s.DeleteMessage(info, waitMsgID)
		s.Reply(info, "❌ *IMAGINE Command Error*\n"+errText)
		return
	}

	// Done — delete the wait message and send the result
	s.DeleteMessage(info, waitMsgID)

	caption := "*AI CREATED IMAGE FOR THIS TEXT 👇*\n " + prompt + "\n"
	if err := s.SendImage(info, imageBytes, caption); err != nil {
		s.Reply(info, "❌ *IMAGINE Command Error*\nFailed to send image: "+err.Error())
	}
}

func init() {
	Register(Command{Name: "imagine", Category: "AI & MEDIA", Desc: "AI text-to-image (Pollinations)", Run: handleImagine})
	Register(Command{Name: "aiimage", Hidden: true, Run: handleImagine})
	Register(Command{Name: "aiimg", Hidden: true, Run: handleImagine})
	Register(Command{Name: "texttoimg", Hidden: true, Run: handleImagine})
	Register(Command{Name: "aipic", Hidden: true, Run: handleImagine})
	Register(Command{Name: "aiphoto", Hidden: true, Run: handleImagine})
	Register(Command{Name: "bing", Hidden: true, Run: handleImagine})
	Register(Command{Name: "txt2img", Hidden: true, Run: handleImagine})
}
