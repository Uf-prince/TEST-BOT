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
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const twAPIBase = "https://api.fxtwitter.com/i/status/"

const twHelpText = "*\U0001f530 TWITTER VIDEO DOWNLOAD COMMAND \U0001f530*\n" +
	"*DO YOU WANT TO DOWNLOAD A TWITTER/X VIDEO? \U0001f914*\n" +
	"*FIRST COPY THE TWEET OR PROFILE LINK \U0001f644*\n" +
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
	kind, kvalue := twClassifyLink(rawURL)
	if kind == "" {
		s.Reply(info, "🔰 *TWITTER DOWNLOAD ERROR*\nPlease provide a valid tweet or profile link (x.com / twitter.com).")
		return
	}

	// owner fix: profile link (x.com/NASA) -> jina se latest VIDEO tweet
	// resolve (video-first — photos sirf fallback)
	statusID := kvalue
	if kind == "profile" {
		var rerr error
		statusID, rerr = twProfileLatestVideoStatusID(ctx, kvalue)
		if rerr != nil {
			s.Reply(info, "🔰 *TWITTER DOWNLOAD ERROR*\nProfile could not be resolved - paste a direct tweet link.")
			return
		}
	}

	tweet, err := twFetchTweet(ctx, statusID)
	if err != nil {
		s.Reply(info, "🔰 *TWITTER DOWNLOAD ERROR*\n"+err.Error())
		return
	}

	// OWNER RULE v3 (thumbnail card): rich preview card — title/creator/
	// likes ke sath "TWITTER VIDEO DOWNLOADING / PLEASE WAIT....". Ye
	// card video/photo send hone ke baad bhi DELETE NAHI hota.
	waitID := s.ReplyWithID(info, twPreviewCard(tweet))

	// Photo-only tweets: download & send up to 4 photos as images.
	if len(tweet.Media.Videos) == 0 && len(tweet.Media.Photos) > 0 {
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
		if sent == 0 {
			s.EditMessage(info, waitID, "🔰 *TWITTER DOWNLOAD ERROR*\nCould not send photo - please try again.")
		}
		return
	}

	if len(tweet.Media.Videos) == 0 {
		s.EditMessage(info, waitID, "🔰 NO VIDEO OR PHOTO IN THIS TWEET 🔰")
		return
	}

	vid := tweet.Media.Videos[0]
	if vid.URL == "" {
		s.EditMessage(info, waitID, "🔰 VIDEO URL NOT FOUND. PLEASE TRY AGAIN 🔰")
		return
	}

	client := mediaHTTPClient()
	path, err := streamDownloadToFile(ctx, client, vid.URL, nil)
	if err != nil {
		s.EditMessage(info, waitID, "🔰 *TWITTER DOWNLOAD ERROR*\nVideo could not be downloaded - please try again.")
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
		s.EditMessage(info, waitID, "🔰 *TWITTER DOWNLOAD ERROR*\nVideo could not be sent.")
		return
	}
	// OWNER RULE: preview/thumbnail card DELETE NAHI hota — video ke
	// baad bhi chat me rehta hai.
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
	cap += "\n*TWITTER VIDEO DOWNLOADING*\n*PLEASE WAIT....*"
	return cap
}

// twPreviewCard builds the DOWNLOADING preview card (OWNER RULE v3):
// caption card jaisa hi structure (title / creator / likes / views) lekin
// ending "TWITTER VIDEO DOWNLOADING" + ek line niche "PLEASE WAIT....".
// Ye card download start hone pe chala jata hai aur video/photo aane ke
// baad bhi DELETE NAHI hota — thumbnail card chat me rehta hai.
func twPreviewCard(tweet *twTweet) string {
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
	cap += "\n*TWITTER VIDEO DOWNLOADING*\n*PLEASE WAIT....*"
	return cap
}


// ── X / Twitter link classifier + profile resolver (owner fix) ────────────
// Bing/DDG search results ACCOUNT links dete hain (x.com/NASA), aur user
// bhi profile link paste kar sakta hai. twExtractTweetID sirf /status/
// links pe ID deta tha — profile pe SILENT FAIL hota tha (na download, na
// error). Ye helpers har x/twitter link ko ek status ID me convert karte
// hain (TG tgLatestPost jaisa pattern).

// twProfileRe matches a bare profile URL (optional scheme/host, optional
// known sub-paths like /with_replies, /media ...).
var twProfileRe = regexp.MustCompile(`^(?:https?://)?(?:www\.|mobile\.)?(?:twitter|x)\.com/([A-Za-z0-9_]{1,15})(?:/(?:with_replies|media|photo|video|search|likes|highlights|articles|followers|following|professional-relationships))*/*$`)

// twJinaStatusRe pulls user + status id out of jina profile markdown
// (photo/video links look like twitter.com/<user>/status/<id>/photo/1).
var twJinaStatusRe = regexp.MustCompile(`(?:twitter|x)\.com/([A-Za-z0-9_]{1,15})/status(?:es)?/(\d{6,20})`)

// twClassifyLink returns ("status", id) | ("profile", handle) | ("", "").
// Bare numeric IDs (15-20 digits) count as status (twExtractTweetID ke
// bare-ID branch ke through).
func twClassifyLink(raw string) (kind, value string) {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if id := twExtractTweetID(raw); id != "" {
		return "status", id
	}
	if m := twProfileRe.FindStringSubmatch(raw); m != nil {
		return "profile", m[1]
	}
	return "", ""
}

// twProfileStatusIDs fetches the profile page through the jina reader proxy
// and returns ALL status IDs newest-first. Twitter snowflake IDs time ke
// sath grow karte hain -> numeric sort = chronological order (pinned PURANE
// tweets document me top pe hote hain, sort unhe sahi jagah rakh deta hai).
func twProfileStatusIDs(ctx context.Context, handle string) ([]string, error) {
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if handle == "" {
		return nil, fmt.Errorf("empty handle")
	}
	md, err := jinaFetch(ctx, "https://twitter.com/"+handle)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range twJinaStatusRe.FindAllStringSubmatch(md, -1) {
		if !seen[m[2]] {
			seen[m[2]] = true
			ids = append(ids, m[2])
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no tweets found")
	}
	sort.Slice(ids, func(i, j int) bool {
		a, ea := strconv.ParseInt(ids[i], 10, 64)
		b, eb := strconv.ParseInt(ids[j], 10, 64)
		if ea == nil && eb == nil {
			return a > b
		}
		return ids[i] > ids[j]
	})
	return ids, nil
}

// twProfileLatestStatusID returns the newest tweet ID (any media type).
func twProfileLatestStatusID(ctx context.Context, handle string) (string, error) {
	ids, err := twProfileStatusIDs(ctx, handle)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// twProfileLatestVideoStatusID — OWNER RULE (video-first): profile ke recent
// tweets newest-first scan karke pehla VIDEO tweet return karta hai (photos
// nahi — user ne bola tha bot sirf photos de raha tha). Agar recent me koi
// video nahi hai to latest PHOTO tweet, warna latest tweet — taake photo-only
// accounts bhi kaam karte rahein.
func twProfileLatestVideoStatusID(ctx context.Context, handle string) (string, error) {
	ids, err := twProfileStatusIDs(ctx, handle)
	if err != nil {
		return "", err
	}
	if len(ids) > 8 {
		ids = ids[:8]
	}
	photoFallback := ""
	for _, id := range ids {
		tw, ferr := twFetchTweet(ctx, id)
		if ferr != nil || tw == nil {
			continue
		}
		if len(tw.Media.Videos) > 0 && tw.Media.Videos[0].URL != "" {
			return id, nil
		}
		if photoFallback == "" && len(tw.Media.Photos) > 0 {
			photoFallback = id
		}
	}
	if photoFallback != "" {
		return photoFallback, nil
	}
	return ids[0], nil
}

// twResolveLinkAny converts ANY x/twitter link (tweet OR profile) into a
// status ID — profile links resolve via jina (latest tweet).
func twResolveLinkAny(ctx context.Context, raw string) (string, error) {
	kind, value := twClassifyLink(raw)
	switch kind {
	case "status":
		return value, nil
	case "profile":
		return twProfileLatestVideoStatusID(ctx, value)
	}
	return "", fmt.Errorf("not an x.com / twitter.com link")
}
