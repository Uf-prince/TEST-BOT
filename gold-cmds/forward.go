package goldcmds

// ============================================================================
// GOLD-MD — FORWARD Command
// File: forward.go
// ----------------------------------------------------------------------------
// Owner-only broadcast forwarding command (no forward tag, raw resend).
//
//   .forward                    → stats: how many groups + chats the bot has
//   .forward chat,group         → numbered list of chats and groups
//   .forward 5,7                → instantly forward to chat #5 and group #7
//                                 (numbers taken from the last list shown)
//
// Forwarding works on the REPLIED-TO message (quote any message and run the
// command) — the raw proto is re-sent with all forwarding markers stripped
// (IsForwarded / ForwardingScore / ForwardedNewsletterMessageInfo), so the
// destination chat sees a CLEAN message with NO "Forwarded" tag.
//
// Style: bot design — bold text, 🔰 emoji, English help text.
// ============================================================================

import (
	"context"

	"google.golang.org/protobuf/proto"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// ── forward session: per-JID memory of the last numbered list ───────────────

type forwardListEntry struct {
	kind string // "chat" or "group"
	jid  types.JID
	name string
}

type forwardSessionData struct {
	chats  []forwardListEntry
	groups []forwardListEntry
}

var (
	forwardSessions   = map[string]*forwardSessionData{}
	forwardSessionsMu chan struct{} = make(chan struct{}, 1)
)

func forwardLock() {
	forwardSessionsMu <- struct{}{}
}
func forwardUnlock() {
	<-forwardSessionsMu
}

// fwdHead is the styled command header used on every .forward reply.
func fwdHead() string {
	return "*🔰 GOLD-MD FORWARD 🔰*\n\n"
}

// fwdGetClient safely returns the live whatsmeow client.
func fwdGetClient(s SessionBridge) *whatsmeow.Client {
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		return nil
	}
	return cli
}

// ── chat / group collection ─────────────────────────────────────────────────

// fwdCollectGroups returns all groups the bot is a member of (JID + name).
func fwdCollectGroups(cli *whatsmeow.Client) []forwardListEntry {
	var out []forwardListEntry
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	groups, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		return out
	}
	for _, g := range groups {
		name := g.Name
		if name == "" {
			name = "Group " + g.JID.String()
		}
		out = append(out, forwardListEntry{kind: "group", jid: g.JID, name: name})
	}
	// alphabetical for stable numbering
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// fwdCollectChats returns all DM chats the bot has (from the contact store:
// sqlite whatsmeow_contacts — every user the bot ever talked to).
func fwdCollectChats(cli *whatsmeow.Client) []forwardListEntry {
	var out []forwardListEntry
	if cli.Store == nil || cli.Store.Contacts == nil {
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
			continue // skip newsletters / bots / channels
		}
		if jid.User == cli.Store.ID.User {
			continue // skip bot itself
		}
		name := ci.FullName
		if name == "" {
			name = ci.FirstName
		}
		if name == "" {
			name = ci.BusinessName
		}
		if name == "" {
			name = "+" + jid.User // phone number fallback
		}
		out = append(out, forwardListEntry{kind: "chat", jid: jid, name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// ── forward tag stripping (the core "no forwarded label" logic) ────────────

// fwdStripForwardMarkers removes every forwarded indicator from a raw proto
// message so the re-sent copy looks like a fresh, clean send:
//   - ContextInfo.IsForwarded            → false
//   - ContextInfo.ForwardingScore        → 0
//   - ContextInfo.ForwardedNewsletterMessageInfo → nil
// Works recursively through nested wrappers (viewOnce, ephemeral, document
// with caption).
func fwdStripForwardMarkers(msg *waProto.Message) *waProto.Message {
	if msg == nil {
		return nil
	}
	fwdStripCI := func(ci *waProto.ContextInfo) {
		if ci == nil {
			return
		}
		ci.IsForwarded = proto.Bool(false)
		ci.ForwardingScore = proto.Uint32(0)
		ci.ForwardedNewsletterMessageInfo = nil
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
		fwdStripCI(msg.ExtendedTextMessage.ContextInfo)
	}
	if msg.ImageMessage != nil && msg.ImageMessage.ContextInfo != nil {
		fwdStripCI(msg.ImageMessage.ContextInfo)
	}
	if msg.VideoMessage != nil && msg.VideoMessage.ContextInfo != nil {
		fwdStripCI(msg.VideoMessage.ContextInfo)
	}
	if msg.AudioMessage != nil && msg.AudioMessage.ContextInfo != nil {
		fwdStripCI(msg.AudioMessage.ContextInfo)
	}
	if msg.StickerMessage != nil && msg.StickerMessage.ContextInfo != nil {
		fwdStripCI(msg.StickerMessage.ContextInfo)
	}
	if msg.DocumentMessage != nil && msg.DocumentMessage.ContextInfo != nil {
		fwdStripCI(msg.DocumentMessage.ContextInfo)
	}
	if msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil &&
		msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil &&
		msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo != nil {
		fwdStripCI(msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo)
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

// fwdResolveTargetProto resolves the message to forward: the QUOTED message
// if the command message is a reply, else the command text itself as a plain
// text message. Returns nil when there is nothing to send.
func fwdResolveTargetProto(s SessionBridge, info types.MessageInfo, args []string) *waProto.Message {
	// 1) quoted message → use its raw proto (clean copy)
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
			}
			if ci != nil && ci.QuotedMessage != nil {
				return fwdStripForwardMarkers(ci.QuotedMessage)
			}
		}
	}
	// 2) fall back to the command text after the numbers
	if len(args) > 0 {
		text := strings.TrimSpace(strings.Join(args, " "))
		if text != "" {
			return &waProto.Message{Conversation: proto.String(text)}
		}
	}
	return nil
}

// ── command handler ─────────────────────────────────────────────────────────

func handleForward(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only (same gate as other owner commands)
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	cli := fwdGetClient(s)
	if cli == nil {
		s.Reply(info, fwdHead()+"*❌ Bot is not connected right now!*")
		return
	}

	// MODE 1 — plain .forward → stats only
	rawArg := strings.TrimSpace(strings.Join(args, " "))
	if rawArg == "" {
		groups := fwdCollectGroups(cli)
		chats := fwdCollectChats(cli)
		forwardLock()
		forwardSessions[info.Sender.String()] = &forwardSessionData{chats: chats, groups: groups}
		forwardUnlock()
		s.Reply(info, fwdHead()+
			"*📊 Your total groups: "+strconv.Itoa(len(groups))+"*\n"+
			"*💬 Your total chats: "+strconv.Itoa(len(chats))+"*\n\n"+
			"*📖 Usage:*\n"+
			"*❰ "+prefix+"forward chat,group ❱* — show numbered list\n"+
			"*❰ "+prefix+"forward 5,7 ❱* — forward to chat #5 and group #7 (reply to any message first)\n\n"+
			"*🔰 GOLD-MD WHATSAPP BOT 🔰*")
		return
	}

	// MODE 2 — .forward chat,group → numbered list
	lower := strings.ToLower(rawArg)
	if strings.Contains(lower, "chat") || strings.Contains(lower, "group") {
		groups := fwdCollectGroups(cli)
		chats := fwdCollectChats(cli)
		forwardLock()
		forwardSessions[info.Sender.String()] = &forwardSessionData{chats: chats, groups: groups}
		forwardUnlock()

		var b strings.Builder
		b.WriteString(fwdHead())
		b.WriteString("*💬 PRIVATE CHATS (" + strconv.Itoa(len(chats)) + "):*\n")
		if len(chats) == 0 {
			b.WriteString("_none_\n")
		}
		for i, c := range chats {
			if i >= 100 {
				b.WriteString("…\n")
				break
			}
			b.WriteString("*❰ " + strconv.Itoa(i+1) + " ❱* " + c.name + "\n")
		}
		b.WriteString("\n*👥 GROUPS (" + strconv.Itoa(len(groups)) + "):*\n")
		if len(groups) == 0 {
			b.WriteString("_none_\n")
		}
		for i, g := range groups {
			if i >= 100 {
				b.WriteString("…\n")
				break
			}
			b.WriteString("*❰ " + strconv.Itoa(i+1) + " ❱* " + g.name + "\n")
		}
		b.WriteString("\n*💡 Now reply to any message and type ❰ " + prefix + "forward 5,7 ❱ to forward it (no forward tag!)*\n\n*🔰 GOLD-MD WHATSAPP BOT 🔰*")
		s.Reply(info, b.String())
		return
	}

	// MODE 3 — .forward 5,7 → instant forward
	forwardLock()
	sess := forwardSessions[info.Sender.String()]
	forwardUnlock()
	if sess == nil {
		s.Reply(info, fwdHead()+
			"*❌ No list loaded yet!*\n\n"+
			"*📖 First type ❰ "+prefix+"forward chat,group ❱ to load the numbered list, then pick numbers.*\n\n"+
			"*🔰 GOLD-MD WHATSAPP BOT 🔰*")
		return
	}

	// parse numbers (split on comma / space)
	numRe := regexp.MustCompile(`\d+`)
	var chatNums, groupNums []int
	// preserve position: chats first set, then groups set — user types
	// ".forward 5{chats},7{group}" style; simplest robust parse: each token
	// maps in order — first number(s) before comma = chats, after = groups.
	tokens := strings.FieldsFunc(rawArg, func(r rune) bool {
		return r == ',' || r == ' ' || r == '+' || r == '{' || r == '}' || r == ':'
	})
	parts := strings.Split(rawArg, ",")
	if len(parts) == 2 {
		// "5,7" — chats part, groups part
		for _, t := range numRe.FindAllString(parts[0], -1) {
			if n, err := strconv.Atoi(t); err == nil {
				chatNums = append(chatNums, n)
			}
		}
		for _, t := range numRe.FindAllString(parts[1], -1) {
			if n, err := strconv.Atoi(t); err == nil {
				groupNums = append(groupNums, n)
			}
		}
	} else {
		// single token list — apply all numbers to groups (most common use)
		for _, t := range numRe.FindAllString(rawArg, -1) {
			if n, err := strconv.Atoi(t); err == nil {
				groupNums = append(groupNums, n)
			}
		}
	}
	_ = tokens

	// resolve the payload (quoted message or text after the numbers)
	payload := fwdResolveTargetProto(s, info, args)
	if payload == nil {
		s.Reply(info, fwdHead()+
			"*❌ Nothing to forward!*\n\n"+
			"*💡 Reply to any message and then type ❰ "+prefix+"forward 5,7 ❱*\n"+
			"*💡 Or add text: ❰ "+prefix+"forward 5,7 your message here ❱*\n\n"+
			"*🔰 GOLD-MD WHATSAPP BOT 🔰*")
		return
	}

	// build the destination list
	var dests []forwardListEntry
	for _, n := range chatNums {
		if n >= 1 && n <= len(sess.chats) {
			dests = append(dests, sess.chats[n-1])
		}
	}
	for _, n := range groupNums {
		if n >= 1 && n <= len(sess.groups) {
			dests = append(dests, sess.groups[n-1])
		}
	}
	if len(dests) == 0 {
		s.Reply(info, fwdHead()+
			"*❌ No valid destinations selected!*\n\n"+
			"*💡 Type ❰ "+prefix+"forward chat,group ❱ to see the list first.*\n\n"+
			"*🔰 GOLD-MD WHATSAPP BOT 🔰*")
		return
	}

	// send to each destination — clean, no forward tag
	var sent, failed int
	for _, d := range dests {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := cli.SendMessage(ctx, d.jid, payload)
		cancel()
		if err != nil {
			failed++
		} else {
			sent++
		}
	}

	s.Reply(info, fwdHead()+
		"*✅ Forward complete!*\n"+
		"*📤 Delivered: "+strconv.Itoa(sent)+" chat(s)*\n"+
		"*❌ Failed: "+strconv.Itoa(failed)+" chat(s)*\n\n"+
		"*🛡️ Sent clean — no forward tag!*\n\n"+
		"*🔰 GOLD-MD WHATSAPP BOT 🔰*")
}

func init() {
	Register(Command{
		Name:      "forward",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO FORWARD ANY MESSAGE TO SELECTED CHATS AND GROUPS WITHOUT A FORWARD TAG. REPLY TO A MESSAGE AND USE .forward 5,7 TO SEND IT CLEAN.",
		OwnerOnly: true,
		Run:       handleForward,
	})
}

// keep unused imports referenced (fmt / events compiled out on some builds)
var _ = fmt.Sprintf
var _ events.Message
