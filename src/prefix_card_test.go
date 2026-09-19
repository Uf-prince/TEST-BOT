package main

import (
	"os"
	"strings"
	"testing"
)

// ============================================================================
// CONNECTED-CARD + PREFIX-RESEND — structure tests (owner order):
// 1. Connected msg me OWNER line NAHI (sirf USER/NUMBER/PREFIX/COMMANDS)
// 2. NOTE text 2/3 MINUTES + NO NEED TO PAIR version
// 3. .prefix change → connected card fresh prefix ke sath foran re-send
// ============================================================================

// stripCC: Go source se comments hatao (real code pe scan chale).
func stripCC(src string) string {
	var out []byte
	inLine, inBlock, inStr := false, false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inLine {
			if c == '\n' {
				inLine = false
				out = append(out, c)
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if inStr {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				out = append(out, src[i+1])
				i++
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch {
		case c == '"' && (len(out) == 0 || out[len(out)-1] != '\\'):
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			inLine = true
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			inBlock = true
			i++
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// TestConnectedCardNoOwnerLine: connected card me OWNER line deleted —
// sirf USER / NUMBER / PREFIX / COMMANDS lines.
func TestConnectedCardNoOwnerLine(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal("manager.go missing:", err)
	}
	src := string(b)

	// Startup card template (fmt.Sprintf block) hi scan karo.
	i := strings.Index(src, "*GOLD-MD HAS BEEN STARTED*")
	if i < 0 {
		t.Fatal("connected card template missing")
	}
	end := strings.Index(src[i:], "`,\n")
	if end < 0 {
		end = 700
	}
	card := src[i : i+end]

	if strings.Contains(card, "OWNER :") {
		t.Error("connected card me ab bhi OWNER line hai — delete hona chahiye tha")
	}
	for _, want := range []string{
		"USER :", "NUMBER :", "PREFIX :", "COMMANDS :",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("connected card me missing line: %q", want)
		}
	}

	// Sprintf args me s.Owner nahi hona chahiye (placeholder delete ho gaya).
	j := strings.Index(src, "GOLD-MD HAS BEEN STARTED")
	args := src[j : j+900]
	if strings.Contains(args, "s.Owner, ownerName") {
		t.Error("sprintf args me s.Owner abhi bhi hai — OWNER placeholder leak")
	}
}

// TestConnectedCardCountsLogo: startup card COMMANDS total must include the
// 1000 hidden logo commands (same as .menu).
func TestConnectedCardCountsLogo(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal("manager.go missing:", err)
	}
	src := string(b)
	if !strings.Contains(src, "totalCmds := coreCount + pluginCount + goldcmds.LogoCount") {
		t.Error("startup card totalCmds me goldcmds.LogoCount add nahi hua")
	}
}

// TestConnectedCardNoteText: NOTE me 2/3 minutes + no-need-to-pair text.
func TestConnectedCardNoteText(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal("manager.go missing:", err)
	}
	src := string(b)

	for _, want := range []string{
		"2 /3  MINUTES",
		"NO NEED TO PAIR",
		"GOLD-MD HAS BEEN STARTED",
		"IMPORTANT NOTE",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("connected card me missing: %q", want)
		}
	}
	// Purana 30 SECONDS text wapas nahi aa sakta.
	if strings.Contains(src, "ONLY 30 SECONDS AND YOUR BOT WILL BE BACK ONLINE IN 30 SECONDS") {
		t.Error("purana 30 SECONDS note text abhi bhi hai")
	}
}

// TestPrefixResendBridge: bridge me NotifyPrefixChanged hai (interface +
// implementation) aur dono prefix-change paths usko call karte hain.
func TestPrefixResendBridge(t *testing.T) {
	// Interface (gold-cmds/api.go).
	b, err := os.ReadFile("../gold-cmds/api.go")
	if err != nil {
		t.Fatal("gold-cmds/api.go missing:", err)
	}
	if !strings.Contains(string(b), "NotifyPrefixChanged()") {
		t.Error("SessionBridge interface me NotifyPrefixChanged missing")
	}

	// Implementation (commands_loader.go).
	b, err = os.ReadFile("commands_loader.go")
	if err != nil {
		t.Fatal("commands_loader.go missing:", err)
	}
	impl := string(b)
	if !strings.Contains(impl, "func (b *bridge) NotifyPrefixChanged()") {
		t.Error("bridge implementation NotifyPrefixChanged missing")
	}
	if !strings.Contains(impl, "sendStartupNotification()") {
		t.Error("NotifyPrefixChanged sendStartupNotification call nahi karta")
	}

	// Prefix change paths (gold-cmds/prefix.go).
	b, err = os.ReadFile("../gold-cmds/prefix.go")
	if err != nil {
		t.Fatal("gold-cmds/prefix.go missing:", err)
	}
	pfx := string(b)
	n := strings.Count(pfx, "s.NotifyPrefixChanged()")
	if n < 2 {
		t.Errorf("prefix.go me NotifyPrefixChanged calls %d — dono paths (null + new) me hone chahiye", n)
	}

	// fakeBridge embed karke interface satisfied — naye method se toot nahi sakta.
}
