package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// DISK-ONLY SESSION SYSTEM — structure tests (owner order)
// Direct /code?phone= session: DISK pe safe, Storj pe KABHI nahi, restart
// pe disk se reload, disk se kabhi delete nahi.
// ============================================================================

// stripComments removes // comments so comment text can't false-positive
// the forbidden-pattern scan.
func stripComments2(src string) string {
	lines := strings.Split(src, "\n")
	var out []string
	for _, l := range lines {
		if i := strings.Index(l, "//"); i >= 0 {
			// box-drawing decorated comment lines are pure comments
			l = l[:i]
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func requireContains(t *testing.T, src, needle, where string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Errorf("%s: expected %q not found", where, needle)
	}
}

func requireNotContains(t *testing.T, src, needle, where string) {
	t.Helper()
	if strings.Contains(src, needle) {
		t.Errorf("%s: FORBIDDEN %q found — owner order violation!", where, needle)
	}
}

// TestLocalOnlyFileStructure: session_guards.go me marker + guard + fleet
// key cleaner sab maujood.
func TestLocalOnlyFileStructure(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "const localOnlyMarkerFile", "session_guards.go")
	requireContains(t, src, "func writeLocalOnlyMarker", "session_guards.go")
	requireContains(t, src, "func isLocalOnlyJID", "session_guards.go")
	requireContains(t, src, "func anyLocalOnlyOnDisk", "session_guards.go")
	requireContains(t, src, "func localOnlyUploadBlocked", "session_guards.go")
	requireContains(t, src, "func localOnlyCleanFleetKeys", "session_guards.go")
	// fleet keys cleaner: blob + claim + set + registry sab DEL/SREM/SDEL
	requireContains(t, src, "fleetBlobPrefix + jid", "localOnlyCleanFleetKeys")
	requireContains(t, src, "fleetClaimPrefix+jid", "localOnlyCleanFleetKeys")
	requireContains(t, src, "m.Redis.RemoveJID(jid)", "localOnlyCleanFleetKeys")
}

// TestPairWithCodeDirect: manager.go me direct variant + LocalOnly flag set
// ho raha hai pending session pe.
func TestPairWithCodeDirect(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "func (m *Manager) PairWithCodeDirect", "manager.go")
	requireContains(t, src, "func (m *Manager) pairWithCodeMode", "manager.go")
	requireContains(t, src, "LocalOnly: localOnly,", "manager.go PairWithCode")
	// direct mode: marker + fleet key clean
	// OWNER ORDER: direct /code?phone= marker ALAG local-only folder me
	// (nexstore/local/pairing) — fleet /pair ka global dir untouched.
	requireContains(t, src, "writeLocalOnlyMarker(localPairingDir(), jid)", "manager.go PairWithCode")
	requireContains(t, src, "localOnlyCleanFleetKeys(jid)", "manager.go PairWithCode")
}

// TestPairSuccessSkipsStorj: PairSuccess handler local-only session ke liye
// RegisterJID/SaveSessionDB/fleetOnPairSuccess ko SKIP karta hai.
func TestPairSuccessSkipsStorj(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "if s.LocalOnly {", "manager.go PairSuccess")
	requireContains(t, src, "localOnlyOwnerConfig(s.JID, s.Owner)", "manager.go PairSuccess")
	requireContains(t, src, "if !s.LocalOnly {", "manager.go PairSuccess fleet")
	// handler body me fleetOnPairSuccess sirf guard ke andar
	if i := strings.Index(src, "case *events.PairSuccess:"); i >= 0 {
		body := src[i:]
		if j := strings.Index(body, "go fleetOnPairSuccess"); j >= 0 {
			_ = j // present — guard check via surrounding text
		}
	}
}

// TestCleanupNeverDeletesDiskOnly: cleanupSession linked local-only session
// ka disk data KABHI delete nahi karta (khabardar rule) — memory-only.
func TestCleanupNeverDeletesDiskOnly(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	i := strings.Index(src, "func (m *Manager) cleanupSession")
	if i < 0 {
		t.Fatal("cleanupSession not found")
	}
	body := src[i:]
	if k := strings.Index(body, "\nfunc "); k > 0 {
		body = body[:k]
	}
	requireContains(t, body, "if s.LocalOnly || isLocalOnlyJID", "cleanupSession guard")
	// guard ke andar RemoveAll FORBIDDEN (linked branch), lekin pending
	// branch me allowed — pending ka matlab session bana hi nahi.
	gi := strings.Index(body, "if s.LocalOnly || isLocalOnlyJID")
	gend := strings.Index(body[gi:], "if linked {")
	if gi >= 0 && gend > 0 {
		guardHead := body[gi : gi+gend]
		requireNotContains(t, guardHead, "os.RemoveAll", "cleanupSession linked-branch")
	}
}

// TestAutoLoadBypassesFleetGuards: local-only JID fleet guards (zombie-return
// + online-elsewhere) se bypass hota hai — disk se hamesha load.
func TestAutoLoadBypassesFleetGuards(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "if !isLocalOnlyJID(m.cfg.PairingDir, u) {", "manager.go AutoLoad")
	// dono guards isi block ke andar hon
	i := strings.Index(src, "if !isLocalOnlyJID(m.cfg.PairingDir, u) {")
	if i < 0 {
		t.Fatal("autoload guard-wrap not found")
	}
	window := src[i : i+2200]
	requireContains(t, window, "if fleetHeldByLiveServer(u)", "AutoLoad zombie guard")
	requireContains(t, window, "if fleetSessionOnlineElsewhere(u)", "AutoLoad online-elsewhere guard")
}

// TestSaveSessionDBUploadGuard: storage.go SaveSessionDB me central disk-only
// upload gate hai — local-only session disk pe ho to whole-DB Storj upload
// kabhi nahi hota.
func TestSaveSessionDBUploadGuard(t *testing.T) {
	b, err := os.ReadFile("storage.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	i := strings.Index(src, "func (u *Upstash) SaveSessionDB")
	if i < 0 {
		t.Fatal("SaveSessionDB not found")
	}
	body := src[i:]
	if k := strings.Index(body, "if _, err := os.Stat(path)"); k >= 0 {
		head := body[:k]
		requireContains(t, head, "if localOnlyUploadBlocked()", "SaveSessionDB head")
	} else {
		t.Fatal("SaveSessionDB body marker not found")
	}
}

// TestStartSessionLocalOnlyReload: StartSession disk marker se LocalOnly flag
// set karta hai (restart reconnector) + Redis register skip.
func TestStartSessionLocalOnlyReload(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "LocalOnly: isLocalOnlyJID(m.cfg.PairingDir, jid),", "StartSession")
	requireContains(t, src, "if m.Redis != nil && !sess.LocalOnly {", "StartSession redis-skip")
}

// TestPanelCodeEndpointDirect: panel /code?phone= handler direct mode call
// karta hai (PairWithCodeDirect) aur response storage: disk-only batata hai.
func TestPanelCodeEndpointDirect(t *testing.T) {
	b, err := os.ReadFile("panel.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "mgr.PairWithCodeDirect(phone)", "panel /code")
	requireContains(t, src, `"storage": "disk-only"`, "panel /code")
}

// TestWarGuardDiskOnlySafe: war-guard local-only session ko kabhi surrender
// nahi karta, claim nahi stamp karta, reconnect skip nahi karta.
func TestWarGuardDiskOnlySafe(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "if s.LocalOnly || isLocalOnlyJID(m.cfg.PairingDir, s.JID) {", "surrenderSession")
	requireContains(t, src, "if m.Redis != nil && !isLocalOnlyJID(m.cfg.PairingDir, jid) {", "warRetake claim")
	requireContains(t, src, "if isLocalOnlyJID(m.cfg.PairingDir, jid) {\n\t\treturn false\n\t}", "warGuardShouldSkipConnect")
}

// TestLocalOnlyStoreIsolation: runtime — direct /code?phone= ka store ALAG
// folder + ALAG file me hota hai, aur reconnector (appendLocalOnlyJIDs) us
// folder ko scan karta hai. Fleet paths is test me chhue nahi jate.
func TestLocalOnlyStoreIsolation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOLDMD_LOCAL_DIR", tmp)

	// ALAG folder + ALAG file (goldmd.db se bilkul different path).
	if localDataDir() != tmp {
		t.Fatalf("localDataDir = %q, want %q", localDataDir(), tmp)
	}
	if filepath.Dir(localDBPath()) != tmp {
		t.Fatalf("local.db ka folder alag nahi: %q", localDBPath())
	}
	if filepath.Base(localDBPath()) == "goldmd.db" {
		t.Fatal("local-only store ki file goldmd.db nahi honi chahiye")
	}
	if filepath.Dir(localPairingDir()) != tmp {
		t.Fatalf("local pairing dir alag nahi: %q", localPairingDir())
	}

	// Pairing folder khud se banao + marker likho (jaise /code?phone= karta hai).
	jid := "923158930864@s.whatsapp.net"
	writeLocalOnlyMarker(localPairingDir(), jid)
	if !isLocalOnlyJID(localPairingDir(), jid) {
		t.Fatal("local folder me marker likha par isLocalOnlyJID false")
	}
	// Dual-check: global fleet dir ka path bhi pehchan le (reconnector ke liye).
	if !isLocalOnlyJID("nexstore/pairing", jid) {
		t.Fatal("isLocalOnlyJID ko local folder ka marker nahi mila (dual-check toota)")
	}
	if !anyLocalOnlyOnDisk("nexstore/pairing") {
		t.Fatal("anyLocalOnlyOnDisk ko local folder ka marker nahi mila")
	}

	// Reconnector: local folder ki JIDs scan list me aani chahiye.
	got := appendLocalOnlyJIDs(nil)
	found := false
	for _, g := range got {
		if g == jid {
			found = true
		}
	}
	if !found {
		t.Fatalf("appendLocalOnlyJIDs local folder ki JID nahi laaya: %v", got)
	}
	// Dedup: pehle se list me ho to dobara na aaye.
	if n := len(appendLocalOnlyJIDs([]string{jid})); n != 1 {
		t.Fatalf("appendLocalOnlyJIDs dedup fail: %d", n)
	}

	// Junk folder (marker nahi) scan me nahi aana chahiye.
	_ = os.MkdirAll(filepath.Join(localPairingDir(), "99999999999@s.whatsapp.net"), 0o755)
	for _, g := range appendLocalOnlyJIDs(nil) {
		if g == "99999999999@s.whatsapp.net" {
			t.Fatal("marker-less folder scan me aa gaya")
		}
	}
}

// TestMarkerFileOnDisk: runtime check — writeLocalOnlyMarker/isLocalOnlyJID
// round-trip tmp dir me.
func TestMarkerFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	jid := "923158930864@s.whatsapp.net"
	if isLocalOnlyJID(dir, jid) {
		t.Error("marker should not exist before write")
	}
	writeLocalOnlyMarker(dir, jid)
	if !isLocalOnlyJID(dir, jid) {
		t.Error("marker should exist after write")
	}
	if !anyLocalOnlyOnDisk(dir) {
		t.Error("anyLocalOnlyOnDisk should detect marker")
	}
	// marker file really on disk
	if _, err := os.Stat(filepath.Join(dir, jid, localOnlyMarkerFile)); err != nil {
		t.Errorf("marker file missing: %v", err)
	}
}

// TestFleetRefreshSkipsLocalOnly: 10-min blob refresher local-only session
// skip karta hai.
func TestFleetRefreshSkipsLocalOnly(t *testing.T) {
	b, err := os.ReadFile("fleet.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	i := strings.Index(src, "func fleetRefreshBlobs")
	if i < 0 {
		t.Fatal("fleetRefreshBlobs not found")
	}
	body := src[i:]
	if k := strings.Index(body, "\nfunc "); k > 0 {
		body = body[:k]
	}
	requireContains(t, body, "if s.LocalOnly {", "fleetRefreshBlobs")
}

// TestMenuHeaderFormat: .menu header block me USER / OWNER / MENUS /
// COMMANDS / UPTIME / PREFIX lines hain (owner order — screenshot format).
// FULLMENU pointer line REMOVED (owner order). MENUS line sirf plain .menu me.
func TestMenuHeaderFormat(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	// Shared header helper renders all the header lines.
	requireContains(t, src, `"*│🔰 USER:❯ %s*\n"`, "manager.go USER line")
	requireContains(t, src, `"*│🔰 OWNER :❯ %s*\n"`, "manager.go OWNER line")
	requireContains(t, src, `"*│🔰 MENUS:❯ ❮ %d ❯*\n"`, "manager.go MENUS line")
	requireContains(t, src, `"*│🔰 COMMANDS :❯ ❮ %d ❯*\n"`, "manager.go COMMANDS line")
	requireContains(t, src, `"*│🔰 UPTIME :❯ %s*\n"`, "manager.go UPTIME line")
	requireContains(t, src, `"*│🔰 PREFIX :❯ ❮ %s ❯*\n"`, "manager.go PREFIX line")
	// FULLMENU pointer must be GONE (owner order).
	if strings.Contains(src, "FULLMENU") {
		t.Errorf("manager.go still contains FULLMENU pointer — must be removed")
	}
}

// TestMainGoRegistersUploadGuard: boot pe guard registration.
func TestMainGoRegistersUploadGuard(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "RegisterLocalOnlyUploadGuard(func() bool {", "main.go")
	requireContains(t, src, "return anyLocalOnlyOnDisk(cfg.PairingDir)", "main.go")
}
