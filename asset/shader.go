package asset

import (
	"encoding/binary"
	"math"

	"aego/core/errs"
	"aego/core/ids"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/shaderx"
)

const (
	shaderMagic   = "AESH"
	shaderVersion = 1
)

const (
	paramRange = 1 << iota
	paramColor
	paramHidden
	paramInstance
)

type ShaderArtifact struct {
	Domain   shader.Domain
	Blend    shader.Blend
	Keywords []string
	Variants []material.Variant
	Params   []shader.Param
	Size     int
	Lanes    int
	Files    []string
	Maps     [4][]shader.Loc
}

type aeshW []byte

func (w *aeshW) u8(v uint8)   { *w = append(*w, v) }
func (w *aeshW) u16(v uint16) { *w = binary.LittleEndian.AppendUint16(*w, v) }
func (w *aeshW) u32(v uint32) { *w = binary.LittleEndian.AppendUint32(*w, v) }
func (w *aeshW) f32(v float32) {
	w.u32(math.Float32bits(v))
}
func (w *aeshW) str(s string) {
	w.u32(uint32(len(s)))
	*w = append(*w, s...)
}
func (w *aeshW) strs(ss []string) {
	w.u32(uint32(len(ss)))
	for _, s := range ss {
		w.str(s)
	}
}

type aeshR struct {
	b   []byte
	bad bool
}

func (r *aeshR) take(n int) []byte {
	if r.bad || n < 0 || len(r.b) < n {
		r.bad = true
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *aeshR) u8() uint8 {
	if v := r.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (r *aeshR) u16() uint16 {
	if v := r.take(2); v != nil {
		return binary.LittleEndian.Uint16(v)
	}
	return 0
}

func (r *aeshR) u32() uint32 {
	if v := r.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}

func (r *aeshR) f32() float32 {
	return math.Float32frombits(r.u32())
}

func (r *aeshR) count(limit int) int {
	n := int(r.u32())
	if n > limit || n > len(r.b) {
		r.bad = true
		return 0
	}
	return n
}

func (r *aeshR) str() string {
	return string(r.take(int(r.u32())))
}

func (r *aeshR) strs(limit int) []string {
	n := r.count(limit)
	out := make([]string, n)
	for i := range out {
		out[i] = r.str()
	}
	return out
}

func EncodeShader(a *ShaderArtifact) []byte {
	w := aeshW(make([]byte, 0, 16<<10))
	w = append(w, shaderMagic...)
	w.u16(shaderVersion)
	w.u16(shader.ABI)
	w.u8(uint8(a.Domain))
	w.u8(uint8(a.Blend))
	w.strs(a.Keywords)
	w.u32(uint32(len(a.Variants)))
	for _, v := range a.Variants {
		w.u32(v.Mask)
		for _, s := range v.Sources {
			w.str(s)
		}
	}
	w.u32(uint32(len(a.Params)))
	for i := range a.Params {
		putParam(&w, &a.Params[i])
	}
	w.u32(uint32(a.Size))
	w.u8(uint8(a.Lanes))
	w.strs(a.Files)
	for _, m := range a.Maps {
		w.u32(uint32(len(m)))
		for _, l := range m {
			w.u32(uint32(l.File))
			w.u32(uint32(l.Line))
		}
	}
	return w
}

func putParam(w *aeshW, p *shader.Param) {
	w.str(p.Name)
	w.u8(uint8(p.Kind))
	var f uint8
	if p.HasRange {
		f |= paramRange
	}
	if p.Color {
		f |= paramColor
	}
	if p.Hidden {
		f |= paramHidden
	}
	if p.Instance {
		f |= paramInstance
	}
	w.u8(f)
	for _, v := range p.Default {
		w.f32(v)
	}
	w.f32(p.Min)
	w.f32(p.Max)
	w.u16(uint16(p.Offset))
	w.u8(uint8(p.Lane))
	w.u8(uint8(p.Comp))
	w.u8(uint8(p.Slot))
	w.str(p.Doc)
	w.str(p.Display)
	w.str(p.Unit)
	w.strs(p.Enum)
}

func badShader(msg string) error {
	return errs.New(errs.AssetBadArtifact, "shader artifact: "+msg)
}

func DecodeShader(b []byte) (*ShaderArtifact, error) {
	if len(b) < 8 || string(b[:4]) != shaderMagic {
		return nil, badShader("bad magic")
	}
	r := &aeshR{b: b[4:]}
	if v := r.u16(); v != shaderVersion {
		return nil, badShader("unknown version")
	}
	if v := r.u16(); v != shader.ABI {
		return nil, badShader("imported for another shader ABI, reimport it")
	}
	a := &ShaderArtifact{Domain: shader.Domain(r.u8()), Blend: shader.Blend(r.u8())}
	a.Keywords = r.strs(shader.MaxKeywords)
	a.Variants = make([]material.Variant, r.count(shader.MaxVariants))
	for i := range a.Variants {
		a.Variants[i].Mask = r.u32()
		for s := range a.Variants[i].Sources {
			a.Variants[i].Sources[s] = r.str()
		}
	}
	a.Params = make([]shader.Param, r.count(shader.MaxParams))
	for i := range a.Params {
		readParam(r, &a.Params[i])
	}
	a.Size = int(r.u32())
	a.Lanes = int(r.u8())
	a.Files = r.strs(1 << 10)
	for i := range a.Maps {
		a.Maps[i] = make([]shader.Loc, r.count(1<<20))
		for j := range a.Maps[i] {
			a.Maps[i][j] = shader.Loc{File: int32(r.u32()), Line: int32(r.u32())}
		}
	}
	if r.bad || len(a.Variants) == 0 || a.Lanes > shader.MaxLanes {
		return nil, badShader("truncated or out of range")
	}
	for i := range a.Params {
		if !paramFits(&a.Params[i], a) {
			return nil, badShader("parameter " + a.Params[i].Name + " is out of range")
		}
	}
	return a, nil
}

func readParam(r *aeshR, p *shader.Param) {
	p.Name = r.str()
	p.Kind = shader.Kind(r.u8())
	f := r.u8()
	p.HasRange, p.Color, p.Hidden, p.Instance = f&paramRange != 0, f&paramColor != 0, f&paramHidden != 0, f&paramInstance != 0
	for i := range p.Default {
		p.Default[i] = r.f32()
	}
	p.Min, p.Max = r.f32(), r.f32()
	p.Offset = int(r.u16())
	p.Lane, p.Comp, p.Slot = int(r.u8()), int(r.u8()), int(r.u8())
	p.Doc, p.Display, p.Unit = r.str(), r.str(), r.str()
	p.Enum = r.strs(64)
}

func paramFits(p *shader.Param, a *ShaderArtifact) bool {
	switch {
	case p.Kind > shader.KindTexture:
		return false
	case p.Kind == shader.KindTexture:
		return p.Slot >= shader.TexUser && p.Slot < shader.TexUser+shader.MaxUserTextures
	case p.Instance:
		return p.Lane < a.Lanes && p.Comp+p.Kind.Lanes() <= 4
	}
	return p.Offset+4*p.Kind.Lanes() <= a.Size
}

func (a *ShaderArtifact) Runtime(id ids.AssetID, name string) *material.Shader {
	return &material.Shader{
		ID:       id,
		Name:     name,
		Domain:   a.Domain,
		Blend:    a.Blend,
		Keywords: a.Keywords,
		Variants: a.Variants,
		Schema:   material.NewSchema(a.Params, a.Size, a.Lanes),
		Files:    a.Files,
		Maps:     a.Maps,
	}
}

// BuildShader generates every variant of a parsed shader in both stages and both GLSL dialects, with the line map of the first variant.
func BuildShader(u *shaderx.Unit) *ShaderArtifact {
	a := &ShaderArtifact{Domain: u.Domain, Blend: u.Blend, Keywords: u.Keywords, Params: u.Params, Size: u.Size, Lanes: u.Lanes, Files: u.Files}
	for vi, mask := range u.Combos {
		v := material.Variant{Mask: mask}
		for si := range v.Sources {
			src, locs := u.Source(si%2 == 1, si >= 2, mask)
			v.Sources[si] = src
			if vi == 0 {
				a.Maps[si] = locs
			}
		}
		a.Variants = append(a.Variants, v)
	}
	return a
}
