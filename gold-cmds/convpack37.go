package goldcmds

// ============================================================================
// GOLD-MD — CONVERTER PACK 37 (10 angle, fraction, percent & misc converters)
// File: convpack37.go
// ============================================================================
//   .degtorad       -> degrees to radians
//   .radtodeg       -> radians to degrees
//   .fractiontodec  -> fraction (a/b) to decimal
//   .dectofraction  -> decimal to fraction
//   .percenttodec   -> percent to decimal
//   .dectopercent   -> decimal to percent
//   .galtooz        -> US gallons to fluid ounces
//   .oztogal        -> fluid ounces to US gallons
//   .stonetokg      -> stones to kilograms
//   .knotstomph     -> knots to mph
// ============================================================================

import (
	"context"
	"math"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func handleDegtorad(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "DEGREES TO RADIANS", "degtorad", "DEG", "RAD"))
			return
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
			return
		}
		numConvReply(s, info, "DEGREES TO RADIANS", "DEG", strconv.FormatFloat(v, 'f', -1, 64), "RAD", strconv.FormatFloat(v*math.Pi/180, 'f', -1, 64))
	})
}

func handleRadtodeg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "RADIANS TO DEGREES", "radtodeg", "RAD", "DEG"))
			return
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
			return
		}
		numConvReply(s, info, "RADIANS TO DEGREES", "RAD", strconv.FormatFloat(v, 'f', -1, 64), "DEG", strconv.FormatFloat(v*180/math.Pi, 'f', -1, 64))
	})
}

func handleFractiontodec(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "FRACTION TO DECIMAL", "fractiontodec", "FRACTION", "DECIMAL"))
			return
		}
		raw := strings.TrimSpace(args[0])
		parts := strings.SplitN(raw, "/", 2)
		if len(parts) != 2 {
			s.Reply(info, "*🔰 USE FORMAT ❯ NUMERATOR/DENOMINATOR (E.G. 3/4)*")
			return
		}
		num, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		den, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil || den == 0 {
			s.Reply(info, "*🔰 INVALID FRACTION, PLEASE CHECK YOUR INPUT*")
			return
		}
		numConvReply(s, info, "FRACTION TO DECIMAL", "FRACTION", raw, "DECIMAL", strconv.FormatFloat(num/den, 'f', -1, 64))
	})
}

func handleDectofraction(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "DECIMAL TO FRACTION", "dectofraction", "DECIMAL", "FRACTION"))
			return
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
			return
		}
		den := int64(1000000)
		num := int64(math.Round(v * float64(den)))
		g := convGCD(num, den)
		if g != 0 {
			num /= g
			den /= g
		}
		numConvReply(s, info, "DECIMAL TO FRACTION", "DECIMAL", strconv.FormatFloat(v, 'f', -1, 64), "FRACTION", strconv.FormatInt(num, 10)+"/"+strconv.FormatInt(den, 10))
	})
}

func convGCD(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func handlePercenttodec(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "PERCENT TO DECIMAL", "percenttodec", "PERCENT", "DECIMAL"))
			return
		}
		raw := strings.TrimSuffix(strings.TrimSpace(args[0]), "%")
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
			return
		}
		numConvReply(s, info, "PERCENT TO DECIMAL", "PERCENT", strconv.FormatFloat(v, 'f', -1, 64)+"%", "DECIMAL", strconv.FormatFloat(v/100, 'f', -1, 64))
	})
}

func handleDectopercent(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, convGuide(prefix, "DECIMAL TO PERCENT", "dectopercent", "DECIMAL", "PERCENT"))
			return
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, "*🔰 INVALID NUMBER, PLEASE GIVE A VALUE*")
			return
		}
		numConvReply(s, info, "DECIMAL TO PERCENT", "DECIMAL", strconv.FormatFloat(v, 'f', -1, 64), "PERCENT", strconv.FormatFloat(v*100, 'f', -1, 64)+"%")
	})
}

func init() {
	Register(Command{Name: "degtorad", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT DEGREES TO RADIANS. USE IT AS .DEGTORAD <VALUE>.", Run: handleDegtorad})
	Register(Command{Name: "radtodeg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT RADIANS TO DEGREES. USE IT AS .RADTODEG <VALUE>.", Run: handleRadtodeg})
	Register(Command{Name: "fractiontodec", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A FRACTION TO A DECIMAL. USE IT AS .FRACTIONTODEC <A/B>.", Run: handleFractiontodec})
	Register(Command{Name: "dectofraction", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A DECIMAL TO A FRACTION. USE IT AS .DECTOFRACTION <VALUE>.", Run: handleDectofraction})
	Register(Command{Name: "percenttodec", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A PERCENT TO A DECIMAL. USE IT AS .PERCENTTODEC <VALUE>.", Run: handlePercenttodec})
	Register(Command{Name: "dectopercent", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT A DECIMAL TO A PERCENT. USE IT AS .DECTOPERCENT <VALUE>.", Run: handleDectopercent})
	Register(Command{Name: "galtooz", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT US GALLONS TO FLUID OUNCES. USE IT AS .GALTOOZ <VALUE>.", Run: makeConvHandler("GALLONS TO FL OZ", "galtooz", "GAL", "FLOZ", func(v float64) float64 { return v * 128 })})
	Register(Command{Name: "oztogal", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT FLUID OUNCES TO US GALLONS. USE IT AS .OZTOGAL <VALUE>.", Run: makeConvHandler("FL OZ TO GALLONS", "oztogal", "FLOZ", "GAL", func(v float64) float64 { return v / 128 })})
	Register(Command{Name: "stonetokg", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT STONES TO KILOGRAMS. USE IT AS .STONETOKG <VALUE>.", Run: makeConvHandler("STONE TO KG", "stonetokg", "ST", "KG", func(v float64) float64 { return v * 6.350293 })})
	Register(Command{Name: "knotstomph", Category: "CONVERTER", Desc: "THIS COMMAND IS USED TO CONVERT KNOTS TO MPH. USE IT AS .KNOTSTOMPH <VALUE>.", Run: makeConvHandler("KNOTS TO MPH", "knotstomph", "KN", "MPH", func(v float64) float64 { return v * 1.150779 })})

	// hidden aliases
	Register(Command{Name: "degtorad2", Category: "CONVERTER", Desc: "Short alias of .degtorad", Hidden: true, Run: handleDegtorad})
	Register(Command{Name: "radtodeg2", Category: "CONVERTER", Desc: "Short alias of .radtodeg", Hidden: true, Run: handleRadtodeg})
	Register(Command{Name: "fractodec", Category: "CONVERTER", Desc: "Short alias of .fractiontodec", Hidden: true, Run: handleFractiontodec})
	Register(Command{Name: "dectofrac", Category: "CONVERTER", Desc: "Short alias of .dectofraction", Hidden: true, Run: handleDectofraction})
	Register(Command{Name: "pcttodec", Category: "CONVERTER", Desc: "Short alias of .percenttodec", Hidden: true, Run: handlePercenttodec})
	Register(Command{Name: "dectopct", Category: "CONVERTER", Desc: "Short alias of .dectopercent", Hidden: true, Run: handleDectopercent})
	Register(Command{Name: "gtofloz", Category: "CONVERTER", Desc: "Short alias of .galtooz", Hidden: true, Run: makeConvHandler("GALLONS TO FL OZ", "galtooz", "GAL", "FLOZ", func(v float64) float64 { return v * 128 })})
	Register(Command{Name: "floztogal", Category: "CONVERTER", Desc: "Short alias of .oztogal", Hidden: true, Run: makeConvHandler("FL OZ TO GALLONS", "oztogal", "FLOZ", "GAL", func(v float64) float64 { return v / 128 })})
	Register(Command{Name: "sttkg", Category: "CONVERTER", Desc: "Short alias of .stonetokg", Hidden: true, Run: makeConvHandler("STONE TO KG", "stonetokg", "ST", "KG", func(v float64) float64 { return v * 6.350293 })})
	Register(Command{Name: "kntomph", Category: "CONVERTER", Desc: "Short alias of .knotstomph", Hidden: true, Run: makeConvHandler("KNOTS TO MPH", "knotstomph", "KN", "MPH", func(v float64) float64 { return v * 1.150779 })})
}
