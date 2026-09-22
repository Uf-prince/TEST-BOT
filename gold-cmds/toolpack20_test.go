package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack20GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"dateadd":     dateaddGuide("."),
		"datesub":     datesubGuide("."),
		"dateformat":  dateformatGuide("."),
		"weeknumber":  weeknumberGuide("."),
		"dayofweek":   dayofweekGuide("."),
		"countdownto": countdowntoGuide("."),
		"timer":       timerGuide("."),
		"stopwatch":   stopwatchGuide("."),
		"pomodoro":    pomodoroGuide("."),
		"ageindays":   ageindaysGuide("."),
	}
	for name, g := range guides {
		if strings.Contains(g, `\n`) {
			t.Fatalf("%s guide contains literal backslash-n", name)
		}
		if !strings.Contains(g, "\n") {
			t.Fatalf("%s guide has no real newline", name)
		}
		if !strings.Contains(g, "🔰") {
			t.Fatalf("%s guide missing 🔰", name)
		}
	}
}

func TestToolpack20Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"dateadd", handleDateadd, []string{"2024-01-01", "30"}},
		{"datesub", handleDatesub, []string{"2024-01-31", "15"}},
		{"dateformat", handleDateformat, []string{"2024-06-15"}},
		{"weeknumber", handleWeeknumber, []string{"2024-06-15"}},
		{"dayofweek", handleDayofweek, []string{"2000-01-01"}},
		{"countdownto", handleCountdownto, []string{"2030-12-31"}},
		{"timer", handleTimer, []string{"300"}},
		{"stopwatch", handleStopwatch, []string{}},
		{"pomodoro", handlePomodoro, []string{"25", "5"}},
		{"ageindays", handleAgeindays, []string{"2000-01-15"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
