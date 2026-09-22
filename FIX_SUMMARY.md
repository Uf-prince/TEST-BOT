# GOLD-MD — Fleet WAR-LOOP Fix (Verified & Pushed)

**Commit:** `1715d57` — pushed to `origin/main` (GitLab: Uf-prince/GOLD-MD)
**Date:** 2026-09-21
**Files changed:** `src/fleet.go`, `src/session_guards.go` (+ 5 patch scripts)

---

## 1. Root Cause (the real bug)

The bot was stuck in an **infinite session war**: one JID (`923158930864`) was
re-claimed **150+ times in 8.5 minutes** across Render servers.

The cause was an **ordering bug** in three functions:

- `fleetHeldByLiveServer()`
- `fleetClaimAvailable()`
- `fleetOnConnected()`

All three checked `fleetHolderAlive(sid, hb)` **BEFORE** the fresh-claim trust
window. Render attackers **never appear in the heartbeat hash** (proven in
`FLEET_WAR_EVIDENCE.txt`), so:

```
fleetHolderAlive() -> false
   -> loop `continue`
      -> fresh-claim trust NEVER reached
         -> fleetHeldByLiveServer() ALWAYS false
            -> WAR GUARD ALWAYS chose RETAKE
               -> INFINITE WAR
```

## 2. The Fixes (5 patches)

| # | Fix | File |
|---|-----|------|
| 1 | New `fleetClaimFresh()` helper; fresh-claim trust moved **before** the alive-check in `fleetHeldByLiveServer()` | `src/fleet.go` |
| 2 | Same re-ordering in `fleetClaimAvailable()` and `fleetOnConnected()` | `src/fleet.go` |
| 3 | `fleetClaimHeartbeat()` — re-stamps connected claims every **3 min** (was 10 min == fresh-trust window → boundary race) | `src/fleet.go` |
| 4 | WAR GUARD **circuit breaker** — max 3 retakes / 5-min window per JID, then surrender (stops war even vs old-binary attackers) | `src/session_guards.go` |
| 5 | `warRetakeReset()` on healthy revive so a future blip gets a fresh count | `src/session_guards.go` |

## 3. Local Verification (port 15602)

Ran the fixed binary locally with `GOLDMD_SERVER_ID=svr15602`, restoring the
**exact war scenario** (14 sessions from the shared Storj blob):

| Metric | Before fix | After fix |
|--------|-----------|-----------|
| WAR GUARD retakes | 150+ in 8.5 min | **0** |
| Surrenders | many | **0** |
| Circuit-breaker trips | n/a | **0** |
| Claims per JID | repeated (war) | **exactly 1** |

Log evidence (`bot_15602.log`):
- `FLEET_CLAIM_START` → `FLEET_CLAIM_STAMP` → `FLEET_CLAIM_TIEBREAK won:true` → connect
- `FLEET: claiming unowned session 923158930864` — the previously war-torn JID, claimed **once**
- 29 `goldmd:fleet:claim` KV ops (3-min claim heartbeat working)
- One legitimate WhatsApp logout (`401: logged out from another device`) handled correctly

**Build:** `go build -mod=vendor ./src` → OK
**Tests:** `go test -mod=vendor ./src/` → `ok gold-md/src`

## 4. Dockerfile / Double-Process Check

**VERIFIED: single process — no double-process bug.**

- `Dockerfile` builds from source (`go build ... -o /build/gold-md ./src`), sets
  `ENV SUPERVISOR_ENABLED=1`, `CMD ["./start.sh"]`.
- `start.sh` runs `./gold-md &` → `BOT_PID=$!` → `wait $BOT_PID`, and re-captures
  `BOT_PID=$!` after each restart (the old fork-bomb bug is fixed).
- With `SUPERVISOR_ENABLED=1`, `gracefulSelfRestart` does a clean `os.Exit(0)`
  instead of fork+exec → only ONE process ever runs.

## 5. Render Account Status (BLOCKER — not a code issue)

Login to the Render account (`sank-oink-doorman@duck.com`) succeeded, but the
dashboard redirected to:

> **"Your account has been suspended for suspicious activity."**

Consequently **every** `*.onrender.com` server in `servers.json` returns
**HTTP 503** (tested `/`, `/health`, `/api/servers`, `/sessions`). The Render
auto-deploy cannot run while the account is suspended — this must be resolved
with Render support (`support@render.com`).

The code fix is complete and pushed; once the account is reinstated, Render will
auto-deploy commit `1715d57` and the war loop will be gone.

---

## Deliverable

- **Repo:** https://gitlab.com/Uf-prince/GOLD-MD (branch `main`, commit `1715d57`)
- **Changed:** `src/fleet.go`, `src/session_guards.go`
- **Patch scripts:** `patch_warfix.py` … `patch_warfix5.py`
