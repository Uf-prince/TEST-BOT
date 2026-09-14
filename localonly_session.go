package main

import (
	"os"
	"path/filepath"
	"sync"
)

// ============================================================================
// GOLD-MD — DISK-ONLY SESSIONS (owner order)
// File: localonly_session.go
// ============================================================================
// OWNER ORDER: "Direct?phone= ye endpoint isko set kr yeh session disk pe
// safe kre storj me na jaye iske sessions na configration sab disk pe hona
// chahye... khabardar hr chize delet ho jaye no issue disk se session nai
// jane chahye... jab cmnds naye banaye ge to bot restart kre ga to session
// disk se delete nai hone chahye"
//
// CLARIFICATION (owner): "Configrations storj se ane chahye sessions disk
// pe continu" — CONFIG (settings/prefix/owner name) Storj se hi aati rahegi,
// sirf SESSION (credentials/device) disk pe rahega.
//
// DIRECT /code?phone=<digits> se pair hone wala session = LOCAL-ONLY:
//   • Device row + creds  → sirf nexstore/goldmd.db (local disk)
//   • Pairing marker      → nexstore/pairing/<jid>/ (local disk) + "localonly"
//     marker file jo bina in-memory session ke bhi JID ko pehchane
//   • Storj pe KABHI nahi jata: na fleet blob (goldmd:fleet:sess:<jid>),
//     na claim (goldmd:fleet:claim:<jid>), na sessions set, na per-JID
//     registry (goldmd:sessiondb:<sid>:jids), na whole-DB backup
//     (goldmd:sessiondb:<sid>:blob) — central guard SaveSessionDB me hi
//     lagta hai taake KOI bhi future code-path leak na kar sake.
//   • Bot restart → AutoLoad disk se uthata hai (fleet guards BYPASS —
//     purani stale claims isko rok nahi sakti)
//   • cleanupSession / war-guard / koi bhi purge path disk data ko HAATH
//     nahi lagata (khabardar rule)
// ============================================================================

// localOnlyMarkerFile: pairing folder ke andar marker — iska hona = ye JID
// disk-only session hai. File system hi source of truth hai (restart ke
// baad bhi in-memory LocalOnly flag zero hota hai, marker rehta hai).
const localOnlyMarkerFile = "localonly"

// localOnlyMu: marker stat cache ki race safety.
var localOnlyMu sync.Mutex

// writeLocalOnlyMarker: direct pairing ke waqt pairing folder me marker
// file bana do. Marker = ye session Storj pe kabhi nahi jayega.
func writeLocalOnlyMarker(pairingDir, jid string) {
	if pairingDir == "" || jid == "" {
		return
	}
	dir := filepath.Join(pairingDir, jid)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, localOnlyMarkerFile), []byte("disk-only\n"), 0o644)
}

// isLocalOnlyJID: kya ye JID ka session disk-only hai? (marker file check)
// In-memory Session ho ya na ho — ye check hamesha kaam karta hai.
func isLocalOnlyJID(pairingDir, jid string) bool {
	if pairingDir == "" || jid == "" {
		return false
	}
	localOnlyMu.Lock()
	_, err := os.Stat(filepath.Join(pairingDir, jid, localOnlyMarkerFile))
	localOnlyMu.Unlock()
	return err == nil
}

// anyLocalOnlyOnDisk: kya ISS server ke pairing dir me koi bhi disk-only
// session pada hai? Whole-DB Storj upload (SaveSessionDB) isi se block
// hota hai — kyunki DB me local-only device rows bhi hoti hain, aur unke
// creds Storj pe leak hona owner order ka violation hai.
func anyLocalOnlyOnDisk(pairingDir string) bool {
	if pairingDir == "" {
		return false
	}
	entries, err := os.ReadDir(pairingDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(pairingDir, e.Name(), localOnlyMarkerFile)); err == nil {
			return true
		}
	}
	return false
}

// localOnlyUploadGuardFn: main.go boot pe register karta hai — SaveSessionDB
// (whole-DB Storj upload) se PEHLE puchta hai: koi local-only session disk
// pe hai? Ha → upload CANCEL (session creds Storj kabhi nahi jayenge).
var localOnlyUploadGuardFn func() bool

// RegisterLocalOnlyUploadGuard: boot-time registration (main.go).
func RegisterLocalOnlyUploadGuard(fn func() bool) {
	localOnlyMu.Lock()
	localOnlyUploadGuardFn = fn
	localOnlyMu.Unlock()
}

// localOnlyUploadBlocked: SaveSessionDB ke andar ka central gate.
func localOnlyUploadBlocked() bool {
	localOnlyMu.Lock()
	fn := localOnlyUploadGuardFn
	localOnlyMu.Unlock()
	return fn != nil && fn()
}

// localOnlyCleanFleetKeys: direct pairing ke WAQT purani fleet keys saaf
// karo (sirf ye ek jagah Storj DELETE hota hai — session ke liye, owner
// order: local-only session Storj se GAYAB rehna chahiye). Ye tab zaroori
// hai jab number pehle fleet session tha (jaise is JID ka blob kisi doosre
// server ne claim kiya hua tha) — purana blob/claim mila to doosre server
// usi se connect karke war shuru kar denge.
//
// FIRE-AND-FORGET: pairing speed pe 0% asar. Config (settings:<jid>)
// SAFE rehta hai — owner order ke mutabiq config Storj me hi rahega.
func localOnlyCleanFleetKeys(jid string) {
	if jid == "" {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		// fleet-level keys (blob / claim / set / meta / fail-mark)
		_, _ = m.Redis.cmd("DEL", fleetClaimPrefix+jid)
		_ = m.Redis.setRem(fleetSessionsSet, jid)
		_ = m.Redis.setDel(fleetBlobPrefix + jid)
		_ = m.Redis.setDel(fleetMetaPrefix + jid)
		_ = m.Redis.setDel(fleetFailMarkPrefix + jid)
		// own-server jids registry (AutoLoad restore list) — local-only
		// session disk se hi milta hai, registry ki zaroorat nahi.
		_ = m.Redis.RemoveJID(jid)
	}()
}

// localOnlyOwnerConfig: pair-success pe owner CONFIG Storj me save karo
// (owner clarification: "configrations storj se ane chahye" — settings
// Storj pe rehti hain, sirf SESSION disk pe). fleetOnPairSuccess ka
// owner-setting hissa hi — blob/claim/set us raaste me nahi lagenge.
func localOnlyOwnerConfig(jid, owner string) {
	if jid == "" || owner == "" {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		m := fleetMgr
		if m == nil || m.Redis == nil {
			return
		}
		if existing := m.Redis.GetSetting(jid, "owner", ""); existing == "" {
			m.Redis.SetSetting(jid, "owner", normalizeJID(owner))
		}
	}()
}

// localOnlyDirEntries helper: nahi chahiye — AutoLoad seedha ReadDir karta
// hai; ye file sirf marker helpers + guard deti hai.
