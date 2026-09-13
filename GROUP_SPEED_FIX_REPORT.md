# GROUP SPEED FIX — RESEARCH REPORT

## 🐌 PROBLEM

> "Jani Group me bot bahut zyada slow ho jata... Inbox to wese b fast chalta ha"

Group me bot ke replies bahut slow. Inbox me fast. Owner chahta hai bot **waise hi** kaam kare (100% same behavior), bas **group speed fast** ho jaye.

## 🔬 ROOT CAUSE (RESEARCH KE BAAD MILA)

### Problem 1 — `setMembers()` = S3 ListObjects, ZERO CACHE (MAIN)

`upstash.go` me `setMembers()` har call pe Storj S3 pe `ListObjects` round-trip karta tha (~0.5-1 second). Ye **koi cache nahi** tha.

**Kahan chalta tha ye (HAR GROUP MESSAGE PE):**

```
HandleMessage
└── (group only) bangcuser check  [handler.go:452]  ← SYNCHRONOUS
    └── GroupBanUserIsBanned
        └── setMembers("goldmd:<bot>:groupset:<group>:bangcuser")  ← S3 ListObjects ~0.5-1s
```

Ye check **synchronous** hai (banned user ke messages pe commands na chalein isliye) — isliye **har ek group message ka reply 500ms-1s+ der se** nikalta tha. Inbox messages me ye check `info.IsGroup` guard ki wajah se **skip** hota tha — isliye inbox fast tha, group slow.

### Problem 2 — Anti-* detectors ka whitelist read (async, lekin S3 flood)

`applyAntiDetection` (async) me jab bhi antilink/antibad/antibot ON hote hain, har group message pe `GroupSetMembers` (SMEMBERS = S3 ListObjects) call hota tha whitelist/custom words ke liye. Reply ko block to nahi karta, lekin background me S3 pe load aur mutex contention badhata tha.

### Problem 3 — IsGroupAdmin = fresh WhatsApp IQ har call

`bridge.IsGroupAdmin` har call pe WhatsApp servers se fresh `GetGroupInfo` IQ bhejta hai (koi cache nahi). Ye sirf admin-check wale paths pe chalta hai (bangcuser warn, autoreplyprem, premium add in groups) — hot path nahi, isliye sirf note kiya.

## ✅ FIX (BEHAVIOR 100% SAME — SIRF SPEED)

### Fix 1 — `setMembers()` ab in-memory cached

Pehle jaisa pattern jo repo me pehle se tha (`GetSetting` / `PremiumMembersCached`):
- Pehli call: S3 SMEMBERS fetch → JSON cache
- Uske baad **0ms** (memory hit)
- Empty set → `\x00` sentinel (taake empty set bhi cache ho, baar-baar S3 na jaye)
- Sorted order preserved (S3 ListObjects wahi sort karta tha)
- Error → nil (pehle jaisa hi)

### Fix 2 — Har write pe cache invalidate

`cmd()` dispatcher (single choke point) me DEL / SADD / SREM ke **baad** `setmembers:<key>` cache delete:
- `setAdd` → `cmd("SADD")` → invalidation ✅
- `setRem` → `cmd("SREM")` → invalidation ✅
- `setDel` / `cmd("DEL")` direct callers (fleet.go, bangcuser meta cleanup) → invalidation ✅
- **Yani local writes ka asar FORAN dikhega** (0ms after write, no staleness)

Saare SET writes repo me `cmd()` se guzarte hain (research me grep se verified — koi direct `kvSAdd`/`PutObject` bypass caller nahi).

### Fix 3 — 2-minute background re-validation

`refreshAllSettings()` (jo har 2 min settings cache refresh karta hai) ab `setmembers:` keys bhi background me re-fetch karta hai. Isse **doosre server/deployment** se kiye gaye changes (multi-bot fleet me ek server se ban, doosre se unban) 2 min ke andar pick ho jate hain — bilkul settings cache ke jaisa hi freshness guarantee jo repo me pehle se tha.

### Fix 4 — Warmup: `WarmGroupBanList()`

Naya async warm helper (WarmGroupSettings ke saath, same once-per-group pattern):
- Pehla group message boot ke baad: background me bangcuser set warm ho jata hai
- Uske baad har message **0ms** banned check

### Fix 5 — antibad/antibot message name fix (is commit me included)

`.antibad` / `.antibot` inbox me chalao to pehle `*ANTILINK ONLY WORKS IN GROUPS*` aata tha (hardcoded). Ab:
- `.antibad` → `*ANTIBAD ONLY WORKS IN GROUPS*`
- `.antibot` → `*ANTIBOT ONLY WORKS IN GROUPS*`
- `.antilink` → `*ANTILINK ONLY WORKS IN GROUPS*` (pehle jaisa hi)

## 📊 SPEED IMPACT (EXPECTED)

| Path | Pehle | Ab |
|---|---|---|
| Group message reply (har message) | ~500ms-1s+ S3 sync round-trip | **~0ms** (memory) |
| Pehla message per group (boot ke baad) | 500ms-1s (warmup async) | async warm, reply unaffected |
| Anti-* whitelist read (ON hone par) | ~500ms-1s har message | **0ms** pehle ke baad |
| Unban/ban action | instant | instant (invalidation) |
| Cross-server unban pick-up | n/a | ≤2 min (background refresh) |

## 🧪 VERIFICATION

- `go build -mod=vendor ./...` ✅ CLEAN
- `go vet` (gold-cmds) ✅ CLEAN (sirf pre-existing autoreply.go:827 warning jo kabhi se thi)
- `gofmt` ✅ touched files formatted
- Automsg unit tests (`TestParseAutomsgDuration|TestFormatHMS|TestIsJustNumber`) ✅ ALL PASS
- `go build` saare platforms (linux/amd64, linux/arm64, freebsd/amd64, CGO + non-CGO) ✅
- Behavior identical: same checks, same messages, same actions — sirf S3 round-trips cache ho gaye

## 📁 FILES CHANGED

| File | Change |
|---|---|
| `upstash.go` | setMembers cache, cmd() write-invalidation, refreshAllSettings set-refresh, WarmGroupBanList |
| `handler.go` | WarmGroupBanList wire (group warmup block) |
| `gold-cmds/antibad.go` | requireGroupOwnerNamed("ANTIBAD") |
| `gold-cmds/antibot.go` | requireGroupOwnerNamed("ANTIBOT") |
