package goldcmds

// Live test for the v2 X/Twitter engine (DDG-jina search + profile
// resolver). Run with:
//   go test -mod=vendor -vet=off -v -count=1 -run 'TestTWTEngine' ./gold-cmds/

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTWTEngine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	// 1) account search — DDG-jina primary, Bing fallback
	res, err := twtAccountSearch(ctx, "ronaldo")
	if err != nil || len(res) == 0 {
		t.Fatalf("twtAccountSearch: %v / %d", err, len(res))
	}
	t.Logf("SEARCH OK - %d results, first: %s %s", len(res), res[0].Handle, res[0].Link)
	if len(res) < 10 {
		t.Logf("WARN: only %d results — 15-target engine merge thin gaya", len(res))
	}

	// 2) profile resolver — pehla profile result -> latest tweet ID
	for _, r := range res {
		handle := strings.TrimPrefix(r.Handle, "@")
		if handle == "" || twExtractTweetID(r.Link) != "" {
			continue
		}
		id, iderr := twProfileLatestStatusID(ctx, handle)
		if iderr != nil || id == "" {
			t.Errorf("twProfileLatestStatusID(%s): %v", handle, iderr)
			return
		}
		t.Logf("PROFILE RESOLVE OK - @%s -> status %s", handle, id)

		// 3) resolved tweet fxtwitter se fetch ho
		tweet, ferr := twFetchTweet(ctx, id)
		if ferr != nil || tweet == nil {
			t.Errorf("twFetchTweet(%s): %v", id, ferr)
			return
		}
		t.Logf("FETCH OK - @%s media videos=%d photos=%d", tweet.Author.ScreenName, len(tweet.Media.Videos), len(tweet.Media.Photos))
		return
	}
	t.Errorf("no profile result to test resolver with")
}

// TestTWTVideoFirst — OWNER RULE (video-first): profile resolver ko pehla
// VIDEO tweet dena chahiye (photos nahi).
func TestTWTVideoFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	// MrBeast — recent timeline me video tweets hote hain
	vid, err := twProfileLatestVideoStatusID(ctx, "MrBeast")
	if err != nil || vid == "" {
		t.Fatalf("twProfileLatestVideoStatusID(MrBeast): %v", err)
	}
	tw, ferr := twFetchTweet(ctx, vid)
	if ferr != nil || tw == nil {
		t.Fatalf("twFetchTweet(%s): %v", vid, ferr)
	}
	if len(tw.Media.Videos) == 0 {
		t.Errorf("video-first resolver returned a NON-VIDEO tweet (photos=%d) - id %s", len(tw.Media.Photos), vid)
		return
	}
	t.Logf("VIDEO-FIRST OK - MrBeast -> status %s (videos=%d photos=%d)", vid, len(tw.Media.Videos), len(tw.Media.Photos))
}
