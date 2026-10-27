package asset

import (
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
)

const MaterialVersion = 1

// MaterialDoc is a parsed .aemat: material version=1 shader=asset(…) { blend=… variants=[…] params { … } }.
type MaterialDoc struct {
	Shader   ids.AssetID
	Blend    shader.Blend
	HasBlend bool
	Variants []string
	Values   []material.Value
	Textures map[string]ids.AssetID
	enums    map[string]string
	colors   map[string]bool
}

func badMaterial(name, msg string) error {
	return errs.New(errs.AssetBadArtifact, "material: "+msg, log.F("path", name))
}

func ParseMaterial(src []byte, name string) (*MaterialDoc, error) {
	n, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if n.Head != "material" {
		return nil, badMaterial(name, "not a material document")
	}
	if v, ok := n.Attr("version"); !ok || v.AsInt() != MaterialVersion {
		return nil, badMaterial(name, "version must be 1")
	}
	v, ok := n.Attr("shader")
	if !ok || v.Kind != aetext.VAsset {
		return nil, badMaterial(name, "shader=asset(…) is missing")
	}
	d := &MaterialDoc{Textures: map[string]ids.AssetID{}, enums: map[string]string{}, colors: map[string]bool{}}
	if d.Shader, err = ids.ParseAssetID(v.Str); err != nil || d.Shader.IsZero() {
		return nil, badMaterial(name, "shader is not an asset id")
	}
	for _, f := range n.Fields {
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
				if w := word(k); w != "" {
					d.Variants = append(d.Variants, w)
				} else {
					return nil, badMaterial(name, "variants is a list of keywords")
				}
			}
		default:
			return nil, badMaterial(name, "unknown field "+f.Name)
		}
	}
	for _, b := range n.Blocks {
		if b.Head != "params" {
			return nil, badMaterial(name, "unknown block "+b.Head)
		}
		for _, f := range b.Fields {
			val, err := d.value(f)
			if err != nil {
				return nil, badMaterial(name, f.Name+": "+err.Error())
			}
			d.Values = append(d.Values, val)
		}
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

func (d *MaterialDoc) value(f aetext.Field) (material.Value, error) {
	out := material.Value{Name: f.Name, Line: f.Pos.Line}
	v := f.Value
	switch v.Kind {
	case aetext.VInt, aetext.VFloat:
		out.Lanes[0], out.N = float32(v.AsFloat()), 1
	case aetext.VBool:
		if v.Bool {
			out.Lanes[0] = 1
		}
		out.N = 1
	case aetext.VColor:
		for c := 0; c < 4; c++ {
			out.Lanes[c] = float32(v.Color>>(24-8*c)&0xff) / 255
		}
		out.N = 4
		d.colors[f.Name] = true
	case aetext.VList:
		if len(v.List) == 0 || len(v.List) > 4 {
			return out, errs.New(errs.AssetBadArtifact, "a vector has 1 to 4 numbers")
		}
		for i, x := range v.List {
			if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
				return out, errs.New(errs.AssetBadArtifact, "a vector holds numbers")
			}
			out.Lanes[i] = float32(x.AsFloat())
		}
		out.N = len(v.List)
	case aetext.VAsset:
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return out, err
		}
		out.Asset = true
		d.Textures[f.Name] = id
	case aetext.VIdent:
		out.N = 1
		d.enums[f.Name] = v.Ident
	default:
		return out, errs.New(errs.AssetBadArtifact, "a parameter is a number, a vector, a color, a bool, an enum name or asset(…)")
	}
	return out, nil
}

// Deps are the shader and the textures the material names.
func (d *MaterialDoc) Deps() []ids.AssetID {
	out := []ids.AssetID{d.Shader}
	for _, id := range d.Textures {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.SortFunc(out[1:], func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

// Build makes the runtime material on sh: enum names become indexes, unknown parameters are skipped (MAT001).
func (d *MaterialDoc) Build(name string, sh *material.Shader, tex func(ids.AssetID) (material.Tex, bool)) (*material.Material, []material.Problem) {
	vals := slices.Clone(d.Values)
	for i := range vals {
		p, ok := sh.Schema.Param(vals[i].Name)
		if !ok {
			continue
		}
		if d.colors[p.Name] && p.Kind == shader.KindVec3 {
			vals[i].N = 3
		}
		if e, ok := d.enums[p.Name]; ok {
			vals[i].Lanes[0] = float32(slices.Index(p.Enum, e))
		}
	}
	probs := material.Validate(sh, d.Variants, vals)
	for _, p := range probs {
		if !p.Warn {
			return nil, probs
		}
	}
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
	for _, v := range vals {
		if v.Asset {
			if t, ok := tex(d.Textures[v.Name]); ok {
				m.SetTexture(v.Name, t)
			}
			continue
		}
		m.Set(v.Name, v.Lanes)
	}
	return m, probs
}
