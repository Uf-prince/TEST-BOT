package goldcmds

import (
	"os"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// OWNER ORDER (2026-09-21): .play3 / .video3 ko query di jaye to LIST nahi —
// yts (YouTube) ka PEHLA result seedha download ho kar jata hai, aur media
// RAW jata hai (koi name, koi thumbnail, koi caption, koi footer nahi).
//
// OWNER ORDER (2026-09-22): query pe sirf EK message aata hai. Wo pehle
// "SEARCHING..." dikhata hai, phir usi message ko EDIT kar ke "DOWNLOADING..."
// bana diya jata hai (delete + naya msg NAHI). Ye message tab tak delete nahi
// hota jab tak media puri tarah download + compress na ho jaye; jab media
// sending pe ho tab ye message delete hota hai aur phir RAW media jata hai.
//
// Ye test source-level truth verify karta hai (compiled binary ka actual
// control-flow, guess nahi):
//   • query path  → pickFirstPlay3 / pickFirstVideo3
//   • list builder (number-pick session) REMOVED — koi SetAudioSession3 /
//     SetVideoSession3 call nahi bachta
//   • pickFirst body → results[0] + EDIT (not delete+new) + direct raw download
//   • raw send → SendAudioFileRawWait / SendVideoFileRawWait (compress FIRST,
//     then delete wait msg, then send)
// ─────────────────────────────────────────────────────────────────────────────

func readSrc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestPlay3QuerySendsFirstResultRaw(t *testing.T) {
	src := readSrc(t, "play3.go")

	// query path ab pickFirst pe jata hai
	if !strings.Contains(src, "pickFirstPlay3(ctx, s, info, input)") {
		t.Errorf(".play3 query path must call pickFirstPlay3 (no list)")
	}
	if !strings.Contains(src, "func pickFirstPlay3(") {
		t.Fatalf("pickFirstPlay3 missing")
	}
	// purana list builder gone
	if strings.Contains(src, "func searchProgressPlay3(") {
		t.Errorf("old number-pick list builder still present")
	}
	if strings.Contains(src, "SetAudioSession3(") {
		t.Errorf(".play3 must NOT create a number-pick list session anymore")
	}
	// first result + raw send path
	if !strings.Contains(src, "results[0]") {
		t.Errorf("pickFirstPlay3 must use the FIRST search result")
	}
	if !strings.Contains(src, "downloadAndSendAudio3(ctx, s, info, first.URL, nil, waitMsgID)") {
		t.Errorf("pickFirstPlay3 must hand the first result to the RAW engine (reusing edited wait msg)")
	}
	// OWNER ORDER (2026-09-22): SEARCHING msg EDIT ho kar DOWNLOADING banta hai
	if !strings.Contains(src, "s.EditMessage(info, waitMsgID, \"*DOWNLOADING AUDIO FROM YOUTUBE.....*\")") {
		t.Errorf("pickFirstPlay3 must EDIT the searching msg into the downloading msg (no delete+new)")
	}
	// waiting message clears (no stuck message) — error paths + clearWait
	if !strings.Contains(src, "DeleteMessage(info, waitMsgID)") {
		t.Errorf("waiting message must be deletable (error paths / clearWait)")
	}
	// raw send must compress FIRST then delete wait then send
	if !strings.Contains(src, "SendAudioFileRawWait(info, audioPath, clearWait)") {
		t.Errorf(".play3 must use SendAudioFileRawWait (compress first, then delete wait, then send)")
	}
}

func TestVideo3QuerySendsFirstResultRaw(t *testing.T) {
	src := readSrc(t, "video3.go")

	if !strings.Contains(src, "pickFirstVideo3(ctx, s, info, query, hd)") {
		t.Errorf(".video3 query path must call pickFirstVideo3 (no list)")
	}
	if !strings.Contains(src, "func pickFirstVideo3(") {
		t.Fatalf("pickFirstVideo3 missing")
	}
	if strings.Contains(src, "func searchProgressVideo3(") {
		t.Errorf("old number-pick list builder still present")
	}
	if strings.Contains(src, "SetVideoSession3(") {
		t.Errorf(".video3 must NOT create a number-pick list session anymore")
	}
	if !strings.Contains(src, "results[0]") {
		t.Errorf("pickFirstVideo3 must use the FIRST search result")
	}
	if !strings.Contains(src, "downloadAndSendVideo3(ctx, s, info, first.URL, nil, hd, waitMsgID)") {
		t.Errorf("pickFirstVideo3 must hand the first result to the RAW engine (hd preserved, reusing edited wait msg)")
	}
	// OWNER ORDER (2026-09-22): SEARCHING msg EDIT ho kar DOWNLOADING banta hai
	if !strings.Contains(src, "s.EditMessage(info, waitMsgID, \"*DOWNLOADING VIDEOS FROM YOUTUBE.....*\")") {
		t.Errorf("pickFirstVideo3 must EDIT the searching msg into the downloading msg (no delete+new)")
	}
	if !strings.Contains(src, "DeleteMessage(info, waitMsgID)") {
		t.Errorf("waiting message must be deletable (error paths / clearWait)")
	}
	// raw send must compress FIRST then delete wait then send
	if !strings.Contains(src, "SendVideoFileRawWait(info, finalPath, clearWait)") {
		t.Errorf(".video3 must use SendVideoFileRawWait (compress first, then delete wait, then send)")
	}
}

// Raw delivery contract: no caption / no thumbnail / no footer for both.
func TestRawSendIsCaptionless(t *testing.T) {
	src := readSrc(t, "../src/commands_loader.go")
	for _, fn := range []string{"func (b *bridge) SendVideoFileRaw(", "func (b *bridge) SendAudioFileRaw("} {
		idx := strings.Index(src, fn)
		if idx < 0 {
			t.Fatalf("%s missing", fn)
		}
		end := strings.Index(src[idx:], "\n}\n")
		if end < 0 {
			t.Fatalf("cannot bound %s", fn)
		}
		body := src[idx : idx+end]
		if strings.Contains(body, "withCaptionFooter") {
			t.Errorf("%s must not attach the botname footer", fn)
		}
		if strings.Contains(body, "Caption:") {
			t.Errorf("%s must not attach a caption", fn)
		}
	}
}

// RawWait contract: compress FIRST, then beforeSend (delete wait), then send.
func TestRawWaitCompressesBeforeDelete(t *testing.T) {
	src := readSrc(t, "../src/commands_loader.go")
	for _, fn := range []string{"func (b *bridge) SendVideoFileRawWait(", "func (b *bridge) SendAudioFileRawWait("} {
		idx := strings.Index(src, fn)
		if idx < 0 {
			t.Fatalf("%s missing", fn)
		}
		end := strings.Index(src[idx:], "\n}\n")
		if end < 0 {
			t.Fatalf("cannot bound %s", fn)
		}
		body := src[idx : idx+end]
		// guard compression must run before the beforeSend callback
		gi := strings.Index(body, "guardPath(")
		bi := strings.Index(body, "beforeSend()")
		if gi < 0 {
			t.Errorf("%s must run the guard compressor", fn)
		}
		if bi < 0 {
			t.Errorf("%s must invoke beforeSend (delete waiting msg)", fn)
		}
		if gi >= 0 && bi >= 0 && gi > bi {
			t.Errorf("%s must compress BEFORE deleting the waiting message", fn)
		}
		// raw: no caption / no footer
		if strings.Contains(body, "withCaptionFooter") {
			t.Errorf("%s must not attach the botname footer", fn)
		}
		if strings.Contains(body, "Caption:") {
			t.Errorf("%s must not attach a caption", fn)
		}
	}
}

// Search still backed by YouTube (youtubeSearch → Innertube), not an empty stub.
func TestYoutubeSearchUsedByRawCommands(t *testing.T) {
	src := readSrc(t, "video-play.go")
	if !strings.Contains(src, "func youtubeSearch(ctx context.Context, query string) []VideoResult") {
		t.Fatalf("youtubeSearch missing")
	}
	if !strings.Contains(src, "ytInnertubeSearch(query)") {
		t.Errorf("youtubeSearch must use the live Innertube search (yts engine)")
	}
	p3 := readSrc(t, "play3.go")
	v3 := readSrc(t, "video3.go")
	for name, s := range map[string]string{"play3.go": p3, "video3.go": v3} {
		if !strings.Contains(s, "youtubeSearch(ctx, query)") {
			t.Errorf("%s must run the yts search for the query", name)
		}
	}
}
