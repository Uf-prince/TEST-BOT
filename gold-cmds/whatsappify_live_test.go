package goldcmds

// Live test for whatsappifyVideo (run manually):
//   WA_LIVE=1 go test -mod=vendor -vet=off -v -count=1 -run 'TestWhatsAppifyLive' ./gold-cmds/ -timeout 120s
// Verifies: mjpeg/HEVC file → h264+faststart transcode works.

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWhatsAppifyLive(t *testing.T) {
	if os.Getenv("WA_LIVE") == "" {
		t.Skip("set WA_LIVE=1 to run the live whatsappify test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// mjpeg problem file (cobalt ne ye thumbnail-as-video diya tha)
	src := "/tmp/v_DZ44VNasyWq.mp4"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("test file %s not present: %v", src, err)
	}
	t.Logf("input ready=%v", isWhatsAppVideoReady(src))
	out, err := whatsappifyVideo(ctx, src)
	if err != nil {
		t.Fatalf("whatsappifyVideo: %v", err)
	}
	t.Logf("output=%s ready=%v", out, isWhatsAppVideoReady(out))
	if !isWhatsAppVideoReady(out) {
		t.Fatal("output still not WhatsApp-ready")
	}
	if out != src {
		defer os.Remove(out)
	}

	// clean h264 file → no-op (same path wapas)
	src2 := "/tmp/igtest.mp4"
	if _, err := os.Stat(src2); err == nil {
		t.Logf("clean input ready=%v", isWhatsAppVideoReady(src2))
		out2, err2 := whatsappifyVideo(ctx, src2)
		if err2 != nil {
			t.Fatalf("whatsappifyVideo clean: %v", err2)
		}
		if out2 != src2 {
			defer os.Remove(out2)
			t.Errorf("clean file should be a no-op, got new path %s", out2)
		} else {
			t.Logf("clean file no-op OK (same path)")
		}
	}
}
