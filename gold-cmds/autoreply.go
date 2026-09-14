package goldcmds

// ============================================================================
// GOLD-MD — .autoreply / .autoreplyprem commands (AI auto-reply, Mistral)
//
// Ported from Node.js pair.js — SAME WORK (0% farak), SAME TEXT (0% farak).
// Crown emoji in Node.js is replaced with 🔰 in GOLD-MD.
//
// AI POOL: 10 Mistral keys hardcoded in-file (repo is private). Round-robin
// pointer + 429 cooldown (Retry-After header, default 30s, cap 60s) +
// last-resort forced attempt when all keys are resting — same as Node.js.
//
// QUEUE: per-chat FIFO (max 3 backlog), max 25 concurrent jobs globally,
// 5s min reply gap per chat, 4s typing (composing) refresh, "⏳ Wait......"
// messages (5s gap) deleted for everyone once the reply is ready, reply sent
// QUOTED to the trigger message (no botname footer — AI replies must look
// like normal human messages), then ONE same-text edit (whatsmeow official
// BuildEdit = protocolMessage type 14) after 1s, then "paused" presence.
//
// STORAGE (Redis, settings:<botJID> hash — same pattern as autoblock):
//   field "autoreply"          = "on"/"groups"/"inbox"/"off" (default off)
//   field "autoreply:delay"    = "true"/"false" (typingDelay, default true)
//   field "autoreply:excluded" = comma-joined excluded user JIDs
//
// TRIGGER (handler.go hook, before command dispatch — same as Node.js which
// runs it BEFORE the isCommand check, so it fires on command messages too):
//   skip own messages / status@broadcast / newsletters / reaction /
//   protocol / senderKeyDistribution / pollUpdate message types.
// ============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// ----------------------------------------------------------------------------
// constants (same values as Node.js pair.js)
// ----------------------------------------------------------------------------

const (
	arModel           = "ministral-8b-latest" // chota small latest model (fast + working)
	arModelFallbacks  = "open-mistral-nemo"   // single fallback if primary ever throttles
	arBaseURL         = "https://api.mistral.ai/v1/chat/completions"
	arTimeout         = 15 * time.Second
	arMaxTokens       = 120
	arUserMaxRunes    = 600
	arReplyMaxRunes   = 400
	arMaxConcurrent   = 25
	arMaxQueuePerChat = 3
	arReplyGap        = 5000 * time.Millisecond
	arTypingRefresh   = 4000 * time.Millisecond
	arWaitMsgGap      = 5000 * time.Millisecond
	arHumanTypingWait = 5000 * time.Millisecond
	arEditDelay       = 1000 * time.Millisecond
)

// arSystemPrompt is the LOCKED system prompt (verbatim from Node.js pair.js).
const arSystemPrompt = "You are a normal person chatting on WhatsApp with a close friend — NOT an AI. RULES:\n" +
	"1) Be warm, friendly, sweet and caring — like texting someone you like talking to. Never sound robotic, cold, or generic.\n" +
	"2) Reply in MAX 2 short lines — never more than 2 lines.\n" +
	"3) Your reply must directly make sense and respond to what the user actually just said — read their message carefully, stay on topic, never give a random or confusing reply.\n" +
	"4) Use 1 emoji naturally where it genuinely fits the feeling of the reply (not every line, never more than 1-2, never flood emojis).\n" +
	"5) Never say you are an AI, a bot, an assistant, or mention any AI/company/tool.\n" +
	"6) Talk simply and casually like a real person: short, simple sentences, no markdown, no long words.\n" +
	"7) Always reply in the EXACT SAME language AND script the user just wrote in — Roman Urdu, Urdu script, English, Arabic, Hindi, or any other language. Detect it from their latest message and match it exactly (do not translate, do not switch script).\n" +
	"8) If asked about yourself, just say you're a normal person chatting.\n" +
	"Reply ONLY with the message text — nothing else."

// goldAutoReplyKeys — the bot's 10 Mistral keys, hardcoded in-file
// (repo is private — same pool as Node.js AUTOREPLY_KEY_1..10).
var goldAutoReplyKeys = []string{
	"DdDToy8oM8Q77qQtGS2jOiVo2YKzWUVA",
	"EqiYLLNXsGkQMfoWfiYCoZOoG7oUDgyQ",
	"L3UbGRxp5p7bpzom9oHDIbewxzZafuVI",
	"yELUZ1qmGzH65SPnFrNeJ80xlQo15sG6",
	"QHm7nLoAYReZCth3zuxiIKKkjBaHJr4E",
	"m3aHFmjbl9iFCMowgNyTvef0SUEgto3w",
	"FJwTlE6PKx7Pl7KPZ6v9xJ8do09dxXl3",
	"cuBhZ30sslYXU0woXuRfc4z7FPcIHQnZ",
	"83VLReTP4tL8omK2vzh0ea8I2aVzgUjM",
	"fJGj2KGvJBGLhlx475U1OWfv4DngP7PU",
}

// ----------------------------------------------------------------------------
// key pool: round-robin pointer + 429 cooldown map
// ----------------------------------------------------------------------------

var (
	arKeyMu         sync.Mutex
	arKeyPtr        int                   // round-robin pointer
	arCooldownUntil = map[int]time.Time{} // key index → rest until
)

// arNextIndex is a safe modulo (same as Node.js nextIndex helper).
func arNextIndex(i, total int) int {
	if total <= 0 {
		return 0
	}
	return ((i % total) + total) % total
}

// arParseRetrySeconds parses the Retry-After header (default 30s fallback).
func arParseRetrySeconds(raw string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
		return n
	}
	return 30
}

// arCallOne makes ONE API call on ONE key. Returns (reply, retrySeconds,
// error); retrySeconds > 0 means HTTP 429 (rate limited).
func arCallOne(apiKey, model, userText string) (string, int, error) {
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": arSystemPrompt},
			{"role": "user", "content": userText},
		},
		"max_tokens": arMaxTokens,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), arTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", arBaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: arTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return "", arParseRetrySeconds(resp.Header.Get("Retry-After")), fmt.Errorf("HTTP 429 (autoreply)")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return "", 0, fmt.Errorf("HTTP %d (autoreply) — %s", resp.StatusCode, string(raw))
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
		return "", 0, fmt.Errorf("empty response (autoreply)")
	}
	text := strings.TrimSpace(data.Choices[0].Message.Content)
	if text == "" {
		return "", 0, fmt.Errorf("empty response (autoreply)")
	}
	return text, 0, nil
}

// goldAutoReplyAI is the multi-key failover (round-robin, 429 cooldown,
// last-resort forced attempt when all keys are resting) — same as Node.js.
// arModelChain returns the full model try-order: primary first, then the
// fallback chain (split from arModelFallbacks). Deduplicated + non-empty.
func arModelChain() []string {
	out := []string{arModel}
	for _, m := range strings.Split(arModelFallbacks, ",") {
		m = strings.TrimSpace(m)
		if m == "" || m == arModel {
			continue
		}
		dup := false
		for _, x := range out {
			if x == m {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, m)
		}
	}
	return out
}

// goldModelCooldowns tracks per-model 429 throttle windows (an IP-throttled
// model gets skipped until its window expires, so we stop hammering it).
var (
	goldModelCdMu    sync.Mutex
	goldModelCdUntil = map[string]time.Time{}
)

// arModelCooling reports whether a model is inside its 429 cooldown window.
func arModelCooling(model string) bool {
	goldModelCdMu.Lock()
	defer goldModelCdMu.Unlock()
	until, ok := goldModelCdUntil[model]
	return ok && time.Now().Before(until)
}

// arModelSetCooldown puts a model on a short cooldown after a 429.
func arModelSetCooldown(model string, retrySec int) {
	if retrySec < 5 {
		retrySec = 5
	}
	if retrySec > 60 {
		retrySec = 60
	}
	goldModelCdMu.Lock()
	goldModelCdUntil[model] = time.Now().Add(time.Duration(retrySec) * time.Second)
	goldModelCdMu.Unlock()
}

func goldAutoReplyAI(userText string) (string, error) {
	total := len(goldAutoReplyKeys)
	if total == 0 {
		return "", fmt.Errorf("Autoreply: koi key nahi mili")
	}
	models := arModelChain()
	arKeyMu.Lock()
	ptr := arKeyPtr
	arKeyMu.Unlock()

	var errs []string
	apiCallsMade := 0
	for attempt := 0; attempt < total; attempt++ {
		keyIdx := arNextIndex(ptr+attempt, total)
		arKeyMu.Lock()
		restUntil, resting := arCooldownUntil[keyIdx]
		now := time.Now()
		arKeyMu.Unlock()
		if resting && now.Before(restUntil) {
			continue
		}
		// try this key on the primary model, then each fallback model
		// (a 429 on a model usually means the model is IP-throttled, so
		// only the KEY cooldown of the LAST-tried model applies)
		for _, model := range models {
			if arModelCooling(model) {
				continue
			}
			apiCallsMade++
			reply, retrySec, err := arCallOne(goldAutoReplyKeys[keyIdx], model, userText)
			if err == nil {
				arKeyMu.Lock()
				delete(arCooldownUntil, keyIdx)
				arKeyPtr = arNextIndex(keyIdx+1, total)
				arKeyMu.Unlock()
				return reply, nil
			}
			if retrySec > 0 {
				arModelSetCooldown(model, retrySec)
			}
			errs = append(errs, fmt.Sprintf("key#%d/%s: %v", keyIdx+1, model, err))
		}
		// whole model chain 429'd on this key — put the key on cooldown too
		arKeyMu.Lock()
		arCooldownUntil[keyIdx] = time.Now().Add(30 * time.Second)
		arKeyMu.Unlock()
	}
	// Saari keys resting thi (koi actual call nahi hui) — last-resort force
	if apiCallsMade == 0 {
		fallbackIdx := arNextIndex(ptr, total)
		for _, model := range models {
			reply, retrySec, err := arCallOne(goldAutoReplyKeys[fallbackIdx], model, userText)
			if err == nil {
				arKeyMu.Lock()
				delete(arCooldownUntil, fallbackIdx)
				arKeyPtr = arNextIndex(fallbackIdx+1, total)
				arKeyMu.Unlock()
				return reply, nil
			}
			if retrySec > 0 {
				arModelSetCooldown(model, retrySec)
			}
			errs = append(errs, fmt.Sprintf("key#%d (forced, all resting)/%s: %v", fallbackIdx+1, model, err))
		}
		arKeyMu.Lock()
		arCooldownUntil[fallbackIdx] = time.Now().Add(30 * time.Second)
		arKeyMu.Unlock()
	}
	arKeyMu.Lock()
	arKeyPtr = arNextIndex(ptr+1, total)
	arKeyMu.Unlock()
	return "", fmt.Errorf("Saari %d Autoreply key(s) fail hui: %s", total, strings.Join(errs, " | "))
}

// arAllKeysCoolingDown is a local-map-only check (no API calls).
func arAllKeysCoolingDown() bool {
	total := len(goldAutoReplyKeys)
	if total == 0 {
		return true
	}
	now := time.Now()
	arKeyMu.Lock()
	defer arKeyMu.Unlock()
	for i := 0; i < total; i++ {
		until, ok := arCooldownUntil[i]
		if !ok || !now.Before(until) {
			return false
		}
	}
	return true
}

// arEarliestKeyFreeAt returns the earliest time any key becomes free.
func arEarliestKeyFreeAt() time.Time {
	total := len(goldAutoReplyKeys)
	now := time.Now()
	var earliest time.Time
	arKeyMu.Lock()
	defer arKeyMu.Unlock()
	for i := 0; i < total; i++ {
		until, ok := arCooldownUntil[i]
		if !ok || !now.Before(until) {
			return now
		}
		if earliest.IsZero() || until.Before(earliest) {
			earliest = until
		}
	}
	if earliest.IsZero() {
		return now
	}
	return earliest
}

// ----------------------------------------------------------------------------
// settings cache (mode + typingDelay, 1-minute TTL, write-through invalidate)
// ----------------------------------------------------------------------------

type arSettingsCfg struct {
	Mode        string
	TypingDelay bool
}

var (
	arSettingsMu sync.Mutex
	arSettings   = map[string]*arSettingsCfg{}
	arSettingsAt = map[string]time.Time{}
)

func arSettingsLoad(s SessionBridge) *arSettingsCfg {
	botJID := s.GetJID()
	arSettingsMu.Lock()
	if c, ok := arSettings[botJID]; ok && time.Since(arSettingsAt[botJID]) < time.Minute {
		arSettingsMu.Unlock()
		return c
	}
	arSettingsMu.Unlock()
	mode := strings.ToLower(s.GetAutoReplyMode("off"))
	delayStr := strings.ToLower(s.GetAutoReplyDelay("true"))
	c := &arSettingsCfg{Mode: mode, TypingDelay: delayStr != "false"}
	arSettingsMu.Lock()
	arSettings[botJID] = c
	arSettingsAt[botJID] = time.Now()
	arSettingsMu.Unlock()
	return c
}

func arSettingsInvalidate(botJID string) {
	arSettingsMu.Lock()
	delete(arSettings, botJID)
	delete(arSettingsAt, botJID)
	arSettingsMu.Unlock()
}

// ----------------------------------------------------------------------------
// excluded users (autoreplyprem add/del) — Redis "autoreply:excluded"
// ----------------------------------------------------------------------------

// arNormalizeJID normalizes any JID to digits@s.whatsapp.net (same as Node.js).
func arNormalizeJID(jid string) string {
	num := stripNonDigits(stripJIDSuffix(jid))
	if num == "" {
		return ""
	}
	return num + "@s.whatsapp.net"
}

var (
	arExclMu  sync.Mutex
	arExclSet = map[string]map[string]bool{}
	arExclAt  = map[string]time.Time{}
	arExclRaw = map[string]string{}
)

func arExcludedLoad(s SessionBridge) map[string]bool {
	botJID := s.GetJID()
	arExclMu.Lock()
	if set, ok := arExclSet[botJID]; ok && time.Since(arExclAt[botJID]) < time.Minute {
		arExclMu.Unlock()
		return set
	}
	arExclMu.Unlock()
	raw := s.GetAutoReplyExcluded("")
	set := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	arExclMu.Lock()
	arExclSet[botJID] = set
	arExclAt[botJID] = time.Now()
	arExclRaw[botJID] = raw
	arExclMu.Unlock()
	return set
}

func arExcludedInvalidate(botJID string) {
	arExclMu.Lock()
	delete(arExclSet, botJID)
	delete(arExclAt, botJID)
	delete(arExclRaw, botJID)
	arExclMu.Unlock()
}

// arExcludedRawList returns the ordered excluded JID list (for .autoreplyprem list).
func arExcludedRawList(s SessionBridge) []string {
	botJID := s.GetJID()
	arExclMu.Lock()
	raw, ok := arExclRaw[botJID]
	fresh := ok && time.Since(arExclAt[botJID]) < time.Minute
	arExclMu.Unlock()
	if !fresh {
		arExcludedLoad(s)
		arExclMu.Lock()
		raw = arExclRaw[botJID]
		arExclMu.Unlock()
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// arAddExclude adds a user to the exclusion list (write-through).
func arAddExclude(s SessionBridge, jid string) {
	norm := arNormalizeJID(jid)
	if norm == "" {
		return
	}
	list := arExcludedRawList(s)
	for _, j := range list {
		if j == norm {
			return
		}
	}
	list = append(list, norm)
	s.SetAutoReplyExcluded(strings.Join(list, ","))
	arExcludedInvalidate(s.GetJID())
}

// arRemoveExclude removes a user from the exclusion list (write-through).
func arRemoveExclude(s SessionBridge, jid string) {
	norm := arNormalizeJID(jid)
	if norm == "" {
		return
	}
	list := arExcludedRawList(s)
	var out []string
	for _, j := range list {
		if j != norm {
			out = append(out, j)
		}
	}
	s.SetAutoReplyExcluded(strings.Join(out, ","))
	arExcludedInvalidate(s.GetJID())
}

// arSenderExcluded checks the trigger sender against the exclusion list
// (checks Sender and SenderAlt for LID <-> phone mapping).
func arSenderExcluded(s SessionBridge, info types.MessageInfo) bool {
	set := arExcludedLoad(s)
	if len(set) == 0 {
		return false
	}
	if set[arNormalizeJID(info.Sender.String())] {
		return true
	}
	if info.SenderAlt.Server != "" {
		if set[arNormalizeJID(info.SenderAlt.String())] {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// queue / typing control
// ----------------------------------------------------------------------------

type arJob struct {
	s           SessionBridge
	info        types.MessageInfo // trigger message info
	quotedProto *waProto.Message  // trigger message proto (captured at enqueue)
	userText    string
	typingDelay bool
}

type arChatQueue struct {
	jobs    []*arJob
	running bool
}

var (
	arQueueMu     sync.Mutex
	arChatQueues  = map[string]*arChatQueue{}
	arActiveJobs  int
	arLastReplyAt = map[string]time.Time{}
)

func arChatKey(chat types.JID) string {
	return strings.ToLower(chat.String())
}

func arSleep(ms time.Duration) {
	time.Sleep(ms)
}

// arTruncateRunes truncates by runes (JS .slice equivalent for text).
func arTruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// arWaitUntilSlot blocks until a global concurrency slot is free (max 25).
func arWaitUntilSlot() {
	for {
		arQueueMu.Lock()
		if arActiveJobs < arMaxConcurrent {
			arActiveJobs++
			arQueueMu.Unlock()
			return
		}
		arQueueMu.Unlock()
		arSleep(250 * time.Millisecond)
	}
}

// arEnqueue adds a job to the per-chat queue (max 3 backlog) and starts the
// drain loop if not already running — same as Node.js enqueue helper.
func arEnqueue(s SessionBridge, info types.MessageInfo, quoted *waProto.Message, userText string, typingDelay bool) {
	key := arChatKey(info.Chat)
	arQueueMu.Lock()
	q := arChatQueues[key]
	if q == nil {
		q = &arChatQueue{}
		arChatQueues[key] = q
	}
	if len(q.jobs) >= arMaxQueuePerChat {
		arQueueMu.Unlock()
		return
	}
	q.jobs = append(q.jobs, &arJob{s: s, info: info, quotedProto: quoted, userText: userText, typingDelay: typingDelay})
	if q.running {
		arQueueMu.Unlock()
		return
	}
	q.running = true
	arQueueMu.Unlock()
	go arDrainQueue(key, q)
}

// arDrainQueue processes the per-chat queue sequentially (reply order kept).
func arDrainQueue(key string, q *arChatQueue) {
	defer func() { _ = recover() }()
	for {
		arQueueMu.Lock()
		if len(q.jobs) == 0 {
			q.running = false
			delete(arChatQueues, key)
			arQueueMu.Unlock()
			return
		}
		job := q.jobs[0]
		q.jobs = q.jobs[1:]
		arQueueMu.Unlock()
		arRunReplyJob(job)
	}
}

// arRunReplyJob is the full reply pipeline — same as Node.js runReplyJob:
// slot wait → 5s per-chat reply gap → typing loop → optional 5s human typing
// wait → AI call loop (wait-msgs while keys cooling / on error) → delete
// wait-msgs → send reply quoted → ONE same-text edit after 1s → paused.
func arRunReplyJob(job *arJob) {
	defer func() { _ = recover() }()
	arWaitUntilSlot()
	chatKey := arChatKey(job.info.Chat)
	defer func() {
		arQueueMu.Lock()
		if arActiveJobs > 0 {
			arActiveJobs--
		}
		arQueueMu.Unlock()
	}()

	// per-chat reply gap (5s minimum between replies in the same chat)
	arQueueMu.Lock()
	last, hasLast := arLastReplyAt[chatKey]
	arQueueMu.Unlock()
	if hasLast {
		if gap := arReplyGap - time.Since(last); gap > 0 {
			arSleep(gap)
		}
	}

	// typing loop — "composing" presence refreshed every 4s until done
	typingDone := make(chan struct{})
	var typingOnce sync.Once
	stopTyping := func() { typingOnce.Do(func() { close(typingDone) }) }
	defer stopTyping()
	go func() {
		defer func() { _ = recover() }()
		_ = job.s.SendChatPresenceUpdate(job.info.Chat, "composing", "")
		tick := time.NewTicker(arTypingRefresh)
		defer tick.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-tick.C:
				_ = job.s.SendChatPresenceUpdate(job.info.Chat, "composing", "")
			}
		}
	}()

	// optional 5s human-feel typing wait before the AI request
	if job.typingDelay {
		arSleep(arHumanTypingWait)
	}

	// ── SINGLE ATTEMPT (no retry loop, no "Wait......" spam) ────────────
	// Owner order: retry loop bilkul hata do. Ek hi try — mila to bhejo,
	// nahi mila to khamoshi (koi wait message, koi spam nahi).
	r, err := goldAutoReplyAI(arTruncateRunes(job.userText, arUserMaxRunes))
	if err != nil {
		return
	}
	reply := arTruncateRunes(strings.TrimSpace(r), arReplyMaxRunes)
	if reply == "" {
		return
	}
	sentID := job.s.SendQuotedTextWithID(job.info, job.quotedProto, reply)
	arQueueMu.Lock()
	arLastReplyAt[chatKey] = time.Now()
	arQueueMu.Unlock()

	// ONE same-text edit after 1s (whatsmeow official BuildEdit = type 14)
	if sentID != "" {
		arSleep(arEditDelay)
		_ = job.s.EditMessage(job.info, sentID, reply)
	}

	// "paused" presence — stops the composing indicator
	stopTyping()
	_ = job.s.SendChatPresenceUpdate(job.info.Chat, "paused", "")
}

// ----------------------------------------------------------------------------
// trigger hook (called from handler.go BEFORE command dispatch)
// ----------------------------------------------------------------------------

// AutoReplyTrigger mirrors the Node.js background autoreply trigger: runs on
// every incoming message (including commands), checks mode/scope/exclusion,
// and enqueues the AI reply job. Never blocks the caller (run in a goroutine).
func AutoReplyTrigger(s SessionBridge, info types.MessageInfo) {
	defer func() { _ = recover() }()
	if info.IsFromMe {
		return
	}
	if info.Chat == types.StatusBroadcastJID || info.Chat.Server == types.NewsletterServer {
		return
	}
	raw := s.GetRawMessage(info)
	if raw == nil {
		return
	}
	// skip non-content message types (same list as Node.js _arSkipTypes)
	if raw.ReactionMessage != nil || raw.ProtocolMessage != nil ||
		raw.SenderKeyDistributionMessage != nil || raw.PollUpdateMessage != nil {
		return
	}
	cfg := arSettingsLoad(s)
	if cfg.Mode == "off" || cfg.Mode == "" {
		return
	}
	isGroup := info.Chat.Server == types.GroupServer
	scopeOk := cfg.Mode == "on" || (cfg.Mode == "groups" && isGroup) || (cfg.Mode == "inbox" && !isGroup)
	if !scopeOk {
		return
	}
	if arSenderExcluded(s, info) {
		return
	}
	userText := strings.TrimSpace(s.GetMessageText(info))
	if userText == "" {
		return
	}
	arEnqueue(s, info, raw, userText, cfg.TypingDelay)
}

// ----------------------------------------------------------------------------
// .autoreply command (owner-only) — texts 0% farak (🔰 → 🔰)
// ----------------------------------------------------------------------------

func arStatusText(mode string) string {
	switch mode {
	case "on":
		return "🔰 ON (ALL)"
	case "groups":
		return "🔰 ON (GROUPS ONLY)"
	case "inbox":
		return "🔰 ON (INBOX ONLY)"
	default:
		return "🔰 OFF"
	}
}

func handleAutoReply(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *AUTOREPLY ERROR — TRY AGAIN*")
		}
	}()
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	botJID := s.GetJID()
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch sub {
	case "":
		st := arStatusText(arSettingsLoad(s).Mode)
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY INFO 🔰*\n\n"+
			"*AUTOREPLY AUTOMATICALLY REPLIES TO INCOMING USER MESSAGES USING AI.*\n"+
			"*IT IS USEFUL WHEN YOU ARE BUSY AND CANNOT REPLY YOURSELF.*\n"+
			"*CHOOSE ALL CHATS, GROUPS ONLY, PRIVATE CHATS ONLY, OR TURN IT OFF.*\n\n"+
			"*CURRENT STATUS :❯ ❮ %s ❯*\n\n"+
			"*TYPE ❮ %sAUTOREPLY ON ❯*\n"+
			"*AUTOREPLY ACTIVE EVERYWHERE (GROUPS + INBOX)*\n\n"+
			"*TYPE ❮ %sAUTOREPLY GROUPS ❯*\n"+
			"*AUTOREPLY ACTIVE IN GROUPS ONLY*\n\n"+
			"*TYPE ❮ %sAUTOREPLY INBOX ❯*\n"+
			"*AUTOREPLY ACTIVE IN PRIVATE CHATS ONLY*\n\n"+
			"*TYPE ❮ %sAUTOREPLY OFF ❯*\n"+
			"*AUTOREPLY COMPLETELY OFF*\n\n"+
			"*TO TURN AUTOREPLY OFF FOR A USER*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM ADD @MENTION/NUMBER ❯*\n\n"+
			"*TO TURN AUTOREPLY ON AGAIN FOR A USER*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM DEL @MENTION/NUMBER ❯*\n\n"+
			"*EXCLUDED USERS LIST*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM LIST ❯*", st, prefix, prefix, prefix, prefix, prefix, prefix, prefix))
		return
	case "on", "groups", "inbox", "off":
		s.SetAutoReplyMode(sub)
		arSettingsInvalidate(botJID)
		st := arStatusText(sub)
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY %s 🔰*\n\n"+
			"*THIS SETTING CONTROLS WHERE AUTOMATIC REPLIES ARE ACTIVE.*\n"+
			"*THE BOT WILL FOLLOW THIS MODE FOR NEW INCOMING MESSAGES.*\n\n"+
			"*AUTOREPLY STATUS :❯ ❮ %s ❯*\n\n"+
			"*AUTOREPLY IS NOW IN %s MODE*", strings.ToUpper(sub), st, strings.ToUpper(sub)))
		return
	case "delay":
		delayArg := ""
		if len(args) > 1 {
			delayArg = strings.ToLower(strings.TrimSpace(args[1]))
		}
		if delayArg != "on" && delayArg != "off" {
			delayOn := arSettingsLoad(s).TypingDelay
			dlySt := "🔰 OFF (INSTANT REPLY)"
			if delayOn {
				dlySt = "🔰 ON (5 SEC TYPING BEFORE REPLY)"
			}
			s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY DELAY INFO 🔰*\n\n"+
				"*WHEN ON, THE BOT SHOWS \"TYPING...\" FOR 5 SECONDS BEFORE SENDING THE AI REPLY — FEELS MORE HUMAN.*\n"+
				"*WHEN OFF, THE REPLY IS SENT AS SOON AS IT'S READY — FASTER, NO EXTRA WAIT.*\n\n"+
				"*CURRENT STATUS :❯ ❮ %s ❯*\n\n"+
				"*TYPE ❮ %sAUTOREPLY DELAY ON ❯*\n"+
				"*TYPE ❮ %sAUTOREPLY DELAY OFF ❯*", dlySt, prefix, prefix))
			return
		}
		if delayArg == "on" {
			s.SetAutoReplyDelay("true")
		} else {
			s.SetAutoReplyDelay("false")
		}
		arSettingsInvalidate(botJID)
		body := "*BOT WILL NOW REPLY INSTANTLY, NO TYPING WAIT.*"
		if delayArg == "on" {
			body = "*BOT WILL NOW SHOW 5 SEC TYPING BEFORE EVERY AI REPLY.*"
		}
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY DELAY %s 🔰*\n\n%s", strings.ToUpper(delayArg), body))
		return
	default:
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY COMMAND INFO 🔰*\n\n"+
			"*AUTOREPLY AUTOMATICALLY REPLIES TO INCOMING USER MESSAGES USING AI.*\n"+
			"*IT HELPS YOU ANSWER USERS WHEN YOU ARE BUSY OR AWAY.*\n"+
			"*REPLIES ARE SHORT, SIMPLE, AND LIMITED TO TWO LINES.*\n\n"+
			"*TYPE ❮ %sAUTOREPLY ON ❯*\n"+
			"*AUTOREPLY ON IN GROUPS + INBOX*\n\n"+
			"*TYPE ❮ %sAUTOREPLY GROUPS ❯*\n"+
			"*AUTOREPLY ON IN GROUPS ONLY*\n\n"+
			"*TYPE ❮ %sAUTOREPLY INBOX ❯*\n"+
			"*AUTOREPLY ON IN PRIVATE CHATS ONLY*\n\n"+
			"*TYPE ❮ %sAUTOREPLY OFF ❯*\n"+
			"*AUTOREPLY COMPLETELY OFF*\n\n"+
			"*TYPE ❮ %sAUTOREPLY DELAY ON/OFF ❯*\n"+
			"*TOGGLE THE 5 SEC \"TYPING...\" BEFORE REPLY*\n\n"+
			"*TO TURN AUTOREPLY OFF FOR A USER:*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM ADD 923XXX ❯*\n"+
			"*OR MENTION THE USER IN A GROUP AND WRITE ❮ %sAUTOREPLYPREM ADD ❯*\n\n"+
			"*TO TURN AUTOREPLY ON AGAIN FOR A USER:*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM DEL 923XXX ❯*\n\n"+
			"*TO VIEW EXCLUDED USERS:*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM LIST ❯*", prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix))
		return
	}
}

// ----------------------------------------------------------------------------
// .autoreplyprem command (owner OR group-admin) — texts 0% farak
// ----------------------------------------------------------------------------

func arPremAllowed(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	if !info.IsGroup {
		return false
	}
	if s.IsGroupAdmin(info.Chat, info.Sender) {
		return true
	}
	if info.SenderAlt.Server != "" {
		if s.IsGroupAdmin(info.Chat, info.SenderAlt) {
			return true
		}
	}
	return false
}

func handleAutoReplyPrem(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			s.Reply(info, "🔰 *AUTOREPLYPREM ERROR — TRY AGAIN*")
		}
	}()
	if !arPremAllowed(s, info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR OWNER / GROUP ADMINS 😎*")
		return
	}
	argsText := strings.Join(args, " ")
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
	}
	// target: mention → quoted participant → last number arg (same as Node.js)
	target, _ := resolvePremTarget(s, info, argsText)

	switch sub {
	case "add":
		if target == "" {
			s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY PREM ADD INFO 🔰*\n\n"+
				"*USE THIS COMMAND TO EXCLUDE A USER FROM AUTOREPLY.*\n"+
				"*THE BOT WILL STAY SILENT ONLY FOR THE SELECTED USER.*\n"+
				"*ALL OTHER USERS WILL CONTINUE TO RECEIVE AUTOMATIC REPLIES.*\n\n"+
				"*3 METHODS TO TURN AUTOREPLY OFF FOR A USER*\n\n"+
				"*METHOD 1*\n"+
				"*MENTION ANY USER AND WRITE ❮ %sAUTOREPLYPREM ADD ❯ SO AUTOREPLY WILL NEVER REPLY TO THAT USER*\n\n"+
				"*METHOD 2*\n"+
				"*TYPE ❮ %sAUTOREPLYPREM ADD 923XXX ❯ ❮ FOR INBOX ❯*\n\n"+
				"*METHOD 3*\n"+
				"*TYPE ❮ %sAUTOREPLYPREM ADD @MENTION ❯ ❮ FOR GROUPS ❯*\n\n"+
				"*AUTOREPLY IS NOW OFF FOR THIS USER — ALL OTHER USERS WILL CONTINUE TO RECEIVE REPLIES*", prefix, prefix, prefix))
			return
		}
		arAddExclude(s, target)
		num := stripNonDigits(stripJIDSuffix(target))
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY PREM USER ADDED 🔰*\n\n"+
			"*THIS USER HAS BEEN ADDED TO THE AUTOREPLY EXCLUSION LIST.*\n"+
			"*THE BOT WILL NOT SEND AUTOMATIC REPLIES TO THIS USER.*\n\n"+
			"*NUMBER :❯ ❮ %s ❯*\n\n"+
			"*AUTOREPLY WILL NEVER REPLY TO THIS USER — THE BOT WILL STAY SILENT ON ALL OF THIS USER'S MESSAGES*", num))
		return
	case "remove", "delete", "del":
		if target == "" {
			s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY PREM DEL INFO 🔰*\n\n"+
				"*USE THIS COMMAND TO REMOVE A USER FROM THE AUTOREPLY EXCLUSION LIST.*\n"+
				"*THE BOT WILL START REPLYING TO THAT USER AGAIN.*\n"+
				"*THIS DOES NOT CHANGE AUTOREPLY FOR OTHER USERS.*\n\n"+
				"*3 METHODS TO TURN AUTOREPLY ON AGAIN FOR A USER*\n\n"+
				"*METHOD 1*\n"+
				"*MENTION ANY USER AND WRITE ❮ %sAUTOREPLYPREM DEL ❯ SO AUTOREPLY WILL START REPLYING TO THAT USER AGAIN*\n\n"+
				"*METHOD 2*\n"+
				"*TYPE ❮ %sAUTOREPLYPREM DEL 923XXX ❯ ❮ FOR INBOX ❯*\n\n"+
				"*METHOD 3*\n"+
				"*TYPE ❮ %sAUTOREPLYPREM DEL @MENTION ❯ ❮ FOR GROUPS ❯*\n\n"+
				"*AUTOREPLY WILL NOW BE ON AGAIN FOR THIS USER*", prefix, prefix, prefix))
			return
		}
		arRemoveExclude(s, target)
		num := stripNonDigits(stripJIDSuffix(target))
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY PREM USER DELETED 🔰*\n\n"+
			"*THIS USER HAS BEEN REMOVED FROM THE AUTOREPLY EXCLUSION LIST.*\n"+
			"*THE BOT CAN NOW SEND AUTOMATIC REPLIES TO THIS USER AGAIN.*\n\n"+
			"*NUMBER :❯ ❮ %s ❯*\n\n"+
			"*AUTOREPLY WILL REPLY TO THIS USER AGAIN LIKE IT DOES TO EVERYONE ELSE*", num))
		return
	case "list":
		list := arExcludedRawList(s)
		if len(list) == 0 {
			s.Reply(info, "*🔰 AUTOREPLY: NO EXCLUDED USERS*\n\n"+
				"*NO USERS HAVE BEEN ADDED TO THE AUTOREPLYPREM EXCLUSION LIST*\n"+
				"*AUTOREPLY IS READY TO WORK FOR ALL USERS ALLOWED BY THE CURRENT MODE.*")
			return
		}
		var b strings.Builder
		for i, j := range list {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.ReplaceAll(j, "@s.whatsapp.net", "")))
		}
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLY EXCLUDED USERS 🔰*\n\n"+
			"*THESE USERS WILL NOT RECEIVE AUTOMATIC REPLIES.*\n\n"+
			"%s\n"+
			"*TOTAL :❯ %d*", strings.TrimRight(b.String(), "\n"), len(list)))
		return
	default:
		s.Reply(info, fmt.Sprintf("*🔰 AUTOREPLYPREM COMMAND INFO 🔰*\n\n"+
			"*THIS COMMAND MANAGES USERS WHO SHOULD NOT RECEIVE AUTOREPLIES.*\n"+
			"*ADD EXCLUDES A USER, DEL ALLOWS REPLIES AGAIN, AND LIST SHOWS EXCLUDED USERS.*\n"+
			"*USE A NUMBER OR MENTION A USER IN A GROUP.*\n\n"+
			"*ADD: TURN AUTOREPLY OFF FOR A USER*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM ADD 923XXX ❯*\n"+
			"*OR MENTION THE USER AND WRITE ❮ %sAUTOREPLYPREM ADD ❯*\n\n"+
			"*DEL: TURN AUTOREPLY ON AGAIN FOR A USER*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM DEL 923XXX ❯*\n"+
			"*OR MENTION THE USER AND WRITE ❮ %sAUTOREPLYPREM DEL ❯*\n\n"+
			"*LIST: VIEW EXCLUDED USERS*\n"+
			"*TYPE ❮ %sAUTOREPLYPREM LIST ❯*", prefix, prefix, prefix, prefix, prefix))
		return
	}
}

func init() {
	Register(Command{
		Name:      "autoreply",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO SET AUTO REPLIES FOR THE BOT. USE IT WITH YOUR WORD AND REPLY TEXT.",
		OwnerOnly: true,
		Run:       handleAutoReply,
	})
	Register(Command{Name: "autoreplymode", OwnerOnly: true, Hidden: true, Run: handleAutoReply})
	Register(Command{
		Name:     "autoreplyprem",
		Category: "OWNER & SYSTEM",
		Desc:     "THIS COMMAND IS USED TO SET PREMIUM AUTO REPLY SETTINGS FOR THE BOT.",
		Run:      handleAutoReplyPrem,
	})
	Register(Command{Name: "autoreplypremium", Hidden: true, Run: handleAutoReplyPrem})
}
