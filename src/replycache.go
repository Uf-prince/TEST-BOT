package main

// ═══════════════════════════════════════════════════════════════════════════
//   PERSISTENT TRANSLATION CACHE  (owner order — bot speed)
//
//   MASLA: .botlanguage set hone ke baad HAR reply live translate hoti thi —
//   har menu, har alert, har jawab = ek HTTP round-trip. Bot slow.
//
//   HAL: ek baar translate, phir cache. Teen layer:
//     1. RAM   — same process, 0ms map lookup (hot path).
//     2. DISK  — nexstore/replycache (0 network; restart ke baad bhi zinda).
//     3. STORJ — per-bot setting field "replycache:<lang>" (disk wipe hone par
//                bhi wapas mil jata hai).
//
//   Lookup RAM → DISK → STORJ. Sirf miss translator tak jata hai, aur nateeja
//   teeno layer me likh diya jata hai. Restart pe disk se, disk khali ho to
//   Storj se — translator dobara nahi chalta.
//
//   Cache LINE-level hai (poora reply nahi): menus har minute badalte hain
//   (uptime/counts), magar unki header/description/guidance lines har menu aur
//   har user me repeat hoti hain. Command-token wali lines already verbatim
//   rehti hain (menutrans.go), to unka translate hi nahi hota.
// ═══════════════════════════════════════════════════════════════════════════

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	goldcmds "gold-md/gold-cmds"
)

var (
	rcDir     = envOr("GOLDMD_REPLY_CACHE_DIR", "nexstore/replycache")
	rcDirOnce sync.Once
	rcReady   bool

	rcMu  sync.Mutex
	rcRAM = map[string]string{} // "<jid>\x00<lang>\x00<sha>" -> translation
	rcHot = map[string]bool{}   // "<jid>\x00<lang>" already pulled from disk/Storj

	rcPendingMu sync.Mutex
	rcPending   = map[string]bool{}
	rcRedis     = map[string]*Upstash{} // botJID -> Redis, for the Storj mirror
)

// rcMaxFieldBytes caps the Storj mirror so a huge cache never turns a setting
// write into a multi-megabyte upload. The disk layer always keeps everything.
const rcMaxFieldBytes = 900_000

// init attaches the cache to the translation layer, so every reply path uses it
// without the caller having to remember (main() also wires it; init covers tests
// and any future entry point).
func init() {
	goldcmds.TrtCacheAttach(
		func(botJID, lang, line string) (string, bool) { return rcGet(botJID, lang, line) },
		func(botJID, lang, line, out string) { rcPut(botJID, lang, line, out) },
	)
}

func rcInit() {
	rcDirOnce.Do(func() {
		if err := os.MkdirAll(rcDir, 0o755); err != nil {
			ErrLog("REPLY-CACHE: mkdir %s failed: %v — disk layer off", rcDir, err)
			return
		}
		rcReady = true
		InfoLog("REPLY-CACHE: ready (dir=%s)", rcDir)
	})
}

// rcVersion tags every cache key. Bump it whenever the translation PIPELINE
// changes (e.g. the caps-softening and prefix-sentinel fixes), because entries
// written by an older pipeline are wrong and must not be served again — the old
// keys are simply never looked up, so stale translations cannot resurface.
const rcVersion = "v2"

func rcHash(line string) string {
	h := sha256.Sum256([]byte(rcVersion + "\x00" + line))
	return hex.EncodeToString(h[:])
}

func rcFile(botJID, lang string) string {
	h := sha256.Sum256([]byte(rcVersion + "\x00" + botJID + "\x00" + lang))
	return filepath.Join(rcDir, hex.EncodeToString(h[:])+".json")
}

func rcField(lang string) string {
	return "replycache:" + rcVersion + ":" + strings.ToLower(strings.TrimSpace(lang))
}

// rcBind records which Redis handle serves a bot, so the background flush can
// mirror to Storj without threading a *Session through it.
func rcBind(botJID string, u *Upstash) {
	if botJID == "" || u == nil {
		return
	}
	rcPendingMu.Lock()
	rcRedis[botJID] = u
	rcPendingMu.Unlock()
}

func rcRedisFor(botJID string) *Upstash {
	rcPendingMu.Lock()
	u := rcRedis[botJID]
	rcPendingMu.Unlock()
	return u
}

// rcWarm pulls this bot+language cache into RAM, once per process. Disk first;
// when the disk copy is missing (fresh container / wiped nexstore) it asks Storj
// (owner order: "disk me data na mile fir setobra storage se data mangwa lo").
func rcWarm(botJID, lang string) {
	rcInit()
	if !rcReady || botJID == "" || lang == "" {
		return
	}
	id := botJID + "\x00" + lang
	rcMu.Lock()
	if rcHot[id] {
		rcMu.Unlock()
		return
	}
	rcHot[id] = true
	rcMu.Unlock()

	m := map[string]string{}
	loaded := false

	if b, err := os.ReadFile(rcFile(botJID, lang)); err == nil {
		if json.Unmarshal(b, &m) == nil && len(m) > 0 {
			loaded = true
		}
	}
	if !loaded {
		if u := rcRedisFor(botJID); u != nil {
			if raw := strings.TrimSpace(u.GetSetting(botJID, rcField(lang), "")); raw != "" {
				if json.Unmarshal([]byte(raw), &m) == nil && len(m) > 0 {
					loaded = true
					// Disk khali tha — Storj se mila, to disk pe bhi likh do.
					if b, err := json.Marshal(m); err == nil {
						p := rcFile(botJID, lang)
						if os.WriteFile(p+".tmp", b, 0o644) == nil {
							_ = os.Rename(p+".tmp", p)
						}
					}
				}
			}
		}
	}
	if !loaded {
		return
	}
	rcMu.Lock()
	for k, v := range m {
		rcRAM[id+"\x00"+k] = v
	}
	rcMu.Unlock()
	InfoLog("REPLY-CACHE: warmed %d lines for %s (%s)", len(m), botJID, lang)
}

// rcGet is the hot-path lookup: RAM only (0 network, 0 disk).
func rcGet(botJID, lang, line string) (string, bool) {
	if botJID == "" || lang == "" {
		return "", false
	}
	rcWarm(botJID, lang)
	rcMu.Lock()
	v, ok := rcRAM[botJID+"\x00"+lang+"\x00"+rcHash(line)]
	rcMu.Unlock()
	return v, ok
}

// rcPut stores one translated line and schedules a batched persist.
func rcPut(botJID, lang, line, out string) {
	if botJID == "" || lang == "" || strings.TrimSpace(out) == "" || out == line {
		return
	}
	rcWarm(botJID, lang)
	id := botJID + "\x00" + lang
	rcMu.Lock()
	rcRAM[id+"\x00"+rcHash(line)] = out
	rcMu.Unlock()
	rcScheduleFlush(id)
}

// rcScheduleFlush coalesces writes: one flush per bot+language per second, so a
// menu burst writes the cache file once instead of once per line.
func rcScheduleFlush(id string) {
	rcPendingMu.Lock()
	if rcPending[id] {
		rcPendingMu.Unlock()
		return
	}
	rcPending[id] = true
	rcPendingMu.Unlock()

	go func() {
		time.Sleep(time.Second)
		rcPendingMu.Lock()
		delete(rcPending, id)
		rcPendingMu.Unlock()
		rcFlush(id)
	}()
}

// rcFlush writes one bot+language slice to disk, then mirrors it to Storj.
func rcFlush(id string) {
	parts := strings.SplitN(id, "\x00", 2)
	if len(parts) != 2 {
		return
	}
	botJID, lang := parts[0], parts[1]
	prefix := id + "\x00"

	rcMu.Lock()
	m := map[string]string{}
	for k, v := range rcRAM {
		if strings.HasPrefix(k, prefix) {
			m[strings.TrimPrefix(k, prefix)] = v
		}
	}
	rcMu.Unlock()
	if len(m) == 0 {
		return
	}

	if rcReady {
		if b, err := json.Marshal(m); err == nil {
			p := rcFile(botJID, lang)
			if os.WriteFile(p+".tmp", b, 0o644) == nil {
				_ = os.Rename(p+".tmp", p)
			}
		}
	}
	u := rcRedisFor(botJID)
	if u == nil {
		return
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return
	}
	if len(payload) > rcMaxFieldBytes {
		if payload, err = json.Marshal(rcTrim(m, rcMaxFieldBytes)); err != nil {
			return
		}
	}
	u.SetSetting(botJID, rcField(lang), string(payload))
}

// rcTrim keeps the shortest lines (static one-liners) when the Storj mirror has
// to be capped. Disk keeps everything regardless.
func rcTrim(m map[string]string, max int) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(m[keys[i]]) < len(m[keys[j]]) })
	out := map[string]string{}
	total := 2
	for _, k := range keys {
		cost := len(k) + len(m[k]) + 6
		if total+cost > max {
			continue
		}
		out[k] = m[k]
		total += cost
	}
	return out
}

// rcCount reports how many lines are cached for a bot+language.
func rcCount(botJID, lang string) int {
	if botJID == "" || lang == "" {
		return 0
	}
	rcWarm(botJID, lang)
	prefix := botJID + "\x00" + lang + "\x00"
	rcMu.Lock()
	n := 0
	for k := range rcRAM {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	rcMu.Unlock()
	return n
}
