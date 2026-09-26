package goldcmds

import "testing"

// TestSoftCapsTitleCasesLabels: house-style caps runs Title case ho jayen taake
// translator unhe proper noun na samjhe (yahi bug tha: "UPTIME" echo ho jata tha).
func TestSoftCapsTitleCasesLabels(t *testing.T) {
	cases := []struct{ in, want string }{
		{"UPTIME", "Uptime"},
		{"*│🔰 UPTIME :❯ 00H 00M*", "*│🔰 Uptime :❯ 00H 00M*"},
		{"COMMANDS", "Commands"},
		{"PREFIX", "Prefix"},
		{"USER", "User"},
		{"OWNER", "Owner"},
		{"MENUS", "Menus"},
		// A lone capital reads as prose already; mixed case is untouched, but a
		// caps RUN inside mixed text is still softened.
		{"I AM ONLINE", "I Am Online"},
		{"Gold-MD", "Gold-Md"},
		{"1250", "1250"},
		{"BOTLANGUAGE", "Botlanguage"},
	}
	for _, c := range cases {
		if got := trtSoftCaps(c.in); got != c.want {
			t.Errorf("trtSoftCaps(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAllCapsDetectsHouseStyle: sirf caps labels pe hi uppercase wapas lagana hai.
func TestAllCapsDetectsHouseStyle(t *testing.T) {
	caps := []string{"UPTIME", "*│🔰 UPTIME :❯ 00H 00M*", "COMMANDS :❯ ❮ 1250 ❯", "🔰 IMPORTANT CMNDS 🔰"}
	for _, s := range caps {
		if !trtAllCaps(s) {
			t.Errorf("trtAllCaps(%q) true hona chahiye", s)
		}
	}
	notCaps := []string{"Uptime", "bot is online", "Gold-MD", "🔰", "1250", ""}
	for _, s := range notCaps {
		if trtAllCaps(s) {
			t.Errorf("trtAllCaps(%q) false hona chahiye", s)
		}
	}
}

// TestProtectAndRestorePrefixes: bare prefix sentinel me jaye aur wapas as-is
// aaye, magar "photo.jpg" / "1.5" jaisa dot chhua na jaye.
func TestProtectAndRestorePrefixes(t *testing.T) {
	roundTrip := []string{
		"*│🔰 PREFIX :❯ ❮ . ❯*",
		"PREFIX :❯ .",
		".MENU",
		"❮ / ❯",
		"❮ ! ❯",
		"❮ # ❯",
	}
	for _, s := range roundTrip {
		if got := trtRestorePrefixes(trtProtectPrefixes(s)); got != s {
			t.Errorf("round trip badla: %q -> %q", s, got)
		}
	}
	untouched := []string{"photo.jpg", "1.5", "e.g. this", "https://x/y.jpg", "a.b.c"}
	for _, s := range untouched {
		if got := trtProtectPrefixes(s); got != s {
			t.Errorf("prose ka dot chhua gaya: %q -> %q", s, got)
		}
	}
}
