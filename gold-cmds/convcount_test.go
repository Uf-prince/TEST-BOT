package goldcmds

import "testing"

func TestConverterCategoryCount(t *testing.T) {
	visible, hidden := 0, 0
	for _, c := range Commands() {
		if c.Category == "CONVERTER" {
			if c.Hidden {
				hidden++
			} else {
				visible++
			}
		}
	}
	t.Logf("CONVERTER visible=%d hidden=%d total=%d", visible, hidden, visible+hidden)
	t.Logf("TOTAL visible commands (CommandsCount) = %d", CommandsCount())
	if visible != 100 {
		t.Fatalf("expected 100 visible CONVERTER commands, got %d", visible)
	}
}
