package goldcmds

// ============================================================================
// GOLD-MD — Command Limits (3-min watchdog + 700MB max file)
// File: limits.go
// ============================================================================
// PUBLIC RELEASE SAFETY (10 users):
//
//   1. Every media command runs under the 3-minute hard watchdog
//      (cmdTimeout in timeout.go). On timeout: context cancelled → every
//      ffmpeg/ffprobe/gs/brotli/libreoffice/python process tied to that
//      context is KILLED instantly (no zombie processes eating CPU on the
//      Modal container) and the user gets "*TRY AGAIN LATER*".
//
//   2. 700MB hard cap on any file the bot will process. Checked BEFORE the
//      download (from WhatsApp message metadata — the fileLength field) so
//      oversized media is rejected instantly without loading it into RAM.
//      Double-checked after download as a safety net.
//
//   3. 0% SPEED IMPACT: the watchdog is a pure goroutine + select — fast
//      commands finish exactly as before, the timer only fires on a hang.
//      The size pre-check is a single integer compare on already-received
//      message metadata. Nothing blocks, nothing polls, nothing sleeps.
// ============================================================================

import (
	"context"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// maxMediaBytes is the hard cap on any media file the bot will process.
// 700MB as configured for public release. Media above this size is rejected
// instantly (pre-download via message metadata, post-download as a net).
const maxMediaBytes = 700 * 1024 * 1024

// mediaTooBigText is the reply for oversized media.
const mediaTooBigText = "*❌ FILE TOO BIG — MAX 700MB*"

// mediaTooBigReply sends the oversized-file reply once.
func mediaTooBigReply(s SessionBridge, info types.MessageInfo) {
	s.Reply(info, mediaTooBigText)
}

// mediaPrecheckSize walks the candidate chain (quoted message first, then
// the message itself) and returns the declared media size from message
// metadata. Returns 0 when unknown (WhatsApp sometimes omits fileLength —
// then the post-download check is the only guard). No download, no I/O —
// zero cost on the fast path.
func mediaPrecheckSize(s SessionBridge, info types.MessageInfo) uint64 {
	raw := s.GetRawMessage(info)
	if raw == nil {
		return 0
	}
	var candidates []*waProto.Message
	if raw.ExtendedTextMessage != nil && raw.ExtendedTextMessage.ContextInfo != nil &&
		raw.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
		candidates = append(candidates, raw.ExtendedTextMessage.ContextInfo.QuotedMessage)
	}
	candidates = append(candidates, raw)

	for _, m := range candidates {
		if m == nil {
			continue
		}
		if m.ImageMessage != nil {
			if fl := m.ImageMessage.GetFileLength(); fl > 0 {
				return fl
			}
		}
		if m.VideoMessage != nil {
			if fl := m.VideoMessage.GetFileLength(); fl > 0 {
				return fl
			}
		}
		if m.AudioMessage != nil {
			if fl := m.AudioMessage.GetFileLength(); fl > 0 {
				return fl
			}
		}
		if m.StickerMessage != nil {
			if fl := m.StickerMessage.GetFileLength(); fl > 0 {
				return fl
			}
		}
		if m.DocumentMessage != nil {
			if fl := m.DocumentMessage.GetFileLength(); fl > 0 {
				return fl
			}
		}
	}
	return 0
}

// bytesWithinLimit is the post-download safety net (len(data) check).
func bytesWithinLimit(n int) bool {
	return n <= maxMediaBytes
}

// ctxTimedOut reports whether the watchdog context has been cancelled
// (3-minute hard limit hit). Pipeline error handlers check this before
// sending their own error reply — on timeout the ONLY reply the user gets
// is the watchdog's "*TRY AGAIN LATER*" (no confusing double message).
func ctxTimedOut(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

// downloadMediaLimited is the guarded download used by media commands:
// it pre-checks the size from message metadata (instant, no download),
// downloads, then post-checks the byte length.
//
// Returns (data, mime, ok, tooBig):
//
//	tooBig=true → the file exceeds 700MB; "*❌ FILE TOO BIG — MAX 700MB*"
//	              has ALREADY been replied — the caller just returns.
//	ok=false    → no media found / download failed; the caller shows its
//	              own command-specific help or error text.
func downloadMediaLimited(s SessionBridge, info types.MessageInfo) (data []byte, mime string, ok bool, tooBig bool) {
	// ── 700MB PRE-CHECK: metadata only, instant reject, no download ──
	if fl := mediaPrecheckSize(s, info); fl > maxMediaBytes {
		mediaTooBigReply(s, info)
		return nil, "", false, true
	}

	data, mime, ok = s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		return nil, "", false, false
	}
	// ── 700MB POST-CHECK: safety net for missing metadata ──
	if !bytesWithinLimit(len(data)) {
		mediaTooBigReply(s, info)
		return nil, "", false, true
	}
	return data, mime, true, false
}
