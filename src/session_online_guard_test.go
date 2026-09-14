package main

import (
	"os"
	"strings"
	"testing"
)

// ============================================================================
// SESSION ONLINE-ELSEWHERE GUARD — structure test (owner order):
// "guard jab Storj session check kare: session kisi AUR server pe already
// ONLINE hai to IGNORE (reconnect NAHI); OFFLINE + logged-in hai to pehle
// jaisa reconnect hi chale."
// ============================================================================

// TestGuardFileStructure: session_guards.go ki zaroori cheezein.
func TestGuardFileStructure(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal("session_guards.go missing:", err)
	}
	src := string(b)

	for _, want := range []string{
		"func fleetSessionOnlineElsewhere(jid string) bool",
		"fleetHeldByLiveServer(jid)",
		"guardRemoteSessionOnline(",
		"/sessions",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("session_guards.go me missing: %q", want)
		}
	}
}

// TestGuardHookAutoLoad: AutoLoad me online-elsewhere guard lagna chahiye.
func TestGuardHookAutoLoad(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal("manager.go missing:", err)
	}
	src := string(b)

	if !strings.Contains(src, "if fleetSessionOnlineElsewhere(u) {") {
		t.Error("AutoLoad hook missing: fleetSessionOnlineElsewhere(u) guard")
	}
	// zombie guard ke BAAD hona chahiye (claim check pehle — cheap & authoritative)
	zi := strings.Index(src, "if fleetHeldByLiveServer(u) {")
	gi := strings.Index(src, "if fleetSessionOnlineElsewhere(u) {")
	if zi == -1 || gi == -1 || gi < zi {
		t.Error("AutoLoad me guard order galat: online-elsewhere check zombie/claim check ke BAAD hona chahiye")
	}
}

// TestGuardHookFleetClaim: fleet claim paths pe guard.
func TestGuardHookFleetClaim(t *testing.T) {
	b, err := os.ReadFile("fleet.go")
	if err != nil {
		t.Fatal("fleet.go missing:", err)
	}
	src := string(b)

	// fleetClaimAvailable: claim se PEHLE online-elsewhere check
	if !strings.Contains(src, "if fleetSessionOnlineElsewhere(jid) {\n\t\t\tInfoLog(\"FLEET: skip %s — session already ONLINE on another server (no claim, no reconnect)\", jid)") {
		t.Error("fleetClaimAvailable hook missing (skip + no claim)")
	}
	// fleetRestoreAndConnect: race-win ke BAAD StartSession se PEHLE
	ri := strings.Index(src, "if fleetSessionOnlineElsewhere(jid) {\n\t\t_, _ = m.Redis.cmd(\"HDEL\", fleetClaimPrefix+jid, fleetSelfID)")
	if ri == -1 {
		t.Error("fleetRestoreAndConnect hook missing (release claim + return)")
	}
	si := strings.Index(src, "if err := m.StartSession(jid); err != nil {")
	if si == -1 || ri > si {
		t.Error("restoreAndConnect me guard StartSession se PEHLE hona chahiye")
	}
}

// TestGuardOrderClaimFirst: guard STEP 1 claim check hai, STEP 2 probe.
func TestGuardOrderClaimFirst(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal("session_guards.go missing:", err)
	}
	src := string(b)
	ci := strings.Index(src, "if fleetHeldByLiveServer(jid) {")
	pi := strings.Index(src, "guardRemoteServers()")
	if ci == -1 || pi == -1 || pi < ci {
		t.Error("guard me order galat: claim check (STEP 1) remote probe (STEP 2) se PEHLE hona chahiye")
	}
}
