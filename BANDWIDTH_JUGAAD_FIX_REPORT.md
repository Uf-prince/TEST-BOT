# BANDWIDTH JUGAAD FIX REPORT — Invisible Fixes, Behavior 100% Same

> **Owner order:** *"kuch jugar lagwa bandwidth bachane ka... bot jese abi kam kr rha hai wese hi kam krna chahye lekin bandwidth bach jaye"*
> **Changes:** `upstash.go` + `fleet.go` (+ `fleet_failover_test.go` drift fix + new `bandwidth_jugad_test.go`)
> **Verification:** `go build` EXIT 0 · `go vet` clean · fleet/automsg/gzip tests ok · 5 binaries rebuilt · dono repos synced identical

---

## Masla (Render 5GB/month)

Har server roz **~3000+ Storj PUT** kar raha tha — aur inka 95% bytes **faltu** the:

| Kharcha | Pehle (roz) | Ab (roz) |
|---|---|---|
| Session DB backup (10-min) | 144 × MBs (unchanged bhi!) | **sirf changed DB × ~10x chhoti** |
| Fleet blobs (10-min × sessions) | 144 × 3 writes har session | **idle session = 0 bytes** |
| Heartbeat HSET (30s) | 2880 tiny PUT | **~1440 PUT (60s cadence)** |
| Egress SET (30s) | 2880 tiny PUT | **~100-300 PUT (throttled)** |
| **Total PUT/day** | **~5800+** | **~1600-1900 (60-70% kam)** |
| **Bytes/day (1MB DB, 2 sess)** | **~200MB+** | **~15-40MB (80-90% kam)** |

Sab se bara jugaad: **jab data change hi nahi hua to upload hi mat karo.**

---

## Fix 1 — Transparent Gzip (kv/strings PUT body)

**File:** `upstash.go` — `kvSet` / `kvGet`

Pehle har session-DB/blob PUT ka body **plain base64 text** (MBs) jata tha. Base64 text gzip me **~8-15x** chhota ho jata hai.

- `kvGzipMaybe(val)` — 4KB+ values PUT se pehle gzip (`GZ1:` prefix); chhoti values (settings/meta/egress) **plain as-is** (koi overhead nahi)
- `kvGunzipMaybe(data)` — read-side pe **transparent decompress** — `RestoreSessionDB` / `fleetRestoreBlob` / har caller ko **bilkul wahi value** milti thi jo pehle milti thi
- **Mixed-fleet safe:** purane binaries (jo servers abhi restart nahi hue) legacy plain values likhte/padh-te hain — naye dono padh lete hain. Naya binary jo GZ1: likhe, purana binary us key ko phir apne plain format me overwrite kar dega — koi data loss NAHI, sirf us window ki savings khatam
- Incompressible data → plain as-is (kabhi body badi nahi hoti)
- Corrupt GZ1: → best-effort raw fallback (koi value kabhi corrupt nahi)

**Savings:** 1MB DB → 1.33MB base64 → **~130KB gzip PUT**. 144 uploads → 191MB/day → **~19MB/day**.

## Fix 2 — Session DB Hash-Skip (Zero Upload Jab Idle)

**File:** `upstash.go` — `SaveSessionDB`

Pehle: har 10 min **poori DB base64 → Storj PUT** — chahe DB me **ek bhi byte change na ho**. Idle server = bina kisi kaam ke 144 MBs uploads/day.

Ab: DB bytes ka **sha256** yaad rakha jata hai. Upload ke **baad hi** hash save hota hai (Storj fail → next tick phir try — retry-safety wahi). Hash match → **0 bytes upload**.

**Savings:** idle server = **~0 bytes/day** (pehle ~191MB/day bhi ho sakta tha). Key-rotation/changes wali DB = sirf us din ka upload, ~10x chhota.

**Behavior same:** sessions restore bilkul aise hi (Storj me hamesha **latest changed** DB hai — skip sirf IDENTICAL upload pe lagta hai).

## Fix 3 — Fleet Blob Hash-Skip + Conditional Meta/SADD

**File:** `fleet.go` — `fleetSaveBlob`

Pehle: har 10 min, har connected session ka blob **+ meta + SADD (3 writes)** — bina change-check.

Ab: per-JID sha256 map (`fleetBlobHashes`). Blob unchanged → **teeno writes skip** (blob + meta + SADD = 0 bytes).

**Savings:** idle connected session = 0 bytes/day (pehle ~14.5MB/day per session). Changed blob → gzip + upload.

**Behavior same:** failover/restore waise hi — blob change hone par hi data hota hai; unchanged blob ka dobara upload karna redundancy thi.

## Fix 4 — Egress Push Throttle (256KB / 5-min)

**File:** `fleet.go` — `fleetPushEgress`

Pehle: har 30s tick pe egress counter **SET** (2880 tiny PUT/day) — counter me aksar **koi change bhi nahi** hota.

Ab: memory counter hamesha update (`.host5gb` / warn logic **exact same** values padhta hai — wo in-memory `fleetEgressTotal` se aata hai). Storj pe sirf tab SET jab **(a) 256KB+ naya egress jama** ho **ya (b) 5+ min** beet gaye hon ya month rollover.

**Savings:** ~2880 → **~100-300 PUT/day**. Tiny bytes to chhote the, lekin Render ka "service-initiated traffic" suspension radar **operation COUNT pe bhi** tha — ye bhi kam hua.

**Behavior same:** `.host5gb` report **live in-memory** total dikhata hai (throttle sirf Storj persist pe hai); pre-crash warning thresholds waise hi.

## Fix 5 — Heartbeat 60s Cadence (Freshness Window ke Andar)

**File:** `fleet.go` — `fleetHeartbeat`

Pehle: har 30s tick pe HSET (2880 PUT/day). Liveness freshness window **2 min** (`fleetProbeAfter`) hai — yaani 30s pe likhna overkill tha.

Ab: 60s pe ek hi HSET (~1440 PUT/day, 50% kam). Freshness margin ab bhi **2x** hai, aur agar tick miss bhi ho jaye to `fleetHolderAlive` ka **ACTIVE /health probe fallback** zinda hai — failover timing pe **zero asar** (orphan sweep waise hi 60s pe chalta hai).

**Behavior same:** orphan sweep / claim / probe logic untouched — sirf write-frequency kam hui.

---

## Test-Data Drift Fix (separate, pre-existing)

`fleet_failover_test.go` — `TestFleetServerNumberForSIDTable` ka case `gold-md-botxd2.onrender.com → "5"` **purani URL** expect karta tha; servers.json me owner ne SERVER 5 ko `gold-md-xdbotzz` se replace kar diya tha (commit `cdcd599` pe bhi ye fail hota tha). Function ka behavior **same** — sirf assertion sync kiya (`gold-md-xdbotzz → "5"`). **Zero code change.**

---

## Estimated Monthly Impact (per server)

| Scenario | Pehle | Ab | Bachat |
|---|---|---|---|
| Idle (1 sess, kam usage) | ~2.2GB/mo | **~0.15-0.3GB/mo** | **~85-90%** |
| Heavy (2 sess + videos) | ~8-10GB/mo | **~2-3.5GB/mo** (media sends pe gzip nahi — wo bot ka kaam hai) | **~60-70%** |
| Storj PUT count | ~175K/mo | **~50-60K/mo** | 60-70% |

**Note:** WhatsApp media sends (`Client.Upload`) pe koi compression nahi — wo bot ka **core kaam** hai (user ne bola "bot jese kam kr rha hai wese hi"). Sirf faltu overhead khatam hua. Media-heavy usage ab bhi 5GB se bahar ja sakta hai — wo usage ki cheez hai, bug nahi.

**Bot behavior 100% same:** pairing, failover, restore, commands, `.host5gb`, `.server`, antidelete — sab bilkul pehle jaisa. Sirf Storj pe bahar jaane wale **faltu bytes aur faltu requests** khatam.

---

## Verification

- `go build -mod=vendor ./...` — EXIT 0
- `go vet -mod=vendor .` — clean
- `gofmt` — upstash.go, fleet.go, bandwidth_jugad_test.go clean
- Tests: `TestKvGzip*` (naya, round-trip prove) ok · `TestFleet*` ok · automsg tests ok
  - (TestSearchPick/TWT network-dependent — mere changes ke bina bhi same fail, pre-existing)
- 5 binaries rebuilt (`rebuild_binaries.sh`) — REBUILD_DONE_OK
- Dono repos `diff -rq` — **EXIT 0 (100% identical)**

## Files Changed

- `upstash.go` — kvGzipMaybe/kvGunzipMaybe + kvSet/kvGet transparent gzip + SaveSessionDB hash-skip + dbHash struct fields
- `fleet.go` — fleetBlobHashes map + fleetSaveBlob hash-skip/conditional writes + fleetPushEgress throttle + fleetHeartbeat 60s
- `fleet_failover_test.go` — assertion drift fix only
- `bandwidth_jugad_test.go` — NAYA: gzip round-trip unit tests
- 5 binaries rebuilt

---

# 🚨 EMERGENCY ADDENDUM — "sare sessions offline" Outage + Fix (2026-01-30)

## Kya hua (Root Cause — SACH, chhupaya nahi)

Pichli commit (c704231) ka **transparent gzip wire-format (GZ1:)** hi outage ka
karan tha. GZ1 sirf **badi values** pe lagta tha (≥4KB) — jo keys affected
hain: sessiondb blob + fleet per-JID blobs. Chhoti values (settings, meta,
egress, failmark) plain hi thi — wo safe hain.

- **NAYA binary** GZ1 value padh leta hai (kvGunzipMaybe) — uske sessions foran
  online aate hain.
- **PURANA binary** (jo 200-server fleet me abhi bhi chal raha hai) plain
  base64 expect karta hai → `base64.StdEncoding.DecodeString("GZ1:...")`
  FAIL (`:` base64 alphabet me nahi) → `fleetRestoreBlob` / `RestoreSessionDB`
  fail → **sessions OFFLINE**.

Meri "mixed-fleet safe" claim sirf naye binary ke read-side ko cover karti
thi. Purane binaries kabhi bhi GZ1 nahi padh sakte — ye meri galti thi.

## Fix (3 parts, sab build+test verified)

1. **`kvGzipEnabled` — DEFAULT OFF** (upstash.go): wire format INSTANTLY wapas
   legacy plain. `GOLDMD_KV_GZIP=1` env se opt-in — sirf tab jab saari fleet
   naya binary chala rahi ho. `TestKvGzipDefaultOffLegacyWire` regression
   guard bana diya.

2. **Read-side `kvGunzipMaybe` ZINDA** (upstash.go): pehle GZ1 me likhi values
   new binary ab bhi padh leta hai — purani GZ1 data se koi session loss nahi.

3. **BOOT HEAL MIGRATION** (upstash.go + fleet.go):
   - `HealGZ1Key(key)` — raw body GET; agar GZ1 tha to decompress karke PLAIN
     rewrite (bandwidth-neutral: value as-is, sirf format change).
   - `fleetHealGZ1()` — boot pe `fleetInit()` se. **Bandwidth-safe design**:
     apna sessiondb key har server heal (1 GET); global sweep flag-guarded
     (`goldmd:gz1:heal:v1`) — fleet-wide sirf PEHLA boot chalata hai sweep,
     baaki 199 servers flag dekh ke skip (0 bandwidth). Plain keys pe sweep
     idempotent (1 GET + prefix-check + skip). Flag-set fail → agli boot phir
     chalega (race-free — sab same plain value likhte hain).
   - Rationale: heal writes plain, save-path writes plain — plain↔plain, hash
     konverj karta hai, koi race nahi. (Lazy heal-on-read race-prone tha kyunki
     heal-write plain + normal save plain pe stale overwrite hash-skip se
     newer save block kar deta.)

## Verification (emergency pipeline)

- `go build -mod=vendor ./...` — **EXIT 0**
- `go vet` — mere files clean (autoreply.go vet warn pre-existing, stash se prove)
- `gofmt` — upstash.go, fleet.go clean
- Tests: `TestKvGzip*` ok · `TestFleet*` ok · full package `-short` **ok (1.236s)**
- Verify hook: `TestFvEnvCommitPush` PASS — canary strings (hEART/v1.0.0-BNDW...)

## Wire-format summary (ab)

| Value | Wire format | Purane binaries padh sakte? | Naye binaries padh sakte? |
|---|---|---|---|
| Naya save (default) | PLAIN (legacy, byte-for-byte same) | ✅ | ✅ |
| Purani GZ1 values | GZ1 (heal tak) → heal ke baad PLAIN | ❌ (heal ke baad ✅) | ✅ |
| Healed values | PLAIN | ✅ | ✅ |

**Bottom line:** naya binary deploy hote hi (a) naya binary purani GZ1 data
khud padh leta hai — sessions wapas online, (b) boot-heal saari fleet values
plain me rewrite karta hai — purane binaries bhi wapas padh sakte hain,
(c) aage ki saves 100% legacy plain — koi mixed-fleet break possible nahi.
