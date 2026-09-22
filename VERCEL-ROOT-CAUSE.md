# GOLD-MD · Vercel — Root Cause + FIX (SOLVED ✅)

**Date:** 2026-09-19 ~07:25 UTC
**Repo:** `Uf-prince/TEST-BOT` (private, `main`)
**Vercel project:** `umar-a4a6/gold-md-xv-botz`

---

## ✅ STATUS: FIXED — Vercel ab FRESH servers serve kar raha hai

```
$ curl https://gold-md-xv-botz.vercel.app/api/servers
SERVER 1 | https://gold-md-xsvrr36.onrender.com | online=true   ← FRESH ✅
SERVER 2 | https://gold-md-xsvrr37.onrender.com | online=true
SERVER 5 | https://gold-md-xsvrr38.onrender.com | online=true
```
GitHub commit status: `Vercel -> success` ✅

---

## 🎯 ASLI WAJAH (2 problems — dono mil gaye)

Vercel auto-deploy **CHAL RAHA THA** (har push pe deployment banta tha), lekin
**build FAIL** hota tha → Vercel **last successful (purana)** deployment serve
karta rehta tha → isi liye **old servers** dikhte the.

### Problem #1 — `vercel.json` me invalid property
Vercel ka exact error:
```
The `vercel.json` schema validation failed with the following message:
should NOT have additional property `public`
```
`vercel.json` me `"public": true` likha tha — ye **invalid** hai. Vercel ne schema
strict kar diya, is liye **har build fail** hone laga (pehle chal jata tha).

**Fix:** `"public": true` line hata di.

### Problem #2 — Commit author Vercel account se linked nahi
Vercel ka exact error:
```
The deployment was blocked because Vercel couldn't find a Git account
for the commit author.  (blockCode: COMMIT_AUTHOR_REQUIRED)
```
Vercel Hobby plan **commit author ka email** check karta hai — wo email kisi
Vercel account se match hona chahiye. Meri commit `bot@local` se thi → block.

**Fix:** Commit sahi author (`UMAR • FAROOQ <ufprince1@gmail.com>`) se ki.

---

## 📊 Evidence (Vercel API se — verified)

| Deployment | Commit | State | Reason |
|---|---|---|---|
| dpl_7SZfSe44... | `7e3225b` | ✅ **READY** | FIXED |
| dpl_2VtuGCNL... | `666338d` | ⛔ BLOCKED | COMMIT_AUTHOR_REQUIRED |
| dpl_AJHAjRXN... | `420b949` | ❌ ERROR | invalid `public` property |
| dpl_Ac2KaQfM... | `3f31d71` | ❌ ERROR | invalid `public` property |
| dpl_Uoj4HWii... | `7701e8d` | ✅ READY | (purana, chal gaya tha) |

---

## 🔑 Bonus: Tumhara "Render wala" URL asal me Vercel build NAHI hai

`gold-md-by-umar.vercel.app` = **PROXY** jo Render bot ko forward karta hai:
- Header: `x-render-origin-server: Render`
- `/health` → `sid: gold-md-xsvr55.onrender.com` (Render bot)
- Source: `OTHER-URL-CHNG-2-VRCEL-URL/api/proxy.js` → `BACKEND = "https://gold-md-xsvr55.onrender.com"`

→ Isliye wo hamesha fresh tha (Render build succeed hota hai). Mera URL asli
static build hai — ab wo bhi fresh hai.

---

## 🛡️ Aage ke liye (permanent)

1. `vercel.json` ab valid hai — future pushes pe build chalega.
2. Commit author hamesha `ufprince1@gmail.com` rakho (Vercel-linked account).
3. Bot ka `.svrchange` command already sahi author se commit karta hai ✅
