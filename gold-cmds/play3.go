package goldcmds

// ============================================================================
// GOLD-MD — .play3 RAW YouTube Audio Downloader
// File: play3.go
// ============================================================================
// .play3 is the RAW twin of .play (play2.go). It reuses the EXACT same turbo
// audio engine (3-client Innertube race + loader.to mp3 fallback + 128k MP3
// remux) and the EXACT same guard compressor, but the delivery is RAW:
//
//   .play3 <query>  →  ONE WAITING MSG (SEARCHING → EDITED to DOWNLOADING)
//                      →  msg stays visible until download + compress done
//                      →  msg DELETE right before the RAW audio is sent
//                      →  DIRECT audio arrives with NO thumbnail, NO name,
//                         NO caption, NO footer — just the bare audio file.
//
// OWNER ORDER (2026-09-22): query pe sirf EK message aata hai. Wo pehle
// "SEARCHING..." dikhata hai, phir usi message ko EDIT kar ke "DOWNLOADING..."
// bana diya jata hai (delete + naya msg NAHI). Ye message tab tak delete nahi
// hota jab tak audio puri tarah download + compress na ho jaye; jab audio
// sending pe ho tab ye message delete hota hai aur phir RAW audio jata hai.
//
// The compressor system (guard.go) is applied automatically inside
// SendAudioFileRawWait → guardPath(guardAudio) — same as .play / .play2.
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
		downloadAndSendAudio3(ctx, s, info, args[0], picked, "")
		return
	}

	input := strings.TrimSpace(strings.Join(args, " "))

	if input == "" {
		s.Reply(info, fmt.Sprintf("*🔰 PLAY3 RAW COMMAND FULL GUIDE 🔰* \n\n*DOWNLOAD AUDIOS FROM YOUTUBE AT MAX SPEED — RAW (NO THUMBNAIL, NO NAME)* \n*TYPE SAME LIKE THAT* \n*%sPLAY3 ❮ AUDIO NAME ❯* \n\n*EXAMPLE LIKE THIS* \n*%sPLAY3 SHAPE OF YOU* \n\n*TYPE COMMAND + AUDIO NAME TO DOWNLOAD AUDIO FROM YOUTUBE*", prefix, prefix))
		return
	}

	// Direct YouTube URL → immediate turbo download
	if strings.Contains(input, "youtube.com/") || strings.Contains(input, "youtu.be/") {
		downloadAndSendAudio3(ctx, s, info, input, nil, "")
		return
	}

	// OWNER ORDER (2026-09-21): query pe LIST nahi — yts ka PEHLA result
	// seedha RAW download (koi name, koi thumbnail, koi caption).
	pickFirstPlay3(ctx, s, info, input)
}

// pickFirstPlay3 — OWNER ORDER (2026-09-22): .play3 <query> ab sirf EK message
// bhejta hai. Pehle "SEARCHING AUDIOS FROM YOUTUBE....." dikhta hai; search
// complete hone par usi message ko EDIT kar ke "DOWNLOADING AUDIO FROM
// YOUTUBE....." bana diya jata hai (delete + naya msg NAHI). Phir PEHLA result
// seedha RAW audio download ho kar jata hai — koi title, koi thumbnail, koi
// caption, koi footer NAHI. Waiting msg tab tak rehta hai jab tak download +
// compress mukammal na ho, phir delete ho kar RAW audio jata hai.
func pickFirstPlay3(ctx context.Context, s SessionBridge, info types.MessageInfo, query string) {
	waitMsgID := s.ReplyWithID(info, "*SEARCHING AUDIOS FROM YOUTUBE.....*")

	results := youtubeSearch(ctx, query)

	if len(results) == 0 {
		if waitMsgID != "" {
			s.DeleteMessage(info, waitMsgID)
		}
		play3CmdError(s, info)
		return
	}

	// PEHLA search result → direct raw download (no list, no number pick).
	first := results[0]
	// EDIT the SAME message → DOWNLOADING (no delete, no new msg).
	if waitMsgID != "" {
		s.EditMessage(info, waitMsgID, "*DOWNLOADING AUDIO FROM YOUTUBE.....*")
	}
	downloadAndSendAudio3(ctx, s, info, first.URL, nil, waitMsgID)
}

// downloadAndSendAudio3 is the RAW turbo audio download pipeline:
// 1. Wait msg (reuse edited msg if provided) → 2. 3-client race fetch →
// 3. loader.to fallback (blocked) → 4. fast audio download → 5. 128k MP3 remux
// 6. compress (guard) → 7. delete wait msg → 8. RAW audio (NO thumbnail,
// NO caption, NO footer).
//
// waitMsgID: agar non-empty ho to wahi (edited) message reuse hota hai; warna
// ek naya "DOWNLOADING..." message bhej diya jata hai.
func downloadAndSendAudio3(ctx context.Context, s SessionBridge, info types.MessageInfo, videoURL string, picked *VideoResult, waitMsgID string) {
	if waitMsgID == "" {
		waitMsgID = s.ReplyWithID(info, "*DOWNLOADING AUDIO FROM YOUTUBE.....*")
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

	// STEP 5: send RAW audio — NO thumbnail, NO name, NO caption, NO footer.
	// The guard compressor runs FIRST inside SendAudioFileRawWait; the waiting
	// message is deleted ONLY after compression is fully done (beforeSend),
	// right before the audio is sent.
	if err := s.SendAudioFileRawWait(info, audioPath, clearWait); err != nil {
		play3CmdError(s, info)
	}
}

func init() {
	// Main raw audio command (visible in menu + count)
	Register(Command{Name: "play3", Category: "DOWNLOADER", Desc: "THIS COMMAND IS USED TO PLAY AND DOWNLOAD YOUTUBE SONGS IN RAW MODE. IT SENDS THE AUDIO DIRECTLY WITH NO THUMBNAIL AND NO NAME.", Run: handlePlay3})
}
