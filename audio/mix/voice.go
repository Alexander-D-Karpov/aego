package mix

import "aego/audio/dsp"

type voiceState uint8

const (
	voiceFree voiceState = iota
	voicePlaying
	voicePaused
)

type voice struct {
	id        VoiceID
	state     voiceState
	mem       *Memory
	st        *Stream
	bus       int
	ch        int
	pos       float64
	ratio     float64
	step      float64
	pitch     float32
	gain      float32
	target    float32
	delta     float32
	ramp      int
	gl, gr    float32
	priority  int32
	age       uint64
	loop      bool
	stopping  bool
	ls, le    int64
	frames    int64
	starved   bool
	starvedAt uint64
}

func (v *voice) rampTo(target float32, frames float64) {
	v.target = target
	n := int(frames)
	if n < 1 {
		v.gain, v.ramp, v.delta = target, 0, 0
		return
	}
	v.ramp = n
	v.delta = (target - v.gain) / float32(n)
}

func (v *voice) next() float32 {
	if v.ramp > 0 {
		v.gain += v.delta
		v.ramp--
		if v.ramp == 0 {
			v.gain = v.target
		}
	}
	return v.gain
}

func (v *voice) setPitch(p float32) {
	if p <= 0 {
		p = 1
	}
	v.pitch = min(max(p, MinPitch), MaxPitch)
	v.step = float64(v.pitch) * v.ratio
}

func (v *voice) setPan(p float32) {
	v.gl, v.gr = dsp.Pan(v.ch, p)
}

func (v *voice) sample(i int64, c int) float32 {
	if v.loop && i >= v.le {
		i = v.ls + (i-v.ls)%(v.le-v.ls)
	}
	if i < 0 || i >= v.frames {
		return 0
	}
	return v.mem.PCM[int(i)*v.ch+c]
}
