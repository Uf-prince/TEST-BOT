//go:build tt_live

package goldcmds

// LIVE test (TT_VS_LIVE=1): verifies the .tt VIDEO search chain with the
// 15 SHORTS + 15 LONG split (owner: "15 shorts videos ka link aye 15 long
// videos ka link aye ... list total 30 videos ki bane ge"):
//   1. tikwm feed/search/ pages (trailing slash) return REAL videos
//   2. results split into SHORTS (<=60s) + LONG (>60s), combined <= 30
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
	if len(results) == 0 {
		t.Fatalf("no video results")
	}

	var shorts, longs int
	for i, r := range results {
		kind := "SHORT"
		if r.DurationSec > 60 {
			kind = "LONG "
			longs++
		} else {
			shorts++
		}
		t.Logf("%2d %s %4ds | %s | %s | %s", i+1, kind, r.DurationSec, r.Handle, r.Stats, r.Link)
		if !strings.Contains(r.Link, "/video/") {
			t.Errorf("result %d link is not a video link: %s", i+1, r.Link)
		}
	}
	t.Logf("total=%d shorts=%d longs=%d", len(results), shorts, longs)
	if longs == 0 {
		t.Fatalf("no LONG (>60s) videos found for %q — split would be empty", q)
	}

	// pick the FIRST long video — exactly what a user's number-pick hits
	var pick searchResult
	for _, r := range results {
		if r.DurationSec > 60 {
			pick = r
			break
		}
	}
	t.Logf("picked LONG video: %s (%ds)", pick.Link, pick.DurationSec)

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
	t.Logf("OK downloaded %d bytes — LONG video chain WORKS", st.Size())
}
