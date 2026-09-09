//go:build tt_live

package goldcmds

// LIVE test (TT_VS_LIVE=1): verifies the fixed .tt VIDEO search chain:
//   1. tikwm feed/search/ (trailing slash — bina slash ke CF challenge)
//      returns REAL videos (tiktok.com/@user/video/ID links)
//   2. video link -> ttSelfFetch (proven self-scrape) resolves playable URL
//   3. tikwm download fallback also alive
//   4. media URL actually downloads (first 256KB)
//
// Usage:
//   export PATH=$PATH:/usr/local/go/bin
//   TT_VS_LIVE=1 go test -tags tt_live -run TestTTVideoSearchLive -v -vet=off ./gold-cmds/

import (
	"context"
	"fmt"
	"io"
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	q := "aja ve mahiya"
	t.Logf("searching tikwm feed/search/ for %q ...", q)
	results, err := ttVideoSearch(ctx, q)
	if err != nil {
		t.Fatalf("ttVideoSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("no video results")
	}
	for i, r := range results {
		t.Logf("%d. %s | %s | %s | %s", i+1, r.Title, r.Handle, r.Stats, r.Link)
		if !strings.Contains(r.Link, "/video/") {
			t.Errorf("result %d link is not a video link: %s", i+1, r.Link)
		}
	}
	first := results[0]
	t.Logf("first result video link: %s", first.Link)

	t.Log("resolving via ttSelfFetch (self-scrape) ...")
	res, err := ttSelfFetch(ctx, first.Link)
	if err != nil {
		t.Logf("self-scrape failed (%v) — trying tikwm fallback", err)
		res2, err2 := tikwmFetchResult(ctx, first.Link)
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
		client = &http.Client{Timeout: 60 * time.Second}
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
	t.Logf("OK downloaded %d bytes — full chain WORKS", st.Size())
}

// ttLiveChunk downloads up to 256KB of a media URL with a browser UA.
func ttLiveChunk(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 256*1024)
	n, _ := io.ReadFull(resp.Body, buf)
	return buf[:n], nil
}
