package goldcmds

// ============================================================================
// GOLD-MD — Instagram Downloader
// File: insta.go
// ============================================================================
// HANDLER: handleInsta — used by .igsearch direct-link router
//   Downloads an Instagram video (best quality) and sends it with thumbnail.
//
// SOURCE: self-contained page scraper (see fb.go). The old ytdlp metadata
//   service (ytdlp-ufprince.onrender.com) was SUSPENDED by Render (HTTP 503),
//   so metadata + the direct .mp4 URL are now read straight from the public
//   Instagram page (fetched with a Googlebot User-Agent):
//     - video URL  : video_versions[].url   (direct CDN .mp4)
//     - title      : og:title
//     - creator    : og:description ("... - <user> on <date>: ...")
//     - thumbnail  : og:image
//   No API key, no third-party service.
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const instaHelpText = "*\U0001f3c5 INSTAGRAM VIDEO DOWNLOAD COMMAND \U0001f3c5*\n" +
	"*DO YOU WANT TO DOWNLOAD AN INSTAGRAM VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE INSTAGRAM VIDEO LINK \U0001f644*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*.IG \u2770INSTAGRAM VIDEO LINK\u2771*\n\n" +
	"*WHEN YOU WRITE LIKE THIS YOUR INSTAGRAM VIDEO WILL BE DOWNLOADED AND SENT HERE \U0001f917*"

// instaMetaFormat models one downloadable format.
type instaMetaFormat struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
	Ext    string `json:"ext"`
}

// instaMetaInfo models the metadata used to build the caption.
type instaMetaInfo struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Channel    string            `json:"channel"`
	Uploader   string            `json:"uploader"`
	Thumbnail  string            `json:"thumbnail"`
	LikeCount  int64             `json:"like_count"`
	CommentCnt int64             `json:"comment_count"`
	Formats    []instaMetaFormat `json:"formats"`
}

func handleInsta(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeoutDur(s, info, socialTimeout, downloaderTimeoutReplyText, func(ctx context.Context) {
		handleInstaAsync(ctx, s, info, args, prefix)
	})
}

func handleInstaAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	instaURL := strings.TrimSpace(strings.Join(args, " "))
	if instaURL == "" {
		s.Reply(info, instaHelpText)
		return
	}
	if !strings.Contains(instaURL, "instagram.com") && !strings.Contains(instaURL, "instagr.am") {
		s.Reply(info, "\U0001f530 *INSTAGRAM DOWNLOAD ERROR*\nPlease provide a valid Instagram link.")
		return
	}

	waitID := s.ReplyWithID(info, "\U0001f530 *Fetching Instagram media...*")

	meta, err := instaFetchMeta(ctx, instaURL)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 *INSTAGRAM DOWNLOAD ERROR*\n"+err.Error())
		return
	}

	// Best quality = last format; lower quality = height 640.
	bestURL := ""
	if len(meta.Formats) > 0 {
		bestURL = meta.Formats[len(meta.Formats)-1].URL
	}
	lowURL := instaFindFormatByHeight(meta, 640)
	if bestURL == "" {
		bestURL = lowURL
	}
	if bestURL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 VIDEO URL NOT FOUND. PLEASE TRY AGAIN \U0001f530")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, bestURL, nil)
	if err != nil && lowURL != "" && lowURL != bestURL {
		// Retry once with the lower-quality URL.
		s.EditMessage(info, waitID, "\U0001f530 *Retrying with lower quality...*")
		path, err = streamDownloadToFile(ctx, client, lowURL, nil)
	}
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 PLEASE TRY AGAIN \U0001f530")
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
	caption := "*\U0001f530 INSTAGRAM VIDEO NAME \U0001f530*\n" +
		"*" + title + "*\n\n"
	if author != "" {
		caption += "*\U0001f530 CREATOR :* " + author + "\n"
	}
	if meta.LikeCount > 0 {
		caption += fmt.Sprintf("*\U0001f530 LIKES :* %d\n", meta.LikeCount)
	}
	caption += "\n*INSTAGRAM VIDEO DOWNLOAD*"

	if err := s.SendVideoFile(info, path, caption, thumb, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "\U0001f530 *INSTAGRAM DOWNLOAD ERROR*\nVideo could not be sent.")
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
	req.Header.Set("User-Agent", fbUserAgent)
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

// instaFetchMeta scrapes the public Instagram page (Googlebot UA) and returns
// the direct .mp4 URL plus title / creator / thumbnail. No API key needed.
func instaFetchMeta(ctx context.Context, instaURL string) (*instaMetaInfo, error) {
	resp, err := fbScrapeIG(ctx, instaURL)
	if err != nil {
		return nil, err
	}
	videoURL, _ := fbResolveVideoURL(resp)
	if videoURL == "" {
		return nil, fmt.Errorf("no downloadable video found for this link")
	}

	title, owner, thumb := igScrapeMeta(ctx, instaURL)

	meta := &instaMetaInfo{
		Title:     title,
		Uploader:  owner,
		Thumbnail: thumb,
		Formats:   []instaMetaFormat{{URL: videoURL}},
	}
	if meta.Title == "" {
		meta.Title = instaTitleFromFilename(resp.Filename)
	}
	return meta, nil
}

// instaTitleFromFilename turns a filename like "instagram_DcgpLPDidLj.mp4"
// into a friendly title ("Instagram DcgpLPDidLj").
func instaTitleFromFilename(filename string) string {
	filename = strings.TrimSuffix(strings.TrimSpace(filename), ".mp4")
	filename = strings.TrimPrefix(filename, "instagram_")
	if filename == "" {
		return "Instagram Video"
	}
	return "Instagram " + filename
}
