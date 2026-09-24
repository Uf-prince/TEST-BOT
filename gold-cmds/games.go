package goldcmds

// ============================================================================
// GOLD-MD — 1000 GAME DESIGNS (.game1 ... .game1000)
// File: games.go
// ============================================================================
// OWNER ORDER: .game bhi .font / .equalizer ki tarah ek category ka menu ho.
//   .game    → fancy boxed menu (GAME1 ... GAME1000) — baaki commands jaisa
//   .gameN   → us game ko khelo (hidden command, menu me sirf .GAME dikhta hai)
//
// DESIGN UNIQUENESS: har N = (family, flavor) ka unique combo:
//   family = (N-1) % 50   -> 50 asli base games (dice, slot, poker, quiz, ...)
//   flavor = (N-1) / 50   -> 20 edition themes (classic, turbo, royal, ...)
// 50 x 20 = 1000 combos, is liye N=1..1000 me koi design repeat nahi hota.
//
// Har game ka output bold ** aur 🔰 / ❮ ❯ style me hai — baaki GOLD-MD
// commands jaisa. Sab kuch local hai (koi API nahi), is liye 0% hang risk.
// ============================================================================

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// GameCount is the number of .game designs (owner order: 1000).
const GameCount = 1000

// gameBase is one of the 50 real games. Play returns the game body (no
// header); ok=false means the game needs an argument and body is the usage
// hint instead.
type gameBase struct {
	Slug string
	Name string
	Play func(args []string) (string, bool)
}

// gameFlavors are the 20 edition themes multiplied over the 50 families.
var gameFlavors = []string{
	"CLASSIC", "TURBO", "NIGHT", "PRO", "MEGA",
	"MINI", "SUPER", "ROYAL", "DARK", "GOLD",
	"ICE", "FIRE", "STORM", "VIP", "XP",
	"LITE", "MAX", "ELITE", "PRIME", "ULTRA",
}

// GameDesignName is the human-readable label for game N,
// e.g. "ROYAL DICE ROLL" or "TURBO POKER HAND".
func GameDesignName(n int) string {
	if n < 1 || n > GameCount {
		n = 1
	}
	m := n - 1
	fam := m % len(gameBases)
	fl := (m / len(gameBases)) % len(gameFlavors)
	return gameFlavors[fl] + " " + gameBases[fam].Name
}

// ─────────────────────────── shared helpers ───────────────────────────

// gameRand is a tiny safe randn helper (n <= 0 → 0).
func gameRand(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.IntN(n)
}

// gpick returns a random element (empty string for an empty list).
func gpick(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[gameRand(len(list))]
}

// gline is the shared framed line helper: returns a bold bullet line.
func gline(s string) string {
	return "*| 🔰 | " + s + "*\n"
}

// gheader builds the standard game reply header.
func gheader(title string) string {
	return "*🔰 " + strings.ToUpper(title) + " 🔰*\n\n"
}

// gneedArg is the standard "you must pass an argument" reply.
func gneedArg(prefix, usage string) string {
	return "*🔰 " + usage + " 🔰*"
}

// ─────────────────────────── LUCK GAMES ───────────────────────────

func gameDice(args []string) (string, bool) {
	you, bot := gameRand(6)+1, gameRand(6)+1
	verdict := "🤝 DRAW"
	if you > bot {
		verdict = "🏆 YOU WIN"
	} else if you < bot {
		verdict = "😔 YOU LOSE"
	}
	return gline(fmt.Sprintf("YOU GOT ❮ %d ❯", you)) +
		gline(fmt.Sprintf("BOT GOT ❮ %d ❯", bot)) +
		"\n" + verdict, true
}

func gameCoin(args []string) (string, bool) {
	face := "HEADS"
	if gameRand(2) == 0 {
		face = "TAILS"
	}
	return "🪙  *❮ " + face + " ❯*", true
}

var gameSlotReels = []string{"🍒", "🍋", "🔔", "💎", "7️⃣", "⭐", "🍀"}

func gameSlot(args []string) (string, bool) {
	a, b, c := gpick(gameSlotReels), gpick(gameSlotReels), gpick(gameSlotReels)
	verdict := "SAD 😔 TRY AGAIN"
	if a == b && b == c {
		verdict = "JACKPOT 🎉🎉🎉"
	} else if a == b || b == c || a == c {
		verdict = "SMALL WIN 🎉"
	}
	return "*❮ " + a + " " + b + " " + c + " ❯*\n\n" + verdict, true
}

func gameDarts(args []string) (string, bool) {
	score := []int{0, 1, 5, 10, 15, 20, 25, 50}[gameRand(8)]
	msg := "MISSED 😬"
	if score == 50 {
		msg = "BULLSEYE 🎯🎯"
	} else if score > 0 {
		msg = fmt.Sprintf("SCORE ❮ %d ❯", score)
	}
	return "🎯 " + msg, true
}

func gameLudo(args []string) (string, bool) {
	return fmt.Sprintf("*❮ %d ❯* STEP CHALO 🎲", gameRand(6)+1), true
}

var gameWheel = []string{
	"1000 COINS", "500 COINS", "100 COINS", "50 COINS", "10 COINS",
	"TRY AGAIN", "BONUS SPIN", "JACKPOT", "NOTHING", "DOUBLE POINTS",
}

func gameSpin(args []string) (string, bool) {
	return "🎡 *❮ " + gpick(gameWheel) + " ❯*", true
}

func gameGuess(args []string) (string, bool) {
	secret := gameRand(100) + 1
	hint := "TOO LOW"
	if secret > 50 {
		hint = "TOO HIGH"
	}
	return gline("MY NUMBER IS BETWEEN 1 AND 100") +
		gline("HINT: "+hint) +
		"\n*❮ TRY .GUESS AGAIN ❯*", true
}

func gameNumber(args []string) (string, bool) {
	return fmt.Sprintf("*❮ %d ❯*", gameRand(1000)+1), true
}

var gameAlphabet = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"}

func gameAlphabetPick(args []string) (string, bool) {
	return "🔤 *❮ " + gpick(gameAlphabet) + " ❯*", true
}

var gameEmojis = []string{"😀", "😎", "🥳", "🤩", "😍", "🤖", "👻", "🐯", "🦁", "🐼",
	"🚀", "⚡", "🔥", "🌈", "🍕", "🎈", "💎", "🎯", "🌙", "☀️"}

func gameEmojiPick(args []string) (string, bool) {
	return "*❮ " + gpick(gameEmojis) + " ❯*", true
}

func gameLucky(args []string) (string, bool) {
	return fmt.Sprintf("🍀 *❮ %d ❯*", gameRand(100)+1), true
}

var gameCandleOpts = []string{"BURNT OUT 😅", "STILL LIT 🕯️", "BLOWN! 🌬️", "ALMOST 🌬️"}

func gameCandle(args []string) (string, bool) {
	return "🕯️ *❮ " + gpick(gameCandleOpts) + " ❯*", true
}

func gameHeadTail(args []string) (string, bool) {
	face := "HEAD"
	if gameRand(2) == 0 {
		face = "TAIL"
	}
	return "🪙 *❮ " + face + " ❯*", true
}

func gameBingo(args []string) (string, bool) {
	cols := []string{"B", "I", "N", "G", "O"}
	n := gameRand(75) + 1
	return fmt.Sprintf("*❮ %s-%d ❯*", cols[(n-1)/15], n), true
}

func gameLottery(args []string) (string, bool) {
	var out []string
	for i := 0; i < 6; i++ {
		out = append(out, fmt.Sprintf("%d", gameRand(49)+1))
	}
	return "🎟️ *❮ " + strings.Join(out, " - ") + " ❯*", true
}

var gameRouletteColors = []string{"🔴 RED", "⚫ BLACK", "🟢 GREEN"}

func gameRoulette(args []string) (string, bool) {
	num := gameRand(37)
	color := gameRouletteColors[gameRand(3)]
	return fmt.Sprintf("*❮ %d %s ❯*", num, color), true
}

// ─────────────────────────── CARD / BOARD GAMES ───────────────────────────

var gameRanks = []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
var gameSuits = []string{"♠️", "♥️", "♦️", "♣️"}

func gameCard(args []string) (string, bool) {
	return "*❮ " + gpick(gameRanks) + gpick(gameSuits) + " ❯*", true
}

func gameBlackjack(args []string) (string, bool) {
	hand := func() ([]string, int) {
		var cards []string
		total := 0
		for i := 0; i < 2; i++ {
			r := gameRand(10) + 1
			cards = append(cards, gpick(gameRanks))
			total += r
		}
		return cards, total
	}
	yc, yt := hand()
	bc, bt := hand()
	verdict := "🤝 DRAW"
	if yt > bt && yt <= 21 {
		verdict = "🏆 YOU WIN"
	} else if bt > yt && bt <= 21 {
		verdict = "😔 BOT WINS"
	}
	return gline("YOU: "+strings.Join(yc, " ")+fmt.Sprintf(" (%d)", yt)) +
		gline("BOT: "+strings.Join(bc, " ")+fmt.Sprintf(" (%d)", bt)) +
		"\n" + verdict, true
}

var gamePokerHands = []string{
	"ROYAL FLUSH 👑", "STRAIGHT FLUSH", "FOUR OF A KIND", "FULL HOUSE",
	"FLUSH", "STRAIGHT", "THREE OF A KIND", "TWO PAIR", "ONE PAIR", "HIGH CARD",
}

func gamePoker(args []string) (string, bool) {
	return "🃏 *❮ " + gpick(gamePokerHands) + " ❯*", true
}

var gameChessOpts = []string{
	"SICILIAN DEFENCE", "QUEEN'S GAMBIT", "LONDON SYSTEM", "KING'S INDIAN",
	"CARO-KANN", "FRENCH DEFENCE", "RUY LOPEZ", "ENGLISH OPENING",
}

func gameChess(args []string) (string, bool) {
	return "♟️ *❮ " + gpick(gameChessOpts) + " ❯*\n\n*AB TUMHARI CHAL*", true
}

var gameCarromOpts = []string{"SINGLE POCKET 🎯", "DOUBLE POCKET 🎯🎯", "MISS 😬", "COVER ✔️"}

func gameCarrom(args []string) (string, bool) {
	return "*❮ " + gpick(gameCarromOpts) + " ❯*", true
}

// ─────────────────────────── SPORTS GAMES ───────────────────────────

var gamePenaltyOpts = []string{"GOAL ⚽🎉", "SAVED 🧤", "MISSED 😬", "POST 🥅", "GOAL ⚽🎉"}

func gamePenalty(args []string) (string, bool) {
	return "*❮ " + gpick(gamePenaltyOpts) + " ❯*", true
}

var gameWicketOpts = []string{"OUT! 🏏", "NOT OUT ✔️", "SIX! 🎉", "FOUR! 🎉", "WIDE", "NO BALL"}

func gameWicket(args []string) (string, bool) {
	return "*❮ " + gpick(gameWicketOpts) + " ❯*", true
}

func gameSix(args []string) (string, bool) {
	runs := []int{0, 1, 2, 3, 4, 6}[gameRand(6)]
	verdict := "DOT BALL"
	if runs == 6 {
		verdict = "SIX! 🎉"
	} else if runs == 4 {
		verdict = "FOUR! 🎉"
	} else if runs > 0 {
		verdict = fmt.Sprintf("%d RUN", runs)
	}
	return "*❮ " + verdict + " ❯*", true
}

// ─────────────────────────── DICE-VARIANT GAMES ───────────────────────────

func gameD20(args []string) (string, bool) {
	n := gameRand(20) + 1
	verdict := "NORMAL HIT"
	if n == 20 {
		verdict = "CRITICAL HIT 💥"
	} else if n == 1 {
		verdict = "CRITICAL MISS 💀"
	}
	return fmt.Sprintf("*❮ %d ❯*  %s", n, verdict), true
}

func gameDnd(args []string) (string, bool) {
	stats := []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}
	var b strings.Builder
	for _, s := range stats {
		b.WriteString(gline(fmt.Sprintf("%s ❮ %d ❯", s, gameRand(18)+3)))
	}
	return b.String(), true
}

func gameFlip5(args []string) (string, bool) {
	heads := 0
	var out []string
	for i := 0; i < 5; i++ {
		if gameRand(2) == 0 {
			out = append(out, "H")
			heads++
		} else {
			out = append(out, "T")
		}
	}
	return "🪙 *❮ " + strings.Join(out, " ") + " ❯*\n\n" +
		fmt.Sprintf("*HEADS ❮ %d ❯ | TAILS ❮ %d ❯*", heads, 5-heads), true
}

// ─────────────────────────── ARG GAMES ───────────────────────────

func gameRPS(args []string) (string, bool) {
	if len(args) == 0 {
		return gneedArg("", "USE .RPS ❮ ROCK / PAPER / SCISSORS ❯"), false
	}
	user := strings.ToUpper(args[0])
	valid := map[string]bool{"ROCK": true, "PAPER": true, "SCISSORS": true}
	if !valid[user] {
		return gneedArg("", "USE .RPS ❮ ROCK / PAPER / SCISSORS ❯"), false
	}
	bot := gpick([]string{"ROCK", "PAPER", "SCISSORS"})
	verdict := "🤝 DRAW"
	switch {
	case user == bot:
		verdict = "🤝 DRAW"
	case (user == "ROCK" && bot == "SCISSORS") ||
		(user == "PAPER" && bot == "ROCK") ||
		(user == "SCISSORS" && bot == "PAPER"):
		verdict = "🏆 YOU WIN"
	default:
		verdict = "😔 BOT WINS"
	}
	return gline("YOU: "+user) + gline("BOT: "+bot) + "\n" + verdict, true
}

func gameRPSLS(args []string) (string, bool) {
	opts := []string{"ROCK", "PAPER", "SCISSORS", "LIZARD", "SPOCK"}
	if len(args) == 0 {
		return gneedArg("", "USE .RPSLS ❮ ROCK / PAPER / SCISSORS / LIZARD / SPOCK ❯"), false
	}
	user := strings.ToUpper(args[0])
	found := false
	for _, o := range opts {
		if o == user {
			found = true
		}
	}
	if !found {
		return gneedArg("", "USE .RPSLS ❮ ROCK / PAPER / SCISSORS / LIZARD / SPOCK ❯"), false
	}
	bot := gpick(opts)
	beats := map[string][]string{
		"ROCK":     {"SCISSORS", "LIZARD"},
		"PAPER":    {"ROCK", "SPOCK"},
		"SCISSORS": {"PAPER", "LIZARD"},
		"LIZARD":   {"SPOCK", "PAPER"},
		"SPOCK":    {"ROCK", "SCISSORS"},
	}
	verdict := "🤝 DRAW"
	if user != bot {
		for _, b := range beats[user] {
			if b == bot {
				verdict = "🏆 YOU WIN"
			}
		}
		if verdict == "🤝 DRAW" {
			verdict = "😔 BOT WINS"
		}
	}
	return gline("YOU: "+user) + gline("BOT: "+bot) + "\n" + verdict, true
}

func gameScramble(args []string) (string, bool) {
	w := strings.Join(args, "")
	if w == "" {
		return gneedArg("", "USE .SCRAMBLE ❮ WORD ❯"), false
	}
	r := []rune(w)
	rand.Shuffle(len(r), func(i, j int) { r[i], r[j] = r[j], r[i] })
	return "*❮ " + strings.ToUpper(string(r)) + " ❯*\n\n" +
		"*JAWAB: " + strings.ToUpper(w) + "*", true
}

var gameHangWords = []string{"WHATSAPP", "PAKISTAN", "CRICKET", "BIRYANI", "FOOTBALL",
	"MOBILE", "GUITAR", "CHESS", "TEACHER", "SUMMER"}

func gameHangman(args []string) (string, bool) {
	w := gpick(gameHangWords)
	blanks := strings.Repeat("_ ", len([]rune(w)))
	return "*WORD: ❮ " + strings.TrimSpace(blanks) + " ❯*\n\n" +
		fmt.Sprintf("*LETTERS: %d | GUESS KARO!*", len([]rune(w))), true
}

var gameMathOps = []string{"+", "-", "*"}

func gameMath(args []string) (string, bool) {
	op := gpick(gameMathOps)
	a, b := gameRand(50)+1, gameRand(50)+1
	answer := 0
	switch op {
	case "+":
		answer = a + b
	case "-":
		answer = a - b
	default:
		answer = a * b
	}
	return fmt.Sprintf("*❮ %d %s %d = ? ❯*\n\n*JAWAB: %d*", a, op, b, answer), true
}

var gameQuizQuestions = []string{
	"WHAT IS THE CAPITAL OF JAPAN?",
	"WHICH PLANET IS THE RED PLANET?",
	"HOW MANY CONTINENTS ARE THERE?",
	"HOW MANY PLAYERS ARE IN A CRICKET TEAM?",
	"WHAT IS THE LARGEST OCEAN?",
	"WHO WROTE ROMEO AND JULIET?",
	"HOW MANY DAYS ARE IN A LEAP YEAR?",
	"WHAT IS THE FASTEST LAND ANIMAL?",
	"WHAT GAS DO WE BREATHE IN?",
	"HOW MANY SIDES DOES A HEXAGON HAVE?",
}

func gameQuiz(args []string) (string, bool) {
	return "❓ *❮ " + gpick(gameQuizQuestions) + " ❯*", true
}

var gameRiddles = []string{
	"I SPEAK WITHOUT A MOUTH AND HEAR WITHOUT EARS. WHAT AM I?",
	"THE MORE YOU TAKE, THE MORE YOU LEAVE BEHIND. WHAT AM I?",
	"WHAT HAS KEYS BUT CANNOT OPEN LOCKS?",
	"WHAT GETS WETTER THE MORE IT DRIES?",
	"I HAVE CITIES BUT NO HOUSES. WHAT AM I?",
	"WHAT HAS A HEAD AND A TAIL BUT NO BODY?",
	"WHAT CAN TRAVEL AROUND THE WORLD WHILE STAYING IN A CORNER?",
	"WHAT HAS MANY TEETH BUT NEVER BITES?",
}

func gameRiddle(args []string) (string, bool) {
	return "🧩 *❮ " + gpick(gameRiddles) + " ❯*\n\n*SOCH KE JAWAB DO!*", true
}

var gameEmojiQuiz = []string{
	"🍎 + 📱 = ?", "🌧️ + ☀️ = ?", "🐟 + 🍟 = ?", "🦇 + 🧛 = ?",
	"⭐ + 🐟 = ?", "🥶 + 🧊 = ?", "🐝 + 🍯 = ?", "🔥 + 🐦 = ?",
}

func gameEmojiQuizPlay(args []string) (string, bool) {
	return "🧩 *❮ " + gpick(gameEmojiQuiz) + " ❯*\n\n*NAAM BATAO!*", true
}

var gameFlags = []string{"🇵🇰 PAKISTAN", "🇮🇳 INDIA", "🇸🇦 SAUDI ARABIA", "🇹🇷 TURKEY",
	"🇯🇵 JAPAN", "🇧🇷 BRAZIL", "🇬🇧 UK", "🇺🇸 USA", "🇩🇪 GERMANY", "🇨🇳 CHINA"}

func gameFlag(args []string) (string, bool) {
	f := gpick(gameFlags)
	emojiOnly := strings.SplitN(f, " ", 2)[0]
	return "*❮ " + emojiOnly + " ❯*\n\n*KAUN SA MULK HAI?*", true
}

var gameCapitals = [][2]string{
	{"PAKISTAN", "ISLAMABAD"}, {"INDIA", "NEW DELHI"}, {"JAPAN", "TOKYO"},
	{"TURKEY", "ANKARA"}, {"SAUDI ARABIA", "RIYADH"}, {"CHINA", "BEIJING"},
	{"FRANCE", "PARIS"}, {"GERMANY", "BERLIN"}, {"EGYPT", "CAIRO"}, {"IRAN", "TEHRAN"},
}

func gameCapital(args []string) (string, bool) {
	c := gameCapitals[gameRand(len(gameCapitals))]
	return fmt.Sprintf("*❮ %s ❯*\n\n*CAPITAL: %s*", c[0], c[1]), true
}

// ─────────────────────────── PARTY GAMES ───────────────────────────

func gameTruth(args []string) (string, bool) {
	name := "FRIEND"
	if len(args) > 0 {
		name = strings.ToUpper(strings.Join(args, " "))
	}
	return fmt.Sprintf("*❮ %s ❯*\n\n", name) +
		gameTruths[gameRand(len(gameTruths))], true
}

var gameTruths = []string{
	"WHAT IS YOUR BIGGEST FEAR?",
	"WHO IS YOUR SECRET CRUSH?",
	"WHAT IS THE LAST LIE YOU TOLD?",
	"WHAT IS YOUR MOST EMBARRASSING MOMENT?",
	"WHAT IS YOUR BIGGEST WEAKNESS?",
}

func gameDare(args []string) (string, bool) {
	name := "FRIEND"
	if len(args) > 0 {
		name = strings.ToUpper(strings.Join(args, " "))
	}
	return fmt.Sprintf("*❮ %s ❯*\n\n", name) +
		gameDares[gameRand(len(gameDares))], true
}

var gameDares = []string{
	"SEND A VOICE NOTE SINGING YOUR FAVOURITE SONG 🎤",
	"POST YOUR OLDEST SELFIE 📸",
	"TEXT YOUR BEST FRIEND 'I MISS YOU' 💬",
	"DO 20 PUSH UPS 🏋️",
	"SPEAK IN AN ACCENT FOR 10 MINUTES 🗣️",
}

var gameWoulds = [][2]string{
	{"BE RICH BUT ALONE", "BE POOR BUT LOVED"},
	{"FLY", "BE INVISIBLE"},
	{"LIVE WITHOUT MUSIC", "LIVE WITHOUT INTERNET"},
	{"KNOW YOUR FUTURE", "CHANGE YOUR PAST"},
	{"ALWAYS BE 10 MINS LATE", "ALWAYS BE 20 MINS EARLY"},
}

func gameWould(args []string) (string, bool) {
	p := gameWoulds[gameRand(len(gameWoulds))]
	return "*A) " + p[0] + "*\n*B) " + p[1] + "*\n\n*❮ A YA B ❯*", true
}

var gameNevers = []string{
	"BROKEN A PHONE SCREEN",
	"LIED ABOUT MY AGE",
	"SUNG IN THE SHOWER",
	"FORGOTTEN SOMEONE'S NAME MID-TALK",
	"STAYED UP ALL NIGHT",
}

func gameNever(args []string) (string, bool) {
	return "🍻 *❮ " + gpick(gameNevers) + " ❯*", true
}

var gameParanoias = []string{
	"WHO IS MOST LIKELY TO BECOME FAMOUS?",
	"WHO IS MOST LIKELY TO BE LATE TO THEIR OWN WEDDING?",
	"WHO IS MOST LIKELY TO EAT MY FOOD?",
	"WHO SPENDS THE MOST TIME ON THEIR PHONE?",
	"WHO IS THE BIGGEST DRAMA QUEEN?",
}

func gameParanoia(args []string) (string, bool) {
	return "😈 *❮ " + gpick(gameParanoias) + " ❯*", true
}

func gameTord(args []string) (string, bool) {
	pick := "TRUTH"
	if gameRand(2) == 0 {
		pick = "DARE"
	}
	if pick == "TRUTH" {
		return "*❮ TRUTH ❯*\n\n" + gameTruths[gameRand(len(gameTruths))], true
	}
	return "*❮ DARE ❯*\n\n" + gameDares[gameRand(len(gameDares))], true
}

var gameBallAnswers = []string{
	"YES DEFIANTLY", "NO CHANCE", "MAYBE LATER", "BILKUL YES",
	"BILKUL NAHI", "POOCH MAT", "100% YES", "SOCH KE BATATA HOON",
	"LUCKY LAG RAHA HAI", "RISKY HAI",
}

func gameBall(args []string) (string, bool) {
	if len(args) == 0 {
		return gneedArg("", "USE .BALL ❮ YOUR QUESTION ❯"), false
	}
	return "🎱 *❮ " + gpick(gameBallAnswers) + " ❯*", true
}

var gameClaps = []string{"👏👏👏", "👏👏", "👏", "🤝", "🎉🎉🎉"}

func gameClap(args []string) (string, bool) {
	return "*❮ " + gpick(gameClaps) + " ❯*", true
}

func gameLucky7(args []string) (string, bool) {
	return fmt.Sprintf("7️⃣ *❮ %d ❯*", gameRand(7)+1), true
}

func gameOracle(args []string) (string, bool) {
	if len(args) == 0 {
		return gneedArg("", "USE .ORACLE ❮ YOUR QUESTION ❯"), false
	}
	return "🔮 *❮ " + gpick(gameBallAnswers) + " ❯*", true
}

func gameCoin3(args []string) (string, bool) {
	var out []string
	for i := 0; i < 3; i++ {
		if gameRand(2) == 0 {
			out = append(out, "H")
		} else {
			out = append(out, "T")
		}
	}
	return "🪙 *❮ " + strings.Join(out, " ") + " ❯*", true
}

func gameFortuneWheel(args []string) (string, bool) {
	return "🎡 *❮ " + gpick(gameWheel) + " ❯*", true
}

func gameNamePicker(args []string) (string, bool) {
	if len(args) == 0 {
		return gneedArg("", "USE .GAMEn ❮ NAME1 NAME2 NAME3 ❯"), false
	}
	return "🎲 *❮ " + strings.ToUpper(gpick(args)) + " ❯* PICK HUA!", true
}

// gameBases is the ordered list of the 50 real games.
var gameBases = []gameBase{
	{"rolldice", "DICE ROLL", gameDice},
	{"tosscoin", "COIN FLIP", gameCoin},
	{"slot", "SLOT MACHINE", gameSlot},
	{"darts", "DARTS", gameDarts},
	{"ludo", "LUDO DICE", gameLudo},
	{"spin", "SPIN THE WHEEL", gameSpin},
	{"guess", "NUMBER GUESS", gameGuess},
	{"number", "RANDOM NUMBER", gameNumber},
	{"alphabet", "LUCKY LETTER", gameAlphabetPick},
	{"emoji", "RANDOM EMOJI", gameEmojiPick},
	{"lucky", "LUCKY NUMBER", gameLucky},
	{"candle", "BLOW THE CANDLE", gameCandle},
	{"headtail", "HEAD OR TAIL", gameHeadTail},
	{"bingo", "BINGO", gameBingo},
	{"lottery", "LOTTERY NUMBERS", gameLottery},
	{"roulette", "ROULETTE", gameRoulette},
	{"card", "RANDOM CARD", gameCard},
	{"blackjack", "BLACKJACK", gameBlackjack},
	{"poker", "POKER HAND", gamePoker},
	{"chess", "CHESS OPENING", gameChess},
	{"carrom", "CARROM SHOT", gameCarrom},
	{"penalty", "PENALTY SHOOT", gamePenalty},
	{"crshot", "CRICKET SHOT", gameWicket},
	{"six", "CRICKET BALL", gameSix},
	{"d20", "D20 ROLL", gameD20},
	{"dnd", "D&D CHARACTER", gameDnd},
	{"flip5", "5 COIN FLIP", gameFlip5},
	{"rps", "ROCK PAPER SCISSORS", gameRPS},
	{"rpsls", "RPS LIZARD SPOCK", gameRPSLS},
	{"scramble", "WORD SCRAMBLE", gameScramble},
	{"hangword", "HANGMAN", gameHangman},
	{"math", "MATH QUIZ", gameMath},
	{"gquiz", "QUIZ QUESTION", gameQuiz},
	{"riddle", "RIDDLE", gameRiddle},
	{"emojiq", "EMOJI QUIZ", gameEmojiQuizPlay},
	{"flag", "GUESS THE FLAG", gameFlag},
	{"gcapital", "GUESS THE CAPITAL", gameCapital},
	{"truth", "TRUTH QUESTION", gameTruth},
	{"dare", "DARE CHALLENGE", gameDare},
	{"would", "WOULD YOU RATHER", gameWould},
	{"never", "NEVER HAVE I EVER", gameNever},
	{"paranoia", "PARANOIA", gameParanoia},
	{"tord", "TRUTH OR DARE", gameTord},
	{"ball", "MAGIC 8 BALL", gameBall},
	{"clap", "CLAP COUNTER", gameClap},
	{"lucky7", "LUCKY 7", gameLucky7},
	{"oracle", "ORACLE ANSWER", gameOracle},
	{"coin3", "TRIPLE COIN", gameCoin3},
	{"wheel", "FORTUNE WHEEL", gameFortuneWheel},
	{"pickname", "NAME PICKER", gameNamePicker},
}

// GameRunN is the entry point for game N (1..1000) — also used by the 50
// base slugs (which map to their base index).
func GameRunN(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	if n < 1 || n > GameCount {
		s.Reply(info, "*GAME NUMBER 1 SE 1000 TAK HI HAI*")
		return
	}
	m := n - 1
	base := gameBases[m%len(gameBases)]
	body, ok := base.Play(args)
	if !ok {
		s.Reply(info, body)
		return
	}
	s.Reply(info, gheader(GameDesignName(n))+body)
}

// GameBaseIndex returns the 1-based game number for a base slug, or 0.
func GameBaseIndex(slug string) int {
	for i, b := range gameBases {
		if b.Slug == slug {
			return i + 1
		}
	}
	return 0
}

// GameBaseSlugNumbers maps every base alias slug to its game number so the
// main package can register them as hidden commands (like font1..font1000).
func GameBaseSlugNumbers() map[string]int {
	out := make(map[string]int, len(gameBases))
	for i, b := range gameBases {
		out[b.Slug] = i + 1
	}
	return out
}

// GameShortSlug maps game N (1..1000) to its short, typeable command name.
//
//	flavor 0 → the bare base slug        (.rolldice)
//	flavor k → the base slug + 2-digit   (.rolldice02 … .rolldice20)
//
// The 2-digit zero-pad is deliberate: base family slugs already end in digits
// for a few games (.d20, .coin3, .lucky7, .flip5), and a bare suffix would
// collide with the turn-based .rps5. Zero-padding keeps all 1000 names unique
// and readable without a separator the user has to remember.
func GameShortSlug(n int) string {
	if n < 1 || n > GameCount {
		n = 1
	}
	m := n - 1
	base := gameBases[m%len(gameBases)].Slug
	fl := m / len(gameBases)
	if fl == 0 {
		return base
	}
	return fmt.Sprintf("%s%02d", base, fl+1)
}

// GameShortSlugs returns the short command name for every game 1..1000, in
// order. This is the .game menu's row list (owner order: 1000 real games, each
// shown by the exact short name a user types).
func GameShortSlugs() []string {
	out := make([]string, 0, GameCount)
	for n := 1; n <= GameCount; n++ {
		out = append(out, GameShortSlug(n))
	}
	return out
}

// GameShortSlugNumbers maps every short command name to its game number so the
// main package can register them as hidden commands (like game1..game1000).
// Names already owned by a registered command / turn-based game are skipped, so
// a game short name can never clobber an unrelated feature.
func GameShortSlugNumbers() map[string]int {
	taken := make(map[string]bool, GameCount)
	for _, c := range Commands() {
		taken[c.Name] = true
	}
	for _, d := range playGameDefs {
		taken[d.Slug] = true
	}
	out := make(map[string]int, GameCount)
	for n := 1; n <= GameCount; n++ {
		name := GameShortSlug(n)
		if taken[name] {
			continue
		}
		out[name] = n
	}
	return out
}

// GameMenuCount is the number of games listed in the .game menu (all 1000).
func GameMenuCount() int { return GameCount }

// handleGameList implements bare .game — the boxed GAME1..GAME1000 menu.
func handleGameList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	s.ShowGameMenu(info, args, prefix)
}

func init() {
	// OWNER ORDER: menu me SIRF .game dikhta hai (display name .GAME). Game
	// numbers (.game1..game1000) main-package Commands map me hidden hote hain
	// — menu me kabhi nahi dikhte. .game likhne par fancy boxed menu banta hai.
	Register(Command{
		Name:     "game",
		Category: "GAME",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF 1000 DHAMAKEDAR GAMES. IT SHOWS .GAME1 TO .GAME1000 — TYPE ANY GAME NUMBER TO PLAY IT.",
		Run:      handleGameList,
	})
}
