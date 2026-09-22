# GOLD-MD · `.server` Command — Naya Format ✅

## Command
`.server` (aliases: `.servers` `.svr` `.svrinfo` `.serverinfo` `.session` `.sessions`)

## Exact output format (live)

```
*ACTIVE BOTS :❯ ❮ 3 ❯*
*ONLINE SERVERS :❯ ❮ 3 ❯*
*OFFLINE SERVERS :❯ ❮ 3 ❯*

*SERVER 1 ACTIVE 1/2*
*SERVER 2 ACTIVE 0/2*
*SERVER 3 ACTIVE 2/2*
*SERVER 4 OFFLINE*
*SERVER 5 OFFLINE*
*SERVER 6 OFFLINE*
```

## Rules
- **ACTIVE BOTS** = total live pairings (jitne bot active hain, sab servers ka sum).
- **ONLINE SERVERS** = kitne servers online hain.
- **OFFLINE SERVERS** = kitne servers offline hain.
- Online servers → `*SERVER {number} ACTIVE {paired}/{max}*` (number order).
- Offline servers → `*SERVER {number} OFFLINE*` (number order).
- Sab data **REAL** hai — `fleetScanAll()` har server ka `/health` live hit karta hai.

## Implementation
- `src/handlers_extra.go` → `CmdServerMenu()` + `fleetServersMenuText()`
- `src/fleet_server_menu_test.go` → unit tests (format + all-offline)
- Build: `CGO_ENABLED=0 go build -mod=vendor ./src/` ✅
- Tests: `TestFleetServersMenuTextFormat` ✅ `TestFleetServersMenuTextAllOffline` ✅

## Git
- Commit `efd5e82` (author: UMAR • FAROOQ <ufprince1@gmail.com>)
- Pushed to `Uf-prince/TEST-BOT` main ✅
- Vercel auto-deploy: **READY** ✅
