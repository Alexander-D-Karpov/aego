package camera

import "math"

func hash32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

func grad(seed uint32, i int32) float32 {
	return float32(hash32(seed^uint32(i)*0x9e3779b1)>>8)/float32(1<<23) - 1
}

func noise1(seed uint32, t float32) float32 {
	fl := float32(math.Floor(float64(t)))
	i := int32(fl)
	f := t - fl
	a := grad(seed, i) * f
	b := grad(seed, i+1) * (f - 1)
	u := f * f * f * (f*(f*6-15) + 10)
	return 2 * (a + (b-a)*u)
}

func seedOf(name string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	return hash32(h)
}
