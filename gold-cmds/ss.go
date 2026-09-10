package goldcmds

// ============================================================================
// GOLD-MD — .SS / .SCREENSHOT / .SSWEB / .SSLINK COMMAND
// Ported from Node.js ss.js — same text, same behaviour.
//
// Kisi bhi website link ka EK SATH 3 screenshots leta hai —
// TABLET size, PC/DESKTOP size, aur ANDROID/MOBILE size — aur
// teeno images ek ke baad ek turant (ek hi run mein) bhej deta hai.
//
// PROVIDER: Microlink (api.microlink.io) — free public endpoint,
// koi API key nahi chahiye (50 requests/day free-tier limit hai).
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ─── Teeno device-sizes jo har baar liye jaate hain (fixed order) ───
type ssViewport struct {
	key      string
	label    string
	width    int
	height   int
	isMobile bool
	hasTouch bool
}

var ssViewports = []ssViewport{
	{key: "tablet", label: "TABLET SIZE", width: 768, height: 1024, isMobile: false, hasTouch: true},
	{key: "pc", label: "PC / DESKTOP SIZE", width: 1920, height: 1080, isMobile: false, hasTouch: false},
	{key: "android", label: "ANDROID / MOBILE SIZE", width: 412, height: 915, isMobile: true, hasTouch: true},
}

const (
	ssHelpText = "*🔰 WEBSITE SCREENSHOT COMMAND 🔰*\n\n" +
		"*PASTE THE WEBSITE LINK*\n*SS ❮ PASTE LINK HERE ❯*\n\n" +
		"*EXAMPLE*\n*SS https://example.com*\n\n"

	ssInvalidText = "*PASTE THE WEBSITE LINK SAME* \n*❮ SS https://example.com ❯*"

	ssErrorText = "🔰 *TRY AGAIN LATER*"
)

var ssUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// ssHTTPGet — HTTP GET with the same headers/timeout as ss.js
// (Go's http.Client follows redirects automatically).
func ssHTTPGet(target string, timeoutMs int) (int, []byte, error) {
	client := &http.Client{
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
	}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", ssUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

// ssNormalizeTargetUrl — bina http/https bhi chal jaye (".ss google.com").
func ssNormalizeTargetUrl(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(s), "http://") && !strings.HasPrefix(strings.ToLower(s), "https://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.String()
}

// ssMicrolinkScreenshot — ek viewport ke liye Microlink se screenshot:
// pehle JSON call se CDN image-URL, phir wahi CDN URL se image bytes.
func ssMicrolinkScreenshot(targetUrl string, vp ssViewport, timeoutMs int) ([]byte, error) {
	apiUrl := "https://api.microlink.io/?url=" + url.QueryEscape(targetUrl) +
		"&screenshot=true&meta=false&waitUntil=networkidle0" +
		"&viewport.width=" + strconv.Itoa(vp.width) +
		"&viewport.height=" + strconv.Itoa(vp.height) +
		"&viewport.isMobile=" + fmt.Sprintf("%t", vp.isMobile) +
		"&viewport.hasTouch=" + fmt.Sprintf("%t", vp.hasTouch)

	_, body, err := ssHTTPGet(apiUrl, timeoutMs)
	if err != nil {
		return nil, err
	}

	var jsonResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Screenshot struct {
				URL string `json:"url"`
			} `json:"screenshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("SS_BAD_RESPONSE")
	}

	if jsonResp.Status != "success" || jsonResp.Data.Screenshot.URL == "" {
		// Microlink free-tier daily-cap ka message alag hota hai — usko
		// readable form mein bata dete hain (same reply text anyway).
		apiMsg := jsonResp.Message
		if len(apiMsg) > 120 {
			apiMsg = apiMsg[:120]
		}
		lower := strings.ToLower(apiMsg)
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "ratelimit") ||
			strings.Contains(lower, "too many") || strings.Contains(lower, "limit exceeded") {
			return nil, fmt.Errorf("SS_RATE_LIMIT")
		}
		return nil, fmt.Errorf("SS_CAPTURE_FAILED")
	}

	status, imgBody, err := ssHTTPGet(jsonResp.Data.Screenshot.URL, timeoutMs)
	if err != nil {
		return nil, err
	}
	if status != 200 || len(imgBody) < 500 {
		return nil, fmt.Errorf("SS_IMAGE_DOWNLOAD_FAILED")
	}
	return imgBody, nil
}

func handleSS(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	rawInput := strings.TrimSpace(strings.Join(args, " "))
	if rawInput == "" {
		s.Reply(info, ssHelpText)
		return
	}

	targetUrl := ssNormalizeTargetUrl(rawInput)
	if targetUrl == "" {
		s.Reply(info, ssInvalidText)
		return
	}

	waitMsgID := s.ReplyWithID(info, "*TAKING SCREENSHOTS....*\n*STEP :❮ 0/3*")

	// Progress edit helper — "STEP: X/3 ❮ LABEL ❯"
	pushStep := func(index int, label string) {
		if waitMsgID == "" {
			return
		}
		s.EditMessage(info, waitMsgID, fmt.Sprintf("*TAKING SCREENSHOTS....*\n*STEP: %d/3 ❮ %s*", index, label))
	}

	type ssShot struct {
		vp     ssViewport
		buffer []byte
	}
	var shots []ssShot

	// ─── Teeno viewports SEQUENTIALLY (rate-limit bachane ke liye),
	// result buffers yahin jama — end mein sab EK SATH bhejte hain ───
	for i, vp := range ssViewports {
		pushStep(i+1, vp.label)
		buffer, err := ssMicrolinkScreenshot(targetUrl, vp, 45000)
		if err != nil {
			s.DeleteMessage(info, waitMsgID)
			s.Reply(info, ssErrorText)
			return
		}
		shots = append(shots, ssShot{vp: vp, buffer: buffer})
	}

	s.DeleteMessage(info, waitMsgID)

	// ─── Teeno images ek ke baad ek, turant bhej dete hain ───
	for _, shot := range shots {
		if err := s.SendImage(info, shot.buffer, fmt.Sprintf("*SS SIZE :❮ %s*", shot.vp.label)); err != nil {
			s.Reply(info, ssErrorText)
			return
		}
	}
}

func init() {
	Register(Command{Name: "ss", Category: "DOWNLOADER", Desc: "Take a website screenshot in tablet, PC/desktop, and Android/mobile sizes from a given link and send all three images.", Run: handleSS})
	Register(Command{Name: "screenshot", Category: "DOWNLOADER", Hidden: true, Run: handleSS})
	Register(Command{Name: "ssweb", Category: "DOWNLOADER", Hidden: true, Run: handleSS})
	Register(Command{Name: "sslink", Category: "DOWNLOADER", Hidden: true, Run: handleSS})
}
