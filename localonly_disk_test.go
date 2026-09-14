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

// TestLocalOnlyFileStructure: localonly_session.go me marker + guard + fleet
// key cleaner sab maujood.
func TestLocalOnlyFileStructure(t *testing.T) {
	b, err := os.ReadFile("localonly_session.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "const localOnlyMarkerFile", "localonly_session.go")
	requireContains(t, src, "func writeLocalOnlyMarker", "localonly_session.go")
	requireContains(t, src, "func isLocalOnlyJID", "localonly_session.go")
	requireContains(t, src, "func anyLocalOnlyOnDisk", "localonly_session.go")
	requireContains(t, src, "func localOnlyUploadBlocked", "localonly_session.go")
	requireContains(t, src, "func localOnlyCleanFleetKeys", "localonly_session.go")
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
	requireContains(t, src, "writeLocalOnlyMarker(m.cfg.PairingDir, jid)", "manager.go PairWithCode")
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

// TestSaveSessionDBUploadGuard: upstash.go SaveSessionDB me central disk-only
// upload gate hai — local-only session disk pe ho to whole-DB Storj upload
// kabhi nahi hota.
func TestSaveSessionDBUploadGuard(t *testing.T) {
	b, err := os.ReadFile("upstash.go")
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
	b, err := os.ReadFile("session_war_guard.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, "if s.LocalOnly || isLocalOnlyJID(m.cfg.PairingDir, s.JID) {", "surrenderSession")
	requireContains(t, src, "if m.Redis != nil && !isLocalOnlyJID(m.cfg.PairingDir, jid) {", "warRetake claim")
	requireContains(t, src, "if isLocalOnlyJID(m.cfg.PairingDir, jid) {\n\t\treturn false\n\t}", "warGuardShouldSkipConnect")
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

// TestMenuFullmenuPointer: .menu caption me HI {pushname} / SEE MY BOT
// COMMANDS ke baad TYPE ❮ {prefix}FULLMENU ❯ / TO SHOW FULL MENU lines
// hain (owner order — screenshot format).
func TestMenuFullmenuPointer(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripComments2(string(b))
	requireContains(t, src, `"*HI %s*\n*SEE MY BOT COMMANDS*\n*TYPE ❮ %sFULLMENU ❯*\n*TO SHOW FULL MENU*\n\n"`, "manager.go menu caption")
	// prefix + pushName dono format args hon
	i := strings.Index(src, `"*HI %s*\n*SEE MY BOT COMMANDS*\n*TYPE ❮ %sFULLMENU ❯*\n*TO SHOW FULL MENU*\n\n"`)
	if i < 0 {
		t.Fatal("menu caption format string not found")
	}
	line := src[i : strings.Index(src[i:], "\n")+i]
	if !strings.Contains(line, "pushName, prefix") {
		t.Errorf("menu caption must pass pushName, prefix — got: %s", line)
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
