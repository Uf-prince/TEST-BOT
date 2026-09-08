#!/usr/bin/env bash
cd /workspace/TEST-BOT
export PATH=/usr/local/go/bin:$PATH
pkill -f gold-md-bot 2>/dev/null
sleep 2
: > bot.log
export PORT=11233
export GOLDMD_PANEL_ENABLED=true
setsid ./gold-md-bot < /dev/null >> bot.log 2>&1 &
echo "BOT_PID=$!"
disown
