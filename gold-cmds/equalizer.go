package goldcmds

// ============================================================================
// GOLD-MD — EQUALIZER (audio effects) category
// File: equalizer.go
// ============================================================================
// OWNER ORDER: .menu me ek naya category "EQUALIZER" ho jis me 5 audio
// effects commands hon. User kisi audio/video ko mention (reply) kar ke
// command likhta hai to us media ki AUDIO par effect lag kar wapas aati hai.
//
//   .slowed   → audio slow + pitch down + halka reverb (slowed + reverb)
//   .revert   → poori audio reverse
//   .robot    → deep robotic voice (pitch shift + echo + vibrato)
//   .bass     → heavy bass boost
//   .dj       → DJ/club feel (echo + bass + treble)
//
// Video diya ho to audio effect uske andar lagta hai aur video wapas bhej
// dete hain (slowed me video bhi slow ho jata hai taake A/V sync rahe).
//
// Memory safety: sab kuch file-to-file on disk (ffmpeg), RAM me sirf chhote
// control buffers — bilkul tomp3.go / video-play.go jaisa pattern.
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// eqEffect describes one audio effect and the ffmpeg filters it applies.
type eqEffect struct {
	// Name is the command name (also the menu label, uppercased).
	Name string
	// Desc is the one-line description shown in the menu.
	Desc string
	// Audio is the -af filter chain applied to the audio stream.
	Audio string
	// StretchVideo is true when the effect changes the audio duration, in
	// which case the video must be retimed (setpts) and re-encoded so the
	// picture stays in sync with the slowed/sped-up audio.
	StretchVideo bool
	// VideoPTS is the setpts multiplier used when StretchVideo is true.
	VideoPTS string
}

// eqEffects is the fixed set of 5 effects (owner order). Order here is the
// order they appear in the EQUALIZER category menu.
var eqEffects = []eqEffect{
	{
		Name:         "slowed",
		Desc:         "THIS COMMAND IS USED TO SLOW DOWN THE AUDIO OF A MENTIONED AUDIO OR VIDEO. IT ALSO LOWERS THE PITCH AND ADDS A SOFT REVERB FOR A SLOWED + REVERB FEEL. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND.",
		Audio:        "asetrate=44100*0.92,aresample=44100,atempo=0.90,aresample=44100,aecho=0.8:0.85:120:0.25",
		StretchVideo: true,
		VideoPTS:     "1.21*PTS",
	},
	{
		Name:  "revert",
		Desc:  "THIS COMMAND IS USED TO REVERSE THE AUDIO OF A MENTIONED AUDIO OR VIDEO, SO IT PLAYS BACKWARDS. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND.",
		Audio: "areverse",
	},
	{
		Name:  "robot",
		Desc:  "THIS COMMAND IS USED TO TURN THE AUDIO OF A MENTIONED AUDIO OR VIDEO INTO A DEEP ROBOTIC VOICE USING PITCH SHIFT, ECHO AND VIBRATO. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND.",
		Audio: "asetrate=44100*0.85,aresample=44100,atempo=1.176,aecho=0.8:0.88:60:0.4,vibrato=f=12:d=0.6",
	},
	{
		Name:  "bass",
		Desc:  "THIS COMMAND IS USED TO BOOST THE BASS OF A MENTIONED AUDIO OR VIDEO FOR A DEEP THUMPING SOUND. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND.",
		Audio: "bass=g=12:f=110:w=0.6,alimiter=limit=0.95",
	},
	{
		Name:  "dj",
		Desc:  "THIS COMMAND IS USED TO GIVE A MENTIONED AUDIO OR VIDEO A DJ / CLUB FEEL WITH ECHO, EXTRA BASS AND BRIGHT TREBLE. REPLY TO ANY AUDIO OR VIDEO WITH THIS COMMAND.",
		Audio: "aecho=0.8:0.9:500|1000:0.6|0.4,bass=g=8:f=100:w=0.5,treble=g=5:f=3000:w=0.5,alimiter=limit=0.95",
	},
}

var eqEffectsBySlug = func() map[string]eqEffect {
	m := make(map[string]eqEffect, len(eqEffects))
	for _, e := range eqEffects {
		m[e.Name] = e
	}
	return m
}()

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
	isVideo := isVideoMime(mime)
	if isVideo {
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
// video. When the effect changes the audio duration (slowed) the video is also
// retimed, so picture and sound stay in sync; otherwise the video stream is
// copied untouched.
func ffmpegApplyEffectVideo(ctx context.Context, inputPath string, eff eqEffect) (string, error) {
	outputPath := inputPath + ".eq.mp4"

	args := []string{"-y", "-i", inputPath, "-map", "0:v:0", "-map", "0:a:0?"}
	if eff.StretchVideo {
		pts := eff.VideoPTS
		if pts == "" {
			pts = "1.0*PTS"
		}
		args = append(args, "-vf", "setpts="+pts)
		args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p")
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

// eqNoMediaText is the help shown when the command is used without a quoted
// audio/video (owner style: FIRST MENTION ...).
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

// handleEqualizerEffect is the shared Run for all 5 effect commands.
func handleEqualizerEffect(name string) func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		eff, ok := eqEffectsBySlug[name]
		if !ok {
			return
		}
		RunWithTimeout(s, info, func(ctx context.Context) {
			handleEqualizerEffectiveAsync(ctx, s, info, prefix, eff)
		})
	}
}

func handleEqualizerEffectiveAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, prefix string, eff eqEffect) {
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
	ext := extForMime(mime)
	inPath, err := writeTempMedia(data, ext)
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

func init() {
	for _, e := range eqEffects {
		e := e
		Register(Command{
			Name:     e.Name,
			Category: "EQUALIZER",
			Desc:     e.Desc,
			Run:      handleEqualizerEffect(e.Name),
		})
	}
}
