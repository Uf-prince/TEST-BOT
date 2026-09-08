package goldcmds

// ============================================================================
// GOLD-MD - .PP COMMAND (SET PROFILE PICTURE) - FINAL CLEAN BUILD
//
// LIVE-VERIFIED SERVER RULES (probed Sept 5, 2026):
//   • WhatsApp does NOT crop or re-encode: the DP stays exactly as uploaded.
//   • Acceptance rule: image sides must be <= 640 (ANY aspect ratio works).
//       full-size 1076x1920          -> instant 406 not-acceptable
//       portrait 359x640 (aspect ok) -> ACCEPTED
//       square 640x640               -> ACCEPTED
//   • 406 ROOT CAUSE was ffmpeg-encoded JPEGs (COM marker "Lavc59.37.100"
//     + yuvj420p); Go stdlib image/jpeg output (plain JFIF, no COM) is
//     accepted by 2026 servers.
//
// LADDER (first success wins; each step tries targets omitted -> PN -> LID):
//   1. ASPECT-FIT - whole picture scaled to fit within 640, aspect ratio
//      preserved: NO crop, NO blur bars, NO padding  (owner priority #1)
//   2. aspect-fit + preview node (dual)
//   3. full-fit 640 square - whole pic + blurred self-background (fallback)
//   4. center-square crop 640x640 (LAST RESORT)
//
// Debug console logging REMOVED per owner request (final clean build).
// Panel tools (kept - they are endpoints, not console logs):
//   /ppdiag?img=<path>       - run this ladder from curl
//   /ppraw?img=&q=&maxside=  - single-IQ server-rule probe
//   /ppcheck                 - GET the current DP back for verification
// ============================================================================

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os/exec"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// ----------------------------------------------------------------------------
// IMAGE PREP (clean JPEGs, Go stdlib encoder = 2026 server-accepted)
// ----------------------------------------------------------------------------

// ppDecodeImage decodes ANY image (JPEG/PNG/WEBP/GIF/BMP) into image.Image.
// ffmpeg is the universal decoder here (stdin pipe -> PNG -> Go decoder);
// if ffmpeg is missing we fall back to Go's registered decoders (jpeg/png).
func ppDecodeImage(data []byte) (image.Image, error) {
	if isFfmpegAvailable() {
		dec := exec.Command("ffmpeg", "-v", "error", "-i", "pipe:0",
			"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "pipe:1")
		dec.Stdin = bytes.NewReader(data)
		var buf bytes.Buffer
		dec.Stdout = &buf
		if err := dec.Run(); err == nil && buf.Len() > 0 {
			img, _, derr := image.Decode(bytes.NewReader(buf.Bytes()))
			if derr == nil {
				return img, nil
			}
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// ppEncodeAspectFitJPEG scales the WHOLE image to fit within maxSide on
// BOTH axes with the aspect ratio PRESERVED - NO crop, NO blur bars, NO
// padding. This is the owner's desired behaviour: the entire picture as
// the DP, exactly as sent, just scaled to the server's side limit.
// LIVE-VERIFIED Sept 5, 2026: portrait 359x640 accepted.
func ppEncodeAspectFitJPEG(data []byte, maxSide int, quality int) []byte {
	img, err := ppDecodeImage(data)
	if err != nil || img == nil {
		return nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxSide || h > maxSide {
		scale := float64(maxSide) / float64(w)
		if s2 := float64(maxSide) / float64(h); s2 < scale {
			scale = s2
		}
		w2 := int(float64(w)*scale + 0.5)
		h2 := int(float64(h)*scale + 0.5)
		if w2 < 1 {
			w2 = 1
		}
		if h2 < 1 {
			h2 = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, w2, h2))
		for y := 0; y < h2; y++ {
			sy := y * h / h2
			if sy >= h {
				sy = h - 1
			}
			for x := 0; x < w2; x++ {
				sx := x * w / w2
				if sx >= w {
					sx = w - 1
				}
				dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
			}
		}
		img = dst
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil
	}
	return out.Bytes()
}

// ppEncodeFullFitJPEG builds a side×side square that contains the WHOLE
// image (no cropping): the original is scaled to FIT inside the square and
// centered, and the background is a heavily blurred copy of the same image
// scaled to cover the square. Fallback only - owner prefers aspect-fit.
func ppEncodeFullFitJPEG(data []byte, side int, quality int) []byte {
	if side < 192 {
		side = 192
	}
	if side > 640 {
		side = 640
	}
	img, err := ppDecodeImage(data)
	if err != nil || img == nil {
		return nil
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()

	// 1) background: scale to COVER the square, center-crop, then blur
	coverScale := float64(side) / float64(sw)
	if s2 := float64(side) / float64(sh); s2 > coverScale {
		coverScale = s2
	}
	cw := int(float64(sw)*coverScale + 0.5)
	ch := int(float64(sh)*coverScale + 0.5)
	if cw < side {
		cw = side
	}
	if ch < side {
		ch = side
	}
	offX := (cw - side) / 2
	offY := (ch - side) / 2
	cover := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		sy := (y + offY) * sh / ch
		if sy < 0 {
			sy = 0
		}
		if sy >= sh {
			sy = sh - 1
		}
		for x := 0; x < side; x++ {
			sx := (x + offX) * sw / cw
			if sx < 0 {
				sx = 0
			}
			if sx >= sw {
				sx = sw - 1
			}
			cover.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	small := ppAverageDownscale(cover, 40)
	canvas := ppBilinearUpscale(small, side, side)

	// 2) foreground: scale the WHOLE image to FIT inside the square
	fitScale := float64(side) / float64(sw)
	if s2 := float64(side) / float64(sh); s2 < fitScale {
		fitScale = s2
	}
	fw := int(float64(sw)*fitScale + 0.5)
	fh := int(float64(sh)*fitScale + 0.5)
	if fw < 1 {
		fw = 1
	}
	if fh < 1 {
		fh = 1
	}
	fx0 := (side - fw) / 2
	fy0 := (side - fh) / 2
	for y := 0; y < fh; y++ {
		sy := y * sh / fh
		if sy >= sh {
			sy = sh - 1
		}
		for x := 0; x < fw; x++ {
			sx := x * sw / fw
			if sx >= sw {
				sx = sw - 1
			}
			canvas.Set(fx0+x, fy0+y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil
	}
	return out.Bytes()
}

// ppEncodeCleanJPEG re-encodes to a center-square-crop JPEG (LAST-RESORT
// fallback only - cuts the picture). Side clamped to 192..640.
func ppEncodeCleanJPEG(data []byte, side int, quality int) []byte {
	if side < 192 {
		side = 192
	}
	if side > 640 {
		side = 640
	}
	img, err := ppDecodeImage(data)
	if err != nil || img == nil {
		return nil
	}
	src := ppSquareCrop(img)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		sy := y * src.Bounds().Dy() / side
		for x := 0; x < side; x++ {
			sx := x * src.Bounds().Dx() / side
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil
	}
	return out.Bytes()
}

// ppAverageDownscale averages src down to smallSide×smallSide (box filter).
func ppAverageDownscale(src image.Image, smallSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, smallSide, smallSide))
	for y := 0; y < smallSide; y++ {
		y0 := y * h / smallSide
		y1 := (y+1)*h/smallSide + 1
		if y1 > h {
			y1 = h
		}
		for x := 0; x < smallSide; x++ {
			x0 := x * w / smallSide
			x1 := (x+1)*w/smallSide + 1
			if x1 > w {
				x1 = w
			}
			var sr, sg, sb, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, bl, _ := src.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					sr += uint64(r)
					sg += uint64(g)
					sb += uint64(bl)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(x, y, color.RGBA{R: uint8(sr / n >> 8), G: uint8(sg / n >> 8), B: uint8(sb / n >> 8), A: 255})
		}
	}
	return dst
}

// ppBilinearUpscale smoothly scales a small RGBA image up to w×h - used to
// turn the 40×40 average into a soft blurred background.
func ppBilinearUpscale(src *image.RGBA, w, h int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw < 2 {
		sw = 2
	}
	if sh < 2 {
		sh = 2
	}
	get := func(x, y int) (float64, float64, float64) {
		if x < 0 {
			x = 0
		}
		if x >= sw {
			x = sw - 1
		}
		if y < 0 {
			y = 0
		}
		if y >= sh {
			y = sh - 1
		}
		c := src.RGBAAt(sb.Min.X+x, sb.Min.Y+y)
		return float64(c.R), float64(c.G), float64(c.B)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(fy)
		var dy float64
		switch {
		case y0 < 0:
			y0, dy = 0, 0
		case y0 >= sh-1:
			y0, dy = sh-2, 1
		default:
			dy = fy - float64(y0)
		}
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(fx)
			var dx float64
			switch {
			case x0 < 0:
				x0, dx = 0, 0
			case x0 >= sw-1:
				x0, dx = sw-2, 1
			default:
				dx = fx - float64(x0)
			}
			r00, g00, b00 := get(x0, y0)
			r10, g10, b10 := get(x0+1, y0)
			r01, g01, b01 := get(x0, y0+1)
			r11, g11, b11 := get(x0+1, y0+1)
			rx0 := r00 + (r10-r00)*dx
			gx0 := g00 + (g10-g00)*dx
			bx0 := b00 + (b10-b00)*dx
			rx1 := r01 + (r11-r01)*dx
			gx1 := g01 + (g11-g01)*dx
			bx1 := b01 + (b11-b01)*dx
			r := rx0 + (rx1-rx0)*dy
			g := gx0 + (gx1-gx0)*dy
			bl := bx0 + (bx1-bx0)*dy
			dst.SetRGBA(x, y, color.RGBA{R: uint8(r + 0.5), G: uint8(g + 0.5), B: uint8(bl + 0.5), A: 255})
		}
	}
	return dst
}

// ppSquareCrop returns the center square of img.
func ppSquareCrop(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	side := w
	if h < w {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	type subImg interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImg); ok {
		return si.SubImage(image.Rect(x0, y0, x0+side, y0+side))
	}
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			dst.Set(x, y, img.At(x0+x, y0+y))
		}
	}
	return dst
}

// ----------------------------------------------------------------------------
// IQ SEND
// ----------------------------------------------------------------------------

// ppTargetJIDs returns the target ladder as (label, jid-string) pairs:
// omitted (empty JID), the account's PN JID, and its LID (LID-migrated
// accounts).
func ppTargetJIDs(client *whatsmeow.Client, pn types.JID) [][2]string {
	out := [][2]string{
		{"omitted", ""},
	}
	if !pn.IsEmpty() {
		out = append(out, [2]string{"pn", pn.String()})
	}
	if client.Store != nil && !client.Store.LID.IsEmpty() {
		out = append(out, [2]string{"lid", client.Store.LID.String()})
	}
	return out
}

// ppSendIQVariant sends ONE profile-picture SET IQ with the given target
// and node list. Returns the raw error (nil = server accepted).
func ppSendIQVariant(client *whatsmeow.Client, target types.JID, content []waBinary.Node, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := client.DangerousInternals().SendIQ(ctx, whatsmeow.DangerousInfoQuery{
		Namespace: "w:profile:picture",
		Type:      whatsmeow.DangerousInfoQueryType("set"),
		To:        types.ServerJID,
		Target:    target,
		Content:   content,
	})
	return err
}

// ppNodeSet builds the picture node list. preview==nil -> single node
// (real WA Web parity), else dual node (image+preview).
func ppNodeSet(data []byte, preview []byte) []waBinary.Node {
	nodes := []waBinary.Node{{
		Tag:     "picture",
		Attrs:   waBinary.Attrs{"type": "image"},
		Content: data,
	}}
	if len(preview) > 0 {
		nodes = append(nodes, waBinary.Node{
			Tag:     "picture",
			Attrs:   waBinary.Attrs{"type": "preview"},
			Content: preview,
		})
	}
	return nodes
}

// ppTryAllTargets runs the target ladder (omitted -> PN -> LID) for one
// node-set, returning the first success or the last error.
func ppTryAllTargets(client *whatsmeow.Client, pn types.JID, content []waBinary.Node, timeout time.Duration) error {
	var lastErr error
	for _, t := range ppTargetJIDs(client, pn) {
		var target types.JID
		if t[1] != "" {
			parsed, err := types.ParseJID(t[1])
			if err != nil {
				continue
			}
			target = parsed
		}
		err := ppSendIQVariant(client, target, content, timeout)
		if err == nil {
			return nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "timed out") ||
			strings.Contains(err.Error(), "context deadline exceeded") {
			time.Sleep(1 * time.Second)
			continue
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return lastErr
}

// ----------------------------------------------------------------------------
// THE LADDER
// ----------------------------------------------------------------------------

// ppTryLadder runs the adaptive ladder and returns nil when ANY (step ×
// target) combination is accepted by the server.
// Priority: aspect-fit whole picture (no crop, no blur) first; full-fit
// blurred square and center-crop only as fallbacks.
func ppTryLadder(client *whatsmeow.Client, data []byte, pn types.JID) error {
	aspectFit := ppEncodeAspectFitJPEG(data, 640, 90)
	fullFit := ppEncodeFullFitJPEG(data, 640, 90)
	cropClean := ppEncodeCleanJPEG(data, 640, 90)
	preview := ppEncodeCleanJPEG(data, 96, 90)

	steps := []struct {
		payload    []byte
		usePreview bool
	}{
		{aspectFit, false}, // 1. whole picture, aspect preserved - NO crop, NO blur
		{aspectFit, true},  // 2. same + preview node
		{fullFit, false},   // 3. whole pic in 640 square + blurred bg (fallback)
		{cropClean, false}, // 4. center-square crop (LAST RESORT)
	}
	for _, st := range steps {
		if len(st.payload) == 0 {
			continue
		}
		var pv []byte
		if st.usePreview {
			if len(preview) == 0 {
				continue
			}
			pv = preview
		}
		if err := ppTryAllTargets(client, pn, ppNodeSet(st.payload, pv), 12*time.Second); err == nil {
			return nil
		}
	}
	return errors.New("WhatsApp rejected every attempt (aspect-fit, full-fit and crop) - please try a different image")
}

// ----------------------------------------------------------------------------
// EXPORTED PANEL TOOLS
// ----------------------------------------------------------------------------

// RunPPLadder is the exported entry used by the panel's /ppdiag endpoint:
// it runs the full ladder on raw image bytes via curl.
func RunPPLadder(client *whatsmeow.Client, data []byte, pnStr string) error {
	pn := types.JID{}
	if j, err := types.ParseJID(pnStr); err == nil {
		pn = j
	}
	return ppTryLadder(client, data, pn)
}

// PPRawTest re-encodes data at its ORIGINAL dimensions (or scaled to fit
// within maxside, aspect preserved, when maxside > 0) using the Go stdlib
// encoder at the given quality, then sends ONE profile-picture SET IQ
// (target omitted). Returns (dims, bytes, err) - used by the panel's
// /ppraw endpoint to probe the server's acceptance rules.
func PPRawTest(client *whatsmeow.Client, data []byte, maxside int, quality int) (string, int, error) {
	img, err := ppDecodeImage(data)
	if err != nil || img == nil {
		return "", 0, fmt.Errorf("decode: %v", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxside > 0 && (w > maxside || h > maxside) {
		scale := float64(maxside) / float64(w)
		if s2 := float64(maxside) / float64(h); s2 < scale {
			scale = s2
		}
		w2 := int(float64(w)*scale + 0.5)
		h2 := int(float64(h)*scale + 0.5)
		if w2 < 1 {
			w2 = 1
		}
		if h2 < 1 {
			h2 = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, w2, h2))
		for y := 0; y < h2; y++ {
			sy := y * h / h2
			if sy >= h {
				sy = h - 1
			}
			for x := 0; x < w2; x++ {
				sx := x * w / w2
				if sx >= w {
					sx = w - 1
				}
				dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
			}
		}
		img = dst
		w, h = w2, h2
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return "", 0, err
	}
	jpg := out.Bytes()
	dims := fmt.Sprintf("%dx%d", w, h)
	err = ppSendIQVariant(client, types.JID{}, ppNodeSet(jpg, nil), 15*time.Second)
	return dims, len(jpg), err
}

// ----------------------------------------------------------------------------
// COMMAND HANDLER
// ----------------------------------------------------------------------------

// ppQuotedMessage returns the quoted (replied-to) message proto of the
// incoming message.
func ppQuotedMessage(raw *waProto.Message) *waProto.Message {
	if raw == nil {
		return nil
	}
	if raw.ExtendedTextMessage != nil && raw.ExtendedTextMessage.ContextInfo != nil {
		if q := raw.ExtendedTextMessage.ContextInfo.QuotedMessage; q != nil {
			return q
		}
	}
	return nil
}

// ppIsImageLike reports whether the quoted message carries an image-type
// media: imageMessage, documentMessage(image/*) or stickerMessage.
func ppIsImageLike(q *waProto.Message) bool {
	if q == nil {
		return false
	}
	if q.ImageMessage != nil {
		return true
	}
	if q.DocumentMessage != nil && q.DocumentMessage.Mimetype != nil &&
		strings.HasPrefix(*q.DocumentMessage.Mimetype, "image/") {
		return true
	}
	if q.StickerMessage != nil {
		return true
	}
	return false
}

func handlePP(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// 0) OWNER-ONLY - same rule as .vv (owner request: .pp was public)
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME*")
		return
	}

	// 1) No quoted message -> help text
	quotedID, _, hasQuote := s.GetQuotedMessageID(info)
	if !hasQuote || quotedID == "" {
		s.Reply(info, "🖼️ *SET PROFILE PICTURE*\n\n"+
			"*HOW TO USE:*\n"+
			"*1. SEND ANY IMAGE*\n"+
			"*2. REPLY WITH .PP*\n\n"+
			"*EXAMPLE:*\n"+
			"*[SEND IMAGE] → REPLY WITH .PP*")
		return
	}

	// 2) Image detection
	q := ppQuotedMessage(s.GetRawMessage(info))
	if !ppIsImageLike(q) {
		s.Reply(info, "❌ *REPLY TO AN IMAGE ONLY*")
		return
	}

	// 3) Download
	var data []byte
	if d, ok := s.DownloadImage(info); ok {
		data = d
	}
	if len(data) == 0 {
		if d, _, ok := s.DownloadQuotedMedia(info); ok {
			data = d
		}
	}
	if len(data) == 0 {
		s.Reply(info, "❌ *DOWNLOAD FAILED*")
		return
	}

	// 4) Client validation
	client := s.GetClient()
	if client == nil {
		s.Reply(info, "❌ *FAILED*\n*NOT CONNECTED*")
		return
	}
	botJID, err := types.ParseJID(s.GetJID())
	if err != nil {
		s.Reply(info, "❌ *FAILED*\n*"+err.Error()+"*")
		return
	}

	// 5) Ladder: whole picture first, no crop, no blur
	if err := ppTryLadder(client, data, botJID); err != nil {
		report := err.Error()
		if strings.Contains(report, "context deadline exceeded") ||
			strings.Contains(report, "timed out") {
			report = "WhatsApp did not respond (timeout)"
		}
		s.Reply(info, "❌ *FAILED*\n*"+report+"*")
		return
	}
	s.Reply(info, "✅ *PROFILE PIC CHANGED SUCCESS*")
}

func init() {
	Register(Command{Name: "pp", Category: "OWNER & SYSTEM", Desc: "Set profile pic from replied image (full pic, no crop)", Run: handlePP})
	Register(Command{Name: "setpp", Category: "OWNER & SYSTEM", Hidden: true, Run: handlePP})
	Register(Command{Name: "setmypp", Category: "OWNER & SYSTEM", Hidden: true, Run: handlePP})
	Register(Command{Name: "mypp", Category: "OWNER & SYSTEM", Hidden: true, Run: handlePP})
	Register(Command{Name: "changepp", Category: "OWNER & SYSTEM", Hidden: true, Run: handlePP})
}
