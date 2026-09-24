package goldcmds

// ============================================================================
// GOLD-MD — GAME pack (50 text games)
// File: games.go
// ============================================================================
// OWNER ORDER: .menu me ek "GAME" category ho, abhi 50 games ke saath.
///  .game   → games ki list (same fancy boxed menu format)
///  .<game> → us game ka result / next move
//
// Har game ka output bold ** aur 🔰 / ❮ ❯ style me hai — baaki GOLD-MD
// commands jaisa. Sab kuch local hai (koi API nahi), is liye 0% hang risk.
//
// Games ke do type hain:
//   - LUCK games  → seedha result (dice, coin, slot, ...)
//   - PLAY games  → ek arg mangte hain (rps <choice>, scramble <word>, ...)
// ============================================================================

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// gameDef describes one game command.
type gameDef struct {
	Slug  string
	Label string // menu label (uppercased)
	Desc  string // menu / help text
	Play  func(args []string) string
}

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

func gameDice(args []string) string {
	you, bot := gameRand(6)+1, gameRand(6)+1
	verdict := "🤝 DRAW"
	if you > bot {
		verdict = "🏆 YOU WIN"
	} else if you < bot {
		verdict = "😔 YOU LOSE"
	}
	return gheader("DICE ROLL") +
		gline(fmt.Sprintf("YOU GOT ❮ %d ❯", you)) +
		gline(fmt.Sprintf("BOT GOT ❮ %d ❯", bot)) +
		"\n" + verdict
}

func gameCoin(args []string) string {
	face := "HEADS"
	if gameRand(2) == 0 {
		face = "TAILS"
	}
	return gheader("COIN FLIP") + "🪙  *❮ " + face + " ❯*"
}

var gameSlotReels = []string{"🍒", "🍋", "🔔", "💎", "7️⃣", "⭐", "🍀"}

func gameSlot(args []string) string {
	a, b, c := gpick(gameSlotReels), gpick(gameSlotReels), gpick(gameSlotReels)
	verdict := "SAD 😔 TRY AGAIN"
	if a == b && b == c {
		verdict = "JACKPOT 🎉🎉🎉"
	} else if a == b || b == c || a == c {
		verdict = "SMALL WIN 🎉"
	}
	return gheader("SLOT MACHINE") +
		"*❮ " + a + " " + b + " " + c + " ❯*\n\n" + verdict
}

func gameDarts(args []string) string {
	score := []int{0, 1, 5, 10, 15, 20, 25, 50}[gameRand(8)]
	msg := "MISSED 😬"
	if score == 50 {
		msg = "BULLSEYE 🎯🎯"
	} else if score > 0 {
		msg = fmt.Sprintf("SCORE ❮ %d ❯", score)
	}
	return gheader("DARTS") + "🎯 " + msg
}

func gameLudo(args []string) string {
	return gheader("LUDO DICE") +
		fmt.Sprintf("*❮ %d ❯* STEP CHALO 🎲", gameRand(6)+1)
}

var gameWheel = []string{
	"1000 COINS", "500 COINS", "100 COINS", "50 COINS", "10 COINS",
	"TRY AGAIN", "BONUS SPIN", "JACKPOT", "NOTHING", "DOUBLE POINTS",
}

func gameSpin(args []string) string {
	return gheader("SPIN THE WHEEL") + "🎡 *❮ " + gpick(gameWheel) + " ❯*"
}

func gameGuess(args []string) string {
	secret := gameRand(100) + 1
	hint := "TOO LOW"
	if secret > 50 {
		hint = "TOO HIGH"
	}
	return gheader("NUMBER GUESS") +
		gline(fmt.Sprintf("MY NUMBER IS BETWEEN 1 AND 100")) +
		gline("HINT: "+hint) +
		"\n*❮ TRY .GUESS AGAIN ❯*"
}

func gameNumber(args []string) string {
	return gheader("RANDOM NUMBER") + fmt.Sprintf("*❮ %d ❯*", gameRand(1000)+1)
}

var gameAlphabet = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"}

func gameAlphabetPick(args []string) string {
	return gheader("LUCKY LETTER") + "🔤 *❮ " + gpick(gameAlphabet) + " ❯*"
}

var gameEmojis = []string{"😀", "😎", "🥳", "🤩", "😍", "🤖", "👻", "🐯", "🦁", "🐼",
	"🚀", "⚡", "🔥", "🌈", "🍕", "🎈", "💎", "🎯", "🌙", "☀️"}

func gameEmojiPick(args []string) string {
	return gheader("RANDOM EMOJI") + "*❮ " + gpick(gameEmojis) + " ❯*"
}

func gameLucky(args []string) string {
	return gheader("LUCKY NUMBER") + fmt.Sprintf("🍀 *❮ %d ❯*", gameRand(100)+1)
}

var gameCandleOpts = []string{"BURNT OUT 😅", "STILL LIT 🕯️", "BLOWN! 🌬️", "ALMOST 🌬️"}

func gameCandle(args []string) string {
	return gheader("BLOW THE CANDLE") + "🕯️ *❮ " + gpick(gameCandleOpts) + " ❯*"
}

func gameHeadTail(args []string) string {
	face := "HEAD"
	if gameRand(2) == 0 {
		face = "TAIL"
	}
	return gheader("HEAD OR TAIL") + "🪙 *❮ " + face + " ❯*"
}

func gameBingo(args []string) string {
	cols := []string{"B", "I", "N", "G", "O"}
	n := gameRand(75) + 1
	return gheader("BINGO") + fmt.Sprintf("*❮ %s-%d ❯*", cols[(n-1)/15], n)
}

func gameLottery(args []string) string {
	var out []string
	for i := 0; i < 6; i++ {
		out = append(out, fmt.Sprintf("%d", gameRand(49)+1))
	}
	return gheader("LOTTERY NUMBERS") + "🎟️ *❮ " + strings.Join(out, " - ") + " ❯*"
}

var gameRouletteColors = []string{"🔴 RED", "⚫ BLACK", "🟢 GREEN"}

func gameRoulette(args []string) string {
	num := gameRand(37)
	color := gameRouletteColors[gameRand(3)]
	return gheader("ROULETTE") + fmt.Sprintf("*❮ %d %s ❯*", num, color)
}

// ─────────────────────────── CARD / BOARD GAMES ───────────────────────────

var gameRanks = []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
var gameSuits = []string{"♠️", "♥️", "♦️", "♣️"}

func gameCard(args []string) string {
	return gheader("RANDOM CARD") + "*❮ " + gpick(gameRanks) + gpick(gameSuits) + " ❯*"
}

func gameBlackjack(args []string) string {
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
	return gheader("BLACKJACK") +
		gline("YOU: "+strings.Join(yc, " ")+fmt.Sprintf(" (%d)", yt)) +
		gline("BOT: "+strings.Join(bc, " ")+fmt.Sprintf(" (%d)", bt)) +
		"\n" + verdict
}

var gamePokerHands = []string{
	"ROYAL FLUSH 👑", "STRAIGHT FLUSH", "FOUR OF A KIND", "FULL HOUSE",
	"FLUSH", "STRAIGHT", "THREE OF A KIND", "TWO PAIR", "ONE PAIR", "HIGH CARD",
}

func gamePoker(args []string) string {
	return gheader("POKER HAND") + "🃏 *❮ " + gpick(gamePokerHands) + " ❯*"
}

var gameChessOpts = []string{
	"SICILIAN DEFENCE", "QUEEN'S GAMBIT", "LONDON SYSTEM", "KING'S INDIAN",
	"CARO-KANN", "FRENCH DEFENCE", "RUY LOPEZ", "ENGLISH OPENING",
}

func gameChess(args []string) string {
	return gheader("CHESS OPENING") +
		"♟️ *❮ " + gpick(gameChessOpts) + " ❯*\n\n*AB TUMHARI CHAL*"
}

var gameCarromOpts = []string{"SINGLE POCKET 🎯", "DOUBLE POCKET 🎯🎯", "MISS 😬", "COVER ✔️"}

func gameCarrom(args []string) string {
	return gheader("CARROM SHOT") + "*❮ " + gpick(gameCarromOpts) + " ❯*"
}

// ─────────────────────────── SPORTS GAMES ───────────────────────────

var gamePenaltyOpts = []string{"GOAL ⚽🎉", "SAVED 🧤", "MISSED 😬", "POST 🥅", "GOAL ⚽🎉"}

func gamePenalty(args []string) string {
	return gheader("PENALTY SHOOT") + "*❮ " + gpick(gamePenaltyOpts) + " ❯*"
}

var gameWicketOpts = []string{"OUT! 🏏", "NOT OUT ✔️", "SIX! 🎉", "FOUR! 🎉", "WIDE", "NO BALL"}

func gameWicket(args []string) string {
	return gheader("CRICKET SHOT") + "*❮ " + gpick(gameWicketOpts) + " ❯*"
}

func gameSix(args []string) string {
	runs := []int{0, 1, 2, 3, 4, 6}[gameRand(6)]
	verdict := "DOT BALL"
	if runs == 6 {
		verdict = "SIX! 🎉"
	} else if runs == 4 {
		verdict = "FOUR! 🎉"
	} else if runs > 0 {
		verdict = fmt.Sprintf("%d RUN", runs)
	}
	return gheader("CRICKET BALL") + "*❮ " + verdict + " ❯*"
}

// ─────────────────────────── DICE-VARIANT GAMES ───────────────────────────

func gameD20(args []string) string {
	n := gameRand(20) + 1
	verdict := "NORMAL HIT"
	if n == 20 {
		verdict = "CRITICAL HIT 💥"
	} else if n == 1 {
		verdict = "CRITICAL MISS 💀"
	}
	return gheader("D20 ROLL") + fmt.Sprintf("*❮ %d ❯*  %s", n, verdict)
}

func gameDnd(args []string) string {
	stats := []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}
	var b strings.Builder
	b.WriteString(gheader("D&D CHARACTER"))
	for _, s := range stats {
		b.WriteString(gline(fmt.Sprintf("%s ❮ %d ❯", s, gameRand(18)+3)))
	}
	return b.String()
}

func gameFlip5(args []string) string {
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
	return gheader("5 COIN FLIP") +
		"🪙 *❮ " + strings.Join(out, " ") + " ❯*\n\n" +
		fmt.Sprintf("*HEADS ❮ %d ❯ | TAILS ❮ %d ❯*", heads, 5-heads)
}

// ─────────────────────────── ARG GAMES ───────────────────────────

func gameRPS(args []string) string {
	if len(args) == 0 {
		return gneedArg("", "USE .RPS ❮ ROCK / PAPER / SCISSORS ❯")
	}
	user := strings.ToUpper(args[0])
	valid := map[string]bool{"ROCK": true, "PAPER": true, "SCISSORS": true}
	if !valid[user] {
		return gneedArg("", "USE .RPS ❮ ROCK / PAPER / SCISSORS ❯")
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
	return gheader("ROCK PAPER SCISSORS") +
		gline("YOU: "+user) + gline("BOT: "+bot) + "\n" + verdict
}

func gameRPSLS(args []string) string {
	opts := []string{"ROCK", "PAPER", "SCISSORS", "LIZARD", "SPOCK"}
	if len(args) == 0 {
		return gneedArg("", "USE .RPSLS ❮ ROCK / PAPER / SCISSORS / LIZARD / SPOCK ❯")
	}
	user := strings.ToUpper(args[0])
	found := false
	for _, o := range opts {
		if o == user {
			found = true
		}
	}
	if !found {
		return gneedArg("", "USE .RPSLS ❮ ROCK / PAPER / SCISSORS / LIZARD / SPOCK ❯")
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
	return gheader("RPS LIZARD SPOCK") +
		gline("YOU: "+user) + gline("BOT: "+bot) + "\n" + verdict
}

func gameScramble(args []string) string {
	w := strings.Join(args, "")
	if w == "" {
		return gneedArg("", "USE .SCRAMBLE ❮ WORD ❯")
	}
	r := []rune(w)
	rand.Shuffle(len(r), func(i, j int) { r[i], r[j] = r[j], r[i] })
	return gheader("WORD SCRAMBLE") +
		"*❮ " + strings.ToUpper(string(r)) + " ❯*\n\n" +
		"*JAWAB: " + strings.ToUpper(w) + "*"
}

var gameHangWords = []string{"WHATSAPP", "PAKISTAN", "CRICKET", "BIRYANI", "FOOTBALL",
	"MOBILE", "GUITAR", "CHESS", "TEACHER", "SUMMER"}

func gameHangman(args []string) string {
	w := gpick(gameHangWords)
	blanks := strings.Repeat("_ ", len([]rune(w)))
	return gheader("HANGMAN") +
		"*WORD: ❮ " + strings.TrimSpace(blanks) + " ❯*\n\n" +
		fmt.Sprintf("*LETTERS: %d | GUESS KARO!*", len([]rune(w)))
}

var gameMathOps = []string{"+", "-", "*"}

func gameMath(args []string) string {
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
	return gheader("MATH QUIZ") +
		fmt.Sprintf("*❮ %d %s %d = ? ❯*\n\n*JAWAB: %d*", a, op, b, answer)
}

func gameQuiz(args []string) string {
	return gheader("QUIZ QUESTION") + "❓ *❮ " + gpick(gameQuizQuestions) + " ❯*"
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

func gameRiddle(args []string) string {
	return gheader("RIDDLE") + "🧩 *❮ " + gpick(gameRiddles) + " ❯*\n\n*SOCH KE JAWAB DO!*"
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

var gameEmojiQuiz = []string{
	"🍎 + 📱 = ?", "🌧️ + ☀️ = ?", "🐟 + 🍟 = ?", "🦇 + 🧛 = ?",
	"⭐ + 🐟 = ?", "🥶 + 🧊 = ?", "🐝 + 🍯 = ?", "🔥 + 🐦 = ?",
}

func gameEmojiQuizPlay(args []string) string {
	return gheader("EMOJI QUIZ") + "🧩 *❮ " + gpick(gameEmojiQuiz) + " ❯*\n\n*NAAM BATAO!*"
}

var gameFlags = []string{"🇵🇰 PAKISTAN", "🇮🇳 INDIA", "🇸🇦 SAUDI ARABIA", "🇹🇷 TURKEY",
	"🇯🇵 JAPAN", "🇧🇷 BRAZIL", "🇬🇧 UK", "🇺🇸 USA", "🇩🇪 GERMANY", "🇨🇳 CHINA"}

func gameFlag(args []string) string {
	f := gpick(gameFlags)
	emojiOnly := strings.SplitN(f, " ", 2)[0]
	return gheader("GUESS THE FLAG") + "*❮ " + emojiOnly + " ❯*\n\n*KAUN SA MULK HAI?*"
}

var gameCapitals = [][2]string{
	{"PAKISTAN", "ISLAMABAD"}, {"INDIA", "NEW DELHI"}, {"JAPAN", "TOKYO"},
	{"TURKEY", "ANKARA"}, {"SAUDI ARABIA", "RIYADH"}, {"CHINA", "BEIJING"},
	{"FRANCE", "PARIS"}, {"GERMANY", "BERLIN"}, {"EGYPT", "CAIRO"}, {"IRAN", "TEHRAN"},
}

func gameCapital(args []string) string {
	c := gameCapitals[gameRand(len(gameCapitals))]
	return gheader("GUESS THE CAPITAL") +
		fmt.Sprintf("*❮ %s ❯*\n\n*CAPITAL: %s*", c[0], c[1])
}

// ─────────────────────────── PARTY GAMES ───────────────────────────

func gameTruth(args []string) string {
	name := "FRIEND"
	if len(args) > 0 {
		name = strings.ToUpper(strings.Join(args, " "))
	}
	return gheader("TRUTH QUESTION") +
		fmt.Sprintf("*❮ %s ❯*\n\n", name) +
		gameTruths[gameRand(len(gameTruths))]
}

var gameTruths = []string{
	"WHAT IS YOUR BIGGEST FEAR?",
	"WHO IS YOUR SECRET CRUSH?",
	"WHAT IS THE LAST LIE YOU TOLD?",
	"WHAT IS YOUR MOST EMBARRASSING MOMENT?",
	"WHAT IS YOUR BIGGEST WEAKNESS?",
}

func gameDare(args []string) string {
	name := "FRIEND"
	if len(args) > 0 {
		name = strings.ToUpper(strings.Join(args, " "))
	}
	return gheader("DARE CHALLENGE") +
		fmt.Sprintf("*❮ %s ❯*\n\n", name) +
		gameDares[gameRand(len(gameDares))]
}

var gameDares = []string{
	"SEND A VOICE NOTE SINGING YOUR FAVOURITE SONG 🎤",
	"POST YOUR OLDEST SELFIE 📸",
	"TEXT YOUR BEST FRIEND 'I MISS YOU' 💬",
	"DO 20 PUSH UPS 🏋️",
	"SPEAK IN AN ACCENT FOR 10 MINUTES 🗣️",
}

func gameWould(args []string) string {
	p := gameWoulds[gameRand(len(gameWoulds))]
	return gheader("WOULD YOU RATHER") +
		"*A) " + p[0] + "*\n*B) " + p[1] + "*\n\n*❮ A YA B ❯*"
}

var gameWoulds = [][2]string{
	{"BE RICH BUT ALONE", "BE POOR BUT LOVED"},
	{"FLY", "BE INVISIBLE"},
	{"LIVE WITHOUT MUSIC", "LIVE WITHOUT INTERNET"},
	{"KNOW YOUR FUTURE", "CHANGE YOUR PAST"},
	{"ALWAYS BE 10 MINS LATE", "ALWAYS BE 20 MINS EARLY"},
}

func gameNever(args []string) string {
	return gheader("NEVER HAVE I EVER") + "🍻 *❮ " + gpick(gameNevers) + " ❯*"
}

var gameNevers = []string{
	"BROKEN A PHONE SCREEN",
	"LIED ABOUT MY AGE",
	"SUNG IN THE SHOWER",
	"FORGOTTEN SOMEONE'S NAME MID-TALK",
	"STAYED UP ALL NIGHT",
}

func gameParanoia(args []string) string {
	return gheader("PARANOIA") + "😈 *❮ " + gpick(gameParanoias) + " ❯*"
}

var gameParanoias = []string{
	"WHO IS MOST LIKELY TO BECOME FAMOUS?",
	"WHO IS MOST LIKELY TO BE LATE TO THEIR OWN WEDDING?",
	"WHO IS MOST LIKELY TO EAT MY FOOD?",
	"WHO SPENDS THE MOST TIME ON THEIR PHONE?",
	"WHO IS THE BIGGEST DRAMA QUEEN?",
}

func gameTord(args []string) string {
	pick := "TRUTH"
	if gameRand(2) == 0 {
		pick = "DARE"
	}
	if pick == "TRUTH" {
		return gheader("TRUTH OR DARE") + "*❮ TRUTH ❯*\n\n" + goTruthQ()
	}
	return gheader("TRUTH OR DARE") + "*❮ DARE ❯*\n\n" + goDareQ()
}

func goTruthQ() string { return gameTruths[gameRand(len(gameTruths))] }
func goDareQ() string  { return gameDares[gameRand(len(gameDares))] }

func gameBall(args []string) string {
	if len(args) == 0 {
		return gneedArg("", "USE .BALL ❮ YOUR QUESTION ❯")
	}
	return gheader("MAGIC 8 BALL") +
		"🎱 *❮ " + gpick(gameBallAnswers) + " ❯*"
}

var gameBallAnswers = []string{
	"YES DEFIANTLY", "NO CHANCE", "MAYBE LATER", "BILKUL YES",
	"BILKUL NAHI", "POOCH MAT", "100% YES", "SOCH KE BATATA HOON",
	"LUCKY LAG RAHA HAI", "RISKY HAI",
}

var gameClaps = []string{"👏👏👏", "👏👏", "👏", "🤝", "🎉🎉🎉"}

func gameClap(args []string) string {
	return gheader("CLAP COUNTER") + "*❮ " + gpick(gameClaps) + " ❯*"
}

// gameDefs is the ordered list of the 50 GAME commands.
var gameDefs = []gameDef{
	{"rolldice", "ROLLDICE", "ROLL TWO DICE AND PLAY AGAINST THE BOT. JUST TYPE .ROLLDICE.", gameDice},
	{"tosscoin", "TOSSCOIN", "TOSS A COIN AND GET HEADS OR TAILS. JUST TYPE .TOSSCOIN.", gameCoin},
	{"slot", "SLOT", "PLAY THE SLOT MACHINE AND SEE IF YOU HIT THE JACKPOT. JUST TYPE .SLOT.", gameSlot},
	{"darts", "DARTS", "THROW A DART AND GET A RANDOM SCORE. JUST TYPE .DARTS.", gameDarts},
	{"ludo", "LUDO", "ROLL A LUDO DICE AND MOVE YOUR PIECE. JUST TYPE .LUDO.", gameLudo},
	{"spin", "SPIN", "SPIN THE PRIZE WHEEL AND WIN COINS OR BONUS. JUST TYPE .SPIN.", gameSpin},
	{"guess", "GUESS", "GET A NUMBER HINT AND TRY TO GUESS THE SECRET NUMBER. JUST TYPE .GUESS.", gameGuess},
	{"rps", "RPS", "PLAY ROCK PAPER SCISSORS AGAINST THE BOT. USE IT AS .RPS <ROCK / PAPER / SCISSORS>.", gameRPS},
	{"rpsls", "RPSLS", "PLAY ROCK PAPER SCISSORS LIZARD SPOCK AGAINST THE BOT. USE IT AS .RPSLS <CHOICE>.", gameRPSLS},
	{"number", "NUMBER", "GET A RANDOM NUMBER BETWEEN 1 AND 1000. JUST TYPE .NUMBER.", gameNumber},
	{"alphabet", "ALPHABET", "GET A RANDOM LUCKY LETTER. JUST TYPE .ALPHABET.", gameAlphabetPick},
	{"emoji", "EMOJI", "GET A RANDOM EMOJI. JUST TYPE .EMOJI.", gameEmojiPick},
	{"lucky", "LUCKY", "GET YOUR RANDOM LUCKY NUMBER. JUST TYPE .LUCKY.", gameLucky},
	{"candle", "CANDLE", "BLOW THE BIRTHDAY CANDLE AND SEE IF IT GOES OUT. JUST TYPE .CANDLE.", gameCandle},
	{"headtail", "HEADTAIL", "PLAY HEAD OR TAIL WITH THE BOT. JUST TYPE .HEADTAIL.", gameHeadTail},
	{"bingo", "BINGO", "GET A RANDOM BINGO NUMBER FROM B TO O. JUST TYPE .BINGO.", gameBingo},
	{"lottery", "LOTTERY", "GET SIX RANDOM LOTTERY NUMBERS. JUST TYPE .LOTTERY.", gameLottery},
	{"roulette", "ROULETTE", "SPIN THE ROULETTE AND GET A NUMBER WITH COLOUR. JUST TYPE .ROULETTE.", gameRoulette},
	{"card", "CARD", "DRAW A RANDOM PLAYING CARD. JUST TYPE .CARD.", gameCard},
	{"blackjack", "BLACKJACK", "PLAY A QUICK HAND OF BLACKJACK AGAINST THE BOT. JUST TYPE .BLACKJACK.", gameBlackjack},
	{"poker", "POKER", "GET A RANDOM POKER HAND RANK. JUST TYPE .POKER.", gamePoker},
	{"chess", "CHESS", "GET A RANDOM CHESS OPENING TO PLAY. JUST TYPE .CHESS.", gameChess},
	{"carrom", "CARROM", "PLAY A CARROM SHOT AND SEE THE RESULT. JUST TYPE .CARROM.", gameCarrom},
	{"penalty", "PENALTY", "SHOOT A PENALTY AND SEE IF IT IS A GOAL. JUST TYPE .PENALTY.", gamePenalty},
	{"crshot", "CRSHOT", "PLAY A CRICKET SHOT AND SEE OUT, FOUR OR SIX. JUST TYPE .CRSHOT.", gameWicket},
	{"six", "SIX", "PLAY A CRICKET BALL AND SEE HOW MANY RUNS YOU GET. JUST TYPE .SIX.", gameSix},
	{"d20", "D20", "ROLL A 20 SIDED DICE WITH CRITICAL HIT AND MISS. JUST TYPE .D20.", gameD20},
	{"dnd", "DND", "ROLL A RANDOM DUNGEONS AND DRAGONS CHARACTER. JUST TYPE .DND.", gameDnd},
	{"flip5", "FLIP5", "FLIP FIVE COINS AND COUNT HEADS AND TAILS. JUST TYPE .FLIP5.", gameFlip5},
	{"scramble", "SCRAMBLE", "SCRAMBLE ANY WORD INTO A GAME. USE IT AS .SCRAMBLE <WORD>.", gameScramble},
	{"hangman", "HANGMAN", "START A HANGMAN WORD AND GUESS THE LETTERS. JUST TYPE .HANGMAN.", gameHangman},
	{"math", "MATH", "GET A RANDOM MATH QUESTION WITH THE ANSWER. JUST TYPE .MATH.", gameMath},
	{"gquiz", "GQUIZ", "GET A RANDOM QUIZ QUESTION. JUST TYPE .GQUIZ.", gameQuiz},
	{"riddle", "RIDDLE", "GET A RANDOM RIDDLE TO SOLVE. JUST TYPE .RIDDLE.", gameRiddle},
	{"emojiq", "EMOJIQ", "GUESS THE EMOJI COMBINATION QUIZ. JUST TYPE .EMOJIQ.", gameEmojiQuizPlay},
	{"flag", "FLAG", "GUESS THE COUNTRY FROM ITS FLAG. JUST TYPE .FLAG.", gameFlag},
	{"gcapital", "GCAPITAL", "GUESS THE CAPITAL CITY OF A COUNTRY. JUST TYPE .GCAPITAL.", gameCapital},
	{"truth", "TRUTH", "GET A RANDOM TRUTH QUESTION FOR ANY FRIEND. USE IT AS .TRUTH <NAME>.", gameTruth},
	{"dare", "DARE", "GET A RANDOM DARE CHALLENGE FOR ANY FRIEND. USE IT AS .DARE <NAME>.", gameDare},
	{"would", "WOULD", "GET A WOULD YOU RATHER QUESTION WITH TWO OPTIONS. JUST TYPE .WOULD.", gameWould},
	{"never", "NEVER", "PLAY NEVER HAVE I EVER WITH A RANDOM STATEMENT. JUST TYPE .NEVER.", gameNever},
	{"paranoia", "PARANOIA", "GET A RANDOM PARANOIA QUESTION ABOUT YOUR FRIENDS. JUST TYPE .PARANOIA.", gameParanoia},
	{"tord", "TORD", "GET A RANDOM TRUTH OR DARE CHALLENGE. JUST TYPE .TORD.", gameTord},
	{"ball", "BALL", "ASK THE MAGIC 8 BALL ANY YES OR NO QUESTION. USE IT AS .BALL <QUESTION>.", gameBall},
	{"clap", "CLAP", "GET A RANDOM APPLAUSE FOR YOUR FRIEND. JUST TYPE .CLAP.", gameClap},
	{"dice2", "DICE2", "ROLL A SINGLE DICE AND SEE YOUR NUMBER. JUST TYPE .DICE2.", gameDice},
	{"coin3", "COIN3", "FLIP A COIN THREE TIMES FAST. JUST TYPE .COIN3.", gameFlip5},
	{"lucky7", "LUCKY7", "GET A RANDOM LUCKY NUMBER BETWEEN 1 AND 7. JUST TYPE .LUCKY7.", gameLucky},
	{"wheel", "WHEEL", "SPIN THE LUCKY WHEEL AGAIN. JUST TYPE .WHEEL.", gameSpin},
	{"oracle", "ORACLE", "GET A RANDOM ORACLE ANSWER FOR YOUR QUESTION. USE IT AS .ORACLE <QUESTION>.", gameBall},
}

// gameBySlug returns the definition for a slug and whether it exists.
func gameBySlug(slug string) (gameDef, bool) {
	for _, g := range gameDefs {
		if g.Slug == slug {
			return g, true
		}
	}
	return gameDef{}, false
}

// gameSlugFromCommand resolves aliases: .game → gameBySlug, and every command
// named exactly like a game slug dispatches that game.
func gameFromCommand(cmd string) (gameDef, bool) {
	c := strings.ToLower(strings.TrimSpace(cmd))
	return gameBySlug(c)
}

// gameRun is the shared dispatcher for a single game command.
func gameRun(def gameDef) func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		if def.Play == nil {
			return
		}
		s.Reply(info, def.Play(args))
	}
}

// handleGameList shows the full GAME list when .game is typed.
func handleGameList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	var b strings.Builder
	b.WriteString("*🔰 GAME MENU 🔰*\n\n")
	b.WriteString("╔════ ≪ •❈• ≫ ════╗\n")
	b.WriteString("*| 🔰 | GAME | 🔰 |*\n")
	for i, g := range gameDefs {
		b.WriteString(fmt.Sprintf("*| %d | %s%s ❮ %s ❯*\n", i+1, prefix, strings.ToUpper(g.Slug), g.Label))
	}
	b.WriteString("╚════ ≪ •❈• ≫ ════╝\n\n")
	b.WriteString(fmt.Sprintf("*TOTAL GAMES ❮ %d ❯* 🎮", len(gameDefs)))
	s.Reply(info, b.String())
}

func init() {
	Register(Command{
		Name:     "game",
		Category: "GAME",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF ALL GAMES. JUST TYPE .GAME TO SEE EVERY GAME AND ITS COMMAND.",
		Run:      handleGameList,
	})
	for _, g := range gameDefs {
		g := g
		Register(Command{
			Name:     g.Slug,
			Category: "GAME",
			Desc:     g.Desc,
			Run:      gameRun(g),
		})
	}
}
