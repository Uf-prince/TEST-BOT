package goldcmds

// ============================================================================
// GOLD-MD — TIMED ADMIN Commands
// File: timedadmin.go
// ----------------------------------------------------------------------------
// COMMAND 1: .dissmisstime XXhXXmXXs @mention      (owner + groups)
//
//   .dissmisstime 00h05m00s @user
//        → after 5 minutes the bot DEMOTES @user (removes admin power).
//          The timer COUNTS DOWN in the background; when it hits 0 the bot
//          directly applies the demote action. NO admin check is performed —
//          the action is applied straight away (owner requirement).
//
// COMMAND 2: .admintime XXhXXmXXs @mention         (owner + groups)
//
//   .admintime 01h00m00s @user
//        → the bot PROMOTES @user to admin for 1 hour. When the time is up the
//          bot DEMOTES @user back to a normal member automatically.
//
// DESIGN (mirrors .automsg — owner requirement):
//   • Bot speed = 0% farak. The whole timer runs in a background goroutine via
//     time.AfterFunc — it NEVER blocks the message handler / Reply path. The
//     command returns immediately after arming the timer.
//   • RAM = 0% farak. Only a tiny struct + one *time.Timer per active timer is
//     held in memory; nothing is polled, nothing is scanned on normal messages.
//   • Time counts down bar-bar (XXhXXmXXs → 0 → action applied).
//   • The bot saves the timer in its own MEMORY (the same secure place it keeps
//     antidelete / antiedit / automsg data) so a pending timer SURVIVES a
//     restart — on reconnect the timer is re-armed from memory.
//   • NO admin check anywhere: the action is applied directly (owner order).
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ── memory namespace (keeps these timers OUT of .automsg list) ───────────────
const timedAdminNS = "timedadmin"

// ── active timers (in-memory, per botJID+chat+target+kind) ───────────────────
//
// This map ONLY holds live *time.Timer pointers so a timer can be cancelled /
// replaced. The durable config lives in the bot's MEMORY. 0% speed impact: the
// map is touched only on arm/restore, never on normal messages.
type timedAdminTimer struct {
	timer    *time.Timer
	cfg      timedAdminConfig
	nextFire time.Time
}

var (
	timedAdminMu     sync.Mutex
	timedAdminTimers = make(map[string]*timedAdminTimer)
)

// timedAdminConfig is the JSON the bot keeps in its MEMORY under
// timedadmin/<id>.json.
type timedAdminConfig struct {
	Kind        string `json:"kind"`         // "dismiss" or "admin"
	Chat        string `json:"chat"`         // group JID
	BotJID      string `json:"bot_jid"`      // which bot session owns this
	Target      string `json:"target"`       // target member JID
	DurationSec int64  `json:"duration_sec"` // total countdown seconds
	FireAtMs    int64  `json:"fire_at_ms"`   // unix-milli when the action fires
	CreatedMs   int64  `json:"created_ms"`   // when first armed
}

// timedAdminKey builds the in-memory timer-map key and the MEMORY object id.
func timedAdminKey(botJID, chat, target, kind string) string {
	return botJID + "|" + chat + "|" + target + "|" + kind
}

// ── flexible time parsing ────────────────────────────────────────────────────
//
// The owner can write the time in ANY of these shapes (case-insensitive):
//
//	00h05m00s   (canonical, same as .automsg)
//	5m          (minutes only)
//	1h30m       (hours + minutes)
//	45s         (seconds only)
//	1h          (hours only)
//	00:05:00    (colon form)
//	5           (bare number = minutes)
//
// The token may appear ANYWHERE in the command (before or after the @mention),
// so we scan every arg and pick the first one that parses as a duration.
var (
	timedAdminHMSRe  = regexp.MustCompile(`^(\d+)h(\d+)m(\d+)s$`)
	timedAdminHMRe   = regexp.MustCompile(`^(\d+)h(\d+)m$`)
	timedAdminHSRe   = regexp.MustCompile(`^(\d+)h(\d+)s$`)
	timedAdminHRe    = regexp.MustCompile(`^(\d+)h(?:r|rs|our|ours)?$`)
	timedAdminMRe    = regexp.MustCompile(`^(\d+)m(?:in|ins|inute|inutes)?$`)
	timedAdminSRe    = regexp.MustCompile(`^(\d+)s(?:ec|ecs|econd|econds)?$`)
	timedAdminDRe    = regexp.MustCompile(`^(\d+)d(?:ay|ays)?$`)
	timedAdminColon  = regexp.MustCompile(`^(\d+):(\d+):(\d+)$`)
	timedAdminColon2 = regexp.MustCompile(`^(\d+):(\d+)$`)
	timedAdminBare   = regexp.MustCompile(`^(\d+)$`)
)

// parseTimedAdminDuration accepts the flexible shapes above and returns the
// total duration. It is a superset of parseAutomsgDuration so the canonical
// XXhXXmXXs form keeps working exactly like .automsg.
func parseTimedAdminDuration(token string) (time.Duration, bool) {
	t := strings.ToLower(strings.TrimSpace(token))
	// strip zero-width / invisible chars WhatsApp sometimes injects, plus
	// surrounding punctuation, so "00h05m00s\u200b" or "`5m`" still parse.
	t = strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff', '\u00a0':
			return -1
		}
		return r
	}, t)
	t = strings.Trim(t, "`'\".,;!?()[]{}<>*_~ ")
	if t == "" {
		return 0, false
	}
	// canonical XXhXXmXXs (reuse the automsg parser for identical behaviour)
	if timedAdminHMSRe.MatchString(t) {
		return parseAutomsgDuration(t)
	}
	if m := timedAdminColon.FindStringSubmatch(t); m != nil {
		h, _ := strconv.ParseInt(m[1], 10, 64)
		mn, _ := strconv.ParseInt(m[2], 10, 64)
		sec, _ := strconv.ParseInt(m[3], 10, 64)
		return timedAdminBuildDur(h, mn, sec)
	}
	// MM:SS form (e.g. 05:00 = 5 minutes)
	if m := timedAdminColon2.FindStringSubmatch(t); m != nil {
		mn, _ := strconv.ParseInt(m[1], 10, 64)
		sec, _ := strconv.ParseInt(m[2], 10, 64)
		return timedAdminBuildDur(0, mn, sec)
	}
	if m := timedAdminHMRe.FindStringSubmatch(t); m != nil {
		h, _ := strconv.ParseInt(m[1], 10, 64)
		mn, _ := strconv.ParseInt(m[2], 10, 64)
		return timedAdminBuildDur(h, mn, 0)
	}
	if m := timedAdminHSRe.FindStringSubmatch(t); m != nil {
		h, _ := strconv.ParseInt(m[1], 10, 64)
		sec, _ := strconv.ParseInt(m[2], 10, 64)
		return timedAdminBuildDur(h, 0, sec)
	}
	if m := timedAdminHRe.FindStringSubmatch(t); m != nil {
		h, _ := strconv.ParseInt(m[1], 10, 64)
		return timedAdminBuildDur(h, 0, 0)
	}
	if m := timedAdminMRe.FindStringSubmatch(t); m != nil {
		mn, _ := strconv.ParseInt(m[1], 10, 64)
		return timedAdminBuildDur(0, mn, 0)
	}
	if m := timedAdminSRe.FindStringSubmatch(t); m != nil {
		sec, _ := strconv.ParseInt(m[1], 10, 64)
		return timedAdminBuildDur(0, 0, sec)
	}
	// days (e.g. 1d, 2days) → hours
	if m := timedAdminDRe.FindStringSubmatch(t); m != nil {
		d, _ := strconv.ParseInt(m[1], 10, 64)
		return timedAdminBuildDur(d*24, 0, 0)
	}
	// bare number → treat as MINUTES (e.g. ".admintime 5 @user" = 5 minutes)
	if m := timedAdminBare.FindStringSubmatch(t); m != nil {
		mn, _ := strconv.ParseInt(m[1], 10, 64)
		return timedAdminBuildDur(0, mn, 0)
	}
	return 0, false
}

// timedAdminBuildDur validates ranges and builds the duration.
func timedAdminBuildDur(h, mn, sec int64) (time.Duration, bool) {
	if mn < 0 || mn > 59 || sec < 0 || sec > 59 || h < 0 || h > 8784 {
		return 0, false
	}
	total := time.Duration(h)*time.Hour + time.Duration(mn)*time.Minute + time.Duration(sec)*time.Second
	if total <= 0 {
		return 0, false
	}
	return total, true
}

// timedAdminFindDuration scans every arg and returns the first token that
// parses as a duration, plus its index. This makes the command work whether
// the owner writes the time BEFORE or AFTER the @mention.
func timedAdminFindDuration(args []string) (time.Duration, int, bool) {
	// 1) single token (most common: 00h05m00s / 5m / 1h30m / 00:05:00 / 5)
	for i, a := range args {
		if d, ok := parseTimedAdminDuration(a); ok {
			return d, i, true
		}
	}
	// 2) space-separated parts (e.g. "00h 05m 00s" or "1h 30m") — join up to
	//    3 consecutive tokens and try to parse the concatenation.
	for i := 0; i < len(args); i++ {
		joined := ""
		for j := i; j < len(args) && j < i+3; j++ {
			joined += strings.TrimSpace(args[j])
			if d, ok := parseTimedAdminDuration(joined); ok {
				return d, i, true
			}
		}
	}
	return 0, -1, false
}

// ── registration ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{
		Name:      "dissmisstime",
		Category:  "GROUP MANAGEMENT",
		Desc:      "THIS COMMAND IS USED TO REMOVE ADMIN POWER FROM A MEMBER AFTER A SET TIME. TAG THE MEMBER AND GIVE A TIME LIKE 00h05m00s.",
		OwnerOnly: true,
		Run:       handleDismissTime,
	})
	Register(Command{
		Name:      "admintime",
		Category:  "GROUP MANAGEMENT",
		Desc:      "THIS COMMAND IS USED TO MAKE A MEMBER ADMIN FOR A SET TIME. TAG THE MEMBER AND GIVE A TIME LIKE 01h00m00s. THE BOT REMOVES ADMIN POWER WHEN THE TIME IS UP.",
		OwnerOnly: true,
		Run:       handleAdminTime,
	})

	// ── hidden aliases (5+ each, same handler) ──
	Register(Command{Name: "dismisstime", OwnerOnly: true, Hidden: true, Run: handleDismissTime})
	Register(Command{Name: "demotetime", OwnerOnly: true, Hidden: true, Run: handleDismissTime})
	Register(Command{Name: "timedismiss", OwnerOnly: true, Hidden: true, Run: handleDismissTime})
	Register(Command{Name: "tempdemote", OwnerOnly: true, Hidden: true, Run: handleDismissTime})
	Register(Command{Name: "autodemote", OwnerOnly: true, Hidden: true, Run: handleDismissTime})

	Register(Command{Name: "tempadmin", OwnerOnly: true, Hidden: true, Run: handleAdminTime})
	Register(Command{Name: "timedadmin", OwnerOnly: true, Hidden: true, Run: handleAdminTime})
	Register(Command{Name: "adminfortime", OwnerOnly: true, Hidden: true, Run: handleAdminTime})
	Register(Command{Name: "tempromote", OwnerOnly: true, Hidden: true, Run: handleAdminTime})
	Register(Command{Name: "timedadm", OwnerOnly: true, Hidden: true, Run: handleAdminTime})
}

// ── handlers ─────────────────────────────────────────────────────────────────

func handleDismissTime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTimedAdminAsync(s, info, args, prefix, "dismiss")
}

func handleAdminTime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleTimedAdminAsync(s, info, args, prefix, "admin")
}

func handleTimedAdminAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string, kind string) {
	cmdName := "dissmisstime"
	actionWord := "DISMISS"
	if kind == "admin" {
		cmdName = "admintime"
		actionWord = "ADMIN"
	}

	// ── OWNER CHECK (direct — no admin check anywhere) ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	// ── SIRF GROUP ──
	if !info.IsGroup {
		s.Reply(info, "*"+strings.ToUpper(cmdName)+" ONLY WORKS IN GROUPS 🔰*")
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}

	// ── no args → usage guide ──
	if len(args) == 0 {
		s.Reply(info, timedAdminUsage(prefix, kind))
		return
	}

	// ── .dissmisstime stop @user / .admintime stop @user → cancel ──
	if timedAdminTryStop(s, info, args, prefix, kind) {
		return
	}

	// ── parse time token (flexible: XXhXXmXXs / 5m / 1h30m / 00:05:00 / 5) ──
	// The time may appear BEFORE or AFTER the @mention, so we scan all args.
	dur, durIdx, ok := timedAdminFindDuration(args)
	if !ok {
		s.Reply(info, "🔰 *Invalid time format.*\n\n*Time must be like:* ```XXhXXmXXs```\n*Example:* ```00h05m00s``` (= 5 minutes)\n*Example:* ```01h00m00s``` (= 1 hour)\n\n*You can also write:* ```5m``` (= 5 min), ```1h30m```, ```45s```, ```00:05:00```")
		return
	}
	durToken := strings.TrimSpace(args[durIdx])

	// ── resolve target (mention or reply) ──
	targets := targetJIDs(s, info)
	if len(targets) == 0 {
		s.Reply(info, "*TAG THE MEMBER YOU WANT TO "+actionWord+" 🔰*\n\n*Example:* ```"+prefix+cmdName+" "+durToken+" @user```")
		return
	}
	target := targets[0]

	// don't target the bot itself
	botJID := client.Store.ID
	if botJID != nil && target.String() == botJID.String() {
		s.Reply(info, "*YOU CAN'T "+actionWord+" ME 🔰*")
		return
	}

	// ── bot memory must be ready (timer is persisted there) ──
	if !s.MemoryReady() {
		s.Reply(info, "🔰 *Bot memory is not ready yet.*\n\n*The bot saves your timer in its own memory (the same secure place it keeps antidelete & antiedit messages).*\n*Please make sure the bot is fully connected and try again.*")
		return
	}

	botJIDStr := s.GetJID()
	chatStr := info.Chat.String()
	key := timedAdminKey(botJIDStr, chatStr, target.String(), kind)

	// cancel any existing timer for this exact target+kind (replace)
	timedAdminCancel(key)

	// build + save config to the bot's MEMORY
	cfg := timedAdminConfig{
		Kind:        kind,
		Chat:        chatStr,
		BotJID:      botJIDStr,
		Target:      target.String(),
		DurationSec: int64(dur.Seconds()),
		FireAtMs:    time.Now().Add(dur).UnixMilli(),
		CreatedMs:   time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(cfg)
	if err := s.MemorySaveNS(timedAdminNS, key, data); err != nil {
		s.Reply(info, "🔰 *TIMED ADMIN Error*\n\n*Bot could not save the timer to its memory:*\n"+err.Error())
		return
	}

	// ── IMMEDIATE ACTION for .admintime ──
	// Owner order: as soon as the time is set the bot must make the member
	// admin RIGHT AWAY (foran). The timer then only DEMOTES them when the
	// time is up. For .dissmisstime nothing happens now — the demote is
	// applied when the timer fires.
	if kind == "admin" {
		if _, perr := client.UpdateGroupParticipants(context.Background(), info.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote); perr != nil {
			s.Reply(info, "🔰 *ADMIN TIME Error*\n\n*Bot could not make the member admin:*\n"+perr.Error())
			// roll back the saved timer so we don't demote a non-admin later
			timedAdminCancel(key)
			if s.MemoryReady() {
				_ = s.MemoryDeleteNS(timedAdminNS, key)
			}
			return
		}
	}

	// arm the timer (0% speed impact — runs fully in background via AfterFunc)
	armTimedAdminTimer(s, key, cfg, dur)

	// ── styled confirmation ──
	title := "🔰 *DISMISS TIMER SET* 🔰"
	actionLine := "*WILL BE DISMISSED (ADMIN REMOVED) AFTER:*"
	if kind == "admin" {
		title = "🔰 *ADMIN TIMER SET* 🔰"
		actionLine = "*IS NOW ADMIN — WILL BE DEMOTED AFTER:*"
	}
	s.Reply(info, fmt.Sprintf(
		"%s\n\n"+
			"🔰 *TIME     :❮ %s ❯*\n"+
			"🔰 *MEMBER   :❮ @%s ❯*\n"+
			"🔰 *GROUP    :❮ %s ❯*\n\n"+
			"%s\n*❮ %s ❯*\n\n"+
			"🔰 *SAVED IN :❮ BOT MEMORY 🔰 ❯*\n"+
			"*TIMER COUNTS DOWN IN BACKGROUND — 0%% SPEED & RAM IMPACT*\n\n"+
			"*TO CANCEL TYPE:* ```%s%s stop @%s```",
		title,
		formatHMS(dur),
		target.User,
		chatStr,
		actionLine,
		formatHMS(dur),
		prefix, cmdName, target.User,
	))
}

// ── timer arming ─────────────────────────────────────────────────────────────

// armTimedAdminTimer schedules the action with time.AfterFunc. 0% speed impact:
// the callback runs in its own goroutine; the message handler is never blocked.
func armTimedAdminTimer(s SessionBridge, key string, cfg timedAdminConfig, dur time.Duration) {
	nextFire := time.Now().Add(dur)

	timedAdminMu.Lock()
	if old, ok := timedAdminTimers[key]; ok && old != nil && old.timer != nil {
		old.timer.Stop()
	}
	t := &timedAdminTimer{cfg: cfg, nextFire: nextFire}
	timedAdminTimers[key] = t
	timedAdminMu.Unlock()

	t.timer = time.AfterFunc(dur, func() {
		timedAdminFire(s, key, cfg)
	})
}

// timedAdminFire applies the promote/demote action and cleans up.
func timedAdminFire(s SessionBridge, key string, cfg timedAdminConfig) {
	defer func() {
		// remove the in-memory timer entry
		timedAdminMu.Lock()
		delete(timedAdminTimers, key)
		timedAdminMu.Unlock()
		// remove the config from bot memory (one-shot timer)
		if s.MemoryReady() {
			_ = s.MemoryDeleteNS(timedAdminNS, key)
		}
	}()

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return
	}
	chat, err := types.ParseJID(cfg.Chat)
	if err != nil || chat.IsEmpty() {
		return
	}
	target, err := types.ParseJID(cfg.Target)
	if err != nil || target.IsEmpty() {
		return
	}

	// synthetic MessageInfo — Reply / mention only need info.Chat
	info := types.MessageInfo{}
	info.Chat = chat

	// At FIRE time BOTH commands DEMOTE the target:
	//   • dismiss → the admin's power is removed after the set time
	//   • admin   → the temporary admin's power is removed when the time is up
	//     (the promote already happened IMMEDIATELY when the command was run)
	change := whatsmeow.ParticipantChangeDemote
	title := "🔰 *MEMBER DISMISSED* 🔰"
	body := "*IS NO LONGER AN ADMIN*\n*NOW A REGULAR MEMBER*"
	if cfg.Kind == "admin" {
		title = "🔰 *ADMIN TIME OVER* 🔰"
		body = "*ADMIN POWER REMOVED*\n*NOW A REGULAR MEMBER AGAIN*"
	}

	_, err = client.UpdateGroupParticipants(context.Background(), chat, []types.JID{target}, change)
	if err != nil {
		s.Reply(info, "🔰 *TIMED ADMIN FAILED:* "+err.Error())
		return
	}

	text := title + "\n\n@" + target.User + " " + body
	_ = sendMentionText(client, info, text, []string{target.String()})
}

// timedAdminCancel stops & removes the in-memory timer for a key (no memory touch).
func timedAdminCancel(key string) {
	timedAdminMu.Lock()
	defer timedAdminMu.Unlock()
	if t, ok := timedAdminTimers[key]; ok {
		if t != nil && t.timer != nil {
			t.timer.Stop()
		}
		delete(timedAdminTimers, key)
	}
}

// ── .dissmisstime / .admintime stop @mention ─────────────────────────────────
// The handler above treats a "stop" token as an invalid time; to keep the flow
// simple and robust we expose a dedicated cancel path via the same command:
//
//	.dissmisstime stop @user   /   .admintime stop @user
//
// This is handled by intercepting the "stop" token before duration parsing.
func timedAdminTryStop(s SessionBridge, info types.MessageInfo, args []string, prefix, kind string) bool {
	// "stop" may appear anywhere (before or after the @mention).
	hasStop := false
	for _, a := range args {
		if strings.EqualFold(strings.TrimSpace(a), "stop") || strings.EqualFold(strings.TrimSpace(a), "cancel") {
			hasStop = true
			break
		}
	}
	if !hasStop {
		return false
	}
	cmdName := "dissmisstime"
	if kind == "admin" {
		cmdName = "admintime"
	}
	targets := targetJIDs(s, info)
	if len(targets) == 0 {
		s.Reply(info, "*TAG THE MEMBER WHOSE TIMER YOU WANT TO STOP 🔰*\n\n*Example:* ```"+prefix+cmdName+" stop @user```")
		return true
	}
	target := targets[0]
	key := timedAdminKey(s.GetJID(), info.Chat.String(), target.String(), kind)
	timedAdminCancel(key)
	if s.MemoryReady() {
		_ = s.MemoryDeleteNS(timedAdminNS, key)
	}
	s.Reply(info, "🔰 *TIMER CANCELLED* 🔰\n\n🔰 *MEMBER :❮ @"+target.User+" ❯*\n🔰 *TIMER REMOVED FROM BOT MEMORY*")
	return true
}

// ── restart restore ──────────────────────────────────────────────────────────

// TimedAdminRestoreSavedTimers re-arms every pending timed-admin timer from bot
// memory. Called from manager.go on *events.Connected so a pending timer
// survives a restart/reconnect. If the fire time already passed while the bot
// was down, the action is applied immediately (catch-up).
func TimedAdminRestoreSavedTimers(s SessionBridge) {
	if !s.MemoryReady() {
		return
	}
	entries, err := s.MemoryListNS(timedAdminNS)
	if err != nil || len(entries) == 0 {
		return
	}
	for _, e := range entries {
		var cfg timedAdminConfig
		if json.Unmarshal(e.Data, &cfg) != nil {
			continue
		}
		if cfg.Kind != "dismiss" && cfg.Kind != "admin" {
			continue
		}
		key := e.ID
		// already armed? (double Connected event guard)
		timedAdminMu.Lock()
		_, active := timedAdminTimers[key]
		timedAdminMu.Unlock()
		if active {
			continue
		}
		// compute remaining time from the saved fire timestamp
		remaining := time.Until(time.UnixMilli(cfg.FireAtMs))
		if remaining <= 0 {
			// missed while down → apply immediately (catch-up)
			go timedAdminFire(s, key, cfg)
			continue
		}
		armTimedAdminTimer(s, key, cfg, remaining)
	}
}

// ── usage guide (GOLD-MD styled) ─────────────────────────────────────────────

func timedAdminUsage(prefix, kind string) string {
	if kind == "admin" {
		return "🔰 *GOLD-MD ADMIN TIME* 🔰\n\n" +
			"*MAKE A MEMBER ADMIN FOR A SET TIME — THE BOT REMOVES ADMIN POWER WHEN THE TIME IS UP*\n\n" +
			"╭─ 🔰 *FORMAT* ─╮\n" +
			"│ ```" + prefix + "admintime XXhXXmXXs @mention```\n" +
			"╰──────────────╯\n\n" +
			"*EXAMPLES:*\n" +
			"🔰 ```" + prefix + "admintime 01h00m00s @user```\n" +
			"   → *@user becomes admin for 1 hour, then auto-demoted*\n\n" +
			"🔰 ```" + prefix + "admintime 00h30m00s @user```\n" +
			"   → *@user becomes admin for 30 minutes*\n\n" +
			"*TO CANCEL:*\n" +
			"🔰 ```" + prefix + "admintime stop @user```\n\n" +
			"*TIME FORMAT: XXhXXmXXs (hours minutes seconds)*\n" +
			"*TIMER RUNS IN BACKGROUND — 0% SPEED & RAM IMPACT*\n" +
			"*THE BOT SAVES YOUR TIMER IN ITS OWN MEMORY 🔰*"
	}
	return "🔰 *GOLD-MD DISMISS TIME* 🔰\n\n" +
		"*REMOVE ADMIN POWER FROM A MEMBER AFTER A SET TIME*\n\n" +
		"╭─ 🔰 *FORMAT* ─╮\n" +
		"│ ```" + prefix + "dissmisstime XXhXXmXXs @mention```\n" +
		"╰──────────────╯\n\n" +
		"*EXAMPLES:*\n" +
		"🔰 ```" + prefix + "dissmisstime 00h05m00s @user```\n" +
		"   → *@user is dismissed (admin removed) after 5 minutes*\n\n" +
		"🔰 ```" + prefix + "dissmisstime 02h00m00s @user```\n" +
		"   → *@user is dismissed after 2 hours*\n\n" +
		"*TO CANCEL:*\n" +
		"🔰 ```" + prefix + "dissmisstime stop @user```\n\n" +
		"*TIME FORMAT: XXhXXmXXs (hours minutes seconds)*\n" +
		"*TIMER RUNS IN BACKGROUND — 0% SPEED & RAM IMPACT*\n" +
		"*THE BOT SAVES YOUR TIMER IN ITS OWN MEMORY 🔰*"
}
