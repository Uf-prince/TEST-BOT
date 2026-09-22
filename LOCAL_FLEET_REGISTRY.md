# GOLD-MD — LOCAL FLEET REGISTRY (owner order, 2026-09-21)

## Owner order (Roman Urdu, verbatim intent)
> "Agar yh session ki info claim servers ki info hamesha local file me se read honi
> chahiye. Agar folder na mile to setobara storage se ek bar load kr lo fir bar bar
> nai. Ek guard bithao jo us folder ke sessions ko online check krta rahe live har
> waqt — jese hi offline ho fleet walo ko info kr de k ye session off hai, iska
> claim dusre server ko do, wo fir claim kre ek bar."

## What was built

A **local-file-first fleet registry** so that session info + claim-holder server
info + server heartbeat/URL info is ALWAYS read from a local JSON file, with a
one-time Storj bulk-load fallback (guarded by a marker so it never repeats).

### New file: `src/fleet_local.go`
- **Local registry**: `nexstore/fleetlocal/registry.json` (atomic tmp+rename writes).
  - `flSession{JID, ServerID, ServerURL, ClaimTS, Online, Updated}`
  - `flServer{SID, URL, TS, Sessions, Max}`
  - `flRegistry{Self, SelfURL, Updated, Sessions, Servers}`
- **`flInit()`** (boot):
  1. Load from disk if `registry.json` exists → **0 network**.
  2. If disk missing AND no `_loaded` marker → **one-time** Storj bulk-load, then
     write `_loaded` marker → **never again** ("bar bar nai").
- **`flStartRefresher()`**: 60s MERGE-refresh — only changed entries pulled from
  Storj; local is never wiped; own fresh claim is never overwritten.
- **`flStartOnlineGuard(m)`**: 15s ticker over the local pairing folder.
  - live → `flSetOnline(jid, true)`
  - offline transition (was online) → `flNotifyOffline(jid)`
- **`flNotifyOffline(jid)`**: local `online=false` + release own claim (Storj HDEL
  + local remove) + fail-marker + `FLEET_LOCAL_OFFLINE_NOTIFY` JSON debug. The
  watchdog/resurrector then dispatches the JID to a FREE server → it claims ONCE.
- **`flMirrorKV(args)`**: central write-through hook (called from `cmdCore`) that
  mirrors every fleet KV write (claim HSET/HDEL/DEL, servers HSET, sessions
  SADD/SREM) into the local registry — no per-call-site changes needed.

### Modified files
| File | Change |
|------|--------|
| `src/fleet.go` | `flInit()` in `fleetBind`; `go flStartRefresher()` in `fleetInit`; local-first reads in `fleetHeartbeatMap`, `fleetClaimHolders`, `fleetServerURL`; `flSetServer` in `fleetHeartbeat` |
| `src/storage.go` | `flMirrorKV(args)` after `dcApply` in `cmdCore` |
| `src/session_watchdog.go` | `flSetOnline(jid,true)` on live; `flNotifyOffline(jid)` on offline |
| `src/resurrector.go` | `flSetOnline(jid,true)` on live; `flNotifyOffline(jid)` on offline |
| `src/main.go` | `flStartOnlineGuard(mgr)` after watchdog start |

### Tests: `src/fleet_local_test.go`
`TestFlParseHeartbeat`, `TestFlClaimRoundTrip`, `TestFlServerRoundTrip`,
`TestFlOnlineAndRemoveClaim`, `TestFlDiskPersistence`, `TestFlMirrorKV`,
`TestFlNotifyOfflineLocal` — all PASS.

## Verification (end-to-end, local run)
1. **First boot** (empty dir): `FLEET-LOCAL: registry gayab — Storj se EK BAAR
   bulk-load...` → `registry.json` created with 13 sessions + 1 server; `_loaded`
   marker written.
2. **Restart**: `FLEET-LOCAL: ready (sessions=13, servers=1)` — loaded **directly
   from disk**, **no** Storj bulk-load (grep count = 0). ✅ "bar bar nai".
3. **Online guard**: offline transitions logged
   `FLEET_LOCAL_OFFLINE_NOTIFY {action: claim-released-notify}` → claim released
   for another server to claim once. ✅
4. `go build` OK, `go vet` clean, full `go test ./src/` PASS.

## Env knobs
- `GOLDMD_FLEET_LOCAL` (default `1`) — enable/disable local registry.
- `GOLDMD_FLEET_LOCAL_DIR` (default `$GOLDMD_DATA_DIR/fleetlocal`).
