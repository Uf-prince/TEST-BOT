package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 20 (10 new date & time commands)
// File: toolpack20.go
// ============================================================================
//   .dateadd <YYYY-MM-DD> <days>   -> add days to a date
//   .datesub <YYYY-MM-DD> <days>   -> subtract days from a date
//   .dateformat <YYYY-MM-DD>       -> show a date in many formats
//   .weeknumber <YYYY-MM-DD>       -> ISO week number of a date
//   .dayofweek <YYYY-MM-DD>        -> weekday name of a date
//   .countdownto <YYYY-MM-DD>      -> days left until a date
//   .timer <seconds>               -> countdown timer info
//   .stopwatch                     -> current stopwatch / uptime info
//   .pomodoro [work] [break]       -> pomodoro schedule
//   .ageindays <YYYY-MM-DD>        -> age expressed in days/weeks/months
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .DATEADD ────────────────────────────────────────────────────────────────

func dateaddGuide(prefix string) string {
	return "*🔰 DATE ADD 🔰*\n\n" +
		"*ADD DAYS TO ANY DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DATEADD <YYYY-MM-DD> <DAYS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DATEADD 2024-01-01 30 ❯*"
}

func handleDateadd(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, dateaddGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER OF DAYS*")
			return
		}
		res := d.AddDate(0, 0, n)
		var b strings.Builder
		b.WriteString("*🔰 DATE ADD 🔰*\n\n")
		b.WriteString("*📅 START ❯ " + d.Format("02 Jan 2006") + "*\n")
		b.WriteString("*➕ ADDED ❯ " + strconv.Itoa(n) + " DAYS*\n\n")
		b.WriteString("*✅ RESULT ❯ " + res.Format("02 Jan 2006") + "*\n")
		b.WriteString("*📆 DAY ❯ " + res.Weekday().String() + "*")
		s.Reply(info, b.String())
	})
}

// ── .DATESUB ────────────────────────────────────────────────────────────────

func datesubGuide(prefix string) string {
	return "*🔰 DATE SUBTRACT 🔰*\n\n" +
		"*SUBTRACT DAYS FROM ANY DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DATESUB <YYYY-MM-DD> <DAYS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DATESUB 2024-01-31 15 ❯*"
}

func handleDatesub(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, datesubGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER OF DAYS*")
			return
		}
		res := d.AddDate(0, 0, -n)
		var b strings.Builder
		b.WriteString("*🔰 DATE SUBTRACT 🔰*\n\n")
		b.WriteString("*📅 START ❯ " + d.Format("02 Jan 2006") + "*\n")
		b.WriteString("*➖ REMOVED ❯ " + strconv.Itoa(n) + " DAYS*\n\n")
		b.WriteString("*✅ RESULT ❯ " + res.Format("02 Jan 2006") + "*\n")
		b.WriteString("*📆 DAY ❯ " + res.Weekday().String() + "*")
		s.Reply(info, b.String())
	})
}

// ── .DATEFORMAT ─────────────────────────────────────────────────────────────

func dateformatGuide(prefix string) string {
	return "*🔰 DATE FORMAT 🔰*\n\n" +
		"*SHOW ANY DATE IN MANY DIFFERENT FORMATS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DATEFORMAT <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DATEFORMAT 2024-06-15 ❯*"
}

func handleDateformat(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, dateformatGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 DATE FORMAT 🔰*\n\n")
		b.WriteString("*📅 ISO ❯ " + d.Format("2006-01-02") + "*\n")
		b.WriteString("*📅 LONG ❯ " + d.Format("Monday, 02 January 2006") + "*\n")
		b.WriteString("*📅 SHORT ❯ " + d.Format("02 Jan 2006") + "*\n")
		b.WriteString("*📅 US ❯ " + d.Format("01/02/2006") + "*\n")
		b.WriteString("*📅 EU ❯ " + d.Format("02/01/2006") + "*\n")
		b.WriteString("*📅 TIME ❯ " + d.Format("15:04:05") + "*\n")
		b.WriteString("*📅 RFC ❯ " + d.Format(time.RFC1123) + "*")
		s.Reply(info, b.String())
	})
}

// ── .WEEKNUMBER ─────────────────────────────────────────────────────────────

func weeknumberGuide(prefix string) string {
	return "*🔰 WEEK NUMBER 🔰*\n\n" +
		"*FIND THE ISO WEEK NUMBER OF ANY DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WEEKNUMBER <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WEEKNUMBER 2024-06-15 ❯*"
}

func handleWeeknumber(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, weeknumberGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		_, week := d.ISOWeek()
		dayOfYear := d.YearDay()
		var b strings.Builder
		b.WriteString("*🔰 WEEK NUMBER 🔰*\n\n")
		b.WriteString("*📅 DATE ❯ " + d.Format("02 Jan 2006") + "*\n\n")
		b.WriteString("*🗓️ ISO WEEK ❯ " + strconv.Itoa(week) + "*\n")
		b.WriteString("*📆 DAY OF YEAR ❯ " + strconv.Itoa(dayOfYear) + "*\n")
		b.WriteString("*📊 QUARTER ❯ Q" + strconv.Itoa((int(d.Month())-1)/3+1) + "*")
		s.Reply(info, b.String())
	})
}

// ── .DAYOFWEEK ──────────────────────────────────────────────────────────────

func dayofweekGuide(prefix string) string {
	return "*🔰 DAY OF WEEK 🔰*\n\n" +
		"*FIND WHICH WEEKDAY A DATE FALLS ON*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DAYOFWEEK <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DAYOFWEEK 2000-01-01 ❯*"
}

func handleDayofweek(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, dayofweekGuide(prefix))
			return
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		weekend := "WEEKDAY"
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			weekend = "WEEKEND"
		}
		var b strings.Builder
		b.WriteString("*🔰 DAY OF WEEK 🔰*\n\n")
		b.WriteString("*📅 DATE ❯ " + d.Format("02 Jan 2006") + "*\n\n")
		b.WriteString("*📆 DAY ❯ " + d.Weekday().String() + "*\n")
		b.WriteString("*🏷️ TYPE ❯ " + weekend + "*")
		s.Reply(info, b.String())
	})
}

// ── .COUNTDOWNTO ────────────────────────────────────────────────────────────

func countdowntoGuide(prefix string) string {
	return "*🔰 COUNTDOWN 🔰*\n\n" +
		"*COUNT THE DAYS LEFT UNTIL ANY FUTURE DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTDOWNTO <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COUNTDOWNTO 2025-12-31 ❯*"
}

func handleCountdownto(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, countdowntoGuide(prefix))
			return
		}
		target, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		now := time.Now()
		days := int(target.Sub(now).Hours() / 24)
		var b strings.Builder
		b.WriteString("*🔰 COUNTDOWN 🔰*\n\n")
		b.WriteString("*🎯 TARGET ❯ " + target.Format("02 Jan 2006") + "*\n")
		b.WriteString("*📅 TODAY ❯ " + now.Format("02 Jan 2006") + "*\n\n")
		if days > 0 {
			b.WriteString("*⏳ DAYS LEFT ❯ " + strconv.Itoa(days) + "*\n")
			b.WriteString("*📆 WEEKS ❯ " + strconv.Itoa(days/7) + "*\n")
			b.WriteString("*🗓️ MONTHS ❯ " + strconv.Itoa(days/30) + "*")
		} else if days == 0 {
			b.WriteString("*🎉 IT IS TODAY!*")
		} else {
			b.WriteString("*✅ THAT DATE HAS PASSED BY " + strconv.Itoa(-days) + " DAYS*")
		}
		s.Reply(info, b.String())
	})
}

// ── .TIMER ──────────────────────────────────────────────────────────────────

func timerGuide(prefix string) string {
	return "*🔰 TIMER 🔰*\n\n" +
		"*SET A COUNTDOWN TIMER AND SEE WHEN IT ENDS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIMER <SECONDS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIMER 300 ❯*"
}

func handleTimer(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, timerGuide(prefix))
			return
		}
		secs, err := strconv.Atoi(args[0])
		if err != nil || secs < 0 {
			s.Reply(info, "*🔰 INVALID SECONDS, USE "+prefix+"TIMER <SECONDS>*")
			return
		}
		end := time.Now().Add(time.Duration(secs) * time.Second)
		var b strings.Builder
		b.WriteString("*🔰 TIMER 🔰*\n\n")
		b.WriteString("*⏱️ DURATION ❯ " + strconv.Itoa(secs) + " SECONDS*\n")
		b.WriteString("*🕐 MINUTES ❯ " + strconv.Itoa(secs/60) + " MIN " + strconv.Itoa(secs%60) + " SEC*\n")
		b.WriteString("*🏁 ENDS AT ❯ " + end.Format("15:04:05") + "*")
		s.Reply(info, b.String())
	})
}

// ── .STOPWATCH ──────────────────────────────────────────────────────────────

func stopwatchGuide(prefix string) string {
	return "*🔰 STOPWATCH 🔰*\n\n" +
		"*SHOW THE CURRENT TIME AND BOT UPTIME REFERENCE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "STOPWATCH ❯*"
}

func handleStopwatch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		now := time.Now()
		var b strings.Builder
		b.WriteString("*🔰 STOPWATCH 🔰*\n\n")
		b.WriteString("*🕐 CURRENT TIME ❯ " + now.Format("15:04:05") + "*\n")
		b.WriteString("*📅 DATE ❯ " + now.Format("02 Jan 2006") + "*\n")
		b.WriteString("*🌍 TIMEZONE ❯ " + now.Format("MST") + "*\n")
		b.WriteString("*⏱️ UNIX ❯ " + strconv.FormatInt(now.Unix(), 10) + "*")
		s.Reply(info, b.String())
	})
}

// ── .POMODORO ───────────────────────────────────────────────────────────────

func pomodoroGuide(prefix string) string {
	return "*🔰 POMODORO TIMER 🔰*\n\n" +
		"*PLAN YOUR FOCUS SESSIONS WITH THE POMODORO METHOD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "POMODORO [WORK_MIN] [BREAK_MIN] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "POMODORO 25 5 ❯*"
}

func handlePomodoro(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		work := 25
		brk := 5
		if len(args) >= 1 {
			if v, e := strconv.Atoi(args[0]); e == nil && v > 0 {
				work = v
			}
		}
		if len(args) >= 2 {
			if v, e := strconv.Atoi(args[1]); e == nil && v > 0 {
				brk = v
			}
		}
		var b strings.Builder
		b.WriteString("*🔰 POMODORO TIMER 🔰*\n\n")
		b.WriteString("*🍅 WORK ❯ " + strconv.Itoa(work) + " MIN*\n")
		b.WriteString("*☕ BREAK ❯ " + strconv.Itoa(brk) + " MIN*\n\n")
		start := time.Now()
		for i := 1; i <= 4; i++ {
			b.WriteString("*" + strconv.Itoa(i) + ". FOCUS ❯ " + start.Format("15:04") + " - " + start.Add(time.Duration(work)*time.Minute).Format("15:04") + "*\n")
			start = start.Add(time.Duration(work) * time.Minute)
			if i < 4 {
				b.WriteString("*   BREAK ❯ " + start.Format("15:04") + " - " + start.Add(time.Duration(brk)*time.Minute).Format("15:04") + "*\n")
				start = start.Add(time.Duration(brk) * time.Minute)
			}
		}
		b.WriteString("\n*🎯 TOTAL ❯ " + strconv.Itoa(4*work+3*brk) + " MIN*")
		s.Reply(info, b.String())
	})
}

// ── .AGEINDAYS ──────────────────────────────────────────────────────────────

func ageindaysGuide(prefix string) string {
	return "*🔰 AGE IN DAYS 🔰*\n\n" +
		"*EXPRESS YOUR AGE IN DAYS, WEEKS, MONTHS AND MORE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "AGEINDAYS <YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "AGEINDAYS 2000-01-15 ❯*"
}

func handleAgeindays(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, ageindaysGuide(prefix))
			return
		}
		dob, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 INVALID DATE, USE FORMAT YYYY-MM-DD*")
			return
		}
		now := time.Now()
		if dob.After(now) {
			s.Reply(info, "*🔰 BIRTHDATE CANNOT BE IN THE FUTURE*")
			return
		}
		days := int(now.Sub(dob).Hours() / 24)
		var b strings.Builder
		b.WriteString("*🔰 AGE IN DAYS 🔰*\n\n")
		b.WriteString("*🎂 BIRTHDATE ❯ " + dob.Format("02 Jan 2006") + "*\n\n")
		b.WriteString("*📆 DAYS ❯ " + strconv.Itoa(days) + "*\n")
		b.WriteString("*🗓️ WEEKS ❯ " + strconv.Itoa(days/7) + "*\n")
		b.WriteString("*📅 MONTHS ❯ " + strconv.Itoa(days/30) + "*\n")
		b.WriteString("*🎉 YEARS ❯ " + strconv.Itoa(days/365) + "*\n")
		b.WriteString("*⏰ HOURS ❯ " + strconv.Itoa(days*24) + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "dateadd", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ADD DAYS TO ANY DATE. USE IT AS .DATEADD <YYYY-MM-DD> <DAYS>.", Run: handleDateadd})
	Register(Command{Name: "datesub", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SUBTRACT DAYS FROM ANY DATE. USE IT AS .DATESUB <YYYY-MM-DD> <DAYS>.", Run: handleDatesub})
	Register(Command{Name: "dateformat", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHOW ANY DATE IN MANY FORMATS. USE IT AS .DATEFORMAT <YYYY-MM-DD>.", Run: handleDateformat})
	Register(Command{Name: "weeknumber", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE ISO WEEK NUMBER OF A DATE. USE IT AS .WEEKNUMBER <YYYY-MM-DD>.", Run: handleWeeknumber})
	Register(Command{Name: "dayofweek", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND WHICH WEEKDAY A DATE FALLS ON. USE IT AS .DAYOFWEEK <YYYY-MM-DD>.", Run: handleDayofweek})
	Register(Command{Name: "countdownto", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO COUNT THE DAYS LEFT UNTIL A FUTURE DATE. USE IT AS .COUNTDOWNTO <YYYY-MM-DD>.", Run: handleCountdownto})
	Register(Command{Name: "timer", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SET A COUNTDOWN TIMER. USE IT AS .TIMER <SECONDS>.", Run: handleTimer})
	Register(Command{Name: "stopwatch", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHOW THE CURRENT TIME AND DATE. USE IT AS .STOPWATCH.", Run: handleStopwatch})
	Register(Command{Name: "pomodoro", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO PLAN FOCUS SESSIONS WITH THE POMODORO METHOD. USE IT AS .POMODORO [WORK_MIN] [BREAK_MIN].", Run: handlePomodoro})
	Register(Command{Name: "ageindays", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO EXPRESS YOUR AGE IN DAYS, WEEKS AND MONTHS. USE IT AS .AGEINDAYS <YYYY-MM-DD>.", Run: handleAgeindays})

	// hidden aliases
	Register(Command{Name: "adddays", Category: "TOOLS", Desc: "Short alias of .dateadd", Hidden: true, Run: handleDateadd})
	Register(Command{Name: "subdays", Category: "TOOLS", Desc: "Short alias of .datesub", Hidden: true, Run: handleDatesub})
	Register(Command{Name: "fmtdate", Category: "TOOLS", Desc: "Short alias of .dateformat", Hidden: true, Run: handleDateformat})
	Register(Command{Name: "weekno", Category: "TOOLS", Desc: "Short alias of .weeknumber", Hidden: true, Run: handleWeeknumber})
	Register(Command{Name: "weekday", Category: "TOOLS", Desc: "Short alias of .dayofweek", Hidden: true, Run: handleDayofweek})
	Register(Command{Name: "daysleft", Category: "TOOLS", Desc: "Short alias of .countdownto", Hidden: true, Run: handleCountdownto})
	Register(Command{Name: "timerstart", Category: "TOOLS", Desc: "Short alias of .timer", Hidden: true, Run: handleTimer})
	Register(Command{Name: "clocknow", Category: "TOOLS", Desc: "Short alias of .stopwatch", Hidden: true, Run: handleStopwatch})
	Register(Command{Name: "focus", Category: "TOOLS", Desc: "Short alias of .pomodoro", Hidden: true, Run: handlePomodoro})
	Register(Command{Name: "agedays", Category: "TOOLS", Desc: "Short alias of .ageindays", Hidden: true, Run: handleAgeindays})
}
