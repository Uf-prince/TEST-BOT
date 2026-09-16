#!/bin/bash
# Fully detached launcher for GOLD-MD on port 11221.
pkill -9 -f "gold-md-bo[t]" 2>/dev/null
sleep 2
cd /workspace/TEST-BOT
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true

TUNNEL_URL=$(grep -ho 'https://[a-z0-9-]*\.trycloudflare\.com' \
  /var/log/supervisor/20242_cloudflared.err.log \
  /var/log/supervisor/20242_cloudflared.out.log 2>/dev/null | tail -1)
if [ -n "$TUNNEL_URL" ]; then
  export GOLDMD_PUBLIC_URL="$TUNNEL_URL"
fi

rm -f bot_11221.log
setsid nohup ./gold-md-bot > bot_11221.log 2>&1 < /dev/null &
echo "launched pid $!" > /tmp/launch_detached.out
sleep 5
if pgrep -f "gold-md-bo[t]" > /dev/null; then
  echo "BOT_RUNNING_OK" >> /tmp/launch_detached.out
else
  echo "BOT_FAILED" >> /tmp/launch_detached.out
fi
