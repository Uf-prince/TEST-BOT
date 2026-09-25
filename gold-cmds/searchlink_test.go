package goldcmds

// ============================================================================
// GOLD-MD — Smart search link detection tests
// File: searchlink_test.go
// ============================================================================
// Verifies the full SearchDirectLink matrix without touching the network:
//   - correct-platform link → routed to the platform downloader
//   - wrong-platform link   → "GIVE ME THE VALID X LINK" error card
//   - self-search link      → query extracted, normal search flow
//   - host matching         → netflix.com is NOT x.com (substring trap)
// ============================================================================

import (
	"regexp"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// fakeBridge records every reply so tests can assert which route ran.
type fakeBridge struct {
	SessionBridge
	replies []string
}

func (f *fakeBridge) Reply(info types.MessageInfo, text string) {
	f.replies = append(f.replies, text)
}

func (f *fakeBridge) ReplyWithID(info types.MessageInfo, text string) string {
	f.replies = append(f.replies, text)
	return "test-msg-id"
}

func (f *fakeBridge) EditMessage(info types.MessageInfo, messageID string, newText string) bool {
	f.replies = append(f.replies, newText)
	return true
}

func (f *fakeBridge) DeleteMessage(info types.MessageInfo, messageID string) error {
	return nil
}

func (f *fakeBridge) SendDocumentFile(info types.MessageInfo, path, filename, mime, caption string) error {
	f.replies = append(f.replies, "[DOCUMENT SENT] "+filename)
	return nil
}

// runLink returns the last reply after feeding args to SearchDirectLink.
func runLink(t *testing.T, kind searchPickKind, arg string) (string, bool) {
	t.Helper()
	fb := &fakeBridge{}
	var info types.MessageInfo
	ok := SearchDirectLink(fb, info, kind, strings.Fields(arg), ".")
	if len(fb.replies) == 0 {
		return "", ok
	}
	return fb.replies[len(fb.replies)-1], ok
}

// ── wrong-platform links → error card ────────────────────────────────────────

func TestWrongLinkApkSearchFacebookLink(t *testing.T) {
	reply, consumed := runLink(t, pickAPK, "https://www.facebook.com/watch?v=1234567890")
	if !consumed {
		t.Fatal("expected the message to be consumed by the error card")
	}
	for _, want := range []string{"GIVE ME THE VALID APK LINK", "IT IS NOT A APK LINK", "EXAMPLE SAME LIKE THAT", "apkcombo.com"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("error card missing %q in:\n%s", want, reply)
		}
	}
	if !strings.Contains(reply, "FACEBOOK.COM") {
		t.Fatal("error card should say the pasted domain is FACEBOOK.COM")
	}
}

func TestWrongLinkTtSearchApkLink(t *testing.T) {
	reply, _ := runLink(t, pickTT, "https://apkcombo.com/whatsapp/com.whatsapp/")
	if !strings.Contains(reply, "GIVE ME THE VALID TIKTOK LINK") {
		t.Fatalf("expected TIKTOK error card, got:\n%s", reply)
	}
}

func TestWrongLinkIgSearchXLink(t *testing.T) {
	reply, _ := runLink(t, pickIG, "https://x.com/elonmusk/status/1234567890")
	if !strings.Contains(reply, "GIVE ME THE VALID INSTAGRAM LINK") {
		t.Fatalf("expected INSTAGRAM error card, got:\n%s", reply)
	}
	if !strings.Contains(reply, "X.COM") {
		t.Fatal("error card should name the pasted domain X.COM")
	}
}

func TestWrongLinkUnknownSite(t *testing.T) {
	reply, _ := runLink(t, pickAPK, "https://www.netflix.com/watch/123")
	if !strings.Contains(reply, "GIVE ME THE VALID APK LINK") {
		t.Fatalf("expected APK error card for unknown site, got:\n%s", reply)
	}
	if !strings.Contains(reply, "NETFLIX.COM") {
		t.Fatal("error card should name NETFLIX.COM")
	}
}

// ── correct-platform links → instant download route ─────────────────────────

func TestCorrectLinkApkSearchApkLink(t *testing.T) {
	fb := &fakeBridge{}
	var info types.MessageInfo
	ok := SearchDirectLink(fb, info, pickAPK, strings.Fields("https://apkcombo.com/whatsapp/com.whatsapp/"), ".")
	if !ok {
		t.Fatal("expected consumption for a valid apkcombo link")
	}
	// searchPickAPK downloads asynchronously — it replies DOWNLOADING APK
	joined := strings.Join(fb.replies, " ")
	if !strings.Contains(joined, "DOWNLOADING APK") {
		t.Fatalf("expected instant APK download flow, replies:\n%s", joined)
	}
}

func TestCorrectLinkTgSearchTmeLink(t *testing.T) {
	_, consumed := runLink(t, pickTG, "https://t.me/channelname/123")
	if !consumed {
		t.Fatal("expected a t.me link to be consumed by the TG route")
	}
}

func TestCorrectLinkTtSearchTiktokLink(t *testing.T) {
	_, consumed := runLink(t, pickTT, "https://www.tiktok.com/@user/video/1234567890")
	if !consumed {
		t.Fatal("expected a tiktok link to be consumed by the TT route")
	}
}

// ── self-search links → query extraction ────────────────────────────────────

func TestSelfSearchApkcomboSearchLink(t *testing.T) {
	q := searchLinkExtractQuery("https://apkcombo.com/search/whatsapp")
	if q != "whatsapp" {
		t.Fatalf("expected whatsapp query, got %q", q)
	}
	q = searchLinkExtractQuery("https://apkcombo.com/search/facebook%20lite")
	if q != "facebook lite" {
		t.Fatalf("expected decoded 'facebook lite', got %q", q)
	}
}

// ── host matching edge cases ─────────────────────────────────────────────────

func TestHostMatchingNetflixIsNotX(t *testing.T) {
	if searchLinkDomainMatch("netflix.com", "x.com") {
		t.Fatal("netflix.com must NOT match x.com (substring trap)")
	}
	if !searchLinkDomainMatch("www.x.com", "x.com") {
		t.Fatal("www.x.com should match x.com")
	}
	if !searchLinkDomainMatch("vm.tiktok.com", "tiktok.com") {
		t.Fatal("vm.tiktok.com should match tiktok.com")
	}
	if searchLinkDomainMatch("faketiktok.com", "tiktok.com") {
		t.Fatal("faketiktok.com must NOT match tiktok.com")
	}
}

func TestSearchLinkHost(t *testing.T) {
	cases := map[string]string{
		"https://VM.TikTok.com/x/abc":       "vm.tiktok.com",
		"https://www.youtube.com/watch?v=x": "www.youtube.com",
		"http://t.me/somechannel/12":        "t.me",
		"https://x.com/user/status/1":       "x.com",
	}
	for in, want := range cases {
		if got := searchLinkHost(in); got != want {
			t.Fatalf("searchLinkHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── link regex sanity ────────────────────────────────────────────────────────

func TestSearchLinkReFindsLinks(t *testing.T) {
	cases := []string{
		"download https://apkcombo.com/whatsapp/com.whatsapp/ now",
		"https://x.com/elonmusk/status/1234567890",
		"http://t.me/channel/99",
	}
	for _, c := range cases {
		if searchLinkRe.FindString(c) == "" {
			t.Fatalf("link regex missed: %s", c)
		}
	}
	if searchLinkRe.FindString("no links here just words") != "" {
		t.Fatal("link regex must not match plain text")
	}
	_ = regexp.MustCompile("a") // keep the regexp import used if assertions change
}
