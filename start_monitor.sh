#!/bin/bash
# Detached launcher for RAM monitor
pkill -f ram_monitor.sh 2>/dev/null
sleep 1
rm -f /workspace/GOLD-MD/ram_monitor.csv
cd /workspace/GOLD-MD
setsid nohup bash ram_monitor.sh > /dev/null 2>&1 < /dev/null &
disown -a
echo "MONITOR_STARTED"
exit 0
