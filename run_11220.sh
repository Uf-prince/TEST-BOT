#!/bin/bash
# GOLD-MD launcher for port 11220 (sandbox)
cd /workspace/TEST-BOT
pkill -9 -f "gold-md-bot-stati[c]" 2>/dev/null
sleep 1
export PORT=11220
export GOLDMD_SERVER_ID=svr11220
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
mkdir -p nexstore/pairing
rm -f bot_11220.log
setsid nohup ./gold-md-bot-static > bot_11220.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 5
if pgrep -f "gold-md-bot-stati[c]" > /dev/null; then
  echo "BOT_RUNNING_OK"
else
  echo "BOT_FAILED"
  tail -30 bot_11220.log 2>/dev/null
fi
exit 0
