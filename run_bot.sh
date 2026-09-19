#!/usr/bin/env bash
cd /workspace/TEST-BOT
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1
export PORT=11221
export GOLDMD_PANEL_ENABLED=1
nohup ./gold-md-bot > bot_11221.log 2>&1 &
echo "started $!"
