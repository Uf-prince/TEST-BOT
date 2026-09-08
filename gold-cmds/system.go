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
	Register(Command{Name: "system", Category: "OWNER & SYSTEM", Desc: "Show bot system / runtime info", Run: handleSystem})
}

// MemoryRestartThresholdBytes exposes the watchdog threshold to the main package.
func MemoryRestartThresholdBytes() uint64 { return memoryRestartThreshold }

// CurrentContainerMemoryBytes returns cgroup memory.current, or zero when unavailable.
func CurrentContainerMemoryBytes() uint64 {
	path := findCgroupFile("memory.current")
	if path == "" {
		return 0
	}
	value, ok := readUintFile(path)
	if !ok {
		return 0
	}
	return value
}

func handleSystem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	memory := readContainerMemory()
	disk := readDiskInfo("/")
	processRSS := readProcessRSS()
	appDisk := directorySize(".")
	tmpDisk := directorySize("/tmp")
	cpu := readContainerCPU()

	text := fmt.Sprintf("🖥️ *GOLD-MD REAL CONTAINER STATUS*\n\n"+
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
		"*CONTAINER FS VIEW* :❯ %s used / %s free\n"+
		"*GPU* :❯ %s\n"+
		"*CONTAINER UPTIME* :❯ %s\n"+
		"*GO RUNTIME* :❯ %s\n"+
		"*BOT IS RUNNING* ✅",
		cpu.limit, cpu.usage, readLoadAverage(), memory.limit, memory.used,
		memory.usedPct, memory.available, memory.peak, memory.swapLimit, memory.swapUsed, processRSS, formatBytes(mem.Alloc),
		formatBytes(mem.HeapAlloc), formatBytes(mem.Sys), runtime.NumGoroutine(),
		"Not exposed by Render", formatBytes(appDisk), formatBytes(tmpDisk), disk.used, disk.free, detectGPU(),
		readContainerUptime(), runtime.Version())

	s.Reply(info, text)
}

type memoryInfo struct {
	limit, used, available, usedPct, peak, swapLimit, swapUsed string
}

type diskInfo struct {
	total, used, free, usedPct string
}

type cpuInfo struct {
	limit, usage string
}

// readContainerMemory reads cgroup memory.current/memory.max, not host /proc/meminfo.
// If the platform does not expose a cgroup limit, it reports unavailable rather than
// displaying the host machine's RAM as the bot's quota.
func readContainerMemory() memoryInfo {
	currentPath := findCgroupFile("memory.current")
	maxPath := findCgroupFile("memory.max")
	if currentPath == "" || maxPath == "" {
		return memoryInfo{"Unavailable (cgroup limit not exposed)", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"}
	}
	current, currentOK := readUintFile(currentPath)
	limitText, err := os.ReadFile(maxPath)
	if !currentOK || err != nil {
		return memoryInfo{"Unavailable (cgroup limit not exposed)", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"}
	}
	limitValue := strings.TrimSpace(string(limitText))
	if limitValue == "max" {
		return memoryInfo{"Unlimited/not exposed", formatBytes(current), "N/A", "N/A", formatCgroupBytes("memory.peak"), formatCgroupBytes("memory.swap.max"), formatCgroupBytes("memory.swap.current")}
	}
	limit, err := strconv.ParseUint(limitValue, 10, 64)
	if err != nil || limit == 0 {
		return memoryInfo{"Unavailable (invalid cgroup limit)", formatBytes(current), "N/A", "N/A", "N/A", "N/A", "N/A"}
	}
	available := uint64(0)
	if limit > current {
		available = limit - current
	}
	return memoryInfo{formatBytes(limit), formatBytes(current), formatBytes(available), formatPercent(current, limit), formatCgroupBytes("memory.peak"), formatCgroupBytes("memory.swap.max"), formatCgroupBytes("memory.swap.current")}
}

func readContainerCPU() cpuInfo {
	path := findCgroupFile("cpu.max")
	if path == "" {
		return cpuInfo{"Unavailable (cgroup limit not exposed)", "N/A"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cpuInfo{"Unavailable", "N/A"}
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return cpuInfo{"Unavailable", "N/A"}
	}
	if fields[0] == "max" {
		return cpuInfo{"No cgroup limit", readCPUUsage(path)}
	}
	quota, qErr := strconv.ParseFloat(fields[0], 64)
	period, pErr := strconv.ParseFloat(fields[1], 64)
	if qErr != nil || pErr != nil || period <= 0 {
		return cpuInfo{"Unavailable", "N/A"}
	}
	return cpuInfo{fmt.Sprintf("%.2f core(s)", quota/period), readCPUUsage(path)}
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
		return diskInfo{"N/A", "N/A", "N/A", "N/A"}
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bavail * blockSize
	used := uint64(0)
	if total > free {
		used = total - free
	}
	return diskInfo{formatBytes(total), formatBytes(used), formatBytes(free), formatPercent(used, total)}
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
	return "Not available (Render Free has no GPU)"
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
