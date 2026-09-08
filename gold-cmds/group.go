package goldcmds

// ============================================================================
// GOLD-MD — Group Management Commands
// File: group.go
// ============================================================================
// Owner-only group management commands converted from UMAR-MD group.js
// (Node.js / Baileys) to Go / whatsmeow.
//
// Commands (all owner-only, used in a group chat):
//   .mute / .unmute            — close / open group to admin-only messages
//   .kick                      — remove the replied/mentioned member(s)
//   .promote / .demote         — make / remove admin
//   .invitelink / .revokelink  — get / reset group invite link
//   .gname <name>              — set the group name
//   .gdesc <desc>              — set the group description / topic
//   .members                   — list all members with admin flags
//   .groupinfo                 — detailed group info
//   .tagall                    — mention every member
//   .tagadmins                 — mention every admin
//   .tagmembers                — mention every member (alias of tagall)
//   .leave                     — leave the group
//   .count                     — quick member count
//   .online                    — live member / admin count
//   .editgc on|off             — toggle group-info edit lock (on=lock, off=unlock)
//   .lockgc                    — enable join approval required (new members need approval)
//   .unlockgc                  — disable join approval (anyone can join freely)
//   .myrole                    — show the caller's role in this group
//   .adminlist                 — list only the admins
//
// NOTE: All admin checks have been REMOVED. The bot attempts the action
// directly — WhatsApp will reject it if the bot lacks permissions, and the
// error is reported back.
//
// whatsmeow group API:
//   Client.UpdateGroupParticipants(ctx, jid, []JID, ParticipantChange{Add|Remove|Promote|Demote})
//   Client.SetGroupName(ctx, jid, name)
//   Client.SetGroupTopic(ctx, jid, prevID, newID, topic)
//   Client.SetGroupAnnounce(ctx, jid, announce)
//   Client.SetGroupLocked(ctx, jid, locked)
//   Client.SetGroupJoinApprovalMode(ctx, jid, mode bool)
//   Client.GetGroupInviteLink(ctx, jid, reset)
//   Client.GetGroupInfo(ctx, jid)
//   Client.LeaveGroup(ctx, jid)
// ============================================================================

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// requireGroup replies with an error and returns false if the command was not
// issued inside a group chat.
func requireGroup(s SessionBridge, info types.MessageInfo) bool {
	if info.IsGroup {
		return true
	}
	s.Reply(info, "*THIS COMMAND CAN ONLY BE USED IN GROUPS 😊*")
	return false
}

// targetJIDs collects the JIDs that a kick/promote/demote command should act
// on, in priority order:
//  1. @-mentioned JIDs in the command message
//  2. the sender of the replied-to message (if the command is a reply)
//
// Returns nil if no target could be determined.
func targetJIDs(s SessionBridge, info types.MessageInfo) []types.JID {
	if mentioned, ok := s.GetMentionedJIDs(info); ok && len(mentioned) > 0 {
		out := make([]types.JID, 0, len(mentioned))
		for _, m := range mentioned {
			if j, err := types.ParseJID(m); err == nil {
				out = append(out, j)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	// fall back to replied-to message sender
	type quotedGetter interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}
	if q, ok := s.(quotedGetter); ok {
		if _, qsender, ok2 := q.GetQuotedMessageID(info); ok2 && qsender != "" {
			if j, err := types.ParseJID(qsender); err == nil {
				return []types.JID{j}
			}
		}
	}
	return nil
}

// sendMentionText sends a plain text message that pings the supplied JIDs.
func sendMentionText(client *whatsmeow.Client, info types.MessageInfo, text string, mentioned []string) error {
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waProto.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	}
	_, err := client.SendMessage(context.Background(), info.Chat, msg)
	return err
}

// fetchGroupInfo is a small convenience wrapper that fetches GroupInfo and
// replies with the error string on failure.
func fetchGroupInfo(s SessionBridge, info types.MessageInfo) (*types.GroupInfo, bool) {
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return nil, false
	}
	gi, err := client.GetGroupInfo(context.Background(), info.Chat)
	if err != nil {
		s.Reply(info, "❌ Failed to fetch group info: "+err.Error())
		return nil, false
	}
	return gi, true
}

// ---------------------------------------------------------------------------
// mute / announce  (close group → admin-only)
// ---------------------------------------------------------------------------

func handleMute(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleMuteAsync(s, info, args, prefix)
}

func handleMuteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	if err := client.SetGroupAnnounce(context.Background(), info.Chat, true); err != nil {
		s.Reply(info, "❌ *MUTE FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔒 *GROUP CHAT TURNED OFF*\n\n*ONLY ADMINS CAN MESSAGE NOW 😊*")
}

// ---------------------------------------------------------------------------
// unmute  (open group → everyone)
// ---------------------------------------------------------------------------

func handleUnmute(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleUnmuteAsync(s, info, args, prefix)
}

func handleUnmuteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	if err := client.SetGroupAnnounce(context.Background(), info.Chat, false); err != nil {
		s.Reply(info, "❌ *UNMUTE FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔓 *GROUP CHAT TURNED ON*\n\n*ALL MEMBERS CAN MESSAGE AGAIN 😊*")
}

// ---------------------------------------------------------------------------
// kick  (remove member)
// ---------------------------------------------------------------------------

func handleKick(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleKickAsync(s, info, args, prefix)
}

func handleKickAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	targets := targetJIDs(s, info)
	if len(targets) == 0 {
		s.Reply(info, "*REPLY OR TAG THE MEMBER YOU WANT TO KICK ❗*")
		return
	}
	botJID := client.Store.ID
	// don't kick self
	for _, t := range targets {
		if t.String() == botJID.String() {
			s.Reply(info, "*YOU CAN'T KICK ME 😤*")
			return
		}
	}
	_, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		s.Reply(info, "❌ *KICK FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("👢 *MEMBER KICKED*\n\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "@%s ", t.User)
	}
	b.WriteString("*HAS BEEN REMOVED FROM THE GROUP* 🚫")
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		mentioned = append(mentioned, t.String())
	}
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *KICK FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// promote  (make admin)
// ---------------------------------------------------------------------------

func handlePromote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handlePromoteAsync(s, info, args, prefix)
}

func handlePromoteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	targets := targetJIDs(s, info)
	if len(targets) == 0 {
		s.Reply(info, "*REPLY OR TAG THE MEMBER YOU WANT TO PROMOTE ❗*")
		return
	}
	_, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, whatsmeow.ParticipantChangePromote)
	if err != nil {
		s.Reply(info, "❌ *PROMOTE FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("⬆️ *NEW ADMIN* 🔰\n\n")
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		fmt.Fprintf(&b, "@%s ", t.User)
		mentioned = append(mentioned, t.String())
	}
	b.WriteString("*IS NOW AN ADMIN OF THIS GROUP*\n*CONGRATULATIONS! 🎉*")
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *PROMOTE FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// demote  (remove admin)
// ---------------------------------------------------------------------------

func handleDemote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDemoteAsync(s, info, args, prefix)
}

func handleDemoteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	targets := targetJIDs(s, info)
	if len(targets) == 0 {
		s.Reply(info, "*REPLY OR TAG THE MEMBER YOU WANT TO DEMOTE ❗*")
		return
	}
	_, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, whatsmeow.ParticipantChangeDemote)
	if err != nil {
		s.Reply(info, "❌ *DEMOTE FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("⬇️ *ADMIN REMOVED*\n\n")
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		fmt.Fprintf(&b, "@%s ", t.User)
		mentioned = append(mentioned, t.String())
	}
	b.WriteString("*IS NO LONGER AN ADMIN*\n*NOW A REGULAR MEMBER*")
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *DEMOTE FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// invitelink / revokelink / link
// ---------------------------------------------------------------------------

func handleInviteLink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleInviteLinkAsync(s, info, false)
}

func handleRevokeLink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleInviteLinkAsync(s, info, true)
}

func handleInviteLinkAsync(s SessionBridge, info types.MessageInfo, reset bool) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	gi, err := client.GetGroupInfo(context.Background(), info.Chat)
	if err != nil {
		s.Reply(info, "❌ Failed to fetch group info: "+err.Error())
		return
	}
	link, err := client.GetGroupInviteLink(context.Background(), info.Chat, reset)
	if err != nil {
		if reset {
			s.Reply(info, "❌ *LINK RESET FAILED:* "+err.Error())
		} else {
			s.Reply(info, "❌ *COULDN'T GET THE LINK:* "+err.Error())
		}
		return
	}
	if reset {
		s.Reply(info, "🔄 *INVITE LINK RESET*\n\n*NEW LINK:* "+link+"\n\n⚠️ *THE OLD LINK NO LONGER WORKS*")
	} else {
		s.Reply(info, "🔗 *GROUP INVITE LINK*\n\n*GROUP:* "+gi.GroupName.Name+"\n*LINK:* "+link+"\n\n⚠️ *ONLY SHARE THIS LINK WITH PEOPLE YOU TRUST*")
	}
}

// ---------------------------------------------------------------------------
// gname  (set group name)
// ---------------------------------------------------------------------------

func handleSetGName(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetGNameAsync(s, info, args, prefix)
}

func handleSetGNameAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		s.Reply(info, "*TYPE THE NEW NAME\nEXAMPLE: .GNAME MY GROUP*")
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	if err := client.SetGroupName(context.Background(), info.Chat, name); err != nil {
		s.Reply(info, "❌ *COULDN'T CHANGE THE NAME:* "+err.Error())
		return
	}
	s.Reply(info, "📝 *GROUP NAME CHANGED*\n\n*NEW NAME:* "+name)
}

// ---------------------------------------------------------------------------
// gdesc  (set group description / topic)
// ---------------------------------------------------------------------------

func handleSetGDesc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetGDescAsync(s, info, args, prefix)
}

func handleSetGDescAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	topic := strings.TrimSpace(strings.Join(args, " "))
	if topic == "" {
		s.Reply(info, "*TYPE THE NEW DESCRIPTION\nEXAMPLE: .GDESC THIS IS MY GROUP*")
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	gi, err := client.GetGroupInfo(context.Background(), info.Chat)
	if err != nil {
		s.Reply(info, "❌ Failed to fetch group info: "+err.Error())
		return
	}
	prevID := gi.TopicID
	newID := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := client.SetGroupTopic(context.Background(), info.Chat, prevID, newID, topic); err != nil {
		s.Reply(info, "❌ *COULDN'T CHANGE THE DESCRIPTION:* "+err.Error())
		return
	}
	s.Reply(info, "📋 *GROUP DESCRIPTION CHANGED*\n\n*NEW DESCRIPTION:*\n"+topic)
}

// ---------------------------------------------------------------------------
// members  (list all members)
// ---------------------------------------------------------------------------

func handleMembers(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleMembersAsync(s, info, args, prefix)
}

func handleMembersAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "👥 *%s*\n", gi.GroupName.Name)
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n")
	adminCount := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		}
	}
	fmt.Fprintf(&b, "*TOTAL MEMBERS:* %d\n", len(gi.Participants))
	fmt.Fprintf(&b, "*ADMINS:* %d\n", adminCount)
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n\n")
	for i, p := range gi.Participants {
		role := "👤"
		if p.IsSuperAdmin {
			role = "🔰"
		} else if p.IsAdmin {
			role = "⭐"
		}
		fmt.Fprintf(&b, "%d. %s +%s\n", i+1, role, p.JID.User)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// adminlist  (list only admins)
// ---------------------------------------------------------------------------

func handleAdminList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAdminListAsync(s, info, args, prefix)
}

func handleAdminListAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📊 *ADMIN LIST — %s*\n", gi.GroupName.Name)
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n")
	admins := make([]types.GroupParticipant, 0)
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins = append(admins, p)
		}
	}
	fmt.Fprintf(&b, "*TOTAL ADMINS:* %d\n\n", len(admins))
	for i, p := range admins {
		role := "⭐ ADMIN"
		if p.IsSuperAdmin {
			role = "🔰 OWNER"
		}
		fmt.Fprintf(&b, "%d. %s: +%s\n", i+1, role, p.JID.User)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// groupinfo  (detailed info)
// ---------------------------------------------------------------------------

func handleGroupInfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGroupInfoAsync(s, info, args, prefix)
}

func handleGroupInfoAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	adminCount := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		}
	}
	var b strings.Builder
	b.WriteString("ℹ️ *GROUP INFO*\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n")
	b.WriteString("📍 *NAME:* " + gi.GroupName.Name + "\n")
	b.WriteString("🆔 *GROUP ID:* " + info.Chat.String() + "\n")
	fmt.Fprintf(&b, "👥 *MEMBERS:* %d\n", len(gi.Participants))
	fmt.Fprintf(&b, "⭐ *ADMINS:* %d\n", adminCount)
	b.WriteString("📅 *CREATED:* " + gi.GroupCreated.Format("1/2/2006") + "\n")
	desc := gi.GroupTopic.Topic
	if gi.GroupTopic.TopicDeleted || desc == "" {
		desc = "NO DESCRIPTION"
	}
	b.WriteString("📋 *DESCRIPTION:*\n" + desc + "\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━")
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// count  (quick member count)
// ---------------------------------------------------------------------------

func handleCount(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleCountAsync(s, info, args, prefix)
}

func handleCountAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	adminCount := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		}
	}
	memberCount := len(gi.Participants) - adminCount
	var b strings.Builder
	b.WriteString("🔢 *GROUP MEMBER COUNT*\n\n")
	fmt.Fprintf(&b, "*GROUP:* %s\n", gi.GroupName.Name)
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&b, "👥 *TOTAL:* %d\n", len(gi.Participants))
	fmt.Fprintf(&b, "🔰 *ADMINS:* %d\n", adminCount)
	fmt.Fprintf(&b, "👤 *MEMBERS:* %d\n", memberCount)
	b.WriteString("━━━━━━━━━━━━━━━━━━━")
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// online  (live member / admin count)
// ---------------------------------------------------------------------------

func handleOnline(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleOnlineAsync(s, info, args, prefix)
}

func handleOnlineAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	adminCount := 0
	memberCount := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		} else {
			memberCount++
		}
	}
	total := len(gi.Participants)
	var b strings.Builder
	b.WriteString("🟢 *GROUP LIVE COUNT*\n\n")
	fmt.Fprintf(&b, "*GROUP:* %s\n", gi.GroupName.Name)
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&b, "👥 *TOTAL PARTICIPANTS:* %d\n", total)
	fmt.Fprintf(&b, "🔰 *ADMINS:* %d\n", adminCount)
	fmt.Fprintf(&b, "👤 *MEMBERS:* %d\n", memberCount)
	b.WriteString("━━━━━━━━━━━━━━━━━━━")
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// myrole  (caller's role)
// ---------------------------------------------------------------------------

func handleMyRole(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleMyRoleAsync(s, info, args, prefix)
}

func handleMyRoleAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	sender := info.Sender.String()
	role := "👤 *REGULAR MEMBER*"
	for _, p := range gi.Participants {
		if p.JID.String() == sender {
			if p.IsSuperAdmin {
				role = "🔰 *GROUP OWNER (SUPER ADMIN)*"
			} else if p.IsAdmin {
				role = "⭐ *GROUP ADMIN*"
			} else {
				role = "👤 *REGULAR MEMBER*"
			}
			break
		}
	}
	if sender == gi.OwnerJID.String() {
		role = "🔰 *GROUP OWNER (SUPER ADMIN)*"
	}
	s.Reply(info, "🏷️ *YOUR ROLE*\n\n"+role)
}

// ---------------------------------------------------------------------------
// tagall / tagmembers  (mention every member)
// ---------------------------------------------------------------------------

func handleTagAll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTagAllAsync(s, info, args, prefix)
}

func handleTagAllAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	custom := strings.TrimSpace(strings.Join(args, " "))
	var b strings.Builder
	if custom != "" {
		fmt.Fprintf(&b, "📢 *%s*\n\n", custom)
	} else {
		b.WriteString("📢 *ATTENTION EVERYONE!*\n\n")
	}
	mentioned := make([]string, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		mentioned = append(mentioned, p.JID.String())
		fmt.Fprintf(&b, "@%s\n", p.JID.User)
	}
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *TAG FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// tagadmin / tagadmins  (mention every admin)
// ---------------------------------------------------------------------------

func handleTagAdmin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTagAdminAsync(s, info, args, prefix)
}

func handleTagAdminAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	admins := make([]types.GroupParticipant, 0)
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins = append(admins, p)
		}
	}
	custom := strings.TrimSpace(strings.Join(args, " "))
	var b strings.Builder
	if custom != "" {
		fmt.Fprintf(&b, "🔰 *ADMINS — %s*\n\n", custom)
	} else {
		b.WriteString("🔰 *ATTENTION ADMINS!*\n\n")
	}
	mentioned := make([]string, 0, len(admins))
	for _, p := range admins {
		mentioned = append(mentioned, p.JID.String())
		role := "⭐"
		if p.IsSuperAdmin {
			role = "🔰"
		}
		fmt.Fprintf(&b, "%s @%s\n", role, p.JID.User)
	}
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *ADMIN TAG FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// leave  (leave the group)
// ---------------------------------------------------------------------------

func handleLeave(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleLeaveAsync(s, info, args, prefix)
}

func handleLeaveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	s.Reply(info, "🚪 *BYE BYE EVERYONE!*\n\n*I'M LEAVING THIS GROUP* 😢\n*TAKE CARE* 🤝")
	if err := client.LeaveGroup(context.Background(), info.Chat); err != nil {
		s.Reply(info, "❌ *LEAVE FAILED:* "+err.Error())
		return
	}
}

// ---------------------------------------------------------------------------
// announce  (broadcast message mentioning all)
// ---------------------------------------------------------------------------

func handleAnnounce(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAnnounceAsync(s, info, args, prefix)
}

func handleAnnounceAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	q := strings.TrimSpace(strings.Join(args, " "))
	if q == "" {
		s.Reply(info, "*TYPE THE ANNOUNCEMENT TEXT\nEXAMPLE: .ANNOUNCE MEETING TODAY*")
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	var b strings.Builder
	b.WriteString("📣 *ANNOUNCEMENT*\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━\n\n")
	b.WriteString(q + "\n\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━")
	mentioned := make([]string, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		mentioned = append(mentioned, p.JID.String())
	}
	if err := sendMentionText(client, info, b.String(), mentioned); err != nil {
		s.Reply(info, "❌ *ANNOUNCEMENT FAILED:* "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// editgc  (group-info edit lock toggle, supports on/off argument)
// ---------------------------------------------------------------------------

func handleEditGC(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleEditGCAsync(s, info, args, prefix)
}

func handleEditGCAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	// Determine target lock state.
	// If an argument is given ("on" → lock, "off" → unlock), use it.
	// Otherwise toggle the current state.
	arg := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	var newLocked bool
	switch arg {
	case "on", "lock", "1", "true":
		newLocked = true
	case "off", "unlock", "0", "false":
		newLocked = false
	default:
		newLocked = !gi.GroupLocked.IsLocked
	}
	if err := client.SetGroupLocked(context.Background(), info.Chat, newLocked); err != nil {
		s.Reply(info, "❌ *FAILED TO UPDATE GROUP EDIT SETTING:* "+err.Error())
		return
	}
	if newLocked {
		s.Reply(info, "🔒 *GROUP INFO LOCKED.* Only admins can edit the group name / description / photo now.")
	} else {
		s.Reply(info, "🔓 *GROUP INFO UNLOCKED.* All members can edit the group info now.")
	}
}

// ---------------------------------------------------------------------------
// lockgc  (enable join approval required — new members need admin approval)
// ---------------------------------------------------------------------------

func handleLockGC(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleLockGCAsync(s, info, args, prefix)
}

func handleLockGCAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	if err := client.SetGroupJoinApprovalMode(context.Background(), info.Chat, true); err != nil {
		s.Reply(info, "❌ *LOCK GC FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔒 *GROUP JOIN APPROVAL ENABLED*\n\n*NEW MEMBERS NOW NEED ADMIN APPROVAL TO JOIN* ✅")
}

// ---------------------------------------------------------------------------
// unlockgc  (disable join approval — anyone can join freely)
// ---------------------------------------------------------------------------

func handleUnlockGC(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleUnlockGCAsync(s, info, args, prefix)
}

func handleUnlockGCAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ WhatsApp client not ready.")
		return
	}
	if err := client.SetGroupJoinApprovalMode(context.Background(), info.Chat, false); err != nil {
		s.Reply(info, "❌ *UNLOCK GC FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔓 *GROUP JOIN APPROVAL DISABLED*\n\n*ANYONE CAN NOW JOIN FREELY WITHOUT APPROVAL* ✅")
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func init() {
	// ── primary group commands ───────────────────────────────────────────
	Register(Command{Name: "mute", Category: "GROUP MANAGEMENT", Desc: "Mute (close) the group — only admins can send", OwnerOnly: true, Run: handleMute})
	Register(Command{Name: "unmute", Category: "GROUP MANAGEMENT", Desc: "Unmute (open) the group — everyone can send", OwnerOnly: true, Run: handleUnmute})
	Register(Command{Name: "kick", Category: "GROUP MANAGEMENT", Desc: "Remove a member from the group", OwnerOnly: true, Run: handleKick})
	Register(Command{Name: "promote", Category: "GROUP MANAGEMENT", Desc: "Make a member a group admin", OwnerOnly: true, Run: handlePromote})
	Register(Command{Name: "demote", Category: "GROUP MANAGEMENT", Desc: "Remove admin rights from a member", OwnerOnly: true, Run: handleDemote})
	Register(Command{Name: "invitelink", Category: "GROUP MANAGEMENT", Desc: "Get the group invite link", OwnerOnly: true, Run: handleInviteLink})
	Register(Command{Name: "revokelink", Category: "GROUP MANAGEMENT", Desc: "Revoke & regenerate the group invite link", OwnerOnly: true, Run: handleRevokeLink})
	Register(Command{Name: "gname", Category: "GROUP MANAGEMENT", Desc: "Change the group name", OwnerOnly: true, Run: handleSetGName})
	Register(Command{Name: "gdesc", Category: "GROUP MANAGEMENT", Desc: "Change the group description", OwnerOnly: true, Run: handleSetGDesc})
	Register(Command{Name: "members", Category: "GROUP MANAGEMENT", Desc: "List all group members", OwnerOnly: true, Run: handleMembers})
	Register(Command{Name: "groupinfo", Category: "GROUP MANAGEMENT", Desc: "Show full group info/settings", OwnerOnly: true, Run: handleGroupInfo})
	Register(Command{Name: "tagall", Category: "GROUP MANAGEMENT", Desc: "Mention (tag) all members in the group", OwnerOnly: true, Run: handleTagAll})
	Register(Command{Name: "tagadmins", Category: "GROUP MANAGEMENT", Desc: "Mention (tag) all group admins", OwnerOnly: true, Run: handleTagAdmin})
	Register(Command{Name: "leave", Category: "GROUP MANAGEMENT", Desc: "Make the bot leave the group", OwnerOnly: true, Run: handleLeave})
	Register(Command{Name: "count", Category: "GROUP MANAGEMENT", Desc: "Show total member count of the group", OwnerOnly: true, Run: handleCount})
	Register(Command{Name: "gonline", Category: "GROUP MANAGEMENT", Desc: "List currently online members in the group", OwnerOnly: true, Run: handleOnline})
	Register(Command{Name: "editgc", Category: "GROUP MANAGEMENT", Desc: "Toggle who can edit group info (admins/all)", OwnerOnly: true, Run: handleEditGC})
	Register(Command{Name: "lockgc", Category: "GROUP MANAGEMENT", Desc: "Lock group — only admins can send messages", OwnerOnly: true, Run: handleLockGC})
	Register(Command{Name: "unlockgc", Category: "GROUP MANAGEMENT", Desc: "Unlock group — everyone can send messages", OwnerOnly: true, Run: handleUnlockGC})
	Register(Command{Name: "myrole", Category: "GROUP MANAGEMENT", Desc: "Show your role/position in the group", OwnerOnly: true, Run: handleMyRole})
	Register(Command{Name: "adminlist", Category: "GROUP MANAGEMENT", Desc: "List all group admins", OwnerOnly: true, Run: handleAdminList})
	Register(Command{Name: "announce", Category: "GROUP MANAGEMENT", Desc: "Toggle announce mode (only admins send)", OwnerOnly: true, Run: handleAnnounce})

	// ── mute aliases (5+) ────────────────────────────────────────────────
	Register(Command{Name: "close", OwnerOnly: true, Hidden: true, Run: handleMute})
	Register(Command{Name: "lockchat", OwnerOnly: true, Hidden: true, Run: handleMute})
	Register(Command{Name: "gclose", OwnerOnly: true, Hidden: true, Run: handleMute})
	Register(Command{Name: "groupmute", OwnerOnly: true, Hidden: true, Run: handleMute})
	Register(Command{Name: "shutup", OwnerOnly: true, Hidden: true, Run: handleMute})

	// ── unmute aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "open", OwnerOnly: true, Hidden: true, Run: handleUnmute})
	Register(Command{Name: "openchat", OwnerOnly: true, Hidden: true, Run: handleUnmute})
	Register(Command{Name: "gopen", OwnerOnly: true, Hidden: true, Run: handleUnmute})
	Register(Command{Name: "groupunmute", OwnerOnly: true, Hidden: true, Run: handleUnmute})
	Register(Command{Name: "unlockchat", OwnerOnly: true, Hidden: true, Run: handleUnmute})

	// ── kick aliases (5+) ────────────────────────────────────────────────
	Register(Command{Name: "remove", OwnerOnly: true, Hidden: true, Run: handleKick})
	Register(Command{Name: "ban", OwnerOnly: true, Hidden: true, Run: handleKick})
	Register(Command{Name: "kickout", OwnerOnly: true, Hidden: true, Run: handleKick})
	Register(Command{Name: "kik", OwnerOnly: true, Hidden: true, Run: handleKick})

	// ── promote aliases (5+) ─────────────────────────────────────────────
	Register(Command{Name: "addadmin", OwnerOnly: true, Hidden: true, Run: handlePromote})
	Register(Command{Name: "makeadmin", OwnerOnly: true, Hidden: true, Run: handlePromote})
	Register(Command{Name: "promoteadmin", OwnerOnly: true, Hidden: true, Run: handlePromote})
	Register(Command{Name: "add", OwnerOnly: true, Hidden: true, Run: handlePromote})
	Register(Command{Name: "adm", OwnerOnly: true, Hidden: true, Run: handlePromote})

	// ── demote aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "removeadmin", OwnerOnly: true, Hidden: true, Run: handleDemote})
	Register(Command{Name: "unadmin", OwnerOnly: true, Hidden: true, Run: handleDemote})
	Register(Command{Name: "demoteadmin", OwnerOnly: true, Hidden: true, Run: handleDemote})
	Register(Command{Name: "deladmin", OwnerOnly: true, Hidden: true, Run: handleDemote})
	Register(Command{Name: "unadm", OwnerOnly: true, Hidden: true, Run: handleDemote})

	// ── invitelink / link aliases (5+) ───────────────────────────────────
	Register(Command{Name: "link", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "glink", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "grouplink", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "linkgc", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "gcgroup", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "gclink", OwnerOnly: true, Hidden: true, Run: handleInviteLink})
	Register(Command{Name: "groupurl", OwnerOnly: true, Hidden: true, Run: handleInviteLink})

	// ── revokelink aliases (5+) ──────────────────────────────────────────
	Register(Command{Name: "resetlink", OwnerOnly: true, Hidden: true, Run: handleRevokeLink})
	Register(Command{Name: "revokegc", OwnerOnly: true, Hidden: true, Run: handleRevokeLink})
	Register(Command{Name: "resetgc", OwnerOnly: true, Hidden: true, Run: handleRevokeLink})
	Register(Command{Name: "newlink", OwnerOnly: true, Hidden: true, Run: handleRevokeLink})
	Register(Command{Name: "regeneratelink", OwnerOnly: true, Hidden: true, Run: handleRevokeLink})

	// ── gname / set group name aliases (5+) ──────────────────────────────
	Register(Command{Name: "setgname", OwnerOnly: true, Hidden: true, Run: handleSetGName})
	Register(Command{Name: "setname", OwnerOnly: true, Hidden: true, Run: handleSetGName})
	Register(Command{Name: "groupname", OwnerOnly: true, Hidden: true, Run: handleSetGName})
	Register(Command{Name: "changename", OwnerOnly: true, Hidden: true, Run: handleSetGName})
	Register(Command{Name: "newgname", OwnerOnly: true, Hidden: true, Run: handleSetGName})
	Register(Command{Name: "gcname", OwnerOnly: true, Hidden: true, Run: handleSetGName})

	// ── gdesc / set group description aliases (5+) ───────────────────────
	Register(Command{Name: "setgdesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "setdesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "groupdesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "changedesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "newgdesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "gcdesc", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})
	Register(Command{Name: "gctopic", OwnerOnly: true, Hidden: true, Run: handleSetGDesc})

	// ── members aliases (5+) ─────────────────────────────────────────────
	Register(Command{Name: "listmembers", OwnerOnly: true, Hidden: true, Run: handleMembers})
	Register(Command{Name: "gcmembers", OwnerOnly: true, Hidden: true, Run: handleMembers})
	Register(Command{Name: "allmembers", OwnerOnly: true, Hidden: true, Run: handleMembers})
	Register(Command{Name: "participantlist", OwnerOnly: true, Hidden: true, Run: handleMembers})
	Register(Command{Name: "showmembers", OwnerOnly: true, Hidden: true, Run: handleMembers})

	// ── groupinfo aliases (5+) ───────────────────────────────────────────
	Register(Command{Name: "gcinfo", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})
	Register(Command{Name: "infogc", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})
	Register(Command{Name: "ginfo", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})
	Register(Command{Name: "groupdata", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})
	Register(Command{Name: "aboutgc", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})
	Register(Command{Name: "groupdetails", OwnerOnly: true, Hidden: true, Run: handleGroupInfo})

	// ── tagall aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "tagmembers", OwnerOnly: true, Hidden: true, Run: handleTagAll})
	Register(Command{Name: "mentionall", OwnerOnly: true, Hidden: true, Run: handleTagAll})
	Register(Command{Name: "everyone", OwnerOnly: true, Hidden: true, Run: handleTagAll})
	Register(Command{Name: "hidetag", OwnerOnly: true, Hidden: true, Run: handleTagAll})
	Register(Command{Name: "tageveryone", OwnerOnly: true, Hidden: true, Run: handleTagAll})
	Register(Command{Name: "all", OwnerOnly: true, Hidden: true, Run: handleTagAll})

	// ── tagadmins aliases (5+) ───────────────────────────────────────────
	Register(Command{Name: "tagadmin", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})
	Register(Command{Name: "mentionadmins", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})
	Register(Command{Name: "admintag", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})
	Register(Command{Name: "tagadm", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})
	Register(Command{Name: "admins", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})
	Register(Command{Name: "notifyadmins", OwnerOnly: true, Hidden: true, Run: handleTagAdmin})

	// ── leave aliases (5+) ───────────────────────────────────────────────
	Register(Command{Name: "leavegc", OwnerOnly: true, Hidden: true, Run: handleLeave})
	Register(Command{Name: "exit", OwnerOnly: true, Hidden: true, Run: handleLeave})
	Register(Command{Name: "exitgroup", OwnerOnly: true, Hidden: true, Run: handleLeave})
	Register(Command{Name: "quitgc", OwnerOnly: true, Hidden: true, Run: handleLeave})
	Register(Command{Name: "goodbye", OwnerOnly: true, Hidden: true, Run: handleLeave})

	// ── count aliases (5+) ───────────────────────────────────────────────
	Register(Command{Name: "membercount", OwnerOnly: true, Hidden: true, Run: handleCount})
	Register(Command{Name: "gccount", OwnerOnly: true, Hidden: true, Run: handleCount})
	Register(Command{Name: "totalmembers", OwnerOnly: true, Hidden: true, Run: handleCount})
	Register(Command{Name: "participants", OwnerOnly: true, Hidden: true, Run: handleCount})
	Register(Command{Name: "howmany", OwnerOnly: true, Hidden: true, Run: handleCount})

	// ── online aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "livecount", OwnerOnly: true, Hidden: true, Run: handleOnline})
	Register(Command{Name: "onlinemembers", OwnerOnly: true, Hidden: true, Run: handleOnline})
	Register(Command{Name: "active", OwnerOnly: true, Hidden: true, Run: handleOnline})
	Register(Command{Name: "gconline", OwnerOnly: true, Hidden: true, Run: handleOnline})
	Register(Command{Name: "present", OwnerOnly: true, Hidden: true, Run: handleOnline})

	// ── editgc aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "editgroup", OwnerOnly: true, Hidden: true, Run: handleEditGC})
	Register(Command{Name: "gcsettings", OwnerOnly: true, Hidden: true, Run: handleEditGC})
	Register(Command{Name: "gcedit", OwnerOnly: true, Hidden: true, Run: handleEditGC})
	Register(Command{Name: "editinfo", OwnerOnly: true, Hidden: true, Run: handleEditGC})
	Register(Command{Name: "groupedit", OwnerOnly: true, Hidden: true, Run: handleEditGC})
	Register(Command{Name: "lockinfo", OwnerOnly: true, Hidden: true, Run: handleEditGC})

	// ── lockgc aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "approvelock", OwnerOnly: true, Hidden: true, Run: handleLockGC})
	Register(Command{Name: "joinapproval", OwnerOnly: true, Hidden: true, Run: handleLockGC})
	Register(Command{Name: "lockjoins", OwnerOnly: true, Hidden: true, Run: handleLockGC})
	Register(Command{Name: "reqapprove", OwnerOnly: true, Hidden: true, Run: handleLockGC})
	Register(Command{Name: "approvalon", OwnerOnly: true, Hidden: true, Run: handleLockGC})
	Register(Command{Name: "gclock", OwnerOnly: true, Hidden: true, Run: handleLockGC})

	// ── unlockgc aliases (5+) ────────────────────────────────────────────
	Register(Command{Name: "approveunlock", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})
	Register(Command{Name: "noapproval", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})
	Register(Command{Name: "unlockjoins", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})
	Register(Command{Name: "freejoin", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})
	Register(Command{Name: "approvaloff", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})
	Register(Command{Name: "gcunlock", OwnerOnly: true, Hidden: true, Run: handleUnlockGC})

	// ── myrole aliases (5+) ──────────────────────────────────────────────
	Register(Command{Name: "role", OwnerOnly: true, Hidden: true, Run: handleMyRole})
	Register(Command{Name: "whoami", OwnerOnly: true, Hidden: true, Run: handleMyRole})
	Register(Command{Name: "myposition", OwnerOnly: true, Hidden: true, Run: handleMyRole})
	Register(Command{Name: "checkrole", OwnerOnly: true, Hidden: true, Run: handleMyRole})
	Register(Command{Name: "amiamin", OwnerOnly: true, Hidden: true, Run: handleMyRole})

	// ── adminlist aliases (5+) ───────────────────────────────────────────
	Register(Command{Name: "adminslist", OwnerOnly: true, Hidden: true, Run: handleAdminList})
	Register(Command{Name: "listadmins", OwnerOnly: true, Hidden: true, Run: handleAdminList})
	Register(Command{Name: "gadmins", OwnerOnly: true, Hidden: true, Run: handleAdminList})
	Register(Command{Name: "showadmins", OwnerOnly: true, Hidden: true, Run: handleAdminList})
	Register(Command{Name: "gcadmins", OwnerOnly: true, Hidden: true, Run: handleAdminList})

	// NOTE: .del is now a message-delete command (delete.go), not a kick alias.
}
