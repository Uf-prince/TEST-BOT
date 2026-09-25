package main

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// newTestUpstash builds an in-memory-only Upstash store (cache is enough for
// setting reads; the Storj command path is never reached for a fresh key).
func newTestUpstash() *Upstash {
	return &Upstash{cache: map[string]cacheEntry{}}
}

func newMenuSession(jid string) (*Session, *Upstash) {
	u := newTestUpstash()
	return &Session{JID: jid, Manager: &Manager{Redis: u}}, u
}

// Per-menu media must override the bot-wide pic/video, and the bot-wide value
// must be used as the fallback when a menu has no override.
func TestMenuMediaResolutionOrder(t *testing.T) {
	s, u := newMenuSession("bot@s.whatsapp.net")

	// Nothing set anywhere → default marker for pic, empty for video.
	if got := s.menuPicURL("logo"); got != defaultBotPicMarker {
		t.Errorf("empty pic = %q, want default marker", got)
	}
	if got := s.menuVideoURL("logo"); got != "" {
		t.Errorf("empty video = %q, want empty", got)
	}

	// Bot-wide set → used by every menu.
	u.cacheSet("settings:bot@s.whatsapp.net:botpic", "https://x/botpic.jpg")
	u.cacheSet("settings:bot@s.whatsapp.net:botvideo", "https://x/botvideo.mp4")
	if got := s.menuPicURL("logo"); got != "https://x/botpic.jpg" {
		t.Errorf("bot pic fallback = %q", got)
	}
	if got := s.menuVideoURL("converter"); got != "https://x/botvideo.mp4" {
		t.Errorf("bot video fallback = %q", got)
	}

	// Per-menu override wins for that menu only.
	u.cacheSet("settings:bot@s.whatsapp.net:menumedia:logo", "https://x/logo.jpg")
	u.cacheSet("settings:bot@s.whatsapp.net:menumedia:logo:video", "https://x/logo.mp4")
	if got := s.menuPicURL("logo"); got != "https://x/logo.jpg" {
		t.Errorf("logo override = %q", got)
	}
	if got := s.menuVideoURL("logo"); got != "https://x/logo.mp4" {
		t.Errorf("logo video override = %q", got)
	}
	if got := s.menuPicURL("game"); got != "https://x/botpic.jpg" {
		t.Errorf("game must keep the bot pic, got %q", got)
	}
	if got := s.menuVideoURL("game"); got != "https://x/botvideo.mp4" {
		t.Errorf("game must keep the bot video, got %q", got)
	}
}

// An empty key (callers with no specific menu) must always use the bot-wide
// values and never touch the per-menu table.
func TestMenuMediaEmptyKeyUsesBotWide(t *testing.T) {
	s, u := newMenuSession("bot@s.whatsapp.net")
	u.cacheSet("settings:bot@s.whatsapp.net:menumedia:menu", "https://x/menu.jpg")
	u.cacheSet("settings:bot@s.whatsapp.net:botpic", "https://x/botpic.jpg")
	if got := s.menuPicURL(""); got != "https://x/botpic.jpg" {
		t.Errorf("empty key = %q, want bot pic", got)
	}
}

// The bridge setter/getter pair must round-trip through Redis and clear on "".
func TestMenuMediaSettingRoundTrip(t *testing.T) {
	s, _ := newMenuSession("bot@s.whatsapp.net")
	br := &bridge{s: s}
	br.SetMenuMediaSetting("Logo", "https://x/logo.jpg")
	if got := br.GetMenuMediaSetting("logo", ""); got != "https://x/logo.jpg" {
		t.Fatalf("get = %q", got)
	}
	if got := br.GetMenuMediaSetting("LOGO:VIDEO", ""); got != "" {
		t.Fatalf("video key should be separate, got %q", got)
	}
	br.SetMenuMediaSetting("logo", "")
	if got := br.GetMenuMediaSetting("logo", ""); got != "" {
		t.Fatalf("after reset = %q, want empty", got)
	}
}

var _ = time.Second
var _ = types.MessageInfo{}
