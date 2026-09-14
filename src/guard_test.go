package main

// ============================================================================
// GUARD COMPRESSOR TESTS — asli ffmpeg round-trip verify
// ============================================================================
// Ye test ek bada testsrc video (~55MB) generate karta hai (testsrc2 + noise,
// high entropy → compress hona mushkil, realistic worst-case) aur phir guard
// ladder chala ke check karta hai ke:
//   1. 50MB limit cross → guard trigger hota hai
//   2. Compressed output limit ke andar aata hai
//   3. Size me real drop hota hai (target ~8MB)
//   4. Fail paths graceful hain (document kind → block, no crash)
// ============================================================================

import (
	"os"
	"os/exec"
	"testing"
)

// needs ffmpeg — sandbox me hai; skip agar nahi
func guardTestFfmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
}

func guardMakeBigVideo(t *testing.T, mb int) string {
	t.Helper()
	out := t.TempDir() + "/big.mp4"
	// testsrc2 1920x1080 ~ big entropy; 60s @ high bitrate ≈ 55MB target
	dur := mb // 1MB/sec approx with these settings
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:amplitude=0.5",
		"-t", itoa(dur),
		"-c:v", "libx264", "-preset", "ultrafast", "-b:v", "8M",
		"-c:a", "aac", "-b:a", "128k",
		out)
	if out2, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("big video gen fail: %v\n%s", err, out2)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat fail: %v", err)
	}
	t.Logf("generated big video: %d bytes (%.1f MB)", st.Size(), float64(st.Size())/(1024*1024))
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Test 1: badi video compress → limit ke andar aani chahiye
func TestGuardCompressVideoBig(t *testing.T) {
	guardTestFfmpeg(t)
	big := guardMakeBigVideo(t, 60) // ~55MB+
	defer os.Remove(big)

	st, _ := os.Stat(big)
	if st.Size() <= guardLimitBytes() {
		t.Fatalf("test setup fail: video (%d bytes) limit se badi honi chahiye", st.Size())
	}

	out, size, note, ok := guardCompressFile(guardVideo, big, guardTargetBytes(), guardLimitBytes())
	if !ok {
		t.Fatalf("guard compress FAILED — big video should compress under limit")
	}
	defer os.Remove(out)
	t.Logf("COMPRESSED: %d -> %d bytes (%.1f MB), note=%q", st.Size(), size, float64(size)/(1024*1024), note)

	if size > guardLimitBytes() {
		t.Fatalf("compressed size %d still over limit %d", size, guardLimitBytes())
	}
	if size >= st.Size() {
		t.Fatalf("no size reduction! %d -> %d", st.Size(), size)
	}
	// compressed file readable hai?
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output file missing: %v", err)
	}
}

// Test 2: document kind (zip/apk) → compress impossible → block path
func TestGuardCompressDocumentFails(t *testing.T) {
	guardTestFfmpeg(t)
	// random-ish binary (zip jaisa — incompressible)
	out := t.TempDir() + "/fake.zip"
	f, _ := os.Create(out)
	for i := 0; i < 1000; i++ {
		f.Write([]byte{byte(i * 7 % 256), byte(i * 13 % 256), byte(i * 29 % 256)})
	}
	f.Close()

	_, _, _, ok := guardCompressFile(guardDocument, out, guardTargetBytes(), guardLimitBytes())
	if ok {
		t.Fatalf("document compress should FAIL (re-encode impossible)")
	}
}

// Test 3: audio ladder
func TestGuardCompressAudio(t *testing.T) {
	guardTestFfmpeg(t)
	src := t.TempDir() + "/big.mp3"
	// 3 min sine @ 320kbps ≈ 7MB — target 1MB chase test
	// (16/32/48kbps ladder me 48k×180s=1.08MB, 32k×180s=0.72MB → target hit)
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=180",
		"-c:a", "libmp3lame", "-b:a", "320k", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("audio gen fail: %v\n%s", err, out)
	}
	defer os.Remove(src)

	// target 1MB pe chase — 320kbps 10min = 24MB → 1MB tak compress hona chahiye
	out, size, _, ok := guardCompressAudio(src, 1024*1024, guardLimitBytes())
	if !ok {
		t.Fatalf("audio compress failed")
	}
	defer os.Remove(out)
	t.Logf("AUDIO: compressed to %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
	if size > 1024*1024*15/10 { // 1.5MB tolerance (target hit ya near)
		t.Fatalf("audio compress too big: %d", size)
	}
}

// Test 4: env knobs
func TestGuardEnvKnobs(t *testing.T) {
	old := os.Getenv("GOLDMD_GUARD_LIMIT_MB")
	defer os.Setenv("GOLDMD_GUARD_LIMIT_MB", old)
	os.Setenv("GOLDMD_GUARD_LIMIT_MB", "25")
	if got := guardLimitBytes(); got != 25*1024*1024 {
		t.Fatalf("limit knob fail: %d", got)
	}
}

// ============================================================================
// FORCE-TIER TESTS — owner order v2: HAR media compressor room me
// 5MB ho ya 500MB — sab compress. Chota (<1MB) pass. Doc >limit block.
// ============================================================================

// Test 5: policy tiers — 3-tier decision table verify
func TestGuardPolicyTiers(t *testing.T) {
	t.Setenv("GOLDMD_GUARD_DISABLED", "")
	t.Setenv("GOLDMD_GUARD_FORCE", "")

	// <floor (1MB) -> tier 0 pass (thumbnails nahi chherrte)
	if got := guardPolicy(512 * 1024); got != 0 {
		t.Fatalf("512KB -> tier %d, want 0 (pass)", got)
	}
	// 5MB (floor < size <= limit) -> tier 1 FORCE
	if got := guardPolicy(5 * 1024 * 1024); got != 1 {
		t.Fatalf("5MB -> tier %d, want 1 (force compress)", got)
	}
	// 20MB -> tier 1 FORCE
	if got := guardPolicy(20 * 1024 * 1024); got != 1 {
		t.Fatalf("20MB -> tier %d, want 1 (force compress)", got)
	}
	// 50MB (== limit) -> tier 1 FORCE (boundary inclusive)
	if got := guardPolicy(50 * 1024 * 1024); got != 1 {
		t.Fatalf("50MB -> tier %d, want 1 (force compress, boundary)", got)
	}
	// 500MB -> tier 2 overlimit
	if got := guardPolicy(500 * 1024 * 1024); got != 2 {
		t.Fatalf("500MB -> tier %d, want 2 (overlimit)", got)
	}
	// 1GB -> tier 2
	if got := guardPolicy(1024 * 1024 * 1024); got != 2 {
		t.Fatalf("1GB -> tier %d, want 2 (overlimit)", got)
	}
	// guard OFF -> sab tier 0
	t.Setenv("GOLDMD_GUARD_DISABLED", "1")
	if got := guardPolicy(700 * 1024 * 1024); got != 0 {
		t.Fatalf("guard OFF 700MB -> tier %d, want 0", got)
	}
}

// Test 6: force knob — GOLDMD_GUARD_FORCE=0 -> purana limit-only mode
func TestGuardForceKnob(t *testing.T) {
	t.Setenv("GOLDMD_GUARD_FORCE", "0")
	if guardForceMode() {
		t.Fatalf("FORCE=0 -> force mode should be OFF")
	}
	// 20MB force-off -> tier 0 pass (limit-only mode: sirf >50MB pakadta)
	if got := guardPolicy(20 * 1024 * 1024); got != 0 {
		t.Fatalf("20MB FORCE=0 -> tier %d, want 0 (limit-only mode)", got)
	}
	// 500MB force-off -> still tier 2 (overlimit always active)
	if got := guardPolicy(500 * 1024 * 1024); got != 2 {
		t.Fatalf("500MB FORCE=0 -> tier %d, want 2 (overlimit still active)", got)
	}
}

// Test 7: 5MB image FORCE compress — imagine ne 5MB banai, guard ne X ki
func TestGuardForceImage5MB(t *testing.T) {
	guardTestFfmpeg(t)
	src := t.TempDir() + "/big5.jpg"
	// pure-noise 2560x1600 @ q1 -> ~5MB JPEG (imagine-generated jaisa high-entropy)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "nullsrc=s=2560x1600",
		"-vf", "geq=random(1)*255:128:128",
		"-frames:v", "1", "-q:v", "1", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("image gen fail: %v\n%s", err, out)
	}
	defer os.Remove(src)
	st, _ := os.Stat(src)
	t.Logf("generated image: %d bytes (%.2f MB)", st.Size(), float64(st.Size())/(1024*1024))
	if st.Size() <= guardFloorBytes() {
		t.Fatalf("test setup fail: image %d should be > 1MB floor", st.Size())
	}

	out, size, note, ok := guardCompressImage(src, guardTargetBytes(), guardLimitBytes())
	if !ok {
		t.Fatalf("5MB image FORCE compress failed")
	}
	defer os.Remove(out)
	t.Logf("IMAGE: %d -> %d bytes (%.2f MB), note=%q", st.Size(), size, float64(size)/(1024*1024), note)
	if size >= st.Size() {
		t.Fatalf("image did not shrink: %d -> %d", st.Size(), size)
	}
	if size > guardTargetBytes()*3 { // quality ladder 8MB target chase karta hai
		t.Fatalf("image too big after ladder: %d", size)
	}
}

// Test 8: 20MB video FORCE compress — aivideo ne 20MB banai, guard ne X ki
func TestGuardForceVideo20MB(t *testing.T) {
	guardTestFfmpeg(t)
	// 21s @ 1MB/s = 20MB — force tier (1-50MB)
	src := guardMakeBigVideo(t, 21)
	defer os.Remove(src)
	st, _ := os.Stat(src)
	if st.Size() <= guardFloorBytes() || st.Size() > guardLimitBytes() {
		t.Fatalf("test setup fail: video %d should be in force tier (1-50MB)", st.Size())
	}

	// engine-level force compress (tier 1)
	out, size, note, ok := guardCompressFile(guardVideo, src, guardTargetBytes(), guardLimitBytes())
	if !ok {
		t.Fatalf("20MB video FORCE compress failed")
	}
	defer os.Remove(out)
	t.Logf("VIDEO20: %d -> %d bytes (%.2f MB), note=%q", st.Size(), size, float64(size)/(1024*1024), note)

	if size >= st.Size() {
		t.Fatalf("no shrink: %d -> %d", st.Size(), size)
	}
	// force tier: target ke andar chase (8MB target, tolerance 2x)
	if size > guardTargetBytes()*2 {
		t.Fatalf("video still too big after ladder: %d (target %d)", size, guardTargetBytes())
	}
}

// Test 9: small file (<1MB floor) -> PASS — thumbnails nahi chherrte
func TestGuardSmallPass(t *testing.T) {
	src := t.TempDir() + "/small.jpg"
	f, _ := os.Create(src)
	f.WriteString("JUNK_NOT_REAL_MEDIA_BUT_SMALL")
	f.Close()
	defer os.Remove(src)
	st, _ := os.Stat(src)
	if st.Size() > guardFloorBytes() {
		t.Fatalf("test setup fail")
	}
	if got := guardPolicy(st.Size()); got != 0 {
		t.Fatalf("small file -> tier %d, want 0 (pass, thumbnails spared)", got)
	}
}

// Test 10: overlimit doc -> BLOCK — 500MB random zip-type media block hota hai
func TestGuardOverLimitDocBlock(t *testing.T) {
	if got := guardPolicy(500 * 1024 * 1024); got != 2 {
		t.Fatalf("500MB -> tier %d, want 2 (overlimit block-on-fail)", got)
	}
	out := t.TempDir() + "/fakebig.zip"
	f, _ := os.Create(out)
	buf := make([]byte, 1024)
	for i := range buf {
		buf[i] = byte(i * 31 % 256)
	}
	for i := 0; i < 1000; i++ { // ~1MB incompressible
		f.Write(buf)
	}
	f.Close()
	defer os.Remove(out)
	if _, _, _, ok := guardCompressFile(guardDocument, out, guardTargetBytes(), guardLimitBytes()); ok {
		t.Fatalf("incompressible doc compress should fail -> overlimit caller blocks")
	}
}
