package main

// ============================================================================
// GOLD-MD  —  ANTIDELETE / ANTIEDIT  event-capture handlers
// File: antidelete_handler.go
// ----------------------------------------------------------------------------
// ANTIDELETE: Storj recovery (10-shard S3) + event-based sender check +
//   Client.Upload send helpers. FULL JSON DEBUGGING (always-on).
// ANTIEDIT: UNTOUCHED original logic.
// ============================================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// mediaTypeLabel returns a human-readable label and the internal media
// type string for a recovered message.  When none of the known message
// payloads are present it returns mtype == "unknown".
//
// isUnknownMediaType / isUnsupportedMessage are used by the antidelete
// and antiedit handlers to SILENTLY IGNORE messages whose type the bot
// cannot meaningfully recover (status replies, ephemeral, view-once,
// system, reaction, etc.).  Previously such deletes still produced a
//   "*DELETED MESSAGE DETECTED* ... *(UNKNOWN message)*"
// notification.  Now -- exactly like status@broadcast deletes -- they
// are silently skipped so the bot never spams an empty/UNKNOWN alert.
func mediaTypeLabel(msg *waProto.Message) (label, mtype string) {
	if msg == nil {
		return "MESSAGE", "conversation"
	}
	switch {
	case msg.ImageMessage != nil:
		return "IMAGE", "imageMessage"
	case msg.VideoMessage != nil:
		return "VIDEO", "videoMessage"
	case msg.AudioMessage != nil:
		return "AUDIO", "audioMessage"
	case msg.StickerMessage != nil:
		return "STICKER", "stickerMessage"
	case msg.DocumentMessage != nil:
		return "DOCUMENT", "documentMessage"
	case msg.Conversation != nil:
		return "MESSAGE", "conversation"
	case msg.ExtendedTextMessage != nil:
		return "MESSAGE", "extendedTextMessage"
	default:
		return "MESSAGE", "unknown"
	}
}

func isTextMessage(mtype string) bool {
	return mtype == "conversation" || mtype == "extendedTextMessage"
}

// isUnknownMediaType reports whether the recovered message has a media
// type the bot cannot handle (the "unknown" fallback branch of
// mediaTypeLabel).
func isUnknownMediaType(mtype string) bool {
	return mtype == "unknown" || mtype == ""
}

// isUnsupportedMessage inspects the raw protobuf message and returns
// true when it carries ONLY payloads the bot deliberately does not
// recover / re-post -- these are the messages that previously produced
// an "*(UNKNOWN message)*" antidelete alert.
//
// Returns true if NONE of the recoverable payloads (image, video,
// audio, sticker, document, conversation, extendedText) are present.
// In that case the message is either empty, a system/reaction/
// ephemeral/view-once message, or some other future/unsupported type --
// all of which must be silently ignored exactly like status@broadcast.
func isUnsupportedMessage(msg *waProto.Message) bool {
	if msg == nil {
		return true
	}
	hasRecoverable := msg.ImageMessage != nil ||
		msg.VideoMessage != nil ||
		msg.AudioMessage != nil ||
		msg.StickerMessage != nil ||
		msg.DocumentMessage != nil ||
		msg.Conversation != nil ||
		msg.ExtendedTextMessage != nil
	return !hasRecoverable
}

// isStatusBroadcastJID returns true if the JID is the WhatsApp status /
// story broadcast (status@broadcast).  Messages here are ephemeral status
// updates — the antidelete / antiedit feature must IGNORE them so the bot
// doesn't re-post deleted stories to its own account.
func isStatusBroadcastJID(jid types.JID) bool {
	return jid.Server == "broadcast" && jid.User == "status"
}

// isStatusRemoteJID returns true if the given JID string is status@broadcast.
func isStatusRemoteJID(jidStr string) bool {
	return jidStr == "status@broadcast" || jidStr == types.StatusBroadcastJID.String()
}

// origIsStatusReply checks whether the recovered original message is a
// status reply / private mention — i.e. someone replied to a status/story.
// WhatsApp marks these messages with ContextInfo.RemoteJID == status@broadcast.
// Such messages must be silently ignored by antidelete/antiedit.
func origIsStatusReply(orig *waProto.Message) bool {
	if orig == nil {
		return false
	}
	// Check all message types that can carry ContextInfo.
	var ci *waProto.ContextInfo
	switch {
	case orig.ExtendedTextMessage != nil:
		ci = orig.ExtendedTextMessage.GetContextInfo()
	case orig.ImageMessage != nil:
		ci = orig.ImageMessage.GetContextInfo()
	case orig.VideoMessage != nil:
		ci = orig.VideoMessage.GetContextInfo()
	case orig.AudioMessage != nil:
		ci = orig.AudioMessage.GetContextInfo()
	case orig.StickerMessage != nil:
		ci = orig.StickerMessage.GetContextInfo()
	case orig.DocumentMessage != nil:
		ci = orig.DocumentMessage.GetContextInfo()
	case orig.Conversation != nil:
		// bare conversation has no ContextInfo
		return false
	default:
		return false
	}
	if ci == nil {
		return false
	}
	return isStatusRemoteJID(ci.GetRemoteJID())
}

func antiScopeAllowed(scope string, isGroup bool) bool {
	var allowed bool
	switch scope {
	case "inbox":
		allowed = !isGroup
	case "groups":
		allowed = isGroup
	default:
		allowed = true
	}
	// JSONDebug("ANTIEDIT_SCOPE", map[string]any{
		// "scope":   scope,
		// "isGroup": isGroup,
		// "allowed": allowed,
		// "stage":   "antiScopeAllowed",
	// })
	return allowed
}

func botOwnNumber(jid string) string {
	s := jid
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

func senderNumberFromJID(jid string) string {
	s := jid
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

func pkDate() string {
	loc, err := time.LoadLocation("Asia/Karachi")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("02/01/2006")
}
func pkTime() string {
	loc, err := time.LoadLocation("Asia/Karachi")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("03:04 PM")
}

// ────────────────────────────────────────────────────────────────────────────
// ANTIDELETE  —  REVOKE protocolMessage detection + Storj recovery + resend
// ────────────────────────────────────────────────────────────────────────────

func (s *Session) detectAndHandleAntiDelete(evt *events.Message) bool {
	msg := evt.Message
	if msg == nil {
		return false
	}
	pm := msg.GetProtocolMessage()
	if pm == nil {
		return false
	}
	pt := pm.GetType()
	//JSONDebug("ANTIDELETE_EVENT", map[string]any{
		//"botJID":          s.JID,
		//"stage":           "protocol_message_seen",
		//"protocolType":    pt.String(),
		//"protocolTypeInt": int32(pt),
		//"isRevoke":        pt == waE2E.ProtocolMessage_REVOKE,
		//"chat":            evt.Info.Chat.String(),
		//"msgID":           evt.Info.ID,
	//})
	if pt == waE2E.ProtocolMessage_REVOKE {
		// fall through to antidelete handling below
	} else {
		return false
	}

	info := evt.Info
	chat := info.Chat

	// ── IGNORE status / story (status@broadcast) messages entirely ──
	// When someone deletes their own status/story, WhatsApp sends a REVOKE
	// event with chat == status@broadcast. We must NOT re-post that story
	// to the bot's own account — just ignore it.
	if isStatusBroadcastJID(chat) {
		return true
	}

	br := &bridge{s: s}
	cfg := br.GetAntiDeleteSetting()
	if !cfg.Enabled {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "antidelete_disabled", "chat": chat.String(),
		//})
		return true
	}
	//JSONDebug("ANTIDELETE_EVENT", map[string]any{
		//"botJID": s.JID, "stage": "setting_read", "enabled": cfg.Enabled,
		//"isGroup": info.IsGroup, "scope": cfg.Scope, "chat": chat.String(),
	//})
	if !antiScopeAllowed(cfg.Scope, info.IsGroup) {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "scope_not_allowed", "scope": cfg.Scope, "isGroup": info.IsGroup,
		//})
		return true
	}

	key := pm.GetKey()
	if key == nil || key.GetID() == "" {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "no_key_or_id",
		//})
		return true
	}

	deletedID := key.GetID()

	// ── IGNORE BOT'S OWN DELETED MESSAGES (event-based v2 fix) ──
	eventSenderPN := info.Sender.String()
	infoIsFromMe := info.IsFromMe
	botNum := botOwnNumber(s.JID)
	senderNum := senderNumberFromJID(eventSenderPN)
	isOwn := infoIsFromMe || (botNum != "" && senderNum != "" && botNum == senderNum)

	//JSONDebug("ANTIDELETE_EVENT", map[string]any{
		//"botJID":              s.JID,
		//"stage":               "own_message_check",
		//"deletedMsgID":        deletedID,
		//"eventSenderPN":       eventSenderPN,
		//"infoIsFromMe":        infoIsFromMe,
		//"isOwn(event-based)":  isOwn,
		//"keyFromMe(flag)":     key.GetFromMe(),
		//"keyParticipant":      key.GetParticipant(),
		//"keyRemoteJID":        key.GetRemoteJID(),
		//"botNum":              botNum,
		//"senderNum":           senderNum,
	//})

	if isOwn {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "skip_own_deleted", "deletedMsgID": deletedID,
		//})
		return true
	}

	// ── IGNORE status/story deletes by key RemoteJID ──
	// The REVOKE event's key may point to status@broadcast even if the
	// event chat is a private JID (status reply deletion). Silently ignore.
	if isStatusRemoteJID(key.GetRemoteJID()) {
		return true
	}

	// ── RECOVER FROM STORJ FIRST, fall back to in-memory cache ──
	ctx := context.Background()
	var orig *waProto.Message
	//recoverSource := "none"  // (debug removed)

	if storj.Ready() {
		stored, _, gerr := storj.GetMessage(ctx, deletedID)
		if gerr == nil && stored != nil {
			orig = stored
			//recoverSource = "storj"  // (debug removed)
			//JSONDebug("ANTIDELETE_EVENT", map[string]any{
				//"botJID":         s.JID,
				//"stage":          "recovered_from_storj",
				//"deletedMsgID":   deletedID,
				//"mediaType":      meta.MediaType,
				//"originalChat":   meta.Chat,
				//"originalSender": meta.Sender,
				//"sizeBytes":      meta.SizeBytes,
			//})
		} else if gerr != nil {
			//JSONDebug("ANTIDELETE_EVENT", map[string]any{
				//"botJID": s.JID, "stage": "storj_get_error", "deletedMsgID": deletedID,
				//"error": gerr.Error(),
			//})
		}
	}
	if orig == nil {
		orig = s.getCachedMessage(deletedID)
		if orig != nil {
			//recoverSource = "cache"  // (debug removed)
			//JSONDebug("ANTIDELETE_EVENT", map[string]any{
				//"botJID": s.JID, "stage": "recovered_from_cache", "deletedMsgID": deletedID,
			//})
		}
	}
	if orig == nil {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "not_found_anywhere", "deletedMsgID": deletedID,
		//})
		return true
	}

	// ── 50 MiB HARD CAP ──
	protoSize := proto.Size(orig)
	if protoSize > maxStorjBytes {
		//JSONDebug("ANTIDELETE_EVENT", map[string]any{
			//"botJID": s.JID, "stage": "skip_over_50mb", "deletedMsgID": deletedID,
			//"protoSize": protoSize, "maxBytes": maxStorjBytes,
		//})
		return true
	}

	// ── IGNORE status replies / private mentions SILENTLY ──
	// If the recovered original message is a status reply (someone replied
	// to a story), do NOT send any ANTIDELETED MSG — just silently ignore.
	if origIsStatusReply(orig) {
		return true
	}

	label, mtype := mediaTypeLabel(orig)

	// ── IGNORE unknown / unsupported message types SILENTLY ──
	// If the recovered original message has NONE of the recoverable
	// payloads (image, video, audio, sticker, document, conversation,
	// extendedText), it is an ephemeral / view-once / system / reaction
	// message (or some other unsupported type).  Previously the bot would
	// still emit a "*DELETED MESSAGE DETECTED* ... *(UNKNOWN message)*"
	// notification for these.  Now -- exactly like status@broadcast and
	// status-reply deletes -- we SILENTLY ignore them so the user never
	// receives an empty / UNKNOWN antidelete alert.
	if isUnsupportedMessage(orig) || isUnknownMediaType(mtype) {
		return true
	}

	isTxt := isTextMessage(mtype)
	senderName := info.PushName
	if senderName == "" {
		senderName = senderNum
	}

	//JSONDebug("ANTIDELETE_EVENT", map[string]any{
		//"botJID":        s.JID,
		//"stage":         "recovered_ready_to_send",
		//"deletedMsgID":  deletedID,
		//"isText":        isTxt,
		//"mediaLabel":    label,
		//"mediaType":     mtype,
		//"recoverSource": recoverSource,
		//"senderName":    senderName,
	//})

	header := fmt.Sprintf("*🔰 DELETED %s DETECTED 🔰*\n\n*FROM :›* %s\n*DATE :›* %s\n*TIME :›* %s",
		label, senderName, pkDate(), pkTime())
	if isTxt {
		header += fmt.Sprintf("\n\n*👇 DELETED %s BELOW 👇*", label)
	}

	go func() {
		// ── delivery MODE: here (same chat) vs inbox (bot's own DM) ──
		target := chat
		br := &bridge{s: s}
		if br.GetAntiDeleteMode() == "inbox" {
			if ownJID, jerr := types.ParseJID(s.JID); jerr == nil {
				target = ownJID
			}
		}
		s.resendDeletedMessage(target, header, orig, mtype, label)
		if storj.Ready() {
			if dErr := storj.DeleteMessage(context.Background(), deletedID, "after_antidelete_recover"); dErr != nil {
				//JSONDebug("ANTIDELETE_EVENT", map[string]any{
					//"botJID": s.JID, "stage": "storj_delete_after_recover_failed",
					//"deletedMsgID": deletedID, "error": dErr.Error(),
				//})
			} else {
				//JSONDebug("ANTIDELETE_EVENT", map[string]any{
					//"botJID": s.JID, "stage": "storj_delete_after_recover_ok",
					//"deletedMsgID": deletedID, "guard": "lifted_and_sleeping",
				//})
			}
		}
	}()

	return true
}

func (s *Session) resendDeletedMessage(chat types.JID, header string, orig *waProto.Message, mtype, label string) {
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] resendDeletedMessage panic: %v", s.JID, r)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "panic", "error": fmt.Sprintf("%v", r),
				//"mediaType": mtype, "chat": chat.String(),
			//})
		}
	}()
	if s.Client == nil || !s.Client.IsConnected() {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "client_not_connected", "mediaType": mtype, "chat": chat.String(),
		//})
		s.sendSimple(chat, header+"\n\n*("+label+" — could not recover)*")
		return
	}

	ctx := context.Background()
	//JSONDebug("ANTIDELETE_SEND", map[string]any{
		//"botJID": s.JID, "stage": "send_start", "mediaType": mtype, "mediaLabel": label, "chat": chat.String(),
	//})

	switch mtype {
	case "conversation", "extendedTextMessage":
		text := fullMessageText(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "text_branch", "mediaType": mtype,
			//"textLen": len(text), "hasText": text != "", "chat": chat.String(),
		//})
		if text == "" {
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "text_empty_skip", "mediaType": mtype, "chat": chat.String(),
			//})
			return
		}
		s.sendSimple(chat, header+"\n\n"+text)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "text_sent", "mediaType": mtype, "chat": chat.String(),
		//})

	case "imageMessage":
		data, mime, ok := s.downloadMediaFromProto(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "image_download_result", "ok": ok,
			//"dataLen": len(data), "mime": mime, "chat": chat.String(),
		//})
		if ok {
			caption := header
			if c := fullMessageText(orig); c != "" {
				caption += "\n\n" + c
			}
			s.sendImage(ctx, chat, data, mime, caption)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "image_sent", "chat": chat.String(),
			//})
		} else {
			s.sendSimple(chat, header+"\n\n*(Image — could not recover)*")
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "image_recover_failed", "chat": chat.String(),
			//})
		}

	case "videoMessage":
		data, mime, ok := s.downloadMediaFromProto(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "video_download_result", "ok": ok,
			//"dataLen": len(data), "mime": mime, "chat": chat.String(),
		//})
		if ok {
			caption := header
			if c := fullMessageText(orig); c != "" {
				caption += "\n\n" + c
			}
			s.sendVideo(ctx, chat, data, mime, caption)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "video_sent", "chat": chat.String(),
			//})
		} else {
			s.sendSimple(chat, header+"\n\n*(Video — could not recover)*")
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "video_recover_failed", "chat": chat.String(),
			//})
		}

	case "audioMessage":
		s.sendSimple(chat, header)
		data, mime, ok := s.downloadMediaFromProto(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "audio_download_result", "ok": ok,
			//"dataLen": len(data), "mime": mime, "chat": chat.String(),
		//})
		if ok {
			ptt := false
			if orig.AudioMessage != nil && orig.AudioMessage.PTT != nil {
				ptt = *orig.AudioMessage.PTT
			}
			s.sendAudio(ctx, chat, data, mime, ptt)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "audio_sent", "ptt": ptt, "chat": chat.String(),
			//})
		} else {
			s.sendSimple(chat, "*(Audio — could not recover)*")
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "audio_recover_failed", "chat": chat.String(),
			//})
		}

	case "stickerMessage":
		s.sendSimple(chat, header)
		data, _, ok := s.downloadMediaFromProto(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "sticker_download_result", "ok": ok,
			//"dataLen": len(data), "chat": chat.String(),
		//})
		if ok {
			s.sendSticker(ctx, chat, data)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "sticker_sent", "chat": chat.String(),
			//})
		} else {
			s.sendSimple(chat, "*(Sticker — could not recover)*")
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "sticker_recover_failed", "chat": chat.String(),
			//})
		}

	case "documentMessage":
		data, mime, ok := s.downloadMediaFromProto(orig)
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "document_download_result", "ok": ok,
			//"dataLen": len(data), "mime": mime, "chat": chat.String(),
		//})
		if ok {
			fileName := "deleted_file"
			if orig.DocumentMessage != nil && orig.DocumentMessage.FileName != nil {
				fileName = *orig.DocumentMessage.FileName
			}
			s.sendDocument(ctx, chat, data, mime, fileName, header)
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "document_sent", "fileName": fileName, "chat": chat.String(),
			//})
		} else {
			s.sendSimple(chat, header+"\n\n*(Document — could not recover)*")
			//JSONDebug("ANTIDELETE_SEND", map[string]any{
				//"botJID": s.JID, "stage": "document_recover_failed", "chat": chat.String(),
			//})
		}

	default:
		// ── SILENTLY IGNORE unknown / unsupported message types ──
		// This is the safety-net for any unsupported message type that
		// somehow reaches the send stage. We must NOT emit the old
		// "*(UNKNOWN message)*" antidelete notification -- instead we
		// silently return, exactly like status@broadcast deletes.
		if isUnsupportedMessage(orig) || isUnknownMediaType(mtype) {
			return
		}
		upper := strings.ToUpper(mtype)
		if upper == "" {
			upper = "UNKNOWN"
		}
		s.sendSimple(chat, header+"\n\n*("+upper+" message)*")
		//JSONDebug("ANTIDELETE_SEND", map[string]any{
			//"botJID": s.JID, "stage": "default_sent", "mediaType": mtype, "chat": chat.String(),
		//})
	}
	//JSONDebug("ANTIDELETE_SEND", map[string]any{
		//"botJID": s.JID, "stage": "send_done", "mediaType": mtype, "chat": chat.String(),
	//})
}

// ────────────────────────────────────────────────────────────────────────────
// ANTIEDIT  —  IsEdit detection + OLD/NEW alert + Storj recovery + Full JSON Debug
// ────────────────────────────────────────────────────────────────────────────

func (s *Session) detectAndHandleAntiEdit(evt *events.Message) bool {
	// ── FULL JSON-TO-JSON DEBUG: entry into the antiedit handler ──
	// JSONDebug("ANTIEDIT_ENTRY", map[string]any{
		// "botJID":  s.JID,
		// "msgID":   func() string { if evt != nil { return evt.Info.ID }; return "" }(),
		// "stage":   "enter_detectAndHandleAntiEdit",
	// })

	msg := evt.Message
	if msg == nil {
		// JSONDebugErr("ANTIEDIT_ENTRY", fmt.Errorf("evt.Message is nil"), map[string]any{
			// "botJID": s.JID, "stage": "msg_nil",
		// })
		return false
	}

	pm := msg.GetProtocolMessage()
	isEditProto := pm != nil && (pm.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT || pm.GetEditedMessage() != nil)
	// Defensive: detect an edit directly from the raw (unwrapped) message too.
	rawEditMsg := evt.RawMessage != nil && evt.RawMessage.GetEditedMessage().GetMessage() != nil
	isEdit := evt.IsEdit || isEditProto || rawEditMsg

	// JSONDebug("ANTIEDIT_ENTRY", map[string]any{
		// "botJID":      s.JID,
		// "msgID":       evt.Info.ID,
		// "evtIsEdit":   evt.IsEdit,
		// "isEditProto": isEditProto,
		// "isEdit":      isEdit,
		// "pmNil":       pm == nil,
		// "pmType":      func() int32 { if pm != nil { return int32(pm.GetType()) }; return -1 }(),
		// "hasEditedMsg": pm != nil && pm.GetEditedMessage() != nil,
		// "stage":       "proto_check",
	// })

	if !isEdit {
		// JSONDebug("ANTIEDIT_SKIP", map[string]any{
			// "botJID": s.JID, "msgID": evt.Info.ID,
			// "reason": "isEdit is false (not an edit event)", "stage": "not_edit",
		// })
		return false
	}

	info := evt.Info
	chat := info.Chat

	// ── IGNORE status / story (status@broadcast) messages entirely ──
	// Edits to status/story messages should never trigger antiedit alerts.
	if isStatusBroadcastJID(chat) {
		return true
	}

	// JSONDebug("ANTIEDIT_EVENT_SEEN", map[string]any{
		// "botJID":      s.JID,
		// "evtIsEdit":   evt.IsEdit,
		// "isEditProto": isEditProto,
		// "msgID":       info.ID,
		// "chat":        chat.String(),
		// "sender":      info.Sender.String(),
		// "isGroup":     info.IsGroup,
		// "isFromMe":    info.IsFromMe,
		// "pushName":    info.PushName,
	// })

	br := &bridge{s: s}
	cfg := br.GetAntiEditSetting()
	allowed := antiScopeAllowed(cfg.Scope, info.IsGroup)

	// JSONDebug("ANTIEDIT_CONFIG_CHECK", map[string]any{
		// "botJID":  s.JID,
		// "enabled": cfg.Enabled,
		// "scope":   cfg.Scope,
		// "isGroup": info.IsGroup,
		// "allowed": allowed,
		// "stage":   "config_check",
	// })

	if !cfg.Enabled || !allowed {
		// JSONDebug("ANTIEDIT_SKIP", map[string]any{
			// "botJID":  s.JID,
			// "msgID":   info.ID,
			// "enabled": cfg.Enabled,
			// "allowed": allowed,
			// "scope":   cfg.Scope,
			// "isGroup": info.IsGroup,
			// "reason":  "antiedit disabled or scope not allowed",
			// "stage":   "disabled_or_scope",
		// })
		return true
	}

	// ── FULL JSON DEBUG: extract origID + newMsg step by step ──
	var origID string
	var newMsg *waProto.Message

	// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
		// "botJID":  s.JID,
		// "msgID":   info.ID,
		// "stage":   "extract_start",
		// "pmNil":   pm == nil,
	// })

	if pm != nil {
		if k := pm.GetKey(); k != nil {
			origID = k.GetID()
			// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "stage": "pm_key_id",
				// "origID": origID, "hasKey": true,
			// })
		} else {
			// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "stage": "pm_key_nil",
			// })
		}
		if pm.GetEditedMessage() != nil {
			newMsg = protoMsgToWaProto(pm.GetEditedMessage())
			// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "stage": "pm_edited_msg",
				// "newMsgNil": newMsg == nil,
			// })
		}
	}

	if origID == "" && evt.RawMessage != nil {
		// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "stage": "rawmsg_fallback",
		// })
		if em := evt.RawMessage.GetEditedMessage(); em != nil && em.GetMessage() != nil {
			// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "stage": "rawmsg_edited_found",
			// })
			if rpm := em.GetMessage().GetProtocolMessage(); rpm != nil {
				if k := rpm.GetKey(); k != nil {
					origID = k.GetID()
					// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
						// "botJID": s.JID, "msgID": info.ID, "stage": "rawmsg_pm_key",
						// "origID": origID,
					// })
				}
				if newMsg == nil && rpm.GetEditedMessage() != nil {
					newMsg = protoMsgToWaProto(rpm.GetEditedMessage())
					// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
						// "botJID": s.JID, "msgID": info.ID, "stage": "rawmsg_pm_edited",
						// "newMsgNil": newMsg == nil,
					// })
				}
			}
		}
	}

	if origID == "" {
		origID = info.ID
		// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "stage": "origid_fallback_evtID",
			// "origID": origID,
		// })
	}
	if newMsg == nil && msg.GetProtocolMessage() == nil {
		newMsg = msg
		// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "stage": "newmsg_fallback_msg",
			// "newMsgNil": newMsg == nil,
		// })
	}

	// JSONDebug("ANTIEDIT_EXTRACT", map[string]any{
		// "botJID":     s.JID,
		// "msgID":      info.ID,
		// "stage":      "extract_done",
		// "origID":     origID,
		// "newMsgNil":  newMsg == nil,
	// })

	// ── FULL JSON DEBUG: recover original from in-memory cache, then Storj ──
	var oldOrig *waProto.Message
	// recoverySource := "none"
	oldOrig = s.getCachedMessage(origID)
	// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
		// "botJID": s.JID, "msgID": info.ID, "origID": origID,
		// "stage": "cache_lookup", "cacheHit": oldOrig != nil,
	// })
	if oldOrig != nil {
		// recoverySource = "in-memory"
	} else if storj.Ready() {
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "storj_lookup_start", "storjReady": true,
		// })
		stored, _, gerr := storj.GetMessage(context.Background(), origID)
		if gerr != nil {
			// JSONDebugErr("ANTIEDIT_RECOVER", gerr, map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "origID": origID,
				// "stage": "storj_lookup_error",
			// })
		}
		if gerr == nil && stored != nil {
			oldOrig = stored
			// recoverySource = "storj"
			// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "origID": origID,
				// "stage": "storj_hit",
			// })
		} else {
			// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
				// "botJID": s.JID, "msgID": info.ID, "origID": origID,
				// "stage": "storj_miss", "storedNil": stored == nil,
			// })
		}
	} else {
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "storj_not_ready",
		// })
	}

	var oldText string
	var oldLabel string
	if oldOrig != nil {
		oldText = fullMessageText(oldOrig)
		oldLabel, _ = mediaTypeLabel(oldOrig)
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "old_parsed", "oldTextLen": len(oldText), "oldLabel": oldLabel,
		// })
	}
	if oldText == "" {
		oldText = "*(not in store)*"
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "old_text_empty_placeholder",
		// })
	}
	if oldLabel == "" {
		oldLabel = "MESSAGE"
	}

	newText := ""
	if newMsg != nil {
		newText = fullMessageText(newMsg)
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "new_parsed", "newTextLen": len(newText),
		// })
	} else {
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "new_msg_nil",
		// })
	}
	isEncrypted := newText == ""
	if isEncrypted {
		newText = "*(message was edited)*"
		// JSONDebug("ANTIEDIT_RECOVER", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "stage": "new_text_encrypted_placeholder",
		// })
	}

	// JSONDebug("ANTIEDIT_PAYLOAD", map[string]any{
		// "botJID":         s.JID,
		// "origID":         origID,
		// "recoverySource": recoverySource,
		// "oldLabel":       oldLabel,
		// "oldText":        oldText,
		// "newText":        newText,
		// "isEncrypted":    isEncrypted,
		// "stage":          "payload_ready",
	// })

	// ── IGNORE status replies / private mentions SILENTLY ──
	// If the edited message is a status reply (someone replied to a story),
	// do NOT send any ANTIEDITED MSG — just silently ignore.
	if oldOrig != nil && origIsStatusReply(oldOrig) {
		return true
	}
	if newMsg != nil && origIsStatusReply(newMsg) {
		return true
	}

	// ── IGNORE unknown / unsupported message types SILENTLY ──
	// If BOTH the old and new versions of the edited message are
	// unsupported types (no recoverable payload AND no text), this is an
	// edit to an ephemeral / view-once / system / reaction message (or
	// some other unsupported type).  Just like the antidelete handler, we
	// SILENTLY ignore it so the user never receives an empty / UNKNOWN
	// antiedit alert.
	oldUnsupported := oldOrig == nil || isUnsupportedMessage(oldOrig)
	newUnsupported := newMsg == nil || isUnsupportedMessage(newMsg)
	if oldUnsupported && newUnsupported && oldText == "" && newText == "" {
		return true
	}

	if !isEncrypted && oldText == newText {
		// JSONDebug("ANTIEDIT_SKIP", map[string]any{
			// "botJID":  s.JID,
			// "msgID":   info.ID,
			// "origID":  origID,
			// "reason":  "oldText equals newText (no real change)",
			// "oldText": oldText,
			// "newText": newText,
			// "stage":   "no_change",
		// })
		return true
	}

	// ── FULL JSON DEBUG: ignore bot's own edited messages ──
	if info.IsFromMe {
		// JSONDebug("ANTIEDIT_SKIP", map[string]any{
			// "botJID": s.JID, "msgID": info.ID, "origID": origID,
			// "reason": "info.IsFromMe is true", "stage": "isFromMe",
		// })
		return true
	}
	botNum := botOwnNumber(s.JID)
	senderNum := senderNumberFromJID(info.Sender.String())
	// JSONDebug("ANTIEDIT_SELF_CHECK", map[string]any{
		// "botJID":    s.JID,
		// "msgID":     info.ID,
		// "origID":    origID,
		// "botNum":    botNum,
		// "senderNum": senderNum,
		// "stage":     "self_check",
	// })
	if botNum != "" && senderNum != "" && botNum == senderNum {
		// JSONDebug("ANTIEDIT_SKIP", map[string]any{
			// "botJID":    s.JID,
			// "msgID":     info.ID,
			// "origID":    origID,
			// "reason":    "botNum == senderNum (own edit)",
			// "botNum":    botNum,
			// "senderNum": senderNum,
			// "stage":     "own_edit",
		// })
		return true
	}

	displayName := info.PushName
	if displayName == "" {
		displayName = "@" + senderNum
	}

	oldQuoted := strings.Join(strings.Split(oldText, "\n"), "\n> ")
	newQuoted := strings.Join(strings.Split(newText, "\n"), "\n> ")

	fullMsg := "*🔍 EDITED " + oldLabel + " DETECTED 🔍*\n" +
		"━━━━━━━━━━━━━━━━━\n\n" +
		"👤 *FROM :* " + displayName + "\n" +
		"📅 *DATE :* " + pkDate() + "\n" +
		"🕔 *TIME :* " + pkTime() + "\n" +
		"\n🔴 *OLD MSG :*\n> " + oldQuoted + "\n\n"
	if isEncrypted {
		fullMsg += "✏️ *New Edited msg :*\n> *see it on the user msg*\n\n"
	} else {
		fullMsg += "✏️ *New Edited msg :*\n> " + newQuoted + "\n\n"
	}
	fullMsg += "━━━━━━━━━━━━━━━━━\n"

	// JSONDebug("ANTIEDIT_ALERT_DISPATCH", map[string]any{
		// "botJID":      s.JID,
		// "chat":        chat.String(),
		// "displayName": displayName,
		// "origID":      origID,
		// "oldLen":      len(oldText),
		// "newLen":      len(newText),
		// "isEncrypted": isEncrypted,
		// "stage":       "before_send",
	// })

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ErrLog("[%s] antiedit send panic: %v", s.JID, r)
				// JSONDebug("ANTIEDIT_SEND_PANIC", map[string]any{
					// "botJID": s.JID,
					// "origID": origID,
					// "panic":  fmt.Sprintf("%v", r),
					// "stage":  "send_recover",
				// })
			}
		}()
		// JSONDebug("ANTIEDIT_ALERT_SEND", map[string]any{
			// "botJID": s.JID, "chat": chat.String(), "origID": origID,
			// "stage": "sendSimple_call",
		// })
		// ── delivery MODE: here (same chat) vs inbox (bot's own DM) ──
		target := chat
		if br.GetAntiEditMode() == "inbox" {
			if ownJID, jerr := types.ParseJID(s.JID); jerr == nil {
				target = ownJID
			}
		}
		s.sendSimple(target, fullMsg)
		// JSONDebug("ANTIEDIT_ALERT_SENT", map[string]any{
			// "botJID": s.JID,
			// "chat":   chat.String(),
			// "origID": origID,
			// "stage":  "sendSimple_done",
		// })
	}()

	// JSONDebug("ANTIEDIT_DONE", map[string]any{
		// "botJID": s.JID, "msgID": info.ID, "origID": origID,
		// "stage": "handled_true",
	// })
	return true
}

func protoMsgToWaProto(m *waE2E.Message) *waProto.Message {
	if m == nil {
		return nil
	}
	return (*waProto.Message)(m)
}

// ────────────────────────────────────────────────────────────────────────────
// Low-level send / download helpers  (Client.Upload — fixes "sirf text" bug)
// ────────────────────────────────────────────────────────────────────────────

func (s *Session) downloadMediaFromProto(msg *waProto.Message) ([]byte, string, bool) {
	if s.Client == nil || !s.Client.IsConnected() {
		return nil, "", false
	}
	dl, mime, ok := extractMediaMessage(msg)
	if !ok || dl == nil {
		return nil, "", false
	}
	data, err := s.Client.Download(context.Background(), dl)
	if err != nil {
		ErrLog("[%s] antidelete media download failed: %v", s.JID, err)
		return nil, "", false
	}
	//JSONDebug("ANTIDELETE_SEND", map[string]any{
		//"botJID": s.JID, "stage": "download_ok",
		//"dataLen": len(data), "mime": mime,
	//})
	return data, mime, true
}

func (s *Session) sendSimple(chat types.JID, text string) {
	if s.Client == nil {
		return
	}
	out := s.withFooter(text)
	_, _ = s.Client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: &out,
	})
}

func (s *Session) sendImage(ctx context.Context, chat types.JID, data []byte, mime, caption string) {
	if s.Client == nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendImage_nil_client", "chat": chat.String()})
		return
	}
	caption = s.withCaptionFooter(caption) // botname footer on every image
	uploaded, err := s.Client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendImage_upload_err", "chat": chat.String(), "error": err.Error(), "dataLen": len(data)})
		return
	}
	_, _ = s.Client.SendMessage(ctx, chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendImage_sent", "chat": chat.String(), "dataLen": len(data), "mime": mime, "sendErr": fmt.Sprintf("%v", sErr)})
}

func (s *Session) sendVideo(ctx context.Context, chat types.JID, data []byte, mime, caption string) {
	if s.Client == nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendVideo_nil_client", "chat": chat.String()})
		return
	}
	caption = s.withCaptionFooter(caption) // botname footer on every video
	uploaded, err := s.Client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendVideo_upload_err", "chat": chat.String(), "error": err.Error(), "dataLen": len(data)})
		return
	}
	_, _ = s.Client.SendMessage(ctx, chat, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendVideo_sent", "chat": chat.String(), "dataLen": len(data), "mime": mime, "sendErr": fmt.Sprintf("%v", sErr)})
}

func (s *Session) sendAudio(ctx context.Context, chat types.JID, data []byte, mime string, ptt bool) {
	if s.Client == nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendAudio_nil_client", "chat": chat.String()})
		return
	}
	uploaded, err := s.Client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendAudio_upload_err", "chat": chat.String(), "error": err.Error(), "dataLen": len(data)})
		return
	}
	_, _ = s.Client.SendMessage(ctx, chat, &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			Mimetype:      proto.String(mime),
			PTT:           proto.Bool(ptt),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendAudio_sent", "chat": chat.String(), "dataLen": len(data), "mime": mime, "ptt": ptt, "sendErr": fmt.Sprintf("%v", sErr)})
}

func (s *Session) sendSticker(ctx context.Context, chat types.JID, data []byte) {
	if s.Client == nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendSticker_nil_client", "chat": chat.String()})
		return
	}
	uploaded, err := s.Client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendSticker_upload_err", "chat": chat.String(), "error": err.Error(), "dataLen": len(data)})
		return
	}
	_, _ = s.Client.SendMessage(ctx, chat, &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendSticker_sent", "chat": chat.String(), "dataLen": len(data), "sendErr": fmt.Sprintf("%v", sErr)})
}

func (s *Session) sendDocument(ctx context.Context, chat types.JID, data []byte, mime, fileName, caption string) {
	if s.Client == nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendDocument_nil_client", "chat": chat.String()})
		return
	}
	caption = s.withCaptionFooter(caption) // botname footer on every document
	uploaded, err := s.Client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendDocument_upload_err", "chat": chat.String(), "error": err.Error(), "dataLen": len(data)})
		return
	}
	_, _ = s.Client.SendMessage(ctx, chat, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			FileName:      proto.String(fileName),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	//JSONDebug("ANTIDELETE_SEND", map[string]any{"botJID": s.JID, "stage": "sendDocument_sent", "chat": chat.String(), "dataLen": len(data), "mime": mime, "fileName": fileName, "sendErr": fmt.Sprintf("%v", sErr)})
}

// waCommonKey is a guard so the waCommon import is always used.
var _ waCommon.MessageKey
