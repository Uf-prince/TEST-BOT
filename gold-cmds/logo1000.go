package goldcmds

// ============================================================================
// GOLD-MD — 1000 NAME LOGO DESIGNS: prompt engine + .logo list command
// File: logo1000.go
// ============================================================================
// OWNER ORDER: akela .logo likhne pe LIST message (1000 tak). logoN <name>
// se us design ka logo banta hai (logo1000_keys.go wali 15-key pool se).
//
// DESIGN UNIQUENESS: N-1 = base + 100×t, jahan:
//   base = (N-1) % 100   → 100 hand-crafted alag-alag scene concepts
//   t    = (N-1) / 100   → 10 alag-alag text treatments (name kaisa likha)
//   l    = (b+t) % 10    → 10 alag-alag lighting moods
// Har (base, t) pair sirf ek hi N pe aata hai — 1000 designs, koi repeat nahi.
// ============================================================================

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── 100 base scene concepts (har ek bilkul alag design) ───────────────────
var logoBases = [100]string{
	// 1-10: FIRE / POWER
	"epic explosion of golden flames and fire wings spreading behind the letters",
	"neon cyberpunk city grid with futuristic skyline and glowing digital lines",
	"royal golden crown resting on a red velvet throne with jewels",
	"fierce dragon breathing fire across the scene, scales glowing",
	"majestic lion head mascot with golden mane flowing",
	"mighty eagle with spread wings soaring, feathers detailed",
	"phoenix rising from burning ashes with fiery trail",
	"tiger face with bold orange and black stripes pattern",
	"wolf howling at a giant full moon on a cliff",
	"skull with glowing ember eyes and smoke wisps",
	// 11-20: STORM / NATURE FORCES
	"massive lightning storm with thunder clouds and electric bolts everywhere",
	"volcanic eruption with rivers of glowing lava and flying embers",
	"frozen glacier world with sharp ice crystals and snowfall",
	"paradise jungle waterfall with mist and tropical leaves",
	"deep space galaxy with swirling nebula and countless stars",
	"astronaut floating in space with earth glowing below",
	"futuristic battle mech robot with glowing core and armor plates",
	"samurai warrior with katana drawn, cherry petals in wind",
	"shadow ninja with twin blades and swirling dark smoke",
	"medieval knight in shining armor before a castle gate",
	// 21-30: ROYAL / LUXURY
	"giant golden king chess piece on a chessboard of marble",
	"soccer stadium at night with blazing floodlights and crowd lights",
	"cricket stadium with stumps, bat and flying dust under stadium lights",
	"basketball on fire bouncing on a court with sparks",
	"supercar in a neon-lit garage with chrome reflections",
	"chromed motorcycle engine with spinning wheels and heat haze",
	"fighter jet flying through clouds at supersonic speed",
	"heavy tank on a battlefield with dust and dramatic sky",
	"pirate ship sailing through a thunder storm on huge waves",
	"ancient temple ruins with carved stone pillars and vines",
	// 31-40: ANCIENT / CIVILIZATIONS
	"egyptian pyramids with pharaoh mask and golden hieroglyphs",
	"roman colosseum arena with gladiator swords and sand dust",
	"greek marble statue warrior with laurel wreath",
	"casino royal table with golden poker chips and playing cards",
	"towering stacks of gold bars and falling dollar bills",
	"luxury diamond jewelry display with sparkling gemstones",
	"red roses garden with petals floating in the air",
	"cherry blossom sakura trees with pink petals falling",
	"misty bamboo forest with soft rays of light",
	"golden desert sand dunes with a caravan silhouette at dusk",
	// 41-50: CITIES / NIGHT
	"rainy night city skyline with reflections on wet streets",
	"tokyo street packed with glowing neon signs and lanterns",
	"underwater coral reef with tropical fish and light rays",
	"tropical island beach at sunset with palm trees",
	"snowy mountain peak with an eagle circling above clouds",
	"mysterious foggy forest with glowing fireflies",
	"haunted victorian mansion with ghosts and green mist",
	"spooky halloween pumpkins with carved faces and candles",
	"cozy christmas winter scene with snow and warm lights",
	"birthday celebration with a huge cake and lit candles",
	// 51-60: MUSIC / PARTY
	"dj turntables with mixing party lights and crowd hands up",
	"electric guitar on a rock concert stage with spotlights",
	"premium headphones with colorful sound waves flowing",
	"studio microphone with pop filter and warm studio glow",
	"retro vinyl record spinning with music notes floating",
	"gaming controller and esports arena with huge screens",
	"chessboard mid-battle with pieces knocked over dramatically",
	"ancient magic library with floating glowing books",
	"alchemy laboratory with bubbling potions and smoke",
	"wizard hat with magic stars and sparkling dust",
	// 61-70: MYSTIC / SPIRITUAL
	"crystal ball glowing with mystical fortune mist",
	"tarot cards spread on a velvet cloth with candles",
	"yin yang symbol with swirling black and white energy",
	"buddha statue in peaceful meditation with lotus flowers",
	"stained glass window with light streaming through colors",
	"islamic geometric art pattern with gold filigree",
	"om symbol carved in stone with spiritual energy rings",
	"royal jewels and crown on purple velvet cushion",
	"flowing silk fabric with golden thread embroidery",
	"luxury marble villa interior with grand staircase",
	// 71-80: LIFESTYLE / CELEBRATION
	"private jet on a runway at golden sunset",
	"luxury yacht on the open ocean at sunrise",
	"champagne bottles popping with golden bubbles flying",
	"grand fireworks exploding across the night sky",
	"aurora borealis northern lights dancing over mountains",
	"colorful rainbow arching over a valley after rain",
	"solar eclipse with dark sun and corona glow",
	"full moon night with silhouetted wolves in mist",
	"steampunk brass gears and clockwork machinery with steam",
	"retro 80s synthwave sun with horizontal grid lines",
	// 81-90: STYLES / ART
	"vaporwave pastel landscape with retro statues and grid",
	"pixel art 8-bit retro video game world",
	"comic book pop art scene with pow effects and halftone dots",
	"graffiti street art wall with spray paint splashes",
	"vintage chalkboard with hand-drawn flourishes",
	"film noir cinematic scene with venetian blind shadows",
	"old parchment scroll with wax seal and quill ink",
	"neon OPEN sign glowing in a dark bar window",
	"las vegas casino strip with marquee lights at night",
	"miami beach sunset with palm trees and ocean drive",
	// 91-100: WORLD / CALLIGRAPHY
	"dubai skyline with the tallest tower glowing at night",
	"eiffel tower in paris with soft evening lights",
	"new york times square with giant billboards",
	"london bridge in dramatic fog at night",
	"great wall of china winding through mountains mist",
	"taj mahal reflecting in water at sunrise",
	"intricate mandala pattern with sacred geometry",
	"kaleidoscope of colors exploding symmetrically",
	"flowing arabic calligraphy ink art on textured canvas",
	"abstract flame of fire swirling with energy sparks",
}

// ── 10 text treatments (NAME ke letters ka look) ─────────────────────────
var logoTexts = [10]string{
	"the name written in massive bold 3D metallic golden letters",
	"the name written in glowing neon tube letters, cyan and magenta",
	"the name written in elegant luxury gold calligraphy script letters",
	"the name written in liquid chrome metal reflective letters",
	"the name written in fiery burning letters made of living flames",
	"the name written in frozen ice letters made of crystals and frost",
	"the name written in graffiti spray paint street art letters",
	"the name written on an embossed vintage brass metal plate",
	"the name written in pixel 8-bit retro video game letters",
	"the name carved in ancient cracked stone letters with moss",
}

// ── 10 lighting moods (poore scene ka mood) ──────────────────────────────
var logoLights = [10]string{
	"dramatic cinematic rim lighting on a dark background",
	"warm golden hour glow with sunset tones",
	"cool blue moonlight tones in night atmosphere",
	"high contrast black and white with one bold red accent",
	"vibrant colorful neon rainbow glow everywhere",
	"soft dreamy bokeh particles floating in background",
	"explosive bright highlights with deep dark shadows",
	"purple and pink synthwave gradient glow",
	"green matrix digital code rain glow",
	"amber and red fire glow with rising smoke",
}

var logoBaseNames = [100]string{
	"Golden Fire Wings", "Neon Cyber City", "Royal Crown Throne", "Dragon Fire",
	"Lion King Mane", "Eagle Wings", "Phoenix Rising", "Tiger Stripes",
	"Wolf Moon", "Ember Skull", "Thunder Storm", "Volcano Lava",
	"Frozen Glacier", "Jungle Waterfall", "Galaxy Nebula", "Astronaut Space",
	"Battle Mech", "Samurai Blade", "Shadow Ninja", "Knight Castle",
	"King Chess", "Soccer Stadium", "Cricket Ground", "Basketball Fire",
	"Supercar Neon", "Chrome Motorcycle", "Fighter Jet", "War Tank",
	"Pirate Storm", "Temple Ruins", "Pharaoh Pyramids", "Gladiator Arena",
	"Greek Statue", "Casino Royal", "Gold Bars", "Diamond Luxury",
	"Red Roses", "Sakura Blossom", "Bamboo Mist", "Desert Dunes",
	"Rainy Skyline", "Tokyo Neon", "Coral Reef", "Island Sunset",
	"Snow Peak", "Foggy Forest", "Haunted Mansion", "Halloween Pumpkins",
	"Christmas Winter", "Birthday Cake", "DJ Party", "Rock Guitar",
	"Headphones Beat", "Studio Mic", "Vinyl Retro", "Esports Arena",
	"Chess Battle", "Magic Library", "Alchemy Lab", "Wizard Magic",
	"Crystal Fortune", "Tarot Mystic", "Yin Yang", "Buddha Zen",
	"Stained Glass", "Islamic Geometry", "Om Spiritual", "Royal Jewels",
	"Silk Gold", "Marble Villa", "Private Jet", "Ocean Yacht",
	"Champagne Pop", "Fireworks Sky", "Aurora Lights", "Rainbow Valley",
	"Solar Eclipse", "Full Moon Night", "Steampunk Gears", "Synthwave 80s",
	"Vaporwave Dream", "Pixel 8-Bit", "Comic Pop Art", "Graffiti Street",
	"Chalk Vintage", "Film Noir", "Parchment Scroll", "Neon Bar Sign",
	"Vegas Casino", "Miami Sunset", "Dubai Tower", "Eiffel Paris",
	"Times Square", "London Fog", "Great Wall", "Taj Mahal",
	"Mandala Sacred", "Kaleidoscope", "Calligraphy Ink", "Abstract Flame",
}

var logoTextNames = [10]string{
	"3D Gold Metal", "Neon Tube", "Gold Calligraphy", "Liquid Chrome",
	"Living Flames", "Ice Crystal", "Graffiti Spray", "Brass Plate",
	"Pixel 8-Bit", "Ancient Stone",
}

var logoLightNames = [10]string{
	"Cinematic Dark", "Golden Hour", "Moonlight Blue", "Noir Red Accent",
	"Neon Rainbow", "Dreamy Bokeh", "Explosive Contrast", "Synthwave Glow",
	"Matrix Code", "Fire Smoke",
}

// LogoPromptForN builds the full dhamakedar prompt for design N with the
// user's name. Text spelling is emphasized so the AI writes the name right.
func LogoPromptForN(n int, name string) string {
	if n < 1 || n > LogoCount {
		n = 1
	}
	b := (n - 1) % 100
	t := (n - 1) / 100
	l := (b + t) % 10
	return fmt.Sprintf(
		"Epic premium logo design, %s. Centered main title: %s spelling exactly \"%s\", "+
			"perfectly readable, large and prominent, centered composition. Scene: %s. %s. "+
			"Hyper-detailed professional 8K logo art, strong depth, sharp focus on the name, "+
			"no other text or watermark anywhere",
		logoTexts[t], name, strings.ToUpper(name), logoBases[b], logoLights[l])
}

// LogoDesignName returns the human-readable design name for caption.
func LogoDesignName(n int) string {
	if n < 1 || n > LogoCount {
		n = 1
	}
	b := (n - 1) % 100
	t := (n - 1) / 100
	l := (b + t) % 10
	return fmt.Sprintf("%s + %s + %s", logoBaseNames[b], logoTextNames[t], logoLightNames[l])
}

// ── .logo — list command (OWNER SPEC format, akela message) ──────────────
func handleLogoList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	var sb strings.Builder
	sb.WriteString("*CREATE TEXT TO IMAGE ( NAME LOGO )*\n\n")
	sb.WriteString("*TYPE SAME LIKE THAT*\n\n")
	for n := 1; n <= LogoCount; n++ {
		sb.WriteString(fmt.Sprintf("*%sLOGO%d ❮ YOUR NAME ❯*\n", prefix, n))
	}
	s.Reply(info, sb.String())
}

// ── logoN <name> — generate that design's logo ───────────────────────────
// LogoRunN is called from the main package's hidden logo1..logo1000
// registrations (commands map) — gold-cmds side exposed entry point.
func LogoRunN(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	if n < 1 || n > LogoCount {
		s.Reply(info, "*LOGO NUMBER 1 SE 1000 TAK HI HAI*")
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		logoRunNAsync(s, info, args, prefix, n)
	}()
	select {
	case <-done:
	case <-time.After(logoTimeout):
		s.Reply(info, "*TRY AGAIN LATER*")
	}
}

func logoRunNAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, n int) {
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		s.Reply(info, fmt.Sprintf(
			"*⬛ LOGO %d — %s ⬛*\n\n"+
				"*IS DESIGN ME NAME LOGO BANANE KE LIYE:*\n"+
				"*❯ %slogo%d ❮ YOUR NAME ❯*\n\n"+
				"*EXAMPLE:*\n"+
				"*❯ %slogo%d UMAR*\n\n"+
				"*TYPE .LOGO SE POORI LIST DEKHO — 1000 DHAMAKEDAR DESIGNS*",
			n, LogoDesignName(n), prefix, n, prefix, n))
		return
	}
	if len(name) > logoMaxNameLen {
		name = name[:logoMaxNameLen]
	}
	nameUp := strings.ToUpper(name)

	waitID := s.ReplyWithID(info, fmt.Sprintf("*⬛ LOGO %d BAN RAHA HAI........*", n))

	deadline := time.Now().Add(logoTimeout)
	for time.Now().Before(deadline) {
		imgData, waitMs, totalWaitMs, busyOnly, err := LogoGenerateImage(LogoPromptForN(n, nameUp))
		if err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "⬛ *LOGO COMMAND ERROR* ⬛\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if imgData != nil {
			s.DeleteMessage(info, waitID)
			caption := fmt.Sprintf(
				"*⬛ LOGO %d — DHAMAKEDAR NAME LOGO ⬛*\n\n"+
					"*⬛ NAME:* %s\n"+
					"*⬛ DESIGN:* %s\n"+
					"*⬛ TIME:* %.1fs\n"+
					"*⬛ SIZE:* 2K\n\n"+
					"*❯❯ DHAMAKEDAR LOGO READY ⬛⬛*",
				n, nameUp, LogoDesignName(n), time.Since(deadline.Add(-logoTimeout)).Seconds())
			if sendErr := s.SendImage(info, imgData, caption); sendErr != nil {
				s.Reply(info, "⬛ *LOGO SEND ERROR* ⬛\n"+strings.ToUpper(sendErr.Error()))
			}
			return
		}
		// All keys busy/resting — edit wait message, sleep, retry
		var waitText string
		if busyOnly {
			waitText = "*LOGO AI SERVER BUSY HAI, THODI DER ME BANEGA........*"
		} else {
			waitText = fmt.Sprintf("*LOGO AI SERVER BUSY HAI — %s ME RETRY HO RAHA HAI........*", formatWaitMs(totalWaitMs))
		}
		s.EditMessage(info, waitID, waitText)
		waitDuration := time.Duration(waitMs+500) * time.Millisecond
		if waitDuration < 1*time.Second {
			waitDuration = 1 * time.Second
		}
		time.Sleep(waitDuration)
		s.EditMessage(info, waitID, fmt.Sprintf("*⬛ LOGO %d BAN RAHA HAI........*", n))
	}
	s.DeleteMessage(info, waitID)
	s.Reply(info, "*TRY AGAIN LATER*")
}

func init() {
	// OWNER ORDER: .menu aur .fullmenu me SIRF .logo dikhta hai (desc ke sath
	// fullmenu me). logo1..logo1000 main-package Commands map me hidden
	// hote hain (logo1000_main.go) — dono menus me kabhi nahi dikhte.
	Register(Command{
		Name:     "logo",
		Category: "AI & MEDIA",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF 1000 DHAMAKEDAR NAME LOGO DESIGNS. IT SHOWS .LOGO1 TO .LOGO1000 — TYPE ANY LOGO NUMBER WITH YOUR NAME TO MAKE A UNIQUE DHAMAKEDAR TEXT LOGO.",
		Run:      handleLogoList,
	})
}
