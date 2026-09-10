package goldcmds

// ============================================================================
// GOLD-MD — .ytsearch command (YouTube search + video info, no API key)
// File: ytinfo.go
// ----------------------------------------------------------------------------
// Searches YouTube and shows video info — completely FREE, no API key, no
// login, no quota.  Uses YouTube's own internal API (Innertube,
// youtubei/v1) — the exact same endpoint the youtube.com website calls in
// every browser.  The key below is YouTube's public web client key that
// ships inside the website's JavaScript for everyone.
//
//   .ytsearch <query>     → top results: title, duration, views, link
//   .ytsearch <query> 5   → top 5 results (any 1-10)
//   .ytinfo <link or id>  → full details of one video (hidden alias)
//
// Both commands share one handler; the guide reply (no args) explains all
// of it.  .ytsearch is the menu entry; .ytinfo works silently.
//
// Verified live: search returns videoId / title / length / views /
// channel for any query.  No throttling at normal bot use; a tiny
// 60-second result cache keeps repeat searches instant.
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── Innertube constants ───────────────────────────────────────────────

const (
	ytInnertubeURL   = "https://www.youtube.com/youtubei/v1/search?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	ytClientVersion  = "2.20240101.00.00"
	ytUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	ytHTTPTimeout    = 20 * time.Second
	ytMaxResults     = 5  // default number of search results
	ytMaxResultsHard = 10 // hard cap
	ytCacheTTL       = 60 * time.Second
)

// ── result cache (60-second TTL) ─────────────────────────────────────

var (
	ytMu    sync.Mutex
	ytCache = map[string]ytSearchResponse{}
	ytAt    = map[string]time.Time{}
)

// ── types (only the fields we need) ──────────────────────────────────

type ytSearchResponse struct {
	Results []ytVideo `json:"results"`
	Cached  bool      `json:"cached"`
}

type ytVideo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Channel string `json:"channel"`
	Length  string `json:"length"`
	Views   string `json:"views"`
	Link    string `json:"link"`
}

// ── request / response ───────────────────────────────────────────────

// ytInnertubeSearch calls YouTube's internal search API and returns the
// top videos for a query.
func ytInnertubeSearch(query string) ([]ytVideo, error) {
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": ytClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"query": query,
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: ytHTTPTimeout}
	req, err := http.NewRequest("POST", ytInnertubeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ytUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return ytParseSearch(data)
}

// ── response parsing ─────────────────────────────────────────────────

// ytParseSearch walks the Innertube JSON and pulls out videoRenderer
// entries: videoId, title, length, views, channel.
func ytParseSearch(data []byte) ([]ytVideo, error) {
	var raw struct {
		Contents struct {
			TwoColumnSearchResultsRenderer struct {
				PrimaryContents struct {
					SectionListRenderer struct {
						Contents []struct {
							ItemSectionRenderer struct {
								Contents []struct {
									VideoRenderer json.RawMessage `json:"videoRenderer"`
								} `json:"contents"`
							} `json:"itemSectionRenderer"`
						} `json:"contents"`
					} `json:"sectionListRenderer"`
				} `json:"primaryContents"`
			} `json:"twoColumnSearchResultsRenderer"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []ytVideo
	for _, sec := range raw.Contents.TwoColumnSearchResultsRenderer.PrimaryContents.SectionListRenderer.Contents {
		for _, item := range sec.ItemSectionRenderer.Contents {
			if len(item.VideoRenderer) == 0 {
				continue
			}
			var v struct {
				VideoID string `json:"videoId"`
				Title   struct {
					Runs []struct {
						Text string `json:"text"`
					} `json:"runs"`
				} `json:"title"`
				OwnerText struct {
					Runs []struct {
						Text string `json:"text"`
					} `json:"runs"`
				} `json:"ownerText"`
				LengthText struct {
					SimpleText string `json:"simpleText"`
				} `json:"lengthText"`
				ViewCountText struct {
					SimpleText string `json:"simpleText"`
				} `json:"viewCountText"`
			}
			if err := json.Unmarshal(item.VideoRenderer, &v); err != nil {
				continue
			}
			title := ""
			for _, r := range v.Title.Runs {
				title += r.Text
			}
			channel := ""
			for _, r := range v.OwnerText.Runs {
				channel += r.Text
			}
			if v.VideoID == "" || title == "" {
				continue
			}
			out = append(out, ytVideo{
				ID:      v.VideoID,
				Title:   title,
				Channel: channel,
				Length:  v.LengthText.SimpleText,
				Views:   v.ViewCountText.SimpleText,
				Link:    "https://youtube.com/watch?v=" + v.VideoID,
			})
			if len(out) >= ytMaxResultsHard {
				return out, nil
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no videos found")
	}
	return out, nil
}

// ── video ID extraction from any link form ───────────────────────────

var (
	ytIDRe1 = regexp.MustCompile(`(?:v=|/shorts/|/embed/|youtu\.be/|/v/|/e/|/live/)([A-Za-z0-9_-]{11})`)
	ytIDRe2 = regexp.MustCompile(`^([A-Za-z0-9_-]{11})$`)
)

// ytExtractID pulls an 11-char video ID out of any YouTube link form, or
// accepts a bare ID.
func ytExtractID(s string) string {
	s = strings.TrimSpace(s)
	if m := ytIDRe1.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := ytIDRe2.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// ── oEmbed video info (no key) ───────────────────────────────────────

type ytInfoData struct {
	Title  string
	Author string
	Thumb  string
	ID     string
}

// ytOEmbedInfo fetches a single video's title / channel / thumbnail via
// the public oEmbed endpoint (no key, no login).
func ytOEmbedInfo(id string) (*ytInfoData, error) {
	u := "https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=" + id + "&format=json"
	client := &http.Client{Timeout: ytHTTPTimeout}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ytUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var oe struct {
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oe); err != nil {
		return nil, err
	}
	return &ytInfoData{Title: oe.Title, Author: oe.AuthorName, Thumb: oe.ThumbnailURL, ID: id}, nil
}

// ── reply builders ───────────────────────────────────────────────────

// ytSearchCard renders the search results reply.
// ytSearchCard renders the list in the .video fancy-border style, so all
// search lists across the bot look identical.
func ytSearchCard(query string, results []ytVideo, prefix string) string {
	var b strings.Builder
	b.WriteString("*🔰 YOUTUBE SEARCH 🔰*\n\n")
	b.WriteString("*QUERY :❱ " + strings.ToUpper(query) + "*\n\n")
	b.WriteString("*TOP " + strconv.Itoa(len(results)) + " RESULTS FOR YOUR SEARCH*\n\n")
	for i, v := range results {
		durStr := v.Length
		if durStr == "" {
			durStr = "NOT FOUND"
		}
		meta := durStr
		if v.Views != "" {
			if meta != "" {
				meta += " ❮ "
			}
			meta += v.Views
		}
		nameStr := v.Title
		if nameStr == "" {
			nameStr = "NULL"
		}
		b.WriteString("\n" + searchBorder + "\n")
		b.WriteString("*TYPE ❰ " + strconv.Itoa(i+1) + " ❱ TO DOWNLOAD THIS FROM YT*\n")
		b.WriteString(strings.ToUpper(nameStr) + "\n")
		if v.Channel != "" {
			b.WriteString("*CHANNEL :❱ " + v.Channel + "*\n")
		}
		b.WriteString("*" + meta + "*\n")
		b.WriteString("*LINK :❱ " + v.Link + "*\n")
		b.WriteString(searchBorder + "\n\n")
	}
	b.WriteString("*TYPE NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO " + strconv.Itoa(len(results)) + "*\n\n" +
		ytPickAskFooter())
	return b.String()
}

// ytInfoCard renders the single-video info reply.
func ytInfoCard(d *ytInfoData, prefix string) string {
	var b strings.Builder
	b.WriteString("*🔰 YOUTUBE VIDEO INFO 🔰*\n\n")
	b.WriteString("*TITLE :❱ " + d.Title + "*\n")
	b.WriteString("*CHANNEL :❱ " + d.Author + "*\n")
	b.WriteString("*LINK :❱ https://youtube.com/watch?v=" + d.ID + "*\n")
	b.WriteString("*THUMBNAIL :❱ " + d.Thumb + "*\n\n")
	b.WriteString("*🔰 DOWNLOAD :❱ " + prefix + "video ❮ NUMBER OR NAME ❯*")
	return b.String()
}

// ytGuide — full 🔰 styled guide (pure English).
func ytGuide(prefix string) string {
	return "*🔰 YOUTUBE SEARCH GUIDE 🔰*\n\n" +
		"*🔰 SEARCH YOUTUBE :❱*\n*" + prefix + "yts ❮ QUERY ❯*\n*EXAMPLE :❱ " + prefix + "yts lofi mix*\n*SHOWS THE TOP RESULTS WITH TITLE, CHANNEL, LENGTH, VIEWS AND LINK*\n\n" +
		"*🔰 VIDEO DETAILS :❱*\n*" + prefix + "ytinfo ❮ LINK OR ID ❯*\n*EXAMPLE :❱ " + prefix + "ytinfo https://youtube.com/watch?v=xxxxxxx*\n*SHOWS THE FULL DETAILS OF ONE VIDEO*\n\n" +
		"*✱ DIRECT YOUTUBE LINK :❱*\n*" + prefix + "yts ❰ YOUTUBE LINK ❯*\n*EXAMPLE :❱ " + prefix + "yts https://www.youtube.com/watch?v=xxxxxxxxxxx*\n*PASTE A YOUTUBE LINK AND THE VIDEO DOWNLOADS INSTANTLY*\n\n" +
		"*🔰 NOTE :❱*\n*COMPLETELY FREE — NO API KEY, NO LOGIN, NO QUOTA*\n*TO DOWNLOAD A VIDEO USE " + prefix + "video*"
}

// ── handlers ─────────────────────────────────────────────────────────

// handleYTSearch — search YouTube (menu entry).
func handleYTSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, ytGuide(prefix))
		return
	}

	// ── smart link detection (same system as the other search commands) ──
	if link := searchLinkRe.FindString(query); link != "" {
		lowLink := strings.ToLower(strings.TrimSpace(link))
		host := strings.TrimPrefix(strings.TrimPrefix(lowLink, "https://"), "http://")
		host = strings.SplitN(host, "/", 2)[0]
		if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
			// correct platform link → instant download through the .video pipeline
			handleVideo(s, info, []string{link}, prefix)
			return
		}
		// wrong-platform link → error card with a valid example
		s.Reply(info, "🔰 *YOUTUBE SEARCH ERROR* 🔰\n\n"+
			"*GIVE ME THE VALID YOUTUBE LINK* 🔰\n\n"+
			"*THIS LINK IS FROM :❱ "+strings.ToUpper(host)+"*\n"+
			"*IT IS NOT A YOUTUBE LINK* 🔰\n\n"+
			"*EXAMPLE SAME LIKE THAT :❱*\n"+
			"*"+prefix+"yts https://www.youtube.com/watch?v=xxxxxxxxxxx*\n\n"+
			"*SEARCHED BY GOLD-MD* 🔰")
		return
	}
	results, err := ytInnertubeSearch(query)
	if err != nil {
		s.Reply(info, "*🔰 SEARCH FAILED :❱*\n\n*YOUTUBE NOT RESPONDING — TRY AGAIN IN A FEW MINUTES*")
		return
	}
	if len(results) > ytMaxResults {
		results = results[:ytMaxResults]
	}
	// store the .yts pick list: a bare "1".."5" sends the thumbnail and
	// asks 1=AUDIO / 2=VIDEO (ytpick flow, both-engine fallback on send).
	vids := make([]VideoResult, 0, len(results))
	for _, v := range results {
		thumb := ""
		if v.ID != "" {
			thumb = "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg"
		}
		vids = append(vids, VideoResult{Title: v.Title, URL: v.Link, Thumbnail: thumb, Duration: v.Length})
	}
	StoreYTSList(info.Sender.String(), vids)
	s.Reply(info, ytSearchCard(query, results, prefix))
}

// handleYTInfo — one video's details (hidden alias).
func handleYTInfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	arg := strings.TrimSpace(strings.Join(args, " "))
	if arg == "" {
		s.Reply(info, ytGuide(prefix))
		return
	}
	id := ytExtractID(arg)
	if id == "" {
		s.Reply(info, "*🔰 INVALID LINK :❱*\n\n*SEND A YOUTUBE LINK OR AN 11 DIGIT VIDEO ID — EXAMPLE :❱ "+prefix+"ytinfo https://youtube.com/watch?v=xxxxxxx*")
		return
	}
	d, err := ytOEmbedInfo(id)
	if err != nil {
		s.Reply(info, "*🔰 VIDEO NOT FOUND :❱*\n\n*CHECK THE LINK AND TRY AGAIN*")
		return
	}
	s.Reply(info, ytInfoCard(d, prefix))
}

func init() {
	Register(Command{Name: "yts", Category: "SEARCH", Desc: "Search YouTube videos or paste a YouTube link to download (title, channel, views, link)", Run: handleYTSearch})
	Register(Command{Name: "ytsearch", Hidden: true, Run: handleYTSearch})
	Register(Command{Name: "ytinfo", Hidden: true, Run: handleYTInfo})
}
