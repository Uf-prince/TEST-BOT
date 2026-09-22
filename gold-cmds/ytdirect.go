package goldcmds

// ============================================================================
// GOLD-MD — Direct YouTube fetch (no third-party API)
// File: ytdirect.go
// ============================================================================
// Uses the public Innertube player endpoint with the ANDROID_VR client.
// This client returns pre-signed googlevideo URLs (no signature deciphering)
// for itag 18 (360p MP4, H.264+AAC combined) and itag 140 (m4a audio).
// Result is mapped into WSFastResponse so existing download pipelines work
// unchanged. This is tried FIRST (fast, ~1s); loader.to is the fallback.
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ── Direct fetch constants ──────────────────────────────────────────────────
const (
	ytDirectPlayerURL = "https://www.youtube.com/youtubei/v1/player?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	ytDirectUserAgent = "com.google.android.apps.youtube.vr.oculus/1.61.48 (Linux; U; Android 12; eureka-user Build/SQ3A.220605.009.A1) gzip"
	ytDirectItagVideo = 18  // 360p MP4, H.264 + AAC combined (WhatsApp-ready)
	ytDirectItagAudio = 140 // m4a audio, AAC 128k
)

// ── Innertube player response structs (only what we need) ──────────────────
type ytDirectPlayerResp struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
	} `json:"playabilityStatus"`
	VideoDetails struct {
		Title     string `json:"title"`
		Author    string `json:"author"`
		ViewCount string `json:"viewCount"`
		LengthSec string `json:"lengthSeconds"`
		Thumb     struct {
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
		} `json:"thumbnail"`
	} `json:"videoDetails"`
	StreamingData struct {
		Formats []struct {
			Itag     int    `json:"itag"`
			URL      string `json:"url"`
			MimeType string `json:"mimeType"`
		} `json:"formats"`
		AdaptiveFormats []struct {
			Itag     int    `json:"itag"`
			URL      string `json:"url"`
			MimeType string `json:"mimeType"`
		} `json:"adaptiveFormats"`
	} `json:"streamingData"`
}

// fetchYTDirect resolves a YouTube URL directly via Innertube (ANDROID_VR).
// Returns a WSFastResponse-shaped result or an error. No API key, no
// third-party service — direct from Google servers, usually under 1 second.
func fetchYTDirect(ctx context.Context, client *http.Client, videoURL string) (*WSFastResponse, error) {
	videoID := ytDirectExtractID(videoURL)
	if videoID == "" {
		return nil, fmt.Errorf("no video id in url")
	}

	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":        "ANDROID_VR",
				"clientVersion":     "1.61.48",
				"deviceMake":        "Oculus",
				"deviceModel":       "Quest 3",
				"androidSdkVersion": 32,
				"osName":            "Android",
				"osVersion":         "12",
				"hl":                "en",
				"gl":                "US",
			},
		},
		"videoId": videoID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ytDirectPlayerURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ytDirectUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("player api status %d", resp.StatusCode)
	}

	var pr ytDirectPlayerResp
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, err
	}
	if pr.PlayabilityStatus.Status != "OK" {
		return nil, fmt.Errorf("playability %s", pr.PlayabilityStatus.Status)
	}

	var out WSFastResponse
	out.Success = true
	out.Metadata.Title = pr.VideoDetails.Title
	out.Metadata.Author = pr.VideoDetails.Author
	out.Metadata.URL = "https://www.youtube.com/watch?v=" + videoID

	if n, err := strconv.Atoi(pr.VideoDetails.ViewCount); err == nil {
		out.Metadata.Views = n
	}
	if sec, err := strconv.Atoi(pr.VideoDetails.LengthSec); err == nil && sec > 0 {
		out.Metadata.Duration = ytDirectFmtDuration(sec)
	}
	if n := len(pr.VideoDetails.Thumb.Thumbnails); n > 0 {
		out.Metadata.Thumbnail = pr.VideoDetails.Thumb.Thumbnails[n-1].URL
	}

	for _, f := range pr.StreamingData.Formats {
		if f.Itag == ytDirectItagVideo && f.URL != "" {
			out.Result.VideoURL = f.URL
			break
		}
	}
	for _, f := range pr.StreamingData.AdaptiveFormats {
		if f.Itag == ytDirectItagAudio && f.URL != "" {
			out.Result.AudioURL = f.URL
			break
		}
	}

	if out.Result.VideoURL == "" && out.Result.AudioURL == "" {
		return nil, fmt.Errorf("no usable stream urls")
	}
	return &out, nil
}

// ytDirectExtractID pulls the 11-char video id from any YouTube link form.
func ytDirectExtractID(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	// bare id
	if len(s) == 11 && !strings.ContainsAny(s, "./?=") {
		return s
	}
	for _, marker := range []string{"watch?v=", "youtu.be/", "shorts/", "embed/", "live/"} {
		if i := strings.Index(s, marker); i >= 0 {
			rest := s[i+len(marker):]
			if amp := strings.IndexAny(rest, "&?#/"); amp >= 0 {
				rest = rest[:amp]
			}
			if len(rest) >= 11 {
				return rest[:11]
			}
		}
	}
	// v= param fallback
	for _, part := range strings.Split(s, "&") {
		if strings.HasPrefix(part, "v=") && len(part) >= 13 {
			return part[2:13]
		}
	}
	return ""
}

// ytDirectFmtDuration renders seconds as h:mm:ss or m:ss.
func ytDirectFmtDuration(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ytDirectTimeout is the HTTP client used for the direct player call.
func ytDirectClient() *http.Client {
	return &http.Client{Timeout: 25 * time.Second}
}
