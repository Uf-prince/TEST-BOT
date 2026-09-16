package goldcmds

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// clJIDSeq gives every test bridge a unique JID so the 1-minute caches
// (cmdreact/cmdname/cmdowner) never leak state between tests.
var clJIDSeq int64

// clBridge is a fake SessionBridge for the cmdreact/cmdname/cmdowner list tests.
type clBridge struct {
	SessionBridge
	settings map[string]string
	replies  []string
	owner    bool
	jid      string
}

func newCLBridge() *clBridge {
	n := atomic.AddInt64(&clJIDSeq, 1)
	return &clBridge{
		settings: map[string]string{},
		owner:    true,
		jid:      fmt.Sprintf("clbot%d@s.whatsapp.net", n),
	}
}

func (f *clBridge) GetJID() string { return f.jid }

func (f *clBridge) GetStatusSetting(field, def string) string {
	if v, ok := f.settings[field]; ok {
		return v
	}
	return def
}

func (f *clBridge) SetStatusSetting(field, val string) { f.settings[field] = val }

func (f *clBridge) DelStatusSetting(field string) { delete(f.settings, field) }

func (f *clBridge) Reply(info types.MessageInfo, text string) { f.replies = append(f.replies, text) }

func (f *clBridge) IsOwner(info types.MessageInfo) bool { return f.owner }

func (f *clBridge) lastReply() string {
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

// ── .cmdreact list ──

func TestCmdReactListEmpty(t *testing.T) {
	b := newCLBridge()
	handleCmdReactAsync(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "CMDREACT LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "NO PER-COMMAND EMOJIS") {
		t.Errorf("reply = %q, want NO PER-COMMAND EMOJIS", r)
	}
}

func TestCmdReactListShowsPerCommand(t *testing.T) {
	b := newCLBridge()
	handleCmdReactAsync(b, types.MessageInfo{}, []string{"ping", "😘"}, ".")
	handleCmdReactAsync(b, types.MessageInfo{}, []string{"menu", "👑"}, ".")
	handleCmdReactAsync(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "CMDREACT LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "ping") || !strings.Contains(r, "menu") {
		t.Errorf("reply missing commands: %q", r)
	}
	if !strings.Contains(r, "TOTAL") {
		t.Errorf("reply missing total: %q", r)
	}
}

func TestCmdReactListMentionsInInfo(t *testing.T) {
	b := newCLBridge()
	handleCmdReactAsync(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "CMDREACT LIST") {
		t.Errorf("info block missing CMDREACT LIST mention: %q", b.lastReply())
	}
}

// ── .cmdname list ──

func TestCmdNameListEmpty(t *testing.T) {
	b := newCLBridge()
	handleCmdName(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "CMDNAME LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "NO CUSTOM NAMES") {
		t.Errorf("reply = %q, want NO CUSTOM NAMES", r)
	}
}

func TestCmdNameListShowsRenames(t *testing.T) {
	b := newCLBridge()
	handleCmdName(b, types.MessageInfo{}, []string{"ping", "to", "umar"}, ".")
	handleCmdName(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "CMDNAME LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "ping") || !strings.Contains(r, "umar") {
		t.Errorf("reply missing rename pair: %q", r)
	}
	if !strings.Contains(r, "TOTAL") {
		t.Errorf("reply missing total: %q", r)
	}
}

func TestCmdNameListMentionsInInfo(t *testing.T) {
	b := newCLBridge()
	handleCmdName(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "CMDNAME LIST") {
		t.Errorf("info block missing CMDNAME LIST mention: %q", b.lastReply())
	}
}

// ── .cmdpublicowner list ──

func TestCmdPublicOwnerListEmpty(t *testing.T) {
	b := newCLBridge()
	handleCmdOwnerPublic(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "OWNER-ONLY LIST IS EMPTY") {
		t.Errorf("reply = %q, want OWNER-ONLY LIST IS EMPTY", r)
	}
}

func TestCmdPublicOwnerListShowsOwners(t *testing.T) {
	b := newCLBridge()
	handleCmdOwner(b, types.MessageInfo{}, []string{"ping"}, ".")
	handleCmdOwnerPublic(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "OWNER-ONLY COMMANDS") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "ping") {
		t.Errorf("reply missing owner-only command: %q", r)
	}
}

func TestCmdPublicOwnerRegistered(t *testing.T) {
	found := false
	for _, c := range Commands() {
		if c.Name == "cmdpublicowner" {
			found = true
			if !c.Hidden {
				t.Errorf("cmdpublicowner must be Hidden (not in menu/count)")
			}
			if !c.OwnerOnly {
				t.Errorf("cmdpublicowner must be OwnerOnly")
			}
		}
	}
	if !found {
		t.Fatalf("cmdpublicowner not registered")
	}
}

func TestCmdChangeGuideMentionsList(t *testing.T) {
	g := cmdChangeGuide(".")
	if !strings.Contains(g, "cmdpublicowner list") {
		t.Errorf("guide missing cmdpublicowner list mention: %q", g)
	}
}
