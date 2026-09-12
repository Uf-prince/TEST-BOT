#!/bin/bash
# Detached launcher for GOLD-MD (LOW-RAM build) on port 11221
cd /workspace/GOLD-MD
pkill -9 -f gold-md-lowram 2>/dev/null
pkill -9 -f gold-md-cgofree 2>/dev/null
sleep 1
rm -f bot_11221.log
export PORT=11221
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_SERVER_ID=svr11221
# PRODUCTION CONFIG (Zerops 1GB / 10 sessions):
#   GOMEMLIMIT 300MB soft target  -> keeps GC efficient, NO thrashing
#   Cleanup 850MB -> cache drop   -> bot stays alive, users unaffected
#   Restart 900MB -> hard reset   -> extreme emergency only
export GOLDMD_RAM_TARGET_MB=300
export GOLDMD_CLEANUP_MB=850
export GOLDMD_RESTART_MB=900
setsid nohup ./gold-md-lowram > bot_11221.log 2>&1 < /dev/null &
BOT_PID=$!
disown -a
echo "BOT_PID=$BOT_PID"
exit 0
