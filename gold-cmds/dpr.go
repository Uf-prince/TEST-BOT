package goldcmds

// ============================================================================
// GOLD-MD — Disappearing Messages (DPR) Command
// File: dpr.go
// ============================================================================
// COMMAND: .dpr <24h|7d|90d|off>    (owner-only)
//   Sets the disappearing-messages timer for the current chat (group or DM).
//
// COMMAND: .dproff                   (owner-only)
//   Turns off disappearing messages in the current chat.
//
// Source: UMAR-MD dpr.js  (Node.js / Baileys)
// Converted to Go / whatsmeow for GOLD-MD.
//
// whatsmeow: Client.SetDisappearingTimer(ctx, chat, timer, ts) with
//   whatsmeow.DisappearingTimerOff / 24Hours / 7Days / 90Days.
// ============================================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const dprHelpText = "*🔰 DISAPPEARING MSGS GUIDE 🔰*\n\n" +
	"*TYPE :❰ ❱ DPR 24 ❰*\n*FOR SET DISAPPEARING MSGS FOR 24 HOURS*\n" +
	"*TYPE :❰ ❱ DPR 7 ❰*\n*FOR SET DISAPPEARING MSGS FOR 7 DAYS*\n" +
	"*TYPE :❰ ❱ DPR 90 ❰*\n*FOR SET DISAPPEARING MSGS FOR 90 DAYS*\n" +
	"*TYPE :❰ ❱ DPR OFF ❰*\n*TO TURN OFF DISAPPEARING MSGS*\n\n" +
	"*SET DISAPPEARING MSGS AS YOUR WISH*"

func handleDPR(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleDPRAsync(s, info, args, prefix)
}

func handleDPRAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	arg := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if arg == "" {
		s.Reply(info, dprHelpText)
		return
	}

	timer, label, ok := parseDPRTimer(arg)
	if !ok {
		s.Reply(info, dprHelpText)
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "❌ *Client not connected.*")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.SetDisappearingTimer(ctx, info.Chat, timer, time.Now()); err != nil {
		s.Reply(info, fmt.Sprintf("*🔰 DPR ERROR* 🔰\n*%s*", strings.ToUpper(err.Error())))
		return
	}

	if timer == whatsmeow.DisappearingTimerOff {
		s.Reply(info, "*DISAPPEARING MESSAGES TURNED OFF*")
	} else {
		s.Reply(info, fmt.Sprintf("*DISAPPEARING MESSAGES SET TO %s*", strings.ToUpper(label)))
	}
}

// handleDPROff is a literal alias that always turns disappearing messages off.
func handleDPROff(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// Reuse the main handler with "off".
	handleDPR(s, info, []string{"off"}, prefix)
}

// parseDPRTimer maps a user string to a whatsmeow disappearing timer duration.
func parseDPRTimer(arg string) (timer time.Duration, label string, ok bool) {
	switch arg {
	case "24h", "24", "1d", "day", "1day":
		return whatsmeow.DisappearingTimer24Hours, "24 HOURS", true
	case "7d", "7", "1w", "week", "1week":
		return whatsmeow.DisappearingTimer7Days, "7 DAYS", true
	case "90d", "90", "3m", "3mo", "3months":
		return whatsmeow.DisappearingTimer90Days, "90 DAYS", true
	case "off", "0", "0d", "0h":
		return whatsmeow.DisappearingTimerOff, "OFF", true
	}
	return 0, "", false
}

func init() {
	Register(Command{Name: "dpr", Category: "OWNER & SYSTEM", Desc: "Set disappearing-message timer (24h/7d/90d/off)", OwnerOnly: true, Run: handleDPR})
	Register(Command{Name: "dproff", Category: "OWNER & SYSTEM", Desc: "Turn off disappearing messages in chat", OwnerOnly: true, Run: handleDPROff})
}
