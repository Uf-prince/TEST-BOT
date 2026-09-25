package main

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// recorder captures the send/edit calls sendAssetTextEdited makes, in order.
type recorder struct {
	calls    []string
	sendText string
	editID   string
	editText string
	sendID   string
}

func (r *recorder) ReplyWithID(_ types.MessageInfo, text string) string {
	r.calls = append(r.calls, "send")
	r.sendText = text
	return r.sendID
}

func (r *recorder) EditMessage(_ types.MessageInfo, id string, newText string) bool {
	r.calls = append(r.calls, "edit")
	r.editID, r.editText = id, newText
	return true
}

// The saved text must be sent and then EDITED to that very same text — the
// autoreply "edited" look. Order matters: edit must reference the sent ID.
func TestSendAssetTextEditedEditsSameText(t *testing.T) {
	r := &recorder{sendID: "MSG123"}
	id := sendAssetTextEdited(r, types.MessageInfo{}, "Umar bhai kaisa hai", 0)
	if id != "MSG123" {
		t.Fatalf("returned id = %q, want MSG123", id)
	}
	if len(r.calls) != 2 || r.calls[0] != "send" || r.calls[1] != "edit" {
		t.Fatalf("calls = %v, want [send edit]", r.calls)
	}
	if r.sendText != "Umar bhai kaisa hai" || r.editText != r.sendText {
		t.Errorf("send %q edit %q — must edit to the same text", r.sendText, r.editText)
	}
	if r.editID != "MSG123" {
		t.Errorf("edited id = %q, want the id returned by send", r.editID)
	}
}

// A failed send (empty ID) must NOT be followed by an edit of an empty ID.
func TestSendAssetTextEditedSkipsEditOnFailedSend(t *testing.T) {
	r := &recorder{sendID: ""}
	if id := sendAssetTextEdited(r, types.MessageInfo{}, "hi", 0); id != "" {
		t.Fatalf("id = %q, want empty", id)
	}
	if len(r.calls) != 1 || r.calls[0] != "send" {
		t.Fatalf("calls = %v, want only [send]", r.calls)
	}
}

// The edit must wait the configured delay (1s like the autoreply pipeline) so
// the text is visible before it flips to "edited".
func TestSendAssetTextEditedWaitsBeforeEdit(t *testing.T) {
	r := &recorder{sendID: "ID1"}
	start := time.Now()
	sendAssetTextEdited(r, types.MessageInfo{}, "hi", 60*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 55*time.Millisecond {
		t.Fatalf("edited after %v, want >= the delay", elapsed)
	}
}
