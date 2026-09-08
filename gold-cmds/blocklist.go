package goldcmds

// ============================================================================
// GOLD-MD — .blist command (WhatsApp REAL block list)
// File: blocklist.go
// ============================================================================
// Ported from UMAR-MD plugins/blocklist.js — SAME WORK (0% farak):
// Shows the bot's own WhatsApp account REAL block list — the same list
// visible in the phone's WhatsApp app (Settings > Privacy > Blocked
// contacts). Pulled from WhatsApp's servers via whatsmeow GetBlocklist()
// (Baileys sock.fetchBlocklist() equivalent).
//
// DISAMBIGUATION: this is NOT "botblocklist" (bot-internal command-ban
// list). This is the REAL WhatsApp-level block list of the owner's own
// account.
//
// OWNER-ONLY. Privacy mask: last 3 digits → "xxx".
// LID → PN best-effort resolution (lid-pn cache + Store.LIDs).
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// blistResolveTimeout bounds each LID→PN resolve attempt.
const blistResolveTimeout = 10 * time.Second

// blistMaskNumber masks the last 3 digits of a number with "xxx"
// (privacy — same as Node.js _UmarMaskBlockedNumber).
func blistMaskNumber(jid string) string {
	num := strings.SplitN(jid, "@", 2)[0]
	num = strings.SplitN(num, ":", 2)[0]
	if len(num) <= 3 {
		return strings.Repeat("x", len(num))
	}
	return num[:len(num)-3] + "xxx"
}

// blistResolveToPN resolves a LID JID to a phone-number JID, best-effort.
// Uses the live client's Store.LIDs cache (persisted in sqlite) — mirrors
// the Node.js umar.resolveToPN with the lid-pn-map.json cache.
// Returns the resolved JID string, or the input unchanged on failure.
func blistResolveToPN(s SessionBridge, jid string) string {
	// Fast path: already a phone-number JID
	if strings.HasSuffix(jid, "@s.whatsapp.net") {
		return jid
	}
	if !strings.HasSuffix(jid, "@lid") {
		return jid
	}
	cli := s.GetClient()
	if cli == nil || cli.Store == nil || cli.Store.LIDs == nil {
		return jid
	}
	lidJID, err := types.ParseJID(jid)
	if err != nil {
		return jid
	}
	ctx, cancel := context.WithTimeout(context.Background(), blistResolveTimeout)
	defer cancel()
	pn, err := cli.Store.LIDs.GetPNForLID(ctx, lidJID)
	if err != nil || pn.IsEmpty() {
		return jid
	}
	out := pn.String()
	if out != "" && !pn.IsEmpty() {
		return out
	}
	return jid
}

// blistFormatLine formats one blocked entry (masked).
func blistFormatLine(jid string, index int) string {
	numPart := strings.SplitN(jid, "@", 2)[0]
	numPart = strings.SplitN(numPart, ":", 2)[0]
	idx := padLeftTwo(index + 1)
	return "*" + idx + ".* " + blistMaskNumber(numPart)
}

// padLeftTwo pads an index to 2 digits ("01.", "02.", ...).
func padLeftTwo(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func handleBlocklist(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleBlocklistAsync(s, info, args, prefix)
}

func handleBlocklistAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// ── OWNER CHECK (defense-in-depth, same as Node.js) ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "*BLOCKLIST FEATURE IS NOT SUPPORTED*")
		return
	}

	bl, err := cli.GetBlocklist(context.Background())
	if err != nil {
		s.Reply(info, "*Try Again 🥺*")
		return
	}

	if bl == nil || len(bl.JIDs) == 0 {
		s.Reply(info, "*🔰 WHATSAPP BLOCK LIST 🔰*\n\n*No blocked contacts found ✅*")
		return
	}

	// WhatsApp se jo bhi jid mile (resolve ho ya na ho) — sab copy
	// karke, sirf mask laga ke, seedha list mein daal do.
	var lines []string
	for i, j := range bl.JIDs {
		resolved := blistResolveToPN(s, j.String())
		lines = append(lines, blistFormatLine(resolved, i))
	}

	text := "*🔰 WHATSAPP BLOCK LIST 🔰*\n\n" +
		"*🔰 TOTAL BLOCKED :➭ " + itoa(len(bl.JIDs)) + "*\n\n" +
		strings.Join(lines, "\n")

	s.Reply(info, text)
}

// ── registration ──────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "blist", Category: "ANTI & PROTECTION", Desc: "Show the REAL WhatsApp block list (owner only)", OwnerOnly: true, Run: handleBlocklist})
	Register(Command{Name: "blocklist", OwnerOnly: true, Hidden: true, Run: handleBlocklist})
	Register(Command{Name: "bllocklist", OwnerOnly: true, Hidden: true, Run: handleBlocklist})
	Register(Command{Name: "blklist", OwnerOnly: true, Hidden: true, Run: handleBlocklist})
}
