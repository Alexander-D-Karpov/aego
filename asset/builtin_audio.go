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

var melodyNotes = [16]int{57, 60, 64, 67, 69, 67, 64, 60, 55, 59, 62, 65, 67, 65, 62, 59}

var melodyBass = [2]int{45, 43}

var (
	builtinAudioOnce sync.Once
	builtinAudioData map[ids.AssetID][]byte
)

func BuiltinAudio(id ids.AssetID) ([]byte, bool) {
	if id.Hi != builtinHi || id.Lo < BuiltinBeep.Lo || id.Lo > BuiltinMelody.Lo {
		return nil, false
	}
	builtinAudioOnce.Do(buildBuiltinAudio)
	data, ok := builtinAudioData[id]
	return data, ok
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

func buildBuiltinAudio() {
	builtinAudioData = make(map[ids.AssetID][]byte, len(builtinSounds))
	for _, b := range builtinSounds {
		pcm := b.synth()
		frames := int64(len(pcm))
		a := &AudioArtifact{
			Format: AudioWAV, Rate: builtinAudioRate, Channels: 1, Frames: frames,
			Gain: 1, Bus: b.bus, Data: pcm16WAV(pcm, builtinAudioRate),
		}
		if b.loopable {
			a.LoopEnd = frames
		}
		builtinAudioData[b.id] = EncodeAudio(a)
	}
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

func synthMelody() []float32 {
	n := auFrames(4)
	note, bass, edge := auFrames(0.25), auFrames(2), auFrames(0.01)
	out := make([]float32, n)
	for i := range out {
		k, j := i/note, i%note
		t := auTime(j)
		f := auMidi(melodyNotes[k%len(melodyNotes)])
		lead := (auSine(f, t) + 0.3*auSine(2*f, t)) * math.Exp(-t/0.12) * auEdge(j, note, edge)
		bk, bj := i/bass, i%bass
		bf := auMidi(melodyBass[bk%len(melodyBass)])
		low := auSine(bf, auTime(bj)) * math.Sin(math.Pi*float64(bj)/float64(bass))
		out[i] = float32(0.22*lead + 0.18*low)
	}
	return out
}
