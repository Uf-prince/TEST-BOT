package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 31 (10 speed & area converters)
// File: convpack31.go
// ============================================================================
//   .kmphtomph        -> km/h to mph
//   .mphtokmph        -> mph to km/h
//   .knotstokmph      -> knots to km/h
//   .msectokmh        -> metres/second to km/h
//   .sqfttosqm        -> square feet to square metres
//   .sqmtosqft        -> square metres to square feet
//   .acrestohectares  -> acres to hectares
//   .hectarestoacres  -> hectares to acres
//   .sqkmtosqmi       -> square km to square miles
//   .sqmitosqkm       -> square miles to square km
// ============================================================================

func init() {
	Register(Command{Name: "kmphtomph", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KM/H TO MPH. USE IT AS .KMPHTOMPH <VALUE>.", Run: makeConvHandler("KM/H TO MPH", "kmphtomph", "KM/H", "MPH", func(v float64) float64 { return v * 0.621371 })})
	Register(Command{Name: "mphtokmph", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MPH TO KM/H. USE IT AS .MPHTOKMPH <VALUE>.", Run: makeConvHandler("MPH TO KM/H", "mphtokmph", "MPH", "KM/H", func(v float64) float64 { return v * 1.609344 })})
	Register(Command{Name: "knotstokmph", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KNOTS TO KM/H. USE IT AS .KNOTSTOKMPH <VALUE>.", Run: makeConvHandler("KNOTS TO KM/H", "knotstokmph", "KN", "KM/H", func(v float64) float64 { return v * 1.852 })})
	Register(Command{Name: "msectokmh", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT METRES PER SECOND TO KM/H. USE IT AS .MSECTOKMH <VALUE>.", Run: makeConvHandler("M/S TO KM/H", "msectokmh", "M/S", "KM/H", func(v float64) float64 { return v * 3.6 })})
	Register(Command{Name: "sqfttosqm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SQUARE FEET TO SQUARE METRES. USE IT AS .SQFTTOSQM <VALUE>.", Run: makeConvHandler("SQ FT TO SQ M", "sqfttosqm", "SQFT", "SQM", func(v float64) float64 { return v * 0.092903 })})
	Register(Command{Name: "sqmtosqft", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SQUARE METRES TO SQUARE FEET. USE IT AS .SQMTOSQFT <VALUE>.", Run: makeConvHandler("SQ M TO SQ FT", "sqmtosqft", "SQM", "SQFT", func(v float64) float64 { return v * 10.763910 })})
	Register(Command{Name: "acrestohectares", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT ACRES TO HECTARES. USE IT AS .ACRESTOHECTARES <VALUE>.", Run: makeConvHandler("ACRES TO HECTARES", "acrestohectares", "AC", "HA", func(v float64) float64 { return v * 0.404686 })})
	Register(Command{Name: "hectarestoacres", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HECTARES TO ACRES. USE IT AS .HECTARESTOACRES <VALUE>.", Run: makeConvHandler("HECTARES TO ACRES", "hectarestoacres", "HA", "AC", func(v float64) float64 { return v * 2.471054 })})
	Register(Command{Name: "sqkmtosqmi", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SQUARE KM TO SQUARE MILES. USE IT AS .SQKMTOSQMI <VALUE>.", Run: makeConvHandler("SQ KM TO SQ MI", "sqkmtosqmi", "SQKM", "SQMI", func(v float64) float64 { return v * 0.386102 })})
	Register(Command{Name: "sqmitosqkm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT SQUARE MILES TO SQUARE KM. USE IT AS .SQMITOSQKM <VALUE>.", Run: makeConvHandler("SQ MI TO SQ KM", "sqmitosqkm", "SQMI", "SQKM", func(v float64) float64 { return v * 2.589988 })})

	// hidden aliases
	Register(Command{Name: "kmhtomph", Category: "CONVERTER", Desc: "Short alias of .kmphtomph", Hidden: true, Run: makeConvHandler("KM/H TO MPH", "kmphtomph", "KM/H", "MPH", func(v float64) float64 { return v * 0.621371 })})
	Register(Command{Name: "mphtokmh", Category: "CONVERTER", Desc: "Short alias of .mphtokmph", Hidden: true, Run: makeConvHandler("MPH TO KM/H", "mphtokmph", "MPH", "KM/H", func(v float64) float64 { return v * 1.609344 })})
	Register(Command{Name: "kntokmh", Category: "CONVERTER", Desc: "Short alias of .knotstokmph", Hidden: true, Run: makeConvHandler("KNOTS TO KM/H", "knotstokmph", "KN", "KM/H", func(v float64) float64 { return v * 1.852 })})
	Register(Command{Name: "mstokmh", Category: "CONVERTER", Desc: "Short alias of .msectokmh", Hidden: true, Run: makeConvHandler("M/S TO KM/H", "msectokmh", "M/S", "KM/H", func(v float64) float64 { return v * 3.6 })})
	Register(Command{Name: "sqfttosqmtr", Category: "CONVERTER", Desc: "Short alias of .sqfttosqm", Hidden: true, Run: makeConvHandler("SQ FT TO SQ M", "sqfttosqm", "SQFT", "SQM", func(v float64) float64 { return v * 0.092903 })})
	Register(Command{Name: "sqmtosqft2", Category: "CONVERTER", Desc: "Short alias of .sqmtosqft", Hidden: true, Run: makeConvHandler("SQ M TO SQ FT", "sqmtosqft", "SQM", "SQFT", func(v float64) float64 { return v * 10.763910 })})
	Register(Command{Name: "actoha", Category: "CONVERTER", Desc: "Short alias of .acrestohectares", Hidden: true, Run: makeConvHandler("ACRES TO HECTARES", "acrestohectares", "AC", "HA", func(v float64) float64 { return v * 0.404686 })})
	Register(Command{Name: "hatoac", Category: "CONVERTER", Desc: "Short alias of .hectarestoacres", Hidden: true, Run: makeConvHandler("HECTARES TO ACRES", "hectarestoacres", "HA", "AC", func(v float64) float64 { return v * 2.471054 })})
	Register(Command{Name: "sqkmtosqmi2", Category: "CONVERTER", Desc: "Short alias of .sqkmtosqmi", Hidden: true, Run: makeConvHandler("SQ KM TO SQ MI", "sqkmtosqmi", "SQKM", "SQMI", func(v float64) float64 { return v * 0.386102 })})
	Register(Command{Name: "sqmitosqkm2", Category: "CONVERTER", Desc: "Short alias of .sqmitosqkm", Hidden: true, Run: makeConvHandler("SQ MI TO SQ KM", "sqmitosqkm", "SQMI", "SQKM", func(v float64) float64 { return v * 2.589988 })})
}
