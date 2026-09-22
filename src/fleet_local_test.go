package main

// fleet_local_test.go — LOCAL FLEET REGISTRY tests (owner order 2026-09-21).
//
// Verifies:
//   1. flParseHeartbeat parses "<ts>|<sessions>|<max>|<url>" + legacy "<ts>".
//   2. flSetClaim/flClaimHolders round-trip (local file read).
//   3. flSetServer/flHeartbeatMap + flServerURL round-trip.
//   4. flSetOnline/flSessionInfo + flRemoveClaim.
//   5. flSave/flLoadFromDisk persistence (disk survives reload).
//   6. flMirrorKV maps fleet KV ops → local registry.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func flTestSetup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	flEnabled = true
	flReady = true
	flDir = dir
	flPath = filepath.Join(dir, "registry.json")
	flMu.Lock()
	flReg = &flRegistry{Sessions: map[string]*flSession{}, Servers: map[string]*flServer{}}
	flMu.Unlock()
}

func TestFlParseHeartbeat(t *testing.T) {
	ts, url, sess, mx := flParseHeartbeat("1790031825|2|2|https://gold-md-xsvr84.onrender.com")
	if ts != 1790031825 {
		t.Fatalf("ts=%d", ts)
	}
	if url != "https://gold-md-xsvr84.onrender.com" {
		t.Fatalf("url=%q", url)
	}
	if sess != 2 || mx != 2 {
		t.Fatalf("sess=%d mx=%d", sess, mx)
	}
	// legacy "<ts>"
	ts2, url2, sess2, mx2 := flParseHeartbeat("1790031825")
	if ts2 != 1790031825 || url2 != "" || sess2 != -1 || mx2 != -1 {
		t.Fatalf("legacy parse: ts=%d url=%q sess=%d mx=%d", ts2, url2, sess2, mx2)
	}
}

func TestFlClaimRoundTrip(t *testing.T) {
	flTestSetup(t)
	jid := "923158930864@s.whatsapp.net"
	flSetClaim(jid, "svr15602", "https://svr15602.example", 1790031825)

	h, ok := flClaimHolders(jid)
	if !ok {
		t.Fatal("flClaimHolders: not found")
	}
	if h["svr15602"] != 1790031825 {
		t.Fatalf("claim ts=%d", h["svr15602"])
	}
	info, ok := flSessionInfo(jid)
	if !ok || info.ServerID != "svr15602" || info.ServerURL != "https://svr15602.example" {
		t.Fatalf("session info=%+v ok=%v", info, ok)
	}
}

func TestFlServerRoundTrip(t *testing.T) {
	flTestSetup(t)
	flSetServer("svr15602", "https://svr15602.example", 1790031825, 1, 2)
	hb := flHeartbeatMap()
	if hb["svr15602"] != 1790031825 {
		t.Fatalf("heartbeat=%v", hb)
	}
	if u := flServerURL("svr15602"); u != "https://svr15602.example" {
		t.Fatalf("url=%q", u)
	}
}

func TestFlOnlineAndRemoveClaim(t *testing.T) {
	flTestSetup(t)
	jid := "923480974696@s.whatsapp.net"
	flSetClaim(jid, fleetSelfID, "https://svr15602.example", 1790031825)
	flSetOnline(jid, true)
	info, _ := flSessionInfo(jid)
	if !info.Online {
		t.Fatal("online should be true")
	}
	// remove claim (offline notify path) — entry rahe, claim clear.
	flRemoveClaim(jid)
	info, ok := flSessionInfo(jid)
	if !ok {
		t.Fatal("entry should remain after flRemoveClaim")
	}
	if info.ServerID != "" || info.ClaimTS != 0 {
		t.Fatalf("claim should be cleared: %+v", info)
	}
	if _, ok := flClaimHolders(jid); ok {
		t.Fatal("flClaimHolders should be empty after remove")
	}
}

func TestFlDiskPersistence(t *testing.T) {
	flTestSetup(t)
	jid := "923488345404@s.whatsapp.net"
	flSetClaim(jid, "svr15602", "https://svr15602.example", 1790031825)
	flSetServer("svr15602", "https://svr15602.example", 1790031825, 1, 2)

	// file mojood honi chahiye
	if _, err := os.Stat(flPath); err != nil {
		t.Fatalf("registry.json missing: %v", err)
	}
	// memory wipe + disk se reload
	flMu.Lock()
	flReg = &flRegistry{Sessions: map[string]*flSession{}, Servers: map[string]*flServer{}}
	flMu.Unlock()
	if !flLoadFromDisk() {
		t.Fatal("flLoadFromDisk failed")
	}
	h, ok := flClaimHolders(jid)
	if !ok || h["svr15602"] != 1790031825 {
		t.Fatalf("reload claim=%v ok=%v", h, ok)
	}
	if flHeartbeatMap()["svr15602"] != 1790031825 {
		t.Fatal("reload heartbeat failed")
	}
}

func TestFlMirrorKV(t *testing.T) {
	flTestSetup(t)
	jid := "94753540320@s.whatsapp.net"
	// HSET claim (own server)
	flMirrorKV([]string{"HSET", fleetClaimPrefix + jid, fleetSelfID, "1790031825"})
	if h, ok := flClaimHolders(jid); !ok || h[fleetSelfID] != 1790031825 {
		t.Fatalf("mirror HSET claim failed: %v ok=%v", h, ok)
	}
	// HSET servers hash
	flMirrorKV([]string{"HSET", fleetServersHash, "svr15602",
		"1790031825|1|2|https://svr15602.example"})
	if flHeartbeatMap()["svr15602"] != 1790031825 {
		t.Fatal("mirror HSET server failed")
	}
	if flServerURL("svr15602") != "https://svr15602.example" {
		t.Fatal("mirror server url failed")
	}
	// SADD sessions set
	flMirrorKV([]string{"SADD", fleetSessionsSet, jid})
	if _, ok := flSessionInfo(jid); !ok {
		t.Fatal("mirror SADD session failed")
	}
	// HDEL own claim
	flMirrorKV([]string{"HDEL", fleetClaimPrefix + jid, fleetSelfID})
	if _, ok := flClaimHolders(jid); ok {
		t.Fatal("mirror HDEL own claim failed")
	}
}

func TestFlNotifyOfflineLocal(t *testing.T) {
	flTestSetup(t)
	jid := "923369946470@s.whatsapp.net"
	flSetClaim(jid, fleetSelfID, "https://svr15602.example", time.Now().Unix())
	flSetOnline(jid, true)
	// fleetMgr nil → Storj part skip, local part chale.
	flNotifyOffline(jid)
	info, ok := flSessionInfo(jid)
	if !ok {
		t.Fatal("entry should remain")
	}
	if info.Online {
		t.Fatal("online should be false after notify")
	}
	if info.ServerID != "" {
		t.Fatalf("claim should be released: %+v", info)
	}
}
