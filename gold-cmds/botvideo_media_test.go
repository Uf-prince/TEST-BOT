package goldcmds

import (
	"strings"
	"testing"
)

// mp4Head is the first bytes of a real ISO base media file (ftyp box).
func mp4Head(n int) []byte {
	b := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	for len(b) < n {
		b = append(b, 0)
	}
	return b
}

// A host HTML page must never pass as a video — this is exactly the bug where
// WhatsApp showed "this video is not available".
func TestBotVideoRejectsHTMLPage(t *testing.T) {
	page := []byte("<!DOCTYPE html><html><head><title>302 Found</title></head><body>...</body></html>")
	if botVideoIsValidMedia(page, "video/mp4") {
		t.Error("HTML page must be rejected as a video")
	}
	tiny := []byte("\x00\x00\x00\x18ftypmp42")
	if botVideoIsValidMedia(tiny, "video/mp4") {
		t.Error("tiny body must be rejected as a video")
	}
	if !botVideoIsValidMedia(mp4Head(4096), "video/mp4") {
		t.Error("real mp4 header must be accepted")
	}
}

// A URL that already names a file is returned untouched (no network call).
func TestResolveDirectMediaURLShortCircuit(t *testing.T) {
	for _, u := range []string{
		"https://qu.ax/x/O7xfZ.mp4",
		"https://cdn.example.com/a.mp4?sig=1",
		"https://cdn.example.com/clip.MP4",
	} {
		if got := ResolveDirectMediaURL(u); got != u {
			t.Errorf("direct url %q must be unchanged, got %q", u, got)
		}
	}
	if got := ResolveDirectMediaURL(""); got != "" {
		t.Errorf("empty url must stay empty, got %q", got)
	}
}

// MediaReferer points at the host origin; qu.ax 302s a Referer-less request to
// an HTML page, so this must be non-empty for absolute URLs.
func TestMediaReferer(t *testing.T) {
	if got := MediaReferer("https://qu.ax/x/a.mp4"); got != "https://qu.ax/" {
		t.Errorf("MediaReferer = %q, want https://qu.ax/", got)
	}
	if got := MediaReferer("not a url"); got != "" {
		t.Errorf("MediaReferer of a relative value = %q, want empty", got)
	}
	if !strings.HasPrefix(BrowserUA, "Mozilla/5.0") {
		t.Errorf("BrowserUA should look like a browser: %q", BrowserUA)
	}
}
