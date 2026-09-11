# 🏆 GOLD-MD — Go Multi-Session WhatsApp Bot

A **Golang** rewrite of the Node.js UMAR multi-session WhatsApp bot, built with the [`whatsmeow`](https://github.com/tulir/whatsmeow) library (the Go equivalent of `@whiskeysockets/baileys`).

> **One server, many connectors.** Each WhatsApp session runs as an independent connector inside a single Go process, auto-loaded in batches on startup — exactly like the original `autoload.js` + `pair.js` architecture.

---

## ✨ Features

| Feature | Description |
|---|---|
| **Multi-session** | One Go process hosts unlimited WhatsApp connections |
| **Auto-load** | All saved sessions reconnect in batches on startup (batch size + delay configurable) |
| **Graceful shutdown** | SIGINT / SIGTERM disconnects every session cleanly |
| **3 core commands** | `alive`, `ping`, `menu` |
| **Owner commands** | `setprefix`, `uptime`, `sessions` |
| **Pairing panel** | Web UI at `http://server:PORT/` — pair new sessions by phone number |
| **Upstash Redis** | Optional persistent config layer (prefix, sudo, banned, settings) — mirrors Node `db.js` |
| **QR + Code pairing** | Both QR-code (terminal) and phone-number pairing-code flows supported |

---

## 📦 Project Structure

```
gold-md/
├── main.go          # Entry point, banner, startup, shutdown
├── config.go        # Env-based config (prefix, owners, batching, Upstash)
├── manager.go       # Session manager (multi-session core, autoload, pairing)
├── handler.go       # Message routing + command dispatcher (port of case.js)
├── commands.go      # alive / ping / menu / setprefix / uptime / sessions
├── upstash.go       # Upstash Redis REST client (port of db.js)
├── panel.go         # HTTP pairing panel + health check
├── logger.go        # Coloured console logger
├── .env.example     # Sample environment file
├── Dockerfile       # Production container
└── go.mod           # Dependencies (whatsmeow + sqlite)
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
Sab credentials (Storj shards, PORT, GOLDMD settings) source mein **hardcoded** hain —
koi `.env` file ya env vars ki zaroorat nahi. Bas build karke chalao.

### 3. Build & Run
```bash
CGO_ENABLED=1 go build -o gold-md .
./gold-md
```

### 4. Pair a Session
Open the panel in your browser:
```
http://localhost:2081/
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
| `index.js` (entry) | `main.go` |
| `pair.js` (session connector) | `manager.go` → `StartSession()` |
| `autoload.js` (batch auto-load) | `manager.go` → `AutoLoad()` |
| `case.js` (command dispatch) | `handler.go` → `HandleMessage()` |
| `db.js` (Upstash Redis) | `upstash.go` |
| PM2 `ecosystem.config.js` | native Go signal handling |
| `nexstore/pairing/<jid>/` dirs | same — `nexstore/pairing/<jid>/` |
| `@upstash/redis` REST client | Upstash REST via `net/http` |

---

## ☁️ Deploying

### Railway
```bash
# 1. Push to GitHub
# 2. Railway → New Project → Deploy from GitHub repo
# 3. Set env vars (PORT, GOLDMD_OWNERS, Upstash creds)
# 4. Railway auto-detects Dockerfile
```

### Fly.io
```bash
fly launch          # generates fly.toml
fly secrets set GOLDMD_OWNERS=923xxxxxxxxx
fly secrets set UPSTASH_REDIS_REST_URL=...
fly secrets set UPSTASH_REDIS_REST_TOKEN=...
fly deploy
```

### Render
```bash
# Web Service → connect repo → Docker → set env vars
```

> **Note:** The sqlite session store uses a local file. On ephemeral filesystems (Railway/Fly/Render free tiers), attach a persistent volume to `/app/nexstore` so sessions survive redeployments. Alternatively, use the Upstash Redis layer for config (prefix/sudo/banned) which persists regardless.

---

## 📝 Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `2081` | HTTP panel port |
| `GOLDMD_PANEL_ENABLED` | `true` | Enable/disable the pairing panel |
| `GOLDMD_DATA_DIR` | `nexstore` | Session data directory |
| `GOLDMD_DEFAULT_PREFIX` | `.` | Default command prefix |
| `GOLDMD_BATCH_SIZE` | `5` | Auto-load batch size |
| `GOLDMD_BATCH_DELAY` | `2` | Seconds between batches |
| `GOLDMD_OWNERS` | — | Comma-separated owner phone numbers |
| `UPSTASH_REDIS_REST_URL` | — | Upstash Redis REST URL (optional) |
| `UPSTASH_REDIS_REST_TOKEN` | — | Upstash Redis REST token (optional) |
| `GOLDMD_DEBUG` | — | Set to `1` for debug logging |

---

## ⚠️ Important Notes

- **WhatsApp ToS:** Automating WhatsApp may violate their Terms of Service. Use responsibly.
- **Session persistence:** The sqlite DB (`goldmd.db`) holds all auth credentials. Back it up.
- **Missing files:** The original Node zip was missing `index.js` and `pair.js`, so the exact command implementations were reconstructed based on `case.js`, `autoload.js`, and `db.js` architecture. The three requested commands (`alive`, `ping`, `menu`) plus owner helpers were built fresh in idiomatic Go.

---

Built with ❤️ in Go · Powered by [whatsmeow](https://github.com/tulir/whatsmeow)
