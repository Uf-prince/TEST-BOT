package goldcmds

// ============================================================================
// GOLD-MD — NEW WhatsApp Group Commands (2026 feature set)
// File: groupnew.go
// ----------------------------------------------------------------------------
// These commands cover the NEW WhatsApp group features rolled out in 2026:
//   • @all mention (admins only in groups > 32 members)  → .tagall already
//     exists; here we add the modern poll + join-request + group-creation set.
//   • Polls with END TIME and HIDE VOTER NAMES            → .poll / .vote
//   • Create a new group from scratch                     → .newgroup
//   • Add a member directly                               → .addmember
//   • Approve / reject pending join requests              → .approve / .reject
//   • List pending join requests                          → .joinrequests
//   • Who can add members (admin only / all members)      → .memberaddmode
//
// DESIGN (same as the rest of GOLD-MD):
//   • Owner-only, group-only, styled with 🔰, Category "GROUP MANAGEMENT".
//   • Hidden aliases (5+ each) so the menu stays clean but every spelling works.
//   • All heavy work runs in a background goroutine (0% speed impact).
//   • No admin check — the bot attempts the action directly and reports the
//     WhatsApp error if it lacks permission (owner order, same as group.go).
//
// whatsmeow APIs used:
//   Client.BuildPollCreation / BuildPollVote
//   Client.CreateGroup(ctx, ReqCreateGroup)
//   Client.UpdateGroupParticipants(ctx, jid, []JID, ParticipantChangeAdd)
//   Client.GetGroupRequestParticipants(ctx, jid)
//   Client.UpdateGroupRequestParticipants(ctx, jid, []JID, approve|reject)
//   Client.SetGroupMemberAddMode(ctx, jid, GroupMemberAddMode)
// ============================================================================

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func init() {
	// ── primary commands ──
	Register(Command{Name: "poll", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CREATE A POLL IN THE GROUP. USE | TO SEPARATE THE QUESTION AND OPTIONS. YOU CAN ALSO SET AN END TIME AND HIDE VOTER NAMES.", OwnerOnly: true, Run: handlePoll})
	Register(Command{Name: "vote", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO VOTE IN A POLL. REPLY TO THE POLL WITH THE OPTION NUMBER OR THE OPTION TEXT.", OwnerOnly: true, Run: handleVote})
	Register(Command{Name: "newgroup", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO CREATE A NEW GROUP. GIVE A NAME AND TAG THE MEMBERS TO ADD.", OwnerOnly: true, Run: handleNewGroup})
	Register(Command{Name: "addmember", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO ADD A MEMBER TO THE GROUP. GIVE THE NUMBER OR TAG THE MEMBER.", OwnerOnly: true, Run: handleAddMember})
	Register(Command{Name: "approve", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO APPROVE PENDING JOIN REQUESTS OF THE GROUP. TAG THE MEMBERS OR USE ALL.", OwnerOnly: true, Run: handleApprove})
	Register(Command{Name: "reject", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO REJECT PENDING JOIN REQUESTS OF THE GROUP. TAG THE MEMBERS OR USE ALL.", OwnerOnly: true, Run: handleReject})
	Register(Command{Name: "joinrequests", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SHOW ALL PENDING JOIN REQUESTS OF THE GROUP.", OwnerOnly: true, Run: handleJoinRequests})
	Register(Command{Name: "memberaddmode", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO SET WHO CAN ADD MEMBERS TO THE GROUP. USE admin OR all.", OwnerOnly: true, Run: handleMemberAddMode})

	// ── poll aliases (5+) ──
	Register(Command{Name: "createpoll", OwnerOnly: true, Hidden: true, Run: handlePoll})
	Register(Command{Name: "newpoll", OwnerOnly: true, Hidden: true, Run: handlePoll})
	Register(Command{Name: "makepoll", OwnerOnly: true, Hidden: true, Run: handlePoll})
	Register(Command{Name: "pollcreate", OwnerOnly: true, Hidden: true, Run: handlePoll})
	Register(Command{Name: "startpoll", OwnerOnly: true, Hidden: true, Run: handlePoll})

	// ── vote aliases (5+) ──
	Register(Command{Name: "pollvote", OwnerOnly: true, Hidden: true, Run: handleVote})
	Register(Command{Name: "castvote", OwnerOnly: true, Hidden: true, Run: handleVote})
	Register(Command{Name: "votepoll", OwnerOnly: true, Hidden: true, Run: handleVote})
	Register(Command{Name: "pollchoice", OwnerOnly: true, Hidden: true, Run: handleVote})
	Register(Command{Name: "choose", OwnerOnly: true, Hidden: true, Run: handleVote})

	// ── newgroup aliases (5+) ──
	Register(Command{Name: "creategroup", OwnerOnly: true, Hidden: true, Run: handleNewGroup})
	Register(Command{Name: "makegroup", OwnerOnly: true, Hidden: true, Run: handleNewGroup})
	Register(Command{Name: "newgc", OwnerOnly: true, Hidden: true, Run: handleNewGroup})
	Register(Command{Name: "creategc", OwnerOnly: true, Hidden: true, Run: handleNewGroup})
	Register(Command{Name: "gcnew", OwnerOnly: true, Hidden: true, Run: handleNewGroup})

	// ── addmember aliases (5+) ──
	Register(Command{Name: "adduser", OwnerOnly: true, Hidden: true, Run: handleAddMember})
	Register(Command{Name: "addto", OwnerOnly: true, Hidden: true, Run: handleAddMember})
	Register(Command{Name: "invitemember", OwnerOnly: true, Hidden: true, Run: handleAddMember})
	Register(Command{Name: "addparticipant", OwnerOnly: true, Hidden: true, Run: handleAddMember})
	Register(Command{Name: "gcadd", OwnerOnly: true, Hidden: true, Run: handleAddMember})

	// ── approve aliases (5+) ──
	Register(Command{Name: "accept", OwnerOnly: true, Hidden: true, Run: handleApprove})
	Register(Command{Name: "approvejoin", OwnerOnly: true, Hidden: true, Run: handleApprove})
	Register(Command{Name: "acceptjoin", OwnerOnly: true, Hidden: true, Run: handleApprove})
	Register(Command{Name: "approveall", OwnerOnly: true, Hidden: true, Run: handleApprove})
	Register(Command{Name: "letjoin", OwnerOnly: true, Hidden: true, Run: handleApprove})

	// ── reject aliases (5+) ──
	Register(Command{Name: "deny", OwnerOnly: true, Hidden: true, Run: handleReject})
	Register(Command{Name: "rejectjoin", OwnerOnly: true, Hidden: true, Run: handleReject})
	Register(Command{Name: "denyjoin", OwnerOnly: true, Hidden: true, Run: handleReject})
	Register(Command{Name: "rejectall", OwnerOnly: true, Hidden: true, Run: handleReject})
	Register(Command{Name: "blockjoin", OwnerOnly: true, Hidden: true, Run: handleReject})

	// ── joinrequests aliases (5+) ──
	Register(Command{Name: "pendingjoins", OwnerOnly: true, Hidden: true, Run: handleJoinRequests})
	Register(Command{Name: "joinlist", OwnerOnly: true, Hidden: true, Run: handleJoinRequests})
	Register(Command{Name: "requestlist", OwnerOnly: true, Hidden: true, Run: handleJoinRequests})
	Register(Command{Name: "pendingrequests", OwnerOnly: true, Hidden: true, Run: handleJoinRequests})
	Register(Command{Name: "joinreq", OwnerOnly: true, Hidden: true, Run: handleJoinRequests})

	// ── memberaddmode aliases (5+) ──
	Register(Command{Name: "addmode", OwnerOnly: true, Hidden: true, Run: handleMemberAddMode})
	Register(Command{Name: "setaddmode", OwnerOnly: true, Hidden: true, Run: handleMemberAddMode})
	Register(Command{Name: "whocanadd", OwnerOnly: true, Hidden: true, Run: handleMemberAddMode})
	Register(Command{Name: "addpermission", OwnerOnly: true, Hidden: true, Run: handleMemberAddMode})
	Register(Command{Name: "memberadd", OwnerOnly: true, Hidden: true, Run: handleMemberAddMode})
}

// ---------------------------------------------------------------------------
// .poll  — create a poll (with optional end time + hide voter names)
// ---------------------------------------------------------------------------
//
// Usage:
//   .poll Question | Option 1 | Option 2 | Option 3
//   .poll Question | Option 1 | Option 2 --time 1h
//   .poll Question | Option 1 | Option 2 --hide
//   .poll Question | Option 1 | Option 2 --time 30m --hide
//
// Flags:
//   --time <dur>   set an end time (5m / 1h30m / 00h30m00s)
//   --hide         hide voter names
//   --multi        allow selecting more than one option

func handlePoll(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handlePollAsync(s, info, args, prefix)
}

func handlePollAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	raw := strings.TrimSpace(strings.Join(args, " "))
	if raw == "" {
		s.Reply(info, pollUsage(prefix))
		return
	}

	// ── extract flags ──
	hide := false
	multi := false
	var endTime time.Duration
	tokens := strings.Fields(raw)
	for i := 0; i < len(tokens); i++ {
		t := strings.ToLower(tokens[i])
		switch {
		case t == "--hide" || t == "-hide":
			hide = true
		case t == "--multi" || t == "-multi":
			multi = true
		case (t == "--time" || t == "-time") && i+1 < len(tokens):
			if d, ok := parseTimedAdminDuration(tokens[i+1]); ok {
				endTime = d
				i++
			}
		case strings.HasPrefix(t, "--time="):
			if d, ok := parseTimedAdminDuration(strings.TrimPrefix(t, "--time=")); ok {
				endTime = d
			}
		}
	}
	// rebuild the body without the flags (keep the | separators)
	body := raw
	for _, f := range []string{"--hide", "-hide", "--multi", "-multi"} {
		body = strings.ReplaceAll(body, f, "")
	}
	// strip --time <dur> / --time=<dur>
	if endTime > 0 {
		body = stripTimeFlag(body)
	}

	parts := strings.Split(body, "|")
	if len(parts) < 3 {
		s.Reply(info, "🔰 *POLL NEEDS A QUESTION AND AT LEAST 2 OPTIONS* 🔰\n\n*Example:* ```"+prefix+"poll Best fruit? | Apple | Mango | Banana```")
		return
	}
	question := strings.TrimSpace(parts[0])
	options := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		o := strings.TrimSpace(p)
		if o != "" {
			options = append(options, o)
		}
	}
	if question == "" || len(options) < 2 {
		s.Reply(info, "🔰 *POLL NEEDS A QUESTION AND AT LEAST 2 OPTIONS* 🔰\n\n*Example:* ```"+prefix+"poll Best fruit? | Apple | Mango | Banana```")
		return
	}
	if len(options) > 12 {
		options = options[:12]
	}

	selectable := 1
	if multi {
		selectable = len(options)
	}

	// ── build the poll message (supports end time + hide voter names) ──
	poll := &waE2E.PollCreationMessage{
		Name:                   proto.String(question),
		SelectableOptionsCount: proto.Uint32(uint32(selectable)),
	}
	for _, o := range options {
		poll.Options = append(poll.Options, &waE2E.PollCreationMessage_Option{OptionName: proto.String(o)})
	}
	if hide {
		poll.HideParticipantName = proto.Bool(true)
	}
	if endTime > 0 {
		poll.EndTime = proto.Int64(time.Now().Add(endTime).Unix())
	}
	msg := &waE2E.Message{
		PollCreationMessageV3: poll,
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: random.Bytes(32),
		},
	}
	if _, err := client.SendMessage(context.Background(), info.Chat, msg); err != nil {
		s.Reply(info, "🔰 *POLL FAILED:* "+err.Error())
		return
	}

	// ── styled confirmation ──
	var b strings.Builder
	b.WriteString("🔰 *POLL CREATED* 🔰\n\n")
	fmt.Fprintf(&b, "🔰 *QUESTION :❮ %s ❯*\n", question)
	fmt.Fprintf(&b, "🔰 *OPTIONS  :❮ %d ❯*\n", len(options))
	if endTime > 0 {
		fmt.Fprintf(&b, "🔰 *ENDS IN  :❮ %s ❯*\n", formatHMS(endTime))
	}
	if hide {
		b.WriteString("🔰 *VOTER NAMES :❮ HIDDEN ❯*\n")
	}
	if multi {
		b.WriteString("🔰 *MULTI SELECT :❮ ON ❯*\n")
	}
	b.WriteString("\n*VOTE WITH:* ```" + prefix + "vote <number>``` *(reply to the poll)*")
	s.Reply(info, b.String())
}

// stripTimeFlag removes "--time <dur>" / "--time=<dur>" from a poll body.
func stripTimeFlag(body string) string {
	tokens := strings.Fields(body)
	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		t := strings.ToLower(tokens[i])
		if t == "--time" || t == "-time" {
			i++ // skip the duration token too
			continue
		}
		if strings.HasPrefix(t, "--time=") || strings.HasPrefix(t, "-time=") {
			continue
		}
		out = append(out, tokens[i])
	}
	return strings.Join(out, " ")
}

// ---------------------------------------------------------------------------
// .vote  — vote in a poll (reply to the poll)
// ---------------------------------------------------------------------------

func handleVote(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleVoteAsync(s, info, args, prefix)
}

func handleVoteAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	if len(args) == 0 {
		s.Reply(info, "🔰 *VOTE IN A POLL* 🔰\n\n*REPLY TO THE POLL WITH:* ```"+prefix+"vote <number>```\n*Example:* ```"+prefix+"vote 2```")
		return
	}

	// need the quoted poll message
	quotedID, quotedSender, ok := getQuotedID(s, info)
	if !ok || quotedID == "" {
		s.Reply(info, "🔰 *REPLY TO A POLL MESSAGE TO VOTE* 🔰\n\n*Example:* ```"+prefix+"vote 2``` *(as a reply to the poll)*")
		return
	}

	// read the poll options from the quoted message
	options := quotedPollOptions(s, info)
	if len(options) == 0 {
		s.Reply(info, "🔰 *THE MESSAGE YOU REPLIED TO IS NOT A POLL* 🔰")
		return
	}

	// resolve the choice: number or exact text
	choice := strings.TrimSpace(strings.Join(args, " "))
	var optionName string
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(options) {
		optionName = options[n-1]
	} else {
		for _, o := range options {
			if strings.EqualFold(strings.TrimSpace(o), choice) {
				optionName = o
				break
			}
		}
	}
	if optionName == "" {
		var b strings.Builder
		b.WriteString("🔰 *INVALID OPTION* 🔰\n\n")
		for i, o := range options {
			fmt.Fprintf(&b, "🔰 *%d.* %s\n", i+1, o)
		}
		b.WriteString("\n*Example:* ```" + prefix + "vote 1```")
		s.Reply(info, b.String())
		return
	}

	// build the poll info the vote must reference
	senderJID, _ := types.ParseJID(quotedSender)
	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     info.Chat,
			Sender:   senderJID,
			IsGroup:  info.IsGroup,
			IsFromMe: false,
		},
		ID: types.MessageID(quotedID),
	}
	voteMsg, err := client.BuildPollVote(context.Background(), pollInfo, []string{optionName})
	if err != nil {
		s.Reply(info, "🔰 *VOTE FAILED:* "+err.Error())
		return
	}
	if _, err := client.SendMessage(context.Background(), info.Chat, voteMsg); err != nil {
		s.Reply(info, "🔰 *VOTE FAILED:* "+err.Error())
		return
	}
	s.Reply(info, "🔰 *VOTE CAST* 🔰\n\n🔰 *OPTION :❮ "+optionName+" ❯*")
}

// getQuotedID is a small wrapper around the optional quotedGetter interface.
func getQuotedID(s SessionBridge, info types.MessageInfo) (string, string, bool) {
	type quotedGetter interface {
		GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	}
	if q, ok := s.(quotedGetter); ok {
		return q.GetQuotedMessageID(info)
	}
	return "", "", false
}

// quotedPollOptions extracts the poll option names from the quoted message of
// the incoming reply. Returns nil if the quoted message is not a poll.
func quotedPollOptions(s SessionBridge, info types.MessageInfo) []string {
	raw := s.GetRawMessage(info)
	if raw == nil {
		return nil
	}
	var ci *waE2E.ContextInfo
	if raw.ExtendedTextMessage != nil {
		ci = raw.ExtendedTextMessage.ContextInfo
	}
	if ci == nil || ci.QuotedMessage == nil {
		return nil
	}
	q := ci.QuotedMessage
	var poll *waE2E.PollCreationMessage
	switch {
	case q.PollCreationMessageV3 != nil:
		poll = q.PollCreationMessageV3
	case q.PollCreationMessageV2 != nil:
		poll = q.PollCreationMessageV2
	case q.PollCreationMessage != nil:
		poll = q.PollCreationMessage
	}
	if poll == nil {
		return nil
	}
	out := make([]string, 0, len(poll.Options))
	for _, o := range poll.Options {
		out = append(out, o.GetOptionName())
	}
	return out
}

// ---------------------------------------------------------------------------
// .newgroup  — create a new group
// ---------------------------------------------------------------------------

func handleNewGroup(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleNewGroupAsync(s, info, args, prefix)
}

func handleNewGroupAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	if len(args) == 0 {
		s.Reply(info, "🔰 *CREATE A NEW GROUP* 🔰\n\n*FORMAT:* ```"+prefix+"newgroup <name> @member1 @member2```\n*Example:* ```"+prefix+"newgroup My Friends @user1 @user2```")
		return
	}

	// name = args up to the first @mention / number token
	nameParts := make([]string, 0)
	for _, a := range args {
		if strings.HasPrefix(a, "@") || isPhoneToken(a) {
			break
		}
		nameParts = append(nameParts, a)
	}
	name := strings.TrimSpace(strings.Join(nameParts, " "))
	if name == "" {
		s.Reply(info, "🔰 *GIVE A NAME FOR THE NEW GROUP* 🔰\n\n*Example:* ```"+prefix+"newgroup My Friends @user1 @user2```")
		return
	}
	if len(name) > 25 {
		name = name[:25]
	}

	// participants = mentions + phone numbers
	participants := collectTargets(s, info, args)
	if len(participants) == 0 {
		s.Reply(info, "🔰 *TAG AT LEAST ONE MEMBER TO ADD TO THE NEW GROUP* 🔰\n\n*Example:* ```"+prefix+"newgroup My Friends @user1 @user2```")
		return
	}

	gi, err := client.CreateGroup(context.Background(), whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participants,
	})
	if err != nil {
		s.Reply(info, "🔰 *CREATE GROUP FAILED:* "+err.Error())
		return
	}
	s.Reply(info, fmt.Sprintf(
		"🔰 *NEW GROUP CREATED* 🔰\n\n"+
			"🔰 *NAME    :❮ %s ❯*\n"+
			"🔰 *MEMBERS :❮ %d ❯*\n"+
			"🔰 *JID     :❮ %s ❯*",
		name, len(participants)+1, gi.JID.String(),
	))
}

// ---------------------------------------------------------------------------
// .addmember  — add a member to the current group
// ---------------------------------------------------------------------------

func handleAddMember(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAddMemberAsync(s, info, args, prefix)
}

func handleAddMemberAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	targets := collectTargets(s, info, args)
	if len(targets) == 0 {
		s.Reply(info, "🔰 *TAG THE MEMBER OR GIVE THE NUMBER TO ADD* 🔰\n\n*Example:* ```"+prefix+"addmember @user```\n*Example:* ```"+prefix+"addmember 923001234567```")
		return
	}
	added, err := client.UpdateGroupParticipants(context.Background(), info.Chat, targets, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		s.Reply(info, "🔰 *ADD MEMBER FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("🔰 *MEMBER(S) ADDED* 🔰\n\n")
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		mentioned = append(mentioned, t.String())
		fmt.Fprintf(&b, "🔰 @%s\n", t.User)
	}
	if len(added) == 0 {
		b.WriteString("\n*NOTE: WhatsApp may require an invite for privacy-restricted users.*")
	}
	_ = sendMentionText(client, info, b.String(), mentioned)
}

// ---------------------------------------------------------------------------
// .approve / .reject  — handle pending join requests
// ---------------------------------------------------------------------------

func handleApprove(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinActionAsync(s, info, args, prefix, true)
}

func handleReject(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinActionAsync(s, info, args, prefix, false)
}

func handleJoinActionAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, approve bool) {
	word := "APPROVE"
	if !approve {
		word = "REJECT"
	}
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}

	pending, err := client.GetGroupRequestParticipants(context.Background(), info.Chat)
	if err != nil {
		s.Reply(info, "🔰 *COULD NOT FETCH JOIN REQUESTS:* "+err.Error())
		return
	}
	if len(pending) == 0 {
		s.Reply(info, "🔰 *NO PENDING JOIN REQUESTS* 🔰")
		return
	}

	// "all" → every pending request; otherwise the mentioned / numbered ones
	all := false
	for _, a := range args {
		if strings.EqualFold(strings.TrimSpace(a), "all") {
			all = true
			break
		}
	}

	var targets []types.JID
	if all || len(args) == 0 {
		for _, p := range pending {
			targets = append(targets, p.JID)
		}
	} else {
		want := collectTargets(s, info, args)
		for _, w := range want {
			for _, p := range pending {
				if sameUser(p.JID, w) {
					targets = append(targets, p.JID)
					break
				}
			}
		}
	}
	if len(targets) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "🔰 *PENDING JOIN REQUESTS (%d)* 🔰\n\n", len(pending))
		for i, p := range pending {
			fmt.Fprintf(&b, "🔰 *%d.* @%s\n", i+1, p.JID.User)
		}
		fmt.Fprintf(&b, "\n*USE:* ```%s%s all``` *or tag the members*", prefix, strings.ToLower(word))
		s.Reply(info, b.String())
		return
	}

	action := whatsmeow.ParticipantChangeApprove
	if !approve {
		action = whatsmeow.ParticipantChangeReject
	}
	if _, err := client.UpdateGroupRequestParticipants(context.Background(), info.Chat, targets, action); err != nil {
		s.Reply(info, "🔰 *"+word+" FAILED:* "+err.Error())
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *JOIN REQUESTS %sD* 🔰\n\n", word)
	mentioned := make([]string, 0, len(targets))
	for _, t := range targets {
		mentioned = append(mentioned, t.String())
		fmt.Fprintf(&b, "🔰 @%s\n", t.User)
	}
	_ = sendMentionText(client, info, b.String(), mentioned)
}

// ---------------------------------------------------------------------------
// .joinrequests  — list pending join requests
// ---------------------------------------------------------------------------

func handleJoinRequests(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleJoinRequestsAsync(s, info, args, prefix)
}

func handleJoinRequestsAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	pending, err := client.GetGroupRequestParticipants(context.Background(), info.Chat)
	if err != nil {
		s.Reply(info, "🔰 *COULD NOT FETCH JOIN REQUESTS:* "+err.Error())
		return
	}
	if len(pending) == 0 {
		s.Reply(info, "🔰 *NO PENDING JOIN REQUESTS* 🔰")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔰 *PENDING JOIN REQUESTS (%d)* 🔰\n\n", len(pending))
	mentioned := make([]string, 0, len(pending))
	for i, p := range pending {
		mentioned = append(mentioned, p.JID.String())
		fmt.Fprintf(&b, "🔰 *%d.* @%s\n", i+1, p.JID.User)
	}
	fmt.Fprintf(&b, "\n*APPROVE:* ```%sapprove all```\n*REJECT:* ```%sreject all```", prefix, prefix)
	_ = sendMentionText(client, info, b.String(), mentioned)
}

// ---------------------------------------------------------------------------
// .memberaddmode  — who can add members (admin only / all members)
// ---------------------------------------------------------------------------

func handleMemberAddMode(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleMemberAddModeAsync(s, info, args, prefix)
}

func handleMemberAddModeAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	if !requireGroup(s, info) {
		return
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	if len(args) == 0 {
		s.Reply(info, "🔰 *MEMBER ADD MODE* 🔰\n\n*SET WHO CAN ADD MEMBERS:*\n🔰 ```"+prefix+"memberaddmode admin``` *(only admins)*\n🔰 ```"+prefix+"memberaddmode all``` *(all members)*")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(args[0]))
	var m types.GroupMemberAddMode
	switch mode {
	case "admin", "admins", "admin_add", "adminadd":
		m = types.GroupMemberAddModeAdmin
	case "all", "everyone", "all_member_add", "allmember":
		m = types.GroupMemberAddModeAllMember
	default:
		s.Reply(info, "🔰 *INVALID MODE* 🔰\n\n*USE:* ```"+prefix+"memberaddmode admin``` *or* ```"+prefix+"memberaddmode all```")
		return
	}
	if err := client.SetGroupMemberAddMode(context.Background(), info.Chat, m); err != nil {
		s.Reply(info, "🔰 *SET ADD MODE FAILED:* "+err.Error())
		return
	}
	label := "ONLY ADMINS"
	if m == types.GroupMemberAddModeAllMember {
		label = "ALL MEMBERS"
	}
	s.Reply(info, "🔰 *MEMBER ADD MODE UPDATED* 🔰\n\n🔰 *WHO CAN ADD :❮ "+label+" ❯*")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// collectTargets gathers JIDs from @mentions, replied-to sender, and bare
// phone-number tokens (e.g. 923001234567).
func collectTargets(s SessionBridge, info types.MessageInfo, args []string) []types.JID {
	out := make([]types.JID, 0)
	seen := map[string]bool{}
	add := func(j types.JID) {
		if j.IsEmpty() {
			return
		}
		k := j.ToNonAD().String()
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, j.ToNonAD())
	}

	// mentions
	if mentioned, ok := s.GetMentionedJIDs(info); ok {
		for _, m := range mentioned {
			if j, err := types.ParseJID(m); err == nil {
				add(j)
			}
		}
	}
	// replied-to sender
	if id, sender, ok := getQuotedID(s, info); ok && id != "" && sender != "" {
		if j, err := types.ParseJID(sender); err == nil {
			add(j)
		}
	}
	// bare phone numbers
	for _, a := range args {
		if isPhoneToken(a) {
			add(types.NewJID(a, types.DefaultUserServer))
		}
	}
	return out
}

// isPhoneToken reports whether a token looks like a bare phone number
// (7-15 digits, optional leading +).
func isPhoneToken(tok string) bool {
	t := strings.TrimSpace(tok)
	t = strings.TrimPrefix(t, "+")
	if len(t) < 7 || len(t) > 15 {
		return false
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// sameUser compares two JIDs by their user part (ignores device/server).
func sameUser(a, b types.JID) bool {
	return a.User != "" && a.User == b.User
}

// pollUsage returns the styled .poll help text.
func pollUsage(prefix string) string {
	return "🔰 *GOLD-MD POLL* 🔰\n\n" +
		"*CREATE A POLL IN THE GROUP* 🔰\n\n" +
		"╭─ 🔰 *FORMAT* ─╮\n" +
		"│ ```" + prefix + "poll Question | Option 1 | Option 2```\n" +
		"╰──────────────╯\n\n" +
		"*EXAMPLES:*\n" +
		"🔰 ```" + prefix + "poll Best fruit? | Apple | Mango | Banana```\n\n" +
		"*OPTIONAL FLAGS:*\n" +
		"🔰 ```--time 1h```   → *poll ends after 1 hour*\n" +
		"🔰 ```--hide```      → *hide voter names*\n" +
		"🔰 ```--multi```     → *allow multiple choices*\n\n" +
		"*FULL EXAMPLE:*\n" +
		"🔰 ```" + prefix + "poll Best fruit? | Apple | Mango | Banana --time 30m --hide```\n\n" +
		"*VOTE WITH:* ```" + prefix + "vote <number>``` *(reply to the poll)*"
}
