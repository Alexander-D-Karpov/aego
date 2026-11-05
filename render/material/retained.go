package material

import (
	"fmt"
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/shader"
)

// RetainUnknown keeps explicit authoring data absent from the current shader without uploading it; a later schema addition resolves the original typed value or rejects the entire reload transaction.
func (m *Material) RetainUnknown(v Value, t Tex, resolved bool) bool {
	if _, ok := m.shader.Schema.Param(v.Name); ok {
		return false
	}
	copyValue := v
	m.values[v.Name] = storedValue{kind: v.Kind, tex: t, pending: &copyValue, resolved: resolved}
	return true
}

// ResolveRetainedTexture updates a dependency without changing shader/schema
// state. The loader calls this on a staged CPU definition, not on a live frame.
func (m *Material) ResolveRetainedTexture(name string, t Tex) bool {
	v, ok := m.values[name]
	if !ok || v.pending == nil || !v.pending.Asset {
		return false
	}
	v.tex, v.resolved = t, true
	m.values[name] = v
	return true
}

func (m *Material) RetainedReference(name string) (ids.AssetID, bool) {
	v, ok := m.values[name]
	if !ok || v.pending == nil || !v.pending.Asset {
		return ids.AssetID{}, false
	}
	return v.pending.Reference, true
}

func resolvePending(p *shader.Param, stored storedValue) (storedValue, error) {
	v := *stored.pending
	bad := func(reason string) (storedValue, error) {
		return storedValue{}, fmt.Errorf("MAT002: retained explicit parameter %s %s", p.Name, reason)
	}
	if v.Symbol != "" {
		if p.Kind != shader.KindInt {
			return bad("is an enum name, but the new parameter is not int")
		}
		i := slices.Index(p.Enum, v.Symbol)
		if i < 0 {
			return bad("names an unavailable enum value " + v.Symbol)
		}
		v.Kind, v.N, v.Int = shader.KindInt, 1, int32(i)
	}
	if v.Color {
		if p.Kind != shader.KindVec3 && p.Kind != shader.KindVec4 {
			return bad("is a color, but the new parameter is not a vector")
		}
		v.Kind, v.N = p.Kind, p.Kind.Lanes()
		if p.Color {
			for i := 0; i < 3; i++ {
				v.Lanes[i] = mathx.SRGBToLinear(v.Lanes[i])
			}
		}
	}
	if v.Kind == shader.KindInt && p.Kind == shader.KindFloat && v.Symbol == "" && v.N == 1 {
		v.Kind = shader.KindFloat
	}
	if v.Kind != p.Kind || v.N != p.Kind.Lanes() {
		return bad("has an incompatible type or component count")
	}
	out := storedValue{kind: p.Kind, tex: stored.tex, resolved: stored.resolved}
	if p.Kind == shader.KindTexture {
		if !v.Reference.IsZero() && !stored.resolved {
			return storedValue{}, fmt.Errorf("MAT004: retained texture %s (%s) is unresolved", p.Name, v.Reference.String())
		}
		return out, nil
	}
	if p.Kind == shader.KindInt {
		out.words[0] = uint32(v.Int)
		return out, nil
	}
	w, ok := words(p, v.Lanes)
	if !ok {
		return bad("is not a finite, valid parameter value")
	}
	out.words = w
	return out, nil
}
