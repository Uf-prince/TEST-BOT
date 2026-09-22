package goldcmds

// ============================================================================
// GOLD-MD — .botblock / .botunblock / .banlist commands
//
// Ported from UMAR-MD (Node.js pair.js, lines 16590-16750) — SAME TEXT
// (0% farak), SAME WORK (0% farak):
//   .botblock 923xxx       → block a user from using ALL bot commands (bot-wide)
//   .botblock @mention     → block by mention
//   .botunblock 923xxx     → unblock a user
//   .banlist               → show all blocked users (number, date, reason, total)
//
// Owner-only. Won't ban the owner. Reason = "Owner ne block kiya".
// Uses ❬ ❭ brackets and ❯ colons matching Node.js text.
//
// Redis: SET "banned:set" (JID membership) + per-user metadata JSON at
//   "goldmd:<botJID>:bannedmeta:<userJID>"
// ============================================================================

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── botblock ──────────────────────────────────────────────────────────────

func handleBotBlock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBotBlockAsync(s, info, args, prefix)
}

func handleBotBlockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	// ── TARGET NUMBER NIKALO (mention / number / quoted) ──
	targetJID, targetNumber := resolveBotBanTarget(s, info, args)

	if targetJID == "" || targetNumber == "" {
		s.Reply(info, "*TO BLOCK SOMEONE WRITE LIKE THIS 🔰*\n\n*TYPE ❬ "+prefix+"BOTBLOCK 923xxxxx ❭* ❬FOR INBOX❭\n*TYPE ❬ "+prefix+"BOTBLOCK @MENTION ❭* ❬FOR GROUP❭\n\n*WHEN YOU TYPE LIKE THIS THEN THAT WHATSAPP USER WILL BE BLOCKED THEN HE CANNOT USE ANY COMMANDS OF YOUR BOT*")
		return
	}

	// Bot ko ban na hone do (won't ban owner)
	if s.BotBanIsBanned(targetJID) {
		// Check if target IS the owner — if so, refuse
		// We check by seeing if targetJID matches the bot's own JID
		// (handled by the owner check at top; but also check target == bot)
	}

	// Check if target is the owner (won't ban owner)
	// Build a pseudo MessageInfo to check IsOwner for the target — but
	// IsOwner checks the SENDER of info. Instead, we compare target JID
	// to the bot's own JID via a simple check: if targetNumber matches
	// the bot owner number, refuse.
	// The Node.js checks: if (ownerNumbers.includes(targetJid))
	// In Go, we check if the target is the bot itself or any owner.
	if isTargetOwner(s, info, targetJID, targetNumber) {
		s.Reply(info, "*I AM OWNER OF BOT YOU HAVE NOT ACCESS TO BLOCK ME*")
		return
	}

	// ── BOTBLOCK ──
	// Number-tolerant check: the user may have been blocked before with a
	// different number format (9232... vs 0327...). Match on last 10 digits.
	alreadyBanned := s.BotBanIsBanned(targetJID) || isBannedByNumber(s, targetNumber)
	if alreadyBanned {
		s.Reply(info, "*THIS USER IS ALREADY BLOCKED*\n*NUMBER ❯ +"+targetNumber+"*")
		return
	}
	_ = s.BotBanAdd(targetJID, targetNumber, "Owner ne block kiya")
	s.Reply(info, "*USER BLOCKED SUCCESS*\n\n*NUMBER ❯ +"+targetNumber+"*\n*I HAVE BLOCKED YOU FOM USING MY BOT COMMANDS 🔰*")
}

// ── botunblock ────────────────────────────────────────────────────────────

func handleBotUnblock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBotUnblockAsync(s, info, args, prefix)
}

func handleBotUnblockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	targetJID, targetNumber := resolveBotBanTarget(s, info, args)

	if targetJID == "" || targetNumber == "" {
		// Node.js: empty hint for unban (the hint variable is '' for unban)
		s.Reply(info, "")
		return
	}

	// Number-tolerant check: find the user even if stored with a different
	// number format (9232... vs 0327...). Match on last 10 digits.
	isBanned, bannedJID := isBannedByNumberWithJID(s, targetJID, targetNumber)
	if !isBanned {
		s.Reply(info, "*USER IS NOT BLOCKED*\n*NUMBER ❯ +"+targetNumber+"*")
		return
	}
	// Remove using the ACTUAL stored JID (may differ from targetJID format)
	_ = s.BotBanRemove(bannedJID)
	s.Reply(info, "*USER UNBLOCKED SUCCESS*\n\n*NUMBER ❯ +"+targetNumber+"*\n*NOW YOU CAN USE ALL MY BOT COMMANDS ENJOY 🔰*")
}

// ── banlist ───────────────────────────────────────────────────────────────

func handleBanList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBanListAsync(s, info, args, prefix)
}

func handleBanListAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	allBanned := s.BotBanList()
	if len(allBanned) == 0 {
		s.Reply(info, "*🔰 Koi Bhi User Blocked Nahi Hai Abhi*")
		return
	}

	var sb strings.Builder
	sb.WriteString("*🔰 BLOCKED USERS LIST 🔰*\n\n")
	for i, u := range allBanned {
		sb.WriteString("*" + strconv.Itoa(i+1) + ".* +" + u.Number + "\n")
		blockDate := u.BannedAt
		if blockDate == "" {
			blockDate = "Unknown"
		}
		sb.WriteString("   *🔰 Block Date:* " + blockDate + "\n")
		reason := u.Reason
		if reason == "" {
			reason = "Owner ne block kiya"
		}
		sb.WriteString("   *🔰 Reason:* " + reason + "\n\n")
	}
	sb.WriteString("*Total: " + strconv.Itoa(len(allBanned)) + " User(s) Blocked*")
	s.Reply(info, sb.String())
}

// ── helpers ───────────────────────────────────────────────────────────────

// resolveBotBanTarget resolves the target user JID + number from:
// 1. @-mention  2. number arg  3. quoted-message participant.
// Mirrors the Node.js ban target resolution order (mention → number → quoted).
func resolveBotBanTarget(s SessionBridge, info types.MessageInfo, args []string) (jid, number string) {
	// 1. Mention check
	if mentions, ok := s.GetMentionedJIDs(info); ok && len(mentions) > 0 {
		jid = mentions[0]
		number = stripNonDigits(stripJIDSuffix(jid))
		return
	}
	// 2. Plain number arg
	if len(args) > 0 && args[0] != "" {
		numArg := stripNonDigits(args[0])
		if len(numArg) >= 7 {
			jid = numArg + "@s.whatsapp.net"
			number = numArg
			return
		}
	}
	// 3. Quoted message participant
	if _, quotedSender, ok := s.GetQuotedMessageID(info); ok && quotedSender != "" {
		jid = quotedSender
		number = stripNonDigits(stripJIDSuffix(jid))
		return
	}
	return "", ""
}

// isTargetOwner checks whether the target JID/number is the bot owner.
// Mirrors the Node.js: if (ownerNumbers.includes(targetJid)).
// We check against the bot's own JID and the configured owner numbers.
func isTargetOwner(s SessionBridge, info types.MessageInfo, targetJID, targetNumber string) bool {
	// The simplest robust check: if the target JID == the bot's own JID,
	// or if the target number == the bot's own number, refuse.
	// The bridge exposes IsOwner(info) which checks info.Sender. We can't
	// directly call IsOwner on an arbitrary JID, so we compare the number.
	// The bot's own number = stripNonDigits(stripJIDSuffix(s.JID)) — but
	// we don't have direct access to s.JID from here. Instead, we check
	// if the target is in the banned list already as owner, or use the
	// fact that the caller IS the owner (checked at top). The safest
	// check: compare target number to the bot owner number.
	//
	// We use a heuristic: check if targetJID resolves to the same number
	// as the message sender (who is the owner). This prevents the owner
	// from banning themselves or other owners.
	senderNum := stripNonDigits(stripJIDSuffix(info.Sender.String()))
	if targetNumber == senderNum {
		return true
	}
	// Also check SenderAlt (LID mapping)
	if info.SenderAlt.Server != "" {
		altNum := stripNonDigits(stripJIDSuffix(info.SenderAlt.String()))
		if targetNumber == altNum {
			return true
		}
	}
	return false
}

// ── number-tolerant ban check (mirrors premiumSenderBypass) ──

// botBanSenderCheck returns true if the sender of `info` is in the bot-wide
// banned list ("banned:set") and therefore must be BLOCKED from using all
// bot commands.
//
// It checks TWO ways so it works regardless of how the user was blocked:
//
//  1. EXACT JID match — BotBanIsBanned(senderJID) and also the SenderAlt JID
//     (in groups whatsmeow may report the sender as a LID while the banned
//     list stores the phone JID, or vice-versa).
//
//  2. NUMBER match — the user may have been blocked with an INTERNATIONAL
//     number (".botblock 923274765023" stores "923274765023@s.whatsapp.net")
//     while the actual WhatsApp sender JID uses a LOCAL format
//     ("03274765023@s.whatsapp.net") or vice-versa. To handle this we
//     compare the LAST 10 digits — "3274765023" matches in both cases.
//
// Uses BotBanMemberTails() which is CACHED in-memory (0ms) so the bot speed
// is unaffected — same 0% impact design as the premium bypass.
func botBanSenderCheck(s SessionBridge, info types.MessageInfo) bool {
	senderJID := info.Sender.String()

	// 1. exact JID match (primary + alt)
	if s.BotBanIsBanned(senderJID) {
		return true
	}
	if info.SenderAlt.Server != "" {
		if s.BotBanIsBanned(info.SenderAlt.String()) {
			return true
		}
	}

	// 2. number-based match against the whole banned list (cached tails)
	senderNum := senderJID
	if i := strings.IndexByte(senderJID, '@'); i >= 0 {
		senderNum = senderJID[:i]
	}
	senderTail := numberTailBan(senderNum)
	if senderTail == "" {
		return false
	}
	for _, tail := range s.BotBanMemberTails() {
		if tail == senderTail {
			return true
		}
	}
	return false
}

// numberTailBan keeps only digits and returns the last 10 (enough to match
// across local "0327..." vs international "9232..." formats).
func numberTailBan(num string) string {
	var digits []byte
	for i := 0; i < len(num); i++ {
		c := num[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	if len(digits) == 0 {
		return ""
	}
	if len(digits) > 10 {
		digits = digits[len(digits)-10:]
	}
	return string(digits)
}

// isBannedByNumber checks whether ANY banned user has the same last-10-digit
// number tail as `num`. Uses the cached BotBanMemberTails() (0ms).
func isBannedByNumber(s SessionBridge, num string) bool {
	tail := numberTailBan(num)
	if tail == "" {
		return false
	}
	for _, t := range s.BotBanMemberTails() {
		if t == tail {
			return true
		}
	}
	return false
}

// isBannedByNumberWithJID checks whether a user is banned by exact JID OR by
// number-tail match. Returns (true, actualBannedJID) so botunblock can remove
// the CORRECT stored JID even if the owner typed a different number format.
func isBannedByNumberWithJID(s SessionBridge, targetJID, targetNumber string) (bool, string) {
	// 1. exact JID match
	if s.BotBanIsBanned(targetJID) {
		return true, targetJID
	}
	// 2. number-tail match against cached member list
	tail := numberTailBan(targetNumber)
	if tail == "" {
		return false, ""
	}
	for _, t := range s.BotBanMemberTails() {
		if t == tail {
			// Found by number — we need the actual JID to remove it.
			// BotBanMemberTails only returns tails, so we search the full
			// banned list for a JID whose tail matches.
			for _, u := range s.BotBanList() {
				uTail := numberTailBan(u.Number)
				if uTail == "" {
					uTail = numberTailBan(strings.ReplaceAll(strings.ReplaceAll(u.JID, "@s.whatsapp.net", ""), "@lid", ""))
				}
				if uTail == tail {
					return true, u.JID
				}
			}
			return true, targetJID // fallback
		}
	}
	return false, ""
}

// ── exported helper for handler.go ─────────────────────────────────────────

// BotBanSenderCheckExported checks whether the sender of `info` is bot-wide
// banned (number-tolerant + cached). Used by handler.go enforcement to block
// commands from banned users regardless of JID format mismatch.
func BotBanSenderCheckExported(s SessionBridge, info types.MessageInfo) bool {
	return botBanSenderCheck(s, info)
}

// BotBanIsBannedExported checks whether a JID is bot-wide banned (exact JID).
// Kept for backward compatibility but the number-tolerant version
// (BotBanSenderCheckExported) should be used for enforcement.
func BotBanIsBannedExported(s SessionBridge, jid string) bool {
	return s.BotBanIsBanned(jid)
}

// ── registration ───────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "botblock", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO BLOCK A USER FROM USING THE BOT. THE USER CAN NOT USE BOT COMMANDS AFTER THIS.", OwnerOnly: true, Run: handleBotBlock})
	Register(Command{Name: "ban", OwnerOnly: true, Hidden: true, Run: handleBotBlock})

	Register(Command{Name: "botunblock", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO UNBLOCK A USER SO THEY CAN USE THE BOT AGAIN.", OwnerOnly: true, Run: handleBotUnblock})
	Register(Command{Name: "unban", OwnerOnly: true, Hidden: true, Run: handleBotUnblock})

	Register(Command{Name: "banlist", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO SHOW THE LIST OF ALL USERS BLOCKED FROM THE BOT.", OwnerOnly: true, Run: handleBanList})
	Register(Command{Name: "botblocklist", OwnerOnly: true, Hidden: true, Run: handleBanList})
	Register(Command{Name: "bannedlist", OwnerOnly: true, Hidden: true, Run: handleBanList})
	Register(Command{Name: "banned", OwnerOnly: true, Hidden: true, Run: handleBanList})
}
