package goldcmds

// ============================================================================
// GOLD-MD — Edit Message Command
// File: edit.go
// ============================================================================
// COMMAND: .edit <new text>    (owner-only, reply to the bot's own message)
//   Edits the quoted message (which must be one of the bot's own text
//   messages) to the new text, then deletes the trigger command message.
//
// Source: UMAR-MD edit.js  (Node.js / Baileys)
// Converted to Go / whatsmeow for GOLD-MD.
//
// whatsmeow: Client.BuildEdit(chat, messageID, newMessage) + SendMessage,
//   then BuildRevoke to delete the trigger.
// ============================================================================

import (
	"context"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const editNoQuoteText = "*🔰 EDIT MESSAGE INFO 🔰*\n\n" +
	"*FIRST MENTION THE MESSAGE WHICH MESSAGE DO YOU WANT TO EDIT AND ADD NEW MESSAGE*\n\n" +
	"*EDIT ❰ NEW MSG HERE ❱*"

const editNotOwnMsgText = "*🔰 EDIT INFO 🔰*\n\n" +
	"*CAN NOT EDIT OTHER WHATSAPP USERS MESSAGES*\n" +
	"*ONLY EDIT OWN MESSAGES*"

const editNoNewText = "*🔰 MESSAGE EDIT ERROR 🔰*\n\n*TYPE THE NEW MESSAGE NOW SAME*" +
	"*EDIT EDIT MY MESSAGE*"

const editFailedText = "*TRY AGAIN LATER TO EDIT MESSAGES*"

func handleEdit(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleEditAsync(s, info, args, prefix)
}

func handleEditAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		return
	}

	newText := strings.TrimSpace(strings.Join(args, " "))
	if newText == "" {
		s.Reply(info, editNoQuoteText)
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "❌ *Client not connected.*")
		return
	}

	// The quoted message ID lives in the cached message proto's ContextInfo.
	quotedID, quotedSender := extractQuotedMessageID(s, info)
	if quotedID == "" {
		s.Reply(info, editNoQuoteText)
		return
	}

	// Ensure the quoted message was sent by the bot (self-edit only).
	botJID := s.GetJID()
	if !isSelfMessage(quotedSender, botJID) {
		s.Reply(info, editNotOwnMsgText)
		return
	}

	// Perform the native WhatsApp edit on the quoted message.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	edited := client.BuildEdit(info.Chat, types.MessageID(quotedID), &waProto.Message{
		Conversation: proto.String(newText),
	})
	if edited == nil {
		s.Reply(info, editFailedText)
		return
	}
	if _, err := client.SendMessage(ctx, info.Chat, edited); err != nil {
		s.Reply(info, editFailedText+"\n*"+strings.ToUpper(err.Error())+"*")
		return
	}

	// Delete the trigger command message.
	_ = s.DeleteMessage(info, info.ID)
}

// extractQuotedMessageID pulls the quoted (replied-to) message ID and sender
// from the cached incoming message proto's ExtendedTextMessage.ContextInfo.
// Uses an optional interface method implemented by the bridge in the main
// package (GetQuotedMessageID) so the gold-cmds package stays decoupled.
func extractQuotedMessageID(s SessionBridge, info types.MessageInfo) (id string, sender string) {
	if q, ok := s.(interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}); ok {
		qid, qsender, ok := q.GetQuotedMessageID(info)
		if ok {
			return qid, qsender
		}
	}
	return "", ""
}

// isSelfMessage reports whether the quoted sender JID matches the bot JID.
func isSelfMessage(quotedSender, botJID string) bool {
	if quotedSender == "" || botJID == "" {
		return false
	}
	return stripResource(quotedSender) == stripResource(botJID)
}

func stripResource(jid string) string {
	if i := strings.Index(jid, "@"); i > 0 {
		return jid[:i]
	}
	return jid
}

func init() {
	Register(Command{Name: "edit", Category: "OWNER & SYSTEM", Desc: "Edit the bot's own quoted message", OwnerOnly: true, Run: handleEdit})
}
