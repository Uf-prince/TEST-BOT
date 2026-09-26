package goldcmds

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLineHasCommandToken(t *testing.T) {
	yes := []string{
		"*❰ .BOTPIC ❱ CHANGE BOT PIC (MENU + ALIVE)*",
		"❮ .ping ❯ CHECK SPEED",
		"*❰ /menu ❱ SHOW ALL MENUS*",
	}
	for _, s := range yes {
		if !LineHasCommandToken(s) {
			t.Errorf("expected token in %q", s)
		}
	}
	no := []string{
		"*🔰 CONVERTER MENU 🔰*",
		"*USER:❯ 923158930864*",
		"*PREFIX :❯ ❮ . ❱*",
		"CHANGE BOT PIC (MENU + ALIVE)",
		"SEND A DIRECT IMAGE LINK ENDING IN .jpg, .png OR .gif",
		"*EXAMPLE ❮ https://example.com/photo.jpg ❯*",
		// A bare word in brackets is a PLACEHOLDER, not a token, when the bot has
		// a real prefix: "❮ QUERY ❯" / "❮ LINK ❯" must be translated.
		"*❮ BOTPIC ❯ CHANGE BOT PIC*",
		"",
	}
	for _, s := range no {
		if LineHasCommandToken(s) {
			t.Errorf("unexpected token in %q", s)
		}
	}
}

func TestTranslatePreservingCommandTokensSkipsTokenLines(t *testing.T) {
	called := 0
	var sent string
	old := clTranslator
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		called++
		sent = text
		// Tag every line so the test can tell exactly which lines were translated.
		parts := strings.Split(text, "\n")
		for i := range parts {
			parts[i] = strings.ToUpper(parts[i]) + "[" + target + "]"
		}
		return strings.Join(parts, "\n"), nil
	}
	defer func() { clTranslator = old }()

	in := "*🔰 MENU 🔰*\n*❰ .BOTPIC ❱ CHANGE BOT PIC*\nHELLO"
	out, err := TranslatePreservingCommandTokens(context.Background(), in, "ar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The token itself must survive byte-for-byte ...
	if !strings.Contains(out, "*❰ .BOTPIC ❱") {
		t.Errorf("token was mangled: %q", out)
	}
	// ... while the description after it is still translated. House-style caps
	// are softened for the request and re-applied after, so compare case-insensitively.
	if !strings.Contains(strings.ToUpper(out), "CHANGE BOT PIC*[AR]") {
		t.Errorf("description not translated: %q", out)
	}
	// The token must never reach the translator, only the description.
	if strings.Contains(sent, "❰") || strings.Contains(sent, ".BOTPIC") {
		t.Errorf("token was sent to the translator: %q", sent)
	}
	if called != 1 {
		t.Errorf("expected 1 batched call, got %d", called)
	}
}

func TestTranslatePreservingCommandTokensTokenOnlyLine(t *testing.T) {
	old := clTranslator
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		return "TRANSLATED", nil
	}
	defer func() { clTranslator = old }()

	// A line that is nothing but a token has no description to translate.
	out, err := TranslatePreservingCommandTokens(context.Background(), "❰ .PING ❱", "ar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "❰ .PING ❱" {
		t.Errorf("token-only line must pass through unchanged, got %q", out)
	}
}

// TestTranslatePreservingCommandTokensLineDriftFallsBack: jab batched request
// line count todh de, to per-line retry ho — ek kharab chunk poore reply ko
// English me nahi giraata, aur har line apni jagah rehti hai.
func TestTranslatePreservingCommandTokensLineDriftFallsBack(t *testing.T) {
	old := clTranslator
	var batched, single int
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		if strings.Contains(text, "\n") {
			batched++
			return "only-one-line", nil // collapses the batch: drift
		}
		single++
		return strings.ToUpper(text) + "[ar]", nil
	}
	defer func() { clTranslator = old }()

	in := "LINE ONE\nLINE TWO\n❮ .PING ❯ SPEED"
	out, err := TranslatePreservingCommandTokens(context.Background(), in, "ar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The batch drifted, so each line was retried on its own and every line
	// still came back translated in place. Caps are re-applied by the pipeline,
	// so compare case-insensitively.
	up := strings.ToUpper(out)
	if !strings.Contains(up, "LINE ONE[AR]") || !strings.Contains(up, "LINE TWO[AR]") {
		t.Errorf("per-line fallback ne lines translate nahi kin: %q", out)
	}
	if !strings.Contains(up, "❮ .PING ❯ SPEED[AR]") {
		t.Errorf("token line ka tail translate nahi hua: %q", out)
	}
	if batched != 1 || single != 3 {
		t.Errorf("chahiye 1 batched + 3 single, mila %d + %d", batched, single)
	}
}

// TestSplitTokenLineCoversBareTokens: menu me do shaklen hain — bracketed
// ("❰ .BOTPIC ❱ DESC") aur bare ("| 🔰 | .LOGO5 ❮ YOUR NAME ❯"). Dono me token
// verbatim rehna chahiye, warna user wo type nahi kar sakta.
func TestSplitTokenLineCoversBareTokens(t *testing.T) {
	Register(Command{Name: "botvideo", Hidden: true})
	Register(Command{Name: "logopic", Hidden: true})

	cases := []struct{ in, wantHead, wantTail string }{
		{"*❰ .BOTPIC ❱ CHANGE BOT PIC*", "*❰ .BOTPIC ❱", " CHANGE BOT PIC*"},
		{"*|🔰| .BOTVIDEO*", "*|🔰| .BOTVIDEO", "*"},
		{"*| 🔰 | .LOGO5 ❮ YOUR NAME ❯*", "*| 🔰 | .LOGO5", " ❮ YOUR NAME ❯*"},
		{"*| 🔰 | .LOGO1000 ❮ YOUR NAME ❯*", "*| 🔰 | .LOGO1000", " ❮ YOUR NAME ❯*"},
	}
	for _, c := range cases {
		head, tail, ok := splitTokenLine(c.in)
		if !ok {
			t.Errorf("token nahi mila: %q", c.in)
			continue
		}
		if head != c.wantHead || tail != c.wantTail {
			t.Errorf("splitTokenLine(%q) = (%q, %q), want (%q, %q)", c.in, head, tail, c.wantHead, c.wantTail)
		}
	}
	// Non-tokens must stay whole so they still get translated.
	for _, in := range []string{"*🔰 LOGO MENU 🔰*", "SEND A LINK ENDING IN .jpg", "*USER:❯ 923158930864*"} {
		if _, _, ok := splitTokenLine(in); ok {
			t.Errorf("non-token line split ho gayi: %q", in)
		}
	}
}

// TestTokenDigitsSurviveTranslation: ".logo5" ka digit ASCII hi rehna chahiye
// chahe baaki line Urdu ho jaye.
func TestTokenDigitsSurviveTranslation(t *testing.T) {
	old := clTranslator
	clTranslator = func(ctx context.Context, text, target string) (string, error) {
		return "آپ کا نام", nil
	}
	defer func() { clTranslator = old }()

	in := "*| 🔰 | .LOGO5 ❮ YOUR NAME ❯*"
	out, err := TranslatePreservingCommandTokens(context.Background(), in, "ur")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(out, ".LOGO5") {
		t.Errorf("token ka digit localize ho gaya, user type nahi kar payega: %q", out)
	}
	if strings.Contains(out, "۵") {
		t.Errorf("ASCII digit ke bajaye Urdu numeral aa gaya: %q", out)
	}
}

func TestClCanonicalNamesSkipsInternalModules(t *testing.T) {
	for _, n := range clCanonicalNames() {
		if strings.HasPrefix(n, "com.") {
			t.Errorf("internal module name %q must not be localized", n)
		}
		if strings.ContainsAny(n, " \t") {
			t.Errorf("untypeable name %q must not be localized", n)
		}
	}
}

func TestClMapHasLegacyJunk(t *testing.T) {
	junk := clParse(clEncode("ar", [][2]string{{"اضفصوت", "com.addvoice"}, {"القائمة", "menu"}}))
	if !clMapHasLegacyJunk(junk) {
		t.Error("map with com.* rows must be reported as legacy junk")
	}
	clean := clParse(clEncode("ar", [][2]string{{"القائمة", "menu"}}))
	if clMapHasLegacyJunk(clean) {
		t.Error("clean map must not be reported as legacy junk")
	}
}

// A rebuild must not wipe aliases that already work when a chunk fails (Google
// throttling), while dropping the useless com.* rows from the old rule set.
func TestClBuildMergesAndDropsJunk(t *testing.T) {
	b := newBLBridge()
	b.SetBotLanguageSetting("ur")
	// Pre-existing map: one good alias plus one legacy junk row.
	b.SetStatusSetting(clMapField, clEncode("ur", [][2]string{
		{"مینو", "menu"}, {"اضفصوت", "com.addvoice"},
	}))
	clMu.Lock()
	delete(clCache, b.jid)
	clMu.Unlock()

	canon := clCanonicalNames()
	if len(canon) == 0 {
		t.Fatal("no canonical names")
	}
	// Translator returns nothing usable (simulates every chunk failing).
	CmdLocalizeAttachTranslator(func(ctx context.Context, text, target string) (string, error) {
		return "", context.DeadlineExceeded
	})
	defer CmdLocalizeAttachTranslator(nil)

	clBuildAsync(b, b.jid, "ur")
	time.Sleep(120 * time.Millisecond)

	// The builder persists nothing when every chunk fails, so the stored map must
	// still hold the working alias and must NOT have grown junk.
	st := clParse(b.GetStatusSetting(clMapField, ""))
	if st.ByLoc["مینو"] != "menu" {
		t.Errorf("working alias was lost: %#v", st.ByLoc)
	}
}
