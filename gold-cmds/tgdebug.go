package goldcmds

// ============================================================================
// GOLD-MD — TG debug (disabled)
// Owner rule: JSON debug engine OFF — zero console/file output.
// ============================================================================

// tgDebug — no-op (debug disabled).
func tgDebug(stage string, fields map[string]any) {}

// tgDebugErr — no-op (debug disabled).
func tgDebugErr(stage string, err error, fields map[string]any) {}
