#!/usr/bin/env bash
# Launch GOLD-MD on port 48467.
#
# The public forwarders (12000/12001) point at 127.0.0.1:48467, so PORT must be
# forced here. Extra runtime env the bot may use (Storj/Upstash) is not
# persisted anywhere; if a live instance still has it, it is lifted straight
# from /proc/<pid>/environ before the old process is killed, so no secret is
# ever written to disk.
set -u
cd /workspace/testbot

OLD_PID=$(pgrep -f "gold-md-4846[7]" | grep -v "^$$\$" | head -1)
if [ -n "$OLD_PID" ] && [ -r "/proc/$OLD_PID/environ" ]; then
  while IFS= read -r -d '' kv; do
    case "$kv" in
      _=*|OLDPWD=*|SHLVL=*|PWD=*|TMUX*|PROMPT_COMMAND=*|PORT=*) continue ;;
    esac
    export "$kv" 2>/dev/null || true
  done < "/proc/$OLD_PID/environ"
fi

export PORT=48467
export GOLDMD_PANEL_ENABLED=true

pkill -9 -f "gold-md-4846[7]" 2>/dev/null
sleep 1
: > bot_48467.log
setsid nohup ./gold-md-48467 > bot_48467.log 2>&1 < /dev/null &
echo "launched pid $!"
sleep 6
if pgrep -f "gold-md-4846[7]" > /dev/null; then
  echo BOT_RUNNING_OK
else
  echo BOT_FAILED
  tail -30 bot_48467.log
fi

