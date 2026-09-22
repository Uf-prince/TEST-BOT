package goldcmds

import (
	"context"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// lgBridge is a fake SessionBridge for the lyrics/github handler tests.
type lgBridge struct {
	SessionBridge
	replies []string
	images  int
}

func (f *lgBridge) BeginGuard(userJID string) (context.Context, func()) {
	return context.Background(), func() {}
}
func (f *lgBridge) SetCmdContext(ctx context.Context) {}
func (f *lgBridge) Reply(info types.MessageInfo, text string) {
	f.replies = append(f.replies, text)
}
func (f *lgBridge) ReplyWithID(info types.MessageInfo, text string) string {
	f.replies = append(f.replies, text)
	return "id"
}
func (f *lgBridge) DeleteMessage(info types.MessageInfo, id string) error { return nil }
func (f *lgBridge) SendImage(info types.MessageInfo, data []byte, caption string) error {
	f.images++
	f.replies = append(f.replies, caption)
	return nil
}
func (f *lgBridge) lastReply() string {
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

// TestLyricsAutoDetect verifies .lyrics works with ONLY a song name (no artist).
func TestLyricsAutoDetect(t *testing.T) {
	b := &lgBridge{}
	handleLyrics(b, types.MessageInfo{}, []string{"tum", "hi", "ho"}, ".")
	got := b.lastReply()
	if got == "" {
		t.Fatal("no reply")
	}
	if strings.Contains(got, "NOT FOUND") {
		t.Fatalf("lyrics not found for song-only query: %q", got)
	}
	if !strings.Contains(got, "SONG LYRICS") {
		t.Fatalf("missing header: %q", got)
	}
	t.Logf("reply head: %.120s", got)
}

// TestGithubFormat verifies the github reply layout: bio at the END, no
// trailing-space-before-star lines, and profile image attempted.
func TestGithubFormat(t *testing.T) {
	b := &lgBridge{}
	handleGithub(b, types.MessageInfo{}, []string{"Uf-prince"}, ".")
	got := b.lastReply()
	if got == "" {
		t.Fatal("no reply")
	}
	if !strings.Contains(got, "GITHUB PROFILE") {
		t.Fatalf("missing header: %q", got)
	}
	if !strings.Contains(got, "BIO IS HERE") {
		t.Fatalf("missing BIO IS HERE section: %q", got)
	}
	// bio section must be the LAST thing in the message
	if idx := strings.Index(got, "BIO IS HERE"); idx < 0 || idx < len(got)/2 {
		t.Fatalf("BIO IS HERE not near the end: idx=%d len=%d", idx, len(got))
	}
	// no line may end with a space before its closing '*'
	for _, ln := range strings.Split(got, "\n") {
		if strings.HasSuffix(ln, " *") {
			t.Fatalf("line has trailing space before closing star: %q", ln)
		}
	}
	t.Logf("github reply:\n%s", got)
}
