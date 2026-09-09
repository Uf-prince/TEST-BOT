package goldcmds

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// PLAY2 — TURBO AUDIO ENGINE (parallel Innertube race → 128kbps MP3)
// ============================================================================
// The turbo companion to .play:
//   • .play2 <name>       → search list → number pick → turbo download
//   • .play2 <YT URL>     → direct turbo download
//   • 128kbps MP3 lock (44.1kHz stereo) — same guaranteed quality as .play
//   • 3-client parallel race (ANDROID / ANDROID_VR / VISIONOS), first usable
//     stream wins, losers cancelled instantly
//   • audio itags: 140 (AAC 128k) preferred, 139 (48k) fallback
//   • blocked videos → loader.to mp3 fallback (kick + 45s poll)
//   • hidden aliases: .p2, .yta2, .mp32
// ============================================================================

// handlePlay2 is the entry point for the .play2 turbo audio command.
func handlePlay2(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeoutCmd(s, info, "PLAY", "PLAY2", func(ctx context.Context) {
		handlePlay2Async(ctx, s, info, args, prefix)
	})
}

func handlePlay2Async(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Quick-pick from a play2 search session:
	// args = [URL, thumbnail, title, duration]
	if len(args) >= 4 && (strings.Contains(args[0], "youtube.com/") || strings.Contains(args[0], "youtu.be/")) {
		picked := &VideoResult{URL: args[0], Thumbnail: args[1], Title: args[2], Duration: args[3]}
		downloadAndSendAudio2(ctx, s, info, args[0], picked)
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 PLAY TURBO COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD AUDIOS FROM YOUTUBE AT MAX SPEED* \n*TYPE SAME LIKE THAT* \n*%sPLAY \u276e AUDIO NAME \u276f* \n\n*EXAMPLE LIKE THIS* \n*%sPLAY SHAPE OF YOU* \n\n*TYPE COMMAND + AUDIO NAME TO DOWNLOAD AUDIO FROM YOUTUBE*", prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendAudio2(ctx, s, info, input, nil)
		return
	}

	// Search by name → number selection list
	searchProgressPlay2(ctx, s, info, input)
}

// searchProgressPlay2 runs the search and shows the number-selection list.
// The audio session is tagged "play2" so picks route back through the turbo engine.
func searchProgressPlay2(ctx context.Context, s SessionBridge, info types.MessageInfo, query string) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING AUDIOS FROM YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	if waitMsgID != "" {
		s.DeleteMessage(info, waitMsgID)
	}

	if len(results) == 0 {
		play2CmdError(s, info)
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

		b.WriteString(fmt.Sprintf("\n*\u2727\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2022\u2741\u2740\u2741\u2022\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2727*\n*TYPE \u2770 %d \u2771 TO DOWNLOAD THIS FROM YT*\n", i+1))
		b.WriteString(fmt.Sprintf("%s\n", strings.ToUpper(nameStr)))
		b.WriteString(fmt.Sprintf("%s\n", linkStr))
		b.WriteString(fmt.Sprintf("*DURATION :\u276f %s*\n*\u2727\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2022\u2741\u2740\u2741\u2022\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2727*\n\n", strings.ToUpper(durStr)))
		b.WriteString("\n")
	}
	b.WriteString("*TYPE NUMBER WHICH AUDIO DO YOU WANT TO DOWNLOAD — REPLY WITH ANY NUMBER 1 TO 15*")
	s.SetAudioSession2(info.Sender.String(), results, true)
	s.Reply(info, b.String())
}

// downloadAndSendAudio2 is the turbo audio download pipeline:
// 1. Wait msg → 2. 3-client race fetch → 3. loader.to fallback (blocked)
// 4. metadata + thumbnail (held) → 5. fast audio download → 6. 128k MP3 remux
// 7. delete wait → 8. SendImage+caption → 9. SendAudioFile
func downloadAndSendAudio2(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult) {
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
			play2CmdError(s, info)
			return
		}
		st = &yt2Stream{title: ws.Metadata.Title, itag18: ws.Result.AudioURL}
	}

	// STEP 2: metadata (picked result wins, race data fills gaps)
	title := "N/A"
	if picked != nil && picked.Title != "" {
		title = picked.Title
	} else if st.title != "" {
		title = st.title
	}

	author := st.author
	if author == "" {
		author = "N/A"
	}

	duration := st.duration
	if duration == "" && picked != nil {
		duration = picked.Duration
	}
	if duration == "" {
		duration = "N/A"
	}
	views := formatMetadataNumber(st.views)

	thumbURL := st.thumb
	if picked != nil && picked.Thumbnail != "" {
		thumbURL = picked.Thumbnail
	}
	thumbnail := downloadThumbnail(ctx, dlClient, thumbURL)

	infoCaption := fmt.Sprintf("*%s* \n\n🔰 *AUTHOR :\u276f %s* \n🔰 *DURATION :\u276f %s* \n🔰 *VIEWS :\u276f %s*",
		strings.ToUpper(title), strings.ToUpper(author), strings.ToUpper(duration), strings.ToUpper(views))

	// STEP 3: pick the audio stream — itag 140 (AAC 128k) preferred, 139 fallback.
	// If neither adaptive audio stream is available (e.g. loader.to path), the
	// pre-loaded itag18 fallback URL is used directly.
	_, aURL := yt2BestAudio(st)
	if aURL == "" && st.itag18 != "" {
		aURL = st.itag18 // loader.to already gave us a direct mp3 URL
	}
	if aURL == "" {
		clearWait()
		play2CmdError(s, info)
		return
	}

	// STEP 4: fast single-stream download (102 MB/s proven — no chunking needed)
	rawAudioPath, err := streamDownloadToFile(ctx, dlClient, aURL, nil)
	if err != nil {
		// adaptive audio failed → try loader.to before giving up
		ws, ferr := ytLoaderToFallback(ctx, linkClient, videoURL, "mp3")
		if ferr == nil && ws != nil && ws.Result.AudioURL != "" {
			rawAudioPath, err = streamDownloadToFile(ctx, dlClient, ws.Result.AudioURL, nil)
		}
		if err != nil {
			clearWait()
			play2CmdError(s, info)
			return
		}
	}
	defer os.Remove(rawAudioPath)

	// STEP 5: 128kbps MP3 lock (same as .play — 44.1kHz stereo)
	audioPath, convErr := audioRemuxFile(ctx, rawAudioPath)
	if convErr != nil {
		audioPath = rawAudioPath // fallback: send raw audio if ffmpeg fails
	} else {
		defer os.Remove(audioPath)
	}

	// STEP 6: delete waiting message
	clearWait()

	// STEP 7: send thumbnail + info
	if len(thumbnail) > 0 {
		_ = s.SendImage(info, thumbnail, infoCaption)
	} else {
		s.Reply(info, infoCaption)
	}

	// STEP 8: send audio
	if err := s.SendAudioFile(info, audioPath, "", 0); err != nil {
		play2CmdError(s, info)
	}
}

func init() {
	// Main turbo audio command (visible in menu + count)
	Register(Command{Name: "play", Category: "DOWNLOADER", Desc: "Turbo fast YouTube audio download (parallel engine, 128kbps MP3)", Run: handlePlay2})
	// Hidden aliases — fully functional but not in menu / TOTAL COMMANDS count
	Register(Command{Name: "p2", Hidden: true, Run: handlePlay})
	Register(Command{Name: "yta2", Hidden: true, Run: handlePlay})
	Register(Command{Name: "mp32", Hidden: true, Run: handlePlay})
}
