package goldcmds

// LIVE probe: TikTok self-scrape engine (download path) real video pe.
// Run: TT_LIVE=1 go test -mod=vendor -v -run TestTTSelfLive ./gold-cmds/ -count=1

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestTTSelfLive(t *testing.T) {
	if os.Getenv("TT_LIVE") != "1" {
		t.Skip("set TT_LIVE=1")
	}
	// 1) feed search se ek REAL fresh video id le lo (sirf test ke liye)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Fresh video: TikTok trending page scrape
	ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/E15AC Safari/604.1"
	testURL := "https://www.tiktok.com/@tiktok/video/7238328201429272540"

	res, err := ttSelfFetch(ctx, testURL)
	if err != nil {
		t.Fatalf("ttSelfFetch FAILED: %v", err)
	}
	playPrev := res.Play
	if len(playPrev) > 60 {
		playPrev = playPrev[:60]
	}
	t.Logf("OK: id=%s title=%.50q play=%s author=%s dur=%d", res.ID, res.Title, playPrev, res.AuthorUnique, res.Duration)
	if res.Play == "" {
		t.Fatal("EMPTY playAddr")
	}

	// 2) playAddr download test (first 2MB)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, res.Play, nil)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", ttReferer)
	c := res.SrcClient
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	r2, err := c.Do(req)
	if err != nil {
		t.Fatalf("playAddr download FAILED: %v", err)
	}
	defer r2.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(r2.Body, 2<<20))
	t.Logf("playAddr status=%d bytes=%d ctype=%s", r2.StatusCode, len(head), r2.Header.Get("Content-Type"))
	if r2.StatusCode != 200 || len(head) < 1000 {
		t.Fatalf("DOWNLOAD BAD: status=%d len=%d", r2.StatusCode, len(head))
	}
	fmt.Println("VIDEO_BYTES_OK")
}
