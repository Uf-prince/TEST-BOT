package goldcmds

// ============================================================================
// GOLD-MD — .TTS  (TEXT TO VOICE)
// File: tts.go
// ============================================================================
// COMMAND:
//   .tts <text>          -> generate a voice note from the text
//   .tts                 -> (reply to a message) speak that message
//
// Powered by Google Translate TTS (free web endpoint, no API key):
//   https://translate.google.com/translate_tts?ie=UTF-8&client=tw-ob&tl=<lang>&q=<text>
//   Returns an MP3 (24 kHz mono). Long text is split into <=190-char chunks
//   and the MP3 frames are concatenated (MP3 is frame-concatenable).
//
// The audio is sent as a NON-PTT audio message (PTT = false) per owner rule.
//
// Aliases (Hidden): ttsay, voice, say, speak
// ============================================================================

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ttsGuide is the guidance message shown when no text is supplied.
func ttsGuide(prefix string) string {
	return "*🔰 TEXT TO VOICE 🔰*\n\n" +
		"*TURN ANY TEXT INTO A VOICE MESSAGE INSTANTLY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TTS <TEXT> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TTS HELLO BROTHER HOW ARE YOU ❯*\n\n" +
		"*OR REPLY TO ANY MESSAGE AND TYPE:*\n" +
		"*❮ " + prefix + "TTS ❯*\n\n" +
		"*THE BOT WILL SEND THE TEXT AS A VOICE NOTE*"
}

// ttsChunk splits text into chunks of at most maxLen runes, preferring to
// break on sentence/word boundaries so the speech sounds natural.
func ttsChunk(text string, maxLen int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			chunks = append(chunks, string(runes))
			break
		}
		cut := maxLen
		// Prefer a sentence boundary (. ! ?) then a space.
		for i := maxLen; i > maxLen/2; i-- {
			if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' {
				cut = i + 1
				break
			}
		}
		if cut == maxLen {
			for i := maxLen; i > maxLen/2; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[:cut])))
		runes = runes[cut:]
	}
	return chunks
}

// ttsFetchChunk downloads one MP3 chunk from Google Translate TTS.
func ttsFetchChunk(ctx context.Context, text, lang string) ([]byte, error) {
	u := "https://translate.google.com/translate_tts?ie=UTF-8&client=tw-ob" +
		"&tl=" + url.QueryEscape(lang) + "&q=" + url.QueryEscape(text)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	req.Header.Set("Referer", "https://translate.google.com/")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tts http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(data) < 128 {
		return nil, fmt.Errorf("tts empty audio")
	}
	return data, nil
}

// ttsGenerate builds the full MP3 for the text (chunked + concatenated).
func ttsGenerate(ctx context.Context, text, lang string) ([]byte, error) {
	chunks := ttsChunk(text, 190)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no text")
	}
	var buf bytes.Buffer
	for _, c := range chunks {
		part, err := ttsFetchChunk(ctx, c, lang)
		if err != nil {
			return nil, err
		}
		buf.Write(part)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("tts produced no audio")
	}
	return buf.Bytes(), nil
}

func handleTTS(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleTTSAsync(ctx, s, info, args, prefix)
	})
}

func handleTTSAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		if q := strings.TrimSpace(s.GetQuotedMessageText(info)); q != "" {
			text = q
		}
	}
	if text == "" {
		s.Reply(info, ttsGuide(prefix))
		return
	}

	waitID := s.ReplyWithID(info, "*GENERATING VOICE....*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	audio, err := ttsGenerate(ctx, text, "en")
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 VOICE GENERATION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	// Probe duration via a temp file (SendAudio needs seconds).
	seconds := uint32(0)
	if tmp, terr := os.CreateTemp("", "gold-tts-*.mp3"); terr == nil {
		if _, werr := tmp.Write(audio); werr == nil {
			_ = tmp.Close()
			seconds = probeAudioDuration(tmp.Name())
		} else {
			_ = tmp.Close()
		}
		_ = os.Remove(tmp.Name())
	}

	if err := s.SendAudio(info, audio, "", seconds); err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 FAILED TO SEND VOICE, PLEASE TRY AGAIN*")
		}
	}
}

func init() {
	Register(Command{Name: "tts", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO CONVERT ANY TEXT INTO A VOICE MESSAGE. USE IT AS .TTS <TEXT> OR REPLY TO A MESSAGE WITH .TTS.", Run: handleTTS})

	// aliases (Hidden)
	Register(Command{Name: "ttsay", Hidden: true, Run: handleTTS})
	Register(Command{Name: "voice", Hidden: true, Run: handleTTS})
	Register(Command{Name: "say", Hidden: true, Run: handleTTS})
	Register(Command{Name: "speak", Hidden: true, Run: handleTTS})
}
