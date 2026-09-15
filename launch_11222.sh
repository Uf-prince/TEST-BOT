#!/bin/bash
# Launch GOLD-MD Host A on port 11222 (TEST-BOT) — PID-FILE approach.
# SIRF port 11222 ka owner process kill hota hai (fuser se exact PID) —
# Host B (11224) ya koi aur process kabhi touch nahi hota.
cd /workspace/test-bot
# agar pehle se port pe koi (purana instance) chal raha hai to exact PID se maar do
OLDPID=$(fuser 11222/tcp 2>/dev/null | tr -d ' ')
if [ -n "$OLDPID" ]; then
  kill -9 $OLDPID 2>/dev/null
  sleep 1
fi
export PORT=11222
export GOLDMD_SERVER_ID=closure-screening-rabbit-recorded.trycloudflare.com
export GOLDMD_MAX_SESSIONS=2
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
rm -f bot_11222.log
setsid nohup ./gold-md-bot > bot_11222.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 5
if curl -s -m 6 http://localhost:11222/health | grep -q '"status":"online"'; then
  echo "HOSTA_RUNNING_OK"
else
  echo "HOSTA_FAILED"
  tail -20 bot_11222.log 2>/dev/null
fi
