package main

import (
	"strings"
	"testing"
	"time"
)

// fleetHolderAlive decision branches (network ke bina):
//   sid=""                    -> false
//   sid=fleetSelfID           -> true (hum khud)
//   heartbeat missing         -> false (purani entry = dead)
//   heartbeat fresh (<2min)   -> true (tick chal raha, no probe)
// fleetServerURL:
//   ""                        -> ""
//   https://x.onrender.com    -> as-is (trailing / strip)
//   x.onrender.com            -> https://x.onrender.com
//   bare-hostname (no dot)    -> "" (URL nahi banta — probe skip)
// fleetHeldByLiveServer: fleet inactive (Storj not ready) -> false
// fleetBind: fleetMgr/fleetDBPath set without watchdog.

func TestFleetHolderAliveBranches(t *testing.T) {
	savedSelf := fleetSelfID
	defer func() { fleetSelfID = savedSelf }()
	fleetSelfID = "server-a.onrender.com"

	hb := map[string]int64{}
	if fleetHolderAlive("", hb) {
		t.Errorf("empty sid must be dead")
	}
	if !fleetHolderAlive(fleetSelfID, hb) {
		t.Errorf("self must always be alive")
	}
	if fleetHolderAlive("gone.onrender.com", hb) {
		t.Errorf("missing heartbeat must be dead")
	}
	// fresh heartbeat (<2min) — no probe needed
	hb["busy.onrender.com"] = time.Now().Unix() - 30
	if !fleetHolderAlive("busy.onrender.com", hb) {
		t.Errorf("fresh heartbeat must be alive without probe")
	}
	// stale heartbeat with unresolvable sid (bare hostname, no dot) —
	// URL empty → dead
	hb["standalone-host"] = time.Now().Unix() - 300 // 5min stale
	if fleetHolderAlive("standalone-host", hb) {
		t.Errorf("stale heartbeat + no URL must be dead")
	}
}

func TestFleetServerURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"https://srv-9.onrender.com", "https://srv-9.onrender.com"},
		{"https://srv-9.onrender.com/", "https://srv-9.onrender.com"},
		{"srv-9.onrender.com", "https://srv-9.onrender.com"},
		{"srv-9.onrender.com/", "https://srv-9.onrender.com"},
		{"barehost", ""},
	}
	for _, c := range cases {
		got := fleetServerURL(c.in)
		if got != c.want {
			t.Errorf("fleetServerURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFleetHeldByLiveServerInactiveFleet(t *testing.T) {
	// Storj not ready in tests → guard must return false (sab local load)
	if fleetHeldByLiveServer("9231500000000@s.whatsapp.net") {
		t.Errorf("inactive fleet must never hold sessions")
	}
}

func TestFleetBindSetsGlobals(t *testing.T) {
	oldMgr, oldDB := fleetMgr, fleetDBPath
	defer func() { fleetMgr, fleetDBPath = oldMgr, oldDB }()

	// nil manager — no-op, no panic, kuch overwrite nahi
	fleetBind(nil, "x.db")
	if fleetMgr != oldMgr {
		t.Errorf("nil bind must not overwrite fleetMgr")
	}
	if fleetDBPath != oldDB {
		t.Errorf("nil bind must not overwrite fleetDBPath")
	}

	// real bind — dono set ho jayein (Redis nil hai → fleet inactive but
	// bind sirf globals set karta hai, watchdog nahi chalata)
	fakeMgr := &Manager{}
	fleetBind(fakeMgr, "bind-test.db")
	if fleetMgr != fakeMgr {
		t.Errorf("fleetMgr not set by fleetBind")
	}
	if fleetDBPath != "bind-test.db" {
		t.Errorf("fleetDBPath not set by fleetBind (got %q)", fleetDBPath)
	}
}

func TestFleetNotifyTextShape(t *testing.T) {
	// sirf compile-time sanity: function exists aur strings helper consistent
	if !strings.Contains(fleetUserPart("923158930864:12@s.whatsapp.net"), "923158930864") {
		t.Errorf("user part extraction broken")
	}
}
