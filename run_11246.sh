#!/usr/bin/env bash
# Launcher for GOLD-MD bot on port 11246
cd /workspace/TEST-BOT
export PATH=/usr/local/go/bin:$PATH

# Stop any old instance
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1

# Fresh log
: > bot.log

# Export all env from .env (read line by line, only KEY=VALUE lines)
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    ''|\#*) continue ;;
  esac
  export "$line" 2>/dev/null || true
done < .env

# Override port to 11246
export PORT=11246
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=3
export GOLDMD_DEBUG=0
export GOLDMD_SERVER_ID=svr1

# Launch fully detached
nohup ./gold-md-bot >> bot.log 2>&1 < /dev/null &
echo "$!" > bot.pid
disown 2>/dev/null || true
echo "BOT_PID=$(cat bot.pid)"
echo "PORT=11246"
exit 0
