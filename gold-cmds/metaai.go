package goldcmds

// ============================================================================
// GOLD-MD — Meta AI Group Commands
// File: metaai.go
// ============================================================================
// Adds or removes Meta AI from the current group chat.
//
// Commands (no admin check — direct action, per owner request):
//   .addmeta   — adds Meta AI to the group
//   .delmeta   — removes Meta AI from the group
//
// Meta AI has two JID representations:
//   1. types.MetaAIJID      = 13135550002@s.whatsapp.net  (old PN-style)
//   2. types.NewMetaAIJID   = 867051314767696@bot          (new BotServer)
//
// The @bot JID is the correct way to add bots — no privacy token needed.
// This command tries @bot → PN → LID (extracted from response) in order.
// ============================================================================

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// metaAIBotJID returns the NEW Meta AI JID on the BotServer.
func metaAIBotJID() types.JID {
	return types.NewMetaAIJID
}

// metaAIPNJID returns the OLD Meta AI JID on the DefaultUserServer.
func metaAIPNJID() types.JID {
	return types.MetaAIJID
}

// checkResult inspects the returned participant slice and returns
// (true, "") if the action was accepted, or (false, "") if rejected.
// It also returns the LID of the participant if present in the response.
func checkResult(resp []types.GroupParticipant) (bool, types.JID) {
	var respLID types.JID
	for _, p := range resp {
		if !p.JID.IsEmpty() && p.JID.Server == types.HiddenUserServer {
			respLID = p.JID
		}
		if p.Error == 0 {
			return true, respLID
		}
		return false, respLID
	}
	return false, respLID
}

// ---------------------------------------------------------------------------
// addmeta  (add Meta AI to group)
// ---------------------------------------------------------------------------

func handleAddMeta(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAddMetaAsync(s, info, args, prefix)
}

func handleAddMetaAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}

	ctx := context.Background()
	groupJID := info.Chat
	botJID := metaAIBotJID()
	pnJID := metaAIPNJID()

	// Attempt 1: Add with @bot JID
	resp, err := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{botJID}, whatsmeow.ParticipantChangeAdd)
	if err == nil {
		ok, respLID := checkResult(resp)
		if ok {
			s.Reply(info, "🔰 *META AI ADDED*\n\n🔰 *Action Completed!*\n_Meta AI added successfully!_")
			return
		}
		// Attempt 1b: Add with LID from response
		if !respLID.IsEmpty() {
			if client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, respLID, pnJID)
			}
			resp1b, err1b := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{respLID}, whatsmeow.ParticipantChangeAdd)
			if err1b == nil {
				ok1b, _ := checkResult(resp1b)
				if ok1b {
					s.Reply(info, "🔰 *META AI ADDED*\n\n🔰 *Action Completed!*\n_Meta AI added successfully!_")
					return
				}
			}
		}
	}

	time.Sleep(2 * time.Second)

	// Attempt 2: Add with PN JID
	resp2, err2 := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{pnJID}, whatsmeow.ParticipantChangeAdd)
	if err2 != nil {
		s.Reply(info, "🔰 *META AI ADD FAILED*\n\n_Tip: Make sure the bot is admin of this group._")
		return
	}

	ok2, respLID2 := checkResult(resp2)
	if ok2 {
		s.Reply(info, "🔰 *META AI ADDED*\n\n🔰 *Action Completed!*\n_Meta AI added successfully!_")
		return
	}

	// Attempt 3: Add with LID from response
	if !respLID2.IsEmpty() {
		if client.Store != nil && client.Store.LIDs != nil {
			_ = client.Store.LIDs.PutLIDMapping(ctx, respLID2, pnJID)
		}
		time.Sleep(2 * time.Second)
		resp3, err3 := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{respLID2}, whatsmeow.ParticipantChangeAdd)
		if err3 == nil {
			ok3, _ := checkResult(resp3)
			if ok3 {
				s.Reply(info, "🔰 *META AI ADDED*\n\n🔰 *Action Completed!*\n_Meta AI added successfully!_")
				return
			}
		}
	}

	s.Reply(info, "🔰 *META AI ADD FAILED*\n\n_Tip: Make sure the bot is admin of this group._")
}

// ---------------------------------------------------------------------------
// delmeta  (remove Meta AI from group)
// ---------------------------------------------------------------------------

func handleDelMeta(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDelMetaAsync(s, info, args, prefix)
}

func handleDelMetaAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}

	ctx := context.Background()
	groupJID := info.Chat
	botJID := metaAIBotJID()
	pnJID := metaAIPNJID()

	// Attempt 1: Remove with @bot JID
	resp, err := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{botJID}, whatsmeow.ParticipantChangeRemove)
	if err == nil {
		ok, respLID := checkResult(resp)
		if ok {
			s.Reply(info, "🔰 *META AI REMOVED*\n\n🔰 *Action Completed!*\n_Meta AI removed successfully!_")
			return
		}
		if !respLID.IsEmpty() && client.Store != nil && client.Store.LIDs != nil {
			_ = client.Store.LIDs.PutLIDMapping(ctx, respLID, pnJID)
		}
	}

	time.Sleep(2 * time.Second)

	// Attempt 2: Remove with PN JID
	resp2, err2 := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{pnJID}, whatsmeow.ParticipantChangeRemove)
	if err2 == nil {
		ok2, respLID2 := checkResult(resp2)
		if ok2 {
			s.Reply(info, "🔰 *META AI REMOVED*\n\n🔰 *Action Completed!*\n_Meta AI removed successfully!_")
			return
		}
		if !respLID2.IsEmpty() && client.Store != nil && client.Store.LIDs != nil {
			_ = client.Store.LIDs.PutLIDMapping(ctx, respLID2, pnJID)
		}

		// Attempt 3: Remove with LID
		if !respLID2.IsEmpty() {
			time.Sleep(2 * time.Second)
			resp3, err3 := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{respLID2}, whatsmeow.ParticipantChangeRemove)
			if err3 == nil {
				ok3, _ := checkResult(resp3)
				if ok3 {
					s.Reply(info, "🔰 *META AI REMOVED*\n\n🔰 *Action Completed!*\n_Meta AI removed successfully!_")
					return
				}
			}
		}
	}

	// Attempt 4: Remove with stored LID
	if client.Store != nil && client.Store.LIDs != nil {
		lid, lidErr := client.Store.LIDs.GetLIDForPN(ctx, pnJID)
		if lidErr == nil && !lid.IsEmpty() {
			time.Sleep(2 * time.Second)
			resp4, err4 := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{lid}, whatsmeow.ParticipantChangeRemove)
			if err4 == nil {
				ok4, _ := checkResult(resp4)
				if ok4 {
					s.Reply(info, "🔰 *META AI REMOVED*\n\n🔰 *Action Completed!*\n_Meta AI removed successfully!_")
					return
				}
			}
		}
	}

	s.Reply(info, "🔰 *META AI REMOVE FAILED*\n\n_Tip: Make sure the bot is admin of this group._")
}

// ---------------------------------------------------------------------------
// command registration
// ---------------------------------------------------------------------------

func init() {
	Register(Command{
		Name:     "addmeta",
		Category: "GROUP MANAGEMENT",
		Desc:     "THIS COMMAND IS USED TO ADD META AI INTO THE GROUP CHAT.",
		Run:      handleAddMeta,
	})
	Register(Command{
		Name:     "delmeta",
		Category: "GROUP MANAGEMENT",
		Desc:     "THIS COMMAND IS USED TO REMOVE META AI FROM THE GROUP CHAT.",
		Run:      handleDelMeta,
	})
}
