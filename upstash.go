package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// GOLD-MD — Upstash Redis config + session-persistence layer (port of db.js)
//
// Mirrors the Node bot's db.js: per-session prefix, sudo list, settings,
// banned list, premium, accounts. Uses the Upstash REST API so no TCP
// redis client is needed (works on Railway / Fly / Render too).
//
// Keys are IDENTICAL to the Node bot:
//   prefix:<jid>      string
//   prefix:keys       set
//   sudo:set          set
//   banned:set        set
//   settings:<jid>    hash
//
// ADDED — session persistence (so sessions survive a server restart even
// when the disk is wiped, e.g. Railway/Fly/Render ephemeral filesystems):
//   goldmd:sessiondb:blob    string  (base64 of the whole sqlite auth file)
//   goldmd:sessiondb:jids    set     (every paired JID, so pairing folders
//                                     can be recreated after a wipe)
// ============================================================================

type Upstash struct {
	BaseURL string
	Token   string
	client  *http.Client

	// serverID namespaces the session-DB persistence keys so each
	// deployment restores only its own sessions (see resolveServerID).
	serverID string

	// in-memory cache (TTL below) — same as db.js.
	// The cache is the #1 speed optimisation: every GetSetting / GetPrefix
	// hits memory (0ms) instead of doing a synchronous HTTP round-trip to
	// Upstash (200-500ms each).
	mu    sync.Mutex
	cache map[string]cacheEntry

	// refreshCtx / refreshCancel control the background cache refresher
	// goroutine. It periodically re-fetches all cached settings from
	// Redis so the cache never goes stale even if the TTL is long. This
	// means: bot speed = instant (cache), but config changes made from
	// other sources still propagate within the refresh interval.
	refreshCtx    context.Context
	refreshCancel context.CancelFunc

	// ── Memory-guard safe re-fetch ──
	// When ClearCache() runs (RAM high), it drops all cache entries and
	// immediately triggers a background re-fetch. To prevent a race where
	// the re-fetch overwrites a config change the user just made via
	// SetSetting, we track:
	//   refetching      — true while a background re-fetch is in progress
	//   pendingUpdates  — keys the user updated DURING the re-fetch; these
	//                     are preserved (never overwritten by re-fetched values)
	refetchMu      sync.Mutex
	refetching     bool
	pendingUpdates map[string]string // cacheKey → user's fresh value

	// warmedGroups tracks which group settings hashes have been warmed
	// (one HGETALL per group per process lifetime) — see WarmGroupSettings.
	warmedGroups map[string]bool
}

// sessionDBKey returns the per-server blob key.
func (u *Upstash) sessionDBKey() string {
	return sessionDBKeyConst + u.serverID + sessionDBKeySuffix
}

// sessionJidsKey returns the per-server JID-set key.
func (u *Upstash) sessionJidsKey() string {
	return sessionJidsKeyConst + u.serverID + sessionJidsKeySuffix
}

type cacheEntry struct {
	value string
	ts    time.Time
}

// upstashCacheTTL is how long a cached entry is considered fresh.
// 3 minutes — short enough that a server restart picks up fresh config
// quickly, but long enough to avoid hammering Redis. The background
// refresher (StartCacheRefresher) refreshes entries BEFORE they expire,
// so in practice the cache is always warm and the TTL is just a safety net.
const upstashCacheTTL = 3 * time.Minute

// upstashRefreshInterval is how often the background refresher wakes up
// to re-fetch all cached settings from Redis. It runs slightly BEFORE the
// TTL expires so entries are always fresh without the bot ever waiting on
// a network call.
const upstashRefreshInterval = 2 * time.Minute

// keys used for full session-DB persistence are NAMESPACED per server so
// that deploying the same repo on a second server does NOT pull the first
// server's sessions. serverID is resolved once at startup (see resolveServerID)
// from the GOLDMD_SERVER_ID env var, falling back to the machine hostname,
// then "default". Each Upstash instance stores its own serverID and builds
// its keys from it.
//
// Examples:
//   GOLDMD_SERVER_ID=svr1 -> goldmd:sessiondb:svr1:blob / :svr1:jids
//   (no env, hostname web-abc) -> goldmd:sessiondb:web-abc:blob / :web-abc:jids
//
// To KEEP the old shared behaviour on purpose, set GOLDMD_SERVER_ID=default.

func resolveServerID() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_SERVER_ID")); v != "" {
		return v
	}
	// .env skipped on some hosts (Modal etc.) — fixed fallback so the Redis
	// session keys stay stable (goldmd:sessiondb:svr1:*) across redeploys.
	return "svr1"
}

const sessionDBKeyConst = "goldmd:sessiondb:"
const sessionJidsKeyConst = "goldmd:sessiondb:"
const sessionDBKeySuffix = ":blob"
const sessionJidsKeySuffix = ":jids"

func NewUpstash(url, token string) *Upstash {
	sid := resolveServerID()
	InfoLog("Upstash serverID resolved: %q (session keys will be namespaced by this id)", sid)
	ctx, cancel := context.WithCancel(context.Background())
	u := &Upstash{
		BaseURL:        strings.TrimRight(url, "/"),
		Token:          token,
		client:         &http.Client{Timeout: 10 * time.Second},
		serverID:       sid,
		cache:          map[string]cacheEntry{},
		refreshCtx:     ctx,
		refreshCancel:  cancel,
		pendingUpdates: map[string]string{},
	}
	// Start the background cache refresher. It silently re-fetches all
	// cached settings from Redis every upstashRefreshInterval so the cache
	// never goes stale. This keeps bot speed instant (cache hits) while
	// ensuring config changes propagate even if they were made from
	// another source (e.g. a different bot instance or direct Redis edit).
	go u.startCacheRefresher()
	return u
}

// ── low-level pipeline command ────────────────────────────────────────────
// Upstash REST: POST {url}  body = ["GET","key"]  header: Authorization Bearer <token>
func (u *Upstash) cmd(args ...string) (json.RawMessage, error) {
	body, _ := json.Marshal(args)
	req, err := http.NewRequest("POST", u.BaseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+u.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("upstash HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	// Upstash wraps result as {"result": ...}
	var wrap struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("upstash parse: %s", string(raw))
	}
	if wrap.Error != "" {
		return nil, fmt.Errorf("upstash: %s", wrap.Error)
	}
	return wrap.Result, nil
}

// ── safe wrappers (return fallback on error — like UmarSafe) ─────────────

func (u *Upstash) safeString(key, fb string) string {
	r, err := u.cmd("GET", key)
	if err != nil {
		return fb
	}
	return trimQuotes(string(r), fb)
}

func (u *Upstash) setString(key, val string) error {
	_, err := u.cmd("SET", key, val)
	return err
}

func (u *Upstash) setMembers(key string) []string {
	r, err := u.cmd("SMEMBERS", key)
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	return arr
}

func (u *Upstash) setAdd(key string, members ...string) error {
	args := append([]string{"SADD", key}, members...)
	_, err := u.cmd(args...)
	if err == nil && key == "premium:set" {
		u.invalidatePremiumCache()
		for _, m := range members {
			u.cacheDel("premium:ismember:" + m)
		}
	}
	if err == nil && key == "banned:set" {
		u.invalidateBannedCache()
		for _, m := range members {
			u.cacheDel("banned:ismember:" + m)
		}
	}
	return err
}

func (u *Upstash) setRem(key string, members ...string) error {
	args := append([]string{"SREM", key}, members...)
	_, err := u.cmd(args...)
	if err == nil && key == "premium:set" {
		u.invalidatePremiumCache()
		for _, m := range members {
			u.cacheDel("premium:ismember:" + m)
		}
	}
	if err == nil && key == "banned:set" {
		u.invalidateBannedCache()
		for _, m := range members {
			u.cacheDel("banned:ismember:" + m)
		}
	}
	return err
}

// ── cache helpers ─────────────────────────────────────────────────────────

// cacheGet returns the cached value for a key.
//
// Stale-while-refresh: if the entry exists but its TTL has expired, we
// STILL return the (stale) value so the bot never blocks on a network
// call. The background refresher (startCacheRefresher) will have already
// refreshed most entries before they expire, so stale returns are rare.
// When they do happen (e.g. right after a server restart), the stale
// value is still correct unless the config changed in the last 3 minutes
// — and even then, the next background refresh cycle picks it up.
//
// The second return value is true if the entry exists (fresh OR stale).
func (u *Upstash) cacheGet(key string) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e, ok := u.cache[key]
	if !ok {
		return "", false
	}
	// Note: we do NOT delete expired entries here. Returning stale data
	// is intentional — the background refresher handles refresh/delete.
	return e.value, true
}

// cacheGetFresh returns the value only if it is within TTL (truly fresh).
// Used by the refresher to decide which entries need re-fetching.
func (u *Upstash) cacheGetFresh(key string) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e, ok := u.cache[key]
	if !ok {
		return "", false
	}
	if time.Since(e.ts) > upstashCacheTTL {
		return "", false
	}
	return e.value, true
}

func (u *Upstash) cacheSet(key, val string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cache[key] = cacheEntry{value: val, ts: time.Now()}
}

func (u *Upstash) setDel(key string) error {
	_, err := u.cmd("DEL", key)
	return err
}

func (u *Upstash) cacheDel(key string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.cache, key)
}

// ── public API used by session/manager (mirrors db.js) ────────────────────

// UmarGetPrefix → GetPrefix
func (u *Upstash) GetPrefix(jid, def string) string {
	ck := "prefix:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v
	}
	p := u.safeString("prefix:"+jid, def)
	u.cacheSet(ck, p)
	return p
}

func (u *Upstash) SetPrefix(jid, prefix string) {
	u.cacheSet("prefix:"+jid, prefix)
	_ = u.setString("prefix:"+jid, prefix)
	_ = u.setAdd("prefix:keys", jid)
}

// UmarIsSudo → IsSudo
func (u *Upstash) IsSudo(jid string) bool {
	r, err := u.cmd("SISMEMBER", "sudo:set", jid)
	if err != nil {
		return false
	}
	return trimQuotes(string(r), "0") == "1"
}

func (u *Upstash) AddSudo(jid string)    { _ = u.setAdd("sudo:set", jid) }
func (u *Upstash) RemoveSudo(jid string) { _ = u.setRem("sudo:set", jid) }
func (u *Upstash) SudoList() []string    { return u.setMembers("sudo:set") }

// UmarIsBanned → IsBanned
// IsBanned checks whether a JID is in the bot-wide banned set.
//
// CACHED: the membership result is cached in-memory (stale-while-refresh)
// so the bot NEVER blocks on a network call for the ban check on every
// incoming message. BanUser/UnbanUser invalidate this cache instantly.
// 0% speed impact.
func (u *Upstash) IsBanned(jid string) bool {
	ck := "banned:ismember:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v == "1"
	}
	r, err := u.cmd("SISMEMBER", "banned:set", jid)
	if err != nil {
		return false
	}
	val := trimQuotes(string(r), "0")
	u.cacheSet(ck, val)
	return val == "1"
}

// BannedMembersCached returns all JIDs in the banned:set list.
//
// CACHED: the full member list is cached in-memory (stale-while-refresh)
// so the number-tolerant ban check (botBanSenderCheck) runs at 0ms instead
// of doing an SMEMBERS network round-trip on every message.
// BanUser/UnbanUser invalidate this cache instantly.
func (u *Upstash) BannedMembersCached() []string {
	ck := "banned:members"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	r, err := u.cmd("SMEMBERS", "banned:set")
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
	} else {
		if b, err := json.Marshal(arr); err == nil {
			u.cacheSet(ck, string(b))
		}
	}
	return arr
}

// CachedBannedTails returns the precomputed "number tail" (last 10 digits)
// of every JID in the bot-wide banned list, resolving each member's preferred
// number (from its goldmd:<bot>:bannedmeta:<jid> key) in ONE call per member
// on first use and caching the merged result. Ban/unban invalidate instantly
// (invalidateBannedCache). Previously the metadata lookup ran UNCACHED on
// EVERY non-owner command dispatch (~290ms per banned member).
func (u *Upstash) CachedBannedTails(botJID string) []string {
	ck := "banned:tails"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var tails []string
		if err := json.Unmarshal([]byte(v), &tails); err == nil {
			return tails
		}
	}
	jids := u.BannedMembersCached()
	if len(jids) == 0 {
		u.cacheSet(ck, "\x00")
		return nil
	}
	tails := make([]string, 0, len(jids))
	for _, jid := range jids {
		num := jid
		if i := strings.IndexByte(jid, '@'); i >= 0 {
			num = jid[:i]
		}
		// prefer stored metadata number (may be local or intl format)
		if raw := u.safeString("goldmd:"+botJID+":bannedmeta:"+jid, ""); raw != "" {
			if m := strings.Index(raw, "\"number\":"); m >= 0 {
				raw = raw[m+9:]
				if e := strings.Index(raw, "\""); e > 0 {
					raw = raw[:e]
				}
				if raw != "" {
					num = raw
				}
			}
		}
		var digits []byte
		for i := 0; i < len(num); i++ {
			if c := num[i]; c >= '0' && c <= '9' {
					digits = append(digits, c)
			}
		}
		if len(digits) == 0 {
			continue
		}
		if len(digits) > 10 {
			digits = digits[len(digits)-10:]
		}
		tails = append(tails, string(digits))
	}
	if b, err := json.Marshal(tails); err == nil {
		u.cacheSet(ck, string(b))
	}
	return tails
}

// invalidateBannedCache clears the cached banned membership/list entries.
// Called by setAdd/setRem whenever banned:set is modified.
func (u *Upstash) invalidateBannedCache() {
	u.cacheDel("banned:members")
	u.cacheDel("banned:tails")
}

func (u *Upstash) BanUser(jid string)   { _ = u.setAdd("banned:set", jid) }
func (u *Upstash) UnbanUser(jid string) { _ = u.setRem("banned:set", jid) }
func (u *Upstash) BannedList() []string { return u.setMembers("banned:set") }

// UmarIsPremium → IsPremium (premium:set)
//
// CACHED: the membership result is cached in-memory (stale-while-refresh,
// same as GetSetting) so the bot NEVER blocks on a network call for the
// premium check on every incoming message. Cache TTL is upstashCacheTTL
// and the background refresher keeps it fresh. PremiumAdd/Remove
// invalidate the per-jid cache entry instantly.
func (u *Upstash) IsPremium(jid string) bool {
	ck := "premium:ismember:" + jid
	if v, ok := u.cacheGet(ck); ok {
		return v == "1"
	}
	r, err := u.cmd("SISMEMBER", "premium:set", jid)
	if err != nil {
		return false
	}
	val := trimQuotes(string(r), "0")
	u.cacheSet(ck, val)
	return val == "1"
}

// PremiumMembersCached returns all JIDs in the premium:set list.
//
// CACHED: the full member list is cached in-memory (stale-while-refresh)
// so the number-tolerant bypass check (premiumSenderBypass) runs at 0ms
// instead of doing an SMEMBERS network round-trip on every message.
// PremiumAdd/Remove invalidate this cache instantly.
func (u *Upstash) PremiumMembersCached() []string {
	ck := "premium:members"
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	r, err := u.cmd("SMEMBERS", "premium:set")
	if err != nil {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(r, &arr); err != nil {
		return nil
	}
	if len(arr) == 0 {
		u.cacheSet(ck, "\x00")
	} else {
		if b, err := json.Marshal(arr); err == nil {
			u.cacheSet(ck, string(b))
		}
	}
	return arr
}

// invalidatePremiumCache clears the cached premium membership/list entries.
// Called by setAdd/setRem whenever premium:set is modified so changes take
// effect immediately.
func (u *Upstash) invalidatePremiumCache() {
	u.cacheDel("premium:members")
}

// settings hash: HGET settings:<jid> <field>
//
// CACHED with stale-while-refresh + background refresher.
//
// Speed: GetSetting reads from the in-memory cache (0ms). This is the #1
// speed optimisation — without it, every call does a synchronous HTTP
// round-trip to Upstash (200-500ms), and a single incoming message can
// trigger 3-5 GetSetting calls (mode, prefix, alwaysonline, autotyping,
// autorecording, antidelete, antiedit ...), adding up to ~1s of latency
// PER MESSAGE.
//
// Freshness: A background goroutine (startCacheRefresher) re-fetches all
// cached settings from Redis every upstashRefreshInterval (2 min), which
// is BEFORE the 3-min TTL expires. So the cache is always fresh without
// the bot ever waiting on a network call.
//
// Config updates: When the user changes a config via WhatsApp commands
// (.autotyping, .botpic, .alwaysonline, etc.), SetSetting is called which
// updates the cache INSTANTLY + writes to Redis. So config changes take
// effect immediately, and the TTL is refreshed (new timestamp).
//
// Server restart: The cache is empty, so the first GetSetting for each
// field does one Redis fetch, then caches it. After that, all reads are
// instant and the background refresher keeps everything fresh.
func (u *Upstash) GetSetting(jid, field, def string) string {
	ck := "settings:" + jid + ":" + field
	if v, ok := u.cacheGet(ck); ok {
		if v == "\x00" { // sentinel for "field does not exist"
			return def
		}
		return v
	}
	// Single HGET: Upstash REST returns JSON null for a MISSING field but a
	// quoted "" for a field that EXISTS with an empty-string value (live-
	// verified). One ~290ms round-trip now distinguishes both cases —
	// previously this cost HEXISTS + HGET (two calls on every cache miss).
	r, err := u.cmd("HGET", "settings:"+jid, field)
	if err != nil {
		return def
	}
	// null (field absent) => sentinel so we don't re-hit Upstash next time;
	// quoted "" (field exists) => real empty value, kept as-is.
	if strings.TrimSpace(string(r)) == "null" || len(r) == 0 {
		u.cacheSet(ck, "\x00")
		return def
	}
	val := trimQuotes(string(r), def)
	u.cacheSet(ck, val)
	return val
}

// SetSetting writes a config value to Redis AND updates the in-memory
// cache INSTANTLY. This is called when the user changes a config via
// WhatsApp commands (.autotyping on, .botpic <url>, .alwaysonline off, etc.).
// The cache update means the very next message will see the new value —
// no TTL wait, no Redis round-trip needed. The TTL timestamp is also
// refreshed (cacheSet sets ts=now), so the entry is fresh again.
func (u *Upstash) SetSetting(jid, field, val string) {
	ck := "settings:" + jid + ":" + field
	u.cacheSet(ck, val)
	// If a background re-fetch (triggered by ClearCache) is in progress,
	// record this update so the re-fetch does NOT overwrite it with the
	// stale value from Redis. The user's change always wins.
	u.refetchMu.Lock()
	if u.refetching {
		u.pendingUpdates[ck] = val
	}
	u.refetchMu.Unlock()
	_, _ = u.cmd("HSET", "settings:"+jid, field, val)
}

// DelSetting removes a field from the settings:<jid> hash (redis-safe delete).
// Used by alivemsg/ownername/ownernumber/botname reset to fully clear the
// field instead of leaving an empty string.
func (u *Upstash) DelSetting(jid, field string) {
	u.cacheDel("settings:" + jid + ":" + field)
	_, _ = u.cmd("HDEL", "settings:"+jid, field)
}

// startCacheRefresher is the background goroutine that keeps the settings
// cache fresh. Every upstashRefreshInterval it:
//  1. Snapshots all cached "settings:*" keys (under the lock, fast).
//  2. For each key, re-fetches the value from Redis (outside the lock,
//     concurrently via a small worker pool so it's fast even with many keys).
//  3. Updates the cache with the fresh value + new timestamp.
//
// This means config changes made from ANY source (WhatsApp commands,
// another bot instance, direct Redis edits) propagate to this bot's
// cache within upstashRefreshInterval — without the bot ever waiting on
// a network call during message processing.
//
// The bot speed is unaffected because GetSetting still reads from cache
// (0ms). The refresher runs in the background and never blocks the
// event handler.
func (u *Upstash) startCacheRefresher() {
	ticker := time.NewTicker(upstashRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-u.refreshCtx.Done():
			return
		case <-ticker.C:
			u.refreshAllSettings()
		}
	}
}

// refreshAllSettings re-fetches all cached settings entries from Redis
// and updates the cache. It runs in the background and is best-effort:
// if a Redis call fails, the existing (possibly stale) cached value is
// kept rather than deleted, so the bot keeps working.
func (u *Upstash) refreshAllSettings() {
	// If a ClearCache-triggered re-fetch is in progress, skip this periodic
	// refresh to avoid a double-fetch race. The re-fetch will repopulate
	// everything, and the next periodic tick will run normally.
	u.refetchMu.Lock()
	busy := u.refetching
	u.refetchMu.Unlock()
	if busy {
		return
	}

	// Snapshot all settings keys (under lock, fast).
	u.mu.Lock()
	var keys []string
	for k := range u.cache {
		if strings.HasPrefix(k, "settings:") {
			keys = append(keys, k)
		}
	}
	u.mu.Unlock()

	if len(keys) == 0 {
		return
	}

	// Refresh each key. We parse "settings:<jid>:<field>" back into jid
	// and field, then do an HGET. This is done WITHOUT holding the cache
	// lock so message processing is never blocked.
	for _, ck := range keys {
		// ck = "settings:<jid>:<field>"
		// Strip "settings:" prefix, then split on first ":"
		rest := strings.TrimPrefix(ck, "settings:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			continue
		}
		jid := rest[:idx]
		field := rest[idx+1:]

		// Single HGET: JSON null => missing field (sentinel); quoted "" =>
		// real empty-string value (live-verified Upstash REST semantics).
		r, err := u.cmd("HGET", "settings:"+jid, field)
		if err != nil {
			// Keep the existing cached value on error (best-effort).
			continue
		}
		if strings.TrimSpace(string(r)) == "null" || len(r) == 0 {
			// Field genuinely does not exist in Redis — cache sentinel so
		// GetSetting returns the default without re-hitting Upstash.
			u.cacheSet(ck, "\x00")
			continue
		}
		val := trimQuotes(string(r), "")
		u.cacheSet(ck, val)
	}
}

// StopCacheRefresher stops the background cache refresher goroutine.
// Called on graceful shutdown.
func (u *Upstash) StopCacheRefresher() {
	if u.refreshCancel != nil {
		u.refreshCancel()
	}
}

// ClearCache drops all cached entries to free memory, then immediately
// triggers a background re-fetch so the cache is repopulated from Redis
// WITHOUT the bot ever blocking on a network call.
//
// Called by the memory watchdog when container RAM crosses 450 MB.
//
// Race-safety: While the re-fetch is running, any SetSetting call (user
// changing config via WhatsApp) is recorded in pendingUpdates. After the
// re-fetch completes, those pending values are written back to the cache,
// so the user's config change is NEVER overwritten by the stale Redis
// value. The user always sees their latest change.
func (u *Upstash) ClearCache() {
	// 1. Snapshot all settings keys BEFORE clearing (so we know what to re-fetch).
	u.mu.Lock()
	var keys []string
	for k := range u.cache {
		if strings.HasPrefix(k, "settings:") {
			keys = append(keys, k)
		}
	}
	n := len(u.cache)
	u.cache = map[string]cacheEntry{}
	// Drop the group warmup markers too: the cache is empty now, so the
	// next message from each group triggers ONE fresh background HGETALL
	// (prevents "marked warmed but nothing cached" after a cache clear).
	u.warmedGroups = nil
	u.mu.Unlock()

	if n > 0 {
		InfoLog("Upstash cache cleared (%d entries, %d settings keys) — re-fetching in background", n, len(keys))
	}

	// 2. Mark re-fetch in progress + clear any stale pending updates.
	u.refetchMu.Lock()
	u.refetching = true
	u.pendingUpdates = map[string]string{}
	u.refetchMu.Unlock()

	// 3. Launch background re-fetch (non-blocking — runs in its own goroutine).
	go u.refetchAfterClear(keys)
}

// refetchAfterClear re-fetches all previously-cached settings keys from
// Redis and re-populates the cache. Runs in a background goroutine so the
// memory watchdog (and bot message processing) never blocks.
//
// After re-fetching, any keys the user updated DURING the re-fetch
// (tracked in pendingUpdates) are written back with the user's value,
// so config changes made while clearing are preserved.
func (u *Upstash) refetchAfterClear(keys []string) {
	// Re-fetch each key from Redis (best-effort — keep going on error).
	for _, ck := range keys {
		// ck = "settings:<jid>:<field>"
		rest := strings.TrimPrefix(ck, "settings:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			continue
		}
		jid := rest[:idx]
		field := rest[idx+1:]

		// HEXISTS-first: preserve empty-string values (e.g. autoreactemojis="",
		// welcomemsg="") that .autoreact reset / .welcome reset intentionally store.
		he, err := u.cmd("HEXISTS", "settings:"+jid, field)
		if err != nil {
			continue // best-effort: skip on error, GetSetting will lazy-fill later
		}
		if strings.TrimSpace(string(he)) == "0" {
			u.cacheSet(ck, "\x00") // sentinel for missing field
			continue
		}
		r, err := u.cmd("HGET", "settings:"+jid, field)
		if err != nil {
			continue
		}
		u.cacheSet(ck, trimQuotes(string(r), ""))
	}

	// 4. Apply pending updates (user's config changes made DURING re-fetch).
	//    These WIN over the re-fetched values — the user's change is preserved.
	u.refetchMu.Lock()
	pending := u.pendingUpdates
	u.pendingUpdates = map[string]string{}
	u.refetching = false
	u.refetchMu.Unlock()

	for ck, val := range pending {
		u.cacheSet(ck, val)
	}

	if len(pending) > 0 {
		InfoLog("Re-fetch complete — %d user config updates preserved", len(pending))
	} else {
		InfoLog("Re-fetch complete — cache repopulated (%d keys)", len(keys))
	}
}

// CacheLen returns the number of cached entries (for diagnostics).
func (u *Upstash) CacheLen() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.cache)
}

// WarmCache loads banned set into memory for fast lookups (UmarLoadBannedToGlobal)
func (u *Upstash) WarmCache() {
	banned := u.BannedList()
	u.mu.Lock()
	for _, b := range banned {
		u.cache["banned:"+b] = cacheEntry{value: "1", ts: time.Now()}
	}
	u.mu.Unlock()
	OkLog("Upstash cache warmed: %d banned entries", len(banned))
}

// PreloadSettings loads ALL fields of the settings:<jid> hash into the
// in-memory cache in a SINGLE HGETALL round-trip. This is called when a
// WhatsApp session connects (bot JID) so that every config value
// (.autoreact, .ownerreact, .welcome, .goodbye, .typing, .statusseen, etc.)
// is immediately available in cache with zero latency on the first message.
//
// Without this, the first access to each setting triggers a separate HGET
// (or HEXISTS+HGET) round-trip to Upstash. Preloading avoids that cold-start
// penalty and ensures config is consistent right after boot / restart.
//
// Uses HGETALL which returns ["field1","val1","field2","val2", ...]. We
// cache each field under "settings:<jid>:<field>". Empty-string values
// (e.g. welcomemsg="" after .welcome reset) are cached as "" (NOT sentinel)
// because HGETALL only returns fields that EXIST — so "" means the field
// is genuinely set to empty, which is a valid stored value.
// WarmGroupSettings warms a group's entire settings hash into cache in
// ONE background HGETALL round-trip (first message from that group only).
// It is fire-and-forget and NEVER blocks the reply path: handler.go calls it
// before dispatch, and by the time the group's anti-features / bangc /
// welcome settings are read, most fields are already cached at 0ms.
// The warmedGroups map ensures each group pays exactly ONE HGETALL for its
// lifetime of this process (restarts re-warm, config changes update via
// SetGroupSetting directly which writes cache too).
func (u *Upstash) WarmGroupSettings(groupJID string) {
	if u.warmedGroups == nil {
		u.mu.Lock()
		if u.warmedGroups == nil {
			u.warmedGroups = map[string]bool{}
		}
		u.mu.Unlock()
	}
	u.mu.Lock()
	_, seen := u.warmedGroups[groupJID]
	u.mu.Unlock()
	if seen {
		return
	}
	u.mu.Lock()
	u.warmedGroups[groupJID] = true
	u.mu.Unlock()
	go func() {
		defer func() { _ = recover() }()
		r, err := u.cmd("HGETALL", "settings:"+groupJID)
		if err != nil {
			return
		}
		var pairs []string
		if err := json.Unmarshal(r, &pairs); err != nil || len(pairs)%2 != 0 {
			return
		}
		for i := 0; i < len(pairs); i += 2 {
			u.cacheSet("settings:"+groupJID+":"+pairs[i], pairs[i+1])
		}
	}()
}

func (u *Upstash) PreloadSettings(jid, defPrefix string) {
	// ── 1. Bot settings hash → cache (single HGETALL round-trip). ──
	// A missing/nonexistent hash returns a JSON null result — handle it
	// silently (fresh pairing: nothing stored yet, nothing to warm).
	r, err := u.cmd("HGETALL", "settings:"+jid)
	if err == nil {
		var pairs []string
		if errJ := json.Unmarshal(r, &pairs); errJ == nil && len(pairs)%2 == 0 {
			count := 0
			for i := 0; i < len(pairs); i += 2 {
				field := pairs[i]
				val := pairs[i+1]
				u.cacheSet("settings:"+jid+":"+field, val)
				count++
			}
			if count > 0 {
				OkLog("Preloaded %d settings for %s", count, jid)
			}
		}
	}

	// ── 2. Hot-path fields: default-sentinel any NOT present in the hash. ──
	// These are read on EVERY message/command dispatch (handler.go). If the
	// owner never set them, the hash has no entry — without this warmup the
	// FIRST message after (re)connect would pay a ~290ms HGET miss each.
	// Sentinel \x00 = "field does not exist → use default" (same semantics
	// as GetSetting miss path, now arrived at instantly).
	for _, f := range []string{"mode", "sudowners", "botname", "ownername", "ownernumber", "botpic"} {
		ck := "settings:" + jid + ":" + f
		if _, ok := u.cacheGet(ck); !ok {
			u.cacheSet(ck, "\x00")
		}
	}

	// ── 3. Prefix (own Redis key, separate from the settings hash). ──
	// GetPrefix(jid, def) checks the cache first — warming it here means the
	// first command after (re)connect resolves its prefix at 0ms instead of
	// a ~290ms REST GET.
	ckp := "prefix:" + jid
	if _, ok := u.cacheGet(ckp); !ok {
		u.cacheSet(ckp, u.safeString("prefix:"+jid, defPrefix))
	}
}

// Ping tests connectivity (UmarConnectDB)
func (u *Upstash) Ping() bool {
	_, err := u.cmd("PING")
	if err != nil {
		ErrLog("Upstash ping failed: %v", err)
		return false
	}
	InfoLog("Upstash Redis health check passed")
	return true
}

// ── util ─────────────────────────────────────────────────────────────────
// Upstash returns quoted JSON strings; trim them.
func trimQuotes(s, fb string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return fb
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ============================================================================
// ── SESSION PERSISTENCE (NEW) ──────────────────────────────────────────────
//
// whatsmeow keeps every session's auth material (device identity, signal
// session keys, prekeys, etc.) inside ONE shared sqlite file
// ("goldmd.db" in cfg.DataDir). On ephemeral hosts that file disappears on
// every restart/redeploy, which is why sessions had to be re-paired.
//
// Fix: back the whole file up to Upstash (base64 string) whenever a session
// connects/pairs, and restore it from Upstash BEFORE the sqlite container
// is opened on the next boot. We also keep a JID set so AutoLoad() can
// recreate the pairing marker folders it depends on.
// ============================================================================

// SaveSessionDB checkpoints SQLite before uploading the auth database. This is
// important when SQLite is using WAL mode: recent credentials may otherwise be
// left in the -wal sidecar and omitted from the Redis snapshot.
func (u *Upstash) SaveSessionDB(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := checkpointSQLite(path); err != nil {
		return fmt.Errorf("checkpoint session database: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	//	JSONDebug("REDIS_SAVE_BLOB", map[string]any{
	//		"path":       path,
	//		"rawBytes":   len(data),
	//		"encodedLen": len(encoded),
	//	})
	if err := u.setString(u.sessionDBKey(), encoded); err != nil {
		return fmt.Errorf("save session blob: %w", err)
	}
	//	JSONDebug("REDIS_SAVE_BLOB_OK", map[string]any{"key": u.sessionDBKey()})
	return nil
}

func checkpointSQLite(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// mode=rwc = read-write-create (never falls back to readonly).
	// _txlock=immediate avoids the WAL/lock race that previously left the
	// DB in a "readonly database" state after a TRUNCATE checkpoint while
	// the main whatsmeow container had the file open.
	db, err := sql.Open("sqlite3", "file:"+absPath+"?mode=rwc&_busy_timeout=10000&_txlock=immediate")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// PASSIVE checkpoint never blocks writers — it only merges the WAL
	// if no one is actively writing. Safe to run while the bot's main
	// connection is live. (TRUNCATE was forcing a full lock that sometimes
	// collided with an in-flight device-store write and left the DB flagged
	// readonly, breaking pairing.)
	_, err = db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	return err
}

// RestoreSessionDB writes the Redis-stored sqlite file back to disk.
// Returns (true, nil) if a backup was found and restored.
func (u *Upstash) RestoreSessionDB(path string) (bool, error) {
	r, err := u.cmd("GET", u.sessionDBKey())
	if err != nil {
		return false, err
	}
	encoded := trimQuotes(string(r), "")
	if encoded == "" {
		//		JSONDebug("REDIS_RESTORE_BLOB_EMPTY", map[string]any{"key": u.sessionDBKey()})
		return false, nil // nothing backed up yet
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	//	JSONDebug("REDIS_RESTORE_BLOB_OK", map[string]any{
	//		"path":     path,
	//		"rawBytes": len(data),
	//	})
	return true, nil
}

// RegisterJID remembers a paired JID so its pairing folder can be recreated
// after a disk wipe (AutoLoad scans folders under cfg.PairingDir).
func (u *Upstash) RegisterJID(jid string) error {
	if err := u.setAdd(u.sessionJidsKey(), jid); err != nil {
		return err
	}
	return nil
}

// ListJIDs returns every JID ever registered.
func (u *Upstash) ListJIDs() []string { return u.setMembers(u.sessionJidsKey()) }

// HasJID reports whether Redis has a registered session for the base JID.
func (u *Upstash) HasJID(jid string) bool {
	r, err := u.cmd("SISMEMBER", u.sessionJidsKey(), jid)
	return err == nil && trimQuotes(string(r), "0") == "1"
}

// RemoveJID drops a JID from the session registry. Called when a session
// logs out (or its linked device is manually removed from the phone) so
// the next SaveSessionDB / AutoLoad no longer tries to restore it.
// This ONLY touches the session registry — prefix/sudo/banned/settings
// config keys for that jid are left completely untouched.
func (u *Upstash) RemoveJID(jid string) error {
	return u.setRem(u.sessionJidsKey(), jid)
}

// DelSessionDB removes the session DB blob from Redis. This is used by
// cleanupSession when the LAST paired device is removed — instead of
// saving an empty DB (which would create a stale-empty-blob that gets
// "restored" on the next restart), we delete the blob entirely so the
// next boot sees REDIS_RESTORE_BLOB_EMPTY and starts truly fresh.
func (u *Upstash) DelSessionDB() error {
	return u.setDel(u.sessionDBKey())
}
