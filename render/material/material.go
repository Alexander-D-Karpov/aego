package material

import (
	"encoding/binary"
	"math"

	"aego/core/ids"
	"aego/gpu"
	"aego/render/shader"
)

type Tex struct {
	Texture gpu.TextureHandle
	Sampler gpu.SamplerHandle
}

type Material struct {
	Name    string
	ID      ids.AssetID
	shader  *Shader
	next    *Shader
	variant int
	mask    uint32
	blend   shader.Blend
	data    []byte
	tex     [shader.MaxUserTextures]Tex
	ub      gpu.BufferHandle
	ubSize  int
	dirty   bool
	failed  bool
}

func New(name string, sh *Shader, mask uint32, blend shader.Blend) (*Material, bool) {
	v := sh.Variant(mask)
	if v < 0 {
		return nil, false
	}
	m := &Material{Name: name, shader: sh, variant: v, mask: mask, blend: blend, data: make([]byte, sh.Schema.Size), dirty: true}
	for i := range sh.Schema.Params {
		if p := &sh.Schema.Params[i]; !p.Instance && p.Kind != shader.KindTexture {
			m.put(p, p.Default)
		}
	}
	return m, true
}

func (m *Material) Shader() *Shader     { return m.shader }
func (m *Material) Mask() uint32        { return m.mask }
func (m *Material) Blend() shader.Blend { return m.blend }
func (m *Material) Lanes() int          { return m.shader.Schema.Lanes }
func (m *Material) Failed() bool        { return m.failed }
func (m *Material) DefaultLanes() *[16]float32 {
	return m.shader.Schema.DefaultLanes()
}

func (m *Material) put(p *shader.Param, v [4]float32) {
	b := m.data[p.Offset:]
	switch p.Kind {
	case shader.KindInt:
		binary.LittleEndian.PutUint32(b, uint32(int32(math.Round(float64(v[0])))))
	case shader.KindBool:
		var u uint32
		if v[0] != 0 {
			u = 1
		}
		binary.LittleEndian.PutUint32(b, u)
	default:
		for c := 0; c < p.Kind.Lanes(); c++ {
			binary.LittleEndian.PutUint32(b[4*c:], math.Float32bits(v[c]))
		}
	}
}

func (m *Material) Set(name string, v [4]float32) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Instance || p.Kind == shader.KindTexture {
		return false
	}
	m.put(p, v)
	m.dirty = true
	return true
}

func (m *Material) Get(name string) ([4]float32, bool) {
	var v [4]float32
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Instance || p.Kind == shader.KindTexture {
		return v, false
	}
	b := m.data[p.Offset:]
	switch p.Kind {
	case shader.KindInt:
		v[0] = float32(int32(binary.LittleEndian.Uint32(b)))
	case shader.KindBool:
		v[0] = float32(binary.LittleEndian.Uint32(b))
	default:
		for c := 0; c < p.Kind.Lanes(); c++ {
			v[c] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*c:]))
		}
	}
	return v, true
}

func (m *Material) SetTexture(name string, t Tex) bool {
	p, ok := m.shader.Schema.Param(name)
	if !ok || p.Kind != shader.KindTexture {
		return false
	}
	m.tex[p.Slot-shader.TexUser] = t
	return true
}

func (m *Material) Reload(sh *Shader) {
	m.next = sh
}

func (m *Material) promote(sh *Shader, v int) {
	old, data, tex := m.shader.Schema, m.data, m.tex
	m.shader, m.variant = sh, v
	m.data = make([]byte, sh.Schema.Size)
	m.tex = [shader.MaxUserTextures]Tex{}
	for i := range sh.Schema.Params {
		p := &sh.Schema.Params[i]
		q, ok := old.Param(p.Name)
		same := ok && q.Kind == p.Kind && q.Instance == p.Instance
		switch {
		case p.Kind == shader.KindTexture:
			if same {
				m.tex[p.Slot-shader.TexUser] = tex[q.Slot-shader.TexUser]
			}
		case p.Instance:
		case same:
			copy(m.data[p.Offset:p.Offset+4*p.Kind.Lanes()], data[q.Offset:])
		default:
			m.put(p, p.Default)
		}
	}
	m.dirty = true
}
