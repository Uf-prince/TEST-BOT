package goldcmds

// ============================================================================
// GOLD-MD — .unblock command (WhatsApp REAL unblock)
// File: unblock.go
// ============================================================================
// Ported from UMAR-MD .block pattern (unblock mirror) — SAME WORK:
//   .unblock (reply)    → unblock the replied-to user
//   .unblock @mention   → unblock the mentioned user (works in groups)
//   .unblock 923xxx     → unblock by number
//   .unblock (inbox)    → unblock the chat itself
//
// Owner-only. Checks the REAL WhatsApp block list first (not blocked →
// says so). Uses whatsmeow UpdateBlocklist with
// BlocklistChangeActionUnblock (Baileys updateBlockStatus 'unblock').
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow/types"
	evt "go.mau.fi/whatsmeow/types/events"
)

func handleUnblock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleUnblockAsync(s, info, args, prefix)
}

func handleUnblockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// ── OWNER CHECK ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	// ── GET TARGET JID (same 4 methods as .block: reply → mention →
	// number → chat itself; mention works in groups via GetMentionedJIDs) ──
	targetJID, targetNumber, methodUsed := resolveBlockTarget(s, info, args)

	if targetJID == "" {
		s.Reply(info,
			"*🔰 UNBLOCK COMMAND INFO 🔰*\n\n"+
				"*4 METHODS TO UNBLOCK:*\n\n"+
				"*1. REPLY:* Reply to user's message and type "+prefix+"unblock\n"+
				"*2. MENTION:* "+prefix+"unblock @username\n"+
				"*3. NUMBER:* "+prefix+"unblock 923xxxxxxxxx\n"+
				"*4. CHAT:* "+prefix+"unblock (in inbox — unblocks the chat")
		return
	}

	// Normalize to @s.whatsapp.net when it has no server part
	if !strings.Contains(targetJID, "@") {
		targetJID = targetNumber + "@s.whatsapp.net"
	}

	// ── CHECK IF TARGET IS BOT ──
	botJIDNum := digitsOnly(strings.SplitN(s.GetJID(), ":", 2)[0])
	if targetNumber == botJIDNum {
		s.Reply(info, "❌ *CANNOT UNBLOCK MYSELF*")
		return
	}

	// ── CHECK IF TARGET IS OWNER ──
	for _, owner := range blockBuildOwnerNumbers(s) {
		if digitsOnly(owner) == targetNumber {
			s.Reply(info, "❌ *CANNOT UNBLOCK OWNER*")
			return
		}
	}

	// ── CHECK IF NOT BLOCKED (REAL WhatsApp block list) ──
	// LID-aware: WhatsApp stores the block list by @lid, so every @lid
	// entry is resolved to its real phone number before comparing, and we
	// get back the EXACT server-stored JID to unblock (guaranteed match).
	blockedJID, isBlocked := blockFindBlockedJID(s, targetJID, targetNumber)
	if !isBlocked {
		s.Reply(info, "*🔰 USER NOT BLOCKED 🔰*\n\n*🔰 NUMBER :➭ +"+targetNumber+"*")
		return
	}

	// ── UNBLOCK ──
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "❌ *FAILED TO UNBLOCK*\n*+"+targetNumber+"*\n*ERROR: client not connected*")
		return
	}
	// Use the exact JID WhatsApp itself stored in the server block list
	// (the @lid form on LID-mode servers) — UpdateBlocklist accepts the
	// LID directly and unblocks it, even when the PN form fails to match.
	unblockJID := blockedJID
	if unblockJID.IsEmpty() {
		jid, err := types.ParseJID(targetJID)
		if err != nil {
			s.Reply(info, "❌ *FAILED TO UNBLOCK*\n*+"+targetNumber+"*\n*ERROR: invalid JID*")
			return
		}
		unblockJID = jid
	}
	if _, err := cli.UpdateBlocklist(context.Background(), unblockJID, evt.BlocklistChangeActionUnblock); err != nil {
		s.Reply(info, "❌ *FAILED TO UNBLOCK*\n*+"+targetNumber+"*\n*ERROR: "+err.Error()+"*")
		return
	}

		s.Reply(info,
		"*🔰 USER UNBLOCKED 🔰*\n\n"+
			"*NUMBER:* +"+targetNumber+"\n"+
			"*METHOD:* "+strings.ToUpper(methodUsed)+"\n\n"+
			"")
}

// ── registration ────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "unblock", Category: "OWNER & SYSTEM", Desc: "Unblock a user on WhatsApp (reply/mention/number/chat)", OwnerOnly: true, Run: handleUnblock})
	Register(Command{Name: "unb", OwnerOnly: true, Hidden: true, Run: handleUnblock})
}
