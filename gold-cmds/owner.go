package goldcmds

// ============================================================================
// GOLD-MD — .owner Command  (send owner's contact card)
// File: owner.go
// ----------------------------------------------------------------------------
// Sends the owner's WhatsApp number + name as a Contact card (vCard) — same
// as sharing a contact in WhatsApp. Uses the owner name set by .ownername
// and the owner number (permanent paired number).
//
//   .owner  → sends a contact card with owner's name + number
//
// PUBLIC command — anyone can use it to get the owner's contact.
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"go.mau.fi/whatsmeow/types"
)

func handleOwner(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Get the owner display name (from .ownername setting, default "UMAR")
	ownerName := s.GetOwnerNameSetting(defaultOwnerName)

	// Get the owner number — prefer the permanent (paired) number
	ownerNum := s.GetPermanentOwnerNumber()
	if ownerNum == "" {
		// fallback: try the ownernumber setting
		ownerNum = s.GetOwnerNumberSetting("")
	}

	if ownerNum == "" {
		s.Reply(info, "*⚠️ OWNER NUMBER NOT SET*\n\n*TYPE ❯ "+prefix+"OWNERNUMBER TO SET YOUR NUMBER*")
		return
	}

	// Send the contact card (vCard) with owner's name + number
	err := s.SendContact(info, ownerName, ownerNum)
	if err != nil {
		s.Reply(info, "*⚠️ FAILED TO SEND OWNER CONTACT*\n\n*Error ❯ "+err.Error()+"*")
		return
	}
}

func init() {
	Register(Command{
		Name:     "owner",
		Category: "OWNER & SYSTEM",
		Desc:     "Send the owner's contact card (number + name as vCard)",
		Run:      handleOwner,
	})
}
