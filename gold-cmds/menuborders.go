package goldcmds

import "strings"

// ============================================================================
// GOLD-MD — MENU BORDERS + IMPORTANT CMNDS block
// File: menuborders.go
// ----------------------------------------------------------------------------
// Shared fancy borders for every menu. The category-menu border previously used
// the ❈ glyph un-bolded; it is now bold (a pair of stars around the whole line)
// with the 🔰 emoji in place of ❈. The IMPORTANT CMNDS block keeps the ❈ mark
// but is bold so WhatsApp renders it as a box.
// ============================================================================

const (
	// MenuBorderTop / MenuBorderBottom frame every category list (.menu,
	// .group, .ai, ...) and the .logo / .font / .game / .equalizer menus.
	MenuBorderTop    = "*╔════ ≪ • 🔰 • ≫ ════╗*"
	MenuBorderBottom = "*╚════ ≪ • 🔰 • ≫ ════╝*"

	// ImportantBorderTop / ImportantBorderBottom frame the IMPORTANT CMNDS
	// block carried by every menu right after its header.
	ImportantBorderTop    = "*╔════ ≪ •❈• ≫ ════╗*"
	ImportantBorderBottom = "*╚════ ≪ •❈• ≫ ════╝*"
)

// MenuImportantCommands maps a menu key to the three per-menu media setter
// commands shown in that menu's IMPORTANT CMNDS block, uppercased (e.g. the AI
// menu → AIMENUPIC / AIMENUVIDEO / AIMENUVOICE). ok is false for an unknown
// key, so the caller can skip the block rather than show wrong commands.
func MenuImportantCommands(key string) (pic, video, voice string, ok bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", "", "", false
	}
	for _, mc := range menuMediaCommands {
		if mc.Key != key {
			continue
		}
		switch mc.Kind {
		case "pic":
			pic = strings.ToUpper(mc.Name)
		case "video":
			video = strings.ToUpper(mc.Name)
		}
		if voice == "" {
			voice = strings.ToUpper(mc.VoiceCmd)
		}
	}
	if pic == "" {
		return "", "", "", false
	}
	if voice == "" {
		voice = strings.ToUpper(key) + "VOICE"
	}
	return pic, video, voice, true
}
