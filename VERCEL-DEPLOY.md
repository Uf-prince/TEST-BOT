# GOLD-MD Panel on Vercel

The control panel (`src/panel.html`) now runs **two ways** from one source of truth:

1. **Go bot** — `src/panel.go` embeds `panel.html` via `//go:embed` and serves it
   at `http://<host>:PORT/` exactly as before. Behaviour is unchanged.
2. **Vercel** — the same page is served statically from `public/index.html`,
   with a serverless function `api/servers.js` that mirrors the Go
   `checkAllServers()` logic and reads the same `servers.json`.

## Files

| File | Purpose |
|---|---|
| `src/panel.html` | **Single source of truth** for the panel page |
| `src/panel.go` | Embeds `panel.html` (`//go:embed panel.html`) |
| `public/index.html` | Copy of `panel.html` served by Vercel |
| `api/servers.js` | Vercel serverless `/api/servers` (mirrors Go) |
| `servers.json` | Server list (shared by Go + Vercel) |
| `vercel.json` | Vercel config |
| `scripts/sync-panel.js` | Copies `src/panel.html` → `public/index.html` |

## Deploy on Vercel (git-connected)

1. Push this repo to GitHub.
2. In Vercel: **Add New → Project → Import** the repo.
3. Framework preset: **Other**. Build command: *(leave empty)*.
   Output directory: *(leave empty — `public/` is auto-detected)*.
4. Deploy. The panel goes live at `https://<project>.vercel.app/`.

The page's JS calls `/api/servers` (the serverless function) to populate the
server dropdown, then POSTs the pairing request **directly** to the selected
Render server's `/pair` endpoint — exactly like the Go panel does.

## Editing the panel

Edit `src/panel.html`, then run:

```bash
node scripts/sync-panel.js
```

This keeps `public/index.html` identical. Rebuild the Go binary to bake the
new page into the bot:

```bash
CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o gold-md-bot ./src
```
