package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 25 (10 new finance & health calculators)
// File: toolpack25.go
// ============================================================================
//   .compoundinterest <p> <rate> <years> -> compound interest growth
//   .inflation <amount> <rate> <years>   -> future value after inflation
//   .macrocalc <cal> <goal>              -> macro split (protein/carbs/fat)
//   .ovulation <lastperiod>              -> ovulation & fertile window
//   .pacecalc <distance_km> <minutes>    -> running pace
//   .pregnancy <lastperiod>              -> pregnancy due date & week
//   .profitcalc <cost> <sell>            -> profit & margin
//   .salarycalc <monthly>                -> yearly / daily / hourly salary
//   .savingscalc <target> <monthly> <rate> -> time to reach savings goal
//   .sleepcalc <wake_time>               -> best bedtimes for sleep cycles
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// ── .COMPOUNDINTEREST ────────────────────────────────────────────────────────

func compoundinterestGuide(prefix string) string {
	return "*🔰 COMPOUND INTEREST 🔰*\n\n" +
		"*CALCULATE COMPOUND INTEREST GROWTH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COMPOUNDINTEREST <PRINCIPAL> <RATE%> <YEARS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COMPOUNDINTEREST 10000 8 5 ❯*"
}

func handleCompoundinterest(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, compoundinterestGuide(prefix))
			return
		}
		p, e1 := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		r, e2 := strconv.ParseFloat(strings.TrimSpace(args[1]), 64)
		y, e3 := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if e1 != nil || e2 != nil || e3 != nil || p <= 0 || y <= 0 {
			s.Reply(info, "*🔰 COMPOUND INTEREST 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		final := p * math.Pow(1+r/100, y)
		interest := final - p
		var out strings.Builder
		out.WriteString("*🔰 COMPOUND INTEREST 🔰*\n\n")
		out.WriteString("*💰 PRINCIPAL ❯ " + fmtMoney(p) + "*\n")
		out.WriteString("*📈 RATE ❯ " + trimFloat(r) + " % / YEAR*\n")
		out.WriteString("*📅 YEARS ❯ " + trimFloat(y) + "*\n\n")
		out.WriteString("*💵 FINAL AMOUNT ❯ " + fmtMoney(final) + "*\n")
		out.WriteString("*➕ INTEREST EARNED ❯ " + fmtMoney(interest) + "*")
		s.Reply(info, out.String())
	})
}

func fmtMoney(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', 2, 64)
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// ── .INFLATION ───────────────────────────────────────────────────────────────

func inflationGuide(prefix string) string {
	return "*🔰 INFLATION CALCULATOR 🔰*\n\n" +
		"*SEE WHAT MONEY IS WORTH AFTER INFLATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "INFLATION <AMOUNT> <RATE%> <YEARS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "INFLATION 100000 6 10 ❯*"
}

func handleInflation(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, inflationGuide(prefix))
			return
		}
		amt, e1 := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		r, e2 := strconv.ParseFloat(strings.TrimSpace(args[1]), 64)
		y, e3 := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if e1 != nil || e2 != nil || e3 != nil || amt <= 0 || y <= 0 {
			s.Reply(info, "*🔰 INFLATION CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		future := amt * math.Pow(1+r/100, y)
		value := amt / math.Pow(1+r/100, y)
		var out strings.Builder
		out.WriteString("*🔰 INFLATION CALCULATOR 🔰*\n\n")
		out.WriteString("*💰 TODAY ❯ " + fmtMoney(amt) + "*\n")
		out.WriteString("*📈 INFLATION ❯ " + trimFloat(r) + " % / YEAR*\n")
		out.WriteString("*📅 YEARS ❯ " + trimFloat(y) + "*\n\n")
		out.WriteString("*💵 SAME GOODS WILL COST ❯ " + fmtMoney(future) + "*\n")
		out.WriteString("*📉 YOUR MONEY WILL BE WORTH ❯ " + fmtMoney(value) + "*")
		s.Reply(info, out.String())
	})
}

// ── .MACROCALC ───────────────────────────────────────────────────────────────

func macrocalcGuide(prefix string) string {
	return "*🔰 MACRO CALCULATOR 🔰*\n\n" +
		"*SPLIT CALORIES INTO PROTEIN / CARBS / FAT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MACROCALC <CALORIES> <GOAL> ❯*\n" +
		"*GOALS ❯ BALANCED, MUSCLE, FATLOSS*\n" +
		"*EXAMPLE ❮ " + prefix + "MACROCALC 2200 MUSCLE ❯*"
}

func handleMacrocalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, macrocalcGuide(prefix))
			return
		}
		cal, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil || cal <= 0 {
			s.Reply(info, "*🔰 MACRO CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE VALID CALORIES*")
			return
		}
		goal := "BALANCED"
		if len(args) > 1 {
			goal = strings.ToUpper(strings.TrimSpace(args[1]))
		}
		var pPct, cPct, fPct float64
		switch goal {
		case "MUSCLE":
			pPct, cPct, fPct = 35, 40, 25
		case "FATLOSS", "FAT", "LOSS":
			pPct, cPct, fPct = 40, 30, 30
		default:
			goal = "BALANCED"
			pPct, cPct, fPct = 30, 40, 30
		}
		protein := cal * pPct / 100 / 4
		carbs := cal * cPct / 100 / 4
		fat := cal * fPct / 100 / 9
		var out strings.Builder
		out.WriteString("*🔰 MACRO CALCULATOR 🔰*\n\n")
		out.WriteString("*🔥 CALORIES ❯ " + trimFloat(cal) + " KCAL*\n")
		out.WriteString("*🎯 GOAL ❯ " + goal + "*\n\n")
		out.WriteString("*🥩 PROTEIN ❯ " + strconv.Itoa(int(protein+0.5)) + " G (" + trimFloat(pPct) + "%)*\n")
		out.WriteString("*🍚 CARBS ❯ " + strconv.Itoa(int(carbs+0.5)) + " G (" + trimFloat(cPct) + "%)*\n")
		out.WriteString("*🥑 FAT ❯ " + strconv.Itoa(int(fat+0.5)) + " G (" + trimFloat(fPct) + "%)*")
		s.Reply(info, out.String())
	})
}

// ── .OVULATION ───────────────────────────────────────────────────────────────

func ovulationGuide(prefix string) string {
	return "*🔰 OVULATION CALCULATOR 🔰*\n\n" +
		"*ESTIMATE OVULATION & FERTILE WINDOW*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "OVULATION <LAST_PERIOD YYYY-MM-DD> [CYCLE_DAYS] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "OVULATION 2026-09-01 28 ❯*"
}

func handleOvulation(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, ovulationGuide(prefix))
			return
		}
		last, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 OVULATION CALCULATOR 🔰*\n\n*❌ USE DATE FORMAT YYYY-MM-DD*")
			return
		}
		cycle := 28
		if len(args) > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil && n >= 20 && n <= 45 {
				cycle = n
			}
		}
		ovulation := last.AddDate(0, 0, cycle-14)
		fertileStart := ovulation.AddDate(0, 0, -5)
		fertileEnd := ovulation.AddDate(0, 0, 1)
		nextPeriod := last.AddDate(0, 0, cycle)
		var out strings.Builder
		out.WriteString("*🔰 OVULATION CALCULATOR 🔰*\n\n")
		out.WriteString("*📅 LAST PERIOD ❯ " + last.Format("02 Jan 2006") + "*\n")
		out.WriteString("*🔄 CYCLE ❯ " + strconv.Itoa(cycle) + " DAYS*\n\n")
		out.WriteString("*🥚 OVULATION ❯ " + ovulation.Format("02 Jan 2006") + "*\n")
		out.WriteString("*💚 FERTILE WINDOW ❯ " + fertileStart.Format("02 Jan") + " - " + fertileEnd.Format("02 Jan 2006") + "*\n")
		out.WriteString("*📆 NEXT PERIOD ❯ " + nextPeriod.Format("02 Jan 2006") + "*\n\n")
		out.WriteString("*⚠️ ESTIMATE ONLY, NOT MEDICAL ADVICE*")
		s.Reply(info, out.String())
	})
}

// ── .PACECALC ────────────────────────────────────────────────────────────────

func pacecalcGuide(prefix string) string {
	return "*🔰 PACE CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR RUNNING PACE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PACECALC <DISTANCE_KM> <MINUTES> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PACECALC 5 25 ❯*"
}

func handlePacecalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, pacecalcGuide(prefix))
			return
		}
		dist, e1 := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		mins, e2 := strconv.ParseFloat(strings.TrimSpace(args[1]), 64)
		if e1 != nil || e2 != nil || dist <= 0 || mins <= 0 {
			s.Reply(info, "*🔰 PACE CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		paceMin := mins / dist
		speed := dist / (mins / 60.0)
		pm := int(paceMin)
		ps := int((paceMin - float64(pm)) * 60)
		var out strings.Builder
		out.WriteString("*🔰 PACE CALCULATOR 🔰*\n\n")
		out.WriteString("*🏃 DISTANCE ❯ " + trimFloat(dist) + " KM*\n")
		out.WriteString("*⏱️ TIME ❯ " + trimFloat(mins) + " MIN*\n\n")
		out.WriteString("*📏 PACE ❯ " + strconv.Itoa(pm) + " MIN " + strconv.Itoa(ps) + " SEC / KM*\n")
		out.WriteString("*⚡ SPEED ❯ " + trimFloat(math.Round(speed*100)/100) + " KM / H*")
		s.Reply(info, out.String())
	})
}

// ── .PREGNANCY ───────────────────────────────────────────────────────────────

func pregnancyGuide(prefix string) string {
	return "*🔰 PREGNANCY CALCULATOR 🔰*\n\n" +
		"*ESTIMATE DUE DATE & CURRENT WEEK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PREGNANCY <LAST_PERIOD YYYY-MM-DD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PREGNANCY 2026-03-01 ❯*"
}

func handlePregnancy(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, pregnancyGuide(prefix))
			return
		}
		last, err := time.Parse("2006-01-02", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 PREGNANCY CALCULATOR 🔰*\n\n*❌ USE DATE FORMAT YYYY-MM-DD*")
			return
		}
		due := last.AddDate(0, 0, 280)
		days := int(time.Since(last).Hours() / 24)
		week := days / 7
		trimester := 1
		if week >= 13 {
			trimester = 2
		}
		if week >= 28 {
			trimester = 3
		}
		var out strings.Builder
		out.WriteString("*🔰 PREGNANCY CALCULATOR 🔰*\n\n")
		out.WriteString("*📅 LAST PERIOD ❯ " + last.Format("02 Jan 2006") + "*\n\n")
		out.WriteString("*👶 DUE DATE ❯ " + due.Format("02 Jan 2006") + "*\n")
		if days >= 0 && days <= 300 {
			out.WriteString("*📆 CURRENT WEEK ❯ " + strconv.Itoa(week) + "*\n")
			out.WriteString("*🔢 TRIMESTER ❯ " + strconv.Itoa(trimester) + "*\n")
			out.WriteString("*⏳ DAYS LEFT ❯ " + strconv.Itoa(280-days) + "*\n\n")
		} else {
			out.WriteString("\n")
		}
		out.WriteString("*⚠️ ESTIMATE ONLY, NOT MEDICAL ADVICE*")
		s.Reply(info, out.String())
	})
}

// ── .PROFITCALC ──────────────────────────────────────────────────────────────

func profitcalcGuide(prefix string) string {
	return "*🔰 PROFIT CALCULATOR 🔰*\n\n" +
		"*CALCULATE PROFIT & MARGIN*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PROFITCALC <COST> <SELLING_PRICE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PROFITCALC 800 1200 ❯*"
}

func handleProfitcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, profitcalcGuide(prefix))
			return
		}
		cost, e1 := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		sell, e2 := strconv.ParseFloat(strings.TrimSpace(args[1]), 64)
		if e1 != nil || e2 != nil || cost <= 0 {
			s.Reply(info, "*🔰 PROFIT CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		profit := sell - cost
		margin := profit / sell * 100
		markup := profit / cost * 100
		var out strings.Builder
		out.WriteString("*🔰 PROFIT CALCULATOR 🔰*\n\n")
		out.WriteString("*💵 COST ❯ " + fmtMoney(cost) + "*\n")
		out.WriteString("*🏷️ SELLING PRICE ❯ " + fmtMoney(sell) + "*\n\n")
		if profit >= 0 {
			out.WriteString("*✅ PROFIT ❯ " + fmtMoney(profit) + "*\n")
		} else {
			out.WriteString("*❌ LOSS ❯ " + fmtMoney(-profit) + "*\n")
		}
		out.WriteString("*📊 MARGIN ❯ " + trimFloat(math.Round(margin*100)/100) + " %*\n")
		out.WriteString("*📈 MARKUP ❯ " + trimFloat(math.Round(markup*100)/100) + " %*")
		s.Reply(info, out.String())
	})
}

// ── .SALARYCALC ──────────────────────────────────────────────────────────────

func salarycalcGuide(prefix string) string {
	return "*🔰 SALARY CALCULATOR 🔰*\n\n" +
		"*CONVERT MONTHLY SALARY TO YEARLY / DAILY / HOURLY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SALARYCALC <MONTHLY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SALARYCALC 50000 ❯*"
}

func handleSalarycalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, salarycalcGuide(prefix))
			return
		}
		monthly, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil || monthly <= 0 {
			s.Reply(info, "*🔰 SALARY CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE A VALID MONTHLY SALARY*")
			return
		}
		yearly := monthly * 12
		daily := yearly / 260
		hourly := daily / 8
		var out strings.Builder
		out.WriteString("*🔰 SALARY CALCULATOR 🔰*\n\n")
		out.WriteString("*📅 MONTHLY ❯ " + fmtMoney(monthly) + "*\n")
		out.WriteString("*📆 YEARLY ❯ " + fmtMoney(yearly) + "*\n")
		out.WriteString("*🗓️ DAILY (260 DAYS) ❯ " + fmtMoney(daily) + "*\n")
		out.WriteString("*⏰ HOURLY (8 HRS) ❯ " + fmtMoney(hourly) + "*")
		s.Reply(info, out.String())
	})
}

// ── .SAVINGSCALC ─────────────────────────────────────────────────────────────

func savingscalcGuide(prefix string) string {
	return "*🔰 SAVINGS CALCULATOR 🔰*\n\n" +
		"*HOW LONG TO REACH A SAVINGS GOAL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SAVINGSCALC <TARGET> <MONTHLY_SAVE> [RATE%] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SAVINGSCALC 1000000 20000 6 ❯*"
}

func handleSavingscalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, savingscalcGuide(prefix))
			return
		}
		target, e1 := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		monthly, e2 := strconv.ParseFloat(strings.TrimSpace(args[1]), 64)
		if e1 != nil || e2 != nil || target <= 0 || monthly <= 0 {
			s.Reply(info, "*🔰 SAVINGS CALCULATOR 🔰*\n\n*❌ PLEASE PROVIDE VALID NUMBERS*")
			return
		}
		rate := 0.0
		if len(args) > 2 {
			if r, err := strconv.ParseFloat(strings.TrimSpace(args[2]), 64); err == nil {
				rate = r
			}
		}
		months := 0
		balance := 0.0
		monthlyRate := rate / 100 / 12
		for balance < target && months < 1200 {
			balance = balance*(1+monthlyRate) + monthly
			months++
		}
		years := months / 12
		remMonths := months % 12
		var out strings.Builder
		out.WriteString("*🔰 SAVINGS CALCULATOR 🔰*\n\n")
		out.WriteString("*🎯 TARGET ❯ " + fmtMoney(target) + "*\n")
		out.WriteString("*💵 MONTHLY SAVE ❯ " + fmtMoney(monthly) + "*\n")
		out.WriteString("*📈 RATE ❯ " + trimFloat(rate) + " % / YEAR*\n\n")
		out.WriteString("*⏳ TIME NEEDED ❯ " + strconv.Itoa(years) + " YR " + strconv.Itoa(remMonths) + " MO*\n")
		out.WriteString("*📅 TOTAL MONTHS ❯ " + strconv.Itoa(months) + "*")
		s.Reply(info, out.String())
	})
}

// ── .SLEEPCALC ───────────────────────────────────────────────────────────────

func sleepcalcGuide(prefix string) string {
	return "*🔰 SLEEP CALCULATOR 🔰*\n\n" +
		"*FIND THE BEST BEDTIMES FOR SLEEP CYCLES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SLEEPCALC <WAKE_TIME HH:MM> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SLEEPCALC 06:30 ❯*"
}

func handleSleepcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, sleepcalcGuide(prefix))
			return
		}
		wake, err := time.Parse("15:04", strings.TrimSpace(args[0]))
		if err != nil {
			s.Reply(info, "*🔰 SLEEP CALCULATOR 🔰*\n\n*❌ USE TIME FORMAT HH:MM (24H)*")
			return
		}
		var out strings.Builder
		out.WriteString("*🔰 SLEEP CALCULATOR 🔰*\n\n")
		out.WriteString("*⏰ WAKE TIME ❯ " + wake.Format("15:04") + "*\n\n")
		out.WriteString("*😴 BEST BEDTIMES (90 MIN CYCLES + 15 MIN TO FALL ASLEEP):*\n")
		for _, cycles := range []int{6, 5, 4, 3} {
			bed := wake.Add(-time.Duration(cycles*90+15) * time.Minute)
			out.WriteString("*🔹 " + strconv.Itoa(cycles) + " CYCLES ❯ " + bed.Format("15:04") + "*\n")
		}
		out.WriteString("\n*💤 6 CYCLES = 9 HRS, 5 = 7.5 HRS, 4 = 6 HRS*")
		s.Reply(info, out.String())
	})
}

// ── REGISTRATION ─────────────────────────────────────────────────────────────

func init() {
	Register(Command{Name: "compoundinterest", Category: "TOOLS", Desc: "Calculate compound interest growth", Run: handleCompoundinterest})
	Register(Command{Name: "inflation", Category: "TOOLS", Desc: "See what money is worth after inflation", Run: handleInflation})
	Register(Command{Name: "macrocalc", Category: "TOOLS", Desc: "Split calories into protein, carbs and fat", Run: handleMacrocalc})
	Register(Command{Name: "ovulation", Category: "TOOLS", Desc: "Estimate ovulation and fertile window", Run: handleOvulation})
	Register(Command{Name: "pacecalc", Category: "TOOLS", Desc: "Calculate your running pace", Run: handlePacecalc})
	Register(Command{Name: "pregnancy", Category: "TOOLS", Desc: "Estimate pregnancy due date and week", Run: handlePregnancy})
	Register(Command{Name: "profitcalc", Category: "TOOLS", Desc: "Calculate profit and margin", Run: handleProfitcalc})
	Register(Command{Name: "salarycalc", Category: "TOOLS", Desc: "Convert monthly salary to yearly/daily/hourly", Run: handleSalarycalc})
	Register(Command{Name: "savingscalc", Category: "TOOLS", Desc: "How long to reach a savings goal", Run: handleSavingscalc})
	Register(Command{Name: "sleepcalc", Category: "TOOLS", Desc: "Find the best bedtimes for sleep cycles", Run: handleSleepcalc})

	Register(Command{Name: "compound", Category: "TOOLS", Desc: "Short alias of .compoundinterest", Hidden: true, Run: handleCompoundinterest})
	Register(Command{Name: "inflationcalc", Category: "TOOLS", Desc: "Short alias of .inflation", Hidden: true, Run: handleInflation})
	Register(Command{Name: "macro", Category: "TOOLS", Desc: "Short alias of .macrocalc", Hidden: true, Run: handleMacrocalc})
	Register(Command{Name: "ovul", Category: "TOOLS", Desc: "Short alias of .ovulation", Hidden: true, Run: handleOvulation})
	Register(Command{Name: "pace", Category: "TOOLS", Desc: "Short alias of .pacecalc", Hidden: true, Run: handlePacecalc})
	Register(Command{Name: "preg", Category: "TOOLS", Desc: "Short alias of .pregnancy", Hidden: true, Run: handlePregnancy})
	Register(Command{Name: "profit", Category: "TOOLS", Desc: "Short alias of .profitcalc", Hidden: true, Run: handleProfitcalc})
	Register(Command{Name: "salary", Category: "TOOLS", Desc: "Short alias of .salarycalc", Hidden: true, Run: handleSalarycalc})
	Register(Command{Name: "savings", Category: "TOOLS", Desc: "Short alias of .savingscalc", Hidden: true, Run: handleSavingscalc})
	Register(Command{Name: "sleep", Category: "TOOLS", Desc: "Short alias of .sleepcalc", Hidden: true, Run: handleSleepcalc})
}
