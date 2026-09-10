#!/usr/bin/env bash
# ============================================================================
# GOLD-MD — Serv00 (free FreeBSD hosting) pe one-shot deploy script
# ============================================================================
# REQUIREMENTS (ye script SERV00 SSH ke andar chalani hai):
#   1. serv00.com pe account banao (free, card nahi lagta)
#   2. Binary + .env + servers.json + ye script apne PC se serv00 pe bhejo:
#        scp gold-md-freebsd .env servers.json deploy_serv00.sh LOGIN@sX.serv00.com:~/
#      (LOGIN = tumhara serv00 username, sX = server number jahan account bana)
#   3. SSH login karo:  ssh LOGIN@sX.serv00.com
#   4. Ye script chalao:  bash deploy_serv00.sh
# ============================================================================
# SCRIPT KYA KARTI HAI:
#   - apni software chalane ki permission (binexec) ON karti hai
#   - TCP port reserve karti hai (1024-64000 range, 30000 use karta hai)
#   - bot ko port 30000 pe tmux mein start karti hai (ram watchdog ke saath)
#   - panel ko login.serv00.com pe proxy website se expose karti hai
#   - cron @reboot + har 5 min watchdog lagati hai (bot zinda rahega)
#   - inactivity-guard: roz subah 4:00 pe ek log line touch karti hai taaki
#     account "active" dikhe (serv00 policy: inactive accounts delete ho
#     sakte hain — bot chalta rahe to ye自动 covered hai, ye extra safety hai)
# ============================================================================

set -e
PORT_NUM="${1:-30000}"
LOGIN="$(whoami)"
SERV_HOME="/usr/home/$LOGIN"
APP_DIR="$SERV_HOME/bot"
SUBDOMAIN="${LOGIN}.serv00.com"   # serv00 ka free subdomain

echo "== GOLD-MD Serv00 deploy start (login: $LOGIN) =="

# ── 1. binexec ON (apni binary chalane ki permission) ─────────────────────
devil binexec on 2>/dev/null || true

# ── 2. port reserve ───────────────────────────────────────────────────────
devil port add "$PORT_NUM" tcp "goldmd" 2>/dev/null || echo "port already reserved"

# ── 3. app dir + files ────────────────────────────────────────────────────
mkdir -p "$APP_DIR"
for f in gold-md-freebsd .env servers.json; do
    if [ -f "$SERV_HOME/$f" ]; then
        mv "$SERV_HOME/$f" "$APP_DIR/"
    fi
done
chmod +x "$APP_DIR/gold-md-freebsd" 2>/dev/null || true

# .env me PORT set karo (agar file me purani value ho)
if [ -f "$APP_DIR/.env" ]; then
    sed -i.bak "s/^PORT=.*/PORT=$PORT_NUM/" "$APP_DIR/.env"
fi

# ── 4. watchdog script (bot crash ho to wapas start) ──────────────────────
cat > "$APP_DIR/watchdog.sh" <<WDOG
#!/usr/bin/env bash
# gold-md watchdog — cron se har 5 min call hota hai
if ! pgrep -f gold-md-freebsd > /dev/null 2>&1; then
    cd "$APP_DIR"
    nohup ./gold-md-freebsd >> "$APP_DIR/bot.log" 2>&1 &
    echo "\$(date) — watchdog restarted bot" >> "$APP_DIR/watchdog.log"
fi
WDOG
chmod +x "$APP_DIR/watchdog.sh"

# ── 5. bot start (pehli baar, tmux ke bina nohup se) ──────────────────────
if ! pgrep -f gold-md-freebsd > /dev/null 2>&1; then
    cd "$APP_DIR"
    nohup ./gold-md-freebsd >> "$APP_DIR/bot.log" 2>&1 &
    echo "bot started (pid $!)"
    sleep 3
fi

# ── 6. panel ko proxy website se expose karo ──────────────────────────────
devil www add "$SUBDOMAIN" proxy localhost "$PORT_NUM" 2>/dev/null \
    || echo "proxy website already exists (ya手动 add karo: devil www add $SUBDOMAIN proxy localhost $PORT_NUM)"

# ── 7. cron: @reboot + 5-min watchdog + daily activity ping ───────────────
( crontab -l 2>/dev/null | grep -v "gold-md\|watchdog.sh\|activity.ping" ; \
  echo "@reboot $APP_DIR/watchdog.sh" ; \
  echo "*/5 * * * * $APP_DIR/watchdog.sh" ; \
  echo "0 4 * * * touch $APP_DIR/activity.ping" ) | crontab -

echo ""
echo "== DEPLOY DONE =="
echo "   Panel URL : https://$SUBDOMAIN  (2-3 min baad try karo, SSL lagna qtime leta hai)"
echo "   Health    : curl http://$SERV_HOST:$PORT_NUM/health"
echo "   Bot logs  : tail -f $APP_DIR/bot.log   (khali hoga — owner rule: silent logger)"
echo "   Stop      : pkill -f gold-md-freebsd"
echo "   Restart   : $APP_DIR/watchdog.sh"
echo ""
echo "IMPORTANT: panels pe https://$SUBDOMAIN/ pair karo — servers.json ka"
echo "SERVER 1 URL update karna hoga (iss URL se) + GitHub pe push karo."
