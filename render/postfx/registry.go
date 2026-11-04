package postfx

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/shader"
)

const ProvisionKind = "postfx.effect"

type AssetRef struct{ ID ids.AssetID }

func (r *AssetRef) AssetRefID() ids.AssetID      { return r.ID }
func (r *AssetRef) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (*AssetRef) AssetKind() string              { return "texture" }

type Parameter struct {
	Name    string      `aego:"name"`
	Kind    shader.Kind `aego:"kind"`
	Value   mathx.Vec4  `aego:"value,omitempty"`
	Integer int32       `aego:"integer,omitempty"`
	Boolean bool        `aego:"boolean,omitempty"`
	Asset   AssetRef    `aego:"asset,asset=texture,omitempty"`
}

type MaterialRef struct{ ID ids.AssetID }

func (r *MaterialRef) AssetRefID() ids.AssetID      { return r.ID }
func (r *MaterialRef) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (*MaterialRef) AssetKind() string              { return "material" }

type Instance struct {
	LUT        AssetRef    `aego:"lut,asset=texture,omitempty"`
	Material   MaterialRef `aego:"material,asset=material,omitempty"`
	TypeID     string      `aego:"typeID"`
	InstanceID string      `aego:"instanceID"`
	Enabled    bool        `aego:"enabled"`
	Version    int32       `aego:"version"`
	Parameters []Parameter `aego:"parameters"`
}

type Phase uint8

const (
	SceneLinear Phase = iota
	DisplayLinear
)

type AlphaPolicy uint8

const (
	PreserveCoverage AlphaPolicy = iota
	OpaqueOnly
	ResampleCoverage
)

type Field struct {
	Packed     bool
	Slot, Lane int
	Name       string
	Kind       shader.Kind
	Default    Parameter
	Min        float32
	Max        float32
	Bounded    bool
}

type Descriptor struct {
	// WritesAllPixels promises the fragment writes every covered pixel without discard.
	WritesAllPixels bool
	// Outside the auxiliary support this effect returns the original scene color.
	PreservesOutsideAux bool
	// AuxiliarySupport bounds all auxiliary RGB output to the non-background
	// input expanded by the returned number of full-resolution pixels. Outside
	// that support the reference filter must produce opaque black.
	// CenterAuxSample promises that tex1 is read only by aego_aux(uv) at
	// the pixel's own local UV. Its final pass reserves phases.zw and
	// auxInfo.zw for the renderer's half-resolution sampling window.
	CenterAuxSample bool
	// LumaThreshold opts in to the same support bound with only producers
	// capable of exceeding this linear Rec.709 luminance.
	LumaThreshold                   func([4][4]float32) float32
	AuxiliarySupport                func(mathx.Color, [4][4]float32) (float32, bool)
	LUT, UsesMaterial, MotionBypass bool
	// HalfPasses describe a bounded separable filter chain. The original input
	// survives until the final composite and may be sampled there along with tex1.
	// HalfReadOffset opts in only when every auxiliary shader fully replaces its
	// destination. It bounds a step's sample displacement from the current
	// pixel in full-resolution units, before linear filtering.
	HalfReadOffset func(step int, params [4][4]float32) (x, y float32)
	HalfPasses     []string
	half           []*Descriptor
	ID             string
	Label          string
	Version        int32
	Phase          Phase
	Alpha          AlphaPolicy
	Fields         []Field
	Neutral        func([4][4]float32) bool
	Fragment       string
}

type Registry struct {
	byID map[string]*Descriptor
	list []*Descriptor
}

func NewRegistry() *Registry {
	r := &Registry{}
	for _, d := range append([]Descriptor{vignette()}, effects()...) {
		if err := r.Register(d); err != nil {
			panic(err)
		}
	}
	return r
}

func (r *Registry) Register(d Descriptor) error {
	if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Label) == "" || d.Version <= 0 || d.Phase > DisplayLinear || d.Alpha > ResampleCoverage || strings.TrimSpace(d.Fragment) == "" {
		return fmt.Errorf("postfx: invalid descriptor %q", d.ID)
	}
	if _, exists := r.byID[d.ID]; exists {
		return fmt.Errorf("postfx: duplicate effect type %q", d.ID)
	}
	if len(d.Fields) > 16 || len(d.HalfPasses) > 4 || (d.LUT && len(d.HalfPasses) > 0) {
		return fmt.Errorf("postfx: %s exceeds pass parameter capacity", d.ID)
	}
	d.Fields = slices.Clone(d.Fields)
	d.HalfPasses = slices.Clone(d.HalfPasses)
	d.half = nil
	for i, frag := range d.HalfPasses {
		if strings.TrimSpace(frag) == "" {
			return fmt.Errorf("empty auxiliary pass")
		}
		d.half = append(d.half, &Descriptor{ID: fmt.Sprintf("%s.half.%d", d.ID, i), Phase: d.Phase, Alpha: d.Alpha, Fragment: frag})
	}
	var lanes [4][4]bool
	for i := range d.Fields {
		f := &d.Fields[i]
		if f.Name == "" || f.Kind >= shader.KindTexture || f.Bounded && (!finite(f.Min) || !finite(f.Max) || f.Min > f.Max) {
			return fmt.Errorf("postfx: %s has invalid or unsupported field %q", d.ID, f.Name)
		}
		if !f.Packed {
			f.Slot, f.Lane = i, 0
		}
		if f.Slot < 0 || f.Slot >= 4 || f.Lane < 0 || f.Lane+f.Kind.Lanes() > 4 {
			return fmt.Errorf("invalid packing for %s", f.Name)
		}
		for l := f.Lane; l < f.Lane+f.Kind.Lanes(); l++ {
			if lanes[f.Slot][l] {
				return fmt.Errorf("overlapping packing for %s", f.Name)
			}
			lanes[f.Slot][l] = true
		}
		for _, prev := range d.Fields[:i] {
			if f.Name == prev.Name {
				return fmt.Errorf("postfx: %s has duplicate field %q", d.ID, f.Name)
			}
		}
		f.Default.Name, f.Default.Kind = f.Name, f.Kind
		if _, err := f.pack(f.Default); err != nil {
			return fmt.Errorf("postfx: %s default: %w", d.ID, err)
		}
	}
	if r.byID == nil {
		r.byID = make(map[string]*Descriptor)
	}
	r.byID[d.ID] = &d
	r.list = append(r.list, &d)
	return nil
}

// Descriptors and their fields are read-only once registered.
func (r *Registry) Lookup(typeID string) (*Descriptor, bool) {
	d, ok := r.byID[typeID]
	return d, ok
}

func (r *Registry) Revision() uint64 { return uint64(len(r.list)) }

func (r *Registry) Descriptors() []*Descriptor { return slices.Clone(r.list) }

func (r *Registry) NewInstance(typeID, instanceID string) (Instance, error) {
	d, ok := r.Lookup(typeID)
	if !ok {
		return Instance{}, fmt.Errorf("postfx: unknown effect type %q", typeID)
	}
	if strings.TrimSpace(instanceID) == "" {
		return Instance{}, fmt.Errorf("postfx: empty instance ID")
	}
	e := Instance{TypeID: typeID, InstanceID: instanceID, Version: d.Version, Enabled: !d.UsesMaterial, Parameters: make([]Parameter, len(d.Fields))}
	for i, f := range d.Fields {
		e.Parameters[i] = f.Default
	}
	return e, nil
}

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func (f *Field) Validate(p Parameter) error {
	if p.Name != f.Name {
		return fmt.Errorf("expected parameter %s, got %q", f.Name, p.Name)
	}
	_, err := f.pack(p)
	return err
}

func (f *Field) pack(p Parameter) ([4]float32, error) {
	var out [4]float32
	if p.Kind != f.Kind {
		return out, fmt.Errorf("parameter %s requires %s, got %s", f.Name, f.Kind.GLSL(), p.Kind.GLSL())
	}
	switch f.Kind {
	case shader.KindInt:
		if int64(float32(p.Integer)) != int64(p.Integer) {
			return out, fmt.Errorf("parameter %s cannot represent %d in a pass uniform", f.Name, p.Integer)
		}
		out[0] = float32(p.Integer)
	case shader.KindBool:
		if p.Boolean {
			out[0] = 1
		}
	default:
		a := [4]float32{p.Value.X, p.Value.Y, p.Value.Z, p.Value.W}
		copy(out[:f.Kind.Lanes()], a[:])
	}
	for _, v := range out[:f.Kind.Lanes()] {
		if !finite(v) || f.Bounded && (v < f.Min || v > f.Max) {
			return out, fmt.Errorf("parameter %s has invalid value %g", f.Name, v)
		}
	}
	return out, nil
}

func (d *Descriptor) Auxiliary() []*Descriptor { return d.half }
