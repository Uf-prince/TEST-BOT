package goldcmds

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBotVideoNormalizeMakesWhatsAppSafeMp4 is the "can't play this video"
// regression test: a VP9/odd-dimension clip must come back as H.264, even
// dimensions and faststart so WhatsApp can actually play it.
func TestBotVideoNormalizeMakesWhatsAppSafeMp4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	src := t.TempDir() + "/in.webm"
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=641x481:rate=15", "-t", "2",
		"-c:v", "libvpx-vp9", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate vp9 sample: %v %s", err, out)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	norm, err := BotVideoNormalizeBytes(raw)
	if err != nil {
		t.Fatalf("normalize error: %v", err)
	}
	if len(norm) == 0 {
		t.Fatal("normalize returned empty bytes")
	}
	out := t.TempDir() + "/out.mp4"
	if err := os.WriteFile(out, norm, 0o600); err != nil {
		t.Fatal(err)
	}

	codec := strings.TrimSpace(ffprobeField(t, out, "stream=codec_name"))
	if codec != "h264" {
		t.Fatalf("normalized codec = %q, want h264", codec)
	}
	dims := strings.TrimSpace(ffprobeField(t, out, "stream=width,height"))
	if dims != "640,480" {
		t.Fatalf("normalized dims = %q, want even 640,480", dims)
	}
	// faststart: the moov atom must precede the media data.
	moov := bytes.Index(norm, []byte("moov"))
	mdat := bytes.Index(norm, []byte("mdat"))
	if moov < 0 || mdat < 0 || moov > mdat {
		t.Fatalf("moov(%d) must precede mdat(%d) for faststart", moov, mdat)
	}
}

func ffprobeField(t *testing.T, path, entries string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", entries, "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", entries, err)
	}
	return string(out)
}
