package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// mmBridge is a SessionBridge fake for the per-menu media commands. It records
// replies and the per-menu media that the handlers store.
type mmBridge struct {
	SessionBridge
	replies []string
	media   map[string]string
	owner   bool
}

func newMMBridge() *mmBridge {
	return &mmBridge{media: map[string]string{}, owner: true}
}

func (b *mmBridge) GetJID() string                            { return "menumedia@s.whatsapp.net" }
func (b *mmBridge) IsOwner(info types.MessageInfo) bool       { return b.owner }
func (b *mmBridge) Reply(info types.MessageInfo, text string) { b.replies = append(b.replies, text) }
func (b *mmBridge) ReplyWithID(info types.MessageInfo, t string) string {
	b.replies = append(b.replies, t)
	return "w-1"
}
func (b *mmBridge) EditMessage(info types.MessageInfo, id, t string) bool { return true }
func (b *mmBridge) DeleteMessage(info types.MessageInfo, id string) error { return nil }
func (b *mmBridge) DownloadImage(info types.MessageInfo) ([]byte, bool)   { return nil, false }
func (b *mmBridge) DownloadQuotedMedia(info types.MessageInfo) ([]byte, string, bool) {
	return nil, "", false
}
func (b *mmBridge) GetMenuMediaSetting(key, def string) string {
	if v := b.media[key]; v != "" {
		return v
	}
	return def
}
func (b *mmBridge) SetMenuMediaSetting(key, url string) { b.media[key] = url }

// Voice URL checks hit the network in production; the tests treat every link
// as playable so the setter paths can be exercised offline.
func (b *mmBridge) VoiceURLPlayable(url string) bool { return true }

func (b *mmBridge) last() string {
	if len(b.replies) == 0 {
		return ""
	}
	return b.replies[len(b.replies)-1]
}

// Every planned command must be registered and owner-only. The two headline
// commands are visible (for discovery); the other 32 stay hidden.
func TestMenuMediaCommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	hidden := []string{"logopic", "logovideo",
		"alivepic", "alivevideo", "fontpic", "gamevideo", "converterpic",
		"convertervideo", "toolsvideo", "equalizerpic", "groupmenupic",
		"aimenupic", "corevideo", "greactionvideo"}
	for _, want := range []string{"menupic", "menuvideo"} {
		c, ok := byName[want]
		if !ok {
			t.Fatalf("command %q not registered", want)
		}
		if !c.OwnerOnly {
			t.Errorf("%q must be owner-only", want)
		}
		if c.Hidden {
			t.Errorf("%q must be visible for discovery", want)
		}
	}
	for _, want := range hidden {
		c, ok := byName[want]
		if !ok {
			t.Fatalf("command %q not registered", want)
		}
		if !c.OwnerOnly {
			t.Errorf("%q must be owner-only", want)
		}
		if !c.Hidden {
			t.Errorf("%q must be hidden", want)
		}
	}
}

// menuMediaSettingKey must give pictures the bare key and videos a distinct
// ":video" suffix so both can be stored for the same menu at once.
func TestMenuMediaSettingKeySeparatesPicAndVideo(t *testing.T) {
	if got := MenuMediaSettingKey("logo", "pic"); got != "logo" {
		t.Errorf("pic key = %q, want logo", got)
	}
	if got := MenuMediaSettingKey("logo", "video"); got != "logo:video" {
		t.Errorf("video key = %q, want logo:video", got)
	}
	if got := MenuMediaSettingKey("LOGO", "VIDEO"); got != "logo:video" {
		t.Errorf("normalised key = %q, want logo:video", got)
	}
}

// .logopic <url> must store the picture under the menu's own key and confirm.
func TestLogopicSetsPerMenuPicture(t *testing.T) {
	b := newMMBridge()
	cmd := commandByName(t, "logopic")
	cmd.Run(b, types.MessageInfo{}, []string{"https://x.test/a.jpg"}, ".")
	if b.media["logo"] != "https://x.test/a.jpg" {
		t.Fatalf("logo pic = %q, want the url", b.media["logo"])
	}
	if !strings.Contains(b.last(), "LOGO MENU PIC UPDATED") {
		t.Fatalf("confirmation = %q", b.last())
	}
}

// .logovideo <url> must store the video under the separate ":video" key so it
// does not overwrite the picture.
func TestLogovideoSetsPerMenuVideoWithoutClobberingPic(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"https://x.test/a.jpg"}, ".")
	commandByName(t, "logovideo").Run(b, types.MessageInfo{}, []string{"https://x.test/v.mp4"}, ".")
	if b.media["logo"] != "https://x.test/a.jpg" {
		t.Errorf("pic clobbered by video: %q", b.media["logo"])
	}
	if b.media["logo:video"] != "https://x.test/v.mp4" {
		t.Fatalf("logo video = %q", b.media["logo:video"])
	}
}

// .logopic reset must clear ONLY the logo picture (empty value → cleared).
func TestLogopicResetClearsOnlyThatMenu(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"https://x.test/a.jpg"}, ".")
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"reset"}, ".")
	if b.media["logo"] != "" {
		t.Fatalf("reset left logo pic = %q", b.media["logo"])
	}
	if !strings.Contains(b.last(), "RESET") {
		t.Fatalf("reset confirmation = %q", b.last())
	}
}

// A wrong-kind link must be rejected (a .mp4 handed to .logopic).
func TestMenupicRejectsVideoLink(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "logopic").Run(b, types.MessageInfo{}, []string{"https://x.test/v.mp4"}, ".")
	if b.media["logo"] != "" {
		t.Fatalf("video link was accepted as a picture: %q", b.media["logo"])
	}
	if !strings.Contains(b.last(), "INVALID IMAGE LINK") {
		t.Fatalf("reply = %q", b.last())
	}
}

// Non-owner use must be refused for every new command.
func TestMenuMediaOwnerOnly(t *testing.T) {
	b := newMMBridge()
	b.owner = false
	commandByName(t, "menuvideo").Run(b, types.MessageInfo{}, []string{"https://x.test/v.mp4"}, ".")
	if b.media["menu:video"] != "" {
		t.Fatalf("non-owner changed the menu video")
	}
	if !strings.Contains(b.last(), "ONLY FOR ME") {
		t.Fatalf("reply = %q", b.last())
	}
}

// No-argument use must return the guide (not store anything).
func TestMenuMediaGuideNoArgs(t *testing.T) {
	b := newMMBridge()
	commandByName(t, "converterpic").Run(b, types.MessageInfo{}, nil, ".")
	if len(b.media) != 0 {
		t.Fatalf("guide call stored media: %v", b.media)
	}
	if !strings.Contains(b.last(), "CONVERTER MENU PIC GUIDE") {
		t.Fatalf("guide = %q", b.last())
	}
}

func commandByName(t *testing.T, name string) Command {
	t.Helper()
	for _, c := range Commands() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("command %q not found", name)
	return Command{}
}
