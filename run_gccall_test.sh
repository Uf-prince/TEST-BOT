#!/bin/bash
# GCCALL TEST LAUNCH — GOLD-MD on port 11221 with LIVE CALL DEBUG (JSON logs)
# GOLDMD_CALL_DEBUG=1 → har call event IN/OUT /workspace/calldebug.jsonl me capture
cd /workspace/TEST-BOT
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_CALL_DEBUG=1
export GOLDMD_DEBUG=1
mkdir -p nexstore/pairing
echo "gold-md-bot starting on :11221 (call debug ON)" > /workspace/bot_11221.log
exec ./gold-md-bot >> /workspace/bot_11221.log 2>&1
