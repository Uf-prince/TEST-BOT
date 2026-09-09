package main

import "sync/atomic"

// ============================================================================
// GOLD-MD — In-flight command (busy) tracking
// File: busy.go
// ============================================================================
// The watchdog (reconnect_watchdog.go) and the memory watchdog (main.go
// memoryWatchdog) both used to fire "bot is stuck" reactions while a heavy
// DOWNLOAD command was still running:
//   - cgroup memory.current (which includes file page-cache) spikes while a
//     big media file is being written to /tmp -> memoryWatchdog Tier 2
//     self-restarted the whole process mid-download.
//   - a transient socket blip during heavy traffic -> Disconnected-event
//     fast-reconnect raced the in-flight download.
//
// Every command dispatch in handler.go now wraps its run with
// beginCmdBusy()/defer-style endCmdBusy(). While ANY command (a download
// pipeline runs up to 3 minutes inside RunWithTimeout) is in flight,
// cmdBusyActive() == true and both watchdogs HOLD OFF:
//   - no self-restart (the download completes first),
//   - no fast-path reconnect (whatsmeow's own EnableAutoReconnect is still
//     there as the safety net for a real disconnect).
// ============================================================================

// cmdBusyCount is the number of currently-executing command dispatches.
var cmdBusyCount int32

// beginCmdBusy marks a command dispatch as in-flight.
func beginCmdBusy() { atomic.AddInt32(&cmdBusyCount, 1) }

// endCmdBusy clears one in-flight command dispatch.
func endCmdBusy() {
	// floor at zero — a bug must never make this go negative and stick busy.
	for {
		v := atomic.LoadInt32(&cmdBusyCount)
		if v <= 0 || atomic.CompareAndSwapInt32(&cmdBusyCount, v, v-1) {
			return
		}
	}
}

// cmdBusyActive reports whether at least one command (possibly a long
// download) is currently executing.
func cmdBusyActive() bool { return atomic.LoadInt32(&cmdBusyCount) > 0 }
