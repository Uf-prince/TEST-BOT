#!/bin/sh
# ============================================================================
# GOLD-MD + ngrok PERMANENT-URL launcher (Back4App Containers / koi bhi host)
#
# SETUP (ek baar, 2 minute):
#   1) https://dashboard.ngrok.com/signup  <- FREE, Google login
#   2) https://dashboard.ngrok.com/get-started/your-authtoken  <- copy token
#   3) Gateway > Domains me apna FREE static domain dekho:
#      -> https://dashboard.ngrok.com/domains
#      -> xyz.ngrok-free.app  (ye PERMANENT hai - kabhi expire nahi hota)
#   4) Container env vars (Back4App App Settings > Environment Variables):
#         NGROK_AUTHTOKEN = <apna authtoken>
#         NGROK_DOMAIN    = xyz.ngrok-free.app
#   5) Redeploy.
#
# FLOW: bot pehle start hota hai (WhatsApp sessions load), phir ngrok agent
# tunnel kholta hai PANEL port pe. Domain PERMANENT hai -> URL expiry ka
# masla khatam. ngrok agent crash ho to auto-restart (while-loop).
# ============================================================================

PORT="${PORT:-11224}"
mkdir -p nexstore/pairing

echo "[start.sh] GOLD-MD starting (PORT=$PORT)"

# --- Bot start (foreground nahi - background me, phir wait) ---
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
    echo "[start.sh] NGROK_AUTHTOKEN/NGROK_DOMAIN set NAHI hai - Back4App ka apna URL + gist registry chalega"
fi

# --- Bot crash bhi ho to restart ---
wait $BOT_PID
while true; do
    echo "[start.sh] bot exit - 5s baad restart"
    sleep 5
    ./gold-md &
    wait $BOT_PID
done
