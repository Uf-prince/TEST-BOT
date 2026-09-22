package goldcmds

// ============================================================================
// GOLD-MD — ANIME REACTION COMMANDS  (.b* BOYS / .g* GIRLS)
// File: reaction.go
// ============================================================================
// Two categories, each with the SAME full reaction set:
//   BREACTION → .b<name>  (e.g. .bhappy .bsad .bangry)  — SOLO BOY anime
//   GREACTION → .g<name>  (e.g. .ghappy .gsad .gangry)  — SOLO GIRL anime
//
// FLOW: user types .bhappy → the command message is DELETED → an anime
// reaction GIF is fetched → converted to mp4 → sent as a looping GIF with
// the caption:  I AM HAPPY 😄
//
// SOURCES (all free, no API key), tried in order:
//   1. gifukai   https://api.gifukai.com/v1/<action>?pairing=<m|f>   (gender!)
//   2. otakugifs https://api.otakugifs.xyz/gif?reaction=<name>
//   3. purrbot   https://api.purrbot.site/v2/img/sfw/<name>/gif
//   4. nekos.life https://nekos.life/api/v2/img/<name>
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

// reactionDef describes one reaction and where to fetch it from.
type reactionDef struct {
	Name    string // user-facing name (happy, sad, angry, ...)
	Emoji   string // emoji shown in the caption
	Gifukai string // gifukai action (supports gender pairing)
	Otaku   string // otakugifs reaction
	Purr    string // purrbot path
	Neko    string // nekos.life path
}

// reactionDefs is the full reaction catalog shared by BOTH categories.
var reactionDefs = []reactionDef{
	{Name: "agree", Emoji: "👍", Gifukai: "nod", Otaku: "", Purr: "", Neko: ""},
	{Name: "airkiss", Emoji: "😘", Gifukai: "", Otaku: "airkiss", Purr: "", Neko: ""},
	{Name: "angry", Emoji: "😠", Gifukai: "angry", Otaku: "", Purr: "angry", Neko: ""},
	{Name: "angrystare", Emoji: "😠", Gifukai: "", Otaku: "angrystare", Purr: "", Neko: ""},
	{Name: "bang", Emoji: "🎬", Gifukai: "shoot", Otaku: "", Purr: "", Neko: ""},
	{Name: "bite", Emoji: "😬", Gifukai: "bite", Otaku: "bite", Purr: "bite", Neko: ""},
	{Name: "bleh", Emoji: "😝", Gifukai: "bleh", Otaku: "bleh", Purr: "", Neko: ""},
	{Name: "blowkiss", Emoji: "😘", Gifukai: "blowkiss", Otaku: "", Purr: "", Neko: ""},
	{Name: "blush", Emoji: "😊", Gifukai: "blush", Otaku: "blush", Purr: "blush", Neko: ""},
	{Name: "bonk", Emoji: "🔨", Gifukai: "bonk", Otaku: "", Purr: "", Neko: ""},
	{Name: "boop", Emoji: "👉", Gifukai: "poke", Otaku: "", Purr: "", Neko: ""},
	{Name: "bored", Emoji: "😒", Gifukai: "bored", Otaku: "", Purr: "", Neko: ""},
	{Name: "brofist", Emoji: "👊", Gifukai: "", Otaku: "brofist", Purr: "", Neko: ""},
	{Name: "bye", Emoji: "👋", Gifukai: "bye", Otaku: "", Purr: "", Neko: ""},
	{Name: "carry", Emoji: "🫂", Gifukai: "carry", Otaku: "", Purr: "", Neko: ""},
	{Name: "celebrate", Emoji: "🎉", Gifukai: "", Otaku: "celebrate", Purr: "", Neko: ""},
	{Name: "cheers", Emoji: "🍻", Gifukai: "", Otaku: "cheers", Purr: "", Neko: ""},
	{Name: "clap", Emoji: "👏", Gifukai: "clap", Otaku: "clap", Purr: "", Neko: ""},
	{Name: "claps", Emoji: "👏", Gifukai: "clap", Otaku: "", Purr: "", Neko: ""},
	{Name: "comfy", Emoji: "😌", Gifukai: "", Otaku: "", Purr: "comfy", Neko: ""},
	{Name: "confused", Emoji: "😕", Gifukai: "confused", Otaku: "confused", Purr: "", Neko: ""},
	{Name: "cool", Emoji: "😎", Gifukai: "", Otaku: "cool", Purr: "", Neko: ""},
	{Name: "cry", Emoji: "😭", Gifukai: "cry", Otaku: "cry", Purr: "cry", Neko: ""},
	{Name: "cuddle", Emoji: "🫂", Gifukai: "cuddle", Otaku: "cuddle", Purr: "cuddle", Neko: "cuddle"},
	{Name: "cya", Emoji: "👋", Gifukai: "bye", Otaku: "", Purr: "", Neko: ""},
	{Name: "dance", Emoji: "💃", Gifukai: "dance", Otaku: "dance", Purr: "dance", Neko: ""},
	{Name: "deny", Emoji: "🙅", Gifukai: "nope", Otaku: "", Purr: "", Neko: ""},
	{Name: "drink", Emoji: "🥤", Gifukai: "sip", Otaku: "", Purr: "", Neko: ""},
	{Name: "drool", Emoji: "🤤", Gifukai: "", Otaku: "drool", Purr: "", Neko: ""},
	{Name: "dunno", Emoji: "🤷", Gifukai: "shrug", Otaku: "", Purr: "", Neko: ""},
	{Name: "eat", Emoji: "🍽️", Gifukai: "eat", Otaku: "", Purr: "", Neko: ""},
	{Name: "evillaugh", Emoji: "😈", Gifukai: "", Otaku: "evillaugh", Purr: "", Neko: ""},
	{Name: "facepalm", Emoji: "🤦", Gifukai: "facepalm", Otaku: "facepalm", Purr: "", Neko: ""},
	{Name: "feed", Emoji: "🍽️", Gifukai: "feed", Otaku: "", Purr: "feed", Neko: "feed"},
	{Name: "fluff", Emoji: "☁️", Gifukai: "", Otaku: "", Purr: "fluff", Neko: ""},
	{Name: "flustered", Emoji: "😳", Gifukai: "blush", Otaku: "", Purr: "", Neko: ""},
	{Name: "gaze", Emoji: "👀", Gifukai: "stare", Otaku: "", Purr: "", Neko: ""},
	{Name: "goodbye", Emoji: "👋", Gifukai: "bye", Otaku: "", Purr: "", Neko: ""},
	{Name: "handhold", Emoji: "🤝", Gifukai: "handhold", Otaku: "handhold", Purr: "", Neko: ""},
	{Name: "handshake", Emoji: "🤝", Gifukai: "handshake", Otaku: "", Purr: "", Neko: ""},
	{Name: "happy", Emoji: "😄", Gifukai: "happy", Otaku: "happy", Purr: "", Neko: ""},
	{Name: "headbang", Emoji: "🤘", Gifukai: "", Otaku: "headbang", Purr: "", Neko: ""},
	{Name: "headpat", Emoji: "🫳", Gifukai: "pat", Otaku: "", Purr: "", Neko: ""},
	{Name: "hello", Emoji: "👋", Gifukai: "hi", Otaku: "", Purr: "", Neko: ""},
	{Name: "hey", Emoji: "👋", Gifukai: "hi", Otaku: "", Purr: "", Neko: ""},
	{Name: "hi", Emoji: "👋", Gifukai: "hi", Otaku: "", Purr: "", Neko: ""},
	{Name: "highfive", Emoji: "🙌", Gifukai: "highfive", Otaku: "", Purr: "", Neko: ""},
	{Name: "hug", Emoji: "🤗", Gifukai: "hug", Otaku: "hug", Purr: "hug", Neko: "hug"},
	{Name: "huh", Emoji: "❓", Gifukai: "", Otaku: "huh", Purr: "", Neko: ""},
	{Name: "idk", Emoji: "🤷", Gifukai: "shrug", Otaku: "", Purr: "", Neko: ""},
	{Name: "kabedon", Emoji: "🧱", Gifukai: "wallslam", Otaku: "", Purr: "", Neko: ""},
	{Name: "kick", Emoji: "🦵", Gifukai: "kick", Otaku: "", Purr: "", Neko: ""},
	{Name: "kill", Emoji: "☠️", Gifukai: "kill", Otaku: "", Purr: "", Neko: ""},
	{Name: "kiss", Emoji: "💋", Gifukai: "kiss", Otaku: "kiss", Purr: "kiss", Neko: "kiss"},
	{Name: "lappillow", Emoji: "🛋️", Gifukai: "lappillow", Otaku: "", Purr: "", Neko: ""},
	{Name: "laugh", Emoji: "😂", Gifukai: "laugh", Otaku: "laugh", Purr: "", Neko: ""},
	{Name: "lay", Emoji: "🛌", Gifukai: "", Otaku: "", Purr: "lay", Neko: ""},
	{Name: "lick", Emoji: "👅", Gifukai: "lick", Otaku: "lick", Purr: "lick", Neko: ""},
	{Name: "like", Emoji: "👍", Gifukai: "thumbsup", Otaku: "", Purr: "", Neko: ""},
	{Name: "lmao", Emoji: "😂", Gifukai: "laugh", Otaku: "", Purr: "", Neko: ""},
	{Name: "lol", Emoji: "😂", Gifukai: "laugh", Otaku: "", Purr: "", Neko: ""},
	{Name: "love", Emoji: "❤️", Gifukai: "", Otaku: "love", Purr: "", Neko: ""},
	{Name: "mad", Emoji: "😠", Gifukai: "angry", Otaku: "mad", Purr: "", Neko: ""},
	{Name: "meow", Emoji: "🐱", Gifukai: "nya", Otaku: "", Purr: "", Neko: "meow"},
	{Name: "murder", Emoji: "☠️", Gifukai: "kill", Otaku: "", Purr: "", Neko: ""},
	{Name: "mwah", Emoji: "😘", Gifukai: "blowkiss", Otaku: "", Purr: "", Neko: ""},
	{Name: "nap", Emoji: "😴", Gifukai: "sleep", Otaku: "", Purr: "", Neko: ""},
	{Name: "nervous", Emoji: "😰", Gifukai: "", Otaku: "nervous", Purr: "", Neko: ""},
	{Name: "no", Emoji: "❌", Gifukai: "nope", Otaku: "no", Purr: "", Neko: ""},
	{Name: "nod", Emoji: "🙂", Gifukai: "nod", Otaku: "", Purr: "", Neko: ""},
	{Name: "nom", Emoji: "😋", Gifukai: "eat", Otaku: "nom", Purr: "", Neko: ""},
	{Name: "nope", Emoji: "🙅", Gifukai: "nope", Otaku: "", Purr: "", Neko: ""},
	{Name: "nosebleed", Emoji: "🩸", Gifukai: "", Otaku: "nosebleed", Purr: "", Neko: ""},
	{Name: "nuzzle", Emoji: "🥰", Gifukai: "", Otaku: "nuzzle", Purr: "", Neko: ""},
	{Name: "nya", Emoji: "🐱", Gifukai: "nya", Otaku: "", Purr: "", Neko: ""},
	{Name: "nyah", Emoji: "🐱", Gifukai: "", Otaku: "nyah", Purr: "", Neko: ""},
	{Name: "pat", Emoji: "🫳", Gifukai: "pat", Otaku: "pat", Purr: "pat", Neko: "pat"},
	{Name: "peck", Emoji: "😗", Gifukai: "kiss", Otaku: "", Purr: "", Neko: ""},
	{Name: "peek", Emoji: "👀", Gifukai: "peek", Otaku: "peek", Purr: "", Neko: ""},
	{Name: "pinch", Emoji: "🤏", Gifukai: "", Otaku: "pinch", Purr: "", Neko: ""},
	{Name: "poke", Emoji: "👉", Gifukai: "poke", Otaku: "poke", Purr: "poke", Neko: ""},
	{Name: "pout", Emoji: "😤", Gifukai: "pout", Otaku: "pout", Purr: "pout", Neko: ""},
	{Name: "punch", Emoji: "👊", Gifukai: "punch", Otaku: "punch", Purr: "", Neko: ""},
	{Name: "rage", Emoji: "😠", Gifukai: "angry", Otaku: "", Purr: "", Neko: ""},
	{Name: "roll", Emoji: "🔄", Gifukai: "", Otaku: "roll", Purr: "", Neko: ""},
	{Name: "run", Emoji: "🏃", Gifukai: "run", Otaku: "run", Purr: "", Neko: ""},
	{Name: "sad", Emoji: "😢", Gifukai: "", Otaku: "sad", Purr: "", Neko: ""},
	{Name: "salute", Emoji: "🫡", Gifukai: "salute", Otaku: "", Purr: "", Neko: ""},
	{Name: "scared", Emoji: "😨", Gifukai: "scared", Otaku: "scared", Purr: "", Neko: ""},
	{Name: "shake", Emoji: "🤝", Gifukai: "shake", Otaku: "", Purr: "", Neko: ""},
	{Name: "shocked", Emoji: "😱", Gifukai: "shocked", Otaku: "", Purr: "", Neko: ""},
	{Name: "shoot", Emoji: "🔫", Gifukai: "shoot", Otaku: "", Purr: "", Neko: ""},
	{Name: "shout", Emoji: "📢", Gifukai: "", Otaku: "shout", Purr: "", Neko: ""},
	{Name: "shrug", Emoji: "🤷", Gifukai: "shrug", Otaku: "shrug", Purr: "", Neko: ""},
	{Name: "shy", Emoji: "😳", Gifukai: "shy", Otaku: "shy", Purr: "", Neko: ""},
	{Name: "sigh", Emoji: "😮💨", Gifukai: "", Otaku: "sigh", Purr: "", Neko: ""},
	{Name: "sing", Emoji: "🎤", Gifukai: "sing", Otaku: "sing", Purr: "", Neko: ""},
	{Name: "sip", Emoji: "🥤", Gifukai: "sip", Otaku: "sip", Purr: "", Neko: ""},
	{Name: "slap", Emoji: "✋", Gifukai: "slap", Otaku: "slap", Purr: "slap", Neko: "slap"},
	{Name: "sleep", Emoji: "😴", Gifukai: "sleep", Otaku: "sleep", Purr: "", Neko: ""},
	{Name: "slowclap", Emoji: "👏", Gifukai: "", Otaku: "slowclap", Purr: "", Neko: ""},
	{Name: "smack", Emoji: "👋", Gifukai: "", Otaku: "smack", Purr: "", Neko: ""},
	{Name: "smile", Emoji: "🙂", Gifukai: "smile", Otaku: "smile", Purr: "smile", Neko: ""},
	{Name: "smug", Emoji: "😏", Gifukai: "smug", Otaku: "smug", Purr: "", Neko: "smug"},
	{Name: "sneeze", Emoji: "🤧", Gifukai: "", Otaku: "sneeze", Purr: "", Neko: ""},
	{Name: "snuggle", Emoji: "🫂", Gifukai: "cuddle", Otaku: "", Purr: "", Neko: ""},
	{Name: "sob", Emoji: "😭", Gifukai: "cry", Otaku: "", Purr: "", Neko: ""},
	{Name: "sorry", Emoji: "🙏", Gifukai: "sorry", Otaku: "sorry", Purr: "", Neko: ""},
	{Name: "spin", Emoji: "🌀", Gifukai: "spin", Otaku: "", Purr: "", Neko: ""},
	{Name: "stare", Emoji: "👀", Gifukai: "stare", Otaku: "stare", Purr: "", Neko: ""},
	{Name: "stop", Emoji: "✋", Gifukai: "", Otaku: "stop", Purr: "", Neko: ""},
	{Name: "surprised", Emoji: "😱", Gifukai: "surprised", Otaku: "surprised", Purr: "", Neko: ""},
	{Name: "sweat", Emoji: "💦", Gifukai: "", Otaku: "sweat", Purr: "", Neko: ""},
	{Name: "taunt", Emoji: "😜", Gifukai: "taunt", Otaku: "", Purr: "", Neko: ""},
	{Name: "teehee", Emoji: "😁", Gifukai: "teehee", Otaku: "", Purr: "", Neko: ""},
	{Name: "think", Emoji: "🤔", Gifukai: "think", Otaku: "", Purr: "", Neko: ""},
	{Name: "thinking", Emoji: "🤔", Gifukai: "think", Otaku: "", Purr: "", Neko: ""},
	{Name: "thumbsup", Emoji: "👍", Gifukai: "thumbsup", Otaku: "thumbsup", Purr: "", Neko: ""},
	{Name: "tickle", Emoji: "🤣", Gifukai: "tickle", Otaku: "tickle", Purr: "tickle", Neko: "tickle"},
	{Name: "tired", Emoji: "😫", Gifukai: "tired", Otaku: "tired", Purr: "", Neko: ""},
	{Name: "wag", Emoji: "🐕", Gifukai: "wag", Otaku: "", Purr: "", Neko: ""},
	{Name: "wallslam", Emoji: "🧱", Gifukai: "wallslam", Otaku: "", Purr: "", Neko: ""},
	{Name: "wave", Emoji: "👋", Gifukai: "wave", Otaku: "wave", Purr: "", Neko: ""},
	{Name: "woah", Emoji: "😲", Gifukai: "", Otaku: "woah", Purr: "", Neko: ""},
	{Name: "yawn", Emoji: "🥱", Gifukai: "yawn", Otaku: "yawn", Purr: "", Neko: ""},
	{Name: "yay", Emoji: "🎉", Gifukai: "yay", Otaku: "yay", Purr: "", Neko: ""},
	{Name: "yeet", Emoji: "🚀", Gifukai: "yeet", Otaku: "", Purr: "", Neko: ""},
	{Name: "yes", Emoji: "✅", Gifukai: "nod", Otaku: "yes", Purr: "", Neko: ""},
	{Name: "zzz", Emoji: "😴", Gifukai: "sleep", Otaku: "", Purr: "", Neko: ""},
}

// reactionFind resolves a reaction name to its definition.
func reactionFind(name string) (reactionDef, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, d := range reactionDefs {
		if d.Name == n {
			return d, true
		}
	}
	return reactionDef{}, false
}

// reactionHTTPGet fetches a URL with a byte cap and a browser-ish UA.
func reactionHTTPGet(ctx context.Context, url string, cap int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (GOLD-MD)")
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

// reactionTryProvider fetches JSON from apiURL, extracts the given field
// (url/link) and downloads the GIF bytes.
func reactionTryProvider(ctx context.Context, apiURL, field string) ([]byte, bool) {
	body, ok := reactionHTTPGet(ctx, apiURL, 1<<20)
	if !ok {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false
	}
	u, _ := m[field].(string)
	if u == "" {
		return nil, false
	}
	return reactionHTTPGet(ctx, u, 25<<20)
}

// reactionFetchGif tries every provider in order and returns the first GIF.
// pairing is "m" (solo boy) or "f" (solo girl) for gifukai.
func reactionFetchGif(ctx context.Context, def reactionDef, pairing string) ([]byte, bool) {
	if def.Gifukai != "" {
		if d, ok := reactionTryProvider(ctx, "https://api.gifukai.com/v1/"+def.Gifukai+"?pairing="+pairing, "url"); ok {
			return d, true
		}
		if d, ok := reactionTryProvider(ctx, "https://api.gifukai.com/v1/"+def.Gifukai, "url"); ok {
			return d, true
		}
	}
	if def.Otaku != "" {
		if d, ok := reactionTryProvider(ctx, "https://api.otakugifs.xyz/gif?reaction="+def.Otaku, "url"); ok {
			return d, true
		}
	}
	if def.Purr != "" {
		if d, ok := reactionTryProvider(ctx, "https://api.purrbot.site/v2/img/sfw/"+def.Purr+"/gif", "link"); ok {
			return d, true
		}
	}
	if def.Neko != "" {
		if d, ok := reactionTryProvider(ctx, "https://nekos.life/api/v2/img/"+def.Neko, "url"); ok {
			return d, true
		}
	}
	return nil, false
}

// reactionGifToMp4 converts raw GIF bytes to an mp4 (H.264, yuv420p, even
// dimensions) suitable for WhatsApp GIF playback.
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

// handleReaction deletes the user's command message, fetches the anime GIF,
// converts it and sends it with the "I AM <NAME> <EMOJI>" caption.
func handleReaction(s SessionBridge, info types.MessageInfo, def reactionDef, pairing string) {
	go handleReactionAsync(s, info, def, pairing)
}

func handleReactionAsync(s SessionBridge, info types.MessageInfo, def reactionDef, pairing string) {
	// 1) Delete the user's command message first (owner order).
	_ = s.DeleteMessage(info, info.ID)

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "⚡ *REACTION ERROR ⚡*\n*BOT CLIENT NOT CONNECTED*")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	gifData, ok := reactionFetchGif(ctx, def, pairing)
	if !ok {
		s.Reply(info, "*🎬 REACTION ERROR 🎬*\n*COULD NOT FETCH GIF, TRY AGAIN*")
		return
	}

	mp4, secs, w, h, ok := reactionGifToMp4(ctx, gifData)
	if !ok {
		s.Reply(info, "*🎬 REACTION ERROR 🎬*\n*CONVERSION FAILED, TRY AGAIN*")
		return
	}

	caption := "I AM " + strings.ToUpper(def.Name) + " " + def.Emoji
	_ = s.SendGif(info, mp4, caption, secs, w, h)
}

func init() {
	for _, d := range reactionDefs {
		def := d
		// BREACTION — BOYS (.b<name>)
		Register(Command{
			Name:     "b" + def.Name,
			Category: "BREACTION",
			Desc:     "BOYS " + strings.ToUpper(def.Name) + " ANIME REACTION",
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				handleReaction(s, info, def, "m")
			},
		})
		// GREACTION — GIRLS (.g<name>)
		Register(Command{
			Name:     "g" + def.Name,
			Category: "GREACTION",
			Desc:     "GIRLS " + strings.ToUpper(def.Name) + " ANIME REACTION",
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				handleReaction(s, info, def, "f")
			},
		})
	}
}
