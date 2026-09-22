package goldcmds

// ============================================================================
// GOLD-MD — tgmenu live tests (channel scan + menu card + availability)
// ============================================================================

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestTGMenuScan — live channel scan on t.me/telegram (control channel).
func TestTGMenuScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inv, err := tgChannelScan(ctx, "https://t.me/telegram")
	if err != nil {
		t.Fatalf("tgChannelScan(telegram): %v", err)
	}
	t.Logf("telegram: text=%v photo=%v video=%v audio=%v",
		inv.textMedia != nil, inv.photoMedia != nil,
		inv.videoMedia != nil, inv.audioAvailable())
	if inv.channel != "telegram" {
		t.Errorf("channel = %q, want telegram", inv.channel)
	}
	if inv.videoMedia == nil {
		t.Errorf("telegram channel should have a video post on /s/ page")
	}

	// menu card must only list available types
	card := tgTypeMenuCard(inv, "Telegram News")
	t.Logf("MENU CARD:\n%s", card)
	if inv.textMedia == nil && strings.Contains(card, "TEXT ONLY") {
		t.Errorf("card shows TEXT line but textMedia is nil")
	}
	if inv.photoMedia == nil && strings.Contains(card, "GET PHOTO") {
		t.Errorf("card shows PHOTO line but photoMedia is nil")
	}
	if inv.videoMedia == nil && strings.Contains(card, "GET VIDEO") {
		t.Errorf("card shows VIDEO line but videoMedia is nil")
	}
	if !inv.audioAvailable() && strings.Contains(card, "GET AUDIO") {
		t.Errorf("card shows AUDIO line but audio unavailable")
	}
}

// TestTGMenuScanDurov — second control channel (photo+video mix).
func TestTGMenuScanDurov(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inv, err := tgChannelScan(ctx, "https://t.me/durov")
	if err != nil {
		t.Fatalf("tgChannelScan(durov): %v", err)
	}
	t.Logf("durov: text=%v photo=%v video=%v",
		inv.textMedia != nil, inv.photoMedia != nil, inv.videoMedia != nil)
	if inv.photoMedia == nil {
		t.Errorf("durov channel should have a photo post")
	}
	card := tgTypeMenuCard(inv, "Durov's Channel")
	if !strings.Contains(card, "GET PHOTO") {
		t.Errorf("durov card missing PHOTO line")
	}
	if inv.textMedia != nil && !strings.Contains(card, "TEXT ONLY") {
		t.Errorf("durov has text but card missing TEXT line")
	}
}

// TestTGMenuScanBadChannel — dead channel → error, empty inventory.
func TestTGMenuScanBadChannel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	inv, err := tgChannelScan(ctx, "https://t.me/thischanneldoesnotexist9999xyz")
	if err == nil {
		// /s/ route redirects (302 → non-200 → error) for dead channels
		if inv != nil && !inv.empty() {
			t.Errorf("dead channel returned non-empty inventory: %+v", inv)
		}
	} else {
		t.Logf("dead channel error (expected): %v", err)
	}
	if inv != nil && inv.audioAvailable() {
		t.Errorf("dead channel can't have audio")
	}
}

// TestTGMenuNotAvailCard — error card text.
func TestTGMenuNotAvailCard(t *testing.T) {
	card := tgTypeNotAvailCard("AUDIO")
	if !strings.Contains(card, "AUDIO") || !strings.Contains(card, "NOT AVAILABLE") {
		t.Errorf("bad not-avail card: %q", card)
	}
}
