#!/usr/bin/env bash
# Launch GOLD-MD on port 15273, fully detached from the calling shell.
cd /workspace/TEST-BOT
export PORT=15273
export GOLDMD_PANEL_ENABLED=true
export SUPERVISOR_ENABLED=1
setsid ./gold-md-cgofree < /dev/null >> bot_15273.log 2>&1 &
echo "launched detached PID $!"
