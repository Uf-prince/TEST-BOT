package goldcmds

import (
	"context"
	"strings"
	"testing"
)

func TestURLSpansDetectsLinks(t *testing.T) {
	cases := []struct {
		in   string
		want string // the exact URL substring that must be protected
	}{
		{".fb https://www.facebook.com/watch/?v=123", "https://www.facebook.com/watch/?v=123"},
		{".tt https://vt.tiktok.com/ZSABC123/", "https://vt.tiktok.com/ZSABC123/"},
		{"LINK :❯ http://example.com/file.zip", "http://example.com/file.zip"},
		{"join www.gold-x.onrender.com now", "www.gold-x.onrender.com"},
		{"chat https://wa.me/923158930864", "https://wa.me/923158930864"},
		{"t.me/goldmdchannel", "t.me/goldmdchannel"},
		{"invite https://chat.whatsapp.com/ABCdef123", "https://chat.whatsapp.com/ABCdef123"},
		{"bare example.com/path/x", "example.com/path/x"},
	}
	for _, c := range cases {
		spans := trtURLSpans(c.in)
		found := false
		for _, sp := range spans {
			if c.in[sp[0]:sp[1]] == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("URL %q not protected in %q (spans=%v)", c.want, c.in, spans)
		}
	}
}

func TestURLTrailingPunctuationTrimmed(t *testing.T) {
	in := "see https://x.com/abc."
	spans := trtURLSpans(in)
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d (%v)", len(spans), spans)
	}
	if got := in[spans[0][0]:spans[0][1]]; got != "https://x.com/abc" {
		t.Errorf("trailing dot not trimmed: got %q", got)
	}
}

func TestURLProtectRestoreRoundtrip(t *testing.T) {
	in := "DOWNLOAD ❯ https://cdn.example.com/v/123.mp4 ❮ done"
	prot, urls := trtProtectURLs(in)
	if len(urls) != 1 || urls[0] != "https://cdn.example.com/v/123.mp4" {
		t.Fatalf("urls=%v", urls)
	}
	if strings.Contains(prot, "https://") {
		t.Fatalf("URL still present after protection: %q", prot)
	}
	out, ok := trtRestoreURLs(prot, urls)
	if !ok || out != in {
		t.Errorf("roundtrip failed: ok=%v out=%q want=%q", ok, out, in)
	}
}

func TestURLRestoreMissingSentinelFails(t *testing.T) {
	_, ok := trtRestoreURLs("no sentinel here", []string{"https://x.com"})
	if ok {
		t.Errorf("expected ok=false when sentinel missing")
	}
}

func TestLocalizeDigitsSkipsURLs(t *testing.T) {
	in := "LINK :❯ https://x.com/a123b ❮ 456"
	out := LocalizeDigits("ur", in)
	if !strings.Contains(out, "https://x.com/a123b") {
		t.Errorf("URL digits were localised: %q", out)
	}
	// The standalone 456 must still become Urdu digits.
	if strings.Contains(out, "456") {
		t.Errorf("standalone number not localised: %q", out)
	}
}

// TestTranslatePreservingURLsEndToEnd proves a link survives the FULL pipeline
// even when the translator mangles everything it can see. The fake translator
// simulates Google's worst habits: it strips "https://", swaps "." for the Urdu
// full stop, and localises digits. Because the URL is protected by a sentinel,
// none of that can reach it.
func TestTranslatePreservingURLsEndToEnd(t *testing.T) {
	old := clTranslator
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		// Mangle the visible text the way the free endpoint does.
		r := strings.NewReplacer(
			"https", "http", "://", " : ", ".", "\u06d4",
			"0", "\u06f0", "1", "\u06f1", "2", "\u06f2", "3", "\u06f3",
			"4", "\u06f4", "5", "\u06f5", "6", "\u06f6", "7", "\u06f7",
			"8", "\u06f8", "9", "\u06f9",
		)
		return r.Replace(text), nil
	}
	defer func() { clTranslator = old }()

	in := "DOWNLOAD :\u276f https://cdn.example.com/v/123.mp4"
	out, err := TranslatePreservingCommandTokens(context.Background(), in, "ur")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out, "https://cdn.example.com/v/123.mp4") {
		t.Errorf("URL was mangled by translation:\n got=%q\nwant contains %q", out, "https://cdn.example.com/v/123.mp4")
	}
}

// TestTranslatePreservingUserLinkCommand covers the exact owner case: a user
// command carrying a link, e.g. ".fb https://www.facebook.com/watch/?v=999".
func TestTranslatePreservingUserLinkCommand(t *testing.T) {
	old := clTranslator
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		return strings.NewReplacer("https", "http", ".", "\u06d4", "/", "\u2044").Replace(text), nil
	}
	defer func() { clTranslator = old }()

	in := "DOWNLOADING :\u276f https://www.facebook.com/watch/?v=999"
	out, err := TranslatePreservingCommandTokens(context.Background(), in, "ur")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out, "https://www.facebook.com/watch/?v=999") {
		t.Errorf("user link mangled:\n got=%q", out)
	}
}
