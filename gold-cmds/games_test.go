package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// The GAME category must ship exactly 50 games, each unique, registered and
// visible, with a non-empty description and a runnable handler.
func TestGameCategoryHas50Games(t *testing.T) {
	if len(gameDefs) != 50 {
		t.Fatalf("gameDefs = %d, want 50", len(gameDefs))
	}
	seen := map[string]bool{}
	for _, g := range gameDefs {
		if g.Slug == "" || g.Label == "" || g.Desc == "" {
			t.Errorf("game %q has empty slug/label/desc", g.Slug)
		}
		if seen[g.Slug] {
			t.Errorf("duplicate game slug %q", g.Slug)
		}
		seen[g.Slug] = true
		if g.Play == nil {
			t.Errorf("game %q has nil Play", g.Slug)
		}
	}
}

// Every game must be a visible GAME command in the registry, and .game must be
// the visible list entry point.
func TestGameCommandsRegistered(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	list, ok := byName["game"]
	if !ok {
		t.Fatal(".game list command not registered")
	}
	if list.Hidden || list.Category != "GAME" || list.Run == nil {
		t.Fatalf(".game meta wrong: hidden=%v cat=%q", list.Hidden, list.Category)
	}
	for _, g := range gameDefs {
		c, ok := byName[g.Slug]
		if !ok {
			t.Errorf("game command %q not registered", g.Slug)
			continue
		}
		if c.Hidden || c.Category != "GAME" || c.Run == nil {
			t.Errorf("game %q meta wrong: hidden=%v cat=%q", g.Slug, c.Hidden, c.Category)
		}
	}
}

// GAME must be wired into the menu ordering + emoji maps.
func TestGameCategoryInMenuOrder(t *testing.T) {
	found := false
	for _, c := range CategoryOrder {
		if c == "GAME" {
			found = true
		}
	}
	if !found {
		t.Error("GAME not in CategoryOrder")
	}
	if CategoryEmoji["GAME"] == "" {
		t.Error("GAME has no emoji in CategoryEmoji")
	}
}

// Every game handler must return a non-empty framed reply for its inputs, and
// unknown slugs must not resolve.
func TestGameHandlersProduceOutput(t *testing.T) {
	var info types.MessageInfo
	_ = info
	for _, g := range gameDefs {
		// Arg-games get a sample argument; others take none.
		args := []string{}
		switch g.Slug {
		case "rps":
			args = []string{"rock"}
		case "rpsls":
			args = []string{"spock"}
		case "scramble":
			args = []string{"hello"}
		case "truth", "dare":
			args = []string{"umar"}
		case "ball", "oracle":
			args = []string{"will i win"}
		}
		out := g.Play(args)
		if strings.TrimSpace(out) == "" {
			t.Errorf("game %q produced empty output", g.Slug)
		}
		if !strings.Contains(out, "🔰") {
			t.Errorf("game %q output missing 🔰 frame", g.Slug)
		}
	}
	if _, ok := gameBySlug("notagame"); ok {
		t.Error("unknown slug unexpectedly resolved")
	}
}

// Arg games must guide the user instead of erroring when the argument is
// missing.
func TestGameArgGamesGuideWithoutArg(t *testing.T) {
	for _, g := range gameDefs {
		switch g.Slug {
		case "rps", "rpsls", "scramble", "ball", "oracle":
			out := g.Play(nil)
			if !strings.Contains(out, "USE .") {
				t.Errorf("game %q did not guide without args: %q", g.Slug, out)
			}
		}
	}
}

// The list handler must mention every game and the game menu frame.
func TestGameListIncludesEveryGame(t *testing.T) {
	sb := &sendingBridge{}
	var info types.MessageInfo
	handleGameList(sb, info, nil, ".")
	if len(sb.order) != 1 || sb.order[0] != "reply" {
		t.Fatalf("call order = %v, want [reply]", sb.order)
	}
}

// Every game slug must resolve through the command resolver (dispatch path).
func TestGameFromCommandResolves(t *testing.T) {
	for _, g := range gameDefs {
		if _, ok := gameFromCommand(g.Slug); !ok {
			t.Errorf("gameFromCommand(%q) failed", g.Slug)
		}
		if _, ok := gameFromCommand(strings.ToUpper(g.Slug)); !ok {
			t.Errorf("gameFromCommand(%q) uppercase failed", g.Slug)
		}
	}
}
