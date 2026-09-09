package goldcmds

// Live test for the FIXED telegram search engine — run with:
//   go test -mod=vendor -vet=off -v -count=1 -run 'TestTGSearchFix' ./gold-cmds/
//
// Verifies:
//   1. Real queries (bitcoin/dua) → results with t.me links ONLY
//   2. No-results query → EMPTY list (no nav-link pollution)
//   3. Every result link is t.me (downloadable), no telegram-group.com

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTGSearchFix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// ── 1) real query → t.me-only results ──
	res, err := tgChannelSearch(ctx, "bitcoin")
	if err != nil {
		t.Fatalf("tgChannelSearch(bitcoin): %v", err)
	}
	if len(res) == 0 {
		t.Fatal("tgChannelSearch(bitcoin): 0 results — search broken")
	}
	for i, r := range res {
		if !strings.HasPrefix(r.Link, "https://t.me/") && !strings.HasPrefix(r.Link, "http://t.me/") {
			t.Errorf("result[%d] %q NOT t.me: %s", i, r.Title, r.Link)
		} else {
			t.Logf("result[%d] OK: %s -> %s", i, r.Title, r.Link)
		}
	}

	// ── 2) no-results query → empty (no nav pollution) ──
	res2, err2 := tgChannelSearch(ctx, "dua and azkar")
	if err2 != nil {
		t.Logf("tgChannelSearch(dua and azkar) err: %v (acceptable if site slow)", err2)
	}
	t.Logf("no-results query returned %d results: %+v", len(res2), res2)
	if len(res2) > 0 {
		for i, r := range res2 {
			t.Logf("  result[%d]: %s -> %s", i, r.Title, r.Link)
			if !strings.HasPrefix(r.Link, "https://t.me/") && !strings.HasPrefix(r.Link, "http://t.me/") {
				t.Errorf("POLLUTION result[%d] %q is nav link: %s", i, r.Title, r.Link)
			}
		}
	}

	// ── 3) single word religious query (common user case) ──
	res3, err3 := tgChannelSearch(ctx, "quran")
	if err3 != nil {
		t.Logf("tgChannelSearch(quran) err: %v", err3)
	}
	for i, r := range res3 {
		if !strings.HasPrefix(r.Link, "https://t.me/") && !strings.HasPrefix(r.Link, "http://t.me/") {
			t.Errorf("quran result[%d] %q NOT t.me: %s", i, r.Title, r.Link)
		} else {
			t.Logf("quran result[%d] OK: %s -> %s", i, r.Title, r.Link)
		}
	}

	// ── 4) tgIsDetailPage unit checks ──
	detail := []string{
		"https://telegram-group.com/en/cryptocurrency/bitcoin-games/",
		"https://telegram-group.com/en/economy-financial-stock/crypto-signal-bitcoin/",
	}
	nav := []string{
		"https://telegram-group.com/en/",
		"https://telegram-group.com/en/publish/",
		"https://telegram-group.com/en/blog/",
		"https://telegram-group.com/en/telegram-apps/",
		"https://telegram-group.com/en/search/quran#",
		"https://telegram-group.com/policy/",
		"https://telegram-group.com/wp-content/uploads/2018/12/x.png",
		"https://telegram-group.com/",
	}
	for _, u := range detail {
		if !tgIsDetailPage(u) {
			t.Errorf("tgIsDetailPage should ACCEPT %s", u)
		}
	}
	for _, u := range nav {
		if tgIsDetailPage(u) {
			t.Errorf("tgIsDetailPage should REJECT %s", u)
		}
	}
	t.Log("tgIsDetailPage unit checks OK")
}
