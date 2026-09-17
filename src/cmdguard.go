package main

// ============================================================================
// GOLD-MD — PER-SESSION / PER-USER "LATEST WINS" COMMAND GUARD
// File: cmdguard.go
// ============================================================================
// OWNER ORDER:
//   "1 user ne bot pair kia huwa us ne koi b video tt fb ig tg koi b
//    downloader command chalaya wo abhi chal rha download ho rhe compressor
//    shield compress kr rhe hai lekin user ne foran new request bhej de ab
//    wo ho jye ge render free trailer ram memory disk full to isko set kro
//    guard laga do ek user ne koi b cmnd chalaya wo chal rha hai background
//    me us ne foran new request bheji pichla sab kam band naye pe kam shuru"
//
//   "Aur ye b krna — Bot A pair 1 ne chalaya video cmnd, same usy time pair
//    bot 2 ne chalaya thek hai. Lekin user 1 ki request per download chal rha
//    lkkin usy time usy ne foran dusra cmnd chalaya lekin user 2 ki ek
//    request jary hai kam ho rha to user 2 ki chalne do. Matlab bot 1 A ka
//    alag system bot b w ka alag system ok...."
//
//   "Matlab per session system hona chahye me bad me 10 max pair b kr skta
//    max pair 50 b kr skta mere mrzi ye system per session kam krna chahye
//    hr session ka apna alag system yhy wala ok ??....."
//
// DESIGN (exactly what the owner asked):
//   • The guard is keyed by (SESSION JID, USER JID).
//       - SESSION JID  = the paired WhatsApp number (bot 1 / bot 2 / ...).
//         Every session has its OWN independent guard map — pair 1 and
//         pair 2 NEVER interfere. 10 pairs or 50 pairs, all independent.
//       - USER JID     = the sender of the command.
//   • When a user sends a NEW command on the SAME session, the PREVIOUS
//     in-flight command of THAT SAME user on THAT SAME session is cancelled
//     instantly (its context is cancelled → every HTTP download / ffmpeg
//     exec tied to it aborts, temp files are cleaned up by the pipeline's
//     deferred removeTempFile).
//   • A DIFFERENT user's in-flight command is NEVER touched — user 2 keeps
//     downloading while user 1 starts a fresh request.
//   • A DIFFERENT session's in-flight command is NEVER touched — bot 2's
//     work is completely isolated from bot 1's.
//
// 0% SPEED / RAM / DISK IMPACT:
//   • One sync.Map (session → guard) + one small map per session.
//   • Per command: one map lookup + one context.WithCancel + one map write.
//   • No polling, no sleep loop, no goroutine spawned by the guard itself.
//   • Entries are deleted the moment a command finishes (release func), so
//     the maps stay tiny (at most one live entry per active user per session).
//   • A command that finishes in 5s behaves EXACTLY as before — the guard
//     only ever does work when a newer command actually supersedes an older
//     one (the exact case the owner wants handled).
// ============================================================================

import (
	"context"
	"sync"
)

// cmdGuardEntry is one in-flight command for a single (session, user) pair.
type cmdGuardEntry struct {
	cancel context.CancelFunc
}

// cmdGuard holds the in-flight commands for ONE session, keyed by user JID.
// Each session gets its own cmdGuard instance → full isolation between pairs.
type cmdGuard struct {
	mu      sync.Mutex
	entries map[string]*cmdGuardEntry
}

// sessionGuards maps session JID → *cmdGuard. sync.Map keeps the hot path
// lock-free for reads (the common case: a session that already has a guard).
var sessionGuards sync.Map

// guardForSession returns the cmdGuard for a session, creating it lazily.
func guardForSession(sessionJID string) *cmdGuard {
	if v, ok := sessionGuards.Load(sessionJID); ok {
		return v.(*cmdGuard)
	}
	g := &cmdGuard{entries: make(map[string]*cmdGuardEntry)}
	actual, _ := sessionGuards.LoadOrStore(sessionJID, g)
	return actual.(*cmdGuard)
}

// BeginCmdGuard starts a new guarded command for (sessionJID, userJID).
//
// It cancels any PREVIOUS in-flight command of the SAME user on the SAME
// session (latest-wins), then returns:
//   - ctx:     a cancellable context the command pipeline must honour
//     (HTTP requests + ffmpeg execs already use it).
//   - release: a func the caller MUST defer — it removes this command's
//     entry (only if it is still the current one) and cancels its context.
//
// Commands of OTHER users on the same session, and commands of ANY user on
// OTHER sessions, are completely untouched.
func BeginCmdGuard(sessionJID, userJID string) (context.Context, func()) {
	g := guardForSession(sessionJID)

	g.mu.Lock()
	// Latest-wins: kill the previous command of this same user on this
	// same session. A different user's entry is a different map key and is
	// never touched.
	if prev := g.entries[userJID]; prev != nil {
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &cmdGuardEntry{cancel: cancel}
	g.entries[userJID] = e
	g.mu.Unlock()

	release := func() {
		g.mu.Lock()
		// Only delete if WE are still the current entry — if a newer
		// command already replaced us, leave the newer entry alone.
		if cur := g.entries[userJID]; cur == e {
			delete(g.entries, userJID)
		}
		g.mu.Unlock()
		cancel()
	}
	return ctx, release
}

// dropSessionGuard removes a session's guard entirely (called when a session
// is cleaned up / unpaired) so the sync.Map never accumulates dead sessions.
func dropSessionGuard(sessionJID string) {
	sessionGuards.Delete(sessionJID)
}
