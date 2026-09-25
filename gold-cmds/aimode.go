package goldcmds

// ============================================================================
// GOLD-MD — .aimode (AI natural-language command resolver)
//
// Ported from Node.js pair.js — SAME WORK (0% farak), SAME TEXT (0% farak).
// Crown emoji in Node.js is replaced with 🔰 in GOLD-MD (owner order).
//
// .aimode lets the bot understand plain everyday sentences (no command prefix)
// and auto-run the matching command. The AI is only a TRANSLATOR — the resolved
// command goes through the exact same dispatch as a typed ".command", so every
// permission check (owner-only, group-admin, mode) still applies. No bypass.
//
// Redis (settings:<botJID> hash, same pattern as autoreply/settings):
//   field "aimode"       = "on"/"off" (default off)
//   field "aimodeprefix" = wake-word (default AI)
//
// MODEL: same working model as .autoreply (owner order) — ministral-8b-latest
// with open-mistral-nemo fallback.
// KEYS (owner order — hardcoded in-file, NO .env): GOLD_API_KEY_1..30 env values
// win when present; otherwise the in-file pool below is used.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	aimModeField      = "aimode"       // Redis field: on/off
	aimPrefixField    = "aimodeprefix" // Redis field: wake-word
	aimResolveTimeout = 30 * time.Second
	aimResolveMaxTok  = 512
	aimMaxKeys        = 30
)

// aimDefaultPrefixWord is the wake-word a fresh bot uses when the owner never
// ran ".aimode prefix <word>" (pair.js AIMODE_DEFAULT_PREFIX_WORD). It keeps
// plain chatter out of the resolver and stops bare command names from working
// without the bot prefix — the owner must type "AI check bot speed".
const aimDefaultPrefixWord = "AI"

// aimModels — same working model as .autoreply (owner order).
var aimModels = []string{arModel, arModelFallbacks}

// goldAIModeKeys — in-file key pool. GOLD_API_KEY_1..N env values override the
// same index when present; the three GOLD keys already working in this repo are
// the seed. Paste GOLD_API_KEY_4..30 here to grow the pool.
var goldAIModeKeys = []string{
	"eC9Sa6R0MZjTb28Ui6tvbZZuh002Av6M",
	"TB8IwNBTtWDg2wwm43P71s2Id8secHmT",
	"XptUKsqj8y6io1X0HKmiIcTgujfjwH8A",
}

var (
	aimKeyMu         sync.Mutex
	aimKeyPtr        int
	aimCooldownUntil = map[int]time.Time{}
)

// aimKeyAt resolves key i: GOLD_API_KEY_<i+1> env wins, else the in-file pool.
func aimKeyAt(i int) string {
	if k := strings.TrimSpace(os.Getenv(fmt.Sprintf("GOLD_API_KEY_%d", i+1))); k != "" {
		return k
	}
	if i < len(goldAIModeKeys) {
		return goldAIModeKeys[i]
	}
	return ""
}

// aimCallOne makes ONE API call on ONE key/model. retrySeconds > 0 = HTTP 429.
func aimCallOne(apiKey, model, system, user string) (string, int, error) {
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"max_tokens": aimResolveMaxTok,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), aimResolveTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", arBaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: aimResolveTimeout}).Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return "", arParseRetrySeconds(resp.Header.Get("Retry-After")), fmt.Errorf("HTTP 429 (aimode)")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return "", 0, fmt.Errorf("HTTP %d (aimode) — %s", resp.StatusCode, string(raw))
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", 0, err
	}
	if len(data.Choices) == 0 {
		return "", 0, fmt.Errorf("empty response (aimode)")
	}
	text := strings.TrimSpace(data.Choices[0].Message.Content)
	if text == "" {
		return "", 0, fmt.Errorf("empty response (aimode)")
	}
	return text, 0, nil
}

// aimGenerate is the multi-key failover (round-robin, 429 cooldown, forced
// last-resort attempt when every key rests) — same shape as autoreply.
func aimGenerate(system, user string) (string, error) {
	var errs []string
	calls := 0
	aimKeyMu.Lock()
	ptr := aimKeyPtr
	aimKeyMu.Unlock()
	for attempt := 0; attempt < aimMaxKeys; attempt++ {
		idx := arNextIndex(ptr+attempt, aimMaxKeys)
		key := aimKeyAt(idx)
		if key == "" {
			continue
		}
		aimKeyMu.Lock()
		until, resting := aimCooldownUntil[idx]
		aimKeyMu.Unlock()
		if resting && time.Now().Before(until) {
			continue
		}
		for _, model := range aimModels {
			if model == "" || arModelCooling(model) {
				continue
			}
			calls++
			out, retrySec, err := aimCallOne(key, model, system, user)
			if err == nil {
				aimKeyMu.Lock()
				delete(aimCooldownUntil, idx)
				aimKeyPtr = arNextIndex(idx+1, aimMaxKeys)
				aimKeyMu.Unlock()
				return out, nil
			}
			if retrySec > 0 {
				arModelSetCooldown(model, retrySec)
			}
			errs = append(errs, fmt.Sprintf("key#%d/%s: %v", idx+1, model, err))
		}
		aimKeyMu.Lock()
		aimCooldownUntil[idx] = time.Now().Add(30 * time.Second)
		aimKeyMu.Unlock()
	}
	// Saari keys resting thi (koi actual call nahi hui) — last-resort force.
	if calls == 0 {
		for i := 0; i < aimMaxKeys; i++ {
			idx := arNextIndex(ptr+i, aimMaxKeys)
			key := aimKeyAt(idx)
			if key == "" {
				continue
			}
			for _, model := range aimModels {
				out, retrySec, err := aimCallOne(key, model, system, user)
				if err == nil {
					aimKeyMu.Lock()
					delete(aimCooldownUntil, idx)
					aimKeyPtr = arNextIndex(idx+1, aimMaxKeys)
					aimKeyMu.Unlock()
					return out, nil
				}
				if retrySec > 0 {
					arModelSetCooldown(model, retrySec)
				}
				errs = append(errs, fmt.Sprintf("key#%d (forced)/%s: %v", idx+1, model, err))
			}
		}
	}
	return "", fmt.Errorf("saari aimode key(s) fail: %s", strings.Join(errs, " | "))
}

// ----------------------------------------------------------------------------
// Redis state
// ----------------------------------------------------------------------------

func aimEnabled(s SessionBridge) bool {
	return strings.EqualFold(strings.TrimSpace(s.GetPresenceSetting(aimModeField, "off")), "on")
}

// AIModeStateLabel renders the ON/OFF label used across the .aimode texts and
// the connected card.
func AIModeStateLabel(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

func aimSetEnabled(s SessionBridge, on bool) {
	v := "off"
	if on {
		v = "on"
	}
	s.SetPresenceSetting(aimModeField, v)
}

// aimPrefixWord returns the wake-word the resolver requires. When the owner
// never set one, the pair.js default ("AI") applies — so AI Mode only reacts to
// messages that start with that word instead of swallowing every plain message.
func aimPrefixWord(s SessionBridge) string {
	return normalizeAimWakeWord(s.GetPresenceSetting(aimPrefixField, ""))
}

// normalizeAimWakeWord resolves the stored wake-word to its effective value.
// A missing field (""), a leaked missing-sentinel ("\x00" — same bug class the
// prefix key had) or a literal "null" all mean "not set", so they fall back to
// the default. Without this the gate regex is built from the sentinel and never
// matches a real message, silently disabling AI Mode. Whitespace-only values are
// treated as unset too, so `\b` cannot be satisfied by an empty pattern.
func normalizeAimWakeWord(stored string) string {
	w := strings.TrimSpace(stored)
	if w == "" || w == "\x00" || strings.EqualFold(w, "null") {
		return aimDefaultPrefixWord
	}
	return w
}

// ----------------------------------------------------------------------------
// corpus + resolve
// ----------------------------------------------------------------------------

type aimCorpusEntry struct {
	Command     string
	Description string
}

// aimGroupOnlyCommands — mirror of Node AIM_GROUP_ONLY_COMMANDS: these can
// never work outside a real group, so they are discarded in inbox/DM.
var aimGroupOnlyCommands = map[string]bool{
	"gcbotoff": true, "gcboton": true,
	"usergcban": true, "usergcunban": true,
	"bangcuser": true, "unbangcuser": true,
}

// aimCoreDescriptions supplies the MENU description for the main-package core
// commands (ping/menu/alive/uptime/sessions), which live outside the gold-cmds
// registry and therefore carry no Desc in the corpus. Without a description the
// AI has no signal for them ("bot ki speed check karo" → ping).
var aimCoreDescriptions = map[string]string{
	"ping":     "Shows the bot response speed / ping time (bot ki speed check karo).",
	"menu":     "Shows the full bot command menu with every category (menu dikhao / list all commands).",
	"alive":    "Shows that the bot is alive and running with uptime info.",
	"uptime":   "Shows how long the bot has been running (bot uptime check).",
	"sessions": "Lists the paired bot sessions/numbers.",
}

// aimCorpus builds the live command corpus (name + description) — the Go
// equivalent of BilalGetKnownCommandCorpus(): every gold-cmds registry command
// (with its MENU description, fed to the AI as "command — description") plus the
// main-package core commands (ping/menu/alive/uptime/sessions) which live
// outside the registry.
func aimCorpus() []aimCorpusEntry {
	seen := map[string]bool{}
	var out []aimCorpusEntry
	for _, c := range Commands() {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		d := c.Desc
		if d == "" {
			d = aimCoreDescriptions[name]
		}
		out = append(out, aimCorpusEntry{Command: name, Description: d})
	}
	for _, name := range cmdNameKnownNames() {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, aimCorpusEntry{Command: name, Description: aimCoreDescriptions[name]})
	}
	return out
}

var aimEscapeRe = regexp.MustCompile(`[.*+?^${}()|\[\]\\]`)

func aimEscapeRegexLiteral(s string) string {
	return aimEscapeRe.ReplaceAllString(s, `\$0`)
}

// aimStopWords are the very common filler tokens that carry no command
// signal — they are dropped before candidate scoring so "karo", "hai", "me"
// etc. never inflate a match.
var aimStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "am": true,
	"to": true, "of": true, "in": true, "on": true, "off": true, "at": true,
	"for": true, "and": true, "or": true, "my": true, "i": true,
	"you": true, "your": true, "it": true, "this": true, "that": true,
	"karo": true, "kar": true, "kro": true, "krdo": true, "kardo": true,
	"hai": true, "ha": true, "ho": true, "hoon": true, "hun": true,
	"ko": true, "ka": true, "ki": true, "ke": true, "se": true, "me": true,
	"mein": true, "ye": true, "yeh": true, "wo": true, "plz": true, "please": true,
	"bhai": true, "yaar": true, "bro": true, "sir": true, "bot": true,
}

// aimSynonyms adds extra scoring tokens (Roman-Urdu / casual phrasing) to a
// command so a plain sentence still shortlists it even when the command NAME and
// its MENU description share no letters (e.g. "gana" → play). These tokens are
// ALSO appended to the description in the AI prompt, which can only help the
// model — they never change a command's real MENU text.
var aimSynonyms = map[string]string{
	"play":       "gana song music mp3 audio gaana bajao sunao",
	"play2":      "gana song music mp3 audio",
	"play3":      "gana song music mp3 audio",
	"video":      "youtube video clip film download uthao nikal do",
	"video2":     "youtube video clip download",
	"video3":     "youtube video clip download",
	"fb":         "facebook fb reel video download",
	"ping":       "speed check response fast slow",
	"menu":       "list commands help sab commands dikhao",
	"uptime":     "kitni der se chal raha running time",
	"alive":      "zinda online check",
	"statusseen": "status seen khud dekh gaya auto view",
	"gcbotoff":   "group lock band kar do close",
	"gcboton":    "group unlock khol do open",
	"usergcban":  "member ko group me ban karo rok do",
	"botblock":   "bot commands se ban karo rok do",
	"anticall":   "calls auto reject band karo ring",
}

// aimDescFor returns the description used for scoring + the AI prompt, with the
// synonym hints appended.
func aimDescFor(c aimCorpusEntry) string {
	if h := aimSynonyms[c.Command]; h != "" {
		if c.Description == "" {
			return h
		}
		return c.Description + " " + h
	}
	return c.Description
}

func aimTokenize(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 2 {
			out = append(out, strings.ToLower(cur.String()))
		}
		cur.Reset()
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// aimLev is the classic Levenshtein distance (same as the Node helper).
func aimLev(a, b string) int {
	m, n := len(a), len(b)
	if m == 0 {
		return n
	}
	if n == 0 {
		return m
	}
	prev := make([]int, n+1)
	cur := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}
	for i := 1; i <= m; i++ {
		cur[0] = i
		for j := 1; j <= n; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[n]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// aimSim returns the similarity (0..1) between two tokens.
func aimSim(a, b string) float64 {
	if a == b {
		return 1
	}
	if len(a) >= 3 && len(b) >= 3 && (strings.Contains(a, b) || strings.Contains(b, a)) {
		return 0.9
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 0
	}
	return 1 - float64(aimLev(a, b))/float64(maxLen)
}

type aimIndexEntry struct {
	entry      aimCorpusEntry
	nameTokens []string
	descTokens []string
}

var (
	aimIndexOnce sync.Once
	aimIndex     []aimIndexEntry
	aimIndexList []aimCorpusEntry
)

// aimBuildIndex tokenizes the corpus ONCE (commands register at init, so the
// index never goes stale) — per-message cost drops to just the message tokens.
func aimBuildIndex() {
	corpus := aimCorpus()
	aimIndex = make([]aimIndexEntry, 0, len(corpus))
	aimIndexList = make([]aimCorpusEntry, 0, len(corpus))
	for _, c := range corpus {
		aimIndex = append(aimIndex, aimIndexEntry{
			entry:      c,
			nameTokens: aimTokenize(c.Command),
			descTokens: aimTokenize(aimDescFor(c)),
		})
		aimIndexList = append(aimIndexList, c)
	}
}

// aimCorpusCached returns the full corpus (index built once).
func aimCorpusCached() []aimCorpusEntry {
	aimIndexOnce.Do(aimBuildIndex)
	return aimIndexList
}

// aimCandidates returns the most promising corpus entries for a message, scored
// by fuzzy token overlap against the command NAME (weight 1.0) and its
// description (weight 0.9 — the symptom-matching signal). GOLD-MD has ~4000
// commands, so the AI gets this shortlist instead of the whole corpus; the
// corpus-membership hard gate afterwards still checks the FULL list, exactly
// like the Node resolver.
func aimCandidates(corpus []aimCorpusEntry, message string, max int, minScore float64) []aimCorpusEntry {
	aimIndexOnce.Do(aimBuildIndex)
	var sig []string
	for _, t := range aimTokenize(message) {
		if !aimStopWords[t] {
			sig = append(sig, t)
		}
	}
	if len(sig) == 0 {
		return nil
	}
	type scored struct {
		e     aimCorpusEntry
		score float64
	}
	out := make([]scored, 0, 64)
	for _, ie := range aimIndex {
		total := 0.0
		for _, t := range sig {
			best := 0.0
			for _, nt := range ie.nameTokens {
				if s := aimSim(t, nt); s > best {
					best = s
				}
			}
			for _, dt := range ie.descTokens {
				if s := aimSim(t, dt) * 0.9; s > best {
					best = s
				}
			}
			if best >= 0.6 {
				total += best
			}
		}
		if total >= minScore {
			out = append(out, scored{e: ie.entry, score: total})
		}
	}
	// insertion sort (desc) — the sliced list stays small.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].score > out[j-1].score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	res := make([]aimCorpusEntry, 0, len(out))
	for _, s := range out {
		res = append(res, s.e)
	}
	return res
}

// aimResolve is the Go port of BilalAiResolveCommandFromMessage: the lexical
// shortlist, ONE AI call, then the JS-level hard gates (corpus membership +
// group-only scope). Returns ok=false on NO_COMMAND_FOUND or any gate failure.
func aimResolve(userMessage string, isGroupChat bool) (string, bool) {
	if strings.TrimSpace(userMessage) == "" {
		return "", false
	}
	corpus := aimCorpusCached()
	if len(corpus) == 0 {
		return "", false
	}
	shortlist := aimCandidates(nil, userMessage, 40, 0.8)
	if len(shortlist) == 0 {
		return "", false
	}
	chatTypeLine := "CURRENT CHAT TYPE: INBOX/PRIVATE DM (yeh message kisi individual ki PRIVATE chat se aaya hai — koi group nahi hai)"
	if isGroupChat {
		chatTypeLine = "CURRENT CHAT TYPE: GROUP (yeh message ek WhatsApp GROUP se aaya hai)"
	}
	var b strings.Builder
	for i, c := range shortlist {
		if i > 0 {
			b.WriteString("\n")
		}
		d := aimDescFor(c)
		if len(d) > 200 {
			d = d[:200]
		}
		if d != "" {
			b.WriteString(c.Command + " — " + d)
		} else {
			b.WriteString(c.Command)
		}
	}
	system := aiCmdResolvePrompt + "\n\n" + chatTypeLine + "\n\nAvailable commands: " + b.String()

	raw, err := aimGenerate(system, userMessage)
	if err != nil {
		return "", false
	}
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '\n'); i >= 0 {
		raw = strings.TrimSpace(raw[:i]) // STRICT: only the first line
	}
	if raw == "" || strings.Contains(strings.ToUpper(raw), "NO_COMMAND_FOUND") {
		return "", false
	}
	cleaned := aimCleanResolved(raw)
	parts := strings.Fields(cleaned)
	if len(parts) == 0 {
		return "", false
	}
	base := strings.ToLower(parts[0])
	found := false
	for _, c := range corpus {
		if c.Command == base {
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	if aimGroupOnlyCommands[base] && !isGroupChat {
		return "", false
	}
	return cleaned, true
}

// aimCleanResolved strips leading punctuation/prefix and any trailing markdown
// or quote noise the model may tack on (e.g. "... off**"), while leaving the
// command name and its verbatim argument untouched.
func aimCleanResolved(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimLeft(s, ".!/#$%&*;(")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*`~_\"'”′ \t")
	return strings.TrimSpace(s)
}

// ----------------------------------------------------------------------------
// resolve cache (repeat-text fast-path, same TTL/size as Node)
// ----------------------------------------------------------------------------

const aimCacheTTL = 10 * 60 * 1000 // ms

var (
	aimCacheMu sync.Mutex
	aimCache   = map[string]aimCacheEntry{}
)

type aimCacheEntry struct {
	value string
	ok    bool
	ts    int64
}

func aimCacheGet(key string) (aimCacheEntry, bool) {
	aimCacheMu.Lock()
	defer aimCacheMu.Unlock()
	e, found := aimCache[key]
	if !found {
		return aimCacheEntry{}, false
	}
	if time.Now().UnixMilli()-e.ts > aimCacheTTL {
		delete(aimCache, key)
		return aimCacheEntry{}, false
	}
	return e, true
}

func aimCacheSet(key, value string, ok bool) {
	aimCacheMu.Lock()
	defer aimCacheMu.Unlock()
	if len(aimCache) >= 500 {
		for k := range aimCache {
			delete(aimCache, k)
			break
		}
	}
	aimCache[key] = aimCacheEntry{value: value, ok: ok, ts: time.Now().UnixMilli()}
}

// ----------------------------------------------------------------------------
// texts — GOLD-MD design: *BOLD*, 🔰 marks, ❮ ❯ value brackets, ❲ ❳ command
// brackets, "LABEL :❵ description" rows.
// ----------------------------------------------------------------------------

// aimOwnerOnlyText matches every other owner-only command in the bot.
func aimOwnerOnlyText() string {
	return "*THIS COMMAND IS ONLY FOR ME 😎*"
}

func aimToggleText(on bool) string {
	if on {
		return `*🔰 AI MODE TURNED ON 🔰*

*AI MODE :❯ ❮ ON ❯*
*WORKING :❯ EVERY CHAT + GROUP*

*DESCRIPTION :❵ NOW TYPE A NORMAL SENTENCE IN ANY CHAT OR GROUP AND THE AI WILL FIND AND RUN THE RIGHT COMMAND FOR YOU.*`
	}
	return `*🔰 AI MODE TURNED OFF 🔰*

*AI MODE :❯ ❮ OFF ❯*
*WORKING :❯ EVERY CHAT + GROUP*

*DESCRIPTION :❵ PLAIN SENTENCES ARE IGNORED NOW. ONLY NORMAL PREFIX COMMANDS WILL WORK UNTIL YOU TURN AI MODE BACK ON.*`
}

func aimStatusText(on bool, word string) string {
	return `*🔰 AI MODE STATUS 🔰*

*AI MODE :❯ ❮ ` + AIModeStateLabel(on) + ` ❯*
*WAKE-WORD :❯ ❮ ` + word + ` ❯*

*DESCRIPTION :❵ YEH SETTING HAR CHAT AUR GROUP PAR APPLY HOTI HAI.*
*HOW TO USE :❵ TYPE ❲ ` + strings.ToUpper(word) + ` CHECK BOT SPEED ❳*`
}

func aimPrefixSetText(pw string) string {
	return `*🔰 AI MODE PREFIX SET 🔰*

*WAKE-WORD :❯ ❮ ` + pw + ` ❯*

*HOW TO USE :❵ DON'T TYPE YOUR MAIN PREFIX (. , ? @) — TYPE SIMPLE TEXT WITH THE AI MODE WAKE-WORD, SAME LIKE THAT*
*` + pw + ` CHECK BOT SPEED*
*` + pw + ` SHOW BOT COMMANDS*
*` + pw + ` CHECK BOT UPTIME*
*` + pw + ` DOWNLOAD VIDEO FROM FACEBOOK LINK : (PASTE LINK)*
*` + pw + ` ACTIVATE THE STATUS*
*` + pw + ` (ASK TO RUN COMMAND)*

*STILL CONFUSED ? SEE THE COMPLETE AIMODE FULL GUIDANCE VIDEO*
https://youtu.be/grZBRtnebgs?is=jYy2wtT5Voxn3XYu`
}

func aimPrefixInfoText(prefix, word string) string {
	return `*🔰 AI MODE PREFIX INFO 🔰*

*CURRENT WAKE-WORD :❯ ❮ ` + strings.ToUpper(word) + ` ❯*

*DESCRIPTION :❵ AI MODE ONLY REACTS TO MESSAGES THAT START WITH THIS WAKE-WORD. CHANGE IT ANY TIME.*

*TYPE ❲ ` + prefix + `AIMODE PREFIX NEWWORD ❳* — set your own wake-word
*EXAMPLE :❵ ` + prefix + `aimode prefix KING*

*HOW TO USE :❵ TYPE ❲ ` + strings.ToUpper(word) + ` CHECK BOT SPEED ❳*`
}

func aimGuideText(prefix string, on bool, word string) string {
	pw := strings.ToUpper(word)
	return `*🔰 AI MODE COMMAND GUIDE 🔰*

*AI MODE :❯ ❮ ` + AIModeStateLabel(on) + ` ❯*
*WAKE-WORD :❯ ❮ ` + pw + ` ❯*

*TYPE ❲ ` + prefix + `AIMODE ON ❳* — ACTIVATE
*TYPE ❲ ` + prefix + `AIMODE OFF ❳* — DE-ACTIVATE
*TYPE ❲ ` + prefix + `AIMODE STATUS ❳* — SHOW CURRENT STATE
*TYPE ❲ ` + prefix + `AIMODE PREFIX ❳* — SHOW THE WAKE-WORD

*SET YOUR OWN WAKE-WORD*
*TYPE ❲ ` + prefix + `AIMODE PREFIX KING ❳*
*TYPE ❲ ` + prefix + `AIMODE PREFIX BOSS ❳*
*TYPE ❲ ` + prefix + `AIMODE PREFIX YOUR NAME ❳*

*HOW TO USE :❵ DON'T TYPE YOUR MAIN PREFIX (. , ? @) — TYPE SIMPLE TEXT WITH THE AI MODE WAKE-WORD, SAME LIKE THAT*
*` + pw + ` CHECK BOT SPEED*
*` + pw + ` SHOW BOT COMMANDS*
*` + pw + ` CHECK BOT UPTIME*
*` + pw + ` DOWNLOAD VIDEO FROM FACEBOOK LINK : (PASTE LINK)*
*` + pw + ` ACTIVATE THE STATUS*
*` + pw + ` (ASK TO RUN COMMAND)*

*STILL CONFUSED ? SEE THE COMPLETE AIMODE FULL GUIDANCE VIDEO*
https://youtu.be/grZBRtnebgs?is=jYy2wtT5Voxn3XYu`
}

func aimUsageText(prefix string) string {
	return `*🔰 AI MODE WRONG FORMAT 🔰*

*TYPE ❲ ` + prefix + `AIMODE ON ❳* / ❲ ` + prefix + `AIMODE OFF ❳*
*TYPE ❲ ` + prefix + `AIMODE STATUS ❳* — SHOW CURRENT STATE
*TYPE ❲ ` + prefix + `AIMODE PREFIX NEWWORD ❳* — SET WAKE-WORD
*TYPE ❲ ` + prefix + `AIMODE ❳* — FULL GUIDE`
}

// ----------------------------------------------------------------------------
// trigger (called from handler.go, before the prefix check)
// ----------------------------------------------------------------------------

// aimCmdRe matches the bare command token "aimode" plus its optional argument.
var aimCmdRe = regexp.MustCompile(`(?i)^aimode(?:\s+([\s\S]*))?\s*$`)

// AIModeTryHandle is the Go port of the Node AI-mode block:
//   - ".aimode on/off/status/prefix/[guide]" (owner-only) is answered here;
//   - otherwise, when AI mode is ON, a plain non-command message is resolved to
//     a real command and returned as a prefixed rewrite for normal dispatch.
//
// Returns handled=true to stop processing; when rewrite != "" the caller must
// set body = rewrite and continue into the normal command dispatch.
func AIModeTryHandle(s SessionBridge, info types.MessageInfo, body, prefix string) (handled bool, rewrite string) {
	defer func() { _ = recover() }()

	body = strings.TrimSpace(body)
	if body == "" {
		return false, ""
	}
	if info.Chat == types.StatusBroadcastJID || info.Chat.Server == types.NewsletterServer {
		return false, ""
	}

	alreadyCmd := prefix != "" && strings.HasPrefix(body, prefix)
	arg := ""
	isAimode := false
	if alreadyCmd {
		if m := aimCmdRe.FindStringSubmatch(strings.TrimSpace(body[len(prefix):])); m != nil {
			isAimode, arg = true, strings.TrimSpace(m[1])
		}
	} else if prefix == "" {
		if m := aimCmdRe.FindStringSubmatch(body); m != nil {
			isAimode, arg = true, strings.TrimSpace(m[1])
		}
	}

	if isAimode {
		if !s.IsOwner(info) {
			s.Reply(info, aimOwnerOnlyText())
			return true, ""
		}
		tokens := strings.Fields(arg)
		action := ""
		if len(tokens) > 0 {
			action = strings.ToLower(tokens[0])
		}
		switch action {
		case "on", "off":
			aimSetEnabled(s, action == "on")
			s.Reply(info, aimToggleText(action == "on"))
			// Owner ke inbox mein foran updated connected card (naya AI MODE
			// state) — pair.js BilalSendConnectedNotice jaisa hi.
			s.NotifyConnectedCard()
		case "status":
			s.Reply(info, aimStatusText(aimEnabled(s), aimPrefixWord(s)))
		case "prefix":
			if len(tokens) < 2 {
				s.Reply(info, aimPrefixInfoText(prefix, aimPrefixWord(s)))
			} else {
				newWord := tokens[1]
				s.SetPresenceSetting(aimPrefixField, newWord)
				s.Reply(info, aimPrefixSetText(strings.ToUpper(newWord)))
				s.NotifyConnectedCard()
			}
		case "":
			s.Reply(info, aimGuideText(prefix, aimEnabled(s), aimPrefixWord(s)))
		default:
			s.Reply(info, aimUsageText(prefix))
		}
		return true, ""
	}

	// ── natural-language resolver ──
	if alreadyCmd || !aimEnabled(s) {
		return false, ""
	}
	body2 := body
	if wake := aimPrefixWord(s); wake != "" {
		re := regexp.MustCompile(`(?i)^` + aimEscapeRegexLiteral(wake) + `\b\s*`)
		m := re.FindString(body)
		if m == "" {
			return false, ""
		}
		body2 = strings.TrimSpace(body[len(m):])
		if body2 == "" {
			return false, ""
		}
	}

	corpus := aimCorpus()
	quickKey := strings.Join(strings.Fields(strings.ToLower(body2)), " ")
	isGroup := info.Chat.Server == types.GroupServer

	resolved, ok := "", false
	for _, c := range corpus { // quick-match 1: whole message == command name
		if c.Command == quickKey {
			resolved, ok = c.Command, true
			break
		}
	}
	if !ok { // quick-match 2: "<cmd> on|off" / "on|off <cmd>"
		if tf := strings.Fields(quickKey); len(tf) == 2 {
			a, b := tf[0], tf[1]
			if a == "on" || a == "off" {
				a, b = b, a
			}
			if b == "on" || b == "off" {
				for _, c := range corpus {
					if c.Command == a {
						resolved, ok = c.Command+" "+b, true
						break
					}
				}
			}
		}
	}
	if !ok {
		cacheKey := "i:" + quickKey
		if isGroup {
			cacheKey = "g:" + quickKey
		}
		if e, hit := aimCacheGet(cacheKey); hit {
			resolved, ok = e.value, e.ok
		} else {
			resolved, ok = aimResolve(body2, isGroup)
			aimCacheSet(cacheKey, resolved, ok)
		}
	}
	if !ok {
		return false, ""
	}

	base := strings.ToLower(strings.Fields(resolved)[0])
	if aimGroupOnlyCommands[base] && !isGroup {
		return false, ""
	}
	// Owner-only gate: the AI is a translator, never a permission bypass.
	for _, c := range Commands() {
		if strings.EqualFold(c.Name, base) && c.OwnerOnly && !s.IsOwner(info) {
			s.Reply(info, aimOwnerOnlyText())
			return true, ""
		}
	}
	return true, prefix + resolved
}

func init() {
	Register(Command{
		Name:      "aimode",
		Category:  "AI",
		Desc:      `Owner-only. Lets the bot respond to plain everyday sentences (no command prefix needed) and auto-run the matching command. "aimode on" / "aimode off" toggles it globally, "aimode status" shows current ON/OFF + wake-word, "aimode prefix <word>" sets/shows the wake-word that messages must start with. No argument shows the full guide.`,
		OwnerOnly: true,
		Run:       handleAIModeCmd,
	})
}

// handleAIModeCmd — direct .aimode invocation (the pre-hook normally catches it;
// this is the registry path so .aimode appears in the AI menu and still works).
func handleAIModeCmd(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	AIModeTryHandle(s, info, prefix+"aimode "+strings.Join(args, " "), prefix)
}

// AIModeEnabledFor reports whether AI Mode is ON for the bot behind botJID
// ("" = the only/current bot). Used by the connected card to show the live
// state — pair.js BilalGetAIModeSetting equivalent.
func AIModeEnabledFor(botJID string) bool {
	if aimSettingReader == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(aimSettingReader(botJID, aimModeField, "off")), "on")
}

// AIModeWakeWordFor returns the effective wake-word for the bot behind botJID
// (the configured one, or the pair.js default "AI"). Used by the connected card.
func AIModeWakeWordFor(botJID string) string {
	if aimSettingReader == nil {
		return aimDefaultPrefixWord
	}
	return normalizeAimWakeWord(aimSettingReader(botJID, aimPrefixField, ""))
}

// aimSettingReader reads one per-bot presence setting (Redis). Attached by the
// main package at startup — amute hook pattern — so the connected card can show
// the live AI MODE state without a full SessionBridge.
var aimSettingReader func(botJID, field, def string) string

// AIModeAttachSettingReader — main package startup par call karta hai.
func AIModeAttachSettingReader(fn func(botJID, field, def string) string) {
	if fn != nil {
		aimSettingReader = fn
	}
}
