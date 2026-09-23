package goldcmds

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(Command{Name: "system", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO SHOW THE FULL SYSTEM INFO OF THE SERVER LIKE RAM, CPU AND STORAGE.", Run: handleSystem})
}

// NOTE (owner order): the old 450/480 MB self-restart threshold and the RAM
// TTL watchdog were REMOVED. The bot now runs on Heroku (plenty of RAM/disk),
// so no memory-based restart / cache-TTL system exists anymore.

// ============================================================================
// cgroup helpers — robust across cgroup v1 (Heroku, Modal) AND cgroup v2
// (Render, Fly, modern Docker). The previous version only searched for cgroup
// v2 filenames (memory.current / memory.max), so on Heroku's cgroup v1 layout
// it found nothing and every RAM/CPU/SWAP field showed "N/A".
// ============================================================================

// cgroupV1Path builds the flat cgroup v1 file path used by hosts like Modal.com
// and Heroku (e.g. /sys/fs/cgroup/memory/memory.limit_in_bytes).
func cgroupV1Path(controller, file string) string {
	return "/sys/fs/cgroup/" + controller + "/" + file
}

// findCgroupFileAny walks /sys/fs/cgroup and returns the first file whose base
// name matches ANY of the given names. This works for both cgroup v1 (flat
// controller dirs like memory/memory.limit_in_bytes) and cgroup v2 (unified
// memory.max), so it finds the dyno's real limits on Heroku, Modal, Render, etc.
func findCgroupFileAny(names ...string) string {
	const root = "/sys/fs/cgroup"
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() && want[entry.Name()] {
			found = path
		}
		return nil
	})
	return found
}

// cgroupExplicitPaths returns known-good direct paths for a cgroup file name,
// covering cgroup v1 controller dirs and the cgroup v2 unified root, plus any
// nested path declared in /proc/self/cgroup (used by some container runtimes).
func cgroupExplicitPaths(name string) []string {
	paths := []string{
		"/sys/fs/cgroup/" + name,             // cgroup v2 unified root
		"/sys/fs/cgroup/memory/" + name,      // cgroup v1 memory controller
		"/sys/fs/cgroup/cpu/" + name,         // cgroup v1 cpu controller
		"/sys/fs/cgroup/cpuacct/" + name,     // cgroup v1 cpuacct controller
		"/sys/fs/cgroup/cpu,cpuacct/" + name, // cgroup v1 combined controller
	}
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
			if len(parts) != 3 {
				continue
			}
			ctrl, cgPath := parts[1], strings.TrimSpace(parts[2])
			if cgPath == "" || cgPath == "/" {
				continue
			}
			for _, c := range strings.Split(ctrl, ",") {
				if c == "" {
					continue
				}
				paths = append(paths, "/sys/fs/cgroup/"+c+cgPath+"/"+name)
			}
			paths = append(paths, "/sys/fs/cgroup"+cgPath+"/"+name)
		}
	}
	return paths
}

// readCgroupString returns the trimmed contents of the first matching cgroup
// file: explicit known paths first (fast), then a full tree walk (robust).
func readCgroupString(names ...string) (string, bool) {
	for _, name := range names {
		for _, p := range cgroupExplicitPaths(name) {
			if data, err := os.ReadFile(p); err == nil {
				return strings.TrimSpace(string(data)), true
			}
		}
	}
	if p := findCgroupFileAny(names...); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(data)), true
		}
	}
	return "", false
}

// readCgroupUint parses the first matching cgroup file as an unsigned integer.
func readCgroupUint(names ...string) (uint64, bool) {
	raw, ok := readCgroupString(names...)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	return v, err == nil
}

// readCgroupInt parses the first matching cgroup file as a signed integer.
func readCgroupInt(names ...string) (int64, bool) {
	raw, ok := readCgroupString(names...)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil
}

// cgroupBytesAny formats a cgroup byte-count file ("N/A (not exposed)" when
// missing, "Unlimited/not exposed" for the kernel's "max"/huge sentinel).
func cgroupBytesAny(names ...string) string {
	raw, ok := readCgroupString(names...)
	if !ok {
		return "N/A (not exposed)"
	}
	if raw == "max" {
		return "Unlimited/not exposed"
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return "N/A"
	}
	if n > (1 << 40) { // >1 TB = host RAM, not a real per-container quota
		return "Unlimited/not exposed"
	}
	return formatBytes(n)
}

// CurrentContainerMemoryBytes returns the container's current memory usage from
// cgroup v2 (memory.current) or cgroup v1 (memory.usage_in_bytes); zero when
// neither is available.
func CurrentContainerMemoryBytes() uint64 {
	if value, ok := readCgroupUint("memory.current", "memory.usage_in_bytes"); ok {
		return value
	}
	return 0
}

func handleSystem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	platform := detectPlatform()
	memory := readContainerMemory()
	disk := readDiskInfo("/")
	processRSS := readProcessRSS()
	appDisk := directorySize(".")
	tmpDisk := directorySize("/tmp")
	dataDir := dataDirPath()
	dataDisk := directorySize(dataDir)
	cpu := readContainerCPU()

	// avg memory view — cgroup peak + current side by side
	var avgMem string
	if memory.peak != "N/A" && memory.used != "N/A" {
		avgMem = memory.peak + " / " + memory.used
	} else {
		avgMem = formatBytes(mem.HeapAlloc) + " (heap)"
	}

	// On overlay/ephemeral filesystems (Modal, some PaaS) statfs reports the
	// host disk (multi-TB) — show an honest "no fixed quota" instead of
	// nonsense like "8388608 TB free".
	fsView := disk.used + " used / " + disk.free + " free"
	if disk.totalB >= (1<<50) {
		fsView = "ephemeral overlay FS (no fixed quota)"
	}

	text := fmt.Sprintf("🔰 *GOLD-MD REAL CONTAINER STATUS*\n\n"+
		"*PLATFORM* :❯ %s\n"+
		"*CPU LIMIT* :❯ %s\n"+
		"*CPU USAGE* :❯ %s\n"+
		"*CPU LOAD* :❯ %s\n"+
		"*RAM LIMIT* :❯ %s\n"+
		"*RAM USED* :❯ %s (%s)\n"+
		"*RAM AVAILABLE* :❯ %s\n"+
		"*RAM PEAK* :❯ %s\n"+
		"*SWAP LIMIT* :❯ %s\n"+
		"*SWAP USED* :❯ %s\n"+
		"*PROCESS RSS* :❯ %s\n"+
		"*PROCESS ALLOC* :❯ %s\n"+
		"*PROCESS HEAP* :❯ %s\n"+
		"*PROCESS SYS* :❯ %s\n"+
		"*GOROUTINES* :❯ %d\n"+
		"*DISK QUOTA* :❯ %s\n"+
		"*APP DISK USED* :❯ %s\n"+
		"*TMP DISK USED* :❯ %s\n"+
		"*DATA VOLUME* :❯ %s (%s)\n"+
		"*CONTAINER FS VIEW* :❯ %s\n"+
		"*GPU* :❯ %s\n"+
		"*CONTAINER UPTIME* :❯ %s\n\n"+
		"*MEMORY / DISK DETAILS*\n"+
		"*RAM PEAK/CURR* :❯ %s / %s\n"+
		"*HEAP ALLOCS/FREES* :❯ %s / %s\n"+
		"*GC CYCLES* :❯ %d total / %d forced\n"+
		"*TOTAL ALLOCATED* :❯ %s\n"+
		"*DISK READ/WRITE* :❯ %s / %s",
		platform, cpu.limit, cpu.usage, readLoadAverage(), memory.limit, memory.used,
		memory.usedPct, memory.available, memory.peak, memory.swapLimit, memory.swapUsed, processRSS, formatBytes(mem.Alloc),
		formatBytes(mem.HeapAlloc), formatBytes(mem.Sys), runtime.NumGoroutine(),
		diskQuotaText(platform), formatBytes(appDisk), formatBytes(tmpDisk),
		dataDir, formatBytes(dataDisk), fsView,
		detectGPU(), readContainerUptime(),
		memory.peak, avgMem, fmtUintWithComma(mem.Mallocs), fmtUintWithComma(mem.Frees),
		mem.NumGC, mem.NumForcedGC,
		formatBytes(mem.TotalAlloc),
		readProcIOStat("read_bytes"), readProcIOStat("write_bytes"))

	s.Reply(info, text)
}

// readProcIOStat reads /proc/self/io field (read_bytes / write_bytes).
func readProcIOStat(field string) string {
	data, err := os.ReadFile("/proc/self/io")
	if err != nil {
		return "N/A"
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// /proc/self/io keys carry a trailing colon (e.g. "read_bytes:"), so
		// strip it before comparing — otherwise every field looked "missing".
		if len(fields) == 2 && strings.TrimSuffix(fields[0], ":") == field {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return "N/A"
			}
			return fmtUintWithComma(value)
		}
	}
	return "N/A"
}

// fmtUintWithComma formats n with thousand separators (1,234,567).
func fmtUintWithComma(n uint64) string {
	if n == 0 {
		return "0"
	}
	digits := strconv.FormatUint(n, 10)
	var b strings.Builder
	pre := len(digits) % 3
	if pre > 0 {
		b.WriteString(digits[:pre])
	}
	for i := pre; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// detectPlatform names the hosting platform for the status card.
// Modal injects MODAL_IS_REMOTE=1 into every container; GOLDMD_PLATFORM lets
// the deploy script override the label on any host.
func detectPlatform() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_PLATFORM")); v != "" {
		return v
	}
	// Heroku: DYNO is always set (e.g. "web.1"). Heroku runs cgroup v1.
	if dyno := strings.TrimSpace(os.Getenv("DYNO")); dyno != "" {
		return "HEROKU (" + dyno + ")"
	}
	if strings.TrimSpace(os.Getenv("MODAL_IS_REMOTE")) != "" {
		p := "GOLD"
		if r := strings.TrimSpace(os.Getenv("MODAL_REGION")); r != "" {
			p += " (" + r + ")"
		}
		return p
	}
	return "self-hosted container"
}

// diskQuotaText replaces the old hardcoded "Not exposed by Render" line.
func diskQuotaText(platform string) string {
	if strings.Contains(platform, "HEROKU") {
		return "Ephemeral dyno filesystem (no fixed quota)"
	}
	if strings.Contains(platform, "GOLD") || strings.Contains(platform, "Modal") {
		return "No fixed quota (volume-backed)"
	}
	return "N/A (not exposed by host)"
}

// dataDirPath mirrors the main config: GOLDMD_DATA_DIR or default nexstore.
func dataDirPath() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_DATA_DIR")); v != "" {
		return v
	}
	return "nexstore"
}

type memoryInfo struct {
	limit, used, available, usedPct, peak, swapLimit, swapUsed string
}

type diskInfo struct {
	total, used, free, usedPct string
	totalB, freeB              uint64
}

type cpuInfo struct {
	limit, usage string
}

// readContainerMemory reads cgroup v2 first (Render/Fly etc.), then cgroup v1
// (Heroku, Modal.com). If neither exposes a real per-container quota it falls
// back to /proc/meminfo (host view, clearly labelled) rather than showing N/A.
func readContainerMemory() memoryInfo {
	// ── cgroup v2 (unified) ──
	if current, ok := readCgroupUint("memory.current"); ok {
		if maxRaw, ok2 := readCgroupString("memory.max"); ok2 {
			if maxRaw == "max" {
				return memoryInfo{"Unlimited/not exposed", formatBytes(current), "N/A", "N/A",
					cgroupBytesAny("memory.peak"), cgroupBytesAny("memory.swap.max"), cgroupBytesAny("memory.swap.current")}
			}
			if limit, err := strconv.ParseUint(maxRaw, 10, 64); err == nil && limit > 0 {
				available := uint64(0)
				if limit > current {
					available = limit - current
				}
				return memoryInfo{formatBytes(limit), formatBytes(current), formatBytes(available), formatPercent(current, limit),
					cgroupBytesAny("memory.peak"), cgroupBytesAny("memory.swap.max"), cgroupBytesAny("memory.swap.current")}
			}
		}
	}
	// ── cgroup v1 (Heroku, Modal.com) ──
	limit, limitOK := readCgroupUint("memory.limit_in_bytes")
	current, currentOK := readCgroupUint("memory.usage_in_bytes")
	if limitOK && currentOK {
		if limit == 0 || limit > (1<<40) { // >1 TB = host RAM, not a real quota
			return memoryInfo{"Unlimited/not exposed", formatBytes(current), "N/A", "N/A",
				cgroupBytesAny("memory.max_usage_in_bytes"), cgroupBytesAny("memory.memsw.limit_in_bytes"), cgroupBytesAny("memory.memsw.usage_in_bytes")}
		}
		available := uint64(0)
		if limit > current {
			available = limit - current
		}
		return memoryInfo{formatBytes(limit), formatBytes(current), formatBytes(available), formatPercent(current, limit),
			cgroupBytesAny("memory.max_usage_in_bytes"), cgroupBytesAny("memory.memsw.limit_in_bytes"), cgroupBytesAny("memory.memsw.usage_in_bytes")}
	}
	// ── fallback: /proc/meminfo (host view, clearly labelled) ──
	if total, avail, ok := readProcMeminfo(); ok {
		used := uint64(0)
		if total > avail {
			used = total - avail
		}
		return memoryInfo{formatBytes(total) + " (host)", formatBytes(used), formatBytes(avail), formatPercent(used, total), "N/A", "N/A", "N/A"}
	}
	return memoryInfo{"Unavailable (cgroup limit not exposed)", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"}
}

// readProcMeminfo returns MemTotal and MemAvailable (bytes) from /proc/meminfo.
func readProcMeminfo() (total, available uint64, ok bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				total = v * 1024
			}
		case "MemAvailable:":
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				available = v * 1024
			}
		}
	}
	return total, available, total > 0
}

// readContainerCPU: GOLDMD_CPU_LIMIT (deploy-configured truth, e.g. Modal's
// guaranteed 0.5 core) wins; then cgroup v2; then cgroup v1 (Heroku/Modal).
func readContainerCPU() cpuInfo {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_CPU_LIMIT")); v != "" {
		return cpuInfo{v, readCPUUsageAny()}
	}
	// ── cgroup v2 ──
	if raw, ok := readCgroupString("cpu.max"); ok {
		fields := strings.Fields(raw)
		if len(fields) >= 2 {
			if fields[0] == "max" {
				return cpuInfo{"No cgroup limit", readCPUUsageAny()}
			}
			quota, qErr := strconv.ParseFloat(fields[0], 64)
			period, pErr := strconv.ParseFloat(fields[1], 64)
			if qErr == nil && pErr == nil && period > 0 {
				return cpuInfo{fmt.Sprintf("%.2f core(s)", quota/period), readCPUUsageAny()}
			}
		}
	}
	// ── cgroup v1 (Heroku, Modal.com) ──
	quota, qOK := readCgroupInt("cpu.cfs_quota_us")
	period, pOK := readCgroupUint("cpu.cfs_period_us")
	if qOK && pOK && quota > 0 && period > 0 {
		return cpuInfo{fmt.Sprintf("%.2f core(s)", float64(quota)/float64(period)), readCPUUsageAny()}
	}
	return cpuInfo{"Unavailable (cgroup limit not exposed)", readCPUUsageAny()}
}

// readCPUUsageAny tries cgroup v2 cpu.stat (usage_usec) then cgroup v1
// cpuacct.usage (nanoseconds).
func readCPUUsageAny() string {
	if p := findCgroupFileAny("cpu.stat"); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "usage_usec" {
					return formatDurationSeconds(fields[1]) + " cumulative"
				}
			}
		}
	}
	if usage, ok := readCgroupUint("cpuacct.usage"); ok {
		return time.Duration(usage).Round(time.Second).String() + " cumulative"
	}
	return "N/A"
}

func readProcessRSS() string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "N/A"
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				return formatBytes(value * 1024)
			}
		}
	}
	return "N/A"
}

func directorySize(path string) uint64 {
	var total uint64
	_ = filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.Type().IsRegular() {
			if info, statErr := entry.Info(); statErr == nil {
				total += uint64(info.Size())
			}
		}
		return nil
	})
	return total
}

func readDiskInfo(path string) diskInfo {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskInfo{"N/A", "N/A", "N/A", "N/A", 0, 0}
	}
	blockSize := uint64(stat.Bsize)
	total := uint64(stat.Blocks) * blockSize
	free := uint64(stat.Bavail) * blockSize
	used := uint64(0)
	if total > free {
		used = total - free
	}
	return diskInfo{formatBytes(total), formatBytes(used), formatBytes(free), formatPercent(used, total), total, free}
}

func readLoadAverage() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "N/A (host load unavailable)"
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return "N/A"
	}
	return fields[0] + " / " + fields[1] + " / " + fields[2] + " (host 1m/5m/15m)"
}

func readContainerUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "N/A"
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "N/A"
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "N/A"
	}
	d := time.Duration(seconds * float64(time.Second))
	return d.Round(time.Second).String()
}

func detectGPU() string {
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return "NVIDIA device detected"
	}
	if _, err := os.Stat("/dev/dri"); err == nil {
		return "GPU device detected"
	}
	return "Not available (no GPU on this host)"
}

func formatDurationSeconds(raw string) string {
	usec, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return "N/A"
	}
	return (time.Duration(usec) * time.Microsecond).Round(time.Second).String()
}

func formatBytes(n uint64) string {
	if n == 0 {
		return "N/A"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func formatPercent(used, total uint64) string {
	if total == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", float64(used)*100/float64(total))
}
