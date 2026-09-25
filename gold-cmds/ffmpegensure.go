// ============================================================================
// GOLD-MD — ffmpeg/ffprobe self-installer (any-platform deploy)
// File: ffmpegensure.go
// ============================================================================
// Owner rule: "jaha b deploy kro bs ho jaye" — koi b platform (Render,
// Docker, VPS, sandbox, shared host) pe ffmpeg guaranteed mile.
//
// Kaise: startup pe ek baar (sync.Once):
//   1. PATH pe ffmpeg mila? -> kuch nahi karna (apt/system install zinda hai)
//   2. Nahi mila? -> static ffmpeg build (johnvansickle.com — zero deps,
//      no root, musl-safe) download karke bot ke data dir me rakh do aur
//      PATH me daal do. Static build me ffmpeg+ffprobe dono aate hain.
//
// Download fail ho (offline host) to bot chalta rehta hai — ffmpeg commands
// un case me purana graceful error dete hain (crash nahi).
// ============================================================================

package goldcmds

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ffmpegStaticURL = "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"
	// mirror (github): agla fallback agar primary down ho
	ffmpegStaticURLMirror = "https://github.com/Uf-prince/TEST-BOT/releases/download/ffmpeg-static/ffmpeg-release-amd64-static.tar.xz"
	ffmpegDownloadTimeout = 4 * time.Minute
)

var (
	ffmpegEnsureOnce sync.Once
	ffmpegInstalled  bool
)

// ffmpegBinDir returns the private dir where static ffmpeg lives
// (created if missing). Preference: <workdir>/nexstore/ffmpeg (persists
// across restarts on hosts with a disk), fallback /tmp/gold-ffmpeg.
func ffmpegBinDir() string {
	for _, base := range []string{"nexstore", "/tmp"} {
		if base == "nexstore" {
			if wd, err := os.Getwd(); err == nil {
				dir := filepath.Join(wd, "nexstore", "ffmpeg")
				if err := os.MkdirAll(dir, 0o755); err == nil {
					return dir
				}
			}
			continue
		}
		dir := filepath.Join(base, "gold-ffmpeg")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	return "/tmp"
}

// ensureFfmpeg makes sure ffmpeg + ffprobe binaries are callable.
// Fast path: already on PATH (system install) -> zero cost.
// Slow path (once per process): download static build into ffmpegBinDir()
// and prepend to PATH for this process + all exec'd children.
// Thread-safe (sync.Once), non-fatal on failure.
func ensureFfmpeg() bool {
	ffmpegEnsureOnce.Do(func() {
		// 1) Fast path — system ffmpeg already on PATH?
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegInstalled = true
			return
		}

		// 2) Download static build (try primary, then mirror).
		dir := ffmpegBinDir()
		ffPath := filepath.Join(dir, "ffmpeg")
		fppPath := filepath.Join(dir, "ffprobe")

		// Already downloaded earlier (persisted dir)? Just ensure exec bit.
		if fileExists(ffPath) && fileExists(fppPath) {
			_ = os.Chmod(ffPath, 0o755)
			_ = os.Chmod(fppPath, 0o755)
			prependPATH(dir)
			ffmpegInstalled = true
			return
		}

		ok := downloadStaticFFmpeg(dir)
		if !ok {
			return // leave ffmpegInstalled = false; graceful degrade
		}
		if fileExists(ffPath) && fileExists(fppPath) {
			_ = os.Chmod(ffPath, 0o755)
			_ = os.Chmod(fppPath, 0o755)
			prependPATH(dir)
			ffmpegInstalled = true
		}
	})
	return ffmpegInstalled
}

// fileExists is a tiny helper (mediautil has one? keep local to avoid clash).
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// prependPATH adds dir to the front of PATH for this process.
func prependPATH(dir string) {
	cur := os.Getenv("PATH")
	if cur == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	// already at front? skip
	if strings.HasPrefix(cur, dir+string(os.PathListSeparator)) {
		return
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+cur)
}

// downloadStaticFFmpeg fetches + extracts the static tar.xz into dir.
// Returns true when ffmpeg+ffprobe are present afterwards.
func downloadStaticFFmpeg(dir string) bool {
	// sanity: only linux/amd64 builds hosted — arm hosts pe apt rasta kholna
	if runtime.GOOS != "linux" {
		return false
	}

	urls := []string{ffmpegStaticURL, ffmpegStaticURLMirror}
	for _, url := range urls {
		if fetchAndExtract(url, dir) {
			return true
		}
	}
	return false
}

// fetchAndExtract downloads tar.xz, untars ffmpeg/ffprobe into dir.
// tar.xz decode: Go stdlib me xz nahi hai — ffmpeg static builds tar.xz
// hote hain. Isliye system `tar` command use karte hain (koi b unix host
// pe tar + xz-utils usually hote hain; nahi to fail-safe false).
func fetchAndExtract(url, dir string) bool {
	tmpTar := filepath.Join(dir, "ffmpeg-static.tar.xz")
	if err := downloadFile(url, tmpTar); err != nil {
		_ = os.Remove(tmpTar)
		return false
	}
	// try `tar -xJf` (xz); fallback `tar -xzf` (gzip naming differs)
	if err := exec.Command("tar", "-xJf", tmpTar, "-C", dir).Run(); err != nil {
		// some hosts' tar lacks xz support; try 7z/bz2 alternatives na — just fail
		_ = os.Remove(tmpTar)
		return false
	}
	_ = os.Remove(tmpTar)

	// tar me files 'ffmpeg-*-amd64-static/ffmpeg' layout me hoti hain —
	// flatten: unhone nested folder me extract kiya hoga, usko upar lao.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && strings.Contains(e.Name(), "-static") {
			nested := filepath.Join(dir, e.Name())
			for _, bin := range []string{"ffmpeg", "ffprobe"} {
				src := filepath.Join(nested, bin)
				dst := filepath.Join(dir, bin)
				if fileExists(src) && !fileExists(dst) {
					_ = os.Rename(src, dst)
				}
			}
			// clean up the nested dir (baki files ki zaroorat nahi)
			_ = os.RemoveAll(nested)
		}
	}
	return fileExists(filepath.Join(dir, "ffmpeg")) && fileExists(filepath.Join(dir, "ffprobe"))
}

// downloadFile streams url to dest with a timeout + size cap.
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: ffmpegDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	// cap: static build ~40MB compressed; allow 250MB safety
	_, err = io.Copy(f, io.LimitReader(resp.Body, 250<<20))
	return err
}

// EnsureFfmpegPublic — startup hook for main.go: background me
// ffmpeg ensure karta hai (system pe nahi to static download).
func EnsureFfmpegPublic() bool {
	return ensureFfmpeg()
}
