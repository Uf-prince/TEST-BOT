#!/usr/bin/env bash
# Launcher for GOLD-MD bot — fully detached so it survives the shell session.
set -u
cd /workspace/TEST-BOT

# Kill any previous instance
pkill -9 -f gold-md-bot 2>/dev/null
sleep 1

# Fresh log
: > bot.log

# ── Storj S3 credentials (10 shards: umar1..umar10) ──
export STORJ_ACCESS_KEY_1=jwfjua62w45i5ebrx3t5nex455ma
export STORJ_SECRET_KEY_1=j2d2e4gjkc43m5sosfe7bznevm25aq627hgljdfhjofr5ezrsxazk
export STORJ_BUCKET_1=umar

export STORJ_ACCESS_KEY_2=jwpiqh2d4ky56hrjrrhrpz7h47cq
export STORJ_SECRET_KEY_2=j2n53jczyngsdci5vpr47ni24mlkdhzzrh2deyq2h4qelsst62da6
export STORJ_BUCKET_2=umar2

export STORJ_ACCESS_KEY_3=jvkcczaogv7syhlhyss2oot3dliq
export STORJ_SECRET_KEY_3=j327kiloo7yhd4x6n6epcvfbzfjuekjrf6bc237cesrv3nohh5kkc
export STORJ_BUCKET_3=umar3

export STORJ_ACCESS_KEY_4=jwknkytkfzph7mtonxq6g6d4ze3q
export STORJ_SECRET_KEY_4=jz24qglsu5wl34kmbsgchpgt2fzyibbdwyyehvqbe6pxvv4pzkpb2
export STORJ_BUCKET_4=umar4

export STORJ_ACCESS_KEY_5=jwjsgr627fnccmxfc4gtllg5bb7q
export STORJ_SECRET_KEY_5=jzihzet2ecmyn3inuglzvxldx3d5i6jnsrs4ky35nsr5tenxro7hg
export STORJ_BUCKET_5=umar5

export STORJ_ACCESS_KEY_6=jxzvkrhsaebljlko6dv2lin7x4wa
export STORJ_SECRET_KEY_6=j3iyzcwnbirmrhup3hc6352gl5n56xfhkna2l4dn3ugm4i7a2gd6s
export STORJ_BUCKET_6=umar6

export STORJ_ACCESS_KEY_7=jw6pkivs3vp6rmdty2da36auzimq
export STORJ_SECRET_KEY_7=j33i2ybq7kd7ouw6w7ltfmew2dzeg2t3v4ryterop75kdeyjdvvvo
export STORJ_BUCKET_7=umar7

export STORJ_ACCESS_KEY_8=ju4a4oqbejb3w7ygbmikkr4vgsna
export STORJ_SECRET_KEY_8=jzo2xqmutggpkbwxpgw5fswerf35miykdavdcfimmghqdpkgyqpxe
export STORJ_BUCKET_8=umar8

export STORJ_ACCESS_KEY_9=juznozmcpfsbwoqboijqwpus3raa
export STORJ_SECRET_KEY_9=j236o3cx4eud3dxraq55lnsnod4aradltsekqsbwk2cv6iebbtwma
export STORJ_BUCKET_9=umar9

export STORJ_ACCESS_KEY_10=ju7o5eflwumsaxxhdgdy6y23nbsq
export STORJ_SECRET_KEY_10=j3oulw7wfaequvvm5ims7xzjdkr5kfqgwecogozv4v25r72ffysog
export STORJ_BUCKET_10=umar10

# Bot env
export PORT=11233
export GOLDMD_DEBUG=0
export GOLDMD_PANEL_ENABLED=true
export GOLDMD_MAX_SESSIONS=3
export GOLDMD_SERVER_ID=svr1

# ── Watchdog loop ──
# Runs the bot in a loop. If the bot exits (self-restart, crash, or OOM),
# the watchdog immediately relaunches it. This is the safety net for
# Render Free tier where os.Exit would otherwise leave the bot dead.
# The bot's internal self-restart (fork+exec) handles most cases, but
# this loop catches any unexpected exits too.
# Set GOLDMD_NO_WATCHDOG=1 to disable the loop (single-run mode).
if [ "${GOLDMD_NO_WATCHDOG:-0}" = "1" ]; then
  # Single-run mode (no loop) — used for one-off testing
  setsid ./gold-md-bot >> bot.log 2>&1 < /dev/null &
  BOT_PID=$!
  disown 2>/dev/null
  echo "$BOT_PID" > bot.pid
  sleep 5
  echo "BOT_PID=$BOT_PID (no-watchdog mode)"
  if kill -0 "$BOT_PID" 2>/dev/null; then echo "STATUS=alive"; else echo "STATUS=dead"; fi
  exit 0
fi

# Watchdog loop mode
echo "[watchdog] Starting GOLD-MD with auto-restart loop..."
RESTART_COUNT=0
MAX_FAST_RESTARTS=5      # max restarts within FAST_RESTART_WINDOW
FAST_RESTART_WINDOW=60   # seconds — if bot dies this fast, it's a crash loop
WINDOW_START=$(date +%s)

while true; do
  # Fresh log for each run (rename old logs)
  if [ -f bot.log ]; then
    mv bot.log "bot.log.$(date +%s).old" 2>/dev/null
  fi

  # Run bot directly (NOT setsid) so `wait` tracks it properly.
  # The watchdog loop itself runs in the background via start_bot.sh.
  ./gold-md-bot >> bot.log 2>&1 &
  BOT_PID=$!
  echo "$BOT_PID" > bot.pid

  echo "[watchdog] Bot started (PID $BOT_PID) at $(date)"

  # Wait for the bot to exit (blocking — tracks the child PID)
  wait "$BOT_PID" 2>/dev/null
  EXIT_CODE=$?
  echo "[watchdog] Bot exited (code $EXIT_CODE) at $(date)"

  # ── Crash-loop protection ──
  NOW=$(date +%s)
  ELAPSED=$((NOW - WINDOW_START))
  if [ "$ELAPSED" -lt "$FAST_RESTART_WINDOW" ]; then
    RESTART_COUNT=$((RESTART_COUNT + 1))
  else
    # Window passed — reset counter
    RESTART_COUNT=0
    WINDOW_START=$NOW
  fi

  if [ "$RESTART_COUNT" -ge "$MAX_FAST_RESTARTS" ]; then
    echo "[watchdog] CRASH LOOP detected ($RESTART_COUNT restarts in ${FAST_RESTART_WINDOW}s) — stopping to avoid resource waste"
    echo "[watchdog] Manual intervention required. Fix the issue and re-run start_bot.sh"
    exit 1
  fi

  # Brief pause before relaunch (avoid hammering)
  sleep 3
  echo "[watchdog] Relaunching bot..."
done
