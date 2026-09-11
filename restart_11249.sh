#!/bin/bash
# Restart GOLD-MD on port 11249 with a fresh binary (detached properly)
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1

cd /workspace/TEST-BOT

# .env REMOVED — Storj creds are hardcoded in storj.go (fallback), PORT below
# Bot env (hardcoded — no .env needed)
export PORT=11249
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=3
export GOLDMD_DEBUG=0

: > bot.log

# Fully detached via setsid — survives this script/session
setsid nohup ./gold-md-bot >> bot.log 2>&1 < /dev/null &
BOT_PID=$!
echo "$BOT_PID" > bot.pid
disown 2>/dev/null || true

sleep 3
if pgrep -f gold-md-bot > /dev/null; then
  echo "BOT_RUNNING pid=$(pgrep -f gold-md-bot | head -1)"
else
  echo "BOT_FAILED"
  tail -10 bot.log
fi
