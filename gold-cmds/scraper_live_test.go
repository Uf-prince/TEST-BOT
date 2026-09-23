package goldcmds

import (
	"context"
	"testing"
	"time"
)

// TestScraperLive validates the free, no-key Facebook + Instagram scrapers.
// Run with: go test -run TestScraperLive -v ./gold-cmds/
func TestScraperLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cases := []struct {
		name string
		url  string
	}{
		{"facebook_watch", "https://www.facebook.com/watch/?v=10153231379946729"},
		{"instagram_reel", "https://www.instagram.com/reel/DN035iN2Dum/"},
	}

	for _, c := range cases {
		resp, err := fbCobaltFetch(ctx, c.url)
		if err != nil {
			t.Errorf("[%s] fetch error: %v", c.name, err)
			continue
		}
		vurl, quality := fbResolveVideoURL(resp)
		if vurl == "" {
			t.Errorf("[%s] no video url (status=%s)", c.name, resp.Status)
			continue
		}
		t.Logf("[%s] OK status=%s quality=%s file=%s url=%.90s", c.name, resp.Status, quality, resp.Filename, vurl)
	}
}
