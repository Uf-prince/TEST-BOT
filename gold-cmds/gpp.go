package goldcmds

// ============================================================================
// GOLD-MD — Group Profile Picture Command
// File: gpp.go
// ============================================================================
// Sets the group profile picture (icon) from a replied-to image.
// Converted from UMAR-MD gpp.js (Node.js / Baileys) to Go / whatsmeow.
//
// Usage:
//   .gpp  — reply to an image with this command to set it as the group DP
//
// whatsmeow API:
//   Client.SetGroupPhoto(ctx, jid, avatar []byte) (string, error)
// ============================================================================

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// gppHelpText is shown when the command is used without a quoted image.
const gppHelpText = "🔰 *GROUP PROFILE PICTURE*\n\n*HOW TO USE:*\n*1. SEND ANY IMAGE*\n*2. REPLY WITH .GPP*\n\n*EXAMPLE:*\n*[SEND IMAGE] → REPLY WITH .GPP*"

func handleGpp(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGppAsync(s, info, args, prefix)
}

func handleGppAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	// must be a group
	if !info.IsGroup {
		s.Reply(info, "🔰 *THIS COMMAND ONLY WORKS IN GROUPS*")
		return
	}

	// check for a quoted message (GetQuotedMessageID is an optional interface
	// method implemented by the bridge)
	type quotedGetter interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}
	hasQuoted := false
	if q, ok := s.(quotedGetter); ok {
		if _, _, ok2 := q.GetQuotedMessageID(info); ok2 {
			hasQuoted = true
		}
	}
	if !hasQuoted {
		s.Reply(info, gppHelpText)
		return
	}

	// download the quoted media (image)
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, "🔰 *REPLY TO AN IMAGE ONLY*")
		return
	}
	// verify it's an image
	if !strings.HasPrefix(mime, "image/") {
		s.Reply(info, "🔰 *REPLY TO AN IMAGE ONLY*")
		return
	}

	client := s.GetClient()
	if client == nil {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}

	// set the group profile picture
	_, err := client.SetGroupPhoto(context.Background(), info.Chat, data)
	if err != nil {
		s.Reply(info, "🔰 *FAILED*\n*"+err.Error()+"*")
		return
	}
	s.Reply(info, "🔰 *GROUP PROFILE PIC CHANGED SUCCESS*")
}

func init() {
	Register(Command{Name: "gpp", Category: "GROUP MANAGEMENT", Desc: "Set the group profile picture (icon)", OwnerOnly: true, Run: handleGpp})
	// aliases
	Register(Command{Name: "setgpp", OwnerOnly: true, Hidden: true, Run: handleGpp})
	Register(Command{Name: "groupdp", OwnerOnly: true, Hidden: true, Run: handleGpp})
	Register(Command{Name: "setgrouppic", OwnerOnly: true, Hidden: true, Run: handleGpp})
	Register(Command{Name: "grouppic", OwnerOnly: true, Hidden: true, Run: handleGpp})
}
