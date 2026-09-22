package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 41 (10 cooking volume & temperature converters)
// File: convpack41.go
// ============================================================================
//   .gramstoml      -> grams to millilitres (water)
//   .mltograms      -> millilitres to grams (water)
//   .oztoml         -> fluid ounces to millilitres
//   .mltofluidoz    -> millilitres to fluid ounces
//   .tbspoml        -> tablespoons to millilitres
//   .mltotbsp       -> millilitres to tablespoons
//   .tspoml         -> teaspoons to millilitres
//   .mltotsp        -> millilitres to teaspoons
//   .celsiusrankine -> celsius to rankine
//   .rankinetocelsius -> rankine to celsius
// ============================================================================

func init() {
	Register(Command{Name: "gramstoml", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT GRAMS TO MILLILITRES (WATER). USE IT AS .GRAMSTOML <VALUE>.", Run: makeConvHandler("GRAMS TO ML", "gramstoml", "G", "ML", func(v float64) float64 { return v })})
	Register(Command{Name: "mltograms", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLILITRES TO GRAMS (WATER). USE IT AS .MLTOGRAMS <VALUE>.", Run: makeConvHandler("ML TO GRAMS", "mltograms", "ML", "G", func(v float64) float64 { return v })})
	Register(Command{Name: "oztoml", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FLUID OUNCES TO MILLILITRES. USE IT AS .OZTOML <VALUE>.", Run: makeConvHandler("FL OZ TO ML", "oztoml", "FL OZ", "ML", func(v float64) float64 { return v * 29.57353 })})
	Register(Command{Name: "mltofluidoz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLILITRES TO FLUID OUNCES. USE IT AS .MLTOFLUIDOZ <VALUE>.", Run: makeConvHandler("ML TO FL OZ", "mltofluidoz", "ML", "FL OZ", func(v float64) float64 { return v * 0.033814 })})
	Register(Command{Name: "tbspoml", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TABLESPOONS TO MILLILITRES. USE IT AS .TBSPOML <VALUE>.", Run: makeConvHandler("TBSP TO ML", "tbspoml", "TBSP", "ML", func(v float64) float64 { return v * 14.78676 })})
	Register(Command{Name: "mltotbsp", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLILITRES TO TABLESPOONS. USE IT AS .MLTOTBSP <VALUE>.", Run: makeConvHandler("ML TO TBSP", "mltotbsp", "ML", "TBSP", func(v float64) float64 { return v * 0.067628 })})
	Register(Command{Name: "tspoml", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TEASPOONS TO MILLILITRES. USE IT AS .TSPOML <VALUE>.", Run: makeConvHandler("TSP TO ML", "tspoml", "TSP", "ML", func(v float64) float64 { return v * 4.928922 })})
	Register(Command{Name: "mltotsp", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLILITRES TO TEASPOONS. USE IT AS .MLTOTSP <VALUE>.", Run: makeConvHandler("ML TO TSP", "mltotsp", "ML", "TSP", func(v float64) float64 { return v * 0.202884 })})
	Register(Command{Name: "celsiusrankine", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CELSIUS TO RANKINE. USE IT AS .CELSIUSRANKINE <VALUE>.", Run: makeConvHandler("CELSIUS TO RANKINE", "celsiusrankine", "C", "R", func(v float64) float64 { return (v + 273.15) * 1.8 })})
	Register(Command{Name: "rankinetocelsius", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT RANKINE TO CELSIUS. USE IT AS .RANKINETOCELSIUS <VALUE>.", Run: makeConvHandler("RANKINE TO CELSIUS", "rankinetocelsius", "R", "C", func(v float64) float64 { return v/1.8 - 273.15 })})

	// hidden aliases
	Register(Command{Name: "gtoml", Category: "CONVERTER", Desc: "Short alias of .gramstoml", Hidden: true, Run: makeConvHandler("GRAMS TO ML", "gramstoml", "G", "ML", func(v float64) float64 { return v })})
	Register(Command{Name: "mltog", Category: "CONVERTER", Desc: "Short alias of .mltograms", Hidden: true, Run: makeConvHandler("ML TO GRAMS", "mltograms", "ML", "G", func(v float64) float64 { return v })})
	Register(Command{Name: "floztoml", Category: "CONVERTER", Desc: "Short alias of .oztoml", Hidden: true, Run: makeConvHandler("FL OZ TO ML", "oztoml", "FL OZ", "ML", func(v float64) float64 { return v * 29.57353 })})
	Register(Command{Name: "mltooz", Category: "CONVERTER", Desc: "Short alias of .mltofluidoz", Hidden: true, Run: makeConvHandler("ML TO FL OZ", "mltofluidoz", "ML", "FL OZ", func(v float64) float64 { return v * 0.033814 })})
	Register(Command{Name: "tbsptoml", Category: "CONVERTER", Desc: "Short alias of .tbspoml", Hidden: true, Run: makeConvHandler("TBSP TO ML", "tbspoml", "TBSP", "ML", func(v float64) float64 { return v * 14.78676 })})
	Register(Command{Name: "mltottbsp", Category: "CONVERTER", Desc: "Short alias of .mltotbsp", Hidden: true, Run: makeConvHandler("ML TO TBSP", "mltotbsp", "ML", "TBSP", func(v float64) float64 { return v * 0.067628 })})
	Register(Command{Name: "tsptoml", Category: "CONVERTER", Desc: "Short alias of .tspoml", Hidden: true, Run: makeConvHandler("TSP TO ML", "tspoml", "TSP", "ML", func(v float64) float64 { return v * 4.928922 })})
	Register(Command{Name: "mltottsp", Category: "CONVERTER", Desc: "Short alias of .mltotsp", Hidden: true, Run: makeConvHandler("ML TO TSP", "mltotsp", "ML", "TSP", func(v float64) float64 { return v * 0.202884 })})
	Register(Command{Name: "ctorankine", Category: "CONVERTER", Desc: "Short alias of .celsiusrankine", Hidden: true, Run: makeConvHandler("CELSIUS TO RANKINE", "celsiusrankine", "C", "R", func(v float64) float64 { return (v + 273.15) * 1.8 })})
	Register(Command{Name: "rtocelsius", Category: "CONVERTER", Desc: "Short alias of .rankinetocelsius", Hidden: true, Run: makeConvHandler("RANKINE TO CELSIUS", "rankinetocelsius", "R", "C", func(v float64) float64 { return v/1.8 - 273.15 })})
}
