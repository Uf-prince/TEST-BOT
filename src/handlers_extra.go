package main

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	goldcmds "gold-md/gold-cmds"
	"google.golang.org/protobuf/proto"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ══════════════════ (merged from anticall_handler.go) ══════════════════
// ============================================================================
// GOLD-MD — ANTICALL call-rejection handler
// File: anticall_handler.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js `umar.ev.on('call', ...)` (lines 18808-18860).
//
// When a 1:1 (inbox) WhatsApp call offer arrives (*events.CallOffer):
//   1. Check if anticall is ON for this bot (Redis settings:<botJID>)
//   2. Skip group calls (GroupJID not empty — only inbox calls rejected)
//   3. Reject the call via Client.RejectCall(ctx, callFrom, callID)
//   4. Send the custom .anticall msg (or ANTICALL_DEFAULT_MSG fallback)
//      to the caller
//
// This is called from Session.EventHandler in manager.go when a
// *events.CallOffer event is received.
// ============================================================================

// handleAntiCall processes an incoming call offer and rejects it if
// anticall is enabled. Mirrors the Node.js `umar.ev.on('call', ...)` logic.
func (s *Session) handleAntiCall(evt *events.CallOffer) {
	// Top-level panic guard — same as EventHandler.
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] recovered panic in handleAntiCall: %v", s.JID, r)
		}
	}()

	// ── GROUP CALL RING → ANTIGCCALL (FIRST — before anticall ON/OFF) ──
	// WhatsApp ka NAYA group-call flow: jab member group me call chalata
	// hai, WhatsApp har member ko ALAG 1:1-style ring bhejta hai
	// (events.CallOffer) jis me group-jid attribute SET hota hai —
	// sirf CallOfferNotice nahi. Routing SABSE PEHLE yahan hai:
	//   • anticall OFF ho tab bhi group ring antigccall ko jaye
	//   • anticall ON ho to bhi group ring antigccall se handle ho
	// Per-group on/off + action check gccallEnforce me hoti hai —
	// antigccall OFF group me silent skip hota hai.
	if !evt.GroupJID.IsEmpty() {
		s.handleAntiGcCallRing(evt)
		return
	}

	// Check if anticall is ON for this bot.
	br := &bridge{s: s}
	if !br.GetAntiCallSetting() {
		return
	}

	// The caller JID — evt.From is the call initiator.
	callerJID := evt.From
	if callerJID.IsEmpty() {
		// DebugLog("[%s] anticall: empty caller JID, skipping", s.JID)
		return
	}

	// The call ID — needed for RejectCall.
	callID := evt.CallID
	if callID == "" {
		// DebugLog("[%s] anticall: empty call ID from %s, skipping", s.JID, callerJID.String())
		return
	}

	// ── ANTICALLPREM BYPASS (LID-TOLERANT — 0% farak antilinkprem se) ──
	// Premium users (.anticallprem add) EXEMPT hain call rejection se —
	// unki calls normally ring hoti hain, reject NAHI hoti. Shared "premium:set"
	// list (antilinkprem / antibotprem / antibadprem / antistatusprem wali).
	//
	// WhatsApp call events caller ko multiple JIDs me le sakte hain —
	// From, CallCreator aur CallCreatorAlt. Jab caller LID protocol use
	// kare to From/CallCreator "@lid" format me hote hain (digits phone
	// number NAHI hote) — asli phone JID CallCreatorAlt ("caller_pn") me
	// milta hai. Isliye TEENO candidates check hote hain: exact JID match
	// + last-10-digit number-tail match — bilkul antilinkprem ke
	// premiumSenderBypass jaisa. Sab checks CACHED (0ms) — speed pe zero asar.
	if anticallCallerIsPremium(br, callerJID.String()) ||
		(!evt.CallCreator.IsEmpty() && anticallCallerIsPremium(br, evt.CallCreator.String())) ||
		(!evt.CallCreatorAlt.IsEmpty() && anticallCallerIsPremium(br, evt.CallCreatorAlt.String())) {
		return
	}

	// Reject the call — same as Node.js `umar.rejectCall(call.id, call.from)`.
	if s.Client != nil {
		err := s.Client.RejectCall(context.Background(), callerJID, callID)
		if err != nil {
			ErrLog("[%s] anticall: failed to reject call from %s (id=%s): %v",
				s.JID, callerJID.String(), callID, err)
		} else {
			OkLog("[%s] anticall: rejected call from %s (id=%s)",
				s.JID, callerJID.String(), callID)
		}
	}

	// Send the custom reject message (or default) to the caller.
	// Same as Node.js: `UmarGetAntiCallMessage(botNumber) || ANTICALL_DEFAULT_MSG`.
	rejectMsg := br.GetAntiCallMessage(goldcmds.ANTICALL_DEFAULT_MSG)
	s.sendSimple(callerJID, rejectMsg)
}

// anticallCallerIsPremium reports whether the given caller JID (koi bhi
// candidate — From / CallCreator / CallCreatorAlt) is in the shared
// premium whitelist ("premium:set"). Do tarah se check — antilinkprem ke
// premiumSenderBypass jaisa, 0% farak:
//
//  1. EXACT JID match — device/agent suffix strip karke
//     ("923...:12@s.whatsapp.net" → "923...@s.whatsapp.net") taaki
//     stored phone JID se seedha match ho jaye.
//
//  2. NUMBER match — last-10-digit tail (local 0327... vs international
//     92327... — dono ka tail same). PremiumMemberTails() CACHED list hai
//     (0ms), isliye bot speed pe zero asar.
func anticallCallerIsPremium(br *bridge, callerJID string) bool {
	// device/agent suffix + server normalize
	// ("user:12@s.whatsapp.net" → "user@s.whatsapp.net")
	norm := anticallNormalizeJID(callerJID)

	// 1. exact JID match — normalized + raw dono try
	if norm != "" && br.PremiumIsMember(norm) {
		return true
	}
	if br.PremiumIsMember(callerJID) {
		return true
	}

	// 2. number-tail match (premiumSenderBypass jaisa number-tolerant check)
	tail := callerNumberTail(norm)
	if tail == "" {
		return false
	}
	for _, t := range br.PremiumMemberTails() {
		if t == tail {
			return true
		}
	}
	return false
}

// anticallNormalizeJID strips device (":N") / agent (".N") suffixes and
// returns "user@server". Device digits se tail match corrupt hota tha
// ("923...:12" ke digits me ":12" bhi shamil ho jata tha) — ye fix karta hai.
func anticallNormalizeJID(jid string) string {
	if jid == "" {
		return ""
	}
	user := jid
	server := ""
	if i := strings.IndexByte(user, '@'); i >= 0 {
		server = user[i+1:]
		user = user[:i]
	}
	if i := strings.IndexByte(user, ':'); i >= 0 {
		user = user[:i]
	}
	if i := strings.IndexByte(user, '.'); i >= 0 {
		user = user[:i]
	}
	if user == "" {
		return ""
	}
	if server != "" {
		return user + "@" + server
	}
	return user
}

// callerNumberTail extracts digits from the (normalized) JID user part and
// returns the last 10 (local 03274765023 vs international 923274765023 —
// dono ka tail same).
func callerNumberTail(jid string) string {
	num := jid
	if i := strings.IndexByte(num, '@'); i >= 0 {
		num = num[:i]
	}
	var digits []byte
	for i := 0; i < len(num); i++ {
		c := num[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	if len(digits) > 10 {
		return string(digits[len(digits)-10:])
	}
	return string(digits)
}

// ══════════════════ (antigccall handler) ══════════════════

// handleAntiGcCall processes an incoming GROUP call OFFER NOTICE and applies
// the configured antigccall action — with FULL JSON DEBUG at every stage.
// Called from EventHandler (case *events.CallOfferNotice) in manager.go.
func (s *Session) handleAntiGcCall(evt *events.CallOfferNotice) {
	// Top-level panic guard — same as EventHandler.
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] recovered panic in handleAntiGcCall: %v", s.JID, r)
		}
	}()

	s.gccallEnforce(evt.From, evt.CallCreator, evt.CallCreatorAlt, evt.CallID, evt.GroupJID)
}

// handleAntiGcCallRing processes an incoming GROUP call RING (the NEW
// WhatsApp flow: events.CallOffer with a non-empty GroupJID — every member
// gets their own 1:1-style ring). Routed here from handleAntiCall.
func (s *Session) handleAntiGcCallRing(evt *events.CallOffer) {
	// Top-level panic guard — same as EventHandler.
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] recovered panic in handleAntiGcCallRing: %v", s.JID, r)
		}
	}()

	s.gccallEnforce(evt.From, evt.CallCreator, evt.CallCreatorAlt, evt.CallID, evt.GroupJID)
}

// gccallEnforce is the SHARED enforcement core — both entry points
// (CallOfferNotice notice + CallOffer ring) call this. It:
//  1. checks antigccall on/off + action for the group
//  2. applies owner/premium bypass (silent pass)
//  3. applies the action: decline/ignore = silent, delete/kick = notify
func (s *Session) gccallEnforce(from, creator, creatorAlt types.JID, callID string, groupJID types.JID) {
	br := &bridge{s: s}

	// Only group calls carry a GroupJID — 1:1 calls stay in handleAntiCall.
	if groupJID.IsEmpty() {
		return
	}

	gid := groupJID.String()

	// Is antigccall ON for this group? Off — nothing, silent.
	on := goldcmds.AntigccallIsOn(br, gid)
	action := goldcmds.AntigccallAction(br, gid)
	if !on {
		return
	}

	// The call creator — param se aata hai (CallCreator; empty
	// ho to From fallback).
	if creator.IsEmpty() {
		creator = from
	}
	if creator.IsEmpty() {
		return
	}

	if callID == "" {
		return
	}

	// ── STAGE 3 — OWNER BYPASS ──
	// The owner's own group calls ALWAYS pass SILENTLY — no decline,
	// no notification. LID-tolerant via normalized JID + number-tail match.
	creatorIsOwner := s.gccallCreatorIsOwner(creator.String()) ||
		(!creatorAlt.IsEmpty() && s.gccallCreatorIsOwner(creatorAlt.String()))
	if creatorIsOwner {
		return
	}

	// ── STAGE 4 — PREMIUM BYPASS (.antigccallprem add) ──
	// Premium members' group calls are silently ignored — LID-tolerant
	// (exact JID + number-tail), exactly like anticallCallerIsPremium.
	creatorIsPremium := anticallCallerIsPremium(br, creator.String()) ||
		(!creatorAlt.IsEmpty() && anticallCallerIsPremium(br, creatorAlt.String()))
	if creatorIsPremium {
		return
	}

	// ── STAGE 6 ── WARN / KICK ── caller-only enforcement ──
	// FINAL SETUP (owner order): actions sirf warn + kick hain.
	//   • kick  ─ caller (group call banane wale) ko DIRECT remove.
	//   • warn  ─ caller ko warning, max warnings pe kick.
	// Kick hi call ko sab ke liye khatam karta hai: WhatsApp jab admin
	// kisi member ko group se remove karta hai aur wo member us waqt
	// group call me connected ho, to server us participant ko call room
	// se bhi force-kick kar deta hai ── creator nikal gaya to call girti
	// hai. (Protocol fact: <terminate> creator-gated hai ─ non-creator
	// ka terminate server drop karta hai, chahe wo participant ho.)
	creatorNum := botOwnNumber(creator.String())

	// ── KICK ── direct remove (antilink kick style)
	if action == "kick" {
		if s.Client != nil {
			_, _ = s.Client.UpdateGroupParticipants(context.Background(), groupJID,
				[]types.JID{creator}, whatsmeow.ParticipantChangeRemove)
		}
		notif := "*\U0001F530 DEAR @" + creatorNum + " REMOVED — GROUP CALL NOT ALLOWED*"
		if s.Client != nil && s.Client.IsConnected() {
			out := s.replyText(notif)
			_, _ = s.Client.SendMessage(context.Background(), groupJID, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(out),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{creator.String()},
					},
				},
			})
		}
		return
	}

	// ── WARN (default) ── warning + max pe kick (antilink warn style)
	maxW := goldcmds.AntigccallMaxWarnings(br, gid)
	newCount := goldcmds.AntigccallIncWarn(br, gid, creator.String())
	if newCount >= maxW {
		// Max warnings reached — kick
		if s.Client != nil {
			_, _ = s.Client.UpdateGroupParticipants(context.Background(), groupJID,
				[]types.JID{creator}, whatsmeow.ParticipantChangeRemove)
		}
		notif := "*\U0001F530 DEAR @" + creatorNum + " REMOVED — GROUP CALLS NOT ALLOWED IN THIS GROUP (MAX WARNINGS REACHED)*"
		if s.Client != nil && s.Client.IsConnected() {
			out := s.replyText(notif)
			_, _ = s.Client.SendMessage(context.Background(), groupJID, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(out),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{creator.String()},
					},
				},
			})
		}
		goldcmds.AntigccallResetWarn(br, gid, creator.String())
		return
	}
	// warning message (antilink warn style — @mention + count)
	notif := "*\U0001F530 DEAR @" + creatorNum + " GROUP CALLS NOT ALLOWED IN THIS GROUP*"
	notif += "\n*WARNING :❱ " + strconv.Itoa(newCount) + "/" + strconv.Itoa(maxW) + "*"
	if s.Client != nil && s.Client.IsConnected() {
		out := s.replyText(notif)
		_, _ = s.Client.SendMessage(context.Background(), groupJID, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(out),
				ContextInfo: &waProto.ContextInfo{
					MentionedJID: []string{creator.String()},
				},
			},
		})
	}
}

// gccallCreatorIsOwner reports whether the given group-call creator JID is
// one of the bot's owners (config OwnerSet / paired s.Owner / Redis sudo
// owners). LID-tolerant: exact normalized JID + number-tail match — the
// owner may appear as @lid in group call events while OwnerSet holds phone
// JIDs.
func (s *Session) gccallCreatorIsOwner(creatorJID string) bool {
	if creatorJID == "" {
		return false
	}
	norm := anticallNormalizeJID(creatorJID)
	if norm != "" && s.Manager != nil && s.Manager.cfg != nil && s.Manager.cfg.IsOwner(norm) {
		return true
	}
	// paired owner (s.Owner = phone number, e.g. "923158930864")
	if s.Owner != "" {
		ownerJID := normalizeJID(s.Owner)
		if ownerJID != "" {
			if norm != "" && (ownerJID == norm || botOwnNumber(ownerJID) == botOwnNumber(norm)) {
				return true
			}
			// number-tail match (LID tolerance)
			if tail := callerNumberTail(creatorJID); tail != "" && tail == callerNumberTail(ownerJID) {
				return true
			}
		}
	}
	// Redis sudo owners (.ownernumber add / .sudo add) + paired number
	if s.Manager != nil && s.Manager.Redis != nil && norm != "" {
		creatorNum := botOwnNumber(norm)
		if creatorNum != "" {
			if creatorNum == botOwnNumber(s.JID) {
				return true
			}
			rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
			if rawSudo != "" {
				for _, n := range strings.Split(rawSudo, ",") {
					if strings.TrimSpace(n) == creatorNum {
						return true
					}
				}
			}
		}
	}
	return false
}

// ══════════════════ (merged from antidelete_handler.go) ══════════════════
// ============================================================================
// GOLD-MD  —  ANTIDELETE / ANTIEDIT  event-capture handlers
// File: antidelete_handler.go
// ----------------------------------------------------------------------------
// ANTIDELETE: Storj recovery (10-shard S3) + event-based sender check +
//   Client.Upload send helpers. FULL JSON DEBUGGING (always-on).
// ANTIEDIT: UNTOUCHED original logic.
// ============================================================================

// mediaTypeLabel returns a human-readable label and the internal media
// type string for a recovered message.  When none of the known message
// payloads are present it returns mtype == "unknown".
//
// isUnknownMediaType / isUnsupportedMessage are used by the antidelete
// and antiedit handlers to SILENTLY IGNORE messages whose type the bot
// cannot meaningfully recover (status replies, ephemeral, view-once,
// system, reaction, etc.).  Previously such deletes still produced a
//
//	"*DELETED MESSAGE DETECTED* ... *(UNKNOWN message)*"
//
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
		header += fmt.Sprintf("\n\n*🔰 DELETED %s BELOW 🔰*", label)
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

	fullMsg := "*🔰 EDITED " + oldLabel + " DETECTED 🔰*\n" +
		"━━━━━━━━━━━━━━━━━\n\n" +
		"🔰 *FROM :* " + displayName + "\n" +
		"🔰 *DATE :* " + pkDate() + "\n" +
		"🔰 *TIME :* " + pkTime() + "\n" +
		"\n🔴 *OLD MSG :*\n> " + oldQuoted + "\n\n"
	if isEncrypted {
		fullMsg += "🔰 *New Edited msg :*\n> *see it on the user msg*\n\n"
	} else {
		fullMsg += "🔰 *New Edited msg :*\n> " + newQuoted + "\n\n"
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
	// ── ANTIDELETE MEDIA SHIELD (owner: jani, 2026-09-16) ──
	// Recovered media ko compressor room se guzaar kar chhota karo, taake
	// Render ka free 5GB outbound (re-upload) bache. Fail/no-gain → original.
	if _, mtype := mediaTypeLabel(msg); mtype != "" {
		if kind, okKind := guardAntideleteKind(mtype); okKind {
			if comp, _ := guardAntideleteBytes(kind, data); len(comp) > 0 {
				data = comp
			}
		}
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
	out := s.replyText(text)
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

// ══════════════════ (merged from newsletter.go) ══════════════════
// ============================================================================
// GOLD-MD — Newsletter channel system
//
// Mirrors the behaviour of pair.js's newsletter auto-follow + forwarded
// newsletter link button, but simplified to a SINGLE channel (the user
// asked to drop pair.js's 2-channel toggle and use just one).
//
//   • On every successful connect we try to FOLLOW the channel (only if the
//     bot is currently a GUEST = not yet following), with in-memory caching
//     so we never spam the follow request and risk a ban.
//   • The 4 core commands (.menu .ping .uptime .alive) attach a
//     "forwarded from channel" link button to their messages via
//     newsletterCtxInfo().  No other message gets this button.
// ============================================================================

// NewsletterChannelInviteKey is the invite key of the SINGLE channel the bot
// follows and advertises.  Taken from
// https://whatsapp.com/channel/0029Vb956DuHFxP4EhLJA316
const NewsletterChannelInviteKey = "0029Vb956DuHFxP4EhLJA316"

// NewsletterChannelName is the human-readable name shown on the forwarded
// link button.  It is refreshed from the live newsletter metadata when the
// bot resolves the invite, but we keep a sensible default here so the button
// works even before the first successful metadata fetch.
const NewsletterChannelName = "GOLD-MD Updates"

// newsletterState caches the resolved channel JID + name and whether we have
// already attempted a follow for this process.  This avoids hammering
// WhatsApp's newsletter API on every connect/reconnect (ban safety, exactly
// like pair.js's RAM cache).
var newsletterState = struct {
	sync.Mutex
	jid      string // resolved channel JID (e.g. 123@newsletter)
	name     string // live channel name
	followed bool   // true once we have successfully followed (or confirmed already following)
}{
	name: NewsletterChannelName,
}

// followNewsletterChannel resolves the channel invite key to a JID and follows
// the channel if the bot is not already a subscriber/admin.  Safe to call on
// every connect — the in-memory cache ensures we only act once per process.
func (s *Session) followNewsletterChannel() {
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Resolve invite key → newsletter JID + metadata.
	// GetNewsletterInfoWithInvite accepts the raw invite key.
	meta, err := s.Client.GetNewsletterInfoWithInvite(ctx, NewsletterChannelInviteKey)
	if err != nil {
		ErrLog("[newsletter] failed to resolve invite %s: %v", NewsletterChannelInviteKey, err)
		return
	}
	if meta == nil || meta.ID.IsEmpty() {
		ErrLog("[newsletter] resolved metadata empty for invite %s", NewsletterChannelInviteKey)
		return
	}

	channelJID := meta.ID.String()
	channelName := meta.ThreadMeta.Name.Text
	if channelName == "" {
		channelName = NewsletterChannelName
	}

	// Cache the resolved JID + name.
	newsletterState.Lock()
	newsletterState.jid = channelJID
	newsletterState.name = channelName
	alreadyFollowed := newsletterState.followed
	newsletterState.Unlock()

	OkLog("[newsletter] resolved channel %s (%s)", channelName, channelJID)

	if alreadyFollowed {
		// Already followed (or already confirmed following) this process.
		return
	}

	// GetNewsletterInfoWithInvite returns ViewerMeta=nil (per whatsmeow docs),
	// so to learn the viewer role we fetch the full info by JID.
	fullMeta, err := s.Client.GetNewsletterInfo(ctx, meta.ID)
	if err == nil && fullMeta != nil && fullMeta.ViewerMeta != nil {
		role := fullMeta.ViewerMeta.Role
		if role == types.NewsletterRoleSubscriber || role == types.NewsletterRoleAdmin || role == types.NewsletterRoleOwner {
			OkLog("[newsletter] already following channel (%s) — role %s", channelName, role)
			newsletterState.Lock()
			newsletterState.followed = true
			newsletterState.Unlock()
			return
		}
	}
	// If role lookup failed or role is GUEST, attempt to follow.

	if err := s.Client.FollowNewsletter(ctx, meta.ID); err != nil {
		ErrLog("[newsletter] failed to follow channel %s: %v", channelName, err)
		return
	}

	newsletterState.Lock()
	newsletterState.followed = true
	newsletterState.Unlock()
	OkLog("[newsletter] 🔰 followed channel %s (%s)", channelName, channelJID)
}

// ensureNewsletterResolved makes a best-effort SYNCHRONOUS attempt to resolve
// the channel JID when it has not been resolved yet.  The staggered background
// follow (wakeNewsletterFollow) only runs 10-30s after connect, so a .menu /
// .alive sent right after pairing (or right after a restart) would otherwise
// be sent BEFORE the channel is known and therefore carry NO forwarded channel
// link button.  Calling this before building the reply guarantees the button
// is attached on the very first menu too.  It is a no-op once the JID is known
// (the common case), so it costs nothing on the hot path.
func (s *Session) ensureNewsletterResolved() {
	newsletterState.Lock()
	have := newsletterState.jid != ""
	newsletterState.Unlock()
	if have {
		return
	}
	s.followNewsletterChannel()
}

// newsletterCtxInfo builds a *waProto.ContextInfo that, when attached to a
// message, renders WhatsApp's "forwarded from <channel>" link button.
//
// This is exactly what pair.js does with forwardingScore=999999 +
// isForwarded=true + forwardedNewsletterMessageInfo{newsletterJid,
// newsletterName, serverMessageId}.
//
// IMPORTANT: only the 4 core commands (.menu .ping .uptime .alive) use this.
func (s *Session) newsletterCtxInfo() *waProto.ContextInfo {
	newsletterState.Lock()
	jid := newsletterState.jid
	name := newsletterState.name
	newsletterState.Unlock()

	if jid == "" {
		// Channel not resolved yet — still attach the button using the known
		// invite-derived info so the button renders.  The JID may be empty on
		// the very first menu/ping before connect resolves the channel; in
		// that case we skip the button (no JID = no valid link).
		return nil
	}

	score := uint32(999999)
	serverID := int32(rand.Intn(900000) + 100000)
	contentType := waProto.ForwardedNewsletterMessageInfo_LINK_CARD
	accessText := fmt.Sprintf("View updates from %s", name)

	return &waProto.ContextInfo{
		ForwardingScore: &score,
		IsForwarded:     proto.Bool(true),
		ForwardedNewsletterMessageInfo: &waProto.ForwardedNewsletterMessageInfo{
			NewsletterJID:     &jid,
			NewsletterName:    &name,
			ServerMessageID:   &serverID,
			ContentType:       &contentType,
			AccessibilityText: &accessText,
		},
	}
}

// ReplyWithNewsletter sends a text reply with the forwarded newsletter channel
// link button attached.  Used ONLY by .menu .ping .uptime .alive.
//
// The botname footer is applied here (via withFooter) so that EVERY message
// routed through this helper — including .ping, .uptime and .alive — carries
// the consistent GOLD-MD bot signature, exactly like messages sent through
// the normal Reply() path. The footer is sanitized so the marker/control
// chars can never leak.
func (s *Session) ReplyWithNewsletter(info types.MessageInfo, text string) {
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}

	// Best-effort: make sure the channel JID is resolved so the forwarded
	// channel link button is attached even on the very first reply after a
	// restart/pairing (no-op once resolved).
	s.ensureNewsletterResolved()

	text = s.replyText(text)

	ctxInfo := s.newsletterCtxInfo()

	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ctxInfo,
		},
	}

	// If the channel hasn't been resolved yet, fall back to a plain
	// conversation message so the command still replies.
	if ctxInfo == nil {
		msg = &waProto.Message{Conversation: proto.String(text)}
	}

	if _, err := s.Client.SendMessage(context.Background(), info.Chat, msg); err != nil {
		ErrLog("[%s] newsletter reply failed: %v", s.JID, err)
	}
}

// SendAliveVideoWithNewsletter sends the owner-set bot video (.botvideo) for
// .alive / startup messages, captioned and carrying the forwarded channel
// button. The video is streamed to WhatsApp from a temp file (no full RAM
// buffering) and deleted afterwards, so large clips never accumulate on disk.
// Returns false when the caller should fall back to the image/text path.
func (s *Session) SendAliveVideoWithNewsletter(info types.MessageInfo, videoURL, caption string) bool {
	if s.Client == nil || !s.Client.IsConnected() || strings.TrimSpace(videoURL) == "" {
		return false
	}
	s.ensureNewsletterResolved()

	// A stored value may be a host SHARE page (qu.ax/O7xfZ) instead of a file;
	// resolve it to the real media URL first — uploading the page HTML is what
	// makes WhatsApp answer "this video is not available".
	videoURL = goldcmds.ResolveDirectMediaURL(videoURL)

	data, err := fetchMediaBytes(videoURL)
	if err != nil || len(data) == 0 || !goldcmds.IsPlayableMedia(data, "") {
		ErrLog("[%s] alive video download rejected (not playable media)", s.JID)
		return false
	}

	f, err := os.CreateTemp("", "goldmd-botvideo-*.mp4")
	if err != nil {
		return false
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return false
	}
	f.Close()

	seconds, width, height := guardProbeMeta(tmpPath)
	data = nil // release the buffer before the upload

	// ffmpeg static build may not be on PATH yet on a fresh boot; probing is
	// best-effort but WhatsApp plays sideways/black videos without it.
	goldcmds.EnsureFfmpegPublic()
	thumb := goldcmds.BotVideoThumbnail(tmpPath)

	f, err = os.Open(tmpPath)
	if err != nil {
		return false
	}
	defer f.Close()
	uploaded, err := s.Client.UploadReader(context.Background(), f, nil, whatsmeow.MediaVideo)
	if err != nil {
		ErrLog("[%s] alive video upload failed: %v", s.JID, err)
		return false
	}

	videoMsg := &waProto.VideoMessage{
		Caption:       proto.String(s.withCaptionFooter(caption)),
		Mimetype:      proto.String("video/mp4"),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uploaded.FileLength),
		Seconds:       proto.Uint32(seconds),
		Width:         proto.Uint32(width),
		Height:        proto.Uint32(height),
		JPEGThumbnail: thumb,
		ContextInfo:   s.newsletterCtxInfo(),
	}
	if _, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		VideoMessage: videoMsg,
	}); err != nil {
		ErrLog("[%s] alive video send failed: %v", s.JID, err)
		return false
	}
	return true
}

// fetchMediaBytes downloads any media URL with a status check, a timeout and a
// hard size cap, so a 404/HTML body can never be uploaded as a video.
func fetchMediaBytes(rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("empty media url")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// qu.ax/catbox 302 the default Go agent to an HTML page; a browser UA is
	// required to receive the actual file bytes.
	// qu.ax/catbox 302 a UA-less request to an HTML page and their file
	// endpoints are hotlink-protected, so BOTH a browser User-Agent and the
	// host Referer are required to receive the actual file bytes.
	req.Header.Set("User-Agent", goldcmds.BrowserUA)
	if ref := goldcmds.MediaReferer(rawURL); ref != "" {
		req.Header.Set("Referer", ref)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("media fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// ReplyImageWithNewsletter sends an image message with caption + the forwarded
// newsletter channel link button attached.  Used ONLY by .menu (the redesigned
// menu sends a header image with a fancy caption) and .alive (bot pic + alive
// msg). The botname footer is appended to the caption here (sanitized) so the
// marker/control chars can never leak and every image reply carries the
// consistent bot signature.
func (s *Session) ReplyImageWithNewsletter(info types.MessageInfo, imgData []byte, caption string) bool {
	if s.Client == nil || !s.Client.IsConnected() {
		return false
	}

	// Best-effort: resolve the channel JID so the forwarded channel link
	// button is attached even on the very first menu after a restart/pairing
	// (no-op once resolved).
	s.ensureNewsletterResolved()

	caption = s.withCaptionFooter(caption)

	uploaded, err := s.Client.Upload(context.Background(), imgData, whatsmeow.MediaImage)
	if err != nil {
		ErrLog("[%s] menu image upload failed: %v", s.JID, err)
		return false
	}

	ctxInfo := s.newsletterCtxInfo()

	imgMsg := &waProto.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(imageMimeForBytes(imgData)),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(imgData))),
		ContextInfo:   ctxInfo,
	}

	if _, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		ImageMessage: imgMsg,
	}); err != nil {
		ErrLog("[%s] menu image send failed: %v", s.JID, err)
		return false
	}
	return true
}

// SendVoiceWithNewsletter sends the owner-set menu/alive VOICE as an audio
// message carrying the forwarded channel link button. Sent as a normal audio
// message (not a PTT voice note): the stored files are MP3, and WhatsApp only
// renders a PTT note correctly for opus/ogg, so playing MP3 as audio is the
// form that actually works. Returns false when nothing could be sent.
func (s *Session) SendVoiceWithNewsletter(info types.MessageInfo, data []byte) bool {
	if s.Client == nil || !s.Client.IsConnected() || len(data) == 0 {
		return false
	}
	s.ensureNewsletterResolved()

	uploaded, err := s.Client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		ErrLog("[%s] menu voice upload failed: %v", s.JID, err)
		return false
	}

	ctxInfo := s.newsletterCtxInfo()

	audioMsg := &waProto.AudioMessage{
		Mimetype:      proto.String("audio/mpeg"),
		PTT:           proto.Bool(false),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(data))),
		ContextInfo:   ctxInfo,
	}

	if _, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		AudioMessage: audioMsg,
	}); err != nil {
		ErrLog("[%s] menu voice send failed: %v", s.JID, err)
		return false
	}
	return true
}

// wakeNewsletterFollow kicks off the channel follow in the background with a
// small random delay (mirrors pair.js's staggered follow to look human and
// avoid ban risk).  Called from EventHandler's *events.Connected case.
func (s *Session) wakeNewsletterFollow() {
	go func() {
		// small stagger so we don't follow the instant we connect
		time.Sleep(time.Duration(10+rand.Intn(20)) * time.Second)
		s.followNewsletterChannel()
	}()
}

// ══════════════════ (merged from aivideo_dispatch.go) ══════════════════
// hasPendingAIVideoSession reports whether the given sender has an active
// aivideo multi-image collection session pending in the gold-cmds package.
func hasPendingAIVideoSession(sender string) bool {
	return goldcmds.HasPendingAIVideoSession(sender)
}

// dispatchAIVideoMedia is called by HandleMessage for every incoming message
// (including image messages) when a pending aivideo session exists for the
// sender. It builds a bridge, detects whether the message carries an image,
// extracts any text, and forwards both to the gold-cmds pending-session
// handler. Returns true if the message was consumed by a session.
func dispatchAIVideoMedia(s *Session, info types.MessageInfo, msg *waProto.Message) bool {
	b := &bridge{s: s}
	hasImg := extractImageMessage(msg) != nil
	text := strings.TrimSpace(getText(msg))
	// Also pick up caption text from image messages (getText ignores captions).
	if text == "" && msg != nil {
		if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ImageMessage.Caption)
		} else if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil &&
			msg.ViewOnceMessage.Message.ImageMessage != nil &&
			msg.ViewOnceMessage.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessage.Message.ImageMessage.Caption)
		} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessageV2.Message.ImageMessage.Caption)
		}
	}
	return goldcmds.DispatchAIVideoIncoming(b, info, text, hasImg)
}

// hasPendingAIVideo2Session reports whether the given sender has an active
// aivideo2 multi-image collection session pending in the gold-cmds package.
func hasPendingAIVideo2Session(sender string) bool {
	return goldcmds.HasPendingAIVideo2Session(sender)
}

// dispatchAIVideo2Media is the aivideo2 counterpart of dispatchAIVideoMedia.
// It is called by HandleMessage for every incoming message when a pending
// aivideo2 session exists for the sender. Returns true if consumed.
func dispatchAIVideo2Media(s *Session, info types.MessageInfo, msg *waProto.Message) bool {
	b := &bridge{s: s}
	hasImg := extractImageMessage(msg) != nil
	text := strings.TrimSpace(getText(msg))
	// Pick up caption text from image messages (getText ignores captions).
	if text == "" && msg != nil {
		if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ImageMessage.Caption)
		} else if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil &&
			msg.ViewOnceMessage.Message.ImageMessage != nil &&
			msg.ViewOnceMessage.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessage.Message.ImageMessage.Caption)
		} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessageV2.Message.ImageMessage.Caption)
		}
	}
	return goldcmds.DispatchAIVideo2Incoming(b, info, text, hasImg)
}

// ══════════════════ (merged from fleet_commands.go) ══════════════════
// ═════════════════════════════════════════════════════════════════════════════
//   GOLD-MD — SERVER COMMANDS
//
//   OWNER ORDER (latest):
//   ".server command ko public bana do — koi bhi servers pair dekh ske
//    easily. Jo jo server OFFLINE ho, online servers ke bad \n\n\n mar ke
//    choti single line me line-by-line dikhta jaye:
//      *SERVER ❮ {server number} ❯ OFFLINE*"
//
//   Commands:
//     .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//     .sessions → PUBLIC servers menu (koi bhi chala sakta hai — pairing
//                 status sabko dikhta hai). .menu me ab bhi hidden.
//     .host5gb  → 5GB bandwidth report — OWNER-ONLY command,
//                 bilkul baaki owner commands jaisa (sirf owner / sudo
//                 owner / bot ka apna account). Non-owner: silent ignore
//                 (menu me visible).
//
//   PUBLIC vs PRIVATE split:
//     • Pairing/status data (servers.json ke 200 servers ka /health) →
//       PUBLIC — koi b dekh sakta hai.
//     • LOCAL SESSIONS detail (asli WhatsApp JIDs / phone numbers) →
//       server report me KABHI nahi — private numbers leak nahi hote.
//
//   OFFLINE FORMAT (owner ka exact order): online servers ke full INFO
//   blocks ke baad \n\n\n (3 enter) lagta hai, phir har offline server sirf
//   EK choti line me:  *SERVER ❮ N ❯ OFFLINE*
// ═════════════════════════════════════════════════════════════════════════════

// isFleetOwnerCommand: .host5gb / .svrchange ka owner-check — bilkul
// handler.go ke owner-check ke SAARE layers (owner order: "dusre cmnds
// jese"):
//  1. cfg.IsOwner(sender)  — GOLDMD_OWNER_NUMBERS + paired owner set
//  2. sender == bot JID    — bot ka apna account (exact JID)
//  3. bare-number match    — bot ka apna number (device-suffix strip,
//     multi-device safe)
//  4. Redis sudo owners    — .ownernumber add / .sudo add wale numbers
//  5. SenderAlt layers     — LID chat me alt form = phone JID (sab
//     upar wale checks alt form pe bhi)
//  6. IsFromMe             — bot owner ke apne device se bheja message
//
// Nil-safe: Manager/cfg/Redis nil ho (tests / edge cases) to sirf layers
// 2/3/5/6 chalete hain — panic kabhi nahi.
func isFleetOwnerCommand(s *Session, info types.MessageInfo) bool {
	sender := ""
	if !info.Sender.IsEmpty() {
		sender = info.Sender.String()
	}

	// 6) bot owner ke apne device se (self-chat / own device)
	if info.IsFromMe {
		return true
	}

	if s == nil {
		return false
	}

	// 2) sender bot ka hi JID hai (exact form)
	if sender != "" && sender == s.JID {
		return true
	}

	// 3) sender bot ka hi number hai (device-suffix tolerant —
	//    bot ke dusre device se bheja ho to bhi owner)
	botNum := botOwnNumber(s.JID)
	if botNum != "" && sender != "" && botOwnNumber(sender) == botNum {
		return true
	}

	// 5a) LID chat: SenderAlt bot ka hi phone JID hai (alt form bina
	//     :device suffix ke, s.JID me suffix ho sakta hai)
	if info.SenderAlt.Server != "" {
		alt := info.SenderAlt.String()
		if alt == s.JID || (botNum != "" && botOwnNumber(alt) == botNum) {
			return true
		}
	}

	// Manager ke bina layers 1/4 nahi chale sakte
	if s.Manager == nil || s.Manager.cfg == nil {
		return false
	}

	// 1) owner set (GOLDMD_OWNER_NUMBERS env + paired owner normalization)
	if sender != "" && s.Manager.cfg.IsOwner(sender) {
		return true
	}

	// 4) Redis sudo owners (.ownernumber add / .sudo add)
	if sender != "" && s.Manager.Redis != nil {
		num := botOwnNumber(sender)
		rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
		if num != "" && rawSudo != "" {
			for _, n := range strings.Split(rawSudo, ",") {
				if strings.TrimSpace(n) == num {
					return true
				}
			}
		}
	}

	// 5b) SenderAlt (LID <-> phone JID mapping) — owner-set + sudo lookup
	if info.SenderAlt.Server != "" {
		alt := info.SenderAlt.String()
		if s.Manager.cfg.IsOwner(alt) {
			return true
		}
		if s.Manager.Redis != nil {
			altNum := botOwnNumber(alt)
			rawSudo := s.Manager.Redis.GetSetting(s.JID, "sudowners", "")
			if altNum != "" && rawSudo != "" {
				for _, n := range strings.Split(rawSudo, ",") {
					if strings.TrimSpace(n) == altNum {
						return true
					}
				}
			}
		}
	}
	return false
}

// hiddenCommands: Commands-map me registered par .menu me KABHI nahi dikhne
// wale secret commands. OWNER ORDER: .host5gb aur .svrchange ab bot se
// DELETE hain (na register, na is set me).
// Server-menu family ab PUBLIC hai (koi bhi chala sakta hai) par .menu
// me ab bhi nahi dikhti (secret rahegi, sirf wahi jaanne wale use karenge).
var hiddenCommands = map[string]bool{
	"m":          true, // menu ka hidden alias — kaam karta hai, menu me nahi dikhta
	"servers":    true,
	"svr":        true,
	"svrinfo":    true,
	"serverinfo": true,
	"session":    true,
	"sessions":   true,
}

func init() {
	// OWNER ORDER: .host5gb aur .svrchange bot se DELETE kar diye gaye —
	// na register hote hain, na owner-only set me. (Unke helpers source me
	// rakhe gaye hain taake dobara jaldi wapas add kiye ja sakein.)

	// .server + aliases — PUBLIC servers menu (owner order: koi bhi
	// servers pair dekh ske). Koi guard nahi — sabke liye open.
	for _, name := range []string{"server", "servers", "svr", "svrinfo", "serverinfo", "session", "sessions"} {
		nm := name
		RegisterCommand(nm, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			s.CmdServerMenu(info, args, prefix)
		})
	}
}

// ── .host5gb — REAL bandwidth report (all running servers) ──
func (s *Session) CmdHost5GB(info types.MessageInfo, args []string, prefix string) {
	s.Reply(info, fleetRender5GBText())
}

// ── .server / .servers / .svr / .svrinfo / .serverinfo / .session /
//
//	.sessions — PUBLIC servers menu (owner ka naya offline format) ──
//
// OWNER ORDER: ek hi fleetScanAll() call (200 servers × parallel /health,
// 4s timeout — QUICK). Online servers full INFO block me, offline servers
// \n\n\n ke baad single choti lines me.
func (s *Session) CmdServerMenu(info types.MessageInfo, args []string, prefix string) {
	// OWNER ORDER: HAR call pe FRESH /health scan — koi cached (fake) data nahi.
	s.Reply(info, fleetServersMenuText(fleetScanAllFresh()))
}

// fleetServersMenuText builds the .server report from a scan slice (pure —
// network-free, unit-testable). OWNER ORDER format:
//
//	*ACTIVE BOTS :❯ ❮ {N} ❯*      ← total live pairings (jitne bot active)
//	*ONLINE SERVERS :❯ ❮ {x} ❯*
//	*OFFLINE SERVERS :❯ ❮ {x} ❯*
//
//	*SERVER 1 ACTIVE 1/2*
//	*SERVER 2 ACTIVE 0/2*
//	...
//	*SERVER 5 OFFLINE*
//	*SERVER 6 OFFLINE*
func fleetServersMenuText(scan []fleetServerInfo) string {
	var b strings.Builder

	online, offline, pairs := 0, 0, 0
	for _, srv := range scan {
		if srv.Online {
			online++
			pairs += srv.Sessions
		} else {
			offline++
		}
	}

	b.WriteString(fmt.Sprintf("*ACTIVE BOTS :❯ ❮ %d ❯*\n", pairs))
	b.WriteString(fmt.Sprintf("*ONLINE SERVERS :❯ ❮ %d ❯*\n", online))
	b.WriteString(fmt.Sprintf("*OFFLINE SERVERS :❯ ❮ %d ❯*\n\n", offline))

	// ONLINE servers — number order, ek line each.
	for _, srv := range scan {
		if !srv.Online {
			continue
		}
		paired := "0/2"
		if srv.Max > 0 {
			paired = fmt.Sprintf("%d/%d", srv.Sessions, srv.Max)
		}
		b.WriteString(fmt.Sprintf("*SERVER %s ACTIVE %s*\n", fleetServerNumber(srv.Name), paired))
	}

	// OFFLINE servers — number order, ek line each.
	for _, srv := range scan {
		if srv.Online {
			continue
		}
		b.WriteString(fmt.Sprintf("*SERVER %s OFFLINE*\n", fleetServerNumber(srv.Name)))
	}

	return b.String()
}
func fleetServerNumber(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) >= 2 {
		return fields[len(fields)-1]
	}
	return name
}

// fleetServerNumberForSID: kisi bhi server sid/URL se servers.json ka
// server NUMBER nikaalta hai ("SERVER 3" -> "3"). Match chain:
//  1. GOLDMD_SERVER_ID / sid          (exact)
//  2. fleetServerURL(sid) == entry URL (http/https + trailing /)
//  3. sid base-hostname == entry URL host (onrender wale short names)
//
// Na mile to "0".
func fleetServerNumberForSID(sid string) string {
	loadServersConfig()
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return "0"
	}
	wantURL := fleetServerURL(sid)
	// base-hostname (dots + scheme strip) — "https://gold-x.onrender.com/" -> "gold-x.onrender.com"
	baseSid := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(sid, "https://"), "http://"), "/")
	for _, e := range serversCfg.Servers {
		if sid == e.URL {
			return fleetServerNumber(e.Name)
		}
		if wantURL != "" && wantURL == strings.TrimSuffix(e.URL, "/") {
			return fleetServerNumber(e.Name)
		}
		eBase := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(e.URL, "https://"), "http://"), "/")
		if baseSid != "" && baseSid == eBase {
			return fleetServerNumber(e.Name)
		}
	}
	return "0"
}
