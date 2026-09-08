package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	cReset   = "\x1b[0m"
	cRed     = "\x1b[31m"
	cGreen   = "\x1b[32m"
	cCyan    = "\x1b[36m"
	cGray    = "\x1b[90m"
	cBold    = "\x1b[1m"
	cYellow  = "\x1b[33m"
	cMagenta = "\x1b[35m"
)

func ts() string { return time.Now().Format("15:04:05") }

// debugEnabled is read once at startup. When GOLDMD_DEBUG=1 (or "true"/"yes")
// the bot prints verbose JSON-tagged logs so the full pairing / reconnect /
// Redis-save / Redis-restore flow is visible in the console. Once everything
// is confirmed working, simply leave GOLDMD_DEBUG unset and the bot goes quiet
// again (same behavior as the original code).
//
// NOTE: ALL console/debug logging has been DISABLED per owner request.
// Every log function below is now a no-op (silent). To re-enable, restore
// the original fmt.Fprintf bodies. (GOLDMD_DEBUG env var no longer has any
// effect because all functions return immediately.)
var debugEnabled = initDebug()

func initDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOLDMD_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ============================================================================
// ALL LOG FUNCTIONS ARE NO-OPS (silent) — owner requested zero console output.
// To re-enable any of these, uncomment the fmt.Fprintf lines inside each body.
// ============================================================================

func errLine(tag, msg string, args ...any) {
	// DISABLED — no console output
	_ = tag
	_ = msg
	_ = args
}

func infoLine(tag, color, msg string, args ...any) {
	// DISABLED — no console output
	_ = tag
	_ = color
	_ = msg
	_ = args
}

// When GOLDMD_DEBUG is set, these print. Otherwise they are silent (original behavior).
// NOW: always silent (no-op) per owner request.
func InfoLog(msg string, args ...any)  { _ = msg; _ = args }
func WarnLog(msg string, args ...any)  { _ = msg; _ = args }
func OkLog(msg string, args ...any)    { _ = msg; _ = args }
func DebugLog(msg string, args ...any) { _ = msg; _ = args }

func ErrLog(msg string, args ...any) { _ = msg; _ = args }

func FatalLog(msg string, args ...any) {
	// DISABLED — no console output, but still exit on fatal
	_ = msg
	_ = args
	os.Exit(1)
}

// antiDebugAlwaysOn forces the antidelete/antiedit JSON debugging in the MAIN
// package to ALWAYS print (regardless of GOLDMD_DEBUG).
// NOW: disabled — all JSONDebug calls are silent no-ops.
const antiDebugAlwaysOn = true

// isAntiStage reports whether a JSONDebug stage belongs to the
// antidelete/antiedit/antistatus subsystem (so it should always be printed).
// NOW: always returns false so JSONDebug never prints.
func isAntiStage(stage string) bool {
	_ = stage
	return false
}

// JSONDebug prints a structured JSON log line tagged with a stage label.
// NOW: silent no-op per owner request (zero console output).
func JSONDebug(stage string, fields map[string]any) {
	_ = stage
	_ = fields
}

// JSONDebugErr is a convenience wrapper that prints an error-tagged JSON line.
// NOW: silent no-op per owner request.
func JSONDebugErr(stage string, err error, extra map[string]any) {
	_ = stage
	_ = err
	_ = extra
}

func jsonCompact(m map[string]any) string {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for k, v := range m {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteByte('"')
		b.WriteString(k)
		b.WriteString("\": ")
		b.WriteString(jsonVal(v))
	}
	b.WriteByte('}')
	return b.String()
}

func jsonVal(v any) string {
	switch x := v.(type) {
	case string:
		return "\"" + x + "\""
	case fmt.Stringer:
		return "\"" + x.String() + "\""
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}
