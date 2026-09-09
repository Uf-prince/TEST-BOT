package goldcmds

import (
	"context"
	"strings"
	"testing"
)

// TestFBProfileVideoListing — URL candidate builder (pure logic, no network).
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
			if !strings.Contains(c, "/videos") {
				t.Errorf("%s: candidate missing /videos: %s", tc.name, c)
			}
			// CRITICAL: koi bhi candidate "people/videos" (broken) NAHI hona chahiye
			if strings.Contains(c, "facebook.com/people/videos") {
				t.Errorf("%s: BROKEN candidate facebook.com/people/videos generated from %s", tc.name, tc.input)
			}
		}
		t.Logf("%s → %v", tc.name, cands)
	}
}

// TestFBNumericIDFromLink — ?id= extraction.
func TestFBNumericIDFromLink(t *testing.T) {
	if got := fbNumericIDFromLink("https://www.facebook.com/people/X/pfbid1/?id=61570780400568&sk=photos"); got != "61570780400568" {
		t.Errorf("numeric id extract failed: got %q", got)
	}
	if got := fbNumericIDFromLink("https://www.facebook.com/zuck"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestFBLatestVideoLinkFixedOffline — offline safety: bogus host pe empty.
func TestFBLatestVideoLinkFixedOffline(t *testing.T) {
	// jinaFetch real network hit karta hai; is test mein sirf empty-input safety
	if fbLatestVideoLinkFixed(context.Background(), "not a url") != "" {
		t.Errorf("expected empty for non-url input")
	}
}
