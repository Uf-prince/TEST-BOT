package goldcmds

// ============================================================================
// GOLD-MD — .video3 RAW YouTube Downloader
// File: video3.go
// ============================================================================
// .video3 is the RAW twin of .video (video2.go). It reuses the EXACT same
// turbo engine (3-client Innertube race + loader.to fallback + ffmpeg remux)
// and the EXACT same guard compressor, but the delivery is RAW:
//
//   .video3 <query>  →  ONE WAITING MSG (SEARCHING → EDITED to DOWNLOADING)
//                       →  msg stays visible until download + compress done
//                       →  msg DELETE right before the RAW video is sent
//                       →  DIRECT video arrives with NO thumbnail, NO name,
//                          NO caption, NO footer — just the bare video file.
//
// OWNER ORDER (2026-09-22): query pe sirf EK message aata hai. Wo pehle
// "SEARCHING..." dikhata hai, phir usi message ko EDIT kar ke "DOWNLOADING..."
// bana diya jata hai (delete + naya msg NAHI). Ye message tab tak delete nahi
// hota jab tak video puri tarah download + compress na ho jaye; jab video
// sending pe ho tab ye message delete hota hai aur phir RAW video jata hai.
//
// The compressor system (guard.go) is applied automatically inside
// SendVideoFileRawWait → guardPath(guardVideo) — same as .video / .video2.
// ============================================================================

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// handleVideo3 is the entry point for the .video3 raw command.
// Supports: URL / name search / number pick from session / "hd" quality flag.
func handleVideo3(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeoutCmd(s, info, "VIDEO", "VIDEO3", func(ctx context.Context) {
		handleVideo3Async(ctx, s, info, args, prefix)
	})
}

func handleVideo3Async(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Quick-pick from a video3 search session:
	// args = [URL, thumbnail, title, duration, "HD"?]
	if len(args) >= 4 && (strings.Contains(args[0], "youtube.com/") || strings.Contains(args[0], "youtu.be/")) {
		hd := len(args) >= 5 && strings.EqualFold(strings.TrimSpace(args[4]), "HD")
		picked := &VideoResult{URL: args[0], Thumbnail: args[1], Title: args[2], Duration: args[3]}
		downloadAndSendVideo3(ctx, s, info, args[0], picked, hd, "")
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO3 RAW COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE AT MAX SPEED — RAW (NO THUMBNAIL, NO NAME)* \n*TYPE SAME LIKE THAT* \n*%sVIDEO3 ❮ VIDEO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sVIDEO3 SHAPE OF YOU* \n\n*FOR HD QUALITY TYPE* \n*%sVIDEO3 HD SHAPE OF YOU* \n\n*TYPE COMMAND + VIDEO NAME TO DOWNLOAD VIDEO FROM YOUTUBE*", prefix, prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendVideo3(ctx, s, info, input, nil, false, "")
		return
	}

	// HD quality flag detection: "hd", "720", "1080" anywhere in the input
	hd := false
	words := strings.Fields(input)
	var nameParts []string
	for _, w := range words {
		lw := strings.ToLower(w)
		if lw == "hd" || lw == "720" || lw == "1080" || lw == "720p" || lw == "1080p" {
			hd = true
			continue
		}
		nameParts = append(nameParts, w)
	}
	query := strings.TrimSpace(strings.Join(nameParts, " "))

	if query == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO3 RAW COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE AT MAX SPEED — RAW* \n*TYPE SAME LIKE THAT* \n*%sVIDEO3 ❮ VIDEO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sVIDEO3 HD SHAPE OF YOU*", prefix, prefix))
		return
	}

	// OWNER ORDER (2026-09-21): query pe LIST nahi — yts ka PEHLA result
	// seedha RAW download (koi name, koi thumbnail, koi caption).
	pickFirstVideo3(ctx, s, info, query, hd)
}

// pickFirstVideo3 — OWNER ORDER (2026-09-22): .video3 <query> ab sirf EK
// message bhejta hai. Pehle "SEARCHING ON YOUTUBE....." dikhta hai; search
// complete hone par usi message ko EDIT kar ke "DOWNLOADING VIDEOS FROM
// YOUTUBE....." bana diya jata hai (delete + naya msg NAHI). Phir PEHLA result
// seedha RAW video download ho kar jata hai — koi title, koi thumbnail, koi
// caption, koi footer NAHI. hd flag wahi kaam karta hai (.video3 hd <query> →
// HD merge pipeline). Waiting msg tab tak rehta hai jab tak download + compress
// mukammal na ho, phir delete ho kar RAW video jata hai.
func pickFirstVideo3(ctx context.Context, s SessionBridge, info types.MessageInfo, query string, hd bool) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING ON YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	if len(results) == 0 {
		if waitMsgID != "" {
			s.DeleteMessage(info, waitMsgID)
		}
		video3CmdError(s, info)
		return
	}

	// PEHLA search result → direct raw download (no list, no number pick).
	first := results[0]
	// EDIT the SAME message → DOWNLOADING (no delete, no new msg).
	if waitMsgID != "" {
		s.EditMessage(info, waitMsgID, "*DOWNLOADING VIDEOS FROM YOUTUBE.....*")
	}
	downloadAndSendVideo3(ctx, s, info, first.URL, nil, hd, waitMsgID)
}

// downloadAndSendVideo3 is the RAW turbo pipeline:
// waiting msg (reuse edited msg if provided) → parallel race →
// (HD merge | 360p stream) → remux → compress (guard) → delete waiting →
// RAW video (NO thumbnail, NO caption, NO footer).
//
// waitMsgID: agar non-empty ho to wahi (edited) message reuse hota hai; warna
// ek naya "DOWNLOADING..." message bhej diya jata hai.
func downloadAndSendVideo3(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult, hd bool, waitMsgID string) {
	if waitMsgID == "" {
		waitMsgID = s.ReplyWithID(info, "*DOWNLOADING VIDEOS FROM YOUTUBE.....*")
	}

	clearWait := func() {
		if waitMsgID != "" {
			s.DeleteMessage(info, waitMsgID)
		}
	}

	linkClient := &http.Client{Timeout: 20 * time.Second}
	dlClient := &http.Client{Timeout: yt2DlTimeout}

	// STEP 1: parallel Innertube race (turbo path)
	var st *yt2Stream
	videoID := ytDirectExtractID(videoURL)
	if videoID != "" {
		st = yt2RaceFetch(ctx, videoID, hd)
	}
	// STEP 1b: everything blocked → loader.to last resort (360p)
	if st == nil {
		ws, err := ytLoaderToFallback(ctx, linkClient, videoURL, "360")
		if err != nil || ws == nil || ws.Result.VideoURL == "" {
			clearWait()
			video3CmdError(s, info)
			return
		}
		st = &yt2Stream{title: ws.Metadata.Title, itag18: ws.Result.VideoURL}
	}

	// STEP 2: pick the stream — HD merge when possible, else single 360p stream
	_, vURL := yt2BestVideo(st)
	_, aURL := yt2BestAudio(st)
	useHD := hd && vURL != "" && aURL != ""

	var finalPath string
	var dlErr error
	if useHD {
		finalPath, dlErr = yt2FetchMerged(ctx, dlClient, vURL, aURL)
		// WhatsApp-safe size cap → fall back to 360p for huge HD files
		if dlErr == nil && st.itag18 != "" {
			if fi, err := os.Stat(finalPath); err == nil && fi.Size() > yt2MaxWASend {
				os.Remove(finalPath)
				finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
			}
		}
		// HD pipeline failed → try 360p before giving up
		if dlErr != nil && st.itag18 != "" {
			finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
		}
	} else if st.itag18 != "" {
		finalPath, dlErr = yt2FetchSimple(ctx, dlClient, st.itag18)
	} else if vURL != "" && aURL != "" {
		// no combined format but adaptive available → merge best (rare)
		finalPath, dlErr = yt2FetchMerged(ctx, dlClient, vURL, aURL)
	} else {
		dlErr = fmt.Errorf("no downloadable stream")
	}

	if dlErr != nil || finalPath == "" {
		clearWait()
		video3CmdError(s, info)
		return
	}
	defer os.Remove(finalPath)

	// STEP 3: send RAW video — NO thumbnail, NO name, NO caption, NO footer.
	// The guard compressor runs FIRST inside SendVideoFileRawWait; the waiting
	// message is deleted ONLY after compression is fully done (beforeSend),
	// right before the video is sent.
	if err := s.SendVideoFileRawWait(info, finalPath, clearWait); err != nil {
		video3CmdError(s, info)
	}
}

func init() {
	// Main raw command (visible in menu + count)
	Register(Command{Name: "video3", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD YOUTUBE VIDEOS IN RAW MODE. IT SENDS THE VIDEO DIRECTLY WITH NO THUMBNAIL AND NO NAME.", Run: handleVideo3})
}
