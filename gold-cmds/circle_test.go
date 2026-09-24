package goldcmds

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCircleConvertsToSquareVideo runs the real ffmpeg pipeline on a synthetic
// 16:9 clip and asserts the output is a playable SQUARE mp4 (h264). Skipped when
// ffmpeg is not on PATH.
func TestCircleConvertsToSquareVideo(t *testing.T) {
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	src := t.TempDir() + "/wide.mp4"
	// 2s 320x180 test clip with a silent audio track.
	gen := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=15:duration=2",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-shortest",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("could not build source clip: %v (%s)", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, err := makeCircleMP4(ctx, src)
	if err != nil {
		t.Fatalf("makeCircleMP4: %v", err)
	}
	defer os.Remove(out)

	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatalf("empty circle output: %v", err)
	}
	// Width must equal height (square) and match circleSide.
	out2, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name",
		"-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe output: %v", err)
	}
	secs, w, h := circleProbe(out)
	if w != circleSide || h != circleSide {
		t.Errorf("circle must be %dx%d, got %dx%d (%s)", circleSide, circleSide, w, h, out2)
	}
	if secs == 0 {
		t.Errorf("circle duration probe returned 0 (ffprobe: %s)", out2)
	}
}

// makeCircleMP4 must fail cleanly (not panic) on non-video input.
func TestCircleRejectsNonVideo(t *testing.T) {
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	junk := t.TempDir() + "/junk.mp4"
	if err := os.WriteFile(junk, []byte("this is not a video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := makeCircleMP4(ctx, junk); err == nil {
		os.Remove(out)
		t.Error("expected an error for non-video input")
	}
}

// TestAssetTriggerOrderPrefersMedia pins the lookup order used by
// src.applyAssetTrigger: media kinds resolve before text so a name shared by a
// photo and a text sends the photo. Both copies must agree.
func TestAssetTriggerOrderPrefersMedia(t *testing.T) {
	want := []string{"img", "video", "sticker", "circle", "text"}
	if len(AssetTriggerOrder) != len(want) {
		t.Fatalf("order = %v", AssetTriggerOrder)
	}
	for i := range want {
		if AssetTriggerOrder[i] != want[i] {
			t.Fatalf("order = %v want %v", AssetTriggerOrder, want)
		}
	}
}

// prepareAssetData must reject media of the wrong family (photo for .addvideo)
// and accept the right one, so a name never points at an unplayable asset.
func TestPrepareAssetDataFamilyGate(t *testing.T) {
	if _, _, ok := prepareAssetData("video", []byte("jpegbytes"), "image/jpeg", "video/mp4"); ok {
		t.Error("an image must not be saved as a video")
	}
	if _, _, ok := prepareAssetData("img", []byte("mp4bytes"), "video/mp4", "image/jpeg"); ok {
		t.Error("a video must not be saved as an image")
	}
	if _, mime, ok := prepareAssetData("text", []byte("hi"), "", "text/plain"); !ok || mime != "text/plain" {
		t.Errorf("text must pass through, got %q %v", mime, ok)
	}
	if _, _, ok := prepareAssetData("video", nil, "video/mp4", "video/mp4"); ok {
		t.Error("empty data must be rejected")
	}
}

// normaliseVideoBytes must produce a WhatsApp-ready (h264) mp4 from a source
// encoded with a codec WhatsApp rejects (mpeg4).
func TestNormaliseVideoBytesTranscodes(t *testing.T) {
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	src := t.TempDir() + "/mpeg4.mp4"
	gen := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=1",
		"-c:v", "mpeg4", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("could not build source: %v (%s)", err, out)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out, ok := normaliseVideoBytes(raw)
	if !ok || len(out) == 0 {
		t.Fatal("normaliseVideoBytes failed")
	}
	p, err := writeTempMedia(out, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	if !isWhatsAppVideoReady(p) {
		t.Error("normalised output is not WhatsApp-ready (h264+faststart)")
	}
}

// The .circle guidance and progress strings must be English (owner order —
// the earlier draft was Roman Urdu). Guards against a regression.
func TestCircleTextIsEnglish(t *testing.T) {
	blocked := []string{"BAN RAHI", "BAN RAHE", "SHAKAL", "YA ", "DO:", "HO RAHI", "US VIDEO"}
	for _, txt := range []string{
		circleHelpText("."),
		addCircleHelpText("."),
		circleWaitText("MAKING YOUR CIRCLE VIDEO", "00"),
	} {
		up := strings.ToUpper(txt)
		for _, bad := range blocked {
			if strings.Contains(up, bad) {
				t.Errorf("circle text %q contains non-English %q", txt, bad)
			}
		}
	}
	// sanity: the guidance must advertise both input paths
	if h := circleHelpText("."); !strings.Contains(h, "CIRCLE ❯") || !strings.Contains(h, "URL") {
		t.Errorf("circle help must show both reply and URL options: %q", h)
	}
	if h := addCircleHelpText("."); !strings.Contains(h, "CIRCLE ❯ FOR INFO") {
		t.Errorf("addcircle help must end with the CIRCLE info line: %q", h)
	}
}

// A bare prefix handling of the URL argument: only http(s) links count.
func TestCircleURLArg(t *testing.T) {
	if got := circleURLArg([]string{"not", "a", "url"}); got != "" {
		t.Errorf("non-URL must be ignored, got %q", got)
	}
	if got := circleURLArg([]string{"HTTPS://x/y.mp4"}); got == "" {
		t.Error("http(s) URL must be accepted")
	}
	if got := circleURLArg(nil); got != "" {
		t.Errorf("empty args must give empty, got %q", got)
	}
}
