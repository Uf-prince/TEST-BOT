package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 40 (10 frequency, torque & fuel economy converters)
// File: convpack40.go
// ============================================================================
//   .hzrtokhz      -> hertz to kilohertz
//   .khztomhz      -> kilohertz to megahertz
//   .mhztoghz      -> megahertz to gigahertz
//   .mpgtokmpl     -> miles per gallon to km per litre
//   .kmpltompg     -> km per litre to miles per gallon
//   .lp100kmtompg  -> litres per 100km to miles per gallon
//   .nmtolf        -> newton-metre to foot-pound
//   .lftonm        -> foot-pound to newton-metre
//   .rpmtorad      -> rpm to radians per second
//   .radtorpm      -> radians per second to rpm
// ============================================================================

func init() {
	Register(Command{Name: "hzrtokhz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT HERTZ TO KILOHERTZ. USE IT AS .HZRTOKHZ <VALUE>.", Run: makeConvHandler("HZ TO KHZ", "hzrtokhz", "HZ", "KHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "khztomhz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOHERTZ TO MEGAHERTZ. USE IT AS .KHZTOMHZ <VALUE>.", Run: makeConvHandler("KHZ TO MHZ", "khztomhz", "KHZ", "MHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "mhztoghz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MEGAHERTZ TO GIGAHERTZ. USE IT AS .MHZTOGHZ <VALUE>.", Run: makeConvHandler("MHZ TO GHZ", "mhztoghz", "MHZ", "GHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "mpgtokmpl", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MILES PER GALLON TO KM PER LITRE. USE IT AS .MPGTOKMPL <VALUE>.", Run: makeConvHandler("MPG TO KM/L", "mpgtokmpl", "MPG", "KM/L", func(v float64) float64 { return v * 0.425144 })})
	Register(Command{Name: "kmpltompg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KM PER LITRE TO MILES PER GALLON. USE IT AS .KMPLTOMPG <VALUE>.", Run: makeConvHandler("KM/L TO MPG", "kmpltompg", "KM/L", "MPG", func(v float64) float64 { return v * 2.35215 })})
	Register(Command{Name: "lp100kmtompg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT LITRES PER 100KM TO MILES PER GALLON. USE IT AS .LP100KMTOMPG <VALUE>.", Run: makeConvHandler("L/100KM TO MPG", "lp100kmtompg", "L/100KM", "MPG", func(v float64) float64 { return 235.215 / v })})
	Register(Command{Name: "nmtolf", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT NEWTON-METRE TO FOOT-POUND. USE IT AS .NMTOLF <VALUE>.", Run: makeConvHandler("NM TO FT-LB", "nmtolf", "NM", "FT-LB", func(v float64) float64 { return v * 0.737562 })})
	Register(Command{Name: "lftonm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FOOT-POUND TO NEWTON-METRE. USE IT AS .LFTONM <VALUE>.", Run: makeConvHandler("FT-LB TO NM", "lftonm", "FT-LB", "NM", func(v float64) float64 { return v * 1.355818 })})
	Register(Command{Name: "rpmtorad", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT RPM TO RADIANS PER SECOND. USE IT AS .RPMTORAD <VALUE>.", Run: makeConvHandler("RPM TO RAD/S", "rpmtorad", "RPM", "RAD/S", func(v float64) float64 { return v * 0.10472 })})
	Register(Command{Name: "radtorpm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT RADIANS PER SECOND TO RPM. USE IT AS .RADTORPM <VALUE>.", Run: makeConvHandler("RAD/S TO RPM", "radtorpm", "RAD/S", "RPM", func(v float64) float64 { return v * 9.549297 })})

	// hidden aliases
	Register(Command{Name: "hztokhz", Category: "CONVERTER", Desc: "Short alias of .hzrtokhz", Hidden: true, Run: makeConvHandler("HZ TO KHZ", "hzrtokhz", "HZ", "KHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "khztoghz", Category: "CONVERTER", Desc: "Short alias of .khztomhz", Hidden: true, Run: makeConvHandler("KHZ TO MHZ", "khztomhz", "KHZ", "MHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "mhztokhz", Category: "CONVERTER", Desc: "Short alias of .mhztoghz", Hidden: true, Run: makeConvHandler("MHZ TO GHZ", "mhztoghz", "MHZ", "GHZ", func(v float64) float64 { return v / 1000 })})
	Register(Command{Name: "mpgtokml", Category: "CONVERTER", Desc: "Short alias of .mpgtokmpl", Hidden: true, Run: makeConvHandler("MPG TO KM/L", "mpgtokmpl", "MPG", "KM/L", func(v float64) float64 { return v * 0.425144 })})
	Register(Command{Name: "kmpltomiles", Category: "CONVERTER", Desc: "Short alias of .kmpltompg", Hidden: true, Run: makeConvHandler("KM/L TO MPG", "kmpltompg", "KM/L", "MPG", func(v float64) float64 { return v * 2.35215 })})
	Register(Command{Name: "lp100tompg", Category: "CONVERTER", Desc: "Short alias of .lp100kmtompg", Hidden: true, Run: makeConvHandler("L/100KM TO MPG", "lp100kmtompg", "L/100KM", "MPG", func(v float64) float64 { return 235.215 / v })})
	Register(Command{Name: "nmtolbft", Category: "CONVERTER", Desc: "Short alias of .nmtolf", Hidden: true, Run: makeConvHandler("NM TO FT-LB", "nmtolf", "NM", "FT-LB", func(v float64) float64 { return v * 0.737562 })})
	Register(Command{Name: "lftonm2", Category: "CONVERTER", Desc: "Short alias of .lftonm", Hidden: true, Run: makeConvHandler("FT-LB TO NM", "lftonm", "FT-LB", "NM", func(v float64) float64 { return v * 1.355818 })})
	Register(Command{Name: "rpmtorads", Category: "CONVERTER", Desc: "Short alias of .rpmtorad", Hidden: true, Run: makeConvHandler("RPM TO RAD/S", "rpmtorad", "RPM", "RAD/S", func(v float64) float64 { return v * 0.10472 })})
	Register(Command{Name: "radstorpm", Category: "CONVERTER", Desc: "Short alias of .radtorpm", Hidden: true, Run: makeConvHandler("RAD/S TO RPM", "radtorpm", "RAD/S", "RPM", func(v float64) float64 { return v * 9.549297 })})
}
