#!/usr/bin/env python3
# storage.go patches for Storadera migration:
# 1) un-comment + activate storjDebug (JSON logs — owner: "json logs lazmi lagana")
# 2) hardcode Storadera creds (owner: "env nahi banana, hardcode kr")
# 3) Region: storjRegion const use
import re, sys

P = "src/storage.go"
src = open(P, encoding="utf-8").read()
orig = src

# ── 1) Activate storjDebug: replace whole commented block ──
commented = re.search(
    r'// func storjDebug\(stage string, fields map\[string\]any\) \{.*?\n// \}\n',
    src, re.S)
if not commented:
    print("PATCH1 FAIL: commented storjDebug block not found")
    sys.exit(1)

active_fn = '''// storjDebug — OWNER REQUEST (2026-09-16): JSON logs LAZMI ("json debug b laga do"
// + "json logs lazmi lagana me pucho ga"). Har storage stage ki ek JSON line
// stderr pe jaati hai (bot_*.log me) — Storadera data-flow (ja raha / aa raha
// kya hai) is se verify hota hai. Tag [KV-JSON] grep-friendly hai.
func storjDebug(stage string, fields map[string]any) {
	out := map[string]any{"stage": stage, "ts": time.Now().Format(time.RFC3339Nano)}
	for k, v := range fields {
		out[k] = v
	}
	raw, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s [KV-JSON] marshal-err: %v stage=%s\\n", time.Now().Format("15:04:05"), err, stage)
		return
	}
	fmt.Fprintf(os.Stderr, "%s [KV-JSON] %s\\n", time.Now().Format("15:04:05"), string(raw))
}
'''
src = src.replace(commented.group(0), active_fn, 1)

# ── 2) Hardcode Storadera creds (single shard set, same [10][3] shape) ──
old_creds = re.search(r'var hardcodedStorjShards = \[10\]\[3\]string\{.*?\n\}\n', src, re.S)
if not old_creds:
    print("PATCH2 FAIL: hardcodedStorjShards not found")
    sys.exit(1)

new_creds = '''// hardcodedStorjShards — STORADERA (2026-09-16, owner jani ne diye).
// OWNER DIRECTIVE: "env nahi banana jese hardcord kam kr rhe to hardcode kr
// files me" — is liye naye creds files me hi hardcode hain, .env NAHI.
// Single credential set: sab 10 slots same creds+bucket ("goldmd") —
// shardForID ka FNV-1a % 10 routing ab effectively ek hi bucket pe
// converge karta hai (migration ke liye INTENTIONAL — purane Storj
// shard-buckets ka data move nahi karna, clean start).
var hardcodedStorjShards = [10][3]string{
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
}
'''
src = src.replace(old_creds.group(0), new_creds, 1)

# ── 3) InitStorj: env-first → hardcode-first (env override hata do) ──
old_env_load = '''		access := os.Getenv(fmt.Sprintf("STORJ_ACCESS_KEY_%d", i))
		secret := os.Getenv(fmt.Sprintf("STORJ_SECRET_KEY_%d", i))
		bucket := os.Getenv(fmt.Sprintf("STORJ_BUCKET_%d", i))
		// .env skipped on some hosts (Modal etc.) — use embedded fallbacks
		if access == "" || secret == "" || bucket == "" {
			access = hardcodedStorjShards[i-1][0]
			secret = hardcodedStorjShards[i-1][1]
			bucket = hardcodedStorjShards[i-1][2]
		}
		if access == "" || secret == "" || bucket == "" {
			continue
		}
'''
new_env_load = '''		// OWNER DIRECTIVE (2026-09-16): env NAHI — seedha hardcoded creds.
		// (Purana Storj env-set (STORJ_ACCESS_KEY_1..10) agar kahin host pe
		// pada ho to wo IGNORE hoga — hardcodedStorjShards hi source of truth.)
		access := hardcodedStorjShards[i-1][0]
		secret := hardcodedStorjShards[i-1][1]
		bucket := hardcodedStorjShards[i-1][2]
		if access == "" || secret == "" || bucket == "" {
			continue
		}
'''
if old_env_load not in src:
    print("PATCH3 FAIL: env-load block not found")
    sys.exit(1)
src = src.replace(old_env_load, new_env_load, 1)

# ── 4) Region: storjRegion const use in minio.New ──
old_region = '\t\tRegion: "us-east-1", // Storj gateway expects a region string; this is a placeholder.'
if old_region not in src:
    print("PATCH4 FAIL: region line not found")
    sys.exit(1)
src = src.replace(old_region, '\t\tRegion: storjRegion, // Storadera eu-east-1 (hardcoded const).', 1)

# ── 5) MakeBucket: region pass karo (Storadera needs it) ──
old_mk = '\t\t\tif err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {'
if old_mk not in src:
    print("PATCH5 FAIL: MakeBucket line not found")
    sys.exit(1)
src = src.replace(old_mk, '\t\t\tif err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: storjRegion}); err != nil {', 1)

if src == orig:
    print("NO CHANGES — abort")
    sys.exit(1)

open(P, "w", encoding="utf-8").write(src)
print("ALL 5 PATCHES OK")
