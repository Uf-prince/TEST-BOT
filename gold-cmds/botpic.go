package goldcmds

// ============================================================================
// GOLD-MD — BOTPIC Command  (change menu + alive image)
// File: botpic.go
// ----------------------------------------------------------------------------
// Ported from UMAR-MD pair.js (.botpic + aliases).  Text style SAME TO SAME,
// only the 👑 emoji is replaced with 🔰 everywhere.  Work is identical to the
// Node bot:
//
//   .botpic                → guide (send image + mention + type .botpic)
//   .botpic reset          → restore default menu/alive image
//   .botpic <image-url>    → set image directly from a link (must end in
//                            .jpg/.jpeg/.png/.gif/.webp)
//   (reply to / send image + .botpic) → download image, upload to ImageKit,
//                            save the URL, menu + alive images change
//
// Upload target: ImageKit (https://upload.imagekit.io/api/v1/files/upload)
// using a 5-key round-robin pool (private ImageKit API keys, same as Node).
// The resulting URL is stored in Redis settings:<botJID> field "botpic" and
// is read by the menu/alive code (manager.go) to override the default image.
//
// Owner-only command.
// ============================================================================

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// stdBase64 returns the standard base64 encoding of the input bytes.
func stdBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// ── 5-KEY IMAGEKIT POOL (round-robin, same slot style as imagine3.js) ──────
// Source: UMAR-MD pair.js _BP_IK_KEYS (first 5 unique keys).
var botpicIKKeys = []string{
	"private_DnVPwgrPVSazmhkP/5VBGfvmwLM=", // KEY_1
	"private_dmm5ozml0vhBdsigRl/ulx8Qsaw=", // KEY_2
	"private_HPgaDvNAWReEWzrF3FK1lBJf66M=", // KEY_3
	"private_Zuv397aHk3smabM9WwUSyYNHLlg=", // KEY_4
	"private_aJcDCmMItBhFNUCFqnIvPw8m7Ho=", // KEY_5
}

var (
	botpicKeyIdx int
	botpicKeyMu  sync.Mutex
)

// botpicPickKey returns the next ImageKit private key (round-robin).
func botpicPickKey() string {
	botpicKeyMu.Lock()
	defer botpicKeyMu.Unlock()
	if len(botpicIKKeys) == 0 {
		return ""
	}
	k := botpicIKKeys[botpicKeyIdx%len(botpicIKKeys)]
	botpicKeyIdx++
	return k
}

// imageLinkRe matches a bare http(s) image URL that ends in an image extension.
var imageLinkRe = regexp.MustCompile(`^(https?://\S+\.(jpe?g|png|gif|webp))$`)

// uploadToImageKit uploads raw image bytes to ImageKit using the given private
// key and returns the hosted URL. Mirrors the Node bot's FormData upload.
func uploadToImageKit(imgData []byte) (string, error) {
	key := botpicPickKey()
	if key == "" {
		return "", fmt.Errorf("no ImageKit key configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// file field (the image)
	fileName := fmt.Sprintf("botpic-%d.jpg", time.Now().UnixMilli())
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(imgData); err != nil {
		return "", err
	}
	// fileName field (required by ImageKit)
	_ = writer.WriteField("fileName", fileName)
	_ = writer.WriteField("useUniqueFileName", "true")
	// dedicated folder — only botpic uploads live here
	_ = writer.WriteField("folder", "/umar-botpic")
	if err := writer.Close(); err != nil {
		return "", err
	}

	auth := base64BasicAuth(key)
	req, err := http.NewRequest(http.MethodPost, "https://upload.imagekit.io/api/v1/files/upload", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ImageKit returned status %d", resp.StatusCode)
	}

	var parsed struct {
		URL      string `json:"url"`
		FilePath string `json:"filePath"`
		Message  string `json:"message"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to parse ImageKit response: %v", err)
	}
	if parsed.URL != "" {
		return parsed.URL, nil
	}
	if parsed.FilePath != "" {
		return parsed.FilePath, nil
	}
	msg := parsed.Message
	if msg == "" {
		msg = parsed.Error
	}
	if msg == "" {
		msg = "upload failed"
	}
	return "", fmt.Errorf("%s", msg)
}

// base64BasicAuth builds the ImageKit Basic auth token: base64("privateKey:").
func base64BasicAuth(privateKey string) string {
	creds := privateKey + ":"
	return stdBase64([]byte(creds))
}

func handleBotPic(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	argRaw := ""
	if len(args) > 0 {
		argRaw = strings.TrimSpace(strings.Join(args, " "))
	}

	// ── RESET ──
	if strings.ToLower(argRaw) == "reset" {
		s.SetBotPicSetting("")
		s.Reply(info, "*🔰 BOT PIC RESET 🔰*\n\n*DEFAULT IMAGE HAS BEEN RESTORED*\n*MENU AND ALIVE BOTH ARE BACK TO DEFAULT*")
		return
	}

	// ── URL se set (direct link diya) ──
	if argRaw != "" {
		firstTok := strings.Fields(argRaw)[0]
		if m := imageLinkRe.FindString(firstTok); m != "" {
			s.SetBotPicSetting(m)
			s.Reply(info, fmt.Sprintf(
				"*🔰 BOT PIC UPDATED 🔰*\n\n"+
					"*MENU + ALIVE DONO KI IMAGE CHANGE HO GYI*\n\n"+
					"*NEW PIC:*\n%s", m))
			return
		}
		// looks like a URL but not an image extension
		if strings.HasPrefix(strings.ToLower(firstTok), "http") {
			s.Reply(info, "*🔰 INVALID IMAGE LINK 🔰*\n\n*LINK .jpg / .jpeg / .png / .gif / .webp pe khatam honi chahiye*")
			return
		}
	}

	// ── IMAGE DETECTION + DOWNLOAD (direct or quoted) ──
	imgData, ok := s.DownloadImage(info)
	if !ok || len(imgData) == 0 {
		// try quoted media as a fallback
		imgData, _, ok = s.DownloadQuotedMedia(info)
	}
	if !ok || len(imgData) == 0 {
		s.Reply(info, fmt.Sprintf(
			"*🔰 BOT IMAGE CHANGE GUIDE 🔰*\n\n"+
				"*DO YOU WANT TO CHANGE YOUR BOT MENU + ALIVE IMAGES*\n\n"+
				"*1❯ SIMPLEY SEND YOU IMAGE HERE*\n"+
				"*2❯ MENTION THE IMAGE IMPORTANT 🔰*\n"+
				"*3❯ AFTER MENTION THE IMAGE TYPE ❰ %sBOTPIC ❱*\n"+
				"*TO CHANGE THE BOT MENU + ALIVE IMAGES*\n\n"+
				"*TO SET ORIGINAL BOT IMAGE TYPE*\n"+
				"*❰ %sBOTPIC RESET ❱*\n\n"+
				"*TO SET THE ORIGINAL DEFOULT BOT IMAGE*",
			prefix, prefix))
		return
	}

	// ── PROCESSING animation ──
	waitID := s.ReplyWithID(info, "*🔰 BOT PIC IMAGEKIT PE UPLOAD HO RAHI HAI...*\n*PROCESSING: 00%*")

	// ticker to bump the percent while uploading (best-effort, non-blocking)
	stop := make(chan struct{})
	go func() {
		percent := 0
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if percent >= 90 {
					continue
				}
				percent += 7
				if percent > 90 {
					percent = 90
				}
				s.EditMessage(info, waitID, fmt.Sprintf("*CHANGING BOT IMAGE*\n*PROCESSING: %02d%%*", percent))
			}
		}
	}()

	// ── UPLOAD to ImageKit ──
	uploadURL, err := uploadToImageKit(imgData)
	close(stop)
	s.DeleteMessage(info, waitID)

	if err != nil || uploadURL == "" {
		s.Reply(info, fmt.Sprintf("🔰 *Upload fail:* %v", err))
		return
	}

	// ── SAVE URL ──
	s.SetBotPicSetting(uploadURL)

	s.Reply(info, fmt.Sprintf(
		"*🔰 BOT IMAGE UPDATED 🔰*\n\n"+
			"*NEW IMAGE FOR MENU + ALIVE HAS BEEN APPLIED*\n\n"+
			"*FOR TEST TYPE ❰ ALIVE ❱ OR ❰ MENU ❱*"))
}

func init() {
	Register(Command{
		Name:      "botpic",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO CHANGE THE BOT PROFILE PICTURE. REPLY TO A PHOTO AND USE THIS COMMAND.",
		OwnerOnly: true,
		Run:       handleBotPic,
	})
	// hidden aliases (same as Node bot)
	Register(Command{Name: "botdp", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "botphoto", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "botimg", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "botimage", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "dpbot", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "ppbot", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "botpp", OwnerOnly: true, Hidden: true, Run: handleBotPic})
	Register(Command{Name: "picbot", OwnerOnly: true, Hidden: true, Run: handleBotPic})
}
