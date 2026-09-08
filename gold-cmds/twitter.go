package goldcmds

// ============================================================================
// GOLD-MD — Twitter/X Video Downloader
// File: twitter.go
// ============================================================================
// HANDLER: handleTwitter — used by .twtsearch direct-link router
//   Downloads a Twitter/X video (best quality) and sends it with thumbnail.
//   Photo-only tweets send up to 4 photos as images.
//
// API: fxtwitter (GET https://api.fxtwitter.com/i/status/<id>)
//   Free, no API key, no strict rate limits. ~0.2s response time.
//   Response JSON: { code, message, tweet: { text, author, likes, retweets,
//     views, media: { photos, videos: [ { url, thumbnail_url, width, height,
//     duration, format, type } ] } } }
//   Video URLs point directly at video.twimg.com CDN (very fast, no proxy).
//
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const twAPIBase = "https://api.fxtwitter.com/i/status/"

const twHelpText = "*\U0001f530 TWITTER VIDEO DOWNLOAD COMMAND \U0001f530*\n" +
	"*DO YOU WANT TO DOWNLOAD A TWITTER/X VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE TWEET LINK \U0001f644*\n" +
	"*THEN WRITE LIKE THIS \U0001f60a*\n\n" +
	"*EXAMPLE :* " + "." + "twt https://x.com/NASASpaceflight/status/1811608378520588583\n\n" +
	"*TO DOWNLOAD TWITTER/X VIDEOS"

// twTweet mirrors the parts of the fxtwitter response we care about.
type twTweet struct {
	URL    string `json:"url"`
	ID     string `json:"id"`
	Text   string `json:"text"`
	Author struct {
		Name       string `json:"name"`
		ScreenName string `json:"screen_name"`
	} `json:"author"`
	Likes    int64 `json:"likes"`
	Retweets int64 `json:"retweets"`
	Replies  int64 `json:"replies"`
	Views    int64 `json:"views"`
	Media    struct {
		Photos []twPhoto `json:"photos"`
		Videos []twVideo `json:"videos"`
	} `json:"media"`
}

type twPhoto struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type twVideo struct {
	URL          string  `json:"url"`
	ThumbnailURL string  `json:"thumbnail_url"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Duration     float64 `json:"duration"`
	Format       string  `json:"format"`
	Type         string  `json:"type"`
}

// twStatusRe extracts the numeric status ID from any tweet URL shape:
// x.com, twitter.com, fxtwitter, vxtwitter, fixupx, mobile links, /statuses/.
var twStatusRe = regexp.MustCompile(`(?:twitter\.com|x\.com|fxtwitter\.com|vxtwitter\.com|fixupx\.com|fixvx\.com|twittpr\.com)/(?:[^/?#]+/)?status(?:es)?/(\d+)`)

var twBareIDRe = regexp.MustCompile(`^\d{15,20}$`)

func twExtractTweetID(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := twStatusRe.FindStringSubmatch(raw); len(m) > 1 {
		return m[1]
	}
	if twBareIDRe.MatchString(raw) {
		return raw
	}
	return ""
}

// twFetchTweet calls fxtwitter for the given status ID.
func twFetchTweet(ctx context.Context, statusID string) (*twTweet, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, twAPIBase+statusID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GOLD-MD/1.0 (+https://github.com/Uf-prince)")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API unreachable: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}
	var payload struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Tweet   *twTweet `json:"tweet"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("bad response from API")
	}
	if payload.Code != 200 || payload.Tweet == nil {
		return nil, fmt.Errorf("tweet not found, private, or deleted")
	}
	return payload.Tweet, nil
}

func handleTwitter(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// HTTP call and ffmpeg job aborts, and the user gets TRY AGAIN LATER.
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleTwitterAsync(ctx, s, info, args, prefix)
	})
}

func handleTwitterAsync(	ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	rawURL := strings.TrimSpace(strings.Join(args, " "))
	if rawURL == "" {
		s.Reply(info, twHelpText)
		return
	}
	statusID := twExtractTweetID(rawURL)
	if statusID == "" {
		s.Reply(info, "❌ *TWITTER DOWNLOAD ERROR*\nPlease provide a valid tweet link (x.com or twitter.com).")
		return
	}

	waitID := s.ReplyWithID(info, "*DOWNLOADING TWITTER VIDEO....*")

	tweet, err := twFetchTweet(ctx, statusID)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TWITTER DOWNLOAD ERROR*\n"+err.Error())
		return
	}

	// Photo-only tweets: download & send up to 4 photos as images.
	if len(tweet.Media.Videos) == 0 && len(tweet.Media.Photos) > 0 {
		s.EditMessage(info, waitID, "🖼 *Sending photo...*")
		client := mediaHTTPClient()
		caption := twBuildCaption(tweet)
		sent := 0
		total := len(tweet.Media.Photos)
		if total > 4 {
			total = 4
		}
		for i := 0; i < total; i++ {
			data := instaFetchThumbnail(ctx, client, tweet.Media.Photos[i].URL)
			if len(data) == 0 {
				continue
			}
			cap := caption
			if len(tweet.Media.Photos) > 1 {
				cap = fmt.Sprintf("%s\n*(%d/%d)*", cap, i+1, total)
			}
			if s.SendImage(info, data, cap) == nil {
				sent++
			}
		}
		s.DeleteMessage(info, waitID)
		if sent == 0 {
			s.Reply(info, "❌ COULD NOT SEND PHOTO 😢")
		}
		return
	}

	if len(tweet.Media.Videos) == 0 {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ NO VIDEO OR PHOTO IN THIS TWEET 🙃")
		return
	}

	vid := tweet.Media.Videos[0]
	if vid.URL == "" {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ VIDEO URL NOT FOUND. PLEASE TRY AGAIN 😭")
		return
	}

	s.EditMessage(info, waitID, "*DOWNLOADING VIDEO....*")

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, vid.URL, nil)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ PLEASE TRY AGAIN 🤗")
		return
	}
	defer removeTempFile(path)

	var thumb []byte
	if vid.ThumbnailURL != "" {
		thumb = instaFetchThumbnail(ctx, client, vid.ThumbnailURL)
	}

	secs, w, h := probeVideoMeta(path)
	if w == 0 && h == 0 {
		w, h = uint32(vid.Width), uint32(vid.Height)
	}

	caption := twBuildCaption(tweet)
	if err := s.SendVideoFile(info, path, caption, thumb, secs, w, h); err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, "❌ *TWITTER DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	s.DeleteMessage(info, waitID)
}

// twBuildCaption builds the standard caption for tweet media.
func twBuildCaption(tweet *twTweet) string {
	title := tweet.Text
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	if title == "" {
		title = "Twitter Video"
	}
	cap := "🔰 *TWITTER VIDEO* 🔰\n*" + title + "*\n\n"
	if tweet.Author.ScreenName != "" {
		cap += "*🔰 CREATOR :* @" + tweet.Author.ScreenName + "\n"
	} else if tweet.Author.Name != "" {
		cap += "*🔰 CREATOR :* " + tweet.Author.Name + "\n"
	}
	if tweet.Likes > 0 {
		cap += fmt.Sprintf("*🔰 LIKES :* %d\n", tweet.Likes)
	}
	if tweet.Views > 0 {
		cap += fmt.Sprintf("*🔰 VIEWS :* %d\n", tweet.Views)
	}
	cap += "\n*TWITTER VIDEO DOWNLOADED*"
	return cap
}
