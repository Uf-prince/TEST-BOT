package goldcmds

// ============================================================================
// GOLD-MD — .report command (report + block as SPAMMING)
// File: report.go
// ============================================================================
// Ported from UMAR-MD plugins/report.js — SAME WORK (0% farak):
//   .report (reply)    → report + block the replied-to user
//   .report @mention   → report + block the mentioned user
//   .report 923xxx     → report + block by number
//   .report (inbox)    → report + block the chat itself
//
// Owner-only. Won't report/block the bot itself or any owner.
// Report = 🚫 reaction signal to the target (whatsmeow has no public
// ReportSpam API — same as Baileys reportSpam optional path), then
// block via UpdateBlocklist (Baileys updateBlockStatus equivalent).
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	evt "go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func handleReport(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleReportAsync(s, info, args, prefix)
}

func handleReportAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// ── OWNER CHECK ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	// ── GET TARGET JID (same 4 methods as .block, LID→PN resolved) ──
	targetJID, targetNumber, methodUsed := resolveBlockTarget(s, info, args)

	if targetJID == "" {
		s.Reply(info,
			"*🔰 REPORT COMMAND INFO 🔰*\n\n"+
				"*4 METHODS TO REPORT:*\n\n"+
				"*1. REPLY:* Reply to user's message and type "+prefix+"report\n"+
				"*2. MENTION:* "+prefix+"report @username\n"+
				"*3. NUMBER:* "+prefix+"report 923xxxxxxxxx\n"+
				"*4. CHAT:* "+prefix+"report (in inbox — reports the chat)")
		return
	}

	// Normalize to @s.whatsapp.net when it has no server part
	if !strings.Contains(targetJID, "@") {
		targetJID = targetNumber + "@s.whatsapp.net"
	}

	// ── CHECK IF TARGET IS BOT ──
	botJIDNum := digitsOnly(strings.SplitN(s.GetJID(), ":", 2)[0])
	if targetNumber == botJIDNum {
		s.Reply(info, "❌ *CANNOT REPORT MYSELF*")
		return
	}

	// ── CHECK IF TARGET IS OWNER ──
	for _, owner := range blockBuildOwnerNumbers(s) {
		if digitsOnly(owner) == targetNumber {
			s.Reply(info, "❌ *CANNOT REPORT OWNER*")
			return
		}
	}

	// ── CHECK IF ALREADY BLOCKED (REAL WhatsApp block list) ──
	alreadyBlocked := blockAlreadyBlocked(s, targetJID, targetNumber)

	// ── 🔥 REPORT + BLOCK ──
	var reportSuccess bool
	var blockSuccess bool
	var errors []string

	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "❌ *REPORT COMMAND FAILED*\nPlease contact *GOLD* to fix this issue.")
		return
	}

	// Step 1: Report as SPAMMING — 🚫 signal to the target
	// (Node.js: sendMessage(targetJid, {text:'🚫'}) — same message here)
	targetJIDParsed, err := types.ParseJID(targetJID)
	if err != nil {
		s.Reply(info, "❌ *REPORT COMMAND FAILED*\nPlease contact *GOLD* to fix this issue.")
		return
	}
	reportMsg := &waProto.Message{
		Conversation: proto.String("🚫"),
	}
	if _, err := cli.SendMessage(context.Background(), targetJIDParsed, reportMsg); err != nil {
		errors = append(errors, "Report failed: "+err.Error())
	} else {
		reportSuccess = true
	}

	// Step 2: Block the user
	if !alreadyBlocked {
		if _, err := cli.UpdateBlocklist(context.Background(), targetJIDParsed, evt.BlocklistChangeActionBlock); err != nil {
			errors = append(errors, "Block failed: "+err.Error())
		} else {
			blockSuccess = true
		}
	} else {
		blockSuccess = true // Already blocked
	}

	// ── RESULT MESSAGE (same as Node.js resultMsg) ──
	var result strings.Builder
	result.WriteString("*🔰 REPORT + BLOCK COMPLETE 🔰*\n\n")
	result.WriteString("*NUMBER:* +" + targetNumber + "\n")
	result.WriteString("*METHOD:* " + strings.ToUpper(methodUsed) + "\n\n")

	if reportSuccess {
		result.WriteString("✅ *REPORTED AS SPAMMING*\n")
	} else {
		result.WriteString("❌ *REPORT FAILED*\n")
	}

	if blockSuccess {
		if alreadyBlocked {
			result.WriteString("✅ *USER ALREADY BLOCKED*\n")
		} else {
			// Node: `*USER ${alreadyBlocked ? 'ALREADY' : ''} BLOCKED*` —
			// empty string leaves a double space. Kept 0% farak.
			result.WriteString("✅ *USER  BLOCKED*\n")
		}
	} else {
		result.WriteString("❌ *BLOCK FAILED*\n")
	}

	if len(errors) > 0 {
		result.WriteString("\n*ERRORS:*\n" + strings.Join(errors, "\n"))
	}

	s.Reply(info, result.String())
}

// ── registration ────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "report", Category: "OWNER & SYSTEM", Desc: "Report + block a user as spamming (reply/mention/number/chat)", OwnerOnly: true, Run: handleReport})
	Register(Command{Name: "rp", OwnerOnly: true, Hidden: true, Run: handleReport})
}
