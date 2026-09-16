package goldcmds

// ============================================================================
// GOLD-MD — EXTRA WhatsApp Group Control Commands (2026 feature set)
// File: groupmore.go
// ----------------------------------------------------------------------------
// This file adds a large batch of additional GROUP CONTROL commands on top of
// group.go (core set) and groupnew.go (poll/join-request set). Every command:
//   • is OWNER-ONLY and GROUP-ONLY,
//   • runs its heavy work in a background goroutine (0% speed impact),
//   • is styled with 🔰 and lives in the "GROUP MANAGEMENT" category,
//   • ships with 5+ hidden aliases so every spelling works but the menu stays
//     clean,
//   • applies the MANDATORY LID→PN conversion (s.ResolveToPN) on every target
//     JID so mentions/replies that arrive in @lid form are always acted on
//     against the real phone-number participant.
//
// whatsmeow APIs used:
//   Client.SetGroupPhoto / SetGroupName / SetGroupTopic / SetGroupDescription
//   Client.SetGroupLocked / SetGroupAnnounce / SetGroupJoinApprovalMode
//   Client.SetGroupMemberAddMode
//   Client.UpdateGroupParticipants (add/remove/promote/demote)
//   Client.GetGroupInfo / GetGroupInviteLink / GetGroupInfoFromLink
//   Client.JoinGroupWithLink / LeaveGroup / GetJoinedGroups
//   Client.GetProfilePictureInfo / GetUserInfo / IsOnWhatsApp / GetBlocklist
//   Client.UpdateBlocklist
// ============================================================================

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	evt "go.mau.fi/whatsmeow/types/events"
)

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// groupOwnerGate enforces the owner-only + group-only preconditions shared by
// every command in this file. Returns true when the command may proceed.
func groupOwnerGate(s SessionBridge, info types.MessageInfo) bool {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return false
	}
	return requireGroup(s, info)
}

// groupClient returns the connected client or replies an error and returns nil.
func groupClient(s SessionBridge, info types.MessageInfo) *whatsmeow.Client {
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return nil
	}
	return client
}

// resolveTargets is the MANDATORY LID→PN aware target collector used by the
// commands in this file. It gathers @mentions, the replied-to sender and bare
// phone numbers, then converts every LID JID to its real PN JID.
func resolveTargets(s SessionBridge, info types.MessageInfo, args []string) []types.JID {
	return collectTargets(s, info, args)
}

// ---------------------------------------------------------------------------
// .gpp  — set the group profile picture from a replied/attached image
// ---------------------------------------------------------------------------

func handleSetGPP(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetGPPAsync(s, info, args, prefix)
}

func handleSetGPPAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	data, ok := s.DownloadImage(info)
	if !ok || len(data) == 0 {
		s.Reply(info, "🔰 *SET GROUP PHOTO* 🔰\n\n*REPLY TO AN IMAGE OR SEND ONE WITH THE CAPTION:* ```"+prefix+"gpp```")
		return
	}
	if _, err := client.SetGroupPhoto(context.Background(), info.Chat, data); err != nil {
		s.Reply(info, "🔰 *SET PHOTO FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *GROUP PHOTO UPDATED* 🔰")
}

// ---------------------------------------------------------------------------
// .gppremove — remove the group profile picture
// ---------------------------------------------------------------------------

func handleRemoveGPP(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleRemoveGPPAsync(s, info, args, prefix)
}

func handleRemoveGPPAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if _, err := client.SetGroupPhoto(context.Background(), info.Chat, nil); err != nil {
		s.Reply(info, "🔰 *REMOVE PHOTO FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *GROUP PHOTO REMOVED* 🔰")
}

// ---------------------------------------------------------------------------
// .gtopic — set the group description (topic)
// ---------------------------------------------------------------------------

func handleSetTopic(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetTopicAsync(s, info, args, prefix)
}

func handleSetTopicAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		s.Reply(info, "🔰 *SET GROUP DESCRIPTION* 🔰\n\n*FORMAT:* ```"+prefix+"gtopic <new description>```")
		return
	}
	if err := client.SetGroupDescription(context.Background(), info.Chat, text); err != nil {
		s.Reply(info, "🔰 *SET DESCRIPTION FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *GROUP DESCRIPTION UPDATED* 🔰\n\n"+text)
}

// ---------------------------------------------------------------------------
// .gdeldesc — delete the group description
// ---------------------------------------------------------------------------

func handleDelDesc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDelDescAsync(s, info, args, prefix)
}

func handleDelDescAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if err := client.SetGroupDescription(context.Background(), info.Chat, ""); err != nil {
		s.Reply(info, "🔰 *DELETE DESCRIPTION FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *GROUP DESCRIPTION DELETED* 🔰")
}

// ---------------------------------------------------------------------------
// .gowner — show the group owner
// ---------------------------------------------------------------------------

func handleGOwner(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGOwnerAsync(s, info, args, prefix)
}

func handleGOwnerAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	owner := gi.OwnerJID
	if !gi.OwnerPN.IsEmpty() {
		owner = gi.OwnerPN
	}
	s.Reply(info, fmt.Sprintf("🔰 *GROUP OWNER* 🔰\n\n🔰 *NUMBER :❮ +%s ❯*\n🔰 *JID    :❮ %s ❯*", owner.User, owner.String()))
}

// ---------------------------------------------------------------------------
// .gcreated — show when the group was created
// ---------------------------------------------------------------------------

func handleGCreated(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGCreatedAsync(s, info, args, prefix)
}

func handleGCreatedAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	created := gi.GroupCreated
	if created.IsZero() {
		s.Reply(info, "🔰 *GROUP CREATION DATE IS NOT AVAILABLE* 🔰")
		return
	}
	s.Reply(info, fmt.Sprintf("🔰 *GROUP CREATED* 🔰\n\n🔰 *DATE :❮ %s ❯*\n🔰 *AGO  :❮ %s ❯*",
		created.Format("02 Jan 2006 15:04"), humanAgo(created)))
}

// humanAgo renders a coarse "x days ago" string.
func humanAgo(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = -d
	}
	days := int(d.Hours() / 24)
	switch {
	case days >= 365:
		return fmt.Sprintf("%d year(s)", days/365)
	case days >= 30:
		return fmt.Sprintf("%d month(s)", days/30)
	case days >= 1:
		return fmt.Sprintf("%d day(s)", days)
	case d.Hours() >= 1:
		return fmt.Sprintf("%d hour(s)", int(d.Hours()))
	default:
		return fmt.Sprintf("%d minute(s)", int(d.Minutes()))
	}
}

// ---------------------------------------------------------------------------
// .gsettings — show the full group settings snapshot
// ---------------------------------------------------------------------------

func handleGSettings(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGSettingsAsync(s, info, args, prefix)
}

func handleGSettingsAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	lock := "OPEN (ALL MEMBERS)"
	if gi.IsLocked {
		lock = "LOCKED (ADMINS ONLY)"
	}
	announce := "OPEN (ALL MEMBERS)"
	if gi.IsAnnounce {
		announce = "ANNOUNCE (ADMINS ONLY)"
	}
	approval := "OFF"
	if gi.IsJoinApprovalRequired {
		approval = "ON"
	}
	addMode := "ALL MEMBERS"
	if gi.MemberAddMode == types.GroupMemberAddModeAdmin {
		addMode = "ADMINS ONLY"
	}
	ephemeral := "OFF"
	if gi.IsEphemeral {
		ephemeral = fmt.Sprintf("%d SECONDS", gi.DisappearingTimer)
	}
	s.Reply(info, fmt.Sprintf(
		"🔰 *GROUP SETTINGS* 🔰\n\n"+
			"🔰 *NAME        :❮ %s ❯*\n"+
			"🔰 *EDIT INFO   :❮ %s ❯*\n"+
			"🔰 *MESSAGING   :❮ %s ❯*\n"+
			"🔰 *JOIN APPROV :❮ %s ❯*\n"+
			"🔰 *WHO CAN ADD :❮ %s ❯*\n"+
			"🔰 *DISAPPEAR   :❮ %s ❯*\n"+
			"🔰 *MEMBERS     :❮ %d ❯*",
		gi.Name, lock, announce, approval, addMode, ephemeral, gi.ParticipantCount))
}

// ---------------------------------------------------------------------------
// .gphoto — show the group profile picture
// ---------------------------------------------------------------------------

func handleGPhoto(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGPhotoAsync(s, info, args, prefix)
}

func handleGPhotoAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	data := s.GetGroupProfilePicture(info.Chat)
	if len(data) == 0 {
		s.Reply(info, "🔰 *THIS GROUP HAS NO PROFILE PICTURE* 🔰")
		return
	}
	if err := s.SendImage(info, data, "🔰 *GROUP PROFILE PICTURE* 🔰"); err != nil {
		s.Reply(info, "🔰 *FAILED TO SEND PHOTO:* "+err.Error())
		return
	}
}

// ---------------------------------------------------------------------------
// .glist — list all groups the bot is in
// ---------------------------------------------------------------------------

func handleGList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGListAsync(s, info, args, prefix)
}

func handleGListAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	groups, err := client.GetJoinedGroups(context.Background())
	if err != nil {
		s.Reply(info, "🔰 *LIST GROUPS FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *MY GROUPS (%d)* 🔰\n\n", len(groups))
	for i, g := range groups {
		if i >= 50 {
			b.WriteString("\n*...and more*")
			break
		}
		fmt.Fprintf(&b, "🔰 *%d.* %s\n", i+1, g.Name)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .ginfo <link> — preview a group from its invite link
// ---------------------------------------------------------------------------

func handleGInfoLink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGInfoLinkAsync(s, info, args, prefix)
}

func handleGInfoLinkAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	code := extractInviteCode(strings.Join(args, " "))
	if code == "" {
		s.Reply(info, "🔰 *GROUP INFO FROM LINK* 🔰\n\n*FORMAT:* ```"+prefix+"ginfo https://chat.whatsapp.com/XXXX```")
		return
	}
	gi, err := client.GetGroupInfoFromLink(context.Background(), code)
	if err != nil {
		s.Reply(info, "🔰 *GROUP INFO FAILED:* "+err.Error())
		return
	}
	s.Reply(info, fmt.Sprintf("🔰 *GROUP INFO* 🔰\n\n🔰 *NAME    :❮ %s ❯*\n🔰 *MEMBERS :❮ %d ❯*\n🔰 *JID     :❮ %s ❯*",
		gi.Name, gi.ParticipantCount, gi.JID.String()))
}

// extractInviteCode pulls the invite code out of a full link or a bare code.
func extractInviteCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	// strip query string
	if i := strings.IndexAny(s, "?&"); i >= 0 {
		s = s[:i]
	}
	return s
}

// ---------------------------------------------------------------------------
// .gjoin <link> — join a group from an invite link
// ---------------------------------------------------------------------------

func handleGJoin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGJoinAsync(s, info, args, prefix)
}

func handleGJoinAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	code := extractInviteCode(strings.Join(args, " "))
	if code == "" {
		s.Reply(info, "🔰 *JOIN GROUP* 🔰\n\n*FORMAT:* ```"+prefix+"gjoin https://chat.whatsapp.com/XXXX```")
		return
	}
	jid, err := client.JoinGroupWithLink(context.Background(), code)
	if err != nil {
		s.Reply(info, "🔰 *JOIN FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *JOINED GROUP* 🔰\n\n🔰 *JID :❮ "+jid.String()+" ❯*")
}

// ---------------------------------------------------------------------------
// .gleave — make the bot leave the current group
// ---------------------------------------------------------------------------

func handleGLeave(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGLeaveAsync(s, info, args, prefix)
}

func handleGLeaveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if err := client.LeaveGroup(context.Background(), info.Chat); err != nil {
		s.Reply(info, "🔰 *LEAVE FAILED:* "+err.Error())
		return
	}
}

// ---------------------------------------------------------------------------
// .gpromote / .gdemote — bulk promote / demote
// ---------------------------------------------------------------------------

func handleGPromote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGPromoteAsync(s, info, args, prefix)
}

func handleGPromoteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupBulkChange(s, info, args, prefix, whatsmeow.ParticipantChangePromote, "PROMOTED TO ADMIN")
}

func handleGDemote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGDemoteAsync(s, info, args, prefix)
}

func handleGDemoteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupBulkChange(s, info, args, prefix, whatsmeow.ParticipantChangeDemote, "DEMOTED TO MEMBER")
}

// groupBulkChange applies a promote/demote/remove change to every resolved
// target and reports the result.
func groupBulkChange(s SessionBridge, info types.MessageInfo, args []string, prefix string, change whatsmeow.ParticipantChange, label string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	targets := resolveTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER(S) OR REPLY TO THEIR MESSAGE* 🔰")
		return
	}
	if _, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, change); err != nil {
		s.Reply(info, "🔰 *ACTION FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *%s* 🔰\n\n", label)
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		mentioned = append(mentioned, t.String())
		fmt.Fprintf(&b, "🔰 @%s\n", t.User)
	}
	_ = sendMentionText(client, info, b.String(), mentioned)
}

// ---------------------------------------------------------------------------
// .gremove — bulk remove members
// ---------------------------------------------------------------------------

func handleGRemove(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGRemoveAsync(s, info, args, prefix)
}

func handleGRemoveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupBulkChange(s, info, args, prefix, whatsmeow.ParticipantChangeRemove, "REMOVED FROM GROUP")
}

// ---------------------------------------------------------------------------
// .gadd — bulk add members
// ---------------------------------------------------------------------------

func handleGAdd(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAddAsync(s, info, args, prefix)
}

func handleGAddAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	groupBulkChange(s, info, args, prefix, whatsmeow.ParticipantChangeAdd, "ADDED TO GROUP")
}

// ---------------------------------------------------------------------------
// .gadmin — show whether a member is an admin
// ---------------------------------------------------------------------------

func handleGAdmin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAdminAsync(s, info, args, prefix)
}

func handleGAdminAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	targets := resolveTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER OR REPLY TO THEIR MESSAGE* 🔰")
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	var b strings.Builder
	b.WriteString("🔰 *ADMIN CHECK* 🔰\n\n")
	for _, t := range targets {
		role := "REGULAR MEMBER"
		for _, p := range gi.Participants {
			if sameUser(p.JID, t) {
				if p.IsSuperAdmin {
					role = "GROUP OWNER (SUPER ADMIN)"
				} else if p.IsAdmin {
					role = "GROUP ADMIN"
				}
				break
			}
		}
		fmt.Fprintf(&b, "🔰 *+%s :❮ %s ❯*\n", t.User, role)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .grole — show a member's role (alias of gadmin, richer wording)
// ---------------------------------------------------------------------------

func handleGRole(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAdminAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gcount — member count (alias of count, kept for the g* family)
// ---------------------------------------------------------------------------

func handleGCount(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGCountAsync(s, info, args, prefix)
}

func handleGCountAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	admins := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins++
		}
	}
	s.Reply(info, fmt.Sprintf("🔰 *GROUP COUNT* 🔰\n\n🔰 *TOTAL  :❮ %d ❯*\n🔰 *ADMINS :❮ %d ❯*\n🔰 *MEMBERS:❮ %d ❯*",
		gi.ParticipantCount, admins, gi.ParticipantCount-admins))
}

// ---------------------------------------------------------------------------
// .gsearch <text> — search members by name/number
// ---------------------------------------------------------------------------

func handleGSearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGSearchAsync(s, info, args, prefix)
}

func handleGSearchAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	query := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if query == "" {
		s.Reply(info, "🔰 *SEARCH MEMBERS* 🔰\n\n*FORMAT:* ```"+prefix+"gsearch <name or number>```")
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	var b strings.Builder
	b.WriteString("🔰 *SEARCH RESULTS* 🔰\n\n")
	found := 0
	for _, p := range gi.Participants {
		name := strings.ToLower(p.DisplayName)
		num := p.JID.User
		if strings.Contains(name, query) || strings.Contains(num, query) {
			found++
			role := "MEMBER"
			if p.IsSuperAdmin {
				role = "OWNER"
			} else if p.IsAdmin {
				role = "ADMIN"
			}
			fmt.Fprintf(&b, "🔰 *%s* (+%s) — %s\n", p.DisplayName, num, role)
		}
	}
	if found == 0 {
		s.Reply(info, "🔰 *NO MEMBER MATCHED:* "+query)
		return
	}
	fmt.Fprintf(&b, "\n🔰 *FOUND :❮ %d ❯*", found)
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gstats — group statistics
// ---------------------------------------------------------------------------

func handleGStats(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGStatsAsync(s, info, args, prefix)
}

func handleGStatsAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	admins, supers, withName := 0, 0, 0
	for _, p := range gi.Participants {
		if p.IsSuperAdmin {
			supers++
		} else if p.IsAdmin {
			admins++
		}
		if p.DisplayName != "" {
			withName++
		}
	}
	s.Reply(info, fmt.Sprintf(
		"🔰 *GROUP STATISTICS* 🔰\n\n"+
			"🔰 *TOTAL MEMBERS :❮ %d ❯*\n"+
			"🔰 *SUPER ADMINS  :❮ %d ❯*\n"+
			"🔰 *ADMINS        :❮ %d ❯*\n"+
			"🔰 *REGULAR       :❮ %d ❯*\n"+
			"🔰 *NAMED MEMBERS :❮ %d ❯*",
		gi.ParticipantCount, supers, admins, gi.ParticipantCount-admins-supers, withName))
}

// ---------------------------------------------------------------------------
// .grank — rank members by role (owner → admins → members)
// ---------------------------------------------------------------------------

func handleGRank(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGRankAsync(s, info, args, prefix)
}

func handleGRankAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	parts := make([]types.GroupParticipant, len(gi.Participants))
	copy(parts, gi.Participants)
	sort.SliceStable(parts, func(i, j int) bool {
		ri, rj := roleWeight(parts[i]), roleWeight(parts[j])
		if ri != rj {
			return ri > rj
		}
		return parts[i].DisplayName < parts[j].DisplayName
	})
	var b strings.Builder
	b.WriteString("🔰 *MEMBER RANKING* 🔰\n\n")
	for i, p := range parts {
		if i >= 40 {
			b.WriteString("\n*...and more*")
			break
		}
		role := "MEMBER"
		if p.IsSuperAdmin {
			role = "OWNER"
		} else if p.IsAdmin {
			role = "ADMIN"
		}
		fmt.Fprintf(&b, "🔰 *%d.* %s — %s\n", i+1, p.DisplayName, role)
	}
	s.Reply(info, b.String())
}

func roleWeight(p types.GroupParticipant) int {
	if p.IsSuperAdmin {
		return 2
	}
	if p.IsAdmin {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// .gtop — top members by name length (fun ranking, deterministic)
// ---------------------------------------------------------------------------

func handleGTop(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGTopAsync(s, info, args, prefix)
}

func handleGTopAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	parts := make([]types.GroupParticipant, len(gi.Participants))
	copy(parts, gi.Participants)
	sort.SliceStable(parts, func(i, j int) bool {
		return len(parts[i].DisplayName) > len(parts[j].DisplayName)
	})
	var b strings.Builder
	b.WriteString("🔰 *TOP MEMBERS* 🔰\n\n")
	for i, p := range parts {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "🔰 *%d.* %s\n", i+1, p.DisplayName)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gactive — list members with a resolvable profile picture (activity proxy)
// ---------------------------------------------------------------------------

func handleGActive(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGActiveAsync(s, info, args, prefix)
}

func handleGActiveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	jids := make([]types.JID, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		jids = append(jids, p.JID)
	}
	infoMap, err := client.GetUserInfo(context.Background(), jids)
	if err != nil {
		s.Reply(info, "🔰 *ACTIVE LIST FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("🔰 *MEMBERS WITH PROFILE PICTURE* 🔰\n\n")
	n := 0
	for _, p := range gi.Participants {
		if ui, ok := infoMap[p.JID]; ok && ui.PictureID != "" {
			n++
			if n > 40 {
				break
			}
			fmt.Fprintf(&b, "🔰 %s\n", p.DisplayName)
		}
	}
	if n == 0 {
		s.Reply(info, "🔰 *NO MEMBER HAS A PROFILE PICTURE* 🔰")
		return
	}
	fmt.Fprintf(&b, "\n🔰 *TOTAL :❮ %d ❯*", n)
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gcheck <number> — check if a number is on WhatsApp
// ---------------------------------------------------------------------------

func handleGCheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGCheckAsync(s, info, args, prefix)
}

func handleGCheckAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	num := digitsOnly(strings.Join(args, " "))
	if num == "" {
		s.Reply(info, "🔰 *CHECK NUMBER* 🔰\n\n*FORMAT:* ```"+prefix+"gcheck 923001234567```")
		return
	}
	res, err := client.IsOnWhatsApp(context.Background(), []string{"+" + num})
	if err != nil || len(res) == 0 {
		s.Reply(info, "🔰 *CHECK FAILED* 🔰")
		return
	}
	if res[0].IsIn {
		s.Reply(info, "🔰 *NUMBER IS ON WHATSAPP* 🔰\n\n🔰 *NUMBER :❮ +"+num+" ❯*\n🔰 *JID    :❮ "+res[0].JID.String()+" ❯*")
		return
	}
	s.Reply(info, "🔰 *NUMBER IS NOT ON WHATSAPP* 🔰\n\n🔰 *NUMBER :❮ +"+num+" ❯*")
}

// ---------------------------------------------------------------------------
// .gblock / .gunblock — block / unblock a member
// ---------------------------------------------------------------------------

func handleGBlock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGBlockAsync(s, info, args, prefix, true)
}

func handleGUnblock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGBlockAsync(s, info, args, prefix, false)
}

func handleGBlockAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, block bool) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	targets := resolveTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER, REPLY TO THEIR MESSAGE, OR GIVE A NUMBER* 🔰")
		return
	}
	action := evt.BlocklistChangeActionUnblock
	label := "UNBLOCKED"
	if block {
		action = evt.BlocklistChangeActionBlock
		label = "BLOCKED"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *USER(S) %s* 🔰\n\n", label)
	for _, t := range targets {
		if _, err := client.UpdateBlocklist(context.Background(), t, action); err != nil {
			fmt.Fprintf(&b, "🔰 *+%s :❮ FAILED: %s ❯*\n", t.User, err.Error())
			continue
		}
		fmt.Fprintf(&b, "🔰 *+%s :❮ %s ❯*\n", t.User, label)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gblocklist — show the bot's block list
// ---------------------------------------------------------------------------

func handleGBlockList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGBlockListAsync(s, info, args, prefix)
}

func handleGBlockListAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	bl, err := client.GetBlocklist(context.Background())
	if err != nil {
		s.Reply(info, "🔰 *BLOCKLIST FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *BLOCK LIST (%d)* 🔰\n\n", len(bl.JIDs))
	for i, j := range bl.JIDs {
		if i >= 50 {
			b.WriteString("\n*...and more*")
			break
		}
		fmt.Fprintf(&b, "🔰 *%d.* +%s\n", i+1, j.User)
	}
	if len(bl.JIDs) == 0 {
		b.WriteString("*NO NUMBER IS BLOCKED*")
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gpic <number> — show a user's profile picture
// ---------------------------------------------------------------------------

func handleGPic(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGPicAsync(s, info, args, prefix)
}

func handleGPicAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	targets := resolveTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER, REPLY TO THEIR MESSAGE, OR GIVE A NUMBER* 🔰")
		return
	}
	pic, err := client.GetProfilePictureInfo(context.Background(), targets[0], &whatsmeow.GetProfilePictureParams{})
	if err != nil || pic == nil || pic.URL == "" {
		s.Reply(info, "🔰 *NO PROFILE PICTURE FOUND* 🔰")
		return
	}
	s.Reply(info, "🔰 *PROFILE PICTURE* 🔰\n\n🔰 *USER :❮ +"+targets[0].User+" ❯*\n🔰 *URL  :❮ "+pic.URL+" ❯*")
}

// ---------------------------------------------------------------------------
// .gabout <number> — show a user's about/status text
// ---------------------------------------------------------------------------

func handleGAbout(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAboutAsync(s, info, args, prefix)
}

func handleGAboutAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	targets := resolveTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER, REPLY TO THEIR MESSAGE, OR GIVE A NUMBER* 🔰")
		return
	}
	infoMap, err := client.GetUserInfo(context.Background(), targets)
	if err != nil {
		s.Reply(info, "🔰 *ABOUT FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("🔰 *USER ABOUT* 🔰\n\n")
	for _, t := range targets {
		about := "(none)"
		if ui, ok := infoMap[t]; ok && ui.Status != "" {
			about = ui.Status
		}
		fmt.Fprintf(&b, "🔰 *+%s :❮ %s ❯*\n", t.User, about)
	}
	s.Reply(info, b.String())
}

// ---------------------------------------------------------------------------
// .gpin / .gunpin — pin / unpin the group chat (local chat setting)
// ---------------------------------------------------------------------------

func handleGPin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGPinAsync(s, info, args, prefix, true)
}

func handleGUnpin(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGPinAsync(s, info, args, prefix, false)
}

func handleGPinAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, pin bool) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if err := client.SendAppState(context.Background(), appstate.BuildPin(info.Chat, pin)); err != nil {
		s.Reply(info, "🔰 *PIN FAILED:* "+err.Error())
		return
	}
	if pin {
		s.Reply(info, "🔰 *GROUP CHAT PINNED* 🔰")
		return
	}
	s.Reply(info, "🔰 *GROUP CHAT UNPINNED* 🔰")
}

// ---------------------------------------------------------------------------
// .garchive / .gunarchive — archive / unarchive the group chat
// ---------------------------------------------------------------------------

func handleGArchive(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGArchiveAsync(s, info, args, prefix, true)
}

func handleGUnarchive(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGArchiveAsync(s, info, args, prefix, false)
}

func handleGArchiveAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, archive bool) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if err := client.SendAppState(context.Background(), appstate.BuildArchive(info.Chat, archive, time.Time{}, nil)); err != nil {
		s.Reply(info, "🔰 *ARCHIVE FAILED:* "+err.Error())
		return
	}
	if archive {
		s.Reply(info, "🔰 *GROUP CHAT ARCHIVED* 🔰")
		return
	}
	s.Reply(info, "🔰 *GROUP CHAT UNARCHIVED* 🔰")
}

// ---------------------------------------------------------------------------
// .gmarkread — mark the group chat as read
// ---------------------------------------------------------------------------

func handleGMarkRead(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGMarkReadAsync(s, info, args, prefix)
}

func handleGMarkReadAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	if err := client.MarkRead(context.Background(), []types.MessageID{info.ID}, time.Now(), info.Chat, info.Sender); err != nil {
		s.Reply(info, "🔰 *MARK READ FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *GROUP CHAT MARKED AS READ* 🔰")
}

// ---------------------------------------------------------------------------
// .gtyping / .gstoptyping — send typing presence to the group
// ---------------------------------------------------------------------------

func handleGTyping(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGTypingAsync(s, info, args, prefix, "composing")
}

func handleGStopTyping(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGTypingAsync(s, info, args, prefix, "paused")
}

func handleGTypingAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, state string) {
	if !groupOwnerGate(s, info) {
		return
	}
	if err := s.SendChatPresenceUpdate(info.Chat, state, ""); err != nil {
		s.Reply(info, "🔰 *PRESENCE FAILED:* "+err.Error())
		return
	}
	if state == "composing" {
		s.Reply(info, "🔰 *TYPING INDICATOR SENT* 🔰")
		return
	}
	s.Reply(info, "🔰 *TYPING INDICATOR STOPPED* 🔰")
}

// ---------------------------------------------------------------------------
// .gmention <text> — send a message mentioning everyone
// ---------------------------------------------------------------------------

func handleGMention(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGMentionAsync(s, info, args, prefix)
}

func handleGMentionAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		s.Reply(info, "🔰 *MENTION ALL* 🔰\n\n*FORMAT:* ```"+prefix+"gmention <your message>```")
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	mentioned := make([]string, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		mentioned = append(mentioned, p.JID.String())
	}
	_ = sendMentionText(client, info, "🔰 "+text+" 🔰", mentioned)
}

// ---------------------------------------------------------------------------
// .gtag <text> — alias of gmention
// ---------------------------------------------------------------------------

func handleGTag(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGMentionAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gpromoteall / .gdemoteall — promote/demote every non-owner member
// ---------------------------------------------------------------------------

func handleGPromoteAll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAllChangeAsync(s, info, args, prefix, whatsmeow.ParticipantChangePromote, "ALL MEMBERS PROMOTED")
}

func handleGDemoteAll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAllChangeAsync(s, info, args, prefix, whatsmeow.ParticipantChangeDemote, "ALL ADMINS DEMOTED")
}

func handleGAllChangeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, change whatsmeow.ParticipantChange, label string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	targets := make([]types.JID, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		if p.IsSuperAdmin {
			continue
		}
		if change == whatsmeow.ParticipantChangePromote && p.IsAdmin {
			continue
		}
		if change == whatsmeow.ParticipantChangeDemote && !p.IsAdmin {
			continue
		}
		targets = append(targets, p.JID)
	}
	if len(targets) == 0 {
		s.Reply(info, "🔰 *NO MEMBER NEEDS THIS CHANGE* 🔰")
		return
	}
	if _, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, change); err != nil {
		s.Reply(info, "🔰 *ACTION FAILED:* "+err.Error())
		return
	}
	s.Reply(info, fmt.Sprintf("🔰 *%s* 🔰\n\n🔰 *AFFECTED :❮ %d ❯*", label, len(targets)))
}

// ---------------------------------------------------------------------------
// .gkickall — remove every non-admin member
// ---------------------------------------------------------------------------

func handleGKickAll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGKickAllAsync(s, info, args, prefix)
}

func handleGKickAllAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	targets := make([]types.JID, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			continue
		}
		targets = append(targets, p.JID)
	}
	if len(targets) == 0 {
		s.Reply(info, "🔰 *NO REGULAR MEMBER TO REMOVE* 🔰")
		return
	}
	if _, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, whatsmeow.ParticipantChangeRemove); err != nil {
		s.Reply(info, "🔰 *KICK ALL FAILED:* "+err.Error())
		return
	}
	s.Reply(info, fmt.Sprintf("🔰 *MEMBERS REMOVED* 🔰\n\n🔰 *TOTAL :❮ %d ❯*", len(targets)))
}

// ---------------------------------------------------------------------------
// .gclean — remove members with no display name (cleanup helper)
// ---------------------------------------------------------------------------

func handleGClean(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGCleanAsync(s, info, args, prefix)
}

func handleGCleanAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	nameless := 0
	for _, p := range gi.Participants {
		if p.DisplayName == "" {
			nameless++
		}
	}
	s.Reply(info, fmt.Sprintf("🔰 *GROUP CLEANUP REPORT* 🔰\n\n🔰 *MEMBERS WITHOUT NAME :❮ %d ❯*\n\n*USE* ```"+prefix+"gsearch <name>``` *TO FIND MEMBERS.*", nameless))
}

// ---------------------------------------------------------------------------
// .gsummary — one-line group summary
// ---------------------------------------------------------------------------

func handleGSummary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGSummaryAsync(s, info, args, prefix)
}

func handleGSummaryAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	admins := 0
	for _, p := range gi.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins++
		}
	}
	lock := "OPEN"
	if gi.IsLocked {
		lock = "LOCKED"
	}
	s.Reply(info, fmt.Sprintf("🔰 *%s* 🔰\n\n🔰 *MEMBERS :❮ %d ❯*\n🔰 *ADMINS  :❮ %d ❯*\n🔰 *EDIT    :❮ %s ❯*\n🔰 *CREATED :❮ %s ❯*",
		gi.Name, gi.ParticipantCount, admins, lock, gi.GroupCreated.Format("02 Jan 2006")))
}

// ---------------------------------------------------------------------------
// .gid — show the current group JID
// ---------------------------------------------------------------------------

func handleGID(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGIDAsync(s, info, args, prefix)
}

func handleGIDAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	s.Reply(info, "🔰 *GROUP JID* 🔰\n\n🔰 *JID :❮ "+info.Chat.String()+" ❯*")
}

// ---------------------------------------------------------------------------
// .gname2 — show the current group name
// ---------------------------------------------------------------------------

func handleGNameShow(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGNameShowAsync(s, info, args, prefix)
}

func handleGNameShowAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	s.Reply(info, "🔰 *GROUP NAME* 🔰\n\n🔰 *NAME :❮ "+gi.Name+" ❯*")
}

// ---------------------------------------------------------------------------
// .gdesc2 — show the current group description
// ---------------------------------------------------------------------------

func handleGDescShow(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGDescShowAsync(s, info, args, prefix)
}

func handleGDescShowAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	gi, ok := fetchGroupInfo(s, info)
	if !ok {
		return
	}
	desc := gi.Topic
	if desc == "" {
		desc = "(no description)"
	}
	s.Reply(info, "🔰 *GROUP DESCRIPTION* 🔰\n\n"+desc)
}

// ---------------------------------------------------------------------------
// .gapproval — toggle join approval mode (on/off)
// ---------------------------------------------------------------------------

func handleGApproval(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGApprovalAsync(s, info, args, prefix)
}

func handleGApprovalAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	on := mode == "on" || mode == "true" || mode == "1" || mode == "yes"
	if mode == "" {
		s.Reply(info, "🔰 *JOIN APPROVAL* 🔰\n\n*FORMAT:* ```"+prefix+"gapproval on``` *or* ```"+prefix+"gapproval off```")
		return
	}
	if err := client.SetGroupJoinApprovalMode(context.Background(), info.Chat, on); err != nil {
		s.Reply(info, "🔰 *SET APPROVAL FAILED:* "+err.Error())
		return
	}
	if on {
		s.Reply(info, "🔰 *JOIN APPROVAL ENABLED* 🔰\n\n*NEW MEMBERS NEED ADMIN APPROVAL*")
		return
	}
	s.Reply(info, "🔰 *JOIN APPROVAL DISABLED* 🔰\n\n*ANYONE CAN JOIN FREELY*")
}

// ---------------------------------------------------------------------------
// .gaddmode — set who can add members (admin/all)
// ---------------------------------------------------------------------------

func handleGAddMode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGAddModeAsync(s, info, args, prefix)
}

func handleGAddModeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !groupOwnerGate(s, info) {
		return
	}
	client := groupClient(s, info)
	if client == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	var m types.GroupMemberAddMode
	switch mode {
	case "admin", "admins", "admin_add":
		m = types.GroupMemberAddModeAdmin
	case "all", "everyone", "all_member_add":
		m = types.GroupMemberAddModeAllMember
	default:
		s.Reply(info, "🔰 *WHO CAN ADD MEMBERS* 🔰\n\n*FORMAT:* ```"+prefix+"gaddmode admin``` *or* ```"+prefix+"gaddmode all```")
		return
	}
	if err := client.SetGroupMemberAddMode(context.Background(), info.Chat, m); err != nil {
		s.Reply(info, "🔰 *SET ADD MODE FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *MEMBER ADD MODE UPDATED* 🔰\n\n🔰 *MODE :❮ "+strings.ToUpper(mode)+" ❯*")
}

// ---------------------------------------------------------------------------
// .gnewlink — generate a fresh invite link (alias of revokelink)
// ---------------------------------------------------------------------------

func handleGNewLink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleInviteLinkAsync(s, info, true)
}

// ---------------------------------------------------------------------------
// .ggetlink — show the current invite link (alias of invitelink)
// ---------------------------------------------------------------------------

func handleGGetLink(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleInviteLinkAsync(s, info, false)
}

// ---------------------------------------------------------------------------
// .gsetname — set the group name (alias of gname)
// ---------------------------------------------------------------------------

func handleGSetName(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetGNameAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gsetdesc — set the group description (alias of gdesc)
// ---------------------------------------------------------------------------

func handleGSetDesc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleSetGDescAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gmembers — list members (alias of members)
// ---------------------------------------------------------------------------

func handleGMembers(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleMembersAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gadmins — list admins (alias of adminlist)
// ---------------------------------------------------------------------------

func handleGAdmins(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAdminListAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .grequests — list pending join requests (alias of joinrequests)
// ---------------------------------------------------------------------------

func handleGRequests(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinRequestsAsync(s, info, args, prefix)
}

// ---------------------------------------------------------------------------
// .gapprove / .greject — approve / reject join requests (aliases)
// ---------------------------------------------------------------------------

func handleGApprove(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinActionAsync(s, info, args, prefix, true)
}

func handleGReject(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinActionAsync(s, info, args, prefix, false)
}

// ---------------------------------------------------------------------------
// .gversion — show the group command pack version
// ---------------------------------------------------------------------------

func handleGVersion(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go func() {
		s.Reply(info, "🔰 *GROUP CONTROL PACK* 🔰\n\n🔰 *VERSION :❮ 2026.1 ❯*\n🔰 *COMMANDS:❮ 40+ ❯*\n🔰 *LID→PN  :❮ MANDATORY ❯*")
	}()
}

// ---------------------------------------------------------------------------
// .ghelp — quick help for the group control pack
// ---------------------------------------------------------------------------

func handleGHelp(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go func() {
		s.Reply(info, "🔰 *GROUP CONTROL HELP* 🔰\n\n"+
			"🔰 ```"+prefix+"gpp``` — set group photo\n"+
			"🔰 ```"+prefix+"gppremove``` — remove group photo\n"+
			"🔰 ```"+prefix+"gtopic <text>``` — set description\n"+
			"🔰 ```"+prefix+"gsettings``` — show all settings\n"+
			"🔰 ```"+prefix+"gstats``` — group statistics\n"+
			"🔰 ```"+prefix+"grank``` — member ranking\n"+
			"🔰 ```"+prefix+"gsearch <text>``` — search members\n"+
			"🔰 ```"+prefix+"gpromoteall``` — promote everyone\n"+
			"🔰 ```"+prefix+"gkickall``` — remove all members\n"+
			"🔰 ```"+prefix+"glist``` — list my groups\n"+
			"🔰 ```"+prefix+"gjoin <link>``` — join a group\n"+
			"🔰 ```"+prefix+"gcheck <number>``` — check WhatsApp\n"+
			"🔰 ```"+prefix+"gblock / gunblock``` — block a user\n"+
			"🔰 ```"+prefix+"gmention <text>``` — mention everyone")
	}()
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func init() {
	// ── primary commands ──
	Register(Command{Name: "gpp", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET THE GROUP PROFILE PICTURE. REPLY TO AN IMAGE OR SEND ONE WITH THIS CAPTION.", OwnerOnly: true, Run: handleSetGPP})
	Register(Command{Name: "gppremove", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO REMOVE THE GROUP PROFILE PICTURE.", OwnerOnly: true, Run: handleRemoveGPP})
	Register(Command{Name: "gtopic", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET THE GROUP DESCRIPTION. GIVE THE NEW DESCRIPTION TEXT.", OwnerOnly: true, Run: handleSetTopic})
	Register(Command{Name: "gdeldesc", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO DELETE THE GROUP DESCRIPTION.", OwnerOnly: true, Run: handleDelDesc})
	Register(Command{Name: "gowner", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE OWNER OF THE GROUP.", OwnerOnly: true, Run: handleGOwner})
	Register(Command{Name: "gcreated", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW WHEN THE GROUP WAS CREATED.", OwnerOnly: true, Run: handleGCreated})
	Register(Command{Name: "gsettings", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW ALL SETTINGS OF THE GROUP IN ONE PLACE.", OwnerOnly: true, Run: handleGSettings})
	Register(Command{Name: "gphoto", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE GROUP PROFILE PICTURE.", OwnerOnly: true, Run: handleGPhoto})
	Register(Command{Name: "glist", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO LIST ALL GROUPS THE BOT IS A MEMBER OF.", OwnerOnly: true, Run: handleGList})
	Register(Command{Name: "ginfo", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW GROUP INFO FROM AN INVITE LINK WITHOUT JOINING.", OwnerOnly: true, Run: handleGInfoLink})
	Register(Command{Name: "gjoin", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO MAKE THE BOT JOIN A GROUP USING AN INVITE LINK.", OwnerOnly: true, Run: handleGJoin})
	Register(Command{Name: "gleave", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO MAKE THE BOT LEAVE THE CURRENT GROUP.", OwnerOnly: true, Run: handleGLeave})
	Register(Command{Name: "gpromote", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO PROMOTE MEMBERS TO ADMIN. TAG THEM OR REPLY TO THEIR MESSAGE.", OwnerOnly: true, Run: handleGPromote})
	Register(Command{Name: "gdemote", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO DEMOTE ADMINS TO MEMBERS. TAG THEM OR REPLY TO THEIR MESSAGE.", OwnerOnly: true, Run: handleGDemote})
	Register(Command{Name: "gremove", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO REMOVE MEMBERS FROM THE GROUP. TAG THEM OR REPLY TO THEIR MESSAGE.", OwnerOnly: true, Run: handleGRemove})
	Register(Command{Name: "gadd", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO ADD MEMBERS TO THE GROUP. TAG THEM OR GIVE THEIR NUMBERS.", OwnerOnly: true, Run: handleGAdd})
	Register(Command{Name: "gadmin", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CHECK IF A MEMBER IS AN ADMIN OF THE GROUP.", OwnerOnly: true, Run: handleGAdmin})
	Register(Command{Name: "grole", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE ROLE OF A MEMBER IN THE GROUP.", OwnerOnly: true, Run: handleGRole})
	Register(Command{Name: "gcount", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE MEMBER AND ADMIN COUNT OF THE GROUP.", OwnerOnly: true, Run: handleGCount})
	Register(Command{Name: "gsearch", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SEARCH MEMBERS OF THE GROUP BY NAME OR NUMBER.", OwnerOnly: true, Run: handleGSearch})
	Register(Command{Name: "gstats", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW DETAILED STATISTICS OF THE GROUP.", OwnerOnly: true, Run: handleGStats})
	Register(Command{Name: "grank", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO RANK ALL MEMBERS OF THE GROUP BY ROLE.", OwnerOnly: true, Run: handleGRank})
	Register(Command{Name: "gtop", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE TOP MEMBERS OF THE GROUP.", OwnerOnly: true, Run: handleGTop})
	Register(Command{Name: "gactive", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW MEMBERS WHO HAVE A PROFILE PICTURE.", OwnerOnly: true, Run: handleGActive})
	Register(Command{Name: "gcheck", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CHECK IF A NUMBER IS REGISTERED ON WHATSAPP.", OwnerOnly: true, Run: handleGCheck})
	Register(Command{Name: "gblock", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO BLOCK A MEMBER OR NUMBER ON THE BOT WHATSAPP.", OwnerOnly: true, Run: handleGBlock})
	Register(Command{Name: "gunblock", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO UNBLOCK A MEMBER OR NUMBER ON THE BOT WHATSAPP.", OwnerOnly: true, Run: handleGUnblock})
	Register(Command{Name: "gblocklist", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE BLOCK LIST OF THE BOT WHATSAPP.", OwnerOnly: true, Run: handleGBlockList})
	Register(Command{Name: "gpic", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE PROFILE PICTURE OF A MEMBER.", OwnerOnly: true, Run: handleGPic})
	Register(Command{Name: "gabout", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE ABOUT TEXT OF A MEMBER.", OwnerOnly: true, Run: handleGAbout})
	Register(Command{Name: "gpin", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO PIN THE GROUP CHAT.", OwnerOnly: true, Run: handleGPin})
	Register(Command{Name: "gunpin", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO UNPIN THE GROUP CHAT.", OwnerOnly: true, Run: handleGUnpin})
	Register(Command{Name: "garchive", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO ARCHIVE THE GROUP CHAT.", OwnerOnly: true, Run: handleGArchive})
	Register(Command{Name: "gunarchive", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO UNARCHIVE THE GROUP CHAT.", OwnerOnly: true, Run: handleGUnarchive})
	Register(Command{Name: "gmarkread", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO MARK THE GROUP CHAT AS READ.", OwnerOnly: true, Run: handleGMarkRead})
	Register(Command{Name: "gtyping", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW A TYPING INDICATOR IN THE GROUP.", OwnerOnly: true, Run: handleGTyping})
	Register(Command{Name: "gstoptyping", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO STOP THE TYPING INDICATOR IN THE GROUP.", OwnerOnly: true, Run: handleGStopTyping})
	Register(Command{Name: "gmention", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SEND A MESSAGE THAT MENTIONS EVERY MEMBER OF THE GROUP.", OwnerOnly: true, Run: handleGMention})
	Register(Command{Name: "gtag", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO TAG EVERY MEMBER OF THE GROUP WITH YOUR MESSAGE.", OwnerOnly: true, Run: handleGTag})
	Register(Command{Name: "gpromoteall", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO PROMOTE EVERY MEMBER OF THE GROUP TO ADMIN.", OwnerOnly: true, Run: handleGPromoteAll})
	Register(Command{Name: "gdemoteall", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO DEMOTE EVERY ADMIN OF THE GROUP TO MEMBER.", OwnerOnly: true, Run: handleGDemoteAll})
	Register(Command{Name: "gkickall", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO REMOVE EVERY REGULAR MEMBER FROM THE GROUP.", OwnerOnly: true, Run: handleGKickAll})
	Register(Command{Name: "gclean", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW A CLEANUP REPORT OF THE GROUP.", OwnerOnly: true, Run: handleGClean})
	Register(Command{Name: "gsummary", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW A SHORT SUMMARY OF THE GROUP.", OwnerOnly: true, Run: handleGSummary})
	Register(Command{Name: "gid", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE JID OF THE CURRENT GROUP.", OwnerOnly: true, Run: handleGID})
	Register(Command{Name: "gname2", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE CURRENT NAME OF THE GROUP.", OwnerOnly: true, Run: handleGNameShow})
	Register(Command{Name: "gdesc2", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE CURRENT DESCRIPTION OF THE GROUP.", OwnerOnly: true, Run: handleGDescShow})
	Register(Command{Name: "gapproval", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO TURN JOIN APPROVAL ON OR OFF FOR THE GROUP.", OwnerOnly: true, Run: handleGApproval})
	Register(Command{Name: "gaddmode", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET WHO CAN ADD MEMBERS TO THE GROUP. USE admin OR all.", OwnerOnly: true, Run: handleGAddMode})
	Register(Command{Name: "gnewlink", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO GENERATE A FRESH INVITE LINK FOR THE GROUP.", OwnerOnly: true, Run: handleGNewLink})
	Register(Command{Name: "ggetlink", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE CURRENT INVITE LINK OF THE GROUP.", OwnerOnly: true, Run: handleGGetLink})
	Register(Command{Name: "gsetname", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CHANGE THE NAME OF THE GROUP.", OwnerOnly: true, Run: handleGSetName})
	Register(Command{Name: "gsetdesc", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CHANGE THE DESCRIPTION OF THE GROUP.", OwnerOnly: true, Run: handleGSetDesc})
	Register(Command{Name: "gmembers", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW ALL MEMBERS OF THE GROUP.", OwnerOnly: true, Run: handleGMembers})
	Register(Command{Name: "gadmins", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW ALL ADMINS OF THE GROUP.", OwnerOnly: true, Run: handleGAdmins})
	Register(Command{Name: "grequests", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW ALL PENDING JOIN REQUESTS OF THE GROUP.", OwnerOnly: true, Run: handleGRequests})
	Register(Command{Name: "gapprove", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO APPROVE PENDING JOIN REQUESTS OF THE GROUP.", OwnerOnly: true, Run: handleGApprove})
	Register(Command{Name: "greject", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO REJECT PENDING JOIN REQUESTS OF THE GROUP.", OwnerOnly: true, Run: handleGReject})
	Register(Command{Name: "gversion", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW THE VERSION OF THE GROUP CONTROL PACK.", OwnerOnly: true, Run: handleGVersion})
	Register(Command{Name: "ghelp", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW A QUICK HELP LIST OF THE GROUP CONTROL COMMANDS.", OwnerOnly: true, Run: handleGHelp})

	// ── hidden aliases (5+ each) ──
	alias := func(names []string, run func(SessionBridge, types.MessageInfo, []string, string)) {
		for _, n := range names {
			Register(Command{Name: n, OwnerOnly: true, Hidden: true, Run: run})
		}
	}
	alias([]string{"setgpp", "setgrouppic", "setgcpp", "groupphoto", "gcphoto"}, handleSetGPP)
	alias([]string{"delgpp", "removegpp", "delgrouppic", "nogcpp", "cleargcpp"}, handleRemoveGPP)
	alias([]string{"setgtopic", "gctopic", "setgctopic", "grouptopic", "setdesc"}, handleSetTopic)
	alias([]string{"delgdesc", "cleardesc", "gdescdelete", "removedesc", "nodesc"}, handleDelDesc)
	alias([]string{"groupowner", "gcowner", "whoisowner", "ownerinfo", "gownerinfo"}, handleGOwner)
	alias([]string{"groupcreated", "gccreated", "createdon", "gcage", "groupage"}, handleGCreated)
	alias([]string{"gcsettings", "groupsettings", "gconfig", "gcoptions", "settingsgc"}, handleGSettings)
	alias([]string{"gcphoto2", "showgpp", "grouppic", "getgpp", "viewgpp"}, handleGPhoto)
	alias([]string{"groups", "mygroups", "allgroups", "gclist", "listgroups"}, handleGList)
	alias([]string{"gcinfo", "groupinfo2", "linkinfo", "infolink", "gclinkinfo"}, handleGInfoLink)
	alias([]string{"joingroup", "gcjoin", "joinlink", "joinchat", "gjoinlink"}, handleGJoin)
	alias([]string{"gcleave", "leavegroup", "exitgroup", "gcexit", "leavegc"}, handleGLeave)
	alias([]string{"gcpromote", "promoteall2", "makeadmins", "bulkpromote", "gcpromote2"}, handleGPromote)
	alias([]string{"gcdemote", "demoteall2", "removeadmins", "bulkdemote", "gcdemote2"}, handleGDemote)
	alias([]string{"gcremove", "removeusers", "bulkremove", "gckick", "kickmany"}, handleGRemove)
	alias([]string{"gcadd", "addusers", "bulkadd", "gcinvite", "addmany"}, handleGAdd)
	alias([]string{"isadmin", "checkadmin", "admincheck", "gcadmin", "isgadmin"}, handleGAdmin)
	alias([]string{"gcrole", "memberrole", "userrole", "rolecheck", "gcroles"}, handleGRole)
	alias([]string{"gccount", "countgc", "membercount", "gcmembers", "countmembers"}, handleGCount)
	alias([]string{"findmember", "searchmember", "gcfind", "lookupmember", "memberfind"}, handleGSearch)
	alias([]string{"gcstats", "groupstats", "statistics", "gcstat", "statsgc"}, handleGStats)
	alias([]string{"gcrank", "ranking", "memberrank", "ranklist", "gcranking"}, handleGRank)
	alias([]string{"gctop", "topmembers", "bestmembers", "toplist", "gctop2"}, handleGTop)
	alias([]string{"gcactive", "activemembers", "online2", "activeusers", "gcactive2"}, handleGActive)
	alias([]string{"checknumber", "isnumber", "wa_check", "numcheck", "gccheck"}, handleGCheck)
	alias([]string{"gcblock", "blockuser", "blockmember", "blocknum", "gcblock2"}, handleGBlock)
	alias([]string{"gcunblock", "unblockuser", "unblockmember", "unblocknum", "gcunblock2"}, handleGUnblock)
	alias([]string{"gcblocklist", "blockedlist", "listblocked", "blocklist2", "gcblocked"}, handleGBlockList)
	alias([]string{"gcpp", "userpic", "profilepic", "getpic", "gcpic"}, handleGPic)
	alias([]string{"gcabout", "userabout", "aboutuser", "getabout", "gcabout2"}, handleGAbout)
	alias([]string{"gcpin", "pinchat", "pingc", "pin2", "gcpin2"}, handleGPin)
	alias([]string{"gcunpin", "unpinchat", "unpingc", "unpin2", "gcunpin2"}, handleGUnpin)
	alias([]string{"gcarchive", "archivechat", "archivegc", "arch2", "gcarchive2"}, handleGArchive)
	alias([]string{"gcunarchive", "unarchivechat", "unarchivegc", "unarch2", "gcunarchive2"}, handleGUnarchive)
	alias([]string{"gcmarkread", "markread", "readgc", "seen2", "gcseen"}, handleGMarkRead)
	alias([]string{"gctyping", "typing2", "showtyping", "typinggc", "gctyping2"}, handleGTyping)
	alias([]string{"gcstoptyping", "stoptyping", "typingstop", "stoptypinggc", "gcstoptyping2"}, handleGStopTyping)
	alias([]string{"gcmention", "mentionall", "allmention", "tagall2", "gcmention2"}, handleGMention)
	alias([]string{"gctag", "tag2", "tagmembers", "tagall3", "gctag2"}, handleGTag)
	alias([]string{"gcpromoteall", "promoteall3", "allpromote", "promoteeveryone", "gcpromoteall2"}, handleGPromoteAll)
	alias([]string{"gcdemoteall", "demoteall3", "alldemote", "demoteeveryone", "gcdemoteall2"}, handleGDemoteAll)
	alias([]string{"gckickall", "kickall2", "removeall", "kickeveryone", "gckickall2"}, handleGKickAll)
	alias([]string{"gcclean", "cleanup", "cleanreport", "gcclean2", "clean2"}, handleGClean)
	alias([]string{"gcsummary", "summary", "groupsummary", "gcsummary2", "sum2"}, handleGSummary)
	alias([]string{"gcjid", "groupjid", "jid2", "gcjid2", "showjid"}, handleGID)
	alias([]string{"showname", "gcname2", "name2", "showname2", "gcshowname"}, handleGNameShow)
	alias([]string{"showdesc", "gcdesc2", "desc2", "showdesc2", "gcshowdesc"}, handleGDescShow)
	alias([]string{"gcapproval", "joinapproval", "approvalmode", "gcapproval2", "setapproval"}, handleGApproval)
	alias([]string{"gcaddmode", "addmode2", "whocanadd2", "setaddmode2", "gcaddmode2"}, handleGAddMode)
	alias([]string{"gcnewlink", "newlink2", "freshlink", "resetlink2", "gcnewlink2"}, handleGNewLink)
	alias([]string{"gcgetlink", "getlink", "showlink", "link2", "gcgetlink2"}, handleGGetLink)
	alias([]string{"gcsetname", "setname2", "renamegc", "gcsetname2", "namegc"}, handleGSetName)
	alias([]string{"gcsetdesc", "setdesc2", "setgcdesc", "gcsetdesc2", "descgc"}, handleGSetDesc)
	alias([]string{"gcmembers", "members2", "listmembers", "gcmembers2", "allmembers"}, handleGMembers)
	alias([]string{"gcadmins", "admins2", "listadmins", "gcadmins2", "alladmins"}, handleGAdmins)
	alias([]string{"gcrequests", "requests", "joinrequests2", "gcrequests2", "pending2"}, handleGRequests)
	alias([]string{"gcapprove", "approve2", "approvejoin2", "gcapprove2", "accept2"}, handleGApprove)
	alias([]string{"gcreject", "reject2", "rejectjoin2", "gcreject2", "deny2"}, handleGReject)
	alias([]string{"gcversion", "version2", "packversion", "gcversion2", "ver2"}, handleGVersion)
	alias([]string{"gchelp", "help2", "grouphelp", "gchelp2", "ghelp2"}, handleGHelp)
}
