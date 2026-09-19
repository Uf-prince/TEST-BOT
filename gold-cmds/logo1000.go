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

// ── 10 text materials (NAME ke letters KIS CHEEZ se bane hain) ──────────
// OWNER ORDER: font sab ka same aa raha tha — isliye material aur font
// alag dimensions hain ab (neeche logoFonts).
var logoTexts = [10]string{
	"letters made of solid 3D metallic gold with beveled edges and polished shine",
	"letters made of glowing neon light tubes in cyan and magenta",
	"letters made of elegant flowing gold calligraphy ink with hand-lettered swashes",
	"letters made of liquid mirror chrome, reflective and curved like mercury",
	"letters made of living fire and burning flames with flying embers",
	"letters made of clear frozen ice crystals with frost and hanging icicles",
	"letters painted in colorful graffiti spray paint with drips and bold outlines",
	"letters stamped and embossed on a vintage brass metal plate",
	"letters built from chunky 8-bit pixel blocks like a retro video game",
	"letters carved deep into ancient cracked grey stone with moss and dust",
}

// ── 10 FONT STYLES (har design ka letterform alag — owner order) ─────────
var logoFonts = [10]string{
	"heavy blocky sans-serif letterforms with thick wide strokes",
	"thin elegant rounded futuristic letterforms",
	"flowing cursive script letterforms with long swash tails",
	"wide stretched geometric letterforms, modern and minimal",
	"sharp aggressive angular letterforms with jagged spikes and metal edges",
	"tall condensed serif letterforms with sharp pointed serifs",
	"bubbly rounded cartoon letterforms with bold outlines and playful curves",
	"classic engraved roman capital letterforms in letterpress stamp style",
	"chunky square pixel letterforms, blocky and retro",
	"rough hand-chiseled capital letterforms in ancient carved style",
}

// ── 6 FONT SIZES (chota se bara tak variety — owner order) ───────────────
var logoSizes = [6]string{
	"small crisp lettering, neat and minimal",
	"medium-sized lettering, clean and balanced",
	"large lettering dominating the center",
	"extra-large massive lettering filling most of the image width",
	"giant oversized lettering stretching edge to edge",
	"bold big lettering with strong powerful presence",
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

var logoFontNames = [10]string{
	"Heavy Blocky", "Thin Futuristic", "Script Swash", "Wide Geometric",
	"Sharp Angular", "Condensed Serif", "Bubbly Cartoon", "Engraved Roman",
	"Pixel Blocky", "Chiseled Ancient",
}

var logoSizeNames = [6]string{
	"Small", "Medium", "Large", "Extra-Large", "Giant", "Bold Big",
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
	f := (b + 3*t) % 10 // font style — har design ka letterform alag
	l := (b + t) % 10   // lighting mood
	sz := (b + 5*t) % 6 // font size — chota se bara tak variety
	return fmt.Sprintf(
		"Epic premium logo design. Main title typography: the name %s spelled exactly \"%s\", "+
			"written in %s, %s, %s, perfectly readable and clearly legible, the name is the "+
			"main focal point of the design. Background scene: %s. Lighting mood: %s. "+
			"Hyper-detailed professional 8K logo art, strong depth, sharp focus on the name, "+
			"no other text or watermark anywhere",
		name, strings.ToUpper(name), logoFonts[f], logoTexts[t], logoSizes[sz], logoBases[b], logoLights[l])
}

// LogoDesignName returns the human-readable design name for caption.
func LogoDesignName(n int) string {
	if n < 1 || n > LogoCount {
		n = 1
	}
	b := (n - 1) % 100
	t := (n - 1) / 100
	f := (b + 3*t) % 10
	l := (b + t) % 10
	sz := (b + 5*t) % 6
	return fmt.Sprintf("%s + %s Font %s + %s + %s", logoBaseNames[b], logoFontNames[f], logoSizeNames[sz], logoTextNames[t], logoLightNames[l])
}

// ── .logo — menu command (OWNER ORDER: fancy boxed menu, same as other
// category menus — NOT plain text). The main package renders the boxed
// .LOGO1..1000 list via ShowLogoMenu (manager.go CmdLogoMenu).
func handleLogoList(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	s.ShowLogoMenu(info, args, prefix)
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

	// OWNER ORDER: waiting message EXACT ye text ho
	waitID := s.ReplyWithID(info, "*CREATING LOGO PLEASE WAIT.....*")

	deadline := time.Now().Add(logoTimeout)
	for time.Now().Before(deadline) {
		imgData, waitMs, totalWaitMs, busyOnly, err := LogoGenerateImage(LogoPromptForN(n, nameUp))
		if err != nil {
			s.DeleteMessage(info, waitID)
			s.Reply(info, "⬛ *LOGO COMMAND ERROR* ⬛\n*"+strings.ToUpper(err.Error())+"*")
			return
		}
		if imgData != nil {
			// OWNER ORDER: image par KUCH caption nahi — SIRF image
			s.DeleteMessage(info, waitID)
			if sendErr := s.SendImage(info, imgData, ""); sendErr != nil {
				s.Reply(info, "⬛ *LOGO SEND ERROR* ⬛\n"+strings.ToUpper(sendErr.Error()))
			}
			return
		}
		// All keys busy/resting — edit wait message, sleep, retry
		_ = totalWaitMs
		if busyOnly || totalWaitMs > 0 {
			// OWNER ORDER: waiting text hi rehta hai — bas aur thoda rukna hai
			s.EditMessage(info, waitID, "*CREATING LOGO PLEASE WAIT.....*")
		}
		waitDuration := time.Duration(waitMs+500) * time.Millisecond
		if waitDuration < 1*time.Second {
			waitDuration = 1 * time.Second
		}
		time.Sleep(waitDuration)
		s.EditMessage(info, waitID, "*CREATING LOGO PLEASE WAIT.....*")
	}
	s.DeleteMessage(info, waitID)
	s.Reply(info, "*TRY AGAIN LATER*")
}

func init() {
	// OWNER ORDER: .menu me SIRF .logo dikhta hai (display name .LOGO).
	// logo1..logo1000 main-package Commands map me hidden hote hain
	// (logo1000_main.go) — menu me kabhi nahi dikhte. .logo likhne par
	// fancy boxed menu banta hai (ShowLogoMenu → manager.go CmdLogoMenu).
	Register(Command{
		Name:     "logo",
		Category: "AI & MEDIA",
		Desc:     "THIS COMMAND IS USED TO SHOW THE LIST OF 1000 DHAMAKEDAR NAME LOGO DESIGNS. IT SHOWS .LOGO1 TO .LOGO1000 — TYPE ANY LOGO NUMBER WITH YOUR NAME TO MAKE A UNIQUE DHAMAKEDAR TEXT LOGO.",
		Run:      handleLogoList,
	})
}
