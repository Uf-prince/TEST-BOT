package goldcmds

// ============================================================================
// GOLD-MD — Clear Chat Command
// File: clear.go
// ============================================================================
// COMMAND: .clear    (owner-only)
//   Clears the current chat (deletes all messages for the bot in this chat).
//   Replies "GROUP CHAT CLEARED" or "INBOX CHAT CLEARED" depending on chat type.
//
// Source: UMAR-MD clear.js  (Node.js / Baileys — chatModify {delete:true})
// Converted to Go / whatsmeow for GOLD-MD.
//
// whatsmeow: appstate.BuildDeleteChat(chatJID, ts, lastKey, deleteMedia) +
//   Client.SendAppState(ctx, patch). This removes the chat from the bot's
//   chat list (clears it), matching the UMAR "clear chat" behavior.
//
// Aliases (Hidden): clearchat, purge
// ============================================================================

import (
	"context"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

func handleClear(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleClearAsync(s, info, args, prefix)
}

func handleClearAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 *Client not connected.*")
		return
	}

	chat := info.Chat
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build a delete-chat app state patch (clears the chat for the bot).
	patch := appstate.BuildDeleteChat(chat, time.Now(), nil, true)
	if err := client.SendAppState(ctx, patch); err != nil {
		s.Reply(info, "🔰 *CLEAR Command Error*\nFailed to clear chat: "+err.Error())
		return
	}

	if strings.HasSuffix(chat.String(), "@g.us") {
		s.Reply(info, "🔰 *GROUP CHAT CLEARED*")
	} else {
		s.Reply(info, "🔰 *INBOX CHAT CLEARED*")
	}
}

func init() {
	Register(Command{Name: "clear", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO CLEAR ALL MESSAGES OF THE CHAT. USE IT IN THE CHAT YOU WANT TO CLEAR.", OwnerOnly: true, Run: handleClear})
	Register(Command{Name: "clearchat", OwnerOnly: true, Hidden: true, Run: handleClear})
	Register(Command{Name: "purge", OwnerOnly: true, Hidden: true, Run: handleClear})
}
