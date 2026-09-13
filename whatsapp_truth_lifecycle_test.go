// WHATSAPP-TRUTH SESSION LIFECYCLE TEST (owner order):
// Session JID chahe jis URL se aaye — pehle WhatsApp se CHECK:
//   • login  → kisi bhi server pe reconnect (registration + save)
//   • logout → SILENT purge (fleet blob/meta/set/claim + own jids/blob),
//              CONFIGURATION (settings:<jid>) HAMESHA SAFE
//   • transient → purge NAHI (retry via fleet cooldown / runtime linter)
package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// ── structural contracts ────────────────────────────────────────────────────

func TestTruthStructuralContracts(t *testing.T) {
	src := readFileOrFail(t, "manager.go")
	fleetSrc := readFileOrFail(t, "fleet.go")

	// 1. verify window StartSession ke andar hai (Connect ke baad).
	if !strings.Contains(src, "whatsappTruthVerified(sess, 12*time.Second)") {
		t.Errorf("StartSession me verify window missing")
	}
	// 2. explicit logout → silent purge + config-safe error.
	if !strings.Contains(src, "fleetPurgeLoggedOutSession(jid)") {
		t.Errorf("explicit-logout purge call missing")
	}
	if !strings.Contains(src, "whatsapp logged out %s — session data purged, configuration kept") {
		t.Errorf("config-safe error message missing")
	}
	// 3. transient → purge NAHI: else-branch (verified ke baad wala) me
	// koi purge call nahi hona chahiye — purge SIRF explicitLogout me.
	elseBlock := substringBetween(t, src, "if verified {", "\t\t}\n\t}")
	if strings.Contains(elseBlock, "fleetPurgeLoggedOutSession") {
		t.Errorf("transient (else) branch me purge ho raha hai (galat — purge sirf explicit logout pe)")
	}
	// 4. helper 3-return signature (verified, explicitLogout, why).
	if !strings.Contains(src, "func whatsappTruthVerified(s *Session, window time.Duration) (bool, bool, string)") {
		t.Errorf("whatsappTruthVerified 3-return signature missing")
	}
	// 5. cleanupSession → deep sync purge (fleetOnCleanup async nahi).
	if !strings.Contains(src, "fleetPurgeLoggedOutSession(s.JID)") {
		t.Errorf("cleanupSession deep purge missing")
	}
	// 6. fleet purge: config SAFE — settings key kabhi delete nahi.
	purgeFn := substringBetween(t, fleetSrc, "func fleetPurgeLoggedOutSession", "REAL EGRESS TRACKING")
	for _, bad := range []string{`"DEL", "settings:`, `HDEL", "settings:` + jidPattern} {
		if strings.Contains(purgeFn, bad) {
			t.Errorf("fleet purge me settings delete ho rahi hai: %q", bad)
		}
	}
	if !strings.Contains(purgeFn, "setDel(fleetBlobPrefix + jid)") {
		t.Errorf("fleet blob delete missing in purge")
	}
	if !strings.Contains(purgeFn, "setRem(fleetSessionsSet, jid)") {
		t.Errorf("fleet set remove missing in purge")
	}
	if !strings.Contains(purgeFn, "RemoveJID(jid)") {
		t.Errorf("own jids registry remove missing in purge")
	}
	// 7. blob-restore 3x fail → purge (dead blob ko kabhi claim na ho).
	if !strings.Contains(fleetSrc, "fleetRestoreAttempts") {
		t.Errorf("fleetRestoreAttempts (3x dead-blob purge) missing")
	}
	// 8. StartSession transient return — fleet retry path.
	if !strings.Contains(fleetSrc, `strings.Contains(err.Error(), "whatsapp logged out")`) {
		t.Errorf("fleet connect-fail classification (logout vs transient) missing")
	}
}

const jidPattern = " PLACEHOLDER "

// ── runtime behaviour: Store.Deleted logout / transient / nil ───────────────

func newTruthClient(t *testing.T, deleted bool) *whatsmeow.Client {
	t.Helper()
	j, err := types.ParseJID("923000000001@s.whatsapp.net")
	if err != nil {
		t.Fatalf("parse jid: %v", err)
	}
	// store.Device direct literal (repo me NewDevice helper nahi hai).
	dev := &store.Device{ID: &j}
	if deleted {
		// 401/403/410/device_removed logout simulate — whatsmeow isi flag
		// ko true karta hai (store.Delete + NoopStore assign).
		dev.Deleted = true
	}
	return whatsmeow.NewClient(dev, nil)
}

func TestTruthVerifyWindowLogout(t *testing.T) {
	s := &Session{JID: "923000000001@s.whatsapp.net"}
	s.Client = newTruthClient(t, true) // Store.Deleted = explicit logout
	verified, explicitLogout, why := whatsappTruthVerified(s, 2*time.Second)
	if verified {
		t.Fatalf("logout state me verified=true aaya")
	}
	if !explicitLogout {
		t.Fatalf("Store.Deleted=true ke baavjood explicitLogout=false (%s)", why)
	}
	if !strings.Contains(why, "explicit logout") {
		t.Errorf("reason me 'explicit logout' hona chahiye, mila: %s", why)
	}
}

func TestTruthVerifyWindowTransient(t *testing.T) {
	s := &Session{JID: "923000000002@s.whatsapp.net"}
	s.Client = newTruthClient(t, false) // na login, na deleted
	verified, explicitLogout, why := whatsappTruthVerified(s, 1*time.Second)
	if verified {
		t.Fatalf("transient state me verified=true")
	}
	if explicitLogout {
		t.Fatalf("transient me explicitLogout=true (purge ho jata — galat)")
	}
	if !strings.Contains(why, "transient") {
		t.Errorf("reason me 'transient' hona chahiye, mila: %s", why)
	}
}

func TestTruthVerifyWindowNilSafe(t *testing.T) {
	verified, explicitLogout, why := whatsappTruthVerified(nil, 1*time.Second)
	if verified || explicitLogout || why == "" {
		t.Errorf("nil session me (false,false,reason) hona chahiye, mila: (%v,%v,%q)", verified, explicitLogout, why)
	}
}

func readFileOrFail(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func substringBetween(t *testing.T, src, start, end string) string {
	t.Helper()
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("start %q missing", start)
	}
	j := strings.Index(src[i+len(start):], end)
	if j < 0 {
		t.Fatalf("end %q after start missing", end)
	}
	return src[i : i+len(start)+j]
}
