package main

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"regexp"
	"strings"
	"time"

	goldcmds "gold-md/gold-cmds"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var startTime = time.Now()
var cmdRegex = regexp.MustCompile(`(?s)^([A-Za-z0-9_]+)(.*)$`)

// Plugin function type for commands
type CommandFunc func(s *Session, info types.MessageInfo, args []string, prefix string)

var Commands = make(map[string]CommandFunc)

// ownerOnlyCommands is the set of command names (lower-cased) that may only be
// used by configured owners / the bot itself. Populated from the gold-cmds
// registry at init() time (commands_loader.go).
var ownerOnlyCommands = make(map[string]bool)

func RegisterCommand(name string, f CommandFunc) {
	Commands[strings.ToLower(name)] = f
}

func (s *Session) HandleMessage(evt *events.Message) {
	// Defense-in-depth panic guard. The EventHandler also has a recover(),
	// but keeping one here too ensures a panic in the synchronous dispatch
	// (aivideo session routing, audio/video selection, command lookup) is
	// caught before it can propagate and crash the event goroutine / bot.
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in HandleMessage: %v", s.JID, r)
		}
	}()

	// ── 🚫 OFFLINE-QUEUE MESSAGE IGNORE ──────────────—────────────────────────────────────
	// Owner: "jab bot band tha jo messages aaye the, online aane pe unka
	// jawab NAHI dena — sirf online hone ke baad bheje gaye commands ka jawab.
	//" WhatsApp offline messages QUEUE karta hai — reconnect hote hi saare
	// purane messages ek saath aate hain (test me 3-4 stale replies nikle
	// the). Ye guard un sab ko TOP pe hi ROK deta hai: command dispatch,
	// session routing, kuch nahi chalega — message poora ignore.
	// Logic: message ka timestamp agar "online aane ke waqt" (onlineFreshAt)
	// se pehle ka hai, ya reconnect ke 300ms grace-window ke andar aaya
	// (buffered push) — ye purana message hai. Sirf ONLINE hone ke BAAD ka
	// message naya hai — uska hi jawab jayega.
	if isOldMessage(s.JID, evt.Info.Timestamp) {
		return // 🚫 purana queued (offline-period) message — ignore, no reply
	}

	// ── 🛡 AUTOBLOCK (country-code instant block) —──────────────────────────────────────
	// Owner: ".autoblock add 92,1,44" → in countries ke numbers se jo bhi
	// personal message aata hai, foran WhatsApp-level block (official block
	// API via whatsmeow UpdateBlocklist — same IQ official clients use).
	// Owner exempt, groups ignored, bot ka apna msg ignored, async block
	// (0 blocking), cached config (0ms check, 0 Upstash calls per msg).
	if !evt.Info.IsGroup && !evt.Info.IsFromMe {
		brAB := &bridge{s: s}
		if goldcmds.AutoblockCheckAndBlock(brAB, evt.Info) {
			return // 🛑 blocked-country sender — message processed nahi hoga
		}
	}

	// // ── FULL INCOMING MESSAGE DUMP ──────────────────────────────────
	// // Logs EVERY incoming message with its complete structure so we can
	// // see exactly how messages arrive and how the code detects them.
	// // This is always-on (not env-dependent) so we never miss a message.
	// _msgFull := evt.Message
	// _rawFull := evt.RawMessage
	// _pmFull := _msgFull.GetProtocolMessage()
	// _pmFullType := int32(-1)
	// _pmFullKeyID := ""
	// _pmFullKeyFromMe := false
	// _pmFullKeyRemoteJID := ""
	// _pmFullHasEditedMsg := false
	// if _pmFull != nil {
	// _pmFullType = int32(_pmFull.GetType())
	// _pmFullKeyID = _pmFull.GetKey().GetID()
	// _pmFullKeyFromMe = _pmFull.GetKey().GetFromMe()
	// _pmFullKeyRemoteJID = _pmFull.GetKey().GetRemoteJID()
	// _pmFullHasEditedMsg = _pmFull.GetEditedMessage() != nil
	// }
	// _editedMsgRaw := _rawFull != nil && _rawFull.GetEditedMessage().GetMessage() != nil
	// _editedMsgInner := _msgFull != nil && _msgFull.GetEditedMessage().GetMessage() != nil
	// _ephemeralMsgRaw := _rawFull != nil && _rawFull.GetEphemeralMessage().GetMessage() != nil
	// _deviceSentRaw := _rawFull != nil && _rawFull.GetDeviceSentMessage().GetMessage() != nil
	// _viewOnceRaw := _rawFull != nil && _rawFull.GetViewOnceMessage().GetMessage() != nil
	// _incomingText := ""
	// if _msgFull != nil {
	// _incomingText = getText(_msgFull)
	// }
	// JSONDebug("INCOMING_MSG_FULL", map[string]any{
	// "stage":              "incoming",
	// "botJID":             s.JID,
	// "msgID":              evt.Info.ID,
	// "chat":               evt.Info.Chat.String(),
	// "sender":             evt.Info.Sender.String(),
	// "senderAlt":          evt.Info.SenderAlt.String(),
	// "pushName":           evt.Info.PushName,
	// "isFromMe":           evt.Info.IsFromMe,
	// "isGroup":            evt.Info.IsGroup,
	// "timestamp":          evt.Info.Timestamp.Unix(),
	// "device":             evt.Info.Sender.Device,
	// "evtIsEdit":          evt.IsEdit,
	// "evtIsEphemeral":     evt.IsEphemeral,
	// "evtIsViewOnce":      evt.IsViewOnce,
	// "evtIsViewOnceV2":    evt.IsViewOnceV2,
	// "evtIsViewOnceV2Ext": evt.IsViewOnceV2Extension,
	// "evtIsDocWithCaption": evt.IsDocumentWithCaption,
	// "evtIsLottieSticker":  evt.IsLottieSticker,
	// "evtIsBotInvoke":     evt.IsBotInvoke,
	// "retryCount":         evt.RetryCount,
	// "msgTypes":           msgTypeNames(_msgFull),
	// "rawMsgTypes":        msgTypeNames(_rawFull),
	// "editedMsgRaw":       _editedMsgRaw,
	// "editedMsgInner":     _editedMsgInner,
	// "ephemeralMsgRaw":    _ephemeralMsgRaw,
	// "deviceSentRaw":      _deviceSentRaw,
	// "viewOnceRaw":        _viewOnceRaw,
	// "pmType":             _pmFullType,
	// "pmKeyID":            _pmFullKeyID,
	// "pmKeyFromMe":        _pmFullKeyFromMe,
	// "pmKeyRemoteJID":     _pmFullKeyRemoteJID,
	// "pmHasEditedMsg":     _pmFullHasEditedMsg,
	// "msgText":            _incomingText,
	// })

	// ── SECRET ENCRYPTED MESSAGE DECRYPT ─────────────────────────────
	// WhatsApp now sends edit messages (and poll edits, event edits) as
	// SecretEncryptedMessage — the actual edit content is encrypted and
	// must be decrypted via DecryptSecretEncryptedMessage() before we can
	// detect it as an edit. Without this, edits arrive as just
	// MessageContextInfo (the outer wrapper) with evt.IsEdit=false and
	// no text, which is exactly the bug we've been chasing.
	if evt.Message != nil && evt.Message.GetSecretEncryptedMessage() != nil {
		// _secretEnc := evt.Message.GetSecretEncryptedMessage()
		// _secretEncType := _secretEnc.GetSecretEncType()
		// JSONDebug("ANTIEDIT_SECRET", map[string]any{
		// "stage":          "secret_encrypted_detected",
		// "botJID":         s.JID,
		// "msgID":          evt.Info.ID,
		// "chat":           evt.Info.Chat.String(),
		// "sender":         evt.Info.Sender.String(),
		// "secretEncType":  int32(_secretEncType),
		// "secretEncStr":   _secretEncType.String(),
		// "isFromMe":       evt.Info.IsFromMe,
		// })
		if s.Client != nil {
			_decrypted, _decErr := s.Client.DecryptSecretEncryptedMessage(context.Background(), evt)
			if _decErr != nil {
				// JSONDebugErr("ANTIEDIT_SECRET", _decErr, map[string]any{
				// "stage":  "decrypt_failed",
				// "botJID": s.JID,
				// "msgID":  evt.Info.ID,
				// })
			} else {
				// Replace the message with the decrypted content
				// _evtOld := evt.Message
				evt.Message = _decrypted
				// JSONDebug("ANTIEDIT_SECRET", map[string]any{
				// "stage":          "decrypt_success",
				// "botJID":         s.JID,
				// "msgID":          evt.Info.ID,
				// "decryptedTypes": msgTypeNames(_decrypted),
				// "oldTypes":       msgTypeNames(_evtOld),
				// })
				// After decryption, check if it's an edit:
				// The decrypted message should contain a ProtocolMessage with
				// type MESSAGE_EDIT and an EditedMessage field.
				_dpm := _decrypted.GetProtocolMessage()
				if _dpm != nil && _dpm.GetType() == waProto.ProtocolMessage_MESSAGE_EDIT {
					// JSONDebug("ANTIEDIT_SECRET", map[string]any{
					// "stage":           "edit_after_decrypt",
					// "botJID":          s.JID,
					// "msgID":           evt.Info.ID,
					// "pmType":          int32(_dpm.GetType()),
					// "pmKeyID":         _dpm.GetKey().GetID(),
					// "pmKeyFromMe":     _dpm.GetKey().GetFromMe(),
					// "pmKeyRemoteJID":  _dpm.GetKey().GetRemoteJID(),
					// "hasEditedMsg":    _dpm.GetEditedMessage() != nil,
					// })
					evt.IsEdit = true
					// Set the message to the EditedMessage content (like UnwrapRaw does)
					if _dpm.GetEditedMessage() != nil {
						evt.Message = _dpm.GetEditedMessage()
					}
					// Also update the message ID to the original message's ID
					// (the edit references the original via the key)
					if _dpm.GetKey().GetID() != "" {
						evt.Info.ID = _dpm.GetKey().GetID()
					}
				} else if _decrypted.GetEditedMessage().GetMessage() != nil {
					// Also handle the EditedMessage wrapper form
					// JSONDebug("ANTIEDIT_SECRET", map[string]any{
					// "stage":  "edited_wrapper_after_decrypt",
					// "botJID": s.JID,
					// "msgID":  evt.Info.ID,
					// })
					evt.IsEdit = true
					evt.Message = _decrypted.GetEditedMessage().GetMessage()
				}
				// Log the final state after decryption
				// JSONDebug("ANTIEDIT_SECRET", map[string]any{
				// "stage":         "post_decrypt_final",
				// "botJID":        s.JID,
				// "msgID":         evt.Info.ID,
				// "evtIsEdit":     evt.IsEdit,
				// "finalMsgTypes": msgTypeNames(evt.Message),
				// "finalMsgText":  getText(evt.Message),
				// })
			}
		}
	}

	msg := evt.Message
	if msg == nil {
		return
	}

	pm := msg.GetProtocolMessage()
	isRevoke := pm != nil && pm.GetType() == 0 // ProtocolMessage_REVOKE
	isEditProto := pm != nil && (pm.GetType() == waProto.ProtocolMessage_MESSAGE_EDIT || pm.GetEditedMessage() != nil)
	// Defensive: also detect an edit directly from the raw (unwrapped) message.
	// whatsmeow normally sets evt.IsEdit when it unwraps an EditedMessage, but some
	// edge cases / older peers send the edit wrapped in a way that doesn't set the
	// flag. Checking evt.RawMessage.GetEditedMessage() directly catches those.
	rawEditMsg := evt.RawMessage != nil && evt.RawMessage.GetEditedMessage().GetMessage() != nil
	isEdit := evt.IsEdit || isEditProto || rawEditMsg

	// // ── FULL JSON DEBUG: edit / revoke detection at the very top of dispatch ──
	// pmType := int32(-1)
	// if pm != nil {
	// pmType = int32(pm.GetType())
	// }
	// JSONDebug("ANTIEDIT_DETECT", map[string]any{
	// "botJID":       s.JID,
	// "msgID":        evt.Info.ID,
	// "chat":         evt.Info.Chat.String(),
	// "sender":       evt.Info.Sender.String(),
	// "evtIsEdit":    evt.IsEdit,
	// "isEditProto":  isEditProto,
	// "rawEditMsg":   rawEditMsg,
	// "isRevoke":     isRevoke,
	// "isEdit":       isEdit,
	// "pmType":       pmType,
	// "hasEditedMsg": pm != nil && pm.GetEditedMessage() != nil,
	// "hasRawEdit":   evt.RawMessage != nil && evt.RawMessage.GetEditedMessage().GetMessage() != nil,
	// "stage":        "detect",
	// })

	// Cache raw message proto for normal messages (never overwrite on REVOKE / edit events)
	if !isRevoke && !isEdit {
		s.cacheMessage(evt.Info.ID, msg)
		if storj.Ready() {
			info0 := evt.Info
			_, mtype0 := mediaTypeLabel(msg)
			go func(id, chat, sender, push string, t time.Time, m *waProto.Message, mt string) {
				_, _, skip, pErr := storj.PutMessage(context.Background(), id, chat, sender, push, t, m, mt)
				_ = pErr
				_ = skip
			}(info0.ID, info0.Chat.String(), info0.Sender.String(), info0.PushName, info0.Timestamp, msg, mtype0)
		}
	}

	// ── ANTIDELETE / ANTIEDIT: protocolMessage + edit detection ──────────
	if s.detectAndHandleAntiDelete(evt) {
		return
	}
	// JSONDebug("ANTIEDIT_DISPATCH_CALL", map[string]any{
	// "botJID": s.JID,
	// "msgID":  evt.Info.ID,
	// "isEdit": isEdit,
	// "stage":  "before_detectAndHandleAntiEdit",
	// })
	handled := s.detectAndHandleAntiEdit(evt)
	// JSONDebug("ANTIEDIT_DISPATCH_CALL", map[string]any{
	// "botJID":  s.JID,
	// "msgID":   evt.Info.ID,
	// "isEdit":  isEdit,
	// "handled": handled,
	// "stage":   "after_detectAndHandleAntiEdit",
	// })
	if handled {
		return
	}

	info := evt.Info
	sender := info.Sender.String()

	// ── FULL JSON DEBUG: dump raw message proto for EVERY group message ──
	// This is UNCONDITIONAL (not gated on !info.IsFromMe) so we see status
	// mention messages even when they come from the bot's own session. This
	// is the earliest point where both info and the cached raw message are
	// available. ANTISTATUS stages always print (isAntiStage in logger.go).
	// 	if info.IsGroup {
	// 		_rawMsg := s.getCachedMessage(info.ID)
	// 		if _rawMsg == nil {
	// Not cached yet (e.g. revoke/edit path) — use evt.Message directly.
	// 			_rawMsg = msg
	// 		}
	// 		_hasGSM := _rawMsg != nil && _rawMsg.GroupStatusMentionMessage != nil
	// 		_hasGS := _rawMsg != nil && _rawMsg.GroupStatusMessage != nil
	// 		_hasGSV2 := _rawMsg != nil && _rawMsg.GroupStatusMessageV2 != nil
	// 		_protoB64 := ""
	// 		_protoLen := 0
	// 		_msgTypeNames := ""
	// 		if _rawMsg != nil {
	// 			if _b, _err := proto.Marshal(_rawMsg); _err == nil {
	// 				_protoLen = len(_b)
	// 				_protoB64 = base64.StdEncoding.EncodeToString(_b)
	// 			}
	// 			_msgTypeNames = handlerMessageTypeLabel(_rawMsg)
	// 		}
	// 		JSONDebug("ANTISTATUS_RAW_MSG", map[string]any{
	// 			"msgID":                    info.ID,
	// 			"chat":                     info.Chat.String(),
	// 			"sender":                   info.Sender.String(),
	// 			"isFromMe":                 info.IsFromMe,
	// 			"pushName":                 info.PushName,
	// 			"msgTypeNames":             _msgTypeNames,
	// 			"hasGroupStatusMentionMsg": _hasGSM,
	// 			"hasGroupStatusMsg":        _hasGS,
	// 			"hasGroupStatusMsgV2":      _hasGSV2,
	// 			"isStatusMention":          _hasGSM || _hasGS || _hasGSV2,
	// 			"rawMsgNil":                _rawMsg == nil,
	// 			"protoWireLen":             _protoLen,
	// 			"protoBase64":              _protoB64,
	// 		})
	// 	}

	// ── AUTO PRESENCE (always online / auto typing / auto recording) ──────
	// Ported from UMAR-MD pair.js (lines 18660-18860). Runs on EVERY
	// incoming message, asynchronously so it never blocks dispatch.
	// Only applies when the message is from someone else (not from the
	// bot itself), matching Node.js behaviour.
	if !info.IsFromMe {
		go s.applyAutoPresence(info)
	}

	// ── AUTO READ ────────────────────────────────────────────────────────
	// Ported from UMAR-MD pair.js (lines 12942-13075). On every incoming
	// message (from others), the bot auto-marks it as read based on the
	// autoread mode (inbox / groups / all). Runs async so it never blocks.
	if !info.IsFromMe {
		go s.applyAutoRead(info)
	}

	// ── AUTO REACT / OWNER REACT ──────────────────────────────────────────
	// Ported from UMAR-MD pair.js (lines 10870-10980). On every incoming
	// real-content message (not a reaction/protocol/status/newsletter), the
	// bot auto-reacts: autoreact on OTHERS' messages (!fromMe), ownerreact
	// on the BOT/OWNER's own messages (fromMe). Smart mode = react with the
	// single emoji if the message has exactly 1, else ❤️. Custom mode =
	// random from the user's emoji list. Runs async so it never blocks.
	go s.applyAutoReact(info, msg)

	// ── AUTO STATUS (seen / react / reply) ────────────────────────────────
	// Ported from UMAR-MD pair.js (lines 15866-15955). When a status
	// (story) message arrives (chat = status@broadcast), the bot auto-sees
	// it, auto-reacts (with a random emoji), and/or auto-replies (with the
	// configured message) — all per-user Redis config. Status messages are
	// NOT processed as commands, so we return after handling.
	if info.Chat == types.StatusBroadcastJID && !info.IsFromMe {
		go s.applyAutoStatus(info, msg)
		return
	}

	// Handle pending aivideo / aivideo2 multi-image sessions FIRST, before
	// the text check. Image messages have no text body, so they would
	// otherwise be ignored. Both session maps are checked so the aivideo2
	// image-collection flow is wired up (it was previously unreachable).
	if hasPendingAIVideoSession(sender) || hasPendingAIVideo2Session(sender) {
		if handled := dispatchAIVideoMedia(s, info, msg); handled {
			return
		}
		if handled := dispatchAIVideo2Media(s, info, msg); handled {
			return
		}
	}

	// ── ANTISTATUS ENFORCEMENT (EARLY — before body check, ASYNC) ──
	// Group status mention messages have NO text body (getText returns ""),
	// so they would be skipped by the `if body == "" { return }` check below.
	// We must enforce antistatus BEFORE that check, otherwise status mentions
	// are never detected/deleted/kicked/warned. This mirrors pair.js where
	// _UmarIsStatusMentionMsg runs on the raw proto, not the text body.
	// NOTE: we do NOT check !info.IsFromMe here because whatsmeow sets
	// IsFromMe=TRUE for group status mention messages even when the actual
	// sender is a different person (not the bot). In Baileys (Node.js) the
	// same message has fromMe=FALSE. So we enforce on ALL group messages
	// and let AntistatusCheckAndEnforce decide based on the raw proto.
	// NOTE 2: enforcement runs in a goroutine (never blocks the reply path).
	// Status-mention messages have empty body, so `body == ""` will return
	// right after — the goroutine still completes its enforcement normally.
	if info.IsGroup {
		// Latency fix: this check previously ran SYNCHRONOUSLY on every
		// group message (~290-580ms Upstash REST on cache miss) delaying the
		// command reply behind it. It is enforcement (delete/kick/warn) —
		// not needed to decide the reply — so it now runs in a goroutine
		// exactly like applyAntiDetection (antilink/antibad/antibot) below.
		// Behavior unchanged: same checks, same actions, just never blocks.
		br0 := &bridge{s: s}
		go func() {
			defer func() { _ = recover() }()
			goldcmds.AntistatusCheckAndEnforce(br0, info)
		}()
	}

	body := getText(msg)
	if body == "" {
		return
	}

	// ── GROUP SETTINGS WARMUP (background, one-time per group) ──
	// On the first message from any group we warm that group's entire
	// settings hash in ONE background HGETALL (feature on/off + action +
	// bangc + welcome). Subsequent messages in that group hit the cache
	// at 0ms — previously the first message paid a ~290-580ms miss for
	// each field the checks read (antistatus reads its on/off field on
	// EVERY group message even when the feature is OFF).
	if info.IsGroup {
		if s.Manager != nil && s.Manager.Redis != nil {
			s.Manager.Redis.WarmGroupSettings(info.Chat.String())
		}
	}

	botJID := s.JID
	prefix := s.resolvePrefix(botJID)

	// ── BANGCUSER (GROUP USER BAN) ENFORCEMENT ──────────────────────────────
	// Ported from UMAR-MD pair.js (lines 10774-10865). If a user is banned
	// in this group, ALL their messages are deleted (if bot is admin) and a
	// warning is sent. This is SYNCHRONOUS (not async) because it must stop
	// further processing — the banned user's message should not trigger
	// commands, anti-detection, voice triggers, or anything else.
	if info.IsGroup && !info.IsFromMe {
		br := &bridge{s: s}
		groupJID := info.Chat.String()
		senderJID := info.Sender.String()
		if goldcmds.GroupBanUserIsBannedExported(br, groupJID, senderJID) {
			// Delete the banned user's message (best-effort, retry not needed —
			// Go's RevokeAnyMessage is a single call). If bot is not admin this
			// will silently fail, which is fine — the warning below tells admins
			// to make the bot admin.
			_ = s.RevokeAnyMessage(info.Chat, info.Sender.String(), info.ID)

			// Cooldown: only send the warning notice once per 30 seconds per
			// (group, user) pair, to avoid flooding the group.
			cdKey := groupJID + ":" + senderJID
			s.bangcuserCooldownMu.Lock()
			if s.bangcuserCooldown == nil {
				s.bangcuserCooldown = make(map[string]time.Time)
			}
			lastNotice, hasCd := s.bangcuserCooldown[cdKey]
			onCd := hasCd && time.Since(lastNotice) < 30*time.Second
			if !onCd {
				s.bangcuserCooldown[cdKey] = time.Now()
				// clean up old entries (bound the map)
				if len(s.bangcuserCooldown) > 500 {
					for k := range s.bangcuserCooldown {
						delete(s.bangcuserCooldown, k)
						break
					}
				}
			}
			s.bangcuserCooldownMu.Unlock()

			if !onCd {
				// Check if bot is admin in this group
				botJIDVal := types.JID{}
				if s.Client.Store.ID != nil {
					botJIDVal = *s.Client.Store.ID
				}
				isBotAdmin := br.IsGroupAdmin(info.Chat, botJIDVal)
				if !isBotAdmin {
					br.ReplyWithMentions(info, "*🔰 BANNED USER DETECTED 🔰*\n\nUSER :\u276f @"+info.Sender.User+"\n\n*THIS MEMBER IS BANNED IN THE GROUP HE CANNOT SEND MESSAGES IN THE GROUP AND I AM NOT ADMIN TO DELETE HIS MESSAGES FIRST MAKE ME ADMIN*\n\n*OR UNBAN THIS USER IN THIS GROUP LIKE THIS*\n*TYPE \u276e "+prefix+"USERGCUNBAN @MENTION \u276f*", []string{info.Sender.String()})
				} else {
					br.ReplyWithMentions(info, "HI @"+info.Sender.User+"\n\n*GROUP ADMINS HAVE BANNED YOU IN THIS GROUP YOU CANNOT MESSAGE IN THIS GROUP 😒*\n\n*CONTACT ADMINS WHY THEY BANNED YOU IN THIS GROUP AND REQUEST ADMINS FOR UNBAN ☺️*", []string{info.Sender.String()})
				}
			}
			return // stop all further processing
		}
	}

	// ── ANTI-DETECTION (antilink / antibot / antibad) ──────────────────
	// Ported from UMAR-MD pair.js (lines 15679-16360). Runs on group
	// messages from others (not the bot itself), asynchronously so it
	// never blocks command dispatch. Each CheckAndEnforce helper returns
	// early if its feature is off, so this is a no-op when nothing is
	// enabled. Commands (messages starting with the prefix) are exempt
	// from antibot; antilink/antibad apply to all non-bot senders.
	if info.IsGroup && !info.IsFromMe {
		go s.applyAntiDetection(info, body)
	}

	// ── CUSTOM VOICE TRIGGER ───────────────────────────────────────────
	// Ported from UMAR-MD pair.js (lines ~17460). When anyone writes a
	// saved voice name (single word, ≤50 chars, no .!/# prefix), the
	// saved audio is auto-sent. Runs on every text message, async, and
	// silently fails if no voice matches.
	go s.applyVoiceTrigger(info, body)

	// ── GOLD-MD AUTOREPLY TRIGGER ──────────────────────────────────────────────
	// Ported from UMAR-MD pair.js (lines ~10913-10944). Runs BEFORE the
	// isCommand check (same as Node.js), so it fires on ALL incoming text
	// messages — including commands. Async (queued internally, per-chat
	// limits, global concurrency cap). Modes: off/groups/inbox/on.
	brAR := &bridge{s: s}
	go goldcmds.AutoReplyTrigger(brAR, info)

	// 🔰 SETTINGS PANEL (.settings — ported from UMAR-MD pair.js) —
	// Runs BEFORE audio/video sessions and command dispatch so that a pending
	// settings session can (a) swallow the reply (handled, no rewrite), or
	// (b) rewrite it into a real prefixed command (e.g. "1" → ".anticall on")
	// which then falls through to the normal command dispatch flow.
	brST := &bridge{s: s}
	if stHandled, stRewrite := goldcmds.SettingsTryHandle(brST, info, body, prefix); stHandled {
		if stRewrite == "" {
			return
		}
		body = stRewrite // owner's number/value → real command, fall through
	}

	// 🔰 COMPRESS TIER SESSION (.compress video/GIF menu — ported from
	// UMAR-MD compress.js interactive reply flow). Runs BEFORE audio/video
	// sessions and command dispatch so a bare "1".."9" reply from a pending
	// compress menu is swallowed here and never treated as a command.
	brCP := &bridge{s: s}
	if goldcmds.CompressTryHandle(brCP, info, body, prefix) {
		return
	}

	// Handle pending audio selections first.
	if sess := getAudioSession(sender); sess != nil {
		trimmed := strings.TrimSpace(body)
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
		}
		var choice int
		if _, err := fmt.Sscanf(trimmed, "%d", &choice); err == nil && choice >= 1 && choice <= len(sess.Results) {
			selected := sess.Results[choice-1]
			clearAudioSession(sender)
			cmdName := "play"
			if sess.Play2 {
				cmdName = "play2"
			}
			if cmd, ok := Commands[cmdName]; ok {
				if sess.Play2 {
					// turbo engine quick-pick: URL + metadata (fast path)
					cmd(s, info, []string{selected.URL, selected.Thumbnail, selected.Title, selected.Duration}, prefix)
				} else {
					cmd(s, info, []string{selected.URL}, prefix)
				}
				return
			}
		}
		// NOTE: 2-minute expiry is the ONLY way this session dies now.
		// A random non-number message no longer kills the session, so the
		// user can keep typing numbers within the valid window.
		if !strings.HasPrefix(body, prefix) {
			return
		}
	}

	// Handle pending video selections FIRST (before prefix check).
	// When a video search is active, the user just types a bare number
	// (e.g. "1", "3", "14") WITHOUT any prefix to pick a result.
	if sess := getVideoSession(sender); sess != nil {
		trimmed := strings.TrimSpace(body)
		// Remove prefix if present, so ".1" and "1" both work
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
		}
		var choice int
		_, err := fmt.Sscanf(trimmed, "%d", &choice)
		if err == nil && choice >= 1 && choice <= len(sess.Results) {
			selected := sess.Results[choice-1]
			clearVideoSession(sender)
			// Trigger the download with the selected result. video2 sessions
			// (turbo engine) route back through the video2 command, with HD
			// quality when the search had HD mode active.
			cmdName := "video"
			if sess.Video2 {
				cmdName = "video2"
			}
			if cmd, ok := Commands[cmdName]; ok {
				if sess.Video2 {
					args := []string{selected.URL, selected.Thumbnail, selected.Title, selected.Duration}
					if sess.HD {
						args = append(args, "HD")
					}
					cmd(s, info, args, prefix)
				} else {
					cmd(s, info, []string{selected.URL, selected.Thumbnail, selected.Title, selected.Duration}, prefix)
				}
				return
			}
		}
		// NOTE: 2-minute expiry is the ONLY way this session dies now.
		// A random non-number message no longer kills the session, so the
		// user can keep picking numbers within the valid window.
		if !strings.HasPrefix(body, prefix) {
			return
		}
		// Otherwise it might be a new command with prefix — fall through.
	}

	if !strings.HasPrefix(body, prefix) {
		return
	}

	after := strings.TrimSpace(body[len(prefix):])

	match := cmdRegex.FindStringSubmatch(after)
	if match == nil {
		return
	}

	command := strings.ToLower(match[1])
	rest := strings.TrimSpace(match[2])
	args := splitArgs(rest)

	isOwner := s.Manager.cfg.IsOwner(sender) || sender == botJID
	// Redis sudo owners (added via .ownernumber add / .sudo add)
	if !isOwner && s.Manager.Redis != nil {
		senderNum := botOwnNumber(sender)
		if senderNum != "" {
			if senderNum == botOwnNumber(s.JID) {
				isOwner = true
			} else {
				rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
				if rawSudo != "" {
					for _, n := range strings.Split(rawSudo, ",") {
						if strings.TrimSpace(n) == senderNum {
							isOwner = true
							break
						}
					}
				}
			}
		}
	}
	// Also check SenderAlt (LID <-> phone JID mapping) for owner matching
	if !isOwner && info.SenderAlt.Server != "" {
		altSender := info.SenderAlt.String()
		if s.Manager.cfg.IsOwner(altSender) || altSender == botJID {
			isOwner = true
		}
		if !isOwner && s.Manager.Redis != nil {
			altNum := botOwnNumber(altSender)
			if altNum != "" {
				if altNum == botOwnNumber(s.JID) {
					isOwner = true
				} else {
					rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
					if rawSudo != "" {
						for _, n := range strings.Split(rawSudo, ",") {
							if strings.TrimSpace(n) == altNum {
								isOwner = true
								break
							}
						}
					}
				}
			}
		}
	}
	// IsFromMe means the message was sent from the bot owner's own device
	if !isOwner && info.IsFromMe {
		isOwner = true
	}

	// ── MODE ENFORCEMENT ──────────────────────────────────────────────
	// The bot's work-mode (public / private / groups / inbox) is persisted
	// in Redis under settings:<botJID> "mode" and changed via the .mode
	// command.  Here we enforce it BEFORE any command runs.  Owner is
	// always exempt (they can use the bot in any mode).  For non-owners:
	//   private → blocked everywhere (owner only)
	//   groups  → allowed only in group chats
	//   inbox   → allowed only in private (1:1) chats
	//   public  → allowed everywhere
	if !isOwner {
		mode := "public"
		if s.Manager.Redis != nil {
			mode = s.Manager.Redis.GetSetting(s.JID, "mode", "public")
		}
		switch mode {
		case "private":
			// Bot works ONLY for owner — silently ignore non-owners.
			return
		case "groups":
			// Bot works only in groups — block non-owners in private chats.
			if !info.IsGroup {
				return
			}
		case "inbox":
			// Bot works only in private (1:1) chats — block non-owners in groups.
			if info.IsGroup {
				return
			}
		case "public":
			// No restriction — bot works everywhere.
		}
	}

	// DEBUG: trace command dispatch

	// ── CMDNAME (CUSTOM COMMAND NAMES) ────────────────────────────────────
	// Owner .cmdname ping to umar karke apne commands ko custom naam de
	// sakta hai. Do checks, dono prefix-fallback se PEHLE (custom name ko
	// greedy match khaane se bachane ke liye):
	//   1. MINE MODE SILENCE — .cmdname mine ke baad typed name owner ke
	//      custom names me se nahi hai to bot bilkul chup rehta hai (owner
	//      ke liye bhi — yahi mine mode ka matlab hai). Lifelines: cmdname
	//      aur menu (jab tak rename nahi kiya) hamesha chalte rehte hain.
	//   2. RESOLVE — custom name (.umar) ko uske original command (.ping)
	//      me resolve karo, phir original hi dispatch hota hai — owner-only
	//      / bancmd / botblock / bangc checks original par hi lagte hain.
	// No renames set (default) → dono no-op, zero change for other bots.
	br := &bridge{s: s}
	if goldcmds.CmdNameMineBlocked(br, command) {
		return
	}
	if orig, ok := goldcmds.CmdNameResolve(br, command); ok {
		command = orig
	}

	// ── PREFIX-MATCH FALLBACK ─────────────────────────────────────────────
	// The cmdRegex `^([A-Za-z0-9_]+)(.*)$` is greedy, so a no-space form like
	// ".pair923xxxx" is parsed as command="pair923xxxx" with no args.  When the
	// exact name isn't registered, check if it starts with a known command name
	// (e.g. "pair") and route there.  This makes ".pair", ".pair 923xxx" and
	// ".pair923xxx" all hit "pair".
	if _, ok := Commands[command]; !ok {
		for base := range Commands {
			if len(base) >= 3 && strings.HasPrefix(command, base) && base != command {
				command = base
				break
			}
		}
	}

	// Check if command exists in registry
	if cmd, ok := Commands[command]; ok {
		// Owner-only protection: any command registered with OwnerOnly=true
		// (plus the legacy sessions) is silently ignored for non-owners.
		if !isOwner && (ownerOnlyCommands[command] || command == "sessions") {
			return
		}
		// ── CMDACCESS (DYNAMIC OWNER-ONLY) ENFORCEMENT ──────────────────────
		// .cmdowner <name> se owner ne kisi command ko owner-only banaya
		// ho to non-owner ke liye wo command silent ho jata hai (chup — koi
		// reply nahi). Owner khud hamesha use kar sakta hai. Check runs on
		// the RESOLVED original command name (cmdname/prefix-match ke baad),
		// aliases bhi cover hote hain (cmdowner alias-expand karta hai).
		if !isOwner && !info.IsFromMe {
			br := &bridge{s: s}
			if goldcmds.CmdAccessIsOwnerOnly(br, command) {
				return
			}
		}
		// ── BANCMD (STOPPED COMMANDS) ENFORCEMENT ─────────────────────────────────
		// Ported from UMAR-MD pair.js (lines 11205-11240). If the command
		// is stopped via .cmdstop/.bancmd, only the owner (or the bot itself)
		// may still use it. Non-owners get the stop notice and the command
		// is not executed. Runs AFTER the owner-only silent return so owners
		// always bypass, and BEFORE the BotBan check.
		if !info.IsFromMe && !isOwner {
			br := &bridge{s: s}
			if goldcmds.BancmdCheckBlocked(br, command) {
				s.Reply(info, goldcmds.BancmdStopText())
				return
			}
		}

		// ── BOT-WIDE BAN (BOTBLOCK) ENFORCEMENT ──────────────────────────────
		// Ported from UMAR-MD pair.js (lines 17561-17578). If the sender is
		// bot-wide banned (via .botblock), they cannot use ANY command. Only
		// applies to non-bot senders. The message is a command (we're inside
		// the command dispatch), so we reply with the blocked notice and stop.
		if !info.IsFromMe {
			br := &bridge{s: s}
			// Number-tolerant + cached ban check (handles 9232... vs 0327... or
			// LID JID mismatch, 0ms cached - same design as premium bypass).
			if goldcmds.BotBanSenderCheckExported(br, info) {
				s.Reply(info, "*I HAVE BLOCKED YOU FROM USING MY BOT COMMANDS 😒*")
				return
			}
		}

		// ── BANGC (GROUP LOCK) ENFORCEMENT ─────────────────────────────────────
		// Ported from UMAR-MD pair.js (lines 10712-10770). If the group is
		// locked (via .bangc), NO command works except bangc/unbangc themselves
		// (so the owner can unlock). Owner is also blocked — only bangc/unbangc
		// bypass. Applies only in groups, only to non-bot senders.
		if info.IsGroup && !info.IsFromMe {
			isBangcBypass := command == "bangc" || command == "unbangc" ||
				command == "gcbotoff" || command == "gcboton" ||
				command == "bangroup" || command == "groupban" ||
				command == "gcban"
			if !isBangcBypass {
				br := &bridge{s: s}
				groupJID := info.Chat.String()
				if goldcmds.BangcIsOn(br, groupJID) {
					s.Reply(info, "*I HAVE TURNED OFF THIS GROUP*\n\n  *NO MEMBER OR ADMINS OF THIS GROUP CAN USE ANY COMMANDS OF MY BOT UNTIL I TURN THIS GROUP BACK ON MYSELF 😒*")
					return
				}
			}
		}

		// React with 🔰 before running the command — same as UMAR-MD.
		// Run async so the reaction send (a WhatsApp network round-trip)
		// does NOT block the actual command from running. This shaves
		// ~200-500ms off every command reply.
		go s.reactCommand(info, command)
		cmd(s, info, args, prefix)
		return
	}

	// Unknown command — no reply. All core commands (alive, ping, menu,
	// uptime, sessions) are now registered in the Commands map via
	// manager.go's init(), so the legacy hardcoded switch is gone.
}

func getText(msg *waProto.Message) string {
	if msg.Conversation != nil && *msg.Conversation != "" {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	return ""
}

func splitArgs(s string) []string {
	var out []string
	cur := ""
	inSpace := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if !inSpace {
				out = append(out, cur)
				cur = ""
			}
			inSpace = true
		} else {
			cur += string(r)
			inSpace = false
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (s *Session) resolvePrefix(jid string) string {
	if s.Manager.Redis != nil {
		return s.Manager.Redis.GetPrefix(jid, s.Manager.cfg.DefaultPrefix)
	}
	return s.Manager.cfg.DefaultPrefix
}

// defaultFooterText is the 3-line GOLD-MD branded footer shown under every
// outgoing message when no custom bot name has been set via .botname.
// The %s placeholder is replaced with the bot's actual command prefix.
const defaultFooterText = "*GOLD-MD WHATSAPP BOT*\nTYPE *❮ %sBOTNAME YOUR NAME ❯*\n*TO CHANGE THIS BOT NAME*"

// brandedFooter returns the 3-line GOLD-MD branded footer with the bot's
// current command prefix injected dynamically.
func (s *Session) brandedFooter() string {
	prefix := s.resolvePrefix(s.JID)
	return fmt.Sprintf(defaultFooterText, prefix)
}

// botNameFooter returns the bot display name (from Redis field "botname") to
// be appended as a signature under every outgoing text message.
//   - If the field holds goldcmds.DefaultBotNameMarker (set on fresh pair /
//     .botname reset), the branded 3-line footer is rendered with the live
//     prefix (so changing the prefix later updates the footer too).
//   - If the field holds a custom name (set via .botname <name>), that name
//     is used as-is.
//   - If the field is missing entirely (pre-existing sessions that paired
//     before this feature), falls back to the branded footer.
func (s *Session) botNameFooter() string {
	foot := ""
	if s.Manager != nil && s.Manager.Redis != nil {
		if v := s.Manager.Redis.GetSetting(s.JID, "botname", ""); v != "" {
			// Bulletproof: treat the value as "use branded default" whenever
			// it is the exact marker OR any mangled variant that still
			// contains the sentinel substring. Redis (Upstash) JSON round-trip
			// can transform the \x01 control bytes, so a plain == check is not
			// enough — substring matching catches every mangled form.
			if v == goldcmds.DefaultBotNameMarker ||
				strings.Contains(v, "GOLD_MD_DEFAULT_FOOTER") ||
				strings.Contains(v, "GOLD_MD_DEFAULT") {
				foot = s.brandedFooter()
			} else {
				foot = v
			}
		}
	}
	if foot == "" {
		foot = s.brandedFooter()
	}
	// Sanitize: strip any residual marker sentinel and ALL control
	// characters (incl. the \x01 bytes used by the marker) so garbage like
	// "GOLD_MD_DEFAULT_FOOTER" / "00\ bot \0\0\0A" can never appear in
	// an outgoing message.
	foot = strings.ReplaceAll(foot, goldcmds.DefaultBotNameMarker, "")
	foot = strings.ReplaceAll(foot, "GOLD_MD_DEFAULT_FOOTER", "")
	foot = sanitizeFooter(foot)
	if strings.TrimSpace(foot) == "" {
		foot = s.brandedFooter()
	}
	return foot
}

// sanitizeFooter removes control characters (ASCII < 0x20 except the
// newline \n and tab \t which are valid in WhatsApp text) from a footer
// string and collapses runs of whitespace/newlines so the footer always
// renders cleanly.
func sanitizeFooter(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			// drop control char
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	// collapse 3+ consecutive newlines into 2
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

// withFooter appends the bot name footer (two blank lines + name) to text.
// Used by Reply / SendTextWithID / sendSimple for plain-text messages.
func (s *Session) withFooter(text string) string {
	return text + "\n\n" + s.botNameFooter()
}

// withCaptionFooter appends the bot name footer to a media CAPTION.
// Unlike withFooter, if the caption is empty the footer is used on its own
// (so even caption-less media still carries the bot signature). If the
// caption already ends with the footer (dedup guard) it is returned as-is.
// This is the single central place that guarantees EVERY bot message --
// text replies AND media (image / video / audio / sticker / document) --
// ships with the botname footer.
func (s *Session) withCaptionFooter(caption string) string {
	foot := s.botNameFooter()
	if foot == "" {
		return caption
	}
	if caption == "" {
		return foot
	}
	// dedup guard: avoid double-appending the footer
	if strings.HasSuffix(caption, foot) {
		return caption
	}
	return caption + "\n\n" + foot
}

func (s *Session) Reply(info types.MessageInfo, text string) {
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}
	text = s.withFooter(text)
	_, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		// ErrLog("[%s] send failed: %v", s.JID, err)
	}
}

// SendTextWithID sends a text message and returns the message ID (for edit/delete).
func (s *Session) SendTextWithID(info types.MessageInfo, text string) string {
	if s.Client == nil || !s.Client.IsConnected() {
		return ""
	}
	text = s.withFooter(text)
	resp, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		// ErrLog("[%s] send failed: %v", s.JID, err)
		return ""
	}
	return resp.ID
}

// EditMessage edits an existing message (sent by the bot) by its message ID.
func (s *Session) EditMessage(info types.MessageInfo, messageID string, newText string) bool {
	if s.Client == nil || !s.Client.IsConnected() {
		return false
	}
	if messageID == "" {
		return false
	}
	// BuildEdit(chat, messageID, newMessage) — edits a message sent by the bot
	editedMsg := s.Client.BuildEdit(info.Chat, types.MessageID(messageID), &waProto.Message{
		Conversation: proto.String(newText),
	})
	if editedMsg == nil {
		return false
	}
	_, err := s.Client.SendMessage(context.Background(), info.Chat, editedMsg)
	if err != nil {
		// ErrLog("[%s] edit failed: %v", s.JID, err)
		return false
	}
	return true
}

// DeleteMsg deletes (revokes) a message sent by the bot by its message ID.
func (s *Session) DeleteMsg(info types.MessageInfo, messageID string) error {
	if s.Client == nil || !s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	if messageID == "" {
		return nil
	}
	fromJID, _ := types.ParseJID(s.JID)
	_, err := s.Client.SendMessage(context.Background(), info.Chat,
		s.Client.BuildRevoke(info.Chat, fromJID, messageID))
	if err != nil {
		// ErrLog("[%s] delete failed: %v", s.JID, err)
	}
	return err
}

// RevokeAnyMessage revokes (deletes for everyone) any message by its ID and
// sender JID. For the bot's own messages, senderJID can be empty. For other
// people's messages in a group (when the bot is admin), pass their JID as
// senderJID. Returns nil on success.
func (s *Session) RevokeAnyMessage(chat types.JID, senderJID string, messageID string) error {
	if s.Client == nil || !s.Client.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	if messageID == "" {
		return nil
	}
	var sender types.JID
	if senderJID != "" {
		sender, _ = types.ParseJID(senderJID)
	}
	_, err := s.Client.SendMessage(context.Background(), chat,
		s.Client.BuildRevoke(chat, sender, messageID))
	if err != nil {
		// ErrLog("[%s] revoke failed: %v", s.JID, err)
	}
	return err
}

func uptime() time.Duration { return time.Since(startTime) }

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, secs)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}

// formatUptimeHMS returns the uptime in zero-padded uppercase
// "XXH XXM XXS" form (e.g. "01H 05M 03S"). When the uptime exceeds a
// day it prepends "XXD " so very long-running bots still read cleanly.
// Used by the .uptime command.
func formatUptimeHMS(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%02dD %02dH %02dM %02dS", days, hours, mins, secs)
	}
	return fmt.Sprintf("%02dH %02dM %02dS", hours, mins, secs)
}

// formatUptimeHM returns the uptime in the compact "XXH XXM" form
// (e.g. "02H 14M"). When the uptime exceeds a day it prepends "XXD "
// so long-running bots still read cleanly. Used by the fancy .menu header.
func formatUptimeHM(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%02dD %02dH %02dM", days, hours, mins)
	}
	return fmt.Sprintf("%02dH %02dM", hours, mins)
}

// ────────────────────────────────────────────────────────────────────────────
// applyAutoPresence — runs on every incoming message (from others).
// Ported from UMAR-MD pair.js (lines 18660-18860):
//  1. ALWAYS ONLINE: if enabled → sendPresence("available"); else "unavailable"
//  2. AUTO TYPING:   if enabled → sendChatPresence(composing, text) then
//     paused after 10 seconds
//  3. AUTO RECORDING: if enabled AND typing is NOT enabled →
//     sendChatPresence(composing, audio) then paused after 10s
//
// Conflict rule: typing and recording cannot both be active. The command
// handlers (presence.go) enforce this at set-time, but we double-check here
// too (recording only applies if typing is OFF).
// ────────────────────────────────────────────────────────────────────────────
func (s *Session) applyAutoPresence(info types.MessageInfo) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in applyAutoPresence: %v", s.JID, r)
		}
	}()
	if s.Client == nil {
		return
	}
	redis := s.Manager.Redis
	if redis == nil {
		return
	}

	// 1. ALWAYS ONLINE
	alwaysOnline := redis.GetSetting(s.JID, "alwaysonline", "false")
	if alwaysOnline == "true" || alwaysOnline == "1" || alwaysOnline == "on" {
		_ = s.Client.SendPresence(context.Background(), types.PresenceAvailable)
	} else {
		_ = s.Client.SendPresence(context.Background(), types.PresenceUnavailable)
	}

	// 2. AUTO TYPING
	autoTyping := redis.GetSetting(s.JID, "autotyping", "false")
	if autoTyping == "true" || autoTyping == "1" || autoTyping == "on" {
		_ = s.Client.SendChatPresence(context.Background(), info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		go func() {
			time.Sleep(10 * time.Second)
			_ = s.Client.SendChatPresence(context.Background(), info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
		}()
		return // typing takes priority — recording skipped
	}

	// 3. AUTO RECORDING (only if typing is NOT on)
	autoRecording := redis.GetSetting(s.JID, "autorecording", "false")
	if autoRecording == "true" || autoRecording == "1" || autoRecording == "on" {
		_ = s.Client.SendChatPresence(context.Background(), info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
		go func() {
			time.Sleep(10 * time.Second)
			_ = s.Client.SendChatPresence(context.Background(), info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaAudio)
		}()
	}
}

// applyAutoRead — runs on every incoming message (from others).
// Ported from UMAR-MD pair.js (lines 12942-13075). Based on the autoread
// mode stored in settings:<botJID> field "autoread":
//   - "off":    do nothing
//   - "inbox":  mark read only in private chats (DMs, !IsGroup)
//   - "groups": mark read only in group chats (IsGroup)
//   - "all":    mark read everywhere
//
// Uses whatsmeow Client.MarkRead to mark the message as read.
// ────────────────────────────────────────────────────────────────────────
func (s *Session) applyAutoRead(info types.MessageInfo) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in applyAutoRead: %v", s.JID, r)
		}
	}()
	if s.Client == nil {
		return
	}
	redis := s.Manager.Redis
	if redis == nil {
		return
	}

	// Skip status@broadcast — handled by applyAutoStatus.
	if info.Chat == types.StatusBroadcastJID {
		return
	}

	br := &bridge{s: s}
	mode := goldcmds.AutoReadMode(br)
	if mode == "off" || mode == "" {
		return
	}

	// Determine whether to mark read based on mode and chat type.
	var shouldRead bool
	switch mode {
	case "inbox":
		shouldRead = !info.IsGroup
	case "groups":
		shouldRead = info.IsGroup
	case "all":
		shouldRead = true
	default:
		return
	}
	if !shouldRead {
		return
	}

	// Mark the message as read. For group chats, the chat is the group JID
	// and the sender is the individual who sent the message. For DMs, the
	// chat is the sender's JID. whatsmeow's MarkRead expects (chat, sender)
	// — for DMs, sender can be empty/zero (same as chat).
	var sender types.JID
	if info.IsGroup {
		sender = info.Sender
	} else {
		sender = info.Sender
	}
	if err := s.Client.MarkRead(context.Background(), []types.MessageID{info.ID}, time.Now(), info.Chat, sender); err != nil {
		// ErrLog("[%s] applyAutoRead MarkRead failed: %v", s.JID, err)
		_ = err
	}
}

// applyAutoStatus — runs on every incoming status (story) message
// (chat = status@broadcast), from someone else. Ported from UMAR-MD
// pair.js (lines 15866-15955):
//  1. AUTO STATUS SEEN: if enabled → MarkRead (mark status as seen)
//  2. AUTO STATUS REACT: if enabled && emojis → send random emoji reaction
//     to the status owner
//  3. AUTO STATUS REPLY: if enabled && message → send text reply to the
//     status owner (as a status comment)
//
// All three use per-user Redis config (settings:<botJID> hash).
// ────────────────────────────────────────────────────────────────────────
func (s *Session) applyAutoStatus(info types.MessageInfo, msg *waProto.Message) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in applyAutoStatus: %v", s.JID, r)
		}
	}()
	if s.Client == nil {
		return
	}
	redis := s.Manager.Redis
	if redis == nil {
		return
	}

	// Dedup: WhatsApp sometimes delivers the same status broadcast twice.
	// Skip if we already handled this status message ID this session.
	if info.ID != "" {
		s.statusSeenIDsMu.Lock()
		if s.statusSeenIDs == nil {
			s.statusSeenIDs = make(map[string]struct{})
		}
		if _, dup := s.statusSeenIDs[info.ID]; dup {
			s.statusSeenIDsMu.Unlock()
			return
		}
		s.statusSeenIDs[info.ID] = struct{}{}
		// bound the map to the last 500 entries
		if len(s.statusSeenIDs) > 500 {
			for k := range s.statusSeenIDs {
				delete(s.statusSeenIDs, k)
				break
			}
		}
		s.statusSeenIDsMu.Unlock()
	}

	// The status owner is info.Sender (the person who posted the status).
	// info.Sender may be a LID JID (@lid) on modern WhatsApp; resolve it to
	// the real PN JID (@s.whatsapp.net) before sending anything.
	rawSender := info.Sender
	statusOwner := rawSender
	if rawSender.Server == types.HiddenUserServer {
		if info.SenderAlt.Server != "" && info.SenderAlt.Server != types.HiddenUserServer {
			statusOwner = info.SenderAlt
		} else if s.Client != nil && s.Client.Store != nil && s.Client.Store.LIDs != nil {
			if pn, err := s.Client.Store.LIDs.GetPNForLID(context.Background(), rawSender); err == nil && !pn.IsEmpty() {
				statusOwner = pn
			} else {
				// ErrLog("[STATUS] could not resolve LID %s to PN (err=%v)", rawSender.String(), err) // debug off
			}
		}
	}
	msgID := info.ID

	br := &bridge{s: s}
	seenOn := goldcmds.StatusSeenIsOn(br)
	reactOn := goldcmds.StatusReactIsOn(br)
	replyOn := goldcmds.StatusReplyIsOn(br)
	emojis := goldcmds.StatusReactEmojis(br)
	replyMsg := goldcmds.StatusReplyText(br)

	// 1. AUTO STATUS SEEN
	if seenOn {
		if err := br.MarkStatusRead(types.StatusBroadcastJID, statusOwner, msgID); err != nil {
			// ErrLog("[STATUS] seen failed owner=%s err=%v", statusOwner.ToNonAD().String(), err) // debug off
		}
	}

	// 2. AUTO STATUS REACT
	if reactOn && len(emojis) > 0 {
		emoji := goldcmds.PickRandomEmoji(emojis)
		if err := br.SendStatusReaction(statusOwner, msgID, emoji); err != nil {
			// ErrLog("[STATUS] react failed owner=%s err=%v", statusOwner.ToNonAD().String(), err) // debug off
		}
	}

	// 3. AUTO STATUS REPLY (private DM to the status owner, quoting the status)
	if replyOn && replyMsg != "" {
		quotedMsg := unwrapStatusMessage(msg)
		// Guard toggle: guard is ON when statusreply is ON (replyOn).
		// When statusreply is OFF, this whole block is skipped, so the
		// guard is effectively OFF too.
		// JSONDebug("STATUS_REPLY_IN", map[string]any{
		// "owner":         statusOwner.ToNonAD().String(),
		// "statusMsgID":   msgID,
		// "replyOn":       replyOn,
		// "reactOn":       reactOn,
		// "seenOn":        seenOn,
		// "rawMsgType":    statusMsgTypeLabel(msg),
		// "unwrappedType": statusMsgTypeLabel(quotedMsg),
		// "hasRealMsg":    statusHasRealType(quotedMsg),
		// "replyText":     replyMsg,
		// })
		if !statusHasRealType(quotedMsg) {
			quotedMsg = statusFallbackQuote(quotedMsg)
		}
		// guardOn = replyOn (guard synced with statusreply toggle)
		if err := br.SendStatusReply(statusOwner, msgID, replyMsg, quotedMsg, replyOn); err != nil {
			// ErrLog("[STATUS] reply failed owner=%s err=%v", statusOwner.ToNonAD().String(), err) // debug off
		}
	}
}

// statusMsgTypeLabel returns a short type label for a status message proto
// (JSON debug helper). Delegates to a local type detector.
func statusMsgTypeLabel(m *waProto.Message) string {
	return handlerMessageTypeLabel(m)
}

// handlerMessageTypeLabel is the local type detector (kept separate from
// commands_loader.go's messageTypeLabel to avoid redeclaration).
func handlerMessageTypeLabel(m *waProto.Message) string {
	if m == nil {
		return "nil"
	}
	switch {
	case m.GroupStatusMentionMessage != nil:
		return "groupStatusMentionMessage"
	case m.GroupStatusMessage != nil:
		return "groupStatusMessage"
	case m.GroupStatusMessageV2 != nil:
		return "groupStatusMessageV2"
	case m.EphemeralMessage != nil:
		return "ephemeralMessage"
	case m.ViewOnceMessageV2 != nil:
		return "viewOnceMessageV2"
	case m.ViewOnceMessage != nil:
		return "viewOnceMessage"
	case m.DocumentWithCaptionMessage != nil:
		return "documentWithCaptionMessage"
	case m.EditedMessage != nil:
		return "editedMessage"
	case m.GroupMentionedMessage != nil:
		return "groupMentionedMessage"
	case m.ViewOnceMessageV2Extension != nil:
		return "viewOnceMessageV2Extension"
	case m.ImageMessage != nil:
		return "imageMessage"
	case m.VideoMessage != nil:
		return "videoMessage"
	case m.PtvMessage != nil:
		return "ptvMessage"
	case m.AudioMessage != nil:
		return "audioMessage"
	case m.StickerMessage != nil:
		return "stickerMessage"
	case m.DocumentMessage != nil:
		return "documentMessage"
	case m.Conversation != nil:
		return "conversation"
	case m.ExtendedTextMessage != nil:
		return "extendedTextMessage"
	case m.ContactMessage != nil:
		return "contactMessage"
	case m.LocationMessage != nil:
		return "locationMessage"
	case m.LiveLocationMessage != nil:
		return "liveLocationMessage"
	case m.GetDeviceSentMessage() != nil:
		return "deviceSentMessage"
	default:
		return "unknown"
	}
}

// statusHasRealType delegates to hasRealMessageType (JSON debug helper).
func statusHasRealType(m *waProto.Message) bool {
	return hasRealMessageType(m)
}

// statusFallbackQuote delegates to buildFallbackQuote (JSON debug helper).
func statusFallbackQuote(m *waProto.Message) *waProto.Message {
	return buildFallbackQuote(m)
}

// unwrapStatusMessage recursively peels off ALL FutureProofMessage-style
// wrappers (EphemeralMessage, ViewOnceMessage, ViewOnceMessageV2,
// DocumentWithCaptionMessage, EditedMessage, GroupMentionedMessage,
// ViewOnceMessageV2Extension) and returns the inner status message proto
// so it can be used as ContextInfo.QuotedMessage for a status reply (quote
// preview). This mirrors Baileys "unwrapStatus" but handles every wrapper
// type, because some statuses arrive wrapped in DocumentWithCaptionMessage
// or EditedMessage instead of the usual ViewOnceMessageV2. If we only peel
// a subset, those statuses yield a wrapper proto as the quoted message,
// which WhatsApp cannot render as a quote preview — so the receiver sees a
// plain text reply (no quote) instead of a quoted reply. Returns the
// original msg if no wrapper is present.
func unwrapStatusMessage(msg *waProto.Message) *waProto.Message {
	if msg == nil {
		return nil
	}
	inner := msg
	// Peel wrappers recursively (bounded by proto depth, which is tiny).
	for i := 0; i < 8; i++ {
		if inner == nil {
			break
		}
		var next *waProto.Message
		switch {
		case inner.EphemeralMessage != nil && inner.EphemeralMessage.Message != nil:
			next = inner.EphemeralMessage.Message
		case inner.ViewOnceMessageV2 != nil && inner.ViewOnceMessageV2.Message != nil:
			next = inner.ViewOnceMessageV2.Message
		case inner.ViewOnceMessage != nil && inner.ViewOnceMessage.Message != nil:
			next = inner.ViewOnceMessage.Message
		case inner.DocumentWithCaptionMessage != nil && inner.DocumentWithCaptionMessage.Message != nil:
			next = inner.DocumentWithCaptionMessage.Message
		case inner.EditedMessage != nil && inner.EditedMessage.Message != nil:
			next = inner.EditedMessage.Message
		case inner.GroupMentionedMessage != nil && inner.GroupMentionedMessage.Message != nil:
			next = inner.GroupMentionedMessage.Message
		case inner.ViewOnceMessageV2Extension != nil && inner.ViewOnceMessageV2Extension.Message != nil:
			next = inner.ViewOnceMessageV2Extension.Message
		}
		if next == nil {
			break // no more wrappers — inner is the real message
		}
		inner = next
	}
	return inner
}

// hasRealMessageType reports whether msg carries a content message type
// that WhatsApp can render as a quote preview (image, video, audio,
// conversation, extended text, sticker, document, contact, location,
// live location). Wrapper-only protos (with nil inner Message) return
// false, which is why a fallback quoted message is needed in that case.
func hasRealMessageType(msg *waProto.Message) bool {
	if msg == nil {
		return false
	}
	return msg.ImageMessage != nil ||
		msg.VideoMessage != nil ||
		msg.AudioMessage != nil ||
		msg.Conversation != nil ||
		msg.ExtendedTextMessage != nil ||
		msg.StickerMessage != nil ||
		msg.DocumentMessage != nil ||
		msg.ContactMessage != nil ||
		msg.LocationMessage != nil ||
		msg.LiveLocationMessage != nil
}

// buildFallbackQuote returns a minimal quoted-message proto for cases where
// unwrapStatusMessage yields a wrapper-only proto (no recognizable inner
// type). WhatsApp needs *something* in QuotedMessage to render a quote
// preview; an empty Conversation makes the quote show as "Photo" / "Status"
// style placeholder rather than rendering nothing (which causes a plain
// un-quoted reply). If the unwrapped proto has a caption-bearing media type
// we still prefer the real proto via hasRealMessageType above; this is only
// the last-resort fallback.
func buildFallbackQuote(unwrapped *waProto.Message) *waProto.Message {
	// Try to extract a caption from the unwrapped proto to use as the quote text.
	if unwrapped != nil {
		if img := unwrapped.ImageMessage; img != nil && img.Caption != nil {
			return &waProto.Message{Conversation: img.Caption}
		}
		if vid := unwrapped.VideoMessage; vid != nil && vid.Caption != nil {
			return &waProto.Message{Conversation: vid.Caption}
		}
		if unwrapped.Conversation != nil {
			return &waProto.Message{Conversation: unwrapped.Conversation}
		}
		if etm := unwrapped.ExtendedTextMessage; etm != nil && etm.Text != nil {
			return &waProto.Message{Conversation: etm.Text}
		}
	}
	return &waProto.Message{Conversation: proto.String("Status")}
}

// ── ANTI-DETECTION (antilink / antibot / antibad) ──────────────────────
// Ported from UMAR-MD pair.js (lines 15679-16360). Runs on group messages
// from others. Each CheckAndEnforce helper checks IsOn internally and
// returns early when its feature is disabled, so calling all three is safe.
// Commands are exempt from antibot (prefix check inside the helper).
func (s *Session) applyAntiDetection(info types.MessageInfo, body string) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in applyAntiDetection: %v", s.JID, r)
		}
	}()
	if s.Client == nil {
		return
	}
	br := &bridge{s: s}
	// Use the full message text (with captions) for detection — mirrors
	// the Node.js body extraction (conversation / extendedText / caption).
	fullBody := br.GetMessageText(info)
	if fullBody == "" {
		fullBody = body
	}
	// Resolve the prefix so antibot can exempt commands.
	prefix := s.resolvePrefix(s.JID)
	// ── FULL JSON DEBUG: dump raw message proto for every group message ──
	// COMMENTED OUT per owner request — no antistatus debug logs.
	// if info.IsGroup {
	// 	rawMsg := br.GetRawMessage(info)
	// 	hasGSM := rawMsg != nil && rawMsg.GroupStatusMentionMessage != nil
	// 	hasGS := rawMsg != nil && rawMsg.GroupStatusMessage != nil
	// 	hasGSV2 := rawMsg != nil && rawMsg.GroupStatusMessageV2 != nil
	// 	protoB64 := ""
	// 	protoLen := 0
	// 	if rawMsg != nil {
	// 		if b, err := proto.Marshal(rawMsg); err == nil {
	// 			protoLen = len(b)
	// 			protoB64 = base64.StdEncoding.EncodeToString(b)
	// 		}
	// 	}
	// 	JSONDebug("ANTISTATUS_RAW_MSG", map[string]any{
	// 		"msgID":                     info.ID,
	// 			"chat":                      info.Chat.String(),
	// 			"sender":                    info.Sender.String(),
	// 			"isFromMe":                  info.IsFromMe,
	// 			"body":                      fullBody,
	// 			"hasGroupStatusMentionMsg":  hasGSM,
	// 			"hasGroupStatusMsg":         hasGS,
	// 			"hasGroupStatusMsgV2":       hasGSV2,
	// 			"isStatusMention":           hasGSM || hasGS || hasGSV2,
	// 			"rawMsgNil":                 rawMsg == nil,
	// 			"protoWireLen":              protoLen,
	// 			"protoBase64":               protoB64,
	// 		})
	// }

	// Run all four checks. They each no-op when off.
	goldcmds.AntilinkCheckAndEnforce(br, info, fullBody)
	goldcmds.AntibadCheckAndEnforce(br, info, fullBody)
	goldcmds.AntibotCheckAndEnforce(br, info, fullBody, prefix)
	goldcmds.AntistatusCheckAndEnforce(br, info)
}

// ── CUSTOM VOICE TRIGGER ───────────────────────────────────────────────
// Ported from UMAR-MD pair.js (lines ~17460). When anyone writes a saved
// voice name (single word, ≤50 chars, no .!/# prefix), the saved audio is
// auto-sent to the chat. Silently fails if no voice matches.
func (s *Session) applyVoiceTrigger(info types.MessageInfo, body string) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in applyVoiceTrigger: %v", s.JID, r)
		}
	}()
	if s.Client == nil {
		return
	}
	name, ok := goldcmds.VoiceTriggerMatch(body)
	if !ok {
		return
	}
	br := &bridge{s: s}
	data, mime, found := br.GetCustomVoice(name)
	if !found || len(data) == 0 {
		return
	}
	_ = br.SendVoiceMessage(info, data, mime)
}

// reactCommand sends a 🔰 reaction to the incoming command message before
// the command runs. Mirrors UMAR-MD's auto-react-on-command behaviour.
//
// .cmdreact control (gold-cmds/cmdreact.go):
//   OFF            → koi react nahi (skip)
//   single emoji   → har command par wahi emoji
//   multi emoji    → har command par random alag-alag emoji
//   per-command    → us command ka apna emoji (general se upar)
//
// command name resolved state me pass hota hai (custom .cmdname rename
// hua ho to original naam — warna per-command emoji match nahi hota).
// reactCommand call site (dispatch) par `command` variable ab final
// resolved name hold karta hai, isliye wahi pass karte hain.
func (s *Session) reactCommand(info types.MessageInfo, command string) {
	defer func() {
		if r := recover(); r != nil {
			// ErrLog("[%s] recovered panic in reactCommand: %v", s.JID, r)
		}
	}()
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}
	// .cmdreact OFF → no reaction on commands
	br := &bridge{s: s}
	emoji := goldcmds.CmdReactEmoji(br, command)
	if emoji == "" {
		return
	}
	msg := s.Client.BuildReaction(info.Chat, info.Sender, info.ID, emoji)
	_, _ = s.Client.SendMessage(context.Background(), info.Chat, msg)
}

// ── AUTO REACT / OWNER REACT ENGINE ────────────────────────────────────────
//
// Ported from UMAR-MD pair.js lines 10870-10980. This is the actual reaction
// engine that fires on every incoming message. The config commands (.autoreact
// / .ownerreact) live in gold-cmds/autoreact.go; this is the runtime that
// reads that config and sends the emoji reaction.
//
// Logic (same to same as Node.js):
//   - Skip status@broadcast and @newsletter chats.
//   - Skip reaction / protocol / senderKeyDistribution / pollUpdate message
//     types (otherwise the bot reacts to its own reactions → infinite loop).
//   - autoreact (enabled && !fromMe): react on OTHER people's messages.
//   - ownerreact (enabled && fromMe): react on BOT/OWNER's own messages.
//   - SMART mode (default / after reset): exactly 1 emoji in body → react
//     with that same emoji; 0 or 2+ emojis → fixed ❤️.
//   - CUSTOM mode (user set their own list): random emoji from the list.
func (s *Session) applyAutoReact(info types.MessageInfo, msg *waProto.Message) {
	defer func() {
		if r := recover(); r != nil {
			// never let a reaction error crash the event goroutine
			_ = r
		}
	}()
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}
	// Skip status@broadcast (handled separately by applyAutoStatus) and
	// newsletter channels.
	if info.Chat == types.StatusBroadcastJID {
		return
	}
	if info.Chat.Server == types.NewsletterServer {
		return
	}
	// Skip non-content message types that would cause an infinite react loop
	// (same skip list as Node.js _arSkipTypes).
	if msg == nil {
		return
	}
	if msg.ReactionMessage != nil || msg.ProtocolMessage != nil ||
		msg.SenderKeyDistributionMessage != nil || msg.PollUpdateMessage != nil {
		return
	}

	br := &bridge{s: s}

	// Determine which feature applies based on fromMe.
	var isOn, customMode bool
	var emojis []string
	if !info.IsFromMe {
		// Auto-react on OTHERS' messages.
		isOn = goldcmds.AutoReactIsOn(br)
		if !isOn {
			return
		}
		customMode = goldcmds.AutoReactCustomMode(br)
		emojis = goldcmds.AutoReactEmojis(br)
	} else {
		// Owner-react on BOT/OWNER's own messages.
		isOn = goldcmds.OwnerReactIsOn(br)
		if !isOn {
			return
		}
		customMode = goldcmds.OwnerReactCustomMode(br)
		emojis = goldcmds.OwnerReactEmojis(br)
	}

	// Get the message body for smart-mode emoji detection.
	body := getText(msg)

	// Pick the reaction emoji.
	emoji := goldcmds.PickAutoReactEmoji(body, customMode, emojis)
	if emoji == "" {
		return
	}

	// The sender of the message we're reacting to. In a group, info.Sender is
	// the actual sender; in a DM, info.Sender is the chat partner (or the bot
	// itself for fromMe). BuildReaction needs the original sender JID.
	sender := info.Sender
	if info.IsFromMe {
		// For owner-react, the "sender" of the bot's own message is the bot
		// itself. Parse the bot's own JID from s.JID (the string form).
		if ownJID, jerr := types.ParseJID(s.JID); jerr == nil && !ownJID.IsEmpty() {
			sender = ownJID
		}
	}

	// Send the reaction (whatsmeow BuildReaction → SendMessage).
	_ = br.SendReaction(info.Chat, sender, info.ID, emoji)
}
