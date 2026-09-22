package goldcmds

// ============================================================================
// GOLD-MD — .ATTP  (RGB NAME STICKER)
// File: attp.go
// ============================================================================
// COMMAND:
//   .attp <name>          -> animated RGB name sticker (WebP)
//   .attp                 -> (reply to a message) use that text as the name
//
// Generates a 512x512 animated WebP sticker where the given name is rendered
// in a bold font and the colour cycles through the full RGB rainbow. Built
// entirely with ffmpeg (drawtext + hue rotation) — no external API.
//
// The sticker is sent via SendSticker (image/webp).
//
// Aliases (Hidden): rgb, rgbtext, attpsticker, namesticker
// ============================================================================

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// attpGuide is the guidance message shown when no name is supplied.
func attpGuide(prefix string) string {
	return "*🔰 RGB NAME STICKER 🔰*\n\n" +
		"*TURN ANY NAME INTO A GLOWING RGB STICKER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ATTP <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ATTP GOLD-MD ❯*\n\n" +
		"*OR REPLY TO ANY MESSAGE AND TYPE:*\n" +
		"*❮ " + prefix + "ATTP ❯*\n\n" +
		"*THE BOT WILL SEND AN ANIMATED RGB STICKER OF THAT NAME*"
}

// attpFontPath returns the first available bold font on the system.
func attpFontPath() string {
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
		"/usr/share/fonts/truetype/freefont/FreeSansBold.ttf",
		"/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// attpEscape escapes text for use inside an ffmpeg drawtext filter.
func attpEscape(s string) string {
	// Backslash first, then the special drawtext characters.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ":", "\\:")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}

// attpBuildSticker renders the animated RGB name sticker to a temp .webp file
// and returns its path. The caller owns cleanup.
func attpBuildSticker(ctx context.Context, name string) (string, error) {
	if !isFfmpegAvailable() {
		return "", fmt.Errorf("ffmpeg not available")
	}
	font := attpFontPath()
	if font == "" {
		return "", fmt.Errorf("no bold font found")
	}

	// Keep the name short so it fits nicely on the sticker.
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 20 {
		runes = runes[:20]
	}
	text := attpEscape(string(runes))

	// Font size scales down for longer names so the text always fits.
	fontSize := 130
	switch {
	case len(runes) > 14:
		fontSize = 60
	case len(runes) > 10:
		fontSize = 78
	case len(runes) > 7:
		fontSize = 96
	case len(runes) > 4:
		fontSize = 112
	}

	out, err := os.CreateTemp("", "gold-attp-*.webp")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	_ = out.Close()

	// 3s @ 20fps = 60 frames. Text drawn in red then hue-rotated a full
	// 360° over the duration -> smooth RGB rainbow. A soft shadow gives a
	// glow so the sticker looks premium on any chat background.
	filter := fmt.Sprintf(
		"drawtext=fontfile=%s:text='%s':fontcolor=red:fontsize=%d:"+
			"x=(w-text_w)/2:y=(h-text_h)/2:"+
			"shadowcolor=black@0.6:shadowx=4:shadowy=4,"+
			"hue=H=2*PI*t/3:s=1",
		font, text, fontSize)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=black@0.0:s=512x512:d=3:r=20",
		"-vf", filter,
		"-c:v", "libwebp", "-loop", "0", "-q:v", "75", "-an",
		outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("ffmpeg rgb sticker: %v, stderr: %s", err, stderr.String())
	}
	if st, serr := os.Stat(outPath); serr != nil || st.Size() < 512 {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("empty sticker output")
	}
	return outPath, nil
}

func handleATTP(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleATTPAsync(ctx, s, info, args, prefix)
	})
}

func handleATTPAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		if q := strings.TrimSpace(s.GetQuotedMessageText(info)); q != "" {
			name = q
		}
	}
	if name == "" {
		s.Reply(info, attpGuide(prefix))
		return
	}

	waitID := s.ReplyWithID(info, "*MAKING RGB STICKER....*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	outPath, err := attpBuildSticker(ctx, name)
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 RGB STICKER FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	defer removeTempFile(outPath)

	webpBytes, err := os.ReadFile(outPath)
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 RGB STICKER FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	if err := s.SendSticker(info, webpBytes); err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 FAILED TO SEND STICKER, PLEASE TRY AGAIN*")
		}
	}
}

func init() {
	Register(Command{Name: "attp", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO MAKE AN ANIMATED RGB NAME STICKER. USE IT AS .ATTP <NAME> OR REPLY TO A MESSAGE WITH .ATTP.", Run: handleATTP})

	// aliases (Hidden)
	Register(Command{Name: "rgb", Hidden: true, Run: handleATTP})
	Register(Command{Name: "rgbtext", Hidden: true, Run: handleATTP})
	Register(Command{Name: "attpsticker", Hidden: true, Run: handleATTP})
	Register(Command{Name: "namesticker", Hidden: true, Run: handleATTP})
}
