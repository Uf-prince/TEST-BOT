package main

// BANDWIDTH JUGAD — transparent gzip round-trip tests (no network).
// Ye tests prove karte hain: value semantics 100% same rehte hain —
// legacy plain values as-is, badi values GZ1: me chhoti hoti hain,
// aur read-side decompress exact original deta hai.

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// EMERGENCY NOTE: GZ1 default OFF hai (mixed-fleet safety). Round-trip
// tests override se ON karke check karte hain - production wire format
// plain rehta hai jab tak GOLDMD_KV_GZIP=1 na ho.

func withKvGzipOn(t *testing.T) {
	t.Helper()
	old := kvGzipEnabled
	kvGzipEnabled = true
	t.Cleanup(func() { kvGzipEnabled = old })
}

func TestKvGzipDefaultOffLegacyWire(t *testing.T) {
	// EMERGENCY regression guard: default (no env) me compression kabhi
	// nahi honi chahiye - wire format 100% legacy plain (purane binaries
	// fleet-wide safe). Ye test fail hona kabhi acceptable NAHI.
	big := strings.Repeat("A", 100000)
	if got := kvGzipMaybe(big); got != big {
		t.Fatalf("default OFF hai to badi value bhi plain honi chahiye (len=%d)", len(got))
	}
}

func TestKvGzipRoundTrip(t *testing.T) {
	withKvGzipOn(t)
	// base64-sqlite jaisa realistic payload: 60KB random-ish + pattern
	raw := bytes.Repeat([]byte("SQLite-format 3\x00goldmd.db rows here..."), 1800)
	enc := base64.StdEncoding.EncodeToString(raw)

	comp := kvGzipMaybe(enc)
	if comp == enc {
		t.Fatalf("big base64 payload compress hona chahiye tha (len=%d)", len(enc))
	}
	if !strings.HasPrefix(comp, kvGzPrefix) {
		t.Fatalf("compressed body me %q prefix hona chahiye", kvGzPrefix)
	}
	// ~10x+ savings expect (base64 text gzip me bahut accha compress hota hai)
	if len(comp) > len(enc)/5 {
		t.Errorf("compression kam hai: %d -> %d (5x se behtar expect)", len(enc), len(comp))
	}

	// decompress → EXACT original
	back := kvGunzipMaybe([]byte(comp))
	if !bytes.Equal(back, []byte(enc)) {
		t.Fatalf("round-trip mismatch: len %d vs %d", len(back), len(enc))
	}
}

func TestKvGzipSmallStaysPlain(t *testing.T) {
	// chhoti values (settings, meta, egress) plain hi — GZ overhead bekar
	small := "mode=on"
	if kvGzipMaybe(small) != small {
		t.Errorf("chhoti value plain honi chahiye (4KB threshold)")
	}
	if kvGzipMaybe("") != "" {
		t.Errorf("empty value plain")
	}
}

func TestKvGunzipLegacyPassthrough(t *testing.T) {
	// legacy plain value (purana binary ne likha) — UNTOUCHED return
	legacy := "plain-old-base64-blob-no-prefix"
	out := kvGunzipMaybe([]byte(legacy))
	if string(out) != legacy {
		t.Errorf("legacy value passthrough hona chahiye, mila %q", string(out))
	}
	// corrupt GZ1: prefix (invalid gzip) → best-effort fallback: raw as-is
	bad := kvGzPrefix + "not-really-gzip-data"
	out2 := kvGunzipMaybe([]byte(bad))
	if string(out2) != bad {
		t.Errorf("corrupt GZ1 fallback raw as-is hona chahiye")
	}
}

func TestKvGzipIncompressibleStaysPlain(t *testing.T) {
	// random bytes base64 (incompressible) — plain hi jana chahiye
	// (length threshold cross + gzip no help → no prefix)
	var rnd []byte
	seed := uint32(123456789)
	for i := 0; i < 8000; i++ { // ~8KB
		seed = seed*1664525 + 1013904223
		rnd = append(rnd, byte(seed>>16))
	}
	val := base64.StdEncoding.EncodeToString(rnd)
	comp := kvGzipMaybe(val)
	// incompressible data pe ya to plain, ya GZ1: lekin bada nahi hona chahiye
	if strings.HasPrefix(comp, kvGzPrefix) && len(comp) >= len(val) {
		t.Errorf("incompressible value GZ me badi ho gayi — plain hona chahiye tha")
	}
}

// TestGZ1HealMakesOldBinaryReadable — FAILOVER GUARD: broken build (c704231)
// ne GZ1 likha tha; HealGZ1Key unhe gunzip karke PLAIN rewrite karta hai.
// Ye test simulate karta hai: GZ1 stored value -> heal -> naya stored body
// plain -> PURANA binary (plain base64 reader, jaise fleetRestoreBlob) exact
// DB bytes decode kar leta hai. Ek server band ho to doosra (purana binary
// bhi) session le sake — mixed-fleet failover ka format-level guarantee.
func TestGZ1HealMakesOldBinaryReadable(t *testing.T) {
	withKvGzipOn(t) // sirf GZ1 banana ke liye (jaise broken build ne likha tha)

	// realistic sqlite-ish DB (session blob)
	rawDB := bytes.Repeat([]byte("SQLite-format 3\x00goldmd-session-rows..."), 500)
	legacyB64 := base64.StdEncoding.EncodeToString(rawDB)

	// broken build ne aise likha tha:
	brokenStored := kvGzipMaybe(legacyB64)
	if !strings.HasPrefix(brokenStored, kvGzPrefix) {
		t.Fatalf("precondition fail: broken stored value GZ1 honi chahiye")
	}

	// PURANA binary plain base64 padhta hai — GZ1 pe FAIL (outage root-cause
	// reproduce: isliye sessions offline gaye the)
	if _, err := base64.StdEncoding.DecodeString(brokenStored); err == nil {
		t.Fatalf("GZ1 purane binary-style reader ke liye readable nahi hona chahiye (outage root-cause)")
	}

	// HEAL: HealGZ1Key ka core path — gunzip -> plain rewrite
	healed := string(kvGunzipMaybe([]byte(brokenStored)))
	if healed != legacyB64 {
		t.Fatalf("heal ke baad exact legacy value wapas aani chahiye (got len=%d want len=%d)",
			len(healed), len(legacyB64))
	}

	// heal ke BAAD purana binary wapas padh leta hai — failover restored:
	back, err := base64.StdEncoding.DecodeString(healed)
	if err != nil {
		t.Fatalf("healed value purane binary-style reader ke liye valid base64 nahi: %v", err)
	}
	if !bytes.Equal(back, rawDB) {
		t.Fatalf("old-binary decode -> DB bytes mismatch (got len=%d want len=%d)",
			len(back), len(rawDB))
	}
}
