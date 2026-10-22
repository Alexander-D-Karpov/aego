package asset

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"

	"aego/core/ids"
)

const builtinAudioRate = 32000

var (
	BuiltinBeep   = ids.AssetID{Hi: builtinHi, Lo: 0xa01}
	BuiltinClick  = ids.AssetID{Hi: builtinHi, Lo: 0xa02}
	BuiltinHum    = ids.AssetID{Hi: builtinHi, Lo: 0xa03}
	BuiltinMelody = ids.AssetID{Hi: builtinHi, Lo: 0xa04}
)

type builtinSound struct {
	id       ids.AssetID
	name     string
	bus      string
	loopable bool
	synth    func() []float32
}

var builtinSounds = [...]builtinSound{
	{BuiltinBeep, "Beep", "sfx", false, synthBeep},
	{BuiltinClick, "Click", "ui", false, synthClick},
	{BuiltinHum, "Hum", "ambience", true, synthHum},
	{BuiltinMelody, "Melody", "music", true, synthMelody},
}

var melodyChords = [4][4]int{{45, 57, 60, 64}, {41, 57, 60, 65}, {48, 55, 60, 64}, {43, 55, 59, 62}}

var melodyBells = [...][3]float64{{0, 76, 0.9}, {1.5, 74, 0.6}, {2, 72, 0.8}, {4, 77, 0.9}, {6, 76, 0.7}, {8, 79, 0.9}, {9.5, 76, 0.6}, {10, 72, 0.8}, {12, 74, 0.9}, {14, 71, 0.7}}

var melodyArp = [8]int{1, 2, 3, 2, 1, 2, 3, 2}

var builtinAudioData [len(builtinSounds)]struct {
	once sync.Once
	data []byte
}

func BuiltinAudio(id ids.AssetID) ([]byte, bool) {
	if id.Hi != builtinHi || id.Lo < BuiltinBeep.Lo || id.Lo > BuiltinMelody.Lo {
		return nil, false
	}
	k := id.Lo - BuiltinBeep.Lo
	c := &builtinAudioData[k]
	c.once.Do(func() { c.data = buildBuiltinAudio(builtinSounds[k]) })
	return c.data, true
}

func BuiltinAudioNamed(name string) (ids.AssetID, bool) {
	for _, b := range builtinSounds {
		if b.name == name {
			return b.id, true
		}
	}
	return ids.AssetID{}, false
}

func BuiltinAudioNames() []string {
	out := make([]string, len(builtinSounds))
	for i, b := range builtinSounds {
		out[i] = b.name
	}
	return out
}

func builtinAudioEntries() []Builtin {
	out := make([]Builtin, len(builtinSounds))
	for i, b := range builtinSounds {
		out[i] = Builtin{ID: b.id, Name: "engine/audio/" + strings.ToLower(b.name), Kind: ArtAudio}
	}
	return out
}

func buildBuiltinAudio(b builtinSound) []byte {
	pcm := b.synth()
	frames := int64(len(pcm))
	a := &AudioArtifact{
		Format: AudioWAV, Rate: builtinAudioRate, Channels: 1, Frames: frames,
		Gain: 1, Bus: b.bus, Data: pcm16WAV(pcm, builtinAudioRate),
	}
	if b.loopable {
		a.LoopEnd = frames
	}
	return EncodeAudio(a)
}

func pcm16WAV(pcm []float32, rate int) []byte {
	n := len(pcm) * 2
	out := make([]byte, 44+n)
	le := binary.LittleEndian
	copy(out[0:], "RIFF")
	le.PutUint32(out[4:], uint32(36+n))
	copy(out[8:], "WAVEfmt ")
	le.PutUint32(out[16:], 16)
	le.PutUint16(out[20:], 1)
	le.PutUint16(out[22:], 1)
	le.PutUint32(out[24:], uint32(rate))
	le.PutUint32(out[28:], uint32(rate*2))
	le.PutUint16(out[32:], 2)
	le.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	le.PutUint32(out[40:], uint32(n))
	for i, s := range pcm {
		s = min(max(s, -1), 1)
		le.PutUint16(out[44+i*2:], uint16(int16(math.Round(float64(s)*32767))))
	}
	return out
}

func auFrames(sec float64) int {
	return int(sec * builtinAudioRate)
}

func auTime(i int) float64 {
	return float64(i) / builtinAudioRate
}

func auSine(f, t float64) float64 {
	return math.Sin(2 * math.Pi * f * t)
}

func auEdge(i, n, edge int) float64 {
	switch {
	case i < edge:
		return float64(i) / float64(edge)
	case n-i < edge:
		return float64(n-i) / float64(edge)
	}
	return 1
}

func auMidi(n int) float64 {
	return 440 * math.Pow(2, float64(n-69)/12)
}

func synthBeep() []float32 {
	n := auFrames(0.15)
	out := make([]float32, n)
	for i := range out {
		t := auTime(i)
		env := math.Exp(-t/0.045) * auEdge(i, n, auFrames(0.004))
		out[i] = float32(0.5 * env * (auSine(880, t) + 0.25*auSine(1760, t)))
	}
	return out
}

func synthClick() []float32 {
	n := auFrames(0.035)
	out := make([]float32, n)
	seed := uint32(0x9e3779b9)
	lp := 0.0
	for i := range out {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		noise := float64(seed)/math.MaxUint32*2 - 1
		lp += 0.35 * (noise - lp)
		out[i] = float32(0.8 * lp * math.Exp(-auTime(i)/0.006) * auEdge(i, n, auFrames(0.001)))
	}
	return out
}

func synthHum() []float32 {
	n := auFrames(2)
	out := make([]float32, n)
	for i := range out {
		t := auTime(i)
		tone := 0.6*auSine(110, t) + 0.3*auSine(165, t) + 0.2*auSine(220, t)
		out[i] = float32(0.3 * tone * (0.8 + 0.2*auSine(1, t)))
	}
	return out
}

// synthMelody is four bars of pad, plucked arpeggio, bass and bells; voices wrap past the end, so the loop has no seam.
func synthMelody() []float32 {
	const beat = 0.5
	n := auFrames(16 * beat)
	dry, wet := make([]float64, n), make([]float64, n)
	add := func(buf []float64, i int, v float64) { buf[i%n] += v }
	seed := uint32(0x2545f491)
	for bar, c := range melodyChords {
		at := auFrames(float64(bar*4) * beat)
		for _, k := range c[1:] {
			auPad(add, dry, wet, at, auMidi(k), 4*beat)
		}
		for step := range 8 {
			k := c[melodyArp[step]] + 12
			auPluck(add, dry, wet, at+auFrames(float64(step)*beat/2), auMidi(k), 0.13, &seed)
		}
		for half := range 2 {
			auBass(add, dry, at+auFrames(float64(half*2)*beat), auMidi(c[0]), 2*beat)
		}
	}
	for _, b := range melodyBells {
		auBell(add, dry, wet, auFrames(b[0]*beat), auMidi(int(b[1])), 0.12*b[2])
	}
	room := auReverb(wet)
	out := make([]float32, n)
	peak := 0.0
	for i := range dry {
		dry[i] += room[i]
		peak = math.Max(peak, math.Abs(dry[i]))
	}
	for i, v := range dry {
		out[i] = float32(0.7 * math.Tanh(1.3*v/peak) / math.Tanh(1.3))
	}
	return out
}

type auAdd func(buf []float64, i int, v float64)

func auSaw(p, dt float64) float64 {
	v := 2*p - 1
	switch {
	case p < dt:
		x := p / dt
		v -= x + x - x*x - 1
	case p > 1-dt:
		x := (p - 1) / dt
		v -= x*x + x + x + 1
	}
	return v
}

func auPad(add auAdd, dry, wet []float64, at int, f, dur float64) {
	n := auFrames(dur + 0.8)
	var ph [2]float64
	lp1, lp2 := 0.0, 0.0
	a := 1 - math.Exp(-2*math.Pi*900/builtinAudioRate)
	for j := range n {
		t := auTime(j)
		v := 0.0
		for k, d := range [2]float64{-0.005, 0.005} {
			fk := f * (1 + d)
			ph[k] += fk / builtinAudioRate
			ph[k] -= math.Floor(ph[k])
			v += auSaw(ph[k], fk/builtinAudioRate)
		}
		lp1 += a * (v - lp1)
		lp2 += a * (lp1 - lp2)
		env := math.Min(t/0.6, 1)
		if t > dur {
			env *= math.Exp(-(t - dur) / 0.25)
		}
		y := 0.05 * lp2 * env
		add(dry, at+j, y)
		add(wet, at+j, y)
	}
}

// auPluck is a Karplus–Strong string with a fractional delay, so high notes stay in tune.
func auPluck(add auAdd, dry, wet []float64, at int, f, gain float64, seed *uint32) {
	n := auFrames(1.2)
	period := builtinAudioRate/f - 0.5
	whole := int(period)
	frac := period - float64(whole)
	y := make([]float64, n)
	lp := 0.0
	for j := range n {
		x := 0.0
		if j <= whole {
			*seed ^= *seed << 13
			*seed ^= *seed >> 17
			*seed ^= *seed << 5
			lp += 0.4 * (float64(*seed)/math.MaxUint32*2 - 1 - lp)
			x = lp
		}
		if j > whole+1 {
			a := (1-frac)*y[j-whole] + frac*y[j-whole-1]
			b := (1-frac)*y[j-whole-1] + frac*y[j-whole-2]
			x += 0.997 * 0.5 * (a + b)
		}
		y[j] = x
		v := gain * x * auEdge(j, n, auFrames(0.02))
		add(dry, at+j, v)
		add(wet, at+j, 0.6*v)
	}
}

func auBass(add auAdd, dry []float64, at int, f, dur float64) {
	n := auFrames(dur)
	for j := range n {
		t := auTime(j)
		env := math.Min(t/0.01, 1) * (0.6 + 0.4*math.Exp(-t/0.3)) * auEdge(j, n, auFrames(0.05))
		add(dry, at+j, 0.11*env*(auSine(f, t)+0.25*auSine(2*f, t)))
	}
}

func auBell(add auAdd, dry, wet []float64, at int, f, gain float64) {
	n := auFrames(2.5)
	for j := range n {
		t := auTime(j)
		v := gain * (auSine(f, t)*math.Exp(-t/0.9) + 0.35*auSine(2.76*f, t)*math.Exp(-t/0.25) + 0.15*auSine(5.4*f, t)*math.Exp(-t/0.08)) * math.Min(t/0.002, 1) * auEdge(j, n, auFrames(0.05))
		add(dry, at+j, v)
		add(wet, at+j, 0.8*v)
	}
}

// auReverb is a one-channel Freeverb, run over two cycles of the loop so its tail wraps into the start.
func auReverb(in []float64) []float64 {
	n := len(in)
	out := make([]float64, n)
	var combs [4][]float64
	var ci [4]int
	var cf [4]float64
	for k, l := range [4]int{1557, 1617, 1491, 1422} {
		combs[k] = make([]float64, l*builtinAudioRate/44100)
	}
	var aps [2][]float64
	var ai [2]int
	for k, l := range [2]int{556, 341} {
		aps[k] = make([]float64, l*builtinAudioRate/44100)
	}
	for i := range 2 * n {
		x := 0.03 * in[i%n]
		y := 0.0
		for k := range combs {
			o := combs[k][ci[k]]
			cf[k] = o*0.6 + cf[k]*0.4
			combs[k][ci[k]] = x + cf[k]*0.85
			ci[k] = (ci[k] + 1) % len(combs[k])
			y += o
		}
		for k := range aps {
			o := aps[k][ai[k]]
			aps[k][ai[k]] = y + o*0.5
			y = o - y
			ai[k] = (ai[k] + 1) % len(aps[k])
		}
		if i >= n {
			out[i-n] = 1.5 * y
		}
	}
	return out
}
