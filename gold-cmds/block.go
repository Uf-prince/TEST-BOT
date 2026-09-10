package goldcmds

// ============================================================================
// GOLD-MD — .block command (WhatsApp REAL block)
// File: block.go
// ============================================================================
// Ported from UMAR-MD plugins/block.js — SAME WORK (0% farak):
//   .block (reply)    → block the replied-to user
//   .block @mention   → block the mentioned user
//   .block 923xxx     → block by number
//   .block (inbox)    → block the chat itself
//
// Owner-only. Won't block the bot itself or any owner. Checks the REAL
// WhatsApp block list first (already blocked → says so).
// Uses whatsmeow GetBlocklist + UpdateBlocklist (Baileys fetchBlocklist +
// updateBlockStatus equivalent).
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	evt "go.mau.fi/whatsmeow/types/events"
)

// blockBuildOwnerNumbers returns the owner JIDs (bot + Redis sudo owners)
// as digits-based JIDs, mirroring the Node.js ownerNumbers array.
func blockBuildOwnerNumbers(s SessionBridge) []string {
	out := []string{}
	botNum := digitsOnly(strings.SplitN(s.GetJID(), ":", 2)[0])
	if botNum != "" {
		out = append(out, botNum+"@s.whatsapp.net")
	}
	for _, num := range s.GetSudoOwners() {
		n := digitsOnly(num)
		if n != "" {
			out = append(out, n+"@s.whatsapp.net")
		}
	}
	return out
}

// resolveBlockTarget finds the target JID + number using the 4 methods
// (reply → mention → number → chat itself). Returns jid, number, method.
func resolveBlockTarget(s SessionBridge, info types.MessageInfo, args []string) (string, string, string) {
	// METHOD 1: Reply to user's message
	if qid, qsender, ok := s.GetQuotedMessageID(info); ok && qid != "" {
		target := qsender
		if target == "" {
			// Quoted message in a private chat has no participant —
			// the quoted sender IS the chat itself.
			if !info.IsGroup {
				target = info.Chat.String()
			}
		}
		if target != "" {
			// LID server: participant arrives as @lid — resolve to the
			// REAL phone-number JID for both display and API call.
			target = ResolveTargetLID(s, info, target)
			num := digitsOnly(strings.SplitN(target, "@", 2)[0])
			if num != "" {
				if !strings.Contains(target, "@") {
					return num + "@s.whatsapp.net", num, "reply"
				}
				return target, num, "reply"
			}
		}
	}

	// METHOD 2: Mention (@user) — works in groups; on LID servers the
	// mentionedJid arrives as @lid — resolve to the REAL phone-number JID.
	if mentioned, ok := s.GetMentionedJIDs(info); ok && len(mentioned) > 0 {
		target := mentioned[0]
		target = ResolveTargetLID(s, info, target)
		num := digitsOnly(strings.SplitN(target, "@", 2)[0])
		if num != "" {
			if !strings.Contains(target, "@") {
				return num + "@s.whatsapp.net", num, "mention"
			}
			return target, num, "mention"
		}
	}

	// METHOD 3: Number in args (.block 923xxxx)
	for _, arg := range args {
		clean := digitsOnly(arg)
		if len(clean) >= 7 && len(clean) <= 15 {
			return clean + "@s.whatsapp.net", clean, "number"
		}
	}

	// METHOD 4: Block the chat itself (inbox only). On LID servers the
	// chat JID is @lid — resolve to the REAL phone-number JID.
	if !info.IsGroup {
		target := info.Chat.String()
		target = ResolveTargetLID(s, info, target)
		num := digitsOnly(strings.SplitN(target, "@", 2)[0])
		if num != "" {
			if !strings.Contains(target, "@") {
				return num + "@s.whatsapp.net", num, "self"
			}
			return target, num, "self"
		}
	}

	return "", "", "none"
}

// blockAlreadyBlocked checks the REAL WhatsApp block list for a JID
// (number-tolerant: matches on last-10-digit tails too).
func blockAlreadyBlocked(s SessionBridge, targetJID, targetNumber string) bool {
	_, found := blockFindBlockedJID(s, targetJID, targetNumber)
	return found
}

// ── LID-aware blocklist matching (fix for .unblock saying
// "USER NOT BLOCKED" when the user IS blocked) ──
//
// WHY: WhatsApp servers store the block list by LID — whatsmeow's
// UpdateBlocklist puts the user's @lid JID in the list (jid attr) and
// only adds pn_jid on the BLOCK action. GetBlocklist/parseBlocklist only
// reads the jid attr, so every entry comes back as a 14-digit @lid JID.
// Comparing those @lid entries against the target's phone number never
// matches → .unblock wrongly said "USER NOT BLOCKED".
//
// FIX: for every @lid entry in the server block list, resolve it to the
// real @s.whatsapp.net phone number (LID store cache first, live usync
// lookup as fallback — same 3-layer chain as lidresolve.go) and compare
// BOTH the raw entry and the resolved PN against the target. Return the
// exact server-stored JID so the caller can unblock by the identifier
// WhatsApp itself stored (guaranteed match, cache or no cache).
func blockFindBlockedJID(s SessionBridge, targetJID, targetNumber string) (types.JID, bool) {
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		return types.JID{}, false
	}
	bl, err := cli.GetBlocklist(context.Background())
	if err != nil || bl == nil {
		return types.JID{}, false
	}
	tail := digitsOnly(targetNumber)
	if len(tail) > 10 {
		tail = tail[len(tail)-10:]
	}
	tgt := digitsOnly(strings.SplitN(targetJID, "@", 2)[0])
	if len(tgt) > 10 {
		tgt = tgt[len(tgt)-10:]
	}

	for _, j := range bl.JIDs {
		entry := j.ToNonAD()
		// direct JID / raw-digit match (PN-form entries)
		if entry.String() == targetJID || entry.String() == strings.SplitN(targetJID, "@", 2)[0] {
			return entry, true
		}
		et := digitsOnly(entry.String())
		if len(et) > 10 {
			et = et[len(et)-10:]
		}
		if et != "" && (et == tail || (tgt != "" && et == tgt)) {
			return entry, true
		}
		// LID entry → resolve to the real phone number, then match.
		// This is the path that actually fires on LID-mode servers.
		if entry.Server == types.HiddenUserServer {
			if pn, ok := blockResolveLIDToPN(cli, entry); ok {
				pt := digitsOnly(pn.String())
				if len(pt) > 10 {
					pt = pt[len(pt)-10:]
				}
				if pt != "" && (pt == tail || (tgt != "" && pt == tgt)) {
					return entry, true
				}
			}
		}
	}
	return types.JID{}, false
}

// blockResolveLIDToPN resolves one blocked @lid JID to its real
// @s.whatsapp.net JID: sqlite LID cache (Store.LIDs) first, then a live
// GetUserInfo usync lookup (which also fills the cache). Best-effort.
func blockResolveLIDToPN(cli *whatsmeow.Client, lid types.JID) (types.JID, bool) {
	// layer 1: sqlite LID cache (auto-filled from incoming messages)
	if cli.Store != nil && cli.Store.LIDs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), lidResolveTimeout)
		pn, err := cli.Store.LIDs.GetPNForLID(ctx, lid)
		cancel()
		if err == nil && !pn.IsEmpty() && pn.Server == types.DefaultUserServer {
			return pn, true
		}
	}
	// layer 2: live usync resolve (GetUserInfo fills the LID cache too)
	ctx, cancel := context.WithTimeout(context.Background(), lidResolveTimeout)
	defer cancel()
	infos, err := cli.GetUserInfo(ctx, []types.JID{lid})
	if err == nil {
		for pnKey, ui := range infos {
			if pnKey.Server == types.DefaultUserServer && !ui.LID.IsEmpty() && ui.LID.User == lid.User {
				return pnKey, true
			}
		}
	}
	return types.JID{}, false
}

func handleBlock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBlockAsync(s, info, args, prefix)
}

func handleBlockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// ── OWNER CHECK ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	// ── GET TARGET JID ──
	targetJID, targetNumber, methodUsed := resolveBlockTarget(s, info, args)

	if targetJID == "" {
		s.Reply(info,
			"*🔰 BLOCK COMMAND INFO 🔰*\n\n"+
				"*4 METHODS TO BLOCK:*\n\n"+
				"*1. REPLY:* Reply to user's message and type "+prefix+"block\n"+
				"*2. MENTION:* "+prefix+"block @username\n"+
				"*3. NUMBER:* "+prefix+"block 923xxxxxxxxx\n"+
				"*4. CHAT:* "+prefix+"block (in inbox — blocks the chat")
		return
	}

	// Normalize to @s.whatsapp.net when it has no server part
	if !strings.Contains(targetJID, "@") {
		targetJID = targetNumber + "@s.whatsapp.net"
	}

	// ── CHECK IF TARGET IS BOT ──
	botJIDNum := digitsOnly(strings.SplitN(s.GetJID(), ":", 2)[0])
	if targetNumber == botJIDNum {
		s.Reply(info, "🔰 *CANNOT BLOCK MYSELF*")
		return
	}

	// ── CHECK IF TARGET IS OWNER ──
	for _, owner := range blockBuildOwnerNumbers(s) {
		if digitsOnly(owner) == targetNumber {
			s.Reply(info, "🔰 *CANNOT BLOCK OWNER*")
			return
		}
	}

	// ── CHECK IF ALREADY BLOCKED (REAL WhatsApp block list) ──
	if blockAlreadyBlocked(s, targetJID, targetNumber) {
		s.Reply(info, "*🔰 USER ALREADY BLOCKED 🔰*\n\n*🔰 NUMBER :➭ +"+targetNumber+"*")
		return
	}

	// ── BLOCK ──
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "🔰 *FAILED TO BLOCK*\n*+"+targetNumber+"*\n*ERROR: client not connected*")
		return
	}
	jid, err := types.ParseJID(targetJID)
	if err != nil {
		s.Reply(info, "🔰 *FAILED TO BLOCK*\n*+"+targetNumber+"*\n*ERROR: invalid JID*")
		return
	}
	if _, err := cli.UpdateBlocklist(context.Background(), jid, evt.BlocklistChangeActionBlock); err != nil {
		s.Reply(info, "🔰 *FAILED TO BLOCK*\n*+"+targetNumber+"*\n*ERROR: "+err.Error()+"*")
		return
	}

	s.Reply(info,
		"*🔰 USER BLOCKED 🔰*\n\n"+
			"*NUMBER:* +"+targetNumber+"\n"+
			"*METHOD:* "+strings.ToUpper(methodUsed)+"\n\n"+
			"")
}

// ── registration ──────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "block", Category: "OWNER & SYSTEM", Desc: "Block a user on WhatsApp (reply/mention/number/chat)", OwnerOnly: true, Run: handleBlock})
	Register(Command{Name: "b", OwnerOnly: true, Hidden: true, Run: handleBlock})
}
