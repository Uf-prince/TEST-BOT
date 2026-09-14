package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestFleetOwnerCommandLIDPN — .host5gb / .svrchange simple BOT-OWNER
// commands hain (owner order: "dusre cmnds jese") — isFleetOwnerCommand
// handler.go ke owner-check ke SAARE layers check karta hai:
// IsFromMe / bot-JID / bare-number / cfg owner set / sudo owners /
// SenderAlt (LID chat).
func TestFleetOwnerCommandLIDPN(t *testing.T) {
	// JID builder helpers
	pn := func(num string) types.JID {
		j, _ := types.ParseJID(num + "@s.whatsapp.net")
		return j
	}
	lid := func(num string) types.JID {
		j, _ := types.ParseJID(num + "@lid")
		return j
	}

	// 1) bot owner ke APNE device se (self-chat) → ALLOW
	s := &Session{JID: "923480974696:1@s.whatsapp.net"}
	info := types.MessageInfo{}
	info.Sender = pn("923480974696")
	info.IsFromMe = true
	if !isFleetOwnerCommand(s, info) {
		t.Errorf("IsFromMe (bot owner device) → expected ALLOW, got DENY")
	}

	// 2) sender bot ka hi exact JID → ALLOW
	info2 := types.MessageInfo{}
	info2.Sender = pn("923480974696")
	if !isFleetOwnerCommand(s, info2) {
		t.Errorf("sender == bot JID → expected ALLOW, got DENY")
	}

	// 3) sender bot ka hi number, dusre device suffix → ALLOW
	info3 := types.MessageInfo{}
	info3.Sender, _ = types.ParseJID("923480974696:5@s.whatsapp.net")
	if !isFleetOwnerCommand(s, info3) {
		t.Errorf("bot number + different device suffix → expected ALLOW, got DENY")
	}

	// 4) RANDOM user (not owner) → DENY
	info4 := types.MessageInfo{}
	info4.Sender = pn("92111222333")
	if isFleetOwnerCommand(s, info4) {
		t.Errorf("random user → expected DENY, got ALLOW")
	}

	// 5) LID chat: sender LID-form + SenderAlt non-owner phone → DENY
	info5 := types.MessageInfo{}
	info5.Sender = lid("55555555")
	info5.SenderAlt = pn("92111222333")
	if isFleetOwnerCommand(s, info5) {
		t.Errorf("LID chat non-owner alt → expected DENY, got ALLOW")
	}

	// 6) LID chat: SenderAlt = bot ka hi JID → ALLOW
	info6 := types.MessageInfo{}
	info6.Sender = lid("987654321098")
	info6.SenderAlt = pn("923480974696")
	if !isFleetOwnerCommand(s, info6) {
		t.Errorf("LID chat + SenderAlt = bot JID → expected ALLOW, got DENY")
	}

	// 7) nil session safe: IsFromMe hi sufficient → ALLOW
	info7 := types.MessageInfo{}
	info7.Sender = pn("923480974696")
	info7.IsFromMe = true
	if !isFleetOwnerCommand(nil, info7) {
		t.Errorf("nil session + IsFromMe → expected ALLOW, got DENY")
	}

	// 8) nil session + random sender → DENY (no panic)
	info8 := types.MessageInfo{}
	info8.Sender = pn("92111222333")
	if isFleetOwnerCommand(nil, info8) {
		t.Errorf("nil session + random user → expected DENY, got ALLOW")
	}

	// 9) Session nil-Manager ke saath random sender → DENY (no panic)
	s9 := &Session{JID: "923480974696:1@s.whatsapp.net"}
	if isFleetOwnerCommand(s9, info4) {
		t.Errorf("nil Manager + random user → expected DENY, got ALLOW")
	}

	// 10) cfg OwnerSet (GOLDMD_OWNER_NUMBERS / paired owner) → ALLOW
	ownJID := "923111222333@s.whatsapp.net"
	m := &Manager{cfg: &Config{OwnerSet: map[string]bool{ownJID: true}}}
	s10 := &Session{JID: "923480974696:1@s.whatsapp.net", Manager: m}
	info10 := types.MessageInfo{}
	info10.Sender = pn("923111222333")
	if !isFleetOwnerCommand(s10, info10) {
		t.Errorf("sender in cfg OwnerSet → expected ALLOW, got DENY")
	}

	// 11) cfg OwnerSet me nahi → DENY
	info11 := types.MessageInfo{}
	info11.Sender = pn("92999988877")
	if isFleetOwnerCommand(s10, info11) {
		t.Errorf("sender NOT in OwnerSet → expected DENY, got ALLOW")
	}

	// 12) LID chat + SenderAlt OwnerSet me → ALLOW
	info12 := types.MessageInfo{}
	info12.Sender = lid("123456789012")
	info12.SenderAlt = pn("923111222333")
	if !isFleetOwnerCommand(s10, info12) {
		t.Errorf("LID chat + SenderAlt in OwnerSet → expected ALLOW, got DENY")
	}
}

// TestFleetOwnerOnlyEnforcement — .host5gb aur .svrchange ownerOnlyCommands
// set me registered hain (fleet_commands.go init) → handler.go dispatch
// non-owner ke liye inhe silently ignore karta hai, bilkul baaki
// owner-only commands jaisa.
func TestFleetOwnerOnlyEnforcement(t *testing.T) {
	if !ownerOnlyCommands["host5gb"] {
		t.Errorf("ownerOnlyCommands[host5gb] expected true, got false")
	}
	if !ownerOnlyCommands["svrchange"] {
		t.Errorf("ownerOnlyCommands[svrchange] expected true, got false")
	}
	// commands registry me dono registered
	if _, ok := Commands["host5gb"]; !ok {
		t.Errorf("Commands[host5gb] not registered")
	}
	if _, ok := Commands["svrchange"]; !ok {
		t.Errorf("Commands[svrchange] not registered")
	}
	// server-menu family PUBLIC — owner-only set me NAHI honi chahiye
	for _, name := range []string{"server", "servers", "svr", "svrinfo", "serverinfo", "session", "sessions"} {
		if ownerOnlyCommands[name] {
			t.Errorf("ownerOnlyCommands[%s] expected false (public command), got true", name)
		}
	}
}
