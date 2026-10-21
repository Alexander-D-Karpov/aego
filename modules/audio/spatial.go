package audio

import (
	"aego/core/curve"
	"aego/core/mathx"
)

type Spatial struct {
	Min     float32
	Max     float32
	Width   float32
	Rolloff curve.Curve
}

var defaultSpatial = Spatial{Min: 1, Max: 10, Width: 0.8, Rolloff: DefaultRolloff()}

func (s *Spatial) At(listener, src mathx.Vec2) (gain, pan float32) {
	d := src.Sub(listener)
	dist := d.Len()
	var t float32
	switch {
	case dist >= s.Max:
		t = 1
	case dist <= s.Min || s.Max <= s.Min:
		t = 0
	default:
		t = (dist - s.Min) / (s.Max - s.Min)
	}
	gain = s.rolloff(t)
	if s.Max > 0 {
		pan = mathx.Clamp(d.X/s.Max, -1, 1) * s.Width
	}
	return gain, pan
}

func (s *Spatial) rolloff(t float32) float32 {
	c := &s.Rolloff
	switch {
	case c.Ease != curve.None:
		return 1 - curve.Ease(c.Ease, t)
	case len(c.Keys) == 0:
		return 1 - t
	}
	t0, t1 := c.Span()
	return mathx.Clamp(c.EvalClamped(t0+(t1-t0)*t), 0, 1)
}
