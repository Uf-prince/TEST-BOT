package goldcmds

import (
	"testing"
	"time"
)

// A freshly-saved sticker must beat an older photo saved under the same name —
// this is the ".addsticker saved but sends the photo instead" regression.
func TestSelectNewestAssetKindStickerWins(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	mods := map[string]time.Time{"img": older, "sticker": newer}
	got := SelectNewestAssetKind("umar", func(kind string) (time.Time, bool) {
		m, ok := mods[kind]
		return m, ok
	})
	if got != "sticker" {
		t.Fatalf("SelectNewestAssetKind = %q, want sticker (newer)", got)
	}
}

// Ties fall back to AssetTriggerOrder (media order: img first).
func TestSelectNewestAssetKindTieFallsBackToOrder(t *testing.T) {
	same := time.Now()
	mods := map[string]time.Time{"img": same, "video": same, "sticker": same}
	got := SelectNewestAssetKind("x", func(kind string) (time.Time, bool) {
		m, ok := mods[kind]
		return m, ok
	})
	if got != "img" {
		t.Fatalf("tie-break = %q, want img (first in AssetTriggerOrder)", got)
	}
}

// No kind present => "".
func TestSelectNewestAssetKindNone(t *testing.T) {
	got := SelectNewestAssetKind("nope", func(string) (time.Time, bool) { return time.Time{}, false })
	if got != "" {
		t.Fatalf("SelectNewestAssetKind = %q, want empty", got)
	}
}
