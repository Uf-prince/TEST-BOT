package goldcmds

// ============================================================================
// GOLD-MD — 1000 EQUALIZER DESIGNS (.eq1 ... .eq1000)
// File: equalizer1000_keys.go
// ============================================================================
// OWNER ORDER: .font / .logo ki tarah .EQUALIZER bhi 1000 commands ka ho.
//   .equalizer        → fancy boxed menu, EQ1 .. EQ1000 ki list
//   .eqN (reply media)→ us combo ka effect us audio/video par lagta hai.
//
// DESIGN UNIQUENESS: har N = (speed, tone, effect) ka unique combo:
//   effect = (N-1) % 10
//   tone   = (N-1)/10 % 10
//   speed  = (N-1)/100 % 10
// 10 x 10 x 10 = 1000 combos, is liye N=1..1000 me koi design repeat nahi hota.
//
// Har filter chain pehle ffmpeg par validate ki gayi hai (bundled static build).
// ============================================================================

// EqCount is the number of .eq designs (owner order: 1000).
const EqCount = 1000

// eqSpeed is one pitch/tempo family. PTS is the setpts multiplier used to
// retime a video so its picture stays in sync with the shifted audio ("" =
// no retiming needed).
type eqSpeed struct {
	Name   string
	Filter string
	PTS    string
}

// eqSpeeds holds 10 pitch/tempo treatments. Index 0 is neutral.
var eqSpeeds = [10]eqSpeed{
	{"NORMAL", "", ""},
	{"SLOWED", "asetrate=44100*0.92,aresample=44100,atempo=0.90", "1.21*PTS"},
	{"DEEP", "asetrate=44100*0.85,aresample=44100,atempo=1.176", ""},
	{"NIGHTCORE", "asetrate=44100*1.18,aresample=44100,atempo=1.15", "0.74*PTS"},
	{"MONSTER", "asetrate=44100*0.80,aresample=44100,atempo=1.05", "1.19*PTS"},
	{"CHIPMUNK", "asetrate=44100*1.30,aresample=44100,atempo=1.10", "0.70*PTS"},
	{"SMOOTH SLOW", "atempo=0.75", "1.33*PTS"},
	{"SUPER FAST", "atempo=1.35", "0.74*PTS"},
	{"DEEP SOFT", "asetrate=44100*0.97,aresample=44100", "1.03*PTS"},
	{"HIGH SOFT", "asetrate=44100*1.05,aresample=44100", "0.95*PTS"},
}

// eqTone is one tonal-balance family. Index 0 is neutral.
type eqTone struct {
	Name   string
	Filter string
}

// eqTones holds 10 tone treatments (bass/treble/EQ/compression).
var eqTones = [10]eqTone{
	{"FLAT", ""},
	{"BASS", "bass=g=12:f=110:w=0.6,alimiter=limit=0.95"},
	{"TREBLE", "treble=g=6:f=3000:w=0.5"},
	{"BASS TREBLE", "bass=g=8:f=100:w=0.5,treble=g=5:f=3000:w=0.5"},
	{"LOW EQ", "equalizer=f=300:t=q:w=1:g=6"},
	{"MID EQ", "equalizer=f=3000:t=q:w=1:g=6"},
	{"TELEPHONE", "lowpass=f=1200,highpass=f=200"},
	{"THIN", "highpass=f=300"},
	{"MUFFLED", "lowpass=f=3000"},
	{"LOUD", "acompressor=threshold=0.089:ratio=9:attack=200:release=1000,volume=1.4"},
}

// eqToneNames / eqEffectNames feed the human-readable design label.
var eqEffects1000 = [10]struct {
	Name   string
	Filter string
}{
	{"ECHO", "aecho=0.8:0.9:300|600:0.5|0.3"},
	{"CHORUS", "chorus=0.7:0.9:55:0.4:0.25:2"},
	{"FLANGER", "flanger=delay=10:depth=3:regen=5:width=80:speed=3:shape=triangular"},
	{"PHASER", "aphaser=in_gain=0.4:out_gain=0.74:delay=3:decay=0.4:speed=0.5:type=t"},
	{"VIBRATO", "vibrato=f=6:d=0.5"},
	{"TREMOLO", "tremolo=f=5:d=0.6"},
	{"REVERSE", "areverse"},
	{"8-BIT", "acrusher=bits=10:mode=log:aa=1"},
	{"SPACE", "chorus=0.5:0.9:50:0.4:0.25:2,aecho=0.8:0.88:60:0.4"},
	{"ROBOT", "asetrate=44100*0.85,aresample=44100,atempo=1.176,aecho=0.8:0.88:60:0.4,vibrato=f=12:d=0.6"},
}

// eqCombo splits a 1-based design number into its three table indices.
func eqCombo(n int) (speed, tone, effect int) {
	if n < 1 || n > EqCount {
		n = 1
	}
	m := n - 1
	effect = m % 10
	m /= 10
	tone = m % 10
	m /= 10
	speed = m % 10
	return
}

// eqComboIndex is the inverse of eqCombo: it builds the design number for a
// (speed, tone, effect) triple. Named aliases use it to point at a combo.
func eqComboIndex(speed, tone, effect int) int {
	return speed*100 + tone*10 + effect + 1
}

// EqAudioFilter returns the ffmpeg -af chain for design N (never empty: it
// always carries at least one working filter).
func EqAudioFilter(n int) string {
	speed, tone, effect := eqCombo(n)
	var parts []string
	for _, f := range []string{eqSpeeds[speed].Filter, eqTones[tone].Filter, eqEffects1000[effect].Filter} {
		if f != "" {
			parts = append(parts, f)
		}
	}
	if len(parts) == 0 {
		// neutral triple (n=1) — still run a no-op-safe limiter so the audio is
		// genuinely re-encoded rather than erroring on an empty filtergraph.
		return "alimiter=limit=0.95"
	}
	return joinComma(parts)
}

// EqVideoPTS returns the setpts multiplier for design N, or "" when the picture
// does not need retiming (the video stream can be copied as-is).
func EqVideoPTS(n int) string {
	speed, _, _ := eqCombo(n)
	return eqSpeeds[speed].PTS
}

// EqDesignName is the human-readable label, e.g. "SLOWED + BASS + ECHO".
func EqDesignName(n int) string {
	speed, tone, effect := eqCombo(n)
	return eqSpeeds[speed].Name + " + " + eqTones[tone].Name + " + " + eqEffects1000[effect].Name
}

// eqNamedPresets maps the 5 originally-ordered named effects onto design
// numbers, so .slowed / .revert / .robot / .bass / .dj keep working as aliases
// of the 1000-combo engine instead of carrying a second filter table.
var eqNamedPresets = map[string]int{
	"slowed": eqComboIndex(1, 0, 0), // slowed pitch + soft echo-free slow
	"revert": eqComboIndex(0, 0, 6), // flat tone + reverse
	"robot":  eqComboIndex(2, 0, 9), // deep pitch + robotic echo/vibrato
	"bass":   eqComboIndex(0, 1, 0), // heavy bass + echo
	"dj":     eqComboIndex(0, 3, 0), // bass+treble + club echo
}

// joinComma is a tiny strings.Join wrapper kept local so the keys file has no
// extra imports beyond the engine's own.
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}
