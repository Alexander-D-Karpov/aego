package curve

import (
	"math"

	"aego/core/mathx"
)

type Easing uint8

const (
	None Easing = iota
	Linear
	InQuad
	OutQuad
	InOutQuad
	InCubic
	OutCubic
	InOutCubic
	InSine
	OutSine
	InOutSine
	InExpo
	OutExpo
	InOutExpo
	InBack
	OutBack
	InOutBack
	InElastic
	OutElastic
	InOutElastic
	InBounce
	OutBounce
	InOutBounce
	EasingCount
)

var easingNames = [EasingCount]string{
	"none", "linear",
	"inQuad", "outQuad", "inOutQuad",
	"inCubic", "outCubic", "inOutCubic",
	"inSine", "outSine", "inOutSine",
	"inExpo", "outExpo", "inOutExpo",
	"inBack", "outBack", "inOutBack",
	"inElastic", "outElastic", "inOutElastic",
	"inBounce", "outBounce", "inOutBounce",
}

func (e Easing) String() string {
	if e < EasingCount {
		return easingNames[e]
	}
	return "none"
}

func ParseEasing(s string) (Easing, bool) {
	for i, n := range easingNames {
		if n == s {
			return Easing(i), true
		}
	}
	return None, false
}

const (
	back1    = 1.70158
	back2    = back1 * 1.525
	back3    = back1 + 1
	elastic1 = 2 * math.Pi / 3
	elastic2 = 2 * math.Pi / 4.5
)

func exp2(x float32) float32 {
	return float32(math.Exp2(float64(x)))
}

func bounce(t float32) float32 {
	const n, d = 7.5625, 2.75
	switch {
	case t < 1/d:
		return n * t * t
	case t < 2/d:
		t -= 1.5 / d
		return n*t*t + 0.75
	case t < 2.5/d:
		t -= 2.25 / d
		return n*t*t + 0.9375
	}
	t -= 2.625 / d
	return n*t*t + 0.984375
}

func Ease(e Easing, t float32) float32 {
	if t != t {
		return 0
	}
	t = mathx.Clamp(t, 0, 1)
	switch e {
	case InQuad:
		return t * t
	case OutQuad:
		u := 1 - t
		return 1 - u*u
	case InOutQuad:
		if t < 0.5 {
			return 2 * t * t
		}
		u := 2 - 2*t
		return 1 - u*u/2
	case InCubic:
		return t * t * t
	case OutCubic:
		u := 1 - t
		return 1 - u*u*u
	case InOutCubic:
		if t < 0.5 {
			return 4 * t * t * t
		}
		u := 2 - 2*t
		return 1 - u*u*u/2
	case InSine:
		return 1 - mathx.Cos(t*mathx.Pi/2)
	case OutSine:
		return mathx.Sin(t * mathx.Pi / 2)
	case InOutSine:
		return (1 - mathx.Cos(mathx.Pi*t)) / 2
	case InExpo:
		if t == 0 {
			return 0
		}
		return exp2(10*t - 10)
	case OutExpo:
		if t == 1 {
			return 1
		}
		return 1 - exp2(-10*t)
	case InOutExpo:
		switch {
		case t == 0, t == 1:
			return t
		case t < 0.5:
			return exp2(20*t-10) / 2
		}
		return (2 - exp2(10-20*t)) / 2
	case InBack:
		return back3*t*t*t - back1*t*t
	case OutBack:
		u := t - 1
		return 1 + back3*u*u*u + back1*u*u
	case InOutBack:
		if t < 0.5 {
			u := 2 * t
			return u * u * ((back2+1)*u - back2) / 2
		}
		u := 2*t - 2
		return (u*u*((back2+1)*u+back2) + 2) / 2
	case InElastic:
		if t == 0 || t == 1 {
			return t
		}
		return -exp2(10*t-10) * mathx.Sin((10*t-10.75)*elastic1)
	case OutElastic:
		if t == 0 || t == 1 {
			return t
		}
		return exp2(-10*t)*mathx.Sin((10*t-0.75)*elastic1) + 1
	case InOutElastic:
		switch {
		case t == 0, t == 1:
			return t
		case t < 0.5:
			return -exp2(20*t-10) * mathx.Sin((20*t-11.125)*elastic2) / 2
		}
		return exp2(10-20*t)*mathx.Sin((20*t-11.125)*elastic2)/2 + 1
	case InBounce:
		return 1 - bounce(1-t)
	case OutBounce:
		return bounce(t)
	case InOutBounce:
		if t < 0.5 {
			return (1 - bounce(1-2*t)) / 2
		}
		return (1 + bounce(2*t-1)) / 2
	}
	return t
}
