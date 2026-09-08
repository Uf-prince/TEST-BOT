# GOLD-MD Downloader Verification Report — Direct Download / Streaming Test
**Test date:** Sep 6, 2026 • **Method:** Live tests with exact production code paths (streamDownloadToFile, probeVideoMeta, fastRemuxFile, whatsmeow UploadReader-equivalent EncryptStream + streaming upload) • **Monitoring:** /proc RSS sampled every 100ms + disk delta

---

## 1. FINAL ANSWER (short)

**YES — direct download streaming is ALREADY IMPLEMENTED and WORKING in your bot.** All 5 downloaders (fb, tiktok, youtube, insta, apk) download to a **disk temp file** with a tiny fixed 64KB RAM buffer, and send via **UploadReader** which streams from disk. **RAM stays ~13MB flat even for a 144MB file.** The downloaders are NOT your crash problem.

---

## 2. Verified live numbers (real URLs, real APIs)

| Test | File size | Peak RAM (process) | Baseline RAM | RAM delta | Disk used | Time |
|---|---|---|---|---|---|---|
| TikTok video (tikwm API) | 5.4 MB | 13.0 MB | 6.9 MB | +6.0 MB | 5.4 MB (temp) | 3.5s |
| YouTube music video (WhiteShadow API, 360p) | 11.3 MB | 12.7 MB | 5.1 MB | +7.6 MB | 22.7 MB (dl+remux) | 7.0s |
| Facebook watch video (cobalt API) | 5.1 MB | 12.4 MB | 7.0 MB | +5.4 MB | 10.3 MB | 4.0s |
| Instagram reel 1080p (ytdlp metadata API) | 6.6 MB | 11.8 MB | 6.6 MB | +5.2 MB | 13.1 MB | 3.1s |
| **APK WhatsApp (XAPK)** | **144.0 MB** | **12.8 MB** | 6.6 MB | +6.1 MB | 144.0 MB | 2.8s |
| **APK Telegram** | **119.4 MB** | **12.6 MB** | 6.7 MB | +5.9 MB | 119.4 MB | 2.5s |
| **2 concurrent 144MB APKs** | 2 × 144 MB | **13.0 MB each** | ~6 MB | ~+7 MB each | 288 MB total | ~3s |
| ffmpeg remux (child proc, -c copy) | 11.3MB in → 11.4MB out | **1 MB** | — | — | +11.4MB | <2s |

**Key observation:** RAM is **FLAT regardless of file size** — 5MB file or 144MB file, peak RAM is the same ~12-13MB. This proves streaming works end-to-end.

---

## 3. Why it's safe (code evidence)

### Download: `streamDownloadToFile` (video-play.go:646)
```go
buf := make([]byte, 64*1024)          // fixed 64KB buffer, allocated ONCE
for {
    n, readErr := resp.Body.Read(buf) // read chunk
    file.Write(buf[:n])               // write to DISK temp file
}
```
Never accumulates. File goes to /tmp as `gold-md-download-*`.

### Send: `SendVideoFile` / `SendAudioFile` / `SendDocumentFile` (commands_loader.go:150-190)
```go
f, _ := os.Open(path)
resp, err := b.s.Client.UploadReader(ctx, f, nil, whatsmeow.MediaVideo)
```
- `UploadReader` → `cbcutil.EncryptStream` (32KB buffer → second temp file) → `rawUpload` (streaming POST with ContentLength, no full-body buffering) — all chunked, all disk-based.

### Probe/metadata: `probeVideoMeta` (mediautil.go)
ffprobe runs via exec on the file — no media in RAM.

### ffmpeg remux: `fastRemuxFile` (video-play.go)
`ffmpeg -c copy` file→file remux. Measured child-process RAM: **1MB**.

---

## 4. RAM budget on Render free (512MB)

| Component | RAM |
|---|---|
| Bot base + 1 session (idle) | ~37-43 MB (measured live: 43MB) |
| Each extra session | +10-15 MB |
| 1 download in progress | +5-8 MB (flat, any size) |
| ffmpeg remux | +1 MB |
| **3 sessions + 3 simultaneous downloads** | **~100-130 MB total** — well under 512MB ✅ |

---

## 5. The REAL risk on Render free (it's DISK + policy, not RAM)

1. **Disk is ephemeral** — temp files vanish on spin-down/restart (good: no cleanup needed; bad: nothing persists — but your session DB already backs up to Upstash every 10 min, so this is handled).
2. **Disk SPACE**: Render free gives a small ephemeral disk (shared container, roughly ~1-2GB usable). A 144MB APK × a few concurrent users = fine. But if 3 users each request 90MB videos at once = ~270MB+ temp + binary + DB — still OK, but worth watching.
3. **No download size guard exists!** `maxVideoMB = 90` is defined in video-play.go:66 but **never checked** — any size downloads. A 500MB-1GB file would fill disk and could kill the instance. This is the one actual gap found.
4. **Service-initiated traffic threshold** (Render docs, verified): "Render may suspend a Free web service that initiates an uncommonly high volume of traffic over the public internet — examples: invoking external APIs, transferring data to or from external object storage." Your bot does exactly this (Upstash + Storj + media CDNs). Heavy use could trigger suspension — this matches your past "crash" experience better than RAM does.

---

## 6. What changes would be needed (ONLY IF you approve — nothing edited)

**A. Add download size guard (small, safe fix)** — check Content-Length (or running downloaded bytes) in streamDownloadToFile; abort >90MB (or configurable GOLDMD_MEDIA_MAX_MB) with "File too large" message. Prevents disk-fill from huge files.

**B. Big-media → Storj link (your approved concept)** — for files >threshold: upload temp file to Storj (your existing 10-shard storage, already has PutObject streaming) and send a link. Disk usage then also returns to ~0 after upload. This also reduces WhatsApp CDN upload traffic (but adds Storj traffic — same Render traffic-suspension exposure).

**C. Nothing needed for RAM** — streaming is already correct. The OOM theory for downloaders is **disproven by live measurement**.

---

## 7. API status check (bonus, verified live)

| API | Status |
|---|---|
| WhiteShadow YouTube (whiteshadow-x-api.onrender.com) | ✅ works (some long videos fail with error 117 — upstream limit, falls back to loader.to) |
| loader.to YouTube fallback | ✅ works (long videos queue ~30-60s+ before ready) |
| cobalt (cobalt-api-ufprince.onrender.com) — FB | ✅ works |
| tikwm — TikTok | ✅ works |
| ytdlp-ufprince metadata — Instagram | ✅ works (must be a REAL reel URL; dead/fake shortcodes return {"success":false}) |
| apkcombo — APK | ✅ works (some apps 404/410 — e.g. Instagram/VLC pages blocked; WhatsApp + Telegram worked fine) |

---

## Verdict for your decision
- **Downloader commands are already stream-optimized. Pushing a "streaming fix" is NOT needed.**
- The only real gaps: (1) no size guard (disk-fill risk), (2) Render free traffic-suspension policy risk from heavy external transfers, (3) ephemeral disk means nothing persists (already solved via Upstash for sessions).
- Decision is yours: approve size guard + Storj link for big files, or leave as-is.
