package goldcmds

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// GOLD-MD — Command Error Cross-Fallback Messages
// File: failmsg.go
// ============================================================================
// When a downloader command fails anywhere in its pipeline (search, fetch,
// download, convert or send), the user gets a clean error reply that points
// them to the alternative command:
//
//   .video  fails -> *VIDEO COMMAND ERROR*  + PLEASE TRY ❮ VIDEO2 ❯ COMMAND
//   .video2 fails -> *VIDEO2 COMMAND ERROR* + PLEASE TRY ❮ VIDEO ❯ COMMAND
//   .play   fails -> *PLAY COMMAND ERROR*   + PLEASE TRY ❮ PLAY2 ❯ COMMAND
//   .play2  fails -> *PLAY2 COMMAND ERROR*  + PLEASE TRY ❮ PLAY ❯ COMMAND
// ============================================================================

// cmdFailMsg builds the cross-fallback error text for a failed command.
func cmdFailMsg(failed, suggest string) string {
	return fmt.Sprintf("*%s COMMAND ERROR*\n\n*PLEASE TRY ❮ %s ❯ COMMAND*", failed, suggest)
}

// videoCmdError is the failure reply for the .video command.
func videoCmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("VIDEO2", "VIDEO"))
}

// video2CmdError is the failure reply for the .video2 command.
func video2CmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("VIDEO", "VIDEO2"))
}

// playCmdError is the failure reply for the .play command.
func playCmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("PLAY2", "PLAY"))
}

// play2CmdError is the failure reply for the .play2 command.
func play2CmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("PLAY", "PLAY2"))
}

// video3CmdError is the failure reply for the .video3 (raw) command.
func video3CmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("VIDEO3", "VIDEO"))
}

// play3CmdError is the failure reply for the .play3 (raw) command.
func play3CmdError(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, cmdFailMsg("PLAY3", "PLAY"))
}

// RunWithTimeoutCmd is RunWithTimeout with a downloader-specific 2-minute
// hard limit and reply. Used by the YouTube downloader commands
// (.video / .video2 / .play / .play2): the timer starts the moment the
// command runs; if nothing lands within 2 minutes the user gets
// "*PLEASE TRY AGAIN LATER*".
//
// 0% SPEED / RAM / DISK IMPACT: pure context.WithTimeout + goroutine +
// select — no polling, no sleep loop. Fast commands are unaffected; the
// timer only fires on a hang. On timeout the context is cancelled so every
// HTTP request / download loop / ffmpeg exec tied to it aborts and its
// deferred temp-file cleanup runs (disk stays safe).
func RunWithTimeoutCmd(s SessionBridge, info types.MessageInfo, failed, suggest string, fn func(ctx context.Context)) {
	_ = failed
	_ = suggest
	// PER-SESSION / PER-USER GUARD (latest wins): a newer command from the
	// SAME user on the SAME session cancels this one instantly. Other users
	// (same session) and any user on other sessions are never affected.
	gctx, release := s.BeginGuard(info.Sender.String())
	defer release()
	ctx, cancel := context.WithTimeout(gctx, cmdDownloaderTimeout)
	defer cancel()
	// Guard compressor shares this SAME 2-min budget: store ctx on the
	// bridge so ffmpeg is killed the moment the timeout fires (owner order).
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
		// Hard 2-minute limit hit: give the goroutine a short grace period
		// to unwind (close bodies, delete temp files) then reply.
		grace := time.After(3 * time.Second)
		select {
		case <-done:
		case <-grace:
		}
		s.Reply(info, downloaderTimeoutReplyText)
	}
}
