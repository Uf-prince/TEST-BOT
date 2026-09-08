package main

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

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow/types/events"
	goldcmds "gold-md/gold-cmds"
)

// handleAntiCall processes an incoming call offer and rejects it if
// anticall is enabled. Mirrors the Node.js `umar.ev.on('call', ...)` logic.
func (s *Session) handleAntiCall(evt *events.CallOffer) {
	// Top-level panic guard — same as EventHandler.
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] recovered panic in handleAntiCall: %v", s.JID, r)
		}
	}()

	// Check if anticall is ON for this bot.
	br := &bridge{s: s}
	if !br.GetAntiCallSetting() {
		return
	}

	// Skip group calls — only reject 1:1 (inbox) calls.
	// In whatsmeow, GroupJID is empty for 1:1 calls. Group calls come
	// as *events.CallOfferNotice (handled separately in EventHandler),
	// but double-check here for safety.
	if !evt.GroupJID.IsEmpty() {
		// DebugLog("[%s] anticall: skipping group call from %s (GroupJID=%s)",
		// s.JID, evt.From.String(), evt.GroupJID.String())
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
