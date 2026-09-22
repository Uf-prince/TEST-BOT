package goldcmds

import (
	"context"
	"testing"
)

// TestNewsFreshness verifies that repeated calls to newsFetch return a large
// pool and that two consecutive calls do not return the identical slice
// (i.e. the news is "fresh" each time).
func TestNewsFreshness(t *testing.T) {
	ctx := context.Background()

	a, ok := newsFetch(ctx, "Pakistan Islamabad", 8)
	if !ok || len(a) == 0 {
		t.Fatalf("first newsFetch failed: ok=%v len=%d", ok, len(a))
	}
	t.Logf("call 1 returned %d items", len(a))
	for i, it := range a {
		t.Logf("  %d. %s", i+1, it[0])
	}

	// Try a few more calls; at least one should differ from the first.
	differed := false
	for attempt := 0; attempt < 5; attempt++ {
		b, ok := newsFetch(ctx, "Pakistan Islamabad", 8)
		if !ok || len(b) == 0 {
			continue
		}
		if b[0][0] != a[0][0] {
			differed = true
			t.Logf("call %d top headline differs: %q", attempt+2, b[0][0])
			break
		}
	}
	if !differed {
		t.Errorf("newsFetch returned the same top headline on every call (not fresh)")
	}
}
