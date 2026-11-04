package render2d

import (
	"fmt"
	"math"
	"slices"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene"
	"aego/schema"
)

const MaxMaterialOverrides = 8

// MaterialParameter is an explicit node value. Kind is serialized so removing a
// parameter or changing its type cannot silently reinterpret the old value.
// Integer and Boolean are separate from vector storage to preserve int32 bits.
type MaterialParameter struct {
	Name    string      `aego:"name"`
	Kind    shader.Kind `aego:"kind"`
	Value   mathx.Vec4  `aego:"value"`
	Integer int32       `aego:"integer"`
	Boolean bool        `aego:"boolean"`
}

// MaterialOverride belongs to the node and is cloned with the scene. Values
// absent from this list inherit the shared material, including inherited defaults.
type MaterialOverride struct {
	Values []MaterialParameter `aego:"values,noscript"`
	lanes  [16]float32
}

// Mesh2D draws the engine mesh vertex layout in a 2D view. Sprite-only shaders
// are rejected instead of being bound to an incompatible vertex layout.
type Mesh2D struct {
	Mesh     asset.Ref[asset.Mesh]     `aego:"mesh,asset=mesh"`
	Texture  asset.Ref[asset.Texture]  `aego:"texture,asset=texture"`
	Material asset.Ref[asset.Material] `aego:"material,asset=material"`
	Tint     mathx.Color               `aego:"tint"`
	Layer    int32                     `aego:"layer"`
	Order    int32                     `aego:"order"`
}

func DefaultMesh2D() Mesh2D { return Mesh2D{Tint: mathx.Color{R: 1, G: 1, B: 1, A: 1}} }

func ParameterKind(p *shader.Param) schema.Kind {
	switch p.Kind {
	case shader.KindFloat:
		return schema.KindFloat32
	case shader.KindInt:
		return schema.KindInt32
	case shader.KindBool:
		return schema.KindBool
	case shader.KindVec2:
		return schema.KindVec2
	case shader.KindVec3:
		return schema.KindVec3
	case shader.KindVec4:
		if p.Color {
			return schema.KindColor
		}
		return schema.KindVec4
	case shader.KindTexture:
		return schema.KindAssetRef
	}
	return schema.KindFloat32
}

func ParameterValue(p *shader.Param, lanes [4]float32, integer int32) schema.Value {
	switch p.Kind {
	case shader.KindFloat:
		return schema.Float32(lanes[0])
	case shader.KindInt:
		return schema.Int32(integer)
	case shader.KindBool:
		return schema.Bool(lanes[0] != 0)
	case shader.KindVec2:
		return schema.Vec2(mathx.Vec2{X: lanes[0], Y: lanes[1]})
	case shader.KindVec3:
		return schema.Vec3(mathx.Vec3{X: lanes[0], Y: lanes[1], Z: lanes[2]})
	default:
		if p.Color {
			return schema.Color(mathx.Color{R: lanes[0], G: lanes[1], B: lanes[2], A: lanes[3]})
		}
		return schema.Vec4(mathx.Vec4{X: lanes[0], Y: lanes[1], Z: lanes[2], W: lanes[3]})
	}
}

func (v MaterialParameter) schemaValue(p *shader.Param) schema.Value {
	lanes := [4]float32{v.Value.X, v.Value.Y, v.Value.Z, v.Value.W}
	if v.Kind == shader.KindBool && v.Boolean {
		lanes[0] = 1
	}
	return ParameterValue(p, lanes, v.Integer)
}

func parameterFromValue(p *shader.Param, value schema.Value) (MaterialParameter, error) {
	out := MaterialParameter{Name: p.Name, Kind: p.Kind}
	if value.Kind != ParameterKind(p) {
		return out, fmt.Errorf("material parameter %s expects %s, got %s", p.Name, ParameterKind(p), value.Kind)
	}
	switch p.Kind {
	case shader.KindFloat:
		out.Value.X = float32(value.F)
	case shader.KindInt:
		if value.I < math.MinInt32 || value.I > math.MaxInt32 {
			return out, fmt.Errorf("material parameter %s is outside the signed int32 range", p.Name)
		}
		out.Integer = int32(value.I)
	case shader.KindBool:
		out.Boolean = value.I != 0
	case shader.KindVec2:
		out.Value = mathx.Vec4{X: value.V.X, Y: value.V.Y}
	case shader.KindVec3:
		out.Value = mathx.Vec4{X: value.V3.X, Y: value.V3.Y, Z: value.V3.Z}
	case shader.KindVec4:
		if p.Color {
			out.Value = mathx.Vec4{X: value.C.R, Y: value.C.G, Z: value.C.B, W: value.C.A}
		} else {
			out.Value = value.V4
		}
	default:
		return out, fmt.Errorf("material parameter %s cannot be overridden", p.Name)
	}
	for _, f := range [...]float32{out.Value.X, out.Value.Y, out.Value.Z, out.Value.W} {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return out, fmt.Errorf("material parameter %s must be finite", p.Name)
		}
	}
	return out, nil
}

func overrideParam(m *material.Material, name string) (*shader.Param, error) {
	if m == nil {
		return nil, fmt.Errorf("no resolved material")
	}
	p, ok := m.Shader().Schema.Param(name)
	if !ok {
		return nil, fmt.Errorf("material %s has no parameter %s", m.Name, name)
	}
	if !p.Instance || p.Kind == shader.KindTexture {
		return nil, fmt.Errorf("material parameter %s is not instance-overridable", name)
	}
	return p, nil
}

func (o *MaterialOverride) index(name string) int {
	for i := range o.Values {
		if o.Values[i].Name == name {
			return i
		}
	}
	return -1
}

func (o *MaterialOverride) indexParam(m *material.Material, p *shader.Param) int {
	for i := range o.Values {
		q, ok := m.Shader().Schema.Param(o.Values[i].Name)
		if ok && q.Name == p.Name {
			return i
		}
	}
	return -1
}
func (o *MaterialOverride) ExplicitFor(m *material.Material, name string) bool {
	if m == nil {
		return false
	}
	p, ok := m.Shader().Schema.Param(name)
	return ok && o.indexParam(m, p) >= 0
}
func (o *MaterialOverride) ResetFor(m *material.Material, name string) bool {
	if m == nil {
		return o.Reset(name)
	}
	p, ok := m.Shader().Schema.Param(name)
	if !ok {
		return o.Reset(name)
	}
	i := o.indexParam(m, p)
	if i < 0 {
		return false
	}
	o.Values = slices.Delete(o.Values, i, i+1)
	return true
}

func (o *MaterialOverride) Explicit(name string) bool { return o.index(name) >= 0 }

func (o *MaterialOverride) Set(m *material.Material, name string, value schema.Value) error {
	p, err := overrideParam(m, name)
	if err != nil {
		return err
	}
	v, err := parameterFromValue(p, value)
	if err != nil {
		return err
	}
	if i := o.indexParam(m, p); i >= 0 {
		o.Values[i] = v
		return nil
	}
	active := 0
	for _, value := range o.Values {
		if q, ok := m.Shader().Schema.Param(value.Name); ok && q.Instance {
			active++
		}
	}
	if active >= MaxMaterialOverrides {
		return fmt.Errorf("a node supports at most %d explicit material parameters", MaxMaterialOverrides)
	}
	o.Values = append(o.Values, v)
	return nil
}

func (o *MaterialOverride) Reset(name string) bool {
	i := o.index(name)
	if i < 0 {
		return false
	}
	o.Values = slices.Delete(o.Values, i, i+1)
	return true
}

func (o *MaterialOverride) Effective(m *material.Material, name string) (schema.Value, error) {
	if m == nil {
		return schema.Value{}, fmt.Errorf("no resolved material")
	}
	p, ok := m.Shader().Schema.Param(name)
	if !ok || p.Kind == shader.KindTexture {
		return schema.Value{}, fmt.Errorf("material has no numeric parameter %s", name)
	}
	if i := o.indexParam(m, p); i >= 0 {
		if o.Values[i].Kind != p.Kind {
			return schema.Value{}, fmt.Errorf("material parameter %s changed type; explicit %s value is preserved", name, o.Values[i].Kind.GLSL())
		}
		return o.Values[i].schemaValue(p), nil
	}
	v, _ := m.Get(name)
	var integer int32
	if p.Kind == shader.KindInt {
		integer, _ = m.GetInt(name)
	}
	return ParameterValue(p, v, integer), nil
}

// Lanes reuses component-owned storage. No maps, allocations, or pointers into
// schema storage are retained across a shader generation change.
func (o *MaterialOverride) Lanes(m *material.Material) (*[16]float32, error) {
	o.lanes = *m.DefaultLanes()
	active := 0
	for i := range o.Values {
		v := &o.Values[i]
		p, ok := m.Shader().Schema.Param(v.Name)
		if !ok {
			continue
		} // unknown semantic data remains serialized
		if !p.Instance || p.Kind != v.Kind {
			return nil, fmt.Errorf("material override %s is incompatible with the current shader", v.Name)
		}
		active++
		if active > MaxMaterialOverrides {
			return nil, fmt.Errorf("too many active material overrides")
		}
		if p.Kind == shader.KindInt {
			if !material.PutLaneInt(&o.lanes, p, v.Integer) {
				return nil, fmt.Errorf("material override %s has an invalid instance layout", v.Name)
			}
			continue
		}
		a := [4]float32{v.Value.X, v.Value.Y, v.Value.Z, v.Value.W}
		if p.Kind == shader.KindBool {
			a[0] = 0
			if v.Boolean {
				a[0] = 1
			}
		}
		if !material.PutLane(&o.lanes, p, a) {
			return nil, fmt.Errorf("material override %s cannot be packed into the instance layout", v.Name)
		}
	}
	return &o.lanes, nil
}

func NodeMaterial(s *scene.Scene, h scene.NodeHandle) (*material.Material, error) {
	if sr, ok := scene.Get[SpriteRenderer](s, h); ok {
		m, found := sr.Material.Material(s.Resolver())
		if found {
			return m, nil
		}
		return nil, fmt.Errorf("sprite material %s is unresolved", sr.Material.ID.String())
	}
	if mr, ok := scene.Get[Mesh2D](s, h); ok {
		m, found := mr.Material.Material(s.Resolver())
		if found {
			return m, nil
		}
		return nil, fmt.Errorf("mesh material %s is unresolved", mr.Material.ID.String())
	}
	return nil, fmt.Errorf("node has no compatible material renderer")
}

func AssignMaterial(s *scene.Scene, h scene.NodeHandle, id ids.AssetID) error {
	if sr, ok := scene.Get[SpriteRenderer](s, h); ok {
		sr.Material.SetAssetRefID(id)
		return nil
	}
	if mr, ok := scene.Get[Mesh2D](s, h); ok {
		mr.Material.SetAssetRefID(id)
		return nil
	}
	return fmt.Errorf("node has no compatible material renderer")
}

func SetMaterialParameter(s *scene.Scene, h scene.NodeHandle, name string, v schema.Value) error {
	m, err := NodeMaterial(s, h)
	if err != nil {
		return err
	}
	// Validate before adding a component, so failed assignments have no side effects.
	p, err := overrideParam(m, name)
	if err != nil {
		return err
	}
	if _, err = parameterFromValue(p, v); err != nil {
		return err
	}
	o, ok := scene.Get[MaterialOverride](s, h)
	if !ok {
		o = scene.Add[MaterialOverride](s, h)
	}
	return o.Set(m, name, v)
}

func GetMaterialParameter(s *scene.Scene, h scene.NodeHandle, name string) (schema.Value, error) {
	m, err := NodeMaterial(s, h)
	if err != nil {
		return schema.Value{}, err
	}
	o, ok := scene.Get[MaterialOverride](s, h)
	if !ok {
		o = &MaterialOverride{}
	}
	return o.Effective(m, name)
}

func ResetMaterialParameter(s *scene.Scene, h scene.NodeHandle, name string) bool {
	o, ok := scene.Get[MaterialOverride](s, h)
	m, _ := NodeMaterial(s, h)
	return ok && o.ResetFor(m, name)
}

func resolveMaterialProperty(s *scene.Scene, h scene.NodeHandle, name string) (scene.BoundProperty, error) {
	m, err := NodeMaterial(s, h)
	if err != nil {
		return scene.BoundProperty{}, err
	}
	p, err := overrideParam(m, name)
	if err != nil {
		return scene.BoundProperty{}, err
	}
	if !scene.Has[MaterialOverride](s, h) {
		return scene.BoundProperty{}, fmt.Errorf("node has no MaterialOverride component")
	}
	return scene.BoundProperty{Key: name, Kind: ParameterKind(p), Resource: m, Generation: m.Generation(), Access: &materialPropertyAccess}, nil
}

func materialPropertyTarget(b scene.BoundProperty, s *scene.Scene, h scene.NodeHandle) (*MaterialOverride, *material.Material, bool) {
	m, err := NodeMaterial(s, h)
	if err != nil || m != b.Resource || m.Generation() != b.Generation {
		return nil, nil, false
	}
	o, ok := scene.Get[MaterialOverride](s, h)
	return o, m, ok
}

var materialPropertyAccess = scene.PropertyAccess{
	Valid: func(b scene.BoundProperty, s *scene.Scene, h scene.NodeHandle) bool {
		_, _, ok := materialPropertyTarget(b, s, h)
		return ok
	},
	Get: func(b scene.BoundProperty, s *scene.Scene, h scene.NodeHandle, dst *schema.Value) bool {
		o, m, ok := materialPropertyTarget(b, s, h)
		if !ok {
			return false
		}
		v, err := o.Effective(m, b.Key)
		if err != nil {
			return false
		}
		*dst = v
		return true
	},
	Set: func(b scene.BoundProperty, s *scene.Scene, h scene.NodeHandle, v schema.Value) bool {
		o, m, ok := materialPropertyTarget(b, s, h)
		return ok && o.Set(m, b.Key, v) == nil
	},
}
