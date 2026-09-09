package goldcmds

// ============================================================================
// GOLD-MD — Facebook Video Downloader
// File: fb.go
// ============================================================================
// HANDLER: handleFB — used by .fbsearch direct-link router
//   Downloads a Facebook video (HD preferred, falls back to SD) and sends it.
//
// API: cobalt instance  (POST https://cobalt-api-ufprince.onrender.com/)
//   Headers:
//     Authorization: Api-Key uf_428765ffed6c4cf9a4c746b517f9089d
//     Accept: application/json
//     Content-Type: application/json
//   Body: { "url": "<facebook url>" }
//   Response JSON: { status, url, filename }   (status: redirect/tunnel/
//   stream/picker/error)
//
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const fbCobaltAPI = "https://cobalt-api-ufprince.onrender.com/"
const fbCobaltAPIKey = "uf_428765ffed6c4cf9a4c746b517f9089d"

const fbHelpText = "*\U0001f3c5 FACEBOOK VIDEO DOWNLOAD COMMAND \U0001f3c5*\n" +
	"*DO YOU WANT TO DOWNLOAD A FACEBOOK VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE FACEBOOK VIDEO LINK \U0001f644*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*.FB \u2770FACEBOOK VIDEO LINK\u2771*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR FACEBOOK VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"

// fbCobaltResponse models the cobalt API response.
type fbCobaltResponse struct {
	Status   string `json:"status"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Error    string `json:"error"`
	Picker   []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"picker"`
}

func handleFB(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleFBAsync(ctx, s, info, args, prefix)
	})
}

func handleFBAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	fbURL := strings.TrimSpace(strings.Join(args, " "))
	if fbURL == "" {
		s.Reply(info, fbHelpText)
		return
	}
	if !strings.Contains(fbURL, "facebook.com") && !strings.Contains(fbURL, "fb.watch") && !strings.Contains(fbURL, "fb.com") {
		s.Reply(info, "❌ *FACEBOOK DOWNLOAD ERROR*\nPlease provide a valid Facebook link.")
		return
	}

	waitID := s.ReplyWithID(info, "⏳ *Fetching Facebook video...*")

	resp, err := fbCobaltFetch(ctx, fbURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *FACEBOOK DOWNLOAD ERROR*\n"+err.Error())
		return
	}

	// Resolve the video URL from the cobalt response.
	videoURL, quality := fbResolveVideoURL(resp)
	if videoURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ YOUR FACEBOOK VIDEO WAS NOT FOUND 😓")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, videoURL, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ PLEASE TRY AGAIN 🤗")
		return
	}
	defer removeTempFile(path)

	title := "Facebook Video"
	if resp.Filename != "" {
		title = strings.TrimSuffix(resp.Filename, ".mp4")
	}
	secs, w, h := probeVideoMeta(path)
	if secs == 0 && w == 0 {
		quality = "HD"
	}
	caption := "*🏅 FACEBOOK VIDEO NAME 🏅*\n" +
		"*" + title + "*\n\n" +
		"*🏅 QUALITY :* " + quality + "\n\n" +
		"*FACEBOOK VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, nil, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *FACEBOOK DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// fbResolveVideoURL picks the best media URL from a cobalt response.
func fbResolveVideoURL(resp *fbCobaltResponse) (videoURL string, quality string) {
	switch resp.Status {
	case "redirect", "tunnel", "stream":
		if resp.URL != "" {
			return resp.URL, "HD"
		}
	case "picker":
		// Picker items are usually images; grab the first video if present.
		for _, item := range resp.Picker {
			if item.Type == "video" && item.URL != "" {
				return item.URL, "HD"
			}
		}
		if len(resp.Picker) > 0 && resp.Picker[0].URL != "" {
			return resp.Picker[0].URL, "SD"
		}
	}
	return "", ""
}

// fbCobaltFetch calls the cobalt API and returns the parsed response.
func fbCobaltFetch(ctx context.Context, fbURL string) (*fbCobaltResponse, error) {
	body, err := json.Marshal(map[string]string{"url": fbURL})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fbCobaltAPI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Api-Key "+fbCobaltAPIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		fbDebug("cobalt_fail", map[string]any{"url": fbURL, "error": err.Error()})
		return nil, fmt.Errorf("API request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		fbDebug("cobalt_http_error", map[string]any{"url": fbURL, "status": res.StatusCode})
		return nil, fmt.Errorf("API returned status %d", res.StatusCode)
	}
	var parsed fbCobaltResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&parsed); err != nil {
		fbDebug("cobalt_parse_error", map[string]any{"url": fbURL, "error": err.Error()})
		return nil, fmt.Errorf("failed to parse API response: %v", err)
	}
	fbDebug("cobalt_response", map[string]any{
		"req_url":  fbURL,
		"status":   parsed.Status,
		"resp_url": parsed.URL,
		"error":    parsed.Error,
	})
	if parsed.Status == "error" {
		if parsed.Error != "" {
			return nil, fmt.Errorf("%s", parsed.Error)
		}
		return nil, fmt.Errorf("API returned error status")
	}
	return &parsed, nil
}

// FBCobaltFetchLive - exported wrapper for the live sandbox test binary.
func FBCobaltFetchLive(ctx context.Context, url string) (*fbCobaltResponse, error) {
	return fbCobaltFetch(ctx, url)
}

// FBResolveVideoURLLive - exported wrapper for the live sandbox test binary.
func FBResolveVideoURLLive(resp *fbCobaltResponse) (string, string) {
	return fbResolveVideoURL(resp)
}
