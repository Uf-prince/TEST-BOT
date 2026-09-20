#!/usr/bin/env bash
# GOLD-MD LOCAL AUTO-DEPLOY WATCHER (Render auto-deploy simulation).
#
# Owner order: "in instances ka auto deploy trigger when changes on kr le"
#
# Ye script LOCAL source files (src/*.go, servers.json) ke mtime ko watch
# karta hai. Jab bhi koi file change hoti hai (jaise Render pe git push se
# auto-deploy hota hai), ye:
#   1. fresh binary build karta hai (current local files se)
#   2. TEENO local bots (11222 / 11131 / 11132) EK SAATH restart karta hai
#      -> bilkul Render jaisa boot-race (saare servers ek saath uthte hain)
#
# Log: /workspace/TEST-BOT/autodeploy.log
set -u
cd /workspace/TEST-BOT

LOG="autodeploy.log"
POLL=5   # seconds

log() { echo "$(date -u +%H:%M:%S) [AUTODEPLOY] $*" >> "$LOG"; }

# signature = max mtime of watched files
sig() {
  find src -name '*.go' -printf '%T@\n' 2>/dev/null | sort -n | tail -1
  stat -c '%Y' servers.json 2>/dev/null
}

last_sig="$(sig | tr '\n' '|')"
log "watcher started (poll=${POLL}s, sig=${last_sig})"

while true; do
  sleep "$POLL"
  cur_sig="$(sig | tr '\n' '|')"
  if [ "$cur_sig" = "$last_sig" ]; then
    continue
  fi

  log "CHANGE detected (sig ${last_sig} -> ${cur_sig}) — deploying..."

  # 1) fresh binary build from CURRENT local files
  export CGO_ENABLED=0
  if go build -mod=vendor -o gold-md-cgofree-new ./src/ >>"$LOG" 2>&1; then
    log "build OK"
  else
    log "build FAILED — skipping restart"
    last_sig="$cur_sig"
    continue
  fi

  # 2) restart ALL 3 instances simultaneously (Render boot-race simulation)
  pkill -9 -x gold-md 2>/dev/null
  pkill -9 -x gold-md-11131 2>/dev/null
  pkill -9 -x gold-md-11132 2>/dev/null
  sleep 2
  cp -f gold-md-cgofree-new gold-md
  cp -f gold-md-cgofree-new gold-md-11131
  cp -f gold-md-cgofree-new gold-md-11132
  chmod +x gold-md gold-md-11131 gold-md-11132
  bash launch_11222.sh >>"$LOG" 2>&1
  bash launch_11131.sh >>"$LOG" 2>&1
  bash launch_11132.sh >>"$LOG" 2>&1
  log "all 3 instances restarted on fresh build"

  last_sig="$cur_sig"
done
