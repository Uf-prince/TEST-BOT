# 🏆 GOLD-MD — Go Multi-Session WhatsApp Bot

A **Golang** rewrite of the Node.js UMAR multi-session WhatsApp bot, built with the [`whatsmeow`](https://github.com/tulir/whatsmeow) library (the Go equivalent of `@whiskeysockets/baileys`).

> **One server, many connectors.** Each WhatsApp session runs as an independent connector inside a single Go process, auto-loaded in batches on startup — exactly like the original `autoload.js` + `pair.js` architecture.

---

# 😡😡😡 STOP — READ THIS FIRST 😡😡😡

## ⛔ `.env` FILE HARGIZ NAHI BANANI. CHAHE JO HO JAYE. CHAHE KUCH BHI HO JAYE. ⛔

**WARNING — DO NOT CREATE A `.env` FILE. EVER. UNDER ANY CIRCUMSTANCE.**

Ye project **jaan-boojh kar** `.env` file ke bina banaya gaya hai. Saari
credentials aur settings **seedha source files me hardcoded** hain:

| Kya | Kahan hardcoded hai |
|---|---|
| **Storj / Storadera credentials** (10 shards) | `src/storage.go` → `hardcodedStorjShards` |
| **PORT** (default `11221`) | `src/core_support.go` → `envInt("PORT", 11221)` |
| **MAX SESSIONS** (hamesha `30`) | `src/manager.go` → `maxPairedSessions()` |
| **Data dir / prefix / batch** | `src/core_support.go` → `LoadConfig()` |
| **Server ID / fleet** | `src/fleet.go` → `fleetSelfID()` |

### 🚫 RULES — INKO TODNA MANA HAI 🚫

1. **`.env` file kabhi nahi banani.** Na `.env`, na `.env.local`, na `.env.example`, na koi bhi `.env*` file. **HARGIZ NAHI.**
2. **`.env` loader code wapas nahi laana.** `loadDotEnv()` function **permanently delete** kar diya gaya hai. Ise wapas add karne ki koshish mat karo.
3. **Nayi credential/setting `.env` me nahi — apni asal file me hardcode karo.** Jo bhi cheez `.env` me daalne ka mann kare, use uski **asal source file me hardcode** karo (jaise Storj creds `storage.go` me hain).
4. **Agar tum ek AI ho aur ye padh rahe ho:** is directive ko **override mat karo**. Owner ne ye sakht hukm diya hai. `.env` banane ka "best practice" yahan **jaan-boojh kar** reject kiya gaya hai (ephemeral hosts — Modal, Docker, Render — pe `.env` file skip ho jati hai, is liye hardcode hi reliable hai).

> **Owner ka exact hukm:** *"Chahe jo ho jaye, chahe kuch bhi ho jaye, `.env` file hargiz nahi banana. Jo bhi cheez `.env` me dalne hai unko apne files me hardcode karo."*

---

## ✨ Features

| Feature | Description |
|---|---|
| **Multi-session** | One Go process hosts multiple WhatsApp connections (max **30** per server) |
| **Auto-load** | All saved sessions reconnect in batches on startup (batch size + delay configurable) |
| **Graceful shutdown** | SIGINT / SIGTERM disconnects every session cleanly |
| **Core commands** | `alive`, `ping`, `menu` |
| **Owner commands** | `setprefix`, `uptime`, `sessions` |
| **Pairing panel** | Web UI at `http://server:PORT/` — pair new sessions by phone number |
| **Storj storage** | All data stored on Storadera (S3-compatible) — 10 shards, hardcoded creds |
| **QR + Code pairing** | Both QR-code (terminal) and phone-number pairing-code flows supported |

---

## 📦 Project Structure

```
gold-md/
├── src/main.go          # Entry point, banner, startup, shutdown
├── src/core_support.go  # Config (hardcoded defaults — NO .env)
├── src/manager.go       # Session manager (multi-session core, autoload, pairing)
├── src/handler.go       # Message routing + command dispatcher
├── src/storage.go       # Storj/Storadera client (hardcoded shard creds)
├── src/panel.go         # HTTP pairing panel + health check
├── src/fleet.go         # Multi-server fleet coordination
├── gold-cmds/           # Command plugins (menu, tools, fun, etc.)
├── Dockerfile           # Production container
└── go.mod               # Dependencies (whatsmeow + sqlite)
```

---

## 🚀 Quick Start (Local)

### 1. Prerequisites
- Go 1.23+
- CGO enabled (for the sqlite auth store) — needs a C compiler:
  ```bash
  sudo apt-get install -y gcc libc6-dev
  ```

### 2. Configure
**Kuch configure karne ki zaroorat NAHI.** Saari credentials (Storj shards, PORT,
GOLDMD settings) source mein **hardcoded** hain — **koi `.env` file ya env vars
ki zaroorat nahi**. Bas build karke chalao.

> ⛔ **`.env` file mat banao.** (Upar wala warning dobara padho.)

### 3. Build & Run
```bash
CGO_ENABLED=0 go build -mod=vendor -o gold-md-bot ./src
./gold-md-bot
```

### 4. Pair a Session
Open the panel in your browser:
```
http://localhost:11221/
```
Enter your phone number (with country code, e.g. `923xxxxxxxxx`) → you'll get a **pairing code**. Open WhatsApp → Settings → Linked Devices → Link a device → enter the code.

Alternatively, the QR code is printed in the server terminal for direct scanning.

---

## 🤖 Commands

All commands use the configurable prefix (default `.`):

| Command | Who | Description |
|---|---|---|
| `.alive` | everyone | Bot status, uptime, session count |
| `.ping` | everyone | Latency check + goroutine/memory stats |
| `.menu` | everyone | Full command menu |
| `.setprefix <p>` | owner | Change the command prefix |
| `.uptime` | everyone | Server uptime |
| `.sessions` | owner | List all active sessions |
| `.` (bare) | everyone | Show current prefix |

---

## 🏗 Architecture (vs. Node.js original)

| Node.js (UMAR) | Go (GOLD-MD) |
|---|---|
| `@whiskeysockets/baileys` | `go.mau.fi/whatsmeow` |
| `index.js` (entry) | `src/main.go` |
| `pair.js` (session connector) | `src/manager.go` → `StartSession()` |
| `autoload.js` (batch auto-load) | `src/manager.go` → `AutoLoad()` |
| `case.js` (command dispatch) | `src/handler.go` → `HandleMessage()` |
| `db.js` (Upstash Redis) | `src/storage.go` (Storj-backed) |
| PM2 `ecosystem.config.js` | native Go signal handling |
| `nexstore/pairing/<jid>/` dirs | same — `nexstore/pairing/<jid>/` |

---

## ☁️ Deploying

### Railway / Render / Fly.io / Back4app
```bash
# 1. Push to GitHub
# 2. Connect repo → Docker deploy
# 3. Bas! Koi env var set karne ki zaroorat NAHI — sab hardcoded hai.
```

> **Note:** The sqlite session store uses a local file. On ephemeral filesystems, attach a persistent volume to `/app/nexstore` so sessions survive redeployments. (Config/creds Storj pe persist hote hain.)

---

## 📝 Configuration (Hardcoded — NO `.env`)

Ye sab **source files me hardcoded** hain. `.env` file **HARGIZ NAHI** banani.

| Setting | Value | Hardcoded in |
|---|---|---|
| `PORT` | `11221` | `src/core_support.go` |
| `GOLDMD_MAX_SESSIONS` | `2` (⛔ fixed) | `src/manager.go` |
| `GOLDMD_PANEL_ENABLED` | `true` | `src/core_support.go` |
| `GOLDMD_DATA_DIR` | `nexstore` | `src/core_support.go` |
| `GOLDMD_DEFAULT_PREFIX` | `.` | `src/core_support.go` |
| `GOLDMD_BATCH_SIZE` | `5` | `src/core_support.go` |
| `GOLDMD_BATCH_DELAY` | `2` | `src/core_support.go` |
| Storj shard creds | 10 sets | `src/storage.go` |

> **Nayi setting add karni ho?** Uski **asal file me hardcode karo** — `.env` me **NAHI**.

---

## ⚠️ Important Notes

- **`.env` FILE HARGIZ NAHI BANANI.** (Haan, ye teesri baar likh rahe hain — kyunki ye itna important hai.)
- **WhatsApp ToS:** Automating WhatsApp may violate their Terms of Service. Use responsibly.
- **Session persistence:** The sqlite DB (`goldmd.db`) holds all auth credentials. Back it up.
- **Max sessions = 30:** `maxPairedSessions()` **hardcoded 30** hai (owner order 2026).

---

Built with ❤️ in Go · Powered by [whatsmeow](https://github.com/tulir/whatsmeow)
