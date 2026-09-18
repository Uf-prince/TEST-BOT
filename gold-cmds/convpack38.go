package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 38 (10 energy & power converters)
// File: convpack38.go
// ============================================================================
//   .joulestocalories     -> joules to calories
//   .caloriestojoules     -> calories to joules
//   .watttokilowatt       -> watts to kilowatts
//   .kilowatttowatt       -> kilowatts to watts
//   .wattttohorsepower    -> watts to horsepower
//   .horsepowertowatt     -> horsepower to watts
//   .kilowatthourtojoules -> kilowatt-hours to joules
//   .jouletokilojoule     -> joules to kilojoules
//   .kilojouletocalorie   -> kilojoules to kilocalories
//   .kilocaloriestojoules -> kilocalories to joules
// ============================================================================

func init() {
	Register(Command{Name: "joulestocalories", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT JOULES TO CALORIES. USE IT AS .JOULESTOCALORIES <VALUE>.", Run: makeConvHandler("JOULES TO CALORIES", "joulestocalories", "J", "CAL", func(v float64) float64 { return v * 0.239006 })})
	Register(Command{Name: "caloriestojoules", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CALORIES TO JOULES. USE IT AS .CALORIESTOJOULES <VALUE>.", Run: makeConvHandler("CALORIES TO JOULES", "caloriestojoules", "CAL", "J", func(v float64) float64 { return v * 4.184 })})
	Register(Command{Name: "watttokilowatt", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT WATTS TO KILOWATTS. USE IT AS .WATTTOKILOWATT <VALUE>.", Run: makeConvHandler("WATTS TO KILOWATTS", "watttokilowatt", "W", "KW", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "kilowatttowatt", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOWATTS TO WATTS. USE IT AS .KILOWATTTOWATT <VALUE>.", Run: makeConvHandler("KILOWATTS TO WATTS", "kilowatttowatt", "KW", "W", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "wattttohorsepower", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT WATTS TO HORSEPOWER. USE IT AS .WATTTOHORSEPOWER <VALUE>.", Run: makeConvHandler("WATTS TO HORSEPOWER", "wattttohorsepower", "W", "HP", func(v float64) float64 { return v * 0.00134102 })})
	Register(Command{Name: "horsepowertowatt", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HORSEPOWER TO WATTS. USE IT AS .HORSEPOWERTOWATT <VALUE>.", Run: makeConvHandler("HORSEPOWER TO WATTS", "horsepowertowatt", "HP", "W", func(v float64) float64 { return v * 745.699872 })})
	Register(Command{Name: "kilowatthourtojoules", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOWATT-HOURS TO JOULES. USE IT AS .KILOWATTHOURTOJOULES <VALUE>.", Run: makeConvHandler("KWH TO JOULES", "kilowatthourtojoules", "KWH", "J", func(v float64) float64 { return v * 3.6e6 })})
	Register(Command{Name: "jouletokilojoule", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT JOULES TO KILOJOULES. USE IT AS .JOULETOKILOJOULE <VALUE>.", Run: makeConvHandler("JOULES TO KILOJOULES", "jouletokilojoule", "J", "KJ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "kilojouletocalorie", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOJOULES TO KILOCALORIES. USE IT AS .KILOJOUETOCALORIE <VALUE>.", Run: makeConvHandler("KJ TO KCAL", "kilojouletocalorie", "KJ", "KCAL", func(v float64) float64 { return v * 0.239006 })})
	Register(Command{Name: "kilocaloriestojoules", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOCALORIES TO JOULES. USE IT AS .KILOCALORIESTOJOULES <VALUE>.", Run: makeConvHandler("KCAL TO JOULES", "kilocaloriestojoules", "KCAL", "J", func(v float64) float64 { return v * 4184 })})

	// hidden aliases
	Register(Command{Name: "jtocal", Category: "CONVERTER", Desc: "Short alias of .joulestocalories", Hidden: true, Run: makeConvHandler("JOULES TO CALORIES", "joulestocalories", "J", "CAL", func(v float64) float64 { return v * 0.239006 })})
	Register(Command{Name: "caltoj", Category: "CONVERTER", Desc: "Short alias of .caloriestojoules", Hidden: true, Run: makeConvHandler("CALORIES TO JOULES", "caloriestojoules", "CAL", "J", func(v float64) float64 { return v * 4.184 })})
	Register(Command{Name: "wtokw", Category: "CONVERTER", Desc: "Short alias of .watttokilowatt", Hidden: true, Run: makeConvHandler("WATTS TO KILOWATTS", "watttokilowatt", "W", "KW", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "kwtoW", Category: "CONVERTER", Desc: "Short alias of .kilowatttowatt", Hidden: true, Run: makeConvHandler("KILOWATTS TO WATTS", "kilowatttowatt", "KW", "W", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "wtohp", Category: "CONVERTER", Desc: "Short alias of .wattttohorsepower", Hidden: true, Run: makeConvHandler("WATTS TO HORSEPOWER", "wattttohorsepower", "W", "HP", func(v float64) float64 { return v * 0.00134102 })})
	Register(Command{Name: "hptow", Category: "CONVERTER", Desc: "Short alias of .horsepowertowatt", Hidden: true, Run: makeConvHandler("HORSEPOWER TO WATTS", "horsepowertowatt", "HP", "W", func(v float64) float64 { return v * 745.699872 })})
	Register(Command{Name: "kwhtoj", Category: "CONVERTER", Desc: "Short alias of .kilowatthourtojoules", Hidden: true, Run: makeConvHandler("KWH TO JOULES", "kilowatthourtojoules", "KWH", "J", func(v float64) float64 { return v * 3.6e6 })})
	Register(Command{Name: "jtokj", Category: "CONVERTER", Desc: "Short alias of .jouletokilojoule", Hidden: true, Run: makeConvHandler("JOULES TO KILOJOULES", "jouletokilojoule", "J", "KJ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "kjtokcal", Category: "CONVERTER", Desc: "Short alias of .kilojouletocalorie", Hidden: true, Run: makeConvHandler("KJ TO KCAL", "kilojouletocalorie", "KJ", "KCAL", func(v float64) float64 { return v * 0.239006 })})
	Register(Command{Name: "kcaltokj", Category: "CONVERTER", Desc: "Short alias of .kilocaloriestojoules", Hidden: true, Run: makeConvHandler("KCAL TO JOULES", "kilocaloriestojoules", "KCAL", "J", func(v float64) float64 { return v * 4184 })})
}
