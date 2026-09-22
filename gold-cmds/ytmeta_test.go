package goldcmds

import (
	"context"
	"testing"
	"time"
)

// TestYt2FetchMetaLive hits the real Innertube /next endpoint and verifies the
// author / views / comments parsing used by the thumbnail caption.
func TestYt2FetchMetaLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	m := yt2FetchMeta(ctx, "dQw4w9WgXcQ")
	if m == nil {
		t.Fatalf("yt2FetchMeta returned nil")
	}
	t.Logf("title=%q author=%q views=%q comments=%q", m.title, m.author, m.views, m.comments)
	if m.author == "" {
		t.Errorf("author empty")
	}
	if m.views == "" {
		t.Errorf("views empty")
	}
	if m.comments == "" {
		t.Errorf("comments empty")
	}
}
