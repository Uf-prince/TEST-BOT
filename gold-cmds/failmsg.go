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

// RunWithTimeoutCmd is RunWithTimeout with a command-specific failure reply:
// even the hard 3-minute watchdog uses the cross-fallback message, so a
// timeout on .video tells the user to try .video2 (and vice versa).
func RunWithTimeoutCmd(s SessionBridge, info types.MessageInfo, failed, suggest string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
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
		// Hard limit hit: give the goroutine a short grace period to
		// unwind (close bodies, delete temp files) then reply.
		grace := time.After(3 * time.Second)
		select {
		case <-done:
		case <-grace:
		}
		s.Reply(info, cmdFailMsg(failed, suggest))
	}
}
