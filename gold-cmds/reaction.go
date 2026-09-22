package goldcmds

// ============================================================================
// GOLD-MD — .BREACTION / .GREACTION  (ANIME REACTION GIFS)
// File: reaction.go
// ============================================================================
// COMMANDS:
//   .breaction              -> menu of 5 BOY anime reactions
//   .breaction <name>       -> sends a BOY anime reaction gif
//   .greaction              -> menu of 5 GIRL anime reactions
//   .greaction <name>       -> sends a GIRL anime reaction gif
//
// REACTIONS (both menus): happy · smile · angry · teeth · sad
//   happy  -> gifukai action "happy"
//   smile  -> gifukai action "smile"
//   angry  -> gifukai action "angry"
//   teeth  -> gifukai action "teehee"  (grin showing teeth)
//   sad    -> gifukai action "cry"
//
// SOURCE: gifukai API (https://api.gifukai.com/v1/<action>?pairing=<m|f>)
//   pairing=m -> SOLO BOY  (BREACTION)
//   pairing=f -> SOLO GIRL (GREACTION)
//   Response JSON: { action, pairing, anime, url, filename, content_type }
//
// FLOW: fetch JSON -> download .gif -> ffmpeg convert to mp4 -> SendGif
//   (SendGif sends a VideoMessage with GifPlayback=true so WhatsApp loops it
//    exactly like a GIF.)
//
// CATEGORIES: "BREACTION" and "GREACTION" — both appear in the .menu.
// ============================================================================

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// reactionDef maps a user-facing reaction name to the gifukai action + emoji.
type reactionDef struct {
	Name   string // user-facing (happy, smile, angry, teeth, sad)
	Action string // gifukai action
	Emoji  string
}

// reactionDefs is the fixed 5-reaction set shown in BOTH menus.
var reactionDefs = []reactionDef{
	{Name: "happy", Action: "happy", Emoji: "\U0001F604"},  // 😄
	{Name: "smile", Action: "smile", Emoji: "\U0001F642"},  // 🙂
	{Name: "angry", Action: "angry", Emoji: "\U0001F620"},  // 😠
	{Name: "teeth", Action: "teehee", Emoji: "\U0001F601"}, // 😁
	{Name: "sad", Action: "cry", Emoji: "\U0001F622"},      // 😢
}

// reactionFind resolves a typed reaction name to its definition.
func reactionFind(name string) (reactionDef, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, d := range reactionDefs {
		if d.Name == n {
			return d, true
		}
	}
	return reactionDef{}, false
}

// reactionMenu builds the 5-reaction menu for the given gender.
// kind is "BOYS" or "GIRLS"; cmd is "breaction" or "greaction".
func reactionMenu(prefix, cmd, kind string) string {
	title := "BREACTION"
	if cmd == "greaction" {
		title = "GREACTION"
	}
	var b strings.Builder
	b.WriteString("*\U0001F530 " + title + " \u2014 " + kind + " ANIME REACTIONS \U0001F530*\n\n")
	b.WriteString("*TYPE ANY OF THESE:*\n\n")
	for _, d := range reactionDefs {
		b.WriteString("*" + d.Emoji + " " + strings.ToUpper(d.Name) + "  \u27A4  " + prefix + cmd + " " + d.Name + "*\n")
	}
	b.WriteString("\n*EXAMPLE \u27A4 " + prefix + cmd + " happy*")
	return b.String()
}

// gifukaiResp is the JSON returned by the gifukai API.
type gifukaiResp struct {
	Action string `json:"action"`
	Anime  string `json:"anime"`
	URL    string `json:"url"`
}

// reactionHTTPGet fetches a URL with the given context and byte cap.
func reactionHTTPGet(ctx context.Context, url string, cap int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cap))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// reactionFetchGif queries the gifukai API for the action+pairing and then
// downloads the returned .gif bytes. Returns the metadata + gif bytes.
func reactionFetchGif(ctx context.Context, action, pairing string) (gifukaiResp, []byte, bool) {
	apiURL := "https://api.gifukai.com/v1/" + action + "?pairing=" + pairing
	body, ok := reactionHTTPGet(ctx, apiURL, 1<<20)
	if !ok {
		return gifukaiResp{}, nil, false
	}
	var meta gifukaiResp
	if err := json.Unmarshal(body, &meta); err != nil || meta.URL == "" {
		return gifukaiResp{}, nil, false
	}
	gifData, ok := reactionHTTPGet(ctx, meta.URL, 25<<20)
	if !ok {
		return meta, nil, false
	}
	return meta, gifData, true
}

// reactionGifToMp4 converts raw .gif bytes to an mp4 (H.264, yuv420p, even
// dimensions) suitable for WhatsApp GIF playback. Returns the mp4 bytes plus
// duration/width/height probed from the result.
func reactionGifToMp4(ctx context.Context, gifData []byte) ([]byte, uint32, uint32, uint32, bool) {
	if !compressBinaryAvailable("ffmpeg") {
		return nil, 0, 0, 0, false
	}
	in, err := os.CreateTemp("", "goldreact-*.gif")
	if err != nil {
		return nil, 0, 0, 0, false
	}
	inPath := in.Name()
	in.Close()
	defer os.Remove(inPath)
	if err := os.WriteFile(inPath, gifData, 0o600); err != nil {
		return nil, 0, 0, 0, false
	}

	out, err := os.CreateTemp("", "goldreact-*.mp4")
	if err != nil {
		return nil, 0, 0, 0, false
	}
	outPath := out.Name()
	out.Close()
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath,
		"-movflags", "+faststart",
		"-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-an",
		outPath)
	if err := cmd.Run(); err != nil {
		return nil, 0, 0, 0, false
	}
	mp4, err := os.ReadFile(outPath)
	if err != nil || len(mp4) == 0 {
		return nil, 0, 0, 0, false
	}
	probe := compressProbeVideo(outPath)
	secs := uint32(probe.DurationSec)
	if secs == 0 {
		secs = 1
	}
	return mp4, secs, uint32(probe.Width), uint32(probe.Height), true
}

// handleReaction is the shared entry point for .breaction / .greaction.
func handleReaction(s SessionBridge, info types.MessageInfo, args []string, prefix, pairing, kind string) {
	go handleReactionAsync(s, info, args, prefix, pairing, kind)
}

func handleReactionAsync(s SessionBridge, info types.MessageInfo, args []string, prefix, pairing, kind string) {
	cmd := "breaction"
	if pairing == "f" {
		cmd = "greaction"
	}

	// No argument -> show the 5-reaction menu.
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		s.Reply(info, reactionMenu(prefix, cmd, kind))
		return
	}

	def, ok := reactionFind(args[0])
	if !ok {
		s.Reply(info, reactionMenu(prefix, cmd, kind))
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "\u26A1 *REACTION ERROR \u26A1*\n*BOT CLIENT NOT CONNECTED*")
		return
	}

	waitID := s.ReplyWithID(info, "*\U0001F530 "+strings.ToUpper(kind)+" "+strings.ToUpper(def.Name)+" REACTION \U0001F530*\n*FETCHING...*")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	meta, gifData, ok := reactionFetchGif(ctx, def.Action, pairing)
	if !ok {
		s.EditMessage(info, waitID, "*\U0001F530 REACTION ERROR \U0001F530*\n*COULD NOT FETCH GIF, TRY AGAIN*")
		return
	}

	mp4, secs, w, h, ok := reactionGifToMp4(ctx, gifData)
	if !ok {
		s.EditMessage(info, waitID, "*\U0001F530 REACTION ERROR \U0001F530*\n*CONVERSION FAILED, TRY AGAIN*")
		return
	}

	caption := "*\U0001F530 " + strings.ToUpper(kind) + " " + strings.ToUpper(def.Name) + " REACTION \U0001F530*\n\n" +
		"*\U0001F530 ANIME :\u27EB " + meta.Anime + "*"

	if err := s.SendGif(info, mp4, caption, secs, w, h); err != nil {
		s.EditMessage(info, waitID, "*\U0001F530 REACTION ERROR \U0001F530*\n*SEND FAILED, TRY AGAIN*")
		return
	}
	s.DeleteMessage(info, waitID)
}

func init() {
	Register(Command{
		Name:     "breaction",
		Category: "BREACTION",
		Desc:     "BOYS ANIME REACTION GIFS. TYPE .BREACTION FOR THE MENU (HAPPY, SMILE, ANGRY, TEETH, SAD).",
		Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
			handleReaction(s, info, args, prefix, "m", "BOYS")
		},
	})
	Register(Command{
		Name:     "greaction",
		Category: "GREACTION",
		Desc:     "GIRLS ANIME REACTION GIFS. TYPE .GREACTION FOR THE MENU (HAPPY, SMILE, ANGRY, TEETH, SAD).",
		Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
			handleReaction(s, info, args, prefix, "f", "GIRLS")
		},
	})
}
