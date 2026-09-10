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
