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
const cmdTimeout = 3 * time.Minute

// timeoutReplyText is sent when a command hits the hard limit.
const timeoutReplyText = "*TRY AGAIN LATER*"

// RunWithTimeout runs fn in a goroutine with a hard 3-minute budget.
// On timeout it cancels the context, waits a short grace period for the
// goroutine to unwind (close bodies, delete temp files), and replies
// TRY AGAIN LATER exactly once.
func RunWithTimeout(s SessionBridge, info types.MessageInfo, fn func(ctx context.Context)) {
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
