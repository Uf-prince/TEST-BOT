package goldcmds

// ============================================================================
// GOLD-MD — .rm command (WhatsApp Read More text generator)
// File: readmore.go
// ----------------------------------------------------------------------------
// Turns plain text into a WhatsApp "Read More" message.  The trick uses a
// long run of the invisible Unicode LEFT-TO-RIGHT MARK (U+200E): WhatsApp
// COUNTS those characters when it decides to collapse a text message into
// a "Read more" button, but they RENDER as nothing.  The collapse point
// always lands inside the invisible run, so whatever comes before it
// stays visible in the chat bubble and whatever comes after it stays
// hidden until the reader taps "Read more".
//
//   .rm I am umar                 → whole text hidden behind Read more
//   .rm Hello everyone|I am umar  → "Hello everyone" visible, rest hidden
//
// .readmore is a hidden alias — it works exactly the same but never shows
// in the menu (same silent-alias pattern as the other commands).
//
// Formula (checked against the latest working Read More generators):
//   visible + U+200E x (1100 - len(visible)) + " " + hidden
//
// U+200E (LRM) is used instead of the older U+202E (RLO) trick — LRM can
// never flip the direction of the user's text, so mixed-content messages
// always render exactly as typed.
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// rmFillerTarget is the character count the invisible run pushes the
// message towards.  WhatsApp collapses text messages into a "Read more"
// button once they pass ~1000 characters, so 1100 keeps a safety margin
// for threshold differences between client versions.
const rmFillerTarget = 1100

// rmMinFiller is the smallest invisible gap between the visible and the
// hidden part, so the seam never shows even with a long visible part.
const rmMinFiller = 100

// rmInvisible is the invisible Unicode LEFT-TO-RIGHT MARK (U+200E) —
// zero width, no rendering, direction-safe.
const rmInvisible = "\u200E"

func handleReadMore(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		s.Reply(info, rmUsage(prefix))
		return
	}

	// .rm visible|hidden  →  the first "|" splits the two parts.
	// .rm text            →  no "|" means the whole text hides.
	visible := ""
	hidden := text
	if parts := strings.SplitN(text, "|", 2); len(parts) == 2 {
		visible = strings.TrimSpace(parts[0])
		hidden = strings.TrimSpace(parts[1])
	}
	if hidden == "" {
		s.Reply(info, rmUsage(prefix))
		return
	}

	// Invisible filler: long enough that the "Read more" collapse point
	// always lands inside it — the visible part stays in the preview and
	// the hidden part only opens on tap.
	filler := rmFillerTarget - len([]rune(visible))
	if filler < rmMinFiller {
		filler = rmMinFiller
	}
	s.Reply(info, visible+strings.Repeat(rmInvisible, filler)+" "+hidden)
}

// rmUsage — full 🔰 styled guide (pure English).
func rmUsage(prefix string) string {
	return "*🔰 USAGE :❱*\n\n" +
		"*RM ❮ TEXT ❯*\n\n" +
		"*" + prefix + "rm I am umar*\n" +
		"*THE WHOLE TEXT HIDES BEHIND A READ MORE BUTTON — THE MESSAGE STAYS COLLAPSED AND ONLY OPENS WHEN SOMEONE TAPS READ MORE*\n\n" +
		"*" + prefix + "rm Hello everyone|I am umar*\n" +
		"*TEXT BEFORE ❮ | ❯ STAYS VISIBLE — TEXT AFTER ❮ | ❯ HIDES BEHIND READ MORE*\n\n" +
		"*🔰 NOTE :❱*\n" +
		"*THE HIDDEN PART IS FULLY INTACT — NOTHING IS DELETED, IT JUST STAYS BEHIND THE BUTTON*"
}

func init() {
	Register(Command{Name: "rm", Category: "AI & MEDIA", Desc: "Create WhatsApp Read More hidden text", Run: handleReadMore})
	Register(Command{Name: "readmore", Hidden: true, Run: handleReadMore})
}
