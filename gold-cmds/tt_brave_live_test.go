package goldcmds

// LIVE probe: Brave HTML TikTok video search (no API, no tikwm).
// Gate: TT_LIVE=1 (never runs in CI).
//
// Verifies the full keyless chain end-to-end:
//  1. ttVideoSearch  — Brave HTML scrape + oembed enrichment
//  2. ttSelfFetch    — TikTok video page self-scrape (playAddr)
//
// TikTok throttles globally-over-visited videos (statusCode 10204,
// web_visit_cnt_more_than_3). That is a TikTok-side popularity limit,
// NOT an engine failure, so throttle errors are logged as skips here.
import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTTBraveLive(t *testing.T) {
	if os.Getenv("TT_LIVE") != "1" {
		t.Skip("TT_LIVE not set — skipping live probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	query := "funny cats"
	results, err := ttVideoSearch(ctx, query)
	if err != nil {
		t.Fatalf("ttVideoSearch FAILED: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("ttVideoSearch returned 0 results")
	}
	t.Logf("RESULTS: %d", len(results))
	for i, r := range results {
		if i >= 8 {
			break
		}
		t.Logf("  [%d] %s | %s\n      %s", i+1, r.Title, r.Handle, r.Link)
	}

	// Download engine on the first 3 picks. Throttle (10204) = TikTok-side
	// popularity limit on that specific video — acceptable, logged as skip.
	ok, throttled := 0, 0
	for i, r := range results {
		if i >= 3 {
			break
		}
		res, err := ttSelfFetch(ctx, r.Link)
		if err != nil {
			if strings.Contains(err.Error(), "throttling") {
				throttled++
				t.Logf("  [%d] SKIP (TikTok throttle 10204, popular video): %v", i+1, err)
			} else {
				t.Errorf("  [%d] ttSelfFetch FAILED (engine error): %v", i+1, err)
			}
			continue
		}
		ok++
		t.Logf("  [%d] DOWNLOAD OK: id=%s dur=%ds play=%s...", i+1, res.ID, res.Duration, res.Play[:60])
	}
	if ok == 0 && throttled == 0 {
		t.Fatal("no result could be fetched and none was throttled — engine problem")
	}
	t.Logf("engine verified: %d ok, %d throttled (TikTok-side)", ok, throttled)

	// Fresh shortlink video — must always give full data (statusCode 0).
	res, err := ttSelfFetch(ctx, "https://vm.tiktok.com/ZSCE88Sv4/")
	if err != nil {
		t.Fatalf("ttSelfFetch on fresh vm link FAILED: %v", err)
	}
	t.Logf("FRESH VIDEO OK: id=%s dur=%ds author=@%s", res.ID, res.Duration, res.AuthorUnique)
}
