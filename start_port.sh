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
# OWNER REQUEST: RAM 30-40 MB — aggressive GC settings
export GOLDMD_RAM_TARGET_MB=40
export GOLDMD_CLEANUP_MB=150
export GOLDMD_RESTART_MB=200
export GOGC=10
setsid nohup ./gold-md-lowram > bot_11221.log 2>&1 < /dev/null &
BOT_PID=$!
disown -a
echo "BOT_PID=$BOT_PID"
exit 0
