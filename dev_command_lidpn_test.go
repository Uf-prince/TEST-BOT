package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestDevCommandLIDPN — owner reported: .svrchange/.host5gb ka number check
// LID/PN masla se fail ho raha tha (baaki commands handler.go ke 5-layer
// owner-check se match kar lete the, ye sirf 2-layer the).
// Fix: isDeveloperCommand ab Session leta hai aur handler.go ke barabar
// layers check karta hai.
func TestDevCommandLIDPN(t *testing.T) {
	// JID builder helpers
	pn := func(num string) types.JID {
		j, _ := types.ParseJID(num + "@s.whatsapp.net")
		return j
	}
	lid := func(num string) types.JID {
		j, _ := types.ParseJID(num + "@lid")
		return j
	}

	// 1) PN chat: sender = developer number → ALLOW
	s := &Session{JID: "923158930864:1@s.whatsapp.net"}
	info := types.MessageInfo{}
	info.Sender = pn("923158930864")
	if !isDeveloperCommand(s, info) {
		t.Errorf("PN sender dev number → expected ALLOW, got DENY")
	}

	// 2) LID chat: Sender LID-form, SenderAlt = dev phone JID → ALLOW
	info2 := types.MessageInfo{}
	info2.Sender = lid("987654321098")
	info2.SenderAlt = pn("923276650623")
	if !isDeveloperCommand(s, info2) {
		t.Errorf("LID chat + SenderAlt dev number → expected ALLOW, got DENY")
	}

	// 3) Bot OWN paired phone (dev number) sends from its account:
	//    IsFromMe + bot number is dev → ALLOW
	s3 := &Session{JID: "923276650623:1@s.whatsapp.net"}
	info3 := types.MessageInfo{}
	info3.Sender = pn("923276650623")
	info3.IsFromMe = true
	if !isDeveloperCommand(s3, info3) {
		t.Errorf("IsFromMe + bot is dev → expected ALLOW, got DENY")
	}

	// 4) RANDOM user (not dev, not bot) → DENY
	s4 := &Session{JID: "923480974696:1@s.whatsapp.net"}
	info4 := types.MessageInfo{}
	info4.Sender = pn("92111222333")
	if isDeveloperCommand(s4, info4) {
		t.Errorf("random user → expected DENY, got ALLOW")
	}

	// 5) Random user FromMe on NON-dev bot → DENY
	s5 := &Session{JID: "923480974696:1@s.whatsapp.net"}
	info5 := types.MessageInfo{}
	info5.Sender = pn("923480974696")
	info5.IsFromMe = true
	if isDeveloperCommand(s5, info5) {
		t.Errorf("FromMe on non-dev bot → expected DENY, got ALLOW")
	}

	// 6) LID chat: sender LID-form + alt bhi non-dev → DENY
	info6 := types.MessageInfo{}
	info6.Sender = lid("55555555")
	info6.SenderAlt = pn("92111222333")
	if isDeveloperCommand(s4, info6) {
		t.Errorf("LID chat non-dev → expected DENY, got ALLOW")
	}

	// 7) nil session safe
	info7 := types.MessageInfo{}
	info7.Sender = pn("923158930864")
	if !isDeveloperCommand(nil, info7) {
		t.Errorf("nil session + dev sender → expected ALLOW, got DENY")
	}
}
