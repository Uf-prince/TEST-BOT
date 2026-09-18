package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 18 (10 new everyday commands)
// File: toolpack18.go
// ============================================================================
//   .bmr <male|female> <kg> <cm> <age>       -> basal metabolic rate
//   .caloriecalc <male|female> <kg> <cm> <age> <act 1-5> -> daily calories
//   .bodyfat <male|female> <cm> <neck> <waist> [hip] -> body fat %
//   .idealweight <male|female> <cm>          -> ideal body weight
//   .waterintake <kg> [exercise_min]         -> daily water target
//   .mortgage <amount> <rate%> <years>       -> monthly mortgage payment
//   .loancalc <amount> <rate%> <months>      -> loan EMI
//   .taxcalc <amount> <rate%>                -> tax and net amount
//   .tipcalc <bill> <tip%> [people]          -> tip and split
//   .discountcalc <price> <discount%>        -> final price and savings
//
// All run locally (no API) and match the GOLD-MD design language exactly
// (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"math"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .BMR ────────────────────────────────────────────────────────────────────

func bmrGuide(prefix string) string {
	return "*🔰 BMR CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR BASAL METABOLIC RATE (CALORIES AT REST)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BMR <MALE|FEMALE> <KG> <CM> <AGE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BMR MALE 70 175 25 ❯*"
}

func handleBmr(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 4 {
			s.Reply(info, bmrGuide(prefix))
			return
		}
		sex := strings.ToLower(strings.TrimSpace(args[0]))
		kg, e1 := strconv.ParseFloat(args[1], 64)
		cm, e2 := strconv.ParseFloat(args[2], 64)
		age, e3 := strconv.ParseFloat(args[3], 64)
		if e1 != nil || e2 != nil || e3 != nil || (sex != "male" && sex != "female") {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"BMR <MALE|FEMALE> <KG> <CM> <AGE>*")
			return
		}
		bmr := 10*kg + 6.25*cm - 5*age
		if sex == "male" {
			bmr += 5
		} else {
			bmr -= 161
		}
		var b strings.Builder
		b.WriteString("*🔰 BMR CALCULATOR 🔰*\n\n")
		b.WriteString("*👤 GENDER ❯ " + strings.ToUpper(sex) + "*\n")
		b.WriteString("*⚖️ WEIGHT ❯ " + strconv.FormatFloat(kg, 'f', 1, 64) + " KG*\n")
		b.WriteString("*📏 HEIGHT ❯ " + strconv.FormatFloat(cm, 'f', 1, 64) + " CM*\n")
		b.WriteString("*🎂 AGE ❯ " + strconv.FormatFloat(age, 'f', 0, 64) + " YEARS*\n\n")
		b.WriteString("*🔥 BMR ❯ " + strconv.FormatFloat(bmr, 'f', 0, 64) + " KCAL/DAY*\n")
		b.WriteString("*🛌 SEDENTARY ❯ " + strconv.FormatFloat(bmr*1.2, 'f', 0, 64) + " KCAL*\n")
		b.WriteString("*🚶 LIGHT ❯ " + strconv.FormatFloat(bmr*1.375, 'f', 0, 64) + " KCAL*\n")
		b.WriteString("*🏃 ACTIVE ❯ " + strconv.FormatFloat(bmr*1.55, 'f', 0, 64) + " KCAL*")
		s.Reply(info, b.String())
	})
}

// ── .CALORIECALC ────────────────────────────────────────────────────────────

func caloriecalcGuide(prefix string) string {
	return "*🔰 CALORIE CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR DAILY CALORIE NEEDS (TDEE)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CALORIECALC <MALE|FEMALE> <KG> <CM> <AGE> <ACTIVITY 1-5> ❯*\n" +
		"*ACTIVITY ❯ 1 SEDENTARY, 2 LIGHT, 3 MODERATE, 4 ACTIVE, 5 VERY ACTIVE*\n" +
		"*EXAMPLE ❮ " + prefix + "CALORIECALC MALE 70 175 25 3 ❯*"
}

func handleCaloriecalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 5 {
			s.Reply(info, caloriecalcGuide(prefix))
			return
		}
		sex := strings.ToLower(strings.TrimSpace(args[0]))
		kg, e1 := strconv.ParseFloat(args[1], 64)
		cm, e2 := strconv.ParseFloat(args[2], 64)
		age, e3 := strconv.ParseFloat(args[3], 64)
		act, e4 := strconv.Atoi(args[4])
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || (sex != "male" && sex != "female") || act < 1 || act > 5 {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"CALORIECALC <MALE|FEMALE> <KG> <CM> <AGE> <1-5>*")
			return
		}
		bmr := 10*kg + 6.25*cm - 5*age
		if sex == "male" {
			bmr += 5
		} else {
			bmr -= 161
		}
		mults := []float64{1.2, 1.375, 1.55, 1.725, 1.9}
		labels := []string{"SEDENTARY", "LIGHT", "MODERATE", "ACTIVE", "VERY ACTIVE"}
		tdee := bmr * mults[act-1]
		var b strings.Builder
		b.WriteString("*🔰 CALORIE CALCULATOR 🔰*\n\n")
		b.WriteString("*👤 GENDER ❯ " + strings.ToUpper(sex) + "*\n")
		b.WriteString("*🏃 ACTIVITY ❯ " + labels[act-1] + "*\n\n")
		b.WriteString("*🔥 MAINTENANCE ❯ " + strconv.FormatFloat(tdee, 'f', 0, 64) + " KCAL*\n")
		b.WriteString("*📉 WEIGHT LOSS ❯ " + strconv.FormatFloat(tdee-500, 'f', 0, 64) + " KCAL*\n")
		b.WriteString("*📈 WEIGHT GAIN ❯ " + strconv.FormatFloat(tdee+500, 'f', 0, 64) + " KCAL*")
		s.Reply(info, b.String())
	})
}

// ── .BODYFAT ────────────────────────────────────────────────────────────────

func bodyfatGuide(prefix string) string {
	return "*🔰 BODY FAT CALCULATOR 🔰*\n\n" +
		"*ESTIMATE BODY FAT % USING THE US NAVY METHOD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "BODYFAT <MALE|FEMALE> <HEIGHT_CM> <NECK_CM> <WAIST_CM> [HIP_CM] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "BODYFAT MALE 175 38 85 ❯*"
}

func handleBodyfat(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 4 {
			s.Reply(info, bodyfatGuide(prefix))
			return
		}
		sex := strings.ToLower(strings.TrimSpace(args[0]))
		height, e1 := strconv.ParseFloat(args[1], 64)
		neck, e2 := strconv.ParseFloat(args[2], 64)
		waist, e3 := strconv.ParseFloat(args[3], 64)
		if e1 != nil || e2 != nil || e3 != nil || (sex != "male" && sex != "female") {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"BODYFAT <MALE|FEMALE> <HEIGHT> <NECK> <WAIST> [HIP]*")
			return
		}
		var bf float64
		if sex == "male" {
			if waist-neck <= 0 {
				s.Reply(info, "*🔰 WAIST MUST BE GREATER THAN NECK*")
				return
			}
			bf = 495/(1.0324-0.19077*math.Log10(waist-neck)+0.15456*math.Log10(height)) - 450
		} else {
			if len(args) < 5 {
				s.Reply(info, "*🔰 FEMALE NEEDS HIP MEASUREMENT, USE "+prefix+"BODYFAT FEMALE <HEIGHT> <NECK> <WAIST> <HIP>*")
				return
			}
			hip, e4 := strconv.ParseFloat(args[4], 64)
			if e4 != nil || waist+hip-neck <= 0 {
				s.Reply(info, "*🔰 INVALID HIP MEASUREMENT*")
				return
			}
			bf = 495/(1.29579-0.35004*math.Log10(waist+hip-neck)+0.22100*math.Log10(height)) - 450
		}
		if bf < 0 {
			bf = 0
		}
		cat := "ESSENTIAL"
		switch {
		case sex == "male" && bf < 6:
			cat = "ESSENTIAL"
		case sex == "male" && bf < 14:
			cat = "ATHLETIC"
		case sex == "male" && bf < 18:
			cat = "FIT"
		case sex == "male" && bf < 25:
			cat = "AVERAGE"
		case sex == "male":
			cat = "OBESE"
		case bf < 14:
			cat = "ESSENTIAL"
		case bf < 21:
			cat = "ATHLETIC"
		case bf < 25:
			cat = "FIT"
		case bf < 32:
			cat = "AVERAGE"
		default:
			cat = "OBESE"
		}
		var b strings.Builder
		b.WriteString("*🔰 BODY FAT CALCULATOR 🔰*\n\n")
		b.WriteString("*👤 GENDER ❯ " + strings.ToUpper(sex) + "*\n")
		b.WriteString("*📏 HEIGHT ❯ " + strconv.FormatFloat(height, 'f', 1, 64) + " CM*\n")
		b.WriteString("*📐 NECK ❯ " + strconv.FormatFloat(neck, 'f', 1, 64) + " CM*\n")
		b.WriteString("*📐 WAIST ❯ " + strconv.FormatFloat(waist, 'f', 1, 64) + " CM*\n\n")
		b.WriteString("*🧮 BODY FAT ❯ " + strconv.FormatFloat(bf, 'f', 1, 64) + " %*\n")
		b.WriteString("*📊 CATEGORY ❯ " + cat + "*")
		s.Reply(info, b.String())
	})
}

// ── .IDEALWEIGHT ────────────────────────────────────────────────────────────

func idealweightGuide(prefix string) string {
	return "*🔰 IDEAL WEIGHT 🔰*\n\n" +
		"*FIND YOUR IDEAL BODY WEIGHT (DEVINE FORMULA)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "IDEALWEIGHT <MALE|FEMALE> <HEIGHT_CM> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "IDEALWEIGHT MALE 175 ❯*"
}

func handleIdealweight(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, idealweightGuide(prefix))
			return
		}
		sex := strings.ToLower(strings.TrimSpace(args[0]))
		cm, err := strconv.ParseFloat(args[1], 64)
		if err != nil || (sex != "male" && sex != "female") {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"IDEALWEIGHT <MALE|FEMALE> <HEIGHT_CM>*")
			return
		}
		inches := cm / 2.54
		over := inches - 60
		base := 50.0
		if sex == "female" {
			base = 45.5
		}
		ideal := base + 2.3*over
		low := ideal * 0.9
		high := ideal * 1.1
		var b strings.Builder
		b.WriteString("*🔰 IDEAL WEIGHT 🔰*\n\n")
		b.WriteString("*👤 GENDER ❯ " + strings.ToUpper(sex) + "*\n")
		b.WriteString("*📏 HEIGHT ❯ " + strconv.FormatFloat(cm, 'f', 1, 64) + " CM*\n\n")
		b.WriteString("*⚖️ IDEAL WEIGHT ❯ " + strconv.FormatFloat(ideal, 'f', 1, 64) + " KG*\n")
		b.WriteString("*📊 HEALTHY RANGE ❯ " + strconv.FormatFloat(low, 'f', 1, 64) + " - " + strconv.FormatFloat(high, 'f', 1, 64) + " KG*")
		s.Reply(info, b.String())
	})
}

// ── .WATERINTAKE ────────────────────────────────────────────────────────────

func waterintakeGuide(prefix string) string {
	return "*🔰 WATER INTAKE 🔰*\n\n" +
		"*CALCULATE YOUR DAILY WATER TARGET*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WATERINTAKE <WEIGHT_KG> [EXERCISE_MINUTES] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WATERINTAKE 70 30 ❯*"
}

func handleWaterintake(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 1 {
			s.Reply(info, waterintakeGuide(prefix))
			return
		}
		kg, err := strconv.ParseFloat(args[0], 64)
		if err != nil || kg <= 0 {
			s.Reply(info, "*🔰 INVALID WEIGHT, USE "+prefix+"WATERINTAKE <WEIGHT_KG> [EXERCISE_MIN]*")
			return
		}
		ml := kg * 35
		exMin := 0.0
		if len(args) >= 2 {
			if v, e := strconv.ParseFloat(args[1], 64); e == nil && v > 0 {
				exMin = v
				ml += (v / 30) * 350
			}
		}
		glasses := ml / 250
		var b strings.Builder
		b.WriteString("*🔰 WATER INTAKE 🔰*\n\n")
		b.WriteString("*⚖️ WEIGHT ❯ " + strconv.FormatFloat(kg, 'f', 1, 64) + " KG*\n")
		if exMin > 0 {
			b.WriteString("*🏃 EXERCISE ❯ " + strconv.FormatFloat(exMin, 'f', 0, 64) + " MIN*\n")
		}
		b.WriteString("\n*💧 DAILY WATER ❯ " + strconv.FormatFloat(ml/1000, 'f', 2, 64) + " LITERS*\n")
		b.WriteString("*🥤 IN ML ❯ " + strconv.FormatFloat(ml, 'f', 0, 64) + " ML*\n")
		b.WriteString("*🍶 GLASSES ❯ " + strconv.FormatFloat(glasses, 'f', 0, 64) + " (250ML EACH)*")
		s.Reply(info, b.String())
	})
}

// ── .MORTGAGE ───────────────────────────────────────────────────────────────

func mortgageGuide(prefix string) string {
	return "*🔰 MORTGAGE CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR MONTHLY MORTGAGE PAYMENT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "MORTGAGE <LOAN_AMOUNT> <ANNUAL_RATE%> <YEARS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "MORTGAGE 200000 5.5 30 ❯*"
}

func handleMortgage(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, mortgageGuide(prefix))
			return
		}
		p, e1 := strconv.ParseFloat(args[0], 64)
		rate, e2 := strconv.ParseFloat(args[1], 64)
		years, e3 := strconv.ParseFloat(args[2], 64)
		if e1 != nil || e2 != nil || e3 != nil || p <= 0 || years <= 0 {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"MORTGAGE <AMOUNT> <RATE%> <YEARS>*")
			return
		}
		n := years * 12
		r := rate / 100 / 12
		var monthly float64
		if r == 0 {
			monthly = p / n
		} else {
			monthly = p * r * math.Pow(1+r, n) / (math.Pow(1+r, n) - 1)
		}
		total := monthly * n
		var b strings.Builder
		b.WriteString("*🔰 MORTGAGE CALCULATOR 🔰*\n\n")
		b.WriteString("*🏦 LOAN ❯ " + strconv.FormatFloat(p, 'f', 2, 64) + "*\n")
		b.WriteString("*📈 RATE ❯ " + strconv.FormatFloat(rate, 'f', 2, 64) + " %*\n")
		b.WriteString("*📅 TERM ❯ " + strconv.FormatFloat(years, 'f', 0, 64) + " YEARS*\n\n")
		b.WriteString("*💵 MONTHLY ❯ " + strconv.FormatFloat(monthly, 'f', 2, 64) + "*\n")
		b.WriteString("*💰 TOTAL PAID ❯ " + strconv.FormatFloat(total, 'f', 2, 64) + "*\n")
		b.WriteString("*📊 TOTAL INTEREST ❯ " + strconv.FormatFloat(total-p, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

// ── .LOANCALC ───────────────────────────────────────────────────────────────

func loancalcGuide(prefix string) string {
	return "*🔰 LOAN EMI CALCULATOR 🔰*\n\n" +
		"*CALCULATE YOUR MONTHLY LOAN INSTALLMENT (EMI)*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LOANCALC <AMOUNT> <ANNUAL_RATE%> <MONTHS> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LOANCALC 50000 12 24 ❯*"
}

func handleLoancalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, loancalcGuide(prefix))
			return
		}
		p, e1 := strconv.ParseFloat(args[0], 64)
		rate, e2 := strconv.ParseFloat(args[1], 64)
		months, e3 := strconv.ParseFloat(args[2], 64)
		if e1 != nil || e2 != nil || e3 != nil || p <= 0 || months <= 0 {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"LOANCALC <AMOUNT> <RATE%> <MONTHS>*")
			return
		}
		r := rate / 100 / 12
		var emi float64
		if r == 0 {
			emi = p / months
		} else {
			emi = p * r * math.Pow(1+r, months) / (math.Pow(1+r, months) - 1)
		}
		total := emi * months
		var b strings.Builder
		b.WriteString("*🔰 LOAN EMI CALCULATOR 🔰*\n\n")
		b.WriteString("*💵 LOAN ❯ " + strconv.FormatFloat(p, 'f', 2, 64) + "*\n")
		b.WriteString("*📈 RATE ❯ " + strconv.FormatFloat(rate, 'f', 2, 64) + " %*\n")
		b.WriteString("*📅 MONTHS ❯ " + strconv.FormatFloat(months, 'f', 0, 64) + "*\n\n")
		b.WriteString("*💳 MONTHLY EMI ❯ " + strconv.FormatFloat(emi, 'f', 2, 64) + "*\n")
		b.WriteString("*💰 TOTAL PAYABLE ❯ " + strconv.FormatFloat(total, 'f', 2, 64) + "*\n")
		b.WriteString("*📊 TOTAL INTEREST ❯ " + strconv.FormatFloat(total-p, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

// ── .TAXCALC ────────────────────────────────────────────────────────────────

func taxcalcGuide(prefix string) string {
	return "*🔰 TAX CALCULATOR 🔰*\n\n" +
		"*QUICKLY CALCULATE TAX AND NET AMOUNT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TAXCALC <AMOUNT> <RATE%> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TAXCALC 1000 15 ❯*"
}

func handleTaxcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, taxcalcGuide(prefix))
			return
		}
		amt, e1 := strconv.ParseFloat(args[0], 64)
		rate, e2 := strconv.ParseFloat(args[1], 64)
		if e1 != nil || e2 != nil {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"TAXCALC <AMOUNT> <RATE%>*")
			return
		}
		tax := amt * rate / 100
		var b strings.Builder
		b.WriteString("*🔰 TAX CALCULATOR 🔰*\n\n")
		b.WriteString("*💵 AMOUNT ❯ " + strconv.FormatFloat(amt, 'f', 2, 64) + "*\n")
		b.WriteString("*📈 TAX RATE ❯ " + strconv.FormatFloat(rate, 'f', 2, 64) + " %*\n\n")
		b.WriteString("*🧾 TAX ❯ " + strconv.FormatFloat(tax, 'f', 2, 64) + "*\n")
		b.WriteString("*✅ NET AMOUNT ❯ " + strconv.FormatFloat(amt-tax, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

// ── .TIPCALC ────────────────────────────────────────────────────────────────

func tipcalcGuide(prefix string) string {
	return "*🔰 TIP CALCULATOR 🔰*\n\n" +
		"*CALCULATE TIP AND SPLIT THE BILL*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIPCALC <BILL> <TIP%> [PEOPLE] ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIPCALC 100 15 4 ❯*"
}

func handleTipcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, tipcalcGuide(prefix))
			return
		}
		bill, e1 := strconv.ParseFloat(args[0], 64)
		rate, e2 := strconv.ParseFloat(args[1], 64)
		if e1 != nil || e2 != nil {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"TIPCALC <BILL> <TIP%> [PEOPLE]*")
			return
		}
		people := 1.0
		if len(args) >= 3 {
			if v, e := strconv.ParseFloat(args[2], 64); e == nil && v >= 1 {
				people = v
			}
		}
		tip := bill * rate / 100
		total := bill + tip
		var b strings.Builder
		b.WriteString("*🔰 TIP CALCULATOR 🔰*\n\n")
		b.WriteString("*🧾 BILL ❯ " + strconv.FormatFloat(bill, 'f', 2, 64) + "*\n")
		b.WriteString("*📈 TIP ❯ " + strconv.FormatFloat(rate, 'f', 1, 64) + " %*\n")
		b.WriteString("*👥 PEOPLE ❯ " + strconv.FormatFloat(people, 'f', 0, 64) + "*\n\n")
		b.WriteString("*💵 TIP AMOUNT ❯ " + strconv.FormatFloat(tip, 'f', 2, 64) + "*\n")
		b.WriteString("*💰 TOTAL ❯ " + strconv.FormatFloat(total, 'f', 2, 64) + "*\n")
		b.WriteString("*🙋 PER PERSON ❯ " + strconv.FormatFloat(total/people, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

// ── .DISCOUNTCALC ───────────────────────────────────────────────────────────

func discountcalcGuide(prefix string) string {
	return "*🔰 DISCOUNT CALCULATOR 🔰*\n\n" +
		"*FIND THE FINAL PRICE AFTER A DISCOUNT*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DISCOUNTCALC <PRICE> <DISCOUNT%> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DISCOUNTCALC 200 25 ❯*"
}

func handleDiscountcalc(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, discountcalcGuide(prefix))
			return
		}
		price, e1 := strconv.ParseFloat(args[0], 64)
		disc, e2 := strconv.ParseFloat(args[1], 64)
		if e1 != nil || e2 != nil {
			s.Reply(info, "*🔰 INVALID INPUT, USE "+prefix+"DISCOUNTCALC <PRICE> <DISCOUNT%>*")
			return
		}
		saved := price * disc / 100
		final := price - saved
		var b strings.Builder
		b.WriteString("*🔰 DISCOUNT CALCULATOR 🔰*\n\n")
		b.WriteString("*🏷️ ORIGINAL ❯ " + strconv.FormatFloat(price, 'f', 2, 64) + "*\n")
		b.WriteString("*📉 DISCOUNT ❯ " + strconv.FormatFloat(disc, 'f', 1, 64) + " %*\n\n")
		b.WriteString("*💸 YOU SAVE ❯ " + strconv.FormatFloat(saved, 'f', 2, 64) + "*\n")
		b.WriteString("*✅ FINAL PRICE ❯ " + strconv.FormatFloat(final, 'f', 2, 64) + "*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "bmr", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR BASAL METABOLIC RATE. USE IT AS .BMR <MALE|FEMALE> <KG> <CM> <AGE>.", Run: handleBmr})
	Register(Command{Name: "caloriecalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR DAILY CALORIE NEEDS. USE IT AS .CALORIECALC <MALE|FEMALE> <KG> <CM> <AGE> <1-5>.", Run: handleCaloriecalc})
	Register(Command{Name: "bodyfat", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO ESTIMATE YOUR BODY FAT PERCENTAGE. USE IT AS .BODYFAT <MALE|FEMALE> <HEIGHT> <NECK> <WAIST> [HIP].", Run: handleBodyfat})
	Register(Command{Name: "idealweight", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND YOUR IDEAL BODY WEIGHT. USE IT AS .IDEALWEIGHT <MALE|FEMALE> <HEIGHT_CM>.", Run: handleIdealweight})
	Register(Command{Name: "waterintake", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR DAILY WATER TARGET. USE IT AS .WATERINTAKE <WEIGHT_KG> [EXERCISE_MIN].", Run: handleWaterintake})
	Register(Command{Name: "mortgage", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR MONTHLY MORTGAGE PAYMENT. USE IT AS .MORTGAGE <AMOUNT> <RATE%> <YEARS>.", Run: handleMortgage})
	Register(Command{Name: "loancalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE YOUR LOAN EMI. USE IT AS .LOANCALC <AMOUNT> <RATE%> <MONTHS>.", Run: handleLoancalc})
	Register(Command{Name: "taxcalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE TAX AND NET AMOUNT. USE IT AS .TAXCALC <AMOUNT> <RATE%>.", Run: handleTaxcalc})
	Register(Command{Name: "tipcalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CALCULATE TIP AND SPLIT A BILL. USE IT AS .TIPCALC <BILL> <TIP%> [PEOPLE].", Run: handleTipcalc})
	Register(Command{Name: "discountcalc", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE FINAL PRICE AFTER A DISCOUNT. USE IT AS .DISCOUNTCALC <PRICE> <DISCOUNT%>.", Run: handleDiscountcalc})

	// hidden aliases
	Register(Command{Name: "metabolism", Category: "TOOLS", Desc: "Short alias of .bmr", Hidden: true, Run: handleBmr})
	Register(Command{Name: "tdee", Category: "TOOLS", Desc: "Short alias of .caloriecalc", Hidden: true, Run: handleCaloriecalc})
	Register(Command{Name: "fatpercent", Category: "TOOLS", Desc: "Short alias of .bodyfat", Hidden: true, Run: handleBodyfat})
	Register(Command{Name: "idealwt", Category: "TOOLS", Desc: "Short alias of .idealweight", Hidden: true, Run: handleIdealweight})
	Register(Command{Name: "watergoal", Category: "TOOLS", Desc: "Short alias of .waterintake", Hidden: true, Run: handleWaterintake})
	Register(Command{Name: "homeloan", Category: "TOOLS", Desc: "Short alias of .mortgage", Hidden: true, Run: handleMortgage})
	Register(Command{Name: "loanemi", Category: "TOOLS", Desc: "Short alias of .loancalc", Hidden: true, Run: handleLoancalc})
	Register(Command{Name: "tax", Category: "TOOLS", Desc: "Short alias of .taxcalc", Hidden: true, Run: handleTaxcalc})
	Register(Command{Name: "tipmoney", Category: "TOOLS", Desc: "Short alias of .tipcalc", Hidden: true, Run: handleTipcalc})
	Register(Command{Name: "discount", Category: "TOOLS", Desc: "Short alias of .discountcalc", Hidden: true, Run: handleDiscountcalc})
}
