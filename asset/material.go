package asset

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
)

const MaterialVersion = 1

// MaterialDoc preserves authoring semantics, including unknown properties and
// explicit values equal to a shader default. Values is a runtime projection;
// authoring edits use SetValue/Unset, not writes into that projection.
type MaterialDoc struct {
	Shader   ids.AssetID
	Blend    shader.Blend
	HasBlend bool
	Variants []string
	Values   []material.Value
	Textures map[string]ids.AssetID

	node   *aetext.Node
	name   string
	enums  map[string]string
	colors map[string]bool
}

func badMaterial(name, msg string) error {
	return errs.New(errs.AssetBadArtifact, "material: "+msg, log.F("path", name))
}

func ParseMaterial(src []byte, name string) (*MaterialDoc, error) {
	if len(src) > maxMaterialBytes/2 {
		return nil, badMaterial(name, "document exceeds size limit")
	}
	if !boundedMaterialSource(src) {
		return nil, badMaterial(name, "document exceeds nesting limit")
	}
	n, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	items := maxMaterialItems
	if !checkMaterialNode(n, 0, &items) {
		return nil, badMaterial(name, "document exceeds semantic limits or contains an invalid value")
	}
	return materialFromNode(n, name)
}

func materialFromNode(n *aetext.Node, name string) (*MaterialDoc, error) {
	if n == nil || n.Head != "material" {
		return nil, badMaterial(name, "not a material document")
	}
	if v, ok := n.Attr("version"); !ok || v.Kind != aetext.VInt || v.Int != MaterialVersion {
		return nil, badMaterial(name, "version must be 1")
	}
	v, ok := n.Attr("shader")
	if !ok || v.Kind != aetext.VAsset {
		return nil, badMaterial(name, "shader=asset(…) is missing")
	}
	d := &MaterialDoc{node: n, name: name}
	var err error
	if d.Shader, err = ids.ParseAssetID(v.Str); err != nil || d.Shader.IsZero() {
		return nil, badMaterial(name, "shader is not an asset id")
	}
	attrs := map[string]bool{}
	for _, f := range n.Attrs {
		if attrs[f.Name] {
			return nil, badMaterial(name, "duplicate attribute "+f.Name)
		}
		attrs[f.Name] = true
	}
	seen := map[string]bool{}
	for _, f := range n.Fields {
		if seen[f.Name] {
			return nil, badMaterial(name, "duplicate field "+f.Name)
		}
		seen[f.Name] = true
		switch f.Name {
		case "blend":
			b, ok := shader.ParseBlend(word(f.Value))
			if !ok {
				return nil, badMaterial(name, "blend is premultiplied, opaque or additive")
			}
			d.Blend, d.HasBlend = b, true
		case "variants":
			if f.Value.Kind != aetext.VList {
				return nil, badMaterial(name, "variants is a list of keywords")
			}
			for _, k := range f.Value.List {
				w := word(k)
				if !validShaderName(w) || slices.Contains(d.Variants, w) {
					return nil, badMaterial(name, "variant keywords must be valid and unique")
				}
				d.Variants = append(d.Variants, w)
			}
		}
	}
	params := 0
	for _, b := range n.Blocks {
		if b.Head == "params" {
			params++
		}
	}
	if params > 1 {
		return nil, badMaterial(name, "duplicate params block")
	}
	if err := d.refresh(); err != nil {
		return nil, err
	}
	return d, nil
}

func word(v aetext.Value) string {
	switch v.Kind {
	case aetext.VIdent:
		return v.Ident
	case aetext.VString:
		return v.Str
	}
	return ""
}

func (d *MaterialDoc) refresh() error {
	d.Values = nil
	d.Textures = map[string]ids.AssetID{}
	d.enums = map[string]string{}
	d.colors = map[string]bool{}
	seen := map[string]bool{}
	for _, b := range d.node.Blocks {
		if b.Head != "params" {
			continue
		}
		for _, f := range b.Fields {
			if seen[f.Name] {
				return badMaterial(d.name, "duplicate parameter "+f.Name)
			}
			seen[f.Name] = true
			v, err := d.project(f)
			if err != nil {
				return badMaterial(d.name, f.Name+": "+err.Error())
			}
			d.Values = append(d.Values, v)
		}
	}
	return nil
}

func (d *MaterialDoc) project(f aetext.Field) (material.Value, error) {
	out := material.Value{Name: f.Name, Line: f.Pos.Line, Typed: true, Kind: shader.Kind(255), N: -1}
	v := f.Value
	switch v.Kind {
	case aetext.VInt:
		out.Kind, out.N = shader.KindInt, 1
		out.Lanes[0] = float32(v.Int)
		if v.Int < math.MinInt32 || v.Int > math.MaxInt32 {
			out.N = -1
		} else {
			out.Int = int32(v.Int)
		}
	case aetext.VFloat:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) || math.Abs(v.Float) > math.MaxFloat32 {
			return out, fmt.Errorf("number is not a finite float32")
		}
		out.Kind, out.N, out.Lanes[0] = shader.KindFloat, 1, float32(v.Float)
	case aetext.VBool:
		out.Kind, out.N = shader.KindBool, 1
		if v.Bool {
			out.Lanes[0] = 1
		}
	case aetext.VColor:
		out.Color = true
		out.Kind, out.N = shader.KindVec4, 4
		for c := 0; c < 4; c++ {
			out.Lanes[c] = float32(v.Color>>(24-8*c)&0xff) / 255
		}
		d.colors[f.Name] = true
	case aetext.VList:
		if len(v.List) < 2 || len(v.List) > 4 {
			return out, nil
		}
		out.N = len(v.List)
		out.Kind = shader.Kind(int(shader.KindVec2) + out.N - 2)
		for i, x := range v.List {
			if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
				out.N = -1
				return out, nil
			}
			f := x.AsFloat()
			if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > math.MaxFloat32 {
				return out, fmt.Errorf("vector component is not a finite float32")
			}
			out.Lanes[i] = float32(f)
		}
	case aetext.VAsset:
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return out, err
		}
		out.Kind, out.N, out.Asset = shader.KindTexture, 0, true
		out.Reference = id
		d.Textures[f.Name] = id
	case aetext.VIdent:
		out.Symbol = v.Ident
		out.Kind, out.N = shader.KindInt, 1
		d.enums[f.Name] = v.Ident
	}
	return out, nil
}

// Value reports only an explicit source value; absence means inherited.
func (d *MaterialDoc) Value(name string) (aetext.Value, bool) {
	if d.node != nil {
		if b := d.node.Block("params", ""); b != nil {
			if v, ok := b.Field(name); ok {
				return v.Clone(), true
			}
		}
	}
	return aetext.Value{}, false
}

func (d *MaterialDoc) SetValue(name string, v aetext.Value) error {
	items := maxMaterialItems
	if !checkMaterialValue(v, 2, &items) {
		return badMaterial(d.name, "parameter exceeds semantic limits or has an invalid value")
	}
	if !validShaderName(name) {
		return badMaterial(d.name, "invalid parameter name "+name)
	}
	if d.node == nil {
		d.node = d.documentNode()
	}
	old := d.node
	d.node = old.Clone()
	b := d.node.Block("params", "")
	if b == nil {
		b = &aetext.Node{Head: "params"}
		d.node.Blocks = append(d.node.Blocks, b)
	}
	b.SetField(name, v.Clone())
	items = maxMaterialItems
	if !checkMaterialNode(d.node, 0, &items) {
		d.node = old
		return badMaterial(d.name, "document exceeds semantic limits")
	}
	if err := d.refresh(); err != nil {
		d.node = old
		_ = d.refresh()
		return err
	}
	return nil
}

func (d *MaterialDoc) Unset(name string) bool {
	if d.node == nil {
		return false
	}
	b := d.node.Block("params", "")
	if b == nil || !b.RemoveField(name) {
		return false
	}
	_ = d.refresh()
	return true
}

func setAttr(n *aetext.Node, name string, v aetext.Value) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs[i].Value = v
			return
		}
	}
	n.Attrs = append(n.Attrs, aetext.Field{Name: name, Value: v})
}

func (d *MaterialDoc) documentNode() *aetext.Node {
	n := d.node.Clone()
	if n == nil {
		n = &aetext.Node{Head: "material"}
	}
	setAttr(n, "version", aetext.Int(MaterialVersion))
	setAttr(n, "shader", aetext.AssetRef(d.Shader.String()))
	if d.HasBlend {
		n.SetField("blend", aetext.Ident(d.Blend.String()))
	} else {
		n.RemoveField("blend")
	}
	if len(d.Variants) > 0 {
		keywords := slices.Clone(d.Variants)
		slices.Sort(keywords)
		items := make([]aetext.Value, len(keywords))
		for i, k := range keywords {
			items[i] = aetext.Ident(k)
		}
		n.SetField("variants", aetext.List(items...))
	} else {
		n.RemoveField("variants")
	}
	return n
}

// MarshalMaterial writes the current document while preserving unknown fields,
// blocks, references and explicit parameter values.
func MarshalMaterial(d *MaterialDoc) []byte { return aetext.Write(d.documentNode()) }

func (d *MaterialDoc) Deps() []ids.AssetID {
	out := []ids.AssetID{d.Shader}
	for _, id := range d.Textures {
		if !id.IsZero() && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.SortFunc(out[1:], func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

func (d *MaterialDoc) preparedValues(sh *material.Shader) []material.Value {
	vals := slices.Clone(d.Values)
	for i := range vals {
		v := &vals[i]
		p, ok := sh.Schema.Param(v.Name)
		if !ok {
			continue
		}
		original := v.Name
		v.Name = p.Name
		if v.Kind == shader.KindInt && p.Kind == shader.KindFloat && v.N == 1 {
			// Integer literals are valid numeric float values. No float is ever
			// silently converted into an integer or boolean material parameter.
			v.Kind = shader.KindFloat
		}
		if d.colors[original] {
			if p.Kind == shader.KindVec3 {
				v.Kind, v.N = shader.KindVec3, 3
			}
			if p.Color {
				for c := 0; c < 3; c++ {
					v.Lanes[c] = mathx.SRGBToLinear(v.Lanes[c])
				}
			}
		}
		if enum, ok := d.enums[original]; ok {
			v.Int = int32(slices.Index(p.Enum, enum))
			v.Lanes[0] = float32(v.Int)
			if v.Int < 0 {
				v.N = -1
			}
		}
	}
	return vals
}

// Validate is independent of GPU availability and performs the same typed
// projection as Build. Unknown semantic data yields warnings and is retained.
func (d *MaterialDoc) Validate(sh *material.Shader) []material.Problem {
	if sh == nil || sh.Schema == nil {
		return []material.Problem{{Code: material.MAT004, Msg: "shader schema is unavailable"}}
	}
	values := d.preparedValues(sh)
	out := material.Validate(sh, d.Variants, values)
	seen := map[string]bool{}
	for _, v := range values {
		if _, known := sh.Schema.Param(v.Name); !known {
			continue
		}
		if seen[v.Name] {
			out = append(out, material.Problem{Code: material.MAT002, Line: v.Line, Msg: v.Name + " is explicitly assigned through multiple current/former names"})
		}
		seen[v.Name] = true
	}
	return out
}

func (d *MaterialDoc) Build(name string, sh *material.Shader, tex func(ids.AssetID) (material.Tex, bool)) (*material.Material, []material.Problem) {
	probs := d.Validate(sh)
	for _, p := range probs {
		if !p.Warn {
			return nil, probs
		}
	}
	vals := d.preparedValues(sh)
	var mask uint32
	for _, k := range d.Variants {
		mask |= 1 << slices.Index(sh.Keywords, k)
	}
	blend := sh.Blend
	if d.HasBlend {
		blend = d.Blend
	}
	m, ok := material.New(name, sh, mask, blend)
	if !ok {
		return nil, append(probs, material.Problem{Code: material.MAT003, Msg: sh.Name + " has no such variant"})
	}
	for i, v := range vals {
		if _, known := sh.Schema.Param(v.Name); !known {
			var resolvedTexture material.Tex
			resolved := v.Reference.IsZero()
			if v.Asset && !resolved && tex != nil {
				resolvedTexture, resolved = tex(v.Reference)
			}
			m.RetainUnknown(v, resolvedTexture, resolved)
			continue
		}
		if v.Asset {
			id := d.Textures[d.Values[i].Name]
			if id.IsZero() {
				m.SetTexture(v.Name, material.Tex{})
				continue
			}
			if tex != nil {
				if t, ok := tex(id); ok {
					p, _ := sh.Schema.Param(v.Name)
					if p.Normal && !t.NormalMap {
						return nil, append(probs, material.Problem{Code: material.MAT004, Line: v.Line, Msg: v.Name + " requires an asset imported with normal semantics (linear, without premultiplication)"})
					}
					if p.Emission && t.EmissionMode == 0 {
						return nil, append(probs, material.Problem{Code: material.MAT004, Line: v.Line, Msg: v.Name + " requires an asset imported as emission RGB or emission mask"})
					}
					if !m.SetTexture(v.Name, t) {
						return nil, append(probs, material.Problem{Code: material.MAT004, Line: v.Line, Msg: v.Name + " requires a compatible texture; only @normal and @emission accept their own region mapping"})
					}
					continue
				}
			}
			if m.SetUnresolvedTexture(v.Name, id) {
				p, _ := sh.Schema.Param(v.Name)
				kind, fallback := "normal", "flat fallback"
				if p.Emission {
					kind, fallback = "emission", "uniform emission fallback"
				}
				probs = append(probs, material.Problem{Code: material.MAT004, Line: v.Line, Warn: true, Msg: fmt.Sprintf("explicit %s %s (%s) is unresolved; reference retained, %s", kind, v.Name, id.String(), fallback)})
				continue
			}
			return nil, append(probs, material.Problem{Code: material.MAT004, Line: v.Line, Msg: fmt.Sprintf("%s texture asset %s cannot be resolved", v.Name, id.String())})
		}
		if v.Kind == shader.KindInt {
			m.SetInt(v.Name, v.Int)
		} else {
			m.Set(v.Name, v.Lanes)
		}
	}
	return m, probs
}
