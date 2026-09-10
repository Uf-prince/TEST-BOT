package goldcmds

// ============================================================================
// GOLD-MD — .GETPP COMMAND (FULL PROFILE LOOKUP)
// File: getpp.go
// ============================================================================
// COMMAND:
//   .getpp                                -> reply/quoted target ka profile
//   .getpp 923xxxxxxxxx                   -> number ka profile
//   .getpp (mention @user ke saath)       -> mentioned user ka profile
//
// KYA BHEJTA HAI (jo mile bhejo, jo na mile NULL):
//   1. PROFILE PIC  — full-res image (GetProfilePictureInfo -> HTTP GET)
//                     na mile -> "NULL" (preview try, phir bhi na mile NULL)
//   2. NUMBER       — JID (user: xxx@s.whatsapp.net / LID @lid)
//   3. ABOUT        — GetUserInfo -> Status field (About text)
//                     khali/na mile -> NULL
//   4. STORY        — honest NULL: whatsmeow me story GET API nahi hai
//                     (sirf Set/Privacy hai). User ne bola tha "jo na mile
//                     NULL aa jaye" — isliye field hai, value NULL.
//   5. EXTRA        — LID, linked devices count, business profile,
//                     verified name (jo mile)
//
// whatsmeow APIs (vendored latest, 2026 protocol):
//   - cli.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{})
//       -> { URL, ID, Type, DirectPath }  (types/user.go:62)
//   - cli.GetUserInfo(ctx, []types.JID{jid})
//       -> map[JID]UserInfo{ VerifiedName, Status, PictureID, Devices, LID }
//   - cli.IsOnWhatsApp(ctx, []string{number})
//       -> [{ IsIn, JID, VerifiedName }]
//   - cli.GetBusinessProfile(ctx, jid) -> business name/desc (may 404)
//
// NOTE: PP URL plain HTTP GET se download hota hai (types/user.go comment:
//       "can be downloaded with a simple HTTP request") — no media decrypt.
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ----------------------------------------------------------------------------
// HELP
// ----------------------------------------------------------------------------

const getppHelpText = "⚡ *GETPP ⚡ USER PROFILE LOOKUP ⚡*\n\n" +
	"*REPLY TO ANY USER'S MESSAGE AND TYPE*\n*GETPP*\n\n" +
	"*OR SEND A NUMBER WITH THE COMMAND*\n*GETPP 923001234567*\n\n" +
	"*YOU WILL GET:*\n" +
	"*PIC , NUMBER , ABOUT , STORY*\n\n" +
	"*WHATEVER IS FOUND WILL BE SENT , WHAT IS NOT FOUND WILL BE NULL*"

// ----------------------------------------------------------------------------
// TARGET RESOLUTION — reply/quoted > mention > arg number > sender khud
// ----------------------------------------------------------------------------

// getppResolveTarget returns the JID to look up. Priority:
//  1. quoted (replied-to) message ka sender
//  2. @mentioned JID in the command message
//  3. args me number (IsOnWhatsApp se real JID)
//  4. fallback: sender khud (group me / DM me)
func getppResolveTarget(s SessionBridge, info types.MessageInfo, args []string) (types.JID, string, bool) {
	// 1) Quoted/replied-to message
	if quotedID, quotedSender, ok := s.GetQuotedMessageID(info); ok {
		_ = quotedID
		if quotedSender != "" {
			if jid, err := types.ParseJID(quotedSender); err == nil && !jid.IsEmpty() {
				return jid, "reply", true
			}
		}
	}

	// 2) @mention in the command message
	if mentioned, ok := s.GetMentionedJIDs(info); ok && len(mentioned) > 0 {
		if jid, err := types.ParseJID(mentioned[0]); err == nil && !jid.IsEmpty() {
			return jid, "mention", true
		}
	}

	// 3) Number in args
	if len(args) > 0 {
		input := strings.TrimSpace(strings.Join(args, " "))
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, input)
		if len(digits) >= 7 && len(digits) <= 15 {
			client := s.GetClient()
			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				results, err := client.IsOnWhatsApp(ctx, []string{"+" + digits})
				if err == nil && len(results) > 0 && results[0].IsIn {
					return results[0].JID, "number", true
				}
			}
		}
	}

	// 4) Fallback: sender khud
	if !info.Sender.IsEmpty() {
		return info.Sender, "self", true
	}
	return types.JID{}, "", false
}

// ----------------------------------------------------------------------------
// PP DOWNLOAD — full-res first, preview fallback
// ----------------------------------------------------------------------------

// getppFetchPP downloads the profile picture bytes for jid.
// Tries full-res ("image") first, then preview thumbnail.
// Returns (bytes, ppID, true) or (nil, "", false) when no PP exists.
func getppFetchPP(client *whatsmeow.Client, ctx context.Context, jid types.JID) ([]byte, string, bool) {
	// Full-res attempt
	info, err := client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{})
	if err == nil && info != nil && info.URL != "" {
		if data, ok := getppHTTPGet(ctx, info.URL); ok {
			return data, info.ID, true
		}
	}
	// Preview fallback
	info, err = client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
	if err == nil && info != nil && info.URL != "" {
		if data, ok := getppHTTPGet(ctx, info.URL); ok {
			return data, info.ID, true
		}
	}
	return nil, "", false
}

// getppHTTPGet downloads URL bytes with a sane timeout (10MB cap).
func getppHTTPGet(ctx context.Context, url string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// ----------------------------------------------------------------------------
// MAIN HANDLER
// ----------------------------------------------------------------------------

func handleGetpp(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleGetppAsync(s, info, args, prefix)
}

func handleGetppAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "⚡ *GETPP ERROR ⚡*\n*BOT CLIENT NOT CONNECTED*")
		return
	}

	if strings.TrimSpace(strings.Join(args, " ")) == "" {
		// no arg — sirf reply/mention bhi na mile to help dikha do.
		if _, _, ok := s.GetQuotedMessageID(info); !ok {
			if _, ok2 := s.GetMentionedJIDs(info); !ok2 {
				s.Reply(info, getppHelpText)
				return
			}
		}
	}

	target, how, ok := getppResolveTarget(s, info, args)
	if !ok {
		s.Reply(info, getppHelpText)
		return
	}

	waitID := s.ReplyWithID(info, "⚡ *GETPP ⚡ FETCHING PROFILE... ⚡*")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// ─── 1) PROFILE PIC ────────────────────────────────────────────────────
	ppData, ppID, hasPP := getppFetchPP(client, ctx, target)

	// ─── 2) USER INFO (About + LID + Devices) ─────────────────────────────
	var about string
	var lid types.JID
	var devices int
	userInfo, uiErr := client.GetUserInfo(ctx, []types.JID{target})
	if uiErr == nil {
		if ui, found := userInfo[target]; found {
			about = strings.TrimSpace(ui.Status)
			lid = ui.LID
			devices = len(ui.Devices)
		}
	}

	// ─── 3) NUMBER / JID STRING ───────────────────────────────────────────
	numStr := target.String()
	if target.User == "" && !lid.IsEmpty() {
		numStr = lid.String() + " (LID)"
	}

	// ─── 4) BUSINESS / VERIFIED NAME (jo mile) ────────────────────────────
	bizLine := "NULL"
	if biz, err := client.GetBusinessProfile(ctx, target); err == nil && biz != nil {
		if bn := strings.TrimSpace(biz.ProfileOptions["business_name"]); bn != "" {
			bizLine = bn
		} else if len(biz.Categories) > 0 && strings.TrimSpace(biz.Categories[0].Name) != "" {
			bizLine = strings.TrimSpace(biz.Categories[0].Name)
		} else if biz.Email != "" {
			bizLine = biz.Email
		}
	}

	// ─── 5) BUILD CARD ────────────────────────────────────────────────────
	ppLine := "NULL"
	if hasPP {
		ppLine = "✅ SENT BELOW"
	}
	aboutLine := "NULL"
	if about != "" {
		aboutLine = about
	}
	storyLine := "NULL (NOT AVAILABLE VIA API)"

	card := "⚡ *GETPP ⚡ PROFILE LOOKUP ⚡*\n\n" +
		"*PIC :* " + ppLine + "\n" +
		"*NUMBER :* " + numStr + "\n" +
		"*ABOUT :* " + aboutLine + "\n" +
		"*STORY :* " + storyLine + "\n"

	// extra lines (jo mile)
	if !lid.IsEmpty() {
		card += "*LID :* " + lid.String() + "\n"
	}
	if devices > 0 {
		card += fmt.Sprintf("*LINKED DEVICES :* %d\n", devices)
	}
	if bizLine != "NULL" {
		card += "*BUSINESS :* " + bizLine + "\n"
	}
	card += "\n*SOURCE :* " + strings.ToUpper(how) + " ⚡ *GOLD-MD*"

	_ = ppID

	// ─── 6) SEND — PP first (image), then card text ───────────────────────
	if hasPP && ppData != nil {
		if err := s.SendImage(info, ppData, card); err != nil {
			// image fail -> text-only fallback (card bhi to bhejna hai)
			s.EditMessage(info, waitID, card)
			return
		}
		s.DeleteMessage(info, waitID) // "FETCHING..." hata do
		return
	}
	// No PP — NULL card text me bhejo
	s.EditMessage(info, waitID, card)
}

// ----------------------------------------------------------------------------
// REGISTER
// ----------------------------------------------------------------------------

func init() {
	Register(Command{
		Name:     "getpp",
		Category: "OWNER & SYSTEM",
		Desc:     "Get user full profile: pic, number, about, story (NULL if not found)",
		Run:      handleGetpp,
	})
}