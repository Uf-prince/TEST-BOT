#!/bin/bash
# Launch GOLD-MD bot on port 11221 (Storadera storage + fleet URL publish)
pkill -9 -f "gold-md-bo[t]" 2>/dev/null
sleep 1
cd /workspace/TEST-BOT
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true

# OWNER FIX B: apna public URL heartbeat me publish hoga (fleet discoverability).
# Quick-tunnel URL restart pe badal sakta hai — isliye HAR LAUNCH pe cloudflared
# log se fresh URL nikalo (supervisor 20242_cloudflared tunnel isi port pe hai).
TUNNEL_URL=$(grep -ho 'https://[a-z0-9-]*\.trycloudflare\.com' \
  /var/log/supervisor/20242_cloudflared.err.log \
  /var/log/supervisor/20242_cloudflared.out.log 2>/dev/null | tail -1)
if [ -n "$TUNNEL_URL" ]; then
  export GOLDMD_PUBLIC_URL="$TUNNEL_URL"
  echo "GOLDMD_PUBLIC_URL=$TUNNEL_URL"
else
  echo "WARN: tunnel URL nahi mila — fleet URL publish OFF"
fi

rm -f bot_11221.log
setsid nohup ./gold-md-bot > bot_11221.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 4
if pgrep -f "gold-md-bo[t]" > /dev/null; then
  echo "BOT_RUNNING_OK"
else
  echo "BOT_FAILED"
  tail -20 bot_11221.log 2>/dev/null
fi
exit 0
