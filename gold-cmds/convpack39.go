package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 39 (10 pressure & force converters)
// File: convpack39.go
// ============================================================================
//   .bartopsi        -> bar to psi
//   .psitobar        -> psi to bar
//   .pascaltobar     -> pascal to bar
//   .bartopascal     -> bar to pascal
//   .atmtopsi        -> atmosphere to psi
//   .psitoatm        -> psi to atmosphere
//   .newtontokgforce -> newton to kilogram-force
//   .kgforcetonewton -> kilogram-force to newton
//   .mmhgtopascal    -> mmHg to pascal
//   .pascaltommhg    -> pascal to mmHg
// ============================================================================

func init() {
	Register(Command{Name: "bartopsi", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BAR TO PSI. USE IT AS .BARTOPSI <VALUE>.", Run: makeConvHandler("BAR TO PSI", "bartopsi", "BAR", "PSI", func(v float64) float64 { return v * 14.503774 })})
	Register(Command{Name: "psitobar", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT PSI TO BAR. USE IT AS .PSITOBAR <VALUE>.", Run: makeConvHandler("PSI TO BAR", "psitobar", "PSI", "BAR", func(v float64) float64 { return v * 0.068948 })})
	Register(Command{Name: "pascaltobar", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT PASCAL TO BAR. USE IT AS .PASCALTOBAR <VALUE>.", Run: makeConvHandler("PASCAL TO BAR", "pascaltobar", "PA", "BAR", func(v float64) float64 { return v / 100000 })})
	Register(Command{Name: "bartopascal", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT BAR TO PASCAL. USE IT AS .BARTOPASCAL <VALUE>.", Run: makeConvHandler("BAR TO PASCAL", "bartopascal", "BAR", "PA", func(v float64) float64 { return v * 100000 })})
	Register(Command{Name: "atmtopsi", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT ATMOSPHERE TO PSI. USE IT AS .ATMTOPSI <VALUE>.", Run: makeConvHandler("ATM TO PSI", "atmtopsi", "ATM", "PSI", func(v float64) float64 { return v * 14.695949 })})
	Register(Command{Name: "psitoatm", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT PSI TO ATMOSPHERE. USE IT AS .PSITOATM <VALUE>.", Run: makeConvHandler("PSI TO ATM", "psitoatm", "PSI", "ATM", func(v float64) float64 { return v * 0.068046 })})
	Register(Command{Name: "newtontokgforce", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT NEWTON TO KILOGRAM-FORCE. USE IT AS .NEWTONTOKGFORCE <VALUE>.", Run: makeConvHandler("NEWTON TO KGF", "newtontokgforce", "N", "KGF", func(v float64) float64 { return v * 0.101972 })})
	Register(Command{Name: "kgforcetonewton", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KILOGRAM-FORCE TO NEWTON. USE IT AS .KGFORCETONEWTON <VALUE>.", Run: makeConvHandler("KGF TO NEWTON", "kgforcetonewton", "KGF", "N", func(v float64) float64 { return v * 9.80665 })})
	Register(Command{Name: "mmhgtopascal", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT MMHG TO PASCAL. USE IT AS .MMHGTOPASCAL <VALUE>.", Run: makeConvHandler("MMHG TO PASCAL", "mmhgtopascal", "MMHG", "PA", func(v float64) float64 { return v * 133.322387 })})
	Register(Command{Name: "pascaltommhg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT PASCAL TO MMHG. USE IT AS .PASCALTOMMHG <VALUE>.", Run: makeConvHandler("PASCAL TO MMHG", "pascaltommhg", "PA", "MMHG", func(v float64) float64 { return v * 0.00750062 })})

	// hidden aliases
	Register(Command{Name: "bartopsi2", Category: "CONVERTER", Desc: "Short alias of .bartopsi", Hidden: true, Run: makeConvHandler("BAR TO PSI", "bartopsi", "BAR", "PSI", func(v float64) float64 { return v * 14.503774 })})
	Register(Command{Name: "psitobar2", Category: "CONVERTER", Desc: "Short alias of .psitobar", Hidden: true, Run: makeConvHandler("PSI TO BAR", "psitobar", "PSI", "BAR", func(v float64) float64 { return v * 0.068948 })})
	Register(Command{Name: "patobar", Category: "CONVERTER", Desc: "Short alias of .pascaltobar", Hidden: true, Run: makeConvHandler("PASCAL TO BAR", "pascaltobar", "PA", "BAR", func(v float64) float64 { return v / 100000 })})
	Register(Command{Name: "bartopa", Category: "CONVERTER", Desc: "Short alias of .bartopascal", Hidden: true, Run: makeConvHandler("BAR TO PASCAL", "bartopascal", "BAR", "PA", func(v float64) float64 { return v * 100000 })})
	Register(Command{Name: "atmtopsi2", Category: "CONVERTER", Desc: "Short alias of .atmtopsi", Hidden: true, Run: makeConvHandler("ATM TO PSI", "atmtopsi", "ATM", "PSI", func(v float64) float64 { return v * 14.695949 })})
	Register(Command{Name: "psitoatm2", Category: "CONVERTER", Desc: "Short alias of .psitoatm", Hidden: true, Run: makeConvHandler("PSI TO ATM", "psitoatm", "PSI", "ATM", func(v float64) float64 { return v * 0.068046 })})
	Register(Command{Name: "ntokgf", Category: "CONVERTER", Desc: "Short alias of .newtontokgforce", Hidden: true, Run: makeConvHandler("NEWTON TO KGF", "newtontokgforce", "N", "KGF", func(v float64) float64 { return v * 0.101972 })})
	Register(Command{Name: "kgfton", Category: "CONVERTER", Desc: "Short alias of .kgforcetonewton", Hidden: true, Run: makeConvHandler("KGF TO NEWTON", "kgforcetonewton", "KGF", "N", func(v float64) float64 { return v * 9.80665 })})
	Register(Command{Name: "mmhgtopa", Category: "CONVERTER", Desc: "Short alias of .mmhgtopascal", Hidden: true, Run: makeConvHandler("MMHG TO PASCAL", "mmhgtopascal", "MMHG", "PA", func(v float64) float64 { return v * 133.322387 })})
	Register(Command{Name: "patommhg", Category: "CONVERTER", Desc: "Short alias of .pascaltommhg", Hidden: true, Run: makeConvHandler("PASCAL TO MMHG", "pascaltommhg", "PA", "MMHG", func(v float64) float64 { return v * 0.00750062 })})
}
