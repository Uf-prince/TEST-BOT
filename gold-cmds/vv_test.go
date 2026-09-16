package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// vvBridge is a fake SessionBridge for .vv tests. It records replies and
// simulates quoted-media presence.
type vvBridge struct {
	SessionBridge
	settings   map[string]string
	replies    []string
	owner      bool
	jid        string
	quotedID   string
	mediaData  []byte
	mediaMime  string
	revoked    int
	deleted    int
	sentMedia  int
	replyWithI int
}

func newVVBridge() *vvBridge {
	return &vvBridge{
		settings: map[string]string{},
		owner:    true,
		jid:      "923000000000@s.whatsapp.net",
	}
}

func (f *vvBridge) GetJID() string { return f.jid }

func (f *vvBridge) GetStatusSetting(field, def string) string {
	if v, ok := f.settings[field]; ok {
		return v
	}
	return def
}

func (f *vvBridge) SetStatusSetting(field, val string) { f.settings[field] = val }

func (f *vvBridge) Reply(info types.MessageInfo, text string) { f.replies = append(f.replies, text) }

func (f *vvBridge) ReplyWithID(info types.MessageInfo, text string) string {
	f.replyWithI++
	f.replies = append(f.replies, text)
	return "wait-id"
}

func (f *vvBridge) DeleteMessage(info types.MessageInfo, id string) error { f.deleted++; return nil }

func (f *vvBridge) RevokeQuotedMessage(chat types.JID, sender, id string) error {
	f.revoked++
	return nil
}

func (f *vvBridge) IsOwner(info types.MessageInfo) bool { return f.owner }

func (f *vvBridge) GetQuotedMessageID(info types.MessageInfo) (string, string, bool) {
	if f.quotedID == "" {
		return "", "", false
	}
	return f.quotedID, "", true
}

func (f *vvBridge) DownloadQuotedMedia(info types.MessageInfo) ([]byte, string, bool) {
	if len(f.mediaData) == 0 {
		return nil, "", false
	}
	return f.mediaData, f.mediaMime, true
}

func (f *vvBridge) SendImage(info types.MessageInfo, data []byte, caption string) error {
	f.sentMedia++
	return nil
}
func (f *vvBridge) SendVideo(info types.MessageInfo, data []byte, caption string, _ []byte, _, _, _ uint32) error {
	f.sentMedia++
	return nil
}
func (f *vvBridge) SendAudio(info types.MessageInfo, data []byte, _ string, _ uint32) error {
	f.sentMedia++
	return nil
}
func (f *vvBridge) SendSticker(info types.MessageInfo, data []byte) error { f.sentMedia++; return nil }
func (f *vvBridge) SendDocument(info types.MessageInfo, data []byte, name, mime, caption string) error {
	f.sentMedia++
	return nil
}

func (f *vvBridge) lastReply() string {
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

// ── bare .vv (no media mentioned) ──

// TestVVNoMediaSameMode — bare .vv in SAME mode shows guidance.
func TestVVNoMediaSameMode(t *testing.T) {
	b := newVVBridge()
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "VIEWONCE COMMAND INFO") {
		t.Errorf("reply = %q, want guidance", b.lastReply())
	}
}

// TestVVNoMediaInboxMode — bare .vv in INBOX mode STILL shows guidance
// (this is the bug the owner reported).
func TestVVNoMediaInboxMode(t *testing.T) {
	b := newVVBridge()
	VVSetMode(b, VVModeInbox)
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "VIEWONCE COMMAND INFO") {
		t.Errorf("reply = %q, want guidance even in inbox mode", b.lastReply())
	}
	if b.revoked != 0 {
		t.Errorf("bare .vv in inbox mode should NOT delete the command message (revoked=%d)", b.revoked)
	}
}

// TestVVHasQuotedMediaFalse — helper returns false when no quoted message.
func TestVVHasQuotedMediaFalse(t *testing.T) {
	b := newVVBridge()
	if VVHasQuotedMedia(b, types.MessageInfo{}) {
		t.Errorf("VVHasQuotedMedia = true, want false when no quoted message")
	}
}

// TestVVHasQuotedMediaTrue — helper returns true when a quoted message exists.
func TestVVHasQuotedMediaTrue(t *testing.T) {
	b := newVVBridge()
	b.quotedID = "ABC123"
	if !VVHasQuotedMedia(b, types.MessageInfo{}) {
		t.Errorf("VVHasQuotedMedia = false, want true when quoted message exists")
	}
}

// ── .vv WITH media mentioned ──

// TestVVWithMediaInboxMode — with media + inbox mode: silent delete + inbox send.
func TestVVWithMediaInboxMode(t *testing.T) {
	b := newVVBridge()
	VVSetMode(b, VVModeInbox)
	b.quotedID = "ABC123"
	b.mediaData = []byte("img")
	b.mediaMime = "image/jpeg"
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if b.revoked != 1 {
		t.Errorf("inbox mode with media should delete the command message (revoked=%d)", b.revoked)
	}
	if b.sentMedia != 1 {
		t.Errorf("inbox mode with media should send the media (sent=%d)", b.sentMedia)
	}
	if len(b.replies) != 0 {
		t.Errorf("inbox mode with media should be silent, got replies: %v", b.replies)
	}
}

// TestVVWithMediaSameMode — with media + same mode: wait msg + send + delete wait.
func TestVVWithMediaSameMode(t *testing.T) {
	b := newVVBridge()
	b.quotedID = "ABC123"
	b.mediaData = []byte("img")
	b.mediaMime = "image/jpeg"
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if b.sentMedia != 1 {
		t.Errorf("same mode with media should send the media (sent=%d)", b.sentMedia)
	}
	if b.replyWithI != 1 {
		t.Errorf("same mode should send a wait message (replyWithID=%d)", b.replyWithI)
	}
	if b.deleted != 1 {
		t.Errorf("same mode should delete the wait message (deleted=%d)", b.deleted)
	}
}

// TestVVWithQuotedButNoMedia — quoted something that is not view-once media.
func TestVVWithQuotedButNoMedia(t *testing.T) {
	b := newVVBridge()
	b.quotedID = "ABC123" // quoted, but DownloadQuotedMedia returns false
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "NOT A VIEWONCE MEDIA") {
		t.Errorf("reply = %q, want NOT A VIEWONCE MEDIA", b.lastReply())
	}
}

// TestVVNonOwner — non-owner gets refused.
func TestVVNonOwner(t *testing.T) {
	b := newVVBridge()
	b.owner = false
	handleVVAsync(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "ONLY FOR ME") {
		t.Errorf("reply = %q, want owner-only refusal", b.lastReply())
	}
}

// TestVVSetGuide — .vvset with no arg shows the guide with current mode.
func TestVVSetGuide(t *testing.T) {
	b := newVVBridge()
	handleVVSet(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "VVSET") {
		t.Errorf("reply = %q, want VVSET guide", b.lastReply())
	}
}
