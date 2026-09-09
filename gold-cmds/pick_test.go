package goldcmds

// Live tests for the search pick pipeline — run with:
//   go test -mod=vendor -vet=off -v -count=1 -run 'TestSearchPick' ./gold-cmds/

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSearchPick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	// 1) APK end-to-end: search → resolve → signed R2 URL (no download here)
	res, err := apkAppSearch(ctx, "whatsapp")
	if err != nil || len(res) == 0 {
		t.Fatalf("apkAppSearch: %v / %d", err, len(res))
	}
	app, err := apkSearchResolve(ctx, res[0])
	if err != nil {
		t.Fatalf("apkSearchResolve: %v", err)
	}
	if app == nil || app.FileURL == "" {
		t.Fatal("apkSearchResolve: no file url")
	}
	if !strings.Contains(app.FileURL, "r2.cloudflarestorage.com") {
		t.Errorf("file url not R2: %s", app.FileURL)
	}
	t.Logf("APK E2E OK — %s %s (%s) -> %s...", app.Title, app.Version, app.Size, app.FileURL[:80])

	// 2) TG latest media post: t.me/telegram → newest media post (V2 /s/ route)
	if media, merr := tgLatestMedia(ctx, "https://t.me/telegram"); merr != nil || media == nil || media.url == "" {
		t.Errorf("tgLatestMedia: %v / %v", merr, media)
	} else {
		t.Logf("TG LATEST MEDIA OK — kind=%s chan=@%s views=%s url=%s", media.kind, media.channel, media.views, media.url[:min(60, len(media.url))])
	}
	// TG specific post (V2 ?before targeting)
	if media, merr := tgFetchPost(ctx, "https://t.me/telegram/459"); merr != nil || media.url == "" {
		t.Logf("TG POST 459 (may be text-only): %v", merr)
	} else {
		t.Logf("TG POST 459 MEDIA OK — kind=%s url=%s", media.kind, media.url[:min(60, len(media.url))])
	}
	// TG photo post via V2 route
	if media, merr := tgFetchPost(ctx, "https://t.me/telegram/452"); merr != nil || media.url == "" {
		t.Logf("TG POST 452 (may be text-only): %v", merr)
	} else {
		t.Logf("TG POST 452 MEDIA OK — kind=%s url=%s", media.kind, media.url[:min(60, len(media.url))])
	}

	// 3) engines quick re-check
	if res, err := fbProfileSearch(ctx, "carti"); err != nil || len(res) == 0 {
		t.Errorf("fbProfileSearch: %v / %d", err, len(res))
	} else {
		t.Logf("FB OK — %d results", len(res))
	}
	if res, err := twtAccountSearch(ctx, "elon musk"); err != nil || len(res) == 0 {
		t.Errorf("twtAccountSearch: %v / %d", err, len(res))
	} else {
		t.Logf("TWT OK — %d results", len(res))
	}
}
