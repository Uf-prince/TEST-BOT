package goldcmds

// ============================================================================
// GOLD-MD — LOCALIZED COMMAND NAMES  (.botlanguage ke saath)
// File: cmdlocalize.go
// ============================================================================
// OWNER ORDER: jab bot ki language set ho, to us language ke command names BHI
// chalne chahiye — jaise Urdu me .مینو, Hindi me .मेनू. Magar:
//   * English command names (.ping, .menu) KABHI band nahi hote — har waqt
//     chalte rehte hain (kisi bhi language me, kisi bhi user ke liye).
//   * Localized names sirf EK ALIAS hote hain — original English handler hi
//     chalta hai, to owner-only / bancmd / cmdowner / mode sab checks waise hi
//     lagte hain (koi bypass nahi).
//
// FLOW:
//   1. Command ki language set hui (settings:<botJID>.botlanguage).
//   2. Command names Redis cache me translate ho kar localize:<CODE> field me
//      save hote hain (server-side cache; ~50 names in one HTTP call).
//   3. Dispatch pe typed token agar localized map me hai -> canonical English
//      name resolve -> normal dispatch (handler.go CmdLocalizeResolve).
//   4. Menu me ENGLISH canonical naam ke saath localized naam bhi dikhta hai,
//      taake user ko pata rahe wo .مینو bhi type kar sakta hai.
//
// SAFETY: translation HTTP call kabhi dispatch ko block nahi karta — resolve
// sirf cache se padhta hai; cache miss pe background build chalti hai aur
// pehle English naam se kaam chalta rehta hai.
// ============================================================================

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	clMapField = "cmdlocalize" // "<lang>:<code1:name1|code2:name2|...>"
)

// clEntry is one localized command name for the current language.
type clEntry struct {
	Local string // localized typed token (lower-cased, e.g. "مینو")
	Canon string // canonical English command name (e.g. "menu")
}

type clState struct {
	Lang  string
	ByLoc map[string]string // localized -> canonical
}

var (
	clMu    sync.Mutex
	clCache = map[string]*clState{}
	clAt    = map[string]time.Time{}

	// clBuildGuard dedups concurrent builds per bot.
	clBuilding = map[string]bool{}

	// clTranslator is attached by the main package so this file does not need
	// the translation transport (keeps gold-cmds free of an extra HTTP client).
	clTranslator func(ctx context.Context, text, target string) (string, error)
)

// CmdLocalizeAttachTranslator lets the main package supply the translator.
func CmdLocalizeAttachTranslator(fn func(ctx context.Context, text, target string) (string, error)) {
	clTranslator = fn
}

// clCanonicalNames returns the command names worth localizing: every VISIBLE
// gold-cmds command (hidden aliases add noise, not value) plus the core command
// names the main package registered. Lower-cased, de-duplicated, sorted.
func clCanonicalNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.ToLower(strings.TrimSpace(n))
		// A command token cannot contain a space, so a name like "dance
		// diffusion" is not typeable and has nothing to localize.
		if n == "" || seen[n] || n == "botlanguage" || strings.ContainsAny(n, " \t") {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, c := range Commands() {
		if c.Hidden {
			continue
		}
		add(c.Name)
	}
	if cmdNameKnownHook != nil {
		for _, n := range cmdNameKnownHook() {
			add(n)
		}
	}
	return out
}

// clParse decodes the stored field "lang:code|local:canon|..." into a state.
func clParse(raw string) *clState {
	st := &clState{ByLoc: map[string]string{}}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return st
	}
	head, body, ok := strings.Cut(raw, ":")
	if !ok {
		return st
	}
	st.Lang = strings.TrimSpace(head)
	for _, row := range strings.Split(body, "|") {
		l, c, ok := strings.Cut(row, "=")
		if !ok {
			continue
		}
		l = strings.ToLower(strings.TrimSpace(l))
		c = strings.ToLower(strings.TrimSpace(c))
		if l == "" || c == "" {
			continue
		}
		st.ByLoc[l] = c
	}
	return st
}

// clEncode writes the state back into the single stored field.
func clEncode(lang string, pairs [][2]string) string {
	rows := make([]string, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, p[0]+"="+p[1])
	}
	return lang + ":" + strings.Join(rows, "|")
}

// clLoad returns the cached localized map for this bot's current language.
// On a cache miss it kicks off a background build and returns an empty state,
// so the very first command after a language change still works (in English)
// while the localized names are being prepared.
func clLoad(s SessionBridge) *clState {
	botJID := s.GetJID()
	clMu.Lock()
	if st, ok := clCache[botJID]; ok && time.Since(clAt[botJID]) < time.Minute {
		clMu.Unlock()
		return st
	}
	clMu.Unlock()

	raw := s.GetStatusSetting(clMapField, "")
	st := clParse(raw)

	clMu.Lock()
	clCache[botJID] = st
	clAt[botJID] = time.Now()
	clMu.Unlock()

	// If the stored map does not match the bot's current language, rebuild it.
	lang := strings.TrimSpace(s.GetBotLanguageSetting(""))
	if st.Lang != lang {
		clBuildAsync(s, botJID, lang)
	}
	return st
}

// clBuildAsync prepares "cmdlocalize" in the background: translate all command
// names in ONE call (newline-separated), then persist. Failures are silent so
// English names always remain the working path.
func clBuildAsync(s SessionBridge, botJID, lang string) {
	if clTranslator == nil || lang == "" || lang == BotLanguageName {
		return
	}
	clMu.Lock()
	if clBuilding[botJID] {
		clMu.Unlock()
		return
	}
	clBuilding[botJID] = true
	clMu.Unlock()

	go func() {
		defer func() {
			clMu.Lock()
			delete(clBuilding, botJID)
			clMu.Unlock()
		}()
		names := clCanonicalNames()
		if len(names) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		// English command names are permanent: never let a localized alias take
		// over a real (or hidden alias) command name, or .poll could start
		// routing to whatever localized string happened to translate to "poll".
		reserved := map[string]bool{}
		for _, c := range Commands() {
			reserved[strings.ToLower(strings.TrimSpace(c.Name))] = true
		}
		for _, n := range names {
			reserved[n] = true
		}
		// The Google endpoint rejects very large `q` values (a single request
		// with ~2700 command names returns HTTP 400), so translate in chunks
		// and stitch the result. Any chunk that fails or comes back with a
		// different line count is skipped — those names simply stay English.
		const chunk = 150
		pairs := make([][2]string, 0, len(names))
		seenLocal := map[string]bool{}
		for start := 0; start < len(names); start += chunk {
			end := start + chunk
			if end > len(names) {
				end = len(names)
			}
			part := names[start:end]
			out, err := clTranslator(ctx, strings.Join(part, "\n"), lang)
			if err != nil {
				continue
			}
			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			if len(lines) != len(part) {
				continue // line count drift -> keep English for this chunk
			}
			for i, canon := range part {
				local := strings.ToLower(strings.TrimSpace(lines[i]))
				// Drop anything unusable: empty, unchanged (meaningless),
				// multi-word (a command token cannot contain spaces), a
				// collision with an existing English/hidden command name, or a
				// duplicate local form already claimed by an earlier command
				// (otherwise the dispatch target would be nondeterministic).
				if local == "" || local == canon || strings.ContainsAny(local, " \t") ||
					reserved[local] || seenLocal[local] {
					continue
				}
				seenLocal[local] = true
				pairs = append(pairs, [2]string{local, canon})
			}
		}
		if len(pairs) == 0 {
			return
		}
		s.SetStatusSetting(clMapField, clEncode(lang, pairs))
		clMu.Lock()
		delete(clCache, botJID)
		clMu.Unlock()
	}()
}

// clResolveLocal is exported for tests: localized typed token -> canonical name.
func clResolveLocal(st *clState, typed string) (string, bool) {
	if st == nil {
		return typed, false
	}
	if c, ok := st.ByLoc[strings.ToLower(strings.TrimSpace(typed))]; ok && c != "" {
		return c, true
	}
	return typed, false
}

// CmdLocalizeResolve maps a typed command name to the English canonical command
// when it is the bot's localized name for it. Command NAMES themselves are never
// translated in dispatch terms — this only translates the TYPED token back to
// English so the normal handler runs. Returns the name unchanged when nothing
// matches (i.e. normal English dispatch).
func CmdLocalizeResolve(s SessionBridge, typed string) (string, bool) {
	return clResolveLocal(clLoad(s), typed)
}

// CmdLocalizeLookup returns the localized name for a canonical English command,
// or "" when there is none. Used by the menu so it can show both.
func CmdLocalizeLookup(s SessionBridge, canon string) string {
	st := clLoad(s)
	canon = strings.ToLower(strings.TrimSpace(canon))
	for local, c := range st.ByLoc {
		if c == canon {
			return local
		}
	}
	return ""
}

// LocalizedCommandList returns every working localized command name for this
// bot's current language as (canonical English, localized) pairs, sorted by the
// English name. Empty when no language is set or nothing has been prepared yet.
func LocalizedCommandList(s SessionBridge) [][2]string {
	st := clLoad(s)
	out := make([][2]string, 0, len(st.ByLoc))
	for local, canon := range st.ByLoc {
		out = append(out, [2]string{canon, local})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
