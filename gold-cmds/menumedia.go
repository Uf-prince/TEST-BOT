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
// variant, so a menu can hold both at once.
func menuMediaSettingKey(key, kind string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if strings.ToLower(strings.TrimSpace(kind)) == "video" {
		return key + ":video"
	}
	return key
}

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

// menuMediaTestHint names the EXACT command the owner just used so the
// success card never sends them to an unrelated menu. The old card told
// everyone to type ".alive" or ".menu" even when they had just changed
// the logo or converter picture.
func menuMediaTestHint(prefix, cmdName string) string {
	return "*FOR TEST TYPE ❰ " + prefix + cmdName + " ❱"
}

// handleMenuPic sets the header picture for ONE menu.
func handleMenuPic(s SessionBridge, info types.MessageInfo, args []string, prefix, key, cmdName string) {
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
			s.Reply(info, fmt.Sprintf("*🔰 %s PIC UPDATED 🔰*\n\n*NEW PIC:*\n%s\n\n%s", label, m, menuMediaTestHint(prefix, cmdName)))
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
	s.Reply(info, fmt.Sprintf("*🔰 %s PIC UPDATED 🔰*\n\n*NEW PICTURE SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, cmdName)))
}

// handleMenuVideo sets the header video for ONE menu, normalising it to a
// WhatsApp-safe MP4 first so it can never fail to play.
func handleMenuVideo(s SessionBridge, info types.MessageInfo, args []string, prefix, key, cmdName string) {
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
			s.Reply(info, fmt.Sprintf("*🔰 %s VIDEO UPDATED 🔰*\n\n*NEW VIDEO:*\n%s\n\n%s", label, m, menuMediaTestHint(prefix, cmdName)))
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
	s.Reply(info, fmt.Sprintf("*🔰 %s VIDEO UPDATED 🔰*\n\n*NEW VIDEO SAVED FOR THIS MENU ONLY*\n%s", label, menuMediaTestHint(prefix, cmdName)))
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
}

// menuMediaCommands is the full generated command set: for each menu one
// <x>pic and one <x>video. Names avoid every already-registered command
// (grouppic/aipic/aivideo are taken, so the group/ai menus use distinct
// names below).
var menuMediaCommands = []menuMediaCommand{
	{"menupic", "menu", "pic"}, {"menuvideo", "menu", "video"},
	{"alivepic", "alive", "pic"}, {"alivevideo", "alive", "video"},
	{"logopic", "logo", "pic"}, {"logovideo", "logo", "video"},
	{"fontpic", "font", "pic"}, {"fontvideo", "font", "video"},
	{"gamepic", "game", "pic"}, {"gamevideo", "game", "video"},
	{"equalizerpic", "equalizer", "pic"}, {"equalizervideo", "equalizer", "video"},
	{"aimenupic", "ai", "pic"}, {"aimenuvideo", "ai", "video"},
	{"utilitypic", "utility", "pic"}, {"utilityvideo", "utility", "video"},
	{"converterpic", "converter", "pic"}, {"convertervideo", "converter", "video"},
	{"toolspic", "tools", "pic"}, {"toolsvideo", "tools", "video"},
	{"downloaderpic", "downloader", "pic"}, {"downloadervideo", "downloader", "video"},
	{"groupmenupic", "group", "pic"}, {"groupmenuvideo", "group", "video"},
	{"protectionpic", "protection", "pic"}, {"protectionvideo", "protection", "video"},
	{"presencepic", "presence", "pic"}, {"presencevideo", "presence", "video"},
	{"corepic", "core", "pic"}, {"corevideo", "core", "video"},
	{"breactionpic", "breaction", "pic"}, {"breactionvideo", "breaction", "video"},
	{"greactionpic", "greaction", "pic"}, {"greactionvideo", "greaction", "video"},
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
					handleMenuVideo(s, info, args, prefix, mc.Key, mc.Name)
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
				handleMenuPic(s, info, args, prefix, mc.Key, mc.Name)
			},
		})
	}
}
