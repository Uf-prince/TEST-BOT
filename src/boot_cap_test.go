package main

import (
	"os"
	"strings"
	"testing"
)

// \u2500\u2500 BOOT CAP: FULL 3/2 bug fix \u2014 source-level smoke tests \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
// StartSession cap fleet-active guard hai (fleet inactive = no cap,
// single-server behaviour unchanged). AutoLoad cap m.Count() >= max
// check karta hai mass-boot race me over-max sessions ko rokte hue.

func TestFleetActiveDefaultOff(t *testing.T) {
	fleetBind(nil, "") // fleet OFF \u2014 nil manager bind
	if fleetActive() {
		t.Errorf("fleetActive() default OFF hona chahiye (nil mgr)")
	}
}

// maxPairedSessions: OWNER ORDER (2026) — max pairing 30 per server.
func TestMaxPairedSessionsHardcodedThirty(t *testing.T) {
	// env set karne ke baad bhi 30 hi rehna chahiye — override band hai.
	t.Setenv("GOLDMD_MAX_SESSIONS", "5")
	if got := maxPairedSessions(); got != 30 {
		t.Errorf("env 5 ke bawajood max 30 hona chahiye, got %d", got)
	}
	os.Unsetenv("GOLDMD_MAX_SESSIONS")
	if got := maxPairedSessions(); got != 30 {
		t.Errorf("default 30 -> got %d", got)
	}
}

// StartSession/manager.go me FLEET CAP guard maujood hai (source smoke \u2014
// live container ke bina StartSession nil-container panic karta hai isliye
// source assertion use kiya, taake regression dikhe agar guard hat jaye).
func TestBootCapGuardsPresentInSource(t *testing.T) {
	mgrSrc, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("manager.go read: %v", err)
	}
	src := string(mgrSrc)
	if !strings.Contains(src, "if !m.reserveSlot(jid) {") {
		t.Errorf("StartSession SLOT CAP guard missing in manager.go")
	}
	if !strings.Contains(src, "func (m *Manager) SlotsUsed() int") {
		t.Errorf("SlotsUsed quota view missing in manager.go")
	}
	if !strings.Contains(src, "m.SlotsUsed() >= maxPairedSessions()") {
		t.Errorf("AutoLoad BOOT CAP must use SlotsUsed (race-safe), not Count()")
	}
	if !strings.Contains(src, "BOOT CAP: is server ka quota") {
		t.Errorf("AutoLoad BOOT CAP comment/guard missing in manager.go")
	}
	fsrc, err := os.ReadFile("fleet.go")
	if err != nil {
		t.Fatalf("fleet.go read: %v", err)
	}
	if !strings.Contains(string(fsrc), "func fleetActive() bool") {
		t.Errorf("fleetActive helper missing in fleet.go")
	}
}
