package main

import "testing"

// TestPrefixCmdMatch verifies the UNIVERSAL .prefix command matcher: it must
// ALWAYS respond to the CURRENT prefix (whatever the owner set) AND to a
// leading '.' (dot), so the owner can always change the prefix.
func TestPrefixCmdMatch(t *testing.T) {
	cases := []struct {
		body   string
		prefix string
		want   string
		ok     bool
	}{
		// default prefix "."
		{".prefix", ".", "", true},
		{".prefix !", ".", "!", true},
		{".prefix null", ".", "null", true},
		{".prefix ;", ".", ";", true},
		// custom prefix ";" — works with ";" AND with "."
		{";prefix", ";", "", true},
		{";prefix +", ";", "+", true},
		{".prefix", ";", "", true},
		{".prefix $", ";", "$", true},
		// custom prefix "+"
		{"+prefix", "+", "", true},
		{"+prefix !", "+", "!", true},
		{".prefix", "+", "", true},
		// custom prefix "$"
		{"$prefix", "$", "", true},
		{".prefix", "$", "", true},
		// emoji prefix
		{"\U0001F530prefix", "\U0001F530", "", true},
		{".prefix", "\U0001F530", "", true},
		// prefix-less mode (empty prefix) — bare "prefix" via normal flow,
		// but the matcher itself only fires on "." here.
		{".prefix", "", "", true},
		// negatives
		{".prefixx", ".", "", false},
		{".pre", ".", "", false},
		{"hello", ".", "", false},
		{".ping", ".", "", false},
		{"prefix", ".", "", false},  // no prefix at all → not matched here
		{";prefix", ".", "", false}, // wrong prefix for this bot
	}
	for _, c := range cases {
		got, ok := prefixCmdMatch(c.body, c.prefix)
		if ok != c.ok || got != c.want {
			t.Errorf("prefixCmdMatch(%q, %q) = (%q, %v); want (%q, %v)",
				c.body, c.prefix, got, ok, c.want, c.ok)
		}
	}
}

// TestPrefixCommandRegistered ensures the .prefix command is present in the
// dispatch map so the universal bypass can actually call it.
func TestPrefixCommandRegistered(t *testing.T) {
	if _, ok := Commands["prefix"]; !ok {
		t.Fatal(`Commands["prefix"] not registered — universal .prefix bypass would be a no-op`)
	}
}
