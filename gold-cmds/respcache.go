package goldcmds

// ============================================================================
// GOLD-MD — RAM Response Cache (owner order)
// File: respcache.go
// ============================================================================
// OWNER ORDER: "iske cmnds k responce wghera RAM me rakhwana ... take user
// akela cmnd likhe to info guidance msg foran pohnch jaye" — command
// responses / guidance text must live in RAM so a lone command gets its
// info/guidance message instantly, while heavy work (downloads etc.) stays
// on disk.
//
// This module is a tiny, lock-guarded RAM cache for RENDERED response text.
// Guide builders are pure functions of (prefix) — so caching their output
// keyed by (name, prefix) means every repeat call is a single map lookup
// (0 string building, 0 storage round-trip, 0 network).
//
// It is deliberately generic: any command can call CachedGuide / respCacheGet
// / respCacheSet. Nothing here touches disk or Storj — it is pure RAM, so it
// adds 0% load to the bot's speed (the guard's own work is off the hot path).
// ============================================================================

import (
	"sync"
	"time"
)

type respEntry struct {
	text string
	at   time.Time
}

var (
	respMu    sync.RWMutex
	respCache = make(map[string]respEntry)
)

// respTTL bounds how long a cached response stays valid. Guides are static
// text, so a long TTL is safe; the entry is simply rebuilt on the next call
// after expiry.
const respTTL = 30 * time.Minute

// respCacheGet returns the cached text for key if present and fresh.
func respCacheGet(key string) (string, bool) {
	respMu.RLock()
	e, ok := respCache[key]
	respMu.RUnlock()
	if !ok || time.Since(e.at) > respTTL {
		return "", false
	}
	return e.text, true
}

// respCacheSet stores text under key in RAM.
func respCacheSet(key, text string) {
	respMu.Lock()
	respCache[key] = respEntry{text: text, at: time.Now()}
	respMu.Unlock()
}

// respCacheLen returns the number of cached responses (for status/logging).
func respCacheLen() int {
	respMu.RLock()
	n := len(respCache)
	respMu.RUnlock()
	return n
}

// CachedGuide returns the cached guidance text for (name, prefix), building it
// once via build(prefix) and keeping it in RAM. Every subsequent call is a
// single map lookup — the guidance message is therefore delivered instantly.
//
// build MUST be a pure function of prefix (no side effects, no storage reads),
// which is true for every *Guide(prefix) helper in this package.
func CachedGuide(name, prefix string, build func(string) string) string {
	key := name + "|" + prefix
	if v, ok := respCacheGet(key); ok {
		return v
	}
	v := build(prefix)
	respCacheSet(key, v)
	return v
}

// RespCacheWarm pre-builds the most common lone-command guidance responses for
// the given prefix so the very first user request is already served from RAM.
// Safe to call at boot and whenever the prefix changes.
func RespCacheWarm(prefix string) {
	if prefix == "" {
		prefix = "."
	}
	// antidelete / antiedit guides (owner explicitly named these).
	CachedGuide("antidelete", prefix, antideleteGuide)
	CachedGuide("antiedit", prefix, antieditGuide)
	// Common info guides.
	CachedGuide("fb", prefix, func(string) string { return fbHelpText })
	CachedGuide("ig", prefix, func(string) string { return instaHelpText })
	CachedGuide("apk", prefix, func(string) string { return apkHelpText })
}
