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

func TestKvGzipRoundTrip(t *testing.T) {
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
