// Package lighting owns the render-time 2D lighting contract. Descriptors are
// presentation data; they neither reference scene nodes nor own simulation state.
package lighting

import (
	"errors"
	"math"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/texture"
)

const (
	MaxLights      = 8
	MaxShadows     = 16
	MaxInputLights = 1024
	MaxOccluders   = 4096
	MaxPoints      = 262144
)

type Type uint8

const (
	Point Type = iota
	Spot
	Directional
	Sprite
)

type ColorMode uint8

const (
	Auto ColorMode = iota
	LDR
	HDR
)

type DebugMode uint8

const (
	DebugLighting DebugMode = iota
	DebugNormals
	DebugShadows
)

type CullMode uint8

const (
	CullNone CullMode = iota
	CullClockwise
	CullCounterClockwise
)

type Environment struct {
	Ambient           mathx.Color
	AmbientIntensity  float32
	ColorMode         ColorMode
	Exposure          float32
	ShadowResolution  int
	MaxShadowedLights int
}

func DefaultEnvironment() Environment {
	return Environment{Ambient: mathx.Color{R: 1, G: 1, B: 1, A: 1}, AmbientIntensity: 1, Exposure: 1, ShadowResolution: 256, MaxShadowedLights: 8}
}

// Light direction is the transformed local +X axis; Height is independent of
// planar shadow extrusion. Directional uses Elevation above the receiving plane.
type Light struct {
	ID                                         ids.NodeID
	Type                                       Type
	Position                                   mathx.Vec2
	Direction                                  mathx.Vec2
	Color                                      mathx.Color
	Intensity, Range, Height                   float32
	InnerAngle, OuterAngle, Falloff, Elevation float32
	Cookie                                     texture.Region
	Layers, ShadowCasterMask                   uint64
	Shadows                                    bool
	ShadowSoftness                             float32
	Priority                                   int32
}

// Occluder points are in presentation-world coordinates; First/Count address Input.Points until the next fill, and geometry/transform revisions remain separate for cache invalidation.
type Occluder struct {
	Bounds                              mathx.AABB2
	ID                                  ids.NodeID
	Part                                uint32
	GeometryRevision, TransformRevision uint64
	First, Count                        int
	Closed                              bool
	Cull                                CullMode
	Layers                              uint64
}

type Input struct {
	Debug          DebugMode
	OutputLit      bool // Other views composing this output may contain Lit receivers.
	Lights         []Light
	Occluders      []Occluder
	Points         []mathx.Vec2
	Environment    Environment
	HasEnvironment bool
	Disabled       bool // View-only inspection override; never a material/scene mutation.
}

func (in *Input) Reset() {
	in.Lights = in.Lights[:0]
	in.Occluders = in.Occluders[:0]
	in.Points = in.Points[:0]
	in.Environment = DefaultEnvironment()
	in.HasEnvironment, in.Disabled = false, false
	in.Debug, in.OutputLit = DebugLighting, false
}

func finite(f float32) bool { return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0) }

func (l Light) Validate() error {
	if l.Type > Sprite {
		return errors.New("LGT001: unknown light type")
	}
	for _, f := range [...]float32{l.Position.X, l.Position.Y, l.Direction.X, l.Direction.Y, l.Color.R, l.Color.G, l.Color.B, l.Color.A, l.Intensity, l.Range, l.Height, l.InnerAngle, l.OuterAngle, l.Falloff, l.Elevation, l.ShadowSoftness} {
		if !finite(f) {
			return errors.New("LGT001: light contains NaN or infinity")
		}
	}
	if l.Intensity < 0 || l.Range < 0 || l.Height < 0 || l.Falloff < 0 || l.ShadowSoftness < 0 || l.Color.R < 0 || l.Color.G < 0 || l.Color.B < 0 {
		return errors.New("LGT001: light energy, dimensions and softness must be nonnegative")
	}
	if l.Type != Directional && l.Range <= 0 {
		return errors.New("LGT001: local light range must be positive")
	}
	if l.Type == Spot && (l.InnerAngle < 0 || l.OuterAngle <= 0 || l.InnerAngle > l.OuterAngle || l.OuterAngle > mathx.Pi) {
		return errors.New("LGT001: spot requires 0 <= innerAngle <= outerAngle <= pi")
	}
	if l.Type == Directional && (l.Elevation <= 0 || l.Elevation > mathx.Pi/2) {
		return errors.New("LGT001: directional elevation must be in (0, pi/2]")
	}
	if l.Type == Directional && l.Shadows {
		return errors.New("LGT002: directional shadows are not supported; use local light shadows")
	}
	return nil
}

func (e Environment) Validate() error {
	for _, f := range [...]float32{e.Ambient.R, e.Ambient.G, e.Ambient.B, e.Ambient.A, e.AmbientIntensity, e.Exposure} {
		if !finite(f) || f < 0 {
			return errors.New("LGT003: environment values must be finite and nonnegative")
		}
	}
	if e.ColorMode > HDR || e.Exposure <= 0 {
		return errors.New("LGT003: invalid color mode or exposure")
	}
	if e.ShadowResolution < 64 || e.ShadowResolution > 2048 || e.ShadowResolution&(e.ShadowResolution-1) != 0 {
		return errors.New("LGT003: shadow resolution must be a power of two between 64 and 2048")
	}
	if e.MaxShadowedLights < 0 || e.MaxShadowedLights > MaxShadows {
		return errors.New("LGT003: shadowed light budget must be between 0 and 16")
	}
	return nil
}

func LightBounds(l *Light) mathx.AABB2 {
	d := mathx.Vec2{X: l.Range, Y: l.Range}
	return mathx.AABB2{Min: l.Position.Sub(d), Max: l.Position.Add(d)}
}

// Affects is the conservative receiver test used by selection and budget
// diagnostics. Spot cones are evaluated per fragment after this range test.
func Affects(l *Light, bounds mathx.AABB2, layers uint64) bool {
	if l.Intensity <= 0 || l.Layers&layers == 0 {
		return false
	}
	if l.Type == Directional {
		return true
	}
	x := max(bounds.Min.X, min(l.Position.X, bounds.Max.X))
	y := max(bounds.Min.Y, min(l.Position.Y, bounds.Max.Y))
	dx, dy := l.Position.X-x, l.Position.Y-y
	return dx*dx+dy*dy <= l.Range*l.Range
}

// Set is the stable selection attached to a draw. Indices refer to the immutable
// current input, and never encode layer masks in floating point GPU values.
type Set struct {
	Indices [MaxLights]int
	Count   int
}

type candidate struct {
	index     int
	priority  int32
	influence float32
	id        ids.NodeID
}

func better(a, b candidate) bool {
	if a.priority != b.priority {
		return a.priority > b.priority
	}
	if a.influence != b.influence {
		return a.influence > b.influence
	}
	if a.id.Hi != b.id.Hi {
		return a.id.Hi < b.id.Hi
	}
	if a.id.Lo != b.id.Lo {
		return a.id.Lo < b.id.Lo
	}
	return a.index < b.index
}

// Select uses fixed-size insertion rather than map iteration and tests receiver bounds, so edge receivers can receive light even when their pivot is outside its range.
func Select(lights []Light, bounds mathx.AABB2, layers uint64) (Set, int) {
	var best [MaxLights]candidate
	n, eligible := 0, 0
	for i := range lights {
		l := &lights[i]
		if l.Intensity <= 0 || l.Layers&layers == 0 {
			continue
		}
		x := max(bounds.Min.X, min(l.Position.X, bounds.Max.X))
		y := max(bounds.Min.Y, min(l.Position.Y, bounds.Max.Y))
		dx, dy := l.Position.X-x, l.Position.Y-y
		d2 := dx*dx + dy*dy
		if l.Type != Directional && d2 > l.Range*l.Range {
			continue
		}
		energy := l.Intensity * max(l.Color.R, max(l.Color.G, l.Color.B))
		if l.Type != Directional {
			energy *= max(float32(0), 1-d2/(l.Range*l.Range))
		}
		c := candidate{i, l.Priority, energy, l.ID}
		eligible++
		at := n
		for at > 0 && better(c, best[at-1]) {
			at--
		}
		if at == MaxLights {
			continue
		}
		if n < MaxLights {
			n++
		}
		for j := n - 1; j > at; j-- {
			best[j] = best[j-1]
		}
		best[at] = c
	}
	var out Set
	out.Count = n
	for i := 0; i < n; i++ {
		out.Indices[i] = best[i].index
	}
	return out, max(0, eligible-n)
}
