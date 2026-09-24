package goldcmds

import (
	"strings"
	"testing"
)

// Every base game must open as a LIVE session via StartBaseGame, send exactly
// ONE message, and keep editing that same message on each move.
func TestBaseGameSessionsRun(t *testing.T) {
	for _, slug := range GameBaseSlugs() {
		info := newGameInfo("92300"+slug, "92311"+slug)
		destroyPlaySession(playKey(info))
		gb := &gameBridge{}
		if !StartBaseGame(gb, info, slug, ".") {
			t.Errorf("StartBaseGame(%s) returned false", slug)
			continue
		}
		if len(gb.order) != 2 || gb.order[0] != "send" || gb.order[1] != "edit" {
			t.Errorf("%s start order = %v, want [send edit]", slug, gb.order)
		}
		if gb.editID != gb.sentID {
			t.Errorf("%s first edit target != sent", slug)
		}
		// A move must edit again (never send a new message), and never panic.
		before := len(gb.order)
		DispatchPlayGameIncoming(gb, info, "hello", ".")
		if len(gb.order) <= before {
			t.Errorf("%s move produced no edit", slug)
		}
		for _, o := range gb.order {
			if o == "reply" {
				t.Errorf("%s sent an extra reply message (want edit-only)", slug)
			}
		}
		destroyPlaySession(playKey(info))
	}
}

// Unknown slugs must be rejected so the caller can fall back to one-shot.
func TestStartBaseGameUnknown(t *testing.T) {
	gb := &gameBridge{}
	if StartBaseGame(gb, newGameInfo("c", "s"), "nope-not-a-game", ".") {
		t.Error("StartBaseGame accepted an unknown slug")
	}
}

// Every game 1..1000 must open as a live session through StartGameN.
func TestStartGameNAllDesigns(t *testing.T) {
	for n := 1; n <= GameCount; n++ {
		info := newGameInfo("92a", "93a")
		destroyPlaySession(playKey(info))
		gb := &gameBridge{}
		if !StartGameN(gb, info, n, ".") {
			t.Fatalf("StartGameN(%d) returned false", n)
		}
		if len(gb.order) != 2 {
			t.Fatalf("game %d order = %v", n, gb.order)
		}
		destroyPlaySession(playKey(info))
	}
}

// The short command names from the .game menu must open the matching game.
func TestStartGameByShortSlug(t *testing.T) {
	for _, n := range []int{1, 2, 51, 500, 1000} {
		name := GameShortSlug(n)
		info := newGameInfo("92b", "93b")
		destroyPlaySession(playKey(info))
		gb := &gameBridge{}
		if !StartGameByShortSlug(gb, info, name, ".") {
			t.Errorf("StartGameByShortSlug(%q) false (game %d)", name, n)
		}
		destroyPlaySession(playKey(info))
	}
	if StartGameByShortSlug(&gameBridge{}, newGameInfo("x", "y"), "not-a-slug", ".") {
		t.Error("StartGameByShortSlug accepted an unknown name")
	}
}

// Each base session must survive repeated moves without leaking or panicking,
// and the rendered board must never be empty.
func TestBaseGameSessionsSurviveMoves(t *testing.T) {
	for _, slug := range GameBaseSlugs() {
		info := newGameInfo("92c"+slug, "93c"+slug)
		destroyPlaySession(playKey(info))
		gb := &gameBridge{}
		if !StartBaseGame(gb, info, slug, ".") {
			continue
		}
		for i := 0; i < 25; i++ {
			DispatchPlayGameIncoming(gb, info, "x", ".")
		}
		last := gb.editText[len(gb.editText)-1]
		if strings.TrimSpace(last) == "" {
			t.Errorf("%s rendered an empty board", slug)
		}
		destroyPlaySession(playKey(info))
	}
	if len(playSessions) != 0 {
		for k := range playSessions {
			t.Errorf("session leak: %s", k)
		}
	}
}

// A running base game must close on its OWN time cap (not only on idle).
func TestBaseGameLifeCapArmed(t *testing.T) {
	info := newGameInfo("92d", "93d")
	destroyPlaySession(playKey(info))
	gb := &gameBridge{}
	if !StartBaseGame(gb, info, "rolldice", ".") {
		t.Fatal("start failed")
	}
	playSessMu.Lock()
	p := playSessions[playKey(info)]
	playSessMu.Unlock()
	if p == nil || p.life == nil {
		t.Fatal("life timer not armed — game has no own time limit")
	}
	if p.idle == nil {
		t.Fatal("idle timer not armed")
	}
	destroyPlaySession(playKey(info))
}

// The branded header must carry the owner/bot display name.
func TestGameHeaderBrand(t *testing.T) {
	h := gameHeader("DICE ROLL", "TEAM OWNER")
	if !strings.Contains(h, "DICE ROLL") || !strings.Contains(h, "TEAM OWNER") {
		t.Errorf("header missing label/brand:\n%s", h)
	}
}
