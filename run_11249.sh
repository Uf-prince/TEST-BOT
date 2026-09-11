#!/usr/bin/env bash
# Launcher for GOLD-MD bot on port 11249
cd /workspace/TEST-BOT
export PATH=/usr/local/go/bin:$PATH

# Stop any old instance
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1

# Fresh log
: > bot.log

# .env REMOVED — Storj creds are hardcoded in storj.go (fallback), PORT below
# Bot env (hardcoded — no .env needed)
export PORT=11249
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=3
export GOLDMD_DEBUG=0

# Launch fully detached
nohup ./gold-md-bot >> bot.log 2>&1 < /dev/null &
echo "$!" > bot.pid
disown 2>/dev/null || true
echo "BOT_PID=$!"
echo "PORT=11249"
