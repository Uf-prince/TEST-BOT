package goldcmds

// ============================================================================
// GOLD-MD — TRANSLATION CACHE HOOKS  (.botlanguage speed)
// File: trtcache.go
// ============================================================================
// OWNER ORDER: bar bar translate karne se bot slow hota hai. Ek baar translate
// ho, phir cache se serve ho — RAM → disk → Storj.
//
// Cache is LINE-level, not reply-level, because most replies are menus whose
// uptime/counts change every minute. The header lines, descriptions and guidance
// lines repeat across every menu and every user, so those are the ones worth
// caching — and they are exactly the lines that get translated (lines carrying a
// command token are already passed through verbatim by menutrans.go).
//
// The main package owns the storage (RAM map → nexstore disk → Storj setting);
// this file only declares the lookup contract and falls back to the live
// translator when no cache is attached.
// ============================================================================

import "context"

var (
	tcGet func(botJID, lang, line string) (string, bool)
	tcPut func(botJID, lang, line, out string)
)

// tcCtxKey carries the bot JID through the translation call so the cache can be
// keyed per bot without changing TranslatePreservingCommandTokens' signature
// (its tests call it directly).
type tcCtxKeyT struct{}

var tcCtxKey tcCtxKeyT

// TrtCacheWithBot tags a context with the bot JID the translation belongs to.
func TrtCacheWithBot(ctx context.Context, botJID string) context.Context {
	if botJID == "" {
		return ctx
	}
	return context.WithValue(ctx, tcCtxKey, botJID)
}

func tcBotFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(tcCtxKey).(string); ok {
		return v
	}
	return ""
}

// TrtCacheAttach wires the persistent translation cache. get must be cheap
// (RAM only) because it runs on the reply hot path; put may persist.
func TrtCacheAttach(get func(botJID, lang, line string) (string, bool), put func(botJID, lang, line, out string)) {
	tcGet, tcPut = get, put
}

// TrtCacheLookup exposes the attached cache for tests.
func TrtCacheLookup(botJID, lang, line string) (string, bool) {
	if tcGet == nil {
		return "", false
	}
	return tcGet(botJID, lang, line)
}
