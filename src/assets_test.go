package main

import (
	"os"
	"path/filepath"
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// newAssetTestBridge builds a bridge with a throwaway DataDir and no Redis.
func newAssetTestBridge(t *testing.T) *bridge {
	t.Helper()
	dir := t.TempDir()
	s := &Session{
		JID:     "15551230000@s.whatsapp.net",
		Manager: &Manager{cfg: &Config{DataDir: dir}},
	}
	return &bridge{s: s}
}

// Save -> Get round-trips bytes + mimetype, and the file lands in the expected
// per-kind folder (this is the addvoice storage contract).
func TestAssetStoreSaveGetRoundTrip(t *testing.T) {
	b := newAssetTestBridge(t)
	payload := []byte("sticker-bytes")

	if !b.SaveCustomAsset("sticker", "Party Bird", payload, "image/webp") {
		t.Fatal("SaveCustomAsset returned false")
	}
	got, mime, ok := b.GetCustomAsset("sticker", "party bird")
	if !ok {
		t.Fatal("GetCustomAsset found nothing")
	}
	if string(got) != string(payload) {
		t.Errorf("bytes mismatch: %q", got)
	}
	if mime != "image/webp" {
		t.Errorf("mime = %q, want image/webp", mime)
	}
	if _, err := os.Stat(filepath.Join(b.assetsDir("sticker"), "party_bird.bin")); err != nil {
		t.Errorf("expected party_bird.bin on disk: %v", err)
	}
}

// A name containing path separators must not escape the assets folder.
func TestAssetNameSanitisedAgainstTraversal(t *testing.T) {
	b := newAssetTestBridge(t)
	if !b.SaveCustomAsset("img", "../../evil", []byte("x"), "image/png") {
		t.Fatal("save failed")
	}
	dir := b.assetsDir("img")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 { // .bin + .mime
		t.Fatalf("want 2 files in %s, got %d", dir, len(entries))
	}
	for _, e := range entries {
		if filepath.Dir(filepath.Join(dir, e.Name())) != dir {
			t.Errorf("entry escaped the folder: %s", e.Name())
		}
	}
}

// List falls back to the folder scan when Redis is absent, and stays sorted.
func TestAssetListWithoutRedis(t *testing.T) {
	b := newAssetTestBridge(t)
	for _, n := range []string{"Zeta", "alpha", "Mid"} {
		b.SaveCustomAsset("video", n, []byte("v"), "video/mp4")
	}
	got := b.ListCustomAssets("video")
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// Delete removes the payload and its sidecars.
func TestAssetDeleteRemovesSidecars(t *testing.T) {
	b := newAssetTestBridge(t)
	b.SaveCustomAsset("text", "hello", []byte("hi there"), "text/plain")
	if !b.DeleteCustomAsset("text", "hello") {
		t.Fatal("delete returned false")
	}
	if _, _, ok := b.GetCustomAsset("text", "hello"); ok {
		t.Error("asset still readable after delete")
	}
	if b.DeleteCustomAsset("text", "hello") {
		t.Error("second delete must return false")
	}
}

// Text assets keep their payload verbatim (no truncation / re-encoding).
func TestAssetTextPayload(t *testing.T) {
	b := newAssetTestBridge(t)
	body := "*BOLD* multiline\ntext with emoji 🔰"
	b.SaveCustomAsset("text", "greet", []byte(body), "text/plain")
	got, mime, ok := b.GetCustomAsset("text", "greet")
	if !ok {
		t.Fatal("text asset missing")
	}
	if string(got) != body {
		t.Errorf("payload changed: %q", got)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %q", mime)
	}
}

// The meta sidecar (.addcircle) round-trips seconds,width,height.
func TestAssetMetaRoundTrip(t *testing.T) {
	b := newAssetTestBridge(t)
	if !b.SaveCustomAssetMeta("circle", "dance", []byte("mp4"), "video/mp4", "12,480,480") {
		t.Fatal("SaveCustomAssetMeta failed")
	}
	_, _, meta, ok := b.GetCustomAssetMeta("circle", "dance")
	if !ok {
		t.Fatal("meta asset missing")
	}
	secs, w, h := parseAssetMeta(meta)
	if secs != 12 || w != 480 || h != 480 {
		t.Errorf("meta parsed as %d,%d,%d", secs, w, h)
	}
	// meta must not leak into the listed names
	for _, n := range b.ListCustomAssets("circle") {
		if n != "dance" {
			t.Errorf("unexpected listed name %q", n)
		}
	}
}

// AssetTriggerMatch accepts a bare single word, rejects commands/whitespace.
func TestAssetTriggerMatch(t *testing.T) {
	if got, ok := goldcmds.AssetTriggerMatch("  MyPhoto "); !ok || got != "myphoto" {
		t.Errorf("bare word rejected: %q %v", got, ok)
	}
	for _, bad := range []string{".addimg", "two words", "", "!x", "#x", "/x"} {
		if _, ok := goldcmds.AssetTriggerMatch(bad); ok {
			t.Errorf("%q must not match", bad)
		}
	}
}

// Every new command must be dispatchable; the aliases must stay hidden so the
// menu does not balloon.
func TestAssetCommandsRegistered(t *testing.T) {
	visible := map[string]bool{}
	all := map[string]bool{}
	for _, c := range goldcmds.Commands() {
		all[c.Name] = true
		if !c.Hidden {
			visible[c.Name] = true
		}
	}
	for _, n := range []string{"addimg", "addvideo", "addsticker", "addtext", "addcircle",
		"delimg", "delvideo", "delsticker", "deltext", "delcircle",
		"imglist", "videolist", "stickerlist", "textlist", "circlelist", "circle"} {
		if !visible[n] {
			t.Errorf("command %q must be visible", n)
		}
	}
	for _, n := range []string{"addimage", "addvid", "addstkr", "addtxt", "addptv",
		"ptv", "videonote", "savecircle", "addvideonote", "delimage", "delvid"} {
		if !all[n] {
			t.Errorf("alias %q must be registered", n)
		}
		if visible[n] {
			t.Errorf("alias %q must be hidden", n)
		}
	}
}

// The add* family must be owner-only (like .addvoice).
func TestAssetCommandsOwnerOnly(t *testing.T) {
	ownerOnly := goldcmds.OwnerOnlySet()
	for _, n := range []string{"addimg", "addvideo", "addsticker", "addtext", "addcircle"} {
		if !ownerOnly[n] {
			t.Errorf("%q must be owner-only", n)
		}
	}
	if ownerOnly["circle"] {
		t.Error(".circle must stay public (any group member can make a circle)")
	}
}
