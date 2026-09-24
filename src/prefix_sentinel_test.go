package main

import "testing"

// newPrefixTestUpstash builds an Upstash with only the in-memory cache wired,
// which is all GetPrefix touches on the cache-hit path.
func newPrefixTestUpstash() *Upstash {
	return &Upstash{cache: map[string]cacheEntry{}}
}

// TestGetPrefixNeverReturnsMissingSentinel guards the regression where a
// session that had never set a prefix showed "\x00" as its prefix (both in the
// panel's /sessions output and in command dispatch) instead of the default ".".
//
// Cause: dcPopulateRAM stores the "\x00" sentinel in the RAM cache for a GET
// whose result was JSON null. GetPrefix returned that raw cache entry without
// applying the default, so the sentinel leaked out as the prefix and the
// prefix match in handler.go compared messages against a NUL byte.
func TestGetPrefixNeverReturnsMissingSentinel(t *testing.T) {
	const def = "."

	cases := []struct {
		name  string
		cache string
		want  string
	}{
		{"missing sentinel falls back to default", "\x00", def},
		{"literal null falls back to default", "null", def},
		{"explicit prefix is preserved", "#", "#"},
		{"default prefix is preserved", ".", "."},
		// ".prefix null" sets an empty prefix deliberately (prefix-less mode):
		// it must NOT be treated as a cache miss.
		{"deliberate empty prefix is preserved", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := newPrefixTestUpstash()
			u.cacheSet("prefix:test@s.whatsapp.net", c.cache)

			got := u.GetPrefix("test@s.whatsapp.net", def)
			if got != c.want {
				t.Errorf("GetPrefix with cached %q = %q, want %q", c.cache, got, c.want)
			}
			if got == "\x00" {
				t.Fatal("missing sentinel leaked out as the prefix")
			}
		})
	}
}

// TestGetPrefixCachesDefault ensures the resolved default is what gets stored,
// so later reads never see the sentinel again.
func TestGetPrefixCachesDefault(t *testing.T) {
	u := newPrefixTestUpstash()
	u.cacheSet("prefix:new@s.whatsapp.net", "\x00")

	if got := u.GetPrefix("new@s.whatsapp.net", "."); got != "." {
		t.Fatalf("got %q want %q", got, ".")
	}
	if v, ok := u.cacheGet("prefix:new@s.whatsapp.net"); !ok || v == "\x00" {
		t.Errorf("cache still holds sentinel: %q ok=%v", v, ok)
	}
}

// TestGetPrefixRoundTripsCustom is the happy path: a prefix set via .prefix must
// come back unchanged.
func TestGetPrefixRoundTripsCustom(t *testing.T) {
	u := newPrefixTestUpstash()
	u.cacheSet("prefix:rt@s.whatsapp.net", "!")

	if got := u.GetPrefix("rt@s.whatsapp.net", "."); got != "!" {
		t.Fatalf("got %q want %q", got, "!")
	}
}

// TestDefaultPrefixIsDot asserts the configured default is a real character
// rather than an encoded NUL.
func TestDefaultPrefixIsDot(t *testing.T) {
	t.Setenv("GOLDMD_DEFAULT_PREFIX", "")
	if got := envOr("GOLDMD_DEFAULT_PREFIX", "."); got != "." {
		t.Fatalf("default prefix = %q, want %q", got, ".")
	}
	if got := envOr("GOLDMD_DEFAULT_PREFIX", "."); got == "\x00" {
		t.Fatal("default prefix must not be a NUL byte")
	}
}
