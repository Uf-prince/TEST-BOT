package goldcmds

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// eqTestFFmpeg skips the test when no ffmpeg is obtainable (offline CI without
// the static build). Otherwise every effect is run through the REAL production
// filter chain on real generated media, so a typo in an -af string fails here.
func eqTestFFmpeg(t *testing.T) {
	t.Helper()
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg unavailable")
	}
}

func eqTestMakeMedia(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	full := append([]string{"-y"}, args...)
	full = append(full, out)
	cmd := exec.Command("ffmpeg", full...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate %s: %v", name, err)
	}
	return out
}

// Every audio effect must produce a playable, non-empty MP3 through the real
// ffmpegApplyEffectFile path.
func TestEqualizerAudioEffectsProduceOutput(t *testing.T) {
	eqTestFFmpeg(t)
	dir := t.TempDir()
	src := eqTestMakeMedia(t, dir, "in.wav",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "pcm_s16le")

	for _, eff := range eqEffects {
		eff := eff
		t.Run(eff.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := ffmpegApplyEffectFile(ctx, src, "audio/wav", eff)
			if err != nil {
				t.Fatalf("%s: %v", eff.Name, err)
			}
			defer removeTempFile(out)
			st, err := os.Stat(out)
			if err != nil || st.Size() == 0 {
				t.Fatalf("%s: output missing/empty (%v)", eff.Name, err)
			}
			if filepath.Ext(out) != ".mp3" {
				t.Errorf("%s: audio output ext = %s, want .mp3", eff.Name, filepath.Ext(out))
			}
		})
	}
}

// Every video effect must keep the picture and rewrite the audio, returning a
// non-empty MP4.
func TestEqualizerVideoEffectsProduceOutput(t *testing.T) {
	eqTestFFmpeg(t)
	dir := t.TempDir()
	src := eqTestMakeMedia(t, dir, "in.mp4",
		"-f", "lavfi", "-i", "testsrc=size=128x72:rate=15:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=300:duration=2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest")

	for _, eff := range eqEffects {
		eff := eff
		t.Run(eff.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			out, err := ffmpegApplyEffectFile(ctx, src, "video/mp4", eff)
			if err != nil {
				t.Fatalf("%s: %v", eff.Name, err)
			}
			defer removeTempFile(out)
			st, err := os.Stat(out)
			if err != nil || st.Size() == 0 {
				t.Fatalf("%s: output missing/empty (%v)", eff.Name, err)
			}
			if filepath.Ext(out) != ".mp4" {
				t.Errorf("%s: video output ext = %s, want .mp4", eff.Name, filepath.Ext(out))
			}
			if !eqHasAudioStream(ctx, out) {
				t.Errorf("%s: output has no audio stream", eff.Name)
			}
		})
	}
}

// A video with no audio track must be detected before ffmpeg runs, so the
// handler can tell the user instead of failing.
func TestEqualizerDetectsMissingAudioTrack(t *testing.T) {
	eqTestFFmpeg(t)
	if !isFfprobeAvailable() {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	src := eqTestMakeMedia(t, dir, "silent.mp4",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if eqHasAudioStream(ctx, src) {
		t.Error("silent video reported as having audio")
	}
}
