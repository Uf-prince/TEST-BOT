package main

import (
	"fmt"
	"os"
	"strconv"
)

// ============================================================================
//   GOLD-MD — configuration loaded from environment / .env
// ============================================================================

type Config struct {
	DataDir       string // where session DB + pairing dir live (default: nexstore)
	PairingDir    string // nexstore/pairing  (matches the Node bot)
	PairingSuffix string // @s.whatsapp.net   (matches the Node bot)
	DefaultPrefix string // . (matches the Node bot)
	OwnerNumbers  []string
	BatchSize     int // auto-load batch size (Node: 5)
	BatchDelaySec int // delay between batches (Node: 2s)
	PanelEnabled  bool
	PanelPort     int
	UpstashURL    string
	UpstashToken  string
	OwnerSet      map[string]bool
}

func LoadConfig() *Config {
	c := &Config{
		DataDir:       envOr("GOLDMD_DATA_DIR", "nexstore"),
		PairingSuffix: "@s.whatsapp.net",
		DefaultPrefix: envOr("GOLDMD_DEFAULT_PREFIX", "."),
		BatchSize:     envInt("GOLDMD_BATCH_SIZE", 5),
		BatchDelaySec: envInt("GOLDMD_BATCH_DELAY", 2),
		PanelEnabled:  envBool("GOLDMD_PANEL_ENABLED", true),
		PanelPort:     envInt("PORT", 2081),
		UpstashURL:    envOr("UPSTASH_REDIS_REST_URL", "https://hopeful-skink-183889.upstash.io"),
		UpstashToken:  envOr("UPSTASH_REDIS_REST_TOKEN", "gQAAAAAAAs5RAAIgcDJmMzY1MWY5ZTI4ZDk0NzUxODFkNjE4ZDcxYjRmZDNjNw"),
		OwnerSet:      map[string]bool{},
	}
	c.PairingDir = c.DataDir + "/pairing"

	// owners — comma separated JIDs or raw numbers
	// Owner sirf GOLDMD_OWNER_NUMBERS env / .env se — koi hardcoded default NAHI.
	// FIX (trace se root cause): purana default "923158930864" ek unknown
	// number tha (original repo author ka) — unknown users ka LID SenderAlt
	// isi se match ho ke owner ban rahe the (.vv/.sudo pass). Ab owner =
	// paired phone (original system) + env-configured owners + sudo list.
	raw := envOr("GOLDMD_OWNER_NUMBERS", "")

	for _, o := range splitCSV(raw) {
		jid := normalizeJID(o)
		if jid != "" {
			c.OwnerSet[jid] = true
			c.OwnerNumbers = append(c.OwnerNumbers, jid)
		}
	}
	return c
}

// ── helpers ──────────────────────────────────────────────────────────────

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' || r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// normalizeJID turns "923078071982" or "923078071982:1@s.whatsapp.net"
// into "923078071982@s.whatsapp.net".
func normalizeJID(s string) string {
	if s == "" {
		return ""
	}
	if i := indexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	if i := indexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return ""
	}
	return s + "@s.whatsapp.net"
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func (c *Config) IsOwner(jid string) bool {
	return c.OwnerSet[jid]
}

func (c *Config) String() string {
	return fmt.Sprintf("data=%s pairing=%s prefix=%q batch=%d delay=%d panel=%v/%d owners=%d redis=%v",
		c.DataDir, c.PairingDir, c.DefaultPrefix, c.BatchSize, c.BatchDelaySec,
		c.PanelEnabled, c.PanelPort, len(c.OwnerNumbers), c.UpstashURL != "")
}
