package goldcmds

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// aimJIDSeq gives every test bridge its own JID so the caches never leak.
var aimJIDSeq int64

// aimBridge is a fake SessionBridge for .aimode tests. It stores the per-bot
// presence settings in a map and records replies.
type aimBridge struct {
	SessionBridge
	settings map[string]string
	replies  []string
	owner    bool
	jid      string
}

func newAimBridge() *aimBridge {
	n := atomic.AddInt64(&aimJIDSeq, 1)
	return &aimBridge{
		settings: map[string]string{},
		owner:    true,
		jid:      fmt.Sprintf("aimbot%d@s.whatsapp.net", n),
	}
}

func (f *aimBridge) GetJID() string { return f.jid }

func (f *aimBridge) GetPresenceSetting(field, def string) string {
	if v, ok := f.settings[field]; ok {
		return v
	}
	return def
}

func (f *aimBridge) SetPresenceSetting(field, val string) { f.settings[field] = val }

func (f *aimBridge) Reply(info types.MessageInfo, text string) { f.replies = append(f.replies, text) }

func (f *aimBridge) NotifyConnectedCard() {}

func (f *aimBridge) IsOwner(info types.MessageInfo) bool { return f.owner }

func aimInfo(group bool) types.MessageInfo {
	chat := types.NewJID("923001234567", types.DefaultUserServer)
	if group {
		chat = types.NewJID("120363000000000000", types.GroupServer)
	}
	return types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: types.NewJID("923001234567", types.DefaultUserServer)},
		ID:            "AIMTEST1",
	}
}

// safeEnv clears any GOLD_API_KEY_N overrides so the tests hit the in-file pool.
func safeEnv(t *testing.T) {
	t.Helper()
	for i := 1; i <= aimMaxKeys; i++ {
		k := fmt.Sprintf("GOLD_API_KEY_%d", i)
		if _, set := os.LookupEnv(k); set {
			os.Unsetenv(k)
		}
	}
}

// ── on/off/status/prefix controls ──────────────────────────────────────────

func TestAIModeToggleAndStatus(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(false)

	if h, _ := AIModeTryHandle(b, info, ".aimode status", "."); !h {
		t.Fatal("aimode status must be handled")
	}
	if !strings.Contains(b.replies[len(b.replies)-1], "*AI MODE :❯ ❮ OFF ❯*") {
		t.Fatalf("initial status should be OFF, got: %s", b.replies[len(b.replies)-1])
	}

	if h, _ := AIModeTryHandle(b, info, ".aimode on", "."); !h {
		t.Fatal("aimode on must be handled")
	}
	if b.settings[aimModeField] != "on" {
		t.Fatalf("mode field = %q, want on", b.settings[aimModeField])
	}
	if !strings.Contains(b.replies[len(b.replies)-1], "*🔰 AI MODE TURNED ON 🔰*") {
		t.Fatalf("unexpected on text: %s", b.replies[len(b.replies)-1])
	}
	if !strings.Contains(b.replies[len(b.replies)-1], "*DESCRIPTION :❵") {
		t.Fatalf("on text missing description row: %s", b.replies[len(b.replies)-1])
	}

	if h, _ := AIModeTryHandle(b, info, ".aimode off", "."); !h {
		t.Fatal("aimode off must be handled")
	}
	if b.settings[aimModeField] != "off" {
		t.Fatalf("mode field = %q, want off", b.settings[aimModeField])
	}
}

func TestAIModePrefixSetAndGuide(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(false)

	if h, _ := AIModeTryHandle(b, info, ".aimode prefix BILAL", "."); !h {
		t.Fatal("aimode prefix must be handled")
	}
	if b.settings[aimPrefixField] != "BILAL" {
		t.Fatalf("prefix field = %q, want BILAL", b.settings[aimPrefixField])
	}
	last := b.replies[len(b.replies)-1]
	if !strings.Contains(last, "*🔰 AI MODE PREFIX SET 🔰*") || !strings.Contains(last, "❮ BILAL ❯") {
		t.Fatalf("prefix set text wrong: %s", last)
	}
	if strings.Contains(last, "👑") {
		t.Fatal("crown emoji must be replaced with 🔰")
	}

	// Bare .aimode → guide text.
	if h, _ := AIModeTryHandle(b, info, ".aimode", "."); !h {
		t.Fatal("bare aimode must be handled")
	}
	guide := b.replies[len(b.replies)-1]
	if !strings.Contains(guide, "*🔰 AI MODE COMMAND GUIDE 🔰*") {
		t.Fatalf("guide missing header: %s", guide)
	}
	if !strings.Contains(guide, "BILAL CHECK BOT SPEED") {
		t.Fatalf("guide should use the stored wake-word: %s", guide)
	}
}

func TestAIModeOwnerOnly(t *testing.T) {
	b := newAimBridge()
	b.owner = false
	if h, _ := AIModeTryHandle(b, aimInfo(false), ".aimode on", "."); !h {
		t.Fatal("non-owner .aimode must still be handled (rejected)")
	}
	if b.settings[aimModeField] == "on" {
		t.Fatal("non-owner must not enable AI mode")
	}
	if b.replies[0] != "*THIS COMMAND IS ONLY FOR ME 😎*" {
		t.Fatalf("owner-only reply must match the bot standard, got: %s", b.replies[0])
	}
}

// ── resolver path (quick-match, no network) ─────────────────────────────────

func TestAIModeQuickMatchExactCommand(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(true) // group, so any group-only gate does not interfere
	b.settings[aimModeField] = "on"

	// "AI ping" = default wake-word + exact command name → quick-match, 0 AI calls.
	h, rewrite := AIModeTryHandle(b, info, "AI ping", ".")
	if !h || rewrite != ".ping" {
		t.Fatalf("quick-match ping: handled=%v rewrite=%q", h, rewrite)
	}
	// Bare "ping" must NOT resolve — the default wake-word gates it, so a plain
	// command name never works without the bot prefix.
	if h, _ := AIModeTryHandle(b, info, "ping", "."); h {
		t.Fatal("bare command name must not resolve when the wake-word is set")
	}
}

func TestAIModeQuickMatchToggle(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(false)
	b.settings[aimModeField] = "on"

	h, rewrite := AIModeTryHandle(b, info, "AI anticall off", ".")
	if !h || rewrite != ".anticall off" {
		t.Fatalf("quick-match toggle: handled=%v rewrite=%q", h, rewrite)
	}
	if rewrite == ".aimode" {
		t.Fatal("must not resolve aimode itself")
	}
}

func TestAIModeIgnoresRealCommands(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(false)
	b.settings[aimModeField] = "on"

	// A message that already carries the bot prefix must never be touched.
	if h, _ := AIModeTryHandle(b, info, ".ping", "."); h {
		t.Fatal("prefixed command must fall through to normal dispatch")
	}
}

func TestAIModeWakeWordGate(t *testing.T) {
	b := newAimBridge()
	info := aimInfo(false)
	b.settings[aimModeField] = "on"
	b.settings[aimPrefixField] = "BILAL"

	// Message that does not start with the wake-word is fully ignored.
	if h, _ := AIModeTryHandle(b, info, "ping", "."); h {
		t.Fatal("message without wake-word must be ignored when a wake-word is set")
	}
	// Message starting with the wake-word is stripped and quick-matched.
	h, rewrite := AIModeTryHandle(b, info, "BILAL ping", ".")
	if !h || rewrite != ".ping" {
		t.Fatalf("wake-word message: handled=%v rewrite=%q", h, rewrite)
	}

	// Fresh bot (no wake-word configured) falls back to the pair.js default
	// "AI" — so bare chatter and bare commands are ignored.
	c := newAimBridge()
	c.settings[aimModeField] = "on"
	if h, _ := AIModeTryHandle(c, info, "menu dikhao", "."); h {
		t.Fatal("default wake-word must gate plain chatter")
	}
	if h, rewrite := AIModeTryHandle(c, info, "AI ping", "."); !h || rewrite != ".ping" {
		t.Fatalf("default AI wake-word: handled=%v rewrite=%q", h, rewrite)
	}
}

// ── registrations / corpus ──────────────────────────────────────────────────

func TestAIModeRegisteredAndOwnerOnly(t *testing.T) {
	if !OwnerOnlySet()["aimode"] {
		t.Fatal("aimode must be registered owner-only")
	}
	found := false
	for _, c := range Commands() {
		if c.Name == "aimode" {
			found = true
			if c.Category != "AI" {
				t.Fatalf("aimode category = %q, want AI", c.Category)
			}
			if c.Hidden {
				t.Fatal("aimode must be visible in the menu")
			}
		}
	}
	if !found {
		t.Fatal("aimode not found in the registry")
	}
}

func TestAIModeCorpusIncludesCoreCommands(t *testing.T) {
	corpus := aimCorpus()
	if len(corpus) == 0 {
		t.Fatal("corpus must not be empty")
	}
	has := map[string]bool{}
	for _, c := range corpus {
		has[c.Command] = true
	}
	for _, want := range []string{"ping", "menu", "alive", "uptime", "aimode"} {
		if !has[want] {
			t.Fatalf("corpus missing core command %q", want)
		}
	}
}

func TestAIModeTextsMatchBotDesign(t *testing.T) {
	b := newAimBridge()
	b.settings[aimModeField] = "on"
	b.settings[aimPrefixField] = "KING"
	info := aimInfo(false)

	AIModeTryHandle(b, info, ".aimode status", ".")
	st := b.replies[len(b.replies)-1]
	for _, want := range []string{"*🔰 AI MODE STATUS 🔰*", "*AI MODE :❯ ❮ ON ❯*", "*WAKE-WORD :❯ ❮ KING ❯*", "*DESCRIPTION :❵"} {
		if !strings.Contains(st, want) {
			t.Fatalf("status text missing %q: %s", want, st)
		}
	}

	AIModeTryHandle(b, info, ".aimode prefix", ".")
	pfx := b.replies[len(b.replies)-1]
	if !strings.Contains(pfx, "*CURRENT WAKE-WORD :❯ ❮ KING ❯*") || !strings.Contains(pfx, "*TYPE ❲ .AIMODE PREFIX NEWWORD ❳*") {
		t.Fatalf("prefix info text wrong: %s", pfx)
	}

	AIModeTryHandle(b, info, ".aimode nonsense", ".")
	bad := b.replies[len(b.replies)-1]
	if !strings.Contains(bad, "*🔰 AI MODE WRONG FORMAT 🔰*") {
		t.Fatalf("wrong-format text wrong: %s", bad)
	}
}

func TestAIModeGroupOnlyGate(t *testing.T) {
	// usergcban in an inbox must be discarded by the hard gate.
	e := aimCorpusEntry{Command: "usergcban"}
	_ = e
	if !aimGroupOnlyCommands["usergcban"] {
		t.Fatal("usergcban must be group-only")
	}
}

func TestAIModeSettingsPanelEntries(t *testing.T) {
	var on, off, pfx *settingsEntry
	for i := range goldSettingsList {
		switch goldSettingsList[i].t {
		case "AI MODE ON":
			on = &goldSettingsList[i]
		case "AI MODE OFF":
			off = &goldSettingsList[i]
		case "AI MODE PREFIX (WAKE-WORD) SET":
			pfx = &goldSettingsList[i]
		}
	}
	if on == nil || off == nil || pfx == nil {
		t.Fatal("AI MODE settings entries missing from the panel")
	}
	if on.c != "aimode on" || off.c != "aimode off" || pfx.c != "aimode prefix" {
		t.Fatalf("AI MODE entries wiring wrong: %q %q %q", on.c, off.c, pfx.c)
	}
	if pfx.ask == "" {
		t.Fatal("prefix entry must ask for a value")
	}
}

func TestAIModeEscapeRegexLiteral(t *testing.T) {
	if got := aimEscapeRegexLiteral("a.b"); got != `a\.b` {
		t.Fatalf("escape = %q, want a\\.b", got)
	}
}

// A stored wake-word that is really a "not set" marker must fall back to the
// default. Regression: the missing sentinel "\x00" (the same value the prefix
// key once leaked) built a gate regex that never matched, so AI Mode silently
// stopped reacting to "AI ..." messages.
func TestAIModeWakeWordSentinelFallsBack(t *testing.T) {
	for _, stored := range []string{"", "  ", "\x00", "null", "NULL"} {
		if got := normalizeAimWakeWord(stored); got != aimDefaultPrefixWord {
			t.Fatalf("normalizeAimWakeWord(%q) = %q, want %q", stored, got, aimDefaultPrefixWord)
		}
	}
	if got := normalizeAimWakeWord("KING"); got != "KING" {
		t.Fatalf("normalizeAimWakeWord(KING) = %q, want KING", got)
	}

	// End-to-end: a session whose stored wake-word is the sentinel must still
	// resolve "AI ping" through the wake-word gate.
	b := newAimBridge()
	b.settings[aimModeField] = "on"
	b.settings[aimPrefixField] = "\x00"
	h, rewrite := AIModeTryHandle(b, aimInfo(false), "AI ping", ".")
	if !h || rewrite != ".ping" {
		t.Fatalf("sentinel wake-word: handled=%v rewrite=%q", h, rewrite)
	}
}

func TestAIModeCleanResolvedStripsNoise(t *testing.T) {
	cases := map[string]string{
		"anticall off**":       "anticall off",
		".ping":                "ping",
		"**play shape of you":  "play shape of you",
		"`menu`":               "menu",
		"\"antilink on\"":      "antilink on",
		"antilink action drop": "antilink action drop",
	}
	for in, want := range cases {
		if got := aimCleanResolved(in); got != want {
			t.Fatalf("aimCleanResolved(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAIModeShortlistCoversIntent(t *testing.T) {
	corpus := aimCorpus()
	cases := map[string]string{
		"bot ki speed check karo":                 "ping",
		"calls auto reject ho rahi hai band karo": "anticall",
		"gana bhejo shape of you":                 "play",
		"facebook se video download karo":         "fb",
		"menu dikhao":                             "menu",
		"status auto seen band karo":              "statusseen",
		"group ko lock kar do":                    "gcbotoff",
	}
	for msg, want := range cases {
		cands := aimCandidates(corpus, msg, 40, 0.8)
		found := false
		for _, c := range cands {
			if c.Command == want {
				found = true
				break
			}
		}
		if !found {
			names := make([]string, 0, len(cands))
			for _, c := range cands {
				names = append(names, c.Command)
			}
			t.Fatalf("shortlist for %q missing %q; got %v", msg, want, names)
		}
	}
}

func TestAIModeCandidatesEmptyForNoise(t *testing.T) {
	corpus := aimCorpus()
	if got := aimCandidates(corpus, "???", 40, 0.8); len(got) != 0 {
		t.Fatalf("punctuation-only message must yield no candidates, got %d", len(got))
	}
}

func TestAIModeKeyAtEnvOverride(t *testing.T) {
	safeEnv(t)
	os.Setenv("GOLD_API_KEY_1", "ENVKEY")
	defer os.Unsetenv("GOLD_API_KEY_1")
	if got := aimKeyAt(0); got != "ENVKEY" {
		t.Fatalf("aimKeyAt(0) = %q, want ENVKEY", got)
	}
	if got := aimKeyAt(1); got == "" || got == "ENVKEY" {
		t.Fatalf("aimKeyAt(1) should fall back to the in-file pool, got %q", got)
	}
}
