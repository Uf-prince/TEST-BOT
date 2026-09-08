package goldcmds

// ============================================================================
// GOLD-MD — LID → PN resolution helper (shared)
// File: lidresolve.go
// ============================================================================
// On LID-mode WhatsApp servers, incoming JIDs arrive as @lid (14-digit
// Linked IDs) instead of real phone numbers (@s.whatsapp.net). This
// helper resolves a @lid JID to the real phone-number JID so commands
// like .block / .unblock always show REAL numbers.
//
// Resolution chain (fastest first):
//   1. info.SenderAlt — the LID↔PN alternative address attached to the
//      message itself by WhatsApp (participant_pn / sender_pn attr)
//   2. cli.Store.LIDs.GetPNForLID — sqlite LID cache (auto-filled from
//      incoming messages via whatsmeow StoreLIDPNMapping — when the
//      target user has EVER messaged, their mapping is already cached)
//   3. cli.GetUserInfo usync — live server resolve (also fills the cache)
// ============================================================================
// NOTE on the UpdateBlocklist API call: whatsmeow handles BOTH forms —
// a @lid jid is used as-is (it resolves the PN internally for pn_jid),
// and a PN jid is resolved to its LID internally. So the API call keeps
// the RAW jid; resolution is only needed for DISPLAY (real number).
// ============================================================================

import (
	"context"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// lidResolveTimeout bounds each LID→PN resolve attempt (DB + usync).
const lidResolveTimeout = 10 * time.Second

// IsLIDJID reports whether the given JID string is a @lid JID.
func IsLIDJID(jid string) bool {
	return strings.HasSuffix(jid, "@lid")
}

// ResolveLIDToPN resolves a @lid JID string to the real @s.whatsapp.net
// phone-number JID string. Non-LID input is returned unchanged.
// Best-effort: on failure returns the input unchanged.
func ResolveLIDToPN(s SessionBridge, info types.MessageInfo, jid string) string {
	if !IsLIDJID(jid) {
		return jid // already a phone-number JID
	}
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		return jid
	}

	// ── layer 1: info.SenderAlt (attached to THIS message) ──
	// Only helps when the target IS the message sender.
	senderStr := info.Sender.String()
	senderAlt := info.SenderAlt.String()
	if senderStr != "" && jid == senderStr && senderAlt != "" && !IsLIDJID(senderAlt) {
		return senderAlt
	}

	lidJID, err := types.ParseJID(jid)
	if err != nil || lidJID.IsEmpty() {
		return jid
	}

	// ── layer 2: sqlite LID cache (Store.LIDs) ──
	if cli.Store != nil && cli.Store.LIDs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), lidResolveTimeout)
		pn, err := cli.Store.LIDs.GetPNForLID(ctx, lidJID)
		cancel()
		if err == nil && !pn.IsEmpty() && pn.Server == types.DefaultUserServer {
			return pn.String()
		}
	}

	// ── layer 3: live usync resolve (fills the cache too) ──
	// GetUserInfo response entries are keyed PN→LID normally; when the
	// server echoes our LID as the key, the LID field is useless — so we
	// scan for an entry whose LID matches ours and take its PN key.
	ctx, cancel := context.WithTimeout(context.Background(), lidResolveTimeout)
	defer cancel()
	infos, err := cli.GetUserInfo(ctx, []types.JID{lidJID})
	if err == nil {
		for pnKey, ui := range infos {
			if pnKey.Server == types.DefaultUserServer && !ui.LID.IsEmpty() && ui.LID.User == lidJID.User {
				return pnKey.String()
			}
		}
	}

	return jid // unresolvable — return unchanged (best-effort)
}

// ResolveTargetLID normalizes a target JID for display purposes:
// @lid → real PN. Non-LID input is returned unchanged.
func ResolveTargetLID(s SessionBridge, info types.MessageInfo, targetJID string) string {
	if IsLIDJID(targetJID) {
		return ResolveLIDToPN(s, info, targetJID)
	}
	return targetJID
}

// lidDisplayNumber returns the REAL phone number digits for a target JID:
// resolves @lid → PN first, then strips the JID down to digits.
// Falls back to the raw digits when resolution fails.
func lidDisplayNumber(s SessionBridge, info types.MessageInfo, targetJID, rawNumber string) string {
	resolved := ResolveTargetLID(s, info, targetJID)
	num := digitsOnly(strings.SplitN(resolved, "@", 2)[0])
	if num != "" {
		return num
	}
	return rawNumber
}
