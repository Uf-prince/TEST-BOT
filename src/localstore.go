package main

// ============================================================================
// GOLD-MD — LOCAL-ONLY SESSION STORE (owner order)
// File: localstore.go
// ============================================================================
// OWNER ORDER (Roman Urdu, verbatim intent):
//   "/code?phone= endpoint ye alag folder alag file me session safe kare. Aur
//    auto-reconnecter — server band hone aur wapas aane pe jo reconnect karta
//    hai (yaani jo /pair wale folder ko bhi check karta hai) — ab dono folders
//    check kare: /pair wala folder + ye naya /code?phone wala folder."
//
// ISOLATION: do alag folder + do alag file.
//   /code?phone=  → DEVICE ROW/KEYS : nexstore/local/local.db       (alag FILE)
//                   MARKER          : nexstore/local/pairing/<jid>/localonly
//   /pair (fleet) → DEVICE ROW/KEYS : nexstore/goldmd.db             (untouched)
//                   MARKER          : nexstore/pairing/<jid>/        (untouched)
//
// Fleet system (fleet.go, fleet_local.go, Storj blob/claim/set, /pair handler)
// ko ye file HAATH NAHI LAGATI — sirf reconnector scan list me local folder
// jorta hai taake restart pe direct-pair sessions bhi wapas connect hon.
// ============================================================================

import (
	"context"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// localMarkerRel: local-only marker file ka naam (pairing folder ke andar).
const localMarkerRel = "localonly"

// localDataDir: local-only store ka root (alag folder).
func localDataDir() string {
	return envOr("GOLDMD_LOCAL_DIR", filepath.Join(envOr("GOLDMD_DATA_DIR", "nexstore"), "local"))
}

// localDBPath: local-only sessions ki ALAG sqlite file.
func localDBPath() string { return filepath.Join(localDataDir(), "local.db") }

// localPairingDir: local-only pairing marker folders ka ALAG folder.
func localPairingDir() string { return filepath.Join(localDataDir(), "pairing") }

// localWAContainer: local-only whatsmeow container (device rows + signal keys).
// main() boot pe openLocalStore() se set karta hai.
var localWAContainer *sqlstore.Container

// openLocalStore: local store (alag folder + alag file) kholo + schema
// upgrade. Boot pe ek hi baar. Fail pe nil — local-only features gracefully
// off rehte hain (fleet path isse bilkul untouched).
func openLocalStore() error {
	if err := os.MkdirAll(localDataDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(localPairingDir(), 0o755); err != nil {
		return err
	}
	c, err := sqlstore.New(
		context.Background(),
		"sqlite",
		"file:"+localDBPath()+"?_foreign_keys=on&_busy_timeout=5000",
		waLog.Noop,
	)
	if err != nil {
		return err
	}
	localWAContainer = c
	migrateLegacyLocalMarkers()
	return nil
}

// deviceContainerFor: JID ke hisaab se SAHI container — local-only JID ka
// device row local store me, fleet JID ka main container me. Dono nil-safe.
func (m *Manager) deviceContainerFor(jid string) *sqlstore.Container {
	if jid != "" && localWAContainer != nil && isLocalOnlyJID(localPairingDir(), jid) {
		return localWAContainer
	}
	return m.container
}

// localMarkerDirFor: JID ke liye local-only marker folder path.
func localMarkerDirFor(jid string) string {
	return filepath.Join(localPairingDir(), jid)
}

// localDeviceExists: kya is JID ka device row local-only store me hai?
func localDeviceExists(jid string) bool {
	if localWAContainer == nil || jid == "" {
		return false
	}
	user := fleetUserPart(jid)
	if user == "" {
		return false
	}
	all, err := localWAContainer.GetAllDevices(context.Background())
	if err != nil {
		return false
	}
	for _, d := range all {
		if d.ID != nil && d.ID.User == user {
			return true
		}
	}
	return false
}

// appendLocalOnlyJIDs: local-only pairing folder ke JIDs list me joro (dedup).
// Reconnector (AutoLoad / watchdog) ise use karta hai — restart ke baad
// /code?phone= sessions bhi wapas connect hon.
func appendLocalOnlyJIDs(users []string) []string {
	seen := map[string]bool{}
	for _, u := range users {
		seen[u] = true
	}
	entries, err := os.ReadDir(localPairingDir())
	if err != nil {
		return users
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jid := e.Name()
		if seen[jid] {
			continue
		}
		if _, err := os.Stat(filepath.Join(localPairingDir(), jid, localMarkerRel)); err != nil {
			continue
		}
		seen[jid] = true
		users = append(users, jid)
	}
	return users
}

// migrateLegacyLocalMarkers: purane (global) pairing dir me pade local-only
// markers ko naye local-only folder me copy karo — taake restart pe
// reconnector unhe naye store se utha le. Idempotent: sirf "localonly" marker
// wale folders dekhta hai, fleet session folders ko chhota bhi nahi karta.
func migrateLegacyLocalMarkers() {
	legacy := filepath.Join(envOr("GOLDMD_DATA_DIR", "nexstore"), "pairing")
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jid := e.Name()
		if _, err := os.Stat(filepath.Join(legacy, jid, localMarkerRel)); err != nil {
			continue // fleet session folder — leave it alone
		}
		dst := localMarkerDirFor(jid)
		_ = os.MkdirAll(dst, 0o755)
		_ = os.WriteFile(filepath.Join(dst, localMarkerRel), []byte("disk-only\n"), 0o644)
		InfoLog("LOCALSTORE: legacy local-only marker migrated: %s", jid)
	}
}
