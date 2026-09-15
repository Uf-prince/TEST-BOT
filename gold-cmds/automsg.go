package goldcmds

// ============================================================================
// GOLD-MD — AUTOMSG Command  (Bot-Memory-backed scheduled auto-message)
// File: automsg.go
// ----------------------------------------------------------------------------
// COMMAND: .automsg XXhXXmXXs repeat/once {msg}    (owner-only)
//
//   .automsg 00h30m00s repeat Assalamualaikum everyone!
//        → every 30 minutes, send "Assalamualaikum everyone!" to this chat.
//          The timer COUNTS DOWN from 30m → 0 → sends msg → RESETS to 30m →
//          counts down again → loops forever. The config STAYS in the bot's
//          MEMORY (persists across restarts) because mode = repeat.
//
//   .automsg 02h00m00s once Reminder: meeting at 5pm
//        → after 2 hours, send the message ONCE. The timer counts down 2h → 0
//          → sends msg → then the config is DELETED from the bot's MEMORY
//          (one-shot).
//
//   .automsg stop       → cancel the active timer in this chat + safe-delete
//                         the config from the bot's MEMORY.
//   .automsg status     → show the active schedule + LIVE countdown for this chat.
//   .automsg list       → show ALL saved schedules (every chat) numbered 1,2,3…
//   .automsg delete     → show the numbered list + "MENTION REPLY A NUMBER TO
//                         DELETE" — owner replies a number → that schedule is
//                         deleted from bot memory + timer cancelled.
//   .automsg on         → re-arm (turn ON) the current chat's saved schedule.
//                         If none is saved → "pehle msg set karo" + full guide.
//   .automsg off        → turn OFF the current chat's active timer (pause).
//                         Config STAYS in bot memory so .automsg on can resume.
//   .automsg            (no arg) → show the usage guide.
//
// DESIGN (per owner requirements):
//   • Bot speed = 0% farak. The entire timer runs in a background goroutine via
//     time.AfterFunc — it NEVER blocks the message handler / Reply path. The
//     command itself returns immediately after arming the timer.
//   • Time counts down bar-bar. For repeat mode, after each fire the duration
//     is RESET and the timer re-arms so the countdown restarts from the top
//     every cycle (XXhXXmXXs → 0 → send → XXhXXmXXs → 0 → send → ...).
//   • The bot saves the schedule in its own MEMORY (the same secure place it
//     keeps antidelete / antiedit messages). repeat → config STAYS in MEMORY so
//     it survives a restart; once → config is DELETED from MEMORY after the
//     single send.
//   • Safe delete: the memory delete is idempotent (no error if already gone).
//   • Live countdown: the next-fire timestamp is stored so .automsg status can
//     show "next fire in 00h29m48s".
//
// The active timer for each (botJID, chat) is tracked in a package-level map so
// .automsg stop / off can cancel it. Re-running .automsg in a chat that already
// has a timer first cancels the old one (replace schedule).
// ============================================================================

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── active timers (in-memory, per botJID+chat) ──────────────────────────────
//
// This map ONLY holds live *time.Timer pointers so .automsg stop can cancel a
// running schedule. The durable config lives in the bot's MEMORY. 0% speed
// impact: the map is touched only on .automsg arm/stop/status, never on normal
// messages.

type automsgTimer struct {
	timer *time.Timer
	// snapshot of the schedule, used by .automsg status for the live countdown
	cfg automsgConfig
	// nextFire is when the timer is expected to fire (for countdown display)
	nextFire time.Time
}

var (
	automsgMu     sync.Mutex
	automsgTimers = make(map[string]*automsgTimer) // key = botJID + "|" + chat
)

// ── pending delete list ─────────────────────────────────────────────────────
// When the owner runs .automsg delete, we show a numbered list and remember it
// here keyed by (botJID + "|" + senderJID). If the owner's NEXT message is a
// plain number, we delete that entry. This lets the owner reply "2" to delete
// schedule #2. The map is per-owner so two owners don't clash.
type automsgPendingDelete struct {
	keys []string // the config IDs in display order (index 0 = #1)
	ts   time.Time
}

var (
	automsgDelMu      sync.Mutex
	automsgDelPending = make(map[string]*automsgPendingDelete)
)

// automsgConfig is the JSON the bot keeps in its MEMORY under automsg/<id>.json.
type automsgConfig struct {
	DurationSec int64  `json:"duration_sec"` // total countdown seconds (XXhXXmXXs)
	Mode        string `json:"mode"`         // "repeat" or "once"
	Message     string `json:"message"`      // the message text to send on fire
	Chat        string `json:"chat"`         // chat JID to send to
	BotJID      string `json:"bot_jid"`      // which bot session owns this
	NextFireMs  int64  `json:"next_fire_ms"` // unix-milli of next fire (for countdown)
	CreatedMs   int64  `json:"created_ms"`   // when first armed
}

// automsgKey builds the in-memory timer-map key and the MEMORY object id.
// Both use the same string so a status/stop/list lookup is one call.
func automsgKey(botJID, chat string) string {
	return botJID + "|" + chat
}

// parseDuration parses a "XXhXXmXXs" token into a duration.
// Examples: "02h30m00s" → 2h30m, "00h00m45s" → 45s, "1h0m0s" → 1h.
// Returns ok=false if the format is wrong or the total is <= 0.
// FIX 2026-09-13: Purana parser pehle trailing "s" strip kar deta tha, is
// wajah se seconds ke digits ke aage unit letter ("s") nahi bachta tha aur
// HAR sahi format (06h05m06s, 02h30m00s, ...) reject ho jata tha. Naya
// parser seedha regex se h/m/s teeno segments match karta hai — koi strip
// nahi, koi ambiguity nahi.
//
// Accepted forms (strict, exactly h + m + s, two-digit padding optional):
//
//	06h05m06s   2h30m0s   00h00m45s   1h2m3s
//
// Rejected: 30m (missing h/s), 2h30m (missing s), 45s (missing h/m),
//
//	2h30m00 (missing trailing s), abc, 0h0m0s (total must be > 0).
var automsgDurRe = regexp.MustCompile(`^(\d+)h(\d+)m(\d+)s$`)

func parseAutomsgDuration(token string) (time.Duration, bool) {
	m := automsgDurRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(token)))
	if m == nil {
		return 0, false
	}
	h, err1 := strconv.ParseInt(m[1], 10, 64)
	mn, err2 := strconv.ParseInt(m[2], 10, 64)
	sec, err3 := strconv.ParseInt(m[3], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	// sane bounds: minutes & seconds must be < 60, hours < ~1 year
	if mn < 0 || mn > 59 {
		return 0, false
	}
	if sec < 0 || sec > 59 {
		return 0, false
	}
	if h < 0 || h > 8784 {
		return 0, false
	}
	total := time.Duration(h)*time.Hour + time.Duration(mn)*time.Minute + time.Duration(sec)*time.Second
	if total <= 0 {
		return 0, false
	}
	return total, true
}

// formatHMS turns a duration back into "XXhXXmXXs" for display.
func formatHMS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int64(d.Hours())
	d -= time.Duration(h) * time.Hour
	m := int64(d.Minutes())
	d -= time.Duration(m) * time.Minute
	s := int64(d.Seconds())
	return fmt.Sprintf("%02dh%02dm%02ds", h, m, s)
}

// ── owner-reply number detection (for .automsg delete) ─────────────────────
// isJustNumber returns true if s is a plain integer like "1", "2", "03".
func isJustNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// delPendingKey is the map key for a owner's pending-delete list.
func delPendingKey(botJID, senderJID string) string {
	return botJID + "|" + senderJID
}

// ── AutomsgTryDeleteReply ──────────────────────────────────────────────────
// Prefix-less number-reply hook for .automsg delete. handler.go isko apne
// prefix-check se PEHLE call karta hai: agar owner ne abhi .automsg delete
// ka numbered list dekha hai aur uska next message sirf ek number hai (jaise
// "2"), to wahi schedule delete ho jata hai — bina prefix type kiye. Baaki
// sab cases me false return hota hai (koi impact nahi).
func AutomsgTryDeleteReply(s SessionBridge, info types.MessageInfo, body string) bool {
	t := strings.TrimSpace(body)
	if !isJustNumber(t) || t == "" {
		return false
	}
	// owner-only feature
	if !s.IsOwner(info) {
		return false
	}
	dpk := delPendingKey(s.GetJID(), info.Sender.String())
	automsgDelMu.Lock()
	pending := automsgDelPending[dpk]
	automsgDelMu.Unlock()
	if pending == nil || len(pending.keys) == 0 {
		return false
	}
	if time.Since(pending.ts) > 2*time.Minute {
		automsgDelMu.Lock()
		delete(automsgDelPending, dpk)
		automsgDelMu.Unlock()
		return false
	}
	go handleAutomsgDeleteByNumber(s, info, ".", pending, t)
	return true
}

func handleAutomsg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAutomsgAsync(s, info, args, prefix)
}

func handleAutomsgAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	// owner-only
	if !s.IsOwner(info) {
		s.Reply(info, "🔰 *THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	botJID := s.GetJID()
	chatStr := info.Chat.String()
	senderStr := info.Sender.String()
	key := automsgKey(botJID, chatStr)
	argStr := strings.TrimSpace(strings.Join(args, " "))

	// ── Check for pending delete reply FIRST ──
	// If the owner was shown a .automsg delete list and now sends a plain number,
	// treat it as "delete schedule #N".
	if isJustNumber(argStr) {
		dpk := delPendingKey(botJID, senderStr)
		automsgDelMu.Lock()
		pending := automsgDelPending[dpk]
		automsgDelMu.Unlock()
		if pending != nil && len(pending.keys) > 0 {
			// expire after 2 minutes
			if time.Since(pending.ts) > 2*time.Minute {
				automsgDelMu.Lock()
				delete(automsgDelPending, dpk)
				automsgDelMu.Unlock()
			} else {
				handleAutomsgDeleteByNumber(s, info, prefix, pending, argStr)
				return
			}
		}
		// if no pending list, fall through (a bare number with no context → usage)
	}

	// ── no arg → usage guide (GOLD-MD styled) ──
	if argStr == "" {
		s.Reply(info, automsgUsage(prefix))
		return
	}

	// ── .automsg list → show ALL saved schedules numbered ──
	if strings.EqualFold(argStr, "list") {
		handleAutomsgList(s, info, prefix)
		return
	}

	// ── .automsg delete → show numbered list + ask for a number ──
	if strings.EqualFold(argStr, "delete") || strings.EqualFold(argStr, "del") {
		handleAutomsgDeletePrompt(s, info, prefix)
		return
	}

	// ── .automsg on → re-arm current chat's saved schedule ──
	if strings.EqualFold(argStr, "on") || strings.EqualFold(argStr, "start") || strings.EqualFold(argStr, "resume") {
		handleAutomsgOn(s, info, prefix, key)
		return
	}

	// ── .automsg off → turn off (pause) current chat ──
	if strings.EqualFold(argStr, "off") || strings.EqualFold(argStr, "pause") {
		handleAutomsgOff(s, info, prefix, key)
		return
	}

	// ── .automsg status → live countdown for this chat ──
	if strings.EqualFold(argStr, "status") || strings.EqualFold(argStr, "st") {
		handleAutomsgStatus(s, info, prefix, key)
		return
	}

	// ── .automsg stop → cancel timer + SAFE-DELETE config (docs me tha, code me missing tha) ──
	if strings.EqualFold(argStr, "stop") || strings.EqualFold(argStr, "cancel") {
		handleAutomsgStop(s, info, prefix, key)
		return
	}

	// ── .automsg XXhXXmXXs repeat/once {msg} → arm schedule ──
	// Token 1 = duration (XXhXXmXXs), token 2 = mode (repeat/once), rest = msg
	parts := strings.Fields(argStr)
	if len(parts) < 3 {
		s.Reply(info, "🔰 *Invalid format.*\n\n*Usage:* ```"+prefix+"automsg XXhXXmXXs repeat/once {your message}```\n\n*Example:* ```"+prefix+"automsg 00h30m00s repeat Assalamualaikum!```")
		return
	}

	durToken := parts[0]
	modeToken := strings.ToLower(parts[1])
	msgText := strings.TrimSpace(strings.Join(parts[2:], " "))

	dur, ok := parseAutomsgDuration(durToken)
	if !ok {
		s.Reply(info, "🔰 *Invalid time format.*\n\n*Time must be like:* ```XXhXXmXXs```\n*Example:* ```02h30m00s``` (= 2 hours 30 min)\n*Example:* ```00h00m45s``` (= 45 seconds)")
		return
	}
	if modeToken != "repeat" && modeToken != "once" {
		s.Reply(info, "🔰 *Invalid mode.*\n\n*Mode must be:* ```repeat``` *or* ```once```\n• *repeat* = sends again & again, timer resets every cycle, schedule STAYS in bot memory\n• *once* = sends one time only, schedule DELETED from bot memory after send")
		return
	}
	if msgText == "" {
		s.Reply(info, "🔰 *No message text provided.*\n\n*Usage:* ```"+prefix+"automsg XXhXXmXXs repeat/once {your message}```")
		return
	}

	// The bot must have its memory ready (it keeps the schedule in the same
	// secure place it keeps antidelete / antiedit messages).
	if !s.MemoryReady() {
		s.Reply(info, "🔰 *Bot memory is not ready yet.*\n\n*The bot saves your schedule in its own memory (the same secure place it keeps antidelete & antiedit messages).*\n*Please make sure the bot is fully connected and try again.*")
		return
	}

	// cancel any existing timer for this chat first (replace schedule)
	automsgCancel(key)

	// build + save config to the bot's MEMORY
	cfg := automsgConfig{
		DurationSec: int64(dur.Seconds()),
		Mode:        modeToken,
		Message:     msgText,
		Chat:        chatStr,
		BotJID:      botJID,
		CreatedMs:   time.Now().UnixMilli(),
	}
	nextFire := time.Now().Add(dur)
	cfg.NextFireMs = nextFire.UnixMilli()

	data, _ := json.Marshal(cfg)
	if err := s.MemorySave(key, data); err != nil {
		s.Reply(info, "🔰 *AUTOMSG Error*\n\n*Bot could not save the schedule to its memory:*\n"+err.Error())
		return
	}

	// arm the timer (0% speed impact — runs fully in background via AfterFunc)
	armAutomsgTimer(s, info, key, cfg, dur)

	// styled confirmation
	modeEmoji := "🔰"
	modeLabel := "REPEAT"
	modeNote := "*TIMER WILL RESET & COUNT AGAIN AFTER EVERY SEND*\n*SCHEDULE STAYS SAFE IN BOT MEMORY (PERSISTS ACROSS RESTART)*"
	if modeToken == "once" {
		modeEmoji = "1️⃣"
		modeLabel = "ONCE"
		modeNote = "*TIMER COUNTS DOWN ONCE → SENDS → SCHEDULE AUTO-REMOVED FROM BOT MEMORY*"
	}

	s.Reply(info, fmt.Sprintf(
		"🔰 *AUTOMSG SCHEDULED* 🔰\n\n"+
			"🔰 *TIME     :❰ %s ❱*\n"+
			"%s *MODE     :❰ %s ❱*\n"+
			"🔰 *MESSAGE  :❰ %s ❱*\n"+
			"🔰 *CHAT     :❰ %s ❱*\n"+
			"🔰 *FIRST FIRE IN :❰ %s ❱*\n"+
			"🔰 *SAVED IN :❰ %s ❱*\n\n"+
			"%s\n\n"+
			"*TO TURN OFF TYPE:* ```%sautomsg off```\n"+
			"*TO SEE ALL TYPE:* ```%sautomsg list```",
		formatHMS(dur),
		modeEmoji, modeLabel,
		msgText,
		chatStr,
		formatHMS(dur),
		"BOT MEMORY 🔰",
		modeNote,
		prefix, prefix,
	))
}

// ── .automsg list ───────────────────────────────────────────────────────────
// Shows every saved schedule across all chats, numbered 1, 2, 3 …
func handleAutomsgList(s SessionBridge, info types.MessageInfo, prefix string) {
	if !s.MemoryReady() {
		s.Reply(info, "🔰 *Bot memory is not ready yet.*\n\n*The bot saves schedules in its own memory.*\n*Please make sure the bot is fully connected and try again.*")
		return
	}
	entries, err := s.MemoryList()
	if err != nil {
		s.Reply(info, "🔰 *AUTOMSG LIST ERROR*\n\n*Bot could not read its memory:*\n"+err.Error())
		return
	}
	if len(entries) == 0 {
		s.Reply(info, "🔰 *AUTOMSG LIST* 🔰\n\n🔰 *No saved auto-message schedules found in bot memory.*\n\n*To create one type:* ```"+prefix+"automsg 00h30m00s repeat <msg>```")
		return
	}
	var sb strings.Builder
	sb.WriteString("🔰 *AUTOMSG LIST* 🔰\n")
	sb.WriteString("🔰 *ALL SAVED SCHEDULES IN BOT MEMORY*\n\n")
	for i, e := range entries {
		var cfg automsgConfig
		if err := json.Unmarshal(e.Data, &cfg); err != nil {
			continue
		}
		modeEmoji := "🔰"
		if cfg.Mode == "once" {
			modeEmoji = "1️⃣"
		}
		// check if timer is currently active
		automsgMu.Lock()
		_, active := automsgTimers[e.ID]
		automsgMu.Unlock()
		statusIcon := "🔴 OFF"
		if active {
			statusIcon = "🟢 ON"
		}
		// truncate long messages for the list
		msgPreview := cfg.Message
		if len(msgPreview) > 40 {
			msgPreview = msgPreview[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("*%d.* %s *%s* %s\n", i+1, modeEmoji, strings.ToUpper(cfg.Mode), statusIcon))
		sb.WriteString(fmt.Sprintf("   🔰 *TIME :* `%s`\n", formatHMS(time.Duration(cfg.DurationSec)*time.Second)))
		sb.WriteString(fmt.Sprintf("   🔰 *MSG  :* %s\n", msgPreview))
		sb.WriteString(fmt.Sprintf("   🔰 *CHAT :* `%s`\n", cfg.Chat))
		if i < len(entries)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n───────────────────\n")
	sb.WriteString("🔰 *TOTAL :* " + strconv.Itoa(len(entries)) + " schedule(s)\n")
	sb.WriteString("🔰 *SAVED IN :* BOT MEMORY 🔰\n")
	sb.WriteString("*TO DELETE TYPE:* ```" + prefix + "automsg delete```\n")
	sb.WriteString("*TO TURN ON/OFF TYPE:* ```" + prefix + "automsg on``` / ```" + prefix + "automsg off```")
	s.Reply(info, sb.String())
}

// ── .automsg delete (prompt) ───────────────────────────────────────────────
// Shows the numbered list and asks the owner to reply a number to delete.
func handleAutomsgDeletePrompt(s SessionBridge, info types.MessageInfo, prefix string) {
	if !s.MemoryReady() {
		s.Reply(info, "🔰 *Bot memory is not ready yet.*\n\n*The bot saves schedules in its own memory.*\n*Please make sure the bot is fully connected and try again.*")
		return
	}
	entries, err := s.MemoryList()
	if err != nil {
		s.Reply(info, "🔰 *AUTOMSG DELETE ERROR*\n\n*Bot could not read its memory:*\n"+err.Error())
		return
	}
	if len(entries) == 0 {
		s.Reply(info, "🔰 *AUTOMSG DELETE* 🔰\n\n🔰 *No saved auto-message schedules found in bot memory.*\n\n*To create one type:* ```"+prefix+"automsg 00h30m00s repeat <msg>```")
		return
	}
	// store the pending list so a number reply can delete
	dpk := delPendingKey(s.GetJID(), info.Sender.String())
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	automsgDelMu.Lock()
	automsgDelPending[dpk] = &automsgPendingDelete{keys: ids, ts: time.Now()}
	automsgDelMu.Unlock()

	var sb strings.Builder
	sb.WriteString("🔰 *AUTOMSG DELETE* 🔰\n")
	sb.WriteString("🔰 *ALL SAVED SCHEDULES IN BOT MEMORY*\n\n")
	for i, e := range entries {
		var cfg automsgConfig
		if err := json.Unmarshal(e.Data, &cfg); err != nil {
			continue
		}
		modeEmoji := "🔰"
		if cfg.Mode == "once" {
			modeEmoji = "1️⃣"
		}
		automsgMu.Lock()
		_, active := automsgTimers[e.ID]
		automsgMu.Unlock()
		statusIcon := "🔴 OFF"
		if active {
			statusIcon = "🟢 ON"
		}
		msgPreview := cfg.Message
		if len(msgPreview) > 40 {
			msgPreview = msgPreview[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("*%d.* %s *%s* %s\n", i+1, modeEmoji, strings.ToUpper(cfg.Mode), statusIcon))
		sb.WriteString(fmt.Sprintf("   🔰 *TIME :* `%s`\n", formatHMS(time.Duration(cfg.DurationSec)*time.Second)))
		sb.WriteString(fmt.Sprintf("   🔰 *MSG  :* %s\n", msgPreview))
		sb.WriteString(fmt.Sprintf("   🔰 *CHAT :* `%s`\n", cfg.Chat))
		sb.WriteString("\n")
	}
	sb.WriteString("───────────────────\n")
	sb.WriteString("🔰 *MENTION REPLY A NUMBER TO DELETE MESSAGE* 🔰\n")
	sb.WriteString("🔰 *Example:* reply `1` to delete schedule #1\n")
	sb.WriteString("🔰 *(reply within 2 minutes)*\n")
	sb.WriteString("🔰 *SAVED IN :* BOT MEMORY 🔰")
	s.Reply(info, sb.String())
}

// ── handleAutomsgDeleteByNumber ─────────────────────────────────────────────
// Called when the owner replies a plain number after .automsg delete.
func handleAutomsgDeleteByNumber(s SessionBridge, info types.MessageInfo, prefix string, pending *automsgPendingDelete, numStr string) {
	n, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil || n < 1 || n > len(pending.keys) {
		s.Reply(info, "🔰 *Invalid number.*\n\n*Please reply a number between 1 and "+strconv.Itoa(len(pending.keys))+".*\n*Or type* ```"+prefix+"automsg delete``` *to see the list again.*")
		return
	}
	key := pending.keys[n-1]

	// cancel the timer if active
	automsgCancel(key)

	// delete from bot memory
	delErr := error(nil)
	if s.MemoryReady() {
		delErr = s.MemoryDelete(key)
	}

	// clear the pending list
	dpk := delPendingKey(s.GetJID(), info.Sender.String())
	automsgDelMu.Lock()
	delete(automsgDelPending, dpk)
	automsgDelMu.Unlock()

	if delErr != nil {
		s.Reply(info, "🔰 *DELETE FAILED*\n\n*Timer cancelled but memory cleanup failed:*\n"+delErr.Error())
		return
	}
	s.Reply(info, fmt.Sprintf(
		"🔰 *AUTOMSG DELETED* 🔰\n\n"+
			"🔰 *Schedule #%d deleted successfully*\n"+
			"🔰 *Removed from bot memory*\n"+
			"🔰 *Timer cancelled*\n\n"+
			"🔰 *SAVED IN :* BOT MEMORY 🔰\n"+
			"*To see remaining schedules type:* ```%sautomsg list```",
		n, prefix,
	))
}

// ── .automsg on ─────────────────────────────────────────────────────────────
// Re-arms the current chat's saved schedule. If none saved → guidance.
func handleAutomsgOn(s SessionBridge, info types.MessageInfo, prefix string, key string) {
	if !s.MemoryReady() {
		s.Reply(info, "🔰 *Bot memory is not ready yet.*\n\n*The bot saves schedules in its own memory.*\n*Please make sure the bot is fully connected and try again.*")
		return
	}
	// already active?
	automsgMu.Lock()
	t, active := automsgTimers[key]
	automsgMu.Unlock()
	if active && t != nil {
		remaining := time.Until(t.nextFire)
		if remaining < 0 {
			remaining = 0
		}
		s.Reply(info, fmt.Sprintf(
			"🔰 *AUTOMSG ALREADY ON* 🔰\n\n"+
				"🟢 *This chat's schedule is already active.*\n\n"+
				"🔰 *DURATION :❰ %s ❱*\n"+
				"🔰 *MODE     :❰ %s ❱*\n"+
				"🔰 *MESSAGE  :❰ %s ❱*\n"+
				"🔰 *NEXT FIRE IN :❰ %s ❱*\n\n"+
				"*TO TURN OFF TYPE:* ```%sautomsg off```",
			formatHMS(time.Duration(t.cfg.DurationSec)*time.Second),
			strings.ToUpper(t.cfg.Mode),
			t.cfg.Message,
			formatHMS(remaining),
			prefix,
		))
		return
	}
	// load saved config from memory
	data, ok, err := s.MemoryLoad(key)
	if err != nil {
		s.Reply(info, "🔰 *AUTOMSG ON ERROR*\n\n*Bot could not read its memory:*\n"+err.Error())
		return
	}
	if !ok || len(data) == 0 {
		// no schedule saved → guidance
		s.Reply(info, automsgNoScheduleGuide(prefix))
		return
	}
	var cfg automsgConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		s.Reply(info, "🔰 *AUTOMSG ON ERROR*\n\n*Saved schedule is corrupted:*\n"+err.Error())
		return
	}
	dur := time.Duration(cfg.DurationSec) * time.Second
	if dur <= 0 {
		s.Reply(info, "🔰 *AUTOMSG ON ERROR*\n\n*Saved schedule has an invalid duration.*\n*Please set a new one:* ```"+prefix+"automsg XXhXXmXXs repeat/once {msg}```")
		return
	}
	// re-arm
	armAutomsgTimer(s, info, key, cfg, dur)

	modeEmoji := "🔰"
	modeLabel := "REPEAT"
	if cfg.Mode == "once" {
		modeEmoji = "1️⃣"
		modeLabel = "ONCE"
	}
	s.Reply(info, fmt.Sprintf(
		"🔰 *AUTOMSG TURNED ON* 🔰\n\n"+
			"🟢 *Schedule resumed successfully*\n\n"+
			"🔰 *TIME     :❰ %s ❱*\n"+
			"%s *MODE     :❰ %s ❱*\n"+
			"🔰 *MESSAGE  :❰ %s ❱*\n"+
			"🔰 *FIRST FIRE IN :❰ %s ❱*\n"+
			"🔰 *SAVED IN :❰ %s ❱*\n\n"+
			"*TO TURN OFF TYPE:* ```%sautomsg off```\n"+
			"*TO SEE ALL TYPE:* ```%sautomsg list```",
		formatHMS(dur),
		modeEmoji, modeLabel,
		cfg.Message,
		formatHMS(dur),
		"BOT MEMORY 🔰",
		prefix, prefix,
	))
}

// ── .automsg off ────────────────────────────────────────────────────────────
// Cancels the current chat's active timer (pause). Config STAYS in memory.
func handleAutomsgOff(s SessionBridge, info types.MessageInfo, prefix string, key string) {
	automsgMu.Lock()
	t, ok := automsgTimers[key]
	automsgMu.Unlock()
	if !ok || t == nil {
		s.Reply(info, "🔰 *AUTOMSG OFF* 🔰\n\n🔰 *No active auto-message schedule in this chat.*\n\n*To set one type:* ```"+prefix+"automsg XXhXXmXXs repeat/once {msg}```\n*To turn on a saved one type:* ```"+prefix+"automsg on```")
		return
	}
	cfg := t.cfg
	automsgCancel(key)
	s.Reply(info, fmt.Sprintf(
		"🔰 *AUTOMSG TURNED OFF* 🔰\n\n"+
			"🔰 *Active timer cancelled (paused)*\n"+
			"🔰 *Schedule STAYS safe in bot memory*\n\n"+
			"🔰 *TIME     :❰ %s ❱*\n"+
			"🔰 *MODE     :❰ %s ❱*\n"+
			"🔰 *MESSAGE  :❰ %s ❱*\n\n"+
			"*TO TURN BACK ON TYPE:* ```%sautomsg on```\n"+
			"*TO DELETE PERMANENTLY TYPE:* ```%sautomsg delete```",
		formatHMS(time.Duration(cfg.DurationSec)*time.Second),
		strings.ToUpper(cfg.Mode),
		cfg.Message,
		prefix, prefix,
	))
}

// automsgNoScheduleGuide is shown when .automsg on is used but nothing is saved.
func automsgNoScheduleGuide(prefix string) string {
	return "🔰 *AUTOMSG ON* 🔰\n\n" +
		"🔰 *No saved schedule found in this chat.*\n\n" +
		"*PEHLE MESSAGE SET KARO!* 🔰\n\n" +
		"╭─ 🔰 *FORMAT* ─╮\n" +
		"│ ```" + prefix + "automsg XXhXXmXXs repeat/once {msg}```\n" +
		"╰──────────────╯\n\n" +
		"*EXAMPLES:*\n" +
		"🔰 ```" + prefix + "automsg 00h30m00s repeat Assalamualaikum!```\n" +
		"   → *sends every 30 min, timer resets each time*\n" +
		"   → *schedule STAYS in bot memory (persists across restart)*\n\n" +
		"🔰 ```" + prefix + "automsg 02h00m00s once Meeting at 5pm```\n" +
		"   → *sends ONCE after 2 hours*\n" +
		"   → *schedule auto-removed from bot memory after send*\n\n" +
		"*MODES:*\n" +
		"🔰 *repeat* = loop forever (time counts down bar-bar)\n" +
		"1️⃣ *once*   = single send then auto-cleanup\n\n" +
		"*AFTER SETTING, USE:*\n" +
		"🟢 ```" + prefix + "automsg on```  → turn on / resume\n" +
		"🔴 ```" + prefix + "automsg off``` → turn off / pause\n" +
		"🔰 ```" + prefix + "automsg list``` → see all schedules\n" +
		"🔰 ```" + prefix + "automsg delete``` → delete a schedule\n\n" +
		"*THE BOT SAVES YOUR SCHEDULE IN ITS OWN MEMORY 🔰*"
}

// armAutomsgTimer starts (or re-arms) the background countdown timer.
//
// 0% speed impact: uses time.AfterFunc which runs the callback in its own
// goroutine. The message handler is never blocked.
//
// For repeat mode: on fire → send msg → RESET duration → re-arm → countdown
// starts again from the top (bar-bar count). Schedule STAYS in bot memory.
func armAutomsgTimer(s SessionBridge, info types.MessageInfo, key string, cfg automsgConfig, dur time.Duration) {
	nextFire := time.Now().Add(dur)

	automsgMu.Lock()
	// cancel an existing timer for this key if present (defensive)
	if old, ok := automsgTimers[key]; ok && old != nil && old.timer != nil {
		old.timer.Stop()
	}
	t := &automsgTimer{
		cfg:      cfg,
		nextFire: nextFire,
	}
	automsgTimers[key] = t
	automsgMu.Unlock()

	t.timer = time.AfterFunc(dur, func() {
		// ── FIRE: send the message ──
		// Use a copy of info so Reply targets the right chat.
		fireInfo := info
		s.Reply(fireInfo, cfg.Message)

		if cfg.Mode == "once" {
			// once → remove schedule from bot memory + remove timer entry
			if s.MemoryReady() {
				_ = s.MemoryDelete(key)
			}
			automsgMu.Lock()
			delete(automsgTimers, key)
			automsgMu.Unlock()
			return
		}

		// repeat → RESET the countdown & re-arm (time counts bar-bar)
		// refresh nextFire in bot memory so countdown survives a restart
		newNext := time.Now().Add(dur)
		cfg.NextFireMs = newNext.UnixMilli()
		if data, err := json.Marshal(cfg); err == nil && s.MemoryReady() {
			_ = s.MemorySave(key, data)
		}
		armAutomsgTimer(s, info, key, cfg, dur)
	})
}

// automsgCancel stops & removes the in-memory timer for a key (no memory touch).
func automsgCancel(key string) {
	automsgMu.Lock()
	defer automsgMu.Unlock()
	if t, ok := automsgTimers[key]; ok {
		if t != nil && t.timer != nil {
			t.timer.Stop()
		}
		delete(automsgTimers, key)
	}
}

// ── .automsg status ────────────────────────────────────────────────────
// Shows the current chat's schedule with a LIVE countdown. If the timer is
// active it reads the in-memory nextFire; if paused (off) it shows the saved
// config from bot memory with "paused" status.
func handleAutomsgStatus(s SessionBridge, info types.MessageInfo, prefix string, key string) {
	automsgMu.Lock()
	t, active := automsgTimers[key]
	automsgMu.Unlock()
	if active && t != nil {
		remaining := time.Until(t.nextFire)
		if remaining < 0 {
			remaining = 0
		}
		modeEmoji := "🔴"
		modeLabel := "REPEAT"
		if t.cfg.Mode == "once" {
			modeEmoji = "1️⃣"
			modeLabel = "ONCE"
		}
		s.Reply(info, fmt.Sprintf(
			"🔴 *AUTOMSG STATUS* 🔴\n\n"+
				"🟢 *SCHEDULE IS ACTIVE*\n\n"+
				"🔴 *TIME     :❰ %s ❱*\n"+
				"%s *MODE     :❰ %s ❱*\n"+
				"🔴 *MESSAGE  :❰ %s ❱*\n"+
				"🔴 *CHAT     :❰ %s ❱*\n"+
				"⏱ *NEXT FIRE IN :❰ %s ❱*\n\n"+
				"*TO PAUSE TYPE:* ```%sautomsg off```\n"+
				"*TO CANCEL TYPE:* ```%sautomsg stop```",
			formatHMS(time.Duration(t.cfg.DurationSec)*time.Second),
			modeEmoji, modeLabel,
			t.cfg.Message,
			t.cfg.Chat,
			formatHMS(remaining),
			prefix, prefix,
		))
		return
	}
	// no live timer → check memory for a paused schedule
	if s.MemoryReady() {
		if data, ok, _ := s.MemoryLoad(key); ok && len(data) > 0 {
			var cfg automsgConfig
			if json.Unmarshal(data, &cfg) == nil {
				s.Reply(info, fmt.Sprintf(
					"🔴 *AUTOMSG STATUS* 🔴\n\n"+
						"🔴 *SCHEDULE IS PAUSED (OFF)*\n\n"+
						"🔴 *TIME     :❰ %s ❱*\n"+
						"🔴 *MODE     :❰ %s ❱*\n"+
						"🔴 *MESSAGE  :❰ %s ❱*\n"+
						"🔴 *CHAT     :❰ %s ❱*\n\n"+
						"*TO RESUME TYPE:* ```%sautomsg on```\n"+
						"*TO CANCEL TYPE:* ```%sautomsg stop```",
					formatHMS(time.Duration(cfg.DurationSec)*time.Second),
					strings.ToUpper(cfg.Mode),
					cfg.Message,
					cfg.Chat,
					prefix, prefix,
				))
				return
			}
		}
	}
	s.Reply(info, "🔴 *AUTOMSG STATUS* 🔴\n\n🔴 *No active or saved auto-message schedule in this chat.*\n\n*To set one type:* ```"+prefix+"automsg XXhXXmXXs repeat/once {msg}```")
}

// ── .automsg stop ────────────────────────────────────────────────────
// Cancel + SAFE-DELETE: cancels the live timer AND removes the config from
// bot memory (idempotent — no error if already gone). Ye file ke header docs
// me described tha ("stop → cancel the active timer + safe-delete the
// config from the bot's MEMORY") par handleAutomsg me "stop" ka dispatch hi
// missing tha — ab added.
func handleAutomsgStop(s SessionBridge, info types.MessageInfo, prefix string, key string) {
	// snapshot config (agar live timer hai) for the confirmation text
	automsgMu.Lock()
	t, active := automsgTimers[key]
	automsgMu.Unlock()
	var hadCfg automsgConfig
	had := false
	if active && t != nil {
		hadCfg = t.cfg
		had = true
	} else if s.MemoryReady() {
		if data, ok, _ := s.MemoryLoad(key); ok && len(data) > 0 {
			if json.Unmarshal(data, &hadCfg) == nil {
				had = true
			}
		}
	}
	// cancel live timer
	automsgCancel(key)
	// safe-delete from bot memory
	delErr := error(nil)
	if s.MemoryReady() {
		delErr = s.MemoryDelete(key)
	}
	if delErr != nil {
		s.Reply(info, "🔴 *AUTOMSG STOP FAILED*\n\n*Timer cancelled but memory cleanup failed:*\n"+delErr.Error())
		return
	}
	if !had {
		s.Reply(info, "🔴 *AUTOMSG STOP* 🔴\n\n🔴 *No active auto-message schedule in this chat.*\n\n*To set one type:* ```"+prefix+"automsg XXhXXmXXs repeat/once {msg}```")
		return
	}
	s.Reply(info, fmt.Sprintf(
		"🔴 *AUTOMSG STOPPED* 🔴\n\n"+
			"❌ *Schedule cancelled & removed from bot memory*\n\n"+
			"🔴 *TIME     :❰ %s ❱*\n"+
			"🔴 *MODE     :❰ %s ❱*\n"+
			"🔴 *MESSAGE  :❰ %s ❱*\n\n"+
			"*TO SET A NEW ONE TYPE:* ```%sautomsg XXhXXmXXs repeat/once {msg}```",
		formatHMS(time.Duration(hadCfg.DurationSec)*time.Second),
		strings.ToUpper(hadCfg.Mode),
		hadCfg.Message,
		prefix,
	))
}

// ── AutomsgRestoreSavedSchedules ───────────────────────────────────────
// RESTART RESTORE: bot restart/reconnect hone par manager.go ke
// *events.Connected handler se call hota hai. Bot ki MEMORY (automsg/ prefix
// objects) me saved har repeat schedule ko padhta hai aur timer RE-ARM kar
// deta hai — is tarah "persists across restart" wala promise actually poora
// hota hai (pehle config save hota tha par restart ke baad koi re-arm nahi
// karta tha, isliye schedule chupchaap mar jata tha).
//
// once-mode schedules restore NAHI hote — wo one-shot hote hain; agar bot
// down tha jab fire hona tha, to ab wapas arm karna risky hai (delayed send
// confusion). Owner dobara set kar sakta hai.
//
// Restore deadline: agar NextFireMs already 2+ durations peeche chuka hai
// (bot bahut der se down tha), to missed cycles nahi bheje jaate — seedha
// fresh full-duration timer arm hota hai.
func AutomsgRestoreSavedSchedules(s SessionBridge) {
	if !s.MemoryReady() {
		return
	}
	entries, err := s.MemoryList()
	if err != nil || len(entries) == 0 {
		return
	}
	restored := 0
	for _, e := range entries {
		var cfg automsgConfig
		if json.Unmarshal(e.Data, &cfg) != nil {
			continue
		}
		// sirf repeat schedules re-arm karo
		if cfg.Mode != "repeat" {
			continue
		}
		dur := time.Duration(cfg.DurationSec) * time.Second
		if dur <= 0 {
			continue
		}
		key := e.ID
		// already armed? (double Connected event guard)
		automsgMu.Lock()
		_, active := automsgTimers[key]
		automsgMu.Unlock()
		if active {
			continue
		}
		// synthetic MessageInfo — Reply sirf info.Chat use karta hai
		chat, jerr := types.ParseJID(cfg.Chat)
		if jerr != nil || chat.IsEmpty() || chat.Server == "" {
			continue
		}
		info := types.MessageInfo{}
		info.Chat = chat
		armAutomsgTimer(s, info, key, cfg, dur)
		restored++
	}
	if restored > 0 {
		// SILENT (owner request): console log off
		// fmt.Printf("[automsg] restart-restore: %d repeat schedule(s) re-armed from bot memory\n", restored)
		_ = restored
	}
}

// automsgUsage returns the GOLD-MD styled help text (no-arg reply).
func automsgUsage(prefix string) string {
	return "🔰 *GOLD-MD AUTOMSG* 🔰\n\n" +
		"*SCHEDULE A MESSAGE TO SEND AUTOMATICALLY AFTER A SET TIME*\n\n" +
		"╭─ 🔰 *FORMAT* ─╮\n" +
		"│ ```" + prefix + "automsg XXhXXmXXs repeat/once {msg}```\n" +
		"╰──────────────╯\n\n" +
		"*EXAMPLES:*\n" +
		"🔰 ```" + prefix + "automsg 00h30m00s repeat Assalamualaikum!```\n" +
		"   → *sends every 30 min, timer resets & counts again each time*\n" +
		"   → *schedule STAYS in bot memory (persists across restart)*\n\n" +
		"🔰 ```" + prefix + "automsg 02h00m00s once Meeting at 5pm```\n" +
		"   → *sends ONCE after 2 hours*\n" +
		"   → *schedule auto-removed from bot memory after send*\n\n" +
		"*MODES:*\n" +
		"🔰 *repeat* = loop forever (time counts down bar-bar)\n" +
		"1️⃣ *once*   = single send then auto-cleanup\n\n" +
		"*COMMANDS:*\n" +
		"🟢 ```" + prefix + "automsg on```     → turn on / resume saved schedule\n" +
		"🔴 ```" + prefix + "automsg off```    → turn off / pause (config stays in memory)\n" +
		"🟢 ```" + prefix + "automsg status```  → live countdown of this chat\n" +
		"🔰 ```" + prefix + "automsg list```   → see ALL saved schedules (numbered)\n" +
		"🔰 ```" + prefix + "automsg delete``` → delete a schedule (reply a number)\n\n" +
		"*TIME FORMAT: XXhXXmXXs (hours minutes seconds)*\n" +
		"*THE BOT SAVES YOUR SCHEDULE IN ITS OWN MEMORY 🔰*"
}

func init() {
	Register(Command{
		Name:      "automsg",
		Category:  "OWNER & SYSTEM",
		Desc:      "THIS COMMAND IS USED TO SET MESSAGES THAT THE BOT SENDS BY ITSELF. YOU CAN SET THE TIME AND TEXT.",
		OwnerOnly: true,
		Run:       handleAutomsg,
	})
	// hidden aliases
	Register(Command{Name: "automessage", OwnerOnly: true, Hidden: true, Run: handleAutomsg})
	Register(Command{Name: "autosend", OwnerOnly: true, Hidden: true, Run: handleAutomsg})
}
