package goldcmds

// ============================================================================
// GOLD-MD — EQUALIZER category (1000 designs) + named effect aliases
// File: equalizer.go
// ============================================================================
// OWNER ORDER: .menu me ek "EQUALIZER" category ho jis me 1000 designs hon.
//   .equalizer         → fancy boxed menu, EQ1 .. EQ1000
//   .eqN (reply media) → us combo ka effect us audio/video par lagta hai.
//   .slowed/.revert/.robot/.bass/.dj → wahi engine, chune hue designs ke naam.
//
// Video diya ho to audio effect uske andar lagta hai aur video wapas bhej dete
// hain (jab pitch badalta hai to video bhi retime hota hai taake A/V sync rahe).
//
// Memory safety: sab kuch file-to-file on disk (ffmpeg), RAM me sirf chhote
// control buffers.
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// eqEffect describes one resolved design (a named alias or an .eqN combo).
type eqEffect struct {
	// Name is the command name (also the menu label, uppercased).
	Name string
	// Desc is the one-line description shown in the menu.
	Desc string
	// Audio is the -af filter chain applied to the audio stream.
	Audio string
	// VideoPTS retimes the picture when non-empty (pitch/tempo changed).
	VideoPTS string
}

// resolveEqDesign builds the concrete effect for design number n.
func resolveEqDesign(n int) eqEffect {
	return eqEffect{
		Name:     fmt.Sprintf("eq%d", n),
		Audio:    EqAudioFilter(n),
		VideoPTS: EqVideoPTS(n),
	}
}

// eqDesignFromNamed returns the design for a named alias, if any.
func eqDesignFromNamed(name string) (eqEffect, bool) {
	n, ok := eqNamedPresets[name]
	if !ok {
		return eqEffect{}, false
	}
	e := resolveEqDesign(n)
	e.Name = name
	return e, true
}

// eqHasAudioStream reports whether the file carries at least one audio stream.
// ffprobe is bundled with the ffmpeg static build (ensureFfmpeg), so this is
// safe on hosts without a system ffprobe. On probe failure it returns true and
// lets ffmpeg produce the real error.
func eqHasAudioStream(ctx context.Context, path string) bool {
	if !isFfprobeAvailable() {
		return true
	}
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=index", "-of", "csv=p=0", path).Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) != ""
}

// ffmpegApplyEffectFile applies eff to inputPath and returns the output path.
// Audio-only input → MP3; video input → MP4 with the same audio effect inside.
func ffmpegApplyEffectFile(ctx context.Context, inputPath, mime string, eff eqEffect) (string, error) {
	if isVideoMime(mime) {
		return ffmpegApplyEffectVideo(ctx, inputPath, eff)
	}
	return ffmpegApplyEffectAudio(ctx, inputPath, eff)
}

// ffmpegApplyEffectAudio encodes the filtered audio to MP3.
func ffmpegApplyEffectAudio(ctx context.Context, inputPath string, eff eqEffect) (string, error) {
	outputPath := inputPath + ".eq.mp3"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath,
		"-vn", "-af", eff.Audio,
		"-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2",
		outputPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg eq audio error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

// ffmpegApplyEffectVideo keeps the picture and rewrites the audio inside the
// video. When the effect changes the audio duration the video is also retimed,
// so picture and sound stay in sync; otherwise the video stream is copied.
func ffmpegApplyEffectVideo(ctx context.Context, inputPath string, eff eqEffect) (string, error) {
	outputPath := inputPath + ".eq.mp4"

	args := []string{"-y", "-i", inputPath, "-map", "0:v:0", "-map", "0:a:0?"}
	if eff.VideoPTS != "" {
		args = append(args, "-vf", "setpts="+eff.VideoPTS,
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p")
	} else {
		args = append(args, "-c:v", "copy")
	}
	args = append(args, "-af", eff.Audio,
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", outputPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg eq video error: %v, stderr: %s", err, stderr.String())
	}
	return outputPath, nil
}

func eqNoMediaText(prefix, name string) string {
	return "*🔰 EQUALIZER INFO 🔰*\n\n" +
		"*FIRST MENTION ANY AUDIO OR VIDEO* 🔰\n" +
		"*THEN TYPE*\n\n" +
		"*❰ " + strings.ToUpper(prefix+name) + " ❱*\n\n" +
		"*TO APPLY THIS EFFECT ON THAT AUDIO / VIDEO*"
}

func eqFailedText(prefix, name string) string {
	return "*🔰 " + strings.ToUpper(name) + " EFFECT FAILED, PLEASE TRY AGAIN 🔰*"
}

// EqRunN is the entry point for the hidden .eq1..eq1000 commands.
func EqRunN(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	if n < 1 || n > EqCount {
		s.Reply(info, "*EQ NUMBER 1 SE 1000 TAK HI HAI*")
		return
	}
	runEqualizerEffect(s, info, prefix, resolveEqDesign(n))
}

// NamedRun is the entry point for the .slowed / .revert / .robot / .bass / .dj
// aliases.
func NamedRun(name string) func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		eff, ok := eqDesignFromNamed(name)
		if !ok {
			return
		}
		runEqualizerEffect(s, info, prefix, eff)
	}
}

// runEqualizerEffect runs one effect end-to-end with the command watchdog.
func runEqualizerEffect(s SessionBridge, info types.MessageInfo, prefix string, eff eqEffect) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		runEqualizerWithContext(ctx, s, info, prefix, eff)
	})
}

func runEqualizerWithContext(ctx context.Context, s SessionBridge, info types.MessageInfo, prefix string, eff eqEffect) {
	data, mime, ok, tooBig := downloadMediaLimited(s, info)
	if tooBig {
		return // already replied by the guard
	}
	if !ok || len(data) == 0 {
		s.Reply(info, eqNoMediaText(prefix, eff.Name))
		return
	}
	if !isFfmpegAvailable() {
		if !ctxTimedOut(ctx) {
			s.Reply(info, eqFailedText(prefix, eff.Name))
		}
		return
	}

	waitID := s.ReplyWithID(info, "*APPLYING "+strings.ToUpper(eff.Name)+" EFFECT...*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	isVideo := isVideoMime(mime)
	inPath, err := writeTempMedia(data, extForMime(mime))
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, eqFailedText(prefix, eff.Name))
		}
		return
	}
	defer removeTempFile(inPath)

	if !eqHasAudioStream(ctx, inPath) {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 IS MEDIA ME KOI AUDIO NAHI HAI — AUDIO YA VIDEO BHEJO JIS ME AWAAZ HO 🔰*")
		}
		return
	}

	outPath, err := ffmpegApplyEffectFile(ctx, inPath, mime, eff)
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, eqFailedText(prefix, eff.Name))
		}
		return
	}
	defer removeTempFile(outPath)

	if isVideo {
		if err := s.SendVideoFileRaw(info, outPath); err != nil && !ctxTimedOut(ctx) {
			s.Reply(info, eqFailedText(prefix, eff.Name))
		}
		return
	}
	if err := s.SendAudioFileRaw(info, outPath); err != nil && !ctxTimedOut(ctx) {
		s.Reply(info, eqFailedText(prefix, eff.Name))
	}
}

// handleEqualizerList implements bare .equalizer — the boxed EQ1..EQ1000 menu.
func handleEqualizerList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	s.ShowEqualizerMenu(info, args, prefix)
}

// eqNamedAliasNames is the fixed list of the 5 named effect aliases.
var eqNamedAliasNames = []string{"slowed", "revert", "robot", "bass", "dj"}

// eqAliasDesc is the menu description for the 5 named effect aliases.
func eqAliasDesc(name string) string {
	switch name {
	case "slowed":
		return "THIS COMMAND IS USED TO SLOW DOWN A MENTIONED AUDIO OR VIDEO WITH A LOWER PITCH AND A SOFT ECHO. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND."
	case "revert":
		return "THIS COMMAND IS USED TO REVERSE THE AUDIO OF A MENTIONED AUDIO OR VIDEO, SO IT PLAYS BACKWARDS. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND."
	case "robot":
		return "THIS COMMAND IS USED TO TURN A MENTIONED AUDIO OR VIDEO INTO A DEEP ROBOTIC VOICE USING PITCH SHIFT, ECHO AND VIBRATO. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND."
	case "bass":
		return "THIS COMMAND IS USED TO BOOST THE BASS OF A MENTIONED AUDIO OR VIDEO FOR A DEEP THUMPING SOUND. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND."
	case "dj":
		return "THIS COMMAND IS USED TO GIVE A MENTIONED AUDIO OR VIDEO A DJ / CLUB FEEL WITH ECHO, EXTRA BASS AND BRIGHT TREBLE. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND."
	}
	return ""
}

func init() {
	// Visible category entry: .equalizer shows the whole 1000-design list.
	Register(Command{
		Name:     "equalizer",
		Category: "EQUALIZER",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF 1000 EQUALIZER DESIGNS. REPLY TO ANY AUDIO OR VIDEO, THEN USE .EQ1 TO .EQ1000 TO APPLY THAT EFFECT.",
		Run:      handleEqualizerList,
	})
	for _, name := range eqNamedAliasNames {
		Register(Command{
			Name:     name,
			Category: "EQUALIZER",
			Desc:     eqAliasDesc(name),
			Run:      NamedRun(name),
		})
	}
}
