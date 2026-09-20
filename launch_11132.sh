#!/usr/bin/env bash
# GOLD-MD instance #3 on port 11132 (fleet test).
# Shares the SAME local DB as the other instances (simulates shared Storj).
cd /workspace/TEST-BOT
pkill -9 -x gold-md-11132 2>/dev/null
sleep 1
rm -f bot_11132.log
export PORT=11132
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_SERVER_ID=svr11132
export GOLDMD_MAX_SESSIONS=2
export GOLDMD_DEBUG=1
export GOLDMD_LOCALDB_DIR="nexstore/localdb"
export GOLDMD_DISK_CACHE_DIR="nexstore/kvcache_11132"
export GOLDMD_PUBLIC_URL="https://newspaper-bath-der-task.trycloudflare.com"
setsid ./gold-md-11132 > bot_11132.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 6
if pgrep -x gold-md-11132 > /dev/null; then
  echo "BOT_11132_RUNNING_OK"
else
  echo "BOT_11132_FAILED"
fi
exit 0
