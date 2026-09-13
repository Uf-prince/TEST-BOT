#!/usr/bin/env python3
# WHATSAPP-TRUTH COUNT FIX — "Storj jhuta count na kare":
#   Count() sirf s.Paired (memory flag, Storj-restore/boot race me set) ginta
#   tha. WhatsApp ne logout kiya ho (logged-out event abhi nahi aaya / miss
#   hua) to bhi count jhuta rehta tha \u2014 FULL 3/2 jaisa over-count + fleet
#   quota jhuti block. Ab:
#   1) Count() = Paired && Client!=nil && IsConnected() && Store.ID!=nil
#      (panel.go ke WhatsApp-truth online check jaisa hi \u2014 disconnect/logout
#      pe count TURANT gir jayega, Storj ka flag kuch kehta rahe)
#   2) reviveIfDead: healthy-socket par bhi IsLoggedIn() lint \u2014 socket
#      zinda magar login-token dead (logout event miss) to cleanupSession
#      (fleetOnCleanup chalta hai \u2014 Storj + fleet registry se bhi saaf)
import sys
def die(m): print("PATCH-FAIL:", m); sys.exit(1)

# ---------- 1) manager.go: Count() WhatsApp-truth ----------
src = open("manager.go").read()
old_count = '''func (m *Manager) Count() int {
\tm.mu.Lock()
\tdefer m.mu.Unlock()
\tn := 0
\tfor _, s := range m.sessions {
\t\tif s != nil && s.Paired {
\t\t\tn++
\t\t}
\t}
\treturn n
}
'''
new_count = '''func (m *Manager) Count() int {
\tm.mu.Lock()
\tdefer m.mu.Unlock()
\tn := 0
\tfor _, s := range m.sessions {
\t\t// WHATSAPP IS TRUTH: sirf Paired flag (memory/Storj-restore) kaafi
\t\t// nahi. Socket zinda + device linked tabhi gino \u2014 WhatsApp logout
\t\t// kar de to Storj/flag jhuta bole to bhi count sahi rahega.
\t\tif s != nil && s.Paired && s.Client != nil &&
\t\t\ts.Client.IsConnected() &&
\t\t\ts.Client.Store != nil && s.Client.Store.ID != nil {
\t\t\tn++
\t\t}
\t}
\treturn n
}
'''
if new_count in src:
    print("Count() already patched, SKIP")
elif src.count(old_count) == 1:
    src = src.replace(old_count, new_count)
    print("Count() WhatsApp-truth: OK")
else:
    die(f"Count anchor count={src.count(old_count)}")

# ---------- 2) reconnect_watchdog.go: logout-linter healthy path ----------
wsrc = open("reconnect_watchdog.go").read()
old_revive = '''\tif s.Client.IsConnected() {
\t\t// Healthy: reset attempts so a future blip again gets fast retries.
\t\tm.resetAttempts(s.JID)
\t\treturn true
\t}
'''
new_revive = '''\tif s.Client.IsConnected() {
\t\t// LOGOUT-LINTER (WhatsApp-truth): socket zinda dikhta hai magar
\t\t// login-token dead (WhatsApp ne logout kiya, event miss/out-of-band)
\t\t// to ye session zinda NAHI hai. Storj/Paired-flag jhuta bole to bhi
\t\t// yahan cleanupSession chalao \u2014 LoggedOut handler jaisa hi effect
\t\t// (device + Redis + fleet registry saaf, count turant gir jayega).
\t\tif !s.Client.IsLoggedIn() {
\t\t\tWarnLog("\u26d1 WATCHDOG: %s socket alive but NOT logged in \u2014 WhatsApp-side logout cleanup", s.JID)
\t\t\tm.cleanupSession(s, "watchdog: socket alive but login dead")
\t\t\treturn true // cleanup ho gaya \u2014 ye pass healthy maano
\t\t}
\t\t// Healthy: reset attempts so a future blip again gets fast retries.
\t\tm.resetAttempts(s.JID)
\t\treturn true
\t}
'''
if new_revive in wsrc:
    print("watchdog lint: already patched, SKIP")
elif wsrc.count(old_revive) == 1:
    wsrc = wsrc.replace(old_revive, new_revive)
    print("watchdog logout-linter: OK")
else:
    die(f"revive anchor count={wsrc.count(old_revive)}")

open("manager.go", "w").write(src)
open("reconnect_watchdog.go", "w").write(wsrc)
print("ALL PATCHES OK")
