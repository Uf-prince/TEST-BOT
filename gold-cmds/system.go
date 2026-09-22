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

const memoryRestartThreshold uint64 = 500 * 1024 * 1024

func init() {
	Register(Command{Name: "system", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO SHOW THE FULL SYSTEM INFO OF THE SERVER LIKE RAM, CPU AND STORAGE.", Run: handleSystem})
}

// MemoryRestartThresholdBytes exposes the watchdog threshold to the main package.
func MemoryRestartThresholdBytes() uint64 { return memoryRestartThreshold }

// cgroupV1Path builds the flat cgroup v1 file path used by hosts like Modal.com
// (Modal exposes cgroup v1 mounts, NOT v2 — that's why limits previously showed
// "Unavailable (cgroup limit not exposed)").
func cgroupV1Path(controller, file string) string {
	return "/sys/fs/cgroup/" + controller + "/" + file
}

// CurrentContainerMemoryBytes returns the container's current memory usage from
// cgroup v2 (memory.current) or cgroup v1 (memory.usage_in_bytes); zero when
// neither is available.
func CurrentContainerMemoryBytes() uint64 {
	if path := findCgroupFile("memory.current"); path != "" {
		if value, ok := readUintFile(path); ok {
			return value
		}
	}
	if value, ok := readUintFile(cgroupV1Path("memory", "memory.usage_in_bytes")); ok {
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
		if len(fields) == 2 && fields[0] == field {
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
// (Modal.com). If neither exposes a real per-container quota it reports
// unavailable rather than displaying the host machine's RAM as the bot's quota.
func readContainerMemory() memoryInfo {
	// ── cgroup v2 ──
	currentPath := findCgroupFile("memory.current")
	maxPath := findCgroupFile("memory.max")
	if currentPath != "" && maxPath != "" {
		current, currentOK := readUintFile(currentPath)
		limitText, err := os.ReadFile(maxPath)
		if currentOK && err == nil {
			limitValue := strings.TrimSpace(string(limitText))
			if limitValue == "max" {
				return memoryInfo{"Unlimited/not exposed", formatBytes(current), "N/A", "N/A", formatCgroupBytes("memory.peak"), formatCgroupBytes("memory.swap.max"), formatCgroupBytes("memory.swap.current")}
			}
			if limit, lerr := strconv.ParseUint(limitValue, 10, 64); lerr == nil && limit > 0 {
				available := uint64(0)
				if limit > current {
					available = limit - current
				}
				return memoryInfo{formatBytes(limit), formatBytes(current), formatBytes(available), formatPercent(current, limit), formatCgroupBytes("memory.peak"), formatCgroupBytes("memory.swap.max"), formatCgroupBytes("memory.swap.current")}
			}
			return memoryInfo{"Unavailable (invalid cgroup limit)", formatBytes(current), "N/A", "N/A", formatCgroupBytes("memory.peak"), formatCgroupBytes("memory.swap.max"), formatCgroupBytes("memory.swap.current")}
		}
	}
	// ── cgroup v1 (Modal.com) ──
	limit, limitOK := readUintFile(cgroupV1Path("memory", "memory.limit_in_bytes"))
	current, currentOK := readUintFile(cgroupV1Path("memory", "memory.usage_in_bytes"))
	if limitOK && currentOK {
		if limit == 0 || limit > (1<<40) { // >1 TB = host RAM, not a real quota
			return memoryInfo{"Unlimited/not exposed", formatBytes(current), "N/A", "N/A", v1MemFile("memory.max_usage_in_bytes"), v1MemFile("memory.memsw_limit_in_bytes"), v1MemFile("memory.memsw_usage_in_bytes")}
		}
		available := uint64(0)
		if limit > current {
			available = limit - current
		}
		return memoryInfo{formatBytes(limit), formatBytes(current), formatBytes(available), formatPercent(current, limit), v1MemFile("memory.max_usage_in_bytes"), v1MemFile("memory.memsw_limit_in_bytes"), v1MemFile("memory.memsw_usage_in_bytes")}
	}
	return memoryInfo{"Unavailable (cgroup limit not exposed)", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"}
}

// v1MemFile formats a cgroup v1 memory controller file ("N/A" when missing).
func v1MemFile(name string) string {
	value, ok := readUintFile(cgroupV1Path("memory", name))
	if !ok {
		return "N/A (not exposed)"
	}
	if value > (1 << 40) {
		return "Unlimited/not exposed"
	}
	return formatBytes(value)
}

// readContainerCPU: GOLDMD_CPU_LIMIT (deploy-configured truth, e.g. Modal's
// guaranteed 0.5 core) wins; then cgroup v2; then cgroup v1 (Modal exposes the
// burst ceiling there, which can look larger than the billing limit).
func readContainerCPU() cpuInfo {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_CPU_LIMIT")); v != "" {
		return cpuInfo{v, readCPUUsageAny()}
	}
	// ── cgroup v2 ──
	if path := findCgroupFile("cpu.max"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 2 {
				if fields[0] == "max" {
					return cpuInfo{"No cgroup limit", readCPUUsage(path)}
				}
				quota, qErr := strconv.ParseFloat(fields[0], 64)
				period, pErr := strconv.ParseFloat(fields[1], 64)
				if qErr == nil && pErr == nil && period > 0 {
					return cpuInfo{fmt.Sprintf("%.2f core(s)", quota/period), readCPUUsage(path)}
				}
			}
			return cpuInfo{"Unavailable", readCPUUsage(path)}
		}
	}
	// ── cgroup v1 (Modal.com) ──
	quota, qOK := readIntFile(cgroupV1Path("cpu", "cpu.cfs_quota_us"))
	period, pOK := readUintFile(cgroupV1Path("cpu", "cpu.cfs_period_us"))
	if qOK && pOK && quota > 0 && period > 0 {
		return cpuInfo{fmt.Sprintf("%.2f core(s) (cgroup burst ceiling)", float64(quota)/float64(period)), readCPUUsageV1()}
	}
	return cpuInfo{"Unavailable (cgroup limit not exposed)", "N/A"}
}

// readCPUUsageAny tries v2 cpu.stat then v1 cpuacct.usage.
func readCPUUsageAny() string {
	if path := findCgroupFile("cpu.max"); path != "" {
		if u := readCPUUsage(path); u != "N/A" {
			return u
		}
	}
	return readCPUUsageV1()
}

func readCPUUsage(cpuMaxPath string) string {
	statPath := filepath.Join(filepath.Dir(cpuMaxPath), "cpu.stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return "N/A"
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			return formatDurationSeconds(fields[1]) + " cumulative"
		}
	}
	return "N/A"
}

// readCPUUsageV1 reads cgroup v1 cpuacct.usage (nanoseconds).
func readCPUUsageV1() string {
	usage, ok := readUintFile(cgroupV1Path("cpuacct", "cpuacct.usage"))
	if !ok {
		return "N/A"
	}
	return time.Duration(usage).Round(time.Second).String() + " cumulative"
}

func findCgroupFile(name string) string {
	const root = "/sys/fs/cgroup"
	var found string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() && entry.Name() == name {
			found = path
		}
		return nil
	})
	return found
}

func readUintFile(path string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return value, err == nil
}

func readIntFile(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return value, err == nil
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

func formatCgroupBytes(name string) string {
	path := findCgroupFile(name)
	if path == "" {
		return "N/A (not exposed)"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "N/A"
	}
	value := strings.TrimSpace(string(data))
	if value == "max" {
		return "Unlimited/not exposed"
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "N/A"
	}
	return formatBytes(n)
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
