package goldcmds

import (
	"strings"
	"testing"
)

// Exactly 1000 games, and every design label is unique (no repeats).
func TestGameDesignCountAndUniqueness(t *testing.T) {
	if GameCount != 1000 {
		t.Fatalf("GameCount = %d, want 1000", GameCount)
	}
	if len(gameBases) != 50 {
		t.Fatalf("gameBases = %d, want 50", len(gameBases))
	}
	if len(gameFlavors) != 20 {
		t.Fatalf("gameFlavors = %d, want 20", len(gameFlavors))
	}
	seen := map[string]int{}
	for n := 1; n <= GameCount; n++ {
		name := GameDesignName(n)
		if name == "" || name == " " {
			t.Fatalf("game %d has empty design name", n)
		}
		if prev, dup := seen[name]; dup {
			t.Fatalf("game %d and %d share design %q", prev, n, name)
		}
		seen[name] = n
	}
	// Out-of-range clamps to 1 rather than panicking.
	if GameDesignName(0) != GameDesignName(1) || GameDesignName(1001) != GameDesignName(1) {
		t.Error("GameDesignName did not clamp out-of-range input")
	}
}

// Every base game must return either a non-empty body or a usage hint, never a
// panic and never an empty string.
func TestGameBasesProduceOutput(t *testing.T) {
	for i, b := range gameBases {
		if b.Slug == "" || b.Name == "" || b.Play == nil {
			t.Errorf("base %d incomplete: %+v", i, b)
			continue
		}
		body, _ := b.Play([]string{"x"})
		if strings.TrimSpace(body) == "" {
			t.Errorf("base %q produced empty body", b.Slug)
		}
	}
}

// Every game n=1..1000 must render without panicking, with a header and a
// non-empty body.
func TestGameRunNAllDesigns(t *testing.T) {
	for n := 1; n <= GameCount; n++ {
		gb := &gameBridge{}
		GameRunN(gb, newGameInfo("g1", "g2"), []string{"x"}, ".", n)
		if len(gb.reply) == 0 || strings.TrimSpace(gb.reply[0]) == "" {
			t.Fatalf("game %d produced no reply", n)
		}
		full := strings.Join(gb.reply, "\n")
		if !strings.Contains(full, "🔰") {
			t.Fatalf("game %d reply missing 🔰 frame:\n%s", n, full)
		}
	}
}

// Argument-taking games (rps, scramble, ball, oracle, pickname) must ask for
// the argument when none is passed.
func TestGameArgGamesAskForArg(t *testing.T) {
	for _, slug := range []string{"rps", "scramble", "ball", "oracle", "pickname"} {
		idx := GameBaseIndex(slug)
		if idx == 0 {
			t.Fatalf("slug %q not found", slug)
		}
		gb := &gameBridge{}
		GameRunN(gb, newGameInfo("g1", "g2"), nil, ".", idx)
		full := strings.Join(gb.reply, "\n")
		if !strings.Contains(strings.ToUpper(full), "USE .") {
			t.Errorf("%s with no arg did not show usage:\n%s", slug, full)
		}
	}
	// With an argument they must play normally.
	idx := GameBaseIndex("rps")
	gb := &gameBridge{}
	GameRunN(gb, newGameInfo("g1", "g2"), []string{"rock"}, ".", idx)
	full := strings.Join(gb.reply, "\n")
	if !strings.Contains(full, "YOU: ROCK") {
		t.Errorf("rps with arg did not play:\n%s", full)
	}
}

// Base slug mapping must cover every base and be collision-free.
func TestGameBaseSlugNumbers(t *testing.T) {
	m := GameBaseSlugNumbers()
	if len(m) != 50 {
		t.Fatalf("slug map has %d entries, want 50", len(m))
	}
	for _, b := range gameBases {
		n, ok := m[b.Slug]
		if !ok || n < 1 || n > 50 {
			t.Errorf("slug %q missing/bad number %d", b.Slug, n)
		}
		if GameDesignName(n) != gameFlavors[0]+" "+b.Name {
			t.Errorf("slug %q maps to n=%d (%q), want %q", b.Slug, n, GameDesignName(n), gameFlavors[0]+" "+b.Name)
		}
	}
}

// The "game" command must exist, be visible, and live in the GAME category —
// and the interactive engine must no longer double-register it.
func TestGameMenuCommandRegisteredOnce(t *testing.T) {
	count := 0
	for _, c := range Commands() {
		if c.Name == "game" {
			count++
			if c.Hidden || c.Category != "GAME" || c.Run == nil {
				t.Errorf(".game command wrong: %+v", c)
			}
		}
	}
	if count != 1 {
		t.Errorf(".game registered %d times, want exactly 1", count)
	}
}

// TestGameRunNOutOfRange: game number beyond the range must reject gracefully.
func TestGameRunNOutOfRange(t *testing.T) {
	for _, n := range []int{0, -1, GameCount + 1} {
		gb := &gameBridge{}
		GameRunN(gb, newGameInfo("g1", "g2"), []string{"x"}, ".", n)
		full := strings.Join(gb.reply, "\n")
		if !strings.Contains(full, "1 SE 1000") {
			t.Errorf("GameRunN(%d) did not reject: %s", n, full)
		}
	}
}
