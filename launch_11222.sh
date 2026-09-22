#!/usr/bin/env bash
# Launch GOLD-MD on port 11222, fully detached.
# STORJ/STORADERA BACKEND ACTIVE (local SQLite disabled).
# FULL DEBUGGING ENABLED (fleet claim, full-to-full JSON, watchdog, reconnect).
cd /workspace/TEST-BOT
pkill -9 -x gold-md 2>/dev/null
sleep 1
rm -f bot_11222.log
export PORT=11222
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_SERVER_ID=svr11222
export GOLDMD_MAX_SESSIONS=2
export GOLDMD_DEBUG=1
# Public URL for fleet discoverability (heartbeat 4th segment).
export GOLDMD_PUBLIC_URL="https://hierarchy-humanitarian-july-nokia.trycloudflare.com"
setsid ./gold-md > bot_11222.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 6
if pgrep -x gold-md > /dev/null; then
  echo "BOT_RUNNING_OK"
else
  echo "BOT_FAILED"
fi
exit 0
