# 🔰 AUTOMSG FIX — APPLIED REPORT

**Problem reported by owner:** `.automsg 06h05m06s once wait` bheja → bot ne **"Invalid time format"** reply kiya — chahe format bilkul sahi tha (`XXhXXmXXs`).

---

## Root Cause (asli bug)

`gold-cmds/automsg.go` ka purana `parseAutomsgDuration()` **trailing "s" pehle strip kar deta tha**:

```
input:  "06h05m06s"
strip:  "06h05m06"        ← s kha gaya
parse:  06h ✓  05m ✓  06 …unit letter nahi mila ✗
result: "Invalid time format"   ← HAR sahi input pe!
```

Seconds ke digits ke baad unit letter (`s`) pehle hi strip ho chuka tha, isliye parser kabhi `06h05m06s`, `02h30m00s`, `00h00m45s` — **koi bhi valid format accept nahi karta tha.** Bot ke paas autommsg ka timer start hona tha hi nahi.

## Fixes Applied (6)

### FIX 1 — Naya regex-based time parser
`^(\\d+)h(\\d+)m(\\d+)s$` — koi strip nahi, koi ambiguity nahi. Valid: `06h05m06s`, `2h30m0s`, `00h00m45s`. Invalid: `30m`, `2h30m` (missing units), `0h0m0s` (zero), `00h60m00s` (out of range). Bounds: h ≤ 8784 (~1 saal), m/s ≤ 59.

### FIX 2 — `.automsg status` subcommand (docs me tha, code me missing)
Live countdown dikhata hai (`NEXT FIRE IN: 00h29m48s`), active schedule ka poora snapshot. Paused schedule ke liye `SCHEDULE IS PAUSED (OFF)` + resume hint.

### FIX 3 — `.automsg stop` subcommand (docs me tha, code me missing)
File ke header docs me likha tha: "stop → cancel the active timer + safe-delete the config from the bot's MEMORY" — par `handleAutomsg` me `stop` ka dispatch tha hi nahi. Ab added: timer cancel + memory se config safe-delete + styled confirmation.

### FIX 4 — Delete number-reply hook (broken tha)
`.automsg delete` numbered list dikhata tha aur bolta tha "reply a number" — **par prefix-check ke wajah se bina prefix ka plain number (`2`) kabhi automsg handler tak pahunchta hi nahi tha.** Ab `AutomsgTryDeleteReply` hook handler.go me prefix-check se PEHLE wire kiya (settings/compress/yts/search hooks jaisa hi pattern). Sirf owner + 2-minute window ke andar.

### FIX 5 — Restart restore (broken tha — "persists across restart" ka jhootha promise)
Config bot memory (Storj) me save hota tha, par **restart ke baad koi re-arm nahi karta tha** — schedule chupchaap mar jata tha. Ab `manager.go` ke `*events.Connected` handler me `AutomsgRestoreSavedSchedules` wire kiya: har saved **repeat** schedule restart/reconnect pe automatically re-arm ho jata hai (background goroutine, 0 blocking, panic-guarded). `once` schedules restore nahi hote (one-shot hote hain — dobara arm karna delayed-send confusion deta).

### FIX 6 — Usage help me duplicate line
`.automsg off → turn off / pause` line help text me **do baar** aa rahi thi (owner ke screenshot me bhi dikhi). Dusra occurrence `.automsg status → live countdown` se replace kiya. + ek chhota pre-existing Sprintf arg-count mismatch bhi theek kiya (11 args → 10, extra arg silently ignore hota tha).

## Verification

- ✅ Unit tests (`gold-cmds/automsg_test.go`): user ka exact case `06h05m06s` + 8 valid + 13 invalid cases — **ALL PASS**
- ✅ `CGO_ENABLED=1 go build` (Dockerfile.ghcr path) — clean
- ✅ `go vet` — automsg.go clean (pre-existing autoreply.go warning untouched)
- ✅ Saari 5 prebuilt binaries same settings se rebuild (`go version -m` se verify ki):
  - `gold-md-cgofree` (linux/amd64, CGO_ENABLED=0, `-s -w`)
  - `gold-md-lowram` (linux/amd64, `-trimpath`)
  - `gold-md-bot-static` (linux/amd64, `-s -w`)
  - `gold-md-bot-arm64` (linux/arm64, `-s -w`)
  - `gold-md-freebsd` (freebsd/amd64, `-s -w`)
- ✅ Har binary me naye fix strings (`AUTOMSG STOPPED`, `restart-restore`) confirm

**Behavior change:** sirf bugfix — automsg ab waise chalega jaise docs me promise kiya gaya tha. Baaki sab commands untouched.
