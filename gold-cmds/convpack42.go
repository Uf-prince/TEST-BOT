package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 42 (10 temperature, angle & time converters)
// File: convpack42.go
// ============================================================================
//   .fahrenheittokelvin -> fahrenheit to kelvin
//   .kelvintofahrenheit -> kelvin to fahrenheit
//   .degreestoradians   -> degrees to radians
//   .radianstodegrees   -> radians to degrees
//   .weekstodays        -> weeks to days
//   .monthstoweeks      -> months to weeks
//   .yearstomonths      -> years to months
//   .decadestoyears     -> decades to years
//   .hourstominutes     -> hours to minutes
//   .daystohours        -> days to hours
// ============================================================================

func init() {
	Register(Command{Name: "fahrenheittokelvin", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FAHRENHEIT TO KELVIN. USE IT AS .FAHRENHEITTOKELVIN <VALUE>.", Run: makeConvHandler("FAHRENHEIT TO KELVIN", "fahrenheittokelvin", "F", "K", func(v float64) float64 { return (v-32)*5/9 + 273.15 })})
	Register(Command{Name: "kelvintofahrenheit", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KELVIN TO FAHRENHEIT. USE IT AS .KELVINTOFAHRENHEIT <VALUE>.", Run: makeConvHandler("KELVIN TO FAHRENHEIT", "kelvintofahrenheit", "K", "F", func(v float64) float64 { return (v-273.15)*9/5 + 32 })})
	Register(Command{Name: "degreestoradians", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT DEGREES TO RADIANS. USE IT AS .DEGREESTORADIANS <VALUE>.", Run: makeConvHandler("DEGREES TO RADIANS", "degreestoradians", "DEG", "RAD", func(v float64) float64 { return v * 0.0174532925 })})
	Register(Command{Name: "radianstodegrees", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT RADIANS TO DEGREES. USE IT AS .RADIANSTODEGREES <VALUE>.", Run: makeConvHandler("RADIANS TO DEGREES", "radianstodegrees", "RAD", "DEG", func(v float64) float64 { return v * 57.2957795 })})
	Register(Command{Name: "weekstodays", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT WEEKS TO DAYS. USE IT AS .WEEKSTODAYS <VALUE>.", Run: makeConvHandler("WEEKS TO DAYS", "weekstodays", "WK", "D", func(v float64) float64 { return v * 7 })})
	Register(Command{Name: "monthstoweeks", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MONTHS TO WEEKS. USE IT AS .MONTHSTOWEEKS <VALUE>.", Run: makeConvHandler("MONTHS TO WEEKS", "monthstoweeks", "MO", "WK", func(v float64) float64 { return v * 4.34524 })})
	Register(Command{Name: "yearstomonths", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT YEARS TO MONTHS. USE IT AS .YEARSTOMONTHS <VALUE>.", Run: makeConvHandler("YEARS TO MONTHS", "yearstomonths", "YR", "MO", func(v float64) float64 { return v * 12 })})
	Register(Command{Name: "decadestoyears", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT DECADES TO YEARS. USE IT AS .DECADESTOYEARS <VALUE>.", Run: makeConvHandler("DECADES TO YEARS", "decadestoyears", "DEC", "YR", func(v float64) float64 { return v * 10 })})
	Register(Command{Name: "hourstominutes", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HOURS TO MINUTES. USE IT AS .HOURSTOMINUTES <VALUE>.", Run: makeConvHandler("HOURS TO MINUTES", "hourstominutes", "HR", "MIN", func(v float64) float64 { return v * 60 })})
	Register(Command{Name: "daystohours", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT DAYS TO HOURS. USE IT AS .DAYSTOHOURS <VALUE>.", Run: makeConvHandler("DAYS TO HOURS", "daystohours", "D", "HR", func(v float64) float64 { return v * 24 })})

	// hidden aliases
	Register(Command{Name: "ftokelvin", Category: "CONVERTER", Desc: "Short alias of .fahrenheittokelvin", Hidden: true, Run: makeConvHandler("FAHRENHEIT TO KELVIN", "fahrenheittokelvin", "F", "K", func(v float64) float64 { return (v-32)*5/9 + 273.15 })})
	Register(Command{Name: "ktofahrenheit", Category: "CONVERTER", Desc: "Short alias of .kelvintofahrenheit", Hidden: true, Run: makeConvHandler("KELVIN TO FAHRENHEIT", "kelvintofahrenheit", "K", "F", func(v float64) float64 { return (v-273.15)*9/5 + 32 })})
	Register(Command{Name: "deg2rad", Category: "CONVERTER", Desc: "Short alias of .degreestoradians", Hidden: true, Run: makeConvHandler("DEGREES TO RADIANS", "degreestoradians", "DEG", "RAD", func(v float64) float64 { return v * 0.0174532925 })})
	Register(Command{Name: "rad2deg", Category: "CONVERTER", Desc: "Short alias of .radianstodegrees", Hidden: true, Run: makeConvHandler("RADIANS TO DEGREES", "radianstodegrees", "RAD", "DEG", func(v float64) float64 { return v * 57.2957795 })})
	Register(Command{Name: "wktod", Category: "CONVERTER", Desc: "Short alias of .weekstodays", Hidden: true, Run: makeConvHandler("WEEKS TO DAYS", "weekstodays", "WK", "D", func(v float64) float64 { return v * 7 })})
	Register(Command{Name: "motowk", Category: "CONVERTER", Desc: "Short alias of .monthstoweeks", Hidden: true, Run: makeConvHandler("MONTHS TO WEEKS", "monthstoweeks", "MO", "WK", func(v float64) float64 { return v * 4.34524 })})
	Register(Command{Name: "yrtomo", Category: "CONVERTER", Desc: "Short alias of .yearstomonths", Hidden: true, Run: makeConvHandler("YEARS TO MONTHS", "yearstomonths", "YR", "MO", func(v float64) float64 { return v * 12 })})
	Register(Command{Name: "dectoyr", Category: "CONVERTER", Desc: "Short alias of .decadestoyears", Hidden: true, Run: makeConvHandler("DECADES TO YEARS", "decadestoyears", "DEC", "YR", func(v float64) float64 { return v * 10 })})
	Register(Command{Name: "hrtomin", Category: "CONVERTER", Desc: "Short alias of .hourstominutes", Hidden: true, Run: makeConvHandler("HOURS TO MINUTES", "hourstominutes", "HR", "MIN", func(v float64) float64 { return v * 60 })})
	Register(Command{Name: "dtohr", Category: "CONVERTER", Desc: "Short alias of .daystohours", Hidden: true, Run: makeConvHandler("DAYS TO HOURS", "daystohours", "D", "HR", func(v float64) float64 { return v * 24 })})
}
