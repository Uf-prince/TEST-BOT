package main

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// newCtxTestBridge builds a bridge with a cached proto message so the quoted
// helpers can resolve ContextInfo without a live client.
func newCtxTestBridge(t *testing.T, id string, msg *waE2E.Message) *bridge {
	t.Helper()
	s := &Session{
		JID:     "15551230000@s.whatsapp.net",
		Manager: &Manager{cfg: &Config{DataDir: t.TempDir()}},
	}
	s.cacheMessage(id, msg)
	return &bridge{s: s}
}

// strPtr is a tiny *string helper.
func strPtr(s string) *string { return &s }

// A reply to a PTV / video-note must resolve the quoted stanza id even though
// the ContextInfo lives on PtvMessage, not ExtendedTextMessage. This is the
// exact case that used to make ".circle" think there was no quoted media.
func TestQuotedIDFromPtvMessage(t *testing.T) {
	inner := &waE2E.VideoMessage{
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:    strPtr("QUOTED-PTV-ID"),
			Participant: strPtr("923001234567@s.whatsapp.net"),
		},
	}
	msg := &waE2E.Message{PtvMessage: inner}
	b := newCtxTestBridge(t, "incoming-ptv", msg)

	id, sender, ok := b.GetQuotedMessageID(types.MessageInfo{ID: "incoming-ptv"})
	if !ok {
		t.Fatal("GetQuotedMessageID = false for a PTV reply, want true")
	}
	if id != "QUOTED-PTV-ID" {
		t.Errorf("id = %q, want QUOTED-PTV-ID", id)
	}
	if sender != "923001234567@s.whatsapp.net" {
		t.Errorf("sender = %q, want the participant", sender)
	}
}

// A reply to a normal video (VideoMessage.ContextInfo) must also resolve.
func TestQuotedIDFromVideoMessage(t *testing.T) {
	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		ContextInfo: &waE2E.ContextInfo{StanzaID: strPtr("QUOTED-VID-ID")},
	}}
	b := newCtxTestBridge(t, "incoming-vid", msg)
	id, _, ok := b.GetQuotedMessageID(types.MessageInfo{ID: "incoming-vid"})
	if !ok || id != "QUOTED-VID-ID" {
		t.Fatalf("GetQuotedMessageID = %q,%v; want QUOTED-VID-ID,true", id, ok)
	}
}

// extractMediaMessage must return the quoted PTV's inner VideoMessage so the
// circle downloader can fetch a video-note replied to by the user.
func TestExtractMediaFromQuotedPtv(t *testing.T) {
	quoted := &waE2E.Message{PtvMessage: &waE2E.VideoMessage{Mimetype: strPtr("video/mp4")}}
	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		ContextInfo: &waE2E.ContextInfo{QuotedMessage: quoted},
	}}
	d, mime, ok := extractMediaMessage(msg)
	if !ok || d == nil {
		t.Fatal("extractMediaMessage found no media in a quoted PTV")
	}
	if mime != "video/mp4" {
		t.Errorf("mime = %q, want video/mp4", mime)
	}
	if _, isVid := d.(*waE2E.VideoMessage); !isVid {
		t.Errorf("downloadable type = %T, want *waE2E.VideoMessage", d)
	}
}

// extractContextInfoFromMsg must dig ContextInfo out of view-once wrappers.
func TestContextInfoFromViewOnce(t *testing.T) {
	inner := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		ContextInfo: &waE2E.ContextInfo{StanzaID: strPtr("VO-ID")},
	}}
	msg := &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: inner}}
	ci := extractContextInfoFromMsg(msg)
	if ci == nil || ci.GetStanzaID() != "VO-ID" {
		t.Fatalf("view-once ContextInfo not extracted: %+v", ci)
	}
}

// Sanity: no reply => no quoted id.
func TestQuotedIDAbsentForBareVideo(t *testing.T) {
	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}
	b := newCtxTestBridge(t, "bare", msg)
	if _, _, ok := b.GetQuotedMessageID(types.MessageInfo{ID: "bare"}); ok {
		t.Error("bare video reported a quoted id")
	}
}
