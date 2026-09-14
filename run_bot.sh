#!/bin/bash
# GOLD-MD launcher for port 11221 (sandbox)
cd /workspace/TEST-BOT
export PORT=11221
export GOLDMD_SERVER_ID=svr11221
export GOLDMD_MAX_SESSIONS=10
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
mkdir -p nexstore/pairing
exec ./gold-md-bot
