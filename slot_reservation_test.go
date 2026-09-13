package main

// SLOT RESERVATION RACE TEST — owner request: "Server band to dusre server
// pe move wo b perfect kam kr rha na?" failover re-verification me humne
// Count()-race pakda: StartSession session ko connect hone ke BAAD map me
// daalta hai, isliye AutoLoad batch-5 / concurrent panel pairings race
// window me sab Count()=0 dekh kar pass ho jate the → FULL 5/2 over-max
// (owner ne ye exact state panel me dekha tha). Ye test file prove karti
// hai ke fix (reserveSlot/SlotsUsed) race ke against atomic hai.

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSlotsUsedCountsInFlightReservations: 2 live-less reservations ke saath
// SlotsUsed = live(0) + in-flight(2) = 2. Quota (max 2) full ho jani
// chahiye — teesri reserve REJECT honi chahiye. (Count() yahan 0 hi deta —
// race wahi tha.)
func TestSlotsUsedCountsInFlightReservations(t *testing.T) {
	m := NewManager(&Config{}, nil) // nil container OK — sirf maps use hote hain
	os.Unsetenv("GOLDMD_MAX_SESSIONS")

	if got := m.SlotsUsed(); got != 0 {
		t.Fatalf("fresh SlotsUsed = %d, want 0", got)
	}
	if !m.reserveSlot("a@s.whatsapp.net") {
		t.Fatalf("first reserve should pass (0 live + 0 in-flight)")
	}
	if !m.reserveSlot("b@s.whatsapp.net") {
		t.Fatalf("second reserve should pass (0 live + 1 in-flight)")
	}
	if got := m.SlotsUsed(); got != 2 {
		t.Fatalf("SlotsUsed after 2 in-flight = %d, want 2 (Count() yahan 0 deta — race tha)", got)
	}
	// quota (default max 2) ab full — teesri reserve reject:
	if m.reserveSlot("c@s.whatsapp.net") {
		t.Fatalf("third reserve must be REJECTED (quota 2/2 full with in-flight)")
	}
	// release karke slot wapas:
	m.releaseSlot("a@s.whatsapp.net")
	if !m.reserveSlot("c@s.whatsapp.net") {
		t.Fatalf("reserve after release should pass")
	}
}

// TestReserveSlotStaleSelfHeal: leaked/orphan reservation (crash, guard
// fail) 10 min baad khud expire — quota permanently block nahi hota.
func TestReserveSlotStaleSelfHeal(t *testing.T) {
	m := NewManager(&Config{}, nil)
	os.Unsetenv("GOLDMD_MAX_SESSIONS")
	m.slotMu.Lock()
	m.slotRes["ghost@s.whatsapp.net"] = time.Now().Add(-11 * time.Minute) // leaked
	m.slotMu.Unlock()
	if got := m.SlotsUsed(); got != 0 {
		t.Fatalf("stale ghost reservation should self-expire; SlotsUsed=%d", got)
	}
	if !m.reserveSlot("fresh@s.whatsapp.net") {
		t.Fatalf("fresh reserve must pass after ghost expiry")
	}
}

// TestSlotsUsedConcurrentHammer: 20 goroutines ek saath SlotsUsed +
// reserveSlot — koi race/panic nahi, max-reservations quota (2) se aage
// nahi badhta. Ye wahi race simulation hai jo AutoLoad batch-5 mass-boot
// me hota tha.
func TestSlotsUsedConcurrentHammer(t *testing.T) {
	m := NewManager(&Config{}, nil)
	os.Unsetenv("GOLDMD_MAX_SESSIONS")
	var wg sync.WaitGroup
	var okCount int64
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			jid := string(rune('a'+n%26)) + "@s.whatsapp.net"
			if m.reserveSlot(jid) {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
			_ = m.SlotsUsed() // race detector: concurrent reads safe
		}(i)
	}
	wg.Wait()
	if okCount > 2 {
		t.Fatalf("quota breach! concurrent reserves granted %d > max 2 — race abhi bhi hai", okCount)
	}
	t.Logf("concurrent hammer: %d/2 reservations granted (race-safe)", okCount)
}

// TestSlotCapSourceGuards: regression guards — fix ke critical anchors
// source me maujood hon. Agar koi refactor inhein hata de to test fail.
func TestSlotCapSourceGuards(t *testing.T) {
	mgrSrc, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("manager.go read: %v", err)
	}
	src := string(mgrSrc)
	for _, anchor := range []string{
		"if !m.reserveSlot(jid) {",
		"func (m *Manager) SlotsUsed() int",
		"func (m *Manager) reserveSlot(jid string) bool",
		"func (m *Manager) releaseSlot(jid string)",
		"m.SlotsUsed() >= maxPairedSessions()",
		"ok = true // connect SUCCESS",
		"pairedOK = true // code diya gaya",
		"m.releaseSlot(s.JID)",
	} {
		if !strings.Contains(src, anchor) {
			t.Errorf("manager.go anchor missing: %q", anchor)
		}
	}
	fsrc, err := os.ReadFile("fleet.go")
	if err != nil {
		t.Fatalf("fleet.go read: %v", err)
	}
	for _, anchor := range []string{
		"m.SlotsUsed() >= max",
		"m.SlotsUsed() >= max",
		"slots %d/%d",
	} {
		if !strings.Contains(string(fsrc), anchor) {
			t.Errorf("fleet.go anchor missing: %q", anchor)
		}
	}
	psrc, err := os.ReadFile("panel.go")
	if err != nil {
		t.Fatalf("panel.go read: %v", err)
	}
	if !strings.Contains(string(psrc), "mgr.SlotsUsed() >= effectiveMax") {
		t.Errorf("panel.go quota anchor missing: mgr.SlotsUsed() >= effectiveMax")
	}
}
