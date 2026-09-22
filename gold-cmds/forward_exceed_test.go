package goldcmds

import (
	"fmt"
	"strings"
	"testing"
)

// TestForwardExceedExampleNumbers verifies the exceed-error example line
// uses numbers clamped to the user's REAL totals:
//   - 3 groups / 176 chats  → *.FORWARD 6,3*  (6 chats OK, 8→3 groups)
//   - 10 groups / 20 chats  → *.FORWARD 6,8*  (both fit, stays default)
//   - 1 group / 1 chat      → *.FORWARD 1,1*  (clamped to minimum)
func TestForwardExceedExampleNumbers(t *testing.T) {
	cases := []struct {
		groups, chats int
		wantExample   string
	}{
		{groups: 3, chats: 176, wantExample: "*.FORWARD 6,3*"},  // owner's real totals
		{groups: 10, chats: 20, wantExample: "*.FORWARD 6,8*"},  // both fit
		{groups: 1, chats: 1, wantExample: "*.FORWARD 1,1*"},    // minimum clamp
		{groups: 5, chats: 4, wantExample: "*.FORWARD 4,5*"},    // both clamped
	}

	for _, c := range cases {
		got := fwdExceedText(".", c.groups, c.chats)
		lines := strings.Split(got, "\n")
		if len(lines) < 6 {
			t.Fatalf("lines=%d, want >= 6", len(lines))
		}
		lastLine := lines[len(lines)-1]
		if lastLine != c.wantExample {
			t.Errorf("groups=%d chats=%d → example line %q, want %q", c.groups, c.chats, lastLine, c.wantExample)
		}
		// totals lines must still show the REAL numbers
		if !strings.Contains(got, "*YOUR TOTAL GROUPS \u276e "+fmt.Sprint(c.groups)+" \u276f*") {
			t.Errorf("groups=%d: totals line missing real group count", c.groups)
		}
		if !strings.Contains(got, "*YOUR TOTAL CHATS \u276e "+fmt.Sprint(c.chats)+" \u276f*") {
			t.Errorf("chats=%d: totals line missing real chat count", c.chats)
		}
		fmt.Printf("groups=%-3d chats=%-4d → example: %s  ✓\n", c.groups, c.chats, lastLine)
	}
	fmt.Println("✔ EXCEED EXAMPLE NUMBERS NOW MATCH THE USER'S REAL TOTALS")
}
