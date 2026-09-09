#!/bin/bash
# Launch GOLD-MD bot on port 11221
pkill -9 -f "gold-md-bo[t]" 2>/dev/null
sleep 1
cd /workspace/test-bot
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
rm -f bot_11221.log
setsid nohup ./gold-md-bot > bot_11221.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 3
if pgrep -f "gold-md-bo[t]" > /dev/null; then
  echo "BOT_RUNNING_OK"
else
  echo "BOT_FAILED"
  cat bot_11221.log 2>/dev/null | tail -30
fi
