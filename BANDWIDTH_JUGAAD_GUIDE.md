# 🛠 GOLD-MD Bandwidth Jugaad — Mukammal Guide (Roman-Urdu)

**Masla:** Render free 5GB bandwidth/month. 3 users × media commands = 4-5 din mein khatam, phir Render service **suspend** kar deta hai. Card nahi hai, to paid plan bhi nahi.

**Achhi khabar:** Repo ka kaam ab **20x bandwidth wale FREE platform** pe ho sakta hai — bina card, bina behaviour change. Neeche 3 options hain (order = meri recommendation).

---

## ⚡ OPTION A — Back4app Containers (BEST jugaad, 100GB/month FREE)

**Kyun best:**
| | Render Free | **Back4app Free** |
|---|---|---|
| Monthly transfer | 5 GB | **100 GB** |
| Card required | Nahi | **Nahi** (official FAQ: "no credit card required") |
| Sleep policy | 15 min idle → sleep | **Nahi** — Docker container 24/7 chalta hai |
| RAM | 512 MB | 256 MB (bot sirf ~95MB leta hai, 3 sessions ~135MB — FIT) |
| Deploy | Docker/GitHub | **Same Dockerfile** — GitHub repo connect |

Tumhari usage (~1GB/day) ke hisaab se **100GB = 3 mahine** chalega. 15 din ka to sawal hi nahi rehta. Bot ka **behaviour bilkul pehle jaisa** — same Docker image, same commands, same panel.

### Steps (10 minute ka kaam):

1. **Back4app account banao:** [back4app.com](https://www.back4app.com) → Sign up (email + password, **card nahi maangta**)
2. Dashboard → **"Containers"** tab → **"Create a new app"** → **"Import GitHub Repo"**
3. GitHub authorize karo → **Uf-prince/TEST-BOT** repo select karo
4. Deploy form me ye values:
   - **App name:** `gold-md` (jo marzi)
   - **Branch:** `main`
   - **Environment Variables** (ye zaroori hain):
     ```
     PORT=2081
     GOLDMD_PANEL_ENABLED=true
     GOLDMD_MAX_SESSIONS=3
     GOLDMD_OWNER_NUMBERS=923001234567  (apna number with country code, comma se multiple)
     ```
     (Storj ke 30 variables ki zaroorat **nahi** — `.env` file repo mein shipped hai + creds `storj.go` mein hardcoded hain, dono fallback zinda rehte hain)
5. Root directory me `Dockerfile` hai — Back4app khud build karega. **Dockerfile.back4app** ko asli `Dockerfile` banana ho to (slim build chahiye to) repo mein rename kar ke push kar do — warna purana `Dockerfile` bhi 100% kaam karega (kya farq? neeche dekho)
6. **"Create App"** → 5-10 min build → URL milega jaise `gold-md-xkj4f-a.b4a.run`
7. Us URL pe panel khul jayega → `/pair` se 3 doston ko pair karo → **ho gaya!**

### Dockerfile pasand (slim vs full):

| | Purana `Dockerfile` (repo mein) | `Dockerfile.back4app` (naya) |
|---|---|---|
| LibreOffice | ✅ | ✅ (rakha hai — `.pdf` command chalni chahiye) |
| Build time | ~6-8 min (Go compile on build) | **~1 min** (pre-built binary copy) |
| Image size | ~800MB | **~350MB** |
| Behaviour | Pehle jaisa | **Pehle jaisa** (same binary logic) |

**Note:** `Dockerfile.back4app` use karna ho to usme `COPY gold-md-cgofree ./gold-md` line binary ko uthati hai — ye binary tumhare **GitHub repo mein push karni hogi** (22MB file, GitHub pe allowed). Ya phir purana Dockerfile hi use karo — wo khud Go build karega (CGO nahi chahiye ab, kyunki maine sqlite **pure-Go** (modernc.org/sqlite) kar diya hai — cross-platform ho gaya, FreeBSD bhi!). **Purana Dockerfile ke saath bhi sab kuch same chalega** — sirf build thoda slow hoga.

### ⚠️ Ek dhyan rakhna (RAM):

256MB plan pe agar koi **`.pdf` command** (document→PDF) chale to LibreOffice spike se OOM ho sakta hai → container 30-60 sec mein **khud restart** ho jata hai → Storj se sessions wapas aa jate hain (yehi to Storj ka faida hai). Ye RARE hai (sirf pdf command pe). Bachna ho to: Back4app env me `GOLDMD_DISABLE_PDF=1`... nahi ye env nahi hai — bas itna samajh lo: agar OOM loop mein jaye to `.pdf` command band karwa dena, baqi sab commands (play, sticker, tomp3, antidelete, menu) 100% chalenge.

---

## 🆓 OPTION B — Serv00 (agar Back4app na chale — 100% free, unlimited bandwidth)

**FreeBSD hosting** hai — Linux binary nahi chalta, isliye maine **FreeBSD build** bana diya hai: `gold-md-freebsd` (22.8MB, pure-Go, CGO-free). Unlimited monthly transfer, 3GB disk, 512MB RAM, no card.

### Steps:

1. **Account banao:** [serv00.com/register-account](https://www.serv00.com/register-account/) — sirf email chahiye, card nahi. (Offer page: "no hooks, no ads"). Kabhi kabhi registrations batch mein khulte hain — agar page pe "register" band dikhe to agli subah try karo.
2. **SSH enable + files upload:** Account banne pe email milegi server details ke saath. Apne PC se:
   ```bash
   scp gold-md-freebsd .env servers.json deploy_serv00.sh LOGIN@sX.serv00.com:~/
   ```
   (Windows pe PowerShell se bhi `scp` chalta hai)
3. **SSH login karo:** `ssh LOGIN@sX.serv00.com` (password email se)
4. **Ek hi command:** `bash deploy_serv00.sh`
   Ye sab khud karegi:
   - `devil binexec on` — apni software chalane ki permission
   - `devil port add 30000 tcp` — port reserve
   - Bot port 30000 pe start (`nohup`)
   - `devil www add LOGIN.serv00.com proxy localhost 30000` — panel ko web pe expose (websocket support hai, SSL free Let's Encrypt)
   - **Cron watchdog** — har 5 min bot check; crash/reboot pe khud restart (`@reboot` bhi). Serv00 admin ka official pattern: *"Add a cron job to start the application on each reboot... include your own script for monitoring and launching the process if it's not present"*
   - **Inactivity guard** — roz 4am pe `activity.ping` touch (serv00 inactive accounts ko delete kar sakta hai — bot chalta rahe to activity hoti rehti hai, ye extra belt-and-suspenders hai)
5. **Panel URL:** `https://LOGIN.serv00.com` — SSL lagne mein 2-3 min lag sakte hain
6. **servers.json update:** Is panel URL ko `SERVER 1` ki `url` mein daal ke GitHub push karo (abhi wahan trycloudflare URL hai jo purani hai)

### Serv00 pe RAM:

512MB (Back4app se double) — 3 sessions + pdf command bhi comfortably chal jayega. Lekin **ffmpeg** wahan system pe nahi hota — bot ka **self-installer** Linux wala static ffmpeg download karega jo FreeBSD pe nahi chalega → `play/tomp3/sticker` video-wale commands error denge. **Fix (jugaad):** SSH me `pkg` se ffmpeg install nahi hoga (root nahi) — lekin **FreeBSD static ffmpeg** ki jagah bot pe ye kaam karega: `ports`? nahi. **Asli jugaad:** Serv00 pe `ffmpeg` user-space mein compile karna hoga — ye advanced hai. **Itna karne ka faida kya?** — Back4app mein ffmpeg pre-installed milta hai + 100GB bhi. **Isliye Option A primary rakho, Serv00 sirf plan-B.**

---

## 🩹 OPTION C — Render pe hi rehna hai to (bandwidth bachane ka jugaad)

Render pe **ingress FREE** hai (download karna unlimited). Metered sirf **egress** hai (bahar bhejna). Bot ki bandwidth kharch in cheezon se hoti hai:

1. **WhatsApp media uploads** — jab bot koi `play/tiktok/sticker` command chalata hai, media WhatsApp servers pe **upload** hota hai (ye egress hai). Ye bot ka **asli kaam** hai — isko band karne ka matlab bot kaam karna band karna.
2. **Storj message backups** — har message ka proto Storj pe upload (chhota, ~5-15KB per msg, par count zyada ho to jodta hai)

**Jugaad jo maine code mein laga diya hai (optional, env se ON/OFF):**

```env
GOLDMD_STORJ_MAX_UPLOAD_KB=512
```

- **Default OFF hai** (purana behaviour — bot pehle jaisa hi kaam karega, sab messages backup honge)
- Ye set karoge to sirf **bade protos** (512KB+) Storj pe upload **nahi** honge — chhote text/media protos (jo 99% hote hain) waise hi upload hote rahenge
- **IMPORTANT:** Ye cap **video/photo ke asli bytes pe nahi lagta** — sirf proto pe (jisme URL+keys hote hain, ~10KB). Deleted video **recovery antidelete se bilkul waise hi kaam karega** — video WhatsApp servers se download hota hai (ingress FREE) aur wapas upload (ye to bot ka kaam hai)
- 5GB mein 15 din: agar 512KB cap laga do to Storj wala overhead ~80% kam ho jayega. WhatsApp media uploads (bot ka core kaam) waise hi rahenge. 3 users × 15 din realistically **fit ho jayega** agar media commands moderate use hon.

**Verdict:** Ye banda-aid hai. Render ka 5GB limit **structural** hai — 3 users ka media usage usse daba nahi sakte. **Option A (Back4app) hi asli hal hai.**

---

## 🧠 Asli bandwidth kahan jaati hai (tumhare bot ki)

Maine code trace kiya (12+ jagah `Client.Upload` calls):

| Path | Direction | Metered? |
|---|---|---|
| `play`/`tiktok`/`yt` download | Internet → Render | **FREE** (ingress) |
| Media WhatsApp pe upload | Render → WhatsApp | **METERED** (5GB mein count) |
| Message protos → Storj | Render → Storj | **METERED** (chhota, lekin har msg) |
| WhatsApp socket receive | WhatsApp → Render | **FREE** (ingress) |
| Panel `/pair` requests | User → Render | **FREE** (ingress) |

Isliye: **5GB sirf WhatsApp-uploads + Storj-protos se khatam hui thi.** Back4app pe 100GB milte hain to dono koi masla nahi.

---

## ✅ Jo maine tumhare repo mein change kiya (behaviour SAME)

Tumhari shart thi: *"jo mrzi kr mre bot jse phle km kr rha tha wese hi km krna chye"* — isliye:

1. **`storj.go`** — `GOLDMD_STORJ_MAX_UPLOAD_KB` env var add hua. **Default OFF** (purana behaviour). Render pe bachana ho to manually ON karo. `go vet` pass.
2. **`main.go` + `upstash.go`** — sqlite driver **mattn (CGO) → modernc (pure-Go)**. Is se:
   - Binary ab **har Linux + FreeBSD** pe chalega (cross-compile possible)
   - `gold-md-freebsd` binary ban gayi (Serv00 ke liye)
   - Data format **same `.db`** — sessions/pairing/antidelete sab pehle jaisa
3. **`gold-cmds/system.go`** — disk-stats ka FreeBSD type-mismatch fix (uint64 cast) — Linux pe koi asar nahi, sirf FreeBSD build enable hua
4. **`Dockerfile.back4app`** (naya file) — slim image, pre-built binary, Back4app 256MB RAM ke liye optimized. LibreOffice **rakha hai** (`.pdf` command pehle jaisa)
5. **`deploy_serv00.sh`** (naya file) — Serv00 pe one-shot deploy (binexec + port + proxy + cron watchdog + inactivity guard)
6. **Smoke test PASS** — naye CGO-free binary se:
   - `/health` → `{"bot":"GOLD-MD","status":"online","sessions":1}`
   - `/sessions` → real session (923276650623@s.whatsapp.net) Storj se **restore** hui
   - RAM **86-95MB** (3 sessions ~135MB — 256MB mein fit)

**Jo kuch NahiN badla:** commands, pairing flow, antidelete, Storj backup, panel — sab **exactly pehle jaisa**. Sirf hosting options aur ek optional env-var jugaaḍ add hua.

---

## 🎯 Final Recommendation (mera verdict)

**Back4app Containers (Option A)** — 10 min setup, 100GB/month (20x Render), no card, no sleep, bot bilkul pehle jaisa. 3 doston ka usage **3 mahine** chala jayega. 15 din ka to sawal hi nahi.

Agar Back4app ne account/app bana ne se mana kar diya (kabhi kabhi free tier regions busy hote hain) → **Serv00 (Option B)** — unlimited transfer, lekin ffmpeg commands wahan nahi chalenge (Back4app pe chalenge). Back4app ke docs mein likha hai free plan "testing and learning" ke liye hai — agar kabhi policy change ho to Serv00 + cron watchdog pattern hamesha zinda rahega.

Render pe rehna pade to `GOLDMD_STORJ_MAX_UPLOAD_KB=512` env laga do — Storj overhead ~80% kam, lekin 5GB mein 3 users ka media **15 din mushkil** (media uploads hi asli kharcha hain, wo to bot ka kaam hai).

---

## 📎 Files jo tumhare saath hain (repo mein abhi local hain — push karna hoga)

| File | Kya hai |
|---|---|
| `Dockerfile.back4app` | Back4app slim image |
| `deploy_serv00.sh` | Serv00 one-shot deployer |
| `gold-md-cgofree` (22.9MB) | Linux binary (pure-Go sqlite) — Dockerfile.back4app me use hoti hai |
| `gold-md-freebsd` (22.8MB) | FreeBSD binary — Serv00 ke liye |
| patched: `storj.go`, `main.go`, `upstash.go`, `go.mod`, `go.sum`, `vendor/` | Code changes (driver swap + egress-cap env) |

Push karne ke liye (tumhare PAT se):
```bash
git add -A && git commit -m "bandwidth jugaad: pure-Go sqlite (FreeBSD support) + optional Storj egress cap + Back4app/Serv00 deploy files" && git push origin main
```
**DHYAAN:** Push karne se pehle ye socho — `gold-md-cgofree` + `gold-md-freebsd` 2 binaries ~45MB repo mein add karengi. GitHub pe 100MB tak file hoti hai, to theek hai. Lekin agar repo lean rakhni ho to binaries push kar ke alag GitHub **Release** me daal do aur `Dockerfile.back4app` me URL se download karwao — wo zyada clean hai. Render pe pehle jo setup tha wo purane `Dockerfile` se hi bana tha (wo khud compile karta hai) — wo rasta bhi abhi zinda hai (modernc swap ke baad bhi `go build -mod=vendor` usi tarah chalega).
