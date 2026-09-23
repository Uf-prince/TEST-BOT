package goldcmds

// ============================================================================
// GOLD-MD — Universal Downloader (.dl)
// File: dl.go
// ============================================================================
// COMMAND: .dl   (Category: DOWNLOADER — SHOWN in the menu)
//
// OWNER ORDER (2026):
//   - ".dl" alias was REMOVED from the .del command (delete.go).
//   - ".dl" is now a UNIVERSAL DOWNLOADER: paste ANY supported link and the
//     bot auto-detects the site, then runs the matching download engine.
//
//   .dl <tiktok link>     -> TikTok engine
//   .dl <facebook link>   -> Facebook engine
//   .dl <instagram link>  -> Instagram engine
//   .dl <telegram link>   -> Telegram engine
//   .dl <twitter/x link>  -> Twitter engine
//   .dl <youtube link>    -> YouTube video engine
//   .dl <mediafire link>  -> MediaFire engine
//   .dl <gdrive link>     -> Google Drive engine
//   .dl <apk link>        -> APK engine
//
//   A bare Google Drive FILE_ID is also accepted (routed to the Drive engine).
// ============================================================================

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const dlHelpText = "*\U0001f530 UNIVERSAL DOWNLOADER \U0001f530*\n\n" +
	"*PASTE ANY SUPPORTED LINK WITH .DL*\n" +
	"*THE BOT AUTO-DETECTS THE SITE AND DOWNLOADS IT*\n\n" +
	"*SUPPORTED SITES.....*\n" +
	"*\U0001f530 TIKTOK*\n" +
	"*\U0001f530 FACEBOOK*\n" +
	"*\U0001f530 INSTAGRAM*\n" +
	"*\U0001f530 TELEGRAM*\n" +
	"*\U0001f530 TWITTER / X*\n" +
	"*\U0001f530 YOUTUBE*\n" +
	"*\U0001f530 MEDIAFIRE*\n" +
	"*\U0001f530 GOOGLE DRIVE*\n" +
	"*\U0001f530 APK*\n\n" +
	"*EXAMPLE.....*\n" +
	"*.DL https://www.tiktok.com/@user/video/xxxxx*\n" +
	"*.DL https://youtu.be/xxxxx*\n\n" +
	"*JUST PASTE THE LINK AND THE BOT WILL DO THE REST \U0001f530*"

const dlUnsupportedText = "\u274c *UNSUPPORTED LINK*\n" +
	"*PASTE A TIKTOK / FACEBOOK / INSTAGRAM / TELEGRAM / TWITTER / YOUTUBE / MEDIAFIRE / GDRIVE / APK LINK*"

func init() {
	Register(Command{Name: "dl", Category: "DOWNLOADER", Desc: "THIS COMMAND IS A UNIVERSAL DOWNLOADER. PASTE ANY SUPPORTED LINK (TIKTOK, FACEBOOK, INSTAGRAM, TELEGRAM, TWITTER, YOUTUBE, MEDIAFIRE, GDRIVE, APK) AND THE BOT AUTO-DETECTS THE SITE AND DOWNLOADS IT.", Run: handleDL})
	// Hidden aliases — same engine, NOT shown in the menu (owner order).
	Register(Command{Name: "download", Hidden: true, Run: handleDL})
	Register(Command{Name: "dlx", Hidden: true, Run: handleDL})
}

// handleDL — universal downloader entry point. Detects the pasted link's
// platform and dispatches to the matching download engine.
func handleDL(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	input := strings.TrimSpace(strings.Join(args, " "))
	if input == "" {
		s.Reply(info, dlHelpText)
		return
	}

	switch dlDetectEngine(input) {
	case "tt":
		handleTikTok(s, info, args, prefix)
	case "fb":
		handleFB(s, info, args, prefix)
	case "ig":
		handleInsta(s, info, args, prefix)
	case "tg":
		handleTG(s, info, args, prefix)
	case "twt":
		handleTwitter(s, info, args, prefix)
	case "yt":
		handleVideo2(s, info, args, prefix)
	case "mf":
		handleMediafire(s, info, args, prefix)
	case "gdrive":
		handleGdrive(s, info, args, prefix)
	case "apk":
		handleAPKSearch(s, info, args, prefix)
	default:
		s.Reply(info, dlUnsupportedText)
	}
}

// dlDetectEngine inspects the input and returns the engine key for the
// detected platform ("" when nothing matches).
func dlDetectEngine(input string) string {
	link := searchLinkRe.FindString(input)
	if link == "" {
		// No URL — accept a bare Google Drive FILE_ID as a fallback.
		if gdriveExtractFileID(input) != "" {
			return "gdrive"
		}
		return ""
	}

	host := searchLinkHost(link)
	switch {
	case searchLinkDomainMatch(host, "tiktok.com") ||
		searchLinkDomainMatch(host, "vm.tiktok.com") ||
		searchLinkDomainMatch(host, "vt.tiktok.com"):
		return "tt"
	case searchLinkDomainMatch(host, "facebook.com") ||
		searchLinkDomainMatch(host, "fb.watch") ||
		searchLinkDomainMatch(host, "fb.com"):
		return "fb"
	case searchLinkDomainMatch(host, "instagram.com") ||
		searchLinkDomainMatch(host, "instagr.am") ||
		searchLinkDomainMatch(host, "ddinstagram.com"):
		return "ig"
	case searchLinkDomainMatch(host, "t.me") ||
		searchLinkDomainMatch(host, "telegram.me") ||
		searchLinkDomainMatch(host, "telegram.dog"):
		return "tg"
	case searchLinkDomainMatch(host, "twitter.com") ||
		searchLinkDomainMatch(host, "x.com"):
		return "twt"
	case searchLinkDomainMatch(host, "youtube.com") ||
		searchLinkDomainMatch(host, "youtu.be") ||
		searchLinkDomainMatch(host, "music.youtube.com"):
		return "yt"
	case searchLinkDomainMatch(host, "mediafire.com"):
		return "mf"
	case searchLinkDomainMatch(host, "drive.google.com") ||
		searchLinkDomainMatch(host, "docs.google.com"):
		return "gdrive"
	case searchLinkDomainMatch(host, "apkcombo.com") ||
		searchLinkDomainMatch(host, "apkpure.com") ||
		searchLinkDomainMatch(host, "apk.support") ||
		searchLinkDomainMatch(host, "apkmirror.com"):
		return "apk"
	}
	return ""
}
