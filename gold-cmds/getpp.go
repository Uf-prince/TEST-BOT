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
// KYA BHEJTA HAI (naya format — owner request):
//   HEADER  : *🔰 PROFILE PIC FETCHED 🔰*
//   1. USER NAME   — pushName (contact cache / quoted msg / SenderAlt)
//   2. USER NUMBER — REAL phone number (LID -> ResolveLIDToPN chain,
//                    same 3-layer logic as .block cmd lidresolve.go)
//   3. USER ABOUT  — GetUserInfo -> Status (khali -> NULL)
//   PP image ke sath card jata hai. STORY LINE REMOVED (owner: nahi chahiye).
//
// TARGET MODES:
//   - INBOX (DM): .getpp likhte hi SENDER (to banda) ka profile foran
//   - GROUP: reply/mention/@mention/number — jo bhi target ho
//   - fallback: sender khud
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

const getppHelpText = "🔰 *GETPP — PROFILE LOOKUP* 🔰\n\n" +
	"*INBOX:* just type *GETPP* — the other person's profile comes instantly\n\n" +
	"*GROUP:* reply to a message / @mention / number\n*GETPP 923001234567*\n\n" +
	"*YOU WILL GET:* PIC + NAME + NUMBER + ABOUT\n*(whatever is found is sent, NULL if not)*"

// ----------------------------------------------------------------------------
// TARGET RESOLUTION — reply/quoted > mention > arg number > sender khud
// ----------------------------------------------------------------------------

// getppResolveTarget returns the JID to look up. Priority:
//  1. quoted (replied-to) message ka sender
//  2. @mentioned JID in the command message
//  3. args me number (IsOnWhatsApp se real JID)
//  4. fallback: sender khud (group me / DM me)
func getppResolveTarget(s SessionBridge, info types.MessageInfo, args []string) (types.JID, string, bool) {
	// 0) INBOX (1:1 DM) — ek hi banda hota hai: doosra waqt Chat JID
	if !info.Chat.IsEmpty() && info.Chat.Server == types.DefaultUserServer && info.Chat != info.Sender.ToNonAD() {
		return info.Chat, "inbox", true
	}

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

// getppPushName returns the best-known WhatsApp display name for jid:
//   1. contact store cache (Store.Contacts.GetContact — sqlite, menu cmd pattern:
//      PushName -> FullName -> FirstName)
//   2. "" — caller shows NULL
func getppPushName(s SessionBridge, info types.MessageInfo, jid types.JID) string {
	// contact store (sqlite cache — jis ne kabhi msg kiya hoga wo cached hai)
	if client := s.GetClient(); client != nil && client.Store != nil && client.Store.Contacts != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ci, err := client.Store.Contacts.GetContact(ctx, jid)
		cancel()
		if err == nil && ci.Found {
			if pn := strings.TrimSpace(ci.PushName); pn != "" {
				return pn
			}
			if fn := strings.TrimSpace(ci.FullName); fn != "" {
				return fn
			}
			if fn := strings.TrimSpace(ci.FirstName); fn != "" {
				return fn
			}
		}
	}
	return ""
}

// getppRealNumber converts a @lid target to the real phone-number JID via
// the SAME 3-layer chain the .block command uses (lidresolve.go):
// SenderAlt attr — sqlite LID cache — live usync resolve. Non-LID
// input is returned unchanged.
func getppRealNumber(s SessionBridge, info types.MessageInfo, jid types.JID) types.JID {
	resolved := ResolveLIDToPN(s, info, jid.String())
	if pn, err := types.ParseJID(resolved); err == nil && !pn.IsEmpty() {
		return pn
	}
	return jid
}

func handleGetppAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "⚡ *GETPP ERROR ⚡*\n*BOT CLIENT NOT CONNECTED*")
		return
	}

	// INBOX me .getpp bina kisi arg ke bhi FORAN chalta hai (Chat = target).
	// GROUP me no-arg: reply/mention na ho to help dikha do (sender fallback
	// group me galat banda dikha deta — isliye inbox exception ke sath
	// no-arg group pe help hi behtar).
	if info.Chat.Server != types.DefaultUserServer {
		if strings.TrimSpace(strings.Join(args, " ")) == "" {
			if _, _, ok := s.GetQuotedMessageID(info); !ok {
				if _, ok2 := s.GetMentionedJIDs(info); !ok2 {
					s.Reply(info, getppHelpText)
					return
				}
			}
		}
	}

	target, _, ok := getppResolveTarget(s, info, args)
	if !ok {
		s.Reply(info, getppHelpText)
		return
	}

	waitID := s.ReplyWithID(info, "⚡ *GETPP ⚡ FETCHING PROFILE... ⚡*")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// ─── 1) PROFILE PIC ────────────────────────────────────────────────────
	ppData, ppID, hasPP := getppFetchPP(client, ctx, target)

	// ─── 2) USER INFO (About + LID) ─────────────────────────────
	var about string
	var lid types.JID
	userInfo, uiErr := client.GetUserInfo(ctx, []types.JID{target})
	if uiErr == nil {
		if ui, found := userInfo[target]; found {
			about = strings.TrimSpace(ui.Status)
			lid = ui.LID
		}
	}

	// ---------- 3) NAME + REAL NUMBER (LID -> PN, blocklist cmd jaisa setup) ----------
	nameStr := getppPushName(s, info, target)
	if nameStr == "" && !info.Sender.IsEmpty() && target.User == info.Sender.User {
		nameStr = strings.TrimSpace(info.PushName)
	}
	nameLine := "NULL"
	if nameStr != "" {
		nameLine = nameStr
	}

	// real number: LID -> PN convert (fail pe target jaisa hai waisa hi)
	realJID := getppRealNumber(s, info, target)
	numStr := realJID.User
	if numStr == "" {
		numStr = target.User
	}
	if numStr == "" {
		numStr = "NULL"
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

	// ---------- 5) BUILD CARD (naya format: PIC image ke sath jata hai) ----------
	aboutLine := "NULL"
	if about != "" {
		aboutLine = about
	}

	card := "*\U0001F530 PROFILE PIC FETCHED \U0001F530*\n\n" +
		"*\U0001F530 USER NAME \U0001F530*\n" + nameLine + "\n\n" +
		"*\U0001F530 USER NUMBER \U0001F530*\n\u276E " + numStr + " \u276F\n\n" +
		"*\U0001F530 USER ABOUT \U0001F530*\n" + aboutLine

	// USING WHATSAPP — business profile mila to BUSINESS, warna MESSENGER
	usingLine := "WHATSAPP MESSENGER"
	if bizLine != "NULL" {
		usingLine = "WHATSAPP BUSINESS"
	}
	card += "\n\n*\U0001F530 USING WHATSAPP \U0001F530*\n" + usingLine

	_ = ppID // ppID fetched, abhi card me nahi jata
	_ = lid  // LID raw rakha; card me REAL NUMBER jata hai (LID->PN)

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