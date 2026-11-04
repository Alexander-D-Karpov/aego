package material

import (
	"encoding/binary"
	"fmt"
	"math"

	"aego/core/ids"
	"aego/gpu"
	"aego/render/shader"
	"aego/render/texture"
)

type Tex struct {
	Texture      gpu.TextureHandle
	Sampler      gpu.SamplerHandle
	Region       texture.Region
	RegionOnly   bool
	NormalMap    bool
	EmissionMode texture.EmissionMode
}

// storedValue keeps explicit values independently of the current GPU layout.
// Removed fields remain here and can be restored by a later compatible reload.
type storedValue struct {
	kind     shader.Kind
	words    [4]uint32
	tex      Tex
	pending  *Value
	resolved bool
}

type Material struct {
	Name       string
	ID         ids.AssetID
	shader     *Shader
	next       *Shader
	variant    int
	mask       uint32
	blend      shader.Blend
	data       []byte
	lanes      [16]float32
	tex        [shader.MaxUserTextures + 2]Tex
	values     map[string]storedValue
	generation uint64
	ub         gpu.BufferHandle
	ubSize     int
	dirty      bool
	failed     bool
}

func New(name string, sh *Shader, mask uint32, blend shader.Blend) (*Material, bool) {
	if sh == nil || sh.Schema == nil || blend > shader.BlendAdditive {
		return nil, false
	}
	v := sh.Variant(mask)
	if v < 0 {
		return nil, false
	}
	m := &Material{Name: name, shader: sh, variant: v, mask: mask, blend: blend,
		data: make([]byte, sh.Schema.Size), values: make(map[string]storedValue, shader.MaxParams), generation: 1, dirty: true}
	for i := range sh.Schema.Params {
		p := &sh.Schema.Params[i]
		if p.Kind != shader.KindTexture {
			m.putWords(p, defaultWords(p))
		}
	}
	return m, true
}

func (m *Material) Shader() *Shader     { return m.shader }
func (m *Material) Mask() uint32        { return m.mask }
func (m *Material) Blend() shader.Blend { return m.blend }
func (m *Material) Lanes() int          { return m.shader.Schema.Lanes }
func (m *Material) Failed() bool        { return m.failed }

// Generation changes only when a new shader/schema is committed.
func (m *Material) Generation() uint64 { return m.generation }

// DefaultLanes returns effective material values, including explicit values of
// instance-overridable parameters. Callers copy it before applying node overrides.
func (m *Material) DefaultLanes() *[16]float32 { return &m.lanes }
func (m *Material) Explicit(name string) bool  { _, ok := m.values[name]; return ok }

func defaultWords(p *shader.Param) (v [4]uint32) {
	switch p.Kind {
	case shader.KindInt:
		n := p.IntDefault
		// Hand-created ABI 1 descriptors used Default for small integer values.
		if n == 0 && p.Default[0] != 0 {
			n = int32(p.Default[0])
		}
		v[0] = uint32(n)
	case shader.KindBool:
		if p.Default[0] != 0 {
			v[0] = 1
		}
	default:
		for c := 0; c < p.Kind.Lanes(); c++ {
			v[c] = math.Float32bits(p.Default[c])
		}
	}
	return
}

func words(p *shader.Param, v [4]float32) (out [4]uint32, ok bool) {
	for c := 0; c < p.Kind.Lanes(); c++ {
		if math.IsNaN(float64(v[c])) || math.IsInf(float64(v[c]), 0) {
			return out, false
		}
	}
	switch p.Kind {
	case shader.KindInt:
		n := float64(v[0])
		if n < math.MinInt32 || n > math.MaxInt32 || n != math.Trunc(n) {
			return out, false
		}
		out[0] = uint32(int32(n))
	case shader.KindBool:
		if v[0] != 0 && v[0] != 1 {
			return out, false
		}
		out[0] = uint32(v[0])
	case shader.KindTexture:
		return out, false
	default:
		for c := 0; c < p.Kind.Lanes(); c++ {
			out[c] = math.Float32bits(v[c])
		}
	}
	return out, true
}

func (m *Material) putWords(p *shader.Param, v [4]uint32) {
	if p.Instance {
		for c := 0; c < p.Kind.Lanes(); c++ {
			m.lanes[p.Lane*4+p.Comp+c] = math.Float32frombits(v[c])
		}
		return
	}
	for c := 0; c < p.Kind.Lanes(); c++ {
		binary.LittleEndian.PutUint32(m.data[p.Offset+4*c:], v[c])
	}
	m.dirty = true
}

func (m *Material) readWords(p *shader.Param) (v [4]uint32) {
	for c := 0; c < p.Kind.Lanes(); c++ {
		if p.Instance {
			v[c] = math.Float32bits(m.lanes[p.Lane*4+p.Comp+c])
		} else {
			v[c] = binary.LittleEndian.Uint32(m.data[p.Offset+4*c:])
		}
	}
	return
}

func (m *Material) Set(name string, v [4]float32) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind == shader.KindTexture {
		return false
	}
	w, ok := words(p, v)
	if !ok {
		return false
	}
	m.putWords(p, w)
	m.values[p.Name] = storedValue{kind: p.Kind, words: w}
	return true
}

// SetInt preserves every int32 value, including values not representable by float32.
func (m *Material) SetInt(name string, v int32) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind != shader.KindInt {
		return false
	}
	w := [4]uint32{uint32(v)}
	m.putWords(p, w)
	m.values[p.Name] = storedValue{kind: p.Kind, words: w}
	return true
}

func (m *Material) Get(name string) ([4]float32, bool) {
	var v [4]float32
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind == shader.KindTexture {
		return v, false
	}
	w := m.readWords(p)
	switch p.Kind {
	case shader.KindInt:
		v[0] = float32(int32(w[0]))
	case shader.KindBool:
		v[0] = float32(w[0])
	default:
		for c := 0; c < p.Kind.Lanes(); c++ {
			v[c] = math.Float32frombits(w[c])
		}
	}
	return v, true
}

func (m *Material) GetInt(name string) (int32, bool) {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind != shader.KindInt {
		return 0, false
	}
	return int32(m.readWords(p)[0]), true
}

// Reset restores inheritance even if the explicit value equalled the default.
func (m *Material) Reset(name string) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok {
		return false
	}
	delete(m.values, p.Name)
	for _, former := range p.Former {
		delete(m.values, former)
	}
	if p.Kind == shader.KindTexture {
		m.tex[textureIndex(p.Slot)] = Tex{}
	} else {
		m.putWords(p, defaultWords(p))
	}
	return true
}

func (m *Material) SetTexture(name string, t Tex) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind != shader.KindTexture || !textureCompatible(p, t) {
		return false
	}
	m.tex[textureIndex(p.Slot)] = t
	m.values[p.Name] = storedValue{kind: p.Kind, tex: t}
	return true
}

func textureCompatible(p *shader.Param, t Tex) bool {
	if t.RegionOnly && !p.Normal && !p.Emission {
		return false
	}
	if t.Texture.IsZero() && t.Region.IsZero() {
		return true // Explicitly empty is distinct from inherited.
	}
	if p.Normal {
		return t.NormalMap
	}
	if p.Emission {
		return t.EmissionMode == texture.EmissionRGB || t.EmissionMode == texture.EmissionMask
	}
	return true
}

// SetUnresolvedTexture preserves an explicit missing surface-channel reference. The
// inspector reports it and strict export rejects its missing dependency.
func (m *Material) SetUnresolvedTexture(name string, id ids.AssetID) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || (!p.Normal && !p.Emission) {
		return false
	}
	v := Value{Name: p.Name, Kind: shader.KindTexture, Typed: true, Asset: true, Reference: id}
	m.values[p.Name] = storedValue{kind: p.Kind, pending: &v}
	m.tex[textureIndex(p.Slot)] = Tex{}
	return true
}

func textureIndex(slot int) int {
	if slot == shader.TexNormal {
		return shader.MaxUserTextures
	}
	if slot == shader.TexEmission {
		return shader.MaxUserTextures + 1
	}
	return slot - shader.TexUser
}

// Normal distinguishes absence from an explicit unresolved map. A material map
// owns its mapping; a sprite-frame pair is considered only when explicit is false.
func (m *Material) Normal() (region texture.Region, explicit, resolved bool) {
	for _, i := range m.shader.Schema.Textures {
		p := &m.shader.Schema.Params[i]
		if !p.Normal {
			continue
		}
		if !m.Explicit(p.Name) {
			return texture.Region{}, false, false
		}
		t := m.tex[textureIndex(p.Slot)]
		region = t.Region
		if region.IsZero() && !t.Texture.IsZero() {
			region = texture.Whole(t.Texture, 1, 1)
		}
		return region, true, !region.IsZero() || m.values[p.Name].pending == nil
	}
	return texture.Region{}, false, false
}

// Emission follows the same explicit/inherited policy as Normal and retains its
// own source-canvas mapping. RGB and scalar masks cannot be confused by filename.
func (m *Material) Emission() (value texture.Emission, explicit, resolved bool) {
	for _, i := range m.shader.Schema.Textures {
		p := &m.shader.Schema.Params[i]
		if !p.Emission {
			continue
		}
		if !m.Explicit(p.Name) {
			return texture.Emission{}, false, false
		}
		t := m.tex[textureIndex(p.Slot)]
		region := t.Region
		if region.IsZero() && !t.Texture.IsZero() {
			region = texture.Whole(t.Texture, 1, 1)
		}
		return texture.Emission{Region: region, Mode: t.EmissionMode}, true, !region.IsZero() || m.values[p.Name].pending == nil
	}
	return texture.Emission{}, false, false
}

func (m *Material) GetTexture(name string) (Tex, bool) {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind != shader.KindTexture {
		return Tex{}, false
	}
	return m.tex[textureIndex(p.Slot)], true
}

// PutLane writes the typed ABI word(s); no integer is converted to float for GPU input.
func PutLane(dst *[16]float32, p *shader.Param, v [4]float32) bool {
	if dst == nil || p == nil || !p.Instance || p.Kind == shader.KindTexture || p.Lane < 0 || p.Comp < 0 || p.Comp+p.Kind.Lanes() > 4 || p.Lane*4+p.Comp+p.Kind.Lanes() > len(dst) {
		return false
	}
	w, ok := words(p, v)
	if !ok {
		return false
	}
	for c := 0; c < p.Kind.Lanes(); c++ {
		dst[p.Lane*4+p.Comp+c] = math.Float32frombits(w[c])
	}
	return true
}

func PutLaneInt(dst *[16]float32, p *shader.Param, v int32) bool {
	if dst == nil || p == nil || !p.Instance || p.Kind != shader.KindInt || p.Lane < 0 || p.Comp < 0 || p.Lane*4+p.Comp >= len(dst) {
		return false
	}
	dst[p.Lane*4+p.Comp] = math.Float32frombits(uint32(v))
	return true
}

// Reload schedules a change; the owning cache commits it at a frame boundary.
func (m *Material) Reload(sh *Shader) { m.next = sh }

func (m *Material) prepare(sh *Shader) (*Material, error) {
	if sh.Domain != m.shader.Domain {
		return nil, fmt.Errorf("MAT004: shader domain changed from %s to %s", m.shader.Domain, sh.Domain)
	}
	var mask uint32
	for i, keyword := range m.shader.Keywords {
		if m.mask&(1<<i) == 0 {
			continue
		}
		found := false
		for j, next := range sh.Keywords {
			if keyword == next {
				mask |= 1 << j
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("MAT003: active keyword %s was removed", keyword)
		}
	}
	n, ok := New(m.Name, sh, mask, m.blend)
	if !ok {
		return nil, fmt.Errorf("MAT003: active variant is not allowed by the new shader")
	}
	n.ID = m.ID
	for name, v := range m.values {
		n.values[name] = v
	}
	for i := range sh.Schema.Params {
		p := &sh.Schema.Params[i]
		value, explicit := n.values[p.Name]
		from := p.Name
		if !explicit {
			for _, former := range p.Former {
				if value, explicit = n.values[former]; explicit {
					from = former
					break
				}
			}
		}
		if !explicit {
			continue
		}
		if value.pending != nil {
			var err error
			value, err = resolvePending(p, value)
			if err != nil {
				return nil, err
			}
			n.values[from] = value
		}
		if value.kind != p.Kind {
			return nil, fmt.Errorf("MAT002: explicit parameter %s changed type from %s to %s", p.Name, value.kind.GLSL(), p.Kind.GLSL())
		}
		if p.Kind == shader.KindTexture {
			if !textureCompatible(p, value.tex) {
				return nil, fmt.Errorf("MAT004: explicit texture %s is incompatible with the new normal/region semantics", p.Name)
			}
			n.tex[textureIndex(p.Slot)] = value.tex
		} else {
			n.putWords(p, value.words)
		}
		if from != p.Name {
			delete(n.values, from)
			n.values[p.Name] = value
		}
	}
	return n, nil
}

func (m *Material) commit(n *Material) {
	m.shader, m.variant, m.mask, m.blend = n.shader, n.variant, n.mask, n.blend
	m.data, m.lanes, m.tex, m.values = n.data, n.lanes, n.tex, n.values
	m.generation++
	m.dirty, m.failed, m.next = true, false, nil
}

func (m *Material) promote(sh *Shader, _ int) error {
	n, err := m.prepare(sh)
	if err != nil {
		return err
	}
	m.commit(n)
	return nil
}

// CloneDefinition copies only CPU material state, including unknown explicit data.
// It owns no GPU resources and may be staged for a transactional source edit.
func (m *Material) CloneDefinition() (*Material, error) { return m.prepare(m.shader) }

func (m *Material) UsesTexture(h gpu.TextureHandle) bool {
	if h.IsZero() {
		return false
	}
	for _, t := range m.tex {
		if t.Texture == h {
			return true
		}
	}
	for _, value := range m.values {
		if value.kind == shader.KindTexture && value.tex.Texture == h {
			return true
		}
	}
	return false
}
