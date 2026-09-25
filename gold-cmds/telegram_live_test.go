package goldcmds

// TG V2 live tests — run with:
//   TG_LIVE=1 go test -mod=vendor -vet=off -v -count=1 -run 'TestTGV2Live' ./gold-cmds/

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTGV2Live(t *testing.T) {
	if os.Getenv("TG_LIVE") != "1" {
		t.Skip("set TG_LIVE=1 to run live TG tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 1) specific post link → media (V2 ?before targeting)
	cases := []struct {
		link string
		kind string // "" → text-only allowed
	}{
		{"https://t.me/telegram/459", "video"},
		{"https://t.me/telegram/452", "photo"},
		{"https://t.me/telegram/445", ""}, // text-only
		{"https://t.me/durov/531", "video"},
	}
	for _, c := range cases {
		media, err := tgFetchPost(ctx, c.link)
		if err != nil {
			t.Errorf("tgFetchPost(%s): %v", c.link, err)
			continue
		}
		if c.kind != "" && media.kind != c.kind {
			t.Errorf("tgFetchPost(%s): kind=%s want=%s", c.link, media.kind, c.kind)
			continue
		}
		if c.kind != "" && media.url == "" {
			t.Errorf("tgFetchPost(%s): empty url", c.link)
			continue
		}
		t.Logf("%s -> kind=%s chan=@%s name=%q views=%s url=%s",
			c.link, media.kind, media.channel, media.name, media.views, minStr(media.url, 80))
	}

	// 2) deleted/nonexistent post → clean error
	if _, err := tgFetchPost(ctx, "https://t.me/durov/999999"); err == nil {
		t.Logf("durov/999999: no error (post may exist?)")
	} else {
		t.Logf("durov/999999 correctly rejected: %v", err)
	}

	// 3) channel link → newest media post
	media, err := tgLatestMedia(ctx, "https://t.me/telegram")
	if err != nil || media == nil || media.url == "" {
		t.Errorf("tgLatestMedia(telegram): %v / %v", err, media)
	} else {
		t.Logf("latest media on @telegram: kind=%s views=%s", media.kind, media.views)
	}

	// 4) media posts channel
	media2, err := tgLatestMedia(ctx, "https://t.me/durov")
	if err != nil || media2 == nil || media2.url == "" {
		t.Errorf("tgLatestMedia(durov): %v / %v", err, media2)
	} else {
		t.Logf("latest media on @durov: kind=%s views=%s", media2.kind, media2.views)
	}

	// 5) nonexistent channel → clean error
	if _, err := tgLatestMedia(ctx, "https://t.me/thischanneldoesnotexist9999xyz"); err == nil {
		t.Errorf("nonexistent channel: expected error")
	} else {
		t.Logf("nonexistent channel correctly rejected: %v", err)
	}

	_ = strings.TrimSpace
}

func minStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
