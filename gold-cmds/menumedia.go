package goldcmds

// ============================================================================
// GOLD-MD — PER-MENU MEDIA  (.menupic / .menuvideo + one pair per menu)
// File: menumedia.go
// ----------------------------------------------------------------------------
// Sibling of .botpic / .botvideo: instead of changing the media for the WHOLE
// bot, these set the header picture / video for ONE menu at a time — the main
// .menu, the .alive card, and every category menu (.logo, .font, .game,
// .ai, .converter, .tools, ...).
//
//   .menupic                 → guide
//   .menupic reset           → drop the custom picture for that menu
//   .menupic <image-url>     → set from a link
//   (reply/send image + .menupic) → upload and save the URL
//   .menuvideo                → same flow for a video header
//
// Media is stored per menu key in Redis (settings:<botJID> field
// "menumedia:<key>") so a per-menu override survives restarts; when a menu has
// no override the code falls back to the bot-wide .botpic / .botvideo and
// finally to the embedded default.
//
// VIDEOS are always normalised to a WhatsApp-safe MP4 (H.264 + AAC, faststart)
// before the URL is stored — that is what prevents WhatsApp's "can't play this
// video" error, which happens for HEVC/VP9/AV1 or non-faststart files.
//
// Owner-only commands. Hidden aliases work silently (never in the menu).
// ============================================================================

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// menuMediaSpec describes one menu whose header media can be customised: the
// stable storage key plus the labels used in the guide / confirmation card.
type menuMediaSpec struct {
	Key   string // Redis key suffix (menumedia:<key>)
	Label string // human label ("LOGO MENU")
}

// menuMediaSpecs is the full catalogue of customisable menus. Keys MUST match
// the menu-key helper used by the renderers (menuMediaKey) and the menu slugs
// in src/manager.go.
var menuMediaSpecs = []menuMediaSpec{
	{"menu", "MAIN MENU"},
	{"alive", "ALIVE CARD"},
	{"logo", "LOGO MENU"},
	{"font", "FONT MENU"},
	{"game", "GAME MENU"},
	{"equalizer", "EQUALIZER MENU"},
	{"ai", "AI MENU"},
	{"utility", "UTILITY MENU"},
	{"converter", "CONVERTER MENU"},
	{"tools", "TOOLS MENU"},
	{"downloader", "DOWNLOADER MENU"},
	{"group", "GROUP MENU"},
	{"protection", "PROTECTION MENU"},
	{"presence", "PRESENCE MENU"},
	{"core", "OWNER & SYSTEM MENU"},
	{"breaction", "BRACTION MENU"},
	{"greaction", "GREACTION MENU"},
}

// menuMediaLabel returns the guide/confirmation label for a storage key.
func menuMediaLabel(key string) string {
	for _, sp := range menuMediaSpecs {
		if sp.Key == key {
			return sp.Label
		}
	}
	return strings.ToUpper(key) + " MENU"
}

// menuMediaFullLabel returns the uppercased guide label for a storage key
// ("logo" -> "LOGO MENU"). The setter/confirmation cards use this instead of
// the short spec Label so the words "MENU PIC" stay contiguous.
func menuMediaFullLabel(key string) string {
	return strings.ToUpper(menuMediaLabel(key))
}

// menuMediaSettingKey is the single source of truth for the Redis field suffix
// of one menu's media. Pictures use the bare key, videos the "<key>:video"
// variant and voices the "<key>:voice" variant, so a menu can hold all three
// at once.
func menuMediaSettingKey(key, kind string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "video":
		return key + ":video"
	case "voice":
		return key + ":voice"
	}
	return key
}

// MenuMediaVoiceOff is the sentinel stored for a voice field to mean "this
// menu has no voice at all" (distinct from "not set", which falls back to the
// bot-wide voice and then to the built-in default). The owner gets it with
// `.logovoice reset`.
const MenuMediaVoiceOff = "off"

// DefaultMenuVoiceURL is the voice every menu and the .alive card carry when
// the owner has not chosen one — the built-in GOLD-MD intro clip.
const DefaultMenuVoiceURL = "https://d.uguu.se/WHyKzyzH.mp3"

// MenuMediaDefaultVoiceURL is the exported default for the src send path.
func MenuMediaDefaultVoiceURL() string { return DefaultMenuVoiceURL }

// MenuMediaVoiceOffValue is the exported "off" sentinel for the src send path.
func MenuMediaVoiceOffValue() string { return MenuMediaVoiceOff }

// MenuMediaSettingKey is the exported form used by the src renderers, so the
// menu header lookup and the setter can never drift apart.
func MenuMediaSettingKey(key, kind string) string { return menuMediaSettingKey(key, kind) }

// menuMediaFullList renders the owner-facing list of every menu's pic/video
// commands, appended to the .menupic / .menuvideo guide so the owner does not
// need to memorise 34 command names.
func menuMediaFullList(prefix string) string {
	var b strings.Builder
	b.WriteString("\n\n*🔰 ALL MENU MEDIA COMMANDS 🔰*\n")
	for _, mc := range menuMediaCommands {
		b.WriteString(fmt.Sprintf("*❰ %s%s ❱ → %s %s*\n", prefix, mc.Name, menuMediaLabel(mc.Key), strings.ToUpper(mc.Kind)))
	}
	return b.String()
}

// menuMediaGuide is the no-argument help card for a menu. cmdName is the real
// registered command (e.g. "logopic"), so the guide always shows the exact
// command the owner just typed — never a generic ".pic"/".video".
func menuMediaGuide(prefix, label, suffix, cmdName string) string {
	cmd := prefix + cmdName
	return "*🔰 " + label + " " + strings.ToUpper(suffix) + " GUIDE 🔰*\n\n" +
		"*DO YOU WANT TO CHANGE YOUR " + label + " " + strings.ToUpper(suffix) + "*\n\n" +
		"*1❯ SIMPLY SEND YOUR " + strings.ToUpper(suffix) + " HERE*\n" +
		"*2❯ REPLY TO THE " + strings.ToUpper(suffix) + " AND TYPE ❰ " + cmd + " ❱*\n" +
		"*3❯ OR SEND A LINK:*\n" +
		"*❰ " + cmd + " <URL> ❱*\n\n" +
		"*TO REMOVE THE CUSTOM " + strings.ToUpper(suffix) + " TYPE*\n" +
		"*❰ " + cmd + " RESET ❱*"
}

// ── VIDEO NORMALISATION (WhatsApp-safe mp4) ─────────────────────────────────

// menuVideoNormalize converts an arbitrary video file to an H.264 + AAC MP4
// with the moov atom at the front (faststart) and writes it to outPath.
// WhatsApp plays only this fingerprint reliably; HEVC/VP9/AV1 or a trailing
// moov atom is exactly what produces "can't play this video". A clip that
// already looks right is re-muxed without re-encoding (fast).
func menuVideoNormalize(inPath, outPath string) error {
	// Fast path: if it is already H.264, just copy streams into a faststart mp4.
	if menuVideoIsH264(inPath) {
		cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", inPath,
			"-c", "copy", "-movflags", "+faststart", outPath)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	// Full path: re-encode to H.264 + AAC, even dimensions (H.264 requires it).
	cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", inPath,
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "26",
		"-profile:v", "main", "-level", "4.0", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", outPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg normalize failed: %w", err)
	}
	return nil
}

// menuVideoIsH264 reports whether the first video stream is already H.264.
func menuVideoIsH264(path string) bool {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "h264"
}

// menuVideoExt returns a container extension for a video mimetype.
func menuVideoExt(mime string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.Contains(m, "3gp"):
		return ".3gp"
	case strings.Contains(m, "webm"):
		return ".webm"
	case strings.Contains(m, "quicktime"), strings.Contains(m, "mov"):
		return ".mov"
	case strings.Contains(m, "x-matroska"), strings.Contains(m, "mkv"):
		return ".mkv"
	default:
		return ".mp4"
	}
}

// ── HANDLERS ────────────────────────────────────────────────────────────────

// BotVideoNormalizeBytes normalises in-memory video bytes to a WhatsApp-safe
// MP4 (H.264 + AAC, faststart, even dimensions). This is the fix for
// WhatsApp's "can't play this video", triggered by HEVC/VP9/AV1 or a trailing
// moov atom. Invalid/non-video bytes are returned unchanged (caller validated).
func BotVideoNormalizeBytes(data []byte) ([]byte, error) {
	in, err := os.CreateTemp("", "goldmd-norm-in-*")
	if err != nil {
		return data, err
	}
	inPath := in.Name()
	defer os.Remove(inPath)
	if _, err := in.Write(data); err != nil {
		in.Close()
		return data, err
	}
	in.Close()

	outPath := inPath + ".mp4"
	defer os.Remove(outPath)
	if err := menuVideoNormalize(inPath, outPath); err != nil {
		return data, err
	}
	out, err := os.ReadFile(outPath)
	if err != nil || len(out) == 0 {
		return data, err
	}
	return out, nil
}

// fetchBotVideoURLBytes downloads a video URL with the browser UA + Referer the
// hosts expect (qu.ax/catbox hotlink-protect their file endpoints) and a size
// cap, so an HTML share page can never be normalised as a video.
func fetchBotVideoURLBytes(raw string) ([]byte, error) {
	raw = resolveDirectMediaURL(strings.TrimSpace(raw))
	if raw == "" {
		return nil, fmt.Errorf("empty video url")
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	if ref := MediaReferer(raw); ref != "" {
		req.Header.Set("Referer", ref)
	}
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("video fetch HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// menuIsVideoMime reports a genuine video mimetype (not gif/webm stickers).
func menuIsVideoMime(mime string) bool {
	m := strings.ToLower(mime)
	return strings.Contains(m, "video") || strings.Contains(m, "mp4") || strings.Contains(m, "quicktime")
}

// mediaURLRe matches a bare http(s) URL ending in an image or video extension.
var mediaURLRe = regexp.MustCompile(`^(https?://\S+\.(jpe?g|png|gif|webp|mp4|3gp|webm|mov|mkv|avi))$`)

// voiceURLRe matches a bare http(s) URL ending in an audio extension.
var voiceURLRe = regexp.MustCompile(`^(https?://\S+\.(mp3|m4a|aac|ogg|opus|wav|weba))$`)

// menuMediaTestHint tells the owner which command OPENS the menu they just
// customised, so they can actually verify the change: `.menu`, `.logo`,
// `.ai`, `.tools`, ... . The media command itself (`.aimenupic`) only SETS
// the picture and shows a guide — it does not render the menu, so pointing
// the owner at it would be misleading.
func menuMediaTestHint(prefix, menuCmd string) string {
	return "*FOR TEST TYPE ❰ " + prefix + menuCmd + " ❱*"
}

// handleMenuPic sets the header picture for ONE menu.
func handleMenuPic(s SessionBridge, info types.MessageInfo, args []string, prefix, key, cmdName, menuCmd string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	label := menuMediaFullLabel(key)
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.ToLower(argRaw) == "reset" {
		s.SetMenuMediaSetting(menuMediaSettingKey(key, "pic"), "")
		s.Reply(info, fmt.Sprintf("*🔰 %s PIC RESET 🔰*\n\n*CUSTOM PICTURE REMOVED*\n*THIS MENU IS BACK TO THE BOT PIC / DEFAULT*", label))
		return
	}
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := mediaURLRe.FindString(firstTok); m != "" && !isVideoExt(m) {
			s.SetMenuMediaSetting(menuMediaSettingKey(key, "pic"), m)
			s.Reply(info, fmt.Sprintf("*🔰 %s PIC UPDATED 🔰*\n\n*NEW PIC:*\n%s\n\n%s", label, m, menuMediaTestHint(prefix, menuCmd)))
			return
		}
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, fmt.Sprintf("*🔰 INVALID IMAGE LINK 🔰*\n\n*LINK .jpg / .jpeg / .png / .gif / .webp PE KHATAM HONI CHAHIYE*"))
			return
		}
	}

	imgData, ok := s.DownloadImage(info)
	if !ok || len(imgData) == 0 {
		imgData, _, ok = s.DownloadQuotedMedia(info)
	}
	if !ok || len(imgData) == 0 {
		guide := menuMediaGuide(prefix, label, "PIC", cmdName)
		if key == "menu" {
			guide += menuMediaFullList(prefix)
		}
		s.Reply(info, guide)
		return
	}

	waitID := s.ReplyWithID(info, "*🔰 "+label+" PIC UPLOAD HO RAHI HAI...*\n*PROCESSING: 00%*")
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
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING %s PIC*\n*PROCESSING: %02d%%*", label, p))
			}
		}
	}()
	url, err := uploadToImageKit(imgData, fmt.Sprintf("menu-%s-%d.jpg", key, time.Now().UnixMilli()), "/goldmd-menus")
	close(stop)
	s.DeleteMessage(info, waitID)
	if err != nil || url == "" {
		s.Reply(info, fmt.Sprintf("🔰 *Upload fail:* %v", err))
		return
	}
	s.SetMenuMediaSetting(menuMediaSettingKey(key, "pic"), url)
	s.Reply(info, fmt.Sprintf("*🔰 %s PIC UPDATED 🔰*\n\n*NEW PICTURE SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, menuCmd)))
}

// handleMenuVideo sets the header video for ONE menu, normalising it to a
// WhatsApp-safe MP4 first so it can never fail to play.
func handleMenuVideo(s SessionBridge, info types.MessageInfo, args []string, prefix, key, cmdName, menuCmd string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	label := menuMediaFullLabel(key)
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.ToLower(argRaw) == "reset" {
		s.SetMenuMediaSetting(menuMediaSettingKey(key, "video"), "")
		s.Reply(info, fmt.Sprintf("*🔰 %s VIDEO RESET 🔰*\n\n*CUSTOM VIDEO REMOVED*\n*THIS MENU IS BACK TO THE BOT VIDEO / PIC*", label))
		return
	}
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := mediaURLRe.FindString(firstTok); m != "" && isVideoExt(m) {
			s.SetMenuMediaSetting(menuMediaSettingKey(key, "video"), m)
			s.Reply(info, fmt.Sprintf("*🔰 %s VIDEO UPDATED 🔰*\n\n*NEW VIDEO:*\n%s\n\n%s", label, m, menuMediaTestHint(prefix, menuCmd)))
			return
		}
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, "*🔰 INVALID VIDEO LINK 🔰*\n\n*LINK .mp4 / .3gp / .webm / .mov / .mkv PE KHATAM HONI CHAHIYE*")
			return
		}
	}

	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 || !menuIsVideoMime(mime) {
		guide := menuMediaGuide(prefix, label, "VIDEO", cmdName)
		if key == "menu" {
			guide += menuMediaFullList(prefix)
		}
		s.Reply(info, guide)
		return
	}
	if !botVideoIsValidMedia(data, mime) {
		s.Reply(info, "*🔰 INVALID VIDEO 🔰*\n\n*YE FILE ASAL VIDEO NAHI HAI (BROKEN YA HTML PAGE)*\n*DOBARA PROPER VIDEO BHEJ KAR TRY KARO*")
		return
	}

	waitID := s.ReplyWithID(info, "*🔰 "+label+" VIDEO PROCESS HO RAHI HAI...*\n*PROCESSING: 00%*")
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
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING %s VIDEO*\n*PROCESSING: %02d%%*", label, p))
			}
		}
	}()

	// Write the upload to a temp file, normalise to WhatsApp-safe mp4, upload.
	in, err := os.CreateTemp("", "goldmd-menu-in-*"+menuVideoExt(mime))
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

	outPath := inPath + ".norm.mp4"
	defer os.Remove(outPath)
	if err := menuVideoNormalize(inPath, outPath); err != nil {
		// Normalisation failed — fall back to the original bytes rather than
		// refusing the upload outright.
		outPath = inPath
	}
	norm, err := os.ReadFile(outPath)
	if err != nil || len(norm) == 0 {
		norm = data
	}

	url, _, err := uploadAnyHost(norm, "goldmd-menu-"+key+".mp4", "video/mp4")
	close(stop)
	s.DeleteMessage(info, waitID)
	if err != nil || url == "" {
		s.Reply(info, fmt.Sprintf("🔰 *Upload fail:* %v", err))
		return
	}
	if direct := resolveDirectMediaURL(url); direct != "" {
		url = direct
	}
	if !botVideoURLIsPlayable(url) {
		s.Reply(info, "*🔰 VIDEO SETUP FAIL 🔰*\n\n*UPLOADED FILE PLAYABLE VIDEO NAHI NIKLI*\n*DOBARA PROPER MP4 BHEJ KAR TRY KARO*")
		return
	}
	s.SetMenuMediaSetting(menuMediaSettingKey(key, "video"), url)
	s.Reply(info, fmt.Sprintf("*🔰 %s VIDEO UPDATED 🔰*\n\n*NEW VIDEO SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, menuCmd)))
}

// ── VOICE (mp3) NORMALISATION ───────────────────────────────────────────────

// menuVoiceIsAudioMime reports a genuine audio mimetype (mp3/m4a/ogg/opus/wav).
// Videos are accepted too — the handler extracts their audio track.
func menuVoiceIsAudioMime(mime string) bool {
	m := strings.ToLower(mime)
	return strings.Contains(m, "audio") || strings.Contains(m, "mpeg") ||
		strings.Contains(m, "mp3") || strings.Contains(m, "ogg") ||
		strings.Contains(m, "opus") || strings.Contains(m, "wav") ||
		strings.Contains(m, "m4a") || strings.Contains(m, "aac") ||
		strings.Contains(m, "webm")
}

// menuVoiceExt returns a file extension for an audio mimetype.
func menuVoiceExt(mime string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.Contains(m, "ogg"):
		return ".ogg"
	case strings.Contains(m, "opus"):
		return ".opus"
	case strings.Contains(m, "wav"):
		return ".wav"
	case strings.Contains(m, "m4a"), strings.Contains(m, "aac"):
		return ".m4a"
	default:
		return ".mp3"
	}
}

// BotVoiceURLIsPlayable does a ranged GET (browser UA + Referer) and confirms
// the server answers with real audio bytes rather than an HTML share/error
// page. This is what guarantees a stored voice URL will actually play.
func BotVoiceURLIsPlayable(raw string) bool {
	raw = resolveDirectMediaURL(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", browserUA)
	if ref := MediaReferer(raw); ref != "" {
		req.Header.Set("Referer", ref)
	}
	req.Header.Set("Range", "bytes=0-4095")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	buf := make([]byte, 2048)
	n, _ := io.ReadFull(resp.Body, buf)
	if n <= 0 {
		return false
	}
	head := strings.TrimSpace(strings.ToLower(string(buf[:n])))
	if strings.HasPrefix(head, "<") || strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html") {
		return false
	}
	ct := strings.ToLower(http.DetectContentType(buf[:n]))
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "html") {
		return false
	}
	return true
}

// menuVoiceToMP3 converts any media file to a normalised MP3 (mono 64k is
// plenty for a short intro clip and keeps the upload small). Falls back to the
// input file when ffmpeg is unavailable.
func menuVoiceToMP3(inPath string) (string, error) {
	out := inPath + ".voice.mp3"
	cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", inPath,
		"-vn", "-codec:a", "libmp3lame", "-b:a", "96k", "-ar", "44100", "-ac", "2", out)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// menuVoiceGuide is the no-argument help card for one menu's voice command.
func menuVoiceGuide(prefix, label, cmdName string, menuCmd string) string {
	cmd := prefix + cmdName
	return "*🔰 " + label + " VOICE GUIDE 🔰*\n\n" +
		"*DO YOU WANT TO CHANGE YOUR " + label + " VOICE*\n" +
		"*THE VOICE PLAYS RIGHT AFTER THIS MENU IS SENT*\n\n" +
		"*1❯ SIMPLY SEND YOUR AUDIO HERE*\n" +
		"*2❯ REPLY TO THE AUDIO AND TYPE ❰ " + cmd + " ❱*\n" +
		"*3❯ OR SEND A LINK:*\n" +
		"*❰ " + cmd + " <MP3-URL> ❱*\n\n" +
		"*TO REMOVE THE VOICE TYPE*\n" +
		"*❰ " + cmd + " RESET ❱*\n\n" +
		"*FOR TEST TYPE ❰ " + prefix + menuCmd + " ❱*"
}

// handleMenuVoice sets the voice (mp3) that plays right after ONE menu / the
// .alive card is sent. Same setter flow as .botvoice / .botpic / .botvideo.
func handleMenuVoice(s SessionBridge, info types.MessageInfo, args []string, prefix, key, cmdName, menuCmd string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}
	label := menuMediaFullLabel(key)
	argRaw := strings.TrimSpace(strings.Join(args, " "))

	if strings.ToLower(argRaw) == "reset" {
		s.SetMenuMediaSetting(menuMediaSettingKey(key, "voice"), MenuMediaVoiceOff)
		s.Reply(info, fmt.Sprintf("*🔰 %s VOICE RESET 🔰*\n\n*VOICE REMOVED FOR THIS MENU*\n*THIS MENU IS NOW SILENT*", label))
		return
	}
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := voiceURLRe.FindString(firstTok); m != "" {
			if !s.VoiceURLPlayable(m) {
				s.Reply(info, "*🔰 VOICE LINK FAIL 🔰*\n\n*YE LINK SE VOICE NIKAL NAHI PAYI*\n*DOBARA PROPER MP3 LINK YA AUDIO BHEJ KAR TRY KARO*")
				return
			}
			s.SetMenuMediaSetting(menuMediaSettingKey(key, "voice"), m)
			s.Reply(info, fmt.Sprintf("*🔰 %s VOICE UPDATED 🔰*\n\n*NEW VOICE:*\n%s\n\n%s", label, m, menuMediaTestHint(prefix, menuCmd)))
			return
		}
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, "*🔰 INVALID VOICE LINK 🔰*\n\n*LINK .mp3 / .m4a / .ogg / .opus / .wav PE KHATAM HONI CHAHIYE*")
			return
		}
	}

	data, mime, ok := s.DownloadQuotedMedia(info)
	if !ok || len(data) == 0 || !menuVoiceIsAudioMime(mime) {
		guide := menuVoiceGuide(prefix, label, cmdName, menuCmd)
		if key == "menu" {
			guide += menuVoiceFullList(prefix)
		}
		s.Reply(info, guide)
		return
	}

	waitID := s.ReplyWithID(info, "*🔰 "+label+" VOICE UPLOAD HO RAHI HAI...*\n*PROCESSING: 00%*")
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
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING %s VOICE*\n*PROCESSING: %02d%%*", label, p))
			}
		}
	}()

	in, err := os.CreateTemp("", "goldmd-menuvoice-in-*"+menuVoiceExt(mime))
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

	// Normalise to MP3 (also extracts the track when a video was sent).
	outPath, nerr := menuVoiceToMP3(inPath)
	if nerr != nil || outPath == "" {
		// ffmpeg missing/failed — upload the original bytes rather than refuse.
		if up, _, uerr := uploadAnyHost(data, "goldmd-menu-"+key+menuVoiceExt(mime), "audio/mpeg"); uerr == nil && up != "" {
			if direct := resolveDirectMediaURL(up); direct != "" {
				up = direct
			}
			close(stop)
			s.DeleteMessage(info, waitID)
			s.SetMenuMediaSetting(menuMediaSettingKey(key, "voice"), up)
			s.Reply(info, fmt.Sprintf("*🔰 %s VOICE UPDATED 🔰*\n\n*NEW VOICE SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, menuCmd)))
			return
		}
		close(stop)
		s.DeleteMessage(info, waitID)
		s.Reply(info, "🔰 *VOICE CONVERT FAIL*")
		return
	}
	defer os.Remove(outPath)
	norm, rerr := os.ReadFile(outPath)
	if rerr != nil || len(norm) == 0 {
		norm = data
	}

	url, _, uerr := uploadAnyHost(norm, "goldmd-menu-"+key+".mp3", "audio/mpeg")
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
		s.Reply(info, "*🔰 VOICE SETUP FAIL 🔰*\n\n*UPLOADED FILE PLAYABLE AUDIO NAHI NIKLI*\n*DOBARA PROPER MP3 BHEJ KAR TRY KARO*")
		return
	}
	s.SetMenuMediaSetting(menuMediaSettingKey(key, "voice"), url)
	s.Reply(info, fmt.Sprintf("*🔰 %s VOICE UPDATED 🔰*\n\n*NEW VOICE SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, menuCmd)))
}

// menuVoiceFullList lists every per-menu voice command on the .menuvoice guide.
func menuVoiceFullList(prefix string) string {
	var b strings.Builder
	b.WriteString("\n\n*🔰 ALL MENU VOICE COMMANDS 🔰*\n")
	for _, mc := range menuMediaCommands {
		b.WriteString(fmt.Sprintf("*❰ %s%s ❱ → %s VOICE*\n", prefix, menuVoiceCommandName(mc), menuMediaLabel(mc.Key)))
	}
	return b.String()
}

// menuVoiceCommandName maps a pic/video command to its voice sibling
// (logopic/logovideo → logovoice; aimenupic → aimenuvoice).
func menuVoiceCommandName(mc menuMediaCommand) string {
	return mc.VoiceCmd
}

// isVideoExt reports whether a URL ends in a video extension.
func isVideoExt(u string) bool {
	lu := strings.ToLower(u)
	for _, e := range []string{".mp4", ".3gp", ".webm", ".mov", ".mkv", ".avi"} {
		if strings.HasSuffix(lu, e) {
			return true
		}
	}
	return false
}

// menuMediaCommand pairs a command name with a menu key and media kind.
type menuMediaCommand struct {
	Name string
	Key  string
	Kind string // "pic" or "video"
	// MenuCmd is the command the owner types to OPEN this menu (what the
	// success card should tell them to check): ".menu" for the main menu,
	// ".logo" / ".font" / ... for dedicated menus, and the category slug
	// (".ai", ".tools", ...) for the rest. It is NOT the media command.
	MenuCmd string
	// VoiceCmd is the name of this menu's voice sibling (.logovoice,
	// .aimenuvoice, ...). One per menu, registered alongside pic/video.
	VoiceCmd string
}

// menuMediaCommands is the full generated command set: for each menu one
// <x>pic and one <x>video. Names avoid every already-registered command
// (grouppic/aipic/aivideo are taken, so the group/ai menus use distinct
// names below).
var menuMediaCommands = []menuMediaCommand{
	{"menupic", "menu", "pic", "menu", "menuvoice"}, {"menuvideo", "menu", "video", "menu", "menuvoice"},
	{"alivepic", "alive", "pic", "alive", "alivevoice"}, {"alivevideo", "alive", "video", "alive", "alivevoice"},
	{"logopic", "logo", "pic", "logo", "logovoice"}, {"logovideo", "logo", "video", "logo", "logovoice"},
	{"fontpic", "font", "pic", "font", "fontvoice"}, {"fontvideo", "font", "video", "font", "fontvoice"},
	{"gamepic", "game", "pic", "game", "gamevoice"}, {"gamevideo", "game", "video", "game", "gamevoice"},
	{"equalizerpic", "equalizer", "pic", "equalizer", "equalizervoice"}, {"equalizervideo", "equalizer", "video", "equalizer", "equalizervoice"},
	{"aimenupic", "ai", "pic", "ai", "aimenuvoice"}, {"aimenuvideo", "ai", "video", "ai", "aimenuvoice"},
	{"utilitypic", "utility", "pic", "utility", "utilityvoice"}, {"utilityvideo", "utility", "video", "utility", "utilityvoice"},
	{"converterpic", "converter", "pic", "converter", "convertervoice"}, {"convertervideo", "converter", "video", "converter", "convertervoice"},
	{"toolspic", "tools", "pic", "tools", "toolsvoice"}, {"toolsvideo", "tools", "video", "tools", "toolsvoice"},
	{"downloaderpic", "downloader", "pic", "downloader", "downloadervoice"}, {"downloadervideo", "downloader", "video", "downloader", "downloadervoice"},
	{"groupmenupic", "group", "pic", "group", "groupmenuvoice"}, {"groupmenuvideo", "group", "video", "group", "groupmenuvoice"},
	{"protectionpic", "protection", "pic", "protection", "protectionvoice"}, {"protectionvideo", "protection", "video", "protection", "protectionvoice"},
	{"presencepic", "presence", "pic", "presence", "presencevoice"}, {"presencevideo", "presence", "video", "presence", "presencevoice"},
	{"corepic", "core", "pic", "core", "corevoice"}, {"corevideo", "core", "video", "core", "corevoice"},
	{"breactionpic", "breaction", "pic", "breaction", "breactionvoice"}, {"breactionvideo", "breaction", "video", "breaction", "breactionvoice"},
	{"greactionpic", "greaction", "pic", "greaction", "greactionvoice"}, {"greactionvideo", "greaction", "video", "greaction", "greactionvoice"},
}

// menuVoiceCommands is the per-menu voice set, derived from menuMediaCommands
// (one voice command per menu — the VoiceCmd of the pic entry).
func menuVoiceCommands() []menuMediaCommand {
	seen := map[string]bool{}
	var out []menuMediaCommand
	for _, mc := range menuMediaCommands {
		if mc.Kind != "pic" || seen[mc.VoiceCmd] {
			continue
		}
		seen[mc.VoiceCmd] = true
		out = append(out, mc)
	}
	return out
}

// menuMediaVisible are the two headline commands shown in the menu (the other
// 32 stay hidden to keep the menu short). Their guides list every command.
var menuMediaVisible = map[string]bool{"menupic": true, "menuvideo": true}

func init() {
	for _, mc := range menuMediaCommands {
		mc := mc
		hidden := !menuMediaVisible[mc.Name]
		if mc.Kind == "video" {
			Register(Command{
				Name:      mc.Name,
				Category:  "OWNER & SYSTEM",
				Desc:      "THIS COMMAND IS USED TO CHANGE ONE MENU'S VIDEO. REPLY TO A VIDEO AND USE THIS COMMAND.",
				OwnerOnly: true,
				Hidden:    hidden,
				Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
					handleMenuVideo(s, info, args, prefix, mc.Key, mc.Name, mc.MenuCmd)
				},
			})
			continue
		}
		Register(Command{
			Name:      mc.Name,
			Category:  "OWNER & SYSTEM",
			Desc:      "THIS COMMAND IS USED TO CHANGE ONE MENU'S PICTURE. REPLY TO A PHOTO AND USE THIS COMMAND.",
			OwnerOnly: true,
			Hidden:    hidden,
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				handleMenuPic(s, info, args, prefix, mc.Key, mc.Name, mc.MenuCmd)
			},
		})
	}

	// One VOICE command per menu (.menuvoice / .logovoice / .aimenuvoice ...).
	// Hidden: the .menuvoice guide lists them all, so they stay out of the menu.
	for _, mc := range menuVoiceCommands() {
		mc := mc
		Register(Command{
			Name:      mc.VoiceCmd,
			Category:  "OWNER & SYSTEM",
			Desc:      "THIS COMMAND IS USED TO CHANGE ONE MENU'S VOICE (MP3). REPLY TO AN AUDIO AND USE THIS COMMAND. THE VOICE PLAYS RIGHT AFTER THAT MENU IS SENT.",
			OwnerOnly: true,
			Hidden:    true,
			Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
				handleMenuVoice(s, info, args, prefix, mc.Key, mc.VoiceCmd, mc.MenuCmd)
			},
		})
	}
}
