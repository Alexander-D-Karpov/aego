package lighting

import (
	"errors"
	"math"
	"math/bits"

	"aego/core/mathx"
)

// MaxShadowVertices bounds a single light's generated mask geometry. Failure
// returns an empty result; a renderer must never publish a partial shadow mask.
const MaxShadowVertices = 262144

var (
	ErrShadowBudget   = errors.New("LGT008: shadow triangle budget exceeded")
	ErrShadowGeometry = errors.New("LGT004: invalid shadow contour or point range")
	ErrShadowLight    = errors.New("LGT001: shadow masks require a finite local light with positive range")
)

func shadowHash(h, v uint64) uint64          { return (h ^ v) * 1099511628211 }
func shadowFloat(h uint64, v float32) uint64 { return shadowHash(h, uint64(math.Float32bits(v))) }

// ShadowKey identifies a light-space mask independent of camera pose, color, intensity, shading height and caster extraction order.
// Geometry and presentation revisions are distinct inputs; device generation invalidates cached GPU layers after restore.
func ShadowKey(light *Light, input *Input, resolution int, deviceGeneration uint64) uint64 {
	h := uint64(14695981039346656037)
	if light == nil || input == nil {
		return h
	}
	h = shadowHash(h, deviceGeneration)
	h = shadowHash(h, uint64(resolution))
	h = shadowHash(h, uint64(light.Type))
	h = shadowHash(h, light.ShadowCasterMask)
	for _, v := range [...]float32{light.Position.X, light.Position.Y, light.Direction.X, light.Direction.Y, light.Range, light.ShadowSoftness} {
		h = shadowFloat(h, v)
	}
	var sum, xor, count uint64
	for i := range input.Occluders {
		o := &input.Occluders[i]
		if o.Layers&light.ShadowCasterMask == 0 {
			continue
		}
		points, ok := shadowPoints(input, o)
		if !ok {
			h = shadowHash(h, ^uint64(0))
			continue
		}
		bounds := shadowBounds(o, points)
		if !shadowBoundsOverlap(light, bounds) {
			continue
		}
		k := uint64(14695981039346656037)
		k = shadowHash(k, o.ID.Hi)
		k = shadowHash(k, o.ID.Lo)
		k = shadowHash(k, uint64(o.Part))
		k = shadowHash(k, o.GeometryRevision)
		k = shadowHash(k, o.TransformRevision)
		k = shadowHash(k, o.Layers)
		k = shadowHash(k, uint64(o.Cull))
		if o.Closed {
			k = shadowHash(k, 1)
		} else {
			k = shadowHash(k, 0)
		}
		k = shadowHash(k, uint64(o.Count))
		for _, v := range [...]float32{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
			k = shadowFloat(k, v)
		}
		sum += k
		xor ^= bits.RotateLeft64(k, int(k&63))
		count++
	}
	h = shadowHash(h, sum)
	h = shadowHash(h, xor)
	return shadowHash(h, count)
}

func shadowPoints(input *Input, o *Occluder) ([]mathx.Vec2, bool) {
	if o.First < 0 || o.Count < 2 || o.Closed && o.Count < 3 || o.First > len(input.Points) || o.Count > len(input.Points)-o.First {
		return nil, false
	}
	return input.Points[o.First : o.First+o.Count], true
}

func shadowBounds(o *Occluder, points []mathx.Vec2) mathx.AABB2 {
	if o.Bounds != (mathx.AABB2{}) || len(points) == 0 {
		return o.Bounds
	}
	// Direct kernel callers may omit prepared bounds. The scene adapter always
	// supplies them, so static shadow-key preparation does not walk all points.
	b := mathx.AABB2{Min: points[0], Max: points[0]}
	for _, p := range points[1:] {
		b.Min.X, b.Min.Y = min(b.Min.X, p.X), min(b.Min.Y, p.Y)
		b.Max.X, b.Max.Y = max(b.Max.X, p.X), max(b.Max.Y, p.Y)
	}
	return b
}

func shadowBoundsOverlap(l *Light, b mathx.AABB2) bool {
	px, py, r := float64(l.Position.X), float64(l.Position.Y), float64(l.Range)
	return float64(b.Min.X) <= px+r && float64(b.Max.X) >= px-r && float64(b.Min.Y) <= py+r && float64(b.Max.Y) >= py-r
}

type shadowPoint struct{ x, y float64 }

func shadowCross(a, b shadowPoint) float64 { return a.x*b.y - a.y*b.x }
func shadowRelative(p mathx.Vec2, l *Light) shadowPoint {
	return shadowPoint{float64(p.X) - float64(l.Position.X), float64(p.Y) - float64(l.Position.Y)}
}

func shadowContainsOrigin(points []mathx.Vec2, l *Light) bool {
	inside := false
	a := shadowRelative(points[len(points)-1], l)
	for _, p := range points {
		b := shadowRelative(p, l)
		if shadowCross(a, b) == 0 && min(a.x, b.x) <= 0 && max(a.x, b.x) >= 0 && min(a.y, b.y) <= 0 && max(a.y, b.y) >= 0 {
			return true
		}
		if (a.y > 0) != (b.y > 0) {
			x := a.x + (b.x-a.x)*(-a.y/(b.y-a.y))
			if x > 0 {
				inside = !inside
			}
		}
		a = b
	}
	return inside
}

func shadowArea(points []mathx.Vec2) float64 {
	o := points[0]
	sum := 0.0
	for i := 1; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		sum += (float64(a.X)-float64(o.X))*(float64(b.Y)-float64(o.Y)) - (float64(a.Y)-float64(o.Y))*(float64(b.X)-float64(o.X))
	}
	return sum
}

func fullShadow(dst []mathx.Vec2) []mathx.Vec2 {
	return append(dst[:0], mathx.Vec2{}, mathx.Vec2{X: 1}, mathx.Vec2{X: 1, Y: 1}, mathx.Vec2{}, mathx.Vec2{X: 1, Y: 1}, mathx.Vec2{Y: 1})
}

// BuildShadow writes an opaque triangle union in normalized [0,1] light-space (Y down). Closed contours containing/touching the light
// or open segments containing it yield a full mask to avoid direction-dependent singularities; zero-length edges and fully collapsed closed contours add nothing.
// Analytic infinite extrusion clips the light square against each segment's two radial planes and the far side of its edge plane, avoiding fixed-length extrusion holes arbitrarily close to an edge.
func BuildShadow(light *Light, input *Input, dst []mathx.Vec2) ([]mathx.Vec2, error) {
	dst = dst[:0]
	if light == nil || input == nil || light.Type == Directional || light.Type > Sprite || !finite(light.Range) || light.Range <= 0 || !finite(light.Position.X) || !finite(light.Position.Y) {
		return dst, ErrShadowLight
	}
	for i := range input.Occluders {
		o := &input.Occluders[i]
		if o.Layers&light.ShadowCasterMask == 0 {
			continue
		}
		points, ok := shadowPoints(input, o)
		if !ok || o.Cull > CullCounterClockwise {
			return dst[:0], ErrShadowGeometry
		}
		for _, p := range points {
			if !finite(p.X) || !finite(p.Y) {
				return dst[:0], ErrShadowGeometry
			}
		}
		if !shadowBoundsOverlap(light, shadowBounds(o, points)) {
			continue
		}
		if o.Closed {
			if shadowArea(points) == 0 {
				continue
			}
			if shadowContainsOrigin(points, light) {
				return fullShadow(dst), nil
			}
		}
		edges := len(points) - 1
		if o.Closed {
			edges++
		}
		for j := 0; j < edges; j++ {
			a, b := shadowRelative(points[j], light), shadowRelative(points[(j+1)%len(points)], light)
			if a == b {
				continue
			}
			cross := shadowCross(a, b)
			// Culling is defined by which side of the directed world edge sees
			// the light. Reflections naturally reverse that winding exactly once.
			if o.Cull == CullClockwise && cross < 0 || o.Cull == CullCounterClockwise && cross > 0 {
				continue
			}
			if cross == 0 {
				if min(a.x, b.x) <= 0 && max(a.x, b.x) >= 0 && min(a.y, b.y) <= 0 && max(a.y, b.y) >= 0 {
					return fullShadow(dst), nil
				}
				continue
			}
			r := float64(light.Range)
			a.x /= r
			a.y /= r
			b.x /= r
			b.y /= r
			var visible bool
			a, b, visible = clipShadowEdge(a, b)
			if !visible || a == b {
				continue
			}
			cross = shadowCross(a, b)
			if cross == 0 {
				continue
			}
			sign := float64(1)
			if cross < 0 {
				sign = -1
			}
			var aa, bb [12]shadowPoint
			aa[0], aa[1], aa[2], aa[3] = shadowPoint{-1, -1}, shadowPoint{1, -1}, shadowPoint{1, 1}, shadowPoint{-1, 1}
			poly := clipShadowPolygon(aa[:4], bb[:0], shadowPlane{sign * a.y * -1, sign * a.x, 0})
			poly = clipShadowPolygon(poly, aa[:0], shadowPlane{sign * b.y, -sign * b.x, 0})
			poly = clipShadowPolygon(poly, bb[:0], shadowPlane{sign * (b.y - a.y), -sign * (b.x - a.x), -sign * cross})
			if len(poly) < 3 {
				continue
			}
			if len(dst)+(len(poly)-2)*3 > MaxShadowVertices {
				return dst[:0], ErrShadowBudget
			}
			p0 := normalizedShadow(poly[0])
			for k := 1; k+1 < len(poly); k++ {
				p1, p2 := normalizedShadow(poly[k]), normalizedShadow(poly[k+1])
				if p0 == p1 || p0 == p2 || p1 == p2 {
					continue
				}
				dst = append(dst, p0, p1, p2)
			}
		}
	}
	return dst, nil
}

type shadowPlane struct{ x, y, c float64 }

func (p shadowPlane) distance(v shadowPoint) float64 { return p.x*v.x + p.y*v.y + p.c }

func clipShadowPolygon(src, dst []shadowPoint, p shadowPlane) []shadowPoint {
	dst = dst[:0]
	if len(src) == 0 {
		return dst
	}
	a := src[len(src)-1]
	da := p.distance(a)
	for _, b := range src {
		db := p.distance(b)
		if (da >= 0) != (db >= 0) {
			t := da / (da - db)
			dst = append(dst, shadowPoint{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t})
		}
		if db >= 0 {
			dst = append(dst, b)
		}
		a, da = b, db
	}
	return dst
}

func normalizedShadow(p shadowPoint) mathx.Vec2 {
	return mathx.Vec2{X: float32(max(0, min(1, .5+.5*p.x))), Y: float32(max(0, min(1, .5+.5*p.y)))}
}

func shadowOutcode(p shadowPoint) uint8 {
	var c uint8
	if p.x < -1 {
		c |= 1
	} else if p.x > 1 {
		c |= 2
	}
	if p.y < -1 {
		c |= 4
	} else if p.y > 1 {
		c |= 8
	}
	return c
}

// Cohen-Sutherland clips the finite caster edge before forming radial planes, assigning the crossing exactly to its square boundary to avoid huge extrusions or infinity in large-coordinate scenes.
func clipShadowEdge(a, b shadowPoint) (shadowPoint, shadowPoint, bool) {
	for i := 0; i < 12; i++ {
		ca, cb := shadowOutcode(a), shadowOutcode(b)
		if ca|cb == 0 {
			return a, b, true
		}
		if ca&cb != 0 {
			return a, b, false
		}
		c := ca
		if c == 0 {
			c = cb
		}
		var p shadowPoint
		switch {
		case c&8 != 0:
			p.y = 1
			p.x = a.x + (b.x-a.x)*(1-a.y)/(b.y-a.y)
		case c&4 != 0:
			p.y = -1
			p.x = a.x + (b.x-a.x)*(-1-a.y)/(b.y-a.y)
		case c&2 != 0:
			p.x = 1
			p.y = a.y + (b.y-a.y)*(1-a.x)/(b.x-a.x)
		default:
			p.x = -1
			p.y = a.y + (b.y-a.y)*(-1-a.x)/(b.x-a.x)
		}
		if c == ca {
			a = p
		} else {
			b = p
		}
	}
	return a, b, false
}
