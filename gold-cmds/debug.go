package goldcmds

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// debugEnabled is read once per process. When GOLDMD_DEBUG=1 (or "true"/"yes"/"on")
// the command package prints verbose JSON-tagged logs. Default off — the bot stays
// silent in normal operation (keeps the server fast, no noisy output).
//
// NOTE: ALL console/debug logging has been DISABLED per owner request.
// jsonDebugPkg and jsonDebugPkgErr are now silent no-ops.
var debugEnabled = initPkgDebug()

// antideleteDebugAlwaysOn forces the antidelete/antiedit JSON debugging to
// ALWAYS print (regardless of GOLDMD_DEBUG).
// NOW: disabled — all jsonDebugPkg calls are silent no-ops.
const antideleteDebugAlwaysOn = true

func initPkgDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOLDMD_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ============================================================================
// ALL LOG FUNCTIONS ARE NO-OPS (silent) — owner requested zero console output.
// To re-enable, uncomment the fmt.Fprintf lines inside each body.
// ============================================================================

// jsonDebugPkg prints a structured JSON log line tagged with a stage label.
// NOW: silent no-op per owner request (zero console output).
func jsonDebugPkg(stage string, fields map[string]any) {
	_ = stage
	_ = fields
}

// jsonDebugPkgErr is a convenience wrapper that prints an error-tagged JSON line.
// NOW: silent no-op per owner request.
func jsonDebugPkgErr(stage string, err error, extra map[string]any) {
	_ = stage
	_ = err
	_ = extra
}

// jsonCompactPkg and jsonValPkg are kept for compatibility (no longer used by
// the no-op debug functions, but may be referenced elsewhere).
func jsonCompactPkg(m map[string]any) string {
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
		b.WriteString(jsonValPkg(v))
	}
	b.WriteByte('}')
	return b.String()
}

func jsonValPkg(v any) string {
	switch x := v.(type) {
	case string:
		return "\"" + x + "\""
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// tsPkg returns a timestamp string (kept for compatibility).
func tsPkg() string { return time.Now().Format("15:04:05") }
