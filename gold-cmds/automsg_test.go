package goldcmds

// AUTOMSG fix verification tests — run with:
//   go test -mod=vendor -vet=off -count=1 -run 'TestAutomsg' ./gold-cmds/
//
// Ye test file exactly us case ko cover karti hai jo owner ne report kiya
// tha: ".automsg 06h05m06s once wait" pe bot "Invalid time format" de raha
// tha — chahe format bilkul sahi tha (purana parser trailing "s" strip kar
// ke seconds ka unit kha jata tha).

import (
	"testing"
	"time"
)

func TestParseAutomsgDurationValid(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		// THE REPORTED BUG: "06h05m06s" (user's exact input) pehle reject hota tha
		{"06h05m06s", 6*time.Hour + 5*time.Minute + 6*time.Second},
		{"00h30m00s", 30 * time.Minute},                       // docs example 1
		{"02h30m00s", 2*time.Hour + 30*time.Minute},           // docs example 2
		{"00h00m45s", 45 * time.Second},                       // docs example 3 (= 45 seconds)
		{"1h2m3s", time.Hour + 2*time.Minute + 3*time.Second}, // unpadded
		{"02h00m00s", 2 * time.Hour},                          // once-mode example
		{"00h04m15s", 4*time.Minute + 15*time.Second},
		{"8784h00m00s", 8784 * time.Hour}, // upper bound (~1 year)
		// case-insensitive + padding spaces
		{" 06H05M06S ", 6*time.Hour + 5*time.Minute + 6*time.Second},
	}
	for _, c := range cases {
		got, ok := parseAutomsgDuration(c.in)
		if !ok {
			t.Errorf("parseAutomsgDuration(%q) = rejected, want accepted", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("parseAutomsgDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseAutomsgDurationInvalid(t *testing.T) {
	cases := []string{
		"",                // empty
		"06h05m06",        // missing trailing s  (purana parser isko garbar accept karta)
		"30m",             // missing h and s
		"2h30m",           // missing s
		"45s",             // missing h and m
		"06x05m06s",       // bad unit letter
		"06h-5m06s",       // negative digits not matched by regex
		"0h0m0s",          // total zero — must reject
		"00h60m00s",       // minutes out of range
		"00h00m60s",       // seconds out of range
		"99999h00m00s",    // hours over sane bound
		"06h05m06s extra", // trailing junk
		"abc",             // garbage
	}
	for _, in := range cases {
		if got, ok := parseAutomsgDuration(in); ok {
			t.Errorf("parseAutomsgDuration(%q) = %v accepted, want rejected", in, got)
		}
	}
}

func TestFormatHMS(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{6*time.Hour + 5*time.Minute + 6*time.Second, "06h05m06s"},
		{30 * time.Minute, "00h30m00s"},
		{45 * time.Second, "00h00m45s"},
		{0, "00h00m00s"},
	}
	for _, c := range cases {
		if got := formatHMS(c.in); got != c.want {
			t.Errorf("formatHMS(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsJustNumber(t *testing.T) {
	valid := []string{"1", "2", "03", " 12 "}
	for _, v := range valid {
		if !isJustNumber(v) {
			t.Errorf("isJustNumber(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "1a", "a1", "1.5", "-1", "one"}
	for _, v := range invalid {
		if isJustNumber(v) {
			t.Errorf("isJustNumber(%q) = true, want false", v)
		}
	}
}
