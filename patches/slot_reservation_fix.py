#!/usr/bin/env python3
# SLOT RESERVATION RACE FIX (Task J — failover re-verification me pakda gaya):
# Count() sirf CONNECTED sessions ginta hai, StartSession session map me
# connect hone ke BAAD daalta hai → AutoLoad batch-5 mass-boot / concurrent
# panel pairings race window me sab Count()=0 dekh kar pass → FULL 5/2
# over-max (owner ne panel me exactly ye dekha tha). FIX: atomic slot
# reservation (reserveSlot/SlotsUsed) — live + in-flight dono ginke.
# Ye patch script REFERENCE ke liye hai — asli changes manager.go,
# fleet.go, panel.go me manually apply hue hain (test: slot_reservation_test.go).
