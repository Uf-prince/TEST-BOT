package main

// Standalone logic test for .svrchange — real Go test (go test -run TestSvr).
// GitHub/GitLab push functions network hit karte hain, is liye sirf
// pure-logic functions test hote hain (apply-changes + parse helpers).

import (
	"strings"
	"testing"
)

// GitHub servers.json format (multi-line, 2-space indent).
const ghSample = `{
  "_comment": "GOLD-MD config. 200 servers, max 2 pairings.",
  "maxPerServer": 2,
  "servers": [
    {
      "name": "SERVER 1",
      "url": "https://gold-md-xsvr1.onrender.com"
    },
    {
      "name": "SERVER 2",
      "url": "https://gold-md-xsvr2.onrender.com"
    },
    {
      "name": "SERVER 14",
      "url": "https://gold-md-xsvr5.onrender.com"
    }
  ]
}`

// GitLab servers.json format (same indent).
const glSample = `{
  "_comment": "GOLD-MD panel config. 20 servers.",
  "maxPerServer": 3,
  "servers": [
    {
      "name": "SERVER 1",
      "url": "https://gold-md-svrr1.onrender.com"
    },
    {
      "name": "SERVER 2",
      "url": "https://gold-md-svrr2-frhr.onrender.com/"
    }
  ]
}`

func TestSvrServerNumber(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"SERVER 9", 9}, {"SERVER 200", 200}, {"SERVER 1", 1},
		{"SERVER 14", 14}, {"garbage", 0}, {"", 0}, {"SERVER", 0},
	}
	for _, c := range cases {
		if got := svrServerNumber(c.in); got != c.want {
			t.Errorf("svrServerNumber(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSvrApplyChangesMulti(t *testing.T) {
	// ghSample: SERVER 1, 2, 14 hain.  9 missing hai.
	changes := map[int]string{
		1:  "https://new-one.onrender.com",
		2:  "https://new-two.onrender.com",
		9:  "https://new-nine.onrender.com",
		14: "https://new-fourteen.onrender.com",
	}
	raw, updated, missing, err := svrApplyChanges(ghSample, changes)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(missing) != 1 || missing[0] != 9 {
		t.Fatalf("missing want [9], got %v", missing)
	}
	if len(updated) != 3 {
		t.Fatalf("updated want 3, got %d: %v", len(updated), updated)
	}
	// URLs replaced?
	if !strings.Contains(raw, `"url": "https://new-one.onrender.com"`) {
		t.Error("SERVER 1 URL not replaced")
	}
	if !strings.Contains(raw, `"url": "https://new-two.onrender.com"`) {
		t.Error("SERVER 2 URL not replaced")
	}
	if !strings.Contains(raw, `"url": "https://new-fourteen.onrender.com"`) {
		t.Error("SERVER 14 URL not replaced")
	}
	// formatting preserved? (multi-line entries intact)
	if !strings.Contains(raw, "    {\n      \"name\": \"SERVER 1\",\n      \"url\":") {
		t.Error("formatting broken — expected multi-line entry preserved")
	}
	// old URLs gone?
	if strings.Contains(raw, "xsvr1.onrender.com") || strings.Contains(raw, "xsvr2.onrender.com") || strings.Contains(raw, "xsvr5.onrender.com") {
		t.Error("old URL still present")
	}
	t.Logf("updated lines:\n%s", strings.Join(updated, "\n"))
}

func TestSvrApplyChangesSameMapReuse(t *testing.T) {
	// dono repos ke liye same map reuse — map mutate nahi hona chahiye.
	changes := map[int]string{1: "https://reused.onrender.com"}
	_, _, missing1, err := svrApplyChanges(ghSample, changes)
	if err != nil || len(missing1) != 0 {
		t.Fatalf("first apply: %v %v", err, missing1)
	}
	// changes map ab bhi {1: reused} hona chahiye
	if len(changes) != 1 || changes[1] != "https://reused.onrender.com" {
		t.Fatalf("changes map mutated: %v", changes)
	}
	_, _, missing2, err := svrApplyChanges(glSample, changes)
	if err != nil || len(missing2) != 0 {
		t.Fatalf("second apply: %v %v", err, missing2)
	}
}

func TestSvrApplyChangesGitLabTrailingSlash(t *testing.T) {
	// GitLab me SERVER 2 ka URL trailing slash ke saath hai — replacement
	// exact line-splice hona chahiye.
	changes := map[int]string{2: "https://gold-md-svrr2-new.onrender.com"}
	raw, updated, missing, err := svrApplyChanges(glSample, changes)
	if err != nil || len(missing) != 0 || len(updated) != 1 {
		t.Fatalf("apply: err=%v missing=%v updated=%v", err, missing, updated)
	}
	if !strings.Contains(raw, `"url": "https://gold-md-svrr2-new.onrender.com"`) {
		t.Error("GitLab SERVER 2 URL not replaced")
	}
	if strings.Contains(raw, "svrr2-frhr") {
		t.Error("old URL still present")
	}
}
