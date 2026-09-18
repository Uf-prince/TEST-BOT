package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 32 (10 data storage converters)
// File: convpack32.go
// ============================================================================
//   .bytestokb   -> bytes to kilobytes
//   .kbtomb      -> kilobytes to megabytes
//   .mbtogb      -> megabytes to gigabytes
//   .gbtotb      -> gigabytes to terabytes
//   .tbtopb      -> terabytes to petabytes
//   .gbtomb      -> gigabytes to megabytes
//   .mbtokb      -> megabytes to kilobytes
//   .kbtobytes   -> kilobytes to bytes
//   .bitstobytes -> bits to bytes
//   .bytestobits -> bytes to bits
// ============================================================================

func init() {
	Register(Command{Name: "bytestokb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BYTES TO KILOBYTES. USE IT AS .BYTESTOKB <VALUE>.", Run: makeConvHandler("BYTES TO KB", "bytestokb", "B", "KB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "kbtomb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOBYTES TO MEGABYTES. USE IT AS .KBTOMB <VALUE>.", Run: makeConvHandler("KB TO MB", "kbtomb", "KB", "MB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "mbtogb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MEGABYTES TO GIGABYTES. USE IT AS .MBTOGB <VALUE>.", Run: makeConvHandler("MB TO GB", "mbtogb", "MB", "GB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "gbtotb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT GIGABYTES TO TERABYTES. USE IT AS .GBTOTB <VALUE>.", Run: makeConvHandler("GB TO TB", "gbtotb", "GB", "TB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "tbtopb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT TERABYTES TO PETABYTES. USE IT AS .TBTOPB <VALUE>.", Run: makeConvHandler("TB TO PB", "tbtopb", "TB", "PB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "gbtomb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT GIGABYTES TO MEGABYTES. USE IT AS .GBTOMB <VALUE>.", Run: makeConvHandler("GB TO MB", "gbtomb", "GB", "MB", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "mbtokb", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MEGABYTES TO KILOBYTES. USE IT AS .MBTOKB <VALUE>.", Run: makeConvHandler("MB TO KB", "mbtokb", "MB", "KB", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "kbtobytes", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOBYTES TO BYTES. USE IT AS .KBTOBYTES <VALUE>.", Run: makeConvHandler("KB TO BYTES", "kbtobytes", "KB", "B", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "bitstobytes", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BITS TO BYTES. USE IT AS .BITSTOBYTES <VALUE>.", Run: makeConvHandler("BITS TO BYTES", "bitstobytes", "BIT", "B", func(v float64) float64 { return v / 8 })})
	Register(Command{Name: "bytestobits", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BYTES TO BITS. USE IT AS .BYTESTOBITS <VALUE>.", Run: makeConvHandler("BYTES TO BITS", "bytestobits", "B", "BIT", func(v float64) float64 { return v * 8 })})

	// hidden aliases
	Register(Command{Name: "btokb", Category: "CONVERTER", Desc: "Short alias of .bytestokb", Hidden: true, Run: makeConvHandler("BYTES TO KB", "bytestokb", "B", "KB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "kbtomb2", Category: "CONVERTER", Desc: "Short alias of .kbtomb", Hidden: true, Run: makeConvHandler("KB TO MB", "kbtomb", "KB", "MB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "mbtogb2", Category: "CONVERTER", Desc: "Short alias of .mbtogb", Hidden: true, Run: makeConvHandler("MB TO GB", "mbtogb", "MB", "GB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "gbtotb2", Category: "CONVERTER", Desc: "Short alias of .gbtotb", Hidden: true, Run: makeConvHandler("GB TO TB", "gbtotb", "GB", "TB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "tbtopb2", Category: "CONVERTER", Desc: "Short alias of .tbtopb", Hidden: true, Run: makeConvHandler("TB TO PB", "tbtopb", "TB", "PB", func(v float64) float64 { return v / 1024 })})
	Register(Command{Name: "gbtomb2", Category: "CONVERTER", Desc: "Short alias of .gbtomb", Hidden: true, Run: makeConvHandler("GB TO MB", "gbtomb", "GB", "MB", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "mbtokb2", Category: "CONVERTER", Desc: "Short alias of .mbtokb", Hidden: true, Run: makeConvHandler("MB TO KB", "mbtokb", "MB", "KB", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "kbtob", Category: "CONVERTER", Desc: "Short alias of .kbtobytes", Hidden: true, Run: makeConvHandler("KB TO BYTES", "kbtobytes", "KB", "B", func(v float64) float64 { return v * 1024 })})
	Register(Command{Name: "bitstob", Category: "CONVERTER", Desc: "Short alias of .bitstobytes", Hidden: true, Run: makeConvHandler("BITS TO BYTES", "bitstobytes", "BIT", "B", func(v float64) float64 { return v / 8 })})
	Register(Command{Name: "bytestobit", Category: "CONVERTER", Desc: "Short alias of .bytestobits", Hidden: true, Run: makeConvHandler("BYTES TO BITS", "bytestobits", "B", "BIT", func(v float64) float64 { return v * 8 })})
}
