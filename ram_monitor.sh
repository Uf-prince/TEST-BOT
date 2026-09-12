#!/bin/bash
# GOLD-MD RAM monitor - every 5s snapshot to CSV
PID=$(pgrep -x gold-md-lowram | head -1)
if [ -z "$PID" ]; then echo "BOT_NOT_RUNNING"; exit 1; fi
OUT=/workspace/GOLD-MD/ram_monitor.csv
echo "time,rss_mb,vsz_mb,cpu_pct,sessions" > "$OUT"
while true; do
    # Bot process RAM
    LINE=$(ps -o rss=,vsz=,pcpu= -p "$PID" 2>/dev/null)
    [ -z "$LINE" ] && break
    RSS=$(echo $LINE | awk '{printf "%.1f", $1/1024}')
    VSZ=$(echo $LINE | awk '{printf "%.1f", $2/1024}')
    CPU=$(echo $LINE | awk '{print $3}')
    # Session count
    SESS=$(curl -s --max-time 3 http://127.0.0.1:11221/sessions 2>/dev/null | grep -o '"count":[0-9]*' | cut -d: -f2)
    [ -z "$SESS" ] && SESS="na"
    echo "$(date +%H:%M:%S),$RSS,$VSZ,$CPU,$SESS" >> "$OUT"
    sleep 5
done
