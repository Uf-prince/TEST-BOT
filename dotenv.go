package main

import (
	"bufio"
	"os"
	"strings"
)

// ============================================================================
// GOLD-MD — minimal .env loader (stdlib only, no new go.mod dependency)
//
// Reads a .env file (if present) from the working directory and sets any
// KEY=VALUE pairs found as real process environment variables — but only
// for keys that aren't already set (so real env vars, e.g. on Railway,
// always take priority over the .env file).
//
// Supported line formats:
//   UPSTASH_REDIS_REST_URL=https://your-db.upstash.io
//   UPSTASH_REDIS_REST_TOKEN="your-token-here"
//   # comments and blank lines are ignored
// ============================================================================

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file — that's fine, just use real env vars
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		// strip surrounding quotes if present
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// don't override a real environment variable that's already set
		if _, exists := os.LookupEnv(key); !exists && key != "" {
			_ = os.Setenv(key, val)
		}
	}
}
