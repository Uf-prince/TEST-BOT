package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 29 (10 weight & mass converters)
// File: convpack29.go
// ============================================================================
//   .kgtolbs            -> kilograms to pounds
//   .lbstokg            -> pounds to kilograms
//   .gramstoounces      -> grams to ounces
//   .ouncestograms      -> ounces to grams
//   .tonstokg           -> metric tons to kilograms
//   .stonektopounds     -> stones to pounds
//   .milligramstograms  -> milligrams to grams
//   .caratstograms      -> carats to grams
//   .quintaltokg        -> quintals to kilograms
//   .metrictonstolbs    -> metric tons to pounds
// ============================================================================

func init() {
	Register(Command{Name: "kgtolbs", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOGRAMS TO POUNDS. USE IT AS .KGTOLBS <VALUE>.", Run: makeConvHandler("KG TO LBS", "kgtolbs", "KG", "LB", func(v float64) float64 { return v * 2.204623 })})
	Register(Command{Name: "lbstokg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT POUNDS TO KILOGRAMS. USE IT AS .LBSTOKG <VALUE>.", Run: makeConvHandler("LBS TO KG", "lbstokg", "LB", "KG", func(v float64) float64 { return v * 0.453592 })})
	Register(Command{Name: "gramstoounces", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT GRAMS TO OUNCES. USE IT AS .GRAMSTOOUNCES <VALUE>.", Run: makeConvHandler("GRAMS TO OUNCES", "gramstoounces", "G", "OZ", func(v float64) float64 { return v * 0.035274 })})
	Register(Command{Name: "ouncestograms", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT OUNCES TO GRAMS. USE IT AS .OUNCESTOGRAMS <VALUE>.", Run: makeConvHandler("OUNCES TO GRAMS", "ouncestograms", "OZ", "G", func(v float64) float64 { return v * 28.349523 })})
	Register(Command{Name: "tonstokg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT METRIC TONS TO KILOGRAMS. USE IT AS .TONSTOKG <VALUE>.", Run: makeConvHandler("TONS TO KG", "tonstokg", "T", "KG", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "stonektopounds", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT STONES TO POUNDS. USE IT AS .STONETOPOUNDS <VALUE>.", Run: makeConvHandler("STONE TO POUNDS", "stonektopounds", "ST", "LB", func(v float64) float64 { return v * 14 })})
	Register(Command{Name: "milligramstograms", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLIGRAMS TO GRAMS. USE IT AS .MILLIGRAMSTOGRAMS <VALUE>.", Run: makeConvHandler("MG TO GRAMS", "milligramstograms", "MG", "G", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "caratstograms", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CARATS TO GRAMS. USE IT AS .CARATSTOGRAMS <VALUE>.", Run: makeConvHandler("CARATS TO GRAMS", "caratstograms", "CT", "G", func(v float64) float64 { return v * 0.2 })})
	Register(Command{Name: "quintaltokg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT QUINTALS TO KILOGRAMS. USE IT AS .QUINTALTOKG <VALUE>.", Run: makeConvHandler("QUINTAL TO KG", "quintaltokg", "Q", "KG", func(v float64) float64 { return v * 100 })})
	Register(Command{Name: "metrictonstolbs", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT METRIC TONS TO POUNDS. USE IT AS .METRICTONSTOLBS <VALUE>.", Run: makeConvHandler("TONS TO LBS", "metrictonstolbs", "T", "LB", func(v float64) float64 { return v * 2204.622622 })})

	// hidden aliases
	Register(Command{Name: "kgtolb", Category: "CONVERTER", Desc: "Short alias of .kgtolbs", Hidden: true, Run: makeConvHandler("KG TO LBS", "kgtolbs", "KG", "LB", func(v float64) float64 { return v * 2.204623 })})
	Register(Command{Name: "lbtokg", Category: "CONVERTER", Desc: "Short alias of .lbstokg", Hidden: true, Run: makeConvHandler("LBS TO KG", "lbstokg", "LB", "KG", func(v float64) float64 { return v * 0.453592 })})
	Register(Command{Name: "gtooz", Category: "CONVERTER", Desc: "Short alias of .gramstoounces", Hidden: true, Run: makeConvHandler("GRAMS TO OUNCES", "gramstoounces", "G", "OZ", func(v float64) float64 { return v * 0.035274 })})
	Register(Command{Name: "oztog", Category: "CONVERTER", Desc: "Short alias of .ouncestograms", Hidden: true, Run: makeConvHandler("OUNCES TO GRAMS", "ouncestograms", "OZ", "G", func(v float64) float64 { return v * 28.349523 })})
	Register(Command{Name: "ttokg", Category: "CONVERTER", Desc: "Short alias of .tonstokg", Hidden: true, Run: makeConvHandler("TONS TO KG", "tonstokg", "T", "KG", func(v float64) float64 { return v * 1000 })})
	Register(Command{Name: "stonetolb", Category: "CONVERTER", Desc: "Short alias of .stonektopounds", Hidden: true, Run: makeConvHandler("STONE TO POUNDS", "stonektopounds", "ST", "LB", func(v float64) float64 { return v * 14 })})
	Register(Command{Name: "mgtog", Category: "CONVERTER", Desc: "Short alias of .milligramstograms", Hidden: true, Run: makeConvHandler("MG TO GRAMS", "milligramstograms", "MG", "G", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "cttog", Category: "CONVERTER", Desc: "Short alias of .caratstograms", Hidden: true, Run: makeConvHandler("CARATS TO GRAMS", "caratstograms", "CT", "G", func(v float64) float64 { return v * 0.2 })})
	Register(Command{Name: "qtokg", Category: "CONVERTER", Desc: "Short alias of .quintaltokg", Hidden: true, Run: makeConvHandler("QUINTAL TO KG", "quintaltokg", "Q", "KG", func(v float64) float64 { return v * 100 })})
	Register(Command{Name: "ttolb", Category: "CONVERTER", Desc: "Short alias of .metrictonstolbs", Hidden: true, Run: makeConvHandler("TONS TO LBS", "metrictonstolbs", "T", "LB", func(v float64) float64 { return v * 2204.622622 })})
}
