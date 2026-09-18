package mathx

import "math"

type Color struct{ R, G, B, A float32 }

func ColorRGB8(r, g, b uint8) Color {
	return ColorRGBA8(r, g, b, 255)
}

func ColorRGBA8(r, g, b, a uint8) Color {
	return Color{
		R: SRGBToLinear(float32(r) / 255),
		G: SRGBToLinear(float32(g) / 255),
		B: SRGBToLinear(float32(b) / 255),
		A: float32(a) / 255,
	}
}

func SRGBToLinear(c float32) float32 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return float32(math.Pow((float64(c)+0.055)/1.055, 2.4))
}

func LinearToSRGB(c float32) float32 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return float32(1.055*math.Pow(float64(c), 1/2.4) - 0.055)
}

func (c Color) Premultiplied() Color {
	return Color{R: c.R * c.A, G: c.G * c.A, B: c.B * c.A, A: c.A}
}

func (c Color) Lerp(o Color, t float32) Color {
	return Color{
		R: Lerp(c.R, o.R, t),
		G: Lerp(c.G, o.G, t),
		B: Lerp(c.B, o.B, t),
		A: Lerp(c.A, o.A, t),
	}
}

func (c Color) PackRGBA8() uint32 {
	return uint32(unitToByte(c.R)) |
		uint32(unitToByte(c.G))<<8 |
		uint32(unitToByte(c.B))<<16 |
		uint32(unitToByte(c.A))<<24
}

func unitToByte(v float32) uint8 {
	return uint8(Clamp(v, 0, 1)*255 + 0.5)
}
