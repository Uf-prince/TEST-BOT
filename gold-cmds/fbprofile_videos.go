package goldcmds

// ============================================================================
// GOLD-MD — FB Profile → Latest Video Resolver (FIXED)
// File: fbprofile_videos.go
// ============================================================================
// BUG (OLD): fbLatestVideoLink people-style URLs ko galat parse karta tha:
//   facebook.com/people/NAME/pfbidXXX/  → profile = "people"
//   → listing = facebook.com/people/videos  (GARBAGE URL)
//   → jina se koi video permalink nahi → empty → guidance card fallback.
//
// FIX (NEW): Multi-route resolver with proper URL parsing:
//   Route 1: vanity URL (facebook.com/zuck) — www + m.facebook dono try
//   Route 2: people-style URL — poora path use hota hai (NAME + pfbid),
//            www + m.facebook dono try
//   Route 3: numeric id URL — direct /videos tab
// Har route jina reader se fetch hota hai (direct hits login-walled hain).
// Pehla successful video permalink jeet-ta hai.
// ============================================================================

import (
	"context"
	"regexp"
	"strings"
)

// fbVideoLinkRe (searchpick.go se shared) — video permalink matcher:
//   facebook.com/<user>/videos/<slug>/<id>
//   facebook.com/<user>/videos/<id>
var fbVideoLinkReFixed = regexp.MustCompile(
	`facebook\.com/([^\s)\"\']+)/videos/(?:([^\s)\"\']+)/)?([0-9]{6,})/?`)

// fbProfileVideoListing builds the /videos listing URL candidates for a
// profile link (people-style, vanity, numeric — sab handled).
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

	// keep only the profile path (drop query strings / sub-paths)
	// e.g. people/Zuckerberg-Ind/pfbidXXX → path segments
	slash := strings.Index(raw, "/")
	path := ""
	if slash >= 0 {
		path = raw[slash:]
	}

	var candidates []string
	if path == "" {
		return candidates
	}

	// People-style: /people/NAME/pfbidXXX — poora path zinda rakho (BUG FIX)
	if strings.HasPrefix(path, "/people/") {
		segs := strings.Split(strings.Trim(path, "/"), "/")
		// ["people", "NAME", "pfbidXXX"] — ya ["people", "NAME"]
		if len(segs) >= 2 && segs[1] != "" {
			peoplePath := "/people/" + segs[1]
			if len(segs) >= 3 && strings.HasPrefix(segs[2], "pfbid") {
				peoplePath += "/" + segs[2]
			}
			candidates = append(candidates,
				"https://www.facebook.com"+peoplePath+"/videos",
				"https://m.facebook.com"+peoplePath+"/videos",
			)
		}
		// numeric id fallback agar pfbid nahi mila
		if id := fbNumericIDFromLink(profileURL); id != "" {
			candidates = append(candidates,
				"https://www.facebook.com/"+id+"/videos",
				"https://m.facebook.com/"+id+"/videos",
			)
		}
		return candidates
	}

	// /profile.php?id=NNN style → numeric id route
	if strings.HasPrefix(path, "/profile.php") {
		if id := fbNumericIDFromLink(profileURL); id != "" {
			candidates = append(candidates,
				"https://www.facebook.com/"+id+"/videos",
				"https://m.facebook.com/"+id+"/videos",
			)
		}
		return candidates
	}

	// Vanity style: /username (ya /username/photos etc.) — sirf pehla segment
	segs := strings.Split(strings.Trim(path, "/"), "/")
	vanity := segs[0]
	if vanity != "" {
		candidates = append(candidates,
			"https://www.facebook.com/"+vanity+"/videos",
			"https://m.facebook.com/"+vanity+"/videos",
		)
	}
	return candidates
}

// fbNumericIDFromLink extracts a numeric profile id from any FB link
// (?id=NNNN query param).
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
	if end >= 6 { // numeric ids are long
		return rest[:end]
	}
	return ""
}

// fbLatestVideoLinkFixed — multi-route resolver (replaces the broken one).
// Returns the latest video permalink + title slug (empty on total failure).
func fbLatestVideoLinkFixed(ctx context.Context, profileURL string) string {
	for _, listing := range fbProfileVideoListing(profileURL) {
		md, err := jinaFetch(ctx, listing)
		if err != nil || md == "" {
			continue
		}
		if m := fbVideoLinkReFixed.FindStringSubmatch(md); m != nil {
			return "https://www.facebook.com/" + m[1] + "/videos/" + m[3]
		}
	}
	return ""
}
// FBLatestVideoLinkLive — exported wrapper for the live sandbox test binary.
func FBLatestVideoLinkLive(ctx context.Context, profileURL string) string {
	return fbLatestVideoLinkFixed(ctx, profileURL)
}
