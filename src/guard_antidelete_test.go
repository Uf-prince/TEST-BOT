package main

// ============================================================================
// ANTIDELETE MEDIA SHIELD TEST (owner: jani)
// ============================================================================
// Verifies guardAntideleteBytes() — the compressor that antidelete recovery
// uses before re-uploading recovered media to WhatsApp (Render outbound save).
//
//   video  : 1080p sample -> compressed (360p) -> size drop measured
//   audio  : mp3 sample    -> compressed (128k) -> size drop measured
//   image  : jpg sample    -> compressed (q80)  -> size drop measured
//   doc    : passthrough (never compressed)
//   small  : below floor   -> passthrough (unchanged)
//
// Run: go test -run TestGuardAntideleteShield -v ./src/
// ============================================================================

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardAntideleteShield(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := "/workspace/guardtest"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no sample dir")
	}

	// ---- VIDEO ----
	var vid string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".mp4") && !strings.Contains(e.Name(), "_pass") &&
			!strings.Contains(e.Name(), "orig_") {
			vid = filepath.Join(dir, e.Name())
			break
		}
	}
	if vid != "" {
		data, _ := os.ReadFile(vid)
		comp, _ := guardAntideleteBytes(guardVideo, data)
		t.Logf("VIDEO  : %s -> %s  (drop %.1f%%)  [%s]",
			guardFmtMB(int64(len(data))), guardFmtMB(int64(len(comp))),
			(1-float64(len(comp))/float64(len(data)))*100, filepath.Base(vid))
		if len(comp) >= len(data) {
			t.Errorf("video not compressed: %d -> %d", len(data), len(comp))
		}
	}

	// ---- AUDIO ----
	audioSrc := filepath.Join(dir, "sample_audio.mp3")
	if _, err := os.Stat(audioSrc); err != nil {
		exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i",
			"sine=frequency=440:duration=30", "-b:a", "320k", audioSrc).Run()
	}
	if adata, err := os.ReadFile(audioSrc); err == nil && len(adata) > 0 {
		comp, _ := guardAntideleteBytes(guardAudio, adata)
		t.Logf("AUDIO  : %s -> %s  (drop %.1f%%)",
			guardFmtMB(int64(len(adata))), guardFmtMB(int64(len(comp))),
			(1-float64(len(comp))/float64(len(adata)))*100)
	}

	// ---- IMAGE ----
	imgSrc := filepath.Join(dir, "sample_image.jpg")
	if _, err := os.Stat(imgSrc); err != nil {
		exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i",
			"testsrc2=size=4000x3000", "-frames:v", "1", "-q:v", "1", imgSrc).Run()
	}
	if idata, err := os.ReadFile(imgSrc); err == nil && len(idata) > 0 {
		comp, _ := guardAntideleteBytes(guardImage, idata)
		t.Logf("IMAGE  : %s -> %s  (drop %.1f%%)",
			guardFmtMB(int64(len(idata))), guardFmtMB(int64(len(comp))),
			(1-float64(len(comp))/float64(len(idata)))*100)
	}

	// ---- DOCUMENT (must passthrough) ----
	docData := make([]byte, 5*1024*1024)
	for i := range docData {
		docData[i] = byte(i % 251)
	}
	comp, _ := guardAntideleteBytes(guardDocument, docData)
	if len(comp) != len(docData) {
		t.Errorf("document must passthrough unchanged: %d -> %d", len(docData), len(comp))
	} else {
		t.Logf("DOC    : passthrough OK (%s unchanged)", guardFmtMB(int64(len(comp))))
	}

	// ---- SMALL (below floor, must passthrough) ----
	small := make([]byte, 100*1024)
	comp2, _ := guardAntideleteBytes(guardVideo, small)
	if len(comp2) != len(small) {
		t.Errorf("small media must passthrough: %d -> %d", len(small), len(comp2))
	} else {
		t.Logf("SMALL  : passthrough OK (%s unchanged)", guardFmtMB(int64(len(comp2))))
	}

	// ---- kind mapping ----
	for _, tc := range []struct {
		mt   string
		want bool
	}{
		{"videoMessage", true}, {"audioMessage", true}, {"imageMessage", true},
		{"documentMessage", false}, {"stickerMessage", false}, {"conversation", false},
	} {
		_, ok := guardAntideleteKind(tc.mt)
		if ok != tc.want {
			t.Errorf("guardAntideleteKind(%q)=%v want %v", tc.mt, ok, tc.want)
		}
	}
	t.Log("KIND MAP: OK")
}
