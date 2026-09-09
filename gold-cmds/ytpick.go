package goldcmds

// ============================================================================
// GOLD-MD — YT Pick Flow (number → thumbnail → 1=AUDIO / 2=VIDEO)
// File: ytpick.go
// ============================================================================
// After a .yts search card the user types a bare number. Instead of the old
// video-session dispatch (which silently downloaded a VIDEO), the pick now:
//
//   1. downloads NOTHING yet — fetches the thumbnail,
//   2. sends it with  *TYPE ❮ 1 ❯ FOR AUDIO*  /  *TYPE ❮ 2 ❯ FOR VIDEO*,
//   3. stores a "choice window" for that sender,
//   4. on "1" → audio (turbo play → classic play2 engines, fallback chain),
//      on "2" → video (turbo video → classic video2 engines, fallback chain).
//
// The choice window lives in this package (no bridge round-trip needed):
// same 2-minute TTL pattern as the search pick sessions.
// ============================================================================

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── choice session store ───────────────────────────────────────────────

type ytsChoiceSession struct {
	Vid    VideoResult
	Expiry time.Time
}

var (
	ytsChoiceMu   sync.Mutex
	ytsChoiceSess = map[string]*ytsChoiceSession{}
	ytsChoiceTTL  = 2 * time.Minute
)

// StoreYTSChoice stores the pending 1=AUDIO / 2=VIDEO choice window.
func StoreYTSChoice(jid string, vid VideoResult) {
	ytsChoiceMu.Lock()
	defer ytsChoiceMu.Unlock()
	ytsChoiceSess[jid] = &ytsChoiceSession{Vid: vid, Expiry: time.Now().Add(ytsChoiceTTL)}
}

// clearYTSChoice kills the choice window.
func clearYTSChoice(jid string) {
	ytsChoiceMu.Lock()
	defer ytsChoiceMu.Unlock()
	delete(ytsChoiceSess, jid)
}

// HasPendingYTSChoice reports whether a sender has an active choice window.
func HasPendingYTSChoice(sender string) bool {
	ytsChoiceMu.Lock()
	defer ytsChoiceMu.Unlock()
	sess, ok := ytsChoiceSess[sender]
	if !ok {
		return false
	}
	if time.Now().After(sess.Expiry) {
		delete(ytsChoiceSess, sender)
		return false
	}
	return true
}

// ── .yts list-pick session (search-card number → thumbnail + ask) ──

// ytsListSession is the .yts search-card pick window for one sender.
type ytsListSession struct {
	Vids   []VideoResult
	Expiry time.Time
}

var (
	ytsListMu   sync.Mutex
	ytsListSess = map[string]*ytsListSession{}
)

// StoreYTSList stores the .yts search results so a bare number pick routes
// into the thumbnail + 1=AUDIO / 2=VIDEO flow (replaces the old raw
// video-session dispatch that silently downloaded a video).
func StoreYTSList(jid string, vids []VideoResult) {
	clearSearchSession(jid)
	clearYTSChoice(jid)
	ytsListMu.Lock()
	defer ytsListMu.Unlock()
	ytsListSess[jid] = &ytsListSession{Vids: vids, Expiry: time.Now().Add(ytsChoiceTTL)}
}

// ClearYTSList kills a pending .yts list-pick window (bridge-callable).
func ClearYTSList(jid string) { clearYTSList(jid) }

// clearYTSList kills a pending .yts list-pick window.
func clearYTSList(jid string) {
	ytsListMu.Lock()
	defer ytsListMu.Unlock()
	delete(ytsListSess, jid)
}

// getYTSList returns the live .yts list session or nil.
func getYTSList(jid string) *ytsListSession {
	ytsListMu.Lock()
	defer ytsListMu.Unlock()
	sess, ok := ytsListSess[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		delete(ytsListSess, jid)
		return nil
	}
	return sess
}

// ytsAskFooter is the ask block appended to the thumbnail caption.
func ytsAskFooter() string {
	return "\n\n*TYPE ❮ 1 ❯ FOR AUDIO*\n*TYPE ❮ 2 ❯ FOR VIDEO*"
}

// ytPickAskFooter is the ask block appended to the .yts search-card footer
// (before any pick happens).
func ytPickAskFooter() string {
	return "*PICK A NUMBER FIRST — THEN TYPE ❮ 1 ❯ FOR AUDIO OR ❮ 2 ❯ FOR VIDEO*"
}

// ── pick action ────────────────────────────────────────────────────────

// ytPickThumbnail runs when the user picks a number on a .yts card:
// downloads the thumbnail and asks AUDIO or VIDEO — no engine download yet.
func ytPickThumbnail(ctx context.Context, s SessionBridge, info types.MessageInfo, vid VideoResult) {
	waitID := s.ReplyWithID(info, "*FETCHING VIDEO INFO.....*")

	dlClient := &http.Client{Timeout: 30 * time.Second}
	thumb := downloadThumbnail(ctx, dlClient, vid.Thumbnail)

	s.DeleteMessage(info, waitID)

	caption := fmt.Sprintf("*%s*", strings.ToUpper(vid.Title))
	if vid.Duration != "" {
		caption += fmt.Sprintf("\n\n🔰 *DURATION :❯ %s*", strings.ToUpper(vid.Duration))
	}
	caption += ytsAskFooter()

	if len(thumb) > 0 {
		if err := s.SendImage(info, thumb, caption); err != nil {
			s.Reply(info, caption)
		}
	} else {
		s.Reply(info, caption)
	}

	// choice window: 1 → audio, 2 → video
	StoreYTSChoice(info.Sender.String(), vid)
}

// YTSTryHandleChoice — call from the main message handler BEFORE the search
// pick router. A bare "1" / "2" (or ".1" / ".2") after a .yts pick sends the
// audio or video directly — both-engine fallback, no guidance messages.
func YTSTryHandleChoice(s SessionBridge, info types.MessageInfo, body, prefix string) bool {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, prefix) {
		trimmed = strings.TrimSpace(trimmed[len(prefix):])
	}
	var num int
	_, numErr := fmt.Sscanf(trimmed, "%d", &num)
	isNum := numErr == nil

	// 1) .yts LIST PICK — a bare number on a .yts search card sends the
	// thumbnail and asks 1=AUDIO / 2=VIDEO (no engine download yet).
	if isNum && num >= 1 {
		if list := getYTSList(info.Sender.String()); list != nil && num <= len(list.Vids) {
			vid := list.Vids[num-1]
			clearYTSList(info.Sender.String())
			clearYTSChoice(info.Sender.String())
			RunWithTimeout(s, info, func(ctx context.Context) {
				ytPickThumbnail(ctx, s, info, vid)
			})
			return true
		}
	}

	// 2) AUDIO / VIDEO CHOICE — a bare "1"/"2" after the thumbnail sends the
	// audio (play turbo → play2 classic) or the video (video turbo →
	// video2 classic) directly, thumbnail already shown.
	sess := getPickableYTSChoice(info.Sender.String())
	if sess == nil {
		return false
	}
	if !isNum || (num != 1 && num != 2) {
		// Not a 1/2 reply — the choice window stays alive (2-min expiry) and
		// the message is NOT swallowed: commands still run.
		return false
	}

	vid := sess.Vid
	clearYTSChoice(info.Sender.String())
	clearSearchSession(info.Sender.String())

	switch num {
	case 1:
		ytsRunAudio(s, info, vid, prefix)
	case 2:
		ytsRunVideo(s, info, vid, prefix)
	}
	return true
}

// getPickableYTSChoice returns the live choice session or nil.
func getPickableYTSChoice(jid string) *ytsChoiceSession {
	ytsChoiceMu.Lock()
	defer ytsChoiceMu.Unlock()
	sess, ok := ytsChoiceSess[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		delete(ytsChoiceSess, jid)
		return nil
	}
	return sess
}

// ── engine runners (both-engine fallback) ──────────────────────────────

// ytsRunAudio: turbo play engine first, classic play2 as fallback.
// The thumbnail + info was already sent — the engines must NOT re-send it,
// so each engine runs in "quick-pick" mode: download + send the media only.
func ytsRunAudio(s SessionBridge, info types.MessageInfo, vid VideoResult, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if ytAudioEngineTurbo(ctx, s, info, vid) {
			return
		}
		if ytAudioEngineClassic(ctx, s, info, vid) {
			return
		}
		s.Reply(info, "*❌ DOWNLOAD FAILED — PLEASE TRY AGAIN LATER*")
	})
}

// ytsRunVideo: turbo video engine first, classic video2 as fallback.
func ytsRunVideo(s SessionBridge, info types.MessageInfo, vid VideoResult, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if ytVideoEngineTurbo(ctx, s, info, vid) {
			return
		}
		if ytVideoEngineClassic(ctx, s, info, vid) {
			return
		}
		s.Reply(info, "*❌ DOWNLOAD FAILED — PLEASE TRY AGAIN LATER*")
	})
}

// ytAudioEngineTurbo: yt2RaceFetch → itag140/139 audio → audioRemuxFile → send.
// Returns true when the audio was sent.
func ytAudioEngineTurbo(ctx context.Context, s SessionBridge, info types.MessageInfo, vid VideoResult) bool {
	dlClient := &http.Client{Timeout: yt2DlTimeout}

	var st *yt2Stream
	videoID := ytDirectExtractID(vid.URL)
	if videoID != "" {
		st = yt2RaceFetch(ctx, videoID, false)
	}
	if st == nil {
		return false
	}
	_, aURL := yt2BestAudio(st)
	if aURL == "" && st.itag18 != "" {
		aURL = st.itag18
	}
	if aURL == "" {
		return false
	}
	rawPath, err := streamDownloadToFile(ctx, dlClient, aURL, nil)
	if err != nil {
		return false
	}
	defer os.Remove(rawPath)

	audioPath, convErr := audioRemuxFile(ctx, rawPath)
	if convErr != nil {
		audioPath = rawPath
	} else {
		defer os.Remove(audioPath)
	}
	return s.SendAudioFile(info, audioPath, "", 0) == nil
}

// ytVideoEngineTurbo: yt2RaceFetch → itag18 (360p WhatsApp-ready) → send.
// Returns true when the video was sent.
func ytVideoEngineTurbo(ctx context.Context, s SessionBridge, info types.MessageInfo, vid VideoResult) bool {
	dlClient := &http.Client{Timeout: yt2DlTimeout}

	var st *yt2Stream
	videoID := ytDirectExtractID(vid.URL)
	if videoID != "" {
		st = yt2RaceFetch(ctx, videoID, false)
	}
	if st == nil {
		return false
	}
	if st.itag18 == "" {
		return false
	}
	finalPath, err := yt2FetchSimple(ctx, dlClient, st.itag18)
	if err != nil || finalPath == "" {
		return false
	}
	defer os.Remove(finalPath)
	return s.SendVideoFile(info, finalPath, "", nil, 0, 0, 0) == nil
}

// ytAudioEngineClassic: fetchYTDirect → loader.to mp3 fallback → send.
func ytAudioEngineClassic(ctx context.Context, s SessionBridge, info types.MessageInfo, vid VideoResult) bool {
	client := &http.Client{Timeout: 120 * time.Second}
	wsAudio, wsErr := fetchYTDirect(ctx, ytDirectClient(), vid.URL)
	if wsErr != nil || wsAudio == nil || wsAudio.Result.AudioURL == "" {
		wsAudio, wsErr = ytLoaderToFallback(ctx, client, vid.URL, "mp3")
	}
	if wsErr != nil || wsAudio == nil || wsAudio.Result.AudioURL == "" {
		return false
	}
	rawPath, err := streamDownloadToFile(ctx, client, wsAudio.Result.AudioURL, nil)
	if err != nil {
		return false
	}
	defer os.Remove(rawPath)

	audioPath, convErr := audioRemuxFile(ctx, rawPath)
	if convErr != nil {
		audioPath = rawPath
	} else {
		defer os.Remove(audioPath)
	}
	return s.SendAudioFile(info, audioPath, "", 0) == nil
}

// ytVideoEngineClassic: fetchYTDirect → loader.to 360 fallback → remux → send.
func ytVideoEngineClassic(ctx context.Context, s SessionBridge, info types.MessageInfo, vid VideoResult) bool {
	client := &http.Client{Timeout: 120 * time.Second}
	ws, err := fetchYTDirect(ctx, ytDirectClient(), vid.URL)
	if err != nil || ws == nil || ws.Result.VideoURL == "" {
		ws, err = ytLoaderToFallback(ctx, client, vid.URL, "360")
	}
	if err != nil || ws == nil || ws.Result.VideoURL == "" {
		return false
	}
	videoPath, dlErr := streamDownloadToFile(ctx, client, ws.Result.VideoURL, nil)
	if dlErr != nil {
		return false
	}
	defer os.Remove(videoPath)

	finalPath, rErr := fastRemuxFile(ctx, videoPath)
	if rErr != nil {
		finalPath = videoPath
	} else {
		defer os.Remove(finalPath)
	}
	return s.SendVideoFile(info, finalPath, "", nil, 0, 0, 0) == nil
}
