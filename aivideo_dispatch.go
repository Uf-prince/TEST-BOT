package main

import (
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"

	goldcmds "gold-md/gold-cmds"
)

// hasPendingAIVideoSession reports whether the given sender has an active
// aivideo multi-image collection session pending in the gold-cmds package.
func hasPendingAIVideoSession(sender string) bool {
	return goldcmds.HasPendingAIVideoSession(sender)
}

// dispatchAIVideoMedia is called by HandleMessage for every incoming message
// (including image messages) when a pending aivideo session exists for the
// sender. It builds a bridge, detects whether the message carries an image,
// extracts any text, and forwards both to the gold-cmds pending-session
// handler. Returns true if the message was consumed by a session.
func dispatchAIVideoMedia(s *Session, info types.MessageInfo, msg *waProto.Message) bool {
	b := &bridge{s: s}
	hasImg := extractImageMessage(msg) != nil
	text := strings.TrimSpace(getText(msg))
	// Also pick up caption text from image messages (getText ignores captions).
	if text == "" && msg != nil {
		if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ImageMessage.Caption)
		} else if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil &&
			msg.ViewOnceMessage.Message.ImageMessage != nil &&
			msg.ViewOnceMessage.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessage.Message.ImageMessage.Caption)
		} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessageV2.Message.ImageMessage.Caption)
		}
	}
	return goldcmds.DispatchAIVideoIncoming(b, info, text, hasImg)
}

// hasPendingAIVideo2Session reports whether the given sender has an active
// aivideo2 multi-image collection session pending in the gold-cmds package.
func hasPendingAIVideo2Session(sender string) bool {
	return goldcmds.HasPendingAIVideo2Session(sender)
}

// dispatchAIVideo2Media is the aivideo2 counterpart of dispatchAIVideoMedia.
// It is called by HandleMessage for every incoming message when a pending
// aivideo2 session exists for the sender. Returns true if consumed.
func dispatchAIVideo2Media(s *Session, info types.MessageInfo, msg *waProto.Message) bool {
	b := &bridge{s: s}
	hasImg := extractImageMessage(msg) != nil
	text := strings.TrimSpace(getText(msg))
	// Pick up caption text from image messages (getText ignores captions).
	if text == "" && msg != nil {
		if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ImageMessage.Caption)
		} else if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil &&
			msg.ViewOnceMessage.Message.ImageMessage != nil &&
			msg.ViewOnceMessage.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessage.Message.ImageMessage.Caption)
		} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage != nil &&
			msg.ViewOnceMessageV2.Message.ImageMessage.Caption != nil {
			text = strings.TrimSpace(*msg.ViewOnceMessageV2.Message.ImageMessage.Caption)
		}
	}
	return goldcmds.DispatchAIVideo2Incoming(b, info, text, hasImg)
}
