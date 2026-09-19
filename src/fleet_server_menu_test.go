package main

import (
	"strings"
	"testing"
)

// TestFleetServersMenuTextFormat — .server report ka OWNER ORDER format:
// 3 summary lines (ACTIVE BOTS / ONLINE SERVERS / OFFLINE SERVERS), phir
// ONLINE servers number-order me (*SERVER N ACTIVE x/2*), phir OFFLINE
// servers number-order me (*SERVER N OFFLINE*).
func TestFleetServersMenuTextFormat(t *testing.T) {
	scan := []fleetServerInfo{
		{Name: "SERVER 1", Online: true, Sessions: 1, Max: 2},
		{Name: "SERVER 2", Online: true, Sessions: 0, Max: 2},
		{Name: "SERVER 3", Online: false},
		{Name: "SERVER 4", Online: false},
		{Name: "SERVER 5", Online: true, Sessions: 2, Max: 2},
	}

	got := fleetServersMenuText(scan)

	// Summary lines — ACTIVE BOTS = total live pairings (1+0+2 = 3).
	wantSummary := "*ACTIVE BOTS :❯ ❮ 3 ❯*\n" +
		"*ONLINE SERVERS :❯ ❮ 3 ❯*\n" +
		"*OFFLINE SERVERS :❯ ❮ 2 ❯*\n\n"
	if !strings.HasPrefix(got, wantSummary) {
		t.Fatalf("summary mismatch.\n got: %q\nwant prefix: %q", got, wantSummary)
	}

	// ONLINE lines (number order).
	for _, want := range []string{
		"*SERVER 1 ACTIVE 1/2*\n",
		"*SERVER 2 ACTIVE 0/2*\n",
		"*SERVER 5 ACTIVE 2/2*\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing online line %q in:\n%s", want, got)
		}
	}

	// OFFLINE lines (number order).
	for _, want := range []string{
		"*SERVER 3 OFFLINE*\n",
		"*SERVER 4 OFFLINE*\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing offline line %q in:\n%s", want, got)
		}
	}

	// ONLINE block must come BEFORE OFFLINE block.
	idxOnline := strings.Index(got, "*SERVER 1 ACTIVE")
	idxOffline := strings.Index(got, "*SERVER 3 OFFLINE")
	if idxOnline < 0 || idxOffline < 0 || idxOnline > idxOffline {
		t.Errorf("online block should precede offline block.\n%s", got)
	}

	// No old "INFORMATION" / "GOLD-MD SERVERS INFO" header.
	if strings.Contains(got, "INFORMATION") || strings.Contains(got, "SERVERS INFO") {
		t.Errorf("old format leaked into new report:\n%s", got)
	}
}

// TestFleetServersMenuTextAllOffline — sab offline: ACTIVE BOTS 0, ONLINE 0.
func TestFleetServersMenuTextAllOffline(t *testing.T) {
	scan := []fleetServerInfo{
		{Name: "SERVER 1", Online: false},
		{Name: "SERVER 2", Online: false},
	}
	got := fleetServersMenuText(scan)
	if !strings.HasPrefix(got, "*ACTIVE BOTS :❯ ❮ 0 ❯*\n*ONLINE SERVERS :❯ ❮ 0 ❯*\n*OFFLINE SERVERS :❯ ❮ 2 ❯*\n\n") {
		t.Fatalf("all-offline summary wrong:\n%s", got)
	}
	if !strings.Contains(got, "*SERVER 1 OFFLINE*\n") || !strings.Contains(got, "*SERVER 2 OFFLINE*\n") {
		t.Fatalf("all-offline lines wrong:\n%s", got)
	}
}
