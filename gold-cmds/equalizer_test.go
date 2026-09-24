package goldcmds

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// BeginGuard keeps the watchdog usable in tests: it hands back a live context
// with a no-op release, so runEqualizerEffect reaches its media-guard branch.
func (sb *sendingBridge) BeginGuard(user string) (context.Context, func()) {
	return context.Background(), func() {}
}

// The 1000-design engine must expose distinct combos, and every design must
// resolve to a non-empty ffmpeg filter chain.
func TestEqualizer1000DesignsUniqueAndNonEmpty(t *testing.T) {
	if EqCount != 1000 {
		t.Fatalf("EqCount = %d, want 1000", EqCount)
	}
	seen := map[string]int{}
	for n := 1; n <= EqCount; n++ {
		f := EqAudioFilter(n)
		if f == "" {
			t.Fatalf("design %d has empty filter", n)
		}
		if prev, dup := seen[f]; dup {
			t.Fatalf("design %d filter duplicates design %d: %q", n, prev, f)
		}
		seen[f] = n
		if EqDesignName(n) == "" {
			t.Fatalf("design %d has empty name", n)
		}
	}
	if len(seen) != EqCount {
		t.Fatalf("unique filters = %d, want %d", len(seen), EqCount)
	}
}

// eqCombo and eqComboIndex must be exact inverses, otherwise named aliases
// would point at the wrong design.
func TestEqualizerComboRoundTrip(t *testing.T) {
	for n := 1; n <= EqCount; n++ {
		s, tn, e := eqCombo(n)
		if got := eqComboIndex(s, tn, e); got != n {
			t.Fatalf("round trip %d -> (%d,%d,%d) -> %d", n, s, tn, e, got)
		}
	}
}

// Only designs whose speed family changes the audio duration may retime the
// picture; unknown/neutral designs must leave the video stream copyable.
func TestEqualizerVideoPTSMatchesSpeed(t *testing.T) {
	for n := 1; n <= EqCount; n++ {
		s, _, _ := eqCombo(n)
		want := eqSpeeds[s].PTS
		if got := EqVideoPTS(n); got != want {
			t.Fatalf("design %d VideoPTS = %q, want %q", n, got, want)
		}
	}
}

// The 5 original named effects stay visible EQUALIZER commands, and each maps
// onto a real design whose filter matches the table lookup.
func TestEqualizerNamedAliasesRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, name := range eqNamedAliasNames {
		c, ok := byName[name]
		if !ok {
			t.Errorf("alias %q not registered", name)
			continue
		}
		if c.Hidden || c.Category != "EQUALIZER" || c.Run == nil {
			t.Errorf("alias %q meta wrong: hidden=%v cat=%q run=%v", name, c.Hidden, c.Category, c.Run != nil)
		}
		eff, ok := eqDesignFromNamed(name)
		if !ok {
			t.Errorf("alias %q has no design", name)
			continue
		}
		if eff.Audio == "" {
			t.Errorf("alias %q resolved to empty filter", name)
		}
	}
}

// The bare .equalizer command must be visible in the EQUALIZER category so the
// menu shows exactly one entry point for the 1000 designs.
func TestEqualizerListCommandRegistered(t *testing.T) {
	found := false
	for _, c := range Commands() {
		if c.Name == "equalizer" {
			found = true
			if c.Hidden || c.Category != "EQUALIZER" || c.Run == nil {
				t.Fatalf("equalizer meta wrong: hidden=%v cat=%q", c.Hidden, c.Category)
			}
		}
	}
	if !found {
		t.Fatal(".equalizer command not registered")
	}
}

// The category must stay wired into the menu ordering + emoji maps.
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

// Without media the handler must guide the user instead of erroring.
func TestEqualizerWithoutMediaGuides(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	eff, _ := eqDesignFromNamed("bass")
	runEqualizerEffect(sb, info, ".", eff)

	if len(sb.order) != 1 || sb.order[0] != "reply" {
		t.Fatalf("call order = %v, want [reply]", sb.order)
	}
}

// An out-of-range design number must be clamped to a valid combo rather than
// panicking.
func TestEqualizerComboClampsOutOfRange(t *testing.T) {
	for _, n := range []int{-5, 0, EqCount + 1, 99999} {
		if EqAudioFilter(n) == "" {
			t.Errorf("EqAudioFilter(%d) empty", n)
		}
		if EqDesignName(n) == "" {
			t.Errorf("EqDesignName(%d) empty", n)
		}
	}
}

// EqRunN must not panic for out-of-range numbers and must reply with the range
// hint instead of applying an effect.
func TestEqualizerRunNOutOfRangeReplies(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	EqRunN(sb, info, nil, ".", EqCount+50)
	if len(sb.order) != 1 || sb.order[0] != "reply" {
		t.Fatalf("call order = %v, want [reply]", sb.order)
	}
}
