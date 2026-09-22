package goldcmds

// Live verification for the IG search engine V2 (run manually, not in CI):
//   go test -mod=vendor -vet=off -v -count=1 -run 'TestIGEngineLive' ./gold-cmds/ -timeout 200s
//   IG_LIVE=1 go test -mod=vendor -vet=off -v -count=1 -run 'TestIGEngineLive' ./gold-cmds/ -timeout 200s
// 1) igEngineSearch multi-source merge (hashtag reels + bing profiles)
// 2) permalink → cobalt fetch (direct download path)
// 3) igClassifyLink normalization checks

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestIGEngineLive(t *testing.T) {
	if os.Getenv("IG_LIVE") == "" {
		t.Skip("set IG_LIVE=1 to run the live Instagram test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	// ── classify tests (offline sanity) ──
	cases := []struct {
		link string
		ok   bool
		out  string
	}{
		{"https://www.instagram.com/reel/DGK5rrAoXnt/", true, "https://www.instagram.com/reel/DGK5rrAoXnt/"},
		{"https://www.instagram.com/cristiano/", true, "https://www.instagram.com/cristiano/"},
		{"https://www.instagram.com/explore/tags/cats/", false, ""},
		{"https://www.youtube.com/watch?v=x", false, ""},
	}
	for _, c := range cases {
		r, ok := igClassifyLink("t", c.link)
		if ok != c.ok || (ok && r.Link != c.out) {
			t.Errorf("classify %s → ok=%v link=%q (want %v %q)", c.link, ok, r.Link, c.ok, c.out)
		}
	}

	// ── hashtag reels search ──
	for _, q := range []string{"funny cats", "cristiano ronaldo"} {
		tags, err := igHashtagSearch(ctx, q)
		if err != nil {
			t.Logf("hashtag %q err: %v", q, err)
		} else {
			t.Logf("hashtag %q → %d results", q, len(tags))
			for i, r := range tags {
				if i >= 5 {
					break
				}
				t.Logf("  %d. %s | %s | %s", i+1, r.Title, r.Stats, r.Link)
			}
		}
	}

	// ── merged engine ──
	res, err := igEngineSearch(ctx, "funny cats")
	if err != nil {
		t.Fatalf("igEngineSearch: %v", err)
	}
	t.Logf("igEngineSearch 'funny cats' → %d results", len(res))
	if len(res) == 0 {
		t.Fatal("no results from merged IG engine")
	}
	perma := 0
	for i, r := range res {
		if i >= 10 {
			break
		}
		t.Logf("  %d. %s | %s | %s", i+1, r.Title, r.Stats, r.Link)
		if igIsPermalink(r.Link) {
			perma++
		}
	}
	if perma == 0 {
		t.Fatal("no permalink results — hashtag engine broken")
	}

	// ── permalink → cobalt end-to-end ──
	var link string
	for _, r := range res {
		if igIsPermalink(r.Link) {
			link = r.Link
			break
		}
	}
	if link != "" {
		resp, err := fbCobaltFetch(ctx, link)
		if err != nil {
			t.Fatalf("cobalt %s: %v", link, err)
		}
		vurl, q := fbResolveVideoURL(resp)
		t.Logf("cobalt %s → status=%s quality=%s url=%.80s", link, resp.Status, q, vurl)
		if vurl == "" {
			t.Fatal("cobalt returned no video URL for hashtag permalink")
		}
	}

	// ── profile search merge (bing) ──
	res2, _ := igEngineSearch(ctx, "cristiano ronaldo")
	t.Logf("igEngineSearch 'cristiano ronaldo' → %d results", len(res2))
	for i, r := range res2 {
		if i >= 6 {
			break
		}
		t.Logf("  %d. %s | %s | %s", i+1, r.Title, r.Handle, r.Link)
	}
	_ = fmt.Sprint()
}
