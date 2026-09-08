# GOLD-MD Bot — RAM / Disk / Render Free Tier Analysis Report

**Date:** 6 Sep 2026 · **Test env:** Same binary, same code, live 1-session measurement
**Bot:** GOLD-MD (Go + whatsmeow) · Render deployment: 3 instances (servers.json)

---

## 1. MEASURED NUMBERS (LIVE, 1 SESSION ACTIVE)

| Metric | Value | Note |
|---|---|---|
| RAM (idle, 1 WhatsApp session live) | **37.4 MB** (RSS) | GC settle ke baad; peak 51 MB activity pe |
| RAM (fresh start, 0 sessions) | ~26.6 MB | Binary load + init |
| **Per-session marginal RAM** | **~11–25 MB steady** | Idle state mein |
| Estimated RAM — 3 sessions idle | **~70–90 MB** | 512 MB limit ke andar bilkul safe |
| CPU (idle + watchdog) | **0.5–2.2%** | Watchdog har 15s (repo author ne khud 0.1 CPU pe ~0.2% measure kiya tha) |
| Threads | 9 | |
| Virtual memory (VmSize) | 1.8 GB | Go runtime — normal, RSS hi asli hai |
| Session DB (goldmd.db, 1 session) | **688 KB** | SQLite, nexstore/ mein |
| Binary size | 28.9 MB | gold-md-bot |
| Repo + vendor | 75 MB | Build context |

**Verdict: RAM idle usage kabhi bhi crash ka waja NAHI.** 3 sessions ~90 MB — 512 MB ka sirf 18%. Masla kahin aur hai (neeche).

---

## 2. RENDER FREE TIER — OFFICIAL LIMITS (render.com/docs/free se verified)

| Limit | Value | Aap ke bot pe asar |
|---|---|---|
| RAM | 512 MB | Idle: safe. **Media processing: OOM danger** |
| CPU | 0.1 CPU | Sticker/video/doc conversion slow |
| **Spin-down** | **15 min bina inbound HTTP/WS** | **Sessions + DB wipe — SABSE BADA MASLA** |
| Filesystem | **Ephemeral** (restart/redeploy/spin-down pe local files LOST) | SQLite session DB gayab |
| Instance hours | **750/month per workspace** | 1 service 24/7 = 720h (fit). **3 servers 24/7 = 2160h (FAIL)** |
| Outbound bandwidth | 100 GB/month (free) | Media uploads/downloads isse khaate hain |
| **Service-initiated traffic** | High volume pe **SUSPEND** | **Storj S3 + Upstash REST = trigger candidates** |
| Persistent disk | Free pe NAHI | Session survive ka rasta nahi |

---

## 3. CRASH KI ASAL WAJAYEIN (PRIORITY ORDER — CODE EVIDENCE KE SATH)

### Wajah #1 — Spin-down + Ephemeral Disk = Session Data Wipe (CRITICAL)
Render free service **15 minute idle pe band ho jati hai** aur spin-down/restart pe **local filesystem DELETE ho jata hai**. Aap ki sessions `nexstore/goldmd.db` (SQLite) mein local disk pe hain. Matlab: raat ko traffic nahi → service spin-down → subah DB gayab → saare users ka session wiped → sab dobara pair karein. Yeh "crash" lagta hai jabke asal mein **data loss** hai. Bot code mein Storj backup infra maujood hai lekin session DB (auth store) sirf local SQLite mein rehti hai — Storj sirf antidelete media backup ke liye hai.

### Wajah #2 — Media Processing RAM Spikes → OOM Kill (HIGH)
Code mein teen jagah poori file memory mein load hoti hai:
- `antidelete_handler.go:1031` → `s.Client.Download()` — poori media RAM mein (1x)
- `storj.go:247` → upload ke waqt `bytes.Buffer` mein meta+raw copy (2x)
- `storj.go:346` → retrieval pe `body.ReadFrom(obj)` + `proto.Unmarshal` (2x)

**Math:** 50 MB video = download 50 MB + buffer copy 50 MB = ~100–150 MB peak. 2–3 concurrent media messages (3 busy groups) = **300–450 MB spike → 512 MB se cross → OOM kill (exit 137)**. Yehi wo "crash" hai jo aap dekh rahe ho jab media zyada chal raha ho.

### Wajah #3 — Service-Initiated Traffic Suspension (MEDIUM-HIGH)
Render free pe agar service **bahar ki taraf high volume traffic** kare to Render usay **suspend** kar deta hai — examples mein explicitly likha hai: *"Transferring data to or from external object storage"*, *"Accessing an external database"*, *"Invoking external APIs"*. Aap ka bot:
- Har antidelete media pe **Storj S3 upload** (object storage)
- Har 2 minute pe **Upstash REST refresh** (`upstashRefreshInterval = 2 * time.Minute`, upstash.go:102)
- aivideo/external API calls

Busy groups + 3 sessions = Storj bandwidth bottle-neck → Render suspension. Yeh bhi "crash" jaisa lagta hai (service hi gayab).

### Wajah #4 — 750 Instance Hours vs 3 Servers (MEDIUM)
`servers.json` mein 3 Render instances hain. Har ek 24/7 chalayi to 3 × 720 = **2160 hours vs 750 limit**. Month ke 10-11 din mein saari free services suspend. Agar sirf 1 instance 24/7 chahiye to 720h — limit ke andar. **3 free servers ek hi workspace/account se 24/7 mumkin NAHI** — 3 alag accounts chahiye.

### Wajah #5 — Docker Image Heavy: LibreOffice + ffmpeg (LOW-MEDIUM)
Runtime image mein `libreoffice-writer/calc/impress + ffmpeg + python3 + poppler` hai. LibreOffice ek conversion pe **200–400 MB RAM** khata hai — 512 MB pe single doc conversion bhi OOM kar sakti hai, aur image size se build pipeline minutes zyada khaate hain.

---

## 4. KYA THEEK HAI ABHI (CONFIRMED)

- MAX_SESSIONS=3 — 512 MB idle budget ke hisaab se sahi decision
- Watchdog 15s — 0.1 CPU pe measured ~0.2% CPU, negligible
- Upstash 2-min refresh — light hai (single REST call), traffic suspension ka risk Storj media se zyada hai, Upstash se kam

---

## 5. FIXES (ACTIONABLE)

1. **Spin-down ko roko:** cron-job.org ya UptimeRobot se har 10 min `/health` pe GET ping (external inbound ping allowed hai — yeh inbound traffic hai, service-initiated nahi). Spin-down nahi hoga to DB wipe bhi nahi.
2. **Session DB ko survivable banao:** Free tier pe disk nahi hoti — DB ko Storj/Upstash pe sync karo (infra already code mein hai) ya ek service ko paid Starter ($7) pe le jao jahan disk attach ho. Ya 3 servers ko 3 alag free accounts pe banao.
3. **Media OOM fix:** `Client.Download()` ki jagah `DownloadTo()` (stream to disk) + storj upload pe `io.Pipe()` streaming — 2x copy khatam. Media size pe cap laga do (e.g. >25 MB skip).
4. **Traffic suspension se bacho:** Storj uploads ko batch/rate-limit karo, ya antidelete media backup sirf groups mein enable karo jahan zaroori ho.
5. **Image slim:** LibreOffice ko optional banao — agar doc-conversion commands mostly unused hain to runtime image se hata do; RAM spike + build time dono kam.
6. **3 servers 24/7 chahiye to:** 3 alag Render accounts (alag emails), har account apne 750h ke sath — aur har server pe ping setup.

---

## 6. LOCAL TEST SETUP (CURRENT)

- Bot: PID running, port 11246, 1 session (`923158930864@s.whatsapp.net`) online
- Tunnel: https://induction-streams-rebound-round.trycloudflare.com (live)
- Local sandbox pe RAM/disk limits nahi hain — yeh numbers Render ke 512 MB/0.1 CPU pe apply honge

**Bottom line:** Bot ki coding idle RAM mein bilkul light hai (3 sessions ~90 MB). Render crash ki asal wajayein: **(1) 15-min spin-down pe session DB wipe, (2) media processing ke 2x RAM spikes, (3) Storj/Upstash external traffic suspension, (4) 750h/month vs 3 servers ka math.**
