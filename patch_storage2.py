#!/usr/bin/env python3
# Storadera migration PATCH-SET 2: JSON debug activation
#   A) un-comment all 27 storjDebug call sites (multi-line aware)
#   B) fix stale STORJ_*-env text in no-creds branch
#   C) KV_OP JSON debug at the public cmd() funnel (writes always, reads on
#      error / GOLDMD_KV_JSON_READS=1 verification mode)
#   D) retry-queue debug (RETRY_QUEUE / RETRY_PURGE / RETRY_REPLAY)
import re, sys

P = 'src/storage.go'
src = open(P, encoding='utf-8').read()
n_orig = len(src)

# ── PATCH A: activate ALL commented storjDebug call sites ──────────────
lines = src.split('\n')
out, i = [], 0
activated, in_block = 0, False
while i < len(lines):
    ln = lines[i]
    if not in_block and re.match(r'^(\s*)// storjDebug\(', ln):
        in_block = True
        activated += 1
        out.append(re.sub(r'^(\s*)// ', r'\1', ln, count=1))
    elif in_block:
        if re.match(r'^\s*// ', ln):
            out.append(re.sub(r'^(\s*)// ', r'\1', ln, count=1))
            if re.search(r'\)\s*$', ln):   # closing "})" — block ends
                in_block = False
        else:                               # non-comment line — block ended
            in_block = False
            out.append(ln)
    else:
        out.append(ln)
    i += 1
src = '\n'.join(out)
print(f"A: activated {activated} storjDebug call sites")
assert activated == 27, f"expected 27 call sites, got {activated}"

# ── PATCH B: stale env-text fix (env lookup already removed in patch-set 1)
b1 = 'storjDebug("STORJ_INIT", map[string]any{"ok": false, "reason": "no STORJ_* env credentials found", "shardCount": 0})'
b1n = 'storjDebug("STORJ_INIT", map[string]any{"ok": false, "reason": "no hardcoded credentials (hardcodedStorjShards empty)", "shardCount": 0})'
assert b1 in src, "B1 anchor missing"
src = src.replace(b1, b1n)
b2 = 'return errors.New("storj: no credential sets configured (STORJ_ACCESS_KEY_1..10 / STORJ_SECRET_KEY_1..10 / STORJ_BUCKET_1..10)")'
b2n = 'return errors.New("storj: hardcodedStorjShards me koi credential set configured nahi hai")'
assert b2 in src, "B2 anchor missing"
src = src.replace(b2, b2n)
print("B: stale env-text fixed (2 replaces)")

# ── PATCH C: KV_OP JSON debug — insert helpers BEFORE "cmd is the public entry point" comment
helpers = '''// ── KV_OP JSON debug (owner directive 2026-09-16: "json logs lazmi") ──
// HAR KV op (settings, fleet, sessiondb — sab public cmd() se guzarte hain)
// ka JSON log [KV-JSON] tag ke sath stderr (bot_11221.log) pe jata hai.
// WRITE ops HAMESHA log hote hain (data Storadera pe JA raha dikhe); READ
// ops sirf error pe — ya GOLDMD_KV_JSON_READS=1 verification mode me sab
// (flood control: cache-served reads normally chup rehte hain).
// Value KABHI log nahi hoti — sirf key/field (80-char truncated) + sizes
// + ok/err. Secret/credential leak ka koi chance nahi.
var kvDebugReads = strings.TrimSpace(os.Getenv("GOLDMD_KV_JSON_READS")) == "1"

func kvLogKey(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "…+" + strconv.Itoa(len(s)-80)
}

func kvLogOp(args []string, res json.RawMessage, err error) {
	if len(args) == 0 {
		return
	}
	op := strings.ToUpper(args[0])
	if !storjWriteRetryable(args) && err == nil && !kvDebugReads {
		return // read + ok + verification-mode-off → silent (flood control)
	}
	f := map[string]any{"op": op, "ok": err == nil}
	if len(args) > 1 {
		f["key"] = kvLogKey(args[1])
	}
	if len(args) > 2 && (op == "HSET" || op == "HGET" || op == "HDEL" || op == "HEXISTS") {
		f["field"] = kvLogKey(args[2])
	}
	if op == "SET" && len(args) > 2 {
		f["valBytes"] = len(args[2])
	}
	if op == "HSET" && len(args) > 3 {
		f["valBytes"] = len(args[3])
	}
	if op == "SADD" || op == "SREM" {
		if len(args) > 2 {
			f["members"] = len(args) - 2
		}
	}
	if res != nil {
		f["resBytes"] = len(res)
	}
	if err != nil {
		f["error"] = err.Error()
	}
	storjDebug("KV_OP", f)
}

'''
anchor_c = '// cmd is the public entry point (all internal + external callers use this).'
assert anchor_c in src, "C anchor missing"
src = src.replace(anchor_c, helpers + anchor_c, 1)
print("C: kvLogOp/kvLogKey helpers inserted before cmd()")

# ── PATCH C2: hook kvLogOp into the cmd() funnel
old_c2 = '''func (u *Upstash) cmd(args ...string) (json.RawMessage, error) {
	res, err := u.cmdCore(args...)
	if err != nil {'''
new_c2 = '''func (u *Upstash) cmd(args ...string) (json.RawMessage, error) {
	res, err := u.cmdCore(args...)
	kvLogOp(args, res, err) // [KV-JSON] owner debug — har KV op ka JSON log
	if err != nil {'''
assert old_c2 in src, "C2 anchor missing"
src = src.replace(old_c2, new_c2, 1)
print("C2: kvLogOp hooked into cmd()")

# ── PATCH D: retry-queue debug
old_d1 = '''	u.retryOps = append(u.retryOps, storjRetryOp{args: cp, ts: time.Now()})
	n := len(u.retryOps)
	u.retryMu.Unlock()'''
new_d1 = '''	u.retryOps = append(u.retryOps, storjRetryOp{args: cp, ts: time.Now()})
	n := len(u.retryOps)
	u.retryMu.Unlock()
	storjDebug("RETRY_QUEUE", map[string]any{"op": cp[0], "key": kvLogKey(cp[1]), "pending": n})'''
assert old_d1 in src, "D1 anchor missing"
src = src.replace(old_d1, new_d1, 1)
print("D1: RETRY_QUEUE debug added")

old_d2 = '''	u.retryOps = kept
	u.retryMu.Unlock()
	if dropped > 0 {'''
new_d2 = '''	u.retryOps = kept
	u.retryMu.Unlock()
	if dropped > 0 {
		storjDebug("RETRY_PURGE", map[string]any{"dropped": dropped, "supersededBy": succOp, "key": kvLogKey(key)})'''
assert old_d2 in src, "D2 anchor missing"
src = src.replace(old_d2, new_d2, 1)
print("D2: RETRY_PURGE debug added")

old_d3 = '''	_ = left
	if len(failed) > 0 {'''
new_d3 = '''	_ = left
	storjDebug("RETRY_REPLAY", map[string]any{"tried": len(ops), "ok": len(ops) - len(failed), "failed": len(failed), "stillPending": left})
	if len(failed) > 0 {'''
assert old_d3 in src, "D3 anchor missing"
src = src.replace(old_d3, new_d3, 1)
print("D3: RETRY_REPLAY debug added")

open(P, 'w', encoding='utf-8').write(src)
print(f"ALL PATCHES OK — file {n_orig} → {len(src)} bytes")
