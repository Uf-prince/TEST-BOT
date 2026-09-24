package goldcmds

// ============================================================================
// GOLD-MD — EVERY GAME IS A RUNNING (SESSION) GAME
// File: playgames_bases.go
// ============================================================================
// OWNER ORDER (revision): "saare games ko chalne wale games bana do — 30s idle
// ya game ke apne time ke hisaab se band ho jaye."
//
// So every base game (the 50 families behind the .game menu) now also opens as
// a LIVE session command, exactly like .ttt / .c4 :
//
//   .<slug>        → game shuru, ek hi message (board) bhejta hai
//   <anything>     → seedha move / next round (bina prefix)
//   30s silence    → message edit ho kar GAME CLOSED
//   120s running   → message edit ho kar TIME UP (game ka apna time)
//
// The random one-shot games (dice, slot, coin, ...) re-roll on every move, so
// they run like an arcade: keep tapping for a new result. The argument games
// (rps, rpsls, scramble, ball, oracle, pickname) feed the player's move into
// the same Play function, so a wrong move flashes the usage hint instead of
// dying. This is why "koi bhi game ka naam likho, game khul jaye" holds for
// all 1000 menu entries — every one maps back to one of these 50 live games.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// baseGameNeedsArg lists the bases whose Play needs the player's move. All
// other bases ignore their argument and just re-roll, so their session treats
// any message as "play again".
var baseGameNeedsArg = map[string]bool{
	"rps":      true,
	"rpsls":    true,
	"scramble": true,
	"ball":     true,
	"oracle":   true,
	"pickname": true,
}

// baseGameHint is the "your move" line for a base game session.
func baseGameHint(slug string) string {
	switch slug {
	case "rps":
		return "ROCK / PAPER / SCISSORS bhejo"
	case "rpsls":
		return "ROCK / PAPER / SCISSORS / LIZARD / SPOCK bhejo"
	case "scramble":
		return "WORD bhejo — main use scramble karke dunga"
	case "ball":
		return "APNA SAWAL bhejo"
	case "oracle":
		return "APNA SAWAL bhejo"
	case "pickname":
		return "NAAM bhejo (jaise: ali sam ravi) — main ek chunuunga"
	default:
		return "PLAY — koi bhi message bhejo, game aage badhega"
	}
}

// newBasePlaySession builds a live session around base game `slug`.
func newBasePlaySession(slug string) *playSession {
	p := &playSession{}
	p.hint = baseGameHint(slug)
	last := ""
	if !baseGameNeedsArg[slug] {
		if body, ok, found := GameBasePlayBySlug(slug, nil); found && ok {
			last = body
		}
	}
	p.render = func() string {
		if last == "" {
			if body, _, found := GameBasePlayBySlug(slug, nil); found {
				return body
			}
			return p.hint
		}
		return last
	}
	p.input = func(text string) {
		var args []string
		if baseGameNeedsArg[slug] {
			args = strings.Fields(text)
		}
		body, ok, found := GameBasePlayBySlug(slug, args)
		if !found {
			return
		}
		if !ok {
			if baseGameNeedsArg[slug] {
				p.flash = "GALAT MOVE — " + baseGameHint(slug)
			}
			return
		}
		last = body
	}
	return p
}

// BaseSlugForGameN returns the base family slug behind game number n, so the
// main package can open it as a live session. Out-of-range n clamps to 1.
func BaseSlugForGameN(n int) string {
	if n < 1 || n > GameCount {
		n = 1
	}
	return gameBases[(n-1)%len(gameBases)].Slug
}

// StartBaseGame opens base game `slug` as a live session. Returns false when
// the slug is not a known base game (caller then falls back to one-shot).
func StartBaseGame(s SessionBridge, info types.MessageInfo, slug, prefix string) bool {
	if _, _, found := GameBasePlayBySlug(slug, nil); !found {
		return false
	}
	startPlaySession(s, info, slug, GameBaseLabel(slug), prefix, newBasePlaySession(slug))
	return true
}

// StartGameN opens game number n (1..1000) as a live session, titled with its
// edition design name (e.g. ".rolldice02" → "TURBO DICE ROLL").
func StartGameN(s SessionBridge, info types.MessageInfo, n int, prefix string) bool {
	if n < 1 || n > GameCount {
		return false
	}
	slug := BaseSlugForGameN(n)
	label := GameDesignName(n)
	startPlaySession(s, info, slug, label, prefix, newBasePlaySession(slug))
	return true
}

// StartGameByShortSlug opens the game a .game-menu short command name maps to
// (e.g. "rolldice02"). Edge flavour editions get their edition title; the base
// (flavour 0) name gets the plain game name.
func StartGameByShortSlug(s SessionBridge, info types.MessageInfo, name, prefix string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for n := 1; n <= GameCount; n++ {
		if GameShortSlug(n) == name {
			return StartGameN(s, info, n, prefix)
		}
	}
	return false
}
