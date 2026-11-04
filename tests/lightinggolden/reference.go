package lightinggolden

import (
	"math"

	"aego/core/mathx"
	"aego/render/lighting"
)

// Surface is an independent CPU description of a rectangular test receiver.
// Basis contains its full local-to-world linear transform, column major.
type Surface struct {
	X, Y, Width, Height float64
	Basis               [4]float64
	Color               [4]float64 // linear, already premultiplied and vertex-quantized
	Normal              [3]float64
	Emission            [3]float64
	Lit, Additive       bool
	// NormalAt allows independently trimmed/frame-mapped normal images.
	NormalAt   func(u, v float64) [3]float64
	EmissionAt func(u, v float64) [3]float64
}

func WhiteSurface() Surface {
	return Surface{X: 16, Y: 16, Width: 24, Height: 24, Basis: [4]float64{1, 0, 0, 1}, Color: [4]float64{1, 1, 1, 1}, Normal: [3]float64{0, 0, 1}, Lit: true}
}

func PMA(c mathx.Color) [4]float64 {
	// Sprite tints are intentionally stored as normalized RGBA8 in the ABI.
	a := float64(c.A)
	return [4]float64{quant(float64(c.R) * a), quant(float64(c.G) * a), quant(float64(c.B) * a), quant(a)}
}

func normalize(v [3]float64) [3]float64 {
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l < 1e-10 {
		return [3]float64{0, 0, 1}
	}
	return [3]float64{v[0] / l, v[1] / l, v[2] / l}
}

// NormalReference performs the image-up to world-down conversion followed by
// the inverse transpose of the surface's model matrix. Size does not rotate or
// rescale a normal; nonuniform node scale does.
func NormalReference(rgb [3]byte, basis [4]float64, flipX, flipY bool) [3]float64 {
	x, y, z := float64(rgb[0])*2/255-1, -(float64(rgb[1])*2/255 - 1), float64(rgb[2])*2/255-1
	if flipX {
		x = -x
	}
	if flipY {
		y = -y
	}
	det := basis[0]*basis[3] - basis[1]*basis[2]
	if math.Abs(det) < 1e-8 {
		return [3]float64{0, 0, 1}
	}
	return normalize([3]float64{(basis[3]*x - basis[1]*y) / det, (-basis[2]*x + basis[0]*y) / det, z})
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }
func quant(x float64) float64   { return math.Floor(clamp01(x)*255+.5) / 255 }
func srgb(x float64) float64 {
	x = clamp01(x)
	if x <= .0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1/2.4) - .055
}

func smooth(a, b, x float64) float64 {
	if a == b {
		if x >= a {
			return 1
		}
		return 0
	}
	t := clamp01((x - a) / (b - a))
	return t * t * (3 - 2*t)
}

// Reference draws in submission order; it neither groups by material nor uses
// renderer batching, GPU packing, shadow triangles or the generated GLSL.
func Reference(in lighting.Input, background [4]float64, surfaces ...Surface) []byte {
	out := make([]byte, Size*Size*4)
	e := in.Environment
	if !in.HasEnvironment {
		e = lighting.DefaultEnvironment()
	}
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			p := background
			if e.ColorMode == lighting.LDR {
				for c := range p {
					p[c] = quant(p[c])
				}
			}
			wx, wy := float64(x)+.5, float64(y)+.5
			for _, s := range surfaces {
				b := s.Basis
				det := b[0]*b[3] - b[1]*b[2]
				if math.Abs(det) < 1e-8 {
					continue
				}
				dx, dy := wx-s.X, wy-s.Y
				lx, ly := (b[3]*dx-b[2]*dy)/det, (-b[1]*dx+b[0]*dy)/det
				u, v := lx/s.Width+.5, ly/s.Height+.5
				if u < 0 || u >= 1 || v < 0 || v >= 1 {
					continue
				}
				n := s.Normal
				if s.NormalAt != nil {
					n = s.NormalAt(u, v)
				}
				src := s.Color
				emission := s.Emission
				if s.EmissionAt != nil {
					emission = s.EmissionAt(u, v)
				}
				if s.Lit {
					var illumination [3]float64
					switch in.Debug {
					case lighting.DebugNormals:
						for c := 0; c < 3; c++ {
							src[c] = (n[c]*.5 + .5) * src[3]
						}
					case lighting.DebugShadows:
						visible := 1.0
						for _, l := range in.Lights {
							if l.Shadows {
								visible = math.Min(visible, shadowVisible(in, l, wx, wy))
							}
						}
						for c := 0; c < 3; c++ {
							src[c] = visible * src[3]
						}
					default:
						illumination = [3]float64{float64(e.Ambient.R * e.AmbientIntensity), float64(e.Ambient.G * e.AmbientIntensity), float64(e.Ambient.B * e.AmbientIntensity)}
						if in.Disabled {
							illumination = [3]float64{1, 1, 1}
						} else {
							for _, l := range in.Lights {
								energy := lightEnergy(in, l, wx, wy, n)
								illumination[0] += float64(l.Color.R) * energy
								illumination[1] += float64(l.Color.G) * energy
								illumination[2] += float64(l.Color.B) * energy
							}
						}
						for c := 0; c < 3; c++ {
							src[c] = src[c]*illumination[c] + emission[c]*src[3]
						}
					}
				}
				if s.Additive {
					for c := 0; c < 3; c++ {
						p[c] += src[c]
					}
				} else {
					for c := 0; c < 4; c++ {
						p[c] = src[c] + p[c]*(1-src[3])
					}
				}
				if e.ColorMode == lighting.LDR {
					for c := range p {
						p[c] = quant(p[c])
					}
				}
			}
			a := clamp01(p[3])
			exposure := float64(e.Exposure)
			if in.Debug != lighting.DebugLighting {
				exposure = 1
			}
			for c := 0; c < 3; c++ {
				straight := 0.0
				if a > 1e-8 {
					straight = math.Max(0, p[c]) / a * exposure
				}
				if e.ColorMode == lighting.HDR && in.Debug == lighting.DebugLighting {
					straight /= 1 + straight
				}
				out[(y*Size+x)*4+c] = byte(math.Floor(clamp01(srgb(straight)*a)*255 + .5))
			}
			out[(y*Size+x)*4+3] = byte(math.Floor(a*255 + .5))
		}
	}
	return out
}

func lightEnergy(in lighting.Input, l lighting.Light, x, y float64, n [3]float64) float64 {
	var direction [3]float64
	atten := 1.0
	if l.Type == lighting.Directional {
		ce := math.Cos(float64(l.Elevation))
		direction = [3]float64{-float64(l.Direction.X) * ce, -float64(l.Direction.Y) * ce, math.Sin(float64(l.Elevation))}
	} else {
		dx, dy := float64(l.Position.X)-x, float64(l.Position.Y)-y
		d := math.Hypot(dx, dy)
		r := float64(l.Range)
		if d >= r {
			return 0
		}
		atten = math.Pow(math.Max(0, 1-d/r), float64(l.Falloff))
		direction = normalize([3]float64{dx, dy, float64(l.Height)})
		if l.Type == lighting.Spot {
			cosine := 1.0
			if d > 1e-6 {
				cosine = (-dx*float64(l.Direction.X) - dy*float64(l.Direction.Y)) / d
			}
			atten *= smooth(math.Cos(float64(l.OuterAngle)/2), math.Cos(float64(l.InnerAngle)/2), cosine)
		}
		if l.Type == lighting.Sprite {
			if l.Cookie.IsZero() {
				return 0
			}
			// The test cookie is a white 2:1 image. Its own aspect is kept inside
			// the square range: letterbox areas carry zero intensity.
			u := (-dx*float64(l.Direction.X)-dy*float64(l.Direction.Y))/(2*r) + .5
			v := (dx*float64(l.Direction.Y)-dy*float64(l.Direction.X))/(2*r) + .5
			if u < 0 || u > 1 || v < .25 || v > .75 {
				return 0
			}
		}
		if l.Shadows {
			atten *= shadowVisible(in, l, x, y)
		}
	}
	dot := math.Max(0, n[0]*direction[0]+n[1]*direction[1]+n[2]*direction[2])
	return float64(l.Intensity) * atten * dot
}

func cross(ax, ay, bx, by float64) float64 { return ax*by - ay*bx }
func shadowVisible(in lighting.Input, l lighting.Light, x, y float64) float64 {
	px, py := float64(l.Position.X), float64(l.Position.Y)
	rx, ry := x-px, y-py
	for _, o := range in.Occluders {
		if o.Layers&l.ShadowCasterMask == 0 || o.Count < 2 {
			continue
		}
		pts := in.Points[o.First : o.First+o.Count]
		if o.Closed {
			inside := false
			for i, a := range pts {
				b := pts[(i+1)%len(pts)]
				ax, ay, bx, by := float64(a.X), float64(a.Y), float64(b.X), float64(b.Y)
				if (ay > py) != (by > py) && px < (bx-ax)*(py-ay)/(by-ay)+ax {
					inside = !inside
				}
			}
			if inside {
				return 0
			}
		}
		n := len(pts) - 1
		if o.Closed {
			n = len(pts)
		}
		for i := 0; i < n; i++ {
			a, b := pts[i], pts[(i+1)%len(pts)]
			sx, sy := float64(b.X-a.X), float64(b.Y-a.Y)
			ax, ay := float64(a.X)-px, float64(a.Y)-py
			den := cross(rx, ry, sx, sy)
			if math.Abs(den) < 1e-10 {
				continue
			}
			t, u := cross(ax, ay, sx, sy)/den, cross(ax, ay, rx, ry)/den
			if t > 0 && t < 1 && u >= 0 && u <= 1 {
				return 0
			}
		}
	}
	return 1
}
