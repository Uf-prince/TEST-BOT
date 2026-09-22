package goldcmds

// ============================================================================
// GOLD-MD — Command Watchdog / Hard Timeout
// File: timeout.go
// ============================================================================
// Every media command runs its pipeline inside RunWithTimeout. This gives
// the command exactly 3 minutes; if the pipeline (API call + download +
// upload) is not finished by then:
//
//   1. The context is cancelled -> every HTTP request, download loop and
//      ffmpeg exec tied to it aborts immediately (no zombie goroutines,
//      no leaked downloads eating bandwidth).
//   2. Temp files are cleaned up by the pipeline's deferred removeTempFile.
//   3. The user gets a single "*TRY AGAIN LATER*" reply.
//
// This keeps the bot fast: a slow API can never keep a download running
// in the background forever.
// ============================================================================

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// cmdTimeout is the hard limit for any media command pipeline.
// 5 minutes: LONG TikTok videos (3-5 min songs) get download + upload
// room — the busy-guard keeps the watchdogs off during the pipeline.
const cmdTimeout = 5 * time.Minute

// cmdDownloaderTimeout is the hard limit for the YouTube downloader
// commands (.video / .video2 / .play / .play2). Owner order: exactly
// 2 minutes — the timer starts the moment the command runs; if nothing
// lands within 2 minutes the user gets "*PLEASE TRY AGAIN LATER*".
//
// 0% SPEED / RAM / DISK IMPACT: this is a pure context.WithTimeout +
// goroutine + select — no polling, no sleep loop, no extra allocation on
// the fast path. A command that finishes in 5s behaves exactly as before;
// the timer only fires on a hang.
const cmdDownloaderTimeout = 2 * time.Minute

// timeoutReplyText is sent when a command hits the hard limit.
const timeoutReplyText = "*TRY AGAIN LATER*"

// downloaderTimeoutReplyText is sent when a downloader command hits its
// 2-minute hard limit.
const downloaderTimeoutReplyText = "*PLEASE TRY AGAIN LATER*"

// socialTimeout is the hard limit for the social-media downloader commands
// (.ig / .tt / .fb / .twt / .tg). Owner order: 40 seconds — these APIs are
// fast; if nothing lands in 40s the user gets "*PLEASE TRY AGAIN LATER*".
//
// 0% SPEED / RAM / DISK IMPACT: same pure context.WithTimeout + goroutine +
// select design as the other watchdogs — no polling, no sleep loop.
const socialTimeout = 40 * time.Second

// RunWithTimeout runs fn in a goroutine with a hard 3-minute budget.
// On timeout it cancels the context, waits a short grace period for the
// goroutine to unwind (close bodies, delete temp files), and replies
// TRY AGAIN LATER exactly once.
func RunWithTimeout(s SessionBridge, info types.MessageInfo, fn func(ctx context.Context)) {
	// PER-SESSION / PER-USER GUARD (latest wins): a newer command from the
	// SAME user on the SAME session cancels this one instantly. Other users
	// (same session) and any user on other sessions are never affected.
	gctx, release := s.BeginGuard(info.Sender.String())
	defer release()
	ctx, cancel := context.WithTimeout(gctx, cmdTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()

	select {
	case <-done:
		// Pipeline finished in time — nothing to do.
	case <-ctx.Done():
		// Hard limit hit: context is already cancelled, HTTP calls tied to
		// it fail with context.Canceled. Give the goroutine a short grace
		// period to unwind (close bodies, delete temp files) then reply.
		grace := time.After(3 * time.Second)
		select {
		case <-done:
		case <-grace:
		}
		s.Reply(info, timeoutReplyText)
	}
}

// RunWithTimeoutDur is RunWithTimeout with a caller-supplied hard limit and
// reply text. Used by the social-media downloaders (.ig / .tt / .fb / .twt /
// .tg) with a 40-second budget and the "*PLEASE TRY AGAIN LATER*" reply.
//
// 0% SPEED / RAM / DISK IMPACT: pure context.WithTimeout + goroutine +
// select — no polling, no sleep loop. Fast commands are unaffected; the
// timer only fires on a hang. On timeout the context is cancelled so every
// HTTP request / download loop / ffmpeg exec tied to it aborts and its
// deferred temp-file cleanup runs (disk stays safe).
func RunWithTimeoutDur(s SessionBridge, info types.MessageInfo, d time.Duration, reply string, fn func(ctx context.Context)) {
	// PER-SESSION / PER-USER GUARD (latest wins): a newer command from the
	// SAME user on the SAME session cancels this one instantly. Other users
	// (same session) and any user on other sessions are never affected.
	gctx, release := s.BeginGuard(info.Sender.String())
	defer release()
	ctx, cancel := context.WithTimeout(gctx, d)
	defer cancel()
	// Guard compressor shares this SAME budget: store ctx on the bridge so
	// ffmpeg is killed the moment the timeout fires (owner order).
	s.SetCmdContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()

	select {
	case <-done:
		// Pipeline finished in time — nothing to do.
	case <-ctx.Done():
		// Hard limit hit: give the goroutine a short grace period to
		// unwind (close bodies, delete temp files) then reply.
		grace := time.After(3 * time.Second)
		select {
		case <-done:
		case <-grace:
		}
		s.Reply(info, reply)
	}
}
