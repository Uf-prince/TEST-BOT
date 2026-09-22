package goldcmds

// ============================================================================
// GOLD-MD — Instagram Downloader
// File: insta.go
// ============================================================================
// HANDLER: handleInsta — used by .igsearch direct-link router
//   Downloads an Instagram video (best quality) and sends it with thumbnail.
//
// API: ytdlp metadata service  (POST https://ytdlp-ufprince.onrender.com/api/metadata)
//   Body: { "url": "<instagram url>" }
//   Response: JSON array — first element holds the metadata:
//     title      : "Video by hustlerera_"
//     thumbnail  : thumbnail JPG URL
//     channel    : uploader username
//     formats    : [ { url, height, ... }, ... ]
//   Best quality  = info["formats"][-1]["url"]        (direct MP4)
//   Lower quality = first format with height == 640
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

const instaMetaAPI = "https://ytdlp-ufprince.onrender.com/api/metadata"

const instaHelpText = "*\U0001f3c5 INSTAGRAM VIDEO DOWNLOAD COMMAND \U0001f3c5*\n" +
	"*DO YOU WANT TO DOWNLOAD AN INSTAGRAM VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE INSTAGRAM VIDEO LINK \U0001f644*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*.IG \u2770INSTAGRAM VIDEO LINK\u2771*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR INSTAGRAM VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"

// instaMetaFormat models one entry of the ytdlp formats array.
type instaMetaFormat struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
	Ext    string `json:"ext"`
}

// instaMetaInfo models info = response[0] from the ytdlp metadata API.
type instaMetaInfo struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Channel     string            `json:"channel"`
	Uploader    string            `json:"uploader"`
	Thumbnail   string            `json:"thumbnail"`
	LikeCount   int64             `json:"like_count"`
	CommentCnt  int64             `json:"comment_count"`
	Formats     []instaMetaFormat `json:"formats"`
}

func handleInsta(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutDur(s, info, socialTimeout, downloaderTimeoutReplyText, func(ctx context.Context) {
		handleInstaAsync(ctx, s, info, args, prefix)
	})
}

func handleInstaAsync(	ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	instaURL := strings.TrimSpace(strings.Join(args, " "))
	if instaURL == "" {
		s.Reply(info, instaHelpText)
		return
	}
	if !strings.Contains(instaURL, "instagram.com") && !strings.Contains(instaURL, "instagr.am") {
		s.Reply(info, "🔰 *INSTAGRAM DOWNLOAD ERROR*\nPlease provide a valid Instagram link.")
		return
	}

	waitID := s.ReplyWithID(info, "🔰 *Fetching Instagram media...*")

	// FAST PATH: try cobalt first (~1-3s). A direct “redirect” CDN URL can
	// be downloaded straight away; “tunnel” URLs are slow (Render proxy) so
	// those fall back to the metadata API for thumbnail + quality ladder.
	var meta *instaMetaInfo
	if cresp, cerr := fbCobaltFetch(ctx, instaURL); cerr == nil {
		if direct, _ := fbResolveVideoURL(cresp); direct != "" && cresp.Status == "redirect" {
			meta = &instaMetaInfo{
				Title:   instaTitleFromFilename(cresp.Filename),
				Formats: []instaMetaFormat{{URL: direct}},
			}
		}
	}
	if meta == nil {
		var err error
		meta, err = instaFetchMeta(ctx, instaURL)
		if err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "🔰 *INSTAGRAM DOWNLOAD ERROR*\n"+err.Error())
			return
		}
	}

	// Best quality = last format (per API docs); lower quality = height 640.
	bestURL := meta.Formats[len(meta.Formats)-1].URL
	lowURL := instaFindFormatByHeight(meta, 640)
	if bestURL == "" {
		bestURL = lowURL
	}
	if bestURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 VIDEO URL NOT FOUND. PLEASE TRY AGAIN 🔰")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, bestURL, nil)
	if err != nil && lowURL != "" && lowURL != bestURL {
		// Retry once with the lower-quality URL.
		s.EditMessage(info, waitID, "🔰 *Retrying with lower quality...*")
		path, err = streamDownloadToFile(ctx, client, lowURL, nil)
	}
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 PLEASE TRY AGAIN 🔰")
		return
	}
	// WhatsApp-compat: HEVC/mjpeg reels ko h264+faststart me convert
	if waPath, werr := whatsappifyVideo(ctx, path); werr == nil && waPath != path {
		defer removeTempFile(path)   // original raw file
		defer removeTempFile(waPath) // converted .wa.mp4 (LEAK FIX)
		path = waPath
	} else {
		defer removeTempFile(path)
	}

	// Optional thumbnail for the video preview message.
	var thumb []byte
	if meta.Thumbnail != "" {
		thumb = instaFetchThumbnail(ctx, client, meta.Thumbnail)
	}

	secs, w, h := probeVideoMeta(path)

	title := meta.Title
	if title == "" {
		title = "Instagram Video"
	}
	author := meta.Uploader
	if author == "" {
		author = meta.Channel
	}
	caption := "*🔰 INSTAGRAM VIDEO NAME 🔰*\n" +
		"*" + title + "*\n\n"
	if author != "" {
		caption += "*🔰 CREATOR :* " + author + "\n"
	}
	if meta.LikeCount > 0 {
		caption += fmt.Sprintf("*🔰 LIKES :* %d\n", meta.LikeCount)
	}
	caption += "\n*INSTAGRAM VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, thumb, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *INSTAGRAM DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// instaFindFormatByHeight returns the URL of the first format matching the
// given height (e.g. 640). Falls back to the smallest non-zero height.
func instaFindFormatByHeight(meta *instaMetaInfo, height int) string {
	var fallback string
	fallbackH := 0
	for _, f := range meta.Formats {
		if f.URL == "" {
			continue
		}
		if f.Height == height {
			return f.URL
		}
		if f.Height > 0 && (fallback == "" || f.Height < fallbackH) {
			fallback = f.URL
			fallbackH = f.Height
		}
	}
	return fallback
}

// instaFetchThumbnail downloads the thumbnail image (bounded to ~2 MB).
func instaFetchThumbnail(ctx context.Context, client *http.Client, thumbURL string) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	res, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil
	}
	return data
}

// instaFetchMeta calls the ytdlp metadata API and returns info = r.json()[0].
func instaFetchMeta(ctx context.Context, instaURL string) (*instaMetaInfo, error) {
	body, err := json.Marshal(map[string]string{"url": instaURL})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, instaMetaAPI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", res.StatusCode)
	}

	// Response is a JSON array; take the first element.
	var all []instaMetaInfo
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&all); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %v", err)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("empty API response")
	}
	return &all[0], nil
}
// instaTitleFromFilename turns a cobalt filename like "instagram_DcgpLPDidLj.mp4"
// into a friendly title ("Instagram DcgpLPDidLj").
func instaTitleFromFilename(filename string) string {
	filename = strings.TrimSuffix(strings.TrimSpace(filename), ".mp4")
	filename = strings.TrimPrefix(filename, "instagram_")
	if filename == "" {
		return "Instagram Video"
	}
	return "Instagram " + filename
}
