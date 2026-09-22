package goldcmds

// ============================================================================
// GOLD-MD — amute scheduler hook (main package bridge)
// File: amute_hook.go
// ============================================================================
// amuteLookupClientByDigits ko main package startup par live session
// registry (Manager.List) se connect karta hai — Node ke
// _UmarFindTrackerForBotNumber ka Go equivalent: bot-number digits se
// live whatsmeow client dhoondho (JID ya Owner field, dono check).
//
// Node me scheduler setInterval se hamesha chalta tha; Go me ye hook
// startup par lagta hai aur scheduler ko foran start kar deta hai.
// ============================================================================

import (
	"go.mau.fi/whatsmeow"
)

// AmuteAttachSessionLookup — main package ise startup par call karta hai.
// lookup: bot-number digits → live client (nil jab session band ho).
func AmuteAttachSessionLookup(lookup func(digits string) *whatsmeow.Client) {
	if lookup != nil {
		amuteLookupClientByDigits = lookup
	}
	// scheduler hamesha chalna chahiye — startup par hi start (Node
	// setInterval(30s) jaisa, commands ke bina bhi enabled groups
	// apne time par mute/unmute hote rahenge).
	amuteSchedulerStart()
}
