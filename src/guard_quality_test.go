package main

// ============================================================================
// GUARD DOUBLE-COMPRESS QUALITY REPORT (owner: jani)
// ============================================================================
// Ye test ASLI production guardCompressFile() engine ko chalata hai (wahi
// function jo .tt / download commands ke SendVideoFile me lagta hai) aur
// measure karta hai:
//
//   PASS 0 : original video      -> size, resolution, bitrate
//   PASS 1 : guard compress #1   -> size, resolution, bitrate, % drop
//   PASS 2 : guard compress #2   -> size, resolution, bitrate, % drop
//
// Output: /workspace/guardtest/quality_report.txt (aur stdout pe bhi).
//
// Run: go test -run TestGuardDoubleCompressQuality -v ./src/
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type vidMeta struct {
	size    int64
	w, h    int
	secs    float64
	bitrate int // kbps (video stream)
}

func probeFull(path string) vidMeta {
	var m vidMeta
	if st, err := os.Stat(path); err == nil {
		m.size = st.Size()
	}
	// width x height
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "x")
		if len(parts) == 2 {
			m.w, _ = strconv.Atoi(parts[0])
			m.h, _ = strconv.Atoi(parts[1])
		}
	}
	// duration
	out, err = exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err == nil {
		m.secs, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	}
	// video bitrate
	out, err = exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=bit_rate", "-of", "csv=p=0", path).Output()
	if err == nil {
		br, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		m.bitrate = br / 1000
	}
	return m
}

func mb(n int64) string { return fmt.Sprintf("%.2f MB", float64(n)/(1024*1024)) }

func pctDrop(from, to int64) string {
	if from <= 0 {
		return "n/a"
	}
	d := (1.0 - float64(to)/float64(from)) * 100.0
	return fmt.Sprintf("%.1f%%", d)
}

func TestGuardDoubleCompressQuality(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := "/workspace/guardtest"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no sample dir: %v", err)
	}
	var samples []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") &&
			!strings.Contains(e.Name(), "_pass") {
			samples = append(samples, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(samples)
	if len(samples) == 0 {
		t.Skip("no sample mp4s")
	}

	var rep strings.Builder
	rep.WriteString("========================================================================\n")
	rep.WriteString("GOLD-MD GUARD — DOUBLE COMPRESS QUALITY REPORT\n")
	rep.WriteString("engine: guardCompressFile(guardVideo, target=8MB, maxLimit=50MB)\n")
	rep.WriteString("========================================================================\n\n")

	for _, src := range samples {
		name := filepath.Base(src)
		rep.WriteString("########################################################################\n")
		rep.WriteString("SAMPLE: " + name + "\n")
		rep.WriteString("########################################################################\n")

		orig := probeFull(src)
		rep.WriteString(fmt.Sprintf("PASS 0 (ORIGINAL)      : size=%s  res=%dx%d  dur=%.1fs  vbitrate=%dkbps\n",
			mb(orig.size), orig.w, orig.h, orig.secs, orig.bitrate))

		// ---- PASS 1 ----
		out1, size1, _, ok1 := guardCompressFile(context.Background(), guardVideo, src, guardTargetBytes(), guardLimitBytes())
		if !ok1 {
			rep.WriteString("PASS 1 (COMPRESS #1)   : FAILED (guard ne compress nahi kiya)\n\n")
			continue
		}
		m1 := probeFull(out1)
		rep.WriteString(fmt.Sprintf("PASS 1 (COMPRESS #1)   : size=%s  res=%dx%d  dur=%.1fs  vbitrate=%dkbps  | size drop=%s  res drop=%s\n",
			mb(size1), m1.w, m1.h, m1.secs, m1.bitrate,
			pctDrop(orig.size, size1), resDrop(orig.h, m1.h)))

		// ---- PASS 2 (double compress) ----
		out2, size2, _, ok2 := guardCompressFile(context.Background(), guardVideo, out1, guardTargetBytes(), guardLimitBytes())
		if !ok2 {
			rep.WriteString("PASS 2 (COMPRESS #2)   : FAILED (dobara compress nahi hua)\n\n")
			os.Remove(out1)
			continue
		}
		m2 := probeFull(out2)
		rep.WriteString(fmt.Sprintf("PASS 2 (COMPRESS #2)   : size=%s  res=%dx%d  dur=%.1fs  vbitrate=%dkbps  | size drop=%s  res drop=%s\n",
			mb(size2), m2.w, m2.h, m2.secs, m2.bitrate,
			pctDrop(size1, size2), resDrop(m1.h, m2.h)))

		rep.WriteString("\n--- CUMULATIVE (original -> double compressed) ---\n")
		rep.WriteString(fmt.Sprintf("TOTAL SIZE DROP        : %s -> %s  = %s\n",
			mb(orig.size), mb(size2), pctDrop(orig.size, size2)))
		rep.WriteString(fmt.Sprintf("TOTAL RESOLUTION DROP  : %dx%d -> %dx%d  = %s\n",
			orig.w, orig.h, m2.w, m2.h, resDrop(orig.h, m2.h)))
		rep.WriteString(fmt.Sprintf("TOTAL BITRATE DROP     : %dkbps -> %dkbps  = %s\n",
			orig.bitrate, m2.bitrate, pctDrop(int64(orig.bitrate), int64(m2.bitrate))))
		rep.WriteString(fmt.Sprintf("QUALITY VERDICT        : %s\n\n", verdict(orig, m2)))

		os.Remove(out1)
		os.Remove(out2)
	}

	repPath := filepath.Join(dir, "quality_report.txt")
	_ = os.WriteFile(repPath, []byte(rep.String()), 0644)
	t.Logf("\n%s", rep.String())
	t.Logf("report written: %s", repPath)
}

func resDrop(fromH, toH int) string {
	if fromH <= 0 {
		return "n/a"
	}
	d := (1.0 - float64(toH)/float64(fromH)) * 100.0
	return fmt.Sprintf("%.1f%% (%dp->%dp)", d, fromH, toH)
}

func verdict(orig, final vidMeta) string {
	// quality proxy: resolution + bitrate retention
	resRet := 100.0
	if orig.h > 0 {
		resRet = float64(final.h) / float64(orig.h) * 100.0
	}
	brRet := 100.0
	if orig.bitrate > 0 {
		brRet = float64(final.bitrate) / float64(orig.bitrate) * 100.0
	}
	loss := 100.0 - (resRet*0.5 + brRet*0.5)
	switch {
	case loss < 20:
		return fmt.Sprintf("GOOD (loss ~%.0f%%)", loss)
	case loss < 40:
		return fmt.Sprintf("NOTICEABLE (loss ~%.0f%%)", loss)
	case loss < 60:
		return fmt.Sprintf("HEAVY (loss ~%.0f%%)", loss)
	default:
		return fmt.Sprintf("SEVERE (loss ~%.0f%%)", loss)
	}
}
