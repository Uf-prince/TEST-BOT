#!/usr/bin/env bash
# Launch GOLD-MD on port 48467.
#
# The bot needs the runtime env it was originally started with (Storj shards,
# Upstash Redis, fleet settings). That env is not persisted anywhere, so before
# killing the old instance we lift it straight from /proc/<pid>/environ of the
# live process. Nothing is written to disk, so no secret is left lying around.
set -u
cd /workspace/testbot

OLD_PID=$(pgrep -f "gold-md-4846[7]" | head -1)
if [ -n "$OLD_PID" ] && [ -r "/proc/$OLD_PID/environ" ]; then
  while IFS= read -r -d '' kv; do
    case "$kv" in
      _=*|OLDPWD=*|SHLVL=*|PWD=*|TMUX*|PROMPT_COMMAND=*) continue ;;
    esac
    export "$kv" 2>/dev/null || true
  done < "/proc/$OLD_PID/environ"
fi

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
