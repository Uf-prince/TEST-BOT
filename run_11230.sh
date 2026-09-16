#!/usr/bin/env bash
# Launcher for GOLD-MD bot on port 11230
cd /workspace/TEST-BOT

# Stop any old instance on this port only
OLDPID=$(fuser 11230/tcp 2>/dev/null | tr -d ' ')
if [ -n "$OLDPID" ]; then
  kill -9 $OLDPID 2>/dev/null
  sleep 1
fi

: > bot_11230.log

export PORT=11230
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_SERVER_ID=svr11230

mkdir -p nexstore/pairing

setsid nohup ./gold-md-bot-static > bot_11230.log 2>&1 < /dev/null &
echo "BOT_PID=$!"
sleep 5
if curl -s -m 6 http://localhost:11230/health | grep -q 'online'; then
  echo "BOT_RUNNING_OK"
else
  echo "BOT_STATUS_UNKNOWN"
  tail -30 bot_11230.log
fi
