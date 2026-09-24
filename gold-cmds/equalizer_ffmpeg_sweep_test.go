package goldcmds

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// TestEqualizerAllDesignsRunThroughFfmpeg is an opt-in end-to-end sweep: every
// one of the 1000 design chains is run through the bundled ffmpeg on a short
// generated tone. It is skipped unless GOLDMD_EQ_SWEEP=1 so the normal test
// suite stays fast.
func TestEqualizerAllDesignsRunThroughFfmpeg(t *testing.T) {
	if os.Getenv("GOLDMD_EQ_SWEEP") != "1" {
		t.Skip("set GOLDMD_EQ_SWEEP=1 to run the full 1000-design ffmpeg sweep")
	}
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	ctx := context.Background()
	bad := []int{}
	for n := 1; n <= EqCount; n++ {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=0.4",
			"-af", EqAudioFilter(n), "-f", "null", "-")
		if out, err := cmd.CombinedOutput(); err != nil {
			bad = append(bad, n)
			t.Logf("design %d (%s) FAILED: %v :: %s", n, EqDesignName(n), err, out)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d designs failed ffmpeg validation: %v", len(bad), bad)
	}
	t.Logf("all %d designs ran through ffmpeg", EqCount)
}
