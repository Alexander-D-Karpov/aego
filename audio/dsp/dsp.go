package dsp

import "math"

const (
	clipKnee  = 0.5
	clipLimit = 1.25
	clipK     = 1 / (3 * (clipLimit - clipKnee) * (clipLimit - clipKnee))
)

func Hermite(y0, y1, y2, y3, t float32) float32 {
	c1 := 0.5 * (y2 - y0)
	c2 := y0 - 2.5*y1 + 2*y2 - 0.5*y3
	c3 := 0.5*(y3-y0) + 1.5*(y1-y2)
	return ((c3*t+c2)*t+c1)*t + y1
}

func SoftClip(x float32) float32 {
	a := x
	if a < 0 {
		a = -a
	}
	if a <= clipKnee {
		return x
	}
	y := float32(1)
	if a < clipLimit {
		u := a - clipKnee
		y = clipKnee + u - clipK*u*u*u
	}
	if x < 0 {
		return -y
	}
	return y
}

func Pan(channels int, pan float32) (float32, float32) {
	pan = min(max(pan, -1), 1)
	if channels == 1 {
		a := float64(pan+1) * math.Pi / 4
		return float32(math.Cos(a)), float32(math.Sin(a))
	}
	return min(1, 1-pan), min(1, 1+pan)
}

func Resample(in []float32, channels, from, to int) []float32 {
	if from == to || from <= 0 || to <= 0 || channels <= 0 {
		return in
	}
	frames := len(in) / channels
	if frames == 0 {
		return nil
	}
	outFrames := int(int64(frames) * int64(to) / int64(from))
	out := make([]float32, outFrames*channels)
	step := float64(from) / float64(to)
	at := func(i, c int) float32 {
		i = min(max(i, 0), frames-1)
		return in[i*channels+c]
	}
	for f := 0; f < outFrames; f++ {
		p := float64(f) * step
		i := int(p)
		t := float32(p - float64(i))
		for c := 0; c < channels; c++ {
			out[f*channels+c] = Hermite(at(i-1, c), at(i, c), at(i+1, c), at(i+2, c), t)
		}
	}
	return out
}
