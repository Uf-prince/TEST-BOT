package main

import "testing"

// TestDedupeServerEntries — duplicate URLs collapse to the FIRST occurrence,
// order preserved, unique entries untouched.
func TestDedupeServerEntries(t *testing.T) {
	in := []serverEntry{
		{Name: "SERVER 1", URL: "https://a.onrender.com"},
		{Name: "SERVER 2", URL: "https://b.onrender.com"},
		{Name: "SERVER 3", URL: "https://a.onrender.com"}, // dup of 1
		{Name: "SERVER 4", URL: "https://A.onrender.com/"}, // dup (case + slash)
		{Name: "SERVER 5", URL: "https://c.onrender.com"},
	}
	out := dedupeServerEntries(in)
	if len(out) != 3 {
		t.Fatalf("want 3 unique, got %d: %+v", len(out), out)
	}
	if out[0].Name != "SERVER 1" || out[1].Name != "SERVER 2" || out[2].Name != "SERVER 5" {
		t.Fatalf("first-occurrence/order wrong: %+v", out)
	}
}

// TestServersJSONNoDuplicateURLs — the shipped servers.json must have all
// unique URLs (the .svr wrong-info root cause).
func TestServersJSONNoDuplicateURLs(t *testing.T) {
	loadServersConfig()
	seen := map[string]string{}
	for _, e := range serversCfg.Servers {
		if prev, ok := seen[e.URL]; ok {
			t.Fatalf("duplicate URL %s in %s and %s", e.URL, prev, e.Name)
		}
		seen[e.URL] = e.Name
	}
}
