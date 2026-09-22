package goldcmds

import "testing"

// TestVideo3Play3Registered — the new raw commands must be registered and
// visible (not hidden) so they appear in the menu + command count.
func TestVideo3Play3Registered(t *testing.T) {
	found := map[string]bool{}
	for _, c := range Commands() {
		if c.Name == "video3" || c.Name == "play3" {
			found[c.Name] = true
			if c.Hidden {
				t.Errorf("%s must be visible (not hidden)", c.Name)
			}
			if c.Run == nil {
				t.Errorf("%s has nil Run", c.Name)
			}
		}
	}
	if !found["video3"] {
		t.Error("video3 not registered")
	}
	if !found["play3"] {
		t.Error("play3 not registered")
	}
}
