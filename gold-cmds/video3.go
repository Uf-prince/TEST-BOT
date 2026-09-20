package goldcmds

// ============================================================================
// GOLD-MD — .video3 RAW YouTube Downloader
// File: video3.go
// ============================================================================
// .video3 is the RAW twin of .video (video2.go). It reuses the EXACT same
// turbo engine (3-client Innertube race + loader.to fallback + ffmpeg remux)
// and the EXACT same guard compressor, but the delivery is RAW:
//
//   .video3 <query>  →  WAITING MSG  →  (error + success) waiting msg DELETE
//                       →  DIRECT video arrives with NO thumbnail, NO name,
//                          NO caption, NO footer — just the bare video file.
//
// The compressor system (guard.go) is applied automatically inside
// SendVideoFileRaw → guardPath(guardVideo) — same as .video / .video2.
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
		downloadAndSendVideo3(ctx, s, info, args[0], picked, hd)
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 VIDEO3 RAW COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD VIDEOS FROM YOUTUBE AT MAX SPEED — RAW (NO THUMBNAIL, NO NAME)* \n*TYPE SAME LIKE THAT* \n*%sVIDEO3 ❮ VIDEO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sVIDEO3 SHAPE OF YOU* \n\n*FOR HD QUALITY TYPE* \n*%sVIDEO3 HD SHAPE OF YOU* \n\n*TYPE COMMAND + VIDEO NAME TO DOWNLOAD VIDEO FROM YOUTUBE*", prefix, prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendVideo3(ctx, s, info, input, nil, false)
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

	// Search by name → number selection list
	searchProgressVideo3(ctx, s, info, query, hd)
}

// searchProgressVideo3 runs the search and shows the number-selection list.
// The session is tagged "video3" so picks route back through the raw engine
// (with HD when requested).
func searchProgressVideo3(ctx context.Context, s SessionBridge, info types.MessageInfo, query string, hd bool) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING ON YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	s.DeleteMessage(info, waitMsgID)

	if len(results) == 0 {
		video3CmdError(s, info)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*TOP %d RESULTS FOR YOUR SEARCH* \n *%s* \n\n", len(results), query))
	sb.WriteString("*FIRST CHECK THE WHOLE LIST AND TYPE ANY NUMBER 1, OR 6 OR 14 OR 2 OR OTHER ANY NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD FROM YOUTUBE* \n\n")
	if hd {
		sb.WriteString("*🔰 HD MODE ACTIVE — SELECTED VIDEO WILL DOWNLOAD IN HD* \n\n")
	}

	for i, r := range results {
		durStr := r.Duration
		if durStr == "" {
			durStr = "NOT FOUND"
		}
		nameStr := r.Title
		if nameStr == "" {
			nameStr = "NULL"
		}
		linkStr := r.URL
		if linkStr == "" {
			linkStr = "NULL"
		}

		sb.WriteString(fmt.Sprintf("\n*🔰═══════•❖❁❖•═══════🔰* \n*TYPE ❰ %d ❱ TO DOWNLOAD THIS FROM YT* \n", i+1))
		sb.WriteString(fmt.Sprintf("%s \n", nameStr))
		sb.WriteString(fmt.Sprintf("%s \n", linkStr))
		sb.WriteString(fmt.Sprintf("*DURATION :❯ %s* \n*🔰═══════•❖❁❖•═══════🔰* \n\n", durStr))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*TYPE NUMBER WHICH VIDEO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO %d*", len(results)))

	s.SetVideoSession3(info.Sender.String(), results, hd)
	s.Reply(info, sb.String())
}

// downloadAndSendVideo3 is the RAW turbo pipeline:
// waiting msg → parallel race → (HD merge | 360p stream) → remux →
// delete waiting → RAW video (NO thumbnail, NO caption, NO footer).
func downloadAndSendVideo3(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult, hd bool) {
	waitMsgID := s.ReplyWithID(info, "*DOWNLOADING VIDEOS FROM YOUTUBE.....*")

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

	// STEP 3: download complete — delete waiting message (error + success)
	clearWait()

	// STEP 4: send RAW video — NO thumbnail, NO name, NO caption, NO footer.
	// The guard compressor runs inside SendVideoFileRaw (same as .video/.video2).
	if err := s.SendVideoFileRaw(info, finalPath); err != nil {
		video3CmdError(s, info)
	}
}

func init() {
	// Main raw command (visible in menu + count)
	Register(Command{Name: "video3", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO DOWNLOAD YOUTUBE VIDEOS IN RAW MODE. IT SENDS THE VIDEO DIRECTLY WITH NO THUMBNAIL AND NO NAME.", Run: handleVideo3})
}
