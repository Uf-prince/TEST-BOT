package goldcmds

// ============================================================================
// GOLD-MD — INTERACTIVE PLAY GAMES (turn-based, ONE edited message)
// ============================================================================
// OWNER ORDER (2026):
//   "Bar bar command chalane ki zarurat nahi — ek game shuru karo, phir user
//    seedha apne move bhejta rahe. Bot khud jawab de aur WAHI message EDIT
//    karta rahe (naye messages ka flood na ho). Jab user 30 second tak kuch
//    na bheje to game khud band ho jaye."
//
// Flow:
//   .<game>          → bot ek message bhejta hai (board) + session start
//   <move>           → user seedha move bhejta hai (bina prefix)
//   bot              → wahi message edit karke naya board dikhata hai
//   30s silence      → same message edit ho kar GAME CLOSED + session delete
//
// Games: ttt c4 hangman guessnum wordle rps5 g21 memory simon mathsprint
//        quizgame highlow mines duel
// ============================================================================

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// playGameIdle is the "user ne kuch nahi bheja" window before auto-close.
const playGameIdle = 30 * time.Second

// playGameLife is a game's OWN hard time cap: a running game closes itself
// after this long even if the player is still active. Together with
// playGameIdle this satisfies the owner order — a game ends on 30s silence OR
// on its own running time, whichever comes first.
const playGameLife = 120 * time.Second

// playSession is ONE turn-based game belonging to ONE (chat, user) pair.
// The whole game lives in a single WhatsApp message that is edited in place.
type playSession struct {
	key    string
	slug   string
	label  string
	brand  string
	prefix string
	s      SessionBridge
	info   types.MessageInfo
	msgID  string

	moves int
	over  bool
	flash string
	hint  string
	idle  *time.Timer
	life  *time.Timer

	render func() string
	input  func(text string)
}

// gameBrand returns the name stamped on every game message. Owner order:
// the game shows the owner's configured display name (`.ownername` /
// `.ownernumber` setting) — falling back to the bot name, then "GOLD-MD".
func gameBrand(s SessionBridge) string {
	if s == nil {
		return "GOLD-MD"
	}
	// Defensive: a partially-implemented bridge (tests, early boot) must never
	// crash a running game just because the branding setters are missing.
	defer func() { _ = recover() }()
	if n := strings.TrimSpace(s.GetOwnerNameSetting("")); n != "" {
		return strings.ToUpper(n)
	}
	// The bot-name setting can hold a "default footer" marker; never let that
	// leak into the game header as if it were a real display name.
	if n := strings.TrimSpace(s.GetBotNameSetting("")); n != "" &&
		!strings.Contains(n, "GOLD_MD_DEFAULT") {
		return strings.ToUpper(n)
	}
	return "GOLD-MD"
}

var (
	playSessions = map[string]*playSession{}
	playSessMu   sync.Mutex
)

func playKey(info types.MessageInfo) string {
	chat := avideoStripDevice(info.Chat.String())
	sender := avideoStripDevice(info.Sender.String())
	if sender == "" {
		sender = chat
	}
	return chat + "::" + sender
}

func (p *playSession) armIdle() {
	if p.idle != nil {
		p.idle.Stop()
	}
	p.idle = time.AfterFunc(playGameIdle, func() { playIdleClose(p) })
}

func (p *playSession) stopIdle() {
	if p.idle != nil {
		p.idle.Stop()
	}
	if p.life != nil {
		p.life.Stop()
	}
}

// armLife starts the game's own hard time cap (once per session).
func (p *playSession) armLife() {
	if p.life != nil {
		return
	}
	p.life = time.AfterFunc(playGameLife, func() { playLifeClose(p) })
}

// push re-renders the game and EDITS the single message in place.
func (p *playSession) push() {
	if p.msgID == "" {
		return
	}
	var b strings.Builder
	b.WriteString(gameHeader(p.label, p.brand))
	b.WriteString(p.render())
	if p.flash != "" {
		b.WriteString("\n\n⚠️ *" + p.flash + "*")
		p.flash = ""
	}
	if p.over {
		b.WriteString("\n\n🏁 *GAME OVER*")
		b.WriteString("\n_NEW GAME: " + p.prefix + p.slug + "_")
	} else {
		b.WriteString("\n\n❮ YOUR MOVE ❯ — " + p.hint)
	}
	p.s.EditMessage(p.info, p.msgID, b.String())
}

// gameHeader is the shared boxed header used by every game message.
func gameHeader(label, brand string) string {
	var b strings.Builder
	b.WriteString("╔════ ≪ •❈• ≫ ════╗\n")
	b.WriteString("*🔰 " + label + " 🔰*\n")
	if brand != "" {
		b.WriteString("*| 🔰 | " + brand + "*\n")
	}
	b.WriteString("╚════ ≪ •❈• ≫ ════╝\n\n")
	return b.String()
}

// closeSession detaches p from the session map (only if it is still the live
// session) and returns true when this caller won the race.
func closeSession(p *playSession) bool {
	playSessMu.Lock()
	defer playSessMu.Unlock()
	cur, ok := playSessions[p.key]
	if !ok || cur != p {
		return false
	}
	delete(playSessions, p.key)
	return true
}

// playIdleClose fires 30s after the player last move: the message is edited
// to a closed state and the session is dropped.
func playIdleClose(p *playSession) {
	defer func() { _ = recover() }()
	if !closeSession(p) {
		return
	}
	p.stopIdle()
	if p.over {
		return
	}
	p.over = true
	p.flash = ""
	txt := gameHeader(p.label, p.brand) + p.render() +
		"\n\n\u23f0 *30 SECOND IDLE \u2014 GAME CLOSED*" +
		"\n_NEW GAME: " + p.prefix + p.slug + "_"
	p.s.EditMessage(p.info, p.msgID, txt)
}

// playLifeClose fires when the game own time cap (playGameLife) is reached:
// the game closes itself even though the player may still be active.
func playLifeClose(p *playSession) {
	defer func() { _ = recover() }()
	if !closeSession(p) {
		return
	}
	p.stopIdle()
	if p.over {
		return
	}
	p.over = true
	p.flash = ""
	txt := gameHeader(p.label, p.brand) + p.render() +
		"\n\n\u231b *TIME UP \u2014 GAME CLOSED*" +
		"\n_NEW GAME: " + p.prefix + p.slug + "_"
	p.s.EditMessage(p.info, p.msgID, txt)
}

func destroyPlaySession(key string) {
	playSessMu.Lock()
	p, ok := playSessions[key]
	delete(playSessions, key)
	playSessMu.Unlock()
	if ok && p != nil {
		p.stopIdle()
	}
}

// startPlaySession attaches a freshly built session to the (chat,user) key,
// sends the initial board message and arms BOTH clocks: the 30s idle window
// and the game's own hard time cap. Shared by the turn-based games and the
// base-family live games, so every game behaves the same way.
func startPlaySession(s SessionBridge, info types.MessageInfo, slug, label, prefix string, p *playSession) {
	key := playKey(info)
	destroyPlaySession(key)
	p.key = key
	p.slug = slug
	p.label = label
	p.prefix = prefix
	p.s = s
	p.brand = gameBrand(s)
	p.info = info
	playSessMu.Lock()
	playSessions[key] = p
	playSessMu.Unlock()
	p.msgID = s.ReplyWithID(info, "*\U0001f530 "+label+" START HO RAHA HAI...*")
	p.push()
	p.armIdle()
	p.armLife()
}

// startPlayGame is the shared ".ttt / .c4 / ..." entry point.
func startPlayGame(s SessionBridge, info types.MessageInfo, def playGameDef, prefix string) {
	startPlaySession(s, info, def.Slug, def.Label, prefix, def.New())
}

// HasPendingPlayGame reports whether this sender has a live game.
func HasPendingPlayGame(sender string) bool {
	playSessMu.Lock()
	defer playSessMu.Unlock()
	for k := range playSessions {
		parts := strings.SplitN(k, "::", 2)
		if len(parts) == 2 && (parts[1] == sender || parts[0] == sender) {
			return true
		}
	}
	return false
}

var playQuitWords = map[string]bool{
	"end": true, "stop": true, "quit": true, "exit": true, "leave": true,
	"band": true, "bandkaro": true, "chodo": true, "chhod": true, "close": true,
}

// DispatchPlayGameIncoming routes a message to the sender's live game.
// Plain text = move. Real commands (prefix) are passed through so .menu etc.
// still work. Returns true when the message was consumed by a game.
func DispatchPlayGameIncoming(s SessionBridge, info types.MessageInfo, text, prefix string) bool {
	key := playKey(info)
	playSessMu.Lock()
	p, ok := playSessions[key]
	playSessMu.Unlock()
	if !ok || p == nil {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	// If it looks like a command, only intercept quit words; else let the
	// normal command path run (so .menu, .ttt, .end ... behave normally).
	if strings.HasPrefix(t, prefix) {
		word := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, prefix)))
		if !playQuitWords[word] {
			return false
		}
		t = word
	}
	if playQuitWords[strings.ToLower(t)] {
		destroyPlaySession(key)
		p.over = true
		p.flash = ""
		p.s.EditMessage(p.info, p.msgID, p.render()+"\n\n🛑 *GAME CLOSED BY PLAYER*")
		return true
	}
	if p.over {
		return false
	}
	p.moves++
	p.input(t)
	p.armIdle()
	p.push()
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// shared helpers
// ─────────────────────────────────────────────────────────────────────────────

func countTrue(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

// playGameDef is one registered interactive game.
type playGameDef struct {
	Slug  string
	Label string
	Desc  string
	New   func() *playSession
}

// ═══════════════════════════ 1. TIC TAC TOE ═══════════════════════════

var tttLines = [][3]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}}

var tttNums = []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}

func tttWinner(b [9]int) int {
	for _, l := range tttLines {
		if b[l[0]] != 0 && b[l[0]] == b[l[1]] && b[l[1]] == b[l[2]] {
			return b[l[0]]
		}
	}
	return 0
}

func tttMinimax(b *[9]int, turn int) int {
	w := tttWinner(*b)
	if w == 2 {
		return 10
	}
	if w == 1 {
		return -10
	}
	full := true
	for _, v := range b {
		if v == 0 {
			full = false
			break
		}
	}
	if full {
		return 0
	}
	best := -1000
	if turn == 1 {
		best = 1000
	}
	for i := 0; i < 9; i++ {
		if b[i] != 0 {
			continue
		}
		b[i] = turn
		score := tttMinimax(b, 3-turn)
		b[i] = 0
		if turn == 2 {
			if score > best {
				best = score
			}
		} else {
			if score < best {
				best = score
			}
		}
	}
	return best
}

func newTTT() *playSession {
	var b [9]int
	p := &playSession{}
	p.hint = "1-9 me se koi khali jagah bhejo (tum ❌ ho)"
	p.render = func() string {
		c := make([]string, 9)
		copy(c, tttNums)
		for i, v := range b {
			switch v {
			case 1:
				c[i] = "❌"
			case 2:
				c[i] = "⭕"
			}
		}
		return c[0] + c[1] + c[2] + "\n" + c[3] + c[4] + c[5] + "\n" + c[6] + c[7] + c[8]
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil || n < 1 || n > 9 {
			p.flash = "1 SE 9 KE BEECH NUMBER BHEJO"
			return
		}
		i := n - 1
		if b[i] != 0 {
			p.flash = "YE JAGAH PEHLE SE BHARI HAI"
			return
		}
		b[i] = 1
		if w := tttWinner(b); w == 1 {
			p.over = true
			return
		}
		// bot move: minimax best, with a 15% slip so it is beatable.
		best, bestIdx := -1000, -1
		for j := 0; j < 9; j++ {
			if b[j] != 0 {
				continue
			}
			b[j] = 2
			sc := tttMinimax(&b, 1)
			b[j] = 0
			if sc > best {
				best, bestIdx = sc, j
			}
		}
		if bestIdx < 0 {
			p.over = true
			return
		}
		if rand.IntN(100) < 15 {
			var free []int
			for j := 0; j < 9; j++ {
				if b[j] == 0 {
					free = append(free, j)
				}
			}
			if len(free) > 0 {
				bestIdx = free[rand.IntN(len(free))]
			}
		}
		b[bestIdx] = 2
		if w := tttWinner(b); w == 2 {
			p.over = true
			return
		}
		for _, v := range b {
			if v == 0 {
				return
			}
		}
		p.over = true
	}
	return p
}

// ═══════════════════════════ 2. CONNECT FOUR ═══════════════════════════

func newC4() *playSession {
	var g [6][7]int // 0 empty, 1 user, 2 bot
	p := &playSession{}
	p.hint = "column bhejo 1-7 (tum 🔴 ho, pehle neeche girta hai)"
	drop := func(col, who int) int {
		for r := 5; r >= 0; r-- {
			if g[r][col] == 0 {
				g[r][col] = who
				return r
			}
		}
		return -1
	}
	win := func(who int) bool {
		for r := 0; r < 6; r++ {
			for c := 0; c < 7; c++ {
				if g[r][c] != who {
					continue
				}
				for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
					n := 0
					for k := 0; k < 4; k++ {
						rr, cc := r+d[0]*k, c+d[1]*k
						if rr < 0 || rr > 5 || cc < 0 || cc > 6 || g[rr][cc] != who {
							break
						}
						n++
					}
					if n == 4 {
						return true
					}
				}
			}
		}
		return false
	}
	full := func() bool {
		for c := 0; c < 7; c++ {
			if g[0][c] == 0 {
				return false
			}
		}
		return true
	}
	p.render = func() string {
		var b strings.Builder
		for r := 0; r < 6; r++ {
			for c := 0; c < 7; c++ {
				switch g[r][c] {
				case 1:
					b.WriteString("🔴")
				case 2:
					b.WriteString("🟡")
				default:
					b.WriteString("⬛")
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("1️⃣2️⃣3️⃣4️⃣5️⃣6️⃣7️⃣")
		return b.String()
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil || n < 1 || n > 7 {
			p.flash = "1 SE 7 KE BEECH COLUMN BHEJO"
			return
		}
		col := n - 1
		if drop(col, 1) < 0 {
			p.flash = "YE COLUMN BHARI HAI"
			return
		}
		if win(1) {
			p.over = true
			return
		}
		if full() {
			p.over = true
			return
		}
		// bot: win, block, else centre-biased random
		try := func(who int) int {
			for _, c := range []int{3, 2, 4, 1, 5, 0, 6} {
				if g[0][c] != 0 {
					continue
				}
				r := drop(c, who)
				ok := win(who)
				g[r][c] = 0
				if ok {
					return c
				}
			}
			return -1
		}
		bc := try(2)
		if bc < 0 {
			bc = try(1)
		}
		if bc < 0 {
			var opts []int
			for _, c := range []int{3, 2, 4, 1, 5, 0, 6} {
				if g[0][c] == 0 {
					opts = append(opts, c)
				}
			}
			bc = opts[rand.IntN(len(opts))]
		}
		drop(bc, 2)
		if win(2) || full() {
			p.over = true
		}
	}
	return p
}

// ═══════════════════════════ 3. HANGMAN ═══════════════════════════

var playWords = []string{"WHATSAPP", "PAKISTAN", "CRICKET", "BIRYANI", "FOOTBALL",
	"MOBILE", "GUITAR", "CHESS", "TEACHER", "SUMMER", "FRIEND", "CAMERA",
	"ROCKET", "MANGO", "WINTER", "GARDEN", "SILVER", "ORANGE", "PLANET", "THUNDER"}

func newHangman() *playSession {
	word := playWords[rand.IntN(len(playWords))]
	guessed := map[rune]bool{}
	wrong := 0
	p := &playSession{}
	p.hint = "ek letter bhejo"
	stages := []string{
		"  ┌───┐\n  │   │\n      │\n      │\n      │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n      │\n      │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n  │   │\n      │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n /│   │\n      │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n /│\\  │\n      │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n /│\\  │\n /    │\n      │\n═══════",
		"  ┌───┐\n  │   │\n  😀  │\n /│\\  │\n / \\  │\n      │\n═══════",
	}
	mask := func() string {
		var b strings.Builder
		for _, r := range word {
			if guessed[r] {
				b.WriteRune(r)
				b.WriteString(" ")
			} else {
				b.WriteString("_ ")
			}
		}
		return strings.TrimSpace(b.String())
	}
	p.render = func() string {
		wrongLetters := []string{}
		for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
			if guessed[r] {
				found := false
				for _, wr := range word {
					if wr == r {
						found = true
					}
				}
				if !found {
					wrongLetters = append(wrongLetters, string(r))
				}
			}
		}
		return stages[wrong] + "\n\n*WORD:* " + mask() +
			fmt.Sprintf("\n❤️ *LIVES:* %d/6", 6-wrong) +
			"\n❌ *WRONG:* " + strings.Join(wrongLetters, " ")
	}
	p.input = func(text string) {
		r := []rune(strings.ToUpper(strings.TrimSpace(text)))
		if len(r) == 0 || (r[0] < 'A' || r[0] > 'Z') {
			p.flash = "SIRF EK LETTER BHEJO"
			return
		}
		ch := r[0]
		if guessed[ch] {
			p.flash = "YE LETTER PEHLE BHEJ CHUKE HO"
			return
		}
		guessed[ch] = true
		inWord := false
		for _, wr := range word {
			if wr == ch {
				inWord = true
			}
		}
		if !inWord {
			wrong++
			if wrong >= 6 {
				p.over = true
				p.flash = "WORD THA: " + word
				return
			}
			return
		}
		for _, wr := range word {
			if !guessed[wr] {
				return
			}
		}
		p.over = true
		p.flash = "SAHI! WORD THA: " + word
	}
	return p
}

// ═══════════════════════════ 4. GUESS THE NUMBER ═══════════════════════════

func newGuessNum() *playSession {
	target := rand.IntN(100) + 1
	tries := 0
	p := &playSession{}
	p.hint = "1-100 ke beech number guess karo"
	p.render = func() string {
		return fmt.Sprintf("*MAIN SOCH RAHA HOON EK NUMBER (1-100)*\n\n🎯 *TRIES:* %d", tries)
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil || n < 1 || n > 100 {
			p.flash = "1 SE 100 KE BEECH NUMBER BHEJO"
			return
		}
		tries++
		switch {
		case n == target:
			p.over = true
			p.flash = fmt.Sprintf("BILKUL SAHI! %d — %d TRIES ME 🎉", target, tries)
		case n < target:
			p.flash = "⬆️ THODA BADA NUMBER (HIGHER)"
		default:
			p.flash = "⬇️ THODA CHOTA NUMBER (LOWER)"
		}
	}
	return p
}

// ═══════════════════════════ 5. WORDLE ═══════════════════════════

var playWordleWords = []string{"ABOUT", "APPLE", "BRAIN", "CHAIR", "DREAM", "EARTH",
	"FLAME", "GRAPE", "HOUSE", "JUICE", "KNIFE", "LIGHT", "MONEY", "NIGHT",
	"OCEAN", "PLANT", "QUEEN", "RIVER", "SNAKE", "TABLE", "URBAN", "VOICE",
	"WATER", "YOUNG", "ZEBRA", "BREAD", "CLOUD", "DANCE", "HONEY", "MUSIC"}

func newWordle() *playSession {
	word := playWordleWords[rand.IntN(len(playWordleWords))]
	var guesses []string
	p := &playSession{}
	p.hint = "5 letter ka word bhejo"
	eval := func(g string) string {
		gr := []rune(g)
		wr := []rune(word)
		res := make([]string, len(gr))
		used := make([]bool, len(wr))
		for i := range gr {
			if i < len(wr) && gr[i] == wr[i] {
				res[i] = "🟩"
				used[i] = true
			}
		}
		for i := range gr {
			if res[i] != "" {
				continue
			}
			res[i] = "⬛"
			for j := range wr {
				if !used[j] && wr[j] == gr[i] {
					res[i] = "🟨"
					used[j] = true
					break
				}
			}
		}
		return strings.Join(res, "")
	}
	p.render = func() string {
		var b strings.Builder
		for i := 0; i < 6; i++ {
			if i < len(guesses) {
				b.WriteString("`" + guesses[i] + "`  " + eval(guesses[i]) + "\n")
			} else {
				b.WriteString("`_____`  ⬜⬜⬜⬜⬜\n")
			}
		}
		return strings.TrimRight(b.String(), "\n")
	}
	p.input = func(text string) {
		g := strings.ToUpper(strings.TrimSpace(text))
		g = strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' {
				return r
			}
			return -1
		}, g)
		if len([]rune(g)) != 5 {
			p.flash = "EXACT 5 LETTER KA WORD BHEJO"
			return
		}
		if len(guesses) >= 6 {
			p.over = true
			return
		}
		guesses = append(guesses, g)
		if g == word {
			p.over = true
			p.flash = "SAHI JAWAB! 🎉"
			return
		}
		if len(guesses) >= 6 {
			p.over = true
			p.flash = "WORD THA: " + word
		}
	}
	return p
}

// ═══════════════════════════ 6. RPS BEST OF 5 ═══════════════════════════

func newRPS5() *playSession {
	uw, bw := 0, 0
	last := ""
	p := &playSession{}
	p.hint = "rock / paper / scissors likho"
	p.render = func() string {
		return fmt.Sprintf("*PLAYER ❮ %d ❯  —  BOT ❮ %d ❯* (first to 3)\n\n%s", uw, bw, last)
	}
	p.input = func(text string) {
		u := strings.ToUpper(strings.TrimSpace(text))
		switch u {
		case "R", "ROCK":
			u = "ROCK"
		case "P", "PAPER":
			u = "PAPER"
		case "S", "SCISSORS", "SCISSOR":
			u = "SCISSORS"
		default:
			p.flash = "rock / paper / scissors me se koi ek likho"
			return
		}
		opts := []string{"ROCK", "PAPER", "SCISSORS"}
		bot := opts[rand.IntN(3)]
		switch {
		case u == bot:
			last = fmt.Sprintf("🤝 DRAW — %s vs %s", u, bot)
		case (u == "ROCK" && bot == "SCISSORS") || (u == "PAPER" && bot == "ROCK") || (u == "SCISSORS" && bot == "PAPER"):
			uw++
			last = fmt.Sprintf("✅ TUM JEETE — %s vs %s", u, bot)
		default:
			bw++
			last = fmt.Sprintf("❌ BOT JEETA — %s vs %s", u, bot)
		}
		if uw >= 3 || bw >= 3 {
			p.over = true
			if uw >= 3 {
				p.flash = "TUM JEET GAYE 🏆"
			} else {
				p.flash = "BOT JEET GAYA 😔"
			}
		}
	}
	return p
}

// ═══════════════════════════ 7. 21 GAME ═══════════════════════════

func newG21() *playSession {
	total := 0
	lastBot := ""
	p := &playSession{}
	p.hint = "1, 2 ya 3 bhejo (jo bhi 21 bolega wo jeetega)"
	p.render = func() string {
		return fmt.Sprintf("*TOTAL ❮ %d ❯ / 21*\n\n%s", total, lastBot)
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil || n < 1 || n > 3 {
			p.flash = "1, 2 YA 3 BHEJO"
			return
		}
		if total+n > 21 {
			p.flash = fmt.Sprintf("21 SE ZYADA HO JAYEGA (%d+%d) — chhota number bhejo", total, n)
			return
		}
		total += n
		if total == 21 {
			p.over = true
			p.flash = "TUM JEET GAYE 🏆 (21 TUMNE BOLA)"
			return
		}
		// bot aims to hit total ≡ 1 (mod 4) → eventually 21.
		k := (1 - total) % 4
		if k <= 0 {
			k += 4
		}
		if k > 3 || total+k > 21 {
			k = rand.IntN(3) + 1
		}
		if total+k > 21 {
			k = 1
		}
		total += k
		if total == 21 {
			p.over = true
			p.flash = fmt.Sprintf("BOT NE 21 BOLA 😔 BOT JEETA (bot ne +%d bola)", k)
			return
		}
		lastBot = fmt.Sprintf("🤖 BOT NE +%d BOLA (total %d)", k, total)
	}
	return p
}

// ═══════════════════════════ 8. MEMORY MATCH ═══════════════════════════

func newMemory() *playSession {
	deck := []string{"🍎", "🍌", "🍇", "🍒", "🥭", "🍍", "🥝", "🍑"}
	cards := make([]string, 0, 16)
	for _, e := range deck {
		cards = append(cards, e, e)
	}
	rand.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
	matched := make([]bool, 16)
	revealed := make([]bool, 16)
	first := -1
	p := &playSession{}
	p.hint = "1-16 me se number bhejo (do same emoji dhundho)"
	p.render = func() string {
		var b strings.Builder
		for r := 0; r < 4; r++ {
			for c := 0; c < 4; c++ {
				i := r*4 + c
				if matched[i] || revealed[i] {
					b.WriteString(cards[i] + " ")
				} else {
					b.WriteString(fmt.Sprintf("`%02d`", i+1) + " ")
				}
			}
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("\n✅ *MATCHED:* %d/8", countTrue(matched)))
		return b.String()
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil || n < 1 || n > 16 {
			p.flash = "1 SE 16 KE BEECH NUMBER BHEJO"
			return
		}
		i := n - 1
		if matched[i] {
			p.flash = "YE CARD PEHLE SE MATCH HO CHUKA HAI"
			return
		}
		if first < 0 {
			first = i
			revealed[i] = true
			return
		}
		if i == first {
			p.flash = "WOHI CARD DOBARA? DOOSRI CARD BHEJO"
			return
		}
		revealed[i] = true
		if cards[i] == cards[first] {
			matched[i] = true
			matched[first] = true
			revealed[i] = false
			revealed[first] = false
			first = -1
			if countTrue(matched) == 16 {
				p.over = true
				p.flash = "SAB MATCH HO GAYE! 🎉"
			}
			return
		}
		revealed[first] = false
		first = -1
		p.flash = "KOI MATCH NAHI — PHIR SE TRY KARO"
	}
	return p
}

// ═══════════════════════════ 9. SIMON SAYS ═══════════════════════════

func newSimon() *playSession {
	var seq []int
	addStep := func() { seq = append(seq, rand.IntN(4)+1) }
	addStep()
	p := &playSession{}
	p.hint = "sequence dohraye jaise: 1 3 2 ya 132"
	seqText := func() string {
		parts := make([]string, len(seq))
		for i, v := range seq {
			parts[i] = strconv.Itoa(v)
		}
		return strings.Join(parts, " ")
	}
	p.render = func() string {
		return fmt.Sprintf("*YAAD RAKHO YE SEQUENCE (1-4):*\n\n🔢  *%s*", seqText())
	}
	p.input = func(text string) {
		g := digitsOnly(text)
		want := ""
		for _, v := range seq {
			want += strconv.Itoa(v)
		}
		if g != want {
			p.over = true
			p.flash = fmt.Sprintf("GALAT! SAHI THA: %s", want)
			return
		}
		addStep()
		p.flash = "SAHI! NAYA SEQUENCE DEKHO ⬆️"
	}
	return p
}

// ═══════════════════════════ 10. MATH SPRINT ═══════════════════════════

func newMathSprint() *playSession {
	score, limit := 0, 10
	var q string
	var ans int
	next := func() {
		op := []string{"+", "-", "×"}[rand.IntN(3)]
		var a, b int
		switch op {
		case "+":
			a, b = rand.IntN(90)+10, rand.IntN(90)+10
			ans = a + b
		case "-":
			a, b = rand.IntN(90)+10, rand.IntN(90)+10
			if a < b {
				a, b = b, a
			}
			ans = a - b
		default:
			a, b = rand.IntN(11)+2, rand.IntN(11)+2
			ans = a * b
			op = "*"
		}
		q = fmt.Sprintf("%d %s %d", a, op, b)
	}
	next()
	p := &playSession{}
	p.hint = "jawab ka number bhejo"
	p.render = func() string {
		return fmt.Sprintf("*❮ %d / %d ❯*\n✅ SCORE: %d\n\n*solve karo:* %s = ?", score, limit, score, q)
	}
	p.input = func(text string) {
		n, err := strconv.Atoi(digitsOnly(text))
		if err != nil {
			p.flash = "SIRF NUMBER BHEJO"
			return
		}
		if n == ans {
			score++
			p.flash = "✅ SAHI!"
		} else {
			p.flash = fmt.Sprintf("❌ GALAT — JAWAB THA %d", ans)
		}
		if score >= limit {
			p.over = true
			p.flash = fmt.Sprintf("🔥 SPRINT KHATAM! SCORE %d/%d", score, limit)
			return
		}
		next()
	}
	return p
}

// ═══════════════════════════ 11. QUIZ GAME ═══════════════════════════

type quizItem struct {
	Q    string
	Opts []string
	Ans  int
}

var playQuizItems = []quizItem{
	{"CAPITAL OF JAPAN?", []string{"TOKYO", "OSAKA", "KYOTO", "SEOUL"}, 0},
	{"HOW MANY CONTINENTS?", []string{"5", "6", "7", "8"}, 2},
	{"LARGEST OCEAN?", []string{"ATLANTIC", "INDIAN", "ARCTIC", "PACIFIC"}, 3},
	{"FASTEST LAND ANIMAL?", []string{"LION", "CHEETAH", "HORSE", "TIGER"}, 1},
	{"GAS WE BREATHE IN?", []string{"OXYGEN", "NITROGEN", "CO2", "HELIUM"}, 0},
	{"HOW MANY SIDES IN HEXAGON?", []string{"5", "6", "7", "8"}, 1},
	{"SUN RISES FROM?", []string{"WEST", "NORTH", "EAST", "SOUTH"}, 2},
	{"LARGEST PLANET?", []string{"EARTH", "MARS", "SATURN", "JUPITER"}, 3},
	{"HOW MANY DAYS IN LEAP YEAR?", []string{"364", "365", "366", "367"}, 2},
	{"NATIONAL ANIMAL OF PAKISTAN?", []string{"MARKHOR", "LION", "TIGER", "ELEPHANT"}, 0},
}

func newQuizGame() *playSession {
	items := make([]quizItem, len(playQuizItems))
	copy(items, playQuizItems)
	rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	if len(items) > 5 {
		items = items[:5]
	}
	idx, score := 0, 0
	p := &playSession{}
	p.hint = "A / B / C / D bhejo"
	p.render = func() string {
		i := items[idx]
		var b strings.Builder
		b.WriteString(fmt.Sprintf("✅ SCORE: %d\n❮ Q %d / %d ❯\n\n*%s*\n", score, idx+1, len(items), i.Q))
		labels := []string{"A", "B", "C", "D"}
		for k, o := range i.Opts {
			b.WriteString(fmt.Sprintf("\n*%s)* %s", labels[k], o))
		}
		return b.String()
	}
	p.input = func(text string) {
		u := strings.ToUpper(strings.TrimSpace(text))
		u = strings.TrimRight(u, ".)")
		if len(u) == 0 {
			p.flash = "A / B / C / D BHEJO"
			return
		}
		labels := []string{"A", "B", "C", "D"}
		chosen := -1
		for k, l := range labels {
			if u == l {
				chosen = k
			}
		}
		if chosen < 0 {
			p.flash = "A / B / C / D ME SE BHEJO"
			return
		}
		i := items[idx]
		if chosen == i.Ans {
			score++
			p.flash = "✅ SAHI — " + i.Opts[i.Ans]
		} else {
			p.flash = "❌ GALAT — SAHI THA: " + i.Opts[i.Ans]
		}
		idx++
		if idx >= len(items) {
			p.over = true
			p.flash = fmt.Sprintf("QUIZ KHATAM! SCORE %d/%d", score, len(items))
		}
	}
	return p
}

// ═══════════════════════════ 12. HIGH LOW ═══════════════════════════

func newHighLow() *playSession {
	cur := rand.IntN(13) + 1
	score := 0
	p := &playSession{}
	p.hint = "H (high) ya L (low) bhejo"
	p.render = func() string {
		return fmt.Sprintf("✅ SCORE: %d\n\n🃏 *CURRENT CARD:* ❮ %s ❯\n\nAGLI CARD IS SE ZYADA (H) YA KAM (L)?", score, cardName(cur))
	}
	p.input = func(text string) {
		u := strings.ToUpper(strings.TrimSpace(text))
		if u != "H" && u != "L" && u != "HIGH" && u != "LOW" {
			p.flash = "H YA L BHEJO"
			return
		}
		if u == "HIGH" {
			u = "H"
		}
		if u == "LOW" {
			u = "L"
		}
		nxt := rand.IntN(13) + 1
		for nxt == cur {
			nxt = rand.IntN(13) + 1
		}
		win := (u == "H" && nxt > cur) || (u == "L" && nxt < cur)
		if win {
			score++
			p.flash = fmt.Sprintf("✅ SAHI! CARD THA ❮ %s ❯", cardName(nxt))
			cur = nxt
			return
		}
		p.over = true
		p.flash = fmt.Sprintf("❌ GALAT — CARD THA ❮ %s ❯ | FINAL SCORE %d", cardName(nxt), score)
	}
	return p
}

// ═══════════════════════════ 13. MINES ═══════════════════════════

func newMines() *playSession {
	const n = 5
	const mineCount = 5
	mines := make([]bool, n*n)
	safe := make([]bool, n*n)
	revealed := make([]bool, n*n)
	placed := 0
	for placed < mineCount {
		i := rand.IntN(n * n)
		if !mines[i] {
			mines[i] = true
			placed++
		}
	}
	adj := func(i int) int {
		r, c := i/n, i%n
		cnt := 0
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				if dr == 0 && dc == 0 {
					continue
				}
				rr, cc := r+dr, c+dc
				if rr >= 0 && rr < n && cc >= 0 && cc < n && mines[rr*n+cc] {
					cnt++
				}
			}
		}
		return cnt
	}
	p := &playSession{}
	p.hint = "1-25 me se cell reveal karo (5 mines hain, bachao!)"
	p.render = func() string {
		var b strings.Builder
		for r := 0; r < n; r++ {
			for c := 0; c < n; c++ {
				i := r*n + c
				switch {
				case revealed[i] && mines[i]:
					b.WriteString("💥 ")
				case revealed[i]:
					b.WriteString(fmt.Sprintf("%d️⃣", adj(i)))
					if adj(i) > 9 {
						b.WriteString(" ")
					}
				default:
					b.WriteString(fmt.Sprintf("`%02d`", i+1) + " ")
				}
			}
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("\n💣 *MINES:* %d | ✅ *SAFE OPEN:* %d/%d", mineCount, countTrue(safe), n*n-mineCount))
		return b.String()
	}
	p.input = func(text string) {
		num, err := strconv.Atoi(digitsOnly(text))
		if err != nil || num < 1 || num > n*n {
			p.flash = "1 SE 25 KE BEECH NUMBER BHEJO"
			return
		}
		i := num - 1
		if revealed[i] {
			p.flash = "YE CELL PEHLE SE OPEN HAI"
			return
		}
		revealed[i] = true
		if mines[i] {
			for j := range mines {
				if mines[j] {
					revealed[j] = true
				}
			}
			p.over = true
			p.flash = "💥 BOOM! MINE PE LAND KAR GAYE — GAME OVER"
			return
		}
		safe[i] = true
		if countTrue(safe) == n*n-mineCount {
			p.over = true
			p.flash = "🎉 SAB SAFE CELLS KHOL DIYE — TUM JEET GAYE!"
		}
	}
	return p
}

// ═══════════════════════════ 14. DICE DUEL ═══════════════════════════

func newDuel() *playSession {
	uw, bw := 0, 0
	target := 50
	last := ""
	p := &playSession{}
	p.hint = "roll karo — koi bhi message bhejo (ya 'roll' likho)"
	p.render = func() string {
		return fmt.Sprintf("*RACE TO %d*\n\n👤 YOU: %d\n🤖 BOT: %d\n\n%s", target, uw, bw, last)
	}
	p.input = func(text string) {
		d6 := func() int { return rand.IntN(6) + 1 }
		u, b := d6()+d6(), d6()+d6()
		uw += u
		bw += b
		last = fmt.Sprintf("🎲 TUMNE %d PHENKA | 🤖 BOT NE %d", u, b)
		if uw >= target || bw >= target {
			p.over = true
			if uw >= target && bw >= target {
				p.flash = "BARABAR! 🤝"
			} else if uw >= target {
				p.flash = "TUM JEET GAYE 🏆"
			} else {
				p.flash = "BOT JEET GAYA 😔"
			}
		}
	}
	return p
}

// ═══════════════════════════ registry ═══════════════════════════

func cardName(n int) string {
	names := map[int]string{1: "A", 11: "J", 12: "Q", 13: "K"}
	if s, ok := names[n]; ok {
		return s
	}
	return strconv.Itoa(n)
}

var playGameDefs = []playGameDef{
	{"ttt", "TIC TAC TOE", "BOT KE AGAINST TIC TAC TOE KHELO. JUST TYPE .TTT.", newTTT},
	{"c4", "CONNECT FOUR", "BOT KE AGAINST CONNECT FOUR KHELO. JUST TYPE .C4.", newC4},
	{"hangman", "HANGMAN", "WORD KE LETTER GUESS KARO — 6 LIVES. JUST TYPE .HANGMAN.", newHangman},
	{"guessnum", "GUESS NUMBER", "1-100 KA NUMBER GUESS KARO, BOT HIGHER/LOWER BATAYEGA. JUST TYPE .GUESSNUM.", newGuessNum},
	{"wordle", "WORDLE", "5 LETTER WORD GUESS KARO — 6 TRIES. JUST TYPE .WORDLE.", newWordle},
	{"rps5", "RPS BEST OF 5", "ROCK PAPER SCISSORS BEST OF 5 BOT KE AGAINST. JUST TYPE .RPS5.", newRPS5},
	{"g21", "21 GAME", "1-3 NUMBER BHEJTE JAO, JO 21 BOLEGA WO JEETEGA. JUST TYPE .G21.", newG21},
	{"memory", "MEMORY MATCH", "16 CARDS ME SE 8 PAIRS MATCH KARO. JUST TYPE .MEMORY.", newMemory},
	{"simon", "SIMON SAYS", "SEQUENCE YAAD RAKH KE DOHRAO. JUST TYPE .SIMON.", newSimon},
	{"mathsprint", "MATH SPRINT", "10 MATH SAWAAL — SPEED SE JAWAB DO. JUST TYPE .MATHSPRINT.", newMathSprint},
	{"quizgame", "QUIZ GAME", "5 SAWAAL KA QUIZ GAME — A/B/C/D. JUST TYPE .QUIZGAME.", newQuizGame},
	{"highlow", "HIGH LOW", "CARD ZYADA (H) YA KAM (L) — STREAK BANAO. JUST TYPE .HIGHLOW.", newHighLow},
	{"mines", "MINES", "5x5 GRID ME 5 MINES SE BACH KE SAFE CELLS KHOLO. JUST TYPE .MINES.", newMines},
	{"duel", "DICE DUEL", "BOT KE AGAINST 50 TAK DICE RACE. JUST TYPE .DUEL.", newDuel},
}

func playGameListText(prefix string) string {
	var b strings.Builder
	b.WriteString("╔════ ≪ •❈• ≫ ════╗\n")
	b.WriteString("*🔰 GAME MENU 🔰*\n")
	b.WriteString("╚════ ≪ •❈• ≫ ════╝\n\n")
	b.WriteString("*Ek baar command bhejo, phir seedha apne move bhejte raho —\nsame message edit hota rahega, 30s chup rahe to game band.*\n")
	for i, g := range playGameDefs {
		b.WriteString(fmt.Sprintf("\n*%d) %s%s* — %s", i+1, prefix, g.Slug, g.Label))
	}
	b.WriteString(fmt.Sprintf("\n\n🎮 *TOTAL GAMES ❮ %d ❯*", len(playGameDefs)))
	b.WriteString("\n\n_Band karna ho to *end* likho._")
	return b.String()
}

func playGameBySlug(slug string) (playGameDef, bool) {
	for _, d := range playGameDefs {
		if d.Slug == slug {
			return d, true
		}
	}
	return playGameDef{}, false
}

func init() {
	// NOTE: the ".game" command itself is registered in games.go (the 1000-game
	// boxed menu). These turn-based commands use their own slugs.
	for _, def := range playGameDefs {
		d := def
		Register(Command{
			Name:     d.Slug,
			Category: "GAME",
			Desc:     d.Desc,
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				startPlayGame(s, info, d, prefix)
			},
		})
	}
}
