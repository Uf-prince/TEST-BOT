#!/usr/bin/env python3
# PATCH 3: PRE-CRASH warning — egress ~90% of 5GB budget cross hone par
# is server ke LOCAL session owners ko STOPPING warning bhejo (once/month).
# Owner ka exact format (server number ke saath).
P = 'fleet.go'
src = open(P).read()
orig = src

# ── A. state var + const (egress vars ke paas, fleetEgressMonth ke baad) ──
old_vars = '''\tfleetEgressMu     sync.Mutex
\tfleetEgressTotal  int64  // bytes this month
\tfleetEgressLastTx int64  // last /proc snapshot
\tfleetEgressMonth  string // "2025-01"
'''
assert src.count(old_vars) == 1, 'egress vars anchor missing'
new_vars = '''\tfleetEgressMu     sync.Mutex
\tfleetEgressTotal  int64  // bytes this month
\tfleetEgressLastTx int64  // last /proc snapshot
\tfleetEgressMonth  string // "2025-01"

\t// pre-crash warning state — ek hi baar per month per process (render
\t// bandwidth khatam hone se PEHLE owners ko batado).
\tfleetWarnMu      sync.Mutex
\tfleetWarnedMonth string // "2025-01" — jis month warning chala
'''
src = src.replace(old_vars, new_vars)

# ── B. warning func — fleetPushEgress ke baad insert ──
anchor = '''// fleetLoadEgress restores the persisted egress counters at boot (survives
// restarts). Boot-gap > 2 ticks → lastTx reset (downtime me egress zero).
func fleetLoadEgress() {'''
assert src.count(anchor) == 1, 'fleetLoadEgress anchor missing'

warn_func = r'''
// fleetWarnPreCrash: RENDER PRE-CRASH WARNING (owner order) — jab is server
// ka egress ~90% of 5GB budget cross ho jaye (Render bandwidth khatam hone
// wala hai), is server ke LOCAL connected session owners ko PEHLE bata do:
//
//	*GOLD-MD SERVER ❮ N ❯ STOPPING*
//
//	*YOUR BOT IS MOVING TO ANOTHER SERVER NO NEED TO PAIR AGAIN YOUR BOT
//	COME BACK ONLINE IN 2/3 MINTS ONLY PLEASE WAIT.....*
//
// Ek hi baar per calendar month per process — spam nahi. Warning ke baad
// egress natural continue hota hai (sirf $100GB hard-stop tak alag).
const fleetWarnThreshold = 0.90 // 90% of budget

func fleetWarnPreCrash() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	used := fleetEgressUsedMB()
	budget := float64(fleetBudgetMB)
	if budget <= 0 || used < budget*fleetWarnThreshold {
		return
	}
	fleetWarnMu.Lock()
	month := time.Now().UTC().Format("2006-01")
	if fleetWarnedMonth == month {
		fleetWarnMu.Unlock()
		return // is month warning already chal chuka
	}
	fleetWarnedMonth = month // abhi mark karo — send fail ho to next boot
	fleetWarnMu.Unlock()

	srvNum := fleetServerNumberForSID(fleetSelfID)
	text := "*GOLD-MD SERVER \u276e " + srvNum + " \u276f STOPPING*\n\n" +
		"*YOUR BOT IS MOVING TO ANOTHER SERVER NO NEED TO PAIR AGAIN YOUR BOT COME BACK ONLINE IN 2/3 MINTS ONLY PLEASE WAIT.....*"

	sent := 0
	for _, sess := range m.List() {
		if sess.Client == nil || !sess.Client.IsConnected() {
			continue
		}
		owner := fleetOwnerFor(sess.JID)
		if owner == "" {
			continue
		}
		ownerJID, err := types.ParseJID(normalizeJID(owner))
		if err != nil || ownerJID.IsEmpty() {
			continue
		}
		if _, err := sess.Client.SendMessage(context.Background(), ownerJID, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: &text},
		}); err == nil {
			sent++
		}
	}
	InfoLog("FLEET: pre-crash warning sent to %d owners (egress %.0fMB / %dMB budget)", sent, used, fleetBudgetMB)
}

'''
src = src.replace(anchor, warn_func + anchor)

# ── C. watchdog me call add (fleetPushEgress ke baad) ──
old_tick = '''\t\tfleetHeartbeat()
\t\tfleetPushEgress()
'''
assert src.count(old_tick) == 1, 'watchdog tick anchor missing'
new_tick = '''\t\tfleetHeartbeat()
\t\tfleetPushEgress()
\t\tfleetWarnPreCrash()
'''
src = src.replace(old_tick, new_tick)

open(P, 'w').write(src)
print('PATCH 3 OK: fleet.go', len(orig), '->', len(src))
