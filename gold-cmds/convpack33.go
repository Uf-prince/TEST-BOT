package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 33 (10 time converters)
// File: convpack33.go
// ============================================================================
//   .secondstominutes -> seconds to minutes
//   .minutestohours   -> minutes to hours
//   .hourstodays      -> hours to days
//   .daystoweeks      -> days to weeks
//   .weekstomonths    -> weeks to months
//   .monthstoyears    -> months to years
//   .yearstodecades   -> years to decades
//   .millistoseconds  -> milliseconds to seconds
//   .secondstomillis  -> seconds to milliseconds
//   .minutestoseconds -> minutes to seconds
// ============================================================================

func init() {
	Register(Command{Name: "secondstominutes", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SECONDS TO MINUTES. USE IT AS .SECONDSTOMINUTES <VALUE>.", Run: makeConvHandler("SECONDS TO MINUTES", "secondstominutes", "SEC", "MIN", func(v float64) float64 { return v / 60 })})
	Register(Command{Name: "minutestohours", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MINUTES TO HOURS. USE IT AS .MINUTESTOHOURS <VALUE>.", Run: makeConvHandler("MINUTES TO HOURS", "minutestohours", "MIN", "HR", func(v float64) float64 { return v / 60 })})
	Register(Command{Name: "hourstodays", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HOURS TO DAYS. USE IT AS .HOURSTODAYS <VALUE>.", Run: makeConvHandler("HOURS TO DAYS", "hourstodays", "HR", "DAY", func(v float64) float64 { return v / 24 })})
	Register(Command{Name: "daystoweeks", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT DAYS TO WEEKS. USE IT AS .DAYSTOWEEKS <VALUE>.", Run: makeConvHandler("DAYS TO WEEKS", "daystoweeks", "DAY", "WK", func(v float64) float64 { return v / 7 })})
	Register(Command{Name: "weekstomonths", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT WEEKS TO MONTHS. USE IT AS .WEEKSTOMONTHS <VALUE>.", Run: makeConvHandler("WEEKS TO MONTHS", "weekstomonths", "WK", "MO", func(v float64) float64 { return v / 4.345 })})
	Register(Command{Name: "monthstoyears", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MONTHS TO YEARS. USE IT AS .MONTHSTOYEARS <VALUE>.", Run: makeConvHandler("MONTHS TO YEARS", "monthstoyears", "MO", "YR", func(v float64) float64 { return v / 12 })})
	Register(Command{Name: "yearstodecades", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT YEARS TO DECADES. USE IT AS .YEARSTODECADES <VALUE>.", Run: makeConvHandler("YEARS TO DECADES", "yearstodecades", "YR", "DEC", func(v float64) float64 { return v / 10 })})
	Register(Command{Name: "millistoseconds", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLISECONDS TO SECONDS. USE IT AS .MILLISTOSECONDS <VALUE>.", Run: makeConvHandler("MILLISECONDS TO SECONDS", "millistoseconds", "MS", "SEC", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "secondstomillis", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SECONDS TO MILLISECONDS. USE IT AS .SECONDSTOMILLIS <VALUE>.", Run: makeConvHandler("SECONDS TO MILLISECONDS", "secondstomillis", "SEC", "MS", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "minutestoseconds", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MINUTES TO SECONDS. USE IT AS .MINUTESTOSECONDS <VALUE>.", Run: makeConvHandler("MINUTES TO SECONDS", "minutestoseconds", "MIN", "SEC", func(v float64) float64 { return v * 60 })})

	// hidden aliases
	Register(Command{Name: "sectomin", Category: "CONVERTER", Desc: "Short alias of .secondstominutes", Hidden: true, Run: makeConvHandler("SECONDS TO MINUTES", "secondstominutes", "SEC", "MIN", func(v float64) float64 { return v / 60 })})
	Register(Command{Name: "mintohr", Category: "CONVERTER", Desc: "Short alias of .minutestohours", Hidden: true, Run: makeConvHandler("MINUTES TO HOURS", "minutestohours", "MIN", "HR", func(v float64) float64 { return v / 60 })})
	Register(Command{Name: "hrtoday", Category: "CONVERTER", Desc: "Short alias of .hourstodays", Hidden: true, Run: makeConvHandler("HOURS TO DAYS", "hourstodays", "HR", "DAY", func(v float64) float64 { return v / 24 })})
	Register(Command{Name: "daytowk", Category: "CONVERTER", Desc: "Short alias of .daystoweeks", Hidden: true, Run: makeConvHandler("DAYS TO WEEKS", "daystoweeks", "DAY", "WK", func(v float64) float64 { return v / 7 })})
	Register(Command{Name: "wktomo", Category: "CONVERTER", Desc: "Short alias of .weekstomonths", Hidden: true, Run: makeConvHandler("WEEKS TO MONTHS", "weekstomonths", "WK", "MO", func(v float64) float64 { return v / 4.345 })})
	Register(Command{Name: "motoyr", Category: "CONVERTER", Desc: "Short alias of .monthstoyears", Hidden: true, Run: makeConvHandler("MONTHS TO YEARS", "monthstoyears", "MO", "YR", func(v float64) float64 { return v / 12 })})
	Register(Command{Name: "yrtodec", Category: "CONVERTER", Desc: "Short alias of .yearstodecades", Hidden: true, Run: makeConvHandler("YEARS TO DECADES", "yearstodecades", "YR", "DEC", func(v float64) float64 { return v / 10 })})
	Register(Command{Name: "mstosec", Category: "CONVERTER", Desc: "Short alias of .millistoseconds", Hidden: true, Run: makeConvHandler("MILLISECONDS TO SECONDS", "millistoseconds", "MS", "SEC", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "sectoms", Category: "CONVERTER", Desc: "Short alias of .secondstomillis", Hidden: true, Run: makeConvHandler("SECONDS TO MILLISECONDS", "secondstomillis", "SEC", "MS", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "mintosec", Category: "CONVERTER", Desc: "Short alias of .minutestoseconds", Hidden: true, Run: makeConvHandler("MINUTES TO SECONDS", "minutestoseconds", "MIN", "SEC", func(v float64) float64 { return v * 60 })})
}
