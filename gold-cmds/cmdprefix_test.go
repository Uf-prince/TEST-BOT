package goldcmds

import (
	"os"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestMain mirrors main.go: it attaches the full dispatchable command-name
// set (gold-cmds registry + core commands like ping/menu/alive) so that
// cmdname/cmdprefix validation sees the same names the real bot does.
func TestMain(m *testing.M) {
	CmdNameAttachKnownCommands(func() []string {
		return []string{"ping", "menu", "alive", "uptime", "sessions"}
	})
	os.Exit(m.Run())
}

// cpBridge is a fake SessionBridge for cmdprefix tests. It stores status
// settings in a map and records replies.
type cpBridge struct {
	SessionBridge
	settings map[string]string
	replies  []string
	owner    bool
	jid      string
}

func newCPBridge() *cpBridge {
	return &cpBridge{settings: map[string]string{}, owner: true, jid: "bot@s.whatsapp.net"}
}

func (f *cpBridge) GetJID() string { return f.jid }

func (f *cpBridge) GetStatusSetting(field, def string) string {
	if v, ok := f.settings[field]; ok {
		return v
	}
	return def
}

func (f *cpBridge) SetStatusSetting(field, val string) { f.settings[field] = val }

func (f *cpBridge) Reply(info types.MessageInfo, text string) { f.replies = append(f.replies, text) }

func (f *cpBridge) IsOwner(info types.MessageInfo) bool { return f.owner }

func (f *cpBridge) lastReply() string {
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

// TestCmdPrefixRegistered verifies the command is in the registry.
func TestCmdPrefixRegistered(t *testing.T) {
	found := false
	for _, c := range Commands() {
		if c.Name == "cmdprefix" {
			found = true
			if !c.OwnerOnly {
				t.Errorf("cmdprefix must be OwnerOnly")
			}
			if c.Category != "OWNER & SYSTEM" {
				t.Errorf("cmdprefix category = %q, want OWNER & SYSTEM", c.Category)
			}
		}
	}
	if !found {
		t.Fatalf("cmdprefix not registered")
	}
}

// TestCmdPrefixStopSingle — .cmdprefix ping stop makes ping prefixless.
func TestCmdPrefixStopSingle(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")
	if !cpLoad(b)["ping"] {
		t.Fatalf("ping not marked prefixless; settings=%v", b.settings)
	}
	if !strings.Contains(b.lastReply(), "STOPPED") {
		t.Errorf("reply missing STOPPED: %q", b.lastReply())
	}
}

// TestCmdPrefixStopMultiple — comma-separated list.
func TestCmdPrefixStopMultiple(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping,menu,alive", "stop"}, ".")
	st := cpLoad(b)
	for _, n := range []string{"ping", "menu", "alive"} {
		if !st[n] {
			t.Errorf("%s not marked prefixless", n)
		}
	}
}

// TestCmdPrefixStartSingle — start removes it again.
func TestCmdPrefixStartSingle(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "start"}, ".")
	if cpLoad(b)["ping"] {
		t.Fatalf("ping still prefixless after start")
	}
	if !strings.Contains(b.lastReply(), "STARTED") {
		t.Errorf("reply missing STARTED: %q", b.lastReply())
	}
}

// TestCmdPrefixStartMultiple — multiple at once.
func TestCmdPrefixStartMultiple(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"alive,menu,ping", "stop"}, ".")
	handleCmdPrefix(b, types.MessageInfo{}, []string{"alive,menu,ping", "start"}, ".")
	if len(cpLoad(b)) != 0 {
		t.Fatalf("stopped set not empty after start: %v", cpLoad(b))
	}
}

// TestCmdPrefixActionFirst — "stop ping" (action first) also works.
func TestCmdPrefixActionFirst(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"stop", "ping"}, ".")
	if !cpLoad(b)["ping"] {
		t.Fatalf("action-first form failed")
	}
}

// TestCmdPrefixReset — reset clears everything.
func TestCmdPrefixReset(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping,menu", "stop"}, ".")
	handleCmdPrefix(b, types.MessageInfo{}, []string{"reset"}, ".")
	if len(cpLoad(b)) != 0 {
		t.Fatalf("reset did not clear: %v", cpLoad(b))
	}
}

// TestCmdPrefixUnknownCommand — unknown names are reported, not stored.
func TestCmdPrefixUnknownCommand(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"notacommand", "stop"}, ".")
	if cpLoad(b)["notacommand"] {
		t.Fatalf("unknown command was stored")
	}
	if !strings.Contains(b.lastReply(), "NOT FOUND") {
		t.Errorf("reply missing NOT FOUND: %q", b.lastReply())
	}
}

// TestCmdPrefixNonOwner — non-owner gets the owner-only reply.
func TestCmdPrefixNonOwner(t *testing.T) {
	b := newCPBridge()
	b.owner = false
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")
	if !strings.Contains(b.lastReply(), "ONLY FOR ME") {
		t.Errorf("non-owner reply = %q, want ONLY FOR ME", b.lastReply())
	}
	if len(cpLoad(b)) != 0 {
		t.Fatalf("non-owner changed state")
	}
}

// TestCmdPrefixRewrite — the handler hook rewrites a bare stopped command.
func TestCmdPrefixRewrite(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")

	// bare "ping" → rewritten to ".ping"
	got, ok := CmdPrefixRewrite(b, "ping", ".")
	if !ok || got != ".ping" {
		t.Fatalf("CmdPrefixRewrite(ping) = %q,%v want .ping,true", got, ok)
	}
	// bare "ping hello" → ".ping hello"
	got, ok = CmdPrefixRewrite(b, "ping hello", ".")
	if !ok || got != ".ping hello" {
		t.Fatalf("CmdPrefixRewrite(ping hello) = %q,%v want .ping hello,true", got, ok)
	}
	// already prefixed → unchanged
	got, ok = CmdPrefixRewrite(b, ".ping", ".")
	if ok || got != ".ping" {
		t.Fatalf("CmdPrefixRewrite(.ping) = %q,%v want .ping,false", got, ok)
	}
	// a non-stopped command → unchanged
	got, ok = CmdPrefixRewrite(b, "menu", ".")
	if ok || got != "menu" {
		t.Fatalf("CmdPrefixRewrite(menu) = %q,%v want menu,false", got, ok)
	}
}

// TestCmdPrefixRewriteAfterStart — after start, bare command no longer rewrites.
func TestCmdPrefixRewriteAfterStart(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "start"}, ".")
	if _, ok := CmdPrefixRewrite(b, "ping", "."); ok {
		t.Fatalf("bare ping still rewritten after start")
	}
}

// TestCmdPrefixRewriteNoPrefixMode — when bot prefix is empty, no-op.
func TestCmdPrefixRewriteNoPrefixMode(t *testing.T) {
	b := newCPBridge()
	handleCmdPrefix(b, types.MessageInfo{}, []string{"ping", "stop"}, ".")
	if _, ok := CmdPrefixRewrite(b, "ping", ""); ok {
		t.Fatalf("rewrite should be no-op when prefix is empty")
	}
}
