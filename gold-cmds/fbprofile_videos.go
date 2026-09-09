package goldcmds

// ============================================================================
// GOLD-MD - FB Profile -> Latest Video Resolver (v3 - REEL + TIMELINE support)
// ============================================================================
// v3: naye FB profiles REELS post karte hain (facebook.com/reel/ID) aur unka
// /videos tab 404 deta hai. Is liye:
//   - REEL permalink matcher  (/reel/ID + /watch/?v=ID)
//   - profile.php?id=NNN TIMELINE route (reels timeline pe hoti hain)
//   - vanity timeline route (reels wahan bhi post hoti hain)
// Pehla successful (reel ya video) permalink jeet-ta hai.
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// fbDebug - JSON debug line stderr pe (supervisor err log me jata hai).
// Zero-log rule sirf console logger ke liye hai; ye debug lines remove
// hongi jab exact waja mil jaye.
// fbDebug - JSON debug line stderr pe (supervisor err log me jata hai).
// Zero-log rule sirf console logger ke liye hai; ye debug lines remove
// hongi jab exact waja mil jaye.
func fbDebug(event string, fields map[string]any) {
	m := map[string]any{"dbg": "fb", "event": event, "ts": time.Now().UTC().Format(time.RFC3339)}
	for k, v := range fields {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stderr, string(b))
}

// fmtErr - error ko string banao (nil -> "").
func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// fbVideoLinkReFixed - /videos/ permalink matcher:
//
//	facebook.com/<user>/videos/<slug>/<id>  ya  facebook.com/<user>/videos/<id>
var fbVideoLinkReFixed = regexp.MustCompile(
	`facebook\.com/([^\s)"\']+)/videos/(?:([^\s)"\']+)/)?([0-9]{6,})/?`)

// fbReelLinkRe - REEL permalink matcher (naye profiles reels post karte hain):
//
//	facebook.com/reel/1234567890123
var fbReelLinkRe = regexp.MustCompile(`facebook\.com/reel/([0-9]{6,})/?`)

// fbWatchLinkRe - watch permalink matcher:
//
//	facebook.com/watch/?v=1234567890123
var fbWatchLinkRe = regexp.MustCompile(`facebook\.com/watch/\?v=([0-9]{6,})/?`)

// fbExtractPermalink scans markdown for a video/reel/watch permalink.
// Pehle REEL dhoondta hai (naye profiles), phir /videos/, phir /watch/.
func fbExtractPermalink(md string) string {
	if m := fbReelLinkRe.FindStringSubmatch(md); m != nil {
		return "https://www.facebook.com/reel/" + m[1]
	}
	if m := fbVideoLinkReFixed.FindStringSubmatch(md); m != nil {
		return "https://www.facebook.com/" + m[1] + "/videos/" + m[3]
	}
	if m := fbWatchLinkRe.FindStringSubmatch(md); m != nil {
		return "https://www.facebook.com/watch/?v=" + m[1]
	}
	return ""
}

// fbProfileVideoListing builds the URL candidates for a profile link.
func fbProfileVideoListing(profileURL string) []string {
	raw := strings.TrimSpace(profileURL)
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	raw = strings.TrimPrefix(raw, "www.")
	raw = strings.TrimPrefix(raw, "m.")
	raw = strings.TrimPrefix(raw, "mbasic.")
	raw = strings.TrimSuffix(raw, "/")
	// host part case-insensitive, path part CASE-SENSITIVE (pfbid tokens)
	lowHost := strings.ToLower(raw)
	if i := strings.Index(lowHost, "/"); i >= 0 {
		raw = lowHost[:i+1] + raw[i+1:]
	} else {
		raw = lowHost
	}

	slash := strings.Index(raw, "/")
	path := ""
	if slash >= 0 {
		path = raw[slash:]
	}

	var candidates []string
	if path == "" {
		return candidates
	}

	numID := fbNumericIDFromLink(profileURL)

	// People-style: /people/NAME/pfbidXXX ya /people/NAME/NNNN
	if strings.HasPrefix(path, "/people/") {
		segs := strings.Split(strings.Trim(path, "/"), "/")
		if len(segs) >= 2 && segs[1] != "" {
			peoplePath := "/people/" + segs[1]
			isPfbid := false
			if len(segs) >= 3 && strings.HasPrefix(segs[2], "pfbid") {
				peoplePath += "/" + segs[2]
				isPfbid = true
				candidates = append(candidates,
					"https://www.facebook.com"+peoplePath+"/videos",
					"https://m.facebook.com"+peoplePath+"/videos",
				)
			}
			// v3.1: numeric teesra segment bhi profile ID hai
			// (facebook.com/people/New-Videos/61593685756994)
			if len(segs) >= 3 && !isPfbid && allDigits(segs[2]) && len(segs[2]) >= 6 {
				if numID == "" {
					numID = segs[2]
				}
			}
		}
		// v3: numeric-ID TIMELINE route FIRST - reels timeline pe hoti hain
		// aur /videos tab in naye profiles ke liye 404 deta hai!
		if numID != "" {
			candidates = append(candidates,
				"https://www.facebook.com/profile.php?id="+numID,
				"https://m.facebook.com/profile.php?id="+numID,
			)
		}
		return candidates
	}

	// /profile.php?id=NNN -> numeric timeline + /videos
	if strings.HasPrefix(path, "/profile.php") {
		if numID != "" {
			candidates = append(candidates,
				"https://www.facebook.com/profile.php?id="+numID,
				"https://www.facebook.com/"+numID+"/videos",
			)
		}
		return candidates
	}

	// Vanity style: /username - /videos tab + timeline (v3)
	segs := strings.Split(strings.Trim(path, "/"), "/")
	vanity := segs[0]
	if vanity != "" {
		candidates = append(candidates,
			"https://www.facebook.com/"+vanity+"/videos",
			"https://m.facebook.com/"+vanity+"/videos",
			"https://www.facebook.com/"+vanity,
		)
	}
	return candidates
}

// allDigits reports whether s is a non-empty all-digit string.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// fbNumericIDFromLink extracts a numeric profile id (?id=NNNN).
func fbNumericIDFromLink(link string) string {
	idx := strings.Index(link, "id=")
	if idx < 0 {
		return ""
	}
	rest := link[idx+3:]
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			end = i
			break
		}
	}
	if end >= 6 {
		return rest[:end]
	}
	return ""
}

// fbLatestVideoLinkFixed - multi-route resolver (v3).
// /videos tab -> profile.php timeline -> vanity timeline - sab try.
// REEL (/reel/ID) and WATCH (/watch/?v=ID) permalinks bhi match.
// Returns video/reel permalink (empty on total failure).
func fbLatestVideoLinkFixed(ctx context.Context, profileURL string) string {
	fbDebug("resolve_start", map[string]any{"profile": profileURL})
	for _, listing := range fbProfileVideoListing(profileURL) {
		md, err := jinaFetch(ctx, listing)
		if err != nil || md == "" {
			fbDebug("route_fetch_empty", map[string]any{"listing": listing, "err": fmtErr(err), "size": len(md)})
			continue
		}
		if link := fbExtractPermalink(md); link != "" {
			fbDebug("resolve_found", map[string]any{"listing": listing, "video_link": link})
			return link
		}
		fbDebug("route_no_permalink", map[string]any{"listing": listing, "size": len(md)})
	}
	fbDebug("resolve_end_empty", map[string]any{"profile": profileURL})
	return ""
}

// FBLatestVideoLinkLive - exported wrapper for the live sandbox test binary.
func FBLatestVideoLinkLive(ctx context.Context, profileURL string) string {
	return fbLatestVideoLinkFixed(ctx, profileURL)
}
