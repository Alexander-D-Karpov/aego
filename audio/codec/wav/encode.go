package wav

import (
	"encoding/binary"
	"math"
)

type EncodeOptions struct {
	Bits      int
	Float     bool
	Loop      bool
	LoopStart int64
	LoopEnd   int64
}

func Encode(pcm []float32, rate, channels int, o EncodeOptions) []byte {
	bits := o.Bits
	switch {
	case o.Float:
		bits = 32
	case bits != 8 && bits != 24 && bits != 32:
		bits = 16
	}
	bps := bits / 8
	data := make([]byte, 0, len(pcm)*bps)
	for _, s := range pcm {
		if !o.Float {
			s = min(max(s, -1), 1)
		}
		switch {
		case o.Float:
			data = binary.LittleEndian.AppendUint32(data, math.Float32bits(s))
		case bps == 1:
			data = append(data, byte(int(math.Round(float64(s)*127))+128))
		case bps == 2:
			data = binary.LittleEndian.AppendUint16(data, uint16(int16(math.Round(float64(s)*32767))))
		case bps == 3:
			v := uint32(int32(math.Round(float64(s) * 8388607)))
			data = append(data, byte(v), byte(v>>8), byte(v>>16))
		default:
			data = binary.LittleEndian.AppendUint32(data, uint32(int32(math.Round(float64(s)*2147483647))))
		}
	}
	format := uint16(fmtPCM)
	if o.Float {
		format = fmtFloat
	}
	pad := len(data) & 1
	smpl := 0
	if o.Loop {
		smpl = 8 + 60
	}
	riff := 4 + 8 + 16 + smpl + 8 + len(data) + pad
	out := make([]byte, 0, riff+8)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(riff))
	out = append(out, "WAVEfmt "...)
	out = binary.LittleEndian.AppendUint32(out, 16)
	out = binary.LittleEndian.AppendUint16(out, format)
	out = binary.LittleEndian.AppendUint16(out, uint16(channels))
	out = binary.LittleEndian.AppendUint32(out, uint32(rate))
	out = binary.LittleEndian.AppendUint32(out, uint32(rate*channels*bps))
	out = binary.LittleEndian.AppendUint16(out, uint16(channels*bps))
	out = binary.LittleEndian.AppendUint16(out, uint16(bits))
	if o.Loop {
		out = append(out, "smpl"...)
		out = binary.LittleEndian.AppendUint32(out, 60)
		for i := 0; i < 7; i++ {
			out = binary.LittleEndian.AppendUint32(out, 0)
		}
		out = binary.LittleEndian.AppendUint32(out, 1)
		out = binary.LittleEndian.AppendUint32(out, 0)
		out = binary.LittleEndian.AppendUint32(out, 0)
		out = binary.LittleEndian.AppendUint32(out, 0)
		out = binary.LittleEndian.AppendUint32(out, uint32(o.LoopStart))
		out = binary.LittleEndian.AppendUint32(out, uint32(o.LoopEnd-1))
		out = binary.LittleEndian.AppendUint32(out, 0)
		out = binary.LittleEndian.AppendUint32(out, 0)
	}
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, data...)
	if pad == 1 {
		out = append(out, 0)
	}
	return out
}
