package goldcmds

import (
	"strings"
	"testing"
)

// TestTTFmtDuration verifies the digital-clock DURATION format
// (owner round 3: "00M : 00S" style, not "5M06S" glued together).
func TestTTFmtDuration(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{0, ""},
		{-5, ""},
		{45, "00M : 45S"},
		{59, "00M : 59S"},
		{60, "01M : 00S"},
		{63, "01M : 03S"},
		{225, "03M : 45S"},
		{306, "05M : 06S"},
		{600, "10M : 00S"},
	}
	for _, c := range cases {
		got := ttFmtDuration(c.sec)
		if got != c.want {
			t.Errorf("ttFmtDuration(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}

// TestTTDurationLine verifies searchCardEntry renders the DURATION line
// in clock style between STATS and LINK.
func TestTTDurationLine(t *testing.T) {
	r := searchResult{
		Title:       "Aaja We Mahiya Full Song",
		Handle:      "@vibe_music_store",
		Stats:       "994.9K PLAYS \xe2\x9d\xb0 30.3K LIKES \xe2\x9d\xb1",
		Link:        "https://www.tiktok.com/@vibe_music_store/video/7631236548943482120",
		DurationSec: 306,
	}
	card := searchCardEntry(16, r, "TIKTOK SEARCH", "USER", "STATS")
	if !strings.Contains(card, "DURATION") {
		t.Fatal("DURATION line missing from card entry")
	}
	i := strings.Index(card, "DURATION")
	seg := card[i : i+30]
	if !strings.Contains(seg, "05M : 06S") {
		t.Errorf("DURATION not clock style: %q", seg)
	}
	// ordering: STATS -> DURATION -> LINK
	si := strings.Index(card, "STATS")
	di := strings.Index(card, "DURATION")
	li := strings.Index(card, "LINK")
	if !(si < di && di < li) {
		t.Errorf("line order wrong: STATS=%d DURATION=%d LINK=%d", si, di, li)
	}
}
