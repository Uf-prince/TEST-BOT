#!/usr/bin/env python3
# WAR-LOOP FIX part 4: WAR GUARD circuit breaker.
# Even with fresh-claim trust, an OLD-binary attacker (deployed before this
# fix) can keep grabbing the claim. To GUARANTEE the war ends, the WAR GUARD
# gives up (surrender) after N retakes of the same JID within a window.
import io, sys

SG = "src/session_guards.go"

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

src = read(SG)

# 1. Add circuit-breaker state + constants right after warRetakeDelay.
old_const = '''// warRetakeDelay: StreamReplaced milne ke baad kitni der me RETAKE attempt
// (1s \u2014 jaldi, attacker ko settle hone ka mauka hi na do).
const warRetakeDelay = 1 * time.Second'''
new_const = '''// warRetakeDelay: StreamReplaced milne ke baad kitni der me RETAKE attempt
// (1s \u2014 jaldi, attacker ko settle hone ka mauka hi na do).
const warRetakeDelay = 1 * time.Second

// \u2500\u2500 WAR GUARD CIRCUIT BREAKER (WAR-LOOP FIX 2026-09-21) \u2500\u2500
// Guarantee: war KABHI infinite nahi. Agar ek hi JID ke liye humne
// warRetakeMax baar retake kar liya (warRetakeWindow ke andar) to hum
// SURRENDER kar dete hain \u2014 chahe claim state kuch bhi ho. Ye un
// attackers ke against bhi kaam karta hai jo PURANE binary pe hain
// (fresh-claim fix unke paas nahi hai) aur claim grab karte rehte hain.
const (
\twarRetakeMax    = 3                // itne retakes ke baad surrender
\twarRetakeWindow = 5 * time.Minute  // is window me ginti
)

var (
\twarRetakeMu    sync.Mutex
\twarRetakeCount = map[string]int{}
\twarRetakeFirst = map[string]time.Time{}
)

// warRetakeAllowed: circuit breaker \u2014 is JID ke liye retake allowed hai?
// false = limit cross \u2192 caller surrender kare (war khatam).
func warRetakeAllowed(jid string) bool {
\twarRetakeMu.Lock()
\tdefer warRetakeMu.Unlock()
\tfirst, ok := warRetakeFirst[jid]
\tif !ok || time.Since(first) > warRetakeWindow {
\t\t// naya window
\t\twarRetakeFirst[jid] = time.Now()
\t\twarRetakeCount[jid] = 0
\t\treturn true
\t}
\treturn warRetakeCount[jid] < warRetakeMax
}

// warRetakeRecord: ek retake attempt record karo.
func warRetakeRecord(jid string) {
\twarRetakeMu.Lock()
\twarRetakeCount[jid]++
\twarRetakeMu.Unlock()
}

// warRetakeReset: session stable ho gaya \u2014 counter saaf (agli war fresh ginti).
func warRetakeReset(jid string) {
\twarRetakeMu.Lock()
\tdelete(warRetakeCount, jid)
\tdelete(warRetakeFirst, jid)
\twarRetakeMu.Unlock()
}'''
if old_const not in src:
    print("ERR: warRetakeDelay const not found")
    sys.exit(1)
src = src.replace(old_const, new_const, 1)

# 2. In warRetake: check circuit breaker before retaking.
old_retake = '''\tjid := s.JID
\tgo func() {
\t\tdefer func() { _ = recover() }()
\t\ttime.Sleep(warRetakeDelay)
\t\tif m.IsShuttingDown() {
\t\t\treturn
\t\t}
\t\t// dobara sach check: ab bhi koi aur live holder nahi? (race me
\t\t// takeover ho gaya ho to surrender hi sahi hai)
\t\tif fleetOtherLiveHolder(jid) {
\t\t\tm.surrenderSession(s, "retake ke waqt doosra live holder mila")
\t\t\treturn
\t\t}'''
new_retake = '''\tjid := s.JID
\t// CIRCUIT BREAKER (WAR-LOOP FIX): agar humne is JID ko window me
\t// warRetakeMax baar retake kar liya \u2014 ab SURRENDER (war khatam).
\t// Ye purane-binary attackers ke against bhi war rokta hai.
\tif !warRetakeAllowed(jid) {
\t\tm.surrenderSession(s, "circuit breaker \u2014 retake limit cross, war khatam")
\t\treturn
\t}
\twarRetakeRecord(jid)
\tgo func() {
\t\tdefer func() { _ = recover() }()
\t\ttime.Sleep(warRetakeDelay)
\t\tif m.IsShuttingDown() {
\t\t\treturn
\t\t}
\t\t// dobara sach check: ab bhi koi aur live holder nahi? (race me
\t\t// takeover ho gaya ho to surrender hi sahi hai)
\t\tif fleetOtherLiveHolder(jid) {
\t\t\tm.surrenderSession(s, "retake ke waqt doosra live holder mila")
\t\t\treturn
\t\t}'''
if old_retake not in src:
    print("ERR: warRetake body not found")
    sys.exit(1)
src = src.replace(old_retake, new_retake, 1)

write(SG, src)
print("OK: session_guards.go patched (circuit breaker)")
