package asset

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/render/material"
	"aego/render/shader"
)

const (
	shaderMagic   = "AESH"
	shaderVersion = 4
)

const (
	paramRange = 1 << iota
	paramColor
	paramHidden
	paramInstance
	paramNormal
	paramEmission
)

type ShaderArtifact struct {
	Features     shader.Features
	Domain       shader.Domain
	Blend        shader.Blend
	Keywords     []string
	Variants     []material.Variant
	Params       []shader.Param
	Size         int
	Lanes        int
	Files        []string
	Maps         [4][]shader.Loc
	Vertex       bool
	Validation   shader.ValidationFlags
	Dependencies []shader.Dependency
	LayoutHash   [32]byte
	ContentHash  [32]byte
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
	if n < 0 || n > limit || n > len(r.b) {
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
	w.u8(uint8(a.Validation))
	if a.Vertex {
		w.u8(1)
	} else {
		w.u8(0)
	}
	w.u32(uint32(a.Features))
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
	w.u32(uint32(len(a.Dependencies)))
	for _, d := range a.Dependencies {
		w.str(d.Path)
		w = append(w, d.Hash[:]...)
	}
	for _, m := range a.Maps {
		w.u32(uint32(len(m)))
		for _, l := range m {
			w.u32(uint32(l.File))
			w.u32(uint32(l.Line))
		}
	}
	a.LayoutHash = shader.FeatureLayoutHash(a.Domain, a.Params, a.Size, a.Lanes, a.Features)
	w = append(w, a.LayoutHash[:]...)
	a.ContentHash = sha256.Sum256(w)
	w = append(w, a.ContentHash[:]...)
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
	if p.Normal {
		f |= paramNormal
	}
	if p.Emission {
		f |= paramEmission
	}
	w.u8(f)
	for _, v := range p.Default {
		w.f32(v)
	}
	w.u32(uint32(p.IntDefault))
	w.strs(p.Former)
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
	if len(b) < 40 || len(b) > shader.MaxArtifactBytes || string(b[:4]) != shaderMagic {
		return nil, badShader("bad magic")
	}
	r := &aeshR{b: b[4:]}
	version := r.u16()
	if version != shaderVersion && version != 3 && version != 2 {
		return nil, badShader("unknown version")
	}
	if v := r.u16(); v != shader.ABI {
		return nil, badShader("imported for another shader ABI, reimport it")
	}
	a := &ShaderArtifact{Domain: shader.Domain(r.u8()), Blend: shader.Blend(r.u8())}
	a.Validation = shader.ValidationFlags(r.u8())
	vertex := r.u8()
	if vertex > 1 {
		return nil, badShader("invalid vertex policy")
	}
	a.Vertex = vertex == 1
	if version >= 3 {
		a.Features = shader.Features(r.u32())
	}
	if version < 4 && a.Features&shader.FeatureSurfaceChannels != 0 {
		return nil, badShader("surface channels require AESH4")
	}
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
		if version < 4 && a.Params[i].Emission {
			return nil, badShader("emission parameters require AESH4")
		}
	}
	a.Size = int(r.u32())
	a.Lanes = int(r.u8())
	a.Files = r.strs(shader.MaxIncludeFiles)
	a.Dependencies = make([]shader.Dependency, r.count(shader.MaxIncludeFiles))
	for i := range a.Dependencies {
		a.Dependencies[i].Path = r.str()
		copy(a.Dependencies[i].Hash[:], r.take(32))
	}
	for i := range a.Maps {
		a.Maps[i] = make([]shader.Loc, r.count(1<<20))
		for j := range a.Maps[i] {
			a.Maps[i][j] = shader.Loc{File: int32(r.u32()), Line: int32(r.u32())}
		}
	}
	copy(a.LayoutHash[:], r.take(32))
	copy(a.ContentHash[:], r.take(32))
	if r.bad || len(r.b) != 0 {
		return nil, badShader("truncated artifact or trailing data")
	}
	if sha256.Sum256(b[:len(b)-32]) != a.ContentHash {
		return nil, badShader("content hash mismatch")
	}
	if err := validateShaderArtifact(a); err != nil {
		return nil, err
	}
	return a, nil
}

func readParam(r *aeshR, p *shader.Param) {
	p.Name = r.str()
	p.Kind = shader.Kind(r.u8())
	f := r.u8()
	if f & ^uint8(paramRange|paramColor|paramHidden|paramInstance|paramNormal|paramEmission) != 0 {
		r.bad = true
	}
	p.HasRange, p.Color, p.Hidden, p.Instance = f&paramRange != 0, f&paramColor != 0, f&paramHidden != 0, f&paramInstance != 0
	p.Normal = f&paramNormal != 0
	p.Emission = f&paramEmission != 0
	for i := range p.Default {
		p.Default[i] = r.f32()
	}
	p.IntDefault = int32(r.u32())
	p.Former = r.strs(shader.MaxParams)
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
		if p.Emission {
			return p.Slot == shader.TexEmission && a.Features&shader.FeatureSurfaceChannels != 0 && !p.Normal
		}
		if p.Normal {
			return p.Slot == shader.TexNormal && a.Features&shader.FeatureLighting != 0
		}
		return p.Slot >= shader.TexUser && p.Slot < shader.TexUser+shader.MaxUserTextures
	case p.Instance:
		return p.Lane < a.Lanes && p.Comp+p.Kind.Lanes() <= 4
	}
	return p.Offset+4*p.Kind.Lanes() <= a.Size
}

func (a *ShaderArtifact) Runtime(id ids.AssetID, name string) *material.Shader {
	return &material.Shader{
		Features: a.Features,
		ID:       id,
		Name:     name,
		Domain:   a.Domain,
		Blend:    a.Blend,
		Keywords: a.Keywords,
		Variants: a.Variants,
		Schema:   material.NewSchema(a.Params, a.Size, a.Lanes),
		Files:    a.Files,
		Maps:     a.Maps,
		Vertex:   a.Vertex,
	}
}

// BuildShader generates every variant of a parsed shader in both stages and both GLSL dialects, with the line map of the first variant.
func BuildShader(u shader.Source) *ShaderArtifact {
	d := u.Descriptor()
	a := &ShaderArtifact{Features: d.Features, Domain: d.Domain, Blend: d.Blend, Keywords: d.Keywords, Params: d.Params, Size: d.Size, Lanes: d.Lanes, Files: d.Files, Vertex: d.Vertex, Dependencies: d.Dependencies, Validation: shader.SchemaChecked}
	for vi, mask := range d.Combos {
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
	a.LayoutHash = shader.FeatureLayoutHash(a.Domain, a.Params, a.Size, a.Lanes, a.Features)
	return a
}

func validShaderName(s string) bool {
	if len(s) == 0 || len(s) > shader.MaxNameLen {
		return false
	}
	for i, c := range s {
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func validateShaderArtifact(a *ShaderArtifact) error {
	if a.Features & ^shader.AllFeatures != 0 {
		return badShader("unknown shader feature")
	}
	if a.Features&shader.FeatureSurfaceChannels != 0 && a.Features&shader.FeatureLighting == 0 {
		return badShader("surface channels require forward2d lighting")
	}
	if a.Features&shader.FeaturePixelCoverage != 0 && (a.Domain != shader.DomainSprite || a.Vertex || a.Features&shader.FeatureLighting != 0) {
		return badShader("pixel coverage requires an unlit sprite without a vertex override")
	}
	if a.Features&shader.FeaturePostSceneLinear != 0 && a.Domain != shader.DomainPostFX {
		return badShader("scene-linear post-processing requires the postfx domain")
	}
	if a.Domain > shader.DomainPostFX || (a.Domain == shader.DomainPostFX && (a.Features&^shader.FeaturePostSceneLinear != 0 || a.Lanes != 0 || a.Vertex)) || a.Blend > shader.BlendAdditive || a.Validation&^shader.AllValidationFlags != 0 || a.Validation&shader.SchemaChecked == 0 {
		return badShader("invalid domain, blend or validation flags")
	}
	if a.Size < 0 || a.Size > shader.MaxParams*16 || a.Size%16 != 0 || a.Lanes < 0 || a.Lanes > shader.MaxLanes || len(a.Variants) == 0 || len(a.Files) == 0 {
		return badShader("invalid layout or missing sources")
	}
	names := map[string]bool{}
	for _, k := range a.Keywords {
		if !validShaderName(k) || names[k] {
			return badShader("invalid or duplicate keyword")
		}
		names[k] = true
	}
	variants := map[uint32]bool{}
	for _, v := range a.Variants {
		if v.Mask >= 1<<len(a.Keywords) || variants[v.Mask] {
			return badShader("invalid or duplicate variant")
		}
		variants[v.Mask] = true
		for si, src := range v.Sources {
			prefix := "#version 330 core\n"
			if si >= 2 {
				prefix = "#version 300 es\n"
			}
			if !strings.HasPrefix(src, prefix) || len(src) > shader.MaxExpandedBytes+(128<<10) {
				return badShader("invalid target source")
			}
			if len(a.Maps[si]) != strings.Count(src, "\n") {
				return badShader("source map does not match generated lines")
			}
		}
	}
	fileNames := map[string]bool{}
	for _, file := range a.Files {
		if file == "" || fileNames[file] {
			return badShader("empty or duplicate source filename")
		}
		fileNames[file] = true
	}
	if len(a.Dependencies) != len(a.Files) {
		return badShader("missing source dependency")
	}
	dependencies := map[string]bool{}
	for _, dep := range a.Dependencies {
		if !fileNames[dep.Path] || dependencies[dep.Path] {
			return badShader("missing or duplicate source dependency")
		}
		dependencies[dep.Path] = true
	}
	for _, m := range a.Maps {
		for _, l := range m {
			if l.File < -1 || int(l.File) >= len(a.Files) || l.File >= 0 && l.Line < 1 {
				return badShader("invalid source-map location")
			}
		}
	}
	var kinds []shader.Kind
	var offsets []int
	var lanes [16]bool
	var samplers [shader.MaxUserTextures + 2]bool
	instanceCount, maxLane := 0, 0
	names = map[string]bool{}
	for _, p := range a.Params {
		if !validShaderName(p.Name) || names[p.Name] || slices.Contains(a.Keywords, p.Name) || !paramFits(&p, a) {
			return badShader("invalid or duplicate parameter " + p.Name)
		}
		names[p.Name] = true
		if p.Normal && p.Kind != shader.KindTexture {
			return badShader("normal hint requires a sampler")
		}
		if p.Emission && (p.Kind != shader.KindTexture || p.Normal) {
			return badShader("emission hint requires a distinct sampler")
		}
		if p.Color && p.Kind != shader.KindVec3 && p.Kind != shader.KindVec4 {
			return badShader("color hint requires a vector")
		}
		if len(p.Enum) > 0 && p.Kind != shader.KindInt {
			return badShader("enum hint requires int")
		}
		enumNames := map[string]bool{}
		for _, name := range p.Enum {
			if name == "" || enumNames[name] {
				return badShader("empty or duplicate enum name")
			}
			enumNames[name] = true
		}
		if p.HasRange && (math.IsNaN(float64(p.Min)) || math.IsNaN(float64(p.Max)) || math.IsInf(float64(p.Min), 0) || math.IsInf(float64(p.Max), 0) || p.Min > p.Max) {
			return badShader("invalid parameter range")
		}
		for _, v := range p.Default {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return badShader("non-finite default")
			}
		}
		if p.Kind == shader.KindBool && p.Default[0] != 0 && p.Default[0] != 1 {
			return badShader("invalid boolean default")
		}
		switch {
		case p.Kind == shader.KindTexture:
			index := p.Slot - shader.TexUser
			if p.Normal {
				index = shader.MaxUserTextures
			}
			if p.Emission {
				index = shader.MaxUserTextures + 1
			}
			if p.Instance || samplers[index] {
				return badShader("invalid sampler layout")
			}
			samplers[index] = true
		case p.Instance:
			instanceCount++
			if instanceCount > shader.MaxInstanceParams {
				return badShader("instance layout exceeds domain budget")
			}
			for j := 0; j < p.Kind.Lanes(); j++ {
				n := p.Lane*4 + p.Comp + j
				if n < 0 || n >= len(lanes) || lanes[n] {
					return badShader("overlapping instance parameters")
				}
				lanes[n] = true
			}
			if p.Lane+1 > maxLane {
				maxLane = p.Lane + 1
			}
		default:
			kinds = append(kinds, p.Kind)
			offsets = append(offsets, p.Offset)
		}
	}
	for _, p := range a.Params {
		for _, old := range p.Former {
			if !validShaderName(old) || names[old] {
				return badShader("conflicting former parameter name")
			}
			names[old] = true
		}
	}
	packed := make([]int, len(kinds))
	if shader.Std140(kinds, packed) != a.Size || !slices.Equal(packed, offsets) || maxLane != a.Lanes {
		return badShader("layout does not match std140 or instance packing")
	}
	if shader.FeatureLayoutHash(a.Domain, a.Params, a.Size, a.Lanes, a.Features) != a.LayoutHash {
		return badShader("layout hash mismatch")
	}
	return nil
}
