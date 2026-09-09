package goldcmds

import (
	"strings"
	"testing"
)

// TestFBProfileVideoListing - URL candidate builder (pure logic, no network).
func TestFBProfileVideoListing(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"people-style", "https://www.facebook.com/people/Zuckerberg-Ind/pfbid04uTmCTa67DtgTBbZ5yAmz8aPE3jjsK99Ntoui6Mom2M1MPJQmPg7FXWbpuU8WKR5l/"},
		{"people-style-query", "https://www.facebook.com/people/Zuckerberg-Ind/pfbid04uTmCTa67DtgTBbZ5yAmz8aPE3jjsK99Ntoui6Mom2M1MPJQmPg7FXWbpuU8WKR5l/?id=61570780400568&sk=photos"},
		{"vanity", "https://www.facebook.com/zuck"},
		{"vanity-m", "https://m.facebook.com/zuck/videos"},
		{"vanity-slash", "https://www.facebook.com/zuckerberg.2025/"},
		{"numeric", "https://www.facebook.com/61570780400568"},
		{"profile-php", "https://www.facebook.com/profile.php?id=61570780400568"},
	}

	for _, tc := range cases {
		cands := fbProfileVideoListing(tc.input)
		if len(cands) == 0 {
			t.Errorf("%s: got 0 candidates for %s", tc.name, tc.input)
			continue
		}
		for _, c := range cands {
			if strings.Contains(c, "facebook.com/people/videos") {
				t.Errorf("%s: BROKEN candidate facebook.com/people/videos from %s", tc.name, tc.input)
			}
		}
		// v3: people-style + ?id= must include profile.php timeline route
		if tc.name == "people-style-query" {
			found := false
			for _, c := range cands {
				if strings.HasPrefix(c, "https://www.facebook.com/profile.php?id=") {
					found = true
				}
			}
			if !found {
				t.Errorf("people-style-query: profile.php timeline route missing: %v", cands)
			}
		}
		t.Logf("%s -> %v", tc.name, cands)
	}
}

// TestFBExtractPermalink - reel + video + watch matcher.
func TestFBExtractPermalink(t *testing.T) {
	if got := fbExtractPermalink("see [reel](https://www.facebook.com/reel/1608803157499500/) now"); got != "https://www.facebook.com/reel/1608803157499500" {
		t.Errorf("reel: got %q", got)
	}
	if got := fbExtractPermalink("[v](https://www.facebook.com/zuck/videos/2281597032594351/)"); got != "https://www.facebook.com/zuck/videos/2281597032594351" {
		t.Errorf("video: got %q", got)
	}
	if got := fbExtractPermalink("[w](https://www.facebook.com/watch/?v=2281597032594351)"); got != "https://www.facebook.com/watch/?v=2281597032594351" {
		t.Errorf("watch: got %q", got)
	}
	if got := fbExtractPermalink("no links here"); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

// TestFBNumericIDFromLink - ?id= extraction.
func TestFBNumericIDFromLink(t *testing.T) {
	if got := fbNumericIDFromLink("https://www.facebook.com/people/X/pfbid1/?id=61570780400568&sk=photos"); got != "61570780400568" {
		t.Errorf("numeric id extract failed: got %q", got)
	}
	if got := fbNumericIDFromLink("https://www.facebook.com/zuck"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
