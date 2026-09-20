package goldcmds

// ============================================================================
// GOLD-MD — .play3 RAW YouTube Audio Downloader
// File: play3.go
// ============================================================================
// .play3 is the RAW twin of .play (play2.go). It reuses the EXACT same turbo
// audio engine (3-client Innertube race + loader.to mp3 fallback + 128k MP3
// remux) and the EXACT same guard compressor, but the delivery is RAW:
//
//   .play3 <query>  →  WAITING MSG  →  (error + success) waiting msg DELETE
//                      →  DIRECT audio arrives with NO thumbnail, NO name,
//                         NO caption, NO footer — just the bare audio file.
//
// The compressor system (guard.go) is applied automatically inside
// SendAudioFileRaw → guardPath(guardAudio) — same as .play / .play2.
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

// handlePlay3 is the entry point for the .play3 raw audio command.
func handlePlay3(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeoutCmd(s, info, "PLAY", "PLAY3", func(ctx context.Context) {
		handlePlay3Async(ctx, s, info, args, prefix)
	})
}

func handlePlay3Async(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Quick-pick from a play3 search session:
	// args = [URL, thumbnail, title, duration]
	if len(args) >= 4 && (strings.Contains(args[0], "youtube.com/") || strings.Contains(args[0], "youtu.be/")) {
		picked := &VideoResult{URL: args[0], Thumbnail: args[1], Title: args[2], Duration: args[3]}
		downloadAndSendAudio3(ctx, s, info, args[0], picked)
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 PLAY3 RAW COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD AUDIOS FROM YOUTUBE AT MAX SPEED — RAW (NO THUMBNAIL, NO NAME)* \n*TYPE SAME LIKE THAT* \n*%sPLAY3 ❮ AUDIO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sPLAY3 SHAPE OF YOU* \n\n*TYPE COMMAND + AUDIO NAME TO DOWNLOAD AUDIO FROM YOUTUBE*", prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendAudio3(ctx, s, info, input, nil)
		return
	}

	// Search by name → number selection list
	searchProgressPlay3(ctx, s, info, input)
}

// searchProgressPlay3 runs the search and shows the number-selection list.
// The audio session is tagged "play3" so picks route back through the raw engine.
func searchProgressPlay3(ctx context.Context, s SessionBridge, info types.MessageInfo, query string) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING AUDIOS FROM YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	if waitMsgID != "" {
		s.DeleteMessage(info, waitMsgID)
	}

	if len(results) == 0 {
		play3CmdError(s, info)
		return
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*TOP %d RESULTS FOR YOUR SEARCH* \n *%s*\n\n", len(results), query))
	b.WriteString("*FIRST CHECK THE WHOLE LIST AND TYPE ANY NUMBER 1, OR 6 OR 14 OR 2 OR OTHER ANY NUMBER WHICH AUDIO DO YOU WANT TO DOWNLOAD FROM YOUTUBE*\n\n")
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

		b.WriteString(fmt.Sprintf("\n*✧═══════════•❁❀❁•═══════════✧*\n*TYPE ❰ %d ❱ TO DOWNLOAD THIS FROM YT*\n", i+1))
		b.WriteString(fmt.Sprintf("%s\n", strings.ToUpper(nameStr)))
		b.WriteString(fmt.Sprintf("%s\n", linkStr))
		b.WriteString(fmt.Sprintf("*DURATION :❯ %s*\n*✧═══════════•❁❀❁•═══════════✧*\n\n", strings.ToUpper(durStr)))
		b.WriteString("\n")
	}
	b.WriteString("*TYPE NUMBER WHICH AUDIO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO 15*")
	s.SetAudioSession3(info.Sender.String(), results)
	s.Reply(info, b.String())
}

// downloadAndSendAudio3 is the RAW turbo audio download pipeline:
// 1. Wait msg → 2. 3-client race fetch → 3. loader.to fallback (blocked)
// 4. fast audio download → 5. 128k MP3 remux → 6. delete wait
// 7. RAW audio (NO thumbnail, NO caption, NO footer).
func downloadAndSendAudio3(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult) {
	waitMsgID := s.ReplyWithID(info, "*DOWNLOADING AUDIO FROM YOUTUBE.....*")

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
		st = yt2RaceFetch(ctx, videoID, false)
	}
	// STEP 1b: everything blocked → loader.to mp3 fallback
	if st == nil {
		ws, err := ytLoaderToFallback(ctx, linkClient, videoURL, "mp3")
		if err != nil || ws == nil || ws.Result.AudioURL == "" {
			clearWait()
			play3CmdError(s, info)
			return
		}
		st = &yt2Stream{title: ws.Metadata.Title, itag18: ws.Result.AudioURL}
	}

	// STEP 2: pick the audio stream — itag 140 (AAC 128k) preferred, 139 fallback.
	// If neither adaptive audio stream is available (e.g. loader.to path), the
	// pre-loaded itag18 fallback URL is used directly.
	_, aURL := yt2BestAudio(st)
	if aURL == "" && st.itag18 != "" {
		aURL = st.itag18 // loader.to already gave us a direct mp3 URL
	}
	if aURL == "" {
		clearWait()
		play3CmdError(s, info)
		return
	}

	// STEP 3: fast single-stream download (102 MB/s proven — no chunking needed)
	rawAudioPath, err := streamDownloadToFile(ctx, dlClient, aURL, nil)
	if err != nil {
		// adaptive audio failed → try loader.to before giving up
		ws, ferr := ytLoaderToFallback(ctx, linkClient, videoURL, "mp3")
		if ferr == nil && ws != nil && ws.Result.AudioURL != "" {
			rawAudioPath, err = streamDownloadToFile(ctx, dlClient, ws.Result.AudioURL, nil)
		}
		if err != nil {
			clearWait()
			play3CmdError(s, info)
			return
		}
	}
	defer os.Remove(rawAudioPath)

	// STEP 4: 128kbps MP3 lock (same as .play — 44.1kHz stereo)
	audioPath, convErr := audioRemuxFile(ctx, rawAudioPath)
	if convErr != nil {
		audioPath = rawAudioPath // fallback: send raw audio if ffmpeg fails
	} else {
		defer os.Remove(audioPath)
	}

	// STEP 5: delete waiting message (error + success)
	clearWait()

	// STEP 6: send RAW audio — NO thumbnail, NO name, NO caption, NO footer.
	// The guard compressor runs inside SendAudioFileRaw (same as .play/.play2).
	if err := s.SendAudioFileRaw(info, audioPath); err != nil {
		play3CmdError(s, info)
	}
}

func init() {
	// Main raw audio command (visible in menu + count)
	Register(Command{Name: "play3", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO PLAY AND DOWNLOAD YOUTUBE SONGS IN RAW MODE. IT SENDS THE AUDIO DIRECTLY WITH NO THUMBNAIL AND NO NAME.", Run: handlePlay3})
}
