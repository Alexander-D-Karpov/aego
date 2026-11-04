package postfx

import (
	"aego/core/ids"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
	"fmt"
	"strings"
)

type Profile struct {
	ReducedMotion bool
	LinearOutput  bool
	Transparent   bool
}

type State uint8

const (
	Active State = iota
	Disabled
	Neutral
	Unsupported
	Unknown
	Invalid
)

type Status struct {
	Phase      Phase
	passIndex  int
	TypeID     string
	InstanceID string
	State      State
	Reason     string
}

type Pass struct {
	LUTID, MaterialID ids.AssetID
	LUT               gpu.TextureHandle
	LUTSize           int
	Material          *material.Material
	Descriptor        *Descriptor
	InstanceID        string
	Params            [4][4]float32
}

// MaterialSource reads a prepared material, including its declared colour phase.
type MaterialSource interface {
	Material(ids.AssetID) (*material.Material, bool)
}

func (d *Descriptor) materialFor(e *Instance, src MaterialSource) *material.Material {
	if !d.UsesMaterial || src == nil || e.Material.ID.IsZero() {
		return nil
	}
	m, _ := src.Material(e.Material.ID)
	return m
}

// Phase returns the phase declared by a Custom material. Older postfx shaders
// and newly added, unassigned Custom instances use the display-linear phase.
func (p Pass) Phase() Phase {
	if p.Descriptor.UsesMaterial && p.Material != nil {
		s := p.Material.Shader()
		if s != nil && s.Domain == shader.DomainPostFX && s.Features&shader.FeaturePostSceneLinear != 0 {
			return SceneLinear
		}
	}
	return p.Descriptor.Phase
}

func (d *Descriptor) PhaseFor(e *Instance, src MaterialSource) Phase {
	return (Pass{Descriptor: d, Material: d.materialFor(e, src)}).Phase()
}

type Plan struct {
	Passes []Pass
	States []Status
}

// Build reuses dst's storage. A rejected stack has no executable passes.
func (r *Registry) Build(dst *Plan, enabled bool, items []Instance, profile Profile) error {
	return r.BuildWithMaterials(dst, enabled, items, profile, nil)
}

func (r *Registry) BuildWithMaterials(dst *Plan, enabled bool, items []Instance, profile Profile, src MaterialSource) error {
	// Backing arrays are reused, but deleted effects must release material and string references.
	clear(dst.Passes)
	clear(dst.States)
	dst.Passes = dst.Passes[:0]
	dst.States = dst.States[:0]
	var first error
	var prevPhase Phase
	for i := range items {
		e := &items[i]
		s := Status{InstanceID: e.InstanceID, TypeID: e.TypeID}
		d, known := r.Lookup(e.TypeID)
		var mat *material.Material
		if known {
			mat = d.materialFor(e, src)
			s.Phase = (Pass{Descriptor: d, Material: mat}).Phase()
		}
		var why error
		switch {
		case strings.TrimSpace(e.InstanceID) == "" || strings.TrimSpace(e.TypeID) == "":
			why = fmt.Errorf("postfx: missing type or instance ID at index %d", i)
		case hasID(items[:i], e.InstanceID):
			why = fmt.Errorf("postfx: duplicate instance ID %q", e.InstanceID)
		case !enabled || !e.Enabled:
			s.State, s.Reason = Disabled, "disabled"
			if !known {
				s.State, s.Reason = Unknown, "effect type unavailable (disabled)"
			}
		case !known:
			why = fmt.Errorf("postfx: %s requires unavailable effect %q", e.InstanceID, e.TypeID)
		case d.Version != e.Version:
			why = fmt.Errorf("postfx: %s (%s): schema version %d, expected %d", e.InstanceID, d.ID, e.Version, d.Version)
		default:
			var p Pass
			p, why = d.compile(e)
			p.Material = mat
			switch {
			case why != nil:
				why = fmt.Errorf("postfx: %s (%s): %w", e.InstanceID, e.TypeID, why)
			case s.Phase < prevPhase:
				why = fmt.Errorf("postfx: %s (%s) crosses the display-linear phase boundary", e.InstanceID, e.TypeID)
			default:
				prevPhase = s.Phase
				switch {
				case profile.ReducedMotion && d.MotionBypass:
					s.State, s.Reason = Disabled, "reduced motion"
				case p.neutral():
					s.State, s.Reason = Neutral, "neutral parameters"
				case profile.LinearOutput && s.Phase == DisplayLinear:
					s.State, s.Reason = Unsupported, "display effect requires a display output"
				case profile.Transparent && d.Alpha == OpaqueOnly:
					s.State, s.Reason = Unsupported, "effect requires an opaque output"
				default:
					s.passIndex = len(dst.Passes)
					dst.Passes = append(dst.Passes, p)
				}
			}
		}
		if why != nil {
			s.State, s.Reason = Invalid, why.Error()
			if first == nil {
				first = why
			}
		}
		dst.States = append(dst.States, s)
	}
	if first != nil {
		clear(dst.Passes)
		dst.Passes = dst.Passes[:0]
	}
	return first
}

func (r *Registry) Validate(items []Instance) error {
	var plan Plan
	return r.Build(&plan, true, items, Profile{})
}

// ValidateForOutput is the strict counterpart of preview's per-effect bypass.
func (r *Registry) ValidateForOutput(enabled bool, items []Instance, profile Profile) error {
	var plan Plan
	if err := r.Build(&plan, enabled, items, profile); err != nil {
		return err
	}
	return plan.CheckOutput()
}

func (p *Plan) CheckOutput() error {
	for _, s := range p.States {
		if s.State == Unsupported {
			return fmt.Errorf("postfx: %s (%s): %s", s.InstanceID, s.TypeID, s.Reason)
		}
	}
	return nil
}

func (p Pass) neutral() bool {
	d := p.Descriptor
	return d.Neutral != nil && d.Neutral(p.Params) && (!d.LUT || p.LUTID.IsZero() || p.Params[0][2] == 0)
}

// UpdateParameters updates the uniforms of a structurally unchanged effect.
// False requests full validation for a changed shape, invalid value or bypass transition.
func (p *Plan) UpdateParameters(i int, d *Descriptor, e *Instance) bool {
	s := p.States[i]
	if s.State == Unknown || s.State == Disabled && !e.Enabled {
		return true
	}
	if s.State != Active && s.State != Neutral {
		return false
	}
	pass, err := d.compile(e)
	if err != nil || pass.neutral() != (s.State == Neutral) {
		return false
	}
	if s.State == Active {
		p.Passes[s.passIndex].Params = pass.Params
	}
	return true
}

func hasID(items []Instance, id string) bool {
	for i := range items {
		if items[i].InstanceID == id {
			return true
		}
	}
	return false
}

func (d *Descriptor) compile(e *Instance) (Pass, error) {
	p := Pass{Descriptor: d, InstanceID: e.InstanceID, LUTID: e.LUT.ID, MaterialID: e.Material.ID}
	if !e.LUT.ID.IsZero() && !d.LUT || !e.Material.ID.IsZero() && !d.UsesMaterial {
		return Pass{}, fmt.Errorf("unexpected effect asset reference")
	}
	if d.UsesMaterial && e.Material.ID.IsZero() {
		return Pass{}, fmt.Errorf("assign a postfx material")
	}
	write := func(f *Field, v [4]float32) { copy(p.Params[f.Slot][f.Lane:], v[:f.Kind.Lanes()]) }
	for i := range d.Fields {
		f := &d.Fields[i]
		v, _ := f.pack(f.Default)
		write(f, v)
	}
	var used uint32
	for _, param := range e.Parameters {
		index := -1
		for i := range d.Fields {
			if d.Fields[i].Name == param.Name {
				index = i
				break
			}
		}
		if index < 0 {
			return Pass{}, fmt.Errorf("unknown parameter %q", param.Name)
		}
		bit := uint32(1 << index)
		if used&bit != 0 {
			return Pass{}, fmt.Errorf("duplicate parameter %q", param.Name)
		}
		used |= bit
		f := &d.Fields[index]
		v, err := f.pack(param)
		if err != nil {
			return Pass{}, err
		}
		write(f, v)
	}
	return p, nil
}
