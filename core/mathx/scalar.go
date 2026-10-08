package mathx

import "math"

const (
	Pi      float32 = math.Pi
	TwoPi   float32 = 2 * math.Pi
	Epsilon float32 = 1e-6
)

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~float32 | ~float64
}

func Clamp[T Number](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func Abs[T Number](v T) T {
	if v < 0 {
		return -v
	}
	return v
}

func Sign[T Number](v T) T {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func Lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

func Deg2Rad(d float32) float32 {
	return d * Pi / 180
}

func Rad2Deg(r float32) float32 {
	return r * 180 / Pi
}

func Sqrt(v float32) float32 {
	return float32(math.Sqrt(float64(v)))
}

func Sin(v float32) float32 {
	return float32(math.Sin(float64(v)))
}

func Cos(v float32) float32 {
	return float32(math.Cos(float64(v)))
}

func Atan2(y, x float32) float32 {
	return float32(math.Atan2(float64(y), float64(x)))
}

func NearlyEqual(a, b, eps float32) bool {
	return Abs(a-b) <= eps
}

func Tan(v float32) float32 {
	return float32(math.Tan(float64(v)))
}

func Asin(v float32) float32 {
	return float32(math.Asin(float64(Clamp(v, -1, 1))))
}

func Acos(v float32) float32 {
	return float32(math.Acos(float64(Clamp(v, -1, 1))))
}

func Inf() float32 {
	return float32(math.Inf(1))
}
