package goldcmds

// ============================================================================
// GOLD-MD — .remini command (Agnes AI image editor + generator)
// File: remini.go
// ============================================================================
// The old hidden "rm" alias was removed: .rm is now the Read More command
// (gold-cmds/readmore.go).
// Ported 1:1 from UMAR-MD remini.js (Agnes AI version).
//
// TWO PROVIDERS:
//   1) MISTRAL = shared "brain" — reads the Roman-Urdu/Urdu/Hindi/English
//      prompt and classifies the edit kind (remove/replace/background/...).
//      Falls back to a regex parser when Mistral fails/absent.
//   2) AGNES   = ONLY editor — input photo + edit-instruction -> edited
//      photo. When NO photo is replied to, it does FRESH text-to-image.
//
// 20-KEY ROTATION (sequential 15-per-key fill, same as remini.js):
//   - Only 1 key active at a time until its 70s window hits 15 requests.
//   - Busy lock: concurrent use skips the key for that scan only.
//   - Rate-limit rest: FIXED 70s rest on retryable statuses
//     (Retry-After header respected), no exponential growth.
//
// Keys from env: REMINI_API_KEY_1 ... REMINI_API_KEY_20
// (hardcoded fallback array below, same as remini.js)
// ============================================================================

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── Mistral (brain) config ─────────────────────────────────────────────────

const (
	reminiMistralHardcodedKey = "zpNFAJiQSe6B4mjUGJoEYocY6EBreT19"
	reminiMistralModel        = "mistral-small-latest"
	reminiMistralChatURL      = "https://api.mistral.ai/v1/chat/completions"
)

var reminiMistralKey = func() string {
	if v := strings.TrimSpace(os.Getenv("MISTRAL_API_KEY")); v != "" {
		return v
	}
	return reminiMistralHardcodedKey
}()

// ── Agnes (editor + generator) config ──────────────────────────────────────

const (
	reminiGenURL       = "https://apihub.agnes-ai.com/v1/images/generations"
	reminiEditModel    = "agnes-image-2.1-flash"
	reminiEditSize     = "1024x1024"
	reminiGenSize      = "2K"
	reminiGenRatio     = "1:1"
	reminiMaxReqPerKey = 15
	reminiReqWindowMs  = 70 * 1000
	reminiBusyPollMs   = 4000
	reminiFixedRestMs  = 70 * 1000 // FIXED rest — never grows
	reminiDisplayExtra = 20 * 1000
)

var (
	reminiEditSeed = strings.TrimSpace(os.Getenv("REMINI_EDIT_SEED"))
	reminiGenSeed  = strings.TrimSpace(os.Getenv("REMINI_GENERATE_SEED"))
)

var hardcodedReminiKeys = []string{
	"sk-7aK1lijwLfyLx5D0dnagAxXJWBGscbwR8TMyFiS8zhvR4eix",
	"sk-9IBkefGMIa62Uw85pTLUTQokPgep6zdB3ECkqLwIs9DDkiVc",
	"sk-pNdokPJwR5m1EK0xgF5nH6FEKHonQRZ08LYI5AGFJvR4wk44",
	"sk-13ETMK35zDkXUORuwXfKDRsfW32A3bIuZyM6jZpn3AdiEP4q",
	"sk-HjJKRBxZD6zbed2xXmGScFuU7ynzoI1MIphFM6yjKziJ0nzZ",
	"sk-mWN1NipTYZ4tRgzyfJoq6AWlIY2OrCdTRMmpAkn132BXKoiV",
	"sk-I8g61r4YLIMqG41YbFOKD8iu5RbCvPvSWupNAOcTgOJF7MjJ",
	"sk-mUYUyKbELGJddM0mEY9IJiZ8GI3XsA97kdFmJa4SxQsg7kND",
	"sk-iNM3madJIJVT7TZmSoO7UotOhf3YveSd6DzWb0bGcEZLcIOl",
	"sk-vicNbyGmLVKv0EoCPRDNK6WhH09IzXx5gUBJelQtoZKQ523h",
	"sk-GX4FuKfvQboUfKbWSmpi47QBiIGxhea5PRfAK8qudK74RGz7",
	"sk-ggTJHAcdRxFrN09ENAAggwh7QOCxlRUE6JoDY9ok93cCpO1r",
	"sk-UpsNTITugDylvJcCwbQytcr2u5s1qyusec2szW2diH8HELyq",
	"sk-HPSmRXLSZnWQCfXuePcSJ3YjWU24RQlaOQUbAJhtV6fWVVlg",
	"sk-a60tSmsj1aSESVLKfSzpm4Btqg3FD68EMZ01tyEc3Rq4WKjZ",
	"sk-2dquvwwfWRcLTh5iH8iAY8mIrJU1Ku4MBgvMe6Vl3Cmvw1Am",
	"sk-h3GDEwjnO43cjDRqMjWCkm8XYuIXAkE3jpEFQP508PvtiGrH",
	"sk-bmIicnCHdtOTbMCYjAA2fYOyb4UJqi2OhTILZmnrS3VaTMNG",
	"sk-K552a8oMzNLVd3ULcZRmTm73QTU3x9N9a7aAKF7zrVbJuLHD",
	"sk-0E6fbBnjTtmQ8sz31ouEnkKtSUVDC74bsVzFfiUNP8VzaKCQ",
}

var reminiRetryableStatuses = map[int]bool{
	408: true, 420: true, 429: true, 500: true, 502: true, 503: true, 504: true, 520: true, 522: true, 524: true,
}

// ── 20-key pool state (same as imagine3 pattern) ───────────────────────────

type reminiKeyEntry struct {
	mu           sync.Mutex
	index        int
	key          string
	restUntil    int64 // unix nano; 0 = free
	busy         bool
	failCount    int
	requestCount int
	windowStart  int64 // unix nano; 0 = window not started
}

var (
	reminiKeyPool   []*reminiKeyEntry
	reminiActiveIdx = 0
	reminiPoolMu    sync.Mutex
	reminiInitOnce  sync.Once
)

func initReminiPool() {
	reminiInitOnce.Do(func() {
		for i := 0; i < len(hardcodedReminiKeys); i++ {
			key := os.Getenv(fmt.Sprintf("REMINI_API_KEY_%d", i+1))
			if key == "" {
				key = hardcodedReminiKeys[i]
			}
			reminiKeyPool = append(reminiKeyPool, &reminiKeyEntry{index: i + 1, key: key})
		}
	})
}

func (e *reminiKeyEntry) isResting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restUntil != 0 && e.restUntil > time.Now().UnixNano()
}

func (e *reminiKeyEntry) msLeft() int64 {
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
// resting until window end if it hit the 15-request cap.
func (e *reminiKeyEntry) noteCapIfNeeded() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixNano()
	if e.windowStart != 0 && (now-e.windowStart)/int64(time.Millisecond) >= reminiReqWindowMs {
		e.requestCount = 0
		e.windowStart = 0
	}
	if e.windowStart != 0 && e.requestCount >= reminiMaxReqPerKey {
		capUntil := e.windowStart + int64(reminiReqWindowMs)*int64(time.Millisecond)
		if e.restUntil == 0 || capUntil > e.restUntil {
			e.restUntil = capUntil
		}
	}
}

// markResting — Retry-After header respected, else FIXED 70s (no growth).
func (e *reminiKeyEntry) markResting(retryAfterSec int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ms := 0
	if retryAfterSec > 0 {
		ms = retryAfterSec * 1000
	} else {
		ms = reminiFixedRestMs
	}
	e.failCount++
	e.restUntil = time.Now().UnixNano() + int64(ms)*int64(time.Millisecond)
}

// getNextAvailableKey finds the next free key (sticky sequential fill).
func reminiGetNextAvailableKey() *reminiKeyEntry {
	reminiPoolMu.Lock()
	defer reminiPoolMu.Unlock()
	initReminiPool()
	scan := reminiActiveIdx
	for tries := 0; tries < len(reminiKeyPool); tries++ {
		idx := scan % len(reminiKeyPool)
		entry := reminiKeyPool[idx]
		entry.noteCapIfNeeded()

		entry.mu.Lock()
		if entry.key == "" {
			entry.mu.Unlock()
			scan++
			reminiActiveIdx = scan
			continue
		}
		if entry.restUntil != 0 && entry.restUntil > time.Now().UnixNano() {
			entry.mu.Unlock()
			scan++
			reminiActiveIdx = scan
			continue
		}
		if entry.busy {
			entry.mu.Unlock()
			scan++ // skip this scan only — sticky pointer stays
			continue
		}
		// Available — lock it
		reminiActiveIdx = idx
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

func (e *reminiKeyEntry) release() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.busy = false
}

// getPoolStatus returns combined wait info when no key is free.
func reminiGetPoolStatus() (hasBusy bool, totalMs, soonestMs int64) {
	initReminiPool()
	for _, e := range reminiKeyPool {
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
	totalMs += reminiDisplayExtra
	return
}

// ── mime guess from magic bytes ────────────────────────────────────────────

func reminiGuessImageMime(buf []byte) string {
	if len(buf) < 4 {
		return "image/png"
	}
	if buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
		return "image/jpeg"
	}
	if buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47 {
		return "image/png"
	}
	if buf[0] == 0x52 && buf[1] == 0x49 && buf[2] == 0x46 && buf[3] == 0x46 {
		return "image/webp"
	}
	if buf[0] == 0x47 && buf[1] == 0x49 && buf[2] == 0x46 {
		return "image/gif"
	}
	return "image/png"
}

// ── classification (kind + params) ─────────────────────────────────────────

type reminiClassification struct {
	kind        string
	object      string
	where       string
	from        string
	to          string
	background  string
	color       string
	text        string
	instruction string
}

var reminiKinds = map[string]bool{
	"remove": true, "add": true, "replace": true, "background": true,
	"recolor": true, "removebg": true, "enhance": true, "upscale": true,
	"restore": true, "text": true, "custom": true, "unclear": true,
}

func reminiBuildSystemPrompt() string {
	return strings.Join([]string{
		"You are a strict intent classifier for a photo-editing bot.",
		"The user message may be written in Roman Urdu, Urdu, Hindi, or English, and can be messy/casual.",
		"Reply with EXACTLY ONE line, pipe-separated (\"|\"), and nothing else — no JSON, no markdown, no explanation.",
		"",
		"Allowed line formats (pick exactly one \"kind\"):",
		"remove|<short name of the object to remove>",
		"add|<short description of the new object/person to add>|<where/relative to what, e.g. \"next to the boy\", \"beside him\", \"in the background\">",
		"replace|<object being replaced>|<new object>",
		"background|<short description of the new background/scene>",
		"recolor|<object whose color changes>|<new color>",
		"removebg",
		"enhance",
		"upscale",
		"restore",
		"text|<exact text/name to write on the image>",
		"custom|<a clear, detailed English rewrite of exactly what the user wants changed>",
		"unclear",
		"",
		"Rules:",
		"- The first field must be EXACTLY one of: remove, add, replace, background, recolor, removebg, enhance, upscale, restore, text, custom, unclear — never invent another word.",
		"- removebg/enhance/upscale/restore/unclear take no extra fields — output just that single word.",
		"- Keep object/color/background fields short (a few words), verbatim from the user's intent, no extra commentary.",
		"- Use kind \"add\" when the user wants a NEW object/person INSERTED into the photo ALONGSIDE the existing subject(s), with the existing subject(s) kept unchanged — e.g. \"is boy k sath ek girl ko khara kro\", \"isme bhi ek cute girl add karo\", \"iske pass ek car laga do\", \"add a dog next to him\". This is NOT a replace — nothing existing is being removed or swapped.",
		"- Use kind \"replace\" ONLY when an existing object/person is being swapped out and disappears — e.g. \"car ko bike se badal do\", \"shirt ko hoodie se change karo\". If the existing subject should stay visible together with something new, that is \"add\", never \"replace\".",
		"- If the user wants a name/word/caption written ON the photo (e.g. \"is per naam Umar likho\", \"write Umar on it\", \"photo pe Ali likhdo\"), use kind \"text\" with the exact text to write — do NOT use background or replace for this.",
		"- Use kind \"custom\" for ANYTHING that does not cleanly fit remove/add/replace/background/recolor/removebg/enhance/upscale/restore/text — e.g. changing a person's face, changing hairstyle, changing clothes/outfit, writing text specifically ON the background (not on the subject), placing a picture/photo/logo INTO the background or onto a wall/object in the scene, changing pose or expression, combining several small edits in one request, or any other specific visual edit the user describes. For \"custom\", rewrite the user's request as one clear, specific, detailed English editing instruction — keep every detail the user gave (what, where, style, color, text content, etc.), do not drop information, and do not add anything the user did not ask for.",
		"- Only output \"unclear\" if the message has NO identifiable editing request at all (e.g. random chat, a greeting, spam) — if there is ANY describable visual edit intent, even a vague or unusual one, use \"custom\" instead of \"unclear\".",
	}, "\n")
}

// sanitizeClassification — first line, split '|', hard membership gate.
func reminiSanitizeClassification(rawOutput string) *reminiClassification {
	firstLine := strings.TrimSpace(strings.Split(rawOutput, "\n")[0])
	if firstLine == "" {
		return nil
	}
	fields := strings.Split(firstLine, "|")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	kind := strings.ToLower(fields[0])
	kind = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, kind)
	if !reminiKinds[kind] || kind == "unclear" {
		return nil
	}
	switch kind {
	case "remove":
		if len(fields) < 2 || fields[1] == "" {
			return nil
		}
		return &reminiClassification{kind: "remove", object: fields[1]}
	case "add":
		if len(fields) < 2 || fields[1] == "" {
			return nil
		}
		where := ""
		if len(fields) >= 3 {
			where = fields[2]
		}
		return &reminiClassification{kind: "add", object: fields[1], where: where}
	case "replace":
		if len(fields) < 3 || fields[1] == "" || fields[2] == "" {
			return nil
		}
		return &reminiClassification{kind: "replace", from: fields[1], to: fields[2]}
	case "background":
		if len(fields) < 2 || fields[1] == "" {
			return nil
		}
		return &reminiClassification{kind: "background", background: fields[1]}
	case "recolor":
		if len(fields) < 3 || fields[1] == "" || fields[2] == "" {
			return nil
		}
		return &reminiClassification{kind: "recolor", object: fields[1], color: fields[2]}
	case "text":
		if len(fields) < 2 || fields[1] == "" {
			return nil
		}
		return &reminiClassification{kind: "text", text: fields[1]}
	case "custom":
		if len(fields) < 2 {
			return nil
		}
		instruction := strings.TrimSpace(strings.Join(fields[1:], "|"))
		if instruction == "" {
			return nil
		}
		return &reminiClassification{kind: "custom", instruction: instruction}
	}
	// removebg / enhance / upscale / restore — no extra fields
	return &reminiClassification{kind: kind}
}

// buildReminiEditInstruction maps a classification to an English instruction.
func reminiBuildEditInstruction(c *reminiClassification) string {
	switch c.kind {
	case "remove":
		return "Remove the " + c.object + " from this image completely and cleanly fill the emptied area with the natural surrounding background. Keep the rest of the image, the subject, lighting and composition exactly the same."
	case "add":
		where := c.where
		if where == "" {
			where = " into this image, naturally placed near the existing subject"
		} else {
			where = " " + where
		}
		return "Add " + c.object + where + ". Do NOT remove, replace, or change the existing subject(s), their pose, the background, or the lighting in any way — only insert the new element and blend it in naturally with matching lighting and perspective. Keep everything else in the original photo exactly the same."
	case "replace":
		return "Replace the " + c.from + " in this image with " + c.to + ". Keep everything else — background, lighting, pose and composition — exactly the same."
	case "background":
		return "Replace only the background of this image with " + c.background + ". Keep the main foreground subject exactly the same, with correct edges and lighting."
	case "recolor":
		return "Change the color of the " + c.object + " in this image to " + c.color + ". Keep everything else in the image exactly the same."
	case "removebg":
		return "Remove the background of this image completely, isolating only the main subject on a clean plain background. Do not alter the subject itself."
	case "enhance":
		return "Enhance this image: improve sharpness, clarity, color balance and contrast and overall quality. Do not add, remove or change any content or subject."
	case "upscale":
		return "Upscale this image to a higher resolution with crisper detail and less noise, without changing its content or composition in any way."
	case "restore":
		return "Restore this old, damaged, faded or blurry photo: repair scratches, blur and washed-out colors while faithfully keeping the original people, objects and scene."
	case "text":
		return "Add the text \"" + c.text + "\" onto this image in a clean, readable, well-placed style. Do not remove or change anything else in the image."
	case "custom":
		return c.instruction + " Apply ONLY this specific change. Keep every other part of the image — the subject(s), their identity, pose, other clothing/features not mentioned, the background, and the lighting — exactly the same unless this instruction explicitly says to change it."
	}
	return ""
}

// classifyPromptWithMistral — Mistral brain call.
func reminiClassifyWithMistral(rawPrompt string) (*reminiClassification, error) {
	if reminiMistralKey == "" {
		return nil, fmt.Errorf("Mistral key missing")
	}
	body := map[string]any{
		"model": reminiMistralModel,
		"messages": []map[string]string{
			{"role": "system", "content": reminiBuildSystemPrompt()},
			{"role": "user", "content": rawPrompt},
		},
		"temperature": 0,
		"max_tokens":  40,
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", reminiMistralChatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+reminiMistralKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Mistral status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("Mistral: empty response")
	}
	raw := parsed.Choices[0].Message.Content
	c := reminiSanitizeClassification(raw)
	if c == nil {
		return nil, fmt.Errorf("Mistral: unclear/invalid classification (raw: \"%s\")", strings.TrimSpace(raw[:min(80, len(raw))]))
	}
	if reminiBuildEditInstruction(c) == "" {
		return nil, fmt.Errorf("Mistral: kind \"%s\" has no instruction builder", c.kind)
	}
	return c, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── REGEX FALLBACK parser (same patterns as remini.js) ─────────────────────

var (
	reminiReRemove1 = regexp.MustCompile(`(?i)(.+?)\s*(ko\s*)?hata\s*(do|den|dein)?$`)
	reminiReRemove2 = regexp.MustCompile(`(?i)^(?:remove|delete)\s+(.+)`)
	reminiReHatao   = regexp.MustCompile(`(?i)^(hatao|hata)\s*`)

	reminiReAdd1     = regexp.MustCompile(`(?i)(.+?)\s*k[ie]?\s*sath\s*(?:ek\s*)?(.+?)\s*(?:ko\s*)?(?:khara|khada|lagao|laga do|add)\s*(?:kro|karo|kardo|kar do)?$`)
	reminiReAdd2     = regexp.MustCompile(`(?i)^add\s+(.+?)\s+(?:next to|beside|with)\s+(.+)`)
	reminiReAdd3     = regexp.MustCompile(`(?i)(?:isme|is\s*photo\s*me|is\s*pic\s*me)\s*(?:bhi\s*)?(?:ek\s*)?(.+?)\s*(?:add|daal)\s*(?:kro|karo|do|den)?$`)
	reminiReAddStart = regexp.MustCompile(`(?i)^add\s+`)

	reminiReReplace1 = regexp.MustCompile(`(?i)(.+?)\s*ko\s*(.+?)\s*se\s*(?:badal|badlo|replace)`)
	reminiReReplace2 = regexp.MustCompile(`(?i)^replace\s+(.+?)\s+with\s+(.+)`)
	reminiReBgKo     = regexp.MustCompile(`(?i)background`)

	reminiReBg1 = regexp.MustCompile(`(?i)background\s*(?:ko\s*)?(.+?)\s*(?:se\s*)?(?:replace|badal|badlo)`)
	reminiReBg2 = regexp.MustCompile(`(?i)background\s*me\s*(.+?)\s*(?:lagao|laga do)`)
	reminiReBg3 = regexp.MustCompile(`(?i)^(?:change|set)\s+(?:the\s+)?background\s+to\s+(.+)`)

	reminiReColor1 = regexp.MustCompile(`(?i)(.+?)\s*ka\s*color\s*(.+?)\s*(?:kardo|kar do|karo)`)
	reminiReColor2 = regexp.MustCompile(`(?i)^change\s+(.+?)\s+color\s+to\s+(.+)`)

	reminiReRemoveBg1 = regexp.MustCompile(`(?i)\bremove\s+(?:the\s+)?background\b`)
	reminiReRemoveBg2 = regexp.MustCompile(`(?i)\bbackground\s*(?:ko\s*)?(?:hata|hatao|nikal|remove)\s*(?:do|den|dein|karo)?\b`)
	reminiReRemoveBg3 = regexp.MustCompile(`(?i)\bbg\s*(?:hata|remove)`)

	reminiReEnhance1 = regexp.MustCompile(`(?i)\benhance\b`)
	reminiReEnhance2 = regexp.MustCompile(`(?i)\bimprove\b`)
	reminiReEnhance3 = regexp.MustCompile(`(?i)quality\s*(?:better|acha|behtar|sudhar)`)
	reminiReEnhance4 = regexp.MustCompile(`(?i)\bsharp\s*(?:kar|karo)`)

	reminiReUpscale1 = regexp.MustCompile(`(?i)\bupscale\b`)
	reminiReUpscale2 = regexp.MustCompile(`(?i)\bhd\s*(?:kar|karo)`)
	reminiReUpscale3 = regexp.MustCompile(`(?i)resolution\s*(?:barhao|badhao|increase)`)

	reminiReRestore1 = regexp.MustCompile(`(?i)\brestore\b`)
	reminiReRestore2 = regexp.MustCompile(`(?i)purani\s*photo`)
	reminiReRestore3 = regexp.MustCompile(`(?i)damaged|blurry|faded`)
	reminiReRestore4 = regexp.MustCompile(`(?i)photo\s*(?:thik|sudhar)`)

	reminiReText1 = regexp.MustCompile(`(?i)(?:is\s*per|photo\s*(?:pe|par|mein|me)?)\s*(?:naam\s*)?(.+?)\s*likh`)
	reminiReText2 = regexp.MustCompile(`(?i)naam\s+(.+?)\s*likh`)
	reminiReText3 = regexp.MustCompile(`(?i)^write\s+(.+?)\s+on\s+(?:it|the\s+image|the\s+photo)`)
)

func reminiTrimLeadingHatao(s string) string {
	return strings.TrimSpace(reminiReHatao.ReplaceAllString(s, ""))
}

func reminiParsePrompt(rawPrompt string) *reminiClassification {
	prompt := strings.TrimSpace(rawPrompt)
	lower := strings.ToLower(prompt)
	var m []string

	// 1) REMOVE
	m = reminiReRemove1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReRemove2.FindStringSubmatch(lower)
	}
	if m != nil {
		obj := reminiTrimLeadingHatao(m[1])
		if obj != "" {
			return &reminiClassification{kind: "remove", object: obj}
		}
	}
	// 2) ADD
	m = reminiReAdd1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReAdd2.FindStringSubmatch(lower)
	}
	if m == nil {
		m = reminiReAdd3.FindStringSubmatch(lower)
	}
	if m != nil {
		isAddWith := reminiReAddStart.MatchString(lower)
		var object, where string
		if isAddWith {
			object = strings.TrimSpace(m[1])
			if len(m) >= 3 {
				where = "next to " + strings.TrimSpace(m[2])
			}
		} else {
			if len(m) >= 3 && m[2] != "" {
				object = strings.TrimSpace(m[2])
			} else {
				object = strings.TrimSpace(m[1])
			}
			where = "next to the existing subject (" + strings.TrimSpace(m[1]) + ")"
		}
		if object != "" {
			return &reminiClassification{kind: "add", object: object, where: where}
		}
	}
	// 3) REPLACE
	m = reminiReReplace1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReReplace2.FindStringSubmatch(lower)
	}
	if m != nil {
		from := strings.TrimSpace(m[1])
		to := strings.TrimSpace(m[2])
		beforeKo := lower
		if idx := strings.Index(lower, "ko"); idx >= 0 {
			beforeKo = lower[:idx]
		}
		if from != "" && to != "" && !strings.Contains(from, "background") && !reminiReBgKo.MatchString(beforeKo) {
			return &reminiClassification{kind: "replace", from: from, to: to}
		}
	}
	// 4) BACKGROUND REPLACE
	m = reminiReBg1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReBg2.FindStringSubmatch(lower)
	}
	if m == nil {
		m = reminiReBg3.FindStringSubmatch(lower)
	}
	if m != nil {
		// NOTE: optional (ko..)/(se..) groups are non-capturing in this Go port, so the
		// background text is ALWAYS in m[1] (JS used m[2] || m[1] equivalently).
		bg := strings.TrimSpace(m[1])
		if bg != "" {
			return &reminiClassification{kind: "background", background: bg}
		}
	}
	// 5) RECOLOR
	m = reminiReColor1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReColor2.FindStringSubmatch(lower)
	}
	if m != nil {
		obj := strings.TrimSpace(m[1])
		color := strings.TrimSpace(m[2])
		if obj != "" && color != "" {
			return &reminiClassification{kind: "recolor", object: obj, color: color}
		}
	}
	// 6) BACKGROUND REMOVE
	if reminiReRemoveBg1.MatchString(lower) || reminiReRemoveBg2.MatchString(lower) || reminiReRemoveBg3.MatchString(lower) {
		return &reminiClassification{kind: "removebg"}
	}
	// 7) ENHANCE
	if reminiReEnhance1.MatchString(lower) || reminiReEnhance2.MatchString(lower) ||
		reminiReEnhance3.MatchString(lower) || reminiReEnhance4.MatchString(lower) {
		return &reminiClassification{kind: "enhance"}
	}
	// 8) UPSCALE
	if reminiReUpscale1.MatchString(lower) || reminiReUpscale2.MatchString(lower) || reminiReUpscale3.MatchString(lower) {
		return &reminiClassification{kind: "upscale"}
	}
	// 9) RESTORE
	if reminiReRestore1.MatchString(lower) || reminiReRestore2.MatchString(lower) ||
		reminiReRestore3.MatchString(lower) || reminiReRestore4.MatchString(lower) {
		return &reminiClassification{kind: "restore"}
	}
	// 10) TEXT ON IMAGE
	m = reminiReText1.FindStringSubmatch(lower)
	if m == nil {
		m = reminiReText2.FindStringSubmatch(lower)
	}
	if m == nil {
		m = reminiReText3.FindStringSubmatch(lower)
	}
	if m != nil {
		text := strings.TrimSpace(m[1])
		if text != "" {
			return &reminiClassification{kind: "text", text: text}
		}
	}
	// 11) NO MATCH — best-effort custom passthrough
	return &reminiClassification{kind: "custom", instruction: prompt}
}

// resolvePromptToClassification: Mistral -> regex fallback.
func reminiResolvePrompt(rawPrompt string) *reminiClassification {
	if reminiMistralKey != "" {
		if c, err := reminiClassifyWithMistral(rawPrompt); err == nil {
			return c
		}
	}
	return reminiParsePrompt(rawPrompt)
}

// ── Agnes API calls (per key) ──────────────────────────────────────────────

// parseAgnesResult handles the shared response shape + fallbacks + download.
func reminiParseAgnesResult(respBody []byte) ([]byte, error) {
	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		URL    string   `json:"url"`
		B64    string   `json:"b64_json"`
		Output []string `json:"output"`
		Images []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"images"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %v body=%s", err, string(respBody))
	}
	resultURL := ""
	resultB64 := ""
	if len(result.Data) > 0 {
		resultURL = result.Data[0].URL
		resultB64 = result.Data[0].B64JSON
	}
	if resultURL == "" && resultB64 == "" {
		resultURL = result.URL
		if resultB64 == "" {
			resultB64 = result.B64
		}
		if resultURL == "" && len(result.Output) > 0 {
			resultURL = result.Output[0]
		}
		if (resultURL == "" && resultB64 == "") && len(result.Images) > 0 {
			resultURL = result.Images[0].URL
			if resultB64 == "" {
				resultB64 = result.Images[0].B64JSON
			}
		}
	}
	if resultURL != "" {
		fileResp, err := http.Get(resultURL)
		if err != nil {
			return nil, fmt.Errorf("download image: %v", err)
		}
		defer fileResp.Body.Close()
		imgData, err := io.ReadAll(fileResp.Body)
		if err != nil {
			return nil, fmt.Errorf("read image: %v", err)
		}
		if len(imgData) == 0 {
			return nil, fmt.Errorf("downloaded empty image")
		}
		return imgData, nil
	}
	if resultB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(resultB64)
		if err != nil {
			return nil, fmt.Errorf("decode b64: %v", err)
		}
		if len(decoded) == 0 {
			return nil, fmt.Errorf("decoded empty b64 image")
		}
		return decoded, nil
	}
	return nil, fmt.Errorf("no url/b64 in response: %s", string(respBody))
}

// reminiDoRequest posts the body with the key; returns (image, retryAfter, err).
func reminiDoRequest(entry *reminiKeyEntry, body map[string]any) ([]byte, int, int, error) {
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", reminiGenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+entry.key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 0} // no time limit
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if reminiRetryableStatuses[resp.StatusCode] {
		retryAfter := 0
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			fmt.Sscanf(ra, "%d", &retryAfter)
		}
		return nil, retryAfter, resp.StatusCode, fmt.Errorf("retryable_status_%d", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		// try to pull an error message out of the body (like the JS does)
		var errJSON struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := ""
		if json.Unmarshal(respBody, &errJSON) == nil && errJSON.Error.Message != "" {
			msg = errJSON.Error.Message
		} else {
			msg = fmt.Sprintf("status=%d body=%s", resp.StatusCode, string(respBody))
		}
		return nil, 0, resp.StatusCode, fmt.Errorf("%s", msg)
	}
	img, err := reminiParseAgnesResult(respBody)
	return img, 0, 200, err
}

// ── Rotation wrappers (edit + fresh) ───────────────────────────────────────

// reminiEditImage: classification once, then rotate keys.
// Returns (image, waitMs, totalWaitMs, busyOnly, err).
func reminiEditImage(imageBuffer []byte, prompt string) ([]byte, int64, int64, bool, error) {
	initReminiPool()
	c := reminiResolvePrompt(prompt)
	instruction := reminiBuildEditInstruction(c)
	if instruction == "" {
		return nil, 0, 0, false, fmt.Errorf("empty edit instruction for kind %s", c.kind)
	}
	configured := 0
	for _, e := range reminiKeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("no REMINI_API_KEY configured")
	}

	mime := reminiGuessImageMime(imageBuffer)
	dataURI := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imageBuffer)

	body := map[string]any{
		"model":  reminiEditModel,
		"prompt": instruction,
		"size":   reminiEditSize,
		"extra_body": map[string]any{
			"image":           []string{dataURI},
			"response_format": "url",
		},
	}
	if reminiEditSeed != "" {
		if seedNum, err := strconv.Atoi(reminiEditSeed); err == nil {
			body["seed"] = seedNum
		}
	}

	for attempt := 0; attempt < len(reminiKeyPool); attempt++ {
		entry := reminiGetNextAvailableKey()
		if entry == nil {
			break
		}
		img, retryAfter, status, err := reminiDoRequest(entry, body)
		entry.release()
		if err == nil {
			entry.mu.Lock()
			entry.failCount = 0
			entry.mu.Unlock()
			return img, 0, 0, false, nil
		}
		if retryAfter > 0 || strings.Contains(err.Error(), "retryable_status_") {
			entry.markResting(retryAfter)
			continue
		}
		// JS: `Image edit fail (key #N, status|??): err.response?.data?.error?.message || err.message`
		statusStr := strconv.Itoa(status)
		if status == 0 {
			statusStr = "??"
		}
		return nil, 0, 0, false, fmt.Errorf("Image edit fail (key #%d, %s): %v", entry.index, statusStr, err)
	}
	hasBusy, totalMs, soonestMs := reminiGetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, int64(reminiBusyPollMs), int64(reminiBusyPollMs), true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}

// reminiGenerateFresh: text-to-image rotation.
func reminiGenerateFresh(prompt string) ([]byte, int64, int64, bool, error) {
	initReminiPool()
	configured := 0
	for _, e := range reminiKeyPool {
		if e.key != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil, 0, 0, false, fmt.Errorf("no REMINI_API_KEY configured")
	}

	body := map[string]any{
		"model":      reminiEditModel,
		"prompt":     prompt,
		"size":       reminiGenSize,
		"ratio":      reminiGenRatio,
		"n":          1,
		"extra_body": map[string]string{"response_format": "url"},
	}
	if reminiGenSeed != "" {
		if seedNum, err := strconv.Atoi(reminiGenSeed); err == nil {
			body["seed"] = seedNum
		}
	}

	for attempt := 0; attempt < len(reminiKeyPool); attempt++ {
		entry := reminiGetNextAvailableKey()
		if entry == nil {
			break
		}
		img, retryAfter, status, err := reminiDoRequest(entry, body)
		entry.release()
		if err == nil {
			entry.mu.Lock()
			entry.failCount = 0
			entry.mu.Unlock()
			return img, 0, 0, false, nil
		}
		if retryAfter > 0 || strings.Contains(err.Error(), "retryable_status_") {
			entry.markResting(retryAfter)
			continue
		}
		// JS: `Fresh image generate fail (key #N, status|??): err.response?.data?.error?.message || err.message`
		statusStr := strconv.Itoa(status)
		if status == 0 {
			statusStr = "??"
		}
		return nil, 0, 0, false, fmt.Errorf("Fresh image generate fail (key #%d, %s): %v", entry.index, statusStr, err)
	}
	hasBusy, totalMs, soonestMs := reminiGetPoolStatus()
	if hasBusy && soonestMs == 0 {
		return nil, int64(reminiBusyPollMs), int64(reminiBusyPollMs), true, nil
	}
	return nil, soonestMs, totalMs, false, nil
}

// ── HELP TEXT (remini.js — 👑 replaced with 🔰) ─────────────────────────────

const reminiHelpText = `*🔰 REMINI AI COMMAND INFO 🔰*

*🔰 HOW TO USE - FULL STEPS 🔰*

*STEP 1: SEND THE PHOTO YOU WANT TO EDIT*
*STEP 2: TAP AND REPLY TO THAT PHOTO*
*STEP 3: WRITE THE COMMAND BELOW IN THE REPLY AND SEND*

*NOTE: FOR EDITING A PHOTO YOU MUST REPLY TO IT — BUT FOR GENERATING A BRAND NEW IMAGE (SEE TOOL 10 BELOW) NO PHOTO/REPLY IS NEEDED*

*COMMANDS + EXAMPLES GUIDE*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*1. REMOVE OBJECT FROM PHOTO*
*COMMAND: .REMINI REMOVE THE CUP*
*WORK: IT WILL DELETE ANY OBJECT, PERSON, LOGO, WATERMARK FROM PHOTO*
*EXAMPLE 1: .REMINI REMOVE THE CUP*
*EXAMPLE 2: .REMINI REMOVE THE PERSON FROM BACKGROUND*
*EXAMPLE 3: .REMINI REMOVE THE WATERMARK*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*2. REPLACE OBJECT IN PHOTO*
*COMMAND: .REMINI CHANGE THE CAR TO BIKE*
*WORK: IT WILL REPLACE OLD OBJECT WITH NEW OBJECT*
*EXAMPLE 1: .REMINI CHANGE THE CAR TO BIKE*
*EXAMPLE 2: .REMINI CHANGE THE DOG TO CAT*
*EXAMPLE 3: .REMINI CHANGE THE T-SHIRT TO HOODIE*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*3. REMOVE BACKGROUND FROM PHOTO*
*COMMAND: .REMINI REMOVE BACKGROUND*
*WORK: IT WILL REMOVE FULL BACKGROUND. PHOTO WILL BECOME PNG TRANSPARENT*
*EXAMPLE 1: .REMINI REMOVE BACKGROUND*
*EXAMPLE 2: .REMINI MAKE BACKGROUND TRANSPARENT*
*EXAMPLE 3: .REMINI DELETE BACKGROUND*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*4. CHANGE BACKGROUND OF PHOTO*
*COMMAND: .REMINI CHANGE BACKGROUND TO SUNSET*
*WORK: IT WILL REMOVE OLD BACKGROUND AND ADD NEW BACKGROUND*
*EXAMPLE 1: .REMINI CHANGE BACKGROUND TO SUNSET*
*EXAMPLE 2: .REMINI CHANGE BACKGROUND TO BEACH*
*EXAMPLE 3: .REMINI CHANGE BACKGROUND TO SPACE*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*5. CHANGE COLOR OF ANYTHING IN PHOTO*
*COMMAND: .REMINI CHANGE SHIRT COLOR TO RED*
*WORK: ONLY THAT OBJECT COLOR WILL CHANGE, EVERYTHING ELSE SAME*
*EXAMPLE 1: .REMINI CHANGE SHIRT COLOR TO RED*
*EXAMPLE 2: .REMINI CHANGE CAR COLOR TO BLACK*
*EXAMPLE 3: .REMINI CHANGE WALL COLOR TO BLUE*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*6. ENHANCE THE IMAGE QUALITY*
*COMMAND: .REMINI ENHANCE PHOTO*
*WORK: IT WILL FIX BLUR, NOISE, DARKNESS. QUALITY WILL IMPROVE*
*EXAMPLE 1: .REMINI ENHANCE PHOTO*
*EXAMPLE 2: .REMINI MAKE THIS PHOTO CLEAR*
*EXAMPLE 3: .REMINI FIX THE QUALITY OF THIS PHOTO*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*7. UPSCALE IMAGE TO HD/4K*
*COMMAND: .REMINI UPSCALE PHOTO TO HD*
*WORK: IT WILL ZOOM 2X, 4X AND MAKE PHOTO CLEAR AND SHARP*
*EXAMPLE 1: .REMINI UPSCALE PHOTO TO HD*
*EXAMPLE 2: .REMINI MAKE THIS PHOTO 4K*
*EXAMPLE 3: .REMINI CONVERT THIS PHOTO TO HIGH QUALITY*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*8. RESTORE OLD DAMAGED PHOTO*
*COMMAND: .REMINI RESTORE OLD PHOTO*
*WORK: IT WILL FIX TORN, FADED, BLACK-WHITE PHOTO TO NEW COLORED PHOTO*
*EXAMPLE 1: .REMINI RESTORE OLD PHOTO*
*EXAMPLE 2: .REMINI RESTORE THIS DAMAGED PHOTO*
*EXAMPLE 3: .REMINI COLORIZE THIS BLACK AND WHITE PHOTO*

*🔰 WARNING: MENTION THE IMAGE FIRST THEN WRITE COMMAND OTHERWISE COMMAND WILL NOT WORK 🔰*
*9. ADD TEXT ON PHOTO*
*COMMAND: .REMINI WRITE UMAR ON THIS*
*WORK: IT WILL WRITE THE TEXT YOU WANT ON THE PHOTO*
*EXAMPLE 1: .REMINI WRITE UMAR-MD ON THIS*
*EXAMPLE 2: .REMINI WRITE HAPPY BIRTHDAY ON THIS*
*EXAMPLE 3: .REMINI WRITE KING ON THIS PHOTO*

*🔰 NO PHOTO NEEDED FOR THIS ONE 🔰*
*10. GENERATE A BRAND NEW IMAGE (TEXT-TO-IMAGE)*
*COMMAND: .REMINI [ IMAGE PROMPT ]*
*WORK: DON'T REPLY TO ANY PHOTO — JUST TYPE .REMINI FOLLOWED BY WHAT IMAGE YOU WANT AND AI WILL CREATE A FRESH NEW IMAGE FROM SCRATCH*
*EXAMPLE 1: .REMINI A LION SITTING IN JUNGLE*
*EXAMPLE 2: .REMINI FUTURISTIC CITY AT NIGHT NEON LIGHTS*
*EXAMPLE 3: .REMINI A CAT FLYING IN THE SKY*

*━━━━━━━━━━━━*
*🔰 IMPORTANT RULES 🔰*
*1. FOR EDITING (TOOLS 1-9) YOU MUST REPLY TO THE PHOTO EVERY TIME*
*2. FOR GENERATING A NEW IMAGE (TOOL 10) DO NOT REPLY TO ANY PHOTO — JUST WRITE THE PROMPT*
*3. YOU CAN WRITE COMMAND IN ENGLISH OR URDU*
*4. USE ONLY 1 COMMAND AT A TIME*`

// handleRemini — main handler (goroutine + 10min timeout wrapper like imagine3).
func handleRemini(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleReminiAsync(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		s.Reply(info, "🔰 *REMINI COMMAND ERROR* 🔰\n*TIMEOUT — PLEASE TRY AGAIN*")
	}
}

func handleReminiAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	prompt := strings.TrimSpace(strings.Join(args, " "))

	if prompt == "" {
		s.Reply(info, reminiHelpText)
		return
	}

	// Quoted/mentioned image? (direct / view-once / quoted image)
	imageBuffer, _ := s.DownloadImage(info)
	if imageBuffer == nil {
		// quoted media might be a document-image or sticker
		if data, mime, ok := s.DownloadQuotedMedia(info); ok && strings.HasPrefix(mime, "image/") {
			imageBuffer = data
		}
	}

	isFreshMode := imageBuffer == nil
	captionTitle := "REMINI AI CREATED IMAGE"
	if isFreshMode {
		captionTitle = "REMINI AI GENERATED IMAGE"
	}

	// simple wait message — stays as-is until the image arrives (no live % edits)
	waitMsgID := s.ReplyWithID(info, "*REMINI AI IS CREATING IMAGE.....*")

	runOnce := func() ([]byte, int64, int64, bool, error) {
		if isFreshMode {
			return reminiGenerateFresh(prompt)
		}
		return reminiEditImage(imageBuffer, prompt)
	}

	imgData, waitMs, _, _, err := runOnce()

	for imgData == nil && err == nil {
		// all keys busy/resting — sleep and retry (wait message stays as-is)
		waitDuration := time.Duration(waitMs+500) * time.Millisecond
		if waitDuration < 1*time.Second {
			waitDuration = 1 * time.Second
		}
		time.Sleep(waitDuration)

		imgData, waitMs, _, _, err = runOnce()
	}

	if err != nil {
		// error path: delete the wait message too, then reply the error
		s.DeleteMessage(info, waitMsgID)
		s.Reply(info, "🔰 *REMINI COMMAND ERROR* 🔰\n*"+strings.ToUpper(err.Error())+"*")
		return
	}

	// success: delete the wait message, then send the image
	s.DeleteMessage(info, waitMsgID)
	caption := "*" + captionTitle + "*\n*YOUR PROMOT TEXT IS* 🔰\n\n" + strings.ToUpper(prompt)
	if sendErr := s.SendImage(info, imgData, caption); sendErr != nil {
		s.Reply(info, "🔰 *REMINI COMMAND ERROR* 🔰\n*"+strings.ToUpper(sendErr.Error())+"*")
	}
}

func init() {
	Register(Command{Name: "remini", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO MAKE ANY PHOTO HD AND CLEAR. REPLY TO A PHOTO AND USE THIS COMMAND TO ENHANCE IT.", Run: handleRemini})
}
