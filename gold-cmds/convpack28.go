package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 28 (10 length converters)
// File: convpack28.go
// ============================================================================
//   .kmtomiles        -> kilometres to miles
//   .milestokm        -> miles to kilometres
//   .mttofeet         -> metres to feet
//   .feettometers     -> feet to metres
//   .cmtoinches       -> centimetres to inches
//   .inchestocm       -> inches to centimetres
//   .mmtocm           -> millimetres to centimetres
//   .yardstometers    -> yards to metres
//   .nauticaltomiles  -> nautical miles to miles
//   .lightyeartokm    -> light years to kilometres
// ============================================================================

func init() {
	Register(Command{Name: "kmtomiles", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOMETRES TO MILES. USE IT AS .KMTOMILES <VALUE>.", Run: makeConvHandler("KM TO MILES", "kmtomiles", "KM", "MI", func(v float64) float64 { return v * 0.621371 })})
	Register(Command{Name: "milestokm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILES TO KILOMETRES. USE IT AS .MILESTOKM <VALUE>.", Run: makeConvHandler("MILES TO KM", "milestokm", "MI", "KM", func(v float64) float64 { return v * 1.609344 })})
	Register(Command{Name: "mttofeet", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT METRES TO FEET. USE IT AS .MTTOFEET <VALUE>.", Run: makeConvHandler("METRES TO FEET", "mttofeet", "M", "FT", func(v float64) float64 { return v * 3.28084 })})
	Register(Command{Name: "feettometers", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FEET TO METRES. USE IT AS .FEETTOMETERS <VALUE>.", Run: makeConvHandler("FEET TO METRES", "feettometers", "FT", "M", func(v float64) float64 { return v * 0.3048 })})
	Register(Command{Name: "cmtoinches", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CENTIMETRES TO INCHES. USE IT AS .CMTOINCHES <VALUE>.", Run: makeConvHandler("CM TO INCHES", "cmtoinches", "CM", "IN", func(v float64) float64 { return v * 0.393701 })})
	Register(Command{Name: "inchestocm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT INCHES TO CENTIMETRES. USE IT AS .INCHESTOCM <VALUE>.", Run: makeConvHandler("INCHES TO CM", "inchestocm", "IN", "CM", func(v float64) float64 { return v * 2.54 })})
	Register(Command{Name: "mmtocm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLIMETRES TO CENTIMETRES. USE IT AS .MMTOCM <VALUE>.", Run: makeConvHandler("MM TO CM", "mmtocm", "MM", "CM", func(v float64) float64 { return v / 10 })})
	Register(Command{Name: "yardstometers", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT YARDS TO METRES. USE IT AS .YARDSTOMETERS <VALUE>.", Run: makeConvHandler("YARDS TO METRES", "yardstometers", "YD", "M", func(v float64) float64 { return v * 0.9144 })})
	Register(Command{Name: "nauticaltomiles", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT NAUTICAL MILES TO MILES. USE IT AS .NAUTICALTOMILES <VALUE>.", Run: makeConvHandler("NAUTICAL TO MILES", "nauticaltomiles", "NM", "MI", func(v float64) float64 { return v * 1.150779 })})
	Register(Command{Name: "lightyeartokm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT LIGHT YEARS TO KILOMETRES. USE IT AS .LIGHTYEARTOKM <VALUE>.", Run: makeConvHandler("LIGHT YEAR TO KM", "lightyeartokm", "LY", "KM", func(v float64) float64 { return v * 9.4607e12 })})

	// hidden aliases
	Register(Command{Name: "kmtomi", Category: "CONVERTER", Desc: "Short alias of .kmtomiles", Hidden: true, Run: makeConvHandler("KM TO MILES", "kmtomiles", "KM", "MI", func(v float64) float64 { return v * 0.621371 })})
	Register(Command{Name: "mitokm", Category: "CONVERTER", Desc: "Short alias of .milestokm", Hidden: true, Run: makeConvHandler("MILES TO KM", "milestokm", "MI", "KM", func(v float64) float64 { return v * 1.609344 })})
	Register(Command{Name: "mtoft", Category: "CONVERTER", Desc: "Short alias of .mttofeet", Hidden: true, Run: makeConvHandler("METRES TO FEET", "mttofeet", "M", "FT", func(v float64) float64 { return v * 3.28084 })})
	Register(Command{Name: "fttom", Category: "CONVERTER", Desc: "Short alias of .feettometers", Hidden: true, Run: makeConvHandler("FEET TO METRES", "feettometers", "FT", "M", func(v float64) float64 { return v * 0.3048 })})
	Register(Command{Name: "cmtoin", Category: "CONVERTER", Desc: "Short alias of .cmtoinches", Hidden: true, Run: makeConvHandler("CM TO INCHES", "cmtoinches", "CM", "IN", func(v float64) float64 { return v * 0.393701 })})
	Register(Command{Name: "intocm", Category: "CONVERTER", Desc: "Short alias of .inchestocm", Hidden: true, Run: makeConvHandler("INCHES TO CM", "inchestocm", "IN", "CM", func(v float64) float64 { return v * 2.54 })})
	Register(Command{Name: "mmtocentimeter", Category: "CONVERTER", Desc: "Short alias of .mmtocm", Hidden: true, Run: makeConvHandler("MM TO CM", "mmtocm", "MM", "CM", func(v float64) float64 { return v / 10 })})
	Register(Command{Name: "ydtom", Category: "CONVERTER", Desc: "Short alias of .yardstometers", Hidden: true, Run: makeConvHandler("YARDS TO METRES", "yardstometers", "YD", "M", func(v float64) float64 { return v * 0.9144 })})
	Register(Command{Name: "nmitomi", Category: "CONVERTER", Desc: "Short alias of .nauticaltomiles", Hidden: true, Run: makeConvHandler("NAUTICAL TO MILES", "nauticaltomiles", "NM", "MI", func(v float64) float64 { return v * 1.150779 })})
	Register(Command{Name: "lytokm", Category: "CONVERTER", Desc: "Short alias of .lightyeartokm", Hidden: true, Run: makeConvHandler("LIGHT YEAR TO KM", "lightyeartokm", "LY", "KM", func(v float64) float64 { return v * 9.4607e12 })})
}
