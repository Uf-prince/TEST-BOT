#!/bin/sh
# ============================================================================
# GOLD-MD launcher — FIXED crash-restart loop (v2)
# ============================================================================
# PURANA BUG: crash-restart loop me `wait $BOT_PID` PURANA (mara hua) PID pe
# chalta tha — naya process spawn hone ke baad PID dobara capture NAHI hota
# tha. Result: loop har 5 second naya bot process bana deta (fork-bomb!) aur
# 2 processes same WhatsApp session pe lar rahe the (kick-kick reconnect
# loop, phantom crashes).
#
# FIX: `wait $BOT_PID` ke baad `BOT_PID=$!` dobara capture. Ab loop me sirf
# EK bot process zinda rehta hai — crash ho to khud restart, duplicate kabhi
# nahi.
#
# ZEROPS NOTE: Zerops pe ye file use NAHI hoti (zerops.yaml me start: ./gold-md
# seedha hai — Zerops khud supervisor hai, SUPERVISOR_ENABLED=1 pe bot clean
# exit karta hai). Ye file Back4App/Docker/VPS hosts ke liye fixed rakhi gayi
# hai.
# ============================================================================

PORT="${PORT:-11224}"
mkdir -p nexstore/pairing

echo "[start.sh] GOLD-MD starting (PORT=$PORT)"

# --- Bot start (background me, phir wait) ---
./gold-md &
BOT_PID=$!

# --- ngrok agent (agar token set hai) ---
if [ -n "$NGROK_AUTHTOKEN" ] && [ -n "$NGROK_DOMAIN" ]; then
    echo "[start.sh] ngrok tunnel start: $NGROK_DOMAIN -> localhost:$PORT"
    (
      while true; do
        /usr/local/bin/ngrok http "$PORT" \
            --url "https://$NGROK_DOMAIN" \
            --log stdout --log-format json --log-level warn \
            --update false > /tmp/ngrok.log 2>&1
        echo "[start.sh] ngrok exit - 5s baad restart"
        sleep 5
      done
    ) &
    NGROK_PID=$!
    echo "[start.sh] ngrok PID=$NGROK_PID"
else
    echo "[start.sh] NGROK_AUTHTOKEN/NGROK_DOMAIN set NAHI hai - public URL + gist registry chalega"
fi

# --- Bot crash bhi ho to restart (FIXED: PID re-capture) ---
wait $BOT_PID
while true; do
    echo "[start.sh] bot exit - 5s baad restart"
    sleep 5
    ./gold-md &
    BOT_PID=$!    # << FIX: naya PID capture (purana bug yahi tha)
    wait $BOT_PID
done
