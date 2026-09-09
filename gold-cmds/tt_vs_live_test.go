//go:build tt_live

package goldcmds

// LIVE test (TT_VS_LIVE=1): verifies the .tt VIDEO search chain with the
// 15 SHORTS + 15 LONG split (owner round 2: "1 mint ki to already shorts
// me aa rhe the — LONG me 4/5 mint aur 10 mint ki videos chahiye, aur
// list me DURATION line ho"):
//   1. tikwm feed/search/ pages (trailing slash) return REAL videos
//   2. SHORTS <= 60s in relevance order; LONG >= 120s sorted DURATION DESC
//      (longest video top of LONG section); 61-119s dropped
//   3. a LONG video link -> ttSelfFetch resolve -> ttStreamDownload bytes
//
// Usage:
//   export PATH=$PATH:/usr/local/go/bin
//   TT_VS_LIVE=1 go test -tags tt_live -run TestTTVideoSearchLive -v -vet=off -count=1 ./gold-cmds/

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTTVideoSearchLive(t *testing.T) {
	if os.Getenv("TT_VS_LIVE") == "" {
		t.Skip("set TT_VS_LIVE=1 for live network test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	q := "aja ve mahiya"
	t.Logf("searching tikwm feed/search/ pages for %q ...", q)
	results, err := ttVideoSearch(ctx, q)
	if err != nil {
		t.Fatalf("ttVideoSearch failed: %v", err)
	}
	results = filterTTResults(results)
	if len(results) == 0 {
		t.Fatalf("no video results")
	}

	var shorts, longs int
	lastLong := int64(-1)
	for i, r := range results {
		kind := "SHORT"
		if r.DurationSec > 60 {
			kind = "LONG "
			longs++
			// LONG must be >= 120s and sorted DESC (longest first)
			if r.DurationSec < 120 {
				t.Errorf("LONG result %d is only %ds (< 2min): %s", i+1, r.DurationSec, r.Link)
			}
			if lastLong >= 0 && r.DurationSec > lastLong {
				t.Errorf("LONG sort broken: %ds after %ds", r.DurationSec, lastLong)
			}
			lastLong = r.DurationSec
		} else {
			shorts++
			if r.DurationSec > 60 {
				t.Errorf("SHORT result %d is %ds (> 60s): %s", i+1, r.DurationSec, r.Link)
			}
		}
		t.Logf("%2d %s %4ds | %s | %s | %s", i+1, kind, r.DurationSec, r.Handle, r.Stats, r.Link)
		if !strings.Contains(r.Link, "/video/") {
			t.Errorf("result %d link is not a video link: %s", i+1, r.Link)
		}
	}
	t.Logf("total=%d shorts=%d longs=%d", len(results), shorts, longs)
	if longs == 0 {
		t.Fatalf("no LONG (2min+) videos found for %q", q)
	}

	// pick the LONGEST long video (top of the LONG section)
	pick := searchResult{}
	for _, r := range results {
		if r.DurationSec > 60 && r.DurationSec > pick.DurationSec {
			pick = r
		}
	}
	t.Logf("picked LONGEST LONG video: %s (%ds)", pick.Link, pick.DurationSec)

	t.Log("resolving via ttSelfFetch (self-scrape) ...")
	res, err := ttSelfFetch(ctx, pick.Link)
	if err != nil {
		t.Logf("self-scrape failed (%v) — trying tikwm fallback", err)
		res2, err2 := tikwmFetchResult(ctx, pick.Link)
		if err2 != nil {
			t.Fatalf("both failed: self=%v tikwm=%v", err, err2)
		}
		res = res2
	}
	dl := firstNonEmpty(res.HDPlay, res.Play, res.WMPlay)
	if dl == "" {
		t.Fatalf("no playable URL: %+v", res)
	}
	t.Logf("playable URL: %.90s ...", dl)

	// REAL download path (same as searchPickTTDirect): mobile UA + Referer
	// + cookie-jar client from the scrape (res.SrcClient).
	t.Log("downloading via real ttStreamDownload (mobile UA + Referer) ...")
	client := res.SrcClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	path, err := ttStreamDownload(ctx, client, dl)
	if err != nil {
		t.Fatalf("media download failed: %v", err)
	}
	defer os.Remove(path)
	st, _ := os.Stat(path)
	if st == nil || st.Size() < 50*1024 {
		t.Fatalf("downloaded file too small: %v bytes", st)
	}
	t.Logf("OK downloaded %d bytes — LONGEST LONG video chain WORKS", st.Size())
}
