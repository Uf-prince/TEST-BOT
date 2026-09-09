package goldcmds

// Live verification for ttSelfFetch (run manually, not in CI):
//   go test -mod=vendor -run TestTTSelfFetchLive ./gold-cmds/ -v -timeout 120s
// Hits a real TikTok page — validates page fetch, JSON parse and playAddr.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestTTSelfFetchLive(t *testing.T) {
	if os.Getenv("TT_LIVE") == "" {
		t.Skip("set TT_LIVE=1 to run the live TikTok test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := ttSelfFetch(ctx, "https://vm.tiktok.com/ZSCE88Sv4")
	if err != nil {
		t.Fatalf("ttSelfFetch failed: %v", err)
	}
	t.Logf("ID=%s title=%.40s dur=%d likes=%d views=%d author=%s",
		res.ID, res.Title, res.Duration, res.DiggCount, res.PlayCount, res.AuthorName)
	if res.Play == "" {
		t.Fatal("empty play URL")
	}

	// partial download check (first 300 KB must be a valid MP4)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, res.Play, nil)
	req.Header.Set("User-Agent", ttMobileUA)
	req.Header.Set("Referer", ttReferer)
	req.Header.Set("Range", "bytes=0-300000")
	c := res.SrcClient
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer resp.Body.Close()
	head := make([]byte, 12)
	if _, err := io.ReadFull(resp.Body, head); err != nil {
		t.Fatalf("read head failed: %v", err)
	}
	t.Logf("status=%d magic=%q", resp.StatusCode, string(head[4:12]))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("bad status %d", resp.StatusCode)
	}
	if string(head[4:8]) != "ftyp" {
		t.Fatalf("not an MP4 (magic %q)", string(head[4:12]))
	}
	fmt.Println("LIVE TEST PASSED")
}
