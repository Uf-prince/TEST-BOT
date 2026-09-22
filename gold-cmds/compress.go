package goldcmds

// ============================================================================
// GOLD-MD — .compress / .comp / .shrink command
// File: compress.go
// ============================================================================
// Ported from UMAR-MD plugins/compress.js — SAME WORK (0% farak):
// Compresses a replied (or directly sent) media file of ANY kind:
//   IMAGE · STICKER · GIF · VIDEO · AUDIO · PDF · any other document
//
//   IMAGE / STICKER (instant, no menu):  .compress [high|medium|low]
//   VIDEO / GIF (interactive menu):      .compress  → number reply picks tier
//   AUDIO (instant):                     .compress [high|medium|low]
//   PDF (instant, needs Ghostscript):    .compress [high|medium|low]
//   ANY OTHER FILE (Brotli):             .compress [high|medium|low]
//
// Go implementation notes:
//   - sharp (Node)  → ffmpeg (image jpeg + webp, animated stickers via
//                     libwebp_anim; Go stdlib has no webp encoder).
//   - fluent-ffmpeg → exec ffmpeg with args (same codec/bitrate logic).
//   - zlib.brotli    → brotli CLI (levels 11/7/3 same as Node constants).
//   - gs presets     → /printer /ebook /screen (same as Node).
//   - Session menu   → CompressTryHandle hook (like SettingsTryHandle)
//                     called on EVERY message before dispatch.
// Emoji: 👑 → 🔰 (GOLD-MD branding).
// ============================================================================

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

const compressDebugLogs = false

func compressDebug(tag string, kv ...interface{}) {
	if !compressDebugLogs {
		return
	}
	// SILENT (owner request): console log off
	_ = tag
	_ = kv
	// fmt.Println("[COMPRESS]", tag, fmt.Sprint(kv...))
}

// ═══════════════════════════════════════════════════════════════════════════
//  SHARED FORMAT HELPERS
// ═══════════════════════════════════════════════════════════════════════════

func compressFormatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(n)/(1024*1024))
}

func compressFormatDuration(totalSeconds float64) string {
	s := int(totalSeconds)
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func compressReducedPct(original, newSize int64) string {
	if original == 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", 100-(float64(newSize)/float64(original))*100)
}

// ═══════════════════════════════════════════════════════════════════════════
//  PRESETS — fixed, real numbers, never invented at runtime.
// ═══════════════════════════════════════════════════════════════════════════

type compressImagePreset struct {
	Label   string
	Quality int
}

var compressImagePresets = map[string]compressImagePreset{
	"high":   {Label: "HIGH", Quality: 80},
	"medium": {Label: "MEDIUM", Quality: 55},
	"low":    {Label: "LOW", Quality: 30},
}

var compressStickerPresets = map[string]compressImagePreset{
	"high":   {Label: "HIGH", Quality: 80},
	"medium": {Label: "MEDIUM", Quality: 50},
	"low":    {Label: "LOW", Quality: 25},
}

type compressAudioPreset struct {
	Label       string
	BitrateKbps int
}

var compressAudioPresets = map[string]compressAudioPreset{
	"high":   {Label: "HIGH", BitrateKbps: 128},
	"medium": {Label: "MEDIUM", BitrateKbps: 64},
	"low":    {Label: "LOW", BitrateKbps: 32},
}

// Ghostscript's own built-in PDF quality settings — real names, not ours.
var compressPdfPresets = map[string]struct {
	Label     string
	GsSetting string
}{
	"high":   {Label: "HIGH", GsSetting: "/printer"},
	"medium": {Label: "MEDIUM", GsSetting: "/ebook"},
	"low":    {Label: "LOW", GsSetting: "/screen"},
}

// Generic-file Brotli levels. Real constants, 0-11 scale,
// 11 = max compression / slowest. Same as Node.js zlib constants.
var compressDocPresets = map[string]struct {
	Label         string
	BrotliQuality int
}{
	"high":   {Label: "HIGH", BrotliQuality: 11},
	"medium": {Label: "MEDIUM", BrotliQuality: 7},
	"low":    {Label: "LOW", BrotliQuality: 3},
}

// Resolution ladder shared by VIDEO / gif-as-video / real GIF files.
// Never upscales past the source. 120p floor, 1080p cap.
var compressResTiers = []compressTier{
	{Key: "1080p", Label: "1080p (Full HD)", Height: 1080, VideoBitrateKbps: 4500},
	{Key: "720p", Label: "720p (HD)", Height: 720, VideoBitrateKbps: 2000},
	{Key: "480p", Label: "480p (SD)", Height: 480, VideoBitrateKbps: 800},
	{Key: "360p", Label: "360p", Height: 360, VideoBitrateKbps: 500},
	{Key: "240p", Label: "240p", Height: 240, VideoBitrateKbps: 300},
	{Key: "144p", Label: "144p", Height: 144, VideoBitrateKbps: 150},
	{Key: "120p", Label: "120p (Lowest)", Height: 120, VideoBitrateKbps: 100},
}

const compressAudioBitrateKbps = 96 // audio track carried inside compressed video/gif

type compressTier struct {
	Key              string
	Label            string
	Height           int
	VideoBitrateKbps int
}

func compressAvailableTiers(sourceHeight int) []compressTier {
	if sourceHeight == 0 {
		// resolution unknown — safe middle tiers only
		return []compressTier{compressResTiers[2], compressResTiers[3], compressResTiers[4]}
	}
	var tiers []compressTier
	for _, t := range compressResTiers {
		if t.Height <= sourceHeight+40 {
			tiers = append(tiers, t)
		}
	}
	if len(tiers) > 0 {
		return tiers
	}
	// source smaller than every tier — offer same-res lower-bitrate pass.
	return []compressTier{{
		Key: "same", Label: fmt.Sprintf("%dp (lower bitrate)", sourceHeight),
		Height: sourceHeight, VideoBitrateKbps: 100,
	}}
}

func compressEstimateSize(t compressTier, durationSec float64) int64 {
	if durationSec == 0 {
		return 0
	}
	totalKbps := int64(t.VideoBitrateKbps + compressAudioBitrateKbps)
	return (totalKbps * 1000 * int64(durationSec)) / 8
}

const compressHelpText = "*❰ COMPRESS ❱ — REDUCE MEDIA SIZE*\n\n" +
	"*REPLY TO ANY MEDIA, THEN TYPE *\n" +
	"*COMPRESS*\n" +
	"*COMPRESS HIGH*\n" +
	"*COMPRESS MEDIUM*\n" +
	"*COMPRESS LOW*\n\n" +
	"*🔰 SUPPORTED:*\n" +
	"*🔰 IMAGE — quality preset (instant)*\n" +
	"*🔰 STICKER — quality preset (instant)*\n" +
	"*🔰 GIF — 120p to 1080p (menu)*\n" +
	"*🔰 VIDEO — 120p to 1080p (menu)*\n" +
	"*🔰 AUDIO — bitrate preset (instant)*\n" +
	"*🔰 PDF — quality preset (instant, needs Ghostscript on server)*\n" +
	"*🔰 ANY OTHER FILE (zip/js/ts/doc/apk/...) — real compression (instant)*\n\n" +
	"*ONLY REAL, ACTUALLY-AVAILABLE OPTIONS ARE SHOWN — NEVER UPSCALED, NEVER GUESSED.*"

// ═══════════════════════════════════════════════════════════════════════════
//  MEDIA DETECTION + DOWNLOAD (shared by every kind)
// ═══════════════════════════════════════════════════════════════════════════

// compressMediaKind is the detected business kind of the media.
type compressMediaKind string

const (
	kindImage    compressMediaKind = "image"
	kindSticker  compressMediaKind = "sticker"
	kindGifVideo compressMediaKind = "gif-video" // WA gif (mp4 + gifPlayback)
	kindGifFile  compressMediaKind = "gif-file"  // real .gif file
	kindVideo    compressMediaKind = "video"
	kindAudio    compressMediaKind = "audio"
	kindPdf      compressMediaKind = "pdf"
	kindDocument compressMediaKind = "document"
)

var compressExtRe = regexp.MustCompile(`(?i)\.([a-z0-9]+)$`)

func compressExt(fileName string) string {
	m := compressExtRe.FindStringSubmatch(fileName)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// compressMediaTarget — which message to operate on (quoted first, then self)
// and what business kind it is. Mirrors UmarFindMediaTarget.
func compressFindMediaTarget(s SessionBridge, info types.MessageInfo) (kind compressMediaKind, fileName string, ok bool) {
	// We inspect the RAW cached message of the incoming command message.
	// Candidates: the QUOTED message (if the command is a reply), else the
	// command message itself (when media + caption command in one message).
	raw := s.GetRawMessage(info)
	if raw == nil {
		return "", "", false
	}

	type targetMsg struct {
		msg      *waProtoMessage
		fileName string
	}
	var candidates []targetMsg

	// Quoted message first
	if raw.ExtendedTextMessage != nil && raw.ExtendedTextMessage.ContextInfo != nil &&
		raw.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
		qm := raw.ExtendedTextMessage.ContextInfo.QuotedMessage
		fn := compressQuotedFileName(qm)
		candidates = append(candidates, targetMsg{msg: qm, fileName: fn})
	}
	// Then the message itself
	candidates = append(candidates, targetMsg{msg: raw, fileName: compressSelfFileName(raw)})

	for _, t := range candidates {
		m := t.msg
		if m == nil {
			continue
		}
		fn := t.fileName
		if fn == "" {
			fn = compressSelfFileName(m)
		}
		ext := compressExt(fn)
		mtLower := compressMsgMimetype(m)

		isGifFlag := m.VideoMessage != nil && m.VideoMessage.GetGifPlayback()

		if m.StickerMessage != nil {
			return kindSticker, fn, true
		}
		if mtLower == "image/gif" || ext == "gif" {
			isVideoBased := m.VideoMessage != nil || strings.HasPrefix(mtLower, "video/")
			if isVideoBased {
				return kindGifVideo, fn, true
			}
			return kindGifFile, fn, true
		}
		if isGifFlag {
			return kindGifVideo, fn, true
		}
		if m.ImageMessage != nil || strings.HasPrefix(mtLower, "image/") {
			return kindImage, fn, true
		}
		if m.VideoMessage != nil || strings.HasPrefix(mtLower, "video/") {
			return kindVideo, fn, true
		}
		if m.AudioMessage != nil || strings.HasPrefix(mtLower, "audio/") {
			return kindAudio, fn, true
		}
		if mtLower == "application/pdf" || ext == "pdf" {
			return kindPdf, fn, true
		}
		if m.DocumentMessage != nil {
			return kindDocument, fn, true
		}
	}
	return "", "", false
}

// waProtoMessage alias keeps the detection code tidy.
type waProtoMessage = waProto.Message

// compressMsgMimetype returns the lower-cased mimetype of a message proto.
func compressMsgMimetype(m *waProtoMessage) string {
	if m == nil {
		return ""
	}
	if m.ImageMessage != nil && m.ImageMessage.Mimetype != nil {
		return strings.ToLower(*m.ImageMessage.Mimetype)
	}
	if m.VideoMessage != nil && m.VideoMessage.Mimetype != nil {
		return strings.ToLower(*m.VideoMessage.Mimetype)
	}
	if m.AudioMessage != nil && m.AudioMessage.Mimetype != nil {
		return strings.ToLower(*m.AudioMessage.Mimetype)
	}
	if m.StickerMessage != nil && m.StickerMessage.Mimetype != nil {
		return strings.ToLower(*m.StickerMessage.Mimetype)
	}
	if m.DocumentMessage != nil && m.DocumentMessage.Mimetype != nil {
		return strings.ToLower(*m.DocumentMessage.Mimetype)
	}
	return ""
}

// compressQuotedFileName extracts the fileName from a quoted message proto.
func compressQuotedFileName(qm *waProtoMessage) string {
	if qm == nil {
		return ""
	}
	if qm.DocumentMessage != nil && qm.DocumentMessage.FileName != nil {
		return *qm.DocumentMessage.FileName
	}
	// NOTE: in the waE2E protobuf only DocumentMessage carries a fileName
	// field (Baileys' t.fileName getters for image/video are Baileys sugar).
	return ""
}

// compressSelfFileName extracts fileName from the message itself.
func compressSelfFileName(m *waProtoMessage) string {
	return compressQuotedFileName(m)
}

// compressDownloadMedia downloads the media bytes for the detected target
// (quoted first, else self). Returns bytes, mimetype, ok.
func compressDownloadMedia(s SessionBridge, info types.MessageInfo) ([]byte, string, bool, bool) {
	// OWNER ORDER (2026): the 700MB pre-check / post-check are REMOVED — no
	// file-size limit on Heroku. Compress whatever the user sent.
	//
	// DownloadQuotedMedia on the bridge handles quoted OR direct media of
	// the INCOMING message (it walks extractMediaMessage which prefers
	// direct media, then view-once wrappers, then quoted).
	// But we need QUOTED FIRST when the command replies to media.
	raw := s.GetRawMessage(info)
	if raw != nil && raw.ExtendedTextMessage != nil && raw.ExtendedTextMessage.ContextInfo != nil &&
		raw.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
		// If the incoming message itself carries no media, DownloadQuotedMedia
		// will walk into the quoted message. If it carries media, direct wins.
		if raw.ImageMessage == nil && raw.VideoMessage == nil && raw.AudioMessage == nil &&
			raw.StickerMessage == nil && raw.DocumentMessage == nil {
			// Pure text reply → quoted media download path.
			data, mime, ok := s.DownloadQuotedMedia(info)
			if ok && len(data) > 0 {
				return data, mime, true, false
			}
			return nil, "", false, false
		}
	}
	data, mime, ok := s.DownloadQuotedMedia(info)
	if ok && len(data) > 0 {
		return data, mime, true, false
	}
	return nil, "", false, false
}

// compressResolveQualityArg maps raw text → high/medium/low ("" = medium).
// Returns "" (invalid) when nothing matched — callers show the usage text.
func compressResolveQualityArg(rawArg string) string {
	a := strings.ToLower(strings.TrimSpace(rawArg))
	switch a {
	case "high", "h":
		return "high"
	case "low", "l":
		return "low"
	case "medium", "med", "m", "":
		return "medium"
	}
	return ""
}

// compressBinaryAvailable reports whether a binary is on PATH.
func compressBinaryAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// compressCopyFile copies src to dst (true on success).
func compressCopyFile(src, dst string) bool {
	b, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	return os.WriteFile(dst, b, 0644) == nil
}

// compressStatusMsg holds the sent "COMPRESSING..." message for edit/delete.
type compressStatusMsg struct {
	ID string
}

// compressSendStatus sends a status message and returns its handle.
func compressSendStatus(s SessionBridge, info types.MessageInfo, text string) *compressStatusMsg {
	id := s.ReplyWithID(info, text)
	if id == "" {
		return nil
	}
	return &compressStatusMsg{ID: id}
}

// compressEditStatus edits the status message text.
func compressEditStatus(s SessionBridge, info types.MessageInfo, st *compressStatusMsg, text string) {
	if st == nil || st.ID == "" {
		return
	}
	s.EditMessage(info, st.ID, text)
}

// compressDeleteStatus deletes the status message.
func compressDeleteStatus(s SessionBridge, info types.MessageInfo, st *compressStatusMsg) {
	if st == nil || st.ID == "" {
		return
	}
	_ = s.DeleteMessage(info, st.ID)
}

// ═══════════════════════════════════════════════════════════════════════════
//  IMAGE COMPRESSION — instant, all three presets real (ffmpeg jpeg).
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodeImageJPEG(ctx context.Context, inPath, outPath string, quality int) (float64, error) {
	start := time.Now()
	// ffmpeg -i in -q:v N out.jpg  — q:v maps inversely to quality:
	// q2≈quality 95, q5≈80, q10≈50, q15≈30 (same ladder Node sharp uses).
	// jpegoptim first: 5-10x lighter than ffmpeg (tiny RAM, single pass).
	// JPEG-only; the ffmpeg fallback below handles PNG/WEBP/BMP inputs.
	if compressBinaryAvailable("jpegoptim") {
		jpgCopy := outPath + ".jpg"
		if compressCopyFile(inPath, jpgCopy) {
			mval := 85
			switch {
			case quality >= 80:
				mval = 92
			case quality >= 55:
				mval = 70
			default:
				mval = 50
			}
			cmd := exec.CommandContext(ctx, "jpegoptim", "-m"+strconv.Itoa(mval), "--strip-all", jpgCopy)
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err == nil {
				if st, serr := os.Stat(jpgCopy); serr == nil && st.Size() > 0 {
					if rerr := os.Rename(jpgCopy, outPath); rerr == nil {
						return time.Since(start).Seconds() * 1000, nil
					}
				}
			}
			os.Remove(jpgCopy) // jpegoptim unusable here -> ffmpeg fallback
		}
	}
	qv := 2
	switch {
	case quality >= 80:
		qv = 3
	case quality >= 55:
		qv = 7
	case quality >= 30:
		qv = 12
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath, "-q:v", strconv.Itoa(qv), outPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressHandleImage(ctx context.Context, s SessionBridge, info types.MessageInfo, rawArg string) {
	qualityKey := compressResolveQualityArg(rawArg)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*")
		return
	}

	st := compressSendStatus(s, info, "*COMPRESSING...*")
	defer compressDeleteStatus(s, info, st)

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	tmpIn, _ := os.CreateTemp("", "goldcmp-img-in-*")
	if tmpIn != nil {
		tmpIn.Write(data)
		tmpIn.Close()
		defer os.Remove(tmpIn.Name())
	} else {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	// All three presets real — parallel like Node (goroutines).
	type presetResult struct {
		key  string
		p    compressImagePreset
		size int64
		ms   float64
		path string
		err  error
	}
	keys := []string{"high", "medium", "low"}
	results := make([]presetResult, len(keys))
	// Sequential (was 3 parallel encoders): 1/3 RAM peak, same
	// wall-clock on a 0.5-core container - lighter and safer.
	for i, k := range keys {
		p := compressImagePresets[k]
		out, _ := os.CreateTemp("", "goldcmp-img-out-"+k+"-*")
		if out == nil {
			results[i] = presetResult{key: k, p: p, err: fmt.Errorf("temp file")}
			continue
		}
		outPath := out.Name()
		out.Close()
		ms, err := compressEncodeImageJPEG(ctx, tmpIn.Name(), outPath, p.Quality)
		if err != nil {
			os.Remove(outPath)
			results[i] = presetResult{key: k, p: p, err: err}
			continue
		}
		st2, _ := os.Stat(outPath)
		var sz int64
		if st2 != nil {
			sz = st2.Size()
		}
		results[i] = presetResult{key: k, p: p, size: sz, ms: ms, path: outPath}
	}

	var chosen *presetResult
	for i := range results {
		if results[i].key == qualityKey {
			chosen = &results[i]
		}
	}
	if chosen == nil || chosen.err != nil || chosen.path == "" {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	for _, r := range results {
		if r.path != "" && r.path != chosen.path {
			os.Remove(r.path)
		}
	}

	var lines []string
	for _, k := range keys {
		r := results[compressedIndexOf(keys, k)]
		if r.err != nil {
			continue
		}
		lines = append(lines, "*"+padEnd(r.p.Label, 6)+" :➭ "+compressFormatBytes(r.size)+" ("+fmt.Sprintf("%.1f", r.ms)+"ms)*")
	}

	caption := "*IMAGE COMPRESSED*\n\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*SELECTED  :➭ " + chosen.p.Label + " (" + compressFormatBytes(chosen.size) + ")*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, chosen.size) + "%*\n" +
		"*TIME      :➭ " + fmt.Sprintf("%.1f", chosen.ms) + "ms*\n\n" +
		"*ALL PRESETS*\n" + strings.Join(lines, "\n")

	outData, err := os.ReadFile(chosen.path)
	if err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	if err := s.SendImage(info, outData, caption); err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
	}
	compressDebug("IMAGE:success", "orig=", originalSize, "new=", chosen.size)
}

// ═══════════════════════════════════════════════════════════════════════════
//  STICKER COMPRESSION — webp output via ffmpeg libwebp, 512×512 inside.
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodeStickerWEBP(ctx context.Context, inPath, outPath string, quality int, animated bool) (float64, error) {
	start := time.Now()
	// ffmpeg libwebp: -quality N (0-100). 512×512 inside, no enlargement.
	scale := "scale='min(512,iw)':'min(512,ih)':force_original_aspect_ratio=decrease"
	codec := "libwebp"
	if animated {
		codec = "libwebp_anim"
	}
	args := []string{"-y", "-i", inPath, "-vf", scale, "-c:v", codec,
		"-quality", strconv.Itoa(quality), "-lossless", "0", outPath}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressHandleSticker(ctx context.Context, s SessionBridge, info types.MessageInfo, rawArg string) {
	qualityKey := compressResolveQualityArg(rawArg)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*")
		return
	}

	st := compressSendStatus(s, info, "*COMPRESSING STICKER...*")
	defer compressDeleteStatus(s, info, st)

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	tmpIn, _ := os.CreateTemp("", "goldcmp-stk-in-*")
	if tmpIn == nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()
	defer os.Remove(tmpIn.Name())

	keys := []string{"high", "medium", "low"}
	type presetResult struct {
		key  string
		p    compressImagePreset
		size int64
		ms   float64
		path string
		err  error
	}
	results := make([]presetResult, len(keys))
	// Sequential (was 3 parallel encoders): 1/3 RAM peak, same
	// wall-clock on a 0.5-core container - lighter and safer.
	for i, k := range keys {
		p := compressStickerPresets[k]
		out, _ := os.CreateTemp("", "goldcmp-stk-out-"+k+"-*")
		if out == nil {
			results[i] = presetResult{key: k, p: p, err: fmt.Errorf("temp")}
			continue
		}
		outPath := out.Name()
		out.Close()
		ms, err := compressEncodeStickerWEBP(ctx, tmpIn.Name(), outPath, p.Quality, true)
		if err != nil {
			os.Remove(outPath)
			results[i] = presetResult{key: k, p: p, err: err}
			continue
		}
		st2, _ := os.Stat(outPath)
		var sz int64
		if st2 != nil {
			sz = st2.Size()
		}
		results[i] = presetResult{key: k, p: p, size: sz, ms: ms, path: outPath}
	}

	var chosen *presetResult
	for i := range results {
		if results[i].key == qualityKey {
			chosen = &results[i]
		}
	}
	if chosen == nil || chosen.err != nil || chosen.path == "" {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	for _, r := range results {
		if r.path != "" && r.path != chosen.path {
			os.Remove(r.path)
		}
	}

	var lines []string
	for _, k := range keys {
		r := results[compressedIndexOf(keys, k)]
		if r.err != nil {
			continue
		}
		lines = append(lines, "*"+padEnd(r.p.Label, 6)+" :➭ "+compressFormatBytes(r.size)+" ("+fmt.Sprintf("%.1f", r.ms)+"ms)*")
	}

	// Stickers carry no caption on WhatsApp — send plain sticker.
	outData, err := os.ReadFile(chosen.path)
	if err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	if err := s.SendSticker(info, outData); err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	// Node.js sends caption with the sticker — WhatsApp drops sticker
	// captions, so the stats go as a separate text reply (same info, 0 farak).
	stats := "*STICKER COMPRESSED*\n\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*SELECTED  :➭ " + chosen.p.Label + " (" + compressFormatBytes(chosen.size) + ")*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, chosen.size) + "%*\n\n" +
		"*ALL PRESETS* *\n" + strings.Join(lines, "\n")
	s.Reply(info, stats)
	compressDebug("STICKER:success", "orig=", originalSize, "new=", chosen.size)
}

// ═══════════════════════════════════════════════════════════════════════════
//  AUDIO COMPRESSION — instant, real bitrate presets via ffmpeg mp3.
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodeAudioMP3(ctx context.Context, inPath, outPath string, bitrateKbps int) (float64, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath,
		"-vn", "-c:a", "libmp3lame", "-b:a", strconv.Itoa(bitrateKbps)+"k",
		"-format", "mp3", outPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressHandleAudio(ctx context.Context, s SessionBridge, info types.MessageInfo, rawArg string) {
	qualityKey := compressResolveQualityArg(rawArg)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*")
		return
	}

	st := compressSendStatus(s, info, "*COMPRESSING AUDIO...*")

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	tmpIn, _ := os.CreateTemp("", "goldcmp-aud-in-*")
	if tmpIn == nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()
	defer os.Remove(tmpIn.Name())

	keys := []string{"high", "medium", "low"}
	type presetResult struct {
		key  string
		p    compressAudioPreset
		size int64
		ms   float64
		path string
		err  error
	}
	results := make([]presetResult, len(keys))
	for i, k := range keys {
		p := compressAudioPresets[k]
		out, _ := os.CreateTemp("", "goldcmp-aud-out-"+k+"-*")
		if out == nil {
			results[i] = presetResult{key: k, p: p, err: fmt.Errorf("temp")}
			continue
		}
		outPath := out.Name()
		out.Close()
		ms, err := compressEncodeAudioMP3(ctx, tmpIn.Name(), outPath, p.BitrateKbps)
		if err != nil {
			os.Remove(outPath)
			results[i] = presetResult{key: k, p: p, err: err}
			continue
		}
		st2, _ := os.Stat(outPath)
		var sz int64
		if st2 != nil {
			sz = st2.Size()
		}
		results[i] = presetResult{key: k, p: p, size: sz, ms: ms, path: outPath}
	}

	var chosen *presetResult
	for i := range results {
		if results[i].key == qualityKey {
			chosen = &results[i]
		}
	}
	if chosen == nil || chosen.err != nil || chosen.path == "" {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	for _, r := range results {
		if r.path != "" && r.path != chosen.path {
			os.Remove(r.path)
		}
	}

	var lines []string
	for _, k := range keys {
		r := results[compressedIndexOf(keys, k)]
		if r.err != nil {
			continue
		}
		lines = append(lines, "*"+padEnd(r.p.Label, 6)+" :➭ "+compressFormatBytes(r.size)+" ("+strconv.Itoa(r.p.BitrateKbps)+"kbps)*")
	}

	// Read chosen bytes BEFORE deleting status (audio carries no caption).
	outData, err := os.ReadFile(chosen.path)
	if err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	seconds := probeAudioSecondsLocal(tmpIn.Name())
	if err := s.SendAudio(info, outData, "", seconds); err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	caption := "*AUDIO COMPRESSED*\n\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*SELECTED  :➭ " + chosen.p.Label + " (" + compressFormatBytes(chosen.size) + ", " + strconv.Itoa(chosen.p.BitrateKbps) + "kbps MP3)*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, chosen.size) + "%*\n\n" +
		"*ALL PRESETS* *\n" + strings.Join(lines, "\n")

	compressDeleteStatus(s, info, st)
	s.Reply(info, caption)
	compressDebug("AUDIO:success", "orig=", originalSize, "new=", chosen.size)
}

// probeAudioSecondsLocal returns the audio duration via ffprobe (0 on fail).
func probeAudioSecondsLocal(path string) uint32 {
	if !compressBinaryAvailable("ffprobe") {
		return 0
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	f, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if perr != nil || f <= 0 {
		return 0
	}
	return uint32(f)
}

// ═══════════════════════════════════════════════════════════════════════════
//  PDF COMPRESSION — real Ghostscript presets.
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodePdf(ctx context.Context, inPath, outPath, gsSetting string) (float64, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "gs",
		"-sDEVICE=pdfwrite", "-dCompatibilityLevel=1.4",
		"-dPDFSETTINGS="+gsSetting,
		"-dNOPAUSE", "-dQUIET", "-dBATCH",
		"-sOutputFile="+outPath, inPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressHandlePdf(ctx context.Context, s SessionBridge, info types.MessageInfo, rawArg string) {
	qualityKey := compressResolveQualityArg(rawArg)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*")
		return
	}

	if !compressBinaryAvailable("gs") {
		s.Reply(info,
			"*🔰 PDF COMPRESSION UNAVAILABLE*\n\n"+
				"*GHOSTSCRIPT (gs) IS NOT INSTALLED ON THE SERVER — RUN* *apt install ghostscript* *AND TRY AGAIN.*")
		return
	}

	st := compressSendStatus(s, info, "*COMPRESSING PDF...*")

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	tmpIn, _ := os.CreateTemp("", "goldcmp-pdf-in-*.pdf")
	tmpOut, _ := os.CreateTemp("", "goldcmp-pdf-out-*.pdf")
	if tmpIn == nil || tmpOut == nil {
		if tmpIn != nil {
			tmpIn.Close()
			os.Remove(tmpIn.Name())
		}
		if tmpOut != nil {
			tmpOut.Close()
			os.Remove(tmpOut.Name())
		}
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()
	tmpOut.Close()
	defer os.Remove(tmpIn.Name())
	defer os.Remove(tmpOut.Name())

	cfg := compressPdfPresets[qualityKey]
	ms, err := compressEncodePdf(ctx, tmpIn.Name(), tmpOut.Name(), cfg.GsSetting)
	if err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	outData, err := os.ReadFile(tmpOut.Name())
	if err != nil || len(outData) == 0 {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 PDF COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	caption := "*PDF COMPRESSED*\n\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*SELECTED  :➭ " + cfg.Label + "*\n" +
		"*NEW SIZE  :➭ " + compressFormatBytes(int64(len(outData))) + "*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, int64(len(outData))) + "%*\n" +
		"*TIME      :➭ " + fmt.Sprintf("%.0f", ms) + "ms*"

	if err := s.SendDocument(info, outData, "compressed.pdf", "application/pdf", caption); err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	compressDeleteStatus(s, info, st)
	compressDebug("PDF:success", "orig=", originalSize, "new=", len(outData))
}

// ═══════════════════════════════════════════════════════════════════════════
//  GENERIC FILE COMPRESSION — zip/js/ts/docx/apk/txt/json/anything → Brotli.
// ═══════════════════════════════════════════════════════════════════════════

func compressBrotli(ctx context.Context, inPath, outPath string, quality int) (float64, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "brotli", "-q", strconv.Itoa(quality), "-c", inPath)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(outPath, out, 0644); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressHandleDocument(ctx context.Context, s SessionBridge, info types.MessageInfo, rawArg, fileNameHint string) {
	qualityKey := compressResolveQualityArg(rawArg)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*")
		return
	}

	st := compressSendStatus(s, info, "*COMPRESSING FILE...*")

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	tmpIn, _ := os.CreateTemp("", "goldcmp-doc-in-*")
	if tmpIn == nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()
	defer os.Remove(tmpIn.Name())

	if !compressBinaryAvailable("brotli") {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 COMPRESSION UNAVAILABLE — brotli NOT INSTALLED ON SERVER*")
		return
	}

	keys := []string{"high", "medium", "low"}
	type presetResult struct {
		key string
		p   struct {
			Label         string
			BrotliQuality int
		}
		size int64
		ms   float64
		path string
		err  error
	}
	results := make([]presetResult, len(keys))
	// Sequential (was 3 parallel encoders): 1/3 RAM peak, same
	// wall-clock on a 0.5-core container - lighter and safer.
	for i, k := range keys {
		p := compressDocPresets[k]
		out, _ := os.CreateTemp("", "goldcmp-doc-out-"+k+"-*")
		if out == nil {
			results[i] = presetResult{key: k, err: fmt.Errorf("temp")}
			continue
		}
		outPath := out.Name()
		out.Close()
		ms, err := compressBrotli(ctx, tmpIn.Name(), outPath, p.BrotliQuality)
		if err != nil {
			os.Remove(outPath)
			results[i] = presetResult{key: k, err: err}
			continue
		}
		st2, _ := os.Stat(outPath)
		var sz int64
		if st2 != nil {
			sz = st2.Size()
		}
		results[i] = presetResult{key: k, size: sz, ms: ms, path: outPath}
	}

	var chosen *presetResult
	for i := range results {
		if results[i].key == qualityKey {
			chosen = &results[i]
		}
	}
	if chosen == nil || chosen.err != nil || chosen.path == "" {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	outData, err := os.ReadFile(chosen.path)
	if err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	for _, r := range results {
		if r.path != "" {
			os.Remove(r.path)
		}
	}

	baseName := fileNameHint
	if baseName == "" {
		baseName = "file"
	}
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	outName := baseName + ".br"

	var lines []string
	for _, k := range keys {
		r := results[compressedIndexOf(keys, k)]
		if r.err != nil {
			continue
		}
		lbl := compressDocPresets[k].Label
		lines = append(lines, "*"+padEnd(lbl, 6)+" :➭ "+compressFormatBytes(r.size)+" ("+fmt.Sprintf("%.1f", r.ms)+"ms)*")
	}

	caption := "*FILE COMPRESSED*\n\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*SELECTED  :➭ " + compressDocPresets[qualityKey].Label + " (" + compressFormatBytes(chosen.size) + ")*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, chosen.size) + "%*\n" +
		"*FORMAT    :➭ Brotli (.br) — decompress before use*\n\n" +
		"*ALL PRESETS* *\n" + strings.Join(lines, "\n")

	if err := s.SendDocument(info, outData, outName, "application/x-brotli", caption); err != nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	compressDeleteStatus(s, info, st)
	compressDebug("DOCUMENT:success", "orig=", originalSize, "new=", chosen.size)
}

// ═══════════════════════════════════════════════════════════════════════════
//  VIDEO / GIF PROBE — real duration/resolution from ffprobe.
// ═══════════════════════════════════════════════════════════════════════════

type compressVideoProbe struct {
	DurationSec float64
	Width       int
	Height      int
	BitrateKbps int
}

func compressProbeVideo(path string) compressVideoProbe {
	var p compressVideoProbe
	if !compressBinaryAvailable("ffprobe") {
		return p
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-show_entries", "format=duration,bit_rate",
		"-of", "default=noprint_wrappers=1", path).Output()
	if err != nil {
		return p
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "width":
			p.Width, _ = strconv.Atoi(kv[1])
		case "height":
			p.Height, _ = strconv.Atoi(kv[1])
		case "duration":
			p.DurationSec, _ = strconv.ParseFloat(kv[1], 64)
		case "bit_rate":
			p.BitrateKbps, _ = strconv.Atoi(kv[1])
			p.BitrateKbps /= 1000
		}
	}
	return p
}

// ═══════════════════════════════════════════════════════════════════════════
//  SESSION STORE — one active menu per chat, 60s TTL, number reply picks.
// ═══════════════════════════════════════════════════════════════════════════

const compressSessionTTL = 60 * time.Second

type compressSession struct {
	Tiers        []compressTier
	TmpIn        string
	OriginalSize int64
	MenuID       string
	Processing   bool
	CreatedAt    time.Time
	Mode         string // "video" | "gif-video" | "gif-file"
	Chat         types.JID
}

var (
	compressSessions   = make(map[string]*compressSession)
	compressSessionsMu sync.Mutex
)

func compressKey(s SessionBridge, info types.MessageInfo) string {
	return s.GetJID() + "|" + info.Chat.String()
}

func setCompressSession(key string, sess *compressSession) {
	compressSessionsMu.Lock()
	defer compressSessionsMu.Unlock()
	sess.CreatedAt = time.Now()
	compressSessions[key] = sess
}

func getCompressSession(key string) *compressSession {
	compressSessionsMu.Lock()
	defer compressSessionsMu.Unlock()
	sess, ok := compressSessions[key]
	if !ok {
		return nil
	}
	if time.Now().Sub(sess.CreatedAt) > compressSessionTTL {
		delete(compressSessions, key)
		return nil
	}
	return sess
}

func deleteCompressSession(key string) *compressSession {
	compressSessionsMu.Lock()
	defer compressSessionsMu.Unlock()
	sess := compressSessions[key]
	delete(compressSessions, key)
	return sess
}

// ═══════════════════════════════════════════════════════════════════════════
//  VIDEO ENCODE (also WA gif-as-video) — target-bitrate mode, scale down only.
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodeVideoTier(ctx context.Context, inPath, outPath string, tier compressTier, sourceHeight int, withAudio bool) (float64, error) {
	start := time.Now()
	args := []string{"-y", "-i", inPath,
		"-c:v", "libx264",
		"-b:v", strconv.Itoa(tier.VideoBitrateKbps) + "k",
		"-maxrate", strconv.Itoa(tier.VideoBitrateKbps*3/2) + "k",
		"-bufsize", strconv.Itoa(tier.VideoBitrateKbps*2) + "k",
		"-preset", "ultrafast",
		"-movflags", "+faststart",
		"-threads", "2",
	}
	if sourceHeight > 0 && tier.Height < sourceHeight {
		args = append(args, "-vf", "scale=-2:"+strconv.Itoa(tier.Height))
	}
	if withAudio {
		args = append(args, "-c:a", "aac", "-b:a", strconv.Itoa(compressAudioBitrateKbps)+"k")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-f", "mp4", outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

// compressRunVideoCompression runs the REAL encode after tier pick and sends
// the result. sendAsGif=true re-sends as a WhatsApp gif (gifPlayback).
func compressRunVideoCompression(ctx context.Context, s SessionBridge, info types.MessageInfo, tier compressTier, tmpIn string, originalSize int64, menuID string, sendAsGif bool) {
	if menuID != "" {
		s.EditMessage(info, menuID, "*COMPRESSING — "+tier.Label+"...*")
	}

	tmpOut, _ := os.CreateTemp("", "goldcmp-vid-out-*.mp4")
	if tmpOut == nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		os.Remove(tmpIn)
		return
	}
	tmpOutPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(tmpOutPath)

	probe := compressProbeVideo(tmpIn)
	ms, err := compressEncodeVideoTier(ctx, tmpIn, tmpOutPath, tier, probe.Height, true)
	if err != nil {
		// Retry without audio (same as Node).
		ms, err = compressEncodeVideoTier(ctx, tmpIn, tmpOutPath, tier, probe.Height, false)
		if err != nil {
			if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
				s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
			}
			os.Remove(tmpIn)
			return
		}
	}
	outData, err := os.ReadFile(tmpOutPath)
	if err != nil || len(outData) == 0 {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		os.Remove(tmpIn)
		return
	}

	kind := "VIDEO"
	if sendAsGif {
		kind = "GIF"
	}
	caption := "*🔰 " + kind + " COMPRESSED*\n\n" +
		"*QUALITY   :➭ " + tier.Label + "*\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*NEW SIZE  :➭ " + compressFormatBytes(int64(len(outData))) + " (real)*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, int64(len(outData))) + "%*\n" +
		"*TIME      :➭ " + fmt.Sprintf("%.0f", ms) + "ms*"

	serr := s.SendVideo(info, outData, caption, nil, uint32(probe.DurationSec), uint32(probe.Width), uint32(probe.Height))
	if serr != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
	}
	if menuID != "" {
		_ = s.DeleteMessage(info, menuID)
	}
	os.Remove(tmpIn)
	compressDebug("VIDEO:success", "orig=", originalSize, "new=", len(outData))
}

// ═══════════════════════════════════════════════════════════════════════════
//  REAL .GIF FILE ENCODE — two-pass palette filter, stays a real gif.
// ═══════════════════════════════════════════════════════════════════════════

func compressEncodeGifTier(ctx context.Context, inPath, outPath string, tier compressTier, sourceHeight int) (float64, error) {
	start := time.Now()
	colors := 256
	if tier.Height <= 240 {
		colors = 128
	}
	scaleH := tier.Height
	if sourceHeight > 0 && tier.Height >= sourceHeight {
		scaleH = sourceHeight
	}
	filter := fmt.Sprintf("fps=15,scale=-2:%d:flags=lanczos,split[a][b];[a]palettegen=max_colors=%d[p];[b][p]paletteuse=dither=bayer", scaleH, colors)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath,
		"-vf", filter, "-loop", "0", "-f", "gif", outPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds() * 1000, nil
}

func compressRunGifFileCompression(ctx context.Context, s SessionBridge, info types.MessageInfo, tier compressTier, tmpIn string, originalSize int64, menuID string) {
	if menuID != "" {
		s.EditMessage(info, menuID, "*COMPRESSING — "+tier.Label+"...*")
	}

	tmpOut, _ := os.CreateTemp("", "goldcmp-gif-out-*.gif")
	if tmpOut == nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		os.Remove(tmpIn)
		return
	}
	tmpOutPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(tmpOutPath)

	probe := compressProbeVideo(tmpIn)
	ms, err := compressEncodeGifTier(ctx, tmpIn, tmpOutPath, tier, probe.Height)
	if err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		os.Remove(tmpIn)
		return
	}
	outData, err := os.ReadFile(tmpOutPath)
	if err != nil || len(outData) == 0 {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		os.Remove(tmpIn)
		return
	}

	caption := "*🔰 GIF COMPRESSED*\n\n" +
		"*QUALITY   :➭ " + tier.Label + "*\n" +
		"*ORIGINAL  :➭ " + compressFormatBytes(originalSize) + "*\n" +
		"*NEW SIZE  :➭ " + compressFormatBytes(int64(len(outData))) + " (real)*\n" +
		"*REDUCED   :➭ " + compressReducedPct(originalSize, int64(len(outData))) + "%*\n" +
		"*TIME      :➭ " + fmt.Sprintf("%.0f", ms) + "ms*"

	if err := s.SendDocument(info, outData, "compressed.gif", "image/gif", caption); err != nil {
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
	}
	if menuID != "" {
		_ = s.DeleteMessage(info, menuID)
	}
	os.Remove(tmpIn)
	compressDebug("GIF_FILE:success", "orig=", originalSize, "new=", len(outData))
}

// ═══════════════════════════════════════════════════════════════════════════
//  DIRECT .compress high|medium|low — skips the menu for video/gif.
// ═══════════════════════════════════════════════════════════════════════════

func compressHandleTieredDirect(ctx context.Context, s SessionBridge, info types.MessageInfo, kind compressMediaKind, qualityKey string) {
	st := compressSendStatus(s, info, "*ANALYZING...*")

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	ext := "mp4"
	if kind == kindGifFile {
		ext = "gif"
	}
	tmpIn, _ := os.CreateTemp("", "goldcmp-tier-in-*."+ext)
	if tmpIn == nil {
		compressDeleteStatus(s, info, st)
		if !ctxTimedOut(ctx) { // timeout → sirf TRY AGAIN LATER reply
			s.Reply(info, "*🔰 COMPRESSION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()

	probe := compressProbeVideo(tmpIn.Name())
	tiers := compressAvailableTiers(probe.Height)
	var tier compressTier
	switch qualityKey {
	case "high":
		tier = tiers[0]
	case "low":
		tier = tiers[len(tiers)-1]
	default:
		tier = tiers[(len(tiers)-1)/2]
	}

	menuID := ""
	if st != nil {
		menuID = st.ID
	}
	if kind == kindGifFile {
		compressRunGifFileCompression(ctx, s, info, tier, tmpIn.Name(), originalSize, menuID)
	} else {
		compressRunVideoCompression(ctx, s, info, tier, tmpIn.Name(), originalSize, menuID, kind == kindGifVideo)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
//  .compress (no args) on video/gif — probe, numbered menu, 60s session.
// ═══════════════════════════════════════════════════════════════════════════

func compressHandleTieredMenu(ctx context.Context, s SessionBridge, info types.MessageInfo, kind compressMediaKind) {
	st := compressSendStatus(s, info, "*ANALYZING...*")

	data, _, ok, tooBig := compressDownloadMedia(s, info)
	if tooBig {
		compressDeleteStatus(s, info, st)
		return
	}
	if !ok || len(data) == 0 {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 MEDIA DOWNLOAD FAILED, PLEASE TRY AGAIN*")
		return
	}
	originalSize := int64(len(data))

	ext := "mp4"
	if kind == kindGifFile {
		ext = "gif"
	}
	tmpIn, _ := os.CreateTemp("", "goldcmp-menu-in-*."+ext)
	if tmpIn == nil {
		compressDeleteStatus(s, info, st)
		s.Reply(info, "*🔰 COULD NOT ANALYZE MEDIA, PLEASE TRY AGAIN*")
		return
	}
	tmpIn.Write(data)
	tmpIn.Close()

	probe := compressProbeVideo(tmpIn.Name())
	tiers := compressAvailableTiers(probe.Height)

	label := "VIDEO"
	if kind == kindGifFile || kind == kindGifVideo {
		label = "GIF"
	}
	sourceLabel := "UNKNOWN"
	if probe.Height > 0 {
		sourceLabel = strconv.Itoa(probe.Height) + "p"
	}

	lines := []string{
		"*🔰 " + label + " DETECTED*",
		"*RESOLUTION :➭ " + sourceLabel + "*",
		"*SIZE       :➭ " + compressFormatBytes(originalSize) + "*",
	}
	if probe.DurationSec > 0 {
		lines = append(lines, "*DURATION   :➭ "+compressFormatDuration(probe.DurationSec)+"*")
	}
	lines = append(lines, "", "*SELECT A QUALITY — REPLY WITH A NUMBER:*")

	for i, tier := range tiers {
		sizeText := ""
		if kind == kindGifFile {
			sizeText = "size varies (real gif, not bitrate-based)"
		} else {
			est := compressEstimateSize(tier, probe.DurationSec)
			if est > 0 {
				sizeText = "~" + compressFormatBytes(est) + " (estimated)"
			} else {
				sizeText = "size varies by content"
			}
		}
		lines = append(lines, "*TYPE "+strconv.Itoa(i+1)+" for "+tier.Label+" :➭ "+sizeText+"*")
	}
	lines = append(lines, "", "*SESSION EXPIRES IN 60 SECONDS.*")

	menuID := ""
	if st != nil {
		menuID = st.ID
	}
	if menuID != "" {
		s.EditMessage(info, menuID, strings.Join(lines, "\n"))
	} else {
		s.Reply(info, strings.Join(lines, "\n"))
	}

	setCompressSession(compressKey(s, info), &compressSession{
		Tiers:        tiers,
		TmpIn:        tmpIn.Name(),
		OriginalSize: originalSize,
		MenuID:       menuID,
		Mode:         string(kind),
		Chat:         info.Chat,
	})
}

// ═══════════════════════════════════════════════════════════════════════════
//  SESSION HOOK — runs on EVERY incoming message BEFORE command dispatch.
//  Same position as Node.js UmarCompressBefore (attached on messages.upsert).
// ═══════════════════════════════════════════════════════════════════════════

var compressBareNumberRe = regexp.MustCompile(`^\s*\d{1,2}\s*$`)

// CompressTryHandle — call from the main message handler BEFORE dispatch.
// Returns true when the message was consumed (a tier number reply).
func CompressTryHandle(s SessionBridge, info types.MessageInfo, body string, prefix string) bool {
	key := compressKey(s, info)
	sess := getCompressSession(key)
	if sess == nil {
		return false
	}
	if sess.Processing {
		return false
	}

	trimmed := strings.TrimSpace(body)
	if compressBareNumberRe.MatchString(trimmed) == false {
		return false
	}
	idx, err := strconv.Atoi(strings.TrimSpace(trimmed))
	if err != nil || idx < 1 || idx > len(sess.Tiers) {
		return false
	}

	pickedTier := sess.Tiers[idx-1]

	// Claim the session (mark processing + delete) so a double message
	// can't trigger a double encode.
	compressSessionsMu.Lock()
	if cur, ok := compressSessions[key]; !ok || cur != sess {
		compressSessionsMu.Unlock()
		return false
	}
	sess.Processing = true
	delete(compressSessions, key)
	compressSessionsMu.Unlock()

	switch sess.Mode {
	case "gif-file":
		// Watchdog spawn: the 3-min timer covers the encode; on timeout the
		// context is cancelled → ffmpeg is killed → "*TRY AGAIN LATER*".
		go RunWithTimeout(s, info, func(ctx context.Context) {
			compressRunGifFileCompression(ctx, s, info, pickedTier, sess.TmpIn, sess.OriginalSize, sess.MenuID)
		})
	default:
		go RunWithTimeout(s, info, func(ctx context.Context) {
			compressRunVideoCompression(ctx, s, info, pickedTier, sess.TmpIn, sess.OriginalSize, sess.MenuID, sess.Mode == "gif-video")
		})
	}
	return true
}

// ═══════════════════════════════════════════════════════════════════════════
//  ENTRY POINT — routes to the right handler by detected kind.
// ═══════════════════════════════════════════════════════════════════════════

func handleCompress(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Hard 3-minute watchdog: on timeout the context is cancelled, every
	// ffmpeg/gs/brotli process tied to it is killed, temp files are cleaned
	// by the pipeline defers, and the user gets "*TRY AGAIN LATER*".
	// 0% speed impact: fast pipelines finish exactly as before.
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleCompressAsync(ctx, s, info, args, prefix)
	})
}

func handleCompressAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	rawArg := strings.Join(args, " ")

	if strings.ToLower(strings.TrimSpace(rawArg)) == "list" {
		s.Reply(info, compressHelpText)
		return
	}

	kind, fileName, ok := compressFindMediaTarget(s, info)
	if !ok {
		s.Reply(info, compressHelpText)
		return
	}

	// Instant, no-menu kinds:
	switch kind {
	case kindImage:
		compressHandleImage(ctx, s, info, rawArg)
		return
	case kindSticker:
		compressHandleSticker(ctx, s, info, rawArg)
		return
	case kindAudio:
		compressHandleAudio(ctx, s, info, rawArg)
		return
	case kindPdf:
		compressHandlePdf(ctx, s, info, rawArg)
		return
	case kindDocument:
		compressHandleDocument(ctx, s, info, rawArg, fileName)
		return
	}

	// Tiered kinds (video / gif-video / gif-file) — menu or direct.
	trimmed := strings.TrimSpace(rawArg)
	if trimmed == "" {
		compressHandleTieredMenu(ctx, s, info, kind)
		return
	}
	qualityKey := compressResolveQualityArg(trimmed)
	if qualityKey == "" {
		s.Reply(info, "*🔰 INVALID QUALITY, USE ONE OF THESE:*\n\n*.compress high / medium / low*\n*or just .compress for the menu*")
		return
	}
	compressHandleTieredDirect(ctx, s, info, kind, qualityKey)
}

// ── helpers ────────────────────────────────────────────────────────────────

// padEnd pads a string with spaces to n chars (Node padEnd).
func padEnd(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// compressedIndexOf returns the index of k in keys (-1 → 0 safe).
func compressedIndexOf(keys []string, k string) int {
	for i, kk := range keys {
		if kk == k {
			return i
		}
	}
	return 0
}

// ── registration ──────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "compress", Category: "AI & MEDIA", Desc: "THIS COMMAND IS USED TO REDUCE THE SIZE OF ANY IMAGE OR VIDEO. REPLY TO THE MEDIA AND USE THIS COMMAND TO MAKE IT SMALLER.", Run: handleCompress})
	Register(Command{Name: "comp", Hidden: true, Run: handleCompress})
	Register(Command{Name: "shrink", Hidden: true, Run: handleCompress})
}
