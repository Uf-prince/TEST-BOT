package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// gameBridge records the exact send + edit calls of a game so we can assert
// the ONE-message design: the board is sent ONCE and every move EDITS it.
type gameBridge struct {
	SessionBridge
	order    []string
	sentID   string
	editID   string
	editText []string
}

func (g *gameBridge) Reply(info types.MessageInfo, text string) {
	g.order = append(g.order, "reply")
}

func (g *gameBridge) ReplyWithID(info types.MessageInfo, text string) string {
	g.order = append(g.order, "send")
	g.sentID = "game-msg-1"
	return g.sentID
}

func (g *gameBridge) EditMessage(info types.MessageInfo, messageID string, newText string) bool {
	g.order = append(g.order, "edit")
	g.editID = messageID
	g.editText = append(g.editText, newText)
	return true
}

func newGameInfo(chat, sender string) types.MessageInfo {
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:   types.NewJID(chat, types.DefaultUserServer),
			Sender: types.NewJID(sender, types.DefaultUserServer),
		},
	}
}

// Exactly 14 interactive games, each unique, registered + visible.
func TestPlayGamesRegistered(t *testing.T) {
	if len(playGameDefs) != 14 {
		t.Fatalf("playGameDefs = %d, want 14", len(playGameDefs))
	}
	seen := map[string]bool{}
	for _, d := range playGameDefs {
		if d.Slug == "" || d.Label == "" || d.Desc == "" || d.New == nil {
			t.Errorf("game %q incomplete", d.Slug)
		}
		if seen[d.Slug] {
			t.Errorf("duplicate slug %q", d.Slug)
		}
		seen[d.Slug] = true
	}
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	if c, ok := byName["game"]; !ok || c.Hidden || c.Category != "GAME" {
		t.Errorf(".game list command wrong: %+v", c)
	}
	for _, d := range playGameDefs {
		c, ok := byName[d.Slug]
		if !ok || c.Hidden || c.Category != "GAME" || c.Run == nil {
			t.Errorf("game %q registry wrong: %+v", d.Slug, c)
		}
	}
}

// GAME category must stay wired into the menu maps.
func TestPlayGameCategoryInMenu(t *testing.T) {
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
		t.Error("GAME has no emoji")
	}
}

// The core ONE-message contract: starting a game sends exactly one message and
// edits nothing; each move then EDITS that same message.
func TestPlayGameSingleMessageEditedInPlace(t *testing.T) {
	destroyPlaySession(playKey(newGameInfo("c", "s")))
	gb := &gameBridge{}
	def, _ := playGameBySlug("guessnum")
	info := newGameInfo("923000000000", "923111111111")
	startPlayGame(gb, info, def, ".")
	if len(gb.order) != 2 || gb.order[0] != "send" || gb.order[1] != "edit" {
		t.Fatalf("start order = %v, want [send edit]", gb.order)
	}
	if gb.editID != gb.sentID {
		t.Errorf("first edit target %q != sent %q", gb.editID, gb.sentID)
	}
	if !HasPendingPlayGame(info.Sender.String()) {
		t.Fatal("session not registered after start")
	}
	// Feed a valid move through the dispatcher — it must EDIT, never send.
	sendsBefore := strings.Count(strings.Join(gb.order, ","), "send")
	if !DispatchPlayGameIncoming(gb, info, "50", ".") {
		t.Fatal("move was not consumed by the game")
	}
	sendsAfter := strings.Count(strings.Join(gb.order, ","), "send")
	if sendsAfter != sendsBefore {
		t.Errorf("move created a NEW message (send count %d -> %d); must only edit", sendsBefore, sendsAfter)
	}
	if gb.order[len(gb.order)-1] != "edit" {
		t.Errorf("last action = %q, want edit", gb.order[len(gb.order)-1])
	}
	destroyPlaySession(playKey(info))
}

// Plain (non-prefix) text must be consumed as a move; a real command must NOT
// be swallowed (so .menu still works mid-game).
func TestPlayGameCommandPassthrough(t *testing.T) {
	gb := &gameBridge{}
	def, _ := playGameBySlug("guessnum")
	info := newGameInfo("923000000099", "923111111199")
	startPlayGame(gb, info, def, ".")
	if DispatchPlayGameIncoming(gb, info, ".menu", ".") {
		t.Error(".menu was swallowed by the game; commands must pass through")
	}
	if !DispatchPlayGameIncoming(gb, info, "42", ".") {
		t.Error("plain move was not consumed")
	}
	destroyPlaySession(playKey(info))
}

// Quit words must close the game from inside or outside a game.
func TestPlayGameQuitWords(t *testing.T) {
	for _, w := range []string{"end", "stop", ".end", "quit"} {
		gb := &gameBridge{}
		def, _ := playGameBySlug("guessnum")
		info := newGameInfo("923000000088", "923111111188")
		startPlayGame(gb, info, def, ".")
		if !DispatchPlayGameIncoming(gb, info, w, ".") {
			t.Errorf("quit word %q not consumed", w)
		}
		if HasPendingPlayGame(info.Sender.String()) {
			t.Errorf("session survived quit word %q", w)
		}
		destroyPlaySession(playKey(info))
	}
}

// The idle timer must close the game (30s) and edit the message to a closed
// state, then drop the session. We shrink the window via a direct call.
func TestPlayGameIdleCloses(t *testing.T) {
	gb := &gameBridge{}
	def, _ := playGameBySlug("guessnum")
	info := newGameInfo("923000000077", "923111111177")
	startPlayGame(gb, info, def, ".")
	playSessMu.Lock()
	p := playSessions[playKey(info)]
	playSessMu.Unlock()
	if p == nil {
		t.Fatal("no session")
	}
	playIdleClose(p)
	if HasPendingPlayGame(info.Sender.String()) {
		t.Error("idle close did not drop the session")
	}
	last := gb.editText[len(gb.editText)-1]
	if !strings.Contains(last, "GAME CLOSED") {
		t.Errorf("idle close edit missing GAME CLOSED text:\n%s", last)
	}
}

// Every game must produce a non-empty board without panicking, and take at
// least a few moves of plausible input without crashing.
func TestPlayGameBoardsRenderAndSurviveInput(t *testing.T) {
	samples := map[string][]string{
		"ttt":        {"5", "1", "9"},
		"c4":         {"4", "3", "7"},
		"hangman":    {"a", "e", "z"},
		"guessnum":   {"50", "25", "75"},
		"wordle":     {"apple", "about", "wrong"},
		"rps5":       {"rock", "paper", "scissors"},
		"g21":        {"1", "2", "3"},
		"memory":     {"1", "2", "3", "4"},
		"simon":      {"1", "12", "123"},
		"mathsprint": {"1", "2", "3"},
		"quizgame":   {"a", "b", "c"},
		"highlow":    {"h", "l", "h"},
		"mines":      {"1", "7", "13"},
		"duel":       {"roll", "go", "roll"},
	}
	for _, d := range playGameDefs {
		gb := &gameBridge{}
		info := newGameInfo("9230000000"+d.Slug, "9231111111"+d.Slug)
		startPlayGame(gb, info, d, ".")
		if len(gb.editText) == 0 || strings.TrimSpace(gb.editText[0]) == "" {
			t.Errorf("%s: empty board", d.Slug)
			destroyPlaySession(playKey(info))
			continue
		}
		if !strings.Contains(gb.editText[0], "🔰") {
			t.Errorf("%s: board missing 🔰 header", d.Slug)
		}
		for _, mv := range samples[d.Slug] {
			// should never panic
			DispatchPlayGameIncoming(gb, info, mv, ".")
		}
		destroyPlaySession(playKey(info))
	}
}

// Games that emit emoji digit boards must stay stable across repeated renders
// (no map-iteration nondeterminism in the visible board).
func TestPlayGameBoardDeterministic(t *testing.T) {
	def, _ := playGameBySlug("ttt")
	p := def.New()
	first := p.render()
	for i := 0; i < 20; i++ {
		if p.render() != first {
			t.Fatal("ttt board render is nondeterministic")
		}
	}
}

// Tiny sanity: no session leaks across the whole suite (idle timers stopped).
func TestPlayGameNoSessionLeak(t *testing.T) {
	playSessMu.Lock()
	n := len(playSessions)
	playSessMu.Unlock()
	if n != 0 {
		t.Errorf("%d play sessions leaked", n)
	}
	_ = time.Second
}
