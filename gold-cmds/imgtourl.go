package goldcmds

// ============================================================================
// GOLD-MD — Image-to-URL Command (ImgBB upload)
// File: imgtourl.go
// ============================================================================
// COMMAND: .imgtourl   (reply to / send an image)
//   Uploads the image to ImgBB and replies with the hosted URL.
//
// Source: UMAR-MD imgtourl.js  (Node.js / Baileys)
// Converted to Go / whatsmeow for GOLD-MD.
//
// API: https://api.imgbb.com/1/upload?key=<KEY>  (multipart form, field "image")
// Key : d415dbed2b70b808654b120fb0ba1915
//
// Aliases (all Hidden): imgbb, tourl, imgurl, uploadimg, upimg, img2url,
//   imageurl, imgtobb, urlimg, imgupload, linkimg
// ============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const imgbbAPIKey = "d415dbed2b70b808654b120fb0ba1915"
const imgbbEndpoint = "https://api.imgbb.com/1/upload"

// imgbbResponse models the relevant fields of the ImgBB JSON response.
type imgbbResponse struct {
	Success bool `json:"success"`
	Data    struct {
		URL string `json:"url"`
	} `json:"data"`
}

func handleImgToURL(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleImgToURLAsync(s, info, args, prefix)
}

func handleImgToURLAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Grab the image bytes via the bridge (handles direct, view-once & quoted).
	imgData, ok := s.DownloadImage(info)
	if !ok || len(imgData) == 0 {
		s.Reply(info, "🔰 Image send karo ya UmarReply karo — *"+prefix+"imgbb*")
		return
	}

	waitID := s.ReplyWithID(info, "*UPLOADING IMAGE....*")

	url, err := uploadToImgBB(imgData)
	s.DeleteMessage(info, waitID)
	if err != nil {
		s.Reply(info, "🔰 Upload failed: "+err.Error())
		return
	}
	if url == "" {
		s.Reply(info, "🔰 Upload failed: ImgBB returned an empty URL.")
		return
	}

	s.Reply(info, fmt.Sprintf("*IMAGE UPLOADED SUCCESS* 🔰\n\n*IMAGE LINK IS BELOW*\n%s", url))
}

// uploadToImgBB uploads raw image bytes to ImgBB and returns the hosted URL.
func uploadToImgBB(imgData []byte) (string, error) {
	// ImgBB accepts multipart/form-data with an "image" file field.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "upload.jpg")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(imgData); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s?key=%s", imgbbEndpoint, imgbbAPIKey)
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ImgBB returned status %d", resp.StatusCode)
	}

	var parsed imgbbResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to parse ImgBB response: %v", err)
	}
	if !parsed.Success || parsed.Data.URL == "" {
		return "", fmt.Errorf("ImgBB success=false or empty URL")
	}
	return parsed.Data.URL, nil
}

func init() {
	Register(Command{Name: "imgtourl", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO CONVERT ANY IMAGE INTO A LINK. REPLY TO A PHOTO AND THE BOT GIVES YOU A DIRECT LINK OF IT.", Run: handleImgToURL})
	Register(Command{Name: "imgbb", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "tourl", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "imgurl", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "uploadimg", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "upimg", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "img2url", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "imageurl", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "imgtobb", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "urlimg", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "imgupload", Hidden: true, Run: handleImgToURL})
	Register(Command{Name: "linkimg", Hidden: true, Run: handleImgToURL})
}
