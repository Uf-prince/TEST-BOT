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

// maxPairedSessions: env override + default 2 (Render free plan 2/server).
func TestMaxPairedSessionsEnv(t *testing.T) {
	t.Setenv("GOLDMD_MAX_SESSIONS", "5")
	if maxPairedSessions() != 5 {
		t.Errorf("env 5 -> got %d", maxPairedSessions())
	}
	os.Unsetenv("GOLDMD_MAX_SESSIONS")
	if maxPairedSessions() != 2 {
		t.Errorf("default 2 -> got %d", maxPairedSessions())
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
	if !strings.Contains(src, "if fleetActive() && m.Count() >= maxPairedSessions()") {
		t.Errorf("StartSession FLEET CAP guard missing in manager.go")
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
