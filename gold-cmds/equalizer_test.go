package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// All 5 effects must exist exactly once, with a non-empty description and a
// real ffmpeg filter chain.
func TestEqualizerFiveEffectsRegistered(t *testing.T) {
	want := []string{"slowed", "revert", "robot", "bass", "dj"}
	if len(eqEffects) != len(want) {
		t.Fatalf("eqEffects = %d, want %d", len(eqEffects), len(want))
	}
	got := map[string]eqEffect{}
	for _, e := range eqEffects {
		if e.Desc == "" {
			t.Errorf("effect %q has empty Desc", e.Name)
		}
		if strings.TrimSpace(e.Audio) == "" {
			t.Errorf("effect %q has empty Audio filter", e.Name)
		}
		got[e.Name] = e
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("effect %q missing", w)
		}
	}
}

// Every effect must be a visible EQUALIZER command in the registry (so it
// shows up in .menu and resolves when typed).
func TestEqualizerCommandsInRegistry(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, e := range eqEffects {
		c, ok := byName[e.Name]
		if !ok {
			t.Errorf("command %q not registered", e.Name)
			continue
		}
		if c.Hidden {
			t.Errorf("command %q is hidden, must be visible", e.Name)
		}
		if c.Category != "EQUALIZER" {
			t.Errorf("command %q category = %q, want EQUALIZER", e.Name, c.Category)
		}
		if c.Run == nil {
			t.Errorf("command %q has nil Run", e.Name)
		}
	}
}

// The category must be part of the menu ordering + emoji maps, otherwise .menu
// would append it unordered / without an emoji.
func TestEqualizerCategoryInMenuOrder(t *testing.T) {
	found := false
	for _, c := range CategoryOrder {
		if c == "EQUALIZER" {
			found = true
		}
	}
	if !found {
		t.Error("EQUALIZER not in CategoryOrder")
	}
	if CategoryEmoji["EQUALIZER"] == "" {
		t.Error("EQUALIZER has no emoji in CategoryEmoji")
	}
}

// Without media the handler must guide the user with its own help text.
func TestEqualizerWithoutMediaGuides(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	handleEqualizerEffectiveAsync(nil, sb, info, ".", eqEffectsBySlug["bass"])

	if len(sb.order) != 1 || sb.order[0] != "reply" {
		t.Fatalf("call order = %v, want [reply]", sb.order)
	}
}

// Asking for an unknown effect name must be a no-op (never panics).
func TestEqualizerUnknownEffectNoop(t *testing.T) {
	if _, ok := eqEffectsBySlug["nope"]; ok {
		t.Error("unknown effect unexpectedly found")
	}
	if eff := handleEqualizerEffect("nope"); eff == nil {
		t.Error("handleEqualizerEffect returned nil for unknown name")
	}
}

// slowed must retime the video (pitch/duration change) while the others leave
// the picture untouched — otherwise A/V sync breaks on slowed videos.
func TestEqualizerVideoStretchOnlyForSlowed(t *testing.T) {
	for _, e := range eqEffects {
		wantStretch := e.Name == "slowed"
		if e.StretchVideo != wantStretch {
			t.Errorf("%s StretchVideo = %v, want %v", e.Name, e.StretchVideo, wantStretch)
		}
		if e.StretchVideo && strings.TrimSpace(e.VideoPTS) == "" {
			t.Errorf("%s stretch enabled but VideoPTS empty", e.Name)
		}
	}
}
