#!/usr/bin/env bash
cd /workspace/TEST-BOT
export PATH=/usr/local/go/bin:$PATH
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1
: > bot.log
# .env REMOVED — Storj creds are hardcoded in storj.go (fallback), PORT below
export PORT=11238
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=3
export GOLDMD_DEBUG=0
exec ./gold-md-bot
