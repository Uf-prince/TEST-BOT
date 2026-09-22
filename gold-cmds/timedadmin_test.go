package goldcmds

import (
	"testing"
	"time"
)

func TestTimedAdminFlexibleDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"00h05m00s", 5 * time.Minute, true},
		{"01h00m00s", time.Hour, true},
		{"5m", 5 * time.Minute, true},
		{"1h30m", 90 * time.Minute, true},
		{"45s", 45 * time.Second, true},
		{"1h", time.Hour, true},
		{"00:05:00", 5 * time.Minute, true},
		{"05:00", 5 * time.Minute, true},
		{"5", 5 * time.Minute, true},
		{"2h", 2 * time.Hour, true},
		{"1d", 24 * time.Hour, true},
		{"2days", 48 * time.Hour, true},
		{"10min", 10 * time.Minute, true},
		{"30sec", 30 * time.Second, true},
		{"`5m`", 5 * time.Minute, true},
		{"5m\u200b", 5 * time.Minute, true},
		{"abc", 0, false},
		{"0h0m0s", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseTimedAdminDuration(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseTimedAdminDuration(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTimedAdminFindDuration(t *testing.T) {
	// time before mention
	if d, _, ok := timedAdminFindDuration([]string{"5m", "@user"}); !ok || d != 5*time.Minute {
		t.Errorf("before-mention failed: %v %v", d, ok)
	}
	// time after mention
	if d, _, ok := timedAdminFindDuration([]string{"@user", "00h05m00s"}); !ok || d != 5*time.Minute {
		t.Errorf("after-mention failed: %v %v", d, ok)
	}
	// space-separated parts
	if d, _, ok := timedAdminFindDuration([]string{"00h", "05m", "00s", "@user"}); !ok || d != 5*time.Minute {
		t.Errorf("space-separated failed: %v %v", d, ok)
	}
	// no time
	if _, _, ok := timedAdminFindDuration([]string{"@user"}); ok {
		t.Errorf("expected no duration")
	}
}
