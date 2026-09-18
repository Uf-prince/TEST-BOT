package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 30 (10 temperature & volume converters)
// File: convpack30.go
// ============================================================================
//   .celsiustofahrenheit -> °C to °F
//   .fahrenheittocelsius -> °F to °C
//   .celsiustokelvin     -> °C to K
//   .kelvintocelsius     -> K to °C
//   .literstogallons     -> litres to US gallons
//   .gallonstoliters     -> US gallons to litres
//   .mltofloz            -> millilitres to fluid ounces
//   .cuptoml             -> US cups to millilitres
//   .pintstoliters       -> US pints to litres
//   .quartstoliters      -> US quarts to litres
// ============================================================================

func init() {
	Register(Command{Name: "celsiustofahrenheit", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CELSIUS TO FAHRENHEIT. USE IT AS .CELSIUSTOFAHRENHEIT <VALUE>.", Run: makeConvHandler("CELSIUS TO FAHRENHEIT", "celsiustofahrenheit", "C", "F", func(v float64) float64 { return v*9/5 + 32 })})
	Register(Command{Name: "fahrenheittocelsius", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FAHRENHEIT TO CELSIUS. USE IT AS .FAHRENHEITTOCELSIUS <VALUE>.", Run: makeConvHandler("FAHRENHEIT TO CELSIUS", "fahrenheittocelsius", "F", "C", func(v float64) float64 { return (v - 32) * 5 / 9 })})
	Register(Command{Name: "celsiustokelvin", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT CELSIUS TO KELVIN. USE IT AS .CELSIUSTOKELVIN <VALUE>.", Run: makeConvHandler("CELSIUS TO KELVIN", "celsiustokelvin", "C", "K", func(v float64) float64 { return v + 273.15 })})
	Register(Command{Name: "kelvintocelsius", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KELVIN TO CELSIUS. USE IT AS .KELVINTOCELSIUS <VALUE>.", Run: makeConvHandler("KELVIN TO CELSIUS", "kelvintocelsius", "K", "C", func(v float64) float64 { return v - 273.15 })})
	Register(Command{Name: "literstogallons", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT LITRES TO US GALLONS. USE IT AS .LITERSTOGALLONS <VALUE>.", Run: makeConvHandler("LITRES TO GALLONS", "literstogallons", "L", "GAL", func(v float64) float64 { return v * 0.264172 })})
	Register(Command{Name: "gallonstoliters", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT US GALLONS TO LITRES. USE IT AS .GALLONSTOLITERS <VALUE>.", Run: makeConvHandler("GALLONS TO LITRES", "gallonstoliters", "GAL", "L", func(v float64) float64 { return v * 3.785412 })})
	Register(Command{Name: "mltofloz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILLILITRES TO FLUID OUNCES. USE IT AS .MLTOFLOZ <VALUE>.", Run: makeConvHandler("ML TO FL OZ", "mltofloz", "ML", "FLOZ", func(v float64) float64 { return v * 0.033814 })})
	Register(Command{Name: "cuptoml", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT US CUPS TO MILLILITRES. USE IT AS .CUPTOML <VALUE>.", Run: makeConvHandler("CUPS TO ML", "cuptoml", "CUP", "ML", func(v float64) float64 { return v * 236.588236 })})
	Register(Command{Name: "pintstoliters", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT US PINTS TO LITRES. USE IT AS .PINTSTOLITERS <VALUE>.", Run: makeConvHandler("PINTS TO LITRES", "pintstoliters", "PT", "L", func(v float64) float64 { return v * 0.473176 })})
	Register(Command{Name: "quartstoliters", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT US QUARTS TO LITRES. USE IT AS .QUARTSTOLITERS <VALUE>.", Run: makeConvHandler("QUARTS TO LITRES", "quartstoliters", "QT", "L", func(v float64) float64 { return v * 0.946353 })})

	// hidden aliases
	Register(Command{Name: "ctof", Category: "CONVERTER", Desc: "Short alias of .celsiustofahrenheit", Hidden: true, Run: makeConvHandler("CELSIUS TO FAHRENHEIT", "celsiustofahrenheit", "C", "F", func(v float64) float64 { return v*9/5 + 32 })})
	Register(Command{Name: "ftoc", Category: "CONVERTER", Desc: "Short alias of .fahrenheittocelsius", Hidden: true, Run: makeConvHandler("FAHRENHEIT TO CELSIUS", "fahrenheittocelsius", "F", "C", func(v float64) float64 { return (v - 32) * 5 / 9 })})
	Register(Command{Name: "ctok", Category: "CONVERTER", Desc: "Short alias of .celsiustokelvin", Hidden: true, Run: makeConvHandler("CELSIUS TO KELVIN", "celsiustokelvin", "C", "K", func(v float64) float64 { return v + 273.15 })})
	Register(Command{Name: "ktoc", Category: "CONVERTER", Desc: "Short alias of .kelvintocelsius", Hidden: true, Run: makeConvHandler("KELVIN TO CELSIUS", "kelvintocelsius", "K", "C", func(v float64) float64 { return v - 273.15 })})
	Register(Command{Name: "ltogal", Category: "CONVERTER", Desc: "Short alias of .literstogallons", Hidden: true, Run: makeConvHandler("LITRES TO GALLONS", "literstogallons", "L", "GAL", func(v float64) float64 { return v * 0.264172 })})
	Register(Command{Name: "galtol", Category: "CONVERTER", Desc: "Short alias of .gallonstoliters", Hidden: true, Run: makeConvHandler("GALLONS TO LITRES", "gallonstoliters", "GAL", "L", func(v float64) float64 { return v * 3.785412 })})
	Register(Command{Name: "mltooz", Category: "CONVERTER", Desc: "Short alias of .mltofloz", Hidden: true, Run: makeConvHandler("ML TO FL OZ", "mltofloz", "ML", "FLOZ", func(v float64) float64 { return v * 0.033814 })})
	Register(Command{Name: "cuptomilliliter", Category: "CONVERTER", Desc: "Short alias of .cuptoml", Hidden: true, Run: makeConvHandler("CUPS TO ML", "cuptoml", "CUP", "ML", func(v float64) float64 { return v * 236.588236 })})
	Register(Command{Name: "pttol", Category: "CONVERTER", Desc: "Short alias of .pintstoliters", Hidden: true, Run: makeConvHandler("PINTS TO LITRES", "pintstoliters", "PT", "L", func(v float64) float64 { return v * 0.473176 })})
	Register(Command{Name: "qttol", Category: "CONVERTER", Desc: "Short alias of .quartstoliters", Hidden: true, Run: makeConvHandler("QUARTS TO LITRES", "quartstoliters", "QT", "L", func(v float64) float64 { return v * 0.946353 })})
}
