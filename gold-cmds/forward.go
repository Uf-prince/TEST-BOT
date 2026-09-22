package goldcmds

// ============================================================================
// GOLD-MD — FORWARD Command
// File: forward.go
// ----------------------------------------------------------------------------
// Owner-only broadcast forwarding command (no forward tag, raw resend).
//
//   .forward                → guide + totals (how many groups / chats)
//   .forward 3,6            → forward the REPLIED-TO message to the first
//                             3 chats and the first 6 groups — INSTANT
//   .forward 3,6 some text  → forward custom text directly (no reply needed)
//
// The raw proto of the quoted message is re-sent with every forwarding marker
// stripped (IsForwarded / ForwardingScore / ForwardedNewsletterMessageInfo),
// so destinations see a CLEAN message with NO "Forwarded" tag.
//
// All replies use the owner's exact message texts (English, 🔰 style).
// ============================================================================

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// ── destination collection ──────────────────────────────────────────────────

type fwdDest struct {
	jid  types.JID
	name string
}

// fwdCollectGroups returns all groups the bot is a member of (server order).
func fwdCollectGroups(cli *whatsmeow.Client) []fwdDest {
	var out []fwdDest
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	groups, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		return out
	}
	for _, g := range groups {
		name := g.Name
		if name == "" {
			name = g.JID.String()
		}
		out = append(out, fwdDest{jid: g.JID, name: name})
	}
	return out
}

// fwdCollectChats returns all DM chats the bot has (contact store — every
// real user the bot ever talked to), sorted alphabetically for stable picks.
func fwdCollectChats(cli *whatsmeow.Client) []fwdDest {
	var out []fwdDest
	if cli.Store == nil || cli.Store.Contacts == nil || cli.Store.ID == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	contacts, err := cli.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return out
	}
	for jid, ci := range contacts {
		if jid.Server != types.DefaultUserServer {
			continue // skip newsletters / channels / bots
		}
		if jid.User == cli.Store.ID.User {
			continue // skip the bot itself
		}
		name := ci.FullName
		if name == "" {
			name = ci.FirstName
		}
		if name == "" {
			name = ci.BusinessName
		}
		if name == "" {
			name = "+" + jid.User
		}
		out = append(out, fwdDest{jid: jid, name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// ── forward tag stripping (the "no forwarded label" core) ──────────────────

// fwdStripForwardMarkers removes every forwarded indicator from a raw proto
// message so the re-sent copy looks like a fresh, clean send.
func fwdStripForwardMarkers(msg *waProto.Message) *waProto.Message {
	if msg == nil {
		return nil
	}
	strip := func(ci *waProto.ContextInfo) {
		if ci == nil {
			return
		}
		ci.IsForwarded = proto.Bool(false)
		ci.ForwardingScore = proto.Uint32(0)
		ci.ForwardedNewsletterMessageInfo = nil
	}
	if msg.ExtendedTextMessage != nil {
		strip(msg.ExtendedTextMessage.ContextInfo)
	}
	if msg.ImageMessage != nil {
		strip(msg.ImageMessage.ContextInfo)
	}
	if msg.VideoMessage != nil {
		strip(msg.VideoMessage.ContextInfo)
	}
	if msg.AudioMessage != nil {
		strip(msg.AudioMessage.ContextInfo)
	}
	if msg.StickerMessage != nil {
		strip(msg.StickerMessage.ContextInfo)
	}
	if msg.DocumentMessage != nil {
		strip(msg.DocumentMessage.ContextInfo)
	}
	if msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil &&
		msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil {
		strip(msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo)
	}
	if msg.ViewOnceMessage != nil {
		fwdStripForwardMarkers(msg.ViewOnceMessage.Message)
	}
	if msg.ViewOnceMessageV2 != nil {
		fwdStripForwardMarkers(msg.ViewOnceMessageV2.Message)
	}
	if msg.EphemeralMessage != nil {
		fwdStripForwardMarkers(msg.EphemeralMessage.Message)
	}
	return msg
}

// ── payload resolution ──────────────────────────────────────────────────────

// fwdResolvePayload resolves the message to forward: the QUOTED message when
// the command message is a reply, else the free text typed after the numbers.
// Returns nil when there is nothing to send.
func fwdResolvePayload(s SessionBridge, info types.MessageInfo, freeText string) *waProto.Message {
	// 1) replied-to message → clean copy of its raw proto
	if qID, _, ok := s.GetQuotedMessageID(info); ok && qID != "" {
		raw := s.GetRawMessage(info)
		if raw != nil {
			var ci *waProto.ContextInfo
			if raw.ExtendedTextMessage != nil {
				ci = raw.ExtendedTextMessage.ContextInfo
			} else if raw.ImageMessage != nil {
				ci = raw.ImageMessage.ContextInfo
			} else if raw.VideoMessage != nil {
				ci = raw.VideoMessage.ContextInfo
			} else if raw.AudioMessage != nil {
				ci = raw.AudioMessage.ContextInfo
			} else if raw.StickerMessage != nil {
				ci = raw.StickerMessage.ContextInfo
			} else if raw.DocumentMessage != nil {
				ci = raw.DocumentMessage.ContextInfo
			}
			if ci != nil && ci.QuotedMessage != nil {
				return fwdStripForwardMarkers(ci.QuotedMessage)
			}
		}
	}
	// 2) free text typed after the numbers
	if strings.TrimSpace(freeText) != "" {
		return &waProto.Message{Conversation: proto.String(strings.TrimSpace(freeText))}
	}
	return nil
}

// ── exact reply texts (owner-specified, English) ────────────────────────────

func fwdGuideText(prefix string, groups, chats int) string {
	g := strconv.Itoa(groups)
	c := strconv.Itoa(chats)
	// example numbers fit INSIDE the real totals, so typing the example
	// as-is always works (no exceed-error loop for small accounts).
	exChats := fwdClampExample(3, chats)
	exGroups := fwdClampExample(6, groups)
	return "*🔰 FORWARD COMMAND GUIDE 🔰*\n\n" +
		"*WITH THIS COMMAND YOU CAN FORWARD YOUR MESSAGE WITHOUT GOING TO ANY GROUP / CHAT*\n\n" +
		"*ALL YOUR WHATSAPP GROUPS / CHATS HAVE BEEN COUNTED*\n\n" +
		"*🔰 TOTAL GROUPS : ❮ " + g + " ❯*\n" +
		"*🔰 TOTAL CHATS : ❮ " + c + " ❯*\n\n" +
		"*NOW TO FORWARD A MESSAGE TO HOWEVER MANY GROUPS / HOWEVER MANY CHATS YOU WANT, DO IT LIKE THIS*\n\n" +
		"*FIRST YOU MUST REPLY TO THE MESSAGE YOU WANT TO FORWARD OTHERWISE IT WON'T FORWARD*\n\n" +
		"*AFTER REPLYING TO THE MESSAGE WRITE IT LIKE THIS*\n\n" +
		"*" + strings.ToUpper(prefix) + "FORWARD CHATS-NUMBER,GROUP-NUMBER*\n\n" +
		"YOU HAVE TO WRITE THE NUMBER IN PLACE OF *\"CHATS\"* AND THE NUMBER IN PLACE OF *GROUP* OF HOW MANY CHATS HOW MANY GROUPS YOU WANT TO SEND THE MESSAGE TO OK\n\n" +
		"FOR EXAMPLE\n" +
		"*" + strings.ToUpper(prefix) + "FORWARD " + strconv.Itoa(exChats) + "," + strconv.Itoa(exGroups) + "*\n\n" +
		"FIRST YOU WILL WRITE THE NUMBER OF *\"CHATS\"* THEN THE NUMBER OF *\"GROUPS\"* OK THEN WHEN YOU WRITE IT LIKE THIS THE MESSAGE WILL BE FORWARDED TO THAT MANY *\"CHATS\"* AND THAT MANY *\"GROUPS\"* *INSTANT*"
}

func fwdSuccessText(groups, chats int) string {
	return "*YOUR MESSAGE FORWARDED TO*\n" +
		"*❮ " + strconv.Itoa(groups) + " ❯ GROUP/S*\n" +
		"*❮ " + strconv.Itoa(chats) + " ❯ CHATS*"
}

func fwdNoMentionText(prefix string, query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		q = strings.ToUpper(prefix) + "FORWARD 6,7"
	}
	return "*YOU HAVEN'T MENTIONED ANY MESSAGE*\n\n" +
		"*FIRST MENTION THE MESSAGE AND TYPE*\n" +
		q
}

// fwdClampExample clamps a default example number to the user's real total.
// If the total is 0 (collection failed / empty account) the default is kept.
func fwdClampExample(def, total int) int {
	if total > 0 && total < def {
		return total
	}
	return def
}

func fwdWrongCmdText(prefix string, groups, chats int) string {
	// example numbers fit INSIDE the user's real totals, so the owner can
	// type the example line as-is and it works (no exceed-error loop).
	exChats := fwdClampExample(6, chats)
	exGroups1 := fwdClampExample(8, groups)
	exGroups2 := fwdClampExample(7, groups)
	return "*YOU HAVE TYPED WRONG COMMAND*\n\n" +
		"*TYPE SAME LIKE THAT*\n" +
		"*" + strings.ToUpper(prefix) + "FORWARD " + strconv.Itoa(exChats) + "," + strconv.Itoa(exGroups1) + "*\n\n" +
		"*" + strings.ToUpper(prefix) + "FORWARD " + strconv.Itoa(exChats) + "," + strconv.Itoa(exGroups2) + " YOUR MESSAGE*\n\n" +
		"*BY TYPING IT LIKE THIS YOUR MESSAGE WILL BE DIRECTLY FORWARDED*"
}

func fwdExceedText(prefix string, groups, chats int) string {
	// example numbers must fit INSIDE the user's real totals:
	// the owner wants the sample line to be typed-as-is and work,
	// so clamp each number to the actual available count.
	exChats := fwdClampExample(6, chats)
	exGroups := fwdClampExample(8, groups)
	return "*YOUR TOTAL GROUPS ❮ " + strconv.Itoa(groups) + " ❯*\n" +
		"*YOUR TOTAL CHATS ❮ " + strconv.Itoa(chats) + " ❯*\n\n" +
		"*YOUR WHATSAPP ONLY HAS THIS MANY GROUPS AND CHATS. YOU HAVE TYPED MORE THAN THAT. PLEASE TYPE THEM CORRECTLY ACCORDING TO THESE CHATS AND GROUPS*\n\n" +
		"*TYPE SAME LIKE THAT*\n" +
		"*" + strings.ToUpper(prefix) + "FORWARD " + strconv.Itoa(exChats) + "," + strconv.Itoa(exGroups) + "*"
}

// ── command handler ─────────────────────────────────────────────────────────

func handleForward(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only gate
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "*❌ Bot is not connected right now!*")
		return
	}

	rawArg := strings.TrimSpace(strings.Join(args, " "))

	// MODE 1 — plain .forward → guide + totals
	if rawArg == "" {
		groups := fwdCollectGroups(cli)
		chats := fwdCollectChats(cli)
		s.Reply(info, fwdGuideText(prefix, len(groups), len(chats)))
		return
	}

	// MODE 2 — .forward N,M [text] → instant forward
	// collect the user's real totals ONCE — every error example below
	// (wrong-command / exceed) uses numbers clamped to these totals, so
	// typing the shown example always works instead of looping errors.
	groups := fwdCollectGroups(cli)
	chats := fwdCollectChats(cli)

	// strict shape: "N,M" followed by optional free text
	first := rawArg
	rest := ""
	if idx := strings.IndexAny(rawArg, " \t"); idx >= 0 {
		first = rawArg[:idx]
		rest = strings.TrimSpace(rawArg[idx+1:])
	}
	// strip braces style like 5{chats},7{group} before validation
	first = strings.ReplaceAll(first, "{", "")
	first = strings.ReplaceAll(first, "}", "")
	parts := strings.Split(first, ",")
	if len(parts) != 2 {
		s.Reply(info, fwdWrongCmdText(prefix, len(groups), len(chats)))
		return
	}
	chatN, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	groupN, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || chatN < 0 || groupN < 0 || (chatN == 0 && groupN == 0) {
		s.Reply(info, fwdWrongCmdText(prefix, len(groups), len(chats)))
		return
	}

	// resolve the payload FIRST (quoted message, then free text).
	// OWNER ORDER: if nothing is mentioned, the NO-MENTION error must be
	// the first reply — even if the numbers exceed the real totals.
	payload := fwdResolvePayload(s, info, rest)
	if payload == nil {
		s.Reply(info, fwdNoMentionText(prefix, strings.TrimSpace(prefix+"forward "+rawArg)))
		return
	}

	// check limits (totals already collected at MODE 2 entry)
	if chatN > len(chats) || groupN > len(groups) {
		s.Reply(info, fwdExceedText(prefix, len(groups), len(chats)))
		return
	}

	// build destination list — first N chats + first M groups
	var dests []fwdDest
	if chatN > 0 && chatN <= len(chats) {
		dests = append(dests, chats[:chatN]...)
	}
	if groupN > 0 && groupN <= len(groups) {
		dests = append(dests, groups[:groupN]...)
	}
	if len(dests) == 0 {
		s.Reply(info, fwdWrongCmdText(prefix, len(groups), len(chats)))
		return
	}

	// send to every destination — clean, no forward tag
	sentChats, sentGroups, failed := 0, 0, 0
	for _, d := range dests {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := cli.SendMessage(ctx, d.jid, payload)
		cancel()
		if err != nil {
			failed++
			continue
		}
		if d.jid.Server == types.GroupServer {
			sentGroups++
		} else {
			sentChats++
		}
	}
	if failed > 0 && sentChats == 0 && sentGroups == 0 {
		s.Reply(info, "*❌ FORWARD FAILED! TRY AGAIN IN A MOMENT*")
		return
	}

	s.Reply(info, fwdSuccessText(sentGroups, sentChats))
}

func init() {
	Register(Command{
		Name:      "forward",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO FORWARD ANY MESSAGE TO MANY CHATS AND GROUPS AT ONCE WITHOUT A FORWARD TAG. REPLY TO A MESSAGE THEN USE .forward 3,6 TO SEND IT TO 3 CHATS AND 6 GROUPS INSTANTLY.",
		OwnerOnly: true,
		Run:       handleForward,
	})
}
