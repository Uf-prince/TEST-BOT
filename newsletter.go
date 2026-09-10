package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// GOLD-MD — Newsletter channel system
//
// Mirrors the behaviour of pair.js's newsletter auto-follow + forwarded
// newsletter link button, but simplified to a SINGLE channel (the user
// asked to drop pair.js's 2-channel toggle and use just one).
//
//   • On every successful connect we try to FOLLOW the channel (only if the
//     bot is currently a GUEST = not yet following), with in-memory caching
//     so we never spam the follow request and risk a ban.
//   • The 4 core commands (.menu .ping .uptime .alive) attach a
//     "forwarded from channel" link button to their messages via
//     newsletterCtxInfo().  No other message gets this button.
// ============================================================================

// NewsletterChannelInviteKey is the invite key of the SINGLE channel the bot
// follows and advertises.  Taken from
// https://whatsapp.com/channel/0029Vb956DuHFxP4EhLJA316
const NewsletterChannelInviteKey = "0029Vb956DuHFxP4EhLJA316"

// NewsletterChannelName is the human-readable name shown on the forwarded
// link button.  It is refreshed from the live newsletter metadata when the
// bot resolves the invite, but we keep a sensible default here so the button
// works even before the first successful metadata fetch.
const NewsletterChannelName = "GOLD-MD Updates"

// newsletterState caches the resolved channel JID + name and whether we have
// already attempted a follow for this process.  This avoids hammering
// WhatsApp's newsletter API on every connect/reconnect (ban safety, exactly
// like pair.js's RAM cache).
var newsletterState = struct {
	sync.Mutex
	jid      string // resolved channel JID (e.g. 123@newsletter)
	name     string // live channel name
	followed bool   // true once we have successfully followed (or confirmed already following)
}{
	name: NewsletterChannelName,
}

// followNewsletterChannel resolves the channel invite key to a JID and follows
// the channel if the bot is not already a subscriber/admin.  Safe to call on
// every connect — the in-memory cache ensures we only act once per process.
func (s *Session) followNewsletterChannel() {
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Resolve invite key → newsletter JID + metadata.
	// GetNewsletterInfoWithInvite accepts the raw invite key.
	meta, err := s.Client.GetNewsletterInfoWithInvite(ctx, NewsletterChannelInviteKey)
	if err != nil {
		ErrLog("[newsletter] failed to resolve invite %s: %v", NewsletterChannelInviteKey, err)
		return
	}
	if meta == nil || meta.ID.IsEmpty() {
		ErrLog("[newsletter] resolved metadata empty for invite %s", NewsletterChannelInviteKey)
		return
	}

	channelJID := meta.ID.String()
	channelName := meta.ThreadMeta.Name.Text
	if channelName == "" {
		channelName = NewsletterChannelName
	}

	// Cache the resolved JID + name.
	newsletterState.Lock()
	newsletterState.jid = channelJID
	newsletterState.name = channelName
	alreadyFollowed := newsletterState.followed
	newsletterState.Unlock()

	OkLog("[newsletter] resolved channel %s (%s)", channelName, channelJID)

	if alreadyFollowed {
		// Already followed (or already confirmed following) this process.
		return
	}

	// GetNewsletterInfoWithInvite returns ViewerMeta=nil (per whatsmeow docs),
	// so to learn the viewer role we fetch the full info by JID.
	fullMeta, err := s.Client.GetNewsletterInfo(ctx, meta.ID)
	if err == nil && fullMeta != nil && fullMeta.ViewerMeta != nil {
		role := fullMeta.ViewerMeta.Role
		if role == types.NewsletterRoleSubscriber || role == types.NewsletterRoleAdmin || role == types.NewsletterRoleOwner {
			OkLog("[newsletter] already following channel (%s) — role %s", channelName, role)
			newsletterState.Lock()
			newsletterState.followed = true
			newsletterState.Unlock()
			return
		}
	}
	// If role lookup failed or role is GUEST, attempt to follow.

	if err := s.Client.FollowNewsletter(ctx, meta.ID); err != nil {
		ErrLog("[newsletter] failed to follow channel %s: %v", channelName, err)
		return
	}

	newsletterState.Lock()
	newsletterState.followed = true
	newsletterState.Unlock()
	OkLog("[newsletter] 🔰 followed channel %s (%s)", channelName, channelJID)
}

// newsletterCtxInfo builds a *waProto.ContextInfo that, when attached to a
// message, renders WhatsApp's "forwarded from <channel>" link button.
//
// This is exactly what pair.js does with forwardingScore=999999 +
// isForwarded=true + forwardedNewsletterMessageInfo{newsletterJid,
// newsletterName, serverMessageId}.
//
// IMPORTANT: only the 4 core commands (.menu .ping .uptime .alive) use this.
func (s *Session) newsletterCtxInfo() *waProto.ContextInfo {
	newsletterState.Lock()
	jid := newsletterState.jid
	name := newsletterState.name
	newsletterState.Unlock()

	if jid == "" {
		// Channel not resolved yet — still attach the button using the known
		// invite-derived info so the button renders.  The JID may be empty on
		// the very first menu/ping before connect resolves the channel; in
		// that case we skip the button (no JID = no valid link).
		return nil
	}

	score := uint32(999999)
	serverID := int32(rand.Intn(900000) + 100000)
	contentType := waProto.ForwardedNewsletterMessageInfo_LINK_CARD
	accessText := fmt.Sprintf("View updates from %s", name)

	return &waProto.ContextInfo{
		ForwardingScore: &score,
		IsForwarded:     proto.Bool(true),
		ForwardedNewsletterMessageInfo: &waProto.ForwardedNewsletterMessageInfo{
			NewsletterJID:     &jid,
			NewsletterName:    &name,
			ServerMessageID:   &serverID,
			ContentType:       &contentType,
			AccessibilityText: &accessText,
		},
	}
}

// ReplyWithNewsletter sends a text reply with the forwarded newsletter channel
// link button attached.  Used ONLY by .menu .ping .uptime .alive.
//
// The botname footer is applied here (via withFooter) so that EVERY message
// routed through this helper — including .ping, .uptime and .alive — carries
// the consistent GOLD-MD bot signature, exactly like messages sent through
// the normal Reply() path. The footer is sanitized so the marker/control
// chars can never leak.
func (s *Session) ReplyWithNewsletter(info types.MessageInfo, text string) {
	if s.Client == nil || !s.Client.IsConnected() {
		return
	}

	text = s.withFooter(text)

	ctxInfo := s.newsletterCtxInfo()

	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ctxInfo,
		},
	}

	// If the channel hasn't been resolved yet, fall back to a plain
	// conversation message so the command still replies.
	if ctxInfo == nil {
		msg = &waProto.Message{Conversation: proto.String(text)}
	}

	if _, err := s.Client.SendMessage(context.Background(), info.Chat, msg); err != nil {
		ErrLog("[%s] newsletter reply failed: %v", s.JID, err)
	}
}

// ReplyImageWithNewsletter sends an image message with caption + the forwarded
// newsletter channel link button attached.  Used ONLY by .menu (the redesigned
// menu sends a header image with a fancy caption) and .alive (bot pic + alive
// msg). The botname footer is appended to the caption here (sanitized) so the
// marker/control chars can never leak and every image reply carries the
// consistent bot signature.
func (s *Session) ReplyImageWithNewsletter(info types.MessageInfo, imgData []byte, caption string) bool {
	if s.Client == nil || !s.Client.IsConnected() {
		return false
	}

	caption = s.withCaptionFooter(caption)

	uploaded, err := s.Client.Upload(context.Background(), imgData, whatsmeow.MediaImage)
	if err != nil {
		ErrLog("[%s] menu image upload failed: %v", s.JID, err)
		return false
	}

	ctxInfo := s.newsletterCtxInfo()

	imgMsg := &waProto.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String("image/png"),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(imgData))),
		ContextInfo:   ctxInfo,
	}

	if _, err := s.Client.SendMessage(context.Background(), info.Chat, &waProto.Message{
		ImageMessage: imgMsg,
	}); err != nil {
		ErrLog("[%s] menu image send failed: %v", s.JID, err)
		return false
	}
	return true
}

// wakeNewsletterFollow kicks off the channel follow in the background with a
// small random delay (mirrors pair.js's staggered follow to look human and
// avoid ban risk).  Called from EventHandler's *events.Connected case.
func (s *Session) wakeNewsletterFollow() {
	go func() {
		// small stagger so we don't follow the instant we connect
		time.Sleep(time.Duration(10+rand.Intn(20)) * time.Second)
		s.followNewsletterChannel()
	}()
}
