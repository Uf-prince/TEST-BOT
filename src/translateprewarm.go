package main

// ═══════════════════════════════════════════════════════════════════════════
//   BULK TRANSLATION PREWARM  (owner order — "ek bar poora bot translate")
//
//   OWNER ORDER: ".botlanguage {language} likha foran us jid k lie PURE bot ko
//   ek bar translate krwa ker cache me safe krwa lo".
//
//   Jab owner language set karta hai, hum SAARE menu renders (har category,
//   .logo, .font, .game, .equalizer, .botstyle) aur startup card ek baar render
//   karte hain aur unki translatable lines translate karwa kar cache me daal
//   dete hain. Uske baad har reply RAM cache se aati hai — 0 HTTP.
//
//   Sirf translatable lines bheji jati hain (menutrans.go ke rule: command-token
//   wali lines verbatim rehti hain, aur unhe translate karna hi nahi hai).
//   Isliye cache exactly wahi lines hold karta hai jo asal replies me aati hain.
//
//   Background me chalta hai — owner ko reply foran milta hai, aur English
//   command names kabhi band nahi hote.
// ═══════════════════════════════════════════════════════════════════════════

import (
	"context"
	"strings"
	"sync"
	"time"

	goldcmds "gold-md/gold-cmds"
)

var pwOnce sync.Map // botJID+lang -> bool, dedups concurrent prewarms

// prewarmAllTranslations renders every menu once and caches its translated
// lines for this bot+language. Runs in the background.
func prewarmAllTranslations(s *Session, lang string) {
	if s == nil || lang == "" || lang == goldcmds.BotLanguageName {
		return
	}
	id := s.JID + "\x00" + lang
	if _, busy := pwOnce.LoadOrStore(id, true); busy {
		return
	}

	go func() {
		defer pwOnce.Delete(id)
		if s.Manager != nil && s.Manager.Redis != nil {
			rcBind(s.JID, s.Manager.Redis)
		}
		rcWarm(s.JID, lang)

		texts := collectPrewarmTexts(s)
		ctx, cancel := context.WithTimeout(goldcmds.TrtCacheWithBot(context.Background(), s.JID), 4*time.Minute)
		defer cancel()
		for _, t := range texts {
			if ctx.Err() != nil {
				break
			}
			// TranslatePreservingCommandTokens already skips token lines and
			// writes every translated line straight into the cache.
			_, _ = goldcmds.TranslatePreservingCommandTokens(ctx, t, lang)
		}
		rcFlush(id)
		InfoLog("PREWARM: %s (%s) — %d lines cached", s.JID, lang, rcCount(s.JID, lang))
	}()
}

// collectPrewarmTexts renders every menu with fixed sample values, so the static
// lines (headers, descriptions, guidance) get cached. Uptime/counts differ per
// call, so only the stable lines end up reused — which is exactly what we want.
func collectPrewarmTexts(s *Session) []string {
	const (
		botNum   = "000000000000"
		ownerNum = "000000000000"
		uptime   = "1H 2M"
		pushName = "OWNER"
		botName  = "GOLD-MD"
	)
	prefix := "."
	if s.Manager != nil && s.Manager.Redis != nil {
		if p := strings.TrimSpace(s.Manager.Redis.GetPrefix(s.JID, ".")); p != "" {
			prefix = p
		}
	}
	st := menuStyleFor(s, "")
	view := &goldcmds.CmdNameView{Renames: map[string]string{}}
	if s.Manager != nil && s.Manager.Redis != nil {
		view = goldcmds.CmdNameViewFor(&bridge{s: s})
	}
	sessCount := 1
	if s.Manager != nil {
		sessCount = s.Manager.Count()
	}

	texts := []string{
		buildLogoMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, st),
		buildFontMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, st),
		buildGameMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, st),
		buildEqualizerMenu(botNum, ownerNum, uptime, prefix, sessCount, st),
		buildBotStyleMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, st),
		buildCategoryMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, "", st),
	}
	for _, cat := range goldcmds.Categories() {
		texts = append(texts, buildCategoryMenu(botNum, ownerNum, uptime, prefix, pushName, botName, sessCount, view, cat, st))
	}
	return texts
}

// prewarmOnLanguageSet is called right after .botlanguage set succeeds.
func prewarmOnLanguageSet(s *Session, lang string) { prewarmAllTranslations(s, lang) }
