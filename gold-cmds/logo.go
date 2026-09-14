package goldcmds

// ============================================================================
// GOLD-MD — LOGO1 & LOGO2 Commands (Pollinations AI + Go text overlay)
// File: logo.go
// ----------------------------------------------------------------------------
// 2 dhamakedar text-logo styles — EACH COMPLETELY DIFFERENT:
//
//   .logo1 UMAR      → Royal Golden Crown style (Serif font, big 160px, lower-center, gold glow+3D)
//   .logo2 GOLD BOT  → Neon Cyberpunk Grid style (Mono font, 120px, top-center, neon cyan glow)
//
// HOW IT WORKS (hybrid for 100% accurate text):
//   1. Pollinations AI generates a themed background/design (free, no API key)
//   2. Go renders the user's text on top using a style-specific TTF font,
//      style-specific size, style-specific position, and style-specific effect
//   3. Final composite image is sent to WhatsApp
//
// Each style has a DIFFERENT font, size, position, and text effect —
// so logo1 and logo2 look completely different, not just color-swapped.
// ============================================================================

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"go.mau.fi/whatsmeow/types"
)

// ── Font paths (different font per style for distinct look) ──────────────────
const (
	fontSerifBold = "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf"    // logo1 — royal serif
	fontMonoBold  = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf" // logo2 — cyberpunk mono
)

// ── Logo style definitions (2 completely different styles) ───────────────────
type logoStyle struct {
	Num       int
	Name      string
	BgPrompt  string // Pollinations background prompt (NO text — just design)
	Seed      int64
	FontPath  string  // which TTF font to use
	FontSize  float64 // exact font size in points
	PosX      int     // text X center position on 1024x1024 canvas
	PosY      int     // text Y center position on 1024x1024 canvas
	TextColor string  // main text color hex
	GlowColor string  // glow/shadow color hex
	Effect    string  // text rendering effect: "gold_3d" or "neon"
	PosLabel  string  // shown in caption
}

var logoStyles = []logoStyle{
	{
		Num:       1,
		Name:      "Royal Golden Crown",
		BgPrompt:  "a premium royal golden crown emblem placed at the TOP of the image on a dark black background, luxurious gold particles, glowing royal aesthetic, ornate decorative golden frame border around edges, 4k ultra detailed, no text anywhere, the LOWER HALF of the image is mostly empty dark space for text overlay, metallic gold and black theme, vertical composition",
		Seed:      2001,
		FontPath:  fontSerifBold,
		FontSize:  160,
		PosX:      512,
		PosY:      760, // lower-center — crown upar, text neeche
		TextColor: "#FFD700",
		GlowColor: "#B8860B",
		Effect:    "gold_3d",
		PosLabel:  "Royal Gold (Serif, big, lower)",
	},
	{
		Num:       2,
		Name:      "Neon Cyberpunk Grid",
		BgPrompt:  "a cyberpunk neon grid background with futuristic digital city skyline at the BOTTOM of the image, purple and cyan glowing lines, dark atmosphere, neon light reflections, 4k ultra detailed, no text anywhere, the UPPER HALF of the image is mostly empty dark space for text overlay, retro futuristic aesthetic, vertical composition",
		Seed:      2002,
		FontPath:  fontMonoBold,
		FontSize:  120,
		PosX:      512,
		PosY:      240, // top-center — city neeche, text upar
		TextColor: "#00FFFF",
		GlowColor: "#FF00FF",
		Effect:    "neon",
		PosLabel:  "Cyberpunk Neon (Mono, top)",
	},
}

// ── Font loading helpers ─────────────────────────────────────────────────────

// fontCache caches parsed font faces by path+size to avoid re-parsing each call.
var fontCache = map[string]font.Face{}

// getFace returns a font.Face for the given TTF path at the given size (in points).
func getFace(path string, size float64) (font.Face, error) {
	key := fmt.Sprintf("%s@%.0f", path, size)
	if f, ok := fontCache[key]; ok {
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("font read %s: %w", path, err)
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("font parse %s: %w", path, err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("face %s: %w", path, err)
	}
	fontCache[key] = face
	return face, nil
}

// ── Color & image helpers ────────────────────────────────────────────────────

// hexToColor parses a #RRGGBB hex string to color.RGBA.
func hexToColor(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.RGBA{255, 255, 255, 255}
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

// fetchBackground downloads the Pollinations-generated background image.
func fetchBackground(prompt string, seed int64) (image.Image, error) {
	u := fmt.Sprintf("%s/%s", pollinationsBase, url.PathEscape(prompt))
	params := url.Values{}
	params.Set("width", "1024")
	params.Set("height", "1024")
	params.Set("model", "flux")
	params.Set("seed", fmt.Sprintf("%d", seed))
	params.Set("nologo", "true")
	fullURL := u + "?" + params.Encode()

	req, _ := http.NewRequest("GET", fullURL, nil)
	req.Header.Set("User-Agent", pollinationsUA)
	if pollinationsKey != "" {
		req.Header.Set("Authorization", "Bearer "+pollinationsKey)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 || len(data) < 500 {
		return nil, fmt.Errorf("pollinations status %d len %d", resp.StatusCode, len(data))
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("jpeg decode: %w", err)
	}
	return img, nil
}

// measureText returns the pixel width of the text using the given face.
func measureText(face font.Face, s string) int {
	d := font.Drawer{Dst: image.NewRGBA(image.Rect(0, 0, 1, 1)), Face: face}
	adv := d.MeasureString(s)
	return adv.Round()
}

// fitFace auto-shrinks the font size until the text fits within maxW pixels.
// Returns a face at the best-fitting size (never larger than startSize).
func fitFace(path string, startSize float64, text string, maxW int) (font.Face, error) {
	size := startSize
	for size > 30 {
		face, err := getFace(path, size)
		if err != nil {
			return nil, err
		}
		if measureText(face, text) <= maxW {
			return face, nil
		}
		size -= 10
	}
	// fallback: smallest reasonable size
	return getFace(path, 30)
}

// drawTextAt draws text centered horizontally at (cx,cy) on dst.
// cy is the visual center (baseline adjusted).
func drawTextAt(dst *image.RGBA, face font.Face, text string, cx, cy int, col color.RGBA) {
	tw := measureText(face, text)
	x := cx - tw/2
	h := face.Metrics().Height.Round()
	y := cy + h/4
	d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(col)}
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}

// ── Text effects (each style gets a completely different look) ───────────────

// renderGold3D: thick dark outline + warm gold glow + 3D drop shadow + main gold.
// Royal, luxurious, bold — for the crown style.
func renderGold3D(dst *image.RGBA, face font.Face, text string, cx, cy int, main, glow color.RGBA) {
	tw := measureText(face, text)
	x := cx - tw/2
	h := face.Metrics().Height.Round()
	y := cy + h/4

	// 1. 3D drop shadow (offset down-right, dark)
	shadowCol := color.RGBA{0, 0, 0, 180}
	for dx := 8; dx >= 4; dx -= 2 {
		d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(shadowCol)}
		d.Dot = fixed.P(x+dx, y+dx)
		d.DrawString(text)
	}

	// 2. Thick dark outline (8 directions, offset 3) for maximum contrast
	outlineCol := color.RGBA{20, 10, 0, 240}
	for off := 3; off >= 1; off-- {
		for _, dx := range []int{-off, 0, off} {
			for _, dy := range []int{-off, 0, off} {
				if dx == 0 && dy == 0 {
					continue
				}
				d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(outlineCol)}
				d.Dot = fixed.P(x+dx, y+dy)
				d.DrawString(text)
			}
		}
	}

	// 3. Warm gold glow (medium spread, amber tone)
	glowCol := color.RGBA{glow.R, glow.G, glow.B, 120}
	for _, off := range []int{6, 4, 2} {
		for _, dx := range []int{-off, 0, off} {
			for _, dy := range []int{-off, 0, off} {
				if dx == 0 && dy == 0 {
					continue
				}
				d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(glowCol)}
				d.Dot = fixed.P(x+dx, y+dy)
				d.DrawString(text)
			}
		}
	}

	// 4. Main gold text on top
	d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(main)}
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}

// renderNeon: wide neon glow (large spread) + bright magenta edge + sharp cyan core.
// Glowing, electric, cyberpunk — no dark outline (neon is light-on-dark).
func renderNeon(dst *image.RGBA, face font.Face, text string, cx, cy int, main, glow color.RGBA) {
	tw := measureText(face, text)
	x := cx - tw/2
	h := face.Metrics().Height.Round()
	y := cy + h/4

	// 1. Wide outer neon glow (big spread, low alpha) — the "bloom"
	for off := 12; off >= 5; off -= 2 {
		alpha := uint8(40 + (12-off)*15) // more alpha as we get closer
		neonGlow := color.RGBA{glow.R, glow.G, glow.B, alpha}
		for _, dx := range []int{-off, 0, off} {
			for _, dy := range []int{-off, 0, off} {
				if dx == 0 && dy == 0 {
					continue
				}
				d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(neonGlow)}
				d.Dot = fixed.P(x+dx, y+dy)
				d.DrawString(text)
			}
		}
	}

	// 2. Bright magenta edge (medium offset, full alpha) — neon rim
	edgeCol := color.RGBA{glow.R, glow.G, glow.B, 255}
	for off := 3; off >= 1; off-- {
		for _, dx := range []int{-off, 0, off} {
			for _, dy := range []int{-off, 0, off} {
				if dx == 0 && dy == 0 {
					continue
				}
				d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(edgeCol)}
				d.Dot = fixed.P(x+dx, y+dy)
				d.DrawString(text)
			}
		}
	}

	// 3. Sharp cyan core (the bright neon text itself)
	d := font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(main)}
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}

// renderEffect dispatches to the correct effect renderer based on style.
func renderEffect(dst *image.RGBA, st logoStyle, face font.Face, text string) {
	main := hexToColor(st.TextColor)
	glow := hexToColor(st.GlowColor)
	switch st.Effect {
	case "gold_3d":
		renderGold3D(dst, face, text, st.PosX, st.PosY, main, glow)
	case "neon":
		renderNeon(dst, face, text, st.PosX, st.PosY, main, glow)
	default:
		drawTextAt(dst, face, text, st.PosX, st.PosY, main)
	}
}

// ── Composite: background + text overlay ─────────────────────────────────────

// compositeLogo builds the final logo: background + overlaid text with style-specific font/size/position/effect.
func compositeLogo(bg image.Image, st logoStyle, text string) ([]byte, error) {
	// Scale background to 1024x1024 (Pollinations sometimes returns 768)
	bounds := bg.Bounds()
	if bounds.Dx() != 1024 || bounds.Dy() != 1024 {
		scaled := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
		sx := float64(bounds.Dx()) / 1024.0
		sy := float64(bounds.Dy()) / 1024.0
		for y := 0; y < 1024; y++ {
			for x := 0; x < 1024; x++ {
				srcX := int(float64(x) * sx)
				srcY := int(float64(y) * sy)
				if srcX >= bounds.Dx() {
					srcX = bounds.Dx() - 1
				}
				if srcY >= bounds.Dy() {
					srcY = bounds.Dy() - 1
				}
				scaled.Set(x, y, bg.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
			}
		}
		bg = scaled
	}
	dst := image.NewRGBA(bg.Bounds())
	draw.Draw(dst, dst.Bounds(), bg, image.Point{}, draw.Src)

	// Fit font: auto-shrink if text is too wide for 900px
	face, err := fitFace(st.FontPath, st.FontSize, strings.ToUpper(text), 900)
	if err != nil {
		return nil, fmt.Errorf("font load: %w", err)
	}

	// Render the style-specific effect
	renderEffect(dst, st, face, strings.ToUpper(text))

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Command handler ──────────────────────────────────────────────────────────

var activeLogoIdx = -1

func handleLogoN(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleLogoNAsync(s, info, args, prefix)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		s.Reply(info, "*LOGO TIMEOUT — TRY AGAIN*")
	}
}

func handleLogoNAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	idx := activeLogoIdx
	if idx < 0 || idx >= len(logoStyles) {
		s.Reply(info, "*LOGO STYLE NOT FOUND*")
		return
	}
	st := logoStyles[idx]

	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		s.Reply(info, fmt.Sprintf(
			"*🔰 LOGO %d — %s*\n\n"+
				"*IS STYLE ME TEXT LOGO BANANE KE LIYE:*\n"+
				"*❯ %slogo%d <your text>*\n\n"+
				"*EXAMPLE:*\n"+
				"*❯ %slogo%d UMAR*\n"+
				"*❯ %slogo%d GOLD BOT*\n\n"+
				"*Jo text likhoge wahi image pe likha aayega is dhamakedar style me! 🔰🔰*",
			st.Num, st.Name, prefix, st.Num, prefix, st.Num, prefix, st.Num))
		return
	}

	// limit text length
	if len(text) > 40 {
		text = text[:40]
	}

	waitID := s.ReplyWithID(info, fmt.Sprintf(
		"*🔰 LOGO %d — %s*\n*TEXT:* %s\n*DHAMAKEDAR LOGO BAN RAHA HAI... 🔰🔰*",
		st.Num, st.Name, strings.ToUpper(text)))

	start := time.Now()

	// 1. fetch background from Pollinations
	bg, err := fetchBackground(st.BgPrompt, st.Seed)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, fmt.Sprintf("🔰 *LOGO %d ERROR*\nBackground: %v", st.Num, err))
		return
	}

	// 2. composite text on top with style-specific font/size/position/effect
	finalBytes, err := compositeLogo(bg, st, text)
	if err != nil {
		s.DeleteMessage(info, waitID)
		s.Reply(info, fmt.Sprintf("🔰 *LOGO %d ERROR*\nComposite: %v", st.Num, err))
		return
	}

	elapsed := time.Since(start)
	s.DeleteMessage(info, waitID)

	caption := fmt.Sprintf(
		"*🔰 LOGO %d — %s 🔰*\n\n"+
			"*🔰 TEXT:* %s\n"+
			"*🔰 STYLE:* %s\n"+
			"*🔰 FONT:* %s | %.0fpx\n"+
			"*🔰 POSITION:* %s\n"+
			"*🔰 EFFECT:* %s\n"+
			"*🔰 AI BG:* Pollinations Flux\n"+
			"*🔰 TIME:* %.1fs\n"+
			"*🔰 SIZE:* 1024×1024\n\n"+
			"*❯❯ DHAMAKEDAR LOGO READY 🔰🔰*",
		st.Num, st.Name, strings.ToUpper(text), st.Name, fontName(st.FontPath), st.FontSize, st.PosLabel, effectLabel(st.Effect), elapsed.Seconds())

	if err := s.SendImage(info, finalBytes, caption); err != nil {
		s.Reply(info, fmt.Sprintf("🔰 *LOGO %d SEND ERROR*\n%s", st.Num, err.Error()))
	}
}

// fontName returns a human-friendly name for a font path.
func fontName(path string) string {
	switch path {
	case fontSerifBold:
		return "DejaVu Serif Bold"
	case fontMonoBold:
		return "DejaVu Mono Bold"
	default:
		return "Default Bold"
	}
}

// effectLabel returns a human-friendly name for an effect.
func effectLabel(e string) string {
	switch e {
	case "gold_3d":
		return "Gold 3D Shadow"
	case "neon":
		return "Neon Glow Bloom"
	default:
		return "Plain"
	}
}

func makeLogoHandler(idx int) func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	return func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
		activeLogoIdx = idx
		handleLogoN(s, info, args, prefix)
	}
}

func init() {
	logoDescs := map[int]string{
		1: "THIS COMMAND IS USED TO MAKE A ROYAL GOLDEN CROWN STYLE TEXT LOGO. TYPE YOUR NAME AFTER IT.",
		2: "THIS COMMAND IS USED TO MAKE A NEON CYBERPUNK STYLE TEXT LOGO. TYPE YOUR NAME AFTER IT.",
	}
	for i, st := range logoStyles {
		name := fmt.Sprintf("logo%d", st.Num)
		Register(Command{
			Name:     name,
			Category: "AI & MEDIA",
			Desc:     logoDescs[st.Num],
			Run:      makeLogoHandler(i),
		})
	}
	// list all styles
	Register(Command{
		Name:     "logolist",
		Category: "AI & MEDIA",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF ALL TEXT LOGO STYLES. IT SHOWS EVERY STYLE WITH ITS NAME AND LOOK.",
		Run: func(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
			var sb strings.Builder
			sb.WriteString("*🔰 GOLD-MD LOGO STYLES — 2 DHAMAKEDAR DESIGNS 🔰*\n\n")
			sb.WriteString("*Har style me font, size, position aur effect sab alag hai! 🔰*\n\n")
			for _, st := range logoStyles {
				sb.WriteString(fmt.Sprintf(" *❯ %slogo%d* — %s\n", prefix, st.Num, st.Name))
				sb.WriteString(fmt.Sprintf("    🔰 %s (%.0fpx) | 🔰 %s | 🔰 %s\n",
					fontName(st.FontPath), st.FontSize, st.PosLabel, effectLabel(st.Effect)))
			}
			sb.WriteString("\n*EXAMPLE:*\n")
			sb.WriteString(fmt.Sprintf(" *❯ %slogo1 UMAR*\n", prefix))
			sb.WriteString(fmt.Sprintf(" *❯ %slogo2 GOLD BOT*\n\n", prefix))
			sb.WriteString("*Dono logos bilkul alag dikhenge — font, size, jagah, effect sab different! 🔰🔰*")
			s.Reply(info, sb.String())
		},
	})
}
