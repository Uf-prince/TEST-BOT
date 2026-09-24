package main

import (
	"os"
	"testing"
)

// The Storj object id must use the SAME sanitisation as the on-disk name, so a
// restored object lands back under the exact local filename.
func TestAssetStorjIDMatchesDiskSanitisation(t *testing.T) {
	got := assetStorjID("15551230000@s.whatsapp.net", sanitiseAssetName("Party Bird"))
	want := "15551230000@s.whatsapp.net/party_bird"
	if got != want {
		t.Fatalf("assetStorjID = %q, want %q", got, want)
	}
}

// Voice lives in a distinct namespace from media assets.
func TestAssetStorjNamespaces(t *testing.T) {
	if assetStorjNS("voice") != "goldmd:voices" {
		t.Errorf("voice ns = %q", assetStorjNS("voice"))
	}
	if assetStorjNS("sticker") != "goldmd:assets:sticker" {
		t.Errorf("sticker ns = %q", assetStorjNS("sticker"))
	}
}

// With no Storj client configured, a save still lands on disk (backup is
// best-effort and must never block the primary local write).
func TestSaveAssetLocalStillWorksWithoutStorj(t *testing.T) {
	b := newAssetTestBridge(t)
	if !b.SaveCustomAsset("sticker", "umar", []byte("webp-bytes"), "image/webp") {
		t.Fatal("save failed without Storj")
	}
	got, mime, ok := b.GetCustomAsset("sticker", "umar")
	if !ok || string(got) != "webp-bytes" || mime != "image/webp" {
		t.Fatalf("round-trip failed: ok=%v mime=%q data=%q", ok, mime, got)
	}
}

// assetModTime is used to decide which kind serves a shared name; a saved
// asset must report a non-zero modtime so a newer save can win.
func TestAssetModTimeNonZeroAfterSave(t *testing.T) {
	b := newAssetTestBridge(t)
	if !b.SaveCustomAsset("img", "umar", []byte("jpeg"), "image/jpeg") {
		t.Fatal("save failed")
	}
	if b.assetModTime("img", "umar").IsZero() {
		t.Error("assetModTime is zero after a save")
	}
	if !b.assetModTime("video", "not-there").IsZero() {
		t.Error("assetModTime non-zero for a missing asset")
	}
}

// A stale .meta from a previous save must not leak into a later plain save.
func TestWriteAssetLocalClearsNothingUnexpected(t *testing.T) {
	b := newAssetTestBridge(t)
	if !b.SaveCustomAsset("img", "umar", []byte("a"), "image/jpeg") {
		t.Fatal("save a failed")
	}
	if !b.SaveCustomAsset("img", "umar", []byte("bb"), "image/png") {
		t.Fatal("save b failed")
	}
	got, mime, ok := b.GetCustomAsset("img", "umar")
	if !ok || string(got) != "bb" || mime != "image/png" {
		t.Fatalf("overwrite failed: ok=%v mime=%q data=%q", ok, mime, got)
	}
	if _, err := os.Stat(b.assetPath("img", "umar")); err != nil {
		t.Fatalf("payload missing: %v", err)
	}
}
