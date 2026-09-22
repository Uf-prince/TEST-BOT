package goldcmds

// ============================================================================
// GOLD-MD — Command Limits (3-min watchdog; NO file-size cap)
// File: limits.go
// ============================================================================
// OWNER ORDER (2026): the 700MB file-size cap is REMOVED — the bot now runs
// on Heroku (plenty of RAM/disk/bandwidth), so there is NO download limit.
//
//   1. Every media command still runs under the 3-minute hard watchdog
//      (cmdTimeout in timeout.go). On timeout: context cancelled → every
//      ffmpeg/ffprobe/gs/brotli/libreoffice/python process tied to that
//      context is KILLED instantly and the user gets "*TRY AGAIN LATER*".
//
//   2. NO file-size cap. The old 700MB pre-check / post-check are gone.
// ============================================================================

import (
	"context"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// maxMediaBytes — OWNER ORDER (2026): the 700MB cap is REMOVED. The bot now
// runs on Heroku (plenty of RAM/disk/bandwidth), so there is NO file-size
// limit. Kept as a very large sentinel so the (now no-op) checks still compile.
const maxMediaBytes = 1 << 62

// mediaTooBigText is the reply for oversized media. With the cap removed this
// never fires, but the string is kept for compatibility.
const mediaTooBigText = "*🔰 FILE TOO BIG*"

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

// bytesWithinLimit — OWNER ORDER (2026): NO size limit. Always true.
func bytesWithinLimit(n int) bool {
	_ = n
	return true
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
	// OWNER ORDER (2026): the 700MB pre-check / post-check are REMOVED — no
	// file-size limit on Heroku. Download whatever the user sent.
	data, mime, ok = s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		return nil, "", false, false
	}
	return data, mime, true, false
}
