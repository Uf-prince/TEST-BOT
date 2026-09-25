package goldcmds

import (
	"context"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// bvBridge is a minimal SessionBridge fake for the .botvideo tests. It records
// replies and the video setting that the handler writes.
type bvBridge struct {
	SessionBridge
	replies []string
	video   string
}

func (b *bvBridge) GetJID() string { return "botvideo@s.whatsapp.net" }
func (b *bvBridge) Reply(info types.MessageInfo, text string) {
	b.replies = append(b.replies, text)
}
func (b *bvBridge) ReplyWithID(info types.MessageInfo, text string) string {
	b.replies = append(b.replies, text)
	return "w-1"
}
func (b *bvBridge) EditMessage(info types.MessageInfo, id, text string) bool { return true }
func (b *bvBridge) DeleteMessage(info types.MessageInfo, id string) error {
	return nil
}
func (b *bvBridge) IsOwner(info types.MessageInfo) bool { return true }
func (b *bvBridge) DownloadQuotedMedia(info types.MessageInfo) ([]byte, string, bool) {
	return nil, "", false
}
func (b *bvBridge) VoiceURLPlayable(url string) bool     { return true }
func (b *bvBridge) GetBotVoiceSetting(def string) string { return def }
func (b *bvBridge) SetBotVoiceSetting(url string)        {}
func (b *bvBridge) GetBotVideoSetting(def string) string {
	if b.video == "" {
		return def
	}
	return b.video
}
func (b *bvBridge) SetBotVideoSetting(url string) { b.video = url }

func (b *bvBridge) last() string {
	if len(b.replies) == 0 {
		return ""
	}
	return b.replies[len(b.replies)-1]
}

// The visible command must live in OWNER & SYSTEM with the GOLD-MD desc style.
func TestBotVideoVisibleRegistration(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	c, ok := byName["botvideo"]
	if !ok {
		t.Fatal("command \"botvideo\" not registered")
	}
	if c.Hidden {
		t.Error("botvideo must be visible")
	}
	if c.Category != "OWNER & SYSTEM" {
		t.Errorf("botvideo category = %q, want OWNER & SYSTEM", c.Category)
	}
	if !strings.HasPrefix(c.Desc, "THIS COMMAND IS USED TO") {
		t.Errorf("botvideo desc not in GOLD-MD style: %q", c.Desc)
	}
	if !c.OwnerOnly {
		t.Error("botvideo must be owner-only")
	}
	if c.Run == nil {
		t.Error("botvideo has nil Run")
	}
}

// Aliases work but must stay hidden.
func TestBotVideoAliasesHidden(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, alias := range []string{"botvid", "botclip", "botmp4", "videobot", "vidbot"} {
		c, ok := byName[alias]
		if !ok {
			t.Errorf("alias %q not registered", alias)
			continue
		}
		if !c.Hidden {
			t.Errorf("alias %q must be hidden", alias)
		}
		if c.Run == nil {
			t.Errorf("alias %q has nil Run", alias)
		}
	}
	if _, ok := byName["clippbot"]; ok {
		t.Error("clippbot must not be registered")
	}
}

// A video URL ending in a video extension is stored directly.
func TestBotVideoDirectLink(t *testing.T) {
	b := &bvBridge{}
	handleBotVideo(b, types.MessageInfo{}, []string{"https://cdn.example.com/clip.mp4"}, ".")
	if b.video != "https://cdn.example.com/clip.mp4" {
		t.Errorf("video setting = %q", b.video)
	}
	if !strings.Contains(b.last(), "BOT VIDEO UPDATED") {
		t.Errorf("unexpected reply: %q", b.last())
	}

	// non-video URL is rejected and the setting stays empty
	b2 := &bvBridge{}
	handleBotVideo(b2, types.MessageInfo{}, []string{"https://cdn.example.com/pic.jpg"}, ".")
	if b2.video != "" {
		t.Errorf("jpg link must not be stored, got %q", b2.video)
	}
	if !strings.Contains(b2.last(), "INVALID VIDEO LINK") {
		t.Errorf("unexpected reply: %q", b2.last())
	}
}

// reset clears the stored video.
func TestBotVideoReset(t *testing.T) {
	b := &bvBridge{video: "https://cdn.example.com/old.mp4"}
	handleBotVideo(b, types.MessageInfo{}, []string{"reset"}, ".")
	if b.video != "" {
		t.Errorf("reset must clear the video, got %q", b.video)
	}
	if !strings.Contains(b.last(), "BOT VIDEO RESET") {
		t.Errorf("unexpected reply: %q", b.last())
	}
}

// No args / no video → the guide is shown (and never advertises aliases).
func TestBotVideoGuide(t *testing.T) {
	b := &bvBridge{}
	handleBotVideo(b, types.MessageInfo{}, nil, ".")
	g := b.last()
	if !strings.Contains(g, "BOT VIDEO CHANGE GUIDE") {
		t.Errorf("guide missing header: %q", g)
	}
	if !strings.Contains(g, ".BOTVIDEO RESET") {
		t.Errorf("guide missing reset line: %q", g)
	}
	for _, alias := range []string{"VIDEOBOT", "BOTCLIP", "BOTMP4", "VIDBOT"} {
		if strings.Contains(strings.ToUpper(g), alias) {
			t.Errorf("guide must not advertise alias %q", alias)
		}
	}
}

// The video extension helper maps the common mimetypes.
func TestBotVideoExtForMime(t *testing.T) {
	cases := map[string]string{
		"video/mp4":                ".mp4",
		"video/3gpp":               ".3gp",
		"video/webm":               ".webm",
		"video/quicktime":          ".mov",
		"video/x-matroska":         ".mkv",
		"application/octet-stream": ".mp4",
	}
	for in, want := range cases {
		if got := botVideoExtForMime(in); got != want {
			t.Errorf("botVideoExtForMime(%q) = %q, want %q", in, got, want)
		}
	}
	_ = context.Background()
}
