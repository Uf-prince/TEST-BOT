package goldcmds

// ============================================================================
// GOLD-MD — View-Once Opener (VV) Command + .vvset mode control
// File: vv.go
// ============================================================================
// COMMAND: .vv    (owner-only, reply to a view-once image/video/audio message)
//   Downloads the view-once media and re-sends it as a normal, permanent
//   media message (image, video, or audio) so it can be saved/forwarded.
//
// TWO DELIVERY MODES (set via .vvset):
//
//   .vvset same   → (DEFAULT) current behaviour: bot replies "OPENING
//                   VIEWONCE MEDIA...", sends the opened media back into the
//                   SAME chat, then deletes its own wait message.
//
//   .vvset inbox  → SILENT mode: no reaction on the .vv command message, the
//                   .vv command message itself is deleted (best-effort), NO
//                   reply is sent, and the opened view-once media is delivered
//                   straight to the bot's OWN private inbox (YOU) silently.
//
// Storage: Redis settings:<botJID> hash, field "vvmode" = "same" | "inbox".
//
// Source: UMAR-MD vv.js  (Node.js / Baileys — downloadContentFromMessage)
// Converted to Go / whatsmeow for GOLD-MD.
//
// The bridge's DownloadQuotedMedia handles ViewOnceMessage /
// ViewOnceMessageV2 / ViewOnceMessageV2Extension wrappers (and quoted media),
// returning the raw bytes + mimetype. We then re-send based on mimetype.
//
// Aliases (Hidden): viewonce, vvopen, openvv, showvv, vvshow, privacyopen
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// vvModeField — Redis settings field holding the delivery mode.
const vvModeField = "vvmode"

// Delivery modes.
const (
	VVModeSame  = "same"  // send opened media back to the same chat (default)
	VVModeInbox = "inbox" // send opened media silently to the bot's own inbox
)

// vvFamilyNames — every registered name that routes to the .vv handler.
var vvFamilyNames = map[string]bool{
	"vv":          true,
	"viewonce":    true,
	"vvopen":      true,
	"openvv":      true,
	"showvv":      true,
	"vvshow":      true,
	"privacyopen": true,
}

// VVIsVVCommand reports whether a (resolved) command name belongs to the
// .vv family. Used by the handler to suppress the command reaction in
// inbox mode.
func VVIsVVCommand(name string) bool {
	return vvFamilyNames[strings.ToLower(strings.TrimSpace(name))]
}

// VVGetMode reads the current .vv delivery mode ("same" default).
func VVGetMode(s SessionBridge) string {
	m := strings.ToLower(strings.TrimSpace(s.GetStatusSetting(vvModeField, VVModeSame)))
	if m != VVModeInbox {
		return VVModeSame
	}
	return VVModeInbox
}

// VVSetMode writes the .vv delivery mode (normalized to same|inbox).
func VVSetMode(s SessionBridge, mode string) {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m != VVModeInbox {
		m = VVModeSame
	}
	s.SetStatusSetting(vvModeField, m)
}

// VVIsInboxMode reports whether .vv is in silent inbox mode.
func VVIsInboxMode(s SessionBridge) bool {
	return VVGetMode(s) == VVModeInbox
}

// VVHasQuotedMedia reports whether the incoming message is a reply to a
// quoted message (i.e. the owner mentioned/replied to some media). Used by
// the handler to decide whether the .vv reaction should be suppressed in
// inbox mode — suppression ONLY applies when a media is actually mentioned.
// A bare ".vv" (no reply) must still get the normal reaction + guidance.
func VVHasQuotedMedia(s SessionBridge, info types.MessageInfo) bool {
	q, ok := s.(interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	})
	if !ok {
		return false
	}
	id, _, ok2 := q.GetQuotedMessageID(info)
	return ok2 && id != ""
}

const vvHelpText = "*🔰 VIEWONCE COMMAND INFO 🔰*\n\n" +
	"*OPENS VIEWONCE MEDIA*\n" +
	"*SUPPORTED TYPES:*\n" +
	"• IMAGE 🔰\n" +
	"• VIDEO 🔰\n" +
	"• AUDIO 🔰\n" +
	"• VOICE NOTE 🔰\n\n" +
	"*REPLY TO ANY VIEWONCE MEDIA WITH THIS COMMAND*\n\n" +
	"*EXAMPLE:* .vv\n\n" +
	"*🔰 OWNER ONLY COMMAND 🔰*"

// vvSetGuide is the .vvset guidance block (bold + 🔰 design, same style as
// the other GOLD-MD command guides). `mode` is the CURRENT mode (INBOX|SAME).
func vvSetGuide(prefix string, mode string) string {
	return "*🔰 VVSET — VIEWONCE DELIVERY MODE 🔰*\n\n" +
		"*CURRENT MODE :❯ " + strings.ToUpper(mode) + "*\n\n" +
		"*COMMANDS:*\n" +
		"*TYPE ❮ " + prefix + "VVSET INBOX ❯*\n" +
		"*WHEN YOU SET VV SETTINGS TO INBOX OR WHEN YOU TYPE ❮ VV ❯ THE BOT SEND VIEWONCE OPENED MESSAGE IN YOUR (YOU) INBOX ONLY*\n\n" +
		"*TYPE ❮ " + prefix + "VVSET SAME ❯*\n" +
		"*WHEN YOU SET VV SETTINGS TO SAME OR WHEN YOU TYPE ❮ VV ❯ THE BOT SEND VIEWONCE OPENED MESSAGE IN SAME CHAT*"
}

func handleVV(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleVVAsync(s, info, args, prefix)
}

// sendVVMedia re-sends the opened view-once bytes to `target` based on mime.
func sendVVMedia(s SessionBridge, target types.MessageInfo, data []byte, mime string) error {
	low := strings.ToLower(mime)
	switch {
	case strings.HasPrefix(low, "image"):
		return s.SendImage(target, data, "*IMAGE OPENED*")
	case strings.HasPrefix(low, "video"):
		return s.SendVideo(target, data, "*VIDEO OPENED*", nil, 0, 0, 0)
	case strings.HasPrefix(low, "audio"):
		return s.SendAudio(target, data, "", 0)
	case strings.Contains(low, "webp"):
		return s.SendSticker(target, data)
	default:
		return s.SendDocument(target, data, "viewonce_media", mime, "*MEDIA OPENED*")
	}
}

func handleVVAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	inboxMode := VVIsInboxMode(s)

	// Check if there is a quoted message at all.
	hasQuoted := false
	if q, ok := s.(interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}); ok {
		if qid, _, ok2 := q.GetQuotedMessageID(info); ok2 && qid != "" {
			hasQuoted = true
		}
	}
	if !hasQuoted {
		// NO MEDIA MENTIONED — always show the guidance message, in BOTH
		// modes (same + inbox). The silent/delete behaviour only applies
		// when the owner actually replies to a view-once media.
		s.Reply(info, vvHelpText)
		return
	}

	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		if inboxMode {
			// SILENT: delete the .vv command message, no reply.
			_ = s.RevokeQuotedMessage(info.Chat, info.Sender.String(), info.ID)
			return
		}
		s.Reply(info, "🔰 *THIS IS NOT A VIEWONCE MEDIA*\n\n*Reply to a ViewOnce image/video/audio with .vv*")
		return
	}

	// ── INBOX MODE: fully silent delivery to the bot's own inbox ──
	if inboxMode {
		// Delete the .vv command message (best-effort — works when the bot
		// can revoke it, e.g. its own message or a group where it is admin).
		_ = s.RevokeQuotedMessage(info.Chat, info.Sender.String(), info.ID)

		// Redirect the media to the bot's OWN private inbox (YOU).
		target := info
		if ownJID, err := types.ParseJID(s.GetJID()); err == nil && !ownJID.IsEmpty() {
			target.Chat = ownJID
		}
		_ = sendVVMedia(s, target, data, mime)
		return
	}

	// ── SAME MODE (default): current behaviour ──
	waitID := s.ReplyWithID(info, "🔰 *OPENING VIEWONCE MEDIA...*")
	sentErr := sendVVMedia(s, info, data, mime)
	s.DeleteMessage(info, waitID)

	if sentErr != nil {
		s.Reply(info, "🔰 *FAILED TO OPEN VIEWONCE MEDIA*\n"+sentErr.Error())
		return
	}
}

// handleVVSet — .vvset guidance + mode switch.
func handleVVSet(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	arg := ""
	if len(args) > 0 {
		arg = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch arg {
	case "inbox":
		VVSetMode(s, VVModeInbox)
		s.Reply(info, "*VIEWONCE MESSAGE DELIVERY CHANGED*\n\n"+
			"*YOU HAVE SET THE VIEWONCE MESSAGE MEDIA PLACE IN ❮ INBOX ❯ NOW WHEN YOU TYPE ❮ "+prefix+"VV ❯ THE BOT SEND VIEWONCE OPENED MESSAGE IN YOUR (YOU) INBOX*")
		return
	case "same":
		VVSetMode(s, VVModeSame)
		s.Reply(info, "*VIEWONCE MESSAGE DELIVERY CHANGED*\n\n"+
			"*YOU HAVE SET THE VIEWONCE MESSAGE MEDIA PLACE IN ❮ SAME ❯ NOW WHEN YOU TYPE ❮ "+prefix+"VV ❯ THE BOT SEND VIEWONCE OPENED MESSAGE IN SAME CHATS/GROUPS*")
		return
	}

	// No / unknown arg → show current mode + full guidance.
	s.Reply(info, vvSetGuide(prefix, VVGetMode(s)))
}

func init() {
	Register(Command{Name: "vv", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO OPEN AND VIEW ONE TIME VIEW MEDIA. REPLY TO A VIEW ONCE PHOTO OR VIDEO AND USE THIS COMMAND.", OwnerOnly: true, Run: handleVV})
	Register(Command{Name: "vvset", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO SET HOW THE .VV COMMAND DELIVERS VIEWONCE MEDIA. CHOOSE INBOX TO SEND IT SILENTLY TO YOUR PRIVATE INBOX OR SAME TO SEND IT IN THE SAME CHAT.", OwnerOnly: true, Run: handleVVSet})
	Register(Command{Name: "viewonce", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "vvopen", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "openvv", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "showvv", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "vvshow", OwnerOnly: true, Hidden: true, Run: handleVV})
	Register(Command{Name: "privacyopen", OwnerOnly: true, Hidden: true, Run: handleVV})
}
