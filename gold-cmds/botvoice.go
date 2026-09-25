package goldcmds

// ============================================================================
// GOLD-MD — BOTVOICE Command  (change menu + alive voice)
// File: botvoice.go
// ----------------------------------------------------------------------------
// Sibling of .botpic / .botvideo: the owner sets an MP3 VOICE that plays right
// after .menu, every category menu and the .alive card. Uploads go through the
// SAME host chain as .url (catbox → qu.ax → uguu → gofile, first success wins)
// so a .botvoice clip is stored exactly where .url stores files. The resulting
// URL is kept in Redis settings:<botJID> field "botvoice".
//
//   .botvoice                → guide
//   .botvoice reset          → silence the bot voice ("off" sentinel)
//   .botvoice default        → restore the built-in default MP3
//   .botvoice <mp3-url>      → set directly from a link
//   (reply to / send audio + .botvoice)
//                            → convert to MP3, upload, save the URL
//
// Owner-only command. Hidden aliases work silently (never in the menu).
// ============================================================================

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// botVoiceGuide is the no-argument help card.
func botVoiceGuide(prefix string) string {
	return "*🔰 BOT VOICE CHANGE GUIDE 🔰*\n\n" +
		"*DO YOU WANT TO CHANGE YOUR BOT MENU + ALIVE VOICE*\n" +
		"*THE VOICE PLAYS RIGHT AFTER THE MENU / ALIVE IS SENT*\n\n" +
		"*1❯ SIMPLY SEND YOUR AUDIO HERE*\n" +
		"*2❯ REPLY TO THE AUDIO AND TYPE ❰ " + prefix + "BOTVOICE ❱*\n" +
		"*3❯ OR SEND A LINK:*\n" +
		"*❰ " + prefix + "BOTVOICE <MP3-URL> ❱*\n\n" +
		"*TO SILENCE THE VOICE TYPE*\n" +
		"*❰ " + prefix + "BOTVOICE RESET ❱*\n\n" +
		"*TO GO BACK TO THE DEFAULT VOICE TYPE*\n" +
		"*❰ " + prefix + "BOTVOICE DEFAULT ❱*"
}

func handleBotVoice(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	argRaw := ""
	if len(args) > 0 {
		argRaw = strings.TrimSpace(strings.Join(args, " "))
	}

	// ── RESET (silence) ──
	switch strings.ToLower(argRaw) {
	case "reset":
		s.SetBotVoiceSetting(MenuMediaVoiceOff)
		s.Reply(info, "*🔰 BOT VOICE RESET 🔰*\n\n*CUSTOM VOICE REMOVED*\n*MENU AND ALIVE ARE NOW SILENT*")
		return
	case "default":
		s.SetBotVoiceSetting("")
		s.Reply(info, "*🔰 BOT VOICE RESET TO DEFAULT 🔰*\n\n*DEFAULT VOICE IS BACK FOR MENU + ALIVE*")
		return
	}

	// ── URL se set ──
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := voiceURLRe.FindString(firstTok); m != "" {
			if !s.VoiceURLPlayable(m) {
				s.Reply(info, "*🔰 VOICE LINK FAIL 🔰*\n\n*YE LINK SE VOICE NIKAL NAHI PAYI*\n*DOBARA PROPER MP3 LINK YA AUDIO BHEJ KAR TRY KARO*")
				return
			}
			s.SetBotVoiceSetting(m)
			s.Reply(info, fmt.Sprintf("*🔰 BOT VOICE UPDATED 🔰*\n\n*MENU + ALIVE DONO KA VOICE CHANGE HO GYA*\n\n*NEW VOICE:*\n%s\n\n*FOR TEST TYPE ❰ ALIVE ❱ OR ❰ MENU ❱*", m))
			return
		}
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, "*🔰 INVALID VOICE LINK 🔰*\n\n*LINK .mp3 / .m4a / .ogg / .opus / .wav PE KHATAM HONI CHAHIYE*")
			return
		}
	}

	// ── AUDIO DETECTION (direct or quoted) ──
	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 || !menuVoiceIsAudioMime(mime) {
		s.Reply(info, botVoiceGuide(prefix))
		return
	}

	waitID := s.ReplyWithID(info, "*🔰 BOT VOICE UPLOAD HO RAHI HAI...*\n*PROCESSING: 00%*")
	stop := make(chan struct{})
	go func() {
		p := 0
		tk := time.NewTicker(500 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				if p >= 90 {
					continue
				}
				p += 7
				if p > 90 {
					p = 90
				}
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING BOT VOICE*\n*PROCESSING: %02d%%*", p))
			}
		}
	}()

	in, err := os.CreateTemp("", "goldmd-botvoice-in-*"+menuVoiceExt(mime))
	if err != nil {
		close(stop)
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *Temp file fail*")
		return
	}
	inPath := in.Name()
	defer os.Remove(inPath)
	if _, err := in.Write(data); err != nil {
		in.Close()
		close(stop)
		s.DeleteMessage(info, waitID)
		s.Reply(info, fmt.Sprintf("🔰 *Write fail:* %v", err))
		return
	}
	in.Close()

	outPath, nerr := menuVoiceToMP3(inPath)
	var norm []byte
	if nerr == nil && outPath != "" {
		defer os.Remove(outPath)
		norm, _ = os.ReadFile(outPath)
	}
	if len(norm) == 0 {
		norm = data
	}

	url, _, uerr := uploadAnyHost(norm, "goldmd-botvoice.mp3", "audio/mpeg")
	close(stop)
	s.DeleteMessage(info, waitID)
	if uerr != nil || url == "" {
		s.Reply(info, fmt.Sprintf("🔰 *Upload fail:* %v", uerr))
		return
	}
	if direct := resolveDirectMediaURL(url); direct != "" {
		url = direct
	}
	if !BotVoiceURLIsPlayable(url) {
		s.Reply(info, "*🔰 VOICE SETUP FAIL 🔰*\n\n*UPLOADED FILE PLAYABLE AUDIO NAHI NIKLI*\n*DOBARA PROPER MP3 BHEJ KAR ❰ "+prefix+"BOTVOICE ❱ TRY KARO*")
		return
	}
	s.SetBotVoiceSetting(url)
	s.Reply(info, "*🔰 BOT VOICE UPDATED 🔰*\n\n"+
		"*NEW VOICE FOR MENU + ALIVE HAS BEEN APPLIED*\n\n"+
		"*FOR TEST TYPE ❰ ALIVE ❱ OR ❰ MENU ❱*")
}

func init() {
	Register(Command{
		Name:      "botvoice",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE BOT MENU AND ALIVE VOICE (MP3). REPLY TO AN AUDIO AND USE THIS COMMAND.",
		OwnerOnly: true,
		Run:       handleBotVoice,
	})
	// hidden aliases (same work, never in the menu)
	for _, alias := range []string{"botaudio", "botmp3", "botvoicech", "voicebot", "audiobot"} {
		Register(Command{Name: alias, OwnerOnly: true, Hidden: true, Run: handleBotVoice})
	}
}
