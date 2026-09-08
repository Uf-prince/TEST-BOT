package goldcmds

// ============================================================================
// GOLD-MD — .addvoice / .delvoice / .voicelist commands
//
// Ported from UMAR-MD (Node.js pair.js, lines 17299-17520) — same text,
// same behaviour, adapted for whatsmeow:
//   .addvoice <name>   (aliases: .savevoice)
//       Quote an audio message and write .addvoice <name> — saves the audio
//       so that whenever anyone writes that name the voice is sent automatically.
//   .delvoice <name>   (aliases: .deletevoice, .removevoice)
//       Deletes a saved voice by name.
//   .voicelist         (aliases: .voices, .listvoices)
//       Lists all saved voices.
//
// Owner-only. Audio is stored on the filesystem (<DataDir>/voices/<botJID>/)
// with a name index in a Redis SET. The trigger (auto-send on name mention)
// is handled in handler.go via the exported VoiceTriggerMatch helper.
// ============================================================================

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// voiceOwnerGate checks group/owner. Voice commands are owner-only and work
// in DM or group (the Node.js version has no group restriction for these).
// Replies with "*THIS COMMAND IS ONLY FOR ME 😎*" when the sender is not owner.
func voiceOwnerGate(s SessionBridge, info types.MessageInfo) bool {
	if s.IsOwner(info) {
		return true
	}
	s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
	return false
}

// examplePrefix returns the prefix to show in help text (uppercase, like Node.js).
// The prefix passed by the dispatcher is used directly; Node.js uses _UmarGetExamplePrefix
// which resolves to the stored prefix or ".". We uppercase it for the help text.
func examplePrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = "."
	}
	return strings.ToUpper(p)
}

// ---------------------------------------------------------------------------
// .addvoice  /  .savevoice
// ---------------------------------------------------------------------------

func handleAddVoice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAddVoiceAsync(s, info, args, prefix)
}

func handleAddVoiceAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !voiceOwnerGate(s, info) {
		return
	}

	name := ""
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
	}

	if name == "" {
		ex := examplePrefix(prefix)
		s.Reply(info, "*🔰 ADDVOICE INFO 🔰*\n\n*QUOTE AN AUDIO MESSAGE AND WRITE:*\n*TYPE ❰ "+ex+"ADDVOICE <NAME> ❱*\n\n*EXAMPLE:*\n*TYPE ❰ "+ex+"ADDVOICE UMAR ❱*\n*TYPE ❰ "+ex+"ADDVOICE HELLO ❱*\n\n*AFTER SAVING, WHENEVER ANYONE WRITES THAT NAME THE VOICE WILL BE SENT AUTOMATICALLY.*")
		return
	}

	// Download the quoted audio (DownloadQuotedMedia follows ContextInfo.QuotedMessage).
	data, _, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 {
		s.Reply(info, "*⚠️ QUOTE AN AUDIO MESSAGE FIRST THEN WRITE .ADDVOICE <NAME>*")
		return
	}

	// Force mime "audio/mp4" — same as Node.js (loudspeaker playback).
	mime := "audio/mp4"
	if s.SaveCustomVoice(name, data, mime) {
		s.Reply(info, "*✅ VOICE SAVED SUCCESSFULLY*\n\n*NAME :❰ "+strings.ToUpper(name)+"*\n\n*NOW WHENEVER ANYONE WRITES* *"+strings.ToUpper(name)+"* *THIS VOICE WILL BE SENT AUTOMATICALLY 🎙️*")
	} else {
		s.Reply(info, "*❌ FAILED TO SAVE VOICE — TRY AGAIN*")
	}
}

// ---------------------------------------------------------------------------
// .delvoice  /  .deletevoice  /  .removevoice
// ---------------------------------------------------------------------------

func handleDelVoice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDelVoiceAsync(s, info, args, prefix)
}

func handleDelVoiceAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !voiceOwnerGate(s, info) {
		return
	}

	name := ""
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
	}

	if name == "" {
		s.Reply(info, "*WRITE THE VOICE NAME TO DELETE\nEXAMPLE: .DELVOICE UMAR*")
		return
	}

	if s.DeleteCustomVoice(name) {
		s.Reply(info, "*✅ VOICE DELETED*\n\n*NAME :❰ "+strings.ToUpper(name)+"*")
	} else {
		s.Reply(info, fmt.Sprintf("*❌ VOICE \"%s\" NOT FOUND*", strings.ToUpper(name)))
	}
}

// ---------------------------------------------------------------------------
// .voicelist  /  .voices  /  .listvoices
// ---------------------------------------------------------------------------

func handleVoiceList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleVoiceListAsync(s, info, args, prefix)
}

func handleVoiceListAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	voices := s.ListCustomVoices()
	if len(voices) == 0 {
		s.Reply(info, "*NO VOICES SAVED YET*\n*Use .ADDVOICE <NAME> to save one*")
		return
	}

	var sb strings.Builder
	sb.WriteString("*🎙️ SAVED VOICES LIST 🎙️*\n\n")
	for i, v := range voices {
		sb.WriteString(fmt.Sprintf("*%d. %s*\n", i+1, strings.ToUpper(v)))
	}
	sb.WriteString(fmt.Sprintf("\n*Total: %d voice(s)*", len(voices)))
	s.Reply(info, sb.String())
}

// ---------------------------------------------------------------------------
// VoiceTriggerMatch checks whether a plain-text body (single word, ≤50 chars,
// no .!/# prefix) matches a saved voice name. Returns the name + true if so.
// Used by handler.go to auto-send the voice. Mirrors Node.js pair.js lines
// ~17460 (custom voice trigger).
// ---------------------------------------------------------------------------

func VoiceTriggerMatch(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || len(trimmed) > 50 {
		return "", false
	}
	// Skip if starts with a command prefix character.
	if trimmed[0] == '.' || trimmed[0] == '!' || trimmed[0] == '/' || trimmed[0] == '#' {
		return "", false
	}
	return strings.ToLower(trimmed), true
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func init() {
	Register(Command{Name: "addvoice", Category: "AI & MEDIA", Desc: "Save a quoted audio as a custom voice", OwnerOnly: true, Run: handleAddVoice})
	Register(Command{Name: "savevoice", OwnerOnly: true, Hidden: true, Run: handleAddVoice})
	Register(Command{Name: "delvoice", Category: "AI & MEDIA", Desc: "Delete a saved custom voice", OwnerOnly: true, Run: handleDelVoice})
	Register(Command{Name: "deletevoice", OwnerOnly: true, Hidden: true, Run: handleDelVoice})
	Register(Command{Name: "removevoice", OwnerOnly: true, Hidden: true, Run: handleDelVoice})
	Register(Command{Name: "voicelist", Category: "AI & MEDIA", Desc: "List all saved custom voices", OwnerOnly: true, Run: handleVoiceList})
	Register(Command{Name: "voices", OwnerOnly: true, Hidden: true, Run: handleVoiceList})
	Register(Command{Name: "listvoices", OwnerOnly: true, Hidden: true, Run: handleVoiceList})
}
