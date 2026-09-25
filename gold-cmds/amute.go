package goldcmds

// ============================================================================
// GOLD-MD — .amute / .aunmute (GROUP AUTO-MUTE / AUTO-UNMUTE SCHEDULER)
// File: amute.go
// ============================================================================
// Ported from UMAR-MD pair.js — SAME WORK (0% farak), same texts:
//   .amute              → status/help (AMUTE/AUNMUTE ON-OFF + times + countdown)
//   .amute on           → turn ON auto-mute scheduler
//   .amute off          → turn OFF auto-mute scheduler
//   .amute 7 30 PM      → group will auto-MUTE daily at that time
//   .aunmute on/off     → same for auto-unmute
//   .aunmute 7 30 AM    → group will auto-UNMUTE daily at that time
//
// Scheduler: har 30 second sab enabled groups check hota hain; jis group
// ke bot-number wali country timezone mein wahi hour:minute chal raha ho
// jo schedule kiya gaya hai → SetGroupAnnounce(true/false) + group message.
// lastMutedDate/lastUnmutedDate (YYYY-MM-DD) guard se din mein sirf 1 baar
// trigger hota hai — bot restart hone pe bhi usi din dobara nahi chalega.
//
// Storage: nexstore/amute.json — read → merge → full-write (Node shim fix),
// verify read-back, 5-min hot cache.
//
// Owner-only, group-only. Emoji: 👑 → 🔰 (GOLD-MD branding).
// Timezone: bot-number ki country (curated calling-code map, PK default).
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// ── per-group auto-mute setting (Node: GroupAutoMute schema) ──

type amuteSetting struct {
	JID             string `json:"jid"`
	BotNumber       string `json:"botNumber"`
	MuteEnabled     bool   `json:"muteEnabled"`
	UnmuteEnabled   bool   `json:"unmuteEnabled"`
	MuteHour        *int   `json:"muteHour"`   // 0-23, 24h normalized
	MuteMinute      *int   `json:"muteMinute"` // 0-59
	UnmuteHour      *int   `json:"unmuteHour"`
	UnmuteMinute    *int   `json:"unmuteMinute"`
	LastMutedDate   string `json:"lastMutedDate"` // "YYYY-MM-DD" once-per-day guard
	LastUnmutedDate string `json:"lastUnmutedDate"`
}

// patch field-set flags (explicit OFF support)
type amutePatchFlags struct {
	muteOff   bool
	unmuteOff bool
}

var (
	amuteMu        sync.Mutex
	amuteCache     = map[string]*amuteSetting{} // hot cache (TTL 5 min)
	amuteCacheTS   = map[string]time.Time{}
	amuteSchedOnce sync.Once
)

const (
	amuteDBDir    = "nexstore"
	amuteDBFile   = "amute.json"
	amuteCacheTTL = 5 * time.Minute
)

// ── storage (read → merge → full-write + verify, Node shim-safe upsert) ──

func amuteDBPath() string {
	return filepath.Join(amuteDBDir, amuteDBFile)
}

func amuteLoadAll() map[string]amuteSetting {
	amuteMu.Lock()
	defer amuteMu.Unlock()
	return amuteLoadAllLocked()
}

func amuteLoadAllLocked() map[string]amuteSetting {
	out := map[string]amuteSetting{}
	data, err := os.ReadFile(amuteDBPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// amuteUpsert merges a patch into the current doc, saves the FULL merged doc
// to disk, re-reads to verify, and updates the hot cache. Returns the saved
// doc (verified when possible, else the merged value).
//
// patch: non-zero fields override; flags turn explicit OFF. This is the Go
// mirror of Node's READ → MERGE → FULL-WRITE upsert (mongo-redis-shim fix).
func amuteUpsert(jid string, patch amuteSetting, flags amutePatchFlags) amuteSetting {
	amuteMu.Lock()
	defer amuteMu.Unlock()

	db := amuteLoadAllLocked()
	current := db[jid] // zero value when absent

	merged := current
	merged.JID = jid
	if patch.BotNumber != "" {
		merged.BotNumber = patch.BotNumber
	}
	if patch.MuteEnabled {
		merged.MuteEnabled = true
	} else if flags.muteOff {
		merged.MuteEnabled = false
	}
	if patch.UnmuteEnabled {
		merged.UnmuteEnabled = true
	} else if flags.unmuteOff {
		merged.UnmuteEnabled = false
	}
	if patch.MuteHour != nil {
		merged.MuteHour = patch.MuteHour
	}
	if patch.MuteMinute != nil {
		merged.MuteMinute = patch.MuteMinute
	}
	if patch.UnmuteHour != nil {
		merged.UnmuteHour = patch.UnmuteHour
	}
	if patch.UnmuteMinute != nil {
		merged.UnmuteMinute = patch.UnmuteMinute
	}
	if patch.LastMutedDate != "" {
		merged.LastMutedDate = patch.LastMutedDate
	}
	if patch.LastUnmutedDate != "" {
		merged.LastUnmutedDate = patch.LastUnmutedDate
	}
	db[jid] = merged

	_ = os.MkdirAll(filepath.Dir(amuteDBPath()), 0o755)
	if out, err := json.MarshalIndent(db, "", "  "); err == nil {
		_ = os.WriteFile(amuteDBPath(), out, 0o644)
	}

	// fresh read-back → cache set (Node's verify step)
	var verify amuteSetting
	if vdata, err := os.ReadFile(amuteDBPath()); err == nil {
		tmp := map[string]amuteSetting{}
		if json.Unmarshal(vdata, &tmp) == nil {
			if v, ok := tmp[jid]; ok {
				verify = v
			}
		}
	}
	saved := merged
	if verify.JID != "" || verify.MuteEnabled || verify.UnmuteEnabled ||
		verify.MuteHour != nil || verify.UnmuteHour != nil || verify.BotNumber != "" {
		saved = verify
	}
	c := saved
	amuteCache[jid] = &c
	amuteCacheTS[jid] = time.Now()
	return saved
}

// amuteGet reads the per-group doc (cache first, TTL 5 min like Node).
func amuteGet(jid string) *amuteSetting {
	amuteMu.Lock()
	cached, has := amuteCache[jid]
	ts := amuteCacheTS[jid]
	amuteMu.Unlock()
	if has && time.Since(ts) <= amuteCacheTTL {
		return cached
	}
	db := amuteLoadAll()
	doc, ok := db[jid]
	if !ok {
		return nil
	}
	amuteMu.Lock()
	amuteCache[jid] = &doc
	amuteCacheTS[jid] = time.Now()
	amuteMu.Unlock()
	return &doc
}

func amuteCacheClear(jid string) {
	amuteMu.Lock()
	delete(amuteCache, jid)
	delete(amuteCacheTS, jid)
	amuteMu.Unlock()
}

// ── bot-number country → timezone (Node's curated map, PK default) ──

var amuteTZMap = map[string]string{
	"PK": "Asia/Karachi", "IN": "Asia/Kolkata", "BD": "Asia/Dhaka",
	"US": "America/New_York", "GB": "Europe/London", "SA": "Asia/Riyadh",
	"AE": "Asia/Dubai", "CA": "America/Toronto", "AU": "Australia/Sydney",
	"ID": "Asia/Jakarta", "MY": "Asia/Kuala_Lumpur", "NP": "Asia/Kathmandu",
	"AF": "Asia/Kabul", "TR": "Europe/Istanbul", "EG": "Africa/Cairo",
	"NG": "Africa/Lagos", "ZA": "Africa/Johannesburg", "DE": "Europe/Berlin",
	"FR": "Europe/Paris", "IT": "Europe/Rome", "ES": "Europe/Madrid",
	"BR": "America/Sao_Paulo", "PH": "Asia/Manila", "SG": "Asia/Singapore",
	"QA": "Asia/Qatar", "KW": "Asia/Kuwait", "OM": "Asia/Muscat",
	"BH": "Asia/Bahrain", "IQ": "Asia/Baghdad", "IR": "Asia/Tehran",
	"CN": "Asia/Shanghai", "JP": "Asia/Tokyo", "KR": "Asia/Seoul",
	"RU": "Europe/Moscow", "LK": "Asia/Colombo",
}

// amuteTZForBot maps bot phone digits → country timezone (PK default).
func amuteTZForBot(botDigits string) string {
	cc := amuteCountryForNumber(botDigits)
	if tz, ok := amuteTZMap[cc]; ok {
		return tz
	}
	return "Asia/Karachi"
}

// amuteCountryForNumber maps phone digits → ISO country code via longest
// calling-code prefix match (curated — Node ke libphonenumber equivalent,
// 3-digit codes pehle, phir 2, phir 1).
func amuteCountryForNumber(digits string) string {
	d := digitsOnly(digits)
	if d == "" {
		return ""
	}
	for len(d) > 0 && d[0] == '0' {
		d = d[1:]
	}
	if len(d) < 8 {
		return ""
	}
	type ccPair struct{ cc, code string }
	two := []ccPair{
		{"PK", "92"}, {"IN", "91"}, {"BD", "880"}, {"LK", "94"},
		{"MY", "60"}, {"SG", "65"}, {"JP", "81"}, {"KR", "82"},
		{"VN", "84"}, {"CN", "86"}, {"TR", "90"}, {"NG", "234"},
		{"EG", "20"}, {"ZA", "27"}, {"GR", "30"}, {"NL", "31"},
		{"BE", "32"}, {"FR", "33"}, {"ES", "34"}, {"HU", "36"},
		{"IT", "39"}, {"RO", "40"}, {"CH", "41"}, {"AT", "43"},
		{"GB", "44"}, {"DK", "45"}, {"SE", "46"}, {"NO", "47"},
		{"PL", "48"}, {"DE", "49"}, {"PE", "51"}, {"MX", "52"},
		{"CU", "53"}, {"AR", "54"}, {"BR", "55"}, {"CL", "56"},
		{"CO", "57"}, {"VE", "58"}, {"AU", "61"}, {"NZ", "64"},
		{"PH", "63"}, {"TH", "66"}, {"ID", "62"}, {"IL", "972"},
		{"JO", "962"}, {"IQ", "964"}, {"KW", "965"}, {"QA", "974"},
		{"BH", "973"}, {"OM", "968"}, {"SA", "966"}, {"AE", "971"},
		{"AF", "93"}, {"NP", "977"}, {"MM", "95"}, {"PT", "351"},
		{"IE", "353"}, {"IS", "354"}, {"AL", "355"}, {"MT", "356"},
		{"CY", "357"}, {"FI", "358"}, {"BG", "359"}, {"LT", "370"},
		{"LV", "371"}, {"EE", "372"}, {"MD", "373"}, {"AM", "374"},
		{"BY", "375"}, {"UA", "380"}, {"KZ", "7"}, {"RU", "7"},
		{"TW", "886"}, {"HK", "852"}, {"MO", "853"}, {"KH", "855"},
		{"LA", "856"}, {"UZ", "998"}, {"TM", "993"}, {"TJ", "992"},
		{"AZ", "994"}, {"GE", "995"}, {"IR", "98"}, {"US", "1"},
		{"CA", "1"},
	}
	// longest prefix first (3-digit codes)
	for _, p := range two {
		if len(p.code) == 3 && strings.HasPrefix(d, p.code) {
			return p.cc
		}
	}
	for _, p := range two {
		if len(p.code) == 2 && strings.HasPrefix(d, p.code) {
			return p.cc
		}
	}
	for _, p := range two {
		if len(p.code) == 1 && strings.HasPrefix(d, p.code) {
			return p.cc
		}
	}
	return ""
}

// amuteNowInTZ returns {hour, minute, dateKey} in the given timezone right
// now (Node: UmarGetNowInBotTimezone). tzdata load fail → UTC fallback.
func amuteNowInTZ(tzName string) (int, int, string) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		n := time.Now().UTC()
		return n.Hour(), n.Minute(), n.Format("2006-01-02")
	}
	n := time.Now().In(loc)
	return n.Hour(), n.Minute(), n.Format("2006-01-02")
}

// amuteTimePartsInTZ returns hour/minute/second/dateKey (countdown math).
func amuteTimePartsInTZ(tzName string) (int, int, int, string) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		n := time.Now().UTC()
		return n.Hour(), n.Minute(), n.Second(), n.Format("2006-01-02")
	}
	n := time.Now().In(loc)
	return n.Hour(), n.Minute(), n.Second(), n.Format("2006-01-02")
}

// ── lenient time parsing (Node: UmarParseLenientTime) ──
// "72 8 Pm" jaisa galat input bhi wrap/clamp ho jata hai; AM/PM zaroori.

var (
	amuteNumRe      = regexp.MustCompile(`\d+`)
	amuteMeridiemRe = regexp.MustCompile(`(?i)\b(am|pm)\b`)
)

// amuteParseLenientTime returns (hour24, minute, ok).
func amuteParseLenientTime(argsStr string) (int, int, bool) {
	if argsStr == "" {
		return 0, 0, false
	}
	nums := amuteNumRe.FindAllString(argsStr, -1)
	if len(nums) < 2 {
		return 0, 0, false
	}
	n0, _ := strconv.Atoi(nums[0])
	n1, _ := strconv.Atoi(nums[1])
	hour12 := n0 % 12
	if hour12 == 0 {
		hour12 = 12
	}
	minute := n1 % 60
	m := amuteMeridiemRe.FindString(argsStr)
	if m == "" {
		return 0, 0, false // AM/PM zaroori hai, warna ambiguous
	}
	hour24 := hour12 % 12
	if strings.EqualFold(m, "pm") {
		hour24 += 12
	}
	return hour24, minute, true
}

// ── hour/minute formatting + countdown (Node helpers) ──

func amuteFormatHourMinute(hour24, minute int) string {
	h := hour24 % 12
	if h == 0 {
		h = 12
	}
	ap := "AM"
	if hour24 >= 12 {
		ap = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", h, minute, ap)
}

// amuteMsUntilNextTrigger — agle scheduled occurrence tak milliseconds
// (bot country timezone mein; aaj ka waqt nikal gaya to kal wala).
func amuteMsUntilNextTrigger(targetHour, targetMinute int, botDigits string) int64 {
	tz := amuteTZForBot(botDigits)
	curH, curM, curS, _ := amuteTimePartsInTZ(tz)
	diffSeconds := (targetHour-curH)*3600 + (targetMinute-curM)*60 - curS
	if diffSeconds <= 0 {
		diffSeconds += 24 * 3600
	}
	return int64(diffSeconds) * 1000
}

func amuteFormatCountdown(ms int64) string {
	totalSeconds := ms / 1000
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60
	return fmt.Sprintf("%d HOURS %d MINUTES %d SECONDS", h, m, s)
}

// ── scheduler (Node: _UmarRunGroupAutoMuteScheduler, har 30s) ──

// amuteSchedulerStart lazily starts the 30-second loop (first command use
// pe). Bot startup pe bhi chal sakta hai — main.go se call hota hai.
func amuteSchedulerStart() {
	amuteSchedOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				amuteSchedulerTick()
			}
		}()
	})
}

// amuteSchedulerTick: one pass over all enabled groups.
func amuteSchedulerTick() {
	defer func() { _ = recover() }()

	db := amuteLoadAll()
	for jid, doc := range db {
		if !doc.MuteEnabled && !doc.UnmuteEnabled {
			continue
		}
		cli := amuteClientForGroup(doc.BotNumber)
		if cli == nil {
			continue
		}
		tz := amuteTZForBot(doc.BotNumber)
		hour, minute, dateKey := amuteNowInTZ(tz)

		// ── auto MUTE ──
		if doc.MuteEnabled && doc.MuteHour != nil && doc.MuteMinute != nil {
			if *doc.MuteHour == hour && *doc.MuteMinute == minute && doc.LastMutedDate != dateKey {
				if gj, err := types.ParseJID(jid); err == nil {
					if err := cli.SetGroupAnnounce(context.Background(), gj, true); err == nil {
						amuteUpsert(jid, amuteSetting{LastMutedDate: dateKey}, amutePatchFlags{})
						amuteCacheClear(jid)
						amuteSendGroupMessage(cli, gj, "*AUTO MUTE TIME COMPLETED*\n*GROUP CHAT CLOSED*\n*ONLY ADMINS CAN SEND MESSAGE TO THIS GROUP*")
					}
				}
			}
		}

		// ── auto UNMUTE ──
		if doc.UnmuteEnabled && doc.UnmuteHour != nil && doc.UnmuteMinute != nil {
			if *doc.UnmuteHour == hour && *doc.UnmuteMinute == minute && doc.LastUnmutedDate != dateKey {
				if gj, err := types.ParseJID(jid); err == nil {
					if err := cli.SetGroupAnnounce(context.Background(), gj, false); err == nil {
						amuteUpsert(jid, amuteSetting{LastUnmutedDate: dateKey}, amutePatchFlags{})
						amuteCacheClear(jid)
						amuteSendGroupMessage(cli, gj, "*AUTO UNMUTE TIME COMPLETED*\n*GROUP CHAT OPENED*\n*NOW  EVERYONE CAN SEND MESSAGE TO THIS GROUP*")
					}
				}
			}
		}
	}
}

// amuteClientForGroup finds the LIVE whatsmeow client for a bot number
// (Node's _UmarFindTrackerForBotNumber equivalent — digits-only match over
// all live sessions). Implemented in amute_hook.go (main-package bridge).
func amuteClientForGroup(botNumber string) *whatsmeow.Client {
	digits := digitsOnly(botNumber)
	if digits == "" {
		return nil
	}
	return amuteLookupClientByDigits(digits)
}

// amuteSendGroupMessage sends a plain text group message (direct client
// send, no footer — Node scheduler me bhi plain sendMessage hai).
func amuteSendGroupMessage(cli *whatsmeow.Client, jid types.JID, text string) {
	go func() {
		defer func() { _ = recover() }()
		_, _ = cli.SendMessage(context.Background(), jid, &waProto.Message{
			Conversation: &text,
		})
	}()
}

// ── commands ──

func handleAmute(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAmuteShared(s, info, args, prefix, true)
}

func handleAunmute(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleAmuteShared(s, info, args, prefix, false)
}

// handleAmuteShared — ek hi logic dono commands ke liye (isMuteCmd flag se
// sirf AMUTE/AUNMUTE words badalte hain, baaki sab 0% farak).
func handleAmuteShared(s SessionBridge, info types.MessageInfo, args []string, prefix string, isMuteCmd bool) {
	// ── OWNER CHECK (direct — Node me bhi admin check nahi) ──
	if !s.IsOwner(info) {
		s.Reply(info, "*THIS COMMAND IS ONLY FOR ME 😎*")
		return
	}

	cmdName := "aunmute"
	if isMuteCmd {
		cmdName = "amute"
	}

	// ── SIRF GROUP ──
	if !info.IsGroup {
		s.Reply(info, "*"+strings.ToUpper(cmdName)+" ONLY WORKS IN GROUPS*")
		return
	}

	// ── bot number (scheduler isi timezone se chalega) ──
	cli := s.GetClient()
	if cli == nil || !cli.IsConnected() {
		s.Reply(info, "🔰 WhatsApp client not ready.")
		return
	}
	botDigits := amuteBotDigits(cli)

	amuteSchedulerStart() // first use pe scheduler launch

	existing := amuteGet(info.Chat.String())

	// ── No args: status/help ──
	if len(args) == 0 {
		muteOn, unmuteOn := "OFF", "OFF"
		muteTimeSet, unmuteTimeSet := false, false
		if existing != nil {
			if existing.MuteEnabled {
				muteOn = "ON"
			}
			if existing.UnmuteEnabled {
				unmuteOn = "ON"
			}
			muteTimeSet = existing.MuteHour != nil
			unmuteTimeSet = existing.UnmuteHour != nil
		}
		muteTimeStr, unmuteTimeStr := "NOT SET", "NOT SET"
		muteCountdownLine, unmuteCountdownLine := "", ""
		if existing != nil && muteTimeSet && existing.MuteMinute != nil {
			muteTimeStr = amuteFormatHourMinute(*existing.MuteHour, *existing.MuteMinute)
			if existing.MuteEnabled {
				ms := amuteMsUntilNextTrigger(*existing.MuteHour, *existing.MuteMinute, botDigits)
				muteCountdownLine = "\n*TIME LEFT FOR AMUTE: " + amuteFormatCountdown(ms) + "*"
			}
		}
		if existing != nil && unmuteTimeSet && existing.UnmuteMinute != nil {
			unmuteTimeStr = amuteFormatHourMinute(*existing.UnmuteHour, *existing.UnmuteMinute)
			if existing.UnmuteEnabled {
				ms := amuteMsUntilNextTrigger(*existing.UnmuteHour, *existing.UnmuteMinute, botDigits)
				unmuteCountdownLine = "\n*TIME LEFT FOR AUNMUTE: " + amuteFormatCountdown(ms) + "*"
			}
		}
		s.Reply(info,
			"*🔰 GROUP AUTO MUTE / UNMUTE INFO 🔰*\n\n"+
				"*TYPE ❲ "+prefix+"AMUTE ON ❳* — turn ON auto-mute scheduler\n"+
				"*TYPE ❲ "+prefix+"AMUTE OFF ❳* — turn OFF auto-mute scheduler\n"+
				"*TYPE ❲ "+prefix+"AMUTE 7 30 PM ❳* — group will auto-MUTE daily at that time\n\n"+
				"*TYPE ❲ "+prefix+"AUNMUTE ON ❳* — turn ON auto-unmute scheduler\n"+
				"*TYPE ❲ "+prefix+"AUNMUTE OFF ❳* — turn OFF auto-unmute scheduler\n"+
				"*TYPE ❲ "+prefix+"AUNMUTE 7 30 AM ❳* — group will auto-UNMUTE daily at that time\n\n"+
				"*AMUTE IS ❲ "+muteOn+" ❳ — TIME: "+muteTimeStr+"*"+muteCountdownLine+"\n"+
				"*AUNMUTE IS ❲ "+unmuteOn+" ❳ — TIME: "+unmuteTimeStr+"*"+unmuteCountdownLine)
		return
	}

	firstArg := strings.ToLower(args[0])

	// ── ON ──
	if firstArg == "on" {
		patch := amuteSetting{BotNumber: botDigits}
		if isMuteCmd {
			patch.MuteEnabled = true
		} else {
			patch.UnmuteEnabled = true
		}
		afterOn := amuteUpsert(info.Chat.String(), patch, amutePatchFlags{})
		nowEnabled := (isMuteCmd && afterOn.MuteEnabled) || (!isMuteCmd && afterOn.UnmuteEnabled)
		if !nowEnabled { // ek baar retry (Node jaisa)
			afterOn = amuteUpsert(info.Chat.String(), patch, amutePatchFlags{})
			nowEnabled = (isMuteCmd && afterOn.MuteEnabled) || (!isMuteCmd && afterOn.UnmuteEnabled)
		}
		if !nowEnabled {
			s.Reply(info, "*🔰 "+strings.ToUpper(cmdName)+" ON FAILED TO SAVE*\n*DATABASE STILL SHOWS OFF. Please try again.*")
			return
		}
		// pehle se time set tha to countdown turant dikha do
		existingHour, existingMinute := -1, -1
		if isMuteCmd && afterOn.MuteHour != nil && afterOn.MuteMinute != nil {
			existingHour, existingMinute = *afterOn.MuteHour, *afterOn.MuteMinute
		} else if !isMuteCmd && afterOn.UnmuteHour != nil && afterOn.UnmuteMinute != nil {
			existingHour, existingMinute = *afterOn.UnmuteHour, *afterOn.UnmuteMinute
		}
		countdownLine := ""
		if existingHour >= 0 {
			ms := amuteMsUntilNextTrigger(existingHour, existingMinute, digitString(botDigits))
			countdownLine = "\n*TIME LEFT: " + amuteFormatCountdown(ms) + "*"
		}
		if isMuteCmd {
			s.Reply(info, "*AMUTE ACTIVATED*\n*NOW SET THE TIME: AMUTE 7 30 PM*"+countdownLine)
		} else {
			s.Reply(info, "*AUNMUTE ACTIVATED*\n*NOW SET THE TIME: AUNMUTE 7 30 AM*"+countdownLine)
		}
		return
	}

	// ── OFF ──
	if firstArg == "off" {
		flags := amutePatchFlags{}
		if isMuteCmd {
			flags.muteOff = true
		} else {
			flags.unmuteOff = true
		}
		amuteUpsert(info.Chat.String(), amuteSetting{}, flags)
		amuteCacheClear(info.Chat.String())
		if isMuteCmd {
			s.Reply(info, "*AMUTE DE-ACTIVATED*")
		} else {
			s.Reply(info, "*AUNMUTE DE-ACTIVATED*")
		}
		return
	}

	// ── time set (pehle feature ON check) ──
	featureOn := false
	if existing != nil {
		if isMuteCmd {
			featureOn = existing.MuteEnabled
		} else {
			featureOn = existing.UnmuteEnabled
		}
	}
	if !featureOn {
		if isMuteCmd {
			s.Reply(info, "*AMUTE IS CURRENTLY OFF*\n*FIRST TURN ❲ AMUTE ON ❳, THEN SET THE TIME*")
		} else {
			s.Reply(info, "*AUNMUTE IS CURRENTLY OFF*\n*FIRST TURN ❲ AUNMUTE ON ❳, THEN SET THE TIME*")
		}
		return
	}

	hour, minute, ok := amuteParseLenientTime(strings.Join(args, " "))
	if !ok {
		s.Reply(info, "*WRONG FORMAT*\n*EXAMPLE: "+strings.ToUpper(cmdName)+" 7 30 PM*")
		return
	}
	hp, mp := hour, minute
	patch := amuteSetting{BotNumber: botDigits}
	if isMuteCmd {
		patch.MuteHour, patch.MuteMinute = &hp, &mp
		patch.LastMutedDate = ""
	} else {
		patch.UnmuteHour, patch.UnmuteMinute = &hp, &mp
		patch.LastUnmutedDate = ""
	}
	afterTimeSet := amuteUpsert(info.Chat.String(), patch, amutePatchFlags{})

	// ── honest-write check (Node jaisa — DB me waqai wahi save hua?) ──
	readSaved := func(doc amuteSetting) (int, int) {
		if isMuteCmd && doc.MuteHour != nil && doc.MuteMinute != nil {
			return *doc.MuteHour, *doc.MuteMinute
		}
		if !isMuteCmd && doc.UnmuteHour != nil && doc.UnmuteMinute != nil {
			return *doc.UnmuteHour, *doc.UnmuteMinute
		}
		return -1, -1
	}
	savedHour, savedMinute := readSaved(afterTimeSet)
	if savedHour != hour || savedMinute != minute {
		afterTimeSet = amuteUpsert(info.Chat.String(), patch, amutePatchFlags{})
		savedHour, savedMinute = readSaved(afterTimeSet)
	}
	if savedHour != hour || savedMinute != minute {
		savedStr := "NOT SET"
		if savedHour >= 0 {
			savedStr = amuteFormatHourMinute(savedHour, savedMinute)
		}
		s.Reply(info, "*🔰 TIME FAILED TO SAVE*\n*YOU TRIED TO SET "+amuteFormatHourMinute(hour, minute)+", BUT THE DATABASE STILL SHOWS "+savedStr+".*\n*This is a storage/DB bug, please try again.*")
		return
	}

	countdownMs := amuteMsUntilNextTrigger(savedHour, savedMinute, botDigits)
	countdownText := amuteFormatCountdown(countdownMs)
	if isMuteCmd {
		s.Reply(info, "*AMUTE TIME SET ❲ "+amuteFormatHourMinute(savedHour, savedMinute)+" ❳*\n*GROUP WILL AUTO-MUTE DAILY AT THIS TIME*\n*TIME LEFT: "+countdownText+"*")
	} else {
		s.Reply(info, "*AUNMUTE TIME SET ❲ "+amuteFormatHourMinute(savedHour, savedMinute)+" ❳*\n*GROUP WILL AUTO-UNMUTE DAILY AT THIS TIME*\n*TIME LEFT: "+countdownText+"*")
	}
}

// ── shared helpers ──

// amuteBotDigits extracts the bot's own number from its client store.
func amuteBotDigits(cli *whatsmeow.Client) string {
	if cli == nil || cli.Store == nil || cli.Store.ID == nil {
		return ""
	}
	return digitsOnly(cli.Store.ID.User)
}

// digitString — digits-only passthrough (keeps helper self-contained).
func digitString(s string) string {
	return digitsOnly(s)
}

// amuteLookupClientByDigits is implemented in amute_hook.go — it bridges
// from goldcmds to the main package's live session registry (Manager).
var amuteLookupClientByDigits = func(digits string) *whatsmeow.Client {
	return nil // default no-op; replaced by the main package at startup
}

// ── registration ──

func init() {
	Register(Command{Name: "amute", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO AUTO MUTE THE GROUP DAILY AT A SET TIME. USE IT WITH A TIME LIKE 7 30 PM.", OwnerOnly: true, Run: handleAmute})
	Register(Command{Name: "aunmute", Category: "GROUP MANAGEMENT", Desc: "THIS COMMAND IS USED TO AUTO UNMUTE THE GROUP DAILY AT A SET TIME. USE IT WITH A TIME LIKE 7 30 AM.", OwnerOnly: true, Run: handleAunmute})
}
