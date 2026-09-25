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

func aimSetEnabled(s SessionBridge, on bool) {
	v := "off"
	if on {
		v = "on"
	}
	s.SetPresenceSetting(aimModeField, v)
}

func aimPrefixWord(s SessionBridge) string {
	return strings.TrimSpace(s.GetPresenceSetting(aimPrefixField, ""))
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
		out = append(out, aimCorpusEntry{Command: name, Description: c.Desc})
	}
	for _, name := range cmdNameKnownNames() {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, aimCorpusEntry{Command: name})
	}
	return out
}

var aimEscapeRe = regexp.MustCompile(`[.*+?^${}()|\[\]\\]`)

func aimEscapeRegexLiteral(s string) string {
	return aimEscapeRe.ReplaceAllString(s, `\$0`)
}

// aimResolve is the Go port of BilalAiResolveCommandFromMessage: ONE AI call,
// then the JS-level hard gates (corpus membership + group-only scope). Returns
// ok=false on NO_COMMAND_FOUND or any gate failure.
func aimResolve(userMessage string, corpus []aimCorpusEntry, isGroupChat bool) (string, bool) {
	if strings.TrimSpace(userMessage) == "" || len(corpus) == 0 {
		return "", false
	}
	chatTypeLine := "CURRENT CHAT TYPE: INBOX/PRIVATE DM (yeh message kisi individual ki PRIVATE chat se aaya hai — koi group nahi hai)"
	if isGroupChat {
		chatTypeLine = "CURRENT CHAT TYPE: GROUP (yeh message ek WhatsApp GROUP se aaya hai)"
	}
	var b strings.Builder
	for i, c := range corpus {
		if i > 0 {
			b.WriteString("\n")
		}
		if c.Description != "" {
			b.WriteString(c.Command + " — " + c.Description)
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
	cleaned := strings.TrimSpace(strings.TrimLeft(raw, ".!/#$%&*;("))
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
// texts (verbatim from Node.js pair.js; 👑 → 🔰)
// ----------------------------------------------------------------------------

func aimOwnerOnlyText() string {
	return `*❌ THIS IS AN OWNER COMMAND*

*SIRF BOT KE OWNER KO YEH COMMAND USE KARNE KI IJAZAT HAI 😎*`
}

func aimPrefixSetText(pw string) string {
	return `*🔰 AI MODE PREFIX SET 🔰*

*YOU AI MODE PREFIX ❮ ` + pw + ` ❯*

*HOW TO USE*
DON'T TYPE YOUR PREFIX EG . , ? @ DON'T TYPE YOUR MAIN PREFIX OK TYPE SIMPLE TEXT WITH *AIMODE PREFIX* SAME LIKE THAT
*FOR EXAMPLES SEEE*
*` + pw + ` CHECK BOT SPEED*
*` + pw + ` SHOW BOT COMMANDS*
*` + pw + ` CHECK BOT UPTIME*
*` + pw + ` DOWNLOAD VIDEO FROM FACEBOOK LINK : (PASTE LINK)*
*` + pw + ` ACTIVATE THE STATUS*
*` + pw + ` (ASK TO RUN COMMAND)*

STILL CONFUSED ? SEE THE COMPLETE AIMODE FULL GUIDANCE VIDEO
https://youtu.be/grZBRtnebgs?is=jYy2wtT5Voxn3XYu`
}

func aimGuideText(prefix, curEnabled, curPrefixWord string) string {
	pw := curPrefixWord
	if pw == "" {
		pw = "AI"
	}
	state := "OFF"
	if strings.EqualFold(curEnabled, "on") {
		state = "ON"
	}
	shown := curPrefixWord
	if shown == "" {
		shown = "NOT SET"
	}
	return `*🔰 AI MODE COMMAND GUIDE 🔰*

*TYPE ❮` + prefix + `AIMODE ON❯ TO ACTIVATE*
*TYPE ❮` + prefix + `AIMODE OFF❯ TO DE-ACTIVATE*

*TYPE ❮` + prefix + `AIMODE PREFIX (NEW PREFIX)❯*
*SET YOUR OWN PREFIX *
*SAME LIKE THAT EXAMPLES*
*TYPE ❮ ` + prefix + `AIMODE PREFIX KING ❯*
*TYPE ❮ ` + prefix + `AIMODE PREFIX BOSS ❯*
*TYPE ❮ ` + prefix + `AIMODE PREFIX BILAL ❯*
*TYPE ❮ ` + prefix + `AIMODE PREFIX YOUR NAME ❯*
TO SET AI MODE PREFIX

*NOW AI MODE IS ❮ ` + state + ` ❯*

*YOU AI MODE PREFIX ❮ ` + shown + ` ❯*

*HOW TO USE*
DON'T TYPE YOUR PREFIX EG . , ? @ DON'T TYPE YOUR MAIN PREFIX OK TYPE SIMPLE TEXT WITH *AIMODE PREFIX* SAME LIKE THAT
*FOR EXAMPLES SEEE*
*` + pw + ` CHECK BOT SPEED*
*` + pw + ` SHOW BOT COMMANDS*
*` + pw + ` CHECK BOT UPTIME*
*` + pw + ` DOWNLOAD VIDEO FROM FACEBOOK LINK : (PASTE LINK)*
*` + pw + ` ACTIVATE THE STATUS*
*` + pw + ` (ASK TO RUN COMMAND)*

*STILL CONFUSED ? SEE THE COMPLETE AIMODE FULL GUIDANCE VIDEO ON YOUTUBE*
https://youtu.be/grZBRtnebgs?is=jYy2wtT5Voxn3XYu`
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
			msg := "🤖 *AI MODE TURNED " + strings.ToUpper(action) + " FOR ALL CHATS/GROUPS (GLOBAL).*"
			if action == "on" {
				msg += "\n\n*YOU CAN NOW TYPE NORMAL SENTENCES IN ANY CHAT/GROUP AND THE AI WILL FIND AND RUN THE RIGHT COMMAND FOR YOU.*"
			}
			s.Reply(info, msg)
		case "status":
			cur, curPfx := aimEnabled(s), aimPrefixWord(s)
			state := "OFF"
			if cur {
				state = "ON"
			}
			line := "No custom wake-word set — AI Mode reads every plain message."
			if curPfx != "" {
				line = "Custom wake-word: *" + curPfx + "*"
			}
			s.Reply(info, "🤖 AI Mode is currently *"+state+"* (yeh har chat/group mein apply hota hai).\n"+line)
		case "prefix":
			if len(tokens) < 2 {
				if curPfx := aimPrefixWord(s); curPfx != "" {
					s.Reply(info, "Current AI Mode wake-word is *"+curPfx+"*.\n\nTo change it: *"+prefix+"aimode prefix <newword>*\nExample: "+prefix+"aimode prefix BILAL")
				} else {
					s.Reply(info, "No wake-word is set right now — AI Mode reacts to every plain message.\n\nTo set one: *"+prefix+"aimode prefix <yourword>*\nExample: "+prefix+"aimode prefix BILAL")
				}
			} else {
				newWord := tokens[1]
				s.SetPresenceSetting(aimPrefixField, newWord)
				s.Reply(info, aimPrefixSetText(strings.ToUpper(newWord)))
			}
		default:
			s.Reply(info, aimGuideText(prefix, s.GetPresenceSetting(aimModeField, "off"), aimPrefixWord(s)))
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
			resolved, ok = aimResolve(body2, corpus, isGroup)
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
			s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
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
