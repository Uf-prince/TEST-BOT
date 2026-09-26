package goldcmds

import "testing"

func TestTRTPlausible(t *testing.T) {
	cases := []struct {
		src, out string
		want     bool
	}{
		{"CHANGE BOT PIC (MENU + ALIVE)", "ت", false},  // throttled: phrase -> 1 rune
		{"CHECK SPEED", "ا", false},                    // throttled
		{"HELLO", "مرحبا", true},                       // single word is fine
		{"SHOW ALL MENUS", "عرض كل القوائم", true},      // real translation
		{"CHANGE BOT PIC", "", false},                  // empty is never usable
		{"*🔰 MENU 🔰*", "ق", false},                    // multi-token line -> 1 rune
		{"*🔰 MENU 🔰*", "*🔰 القائمة 🔰*", true},
	}
	for _, c := range cases {
		if got := trtPlausible(c.src, c.out); got != c.want {
			t.Errorf("trtPlausible(%q,%q) = %v want %v", c.src, c.out, got, c.want)
		}
	}
}
