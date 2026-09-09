#!/bin/bash
# FB-fix deploy: kill old bot, start new binary on port 11221
cd /workspace/TEST-BOT
# kill any existing instance (pattern avoids matching this script's shell)
pkill -9 -f "gold-md-bo[t]" 2>/dev/null
sleep 1
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
rm -f /workspace/TEST-BOT/bot_11221.log
setsid nohup ./gold-md-bot > /workspace/TEST-BOT/bot_11221.log 2>&1 &
echo "launched pid $!"
