package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// \u2500\u2500 WHATSAPP-TRUTH COUNT + LOGOUT-LINTER smoke tests \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500

// Count() ab Paired flag kaafi nahi \u2014 socket + device bhi chahiye.
// (Source smoke \u2014 live WhatsApp client ke bina behavioral test possible
// nahi; regression-guard ke liye source assertions.)
func TestCountWhatsAppTruthSource(t *testing.T) {
	srcB, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("manager.go: %v", err)
	}
	src := string(srcB)
	if !strings.Contains(src, "WHATSAPP IS TRUTH: sirf Paired flag") {
		t.Errorf("Count() WhatsApp-truth logic missing")
	}
	if !strings.Contains(src, "s.Client.IsConnected() &&") {
		t.Errorf("Count() IsConnected check missing")
	}
	if !strings.Contains(src, "LoginDeadSince time.Time") {
		t.Errorf("Session.LoginDeadSince field missing")
	}
	wb, err := os.ReadFile("reconnect_watchdog.go")
	if err != nil {
		t.Fatalf("reconnect_watchdog.go: %v", err)
	}
	wsrc := string(wb)
	if !strings.Contains(wsrc, "login dead 60s+") {
		t.Errorf("logout-linter 60s grace missing")
	}
}

// Grace window: LoginDeadSince zero hai to linter sirf mark karta hai.
// (Field access + reset semantics compile-level guaranteed.)
func TestLoginDeadSinceFieldSemantics(t *testing.T) {
	s := &Session{JID: "923000000001@s.whatsapp.net"}
	if !s.LoginDeadSince.IsZero() {
		t.Errorf("LoginDeadSince default zero hona chahiye")
	}
	s.LoginDeadSince = time.Now()
	if s.LoginDeadSince.IsZero() {
		t.Errorf("set karne ke baad non-zero hona chahiye")
	}
	s.LoginDeadSince = time.Time{}
	if !s.LoginDeadSince.IsZero() {
		t.Errorf("reset pe wapas zero hona chahiye")
	}
}
