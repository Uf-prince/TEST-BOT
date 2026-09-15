#!/bin/bash
# GOLD-MD launch on port 11221 (final gccall setup — warn/kick only)
cd /workspace/TEST-BOT
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_DEBUG=1
mkdir -p nexstore/pairing
echo "gold-md-bot starting on :11221 (final gccall: warn/kick)" > /workspace/bot_11221.log
exec ./gold-md-bot >> /workspace/bot_11221.log 2>&1
