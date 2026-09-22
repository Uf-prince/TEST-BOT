package goldcmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// onBridge is a fake SessionBridge for ownernumber / sudo tests.
type onBridge struct {
	SessionBridge
	replies   []string
	owner     bool
	perm      string
	mainOwner string
	sudo      []string
}

func newONBridge() *onBridge {
	return &onBridge{
		owner: true,
		perm:  "923000000000",
	}
}

func (f *onBridge) IsOwner(info types.MessageInfo) bool { return f.owner }

func (f *onBridge) Reply(info types.MessageInfo, text string) { f.replies = append(f.replies, text) }

func (f *onBridge) GetPermanentOwnerNumber() string { return f.perm }

func (f *onBridge) GetOwnerNumberSetting(def string) string {
	if f.mainOwner != "" {
		return f.mainOwner
	}
	return def
}

func (f *onBridge) SetOwnerNumberSetting(number string) { f.mainOwner = number }

func (f *onBridge) GetSudoOwners() []string { return f.sudo }

func (f *onBridge) SetSudoOwners(numbers []string) { f.sudo = numbers }

func (f *onBridge) lastReply() string {
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

// TestSudoListEmpty — .sudo list with no sudo owners.
func TestSudoListEmpty(t *testing.T) {
	b := newONBridge()
	handleOwnerNumber(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "SUDO OWNERS LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "NO SUDO OWNERS") {
		t.Errorf("reply = %q, want NO SUDO OWNERS", r)
	}
}

// TestSudoListShowsOwners — .sudo list shows all sudo owners + count.
func TestSudoListShowsOwners(t *testing.T) {
	b := newONBridge()
	handleOwnerNumber(b, types.MessageInfo{}, []string{"add", "923111111111,923222222222"}, ".")
	handleOwnerNumber(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if !strings.Contains(r, "SUDO OWNERS LIST") {
		t.Errorf("reply missing header: %q", r)
	}
	if !strings.Contains(r, "923111111111") || !strings.Contains(r, "923222222222") {
		t.Errorf("reply missing sudo numbers: %q", r)
	}
	if !strings.Contains(r, "TOTAL SUDO") {
		t.Errorf("reply missing total: %q", r)
	}
	if !strings.Contains(r, "923000000000") {
		t.Errorf("reply missing main owner: %q", r)
	}
}

// TestSudoListAfterDel — deleted sudo owner no longer appears.
func TestSudoListAfterDel(t *testing.T) {
	b := newONBridge()
	handleOwnerNumber(b, types.MessageInfo{}, []string{"add", "923111111111,923222222222"}, ".")
	handleOwnerNumber(b, types.MessageInfo{}, []string{"del", "923111111111"}, ".")
	handleOwnerNumber(b, types.MessageInfo{}, []string{"list"}, ".")
	r := b.lastReply()
	if strings.Contains(r, "923111111111") {
		t.Errorf("deleted sudo still listed: %q", r)
	}
	if !strings.Contains(r, "923222222222") {
		t.Errorf("remaining sudo missing: %q", r)
	}
}

// TestSudoListNonOwner — non-owner cannot use .sudo list.
func TestSudoListNonOwner(t *testing.T) {
	b := newONBridge()
	b.owner = false
	handleOwnerNumber(b, types.MessageInfo{}, []string{"list"}, ".")
	if !strings.Contains(b.lastReply(), "ONLY FOR ME") {
		t.Errorf("reply = %q, want owner-only refusal", b.lastReply())
	}
}

// TestOwnerNumberInfoMentionsList — the no-arg info block mentions SUDO LIST.
func TestOwnerNumberInfoMentionsList(t *testing.T) {
	b := newONBridge()
	handleOwnerNumber(b, types.MessageInfo{}, nil, ".")
	if !strings.Contains(b.lastReply(), "SUDO LIST") {
		t.Errorf("info block missing SUDO LIST mention: %q", b.lastReply())
	}
}
